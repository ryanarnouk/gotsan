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
		ptr, ok := t.Underlying().(*types.Pointer)
		// type is a pointer
		if ok {
			t = ptr.Elem()
			continue
		}

		// type is named
		named, ok := t.(*types.Named)
		if !ok {
			return ""
		}

		// type is part of an object (i.e., within a struct)
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
		// Recurse on the owner
		return ownerTypeNameForAddress(v.X)
	default:
		return ""
	}
}

func dataInvariantForAddress(addr ssa.Value, registry *ir.ContractRegistry) (string, *ir.DataInvariant) {
	if registry == nil {
		return "", nil
	}

	// data type is not an object
	obj := resolveValueToObject(addr)
	if obj == nil {
		return "", nil
	}

	// address data type contains an owner
	ownerType := ownerTypeNameForAddress(addr)
	if ownerType != "" {
		qualifiedKey := ownerType + "." + obj.Name()

		// Check for the data invariant in the registry
		// with the resolved name
		invariant, ok := registry.Data[qualifiedKey]
		if ok {
			return qualifiedKey, invariant
		}
	}

	// does not contain an owner, standalone
	// and not defined within a struct
	invariant, ok := registry.Data[obj.Name()]
	if ok {
		return obj.Name(), invariant
	}

	return "", nil
}

func resolveGuardLockObject(fn *ssa.Function, addr ssa.Value, mutexName string) types.Object {
	if fn == nil || mutexName == "" {
		return nil
	}

	parts := strings.Split(mutexName, ".")

	if fieldAddr, ok := addr.(*ssa.FieldAddr); ok {
		if obj := resolveValueField(fieldAddr.X, parts); obj != nil {
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
		logger.Debugf("Could not resolve @guarded_by target '%s' in %s for field %s",
			invariant.MutexName, fn.Name(), dataName)
		reportUnresolvableAnnotation("guarded_by", invariant.MutexName, invariant.Pos, reporter, fset)
		return
	}

	if !state.HeldLocks[requiredLock] {
		reportGuardViolation(instr, dataName, invariant.MutexName, reporter, fset)
	}
}
