/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/36114
 * Buggy version: Before 03a1df95369ddead968e48697038904c84578d00
 * fix commit-id: 03a1df95369ddead968e48697038904c84578d00
 * Flaky: Context-dependent
 * Description: Recursive mutex acquisition deadlock in the tear-down error path
 * during HotAddVHDs. When HotAddVhd fails, hotAddVHDsAtStart tries to call
 * hotRemoveVHDsAtStart to cleanup, but hotRemoveVHDsAtStart tries to acquire
 * the same lock that's already held by hotAddVHDsAtStart, causing a deadlock.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type VirtualDisk struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

type ServiceVM struct {
	mu           sync.Mutex
	// @guarded_by(mu)
	attachedVHDs map[string]int
	// @guarded_by(mu)
	vhdCount int
	fail     bool // Simulate config.HotAddVhd failure
}

// BUG: This method acquires the lock and then calls hotRemoveVHDs
// in the error path, which tries to acquire the same lock -> DEADLOCK
//
// @acquires(svm.mu)
func (svm *ServiceVM) hotAddVHDsAtStart(vhds ...VirtualDisk) error {
	svm.mu.Lock()
	defer svm.mu.Unlock()

	for i, vhd := range vhds {
		// Simulate config.HotAddVhd call
		if svm.fail {
			// BUG: Trying to call a method that will acquire mu while we hold it
			// This causes a deadlock because we already hold the lock
			return svm.hotRemoveVHDsAtStart(vhds[:i]...)
		}

		svm.attachedVHDs[vhd.HostPath] = 1
		svm.vhdCount++
	}

	return nil
}

// BUG: This method tries to acquire the lock
// If called from hotAddVHDsAtStart with the lock already held, DEADLOCK
//
// @acquires(svm.mu)
func (svm *ServiceVM) hotRemoveVHDsAtStart(vhds ...VirtualDisk) error {
	svm.mu.Lock() // Tries to acquire but already held by caller -> DEADLOCK
	defer svm.mu.Unlock()

	for _, vhd := range vhds {
		delete(svm.attachedVHDs, vhd.HostPath)
		svm.vhdCount--
	}

	return nil
}

func TestMoby36114(t *testing.T) {
	svm := &ServiceVM{
		fail: true, // Force hotAddVhd to fail
	}

	// Initialize guarded fields under lock
	svm.mu.Lock()
	svm.attachedVHDs = make(map[string]int)
	svm.vhdCount = 0
	svm.mu.Unlock()

	vhds := []VirtualDisk{
		{
			HostPath:      "/path/to/vhd1",
			ContainerPath: "/mnt/vhd1",
			ReadOnly:      false,
		},
		{
			HostPath:      "/path/to/vhd2",
			ContainerPath: "/mnt/vhd2",
			ReadOnly:      false,
		},
	}

	// This call will deadlock: hotAddVHDsAtStart acquires the lock,
	// then on error tries to call hotRemoveVHDsAtStart which tries
	// to acquire the same lock -> DEADLOCK
	go func() {
		_ = svm.hotAddVHDsAtStart(vhds...)
	}()

	// Give the deadlock a chance to occur
	time.Sleep(100 * time.Millisecond)
}
