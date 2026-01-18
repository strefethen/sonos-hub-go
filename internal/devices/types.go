package devices

import "time"

// DeviceRole matches the shared enum values.
type DeviceRole string

const (
	DeviceRoleNormal            DeviceRole = "NORMAL"
	DeviceRoleHomeTheaterMaster DeviceRole = "HOME_THEATER_MASTER"
	DeviceRoleSurround          DeviceRole = "SURROUND"
	DeviceRoleSub               DeviceRole = "SUB"
)

// DeviceHealthStatus mirrors Node health values.
type DeviceHealthStatus string

const (
	DeviceHealthOK       DeviceHealthStatus = "OK"
	DeviceHealthDegraded DeviceHealthStatus = "DEGRADED"
	DeviceHealthOffline  DeviceHealthStatus = "OFFLINE"
)

const (
	DegradedThreshold = 1
	OfflineThreshold  = 3
	RemovalThreshold  = 1440
)

// RawSonosDevice is the direct discovery payload.
type RawSonosDevice struct {
	UDN             string
	IP              string
	Model           string
	ModelNumber     string
	RoomName        string
	SerialNumber    string
	SoftwareVersion string
	HardwareVersion string
	SupportsAirPlay bool
	ZoneGroupState  string
	DiscoveredAt    time.Time
	Location        string
}

// PhysicalDevice is a single Sonos speaker.
type PhysicalDevice struct {
	DeviceID             string
	UDN                  string
	IP                   string
	Model                string
	ModelNumber          string
	RoomName             string
	Role                 DeviceRole
	IsCoordinatorCapable bool
	SupportsAirPlay      bool
	LastSeenAt           time.Time
	Capabilities         map[string]any
	Health               DeviceHealthStatus
	MissedScans          int
}

// LogicalDevice is the targetable entity.
type LogicalDevice struct {
	DeviceID             string
	RoomName             string
	IP                   string
	Model                string
	Role                 DeviceRole
	IsTargetable         bool
	IsCoordinatorCapable bool
	SupportsAirPlay      bool
	LogicalGroupID       string
	PhysicalDevices      []PhysicalDevice
	LastSeenAt           time.Time
	Health               DeviceHealthStatus
	MissedScans          int
}

// DeviceTopology is the full relationship graph.
type DeviceTopology struct {
	Devices           []LogicalDevice
	HomeTheaterGroups []HomeTheaterGroup
	StereoPairs       []StereoPair
	UpdatedAt         time.Time
}

// HomeTheaterGroup represents Arc + surrounds + sub.
type HomeTheaterGroup struct {
	GroupID   string
	Master    PhysicalDevice
	Surrounds []PhysicalDevice
	Sub       *PhysicalDevice
}

// StereoPair represents a stereo pair.
type StereoPair struct {
	PairID      string
	RoomName    string
	Left        PhysicalDevice
	Right       PhysicalDevice
	Coordinator PhysicalDevice
}

// ZoneGroupTopology is the parsed topology from GetZoneGroupState.
type ZoneGroupTopology struct {
	Groups []ZoneGroup
}

type ZoneGroup struct {
	GroupID        string
	CoordinatorUDN string
	Members        []ZoneMember
}

type ZoneMember struct {
	UDN           string
	Location      string
	ZoneName      string
	IsCoordinator bool
	IsSatellite   bool
	IsSubwoofer   bool
	ChannelMapSet string
}

// SONOS_MODELS defines model capabilities.
var SONOS_MODELS = map[string]struct {
	IsCoordinatorCapable bool
	SupportsAirPlay      bool
	IsHomeTheaterCapable bool
}{
	"S18": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: true},
	"S34": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: true},
	"S14": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: true},
	"S11": {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: true},
	"S9":  {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: true},
	"S38": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: true},
	"S21": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S27": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S17": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S23": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S36": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S40": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S6":  {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S3":  {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S31": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S24": {IsCoordinatorCapable: true, SupportsAirPlay: true, IsHomeTheaterCapable: false},
	"S1":  {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S12": {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S5":  {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S15": {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S22": {IsCoordinatorCapable: true, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S2":  {IsCoordinatorCapable: false, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S10": {IsCoordinatorCapable: false, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S33": {IsCoordinatorCapable: false, SupportsAirPlay: false, IsHomeTheaterCapable: false},
	"S37": {IsCoordinatorCapable: false, SupportsAirPlay: false, IsHomeTheaterCapable: false},
}
