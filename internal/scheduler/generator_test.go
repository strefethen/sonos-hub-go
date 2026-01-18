package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
)

func setupTestGeneratorDB(t *testing.T) (*JobGenerator, *RoutinesRepository, *JobsRepository, *HolidaysRepository, *db.DBPair) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	routinesRepo := NewRoutinesRepository(dbPair)
	jobsRepo := NewJobsRepository(dbPair)
	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(routinesRepo, jobsRepo, holidaysRepo, nil)

	return generator, routinesRepo, jobsRepo, holidaysRepo, dbPair
}

// Test weekly schedule calculation
func TestCalculateNextRun_Weekly(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test-weekly",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5}, // Mon-Fri
		ScheduleTime:     "09:00",
		Timezone:         "America/Los_Angeles",
		Enabled:          true,
	}

	loc, _ := time.LoadLocation("America/Los_Angeles")
	// Monday Jan 15, 2024 at 8:00 AM
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, loc)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.Monday, nextRun.Weekday())
	require.Equal(t, 9, nextRun.Hour())
	require.Equal(t, 0, nextRun.Minute())
}

func TestCalculateNextRun_WeeklySkipToNextDay(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test-weekly",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 3, 5}, // Mon, Wed, Fri
		ScheduleTime:     "09:00",
		Timezone:         "America/Los_Angeles",
		Enabled:          true,
	}

	loc, _ := time.LoadLocation("America/Los_Angeles")
	// Monday Jan 15, 2024 at 10:00 AM (after scheduled time)
	after := time.Date(2024, 1, 15, 10, 0, 0, 0, loc)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.Wednesday, nextRun.Weekday())
	require.Equal(t, 17, nextRun.Day())
}

func TestCalculateNextRun_WeeklyWeekend(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test-weekend",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{0, 6}, // Sunday, Saturday
		ScheduleTime:     "10:00",
		Timezone:         "UTC",
		Enabled:          true,
	}

	// Wednesday Jan 17, 2024
	after := time.Date(2024, 1, 17, 10, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.Saturday, nextRun.Weekday())
	require.Equal(t, 20, nextRun.Day())
}

// Test monthly schedule calculation
func TestCalculateNextRun_Monthly(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	day := 15
	routine := &Routine{
		RoutineID:    "test-monthly",
		ScheduleType: ScheduleTypeMonthly,
		ScheduleDay:  &day,
		ScheduleTime: "10:00",
		Timezone:     "UTC",
		Enabled:      true,
	}

	// Jan 10, 2024
	after := time.Date(2024, 1, 10, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, 15, nextRun.Day())
	require.Equal(t, time.January, nextRun.Month())
}

func TestCalculateNextRun_MonthlyNextMonth(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	day := 15
	routine := &Routine{
		RoutineID:    "test-monthly",
		ScheduleType: ScheduleTypeMonthly,
		ScheduleDay:  &day,
		ScheduleTime: "10:00",
		Timezone:     "UTC",
		Enabled:      true,
	}

	// Jan 20, 2024 (after the 15th)
	after := time.Date(2024, 1, 20, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, 15, nextRun.Day())
	require.Equal(t, time.February, nextRun.Month())
}

// Test yearly schedule calculation
func TestCalculateNextRun_Yearly(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	month := 7 // July
	day := 4
	routine := &Routine{
		RoutineID:     "test-yearly",
		ScheduleType:  ScheduleTypeYearly,
		ScheduleMonth: &month,
		ScheduleDay:   &day,
		ScheduleTime:  "12:00",
		Timezone:      "UTC",
		Enabled:       true,
	}

	// Jan 15, 2024
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.July, nextRun.Month())
	require.Equal(t, 4, nextRun.Day())
	require.Equal(t, 2024, nextRun.Year())
}

func TestCalculateNextRun_YearlyNextYear(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	month := 1 // January
	day := 1
	routine := &Routine{
		RoutineID:     "test-yearly",
		ScheduleType:  ScheduleTypeYearly,
		ScheduleMonth: &month,
		ScheduleDay:   &day,
		ScheduleTime:  "00:00",
		Timezone:      "UTC",
		Enabled:       true,
	}

	// Feb 15, 2024 (after Jan 1)
	after := time.Date(2024, 2, 15, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.January, nextRun.Month())
	require.Equal(t, 1, nextRun.Day())
	require.Equal(t, 2025, nextRun.Year())
}

// Test one-time (once) schedule calculation
func TestCalculateNextRun_Once(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	month := 6 // June
	day := 15
	routine := &Routine{
		RoutineID:     "test-once",
		ScheduleType:  ScheduleTypeOnce,
		ScheduleMonth: &month,
		ScheduleDay:   &day,
		ScheduleTime:  "14:00",
		Timezone:      "UTC",
		Enabled:       true,
	}

	// Jan 15, 2024
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.Equal(t, time.June, nextRun.Month())
	require.Equal(t, 15, nextRun.Day())
	require.Equal(t, 14, nextRun.Hour())
}

