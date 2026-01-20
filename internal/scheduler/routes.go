package scheduler

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/music"
	"github.com/strefethen/sonos-hub-go/internal/scene"
)

// RegisterRoutes wires scheduler routes to the router.
func RegisterRoutes(router chi.Router, routinesRepo *RoutinesRepository, jobsRepo *JobsRepository, holidaysRepo *HolidaysRepository, sceneService *scene.Service, deviceService *devices.Service, musicService *music.Service) {
	// Routine CRUD
	router.Method(http.MethodPost, "/v1/routines", api.Handler(createRoutine(routinesRepo, sceneService, deviceService, musicService)))
	router.Method(http.MethodGet, "/v1/routines", api.Handler(listRoutines(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodGet, "/v1/routines/{routine_id}", api.Handler(getRoutine(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPut, "/v1/routines/{routine_id}", api.Handler(updateRoutine(routinesRepo, sceneService, deviceService, musicService)))
	router.Method(http.MethodDelete, "/v1/routines/{routine_id}", api.Handler(deleteRoutine(routinesRepo, sceneService)))

	// Routine actions
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/enable", api.Handler(enableRoutine(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/disable", api.Handler(disableRoutine(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/trigger", api.Handler(triggerRoutine(routinesRepo, jobsRepo)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/snooze", api.Handler(snoozeRoutine(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/unsnooze", api.Handler(unsnoozeRoutine(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/skip", api.Handler(skipNextOccurrence(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/unskip", api.Handler(unskipNextOccurrence(routinesRepo, deviceService, musicService)))
	router.Method(http.MethodPost, "/v1/routines/{routine_id}/run", api.Handler(runRoutine(routinesRepo, jobsRepo)))
	router.Method(http.MethodPost, "/v1/routines/test", api.Handler(testRoutine(sceneService)))

	// Jobs
	router.Method(http.MethodGet, "/v1/jobs/{job_id}", api.Handler(getJob(jobsRepo)))
	router.Method(http.MethodGet, "/v1/routines/{routine_id}/jobs", api.Handler(listJobsForRoutine(routinesRepo, jobsRepo)))

	// Executions (jobs across all routines)
	router.Method(http.MethodGet, "/v1/executions", api.Handler(listExecutions(jobsRepo, routinesRepo)))
	router.Method(http.MethodPost, "/v1/executions/{execution_id}/retry", api.Handler(retryExecution(jobsRepo)))

	// Holidays
	router.Method(http.MethodPost, "/v1/holidays", api.Handler(createHoliday(holidaysRepo)))
	router.Method(http.MethodGet, "/v1/holidays", api.Handler(listHolidays(holidaysRepo)))
	router.Method(http.MethodGet, "/v1/holidays/check", api.Handler(checkHoliday(holidaysRepo)))
	router.Method(http.MethodGet, "/v1/holidays/{holiday_id}", api.Handler(getHoliday(holidaysRepo)))
	router.Method(http.MethodDelete, "/v1/holidays/{holiday_id}", api.Handler(deleteHoliday(holidaysRepo)))
}

// ==========================================================================
// Routine Handlers
// ==========================================================================

// ScheduleInput handles nested schedule from iOS.
// iOS sends { "schedule": { "type": "weekly", "weekdays": [2,3,4,5,6], "time": "07:30" } }
// but Go expects flat fields: schedule_type, schedule_weekdays, schedule_time.
type ScheduleInput struct {
	Type     string `json:"type"`
	Weekdays []int  `json:"weekdays,omitempty"`
	Month    *int   `json:"month,omitempty"`
	Day      *int   `json:"day,omitempty"`
	Time     string `json:"time"`
}

// createRoutineRequest is the input structure for creating a routine.
// It supports both scene_id (pre-existing scene) and speakers (auto-create scene).
// iOS sends nested music_policy and schedule objects which we flatten to database columns.
type createRoutineRequest struct {
	CreateRoutineInput
	Speakers    []SpeakerInput `json:"speakers,omitempty"`     // iOS sends speakers instead of scene_id
	MusicPolicy *MusicPolicy   `json:"music_policy,omitempty"` // Nested music policy from iOS
	Schedule    *ScheduleInput `json:"schedule,omitempty"`     // Nested schedule from iOS
}

func createRoutine(routinesRepo *RoutinesRepository, sceneService *scene.Service, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req createRoutineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate required fields
		if req.Name == "" {
			return apperrors.NewValidationError("name is required", nil)
		}

		// Require either scene_id OR speakers
		if req.SceneID == "" && len(req.Speakers) == 0 {
			return apperrors.NewValidationError("either scene_id or speakers is required", nil)
		}

		// Auto-create scene if speakers provided and no scene_id
		if len(req.Speakers) > 0 && req.SceneID == "" {
			// Convert SpeakerInput to SceneMember
			members := make([]scene.SceneMember, len(req.Speakers))
			for i, s := range req.Speakers {
				vol := s.Volume
				members[i] = scene.SceneMember{
					UDN:          s.UDN,
					TargetVolume: &vol,
				}
			}

			// Auto-create scene for this routine
			description := "Auto-created scene for routine"
			newScene, err := sceneService.CreateScene(scene.CreateSceneInput{
				Name:        "Routine: " + req.Name,
				Description: &description,
				Members:     members,
			})
			if err != nil {
				log.Printf("Failed to auto-create scene for routine: %v", err)
				return apperrors.NewInternalError("Failed to create scene for routine")
			}
			req.SceneID = newScene.SceneID
			log.Printf("Auto-created scene %s for routine %s", newScene.SceneID, req.Name)

			// Also convert speakers to internal format for storage
			req.SpeakersJSON = make([]Speaker, len(req.Speakers))
			for i, s := range req.Speakers {
				vol := s.Volume
				req.SpeakersJSON[i] = Speaker{
					UDN:    s.UDN,
					Volume: &vol,
				}
			}
		}

		// Verify scene exists (either pre-existing or just created)
		existingScene, err := sceneService.GetScene(req.SceneID)
		if err != nil {
			return apperrors.NewInternalError("Failed to verify scene")
		}
		if existingScene == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": req.SceneID}, nil)
		}

		// Process nested music_policy from iOS and flatten to database columns
		if req.MusicPolicy != nil {
			processMusicPolicy(&req.CreateRoutineInput, req.MusicPolicy)
		}

		// Process nested schedule from iOS and flatten to database columns
		if req.Schedule != nil {
			processSchedule(&req.CreateRoutineInput, req.Schedule)
		}

		routine, err := routinesRepo.Create(req.CreateRoutineInput)
		if err != nil {
			log.Printf("Failed to create routine: %v", err)
			return apperrors.NewInternalError("Failed to create routine")
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusCreated, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func listRoutines(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		limit := 20
		offset := 0
		enabledOnly := false

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
		if e := r.URL.Query().Get("enabled"); e != "" {
			enabledOnly = e == "true" || e == "1"
		}

		routines, total, err := routinesRepo.List(limit, offset, enabledOnly)
		if err != nil {
			log.Printf("GET /v1/routines error: %v", err)
			return apperrors.NewInternalError("Failed to list routines")
		}

		log.Printf("GET /v1/routines: returning %d routines (total: %d, limit: %d, offset: %d)", len(routines), total, limit, offset)

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		formatted := make([]map[string]any, 0, len(routines))
		for _, routine := range routines {
			formatted = append(formatted, formatRoutineWithEnrichment(&routine, deviceRoomMap, musicService))
		}

		hasMore := offset+len(routines) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/routines", formatted, hasMore)
	}
}

func getRoutine(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		routine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

// buildDeviceRoomMap creates a map of udn -> room_name from the device service.
// NON-BLOCKING: Returns empty map if topology not yet available.
// Matches Node.js behavior: continue without room names if device registry unavailable.
func buildDeviceRoomMap(deviceService *devices.Service) map[string]string {
	deviceRoomMap := make(map[string]string)
	if deviceService == nil {
		return deviceRoomMap
	}
	// Use non-blocking call - don't trigger discovery
	topology := deviceService.GetTopologyIfCached()
	if topology != nil {
		for _, device := range topology.Devices {
			deviceRoomMap[device.UDN] = device.RoomName
		}
	}
	return deviceRoomMap
}

// updateRoutineRequest is the input structure for updating a routine.
// It supports both scene_id and speakers (to update scene members).
// iOS sends nested music_policy and schedule objects which we flatten to database columns.
type updateRoutineRequest struct {
	UpdateRoutineInput
	Speakers    []SpeakerInput `json:"speakers,omitempty"`     // iOS sends speakers to update scene members
	MusicPolicy *MusicPolicy   `json:"music_policy,omitempty"` // Nested music policy from iOS
	Schedule    *ScheduleInput `json:"schedule,omitempty"`     // Nested schedule from iOS
}

func updateRoutine(routinesRepo *RoutinesRepository, sceneService *scene.Service, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		var req updateRoutineRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Get existing routine to find current scene_id
		existingRoutine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if existingRoutine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// If speakers are provided, update the scene members
		if len(req.Speakers) > 0 {
			// Convert SpeakerInput to SceneMember
			members := make([]scene.SceneMember, len(req.Speakers))
			for i, s := range req.Speakers {
				vol := s.Volume
				members[i] = scene.SceneMember{
					UDN:          s.UDN,
					TargetVolume: &vol,
				}
			}

			// Update existing scene with new members
			sceneID := existingRoutine.SceneID
			if req.SceneID != nil {
				sceneID = *req.SceneID
			}

			_, err := sceneService.UpdateScene(sceneID, scene.UpdateSceneInput{
				Members: members,
			})
			if err != nil {
				log.Printf("Failed to update scene members: %v", err)
				return apperrors.NewInternalError("Failed to update scene")
			}
			log.Printf("Updated scene %s members for routine %s", sceneID, routineID)

			// Also convert speakers to internal format for storage
			req.SpeakersJSON = make([]Speaker, len(req.Speakers))
			for i, s := range req.Speakers {
				vol := s.Volume
				req.SpeakersJSON[i] = Speaker{
					UDN:    s.UDN,
					Volume: &vol,
				}
			}
		}

		// If scene_id is being updated, verify it exists
		if req.SceneID != nil {
			existingScene, err := sceneService.GetScene(*req.SceneID)
			if err != nil {
				return apperrors.NewInternalError("Failed to verify scene")
			}
			if existingScene == nil {
				return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": *req.SceneID}, nil)
			}
		}

		// Process nested music_policy from iOS and flatten to database columns
		if req.MusicPolicy != nil {
			processMusicPolicyUpdate(&req.UpdateRoutineInput, req.MusicPolicy)
		}

		// Process nested schedule from iOS and flatten to database columns
		if req.Schedule != nil {
			processScheduleUpdate(&req.UpdateRoutineInput, req.Schedule)
		}

		routine, err := routinesRepo.Update(routineID, req.UpdateRoutineInput)
		if err != nil {
			return apperrors.NewInternalError("Failed to update routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func deleteRoutine(routinesRepo *RoutinesRepository, sceneService *scene.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		// Get routine to find scene_id before deletion
		routine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Delete routine first (jobs deleted via CASCADE)
		err = routinesRepo.Delete(routineID)
		if err != nil {
			if err == sql.ErrNoRows {
				return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
			}
			return apperrors.NewInternalError("Failed to delete routine")
		}

		// Delete the associated scene (ignore errors, scene may already be deleted)
		if routine.SceneID != "" && sceneService != nil {
			_ = sceneService.DeleteScene(routine.SceneID)
		}

		// Return 204 No Content with empty body (Node.js parity)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func enableRoutine(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		enabled := true
		routine, err := routinesRepo.Update(routineID, UpdateRoutineInput{Enabled: &enabled})
		if err != nil {
			return apperrors.NewInternalError("Failed to enable routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func disableRoutine(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		enabled := false
		routine, err := routinesRepo.Update(routineID, UpdateRoutineInput{Enabled: &enabled})
		if err != nil {
			return apperrors.NewInternalError("Failed to disable routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func triggerRoutine(routinesRepo *RoutinesRepository, jobsRepo *JobsRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		// Verify routine exists
		routine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Create a job scheduled for now (immediate execution)
		job, err := jobsRepo.Create(CreateJobInput{
			RoutineID:    routineID,
			ScheduledFor: time.Now().UTC(),
		})
		if err != nil {
			return apperrors.NewInternalError("Failed to create job")
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusAccepted, formatJob(job))
	}
}

// SnoozeInput represents the request body for snoozing a routine.
type SnoozeInput struct {
	Until time.Time `json:"until"`
}

func snoozeRoutine(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		var input SnoozeInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate snooze time is in the future
		if input.Until.IsZero() {
			return apperrors.NewValidationError("until time is required", nil)
		}
		if input.Until.Before(time.Now()) {
			return apperrors.NewValidationError("until time must be in the future", map[string]any{
				"until":   input.Until.Format(time.RFC3339),
				"current": time.Now().UTC().Format(time.RFC3339),
			})
		}

		routine, err := routinesRepo.Update(routineID, UpdateRoutineInput{SnoozeUntil: &input.Until})
		if err != nil {
			return apperrors.NewInternalError("Failed to snooze routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func unsnoozeRoutine(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		routine, err := routinesRepo.ClearSnooze(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to unsnooze routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

func skipNextOccurrence(routinesRepo *RoutinesRepository, deviceService *devices.Service, musicService *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		skipNext := true
		routine, err := routinesRepo.Update(routineID, UpdateRoutineInput{SkipNext: &skipNext})
		if err != nil {
			return apperrors.NewInternalError("Failed to skip routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Build device room map for speaker enrichment
		deviceRoomMap := buildDeviceRoomMap(deviceService)

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatRoutineWithEnrichment(routine, deviceRoomMap, musicService))
	}
}

// ==========================================================================
// Job Handlers
// ==========================================================================

func getJob(jobsRepo *JobsRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		jobID := chi.URLParam(r, "job_id")

		job, err := jobsRepo.GetByID(jobID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get job")
		}
		if job == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeJobNotFound, "Job not found", 404, map[string]any{"job_id": jobID}, nil)
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatJob(job))
	}
}

func listJobsForRoutine(routinesRepo *RoutinesRepository, jobsRepo *JobsRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		// Verify routine exists
		routine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

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

		jobs, total, err := jobsRepo.ListByRoutineID(routineID, limit, offset)
		if err != nil {
			return apperrors.NewInternalError("Failed to list jobs")
		}

		formatted := make([]map[string]any, 0, len(jobs))
		for _, job := range jobs {
			formatted = append(formatted, formatJob(&job))
		}

		hasMore := offset+len(jobs) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/routines/"+routineID+"/jobs", formatted, hasMore)
	}
}

// ==========================================================================
// Holiday Handlers
// ==========================================================================

// CreateHolidayInput represents the request body for creating a holiday.
type CreateHolidayAPIInput struct {
	Name      string `json:"name"`
	Date      string `json:"date"` // YYYY-MM-DD format
	IsCustom  bool   `json:"is_custom"`
	Recurring bool   `json:"recurring"`
}

func createHoliday(holidaysRepo *HolidaysRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var input CreateHolidayAPIInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if input.Name == "" {
			return apperrors.NewValidationError("name is required", nil)
		}
		if input.Date == "" {
			return apperrors.NewValidationError("date is required", nil)
		}

		// Parse date
		date, err := time.Parse("2006-01-02", input.Date)
		if err != nil {
			return apperrors.NewValidationError("invalid date format, expected YYYY-MM-DD", map[string]any{"date": input.Date})
		}

		holiday, err := holidaysRepo.Create(CreateHolidayInput{
			Date:     date,
			Name:     input.Name,
			IsCustom: input.IsCustom,
		})
		if err != nil {
			return apperrors.NewInternalError("Failed to create holiday")
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusCreated, formatHoliday(holiday))
	}
}

func listHolidays(holidaysRepo *HolidaysRepository) func(w http.ResponseWriter, r *http.Request) error {
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

		holidays, total, err := holidaysRepo.List(limit, offset)
		if err != nil {
			return apperrors.NewInternalError("Failed to list holidays")
		}

		formatted := make([]map[string]any, 0, len(holidays))
		for _, holiday := range holidays {
			formatted = append(formatted, formatHoliday(&holiday))
		}

		hasMore := offset+len(holidays) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/holidays", formatted, hasMore)
	}
}

func getHoliday(holidaysRepo *HolidaysRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		holidayID := chi.URLParam(r, "holiday_id")

		holiday, err := holidaysRepo.GetByID(holidayID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get holiday")
		}
		if holiday == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeHolidayNotFound, "Holiday not found", 404, map[string]any{"holiday_id": holidayID}, nil)
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatHoliday(holiday))
	}
}

func deleteHoliday(holidaysRepo *HolidaysRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		holidayID := chi.URLParam(r, "holiday_id")

		err := holidaysRepo.Delete(holidayID)
		if err != nil {
			if err == sql.ErrNoRows {
				return apperrors.NewAppError(apperrors.ErrorCodeHolidayNotFound, "Holiday not found", 404, map[string]any{"holiday_id": holidayID}, nil)
			}
			return apperrors.NewInternalError("Failed to delete holiday")
		}

		// Return 204 No Content with empty body
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func checkHoliday(holidaysRepo *HolidaysRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		dateStr := r.URL.Query().Get("date")
		if dateStr == "" {
			// Default to today
			dateStr = time.Now().Format("2006-01-02")
		}

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return apperrors.NewValidationError("invalid date format, expected YYYY-MM-DD", map[string]any{"date": dateStr})
		}

		isHoliday, holiday, err := holidaysRepo.IsHolidayWithDetails(date)
		if err != nil {
			return apperrors.NewInternalError("Failed to check holiday")
		}

		result := map[string]any{
			"object":     "holiday_check",
			"date":       dateStr,
			"is_holiday": isHoliday,
		}
		if holiday != nil {
			result["holiday"] = formatHoliday(holiday)
		}

		// Stripe-style: return action result directly
		return api.WriteAction(w, http.StatusOK, result)
	}
}

// ==========================================================================
// Formatters
// ==========================================================================

// formatRoutineWithDeviceMap formats a routine with optional device room name enrichment.
// This mirrors the Node.js formatRoutineWithSpeakers function.
func formatRoutineWithDeviceMap(routine *Routine, deviceRoomMap map[string]string) map[string]any {
	result := map[string]any{
		"object":            api.ObjectRoutine,
		"id":                routine.RoutineID,
		"name":              routine.Name,
		"enabled":           routine.Enabled,
		"timezone":          routine.Timezone,
		"holiday_behavior":  string(routine.HolidayBehavior),
		"scene_id":          routine.SceneID,
		"skip_next":         routine.SkipNext,
		"occasions_enabled": routine.OccasionsEnabled,
		"created_at":        api.RFC3339Millis(routine.CreatedAt),
		"updated_at":        api.RFC3339Millis(routine.UpdatedAt),
	}

	// Build nested schedule object (iOS expected format)
	schedule := map[string]any{
		"type": string(routine.ScheduleType),
		"time": routine.ScheduleTime,
	}
	// Always include weekdays (iOS expects it even when empty)
	if len(routine.ScheduleWeekdays) > 0 {
		schedule["weekdays"] = routine.ScheduleWeekdays
	} else {
		schedule["weekdays"] = []int{}
	}
	if routine.ScheduleMonth != nil {
		schedule["month"] = *routine.ScheduleMonth
	}
	if routine.ScheduleDay != nil {
		schedule["day"] = *routine.ScheduleDay
	}
	result["schedule"] = schedule

	// Build nested music_policy object (iOS expected format)
	// Node.js only includes sonos_favorite_* and music_content for FIXED policy
	if routine.MusicPolicyType != "" {
		musicPolicy := map[string]any{
			"type": string(routine.MusicPolicyType),
		}

		// Only include sonos_favorite_* and music_content for FIXED policy (Node.js API parity)
		if routine.MusicPolicyType == MusicPolicyTypeFixed {
			// Node.js reads sonos_favorite_id from database column
			if routine.MusicSonosFavoriteID != nil {
				musicPolicy["sonos_favorite_id"] = *routine.MusicSonosFavoriteID
			} else {
				musicPolicy["sonos_favorite_id"] = nil
			}

			// Node.js extracts these fields from music_content_json ONLY when type is "sonos_favorite"
			// It does NOT read from the legacy database columns (music_sonos_favorite_name, etc.)
			var sonosFavoriteName, sonosFavoriteArtworkUrl, sonosFavoriteServiceLogoUrl, sonosFavoriteServiceName any
			sonosFavoriteName = nil
			sonosFavoriteArtworkUrl = nil
			sonosFavoriteServiceLogoUrl = nil
			sonosFavoriteServiceName = nil

			var musicContentForAPI any
			musicContentForAPI = nil

			if routine.MusicContentJSON != nil && *routine.MusicContentJSON != "" {
				var content map[string]any
				if err := json.Unmarshal([]byte(*routine.MusicContentJSON), &content); err == nil {
					contentType, _ := content["type"].(string)

					if contentType == "sonos_favorite" {
						// Extract metadata from sonos_favorite content
						if name, ok := content["name"]; ok {
							sonosFavoriteName = name
						}
						if artworkUrl, ok := content["artworkUrl"]; ok {
							sonosFavoriteArtworkUrl = artworkUrl
						}
						if serviceLogoUrl, ok := content["serviceLogoUrl"]; ok {
							sonosFavoriteServiceLogoUrl = serviceLogoUrl
						}
						if serviceName, ok := content["serviceName"]; ok {
							sonosFavoriteServiceName = serviceName
						}
					}

					// Only include music_content for direct type (not sonos_favorite)
					// iOS DirectMusicContent struct requires service, content_type, content_id fields
					// which sonos_favorite doesn't have - it uses the extracted metadata fields above
					if contentType == "direct" {
						// Transform camelCase keys to snake_case for API response
						normalized := make(map[string]any)
						for k, v := range content {
							switch k {
							case "contentType":
								normalized["content_type"] = v
							case "contentId":
								normalized["content_id"] = v
							case "artworkUrl":
								normalized["artwork_url"] = v
							case "favoriteId":
								normalized["favorite_id"] = v
							case "serviceLogoUrl":
								normalized["service_logo_url"] = v
							case "serviceName":
								normalized["service_name"] = v
							default:
								normalized[k] = v
							}
						}
						musicContentForAPI = normalized
					}
				}
			}

			musicPolicy["sonos_favorite_name"] = sonosFavoriteName
			musicPolicy["sonos_favorite_artwork_url"] = sonosFavoriteArtworkUrl
			musicPolicy["sonos_favorite_service_logo_url"] = sonosFavoriteServiceLogoUrl
			musicPolicy["sonos_favorite_service_name"] = sonosFavoriteServiceName
			musicPolicy["music_content"] = musicContentForAPI
		}

		// Only include set_id, fallback_behavior, no_repeat_window_minutes for SHUFFLE/ROTATION (Node.js API parity)
		// FIXED policy does NOT include these fields
		if routine.MusicPolicyType != MusicPolicyTypeFixed {
			if routine.MusicSetID != nil {
				musicPolicy["set_id"] = *routine.MusicSetID
			} else {
				musicPolicy["set_id"] = nil
			}
			if routine.MusicFallbackBehavior != nil {
				musicPolicy["fallback_behavior"] = *routine.MusicFallbackBehavior
			} else {
				musicPolicy["fallback_behavior"] = nil
			}
			if routine.MusicNoRepeatWindowMinutes != nil {
				musicPolicy["no_repeat_window_minutes"] = *routine.MusicNoRepeatWindowMinutes
			} else {
				musicPolicy["no_repeat_window_minutes"] = nil
			}
		}

		result["music_policy"] = musicPolicy
	}

	// Build music_set object for display (iOS expected format)
	// Node.js always includes music_set for FIXED policy (object or null)
	// See Node.js formatRoutineWithSpeakers lines 225-245
	if routine.MusicPolicyType == MusicPolicyTypeFixed {
		var musicSetValue any = nil

		if routine.MusicContentJSON != nil && *routine.MusicContentJSON != "" {
			var content map[string]any
			if err := json.Unmarshal([]byte(*routine.MusicContentJSON), &content); err == nil {
				contentType, _ := content["type"].(string)

				if contentType == "direct" {
					// Direct content: use title, artworkUrl, service
					service, _ := content["service"].(string)
					serviceName := service
					if serviceName == "apple_music" {
						serviceName = "Apple Music"
					} else if serviceName == "spotify" {
						serviceName = "Spotify"
					}
					title, _ := content["title"].(string)
					artworkUrl, _ := content["artworkUrl"].(string)
					var artworkUrlVal any = nil
					if artworkUrl != "" {
						artworkUrlVal = artworkUrl
					}
					musicSetValue = map[string]any{
						"name":             title,
						"artwork_url":      artworkUrlVal,
						"service_logo_url": nil,
						"service_name":     serviceName,
					}
				} else if contentType == "sonos_favorite" {
					// Sonos favorite: use name and artworkUrl from content if present
					name, nameOk := content["name"].(string)
					artworkUrl, artworkOk := content["artworkUrl"].(string)

					// Node.js: if sonosFavoriteName || sonosFavoriteArtworkUrl
					if (nameOk && name != "") || (artworkOk && artworkUrl != "") {
						var nameVal, artworkVal, serviceLogoVal, serviceNameVal any = nil, nil, nil, nil
						if nameOk && name != "" {
							nameVal = name
						}
						if artworkOk && artworkUrl != "" {
							artworkVal = artworkUrl
						}
						if serviceLogoUrl, ok := content["serviceLogoUrl"].(string); ok && serviceLogoUrl != "" {
							serviceLogoVal = serviceLogoUrl
						}
						if serviceName, ok := content["serviceName"].(string); ok && serviceName != "" {
							serviceNameVal = serviceName
						}
						musicSetValue = map[string]any{
							"name":             nameVal,
							"artwork_url":      artworkVal,
							"service_logo_url": serviceLogoVal,
							"service_name":     serviceNameVal,
						}
					}
				}
			}
		}

		result["music_set"] = musicSetValue
	}

	// Build nested constraints object (iOS expected format)
	constraints := map[string]any{}
	if routine.ArcTVPolicy != nil {
		constraints["arc_tv_policy"] = *routine.ArcTVPolicy
	} else {
		constraints["arc_tv_policy"] = nil
	}
	result["constraints"] = constraints

	// Template ID
	if routine.TemplateID != nil {
		result["template_id"] = *routine.TemplateID
	} else {
		result["template_id"] = nil
	}

	if routine.Description != nil {
		result["description"] = *routine.Description
	}
	if routine.SnoozeUntil != nil && !routine.SnoozeUntil.IsZero() {
		result["snooze_until"] = api.RFC3339Millis(*routine.SnoozeUntil)
	}

	// Speakers with room_name enrichment from device registry
	// Node.js always includes speakers array (empty if none)
	speakers := make([]map[string]any, 0, len(routine.SpeakersJSON))
	for _, s := range routine.SpeakersJSON {
		speaker := map[string]any{"udn": s.UDN}
		if s.Volume != nil {
			speaker["volume"] = *s.Volume
		} else {
			speaker["volume"] = nil
		}
		// Add room_name from device registry lookup
		if deviceRoomMap != nil {
			if roomName, ok := deviceRoomMap[s.UDN]; ok {
				speaker["room_name"] = roomName
			} else {
				speaker["room_name"] = nil
			}
		} else {
			speaker["room_name"] = nil
		}
		speakers = append(speakers, speaker)
	}
	result["speakers"] = speakers
	if routine.LastRunAt != nil {
		result["last_run_at"] = api.RFC3339Millis(*routine.LastRunAt)
	}
	if routine.NextRunAt != nil {
		result["next_run_at"] = api.RFC3339Millis(*routine.NextRunAt)
	}

	return result
}

// formatRoutineWithEnrichment formats a routine with device and music set enrichment.
// For ROTATION/SHUFFLE policies, fetches enrichment data from the music set to populate artwork.
func formatRoutineWithEnrichment(routine *Routine, deviceRoomMap map[string]string, musicService *music.Service) map[string]any {
	result := formatRoutineWithDeviceMap(routine, deviceRoomMap)

	// For ROTATION/SHUFFLE policies, fetch enrichment from the music set
	// This provides artwork_url from the first item in the set
	if (routine.MusicPolicyType == MusicPolicyTypeRotation || routine.MusicPolicyType == MusicPolicyTypeShuffle) && musicService != nil {
		if routine.MusicSetID != nil && *routine.MusicSetID != "" {
			enrichment, err := musicService.GetSetEnrichment(*routine.MusicSetID)
			if err == nil && enrichment != nil {
				musicSet := map[string]any{
					"name":             enrichment.Name,
					"artwork_url":      nil,
					"service_logo_url": nil,
					"service_name":     nil,
				}
				if enrichment.ArtworkURL != nil {
					musicSet["artwork_url"] = *enrichment.ArtworkURL
				}
				if enrichment.ServiceLogoURL != nil {
					musicSet["service_logo_url"] = *enrichment.ServiceLogoURL
				}
				if enrichment.ServiceName != nil {
					musicSet["service_name"] = *enrichment.ServiceName
				}
				result["music_set"] = musicSet
			} else {
				result["music_set"] = nil
			}
		} else {
			result["music_set"] = nil
		}
	}

	return result
}

// formatRoutine is a convenience wrapper for formatRoutineWithDeviceMap without device enrichment.
func formatRoutine(routine *Routine) map[string]any {
	return formatRoutineWithDeviceMap(routine, nil)
}

func formatJob(job *Job) map[string]any {
	result := map[string]any{
		"object":        api.ObjectJob,
		"id":            job.JobID,
		"routine_id":    job.RoutineID,
		"scheduled_for": api.RFC3339Millis(job.ScheduledFor),
		"status":        string(job.Status),
		"attempts":      job.Attempts,
		"created_at":    api.RFC3339Millis(job.CreatedAt),
		"updated_at":    api.RFC3339Millis(job.UpdatedAt),
	}

	if job.LastError != nil {
		result["last_error"] = *job.LastError
	}
	if job.SceneExecutionID != nil {
		result["scene_execution_id"] = *job.SceneExecutionID
	}
	if job.RetryAfter != nil {
		result["retry_after"] = api.RFC3339Millis(*job.RetryAfter)
	}
	if job.ClaimedAt != nil {
		result["claimed_at"] = api.RFC3339Millis(*job.ClaimedAt)
	}
	if job.IdempotencyKey != nil {
		result["idempotency_key"] = *job.IdempotencyKey
	}
	if job.StartedAt != nil {
		result["started_at"] = api.RFC3339Millis(*job.StartedAt)
	}
	if job.CompletedAt != nil {
		result["completed_at"] = api.RFC3339Millis(*job.CompletedAt)
	}
	if job.Result != nil {
		result["result"] = *job.Result
	}

	return result
}

// formatJobAsExecution formats a job as an execution matching Node.js /v1/executions format
func formatJobAsExecution(job *Job, routineNames map[string]string) map[string]any {
	// Map job status to iOS outcome
	outcome := "failed"
	switch job.Status {
	case JobStatusSkipped:
		outcome = "skipped"
	case JobStatusFailed:
		outcome = "failed"
	case JobStatusCompleted:
		outcome = "success"
	}

	// Look up routine name from map
	routineName := "Unknown Routine"
	if name, ok := routineNames[job.RoutineID]; ok {
		routineName = name
	}

	result := map[string]any{
		"object":          "execution", // Used for iOS executions list view
		"id":              job.JobID,
		"routine_id":      job.RoutineID,
		"routine_name":    routineName,
		"timestamp":       api.RFC3339Millis(job.ScheduledFor),
		"outcome":         outcome,
		"target_devices":  []string{},
		"content_played":  nil,
		"failure_reason":  nil,
		"failure_message": nil,
		"fallback_used":   false,
	}

	if job.Status == JobStatusFailed {
		result["failure_reason"] = "execution_failed"
	}
	if job.LastError != nil {
		result["failure_message"] = *job.LastError
	}

	return result
}

func formatHoliday(holiday *Holiday) map[string]any {
	// Use HolidayID if set, otherwise fall back to date
	id := holiday.Date
	if holiday.HolidayID != "" {
		id = holiday.HolidayID
	}

	result := map[string]any{
		"object":    api.ObjectHoliday,
		"id":        id,
		"date":      holiday.Date,
		"name":      holiday.Name,
		"is_custom": holiday.IsCustom,
	}

	if holiday.Recurring {
		result["recurring"] = holiday.Recurring
	}
	if !holiday.CreatedAt.IsZero() {
		result["created_at"] = api.RFC3339Millis(holiday.CreatedAt)
	}

	return result
}

// ==========================================================================
// Music Policy Processing
// ==========================================================================

// processMusicPolicy extracts nested music_policy from iOS request and flattens
// to database columns. It also builds music_content_json with serviceLogoUrl.
func processMusicPolicy(input *CreateRoutineInput, policy *MusicPolicy) {
	if policy == nil {
		return
	}

	// Set music policy type from nested policy
	if policy.Type != "" {
		policyType := MusicPolicyType(policy.Type)
		input.MusicPolicyType = policyType
	}

	// For FIXED policy, extract Sonos favorite fields
	if policy.Type == "FIXED" {
		if policy.SonosFavoriteID != nil {
			input.MusicSonosFavoriteID = policy.SonosFavoriteID
		}

		// Build music_content_json with all metadata including serviceLogoUrl
		if policy.SonosFavoriteID != nil || policy.MusicContent != nil {
			contentJSON := buildMusicContentJSON(policy)
			if contentJSON != "" {
				input.MusicContentJSON = &contentJSON
			}
		}
	}

	// For ROTATION/SHUFFLE policy, extract set fields
	if policy.Type == "ROTATION" || policy.Type == "SHUFFLE" {
		if policy.SetID != nil {
			input.MusicSetID = policy.SetID
		}
		if policy.NoRepeatWindow != nil {
			input.MusicNoRepeatWindowMinutes = policy.NoRepeatWindow
		}
		if policy.NoRepeatWindowMinutes != nil {
			input.MusicNoRepeatWindowMinutes = policy.NoRepeatWindowMinutes
		}
		if policy.FallbackBehavior != nil {
			input.MusicFallbackBehavior = policy.FallbackBehavior
		}
	}
}

// processMusicPolicyUpdate extracts nested music_policy from iOS request for updates.
func processMusicPolicyUpdate(input *UpdateRoutineInput, policy *MusicPolicy) {
	if policy == nil {
		return
	}

	// Set music policy type from nested policy
	if policy.Type != "" {
		policyType := MusicPolicyType(policy.Type)
		input.MusicPolicyType = &policyType
	}

	// For FIXED policy, extract Sonos favorite fields
	if policy.Type == "FIXED" {
		if policy.SonosFavoriteID != nil {
			input.MusicSonosFavoriteID = policy.SonosFavoriteID
		}

		// Build music_content_json with all metadata including serviceLogoUrl
		if policy.SonosFavoriteID != nil || policy.MusicContent != nil {
			contentJSON := buildMusicContentJSON(policy)
			if contentJSON != "" {
				input.MusicContentJSON = &contentJSON
			}
		}
	}

	// For ROTATION/SHUFFLE policy, extract set fields
	if policy.Type == "ROTATION" || policy.Type == "SHUFFLE" {
		if policy.SetID != nil {
			input.MusicSetID = policy.SetID
		}
		if policy.NoRepeatWindow != nil {
			input.MusicNoRepeatWindowMinutes = policy.NoRepeatWindow
		}
		if policy.NoRepeatWindowMinutes != nil {
			input.MusicNoRepeatWindowMinutes = policy.NoRepeatWindowMinutes
		}
		if policy.FallbackBehavior != nil {
			input.MusicFallbackBehavior = policy.FallbackBehavior
		}
	}
}

// buildMusicContentJSON constructs the music_content_json string from music policy.
// This JSON is stored in the database and used to populate music_set display info.
// Node.js format: {"type":"sonos_favorite","favoriteId":"FV:2/77","name":"Title","artworkUrl":"...","serviceLogoUrl":"...","serviceName":"..."}
func buildMusicContentJSON(policy *MusicPolicy) string {
	if policy == nil {
		return ""
	}

	content := make(map[string]any)

	// Handle direct music content (Apple Music, Spotify)
	if policy.MusicContent != nil && policy.MusicContent.Type == "direct" {
		content["type"] = "direct"
		if policy.MusicContent.Service != nil {
			content["service"] = *policy.MusicContent.Service
		}
		if policy.MusicContent.ContentType != nil {
			content["contentType"] = *policy.MusicContent.ContentType
		}
		if policy.MusicContent.ContentID != nil {
			content["contentId"] = *policy.MusicContent.ContentID
		}
		if policy.MusicContent.Title != nil {
			content["title"] = *policy.MusicContent.Title
		}
		if policy.MusicContent.ArtworkUrl != nil {
			content["artworkUrl"] = *policy.MusicContent.ArtworkUrl
		}
	} else if policy.SonosFavoriteID != nil {
		// Sonos favorite content
		content["type"] = "sonos_favorite"
		content["favoriteId"] = *policy.SonosFavoriteID

		if policy.SonosFavoriteName != nil {
			content["name"] = *policy.SonosFavoriteName
		}
		if policy.SonosFavoriteArtworkUrl != nil {
			content["artworkUrl"] = *policy.SonosFavoriteArtworkUrl
		}
		if policy.SonosFavoriteServiceLogoUrl != nil {
			content["serviceLogoUrl"] = *policy.SonosFavoriteServiceLogoUrl
		}
		if policy.SonosFavoriteServiceName != nil {
			content["serviceName"] = *policy.SonosFavoriteServiceName
		}
	} else {
		return ""
	}

	data, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	return string(data)
}

// ==========================================================================
// Schedule Processing
// ==========================================================================

// processSchedule extracts nested schedule from iOS request and flattens
// to database columns for routine creation.
func processSchedule(input *CreateRoutineInput, schedule *ScheduleInput) {
	if schedule == nil {
		return
	}

	if schedule.Type != "" {
		input.ScheduleType = ScheduleType(schedule.Type)
	}
	if len(schedule.Weekdays) > 0 {
		input.ScheduleWeekdays = schedule.Weekdays
	}
	if schedule.Month != nil {
		input.ScheduleMonth = schedule.Month
	}
	if schedule.Day != nil {
		input.ScheduleDay = schedule.Day
	}
	if schedule.Time != "" {
		input.ScheduleTime = schedule.Time
	}
}

// processScheduleUpdate extracts nested schedule from iOS request and flattens
// to database columns for routine updates.
func processScheduleUpdate(input *UpdateRoutineInput, schedule *ScheduleInput) {
	if schedule == nil {
		return
	}

	if schedule.Type != "" {
		st := ScheduleType(schedule.Type)
		input.ScheduleType = &st
	}
	if len(schedule.Weekdays) > 0 {
		input.ScheduleWeekdays = schedule.Weekdays
	}
	if schedule.Month != nil {
		input.ScheduleMonth = schedule.Month
	}
	if schedule.Day != nil {
		input.ScheduleDay = schedule.Day
	}
	if schedule.Time != "" {
		input.ScheduleTime = &schedule.Time
	}
}

// ==========================================================================
// Additional Routine Handlers
// ==========================================================================

func unskipNextOccurrence(routinesRepo *RoutinesRepository, _ *devices.Service, _ *music.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		skipNext := false
		routine, err := routinesRepo.Update(routineID, UpdateRoutineInput{SkipNext: &skipNext})
		if err != nil {
			return apperrors.NewInternalError("Failed to unskip routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Stripe-style: return action result directly
		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":     "unskip",
			"message":    "Skip cancelled - routine will execute as scheduled",
			"routine_id": routine.RoutineID,
			"skip_next":  false,
		})
	}
}

// RunRoutineInput represents the request body for running a routine with options.
type RunRoutineInput struct {
	DeviceOverride *string `json:"device_override,omitempty"`
}

func runRoutine(routinesRepo *RoutinesRepository, jobsRepo *JobsRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		routineID := chi.URLParam(r, "routine_id")

		var input RunRoutineInput
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				return apperrors.NewValidationError("invalid request body", nil)
			}
		}

		// Verify routine exists
		routine, err := routinesRepo.GetByID(routineID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get routine")
		}
		if routine == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeRoutineNotFound, "Routine not found", 404, map[string]any{"routine_id": routineID}, nil)
		}

		// Create a job scheduled for now (immediate execution)
		job, err := jobsRepo.Create(CreateJobInput{
			RoutineID:    routineID,
			ScheduledFor: time.Now().UTC(),
		})
		if err != nil {
			return apperrors.NewInternalError("Failed to create job")
		}

		jobResponse := formatJob(job)
		if input.DeviceOverride != nil {
			jobResponse["device_override"] = *input.DeviceOverride
		}

		// Stripe-style: return resource directly (202 Accepted for async execution)
		return api.WriteResource(w, http.StatusAccepted, jobResponse)
	}
}

// TestRoutineInput represents the request body for testing a routine without saving.
type TestRoutineInput struct {
	SceneID string   `json:"scene_id"`
	UDNs    []string `json:"udns,omitempty"`
}

func testRoutine(sceneService *scene.Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var input TestRoutineInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if input.SceneID == "" {
			return apperrors.NewValidationError("scene_id is required", nil)
		}

		// Verify scene exists
		existingScene, err := sceneService.GetScene(input.SceneID)
		if err != nil {
			return apperrors.NewInternalError("Failed to verify scene")
		}
		if existingScene == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeSceneNotFound, "Scene not found", 404, map[string]any{"scene_id": input.SceneID}, nil)
		}

		// For test execution, we just validate and return a preview
		// Actual execution would require more infrastructure
		// Stripe-style: return action result directly
		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":     "routine_test",
			"status":     "validated",
			"scene_id":   input.SceneID,
			"scene_name": existingScene.Name,
			"udns":       input.UDNs,
			"message":    "Routine configuration is valid",
		})
	}
}

