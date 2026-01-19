package sonos

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// RegisterRoutes wires Sonos routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Route("/v1/sonos/playback", func(playback chi.Router) {
		playback.Method(http.MethodPost, "/stop", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string `json:"device_id"`
			}
			if err := decodeJSON(r, &body); err != nil || body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Stop(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to stop playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"device_id":  body.DeviceID,
				"action":     "stop",
				"stopped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/pause", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string `json:"device_id"`
			}
			if err := decodeJSON(r, &body); err != nil || body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Pause(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to pause playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":    "playback_action",
				"device_id": body.DeviceID,
				"action":    "pause",
				"paused_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/play", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string `json:"device_id"`
			}
			if err := decodeJSON(r, &body); err != nil || body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Play(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to start playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"device_id":  body.DeviceID,
				"action":     "play",
				"resumed_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/next", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string `json:"device_id"`
			}
			if err := decodeJSON(r, &body); err != nil || body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Next(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to skip track")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"device_id":  body.DeviceID,
				"action":     "next",
				"skipped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/previous", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string `json:"device_id"`
			}
			if err := decodeJSON(r, &body); err != nil || body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Previous(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to skip track")
			}
			if err := service.Previous(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to skip track")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"device_id":  body.DeviceID,
				"action":     "previous",
				"skipped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodGet, "/state", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := r.URL.Query().Get("device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			state, err := service.GetTransportInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch transport state")
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":    "playback_state",
				"device_id": deviceID,
				"state":     state.CurrentTransportState,
				"status":    state.CurrentTransportStatus,
				"speed":     state.CurrentSpeed,
			})
		}))

		playback.Method(http.MethodGet, "/now-playing", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := r.URL.Query().Get("device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id query parameter is required", nil)
			}

			entryIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			zoneState, err := service.GetZoneGroupState(entryIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone group state")
			}

			uuidToIP := map[string]string{}
			ipRegex := regexp.MustCompile(`http://([^:]+):`)
			for _, group := range zoneState.Groups {
				for _, member := range group.Members {
					match := ipRegex.FindStringSubmatch(member.Location)
					if len(match) > 1 {
						uuidToIP[member.UUID] = match[1]
					}
				}
			}

			groups := make([]map[string]any, 0)
			for _, group := range zoneState.Groups {
				visibleMembers := make([]soapMember, 0)
				for _, member := range group.Members {
					if member.IsVisible {
						visibleMembers = append(visibleMembers, soapMember{
							UUID:          member.UUID,
							ZoneName:      member.ZoneName,
							IsCoordinator: member.IsCoordinator,
						})
					}
				}
				if len(visibleMembers) == 0 {
					continue
				}

				var coordinator *soapMember
				for _, member := range visibleMembers {
					if member.IsCoordinator {
						coordinator = &member
						break
					}
				}
				if coordinator == nil {
					continue
				}

				coordinatorIP := uuidToIP[coordinator.UUID]
				if coordinatorIP == "" {
					continue
				}

				transportInfo, err := service.GetTransportInfo(coordinatorIP)
				if err != nil {
					continue
				}
				positionInfo, err := service.GetPositionInfo(coordinatorIP)
				if err != nil {
					continue
				}
				mediaInfo, err := service.GetMediaInfo(coordinatorIP)
				if err != nil {
					continue
				}
				volumeInfo, err := service.GetVolume(coordinatorIP)
				if err != nil {
					continue
				}
				muteInfo, err := service.GetMute(coordinatorIP)
				if err != nil {
					continue
				}

				isTV := strings.Contains(mediaInfo.CurrentURI, "x-sonos-htastream")

				var track any = nil
				metadata := ParseDidlMetadata(positionInfo.TrackMetaData, positionInfo.TrackURI)
				if isTV {
					track = map[string]any{
						"title":            "TV",
						"artist":           nil,
						"album":            nil,
						"album_art_uri":    nil,
						"duration_seconds": nil,
						"position_seconds": nil,
						"source":           "tv",
						"service_name":     "TV Audio",
						"service_logo_url": nil,
					}
				} else if metadata != nil && transportInfo.CurrentTransportState != "STOPPED" {
					albumArt := metadata.AlbumArtURI
					if albumArt != "" {
						albumArt = normalizeAlbumArtURI(albumArt, coordinatorIP)
					}

					serviceLogo := ""
					if metadata.ServiceName != "" {
						serviceLogo = GetServiceLogoFromName(metadata.ServiceName)
					}

					var logo any = nil
					if serviceLogo != "" {
						logo = serviceLogo
					}

					var art any = nil
					if albumArt != "" {
						art = albumArt
					}

					track = map[string]any{
						"title":            metadata.Title,
						"artist":           emptyToNil(metadata.Artist),
						"album":            emptyToNil(metadata.Album),
						"album_art_uri":    art,
						"duration_seconds": ParseDuration(positionInfo.TrackDuration),
						"position_seconds": ParseDuration(positionInfo.RelTime),
						"source":           metadata.Source,
						"service_name":     emptyToNil(metadata.ServiceName),
						"service_logo_url": logo,
					}
				}

				container := any(nil)
				containerMeta := ParseContainerMetadata(mediaInfo.CurrentURIMetaData)
				if containerMeta != nil && containerMeta.Name != "" {
					container = map[string]any{
						"name": containerMeta.Name,
						"type": containerMeta.Type,
					}
				}

				displayState := transportInfo.CurrentTransportState

				memberRooms := make([]string, 0)
				for _, member := range visibleMembers {
					if member.IsCoordinator {
						continue
					}
					memberRooms = append(memberRooms, member.ZoneName)
				}

				groups = append(groups, map[string]any{
					"coordinator_id": coordinator.UUID,
					"room_name":      coordinator.ZoneName,
					"member_rooms":   memberRooms,
					"playback": map[string]any{
						"state":     displayState,
						"volume":    volumeInfo.CurrentVolume,
						"muted":     muteInfo.CurrentMute,
						"track":     track,
						"container": container,
						"isTV":      isTV,
					},
				})
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":       "now_playing",
				"groups":       groups,
				"total_groups": len(groups),
			})
		}))
	})

	router.Route("/v1/sonos/groups", func(groups chi.Router) {
		groups.Method(http.MethodGet, "/", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := r.URL.Query().Get("device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			zoneState, err := service.GetZoneGroupState(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone group state")
			}

			groupsResponse := make([]map[string]any, 0, len(zoneState.Groups))
			for _, group := range zoneState.Groups {
				members := make([]map[string]any, 0)
				for _, member := range group.Members {
					if !member.IsVisible {
						continue
					}
					members = append(members, map[string]any{
						"uuid":           member.UUID,
						"zone_name":      member.ZoneName,
						"is_coordinator": member.IsCoordinator,
					})
				}

				groupsResponse = append(groupsResponse, map[string]any{
					"coordinator":  group.Coordinator,
					"members":      members,
					"member_count": len(members),
				})
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":       "groups",
				"items":        groupsResponse,
				"total_groups": len(groupsResponse),
			})
		}))

		groups.Method(http.MethodPost, "/", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				CoordinatorDeviceID string   `json:"coordinator_device_id"`
				MemberDeviceIDs     []string `json:"member_device_ids"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("coordinator_device_id is required", nil)
			}
			if body.CoordinatorDeviceID == "" {
				return apperrors.NewValidationError("coordinator_device_id is required", nil)
			}

			memberIDs := make([]string, 0, len(body.MemberDeviceIDs))
			for _, id := range body.MemberDeviceIDs {
				if id != "" && id != body.CoordinatorDeviceID {
					memberIDs = append(memberIDs, id)
				}
			}

			if len(memberIDs) == 0 {
				return api.WriteAction(w, http.StatusOK, map[string]any{
					"object":                "group_create",
					"coordinator_device_id": body.CoordinatorDeviceID,
					"coordinator_uuid":      nil,
					"coordinator_name":      nil,
					"member_results":        []map[string]any{},
					"all_succeeded":         true,
				})
			}

			coordinatorIP, err := service.ResolveDeviceIP(body.CoordinatorDeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve coordinator device")
			}

			memberIPs := make([]string, 0, len(memberIDs))
			for _, id := range memberIDs {
				ip, err := service.ResolveDeviceIP(id)
				if err != nil {
					memberIPs = append(memberIPs, "")
					continue
				}
				memberIPs = append(memberIPs, ip)
			}

			zoneAttrs, err := service.GetZoneAttributes(coordinatorIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone attributes")
			}

			topology, err := service.GetZoneGroupState(coordinatorIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone group state")
			}

			coordinatorUUID := ""
			for _, group := range topology.Groups {
				for _, member := range group.Members {
					if member.ZoneName == zoneAttrs.CurrentZoneName {
						coordinatorUUID = member.UUID
						break
					}
				}
				if coordinatorUUID != "" {
					break
				}
			}

			if coordinatorUUID == "" {
				return apperrors.NewValidationError("Could not determine coordinator UUID", nil)
			}

			memberResults := make([]map[string]any, 0, len(memberIDs))
			for idx, memberIP := range memberIPs {
				memberID := memberIDs[idx]
				if memberIP == "" {
					memberResults = append(memberResults, map[string]any{
						"device_id": memberID,
						"success":   false,
						"error":     "Unable to resolve device",
					})
					continue
				}

				err := service.SetAVTransportURI(memberIP, "x-rincon:"+coordinatorUUID)
				if err != nil {
					memberResults = append(memberResults, map[string]any{
						"device_id": memberID,
						"success":   false,
						"error":     err.Error(),
					})
					continue
				}
				memberResults = append(memberResults, map[string]any{
					"device_id": memberID,
					"success":   true,
				})
			}

			allSucceeded := true
			for _, result := range memberResults {
				if success, ok := result["success"].(bool); ok && !success {
					allSucceeded = false
					break
				}
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":                "group_create",
				"coordinator_device_id": body.CoordinatorDeviceID,
				"coordinator_uuid":      coordinatorUUID,
				"coordinator_name":      zoneAttrs.CurrentZoneName,
				"member_results":        memberResults,
				"all_succeeded":         allSucceeded,
			})
		}))

		groups.Method(http.MethodPost, "/ungroup", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceIDs []string `json:"device_ids"`
				IPs       []string `json:"ips"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("device_ids or ips array is required and must not be empty", nil)
			}
			if len(body.DeviceIDs) == 0 && len(body.IPs) == 0 {
				return apperrors.NewValidationError("device_ids or ips array is required and must not be empty", nil)
			}

			resolvedIPs := make([]string, 0, len(body.DeviceIDs))
			for _, id := range body.DeviceIDs {
				ip, err := service.ResolveDeviceIP(id)
				if err != nil {
					resolvedIPs = append(resolvedIPs, "")
					continue
				}
				resolvedIPs = append(resolvedIPs, ip)
			}

			ungroupResults := make([]map[string]any, 0)
			for idx, ip := range resolvedIPs {
				deviceID := body.DeviceIDs[idx]
				if ip == "" {
					ungroupResults = append(ungroupResults, map[string]any{
						"device_id": deviceID,
						"success":   false,
						"error":     "Unable to resolve device",
					})
					continue
				}

				err := service.BecomeCoordinatorOfStandaloneGroup(ip)
				if err != nil {
					ungroupResults = append(ungroupResults, map[string]any{
						"device_id": deviceID,
						"success":   false,
						"error":     err.Error(),
					})
					continue
				}
				ungroupResults = append(ungroupResults, map[string]any{
					"device_id": deviceID,
					"success":   true,
				})
			}

			for _, ip := range body.IPs {
				if ip == "" {
					continue
				}
				err := service.BecomeCoordinatorOfStandaloneGroup(ip)
				if err != nil {
					ungroupResults = append(ungroupResults, map[string]any{
						"device_id": ip,
						"success":   false,
						"error":     err.Error(),
					})
					continue
				}
				ungroupResults = append(ungroupResults, map[string]any{
					"device_id": ip,
					"success":   true,
				})
			}

			allSucceeded := true
			for _, result := range ungroupResults {
				if success, ok := result["success"].(bool); ok && !success {
					allSucceeded = false
					break
				}
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":          "ungroup",
				"ungroup_results": ungroupResults,
				"all_succeeded":   allSucceeded,
			})
		}))
	})

	router.Route("/v1/sonos/volume", func(volume chi.Router) {
		volume.Method(http.MethodPost, "/", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string   `json:"device_id"`
				Volume   *float64 `json:"volume"`
				Ramp     *struct {
					Enabled    bool   `json:"enabled"`
					DurationMs *int   `json:"duration_ms"`
					Curve      string `json:"curve"`
				} `json:"ramp"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.Volume == nil || *body.Volume < 0 || *body.Volume > 100 {
				return apperrors.NewValidationError("volume must be a number between 0 and 100", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			currentVolume, err := service.GetVolume(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch volume")
			}

			memberIPs := getGroupMemberIPs(service, deviceIP)
			if len(memberIPs) == 0 {
				return apperrors.NewValidationError("No devices resolved for volume control", nil)
			}

			target := int(math.Round(*body.Volume))
			ramped := false
			var results []deviceVolumeResult
			if body.Ramp != nil && body.Ramp.Enabled && body.Ramp.DurationMs != nil && *body.Ramp.DurationMs > 0 {
				curve := body.Ramp.Curve
				if curve == "" {
					curve = "linear"
				}
				results = executeVolumeRamp(service, memberIPs, currentVolume.CurrentVolume, target, *body.Ramp.DurationMs, curve)
				ramped = true
			} else {
				results = setVolumeOnDevices(service, memberIPs, target)
			}

			succeeded, failed := countResults(results)

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":          "volume_action",
				"device_id":       body.DeviceID,
				"volume":          target,
				"previous_volume": currentVolume.CurrentVolume,
				"ramped":          ramped,
				"all_succeeded":   failed == 0,
				"succeeded_count": succeeded,
				"failed_count":    failed,
			})
		}))

		volume.Method(http.MethodPost, "/set", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID string   `json:"device_id"`
				Level    *float64 `json:"level"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.Level == nil || *body.Level < 0 || *body.Level > 100 {
				return apperrors.NewValidationError("level must be a number between 0 and 100", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			currentVolume, err := service.GetVolume(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch volume")
			}

			memberIPs := getGroupMemberIPs(service, deviceIP)
			if len(memberIPs) == 0 {
				return apperrors.NewValidationError("No devices resolved for volume control", nil)
			}

			target := int(math.Round(*body.Level))
			results := setVolumeOnDevices(service, memberIPs, target)
			succeeded, failed := countResults(results)

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":          "volume_action",
				"device_id":       body.DeviceID,
				"level":           target,
				"previous_level":  currentVolume.CurrentVolume,
				"all_succeeded":   failed == 0,
				"succeeded_count": succeeded,
				"failed_count":    failed,
			})
		}))

		volume.Method(http.MethodPost, "/ramp", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				DeviceID    string   `json:"device_id"`
				TargetLevel *float64 `json:"target_level"`
				DurationMs  *int     `json:"duration_ms"`
				Curve       string   `json:"curve"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.DeviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}
			if body.TargetLevel == nil || *body.TargetLevel < 0 || *body.TargetLevel > 100 {
				return apperrors.NewValidationError("target_level must be a number between 0 and 100", nil)
			}

			durationMs := 2000
			if body.DurationMs != nil {
				durationMs = *body.DurationMs
			}
			if durationMs < 0 {
				return apperrors.NewValidationError("duration_ms must be non-negative", nil)
			}

			curve := body.Curve
			if curve == "" {
				curve = "linear"
			}

			deviceIP, err := service.ResolveDeviceIP(body.DeviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			currentVolume, err := service.GetVolume(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch volume")
			}

			memberIPs := getGroupMemberIPs(service, deviceIP)
			if len(memberIPs) == 0 {
				return apperrors.NewValidationError("No devices resolved for volume control", nil)
			}

			target := int(math.Round(*body.TargetLevel))
			results := executeVolumeRamp(service, memberIPs, currentVolume.CurrentVolume, target, durationMs, curve)
			succeeded, failed := countResults(results)

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":          "volume_ramp",
				"device_id":       body.DeviceID,
				"start_level":     currentVolume.CurrentVolume,
				"target_level":    target,
				"duration_ms":     durationMs,
				"curve":           curve,
				"all_succeeded":   failed == 0,
				"succeeded_count": succeeded,
				"failed_count":    failed,
			})
		}))
	})

	router.Method(http.MethodGet, "/v1/sonos/alarms", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		deviceID := r.URL.Query().Get("device_id")
		if deviceID == "" {
			return apperrors.NewValidationError("device_id query parameter is required", nil)
		}

		deviceIP, err := service.ResolveDeviceIP(deviceID)
		if err != nil {
			return apperrors.NewInternalError("Failed to resolve device")
		}

		result, err := service.ListAlarms(deviceIP)
		if err != nil {
			return apperrors.NewInternalError("Failed to fetch alarms")
		}

		alarms := make([]map[string]any, 0, len(result.Alarms))
		for _, alarm := range result.Alarms {
			alarms = append(alarms, map[string]any{
				"id":                   alarm.ID,
				"start_time":           alarm.StartTime,
				"duration":             alarm.Duration,
				"recurrence":           alarm.Recurrence,
				"enabled":              alarm.Enabled,
				"room_uuid":            alarm.RoomUUID,
				"program_uri":          alarm.ProgramURI,
				"program_metadata":     alarm.ProgramMetaData,
				"play_mode":            alarm.PlayMode,
				"volume":               alarm.Volume,
				"include_linked_zones": alarm.IncludeLinkedZones,
			})
		}

		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object":             "alarms",
			"items":              alarms,
			"alarm_list_version": result.AlarmListVersion,
		})
	}))

	router.Method(http.MethodGet, "/v1/sonos/favorites", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		startStr := r.URL.Query().Get("start")
		countStr := r.URL.Query().Get("count")

		startIndex := 0
		if startStr != "" {
			val, err := strconv.Atoi(startStr)
			if err != nil || val < 0 {
				return apperrors.NewValidationError("start must be a non-negative integer", nil)
			}
			startIndex = val
		}

		requestedCount := 100
		if countStr != "" {
			val, err := strconv.Atoi(countStr)
			if err != nil || val < 1 || val > 1000 {
				return apperrors.NewValidationError("count must be an integer between 1 and 1000", nil)
			}
			requestedCount = val
		}

		result, err := service.BrowseFavorites(startIndex, requestedCount)
		if err != nil {
			return apperrors.NewInternalError("Failed to fetch favorites")
		}

		favorites := make([]map[string]any, 0, len(result.Items))
		for _, fav := range result.Items {
			// Detect content type from upnp:class
			contentType := fav.ContentType
			if contentType == "" {
				contentType = detectContentTypeFromClass(fav.UpnpClass, fav.Resource)
			}

			// Fall back to detecting service name from resource/metadata
			serviceName := fav.ServiceName
			if serviceName == "" {
				serviceName = detectServiceName(fav.Resource, fav.ResourceMetaData)
			}

			// Fall back to deriving logo URL from service name
			serviceLogoURL := fav.ServiceLogoURL
			if serviceLogoURL == "" && serviceName != "" {
				serviceLogoURL = GetServiceLogoFromName(serviceName)
			}

			// Convert ordinal to integer (Node.js returns number)
			ordinal, _ := strconv.Atoi(fav.Ordinal)

			favorites = append(favorites, map[string]any{
				"id":                fav.ID,
				"parent_id":         fav.ParentID,
				"title":             fav.Title,
				"ordinal":           ordinal,
				"upnp_class":        fav.UpnpClass,
				"content_type":      contentType,
				"favorite_type":     fav.FavoriteType,
				"service_name":      serviceName,
				"service_logo_url":  serviceLogoURL,
				"album_art_uri":     fav.AlbumArtURI,
				"resource":          fav.Resource,
				"protocol_info":     fav.ProtocolInfo,
				"resource_metadata": fav.ResourceMetaData,
			})
		}

		// Add object field to each favorite
		for i := range favorites {
			favorites[i]["object"] = "favorite"
		}

		hasMore := startIndex+len(favorites) < result.TotalMatches
		return api.WriteList(w, "/v1/sonos/favorites", favorites, hasMore)
	}))

	router.Route("/v1/sonos/players", func(players chi.Router) {
		players.Method(http.MethodGet, "/", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := r.URL.Query().Get("device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			zoneState, err := service.GetZoneGroupState(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone group state")
			}

			playersResponse := make([]map[string]any, 0)
			groupSummary := make([]map[string]any, 0, len(zoneState.Groups))
			for _, group := range zoneState.Groups {
				for _, member := range group.Members {
					if !member.IsVisible {
						continue
					}
					playersResponse = append(playersResponse, map[string]any{
						"uuid":              member.UUID,
						"zone_name":         member.ZoneName,
						"location":          member.Location,
						"is_coordinator":    member.IsCoordinator,
						"group_coordinator": group.Coordinator,
					})
				}
				groupSummary = append(groupSummary, map[string]any{
					"coordinator":  group.Coordinator,
					"member_count": len(group.Members),
				})
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":  "players",
				"players": playersResponse,
				"groups":  groupSummary,
			})
		}))

		players.Method(http.MethodGet, "/{device_id}/state", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := chi.URLParam(r, "device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			transportInfo, err := service.GetTransportInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch transport info")
			}
			positionInfo, err := service.GetPositionInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch position info")
			}
			volumeInfo, err := service.GetVolume(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch volume")
			}
			muteInfo, err := service.GetMute(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch mute state")
			}

			var currentTrack any = nil
			if positionInfo.TrackURI != "" {
				currentTrack = map[string]any{
					"uri":      positionInfo.TrackURI,
					"duration": positionInfo.TrackDuration,
					"position": positionInfo.RelTime,
					"metadata": emptyToNil(positionInfo.TrackMetaData),
				}
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":           "player_state",
				"device_id":        deviceID,
				"transport_state":  transportInfo.CurrentTransportState,
				"transport_status": transportInfo.CurrentTransportStatus,
				"volume":           volumeInfo.CurrentVolume,
				"muted":            muteInfo.CurrentMute,
				"current_track":    currentTrack,
			})
		}))

		players.Method(http.MethodGet, "/{device_id}/tv-status", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			deviceID := chi.URLParam(r, "device_id")
			if deviceID == "" {
				return apperrors.NewValidationError("device_id is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(deviceID)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			positionInfo, err := service.GetPositionInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch position info")
			}
			transportInfo, err := service.GetTransportInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch transport info")
			}

			currentURI := positionInfo.TrackURI
			isTVInput := false
			for _, pattern := range tvInputPatterns {
				if strings.HasPrefix(currentURI, pattern) {
					isTVInput = true
					break
				}
			}

			isPlaying := transportInfo.CurrentTransportState == "PLAYING"
			isTVActive := isTVInput && isPlaying

			source := "idle"
			switch {
			case strings.HasPrefix(currentURI, "x-sonos-htastream:") || strings.HasPrefix(currentURI, "x-sonos-vli:"):
				source = "tv"
			case strings.HasPrefix(currentURI, "x-rincon-stream:"):
				source = "line-in"
			case currentURI != "":
				source = "music"
			}

			confidence := "HIGH"
			if transportInfo.CurrentTransportState == "TRANSITIONING" {
				confidence = "MEDIUM"
			} else if currentURI == "" {
				confidence = "LOW"
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object":          "tv_status",
				"device_id":       deviceID,
				"is_tv_active":    isTVActive,
				"confidence":      confidence,
				"source":          source,
				"transport_state": transportInfo.CurrentTransportState,
				"current_uri":     emptyToNil(currentURI),
				"last_checked_at": api.RFC3339Millis(time.Now()),
			})
		}))
	})
}

// RegisterPlayRoutes wires play routes to the router.
func RegisterPlayRoutes(router chi.Router, playService *PlayService) {
	// POST /v1/sonos/play - Resume playback
	router.Method(http.MethodPost, "/v1/sonos/play", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		var req PlayRequest
		if err := decodeJSON(r, &req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if req.CoordinatorDeviceID == nil && req.IP == nil {
			return apperrors.NewValidationError("coordinator_device_id or ip is required", nil)
		}

		result, err := playService.Play(r.Context(), req)
		if err != nil {
			return apperrors.NewInternalError("Failed to start playback: " + err.Error())
		}

		return api.WriteAction(w, http.StatusOK, result)
	}))

	// POST /v1/sonos/play/favorite - Play a Sonos favorite
	router.Method(http.MethodPost, "/v1/sonos/play/favorite", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		var req PlayFavoriteRequest
		if err := decodeJSON(r, &req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if req.FavoriteID == "" {
			return apperrors.NewValidationError("favorite_id is required", nil)
		}

		if req.DeviceID == nil && req.IP == nil {
			return apperrors.NewValidationError("device_id or ip is required", nil)
		}

		result, err := playService.PlayFavorite(r.Context(), req)
		if err != nil {
			if _, ok := err.(*FavoriteNotFoundError); ok {
				return apperrors.NewValidationError("favorite not found: "+req.FavoriteID, nil)
			}
			return apperrors.NewInternalError("Failed to play favorite: " + err.Error())
		}

		return api.WriteAction(w, http.StatusOK, result)
	}))

	// POST /v1/sonos/play/content - Play direct content
	router.Method(http.MethodPost, "/v1/sonos/play/content", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		var req PlayContentRequest
		if err := decodeJSON(r, &req); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		if req.DeviceID == nil && req.IP == nil {
			return apperrors.NewValidationError("device_id or ip is required", nil)
		}

		if req.Content.Type == "" {
			return apperrors.NewValidationError("content.type is required", nil)
		}

		result, err := playService.PlayContent(r.Context(), req)
		if err != nil {
			if _, ok := err.(*ServiceNotSupportedError); ok {
				return apperrors.NewValidationError(err.Error(), nil)
			}
			if _, ok := err.(*ServiceNeedsBootstrapError); ok {
				return apperrors.NewValidationError(err.Error(), nil)
			}
			return apperrors.NewInternalError("Failed to play content: " + err.Error())
		}

		return api.WriteAction(w, http.StatusOK, result)
	}))

	// GET /v1/sonos/services - Get all music service statuses
	router.Method(http.MethodGet, "/v1/sonos/services", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		deviceID := r.URL.Query().Get("device_id")
		ip := r.URL.Query().Get("ip")

		if deviceID == "" && ip == "" {
			return apperrors.NewValidationError("device_id or ip query parameter is required", nil)
		}

		deviceIP := ip
		if deviceIP == "" && deviceID != "" {
			// We need to resolve the device ID to IP
			// For now, just pass empty and let the service handle it
			deviceIP = deviceID // The service will resolve this
		}

		services, err := playService.GetServices(r.Context(), deviceIP)
		if err != nil {
			return apperrors.NewInternalError("Failed to get services: " + err.Error())
		}

		return api.WriteResource(w, http.StatusOK, map[string]any{
			"object": "services",
			"items":  services,
			"count":  len(services),
		})
	}))

	// GET /v1/sonos/services/{service}/health - Get health status for a specific service
	router.Method(http.MethodGet, "/v1/sonos/services/{service}/health", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
		service := chi.URLParam(r, "service")
		if service == "" {
			return apperrors.NewValidationError("service name is required", nil)
		}

		deviceID := r.URL.Query().Get("device_id")
		ip := r.URL.Query().Get("ip")

		if deviceID == "" && ip == "" {
			return apperrors.NewValidationError("device_id or ip query parameter is required", nil)
		}

		deviceIP := ip
		if deviceIP == "" && deviceID != "" {
			deviceIP = deviceID
		}

		status, err := playService.GetServiceHealth(r.Context(), service, deviceIP)
		if err != nil {
			return apperrors.NewInternalError("Failed to get service health: " + err.Error())
		}

		return api.WriteResource(w, http.StatusOK, status)
	}))
}

type soapMember struct {
	UUID          string
	ZoneName      string
	IsCoordinator bool
}

type deviceVolumeResult struct {
	IP      string
	Success bool
	Error   string
}

func normalizeAlbumArtURI(uri string, deviceIP string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		return uri
	}
	if strings.HasPrefix(uri, "/") {
		return "http://" + deviceIP + ":1400" + uri
	}
	return uri
}

// detectContentTypeFromClass detects content type from upnp:class and resource URI.
func detectContentTypeFromClass(upnpClass, resource string) string {
	class := strings.ToLower(upnpClass)
	res := strings.ToLower(resource)

	// Check for radio/streaming first
	if strings.Contains(class, "audiobroadcast") || strings.Contains(class, "radio") ||
		strings.Contains(res, "x-sonosapi-stream") || strings.Contains(res, "x-sonosapi-radio") {
		return "station"
	}

	// Check for playlist
	if strings.Contains(class, "playlistcontainer") || strings.Contains(class, "playlist") ||
		strings.HasPrefix(res, "x-rincon-cpcontainer:") {
		return "playlist"
	}

	// Check for album
	if strings.Contains(class, "album") || strings.Contains(class, "musicalbum") {
		return "album"
	}

	// Default to track
	return "track"
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
}

var tvInputPatterns = []string{
	"x-sonos-htastream:",
	"x-rincon-stream:",
	"x-sonos-vli:",
}

func getGroupMemberIPs(service *Service, targetDeviceIP string) []string {
	zoneState, err := service.GetZoneGroupState(targetDeviceIP)
	if err != nil {
		return []string{targetDeviceIP}
	}

	uuidToIP := map[string]string{}
	ipRegex := regexp.MustCompile(`http://([^:]+):`)
	for _, group := range zoneState.Groups {
		for _, member := range group.Members {
			match := ipRegex.FindStringSubmatch(member.Location)
			if len(match) > 1 {
				uuidToIP[member.UUID] = match[1]
			}
		}
	}

	for _, group := range zoneState.Groups {
		memberIPs := make([]string, 0)
		for _, member := range group.Members {
			if !member.IsVisible {
				continue
			}
			if ip, ok := uuidToIP[member.UUID]; ok && ip != "" {
				memberIPs = append(memberIPs, ip)
			}
		}
		for _, ip := range memberIPs {
			if ip == targetDeviceIP {
				return memberIPs
			}
		}
	}

	return []string{targetDeviceIP}
}

