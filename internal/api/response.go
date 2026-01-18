package api

import (
	"encoding/json"
	"net/http"

	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// Pagination represents standard pagination metadata for list responses.
type Pagination struct {
	Total   int  `json:"total"`
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

type errorResponse struct {
	RequestID string              `json:"request_id"`
	Error     apperrors.ErrorBody `json:"error"`
}

// WriteJSON sends a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(payload)
}

// WriteError serializes an AppError into the standard error response.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := apperrors.EnsureAppError(err)

	response := errorResponse{
		RequestID: GetRequestID(r),
		Error:     appErr.ErrorBody(),
	}

	_ = WriteJSON(w, appErr.StatusCode, response)
}

// SingleResponse writes a single resource response with a dynamic resource key.
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
// Example: ActionResponse(w, r, http.StatusOK, map[string]any{"execution_id": "...", "status": "started"})
// Produces: {"request_id": "...", "result": {...}}
func ActionResponse(w http.ResponseWriter, r *http.Request, status int, result any) error {
	resp := map[string]any{
		"request_id": GetRequestID(r),
		"result":     result,
	}
	return WriteJSON(w, status, resp)
}
