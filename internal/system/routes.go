package system

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// RegisterRoutes wires system routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Method(http.MethodGet, "/v1/system/info", api.Handler(getSystemInfo(service)))
	router.Method(http.MethodGet, "/v1/dashboard", api.Handler(getDashboard(service)))
}

// getSystemInfo handles GET /v1/system/info
func getSystemInfo(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		info, err := service.GetSystemInfo()
		if err != nil {
			return apperrors.NewInternalError("Failed to get system info")
		}

		return api.SingleResponse(w, r, http.StatusOK, "info", formatSystemInfo(info))
	}
}

// getDashboard handles GET /v1/dashboard
func getDashboard(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		data, err := service.GetDashboardData()
		if err != nil {
			return apperrors.NewInternalError("Failed to get dashboard data")
		}

		return api.SingleResponse(w, r, http.StatusOK, "dashboard", formatDashboardData(data))
	}
}

// formatSystemInfo formats SystemInfo for JSON response.
// Matches Node.js system.ts SystemInfoResponse interface exactly.
func formatSystemInfo(info *SystemInfo) map[string]any {
	result := map[string]any{
		"hub_version":       info.HubVersion,
		"uptime_seconds":    info.Uptime,
		"memory_mb":         info.MemoryUsageMB,
		"cpu_percent":       0, // Go doesn't have easy CPU percent - kept for iOS backwards compat
		"redis_connected":   true, // Kept for iOS backwards compatibility (Redis removed)
		"sqlite_connected":  info.SQLiteConnected,
		"devices_online":    info.DevicesOnline,
		"devices_total":     info.DevicesTotal,
		"scheduler_running": info.SchedulerRunning,
	}

	if info.LastDiscovery != nil {
		result["last_discovery"] = info.LastDiscovery.UTC().Format(time.RFC3339)
	} else {
		result["last_discovery"] = nil
	}

	return result
}

// formatDashboardData formats DashboardData for JSON response.
func formatDashboardData(data *DashboardData) map[string]any {
	result := map[string]any{
		"upcoming_routines": formatRoutineSummaries(data.UpcomingRoutines),
		"attention_items":   formatAttentionItems(data.AttentionItems),
	}

	// Always include "next_up" for API parity with Node.js (null if not set)
	if data.NextRoutine != nil {
		result["next_up"] = formatRoutineSummary(data.NextRoutine)
	} else {
		result["next_up"] = nil
	}

	return result
}

// formatRoutineSummaries formats a slice of RoutineSummary for JSON response.
func formatRoutineSummaries(routines []RoutineSummary) []map[string]any {
	result := make([]map[string]any, 0, len(routines))
	for _, r := range routines {
		result = append(result, formatRoutineSummary(&r))
	}
	return result
}

// formatRoutineSummary formats a single RoutineSummary for JSON response.
// Uses iOS-expected field names: routine_name, scheduled_time
func formatRoutineSummary(r *RoutineSummary) map[string]any {
	result := map[string]any{
		"routine_id":   r.RoutineID,
		"routine_name": r.Name, // iOS expects "routine_name" not "name"
	}

	if r.NextRunAt != nil {
		result["scheduled_time"] = r.NextRunAt.UTC().Format(time.RFC3339) // iOS expects "scheduled_time"
	}

	// Optional enrichment fields
	if len(r.TargetRooms) > 0 {
		result["target_rooms"] = r.TargetRooms
	}
	if r.MusicPreview != nil {
		result["music_preview"] = *r.MusicPreview
	}
	if r.ArtworkURL != nil {
		result["artwork_url"] = *r.ArtworkURL
	}
	if r.TemplateID != nil {
		result["template_id"] = *r.TemplateID
	}

	return result
}

// formatAttentionItems formats a slice of AttentionItem for JSON response.
func formatAttentionItems(items []AttentionItem) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		formatted := map[string]any{
			"type":     item.Type,
			"severity": item.Severity,
			"message":  item.Message,
		}
		if item.Details != nil {
			formatted["details"] = item.Details
		}
		if item.ResolveHint != "" {
			formatted["resolve_hint"] = item.ResolveHint
		}
		result = append(result, formatted)
	}
	return result
}
