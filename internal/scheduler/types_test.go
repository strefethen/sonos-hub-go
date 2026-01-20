package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScheduleJSON(t *testing.T) {
	cronExpr := "0 7 * * *"
	schedule := Schedule{
		Type:     ScheduleTypeCron,
		CronExpr: &cronExpr,
	}

	data, err := json.Marshal(schedule)
	require.NoError(t, err)

	var decoded Schedule
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, ScheduleTypeCron, decoded.Type)
	require.NotNil(t, decoded.CronExpr)
	require.Equal(t, "0 7 * * *", *decoded.CronExpr)
	require.Nil(t, decoded.IntervalMinutes)
	require.Nil(t, decoded.RunAt)
}

func TestScheduleIntervalJSON(t *testing.T) {
	intervalMinutes := 30
	schedule := Schedule{
		Type:            ScheduleTypeInterval,
		IntervalMinutes: &intervalMinutes,
	}

	data, err := json.Marshal(schedule)
	require.NoError(t, err)

	var decoded Schedule
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, ScheduleTypeInterval, decoded.Type)
	require.Nil(t, decoded.CronExpr)
	require.NotNil(t, decoded.IntervalMinutes)
	require.Equal(t, 30, *decoded.IntervalMinutes)
	require.Nil(t, decoded.RunAt)
}

func TestScheduleOneTimeJSON(t *testing.T) {
	runAt := time.Now().UTC().Truncate(time.Second)
	schedule := Schedule{
		Type:  ScheduleTypeOneTime,
		RunAt: &runAt,
	}

	data, err := json.Marshal(schedule)
	require.NoError(t, err)

	var decoded Schedule
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, ScheduleTypeOneTime, decoded.Type)
	require.Nil(t, decoded.CronExpr)
	require.Nil(t, decoded.IntervalMinutes)
	require.NotNil(t, decoded.RunAt)
	require.Equal(t, runAt, *decoded.RunAt)
}

func TestScheduleJSONOmitsEmpty(t *testing.T) {
	schedule := Schedule{
		Type: ScheduleTypeCron,
	}

	data, err := json.Marshal(schedule)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "CRON", m["type"])
	_, hasCronExpr := m["cron_expr"]
	require.False(t, hasCronExpr)
	_, hasInterval := m["interval_minutes"]
	require.False(t, hasInterval)
	_, hasRunAt := m["run_at"]
	require.False(t, hasRunAt)
}

func TestMusicPolicyJSON(t *testing.T) {
	favoriteID := "FV:2/77"
	favoriteName := "Tim McGraw"
	artworkUrl := "https://example.com/artwork.jpg"
	serviceLogoUrl := "/v1/assets/service-logos/spotify.png"
	serviceName := "Spotify"

	policy := MusicPolicy{
		Type:                       "FIXED",
		SonosFavoriteID:            &favoriteID,
		SonosFavoriteName:          &favoriteName,
		SonosFavoriteArtworkUrl:    &artworkUrl,
		SonosFavoriteServiceLogoUrl: &serviceLogoUrl,
		SonosFavoriteServiceName:   &serviceName,
	}

	data, err := json.Marshal(policy)
	require.NoError(t, err)

	var decoded MusicPolicy
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "FIXED", decoded.Type)
	require.NotNil(t, decoded.SonosFavoriteID)
	require.Equal(t, "FV:2/77", *decoded.SonosFavoriteID)
	require.NotNil(t, decoded.SonosFavoriteName)
	require.Equal(t, "Tim McGraw", *decoded.SonosFavoriteName)
	require.NotNil(t, decoded.SonosFavoriteServiceLogoUrl)
	require.Equal(t, "/v1/assets/service-logos/spotify.png", *decoded.SonosFavoriteServiceLogoUrl)
	require.NotNil(t, decoded.SonosFavoriteServiceName)
	require.Equal(t, "Spotify", *decoded.SonosFavoriteServiceName)
}

func TestMusicPolicyJSONOmitsEmpty(t *testing.T) {
	policy := MusicPolicy{}

	data, err := json.Marshal(policy)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasFavoriteID := m["sonos_favorite_id"]
	require.False(t, hasFavoriteID)
	_, hasFavoriteName := m["sonos_favorite_name"]
	require.False(t, hasFavoriteName)
	_, hasServiceLogoUrl := m["sonos_favorite_service_logo_url"]
	require.False(t, hasServiceLogoUrl)
}

