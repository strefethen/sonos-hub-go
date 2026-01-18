package music

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// rfc3339Millis formats time with milliseconds to match Node.js ISO format
func rfc3339Millis(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// RegisterRoutes wires music catalog routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	// Set CRUD
	router.Method(http.MethodPost, "/v1/music/sets", api.Handler(createSet(service)))
	router.Method(http.MethodGet, "/v1/music/sets", api.Handler(listSets(service)))
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}", api.Handler(getSet(service)))
	router.Method(http.MethodPatch, "/v1/music/sets/{set_id}", api.Handler(updateSet(service)))
	router.Method(http.MethodDelete, "/v1/music/sets/{set_id}", api.Handler(deleteSet(service)))

	// Item management
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/items", api.Handler(addItem(service)))
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}/items", api.Handler(listItems(service)))
	router.Method(http.MethodDelete, "/v1/music/sets/{set_id}/items/{sonos_favorite_id}", api.Handler(removeItem(service)))
	router.Method(http.MethodPut, "/v1/music/sets/{set_id}/items/reorder", api.Handler(reorderItems(service)))

	// History
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}/history", api.Handler(getHistory(service)))

	// Play music set on device
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/play", api.Handler(playSet(service)))

	// Search and suggestions (placeholders)
	router.Method(http.MethodGet, "/v1/music/search", api.Handler(searchMusic(service)))
	router.Method(http.MethodGet, "/v1/music/suggestions", api.Handler(getMusicSuggestions(service)))

	// Providers
	router.Method(http.MethodGet, "/v1/music/providers", api.Handler(listProviders(service)))
}

// createSet handles POST /v1/music/sets
// Returns set at root level matching Node.js format
func createSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var input CreateSetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate required fields
		if input.Name == "" {
			return apperrors.NewValidationError("name is required", nil)
		}

		// Validate selection_policy
		if input.SelectionPolicy == "" {
			return apperrors.NewValidationError("selection_policy is required", nil)
		}
		if input.SelectionPolicy != string(SelectionPolicyRotation) && input.SelectionPolicy != string(SelectionPolicyShuffle) {
			return apperrors.NewValidationError("selection_policy must be ROTATION or SHUFFLE", map[string]any{
				"allowed_values": []string{string(SelectionPolicyRotation), string(SelectionPolicyShuffle)},
			})
		}

		set, err := service.CreateSet(input)
		if err != nil {
			return apperrors.NewInternalError("Failed to create set")
		}

		// Return standardized response with "set" key
		setResponse := map[string]any{
			"set_id":           set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"occasion_start":   set.OccasionStart,
			"occasion_end":     set.OccasionEnd,
			"created_at":       rfc3339Millis(set.CreatedAt),
			"updated_at":       rfc3339Millis(set.UpdatedAt),
		}
		return api.SingleResponse(w, r, http.StatusCreated, "set", setResponse)
	}
}

// listSets handles GET /v1/music/sets
// Node.js returns all sets without pagination
func listSets(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Get all sets (no pagination, matches Node.js)
		sets, _, err := service.ListSets(1000, 0)
		if err != nil {
			return apperrors.NewInternalError("Failed to list sets")
		}

		formatted := make([]map[string]any, 0, len(sets))
		for _, set := range sets {
			formattedSet := formatSet(&set)
			// Enrich with service info from items (preserve insertion order, dedupe)
			var names []string
			var logos []string
			seenNames := make(map[string]bool)
			seenLogos := make(map[string]bool)
			if items, _, err := service.ListItems(set.SetID, 100, 0); err == nil {
				for _, item := range items {
					if item.ServiceName != nil && *item.ServiceName != "" {
						if !seenNames[*item.ServiceName] {
							seenNames[*item.ServiceName] = true
							names = append(names, *item.ServiceName)
						}
					}
					if item.ServiceLogoURL != nil && *item.ServiceLogoURL != "" {
						if !seenLogos[*item.ServiceLogoURL] {
							seenLogos[*item.ServiceLogoURL] = true
							logos = append(logos, *item.ServiceLogoURL)
						}
					}
				}
			}
			// Return null (not empty array) when no items - matches Node.js behavior
			if len(names) > 0 {
				formattedSet["service_names"] = names
			} else {
				formattedSet["service_names"] = nil
			}
			if len(logos) > 0 {
				formattedSet["service_logo_urls"] = logos
			} else {
				formattedSet["service_logo_urls"] = nil
			}
			formatted = append(formatted, formattedSet)
		}

		// Return standardized response with request_id (no pagination for small fixed list)
		return api.ListResponse(w, r, http.StatusOK, "sets", formatted, nil)
	}
}

