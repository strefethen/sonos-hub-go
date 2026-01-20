package music

import (
	"context"
	"strconv"
	"strings"

	"github.com/strefethen/sonos-hub-go/internal/devices"
	"github.com/strefethen/sonos-hub-go/internal/sonos/soap"
)

// =============================================================================
// Library Provider Types
// =============================================================================

// LibraryContentType maps API content types to SOAP library types.
// Only albums, artists, and tracks are supported by the Sonos Music Library.
var libraryContentTypeMap = map[string]soap.MusicLibraryContentType{
	"albums":  soap.MusicLibraryAlbum,
	"artists": soap.MusicLibraryArtist,
	"tracks":  soap.MusicLibraryTrack,
}

// LibrarySearchResult represents results from a library search.
type LibrarySearchResult struct {
	Provider   string                    `json:"provider"`
	Query      string                    `json:"query"`
	Results    map[string][]LibraryItem  `json:"results"`
	Pagination LibrarySearchPagination   `json:"pagination"`
}

// LibrarySearchPagination contains pagination info.
type LibrarySearchPagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// LibraryItem represents a music library item in API format.
type LibraryItem struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ContentType string  `json:"content_type"`
	Provider    string  `json:"provider"`
	ArtistName  *string `json:"artist_name,omitempty"`
	AlbumName   *string `json:"album_name,omitempty"`
	ArtworkURL  *string `json:"artwork_url,omitempty"`
	PlaybackURI *string `json:"playback_uri,omitempty"`
	DurationMs  *int    `json:"duration_ms,omitempty"`
}

// =============================================================================
// Library Provider
// =============================================================================

// LibraryProvider searches the Sonos Music Library via UPnP ContentDirectory.
type LibraryProvider struct {
	soapClient    *soap.Client
	deviceService *devices.Service
}

// NewLibraryProvider creates a new music library provider.
func NewLibraryProvider(soapClient *soap.Client, deviceService *devices.Service) *LibraryProvider {
	return &LibraryProvider{
		soapClient:    soapClient,
		deviceService: deviceService,
	}
}

// Search searches the music library for content matching the query.
// types is a slice of content type strings: "albums", "artists", "tracks"
func (p *LibraryProvider) Search(ctx context.Context, query string, types []string, limit, offset int) (*LibrarySearchResult, error) {
	// Get a device IP to query
	deviceIP := p.getDeviceIP()
	if deviceIP == "" {
		// No devices available - return empty results
		return &LibrarySearchResult{
			Provider: "library",
			Query:    query,
			Results:  map[string][]LibraryItem{},
			Pagination: LibrarySearchPagination{
				Limit:  limit,
				Offset: offset,
				Total:  0,
			},
		}, nil
	}

	// If no types specified, use defaults
	if len(types) == 0 {
		types = []string{"albums", "artists", "tracks"}
	}

	// Distribute limit across types
	perTypeLimit := limit
	if len(types) > 1 {
		perTypeLimit = (limit + len(types) - 1) / len(types) // ceil division
	}

	results := make(map[string][]LibraryItem)
	totalItems := 0

	for _, contentType := range types {
		libraryType, ok := libraryContentTypeMap[contentType]
		if !ok {
			// Type not supported (e.g., stations, podcasts)
			continue
		}

		browseResult, err := p.soapClient.SearchMusicLibrary(
			ctx,
			deviceIP,
			libraryType,
			query,
			offset,
			perTypeLimit,
		)
		if err != nil {
			// Log but continue with other types
			continue
		}

		if len(browseResult.Items) > 0 {
			items := make([]LibraryItem, 0, len(browseResult.Items))
			for _, item := range browseResult.Items {
				items = append(items, p.convertToLibraryItem(item, contentType))
			}
			results[contentType] = items
			totalItems += browseResult.TotalMatches
		}
	}

	return &LibrarySearchResult{
		Provider: "library",
		Query:    query,
		Results:  results,
		Pagination: LibrarySearchPagination{
			Limit:  limit,
			Offset: offset,
			Total:  totalItems,
		},
	}, nil
}

// getDeviceIP returns an IP address of a Sonos device to query.
func (p *LibraryProvider) getDeviceIP() string {
	if p.deviceService == nil {
		return ""
	}

	devices, err := p.deviceService.GetDevices()
	if err != nil || len(devices) == 0 {
		return ""
	}

	// Return the first device's IP
	return devices[0].IP
}

// convertToLibraryItem converts a SOAP MusicLibraryItem to the API LibraryItem format.
func (p *LibraryProvider) convertToLibraryItem(item soap.MusicLibraryItem, contentType string) LibraryItem {
	result := LibraryItem{
		ID:          item.ID,
		Name:        item.Title,
		ContentType: contentType,
		Provider:    "library",
	}

	if item.ArtistName != "" {
		result.ArtistName = &item.ArtistName
	}
	if item.AlbumName != "" {
		result.AlbumName = &item.AlbumName
	}
	if item.AlbumArtURI != "" {
		result.ArtworkURL = &item.AlbumArtURI
	}
	if item.Resource != "" {
		result.PlaybackURI = &item.Resource
	}
	if item.Duration != "" {
		if durationMs := parseDuration(item.Duration); durationMs > 0 {
			result.DurationMs = &durationMs
		}
	}

	return result
}

// parseDuration parses duration string (HH:MM:SS or H:MM:SS) to milliseconds.
func parseDuration(duration string) int {
	parts := strings.Split(duration, ":")
	if len(parts) != 3 {
		return 0
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])
	seconds, _ := strconv.Atoi(parts[2])

	return (hours*3600 + minutes*60 + seconds) * 1000
}