func TestRoutineJSON(t *testing.T) {
	description := "Morning music routine"
	favoriteID := "FV:2/77"
	now := time.Now().UTC().Truncate(time.Second)
	lastRun := now.Add(-24 * time.Hour)
	nextRun := now.Add(time.Hour)

	routine := Routine{
		RoutineID:        "routine-123",
		Name:             "Morning Music",
		Description:      &description,
		SceneID:          "scene-456",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "07:00",
		MusicPolicy: &MusicPolicy{
			Type:            "FIXED",
			SonosFavoriteID: &favoriteID,
		},
		HolidayBehavior: HolidayBehaviorSkip,
		Timezone:        "America/New_York",
		Enabled:         true,
		LastRunAt:       &lastRun,
		NextRunAt:       &nextRun,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	data, err := json.Marshal(routine)
	require.NoError(t, err)

	var decoded Routine
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "routine-123", decoded.RoutineID)
	require.Equal(t, "Morning Music", decoded.Name)
	require.NotNil(t, decoded.Description)
	require.Equal(t, "Morning music routine", *decoded.Description)
	require.Equal(t, "scene-456", decoded.SceneID)
	require.Equal(t, ScheduleTypeWeekly, decoded.ScheduleType)
	require.Equal(t, []int{1, 2, 3, 4, 5}, decoded.ScheduleWeekdays)
	require.Equal(t, "07:00", decoded.ScheduleTime)
	require.NotNil(t, decoded.MusicPolicy)
	require.Equal(t, "FIXED", decoded.MusicPolicy.Type)
	require.NotNil(t, decoded.MusicPolicy.SonosFavoriteID)
	require.Equal(t, "FV:2/77", *decoded.MusicPolicy.SonosFavoriteID)
	require.Equal(t, HolidayBehaviorSkip, decoded.HolidayBehavior)
	require.Equal(t, "America/New_York", decoded.Timezone)
	require.True(t, decoded.Enabled)
	require.NotNil(t, decoded.LastRunAt)
	require.NotNil(t, decoded.NextRunAt)
}

func TestRoutineJSONMinimal(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	routine := Routine{
		RoutineID:       "routine-123",
		Name:            "Simple Routine",
		SceneID:         "scene-456",
		ScheduleType:    ScheduleTypeWeekly,
		ScheduleTime:    "08:00",
		HolidayBehavior: HolidayBehaviorSkip,
		Timezone:        "UTC",
		Enabled:         false,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	data, err := json.Marshal(routine)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasDescription := m["description"]
	require.False(t, hasDescription)
	_, hasMusicPolicy := m["music_policy"]
	require.False(t, hasMusicPolicy)
	_, hasLastRun := m["last_run_at"]
	require.False(t, hasLastRun)
	_, hasNextRun := m["next_run_at"]
	require.False(t, hasNextRun)
}

func TestJobJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scheduledFor := now.Add(time.Hour)
	claimedAt := now.Add(59 * time.Minute)
	startedAt := now.Add(60 * time.Minute)
	completedAt := now.Add(61 * time.Minute)
	result := `{"execution_id":"exec-789","status":"success"}`

	job := Job{
		JobID:        "job-123",
		RoutineID:    "routine-456",
		ScheduledFor: scheduledFor,
		Status:       JobStatusCompleted,
		ClaimedAt:    &claimedAt,
		StartedAt:    &startedAt,
		CompletedAt:  &completedAt,
		Attempts:     1,
		LastError:    nil,
		Result:       &result,
		CreatedAt:    now,
	}

	data, err := json.Marshal(job)
	require.NoError(t, err)

	var decoded Job
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "job-123", decoded.JobID)
	require.Equal(t, "routine-456", decoded.RoutineID)
	require.Equal(t, scheduledFor, decoded.ScheduledFor)
	require.Equal(t, JobStatusCompleted, decoded.Status)
	require.NotNil(t, decoded.ClaimedAt)
	require.NotNil(t, decoded.StartedAt)
	require.NotNil(t, decoded.CompletedAt)
	require.Equal(t, 1, decoded.Attempts)
	require.Nil(t, decoded.LastError)
	require.NotNil(t, decoded.Result)
	require.Equal(t, result, *decoded.Result)
}