func TestCalculateNextRun_OnceAlreadyPassed(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	month := 1 // January
	day := 10
	routine := &Routine{
		RoutineID:     "test-once",
		ScheduleType:  ScheduleTypeOnce,
		ScheduleMonth: &month,
		ScheduleDay:   &day,
		ScheduleTime:  "14:00",
		Timezone:      "UTC",
		Enabled:       true,
	}

	// Jan 15, 2024 (after Jan 10)
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)
	require.True(t, nextRun.IsZero())
}

// Test timezone handling
func TestCalculateNextRun_Timezone(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test-tz",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "09:00",
		Timezone:         "America/New_York",
		Enabled:          true,
	}

	// Use UTC time, expect result in New York time
	after := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC) // 7 AM in NY

	nextRun, err := generator.CalculateNextRun(routine, after)
	require.NoError(t, err)

	nyLoc, _ := time.LoadLocation("America/New_York")
	nextRunNY := nextRun.In(nyLoc)
	require.Equal(t, 9, nextRunNY.Hour())
	require.Equal(t, 0, nextRunNY.Minute())
}

func TestCalculateNextRun_DifferentTimezones(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	testCases := []struct {
		name     string
		timezone string
	}{
		{"Los Angeles", "America/Los_Angeles"},
		{"New York", "America/New_York"},
		{"London", "Europe/London"},
		{"Tokyo", "Asia/Tokyo"},
		{"UTC", "UTC"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			routine := &Routine{
				RoutineID:        "test-tz-" + tc.name,
				ScheduleType:     ScheduleTypeWeekly,
				ScheduleWeekdays: []int{1, 2, 3, 4, 5},
				ScheduleTime:     "09:00",
				Timezone:         tc.timezone,
				Enabled:          true,
			}

			loc, err := time.LoadLocation(tc.timezone)
			require.NoError(t, err)

			// Monday at 8 AM in the target timezone
			after := time.Date(2024, 1, 15, 8, 0, 0, 0, loc)

			nextRun, err := generator.CalculateNextRun(routine, after)
			require.NoError(t, err)

			nextRunLocal := nextRun.In(loc)
			require.Equal(t, 9, nextRunLocal.Hour())
			require.Equal(t, 0, nextRunLocal.Minute())
		})
	}
}

// Test holiday SKIP behavior
func TestApplyHolidayBehavior_Skip(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(nil, nil, holidaysRepo, nil)

	// Add a holiday
	_, err = holidaysRepo.Create(CreateHolidayInput{
		Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Name:     "Test Holiday",
		IsCustom: false,
	})
	require.NoError(t, err)

	routine := &Routine{
		HolidayBehavior: HolidayBehaviorSkip,
	}

	scheduledFor := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	result, err := generator.ApplyHolidayBehavior(routine, scheduledFor)
	require.NoError(t, err)
	require.Nil(t, result) // Should be nil for SKIP
}

func TestApplyHolidayBehavior_SkipNonHoliday(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(nil, nil, holidaysRepo, nil)

	routine := &Routine{
		HolidayBehavior: HolidayBehaviorSkip,
	}

	scheduledFor := time.Date(2024, 1, 16, 9, 0, 0, 0, time.UTC)

	result, err := generator.ApplyHolidayBehavior(routine, scheduledFor)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, scheduledFor, *result)
}

// Test holiday DELAY behavior
func TestApplyHolidayBehavior_Delay(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(nil, nil, holidaysRepo, nil)

	// Add a holiday
	_, err = holidaysRepo.Create(CreateHolidayInput{
		Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Name:     "Test Holiday",
		IsCustom: false,
	})
	require.NoError(t, err)

	routine := &Routine{
		HolidayBehavior: HolidayBehaviorDelay,
	}

	scheduledFor := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	result, err := generator.ApplyHolidayBehavior(routine, scheduledFor)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 16, result.Day()) // Should be delayed to Jan 16
}

func TestApplyHolidayBehavior_DelayMultipleDays(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(nil, nil, holidaysRepo, nil)

	// Add multiple consecutive holidays
	for i := 15; i <= 17; i++ {
		_, err = holidaysRepo.Create(CreateHolidayInput{
			Date:     time.Date(2024, 1, i, 0, 0, 0, 0, time.UTC),
			Name:     "Holiday " + string(rune('0'+i-14)),
			IsCustom: false,
		})
		require.NoError(t, err)
	}

	routine := &Routine{
		HolidayBehavior: HolidayBehaviorDelay,
	}

	scheduledFor := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	result, err := generator.ApplyHolidayBehavior(routine, scheduledFor)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 18, result.Day()) // Should skip to Jan 18
}

