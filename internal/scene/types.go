package scene

import "time"

// CoordinatorPreference determines how the coordinator is selected.
type CoordinatorPreference string

const (
	CoordinatorPreferenceArcFirst CoordinatorPreference = "ARC_FIRST"
)

// FallbackPolicy determines what to do when the preferred coordinator is unavailable.
type FallbackPolicy string

const (
	FallbackPolicyPlaybaseIfArcTVActive FallbackPolicy = "PLAYBASE_IF_ARC_TV_ACTIVE"
)

// ExecutionStatus represents the state of a scene execution.
type ExecutionStatus string

const (
	ExecutionStatusStarting         ExecutionStatus = "STARTING"
	ExecutionStatusPlayingConfirmed ExecutionStatus = "PLAYING_CONFIRMED"
	ExecutionStatusFailed           ExecutionStatus = "FAILED"
	ExecutionStatusRolledBack       ExecutionStatus = "ROLLED_BACK"
)

// StepStatus represents the state of an execution step.
type StepStatus string

const (
	StepStatusPending   StepStatus = "pending"
	StepStatusRunning   StepStatus = "running"
	StepStatusCompleted StepStatus = "completed"
	StepStatusFailed    StepStatus = "failed"
	StepStatusSkipped   StepStatus = "skipped"
)

// IssueType represents the type of preflight issue detected.
type IssueType string

const (
	IssueTypeTVMode         IssueType = "TV_MODE"
	IssueTypeNotCoordinator IssueType = "NOT_COORDINATOR"
	IssueTypeTransitioning  IssueType = "TRANSITIONING"
	IssueTypeOffline        IssueType = "OFFLINE"
	IssueTypeNoPlayAction   IssueType = "NO_PLAY_ACTION"
)

// QueueMode determines how music content is queued.
type QueueMode string

const (
	QueueModeReplaceAndPlay QueueMode = "REPLACE_AND_PLAY"
	QueueModePlayNext       QueueMode = "PLAY_NEXT"
	QueueModeAddToEnd       QueueMode = "ADD_TO_END"
	QueueModeQueueOnly      QueueMode = "QUEUE_ONLY"
)

// GroupBehavior determines how grouping is handled.
type GroupBehavior string

const (
	GroupBehaviorAutoRedirect      GroupBehavior = "AUTO_REDIRECT"
	GroupBehaviorUngroupAndPlay    GroupBehavior = "UNGROUP_AND_PLAY"
	GroupBehaviorRequireCoordinator GroupBehavior = "REQUIRE_COORDINATOR"
)

// TVPolicy determines how TV mode is handled.
type TVPolicy string

const (
	TVPolicySkip       TVPolicy = "SKIP"
	TVPolicyUseFallback TVPolicy = "USE_FALLBACK"
	TVPolicyAlwaysPlay TVPolicy = "ALWAYS_PLAY"
)

// SceneMember represents a device that participates in a scene.
type SceneMember struct {
	DeviceID     string `json:"device_id"`
	RoomName     string `json:"room_name,omitempty"`
	TargetVolume *int   `json:"target_volume,omitempty"`
	Mute         *bool  `json:"mute,omitempty"`
}

// VolumeRamp defines volume ramping behavior.
type VolumeRamp struct {
	Enabled    bool   `json:"enabled"`
	DurationMs *int   `json:"duration_ms,omitempty"`
	Curve      string `json:"curve,omitempty"` // linear, ease-in, ease-out
}

// Teardown defines behavior after scene playback.
type Teardown struct {
	UngroupAfterMs       *int `json:"ungroup_after_ms,omitempty"`
	RestorePreviousState bool `json:"restore_previous_state,omitempty"`
}

