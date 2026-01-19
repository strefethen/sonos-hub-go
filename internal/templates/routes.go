package templates

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// RoutineTemplate represents a predefined routine configuration.
type RoutineTemplate struct {
	TemplateID     string  `json:"template_id"`
	Name           string  `json:"name"`
	Description    *string `json:"description,omitempty"`
	Category       string  `json:"category"`
	SortOrder      int     `json:"sort_order"`
	Icon           *string `json:"icon,omitempty"`
	ImageName      *string `json:"image_name,omitempty"`
	GradientColor1 *string `json:"gradient_color_1,omitempty"`
	GradientColor2 *string `json:"gradient_color_2,omitempty"`
	AccentColor    *string `json:"accent_color,omitempty"`

	// Schedule fields
	Timezone         string  `json:"timezone"`
	ScheduleType     string  `json:"schedule_type"`
	ScheduleWeekdays *string `json:"schedule_weekdays,omitempty"`
	ScheduleMonth    *int    `json:"schedule_month,omitempty"`
	ScheduleDay      *int    `json:"schedule_day,omitempty"`
	ScheduleTime     string  `json:"schedule_time"`

	// Speaker targeting
	SuggestedSpeakers *string `json:"suggested_speakers,omitempty"`

	// Music fields
	MusicPolicyType            string  `json:"music_policy_type"`
	MusicSetID                 *string `json:"music_set_id,omitempty"`
	MusicSonosFavoriteID       *string `json:"music_sonos_favorite_id,omitempty"`
	MusicNoRepeatWindowMinutes *int    `json:"music_no_repeat_window_minutes,omitempty"`
	MusicFallbackBehavior      *string `json:"music_fallback_behavior,omitempty"`

	// Behavior fields
	HolidayBehavior string `json:"holiday_behavior"`
	ArcTVPolicy     string `json:"arc_tv_policy"`

	CreatedAt time.Time `json:"created_at"`
}

// DBPair interface for dependency injection.
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// Service provides template management functionality.
type Service struct {
	reader *sql.DB
}

// NewService creates a new templates service that reads from the database.
func NewService(dbPair DBPair) *Service {
	return &Service{
		reader: dbPair.Reader(),
	}
}

// RegisterRoutes wires template routes to the router.
func RegisterRoutes(router chi.Router, service *Service) {
	router.Method(http.MethodGet, "/v1/routine-templates", api.Handler(listTemplates(service)))
	router.Method(http.MethodGet, "/v1/routine-templates/{template_id}", api.Handler(getTemplate(service)))
}

// listTemplates handles GET /v1/routine-templates
func listTemplates(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		category := r.URL.Query().Get("category")

		var rows *sql.Rows
		var err error

		if category != "" {
			rows, err = service.reader.Query(`
				SELECT template_id, name, description, category, sort_order, icon, image_name,
				       gradient_color_1, gradient_color_2, accent_color,
				       timezone, schedule_type, schedule_weekdays, schedule_month, schedule_day, schedule_time,
				       suggested_speakers, music_policy_type, music_set_id, music_sonos_favorite_id,
				       music_no_repeat_window_minutes, music_fallback_behavior,
				       holiday_behavior, arc_tv_policy, created_at
				FROM routine_templates
				WHERE category = ?
				ORDER BY sort_order, name
			`, category)
		} else {
			rows, err = service.reader.Query(`
				SELECT template_id, name, description, category, sort_order, icon, image_name,
				       gradient_color_1, gradient_color_2, accent_color,
				       timezone, schedule_type, schedule_weekdays, schedule_month, schedule_day, schedule_time,
				       suggested_speakers, music_policy_type, music_set_id, music_sonos_favorite_id,
				       music_no_repeat_window_minutes, music_fallback_behavior,
				       holiday_behavior, arc_tv_policy, created_at
				FROM routine_templates
				ORDER BY sort_order, name
			`)
		}
		if err != nil {
			return apperrors.NewInternalError("Failed to fetch templates")
		}
		defer rows.Close()

		templates := make([]map[string]any, 0)
		for rows.Next() {
			t, err := scanTemplate(rows)
			if err != nil {
				continue
			}
			templates = append(templates, formatTemplate(t))
		}

		return api.WriteList(w, "/v1/routine-templates", templates, false)
	}
}

// getTemplate handles GET /v1/routine-templates/{template_id}
func getTemplate(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		templateID := chi.URLParam(r, "template_id")

		row := service.reader.QueryRow(`
			SELECT template_id, name, description, category, sort_order, icon, image_name,
			       gradient_color_1, gradient_color_2, accent_color,
			       timezone, schedule_type, schedule_weekdays, schedule_month, schedule_day, schedule_time,
			       suggested_speakers, music_policy_type, music_set_id, music_sonos_favorite_id,
			       music_no_repeat_window_minutes, music_fallback_behavior,
			       holiday_behavior, arc_tv_policy, created_at
			FROM routine_templates
			WHERE template_id = ?
		`, templateID)

		t, err := scanTemplateRow(row)
		if err != nil {
			if err == sql.ErrNoRows {
				return apperrors.NewNotFoundError("Template not found", map[string]any{
					"template_id": templateID,
				})
			}
			return apperrors.NewInternalError("Failed to fetch template")
		}

		return api.WriteResource(w, http.StatusOK, formatTemplate(t))
	}
}

