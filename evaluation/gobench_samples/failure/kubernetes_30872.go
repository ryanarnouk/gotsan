/*
 * Project: kubernetes
 * Issue or PR  : https://github.com/kubernetes/kubernetes/pull/30872
 * Buggy version: Before fix
 * fix commit-id: b15c2d67e672897fc1b30dd879d8f4c6065181a5
 * Flaky: Yes (AB/BA deadlock depends on timing)
 * Description: Deadlock in federated informer. OnAdd locks ClusterInformer state
 * while calling syncAll which tries to lock subinformer. Meanwhile Synced() locks
 * then tries to access ClusterInformer - classic AB/BA deadlock.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type FederatedInformer struct {
	mu sync.Mutex
	// @guarded_by(mu)
	ClusterInformer map[string]string
}

type SubInformer struct {
	mu sync.Mutex
	// @guarded_by(mu)
	items map[string]string
}

// @acquires(fi.mu)
func (fi *FederatedInformer) OnAdd(cluster string, si *SubInformer) {
	fi.mu.Lock()
	defer fi.mu.Unlock()

	fi.ClusterInformer[cluster] = "added"

	si.mu.Lock()
	si.items["cluster"] = cluster
	si.mu.Unlock()
}

// @acquires(si.mu)
func (si *SubInformer) Synced(fi *FederatedInformer) bool {
	si.mu.Lock()
	defer si.mu.Unlock()

	synced := len(si.items) > 0

	fi.mu.Lock()
	inSync := len(fi.ClusterInformer) > 0
	fi.mu.Unlock()

	return synced && inSync
}

func TestKubernetes30872(t *testing.T) {
	fi := &FederatedInformer{
		ClusterInformer: make(map[string]string),
	}

	si := &SubInformer{
		items: make(map[string]string),
	}

	// Thread A: OnAdd (Path A: fi.mu -> si.mu)
	addDone := make(chan struct{}, 1)
	go func() {
		fi.OnAdd("cluster1", si)
		addDone <- struct{}{}
	}()

	time.Sleep(5 * time.Millisecond)

	// Thread B: Synced (Path B: si.mu -> fi.mu)
	// This can deadlock with Thread A
	syncedDone := make(chan bool, 1)
	go func() {
		syncedDone <- si.Synced(fi)
	}()

	select {
	case <-addDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: OnAdd blocked (AB/BA deadlock)")
	}

	select {
	case <-syncedDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: Synced blocked (AB/BA deadlock)")
	}
}
