package gobench_samples

import (
	"errors"
	"sync"
	"testing"
)

type Manifest struct {
	Implements []string
}

type Plugin3048 struct {
	activateWait *sync.Cond
	activateErr  error
	Manifest     *Manifest
}

func (p *Plugin3048) waitActive3048() error {
	p.activateWait.L.Lock()
	for !p.activated() {
		p.activateWait.Wait()
	}
	p.activateWait.L.Unlock()
	return p.activateErr
}

func (p *Plugin3048) activated() bool {
	return p.Manifest != nil
}

func testActive3048(p *Plugin3048) {
	done := make(chan struct{})
	go func() {
		p.waitActive3048()
		close(done)
	}()
	<-done
}
func TestMoby30408(t *testing.T) {
	p := &Plugin3048{activateWait: sync.NewCond(&sync.Mutex{})}
	p.activateErr = errors.New("some junk happened")

	testActive(p)
}
