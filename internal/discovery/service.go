package discovery

import (
	"context"
	"log"
	"net/url"
	"strings"
	"time"
)

// DiscoverDevices performs multi-pass SSDP discovery and optional fallback probes.
func DiscoverDevices(ctx context.Context, passes int, passInterval, timeout time.Duration, knownIPs []string) ([]*RawDevice, error) {
	log.Printf("Starting discovery with %d known IPs: %v", len(knownIPs), knownIPs)

	responses, err := Discover(ctx, passes, passInterval, timeout)
	if err != nil {
		log.Printf("SSDP discovery error: %v", err)
		return nil, err
	}
	log.Printf("SSDP returned %d responses", len(responses))

	devices := make([]*RawDevice, 0)
	seenIPs := make(map[string]struct{})

	// Probe SSDP-discovered devices
	for _, resp := range responses {
		loc := resp.Location
		ip := extractHost(loc)
		if ip == "" {
			continue
		}
		seenIPs[ip] = struct{}{}

		// Use a fresh context for each probe to avoid timeout propagation
		probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		device, err := ProbeDevice(probeCtx, ip)
		cancel()

		if err != nil {
			log.Printf("SSDP probe failed for %s: %v", ip, err)
			continue
		}
		if device == nil {
			log.Printf("SSDP probe returned nil for %s", ip)
			continue
		}
		device.Location = loc
		devices = append(devices, device)
		log.Printf("SSDP discovered device: %s (%s)", device.RoomName, ip)
	}

	// Probe known IPs that weren't discovered via SSDP
	log.Printf("Probing %d known IPs not found via SSDP", len(knownIPs)-len(seenIPs))
	for _, ip := range knownIPs {
		if _, ok := seenIPs[ip]; ok {
			log.Printf("Skipping %s - already discovered via SSDP", ip)
			continue
		}

		// Use a fresh context for each probe
		probeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		device, err := ProbeDevice(probeCtx, ip)
		cancel()

		if err != nil {
			log.Printf("Fallback probe failed for %s: %v", ip, err)
			continue
		}
		if device == nil {
			log.Printf("Fallback probe returned nil for %s", ip)
			continue
		}
		devices = append(devices, device)
		log.Printf("Fallback discovered device: %s (%s)", device.RoomName, ip)
	}

	log.Printf("Discovery complete: %d devices found", len(devices))
	return devices, nil
}

func extractHost(location string) string {
	if location == "" {
		return ""
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	return strings.TrimSpace(host)
}
