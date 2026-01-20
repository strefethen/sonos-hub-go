package scene

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSceneMemberJSON(t *testing.T) {
	volume := 50
	mute := false

	member := SceneMember{
		UDN:          "RINCON_TEST123456789",
		RoomName:     "Living Room",
		TargetVolume: &volume,
		Mute:         &mute,
	}

	data, err := json.Marshal(member)
	require.NoError(t, err)

	var decoded SceneMember
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, member.UDN, decoded.UDN)
	require.Equal(t, member.RoomName, decoded.RoomName)
	require.NotNil(t, decoded.TargetVolume)
	require.Equal(t, 50, *decoded.TargetVolume)
	require.NotNil(t, decoded.Mute)
	require.False(t, *decoded.Mute)
}

func TestSceneMemberJSONOmitsEmpty(t *testing.T) {
	member := SceneMember{
		UDN: "RINCON_TEST123456789",
	}

	data, err := json.Marshal(member)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "RINCON_TEST123456789", m["udn"])
	_, hasRoomName := m["room_name"]
	require.False(t, hasRoomName)
	_, hasVolume := m["target_volume"]
	require.False(t, hasVolume)
	_, hasMute := m["mute"]
	require.False(t, hasMute)
}

func TestVolumeRampJSON(t *testing.T) {
	duration := 5000
	ramp := VolumeRamp{
		Enabled:    true,
		DurationMs: &duration,
		Curve:      "ease-in",
	}

	data, err := json.Marshal(ramp)
	require.NoError(t, err)

	var decoded VolumeRamp
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.True(t, decoded.Enabled)
	require.NotNil(t, decoded.DurationMs)
	require.Equal(t, 5000, *decoded.DurationMs)
	require.Equal(t, "ease-in", decoded.Curve)
}

func TestTeardownJSON(t *testing.T) {
	ungroupAfter := 30000
	teardown := Teardown{
		UngroupAfterMs:       &ungroupAfter,
		RestorePreviousState: true,
	}

	data, err := json.Marshal(teardown)
	require.NoError(t, err)

	var decoded Teardown
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.UngroupAfterMs)
	require.Equal(t, 30000, *decoded.UngroupAfterMs)
	require.True(t, decoded.RestorePreviousState)
}

func TestSceneJSON(t *testing.T) {
	description := "Test scene"
	volume := 40
	now := time.Now().UTC().Truncate(time.Second)

	scene := Scene{
		SceneID:               "scene-123",
		Name:                  "Morning Music",
		Description:           &description,
		CoordinatorPreference: string(CoordinatorPreferenceArcFirst),
		FallbackPolicy:        string(FallbackPolicyPlaybaseIfArcTVActive),
		Members: []SceneMember{
			{UDN: "RINCON_TEST001", TargetVolume: &volume},
			{UDN: "RINCON_TEST002", RoomName: "Kitchen"},
		},
		VolumeRamp: &VolumeRamp{Enabled: false},
		Teardown:   nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	data, err := json.Marshal(scene)
	require.NoError(t, err)

	var decoded Scene
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, scene.SceneID, decoded.SceneID)
	require.Equal(t, scene.Name, decoded.Name)
	require.NotNil(t, decoded.Description)
	require.Equal(t, "Test scene", *decoded.Description)
	require.Equal(t, "ARC_FIRST", decoded.CoordinatorPreference)
	require.Equal(t, "PLAYBASE_IF_ARC_TV_ACTIVE", decoded.FallbackPolicy)
	require.Len(t, decoded.Members, 2)
	require.NotNil(t, decoded.VolumeRamp)
	require.False(t, decoded.VolumeRamp.Enabled)
	require.Nil(t, decoded.Teardown)
}

func TestExecutionStepJSON(t *testing.T) {
	startedAt := time.Now().UTC().Truncate(time.Second)
	endedAt := startedAt.Add(time.Second)

	step := ExecutionStep{
		Step:      "acquire_lock",
		Status:    StepStatusCompleted,
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
		Error:     "",
		Details:   map[string]any{"coordinator": "device-123"},
	}

	data, err := json.Marshal(step)
	require.NoError(t, err)

	var decoded ExecutionStep
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "acquire_lock", decoded.Step)
	require.Equal(t, StepStatusCompleted, decoded.Status)
	require.NotNil(t, decoded.StartedAt)
	require.NotNil(t, decoded.EndedAt)
	require.NotNil(t, decoded.Details)
	require.Equal(t, "device-123", decoded.Details["coordinator"])
}

func TestVerificationJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	verification := Verification{
		PlaybackConfirmed:       true,
		TransportState:          "PLAYING",
		TrackURI:                "x-rincon-queue:RINCON_123#0",
		CheckedAt:               now,
		VerificationUnavailable: false,
	}

	data, err := json.Marshal(verification)
	require.NoError(t, err)

	var decoded Verification
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.True(t, decoded.PlaybackConfirmed)
	require.Equal(t, "PLAYING", decoded.TransportState)
	require.Equal(t, "x-rincon-queue:RINCON_123#0", decoded.TrackURI)
	require.False(t, decoded.VerificationUnavailable)
}

func TestSceneExecutionJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	endedAt := now.Add(5 * time.Second)
	idempotencyKey := "idem-123"
	coordinator := "device-123"
	errMsg := "test error"

	execution := SceneExecution{
		SceneExecutionID:    "exec-123",
		SceneID:             "scene-123",
		IdempotencyKey:      &idempotencyKey,
		CoordinatorUsedUDN:  &coordinator,
		Status:              ExecutionStatusFailed,
		StartedAt:           now,
		EndedAt:             &endedAt,
		Steps:               DefaultExecutionSteps(),
		Verification:        nil,
		Error:               &errMsg,
	}

	data, err := json.Marshal(execution)
	require.NoError(t, err)

	var decoded SceneExecution
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "exec-123", decoded.SceneExecutionID)
	require.Equal(t, "scene-123", decoded.SceneID)
	require.NotNil(t, decoded.IdempotencyKey)
	require.Equal(t, "idem-123", *decoded.IdempotencyKey)
	require.NotNil(t, decoded.CoordinatorUsedUDN)
	require.Equal(t, "device-123", *decoded.CoordinatorUsedUDN)
	require.Equal(t, ExecutionStatusFailed, decoded.Status)
	require.NotNil(t, decoded.EndedAt)
	require.Len(t, decoded.Steps, 8)
	require.Nil(t, decoded.Verification)
	require.NotNil(t, decoded.Error)
	require.Equal(t, "test error", *decoded.Error)
}

func TestDefaultExecutionSteps(t *testing.T) {
	steps := DefaultExecutionSteps()

	require.Len(t, steps, 8)

	expectedSteps := []string{
		"acquire_lock",
		"determine_coordinator",
		"ensure_group",
		"apply_volume",
		"pre_flight_check",
		"start_playback",
		"verify_playback",
		"release_lock",
	}

	for i, expected := range expectedSteps {
		require.Equal(t, expected, steps[i].Step)
		require.Equal(t, StepStatusPending, steps[i].Status)
		require.Nil(t, steps[i].StartedAt)
		require.Nil(t, steps[i].EndedAt)
		require.Empty(t, steps[i].Error)
		require.Nil(t, steps[i].Details)
	}
}

func TestMusicContentJSON(t *testing.T) {
	content := MusicContent{
		Type:            "sonos_favorite",
		SonosFavoriteID: "fav-123",
	}

	data, err := json.Marshal(content)
	require.NoError(t, err)

	var decoded MusicContent
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "sonos_favorite", decoded.Type)
	require.Equal(t, "fav-123", decoded.SonosFavoriteID)
}

func TestExecuteOptionsJSON(t *testing.T) {
	options := ExecuteOptions{
		MusicContent: &MusicContent{
			Type:            "sonos_favorite",
			SonosFavoriteID: "fav-123",
		},
		QueueMode:     QueueModeReplaceAndPlay,
		GroupBehavior: GroupBehaviorAutoRedirect,
		TVPolicy:      TVPolicySkip,
	}

	data, err := json.Marshal(options)
	require.NoError(t, err)

	var decoded ExecuteOptions
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.MusicContent)
	require.Equal(t, "sonos_favorite", decoded.MusicContent.Type)
	require.Equal(t, QueueModeReplaceAndPlay, decoded.QueueMode)
	require.Equal(t, GroupBehaviorAutoRedirect, decoded.GroupBehavior)
	require.Equal(t, TVPolicySkip, decoded.TVPolicy)
}

func TestDeviceResultJSON(t *testing.T) {
	result := DeviceResult{
		UDN:     "RINCON_TEST123456789",
		Success: false,
		Error:   "connection timeout",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded DeviceResult
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "RINCON_TEST123456789", decoded.UDN)
	require.False(t, decoded.Success)
	require.Equal(t, "connection timeout", decoded.Error)
}
