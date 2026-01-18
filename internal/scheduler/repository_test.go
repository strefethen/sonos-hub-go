package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
	"github.com/strefethen/sonos-hub-go/internal/scene"
)

func setupTestDB(t *testing.T) (*RoutinesRepository, *JobsRepository, *HolidaysRepository, *scene.ScenesRepository) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return NewRoutinesRepository(dbPair),
		NewJobsRepository(dbPair),
		NewHolidaysRepository(dbPair),
		scene.NewScenesRepository(dbPair)
}

// ==========================================================================
// RoutinesRepository Tests
// ==========================================================================

func TestRoutinesRepository_Create(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	// Create a scene first (FK constraint)
	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	enabled := true
	weekdays := []int{1, 2, 3, 4, 5} // Monday-Friday
	input := CreateRoutineInput{
		Name:             "Morning Music",
		Enabled:          &enabled,
		Timezone:         "America/Los_Angeles",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: weekdays,
		ScheduleTime:     "07:00",
		HolidayBehavior:  HolidayBehaviorSkip,
		SceneID:          s.SceneID,
		MusicPolicyType:  MusicPolicyTypeRotation,
	}

	routine, err := routinesRepo.Create(input)
	require.NoError(t, err)
	require.NotNil(t, routine)
	require.NotEmpty(t, routine.RoutineID)
	require.Equal(t, "Morning Music", routine.Name)
	require.True(t, routine.Enabled)
	require.Equal(t, "America/Los_Angeles", routine.Timezone)
	require.Equal(t, ScheduleTypeWeekly, routine.ScheduleType)
	require.Equal(t, weekdays, routine.ScheduleWeekdays)
	require.Equal(t, "07:00", routine.ScheduleTime)
	require.Equal(t, HolidayBehaviorSkip, routine.HolidayBehavior)
	require.Equal(t, s.SceneID, routine.SceneID)
	require.Equal(t, MusicPolicyTypeRotation, routine.MusicPolicyType)
	require.False(t, routine.SkipNext)
}

