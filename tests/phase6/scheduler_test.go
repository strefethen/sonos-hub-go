package phase6

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/server"
)

// ==========================================================================
// Response Types
// ==========================================================================

type routineResponse struct {
	RequestID string         `json:"request_id"`
	Routine   map[string]any `json:"routine"`
}

type listRoutinesResponse struct {
	RequestID  string           `json:"request_id"`
	Routines   []map[string]any `json:"routines"`
	Pagination map[string]any   `json:"pagination"`
}

type jobResponse struct {
	RequestID string         `json:"request_id"`
	Job       map[string]any `json:"job"`
}

type listJobsResponse struct {
	RequestID  string           `json:"request_id"`
	Jobs       []map[string]any `json:"jobs"`
	Pagination map[string]any   `json:"pagination"`
}

type holidayResponse struct {
	RequestID string         `json:"request_id"`
	Holiday   map[string]any `json:"holiday"`
}

type listHolidaysResponse struct {
	RequestID  string           `json:"request_id"`
	Holidays   []map[string]any `json:"holidays"`
	Pagination map[string]any   `json:"pagination"`
}

type holidayCheckResponse struct {
	RequestID    string         `json:"request_id"`
	HolidayCheck map[string]any `json:"holiday_check"`
}

type errorResponse struct {
	RequestID string         `json:"request_id"`
	Error     map[string]any `json:"error"`
}

// ==========================================================================
// Test Helpers
// ==========================================================================

func setupSchedulerTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	t.Setenv("JWT_SECRET", "this-is-a-development-secret-string-32chars")
	t.Setenv("NODE_ENV", "development")
	t.Setenv("ALLOW_TEST_MODE", "true")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sonos-hub.db")
	t.Setenv("SQLITE_DB_PATH", dbPath)

	cfg, err := config.Load()
	require.NoError(t, err)

	handler, shutdown, err := server.NewHandler(cfg, server.Options{DisableDiscovery: true})
	require.NoError(t, err)

	ts := httptest.NewServer(handler)

	return ts, func() {
		ts.Close()
		require.NoError(t, shutdown(nil))
	}
}

func doSchedulerRequest(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		buf = bytes.NewBuffer(payload)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// createTestScene creates a scene and returns its ID for use in routine tests
func createTestScene(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	createPayload := map[string]any{
		"name":        "Test Scene for Scheduler",
		"description": "A test scene for scheduler tests",
		"members":     []map[string]any{},
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	return createResp.Scene["scene_id"].(string)
}

// ==========================================================================
// Routine CRUD Tests
// ==========================================================================

func TestRoutineCRUD(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Create a scene first (routines require a valid scene_id)
	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Morning Alarm",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5}, // Mon-Fri
		"schedule_time":     "07:30",
		"holiday_behavior":  "SKIP",
		"enabled":           true,
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.NotEmpty(t, createResp.RequestID)
	require.NotEmpty(t, createResp.Routine["routine_id"])
	require.Equal(t, "Morning Alarm", createResp.Routine["name"])
	require.Equal(t, sceneID, createResp.Routine["scene_id"])
	require.Equal(t, "America/New_York", createResp.Routine["timezone"])
	require.Equal(t, "SKIP", createResp.Routine["holiday_behavior"])
	require.Equal(t, true, createResp.Routine["enabled"])

	// Check nested schedule object
	schedule := createResp.Routine["schedule"].(map[string]any)
	require.Equal(t, "weekly", schedule["type"])
	require.Equal(t, "07:30", schedule["time"])

	routineID := createResp.Routine["routine_id"].(string)

	// Get routine
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/"+routineID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getResp))
	resp.Body.Close()

	require.Equal(t, routineID, getResp.Routine["routine_id"])
	require.Equal(t, "Morning Alarm", getResp.Routine["name"])

	// Update routine
	updatePayload := map[string]any{
		"name":          "Evening Routine",
		"schedule_time": "18:00",
	}
	resp = doSchedulerRequest(t, http.MethodPut, ts.URL+"/v1/routines/"+routineID, updatePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updateResp))
	resp.Body.Close()

	require.Equal(t, "Evening Routine", updateResp.Routine["name"])
	// Check nested schedule time
	updatedSchedule := updateResp.Routine["schedule"].(map[string]any)
	require.Equal(t, "18:00", updatedSchedule["time"])
	// Other fields should be preserved
	require.Equal(t, sceneID, updateResp.Routine["scene_id"])

	// Delete routine
	resp = doSchedulerRequest(t, http.MethodDelete, ts.URL+"/v1/routines/"+routineID, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify deleted
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/"+routineID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestListRoutinesWithPagination(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create 5 routines
	for i := 0; i < 5; i++ {
		createPayload := map[string]any{
			"name":              "Routine " + string(rune('A'+i)),
			"scene_id":          sceneID,
			"timezone":          "America/New_York",
			"schedule_type":     "weekly",
			"schedule_weekdays": []int{1, 2, 3, 4, 5},
			"schedule_time":     "07:30",
		}
		resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// List with limit
	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines?limit=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listRoutinesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Routines, 3)
	require.Equal(t, 5, int(listResp.Pagination["total"].(float64)))
	require.True(t, listResp.Pagination["has_more"].(bool))

	// List with offset
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines?limit=3&offset=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Routines, 2)
	require.False(t, listResp.Pagination["has_more"].(bool))
}

func TestCreateRoutineWithNonExistentScene(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Try to create routine with non-existent scene_id
	createPayload := map[string]any{
		"name":              "Invalid Routine",
		"scene_id":          "nonexistent-scene-id",
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "SCENE_NOT_FOUND", errResp.Error["code"])
}

// ==========================================================================
// Routine Controls Tests
// ==========================================================================

func TestEnableDisableRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine (enabled by default)
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
		"enabled":           true,
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)
	require.Equal(t, true, createResp.Routine["enabled"])

	// Disable routine
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/disable", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var disableResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&disableResp))
	resp.Body.Close()

	require.Equal(t, false, disableResp.Routine["enabled"])

	// Enable routine
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/enable", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var enableResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&enableResp))
	resp.Body.Close()

	require.Equal(t, true, enableResp.Routine["enabled"])
}

func TestSnoozeUnsnoozeRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)

	// Snooze routine until tomorrow
	snoozeUntil := time.Now().Add(24 * time.Hour).UTC()
	snoozePayload := map[string]any{
		"until": snoozeUntil.Format(time.RFC3339),
	}
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/snooze", snoozePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var snoozeResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&snoozeResp))
	resp.Body.Close()

	require.NotNil(t, snoozeResp.Routine["snooze_until"])

	// Unsnooze routine
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/unsnooze", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var unsnoozeResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&unsnoozeResp))
	resp.Body.Close()

	require.Nil(t, unsnoozeResp.Routine["snooze_until"])
}

func TestSkipNextRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)
	require.Equal(t, false, createResp.Routine["skip_next"])

	// Skip next occurrence
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/skip", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var skipResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&skipResp))
	resp.Body.Close()

	require.Equal(t, true, skipResp.Routine["skip_next"])
}

func TestTriggerRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)

	// Trigger routine (manual execution)
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/trigger", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var triggerResp jobResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&triggerResp))
	resp.Body.Close()

	require.NotEmpty(t, triggerResp.Job["job_id"])
	require.Equal(t, routineID, triggerResp.Job["routine_id"])
	require.Equal(t, "PENDING", triggerResp.Job["status"])
}

// ==========================================================================
// Jobs Tests
// ==========================================================================

func TestGetJobAfterTrigger(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)

	// Trigger routine to create a job
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/trigger", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var triggerResp jobResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&triggerResp))
	resp.Body.Close()

	jobID := triggerResp.Job["job_id"].(string)

	// Get the job
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/jobs/"+jobID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getJobResp jobResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getJobResp))
	resp.Body.Close()

	require.Equal(t, jobID, getJobResp.Job["job_id"])
	require.Equal(t, routineID, getJobResp.Job["routine_id"])
	require.NotEmpty(t, getJobResp.Job["scheduled_for"])
	require.NotEmpty(t, getJobResp.Job["created_at"])
}

func TestListJobsForRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)

	// Trigger routine to create a job
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/trigger", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var triggerResp jobResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&triggerResp))
	resp.Body.Close()

	jobID := triggerResp.Job["job_id"].(string)

	// List jobs for routine
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/"+routineID+"/jobs", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listJobsResp listJobsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listJobsResp))
	resp.Body.Close()

	// Verify that the job we created is in the list
	require.GreaterOrEqual(t, len(listJobsResp.Jobs), 1)
	require.GreaterOrEqual(t, int(listJobsResp.Pagination["total"].(float64)), 1)

	// Verify the job we triggered is in the list
	found := false
	for _, job := range listJobsResp.Jobs {
		if job["job_id"] == jobID {
			found = true
			require.Equal(t, routineID, job["routine_id"])
			break
		}
	}
	require.True(t, found, "triggered job should be in the list")
}

// ==========================================================================
// Holidays Tests
// ==========================================================================

