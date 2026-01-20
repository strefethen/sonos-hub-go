package devices

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

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

		// Stripe-style list response
		return api.WriteList(w, "/v1/devices", formatted, false)
	}))

	router.Method(http.MethodGet, "/v1/devices/{udn}", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		udn := chi.URLParam(r, "udn")

		device, err := service.GetDevice(udn)
		if err != nil {
			return apperrors.NewInternalError("Failed to load device")
		}
		if device == nil {
			return apperrors.NewNotFoundResource("Device", udn)
		}

		// Stripe-style: return resource directly
		return api.WriteResource(w, http.StatusOK, formatDevice(*device))
	}))

	router.Method(http.MethodPost, "/v1/devices/rescan", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		count, durationMs, err := service.Rescan()
		if err != nil {
			return apperrors.NewInternalError("Device rescan failed")
		}

		// Stripe-style: return action result directly with object type
		return api.WriteAction(w, http.StatusOK, map[string]any{
			"object":        "rescan",
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

		// Stripe-style: return resource directly with object type
		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":              "topology",
			"devices":             devices,
			"home_theater_groups": homeTheaterGroups,
			"stereo_pairs":        stereoPairs,
			"updated_at":          api.RFC3339Millis(topology.UpdatedAt),
		})
	}))

	router.Method(http.MethodGet, "/v1/devices/stats", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		topology, err := service.GetTopology()
		if err != nil {
			// Stripe-style: return resource directly with object type
			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":         "device_stats",
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

		// Stripe-style: return resource directly with object type
		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":         "device_stats",
			"total":          len(topology.Devices),
			"online":         online,
			"offline":        offline,
			"last_discovery": api.RFC3339Millis(topology.UpdatedAt),
		})
	}))
}

func formatDevice(device LogicalDevice) map[string]any {
	logicalGroup := any(nil)
	if device.LogicalGroupID != "" {
		logicalGroup = device.LogicalGroupID
	}

	physicalCount := len(device.PhysicalDevices)
	if physicalCount == 0 {
		physicalCount = 1
	}

	return map[string]any{
		"object":                 api.ObjectDevice,
		"udn":                    device.UDN, // Primary identifier
		"room_name":              device.RoomName,
		"ip":                     device.IP,
		"model":                  device.Model,
		"role":                   device.Role,
		"is_targetable":          device.IsTargetable,
		"is_coordinator_capable": device.IsCoordinatorCapable,
		"supports_airplay":       device.SupportsAirPlay,
		"logical_group_id":       logicalGroup,
		"last_seen_at":           api.RFC3339Millis(device.LastSeenAt),
		"physical_device_count":  physicalCount,
		"health":                 device.Health,
		"missed_scans":           device.MissedScans,
	}
}

func formatPhysicalDevice(device PhysicalDevice) map[string]any {
	return map[string]any{
		"object":                 api.ObjectPhysicalDevice,
		"udn":                    device.UDN, // Primary identifier
		"model":                  device.Model,
		"model_number":           device.ModelNumber,
		"room_name":              device.RoomName,
		"role":                   device.Role,
		"is_coordinator_capable": device.IsCoordinatorCapable,
		"supports_airplay":       device.SupportsAirPlay,
		"last_seen_at":           api.RFC3339Millis(device.LastSeenAt),
		"health":                 device.Health,
		"missed_scans":           device.MissedScans,
	}
}

func dedupeDevices(devices []LogicalDevice) []LogicalDevice {
	byUDN := make(map[string]LogicalDevice)
	for _, device := range devices {
		existing, ok := byUDN[device.UDN]
		if !ok || device.LastSeenAt.After(existing.LastSeenAt) {
			byUDN[device.UDN] = device
		}
	}

	result := make([]LogicalDevice, 0, len(byUDN))
	for _, device := range byUDN {
		result = append(result, device)
	}
	return result
}
