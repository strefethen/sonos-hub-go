package system

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSystemInfoDefaults(t *testing.T) {
	info := SystemInfo{
		HubVersion:       "1.0.0",
		Uptime:           3600,
		MemoryUsageMB:    50.5,
		SQLiteConnected:  true,
		DevicesOnline:    4,
		DevicesTotal:     5,
		SchedulerRunning: true,
	}

	require.Equal(t, "1.0.0", info.HubVersion)
	require.Equal(t, int64(3600), info.Uptime)
	require.Equal(t, 50.5, info.MemoryUsageMB)
	require.True(t, info.SQLiteConnected)
	require.Equal(t, 4, info.DevicesOnline)
	require.Equal(t, 5, info.DevicesTotal)
	require.True(t, info.SchedulerRunning)
	require.Nil(t, info.LastDiscovery)
}

func TestSystemInfoWithLastDiscovery(t *testing.T) {
	now := time.Now()
	info := SystemInfo{
		HubVersion:    "1.0.0",
		LastDiscovery: &now,
	}

	require.NotNil(t, info.LastDiscovery)
	require.Equal(t, now, *info.LastDiscovery)
}

func TestRoutineSummary(t *testing.T) {
	nextRun := time.Now().Add(time.Hour)
	summary := RoutineSummary{
		RoutineID: "routine-123",
		Name:      "Morning Wake Up",
		NextRunAt: &nextRun,
		SceneID:   "scene-456",
		Enabled:   true,
	}

	require.Equal(t, "routine-123", summary.RoutineID)
	require.Equal(t, "Morning Wake Up", summary.Name)
	require.NotNil(t, summary.NextRunAt)
	require.Equal(t, "scene-456", summary.SceneID)
	require.True(t, summary.Enabled)
}

func TestAttentionItem(t *testing.T) {
	item := AttentionItem{
		Type:     "device_offline",
		Severity: "warning",
		Message:  "Some devices are offline",
		Details: map[string]any{
			"offline_count": 2,
		},
		ResolveHint: "Check device power and network connectivity",
	}

	require.Equal(t, "device_offline", item.Type)
	require.Equal(t, "warning", item.Severity)
	require.Equal(t, "Some devices are offline", item.Message)
	require.NotNil(t, item.Details)
	require.Equal(t, 2, item.Details["offline_count"])
	require.Equal(t, "Check device power and network connectivity", item.ResolveHint)
}

func TestDashboardData(t *testing.T) {
	nextRun := time.Now().Add(time.Hour)
	nextRoutine := RoutineSummary{
		RoutineID: "routine-123",
		Name:      "Next Routine",
		NextRunAt: &nextRun,
		SceneID:   "scene-456",
		Enabled:   true,
	}

	dashboard := DashboardData{
		NextRoutine: &nextRoutine,
		UpcomingRoutines: []RoutineSummary{
			nextRoutine,
			{RoutineID: "routine-456", Name: "Another Routine", Enabled: true},
		},
		AttentionItems: []AttentionItem{
			{Type: "failed_jobs", Severity: "error", Message: "Some routines failed"},
		},
	}

	require.NotNil(t, dashboard.NextRoutine)
	require.Equal(t, "routine-123", dashboard.NextRoutine.RoutineID)
	require.Len(t, dashboard.UpcomingRoutines, 2)
	require.Len(t, dashboard.AttentionItems, 1)
	require.Equal(t, "failed_jobs", dashboard.AttentionItems[0].Type)
}

func TestDashboardDataEmpty(t *testing.T) {
	dashboard := DashboardData{
		UpcomingRoutines: []RoutineSummary{},
		AttentionItems:   []AttentionItem{},
	}

	require.Nil(t, dashboard.NextRoutine)
	require.Empty(t, dashboard.UpcomingRoutines)
	require.Empty(t, dashboard.AttentionItems)
}

func TestVersionDefault(t *testing.T) {
	// Version should have a default value
	require.NotEmpty(t, Version)
}
