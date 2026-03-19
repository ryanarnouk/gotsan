/*
 * Project: kubernetes
 * Issue or PR  : https://github.com/kubernetes/kubernetes/pull/62464
 * Buggy version: Before fca65dcd645e92969519d80a7bb734b3da8c2eeb
 * fix commit-id: fca65dcd645e92969519d80a7bb734b3da8c2eeb
 * Flaky: Context-dependent on goroutine scheduling
 * Description: Deadlock in CPUManager due to unsafe recursive RLock() on RWMutex.
 * GetCPUSetOrDefault tries to RLock twice while SetDefaultCPUSet holds WLock.
 * According to golang/go#15418, this is not safe and can deadlock.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type CPUManager struct {
	mu sync.RWMutex
	// @guarded_by(mu)
	cpuSets map[string]string
}

// @acquires(cm.mu)
func (cm *CPUManager) GetCPUSetOrDefault(pod string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cpuSet, exists := cm.cpuSets[pod]; exists {
		return cpuSet
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return "default"
}

// @acquires(cm.mu)
func (cm *CPUManager) SetDefaultCPUSet(defaultSet string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cpuSets["default"] = defaultSet
	return nil
}

func TestKubernetes62464(t *testing.T) {
	cm := &CPUManager{
		cpuSets: make(map[string]string),
	}

	cm.mu.Lock()
	cm.cpuSets["pod1"] = "cpuset1"
	cm.mu.Unlock()

	getChan := make(chan string, 1)
	go func() {
		result := cm.GetCPUSetOrDefault("pod1")
		getChan <- result
	}()

	time.Sleep(5 * time.Millisecond)

	setDone := make(chan error, 1)
	go func() {
		setDone <- cm.SetDefaultCPUSet("newdefault")
	}()

	time.Sleep(5 * time.Millisecond)

	readChan := make(chan string, 1)
	go func() {
		result := cm.GetCPUSetOrDefault("pod2")
		readChan <- result
	}()

	// Check for deadlock - the recursive RLock should cause issues
	select {
	case <-getChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: first GetCPUSetOrDefault blocked (recursive RLock issue)")
	}

	select {
	case err := <-setDone:
		if err != nil {
			t.Logf("set failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: SetDefaultCPUSet blocked")
	}

	select {
	case <-readChan:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: second GetCPUSetOrDefault blocked")
	}
}
