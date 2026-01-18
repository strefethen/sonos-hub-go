package scene

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// ScenesRepository handles database operations for scenes.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type ScenesRepository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewScenesRepository creates a new ScenesRepository.
func NewScenesRepository(dbPair DBPair) *ScenesRepository {
	return &ScenesRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// Create creates a new scene.
func (r *ScenesRepository) Create(input CreateSceneInput) (*Scene, error) {
	sceneID := uuid.New().String()
	now := nowISO()

	coordinatorPref := input.CoordinatorPreference
	if coordinatorPref == "" {
		coordinatorPref = string(CoordinatorPreferenceArcFirst)
	}

	fallbackPolicy := input.FallbackPolicy
	if fallbackPolicy == "" {
		fallbackPolicy = string(FallbackPolicyPlaybaseIfArcTVActive)
	}

	members := input.Members
	if members == nil {
		members = []SceneMember{}
	}

	membersJSON, err := json.Marshal(members)
	if err != nil {
		return nil, err
	}

	var volumeRampJSON []byte
	if input.VolumeRamp != nil {
		volumeRampJSON, err = json.Marshal(input.VolumeRamp)
		if err != nil {
			return nil, err
		}
	}

	var teardownJSON []byte
	if input.Teardown != nil {
		teardownJSON, err = json.Marshal(input.Teardown)
		if err != nil {
			return nil, err
		}
	}

	_, err = r.writer.Exec(`
		INSERT INTO scenes (scene_id, name, description, coordinator_preference, fallback_policy, members, volume_ramp, teardown, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sceneID, input.Name, input.Description, coordinatorPref, fallbackPolicy, string(membersJSON), nullableString(volumeRampJSON), nullableString(teardownJSON), now, now)
	if err != nil {
		return nil, err
	}

	return r.GetByID(sceneID)
}

// GetByID retrieves a scene by ID.
func (r *ScenesRepository) GetByID(sceneID string) (*Scene, error) {
	row := r.reader.QueryRow(`
		SELECT scene_id, name, description, coordinator_preference, fallback_policy, members, volume_ramp, teardown, created_at, updated_at
		FROM scenes
		WHERE scene_id = ?
	`, sceneID)

	return r.scanScene(row)
}

// List retrieves scenes with pagination.
func (r *ScenesRepository) List(limit, offset int) ([]Scene, int, error) {
	var total int
	err := r.reader.QueryRow("SELECT COUNT(*) FROM scenes").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.reader.Query(`
		SELECT scene_id, name, description, coordinator_preference, fallback_policy, members, volume_ramp, teardown, created_at, updated_at
		FROM scenes
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var scenes []Scene
	for rows.Next() {
		scene, err := r.scanSceneRows(rows)
		if err != nil {
			return nil, 0, err
		}
		scenes = append(scenes, *scene)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if scenes == nil {
		scenes = []Scene{}
	}

	return scenes, total, nil
}

// Update updates a scene.
func (r *ScenesRepository) Update(sceneID string, input UpdateSceneInput) (*Scene, error) {
	existing, err := r.GetByID(sceneID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	name := existing.Name
	if input.Name != nil {
		name = *input.Name
	}

	description := existing.Description
	if input.Description != nil {
		description = input.Description
	}

	coordinatorPref := existing.CoordinatorPreference
	if input.CoordinatorPreference != nil {
		coordinatorPref = *input.CoordinatorPreference
	}

	fallbackPolicy := existing.FallbackPolicy
	if input.FallbackPolicy != nil {
		fallbackPolicy = *input.FallbackPolicy
	}

	members := existing.Members
	if input.Members != nil {
		members = input.Members
	}

	volumeRamp := existing.VolumeRamp
	if input.VolumeRamp != nil {
		volumeRamp = input.VolumeRamp
	}

	teardown := existing.Teardown
	if input.Teardown != nil {
		teardown = input.Teardown
	}

	membersJSON, err := json.Marshal(members)
	if err != nil {
		return nil, err
	}

	var volumeRampJSON []byte
	if volumeRamp != nil {
		volumeRampJSON, err = json.Marshal(volumeRamp)
		if err != nil {
			return nil, err
		}
	}

	var teardownJSON []byte
	if teardown != nil {
		teardownJSON, err = json.Marshal(teardown)
		if err != nil {
			return nil, err
		}
	}

	now := nowISO()
	_, err = r.writer.Exec(`
		UPDATE scenes
		SET name = ?, description = ?, coordinator_preference = ?, fallback_policy = ?, members = ?, volume_ramp = ?, teardown = ?, updated_at = ?
		WHERE scene_id = ?
	`, name, description, coordinatorPref, fallbackPolicy, string(membersJSON), nullableString(volumeRampJSON), nullableString(teardownJSON), now, sceneID)
	if err != nil {
		return nil, err
	}

	return r.GetByID(sceneID)
}

// Delete deletes a scene and its executions.
func (r *ScenesRepository) Delete(sceneID string) error {
	// Delete executions first (FK constraint)
	_, err := r.writer.Exec("DELETE FROM scene_executions WHERE scene_id = ?", sceneID)
	if err != nil {
		return err
	}

	result, err := r.writer.Exec("DELETE FROM scenes WHERE scene_id = ?", sceneID)
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

func (r *ScenesRepository) scanScene(row *sql.Row) (*Scene, error) {
	var scene Scene
	var description sql.NullString
	var membersJSON string
	var volumeRampJSON sql.NullString
	var teardownJSON sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(
		&scene.SceneID,
		&scene.Name,
		&description,
		&scene.CoordinatorPreference,
		&scene.FallbackPolicy,
		&membersJSON,
		&volumeRampJSON,
		&teardownJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseScene(&scene, description, membersJSON, volumeRampJSON, teardownJSON, createdAt, updatedAt)
}

func (r *ScenesRepository) scanSceneRows(rows *sql.Rows) (*Scene, error) {
	var scene Scene
	var description sql.NullString
	var membersJSON string
	var volumeRampJSON sql.NullString
	var teardownJSON sql.NullString
	var createdAt, updatedAt string

	err := rows.Scan(
		&scene.SceneID,
		&scene.Name,
		&description,
		&scene.CoordinatorPreference,
		&scene.FallbackPolicy,
		&membersJSON,
		&volumeRampJSON,
		&teardownJSON,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	return r.parseScene(&scene, description, membersJSON, volumeRampJSON, teardownJSON, createdAt, updatedAt)
}

func (r *ScenesRepository) parseScene(scene *Scene, description sql.NullString, membersJSON string, volumeRampJSON, teardownJSON sql.NullString, createdAt, updatedAt string) (*Scene, error) {
	if description.Valid {
		scene.Description = &description.String
	}

	if err := json.Unmarshal([]byte(membersJSON), &scene.Members); err != nil {
		return nil, err
	}

	if volumeRampJSON.Valid && volumeRampJSON.String != "" {
		var volumeRamp VolumeRamp
		if err := json.Unmarshal([]byte(volumeRampJSON.String), &volumeRamp); err != nil {
			return nil, err
		}
		scene.VolumeRamp = &volumeRamp
	}

	if teardownJSON.Valid && teardownJSON.String != "" {
		var teardown Teardown
		if err := json.Unmarshal([]byte(teardownJSON.String), &teardown); err != nil {
			return nil, err
		}
		scene.Teardown = &teardown
	}

	var err error
	scene.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		scene.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	scene.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		scene.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return scene, nil
}

// ExecutionsRepository handles database operations for scene executions.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type ExecutionsRepository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewExecutionsRepository creates a new ExecutionsRepository.
func NewExecutionsRepository(dbPair DBPair) *ExecutionsRepository {
	return &ExecutionsRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// Create creates a new scene execution.
func (r *ExecutionsRepository) Create(input CreateExecutionInput) (*SceneExecution, error) {
	execID := uuid.New().String()
	now := nowISO()
	steps := DefaultExecutionSteps()

	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return nil, err
	}

	_, err = r.writer.Exec(`
		INSERT INTO scene_executions (scene_execution_id, scene_id, idempotency_key, status, started_at, steps)
		VALUES (?, ?, ?, ?, ?, ?)
	`, execID, input.SceneID, input.IdempotencyKey, string(ExecutionStatusStarting), now, string(stepsJSON))
	if err != nil {
		return nil, err
	}

	return r.GetByID(execID)
}

// GetByID retrieves an execution by ID.
func (r *ExecutionsRepository) GetByID(execID string) (*SceneExecution, error) {
	row := r.reader.QueryRow(`
		SELECT scene_execution_id, scene_id, idempotency_key, coordinator_used_device_id, status, started_at, ended_at, steps, verification, error
		FROM scene_executions
		WHERE scene_execution_id = ?
	`, execID)

	return r.scanExecution(row)
}

// GetByIdempotencyKey retrieves an execution by idempotency key.
func (r *ExecutionsRepository) GetByIdempotencyKey(key string) (*SceneExecution, error) {
	row := r.reader.QueryRow(`
		SELECT scene_execution_id, scene_id, idempotency_key, coordinator_used_device_id, status, started_at, ended_at, steps, verification, error
		FROM scene_executions
		WHERE idempotency_key = ?
	`, key)

	return r.scanExecution(row)
}

// ListBySceneID retrieves executions for a scene with pagination.
func (r *ExecutionsRepository) ListBySceneID(sceneID string, limit, offset int) ([]SceneExecution, int, error) {
	var total int
	err := r.reader.QueryRow("SELECT COUNT(*) FROM scene_executions WHERE scene_id = ?", sceneID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.reader.Query(`
		SELECT scene_execution_id, scene_id, idempotency_key, coordinator_used_device_id, status, started_at, ended_at, steps, verification, error
		FROM scene_executions
		WHERE scene_id = ?
		ORDER BY started_at DESC
		LIMIT ? OFFSET ?
	`, sceneID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var executions []SceneExecution
	for rows.Next() {
		exec, err := r.scanExecutionRows(rows)
		if err != nil {
			return nil, 0, err
		}
		executions = append(executions, *exec)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if executions == nil {
		executions = []SceneExecution{}
	}

	return executions, total, nil
}

// UpdateStep updates a single step in an execution using a transaction.
func (r *ExecutionsRepository) UpdateStep(execID, stepName string, update StepUpdate) error {
	tx, err := r.writer.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var stepsJSON string
	err = tx.QueryRow("SELECT steps FROM scene_executions WHERE scene_execution_id = ?", execID).Scan(&stepsJSON)
	if err != nil {
		return err
	}

	var steps []ExecutionStep
	if err = json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return err
	}

	found := false
	for i := range steps {
		if steps[i].Step == stepName {
			found = true
			if update.Status != nil {
				steps[i].Status = *update.Status
			}
			if update.StartedAt != nil {
				steps[i].StartedAt = update.StartedAt
			}
			if update.EndedAt != nil {
				steps[i].EndedAt = update.EndedAt
			}
			if update.Error != nil {
				steps[i].Error = *update.Error
			}
			if update.Details != nil {
				steps[i].Details = update.Details
			}
			break
		}
	}

	if !found {
		return errors.New("step not found: " + stepName)
	}

	newStepsJSON, err := json.Marshal(steps)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE scene_executions SET steps = ? WHERE scene_execution_id = ?", string(newStepsJSON), execID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SetCoordinator sets the coordinator device ID.
func (r *ExecutionsRepository) SetCoordinator(execID, deviceID string) error {
	_, err := r.writer.Exec(`
		UPDATE scene_executions
		SET coordinator_used_device_id = ?
		WHERE scene_execution_id = ?
	`, deviceID, execID)
	return err
}

// Complete marks an execution as complete.
func (r *ExecutionsRepository) Complete(execID string, status ExecutionStatus, verification *Verification, errMsg *string) error {
	now := nowISO()

	var verificationJSON []byte
	var err error
	if verification != nil {
		verificationJSON, err = json.Marshal(verification)
		if err != nil {
			return err
		}
	}

	_, err = r.writer.Exec(`
		UPDATE scene_executions
		SET status = ?, ended_at = ?, verification = ?, error = ?
		WHERE scene_execution_id = ?
	`, string(status), now, nullableString(verificationJSON), errMsg, execID)
	return err
}

func (r *ExecutionsRepository) scanExecution(row *sql.Row) (*SceneExecution, error) {
	var exec SceneExecution
	var idempotencyKey sql.NullString
	var coordinator sql.NullString
	var status string
	var startedAt string
	var endedAt sql.NullString
	var stepsJSON string
	var verificationJSON sql.NullString
	var errorMsg sql.NullString

	err := row.Scan(
		&exec.SceneExecutionID,
		&exec.SceneID,
		&idempotencyKey,
		&coordinator,
		&status,
		&startedAt,
		&endedAt,
		&stepsJSON,
		&verificationJSON,
		&errorMsg,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseExecution(&exec, idempotencyKey, coordinator, status, startedAt, endedAt, stepsJSON, verificationJSON, errorMsg)
}

func (r *ExecutionsRepository) scanExecutionRows(rows *sql.Rows) (*SceneExecution, error) {
	var exec SceneExecution
	var idempotencyKey sql.NullString
	var coordinator sql.NullString
	var status string
	var startedAt string
	var endedAt sql.NullString
	var stepsJSON string
	var verificationJSON sql.NullString
	var errorMsg sql.NullString

	err := rows.Scan(
		&exec.SceneExecutionID,
		&exec.SceneID,
		&idempotencyKey,
		&coordinator,
		&status,
		&startedAt,
		&endedAt,
		&stepsJSON,
		&verificationJSON,
		&errorMsg,
	)
	if err != nil {
		return nil, err
	}

	return r.parseExecution(&exec, idempotencyKey, coordinator, status, startedAt, endedAt, stepsJSON, verificationJSON, errorMsg)
}

func (r *ExecutionsRepository) parseExecution(exec *SceneExecution, idempotencyKey, coordinator sql.NullString, status, startedAt string, endedAt sql.NullString, stepsJSON string, verificationJSON, errorMsg sql.NullString) (*SceneExecution, error) {
	if idempotencyKey.Valid {
		exec.IdempotencyKey = &idempotencyKey.String
	}
	if coordinator.Valid {
		exec.CoordinatorUsedDeviceID = &coordinator.String
	}

	exec.Status = ExecutionStatus(status)

	var err error
	exec.StartedAt, err = time.Parse(time.RFC3339, startedAt)
	if err != nil {
		exec.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startedAt)
	}

	if endedAt.Valid {
		t, err := time.Parse(time.RFC3339, endedAt.String)
		if err != nil {
			t, _ = time.Parse("2006-01-02 15:04:05", endedAt.String)
		}
		exec.EndedAt = &t
	}

	if err := json.Unmarshal([]byte(stepsJSON), &exec.Steps); err != nil {
		return nil, err
	}

	if verificationJSON.Valid && verificationJSON.String != "" {
		var verification Verification
		if err := json.Unmarshal([]byte(verificationJSON.String), &verification); err != nil {
			return nil, err
		}
		exec.Verification = &verification
	}

	if errorMsg.Valid {
		exec.Error = &errorMsg.String
	}

	return exec, nil
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