// Test holiday RUN behavior
func TestApplyHolidayBehavior_Run(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(nil, nil, holidaysRepo, nil)

	// Add a holiday
	_, err = holidaysRepo.Create(CreateHolidayInput{
		Date:     time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Name:     "Test Holiday",
		IsCustom: false,
	})
	require.NoError(t, err)

	routine := &Routine{
		HolidayBehavior: HolidayBehaviorRun,
	}

	scheduledFor := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	result, err := generator.ApplyHolidayBehavior(routine, scheduledFor)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, scheduledFor, *result) // Should run even on holiday
}

// Test GenerateJobForRoutine
func TestGenerateJobForRoutine_DisabledRoutine(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID: "test",
		Enabled:   false,
	}

	job, err := generator.GenerateJobForRoutine(routine, time.Now())
	require.NoError(t, err)
	require.Nil(t, job)
}

func TestGenerateJobForRoutine_Snoozed(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	snoozeUntil := time.Now().Add(1 * time.Hour)
	routine := &Routine{
		RoutineID:        "test",
		Enabled:          true,
		SnoozeUntil:      &snoozeUntil,
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "09:00",
		Timezone:         "UTC",
	}

	job, err := generator.GenerateJobForRoutine(routine, time.Now())
	require.NoError(t, err)
	require.Nil(t, job)
}

func TestGenerateJobForRoutine_SkipNext(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test",
		Enabled:          true,
		SkipNext:         true,
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "09:00",
		Timezone:         "UTC",
	}

	job, err := generator.GenerateJobForRoutine(routine, time.Now())
	require.NoError(t, err)
	require.Nil(t, job)
}

// Test HolidaysRepository IsHoliday method
func TestGenerator_HolidaysRepository_IsHoliday(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	repo := NewHolidaysRepository(dbPair)

	// Add a holiday
	_, err = repo.Create(CreateHolidayInput{
		Date:     time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC),
		Name:     "Christmas",
		IsCustom: false,
	})
	require.NoError(t, err)

	// Test holiday
	christmas := time.Date(2024, 12, 25, 12, 0, 0, 0, time.UTC)
	isHoliday, holiday, err := repo.IsHolidayWithDetails(christmas)
	require.NoError(t, err)
	require.True(t, isHoliday)
	require.NotNil(t, holiday)
	require.Equal(t, "Christmas", holiday.Name)

	// Test non-holiday
	normalDay := time.Date(2024, 12, 26, 12, 0, 0, 0, time.UTC)
	isHoliday, holiday, err = repo.IsHolidayWithDetails(normalDay)
	require.NoError(t, err)
	require.False(t, isHoliday)
	require.Nil(t, holiday)
}

// Test JobsRepository integration
func TestJobsRepository_CreateAndRetrieve(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	// Create a scene and routine first
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = dbPair.Writer().Exec(`INSERT INTO scenes (scene_id, name, members, created_at, updated_at) VALUES ('scene-1', 'Test', '[]', ?, ?)`, now, now)
	require.NoError(t, err)
	_, err = dbPair.Writer().Exec(`INSERT INTO routines (routine_id, name, enabled, timezone, schedule_type, schedule_time, holiday_behavior, scene_id, created_at, updated_at)
		VALUES ('routine-1', 'Test', 1, 'UTC', 'weekly', '09:00', 'SKIP', 'scene-1', ?, ?)`, now, now)
	require.NoError(t, err)

	repo := NewJobsRepository(dbPair)

	scheduledFor := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)
	idempotencyKey := "routine-1:2024-01-15T09:00:00Z"
	job, err := repo.CreateWithInput(CreateJobInput{
		RoutineID:      "routine-1",
		ScheduledFor:   scheduledFor,
		IdempotencyKey: &idempotencyKey,
	})
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotEmpty(t, job.JobID)
	require.Equal(t, "routine-1", job.RoutineID)
	require.Equal(t, JobStatusPending, job.Status)
	require.Equal(t, 0, job.Attempts)

	// Retrieve by ID
	retrieved, err := repo.GetByID(job.JobID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, job.JobID, retrieved.JobID)
}

// Test parseScheduleTime
func TestParseScheduleTime(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectHour  int
		expectMin   int
		expectError bool
	}{
		{"standard", "09:00", 9, 0, false},
		{"with seconds", "14:30:00", 14, 30, false},
		{"midnight", "00:00", 0, 0, false},
		{"end of day", "23:59", 23, 59, false},
		{"invalid format", "9", 0, 0, true},
		{"invalid hour", "25:00", 0, 0, true},
		{"invalid minute", "09:60", 0, 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hour, minute, err := parseScheduleTime(tc.input)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectHour, hour)
				require.Equal(t, tc.expectMin, minute)
			}
		})
	}
}

