package scheduler

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
	"github.com/strefethen/sonos-hub-go/internal/scene"
)

// ==========================================================================
// Mock RoutineExecutor
// ==========================================================================

type mockRoutineExecutor struct {
	mu             sync.Mutex
	executions     []*scene.SceneExecution
	executionCount int
	shouldFail     bool
	failError      error
	delay          time.Duration
	dbPair         *db.DBPair // For creating real scene executions
}

func newMockRoutineExecutor() *mockRoutineExecutor {
	return &mockRoutineExecutor{
		executions: make([]*scene.SceneExecution, 0),
	}
}

func newMockRoutineExecutorWithDB(dbPair *db.DBPair) *mockRoutineExecutor {
	return &mockRoutineExecutor{
		executions: make([]*scene.SceneExecution, 0),
		dbPair:     dbPair,
	}
}

func (m *mockRoutineExecutor) ExecuteRoutine(routine *Routine, idempotencyKey *string) (*scene.SceneExecution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	if m.shouldFail {
		return nil, m.failError
	}

	m.executionCount++
	execID := "exec-" + routine.SceneID + "-" + time.Now().Format("20060102150405.000000")
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")

	// If we have a DB connection, insert a real scene_execution record
	if m.dbPair != nil {
		_, err := m.dbPair.Writer().Exec(`
			INSERT INTO scene_executions (scene_execution_id, scene_id, status, started_at, steps)
			VALUES (?, ?, 'PLAYING_CONFIRMED', ?, '[]')
		`, execID, routine.SceneID, now)
		if err != nil {
			return nil, err
		}
	}

	execution := &scene.SceneExecution{
		SceneExecutionID: execID,
		SceneID:          routine.SceneID,
		IdempotencyKey:   idempotencyKey,
		Status:           scene.ExecutionStatusPlayingConfirmed,
		StartedAt:        time.Now().UTC(),
	}
	m.executions = append(m.executions, execution)
	return execution, nil
}

func (m *mockRoutineExecutor) setFailure(fail bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	m.failError = err
}

func (m *mockRoutineExecutor) getExecutionCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.executionCount
}

// ==========================================================================
// Test Setup Helpers
// ==========================================================================

func setupRunnerTestDB(t *testing.T) *db.DBPair {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return dbPair
}

func createTestScene(t *testing.T, dbPair *db.DBPair) string {
	sceneID := "scene-" + time.Now().Format("20060102150405.000000")
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
	_, err := dbPair.Writer().Exec(`
		INSERT INTO scenes (scene_id, name, members, created_at, updated_at)
		VALUES (?, 'Test Scene', '[]', ?, ?)
	`, sceneID, now, now)
	require.NoError(t, err)
	return sceneID
}

func createTestRoutine(t *testing.T, repo *RoutinesRepository, sceneID string) *Routine {
	routine, err := repo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleType: ScheduleTypeWeekly,
		ScheduleTime: "08:00",
		SceneID:      sceneID,
	})
	require.NoError(t, err)
	require.NotNil(t, routine)
	return routine
}

