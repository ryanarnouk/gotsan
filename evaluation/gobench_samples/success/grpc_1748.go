package gobench_samples

import (
	"sync"
	"testing"
	"time"
)

var minConnectTimeout = 10 * time.Second

// @guarded_by(balanceMutex)
var balanceMutex sync.Mutex // We add this for avoiding other data race

type Balancer interface {
	HandleResolvedAddrs()
}

type Builder1748 interface {
	Build(cc balancer_ClientConn1748) Balancer
}

func newPickfirstBuilder1748() Builder1748 {
	return &pickfirstBuilder1748{}
}

type pickfirstBuilder1748 struct{}

func (*pickfirstBuilder1748) Build(cc balancer_ClientConn1748) Balancer {
	return &pickfirstBalancer{cc: cc}
}

type SubConn1748 interface {
	Connect()
}

type balancer_ClientConn1748 interface {
	NewSubConn1748() SubConn1748
}

type pickfirstBalancer struct {
	cc balancer_ClientConn1748
	sc SubConn1748
}

func (b *pickfirstBalancer) HandleResolvedAddrs() {
	b.sc = b.cc.NewSubConn1748()
	b.sc.Connect()
}

type pickerWrapper struct {
	mu sync.Mutex
}

type acBalancerWrapper struct {
	mu sync.Mutex
	// @guarded_by(mu)
	ac *addrConn
}

type addrConn struct {
	cc *ClientConn1748
	// @guarded_by(mu)
	acbw SubConn1748
	mu   sync.Mutex
}

func (ac *addrConn) resetTransport() {
	_ = minConnectTimeout
}

func (ac *addrConn) transportMonitor() {
	ac.resetTransport()
}

func (ac *addrConn) connect() {
	go func() {
		ac.transportMonitor()
	}()
}

func (acbw *acBalancerWrapper) Connect() {
	acbw.mu.Lock()
	defer acbw.mu.Unlock()
	acbw.ac.connect()
}

func newPickerWrapper() *pickerWrapper {
	return &pickerWrapper{}
}

type ClientConn1748 struct {
	mu sync.Mutex
}

func (cc *ClientConn1748) switchBalancer() {
	Builder1748 := newPickfirstBuilder1748()
	newCCBalancerWrapper(cc, Builder1748)
}

func (cc *ClientConn1748) newAddrConn() *addrConn {
	return &addrConn{cc: cc}
}

type ccBalancerWrapper struct {
	cc *ClientConn1748
	// @guarded_by(balanceMutex)
	balancer Balancer
}

func (ccb *ccBalancerWrapper) watcher() {
	for i := 0; i < 10; i++ {
		balanceMutex.Lock()
		if ccb.balancer != nil {
			balanceMutex.Unlock()
			ccb.balancer.HandleResolvedAddrs()
		} else {
			balanceMutex.Unlock()
		}
	}
}

// @acquires(acbw.ac.mu)
func (ccb *ccBalancerWrapper) NewSubConn1748() SubConn1748 {
	ac := ccb.cc.newAddrConn()
	acbw := &acBalancerWrapper{ac: ac}
	acbw.ac.mu.Lock()
	ac.acbw = acbw
	acbw.ac.mu.Unlock()
	return acbw
}

func newCCBalancerWrapper(cc *ClientConn1748, b Builder1748) {
	ccb := &ccBalancerWrapper{cc: cc}
	go ccb.watcher()
	balanceMutex.Lock()
	defer balanceMutex.Unlock()
	ccb.balancer = b.Build(ccb)
}

func TestGrpc1748(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		mctBkp := minConnectTimeout
		// Call this only after transportMonitor goroutine has ended.
		defer func() {
			minConnectTimeout = mctBkp
		}()
		cc := &ClientConn1748{}
		cc.switchBalancer()
	}()
	wg.Wait()
}
