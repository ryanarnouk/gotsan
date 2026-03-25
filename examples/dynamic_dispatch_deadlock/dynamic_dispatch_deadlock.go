package main

import "sync"

var (
	muA sync.Mutex
	muB sync.Mutex
)

type Task interface {
	Work()
}

type ABWorker struct {
}

// @acquires(muA)
// @acquires(muB)
func (w *ABWorker) Work() {
	muA.Lock()
	defer muA.Unlock()

	// Nested lock while holding first.
	muB.Lock()
	defer muB.Unlock()
}

type BAWorker struct {
}

// @acquires(muB)
// @acquires(muA)
func (w *BAWorker) Work() {
	muB.Lock()
	defer muB.Unlock()

	// Nested lock while holding first.
	muA.Lock()
	defer muA.Unlock()
}

func runTask(t Task) {
	// Dynamic dispatch happens here through the Task interface.
	t.Work()
}

func launchAB() {
	go runTask(&ABWorker{})
}

func launchBA() {
	go runTask(&BAWorker{})
}

func main() {
	launchAB()
	launchBA()

	select {}
}