// scanTemplate scans a row into a RoutineTemplate.
func scanTemplate(rows *sql.Rows) (*RoutineTemplate, error) {
	var t RoutineTemplate
	var createdAt string

	err := rows.Scan(
		&t.TemplateID, &t.Name, &t.Description, &t.Category, &t.SortOrder,
		&t.Icon, &t.ImageName, &t.GradientColor1, &t.GradientColor2, &t.AccentColor,
		&t.Timezone, &t.ScheduleType, &t.ScheduleWeekdays, &t.ScheduleMonth, &t.ScheduleDay, &t.ScheduleTime,
		&t.SuggestedSpeakers, &t.MusicPolicyType, &t.MusicSetID, &t.MusicSonosFavoriteID,
		&t.MusicNoRepeatWindowMinutes, &t.MusicFallbackBehavior,
		&t.HolidayBehavior, &t.ArcTVPolicy, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if t.CreatedAt.IsZero() {
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}

	return &t, nil
}

// scanTemplateRow scans a single row into a RoutineTemplate.
func scanTemplateRow(row *sql.Row) (*RoutineTemplate, error) {
	var t RoutineTemplate
	var createdAt string

	err := row.Scan(
		&t.TemplateID, &t.Name, &t.Description, &t.Category, &t.SortOrder,
		&t.Icon, &t.ImageName, &t.GradientColor1, &t.GradientColor2, &t.AccentColor,
		&t.Timezone, &t.ScheduleType, &t.ScheduleWeekdays, &t.ScheduleMonth, &t.ScheduleDay, &t.ScheduleTime,
		&t.SuggestedSpeakers, &t.MusicPolicyType, &t.MusicSetID, &t.MusicSonosFavoriteID,
		&t.MusicNoRepeatWindowMinutes, &t.MusicFallbackBehavior,
		&t.HolidayBehavior, &t.ArcTVPolicy, &createdAt,
	)
	if err != nil {
		return nil, err
	}

	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if t.CreatedAt.IsZero() {
		t.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}

	return &t, nil
}

// formatTemplate formats a RoutineTemplate for JSON response.
func formatTemplate(t *RoutineTemplate) map[string]any {
	result := map[string]any{
		"object":            api.ObjectRoutineTemplate,
		"id":                t.TemplateID,
		"name":              t.Name,
		"category":          t.Category,
		"sort_order":        t.SortOrder,
		"timezone":          t.Timezone,
		"schedule_type":     t.ScheduleType,
		"schedule_time":     t.ScheduleTime,
		"music_policy_type": t.MusicPolicyType,
		"holiday_behavior":  t.HolidayBehavior,
		"arc_tv_policy":     t.ArcTVPolicy,
		"created_at":        api.RFC3339Millis(t.CreatedAt),
	}

	if t.Description != nil {
		result["description"] = *t.Description
	}
	if t.Icon != nil {
		result["icon"] = *t.Icon
	}
	if t.ImageName != nil {
		result["image_name"] = *t.ImageName
	}
	if t.GradientColor1 != nil {
		result["gradient_color_1"] = *t.GradientColor1
	}
	if t.GradientColor2 != nil {
		result["gradient_color_2"] = *t.GradientColor2
	}
	if t.AccentColor != nil {
		result["accent_color"] = *t.AccentColor
	}
	if t.ScheduleWeekdays != nil {
		var weekdays []int
		if json.Unmarshal([]byte(*t.ScheduleWeekdays), &weekdays) == nil {
			result["schedule_weekdays"] = weekdays
		}
	}
	if t.ScheduleMonth != nil {
		result["schedule_month"] = *t.ScheduleMonth
	}
	if t.ScheduleDay != nil {
		result["schedule_day"] = *t.ScheduleDay
	}
	if t.SuggestedSpeakers != nil {
		var speakers []map[string]any
		if json.Unmarshal([]byte(*t.SuggestedSpeakers), &speakers) == nil {
			result["suggested_speakers"] = speakers
		}
	}
	if t.MusicSetID != nil {
		result["music_set_id"] = *t.MusicSetID
	}
	if t.MusicSonosFavoriteID != nil {
		result["music_sonos_favorite_id"] = *t.MusicSonosFavoriteID
	}
	if t.MusicNoRepeatWindowMinutes != nil {
		result["music_no_repeat_window_minutes"] = *t.MusicNoRepeatWindowMinutes
	}
	if t.MusicFallbackBehavior != nil {
		result["music_fallback_behavior"] = *t.MusicFallbackBehavior
	}

	return result
}
