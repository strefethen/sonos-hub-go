package scene

import (
	"errors"
	"log"
	"sync"
	"time"
)

// ErrLockTimeout is returned when lock acquisition times out.
var ErrLockTimeout = errors.New("coordinator lock timeout")

// ErrLockHeld is returned when trying to acquire a lock that is already held.
var ErrLockHeld = errors.New("coordinator lock already held")

// DefaultLockTimeout is the default timeout for lock acquisition.
const DefaultLockTimeout = 60 * time.Second

// deviceMutex represents a mutex for a single device.
type deviceMutex struct {
	mu       sync.Mutex
	locked   bool
	lockTime time.Time
	owner    string // For debugging purposes
}

// CoordinatorLock provides per-device locking for scene execution.
// It prevents concurrent scene executions on the same coordinator device.
type CoordinatorLock struct {
	mu      sync.Mutex
	mutexes map[string]*deviceMutex
	logger  *log.Logger
}

// NewCoordinatorLock creates a new CoordinatorLock.
func NewCoordinatorLock(logger *log.Logger) *CoordinatorLock {
	if logger == nil {
		logger = log.Default()
	}
	return &CoordinatorLock{
		mutexes: make(map[string]*deviceMutex),
		logger:  logger,
	}
}

// WithLock executes a function while holding the lock for a device.
// The lock is automatically released when the function returns.
// If the lock cannot be acquired within the timeout, ErrLockTimeout is returned.
func (cl *CoordinatorLock) WithLock(deviceID string, timeout time.Duration, fn func() error) error {
	if timeout == 0 {
		timeout = DefaultLockTimeout
	}

	dm := cl.getOrCreateDeviceMutex(deviceID)

	// Try to acquire the lock with timeout
	acquired := make(chan struct{})
	go func() {
		dm.mu.Lock()
		close(acquired)
	}()

	select {
	case <-acquired:
		// Lock acquired
	case <-time.After(timeout):
		// Timeout - the goroutine will eventually acquire and release the lock
		// but we return timeout error to the caller
		go func() {
			<-acquired
			dm.mu.Unlock()
		}()
		return ErrLockTimeout
	}

	dm.locked = true
	dm.lockTime = time.Now()
	dm.owner = deviceID

	cl.logger.Printf("Acquired coordinator lock for device %s", deviceID)

	// Set up auto-release timer as safety net
	autoRelease := time.AfterFunc(timeout, func() {
		cl.logger.Printf("Auto-releasing coordinator lock for device %s after timeout", deviceID)
		dm.mu.Unlock()
		dm.locked = false
	})

	defer func() {
		autoRelease.Stop()
		dm.locked = false
		dm.mu.Unlock()
		cl.logger.Printf("Released coordinator lock for device %s", deviceID)
	}()

	return fn()
}

// TryLock attempts to acquire the lock without blocking.
// Returns true if the lock was acquired, false if it's already held.
func (cl *CoordinatorLock) TryLock(deviceID string) bool {
	dm := cl.getOrCreateDeviceMutex(deviceID)

	if dm.mu.TryLock() {
		dm.locked = true
		dm.lockTime = time.Now()
		dm.owner = deviceID
		cl.logger.Printf("Acquired coordinator lock for device %s (try)", deviceID)
		return true
	}
	return false
}

// Unlock releases a lock that was acquired via TryLock.
func (cl *CoordinatorLock) Unlock(deviceID string) {
	dm := cl.getOrCreateDeviceMutex(deviceID)
	if dm.locked {
		dm.locked = false
		dm.mu.Unlock()
		cl.logger.Printf("Released coordinator lock for device %s (manual)", deviceID)
	}
}

// IsLocked returns whether a device is currently locked.
// This is a non-blocking check.
func (cl *CoordinatorLock) IsLocked(deviceID string) bool {
	cl.mu.Lock()
	dm, exists := cl.mutexes[deviceID]
	cl.mu.Unlock()

	if !exists {
		return false
	}
	return dm.locked
}

// LockInfo returns information about a lock.
func (cl *CoordinatorLock) LockInfo(deviceID string) (locked bool, lockTime time.Time, duration time.Duration) {
	cl.mu.Lock()
	dm, exists := cl.mutexes[deviceID]
	cl.mu.Unlock()

	if !exists || !dm.locked {
		return false, time.Time{}, 0
	}

	return true, dm.lockTime, time.Since(dm.lockTime)
}

func (cl *CoordinatorLock) getOrCreateDeviceMutex(deviceID string) *deviceMutex {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	dm, exists := cl.mutexes[deviceID]
	if !exists {
		dm = &deviceMutex{}
		cl.mutexes[deviceID] = dm
	}
	return dm
}
