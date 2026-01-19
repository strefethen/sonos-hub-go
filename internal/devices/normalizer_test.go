package devices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentifyStereoPair_CombinedChannelMapSet(t *testing.T) {
	// Test case: Combined ChannelMapSet on one member (format: "RINCON_A:LF,LF;RINCON_B:RF,RF")
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_LeftSpeaker": {
			DeviceID:  "left-device-id",
			UDN:       "RINCON_LeftSpeaker",
			IP:        "192.168.1.13",
			Model:     "Play:1",
			RoomName:  "Kitchen (L)",
			Role:      DeviceRoleNormal,
			LastSeenAt: time.Now(),
		},
		"RINCON_RightSpeaker": {
			DeviceID:  "right-device-id",
			UDN:       "RINCON_RightSpeaker",
			IP:        "192.168.1.12",
			Model:     "Play:1",
			RoomName:  "Kitchen (R)",
			Role:      DeviceRoleNormal,
			LastSeenAt: time.Now(),
		},
	}

	members := []ZoneMember{
		{
			UDN:           "RINCON_LeftSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "RINCON_LeftSpeaker:LF,LF;RINCON_RightSpeaker:RF,RF",
			IsCoordinator: true,
		},
		{
			UDN:           "RINCON_RightSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "",
			IsCoordinator: false,
		},
	}

	pair := identifyStereoPair(members, "RINCON_LeftSpeaker", devicesByUDN)

	require.NotNil(t, pair, "Expected stereo pair to be identified")
	assert.Equal(t, "Kitchen", pair.RoomName)
	assert.Equal(t, "RINCON_LeftSpeaker", pair.Left.UDN)
	assert.Equal(t, "RINCON_RightSpeaker", pair.Right.UDN)
	assert.Equal(t, "RINCON_LeftSpeaker", pair.Coordinator.UDN)
}

func TestIdentifyStereoPair_SeparateChannelMapSet(t *testing.T) {
	// Test case: Each member has their own ChannelMapSet
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_LeftSpeaker": {
			DeviceID:  "left-device-id",
			UDN:       "RINCON_LeftSpeaker",
			IP:        "192.168.1.13",
			Model:     "Play:1",
			RoomName:  "Kitchen (L)",
			Role:      DeviceRoleNormal,
			LastSeenAt: time.Now(),
		},
		"RINCON_RightSpeaker": {
			DeviceID:  "right-device-id",
			UDN:       "RINCON_RightSpeaker",
			IP:        "192.168.1.12",
			Model:     "Play:1",
			RoomName:  "Kitchen (R)",
			Role:      DeviceRoleNormal,
			LastSeenAt: time.Now(),
		},
	}

	members := []ZoneMember{
		{
			UDN:           "RINCON_LeftSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "RINCON_LeftSpeaker:LF,LF",
			IsCoordinator: true,
		},
		{
			UDN:           "RINCON_RightSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "RINCON_RightSpeaker:RF,RF",
			IsCoordinator: false,
		},
	}

	pair := identifyStereoPair(members, "RINCON_LeftSpeaker", devicesByUDN)

	require.NotNil(t, pair, "Expected stereo pair to be identified")
	assert.Equal(t, "Kitchen", pair.RoomName)
	assert.Equal(t, "RINCON_LeftSpeaker", pair.Left.UDN)
	assert.Equal(t, "RINCON_RightSpeaker", pair.Right.UDN)
}

func TestIdentifyStereoPair_NotEnoughMembers(t *testing.T) {
	// Test case: Only one member - should not identify as stereo pair
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_Speaker": {
			DeviceID: "device-id",
			UDN:      "RINCON_Speaker",
			IP:       "192.168.1.15",
			Model:    "Playbase",
			RoomName: "Living room",
			Role:     DeviceRoleNormal,
		},
	}

	members := []ZoneMember{
		{
			UDN:           "RINCON_Speaker",
			ZoneName:      "Living room",
			ChannelMapSet: "",
			IsCoordinator: true,
		},
	}

	pair := identifyStereoPair(members, "RINCON_Speaker", devicesByUDN)
	assert.Nil(t, pair, "Single device should not be identified as stereo pair")
}

func TestIdentifyStereoPair_TooManyMembers(t *testing.T) {
	// Test case: More than 2 members (like home theater group) - should not identify as stereo pair
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_Main": {
			DeviceID: "main-id",
			UDN:      "RINCON_Main",
			IP:       "192.168.1.10",
			Model:    "Arc",
			RoomName: "Home Theater",
			Role:     DeviceRoleNormal,
		},
		"RINCON_Left": {
			DeviceID: "left-id",
			UDN:      "RINCON_Left",
			IP:       "192.168.1.17",
			Model:    "Play:1",
			RoomName: "Home Theater (LR)",
			Role:     DeviceRoleSurround,
		},
		"RINCON_Right": {
			DeviceID: "right-id",
			UDN:      "RINCON_Right",
			IP:       "192.168.1.29",
			Model:    "Play:1",
			RoomName: "Home Theater (RR)",
			Role:     DeviceRoleSurround,
		},
		"RINCON_Sub": {
			DeviceID: "sub-id",
			UDN:      "RINCON_Sub",
			IP:       "192.168.1.76",
			Model:    "Sub Mini",
			RoomName: "Home Theater (SW)",
			Role:     DeviceRoleSub,
		},
	}

	members := []ZoneMember{
		{UDN: "RINCON_Main", ZoneName: "Home Theater", IsCoordinator: true},
		{UDN: "RINCON_Left", ZoneName: "Home Theater", IsSatellite: true},
		{UDN: "RINCON_Right", ZoneName: "Home Theater", IsSatellite: true},
		{UDN: "RINCON_Sub", ZoneName: "Home Theater", IsSubwoofer: true},
	}

	pair := identifyStereoPair(members, "RINCON_Main", devicesByUDN)
	assert.Nil(t, pair, "Home theater group should not be identified as stereo pair")
}

