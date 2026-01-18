package scheduler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ==========================================================================
// Additional Types (not defined in generator.go)
// ==========================================================================

// MusicPolicyType determines how music is selected.
type MusicPolicyType string

const (
	MusicPolicyTypeFixed    MusicPolicyType = "FIXED"
	MusicPolicyTypeRotation MusicPolicyType = "ROTATION"
	MusicPolicyTypeShuffle  MusicPolicyType = "SHUFFLE"
)

// ArcTVPolicy determines behavior when Arc is in TV mode.
type ArcTVPolicy string

const (
	ArcTVPolicySkip        ArcTVPolicy = "SKIP"
	ArcTVPolicyUseFallback ArcTVPolicy = "USE_FALLBACK"
	ArcTVPolicyAlwaysPlay  ArcTVPolicy = "ALWAYS_PLAY"
)

// Speaker represents a speaker configuration for a routine.
type Speaker struct {
	DeviceID string `json:"device_id"`
	Volume   *int   `json:"volume,omitempty"`
}

// ==========================================================================
// Input Types
// ==========================================================================

// CreateRoutineInput contains the input for creating a routine.
type CreateRoutineInput struct {
	Name                       string          `json:"name"`
	Enabled                    *bool           `json:"enabled,omitempty"`
	Timezone                   string          `json:"timezone"`
	ScheduleType               ScheduleType    `json:"schedule_type"`
	ScheduleWeekdays           []int           `json:"schedule_weekdays,omitempty"`
	ScheduleMonth              *int            `json:"schedule_month,omitempty"`
	ScheduleDay                *int            `json:"schedule_day,omitempty"`
	ScheduleTime               string          `json:"schedule_time"`
	HolidayBehavior            HolidayBehavior `json:"holiday_behavior,omitempty"`
	SceneID                    string          `json:"scene_id"`
	MusicMode                  string          `json:"music_mode,omitempty"`
	MusicPolicyType            MusicPolicyType `json:"music_policy_type,omitempty"`
	MusicSetID                 *string         `json:"music_set_id,omitempty"`
	MusicSonosFavoriteID       *string         `json:"music_sonos_favorite_id,omitempty"`
	MusicContentType           *string         `json:"music_content_type,omitempty"`
	MusicContentJSON           *string         `json:"music_content_json,omitempty"`
	MusicNoRepeatWindow        *int            `json:"music_no_repeat_window,omitempty"`
	MusicNoRepeatWindowMinutes *int            `json:"music_no_repeat_window_minutes,omitempty"`
	MusicFallbackBehavior      *string         `json:"music_fallback_behavior,omitempty"`
	ArcTVPolicy                *ArcTVPolicy    `json:"arc_tv_policy,omitempty"`
	TemplateID                 *string         `json:"template_id,omitempty"`
	SpeakersJSON               []Speaker       `json:"speakers,omitempty"`
}

// UpdateRoutineInput contains the input for updating a routine.
type UpdateRoutineInput struct {
	Name                       *string          `json:"name,omitempty"`
	Enabled                    *bool            `json:"enabled,omitempty"`
	Timezone                   *string          `json:"timezone,omitempty"`
	ScheduleType               *ScheduleType    `json:"schedule_type,omitempty"`
	ScheduleWeekdays           []int            `json:"schedule_weekdays,omitempty"`
	ScheduleMonth              *int             `json:"schedule_month,omitempty"`
	ScheduleDay                *int             `json:"schedule_day,omitempty"`
	ScheduleTime               *string          `json:"schedule_time,omitempty"`
	HolidayBehavior            *HolidayBehavior `json:"holiday_behavior,omitempty"`
	SceneID                    *string          `json:"scene_id,omitempty"`
	MusicMode                  *string          `json:"music_mode,omitempty"`
	MusicPolicyType            *MusicPolicyType `json:"music_policy_type,omitempty"`
	MusicSetID                 *string          `json:"music_set_id,omitempty"`
	MusicSonosFavoriteID       *string          `json:"music_sonos_favorite_id,omitempty"`
	MusicContentType           *string          `json:"music_content_type,omitempty"`
	MusicContentJSON           *string          `json:"music_content_json,omitempty"`
	MusicNoRepeatWindow        *int             `json:"music_no_repeat_window,omitempty"`
	MusicNoRepeatWindowMinutes *int             `json:"music_no_repeat_window_minutes,omitempty"`
	MusicFallbackBehavior      *string          `json:"music_fallback_behavior,omitempty"`
	ArcTVPolicy                *ArcTVPolicy     `json:"arc_tv_policy,omitempty"`
	SkipNext                   *bool            `json:"skip_next,omitempty"`
	SnoozeUntil                *time.Time       `json:"snooze_until,omitempty"`
	TemplateID                 *string          `json:"template_id,omitempty"`
	SpeakersJSON               []Speaker        `json:"speakers,omitempty"`
}

// CreateJobInput contains the input for creating a job.
type CreateJobInput struct {
	RoutineID      string    `json:"routine_id"`
	ScheduledFor   time.Time `json:"scheduled_for"`
	IdempotencyKey *string   `json:"idempotency_key,omitempty"`
}

// CreateHolidayInput contains the input for creating a holiday.
type CreateHolidayInput struct {
	Date     time.Time `json:"date"`
	Name     string    `json:"name"`
	IsCustom bool      `json:"is_custom"`
}

