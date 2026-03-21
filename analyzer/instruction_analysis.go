package analyzer

import (
	"go/token"
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
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	for _, instr := range instrs {
		switch msg := instr.(type) {
		case *ssa.Call:
			handleCallInstruction(fn, msg, state, registry, reporter, fset)
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
	fset *token.FileSet) {
	if calleeFn == nil {
		return
	}
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
		handleStaticCalleeFunction(callee, msg, registry, state, reporter, fset)
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
	if isLockCallCommon(&msg.Call) {
		obj := getLockObjectFromCallCommon(&msg.Call)
		if obj != nil {
			state.DeferredLocks[obj] = true
		}
	} else if isUnlockCallCommon(&msg.Call) {
		obj := getLockObjectFromCallCommon(&msg.Call)
		if obj != nil {
			state.DeferredUnlocks[obj] = true
		}
	}
}
