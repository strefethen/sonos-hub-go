package api

import (
	"encoding/json"
	"net/http"

	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// =============================================================================
// Stripe API Standard Response Types
// =============================================================================

// StripeListResponse is the Stripe-style list response for all collection endpoints.
// Example: {"object": "list", "data": [...], "has_more": false, "url": "/v1/routines"}
type StripeListResponse struct {
	Object  string `json:"object"`   // Always "list"
	Data    any    `json:"data"`     // Array of resources
	HasMore bool   `json:"has_more"` // Whether more items exist beyond this page
	URL     string `json:"url"`      // The URL for this list endpoint
}

// StripeErrorResponse wraps errors in Stripe format.
type StripeErrorResponse struct {
	Error apperrors.StripeErrorBody `json:"error"`
}

// =============================================================================
// Legacy Types (kept for backwards compatibility during migration)
// =============================================================================

// Pagination represents standard pagination metadata for list responses.
// Deprecated: Use WriteList instead which uses Stripe-style has_more pagination.
type Pagination struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

// =============================================================================
// Core Response Functions
// =============================================================================

// WriteJSON sends a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

// WriteError serializes an AppError into the Stripe-style error response.
// Response format: {"error": {"type": "...", "code": "...", "message": "..."}}
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := apperrors.EnsureAppError(err)

	response := StripeErrorResponse{
		Error: appErr.StripeErrorBody(),
	}

	_ = WriteJSON(w, appErr.StatusCode, response)
}

// =============================================================================
// Stripe-Style Response Helpers
// =============================================================================

// WriteList writes a Stripe-style list response.
// Example: WriteList(w, "/v1/routines", routines, false)
// Produces: {"object": "list", "data": [...], "has_more": false, "url": "/v1/routines"}
func WriteList(w http.ResponseWriter, url string, data any, hasMore bool) error {
	return WriteJSON(w, http.StatusOK, StripeListResponse{
		Object:  "list",
		Data:    data,
		HasMore: hasMore,
		URL:     url,
	})
}

// WriteResource writes a single resource directly (Stripe-style, no wrapper).
// The resource should already have an "object" field set.
// Example: WriteResource(w, http.StatusOK, routine)
// Produces: {"object": "routine", "id": "...", "name": "...", ...}
func WriteResource(w http.ResponseWriter, status int, resource any) error {
	return WriteJSON(w, status, resource)
}

// WriteAction writes an action result directly (Stripe-style, no wrapper).
// The result should already have an "object" field set.
// Example: WriteAction(w, http.StatusAccepted, execution)
// Produces: {"object": "execution", "id": "...", "status": "started", ...}
func WriteAction(w http.ResponseWriter, status int, result any) error {
	return WriteJSON(w, status, result)
}

// =============================================================================
// Legacy Response Helpers (for gradual migration)
// =============================================================================

// SingleResponse writes a single resource response with a dynamic resource key.
// Deprecated: Use WriteResource instead for Stripe-style responses.
// Example: SingleResponse(w, r, http.StatusOK, "routine", routine)
// Produces: {"request_id": "...", "routine": {...}}
func SingleResponse(w http.ResponseWriter, r *http.Request, status int, key string, resource any) error {
	resp := map[string]any{
		"request_id": GetRequestID(r),
		key:          resource,
	}
	return WriteJSON(w, status, resp)
}

// ListResponse writes a collection response with a dynamic collection key and optional pagination.
// Deprecated: Use WriteList instead for Stripe-style responses.
// Example: ListResponse(w, r, http.StatusOK, "routines", routines, &Pagination{...})
// Produces: {"request_id": "...", "routines": [...], "pagination": {...}}
func ListResponse(w http.ResponseWriter, r *http.Request, status int, key string, items any, pagination *Pagination) error {
	resp := map[string]any{
		"request_id": GetRequestID(r),
		key:          items,
	}
	if pagination != nil {
		resp["pagination"] = pagination
	}
	return WriteJSON(w, status, resp)
}

// ActionResponse writes a response for non-CRUD action endpoints.
// Deprecated: Use WriteAction instead for Stripe-style responses.
// Example: ActionResponse(w, r, http.StatusOK, map[string]any{"execution_id": "...", "status": "started"})
// Produces: {"request_id": "...", "result": {...}}
func ActionResponse(w http.ResponseWriter, r *http.Request, status int, result any) error {
	resp := map[string]any{
		"request_id": GetRequestID(r),
		"result":     result,
	}
	return WriteJSON(w, status, resp)
}
