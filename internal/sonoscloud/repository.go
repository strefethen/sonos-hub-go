package sonoscloud

import (
	"database/sql"
	"errors"
	"time"
)

const tokenKey = "sonos_cloud_token"

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

// Init creates the sonos_cloud_tokens table if it doesn't exist.
func (r *Repository) Init() error {
	_, err := r.writer.Exec(`
		CREATE TABLE IF NOT EXISTS sonos_cloud_tokens (
			key TEXT PRIMARY KEY,
			access_token TEXT NOT NULL,
			refresh_token TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			token_type TEXT NOT NULL,
			scope TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	return err
}

// GetToken retrieves the stored token.
func (r *Repository) GetToken() (*TokenPair, error) {
	row := r.reader.QueryRow(`
		SELECT access_token, refresh_token, expires_at, token_type, scope, created_at
		FROM sonos_cloud_tokens
		WHERE key = ?
	`, tokenKey)

	var token TokenPair
	var expiresAt, createdAt string

	err := row.Scan(
		&token.AccessToken,
		&token.RefreshToken,
		&expiresAt,
		&token.TokenType,
		&token.Scope,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	token.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		token.ExpiresAt, _ = time.Parse("2006-01-02 15:04:05", expiresAt)
	}

	token.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		token.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}

	return &token, nil
}

// SaveToken stores or updates the token.
func (r *Repository) SaveToken(token *TokenPair) error {
	now := nowISO()
	expiresAt := token.ExpiresAt.UTC().Format(time.RFC3339)
	createdAt := token.CreatedAt.UTC().Format(time.RFC3339)

	_, err := r.writer.Exec(`
		INSERT INTO sonos_cloud_tokens (key, access_token, refresh_token, expires_at, token_type, scope, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			access_token = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at = excluded.expires_at,
			token_type = excluded.token_type,
			scope = excluded.scope,
			updated_at = excluded.updated_at
	`, tokenKey, token.AccessToken, token.RefreshToken, expiresAt, token.TokenType, token.Scope, createdAt, now)
	return err
}

// DeleteToken removes the stored token.
func (r *Repository) DeleteToken() error {
	result, err := r.writer.Exec("DELETE FROM sonos_cloud_tokens WHERE key = ?", tokenKey)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
