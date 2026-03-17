package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

type lockRef struct {
	Obj  types.Object
	Name string
}

type goroutineAcquireSite struct {
	GoInstr *ssa.Go
	Callee  *ssa.Function
	Order   []lockRef
}

type functionCallSite struct {
	CallInstr *ssa.Call
	Callee    *ssa.Function
	Order     []lockRef
}

func lockDisplayName(l lockRef) string {
	if l.Obj != nil {
		return l.Obj.Name()
	}
	if l.Name != "" {
		return l.Name
	}
	return "<unknown lock>"
}

func sameLock(a lockRef, b lockRef) bool {
	if a.Obj != nil && b.Obj != nil {
		return a.Obj == b.Obj
	}
	if a.Name != "" && b.Name != "" {
		return a.Name == b.Name
	}
	return false
}

func indexOfLock(order []lockRef, lock lockRef) int {
	for i, curr := range order {
		if sameLock(curr, lock) {
			return i
		}
	}
	return -1
}

func findOrderInversion(a []lockRef, b []lockRef) (lockRef, lockRef, bool) {
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			first := a[i]
			second := a[j]
			if sameLock(first, second) {
				continue
			}

			posFirst := indexOfLock(b, first)
			posSecond := indexOfLock(b, second)
			if posFirst == -1 || posSecond == -1 {
				continue
			}

			if posSecond < posFirst {
				return first, second, true
			}
		}
	}

	return lockRef{}, lockRef{}, false
}

func acquireOrderForGoCall(goInstr *ssa.Go, callee *ssa.Function, contract *ir.FunctionContract) []lockRef {
	if goInstr == nil || callee == nil || contract == nil {
		return nil
	}

	acquires := contract.Expectations[ir.Acquires]
	if len(acquires) == 0 {
		return nil
	}

	order := make([]lockRef, 0, len(acquires))
	for _, req := range acquires {
		obj := resolveObjectAtInvocation(callee, goInstr.Call.Args, req.Target)
		order = append(order, lockRef{Obj: obj, Name: req.Target})
	}

	return order
}

func acquireOrderForCall(callInstr *ssa.Call, callee *ssa.Function, contract *ir.FunctionContract) []lockRef {
	if callInstr == nil || callee == nil || contract == nil {
		return nil
	}

	acquires := contract.Expectations[ir.Acquires]
	if len(acquires) == 0 {
		return nil
	}

	order := make([]lockRef, 0, len(acquires))
	for _, req := range acquires {
		obj := resolveObjectAtInvocation(callee, callInstr.Call.Args, req.Target)
		order = append(order, lockRef{Obj: obj, Name: req.Target})
	}

	return order
}

func appendLockIfMissing(order []lockRef, lock lockRef) []lockRef {
	for _, existing := range order {
		if sameLock(existing, lock) {
			return order
		}
	}

	return append(order, lock)
}

func collectTransitiveAcquireOrder(
	callee *ssa.Function,
	invocationArgs []ssa.Value,
	registry *ir.ContractRegistry,
	active map[*ssa.Function]bool,
) []lockRef {
	if callee == nil || registry == nil {
		return nil
	}

	if active[callee] {
		// Break cycles while preserving lock order discovered so far.
		return nil
	}

	active[callee] = true
	defer delete(active, callee)

	order := make([]lockRef, 0)

	contract := contractForFunction(callee, registry)
	if contract != nil {
		acquires := contract.Expectations[ir.Acquires]
		for _, req := range acquires {
			obj := resolveObjectAtInvocation(callee, invocationArgs, req.Target)
			order = appendLockIfMissing(order, lockRef{Obj: obj, Name: req.Target})
		}
	}

	for _, block := range callee.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			nestedCallee := callInstr.Call.StaticCallee()
			if nestedCallee == nil {
				continue
			}

			nestedOrder := collectTransitiveAcquireOrder(nestedCallee, callInstr.Call.Args, registry, active)
			for _, lock := range nestedOrder {
				order = appendLockIfMissing(order, lock)
			}
		}
	}

	return order
}

func acquireOrderForGoSite(goInstr *ssa.Go, registry *ir.ContractRegistry) []lockRef {
	if goInstr == nil || registry == nil {
		return nil
	}

	callee := goInstr.Call.StaticCallee()
	if callee == nil {
		return nil
	}

	return collectTransitiveAcquireOrder(callee, goInstr.Call.Args, registry, map[*ssa.Function]bool{})
}

func acquireOrderForCallSite(callInstr *ssa.Call, registry *ir.ContractRegistry) []lockRef {
	if callInstr == nil || registry == nil {
		return nil
	}

	callee := callInstr.Call.StaticCallee()
	if callee == nil {
		return nil
	}

	return collectTransitiveAcquireOrder(callee, callInstr.Call.Args, registry, map[*ssa.Function]bool{})
}

func detectGoroutineLockOrderInversions(
	fn *ssa.Function,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if fn == nil || len(fn.Blocks) == 0 || registry == nil {
		return
	}

	sites := make([]goroutineAcquireSite, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			goInstr, ok := instr.(*ssa.Go)
			if !ok {
				continue
			}

			callee := goInstr.Call.StaticCallee()
			if callee == nil {
				continue
			}

			order := acquireOrderForGoSite(goInstr, registry)
			if len(order) < 2 {
				continue
			}

			sites = append(sites, goroutineAcquireSite{
				GoInstr: goInstr,
				Callee:  callee,
				Order:   order,
			})
		}
	}

	for i := 0; i < len(sites); i++ {
		for j := i + 1; j < len(sites); j++ {
			firstLock, secondLock, found := findOrderInversion(sites[i].Order, sites[j].Order)
			if !found {
				continue
			}

			reportGoroutineLockOrderInversion(
				sites[i].GoInstr,
				sites[j].GoInstr,
				sites[i].Callee,
				sites[j].Callee,
				lockDisplayName(firstLock),
				lockDisplayName(secondLock),
				reporter,
				fset,
			)
		}
	}
}

func detectSingleThreadedLockOrderInversions(
	fn *ssa.Function,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if fn == nil || len(fn.Blocks) == 0 || registry == nil {
		return
	}

	sites := make([]functionCallSite, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			callee := callInstr.Call.StaticCallee()
			if callee == nil {
				continue
			}

			order := acquireOrderForCallSite(callInstr, registry)
			if len(order) < 2 {
				continue
			}

			sites = append(sites, functionCallSite{
				CallInstr: callInstr,
				Callee:    callee,
				Order:     order,
			})
		}
	}

	for i := 0; i < len(sites); i++ {
		for j := i + 1; j < len(sites); j++ {
			firstLock, secondLock, found := findOrderInversion(sites[i].Order, sites[j].Order)
			if !found {
				continue
			}

			reportSingleThreadedLockOrderInversion(
				sites[i].CallInstr,
				sites[j].CallInstr,
				sites[i].Callee,
				sites[j].Callee,
				lockDisplayName(firstLock),
				lockDisplayName(secondLock),
				reporter,
				fset,
			)
		}
	}
}