// getSet handles GET /v1/music/sets/{set_id}
// Returns set with items, matching Node.js format exactly
func getSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		set, err := service.GetSet(setID)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to get set")
		}

		// Get items for this set
		items, _, err := service.ListItems(setID, 1000, 0)
		if err != nil {
			return apperrors.NewInternalError("Failed to get set items")
		}

		// Build service_logo_urls and service_names (preserve order, dedupe)
		var serviceLogoURLs []string
		var serviceNames []string
		seenLogos := make(map[string]bool)
		seenNames := make(map[string]bool)

		formattedItems := make([]map[string]any, 0, len(items))
		for _, item := range items {
			// Track unique service logos and names
			if item.ServiceLogoURL != nil && *item.ServiceLogoURL != "" {
				if !seenLogos[*item.ServiceLogoURL] {
					seenLogos[*item.ServiceLogoURL] = true
					serviceLogoURLs = append(serviceLogoURLs, *item.ServiceLogoURL)
				}
			}
			if item.ServiceName != nil && *item.ServiceName != "" {
				if !seenNames[*item.ServiceName] {
					seenNames[*item.ServiceName] = true
					serviceNames = append(serviceNames, *item.ServiceName)
				}
			}

			// Build music_content from sonos_favorite_id
			musicContent := map[string]any{
				"type":        "sonos_favorite",
				"favorite_id": item.SonosFavoriteID,
			}

			// Format item matching Node.js structure
			formattedItem := map[string]any{
				"sonos_favorite_id": item.SonosFavoriteID,
				"content_type":      item.ContentType,
				"music_content":     musicContent,
				"position":          item.Position,
				"added_at":          rfc3339Millis(item.AddedAt),
				"service_logo_url":  nil,
				"service_name":      nil,
				"display_name":      nil,
				"artwork_url":       nil,
			}
			if item.ServiceLogoURL != nil {
				formattedItem["service_logo_url"] = *item.ServiceLogoURL
			}
			if item.ServiceName != nil {
				formattedItem["service_name"] = *item.ServiceName
			}
			// TODO: display_name and artwork_url would come from favorites lookup

			formattedItems = append(formattedItems, formattedItem)
		}

		// Build standardized response with "set" key
		setResponse := map[string]any{
			"set_id":           set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"occasion_start":   set.OccasionStart,
			"occasion_end":     set.OccasionEnd,
			"created_at":       rfc3339Millis(set.CreatedAt),
			"updated_at":       rfc3339Millis(set.UpdatedAt),
			"items":            formattedItems,
		}

		// Return null instead of empty array for service_logo_urls/service_names
		if len(serviceLogoURLs) > 0 {
			setResponse["service_logo_urls"] = serviceLogoURLs
		} else {
			setResponse["service_logo_urls"] = nil
		}
		if len(serviceNames) > 0 {
			setResponse["service_names"] = serviceNames
		} else {
			setResponse["service_names"] = nil
		}

		return api.SingleResponse(w, r, http.StatusOK, "set", setResponse)
	}
}

// updateSet handles PATCH /v1/music/sets/{set_id}
// Returns set at root level matching Node.js format
func updateSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		var input UpdateSetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate selection_policy if provided
		if input.SelectionPolicy != nil {
			if *input.SelectionPolicy != string(SelectionPolicyRotation) && *input.SelectionPolicy != string(SelectionPolicyShuffle) {
				return apperrors.NewValidationError("selection_policy must be ROTATION or SHUFFLE", map[string]any{
					"allowed_values": []string{string(SelectionPolicyRotation), string(SelectionPolicyShuffle)},
				})
			}
		}

		set, err := service.UpdateSet(setID, input)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to update set")
		}

		// Return standardized response with "set" key
		setResponse := map[string]any{
			"set_id":           set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"occasion_start":   set.OccasionStart,
			"occasion_end":     set.OccasionEnd,
			"created_at":       rfc3339Millis(set.CreatedAt),
			"updated_at":       rfc3339Millis(set.UpdatedAt),
		}
		return api.SingleResponse(w, r, http.StatusOK, "set", setResponse)
	}
}