func TestRoutinesRepository_CreateWithDefaults(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	input := CreateRoutineInput{
		Name:         "Basic Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	}

	routine, err := routinesRepo.Create(input)
	require.NoError(t, err)
	require.True(t, routine.Enabled) // Default
	require.Equal(t, ScheduleTypeWeekly, routine.ScheduleType)
	require.Equal(t, HolidayBehaviorSkip, routine.HolidayBehavior)
	require.Equal(t, MusicPolicyTypeFixed, routine.MusicPolicyType)
}

func TestRoutinesRepository_GetByID(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "09:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	fetched, err := routinesRepo.GetByID(routine.RoutineID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, routine.RoutineID, fetched.RoutineID)
	require.Equal(t, "Test Routine", fetched.Name)
}

func TestRoutinesRepository_GetByID_NotFound(t *testing.T) {
	routinesRepo, _, _, _ := setupTestDB(t)

	routine, err := routinesRepo.GetByID("nonexistent")
	require.NoError(t, err)
	require.Nil(t, routine)
}

func TestRoutinesRepository_List(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	// Create 5 routines
	for i := 0; i < 5; i++ {
		enabled := i%2 == 0 // Alternate enabled/disabled
		_, err := routinesRepo.Create(CreateRoutineInput{
			Name:         "Routine " + string(rune('A'+i)),
			Timezone:     "UTC",
			ScheduleTime: "08:00",
			SceneID:      s.SceneID,
			Enabled:      &enabled,
		})
		require.NoError(t, err)
	}

	// List all
	routines, total, err := routinesRepo.List(3, 0, false)
	require.NoError(t, err)
	require.Len(t, routines, 3)
	require.Equal(t, 5, total)

	// List with offset
	routines, total, err = routinesRepo.List(3, 3, false)
	require.NoError(t, err)
	require.Len(t, routines, 2)
	require.Equal(t, 5, total)

	// List enabled only
	routines, total, err = routinesRepo.List(10, 0, true)
	require.NoError(t, err)
	require.Len(t, routines, 3) // A, C, E are enabled (indices 0, 2, 4)
	require.Equal(t, 3, total)
}

func TestRoutinesRepository_Update(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Original Name",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	newName := "Updated Name"
	newTime := "10:00"
	newEnabled := false
	updated, err := routinesRepo.Update(routine.RoutineID, UpdateRoutineInput{
		Name:         &newName,
		ScheduleTime: &newTime,
		Enabled:      &newEnabled,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Updated Name", updated.Name)
	require.Equal(t, "10:00", updated.ScheduleTime)
	require.False(t, updated.Enabled)
	require.Equal(t, "UTC", updated.Timezone) // Preserved
}

func TestRoutinesRepository_Update_NotFound(t *testing.T) {
	routinesRepo, _, _, _ := setupTestDB(t)

	newName := "Updated"
	updated, err := routinesRepo.Update("nonexistent", UpdateRoutineInput{Name: &newName})
	require.NoError(t, err)
	require.Nil(t, updated)
}

func TestRoutinesRepository_Delete(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "To Delete",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	err = routinesRepo.Delete(routine.RoutineID)
	require.NoError(t, err)

	fetched, err := routinesRepo.GetByID(routine.RoutineID)
	require.NoError(t, err)
	require.Nil(t, fetched)
}

func TestRoutinesRepository_Delete_NotFound(t *testing.T) {
	routinesRepo, _, _, _ := setupTestDB(t)

	err := routinesRepo.Delete("nonexistent")
	require.Error(t, err)
}

func TestRoutinesRepository_GetDueRoutines(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	enabled := true
	disabled := false
	skipNext := true

	// Create enabled routine
	_, err = routinesRepo.Create(CreateRoutineInput{
		Name:         "Enabled Routine",
		Enabled:      &enabled,
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	// Create disabled routine
	_, err = routinesRepo.Create(CreateRoutineInput{
		Name:         "Disabled Routine",
		Enabled:      &disabled,
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	// Create routine with skip_next
	r3, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Skip Next Routine",
		Enabled:      &enabled,
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)
	_, err = routinesRepo.Update(r3.RoutineID, UpdateRoutineInput{SkipNext: &skipNext})
	require.NoError(t, err)

	// Get due routines
	now := time.Now().UTC()
	routines, err := routinesRepo.GetDueRoutines(now)
	require.NoError(t, err)
	require.Len(t, routines, 1)
	require.Equal(t, "Enabled Routine", routines[0].Name)
}

func TestRoutinesRepository_WithSpeakers(t *testing.T) {
	routinesRepo, _, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	vol := 50
	speakers := []Speaker{
		{DeviceID: "device-1", Volume: &vol},
		{DeviceID: "device-2"},
	}

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Routine with Speakers",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
		SpeakersJSON: speakers,
	})
	require.NoError(t, err)
	require.Len(t, routine.SpeakersJSON, 2)
	require.Equal(t, "device-1", routine.SpeakersJSON[0].DeviceID)
	require.NotNil(t, routine.SpeakersJSON[0].Volume)
	require.Equal(t, 50, *routine.SpeakersJSON[0].Volume)
	require.Nil(t, routine.SpeakersJSON[1].Volume)
}

// ==========================================================================
// JobsRepository Tests
// ==========================================================================

func TestJobsRepository_Create(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	scheduledFor := time.Now().Add(time.Hour).UTC()
	idemKey := "job-idem-123"
	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:      routine.RoutineID,
		ScheduledFor:   scheduledFor,
		IdempotencyKey: &idemKey,
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotEmpty(t, job.JobID)
	require.Equal(t, routine.RoutineID, job.RoutineID)
	require.Equal(t, JobStatusPending, job.Status)
	require.Equal(t, 0, job.Attempts)
	require.NotNil(t, job.IdempotencyKey)
	require.Equal(t, "job-idem-123", *job.IdempotencyKey)
}

func TestJobsRepository_GetByID(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, job.JobID, fetched.JobID)
}

func TestJobsRepository_GetByID_NotFound(t *testing.T) {
	_, jobsRepo, _, _ := setupTestDB(t)

	job, err := jobsRepo.GetByID("nonexistent")
	require.NoError(t, err)
	require.Nil(t, job)
}

func TestJobsRepository_ListByRoutineID(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	// Create 5 jobs
	for i := 0; i < 5; i++ {
		_, err := jobsRepo.Create(CreateJobInput{
			RoutineID:    routine.RoutineID,
			ScheduledFor: time.Now().Add(time.Duration(i) * time.Hour).UTC(),
		})
		require.NoError(t, err)
	}

	jobs, total, err := jobsRepo.ListByRoutineID(routine.RoutineID, 3, 0)
	require.NoError(t, err)
	require.Len(t, jobs, 3)
	require.Equal(t, 5, total)

	jobs, total, err = jobsRepo.ListByRoutineID(routine.RoutineID, 3, 3)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	require.Equal(t, 5, total)
}

func TestJobsRepository_GetPendingJobs(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	// Create jobs at different times
	_, err = jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(2 * time.Hour).UTC(),
	})
	require.NoError(t, err)

	_, err = jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(1 * time.Hour).UTC(),
	})
	require.NoError(t, err)

	jobs, err := jobsRepo.GetPendingJobs(10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	// Should be ordered by scheduled_for ASC
	require.True(t, jobs[0].ScheduledFor.Before(jobs[1].ScheduledFor))
}

