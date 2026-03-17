/*
 * Project: cockroach
 * Issue or PR  : https://github.com/cockroachdb/cockroach/pull/10214
 * Buggy version: 7207111aa3a43df0552509365fdec741a53f873f
 * fix commit-id: 27e863d90ab0660494778f1c35966cc5ddc38e32
 * Flaky: 3/100
 * Description: This deadlock is caused by different order when acquiring
 * coalescedMu.Lock() and raftMu.Lock(). The fix is to refactor sendQueuedHeartbeats()
 * so that cockroachdb can unlock coalescedMu before locking raftMu.
 */
package gobench_samples

// | Bug ID|  Ref | Patch | Type | SubType | SubsubType |
// | ----  | ---- | ----  | ---- | ---- | ---- |
// |[cockroach#10214]|[pull request]|[patch]| Blocking | Resource Deadlock | AB-BA deadlock |

// [cockroach#10214]:(cockroach10214_test.go)
// [patch]:https://github.com/cockroachdb/cockroach/pull/10214/files
// [pull request]:https://github.com/cockroachdb/cockroach/pull/10214

import (
	"sync"
	"testing"
	"unsafe"
)

type Store10214 struct {
	coalescedMu struct {
		sync.Mutex
		heartbeatResponses []int
	}
	mu struct {
		replicas map[int]*Replica10214
	}
}

// @acquires(s.coalescedMu)
func (s *Store10214) sendQueuedHeartbeats() {
	s.coalescedMu.Lock()         // LockA acquire
	defer s.coalescedMu.Unlock() // LockA release
	for i := 0; i < len(s.coalescedMu.heartbeatResponses); i++ {
		s.sendQueuedHeartbeatsToNode() // LockB
	}
}

func (s *Store10214) sendQueuedHeartbeatsToNode() {
	for i := 0; i < len(s.mu.replicas); i++ {
		r := s.mu.replicas[i]
		r.reportUnreachable() // LockB
	}
}

type Replica10214 struct {
	raftMu sync.Mutex
	mu     sync.Mutex
	store  *Store10214
}

// @acquires(r.raftMu)
func (r *Replica10214) reportUnreachable() {
	r.raftMu.Lock() // LockB acquire
	//+time.Sleep(time.Nanosecond)
	defer r.raftMu.Unlock()
	// LockB release
}

// @acquires(r.raftMu)
func (r *Replica10214) tick() {
	r.raftMu.Lock() // LockB acquire
	defer r.raftMu.Unlock()
	r.tickRaftMuLocked()
	// LockB release
}

// @acquires(r.mu)
func (r *Replica10214) tickRaftMuLocked() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.maybeQuiesceLocked() {
		return
	}
}
func (r *Replica10214) maybeQuiesceLocked() bool {
	for i := 0; i < 2; i++ {
		if !r.maybeCoalesceHeartbeat() {
			return true
		}
	}
	return false
}

// @acquires(r.store.coalescedMu)
func (r *Replica10214) maybeCoalesceHeartbeat() bool {
	msgtype := uintptr(unsafe.Pointer(r)) % 3
	switch msgtype {
	case 0, 1, 2:
		r.store.coalescedMu.Lock() // LockA acquire
	default:
		return false
	}
	r.store.coalescedMu.Unlock() // LockA release
	return true
}

func TestCockroach10214(t *testing.T) {
	store := &Store10214{}
	responses := &store.coalescedMu.heartbeatResponses
	*responses = append(*responses, 1, 2)
	store.mu.replicas = make(map[int]*Replica10214)

	rp1 := &Replica10214{
		store: store,
	}
	rp2 := &Replica10214{
		store: store,
	}
	store.mu.replicas[0] = rp1
	store.mu.replicas[1] = rp2

	go func() {
		store.sendQueuedHeartbeats()
	}()

	go func() {
		rp1.tick()
	}()
}
