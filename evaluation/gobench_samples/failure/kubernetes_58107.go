/*
 * Project: kubernetes
 * Issue or PR  : https://github.com/kubernetes/kubernetes/pull/58107
 * Buggy version: Before fix
 * fix commit-id: 15b1d165fb3124ee5bb5d72d2da2d4cff1946d57
 * Flaky: Yes (depends on worker queue timing)
 * Description: Resource quota controller worker deadlock. Workers acquire read lock
 * while idle waiting for work. Sync() needs write lock. If Sync() blocks on write,
 * workers trying to release read lock become blocked on read lock reacquisition.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type QuotaControllerWorker struct {
	mu sync.RWMutex
	// @guarded_by(mu)
	syncNeeded bool
	// @guarded_by(mu)
	quotas map[string]int
}

// @acquires(qc.mu)
func (qc *QuotaControllerWorker) workerProcess() error {
	qc.mu.RLock()
	defer qc.mu.RUnlock()

	if qc.syncNeeded {
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// @acquires(qc.mu)
func (qc *QuotaControllerWorker) Sync() error {
	qc.mu.Lock()
	defer qc.mu.Unlock()

	qc.syncNeeded = true
	qc.quotas["memory"] = 10
	return nil
}

func TestKubernetes58107(t *testing.T) {
	qc := &QuotaControllerWorker{
		syncNeeded: false,
		quotas:     make(map[string]int),
	}

	workerDone := make(chan error, 1)
	go func() {
		workerDone <- qc.workerProcess()
	}()

	time.Sleep(10 * time.Millisecond)

	syncDone := make(chan error, 1)
	go func() {
		syncDone <- qc.Sync()
	}()

	time.Sleep(10 * time.Millisecond)

	worker2Done := make(chan error, 1)
	go func() {
		worker2Done <- qc.workerProcess()
	}()

	select {
	case err := <-syncDone:
		if err != nil {
			t.Logf("sync failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: Sync blocked waiting for worker RLocks to be released")
	}

	select {
	case err := <-workerDone:
		if err != nil {
			t.Logf("worker 1 failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: worker 1 blocked")
	}

	select {
	case err := <-worker2Done:
		if err != nil {
			t.Logf("worker 2 failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: worker 2 blocked trying to acquire RLock behind Sync's WLock")
	}
}
