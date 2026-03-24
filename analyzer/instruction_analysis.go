package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

// Analyze the instructions of a given block, updating lock/defer state in accordance with SSA side effects.
func analyzeInstructions(
	fn *ssa.Function,
	instrs []ssa.Instruction,
	contract *ir.FunctionContract,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	recursion *recursionGraph,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	for _, instr := range instrs {
		switch msg := instr.(type) {
		case *ssa.Call:
			handleCallInstruction(fn, msg, state, registry, recursion, reporter, fset)
		case *ssa.Defer:
			registerDeferInstruction(msg, state)
		case *ssa.RunDefers:
			applyDeferredEffects(state)
		case *ssa.UnOp:
			// Dereference (MUL referring to a * in a pointer dereference access)
			// Will become a pointer in SSA addressable memory accesses (i.e., shared memory constructs)
			if msg.Op == token.MUL {
				checkGuardedByAccess(msg, fn, msg.X, state, registry, reporter, fset)
			}
		case *ssa.Store:
			// Store
			checkGuardedByAccess(msg, fn, msg.Addr, state, registry, reporter, fset)
		case *ssa.Return:
			checkReturnPath(fn, msg, contract, state, reporter, fset)
		}
	}
}

func mergeLockSet(dst LockSet, src LockSet) {
	for obj := range src {
		dst[obj] = true
	}
}

func resolveFunctionFromValue(v ssa.Value) *ssa.Function {
	switch n := v.(type) {
	case *ssa.Function:
		return n
	case *ssa.MakeClosure:
		if n.Fn == nil {
			return nil
		}
		fn, _ := n.Fn.(*ssa.Function)
		return fn
	case *ssa.ChangeType:
		return resolveFunctionFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveFunctionFromValue(n.X)
	case *ssa.UnOp:
		return resolveFunctionFromValue(n.X)
	case *ssa.Alloc:
		refs := n.Referrers()
		if refs == nil {
			return nil
		}
		for _, ref := range *refs {
			store, ok := ref.(*ssa.Store)
			if !ok {
				continue
			}
			if fn := resolveFunctionFromValue(store.Val); fn != nil {
				return fn
			}
		}
	}

	return nil
}

func resolveParameterFromValue(v ssa.Value) *ssa.Parameter {
	if v == nil {
		return nil
	}

	switch n := v.(type) {
	case *ssa.Parameter:
		return n
	case *ssa.ChangeType:
		return resolveParameterFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveParameterFromValue(n.X)
	case *ssa.UnOp:
		return resolveParameterFromValue(n.X)
	default:
		return nil
	}
}

func resolveConcreteTypesFromValue(v ssa.Value) []types.Type {
	if v == nil {
		return nil
	}

	switch n := v.(type) {
	case *ssa.MakeInterface:
		if n.X == nil {
			return nil
		}
		return []types.Type{n.X.Type()}
	case *ssa.Call:
		callee := n.Call.StaticCallee()
		if callee == nil {
			return nil
		}
		return inferConcreteReturnTypes(callee)
	case *ssa.ChangeType:
		return resolveConcreteTypesFromValue(n.X)
	case *ssa.ChangeInterface:
		return resolveConcreteTypesFromValue(n.X)
	case *ssa.UnOp:
		return resolveConcreteTypesFromValue(n.X)
	default:
		if t := v.Type(); t != nil {
			return []types.Type{t}
		}
	}

	return nil
}

func inferConcreteReturnTypes(fn *ssa.Function) []types.Type {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}

	out := make([]types.Type, 0)
	seen := make(map[string]bool)

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}

			for _, result := range ret.Results {
				for _, t := range resolveConcreteTypesFromValue(result) {
					if t == nil {
						continue
					}
					key := t.String()
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, t)
				}
			}
		}
	}

	return out
}

