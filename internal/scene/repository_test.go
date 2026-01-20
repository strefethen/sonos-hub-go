package scene

import (
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
)

func setupTestDB(t *testing.T) *ScenesRepository {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return NewScenesRepository(dbPair)
}

func setupTestDBWithExec(t *testing.T) (*ScenesRepository, *ExecutionsRepository) {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	return NewScenesRepository(dbPair), NewExecutionsRepository(dbPair)
}

func TestScenesRepository_Create(t *testing.T) {
	repo := setupTestDB(t)

	volume := 50
	description := "Test scene"
	input := CreateSceneInput{
		Name:        "Morning Music",
		Description: &description,
		Members: []SceneMember{
			{UDN: "RINCON_TEST001", TargetVolume: &volume},
			{UDN: "RINCON_TEST002", RoomName: "Kitchen"},
		},
		VolumeRamp: &VolumeRamp{Enabled: true, Curve: "linear"},
	}

	scene, err := repo.Create(input)
	require.NoError(t, err)
	require.NotNil(t, scene)
	require.NotEmpty(t, scene.SceneID)
	require.Equal(t, "Morning Music", scene.Name)
	require.NotNil(t, scene.Description)
	require.Equal(t, "Test scene", *scene.Description)
	require.Equal(t, "ARC_FIRST", scene.CoordinatorPreference)
	require.Equal(t, "PLAYBASE_IF_ARC_TV_ACTIVE", scene.FallbackPolicy)
	require.Len(t, scene.Members, 2)
	require.NotNil(t, scene.VolumeRamp)
	require.True(t, scene.VolumeRamp.Enabled)
	require.Nil(t, scene.Teardown)
}

func TestScenesRepository_GetByID(t *testing.T) {
	repo := setupTestDB(t)

	scene, err := repo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{{UDN: "RINCON_TEST001"}},
	})
	require.NoError(t, err)

	fetched, err := repo.GetByID(scene.SceneID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, scene.SceneID, fetched.SceneID)
	require.Equal(t, "Test Scene", fetched.Name)
}

func TestScenesRepository_GetByID_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	scene, err := repo.GetByID("nonexistent")
	require.NoError(t, err)
	require.Nil(t, scene)
}

func TestScenesRepository_List(t *testing.T) {
	repo := setupTestDB(t)

	for i := 0; i < 5; i++ {
		_, err := repo.Create(CreateSceneInput{
			Name:    "Scene " + string(rune('A'+i)),
			Members: []SceneMember{},
		})
		require.NoError(t, err)
	}

	scenes, total, err := repo.List(3, 0)
	require.NoError(t, err)
	require.Len(t, scenes, 3)
	require.Equal(t, 5, total)

	scenes, total, err = repo.List(3, 3)
	require.NoError(t, err)
	require.Len(t, scenes, 2)
	require.Equal(t, 5, total)
}

