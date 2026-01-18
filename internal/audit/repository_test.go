package audit

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
)

func setupTestDB(t *testing.T) *Repository {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return NewRepository(dbPair)
}

func TestRepository_InsertEvent(t *testing.T) {
	repo := setupTestDB(t)

	requestID := "req-123"
	routineID := "routine-456"
	input := WriteEventInput{
		Type:      string(EventRoutineCreated),
		RequestID: &requestID,
		RoutineID: &routineID,
		Message:   "Routine execution started",
		Payload: map[string]any{
			"scene_id": "scene-789",
		},
	}

	event, err := repo.InsertEvent(input)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotEmpty(t, event.EventID)
	require.Equal(t, string(EventRoutineCreated), event.Type)
	require.Equal(t, EventLevelInfo, event.Level) // default level
	require.NotNil(t, event.RequestID)
	require.Equal(t, "req-123", *event.RequestID)
	require.NotNil(t, event.RoutineID)
	require.Equal(t, "routine-456", *event.RoutineID)
	require.Nil(t, event.JobID)
	require.Nil(t, event.SceneExecutionID)
	require.Nil(t, event.DeviceID)
	require.Equal(t, "Routine execution started", event.Message)
	require.Equal(t, "scene-789", event.Payload["scene_id"])
	require.False(t, event.Timestamp.IsZero())
}

func TestRepository_InsertEvent_WithLevel(t *testing.T) {
	repo := setupTestDB(t)

	level := EventLevelError
	input := WriteEventInput{
		Type:    string(EventDeviceOffline),
		Level:   &level,
		Message: "Device failed to respond",
	}

	event, err := repo.InsertEvent(input)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, EventLevelError, event.Level)
}

func TestRepository_InsertEvent_NilPayload(t *testing.T) {
	repo := setupTestDB(t)

	input := WriteEventInput{
		Type:    string(EventSystemStartup),
		Message: "No payload",
	}

	event, err := repo.InsertEvent(input)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotNil(t, event.Payload)
	require.Empty(t, event.Payload)
}

func TestRepository_GetEvent(t *testing.T) {
	repo := setupTestDB(t)

	input := WriteEventInput{
		Type:    string(EventSystemStartup),
		Message: "Test message",
	}

	created, err := repo.InsertEvent(input)
	require.NoError(t, err)

	fetched, err := repo.GetEvent(created.EventID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, created.EventID, fetched.EventID)
	require.Equal(t, string(EventSystemStartup), fetched.Type)
	require.Equal(t, "Test message", fetched.Message)
}

func TestRepository_GetEvent_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	event, err := repo.GetEvent("nonexistent-id")
	require.NoError(t, err)
	require.Nil(t, event)
}

func TestRepository_QueryEvents_NoFilters(t *testing.T) {
	repo := setupTestDB(t)

	// Create multiple events
	for i := 0; i < 5; i++ {
		_, err := repo.InsertEvent(WriteEventInput{
			Type:    string(EventSystemStartup),
			Message: "Event message",
		})
		require.NoError(t, err)
	}

	events, total, err := repo.QueryEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Len(t, events, 5)
	require.Equal(t, 5, total)
}

func TestRepository_QueryEvents_WithTypeFilter(t *testing.T) {
	repo := setupTestDB(t)

	_, err := repo.InsertEvent(WriteEventInput{Type: string(EventRoutineCreated), Message: "A1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventRoutineCreated), Message: "A2"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventRoutineDeleted), Message: "B1"})
	require.NoError(t, err)

	typeFilter := string(EventRoutineCreated)
	events, total, err := repo.QueryEvents(EventQueryFilters{Type: &typeFilter})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, 2, total)
	for _, e := range events {
		require.Equal(t, string(EventRoutineCreated), e.Type)
	}
}

func TestRepository_QueryEvents_WithLevelFilter(t *testing.T) {
	repo := setupTestDB(t)

	infoLevel := EventLevelInfo
	errorLevel := EventLevelError

	_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Level: &infoLevel, Message: "Info 1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventSystemError), Level: &errorLevel, Message: "Error 1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventSystemError), Level: &errorLevel, Message: "Error 2"})
	require.NoError(t, err)

	events, total, err := repo.QueryEvents(EventQueryFilters{Level: &errorLevel})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, 2, total)
	for _, e := range events {
		require.Equal(t, EventLevelError, e.Level)
	}
}

