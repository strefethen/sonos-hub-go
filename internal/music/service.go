package music

import (
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
)

// Service provides music catalog management functionality.
type Service struct {
	cfg         config.Config
	logger      *log.Logger
	setsRepo    *MusicSetRepository
	itemsRepo   *SetItemRepository
	historyRepo *PlayHistoryRepository
}

// NewService creates a new music catalog service.
// Accepts a DBPair for optimal SQLite concurrency with separate reader/writer pools.
func NewService(cfg config.Config, dbPair DBPair, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		cfg:         cfg,
		logger:      logger,
		setsRepo:    NewMusicSetRepository(dbPair),
		itemsRepo:   NewSetItemRepository(dbPair),
		historyRepo: NewPlayHistoryRepository(dbPair),
	}
}

// ==========================================================================
// Set CRUD
// ==========================================================================

// CreateSet creates a new music set.
func (s *Service) CreateSet(input CreateSetInput) (*MusicSet, error) {
	set, err := s.setsRepo.Create(input)
	if err != nil {
		s.logger.Printf("Failed to create music set: %v", err)
		return nil, err
	}

	s.logger.Printf("Created music set: %s (%s)", set.Name, set.SetID)
	return set, nil
}

// GetSet retrieves a music set by ID.
func (s *Service) GetSet(setID string) (*MusicSet, error) {
	set, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	// Populate item count
	count, err := s.itemsRepo.Count(setID)
	if err != nil {
		return nil, err
	}
	set.ItemCount = count

	return set, nil
}

// ListSets retrieves music sets with pagination.
func (s *Service) ListSets(limit, offset int) ([]MusicSet, int, error) {
	sets, total, err := s.setsRepo.List(limit, offset)
	if err != nil {
		return nil, 0, err
	}

	// Populate item counts for each set
	for i := range sets {
		count, err := s.itemsRepo.Count(sets[i].SetID)
		if err != nil {
			return nil, 0, err
		}
		sets[i].ItemCount = count
	}

	return sets, total, nil
}

// UpdateSet updates a music set.
func (s *Service) UpdateSet(setID string, input UpdateSetInput) (*MusicSet, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	set, err := s.setsRepo.Update(setID, input)
	if err != nil {
		s.logger.Printf("Failed to update music set %s: %v", setID, err)
		return nil, err
	}

	// Populate item count
	count, err := s.itemsRepo.Count(setID)
	if err != nil {
		return nil, err
	}
	set.ItemCount = count

	s.logger.Printf("Updated music set: %s", setID)
	return set, nil
}

// DeleteSet soft-deletes a music set.
func (s *Service) DeleteSet(setID string) error {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return err
	}
	if existing == nil {
		return &SetNotFoundError{SetID: setID}
	}

	if err := s.setsRepo.Delete(setID); err != nil {
		s.logger.Printf("Failed to delete music set %s: %v", setID, err)
		return err
	}

	s.logger.Printf("Deleted music set: %s", setID)
	return nil
}

// GetSetIncludingDeleted retrieves a music set including soft-deleted ones (for restore).
func (s *Service) GetSetIncludingDeleted(setID string) (*MusicSet, bool, error) {
	return s.setsRepo.GetByIDIncludingDeleted(setID)
}

// RestoreSet restores a soft-deleted music set.
func (s *Service) RestoreSet(setID string) (*MusicSet, error) {
	set, err := s.setsRepo.Restore(setID)
	if err != nil {
		s.logger.Printf("Failed to restore music set %s: %v", setID, err)
		return nil, err
	}

	s.logger.Printf("Restored music set: %s", setID)
	return set, nil
}

// ==========================================================================
// Item Management
// ==========================================================================

// AddItem adds an item to a music set.
func (s *Service) AddItem(setID string, input AddItemInput) (*SetItem, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	item, err := s.itemsRepo.Add(setID, input)
	if err != nil {
		s.logger.Printf("Failed to add item to set %s: %v", setID, err)
		return nil, err
	}

	s.logger.Printf("Added item %s to set %s at position %d", input.SonosFavoriteID, setID, item.Position)
	return item, nil
}

// RemoveItem removes an item from a music set.
func (s *Service) RemoveItem(setID, sonosFavoriteID string) error {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return err
	}
	if existing == nil {
		return &SetNotFoundError{SetID: setID}
	}

	// Verify item exists
	item, err := s.itemsRepo.GetItem(setID, sonosFavoriteID)
	if err != nil {
		return err
	}
	if item == nil {
		return &ItemNotFoundError{SetID: setID, SonosFavoriteID: sonosFavoriteID}
	}

	if err := s.itemsRepo.Remove(setID, sonosFavoriteID); err != nil {
		s.logger.Printf("Failed to remove item %s from set %s: %v", sonosFavoriteID, setID, err)
		return err
	}

	s.logger.Printf("Removed item %s from set %s", sonosFavoriteID, setID)
	return nil
}