// Test containsWeekday
func TestContainsWeekday(t *testing.T) {
	weekdays := []time.Weekday{time.Monday, time.Wednesday, time.Friday}

	require.True(t, containsWeekday(weekdays, time.Monday))
	require.True(t, containsWeekday(weekdays, time.Wednesday))
	require.True(t, containsWeekday(weekdays, time.Friday))
	require.False(t, containsWeekday(weekdays, time.Tuesday))
	require.False(t, containsWeekday(weekdays, time.Saturday))
	require.False(t, containsWeekday(weekdays, time.Sunday))
}

// Test error cases
func TestCalculateNextRun_InvalidTimezone(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:        "test",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "09:00",
		Timezone:         "Invalid/Timezone",
		Enabled:          true,
	}

	_, err := generator.CalculateNextRun(routine, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid timezone")
}

func TestCalculateNextRun_MissingWeekdays(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:    "test",
		ScheduleType: ScheduleTypeWeekly,
		ScheduleTime: "09:00",
		Timezone:     "UTC",
		Enabled:      true,
	}

	_, err := generator.CalculateNextRun(routine, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "schedule_weekdays is required")
}

func TestCalculateNextRun_MissingScheduleDay(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:    "test",
		ScheduleType: ScheduleTypeMonthly,
		ScheduleTime: "09:00",
		Timezone:     "UTC",
		Enabled:      true,
	}

	_, err := generator.CalculateNextRun(routine, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "schedule_day is required")
}

func TestCalculateNextRun_UnsupportedScheduleType(t *testing.T) {
	generator, _, _, _, _ := setupTestGeneratorDB(t)

	routine := &Routine{
		RoutineID:    "test",
		ScheduleType: "UNKNOWN",
		Timezone:     "UTC",
		Enabled:      true,
	}

	_, err := generator.CalculateNextRun(routine, time.Now())
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported schedule type")
}

// Test CronScheduleParser
func TestCronScheduleParser_ParseCron(t *testing.T) {
	parser := &CronScheduleParser{}

	// Test daily at 9 AM
	after := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	next, err := parser.ParseCron("0 9 * * *", after, time.UTC)
	require.NoError(t, err)
	require.Equal(t, 9, next.Hour())
	require.Equal(t, 0, next.Minute())
	require.Equal(t, 15, next.Day())
}

func TestCronScheduleParser_ParseCronWeekdays(t *testing.T) {
	parser := &CronScheduleParser{}

	// Test Mon-Fri at 9 AM
	// Friday Jan 12, 2024 at 10 AM
	after := time.Date(2024, 1, 12, 10, 0, 0, 0, time.UTC)
	next, err := parser.ParseCron("0 9 * * 1-5", after, time.UTC)
	require.NoError(t, err)
	// Should be Monday Jan 15
	require.Equal(t, time.Monday, next.Weekday())
	require.Equal(t, 15, next.Day())
}

func TestCronScheduleParser_InvalidCron(t *testing.T) {
	parser := &CronScheduleParser{}

	_, err := parser.ParseCron("invalid", time.Now(), time.UTC)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid cron expression")
}

// Test GenerateJobs integration
func TestGenerateJobs_Integration(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	defer dbPair.Close()

	routinesRepo := NewRoutinesRepository(dbPair)
	jobsRepo := NewJobsRepository(dbPair)
	holidaysRepo := NewHolidaysRepository(dbPair)
	generator := NewJobGenerator(routinesRepo, jobsRepo, holidaysRepo, nil)

	// Create a scene
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = dbPair.Writer().Exec(`INSERT INTO scenes (scene_id, name, members, created_at, updated_at) VALUES ('scene-1', 'Test', '[]', ?, ?)`, now, now)
	require.NoError(t, err)

	// Create a routine
	routine, err := routinesRepo.Create(CreateRoutineInput{
		Name:             "Test Routine",
		Timezone:         "UTC",
		ScheduleType:     ScheduleTypeWeekly,
		ScheduleWeekdays: []int{1, 2, 3, 4, 5},
		ScheduleTime:     "09:00",
		HolidayBehavior:  HolidayBehaviorSkip,
		SceneID:          "scene-1",
	})
	require.NoError(t, err)
	require.NotNil(t, routine)

	// Generate jobs for a Monday morning
	testTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC) // Monday at 8 AM
	count, err := generator.GenerateJobs(testTime)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Verify job was created
	jobs, total, err := jobsRepo.ListByRoutineID(routine.RoutineID, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, jobs, 1)
	require.Equal(t, routine.RoutineID, jobs[0].RoutineID)
}
