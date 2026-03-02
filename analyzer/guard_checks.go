package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/logger"
	"gotsan/utils/report"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func typeNameFromValue(val ssa.Value) string {
	if val == nil {
		return ""
	}

	t := val.Type()
	for {
		if ptr, ok := t.Underlying().(*types.Pointer); ok {
			t = ptr.Elem()
			continue
		}

		named, ok := t.(*types.Named)
		if !ok {
			return ""
		}

		if named.Obj() != nil {
			return ir.NormalizeTypeName(named.Obj().Name())
		}
		return ""
	}
}

func ownerTypeNameForAddress(addr ssa.Value) string {
	switch v := addr.(type) {
	case *ssa.FieldAddr:
		return typeNameFromValue(v.X)
	case *ssa.IndexAddr:
		return ownerTypeNameForAddress(v.X)
	default:
		return ""
	}
}

func dataInvariantForAddress(addr ssa.Value, registry *ir.ContractRegistry) (string, *ir.DataInvariant) {
	if registry == nil {
		return "", nil
	}

	obj := traceToObject(addr)
	if obj == nil {
		return "", nil
	}

	ownerType := ownerTypeNameForAddress(addr)
	if ownerType != "" {
		qualifiedKey := ownerType + "." + obj.Name()
		if inv, ok := registry.Data[qualifiedKey]; ok {
			return qualifiedKey, inv
		}
	}

	if inv, ok := registry.Data[obj.Name()]; ok {
		return obj.Name(), inv
	}

	return "", nil
}

func resolveGuardLockObject(fn *ssa.Function, addr ssa.Value, mutexName string) types.Object {
	if fn == nil || mutexName == "" {
		return nil
	}

	parts := strings.Split(mutexName, ".")

	if fieldAddr, ok := addr.(*ssa.FieldAddr); ok {
		if obj := findFieldPathInValue(fieldAddr.X, parts); obj != nil {
			return obj
		}
	}

	return resolveObjectInScope(fn, mutexName)
}

func checkGuardedByAccess(
	instr ssa.Instruction,
	fn *ssa.Function,
	addr ssa.Value,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if state == nil || fn == nil || addr == nil {
		return
	}

	dataName, invariant := dataInvariantForAddress(addr, registry)
	if invariant == nil {
		return
	}

	requiredLock := resolveGuardLockObject(fn, addr, invariant.MutexName)
	if requiredLock == nil {
		logger.Debugf("Warning: Could not resolve guard lock '%s' in function %s for %s\n",
			invariant.MutexName, fn.Name(), dataName)
		return
	}

	if !state.HeldLocks[requiredLock] {
		reportGuardViolation(instr, dataName, invariant.MutexName, reporter, fset)
	}
}
