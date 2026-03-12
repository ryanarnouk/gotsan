package analyzer

import (
	"go/token"
	"gotsan/ir"
	"gotsan/utils/logger"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

func receiverTypeName(fn *ssa.Function) string {
	if fn.Signature == nil {
		return ""
	}

	recv := fn.Signature.Recv()
	if recv == nil {
		return ""
	}

	return ir.NormalizeTypeName(recv.Type().String())
}

// Retrieve function contract from the registry
func contractForFunction(fn *ssa.Function, registry *ir.ContractRegistry) *ir.FunctionContract {
	if fn == nil {
		return nil
	}

	recv := receiverTypeName(fn)

	// If the function is a method, and has a receiver
	// retrieve it by normalizing the type name to the function name
	// Try method key first (if receiver exists)
	if recv != "" {
		if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), recv)]; c != nil {
			return c
		}
	}

	// Fallback to plain function name
	return registry.Functions[fn.Name()]
}

// Creates the initial lockset for a function, according to the Requires
// tag that is provided, and matches the function contract
func createInitialLockset(fn *ssa.Function, contract *ir.FunctionContract, reporter *report.Reporter, fset *token.FileSet) LockSet {
	// Setup initial state
	initialLockset := make(LockSet)

	if contract != nil {
		requires := contract.Expectations[ir.Requires]
		for _, expectation := range requires {
			obj := resolveObjectInScope(fn, expectation.Target)
			if obj != nil {
				initialLockset[obj] = true
				logger.Debugf("Initialized path with lock: %v", obj.Name())
			} else {
				logger.Debugf("Could not resolve @requires target '%s' in %s — reported at call sites",
					expectation.Target, fn.Name())
				reportUnresolvableAnnotation(ir.Requires.String(), expectation.Target, contract.Pos, reporter, fset)
			}
		}
	}

	return initialLockset
}