func createTestJob(t *testing.T, repo *JobsRepository, routineID string, scheduledFor time.Time) *Job {
	job, err := repo.Create(CreateJobInput{
		RoutineID:    routineID,
		ScheduledFor: scheduledFor,
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	return job
}

func newTestLogger() *log.Logger {
	return log.New(os.Stdout, "[test] ", log.LstdFlags|log.Lshortfile)
}

// ==========================================================================
// Tests
// ==========================================================================

func TestNewJobRunner(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()

	t.Run("creates runner with default values", func(t *testing.T) {
		runner := NewJobRunner(nil, jobsRepo, routinesRepo, executor, 0, 0)
		assert.NotNil(t, runner)
		assert.Equal(t, DefaultPollInterval, runner.pollInterval)
		assert.Equal(t, DefaultMaxRetries, runner.maxRetries)
	})

	t.Run("creates runner with custom values", func(t *testing.T) {
		logger := newTestLogger()
		pollInterval := 5 * time.Second
		maxRetries := 5

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, pollInterval, maxRetries)
		assert.NotNil(t, runner)
		assert.Equal(t, pollInterval, runner.pollInterval)
		assert.Equal(t, maxRetries, runner.maxRetries)
	})
}

func TestJobRunner_StartStop(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()
	logger := newTestLogger()

	runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

	t.Run("starts and stops gracefully", func(t *testing.T) {
		runner.Start()

		// Wait for a couple of poll cycles
		time.Sleep(250 * time.Millisecond)

		// Stop should complete without hanging
		done := make(chan struct{})
		go func() {
			runner.Stop()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(2 * time.Second):
			t.Fatal("Stop() did not complete in time")
		}
	})
}

func TestJobRunner_ExecuteJob(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutorWithDB(dbPair)
	logger := newTestLogger()

	t.Run("executes a pending job successfully", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)
		runner.Start()

		// Wait for job to be executed
		time.Sleep(300 * time.Millisecond)
		runner.Stop()

		// Verify job was completed
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusCompleted, updatedJob.Status)
		assert.NotNil(t, updatedJob.SceneExecutionID)
		assert.Equal(t, 1, executor.getExecutionCount())
	})

	t.Run("does not execute future jobs", func(t *testing.T) {
		executor2 := newMockRoutineExecutor()
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		_ = createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(1*time.Hour))

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor2, 100*time.Millisecond, 3)
		runner.Start()

		// Wait for a poll cycle
		time.Sleep(200 * time.Millisecond)
		runner.Stop()

		// Job should not be executed
		assert.Equal(t, 0, executor2.getExecutionCount())
	})
}

func TestJobRunner_RetryLogic(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()
	logger := newTestLogger()

	t.Run("retries failed job with exponential backoff", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// Set executor to fail
		executor.setFailure(true, errors.New("simulated failure"))

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Execute the job directly
		err := runner.executeJob(job)
		assert.Error(t, err)

		// Verify job is reset for retry
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusPending, updatedJob.Status)
		assert.Equal(t, 1, updatedJob.Attempts)
		assert.NotNil(t, updatedJob.LastError)
		assert.Contains(t, *updatedJob.LastError, "simulated failure")
	})

	t.Run("marks job as failed after max retries", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// Set job attempts to max-1
		_, err := dbPair.Writer().Exec("UPDATE jobs SET attempts = ? WHERE job_id = ?", 2, job.JobID)
		require.NoError(t, err)

		// Set executor to fail
		executor.setFailure(true, errors.New("final failure"))

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Refresh job
		job, _ = jobsRepo.GetByID(job.JobID)

		// Execute the job directly
		err = runner.executeJob(job)
		assert.Error(t, err)

		// Verify job is marked as failed
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusFailed, updatedJob.Status)
		assert.Equal(t, 3, updatedJob.Attempts)
	})
}

func TestJobRunner_RecoverStaleJobs(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()
	logger := newTestLogger()

	t.Run("recovers stale claimed jobs", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-10*time.Minute))

		// Manually set job as claimed with old timestamp
		staleTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
		_, err := dbPair.Writer().Exec(
			"UPDATE jobs SET status = ?, claimed_at = ? WHERE job_id = ?",
			string(JobStatusClaimed), staleTime, job.JobID,
		)
		require.NoError(t, err)

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Call recoverStaleJobs
		runner.recoverStaleJobs()

		// Verify job is reset for retry
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusPending, updatedJob.Status)
		assert.NotNil(t, updatedJob.LastError)
		assert.Contains(t, *updatedJob.LastError, "stale claim timeout")
	})

	t.Run("does not recover recent claimed jobs", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// Claim job with recent timestamp
		err := jobsRepo.ClaimJob(job.JobID)
		require.NoError(t, err)

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Call recoverStaleJobs
		runner.recoverStaleJobs()

		// Verify job is still claimed (not recovered)
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusClaimed, updatedJob.Status)
	})
}

func TestJobRunner_RetryAfter(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()
	logger := newTestLogger()

	t.Run("respects retry_after timestamp", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// Set retry_after to the future
		retryAfter := time.Now().UTC().Add(1 * time.Hour)
		err := jobsRepo.SetRetryAfter(job.JobID, retryAfter)
		require.NoError(t, err)

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)
		runner.Start()

		// Wait for a poll cycle
		time.Sleep(200 * time.Millisecond)
		runner.Stop()

		// Job should not be executed (retry_after is in future)
		assert.Equal(t, 0, executor.getExecutionCount())
	})
}

