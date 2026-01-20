package scene

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

// RegisterRoutes wires scene routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	// Scene CRUD
	router.Method(http.MethodPost, "/v1/scenes", api.Handler(createScene(service)))
	router.Method(http.MethodGet, "/v1/scenes", api.Handler(listScenes(service)))
	router.Method(http.MethodGet, "/v1/scenes/{scene_id}", api.Handler(getScene(service)))
	router.Method(http.MethodPut, "/v1/scenes/{scene_id}", api.Handler(updateScene(service)))
	router.Method(http.MethodDelete, "/v1/scenes/{scene_id}", api.Handler(deleteScene(service)))

	// Scene execution
	router.Method(http.MethodPost, "/v1/scenes/{scene_id}/execute", api.Handler(executeScene(service)))
	router.Method(http.MethodPost, "/v1/scenes/{scene_id}/stop", api.Handler(stopScene(service)))

	// Executions
	router.Method(http.MethodGet, "/v1/scenes/{scene_id}/executions", api.Handler(listExecutions(service)))
}

func createScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var input CreateSceneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if input.Name == "" {
			return apperrors.NewValidationError("name is required", nil)
		}

		scene, err := service.CreateScene(input)
		if err != nil {
			return apperrors.NewInternalError("Failed to create scene")
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusCreated, formatScene(scene))
	}
}

func listScenes(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		limit := 100
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

		scenes, total, err := service.ListScenes(limit, offset)
		if err != nil {
			return apperrors.NewInternalError("Failed to list scenes")
		}

		formatted := make([]map[string]any, 0, len(scenes))
		for _, scene := range scenes {
			formatted = append(formatted, formatScene(&scene))
		}

		hasMore := offset+len(scenes) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/scenes", formatted, hasMore)
	}
}

func getScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		scene, err := service.GetScene(sceneID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get scene")
		}
		if scene == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": sceneID}, nil)
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatScene(scene))
	}
}

func updateScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		var input UpdateSceneInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		scene, err := service.UpdateScene(sceneID, input)
		if err != nil {
			return apperrors.NewInternalError("Failed to update scene")
		}
		if scene == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": sceneID}, nil)
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatScene(scene))
	}
}

func deleteScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		err := service.DeleteScene(sceneID)
		if err != nil {
			var inUseErr *SceneInUseError
			if errors.As(err, &inUseErr) {
				return apperrors.NewConflictError("Scene is referenced by routines and cannot be deleted", map[string]any{
					"scene_id":      sceneID,
					"routine_count": inUseErr.RoutineCount,
				})
			}
			var notFoundErr *SceneNotFoundError
			if errors.As(err, &notFoundErr) {
				return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": sceneID}, nil)
			}
			return apperrors.NewInternalError("Failed to delete scene")
		}

		// Return 204 No Content with empty body (Node.js parity)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func executeScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		// Get idempotency key from header
		var idempotencyKey *string
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			idempotencyKey = &key
		}

		// Parse options from body
		var options ExecuteOptions
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&options); err != nil {
				return apperrors.NewValidationError("invalid request body", nil)
			}
		}

		execution, err := service.ExecuteScene(sceneID, idempotencyKey, options)
		if err != nil {
			var notFoundErr *SceneNotFoundError
			if errors.As(err, &notFoundErr) {
				return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": sceneID}, nil)
			}
			return apperrors.NewInternalError("Failed to execute scene")
		}

		// Check if this was a duplicate request (idempotent)
		isIdempotent := false
		if idempotencyKey != nil && execution.IdempotencyKey != nil && *execution.IdempotencyKey == *idempotencyKey {
			// Check if the execution was already in progress before this request
			if execution.Status != ExecutionStatusStarting {
				isIdempotent = true
			}
		}

		// Stripe-style: return execution object directly
		execResponse := formatExecution(execution)
		execResponse["idempotent"] = isIdempotent

		return api.WriteAction(w, http.StatusAccepted, execResponse)
	}
}

func stopScene(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		results, err := service.StopScene(sceneID)
		if err != nil {
			var notFoundErr *SceneNotFoundError
			if errors.As(err, &notFoundErr) {
				return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": sceneID}, nil)
			}
			return apperrors.NewInternalError("Failed to stop scene")
		}

		allSucceeded := true
		for _, res := range results {
			if !res.Success {
				allSucceeded = false
				break
			}
		}

		status := http.StatusOK
		if !allSucceeded {
			status = http.StatusMultiStatus // 207
		}

		// Stripe-style: return action result directly with object type
		return api.WriteAction(w, status, map[string]any{
			"object":        "scene_stop",
			"scene_id":      sceneID,
			"results":       results,
			"all_succeeded": allSucceeded,
			"stopped_at":    api.RFC3339Millis(time.Now()),
		})
	}
}

