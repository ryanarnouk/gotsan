/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/17176
 * Buggy version: Before 7777c1be9bd5604014a0ac3f16e85960c4c0779a
 * fix commit-id: 7777c1be9bd5604014a0ac3f16e85960c4c0779a
 * Flaky: Yes (depends on whether deleted devices exist)
 * Description: Deadlock in devmapper cleanup. The cleanupDeleted() function
 * acquires devices.Lock() but returns early without releasing the lock when
 * there are no deleted devices. This causes the next operation trying to
 * acquire the lock to hang indefinitely.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type DeviceManager struct {
	mu sync.Mutex
	// @guarded_by(mu)
	deletedDevices []string
	// @guarded_by(mu)
	activeDevices map[string]bool
}

// BUG: This function acquires the lock but has an early return
// path that doesn't release the lock. If there are no deleted devices,
// the function returns while still holding the lock.
// @acquires(dm.mu)
// @returns(dm.mu) - BUG: early return path returns with lock held
func (dm *DeviceManager) cleanupDeleted() error {
	dm.mu.Lock()

	// BUG: Early return without releasing the lock!
	// If no deleted devices exist, the function returns while still
	// holding the mutex, causing any subsequent lock attempt to deadlock.
	if len(dm.deletedDevices) == 0 {
		return nil // BUG: Lock not released! Caller will deadlock
	}

	// Clean up deleted devices only reached if deletedDevices not empty
	for _, devID := range dm.deletedDevices {
		delete(dm.activeDevices, devID)
	}
	dm.deletedDevices = make([]string, 0)

	dm.mu.Unlock()
	return nil
}

// Simulates trying to access devices - will deadlock if lock is held
//
// @acquires(dm.mu)
func (dm *DeviceManager) getDeviceCount() int {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	return len(dm.activeDevices)
}

func TestMoby17176(t *testing.T) {
	dm := &DeviceManager{
		deletedDevices: []string{},
		activeDevices:  map[string]bool{},
	}

	// Initialize the locked state properly
	dm.activeDevices["dev1"] = true
	dm.activeDevices["dev2"] = true

	// Thread A: Call cleanup when there are NO deleted devices
	// This will deadlock in the buggy version due to early return without unlock
	go func() {
		err := dm.cleanupDeleted()
		if err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

	time.Sleep(10 * time.Millisecond)

	// Thread B: Try to get device count
	// This will deadlock because cleanupDeleted() is holding the lock
	countDone := make(chan int, 1)
	go func() {
		countDone <- dm.getDeviceCount()
	}()

	select {
	case count := <-countDone:
		if count != 2 {
			t.Fatalf("expected count 2, got %d", count)
		}
	case <-time.After(500 * time.Millisecond):
		// BUG: Deadlock - getDeviceCount is blocked waiting for lock
		// that cleanupDeleted is holding due to early return without unlock
		t.Fatal("deadlock: getDeviceCount timed out waiting for lock")
	}
}
