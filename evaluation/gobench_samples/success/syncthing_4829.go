/*
 * Project: syncthing
 * Issue or PR  : https://github.com/syncthing/syncthing/pull/4829
 * Buggy version: e7dc2f91900c9394529190de921c96b3b2d4eb44 (before fix)
 * fix commit-id: dac6e17a50d041857f58d86c341f64e92f29abd6
 * Flaky: Yes (intermittent, context-dependent)
 * Description: This deadlock is caused by a single goroutine trying to acquire
 * a read lock (RLock) on a mutex that it already holds with a write lock (Lock).
 * clearAddresses() acquires m.mut with Lock(), then calls notify() which tries
 * to acquire m.mut with RLock(), causing a deadlock.
 */
package gobench_samples

// | Bug ID|  Ref | Patch | Type | SubType | SubsubType |
// | ----  | ---- | ----  | ---- | ---- | ---- |
// |[syncthing#4829]|[pull request]|[patch]| Blocking | Resource Deadlock | Self-Deadlock (Lock-RLock) |

// [syncthing#4829]:(syncthing4829_test.go)
// [patch]:https://github.com/syncthing/syncthing/pull/4829/files
// [pull request]:https://github.com/syncthing/syncthing/pull/4829

import (
	"net"
	"sync"
	"testing"
)

type SyncthingAddress struct {
	IP   net.IP
	Port int
}

type Mapping struct {
	mu sync.RWMutex
	// @guarded_by(mu)
	extAddresses map[string]SyncthingAddress
	// @guarded_by(mu)
	subscribers []MappingChangeSubscriber
	// @guarded_by(mu)
	expires int
}

type MappingChangeSubscriber interface {
	NotifyAddressChanged(added, removed []SyncthingAddress)
}

type MockSubscriber struct{}

func (m *MockSubscriber) NotifyAddressChanged(added, removed []SyncthingAddress) {}

// BUG: This method acquires a write lock, then calls notify() which
// tries to acquire a read lock on the same mutex. Since the thread
// already holds a write lock, it cannot also acquire a read lock.
// This causes a self-deadlock (single-threaded deadlock).
//
// @acquires(m.mu)
func (m *Mapping) clearAddresses() {
	m.mu.Lock() // Writer Lock acquired

	removed := []SyncthingAddress{}
	for id := range m.extAddresses {
		addr := m.extAddresses[id]
		removed = append(removed, addr)
		delete(m.extAddresses, id)
	}

	// BUG: Still holding writer lock here, but calling notify
	// which tries to acquire reader lock on the same mutex
	if len(removed) > 0 {
		m.notify(nil, removed) // Tries to acquire RLock while holding Lock -> DEADLOCK
	}

	// These lines would execute after notify returns, but they never do
	m.expires = 0
	m.mu.Unlock() // Writer Lock release (never reached)
}

// BUG: This method tries to acquire a read lock.
// If called from clearAddresses(), the thread already holds a write lock
// on the same mutex, which causes a self-deadlock.
//
// @acquires(m.mu)
func (m *Mapping) notify(added, removed []SyncthingAddress) {
	m.mu.RLock() // Tries to acquire reader lock - DEADLOCK HERE if called from clearAddresses

	subscribers := make([]MappingChangeSubscriber, len(m.subscribers))
	copy(subscribers, m.subscribers)

	m.mu.RUnlock()

	for _, sub := range subscribers {
		sub.NotifyAddressChanged(added, removed)
	}
}

// @acquires(m.mu)
func (m *Mapping) Subscribe(sub MappingChangeSubscriber) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers = append(m.subscribers, sub)
}

func TestSyncthing4829(t *testing.T) {
	mapping := &Mapping{}

	// Initialize guarded fields under lock
	mapping.mu.Lock()
	mapping.extAddresses = make(map[string]SyncthingAddress)
	mapping.subscribers = []MappingChangeSubscriber{}
	mapping.mu.Unlock()

	// Add a mock subscriber to ensure notify will be called
	subscriber := &MockSubscriber{}
	mapping.Subscribe(subscriber)

	// Add a mapped address under lock
	mapping.mu.Lock()
	mapping.extAddresses["test"] = SyncthingAddress{
		IP:   net.ParseIP("192.168.0.1"),
		Port: 1024,
	}
	mapping.mu.Unlock()

	// This call will deadlock: clearAddresses locks mut, then calls notify
	// which tries to acquire the same lock with RLock
	mapping.clearAddresses()
}
