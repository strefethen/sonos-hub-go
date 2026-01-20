package sonos

import (
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// ZoneGroupCache caches zone group topology state with a configurable TTL.
// Zone group topology changes infrequently (grouping, device additions) so caching
// significantly reduces SOAP calls when fetching now-playing state.
type ZoneGroupCache struct {
	mu       sync.RWMutex
	state    *soap.ZoneGroupState
	cachedAt time.Time
	ttl      time.Duration
}

// NewZoneGroupCache creates a new cache with the specified TTL.
func NewZoneGroupCache(ttl time.Duration) *ZoneGroupCache {
	return &ZoneGroupCache{
		ttl: ttl,
	}
}

// Get returns the cached zone group state if it exists and is still fresh.
// Returns nil if cache is empty or expired.
func (c *ZoneGroupCache) Get() *soap.ZoneGroupState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.state == nil {
		return nil
	}

	if time.Since(c.cachedAt) > c.ttl {
		return nil
	}

	return c.state
}

// Set stores the zone group state in the cache.
func (c *ZoneGroupCache) Set(state *soap.ZoneGroupState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = state
	c.cachedAt = time.Now()
}

// Invalidate clears the cache. Call this when zone topology changes are detected
// (e.g., from UPnP events).
func (c *ZoneGroupCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = nil
	c.cachedAt = time.Time{}
}

// GetOrFetch returns cached state if fresh, otherwise calls the fetcher function.
// This provides a convenient cache-aside pattern.
func (c *ZoneGroupCache) GetOrFetch(fetcher func() (*soap.ZoneGroupState, error)) (*soap.ZoneGroupState, error) {
	// First try the cache with read lock
	if state := c.Get(); state != nil {
		return state, nil
	}

	// Cache miss - fetch new state
	state, err := fetcher()
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.Set(state)
	return state, nil
}

// Stats returns cache statistics for debugging/monitoring.
type CacheStats struct {
	CachedAt  time.Time
	Age       time.Duration
	TTL       time.Duration
	HasData   bool
	IsFresh   bool
	GroupCount int
}

func (c *ZoneGroupCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		TTL:     c.ttl,
		HasData: c.state != nil,
	}

	if c.state != nil {
		stats.CachedAt = c.cachedAt
		stats.Age = time.Since(c.cachedAt)
		stats.IsFresh = stats.Age <= c.ttl
		stats.GroupCount = len(c.state.Groups)
	}

	return stats
}
