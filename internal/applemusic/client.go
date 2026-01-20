package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Apple Music API.
type Client struct {
	tokenManager *TokenManager
	httpClient   *http.Client
	baseURL      string
	storefront   string
}

// ClientConfig holds configuration for creating a Client.
type ClientConfig struct {
	TokenManager *TokenManager // Required: manages developer tokens
	BaseURL      string        // Optional: defaults to https://api.music.apple.com
	Storefront   string        // Optional: defaults to "us"
	Timeout      time.Duration // Optional: HTTP timeout, defaults to 10s
}

// NewClient creates a new Apple Music API client.
func NewClient(cfg ClientConfig) *Client {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.music.apple.com"
	}

	storefront := cfg.Storefront
	if storefront == "" {
		storefront = "us"
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		tokenManager: cfg.TokenManager,
		httpClient:   &http.Client{Timeout: timeout},
		baseURL:      baseURL,
		storefront:   storefront,
	}
}

// SearchResult represents the normalized search results for our API.
type SearchResult struct {
	Results    map[string][]APISearchResult `json:"results"`
	Pagination Pagination                   `json:"pagination"`
}

// Pagination contains pagination information.
type Pagination struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// Search performs a search on the Apple Music catalog.
// types should be comma-separated: "songs,albums,playlists"
func (c *Client) Search(ctx context.Context, query string, types string, limit, offset int) (*SearchResult, error) {
	if query == "" {
		return &SearchResult{
			Results:    make(map[string][]APISearchResult),
			Pagination: Pagination{Limit: limit, Offset: offset},
		}, nil
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/v1/catalog/%s/search", c.baseURL, c.storefront)

	// Build query parameters
	params := url.Values{}
	params.Set("term", query)
	if types != "" {
		params.Set("types", types)
	} else {
		params.Set("types", "songs,albums,artists,playlists")
	}
	params.Set("limit", fmt.Sprintf("%d", limit))
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}

	fullURL := endpoint + "?" + params.Encode()

	// Make request
	resp, err := c.doRequest(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Transform to our API format
	return c.transformSearchResults(&searchResp, limit, offset), nil
}

// SuggestionsResult represents the normalized suggestions for our API.
type SuggestionsResult struct {
	Terms      []APISuggestion `json:"terms"`
	TopResults []APITopResult  `json:"top_results"`
}

// GetSuggestions fetches search suggestions from Apple Music.
// types should be comma-separated for top results filtering.
func (c *Client) GetSuggestions(ctx context.Context, query string, types string, limit int) (*SuggestionsResult, error) {
	if query == "" {
		return &SuggestionsResult{
			Terms:      []APISuggestion{},
			TopResults: []APITopResult{},
		}, nil
	}

	// Build URL
	endpoint := fmt.Sprintf("%s/v1/catalog/%s/search/suggestions", c.baseURL, c.storefront)

	// Build query parameters
	params := url.Values{}
	params.Set("term", query)
	params.Set("kinds", "terms,topResults")
	if types != "" {
		params.Set("types", types)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	fullURL := endpoint + "?" + params.Encode()

	// Make request
	resp, err := c.doRequest(ctx, fullURL)
	if err != nil {
		return nil, fmt.Errorf("suggestions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("suggestions failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var suggestResp SuggestionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&suggestResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Transform to our API format
	return c.transformSuggestions(&suggestResp), nil
}

// doRequest makes an authenticated HTTP request to the Apple Music API.
func (c *Client) doRequest(ctx context.Context, url string) (*http.Response, error) {
	token, err := c.tokenManager.GetToken()
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// transformSearchResults converts Apple Music API response to our format.
func (c *Client) transformSearchResults(resp *SearchResponse, limit, offset int) *SearchResult {
	results := make(map[string][]APISearchResult)
	total := 0

	// Transform songs
	if resp.Results.Songs != nil && len(resp.Results.Songs.Data) > 0 {
		songs := make([]APISearchResult, 0, len(resp.Results.Songs.Data))
		for _, r := range resp.Results.Songs.Data {
			songs = append(songs, c.transformResource(&r, "tracks"))
		}
		results["tracks"] = songs
		total += len(songs)
	}

	// Transform albums
	if resp.Results.Albums != nil && len(resp.Results.Albums.Data) > 0 {
		albums := make([]APISearchResult, 0, len(resp.Results.Albums.Data))
		for _, r := range resp.Results.Albums.Data {
			albums = append(albums, c.transformResource(&r, "albums"))
		}
		results["albums"] = albums
		total += len(albums)
	}

	// Transform artists
	if resp.Results.Artists != nil && len(resp.Results.Artists.Data) > 0 {
		artists := make([]APISearchResult, 0, len(resp.Results.Artists.Data))
		for _, r := range resp.Results.Artists.Data {
			artists = append(artists, c.transformResource(&r, "artists"))
		}
		results["artists"] = artists
		total += len(artists)
	}

	// Transform playlists
	if resp.Results.Playlists != nil && len(resp.Results.Playlists.Data) > 0 {
		playlists := make([]APISearchResult, 0, len(resp.Results.Playlists.Data))
		for _, r := range resp.Results.Playlists.Data {
			playlists = append(playlists, c.transformResource(&r, "playlists"))
		}
		results["playlists"] = playlists
		total += len(playlists)
	}

	// Transform stations
	if resp.Results.Stations != nil && len(resp.Results.Stations.Data) > 0 {
		stations := make([]APISearchResult, 0, len(resp.Results.Stations.Data))
		for _, r := range resp.Results.Stations.Data {
			stations = append(stations, c.transformResource(&r, "stations"))
		}
		results["stations"] = stations
		total += len(stations)
	}

	return &SearchResult{
		Results: results,
		Pagination: Pagination{
			Limit:  limit,
			Offset: offset,
			Total:  total,
		},
	}
}

// transformResource converts a single Apple Music resource to our API format.
func (c *Client) transformResource(r *Resource, contentType string) APISearchResult {
	result := APISearchResult{
		ID:          r.ID,
		Name:        r.Attributes.Name,
		ContentType: contentType,
		Provider:    "apple_music",
	}

	// Set artist name if present
	if r.Attributes.ArtistName != "" {
		artistName := r.Attributes.ArtistName
		result.ArtistName = &artistName
	}

	// Set album name if present
	if r.Attributes.AlbumName != "" {
		albumName := r.Attributes.AlbumName
		result.AlbumName = &albumName
	}

	// Set curator name for playlists
	if r.Attributes.CuratorName != "" {
		curatorName := r.Attributes.CuratorName
		result.CuratorName = &curatorName
	}

	// Set artwork URL (300x300 is a good default)
	if r.Attributes.Artwork != nil {
		artworkURL := r.Attributes.Artwork.GetArtworkURL(300, 300)
		if artworkURL != "" {
			result.ArtworkURL = &artworkURL
		}
	}

	// Set playback URI (Apple Music URL for now, could be enhanced for direct playback)
	if r.Attributes.URL != "" {
		result.PlaybackURI = &r.Attributes.URL
	}

	// Set duration for tracks
	if r.Attributes.DurationMs > 0 {
		durationMs := r.Attributes.DurationMs
		result.DurationMs = &durationMs
	}

	return result
}

// transformSuggestions converts Apple Music suggestions to our API format.
func (c *Client) transformSuggestions(resp *SuggestionsResponse) *SuggestionsResult {
	result := &SuggestionsResult{
		Terms:      []APISuggestion{},
		TopResults: []APITopResult{},
	}

	for _, s := range resp.Results.Suggestions {
		switch s.Kind {
		case "terms":
			term := APISuggestion{
				Term:        s.SearchTerm,
				DisplayTerm: s.DisplayTerm,
			}
			if term.DisplayTerm == "" {
				term.DisplayTerm = s.SearchTerm
			}
			result.Terms = append(result.Terms, term)

		case "topResults":
			if s.Content != nil {
				topResult := APITopResult{
					ID:          s.Content.ID,
					Name:        s.Content.Attributes.Name,
					ContentType: mapAppleTypeToContentType(s.Content.Type),
				}
				if s.Content.Attributes.ArtistName != "" {
					artistName := s.Content.Attributes.ArtistName
					topResult.ArtistName = &artistName
				}
				if s.Content.Attributes.Artwork != nil {
					artworkURL := s.Content.Attributes.Artwork.GetArtworkURL(300, 300)
					if artworkURL != "" {
						topResult.ArtworkURL = &artworkURL
					}
				}
				result.TopResults = append(result.TopResults, topResult)
			}
		}
	}

	return result
}

// mapAppleTypeToContentType converts Apple Music resource types to our content types.
func mapAppleTypeToContentType(appleType string) string {
	// Apple uses singular form (song, album), we use plural (tracks, albums)
	switch strings.TrimSuffix(appleType, "s") {
	case "song":
		return "tracks"
	case "album":
		return "albums"
	case "artist":
		return "artists"
	case "playlist":
		return "playlists"
	case "station":
		return "stations"
	default:
		return appleType
	}
}
