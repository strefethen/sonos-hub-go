package audit

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/strefethen/sonos-hub-go/internal/config"
)

// Default configuration values
const (
	DefaultRetentionDays     = 90
	DefaultPruneInterval     = 24 * time.Hour
	DefaultQueryLimit        = 100
	MaxQueryLimit            = 1000
	MaxConsecutiveFailures   = 3
)

// Service provides audit log management functionality.
type Service struct {
	cfg                 config.Config
	logger              *log.Logger
	repo                *Repository
	retentionDays       int
	pruneInterval       time.Duration
	defaultQueryLimit   int
	maxQueryLimit       int
	stopCh              chan struct{}
	wg                  sync.WaitGroup
	healthy             bool
	healthMu            sync.RWMutex
	consecutiveFailures int
}

// NewService creates a new audit service.
// Accepts a DBPair for optimal SQLite concurrency with separate reader/writer pools.
func NewService(cfg config.Config, dbPair DBPair, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}

	repo := NewRepository(dbPair)

	return &Service{
		cfg:               cfg,
		logger:            logger,
		repo:              repo,
		retentionDays:     DefaultRetentionDays,
		pruneInterval:     DefaultPruneInterval,
		defaultQueryLimit: DefaultQueryLimit,
		maxQueryLimit:     MaxQueryLimit,
		stopCh:            make(chan struct{}),
		healthy:           true,
	}
}

// RecordEvent writes a new audit event.
// Logs at debug level, tracks health.
func (s *Service) RecordEvent(input WriteEventInput) (*AuditEvent, error) {
	// Default level to INFO if not provided
	if input.Level == nil {
		level := EventLevelInfo
		input.Level = &level
	}

	s.logger.Printf("[DEBUG] Recording audit event: type=%s level=%s message=%s",
		input.Type, *input.Level, input.Message)

	event, err := s.repo.InsertEvent(input)
	if err != nil {
		s.recordFailure()
		return nil, fmt.Errorf("failed to record audit event: %w", err)
	}

	s.recordSuccess()
	return event, nil
}

// QueryEvents retrieves events with filters and pagination.
// Clamps limit to maxQueryLimit.
// Returns: events, total count, hasMore flag, error.
func (s *Service) QueryEvents(filters EventQueryFilters) ([]AuditEvent, int, bool, error) {
	// Apply default limit if not specified
	if filters.Limit == 0 {
		filters.Limit = s.defaultQueryLimit
	}

	// Clamp limit to maxQueryLimit
	if filters.Limit > s.maxQueryLimit {
		filters.Limit = s.maxQueryLimit
	}

	events, total, err := s.repo.QueryEvents(filters)
	if err != nil {
		s.recordFailure()
		return nil, 0, false, fmt.Errorf("failed to query audit events: %w", err)
	}

	s.recordSuccess()

	// Calculate hasMore: are there more events beyond this page?
	hasMore := filters.Offset+len(events) < total

	return events, total, hasMore, nil
}

// GetEvent retrieves a single event by ID.
func (s *Service) GetEvent(eventID string) (*AuditEvent, error) {
	event, err := s.repo.GetEvent(eventID)
	if err != nil {
		s.recordFailure()
		return nil, fmt.Errorf("failed to get audit event: %w", err)
	}

	if event == nil {
		return nil, &EventNotFoundError{EventID: eventID}
	}

	s.recordSuccess()
	return event, nil
}

// StartPruneJob starts the background prune job.
// Runs immediately on start, then at pruneInterval.
func (s *Service) StartPruneJob() {
	s.logger.Printf("Starting audit prune job (interval: %v, retention: %d days)",
		s.pruneInterval, s.retentionDays)

	s.wg.Add(1)
	go s.runPruneLoop()
}

// StopPruneJob stops the background prune job.
func (s *Service) StopPruneJob() {
	s.logger.Printf("Stopping audit prune job")
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Printf("Audit prune job stopped")
}

// runPruneLoop is the background goroutine that periodically prunes old events.
func (s *Service) runPruneLoop() {
	defer s.wg.Done()

	// Run immediately on start
	if count, err := s.Prune(); err != nil {
		s.logger.Printf("Error pruning audit events on start: %v", err)
	} else if count > 0 {
		s.logger.Printf("Pruned %d audit events on startup", count)
	}

	ticker := time.NewTicker(s.pruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if count, err := s.Prune(); err != nil {
				s.logger.Printf("Error pruning audit events: %v", err)
			} else if count > 0 {
				s.logger.Printf("Pruned %d audit events", count)
			}
		}
	}
}

// Prune manually triggers pruning, returns count deleted.
func (s *Service) Prune() (int64, error) {
	count, err := s.repo.PruneOldEvents(s.retentionDays)
	if err != nil {
		s.recordFailure()
		return 0, fmt.Errorf("failed to prune audit events: %w", err)
	}

	s.recordSuccess()
	return count, nil
}

// IsHealthy returns current health status.
func (s *Service) IsHealthy() bool {
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	return s.healthy
}

// recordSuccess resets the consecutive failure count and marks service as healthy.
func (s *Service) recordSuccess() {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	s.consecutiveFailures = 0
	s.healthy = true
}

// recordFailure increments the consecutive failure count and marks unhealthy after threshold.
func (s *Service) recordFailure() {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	s.consecutiveFailures++
	if s.consecutiveFailures >= MaxConsecutiveFailures {
		s.healthy = false
	}
}

// EventNotFoundError is returned when an audit event is not found.
type EventNotFoundError struct {
	EventID string
}

func (e *EventNotFoundError) Error() string {
	return fmt.Sprintf("audit event not found: %s", e.EventID)
}
