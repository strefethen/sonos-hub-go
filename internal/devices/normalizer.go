package devices

import (
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// sonosNamespace is used for generating stable group IDs (home theater, stereo pairs)
// These are internal identifiers for grouping logic, not exposed as device identifiers.
const sonosNamespace = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

var (
	leftSuffixRegex   = regexp.MustCompile(`^(.+?)\s*\(L\)\s*$`)
	rightSuffixRegex  = regexp.MustCompile(`^(.+?)\s*\(R\)\s*$`)
	stereoSuffixRegex = regexp.MustCompile(`\s*\((L|R)\)\s*$`)
	roomSuffixRegex   = regexp.MustCompile(`\s*\((L|R|LF,RF|LR|RR|SW)\)\s*$`)
	udnSuffixRegex    = regexp.MustCompile(`_(MR|LR|RR|SW|LF|RF)$`)
)

// normalizeUDN strips the _MR, _LR, etc. suffix from UDNs for topology matching.
// Device description XML returns UDNs like RINCON_xxx_MR but zone topology uses RINCON_xxx.
func normalizeUDN(udn string) string {
	return udnSuffixRegex.ReplaceAllString(udn, "")
}

// NormalizeDevices transforms raw devices into logical devices.
func NormalizeDevices(raw []RawSonosDevice, topology *ZoneGroupTopology) DeviceTopology {
	physicalDevices := make([]PhysicalDevice, 0, len(raw))
	devicesByUDN := make(map[string]PhysicalDevice)

	for _, device := range raw {
		physical := createPhysicalDevice(device)
		physicalDevices = append(physicalDevices, physical)
		// Use normalized UDN as key for topology matching (strips _MR suffix if present)
		normalizedUDN := normalizeUDN(physical.UDN)
		devicesByUDN[normalizedUDN] = physical
	}

	homeTheaterGroups := make([]HomeTheaterGroup, 0)
	stereoPairs := make([]StereoPair, 0)
	processedUDNs := make(map[string]struct{})

	if topology != nil {
		for _, group := range topology.Groups {
			if ht := identifyHomeTheaterGroup(group.Members, devicesByUDN); ht != nil {
				homeTheaterGroups = append(homeTheaterGroups, *ht)
				processedUDNs[ht.Master.UDN] = struct{}{}
				for _, surround := range ht.Surrounds {
					processedUDNs[surround.UDN] = struct{}{}
				}
				if ht.Sub != nil {
					processedUDNs[ht.Sub.UDN] = struct{}{}
				}
				continue
			}

			if pair := identifyStereoPair(group.Members, group.CoordinatorUDN, devicesByUDN); pair != nil {
				stereoPairs = append(stereoPairs, *pair)
				processedUDNs[pair.Left.UDN] = struct{}{}
				processedUDNs[pair.Right.UDN] = struct{}{}
			}
		}
	}

	fallbackPairs := identifyStereoPairsByRoomName(physicalDevices, processedUDNs)
	for _, pair := range fallbackPairs {
		stereoPairs = append(stereoPairs, pair)
		processedUDNs[pair.Left.UDN] = struct{}{}
		processedUDNs[pair.Right.UDN] = struct{}{}
	}

	logicalDevices := make([]LogicalDevice, 0)

	for _, group := range homeTheaterGroups {
		master := group.Master
		master.Role = DeviceRoleHomeTheaterMaster

		surrounds := make([]PhysicalDevice, 0, len(group.Surrounds))
		for _, surround := range group.Surrounds {
			surround.Role = DeviceRoleSurround
			surrounds = append(surrounds, surround)
		}

		var sub *PhysicalDevice
		if group.Sub != nil {
			updated := *group.Sub
			updated.Role = DeviceRoleSub
			sub = &updated
		}

		physical := []PhysicalDevice{master}
		physical = append(physical, surrounds...)
		if sub != nil {
			physical = append(physical, *sub)
		}

		logicalDevices = append(logicalDevices, LogicalDevice{
			UDN:                  master.UDN,
			RoomName:             cleanRoomName(master.RoomName),
			IP:                   master.IP,
			Model:                master.Model,
			Role:                 DeviceRoleHomeTheaterMaster,
			IsTargetable:         true,
			IsCoordinatorCapable: master.IsCoordinatorCapable,
			SupportsAirPlay:      master.SupportsAirPlay,
			LogicalGroupID:       group.GroupID,
			PhysicalDevices:      physical,
			LastSeenAt:           master.LastSeenAt,
		})
	}

	for _, pair := range stereoPairs {
		lastSeen := pair.Left.LastSeenAt
		if pair.Right.LastSeenAt.After(lastSeen) {
			lastSeen = pair.Right.LastSeenAt
		}
		logicalDevices = append(logicalDevices, LogicalDevice{
			UDN:                  pair.Coordinator.UDN,
			RoomName:             pair.RoomName,
			IP:                   pair.Coordinator.IP,
			Model:                pair.Left.Model + " (Stereo Pair)",
			Role:                 DeviceRoleNormal,
			IsTargetable:         true,
			IsCoordinatorCapable: pair.Left.IsCoordinatorCapable || pair.Right.IsCoordinatorCapable,
			SupportsAirPlay:      pair.Left.SupportsAirPlay || pair.Right.SupportsAirPlay,
			LogicalGroupID:       pair.PairID,
			PhysicalDevices:      []PhysicalDevice{pair.Left, pair.Right},
			LastSeenAt:           lastSeen,
		})
	}

	for _, device := range physicalDevices {
		if _, exists := processedUDNs[device.UDN]; exists {
			continue
		}

		isStereoMember := stereoSuffixRegex.MatchString(device.RoomName)
		isTargetable := !isStereoMember && device.Role == DeviceRoleNormal && device.IsCoordinatorCapable

		logicalDevices = append(logicalDevices, LogicalDevice{
			UDN:                  device.UDN,
			RoomName:             device.RoomName,
			IP:                   device.IP,
			Model:                device.Model,
			Role:                 device.Role,
			IsTargetable:         isTargetable,
			IsCoordinatorCapable: device.IsCoordinatorCapable,
			SupportsAirPlay:      device.SupportsAirPlay,
			LogicalGroupID:       "",
			PhysicalDevices:      []PhysicalDevice{device},
			LastSeenAt:           device.LastSeenAt,
		})
	}

	return DeviceTopology{
		Devices:           logicalDevices,
		HomeTheaterGroups: homeTheaterGroups,
		StereoPairs:       stereoPairs,
		UpdatedAt:         time.Now(),
	}
}

// GetTargetableDevices filters topology to targetable devices.
func GetTargetableDevices(topology DeviceTopology) []LogicalDevice {
	devices := make([]LogicalDevice, 0, len(topology.Devices))
	for _, device := range topology.Devices {
		if device.IsTargetable {
			devices = append(devices, device)
		}
	}
	return devices
}

func cleanRoomName(roomName string) string {
	return strings.TrimSpace(roomSuffixRegex.ReplaceAllString(roomName, ""))
}

func createPhysicalDevice(raw RawSonosDevice) PhysicalDevice {
	modelInfo, ok := SONOS_MODELS[raw.ModelNumber]
	if !ok {
		modelInfo = struct {
			IsCoordinatorCapable bool
			SupportsAirPlay      bool
			IsHomeTheaterCapable bool
		}{
			IsCoordinatorCapable: true,
			SupportsAirPlay:      false,
			IsHomeTheaterCapable: false,
		}
	}

	role := DeviceRoleNormal
	switch raw.ModelNumber {
	case "S2", "S10", "S33", "S37":
		role = DeviceRoleSub
	}

	return PhysicalDevice{
		UDN:                  raw.UDN,
		IP:                   raw.IP,
		Model:                raw.Model,
		ModelNumber:          raw.ModelNumber,
		RoomName:             raw.RoomName,
		Role:                 role,
		IsCoordinatorCapable: modelInfo.IsCoordinatorCapable,
		SupportsAirPlay:      raw.SupportsAirPlay || modelInfo.SupportsAirPlay,
		LastSeenAt:           raw.DiscoveredAt,
		Capabilities: map[string]any{
			"softwareVersion": raw.SoftwareVersion,
			"hardwareVersion": raw.HardwareVersion,
			"serialNumber":    raw.SerialNumber,
		},
	}
}

func identifyHomeTheaterGroup(members []ZoneMember, devicesByUDN map[string]PhysicalDevice) *HomeTheaterGroup {
	satellites := make([]ZoneMember, 0)
	subs := make([]ZoneMember, 0)
	for _, member := range members {
		if member.IsSatellite {
			satellites = append(satellites, member)
		}
		if member.IsSubwoofer {
			subs = append(subs, member)
		}
	}

	if len(satellites) == 0 && len(subs) == 0 {
		return nil
	}

	var coordinator *ZoneMember
	for _, member := range members {
		if member.IsCoordinator {
			coordinator = &member
			break
		}
	}
	if coordinator == nil {
		return nil
	}

	master, ok := devicesByUDN[coordinator.UDN]
	if !ok {
		return nil
	}

	surroundDevices := make([]PhysicalDevice, 0, len(satellites))
	for _, sat := range satellites {
		if device, ok := devicesByUDN[sat.UDN]; ok {
			surroundDevices = append(surroundDevices, device)
		}
	}

	var subDevice *PhysicalDevice
	for _, sub := range subs {
		if device, ok := devicesByUDN[sub.UDN]; ok {
			copy := device
			subDevice = &copy
			break
		}
	}

	namespace := uuid.MustParse(sonosNamespace)
	groupID := uuid.NewSHA1(namespace, []byte("ht-"+master.UDN)).String()

	return &HomeTheaterGroup{
		GroupID:   groupID,
		Master:    master,
		Surrounds: surroundDevices,
		Sub:       subDevice,
	}
}

func identifyStereoPair(members []ZoneMember, coordinatorUDN string, devicesByUDN map[string]PhysicalDevice) *StereoPair {
	log.Printf("[STEREO-DIAG] Checking zone: %d members, coordinatorUDN=%s", len(members), coordinatorUDN)

	// Log all members and their channel maps
	for i, member := range members {
		log.Printf("[STEREO-DIAG]   member[%d]: UDN=%s, ZoneName=%s, ChannelMapSet=%q",
			i, member.UDN, member.ZoneName, member.ChannelMapSet)
	}

	// Log devicesByUDN keys for comparison
	var keys []string
	for k := range devicesByUDN {
		keys = append(keys, k)
	}
	log.Printf("[STEREO-DIAG]   devicesByUDN keys: %v", keys)

	if len(members) != 2 {
		log.Printf("[STEREO-DIAG]   FAIL: len(members)=%d, expected 2", len(members))
		return nil
	}

	hasStereoPattern := false
	channelSets := make([]string, 0, len(members))
	for _, member := range members {
		if member.ChannelMapSet != "" {
			channelSets = append(channelSets, member.ChannelMapSet)
			if strings.Contains(member.ChannelMapSet, "LF,LF") || strings.Contains(member.ChannelMapSet, "RF,RF") {
				hasStereoPattern = true
			}
		}
	}

	log.Printf("[STEREO-DIAG]   hasStereoPattern=%v, channelSets=%v", hasStereoPattern, channelSets)

	if !hasStereoPattern {
		log.Printf("[STEREO-DIAG]   FAIL: no stereo pattern (LF,LF or RF,RF) found in ChannelMapSet")
		return nil
	}

	var leftUDN string
	var rightUDN string
	for _, channelMap := range channelSets {
		parts := strings.Split(channelMap, ";")
		for _, part := range parts {
			seg := strings.SplitN(part, ":", 2)
			if len(seg) != 2 {
				continue
			}
			udn := seg[0]
			channel := seg[1]
			switch channel {
			case "LF,LF":
				leftUDN = udn
			case "RF,RF":
				rightUDN = udn
			}
		}
	}

	log.Printf("[STEREO-DIAG]   Parsed: leftUDN=%s, rightUDN=%s", leftUDN, rightUDN)

	if leftUDN == "" || rightUDN == "" {
		log.Printf("[STEREO-DIAG]   FAIL: could not parse left/right UDNs from ChannelMapSet")
		return nil
	}

	left, okLeft := devicesByUDN[leftUDN]
	right, okRight := devicesByUDN[rightUDN]

	log.Printf("[STEREO-DIAG]   Direct lookup: leftFound=%v, rightFound=%v", okLeft, okRight)

	// If direct lookup failed, try with normalized UDNs (strip uuid: prefix if present)
	if !okLeft || !okRight {
		normalizedLeft := strings.TrimPrefix(leftUDN, "uuid:")
		normalizedRight := strings.TrimPrefix(rightUDN, "uuid:")
		log.Printf("[STEREO-DIAG]   Trying normalized: leftUDN=%s, rightUDN=%s", normalizedLeft, normalizedRight)

		if !okLeft {
			left, okLeft = devicesByUDN[normalizedLeft]
		}
		if !okRight {
			right, okRight = devicesByUDN[normalizedRight]
		}
		log.Printf("[STEREO-DIAG]   After normalization: leftFound=%v, rightFound=%v", okLeft, okRight)
	}

	if !okLeft || !okRight {
		log.Printf("[STEREO-DIAG]   FAIL: devices not found in devicesByUDN map")
		return nil
	}

	// Compare using normalized UDNs (topology uses short form without _MR suffix)
	coordinator := left
	if normalizeUDN(right.UDN) == coordinatorUDN {
		coordinator = right
	}

	namespace := uuid.MustParse(sonosNamespace)
	pairID := uuid.NewSHA1(namespace, []byte("pair-"+left.UDN)).String()

	log.Printf("[STEREO-DIAG]   SUCCESS: Stereo pair identified - Room=%s, Coordinator=%s",
		cleanRoomName(left.RoomName), coordinator.RoomName)

	return &StereoPair{
		PairID:      pairID,
		RoomName:    cleanRoomName(left.RoomName),
		Left:        left,
		Right:       right,
		Coordinator: coordinator,
	}
}

func identifyStereoPairsByRoomName(physicalDevices []PhysicalDevice, processed map[string]struct{}) []StereoPair {
	leftDevices := make(map[string]PhysicalDevice)
	rightDevices := make(map[string]PhysicalDevice)

	for _, device := range physicalDevices {
		if _, ok := processed[device.UDN]; ok {
			continue
		}

		if match := leftSuffixRegex.FindStringSubmatch(device.RoomName); len(match) > 1 {
			leftDevices[strings.TrimSpace(match[1])] = device
			continue
		}
		if match := rightSuffixRegex.FindStringSubmatch(device.RoomName); len(match) > 1 {
			rightDevices[strings.TrimSpace(match[1])] = device
		}
	}

	pairs := make([]StereoPair, 0)
	for room, left := range leftDevices {
		right, ok := rightDevices[room]
		if !ok {
			continue
		}
		namespace := uuid.MustParse(sonosNamespace)
		pairID := uuid.NewSHA1(namespace, []byte("pair-"+left.UDN)).String()
		pairs = append(pairs, StereoPair{
			PairID:      pairID,
			RoomName:    room,
			Left:        left,
			Right:       right,
			Coordinator: left,
		})
	}

	return pairs
}

// UpdateDeviceLastSeen updates the last-seen timestamp in a topology.
func UpdateDeviceLastSeen(topology DeviceTopology, udn string, timestamp time.Time) DeviceTopology {
	updatedDevices := make([]LogicalDevice, 0, len(topology.Devices))
	for _, logical := range topology.Devices {
		updatedPhysical := make([]PhysicalDevice, 0, len(logical.PhysicalDevices))
		for _, physical := range logical.PhysicalDevices {
			if physical.UDN == udn {
				physical.LastSeenAt = timestamp
			}
			updatedPhysical = append(updatedPhysical, physical)
		}

		latest := updatedPhysical[0].LastSeenAt
		for _, physical := range updatedPhysical {
			if physical.LastSeenAt.After(latest) {
				latest = physical.LastSeenAt
			}
		}

		logical.PhysicalDevices = updatedPhysical
		logical.LastSeenAt = latest
		updatedDevices = append(updatedDevices, logical)
	}

	topology.Devices = updatedDevices
	topology.UpdatedAt = time.Now()
	return topology
}
