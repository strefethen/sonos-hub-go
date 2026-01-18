package scheduler

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// MaxDelayIterations is the maximum number of days to search for a non-holiday date.
const MaxDelayIterations = 30

// JobGenerator handles the generation of jobs for routines.
type JobGenerator struct {
	routinesRepo *RoutinesRepository
	jobsRepo     *JobsRepository
	holidaysRepo *HolidaysRepository
	logger       *log.Logger
}

// NewJobGenerator creates a new JobGenerator.
func NewJobGenerator(routinesRepo *RoutinesRepository, jobsRepo *JobsRepository,
	holidaysRepo *HolidaysRepository, logger *log.Logger) *JobGenerator {
	return &JobGenerator{
		routinesRepo: routinesRepo,
		jobsRepo:     jobsRepo,
		holidaysRepo: holidaysRepo,
		logger:       logger,
	}
}

// CalculateNextRun calculates the next run time for a routine.
// Handles CRON, INTERVAL, ONE_TIME, weekly, monthly, and yearly schedule types.
// Uses the routine's timezone for calculations.
func (g *JobGenerator) CalculateNextRun(routine *Routine, after time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(routine.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", routine.Timezone, err)
	}

	afterLocal := after.In(loc)

	switch routine.ScheduleType {
	case ScheduleTypeCron:
		return g.calculateCronNextRun(routine, afterLocal, loc)
	case ScheduleTypeInterval:
		return g.calculateIntervalNextRun(routine, afterLocal, loc)
	case ScheduleTypeOneTime, ScheduleTypeOnce:
		return g.calculateOneTimeNextRun(routine, afterLocal, loc)
	case ScheduleTypeWeekly:
		return g.calculateWeeklyNextRun(routine, afterLocal, loc)
	case ScheduleTypeMonthly:
		return g.calculateMonthlyNextRun(routine, afterLocal, loc)
	case ScheduleTypeYearly:
		return g.calculateYearlyNextRun(routine, afterLocal, loc)
	default:
		return time.Time{}, fmt.Errorf("unsupported schedule type: %s", routine.ScheduleType)
	}
}

func (g *JobGenerator) calculateCronNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	// For now, cron expressions are not stored in the current schema
	// This is a placeholder for future CRON support
	return time.Time{}, errors.New("CRON schedule type requires cron expression support")
}

func (g *JobGenerator) calculateIntervalNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	// For now, interval schedules are not stored in the current schema
	// This is a placeholder for future INTERVAL support
	return time.Time{}, errors.New("INTERVAL schedule type requires interval_minutes support")
}

func (g *JobGenerator) calculateOneTimeNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	// For one-time schedules, check if ScheduleDay and ScheduleMonth are set
	if routine.ScheduleMonth == nil || routine.ScheduleDay == nil {
		return time.Time{}, errors.New("schedule_month and schedule_day are required for one-time schedule type")
	}

	hour, minute, err := parseScheduleTime(routine.ScheduleTime)
	if err != nil {
		return time.Time{}, err
	}

	// Construct the one-time run date
	runAt := time.Date(after.Year(), time.Month(*routine.ScheduleMonth), *routine.ScheduleDay, hour, minute, 0, 0, loc)

	// If the date is in the past, return zero time (already executed)
	if runAt.Before(after) || runAt.Equal(after) {
		return time.Time{}, nil
	}

	return runAt, nil
}

func (g *JobGenerator) calculateWeeklyNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	if len(routine.ScheduleWeekdays) == 0 {
		return time.Time{}, errors.New("schedule_weekdays is required for weekly schedule type")
	}

	// Parse time from schedule_time (format: "HH:MM" or "HH:MM:SS")
	hour, minute, err := parseScheduleTime(routine.ScheduleTime)
	if err != nil {
		return time.Time{}, err
	}

	// Convert []int to []time.Weekday
	weekdays := make([]time.Weekday, 0, len(routine.ScheduleWeekdays))
	for _, d := range routine.ScheduleWeekdays {
		if d >= 0 && d <= 6 {
			weekdays = append(weekdays, time.Weekday(d))
		}
	}

	if len(weekdays) == 0 {
		return time.Time{}, errors.New("no valid weekdays specified")
	}

	// Start from the day after 'after' and check each day
	candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, loc)

	// If today's scheduled time is still in the future and today is a valid weekday, use it
	if candidate.After(after) && containsWeekday(weekdays, candidate.Weekday()) {
		return candidate, nil
	}

	// Otherwise, find the next valid weekday
	for i := 1; i <= 7; i++ {
		candidate = candidate.AddDate(0, 0, 1)
		if containsWeekday(weekdays, candidate.Weekday()) {
			return candidate, nil
		}
	}

	return time.Time{}, errors.New("unable to find next run time")
}

func (g *JobGenerator) calculateMonthlyNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	if routine.ScheduleDay == nil {
		return time.Time{}, errors.New("schedule_day is required for monthly schedule type")
	}

	hour, minute, err := parseScheduleTime(routine.ScheduleTime)
	if err != nil {
		return time.Time{}, err
	}

	day := *routine.ScheduleDay

	// Try this month
	candidate := time.Date(after.Year(), after.Month(), day, hour, minute, 0, 0, loc)
	if candidate.After(after) && candidate.Day() == day {
		return candidate, nil
	}

	// Try next month
	candidate = time.Date(after.Year(), after.Month()+1, day, hour, minute, 0, 0, loc)
	// Handle day overflow (e.g., Feb 30 -> Mar 2)
	for candidate.Day() != day && candidate.Day() < day {
		candidate = candidate.AddDate(0, 1, 0)
	}

	return candidate, nil
}

