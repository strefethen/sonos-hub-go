package scene

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorLock_WithLock_Success(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	executed := false
	err := lock.WithLock("device-1", time.Second, func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	require.True(t, executed)
}

func TestCoordinatorLock_WithLock_FunctionError(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	expectedErr := errors.New("test error")
	err := lock.WithLock("device-1", time.Second, func() error {
		return expectedErr
	})

	require.Equal(t, expectedErr, err)
}

func TestCoordinatorLock_WithLock_ReleasesOnError(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	_ = lock.WithLock("device-1", time.Second, func() error {
		return errors.New("test error")
	})

	// Lock should be released, so we can acquire again
	executed := false
	err := lock.WithLock("device-1", time.Second, func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	require.True(t, executed)
}

func TestCoordinatorLock_WithLock_ReleasesOnPanic(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	func() {
		defer func() { recover() }()
		_ = lock.WithLock("device-1", time.Second, func() error {
			panic("test panic")
		})
	}()

	// Lock should be released, so we can acquire again
	executed := false
	err := lock.WithLock("device-1", time.Second, func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	require.True(t, executed)
}

func TestCoordinatorLock_WithLock_Timeout(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	// Acquire the lock
	acquired := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = lock.WithLock("device-1", 5*time.Second, func() error {
			close(acquired)
			<-done // Wait for signal to release
			return nil
		})
	}()

	<-acquired // Wait for lock to be acquired

	// Try to acquire with short timeout - should fail
	start := time.Now()
	err := lock.WithLock("device-1", 100*time.Millisecond, func() error {
		return nil
	})

	elapsed := time.Since(start)
	require.ErrorIs(t, err, ErrLockTimeout)
	require.Less(t, elapsed, 200*time.Millisecond)

	close(done) // Release the original lock
}

func TestCoordinatorLock_WithLock_DifferentDevices(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	var wg sync.WaitGroup
	var executed1, executed2 atomic.Bool

	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = lock.WithLock("device-1", time.Second, func() error {
			time.Sleep(50 * time.Millisecond)
			executed1.Store(true)
			return nil
		})
	}()

	go func() {
		defer wg.Done()
		_ = lock.WithLock("device-2", time.Second, func() error {
			time.Sleep(50 * time.Millisecond)
			executed2.Store(true)
			return nil
		})
	}()

	wg.Wait()

	require.True(t, executed1.Load())
	require.True(t, executed2.Load())
}

func TestCoordinatorLock_WithLock_Sequential(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	var sequence []int
	var mu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(3)

	for i := 1; i <= 3; i++ {
		go func(n int) {
			defer wg.Done()
			_ = lock.WithLock("device-1", 2*time.Second, func() error {
				mu.Lock()
				sequence = append(sequence, n)
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				return nil
			})
		}(i)
		time.Sleep(5 * time.Millisecond) // Stagger starts slightly
	}

	wg.Wait()

	// All should have executed
	require.Len(t, sequence, 3)
}

func TestCoordinatorLock_TryLock(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	// First try should succeed
	acquired := lock.TryLock("device-1")
	require.True(t, acquired)
	require.True(t, lock.IsLocked("device-1"))

	// Second try should fail
	acquired2 := lock.TryLock("device-1")
	require.False(t, acquired2)

	// Release and try again
	lock.Unlock("device-1")
	require.False(t, lock.IsLocked("device-1"))

	acquired3 := lock.TryLock("device-1")
	require.True(t, acquired3)
	lock.Unlock("device-1")
}

func TestCoordinatorLock_IsLocked(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	// Initially not locked
	require.False(t, lock.IsLocked("device-1"))

	// Lock and check
	acquired := lock.TryLock("device-1")
	require.True(t, acquired)
	require.True(t, lock.IsLocked("device-1"))

	// Different device not locked
	require.False(t, lock.IsLocked("device-2"))

	lock.Unlock("device-1")
	require.False(t, lock.IsLocked("device-1"))
}

func TestCoordinatorLock_LockInfo(t *testing.T) {
	lock := NewCoordinatorLock(nil)

	// Not locked
	locked, lockTime, duration := lock.LockInfo("device-1")
	require.False(t, locked)
	require.True(t, lockTime.IsZero())
	require.Zero(t, duration)

	// Lock and check
	acquired := lock.TryLock("device-1")
	require.True(t, acquired)

	time.Sleep(10 * time.Millisecond)

	locked, lockTime, duration = lock.LockInfo("device-1")
	require.True(t, locked)
	require.False(t, lockTime.IsZero())
	require.GreaterOrEqual(t, duration, 10*time.Millisecond)

	lock.Unlock("device-1")
}
