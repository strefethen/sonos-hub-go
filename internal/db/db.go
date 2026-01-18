package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DBPair holds separate read and write connections for optimal SQLite concurrency.
// With WAL mode, readers don't block writers and vice versa.
// Using separate pools allows concurrent reads while serializing writes.
type DBPair struct {
	reader *sql.DB // Multiple connections for concurrent reads
	writer *sql.DB // Single connection for serialized writes
}

// Reader returns the read-only database connection pool.
func (p *DBPair) Reader() *sql.DB { return p.reader }

// Writer returns the read-write database connection pool.
func (p *DBPair) Writer() *sql.DB { return p.writer }

// Close closes both database connections.
func (p *DBPair) Close() error {
	var errs []error
	if err := p.reader.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close reader: %w", err))
	}
	if err := p.writer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close writer: %w", err))
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Init opens the SQLite database with optimal connection pooling for concurrency.
// Returns a DBPair with separate reader and writer pools.
func Init(dbPath string) (*DBPair, error) {
	if dbPath == "" {
		return nil, errors.New("db path is required")
	}

	if err := ensureDir(dbPath); err != nil {
		return nil, err
	}

	// Writer: Single connection, handles all writes
	// - _journal=WAL: Write-ahead logging for concurrent reads
	// - _busy_timeout=5000: Wait up to 5 seconds for locks
	// - cache=shared: Share cache between connections for consistency
	// - mode=rwc: Read-write-create mode
	writerConnStr := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000&cache=shared&mode=rwc", dbPath)
	writer, err := sql.Open("sqlite3", writerConnStr)
	if err != nil {
		return nil, fmt.Errorf("open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)        // SQLite serializes writes anyway
	writer.SetMaxIdleConns(1)        // Keep one connection warm
	writer.SetConnMaxLifetime(time.Hour)

	// Apply PRAGMAs on writer (affects the database)
	if _, err := writer.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		writer.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := writer.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		writer.Close()
		return nil, fmt.Errorf("set foreign_keys: %w", err)
	}

	// Reader: Multiple connections for concurrent reads
	// - mode=ro: Read-only mode
	readerConnStr := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000&cache=shared&mode=ro", dbPath)
	reader, err := sql.Open("sqlite3", readerConnStr)
	if err != nil {
		writer.Close()
		return nil, fmt.Errorf("open reader: %w", err)
	}
	reader.SetMaxOpenConns(4)        // Allow 4 concurrent readers
	reader.SetMaxIdleConns(2)        // Keep 2 connections warm
	reader.SetConnMaxLifetime(time.Hour)

	// Apply schema using writer
	if _, err := writer.Exec(schemaSQL); err != nil {
		reader.Close()
		writer.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := runMigrations(writer); err != nil {
		reader.Close()
		writer.Close()
		return nil, err
	}

	return &DBPair{reader: reader, writer: writer}, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

func runMigrations(db *sql.DB) error {
	jobsColumns, err := tableColumns(db, "jobs")
	if err != nil {
		return err
	}

	if !jobsColumns["retry_after"] {
		if _, err := db.Exec("ALTER TABLE jobs ADD COLUMN retry_after TEXT"); err != nil {
			return fmt.Errorf("add jobs.retry_after: %w", err)
		}
	}

	if !jobsColumns["claimed_at"] {
		if _, err := db.Exec("ALTER TABLE jobs ADD COLUMN claimed_at TEXT"); err != nil {
			return fmt.Errorf("add jobs.claimed_at: %w", err)
		}
	}

	if !jobsColumns["idempotency_key"] {
		if _, err := db.Exec("ALTER TABLE jobs ADD COLUMN idempotency_key TEXT"); err != nil {
			return fmt.Errorf("add jobs.idempotency_key: %w", err)
		}
		if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs(idempotency_key) WHERE idempotency_key IS NOT NULL"); err != nil {
			return fmt.Errorf("create idx_jobs_idempotency: %w", err)
		}
	}

	routinesColumns, err := tableColumns(db, "routines")
	if err != nil {
		return err
	}

	if !routinesColumns["speakers_json"] {
		if _, err := db.Exec("ALTER TABLE routines ADD COLUMN speakers_json TEXT"); err != nil {
			return fmt.Errorf("add routines.speakers_json: %w", err)
		}
	}

	if !routinesColumns["last_run_at"] {
		if _, err := db.Exec("ALTER TABLE routines ADD COLUMN last_run_at TEXT"); err != nil {
			return fmt.Errorf("add routines.last_run_at: %w", err)
		}
	}

	if err := backfillSpeakersJSON(db); err != nil {
		return err
	}

	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var defaultVal sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

type sceneMember struct {
	DeviceID     string `json:"device_id"`
	TargetVolume *int   `json:"target_volume"`
}

type speaker struct {
	DeviceID string `json:"device_id"`
	Volume   *int   `json:"volume"`
}

func backfillSpeakersJSON(db *sql.DB) error {
	rows, err := db.Query(`
    SELECT r.routine_id, r.scene_id
    FROM routines r
    WHERE r.speakers_json IS NULL
  `)
	if err != nil {
		return err
	}
	defer rows.Close()

	type routineRow struct {
		routineID string
		sceneID   string
	}

	var routines []routineRow
	for rows.Next() {
		var r routineRow
		if err := rows.Scan(&r.routineID, &r.sceneID); err != nil {
			return err
		}
		routines = append(routines, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(routines) == 0 {
		return nil
	}

	getSceneStmt, err := db.Prepare("SELECT members FROM scenes WHERE scene_id = ?")
	if err != nil {
		return err
	}
	defer getSceneStmt.Close()

	updateStmt, err := db.Prepare("UPDATE routines SET speakers_json = ?, updated_at = ? WHERE routine_id = ?")
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	now := nowISO()

	for _, routine := range routines {
		var membersJSON string
		if err := getSceneStmt.QueryRow(routine.sceneID).Scan(&membersJSON); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}

		var members []sceneMember
		if err := json.Unmarshal([]byte(membersJSON), &members); err != nil {
			continue
		}

		speakers := make([]speaker, 0, len(members))
		for _, m := range members {
			speakers = append(speakers, speaker{DeviceID: m.DeviceID, Volume: m.TargetVolume})
		}

		payload, err := json.Marshal(speakers)
		if err != nil {
			continue
		}

		if _, err := updateStmt.Exec(string(payload), now, routine.routineID); err != nil {
			return err
		}
	}

	return nil
}

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05Z07:00")
}
