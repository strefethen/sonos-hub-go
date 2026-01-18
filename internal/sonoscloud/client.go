package sonoscloud

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	stateTTL          = 10 * time.Minute
	stateCleanupInterval = 60 * time.Second
	tokenRefreshBuffer   = 5 * time.Minute
)

// stateEntry holds a state token with its expiration time.
type stateEntry struct {
	state     string
	expiresAt time.Time
}

// StateStore manages OAuth state tokens for CSRF protection.
type StateStore struct {
	mu      sync.RWMutex
	states  map[string]stateEntry
	stopCh  chan struct{}
	stopped bool
}

// NewStateStore creates a new StateStore and starts the cleanup goroutine.
func NewStateStore() *StateStore {
	s := &StateStore{
		states: make(map[string]stateEntry),
		stopCh: make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

// Generate creates a new random state token and stores it.
func (s *StateStore) Generate() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)

	s.mu.Lock()
	s.states[state] = stateEntry{
		state:     state,
		expiresAt: time.Now().Add(stateTTL),
	}
	s.mu.Unlock()

	return state, nil
}

// Validate checks if a state token is valid and removes it.
func (s *StateStore) Validate(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.states[state]
	if !ok {
		return false
	}

	delete(s.states, state)

	if time.Now().After(entry.expiresAt) {
		return false
	}

	return true
}

// Stop stops the cleanup goroutine.
func (s *StateStore) Stop() {
	s.mu.Lock()
	if !s.stopped {
		s.stopped = true
		close(s.stopCh)
	}
	s.mu.Unlock()
}

func (s *StateStore) cleanupLoop() {
	ticker := time.NewTicker(stateCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *StateStore) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for state, entry := range s.states {
		if now.After(entry.expiresAt) {
			delete(s.states, state)
		}
	}
}

// Client handles OAuth operations with Sonos Cloud.
type Client struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
	repo         *Repository
	stateStore   *StateStore
	mu           sync.RWMutex
}

// NewClient creates a new Sonos Cloud OAuth client.
func NewClient(clientID, clientSecret, redirectURI string, repo *Repository) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		repo:       repo,
		stateStore: NewStateStore(),
	}
}

// Stop cleans up resources.
func (c *Client) Stop() {
	c.stateStore.Stop()
}

// GetAuthURL returns the OAuth authorization URL with a new state token.
func (c *Client) GetAuthURL() (*AuthStartResponse, error) {
	state, err := c.stateStore.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate state: %w", err)
	}

	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("response_type", "code")
	params.Set("state", state)
	params.Set("scope", DefaultScope)
	params.Set("redirect_uri", c.redirectURI)

	authURL := SonosAuthURL + "?" + params.Encode()

	return &AuthStartResponse{
		AuthURL: authURL,
		State:   state,
	}, nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *Client) ExchangeCode(code, state string) (*TokenPair, error) {
	if !c.stateStore.Validate(state) {
		return nil, fmt.Errorf("invalid or expired state")
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", c.redirectURI)

	token, err := c.tokenRequest(data)
	if err != nil {
		return nil, err
	}

	if err := c.repo.SaveToken(token); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	return token, nil
}

// RefreshToken refreshes the access token using the refresh token.
func (c *Client) RefreshToken() (*TokenPair, error) {
	existing, err := c.repo.GetToken()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	if existing == nil {
		return nil, fmt.Errorf("no token to refresh")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", existing.RefreshToken)

	token, err := c.tokenRequest(data)
	if err != nil {
		return nil, err
	}

	// Preserve the original created_at
	token.CreatedAt = existing.CreatedAt

	if err := c.repo.SaveToken(token); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	return token, nil
}

// GetStatus returns the current connection status.
func (c *Client) GetStatus() (*ConnectionStatus, error) {
	token, err := c.repo.GetToken()
	if err != nil {
		return nil, err
	}

	if token == nil {
		return &ConnectionStatus{
			Connected: false,
		}, nil
	}

	return &ConnectionStatus{
		Connected:   !token.IsExpired(),
		ExpiresAt:   &token.ExpiresAt,
		ConnectedAt: &token.CreatedAt,
		Scope:       token.Scope,
	}, nil
}

// Disconnect removes the stored tokens.
func (c *Client) Disconnect() error {
	return c.repo.DeleteToken()
}

// GetValidToken returns a valid access token, refreshing if necessary.
func (c *Client) GetValidToken() (*TokenPair, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	token, err := c.repo.GetToken()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	if token == nil {
		return nil, fmt.Errorf("not connected to Sonos Cloud")
	}

	// Refresh if token expires within 5 minutes
	if token.ExpiresWithin(tokenRefreshBuffer) {
		refreshed, err := c.refreshTokenLocked(token)
		if err != nil {
			// If refresh fails but token is still valid, use existing
			if !token.IsExpired() {
				return token, nil
			}
			return nil, fmt.Errorf("refresh token: %w", err)
		}
		return refreshed, nil
	}

	return token, nil
}

func (c *Client) refreshTokenLocked(existing *TokenPair) (*TokenPair, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", existing.RefreshToken)

	token, err := c.tokenRequest(data)
	if err != nil {
		return nil, err
	}

	token.CreatedAt = existing.CreatedAt

	if err := c.repo.SaveToken(token); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}

	return token, nil
}

func (c *Client) tokenRequest(data url.Values) (*TokenPair, error) {
	req, err := http.NewRequest(http.MethodPost, SonosTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+c.basicAuth())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr APIError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.ErrorCode != "" {
			apiErr.HTTPStatus = resp.StatusCode
			return nil, &apiErr
		}
		return nil, fmt.Errorf("token request failed: %s", resp.Status)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &TokenPair{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

func (c *Client) basicAuth() string {
	auth := c.clientID + ":" + c.clientSecret
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// GetHouseholds retrieves all households for the authenticated user.
func (c *Client) GetHouseholds() ([]Household, error) {
	var resp HouseholdsResponse
	if err := c.apiRequest(http.MethodGet, "/households", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Households, nil
}

// GetGroups retrieves all groups for a household.
func (c *Client) GetGroups(householdID string) (*GroupsResponse, error) {
	var resp GroupsResponse
	path := fmt.Sprintf("/households/%s/groups", householdID)
	if err := c.apiRequest(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPlayers retrieves all players for a household.
func (c *Client) GetPlayers(householdID string) ([]Player, error) {
	groups, err := c.GetGroups(householdID)
	if err != nil {
		return nil, err
	}
	return groups.Players, nil
}

// PlayAudioClip plays an audio clip on a player.
func (c *Client) PlayAudioClip(playerID string, clip *AudioClipRequest) (*AudioClipResponse, error) {
	var resp AudioClipResponse
	path := fmt.Sprintf("/players/%s/audioClip", playerID)
	if err := c.apiRequest(http.MethodPost, path, clip, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) apiRequest(method, path string, body any, result any) error {
	token, err := c.GetValidToken()
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, SonosAPIBase+path, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.ErrorCode != "" {
			apiErr.HTTPStatus = resp.StatusCode
			return &apiErr
		}
		return fmt.Errorf("api request failed: %s - %s", resp.Status, string(respBody))
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
	}

	return nil
}