func TestHolidayCRUD(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Create holiday
	createPayload := map[string]any{
		"name":      "Christmas",
		"date":      "2025-12-25",
		"is_custom": false,
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/holidays", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp holidayResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.NotEmpty(t, createResp.RequestID)
	require.Equal(t, "2025-12-25", createResp.Holiday["date"])
	require.Equal(t, "Christmas", createResp.Holiday["name"])
	require.Equal(t, false, createResp.Holiday["is_custom"])

	holidayID := createResp.Holiday["holiday_id"].(string)

	// Get holiday
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays/"+holidayID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp holidayResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getResp))
	resp.Body.Close()

	require.Equal(t, "Christmas", getResp.Holiday["name"])

	// Delete holiday
	resp = doSchedulerRequest(t, http.MethodDelete, ts.URL+"/v1/holidays/"+holidayID, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify deleted
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays/"+holidayID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestListHolidays(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Create multiple holidays
	holidays := []map[string]any{
		{"name": "New Year's Day", "date": "2025-01-01", "is_custom": false},
		{"name": "Independence Day", "date": "2025-07-04", "is_custom": false},
		{"name": "Christmas", "date": "2025-12-25", "is_custom": false},
	}

	for _, holiday := range holidays {
		resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/holidays", holiday)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// List holidays
	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listHolidaysResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Holidays, 3)
	require.Equal(t, 3, int(listResp.Pagination["total"].(float64)))
}

func TestCheckIfDateIsHoliday(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Create a holiday
	createPayload := map[string]any{
		"name":      "Christmas",
		"date":      "2025-12-25",
		"is_custom": false,
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/holidays", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Check if Christmas is a holiday
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays/check?date=2025-12-25", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var checkResp holidayCheckResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&checkResp))
	resp.Body.Close()

	require.Equal(t, "2025-12-25", checkResp.HolidayCheck["date"])
	require.Equal(t, true, checkResp.HolidayCheck["is_holiday"])
	require.NotNil(t, checkResp.HolidayCheck["holiday"])

	// Check a non-holiday date
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays/check?date=2025-12-26", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&checkResp))
	resp.Body.Close()

	require.Equal(t, "2025-12-26", checkResp.HolidayCheck["date"])
	require.Equal(t, false, checkResp.HolidayCheck["is_holiday"])
}

// ==========================================================================
// Error Cases Tests
// ==========================================================================

func TestRoutineNotFound(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/nonexistent-id", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "ROUTINE_NOT_FOUND", errResp.Error["code"])
}

func TestJobNotFound(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/jobs/nonexistent-id", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "JOB_NOT_FOUND", errResp.Error["code"])
}

func TestDeleteRoutineWithPendingJobs(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	sceneID := createTestScene(t, ts)

	// Create routine
	createPayload := map[string]any{
		"name":              "Test Routine",
		"scene_id":          sceneID,
		"timezone":          "America/New_York",
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "07:30",
	}
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp routineResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	routineID := createResp.Routine["routine_id"].(string)

	// Trigger routine to create a pending job
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/"+routineID+"/trigger", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	resp.Body.Close()

	// Delete routine - should succeed with ON DELETE CASCADE (jobs are auto-deleted)
	resp = doSchedulerRequest(t, http.MethodDelete, ts.URL+"/v1/routines/"+routineID, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify routine is deleted
	resp = doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/"+routineID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestHolidayNotFound(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/holidays/2099-01-01", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "HOLIDAY_NOT_FOUND", errResp.Error["code"])
}

func TestRoutineValidationErrors(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Missing name
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", map[string]any{
		"scene_id": "some-scene-id",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Missing scene_id
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines", map[string]any{
		"name": "Test Routine",
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Invalid body
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/routines", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	resp, _ = http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestEnableDisableNonExistentRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	// Try to enable non-existent routine
	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/nonexistent-id/enable", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Try to disable non-existent routine
	resp = doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/nonexistent-id/disable", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestTriggerNonExistentRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	resp := doSchedulerRequest(t, http.MethodPost, ts.URL+"/v1/routines/nonexistent-id/trigger", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "ROUTINE_NOT_FOUND", errResp.Error["code"])
}

func TestListJobsForNonExistentRoutine(t *testing.T) {
	ts, cleanup := setupSchedulerTestServer(t)
	defer cleanup()

	resp := doSchedulerRequest(t, http.MethodGet, ts.URL+"/v1/routines/nonexistent-id/jobs", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp errorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	require.Equal(t, "ROUTINE_NOT_FOUND", errResp.Error["code"])
}