func resolveMethodTargetsForType(pkg *ssa.Package, recvType types.Type, methodName string) []*ssa.Function {
	if pkg == nil || recvType == nil || methodName == "" {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	addFromMethodSet := func(t types.Type) {
		if t == nil {
			return
		}

		methodSet := pkg.Prog.MethodSets.MethodSet(t)
		for i := 0; i < methodSet.Len(); i++ {
			sel := methodSet.At(i)
			if sel == nil || sel.Obj() == nil || sel.Obj().Name() != methodName {
				continue
			}

			fn := pkg.Prog.MethodValue(sel)
			if fn == nil || seen[fn] {
				continue
			}

			seen[fn] = true
			out = append(out, fn)
		}
	}

	addFromMethodSet(recvType)
	if _, ok := recvType.Underlying().(*types.Pointer); !ok {
		addFromMethodSet(types.NewPointer(recvType))
	}

	return out
}

func appendUniqueFunction(targets []*ssa.Function, fn *ssa.Function, seen map[*ssa.Function]bool) []*ssa.Function {
	if fn == nil || seen[fn] {
		return targets
	}
	seen[fn] = true
	return append(targets, fn)
}

func resolveDynamicCallTargets(callerFn *ssa.Function, msg *ssa.Call) []*ssa.Function {
	if callerFn == nil || msg == nil {
		return nil
	}

	targets := make([]*ssa.Function, 0)
	seen := make(map[*ssa.Function]bool)

	if direct := resolveFunctionFromValue(msg.Call.Value); direct != nil {
		targets = appendUniqueFunction(targets, direct, seen)
	}

	if msg.Call.Method != nil {
		for _, recvType := range resolveConcreteTypesFromValue(msg.Call.Value) {
			for _, target := range resolveMethodTargetsForType(callerFn.Pkg, recvType, msg.Call.Method.Name()) {
				targets = appendUniqueFunction(targets, target, seen)
			}
		}
	}

	unop, ok := msg.Call.Value.(*ssa.UnOp)
	if !ok {
		return targets
	}

	fieldAddr, ok := unop.X.(*ssa.FieldAddr)
	if !ok || callerFn.Pkg == nil {
		return targets
	}

	for fn := range collectPackageFunctions(callerFn.Pkg) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				store, ok := instr.(*ssa.Store)
				if !ok {
					continue
				}

				storeFieldAddr, ok := store.Addr.(*ssa.FieldAddr)
				if !ok {
					continue
				}

				if storeFieldAddr.Field != fieldAddr.Field {
					continue
				}

				if storeFieldAddr.X.Type().String() != fieldAddr.X.Type().String() {
					continue
				}

				target := resolveFunctionFromValue(store.Val)
				targets = appendUniqueFunction(targets, target, seen)

				if target == nil {
					param := resolveParameterFromValue(store.Val)
					if param != nil {
						for _, bound := range resolveParameterBindingTargets(fn, param, callerFn.Pkg) {
							targets = appendUniqueFunction(targets, bound, seen)
						}

						if msg.Call.Method != nil {
							for _, bound := range resolveParameterBindingMethodTargets(fn, param, callerFn.Pkg, msg.Call.Method.Name()) {
								targets = appendUniqueFunction(targets, bound, seen)
							}
						}
					}
				}
			}
		}
	}

	return targets
}

