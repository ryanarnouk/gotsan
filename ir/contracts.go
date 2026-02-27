package ir

import (
	"fmt"
	"go/token"
	"gotsan/utils"
	"sort"
	"strings"
	"unicode"
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

// Represents data field (within a struct) or a variable guarded by a mutex
type DataInvariant struct {
	MutexName string
	Pos       token.Pos
}

// Represents all concurrency contracts in a program
// Populated by AST Visitor and then consumed by the
// SSA/CFG Analyzer to verify lock patterns
type ContractRegistry struct {
	Functions map[string]*FunctionContract
	Data      map[string]*DataInvariant
}

func NewContractRegistry() *ContractRegistry {
	return &ContractRegistry{
		Functions: make(map[string]*FunctionContract),
		Data:      make(map[string]*DataInvariant),
	}
}

func MakeFunctionKey(name string, receiverType string) string {
	if receiverType == "" {
		return name
	}
	return receiverType + "." + name
}

func NormalizeTypeName(typeName string) string {
	if typeName == "" {
		return ""
	}

	var out strings.Builder
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		t := token.String()
		if strings.Contains(t, ".") {
			t = t[strings.LastIndex(t, ".")+1:]
		}
		out.WriteString(t)
		token.Reset()
	}

	for _, r := range typeName {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' {
			token.WriteRune(r)
			continue
		}

		flush()
		out.WriteRune(r)
	}
	flush()

	return out.String()
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

			posStr := utils.FormatPos(fset, fc.Pos)
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

	// -------- Data Invariants --------
	fmt.Println("\n-- Data Invariants/Guards --")

	if len(cr.Data) == 0 {
		fmt.Println("(none)")
	} else {
		fieldNames := make([]string, 0, len(cr.Data))
		for name := range cr.Data {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)

		for _, field := range fieldNames {
			g := cr.Data[field]
			if g == nil {
				fmt.Printf("%s: <nil>\n", field)
				continue
			}

			posStr := utils.FormatPos(fset, g.Pos)
			if posStr != "" {
				fmt.Printf("%s guarded by %s @ %s\n", field, g.MutexName, posStr)
			} else {
				fmt.Printf("%s guarded by %s\n", field, g.MutexName)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 26))
}
