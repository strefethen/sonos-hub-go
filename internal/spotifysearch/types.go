package spotifysearch

// SpotifyContentType represents searchable content types
type SpotifyContentType string

const (
	ContentTypeAlbums     SpotifyContentType = "albums"
	ContentTypeArtists    SpotifyContentType = "artists"
	ContentTypeTracks     SpotifyContentType = "tracks"
	ContentTypePlaylists  SpotifyContentType = "playlists"
	ContentTypeGenres     SpotifyContentType = "genres"
	ContentTypeAudiobooks SpotifyContentType = "audiobooks"
	ContentTypePodcasts   SpotifyContentType = "podcasts"
)

// AllContentTypes returns all supported content types for search
func AllContentTypes() []SpotifyContentType {
	return []SpotifyContentType{
		ContentTypeAlbums,
		ContentTypeArtists,
		ContentTypeTracks,
		ContentTypePlaylists,
		ContentTypeGenres,
		ContentTypeAudiobooks,
		ContentTypePodcasts,
	}
}

// --- Outgoing messages (server → extension) ---

// SearchRequest is sent to the extension to initiate a search
type SearchRequest struct {
	Type         string               `json:"type"` // "search"
	RequestID    string               `json:"requestId"`
	Query        string               `json:"query"`
	ContentTypes []SpotifyContentType `json:"contentTypes"`
}

// PingMessage is sent to keep the connection alive
type PingMessage struct {
	Type string `json:"type"` // "ping"
}

// --- Incoming messages (extension → server) ---

// IncomingMessage is the base structure for messages from the extension
type IncomingMessage struct {
	Type      string `json:"type"` // "searchResult" or "pong"
	RequestID string `json:"requestId,omitempty"`
}

// SearchResultMessage contains search results from the extension
type SearchResultMessage struct {
	Type      string               `json:"type"`
	RequestID string               `json:"requestId"`
	Results   GroupedSearchResults `json:"results"`
	Error     string               `json:"error,omitempty"`
}

// GroupedSearchResults contains search results grouped by content type
type GroupedSearchResults struct {
	Albums     []SpotifyAlbum     `json:"albums,omitempty"`
	Artists    []SpotifyArtist    `json:"artists,omitempty"`
	Tracks     []SpotifyTrack     `json:"tracks,omitempty"`
	Playlists  []SpotifyPlaylist  `json:"playlists,omitempty"`
	Genres     []SpotifyGenre     `json:"genres,omitempty"`
	Audiobooks []SpotifyAudiobook `json:"audiobooks,omitempty"`
	Podcasts   []SpotifyPodcast   `json:"podcasts,omitempty"`
}

// SpotifyTrack represents a track from Spotify search results
type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URI        string `json:"uri"`
	ImageURL   string `json:"imageUrl"`
	ArtistName string `json:"artistName"`
	AlbumName  string `json:"albumName"`
	DurationMs int    `json:"durationMs"`
}

// SpotifyAlbum represents an album from Spotify search results
type SpotifyAlbum struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URI        string `json:"uri"`
	ImageURL   string `json:"imageUrl"`
	ArtistName string `json:"artistName"`
}

// SpotifyArtist represents an artist from Spotify search results
type SpotifyArtist struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URI      string `json:"uri"`
	ImageURL string `json:"imageUrl"`
}

// SpotifyPlaylist represents a playlist from Spotify search results
type SpotifyPlaylist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URI         string `json:"uri"`
	ImageURL    string `json:"imageUrl"`
	OwnerName   string `json:"ownerName"`
	Description string `json:"description,omitempty"`
}

// SpotifyGenre represents a genre from Spotify search results
type SpotifyGenre struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URI      string `json:"uri"`
	ImageURL string `json:"imageUrl"`
}

// SpotifyAudiobook represents an audiobook from Spotify search results
type SpotifyAudiobook struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	URI        string `json:"uri"`
	ImageURL   string `json:"imageUrl"`
	AuthorName string `json:"authorName"`
}

// SpotifyPodcast represents a podcast from Spotify search results
type SpotifyPodcast struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URI           string `json:"uri"`
	ImageURL      string `json:"imageUrl"`
	PublisherName string `json:"publisherName"`
}

// ConnectionStatus represents the extension connection state
type ConnectionStatus struct {
	Extension       string `json:"extension"` // "connected" or "disconnected"
	PendingSearches int    `json:"pendingSearches"`
}
