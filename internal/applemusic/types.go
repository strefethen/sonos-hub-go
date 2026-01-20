package applemusic

// SearchResponse represents the Apple Music API search response.
// Apple returns results grouped by type under the "results" key.
type SearchResponse struct {
	Results SearchResults `json:"results"`
}

// SearchResults contains arrays of resources grouped by type.
type SearchResults struct {
	Songs     *ResourceResponse `json:"songs,omitempty"`
	Albums    *ResourceResponse `json:"albums,omitempty"`
	Artists   *ResourceResponse `json:"artists,omitempty"`
	Playlists *ResourceResponse `json:"playlists,omitempty"`
	Stations  *ResourceResponse `json:"stations,omitempty"`
}

// ResourceResponse wraps an array of resources with pagination info.
type ResourceResponse struct {
	Data []Resource `json:"data"`
	Href string     `json:"href,omitempty"`
	Next string     `json:"next,omitempty"`
}

// Resource represents a single Apple Music resource (song, album, playlist, etc.).
type Resource struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Href       string     `json:"href"`
	Attributes Attributes `json:"attributes"`
}

// Attributes contains the metadata for a resource.
type Attributes struct {
	// Common fields
	Name        string  `json:"name"`
	ArtistName  string  `json:"artistName,omitempty"`
	Artwork     *Artwork `json:"artwork,omitempty"`
	URL         string  `json:"url,omitempty"`
	PlayParams  *PlayParams `json:"playParams,omitempty"`

	// Track-specific
	AlbumName   string `json:"albumName,omitempty"`
	DurationMs  int    `json:"durationInMillis,omitempty"`
	TrackNumber int    `json:"trackNumber,omitempty"`
	DiscNumber  int    `json:"discNumber,omitempty"`

	// Album-specific
	TrackCount      int    `json:"trackCount,omitempty"`
	ReleaseDate     string `json:"releaseDate,omitempty"`
	RecordLabel     string `json:"recordLabel,omitempty"`

	// Playlist-specific
	Description     *EditorialNotes `json:"description,omitempty"`
	CuratorName     string          `json:"curatorName,omitempty"`
	LastModifiedDate string         `json:"lastModifiedDate,omitempty"`

	// Station-specific
	IsLive          bool   `json:"isLive,omitempty"`
	EditorialNotes  *EditorialNotes `json:"editorialNotes,omitempty"`
}

// Artwork represents image artwork with dimensions.
type Artwork struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	URL        string `json:"url"`
	BgColor    string `json:"bgColor,omitempty"`
	TextColor1 string `json:"textColor1,omitempty"`
	TextColor2 string `json:"textColor2,omitempty"`
	TextColor3 string `json:"textColor3,omitempty"`
	TextColor4 string `json:"textColor4,omitempty"`
}

// PlayParams contains playback parameters for a resource.
type PlayParams struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// EditorialNotes contains text descriptions.
type EditorialNotes struct {
	Standard string `json:"standard,omitempty"`
	Short    string `json:"short,omitempty"`
}

// SuggestionsResponse represents the Apple Music suggestions API response.
type SuggestionsResponse struct {
	Results SuggestionResults `json:"results"`
}

// SuggestionResults contains suggestion data.
type SuggestionResults struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion represents a single search suggestion.
type Suggestion struct {
	Kind       string    `json:"kind"` // "terms" or "topResults"
	SearchTerm string    `json:"searchTerm,omitempty"`
	DisplayTerm string   `json:"displayTerm,omitempty"`
	Content    *Resource `json:"content,omitempty"` // For topResults
}

// APISearchResult represents our normalized search result format.
type APISearchResult struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ContentType string  `json:"content_type"`
	Provider    string  `json:"provider"`
	ArtistName  *string `json:"artist_name,omitempty"`
	AlbumName   *string `json:"album_name,omitempty"`
	ArtworkURL  *string `json:"artwork_url,omitempty"`
	PlaybackURI *string `json:"playback_uri,omitempty"`
	DurationMs  *int    `json:"duration_ms,omitempty"`
	CuratorName *string `json:"curator_name,omitempty"`
}

// APISuggestion represents our normalized suggestion format.
type APISuggestion struct {
	Term        string `json:"term"`
	DisplayTerm string `json:"display_term"`
}

// APITopResult represents a top result suggestion.
type APITopResult struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ContentType string  `json:"content_type"`
	ArtistName  *string `json:"artist_name,omitempty"`
	ArtworkURL  *string `json:"artwork_url,omitempty"`
}

// GetArtworkURL returns the artwork URL with specified dimensions.
// Apple Music artwork URLs contain {w}x{h} placeholders.
func (a *Artwork) GetArtworkURL(width, height int) string {
	if a == nil || a.URL == "" {
		return ""
	}
	url := a.URL
	url = replaceSize(url, "{w}", width)
	url = replaceSize(url, "{h}", height)
	return url
}

// replaceSize replaces a placeholder with a dimension value.
func replaceSize(url, placeholder string, size int) string {
	result := ""
	for i := 0; i < len(url); {
		if i+len(placeholder) <= len(url) && url[i:i+len(placeholder)] == placeholder {
			result += itoa(size)
			i += len(placeholder)
		} else {
			result += string(url[i])
			i++
		}
	}
	return result
}

// itoa converts an integer to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
