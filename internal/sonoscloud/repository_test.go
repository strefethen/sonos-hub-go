package sonoscloud

import (
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/strefethen/sonos-hub-go/internal/db"
)

func setupTestDB(t *testing.T) *Repository {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	dbPair, err := db.Init(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { dbPair.Close() })

	repo := NewRepository(dbPair)
	err = repo.Init()
	require.NoError(t, err)

	return repo
}

func TestRepository_Init(t *testing.T) {
	repo := setupTestDB(t)
	require.NotNil(t, repo)
}

func TestRepository_SaveAndGetToken(t *testing.T) {
	repo := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)

	token := &TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}

	err := repo.SaveToken(token)
	require.NoError(t, err)

	fetched, err := repo.GetToken()
	require.NoError(t, err)
	require.NotNil(t, fetched)

	require.Equal(t, "access-token-123", fetched.AccessToken)
	require.Equal(t, "refresh-token-456", fetched.RefreshToken)
	require.Equal(t, "Bearer", fetched.TokenType)
	require.Equal(t, "playback-control-all", fetched.Scope)
	require.WithinDuration(t, expiresAt, fetched.ExpiresAt, time.Second)
	require.WithinDuration(t, now, fetched.CreatedAt, time.Second)
}

func TestRepository_GetToken_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	token, err := repo.GetToken()
	require.NoError(t, err)
	require.Nil(t, token)
}

func TestRepository_SaveToken_Update(t *testing.T) {
	repo := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)

	// Save initial token
	token1 := &TokenPair{
		AccessToken:  "access-token-1",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}
	err := repo.SaveToken(token1)
	require.NoError(t, err)

	// Update with new token
	token2 := &TokenPair{
		AccessToken:  "access-token-2",
		RefreshToken: "refresh-token-2",
		ExpiresAt:    now.Add(2 * time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}
	err = repo.SaveToken(token2)
	require.NoError(t, err)

	// Verify update
	fetched, err := repo.GetToken()
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, "access-token-2", fetched.AccessToken)
	require.Equal(t, "refresh-token-2", fetched.RefreshToken)
}

func TestRepository_DeleteToken(t *testing.T) {
	repo := setupTestDB(t)

	now := time.Now().UTC().Truncate(time.Second)

	token := &TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}

	err := repo.SaveToken(token)
	require.NoError(t, err)

	err = repo.DeleteToken()
	require.NoError(t, err)

	fetched, err := repo.GetToken()
	require.NoError(t, err)
	require.Nil(t, fetched)
}

func TestRepository_DeleteToken_NotFound(t *testing.T) {
	repo := setupTestDB(t)

	err := repo.DeleteToken()
	require.Error(t, err)
}

func TestRepository_MultipleInitCalls(t *testing.T) {
	repo := setupTestDB(t)

	// Call Init again - should be idempotent
	err := repo.Init()
	require.NoError(t, err)

	// Should still work
	now := time.Now().UTC().Truncate(time.Second)
	token := &TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}

	err = repo.SaveToken(token)
	require.NoError(t, err)

	fetched, err := repo.GetToken()
	require.NoError(t, err)
	require.NotNil(t, fetched)
}
