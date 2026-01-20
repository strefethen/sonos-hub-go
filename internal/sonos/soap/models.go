package soap

// TransportInfo mirrors Sonos GetTransportInfo response.
type TransportInfo struct {
	CurrentTransportState  string
	CurrentTransportStatus string
	CurrentSpeed           string
}

// PositionInfo mirrors Sonos GetPositionInfo response.
type PositionInfo struct {
	Track         int
	TrackDuration string
	TrackMetaData string
	TrackURI      string
	RelTime       string
	AbsTime       string
}

// MediaInfo mirrors Sonos GetMediaInfo response.
type MediaInfo struct {
	NrTracks           int
	MediaDuration      string
	CurrentURI         string
	CurrentURIMetaData string
}

// VolumeInfo mirrors Sonos GetVolume response.
type VolumeInfo struct {
	CurrentVolume int
}

// MuteInfo mirrors Sonos GetMute response.
type MuteInfo struct {
	CurrentMute bool
}

// ZoneGroupState mirrors GetZoneGroupState result (minimal subset needed).
type ZoneGroupState struct {
	Groups []ZoneGroup
}

// ZoneGroup represents a Sonos group.
type ZoneGroup struct {
	ID          string
	Coordinator string
	Members     []ZoneMember
}

// ZoneMember represents a member device in a group.
type ZoneMember struct {
	UUID             string
	ZoneName         string
	Location         string
	IsCoordinator    bool
	IsVisible        bool
	IsSatellite      bool
	IsSubwoofer      bool
	ChannelMapSet    string
	HdmiCecAvailable bool // true if device has HDMI capability (Arc, Beam, Ray)
}

// FavoriteItem represents a Sonos favorite (subset).
type FavoriteItem struct {
	ID               string
	ParentID         string
	Title            string
	Ordinal          string
	UpnpClass        string
	ContentType      string
	FavoriteType     string
	ServiceName      string
	ServiceLogoURL   string
	AlbumArtURI      string
	Resource         string
	ProtocolInfo     string
	ResourceMetaData string
}

// Alarm mirrors an alarm item from ListAlarms.
type Alarm struct {
	ID                 string
	StartTime          string
	Duration           string
	Recurrence         string
	Enabled            bool
	RoomUUID           string
	ProgramURI         string
	ProgramMetaData    string
	PlayMode           string
	Volume             int
	IncludeLinkedZones bool
}

// AlarmListResult mirrors ListAlarms response.
type AlarmListResult struct {
	AlarmListVersion string
	Alarms           []Alarm
}