// deleteSet handles DELETE /v1/music/sets/{set_id}
// Returns 204 No Content with empty body matching Node.js
func deleteSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		err := service.DeleteSet(setID)
		if err != nil {
			// Check for not found error
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to delete set")
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// addItem handles POST /v1/music/sets/{set_id}/items
// Returns item at root level matching Node.js format
func addItem(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		var input AddItemInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate required fields
		if input.SonosFavoriteID == "" {
			return apperrors.NewValidationError("sonos_favorite_id is required", nil)
		}

		item, err := service.AddItem(setID, input)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to add item to set")
		}

		// Build music_content
		musicContent := map[string]any{
			"type":        "sonos_favorite",
			"favorite_id": item.SonosFavoriteID,
		}

		// Return standardized response with "item" key
		itemResponse := map[string]any{
			"set_id":            item.SetID,
			"sonos_favorite_id": item.SonosFavoriteID,
			"content_type":      item.ContentType,
			"music_content":     musicContent,
			"position":          item.Position,
			"added_at":          rfc3339Millis(item.AddedAt),
			"service_logo_url":  nil,
			"service_name":      nil,
			"display_name":      nil,
			"artwork_url":       nil,
		}
		if item.ServiceLogoURL != nil {
			itemResponse["service_logo_url"] = *item.ServiceLogoURL
		}
		if item.ServiceName != nil {
			itemResponse["service_name"] = *item.ServiceName
		}

		return api.SingleResponse(w, r, http.StatusCreated, "item", itemResponse)
	}
}

// listItems handles GET /v1/music/sets/{set_id}/items
func listItems(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		limit := 20
		offset := 0

		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		items, total, err := service.ListItems(setID, limit, offset)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to list items")
		}

		formatted := make([]map[string]any, 0, len(items))
		for _, item := range items {
			formatted = append(formatted, formatItem(&item))
		}

		pagination := &api.Pagination{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: offset+len(items) < total,
		}
		return api.ListResponse(w, r, http.StatusOK, "items", formatted, pagination)
	}
}

// removeItem handles DELETE /v1/music/sets/{set_id}/items/{sonos_favorite_id}
// Returns 204 No Content with empty body matching Node.js
func removeItem(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")
		sonosFavoriteID := chi.URLParam(r, "sonos_favorite_id")

		err := service.RemoveItem(setID, sonosFavoriteID)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			if isItemNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeItemNotFound, "Item not found", 404, map[string]any{
					"set_id":            setID,
					"sonos_favorite_id": sonosFavoriteID,
				}, nil)
			}
			return apperrors.NewInternalError("Failed to remove item from set")
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// reorderItems handles PUT /v1/music/sets/{set_id}/items/reorder
// Note: Go expects {"items": ["id1", "id2"]} while Node.js expects {"positions": [{sonos_favorite_id, position}]}
// Returns { success: true } matching Node.js format
func reorderItems(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		var input ReorderItemsInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate required fields
		if len(input.Items) == 0 {
			return apperrors.NewValidationError("items array is required and must not be empty", nil)
		}

		err := service.ReorderItems(setID, input)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			if isItemNotFoundError(err) {
				return apperrors.NewValidationError("items array contains invalid IDs", map[string]any{
					"set_id": setID,
				})
			}
			return apperrors.NewInternalError("Failed to reorder items")
		}

		// Return standardized action response
		return api.ActionResponse(w, r, http.StatusOK, map[string]any{
			"success": true,
		})
	}
}

// getHistory handles GET /v1/music/sets/{set_id}/history
func getHistory(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		limit := 20
		offset := 0

		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		history, total, err := service.GetHistory(setID, limit, offset)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to get history")
		}

		formatted := make([]map[string]any, 0, len(history))
		for _, h := range history {
			formatted = append(formatted, formatHistory(&h))
		}

		pagination := &api.Pagination{
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: offset+len(history) < total,
		}
		return api.ListResponse(w, r, http.StatusOK, "history", formatted, pagination)
	}
}

