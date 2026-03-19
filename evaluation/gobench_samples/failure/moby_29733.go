package moby29733

import (
	"sync"
	"testing"
)

type Plugin struct {
	activated    bool
	activateWait *sync.Cond
}

type plugins struct {
	sync.Mutex
	// @guarded_by(sync)
	plugins map[int]*Plugin
}

// @acquires(p.activateWait.L)
// @returns(p.activateWait.L)
func (p *Plugin) waitActive() {
	p.activateWait.L.Lock()
	for !p.activated {
		p.activateWait.Wait()
	}
	p.activateWait.L.Unlock()
}

type extpointHandlers struct {
	sync.RWMutex
	// @guarded_by(sync)
	//This is probably wrong
	extpointHandlers map[int]struct{}
}

var (
	storage  = plugins{plugins: make(map[int]*Plugin)}
	handlers = extpointHandlers{extpointHandlers: make(map[int]struct{})}
)

// @acquires(handlers)
// @returns(handlers)
func Handle() {
	handlers.Lock()
	for _, p := range storage.plugins {
		p.activated = false
	}
	handlers.Unlock()
}

func testActive(p *Plugin) {
	done := make(chan struct{})
	go func() {
		p.waitActive()
		close(done)
	}()
	<-done
}

func TestMoby29733(t *testing.T) {
	p := &Plugin{activateWait: sync.NewCond(&sync.Mutex{})}
	storage.plugins[0] = p

	testActive(p)
	Handle()
	testActive(p)
}
