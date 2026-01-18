package sonos

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// MusicServiceCredentials contains authentication info extracted from existing favorites
// Used by MusicURIBuilder for building service-specific URIs
type MusicServiceCredentials struct {
	SID           int
	SN            int
	Token         int
	SessionSuffix string
	ExtractedAt   time.Time
}

// MusicServiceConfig defines URI building configuration for a music service
type MusicServiceConfig struct {
	SID          int                               // Service ID (Spotify=12, Apple Music=204)
	TokenPrefix  string                            // e.g., "SA_RINCON"
	ContentTypes map[string]MusicContentTypeConfig
}

// MusicContentTypeConfig defines how to build URIs for a specific content type
type MusicContentTypeConfig struct {
	URIScheme    string // "x-rincon-cpcontainer", "x-sonos-http", "x-sonosapi-radio"
	ItemIDPrefix string // Hex prefix for item IDs
	IDPrefix     string // Content ID prefix (e.g., "spotify:playlist:")
	IDSuffix     string // Optional suffix (e.g., ".mp4" for Apple Music tracks)
	DefaultFlags int    // Playback flags
	UpnpClass    string // UPnP class for DIDL metadata
}

// musicServiceConfigs contains the known working configurations for music services
var musicServiceConfigs = map[string]MusicServiceConfig{
	"spotify": {
		SID:         12,
		TokenPrefix: "SA_RINCON",
		ContentTypes: map[string]MusicContentTypeConfig{
			"playlist": {
				URIScheme:    "x-rincon-cpcontainer",
				ItemIDPrefix: "1006206c",
				IDPrefix:     "spotify:playlist:",
				DefaultFlags: 8300,
				UpnpClass:    "object.container.playlistContainer",
			},
			"album": {
				URIScheme:    "x-rincon-cpcontainer",
				ItemIDPrefix: "1004006c",
				IDPrefix:     "spotify:album:",
				DefaultFlags: 108,
				UpnpClass:    "object.container.album.musicAlbum",
			},
			"station": {
				URIScheme:    "x-sonosapi-radio",
				ItemIDPrefix: "100c206c",
				IDPrefix:     "spotify:artistRadio:",
				DefaultFlags: 8300,
				UpnpClass:    "object.item.audioItem.audioBroadcast",
			},
			"track": {
				URIScheme:    "x-sonos-http",
				ItemIDPrefix: "00032020",
				IDPrefix:     "spotify:track:",
				DefaultFlags: 8224,
				UpnpClass:    "object.item.audioItem.musicTrack",
			},
		},
	},
	"apple_music": {
		SID:         204,
		TokenPrefix: "SA_RINCON",
		ContentTypes: map[string]MusicContentTypeConfig{
			"track": {
				URIScheme:    "x-sonos-http",
				ItemIDPrefix: "10032028",
				IDPrefix:     "song:",
				IDSuffix:     ".mp4",
				DefaultFlags: 8232,
				UpnpClass:    "object.item.audioItem.musicTrack",
			},
			"playlist": {
				URIScheme:    "x-rincon-cpcontainer",
				ItemIDPrefix: "1006206c",
				IDPrefix:     "playlist:",
				DefaultFlags: 8300,
				UpnpClass:    "object.container.playlistContainer",
			},
			"album": {
				URIScheme:    "x-rincon-cpcontainer",
				ItemIDPrefix: "1004206c",
				IDPrefix:     "libraryalbum:l.",
				DefaultFlags: 8300,
				UpnpClass:    "object.container.album.musicAlbum",
			},
			"station": {
				URIScheme:    "x-sonosapi-radio",
				ItemIDPrefix: "100c706c",
				IDPrefix:     "radio:",
				DefaultFlags: 28780,
				UpnpClass:    "object.item.audioItem.audioBroadcast",
			},
		},
	},
}

// MusicURIBuilder constructs URIs for playing content from music services on Sonos
type MusicURIBuilder struct{}

// NewMusicURIBuilder creates a new MusicURIBuilder instance
func NewMusicURIBuilder() *MusicURIBuilder {
	return &MusicURIBuilder{}
}

// BuildURI constructs a playable URI for the given service and content.
// credentials contains SID, SN, token info extracted from existing favorites.
func (b *MusicURIBuilder) BuildURI(service, contentType, contentID, title string, credentials *MusicServiceCredentials) (string, string, error) {
	config, ok := b.GetServiceConfig(service)
	if !ok {
		return "", "", fmt.Errorf("unsupported service: %s", service)
	}

	typeConfig, ok := b.GetContentTypeConfig(service, contentType)
	if !ok {
		return "", "", fmt.Errorf("unsupported content type %s for service %s", contentType, service)
	}

	// Determine SID and SN - use credentials if provided, otherwise use defaults
	sid := config.SID
	sn := 1 // Default serial number
	flags := typeConfig.DefaultFlags

	if credentials != nil {
		sid = credentials.SID
		sn = credentials.SN
	}

	// Build the item ID: prefix + encoded content identifier
	encodedID := url.QueryEscape(typeConfig.IDPrefix + contentID + typeConfig.IDSuffix)
	itemID := typeConfig.ItemIDPrefix + encodedID

	// Build the URI based on the scheme
	var uri string
	switch typeConfig.URIScheme {
	case "x-rincon-cpcontainer":
		uri = b.BuildContainerURI(itemID, sid, sn, flags)
	case "x-sonos-http":
		uri = b.BuildStreamURI(itemID, sid, sn, flags)
	case "x-sonosapi-radio":
		uri = b.BuildRadioURI(itemID, sid, sn, flags)
	default:
		return "", "", fmt.Errorf("unsupported URI scheme: %s", typeConfig.URIScheme)
	}

	// Build the DIDL metadata
	metadata := b.BuildDIDLMetadata(service, contentType, contentID, title, credentials)

	return uri, metadata, nil
}

