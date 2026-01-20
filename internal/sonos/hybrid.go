package sonos

import (
	"log"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// DataSource indicates where playback data came from.
type DataSource string

const (
	DataSourceCache    DataSource = "upnp_cache"
	DataSourceSOAPPoll DataSource = "soap_poll"
)

// HybridPlaybackInfo wraps GroupPlaybackInfo with source tracking.
type HybridPlaybackInfo struct {
	GroupPlaybackInfo
	Source    DataSource
	CacheAge  time.Duration // Age of cached data (0 if from SOAP)
	FetchTime time.Duration // Time taken to fetch (for SOAP)
}

// HybridGroupResult combines coordinator info with hybrid playback data.
type HybridGroupResult struct {
	Coordinator CoordinatorInfo
	Playback    HybridPlaybackInfo
}

// GetPlaybackFromCache attempts to build playback info from the state cache.
// Returns nil if cache is unavailable or stale for this device.
func GetPlaybackFromCache(svc *Service, coordinatorIP string) *HybridPlaybackInfo {
	if svc.StateProvider == nil {
		log.Printf("HYBRID: StateProvider is nil")
		return nil
	}

	state := svc.StateProvider.GetPlaybackState(coordinatorIP)
	if state == nil {
		log.Printf("HYBRID: No cached state for %s", coordinatorIP)
		return nil
	}

	// Convert cached state to GroupPlaybackInfo format
	result := &HybridPlaybackInfo{
		Source:   DataSourceCache,
		CacheAge: time.Since(state.UpdatedAt),
	}

	// Transport info from cache
	result.TransportInfo = &soap.TransportInfo{
		CurrentTransportState:  state.TransportState,
		CurrentTransportStatus: state.TransportStatus,
		CurrentSpeed:           "1",
	}

	// Volume info from cache
	result.VolumeInfo = &soap.VolumeInfo{
		CurrentVolume: state.Volume,
	}

	// Mute info from cache
	result.MuteInfo = &soap.MuteInfo{
		CurrentMute: state.Muted,
	}

	// Position info from cache (only if we have data)
	if state.CurrentTrackURI != "" || state.TrackMetaData != "" {
		result.PositionInfo = &soap.PositionInfo{
			TrackURI:      state.CurrentTrackURI,
			TrackDuration: state.TrackDuration,
			RelTime:       state.RelativeTime,
			TrackMetaData: state.TrackMetaData,
		}
	}

	// Media info from cache
	if state.CurrentURIMetaData != "" {
		result.MediaInfo = &soap.MediaInfo{
			CurrentURIMetaData: state.CurrentURIMetaData,
		}
	}

	return result
}

// FetchPlaybackWithFallback tries cache first, then falls back to SOAP poll.
func FetchPlaybackWithFallback(svc *Service, coordinatorIP string) HybridPlaybackInfo {
	// Try cache first
	if cached := GetPlaybackFromCache(svc, coordinatorIP); cached != nil {
		log.Printf("HYBRID: Cache hit for %s (age: %v)", coordinatorIP, cached.CacheAge.Round(time.Millisecond))
		return *cached
	}

	// Fall back to SOAP
	log.Printf("HYBRID: SOAP fallback for %s", coordinatorIP)
	start := time.Now()
	playback := FetchGroupPlayback(svc, coordinatorIP)

	return HybridPlaybackInfo{
		GroupPlaybackInfo: playback,
		Source:            DataSourceSOAPPoll,
		FetchTime:         time.Since(start),
	}
}

// FetchAllGroupsPlaybackHybrid fetches playback for all groups using cache-first approach.
// Phase 1: Check cache for all coordinators
// Phase 2: Parallel SOAP fetch only for cache misses
func FetchAllGroupsPlaybackHybrid(svc *Service, coordinators []CoordinatorInfo) ([]HybridGroupResult, map[string]DataSource) {
	results := make([]HybridGroupResult, len(coordinators))
	sources := make(map[string]DataSource)

	// Phase 1: Check cache for all coordinators
	cacheMisses := make([]int, 0, len(coordinators))

	for i, coord := range coordinators {
		if cached := GetPlaybackFromCache(svc, coord.IP); cached != nil {
			results[i] = HybridGroupResult{
				Coordinator: coord,
				Playback:    *cached,
			}
			sources[coord.IP] = DataSourceCache
		} else {
			cacheMisses = append(cacheMisses, i)
		}
	}

	// Log cache statistics
	cacheHits := len(coordinators) - len(cacheMisses)
	if cacheHits > 0 || len(cacheMisses) > 0 {
		log.Printf("HYBRID: %d cache hits, %d SOAP fetches needed", cacheHits, len(cacheMisses))
	}

	// Phase 2: If no cache misses, we're done
	if len(cacheMisses) == 0 {
		return results, sources
	}

	// Build list of coordinators that need SOAP fetch
	missCoordinators := make([]CoordinatorInfo, len(cacheMisses))
	for i, idx := range cacheMisses {
		missCoordinators[i] = coordinators[idx]
	}

	// Parallel fetch for cache misses
	soapResults := FetchAllGroupsPlayback(svc, missCoordinators)

	// Merge SOAP results back into main results
	for i, idx := range cacheMisses {
		coord := coordinators[idx]
		results[idx] = HybridGroupResult{
			Coordinator: coord,
			Playback: HybridPlaybackInfo{
				GroupPlaybackInfo: soapResults[i].Playback,
				Source:            DataSourceSOAPPoll,
			},
		}
		sources[coord.IP] = DataSourceSOAPPoll
	}

	return results, sources
}

// DataSourceStats provides statistics about data sources used.
type DataSourceStats struct {
	TotalGroups int
	CacheHits   int
	SOAPFetches int
	Sources     map[string]DataSource
}

// GetDataSourceStats calculates statistics from a sources map.
func GetDataSourceStats(sources map[string]DataSource) DataSourceStats {
	stats := DataSourceStats{
		TotalGroups: len(sources),
		Sources:     sources,
	}

	for _, source := range sources {
		switch source {
		case DataSourceCache:
			stats.CacheHits++
		case DataSourceSOAPPoll:
			stats.SOAPFetches++
		}
	}

	return stats
}
