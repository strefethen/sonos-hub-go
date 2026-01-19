package phase6

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stripe-style: single resource returned directly with object field
type setResponse map[string]any

// Stripe-style list response for sets
type listSetsResponse struct {
	Object  string           `json:"object"`
	Data    []map[string]any `json:"data"`
	HasMore bool             `json:"has_more"`
	URL     string           `json:"url"`
}

// Stripe-style list response for items
type listItemsResponse struct {
	Object  string           `json:"object"`
	Data    []map[string]any `json:"data"`
	HasMore bool             `json:"has_more"`
	URL     string           `json:"url"`
}

// Stripe-style: single item returned directly with object field
type itemResponse map[string]any

// Note: actionResponse is defined in phase6_test.go (shared across package)

// ==========================================================================
// Set CRUD Tests
// ==========================================================================

func TestMusicSetCRUD(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set with ROTATION policy
	createPayload := map[string]any{
		"name":             "Morning Playlist",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	// Stripe-style: resource returned directly with object field
	require.Equal(t, "music_set", createResp["object"])
	require.NotEmpty(t, createResp["id"])
	require.Equal(t, "Morning Playlist", createResp["name"])
	require.Equal(t, "ROTATION", createResp["selection_policy"])
	require.Equal(t, float64(0), createResp["current_index"])

	setID := createResp["id"].(string)

	// Get set
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var getResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&getResp))
	resp.Body.Close()

	require.Equal(t, setID, getResp["id"])
	require.Equal(t, "Morning Playlist", getResp["name"])

	// Update set - change name
	updatePayload := map[string]any{
		"name": "Evening Playlist",
	}
	resp = doRequest(t, http.MethodPatch, ts.URL+"/v1/music/sets/"+setID, updatePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var updateResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updateResp))
	resp.Body.Close()

	require.Equal(t, "Evening Playlist", updateResp["name"])
	require.Equal(t, "ROTATION", updateResp["selection_policy"]) // Unchanged

	// Update set - change policy
	updatePayload = map[string]any{
		"selection_policy": "SHUFFLE",
	}
	resp = doRequest(t, http.MethodPatch, ts.URL+"/v1/music/sets/"+setID, updatePayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&updateResp))
	resp.Body.Close()

	require.Equal(t, "Evening Playlist", updateResp["name"])       // Unchanged
	require.Equal(t, "SHUFFLE", updateResp["selection_policy"]) // Changed

	// Delete set
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/music/sets/"+setID, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify deleted
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestMusicSetCreateWithShufflePolicy(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set with SHUFFLE policy
	createPayload := map[string]any{
		"name":             "Shuffle Playlist",
		"selection_policy": "SHUFFLE",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	require.Equal(t, "Shuffle Playlist", createResp["name"])
	require.Equal(t, "SHUFFLE", createResp["selection_policy"])
}

func TestMusicSetList(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create 5 sets
	for i := 0; i < 5; i++ {
		createPayload := map[string]any{
			"name":             "Playlist " + string(rune('A'+i)),
			"selection_policy": "ROTATION",
		}
		resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// List all sets (Stripe-style list response)
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listSetsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	// Should return all 5 sets
	require.Equal(t, "list", listResp.Object)
	require.Len(t, listResp.Data, 5)

	// Verify each set has expected fields
	for _, set := range listResp.Data {
		require.NotEmpty(t, set["id"])
		require.NotEmpty(t, set["name"])
		require.NotEmpty(t, set["selection_policy"])
	}
}

// ==========================================================================
// Item Management Tests
// ==========================================================================

func TestMusicSetItemManagement(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Item Test Playlist",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Add item
	addItemPayload := map[string]any{
		"sonos_favorite_id": "favorite-001",
		"content_type":      "sonos_favorite",
	}
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+setID+"/items", addItemPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var addItemResp itemResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&addItemResp))
	resp.Body.Close()

	require.Equal(t, "set_item", addItemResp["object"])
	require.Equal(t, setID, addItemResp["set_id"])
	require.Equal(t, "favorite-001", addItemResp["sonos_favorite_id"])
	require.Equal(t, float64(0), addItemResp["position"])

	// Get items
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID+"/items", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listItemsResp listItemsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listItemsResp))
	resp.Body.Close()

	require.Equal(t, "list", listItemsResp.Object)
	require.Len(t, listItemsResp.Data, 1)
	require.Equal(t, "favorite-001", listItemsResp.Data[0]["sonos_favorite_id"])

	// Add multiple items
	for i := 2; i <= 4; i++ {
		addItemPayload = map[string]any{
			"sonos_favorite_id": "favorite-00" + string(rune('0'+i)),
			"content_type":      "sonos_favorite",
		}
		resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+setID+"/items", addItemPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Verify all items
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID+"/items", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listItemsResp))
	resp.Body.Close()

	require.Len(t, listItemsResp.Data, 4)

	// Remove item
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/music/sets/"+setID+"/items/favorite-002", nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// Verify item removed
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID+"/items", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listItemsResp))
	resp.Body.Close()

	require.Len(t, listItemsResp.Data, 3)
}