// ==========================================================================
// Execution Handlers
// ==========================================================================

func listExecutions(jobsRepo *JobsRepository, routinesRepo *RoutinesRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		limit := 50 // Match Node.js default
		offset := 0
		statusFilter := ""

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
		statusFilter = r.URL.Query().Get("status")

		jobs, total, err := jobsRepo.ListAll(limit, offset, statusFilter)
		if err != nil {
			return apperrors.NewInternalError("Failed to list executions")
		}

		// Build routine name map for display
		routineNames := make(map[string]string)
		routines, _, err := routinesRepo.List(1000, 0, false) // Get all routines
		if err == nil {
			for _, routine := range routines {
				routineNames[routine.RoutineID] = routine.Name
			}
		}

		// Format jobs as executions matching Node.js structure
		executions := make([]map[string]any, 0, len(jobs))
		for _, job := range jobs {
			executions = append(executions, formatJobAsExecution(&job, routineNames))
		}

		hasMore := offset+len(jobs) < total
		// Stripe-style list response
		return api.WriteList(w, "/v1/executions", executions, hasMore)
	}
}

func retryExecution(jobsRepo *JobsRepository) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		executionID := chi.URLParam(r, "execution_id")

		// Get the original job
		originalJob, err := jobsRepo.GetByID(executionID)
		if err != nil {
			return apperrors.NewInternalError("Failed to get execution")
		}
		if originalJob == nil {
			return apperrors.NewAppError(apperrors.ErrorCodeJobNotFound, "Execution not found", 404, map[string]any{"execution_id": executionID}, nil)
		}

		// Only allow retrying failed jobs
		if originalJob.Status != JobStatusFailed {
			return apperrors.NewConflictError("Only failed executions can be retried", map[string]any{
				"execution_id": executionID,
				"status":       string(originalJob.Status),
			})
		}

		// Create a new job for retry
		newJob, err := jobsRepo.Create(CreateJobInput{
			RoutineID:    originalJob.RoutineID,
			ScheduledFor: time.Now().UTC(),
		})
		if err != nil {
			return apperrors.NewInternalError("Failed to create retry job")
		}

		// Stripe-style: return action result directly
		return api.WriteAction(w, http.StatusAccepted, map[string]any{
			"object":           "retry",
			"execution_id":     executionID,
			"new_execution_id": newJob.JobID,
			"status":           "STARTED",
		})
	}
}
