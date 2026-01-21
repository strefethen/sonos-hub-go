package music

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
	"github.com/strefethen/sonos-hub-go/internal/applemusic"
	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
	"github.com/strefethen/sonos-hub-go/internal/spotifysearch"
)

// RegisterRoutes wires music catalog routes to the router.
// spotifyManager is optional - if nil, Spotify search will return 503.
// appleClient is optional - if nil, Apple Music search will return 503.
// soapClient and deviceService are optional - if nil, library search will return empty results.
func RegisterRoutes(router chi.Router, service *Service, spotifyManager *spotifysearch.ConnectionManager, appleClient *applemusic.Client, soapClient *soap.Client, deviceService *devices.Service) {
	// Create library provider if dependencies are available
	var libraryProvider *LibraryProvider
	if soapClient != nil && deviceService != nil {
		libraryProvider = NewLibraryProvider(soapClient, deviceService)
	}
	// Set CRUD
	router.Method(http.MethodPost, "/v1/music/sets", api.Handler(createSet(service)))
	router.Method(http.MethodGet, "/v1/music/sets", api.Handler(listSets(service)))
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}", api.Handler(getSet(service)))
	router.Method(http.MethodPatch, "/v1/music/sets/{set_id}", api.Handler(updateSet(service)))
	router.Method(http.MethodDelete, "/v1/music/sets/{set_id}", api.Handler(deleteSet(service)))
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/restore", api.Handler(restoreSet(service)))

	// Item management
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/items", api.Handler(addItem(service)))
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}/items", api.Handler(listItems(service)))
	router.Method(http.MethodDelete, "/v1/music/sets/{set_id}/items/{sonos_favorite_id}", api.Handler(removeItem(service)))
	router.Method(http.MethodPut, "/v1/music/sets/{set_id}/items/reorder", api.Handler(reorderItems(service)))

	// History
	router.Method(http.MethodGet, "/v1/music/sets/{set_id}/history", api.Handler(getHistory(service)))

	// Content management (iOS app format)
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/content", api.Handler(addContent(service)))
	router.Method(http.MethodDelete, "/v1/music/sets/{set_id}/content/{position}", api.Handler(removeContentByPosition(service)))

	// Play music set on device
	router.Method(http.MethodPost, "/v1/music/sets/{set_id}/play", api.Handler(playSet(service)))

	// Search and suggestions
	router.Method(http.MethodGet, "/v1/music/search", api.Handler(searchMusic(spotifyManager, appleClient, libraryProvider)))
	router.Method(http.MethodGet, "/v1/music/suggestions", api.Handler(getMusicSuggestions(appleClient)))

	// Providers
	router.Method(http.MethodGet, "/v1/music/providers", api.Handler(listProviders(service, spotifyManager, appleClient)))
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

		// Stripe-style: return resource directly
		setResponse := map[string]any{
			"object":           api.ObjectMusicSet,
			"id":               set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"created_at":       api.RFC3339Millis(set.CreatedAt),
			"updated_at":       api.RFC3339Millis(set.UpdatedAt),
		}
		// Only include occasion fields when they have actual values
		if set.OccasionStart != nil && *set.OccasionStart != "" {
			setResponse["occasion_start"] = *set.OccasionStart
		}
		if set.OccasionEnd != nil && *set.OccasionEnd != "" {
			setResponse["occasion_end"] = *set.OccasionEnd
		}
		return api.WriteResource(w, http.StatusCreated, setResponse)
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
				// If set has no artwork, use first item's artwork
				if formattedSet["artwork_url"] == nil && len(items) > 0 {
					firstItem := items[0]
					if firstItem.ArtworkURL != nil && *firstItem.ArtworkURL != "" {
						formattedSet["artwork_url"] = *firstItem.ArtworkURL
					} else if firstItem.ContentJSON != nil && *firstItem.ContentJSON != "" {
						var metadata ContentMetadata
						if err := json.Unmarshal([]byte(*firstItem.ContentJSON), &metadata); err == nil && metadata.ArtworkURL != "" {
							formattedSet["artwork_url"] = metadata.ArtworkURL
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

		// Stripe-style list response (no pagination for small fixed list)
		return api.WriteList(w, "/v1/music/sets", formatted, false)
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

			// Build music_content from stored content_json (preserves title, service, etc.)
			var musicContent map[string]any
			if item.ContentJSON != nil && *item.ContentJSON != "" {
				if err := json.Unmarshal([]byte(*item.ContentJSON), &musicContent); err != nil {
					// Fallback to sonos_favorite structure on parse error
					musicContent = map[string]any{
						"type":        "sonos_favorite",
						"favorite_id": item.SonosFavoriteID,
					}
				}
			} else {
				// No content_json, build basic sonos_favorite
				musicContent = map[string]any{
					"type":        "sonos_favorite",
					"favorite_id": item.SonosFavoriteID,
				}
			}

			// Format item matching Node.js structure
			formattedItem := map[string]any{
				"sonos_favorite_id": item.SonosFavoriteID,
				"content_type":      item.ContentType,
				"music_content":     musicContent,
				"position":          item.Position,
				"added_at":          api.RFC3339Millis(item.AddedAt),
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
			if item.ArtworkURL != nil {
				formattedItem["artwork_url"] = *item.ArtworkURL
			}
			// Populate display_name from item or extract from content_json
			if item.DisplayName != nil {
				formattedItem["display_name"] = *item.DisplayName
			} else if musicContent != nil {
				// Try to extract title/name from content_json as fallback
				if title, ok := musicContent["title"].(string); ok && title != "" {
					formattedItem["display_name"] = title
				} else if name, ok := musicContent["name"].(string); ok && name != "" {
					formattedItem["display_name"] = name
				}
			}

			formattedItems = append(formattedItems, formattedItem)
		}

		// Stripe-style: return resource directly
		setResponse := map[string]any{
			"object":           api.ObjectMusicSet,
			"id":               set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"created_at":       api.RFC3339Millis(set.CreatedAt),
			"updated_at":       api.RFC3339Millis(set.UpdatedAt),
			"items":            formattedItems,
		}
		// Only include occasion fields when they have actual values
		if set.OccasionStart != nil && *set.OccasionStart != "" {
			setResponse["occasion_start"] = *set.OccasionStart
		}
		if set.OccasionEnd != nil && *set.OccasionEnd != "" {
			setResponse["occasion_end"] = *set.OccasionEnd
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

		return api.WriteResource(w, http.StatusOK, setResponse)
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

		// Stripe-style: return resource directly
		setResponse := map[string]any{
			"object":           api.ObjectMusicSet,
			"id":               set.SetID,
			"name":             set.Name,
			"selection_policy": set.SelectionPolicy,
			"current_index":    set.CurrentIndex,
			"created_at":       api.RFC3339Millis(set.CreatedAt),
			"updated_at":       api.RFC3339Millis(set.UpdatedAt),
		}
		// Only include occasion fields when they have actual values
		if set.OccasionStart != nil && *set.OccasionStart != "" {
			setResponse["occasion_start"] = *set.OccasionStart
		}
		if set.OccasionEnd != nil && *set.OccasionEnd != "" {
			setResponse["occasion_end"] = *set.OccasionEnd
		}
		return api.WriteResource(w, http.StatusOK, setResponse)
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

// restoreSet handles POST /v1/music/sets/{set_id}/restore
// Restores a soft-deleted music set
func restoreSet(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		// Check if set exists and get its deletion state
		set, isDeleted, err := service.GetSetIncludingDeleted(setID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get set")
		}
		if set == nil {
			// Never existed or already hard-deleted by cleanup
			return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
		}
		if !isDeleted {
			// Set exists but is not deleted (409 Conflict)
			return apperrors.NewConflictError("Set is not deleted", map[string]any{"set_id": setID})
		}

		// Restore the set
		restoredSet, err := service.RestoreSet(setID)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to restore set")
		}

		// Stripe-style: return resource directly
		setResponse := map[string]any{
			"object":           api.ObjectMusicSet,
			"id":               restoredSet.SetID,
			"name":             restoredSet.Name,
			"selection_policy": restoredSet.SelectionPolicy,
			"current_index":    restoredSet.CurrentIndex,
			"created_at":       api.RFC3339Millis(restoredSet.CreatedAt),
			"updated_at":       api.RFC3339Millis(restoredSet.UpdatedAt),
		}
		if restoredSet.OccasionStart != nil && *restoredSet.OccasionStart != "" {
			setResponse["occasion_start"] = *restoredSet.OccasionStart
		}
		if restoredSet.OccasionEnd != nil && *restoredSet.OccasionEnd != "" {
			setResponse["occasion_end"] = *restoredSet.OccasionEnd
		}
		return api.WriteResource(w, http.StatusOK, setResponse)
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

		// Stripe-style: return resource directly
		itemResponse := map[string]any{
			"object":            api.ObjectSetItem,
			"set_id":            item.SetID,
			"sonos_favorite_id": item.SonosFavoriteID,
			"content_type":      item.ContentType,
			"music_content":     musicContent,
			"position":          item.Position,
			"added_at":          api.RFC3339Millis(item.AddedAt),
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
		if item.ArtworkURL != nil {
			itemResponse["artwork_url"] = *item.ArtworkURL
		}
		if item.DisplayName != nil {
			itemResponse["display_name"] = *item.DisplayName
		}

		return api.WriteResource(w, http.StatusCreated, itemResponse)
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

		hasMore := offset+len(items) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/music/sets/"+setID+"/items", formatted, hasMore)
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

		// Stripe-style: return action result directly
		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":  "reorder",
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

		hasMore := offset+len(history) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/music/sets/"+setID+"/history", formatted, hasMore)
	}
}

// formatSet formats a MusicSet for JSON response.
func formatSet(set *MusicSet) map[string]any {
	result := map[string]any{
		"object":           api.ObjectMusicSet,
		"id":               set.SetID,
		"name":             set.Name,
		"selection_policy": set.SelectionPolicy,
		"current_index":    set.CurrentIndex,
		"item_count":       set.ItemCount,
		"created_at":       api.RFC3339Millis(set.CreatedAt),
		"updated_at":       api.RFC3339Millis(set.UpdatedAt),
	}
	// Only include occasion fields when they have actual values
	// Empty strings should become null/omitted for iOS filtering
	if set.OccasionStart != nil && *set.OccasionStart != "" {
		result["occasion_start"] = *set.OccasionStart
	}
	if set.OccasionEnd != nil && *set.OccasionEnd != "" {
		result["occasion_end"] = *set.OccasionEnd
	}
	if set.ArtworkURL != nil {
		result["artwork_url"] = *set.ArtworkURL
	}
	return result
}

// formatItem formats a SetItem for JSON response.
// Note: SetItem uses a compound key (set_id + position), so set_id remains as-is.
func formatItem(item *SetItem) map[string]any {
	result := map[string]any{
		"object":            api.ObjectSetItem,
		"set_id":            item.SetID,
		"sonos_favorite_id": item.SonosFavoriteID,
		"position":          item.Position,
		"content_type":      item.ContentType,
		"added_at":          api.RFC3339Millis(item.AddedAt),
	}

	if item.ServiceLogoURL != nil {
		result["service_logo_url"] = *item.ServiceLogoURL
	}
	if item.ServiceName != nil {
		result["service_name"] = *item.ServiceName
	}
	if item.ArtworkURL != nil {
		result["artwork_url"] = *item.ArtworkURL
	}
	if item.DisplayName != nil {
		result["display_name"] = *item.DisplayName
	}
	if item.ContentJSON != nil {
		result["content_json"] = *item.ContentJSON
	}

	return result
}

// formatHistory formats a PlayHistory for JSON response.
func formatHistory(h *PlayHistory) map[string]any {
	result := map[string]any{
		"object":            api.ObjectPlayHistory,
		"id":                h.ID,
		"sonos_favorite_id": h.SonosFavoriteID,
		"played_at":         api.RFC3339Millis(h.PlayedAt),
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

// PositionNotFoundError represents an item not found at a specific position.
type PositionNotFoundError struct {
	SetID    string
	Position int
}

func (e *PositionNotFoundError) Error() string {
	return "item not found at position"
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

// isPositionNotFoundError checks if the error is a PositionNotFoundError.
func isPositionNotFoundError(err error) bool {
	_, ok := err.(*PositionNotFoundError)
	return ok
}

// getContentType extracts the actual content type from MusicContent.
// For "direct" type content (Spotify, etc.), the actual content type (playlist, album, podcast, etc.)
// is stored in ContentType field. For backwards compatibility, falls back to Type discriminator.
func getContentType(mc MusicContent) string {
	if mc.ContentType != nil && *mc.ContentType != "" {
		return *mc.ContentType // Use actual content type (podcast, playlist, album, etc.)
	}
	return mc.Type // Fallback to discriminator for backwards compatibility
}

// ==========================================================================
// Content Management Handlers (iOS app format)
// ==========================================================================

// addContent handles POST /v1/music/sets/{set_id}/content
// This endpoint uses the MusicContent format expected by the iOS app.
func addContent(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")

		var input AddContentInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate music_content
		if input.MusicContent.Type == "" {
			return apperrors.NewValidationError("music_content.type is required", nil)
		}

		// For sonos_favorite type, require favorite_id
		if input.MusicContent.Type == "sonos_favorite" && (input.MusicContent.FavoriteID == nil || *input.MusicContent.FavoriteID == "") {
			return apperrors.NewValidationError("music_content.favorite_id is required for sonos_favorite type", nil)
		}

		// Build sonos_favorite_id from the music content
		// For sonos favorites, use the favorite_id directly
		// For other types, construct a unique identifier
		var sonosFavoriteID string
		if input.MusicContent.Type == "sonos_favorite" && input.MusicContent.FavoriteID != nil {
			sonosFavoriteID = *input.MusicContent.FavoriteID
		} else if input.MusicContent.ContentID != nil {
			// For streaming services, use service:content_id format
			serviceName := "unknown"
			if input.MusicContent.Service != nil {
				serviceName = *input.MusicContent.Service
			}
			sonosFavoriteID = serviceName + ":" + *input.MusicContent.ContentID
		} else {
			return apperrors.NewValidationError("music_content must have favorite_id or content_id", nil)
		}

		// Build content_json to store the full MusicContent
		contentJSON, err := json.Marshal(input.MusicContent)
		if err != nil {
			return apperrors.NewInternalError("Failed to serialize music content")
		}
		contentJSONStr := string(contentJSON)

		// Create AddItemInput from the content
		addInput := AddItemInput{
			SonosFavoriteID: sonosFavoriteID,
			ServiceLogoURL:  input.ServiceLogoURL,
			ServiceName:     input.ServiceName,
			ArtworkURL:      input.ArtworkURL,      // CRITICAL: Pass artwork_url to storage
			DisplayName:     input.DisplayName,     // CRITICAL: Pass display_name to storage
			ContentType:     getContentType(input.MusicContent),
			ContentJSON:     &contentJSONStr,
		}

		item, err := service.AddItem(setID, addInput)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			return apperrors.NewInternalError("Failed to add content to set")
		}

		// Stripe-style: return resource directly
		itemResponse := map[string]any{
			"object":            api.ObjectSetItem,
			"set_id":            item.SetID,
			"sonos_favorite_id": item.SonosFavoriteID,
			"content_type":      item.ContentType,
			"music_content":     input.MusicContent,
			"position":          item.Position,
			"added_at":          api.RFC3339Millis(item.AddedAt),
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
		if item.ArtworkURL != nil {
			itemResponse["artwork_url"] = *item.ArtworkURL
		}
		if item.DisplayName != nil {
			itemResponse["display_name"] = *item.DisplayName
		}

		return api.WriteResource(w, http.StatusCreated, itemResponse)
	}
}

// removeContentByPosition handles DELETE /v1/music/sets/{set_id}/content/{position}
// Removes an item from a music set by its position (0-indexed).
func removeContentByPosition(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		setID := chi.URLParam(r, "set_id")
		positionStr := chi.URLParam(r, "position")

		position, err := strconv.Atoi(positionStr)
		if err != nil || position < 0 {
			return apperrors.NewValidationError("position must be a non-negative integer", map[string]any{
				"provided": positionStr,
			})
		}

		err = service.RemoveItemByPosition(setID, position)
		if err != nil {
			if isSetNotFoundError(err) {
				return apperrors.NewAppError(apperrors.ErrorCodeSetNotFound, "Set not found", 404, map[string]any{"set_id": setID}, nil)
			}
			if isPositionNotFoundError(err) {
				return apperrors.NewAppError("ITEM_NOT_FOUND", "Item not found at position", 404, map[string]any{
					"set_id":   setID,
					"position": position,
				}, nil)
			}
			return apperrors.NewInternalError("Failed to remove content from set")
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
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

		// Accept either speaker_id (Node.js) or udn (Go)
		speakerID := input.SpeakerID
		if speakerID == "" {
			speakerID = input.UDN
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
		// For now, return stub response with Stripe-style format

		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":             "play_set",
			"scene_id":           nil, // Would be created scene ID
			"scene_execution_id": nil, // Would be execution ID
			"music_content":      musicContent,
			"status":             "playing",
		})
	}
}

// ==========================================================================
// Search Handler
// ==========================================================================

// searchMusic handles GET /v1/music/search
// Mirrors Node.js music-search.ts format
func searchMusic(spotifyManager *spotifysearch.ConnectionManager, appleClient *applemusic.Client, libraryProvider *LibraryProvider) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		query := r.URL.Query().Get("query")
		if query == "" {
			query = r.URL.Query().Get("q") // Fallback for legacy parameter
		}
		provider := r.URL.Query().Get("provider")
		typesParam := r.URL.Query().Get("types") // comma-separated content types

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
				"supported_providers": []string{"apple_music", "library", "spotify"},
			})
		}

		// Handle Spotify search via WebSocket extension
		if provider == "spotify" {
			if spotifyManager == nil || !spotifyManager.IsConnected() {
				return apperrors.NewAppError("SERVICE_UNAVAILABLE", "Spotify search extension not connected", 503, nil, nil)
			}

			if query == "" {
				return apperrors.NewValidationError("query is required for spotify search", nil)
			}

			// Parse content types from request
			contentTypes := spotifysearch.AllContentTypes()
			if typesParam != "" {
				contentTypes = nil
				for _, t := range strings.Split(typesParam, ",") {
					t = strings.TrimSpace(t)
					contentTypes = append(contentTypes, spotifysearch.SpotifyContentType(t))
				}
			}

			// Perform search via extension
			results, err := spotifyManager.Search(r.Context(), query, contentTypes)
			if err != nil {
				if err == spotifysearch.ErrExtensionNotConnected {
					return apperrors.NewAppError("SERVICE_UNAVAILABLE", "Spotify search extension not connected", 503, nil, nil)
				}
				if err == spotifysearch.ErrSearchTimeout {
					return apperrors.NewAppError("SEARCH_TIMEOUT", "Spotify search timed out", 504, nil, nil)
				}
				return apperrors.NewInternalError("Spotify search failed")
			}

			// Convert to API response format (snake_case)
			resultsMap := make(map[string]any)
			if results.Tracks != nil && len(results.Tracks) > 0 {
				tracks := make([]map[string]any, len(results.Tracks))
				for i, t := range results.Tracks {
					tracks[i] = map[string]any{
						"id":           t.ID,
						"name":         t.Name,
						"playback_uri": t.URI,
						"artwork_url":  t.ImageURL,
						"artist_name":  t.ArtistName,
						"album_name":   t.AlbumName,
						"duration_ms":  t.DurationMs,
						"content_type": "tracks",
						"provider":     "spotify",
					}
				}
				resultsMap["tracks"] = tracks
			}
			if results.Albums != nil && len(results.Albums) > 0 {
				albums := make([]map[string]any, len(results.Albums))
				for i, a := range results.Albums {
					albums[i] = map[string]any{
						"id":           a.ID,
						"name":         a.Name,
						"playback_uri": a.URI,
						"artwork_url":  a.ImageURL,
						"artist_name":  a.ArtistName,
						"content_type": "albums",
						"provider":     "spotify",
					}
				}
				resultsMap["albums"] = albums
			}
			if results.Artists != nil && len(results.Artists) > 0 {
				artists := make([]map[string]any, len(results.Artists))
				for i, a := range results.Artists {
					artists[i] = map[string]any{
						"id":           a.ID,
						"name":         a.Name,
						"playback_uri": a.URI,
						"artwork_url":  a.ImageURL,
						"content_type": "artists",
						"provider":     "spotify",
					}
				}
				resultsMap["artists"] = artists
			}
			if results.Playlists != nil && len(results.Playlists) > 0 {
				playlists := make([]map[string]any, len(results.Playlists))
				for i, p := range results.Playlists {
					playlists[i] = map[string]any{
						"id":           p.ID,
						"name":         p.Name,
						"playback_uri": p.URI,
						"artwork_url":  p.ImageURL,
						"owner_name":   p.OwnerName,
						"description":  p.Description,
						"content_type": "playlists",
						"provider":     "spotify",
					}
				}
				resultsMap["playlists"] = playlists
			}
			if results.Genres != nil && len(results.Genres) > 0 {
				genres := make([]map[string]any, len(results.Genres))
				for i, g := range results.Genres {
					genres[i] = map[string]any{
						"id":           g.ID,
						"name":         g.Name,
						"playback_uri": g.URI,
						"artwork_url":  g.ImageURL,
						"content_type": "genres",
						"provider":     "spotify",
					}
				}
				resultsMap["genres"] = genres
			}
			if results.Audiobooks != nil && len(results.Audiobooks) > 0 {
				audiobooks := make([]map[string]any, len(results.Audiobooks))
				for i, a := range results.Audiobooks {
					audiobooks[i] = map[string]any{
						"id":           a.ID,
						"name":         a.Name,
						"playback_uri": a.URI,
						"artwork_url":  a.ImageURL,
						"author_name":  a.AuthorName,
						"content_type": "audiobooks",
						"provider":     "spotify",
					}
				}
				resultsMap["audiobooks"] = audiobooks
			}
			if results.Podcasts != nil && len(results.Podcasts) > 0 {
				podcasts := make([]map[string]any, len(results.Podcasts))
				for i, p := range results.Podcasts {
					podcasts[i] = map[string]any{
						"id":             p.ID,
						"name":           p.Name,
						"playback_uri":   p.URI,
						"artwork_url":    p.ImageURL,
						"publisher_name": p.PublisherName,
						"content_type":   "podcasts",
						"provider":       "spotify",
					}
				}
				resultsMap["podcasts"] = podcasts
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":   "music_search",
				"provider": provider,
				"query":    query,
				"results":  resultsMap,
				"pagination": map[string]any{
					"limit":  limit,
					"offset": offset,
					"total":  0, // Extension doesn't provide total count
				},
			})
		}

		// Handle Library search via UPnP ContentDirectory
		if provider == "library" {
			if libraryProvider == nil {
				// No library provider available - return empty results
				return api.WriteResource(w, http.StatusOK, map[string]any{
					"object":   "music_search",
					"provider": provider,
					"query":    query,
					"results":  map[string][]any{},
					"pagination": map[string]any{
						"limit":  limit,
						"offset": offset,
						"total":  0,
					},
				})
			}

			// Parse content types from request
			var types []string
			if typesParam != "" {
				for _, t := range strings.Split(typesParam, ",") {
					types = append(types, strings.TrimSpace(t))
				}
			}

			// Perform library search
			result, err := libraryProvider.Search(r.Context(), query, types, limit, offset)
			if err != nil {
				return apperrors.NewInternalError("Library search failed")
			}

			// Convert LibraryItem results to API format
			resultsMap := make(map[string]any)
			for contentType, items := range result.Results {
				apiItems := make([]map[string]any, len(items))
				for i, item := range items {
					apiItems[i] = map[string]any{
						"id":           item.ID,
						"name":         item.Name,
						"content_type": item.ContentType,
						"provider":     item.Provider,
						"artist_name":  item.ArtistName,
						"album_name":   item.AlbumName,
						"artwork_url":  item.ArtworkURL,
						"playback_uri": item.PlaybackURI,
						"duration_ms":  item.DurationMs,
					}
				}
				resultsMap[contentType] = apiItems
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":   "music_search",
				"provider": provider,
				"query":    query,
				"results":  resultsMap,
				"pagination": map[string]any{
					"limit":  result.Pagination.Limit,
					"offset": result.Pagination.Offset,
					"total":  result.Pagination.Total,
				},
			})
		}

		// Handle Apple Music search
		if provider == "apple_music" {
			if appleClient == nil {
				return apperrors.NewAppError("SERVICE_UNAVAILABLE", "Apple Music not configured", 503, nil, nil)
			}

			// Apple Music API has a max limit of 25
			appleLimit := limit
			if appleLimit > 25 {
				appleLimit = 25
			}

			// Perform Apple Music search
			result, err := appleClient.Search(r.Context(), query, typesParam, appleLimit, offset)
			if err != nil {
				return apperrors.NewInternalError("Apple Music search failed: " + err.Error())
			}

			// Parse types parameter into array for response
			var types []string
			if typesParam != "" {
				for _, t := range strings.Split(typesParam, ",") {
					types = append(types, strings.TrimSpace(t))
				}
			} else {
				types = []string{"songs", "albums", "artists", "playlists"}
			}

			// Convert results to API format (snake_case)
			// iOS expects "type" field on each item (not "content_type")
			resultsMap := make(map[string]any)
			for contentType, items := range result.Results {
				apiItems := make([]map[string]any, len(items))
				for i, item := range items {
					apiItem := map[string]any{
						"id":   item.ID,
						"name": item.Name,
						"type": item.ContentType, // iOS expects "type" not "content_type"
					}
					if item.ArtistName != nil {
						apiItem["artist_name"] = *item.ArtistName
					}
					if item.AlbumName != nil {
						apiItem["album_name"] = *item.AlbumName
					}
					if item.ArtworkURL != nil {
						apiItem["artwork_url"] = *item.ArtworkURL
					}
					if item.PlaybackURI != nil {
						apiItem["playback_uri"] = *item.PlaybackURI
					}
					if item.DurationMs != nil {
						apiItem["duration_ms"] = *item.DurationMs
					}
					if item.CuratorName != nil {
						apiItem["curator_name"] = *item.CuratorName
					}
					apiItems[i] = apiItem
				}
				resultsMap[contentType] = apiItems
			}

			// iOS expects: query, types (array), results, totals (optional)
			return api.WriteResource(w, http.StatusOK, map[string]any{
				"query":   query,
				"types":   types,
				"results": resultsMap,
			})
		}

		// Unknown provider
		return apperrors.NewValidationError("unsupported provider", map[string]any{
			"provider":            provider,
			"supported_providers": []string{"apple_music", "library", "spotify"},
		})
	}
}