// GetItems retrieves all items in a music set.
func (s *Service) GetItems(setID string) ([]SetItem, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	return s.itemsRepo.GetItems(setID)
}

// RemoveItemByPosition removes an item from a music set by its position.
func (s *Service) RemoveItemByPosition(setID string, position int) error {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return err
	}
	if existing == nil {
		return &SetNotFoundError{SetID: setID}
	}

	// Verify item exists at position
	item, err := s.itemsRepo.GetByPosition(setID, position)
	if err != nil {
		return err
	}
	if item == nil {
		return &PositionNotFoundError{SetID: setID, Position: position}
	}

	if err := s.itemsRepo.RemoveByPosition(setID, position); err != nil {
		s.logger.Printf("Failed to remove item at position %d from set %s: %v", position, setID, err)
		return err
	}

	s.logger.Printf("Removed item at position %d from set %s", position, setID)
	return nil
}

// ListItems retrieves items in a music set with pagination.
// Uses database-level pagination for efficiency.
func (s *Service) ListItems(setID string, limit, offset int) ([]SetItem, int, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, 0, err
	}
	if existing == nil {
		return nil, 0, &SetNotFoundError{SetID: setID}
	}

	// Use database-level pagination for efficiency
	return s.itemsRepo.GetItemsPaginated(setID, limit, offset)
}

// ReorderItems reorders items in a music set.
func (s *Service) ReorderItems(setID string, input ReorderItemsInput) error {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return err
	}
	if existing == nil {
		return &SetNotFoundError{SetID: setID}
	}

	// Verify all IDs exist in the set
	items, err := s.itemsRepo.GetItems(setID)
	if err != nil {
		return err
	}

	// Create a map of existing item IDs
	existingIDs := make(map[string]bool)
	for _, item := range items {
		existingIDs[item.SonosFavoriteID] = true
	}

	// Check that all provided IDs exist
	for _, id := range input.Items {
		if !existingIDs[id] {
			return &ItemNotFoundError{SetID: setID, SonosFavoriteID: id}
		}
	}

	// Check that all existing items are in the ordered list
	providedIDs := make(map[string]bool)
	for _, id := range input.Items {
		providedIDs[id] = true
	}
	for _, item := range items {
		if !providedIDs[item.SonosFavoriteID] {
			return &ItemNotFoundError{SetID: setID, SonosFavoriteID: item.SonosFavoriteID}
		}
	}

	if err := s.itemsRepo.Reorder(setID, input.Items); err != nil {
		s.logger.Printf("Failed to reorder items in set %s: %v", setID, err)
		return err
	}

	s.logger.Printf("Reordered %d items in set %s", len(input.Items), setID)
	return nil
}

// ==========================================================================
// Selection Logic
// ==========================================================================

// SelectItem selects an item from a music set based on its selection policy.
func (s *Service) SelectItem(setID string, input SelectItemInput) (*SelectionResult, error) {
	// Get the set
	set, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	// Get all items
	items, err := s.itemsRepo.GetItems(setID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, &EmptySetError{SetID: setID}
	}

	switch SelectionPolicy(set.SelectionPolicy) {
	case SelectionPolicyShuffle:
		return s.selectShuffle(set, items, input.NoRepeatWindowMinutes)
	case SelectionPolicyRotation:
		fallthrough
	default:
		return s.selectRotation(set, items)
	}
}

// selectRotation selects the next item in rotation order.
func (s *Service) selectRotation(set *MusicSet, items []SetItem) (*SelectionResult, error) {
	// Get item at current_index % item_count
	itemCount := len(items)
	selectedIndex := set.CurrentIndex % itemCount

	// Find the item at this position
	var selectedItem *SetItem
	for i := range items {
		if items[i].Position == selectedIndex {
			selectedItem = &items[i]
			break
		}
	}

	// If no item at exact position, use index into sorted slice
	if selectedItem == nil {
		selectedItem = &items[selectedIndex]
	}

	// Atomically increment the index
	newIndex, err := s.setsRepo.IncrementIndex(set.SetID)
	if err != nil {
		s.logger.Printf("Failed to increment index for set %s: %v", set.SetID, err)
		return nil, err
	}

	s.logger.Printf("Rotation selected item %s from set %s (index %d -> %d)",
		selectedItem.SonosFavoriteID, set.SetID, set.CurrentIndex, newIndex)

	return &SelectionResult{
		Item:        selectedItem,
		NextIndex:   newIndex,
		WasShuffled: false,
	}, nil
}

