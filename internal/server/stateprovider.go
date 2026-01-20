package server

import (
	"github.com/strefethen/sonos-hub-go/internal/sonos"
	"github.com/strefethen/sonos-hub-go/internal/sonos/events"
)

// StateCacheAdapter adapts events.StateCache to implement sonos.StateProvider.
// This breaks the import cycle between sonos and events packages.
type StateCacheAdapter struct {
	cache *events.StateCache
}

// NewStateCacheAdapter creates a new adapter wrapping the given StateCache.
func NewStateCacheAdapter(cache *events.StateCache) *StateCacheAdapter {
	return &StateCacheAdapter{cache: cache}
}

// GetPlaybackState implements sonos.StateProvider.
// It retrieves the device state from the cache and converts it to sonos.PlaybackState.
func (a *StateCacheAdapter) GetPlaybackState(deviceIP string) *sonos.PlaybackState {
	if a.cache == nil {
		return nil
	}

	state := a.cache.Get(deviceIP)
	if state == nil {
		return nil
	}

	return &sonos.PlaybackState{
		DeviceIP:           state.DeviceIP,
		DeviceUDN:          state.DeviceUDN,
		TransportState:     state.TransportState,
		TransportStatus:    state.TransportStatus,
		CurrentTrackURI:    state.CurrentTrackURI,
		TrackDuration:      state.TrackDuration,
		RelativeTime:       state.RelativeTime,
		TrackMetaData:      state.CurrentTrackMetaData,
		CurrentURIMetaData: state.CurrentURIMetaData,
		Volume:             state.Volume,
		Muted:              state.Muted,
		UpdatedAt:          state.UpdatedAt,
		Source:             state.Source,
	}
}
