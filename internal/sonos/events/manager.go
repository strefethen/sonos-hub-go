package events

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos"
)

// Manager orchestrates UPnP event subscriptions for Sonos devices.
// It handles subscription lifecycle, renewal, and event processing.
type Manager struct {
	config         ManagerConfig
	subClient      *SubscriptionClient
	stateCache     *StateCache
	zoneCache      *sonos.ZoneGroupCache

	mu             sync.RWMutex
	subscriptions  map[string]*Subscription // keyed by SID
	deviceSubs     map[string][]string      // device IP -> list of SIDs
	callbackURL    string
	localIP        string
	port           int

	// Track subscription state per device for idempotency and backoff
	subscribedDevices map[string]*DeviceSubscriptionState // device IP -> state

	stopCh         chan struct{}
	stopped        bool
	stats          ManagerStats

	// Time function for testing
	now func() time.Time
}

// NewManager creates a new event subscription manager.
func NewManager(config ManagerConfig, port int, zoneCache *sonos.ZoneGroupCache) *Manager {
	return &Manager{
		config:            config,
		subClient:         NewSubscriptionClient(10 * time.Second),
		stateCache:        NewStateCache(config.StateCacheTTL),
		zoneCache:         zoneCache,
		subscriptions:     make(map[string]*Subscription),
		deviceSubs:        make(map[string][]string),
		subscribedDevices: make(map[string]*DeviceSubscriptionState),
		port:              port,
		stopCh:            make(chan struct{}),
		stats: ManagerStats{
			Enabled: config.Enabled,
		},
		now: time.Now,
	}
}

// Start begins the event subscription manager.
// It discovers the local IP and starts the renewal loop.
func (m *Manager) Start() error {
	if !m.config.Enabled {
		log.Printf("UPNP: Event subscriptions disabled")
		return nil
	}

	// Discover local IP for callback URL
	localIP, err := m.discoverLocalIP()
	if err != nil {
		return fmt.Errorf("discover local IP: %w", err)
	}
	m.localIP = localIP

	// Build callback URL
	port := m.port
	if m.config.CallbackPort > 0 {
		port = m.config.CallbackPort
	}
	m.callbackURL = fmt.Sprintf("http://%s:%d/upnp/notify", m.localIP, port)

	log.Printf("UPNP: Event manager started, callback URL: %s", m.callbackURL)

	// Start renewal goroutine
	go m.renewalLoop()

	return nil
}

// Stop gracefully shuts down the manager.
// It unsubscribes from all devices and stops the renewal loop.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.stopped = true
	close(m.stopCh)
	m.mu.Unlock()

	// Unsubscribe from all devices
	m.unsubscribeAll(ctx)

	log.Printf("UPNP: Event manager stopped")
	return nil
}

// SubscribeDevice subscribes to events from a Sonos device.
// It subscribes to all configured services. This method is idempotent - calling it
// for an already-subscribed device is a no-op for services that are already subscribed.
func (m *Manager) SubscribeDevice(ctx context.Context, deviceIP, deviceUDN string) error {
	if !m.config.Enabled {
		return nil
	}

	// Check if already fully subscribed (fast path, avoids goroutine work)
	if m.IsDeviceFullySubscribed(deviceIP) {
		return nil
	}

	// Check backoff before attempting subscription
	if !m.shouldAttemptSubscription(deviceIP) {
		return nil
	}

	// Get or create device state
	m.mu.Lock()
	state := m.subscribedDevices[deviceIP]
	if state == nil {
		state = &DeviceSubscriptionState{
			DeviceIP:     deviceIP,
			DeviceUDN:    deviceUDN,
			Services:     make(map[ServiceType]string),
			SubscribedAt: m.now(),
		}
		m.subscribedDevices[deviceIP] = state
	}
	state.LastAttemptAt = m.now()
	// Copy existing services to check which ones need subscription
	existingServices := make(map[ServiceType]string)
	for k, v := range state.Services {
		existingServices[k] = v
	}
	m.mu.Unlock()

	paths := EventPaths()
	failureCount := 0
	successCount := 0

	for _, serviceType := range m.config.Services {
		// Skip if already subscribed to this service
		if _, ok := existingServices[serviceType]; ok {
			continue
		}

		path, ok := paths[serviceType]
		if !ok {
			continue
		}

		// Build service-specific callback URL
		callbackURL := m.buildCallbackURL(serviceType)

		sid, timeout, err := m.subClient.Subscribe(ctx, deviceIP, path, callbackURL, m.config.SubscriptionTimeout)
		if err != nil {
			log.Printf("UPNP: Failed to subscribe %s on %s: %v", serviceType, deviceIP, err)
			m.mu.Lock()
			m.stats.SubscriptionFailures++
			m.mu.Unlock()
			failureCount++
			continue
		}

		// Calculate renewal time, ensuring it's at least 60 seconds from now
		// to avoid immediate renewal loops from bad timeout values
		renewIn := timeout - m.config.RenewalBuffer
		if renewIn < 60 {
			renewIn = 60
		}

		sub := &Subscription{
			SID:          sid,
			DeviceIP:     deviceIP,
			DeviceUDN:    deviceUDN,
			ServiceType:  serviceType,
			CallbackURL:  callbackURL,
			Timeout:      timeout,
			SubscribedAt: m.now(),
			RenewAt:      m.now().Add(time.Duration(renewIn) * time.Second),
		}

		m.addSubscription(sub)

		// Update device state with the new service subscription
		m.mu.Lock()
		if m.subscribedDevices[deviceIP] != nil {
			m.subscribedDevices[deviceIP].Services[serviceType] = sid
			m.subscribedDevices[deviceIP].FailureCount = 0 // Reset on success
		}
		m.mu.Unlock()

		successCount++
		log.Printf("UPNP: Subscribed to %s on %s (SID: %s, timeout: %ds)", serviceType, deviceIP, sid, timeout)
	}

	// Update failure count for backoff
	if failureCount > 0 && successCount == 0 {
		m.mu.Lock()
		if m.subscribedDevices[deviceIP] != nil {
			m.subscribedDevices[deviceIP].FailureCount++
		}
		m.mu.Unlock()
	}

	return nil
}

