package analysis

import (
	"fmt"
	"go/token"
	"sort"
	"strings"
)

// Represents a specific lock invariant
// with the "Kind" mapping to an annotation function
// And the Target being the specific mutex
type Requirement struct {
	Kind   AnnotationKind
	Target string
}

// Represents concurrency invariants for specific function
type FunctionContract struct {
	Expectations []Requirement
	Pos          token.Pos
}

// Represents data field guarded by a mutex
type FieldGuard struct {
	MutexName string
	Pos       token.Pos
}

// Represents all concurrency contracts in a program
// Populated by AST Visitor and then consumed by the
// SSA/CFG Analyzer to verify lock patterns
type ContractRegistry struct {
	Functions map[string]*FunctionContract
	Guards    map[string]*FieldGuard
}

func NewContractRegistry() *ContractRegistry {
	return &ContractRegistry{
		Functions: make(map[string]*FunctionContract),
		Guards:    make(map[string]*FieldGuard),
	}
}

func (cr *ContractRegistry) PrintContractRegistry(fset *token.FileSet) {
	if cr == nil {
		fmt.Println("<nil ContractRegistry>")
		return
	}

	fmt.Println("=== Contract Registry ===")

	// -------- Functions --------
	fmt.Println("\n-- Functions --")

	if len(cr.Functions) == 0 {
		fmt.Println("(none)")
	} else {
		fnNames := make([]string, 0, len(cr.Functions))
		for name := range cr.Functions {
			fnNames = append(fnNames, name)
		}
		sort.Strings(fnNames)

		for _, fn := range fnNames {
			fc := cr.Functions[fn]
			if fc == nil {
				fmt.Printf("%s: <nil>\n", fn)
				continue
			}

			posStr := formatPos(fset, fc.Pos)
			if posStr != "" {
				fmt.Printf("%s @ %s\n", fn, posStr)
			} else {
				fmt.Printf("%s\n", fn)
			}

			if len(fc.Expectations) == 0 {
				fmt.Println("  (no expectations)")
				continue
			}

			for _, req := range fc.Expectations {
				fmt.Printf("  - %s\n", req.Kind.String())
			}
		}
	}

	// -------- Guards --------
	fmt.Println("\n-- Field Guards --")

	if len(cr.Guards) == 0 {
		fmt.Println("(none)")
	} else {
		fieldNames := make([]string, 0, len(cr.Guards))
		for name := range cr.Guards {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)

		for _, field := range fieldNames {
			g := cr.Guards[field]
			if g == nil {
				fmt.Printf("%s: <nil>\n", field)
				continue
			}

			posStr := formatPos(fset, g.Pos)
			if posStr != "" {
				fmt.Printf("%s guarded by %s @ %s\n", field, g.MutexName, posStr)
			} else {
				fmt.Printf("%s guarded by %s\n", field, g.MutexName)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 26))
}

func formatPos(fset *token.FileSet, pos token.Pos) string {
	if fset == nil || pos == token.NoPos {
		return ""
	}
	p := fset.Position(pos)
	// file:line:col
	return fmt.Sprintf("%s:%d:%d", p.Filename, p.Line, p.Column)
}
