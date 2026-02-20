package analyzer

import (
	"fmt"
	"go/types"
	"gotsan/ir"
	"gotsan/utils"

	"golang.org/x/tools/go/ssa"
)

// Analyze a function, recursively handling any anonymous functinos
// within it's body
func analyzeFunction(fn *ssa.Function, registry *ir.ContractRegistry) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}

	// TODO: Perform SSA/CFG analysis for this function

	fmt.Println("Function analyzed: ", fn.Name(), registry.Functions[fn.Name()])
	utils.PrintFunctionBlocks(fn)

	// Recurse through any anonymous functions
	for _, anon := range fn.AnonFuncs {
		analyzeFunction(anon, registry)
	}
}

func findMethodsForType(pkg *ssa.Package, t types.Type, registry *ir.ContractRegistry) {
	// Check methods/interface implementing a type
	methodSet := pkg.Prog.MethodSets.MethodSet(t)
	for i := range methodSet.Len() {
		selection := methodSet.At(i)
		fn := pkg.Prog.MethodValue(selection)
		if fn != nil && fn.Pkg == pkg {
			analyzeFunction(fn, registry)
		}
	}

	// 2. Check the pointer to the type
	ptrMset := pkg.Prog.MethodSets.MethodSet(types.NewPointer(t))
	for i := range ptrMset.Len() {
		if fn := pkg.Prog.MethodValue(ptrMset.At(i)); fn != nil {
			analyzeFunction(fn, registry)
		}
	}
}

func Run(pkg *ssa.Package, registry *ir.ContractRegistry) {
	for _, member := range pkg.Members {
		switch n := member.(type) {
		case *ssa.Function:
			analyzeFunction(n, registry)
		case *ssa.Type:
			// Check if the type has any methods
			findMethodsForType(pkg, n.Type(), registry)
		}
	}
}
