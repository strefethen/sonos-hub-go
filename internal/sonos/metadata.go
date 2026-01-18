package sonos

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// TrackMetadata represents a Sonos track payload.
type TrackMetadata struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtURI string
	Source      string
	ServiceName string
}

// ContainerMetadata represents a playlist/station container.
type ContainerMetadata struct {
	Name string
	Type string
}

// ParseDidlMetadata parses DIDL-Lite metadata into a TrackMetadata struct.
func ParseDidlMetadata(didlXML string, trackURI string) *TrackMetadata {
	if strings.TrimSpace(didlXML) == "" || didlXML == "NOT_IMPLEMENTED" {
		return nil
	}

	item := parseDidlItem(didlXML)
	if item == nil {
		return nil
	}

	metadata := &TrackMetadata{
		Title:       firstNonEmpty(item.Title, "Unknown"),
		Artist:      item.Artist,
		Album:       item.Album,
		AlbumArtURI: item.AlbumArtURI,
		Source:      determineSource(trackURI, item.UpnpClass),
		ServiceName: detectServiceName(trackURI, didlXML),
	}

	return metadata
}

// ParseContainerMetadata parses DIDL-Lite container info.
func ParseContainerMetadata(didlXML string) *ContainerMetadata {
	if strings.TrimSpace(didlXML) == "" || didlXML == "NOT_IMPLEMENTED" {
		return nil
	}

	item := parseDidlItem(didlXML)
	if item == nil {
		return nil
	}

	containerType := "unknown"
	upnpClass := item.UpnpClass
	switch {
	case strings.Contains(upnpClass, "audioBroadcast") || strings.Contains(upnpClass, "radio"):
		containerType = "station"
	case strings.Contains(upnpClass, "playlistContainer") || strings.Contains(upnpClass, "playlist"):
		containerType = "playlist"
	case strings.Contains(upnpClass, "album") || strings.Contains(upnpClass, "musicAlbum"):
		containerType = "album"
	case strings.Contains(upnpClass, "musicTrack") || strings.Contains(upnpClass, "audioItem"):
		containerType = "track"
	case strings.Contains(upnpClass, "storageFolder") || strings.Contains(upnpClass, "container"):
		containerType = "queue"
	}

	return &ContainerMetadata{
		Name: item.Title,
		Type: containerType,
	}
}

// ParseDuration parses a HH:MM:SS string to seconds.
func ParseDuration(duration string) int {
	if duration == "" || duration == "NOT_IMPLEMENTED" {
		return 0
	}
	parts := strings.Split(duration, ":")
	if len(parts) != 3 {
		return 0
	}
	hours := parseInt(parts[0])
	minutes := parseInt(parts[1])
	seconds := parseInt(parts[2])
	return (hours * 3600) + (minutes * 60) + seconds
}

// GetServiceLogoFromName returns a static logo path for known service names.
func GetServiceLogoFromName(serviceName string) string {
	name := strings.ToLower(serviceName)

	switch {
	case strings.Contains(name, "spotify"):
		return "/v1/assets/service-logos/spotify.png"
	case strings.Contains(name, "apple music") || name == "apple":
		return "/v1/assets/service-logos/apple-music.png"
	case strings.Contains(name, "amazon"):
		return "/v1/assets/service-logos/amazon-music.png"
	case strings.Contains(name, "tunein") || strings.Contains(name, "radiotime"):
		return "/v1/assets/service-logos/tunein.png"
	case strings.Contains(name, "pandora"):
		return "/v1/assets/service-logos/pandora.png"
	case strings.Contains(name, "deezer"):
		return "/v1/assets/service-logos/deezer.png"
	case strings.Contains(name, "tidal"):
		return "/v1/assets/service-logos/tidal.png"
	case strings.Contains(name, "soundcloud"):
		return "/v1/assets/service-logos/soundcloud.png"
	case strings.Contains(name, "youtube"):
		return "/v1/assets/service-logos/youtube-music.png"
	case strings.Contains(name, "audible"):
		return "/v1/assets/service-logos/audible.png"
	case strings.Contains(name, "plex"):
		return "/v1/assets/service-logos/plex.png"
	case strings.Contains(name, "sonos radio") || strings.Contains(name, "sonos"):
		return "/v1/assets/service-logos/sonos-radio.png"
	default:
		return ""
	}
}

type didlItem struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtURI string
	UpnpClass   string
}

