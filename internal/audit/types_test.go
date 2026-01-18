package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventTypeConstants(t *testing.T) {
	// Verify all event type constants have expected values
	require.Equal(t, EventType("ROUTINE_CREATED"), EventRoutineCreated)
	require.Equal(t, EventType("ROUTINE_UPDATED"), EventRoutineUpdated)
	require.Equal(t, EventType("ROUTINE_DELETED"), EventRoutineDeleted)
	require.Equal(t, EventType("JOB_SCHEDULED"), EventJobScheduled)
	require.Equal(t, EventType("JOB_STARTED"), EventJobStarted)
	require.Equal(t, EventType("JOB_COMPLETED"), EventJobCompleted)
	require.Equal(t, EventType("JOB_FAILED"), EventJobFailed)
	require.Equal(t, EventType("JOB_SKIPPED"), EventJobSkipped)
	require.Equal(t, EventType("SCENE_CREATED"), EventSceneCreated)
	require.Equal(t, EventType("SCENE_UPDATED"), EventSceneUpdated)
	require.Equal(t, EventType("SCENE_EXECUTION_STARTED"), EventSceneExecutionStarted)
	require.Equal(t, EventType("SCENE_EXECUTION_STEP"), EventSceneExecutionStep)
	require.Equal(t, EventType("SCENE_EXECUTION_COMPLETED"), EventSceneExecutionCompleted)
	require.Equal(t, EventType("SCENE_EXECUTION_FAILED"), EventSceneExecutionFailed)
	require.Equal(t, EventType("DEVICE_DISCOVERED"), EventDeviceDiscovered)
	require.Equal(t, EventType("DEVICE_OFFLINE"), EventDeviceOffline)
	require.Equal(t, EventType("PLAYBACK_VERIFIED"), EventPlaybackVerified)
	require.Equal(t, EventType("PLAYBACK_FAILED"), EventPlaybackFailed)
	require.Equal(t, EventType("SYSTEM_STARTUP"), EventSystemStartup)
	require.Equal(t, EventType("SYSTEM_ERROR"), EventSystemError)
}

func TestEventLevelConstants(t *testing.T) {
	require.Equal(t, EventLevel("DEBUG"), EventLevelDebug)
	require.Equal(t, EventLevel("INFO"), EventLevelInfo)
	require.Equal(t, EventLevel("WARN"), EventLevelWarn)
	require.Equal(t, EventLevel("ERROR"), EventLevelError)
}

func TestEventLevelAliases(t *testing.T) {
	// Test that the short-form aliases work correctly
	require.Equal(t, EventLevelDebug, LevelDebug)
	require.Equal(t, EventLevelInfo, LevelInfo)
	require.Equal(t, EventLevelWarn, LevelWarn)
	require.Equal(t, EventLevelError, LevelError)
}

func TestEventCorrelationJSON(t *testing.T) {
	requestID := "req-123"
	routineID := "routine-456"
	jobID := "job-789"
	sceneExecID := "exec-012"
	deviceID := "device-345"

	correlation := EventCorrelation{
		RequestID:        &requestID,
		RoutineID:        &routineID,
		JobID:            &jobID,
		SceneExecutionID: &sceneExecID,
		DeviceID:         &deviceID,
	}

	data, err := json.Marshal(correlation)
	require.NoError(t, err)

	var decoded EventCorrelation
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.RequestID)
	require.Equal(t, "req-123", *decoded.RequestID)
	require.NotNil(t, decoded.RoutineID)
	require.Equal(t, "routine-456", *decoded.RoutineID)
	require.NotNil(t, decoded.JobID)
	require.Equal(t, "job-789", *decoded.JobID)
	require.NotNil(t, decoded.SceneExecutionID)
	require.Equal(t, "exec-012", *decoded.SceneExecutionID)
	require.NotNil(t, decoded.DeviceID)
	require.Equal(t, "device-345", *decoded.DeviceID)
}

func TestEventCorrelationJSONOmitsEmpty(t *testing.T) {
	correlation := EventCorrelation{}

	data, err := json.Marshal(correlation)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasRequestID := m["request_id"]
	require.False(t, hasRequestID)
	_, hasRoutineID := m["routine_id"]
	require.False(t, hasRoutineID)
	_, hasJobID := m["job_id"]
	require.False(t, hasJobID)
	_, hasSceneExecID := m["scene_execution_id"]
	require.False(t, hasSceneExecID)
	_, hasDeviceID := m["device_id"]
	require.False(t, hasDeviceID)
}

