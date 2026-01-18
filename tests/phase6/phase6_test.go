package phase6

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/server"
)

type sceneResponse struct {
	RequestID string         `json:"request_id"`
	Scene     map[string]any `json:"scene"`
}

type listScenesResponse struct {
	RequestID  string           `json:"request_id"`
	Scenes     []map[string]any `json:"scenes"`
	Pagination map[string]any   `json:"pagination"`
}

type executeResponse struct {
	RequestID string         `json:"request_id"`
	Execution map[string]any `json:"execution"`
}

type listExecutionsResponse struct {
	RequestID  string           `json:"request_id"`
	Executions []map[string]any `json:"executions"`
	Pagination map[string]any   `json:"pagination"`
}

func setupTestServer(t *testing.T) (*httptest.Server, func()) {
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

func doRequest(t *testing.T, method, url string, body any) *http.Response {
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

func TestSceneCRUD(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create scene
	createPayload := map[string]any{
		"name":        "Morning Music",
		"description": "Wake up playlist",
		"members": []map[string]any{
			{"device_id": "device-123", "target_volume": 40},
			{"device_id": "device-456", "room_name": "Kitchen"},
		},
		"volume_ramp": map[string]any{
			"enabled":     true,
			"duration_ms": 5000,
			"curve":       "ease-in",
		},
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.NotEmpty(t, createResp.RequestID)
	require.NotEmpty(t, createResp.Scene["scene_id"])
	require.Equal(t, "Morning Music", createResp.Scene["name"])
	require.Equal(t, "Wake up playlist", createResp.Scene["description"])
	require.Equal(t, "ARC_FIRST", createResp.Scene["coordinator_preference"])
	require.Equal(t, "PLAYBASE_IF_ARC_TV_ACTIVE", createResp.Scene["fallback_policy"])

	sceneID := createResp.Scene["scene_id"].(string)

	// Get scene
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/scenes/"+sceneID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getResp))
	resp.Body.Close()

	require.Equal(t, sceneID, getResp.Scene["scene_id"])
	require.Equal(t, "Morning Music", getResp.Scene["name"])

	// Update scene
	updatePayload := map[string]any{
		"name": "Evening Music",
	}
	resp = doRequest(t, http.MethodPut, ts.URL+"/v1/scenes/"+sceneID, updatePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updateResp))
	resp.Body.Close()

	require.Equal(t, "Evening Music", updateResp.Scene["name"])
	// Members should be preserved
	members := updateResp.Scene["members"].([]any)
	require.Len(t, members, 2)

	// List scenes
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/scenes", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listScenesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Scenes, 1)
	require.Equal(t, 1, int(listResp.Pagination["total"].(float64)))

	// Delete scene
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/scenes/"+sceneID, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify deleted
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/scenes/"+sceneID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestSceneNotFound(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/scenes/nonexistent-id", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "SCENE_NOT_FOUND", errorData["code"])
}

func TestSceneValidation(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Missing name
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", map[string]any{
		"members": []map[string]any{},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Invalid body
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/scenes", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	resp, _ = http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestSceneExecuteCreatesExecution(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create scene
	createPayload := map[string]any{
		"name":    "Test Scene",
		"members": []map[string]any{},
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	sceneID := createResp.Scene["scene_id"].(string)

	// Execute scene
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/scenes/"+sceneID+"/execute", nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var execResp executeResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&execResp))
	resp.Body.Close()

	require.NotEmpty(t, execResp.Execution["scene_execution_id"])
	require.Equal(t, sceneID, execResp.Execution["scene_id"])
	require.Equal(t, "STARTING", execResp.Execution["status"])
	require.False(t, execResp.Execution["idempotent"].(bool))

	_ = execResp.Execution["scene_execution_id"].(string)
}

func TestSceneExecuteIdempotency(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create scene
	createPayload := map[string]any{
		"name":    "Test Scene",
		"members": []map[string]any{},
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	sceneID := createResp.Scene["scene_id"].(string)

	// Execute with idempotency key
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/scenes/"+sceneID+"/execute", bytes.NewBuffer(nil))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	req.Header.Set("Idempotency-Key", "test-key-123")
	resp, _ = http.DefaultClient.Do(req)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var execResp1 executeResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&execResp1))
	resp.Body.Close()

	execID := execResp1.Execution["scene_execution_id"].(string)

	// Execute again with same idempotency key
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/v1/scenes/"+sceneID+"/execute", bytes.NewBuffer(nil))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	req.Header.Set("Idempotency-Key", "test-key-123")
	resp, _ = http.DefaultClient.Do(req)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var execResp2 executeResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&execResp2))
	resp.Body.Close()

	// Should return the same execution
	require.Equal(t, execID, execResp2.Execution["scene_execution_id"])
}

func TestSceneExecutionsList(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create scene
	createPayload := map[string]any{
		"name":    "Test Scene",
		"members": []map[string]any{},
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	sceneID := createResp.Scene["scene_id"].(string)

	// Create multiple executions
	for i := 0; i < 3; i++ {
		resp = doRequest(t, http.MethodPost, ts.URL+"/v1/scenes/"+sceneID+"/execute", nil)
		require.Equal(t, http.StatusAccepted, resp.StatusCode)
		resp.Body.Close()
	}

	// List executions
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/scenes/"+sceneID+"/executions", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listExecutionsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Executions, 3)
	require.Equal(t, 3, int(listResp.Pagination["total"].(float64)))
}

func TestSceneStop(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create scene with no members (will succeed but have empty results)
	createPayload := map[string]any{
		"name":    "Test Scene",
		"members": []map[string]any{},
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp sceneResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	sceneID := createResp.Scene["scene_id"].(string)

	// Stop scene
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/scenes/"+sceneID+"/stop", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var stopResp actionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&stopResp))
	resp.Body.Close()

	require.Equal(t, sceneID, stopResp.Result["scene_id"])
	require.True(t, stopResp.Result["all_succeeded"].(bool))
}

func TestListScenesPagination(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create 5 scenes
	for i := 0; i < 5; i++ {
		createPayload := map[string]any{
			"name":    "Scene " + string(rune('A'+i)),
			"members": []map[string]any{},
		}
		resp := doRequest(t, http.MethodPost, ts.URL+"/v1/scenes", createPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// List with limit
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/scenes?limit=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listScenesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Scenes, 3)
	require.Equal(t, 5, int(listResp.Pagination["total"].(float64)))
	require.True(t, listResp.Pagination["has_more"].(bool))

	// List with offset
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/scenes?limit=3&offset=3", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Scenes, 2)
	require.False(t, listResp.Pagination["has_more"].(bool))
}
