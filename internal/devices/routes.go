package devices

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// rfc3339Millis formats time with milliseconds to match Node.js ISO format
func rfc3339Millis(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// RegisterRoutes wires device routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Method(http.MethodGet, "/v1/devices", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		devices, err := service.GetDevices()
		if err != nil {
			return apperrors.NewInternalError("Failed to load devices")
		}

		targetable := dedupeDevices(devices)
		formatted := make([]map[string]any, 0, len(targetable))
		for _, device := range targetable {
			formatted = append(formatted, formatDevice(device))
		}

		// Small fixed list - no pagination needed, use ListResponse
		return api.ListResponse(w, r, http.StatusOK, "devices", formatted, nil)
	}))

	router.Method(http.MethodGet, "/v1/devices/{device_id}", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		deviceID := chi.URLParam(r, "device_id")

		device, err := service.GetDevice(deviceID)
		if err != nil {
			return apperrors.NewInternalError("Failed to load device")
		}
		if device == nil {
			return apperrors.NewNotFoundResource("Device", deviceID)
		}

		return api.SingleResponse(w, r, http.StatusOK, "device", formatDevice(*device))
	}))

	router.Method(http.MethodPost, "/v1/devices/rescan", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		count, durationMs, err := service.Rescan()
		if err != nil {
			return apperrors.NewInternalError("Device rescan failed")
		}

		return api.ActionResponse(w, r, http.StatusOK, map[string]any{
			"devices_found": count,
			"duration_ms":   durationMs,
		})
	}))

	router.Method(http.MethodGet, "/v1/devices/topology", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		topology, err := service.GetTopology()
		if err != nil {
			return apperrors.NewInternalError("Failed to load topology")
		}

		devices := make([]map[string]any, 0, len(topology.Devices))
		for _, device := range topology.Devices {
			devices = append(devices, formatDevice(device))
		}

		homeTheaterGroups := make([]map[string]any, 0, len(topology.HomeTheaterGroups))
		for _, group := range topology.HomeTheaterGroups {
			var sub any
			if group.Sub != nil {
				sub = formatPhysicalDevice(*group.Sub)
			}
			homeTheaterGroups = append(homeTheaterGroups, map[string]any{
				"group_id": group.GroupID,
				"master":   formatPhysicalDevice(group.Master),
				"surrounds": func() []map[string]any {
					items := make([]map[string]any, 0, len(group.Surrounds))
					for _, surround := range group.Surrounds {
						items = append(items, formatPhysicalDevice(surround))
					}
					return items
				}(),
				"sub": sub,
			})
		}

		stereoPairs := make([]map[string]any, 0, len(topology.StereoPairs))
		for _, pair := range topology.StereoPairs {
			stereoPairs = append(stereoPairs, map[string]any{
				"pair_id":   pair.PairID,
				"room_name": pair.RoomName,
				"left":      formatPhysicalDevice(pair.Left),
				"right":     formatPhysicalDevice(pair.Right),
			})
		}

		return api.SingleResponse(w, r, http.StatusOK, "topology", map[string]any{
			"devices":             devices,
			"home_theater_groups": homeTheaterGroups,
			"stereo_pairs":        stereoPairs,
			"updated_at":          topology.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}))

	router.Method(http.MethodGet, "/v1/devices/stats", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		topology, err := service.GetTopology()
		if err != nil {
			return api.SingleResponse(w, r, http.StatusOK, "stats", map[string]any{
				"total":          0,
				"online":         0,
				"offline":        0,
				"last_discovery": nil,
			})
		}

		online := 0
		offline := 0
		for _, device := range topology.Devices {
			switch device.Health {
			case DeviceHealthOK:
				online++
			case DeviceHealthOffline, DeviceHealthDegraded:
				offline++
			}
		}

		return api.SingleResponse(w, r, http.StatusOK, "stats", map[string]any{
			"total":          len(topology.Devices),
			"online":         online,
			"offline":        offline,
			"last_discovery": topology.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}))
}

func formatDevice(device LogicalDevice) map[string]any {
	var primaryUDN any = nil
	if len(device.PhysicalDevices) > 0 {
		primaryUDN = device.PhysicalDevices[0].UDN
	}

	logicalGroup := any(nil)
	if device.LogicalGroupID != "" {
		logicalGroup = device.LogicalGroupID
	}

	physicalCount := len(device.PhysicalDevices)
	if physicalCount == 0 {
		physicalCount = 1
	}

	return map[string]any{
		"device_id":              device.DeviceID,
		"udn":                    primaryUDN,
		"room_name":              device.RoomName,
		"ip":                     device.IP,
		"model":                  device.Model,
		"role":                   device.Role,
		"is_targetable":          device.IsTargetable,
		"is_coordinator_capable": device.IsCoordinatorCapable,
		"supports_airplay":       device.SupportsAirPlay,
		"logical_group_id":       logicalGroup,
		"last_seen_at":           rfc3339Millis(device.LastSeenAt),
		"physical_device_count":  physicalCount,
	}
}

func formatPhysicalDevice(device PhysicalDevice) map[string]any {
	return map[string]any{
		"device_id":              device.DeviceID,
		"udn":                    device.UDN,
		"model":                  device.Model,
		"model_number":           device.ModelNumber,
		"room_name":              device.RoomName,
		"role":                   device.Role,
		"is_coordinator_capable": device.IsCoordinatorCapable,
		"supports_airplay":       device.SupportsAirPlay,
		"last_seen_at":           rfc3339Millis(device.LastSeenAt),
	}
}

func dedupeDevices(devices []LogicalDevice) []LogicalDevice {
	byID := make(map[string]LogicalDevice)
	for _, device := range devices {
		existing, ok := byID[device.DeviceID]
		if !ok || device.LastSeenAt.After(existing.LastSeenAt) {
			byID[device.DeviceID] = device
		}
	}

	result := make([]LogicalDevice, 0, len(byID))
	for _, device := range byID {
		result = append(result, device)
	}
	return result
}
