/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/29733
 * Buggy version: Before 48ed4f0639d2f290603a04ec146beb3f9569280f
 * fix commit-id: 48ed4f0639d2f290603a04ec146beb3f9569280f
 * Flaky: Yes (context-dependent on plugin activation timing)
 * Description: Deadlock in plugin handlers. When a plugin handler is registered
 * after the plugin is already activated, the activated flag is reset to false.
 * Any code waiting for activation (via waitActive) will block forever because
 * the condition variable is never broadcast again if no broadcast happens after
 * the flag is reset AND the code doesn't check for handler update state.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type PluginWithHandler struct {
	mu           sync.Mutex
	activateWait *sync.Cond
	// @guarded_by(mu)
	activated bool
	// @guarded_by(mu)
	handlersRegistered bool
}

// BUG: This method waits for activation and checks activated flag,
// but doesn't check if handlers have been registered. If a handler
// is registered while activated is true, activated gets reset to false
// without broadcast, and any existing waiters will hang forever.
//
// @acquires(p.mu)
func (p *PluginWithHandler) waitActive() error {
	p.mu.Lock()

	// BUG: Only checks activated flag, not handlersRegistered
	// If handlers are registered after activation, activated might be reset
	// but waiters will continue waiting with no further broadcasts
	for !p.activated {
		p.activateWait.Wait()
	}

	p.mu.Unlock()
	return nil
}

// Marks the plugin as activated and broadcasts.
// @acquires(p.mu)
func (p *PluginWithHandler) setActivated() {
	p.mu.Lock()

	p.activated = true
	p.activateWait.Broadcast()

	p.mu.Unlock()
}

// BUG: Registers a handler by resetting the activated flag.
// Any existing waiters in waitActive() will wake up, check activated (now false),
// and wait forever because there's no subsequent broadcast.
//
// @acquires(p.mu)
func (p *PluginWithHandler) registerHandler() {
	p.mu.Lock()

	// BUG: Setting activated to false without proper wait loop in waitActive
	// to check for handler state changes. Existing waiters will hang.
	p.activated = false
	p.handlersRegistered = true
	// BUG: No broadcast here to wake up and re-check conditions

	p.mu.Unlock()
}

func TestMoby29733(t *testing.T) {
	p := &PluginWithHandler{
		activateWait: sync.NewCond(&sync.Mutex{}),
	}

	// Initialize guarded fields under lock
	p.mu.Lock()
	p.activated = false
	p.handlersRegistered = false
	p.activateWait = sync.NewCond(&p.mu)
	p.mu.Unlock()

	// Thread A: Activate the plugin
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.setActivated()
	}()

	// Thread B: Wait for activation (will succeed)
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- p.waitActive()
	}()

	// Give time for waitActive to pass
	time.Sleep(50 * time.Millisecond)

	// Thread C: Register a handler, resetting activated flag
	// This will cause deadlock if any new thread tries to wait
	go func() {
		p.registerHandler()
	}()

	// Thread D: Try to wait for activation after handler registration
	// This will deadlock because activated is now false and no broadcast occurs
	deadlockChan := make(chan error, 1)
	go func() {
		deadlockChan <- p.waitActive()
	}()

	// Original waiter should have succeeded
	select {
	case <-waitDone:
		// OK - original waiter succeeded
	case <-time.After(100 * time.Millisecond):
		t.Fatal("original waitActive deadlocked")
	}

	// New waiter will deadlock
	select {
	case <-deadlockChan:
		// Should not reach here
		t.Fatal("new waitActive succeeded when it should deadlock")
	case <-time.After(200 * time.Millisecond):
		// BUG: Deadlock occurs - new waiter never returns
	}
}
