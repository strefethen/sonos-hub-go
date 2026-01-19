//go:build parity

package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	NodeJSBaseURL = "http://localhost:9000"
	GoBaseURL     = "http://localhost:9001"
)

// fetchJSON fetches a URL with X-Test-Mode header and decodes JSON
func fetchJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "endpoint %s returned %d", url, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

// extractListData extracts array from Go's "data"/"items" or Node's nested structure
func extractListData(body map[string]any, nodeKey string) []any {
	// Try Go-style first (data is directly an array)
	if data, ok := body["data"].([]any); ok {
		return data
	}
	// Try Go-style "items" key (used for alarms, groups, services, etc.)
	if data, ok := body["items"].([]any); ok {
		return data
	}
	// Try Node.js-style (data is an object containing the named key)
	if dataObj, ok := body["data"].(map[string]any); ok {
		if arr, ok := dataObj[nodeKey].([]any); ok {
			return arr
		}
	}
	// Try top-level named key as fallback (Node.js sometimes uses resource-specific keys)
	if data, ok := body[nodeKey].([]any); ok {
		return data
	}
	return nil
}

// extractString extracts a string field, optionally from a nested key
func extractString(resp map[string]any, field, nestedKey string) string {
	if nestedKey != "" {
		if nested, ok := resp[nestedKey].(map[string]any); ok {
			if val, ok := nested[field].(string); ok {
				return val
			}
		}
	}
	if val, ok := resp[field].(string); ok {
		return val
	}
	return ""
}

// extractSingleResource extracts object from Go's direct return or Node's named wrapper
func extractSingleResource(body map[string]any, nodeKey string) map[string]any {
	// Go returns resource directly (has "object" field)
	if _, hasObject := body["object"]; hasObject {
		return body
	}
	// Node.js wraps in named key
	if resource, ok := body[nodeKey].(map[string]any); ok {
		return resource
	}
	// Try "data" key (scenes use this)
	if resource, ok := body["data"].(map[string]any); ok {
		return resource
	}
	// Node.js sometimes returns resource directly with ID field (e.g., set_id, template_id)
	idKey := nodeKey + "_id"
	if _, hasID := body[idKey]; hasID {
		return body
	}
	return nil
}

// getFirstID extracts first ID from a list response for subsequent tests
func getFirstID(t *testing.T, list []any, idKey string) string {
	t.Helper()
	if len(list) == 0 {
		t.Skip("No items in list to test single resource endpoint")
	}
	item, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("First item is not a map: %T", list[0])
	}
	id, ok := item[idKey].(string)
	if !ok {
		t.Fatalf("ID key %q not found or not a string in item", idKey)
	}
	return id
}

// fetchJSONWithStatus fetches a URL and returns both body and status code
func fetchJSONWithStatus(t *testing.T, url string) (map[string]any, int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode
	}
	return result, resp.StatusCode
}

// getFirstDeviceID gets a device ID for device-dependent tests
func getFirstDeviceID(t *testing.T, baseURL string) string {
	t.Helper()
	resp := fetchJSON(t, baseURL+"/v1/devices")
	devices := extractListData(resp, "devices")
	if len(devices) == 0 {
		t.Skip("No devices available for device-dependent test")
	}
	return getFirstID(t, devices, "device_id")
}

// ============================================================================
// CRUD Helper Functions
// ============================================================================

// postJSON sends POST request and returns parsed response with status code
func postJSON(t *testing.T, url string, payload any) (map[string]any, int) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result) // Ignore error for empty responses
	return result, resp.StatusCode
}

// putJSON sends PUT request and returns parsed response with status code
func putJSON(t *testing.T, url string, payload any) (map[string]any, int) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode
}

// patchJSON sends PATCH request and returns parsed response with status code
func patchJSON(t *testing.T, url string, payload any) (map[string]any, int) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode
}

// deleteResource sends DELETE request and returns status code (for use in tests)
func deleteResource(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	return resp.StatusCode
}

// deleteResourceNoFail sends DELETE request without testing.T (for TestMain cleanup)
func deleteResourceNoFail(url string) error {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		return fmt.Errorf("delete failed with status %d", resp.StatusCode)
	}
	return nil
}

