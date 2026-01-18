package templates

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	service := NewService()
	require.NotNil(t, service)
	require.NotEmpty(t, service.templates)
}

func TestEmbeddedTemplates(t *testing.T) {
	templates := getEmbeddedTemplates()
	require.NotEmpty(t, templates)

	// Verify we have expected template categories
	categories := make(map[string]bool)
	for _, tmpl := range templates {
		categories[tmpl.Category] = true
	}

	require.True(t, categories["morning"], "should have morning category")
	require.True(t, categories["evening"], "should have evening category")
}

func TestTemplateHasRequiredFields(t *testing.T) {
	templates := getEmbeddedTemplates()

	for _, tmpl := range templates {
		require.NotEmpty(t, tmpl.TemplateID, "template_id should not be empty")
		require.NotEmpty(t, tmpl.Name, "name should not be empty")
		require.NotEmpty(t, tmpl.Description, "description should not be empty")
		require.NotEmpty(t, tmpl.Category, "category should not be empty")
		require.NotEmpty(t, tmpl.ScheduleType, "schedule_type should not be empty")
		require.NotEmpty(t, tmpl.Actions, "actions should not be empty")
		require.False(t, tmpl.CreatedAt.IsZero(), "created_at should not be zero")
	}
}

func TestTemplateAction(t *testing.T) {
	action := TemplateAction{
		Type: "play_music",
		Parameters: map[string]any{
			"source":  "music_set",
			"shuffle": true,
		},
	}

	require.Equal(t, "play_music", action.Type)
	require.NotNil(t, action.Parameters)
	require.Equal(t, "music_set", action.Parameters["source"])
	require.Equal(t, true, action.Parameters["shuffle"])
}

func TestFormatTemplate(t *testing.T) {
	templates := getEmbeddedTemplates()
	require.NotEmpty(t, templates)

	formatted := formatTemplate(&templates[0])
	require.NotNil(t, formatted)

	require.Contains(t, formatted, "template_id")
	require.Contains(t, formatted, "name")
	require.Contains(t, formatted, "description")
	require.Contains(t, formatted, "category")
	require.Contains(t, formatted, "schedule_type")
	require.Contains(t, formatted, "actions")
	require.Contains(t, formatted, "created_at")
}

func TestFormatActions(t *testing.T) {
	actions := []TemplateAction{
		{Type: "play_music", Parameters: map[string]any{"source": "music_set"}},
		{Type: "set_volume", Parameters: map[string]any{"volume": 30}},
	}

	formatted := formatActions(actions)
	require.Len(t, formatted, 2)

	require.Equal(t, "play_music", formatted[0]["type"])
	require.NotNil(t, formatted[0]["parameters"])

	require.Equal(t, "set_volume", formatted[1]["type"])
	require.NotNil(t, formatted[1]["parameters"])
}

func TestMorningWakeUpTemplate(t *testing.T) {
	templates := getEmbeddedTemplates()

	var morningWakeUp *RoutineTemplate
	for i, tmpl := range templates {
		if tmpl.TemplateID == "morning-wake-up" {
			morningWakeUp = &templates[i]
			break
		}
	}

	require.NotNil(t, morningWakeUp, "morning-wake-up template should exist")
	require.Equal(t, "Morning Wake Up", morningWakeUp.Name)
	require.Equal(t, "morning", morningWakeUp.Category)
	require.Equal(t, "weekly", morningWakeUp.ScheduleType)
	require.Equal(t, "07:00", morningWakeUp.DefaultTime)
	require.NotEmpty(t, morningWakeUp.Actions)
	require.Contains(t, morningWakeUp.Tags, "wake")
	require.Contains(t, morningWakeUp.Tags, "morning")
}

func TestBedtimeWindDownTemplate(t *testing.T) {
	templates := getEmbeddedTemplates()

	var bedtime *RoutineTemplate
	for i, tmpl := range templates {
		if tmpl.TemplateID == "bedtime-wind-down" {
			bedtime = &templates[i]
			break
		}
	}

	require.NotNil(t, bedtime, "bedtime-wind-down template should exist")
	require.Equal(t, "Bedtime Wind Down", bedtime.Name)
	require.Equal(t, "evening", bedtime.Category)
	require.Equal(t, "22:00", bedtime.DefaultTime)

	// Should have a sleep timer action
	hasSleepTimer := false
	for _, action := range bedtime.Actions {
		if action.Type == "sleep_timer" {
			hasSleepTimer = true
			break
		}
	}
	require.True(t, hasSleepTimer, "bedtime template should have sleep_timer action")
}

func TestTemplateCategories(t *testing.T) {
	templates := getEmbeddedTemplates()

	expectedCategories := []string{"morning", "evening", "trigger", "productivity", "entertainment", "fitness", "kids"}
	foundCategories := make(map[string]bool)

	for _, tmpl := range templates {
		foundCategories[tmpl.Category] = true
	}

	for _, expected := range expectedCategories {
		require.True(t, foundCategories[expected], "should have category: "+expected)
	}
}
