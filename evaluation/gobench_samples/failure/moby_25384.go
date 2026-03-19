/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/issues/25384
 * Buggy version: Before 5de2e6d7b8494e662a7b53c287dad25bc6139747
 * fix commit-id: 5de2e6d7b8494e662a7b53c287dad25bc6139747
 * Flaky: Yes (timing-dependent with multiple plugins)
 * Description: Deadlock when loading multiple plugins. The plugin loading
 * mechanism uses a WaitGroup to wait for all plugins to load, but the
 * Done() count doesn't match the initial Add() count, causing the Wait()
 * to hang forever when multiple plugins are installed.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type PluginManager struct {
	mu sync.Mutex
	// @guarded_by(mu)
	loadWaiters *sync.WaitGroup
}

// BUG: This function adds to the WaitGroup for each plugin
// but might not call Done() for all of them, causing Wait() to hang.
//
// @acquires(pm.mu)
func (pm *PluginManager) loadPlugin(id int, shouldFail bool) {
	pm.mu.Lock()
	wg := pm.loadWaiters
	pm.mu.Unlock()

	if shouldFail {
		// BUG: Early return without calling Done() on the WaitGroup
		// This leaves the count incremented, so Wait() will never complete
		return
	}

	// Do some work...
	time.Sleep(10 * time.Millisecond)

	// Only reached on success
	wg.Done()
}

// BUG: Waits for all plugins to finish loading
// If any plugin fails and returns early without Done(),
// this will wait forever.
//
// @acquires(pm.mu)
func (pm *PluginManager) waitForPlugins(timeout time.Duration) error {
	pm.mu.Lock()
	wg := pm.loadWaiters
	pm.mu.Unlock()

	// Create a done channel to handle timeout
	done := make(chan struct{})
	go func() {
		wg.Wait() // Waits for all Done() calls to match Add() calls
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return ErrPluginLoadTimeout
	}
}

type LoadError struct{}

func (e *LoadError) Error() string {
	return "plugin load failed"
}

var ErrPluginLoadTimeout = &LoadError{}

func TestMoby25384(t *testing.T) {
	pm := &PluginManager{}

	pm.mu.Lock()
	pm.loadWaiters = &sync.WaitGroup{}
	pm.mu.Unlock()

	// Load 3 plugins, but one will fail
	plugins := []int{1, 2, 3}

	pm.mu.Lock()
	pm.loadWaiters.Add(len(plugins))
	pm.mu.Unlock()

	// Load plugins in parallel
	for _, id := range plugins {
		go func(pluginID int) {
			// Plugin 2 will fail
			shouldFail := pluginID == 2
			pm.loadPlugin(pluginID, shouldFail)
		}(id)
	}

	// Try to wait for all plugins to load
	// BUG: This will deadlock because plugin 2 returned without Done()
	select {
	case <-time.After(500 * time.Millisecond):
		// BUG: Deadlock - WaitGroup.Wait() never returns
		// because not all Done() calls were made
	default:
		t.Fatal("expected timeout from deadlock")
	}

	// Only 2 out of 3 plugins called Done()
	// So Wait() will hang forever waiting for the third
	err := pm.waitForPlugins(100 * time.Millisecond)
	if err != ErrPluginLoadTimeout {
		t.Fatalf("expected timeout, got %v", err)
	}
}