// fetchJSONNoFail fetches JSON without testing.T (for TestMain cleanup)
func fetchJSONNoFail(url string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Test-Mode", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed with status %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// compareResponsesIgnoring compares two responses, ignoring specified fields
// It does a deep comparison after removing the ignored fields from both maps
func compareResponsesIgnoring(t *testing.T, nodeResp, goResp map[string]any, ignoreFields []string) {
	t.Helper()

	// Deep copy both maps
	nodeCopy := deepCopyMap(nodeResp)
	goCopy := deepCopyMap(goResp)

	// Remove ignored fields from both
	for _, field := range ignoreFields {
		delete(nodeCopy, field)
		delete(goCopy, field)
	}

	// Compare remaining structure
	if !reflect.DeepEqual(nodeCopy, goCopy) {
		t.Errorf("Response structure mismatch:\nNode.js: %+v\nGo:      %+v", nodeCopy, goCopy)
	}
}

// deepCopyMap creates a deep copy of a map[string]any
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			result[k] = deepCopyMap(val)
		case []any:
			result[k] = deepCopySlice(val)
		default:
			result[k] = v
		}
	}
	return result
}

// deepCopySlice creates a deep copy of a []any
func deepCopySlice(s []any) []any {
	if s == nil {
		return nil
	}
	result := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			result[i] = deepCopyMap(val)
		case []any:
			result[i] = deepCopySlice(val)
		default:
			result[i] = v
		}
	}
	return result
}

// cleanupStaleTestData removes any leftover test data with "parity_test_" prefix
func cleanupStaleTestData() {
	log.Println("Cleaning up stale parity test data...")

	// Try both servers - cleanup from whichever responds
	baseURLs := []string{NodeJSBaseURL, GoBaseURL}

	for _, baseURL := range baseURLs {
		// Cleanup scenes
		if resp, err := fetchJSONNoFail(baseURL + "/v1/scenes"); err == nil {
			scenes := extractListData(resp, "data")
			for _, s := range scenes {
				scene := s.(map[string]any)
				name, _ := scene["name"].(string)
				if strings.HasPrefix(name, "parity_test_") {
					id := scene["scene_id"].(string)
					if err := deleteResourceNoFail(baseURL + "/v1/scenes/" + id); err == nil {
						log.Printf("  Deleted stale scene: %s", name)
					}
				}
			}
		}

		// Cleanup routines
		if resp, err := fetchJSONNoFail(baseURL + "/v1/routines"); err == nil {
			routines := extractListData(resp, "routines")
			for _, r := range routines {
				routine := r.(map[string]any)
				name, _ := routine["name"].(string)
				if strings.HasPrefix(name, "parity_test_") {
					id := routine["routine_id"].(string)
					if err := deleteResourceNoFail(baseURL + "/v1/routines/" + id); err == nil {
						log.Printf("  Deleted stale routine: %s", name)
					}
				}
			}
		}

		// Cleanup music sets
		if resp, err := fetchJSONNoFail(baseURL + "/v1/music/sets"); err == nil {
			sets := extractListData(resp, "sets")
			for _, s := range sets {
				set := s.(map[string]any)
				name, _ := set["name"].(string)
				if strings.HasPrefix(name, "parity_test_") {
					id := set["set_id"].(string)
					if err := deleteResourceNoFail(baseURL + "/v1/music/sets/" + id); err == nil {
						log.Printf("  Deleted stale music set: %s", name)
					}
				}
			}
		}

		// If we successfully cleaned up from this server, break (shared DB)
		break
	}
}

// TestMain runs before/after all tests in this package
func TestMain(m *testing.M) {
	cleanupStaleTestData() // Best-effort cleanup of leftover test data
	code := m.Run()
	cleanupStaleTestData() // Cleanup after tests
	os.Exit(code)
}

func TestDeviceListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/devices")
	goResp := fetchJSON(t, GoBaseURL+"/v1/devices")

	nodeDevices := extractListData(nodeResp, "devices")
	goDevices := extractListData(goResp, "devices")

	require.Equal(t, len(nodeDevices), len(goDevices), "device count mismatch")

	// Build map by device_id
	nodeByID := make(map[string]map[string]any)
	for _, d := range nodeDevices {
		device := d.(map[string]any)
		nodeByID[device["device_id"].(string)] = device
	}

	// Compare each Go device to Node device
	for _, d := range goDevices {
		goDevice := d.(map[string]any)
		deviceID := goDevice["device_id"].(string)
		nodeDevice, exists := nodeByID[deviceID]
		require.True(t, exists, "Go has device %s not in Node.js", deviceID)

		require.Equal(t, nodeDevice["room_name"], goDevice["room_name"], "room_name mismatch for %s", deviceID)
		require.Equal(t, nodeDevice["model"], goDevice["model"], "model mismatch for %s", deviceID)
		require.Equal(t, nodeDevice["ip"], goDevice["ip"], "ip mismatch for %s", deviceID)
	}
}

func TestRoutineListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/routines")
	goResp := fetchJSON(t, GoBaseURL+"/v1/routines")

	nodeRoutines := extractListData(nodeResp, "routines")
	goRoutines := extractListData(goResp, "routines")

	require.Equal(t, len(nodeRoutines), len(goRoutines), "routine count mismatch")

	nodeByID := make(map[string]map[string]any)
	for _, r := range nodeRoutines {
		routine := r.(map[string]any)
		nodeByID[routine["routine_id"].(string)] = routine
	}

	for _, r := range goRoutines {
		goRoutine := r.(map[string]any)
		routineID := goRoutine["routine_id"].(string)
		nodeRoutine, exists := nodeByID[routineID]
		require.True(t, exists, "Go has routine %s not in Node.js", routineID)

		require.Equal(t, nodeRoutine["name"], goRoutine["name"], "name mismatch for %s", routineID)
		require.Equal(t, nodeRoutine["enabled"], goRoutine["enabled"], "enabled mismatch for %s", routineID)
	}
}

func TestMusicSetsListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/music/sets")
	goResp := fetchJSON(t, GoBaseURL+"/v1/music/sets")

	nodeSets := extractListData(nodeResp, "sets")
	goSets := extractListData(goResp, "sets")

	require.Equal(t, len(nodeSets), len(goSets), "music set count mismatch")

	nodeByID := make(map[string]map[string]any)
	for _, s := range nodeSets {
		set := s.(map[string]any)
		nodeByID[set["set_id"].(string)] = set
	}

	for _, s := range goSets {
		goSet := s.(map[string]any)
		setID := goSet["set_id"].(string)
		nodeSet, exists := nodeByID[setID]
		require.True(t, exists, "Go has set %s not in Node.js", setID)

		require.Equal(t, nodeSet["name"], goSet["name"], "name mismatch for %s", setID)
	}
}

func TestFavoritesListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/sonos/favorites")
	goResp := fetchJSON(t, GoBaseURL+"/v1/sonos/favorites")

	nodeFavs := extractListData(nodeResp, "favorites")
	goFavs := extractListData(goResp, "favorites")

	require.Equal(t, len(nodeFavs), len(goFavs), "favorites count mismatch")
}

func TestExecutionsListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/executions")
	goResp := fetchJSON(t, GoBaseURL+"/v1/executions")

	nodeExecs := extractListData(nodeResp, "executions")
	goExecs := extractListData(goResp, "executions")

	require.Equal(t, len(nodeExecs), len(goExecs), "executions count mismatch")
}

func TestDashboardParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/dashboard")
	goResp := fetchJSON(t, GoBaseURL+"/v1/dashboard")

	// Both should have these top-level keys (may be nested differently)
	// Just verify the data exists, not exact structure
	require.NotNil(t, nodeResp)
	require.NotNil(t, goResp)
}

func TestSystemInfoParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/system/info")
	goResp := fetchJSON(t, GoBaseURL+"/v1/system/info")

	// Both use hub_version field
	nodeVersion := extractString(nodeResp, "hub_version", "")
	goVersion := extractString(goResp, "hub_version", "")

	// Both should have a version
	require.NotEmpty(t, nodeVersion, "Node.js missing hub_version")
	require.NotEmpty(t, goVersion, "Go missing hub_version")

	// Verify key fields are present in both
	require.NotNil(t, nodeResp["uptime_seconds"], "Node.js missing uptime_seconds")
	require.NotNil(t, goResp["uptime_seconds"], "Go missing uptime_seconds")
	require.NotNil(t, nodeResp["devices_total"], "Node.js missing devices_total")
	require.NotNil(t, goResp["devices_total"], "Go missing devices_total")
}

// ============================================================================
// Simple List Endpoint Tests
// ============================================================================

func TestSceneListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes")
	goResp := fetchJSON(t, GoBaseURL+"/v1/scenes")

	nodeScenes := extractListData(nodeResp, "data")
	goScenes := extractListData(goResp, "data")

	require.Equal(t, len(nodeScenes), len(goScenes), "scene count mismatch")

	if len(nodeScenes) == 0 {
		return
	}

	nodeByID := make(map[string]map[string]any)
	for _, s := range nodeScenes {
		scene := s.(map[string]any)
		nodeByID[scene["scene_id"].(string)] = scene
	}

	for _, s := range goScenes {
		goScene := s.(map[string]any)
		sceneID := goScene["scene_id"].(string)
		nodeScene, exists := nodeByID[sceneID]
		require.True(t, exists, "Go has scene %s not in Node.js", sceneID)

		require.Equal(t, nodeScene["name"], goScene["name"], "name mismatch for %s", sceneID)
	}
}

func TestTemplateListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/routine-templates")
	goResp := fetchJSON(t, GoBaseURL+"/v1/routine-templates")

	nodeTemplates := extractListData(nodeResp, "templates")
	goTemplates := extractListData(goResp, "templates")

	require.Equal(t, len(nodeTemplates), len(goTemplates), "template count mismatch")

	if len(nodeTemplates) == 0 {
		return
	}

	nodeByID := make(map[string]map[string]any)
	for _, tmpl := range nodeTemplates {
		template := tmpl.(map[string]any)
		nodeByID[template["template_id"].(string)] = template
	}

	for _, tmpl := range goTemplates {
		goTemplate := tmpl.(map[string]any)
		templateID := goTemplate["template_id"].(string)
		nodeTemplate, exists := nodeByID[templateID]
		require.True(t, exists, "Go has template %s not in Node.js", templateID)

		require.Equal(t, nodeTemplate["name"], goTemplate["name"], "name mismatch for %s", templateID)
	}
}

func TestMusicProvidersListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/music/providers")
	goResp := fetchJSON(t, GoBaseURL+"/v1/music/providers")

	nodeProviders := extractListData(nodeResp, "providers")
	goProviders := extractListData(goResp, "providers")

	require.Equal(t, len(nodeProviders), len(goProviders), "provider count mismatch")
}

func TestAuditEventsListParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/audit/events")
	goResp := fetchJSON(t, GoBaseURL+"/v1/audit/events")

	nodeEvents := extractListData(nodeResp, "events")
	goEvents := extractListData(goResp, "events")

	require.Equal(t, len(nodeEvents), len(goEvents), "audit event count mismatch")
}

func TestSonosPlayersListParity(t *testing.T) {
	// Endpoint requires device_id parameter
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/sonos/players?device_id="+deviceID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/sonos/players?device_id="+deviceID)

	nodePlayers := extractListData(nodeResp, "players")
	goPlayers := extractListData(goResp, "players")

	require.Equal(t, len(nodePlayers), len(goPlayers), "player count mismatch")
}

// ============================================================================
// Single Resource Endpoint Tests
// ============================================================================

func TestDeviceByIDParity(t *testing.T) {
	// Get a device ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/devices")
	nodeDevices := extractListData(nodeListResp, "devices")
	deviceID := getFirstID(t, nodeDevices, "device_id")

	// Fetch single device from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/devices/"+deviceID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/devices/"+deviceID)

	nodeDevice := extractSingleResource(nodeResp, "device")
	goDevice := extractSingleResource(goResp, "device")

	require.NotNil(t, nodeDevice, "Node.js device is nil")
	require.NotNil(t, goDevice, "Go device is nil")

	require.Equal(t, nodeDevice["device_id"], goDevice["device_id"], "device_id mismatch")
	require.Equal(t, nodeDevice["room_name"], goDevice["room_name"], "room_name mismatch")
	require.Equal(t, nodeDevice["model"], goDevice["model"], "model mismatch")
}

func TestSceneByIDParity(t *testing.T) {
	// Get a scene ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes")
	nodeScenes := extractListData(nodeListResp, "data")
	sceneID := getFirstID(t, nodeScenes, "scene_id")

	// Fetch single scene from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes/"+sceneID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/scenes/"+sceneID)

	nodeScene := extractSingleResource(nodeResp, "data")
	goScene := extractSingleResource(goResp, "scene")

	require.NotNil(t, nodeScene, "Node.js scene is nil")
	require.NotNil(t, goScene, "Go scene is nil")

	require.Equal(t, nodeScene["scene_id"], goScene["scene_id"], "scene_id mismatch")
	require.Equal(t, nodeScene["name"], goScene["name"], "name mismatch")
}

