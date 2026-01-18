//go:build integration

package phase1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/server"
)

type devicesResponse struct {
	Count   int `json:"count"`
	Devices []struct {
		DeviceID string `json:"device_id"`
		RoomName string `json:"room_name"`
	} `json:"devices"`
}

func TestPhase1Devices(t *testing.T) {
	t.Setenv("JWT_SECRET", "this-is-a-development-secret-string-32chars")
	t.Setenv("NODE_ENV", "development")
	t.Setenv("ALLOW_TEST_MODE", "true")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "sonos-hub.db")
	t.Setenv("SQLITE_DB_PATH", dbPath)

	cfg, err := config.Load()
	require.NoError(t, err)

	handler, shutdown, err := server.NewHandler(cfg, server.Options{DisableDiscovery: false})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, shutdown(nil))
	}()

	ts := httptest.NewServer(handler)
	defer ts.Close()

	testClient := &http.Client{Timeout: 20 * time.Second}
	require.NoError(t, triggerRescan(testClient, ts.URL))

	deviceID := os.Getenv("SONOS_TEST_DEVICE_ID")
	if deviceID == "" {
		deviceID = "Home Theater"
	}

	device := waitForDevice(t, testClient, ts.URL, deviceID, 60*time.Second)
	require.NotEmpty(t, device.DeviceID)
	require.Equal(t, deviceID, device.RoomName)

	resp, err := doRequest(testClient, http.MethodGet, ts.URL+"/v1/devices/"+device.DeviceID, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func triggerRescan(client *http.Client, baseURL string) error {
	resp, err := doRequest(client, http.MethodPost, baseURL+"/v1/devices/rescan", map[string]any{})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &httpError{StatusCode: resp.StatusCode}
	}
	return nil
}

func waitForDevice(t *testing.T, client *http.Client, baseURL string, deviceID string, timeout time.Duration) struct {
	DeviceID string
	RoomName string
} {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := doRequest(client, http.MethodGet, baseURL+"/v1/devices", nil)
		require.NoError(t, err)
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		var payload devicesResponse
		err = json.NewDecoder(resp.Body).Decode(&payload)
		resp.Body.Close()
		require.NoError(t, err)

		for _, device := range payload.Devices {
			if device.RoomName == deviceID || device.DeviceID == deviceID {
				return struct {
					DeviceID string
					RoomName string
				}{DeviceID: device.DeviceID, RoomName: device.RoomName}
			}
		}

		time.Sleep(2 * time.Second)
	}

	t.Fatalf("device %q not discovered within timeout", deviceID)
	return struct {
		DeviceID string
		RoomName string
	}{}
}

func doRequest(client *http.Client, method, url string, body any) (*http.Response, error) {
	var buf *bytes.Buffer
	if body != nil {
		payload, _ := json.Marshal(body)
		buf = bytes.NewBuffer(payload)
	} else {
		buf = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Mode", "true")
	return client.Do(req)
}

type httpError struct {
	StatusCode int
}

func (err *httpError) Error() string {
	return "unexpected status code"
}
