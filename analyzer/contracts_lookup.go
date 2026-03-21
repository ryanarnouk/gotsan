package analyzer

import (
	"go/token"
	"gotsan/ir"
	"gotsan/utils/logger"
	"gotsan/utils/report"
	"strings"

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
	if fn == nil || registry == nil {
		return nil
	}

	if fn.Pos() != token.NoPos {
		if c := registry.FunctionsByPos[fn.Pos()]; c != nil {
			return c
		}
	}

	recv := receiverTypeName(fn)

	// If the function is a method, and has a receiver
	// retrieve it by normalizing the type name to the function name
	// Try method key first (if receiver exists)
	if recv != "" {
		if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), recv)]; c != nil {
			return c
		}

		if strings.HasPrefix(recv, "*") {
			if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), strings.TrimPrefix(recv, "*"))]; c != nil {
				return c
			}
		} else {
			if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), "*"+recv)]; c != nil {
				return c
			}
		}
	}

	// Some SSA variants lose Signature.Recv, but still carry receiver-like first
	// parameter (e.g., c *tableNameCache). Try that before plain-name fallback.
	if recv == "" && len(fn.Params) > 0 {
		paramRecv := ir.NormalizeTypeName(fn.Params[0].Type().String())
		if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), paramRecv)]; c != nil {
			return c
		}

		if strings.HasPrefix(paramRecv, "*") {
			if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), strings.TrimPrefix(paramRecv, "*"))]; c != nil {
				return c
			}
		} else {
			if c := registry.Functions[ir.MakeFunctionKey(fn.Name(), "*"+paramRecv)]; c != nil {
				return c
			}
		}
	}

	// Last chance: disambiguate same-name contracts (e.g., remove methods on
	// different receivers) by preferring expectations resolvable in this function.
	if c := bestResolvableSameNameContract(fn, registry); c != nil {
		return c
	}

	// Final fallback to plain function name.
	return registry.Functions[fn.Name()]
}

func expectationResolvableScore(fn *ssa.Function, c *ir.FunctionContract) int {
	if fn == nil || c == nil {
		return 0
	}

	score := 0
	kinds := []ir.AnnotationKind{ir.Requires, ir.Acquires, ir.Returns}
	for _, kind := range kinds {
		for _, req := range c.Expectations[kind] {
			if resolveObjectInScope(fn, req.Target) != nil {
				score++
			}
		}
	}

	return score
}

func bestResolvableSameNameContract(fn *ssa.Function, registry *ir.ContractRegistry) *ir.FunctionContract {
	if fn == nil || registry == nil {
		return nil
	}

	suffix := "." + fn.Name()
	var best *ir.FunctionContract
	bestScore := 0

	for key, c := range registry.Functions {
		if c == nil {
			continue
		}

		if key != fn.Name() && !strings.HasSuffix(key, suffix) {
			continue
		}

		score := expectationResolvableScore(fn, c)
		if score > bestScore {
			best = c
			bestScore = score
		}
	}

	if bestScore > 0 {
		return best
	}

	return nil
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
