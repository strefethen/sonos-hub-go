package scene

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// Service provides scene management functionality.
type Service struct {
	cfg           config.Config
	logger        *log.Logger
	reader        *sql.DB // For ad-hoc read queries
	scenesRepo    *ScenesRepository
	execRepo      *ExecutionsRepository
	lock          *CoordinatorLock
	preflight     *PreFlightChecker
	executor      *Executor
	deviceService *devices.Service
	soapClient    *soap.Client
}

// NewService creates a new scene service.
// Accepts a DBPair for optimal SQLite concurrency with separate reader/writer pools.
func NewService(
	cfg config.Config,
	dbPair DBPair,
	logger *log.Logger,
	deviceService *devices.Service,
	soapClient *soap.Client,
) *Service {
	if logger == nil {
		logger = log.Default()
	}

	timeout := time.Duration(cfg.SonosTimeoutMs) * time.Millisecond

	scenesRepo := NewScenesRepository(dbPair)
	execRepo := NewExecutionsRepository(dbPair)
	lock := NewCoordinatorLock(logger)
	preflight := NewPreFlightChecker(soapClient, timeout, logger)
	executor := NewExecutor(logger, execRepo, lock, preflight, deviceService, soapClient, timeout)

	return &Service{
		cfg:           cfg,
		logger:        logger,
		reader:        dbPair.Reader(),
		scenesRepo:    scenesRepo,
		execRepo:      execRepo,
		lock:          lock,
		preflight:     preflight,
		executor:      executor,
		deviceService: deviceService,
		soapClient:    soapClient,
	}
}

// CreateScene creates a new scene.
func (s *Service) CreateScene(input CreateSceneInput) (*Scene, error) {
	return s.scenesRepo.Create(input)
}

// GetScene retrieves a scene by ID.
func (s *Service) GetScene(sceneID string) (*Scene, error) {
	return s.scenesRepo.GetByID(sceneID)
}

// ListScenes retrieves scenes with pagination.
func (s *Service) ListScenes(limit, offset int) ([]Scene, int, error) {
	return s.scenesRepo.List(limit, offset)
}

// UpdateScene updates a scene.
func (s *Service) UpdateScene(sceneID string, input UpdateSceneInput) (*Scene, error) {
	return s.scenesRepo.Update(sceneID, input)
}

// DeleteScene deletes a scene.
// Returns an error if the scene is referenced by routines.
func (s *Service) DeleteScene(sceneID string) error {
	// Check if scene is referenced by routines
	var count int
	err := s.reader.QueryRow("SELECT COUNT(*) FROM routines WHERE scene_id = ?", sceneID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if count > 0 {
		return &SceneInUseError{SceneID: sceneID, RoutineCount: count}
	}

	return s.scenesRepo.Delete(sceneID)
}

// ExecuteScene starts an async execution of a scene.
// Returns immediately with the execution record; actual execution happens in background.
func (s *Service) ExecuteScene(sceneID string, idempotencyKey *string, options ExecuteOptions) (*SceneExecution, error) {
	// Check for existing execution with same idempotency key
	if idempotencyKey != nil && *idempotencyKey != "" {
		existing, err := s.execRepo.GetByIdempotencyKey(*idempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			s.logger.Printf("Returning existing execution for idempotency key: %s", *idempotencyKey)
			return existing, nil
		}
	}

	// Verify scene exists
	scene, err := s.scenesRepo.GetByID(sceneID)
	if err != nil {
		return nil, err
	}
	if scene == nil {
		return nil, &SceneNotFoundError{SceneID: sceneID}
	}

	// Create execution record
	execution, err := s.execRepo.Create(CreateExecutionInput{
		SceneID:        sceneID,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	// Start async execution
	go func() {
		if _, err := s.executor.Execute(scene, execution, options); err != nil {
			s.logger.Printf("Scene execution failed: %v", err)
			// Ensure execution is marked as failed
			current, _ := s.execRepo.GetByID(execution.SceneExecutionID)
			if current != nil && current.Status == ExecutionStatusStarting {
				errMsg := err.Error()
				_ = s.execRepo.Complete(execution.SceneExecutionID, ExecutionStatusFailed, nil, &errMsg)
			}
		}
	}()

	return execution, nil
}

// GetExecution retrieves an execution by ID.
func (s *Service) GetExecution(execID string) (*SceneExecution, error) {
	return s.execRepo.GetByID(execID)
}

// ListExecutions retrieves executions for a scene with pagination.
func (s *Service) ListExecutions(sceneID string, limit, offset int) ([]SceneExecution, int, error) {
	return s.execRepo.ListBySceneID(sceneID, limit, offset)
}

// StartScene starts playback on all scene members.
// Returns partial success results.
func (s *Service) StartScene(sceneID string) ([]DeviceResult, error) {
	scene, err := s.scenesRepo.GetByID(sceneID)
	if err != nil {
		return nil, err
	}
	if scene == nil {
		return nil, &SceneNotFoundError{SceneID: sceneID}
	}

	return s.executeOnMembers(scene, func(ip string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.SonosTimeoutMs)*time.Millisecond)
		defer cancel()
		return s.soapClient.Play(ctx, ip)
	})
}

// StopScene stops playback on all scene members.
// Returns partial success results.
func (s *Service) StopScene(sceneID string) ([]DeviceResult, error) {
	scene, err := s.scenesRepo.GetByID(sceneID)
	if err != nil {
		return nil, err
	}
	if scene == nil {
		return nil, &SceneNotFoundError{SceneID: sceneID}
	}

	return s.executeOnMembers(scene, func(ip string) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.SonosTimeoutMs)*time.Millisecond)
		defer cancel()
		return s.soapClient.Stop(ctx, ip)
	})
}

// executeOnMembers runs a function on all scene members in parallel.
func (s *Service) executeOnMembers(scene *Scene, fn func(ip string) error) ([]DeviceResult, error) {
	results := make([]DeviceResult, len(scene.Members))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, member := range scene.Members {
		wg.Add(1)
		go func(idx int, m SceneMember) {
			defer wg.Done()

			result := DeviceResult{DeviceID: m.DeviceID}

			ip, err := s.deviceService.ResolveDeviceIP(m.DeviceID)
			if err != nil || ip == "" {
				result.Success = false
				result.Error = "failed to resolve device IP"
				if err != nil {
					result.Error = err.Error()
				}
			} else {
				if err := fn(ip); err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					result.Success = true
				}
			}

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, member)
	}

	wg.Wait()
	return results, nil
}

// IsLocked checks if a coordinator is currently locked.
func (s *Service) IsLocked(deviceID string) bool {
	return s.lock.IsLocked(deviceID)
}

// SceneNotFoundError is returned when a scene is not found.
type SceneNotFoundError struct {
	SceneID string
}

func (e *SceneNotFoundError) Error() string {
	return "scene not found: " + e.SceneID
}

// SceneInUseError is returned when trying to delete a scene that is referenced by routines.
type SceneInUseError struct {
	SceneID      string
	RoutineCount int
}

func (e *SceneInUseError) Error() string {
	return "scene is referenced by routines and cannot be deleted"
}