func TestIdentifyStereoPair_NoStereoPattern(t *testing.T) {
	// Test case: Two members but no LF,LF or RF,RF patterns
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_A": {DeviceID: "a-id", UDN: "RINCON_A", RoomName: "Room"},
		"RINCON_B": {DeviceID: "b-id", UDN: "RINCON_B", RoomName: "Room"},
	}

	members := []ZoneMember{
		{UDN: "RINCON_A", ZoneName: "Room", ChannelMapSet: ""},
		{UDN: "RINCON_B", ZoneName: "Room", ChannelMapSet: ""},
	}

	pair := identifyStereoPair(members, "RINCON_A", devicesByUDN)
	assert.Nil(t, pair, "Two members without stereo pattern should not be identified as stereo pair")
}

func TestIdentifyStereoPair_CoordinatorIsRight(t *testing.T) {
	// Test case: Right speaker is the coordinator
	devicesByUDN := map[string]PhysicalDevice{
		"RINCON_LeftSpeaker": {
			DeviceID: "left-device-id",
			UDN:      "RINCON_LeftSpeaker",
			RoomName: "Kitchen (L)",
		},
		"RINCON_RightSpeaker": {
			DeviceID: "right-device-id",
			UDN:      "RINCON_RightSpeaker",
			RoomName: "Kitchen (R)",
		},
	}

	members := []ZoneMember{
		{
			UDN:           "RINCON_LeftSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "RINCON_LeftSpeaker:LF,LF;RINCON_RightSpeaker:RF,RF",
			IsCoordinator: false,
		},
		{
			UDN:           "RINCON_RightSpeaker",
			ZoneName:      "Kitchen",
			ChannelMapSet: "",
			IsCoordinator: true,
		},
	}

	// The coordinator UDN is the right speaker
	pair := identifyStereoPair(members, "RINCON_RightSpeaker", devicesByUDN)

	require.NotNil(t, pair)
	assert.Equal(t, "RINCON_RightSpeaker", pair.Coordinator.UDN, "Coordinator should be the right speaker")
}

func TestCleanRoomName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Kitchen (L)", "Kitchen"},
		{"Kitchen (R)", "Kitchen"},
		{"Kitchen", "Kitchen"},
		{"Home Theater (LF,RF)", "Home Theater"},
		{"Home Theater (LR)", "Home Theater"},
		{"Home Theater (RR)", "Home Theater"},
		{"Home Theater (SW)", "Home Theater"},
		{"  Kitchen (L)  ", "Kitchen"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := cleanRoomName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIdentifyStereoPairsByRoomName_Fallback(t *testing.T) {
	// Test the fallback stereo pair detection by room name suffixes
	physicalDevices := []PhysicalDevice{
		{
			DeviceID: "left-id",
			UDN:      "RINCON_Left",
			RoomName: "Kitchen (L)",
			IP:       "192.168.1.13",
		},
		{
			DeviceID: "right-id",
			UDN:      "RINCON_Right",
			RoomName: "Kitchen (R)",
			IP:       "192.168.1.12",
		},
		{
			DeviceID: "standalone-id",
			UDN:      "RINCON_Standalone",
			RoomName: "Living room",
			IP:       "192.168.1.15",
		},
	}

	processed := make(map[string]struct{})
	pairs := identifyStereoPairsByRoomName(physicalDevices, processed)

	require.Len(t, pairs, 1, "Should find exactly one stereo pair")
	assert.Equal(t, "Kitchen", pairs[0].RoomName)
	assert.Equal(t, "RINCON_Left", pairs[0].Left.UDN)
	assert.Equal(t, "RINCON_Right", pairs[0].Right.UDN)
}

func TestIdentifyStereoPairsByRoomName_AlreadyProcessed(t *testing.T) {
	// Test that already processed devices are skipped
	physicalDevices := []PhysicalDevice{
		{
			DeviceID: "left-id",
			UDN:      "RINCON_Left",
			RoomName: "Kitchen (L)",
		},
		{
			DeviceID: "right-id",
			UDN:      "RINCON_Right",
			RoomName: "Kitchen (R)",
		},
	}

	// Mark the left speaker as already processed
	processed := map[string]struct{}{
		"RINCON_Left": {},
	}

	pairs := identifyStereoPairsByRoomName(physicalDevices, processed)
	assert.Len(t, pairs, 0, "Should not create pair when one device is already processed")
}

func TestNormalizeUDN(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"RINCON_12345_MR", "RINCON_12345"},
		{"RINCON_12345_LR", "RINCON_12345"},
		{"RINCON_12345_RR", "RINCON_12345"},
		{"RINCON_12345_SW", "RINCON_12345"},
		{"RINCON_12345_LF", "RINCON_12345"},
		{"RINCON_12345_RF", "RINCON_12345"},
		{"RINCON_12345", "RINCON_12345"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeUDN(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