// UnsubscribeDevice removes all subscriptions for a device.
func (m *Manager) UnsubscribeDevice(ctx context.Context, deviceIP string) {
	m.mu.Lock()
	sids := m.deviceSubs[deviceIP]
	m.mu.Unlock()

	paths := EventPaths()
	for _, sid := range sids {
		sub := m.findSubscriptionBySID(sid)
		if sub == nil {
			continue
		}

		path, ok := paths[sub.ServiceType]
		if !ok {
			continue
		}

		if err := m.subClient.Unsubscribe(ctx, deviceIP, path, sid); err != nil {
			log.Printf("UPNP: Failed to unsubscribe %s: %v", sid, err)
		}

		m.removeSubscription(sid)
	}
}

// GetStateCache returns the state cache for reading device states.
func (m *Manager) GetStateCache() *StateCache {
	return m.stateCache
}

// Stats returns manager statistics.
func (m *Manager) Stats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := m.stats
	stats.ActiveSubscriptions = len(m.subscriptions)
	stats.TotalDevices = len(m.deviceSubs)

	if m.stateCache != nil {
		hits, misses, _ := m.stateCache.Stats()
		stats.CacheHits = hits
		stats.CacheMisses = misses
	}

	return stats
}

// buildCallbackURL builds a service-specific callback URL.
func (m *Manager) buildCallbackURL(serviceType ServiceType) string {
	suffix := ""
	switch serviceType {
	case ServiceAVTransport:
		suffix = "/avtransport"
	case ServiceRenderingControl:
		suffix = "/renderingcontrol"
	case ServiceZoneGroupTopology:
		suffix = "/topology"
	}
	return m.callbackURL + suffix
}

// addSubscription adds a subscription to the manager's tracking.
func (m *Manager) addSubscription(sub *Subscription) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.subscriptions[sub.SID] = sub

	if _, ok := m.deviceSubs[sub.DeviceIP]; !ok {
		m.deviceSubs[sub.DeviceIP] = make([]string, 0)
	}
	m.deviceSubs[sub.DeviceIP] = append(m.deviceSubs[sub.DeviceIP], sub.SID)
}

// removeSubscription removes a subscription from tracking.
// It also updates subscribedDevices state to maintain accurate service tracking.
func (m *Manager) removeSubscription(sid string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptions[sid]
	if !ok {
		return
	}

	delete(m.subscriptions, sid)

	// Remove from device subs list
	if sids, ok := m.deviceSubs[sub.DeviceIP]; ok {
		for i, s := range sids {
			if s == sid {
				m.deviceSubs[sub.DeviceIP] = append(sids[:i], sids[i+1:]...)
				break
			}
		}
		if len(m.deviceSubs[sub.DeviceIP]) == 0 {
			delete(m.deviceSubs, sub.DeviceIP)
		}
	}

	// Update subscribedDevices state - remove the service mapping for this SID
	if state, ok := m.subscribedDevices[sub.DeviceIP]; ok {
		for svc, storedSID := range state.Services {
			if storedSID == sid {
				delete(state.Services, svc)
				break
			}
		}
		// If no services left, remove the device state entirely
		if len(state.Services) == 0 {
			delete(m.subscribedDevices, sub.DeviceIP)
		}
	}
}

