package sonoscloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// Repository handles database operations for Sonos Cloud OAuth tokens.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type Repository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewRepository creates a new Repository.
func NewRepository(dbPair DBPair) *Repository {
	return &Repository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// GetToken retrieves the stored token.
// Compatible with Node.js schema which uses id=1 and expires_at as unix timestamp in SECONDS.
func (r *Repository) GetToken(ctx context.Context) (*TokenPair, error) {
	row := r.reader.QueryRowContext(ctx, `
		SELECT access_token, refresh_token, expires_at, scope, household_id, created_at, updated_at
		FROM sonos_cloud_tokens
		WHERE id = 1
	`)

	var token TokenPair
	var expiresAtUnix int64
	var householdID sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&token.AccessToken,
		&token.RefreshToken,
		&expiresAtUnix,
		&token.Scope,
		&householdID,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan token: %w", err)
	}

	// Node.js stores expires_at as unix timestamp in SECONDS (not milliseconds)
	token.ExpiresAt = time.Unix(expiresAtUnix, 0)
	token.TokenType = "Bearer" // Node.js doesn't store this, default to Bearer
	if householdID.Valid {
		token.HouseholdID = &householdID.String
	}

	token.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		token.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}

	token.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		token.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return &token, nil
}

// SaveToken stores or updates the token using UPSERT pattern.
// Stores expires_at as Unix timestamp in SECONDS to match Node.js.
func (r *Repository) SaveToken(ctx context.Context, token *TokenPair) error {
	expiresAtUnix := token.ExpiresAt.Unix() // SECONDS, not milliseconds

	_, err := r.writer.ExecContext(ctx, `
		INSERT INTO sonos_cloud_tokens (id, access_token, refresh_token, expires_at, scope, household_id, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at = excluded.expires_at,
			scope = excluded.scope,
			household_id = COALESCE(excluded.household_id, sonos_cloud_tokens.household_id),
			updated_at = CURRENT_TIMESTAMP
	`, token.AccessToken, token.RefreshToken, expiresAtUnix, token.Scope, token.HouseholdID)

	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}

// UpdateHouseholdID updates just the household_id field.
func (r *Repository) UpdateHouseholdID(ctx context.Context, householdID string) error {
	_, err := r.writer.ExecContext(ctx, `
		UPDATE sonos_cloud_tokens
		SET household_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, householdID)
	if err != nil {
		return fmt.Errorf("update household_id: %w", err)
	}
	return nil
}

// DeleteToken removes the stored token.
func (r *Repository) DeleteToken(ctx context.Context) error {
	_, err := r.writer.ExecContext(ctx, `DELETE FROM sonos_cloud_tokens WHERE id = 1`)
	if err != nil {
		return fmt.Errorf("delete token: %w", err)
	}
	return nil
}
