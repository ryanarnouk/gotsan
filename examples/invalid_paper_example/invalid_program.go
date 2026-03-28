package invalid_paper_example

import "sync"

type Example struct {
	mu sync.Mutex

	// @guarded_by(mu)
	importantField int
}

// @requires(example.mu)
// @returns(example.mu)
func func2(example *Example) {
	example.importantField = 1
}

// @acquires(example.mu)
func func1(value *int, example *Example) {
	example.mu.Lock()
	if *value == 0 {
		// Does not unlock Mutex on function exit
		return
	} else if *value == 1 {
		example.mu.Unlock()
		// Not currently guarded by Mutex
		example.importantField = 2
	} else {
		example.mu.Unlock()
		// func2 requies mutex to be held, no longer held
		func2(example)
	}
}

func main() {
	example := Example{}
	value := 0
	func1(&value, &example)
}
