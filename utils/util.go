package utils

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

func FormatPos(fset *token.FileSet, pos token.Pos) string {
	if fset == nil || pos == token.NoPos {
		return ""
	}
	p := fset.Position(pos)
	// file:line:col
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}

// Testing utility to print a basic block of a function in SSA form
func PrintFunctionBlocks(fn *ssa.Function) {
	fmt.Printf("Blocks for function: %s\n", fn.String())

	for _, block := range fn.Blocks {
		fmt.Printf("\n--- Block %d ---\n", block.Index)

		// 1. Print the instructions in the block
		for _, instr := range block.Instrs {
			fmt.Printf("  %v\t\t(%T)\n", instr, instr)
		}

		// 2. Print where this block can go next (Successors)
		if len(block.Succs) > 0 {
			fmt.Print("  Successors: ")
			for _, succ := range block.Succs {
				fmt.Printf("Block %d ", succ.Index)
			}
			fmt.Println()
		} else {
			fmt.Println("  Successors: (Exit)")
		}
	}
}
