package sonos

import (
	"context"
	"errors"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// PlaybackState contains playback information from the state cache.
// This mirrors events.DeviceState but avoids the import cycle.
type PlaybackState struct {
	DeviceIP         string
	DeviceUDN        string
	TransportState   string
	TransportStatus  string
	CurrentTrackURI  string
	TrackDuration    string
	RelativeTime     string
	TrackMetaData    string
	CurrentURIMetaData string
	Volume           int
	Muted            bool
	UpdatedAt        time.Time
	Source           string
}

// StateProvider is an interface for accessing device playback state.
// This is implemented by events.StateCache but defined here to avoid import cycles.
type StateProvider interface {
	// GetPlaybackState returns the playback state for a device IP, or nil if not cached/stale.
	GetPlaybackState(deviceIP string) *PlaybackState
}

// Service exposes Sonos operations needed by routes.
type Service struct {
	DeviceService   *devices.Service
	SoapClient      *soap.Client
	DefaultDeviceIP string
	SoapTimeout     time.Duration
	ZoneCache       *ZoneGroupCache
	StateProvider   StateProvider // UPnP event state cache for hybrid data layer
}

// NewService creates a new Sonos service with the given dependencies.
func NewService(deviceService *devices.Service, soapClient *soap.Client, defaultIP string, timeout time.Duration) *Service {
	return &Service{
		DeviceService:   deviceService,
		SoapClient:      soapClient,
		DefaultDeviceIP: defaultIP,
		SoapTimeout:     timeout,
		ZoneCache:       NewZoneGroupCache(30 * time.Second), // Default 30s TTL
	}
}

// NewServiceWithConfig creates a new Sonos service with configuration options.
func NewServiceWithConfig(deviceService *devices.Service, soapClient *soap.Client, defaultIP string, timeout time.Duration, zoneCacheTTL time.Duration) *Service {
	return &Service{
		DeviceService:   deviceService,
		SoapClient:      soapClient,
		DefaultDeviceIP: defaultIP,
		SoapTimeout:     timeout,
		ZoneCache:       NewZoneGroupCache(zoneCacheTTL),
	}
}

// NewServiceWithStateProvider creates a new Sonos service with a state provider for hybrid data fetching.
func NewServiceWithStateProvider(deviceService *devices.Service, soapClient *soap.Client, defaultIP string, timeout time.Duration, zoneCacheTTL time.Duration, stateProvider StateProvider) *Service {
	return &Service{
		DeviceService:   deviceService,
		SoapClient:      soapClient,
		DefaultDeviceIP: defaultIP,
		SoapTimeout:     timeout,
		ZoneCache:       NewZoneGroupCache(zoneCacheTTL),
		StateProvider:   stateProvider,
	}
}

// ResolveDeviceIP resolves the IP for a device ID, falling back to default.
func (service *Service) ResolveDeviceIP(deviceID string) (string, error) {
	if service.DeviceService == nil {
		if service.DefaultDeviceIP == "" {
			return "", errors.New("no device resolver available")
		}
		return service.DefaultDeviceIP, nil
	}

	ip, err := service.DeviceService.ResolveDeviceIP(deviceID)
	if err != nil {
		return "", err
	}
	if ip == "" {
		if service.DefaultDeviceIP == "" {
			return "", errors.New("device not found")
		}
		return service.DefaultDeviceIP, nil
	}
	return ip, nil
}

func (service *Service) ListAlarms(deviceIP string) (soap.AlarmListResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.ListAlarms(ctx, deviceIP)
}

func (service *Service) BrowseFavorites(start, count int) (soap.BrowseResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Browse(ctx, service.DefaultDeviceIP, "FV:2", "BrowseDirectChildren", "*", start, count)
}

func (service *Service) Stop(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Stop(ctx, deviceIP)
}

func (service *Service) Pause(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Pause(ctx, deviceIP)
}

func (service *Service) Play(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Play(ctx, deviceIP)
}

func (service *Service) Next(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Next(ctx, deviceIP)
}

func (service *Service) Previous(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.Previous(ctx, deviceIP)
}

func (service *Service) GetTransportInfo(deviceIP string) (soap.TransportInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetTransportInfo(ctx, deviceIP)
}

func (service *Service) GetPositionInfo(deviceIP string) (soap.PositionInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetPositionInfo(ctx, deviceIP)
}

func (service *Service) GetMediaInfo(deviceIP string) (soap.MediaInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetMediaInfo(ctx, deviceIP)
}

func (service *Service) GetVolume(deviceIP string) (soap.VolumeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetVolume(ctx, deviceIP)
}

func (service *Service) SetVolume(deviceIP string, level int) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.SetVolume(ctx, deviceIP, level)
}

func (service *Service) GetMute(deviceIP string) (soap.MuteInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetMute(ctx, deviceIP)
}

func (service *Service) GetZoneGroupState(deviceIP string) (soap.ZoneGroupState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetZoneGroupState(ctx, deviceIP)
}

// GetZoneGroupStateCached returns zone group state with caching.
// It checks the cache first and falls back to a SOAP call if cache is stale.
func (service *Service) GetZoneGroupStateCached(deviceIP string) (*soap.ZoneGroupState, error) {
	if service.ZoneCache == nil {
		state, err := service.GetZoneGroupState(deviceIP)
		if err != nil {
			return nil, err
		}
		return &state, nil
	}

	return service.ZoneCache.GetOrFetch(func() (*soap.ZoneGroupState, error) {
		state, err := service.GetZoneGroupState(deviceIP)
		if err != nil {
			return nil, err
		}
		return &state, nil
	})
}

func (service *Service) GetZoneAttributes(deviceIP string) (soap.ZoneAttributes, error) {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.GetZoneAttributes(ctx, deviceIP)
}

func (service *Service) SetAVTransportURI(deviceIP string, uri string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.SetAVTransportURI(ctx, deviceIP, uri, "")
}

func (service *Service) BecomeCoordinatorOfStandaloneGroup(deviceIP string) error {
	ctx, cancel := context.WithTimeout(context.Background(), service.SoapTimeout)
	defer cancel()
	return service.SoapClient.BecomeCoordinatorOfStandaloneGroup(ctx, deviceIP)
}
