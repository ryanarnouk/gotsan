/*
 * Project: cockroach
 * Issue or PR  : https://github.com/cockroachdb/cockroach/pull/3710
 * Buggy version: 4afdd4860fd7c3bd9e92489f84a95e5cc7d11a0d
 * fix commit-id: cb65190f9caaf464723e7d072b1f1b69a044ef7b
 * Flaky: 2/100
 * Description: This deadlock is casued by acquiring a RLock twice in a call chain.
 * ForceRaftLogScanAndProcess(acquire s.mu.RLock()) ->MaybeAdd()->shouldQueue()->
 * getTruncatableIndexes()->RaftStatus(acquire s.mu.Rlock())
 */

package gobench_samples

import (
	"sync"
	"testing"
	"unsafe"
)

type Store3710 struct {
	raftLogQueue *baseQueue
	replicas     map[int]*Replica3710

	mu struct {
		sync.RWMutex
	}
}

// @acquires(s.mu)
func (s *Store3710) ForceRaftLogScanAndProcess() {
	s.mu.RLock()
	for _, r := range s.replicas {
		s.raftLogQueue.MaybeAdd(r)
	}
	s.mu.RUnlock()
}

// @acquires(s.mu)
func (s *Store3710) RaftStatus() {
	s.mu.RLock()
	defer s.mu.RUnlock()
}

// @acquires(s.mu)
func (s *Store3710) processRaft() {
	go func() {
		for {
			var replicas []*Replica3710
			s.mu.Lock()
			for _, r := range s.replicas {
				replicas = append(replicas, r)
			}
			s.mu.Unlock()
			break
		}
	}()
}

type Replica3710 struct {
	store *Store3710
}

type baseQueue struct {
	sync.Mutex
	impl *raftLogQueue
}

// @acquire(bq)
func (bq *baseQueue) MaybeAdd(repl *Replica3710) {
	bq.Lock()
	defer bq.Unlock()
	bq.impl.shouldQueue(repl)
}

type raftLogQueue struct{}

func (*raftLogQueue) shouldQueue(r *Replica3710) {
	getTruncatableIndexes(r)
}

func getTruncatableIndexes(r *Replica3710) {
	r.store.RaftStatus()
}

func NewStore() *Store3710 {
	rlq := &raftLogQueue{}
	bq := &baseQueue{impl: rlq}
	store := &Store3710{
		raftLogQueue: bq,
		replicas:     make(map[int]*Replica3710),
	}
	r1 := &Replica3710{store}
	r2 := &Replica3710{store}

	makeKey := func(r *Replica3710) int {
		return int((uintptr(unsafe.Pointer(r)) >> 1) % 7)
	}
	store.replicas[makeKey(r1)] = r1
	store.replicas[makeKey(r2)] = r2

	return store
}

// / G1 										G2
// / store.ForceRaftLogScanAndProcess()
// / s.mu.RLock()
// / s.raftLogQueue.MaybeAdd()
// / bq.impl.shouldQueue()
// / getTruncatableIndexes()
// / r.store.RaftStatus()
// / 										store.processRaft()
// / 										s.mu.Lock()
// / s.mu.RLock()
// / ----------------------G1,G2 deadlock---------------------
func TestCockroach3710(t *testing.T) {
	store := NewStore()
	go store.ForceRaftLogScanAndProcess() // G1
	go store.processRaft()                // G2
}