func TestMusicSetItemReorder(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Reorder Test Playlist",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Add items
	items := []string{"favorite-A", "favorite-B", "favorite-C"}
	for _, fav := range items {
		addItemPayload := map[string]any{
			"sonos_favorite_id": fav,
			"content_type":      "sonos_favorite",
		}
		resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+setID+"/items", addItemPayload)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		resp.Body.Close()
	}

	// Reorder items (reverse order) - uses PUT, not POST
	reorderPayload := map[string]any{
		"items": []string{"favorite-C", "favorite-B", "favorite-A"},
	}
	resp = doRequest(t, http.MethodPut, ts.URL+"/v1/music/sets/"+setID+"/items/reorder", reorderPayload)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Reorder returns Stripe-style response with object field
	var reorderResp actionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reorderResp))
	resp.Body.Close()

	require.Equal(t, "reorder", reorderResp["object"])
	require.True(t, reorderResp["success"].(bool))

	// Verify new order by fetching items
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID+"/items", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var listResp listItemsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&listResp))
	resp.Body.Close()

	require.Len(t, listResp.Data, 3)
	require.Equal(t, "favorite-C", listResp.Data[0]["sonos_favorite_id"])
	require.Equal(t, "favorite-B", listResp.Data[1]["sonos_favorite_id"])
	require.Equal(t, "favorite-A", listResp.Data[2]["sonos_favorite_id"])
}

// ==========================================================================
// Play History Tests
// ==========================================================================

func TestMusicSetPlayHistory(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "History Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Get history for set (should be empty initially)
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+setID+"/history", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var historyResp listItemsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&historyResp))
	resp.Body.Close()

	require.Equal(t, "list", historyResp.Object)
	require.Len(t, historyResp.Data, 0)
}

// ==========================================================================
// Error Cases Tests
// ==========================================================================

func TestMusicSetNotFound(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Get non-existent set - should return 404
	resp := doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/nonexistent-id", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "SET_NOT_FOUND", errorData["code"])
}

func TestMusicSetItemNotFound(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Item Error Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Try to remove non-existent item - should return 404
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/music/sets/"+setID+"/items/nonexistent-fav", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "ITEM_NOT_FOUND", errorData["code"])
}

func TestMusicSetInvalidSelectionPolicy(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set with invalid selection_policy - should return 400
	createPayload := map[string]any{
		"name":             "Invalid Policy Test",
		"selection_policy": "INVALID_POLICY",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
}

func TestMusicSetMissingRequiredFields(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set without name - should return 400
	createPayload := map[string]any{
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Create set without selection_policy - should return 400
	createPayload = map[string]any{
		"name": "Test",
	}
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()

	// Invalid JSON body
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/music/sets", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	resp, _ = http.DefaultClient.Do(req)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestMusicSetUpdateInvalidPolicy(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create valid set
	createPayload := map[string]any{
		"name":             "Update Policy Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Update with invalid selection_policy - should return 400
	updatePayload := map[string]any{
		"selection_policy": "INVALID_POLICY",
	}
	resp = doRequest(t, http.MethodPatch, ts.URL+"/v1/music/sets/"+setID, updatePayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
}

func TestMusicSetReorderWithInvalidItems(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Reorder Error Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Add one item
	addItemPayload := map[string]any{
		"sonos_favorite_id": "fav-1",
		"content_type":      "sonos_favorite",
	}
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+setID+"/items", addItemPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	// Try to reorder with invalid item ID - should return 400
	reorderPayload := map[string]any{
		"items": []string{"fav-1", "nonexistent-fav"},
	}
	resp = doRequest(t, http.MethodPut, ts.URL+"/v1/music/sets/"+setID+"/items/reorder", reorderPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
}

func TestMusicSetReorderEmptyItems(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Reorder Empty Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Try to reorder with empty items - should return 400
	reorderPayload := map[string]any{
		"items": []string{},
	}
	resp = doRequest(t, http.MethodPut, ts.URL+"/v1/music/sets/"+setID+"/items/reorder", reorderPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

func TestMusicSetAddItemMissingFields(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create set
	createPayload := map[string]any{
		"name":             "Add Item Error Test",
		"selection_policy": "ROTATION",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets", createPayload)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp setResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&createResp))
	resp.Body.Close()

	setID := createResp["id"].(string)

	// Try to add item without sonos_favorite_id - should return 400
	addItemPayload := map[string]any{
		"content_type": "sonos_favorite",
	}
	resp = doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+setID+"/items", addItemPayload)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	resp.Body.Close()

	errorData := errResp["error"].(map[string]any)
	require.Equal(t, "VALIDATION_ERROR", errorData["code"])
}

func TestMusicSetOperationsOnNonExistentSet(t *testing.T) {
	ts, cleanup := setupTestServer(t)
	defer cleanup()

	nonExistentID := "nonexistent-set-id"

	// Add item to non-existent set - should return 404
	addItemPayload := map[string]any{
		"sonos_favorite_id": "fav-1",
		"content_type":      "sonos_favorite",
	}
	resp := doRequest(t, http.MethodPost, ts.URL+"/v1/music/sets/"+nonExistentID+"/items", addItemPayload)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// List items from non-existent set - should return 404
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+nonExistentID+"/items", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Delete item from non-existent set - should return 404
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/music/sets/"+nonExistentID+"/items/fav-1", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Get history from non-existent set - should return 404
	resp = doRequest(t, http.MethodGet, ts.URL+"/v1/music/sets/"+nonExistentID+"/history", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Reorder items in non-existent set - should return 404
	reorderPayload := map[string]any{
		"items": []string{"fav-1"},
	}
	resp = doRequest(t, http.MethodPut, ts.URL+"/v1/music/sets/"+nonExistentID+"/items/reorder", reorderPayload)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Update non-existent set - should return 404
	updatePayload := map[string]any{
		"name": "New Name",
	}
	resp = doRequest(t, http.MethodPatch, ts.URL+"/v1/music/sets/"+nonExistentID, updatePayload)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// Delete non-existent set - should return 404
	resp = doRequest(t, http.MethodDelete, ts.URL+"/v1/music/sets/"+nonExistentID, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}
