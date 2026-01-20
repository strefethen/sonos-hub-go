package events

import (
	"log"
	"sync"
	"time"
)

// StateCache provides thread-safe caching of device playback states.
// States are updated from UPnP events and read by API handlers.
type StateCache struct {
	mu     sync.RWMutex
	states map[string]*DeviceState // keyed by device IP
	ttl    time.Duration

	// Statistics
	hits   int64
	misses int64
}

// NewStateCache creates a new state cache with the given TTL.
func NewStateCache(ttl time.Duration) *StateCache {
	return &StateCache{
		states: make(map[string]*DeviceState),
		ttl:    ttl,
	}
}

// Get returns the device state for the given IP if it exists and is fresh.
// Returns nil if not found or stale.
func (c *StateCache) Get(deviceIP string) *DeviceState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, ok := c.states[deviceIP]
	if !ok {
		// Debug: list what IPs ARE in the cache
		ips := make([]string, 0, len(c.states))
		for ip := range c.states {
			ips = append(ips, ip)
		}
		log.Printf("CACHE: Miss for %s, cache has: %v", deviceIP, ips)
		c.misses++
		return nil
	}

	if !state.IsFresh(c.ttl) {
		log.Printf("CACHE: Stale data for %s (age: %v, ttl: %v)", deviceIP, time.Since(state.UpdatedAt), c.ttl)
		c.misses++
		return nil
	}

	c.hits++
	// Return a copy to prevent races
	stateCopy := *state
	return &stateCopy
}

// GetByUDN returns the device state for the given UDN if it exists and is fresh.
func (c *StateCache) GetByUDN(udn string) *DeviceState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, state := range c.states {
		if state.DeviceUDN == udn && state.IsFresh(c.ttl) {
			c.hits++
			stateCopy := *state
			return &stateCopy
		}
	}

	c.misses++
	return nil
}

// Set stores or updates the device state.
func (c *StateCache) Set(deviceIP string, state *DeviceState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state.UpdatedAt = time.Now()
	c.states[deviceIP] = state
}

// UpdateTransport updates transport-related fields for a device.
func (c *StateCache) UpdateTransport(deviceIP string, event *AVTransportEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.states[deviceIP]
	if !ok {
		state = &DeviceState{
			DeviceIP: deviceIP,
			Source:   "upnp_event",
		}
		c.states[deviceIP] = state
	}

	now := time.Now()
	hasTransportState := false // Track if we got meaningful transport state data

	if event.TransportState != "" {
		state.TransportState = event.TransportState
		hasTransportState = true
	}
	if event.TransportStatus != "" {
		state.TransportStatus = event.TransportStatus
	}
	if event.CurrentTrackURI != "" {
		state.CurrentTrackURI = event.CurrentTrackURI
	}
	if event.CurrentTrackMetaData != "" {
		state.CurrentTrackMetaData = event.CurrentTrackMetaData
	}
	if event.TrackDuration != "" {
		state.TrackDuration = event.TrackDuration
	}
	if event.RelTime != "" {
		state.RelativeTime = event.RelTime
	}
	if event.AVTransportURI != "" {
		state.AVTransportURI = event.AVTransportURI
	}
	if event.AVTransportURIMetaData != "" {
		state.CurrentURIMetaData = event.AVTransportURIMetaData
	}

	state.TransportUpdatedAt = now
	// Only update main freshness timestamp if we got transport state.
	// This prevents position-only updates from masking stale/empty transport state.
	if hasTransportState {
		state.UpdatedAt = now
	}
	state.Source = "upnp_event"
}

// UpdateVolume updates volume-related fields for a device.
func (c *StateCache) UpdateVolume(deviceIP string, event *RenderingControlEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.states[deviceIP]
	if !ok {
		state = &DeviceState{
			DeviceIP: deviceIP,
			Source:   "upnp_event",
		}
		c.states[deviceIP] = state
	}

	now := time.Now()
	state.Volume = event.Volume
	state.Muted = event.Muted
	state.VolumeUpdatedAt = now
	state.UpdatedAt = now
	state.Source = "upnp_event"
}

// SetUDN associates a UDN with a device IP.
func (c *StateCache) SetUDN(deviceIP, udn string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.states[deviceIP]
	if !ok {
		state = &DeviceState{
			DeviceIP: deviceIP,
			Source:   "upnp_event",
		}
		c.states[deviceIP] = state
	}
	state.DeviceUDN = udn
}

// Remove removes a device from the cache.
func (c *StateCache) Remove(deviceIP string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, deviceIP)
}

// Clear removes all entries from the cache.
func (c *StateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = make(map[string]*DeviceState)
}

// List returns all cached states (for debugging/monitoring).
func (c *StateCache) List() []*DeviceState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*DeviceState, 0, len(c.states))
	for _, state := range c.states {
		stateCopy := *state
		result = append(result, &stateCopy)
	}
	return result
}

// Stats returns cache statistics.
func (c *StateCache) Stats() (hits, misses int64, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, len(c.states)
}

// Prune removes stale entries from the cache.
func (c *StateCache) Prune() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	for ip, state := range c.states {
		if !state.IsFresh(c.ttl) {
			delete(c.states, ip)
			pruned++
		}
	}
	return pruned
}

// CloudPlaybackEvent represents playback data from a Sonos Cloud webhook.
type CloudPlaybackEvent struct {
	PlaybackState string // PLAYING, PAUSED, STOPPED
	Volume        *int   // 0-100, nil if not included
	Muted         *bool  // nil if not included
}

// CloudMetadataEvent represents metadata from a Sonos Cloud webhook.
type CloudMetadataEvent struct {
	TrackName    string
	ArtistName   string
	AlbumName    string
	AlbumArtURI  string
	ContainerURI string
}

// UpdateFromCloud updates the state cache from a Sonos Cloud webhook event.
// Since Cloud webhooks identify groups by groupId (not IP), the caller must
// resolve the groupId to a device IP before calling this method.
func (c *StateCache) UpdateFromCloud(deviceIP string, playback *CloudPlaybackEvent, metadata *CloudMetadataEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, ok := c.states[deviceIP]
	if !ok {
		state = &DeviceState{
			DeviceIP: deviceIP,
			Source:   "cloud_webhook",
		}
		c.states[deviceIP] = state
	}

	now := time.Now()

	if playback != nil {
		// Map Cloud API playback states to UPnP-style states
		switch playback.PlaybackState {
		case "PLAYING":
			state.TransportState = "PLAYING"
		case "PAUSED":
			state.TransportState = "PAUSED_PLAYBACK"
		case "STOPPED", "IDLE":
			state.TransportState = "STOPPED"
		case "TRANSITIONING":
			state.TransportState = "TRANSITIONING"
		}

		if playback.Volume != nil {
			state.Volume = *playback.Volume
			state.VolumeUpdatedAt = now
		}

		if playback.Muted != nil {
			state.Muted = *playback.Muted
		}

		state.TransportUpdatedAt = now
	}

	if metadata != nil {
		// Build DIDL-lite style metadata string if we have track info
		// This is a simplified version - full DIDL would be more complex
		if metadata.TrackName != "" {
			// For now, store the track name as metadata
			// The hybrid layer can parse this or we can build proper DIDL later
			state.CurrentTrackMetaData = metadata.TrackName
		}
	}

	state.UpdatedAt = now
	state.Source = "cloud_webhook"
}