func TestJobsRepository_ClaimJob(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusClaimed, fetched.Status)
	require.NotNil(t, fetched.ClaimedAt)
}

func TestJobsRepository_ClaimJob_AlreadyClaimed(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	// First claim succeeds
	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)

	// Second claim fails
	err = jobsRepo.ClaimJob(job.JobID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already claimed")
}

func TestJobsRepository_StartJob(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)

	err = jobsRepo.StartJob(job.JobID)
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusRunning, fetched.Status)
}

func TestJobsRepository_CompleteJob(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)
	err = jobsRepo.StartJob(job.JobID)
	require.NoError(t, err)

	err = jobsRepo.CompleteJob(job.JobID, "")
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusCompleted, fetched.Status)
	require.Nil(t, fetched.SceneExecutionID) // No scene execution ID when empty string passed
}

func TestJobsRepository_FailJob_WithRetry(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)
	err = jobsRepo.StartJob(job.JobID)
	require.NoError(t, err)

	err = jobsRepo.FailJob(job.JobID, "temporary error", true)
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusPending, fetched.Status) // Back to pending for retry
	require.Equal(t, 1, fetched.Attempts)
	require.NotNil(t, fetched.LastError)
	require.Equal(t, "temporary error", *fetched.LastError)
	require.Nil(t, fetched.ClaimedAt) // Cleared for retry
}

func TestJobsRepository_FailJob_NoRetry(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)
	err = jobsRepo.StartJob(job.JobID)
	require.NoError(t, err)

	err = jobsRepo.FailJob(job.JobID, "permanent error", false)
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusFailed, fetched.Status)
	require.Equal(t, 1, fetched.Attempts)
	require.NotNil(t, fetched.LastError)
	require.Equal(t, "permanent error", *fetched.LastError)
}

func TestJobsRepository_SkipJob(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.SkipJob(job.JobID, "holiday")
	require.NoError(t, err)

	fetched, err := jobsRepo.GetByID(job.JobID)
	require.NoError(t, err)
	require.Equal(t, JobStatusSkipped, fetched.Status)
	require.NotNil(t, fetched.LastError)
	require.Equal(t, "holiday", *fetched.LastError)
}

func TestJobsRepository_GetStaleClaimedJobs(t *testing.T) {
	routinesRepo, jobsRepo, _, scenesRepo := setupTestDB(t)

	s, err := scenesRepo.Create(scene.CreateSceneInput{
		Name:    "Test Scene",
		Members: []scene.SceneMember{},
	})
	require.NoError(t, err)

	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:         "Test Routine",
		Timezone:     "UTC",
		ScheduleTime: "08:00",
		SceneID:      s.SceneID,
	})
	require.NoError(t, err)

	job, err := jobsRepo.Create(CreateJobInput{
		RoutineID:    routine.RoutineID,
		ScheduledFor: time.Now().Add(time.Hour).UTC(),
	})
	require.NoError(t, err)

	err = jobsRepo.ClaimJob(job.JobID)
	require.NoError(t, err)

	// With a timeout that causes job to be stale - since RFC3339 has second precision,
	// we need to sleep for over a second to ensure the comparison works
	time.Sleep(1100 * time.Millisecond)
	staleJobs, err := jobsRepo.GetStaleClaimedJobs(500 * time.Millisecond)
	require.NoError(t, err)
	require.Len(t, staleJobs, 1)
	require.Equal(t, job.JobID, staleJobs[0].JobID)

	// With a long timeout, no stale jobs
	staleJobs, err = jobsRepo.GetStaleClaimedJobs(time.Hour)
	require.NoError(t, err)
	require.Len(t, staleJobs, 0)
}

// ==========================================================================
// HolidaysRepository Tests
// ==========================================================================