// ==========================================================================
// Suggestions Handler
// ==========================================================================

// getMusicSuggestions handles GET /v1/music/suggestions
// Mirrors Node.js music-search.ts suggestions format
func getMusicSuggestions(appleClient *applemusic.Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		query := r.URL.Query().Get("query")
		if query == "" {
			query = r.URL.Query().Get("q") // Fallback
		}
		provider := r.URL.Query().Get("provider")
		typesParam := r.URL.Query().Get("types")

		// Parse limit
		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 25 {
				limit = parsed
			}
		}

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

		// Check if Apple Music is configured
		if appleClient == nil {
			return apperrors.NewAppError("SERVICE_UNAVAILABLE", "Apple Music not configured", 503, nil, nil)
		}

		// Get suggestions from Apple Music API
		result, err := appleClient.GetSuggestions(r.Context(), query, typesParam, limit)
		if err != nil {
			return apperrors.NewInternalError("Apple Music suggestions failed: " + err.Error())
		}

		// Convert terms to API format
		terms := make([]map[string]string, len(result.Terms))
		for i, t := range result.Terms {
			terms[i] = map[string]string{
				"term":         t.Term,
				"display_term": t.DisplayTerm,
			}
		}

		// Convert top results to API format
		topResults := make([]map[string]any, len(result.TopResults))
		for i, tr := range result.TopResults {
			topResult := map[string]any{
				"id":           tr.ID,
				"name":         tr.Name,
				"content_type": tr.ContentType,
			}
			if tr.ArtistName != nil {
				topResult["artist_name"] = *tr.ArtistName
			}
			if tr.ArtworkURL != nil {
				topResult["artwork_url"] = *tr.ArtworkURL
			}
			topResults[i] = topResult
		}

		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":      "music_suggestions",
			"provider":    provider,
			"query":       query,
			"terms":       terms,
			"top_results": topResults,
		})
	}
}

