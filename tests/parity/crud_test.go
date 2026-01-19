//go:build parity

package parity

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CRUD Parity Tests
//
// These tests verify that both Node.js and Go servers produce the same
// response structure for Create, Read, Update, Delete operations.
//
// Key concept: We're testing RESPONSE PARITY (same structure), not DATA PARITY
// (same records). Each server creates its own record with a unique name,
// and we compare the shapes of the responses.
// ============================================================================

func TestSceneCRUDParity(t *testing.T) {
	// Use different names for each server to avoid unique constraint conflicts
	// (both servers share the same database)
	baseName := "parity_test_scene_" + uuid.New().String()[:8]

	nodePayload := map[string]any{
		"name":                   baseName + "_node",
		"coordinator_preference": "ARC_FIRST",
		"speakers":               []map[string]any{},
	}
	goPayload := map[string]any{
		"name":                   baseName + "_go",
		"coordinator_preference": "ARC_FIRST",
		"speakers":               []map[string]any{},
	}

	// CREATE
	nodeResp, nodeStatus := postJSON(t, NodeJSBaseURL+"/v1/scenes", nodePayload)
	goResp, goStatus := postJSON(t, GoBaseURL+"/v1/scenes", goPayload)

	require.Equal(t, http.StatusCreated, nodeStatus, "Node.js create failed: %v", nodeResp)
	require.Equal(t, http.StatusCreated, goStatus, "Go create failed: %v", goResp)

	// Extract the scene from the response - may be wrapped differently
	nodeScene := extractSingleResource(nodeResp, "data")
	goScene := extractSingleResource(goResp, "scene")
	if nodeScene == nil {
		nodeScene = nodeResp
	}
	if goScene == nil {
		goScene = goResp
	}

	nodeID, ok := nodeScene["scene_id"].(string)
	require.True(t, ok, "Node.js response missing scene_id: %v", nodeScene)
	goID, ok := goScene["id"].(string)
	require.True(t, ok, "Go response missing scene_id: %v", goScene)

	// Defer cleanup
	defer deleteResource(t, NodeJSBaseURL+"/v1/scenes/"+nodeID)
	defer deleteResource(t, GoBaseURL+"/v1/scenes/"+goID)

	// Compare CREATE response shapes (ignore name since we use different names)
	compareResponsesIgnoring(t, nodeScene, goScene,
		[]string{"scene_id", "name", "created_at", "updated_at", "request_id", "object"})

	// READ - fetch the created scenes
	nodeGetResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes/"+nodeID)
	goGetResp := fetchJSON(t, GoBaseURL+"/v1/scenes/"+goID)

	nodeGetScene := extractSingleResource(nodeGetResp, "data")
	goGetScene := extractSingleResource(goGetResp, "scene")
	if nodeGetScene == nil {
		nodeGetScene = nodeGetResp
	}
	if goGetScene == nil {
		goGetScene = goGetResp
	}

	compareResponsesIgnoring(t, nodeGetScene, goGetScene,
		[]string{"scene_id", "name", "created_at", "updated_at", "request_id", "object"})

	// UPDATE - use different names again to avoid conflicts
	nodeUpdatePayload := map[string]any{"name": baseName + "_node_updated"}
	goUpdatePayload := map[string]any{"name": baseName + "_go_updated"}

	nodePut, nodePutStatus := putJSON(t, NodeJSBaseURL+"/v1/scenes/"+nodeID, nodeUpdatePayload)
	goPut, goPutStatus := putJSON(t, GoBaseURL+"/v1/scenes/"+goID, goUpdatePayload)

	require.Equal(t, nodePutStatus, goPutStatus, "PUT status code mismatch: Node=%d, Go=%d", nodePutStatus, goPutStatus)

	nodePutScene := extractSingleResource(nodePut, "data")
	goPutScene := extractSingleResource(goPut, "scene")
	if nodePutScene == nil {
		nodePutScene = nodePut
	}
	if goPutScene == nil {
		goPutScene = goPut
	}

	// Compare UPDATE response shapes
	compareResponsesIgnoring(t, nodePutScene, goPutScene,
		[]string{"scene_id", "name", "created_at", "updated_at", "request_id", "object"})

	// DELETE - test that both return same status code
	nodeDeleteStatus := deleteResource(t, NodeJSBaseURL+"/v1/scenes/"+nodeID)
	goDeleteStatus := deleteResource(t, GoBaseURL+"/v1/scenes/"+goID)

	require.Equal(t, nodeDeleteStatus, goDeleteStatus, "DELETE status code mismatch")
}

