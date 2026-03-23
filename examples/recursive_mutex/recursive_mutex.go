package recursive_mutex

import (
	"sync"
)

type Struct struct{ sync.Mutex }

func main() {
	var s Struct
	s.A()
}

// @acquires(s.Mutex)
// @returns(s.Mutex)
func (s *Struct) A() {
	s.Lock()
	s.C(3)
	s.Unlock()
}

// @acquires(s.Mutex)
// @returns(s.Mutex)
func (s *Struct) B() {
	s.Lock()
	s.Unlock()
}

// @requires(s.Mutex)
// @returns(s.Mutex)
func (s *Struct) C(n int) {
	if n <= 0 {
		return
	}

	s.B()
	s.C(n - 1)
}