// ==========================================================================
// Providers Handler
// ==========================================================================

// MusicProvider represents a music service provider.
type MusicProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	RequiresAuth bool   `json:"requires_auth"`
}

// listProviders handles GET /v1/music/providers
// Mirrors Node.js music-search.ts providers format
func listProviders(service *Service, spotifyManager *spotifysearch.ConnectionManager, appleClient *applemusic.Client) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		// Return available search providers
		// Matches Node.js music-search.ts /v1/music/providers response, with Stripe-style envelope

		// Apple Music provider with configuration status
		appleStatus := "not_configured"
		if appleClient != nil {
			appleStatus = "configured"
		}
		providers := []map[string]any{
			{
				"object":               "music_provider",
				"name":                 "apple_music",
				"display_name":         "Apple Music",
				"supported_types":      []string{"albums", "artists", "tracks", "playlists", "stations"},
				"supports_suggestions": true,
				"status":               appleStatus,
			},
			{
				"object":               "music_provider",
				"name":                 "library",
				"display_name":         "Music Library",
				"supported_types":      []string{"albums", "artists", "tracks", "playlists"},
				"supports_suggestions": false,
			},
		}

		// Add Spotify provider with connection status
		spotifyStatus := "disconnected"
		if spotifyManager != nil && spotifyManager.IsConnected() {
			spotifyStatus = "connected"
		}
		providers = append(providers, map[string]any{
			"object":               "music_provider",
			"name":                 "spotify",
			"display_name":         "Spotify",
			"supported_types":      []string{"albums", "artists", "tracks", "playlists", "genres", "audiobooks", "podcasts"},
			"supports_suggestions": false,
			"status":               spotifyStatus,
		})

		// Stripe-style list response (small fixed list - no pagination needed)
		return api.WriteList(w, "/v1/music/providers", providers, false)
	}
}
