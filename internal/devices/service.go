package devices

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
	"github.com/strefethen/sonos-hub-go/internal/discovery"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

const staleIPThreshold = 7 * 24 * time.Hour

var errNoTopology = errors.New("no topology available")

type discoveryResult struct {
	devices    int
	durationMs int64
	err        error
}

type Service struct {
	cfg                config.Config
	logger             *log.Logger
	soapClient         *soap.Client
	topologyMu         sync.RWMutex
	topology           *DeviceTopology
	lastDiscoveryError error
	testMode           bool // Skip SSDP discovery in test mode

	knownIPsMu sync.Mutex
	knownIPs   map[string]time.Time

	discoveryMu       sync.Mutex
	discoveryInFlight bool
	discoveryWaiters  []chan discoveryResult

	periodicMu     sync.Mutex
	periodicCancel context.CancelFunc
}

func NewService(cfg config.Config, logger *log.Logger, soapClient *soap.Client) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{
		cfg:        cfg,
		logger:     logger,
		soapClient: soapClient,
		knownIPs:   make(map[string]time.Time),
	}
}

// SetTestMode enables or disables test mode. In test mode, SSDP discovery is skipped
// and empty results are returned to avoid blocking on network operations.
func (service *Service) SetTestMode(enabled bool) {
	service.testMode = enabled
}

// IsTestMode returns whether the service is in test mode.
func (service *Service) IsTestMode() bool {
	return service.testMode
}

func (service *Service) GetDevices() ([]LogicalDevice, error) {
	service.topologyMu.RLock()
	if service.topology != nil {
		devices := GetTargetableDevices(*service.topology)
		service.topologyMu.RUnlock()
		return devices, nil
	}
	service.topologyMu.RUnlock()

	// In test mode, skip discovery and return empty list
	if service.testMode {
		return []LogicalDevice{}, nil
	}

	if _, err := service.performDiscovery(); err != nil {
		return []LogicalDevice{}, err
	}

	service.topologyMu.RLock()
	defer service.topologyMu.RUnlock()
	if service.topology == nil {
		return []LogicalDevice{}, nil
	}
	return GetTargetableDevices(*service.topology), nil
}

func (service *Service) GetDevice(deviceID string) (*LogicalDevice, error) {
	service.topologyMu.RLock()
	if service.topology != nil {
		device := findDevice(service.topology.Devices, service.logger, deviceID)
		service.topologyMu.RUnlock()
		return device, nil
	}
	service.topologyMu.RUnlock()

	return nil, nil
}

func (service *Service) GetTopology() (DeviceTopology, error) {
	service.topologyMu.RLock()
	if service.topology != nil {
		topology := *service.topology
		service.topologyMu.RUnlock()
		return topology, nil
	}
	service.topologyMu.RUnlock()

	// In test mode, skip discovery and return empty topology
	if service.testMode {
		return DeviceTopology{Devices: []LogicalDevice{}}, nil
	}

	if _, err := service.performDiscovery(); err != nil {
		return DeviceTopology{}, err
	}

	service.topologyMu.RLock()
	defer service.topologyMu.RUnlock()
	if service.topology == nil {
		return DeviceTopology{}, errNoTopology
	}
	return *service.topology, nil
}

// GetTopologyIfCached returns the cached topology without triggering discovery.
// Returns nil if no topology is cached yet.
// This matches Node.js behavior where device lookup failure continues without room names.
func (service *Service) GetTopologyIfCached() *DeviceTopology {
	service.topologyMu.RLock()
	defer service.topologyMu.RUnlock()
	return service.topology
}

func (service *Service) Rescan() (int, int64, error) {
	result, err := service.performDiscovery()
	return result.devices, result.durationMs, err
}

