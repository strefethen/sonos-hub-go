package sonos

import (
	"sync"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// GroupPlaybackInfo holds all playback-related info for a single group.
type GroupPlaybackInfo struct {
	TransportInfo *soap.TransportInfo
	PositionInfo  *soap.PositionInfo
	MediaInfo     *soap.MediaInfo
	VolumeInfo    *soap.VolumeInfo
	MuteInfo      *soap.MuteInfo
	Error         error
}

// CoordinatorInfo represents a group coordinator for parallel fetching.
type CoordinatorInfo struct {
	UUID             string
	ZoneName         string
	IP               string
	HdmiCecAvailable bool
	MemberRooms      []string
}

// GroupPlaybackResult combines coordinator info with fetched playback data.
type GroupPlaybackResult struct {
	Coordinator CoordinatorInfo
	Playback    GroupPlaybackInfo
}

// FetchGroupPlayback fetches all playback info for a single group in parallel.
// It uses smart skipping: if transport state is STOPPED, it skips position info
// since there's no track progress to report. Media info is always fetched
// because it's needed for TV mode detection.
func FetchGroupPlayback(svc *Service, coordinatorIP string) GroupPlaybackInfo {
	result := GroupPlaybackInfo{}

	// First, fetch transport info (needed for smart skipping decision)
	transport, err := svc.GetTransportInfo(coordinatorIP)
	if err != nil {
		result.Error = err
		return result
	}
	result.TransportInfo = &transport

	// Determine if we should skip position info (STOPPED means no track progress)
	// TRANSITIONING should NOT skip - track may be loading
	// Note: We always fetch mediaInfo because it's needed for TV mode detection
	skipPositionInfo := transport.CurrentTransportState == "STOPPED"

	// Now fetch remaining info in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Volume
	wg.Add(1)
	go func() {
		defer wg.Done()
		vol, err := svc.GetVolume(coordinatorIP)
		mu.Lock()
		if err == nil {
			result.VolumeInfo = &vol
		}
		mu.Unlock()
	}()

	// Mute
	wg.Add(1)
	go func() {
		defer wg.Done()
		mute, err := svc.GetMute(coordinatorIP)
		mu.Lock()
		if err == nil {
			result.MuteInfo = &mute
		}
		mu.Unlock()
	}()

	// Position info (skip if STOPPED - no track progress to report)
	if !skipPositionInfo {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pos, err := svc.GetPositionInfo(coordinatorIP)
			mu.Lock()
			if err == nil {
				result.PositionInfo = &pos
			}
			mu.Unlock()
		}()
	}

	// Media info - always fetch (needed for TV mode detection via x-sonos-htastream URI)
	wg.Add(1)
	go func() {
		defer wg.Done()
		media, err := svc.GetMediaInfo(coordinatorIP)
		mu.Lock()
		if err == nil {
			result.MediaInfo = &media
		}
		mu.Unlock()
	}()

	wg.Wait()
	return result
}

// FetchAllGroupsPlayback fetches playback info for all groups in parallel.
// This is the main optimization - instead of sequential fetching per group,
// all groups are fetched concurrently.
func FetchAllGroupsPlayback(svc *Service, coordinators []CoordinatorInfo) []GroupPlaybackResult {
	results := make([]GroupPlaybackResult, len(coordinators))
	var wg sync.WaitGroup

	for i, coord := range coordinators {
		wg.Add(1)
		go func(idx int, coordinator CoordinatorInfo) {
			defer wg.Done()
			playback := FetchGroupPlayback(svc, coordinator.IP)
			results[idx] = GroupPlaybackResult{
				Coordinator: coordinator,
				Playback:    playback,
			}
		}(i, coord)
	}

	wg.Wait()
	return results
}

// ExtractCoordinators extracts coordinator info from zone group state.
// This is a helper function to prepare data for parallel fetching.
func ExtractCoordinators(zoneState *soap.ZoneGroupState, uuidToIP map[string]string) []CoordinatorInfo {
	coordinators := make([]CoordinatorInfo, 0, len(zoneState.Groups))

	for _, group := range zoneState.Groups {
		visibleMembers := make([]soap.ZoneMember, 0)
		for _, member := range group.Members {
			if member.IsVisible {
				visibleMembers = append(visibleMembers, member)
			}
		}
		if len(visibleMembers) == 0 {
			continue
		}

		var coordinator *soap.ZoneMember
		for i := range visibleMembers {
			if visibleMembers[i].IsCoordinator {
				coordinator = &visibleMembers[i]
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

		memberRooms := make([]string, 0)
		for _, member := range visibleMembers {
			if member.IsCoordinator {
				continue
			}
			memberRooms = append(memberRooms, member.ZoneName)
		}

		coordinators = append(coordinators, CoordinatorInfo{
			UUID:             coordinator.UUID,
			ZoneName:         coordinator.ZoneName,
			IP:               coordinatorIP,
			HdmiCecAvailable: coordinator.HdmiCecAvailable,
			MemberRooms:      memberRooms,
		})
	}

	return coordinators
}

// BuildUUIDToIPMap extracts a UUID to IP mapping from zone group state.
func BuildUUIDToIPMap(zoneState *soap.ZoneGroupState) map[string]string {
	uuidToIP := make(map[string]string, len(zoneState.Groups)*4) // Estimate 4 members per group
	for _, group := range zoneState.Groups {
		for _, member := range group.Members {
			match := ipRegex.FindStringSubmatch(member.Location)
			if len(match) > 1 {
				uuidToIP[member.UUID] = match[1]
			}
		}
	}
	return uuidToIP
}
