package valid_paper_example

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
func func1(example *Example) {
	example.mu.Lock()
	func2(example)
	example.mu.Unlock()
}

func main() {
	example := Example{}
	func1(&example)
}
