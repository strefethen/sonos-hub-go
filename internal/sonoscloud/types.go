package sonoscloud

import "time"

// Sonos Cloud API constants
const (
	SonosAuthURL  = "https://api.sonos.com/login/v3/oauth"
	SonosTokenURL = "https://api.sonos.com/login/v3/oauth/access"
	SonosAPIBase  = "https://api.ws.sonos.com/control/api/v1"
	DefaultScope  = "playback-control-all"
)

// TokenPair holds OAuth access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
	HouseholdID  *string   `json:"household_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// IsExpired returns true if the token has expired
func (t *TokenPair) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// ExpiresWithin returns true if the token expires within the given duration
func (t *TokenPair) ExpiresWithin(d time.Duration) bool {
	return time.Now().Add(d).After(t.ExpiresAt)
}

// ConnectionStatus represents the current Sonos Cloud connection state
type ConnectionStatus struct {
	Connected   bool       `json:"connected"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ConnectedAt *time.Time `json:"connected_at,omitempty"`
	Scope       string     `json:"scope,omitempty"`
}

// Household represents a Sonos household
type Household struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// HouseholdsResponse is the API response for listing households
type HouseholdsResponse struct {
	Households []Household `json:"households"`
}

// Group represents a Sonos playback group
type Group struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CoordinatorID string   `json:"coordinatorId"`
	PlaybackState string   `json:"playbackState"`
	PlayerIDs     []string `json:"playerIds"`
	AreaIDs       []string `json:"areaIds,omitempty"`
}

// GroupsResponse is the API response for listing groups
type GroupsResponse struct {
	Groups  []Group  `json:"groups"`
	Players []Player `json:"players"`
}

// Player represents a Sonos player
type Player struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	WebSocketURL    string   `json:"websocketUrl,omitempty"`
	SoftwareVersion string   `json:"softwareVersion,omitempty"`
	APIVersion      string   `json:"apiVersion,omitempty"`
	MinAPIVersion   string   `json:"minApiVersion,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	DeviceIDs       []string `json:"deviceIds,omitempty"`
	Icon            string   `json:"icon,omitempty"`
}

// AudioClipRequest represents a request to play an audio clip
type AudioClipRequest struct {
	Name              string `json:"name,omitempty"`
	AppID             string `json:"appId"`
	StreamURL         string `json:"streamUrl,omitempty"`
	ClipType          string `json:"clipType,omitempty"`
	Priority          string `json:"priority,omitempty"`
	Volume            int    `json:"volume,omitempty"`
	HTTPAuthorization string `json:"httpAuthorization,omitempty"`
}

// AudioClipResponse represents the response from playing an audio clip
type AudioClipResponse struct {
	Object   string `json:"object"`
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	AppID    string `json:"appId"`
	Priority string `json:"priority,omitempty"`
	ClipType string `json:"clipType,omitempty"`
	Status   string `json:"status,omitempty"`
}

// tokenResponse is the internal response from the OAuth token endpoint
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// AuthStartResponse is the response for starting OAuth flow
type AuthStartResponse struct {
	Object  string `json:"object"`
	AuthURL string `json:"auth_url"`
	State   string `json:"state"`
}

// APIError represents an error from the Sonos Cloud API
type APIError struct {
	ErrorCode   string `json:"errorCode"`
	Reason      string `json:"reason"`
	HTTPStatus  int    `json:"-"`
}

func (e *APIError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return e.ErrorCode
}
