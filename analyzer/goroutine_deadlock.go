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

func firstRepeatedLock(order []lockRef) (lockRef, bool) {
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if sameLock(order[i], order[j]) {
				return order[i], true
			}
		}
	}

	return lockRef{}, false
}

func containsLock(order []lockRef, lock lockRef) bool {
	return indexOfLock(order, lock) != -1
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
		if a.Obj == b.Obj {
			return true
		}
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

func resolveGoCallTargets(callerFn *ssa.Function, goInstr *ssa.Go) []*ssa.Function {
	if callerFn == nil || goInstr == nil {
		return nil
	}

	targets := make([]*ssa.Function, 0, 1)
	seen := make(map[*ssa.Function]bool)

	if staticCallee := goInstr.Call.StaticCallee(); staticCallee != nil {
		targets = appendUniqueFunction(targets, staticCallee, seen)
	}

	if direct := resolveFunctionFromValue(goInstr.Call.Value); direct != nil {
		targets = appendUniqueFunction(targets, direct, seen)
	}

	if callerFn.Pkg != nil {
		if param := resolveParameterFromValue(goInstr.Call.Value); param != nil {
			for _, bound := range resolveParameterBindingTargets(callerFn, param, callerFn.Pkg) {
				targets = appendUniqueFunction(targets, bound, seen)
			}

			if goInstr.Call.Method != nil {
				for _, bound := range resolveParameterBindingMethodTargets(callerFn, param, callerFn.Pkg, goInstr.Call.Method.Name()) {
					targets = appendUniqueFunction(targets, bound, seen)
				}
			}
		}
	}

	if goInstr.Call.Method != nil {
		for _, recvType := range resolveConcreteTypesFromValue(goInstr.Call.Value) {
			for _, target := range resolveMethodTargetsForType(callerFn.Pkg, recvType, goInstr.Call.Method.Name()) {
				targets = appendUniqueFunction(targets, target, seen)
			}
		}
	}

	return targets
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
			order = append(order, lockRef{Obj: obj, Name: req.Target})
		}
	}

	for _, block := range callee.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			targets := make([]*ssa.Function, 0, 1)
			seenTargets := make(map[*ssa.Function]bool)
			if nestedCallee := callInstr.Call.StaticCallee(); nestedCallee != nil {
				targets = appendUniqueFunction(targets, nestedCallee, seenTargets)
			} else {
				for _, dynamicTarget := range resolveDynamicCallTargets(callee, callInstr) {
					targets = appendUniqueFunction(targets, dynamicTarget, seenTargets)
				}
			}

			for _, target := range targets {
				if target == nil {
					continue
				}

				nestedOrder := collectTransitiveAcquireOrder(target, callInstr.Call.Args, registry, active)
				order = append(order, nestedOrder...)
			}
		}
	}

	return order
}

func acquireOrderForGoSite(callerFn *ssa.Function, goInstr *ssa.Go, registry *ir.ContractRegistry) []lockRef {
	if callerFn == nil || goInstr == nil || registry == nil {
		return nil
	}

	targets := resolveGoCallTargets(callerFn, goInstr)
	if len(targets) == 0 {
		return nil
	}

	order := make([]lockRef, 0)
	for _, callee := range targets {
		if callee == nil {
			continue
		}
		nested := collectTransitiveAcquireOrder(callee, goInstr.Call.Args, registry, map[*ssa.Function]bool{})
		order = append(order, nested...)
	}

	return order
}

func acquireOrderForCallSite(callerFn *ssa.Function, callInstr *ssa.Call, registry *ir.ContractRegistry) []lockRef {
	if callerFn == nil || callInstr == nil || registry == nil {
		return nil
	}

	targets := make([]*ssa.Function, 0, 1)
	seenTargets := make(map[*ssa.Function]bool)
	if callee := callInstr.Call.StaticCallee(); callee != nil {
		targets = appendUniqueFunction(targets, callee, seenTargets)
	} else {
		for _, dynamicTarget := range resolveDynamicCallTargets(callerFn, callInstr) {
			targets = appendUniqueFunction(targets, dynamicTarget, seenTargets)
		}
	}

	if len(targets) == 0 {
		return nil
	}

	order := make([]lockRef, 0)
	for _, callee := range targets {
		if callee == nil {
			continue
		}
		nested := collectTransitiveAcquireOrder(callee, callInstr.Call.Args, registry, map[*ssa.Function]bool{})
		order = append(order, nested...)
	}

	return order
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
			order := acquireOrderForGoSite(fn, goInstr, registry)
			if len(order) == 0 {
				continue
			}
			if callee == nil {
				callee = resolveFunctionFromValue(goInstr.Call.Value)
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
			repeatedLockA, repeatedA := firstRepeatedLock(sites[i].Order)
			if repeatedA && containsLock(sites[j].Order, repeatedLockA) {
				reportGoroutineRecursiveLockPotentialDeadlock(
					sites[i].GoInstr,
					sites[j].GoInstr,
					sites[i].Callee,
					sites[j].Callee,
					lockDisplayName(repeatedLockA),
					reporter,
					fset,
				)
			}

			repeatedLockB, repeatedB := firstRepeatedLock(sites[j].Order)
			if repeatedB && containsLock(sites[i].Order, repeatedLockB) {
				reportGoroutineRecursiveLockPotentialDeadlock(
					sites[j].GoInstr,
					sites[i].GoInstr,
					sites[j].Callee,
					sites[i].Callee,
					lockDisplayName(repeatedLockB),
					reporter,
					fset,
				)
			}

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

			order := acquireOrderForCallSite(fn, callInstr, registry)
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

func detectPackageWideGoroutineLockOrderInversions(
	pkg *ssa.Package,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if pkg == nil || registry == nil {
		return
	}

	sites := make([]goroutineAcquireSite, 0)
	for fn := range collectPackageFunctions(pkg) {
		if fn == nil || len(fn.Blocks) == 0 {
			continue
		}

		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				goInstr, ok := instr.(*ssa.Go)
				if !ok {
					continue
				}

				order := acquireOrderForGoSite(fn, goInstr, registry)
				if len(order) == 0 {
					continue
				}

				callee := goInstr.Call.StaticCallee()
				if callee == nil {
					callee = resolveFunctionFromValue(goInstr.Call.Value)
				}

				sites = append(sites, goroutineAcquireSite{
					GoInstr: goInstr,
					Callee:  callee,
					Order:   order,
				})
			}
		}
	}

	for i := 0; i < len(sites); i++ {
		for j := i + 1; j < len(sites); j++ {
			repeatedLockA, repeatedA := firstRepeatedLock(sites[i].Order)
			if repeatedA && containsLock(sites[j].Order, repeatedLockA) {
				reportGoroutineRecursiveLockPotentialDeadlock(
					sites[i].GoInstr,
					sites[j].GoInstr,
					sites[i].Callee,
					sites[j].Callee,
					lockDisplayName(repeatedLockA),
					reporter,
					fset,
				)
			}

			repeatedLockB, repeatedB := firstRepeatedLock(sites[j].Order)
			if repeatedB && containsLock(sites[i].Order, repeatedLockB) {
				reportGoroutineRecursiveLockPotentialDeadlock(
					sites[j].GoInstr,
					sites[i].GoInstr,
					sites[j].Callee,
					sites[i].Callee,
					lockDisplayName(repeatedLockB),
					reporter,
					fset,
				)
			}

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
