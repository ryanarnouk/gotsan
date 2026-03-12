package main

import "sync"

var mu1 sync.Mutex
var mu2 sync.Mutex

// @acquires(mu1)
func acquire1() {
	mu1.Lock()
	defer mu1.Unlock()
	// do something
}

// @acquires(mu2)
func acquire2() {
	mu2.Lock()
	defer mu2.Unlock()
	// do something
}

// @acquires(mu1)
// @acquires(mu2)
func acquireBoth_1then2() {
	mu1.Lock()
	mu2.Lock()
	mu2.Unlock()
	mu1.Unlock()
}

// @acquires(mu2)
// @acquires(mu1)
func acquireBoth_2then1() {
	mu2.Lock()
	mu1.Lock()
	mu1.Unlock()
	mu2.Unlock()
}

func singleThreadedExample() {
	// These two calls acquire the same locks in different orders
	// This could cause deadlock if executed concurrently
	acquireBoth_1then2() // mu1 -> mu2
	acquireBoth_2then1() // mu2 -> mu1  <- this should be an inversion/deadlock
}

func main() {
	singleThreadedExample()
}