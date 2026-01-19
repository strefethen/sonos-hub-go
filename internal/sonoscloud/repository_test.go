package sonoscloud

import (
	"context"
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
	// No need to call Init() - schema is handled by db.Init()

	return repo
}

func TestRepository_SaveAndGetToken(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

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

	err := repo.SaveToken(ctx, token)
	require.NoError(t, err)

	fetched, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.NotNil(t, fetched)

	require.Equal(t, "access-token-123", fetched.AccessToken)
	require.Equal(t, "refresh-token-456", fetched.RefreshToken)
	require.Equal(t, "Bearer", fetched.TokenType)
	require.Equal(t, "playback-control-all", fetched.Scope)
	require.WithinDuration(t, expiresAt, fetched.ExpiresAt, time.Second)
}

func TestRepository_GetToken_NotFound(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	token, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.Nil(t, token)
}

func TestRepository_SaveToken_Update(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

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
	err := repo.SaveToken(ctx, token1)
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
	err = repo.SaveToken(ctx, token2)
	require.NoError(t, err)

	// Verify update
	fetched, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, "access-token-2", fetched.AccessToken)
	require.Equal(t, "refresh-token-2", fetched.RefreshToken)
}

func TestRepository_DeleteToken(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	token := &TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}

	err := repo.SaveToken(ctx, token)
	require.NoError(t, err)

	err = repo.DeleteToken(ctx)
	require.NoError(t, err)

	fetched, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.Nil(t, fetched)
}

func TestRepository_DeleteToken_NotFound(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	// DeleteToken on empty table should not error (no rows affected is fine)
	err := repo.DeleteToken(ctx)
	require.NoError(t, err)
}

func TestRepository_SaveToken_PreservesHouseholdID(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	householdID := "HH_123456"

	// Save token with household_id
	token1 := &TokenPair{
		AccessToken:  "access-token-1",
		RefreshToken: "refresh-token-1",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		HouseholdID:  &householdID,
		CreatedAt:    now,
	}
	err := repo.SaveToken(ctx, token1)
	require.NoError(t, err)

	// Update with new token that has nil household_id (simulating token refresh)
	token2 := &TokenPair{
		AccessToken:  "access-token-2",
		RefreshToken: "refresh-token-2",
		ExpiresAt:    now.Add(2 * time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		HouseholdID:  nil, // Not set in refresh response
		CreatedAt:    now,
	}
	err = repo.SaveToken(ctx, token2)
	require.NoError(t, err)

	// Verify household_id was preserved via COALESCE
	fetched, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.NotNil(t, fetched.HouseholdID)
	require.Equal(t, "HH_123456", *fetched.HouseholdID)
}

func TestRepository_UpdateHouseholdID(t *testing.T) {
	repo := setupTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Save initial token without household_id
	token := &TokenPair{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		ExpiresAt:    now.Add(time.Hour),
		TokenType:    "Bearer",
		Scope:        "playback-control-all",
		CreatedAt:    now,
	}
	err := repo.SaveToken(ctx, token)
	require.NoError(t, err)

	// Update household_id
	err = repo.UpdateHouseholdID(ctx, "HH_UPDATED")
	require.NoError(t, err)

	// Verify update
	fetched, err := repo.GetToken(ctx)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.NotNil(t, fetched.HouseholdID)
	require.Equal(t, "HH_UPDATED", *fetched.HouseholdID)
}