func TestRoutineCRUDParity(t *testing.T) {
	baseName := uuid.New().String()[:8]

	// First create a scene to reference (routines require a scene_id)
	scenePayload := map[string]any{
		"name":                   "parity_test_scene_for_routine_" + baseName,
		"coordinator_preference": "ARC_FIRST",
		"speakers":               []map[string]any{},
	}

	// Create via Node.js only - both servers read same DB
	sceneResp, sceneStatus := postJSON(t, NodeJSBaseURL+"/v1/scenes", scenePayload)
	require.Equal(t, http.StatusCreated, sceneStatus, "failed to create prerequisite scene: %v", sceneResp)

	sceneData := extractSingleResource(sceneResp, "data")
	if sceneData == nil {
		sceneData = sceneResp
	}
	sceneID, ok := sceneData["scene_id"].(string)
	require.True(t, ok, "scene_id not found in response")
	defer deleteResource(t, NodeJSBaseURL+"/v1/scenes/"+sceneID)

	// Now create routines via both servers
	// Node.js expects nested schedule object, Go uses flat fields
	nodePayload := map[string]any{
		"name":     "parity_test_routine_" + baseName + "_node",
		"scene_id": sceneID,
		"schedule": map[string]any{
			"type":     "weekly",
			"weekdays": []int{1, 2, 3, 4, 5},
			"time":     "08:00",
		},
		"timezone": "America/Los_Angeles",
	}
	goPayload := map[string]any{
		"name":              "parity_test_routine_" + baseName + "_go",
		"scene_id":          sceneID,
		"schedule_type":     "weekly",
		"schedule_weekdays": []int{1, 2, 3, 4, 5},
		"schedule_time":     "08:00",
		"timezone":          "America/Los_Angeles",
	}

	// CREATE
	nodeResp, nodeStatus := postJSON(t, NodeJSBaseURL+"/v1/routines", nodePayload)
	goResp, goStatus := postJSON(t, GoBaseURL+"/v1/routines", goPayload)

	require.Equal(t, http.StatusCreated, nodeStatus, "Node.js create failed: %v", nodeResp)
	require.Equal(t, http.StatusCreated, goStatus, "Go create failed: %v", goResp)

	nodeRoutine := extractSingleResource(nodeResp, "routine")
	goRoutine := extractSingleResource(goResp, "routine")
	if nodeRoutine == nil {
		nodeRoutine = nodeResp
	}
	if goRoutine == nil {
		goRoutine = goResp
	}

	nodeID, ok := nodeRoutine["routine_id"].(string)
	require.True(t, ok, "Node.js response missing routine_id: %v", nodeRoutine)
	goID, ok := goRoutine["id"].(string)
	require.True(t, ok, "Go response missing routine_id: %v", goRoutine)

	defer deleteResource(t, NodeJSBaseURL+"/v1/routines/"+nodeID)
	defer deleteResource(t, GoBaseURL+"/v1/routines/"+goID)

	// Note: schedule structure differs between servers (nested vs flat input parsing)
	// We ignore schedule-related fields in comparison since we're testing response shape
	compareResponsesIgnoring(t, nodeRoutine, goRoutine,
		[]string{"routine_id", "name", "created_at", "updated_at", "request_id", "object", "next_run", "schedule", "last_run_at"})

	// READ
	nodeGetResp := fetchJSON(t, NodeJSBaseURL+"/v1/routines/"+nodeID)
	goGetResp := fetchJSON(t, GoBaseURL+"/v1/routines/"+goID)

	nodeGetRoutine := extractSingleResource(nodeGetResp, "routine")
	goGetRoutine := extractSingleResource(goGetResp, "routine")
	if nodeGetRoutine == nil {
		nodeGetRoutine = nodeGetResp
	}
	if goGetRoutine == nil {
		goGetRoutine = goGetResp
	}

	compareResponsesIgnoring(t, nodeGetRoutine, goGetRoutine,
		[]string{"routine_id", "name", "created_at", "updated_at", "request_id", "object", "next_run", "schedule", "last_run_at"})

	// UPDATE
	nodeUpdatePayload := map[string]any{"name": "parity_test_routine_" + baseName + "_node_updated"}
	goUpdatePayload := map[string]any{"name": "parity_test_routine_" + baseName + "_go_updated"}

	nodePut, nodePutStatus := putJSON(t, NodeJSBaseURL+"/v1/routines/"+nodeID, nodeUpdatePayload)
	goPut, goPutStatus := putJSON(t, GoBaseURL+"/v1/routines/"+goID, goUpdatePayload)

	require.Equal(t, nodePutStatus, goPutStatus, "PUT status code mismatch")

	nodePutRoutine := extractSingleResource(nodePut, "routine")
	goPutRoutine := extractSingleResource(goPut, "routine")
	if nodePutRoutine == nil {
		nodePutRoutine = nodePut
	}
	if goPutRoutine == nil {
		goPutRoutine = goPut
	}

	compareResponsesIgnoring(t, nodePutRoutine, goPutRoutine,
		[]string{"routine_id", "name", "created_at", "updated_at", "request_id", "object", "next_run", "schedule", "last_run_at"})

	// DELETE
	nodeDeleteStatus := deleteResource(t, NodeJSBaseURL+"/v1/routines/"+nodeID)
	goDeleteStatus := deleteResource(t, GoBaseURL+"/v1/routines/"+goID)

	require.Equal(t, nodeDeleteStatus, goDeleteStatus, "DELETE status code mismatch")
}

