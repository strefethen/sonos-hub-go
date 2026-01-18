package sonoscloud

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenPairJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	token := TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}

	data, err := json.Marshal(token)
	require.NoError(t, err)

	var decoded TokenPair
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, token.AccessToken, decoded.AccessToken)
	require.Equal(t, token.RefreshToken, decoded.RefreshToken)
	require.Equal(t, token.TokenType, decoded.TokenType)
	require.Equal(t, token.Scope, decoded.Scope)
}

func TestTokenPair_IsExpired(t *testing.T) {
	// Not expired
	token := TokenPair{
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.False(t, token.IsExpired())

	// Expired
	token.ExpiresAt = time.Now().Add(-time.Hour)
	require.True(t, token.IsExpired())
}

func TestTokenPair_ExpiresWithin(t *testing.T) {
	// Expires in 30 minutes
	token := TokenPair{
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Should expire within 1 hour
	require.True(t, token.ExpiresWithin(time.Hour))

	// Should not expire within 15 minutes
	require.False(t, token.ExpiresWithin(15*time.Minute))
}

func TestConnectionStatusJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	status := ConnectionStatus{
		Connected:   true,
		ExpiresAt:   &expiresAt,
		ConnectedAt: &now,
		Scope:       "playback-control-all",
	}

	data, err := json.Marshal(status)
	require.NoError(t, err)

	var decoded ConnectionStatus
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.True(t, decoded.Connected)
	require.NotNil(t, decoded.ExpiresAt)
	require.NotNil(t, decoded.ConnectedAt)
	require.Equal(t, "playback-control-all", decoded.Scope)
}

func TestConnectionStatusJSON_OmitsEmpty(t *testing.T) {
	status := ConnectionStatus{
		Connected: false,
	}

	data, err := json.Marshal(status)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.False(t, m["connected"].(bool))
	_, hasExpiresAt := m["expires_at"]
	require.False(t, hasExpiresAt)
	_, hasConnectedAt := m["connected_at"]
	require.False(t, hasConnectedAt)
}

func TestHouseholdJSON(t *testing.T) {
	household := Household{
		ID:   "household-123",
		Name: "My Home",
	}

	data, err := json.Marshal(household)
	require.NoError(t, err)

	var decoded Household
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "household-123", decoded.ID)
	require.Equal(t, "My Home", decoded.Name)
}

func TestHouseholdsResponseJSON(t *testing.T) {
	resp := HouseholdsResponse{
		Households: []Household{
			{ID: "household-1", Name: "Home 1"},
			{ID: "household-2", Name: "Home 2"},
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded HouseholdsResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Len(t, decoded.Households, 2)
	require.Equal(t, "household-1", decoded.Households[0].ID)
	require.Equal(t, "household-2", decoded.Households[1].ID)
}

func TestGroupJSON(t *testing.T) {
	group := Group{
		ID:            "group-123",
		Name:          "Living Room",
		CoordinatorID: "player-1",
		PlaybackState: "PLAYING",
		PlayerIDs:     []string{"player-1", "player-2"},
		AreaIDs:       []string{"area-1"},
	}

	data, err := json.Marshal(group)
	require.NoError(t, err)

	var decoded Group
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "group-123", decoded.ID)
	require.Equal(t, "Living Room", decoded.Name)
	require.Equal(t, "player-1", decoded.CoordinatorID)
	require.Equal(t, "PLAYING", decoded.PlaybackState)
	require.Len(t, decoded.PlayerIDs, 2)
	require.Len(t, decoded.AreaIDs, 1)
}

func TestPlayerJSON(t *testing.T) {
	player := Player{
		ID:              "player-123",
		Name:            "Kitchen",
		WebSocketURL:    "wss://example.com/ws",
		SoftwareVersion: "15.0",
		APIVersion:      "1.25.0",
		MinAPIVersion:   "1.1.0",
		Capabilities:    []string{"PLAYBACK", "CLOUD", "AUDIO_CLIP"},
		DeviceIDs:       []string{"device-1"},
		Icon:            "speaker-icon",
	}

	data, err := json.Marshal(player)
	require.NoError(t, err)

	var decoded Player
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "player-123", decoded.ID)
	require.Equal(t, "Kitchen", decoded.Name)
	require.Equal(t, "wss://example.com/ws", decoded.WebSocketURL)
	require.Equal(t, "15.0", decoded.SoftwareVersion)
	require.Equal(t, "1.25.0", decoded.APIVersion)
	require.Equal(t, "1.1.0", decoded.MinAPIVersion)
	require.Len(t, decoded.Capabilities, 3)
	require.Contains(t, decoded.Capabilities, "AUDIO_CLIP")
}

