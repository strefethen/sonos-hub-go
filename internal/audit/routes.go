package audit

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// MaxMessageLength is the maximum allowed length for audit event messages.
const MaxMessageLength = 2000

// validEventTypes defines all valid audit event types.
var validEventTypes = map[string]bool{
	string(EventRoutineCreated):          true,
	string(EventRoutineUpdated):          true,
	string(EventRoutineDeleted):          true,
	string(EventJobScheduled):            true,
	string(EventJobStarted):              true,
	string(EventJobCompleted):            true,
	string(EventJobFailed):               true,
	string(EventJobSkipped):              true,
	string(EventSceneCreated):            true,
	string(EventSceneUpdated):            true,
	string(EventSceneExecutionStarted):   true,
	string(EventSceneExecutionStep):      true,
	string(EventSceneExecutionCompleted): true,
	string(EventSceneExecutionFailed):    true,
	string(EventDeviceDiscovered):        true,
	string(EventDeviceOffline):           true,
	string(EventPlaybackVerified):        true,
	string(EventPlaybackFailed):          true,
	string(EventSystemStartup):           true,
	string(EventSystemError):             true,
}

// validEventLevels defines all valid audit event levels.
var validEventLevels = map[string]EventLevel{
	"INFO":  EventLevelInfo,
	"WARN":  EventLevelWarn,
	"ERROR": EventLevelError,
}

// ==========================================================================
// Request Types
// ==========================================================================

// CreateEventRequest represents the request body for POST /v1/audit/events.
type CreateEventRequest struct {
	Type        string                 `json:"type"`
	Level       string                 `json:"level,omitempty"`
	Message     string                 `json:"message"`
	Correlation *CreateEventCorrelation `json:"correlation,omitempty"`
	Payload     map[string]any         `json:"payload,omitempty"`
}

// CreateEventCorrelation contains correlation IDs for linking related events.
type CreateEventCorrelation struct {
	RequestID        *string `json:"request_id,omitempty"`
	RoutineID        *string `json:"routine_id,omitempty"`
	JobID            *string `json:"job_id,omitempty"`
	SceneExecutionID *string `json:"scene_execution_id,omitempty"`
	DeviceID         *string `json:"device_id,omitempty"`
}

// ==========================================================================
// Route Registration
// ==========================================================================

// RegisterRoutes wires audit routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Method(http.MethodGet, "/v1/audit/events", api.Handler(queryEvents(service)))
	router.Method(http.MethodGet, "/v1/audit/events/{event_id}", api.Handler(getEvent(service)))
	router.Method(http.MethodPost, "/v1/audit/events", api.Handler(recordEvent(service)))
}

// ==========================================================================
// Handlers
// ==========================================================================

// queryEvents retrieves audit events with optional filters.
// GET /v1/audit/events
func queryEvents(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		filters, err := parseQueryFilters(r)
		if err != nil {
			return err
		}

		events, total, hasMore, err := service.QueryEvents(filters)
		if err != nil {
			return apperrors.NewInternalError("Failed to query audit events")
		}

		formatted := make([]map[string]any, 0, len(events))
		for _, event := range events {
			formatted = append(formatted, formatEvent(&event))
		}

		pagination := &api.Pagination{
			Total:   total,
			Limit:   filters.Limit,
			Offset:  filters.Offset,
			HasMore: hasMore,
		}
		return api.ListResponse(w, r, http.StatusOK, "events", formatted, pagination)
	}
}

// getEvent retrieves a single audit event by ID.
// GET /v1/audit/events/{event_id}
func getEvent(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		eventID := chi.URLParam(r, "event_id")

		event, err := service.GetEvent(eventID)
		if err != nil {
			var notFoundErr *EventNotFoundError
			if errors.As(err, &notFoundErr) {
				return apperrors.NewAppError(apperrors.ErrorCodeEventNotFound, "Event not found", 404, map[string]any{
					"event_id": eventID,
				}, nil)
			}
			return apperrors.NewInternalError("Failed to get audit event")
		}

		return api.SingleResponse(w, r, http.StatusOK, "event", formatEvent(event))
	}
}

