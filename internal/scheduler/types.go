package scheduler

import (
	"database/sql"
	"time"
)

// ==========================================================================
// Status and Behavior Types
// ==========================================================================

// JobStatus represents the state of a scheduled job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "PENDING"
	JobStatusScheduled  JobStatus = "SCHEDULED"
	JobStatusClaimed    JobStatus = "CLAIMED"
	JobStatusRunning    JobStatus = "RUNNING"
	JobStatusCompleted  JobStatus = "COMPLETED"
	JobStatusFailed     JobStatus = "FAILED"
	JobStatusSkipped    JobStatus = "SKIPPED"
	JobStatusRetrying   JobStatus = "RETRYING"
)

// HolidayBehavior represents how a routine handles holidays.
type HolidayBehavior string

const (
	HolidayBehaviorSkip  HolidayBehavior = "SKIP"
	HolidayBehaviorDelay HolidayBehavior = "DELAY"
	HolidayBehaviorRun   HolidayBehavior = "RUN"
)

// ScheduleType represents the type of schedule.
type ScheduleType string

const (
	ScheduleTypeCron     ScheduleType = "CRON"
	ScheduleTypeInterval ScheduleType = "INTERVAL"
	ScheduleTypeOneTime  ScheduleType = "ONE_TIME"
	ScheduleTypeOnce     ScheduleType = "once"
	ScheduleTypeWeekly   ScheduleType = "weekly"
	ScheduleTypeMonthly  ScheduleType = "monthly"
	ScheduleTypeYearly   ScheduleType = "yearly"
)

// ==========================================================================
// Domain Types (for API compatibility)
// ==========================================================================

// Schedule defines when a routine should run (API model).
type Schedule struct {
	Type            ScheduleType `json:"type"`
	CronExpr        *string      `json:"cron_expr,omitempty"`
	IntervalMinutes *int         `json:"interval_minutes,omitempty"`
	RunAt           *time.Time   `json:"run_at,omitempty"`
}

// MusicPolicy defines playback preferences for a routine (API model).
type MusicPolicy struct {
	ShuffleMode     *string `json:"shuffle_mode,omitempty"`
	RepeatMode      *string `json:"repeat_mode,omitempty"`
	PreferredSource *string `json:"preferred_source,omitempty"`
}

// ==========================================================================
// Database Model Types
// ==========================================================================

// Routine represents a scheduled routine (database model).
type Routine struct {
	RoutineID        string          `json:"routine_id"`
	Name             string          `json:"name"`
	Enabled          bool            `json:"enabled"`
	Timezone         string          `json:"timezone"`
	ScheduleType     ScheduleType    `json:"schedule_type"`
	ScheduleWeekdays []int           `json:"schedule_weekdays,omitempty"`
	ScheduleMonth    *int            `json:"schedule_month,omitempty"`
	ScheduleDay      *int            `json:"schedule_day,omitempty"`
	ScheduleTime     string          `json:"schedule_time"`
	HolidayBehavior  HolidayBehavior `json:"holiday_behavior"`
	SceneID          string          `json:"scene_id"`
	MusicPolicyType  MusicPolicyType `json:"music_policy_type,omitempty"`
	SpeakersJSON     []Speaker       `json:"speakers,omitempty"`
	SkipNext         bool            `json:"skip_next"`
	SnoozeUntil      *time.Time      `json:"snooze_until,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`

	// Music configuration fields
	MusicSetID                      *string `json:"music_set_id,omitempty"`
	MusicSonosFavoriteID            *string `json:"music_sonos_favorite_id,omitempty"`
	MusicSonosFavoriteName          *string `json:"music_sonos_favorite_name,omitempty"`
	MusicSonosFavoriteArtworkUrl    *string `json:"music_sonos_favorite_artwork_url,omitempty"`
	MusicSonosFavoriteServiceLogoUrl *string `json:"music_sonos_favorite_service_logo_url,omitempty"`
	MusicSonosFavoriteServiceName   *string `json:"music_sonos_favorite_service_name,omitempty"`
	MusicContentType                *string `json:"music_content_type,omitempty"`
	MusicContentJSON                *string `json:"music_content_json,omitempty"`
	MusicNoRepeatWindowMinutes      *int    `json:"music_no_repeat_window_minutes,omitempty"`
	MusicFallbackBehavior           *string `json:"music_fallback_behavior,omitempty"`
	TemplateID                      *string `json:"template_id,omitempty"`
	ArcTVPolicy                     *string `json:"arc_tv_policy,omitempty"`
	OccasionsEnabled                bool    `json:"occasions_enabled"`

	// API compatibility fields (for serialization with Schedule struct)
	Description *string      `json:"description,omitempty"`
	Schedule    Schedule     `json:"-"` // Excluded from JSON, construct from flat fields
	MusicPolicy *MusicPolicy `json:"music_policy,omitempty"`
	LastRunAt   *time.Time   `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time   `json:"next_run_at,omitempty"`
}

// Job represents a scheduled job instance (database model).
type Job struct {
	JobID            string     `json:"job_id"`
	RoutineID        string     `json:"routine_id"`
	ScheduledFor     time.Time  `json:"scheduled_for"`
	Status           JobStatus  `json:"status"`
	Attempts         int        `json:"attempts"`
	LastError        *string    `json:"last_error,omitempty"`
	SceneExecutionID *string    `json:"scene_execution_id,omitempty"`
	RetryAfter       *time.Time `json:"retry_after,omitempty"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`
	IdempotencyKey   *string    `json:"idempotency_key,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`

	// API compatibility fields
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Result      *string    `json:"result,omitempty"`
}