// ==========================================================================
// RoutinesRepository Core Methods
// ==========================================================================

// GetByID retrieves a routine by ID.
func (r *RoutinesRepository) GetByID(routineID string) (*Routine, error) {
	row := r.reader.QueryRow(`
		SELECT routine_id, name, enabled, timezone, schedule_type, schedule_weekdays,
			schedule_month, schedule_day, schedule_time, holiday_behavior, scene_id,
			music_policy_type, speakers_json, skip_next, snooze_until, created_at, updated_at,
			music_set_id, music_sonos_favorite_id, template_id, arc_tv_policy,
			music_sonos_favorite_name, music_sonos_favorite_artwork_url,
			music_sonos_favorite_service_logo_url, music_sonos_favorite_service_name,
			music_content_type, music_content_json, music_no_repeat_window_minutes,
			music_fallback_behavior, occasions_enabled, last_run_at
		FROM routines
		WHERE routine_id = ?
	`, routineID)

	return r.scanRoutineRow(row)
}

// scanRoutineRow scans a single row into a Routine.
func (r *RoutinesRepository) scanRoutineRow(row *sql.Row) (*Routine, error) {
	var routine Routine
	var enabled int
	var weekdaysJSON sql.NullString
	var scheduleMonth, scheduleDay sql.NullInt64
	var musicPolicyType sql.NullString
	var speakersJSON sql.NullString
	var skipNext int
	var snoozeUntil sql.NullString
	var createdAt, updatedAt string
	var musicSetID, musicSonosFavoriteID, templateID, arcTVPolicy sql.NullString
	var musicSonosFavoriteName, musicSonosFavoriteArtworkUrl sql.NullString
	var musicSonosFavoriteServiceLogoUrl, musicSonosFavoriteServiceName sql.NullString
	var musicContentType, musicContentJSON sql.NullString
	var musicNoRepeatWindowMinutes sql.NullInt64
	var musicFallbackBehavior sql.NullString
	var occasionsEnabled int
	var lastRunAt sql.NullString

	err := row.Scan(
		&routine.RoutineID,
		&routine.Name,
		&enabled,
		&routine.Timezone,
		&routine.ScheduleType,
		&weekdaysJSON,
		&scheduleMonth,
		&scheduleDay,
		&routine.ScheduleTime,
		&routine.HolidayBehavior,
		&routine.SceneID,
		&musicPolicyType,
		&speakersJSON,
		&skipNext,
		&snoozeUntil,
		&createdAt,
		&updatedAt,
		&musicSetID,
		&musicSonosFavoriteID,
		&templateID,
		&arcTVPolicy,
		&musicSonosFavoriteName,
		&musicSonosFavoriteArtworkUrl,
		&musicSonosFavoriteServiceLogoUrl,
		&musicSonosFavoriteServiceName,
		&musicContentType,
		&musicContentJSON,
		&musicNoRepeatWindowMinutes,
		&musicFallbackBehavior,
		&occasionsEnabled,
		&lastRunAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseRoutine(&routine, enabled, weekdaysJSON, scheduleMonth, scheduleDay, musicPolicyType, speakersJSON, skipNext, snoozeUntil, createdAt, updatedAt, musicSetID, musicSonosFavoriteID, templateID, arcTVPolicy, musicSonosFavoriteName, musicSonosFavoriteArtworkUrl, musicSonosFavoriteServiceLogoUrl, musicSonosFavoriteServiceName, musicContentType, musicContentJSON, musicNoRepeatWindowMinutes, musicFallbackBehavior, occasionsEnabled, lastRunAt)
}

// scanRoutineRows scans a row from rows into a Routine.
func (r *RoutinesRepository) scanRoutineRows(rows *sql.Rows) (*Routine, error) {
	var routine Routine
	var enabled int
	var weekdaysJSON sql.NullString
	var scheduleMonth, scheduleDay sql.NullInt64
	var musicPolicyType sql.NullString
	var speakersJSON sql.NullString
	var skipNext int
	var snoozeUntil sql.NullString
	var createdAt, updatedAt string
	var musicSetID, musicSonosFavoriteID, templateID, arcTVPolicy sql.NullString
	var musicSonosFavoriteName, musicSonosFavoriteArtworkUrl sql.NullString
	var musicSonosFavoriteServiceLogoUrl, musicSonosFavoriteServiceName sql.NullString
	var musicContentType, musicContentJSON sql.NullString
	var musicNoRepeatWindowMinutes sql.NullInt64
	var musicFallbackBehavior sql.NullString
	var occasionsEnabled int
	var lastRunAt sql.NullString

	err := rows.Scan(
		&routine.RoutineID,
		&routine.Name,
		&enabled,
		&routine.Timezone,
		&routine.ScheduleType,
		&weekdaysJSON,
		&scheduleMonth,
		&scheduleDay,
		&routine.ScheduleTime,
		&routine.HolidayBehavior,
		&routine.SceneID,
		&musicPolicyType,
		&speakersJSON,
		&skipNext,
		&snoozeUntil,
		&createdAt,
		&updatedAt,
		&musicSetID,
		&musicSonosFavoriteID,
		&templateID,
		&arcTVPolicy,
		&musicSonosFavoriteName,
		&musicSonosFavoriteArtworkUrl,
		&musicSonosFavoriteServiceLogoUrl,
		&musicSonosFavoriteServiceName,
		&musicContentType,
		&musicContentJSON,
		&musicNoRepeatWindowMinutes,
		&musicFallbackBehavior,
		&occasionsEnabled,
		&lastRunAt,
	)
	if err != nil {
		return nil, err
	}

	return r.parseRoutine(&routine, enabled, weekdaysJSON, scheduleMonth, scheduleDay, musicPolicyType, speakersJSON, skipNext, snoozeUntil, createdAt, updatedAt, musicSetID, musicSonosFavoriteID, templateID, arcTVPolicy, musicSonosFavoriteName, musicSonosFavoriteArtworkUrl, musicSonosFavoriteServiceLogoUrl, musicSonosFavoriteServiceName, musicContentType, musicContentJSON, musicNoRepeatWindowMinutes, musicFallbackBehavior, occasionsEnabled, lastRunAt)
}

// parseRoutine parses nullable fields into a Routine.
func (r *RoutinesRepository) parseRoutine(routine *Routine, enabled int, weekdaysJSON sql.NullString, scheduleMonth, scheduleDay sql.NullInt64, musicPolicyType, speakersJSON sql.NullString, skipNext int, snoozeUntil sql.NullString, createdAt, updatedAt string, musicSetID, musicSonosFavoriteID, templateID, arcTVPolicy, musicSonosFavoriteName, musicSonosFavoriteArtworkUrl, musicSonosFavoriteServiceLogoUrl, musicSonosFavoriteServiceName, musicContentType, musicContentJSON sql.NullString, musicNoRepeatWindowMinutes sql.NullInt64, musicFallbackBehavior sql.NullString, occasionsEnabled int, lastRunAt sql.NullString) (*Routine, error) {
	routine.Enabled = enabled == 1
	routine.SkipNext = skipNext == 1
	routine.OccasionsEnabled = occasionsEnabled == 1

	if weekdaysJSON.Valid && weekdaysJSON.String != "" {
		if err := json.Unmarshal([]byte(weekdaysJSON.String), &routine.ScheduleWeekdays); err != nil {
			return nil, fmt.Errorf("failed to parse schedule_weekdays: %w", err)
		}
	}

	if scheduleMonth.Valid {
		month := int(scheduleMonth.Int64)
		routine.ScheduleMonth = &month
	}

	if scheduleDay.Valid {
		day := int(scheduleDay.Int64)
		routine.ScheduleDay = &day
	}

	if musicPolicyType.Valid {
		routine.MusicPolicyType = MusicPolicyType(musicPolicyType.String)
	}

	if speakersJSON.Valid && speakersJSON.String != "" {
		if err := json.Unmarshal([]byte(speakersJSON.String), &routine.SpeakersJSON); err != nil {
			return nil, fmt.Errorf("failed to parse speakers_json: %w", err)
		}
	}

	if snoozeUntil.Valid {
		t, err := time.Parse(time.RFC3339, snoozeUntil.String)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", snoozeUntil.String)
		}
		routine.SnoozeUntil = &t
	}

	// Parse additional music configuration fields
	if musicSetID.Valid {
		routine.MusicSetID = &musicSetID.String
	}
	if musicSonosFavoriteID.Valid {
		routine.MusicSonosFavoriteID = &musicSonosFavoriteID.String
	}
	if templateID.Valid {
		routine.TemplateID = &templateID.String
	}
	if arcTVPolicy.Valid {
		routine.ArcTVPolicy = &arcTVPolicy.String
	}
	if musicSonosFavoriteName.Valid {
		routine.MusicSonosFavoriteName = &musicSonosFavoriteName.String
	}
	if musicSonosFavoriteArtworkUrl.Valid {
		routine.MusicSonosFavoriteArtworkUrl = &musicSonosFavoriteArtworkUrl.String
	}
	if musicSonosFavoriteServiceLogoUrl.Valid {
		routine.MusicSonosFavoriteServiceLogoUrl = &musicSonosFavoriteServiceLogoUrl.String
	}
	if musicSonosFavoriteServiceName.Valid {
		routine.MusicSonosFavoriteServiceName = &musicSonosFavoriteServiceName.String
	}
	if musicContentType.Valid {
		routine.MusicContentType = &musicContentType.String
	}
	if musicContentJSON.Valid {
		routine.MusicContentJSON = &musicContentJSON.String
	}
	if musicNoRepeatWindowMinutes.Valid {
		v := int(musicNoRepeatWindowMinutes.Int64)
		routine.MusicNoRepeatWindowMinutes = &v
	}
	if musicFallbackBehavior.Valid {
		routine.MusicFallbackBehavior = &musicFallbackBehavior.String
	}
	if lastRunAt.Valid {
		t, err := time.Parse(time.RFC3339, lastRunAt.String)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", lastRunAt.String)
		}
		routine.LastRunAt = &t
	}

	var err error
	routine.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		routine.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	routine.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		routine.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return routine, nil
}

