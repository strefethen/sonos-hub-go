package discovery

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"
)

// httpClient is a shared client with reasonable timeouts to prevent hanging on unreachable devices.
var httpClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second,
		IdleConnTimeout:     30 * time.Second,
	},
}

type RawDevice struct {
	UDN             string
	IP              string
	Model           string
	ModelNumber     string
	RoomName        string
	SerialNumber    string
	SoftwareVersion string
	HardwareVersion string
	SupportsAirPlay bool
	Location        string
	ZoneGroupState  string
	DiscoveredAt    time.Time
}

func ProbeDevice(ctx context.Context, ip string) (*RawDevice, error) {
	location := "http://" + ip + ":1400/xml/device_description.xml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	deviceInfo, err := ParseDeviceDescription(body)
	if err != nil || deviceInfo == nil {
		return nil, nil
	}

	roomName := deviceInfo.RoomName
	var zoneState string

	zoneURL := "http://" + ip + ":1400/status/zp"
	zoneReq, err := http.NewRequestWithContext(ctx, http.MethodGet, zoneURL, nil)
	if err == nil {
		if zoneResp, err := httpClient.Do(zoneReq); err == nil {
			defer zoneResp.Body.Close()
			if zoneResp.StatusCode < 300 {
				zoneBody, _ := io.ReadAll(zoneResp.Body)
				zoneState = string(zoneBody)
				if zoneInfo, _ := ParseZoneInfo(zoneBody); zoneInfo != nil && zoneInfo.RoomName != "" {
					roomName = zoneInfo.RoomName
				}
			}
		}
	}

	udn := deviceInfo.UDN
	if udn == "" {
		udn = "probe_" + ip
	}

	return &RawDevice{
		UDN:             udn,
		IP:              ip,
		Model:           deviceInfo.ModelName,
		ModelNumber:     deviceInfo.ModelNumber,
		RoomName:        roomName,
		SerialNumber:    deviceInfo.SerialNumber,
		SoftwareVersion: deviceInfo.SoftwareVersion,
		HardwareVersion: deviceInfo.HardwareVersion,
		SupportsAirPlay: deviceInfo.SupportsAirPlay,
		Location:        location,
		ZoneGroupState:  zoneState,
		DiscoveredAt:    time.Now(),
	}, nil
}
