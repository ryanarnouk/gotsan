/*
 * Project: hugo
 * Issue or PR  : https://github.com/gohugoio/hugo/pull/3251
 * Buggy version: Before fix
 * fix commit-id: 79b34c2f1e0ba91ff5f4f879dc42eddfd82cc563
 * Flaky: Yes (semaphore throttling timing)
 * Description: Deadlock in getJSON during page rendering. Template rendering
 * acquires lock while calling getJSON, which tries to acquire semaphore.
 * Concurrent getJSON calls with limited semaphore permits cause deadlock.
 */
package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

type PageRenderer struct {
	mu sync.Mutex
	// @guarded_by(mu)
	rendered map[string]string
}

type JSONFetcher struct {
	semaphore chan struct{}
	cache     map[string]interface{}
	mu        sync.Mutex
	// @guarded_by(mu)
	pending bool
}

// @acquires(pr.mu)
func (pr *PageRenderer) RenderPage(pageID string, jf *JSONFetcher) error {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	data, err := jf.GetJSON("https://api.github.com/user")
	if err != nil {
		return err
	}

	pr.rendered[pageID] = data.(string)
	return nil
}

// @acquires(jf.mu)
func (jf *JSONFetcher) GetJSON(url string) (interface{}, error) {
	jf.semaphore <- struct{}{}
	defer func() { <-jf.semaphore }()

	jf.mu.Lock()
	if cached, ok := jf.cache[url]; ok {
		jf.mu.Unlock()
		return cached, nil
	}
	jf.pending = true
	jf.mu.Unlock()

	time.Sleep(50 * time.Millisecond)

	jf.mu.Lock()
	jf.cache[url] = "json_data"
	jf.pending = false
	jf.mu.Unlock()

	return "json_data", nil
}

func TestHugo3251(t *testing.T) {
	pr := &PageRenderer{
		rendered: make(map[string]string),
	}

	jf := &JSONFetcher{
		semaphore: make(chan struct{}, 1),
		cache:     make(map[string]interface{}),
		pending:   false,
	}

	renderDone := make(chan error, 1)
	go func() {
		renderDone <- pr.RenderPage("page1", jf)
	}()

	time.Sleep(10 * time.Millisecond)

	render2Done := make(chan error, 1)
	go func() {
		render2Done <- pr.RenderPage("page2", jf)
	}()

	select {
	case err := <-renderDone:
		if err != nil {
			t.Logf("render 1 failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: RenderPage 1 blocked")
	}

	select {
	case err := <-render2Done:
		if err != nil {
			t.Logf("render 2 failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deadlock: RenderPage 2 blocked (semaphore deadlock in getJSON)")
	}
}
