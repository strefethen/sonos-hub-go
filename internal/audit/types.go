package audit

// EventType represents the type of audit event.
type EventType string

const (
	EventRoutineCreated          EventType = "ROUTINE_CREATED"
	EventRoutineUpdated          EventType = "ROUTINE_UPDATED"
	EventRoutineDeleted          EventType = "ROUTINE_DELETED"
	EventJobScheduled            EventType = "JOB_SCHEDULED"
	EventJobStarted              EventType = "JOB_STARTED"
	EventJobCompleted            EventType = "JOB_COMPLETED"
	EventJobFailed               EventType = "JOB_FAILED"
	EventJobSkipped              EventType = "JOB_SKIPPED"
	EventSceneCreated            EventType = "SCENE_CREATED"
	EventSceneUpdated            EventType = "SCENE_UPDATED"
	EventSceneExecutionStarted   EventType = "SCENE_EXECUTION_STARTED"
	EventSceneExecutionStep      EventType = "SCENE_EXECUTION_STEP"
	EventSceneExecutionCompleted EventType = "SCENE_EXECUTION_COMPLETED"
	EventSceneExecutionFailed    EventType = "SCENE_EXECUTION_FAILED"
	EventDeviceDiscovered        EventType = "DEVICE_DISCOVERED"
	EventDeviceOffline           EventType = "DEVICE_OFFLINE"
	EventPlaybackVerified        EventType = "PLAYBACK_VERIFIED"
	EventPlaybackFailed          EventType = "PLAYBACK_FAILED"
	EventSystemStartup           EventType = "SYSTEM_STARTUP"
	EventSystemError             EventType = "SYSTEM_ERROR"
)

// EventCorrelation contains IDs that link related events together.
type EventCorrelation struct {
	RequestID        *string `json:"request_id,omitempty"`
	RoutineID        *string `json:"routine_id,omitempty"`
	JobID            *string `json:"job_id,omitempty"`
	SceneExecutionID *string `json:"scene_execution_id,omitempty"`
	DeviceID         *string `json:"device_id,omitempty"`
}

// Alias constants to match new naming convention while preserving compatibility
// with existing code that uses EventLevel* prefix.
const (
	LevelDebug = EventLevelDebug
	LevelInfo  = EventLevelInfo
	LevelWarn  = EventLevelWarn
	LevelError = EventLevelError
)