func TestScenesRepository_Update(t *testing.T) {
	repo := setupTestDB(t)

	scene, err := repo.Create(CreateSceneInput{
		Name:    "Original Name",
		Members: []SceneMember{{UDN: "RINCON_TEST001"}},
	})
	require.NoError(t, err)

	newName := "Updated Name"
	updated, err := repo.Update(scene.SceneID, UpdateSceneInput{
		Name: &newName,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Updated Name", updated.Name)
	require.Len(t, updated.Members, 1) // Members preserved
}

func TestScenesRepository_Update_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	newName := "Updated"
	updated, err := repo.Update("nonexistent", UpdateSceneInput{Name: &newName})
	require.NoError(t, err)
	require.Nil(t, updated)
}

func TestScenesRepository_Delete(t *testing.T) {
	repo := setupTestDB(t)

	scene, err := repo.Create(CreateSceneInput{
		Name:    "To Delete",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	err = repo.Delete(scene.SceneID)
	require.NoError(t, err)

	fetched, err := repo.GetByID(scene.SceneID)
	require.NoError(t, err)
	require.Nil(t, fetched)
}

func TestScenesRepository_Delete_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	err := repo.Delete("nonexistent")
	require.Error(t, err)
}

func TestExecutionsRepository_Create(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	idempotencyKey := "idem-123"
	exec, err := execRepo.Create(CreateExecutionInput{
		SceneID:        scene.SceneID,
		IdempotencyKey: &idempotencyKey,
	})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.NotEmpty(t, exec.SceneExecutionID)
	require.Equal(t, scene.SceneID, exec.SceneID)
	require.NotNil(t, exec.IdempotencyKey)
	require.Equal(t, "idem-123", *exec.IdempotencyKey)
	require.Equal(t, ExecutionStatusStarting, exec.Status)
	require.Len(t, exec.Steps, 8)
}

func TestExecutionsRepository_GetByIdempotencyKey(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	idempotencyKey := "unique-key-123"
	exec, err := execRepo.Create(CreateExecutionInput{
		SceneID:        scene.SceneID,
		IdempotencyKey: &idempotencyKey,
	})
	require.NoError(t, err)

	found, err := execRepo.GetByIdempotencyKey("unique-key-123")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, exec.SceneExecutionID, found.SceneExecutionID)
}

func TestExecutionsRepository_UpdateStep(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	exec, err := execRepo.Create(CreateExecutionInput{SceneID: scene.SceneID})
	require.NoError(t, err)

	status := StepStatusRunning
	err = execRepo.UpdateStep(exec.SceneExecutionID, "acquire_lock", StepUpdate{
		Status: &status,
	})
	require.NoError(t, err)

	updated, err := execRepo.GetByID(exec.SceneExecutionID)
	require.NoError(t, err)
	require.Equal(t, StepStatusRunning, updated.Steps[0].Status)
}

func TestExecutionsRepository_SetCoordinator(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	exec, err := execRepo.Create(CreateExecutionInput{SceneID: scene.SceneID})
	require.NoError(t, err)

	err = execRepo.SetCoordinator(exec.SceneExecutionID, "RINCON_COORDINATOR001")
	require.NoError(t, err)

	updated, err := execRepo.GetByID(exec.SceneExecutionID)
	require.NoError(t, err)
	require.NotNil(t, updated.CoordinatorUsedUDN)
	require.Equal(t, "RINCON_COORDINATOR001", *updated.CoordinatorUsedUDN)
}

func TestExecutionsRepository_Complete(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	exec, err := execRepo.Create(CreateExecutionInput{SceneID: scene.SceneID})
	require.NoError(t, err)

	verification := &Verification{
		PlaybackConfirmed: true,
		TransportState:    "PLAYING",
	}
	err = execRepo.Complete(exec.SceneExecutionID, ExecutionStatusPlayingConfirmed, verification, nil)
	require.NoError(t, err)

	updated, err := execRepo.GetByID(exec.SceneExecutionID)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusPlayingConfirmed, updated.Status)
	require.NotNil(t, updated.EndedAt)
	require.NotNil(t, updated.Verification)
	require.True(t, updated.Verification.PlaybackConfirmed)
}

func TestExecutionsRepository_Complete_WithError(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	exec, err := execRepo.Create(CreateExecutionInput{SceneID: scene.SceneID})
	require.NoError(t, err)

	errMsg := "device offline"
	err = execRepo.Complete(exec.SceneExecutionID, ExecutionStatusFailed, nil, &errMsg)
	require.NoError(t, err)

	updated, err := execRepo.GetByID(exec.SceneExecutionID)
	require.NoError(t, err)
	require.Equal(t, ExecutionStatusFailed, updated.Status)
	require.NotNil(t, updated.Error)
	require.Equal(t, "device offline", *updated.Error)
}

func TestExecutionsRepository_ListBySceneID(t *testing.T) {
	scenesRepo, execRepo := setupTestDBWithExec(t)

	scene, err := scenesRepo.Create(CreateSceneInput{
		Name:    "Test Scene",
		Members: []SceneMember{},
	})
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		_, err := execRepo.Create(CreateExecutionInput{SceneID: scene.SceneID})
		require.NoError(t, err)
	}

	executions, total, err := execRepo.ListBySceneID(scene.SceneID, 3, 0)
	require.NoError(t, err)
	require.Len(t, executions, 3)
	require.Equal(t, 5, total)
}
