/*
 * Project: kubernetes
 * Issue or PR  : https://github.com/kubernetes/kubernetes/pull/13135
 * Buggy version: Before fix
 * fix commit-id: 9f1d2af5fb59e2797d4f5b859d45504e1e9dae4e
 * Flaky: Yes (depends on watch callback timing)
 * Description: Deadlock in Cacher watchCache on etcd errors. StartCaching() holds
 * lock while delivering event to watchers via callbacks. Callbacks try to re-acquire
 * the same lock, causing deadlock when stopping/restarting the cache.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type Cacher struct {
	mu sync.RWMutex
	// @guarded_by(mu)
	caching bool
	// @guarded_by(mu)
	watchCallbacks []func()
}

// @acquires(c.mu)
// @returns(c.mu)
func (c *Cacher) StartCaching() {
	c.mu.Lock()

	if c.caching {
		c.mu.Unlock()
		return
	}

	c.caching = true

	for _, callback := range c.watchCallbacks {
		callback()
	}

	c.mu.Unlock()
}

// @acquires(c.mu)
func (c *Cacher) onCachingStarted() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.caching = false
}

// @acquires(c.mu)
func (c *Cacher) WaitForSync() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.caching
}

func TestKubernetes13135(t *testing.T) {
	c := &Cacher{
		caching:        false,
		watchCallbacks: make([]func(), 0),
	}

	// Register callback that tries to re-acquire lock
	c.watchCallbacks = append(c.watchCallbacks, c.onCachingStarted)

	// Thread A: StartCaching (holds lock, calls callbacks)
	startDone := make(chan struct{}, 1)
	go func() {
		c.StartCaching()
		startDone <- struct{}{}
	}()

	time.Sleep(10 * time.Millisecond)

	// Thread B: Try to wait for sync
	waitDone := make(chan bool, 1)
	go func() {
		waitDone <- c.WaitForSync()
	}()

	select {
	case <-startDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: StartCaching blocked (callback re-acquires lock)")
	}

	select {
	case <-waitDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: WaitForSync blocked waiting for StartCaching to release lock")
	}
}