func parseDidlItem(didlXML string) *didlItem {
	decoder := xml.NewDecoder(bytes.NewReader([]byte(didlXML)))
	var currentElement string
	var inItem bool
	item := &didlItem{}

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch elem := token.(type) {
		case xml.StartElement:
			local := elem.Name.Local
			if local == "item" || local == "container" {
				inItem = true
				continue
			}
			if inItem {
				currentElement = local
			}
		case xml.EndElement:
			if inItem {
				currentElement = ""
				if elem.Name.Local == "item" || elem.Name.Local == "container" {
					inItem = false
					if item.Title != "" || item.Artist != "" || item.Album != "" || item.AlbumArtURI != "" || item.UpnpClass != "" {
						return item
					}
				}
			}
		case xml.CharData:
			if !inItem {
				continue
			}
			value := strings.TrimSpace(string(elem))
			if value == "" {
				continue
			}
			switch currentElement {
			case "title":
				if item.Title == "" {
					item.Title = value
				}
			case "creator":
				if item.Artist == "" {
					item.Artist = value
				}
			case "albumArtist":
				if item.Artist == "" {
					item.Artist = value
				}
			case "artist":
				if item.Artist == "" {
					item.Artist = value
				}
			case "album":
				if item.Album == "" {
					item.Album = value
				}
			case "albumArtURI":
				if item.AlbumArtURI == "" {
					item.AlbumArtURI = value
				}
			case "class":
				if item.UpnpClass == "" {
					item.UpnpClass = value
				}
			}
		}
	}

	if item.Title == "" && item.Artist == "" && item.Album == "" && item.AlbumArtURI == "" && item.UpnpClass == "" {
		return nil
	}
	return item
}

func determineSource(trackURI string, upnpClass string) string {
	uri := strings.ToLower(trackURI)

	switch {
	case strings.Contains(uri, "x-sonos-http:song%3a") || strings.Contains(uri, "apple"):
		return "apple_music"
	case strings.Contains(uri, "x-sonos-htastream") || strings.Contains(uri, "spdif"):
		return "tv"
	case strings.Contains(uri, "x-rincon-stream") && strings.Contains(uri, "linein"):
		return "line_in"
	case strings.Contains(uri, "x-sonosapi-radio") ||
		strings.Contains(uri, "x-sonosapi-stream") ||
		strings.Contains(uri, "x-sonosapi-hls-static") ||
		strings.Contains(uri, "x-rincon-cpcontainer") ||
		strings.Contains(uri, "x-sonos-favorite"):
		return "sonos_favorite"
	case strings.Contains(upnpClass, "audioItem.audioBroadcast"):
		return "sonos_favorite"
	default:
		return "unknown"
	}
}

func detectServiceName(trackURI string, metadata string) string {
	uri := strings.ToLower(trackURI)
	meta := strings.ToLower(metadata)

	switch {
	case strings.Contains(uri, "spotify") || strings.Contains(meta, "spotify"):
		return "Spotify"
	case strings.Contains(uri, "apple") ||
		strings.Contains(meta, "apple") ||
		strings.Contains(uri, "music.apple.com") ||
		strings.Contains(meta, "com.apple.music") ||
		strings.Contains(uri, "x-sonos-http:song%3a"):
		return "Apple Music"
	case strings.Contains(uri, "amazon") ||
		strings.Contains(meta, "amazon") ||
		strings.Contains(uri, "amzn") ||
		strings.Contains(meta, "amzn") ||
		strings.Contains(uri, "prime") ||
		strings.Contains(meta, "prime"):
		return "Amazon Music"
	case strings.Contains(uri, "tunein") ||
		strings.Contains(meta, "tunein") ||
		strings.Contains(uri, "radiotime") ||
		strings.Contains(meta, "radiotime"):
		return "TuneIn"
	case strings.Contains(uri, "pandora") || strings.Contains(meta, "pandora"):
		return "Pandora"
	case strings.Contains(uri, "deezer") || strings.Contains(meta, "deezer"):
		return "Deezer"
	case strings.Contains(uri, "tidal") ||
		strings.Contains(meta, "tidal") ||
		strings.Contains(uri, "wimp") ||
		strings.Contains(meta, "wimp"):
		return "Tidal"
	case strings.Contains(uri, "soundcloud") || strings.Contains(meta, "soundcloud"):
		return "SoundCloud"
	case strings.Contains(uri, "youtube") || strings.Contains(meta, "youtube"):
		return "YouTube Music"
	case strings.Contains(uri, "audible") || strings.Contains(meta, "audible"):
		return "Audible"
	case strings.Contains(uri, "plex") || strings.Contains(meta, "plex"):
		return "Plex"
	case strings.Contains(uri, "sonos-radio") ||
		strings.Contains(meta, "sonos radio") ||
		strings.Contains(meta, "sa_rincon77575"):
		return "Sonos Radio"
	default:
		return ""
	}
}

func parseInt(value string) int {
	parsed := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0
		}
		parsed = (parsed * 10) + int(ch-'0')
	}
	return parsed
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