func TestEventCorrelationPartialJSON(t *testing.T) {
	jobID := "job-123"

	correlation := EventCorrelation{
		JobID: &jobID,
	}

	data, err := json.Marshal(correlation)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "job-123", m["job_id"])
	_, hasRequestID := m["request_id"]
	require.False(t, hasRequestID)
	_, hasRoutineID := m["routine_id"]
	require.False(t, hasRoutineID)
}

func TestAuditEventJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	jobID := "job-123"
	routineID := "routine-456"

	event := AuditEvent{
		EventID:   "event-789",
		Timestamp: now,
		Type:      string(EventJobCompleted),
		Level:     EventLevelInfo,
		JobID:     &jobID,
		RoutineID: &routineID,
		Message:   "Job completed successfully",
		Payload: map[string]any{
			"duration_ms": 1500,
			"scene_id":    "scene-012",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded AuditEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "event-789", decoded.EventID)
	require.Equal(t, now, decoded.Timestamp)
	require.Equal(t, string(EventJobCompleted), decoded.Type)
	require.Equal(t, EventLevelInfo, decoded.Level)
	require.NotNil(t, decoded.JobID)
	require.Equal(t, "job-123", *decoded.JobID)
	require.NotNil(t, decoded.RoutineID)
	require.Equal(t, "routine-456", *decoded.RoutineID)
	require.Equal(t, "Job completed successfully", decoded.Message)
	require.NotNil(t, decoded.Payload)
	require.Equal(t, float64(1500), decoded.Payload["duration_ms"])
	require.Equal(t, "scene-012", decoded.Payload["scene_id"])
}

func TestAuditEventJSONWithEmptyPayload(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	event := AuditEvent{
		EventID:   "event-123",
		Timestamp: now,
		Type:      string(EventSystemStartup),
		Level:     EventLevelInfo,
		Message:   "System started",
		Payload:   nil,
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded AuditEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "event-123", decoded.EventID)
	require.Equal(t, string(EventSystemStartup), decoded.Type)
	require.Equal(t, EventLevelInfo, decoded.Level)
	require.Equal(t, "System started", decoded.Message)
	require.Nil(t, decoded.Payload)
}

func TestAuditEventJSONErrorLevel(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	deviceID := "device-123"

	event := AuditEvent{
		EventID:   "event-456",
		Timestamp: now,
		Type:      string(EventJobFailed),
		Level:     EventLevelError,
		DeviceID:  &deviceID,
		Message:   "Failed to execute job",
		Payload: map[string]any{
			"error": "connection timeout",
		},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded AuditEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, string(EventJobFailed), decoded.Type)
	require.Equal(t, EventLevelError, decoded.Level)
	require.NotNil(t, decoded.DeviceID)
	require.Equal(t, "device-123", *decoded.DeviceID)
	require.Equal(t, "connection timeout", decoded.Payload["error"])
}

func TestAuditEventJSONWarnLevel(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	event := AuditEvent{
		EventID:   "event-789",
		Timestamp: now,
		Type:      string(EventJobSkipped),
		Level:     EventLevelWarn,
		Message:   "Job skipped due to conflicting execution",
		Payload:   map[string]any{},
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded AuditEvent
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, string(EventJobSkipped), decoded.Type)
	require.Equal(t, EventLevelWarn, decoded.Level)
}

func TestAuditEventUnmarshalFromRawJSON(t *testing.T) {
	rawJSON := `{
		"event_id": "evt-001",
		"timestamp": "2024-01-15T10:30:00Z",
		"type": "SCENE_EXECUTION_COMPLETED",
		"level": "INFO",
		"scene_execution_id": "exec-123",
		"device_id": "dev-456",
		"message": "Scene execution completed",
		"payload": {
			"duration_ms": 2500,
			"steps_completed": 5
		}
	}`

	var event AuditEvent
	err := json.Unmarshal([]byte(rawJSON), &event)
	require.NoError(t, err)

	require.Equal(t, "evt-001", event.EventID)
	require.Equal(t, string(EventSceneExecutionCompleted), event.Type)
	require.Equal(t, EventLevelInfo, event.Level)
	require.NotNil(t, event.SceneExecutionID)
	require.Equal(t, "exec-123", *event.SceneExecutionID)
	require.NotNil(t, event.DeviceID)
	require.Equal(t, "dev-456", *event.DeviceID)
	require.Equal(t, "Scene execution completed", event.Message)
	require.Equal(t, float64(2500), event.Payload["duration_ms"])
	require.Equal(t, float64(5), event.Payload["steps_completed"])
}