func TestHolidaysRepository_Create(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	date := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)
	holiday, err := holidaysRepo.Create(CreateHolidayInput{
		Date:     date,
		Name:     "Christmas",
		IsCustom: false,
	})
	require.NoError(t, err)
	require.NotNil(t, holiday)
	require.Equal(t, "Christmas", holiday.Name)
	require.False(t, holiday.IsCustom)
	require.Equal(t, "2025-12-25", holiday.Date)
}

func TestHolidaysRepository_GetByID(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := holidaysRepo.Create(CreateHolidayInput{
		Date:     date,
		Name:     "New Year",
		IsCustom: false,
	})
	require.NoError(t, err)

	holiday, err := holidaysRepo.GetByID("2025-01-01")
	require.NoError(t, err)
	require.NotNil(t, holiday)
	require.Equal(t, "New Year", holiday.Name)
}

func TestHolidaysRepository_GetByID_NotFound(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	holiday, err := holidaysRepo.GetByID("2099-01-01")
	require.NoError(t, err)
	require.Nil(t, holiday)
}

func TestHolidaysRepository_List(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	dates := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 7, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC),
	}
	names := []string{"New Year", "Independence Day", "Thanksgiving", "Christmas"}

	for i, date := range dates {
		_, err := holidaysRepo.Create(CreateHolidayInput{
			Date: date,
			Name: names[i],
		})
		require.NoError(t, err)
	}

	holidays, total, err := holidaysRepo.List(2, 0)
	require.NoError(t, err)
	require.Len(t, holidays, 2)
	require.Equal(t, 4, total)
	require.Equal(t, "New Year", holidays[0].Name) // Ordered by date

	holidays, total, err = holidaysRepo.List(2, 2)
	require.NoError(t, err)
	require.Len(t, holidays, 2)
	require.Equal(t, 4, total)
}

func TestHolidaysRepository_Delete(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := holidaysRepo.Create(CreateHolidayInput{
		Date: date,
		Name: "New Year",
	})
	require.NoError(t, err)

	err = holidaysRepo.Delete("2025-01-01")
	require.NoError(t, err)

	holiday, err := holidaysRepo.GetByID("2025-01-01")
	require.NoError(t, err)
	require.Nil(t, holiday)
}

func TestHolidaysRepository_Delete_NotFound(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	err := holidaysRepo.Delete("2099-01-01")
	require.Error(t, err)
}

func TestHolidaysRepository_IsHoliday(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	date := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)
	_, err := holidaysRepo.Create(CreateHolidayInput{
		Date: date,
		Name: "Christmas",
	})
	require.NoError(t, err)

	// Check Christmas
	isHoliday, holiday, err := holidaysRepo.IsHoliday(time.Date(2025, 12, 25, 15, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, isHoliday)
	require.NotNil(t, holiday)
	require.Equal(t, "Christmas", holiday.Name)

	// Check non-holiday
	isHoliday, holiday, err = holidaysRepo.IsHoliday(time.Date(2025, 12, 26, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.False(t, isHoliday)
	require.Nil(t, holiday)
}

func TestHolidaysRepository_GetHolidaysInRange(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	dates := []time.Time{
		time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 7, 4, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC),
	}
	names := []string{"New Year", "Independence Day", "Thanksgiving", "Christmas"}

	for i, date := range dates {
		_, err := holidaysRepo.Create(CreateHolidayInput{
			Date: date,
			Name: names[i],
		})
		require.NoError(t, err)
	}

	// Get holidays in Q4
	start := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	holidays, err := holidaysRepo.GetHolidaysInRange(start, end)
	require.NoError(t, err)
	require.Len(t, holidays, 2)
	require.Equal(t, "Thanksgiving", holidays[0].Name)
	require.Equal(t, "Christmas", holidays[1].Name)

	// Get all holidays in 2025
	start = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end = time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	holidays, err = holidaysRepo.GetHolidaysInRange(start, end)
	require.NoError(t, err)
	require.Len(t, holidays, 4)
}

func TestHolidaysRepository_CustomHoliday(t *testing.T) {
	_, _, holidaysRepo, _ := setupTestDB(t)

	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	holiday, err := holidaysRepo.Create(CreateHolidayInput{
		Date:     date,
		Name:     "Company Retreat",
		IsCustom: true,
	})
	require.NoError(t, err)
	require.True(t, holiday.IsCustom)

	fetched, err := holidaysRepo.GetByID("2025-06-15")
	require.NoError(t, err)
	require.True(t, fetched.IsCustom)
}
