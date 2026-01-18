package templates

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/strefethen/sonos-hub-go/internal/api"
	"github.com/strefethen/sonos-hub-go/internal/apperrors"
)

// RoutineTemplate represents a predefined routine configuration.
type RoutineTemplate struct {
	TemplateID   string          `json:"template_id"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	Icon         string          `json:"icon,omitempty"`
	ScheduleType string          `json:"schedule_type"`
	DefaultTime  string          `json:"default_time,omitempty"`
	Actions      []TemplateAction `json:"actions"`
	Tags         []string        `json:"tags,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// TemplateAction represents an action within a template.
type TemplateAction struct {
	Type       string         `json:"type"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// Service provides template management functionality.
type Service struct {
	templates []RoutineTemplate
}

// NewService creates a new templates service with embedded templates.
func NewService() *Service {
	return &Service{
		templates: getEmbeddedTemplates(),
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

		templates := service.templates
		if category != "" {
			filtered := make([]RoutineTemplate, 0)
			for _, t := range templates {
				if t.Category == category {
					filtered = append(filtered, t)
				}
			}
			templates = filtered
		}

		formatted := make([]map[string]any, 0, len(templates))
		for _, t := range templates {
			formatted = append(formatted, formatTemplate(&t))
		}

		// Templates is a small fixed list, so pagination is not needed
		return api.ListResponse(w, r, http.StatusOK, "templates", formatted, nil)
	}
}

// getTemplate handles GET /v1/routine-templates/{template_id}
func getTemplate(service *Service) func(w http.ResponseWriter, r *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		templateID := chi.URLParam(r, "template_id")

		var found *RoutineTemplate
		for _, t := range service.templates {
			if t.TemplateID == templateID {
				found = &t
				break
			}
		}

		if found == nil {
			return apperrors.NewNotFoundError("Template not found", map[string]any{
				"template_id": templateID,
			})
		}

		return api.SingleResponse(w, r, http.StatusOK, "template", formatTemplate(found))
	}
}

// formatTemplate formats a RoutineTemplate for JSON response.
func formatTemplate(t *RoutineTemplate) map[string]any {
	result := map[string]any{
		"template_id":   t.TemplateID,
		"name":          t.Name,
		"description":   t.Description,
		"category":      t.Category,
		"schedule_type": t.ScheduleType,
		"actions":       formatActions(t.Actions),
		"created_at":    t.CreatedAt.UTC().Format(time.RFC3339),
	}

	if t.Icon != "" {
		result["icon"] = t.Icon
	}
	if t.DefaultTime != "" {
		result["default_time"] = t.DefaultTime
	}
	if len(t.Tags) > 0 {
		result["tags"] = t.Tags
	}

	return result
}

// formatActions formats template actions for JSON response.
func formatActions(actions []TemplateAction) []map[string]any {
	result := make([]map[string]any, 0, len(actions))
	for _, a := range actions {
		action := map[string]any{
			"type": a.Type,
		}
		if a.Parameters != nil {
			action["parameters"] = a.Parameters
		}
		result = append(result, action)
	}
	return result
}

// getEmbeddedTemplates returns the built-in routine templates.
func getEmbeddedTemplates() []RoutineTemplate {
	now := time.Now()

	return []RoutineTemplate{
		{
			TemplateID:   "morning-wake-up",
			Name:         "Morning Wake Up",
			Description:  "Start your day with music to wake up gently",
			Category:     "morning",
			Icon:         "sunrise",
			ScheduleType: "weekly",
			DefaultTime:  "07:00",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 30, "fade_in": true}},
			},
			Tags:      []string{"wake", "morning", "music"},
			CreatedAt: now,
		},
		{
			TemplateID:   "bedtime-wind-down",
			Name:         "Bedtime Wind Down",
			Description:  "Relax before sleep with calming sounds",
			Category:     "evening",
			Icon:         "moon",
			ScheduleType: "weekly",
			DefaultTime:  "22:00",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set", "shuffle": true}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 20}},
				{Type: "sleep_timer", Parameters: map[string]any{"minutes": 30}},
			},
			Tags:      []string{"sleep", "evening", "relax"},
			CreatedAt: now,
		},
		{
			TemplateID:   "arrival-home",
			Name:         "Arrival Home",
			Description:  "Welcome yourself home with your favorite tunes",
			Category:     "trigger",
			Icon:         "home",
			ScheduleType: "once",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "sonos_favorites"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 40}},
			},
			Tags:      []string{"home", "arrival", "welcome"},
			CreatedAt: now,
		},
		{
			TemplateID:   "weekend-morning",
			Name:         "Weekend Morning",
			Description:  "Start your weekend with upbeat music",
			Category:     "morning",
			Icon:         "sun",
			ScheduleType: "weekly",
			DefaultTime:  "09:00",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 45, "fade_in": true}},
			},
			Tags:      []string{"weekend", "morning", "upbeat"},
			CreatedAt: now,
		},
		{
			TemplateID:   "focus-time",
			Name:         "Focus Time",
			Description:  "Background music for concentration and productivity",
			Category:     "productivity",
			Icon:         "headphones",
			ScheduleType: "once",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set", "genre": "ambient"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 25}},
			},
			Tags:      []string{"focus", "work", "productivity", "ambient"},
			CreatedAt: now,
		},
		{
			TemplateID:   "dinner-party",
			Name:         "Dinner Party",
			Description:  "Set the mood for entertaining guests",
			Category:     "entertainment",
			Icon:         "wine",
			ScheduleType: "once",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set", "shuffle": true}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 35}},
				{Type: "group_speakers", Parameters: map[string]any{"rooms": []string{"living_room", "dining_room"}}},
			},
			Tags:      []string{"party", "dinner", "entertainment", "social"},
			CreatedAt: now,
		},
		{
			TemplateID:   "workout",
			Name:         "Workout",
			Description:  "High energy music to power your exercise routine",
			Category:     "fitness",
			Icon:         "activity",
			ScheduleType: "once",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set", "genre": "workout"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 60}},
			},
			Tags:      []string{"workout", "exercise", "fitness", "energy"},
			CreatedAt: now,
		},
		{
			TemplateID:   "kids-bedtime",
			Name:         "Kids Bedtime",
			Description:  "Gentle lullabies and stories for children",
			Category:     "kids",
			Icon:         "star",
			ScheduleType: "weekly",
			DefaultTime:  "19:30",
			Actions: []TemplateAction{
				{Type: "play_music", Parameters: map[string]any{"source": "music_set", "genre": "lullaby"}},
				{Type: "set_volume", Parameters: map[string]any{"volume": 15}},
				{Type: "sleep_timer", Parameters: map[string]any{"minutes": 45}},
			},
			Tags:      []string{"kids", "bedtime", "lullaby", "children"},
			CreatedAt: now,
		},
	}
}
