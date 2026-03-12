package main

import (
	"math/rand"
	"sync"
	"time"
)

var (
	mu   sync.Mutex
	data int // @guarded_by(mu)
)

// An example where Go's race flag does not exercise a branch path containing a concurrency bug due to the low chances of hitting that branch.
func worker() {
	if rand.Intn(100) == 0 {
		// Rare bug: skip the mutex
		data++
	} else {
		mu.Lock()
		data++
		mu.Unlock()
	}
}

func main() {
	go worker()
	go worker()

	time.Sleep(time.Millisecond)
}
