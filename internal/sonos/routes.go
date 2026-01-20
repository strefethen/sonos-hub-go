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
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// RegisterRoutes wires Sonos routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Route("/v1/sonos/playback", func(playback chi.Router) {
		playback.Method(http.MethodPost, "/stop", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN string `json:"udn"`
			}
			if err := decodeJSON(r, &body); err != nil || body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Stop(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to stop playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"udn":        body.UDN,
				"action":     "stop",
				"stopped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/pause", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN string `json:"udn"`
			}
			if err := decodeJSON(r, &body); err != nil || body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Pause(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to pause playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":    "playback_action",
				"udn":       body.UDN,
				"action":    "pause",
				"paused_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/play", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN string `json:"udn"`
			}
			if err := decodeJSON(r, &body); err != nil || body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Play(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to start playback")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"udn":        body.UDN,
				"action":     "play",
				"resumed_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/next", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN string `json:"udn"`
			}
			if err := decodeJSON(r, &body); err != nil || body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			if err := service.Next(deviceIP); err != nil {
				return apperrors.NewInternalError("Failed to skip track")
			}

			return api.WriteAction(w, http.StatusOK, map[string]any{
				"object":     "playback_action",
				"udn":        body.UDN,
				"action":     "next",
				"skipped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodPost, "/previous", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN string `json:"udn"`
			}
			if err := decodeJSON(r, &body); err != nil || body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
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
				"udn":        body.UDN,
				"action":     "previous",
				"skipped_at": api.RFC3339Millis(time.Now()),
			})
		}))

		playback.Method(http.MethodGet, "/state", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			udn := r.URL.Query().Get("udn")
			if udn == "" {
				return apperrors.NewValidationError("udn query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(udn)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}
			state, err := service.GetTransportInfo(deviceIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch transport state")
			}

			return api.WriteResource(w, http.StatusOK, map[string]any{
				"object": "playback_state",
				"udn":    udn,
				"state":  state.CurrentTransportState,
				"status": state.CurrentTransportStatus,
				"speed":  state.CurrentSpeed,
			})
		}))

		playback.Method(http.MethodGet, "/now-playing", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			udn := r.URL.Query().Get("udn")
			if udn == "" {
				return apperrors.NewValidationError("udn query parameter is required", nil)
			}

			// Check for debug flag to include data sources
			includeDebug := r.URL.Query().Get("debug") == "true"

			entryIP, err := service.ResolveDeviceIP(udn)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve device")
			}

			// Use cached zone group state (30s TTL by default)
			zoneState, err := service.GetZoneGroupStateCached(entryIP)
			if err != nil {
				return apperrors.NewInternalError("Failed to fetch zone group state")
			}

			// Build UUID to IP mapping
			uuidToIP := BuildUUIDToIPMap(zoneState)

			// Extract coordinator info for parallel fetching
			coordinators := ExtractCoordinators(zoneState, uuidToIP)

			// Fetch all groups using hybrid approach (cache-first with SOAP fallback)
			results, dataSources := FetchAllGroupsPlaybackHybrid(service, coordinators)

			// Build response from hybrid results
			groups := make([]map[string]any, 0, len(results))
			for _, result := range results {
				// Convert HybridGroupResult to GroupPlaybackResult for buildNowPlayingGroup
				groupResult := GroupPlaybackResult{
					Coordinator: result.Coordinator,
					Playback:    result.Playback.GroupPlaybackInfo,
				}
				groupData := buildNowPlayingGroup(groupResult)
				if groupData != nil {
					// Add data source to group if debugging
					if includeDebug {
						groupData["_data_source"] = string(result.Playback.Source)
						if result.Playback.Source == DataSourceCache {
							groupData["_cache_age_ms"] = result.Playback.CacheAge.Milliseconds()
						}
					}
					groups = append(groups, groupData)
				}
			}

			response := map[string]any{
				"object":       "now_playing",
				"groups":       groups,
				"total_groups": len(groups),
			}

			// Include data source stats if debugging
			if includeDebug {
				stats := GetDataSourceStats(dataSources)
				response["_data_sources"] = map[string]any{
					"cache_hits":   stats.CacheHits,
					"soap_fetches": stats.SOAPFetches,
				}
			}

			return api.WriteResource(w, http.StatusOK, response)
		}))
	})

	router.Route("/v1/sonos/groups", func(groups chi.Router) {
		groups.Method(http.MethodGet, "/", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			udn := r.URL.Query().Get("udn")
			if udn == "" {
				return apperrors.NewValidationError("udn query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(udn)
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
				CoordinatorUDN string   `json:"coordinator_udn"`
				MemberUDNs     []string `json:"member_udns"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("coordinator_udn is required", nil)
			}
			if body.CoordinatorUDN == "" {
				return apperrors.NewValidationError("coordinator_udn is required", nil)
			}

			memberUDNs := make([]string, 0, len(body.MemberUDNs))
			for _, udn := range body.MemberUDNs {
				if udn != "" && udn != body.CoordinatorUDN {
					memberUDNs = append(memberUDNs, udn)
				}
			}

			if len(memberUDNs) == 0 {
				return api.WriteAction(w, http.StatusOK, map[string]any{
					"object":           "group_create",
					"coordinator_udn":  body.CoordinatorUDN,
					"coordinator_uuid": nil,
					"coordinator_name": nil,
					"member_results":   []map[string]any{},
					"all_succeeded":    true,
				})
			}

			coordinatorIP, err := service.ResolveDeviceIP(body.CoordinatorUDN)
			if err != nil {
				return apperrors.NewInternalError("Failed to resolve coordinator device")
			}

			memberIPs := make([]string, 0, len(memberUDNs))
			for _, memberUDN := range memberUDNs {
				ip, err := service.ResolveDeviceIP(memberUDN)
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

			memberResults := make([]map[string]any, 0, len(memberUDNs))
			for idx, memberIP := range memberIPs {
				memberUDN := memberUDNs[idx]
				if memberIP == "" {
					memberResults = append(memberResults, map[string]any{
						"udn":     memberUDN,
						"success": false,
						"error":   "Unable to resolve device",
					})
					continue
				}

				err := service.SetAVTransportURI(memberIP, "x-rincon:"+coordinatorUUID)
				if err != nil {
					memberResults = append(memberResults, map[string]any{
						"udn":     memberUDN,
						"success": false,
						"error":   err.Error(),
					})
					continue
				}
				memberResults = append(memberResults, map[string]any{
					"udn":     memberUDN,
					"success": true,
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
				"object":           "group_create",
				"coordinator_udn":  body.CoordinatorUDN,
				"coordinator_uuid": coordinatorUUID,
				"coordinator_name": zoneAttrs.CurrentZoneName,
				"member_results":   memberResults,
				"all_succeeded":    allSucceeded,
			})
		}))

		groups.Method(http.MethodPost, "/ungroup", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDNs []string `json:"udns"`
				IPs  []string `json:"ips"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("udns or ips array is required and must not be empty", nil)
			}
			if len(body.UDNs) == 0 && len(body.IPs) == 0 {
				return apperrors.NewValidationError("udns or ips array is required and must not be empty", nil)
			}

			resolvedIPs := make([]string, 0, len(body.UDNs))
			for _, udn := range body.UDNs {
				ip, err := service.ResolveDeviceIP(udn)
				if err != nil {
					resolvedIPs = append(resolvedIPs, "")
					continue
				}
				resolvedIPs = append(resolvedIPs, ip)
			}

			ungroupResults := make([]map[string]any, 0)
			for idx, ip := range resolvedIPs {
				udn := body.UDNs[idx]
				if ip == "" {
					ungroupResults = append(ungroupResults, map[string]any{
						"udn":     udn,
						"success": false,
						"error":   "Unable to resolve device",
					})
					continue
				}

				err := service.BecomeCoordinatorOfStandaloneGroup(ip)
				if err != nil {
					ungroupResults = append(ungroupResults, map[string]any{
						"udn":     udn,
						"success": false,
						"error":   err.Error(),
					})
					continue
				}
				ungroupResults = append(ungroupResults, map[string]any{
					"udn":     udn,
					"success": true,
				})
			}

			for _, ip := range body.IPs {
				if ip == "" {
					continue
				}
				err := service.BecomeCoordinatorOfStandaloneGroup(ip)
				if err != nil {
					ungroupResults = append(ungroupResults, map[string]any{
						"udn":     ip,
						"success": false,
						"error":   err.Error(),
					})
					continue
				}
				ungroupResults = append(ungroupResults, map[string]any{
					"udn":     ip,
					"success": true,
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
				UDN    string   `json:"udn"`
				Volume *float64 `json:"volume"`
				Ramp   *struct {
					Enabled    bool   `json:"enabled"`
					DurationMs *int   `json:"duration_ms"`
					Curve      string `json:"curve"`
				} `json:"ramp"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("udn is required", nil)
			}
			if body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}
			if body.Volume == nil || *body.Volume < 0 || *body.Volume > 100 {
				return apperrors.NewValidationError("volume must be a number between 0 and 100", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
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
				"udn":             body.UDN,
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
				UDN   string   `json:"udn"`
				Level *float64 `json:"level"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("udn is required", nil)
			}
			if body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}
			if body.Level == nil || *body.Level < 0 || *body.Level > 100 {
				return apperrors.NewValidationError("level must be a number between 0 and 100", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
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
				"udn":             body.UDN,
				"level":           target,
				"previous_level":  currentVolume.CurrentVolume,
				"all_succeeded":   failed == 0,
				"succeeded_count": succeeded,
				"failed_count":    failed,
			})
		}))

		volume.Method(http.MethodPost, "/ramp", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			var body struct {
				UDN         string   `json:"udn"`
				TargetLevel *float64 `json:"target_level"`
				DurationMs  *int     `json:"duration_ms"`
				Curve       string   `json:"curve"`
			}
			if err := decodeJSON(r, &body); err != nil {
				return apperrors.NewValidationError("udn is required", nil)
			}
			if body.UDN == "" {
				return apperrors.NewValidationError("udn is required", nil)
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

			deviceIP, err := service.ResolveDeviceIP(body.UDN)
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
				"udn":             body.UDN,
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
		udn := r.URL.Query().Get("udn")
		if udn == "" {
			return apperrors.NewValidationError("udn query parameter is required", nil)
		}

		deviceIP, err := service.ResolveDeviceIP(udn)
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
			udn := r.URL.Query().Get("udn")
			if udn == "" {
				return apperrors.NewValidationError("udn query parameter is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(udn)
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

		players.Method(http.MethodGet, "/{udn}/state", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			udn := chi.URLParam(r, "udn")
			if udn == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(udn)
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
				"udn":              udn,
				"transport_state":  transportInfo.CurrentTransportState,
				"transport_status": transportInfo.CurrentTransportStatus,
				"volume":           volumeInfo.CurrentVolume,
				"muted":            muteInfo.CurrentMute,
				"current_track":    currentTrack,
			})
		}))

		players.Method(http.MethodGet, "/{udn}/tv-status", api.Handler(func(w http.ResponseWriter, r *http.Request) error {
			udn := chi.URLParam(r, "udn")
			if udn == "" {
				return apperrors.NewValidationError("udn is required", nil)
			}

			deviceIP, err := service.ResolveDeviceIP(udn)
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
				"udn":             udn,
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

		if req.CoordinatorUDN == nil && req.IP == nil {
			return apperrors.NewValidationError("coordinator_udn or ip is required", nil)
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

		if req.UDN == nil && req.IP == nil {
			return apperrors.NewValidationError("udn or ip is required", nil)
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

		if req.UDN == nil && req.IP == nil {
			return apperrors.NewValidationError("udn or ip is required", nil)
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
		udn := r.URL.Query().Get("udn")
		ip := r.URL.Query().Get("ip")

		if udn == "" && ip == "" {
			return apperrors.NewValidationError("udn or ip query parameter is required", nil)
		}

		// Resolve UDN to IP if needed - GetServices requires an IP address
		deviceIP := ip
		if deviceIP == "" && udn != "" {
			resolvedIP, _, err := playService.resolveDeviceIP(&udn, nil)
			if err != nil {
				return apperrors.NewNotFoundError("Device not found", nil)
			}
			deviceIP = resolvedIP
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

		udn := r.URL.Query().Get("udn")
		ip := r.URL.Query().Get("ip")

		if udn == "" && ip == "" {
			return apperrors.NewValidationError("udn or ip query parameter is required", nil)
		}

		// Resolve UDN to IP if needed - GetServiceHealth requires an IP address
		deviceIP := ip
		if deviceIP == "" && udn != "" {
			resolvedIP, _, err := playService.resolveDeviceIP(&udn, nil)
			if err != nil {
				return apperrors.NewNotFoundError("Device not found", nil)
			}
			deviceIP = resolvedIP
		}

		status, err := playService.GetServiceHealth(r.Context(), service, deviceIP)
		if err != nil {
			return apperrors.NewInternalError("Failed to get service health: " + err.Error())
		}

		return api.WriteResource(w, http.StatusOK, status)
	}))
}

type soapMember struct {
	UUID             string
	ZoneName         string
	IsCoordinator    bool
	HdmiCecAvailable bool
}

// buildNowPlayingGroup builds the response map for a single group from parallel fetch results.
func buildNowPlayingGroup(result GroupPlaybackResult) map[string]any {
	coord := result.Coordinator
	pb := result.Playback

	// If we had an error or missing transport info, skip this group
	if pb.Error != nil || pb.TransportInfo == nil {
		return nil
	}

	// For volume/mute, we need valid data
	if pb.VolumeInfo == nil || pb.MuteInfo == nil {
		return nil
	}

	transportInfo := pb.TransportInfo
	volumeInfo := pb.VolumeInfo
	muteInfo := pb.MuteInfo

	// Position and media info may be nil if transport state was STOPPED (smart skipping)
	var positionInfo *soap.PositionInfo
	var mediaInfo *soap.MediaInfo
	if pb.PositionInfo != nil {
		positionInfo = pb.PositionInfo
	}
	if pb.MediaInfo != nil {
		mediaInfo = pb.MediaInfo
	}

	// Determine if TV mode
	isTV := false
	currentURI := ""
	if mediaInfo != nil {
		currentURI = mediaInfo.CurrentURI
		isTV = strings.Contains(currentURI, "x-sonos-htastream")
	}

	// Build track info
	var track any = nil
	if isTV {
		inputType := "Optical"
		if coord.HdmiCecAvailable {
			inputType = "HDMI"
		}
		track = map[string]any{
			"title":            "TV",
			"artist":           nil,
			"album":            nil,
			"album_art_uri":    nil,
			"duration_seconds": nil,
			"position_seconds": nil,
			"source":           "tv",
			"service_name":     inputType,
			"service_logo_url": nil,
		}
	} else if positionInfo != nil && transportInfo.CurrentTransportState != "STOPPED" {
		metadata := ParseDidlMetadata(positionInfo.TrackMetaData, positionInfo.TrackURI)
		if metadata != nil {
			albumArt := metadata.AlbumArtURI
			if albumArt != "" {
				albumArt = normalizeAlbumArtURI(albumArt, coord.IP)
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
	}

	// Build container info
	container := any(nil)
	if !isTV && mediaInfo != nil {
		containerMeta := ParseContainerMetadata(mediaInfo.CurrentURIMetaData)
		if containerMeta != nil && containerMeta.Name != "" && !strings.HasPrefix(containerMeta.Name, "RINCON_") {
			container = map[string]any{
				"name": containerMeta.Name,
				"type": containerMeta.Type,
			}
		}
	}

	// Determine display state
	// Keep transport state as-is (PLAYING, PAUSED_PLAYBACK, etc.)
	// The isTV field already indicates TV mode, so clients can use both fields
	// to determine appropriate display (e.g., show playing animation for TV when state=PLAYING)
	displayState := transportInfo.CurrentTransportState
	if displayState == "" {
		// Empty string is not a valid iOS PlaybackState enum value
		// Default to STOPPED when transport state is unavailable
		displayState = "STOPPED"
	}

	return map[string]any{
		"coordinator_id": coord.UUID,
		"room_name":      coord.ZoneName,
		"member_rooms":   coord.MemberRooms,
		"playback": map[string]any{
			"state":     displayState,
			"volume":    volumeInfo.CurrentVolume,
			"muted":     muteInfo.CurrentMute,
			"track":     track,
			"container": container,
			"isTV":      isTV,
		},
	}
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

// ipRegex extracts IP address from Sonos location URLs (e.g., "http://192.168.1.10:1400/xml/device_description.xml")
var ipRegex = regexp.MustCompile(`http://([^:]+):`)

func getGroupMemberIPs(service *Service, targetDeviceIP string) []string {
	zoneState, err := service.GetZoneGroupState(targetDeviceIP)
	if err != nil {
		return []string{targetDeviceIP}
	}

	uuidToIP := map[string]string{}
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
