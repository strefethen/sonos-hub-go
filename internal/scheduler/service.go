package scheduler

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/scene"
)

// Service provides scheduler management functionality.
type Service struct {
	cfg             config.Config
	logger          *log.Logger
	reader          *sql.DB // For ad-hoc read queries
	writer          *sql.DB // For ad-hoc write queries
	routinesRepo    *RoutinesRepository
	jobsRepo        *JobsRepository
	holidaysRepo    *HolidaysRepository
	generator       *JobGenerator
	runner          *JobRunner
	routineExecutor RoutineExecutor

	// Runner control
	stopChan chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
}

// NewService creates a new scheduler service.
// Accepts a DBPair for optimal SQLite concurrency with separate reader/writer pools.
func NewService(
	cfg config.Config,
	dbPair DBPair,
	logger *log.Logger,
	routineExecutor RoutineExecutor,
) *Service {
	if logger == nil {
		logger = log.Default()
	}

	routinesRepo := NewRoutinesRepository(dbPair)
	jobsRepo := NewJobsRepository(dbPair)
	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(routinesRepo, jobsRepo, holidaysRepo, logger)

	// Create job runner
	runner := NewJobRunner(
		logger,
		jobsRepo,
		routinesRepo,
		routineExecutor,
		DefaultPollInterval,
		DefaultMaxRetries,
	)

	return &Service{
		cfg:             cfg,
		logger:          logger,
		reader:          dbPair.Reader(),
		writer:          dbPair.Writer(),
		routinesRepo:    routinesRepo,
		jobsRepo:        jobsRepo,
		holidaysRepo:    holidaysRepo,
		generator:       generator,
		runner:          runner,
		routineExecutor: routineExecutor,
		stopChan:        make(chan struct{}),
	}
}

// ==========================================================================
// Lifecycle
// ==========================================================================

// Start starts the job runner and generation ticker.
func (s *Service) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	s.logger.Printf("Starting scheduler service")

	// Start job runner (it has its own goroutine management)
	s.runner.Start()

	// Start job generation ticker
	s.wg.Add(1)
	go s.runGenerationTicker()
}

// IsRunning returns true if the scheduler is currently running.
func (s *Service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Stop stops the job runner and generation ticker.
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.logger.Printf("Stopping scheduler service")

	// Stop the job runner
	s.runner.Stop()

	// Wait for generation ticker to stop
	s.wg.Wait()
	s.logger.Printf("Scheduler service stopped")
}

func (s *Service) runGenerationTicker() {
	defer s.wg.Done()

	ticker := time.NewTicker(60 * time.Second) // Generate jobs every minute
	defer ticker.Stop()

	// Generate jobs immediately on start
	if count, err := s.GenerateUpcomingJobs(); err != nil {
		s.logger.Printf("Error generating jobs on start: %v", err)
	} else if count > 0 {
		s.logger.Printf("Generated %d job(s) on startup", count)
	}

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if count, err := s.GenerateUpcomingJobs(); err != nil {
				s.logger.Printf("Error generating jobs: %v", err)
			} else if count > 0 {
				s.logger.Printf("Generated %d job(s)", count)
			}
		}
	}
}

// ==========================================================================
// Routine CRUD
// ==========================================================================

// CreateRoutine creates a new routine.
func (s *Service) CreateRoutine(input CreateRoutineInput) (*Routine, error) {
	// Validate scene_id exists
	if s.routineExecutor != nil {
		sceneExists, err := s.validateSceneExists(input.SceneID)
		if err != nil {
			return nil, fmt.Errorf("failed to validate scene: %w", err)
		}
		if !sceneExists {
			return nil, &SceneNotFoundError{SceneID: input.SceneID}
		}
	}

	return s.routinesRepo.Create(input)
}

// GetRoutine retrieves a routine by ID.
func (s *Service) GetRoutine(routineID string) (*Routine, error) {
	routine, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if routine == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}
	return routine, nil
}

// ListRoutines retrieves routines with pagination and optional filtering.
func (s *Service) ListRoutines(limit, offset int, enabledOnly bool) ([]Routine, int, error) {
	return s.routinesRepo.List(limit, offset, enabledOnly)
}

