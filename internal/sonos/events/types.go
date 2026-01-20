package events

import (
	"time"
)

// UPnP GENA service paths for Sonos devices
const (
	AVTransportEventPath       = "/MediaRenderer/AVTransport/Event"
	RenderingControlEventPath  = "/MediaRenderer/RenderingControl/Event"
	ZoneGroupTopologyEventPath = "/ZoneGroupTopology/Event"
)

// ServiceType represents the type of UPnP service
type ServiceType string

const (
	ServiceAVTransport       ServiceType = "AVTransport"
	ServiceRenderingControl  ServiceType = "RenderingControl"
	ServiceZoneGroupTopology ServiceType = "ZoneGroupTopology"
)

// EventPaths returns the subscription paths for each service type
func EventPaths() map[ServiceType]string {
	return map[ServiceType]string{
		ServiceAVTransport:       AVTransportEventPath,
		ServiceRenderingControl:  RenderingControlEventPath,
		ServiceZoneGroupTopology: ZoneGroupTopologyEventPath,
	}
}

// Subscription represents an active UPnP event subscription
type Subscription struct {
	SID         string      // Subscription ID returned by device
	DeviceIP    string      // IP address of the subscribed device
	DeviceUDN   string      // UDN of the subscribed device
	ServiceType ServiceType // Type of service subscribed to
	CallbackURL string      // Our callback URL for NOTIFY events
	Timeout     int         // Subscription timeout in seconds
	SubscribedAt time.Time  // When the subscription was created
	RenewAt      time.Time  // When the subscription should be renewed
	SEQ         int         // Last received sequence number
}

// IsExpiringSoon returns true if the subscription should be renewed
func (s *Subscription) IsExpiringSoon() bool {
	// Renew 60 seconds before expiry
	return time.Now().After(s.RenewAt)
}

// IsExpired returns true if the subscription has expired
func (s *Subscription) IsExpired() bool {
	return time.Now().After(s.SubscribedAt.Add(time.Duration(s.Timeout) * time.Second))
}

// DeviceState represents the current playback state of a device
// This is updated from UPnP events and cached for fast access
type DeviceState struct {
	// Device identification
	DeviceIP  string
	DeviceUDN string

	// Transport state (from AVTransport events)
	TransportState  string // PLAYING, PAUSED_PLAYBACK, STOPPED, TRANSITIONING
	TransportStatus string // OK, ERROR
	CurrentTrackURI string
	TrackDuration   string // HH:MM:SS format
	RelativeTime    string // HH:MM:SS format

	// Track metadata (from AVTransport events)
	TrackMetaData        string
	CurrentTrackMetaData string

	// Container metadata
	CurrentURIMetaData string
	AVTransportURI     string

	// Rendering state (from RenderingControl events)
	Volume int
	Muted  bool

	// Timestamps
	UpdatedAt          time.Time
	TransportUpdatedAt time.Time
	VolumeUpdatedAt    time.Time

	// Source tracking
	Source string // "upnp_event", "soap_poll"
}

// IsFresh returns true if the state was updated within the TTL
func (s *DeviceState) IsFresh(ttl time.Duration) bool {
	return time.Since(s.UpdatedAt) <= ttl
}

// NotifyEvent represents a parsed NOTIFY event from a Sonos device
type NotifyEvent struct {
	SID         string      // Subscription ID
	SEQ         int         // Sequence number
	ServiceType ServiceType // Inferred from callback path
	DeviceIP    string      // Source device IP
	Properties  map[string]string // Parsed property values
	RawBody     []byte      // Raw event body for debugging
}

// AVTransportEvent represents parsed AVTransport event data
type AVTransportEvent struct {
	TransportState       string
	TransportStatus      string
	CurrentTrackURI      string
	CurrentTrackMetaData string
	TrackDuration        string
	RelTime              string
	AVTransportURI       string
	AVTransportURIMetaData string
}

// RenderingControlEvent represents parsed RenderingControl event data
type RenderingControlEvent struct {
	Volume int
	Muted  bool
}

// ZoneGroupTopologyEvent represents parsed ZoneGroupTopology event data
type ZoneGroupTopologyEvent struct {
	ZoneGroupState string // Raw XML zone group state
	Changed        bool   // Whether the topology changed
}

// ManagerConfig holds configuration for the event manager
type ManagerConfig struct {
	// Enabled controls whether UPnP events are enabled
	Enabled bool

	// CallbackPort is the port for the NOTIFY callback server
	// If 0, uses the main server port
	CallbackPort int

	// SubscriptionTimeout is the requested subscription duration in seconds
	// Sonos typically accepts 1-3600 seconds
	SubscriptionTimeout int

	// RenewalBuffer is how many seconds before expiry to renew subscriptions
	RenewalBuffer int

	// StateCacheTTL is how long to consider event-based state as fresh
	StateCacheTTL time.Duration

	// Services lists which services to subscribe to
	// Default is all three: AVTransport, RenderingControl, ZoneGroupTopology
	Services []ServiceType
}

// DefaultManagerConfig returns the default configuration
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		Enabled:             true,
		CallbackPort:        0, // Use main server port
		SubscriptionTimeout: 3600,
		RenewalBuffer:       60,
		StateCacheTTL:       30 * time.Second,
		Services: []ServiceType{
			ServiceAVTransport,
			ServiceRenderingControl,
			ServiceZoneGroupTopology,
		},
	}
}

// ManagerStats provides statistics about the event manager
type ManagerStats struct {
	Enabled              bool
	ActiveSubscriptions  int
	TotalDevices         int
	EventsReceived       int64
	EventsProcessed      int64
	SubscriptionFailures int64
	RenewalFailures      int64
	LastEventAt          time.Time
	CacheHits            int64
	CacheMisses          int64
}