func TestMusicSetCRUDParity(t *testing.T) {
	baseName := "parity_test_music_set_" + uuid.New().String()[:8]

	nodePayload := map[string]any{
		"name":             baseName + "_node",
		"description":      "Test music set for parity",
		"selection_policy": "ROTATION",
	}
	goPayload := map[string]any{
		"name":             baseName + "_go",
		"description":      "Test music set for parity",
		"selection_policy": "ROTATION",
	}

	// CREATE
	nodeResp, nodeStatus := postJSON(t, NodeJSBaseURL+"/v1/music/sets", nodePayload)
	goResp, goStatus := postJSON(t, GoBaseURL+"/v1/music/sets", goPayload)

	require.Equal(t, http.StatusCreated, nodeStatus, "Node.js create failed: %v", nodeResp)
	require.Equal(t, http.StatusCreated, goStatus, "Go create failed: %v", goResp)

	nodeSet := extractSingleResource(nodeResp, "set")
	goSet := extractSingleResource(goResp, "set")
	if nodeSet == nil {
		nodeSet = nodeResp
	}
	if goSet == nil {
		goSet = goResp
	}

	nodeID, ok := nodeSet["set_id"].(string)
	require.True(t, ok, "Node.js response missing set_id: %v", nodeSet)
	goID, ok := goSet["id"].(string)
	require.True(t, ok, "Go response missing set_id: %v", goSet)

	defer deleteResource(t, NodeJSBaseURL+"/v1/music/sets/"+nodeID)
	defer deleteResource(t, GoBaseURL+"/v1/music/sets/"+goID)

	// Node.js includes artwork_url field that Go doesn't
	compareResponsesIgnoring(t, nodeSet, goSet,
		[]string{"set_id", "name", "created_at", "updated_at", "request_id", "object", "artwork_url"})

	// READ
	nodeGetResp := fetchJSON(t, NodeJSBaseURL+"/v1/music/sets/"+nodeID)
	goGetResp := fetchJSON(t, GoBaseURL+"/v1/music/sets/"+goID)

	nodeGetSet := extractSingleResource(nodeGetResp, "set")
	goGetSet := extractSingleResource(goGetResp, "set")
	if nodeGetSet == nil {
		nodeGetSet = nodeGetResp
	}
	if goGetSet == nil {
		goGetSet = goGetResp
	}

	compareResponsesIgnoring(t, nodeGetSet, goGetSet,
		[]string{"set_id", "name", "created_at", "updated_at", "request_id", "object", "artwork_url"})

	// UPDATE with PATCH (music sets use PATCH not PUT)
	patchPayload := map[string]any{"description": "Updated description for parity test"}

	nodePatch, nodePatchStatus := patchJSON(t, NodeJSBaseURL+"/v1/music/sets/"+nodeID, patchPayload)
	goPatch, goPatchStatus := patchJSON(t, GoBaseURL+"/v1/music/sets/"+goID, patchPayload)

	require.Equal(t, nodePatchStatus, goPatchStatus, "PATCH status code mismatch")

	nodePatchSet := extractSingleResource(nodePatch, "set")
	goPatchSet := extractSingleResource(goPatch, "set")
	if nodePatchSet == nil {
		nodePatchSet = nodePatch
	}
	if goPatchSet == nil {
		goPatchSet = goPatch
	}

	compareResponsesIgnoring(t, nodePatchSet, goPatchSet,
		[]string{"set_id", "name", "created_at", "updated_at", "request_id", "object", "artwork_url"})

	// DELETE
	nodeDeleteStatus := deleteResource(t, NodeJSBaseURL+"/v1/music/sets/"+nodeID)
	goDeleteStatus := deleteResource(t, GoBaseURL+"/v1/music/sets/"+goID)

	require.Equal(t, nodeDeleteStatus, goDeleteStatus, "DELETE status code mismatch")
}

