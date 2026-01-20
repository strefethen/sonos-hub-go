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
	ArcTVPolicy      string    `json:"arc_tv_policy"`
	FallbackUDN      *string   `json:"fallback_udn,omitempty"`
	FallbackRooms    []string  `json:"fallback_rooms,omitempty"`
	AlwaysSkipOnTV   bool      `json:"always_skip_on_tv"`
	RetryOnTVTimeout int       `json:"retry_on_tv_timeout_seconds"`
	UpdatedAt        time.Time `json:"updated_at"`
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

		return api.WriteResource(w, http.StatusOK, formatTVRoutingSettings(settings))
	}
}

// UpdateTVRoutingInput represents the request body for updating TV routing settings.
type UpdateTVRoutingInput struct {
	ArcTVPolicy      *string  `json:"arc_tv_policy,omitempty"`
	FallbackUDN      *string  `json:"fallback_udn,omitempty"`
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

		return api.WriteResource(w, http.StatusOK, formatTVRoutingSettings(settings))
	}
}

// GetTVRoutingSettings retrieves the current TV routing settings from key-value store.
func (s *Service) GetTVRoutingSettings() (*TVRoutingSettings, error) {
	// Default settings
	settings := &TVRoutingSettings{
		ArcTVPolicy:      "USE_FALLBACK",
		AlwaysSkipOnTV:   false,
		RetryOnTVTimeout: 5,
		FallbackRooms:    []string{},
		UpdatedAt:        time.Now(),
	}

	// Try to load from JSON blob first (new format)
	var value sql.NullString
	var updatedAt string
	err := s.reader.QueryRow(`
		SELECT value, updated_at FROM settings WHERE key = 'tv_routing'
	`).Scan(&value, &updatedAt)

	if err == nil && value.Valid && value.String != "" {
		// New JSON format
		if err := json.Unmarshal([]byte(value.String), settings); err != nil {
			s.logger.Printf("Failed to parse tv_routing JSON: %v", err)
		}
		settings.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if settings.UpdatedAt.IsZero() {
			settings.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
		}
		return settings, nil
	}

	// Fall back to individual keys
	rows, err := s.reader.Query(`
		SELECT key, value, updated_at FROM settings
		WHERE key IN ('tv_routing_enabled', 'tv_default_fallback_udn', 'tv_default_policy')
	`)
	if err != nil {
		if err == sql.ErrNoRows {
			return settings, nil
		}
		return nil, err
	}
	defer rows.Close()

	var latestUpdate time.Time
	for rows.Next() {
		var key, val, ts string
		if err := rows.Scan(&key, &val, &ts); err != nil {
			continue
		}

		parsed, _ := time.Parse(time.RFC3339, ts)
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02 15:04:05", ts)
		}
		if parsed.After(latestUpdate) {
			latestUpdate = parsed
		}

		switch key {
		case "tv_default_policy":
			settings.ArcTVPolicy = val
		case "tv_default_fallback_udn":
			if val != "" {
				settings.FallbackUDN = &val
			}
		case "tv_routing_enabled":
			settings.AlwaysSkipOnTV = val != "true"
		}
	}

	if !latestUpdate.IsZero() {
		settings.UpdatedAt = latestUpdate
	}

	return settings, nil
}

// UpdateTVRoutingSettings updates the TV routing settings using JSON blob in key-value store.
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
	if input.FallbackUDN != nil {
		current.FallbackUDN = input.FallbackUDN
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

	// Serialize to JSON
	now := time.Now().UTC()
	current.UpdatedAt = now

	jsonBytes, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}

	// Upsert the settings as JSON blob
	_, err = s.writer.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES ('tv_routing', ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, string(jsonBytes), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	return current, nil
}

// formatTVRoutingSettings formats TVRoutingSettings for JSON response.
func formatTVRoutingSettings(settings *TVRoutingSettings) map[string]any {
	result := map[string]any{
		"object":                      "tv_routing_settings",
		"arc_tv_policy":               settings.ArcTVPolicy,
		"always_skip_on_tv":           settings.AlwaysSkipOnTV,
		"retry_on_tv_timeout_seconds": settings.RetryOnTVTimeout,
		"fallback_rooms":              settings.FallbackRooms,
		"updated_at":                  settings.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if settings.FallbackUDN != nil {
		result["fallback_udn"] = *settings.FallbackUDN
	}

	return result
}
