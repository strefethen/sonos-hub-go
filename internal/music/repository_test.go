package music

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
)

func setupTestDB(t *testing.T) (*MusicSetRepository, *SetItemRepository, *PlayHistoryRepository, *sql.DB) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return NewMusicSetRepository(dbPair),
		NewSetItemRepository(dbPair),
		NewPlayHistoryRepository(dbPair),
		dbPair.Writer() // Return writer for direct DB operations in tests
}

// ==========================================================================
// MusicSetRepository Tests
// ==========================================================================

func TestMusicSetRepository_Create(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	input := CreateSetInput{
		Name:            "Morning Playlist",
		SelectionPolicy: string(SelectionPolicyRotation),
	}

	set, err := setRepo.Create(input)
	require.NoError(t, err)
	require.NotNil(t, set)
	require.NotEmpty(t, set.SetID)
	require.Equal(t, "Morning Playlist", set.Name)
	require.Equal(t, string(SelectionPolicyRotation), set.SelectionPolicy)
	require.Equal(t, 0, set.CurrentIndex)
}

func TestMusicSetRepository_CreateWithShuffle(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	input := CreateSetInput{
		Name:            "Shuffle Playlist",
		SelectionPolicy: string(SelectionPolicyShuffle),
	}

	set, err := setRepo.Create(input)
	require.NoError(t, err)
	require.Equal(t, string(SelectionPolicyShuffle), set.SelectionPolicy)
}

func TestMusicSetRepository_GetByID(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	created, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	fetched, err := setRepo.GetByID(created.SetID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, created.SetID, fetched.SetID)
	require.Equal(t, "Test Set", fetched.Name)
}

func TestMusicSetRepository_GetByID_NotFound(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.GetByID("nonexistent")
	require.NoError(t, err)
	require.Nil(t, set)
}

func TestMusicSetRepository_List(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	// Create 5 sets
	for i := 0; i < 5; i++ {
		_, err := setRepo.Create(CreateSetInput{
			Name:            "Set " + string(rune('A'+i)),
			SelectionPolicy: string(SelectionPolicyRotation),
		})
		require.NoError(t, err)
	}

	sets, total, err := setRepo.List(3, 0)
	require.NoError(t, err)
	require.Len(t, sets, 3)
	require.Equal(t, 5, total)

	sets, total, err = setRepo.List(3, 3)
	require.NoError(t, err)
	require.Len(t, sets, 2)
	require.Equal(t, 5, total)
}

func TestMusicSetRepository_List_Empty(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	sets, total, err := setRepo.List(10, 0)
	require.NoError(t, err)
	require.Len(t, sets, 0)
	require.Equal(t, 0, total)
}