// findSubscriptionBySID finds a subscription by its SID.
func (m *Manager) findSubscriptionBySID(sid string) *Subscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subscriptions[sid]
}

// updateSubscriptionSEQ updates the sequence number for a subscription.
func (m *Manager) updateSubscriptionSEQ(sid string, seq int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sub, ok := m.subscriptions[sid]; ok {
		sub.SEQ = seq
	}
}

// renewalLoop periodically checks and renews expiring subscriptions.
func (m *Manager) renewalLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.renewExpiring()
		case <-m.stopCh:
			return
		}
	}
}

// renewExpiring renews subscriptions that are about to expire.
func (m *Manager) renewExpiring() {
	m.mu.RLock()
	var toRenew []*Subscription
	for _, sub := range m.subscriptions {
		if sub.IsExpiringSoon() {
			toRenew = append(toRenew, sub)
		}
	}
	m.mu.RUnlock()

	paths := EventPaths()
	for _, sub := range toRenew {
		path, ok := paths[sub.ServiceType]
		if !ok {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		timeout, err := m.subClient.Renew(ctx, sub.DeviceIP, path, sub.SID, m.config.SubscriptionTimeout)
		cancel()

		if err == ErrSubscriptionNotFound {
			// Subscription expired, need to resubscribe
			log.Printf("UPNP: Subscription expired, resubscribing: %s", sub.SID)
			m.removeSubscription(sub.SID)
			m.SubscribeDevice(context.Background(), sub.DeviceIP, sub.DeviceUDN)
			continue
		}

		if err != nil {
			log.Printf("UPNP: Failed to renew %s: %v", sub.SID, err)
			m.mu.Lock()
			m.stats.RenewalFailures++
			m.mu.Unlock()
			continue
		}

		// Calculate renewal time with defensive minimum
		renewIn := timeout - m.config.RenewalBuffer
		if renewIn < 60 {
			renewIn = 60
		}

		m.mu.Lock()
		sub.Timeout = timeout
		sub.RenewAt = m.now().Add(time.Duration(renewIn) * time.Second)
		m.mu.Unlock()

		log.Printf("UPNP: Renewed subscription %s (timeout: %ds)", sub.SID, timeout)
	}
}

// unsubscribeAll unsubscribes from all devices.
func (m *Manager) unsubscribeAll(ctx context.Context) {
	m.mu.RLock()
	sids := make([]string, 0, len(m.subscriptions))
	for sid := range m.subscriptions {
		sids = append(sids, sid)
	}
	m.mu.RUnlock()

	paths := EventPaths()
	for _, sid := range sids {
		sub := m.findSubscriptionBySID(sid)
		if sub == nil {
			continue
		}

		path, ok := paths[sub.ServiceType]
		if !ok {
			continue
		}

		m.subClient.Unsubscribe(ctx, sub.DeviceIP, path, sid)
		m.removeSubscription(sid)
	}
}

// discoverLocalIP discovers the local IP address to use in callback URLs.
// It connects to a well-known address to determine which interface to use.
func (m *Manager) discoverLocalIP() (string, error) {
	// Connect to a well-known address (doesn't actually send data)
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// IsEnabled returns whether the event manager is enabled.
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// GetCallbackURL returns the callback URL for registrations.
func (m *Manager) GetCallbackURL() string {
	return m.callbackURL
}

// IsDeviceFullySubscribed returns true if the device has active subscriptions for all required services.
// This is used by the discovery callback to avoid spawning goroutines for already-subscribed devices.
func (m *Manager) IsDeviceFullySubscribed(deviceIP string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.subscribedDevices[deviceIP]
	if !exists {
		return false
	}
	return state.IsFullySubscribed(m.config.Services)
}

// shouldAttemptSubscription returns true if we should attempt to subscribe to the device.
// It implements exponential backoff for failed devices.
func (m *Manager) shouldAttemptSubscription(deviceIP string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.subscribedDevices[deviceIP]
	if !exists {
		return true // No state, go ahead
	}

	if state.FailureCount == 0 {
		return true // No failures, go ahead
	}

	// Exponential backoff: 30s, 60s, 120s, 240s, 480s, 600s max
	backoffSeconds := 30 * (1 << state.FailureCount)
	if backoffSeconds > 600 {
		backoffSeconds = 600
	}
	return m.now().Sub(state.LastAttemptAt) > time.Duration(backoffSeconds)*time.Second
}
