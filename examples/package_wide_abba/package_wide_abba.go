package main

import "sync"

var (
	muA sync.Mutex
	muB sync.Mutex
)

// @acquires(muA)
// @acquires(muB)
func lockAB() {
	muA.Lock()
	muB.Lock()
	muB.Unlock()
	muA.Unlock()
}

// @acquires(muB)
// @acquires(muA)
func lockBA() {
	muB.Lock()
	muA.Lock()
	muA.Unlock()
	muB.Unlock()
}

func launchAB() {
	go lockAB()
}

func launchBA() {
	go lockBA()
}

func main() {
	launchAB()
	launchBA()
	select {}
}