func TestMusicSetRepository_Update(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Original Name",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	newName := "Updated Name"
	newPolicy := string(SelectionPolicyShuffle)
	updated, err := setRepo.Update(set.SetID, UpdateSetInput{
		Name:            &newName,
		SelectionPolicy: &newPolicy,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Updated Name", updated.Name)
	require.Equal(t, string(SelectionPolicyShuffle), updated.SelectionPolicy)
}

func TestMusicSetRepository_Update_PartialUpdate(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Original Name",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	newName := "Updated Name"
	updated, err := setRepo.Update(set.SetID, UpdateSetInput{
		Name: &newName,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Updated Name", updated.Name)
	require.Equal(t, string(SelectionPolicyRotation), updated.SelectionPolicy) // Preserved
}

func TestMusicSetRepository_Update_NotFound(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	newName := "Updated"
	updated, err := setRepo.Update("nonexistent", UpdateSetInput{Name: &newName})
	require.NoError(t, err)
	require.Nil(t, updated)
}

func TestMusicSetRepository_Delete(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "To Delete",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	err = setRepo.Delete(set.SetID)
	require.NoError(t, err)

	fetched, err := setRepo.GetByID(set.SetID)
	require.NoError(t, err)
	require.Nil(t, fetched)
}

func TestMusicSetRepository_Delete_NotFound(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	err := setRepo.Delete("nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMusicSetRepository_UpdateCurrentIndex(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)
	require.Equal(t, 0, set.CurrentIndex)

	err = setRepo.UpdateCurrentIndex(set.SetID, 5)
	require.NoError(t, err)

	fetched, err := setRepo.GetByID(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 5, fetched.CurrentIndex)
}

func TestMusicSetRepository_UpdateCurrentIndex_NotFound(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	err := setRepo.UpdateCurrentIndex("nonexistent", 5)
	require.Error(t, err)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestMusicSetRepository_IncrementIndex(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)
	require.Equal(t, 0, set.CurrentIndex)

	newIndex, err := setRepo.IncrementIndex(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 1, newIndex)

	newIndex, err = setRepo.IncrementIndex(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 2, newIndex)

	fetched, err := setRepo.GetByID(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 2, fetched.CurrentIndex)
}

func TestMusicSetRepository_IncrementIndex_NotFound(t *testing.T) {
	setRepo, _, _, _ := setupTestDB(t)

	_, err := setRepo.IncrementIndex("nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

// ==========================================================================
// SetItemRepository Tests
// ==========================================================================

func TestSetItemRepository_Add(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	logoURL := "https://example.com/logo.png"
	serviceName := "Spotify"
	item, err := itemRepo.Add(set.SetID, AddItemInput{
		SonosFavoriteID: "fav-123",
		ServiceLogoURL:  &logoURL,
		ServiceName:     &serviceName,
	})
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, set.SetID, item.SetID)
	require.Equal(t, "fav-123", item.SonosFavoriteID)
	require.Equal(t, 0, item.Position)
	require.NotNil(t, item.ServiceLogoURL)
	require.Equal(t, "https://example.com/logo.png", *item.ServiceLogoURL)
	require.NotNil(t, item.ServiceName)
	require.Equal(t, "Spotify", *item.ServiceName)
	require.Equal(t, "sonos_favorite", item.ContentType)
}

func TestSetItemRepository_Add_AutoPosition(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Add three items
	item1, err := itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	require.Equal(t, 0, item1.Position)

	item2, err := itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)
	require.Equal(t, 1, item2.Position)

	item3, err := itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-3"})
	require.NoError(t, err)
	require.Equal(t, 2, item3.Position)
}

func TestSetItemRepository_Add_WithContentType(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	contentJSON := `{"uri": "x-rincon-cpcontainer:..."}`
	item, err := itemRepo.Add(set.SetID, AddItemInput{
		SonosFavoriteID: "fav-123",
		ContentType:     "playlist",
		ContentJSON:     &contentJSON,
	})
	require.NoError(t, err)
	require.Equal(t, "playlist", item.ContentType)
	require.NotNil(t, item.ContentJSON)
	require.Equal(t, `{"uri": "x-rincon-cpcontainer:..."}`, *item.ContentJSON)
}

func TestSetItemRepository_Remove(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-123"})
	require.NoError(t, err)

	err = itemRepo.Remove(set.SetID, "fav-123")
	require.NoError(t, err)

	item, err := itemRepo.GetItem(set.SetID, "fav-123")
	require.NoError(t, err)
	require.Nil(t, item)
}

func TestSetItemRepository_Remove_NotFound(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	err = itemRepo.Remove(set.SetID, "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestSetItemRepository_GetItems(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Add three items
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-3"})
	require.NoError(t, err)

	items, err := itemRepo.GetItems(set.SetID)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, "fav-1", items[0].SonosFavoriteID)
	require.Equal(t, "fav-2", items[1].SonosFavoriteID)
	require.Equal(t, "fav-3", items[2].SonosFavoriteID)
}

func TestSetItemRepository_GetItems_Empty(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	items, err := itemRepo.GetItems(set.SetID)
	require.NoError(t, err)
	require.Len(t, items, 0)
}

func TestSetItemRepository_GetItem(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-123"})
	require.NoError(t, err)

	item, err := itemRepo.GetItem(set.SetID, "fav-123")
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "fav-123", item.SonosFavoriteID)
}

func TestSetItemRepository_GetItem_NotFound(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	item, err := itemRepo.GetItem(set.SetID, "nonexistent")
	require.NoError(t, err)
	require.Nil(t, item)
}

func TestSetItemRepository_Reorder(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Add three items in order: fav-1, fav-2, fav-3
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-3"})
	require.NoError(t, err)

	// Reorder to: fav-3, fav-1, fav-2
	err = itemRepo.Reorder(set.SetID, []string{"fav-3", "fav-1", "fav-2"})
	require.NoError(t, err)

	items, err := itemRepo.GetItems(set.SetID)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, "fav-3", items[0].SonosFavoriteID)
	require.Equal(t, 0, items[0].Position)
	require.Equal(t, "fav-1", items[1].SonosFavoriteID)
	require.Equal(t, 1, items[1].Position)
	require.Equal(t, "fav-2", items[2].SonosFavoriteID)
	require.Equal(t, 2, items[2].Position)
}

func TestSetItemRepository_Reorder_ItemNotFound(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)

	err = itemRepo.Reorder(set.SetID, []string{"fav-1", "nonexistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "item not found")
}

func TestSetItemRepository_Count(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	count, err := itemRepo.Count(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 0, count)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)

	count, err = itemRepo.Count(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestSetItemRepository_GetByPosition(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-3"})
	require.NoError(t, err)

	item, err := itemRepo.GetByPosition(set.SetID, 1)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "fav-2", item.SonosFavoriteID)
}

func TestSetItemRepository_GetByPosition_NotFound(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	item, err := itemRepo.GetByPosition(set.SetID, 99)
	require.NoError(t, err)
	require.Nil(t, item)
}

func TestSetItemRepository_CascadeDelete(t *testing.T) {
	setRepo, itemRepo, _, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "fav-2"})
	require.NoError(t, err)

	// Delete the set - should cascade delete items
	err = setRepo.Delete(set.SetID)
	require.NoError(t, err)

	// Items should be gone
	items, err := itemRepo.GetItems(set.SetID)
	require.NoError(t, err)
	require.Len(t, items, 0)
}

// ==========================================================================
// PlayHistoryRepository Tests
// ==========================================================================

func TestPlayHistoryRepository_Record(t *testing.T) {
	setRepo, _, historyRepo, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	err = historyRepo.Record("fav-123", &set.SetID, nil)
	require.NoError(t, err)

	history, err := historyRepo.GetHistory("fav-123", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "fav-123", history[0].SonosFavoriteID)
	require.NotNil(t, history[0].SetID)
	require.Equal(t, set.SetID, *history[0].SetID)
	require.Nil(t, history[0].RoutineID)
}

func TestPlayHistoryRepository_Record_WithRoutine(t *testing.T) {
	_, _, historyRepo, conn := setupTestDB(t)

	// Create a scene and routine first (FK constraint)
	sceneID := "scene-test-123"
	_, err := conn.Exec(`
		INSERT INTO scenes (scene_id, name, coordinator_preference, fallback_policy, members, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, sceneID, "Test Scene", "ARC_FIRST", "PLAYBASE_IF_ARC_TV_ACTIVE", "[]")
	require.NoError(t, err)

	routineID := "routine-test-123"
	_, err = conn.Exec(`
		INSERT INTO routines (routine_id, name, enabled, timezone, schedule_type, schedule_time, holiday_behavior, scene_id, music_mode, music_policy_type, skip_next, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`, routineID, "Test Routine", 1, "UTC", "weekly", "08:00", "SKIP", sceneID, "FIXED", "FIXED", 0)
	require.NoError(t, err)

	err = historyRepo.Record("fav-123", nil, &routineID)
	require.NoError(t, err)

	history, err := historyRepo.GetHistory("fav-123", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Nil(t, history[0].SetID)
	require.NotNil(t, history[0].RoutineID)
	require.Equal(t, routineID, *history[0].RoutineID)
}

func TestPlayHistoryRepository_Record_NoSetOrRoutine(t *testing.T) {
	_, _, historyRepo, _ := setupTestDB(t)

	err := historyRepo.Record("fav-123", nil, nil)
	require.NoError(t, err)

	history, err := historyRepo.GetHistory("fav-123", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Nil(t, history[0].SetID)
	require.Nil(t, history[0].RoutineID)
}

func TestPlayHistoryRepository_GetHistory(t *testing.T) {
	_, _, historyRepo, _ := setupTestDB(t)

	// Record multiple plays
	for i := 0; i < 5; i++ {
		err := historyRepo.Record("fav-123", nil, nil)
		require.NoError(t, err)
	}

	history, err := historyRepo.GetHistory("fav-123", 3)
	require.NoError(t, err)
	require.Len(t, history, 3)
}

func TestPlayHistoryRepository_GetHistory_Empty(t *testing.T) {
	_, _, historyRepo, _ := setupTestDB(t)

	history, err := historyRepo.GetHistory("nonexistent", 10)
	require.NoError(t, err)
	require.Len(t, history, 0)
}

func TestPlayHistoryRepository_GetSetHistory(t *testing.T) {
	setRepo, _, historyRepo, _ := setupTestDB(t)

	set1, err := setRepo.Create(CreateSetInput{
		Name:            "Set 1",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	set2, err := setRepo.Create(CreateSetInput{
		Name:            "Set 2",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Record plays for different sets
	for i := 0; i < 3; i++ {
		err := historyRepo.Record("fav-1", &set1.SetID, nil)
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		err := historyRepo.Record("fav-2", &set2.SetID, nil)
		require.NoError(t, err)
	}

	history, err := historyRepo.GetSetHistory(set1.SetID, 10)
	require.NoError(t, err)
	require.Len(t, history, 3)

	history, err = historyRepo.GetSetHistory(set2.SetID, 10)
	require.NoError(t, err)
	require.Len(t, history, 2)
}

func TestPlayHistoryRepository_WasPlayedRecently(t *testing.T) {
	_, _, historyRepo, _ := setupTestDB(t)

	// Record a play
	err := historyRepo.Record("fav-123", nil, nil)
	require.NoError(t, err)

	// Should be played recently (within 5 minutes)
	wasPlayed, err := historyRepo.WasPlayedRecently("fav-123", 5)
	require.NoError(t, err)
	require.True(t, wasPlayed)

	// Different favorite should not have been played
	wasPlayed, err = historyRepo.WasPlayedRecently("fav-other", 5)
	require.NoError(t, err)
	require.False(t, wasPlayed)
}

func TestPlayHistoryRepository_GetRecentlyPlayedInSet(t *testing.T) {
	setRepo, _, historyRepo, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Record plays for different favorites in the same set
	err = historyRepo.Record("fav-1", &set.SetID, nil)
	require.NoError(t, err)
	err = historyRepo.Record("fav-2", &set.SetID, nil)
	require.NoError(t, err)
	err = historyRepo.Record("fav-1", &set.SetID, nil) // Play fav-1 again
	require.NoError(t, err)

	recentlyPlayed, err := historyRepo.GetRecentlyPlayedInSet(set.SetID, 5)
	require.NoError(t, err)
	require.Len(t, recentlyPlayed, 2) // Distinct favorites
	require.Contains(t, recentlyPlayed, "fav-1")
	require.Contains(t, recentlyPlayed, "fav-2")
}

func TestPlayHistoryRepository_GetRecentlyPlayedInSet_Empty(t *testing.T) {
	setRepo, _, historyRepo, _ := setupTestDB(t)

	set, err := setRepo.Create(CreateSetInput{
		Name:            "Test Set",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	recentlyPlayed, err := historyRepo.GetRecentlyPlayedInSet(set.SetID, 5)
	require.NoError(t, err)
	require.Len(t, recentlyPlayed, 0)
}

func TestPlayHistoryRepository_Prune(t *testing.T) {
	_, _, historyRepo, conn := setupTestDB(t)

	// Record a play
	err := historyRepo.Record("fav-123", nil, nil)
	require.NoError(t, err)

	// Manually insert an old record (30 days ago)
	oldTime := time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
	_, err = conn.Exec(`
		INSERT INTO play_history (sonos_favorite_id, played_at)
		VALUES (?, ?)
	`, "fav-old", oldTime)
	require.NoError(t, err)

	// Prune records older than 7 days
	deleted, err := historyRepo.Prune(7)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	// Recent record should still exist
	history, err := historyRepo.GetHistory("fav-123", 10)
	require.NoError(t, err)
	require.Len(t, history, 1)

	// Old record should be deleted
	history, err = historyRepo.GetHistory("fav-old", 10)
	require.NoError(t, err)
	require.Len(t, history, 0)
}

func TestPlayHistoryRepository_Prune_NoOldRecords(t *testing.T) {
	_, _, historyRepo, _ := setupTestDB(t)

	// Record a recent play
	err := historyRepo.Record("fav-123", nil, nil)
	require.NoError(t, err)

	// Prune records older than 7 days - should delete nothing
	deleted, err := historyRepo.Prune(7)
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)
}

// ==========================================================================
// Integration Tests
// ==========================================================================

func TestIntegration_FullWorkflow(t *testing.T) {
	setRepo, itemRepo, historyRepo, _ := setupTestDB(t)

	// Create a music set
	set, err := setRepo.Create(CreateSetInput{
		Name:            "Morning Playlist",
		SelectionPolicy: string(SelectionPolicyRotation),
	})
	require.NoError(t, err)

	// Add items to the set
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "spotify-playlist-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "apple-music-album-1"})
	require.NoError(t, err)
	_, err = itemRepo.Add(set.SetID, AddItemInput{SonosFavoriteID: "tunein-radio-1"})
	require.NoError(t, err)

	// Verify items
	count, err := itemRepo.Count(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 3, count)

	// Get item by current index
	item, err := itemRepo.GetByPosition(set.SetID, set.CurrentIndex)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "spotify-playlist-1", item.SonosFavoriteID)

	// Record play and increment index
	err = historyRepo.Record(item.SonosFavoriteID, &set.SetID, nil)
	require.NoError(t, err)

	newIndex, err := setRepo.IncrementIndex(set.SetID)
	require.NoError(t, err)
	require.Equal(t, 1, newIndex)

	// Check play history
	wasPlayed, err := historyRepo.WasPlayedRecently("spotify-playlist-1", 5)
	require.NoError(t, err)
	require.True(t, wasPlayed)

	// Get next item
	item, err = itemRepo.GetByPosition(set.SetID, newIndex)
	require.NoError(t, err)
	require.NotNil(t, item)
	require.Equal(t, "apple-music-album-1", item.SonosFavoriteID)
}
