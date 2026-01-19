package music

import "time"

// SelectionPolicy determines how items are selected from a music set.
type SelectionPolicy string

const (
	SelectionPolicyRotation SelectionPolicy = "ROTATION"
	SelectionPolicyShuffle  SelectionPolicy = "SHUFFLE"
)

// ContentType represents the source type of music content.
type ContentType string

const (
	ContentTypeSonosFavorite ContentType = "sonos_favorite"
	ContentTypeAppleMusic    ContentType = "apple_music"
	ContentTypeDirect        ContentType = "direct"
)

// QueueMode determines how music content is queued.
type QueueMode string

const (
	QueueModeReplaceAndPlay QueueMode = "REPLACE_AND_PLAY"
	QueueModePlayNext       QueueMode = "PLAY_NEXT"
	QueueModeAddToEnd       QueueMode = "ADD_TO_END"
	QueueModeQueueOnly      QueueMode = "QUEUE_ONLY"
)

// MusicSet represents a collection of music items that can be played together.
type MusicSet struct {
	SetID           string    `json:"set_id"`
	Name            string    `json:"name"`
	SelectionPolicy string    `json:"selection_policy"`
	CurrentIndex    int       `json:"current_index"`
	OccasionStart   *string   `json:"occasion_start,omitempty"` // MM-DD format
	OccasionEnd     *string   `json:"occasion_end,omitempty"`   // MM-DD format
	ItemCount       int       `json:"item_count,omitempty"`     // Computed field
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SetItem represents a single item within a music set.
type SetItem struct {
	SetID           string    `json:"set_id"`
	SonosFavoriteID string    `json:"sonos_favorite_id"`
	Position        int       `json:"position"`
	ServiceLogoURL  *string   `json:"service_logo_url,omitempty"`
	ServiceName     *string   `json:"service_name,omitempty"`
	ContentType     string    `json:"content_type"`
	ContentJSON     *string   `json:"content_json,omitempty"`
	AddedAt         time.Time `json:"added_at"`
}

// PlayHistory represents a record of when a music item was played.
type PlayHistory struct {
	ID              int64     `json:"id"`
	SonosFavoriteID string    `json:"sonos_favorite_id"`
	SetID           *string   `json:"set_id,omitempty"`
	RoutineID       *string   `json:"routine_id,omitempty"`
	PlayedAt        time.Time `json:"played_at"`
}

// ContentMetadata contains extended content metadata stored in content_json.
type ContentMetadata struct {
	Title        string `json:"title,omitempty"`
	Artist       string `json:"artist,omitempty"`
	Album        string `json:"album,omitempty"`
	ArtworkURL   string `json:"artwork_url,omitempty"`
	URI          string `json:"uri,omitempty"`
	Metadata     string `json:"metadata,omitempty"`      // Sonos DIDL metadata
	AppleMusicID string `json:"apple_music_id,omitempty"`
}

// SelectionResult contains the result of selecting an item from a music set.
type SelectionResult struct {
	Item        *SetItem `json:"item"`
	NextIndex   int      `json:"next_index"`
	WasShuffled bool     `json:"was_shuffled"`
}

// CreateSetInput contains the input for creating a music set.
type CreateSetInput struct {
	Name            string  `json:"name"`
	SelectionPolicy string  `json:"selection_policy"`
	OccasionStart   *string `json:"occasion_start,omitempty"` // MM-DD format
	OccasionEnd     *string `json:"occasion_end,omitempty"`   // MM-DD format
}

// UpdateSetInput contains the input for updating a music set.
type UpdateSetInput struct {
	Name            *string `json:"name,omitempty"`
	SelectionPolicy *string `json:"selection_policy,omitempty"`
	OccasionStart   *string `json:"occasion_start,omitempty"` // MM-DD format, nil to keep, empty string to clear
	OccasionEnd     *string `json:"occasion_end,omitempty"`   // MM-DD format, nil to keep, empty string to clear
}

// AddItemInput contains the input for adding an item to a music set.
type AddItemInput struct {
	SonosFavoriteID string  `json:"sonos_favorite_id"`
	ServiceLogoURL  *string `json:"service_logo_url,omitempty"`
	ServiceName     *string `json:"service_name,omitempty"`
	ContentType     string  `json:"content_type,omitempty"`
	ContentJSON     *string `json:"content_json,omitempty"`
}

// ReorderItemsInput contains the input for reordering items in a music set.
type ReorderItemsInput struct {
	Items []string `json:"items"` // Ordered list of sonos_favorite_ids
}

// PlaySetInput contains the input for playing a music set on a device.
type PlaySetInput struct {
	DeviceID  string `json:"device_id"`
	SpeakerID string `json:"speaker_id"` // Node.js uses speaker_id
	Volume    *int   `json:"volume,omitempty"`
	QueueMode string `json:"queue_mode,omitempty"`
}

// SelectItemInput contains the input for selecting an item from a music set.
type SelectItemInput struct {
	NoRepeatWindowMinutes *int `json:"no_repeat_window_minutes,omitempty"`
}

// MusicContent represents content that can be added to a music set.
// This is the iOS-compatible format used by the /content endpoints.
type MusicContent struct {
	Type        string  `json:"type"`                   // "sonos_favorite", "apple_music", "direct"
	FavoriteID  *string `json:"favorite_id,omitempty"`  // For sonos_favorite type
	Service     *string `json:"service,omitempty"`      // "spotify", "apple_music"
	ContentType *string `json:"content_type,omitempty"` // "playlist", "album", "track", "station"
	ContentID   *string `json:"content_id,omitempty"`   // Service-specific ID
}

// AddContentInput contains the input for adding content to a music set.
// Used by POST /v1/music/sets/{setId}/content (iOS app format).
type AddContentInput struct {
	MusicContent   MusicContent `json:"music_content"`
	ServiceLogoURL *string      `json:"service_logo_url,omitempty"`
	ServiceName    *string      `json:"service_name,omitempty"`
	DisplayName    *string      `json:"display_name,omitempty"`
	ArtworkURL     *string      `json:"artwork_url,omitempty"`
}