func (service *Service) StartPeriodicDiscovery() {
	service.periodicMu.Lock()
	defer service.periodicMu.Unlock()

	if service.periodicCancel != nil {
		return
	}

	if service.cfg.SSDPRescanIntervalMs <= 0 {
		service.logger.Print("Periodic discovery disabled")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	service.periodicCancel = cancel

	service.logger.Printf("Starting periodic discovery interval=%dms", service.cfg.SSDPRescanIntervalMs)

	go func() {
		ticker := time.NewTicker(time.Duration(service.cfg.SSDPRescanIntervalMs) * time.Millisecond)
		defer ticker.Stop()

		if _, err := service.performDiscovery(); err != nil {
			service.logger.Printf("Initial discovery failed: %v", err)
		}

		for {
			select {
			case <-ticker.C:
				if _, err := service.performDiscovery(); err != nil {
					service.logger.Printf("Periodic discovery failed: %v", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (service *Service) StopPeriodicDiscovery() {
	service.periodicMu.Lock()
	defer service.periodicMu.Unlock()
	if service.periodicCancel != nil {
		service.periodicCancel()
		service.periodicCancel = nil
	}
}

func (service *Service) IsHealthy() bool {
	service.topologyMu.RLock()
	defer service.topologyMu.RUnlock()
	if service.lastDiscoveryError != nil && service.topology == nil {
		return false
	}
	return true
}

func (service *Service) ResolveDeviceIP(deviceID string) (string, error) {
	device, err := service.GetDevice(deviceID)
	if err != nil {
		return "", err
	}
	if device != nil {
		return device.IP, nil
	}

	service.logger.Printf("Device not found in topology, triggering rescan: %s", deviceID)
	if _, err := service.performDiscovery(); err != nil {
		service.logger.Printf("Rescan failed while resolving device: %v", err)
		return "", err
	}

	device, err = service.GetDevice(deviceID)
	if err != nil {
		return "", err
	}
	if device != nil {
		service.logger.Printf("Device resolved after rescan: %s -> %s", deviceID, device.IP)
		return device.IP, nil
	}
	return "", nil
}

func (service *Service) performDiscovery() (discoveryResult, error) {
	service.discoveryMu.Lock()
	if service.discoveryInFlight {
		ch := make(chan discoveryResult, 1)
		service.discoveryWaiters = append(service.discoveryWaiters, ch)
		service.discoveryMu.Unlock()
		result := <-ch
		return result, result.err
	}
	service.discoveryInFlight = true
	service.discoveryMu.Unlock()

	result := service.runDiscovery()

	service.discoveryMu.Lock()
	waiters := service.discoveryWaiters
	service.discoveryWaiters = nil
	service.discoveryInFlight = false
	service.discoveryMu.Unlock()

	for _, ch := range waiters {
		ch <- result
		close(ch)
	}

	return result, result.err
}

func (service *Service) runDiscovery() discoveryResult {
	start := time.Now()
	service.lastDiscoveryError = nil

	knownIPs := service.loadKnownIPs()
	allKnown := append([]string{}, service.cfg.StaticDeviceIPs...)
	allKnown = append(allKnown, knownIPs...)
	allKnown = dedupeStrings(allKnown)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(service.cfg.SSDPDiscoveryTimeoutMs)*time.Millisecond)
	defer cancel()

	rawDevices, err := discovery.DiscoverDevices(ctx, service.cfg.SSDPDiscoveryPasses, time.Duration(service.cfg.SSDPPassIntervalMs)*time.Millisecond, time.Duration(service.cfg.SSDPDiscoveryTimeoutMs)*time.Millisecond, allKnown)
	if err != nil {
		service.lastDiscoveryError = err
		return discoveryResult{err: err}
	}

	devices := make([]RawSonosDevice, 0, len(rawDevices))
	for _, raw := range rawDevices {
		if raw == nil {
			continue
		}
		devices = append(devices, RawSonosDevice{
			UDN:             raw.UDN,
			IP:              raw.IP,
			Model:           raw.Model,
			ModelNumber:     raw.ModelNumber,
			RoomName:        raw.RoomName,
			SerialNumber:    raw.SerialNumber,
			SoftwareVersion: raw.SoftwareVersion,
			HardwareVersion: raw.HardwareVersion,
			SupportsAirPlay: raw.SupportsAirPlay,
			ZoneGroupState:  raw.ZoneGroupState,
			DiscoveredAt:    raw.DiscoveredAt,
			Location:        raw.Location,
		})
	}

	if len(devices) == 0 {
		return discoveryResult{
			devices:    0,
			durationMs: time.Since(start).Milliseconds(),
		}
	}

	service.saveKnownIPs(devices)
	service.pruneStaleIPs()

	var zoneTopology *ZoneGroupTopology
	firstDevice := devices[0]
	if firstDevice.IP != "" {
		zoneTopology = service.fetchZoneGroupTopology(firstDevice.IP)
	}

	newTopology := NormalizeDevices(devices, zoneTopology)

	service.topologyMu.Lock()
	merged := mergeTopologies(newTopology, service.topology)
	service.topology = &merged
	service.topologyMu.Unlock()

	return discoveryResult{
		devices:    len(service.topology.Devices),
		durationMs: time.Since(start).Milliseconds(),
	}
}

func (service *Service) fetchZoneGroupTopology(ip string) *ZoneGroupTopology {
	service.logger.Printf("[TOPOLOGY-DIAG] Fetching zone group topology from IP=%s, timeout=%dms", ip, service.cfg.SonosTimeoutMs)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(service.cfg.SonosTimeoutMs)*time.Millisecond)
	defer cancel()

	state, err := service.soapClient.GetZoneGroupState(ctx, ip)
	if err != nil {
		service.logger.Printf("[TOPOLOGY-DIAG] FAIL: GetZoneGroupState error: %v", err)
		return nil
	}

	service.logger.Printf("[TOPOLOGY-DIAG] Got zone state, %d groups", len(state.Groups))
	for i, group := range state.Groups {
		service.logger.Printf("[TOPOLOGY-DIAG]   Group[%d]: CoordinatorUDN=%s, %d members",
			i, group.Coordinator, len(group.Members))
		for j, member := range group.Members {
			service.logger.Printf("[TOPOLOGY-DIAG]     Member[%d]: UUID=%s, ZoneName=%s, ChannelMapSet=%q, Satellite=%v, SW=%v",
				j, member.UUID, member.ZoneName, member.ChannelMapSet, member.IsSatellite, member.IsSubwoofer)
		}
	}

	topology := convertZoneGroupState(state)
	if topology == nil {
		service.logger.Printf("[TOPOLOGY-DIAG] FAIL: convertZoneGroupState returned nil")
		return nil
	}
	service.logger.Printf("[TOPOLOGY-DIAG] Converted topology: %d groups", len(topology.Groups))
	return topology
}

func (service *Service) loadKnownIPs() []string {
	service.knownIPsMu.Lock()
	defer service.knownIPsMu.Unlock()

	cutoff := time.Now().Add(-staleIPThreshold)
	ips := make([]string, 0, len(service.knownIPs))
	for ip, seenAt := range service.knownIPs {
		if seenAt.After(cutoff) {
			ips = append(ips, ip)
		}
	}
	return ips
}

func (service *Service) saveKnownIPs(devices []RawSonosDevice) {
	service.knownIPsMu.Lock()
	defer service.knownIPsMu.Unlock()
	now := time.Now()
	for _, device := range devices {
		if device.IP != "" {
			service.knownIPs[device.IP] = now
		}
	}
}

func (service *Service) pruneStaleIPs() {
	service.knownIPsMu.Lock()
	defer service.knownIPsMu.Unlock()
	cutoff := time.Now().Add(-staleIPThreshold)
	for ip, seenAt := range service.knownIPs {
		if seenAt.Before(cutoff) {
			delete(service.knownIPs, ip)
		}
	}
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		val := strings.TrimSpace(value)
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		result = append(result, val)
	}
	return result
}
