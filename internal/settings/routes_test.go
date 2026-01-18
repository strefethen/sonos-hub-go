package settings

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTVRoutingSettingsDefaults(t *testing.T) {
	settings := TVRoutingSettings{
		ArcTVPolicy:      "SKIP",
		AlwaysSkipOnTV:   false,
		RetryOnTVTimeout: 5,
		FallbackRooms:    []string{},
		UpdatedAt:        time.Now(),
	}

	require.Equal(t, "SKIP", settings.ArcTVPolicy)
	require.False(t, settings.AlwaysSkipOnTV)
	require.Equal(t, 5, settings.RetryOnTVTimeout)
	require.Empty(t, settings.FallbackRooms)
	require.Nil(t, settings.FallbackDeviceID)
}

func TestTVRoutingSettingsWithFallback(t *testing.T) {
	fallbackDeviceID := "device-123"
	settings := TVRoutingSettings{
		ArcTVPolicy:      "USE_FALLBACK",
		FallbackDeviceID: &fallbackDeviceID,
		FallbackRooms:    []string{"living_room", "kitchen"},
		AlwaysSkipOnTV:   false,
		RetryOnTVTimeout: 10,
		UpdatedAt:        time.Now(),
	}

	require.Equal(t, "USE_FALLBACK", settings.ArcTVPolicy)
	require.NotNil(t, settings.FallbackDeviceID)
	require.Equal(t, "device-123", *settings.FallbackDeviceID)
	require.Len(t, settings.FallbackRooms, 2)
	require.Contains(t, settings.FallbackRooms, "living_room")
	require.Contains(t, settings.FallbackRooms, "kitchen")
}

func TestUpdateTVRoutingInput(t *testing.T) {
	policy := "ALWAYS_PLAY"
	alwaysSkip := true
	timeout := 15

	input := UpdateTVRoutingInput{
		ArcTVPolicy:      &policy,
		AlwaysSkipOnTV:   &alwaysSkip,
		RetryOnTVTimeout: &timeout,
		FallbackRooms:    []string{"bedroom"},
	}

	require.NotNil(t, input.ArcTVPolicy)
	require.Equal(t, "ALWAYS_PLAY", *input.ArcTVPolicy)
	require.NotNil(t, input.AlwaysSkipOnTV)
	require.True(t, *input.AlwaysSkipOnTV)
	require.NotNil(t, input.RetryOnTVTimeout)
	require.Equal(t, 15, *input.RetryOnTVTimeout)
	require.Len(t, input.FallbackRooms, 1)
}

func TestUpdateTVRoutingInputPartial(t *testing.T) {
	policy := "SKIP"
	input := UpdateTVRoutingInput{
		ArcTVPolicy: &policy,
	}

	require.NotNil(t, input.ArcTVPolicy)
	require.Nil(t, input.AlwaysSkipOnTV)
	require.Nil(t, input.RetryOnTVTimeout)
	require.Nil(t, input.FallbackDeviceID)
	require.Nil(t, input.FallbackRooms)
}

func TestFormatTVRoutingSettings(t *testing.T) {
	fallbackDeviceID := "device-123"
	settings := &TVRoutingSettings{
		ArcTVPolicy:      "USE_FALLBACK",
		FallbackDeviceID: &fallbackDeviceID,
		FallbackRooms:    []string{"living_room"},
		AlwaysSkipOnTV:   true,
		RetryOnTVTimeout: 10,
		UpdatedAt:        time.Now(),
	}

	formatted := formatTVRoutingSettings(settings)

	require.Equal(t, "USE_FALLBACK", formatted["arc_tv_policy"])
	require.Equal(t, "device-123", formatted["fallback_device_id"])
	require.Equal(t, true, formatted["always_skip_on_tv"])
	require.Equal(t, 10, formatted["retry_on_tv_timeout_seconds"])
	require.NotNil(t, formatted["fallback_rooms"])
	require.NotNil(t, formatted["updated_at"])
}

func TestFormatTVRoutingSettingsWithoutFallbackDevice(t *testing.T) {
	settings := &TVRoutingSettings{
		ArcTVPolicy:      "SKIP",
		FallbackRooms:    []string{},
		AlwaysSkipOnTV:   false,
		RetryOnTVTimeout: 5,
		UpdatedAt:        time.Now(),
	}

	formatted := formatTVRoutingSettings(settings)

	require.Equal(t, "SKIP", formatted["arc_tv_policy"])
	_, hasFallbackDevice := formatted["fallback_device_id"]
	require.False(t, hasFallbackDevice)
}

func TestTVRoutingSettingsJSON(t *testing.T) {
	fallbackDeviceID := "device-123"
	settings := TVRoutingSettings{
		ArcTVPolicy:      "USE_FALLBACK",
		FallbackDeviceID: &fallbackDeviceID,
		FallbackRooms:    []string{"living_room", "kitchen"},
		AlwaysSkipOnTV:   true,
		RetryOnTVTimeout: 10,
		UpdatedAt:        time.Now(),
	}

	data, err := json.Marshal(settings)
	require.NoError(t, err)

	var decoded TVRoutingSettings
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "USE_FALLBACK", decoded.ArcTVPolicy)
	require.NotNil(t, decoded.FallbackDeviceID)
	require.Equal(t, "device-123", *decoded.FallbackDeviceID)
	require.Len(t, decoded.FallbackRooms, 2)
	require.True(t, decoded.AlwaysSkipOnTV)
	require.Equal(t, 10, decoded.RetryOnTVTimeout)
}

func TestUpdateTVRoutingInputJSON(t *testing.T) {
	policy := "SKIP"
	alwaysSkip := false

	input := UpdateTVRoutingInput{
		ArcTVPolicy:    &policy,
		AlwaysSkipOnTV: &alwaysSkip,
		FallbackRooms:  []string{"bedroom"},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded UpdateTVRoutingInput
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.ArcTVPolicy)
	require.Equal(t, "SKIP", *decoded.ArcTVPolicy)
	require.NotNil(t, decoded.AlwaysSkipOnTV)
	require.False(t, *decoded.AlwaysSkipOnTV)
	require.Len(t, decoded.FallbackRooms, 1)
}

func TestValidPolicies(t *testing.T) {
	validPolicies := []string{"SKIP", "USE_FALLBACK", "ALWAYS_PLAY"}

	for _, policy := range validPolicies {
		settings := TVRoutingSettings{
			ArcTVPolicy: policy,
		}
		require.Equal(t, policy, settings.ArcTVPolicy)
	}
}
