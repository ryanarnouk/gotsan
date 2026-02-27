package analyzer

import "golang.org/x/tools/go/ssa"

// a worklist of blocks
type worklist struct {
	// queue of blocks to process
	queue []*ssa.BasicBlock
	// keep track of which blocks exist within the queue
	inQueue map[int]bool
}

func newBlockWorklist(entry *ssa.BasicBlock) *worklist {
	return &worklist{
		queue:   []*ssa.BasicBlock{entry},
		inQueue: map[int]bool{entry.Index: true},
	}
}

func (w *worklist) Push(b *ssa.BasicBlock) {
	if !w.inQueue[b.Index] {
		w.queue = append(w.queue, b)
		w.inQueue[b.Index] = true
	}
}

func (w *worklist) Pop() *ssa.BasicBlock {
	b := w.queue[0]
	w.queue = w.queue[1:]
	w.inQueue[b.Index] = false
	return b
}

func (w *worklist) Empty() bool {
	return len(w.queue) == 0
}
