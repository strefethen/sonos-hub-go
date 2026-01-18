package settings

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// TVRoutingSettings holds TV routing configuration.
type TVRoutingSettings struct {
	ArcTVPolicy       string   `json:"arc_tv_policy"`
	FallbackDeviceID  *string  `json:"fallback_device_id,omitempty"`
	FallbackRooms     []string `json:"fallback_rooms,omitempty"`
	AlwaysSkipOnTV    bool     `json:"always_skip_on_tv"`
	RetryOnTVTimeout  int      `json:"retry_on_tv_timeout_seconds"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// Service provides settings management functionality.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type Service struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
	logger *log.Logger
}

// NewService creates a new settings service.
// Accepts a DBPair for optimal SQLite concurrency with separate reader/writer pools.
func NewService(dbPair DBPair, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		reader: dbPair.Reader(),
		writer: dbPair.Writer(),
		logger: logger,
	}
}

// RegisterRoutes wires settings routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Method(http.MethodGet, "/v1/settings/tv-routing", api.Handler(getTVRoutingSettings(service)))
	router.Method(http.MethodPut, "/v1/settings/tv-routing", api.Handler(updateTVRoutingSettings(service)))
}

// getTVRoutingSettings handles GET /v1/settings/tv-routing
func getTVRoutingSettings(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		settings, err := service.GetTVRoutingSettings()
		if err != nil {
			return apperrors.NewInternalError("Failed to get TV routing settings")
		}

		return api.SingleResponse(w, r, http.StatusOK, "settings", formatTVRoutingSettings(settings))
	}
}

// UpdateTVRoutingInput represents the request body for updating TV routing settings.
type UpdateTVRoutingInput struct {
	ArcTVPolicy      *string  `json:"arc_tv_policy,omitempty"`
	FallbackDeviceID *string  `json:"fallback_device_id,omitempty"`
	FallbackRooms    []string `json:"fallback_rooms,omitempty"`
	AlwaysSkipOnTV   *bool    `json:"always_skip_on_tv,omitempty"`
	RetryOnTVTimeout *int     `json:"retry_on_tv_timeout_seconds,omitempty"`
}

// updateTVRoutingSettings handles PUT /v1/settings/tv-routing
func updateTVRoutingSettings(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		var input UpdateTVRoutingInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return apperrors.NewValidationError("invalid request body", nil)
		}

		// Validate arc_tv_policy if provided
		if input.ArcTVPolicy != nil {
			validPolicies := []string{"SKIP", "USE_FALLBACK", "ALWAYS_PLAY"}
			valid := false
			for _, p := range validPolicies {
				if *input.ArcTVPolicy == p {
					valid = true
					break
				}
			}
			if !valid {
				return apperrors.NewValidationError("arc_tv_policy must be one of: SKIP, USE_FALLBACK, ALWAYS_PLAY", map[string]any{
					"allowed_values": validPolicies,
				})
			}
		}

		settings, err := service.UpdateTVRoutingSettings(input)
		if err != nil {
			return apperrors.NewInternalError("Failed to update TV routing settings")
		}

		return api.SingleResponse(w, r, http.StatusOK, "settings", formatTVRoutingSettings(settings))
	}
}

// GetTVRoutingSettings retrieves the current TV routing settings.
func (s *Service) GetTVRoutingSettings() (*TVRoutingSettings, error) {
	var settings TVRoutingSettings
	var fallbackRoomsJSON sql.NullString
	var fallbackDeviceID sql.NullString
	var alwaysSkipOnTV int
	var updatedAt string

	err := s.reader.QueryRow(`
		SELECT arc_tv_policy, fallback_device_id, fallback_rooms_json, always_skip_on_tv,
			retry_on_tv_timeout_seconds, updated_at
		FROM settings
		WHERE setting_key = 'tv_routing'
	`).Scan(&settings.ArcTVPolicy, &fallbackDeviceID, &fallbackRoomsJSON, &alwaysSkipOnTV,
		&settings.RetryOnTVTimeout, &updatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			// Return defaults
			return &TVRoutingSettings{
				ArcTVPolicy:      "SKIP",
				AlwaysSkipOnTV:   false,
				RetryOnTVTimeout: 5,
				FallbackRooms:    []string{},
				UpdatedAt:        time.Now(),
			}, nil
		}
		return nil, err
	}

	settings.AlwaysSkipOnTV = alwaysSkipOnTV == 1

	if fallbackDeviceID.Valid {
		settings.FallbackDeviceID = &fallbackDeviceID.String
	}

	if fallbackRoomsJSON.Valid && fallbackRoomsJSON.String != "" {
		if err := json.Unmarshal([]byte(fallbackRoomsJSON.String), &settings.FallbackRooms); err != nil {
			s.logger.Printf("Failed to parse fallback_rooms_json: %v", err)
			settings.FallbackRooms = []string{}
		}
	} else {
		settings.FallbackRooms = []string{}
	}

	settings.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if settings.UpdatedAt.IsZero() {
		settings.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return &settings, nil
}

// UpdateTVRoutingSettings updates the TV routing settings.
func (s *Service) UpdateTVRoutingSettings(input UpdateTVRoutingInput) (*TVRoutingSettings, error) {
	// Get current settings
	current, err := s.GetTVRoutingSettings()
	if err != nil {
		return nil, err
	}

	// Apply updates
	if input.ArcTVPolicy != nil {
		current.ArcTVPolicy = *input.ArcTVPolicy
	}
	if input.FallbackDeviceID != nil {
		current.FallbackDeviceID = input.FallbackDeviceID
	}
	if input.FallbackRooms != nil {
		current.FallbackRooms = input.FallbackRooms
	}
	if input.AlwaysSkipOnTV != nil {
		current.AlwaysSkipOnTV = *input.AlwaysSkipOnTV
	}
	if input.RetryOnTVTimeout != nil {
		current.RetryOnTVTimeout = *input.RetryOnTVTimeout
	}

	// Serialize fallback rooms
	var fallbackRoomsJSON *string
	if len(current.FallbackRooms) > 0 {
		bytes, err := json.Marshal(current.FallbackRooms)
		if err != nil {
			return nil, err
		}
		str := string(bytes)
		fallbackRoomsJSON = &str
	}

	now := time.Now().UTC().Format(time.RFC3339)
	alwaysSkipInt := 0
	if current.AlwaysSkipOnTV {
		alwaysSkipInt = 1
	}

	// Upsert the settings
	_, err = s.writer.Exec(`
		INSERT INTO settings (setting_key, arc_tv_policy, fallback_device_id, fallback_rooms_json,
			always_skip_on_tv, retry_on_tv_timeout_seconds, updated_at)
		VALUES ('tv_routing', ?, ?, ?, ?, ?, ?)
		ON CONFLICT(setting_key) DO UPDATE SET
			arc_tv_policy = excluded.arc_tv_policy,
			fallback_device_id = excluded.fallback_device_id,
			fallback_rooms_json = excluded.fallback_rooms_json,
			always_skip_on_tv = excluded.always_skip_on_tv,
			retry_on_tv_timeout_seconds = excluded.retry_on_tv_timeout_seconds,
			updated_at = excluded.updated_at
	`, current.ArcTVPolicy, current.FallbackDeviceID, fallbackRoomsJSON, alwaysSkipInt,
		current.RetryOnTVTimeout, now)
	if err != nil {
		return nil, err
	}

	current.UpdatedAt = time.Now().UTC()
	return current, nil
}

// formatTVRoutingSettings formats TVRoutingSettings for JSON response.
func formatTVRoutingSettings(settings *TVRoutingSettings) map[string]any {
	result := map[string]any{
		"arc_tv_policy":               settings.ArcTVPolicy,
		"always_skip_on_tv":           settings.AlwaysSkipOnTV,
		"retry_on_tv_timeout_seconds": settings.RetryOnTVTimeout,
		"fallback_rooms":              settings.FallbackRooms,
		"updated_at":                  settings.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if settings.FallbackDeviceID != nil {
		result["fallback_device_id"] = *settings.FallbackDeviceID
	}

	return result
}