func TestJobJSONFailed(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	scheduledFor := now.Add(time.Hour)
	lastError := "scene execution failed: timeout"

	job := Job{
		JobID:        "job-123",
		RoutineID:    "routine-456",
		ScheduledFor: scheduledFor,
		Status:       JobStatusFailed,
		Attempts:     3,
		LastError:    &lastError,
		CreatedAt:    now,
	}

	data, err := json.Marshal(job)
	require.NoError(t, err)

	var decoded Job
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, JobStatusFailed, decoded.Status)
	require.Equal(t, 3, decoded.Attempts)
	require.NotNil(t, decoded.LastError)
	require.Equal(t, "scene execution failed: timeout", *decoded.LastError)
	require.Nil(t, decoded.Result)
}

func TestJobJSONOmitsEmpty(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	job := Job{
		JobID:        "job-123",
		RoutineID:    "routine-456",
		ScheduledFor: now,
		Status:       JobStatusScheduled,
		Attempts:     0,
		CreatedAt:    now,
	}

	data, err := json.Marshal(job)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasClaimed := m["claimed_at"]
	require.False(t, hasClaimed)
	_, hasStarted := m["started_at"]
	require.False(t, hasStarted)
	_, hasCompleted := m["completed_at"]
	require.False(t, hasCompleted)
	_, hasLastError := m["last_error"]
	require.False(t, hasLastError)
	_, hasResult := m["result"]
	require.False(t, hasResult)
}

func TestHolidayJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	holidayDate := "2024-12-25"

	holiday := Holiday{
		HolidayID: "holiday-123",
		Name:      "Christmas",
		Date:      holidayDate,
		Recurring: true,
		CreatedAt: now,
	}

	data, err := json.Marshal(holiday)
	require.NoError(t, err)

	var decoded Holiday
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "holiday-123", decoded.HolidayID)
	require.Equal(t, "Christmas", decoded.Name)
	require.Equal(t, holidayDate, decoded.Date)
	require.True(t, decoded.Recurring)
}

func TestHolidayJSONNonRecurring(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	holidayDate := "2024-07-04"

	holiday := Holiday{
		HolidayID: "holiday-456",
		Name:      "Company Event",
		Date:      holidayDate,
		Recurring: false,
		CreatedAt: now,
	}

	data, err := json.Marshal(holiday)
	require.NoError(t, err)

	var decoded Holiday
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "holiday-456", decoded.HolidayID)
	require.Equal(t, "Company Event", decoded.Name)
	require.False(t, decoded.Recurring)
}

func TestCreateRoutineInputJSON(t *testing.T) {
	description := "Test routine"
	cronExpr := "0 8 * * *"
	enabled := true
	favoriteID := "FV:2/77"
	input := CreateRoutineInputAPI{
		Name:        "Test Routine",
		Description: &description,
		SceneID:     "scene-123",
		Schedule: Schedule{
			Type:     ScheduleTypeCron,
			CronExpr: &cronExpr,
		},
		MusicPolicy: &MusicPolicy{
			Type:            "FIXED",
			SonosFavoriteID: &favoriteID,
		},
		HolidayBehavior: HolidayBehaviorDelay,
		Timezone:        "America/Los_Angeles",
		Enabled:         &enabled,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded CreateRoutineInputAPI
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "Test Routine", decoded.Name)
	require.NotNil(t, decoded.Description)
	require.Equal(t, "Test routine", *decoded.Description)
	require.Equal(t, "scene-123", decoded.SceneID)
	require.Equal(t, ScheduleTypeCron, decoded.Schedule.Type)
	require.NotNil(t, decoded.MusicPolicy)
	require.Equal(t, HolidayBehaviorDelay, decoded.HolidayBehavior)
	require.Equal(t, "America/Los_Angeles", decoded.Timezone)
	require.NotNil(t, decoded.Enabled)
	require.True(t, *decoded.Enabled)
}

