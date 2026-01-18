package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/scene"
)

// ==========================================================================
// Constants
// ==========================================================================

const (
	// DefaultPollInterval is the default interval between job polls.
	DefaultPollInterval = 10 * time.Second

	// DefaultMaxRetries is the default maximum number of retry attempts.
	DefaultMaxRetries = 3

	// StaleJobTimeout is the duration after which a claimed job is considered stale.
	StaleJobTimeout = 5 * time.Minute

	// MaxPendingJobs is the maximum number of pending jobs to fetch per poll.
	MaxPendingJobs = 100
)

// ==========================================================================
// SceneExecutor Interface
// ==========================================================================

// SceneExecutor defines the interface for scene execution.
// This allows dependency injection for testing and decouples the scheduler from the scene package.
type SceneExecutor interface {
	ExecuteScene(sceneID string, idempotencyKey *string, options scene.ExecuteOptions) (*scene.SceneExecution, error)
}

// ==========================================================================
// JobRunner
// ==========================================================================

// JobRunner is responsible for polling, claiming, and executing scheduled jobs.
// It handles:
// - Polling for pending jobs at a configurable interval
// - Claiming and executing jobs atomically
// - Retry logic with exponential backoff
// - Recovery of stale claimed jobs after crashes
type JobRunner struct {
	logger       *log.Logger
	jobsRepo     *JobsRepository
	routinesRepo *RoutinesRepository
	sceneService SceneExecutor
	pollInterval time.Duration
	maxRetries   int
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// NewJobRunner creates a new JobRunner instance.
func NewJobRunner(
	logger *log.Logger,
	jobsRepo *JobsRepository,
	routinesRepo *RoutinesRepository,
	sceneService SceneExecutor,
	pollInterval time.Duration,
	maxRetries int,
) *JobRunner {
	if logger == nil {
		logger = log.Default()
	}
	if pollInterval <= 0 {
		pollInterval = DefaultPollInterval
	}
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	return &JobRunner{
		logger:       logger,
		jobsRepo:     jobsRepo,
		routinesRepo: routinesRepo,
		sceneService: sceneService,
		pollInterval: pollInterval,
		maxRetries:   maxRetries,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the polling loop in a goroutine.
// It first recovers any stale claimed jobs, then starts polling for pending jobs.
func (r *JobRunner) Start() {
	r.logger.Printf("Job runner starting with poll interval: %v, max retries: %d", r.pollInterval, r.maxRetries)

	// Recover stale jobs on startup
	r.recoverStaleJobs()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.runPollLoop()
	}()
}

// Stop gracefully stops the runner.
// It signals the polling loop to stop and waits for it to complete.
func (r *JobRunner) Stop() {
	r.logger.Println("Job runner stopping...")
	close(r.stopCh)
	r.wg.Wait()
	r.logger.Println("Job runner stopped")
}

// runPollLoop runs the main polling loop until stopped.
func (r *JobRunner) runPollLoop() {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	// Do an initial poll immediately
	r.poll()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.poll()
		}
	}
}

// poll checks for pending jobs and executes them.
func (r *JobRunner) poll() {
	jobs, err := r.jobsRepo.GetPendingJobs(MaxPendingJobs)
	if err != nil {
		r.logger.Printf("Error fetching pending jobs: %v", err)
		return
	}

	if len(jobs) == 0 {
		return
	}

	r.logger.Printf("Found %d pending job(s)", len(jobs))

	now := time.Now().UTC()
	for i := range jobs {
		job := &jobs[i]

		// Only execute jobs that are due
		if job.ScheduledFor.After(now) {
			continue
		}

		// Check retry_after if set
		if job.RetryAfter != nil && job.RetryAfter.After(now) {
			continue
		}

		if err := r.executeJob(job); err != nil {
			r.logger.Printf("Error executing job %s: %v", job.JobID, err)
		}
	}
}

