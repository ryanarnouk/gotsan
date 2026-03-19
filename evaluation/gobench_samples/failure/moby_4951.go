/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/4951
 * Buggy version: Before 2ffef1b7eb618162673c6ffabccb9ca57c7dfce3
 * fix commit-id: 2ffef1b7eb618162673c6ffabccb9ca57c7dfce3
 * Flaky: 3/100 (context-dependent)
 * Description: This is an AB-BA deadlock in devicemapper backend where lock
 * ordering is violated. Thread A acquires global lock, then device lock, releases
 * global lock, sleeps, then tries to re-acquire global lock. Meanwhile, Thread B
 * acquires global lock and blocks trying to acquire the device lock held by A,
 * creating a deadlock cycle.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type DevInfo struct {
	lock sync.Mutex
	// @guarded_by(lock)
	active bool
	// @guarded_by(lock)
	hash string
}

type DeviceSet struct {
	mu sync.Mutex
	// @guarded_by(mu)
	devices map[string]*DevInfo
}

// BUG: Lock ordering violation
// This method acquires mu (global) then info.lock (device)
// which violates the required ordering of device.lock before mu
//
// @acquires(ds.mu)
func (ds *DeviceSet) removeDevice(hash string) error {
	// Thread A: Acquire global lock first (WRONG ORDER)
	ds.mu.Lock()
	info := ds.devices[hash]
	ds.mu.Unlock() // Release but will need it again

	if info == nil {
		return nil
	}

	// Then acquire device lock
	info.lock.Lock()
	defer info.lock.Unlock()

	// Simulate work that requires global lock
	time.Sleep(10 * time.Millisecond)

	// Try to re-acquire global lock while holding device lock
	// This can deadlock with activateDevice
	ds.mu.Lock()
	defer ds.mu.Unlock()

	info.active = false

	return nil
}

// BUG: Lock ordering violation
// Acquires mu (global) before info.lock (device)
// When Thread A holds info.lock and tries to acquire mu,
// and Thread B holds mu and tries to acquire info.lock: DEADLOCK
//
// @acquires(ds.mu)
func (ds *DeviceSet) activateDevice(hash string) error {
	// Thread B: Acquire global lock first
	ds.mu.Lock()
	info := ds.devices[hash] // Access guarded_by(mu)
	if info == nil {
		ds.mu.Unlock()
		return nil
	}

	// Try to acquire device lock while still holding global lock
	// This blocks if Thread A holds device lock
	info.lock.Lock()
	defer info.lock.Unlock()
	defer ds.mu.Unlock()

	info.active = true

	return nil
}

func TestMoby4951(t *testing.T) {
	ds := &DeviceSet{}

	// Initialize the device map under lock
	ds.mu.Lock()
	ds.devices = make(map[string]*DevInfo)

	info := &DevInfo{}
	// Acquire lock on the new device before accessing its guarded fields
	info.lock.Lock()
	info.hash = "device1"
	info.active = false
	info.lock.Unlock()

	ds.devices["device1"] = info
	ds.mu.Unlock()

	// Thread A: acquire global lock, device lock, release global lock, sleep
	// This demonstrates the lock release/re-acquire pattern
	go func() {
		ds.removeDevice("device1")
	}()

	time.Sleep(5 * time.Millisecond)

	// Thread B: try to acquire global lock then device lock
	// If Thread A is sleeping and holds device lock, this deadlocks
	go func() {
		ds.activateDevice("device1")
	}()

	// Give deadlock a chance to occur
	time.Sleep(100 * time.Millisecond)
}
