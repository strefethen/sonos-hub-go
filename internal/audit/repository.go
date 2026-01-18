package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EventLevel represents the severity level of an audit event.
type EventLevel string

const (
	EventLevelDebug EventLevel = "DEBUG"
	EventLevelInfo  EventLevel = "INFO"
	EventLevelWarn  EventLevel = "WARN"
	EventLevelError EventLevel = "ERROR"
)

// AuditEvent represents a single audit event.
type AuditEvent struct {
	EventID          string         `json:"event_id"`
	Timestamp        time.Time      `json:"timestamp"`
	Type             string         `json:"type"`
	Level            EventLevel     `json:"level"`
	RequestID        *string        `json:"request_id,omitempty"`
	RoutineID        *string        `json:"routine_id,omitempty"`
	JobID            *string        `json:"job_id,omitempty"`
	SceneExecutionID *string        `json:"scene_execution_id,omitempty"`
	DeviceID         *string        `json:"device_id,omitempty"`
	Message          string         `json:"message"`
	Payload          map[string]any `json:"payload"`
}

// WriteEventInput contains the fields for creating a new audit event.
type WriteEventInput struct {
	Type             string         `json:"type"`
	Level            *EventLevel    `json:"level,omitempty"`
	RequestID        *string        `json:"request_id,omitempty"`
	RoutineID        *string        `json:"routine_id,omitempty"`
	JobID            *string        `json:"job_id,omitempty"`
	SceneExecutionID *string        `json:"scene_execution_id,omitempty"`
	DeviceID         *string        `json:"device_id,omitempty"`
	Message          string         `json:"message"`
	Payload          map[string]any `json:"payload,omitempty"`
}