func TestRoutineByIDParity(t *testing.T) {
	// Get a routine ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/routines")
	nodeRoutines := extractListData(nodeListResp, "routines")
	routineID := getFirstID(t, nodeRoutines, "routine_id")

	// Fetch single routine from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/routines/"+routineID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/routines/"+routineID)

	nodeRoutine := extractSingleResource(nodeResp, "routine")
	goRoutine := extractSingleResource(goResp, "routine")

	require.NotNil(t, nodeRoutine, "Node.js routine is nil")
	require.NotNil(t, goRoutine, "Go routine is nil")

	require.Equal(t, nodeRoutine["routine_id"], goRoutine["routine_id"], "routine_id mismatch")
	require.Equal(t, nodeRoutine["name"], goRoutine["name"], "name mismatch")
	require.Equal(t, nodeRoutine["enabled"], goRoutine["enabled"], "enabled mismatch")
}

func TestMusicSetByIDParity(t *testing.T) {
	// Get a music set ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/music/sets")
	nodeSets := extractListData(nodeListResp, "sets")
	setID := getFirstID(t, nodeSets, "set_id")

	// Fetch single set from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/music/sets/"+setID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/music/sets/"+setID)

	nodeSet := extractSingleResource(nodeResp, "set")
	goSet := extractSingleResource(goResp, "set")

	require.NotNil(t, nodeSet, "Node.js set is nil")
	require.NotNil(t, goSet, "Go set is nil")

	require.Equal(t, nodeSet["set_id"], goSet["set_id"], "set_id mismatch")
	require.Equal(t, nodeSet["name"], goSet["name"], "name mismatch")
}

func TestTemplateByIDParity(t *testing.T) {
	// Get a template ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/routine-templates")
	nodeTemplates := extractListData(nodeListResp, "templates")
	templateID := getFirstID(t, nodeTemplates, "template_id")

	// Fetch single template from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/routine-templates/"+templateID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/routine-templates/"+templateID)

	nodeTemplate := extractSingleResource(nodeResp, "template")
	goTemplate := extractSingleResource(goResp, "template")

	require.NotNil(t, nodeTemplate, "Node.js template is nil")
	require.NotNil(t, goTemplate, "Go template is nil")

	require.Equal(t, nodeTemplate["template_id"], goTemplate["template_id"], "template_id mismatch")
	require.Equal(t, nodeTemplate["name"], goTemplate["name"], "name mismatch")
}

func TestAuditEventByIDParity(t *testing.T) {
	// Get an audit event ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/audit/events")
	nodeEvents := extractListData(nodeListResp, "events")
	eventID := getFirstID(t, nodeEvents, "event_id")

	// Fetch single event from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/audit/events/"+eventID)
	goResp := fetchJSON(t, GoBaseURL+"/v1/audit/events/"+eventID)

	nodeEvent := extractSingleResource(nodeResp, "event")
	goEvent := extractSingleResource(goResp, "event")

	require.NotNil(t, nodeEvent, "Node.js event is nil")
	require.NotNil(t, goEvent, "Go event is nil")

	require.Equal(t, nodeEvent["event_id"], goEvent["event_id"], "event_id mismatch")
}

// ============================================================================
// Nested List Endpoint Tests
// ============================================================================

func TestSceneExecutionsParity(t *testing.T) {
	// Get a scene ID from the list
	nodeListResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes")
	nodeScenes := extractListData(nodeListResp, "data")
	sceneID := getFirstID(t, nodeScenes, "scene_id")

	// Fetch executions from both
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/scenes/"+sceneID+"/executions")
	goResp := fetchJSON(t, GoBaseURL+"/v1/scenes/"+sceneID+"/executions")

	nodeExecutions := extractListData(nodeResp, "executions")
	goExecutions := extractListData(goResp, "executions")

	require.Equal(t, len(nodeExecutions), len(goExecutions), "scene executions count mismatch")
}

// ============================================================================
// Status/Info Endpoint Tests
// ============================================================================

func TestHealthParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/health")
	goResp := fetchJSON(t, GoBaseURL+"/v1/health")

	// Both should have a status field
	nodeStatus := extractString(nodeResp, "status", "")
	goStatus := extractString(goResp, "status", "")

	require.NotEmpty(t, nodeStatus, "Node.js missing status")
	require.NotEmpty(t, goStatus, "Go missing status")
}

func TestDeviceTopologyParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/devices/topology")
	goResp := fetchJSON(t, GoBaseURL+"/v1/devices/topology")

	// Extract topology data
	nodeTopology := extractListData(nodeResp, "topology")
	goTopology := extractListData(goResp, "topology")

	require.Equal(t, len(nodeTopology), len(goTopology), "topology count mismatch")
}

func TestDeviceStatsParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/devices/stats")
	goResp := fetchJSON(t, GoBaseURL+"/v1/devices/stats")

	// Extract stats - may be nested under "stats" or at top level
	nodeStats := extractSingleResource(nodeResp, "stats")
	goStats := extractSingleResource(goResp, "stats")
	if nodeStats == nil {
		nodeStats = nodeResp
	}
	if goStats == nil {
		goStats = goResp
	}

	// Both should have total_devices or similar fields
	require.NotNil(t, nodeStats, "Node.js stats response is nil")
	require.NotNil(t, goStats, "Go stats response is nil")
}

func TestTVRoutingSettingsParity(t *testing.T) {
	nodeResp := fetchJSON(t, NodeJSBaseURL+"/v1/settings/tv-routing")
	goResp := fetchJSON(t, GoBaseURL+"/v1/settings/tv-routing")

	// Both should return settings data
	require.NotNil(t, nodeResp, "Node.js settings response is nil")
	require.NotNil(t, goResp, "Go settings response is nil")
}

// ============================================================================
// Device-Dependent Sonos Endpoint Tests
// ============================================================================

func TestSonosPlaybackStateParity(t *testing.T) {
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/playback/state?device_id="+deviceID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/playback/state?device_id="+deviceID)

	// Both should return same status code
	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	require.NotNil(t, nodeResp, "Node.js playback state is nil")
	require.NotNil(t, goResp, "Go playback state is nil")
}

func TestSonosNowPlayingParity(t *testing.T) {
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/playback/now-playing?device_id="+deviceID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/playback/now-playing?device_id="+deviceID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	require.NotNil(t, nodeResp, "Node.js now playing is nil")
	require.NotNil(t, goResp, "Go now playing is nil")
}

func TestSonosGroupsParity(t *testing.T) {
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/groups?device_id="+deviceID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/groups?device_id="+deviceID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	nodeGroups := extractListData(nodeResp, "groups")
	goGroups := extractListData(goResp, "groups")

	require.Equal(t, len(nodeGroups), len(goGroups), "groups count mismatch")
}

func TestSonosAlarmsParity(t *testing.T) {
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/alarms?device_id="+deviceID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/alarms?device_id="+deviceID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	nodeAlarms := extractListData(nodeResp, "alarms")
	goAlarms := extractListData(goResp, "alarms")

	require.Equal(t, len(nodeAlarms), len(goAlarms), "alarms count mismatch")
}

func TestSonosPlayerStateParity(t *testing.T) {
	// Get a device ID first, then get players with that device_id
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodePlayersResp := fetchJSON(t, NodeJSBaseURL+"/v1/sonos/players?device_id="+deviceID)
	nodePlayers := extractListData(nodePlayersResp, "players")
	if len(nodePlayers) == 0 {
		t.Skip("No players available for test")
	}
	// Players have uuid, not player_id
	playerUUID := getFirstID(t, nodePlayers, "uuid")

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/players/"+playerUUID+"/state")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/players/"+playerUUID+"/state")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	require.NotNil(t, nodeResp, "Node.js player state is nil")
	require.NotNil(t, goResp, "Go player state is nil")
}

func TestSonosPlayerTVStatusParity(t *testing.T) {
	// Get a device ID first, then get players with that device_id
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodePlayersResp := fetchJSON(t, NodeJSBaseURL+"/v1/sonos/players?device_id="+deviceID)
	nodePlayers := extractListData(nodePlayersResp, "players")
	if len(nodePlayers) == 0 {
		t.Skip("No players available for test")
	}
	// Players have uuid, not player_id
	playerUUID := getFirstID(t, nodePlayers, "uuid")

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/players/"+playerUUID+"/tv-status")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/players/"+playerUUID+"/tv-status")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status (TV status may not be available)")
	}

	require.NotNil(t, nodeResp, "Node.js TV status is nil")
	require.NotNil(t, goResp, "Go TV status is nil")
}

