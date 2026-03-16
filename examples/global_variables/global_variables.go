package simple

import (
	"fmt"
	"sync"
)

var (
	mu sync.Mutex
)

var test = "test string"

// @requires(mu)
func unguardedLockAccess() {
	fmt.Println(test)
}

func main() {
	unguardedLockAccess()
}