func TestWriteEventInputDefaults(t *testing.T) {
	input := WriteEventInput{
		Type:      string(EventRoutineCreated),
		Level:    nil, // Should default to INFO when processed
		Message:  "Routine created",
		RoutineID: ptrString("routine-123"),
		Payload: map[string]any{
			"name": "Morning Routine",
		},
	}

	require.Equal(t, string(EventRoutineCreated), input.Type)
	require.Nil(t, input.Level)
	require.Equal(t, "Routine created", input.Message)
	require.NotNil(t, input.RoutineID)
	require.Equal(t, "routine-123", *input.RoutineID)
	require.Equal(t, "Morning Routine", input.Payload["name"])
}

func TestWriteEventInputWithLevel(t *testing.T) {
	level := EventLevelError
	input := WriteEventInput{
		Type:    string(EventSystemError),
		Level:   &level,
		Message: "Critical system error",
		Payload: map[string]any{
			"error_code": "ERR_001",
		},
	}

	require.Equal(t, string(EventSystemError), input.Type)
	require.NotNil(t, input.Level)
	require.Equal(t, EventLevelError, *input.Level)
}

func TestEventQueryFilters(t *testing.T) {
	startDate := "2024-01-14T10:30:00Z"
	endDate := "2024-01-15T10:30:00Z"
	eventType := string(EventJobCompleted)
	level := EventLevelInfo
	jobID := "job-123"
	routineID := "routine-456"
	sceneExecID := "exec-789"
	deviceID := "device-012"

	filters := EventQueryFilters{
		StartDate:        &startDate,
		EndDate:          &endDate,
		Type:             &eventType,
		Level:            &level,
		JobID:            &jobID,
		RoutineID:        &routineID,
		SceneExecutionID: &sceneExecID,
		DeviceID:         &deviceID,
		Limit:            100,
		Offset:           50,
	}

	require.NotNil(t, filters.StartDate)
	require.NotNil(t, filters.EndDate)
	require.NotNil(t, filters.Type)
	require.Equal(t, string(EventJobCompleted), *filters.Type)
	require.NotNil(t, filters.Level)
	require.Equal(t, EventLevelInfo, *filters.Level)
	require.NotNil(t, filters.JobID)
	require.Equal(t, "job-123", *filters.JobID)
	require.NotNil(t, filters.RoutineID)
	require.Equal(t, "routine-456", *filters.RoutineID)
	require.NotNil(t, filters.SceneExecutionID)
	require.Equal(t, "exec-789", *filters.SceneExecutionID)
	require.NotNil(t, filters.DeviceID)
	require.Equal(t, "device-012", *filters.DeviceID)
	require.Equal(t, 100, filters.Limit)
	require.Equal(t, 50, filters.Offset)
}

func TestEventQueryFiltersEmpty(t *testing.T) {
	filters := EventQueryFilters{
		Limit:  50,
		Offset: 0,
	}

	require.Nil(t, filters.StartDate)
	require.Nil(t, filters.EndDate)
	require.Nil(t, filters.Type)
	require.Nil(t, filters.Level)
	require.Nil(t, filters.JobID)
	require.Nil(t, filters.RoutineID)
	require.Nil(t, filters.SceneExecutionID)
	require.Nil(t, filters.DeviceID)
	require.Equal(t, 50, filters.Limit)
	require.Equal(t, 0, filters.Offset)
}

func TestEventTypeStringConversion(t *testing.T) {
	// Test that EventType can be converted to string and back
	eventType := EventJobCompleted
	str := string(eventType)
	require.Equal(t, "JOB_COMPLETED", str)

	// Test that a string can be converted to EventType
	fromStr := EventType(str)
	require.Equal(t, EventJobCompleted, fromStr)
}

func TestEventCorrelationToAuditEventFields(t *testing.T) {
	// Test creating an AuditEvent from EventCorrelation fields
	correlation := EventCorrelation{
		RequestID:        ptrString("req-123"),
		RoutineID:        ptrString("routine-456"),
		JobID:            ptrString("job-789"),
		SceneExecutionID: ptrString("exec-012"),
		DeviceID:         ptrString("device-345"),
	}

	event := AuditEvent{
		EventID:          "event-001",
		Type:             string(EventJobStarted),
		Level:            EventLevelInfo,
		RequestID:        correlation.RequestID,
		RoutineID:        correlation.RoutineID,
		JobID:            correlation.JobID,
		SceneExecutionID: correlation.SceneExecutionID,
		DeviceID:         correlation.DeviceID,
		Message:          "Job started",
		Payload:          map[string]any{},
	}

	require.Equal(t, "req-123", *event.RequestID)
	require.Equal(t, "routine-456", *event.RoutineID)
	require.Equal(t, "job-789", *event.JobID)
	require.Equal(t, "exec-012", *event.SceneExecutionID)
	require.Equal(t, "device-345", *event.DeviceID)
}

// ptrString is a helper function to create a pointer to a string
func ptrString(s string) *string {
	return &s
}
