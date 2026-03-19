/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/issues/27782
 * Buggy version: Before 1f8e41bcaa1fb8f8a5caf44a7e3cf08eed1c85f5
 * fix commit-id: 1f8e41bcaa1fb8f8a5caf44a7e3cf08eed1c85f5
 * Flaky: Timing-dependent on concurrent read/write events
 * Description: Deadlock in storage driver event handling. A condition variable
 * is used to coordinate event notifications, but certain code paths update state
 * without broadcasting the condition. Waiters on that condition will block
 * indefinitely if their signaling event never occurs or broadcast is missing.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type StorageEvent struct {
	EventType string
	DeviceID  string
}

type StorageDriver struct {
	mu        sync.Mutex
	eventWait *sync.Cond
	// @guarded_by(mu)
	pendingEvents []StorageEvent
	// @guarded_by(mu)
	lastEventType string
}

// BUG: This function marks a write event as completed and should notify
// waiters, but might not broadcast in all cases. If a waiter is blocked
// waiting for write event completion, they'll wait forever.
//
// @acquires(sd.mu)
func (sd *StorageDriver) completeWriteEvent(deviceID string) {
	sd.mu.Lock()

	// Process the write event
	sd.lastEventType = "write_complete"

	// BUG: This only broadcasts if there are pending events
	// But if there are no pending events, waiters waiting for write completion
	// will continue waiting despite the event being done
	if len(sd.pendingEvents) > 0 {
		sd.eventWait.Broadcast() // Only broadcasts if events are pending
	}
	// BUG: Missing else case or unconditional broadcast

	sd.mu.Unlock()
}

// This function waits for write events to be processed.
// If completeWriteEvent() doesn't broadcast when it should,
// this will block forever.
//
// @acquires(sd.mu)
func (sd *StorageDriver) waitForWriteCompletion(timeout time.Duration) error {
	sd.mu.Lock()

	// Wait for write event to complete
	for sd.lastEventType != "write_complete" {
		// Create a channel for timeout
		done := make(chan struct{})
		go func() {
			sd.eventWait.Wait()
			close(done)
		}()

		sd.mu.Unlock()

		select {
		case <-done:
			// Event was signaled, re-acquire lock
			sd.mu.Lock()
		case <-time.After(timeout):
			// Timeout waiting for event
			return ErrEventTimeout
		}
	}

	sd.mu.Unlock()
	return nil
}

// Similar to completeWriteEvent but for read events
// BUG: Same pattern - might not broadcast if pending events list is empty
//
// @acquires(sd.mu)
func (sd *StorageDriver) completeReadEvent(deviceID string) {
	sd.mu.Lock()

	sd.lastEventType = "read_complete"

	// BUG: Only broadcasts if pending events - missing broadcast for empty list
	if len(sd.pendingEvents) > 0 {
		sd.eventWait.Broadcast()
	}

	sd.mu.Unlock()
}

type EventError struct{}

func (e *EventError) Error() string {
	return "event timeout"
}

var ErrEventTimeout = &EventError{}

func TestMoby27782(t *testing.T) {
	sd := &StorageDriver{
		pendingEvents: make([]StorageEvent, 0),
		eventWait:     nil,
	}

	// Initialize under lock
	sd.mu.Lock()
	sd.eventWait = sync.NewCond(&sd.mu)
	sd.lastEventType = "none"
	sd.mu.Unlock()

	// Scenario 1: No pending events, but someone is waiting for completion
	// Thread A: Start waiting for write completion
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- sd.waitForWriteCompletion(100 * time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)

	// Thread B: Process a write event
	// The pending events list is empty, so completeWriteEvent won't broadcast
	go func() {
		sd.completeWriteEvent("device1")
	}()

	// Check if waiter unblocks
	// BUG: Deadlock - the waiter is waiting for write_complete but
	// completeWriteEvent didn't broadcast because pendingEvents was empty
	select {
	case err := <-waitDone:
		if err == ErrEventTimeout {
			// Expected: timeout because event wasn't signaled
			t.Logf("waiter timed out as expected due to missing broadcast")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: waitForWriteCompletion never returned")
	}

	// Scenario 2: Multiple operations waiting for completion
	sd.mu.Lock()
	sd.lastEventType = "none"
	sd.mu.Unlock()

	// Thread C: Multiple waiters for read completion
	readWait1 := make(chan error, 1)
	readWait2 := make(chan error, 1)

	go func() {
		sd.mu.Lock()
		for sd.lastEventType != "read_complete" {
			done := make(chan struct{})
			go func() {
				sd.eventWait.Wait()
				close(done)
			}()
			sd.mu.Unlock()

			select {
			case <-done:
				sd.mu.Lock()
			case <-time.After(100 * time.Millisecond):
				readWait1 <- ErrEventTimeout
				return
			}
		}
		sd.mu.Unlock()
		readWait1 <- nil
	}()

	go func() {
		sd.mu.Lock()
		for sd.lastEventType != "read_complete" {
			done := make(chan struct{})
			go func() {
				sd.eventWait.Wait()
				close(done)
			}()
			sd.mu.Unlock()

			select {
			case <-done:
				sd.mu.Lock()
			case <-time.After(100 * time.Millisecond):
				readWait2 <- ErrEventTimeout
				return
			}
		}
		sd.mu.Unlock()
		readWait2 <- nil
	}()

	time.Sleep(20 * time.Millisecond)

	// Thread D: Complete read event with no pending events
	// BUG: Won't broadcast, so both waiters will timeout
	sd.completeReadEvent("device2")

	select {
	case <-readWait1:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: read waiter 1 never returned")
	}

	select {
	case <-readWait2:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: read waiter 2 never returned")
	}
}