// EventQueryFilters contains optional filters for querying events.
type EventQueryFilters struct {
	Type             *string     `json:"type,omitempty"`
	Level            *EventLevel `json:"level,omitempty"`
	StartDate        *string     `json:"start_date,omitempty"` // ISO 8601 format
	EndDate          *string     `json:"end_date,omitempty"`   // ISO 8601 format
	JobID            *string     `json:"job_id,omitempty"`
	RoutineID        *string     `json:"routine_id,omitempty"`
	SceneExecutionID *string     `json:"scene_execution_id,omitempty"`
	DeviceID         *string     `json:"device_id,omitempty"`
	Limit            int         `json:"limit,omitempty"`
	Offset           int         `json:"offset,omitempty"`
}

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// Repository handles database operations for audit events.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type Repository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewRepository creates a new audit Repository.
func NewRepository(dbPair DBPair) *Repository {
	return &Repository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// InsertEvent writes a new audit event to the database.
// Generates UUID, captures timestamp, defaults level to INFO.
func (r *Repository) InsertEvent(input WriteEventInput) (*AuditEvent, error) {
	eventID := uuid.New().String()
	timestamp := nowISO()

	level := EventLevelInfo
	if input.Level != nil {
		level = *input.Level
	}

	payload := input.Payload
	if payload == nil {
		payload = map[string]any{}
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	_, err = r.writer.Exec(`
		INSERT INTO audit_events (event_id, timestamp, type, level, request_id, routine_id, job_id, scene_execution_id, device_id, message, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, eventID, timestamp, input.Type, string(level), input.RequestID, input.RoutineID, input.JobID, input.SceneExecutionID, input.DeviceID, input.Message, string(payloadJSON))
	if err != nil {
		return nil, err
	}

	return r.GetEvent(eventID)
}

// WriteEvent is an alias for InsertEvent to match the service interface.
func (r *Repository) WriteEvent(input WriteEventInput) (*AuditEvent, error) {
	return r.InsertEvent(input)
}

// GetEvent retrieves a single event by ID.
// Returns nil, nil if not found.
func (r *Repository) GetEvent(eventID string) (*AuditEvent, error) {
	row := r.reader.QueryRow(`
		SELECT event_id, timestamp, type, level, request_id, routine_id, job_id, scene_execution_id, device_id, message, payload
		FROM audit_events
		WHERE event_id = ?
	`, eventID)

	return r.scanEvent(row)
}

// GetByID is an alias for GetEvent to match the service interface.
func (r *Repository) GetByID(eventID string) (*AuditEvent, error) {
	return r.GetEvent(eventID)
}

// QueryEvents retrieves events matching filters with pagination.
// Builds WHERE clause dynamically based on provided filters.
// Orders by timestamp DESC (newest first).
// Returns events, total count, and error.
func (r *Repository) QueryEvents(filters EventQueryFilters) ([]AuditEvent, int, error) {
	whereClause, args := r.buildWhereClause(filters)

	// Get total count first
	countQuery := "SELECT COUNT(*) FROM audit_events " + whereClause
	var total int
	err := r.reader.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limit := filters.Limit
	if limit <= 0 {
		limit = 100 // default limit
	}

	query := `
		SELECT event_id, timestamp, type, level, request_id, routine_id, job_id, scene_execution_id, device_id, message, payload
		FROM audit_events
		` + whereClause + `
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?
	`
	queryArgs := append(args, limit, filters.Offset)

	rows, err := r.reader.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var events []AuditEvent
	for rows.Next() {
		event, err := r.scanEventRows(rows)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, *event)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if events == nil {
		events = []AuditEvent{}
	}

	return events, total, nil
}

// CountEvents counts total events matching filters (for pagination).
func (r *Repository) CountEvents(filters EventQueryFilters) (int, error) {
	whereClause, args := r.buildWhereClause(filters)

	query := "SELECT COUNT(*) FROM audit_events " + whereClause

	var count int
	err := r.reader.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// PruneOldEvents deletes events older than retentionDays.
// Returns number of rows deleted.
func (r *Repository) PruneOldEvents(retentionDays int) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).Format(time.RFC3339)

	result, err := r.writer.Exec(`
		DELETE FROM audit_events
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// Prune deletes events older than the cutoff time.
// Returns number of rows deleted. This method matches the service interface.
func (r *Repository) Prune(cutoff time.Time) (int64, error) {
	result, err := r.writer.Exec(`
		DELETE FROM audit_events
		WHERE timestamp < ?
	`, cutoff.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// buildWhereClause builds a dynamic WHERE clause based on provided filters.
func (r *Repository) buildWhereClause(filters EventQueryFilters) (string, []any) {
	conditions := []string{}
	args := []any{}

	if filters.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *filters.Type)
	}
	if filters.Level != nil {
		conditions = append(conditions, "level = ?")
		args = append(args, string(*filters.Level))
	}
	if filters.RoutineID != nil {
		conditions = append(conditions, "routine_id = ?")
		args = append(args, *filters.RoutineID)
	}
	if filters.JobID != nil {
		conditions = append(conditions, "job_id = ?")
		args = append(args, *filters.JobID)
	}
	if filters.SceneExecutionID != nil {
		conditions = append(conditions, "scene_execution_id = ?")
		args = append(args, *filters.SceneExecutionID)
	}
	if filters.DeviceID != nil {
		conditions = append(conditions, "device_id = ?")
		args = append(args, *filters.DeviceID)
	}
	if filters.StartDate != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *filters.StartDate)
	}
	if filters.EndDate != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *filters.EndDate)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

func (r *Repository) scanEvent(row *sql.Row) (*AuditEvent, error) {
	var event AuditEvent
	var timestamp string
	var level string
	var requestID sql.NullString
	var routineID sql.NullString
	var jobID sql.NullString
	var sceneExecutionID sql.NullString
	var deviceID sql.NullString
	var payloadJSON string

	err := row.Scan(
		&event.EventID,
		&timestamp,
		&event.Type,
		&level,
		&requestID,
		&routineID,
		&jobID,
		&sceneExecutionID,
		&deviceID,
		&event.Message,
		&payloadJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseEvent(&event, timestamp, level, requestID, routineID, jobID, sceneExecutionID, deviceID, payloadJSON)
}

func (r *Repository) scanEventRows(rows *sql.Rows) (*AuditEvent, error) {
	var event AuditEvent
	var timestamp string
	var level string
	var requestID sql.NullString
	var routineID sql.NullString
	var jobID sql.NullString
	var sceneExecutionID sql.NullString
	var deviceID sql.NullString
	var payloadJSON string

	err := rows.Scan(
		&event.EventID,
		&timestamp,
		&event.Type,
		&level,
		&requestID,
		&routineID,
		&jobID,
		&sceneExecutionID,
		&deviceID,
		&event.Message,
		&payloadJSON,
	)
	if err != nil {
		return nil, err
	}

	return r.parseEvent(&event, timestamp, level, requestID, routineID, jobID, sceneExecutionID, deviceID, payloadJSON)
}

func (r *Repository) parseEvent(event *AuditEvent, timestamp, level string, requestID, routineID, jobID, sceneExecutionID, deviceID sql.NullString, payloadJSON string) (*AuditEvent, error) {
	var err error
	event.Timestamp, err = time.Parse(time.RFC3339, timestamp)
	if err != nil {
		event.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestamp)
	}

	event.Level = EventLevel(level)

	if requestID.Valid {
		event.RequestID = &requestID.String
	}
	if routineID.Valid {
		event.RoutineID = &routineID.String
	}
	if jobID.Valid {
		event.JobID = &jobID.String
	}
	if sceneExecutionID.Valid {
		event.SceneExecutionID = &sceneExecutionID.String
	}
	if deviceID.Valid {
		event.DeviceID = &deviceID.String
	}

	if err := json.Unmarshal([]byte(payloadJSON), &event.Payload); err != nil {
		return nil, err
	}

	return event, nil
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