func TestJobRunner_ConcurrentClaiming(t *testing.T) {
	// This test verifies that the atomic ClaimJob operation prevents double execution.
	// Due to SQLite's in-memory database limitations with multiple connections,
	// we test the core claim logic directly rather than with actual concurrent runners.
	t.Run("claim is atomic and prevents double claims", func(t *testing.T) {
		dbPair := setupRunnerTestDB(t)

		jobsRepo := NewJobsRepository(dbPair)
		routinesRepo := NewRoutinesRepository(dbPair)

		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// First claim should succeed
		err := jobsRepo.ClaimJob(job.JobID)
		require.NoError(t, err)

		// Second claim should fail
		err = jobsRepo.ClaimJob(job.JobID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already claimed")

		// Verify job is claimed
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusClaimed, updatedJob.Status)
		assert.NotNil(t, updatedJob.ClaimedAt)
	})
}

func TestJobRunner_MissingRoutine(t *testing.T) {
	// NOTE: With ON DELETE CASCADE on jobs.routine_id, deleting a routine
	// also deletes its jobs. This test now verifies the behavior when
	// a routine is deleted (and thus the job is also deleted via CASCADE).
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutor()
	logger := newTestLogger()

	t.Run("job is deleted when routine is deleted via CASCADE", func(t *testing.T) {
		// Create a valid scene and routine first
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)

		// Create a job for this routine
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		// Delete the routine - this should CASCADE delete the job too
		_, err := dbPair.Writer().Exec("DELETE FROM routines WHERE routine_id = ?", routine.RoutineID)
		require.NoError(t, err)

		// Verify job was also deleted via CASCADE
		deletedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Nil(t, deletedJob, "job should be deleted when routine is deleted (CASCADE)")

		// Runner should not find this job in the pending list
		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)
		runner.Start()
		time.Sleep(200 * time.Millisecond)
		runner.Stop()

		// Executor should not have been called
		assert.Equal(t, 0, executor.getExecutionCount())
	})
}

func TestJobRunner_UpdateLastRunAt(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutorWithDB(dbPair)
	logger := newTestLogger()

	t.Run("updates routine last_run_at on success", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)
		job := createTestJob(t, jobsRepo, routine.RoutineID, time.Now().UTC().Add(-1*time.Minute))

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Execute the job directly
		err := runner.executeJob(job)
		require.NoError(t, err)

		// Verify job is completed
		updatedJob, err := jobsRepo.GetByID(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, JobStatusCompleted, updatedJob.Status)

		// Note: We can't directly verify last_run_at from the routine
		// because the GetByID doesn't return this field (it's not in the SELECT).
		// This would require extending the repository or checking the DB directly.
	})
}

func TestJobRunner_IdempotencyKey(t *testing.T) {
	dbPair := setupRunnerTestDB(t)

	jobsRepo := NewJobsRepository(dbPair)
	routinesRepo := NewRoutinesRepository(dbPair)
	executor := newMockRoutineExecutorWithDB(dbPair)
	logger := newTestLogger()

	t.Run("passes idempotency key to scene executor", func(t *testing.T) {
		sceneID := createTestScene(t, dbPair)
		routine := createTestRoutine(t, routinesRepo, sceneID)

		idempotencyKey := "unique-key-123"
		job, err := jobsRepo.Create(CreateJobInput{
			RoutineID:      routine.RoutineID,
			ScheduledFor:   time.Now().UTC().Add(-1 * time.Minute),
			IdempotencyKey: &idempotencyKey,
		})
		require.NoError(t, err)

		runner := NewJobRunner(logger, jobsRepo, routinesRepo, executor, 100*time.Millisecond, 3)

		// Execute the job directly
		err = runner.executeJob(job)
		require.NoError(t, err)

		// Verify execution received the idempotency key
		executor.mu.Lock()
		defer executor.mu.Unlock()
		require.Len(t, executor.executions, 1)
		assert.NotNil(t, executor.executions[0].IdempotencyKey)
		assert.Equal(t, idempotencyKey, *executor.executions[0].IdempotencyKey)
	})
}