// BuildContainerURI builds x-rincon-cpcontainer URI for playlists/albums
func (b *MusicURIBuilder) BuildContainerURI(itemID string, sid, sn, flags int) string {
	return fmt.Sprintf("x-rincon-cpcontainer:%s?sid=%d&flags=%d&sn=%d", itemID, sid, flags, sn)
}

// BuildStreamURI builds x-sonos-http URI for tracks
func (b *MusicURIBuilder) BuildStreamURI(itemID string, sid, sn, flags int) string {
	return fmt.Sprintf("x-sonos-http:%s?sid=%d&flags=%d&sn=%d", itemID, sid, flags, sn)
}

// BuildRadioURI builds x-sonosapi-radio URI for stations
func (b *MusicURIBuilder) BuildRadioURI(itemID string, sid, sn, flags int) string {
	return fmt.Sprintf("x-sonosapi-radio:%s?sid=%d&flags=%d&sn=%d", itemID, sid, flags, sn)
}

// BuildDIDLMetadata builds the DIDL-Lite XML metadata for the content
func (b *MusicURIBuilder) BuildDIDLMetadata(service, contentType, contentID, title string, credentials *MusicServiceCredentials) string {
	config, ok := b.GetServiceConfig(service)
	if !ok {
		return ""
	}

	typeConfig, ok := b.GetContentTypeConfig(service, contentType)
	if !ok {
		return ""
	}

	// Build the item ID for the metadata
	encodedID := url.QueryEscape(typeConfig.IDPrefix + contentID + typeConfig.IDSuffix)
	itemID := typeConfig.ItemIDPrefix + encodedID

	// Build the desc element with service credentials
	// Format: SA_RINCON{SID}_{SN}_X_#Svc{SID}-{suffix}-Token
	sid := config.SID
	sn := 1
	token := 0
	sessionSuffix := ""

	if credentials != nil {
		sid = credentials.SID
		sn = credentials.SN
		token = credentials.Token
		sessionSuffix = credentials.SessionSuffix
	}

	// Build the desc value
	var descValue string
	if sessionSuffix != "" {
		descValue = fmt.Sprintf("%s%d_%d_X_#Svc%d-%s-Token", config.TokenPrefix, sid, sn, sid, sessionSuffix)
	} else if token > 0 {
		descValue = fmt.Sprintf("%s%d_%d_X_#Svc%d-0-Token", config.TokenPrefix, sid, sn, sid)
	} else {
		descValue = fmt.Sprintf("%s%d_%d_X_#Svc%d-0-Token", config.TokenPrefix, sid, sn, sid)
	}

	// Escape the title for XML
	escapedTitle := escapeXMLContent(title)

	// Build the DIDL-Lite XML
	didl := fmt.Sprintf(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="%s" parentID="-1" restricted="true"><dc:title>%s</dc:title><upnp:class>%s</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">%s</desc></item></DIDL-Lite>`,
		itemID,
		escapedTitle,
		typeConfig.UpnpClass,
		descValue,
	)

	return didl
}

// GetServiceConfig returns the configuration for a service
func (b *MusicURIBuilder) GetServiceConfig(service string) (*MusicServiceConfig, bool) {
	normalizedService := strings.ToLower(strings.TrimSpace(service))
	config, ok := musicServiceConfigs[normalizedService]
	if !ok {
		return nil, false
	}
	return &config, true
}

// GetContentTypeConfig returns config for a specific content type
func (b *MusicURIBuilder) GetContentTypeConfig(service, contentType string) (*MusicContentTypeConfig, bool) {
	config, ok := b.GetServiceConfig(service)
	if !ok {
		return nil, false
	}

	normalizedType := strings.ToLower(strings.TrimSpace(contentType))
	typeConfig, ok := config.ContentTypes[normalizedType]
	if !ok {
		return nil, false
	}
	return &typeConfig, true
}

// IsServiceSupported checks if a service is supported for direct playback
func (b *MusicURIBuilder) IsServiceSupported(service string) bool {
	_, ok := b.GetServiceConfig(service)
	return ok
}

// GetSupportedContentTypes returns content types supported by a service
func (b *MusicURIBuilder) GetSupportedContentTypes(service string) []string {
	config, ok := b.GetServiceConfig(service)
	if !ok {
		return nil
	}

	types := make([]string, 0, len(config.ContentTypes))
	for contentType := range config.ContentTypes {
		types = append(types, contentType)
	}
	return types
}

// GetSupportedServices returns all supported service names
func (b *MusicURIBuilder) GetSupportedServices() []string {
	services := make([]string, 0, len(musicServiceConfigs))
	for service := range musicServiceConfigs {
		services = append(services, service)
	}
	return services
}

// escapeXMLContent escapes special characters for XML content
func escapeXMLContent(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
