package analyzer

import (
	"go/token"
	"gotsan/ir"
	"gotsan/utils"
	"gotsan/utils/logger"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

func updateSuccessorState(
	succ *ssa.BasicBlock,
	current AnalysisState,
	blockEntryStates map[int]AnalysisState,
	worklist *worklist,
) {
	existing, seen := blockEntryStates[succ.Index]

	if !seen {
		blockEntryStates[succ.Index] = current.Copy()
		worklist.Push(succ)
		return
	}

	merged := existing.Intersect(current)
	if !existing.Equals(merged) {
		blockEntryStates[succ.Index] = merged
		worklist.Push(succ)
	}
}

// Perform analysis for a given function using depth first search
// to uncover every possible program path
func functionDepthFirstSearch(
	fn *ssa.Function,
	initialLockset LockSet,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if len(fn.Blocks) == 0 {
		return
	}

	entry := fn.Blocks[0]
	blockEntryStates := map[int]AnalysisState{
		entry.Index: newAnalysisState(initialLockset),
	}

	worklist := newBlockWorklist(entry)

	for !worklist.Empty() {
		curr := worklist.Pop()

		entryState := blockEntryStates[curr.Index]
		currentState := entryState.Copy()

		analyzeInstructions(fn, curr.Instrs, &currentState, registry, reporter, fset)
		if logger.IsVerbose() {
			utils.PrintSSABlock(curr)
		}

		for _, succ := range curr.Succs {
			updateSuccessorState(
				succ,
				currentState,
				blockEntryStates,
				worklist,
			)
		}
	}
}