// Scene represents a configured playback scene.
type Scene struct {
	SceneID               string        `json:"scene_id"`
	Name                  string        `json:"name"`
	Description           *string       `json:"description,omitempty"`
	CoordinatorPreference string        `json:"coordinator_preference"`
	FallbackPolicy        string        `json:"fallback_policy"`
	Members               []SceneMember `json:"members"`
	VolumeRamp            *VolumeRamp   `json:"volume_ramp,omitempty"`
	Teardown              *Teardown     `json:"teardown,omitempty"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

// ExecutionStep represents a single step in scene execution.
type ExecutionStep struct {
	Step      string         `json:"step"`
	Status    StepStatus     `json:"status"`
	StartedAt *time.Time     `json:"started_at,omitempty"`
	EndedAt   *time.Time     `json:"ended_at,omitempty"`
	Error     string         `json:"error,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// Verification contains the result of playback verification.
type Verification struct {
	PlaybackConfirmed       bool      `json:"playback_confirmed"`
	TransportState          string    `json:"transport_state,omitempty"`
	TrackURI                string    `json:"track_uri,omitempty"`
	CheckedAt               time.Time `json:"checked_at"`
	VerificationUnavailable bool      `json:"verification_unavailable,omitempty"`
}

// SceneExecution represents a single execution of a scene.
type SceneExecution struct {
	SceneExecutionID        string          `json:"scene_execution_id"`
	SceneID                 string          `json:"scene_id"`
	IdempotencyKey          *string         `json:"idempotency_key,omitempty"`
	CoordinatorUsedDeviceID *string         `json:"coordinator_used_device_id,omitempty"`
	Status                  ExecutionStatus `json:"status"`
	StartedAt               time.Time       `json:"started_at"`
	EndedAt                 *time.Time      `json:"ended_at,omitempty"`
	Steps                   []ExecutionStep `json:"steps"`
	Verification            *Verification   `json:"verification,omitempty"`
	Error                   *string         `json:"error,omitempty"`
}

// MusicContent represents the content to play.
type MusicContent struct {
	Type            string `json:"type"` // sonos_favorite, direct
	SonosFavoriteID string `json:"sonos_favorite_id,omitempty"`
	URI             string `json:"uri,omitempty"`
	Metadata        string `json:"metadata,omitempty"`
}

// ExecuteOptions contains options for scene execution.
type ExecuteOptions struct {
	MusicContent  *MusicContent `json:"content,omitempty"`
	QueueMode     QueueMode     `json:"queue_mode,omitempty"`
	GroupBehavior GroupBehavior `json:"group_behavior,omitempty"`
	TVPolicy      TVPolicy      `json:"tv_policy,omitempty"`
	FavoriteID    string        `json:"favorite_id,omitempty"` // deprecated
}

// CreateSceneInput contains the input for creating a scene.
type CreateSceneInput struct {
	Name                  string        `json:"name"`
	Description           *string       `json:"description,omitempty"`
	CoordinatorPreference string        `json:"coordinator_preference,omitempty"`
	FallbackPolicy        string        `json:"fallback_policy,omitempty"`
	Members               []SceneMember `json:"members"`
	VolumeRamp            *VolumeRamp   `json:"volume_ramp,omitempty"`
	Teardown              *Teardown     `json:"teardown,omitempty"`
}

// UpdateSceneInput contains the input for updating a scene.
type UpdateSceneInput struct {
	Name                  *string       `json:"name,omitempty"`
	Description           *string       `json:"description,omitempty"`
	CoordinatorPreference *string       `json:"coordinator_preference,omitempty"`
	FallbackPolicy        *string       `json:"fallback_policy,omitempty"`
	Members               []SceneMember `json:"members,omitempty"`
	VolumeRamp            *VolumeRamp   `json:"volume_ramp,omitempty"`
	Teardown              *Teardown     `json:"teardown,omitempty"`
}

// CreateExecutionInput contains the input for creating an execution.
type CreateExecutionInput struct {
	SceneID        string
	IdempotencyKey *string
}

// StepUpdate contains the fields to update on an execution step.
type StepUpdate struct {
	Status    *StepStatus
	StartedAt *time.Time
	EndedAt   *time.Time
	Error     *string
	Details   map[string]any
}

// DeviceResult represents the result of an operation on a single device.
type DeviceResult struct {
	DeviceID string `json:"device_id"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// DefaultExecutionSteps returns the initial steps for a new execution.
func DefaultExecutionSteps() []ExecutionStep {
	steps := []string{
		"acquire_lock",
		"determine_coordinator",
		"ensure_group",
		"apply_volume",
		"pre_flight_check",
		"start_playback",
		"verify_playback",
		"release_lock",
	}
	result := make([]ExecutionStep, len(steps))
	for i, step := range steps {
		result[i] = ExecutionStep{
			Step:   step,
			Status: StepStatusPending,
		}
	}
	return result
}