// Holiday represents a holiday date (database model).
type Holiday struct {
	Date     string `json:"date"`
	Name     string `json:"name"`
	IsCustom bool   `json:"is_custom"`

	// API compatibility fields
	HolidayID string    `json:"holiday_id,omitempty"`
	Recurring bool      `json:"recurring,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// ==========================================================================
// Repository Types
// ==========================================================================

// DBPair interface for dependency injection (matches db.DBPair)
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// RoutinesRepository handles database operations for routines.
type RoutinesRepository struct {
	reader *sql.DB
	writer *sql.DB
}

// JobsRepository handles database operations for jobs.
type JobsRepository struct {
	reader *sql.DB
	writer *sql.DB
}

// HolidaysRepository handles database operations for holidays.
type HolidaysRepository struct {
	reader *sql.DB
	writer *sql.DB
}

// NewRoutinesRepository creates a new RoutinesRepository.
func NewRoutinesRepository(dbPair DBPair) *RoutinesRepository {
	return &RoutinesRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// NewJobsRepository creates a new JobsRepository.
func NewJobsRepository(dbPair DBPair) *JobsRepository {
	return &JobsRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// NewHolidaysRepository creates a new HolidaysRepository.
func NewHolidaysRepository(dbPair DBPair) *HolidaysRepository {
	return &HolidaysRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// ==========================================================================
// API Input Types (for API compatibility)
// ==========================================================================

// CreateRoutineInputAPI contains the input for creating a routine (API model with nested Schedule).
type CreateRoutineInputAPI struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description,omitempty"`
	SceneID         string          `json:"scene_id"`
	Schedule        Schedule        `json:"schedule"`
	MusicPolicy     *MusicPolicy    `json:"music_policy,omitempty"`
	HolidayBehavior HolidayBehavior `json:"holiday_behavior,omitempty"`
	Timezone        string          `json:"timezone"`
	Enabled         *bool           `json:"enabled,omitempty"`
}

// UpdateRoutineInputAPI contains the input for updating a routine (API model with nested Schedule).
type UpdateRoutineInputAPI struct {
	Name            *string          `json:"name,omitempty"`
	Description     *string          `json:"description,omitempty"`
	SceneID         *string          `json:"scene_id,omitempty"`
	Schedule        *Schedule        `json:"schedule,omitempty"`
	MusicPolicy     *MusicPolicy     `json:"music_policy,omitempty"`
	HolidayBehavior *HolidayBehavior `json:"holiday_behavior,omitempty"`
	Timezone        *string          `json:"timezone,omitempty"`
	Enabled         *bool            `json:"enabled,omitempty"`
}

// CreateJobInputAPI contains the input for creating a job (API model).
type CreateJobInputAPI struct {
	RoutineID    string    `json:"routine_id"`
	ScheduledFor time.Time `json:"scheduled_for"`
}

// CreateHolidayInputAPI contains the input for creating a holiday (API model).
type CreateHolidayInputAPI struct {
	Name      string    `json:"name"`
	Date      time.Time `json:"date"`
	Recurring bool      `json:"recurring"`
}