// formatSet formats a MusicSet for JSON response.
func formatSet(set *MusicSet) map[string]any {
	result := map[string]any{
		"set_id":           set.SetID,
		"name":             set.Name,
		"selection_policy": set.SelectionPolicy,
		"current_index":    set.CurrentIndex,
		"item_count":       set.ItemCount,
		"occasion_start":   set.OccasionStart,
		"occasion_end":     set.OccasionEnd,
		"created_at":       rfc3339Millis(set.CreatedAt),
		"updated_at":       rfc3339Millis(set.UpdatedAt),
	}
	return result
}

// formatItem formats a SetItem for JSON response.
func formatItem(item *SetItem) map[string]any {
	result := map[string]any{
		"set_id":           item.SetID,
		"sonos_favorite_id": item.SonosFavoriteID,
		"position":         item.Position,
		"content_type":     item.ContentType,
		"added_at":         rfc3339Millis(item.AddedAt),
	}

	if item.ServiceLogoURL != nil {
		result["service_logo_url"] = *item.ServiceLogoURL
	}
	if item.ServiceName != nil {
		result["service_name"] = *item.ServiceName
	}
	if item.ContentJSON != nil {
		result["content_json"] = *item.ContentJSON
	}

	return result
}

// formatHistory formats a PlayHistory for JSON response.
func formatHistory(h *PlayHistory) map[string]any {
	result := map[string]any{
		"id":               h.ID,
		"sonos_favorite_id": h.SonosFavoriteID,
		"played_at":        rfc3339Millis(h.PlayedAt),
	}

	if h.SetID != nil {
		result["set_id"] = *h.SetID
	}
	if h.RoutineID != nil {
		result["routine_id"] = *h.RoutineID
	}

	return result
}

// Error type checking helpers - these should match error types defined in the service layer

// SetNotFoundError represents a set not found error.
type SetNotFoundError struct {
	SetID string
}

func (e *SetNotFoundError) Error() string {
	return "set not found: " + e.SetID
}

// ItemNotFoundError represents an item not found error.
type ItemNotFoundError struct {
	SetID           string
	SonosFavoriteID string
}

func (e *ItemNotFoundError) Error() string {
	return "item not found in set"
}

// EmptySetError represents an attempt to select from an empty set.
type EmptySetError struct {
	SetID string
}

func (e *EmptySetError) Error() string {
	return "set is empty: " + e.SetID
}

// isSetNotFoundError checks if the error is a SetNotFoundError.
func isSetNotFoundError(err error) bool {
	_, ok := err.(*SetNotFoundError)
	return ok
}

// isItemNotFoundError checks if the error is an ItemNotFoundError.
func isItemNotFoundError(err error) bool {
	_, ok := err.(*ItemNotFoundError)
	return ok
}

// isEmptySetError checks if the error is an EmptySetError.
func isEmptySetError(err error) bool {
	_, ok := err.(*EmptySetError)
	return ok
}

// ==========================================================================
// Play Set Handler
// ==========================================================================

// playSet handles POST /v1/music/sets/{set_id}/play
// Mirrors Node.js: selects item, creates ephemeral scene, executes playback
// Note: Scene engine integration not yet implemented - returns stub response
func playSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		var input PlaySetInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Accept either speaker_id (Node.js) or device_id (Go)
		speakerID := input.SpeakerID
		if speakerID == "" {
			speakerID = input.DeviceID
		}
		if speakerID == "" {
			return apperrors.NewValidationError("speaker_id is required", nil)
		}

		// 1. Verify set exists and get items
		_, err := service.GetSet(setID)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to get set")
		}

		items, _, err := service.ListItems(setID, 1000, 0)
		if err != nil {
			return apperrors.NewInternalError("Failed to get set items")
		}
		if len(items) == 0 {
			return apperrors.NewAppError("SET_EMPTY", "Music set has no items to play", 400, nil, nil)
		}

		// 2. Select next content from set using its policy (168 hours = 1 week no-repeat)
		noRepeatHours := 168
		selectionInput := SelectItemInput{NoRepeatWindowMinutes: &noRepeatHours}
		result, err := service.SelectItem(setID, selectionInput)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			if isEmptySetError(err) {
				return apperrors.NewAppError("SELECTION_FAILED", "Could not select music from set", 400, nil, nil)
			}
			return apperrors.NewInternalError("Failed to select item")
		}

		// Build music_content for response
		var musicContent map[string]any
		if result.Item != nil {
			musicContent = map[string]any{
				"type":        "sonos_favorite",
				"favorite_id": result.Item.SonosFavoriteID,
			}
		}

		// Note: Scene engine integration not yet implemented
		// Node.js would:
		// 3. Create ephemeral scene with name `Play: ${set.name}`
		// 4. Execute scene with musicContent and favoriteId
		// 5. Schedule cleanup of ephemeral scene after 5 minutes
		// For now, return stub response with standardized format

		return api.ActionResponse(w, r, http.StatusOK, map[string]any{
			"scene_id":           nil, // Would be created scene ID
			"scene_execution_id": nil, // Would be execution ID
			"music_content":      musicContent,
			"status":             "playing",
		})
	}
}