func resolveParameterBindingTargets(fn *ssa.Function, param *ssa.Parameter, pkg *ssa.Package) []*ssa.Function {
	if fn == nil || param == nil || pkg == nil {
		return nil
	}

	idx := -1
	for i, p := range fn.Params {
		if p == param {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	for caller := range collectPackageFunctions(pkg) {
		for _, block := range caller.Blocks {
			for _, instr := range block.Instrs {
				callInstr, ok := instr.(*ssa.Call)
				if !ok {
					continue
				}

				if callInstr.Call.StaticCallee() != fn {
					continue
				}

				if idx >= len(callInstr.Call.Args) {
					continue
				}

				target := resolveFunctionFromValue(callInstr.Call.Args[idx])
				if target == nil || seen[target] {
					continue
				}

				seen[target] = true
				out = append(out, target)
			}
		}
	}

	return out
}

func resolveParameterBindingMethodTargets(
	fn *ssa.Function,
	param *ssa.Parameter,
	pkg *ssa.Package,
	methodName string,
) []*ssa.Function {
	if fn == nil || param == nil || pkg == nil || methodName == "" {
		return nil
	}

	idx := -1
	for i, p := range fn.Params {
		if p == param {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}

	seen := make(map[*ssa.Function]bool)
	out := make([]*ssa.Function, 0)

	for caller := range collectPackageFunctions(pkg) {
		for _, block := range caller.Blocks {
			for _, instr := range block.Instrs {
				callInstr, ok := instr.(*ssa.Call)
				if !ok {
					continue
				}

				if callInstr.Call.StaticCallee() != fn {
					continue
				}

				if idx >= len(callInstr.Call.Args) {
					continue
				}

				arg := callInstr.Call.Args[idx]
				for _, recvType := range resolveConcreteTypesFromValue(arg) {
					for _, target := range resolveMethodTargetsForType(pkg, recvType, methodName) {
						if target == nil || seen[target] {
							continue
						}

						seen[target] = true
						out = append(out, target)
					}
				}
			}
		}
	}

	return out
}

func collectFunctionLockEffects(fn *ssa.Function, seen map[*ssa.Function]bool) (LockSet, LockSet) {
	locks := make(LockSet)
	unlocks := make(LockSet)
	if fn == nil {
		return locks, unlocks
	}

	if seen[fn] {
		return locks, unlocks
	}
	seen[fn] = true

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			if isLockCall(callInstr) {
				if obj := getLockObject(callInstr); obj != nil {
					locks[obj] = true
				}
				continue
			}

			if isUnlockCall(callInstr) {
				if obj := getLockObject(callInstr); obj != nil {
					unlocks[obj] = true
				}
				continue
			}

			if callee := callInstr.Call.StaticCallee(); callee != nil {
				nestedLocks, nestedUnlocks := collectFunctionLockEffects(callee, seen)
				mergeLockSet(locks, nestedLocks)
				mergeLockSet(unlocks, nestedUnlocks)
				continue
			}

			if nested := resolveFunctionFromValue(callInstr.Call.Value); nested != nil {
				nestedLocks, nestedUnlocks := collectFunctionLockEffects(nested, seen)
				mergeLockSet(locks, nestedLocks)
				mergeLockSet(unlocks, nestedUnlocks)
				continue
			}

			for _, target := range resolveDynamicCallTargets(fn, callInstr) {
				if target == nil {
					continue
				}
				nestedLocks, nestedUnlocks := collectFunctionLockEffects(target, seen)
				mergeLockSet(locks, nestedLocks)
				mergeLockSet(unlocks, nestedUnlocks)
			}
		}
	}

	return locks, unlocks
}

func collectDeferredCallLockEffects(common *ssa.CallCommon) (LockSet, LockSet) {
	locks := make(LockSet)
	unlocks := make(LockSet)
	if common == nil {
		return locks, unlocks
	}

	if isLockCallCommon(common) {
		if obj := getLockObjectFromCallCommon(common); obj != nil {
			locks[obj] = true
		}
		return locks, unlocks
	}

	if isUnlockCallCommon(common) {
		if obj := getLockObjectFromCallCommon(common); obj != nil {
			unlocks[obj] = true
		}
		return locks, unlocks
	}

	seen := make(map[*ssa.Function]bool)

	if callee := common.StaticCallee(); callee != nil {
		nestedLocks, nestedUnlocks := collectFunctionLockEffects(callee, seen)
		mergeLockSet(locks, nestedLocks)
		mergeLockSet(unlocks, nestedUnlocks)
	}

	if dynamic := resolveFunctionFromValue(common.Value); dynamic != nil {
		nestedLocks, nestedUnlocks := collectFunctionLockEffects(dynamic, seen)
		mergeLockSet(locks, nestedLocks)
		mergeLockSet(unlocks, nestedUnlocks)
	}

	if closure, ok := common.Value.(*ssa.MakeClosure); ok {
		for _, binding := range closure.Bindings {
			boundFn := resolveFunctionFromValue(binding)
			if boundFn == nil {
				continue
			}
			nestedLocks, nestedUnlocks := collectFunctionLockEffects(boundFn, seen)
			mergeLockSet(locks, nestedLocks)
			mergeLockSet(unlocks, nestedUnlocks)
		}
	}

	return locks, unlocks
}

func isHeldLockEquivalent(heldLocks LockSet, candidate types.Object) bool {
	if candidate == nil || len(heldLocks) == 0 {
		return false
	}

	if heldLocks[candidate] {
		return true
	}

	for heldObj := range heldLocks {
		if heldObj == nil {
			continue
		}
		if heldObj.Name() == candidate.Name() {
			return true
		}
	}

	return false
}

func checkReturnPath(
	fn *ssa.Function,
	ret *ssa.Return,
	contract *ir.FunctionContract,
	state *AnalysisState,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if contract != nil {
		returns := contract.Expectations[ir.Returns]
		if len(returns) > 0 {
			for _, exp := range returns {
				checkReturnsExpectation(fn, ret, state, reporter, fset, exp, contract.Pos)
			}
			return
		}
	}

	if len(state.HeldLocks) > 0 {
		reportUndeclaredReturnedLock(fn, ret, state.HeldLocks, reporter, fset)
	}
}

func checkReturnsExpectation(
	fn *ssa.Function,
	ret *ssa.Return,
	state *AnalysisState,
	reporter *report.Reporter,
	fset *token.FileSet,
	exp ir.Requirement,
	contractPos token.Pos,
) {
	requiredLockObject := resolveObjectInScope(fn, exp.Target)
	if requiredLockObject == nil {
		reportUnresolvableAnnotation(ir.Returns.String(), exp.Target, contractPos, reporter, fset)
		return
	}

	if !state.HeldLocks[requiredLockObject] {
		reportReturnMissingLock(fn, ret, exp.Target, reporter, fset)
	}
}

func checkRequiresExpectation(exp ir.Requirement, calleeFn *ssa.Function, callSite *ssa.Call, state *AnalysisState, reporter *report.Reporter,
	fset *token.FileSet) {
	// Map the requirement to the caller's objects
	// Turn the mutex name in the annotation to an SSA object
	requiredLockObject := resolveObjectAtCallSite(callSite, exp.Target)
	if requiredLockObject == nil {
		if annotationRootIsCallsiteLocal(calleeFn, exp.Target) {
			reportCallsiteLocalRootAnnotation(ir.Requires.String(), exp.Target, calleeFn, callSite.Pos(), reporter, fset)
			return
		}

		reportUnresolvableAnnotation(ir.Requires.String(), exp.Target, callSite.Pos(), reporter, fset)
		return
	}

	if !state.HeldLocks[requiredLockObject] {
		reportMissingLock(callSite, calleeFn, exp.Target, reporter, fset)
	}
}

func checkAcquiresExpectation(exp ir.Requirement, calleeFn *ssa.Function, callSite *ssa.Call, state *AnalysisState, reporter *report.Reporter,
	fset *token.FileSet) {
	// Map the requirement to the caller's objects
	// Turn the mutex name in the annotation to an SSA object
	acquiredLockObject := resolveObjectAtCallSite(callSite, exp.Target)
	if acquiredLockObject == nil {
		if annotationRootIsCallsiteLocal(calleeFn, exp.Target) {
			reportCallsiteLocalRootAnnotation(ir.Acquires.String(), exp.Target, calleeFn, callSite.Pos(), reporter, fset)
			return
		}

		reportUnresolvableAnnotation(ir.Acquires.String(), exp.Target, callSite.Pos(), reporter, fset)
		return
	}

	if state.HeldLocks[acquiredLockObject] {
		reportAlreadyAcquiredLock(callSite, calleeFn, exp.Target, reporter, fset)
	}
}

// For a new function that is called, retrieve the contract
// and verify that all expectations are met with respect to
// the current lockset
// fn is the callee function, this function is invoked from the caller
func handleStaticCalleeFunction(calleeFn *ssa.Function, callSite *ssa.Call, registry *ir.ContractRegistry, state *AnalysisState, reporter *report.Reporter,
	recursion *recursionGraph, fset *token.FileSet, callerFn *ssa.Function) {
	if calleeFn == nil {
		return
	}

	checkRecursiveCallLockReacquireHeuristic(callerFn, calleeFn, callSite, state, registry, recursion, reporter, fset)

	contract := contractForFunction(calleeFn, registry)
	if contract == nil {
		return
	}

	requires := contract.Expectations[ir.Requires]
	for _, exp := range requires {
		checkRequiresExpectation(exp, calleeFn, callSite, state, reporter, fset)
	}

	acquires := contract.Expectations[ir.Acquires]
	for _, exp := range acquires {
		checkAcquiresExpectation(exp, calleeFn, callSite, state, reporter, fset)
	}
}

func handleCallInstruction(
	fn *ssa.Function,
	msg *ssa.Call,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	recursion *recursionGraph,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if isLockCall(msg) {
		obj := getLockObject(msg)
		if obj != nil {
			if state.MayHeldLocks[obj] {
				reportReacquiredLock(msg, fn, obj.Name(), reporter, fset)
			}
			state.HeldLocks[obj] = true
			state.MayHeldLocks[obj] = true
		}
	} else if isUnlockCall(msg) {
		obj := getLockObject(msg)
		if obj != nil {
			delete(state.HeldLocks, obj)
			delete(state.MayHeldLocks, obj)
		}
	} else {
		callee := msg.Call.StaticCallee()
		if callee != nil {
			handleStaticCalleeFunction(callee, msg, registry, state, reporter, recursion, fset, fn)

			if len(state.HeldLocks) > 0 {
				acquiredLocks, _ := collectFunctionLockEffects(callee, make(map[*ssa.Function]bool))
				for obj := range acquiredLocks {
					if obj == nil || !isHeldLockEquivalent(state.HeldLocks, obj) {
						continue
					}
					reportAlreadyAcquiredLock(msg, callee, obj.Name(), reporter, fset)
				}
			}

			return
		}

		targets := resolveDynamicCallTargets(fn, msg)
		if len(targets) == 0 {
			reportDynamicCallbackWhileHoldingLocks(msg, fn, state.HeldLocks, reporter, fset)
			return
		}

		reportedReacquire := false
		for _, target := range targets {
			if target == nil {
				continue
			}

			handleStaticCalleeFunction(target, msg, registry, state, reporter, recursion, fset, fn)

			acquiredLocks, _ := collectFunctionLockEffects(target, make(map[*ssa.Function]bool))
			for obj := range acquiredLocks {
				if obj == nil || !isHeldLockEquivalent(state.HeldLocks, obj) {
					continue
				}

				reportAlreadyAcquiredLock(msg, target, obj.Name(), reporter, fset)
				reportedReacquire = true
			}
		}

		if !reportedReacquire {
			reportDynamicCallbackWhileHoldingLocks(msg, fn, state.HeldLocks, reporter, fset)
		}
	}
}

// DEFER STATEMENT HELPERS
// ---------------------------------------------------------------------------

// Run at the end of the function to handle any state modifications
// made in the "defer" keyword, seen earlier in the function
func applyDeferredEffects(state *AnalysisState) {
	// Add any locks that were deferred to the lockset
	for obj := range state.DeferredLocks {
		state.HeldLocks[obj] = true
		state.MayHeldLocks[obj] = true
	}

	// Remove any locks from the lockset that were unlocked in a defer step
	for obj := range state.DeferredUnlocks {
		delete(state.HeldLocks, obj)
		delete(state.MayHeldLocks, obj)
	}
	state.DeferredLocks = make(LockSet)
	state.DeferredUnlocks = make(LockSet)
}

// Add deferred statements to the state, such that they are later run when the function
// is being returned (or when ssa.RunDefers exists in the SSA)
func registerDeferInstruction(msg *ssa.Defer, state *AnalysisState) {
	deferredLocks, deferredUnlocks := collectDeferredCallLockEffects(&msg.Call)
	for obj := range deferredLocks {
		state.DeferredLocks[obj] = true
	}
	for obj := range deferredUnlocks {
		state.DeferredUnlocks[obj] = true
	}
}