func setVolumeOnDevices(service *Service, memberIPs []string, level int) []deviceVolumeResult {
	results := make([]deviceVolumeResult, len(memberIPs))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, ip := range memberIPs {
		wg.Add(1)
		go func(idx int, targetIP string) {
			defer wg.Done()
			err := service.SetVolume(targetIP, level)
			result := deviceVolumeResult{IP: targetIP, Success: err == nil}
			if err != nil {
				result.Error = err.Error()
			}
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, ip)
	}

	wg.Wait()
	return results
}

func executeVolumeRamp(service *Service, memberIPs []string, startLevel, targetLevel, durationMs int, curve string) []deviceVolumeResult {
	if durationMs <= 0 {
		return setVolumeOnDevices(service, memberIPs, targetLevel)
	}

	if startLevel == targetLevel {
		results := make([]deviceVolumeResult, 0, len(memberIPs))
		for _, ip := range memberIPs {
			results = append(results, deviceVolumeResult{IP: ip, Success: true})
		}
		return results
	}

	stepCount := int(math.Max(1, float64(durationMs/50)))
	levelDiff := float64(targetLevel - startLevel)
	stepDelay := time.Duration(float64(durationMs)/float64(stepCount)) * time.Millisecond

	failedDevices := map[string]struct{}{}
	var lastResults []deviceVolumeResult

	for step := 1; step <= stepCount; step++ {
		progress := float64(step) / float64(stepCount)
		switch curve {
		case "ease-in":
			progress = progress * progress
		case "ease-out":
			progress = 1 - math.Pow(1-progress, 2)
		}

		newLevel := int(math.Round(float64(startLevel) + levelDiff*progress))
		activeIPs := make([]string, 0, len(memberIPs))
		for _, ip := range memberIPs {
			if _, ok := failedDevices[ip]; !ok {
				activeIPs = append(activeIPs, ip)
			}
		}
		if len(activeIPs) == 0 {
			break
		}

		stepResults := setVolumeOnDevices(service, activeIPs, newLevel)
		for _, result := range stepResults {
			if !result.Success {
				failedDevices[result.IP] = struct{}{}
			}
		}
		lastResults = stepResults

		if step < stepCount {
			time.Sleep(stepDelay)
		}
	}

	finalIPs := make([]string, 0, len(memberIPs))
	for _, ip := range memberIPs {
		if _, ok := failedDevices[ip]; !ok {
			finalIPs = append(finalIPs, ip)
		}
	}
	if len(finalIPs) > 0 {
		lastResults = setVolumeOnDevices(service, finalIPs, targetLevel)
	}

	results := make([]deviceVolumeResult, 0, len(memberIPs))
	for _, ip := range memberIPs {
		if _, ok := failedDevices[ip]; ok {
			results = append(results, deviceVolumeResult{IP: ip, Success: false, Error: "Device failed during ramp"})
			continue
		}
		found := false
		for _, res := range lastResults {
			if res.IP == ip {
				results = append(results, res)
				found = true
				break
			}
		}
		if !found {
			results = append(results, deviceVolumeResult{IP: ip, Success: false, Error: "Result not found"})
		}
	}

	return results
}

func countResults(results []deviceVolumeResult) (int, int) {
	succeeded := 0
	failed := 0
	for _, res := range results {
		if res.Success {
			succeeded++
		} else {
			failed++
		}
	}
	return succeeded, failed
}