func TestRepository_QueryEvents_WithCorrelationFilters(t *testing.T) {
	repo := setupTestDB(t)

	routineID := "routine-123"
	jobID := "job-456"
	otherRoutine := "routine-999"

	_, err := repo.InsertEvent(WriteEventInput{Type: string(EventJobStarted), RoutineID: &routineID, JobID: &jobID, Message: "M1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventJobStarted), RoutineID: &routineID, Message: "M2"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventJobStarted), RoutineID: &otherRoutine, Message: "M3"})
	require.NoError(t, err)

	// Filter by routine_id
	events, total, err := repo.QueryEvents(EventQueryFilters{RoutineID: &routineID})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, 2, total)

	// Filter by routine_id and job_id
	events, total, err = repo.QueryEvents(EventQueryFilters{RoutineID: &routineID, JobID: &jobID})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 1, total)
}

func TestRepository_QueryEvents_WithDateFilters(t *testing.T) {
	repo := setupTestDB(t)

	// Insert events
	_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M2"})
	require.NoError(t, err)

	// Use a date range that includes now
	startDate := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	endDate := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	events, total, err := repo.QueryEvents(EventQueryFilters{StartDate: &startDate, EndDate: &endDate})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, 2, total)

	// Use a date range in the past
	oldStartDate := "2020-01-01T00:00:00Z"
	oldEndDate := "2020-01-02T00:00:00Z"
	events, total, err = repo.QueryEvents(EventQueryFilters{StartDate: &oldStartDate, EndDate: &oldEndDate})
	require.NoError(t, err)
	require.Len(t, events, 0)
	require.Equal(t, 0, total)
}

func TestRepository_QueryEvents_WithPagination(t *testing.T) {
	repo := setupTestDB(t)

	for i := 0; i < 10; i++ {
		_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M"})
		require.NoError(t, err)
	}

	// First page
	events, total, err := repo.QueryEvents(EventQueryFilters{Limit: 3, Offset: 0})
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, 10, total)

	// Second page
	events, total, err = repo.QueryEvents(EventQueryFilters{Limit: 3, Offset: 3})
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, 10, total)

	// Last partial page
	events, total, err = repo.QueryEvents(EventQueryFilters{Limit: 3, Offset: 9})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 10, total)
}

func TestRepository_QueryEvents_OrderedByTimestampDesc(t *testing.T) {
	repo := setupTestDB(t)

	// Insert events with delays > 1 second to ensure different timestamps
	// (RFC3339 has second precision)
	_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "First"})
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "Second"})
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)
	_, err = repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "Third"})
	require.NoError(t, err)

	events, _, err := repo.QueryEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Newest first
	require.Equal(t, "Third", events[0].Message)
	require.Equal(t, "Second", events[1].Message)
	require.Equal(t, "First", events[2].Message)
}

func TestRepository_CountEvents_NoFilters(t *testing.T) {
	repo := setupTestDB(t)

	for i := 0; i < 7; i++ {
		_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M"})
		require.NoError(t, err)
	}

	count, err := repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 7, count)
}