// ==========================================================================
// RoutinesRepository Additional Methods
// ==========================================================================

// Create creates a new routine.
func (r *RoutinesRepository) Create(input CreateRoutineInput) (*Routine, error) {
	routineID := uuid.New().String()
	now := nowISO()

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	holidayBehavior := input.HolidayBehavior
	if holidayBehavior == "" {
		holidayBehavior = HolidayBehaviorSkip
	}

	musicMode := input.MusicMode
	if musicMode == "" {
		musicMode = "FIXED"
	}

	musicPolicyType := input.MusicPolicyType
	if musicPolicyType == "" {
		musicPolicyType = MusicPolicyTypeFixed
	}

	scheduleType := input.ScheduleType
	if scheduleType == "" {
		scheduleType = ScheduleTypeWeekly
	}

	var weekdaysJSON *string
	if len(input.ScheduleWeekdays) > 0 {
		bytes, err := json.Marshal(input.ScheduleWeekdays)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		weekdaysJSON = &s
	}

	var speakersJSON *string
	if len(input.SpeakersJSON) > 0 {
		bytes, err := json.Marshal(input.SpeakersJSON)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		speakersJSON = &s
	}

	var arcTVPolicyStr *string
	if input.ArcTVPolicy != nil {
		s := string(*input.ArcTVPolicy)
		arcTVPolicyStr = &s
	}

	_, err := r.writer.Exec(`
		INSERT INTO routines (
			routine_id, name, enabled, timezone, schedule_type, schedule_weekdays,
			schedule_month, schedule_day, schedule_time, holiday_behavior, scene_id,
			music_mode, music_policy_type, music_set_id, music_sonos_favorite_id,
			music_content_type, music_content_json, music_no_repeat_window,
			music_no_repeat_window_minutes, music_fallback_behavior, arc_tv_policy,
			skip_next, snooze_until, template_id, speakers_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		routineID, input.Name, boolToInt(enabled), input.Timezone, string(scheduleType),
		weekdaysJSON, input.ScheduleMonth, input.ScheduleDay, input.ScheduleTime,
		string(holidayBehavior), input.SceneID, musicMode, string(musicPolicyType),
		input.MusicSetID, input.MusicSonosFavoriteID, input.MusicContentType,
		input.MusicContentJSON, input.MusicNoRepeatWindow, input.MusicNoRepeatWindowMinutes,
		input.MusicFallbackBehavior, arcTVPolicyStr, 0, nil, input.TemplateID,
		speakersJSON, now, now,
	)
	if err != nil {
		return nil, err
	}

	return r.GetByID(routineID)
}

// List retrieves routines with pagination and optional filtering.
func (r *RoutinesRepository) List(limit, offset int, enabledOnly bool) ([]Routine, int, error) {
	var total int
	var countQuery string
	if enabledOnly {
		countQuery = "SELECT COUNT(*) FROM routines WHERE enabled = 1"
	} else {
		countQuery = "SELECT COUNT(*) FROM routines"
	}
	err := r.reader.QueryRow(countQuery).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	var query string
	if enabledOnly {
		query = `
			SELECT routine_id, name, enabled, timezone, schedule_type, schedule_weekdays,
				schedule_month, schedule_day, schedule_time, holiday_behavior, scene_id,
				music_policy_type, speakers_json, skip_next, snooze_until, created_at, updated_at,
				music_set_id, music_sonos_favorite_id, template_id, arc_tv_policy,
				music_sonos_favorite_name, music_sonos_favorite_artwork_url,
				music_sonos_favorite_service_logo_url, music_sonos_favorite_service_name,
				music_content_type, music_content_json, music_no_repeat_window_minutes,
				music_fallback_behavior, occasions_enabled, last_run_at
			FROM routines
			WHERE enabled = 1
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
	} else {
		query = `
			SELECT routine_id, name, enabled, timezone, schedule_type, schedule_weekdays,
				schedule_month, schedule_day, schedule_time, holiday_behavior, scene_id,
				music_policy_type, speakers_json, skip_next, snooze_until, created_at, updated_at,
				music_set_id, music_sonos_favorite_id, template_id, arc_tv_policy,
				music_sonos_favorite_name, music_sonos_favorite_artwork_url,
				music_sonos_favorite_service_logo_url, music_sonos_favorite_service_name,
				music_content_type, music_content_json, music_no_repeat_window_minutes,
				music_fallback_behavior, occasions_enabled, last_run_at
			FROM routines
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
	}

	rows, err := r.reader.Query(query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var routines []Routine
	for rows.Next() {
		routine, err := r.scanRoutineRows(rows)
		if err != nil {
			return nil, 0, err
		}
		routines = append(routines, *routine)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if routines == nil {
		routines = []Routine{}
	}

	return routines, total, nil
}

// Update updates a routine.
func (r *RoutinesRepository) Update(routineID string, input UpdateRoutineInput) (*Routine, error) {
	existing, err := r.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}

	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	timezone := existing.Timezone
	if input.Timezone != nil {
		timezone = *input.Timezone
	}

	scheduleType := existing.ScheduleType
	if input.ScheduleType != nil {
		scheduleType = *input.ScheduleType
	}

	var scheduleWeekdays *string
	if input.ScheduleWeekdays != nil {
		bytes, err := json.Marshal(input.ScheduleWeekdays)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		scheduleWeekdays = &s
	} else if len(existing.ScheduleWeekdays) > 0 {
		bytes, err := json.Marshal(existing.ScheduleWeekdays)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		scheduleWeekdays = &s
	}

	scheduleMonth := existing.ScheduleMonth
	if input.ScheduleMonth != nil {
		scheduleMonth = input.ScheduleMonth
	}

	scheduleDay := existing.ScheduleDay
	if input.ScheduleDay != nil {
		scheduleDay = input.ScheduleDay
	}

	scheduleTime := existing.ScheduleTime
	if input.ScheduleTime != nil {
		scheduleTime = *input.ScheduleTime
	}

	holidayBehavior := existing.HolidayBehavior
	if input.HolidayBehavior != nil {
		holidayBehavior = *input.HolidayBehavior
	}

	sceneID := existing.SceneID
	if input.SceneID != nil {
		sceneID = *input.SceneID
	}

	skipNext := existing.SkipNext
	if input.SkipNext != nil {
		skipNext = *input.SkipNext
	}

	snoozeUntil := existing.SnoozeUntil
	if input.SnoozeUntil != nil {
		snoozeUntil = input.SnoozeUntil
	}

	var snoozeUntilStr *string
	if snoozeUntil != nil {
		s := snoozeUntil.UTC().Format(time.RFC3339)
		snoozeUntilStr = &s
	}

	// Music policy fields
	musicPolicyType := existing.MusicPolicyType
	if input.MusicPolicyType != nil {
		musicPolicyType = *input.MusicPolicyType
	}

	musicSetID := existing.MusicSetID
	if input.MusicSetID != nil {
		musicSetID = input.MusicSetID
	}

	musicSonosFavoriteID := existing.MusicSonosFavoriteID
	if input.MusicSonosFavoriteID != nil {
		musicSonosFavoriteID = input.MusicSonosFavoriteID
	}

	musicContentType := existing.MusicContentType
	if input.MusicContentType != nil {
		musicContentType = input.MusicContentType
	}

	musicContentJSON := existing.MusicContentJSON
	if input.MusicContentJSON != nil {
		musicContentJSON = input.MusicContentJSON
	}

	musicNoRepeatWindowMinutes := existing.MusicNoRepeatWindowMinutes
	if input.MusicNoRepeatWindowMinutes != nil {
		musicNoRepeatWindowMinutes = input.MusicNoRepeatWindowMinutes
	}

	musicFallbackBehavior := existing.MusicFallbackBehavior
	if input.MusicFallbackBehavior != nil {
		musicFallbackBehavior = input.MusicFallbackBehavior
	}

	arcTVPolicy := existing.ArcTVPolicy
	if input.ArcTVPolicy != nil {
		s := string(*input.ArcTVPolicy)
		arcTVPolicy = &s
	}

	templateID := existing.TemplateID
	if input.TemplateID != nil {
		templateID = input.TemplateID
	}

	// Handle speakers JSON update
	var speakersJSONStr *string
	if input.SpeakersJSON != nil {
		bytes, err := json.Marshal(input.SpeakersJSON)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		speakersJSONStr = &s
	} else if existing.SpeakersJSON != nil {
		bytes, err := json.Marshal(existing.SpeakersJSON)
		if err != nil {
			return nil, err
		}
		s := string(bytes)
		speakersJSONStr = &s
	}

	now := nowISO()
	_, err = r.writer.Exec(`
		UPDATE routines SET
			name = ?, enabled = ?, timezone = ?, schedule_type = ?, schedule_weekdays = ?,
			schedule_month = ?, schedule_day = ?, schedule_time = ?, holiday_behavior = ?,
			scene_id = ?, skip_next = ?, snooze_until = ?,
			music_policy_type = ?, music_set_id = ?, music_sonos_favorite_id = ?,
			music_content_type = ?, music_content_json = ?, music_no_repeat_window_minutes = ?,
			music_fallback_behavior = ?, arc_tv_policy = ?, template_id = ?, speakers_json = ?,
			updated_at = ?
		WHERE routine_id = ?
	`,
		name, boolToInt(enabled), timezone, string(scheduleType), scheduleWeekdays,
		scheduleMonth, scheduleDay, scheduleTime, string(holidayBehavior), sceneID,
		boolToInt(skipNext), snoozeUntilStr,
		string(musicPolicyType), musicSetID, musicSonosFavoriteID,
		musicContentType, musicContentJSON, musicNoRepeatWindowMinutes,
		musicFallbackBehavior, arcTVPolicy, templateID, speakersJSONStr,
		now, routineID,
	)
	if err != nil {
		return nil, err
	}

	return r.GetByID(routineID)
}

// ClearSnooze removes the snooze from a routine.
func (r *RoutinesRepository) ClearSnooze(routineID string) (*Routine, error) {
	existing, err := r.GetByID(routineID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	now := nowISO()
	_, err = r.writer.Exec(`
		UPDATE routines SET snooze_until = NULL, updated_at = ?
		WHERE routine_id = ?
	`, now, routineID)
	if err != nil {
		return nil, err
	}

	return r.GetByID(routineID)
}

// Delete deletes a routine.
func (r *RoutinesRepository) Delete(routineID string) error {
	result, err := r.writer.Exec("DELETE FROM routines WHERE routine_id = ?", routineID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// UpdateNextRunAt updates the next run time for a routine.
// Note: The current schema doesn't have a next_run_at column, so this updates updated_at.
func (r *RoutinesRepository) UpdateNextRunAt(routineID string, nextRunAt time.Time) error {
	now := nowISO()
	_, err := r.writer.Exec(`
		UPDATE routines SET updated_at = ?
		WHERE routine_id = ?
	`, now, routineID)
	return err
}

// GetDueRoutines returns routines that are due to run.
// Note: This returns all enabled routines that are not snoozed and don't have skip_next set.
// The actual scheduling logic is handled by the scheduler service.
func (r *RoutinesRepository) GetDueRoutines(now time.Time) ([]Routine, error) {
	rows, err := r.reader.Query(`
		SELECT routine_id, name, enabled, timezone, schedule_type, schedule_weekdays,
			schedule_month, schedule_day, schedule_time, holiday_behavior, scene_id,
			music_policy_type, speakers_json, skip_next, snooze_until, created_at, updated_at,
			music_set_id, music_sonos_favorite_id, template_id, arc_tv_policy,
			music_sonos_favorite_name, music_sonos_favorite_artwork_url,
			music_sonos_favorite_service_logo_url, music_sonos_favorite_service_name,
			music_content_type, music_content_json, music_no_repeat_window_minutes,
			music_fallback_behavior, occasions_enabled, last_run_at
		FROM routines
		WHERE enabled = 1 AND skip_next = 0
		  AND (snooze_until IS NULL OR snooze_until <= ?)
	`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var routines []Routine
	for rows.Next() {
		routine, err := r.scanRoutineRows(rows)
		if err != nil {
			return nil, err
		}
		routines = append(routines, *routine)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if routines == nil {
		routines = []Routine{}
	}

	return routines, nil
}

// ==========================================================================
// JobsRepository Core Methods
// ==========================================================================

// GetByID retrieves a job by ID.
func (r *JobsRepository) GetByID(jobID string) (*Job, error) {
	row := r.reader.QueryRow(`
		SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
			scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
		FROM jobs
		WHERE job_id = ?
	`, jobID)

	return r.scanJobRow(row)
}

// scanJobRow scans a single row into a Job.
func (r *JobsRepository) scanJobRow(row *sql.Row) (*Job, error) {
	var job Job
	var lastError, sceneExecutionID, retryAfter, claimedAt, idempotencyKey sql.NullString
	var scheduledFor, createdAt, updatedAt string
	var status string

	err := row.Scan(
		&job.JobID,
		&job.RoutineID,
		&scheduledFor,
		&status,
		&job.Attempts,
		&lastError,
		&sceneExecutionID,
		&retryAfter,
		&claimedAt,
		&idempotencyKey,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseJob(&job, status, scheduledFor, lastError, sceneExecutionID, retryAfter, claimedAt, idempotencyKey, createdAt, updatedAt)
}

// parseJob parses nullable fields into a Job.
func (r *JobsRepository) parseJob(job *Job, status, scheduledFor string, lastError, sceneExecutionID, retryAfter, claimedAt, idempotencyKey sql.NullString, createdAt, updatedAt string) (*Job, error) {
	job.Status = JobStatus(status)

	var err error
	job.ScheduledFor, err = time.Parse(time.RFC3339, scheduledFor)
	if err != nil {
		job.ScheduledFor, _ = time.Parse("2006-01-02 15:04:05", scheduledFor)
	}

	if lastError.Valid {
		job.LastError = &lastError.String
	}
	if sceneExecutionID.Valid {
		job.SceneExecutionID = &sceneExecutionID.String
	}
	if retryAfter.Valid {
		t, err := time.Parse(time.RFC3339, retryAfter.String)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", retryAfter.String)
		}
		job.RetryAfter = &t
	}
	if claimedAt.Valid {
		t, err := time.Parse(time.RFC3339, claimedAt.String)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", claimedAt.String)
		}
		job.ClaimedAt = &t
	}
	if idempotencyKey.Valid {
		job.IdempotencyKey = &idempotencyKey.String
	}

	job.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		job.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	job.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		job.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return job, nil
}

// ==========================================================================
// JobsRepository Additional Methods
// ==========================================================================

// Create creates a new job (alias for CreateWithInput).
func (r *JobsRepository) Create(input CreateJobInput) (*Job, error) {
	return r.CreateWithInput(input)
}

// CreateWithInput creates a new job from a CreateJobInput struct.
func (r *JobsRepository) CreateWithInput(input CreateJobInput) (*Job, error) {
	jobID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	scheduledForStr := input.ScheduledFor.UTC().Format(time.RFC3339)

	idempotencyKey := input.IdempotencyKey
	if idempotencyKey == nil {
		key := fmt.Sprintf("%s:%s", input.RoutineID, scheduledForStr)
		idempotencyKey = &key
	}

	_, err := r.writer.Exec(`
		INSERT INTO jobs (job_id, routine_id, scheduled_for, status, attempts, idempotency_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, jobID, input.RoutineID, scheduledForStr, string(JobStatusPending), 0, idempotencyKey, now, now)
	if err != nil {
		return nil, err
	}

	return r.GetByID(jobID)
}

// ListByRoutineID retrieves jobs for a routine with pagination.
func (r *JobsRepository) ListByRoutineID(routineID string, limit, offset int) ([]Job, int, error) {
	var total int
	err := r.reader.QueryRow("SELECT COUNT(*) FROM jobs WHERE routine_id = ?", routineID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.reader.Query(`
		SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
			scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
		FROM jobs
		WHERE routine_id = ?
		ORDER BY scheduled_for DESC
		LIMIT ? OFFSET ?
	`, routineID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := r.scanJobRows(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if jobs == nil {
		jobs = []Job{}
	}

	return jobs, total, nil
}

// GetPendingJobs retrieves scheduled jobs ordered by scheduled_for.
func (r *JobsRepository) GetPendingJobs(limit int) ([]Job, error) {
	rows, err := r.reader.Query(`
		SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
			scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
		FROM jobs
		WHERE status = ?
		ORDER BY scheduled_for ASC
		LIMIT ?
	`, string(JobStatusPending), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := r.scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if jobs == nil {
		jobs = []Job{}
	}

	return jobs, nil
}

// ClaimJob atomically sets status=CLAIMED and claimed_at=now.
func (r *JobsRepository) ClaimJob(jobID string) error {
	now := nowISO()
	result, err := r.writer.Exec(`
		UPDATE jobs SET status = ?, claimed_at = ?, updated_at = ?
		WHERE job_id = ? AND status = ?
	`, string(JobStatusClaimed), now, now, jobID, string(JobStatusPending))
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("job not found or already claimed")
	}

	return nil
}

// StartJob sets status=RUNNING.
func (r *JobsRepository) StartJob(jobID string) error {
	now := nowISO()
	_, err := r.writer.Exec(`
		UPDATE jobs SET status = ?, updated_at = ?
		WHERE job_id = ?
	`, string(JobStatusRunning), now, jobID)
	return err
}

// CompleteJob sets status=COMPLETED.
func (r *JobsRepository) CompleteJob(jobID string, sceneExecutionID string) error {
	now := nowISO()
	var execID *string
	if sceneExecutionID != "" {
		execID = &sceneExecutionID
	}
	_, err := r.writer.Exec(`
		UPDATE jobs SET status = ?, scene_execution_id = ?, updated_at = ?
		WHERE job_id = ?
	`, string(JobStatusCompleted), execID, now, jobID)
	return err
}

// FailJob increments attempts, sets last_error, and conditionally sets status=FAILED.
func (r *JobsRepository) FailJob(jobID string, errMsg string, canRetry bool) error {
	now := nowISO()

	if canRetry {
		// Increment attempts, set error, but keep status as PENDING for retry
		_, err := r.writer.Exec(`
			UPDATE jobs SET
				attempts = attempts + 1,
				last_error = ?,
				status = ?,
				claimed_at = NULL,
				updated_at = ?
			WHERE job_id = ?
		`, errMsg, string(JobStatusPending), now, jobID)
		return err
	}

	// No retry, mark as failed
	_, err := r.writer.Exec(`
		UPDATE jobs SET
			attempts = attempts + 1,
			last_error = ?,
			status = ?,
			updated_at = ?
		WHERE job_id = ?
	`, errMsg, string(JobStatusFailed), now, jobID)
	return err
}

// SkipJob sets status=SKIPPED.
func (r *JobsRepository) SkipJob(jobID string, reason string) error {
	now := nowISO()
	_, err := r.writer.Exec(`
		UPDATE jobs SET status = ?, last_error = ?, updated_at = ?
		WHERE job_id = ?
	`, string(JobStatusSkipped), reason, now, jobID)
	return err
}

// GetStaleClaimedJobs returns jobs that were claimed but not completed within the timeout.
func (r *JobsRepository) GetStaleClaimedJobs(olderThan time.Duration) ([]Job, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	rows, err := r.reader.Query(`
		SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
			scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
		FROM jobs
		WHERE status = ? AND claimed_at < ?
	`, string(JobStatusClaimed), cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := r.scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if jobs == nil {
		jobs = []Job{}
	}

	return jobs, nil
}

func (r *JobsRepository) scanJobRows(rows *sql.Rows) (*Job, error) {
	var job Job
	var lastError, sceneExecutionID, retryAfter, claimedAt, idempotencyKey sql.NullString
	var scheduledFor, createdAt, updatedAt string
	var status string

	err := rows.Scan(
		&job.JobID,
		&job.RoutineID,
		&scheduledFor,
		&status,
		&job.Attempts,
		&lastError,
		&sceneExecutionID,
		&retryAfter,
		&claimedAt,
		&idempotencyKey,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	return r.parseJob(&job, status, scheduledFor, lastError, sceneExecutionID, retryAfter, claimedAt, idempotencyKey, createdAt, updatedAt)
}

// ==========================================================================
// HolidaysRepository Additional Methods
// ==========================================================================

// Create creates a new holiday.
func (r *HolidaysRepository) Create(input CreateHolidayInput) (*Holiday, error) {
	dateStr := input.Date.Format("2006-01-02")

	_, err := r.writer.Exec(`
		INSERT INTO holidays (date, name, is_custom)
		VALUES (?, ?, ?)
	`, dateStr, input.Name, boolToInt(input.IsCustom))
	if err != nil {
		return nil, err
	}

	return r.GetByID(dateStr)
}

// GetByID retrieves a holiday by date string.
func (r *HolidaysRepository) GetByID(holidayID string) (*Holiday, error) {
	var holiday Holiday
	var isCustom int

	err := r.reader.QueryRow(`
		SELECT date, name, is_custom
		FROM holidays
		WHERE date = ?
	`, holidayID).Scan(&holiday.Date, &holiday.Name, &isCustom)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	holiday.IsCustom = isCustom == 1

	return &holiday, nil
}

// List retrieves holidays with pagination.
func (r *HolidaysRepository) List(limit, offset int) ([]Holiday, int, error) {
	var total int
	err := r.reader.QueryRow("SELECT COUNT(*) FROM holidays").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.reader.Query(`
		SELECT date, name, is_custom
		FROM holidays
		ORDER BY date ASC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var holidays []Holiday
	for rows.Next() {
		var holiday Holiday
		var isCustom int

		if err := rows.Scan(&holiday.Date, &holiday.Name, &isCustom); err != nil {
			return nil, 0, err
		}

		holiday.IsCustom = isCustom == 1

		holidays = append(holidays, holiday)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if holidays == nil {
		holidays = []Holiday{}
	}

	return holidays, total, nil
}

// Delete deletes a holiday.
func (r *HolidaysRepository) Delete(holidayID string) error {
	result, err := r.writer.Exec("DELETE FROM holidays WHERE date = ?", holidayID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// IsHoliday checks if a date is a holiday (alias for IsHolidayWithDetails).
func (r *HolidaysRepository) IsHoliday(date time.Time) (bool, *Holiday, error) {
	return r.IsHolidayWithDetails(date)
}

// IsHolidayWithDetails checks if a date is a holiday and returns the holiday details.
func (r *HolidaysRepository) IsHolidayWithDetails(date time.Time) (bool, *Holiday, error) {
	dateStr := date.Format("2006-01-02")

	holiday, err := r.GetByID(dateStr)
	if err != nil {
		return false, nil, err
	}

	if holiday != nil {
		return true, holiday, nil
	}

	return false, nil, nil
}

// GetHolidaysInRange retrieves holidays within a date range.
func (r *HolidaysRepository) GetHolidaysInRange(start, end time.Time) ([]Holiday, error) {
	startStr := start.Format("2006-01-02")
	endStr := end.Format("2006-01-02")

	rows, err := r.reader.Query(`
		SELECT date, name, is_custom
		FROM holidays
		WHERE date >= ? AND date <= ?
		ORDER BY date ASC
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var holidays []Holiday
	for rows.Next() {
		var holiday Holiday
		var isCustom int

		if err := rows.Scan(&holiday.Date, &holiday.Name, &isCustom); err != nil {
			return nil, err
		}

		holiday.IsCustom = isCustom == 1

		holidays = append(holidays, holiday)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if holidays == nil {
		holidays = []Holiday{}
	}

	return holidays, nil
}

// UpdateLastRunAt updates the last_run_at timestamp for a routine.
func (r *RoutinesRepository) UpdateLastRunAt(routineID string, lastRunAt time.Time) error {
	now := nowISO()
	lastRunAtStr := lastRunAt.UTC().Format(time.RFC3339)
	_, err := r.writer.Exec(`
		UPDATE routines SET last_run_at = ?, updated_at = ?
		WHERE routine_id = ?
	`, lastRunAtStr, now, routineID)
	return err
}

// SetRetryAfter sets the retry_after timestamp for a job.
func (r *JobsRepository) SetRetryAfter(jobID string, retryAfter time.Time) error {
	now := nowISO()
	retryAfterStr := retryAfter.UTC().Format(time.RFC3339)
	_, err := r.writer.Exec(`
		UPDATE jobs SET retry_after = ?, updated_at = ?
		WHERE job_id = ?
	`, retryAfterStr, now, jobID)
	return err
}

// ListAll retrieves all jobs with pagination and optional status filtering.
func (r *JobsRepository) ListAll(limit, offset int, statusFilter string) ([]Job, int, error) {
	var total int
	var countQuery string
	var args []any

	if statusFilter != "" {
		countQuery = "SELECT COUNT(*) FROM jobs WHERE status = ?"
		args = append(args, statusFilter)
	} else {
		countQuery = "SELECT COUNT(*) FROM jobs"
	}

	err := r.reader.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	var query string
	if statusFilter != "" {
		query = `
			SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
				scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
			FROM jobs
			WHERE status = ?
			ORDER BY scheduled_for DESC
			LIMIT ? OFFSET ?
		`
		args = []any{statusFilter, limit, offset}
	} else {
		query = `
			SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
				scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
			FROM jobs
			ORDER BY scheduled_for DESC
			LIMIT ? OFFSET ?
		`
		args = []any{limit, offset}
	}

	rows, err := r.reader.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := r.scanJobRows(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if jobs == nil {
		jobs = []Job{}
	}

	return jobs, total, nil
}

// GetStaleRunningJobs returns jobs that are in RUNNING state but haven't completed within the timeout.
func (r *JobsRepository) GetStaleRunningJobs(olderThan time.Duration) ([]Job, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	rows, err := r.reader.Query(`
		SELECT job_id, routine_id, scheduled_for, status, attempts, last_error,
			scene_execution_id, retry_after, claimed_at, idempotency_key, created_at, updated_at
		FROM jobs
		WHERE status = ? AND claimed_at < ?
	`, string(JobStatusRunning), cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		job, err := r.scanJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if jobs == nil {
		jobs = []Job{}
	}

	return jobs, nil
}

// ==========================================================================
// Helpers
// ==========================================================================

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