// UpdateRoutine updates a routine.
func (s *Service) UpdateRoutine(routineID string, input UpdateRoutineInput) (*Routine, error) {
	// Check if routine exists
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	// Validate scene_id if being updated
	if input.SceneID != nil && s.routineExecutor != nil {
		sceneExists, err := s.validateSceneExists(*input.SceneID)
		if err != nil {
			return nil, fmt.Errorf("failed to validate scene: %w", err)
		}
		if !sceneExists {
			return nil, &SceneNotFoundError{SceneID: *input.SceneID}
		}
	}

	return s.routinesRepo.Update(routineID, input)
}

// DeleteRoutine deletes a routine.
// Returns an error if the routine has pending jobs.
func (s *Service) DeleteRoutine(routineID string) error {
	// Check if routine exists
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return err
	}
	if existing == nil {
		return &RoutineNotFoundError{RoutineID: routineID}
	}

	// Check for pending jobs
	jobs, total, err := s.jobsRepo.ListByRoutineID(routineID, 100, 0)
	if err != nil {
		return err
	}

	// Count pending jobs
	pendingCount := 0
	for _, job := range jobs {
		if job.Status == JobStatusPending || job.Status == JobStatusScheduled || job.Status == JobStatusClaimed {
			pendingCount++
		}
	}
	if pendingCount > 0 {
		return &RoutineHasPendingJobsError{RoutineID: routineID, JobCount: pendingCount}
	}

	// Delete associated jobs first (cascade delete)
	if total > 0 {
		_, err = s.writer.Exec("DELETE FROM jobs WHERE routine_id = ?", routineID)
		if err != nil {
			return fmt.Errorf("failed to delete associated jobs: %w", err)
		}
	}

	return s.routinesRepo.Delete(routineID)
}

// validateSceneExists checks if a scene exists by attempting to execute it with a nil idempotency key.
// This is a helper that uses the scene service to verify the scene exists.
func (s *Service) validateSceneExists(sceneID string) (bool, error) {
	// Query the scenes table directly to check existence
	var count int
	err := s.reader.QueryRow("SELECT COUNT(*) FROM scenes WHERE scene_id = ?", sceneID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ==========================================================================
// Routine Controls
// ==========================================================================

// EnableRoutine enables a routine.
func (s *Service) EnableRoutine(routineID string) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	enabled := true
	return s.routinesRepo.Update(routineID, UpdateRoutineInput{Enabled: &enabled})
}

// DisableRoutine disables a routine.
func (s *Service) DisableRoutine(routineID string) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	enabled := false
	return s.routinesRepo.Update(routineID, UpdateRoutineInput{Enabled: &enabled})
}

// TriggerRoutine manually triggers a routine, creating a job scheduled for now.
func (s *Service) TriggerRoutine(routineID string) (*Job, error) {
	routine, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if routine == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	// Create job with scheduled_for = now (bypasses normal schedule)
	now := time.Now().UTC()
	idempotencyKey := fmt.Sprintf("manual:%s:%d", routineID, now.UnixNano())

	job, err := s.jobsRepo.Create(CreateJobInput{
		RoutineID:      routineID,
		ScheduledFor:   now,
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create triggered job: %w", err)
	}

	return job, nil
}

// SnoozeRoutine snoozes a routine until the specified time.
func (s *Service) SnoozeRoutine(routineID string, until time.Time) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	return s.routinesRepo.Update(routineID, UpdateRoutineInput{SnoozeUntil: &until})
}

// UnsnoozeRoutine removes the snooze from a routine.
func (s *Service) UnsnoozeRoutine(routineID string) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	return s.clearSnooze(routineID)
}

// clearSnooze directly clears the snooze_until field.
func (s *Service) clearSnooze(routineID string) (*Routine, error) {
	now := nowISO()
	_, err := s.writer.Exec(`
		UPDATE routines SET snooze_until = NULL, updated_at = ?
		WHERE routine_id = ?
	`, now, routineID)
	if err != nil {
		return nil, err
	}
	return s.routinesRepo.GetByID(routineID)
}

// SkipNextRoutine sets skip_next=true on the routine, causing the next scheduled job to be skipped.
func (s *Service) SkipNextRoutine(routineID string) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	skipNext := true
	return s.routinesRepo.Update(routineID, UpdateRoutineInput{SkipNext: &skipNext})
}

// ClearSkipNext clears the skip_next flag.
func (s *Service) ClearSkipNext(routineID string) (*Routine, error) {
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &RoutineNotFoundError{RoutineID: routineID}
	}

	skipNext := false
	return s.routinesRepo.Update(routineID, UpdateRoutineInput{SkipNext: &skipNext})
}

// ==========================================================================
// Job Queries
// ==========================================================================