func TestSonosServicesParity(t *testing.T) {
	deviceID := getFirstDeviceID(t, NodeJSBaseURL)

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos/services?device_id="+deviceID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos/services?device_id="+deviceID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Endpoint returned non-OK status")
	}

	nodeServices := extractListData(nodeResp, "services")
	goServices := extractListData(goResp, "services")

	require.Equal(t, len(nodeServices), len(goServices), "services count mismatch")
}

// ============================================================================
// Sonos Cloud Endpoint Tests
// ============================================================================

func TestSonosCloudStatusParity(t *testing.T) {
	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/auth/status")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos-cloud/auth/status")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Sonos Cloud auth not configured")
	}

	require.NotNil(t, nodeResp, "Node.js auth status is nil")
	require.NotNil(t, goResp, "Go auth status is nil")
}

func TestSonosCloudHouseholdsParity(t *testing.T) {
	// Check auth status first
	_, authStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/auth/status")
	if authStatus != http.StatusOK {
		t.Skip("Sonos Cloud auth not configured")
	}

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/households")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos-cloud/households")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Could not fetch households")
	}

	nodeHouseholds := extractListData(nodeResp, "households")
	goHouseholds := extractListData(goResp, "households")

	require.Equal(t, len(nodeHouseholds), len(goHouseholds), "households count mismatch")
}

func TestSonosCloudGroupsParity(t *testing.T) {
	// Check auth status first
	_, authStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/auth/status")
	if authStatus != http.StatusOK {
		t.Skip("Sonos Cloud auth not configured")
	}

	// Get household ID first
	householdsResp, householdStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/households")
	if householdStatus != http.StatusOK {
		t.Skip("Could not fetch households")
	}
	households := extractListData(householdsResp, "households")
	if len(households) == 0 {
		t.Skip("No households available")
	}
	householdID := getFirstID(t, households, "id")

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/groups?householdId="+householdID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos-cloud/groups?householdId="+householdID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Could not fetch groups")
	}

	nodeGroups := extractListData(nodeResp, "groups")
	goGroups := extractListData(goResp, "groups")

	require.Equal(t, len(nodeGroups), len(goGroups), "groups count mismatch")
}

func TestSonosCloudPlayersParity(t *testing.T) {
	// Check auth status first
	_, authStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/auth/status")
	if authStatus != http.StatusOK {
		t.Skip("Sonos Cloud auth not configured")
	}

	// Get household ID first
	householdsResp, householdStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/households")
	if householdStatus != http.StatusOK {
		t.Skip("Could not fetch households")
	}
	households := extractListData(householdsResp, "households")
	if len(households) == 0 {
		t.Skip("No households available")
	}
	householdID := getFirstID(t, households, "id")

	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/sonos-cloud/players?householdId="+householdID)
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/sonos-cloud/players?householdId="+householdID)

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skip("Could not fetch players")
	}

	nodePlayers := extractListData(nodeResp, "players")
	goPlayers := extractListData(goResp, "players")

	require.Equal(t, len(nodePlayers), len(goPlayers), "players count mismatch")
}

// ============================================================================
// Query Parameter Endpoint Tests
// ============================================================================

func TestMusicSearchParity(t *testing.T) {
	// Node.js requires provider=apple_music and query parameters
	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/music/search?provider=apple_music&query=jazz")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/music/search?provider=apple_music&query=jazz")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skipf("Music search endpoint not available (status: %d)", nodeStatus)
	}

	nodeResults := extractListData(nodeResp, "results")
	goResults := extractListData(goResp, "results")

	require.Equal(t, len(nodeResults), len(goResults), "search results count mismatch")
}

func TestMusicSuggestionsParity(t *testing.T) {
	// Node.js requires provider=apple_music and query parameters
	nodeResp, nodeStatus := fetchJSONWithStatus(t, NodeJSBaseURL+"/v1/music/suggestions?provider=apple_music&query=jazz")
	goResp, goStatus := fetchJSONWithStatus(t, GoBaseURL+"/v1/music/suggestions?provider=apple_music&query=jazz")

	require.Equal(t, nodeStatus, goStatus, "status code mismatch")
	if nodeStatus != http.StatusOK {
		t.Skipf("Music suggestions endpoint not available (status: %d)", nodeStatus)
	}

	nodeSuggestions := extractListData(nodeResp, "suggestions")
	goSuggestions := extractListData(goResp, "suggestions")

	require.Equal(t, len(nodeSuggestions), len(goSuggestions), "suggestions count mismatch")
}