// ==========================================================================
// Search Handler (Placeholder)
// ==========================================================================

// searchMusic handles GET /v1/music/search
// Mirrors Node.js music-search.ts format
func searchMusic(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		query := r.URL.Query().Get("query")
		if query == "" {
			query = r.URL.Query().Get("q") // Fallback for legacy parameter
		}
		provider := r.URL.Query().Get("provider")

		// Parse limit and offset
		limit := 25
		offset := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		// Validate provider
		if provider == "" {
			return apperrors.NewValidationError("provider is required", map[string]any{
				"supported_providers": []string{"apple_music", "library"},
			})
		}

		// Apple Music search not yet implemented - return proper format with empty results
		// This matches Node.js music-search.ts response format, with standardized envelope
		return api.SingleResponse(w, r, http.StatusOK, "search", map[string]any{
			"provider": provider,
			"query":    query,
			"results":  map[string][]any{}, // Empty results grouped by type
			"pagination": map[string]any{
				"limit":  limit,
				"offset": offset,
				"total":  0,
			},
		})
	}
}

// ==========================================================================
// Suggestions Handler (Placeholder)
// ==========================================================================

// getMusicSuggestions handles GET /v1/music/suggestions
// Mirrors Node.js music-search.ts suggestions format
func getMusicSuggestions(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		query := r.URL.Query().Get("query")
		if query == "" {
			query = r.URL.Query().Get("q") // Fallback
		}
		provider := r.URL.Query().Get("provider")

		// Validate provider - only apple_music supports suggestions
		if provider == "" {
			return apperrors.NewValidationError("provider is required", nil)
		}
		if provider != "apple_music" {
			return apperrors.NewValidationError("provider does not support suggestions", map[string]any{
				"provider":            provider,
				"supported_providers": []string{"apple_music"},
			})
		}

		// Apple Music suggestions not yet implemented - return proper format
		// Matches Node.js music-search.ts suggestions response, with standardized envelope
		return api.SingleResponse(w, r, http.StatusOK, "suggestions", map[string]any{
			"provider":    provider,
			"query":       query,
			"terms":       []map[string]string{}, // Empty terms array
			"top_results": []map[string]any{},    // Empty top results
		})
	}
}

// ==========================================================================
// Providers Handler
// ==========================================================================

// MusicProvider represents a music service provider.
type MusicProvider struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	RequiresAuth bool  `json:"requires_auth"`
}

// listProviders handles GET /v1/music/providers
// Mirrors Node.js music-search.ts providers format
func listProviders(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Return available search providers
		// Matches Node.js music-search.ts /v1/music/providers response, with standardized envelope
		providers := []map[string]any{
			{
				"name":                 "apple_music",
				"display_name":         "Apple Music",
				"supported_types":      []string{"albums", "artists", "tracks", "playlists", "stations"},
				"supports_suggestions": true,
			},
			{
				"name":                 "library",
				"display_name":         "Sonos Music Library",
				"supported_types":      []string{"albums", "artists", "tracks", "genres"},
				"supports_suggestions": false,
			},
		}

		// Small fixed list - no pagination needed
		return api.ListResponse(w, r, http.StatusOK, "providers", providers, nil)
	}
}