// GetJob retrieves a job by ID.
func (s *Service) GetJob(jobID string) (*Job, error) {
	job, err := s.jobsRepo.GetByID(jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, &JobNotFoundError{JobID: jobID}
	}
	return job, nil
}

// ListJobsForRoutine retrieves jobs for a routine with pagination.
func (s *Service) ListJobsForRoutine(routineID string, limit, offset int) ([]Job, int, error) {
	// Verify routine exists
	existing, err := s.routinesRepo.GetByID(routineID)
	if err != nil {
		return nil, 0, err
	}
	if existing == nil {
		return nil, 0, &RoutineNotFoundError{RoutineID: routineID}
	}

	return s.jobsRepo.ListByRoutineID(routineID, limit, offset)
}

// ==========================================================================
// Holiday Management
// ==========================================================================

// CreateHoliday creates a new holiday.
func (s *Service) CreateHoliday(input CreateHolidayInput) (*Holiday, error) {
	return s.holidaysRepo.Create(input)
}

// GetHoliday retrieves a holiday by ID (date string).
func (s *Service) GetHoliday(holidayID string) (*Holiday, error) {
	holiday, err := s.holidaysRepo.GetByID(holidayID)
	if err != nil {
		return nil, err
	}
	if holiday == nil {
		return nil, &HolidayNotFoundError{HolidayID: holidayID}
	}
	return holiday, nil
}

// ListHolidays retrieves holidays with pagination.
func (s *Service) ListHolidays(limit, offset int) ([]Holiday, int, error) {
	return s.holidaysRepo.List(limit, offset)
}

// DeleteHoliday deletes a holiday.
func (s *Service) DeleteHoliday(holidayID string) error {
	err := s.holidaysRepo.Delete(holidayID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &HolidayNotFoundError{HolidayID: holidayID}
		}
		return err
	}
	return nil
}

// IsHoliday checks if a date is a holiday.
func (s *Service) IsHoliday(date time.Time) (bool, *Holiday, error) {
	return s.holidaysRepo.IsHoliday(date)
}

// ==========================================================================
// Job Generation
// ==========================================================================

// GenerateUpcomingJobs generates jobs for all due routines.
// This is called periodically by the generation ticker.
func (s *Service) GenerateUpcomingJobs() (int, error) {
	return s.generator.GenerateJobs(time.Now())
}

// ==========================================================================
// Scene Service Adapter
// ==========================================================================

// SceneServiceAdapter wraps a scene.Service to implement SceneExecutor.
type SceneServiceAdapter struct {
	sceneService *scene.Service
}

// NewSceneServiceAdapter creates a new adapter for scene.Service.
func NewSceneServiceAdapter(sceneService *scene.Service) *SceneServiceAdapter {
	return &SceneServiceAdapter{sceneService: sceneService}
}

// ExecuteScene implements SceneExecutor interface.
func (a *SceneServiceAdapter) ExecuteScene(sceneID string, idempotencyKey *string, options scene.ExecuteOptions) (*scene.SceneExecution, error) {
	return a.sceneService.ExecuteScene(sceneID, idempotencyKey, options)
}

// ==========================================================================
// Error Types
// ==========================================================================

// RoutineNotFoundError is returned when a routine is not found.
type RoutineNotFoundError struct {
	RoutineID string
}

func (e *RoutineNotFoundError) Error() string {
	return fmt.Sprintf("routine not found: %s", e.RoutineID)
}

// RoutineHasPendingJobsError is returned when trying to delete a routine with pending jobs.
type RoutineHasPendingJobsError struct {
	RoutineID string
	JobCount  int
}

func (e *RoutineHasPendingJobsError) Error() string {
	return fmt.Sprintf("routine %s has %d pending jobs and cannot be deleted", e.RoutineID, e.JobCount)
}

// JobNotFoundError is returned when a job is not found.
type JobNotFoundError struct {
	JobID string
}

func (e *JobNotFoundError) Error() string {
	return fmt.Sprintf("job not found: %s", e.JobID)
}

// HolidayNotFoundError is returned when a holiday is not found.
type HolidayNotFoundError struct {
	HolidayID string
}

func (e *HolidayNotFoundError) Error() string {
	return fmt.Sprintf("holiday not found: %s", e.HolidayID)
}

// SceneNotFoundError is returned when a scene is not found.
type SceneNotFoundError struct {
	SceneID string
}

func (e *SceneNotFoundError) Error() string {
	return fmt.Sprintf("scene not found: %s", e.SceneID)
}