func TestRepository_CountEvents_WithFilters(t *testing.T) {
	repo := setupTestDB(t)

	typeA := string(EventRoutineCreated)
	typeB := string(EventRoutineDeleted)
	errorLevel := EventLevelError

	_, err := repo.InsertEvent(WriteEventInput{Type: typeA, Message: "M1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: typeA, Level: &errorLevel, Message: "M2"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: typeB, Message: "M3"})
	require.NoError(t, err)

	// Count typeA only
	count, err := repo.CountEvents(EventQueryFilters{Type: &typeA})
	require.NoError(t, err)
	require.Equal(t, 2, count)

	// Count typeA with ERROR level
	count, err = repo.CountEvents(EventQueryFilters{Type: &typeA, Level: &errorLevel})
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestRepository_PruneOldEvents(t *testing.T) {
	repo := setupTestDB(t)

	// Insert some events
	for i := 0; i < 5; i++ {
		_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M"})
		require.NoError(t, err)
	}

	// Verify events exist
	count, err := repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 5, count)

	// Wait a bit so events are in the past relative to cutoff
	time.Sleep(100 * time.Millisecond)

	// Prune with -1 retention days means cutoff is in the future (now + 1 day)
	// This should delete all events
	deleted, err := repo.PruneOldEvents(-1)
	require.NoError(t, err)
	require.Equal(t, int64(5), deleted)

	// Verify all deleted
	count, err = repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestRepository_PruneOldEvents_RetentionDays(t *testing.T) {
	repo := setupTestDB(t)

	// Insert events
	for i := 0; i < 3; i++ {
		_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M"})
		require.NoError(t, err)
	}

	// Prune with 30 days retention (should delete nothing since events are new)
	deleted, err := repo.PruneOldEvents(30)
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)

	// Verify nothing deleted
	count, err := repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 3, count)
}

func TestRepository_Prune(t *testing.T) {
	repo := setupTestDB(t)

	// Insert some events
	for i := 0; i < 5; i++ {
		_, err := repo.InsertEvent(WriteEventInput{Type: string(EventSystemStartup), Message: "M"})
		require.NoError(t, err)
	}

	// Verify events exist
	count, err := repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 5, count)

	// Prune with future cutoff (should delete all events)
	cutoff := time.Now().UTC().Add(1 * time.Hour)
	deleted, err := repo.Prune(cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(5), deleted)

	// Verify all deleted
	count, err = repo.CountEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestRepository_InsertEvent_AllCorrelationFields(t *testing.T) {
	repo := setupTestDB(t)

	requestID := "req-123"
	routineID := "routine-456"
	jobID := "job-789"
	sceneExecID := "scene-exec-abc"
	deviceID := "device-xyz"

	input := WriteEventInput{
		Type:             string(EventJobCompleted),
		RequestID:        &requestID,
		RoutineID:        &routineID,
		JobID:            &jobID,
		SceneExecutionID: &sceneExecID,
		DeviceID:         &deviceID,
		Message:          "All fields populated",
	}

	event, err := repo.InsertEvent(input)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotNil(t, event.RequestID)
	require.Equal(t, "req-123", *event.RequestID)
	require.NotNil(t, event.RoutineID)
	require.Equal(t, "routine-456", *event.RoutineID)
	require.NotNil(t, event.JobID)
	require.Equal(t, "job-789", *event.JobID)
	require.NotNil(t, event.SceneExecutionID)
	require.Equal(t, "scene-exec-abc", *event.SceneExecutionID)
	require.NotNil(t, event.DeviceID)
	require.Equal(t, "device-xyz", *event.DeviceID)
}

func TestRepository_QueryEvents_EmptyResult(t *testing.T) {
	repo := setupTestDB(t)

	events, total, err := repo.QueryEvents(EventQueryFilters{})
	require.NoError(t, err)
	require.NotNil(t, events)
	require.Len(t, events, 0)
	require.Equal(t, 0, total)
}

func TestRepository_QueryEvents_MultipleFilters(t *testing.T) {
	repo := setupTestDB(t)

	routineID := "routine-123"
	deviceID := "device-456"
	otherDevice := "device-789"
	errorLevel := EventLevelError
	infoLevel := EventLevelInfo
	eventType := string(EventJobFailed)

	// Create events with different combinations
	_, err := repo.InsertEvent(WriteEventInput{Type: eventType, RoutineID: &routineID, DeviceID: &deviceID, Level: &errorLevel, Message: "M1"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: eventType, RoutineID: &routineID, DeviceID: &deviceID, Level: &infoLevel, Message: "M2"})
	require.NoError(t, err)
	_, err = repo.InsertEvent(WriteEventInput{Type: eventType, RoutineID: &routineID, DeviceID: &otherDevice, Level: &errorLevel, Message: "M3"})
	require.NoError(t, err)

	// Filter by multiple criteria
	events, total, err := repo.QueryEvents(EventQueryFilters{
		Type:      &eventType,
		RoutineID: &routineID,
		DeviceID:  &deviceID,
		Level:     &errorLevel,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, 1, total)
	require.Equal(t, "M1", events[0].Message)
}

func TestRepository_WriteEvent_Alias(t *testing.T) {
	repo := setupTestDB(t)

	input := WriteEventInput{
		Type:    string(EventSystemStartup),
		Message: "Test WriteEvent alias",
	}

	event, err := repo.WriteEvent(input)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.Equal(t, string(EventSystemStartup), event.Type)
}

func TestRepository_GetByID_Alias(t *testing.T) {
	repo := setupTestDB(t)

	input := WriteEventInput{
		Type:    string(EventSystemStartup),
		Message: "Test GetByID alias",
	}

	created, err := repo.InsertEvent(input)
	require.NoError(t, err)

	fetched, err := repo.GetByID(created.EventID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, created.EventID, fetched.EventID)
}
