/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/30408
 * Buggy version: Before 113e9f07f49e9089df41585aeb355697b4c96120
 * fix commit-id: 113e9f07f49e9089df41585aeb355697b4c96120
 * Flaky: Yes (race condition on activateErr check)
 * Description: Deadlock in plugin activation. The waitActive function waits
 * for the plugin to be activated, but if an activation error occurs, the
 * condition variable is never broadcast again. The wait loop only checks if
 * activated() is true, not if there was an error, so it waits forever even
 * though activation failed.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type Plugin struct {
	mu           sync.Mutex
	activateWait *sync.Cond
	// @guarded_by(mu)
	activated bool
	// @guarded_by(mu)
	activateErr error
}

// BUG: This function waits for the plugin to be activated, but doesn't
// check for activation errors. If an error occurs instead of activation,
// this function will wait forever because the condition is never broadcast
// again and activated never becomes true.
//
// @acquires(p.mu)
func (p *Plugin) waitActive() error {
	p.mu.Lock()

	// BUG: This loop only checks if activated is true
	// If activateErr is set instead, the condition will be broadcast once
	// but the loop will never exit because activated is still false
	for !p.activated {
		// Never woken again if error occurred -> DEADLOCK
		p.activateWait.Wait()
	}

	// Safely read error while lock is held
	err := p.activateErr
	p.mu.Unlock()
	return err
}

// Simulates the plugin activation process. On error, it sets activateErr
// and broadcasts, but waitActive doesn't check the error state.
//
// @acquires(p.mu)
func (p *Plugin) activate(shouldFail bool) {
	p.mu.Lock()

	if shouldFail {
		// BUG: Set error and broadcast, but waitActive only checks activated
		p.activateErr = &ActivationError{}
		p.activateWait.Broadcast()
		// Thread waiting in waitActive wakes up, checks activated (still false),
		// and goes back to wait forever -> DEADLOCK
		p.mu.Unlock()
		return
	}

	p.activated = true
	p.activateWait.Broadcast()
	p.mu.Unlock()
}

type ActivationError struct{}

func (e *ActivationError) Error() string {
	return "plugin activation failed"
}

func TestMoby30408(t *testing.T) {
	p := &Plugin{
		activateWait: sync.NewCond(&sync.Mutex{}),
	}

	// Initialize guarded fields under lock
	p.mu.Lock()
	p.activated = false
	p.activateErr = nil
	// Update activateWait to use our mu
	p.activateWait = sync.NewCond(&p.mu)
	p.mu.Unlock()

	// Thread A: Try to wait for the plugin to activate
	done := make(chan error, 1)
	go func() {
		done <- p.waitActive()
	}()

	// Give waitActive time to acquire the lock and start waiting
	time.Sleep(10 * time.Millisecond)

	// Thread B: Attempt activation with an error
	// This will broadcast the condition, but waitActive only checks
	// if the plugin is activated, not if there was an error
	go func() {
		p.activate(true) // shouldFail = true
	}()

	// This will timeout because waitActive is waiting forever
	// It woke up from the broadcast, checked activated (false), and went back to wait
	select {
	case <-done:
		// Should not reach here - deadlock expected
		t.Fatal("waitActive returned when it should have deadlocked")
	case <-time.After(500 * time.Millisecond):
		// BUG: Deadlock - waitActive never returns
		// GOTSAN should detect this condition variable issue
	}
}
