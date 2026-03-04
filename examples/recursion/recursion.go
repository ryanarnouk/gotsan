package recursion

// Examples and test using recursive functions
// and verifying analysis does not fail

import (
	"fmt"
	"sync"
)

var (
	mu          sync.Mutex
	sharedValue int
)

// @acquires(mu)
func SafeRecursiveCounter(n int) {
	mu.Lock()
	fmt.Printf("Counter at: %d\n", n)
	mu.Unlock() // Lock is NOT retained

	if n > 0 {
		SafeRecursiveCounter(n - 1)
	}
}

// @acquires(mu)
func ExportedUpdate(n int) {
	mu.Lock()
	defer mu.Unlock()

	recursiveInternalLogic(n)
}

// @requires(mu)
// @returns(mu)
func recursiveInternalLogic(n int) {
	if n <= 0 {
		return
	}
	sharedValue += n

	// The mutex remains acquired during this call
	recursiveInternalLogic(n - 1)
}

// @acquires(mu)
// @returns(mu)
func DeadlockingFactorial(n int) int {
	mu.Lock()
	defer mu.Unlock()

	if n <= 1 {
		return 1
	}

	// This should be flagged: DeadlockingFactorial 'requires' mu to be
	// unlocked to 'acquire' it, but the caller currently holds it.
	return n * DeadlockingFactorial(n-1)
}

// Factorial is the entry point for the calculation.
// It handles the initial acquisition of the lock.
// @acquires(mu)
func Factorial(n uint64) uint64 {
	mu.Lock()
	defer mu.Unlock()

	return recursiveFactorial(n)
}

// recursiveFactorial performs the actual calculation.
// It relies on the caller to have already acquired the lock.
// @requires(mu)
// @returns(mu)
func recursiveFactorial(n uint64) uint64 {
	if n == 0 {
		return 1
	}

	// The tool should see that mu is held throughout this recursive call.
	return n * recursiveFactorial(n-1)
}

func main() {
	fmt.Println("--- Safe Recursion ---")
	SafeRecursiveCounter(3)

	fmt.Println("\n--- Internal Helper ---")
	ExportedUpdate(5)
	fmt.Printf("Final Shared Value: %d\n", sharedValue)

	// fmt.Println(DeadlockingFactorial(5)) // Logic test: will deadlock
}