// selectShuffle randomly selects an item, optionally avoiding recently played items.
func (s *Service) selectShuffle(set *MusicSet, items []SetItem, noRepeatWindowMinutes *int) (*SelectionResult, error) {
	availableItems := items

	// If no-repeat window is specified, filter out recently played items
	if noRepeatWindowMinutes != nil && *noRepeatWindowMinutes > 0 {
		recentlyPlayed, err := s.historyRepo.GetRecentlyPlayedInSet(set.SetID, *noRepeatWindowMinutes)
		if err != nil {
			s.logger.Printf("Failed to get recently played items for set %s: %v", set.SetID, err)
			// Continue with all items if we can't get history
		} else if len(recentlyPlayed) > 0 {
			// Create a set of recently played IDs
			recentlyPlayedSet := make(map[string]bool)
			for _, id := range recentlyPlayed {
				recentlyPlayedSet[id] = true
			}

			// Filter out recently played items
			var filtered []SetItem
			for _, item := range items {
				if !recentlyPlayedSet[item.SonosFavoriteID] {
					filtered = append(filtered, item)
				}
			}

			// Only use filtered list if it's not empty
			if len(filtered) > 0 {
				availableItems = filtered
				s.logger.Printf("Filtered out %d recently played items from set %s, %d available",
					len(items)-len(filtered), set.SetID, len(filtered))
			} else {
				s.logger.Printf("All items in set %s were recently played, using full list", set.SetID)
			}
		}
	}

	// Randomly select from available items
	rand.Seed(time.Now().UnixNano())
	selectedIndex := rand.Intn(len(availableItems))
	selectedItem := &availableItems[selectedIndex]

	s.logger.Printf("Shuffle selected item %s from set %s (from %d available)",
		selectedItem.SonosFavoriteID, set.SetID, len(availableItems))

	return &SelectionResult{
		Item:        selectedItem,
		NextIndex:   set.CurrentIndex, // Index doesn't change for shuffle
		WasShuffled: true,
	}, nil
}

// ==========================================================================
// Play History
// ==========================================================================

// RecordPlay records that a favorite was played.
func (s *Service) RecordPlay(sonosFavoriteID string, setID, routineID *string) error {
	if err := s.historyRepo.Record(sonosFavoriteID, setID, routineID); err != nil {
		s.logger.Printf("Failed to record play for %s: %v", sonosFavoriteID, err)
		return err
	}

	s.logger.Printf("Recorded play for %s (set: %v, routine: %v)", sonosFavoriteID, setID, routineID)
	return nil
}

// GetPlayHistory retrieves play history for a music set.
func (s *Service) GetPlayHistory(setID string, limit int) ([]PlayHistory, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &SetNotFoundError{SetID: setID}
	}

	return s.historyRepo.GetSetHistory(setID, limit)
}

// GetHistory retrieves play history for a music set with pagination.
func (s *Service) GetHistory(setID string, limit, offset int) ([]PlayHistory, int, error) {
	// Verify set exists
	existing, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, 0, err
	}
	if existing == nil {
		return nil, 0, &SetNotFoundError{SetID: setID}
	}

	// Get history with a larger limit to calculate total
	// Note: For proper pagination, the repository should support counting
	// For now, we'll use a workaround
	allHistory, err := s.historyRepo.GetSetHistory(setID, limit+offset+1000)
	if err != nil {
		return nil, 0, err
	}

	total := len(allHistory)

	// Apply pagination
	if offset >= total {
		return []PlayHistory{}, total, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	return allHistory[offset:end], total, nil
}

// ==========================================================================
// Enrichment Methods
// ==========================================================================

// MusicSetEnrichment contains display information for a music set, used for routine responses.
type MusicSetEnrichment struct {
	Name           string  `json:"name"`
	ArtworkURL     *string `json:"artwork_url"`
	ServiceLogoURL *string `json:"service_logo_url"`
	ServiceName    *string `json:"service_name"`
}

// GetSetEnrichment retrieves display information for a music set.
// Returns nil if the set is not found (does not return error for not found).
func (s *Service) GetSetEnrichment(setID string) (*MusicSetEnrichment, error) {
	// Get the set
	set, err := s.setsRepo.GetByID(setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, nil // Not found, return nil without error
	}

	enrichment := &MusicSetEnrichment{
		Name: set.Name,
	}

	// Get the first item to extract artwork and service info
	items, err := s.itemsRepo.GetItems(setID)
	if err != nil {
		s.logger.Printf("Failed to get items for set enrichment %s: %v", setID, err)
		return enrichment, nil // Return partial enrichment without items info
	}

	if len(items) > 0 {
		firstItem := items[0]

		// Get service info from first item
		enrichment.ServiceLogoURL = firstItem.ServiceLogoURL
		enrichment.ServiceName = firstItem.ServiceName

		// Get artwork_url directly from item (preferred - stored at item level)
		if firstItem.ArtworkURL != nil && *firstItem.ArtworkURL != "" {
			enrichment.ArtworkURL = firstItem.ArtworkURL
		} else if firstItem.ContentJSON != nil && *firstItem.ContentJSON != "" {
			// Fallback: try to get artwork_url from content_json for legacy items
			var metadata ContentMetadata
			if err := json.Unmarshal([]byte(*firstItem.ContentJSON), &metadata); err == nil {
				if metadata.ArtworkURL != "" {
					enrichment.ArtworkURL = &metadata.ArtworkURL
				}
			}
		}
	}

	return enrichment, nil
}
