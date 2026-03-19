/*
 * Project: moby (Docker)
 * Issue or PR  : https://github.com/moby/moby/pull/7559
 * Buggy version: Before 6cbb8e070d6c3a66bf48fbe5cbf689557eee23db
 * fix commit-id: 6cbb8e070d6c3a66bf48fbe5cbf689557eee23db
 * Flaky: Yes (when DialUDP fails)
 * Description: This deadlock is caused by acquiring a lock and then
 * calling continue on an error without unlocking. The UDP proxy acquires
 * connTrackLock, then tries to dial a UDP connection. On error, it logs
 * and continues WITHOUT unlocking, leaving the lock held permanently.
 * The next iteration tries to acquire the same lock and deadlocks.
 */
package gobench_samples

import (
	"net"
	"sync"
	"testing"
	"time"
)

type UDPProxy struct {
	connTrackLock sync.Mutex
	// @guarded_by(connTrackLock)
	connTrackTable map[string]*net.UDPConn
	backendAddr    *net.UDPAddr
	frontendAddr   *net.UDPAddr
	fail           bool // Simulate dial failure
}

// BUG: This method acquires connTrackLock in a loop, but if dialBackend()
// fails, the continue statement skips the Unlock, leaving the lock held.
// On the next loop iteration, trying to acquire the same lock causes a deadlock.
//
// @acquires(proxy.connTrackLock)
func (proxy *UDPProxy) Run() {
	for i := 0; i < 10; i++ {
		fromKey := "client_" + string(rune(i))

		proxy.connTrackLock.Lock() // Lock acquired

		proxyConn, err := proxy.dialBackend()
		if err != nil {
			// BUG: Lock is held but not released before continue
			// This causes temporary lock contention on next iteration
			// and eventual deadlock if another goroutine holds the lock
			continue // ERROR PATH: Lock not released!
		}

		proxy.connTrackTable[fromKey] = proxyConn
		proxy.connTrackLock.Unlock()
	}
}

func (proxy *UDPProxy) dialBackend() (*net.UDPConn, error) {
	if proxy.fail {
		return nil, &net.OpError{
			Op:     "dial",
			Net:    "udp",
			Source: nil,
			Addr:   proxy.backendAddr,
			Err:    net.UnknownNetworkError("simulated failure"),
		}
	}
	conn, err := net.DialUDP("udp", nil, proxy.backendAddr)
	return conn, err
}

// This goroutine tries to acquire the lock while Run() holds it
// @acquires(proxy.connTrackLock)
func (proxy *UDPProxy) blockingOperation() {
	time.Sleep(10 * time.Millisecond)
	proxy.connTrackLock.Lock() // Tries to acquire but Run() still holds it
	defer proxy.connTrackLock.Unlock()
}

func TestMoby7559(t *testing.T) {
	// Parse a valid UDP address
	backendAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("Failed to resolve UDP address: %v", err)
	}

	proxy := &UDPProxy{
		backendAddr: backendAddr,
		fail:        true, // Force dial to fail
	}

	// Initialize the guarded field under lock
	proxy.connTrackLock.Lock()
	proxy.connTrackTable = make(map[string]*net.UDPConn)
	proxy.connTrackLock.Unlock()

	// Start the proxy which will fail on dial and not release the lock
	go func() {
		proxy.Run()
	}()

	// Try to acquire the lock from another goroutine
	// This will deadlock because Run() holds and never releases the lock
	go func() {
		proxy.blockingOperation()
	}()

	// Give the deadlock a chance to occur
	time.Sleep(100 * time.Millisecond)
}
