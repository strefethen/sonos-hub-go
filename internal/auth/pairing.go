package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type pairingEntry struct {
	createdAt time.Time
	requestID string
}

// PairingStore tracks pending pairing codes.
type PairingStore struct {
	mu      sync.Mutex
	entries map[string]pairingEntry
	ttl     time.Duration
}

func NewPairingStore(ttl time.Duration) *PairingStore {
	return &PairingStore{
		entries: make(map[string]pairingEntry),
		ttl:     ttl,
	}
}

// StartCleanup removes expired codes periodically until the context is canceled.
func (store *PairingStore) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				store.CleanupExpired()
			case <-ctx.Done():
				store.Clear()
				return
			}
		}
	}()
}

// CleanupExpired removes expired pairing codes.
func (store *PairingStore) CleanupExpired() {
	store.mu.Lock()
	defer store.mu.Unlock()

	now := time.Now()
	for code, entry := range store.entries {
		if now.Sub(entry.createdAt) > store.ttl {
			delete(store.entries, code)
		}
	}
}

// Clear wipes all entries from the store.
func (store *PairingStore) Clear() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.entries = make(map[string]pairingEntry)
}

// Create generates and stores a new pairing code.
func (store *PairingStore) Create(requestID string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	for attempts := 0; attempts < 10; attempts++ {
		code, err := randomPairingCode()
		if err != nil {
			return "", err
		}
		if _, exists := store.entries[code]; exists {
			continue
		}
		store.entries[code] = pairingEntry{
			createdAt: time.Now(),
			requestID: requestID,
		}
		return code, nil
	}

	return "", fmt.Errorf("unable to generate unique pairing code")
}

// Lookup checks a pairing code and reports if it exists and is expired.
func (store *PairingStore) Lookup(code string) (pairingEntry, bool, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()

	entry, ok := store.entries[code]
	if !ok {
		return pairingEntry{}, false, false
	}
	expired := time.Since(entry.createdAt) > store.ttl
	return entry, true, expired
}

// Consume removes a pairing code from the store.
func (store *PairingStore) Consume(code string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.entries, code)
}

func randomPairingCode() (string, error) {
	max := big.NewInt(900000)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	code := 100000 + n.Int64()
	return fmt.Sprintf("%06d", code), nil
}
