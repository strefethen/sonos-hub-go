package music

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DBPair interface for dependency injection (matches db.DBPair).
type DBPair interface {
	Reader() *sql.DB
	Writer() *sql.DB
}

// ==========================================================================
// MusicSetRepository
// ==========================================================================

// MusicSetRepository handles database operations for music sets.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type MusicSetRepository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewMusicSetRepository creates a new MusicSetRepository.
func NewMusicSetRepository(dbPair DBPair) *MusicSetRepository {
	return &MusicSetRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// Create creates a new music set.
func (r *MusicSetRepository) Create(input CreateSetInput) (*MusicSet, error) {
	setID := uuid.New().String()
	now := nowISO()

	_, err := r.writer.Exec(`
		INSERT INTO music_sets (set_id, name, selection_policy, current_index, occasion_start, occasion_end, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, setID, input.Name, input.SelectionPolicy, 0, input.OccasionStart, input.OccasionEnd, now, now)
	if err != nil {
		return nil, err
	}

	return r.GetByID(setID)
}

// GetByID retrieves a music set by ID.
func (r *MusicSetRepository) GetByID(setID string) (*MusicSet, error) {
	row := r.reader.QueryRow(`
		SELECT set_id, name, selection_policy, current_index, occasion_start, occasion_end, created_at, updated_at
		FROM music_sets
		WHERE set_id = ?
	`, setID)

	return r.scanMusicSet(row)
}

// List retrieves music sets with pagination.
func (r *MusicSetRepository) List(limit, offset int) ([]MusicSet, int, error) {
	var total int
	err := r.reader.QueryRow("SELECT COUNT(*) FROM music_sets").Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.reader.Query(`
		SELECT set_id, name, selection_policy, current_index, occasion_start, occasion_end, created_at, updated_at
		FROM music_sets
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sets []MusicSet
	for rows.Next() {
		set, err := r.scanMusicSetRows(rows)
		if err != nil {
			return nil, 0, err
		}
		sets = append(sets, *set)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if sets == nil {
		sets = []MusicSet{}
	}

	return sets, total, nil
}

// Update updates a music set.
func (r *MusicSetRepository) Update(setID string, input UpdateSetInput) (*MusicSet, error) {
	existing, err := r.GetByID(setID)
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

	selectionPolicy := existing.SelectionPolicy
	if input.SelectionPolicy != nil {
		selectionPolicy = *input.SelectionPolicy
	}

	// Handle occasion fields - nil means keep existing, empty string means clear
	var occasionStart, occasionEnd *string
	if input.OccasionStart != nil {
		if *input.OccasionStart == "" {
			occasionStart = nil // clear
		} else {
			occasionStart = input.OccasionStart
		}
	} else {
		occasionStart = existing.OccasionStart
	}
	if input.OccasionEnd != nil {
		if *input.OccasionEnd == "" {
			occasionEnd = nil // clear
		} else {
			occasionEnd = input.OccasionEnd
		}
	} else {
		occasionEnd = existing.OccasionEnd
	}

	now := nowISO()
	_, err = r.writer.Exec(`
		UPDATE music_sets
		SET name = ?, selection_policy = ?, occasion_start = ?, occasion_end = ?, updated_at = ?
		WHERE set_id = ?
	`, name, selectionPolicy, occasionStart, occasionEnd, now, setID)
	if err != nil {
		return nil, err
	}

	return r.GetByID(setID)
}

// Delete deletes a music set.
func (r *MusicSetRepository) Delete(setID string) error {
	result, err := r.writer.Exec("DELETE FROM music_sets WHERE set_id = ?", setID)
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

// UpdateCurrentIndex updates the current index of a music set.
func (r *MusicSetRepository) UpdateCurrentIndex(setID string, index int) error {
	now := nowISO()
	result, err := r.writer.Exec(`
		UPDATE music_sets
		SET current_index = ?, updated_at = ?
		WHERE set_id = ?
	`, index, now, setID)
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

// IncrementIndex atomically increments the current index and returns the new value.
func (r *MusicSetRepository) IncrementIndex(setID string) (int, error) {
	tx, err := r.writer.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	now := nowISO()
	_, err = tx.Exec(`
		UPDATE music_sets
		SET current_index = current_index + 1, updated_at = ?
		WHERE set_id = ?
	`, now, setID)
	if err != nil {
		return 0, err
	}

	var newIndex int
	err = tx.QueryRow(`
		SELECT current_index
		FROM music_sets
		WHERE set_id = ?
	`, setID).Scan(&newIndex)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, sql.ErrNoRows
		}
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return newIndex, nil
}

func (r *MusicSetRepository) scanMusicSet(row *sql.Row) (*MusicSet, error) {
	var set MusicSet
	var createdAt, updatedAt string
	var occasionStart, occasionEnd sql.NullString

	err := row.Scan(
		&set.SetID,
		&set.Name,
		&set.SelectionPolicy,
		&set.CurrentIndex,
		&occasionStart,
		&occasionEnd,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if occasionStart.Valid {
		set.OccasionStart = &occasionStart.String
	}
	if occasionEnd.Valid {
		set.OccasionEnd = &occasionEnd.String
	}

	return r.parseMusicSet(&set, createdAt, updatedAt)
}

func (r *MusicSetRepository) scanMusicSetRows(rows *sql.Rows) (*MusicSet, error) {
	var set MusicSet
	var createdAt, updatedAt string
	var occasionStart, occasionEnd sql.NullString

	err := rows.Scan(
		&set.SetID,
		&set.Name,
		&set.SelectionPolicy,
		&set.CurrentIndex,
		&occasionStart,
		&occasionEnd,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if occasionStart.Valid {
		set.OccasionStart = &occasionStart.String
	}
	if occasionEnd.Valid {
		set.OccasionEnd = &occasionEnd.String
	}

	return r.parseMusicSet(&set, createdAt, updatedAt)
}

func (r *MusicSetRepository) parseMusicSet(set *MusicSet, createdAt, updatedAt string) (*MusicSet, error) {
	var err error
	set.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		set.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	set.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		set.UpdatedAt, _ = time.Parse("2006-01-02 15:04:05", updatedAt)
	}

	return set, nil
}

// ==========================================================================
// SetItemRepository
// ==========================================================================

// SetItemRepository handles database operations for set items.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type SetItemRepository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewSetItemRepository creates a new SetItemRepository.
func NewSetItemRepository(dbPair DBPair) *SetItemRepository {
	return &SetItemRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// Add adds an item to a music set.
func (r *SetItemRepository) Add(setID string, input AddItemInput) (*SetItem, error) {
	// Get the next position
	var maxPosition sql.NullInt64
	err := r.reader.QueryRow(`
		SELECT MAX(position)
		FROM set_items
		WHERE set_id = ?
	`, setID).Scan(&maxPosition)
	if err != nil {
		return nil, err
	}

	nextPosition := 0
	if maxPosition.Valid {
		nextPosition = int(maxPosition.Int64) + 1
	}

	now := nowISO()
	contentType := input.ContentType
	if contentType == "" {
		contentType = "sonos_favorite"
	}

	_, err = r.writer.Exec(`
		INSERT INTO set_items (set_id, sonos_favorite_id, position, added_at, service_logo_url, service_name, content_type, content_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, setID, input.SonosFavoriteID, nextPosition, now, input.ServiceLogoURL, input.ServiceName, contentType, input.ContentJSON)
	if err != nil {
		return nil, err
	}

	return r.GetItem(setID, input.SonosFavoriteID)
}

// Remove removes an item from a music set.
func (r *SetItemRepository) Remove(setID, sonosFavoriteID string) error {
	result, err := r.writer.Exec(`
		DELETE FROM set_items
		WHERE set_id = ? AND sonos_favorite_id = ?
	`, setID, sonosFavoriteID)
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

// GetItems retrieves all items in a music set ordered by position.
func (r *SetItemRepository) GetItems(setID string) ([]SetItem, error) {
	rows, err := r.reader.Query(`
		SELECT set_id, sonos_favorite_id, position, added_at, service_logo_url, service_name, content_type, content_json
		FROM set_items
		WHERE set_id = ?
		ORDER BY position ASC
	`, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []SetItem
	for rows.Next() {
		item, err := r.scanSetItemRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if items == nil {
		items = []SetItem{}
	}

	return items, nil
}

// GetItem retrieves a specific item from a music set.
func (r *SetItemRepository) GetItem(setID, sonosFavoriteID string) (*SetItem, error) {
	row := r.reader.QueryRow(`
		SELECT set_id, sonos_favorite_id, position, added_at, service_logo_url, service_name, content_type, content_json
		FROM set_items
		WHERE set_id = ? AND sonos_favorite_id = ?
	`, setID, sonosFavoriteID)

	return r.scanSetItem(row)
}

// Reorder reorders items in a music set using a transaction.
func (r *SetItemRepository) Reorder(setID string, orderedIDs []string) error {
	tx, err := r.writer.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Update each item's position based on its index in orderedIDs
	for position, sonosFavoriteID := range orderedIDs {
		result, err := tx.Exec(`
			UPDATE set_items
			SET position = ?
			WHERE set_id = ? AND sonos_favorite_id = ?
		`, position, setID, sonosFavoriteID)
		if err != nil {
			return err
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return errors.New("item not found: " + sonosFavoriteID)
		}
	}

	return tx.Commit()
}

// Count returns the number of items in a music set.
func (r *SetItemRepository) Count(setID string) (int, error) {
	var count int
	err := r.reader.QueryRow(`
		SELECT COUNT(*)
		FROM set_items
		WHERE set_id = ?
	`, setID).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetItemsPaginated retrieves items in a music set with database-level pagination.
func (r *SetItemRepository) GetItemsPaginated(setID string, limit, offset int) ([]SetItem, int, error) {
	// Get total count
	var total int
	err := r.reader.QueryRow(`
		SELECT COUNT(*)
		FROM set_items
		WHERE set_id = ?
	`, setID).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated items
	rows, err := r.reader.Query(`
		SELECT set_id, sonos_favorite_id, position, added_at, service_logo_url, service_name, content_type, content_json
		FROM set_items
		WHERE set_id = ?
		ORDER BY position ASC
		LIMIT ? OFFSET ?
	`, setID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []SetItem
	for rows.Next() {
		item, err := r.scanSetItemRows(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if items == nil {
		items = []SetItem{}
	}

	return items, total, nil
}

// GetByPosition retrieves an item by its position in the set.
func (r *SetItemRepository) GetByPosition(setID string, position int) (*SetItem, error) {
	row := r.reader.QueryRow(`
		SELECT set_id, sonos_favorite_id, position, added_at, service_logo_url, service_name, content_type, content_json
		FROM set_items
		WHERE set_id = ? AND position = ?
	`, setID, position)

	return r.scanSetItem(row)
}

// RemoveByPosition removes an item from a music set by its position.
// After removal, it reorders remaining items to maintain contiguous positions.
func (r *SetItemRepository) RemoveByPosition(setID string, position int) error {
	tx, err := r.writer.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Delete the item at the given position
	result, err := tx.Exec(`
		DELETE FROM set_items
		WHERE set_id = ? AND position = ?
	`, setID, position)
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

	// Reorder remaining items to close the gap
	_, err = tx.Exec(`
		UPDATE set_items
		SET position = position - 1
		WHERE set_id = ? AND position > ?
	`, setID, position)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *SetItemRepository) scanSetItem(row *sql.Row) (*SetItem, error) {
	var item SetItem
	var addedAt string
	var serviceLogoURL, serviceName, contentJSON sql.NullString

	err := row.Scan(
		&item.SetID,
		&item.SonosFavoriteID,
		&item.Position,
		&addedAt,
		&serviceLogoURL,
		&serviceName,
		&item.ContentType,
		&contentJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return r.parseSetItem(&item, addedAt, serviceLogoURL, serviceName, contentJSON)
}

func (r *SetItemRepository) scanSetItemRows(rows *sql.Rows) (*SetItem, error) {
	var item SetItem
	var addedAt string
	var serviceLogoURL, serviceName, contentJSON sql.NullString

	err := rows.Scan(
		&item.SetID,
		&item.SonosFavoriteID,
		&item.Position,
		&addedAt,
		&serviceLogoURL,
		&serviceName,
		&item.ContentType,
		&contentJSON,
	)
	if err != nil {
		return nil, err
	}

	return r.parseSetItem(&item, addedAt, serviceLogoURL, serviceName, contentJSON)
}

func (r *SetItemRepository) parseSetItem(item *SetItem, addedAt string, serviceLogoURL, serviceName, contentJSON sql.NullString) (*SetItem, error) {
	var err error
	item.AddedAt, err = time.Parse(time.RFC3339, addedAt)
	if err != nil {
		item.AddedAt, _ = time.Parse("2006-01-02 15:04:05", addedAt)
	}

	if serviceLogoURL.Valid {
		item.ServiceLogoURL = &serviceLogoURL.String
	}
	if serviceName.Valid {
		item.ServiceName = &serviceName.String
	}
	if contentJSON.Valid {
		item.ContentJSON = &contentJSON.String
	}

	return item, nil
}

// ==========================================================================
// PlayHistoryRepository
// ==========================================================================

// PlayHistoryRepository handles database operations for play history.
// Uses separate reader/writer connections for optimal SQLite concurrency.
type PlayHistoryRepository struct {
	reader *sql.DB // For SELECT queries
	writer *sql.DB // For INSERT/UPDATE/DELETE
}

// NewPlayHistoryRepository creates a new PlayHistoryRepository.
func NewPlayHistoryRepository(dbPair DBPair) *PlayHistoryRepository {
	return &PlayHistoryRepository{reader: dbPair.Reader(), writer: dbPair.Writer()}
}

// Record records a play history entry.
func (r *PlayHistoryRepository) Record(sonosFavoriteID string, setID, routineID *string) error {
	now := nowISO()

	_, err := r.writer.Exec(`
		INSERT INTO play_history (sonos_favorite_id, set_id, routine_id, played_at)
		VALUES (?, ?, ?, ?)
	`, sonosFavoriteID, setID, routineID, now)

	return err
}

// GetHistory retrieves play history for a specific favorite.
func (r *PlayHistoryRepository) GetHistory(sonosFavoriteID string, limit int) ([]PlayHistory, error) {
	rows, err := r.reader.Query(`
		SELECT id, sonos_favorite_id, set_id, routine_id, played_at
		FROM play_history
		WHERE sonos_favorite_id = ?
		ORDER BY played_at DESC
		LIMIT ?
	`, sonosFavoriteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []PlayHistory
	for rows.Next() {
		h, err := r.scanPlayHistoryRows(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, *h)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if history == nil {
		history = []PlayHistory{}
	}

	return history, nil
}

// GetSetHistory retrieves play history for a specific set.
func (r *PlayHistoryRepository) GetSetHistory(setID string, limit int) ([]PlayHistory, error) {
	rows, err := r.reader.Query(`
		SELECT id, sonos_favorite_id, set_id, routine_id, played_at
		FROM play_history
		WHERE set_id = ?
		ORDER BY played_at DESC
		LIMIT ?
	`, setID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []PlayHistory
	for rows.Next() {
		h, err := r.scanPlayHistoryRows(rows)
		if err != nil {
			return nil, err
		}
		history = append(history, *h)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if history == nil {
		history = []PlayHistory{}
	}

	return history, nil
}

// WasPlayedRecently checks if a favorite was played within the specified minutes.
func (r *PlayHistoryRepository) WasPlayedRecently(sonosFavoriteID string, withinMinutes int) (bool, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(withinMinutes) * time.Minute).Format(time.RFC3339)

	var count int
	err := r.reader.QueryRow(`
		SELECT COUNT(*)
		FROM play_history
		WHERE sonos_favorite_id = ? AND played_at >= ?
	`, sonosFavoriteID, cutoff).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetRecentlyPlayedInSet returns sonos_favorite_ids that were played recently in a set.
func (r *PlayHistoryRepository) GetRecentlyPlayedInSet(setID string, withinMinutes int) ([]string, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(withinMinutes) * time.Minute).Format(time.RFC3339)

	rows, err := r.reader.Query(`
		SELECT DISTINCT sonos_favorite_id
		FROM play_history
		WHERE set_id = ? AND played_at >= ?
		ORDER BY played_at DESC
	`, setID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var favoriteIDs []string
	for rows.Next() {
		var favoriteID string
		if err := rows.Scan(&favoriteID); err != nil {
			return nil, err
		}
		favoriteIDs = append(favoriteIDs, favoriteID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if favoriteIDs == nil {
		favoriteIDs = []string{}
	}

	return favoriteIDs, nil
}

// Prune deletes old play history records and returns the count of deleted records.
func (r *PlayHistoryRepository) Prune(olderThanDays int) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -olderThanDays).Format(time.RFC3339)

	result, err := r.writer.Exec(`
		DELETE FROM play_history
		WHERE played_at < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

func (r *PlayHistoryRepository) scanPlayHistoryRows(rows *sql.Rows) (*PlayHistory, error) {
	var h PlayHistory
	var setID, routineID sql.NullString
	var playedAt string

	err := rows.Scan(
		&h.ID,
		&h.SonosFavoriteID,
		&setID,
		&routineID,
		&playedAt,
	)
	if err != nil {
		return nil, err
	}

	if setID.Valid {
		h.SetID = &setID.String
	}
	if routineID.Valid {
		h.RoutineID = &routineID.String
	}

	var parseErr error
	h.PlayedAt, parseErr = time.Parse(time.RFC3339, playedAt)
	if parseErr != nil {
		h.PlayedAt, _ = time.Parse("2006-01-02 15:04:05", playedAt)
	}

	return &h, nil
}

// ==========================================================================
// Helpers
// ==========================================================================

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
