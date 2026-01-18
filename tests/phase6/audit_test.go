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

type auditEventResponse struct {
	RequestID string         `json:"request_id"`
	Event     map[string]any `json:"event"`
}

type auditEventsListResponse struct {
	RequestID  string           `json:"request_id"`
	Events     []map[string]any `json:"events"`
	Pagination map[string]any   `json:"pagination"`
}

func setupAuditTestServer(t *testing.T) (*httptest.Server, func()) {
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

func doAuditRequest(t *testing.T, method, url string, body any) *http.Response {
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

// ==========================================================================
// Event Recording Tests
// ==========================================================================

func TestAuditRecordEventWithAllFields(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	jobID := "job-123"
	routineID := "routine-456"
	sceneExecID := "exec-789"
	deviceID := "device-abc"
	requestID := "req-xyz"

	createPayload := map[string]any{
		"type":    "JOB_COMPLETED",
		"level":   "INFO",
		"message": "Job completed successfully",
		"correlation": map[string]any{
			"job_id":             jobID,
			"routine_id":         routineID,
			"scene_execution_id": sceneExecID,
			"device_id":          deviceID,
			"request_id":         requestID,
		},
		"payload": map[string]any{
			"duration_ms": 1500,
			"retries":     0,
		},
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.NotEmpty(t, createResp.RequestID)
	require.NotEmpty(t, createResp.Event["event_id"])
	require.Equal(t, "JOB_COMPLETED", createResp.Event["type"])
	require.Equal(t, "INFO", createResp.Event["level"])
	require.Equal(t, "Job completed successfully", createResp.Event["message"])
	require.NotEmpty(t, createResp.Event["timestamp"])

	// Verify correlation IDs in nested object
	correlation := createResp.Event["correlation"].(map[string]any)
	require.Equal(t, jobID, correlation["job_id"])
	require.Equal(t, routineID, correlation["routine_id"])
	require.Equal(t, sceneExecID, correlation["scene_execution_id"])
	require.Equal(t, deviceID, correlation["device_id"])

	// Verify payload
	payload := createResp.Event["payload"].(map[string]any)
	require.Equal(t, float64(1500), payload["duration_ms"])
	require.Equal(t, float64(0), payload["retries"])
}

func TestAuditRecordEventWithMinimalFields(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	createPayload := map[string]any{
		"type":    "SYSTEM_STARTUP",
		"message": "System started",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.NotEmpty(t, createResp.Event["event_id"])
	require.Equal(t, "SYSTEM_STARTUP", createResp.Event["type"])
	require.Equal(t, "System started", createResp.Event["message"])
}

func TestAuditRecordEventDefaultLevelIsInfo(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	createPayload := map[string]any{
		"type":    "DEVICE_DISCOVERED",
		"message": "New device discovered",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	// Default level should be INFO
	require.Equal(t, "INFO", createResp.Event["level"])
}

func TestAuditRecordEventTimestampIsSet(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	beforeCreate := time.Now().UTC().Add(-time.Second)

	createPayload := map[string]any{
		"type":    "SYSTEM_STARTUP",
		"message": "System started",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	afterCreate := time.Now().UTC().Add(time.Second)

	var createResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	timestampStr := createResp.Event["timestamp"].(string)
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	require.NoError(t, err)

	// Timestamp should be between before and after
	require.True(t, timestamp.After(beforeCreate) || timestamp.Equal(beforeCreate))
	require.True(t, timestamp.Before(afterCreate) || timestamp.Equal(afterCreate))
}

// ==========================================================================
// Event Query Tests
// ==========================================================================

func TestAuditQueryAllEvents(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create some events
	for i := 0; i < 3; i++ {
		payload := map[string]any{
			"type":    "JOB_STARTED",
			"message": "Job started",
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Query all events
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.GreaterOrEqual(t, len(listResp.Events), 3)
	require.NotNil(t, listResp.Pagination)
	require.GreaterOrEqual(t, int(listResp.Pagination["total"].(float64)), 3)
}

func TestAuditQueryWithTypeFilter(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create events of different types
	types := []string{"JOB_STARTED", "JOB_COMPLETED", "JOB_FAILED"}
	for _, eventType := range types {
		payload := map[string]any{
			"type":    eventType,
			"message": "Event: " + eventType,
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Query with type filter
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?type=JOB_COMPLETED", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		require.Equal(t, "JOB_COMPLETED", event["type"])
	}
}

func TestAuditQueryWithLevelFilter(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create events with different levels (INFO, WARN, ERROR - no DEBUG)
	levels := []string{"INFO", "WARN", "ERROR"}
	for _, level := range levels {
		payload := map[string]any{
			"type":    "SYSTEM_ERROR",
			"level":   level,
			"message": "Event at level " + level,
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Query with level filter
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?level=ERROR", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		require.Equal(t, "ERROR", event["level"])
	}
}

func TestAuditQueryWithDateRangeFilter(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create an event
	payload := map[string]any{
		"type":    "JOB_STARTED",
		"message": "Job started",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Query with date range (from now minus 1 hour to now plus 1 hour)
	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)

	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?from="+from+"&to="+to, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.GreaterOrEqual(t, len(listResp.Events), 1)
}

func TestAuditQueryWithCorrelationIDFilters(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	jobID := "test-job-filter"
	routineID := "test-routine-filter"
	sceneExecID := "test-exec-filter"
	deviceID := "test-device-filter"

	// Create events with different correlation IDs using the nested correlation structure
	payloads := []map[string]any{
		{"type": "JOB_STARTED", "message": "Job event", "correlation": map[string]any{"job_id": jobID}},
		{"type": "ROUTINE_CREATED", "message": "Routine event", "correlation": map[string]any{"routine_id": routineID}},
		{"type": "SCENE_EXECUTION_STARTED", "message": "Scene exec event", "correlation": map[string]any{"scene_execution_id": sceneExecID}},
		{"type": "DEVICE_DISCOVERED", "message": "Device event", "correlation": map[string]any{"device_id": deviceID}},
	}

	for _, payload := range payloads {
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Test job_id filter
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?job_id="+jobID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()
	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		correlation := event["correlation"].(map[string]any)
		require.Equal(t, jobID, correlation["job_id"])
	}

	// Test routine_id filter
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?routine_id="+routineID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()
	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		correlation := event["correlation"].(map[string]any)
		require.Equal(t, routineID, correlation["routine_id"])
	}

	// Test scene_execution_id filter
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?scene_execution_id="+sceneExecID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()
	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		correlation := event["correlation"].(map[string]any)
		require.Equal(t, sceneExecID, correlation["scene_execution_id"])
	}

	// Test device_id filter
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?device_id="+deviceID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()
	require.GreaterOrEqual(t, len(listResp.Events), 1)
	for _, event := range listResp.Events {
		correlation := event["correlation"].(map[string]any)
		require.Equal(t, deviceID, correlation["device_id"])
	}
}

func TestAuditQueryWithPagination(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create 5 events
	for i := 0; i < 5; i++ {
		payload := map[string]any{
			"type":    "JOB_STARTED",
			"message": "Job started",
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Query with limit
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?limit=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Events, 3)
	require.GreaterOrEqual(t, int(listResp.Pagination["total"].(float64)), 5)
	require.True(t, listResp.Pagination["has_more"].(bool))

	// Query with offset
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?limit=3&offset=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Events, 2)
	require.False(t, listResp.Pagination["has_more"].(bool))
}

func TestAuditQueryHasMoreFlag(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create exactly 5 events
	for i := 0; i < 5; i++ {
		payload := map[string]any{
			"type":    "JOB_STARTED",
			"message": "Job started",
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Query first page (limit=2) - should have more
	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?limit=2&offset=0", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.True(t, listResp.Pagination["has_more"].(bool))

	// Query last page (offset=4, limit=2) - should NOT have more
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?limit=2&offset=4", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.False(t, listResp.Pagination["has_more"].(bool))
}

// ==========================================================================
// Single Event Retrieval Tests
// ==========================================================================

func TestAuditGetEventByID(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Create an event
	createPayload := map[string]any{
		"type":    "JOB_COMPLETED",
		"level":   "INFO",
		"message": "Job completed successfully",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	eventID := createResp.Event["event_id"].(string)

	// Get the event by ID
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events/"+eventID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp auditEventResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getResp))
	resp.Body.Close()

	require.Equal(t, eventID, getResp.Event["event_id"])
	require.Equal(t, "JOB_COMPLETED", getResp.Event["type"])
	require.Equal(t, "INFO", getResp.Event["level"])
	require.Equal(t, "Job completed successfully", getResp.Event["message"])
}

func TestAuditGetNonExistentEvent(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	resp := doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events/nonexistent-event-id", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "EVENT_NOT_FOUND", errorData["code"])
}

// ==========================================================================
// Error Cases Tests
// ==========================================================================

func TestAuditInvalidEventType(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	payload := map[string]any{
		"type":    "INVALID_TYPE",
		"message": "This should fail",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
}

func TestAuditMissingRequiredFields(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Missing type
	payload := map[string]any{
		"message": "Missing type",
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
	require.Contains(t, errorData["message"], "type")

	// Note: message is optional per the current implementation (empty string is allowed)
}

func TestAuditInvalidRequestBody(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/audit/events", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	resp, _ := http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestAuditEventWithDifferentLevels(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	// Only INFO, WARN, ERROR are valid (no DEBUG)
	levels := []string{"INFO", "WARN", "ERROR"}

	for _, level := range levels {
		payload := map[string]any{
			"type":    "SYSTEM_ERROR",
			"level":   level,
			"message": "Event at level " + level,
		}
		resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp auditEventResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
		resp.Body.Close()

		require.Equal(t, level, createResp.Event["level"])
	}
}

func TestAuditQueryCombinedFilters(t *testing.T) {
	ts, cleanup := setupAuditTestServer(t)
	defer cleanup()

	jobID := "combined-filter-job"

	// Create events with specific attributes
	payload := map[string]any{
		"type":    "JOB_COMPLETED",
		"level":   "INFO",
		"message": "Combined filter test",
		"correlation": map[string]any{
			"job_id": jobID,
		},
	}
	resp := doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Create another event with different type but same job_id
	payload = map[string]any{
		"type":    "JOB_STARTED",
		"level":   "INFO",
		"message": "Different type",
		"correlation": map[string]any{
			"job_id": jobID,
		},
	}
	resp = doAuditRequest(t, http.MethodPost, ts.URL+"/v1/audit/events", payload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Query with combined filters
	resp = doAuditRequest(t, http.MethodGet, ts.URL+"/v1/audit/events?type=JOB_COMPLETED&job_id="+jobID+"&level=INFO", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp auditEventsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Events, 1)
	require.Equal(t, "JOB_COMPLETED", listResp.Events[0]["type"])
	correlation := listResp.Events[0]["correlation"].(map[string]any)
	require.Equal(t, jobID, correlation["job_id"])
}
