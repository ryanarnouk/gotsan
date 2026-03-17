package gobench_samples

// | Bug ID|  Ref | Patch | Type | SubType | SubsubType |
// | ----  | ---- | ----  | ---- | ---- | ---- |
// |[grpc#795]|[pull request]|[patch]| Blocking | Resource Deadlock | Double locking |

// [grpc#795]:(grpc795_test.go)
// [patch]:https://github.com/grpc/grpc-go/pull/795/files
// [pull request]:https://github.com/grpc/grpc-go/pull/795

// ## Description

// line 20 missing unlock

import (
	"sync"
	"testing"
)

type Server struct {
	mu    sync.Mutex
	drain bool
}

// NOTE: if you add the @requires tag to this function, no warnings
// will be emitted. But otherwise, under assumption that the programmer
// expects this function to release locks (and therefore they
// did not add the annotation) it should return an error
// @acquires(s.mu)
func (s *Server) GracefulStop() {
	s.mu.Lock()
	if s.drain == true {
		s.mu.Lock()
		return
	}
	s.drain = true
} // Missing Unlock

// @acquires(s.mu)
func (s *Server) Serve() {
	s.mu.Lock()
	s.mu.Unlock()
}

func NewServer() *Server {
	return &Server{}
}

type test struct {
	srv *Server
}

func (te *test) startServer() {
	s := NewServer()
	te.srv = s
	go s.Serve()
}

func newTest() *test {
	return &test{}
}

func testServerGracefulStopIdempotent() {
	te := newTest()

	te.startServer()

	for i := 0; i < 3; i++ {
		te.srv.GracefulStop()
	}
}

func TestGrpc795(t *testing.T) {
	testServerGracefulStopIdempotent()
}
