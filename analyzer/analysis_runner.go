package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/logger"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

// Analyze a function, recursively handling any anonymous functions
// within it's body
func analyzeFunction(
	fn *ssa.Function,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}

	// Setup initial state
	contract := contractForFunction(fn, registry)
	initialLockset := createInitialLockset(fn, contract)

	logger.Debugf("Function analyzed: %s %v", fn.Name(), contract)
	// Begin DFS through function
	functionDepthFirstSearch(fn, initialLockset, registry, reporter, fset)

	// Recurse through any anonymous functions
	for _, anon := range fn.AnonFuncs {
		analyzeFunction(anon, registry, reporter, fset)
	}
}

// Find methods in a program (i.e., functions part of an interface)
// Example: func (example e) function() { ... }
// where function a method of type "example"
func findMethodsForType(
	pkg *ssa.Package,
	t types.Type,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	// Check methods/interface implementing a type
	methodSet := pkg.Prog.MethodSets.MethodSet(t)
	for i := range methodSet.Len() {
		selection := methodSet.At(i)
		fn := pkg.Prog.MethodValue(selection)
		if fn != nil && fn.Pkg == pkg {
			analyzeFunction(fn, registry, reporter, fset)
		}
	}

	// Check the pointer to the type
	ptrMset := pkg.Prog.MethodSets.MethodSet(types.NewPointer(t))
	for i := range ptrMset.Len() {
		if fn := pkg.Prog.MethodValue(ptrMset.At(i)); fn != nil {
			analyzeFunction(fn, registry, reporter, fset)
		}
	}
}

func Run(pkg *ssa.Package, registry *ir.ContractRegistry, reporter *report.Reporter, fset *token.FileSet) {
	for _, member := range pkg.Members {
		switch n := member.(type) {
		case *ssa.Function:
			analyzeFunction(n, registry, reporter, fset)
		case *ssa.Type:
			// Check if the type has any methods
			// This appears when using an interface
			findMethodsForType(pkg, n.Type(), registry, reporter, fset)
		}
	}
}