// executeJob claims and runs a single job.
func (r *JobRunner) executeJob(job *Job) error {
	r.logger.Printf("Claiming job %s (routine: %s, scheduled: %s)",
		job.JobID, job.RoutineID, job.ScheduledFor.Format(time.RFC3339))

	// Step 1: Claim job (atomic status update)
	if err := r.jobsRepo.ClaimJob(job.JobID); err != nil {
		return fmt.Errorf("failed to claim job: %w", err)
	}

	// Step 2: Start job (set status=RUNNING)
	if err := r.jobsRepo.StartJob(job.JobID); err != nil {
		// Job was claimed but we failed to start it - mark for retry
		r.handleJobFailure(job, fmt.Errorf("failed to start job: %w", err))
		return err
	}

	// Step 3: Get routine for job
	routine, err := r.routinesRepo.GetByID(job.RoutineID)
	if err != nil {
		r.handleJobFailure(job, fmt.Errorf("failed to get routine: %w", err))
		return err
	}
	if routine == nil {
		err := fmt.Errorf("routine not found: %s", job.RoutineID)
		r.handleJobFailure(job, err)
		return err
	}

	// Step 4: Execute scene with routine's scene_id
	execution, err := r.sceneService.ExecuteScene(
		routine.SceneID,
		job.IdempotencyKey,
		scene.ExecuteOptions{},
	)
	if err != nil {
		r.handleJobFailure(job, err)
		return err
	}

	// Step 5: Complete job with result
	sceneExecutionID := ""
	if execution != nil {
		sceneExecutionID = execution.SceneExecutionID
	}

	if err := r.jobsRepo.CompleteJob(job.JobID, sceneExecutionID); err != nil {
		r.logger.Printf("Warning: failed to mark job %s as completed: %v", job.JobID, err)
		// Don't return error here - the job was actually executed
	}

	// Step 6: Update routine's last_run_at
	if err := r.routinesRepo.UpdateLastRunAt(job.RoutineID, time.Now().UTC()); err != nil {
		r.logger.Printf("Warning: failed to update last_run_at for routine %s: %v", job.RoutineID, err)
		// Don't return error - this is not critical
	}

	r.logger.Printf("Job %s completed successfully (execution: %s)", job.JobID, sceneExecutionID)
	return nil
}

// handleJobFailure processes a job failure with retry logic.
func (r *JobRunner) handleJobFailure(job *Job, execErr error) {
	errMsg := execErr.Error()
	attempts := job.Attempts + 1

	// Check if we can retry
	canRetry := attempts < r.maxRetries

	if canRetry {
		// Calculate exponential backoff: 1s, 2s, 4s, 8s, etc.
		backoffSeconds := 1 << attempts // 2^attempts
		retryAfter := time.Now().UTC().Add(time.Duration(backoffSeconds) * time.Second)

		r.logger.Printf("Job %s failed (attempt %d/%d): %s. Will retry after %s",
			job.JobID, attempts, r.maxRetries, errMsg, retryAfter.Format(time.RFC3339))

		// Set retry_after for the job
		if err := r.jobsRepo.SetRetryAfter(job.JobID, retryAfter); err != nil {
			r.logger.Printf("Warning: failed to set retry_after for job %s: %v", job.JobID, err)
		}
	} else {
		r.logger.Printf("Job %s failed permanently after %d attempts: %s",
			job.JobID, attempts, errMsg)
	}

	// Update job status
	if err := r.jobsRepo.FailJob(job.JobID, errMsg, canRetry); err != nil {
		r.logger.Printf("Error updating failed job %s: %v", job.JobID, err)
	}
}

// recoverStaleJobs finds jobs that were claimed but not completed and resets them.
// This handles crash recovery scenarios where a job runner died while processing a job.
func (r *JobRunner) recoverStaleJobs() {
	staleJobs, err := r.jobsRepo.GetStaleClaimedJobs(StaleJobTimeout)
	if err != nil {
		r.logger.Printf("Error fetching stale jobs: %v", err)
		return
	}

	if len(staleJobs) == 0 {
		r.logger.Println("No stale jobs to recover")
		return
	}

	r.logger.Printf("Found %d stale claimed job(s) to recover", len(staleJobs))

	for _, job := range staleJobs {
		claimedAtStr := "unknown"
		if job.ClaimedAt != nil {
			claimedAtStr = job.ClaimedAt.Format(time.RFC3339)
		}
		r.logger.Printf("Recovering stale job %s (claimed at: %s)",
			job.JobID, claimedAtStr)

		// Reset job to pending status for retry
		errMsg := "job recovered after stale claim timeout"
		canRetry := job.Attempts < r.maxRetries

		if err := r.jobsRepo.FailJob(job.JobID, errMsg, canRetry); err != nil {
			r.logger.Printf("Error recovering stale job %s: %v", job.JobID, err)
			continue
		}

		if canRetry {
			r.logger.Printf("Stale job %s reset to pending for retry (attempt %d/%d)",
				job.JobID, job.Attempts+1, r.maxRetries)
		} else {
			r.logger.Printf("Stale job %s marked as failed (max retries exceeded)",
				job.JobID)
		}
	}
}

// RecoverStaleRunningJobs returns jobs in RUNNING state that are stale.
// This is useful for recovering from crashes during execution.
func (r *JobRunner) RecoverStaleRunningJobs(olderThan time.Duration) ([]Job, error) {
	return r.jobsRepo.GetStaleRunningJobs(olderThan)
}