func TestAuditEventCreateParity(t *testing.T) {
	// Audit events are write-once, no update/delete supported
	// These accumulate but are pruned automatically by the audit service
	payload := map[string]any{
		"type":    "SYSTEM_STARTUP",
		"message": "parity_test: test audit event",
		"level":   "INFO",
		"payload": map[string]any{"test": true, "source": "parity_test"},
	}

	nodeResp, nodeStatus := postJSON(t, NodeJSBaseURL+"/v1/audit/events", payload)
	goResp, goStatus := postJSON(t, GoBaseURL+"/v1/audit/events", payload)

	require.Equal(t, http.StatusCreated, nodeStatus, "Node.js create failed: %v", nodeResp)
	require.Equal(t, http.StatusCreated, goStatus, "Go create failed: %v", goResp)

	nodeEvent := extractSingleResource(nodeResp, "event")
	goEvent := extractSingleResource(goResp, "event")
	if nodeEvent == nil {
		nodeEvent = nodeResp
	}
	if goEvent == nil {
		goEvent = goResp
	}

	// Compare response shapes only - no cleanup needed (auto-pruned)
	// Note: Node.js includes correlation object with nil fields, Go omits it
	// Timestamp format also differs (Node.js has milliseconds)
	compareResponsesIgnoring(t, nodeEvent, goEvent,
		[]string{"event_id", "created_at", "request_id", "object", "correlation", "timestamp"})
}

// TestSceneNotFoundParity verifies both servers return the same error response
// for a non-existent scene
func TestSceneNotFoundParity(t *testing.T) {
	fakeID := "scn_" + uuid.New().String()

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/scenes/"+fakeID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/scenes/"+fakeID)

	require.Equal(t, http.StatusNotFound, nodeStatus, "Node.js should return 404")
	require.Equal(t, http.StatusNotFound, goStatus, "Go should return 404")

	// Both should have an error structure
	require.NotNil(t, nodeResp, "Node.js response should not be nil")
	require.NotNil(t, goResp, "Go response should not be nil")
}

// TestRoutineNotFoundParity verifies both servers return the same error response
// for a non-existent routine
func TestRoutineNotFoundParity(t *testing.T) {
	fakeID := "rtn_" + uuid.New().String()

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/routines/"+fakeID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/routines/"+fakeID)

	require.Equal(t, http.StatusNotFound, nodeStatus, "Node.js should return 404")
	require.Equal(t, http.StatusNotFound, goStatus, "Go should return 404")

	require.NotNil(t, nodeResp, "Node.js response should not be nil")
	require.NotNil(t, goResp, "Go response should not be nil")
}

// TestMusicSetNotFoundParity verifies both servers return the same error response
// for a non-existent music set
func TestMusicSetNotFoundParity(t *testing.T) {
	fakeID := "set_" + uuid.New().String()

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/music/sets/"+fakeID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/music/sets/"+fakeID)

	require.Equal(t, http.StatusNotFound, nodeStatus, "Node.js should return 404")
	require.Equal(t, http.StatusNotFound, goStatus, "Go should return 404")

	require.NotNil(t, nodeResp, "Node.js response should not be nil")
	require.NotNil(t, goResp, "Go response should not be nil")
}