func listExecutions(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		sceneID := chi.URLParam(r, "scene_id")

		limit := 20
		offset := 0

		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 20 {
				limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		executions, total, err := service.ListExecutions(sceneID, limit, offset)
		if err != nil {
			return apperrors.NewInternalError("Failed to list executions")
		}

		formatted := make([]map[string]any, 0, len(executions))
		for _, exec := range executions {
			formatted = append(formatted, formatExecution(&exec))
		}

		hasMore := offset+len(executions) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/scenes/"+sceneID+"/executions", formatted, hasMore)
	}
}

func formatScene(scene *Scene) map[string]any {
	members := make([]map[string]any, 0, len(scene.Members))
	for _, m := range scene.Members {
		member := map[string]any{
			"udn": m.UDN,
		}
		if m.RoomName != "" {
			member["room_name"] = m.RoomName
		}
		if m.TargetVolume != nil {
			member["target_volume"] = *m.TargetVolume
		}
		if m.Mute != nil {
			member["mute"] = *m.Mute
		}
		members = append(members, member)
	}

	result := map[string]any{
		"object":                 api.ObjectScene,
		"id":                     scene.SceneID,
		"name":                   scene.Name,
		"coordinator_preference": scene.CoordinatorPreference,
		"fallback_policy":        scene.FallbackPolicy,
		"members":                members,
		"created_at":             api.RFC3339Millis(scene.CreatedAt),
		"updated_at":             api.RFC3339Millis(scene.UpdatedAt),
	}

	// Always include description (null if not set) for API parity with Node.js
	if scene.Description != nil {
		result["description"] = *scene.Description
	} else {
		result["description"] = nil
	}

	// Always include volume_ramp (null if not set) for API parity with Node.js
	if scene.VolumeRamp != nil {
		volumeRamp := map[string]any{
			"enabled": scene.VolumeRamp.Enabled,
		}
		if scene.VolumeRamp.DurationMs != nil {
			volumeRamp["duration_ms"] = *scene.VolumeRamp.DurationMs
		}
		if scene.VolumeRamp.Curve != "" {
			volumeRamp["curve"] = scene.VolumeRamp.Curve
		}
		result["volume_ramp"] = volumeRamp
	} else {
		result["volume_ramp"] = nil
	}

	// Always include teardown (null if not set) for API parity with Node.js
	if scene.Teardown != nil {
		teardown := map[string]any{
			"restore_previous_state": scene.Teardown.RestorePreviousState,
		}
		if scene.Teardown.UngroupAfterMs != nil {
			teardown["ungroup_after_ms"] = *scene.Teardown.UngroupAfterMs
		}
		result["teardown"] = teardown
	} else {
		result["teardown"] = nil
	}

	return result
}

func formatExecution(exec *SceneExecution) map[string]any {
	steps := make([]map[string]any, 0, len(exec.Steps))
	for _, s := range exec.Steps {
		step := map[string]any{
			"step":   s.Step,
			"status": string(s.Status),
		}
		if s.StartedAt != nil {
			step["started_at"] = api.RFC3339Millis(*s.StartedAt)
		}
		if s.EndedAt != nil {
			step["ended_at"] = api.RFC3339Millis(*s.EndedAt)
		}
		if s.Error != "" {
			step["error"] = s.Error
		}
		if s.Details != nil {
			step["details"] = s.Details
		}
		steps = append(steps, step)
	}

	result := map[string]any{
		"object":     api.ObjectSceneExecution,
		"id":         exec.SceneExecutionID,
		"scene_id":   exec.SceneID,
		"status":     string(exec.Status),
		"started_at": api.RFC3339Millis(exec.StartedAt),
		"steps":      steps,
	}

	if exec.IdempotencyKey != nil {
		result["idempotency_key"] = *exec.IdempotencyKey
	}
	if exec.CoordinatorUsedUDN != nil {
		result["coordinator_used_udn"] = *exec.CoordinatorUsedUDN
	}
	if exec.EndedAt != nil {
		result["ended_at"] = api.RFC3339Millis(*exec.EndedAt)
	}
	if exec.Error != nil {
		result["error"] = *exec.Error
	}
	if exec.Verification != nil {
		result["verification"] = map[string]any{
			"playback_confirmed":       exec.Verification.PlaybackConfirmed,
			"transport_state":          exec.Verification.TransportState,
			"track_uri":                exec.Verification.TrackURI,
			"checked_at":               api.RFC3339Millis(exec.Verification.CheckedAt),
			"verification_unavailable": exec.Verification.VerificationUnavailable,
		}
	}

	return result
}