func TestAudioClipRequestJSON(t *testing.T) {
	clip := AudioClipRequest{
		Name:              "Doorbell",
		AppID:             "com.example.app",
		StreamURL:         "https://example.com/audio.mp3",
		ClipType:          "CUSTOM",
		Priority:          "HIGH",
		Volume:            50,
		HTTPAuthorization: "Bearer token123",
	}

	data, err := json.Marshal(clip)
	require.NoError(t, err)

	var decoded AudioClipRequest
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "Doorbell", decoded.Name)
	require.Equal(t, "com.example.app", decoded.AppID)
	require.Equal(t, "https://example.com/audio.mp3", decoded.StreamURL)
	require.Equal(t, "CUSTOM", decoded.ClipType)
	require.Equal(t, "HIGH", decoded.Priority)
	require.Equal(t, 50, decoded.Volume)
	require.Equal(t, "Bearer token123", decoded.HTTPAuthorization)
}

func TestAudioClipRequestJSON_OmitsEmpty(t *testing.T) {
	clip := AudioClipRequest{
		AppID: "com.example.app",
	}

	data, err := json.Marshal(clip)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	require.Equal(t, "com.example.app", m["appId"])
	_, hasName := m["name"]
	require.False(t, hasName)
	_, hasStreamURL := m["streamUrl"]
	require.False(t, hasStreamURL)
}

func TestAudioClipResponseJSON(t *testing.T) {
	resp := AudioClipResponse{
		ID:       "clip-123",
		Name:     "Doorbell",
		AppID:    "com.example.app",
		Priority: "HIGH",
		ClipType: "CUSTOM",
		Status:   "ACTIVE",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded AudioClipResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "clip-123", decoded.ID)
	require.Equal(t, "Doorbell", decoded.Name)
	require.Equal(t, "com.example.app", decoded.AppID)
	require.Equal(t, "HIGH", decoded.Priority)
	require.Equal(t, "CUSTOM", decoded.ClipType)
	require.Equal(t, "ACTIVE", decoded.Status)
}

func TestGroupsResponseJSON(t *testing.T) {
	resp := GroupsResponse{
		Groups: []Group{
			{ID: "group-1", Name: "Living Room", CoordinatorID: "player-1", PlaybackState: "PLAYING", PlayerIDs: []string{"player-1"}},
		},
		Players: []Player{
			{ID: "player-1", Name: "Sonos One"},
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded GroupsResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Len(t, decoded.Groups, 1)
	require.Len(t, decoded.Players, 1)
	require.Equal(t, "group-1", decoded.Groups[0].ID)
	require.Equal(t, "player-1", decoded.Players[0].ID)
}

func TestAuthStartResponseJSON(t *testing.T) {
	resp := AuthStartResponse{
		AuthURL: "https://api.sonos.com/login/v3/oauth?client_id=abc",
		State:   "random-state-123",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded AuthStartResponse
	require.NoError(t, json.Unmarshal(data, &decoded))

	require.Equal(t, "https://api.sonos.com/login/v3/oauth?client_id=abc", decoded.AuthURL)
	require.Equal(t, "random-state-123", decoded.State)
}

func TestAPIError(t *testing.T) {
	err := &APIError{
		ErrorCode:  "ERROR_UNAUTHORIZED",
		Reason:     "Invalid access token",
		HTTPStatus: 401,
	}

	require.Equal(t, "Invalid access token", err.Error())

	err2 := &APIError{
		ErrorCode:  "ERROR_UNAUTHORIZED",
		HTTPStatus: 401,
	}

	require.Equal(t, "ERROR_UNAUTHORIZED", err2.Error())
}

func TestConstants(t *testing.T) {
	require.Equal(t, "https://api.sonos.com/login/v3/oauth", SonosAuthURL)
	require.Equal(t, "https://api.sonos.com/login/v3/oauth/access", SonosTokenURL)
	require.Equal(t, "https://api.ws.sonos.com/control/api/v1", SonosAPIBase)
	require.Equal(t, "playback-control-all", DefaultScope)
}
