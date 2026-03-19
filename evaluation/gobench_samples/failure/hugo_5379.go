/*
 * Project: hugo
 * Issue or PR  : https://github.com/gohugoio/hugo/pull/5379
 * Buggy version: Before fix
 * fix commit-id: 729593c842794eaf7127050953a5c2256d332051
 * Flaky: Yes (timing-dependent on content build timeout)
 * Description: Deadlock in content builder when timeout occurs. BuildContent()
 * holds lock while building. When timeout fires, the timeout handler tries to
 * re-acquire the same lock, causing a deadlock with pending operations.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type ContentBuilder struct {
	mu sync.Mutex
	// @guarded_by(mu)
	content map[string]string
	// @guarded_by(mu)
	building bool
}

// @acquires(cb.mu)
func (cb *ContentBuilder) BuildContent(timeout time.Duration) error {
	cb.mu.Lock()
	cb.building = true

	done := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cb.content["page"] = "content"
		done <- nil
	}()

	select {
	case err := <-done:
		cb.building = false
		cb.mu.Unlock()
		return err
	case <-time.After(timeout):
		cb.onTimeout()
		cb.building = false
		cb.mu.Unlock()
		return nil
	}
}

func (cb *ContentBuilder) onTimeout() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.building = false
	cb.content["error"] = "timeout"
}

// @acquires(cb.mu)
func (cb *ContentBuilder) GetContent() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if content, ok := cb.content["page"]; ok {
		return content
	}
	return ""
}

func TestHugo5379(t *testing.T) {
	cb := &ContentBuilder{
		content:  make(map[string]string),
		building: false,
	}

	// Thread A: BuildContent with short timeout (will trigger timeout)
	buildDone := make(chan error, 1)
	go func() {
		buildDone <- cb.BuildContent(20 * time.Millisecond)
	}()

	time.Sleep(5 * time.Millisecond)

	// Thread B: Try to get content while build is happening
	contentDone := make(chan string, 1)
	go func() {
		contentDone <- cb.GetContent()
	}()

	select {
	case err := <-buildDone:
		if err != nil {
			t.Logf("build failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: BuildContent blocked (timeout handler re-acquires lock)")
	}

	select {
	case <-contentDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: GetContent blocked waiting for BuildContent")
	}
}
