package events

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SubscriptionClient handles UPnP GENA subscription requests.
type SubscriptionClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewSubscriptionClient creates a new subscription client.
func NewSubscriptionClient(timeout time.Duration) *SubscriptionClient {
	return &SubscriptionClient{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// Subscribe sends a SUBSCRIBE request to a Sonos device.
// Returns the subscription ID (SID) and timeout on success.
func (c *SubscriptionClient) Subscribe(ctx context.Context, deviceIP string, servicePath string, callbackURL string, timeout int) (sid string, actualTimeout int, err error) {
	url := fmt.Sprintf("http://%s:1400%s", deviceIP, servicePath)

	req, err := http.NewRequestWithContext(ctx, "SUBSCRIBE", url, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}

	// Set GENA headers
	req.Header.Set("CALLBACK", fmt.Sprintf("<%s>", callbackURL))
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", fmt.Sprintf("Second-%d", timeout))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("subscribe request: %w", err)
	}
	defer resp.Body.Close()

	// Drain and discard response body
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("subscribe failed: %s", resp.Status)
	}

	// Extract SID from response
	sid = ParseSID(resp.Header.Get("SID"))
	if sid == "" {
		return "", 0, fmt.Errorf("no SID in response")
	}

	actualTimeout = ParseTimeout(resp.Header.Get("TIMEOUT"))

	return sid, actualTimeout, nil
}

// Renew sends a subscription renewal request.
func (c *SubscriptionClient) Renew(ctx context.Context, deviceIP string, servicePath string, sid string, timeout int) (actualTimeout int, err error) {
	url := fmt.Sprintf("http://%s:1400%s", deviceIP, servicePath)

	req, err := http.NewRequestWithContext(ctx, "SUBSCRIBE", url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}

	// Set renewal headers (no CALLBACK or NT for renewals)
	req.Header.Set("SID", sid)
	req.Header.Set("TIMEOUT", fmt.Sprintf("Second-%d", timeout))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("renew request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusPreconditionFailed {
		// HTTP 412 means the subscription doesn't exist - need to resubscribe
		return 0, ErrSubscriptionNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("renew failed: %s", resp.Status)
	}

	actualTimeout = ParseTimeout(resp.Header.Get("TIMEOUT"))
	return actualTimeout, nil
}

// Unsubscribe sends an UNSUBSCRIBE request to a Sonos device.
func (c *SubscriptionClient) Unsubscribe(ctx context.Context, deviceIP string, servicePath string, sid string) error {
	url := fmt.Sprintf("http://%s:1400%s", deviceIP, servicePath)

	req, err := http.NewRequestWithContext(ctx, "UNSUBSCRIBE", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("SID", sid)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Don't fail on network errors - device may be offline
		return nil
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	// 412 means subscription already gone - that's fine
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unsubscribe failed: %s", resp.Status)
	}

	return nil
}

// ErrSubscriptionNotFound indicates the subscription doesn't exist (HTTP 412).
var ErrSubscriptionNotFound = fmt.Errorf("subscription not found")