// recordEvent creates a new audit event.
// POST /v1/audit/events
func recordEvent(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req CreateEventRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate required fields
		if req.Type == "" {
			return apperrors.NewValidationError("type is required", nil)
		}

		// Validate event type
		if !validEventTypes[req.Type] {
			return apperrors.NewValidationError("invalid event type", map[string]any{
				"type": req.Type,
			})
		}

		// Validate message length
		if len(req.Message) > MaxMessageLength {
			return apperrors.NewValidationError("message too long", map[string]any{
				"max_length":    MaxMessageLength,
				"actual_length": len(req.Message),
			})
		}

		// Build WriteEventInput
		input := WriteEventInput{
			Type:    req.Type,
			Message: req.Message,
			Payload: req.Payload,
		}

		// Parse and validate level if provided
		if req.Level != "" {
			level, ok := validEventLevels[req.Level]
			if !ok {
				return apperrors.NewValidationError("invalid level", map[string]any{
					"level":        req.Level,
					"valid_levels": []string{"INFO", "WARN", "ERROR"},
				})
			}
			input.Level = &level
		}

		// Apply correlation IDs
		if req.Correlation != nil {
			input.RequestID = req.Correlation.RequestID
			input.RoutineID = req.Correlation.RoutineID
			input.JobID = req.Correlation.JobID
			input.SceneExecutionID = req.Correlation.SceneExecutionID
			input.DeviceID = req.Correlation.DeviceID
		}

		event, err := service.RecordEvent(input)
		if err != nil {
			return apperrors.NewInternalError("Failed to record audit event")
		}

		return api.SingleResponse(w, r, http.StatusCreated, "event", formatEvent(event))
	}
}

// ==========================================================================
// Helper Functions
// ==========================================================================

// parseQueryFilters extracts and validates query parameters for event filtering.
func parseQueryFilters(r *http.Request) (EventQueryFilters, error) {
	filters := EventQueryFilters{
		Limit:  DefaultQueryLimit,
		Offset: 0,
	}

	query := r.URL.Query()

	// Parse 'from' (inclusive start datetime)
	if from := query.Get("from"); from != "" {
		if _, err := time.Parse(time.RFC3339, from); err != nil {
			return filters, apperrors.NewValidationError("invalid 'from' datetime format, expected ISO 8601", map[string]any{"from": from})
		}
		filters.StartDate = &from
	}

	// Parse 'to' (inclusive end datetime)
	if to := query.Get("to"); to != "" {
		if _, err := time.Parse(time.RFC3339, to); err != nil {
			return filters, apperrors.NewValidationError("invalid 'to' datetime format, expected ISO 8601", map[string]any{"to": to})
		}
		filters.EndDate = &to
	}

	// Parse 'type'
	if eventType := query.Get("type"); eventType != "" {
		filters.Type = &eventType
	}

	// Parse 'level'
	if level := query.Get("level"); level != "" {
		parsedLevel, ok := validEventLevels[level]
		if !ok {
			return filters, apperrors.NewValidationError("invalid level", map[string]any{
				"level":        level,
				"valid_levels": []string{"INFO", "WARN", "ERROR"},
			})
		}
		filters.Level = &parsedLevel
	}

	// Parse correlation ID filters
	if jobID := query.Get("job_id"); jobID != "" {
		filters.JobID = &jobID
	}
	if routineID := query.Get("routine_id"); routineID != "" {
		filters.RoutineID = &routineID
	}
	if sceneExecutionID := query.Get("scene_execution_id"); sceneExecutionID != "" {
		filters.SceneExecutionID = &sceneExecutionID
	}
	if deviceID := query.Get("device_id"); deviceID != "" {
		filters.DeviceID = &deviceID
	}

	// Parse 'limit' (1-1000, default 100)
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > MaxQueryLimit {
			return filters, apperrors.NewValidationError("invalid limit, must be between 1 and 1000", map[string]any{
				"limit": limitStr,
			})
		}
		filters.Limit = limit
	}

	// Parse 'offset' (>= 0, default 0)
	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return filters, apperrors.NewValidationError("invalid offset, must be >= 0", map[string]any{
				"offset": offsetStr,
			})
		}
		filters.Offset = offset
	}

	return filters, nil
}

// formatEvent formats an AuditEvent for JSON response.
func formatEvent(event *AuditEvent) map[string]any {
	result := map[string]any{
		"event_id":  event.EventID,
		"timestamp": event.Timestamp.UTC().Format(time.RFC3339),
		"type":      event.Type,
		"level":     string(event.Level),
		"message":   event.Message,
	}

	// Add correlation object with present IDs
	correlation := map[string]any{}
	if event.RequestID != nil {
		correlation["request_id"] = *event.RequestID
	}
	if event.RoutineID != nil {
		correlation["routine_id"] = *event.RoutineID
	}
	if event.JobID != nil {
		correlation["job_id"] = *event.JobID
	}
	if event.SceneExecutionID != nil {
		correlation["scene_execution_id"] = *event.SceneExecutionID
	}
	if event.DeviceID != nil {
		correlation["device_id"] = *event.DeviceID
	}
	if len(correlation) > 0 {
		result["correlation"] = correlation
	}

	// Add payload if present and non-empty
	if event.Payload != nil && len(event.Payload) > 0 {
		result["payload"] = event.Payload
	}

	return result
}