func (g *JobGenerator) calculateYearlyNextRun(routine *Routine, after time.Time, loc *time.Location) (time.Time, error) {
	if routine.ScheduleMonth == nil || routine.ScheduleDay == nil {
		return time.Time{}, errors.New("schedule_month and schedule_day are required for yearly schedule type")
	}

	hour, minute, err := parseScheduleTime(routine.ScheduleTime)
	if err != nil {
		return time.Time{}, err
	}

	month := time.Month(*routine.ScheduleMonth)
	day := *routine.ScheduleDay

	// Try this year
	candidate := time.Date(after.Year(), month, day, hour, minute, 0, 0, loc)
	if candidate.After(after) {
		return candidate, nil
	}

	// Try next year
	candidate = time.Date(after.Year()+1, month, day, hour, minute, 0, 0, loc)
	return candidate, nil
}

// GenerateJobs creates jobs for all due routines.
// Returns the number of jobs created.
func (g *JobGenerator) GenerateJobs(now time.Time) (int, error) {
	routines, err := g.routinesRepo.GetDueRoutines(now)
	if err != nil {
		return 0, fmt.Errorf("failed to get due routines: %w", err)
	}

	count := 0
	for i := range routines {
		routine := &routines[i]
		job, err := g.GenerateJobForRoutine(routine, now)
		if err != nil {
			if g.logger != nil {
				g.logger.Printf("Error generating job for routine %s: %v", routine.RoutineID, err)
			}
			continue
		}
		if job != nil {
			count++
		}
	}

	return count, nil
}

// GenerateJobForRoutine creates a job for a specific routine if due.
// Handles holiday behavior (SKIP or DELAY).
func (g *JobGenerator) GenerateJobForRoutine(routine *Routine, now time.Time) (*Job, error) {
	if !routine.Enabled {
		return nil, nil
	}

	// Check if snoozed
	if routine.SnoozeUntil != nil && now.Before(*routine.SnoozeUntil) {
		return nil, nil
	}

	// Check if skip_next is set
	if routine.SkipNext {
		return nil, nil
	}

	// Calculate next run time
	nextRun, err := g.CalculateNextRun(routine, now)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate next run: %w", err)
	}

	// If no next run time (e.g., one-time already run), return nil
	if nextRun.IsZero() {
		return nil, nil
	}

	// Apply holiday behavior
	scheduledFor, err := g.ApplyHolidayBehavior(routine, nextRun)
	if err != nil {
		return nil, fmt.Errorf("failed to apply holiday behavior: %w", err)
	}

	// If nil, routine should be skipped (holiday)
	if scheduledFor == nil {
		return nil, nil
	}

	// Generate idempotency key
	scheduledForStr := scheduledFor.UTC().Format(time.RFC3339)
	idempotencyKey := fmt.Sprintf("%s:%s", routine.RoutineID, scheduledForStr)

	// Create the job using the repository
	input := CreateJobInput{
		RoutineID:      routine.RoutineID,
		ScheduledFor:   *scheduledFor,
		IdempotencyKey: &idempotencyKey,
	}

	job, err := g.jobsRepo.CreateWithInput(input)
	if err != nil {
		// Check if it's a duplicate (idempotency key conflict)
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// ApplyHolidayBehavior adjusts the scheduled time based on holiday behavior.
// SKIP: Returns nil (no job created)
// DELAY: Finds next non-holiday date
// RUN: Returns the original scheduled time
func (g *JobGenerator) ApplyHolidayBehavior(routine *Routine, scheduledFor time.Time) (*time.Time, error) {
	isHoliday, _, err := g.holidaysRepo.IsHolidayWithDetails(scheduledFor)
	if err != nil {
		return nil, fmt.Errorf("failed to check holiday: %w", err)
	}

	if !isHoliday {
		return &scheduledFor, nil
	}

	switch routine.HolidayBehavior {
	case HolidayBehaviorSkip:
		return nil, nil
	case HolidayBehaviorRun:
		return &scheduledFor, nil
	case HolidayBehaviorDelay:
		return g.findNextNonHoliday(scheduledFor)
	default:
		// Default to SKIP behavior
		return nil, nil
	}
}

func (g *JobGenerator) findNextNonHoliday(from time.Time) (*time.Time, error) {
	candidate := from

	for i := 0; i < MaxDelayIterations; i++ {
		candidate = candidate.AddDate(0, 0, 1)

		isHoliday, _, err := g.holidaysRepo.IsHolidayWithDetails(candidate)
		if err != nil {
			return nil, err
		}

		if !isHoliday {
			return &candidate, nil
		}
	}

	return nil, fmt.Errorf("could not find non-holiday date within %d days", MaxDelayIterations)
}

// Helper functions

func parseScheduleTime(timeStr string) (hour, minute int, err error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	_, err = fmt.Sscanf(parts[0], "%d", &hour)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour: %w", err)
	}

	_, err = fmt.Sscanf(parts[1], "%d", &minute)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute: %w", err)
	}

	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour out of range: %d", hour)
	}
	if minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute out of range: %d", minute)
	}

	return hour, minute, nil
}

func containsWeekday(weekdays []time.Weekday, day time.Weekday) bool {
	for _, w := range weekdays {
		if w == day {
			return true
		}
	}
	return false
}

// CronScheduleParser provides CRON expression parsing using robfig/cron.
// This can be used when CRON support is added to the schema.
type CronScheduleParser struct{}

// ParseCron parses a cron expression and returns the next time after 'after'.
func (p *CronScheduleParser) ParseCron(expression string, after time.Time, loc *time.Location) (time.Time, error) {
	// Use standard cron parser (5 fields: minute, hour, day-of-month, month, day-of-week)
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(expression)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}

	next := schedule.Next(after.In(loc))
	return next, nil
}
