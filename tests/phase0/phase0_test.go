package phase0

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/server"
)

type healthResponse struct {
	Status string `json:"status"`
}

type pairStartResponse struct {
	RequestID string         `json:"request_id"`
	Result    map[string]any `json:"result"`
}

type pairCompleteResponse struct {
	RequestID string `json:"request_id"`
	Tokens    struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresInSec int    `json:"expires_in_sec"`
	} `json:"tokens"`
}

type refreshResponse struct {
	RequestID string `json:"request_id"`
	Tokens    struct {
		AccessToken  string `json:"access_token"`
		ExpiresInSec int    `json:"expires_in_sec"`
	} `json:"tokens"`
}

func TestPhase0HealthAndAuth(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-development-secret-string-32chars")
	t.Setenv("NODE_ENV", "development")
	t.Setenv("ALLOW_TEST_MODE", "true")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sonos-hub.db")
	t.Setenv("SQLITE_DB_PATH", dbPath)

	cfg, err := config.Load()
	require.NoError(t, err)

	handler, shutdown, err := server.NewHandler(cfg, server.Options{DisableDiscovery: true})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, shutdown(nil))
	}()

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/v1/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var health healthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	require.Equal(t, "healthy", health.Status)
	require.NoError(t, resp.Body.Close())

	startPayload := map[string]any{}
	startBody, _ := json.Marshal(startPayload)
	startResp, err := http.Post(server.URL+"/v1/auth/pair/start", "application/json", bytes.NewReader(startBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, startResp.StatusCode)

	var start pairStartResponse
	require.NoError(t, json.NewDecoder(startResp.Body).Decode(&start))
	pairingHint := start.Result["pairing_hint"].(string)
	require.NotEmpty(t, pairingHint)
	require.NoError(t, startResp.Body.Close())

	code := extractPairingCode(t, pairingHint)

	completePayload := map[string]any{
		"pair_code":   code,
		"device_name": "Test Device",
	}
	completeBody, _ := json.Marshal(completePayload)
	completeResp, err := http.Post(server.URL+"/v1/auth/pair/complete", "application/json", bytes.NewReader(completeBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, completeResp.StatusCode)

	var complete pairCompleteResponse
	require.NoError(t, json.NewDecoder(completeResp.Body).Decode(&complete))
	require.NotEmpty(t, complete.Tokens.AccessToken)
	require.NotEmpty(t, complete.Tokens.RefreshToken)
	require.NoError(t, completeResp.Body.Close())

	refreshPayload := map[string]any{
		"refresh_token": complete.Tokens.RefreshToken,
	}
	refreshBody, _ := json.Marshal(refreshPayload)
	refreshResp, err := http.Post(server.URL+"/v1/auth/refresh", "application/json", bytes.NewReader(refreshBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, refreshResp.StatusCode)

	var refresh refreshResponse
	require.NoError(t, json.NewDecoder(refreshResp.Body).Decode(&refresh))
	require.NotEmpty(t, refresh.Tokens.AccessToken)
	require.NoError(t, refreshResp.Body.Close())
}

func extractPairingCode(t *testing.T, hint string) string {
	t.Helper()
	re := regexp.MustCompile(`Code:\s*([0-9]{6})`)
	match := re.FindStringSubmatch(hint)
	require.Len(t, match, 2)
	return match[1]
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