func TestCreateRoutineInputJSONMinimal(t *testing.T) {
	input := CreateRoutineInputAPI{
		Name:     "Simple Routine",
		SceneID:  "scene-123",
		Schedule: Schedule{Type: ScheduleTypeCron},
		Timezone: "UTC",
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "Simple Routine", m["name"])
	require.Equal(t, "scene-123", m["scene_id"])
	_, hasDescription := m["description"]
	require.False(t, hasDescription)
	_, hasMusicPolicy := m["music_policy"]
	require.False(t, hasMusicPolicy)
	_, hasEnabled := m["enabled"]
	require.False(t, hasEnabled)
}

func TestUpdateRoutineInputJSON(t *testing.T) {
	name := "Updated Name"
	description := "Updated description"
	enabled := false
	behavior := HolidayBehaviorDelay
	cronExpr := "0 9 * * *"

	input := UpdateRoutineInputAPI{
		Name:            &name,
		Description:     &description,
		Enabled:         &enabled,
		HolidayBehavior: &behavior,
		Schedule: &Schedule{
			Type:     ScheduleTypeCron,
			CronExpr: &cronExpr,
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded UpdateRoutineInputAPI
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.Name)
	require.Equal(t, "Updated Name", *decoded.Name)
	require.NotNil(t, decoded.Description)
	require.Equal(t, "Updated description", *decoded.Description)
	require.NotNil(t, decoded.Enabled)
	require.False(t, *decoded.Enabled)
	require.NotNil(t, decoded.HolidayBehavior)
	require.Equal(t, HolidayBehaviorDelay, *decoded.HolidayBehavior)
	require.NotNil(t, decoded.Schedule)
	require.Equal(t, ScheduleTypeCron, decoded.Schedule.Type)
}

func TestUpdateRoutineInputJSONPartial(t *testing.T) {
	enabled := true
	input := UpdateRoutineInput{
		Enabled: &enabled,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasEnabled := m["enabled"]
	require.True(t, hasEnabled)
	_, hasName := m["name"]
	require.False(t, hasName)
	_, hasDescription := m["description"]
	require.False(t, hasDescription)
	_, hasSceneID := m["scene_id"]
	require.False(t, hasSceneID)
}

func TestCreateJobInputJSON(t *testing.T) {
	scheduledFor := time.Now().UTC().Truncate(time.Second).Add(time.Hour)

	input := CreateJobInput{
		RoutineID:    "routine-123",
		ScheduledFor: scheduledFor,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded CreateJobInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "routine-123", decoded.RoutineID)
	require.Equal(t, scheduledFor, decoded.ScheduledFor)
}

func TestCreateHolidayInputJSON(t *testing.T) {
	holidayDate := time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)

	input := CreateHolidayInputAPI{
		Name:      "Christmas",
		Date:      holidayDate,
		Recurring: true,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded CreateHolidayInputAPI
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "Christmas", decoded.Name)
	require.Equal(t, holidayDate, decoded.Date)
	require.True(t, decoded.Recurring)
}

func TestJobStatusValues(t *testing.T) {
	statuses := []JobStatus{
		JobStatusScheduled,
		JobStatusClaimed,
		JobStatusRunning,
		JobStatusCompleted,
		JobStatusFailed,
		JobStatusSkipped,
	}

	expectedValues := []string{
		"SCHEDULED",
		"CLAIMED",
		"RUNNING",
		"COMPLETED",
		"FAILED",
		"SKIPPED",
	}

	for i, status := range statuses {
		require.Equal(t, expectedValues[i], string(status))
	}
}

func TestHolidayBehaviorValues(t *testing.T) {
	behaviors := []HolidayBehavior{
		HolidayBehaviorSkip,
		HolidayBehaviorDelay,
	}

	expectedValues := []string{
		"SKIP",
		"DELAY",
	}

	for i, behavior := range behaviors {
		require.Equal(t, expectedValues[i], string(behavior))
	}
}

func TestScheduleTypeValues(t *testing.T) {
	types := []ScheduleType{
		ScheduleTypeCron,
		ScheduleTypeInterval,
		ScheduleTypeOneTime,
	}

	expectedValues := []string{
		"CRON",
		"INTERVAL",
		"ONE_TIME",
	}

	for i, schedType := range types {
		require.Equal(t, expectedValues[i], string(schedType))
	}
}
