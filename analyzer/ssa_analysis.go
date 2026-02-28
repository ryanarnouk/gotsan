package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils"
	"gotsan/utils/logger"
	"gotsan/utils/report"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func reportMissingLock(
	msg *ssa.Call,
	callee *ssa.Function,
	target string,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if reporter == nil || fset == nil {
		logger.Infof("ERROR: Call to %s requires lock %s, but it's not held\n", callee.Name(), target)
		return
	}

	position := fset.Position(msg.Pos())
	reporter.Warn(report.Diagnostic{
		Pos:      msg.Pos(),
		File:     position.Filename,
		Line:     position.Line,
		Column:   position.Column,
		Severity: "warning",
		Message:  "Call to " + callee.Name() + " requires lock " + target + ", but it's not held",
	})
}

func isLockCallCommon(common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}

	fn := common.StaticCallee()
	if fn == nil {
		return false // It's a dynamic call (interface or func variable)
	}

	name := fn.Name()
	if name != "Lock" && name != "RLock" {
		return false
	}

	if fn.Pkg != nil && fn.Pkg.Pkg.Path() == "sync" {
		return true
	}

	// Fallback: Check the string representation (e.g., "(*sync.Mutex).Lock")
	// This handles cases where the Pkg pointer might be nil in some SSA builds
	fullPath := fn.String()
	return fullPath == "(*sync.Mutex).Lock" ||
		fullPath == "(*sync.RWMutex).Lock" ||
		fullPath == "(*sync.RWMutex).RLock"
}

func isLockCall(call *ssa.Call) bool {
	return isLockCallCommon(&call.Call)
}

func isUnlockCallCommon(common *ssa.CallCommon) bool {
	if common == nil {
		return false
	}

	fn := common.StaticCallee()
	if fn == nil {
		return false
	}

	name := fn.Name()
	if name != "Unlock" && name != "RUnlock" {
		return false
	}

	fullPath := fn.String()
	return fullPath == "(*sync.Mutex).Unlock" ||
		fullPath == "(*sync.RWMutex).Unlock" ||
		fullPath == "(*sync.RWMutex).RUnlock"
}

func isUnlockCall(call *ssa.Call) bool {
	return isUnlockCallCommon(&call.Call)
}

func traceToObject(val ssa.Value) types.Object {
	for {
		switch v := val.(type) {
		case *ssa.FieldAddr:
			// This is the common case: &a.mu
			// We get the struct field object directly
			ptr := v.X.Type().Underlying().(*types.Pointer)
			strct := ptr.Elem().Underlying().(*types.Struct)
			return strct.Field(v.Field)

		case *ssa.UnOp:
			// If it's a pointer dereference (*ptr), look at the pointer
			val = v.X

		case *ssa.IndexAddr:
			// If it's a mutex in a slice (locks[i]), we treat the slice
			// object itself as the lock for simplicity in Week 2
			return traceToObject(v.X)

		case *ssa.Parameter:
			// If it was passed in as an argument
			return v.Object()

		case *ssa.Global:
			// If it's a global mutex
			return v.Object()

		default:
			// If we hit something we don't recognize, stop
			return nil
		}
	}
}

// Resolve a mutex variable name in an annotation to the corresponding assignment
// in the SSA blocks
func resolveObjectInScope(fn *ssa.Function, targetName string) types.Object {
	if targetName == "" {
		return nil
	}

	parts := strings.Split(targetName, ".")
	if len(parts) > 1 {
		return resolvePathInScope(fn, parts)
	}

	// Handle single-name resolution
	return resolveSingleName(fn, targetName)

}

// resolvePathInScope handles names with dots (e.g., "a.mu" or "mu.lock")
func resolvePathInScope(fn *ssa.Function, parts []string) types.Object {
	first := parts[0]

	// 1. Check if the first part is a parameter (includes receiver)
	for _, p := range fn.Params {
		if p.Name() == first {
			return findFieldPathInType(p.Type(), parts[1:])
		}
	}

	// 2. Implicit receiver check (the first part might be a field of the receiver)
	if len(fn.Params) > 0 {
		recv := fn.Params[0]
		// Case: "field.subfield" where "field" is on the receiver
		return findFieldPathInType(recv.Type(), parts)
	}

	return nil
}

// resolveSingleName handles simple identifiers (e.g., "mu")
func resolveSingleName(fn *ssa.Function, name string) types.Object {
	// 1. Check Parameters
	if obj := findInParams(fn, name); obj != nil {
		return obj
	}

	// 2. Check Implicit Receiver Fields
	if obj := findInReceiverFields(fn, name); obj != nil {
		return obj
	}

	// 3. Check Package Globals
	return findInPackageGlobals(fn, name)
}

func findInParams(fn *ssa.Function, name string) types.Object {
	for _, p := range fn.Params {
		if p.Name() == name {
			return p.Object()
		}
	}
	return nil
}

func findInReceiverFields(fn *ssa.Function, name string) types.Object {
	if len(fn.Params) == 0 {
		return nil
	}

	// Use a helper to peel away pointers and find the underlying struct
	strct, ok := getUnderlyingStruct(fn.Params[0].Type())
	if !ok {
		return nil
	}

	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if field.Name() == name {
			return field
		}
	}
	return nil
}

func findInPackageGlobals(fn *ssa.Function, name string) types.Object {
	if fn.Pkg != nil {
		if member, ok := fn.Pkg.Members[name]; ok {
			return member.Object()
		}
	}
	return nil
}

// Helper to handle the pointer/struct traversal logic
func getUnderlyingStruct(t types.Type) (*types.Struct, bool) {
	curr := t.Underlying()
	if ptr, ok := curr.(*types.Pointer); ok {
		curr = ptr.Elem().Underlying()
	}
	strct, ok := curr.(*types.Struct)
	return strct, ok
}

func findFieldPathInType(typ types.Type, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return nil
	}

	var found types.Object
	current := typ

	for _, fieldName := range fieldPath {
		if ptr, ok := current.Underlying().(*types.Pointer); ok {
			current = ptr.Elem()
		}

		strct, ok := current.Underlying().(*types.Struct)
		if !ok {
			return nil
		}

		matched := false
		for i := 0; i < strct.NumFields(); i++ {
			field := strct.Field(i)
			if field.Name() == fieldName {
				found = field
				current = field.Type()
				matched = true
				break
			}
		}

		if !matched {
			return nil
		}
	}

	return found
}

func getLockObject(instr *ssa.Call) types.Object {
	if len(instr.Call.Args) == 0 {
		return nil
	}

	receiver := instr.Call.Args[0]

	return traceToObject(receiver)
}

func getLockObjectFromCallCommon(common *ssa.CallCommon) types.Object {
	if common == nil || len(common.Args) == 0 {
		return nil
	}

	receiver := common.Args[0]
	return traceToObject(receiver)
}

func resolveObjectAtCallSite(call *ssa.Call, targetName string) types.Object {
	callee := call.Call.StaticCallee()
	if callee == nil || targetName == "" {
		return nil
	}

	parts := strings.Split(targetName, ".")
	if len(parts) == 0 {
		return nil
	}

	// Try mapping explicit parameter targets first (e.g., from.mu, to.mu, a.mu)
	first := parts[0]
	for i, p := range callee.Params {
		if p.Name() == first && i < len(call.Call.Args) {
			if len(parts) == 1 {
				return traceToObject(call.Call.Args[i])
			}
			return findFieldPathInValue(call.Call.Args[i], parts[1:])
		}
	}

	// Fallback: interpret target relative to receiver
	if len(call.Call.Args) > 0 {
		receiver := call.Call.Args[0]
		if len(parts) == 1 {
			return findFieldPathInValue(receiver, parts)
		}

		// Handles receiver-qualified targets like b.mu or b.vault.mu
		if len(callee.Params) > 0 && callee.Params[0].Name() == first {
			return findFieldPathInValue(receiver, parts[1:])
		}
		return findFieldPathInValue(receiver, parts)
	}
	return nil
}

func findFieldPathInValue(val ssa.Value, fieldPath []string) types.Object {
	if len(fieldPath) == 0 {
		return traceToObject(val)
	}

	typ := val.Type()
	var found types.Object

	for _, fieldName := range fieldPath {
		if ptr, ok := typ.Underlying().(*types.Pointer); ok {
			typ = ptr.Elem()
		}

		strct, ok := typ.Underlying().(*types.Struct)
		if !ok {
			return nil
		}

		matched := false
		for i := 0; i < strct.NumFields(); i++ {
			field := strct.Field(i)
			if field.Name() == fieldName {
				found = field
				typ = field.Type()
				matched = true
				break
			}
		}

		if !matched {
			return nil
		}
	}

	return found
}

func contractForFunction(fn *ssa.Function, registry *ir.ContractRegistry) *ir.FunctionContract {
	if fn == nil {
		return nil
	}

	recv := ""
	if fn.Signature != nil && fn.Signature.Recv() != nil {
		recv = ir.NormalizeTypeName(fn.Signature.Recv().Type().String())
	}

	if contract, ok := registry.Functions[ir.MakeFunctionKey(fn.Name(), recv)]; ok {
		return contract
	}

	if contract, ok := registry.Functions[fn.Name()]; ok {
		return contract
	}

	return nil
}

func handleCallInstruction(
	msg *ssa.Call,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if isLockCall(msg) {
		obj := getLockObject(msg)
		if obj != nil {
			state.HeldLocks[obj] = true
		}
	} else if isUnlockCall(msg) {
		obj := getLockObject(msg)
		if obj != nil {
			delete(state.HeldLocks, obj)
		}
	} else {
		callee := msg.Call.StaticCallee()
		if callee != nil {
			contract := contractForFunction(callee, registry)
			if contract != nil {
				for _, exp := range contract.Expectations {
					if exp.Kind == ir.Requires {
						// Map the requirement to the caller's objects
						reqObj := resolveObjectAtCallSite(msg, exp.Target)
						if !state.HeldLocks[reqObj] {
							reportMissingLock(msg, callee, exp.Target, reporter, fset)
						}
					}
				}
			}
		}
	}
}

func applyDeferredEffects(state *AnalysisState) {
	for obj := range state.DeferredLocks {
		state.HeldLocks[obj] = true
	}
	for obj := range state.DeferredUnlocks {
		delete(state.HeldLocks, obj)
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

// Analyze the instructions of a given block, updating lock/defer state in accordance with SSA side effects.
func analyzeInstructions(
	instrs []ssa.Instruction,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	for _, instr := range instrs {
		switch msg := instr.(type) {
		case *ssa.Call:
			handleCallInstruction(msg, state, registry, reporter, fset)
		case *ssa.Defer:
			registerDeferInstruction(msg, state)
		case *ssa.RunDefers:
			applyDeferredEffects(state)
		}
	}
}

func updateSuccessorState(
	succ *ssa.BasicBlock,
	current AnalysisState,
	blockEntryStates map[int]AnalysisState,
	worklist *worklist,
) {
	existing, seen := blockEntryStates[succ.Index]

	if !seen {
		blockEntryStates[succ.Index] = current.Copy()
		worklist.Push(succ)
		return
	}

	merged := existing.Intersect(current)
	if !existing.Equals(merged) {
		blockEntryStates[succ.Index] = merged
		worklist.Push(succ)
	}
}

// Perform analysis for a given function using depth first search
// to uncover every possible program path
func functionDepthFirstSearch(
	fn *ssa.Function,
	initialLockset LockSet,
	registry *ir.ContractRegistry,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if len(fn.Blocks) == 0 {
		return
	}

	entry := fn.Blocks[0]
	blockEntryStates := map[int]AnalysisState{
		entry.Index: newAnalysisState(initialLockset),
	}

	worklist := newBlockWorklist(entry)

	for !worklist.Empty() {
		curr := worklist.Pop()

		entryState := blockEntryStates[curr.Index]
		currentState := entryState.Copy()

		analyzeInstructions(curr.Instrs, &currentState, registry, reporter, fset)
		if logger.IsVerbose() {
			utils.PrintSSABlock(curr)
		}

		for _, succ := range curr.Succs {
			updateSuccessorState(
				succ,
				currentState,
				blockEntryStates,
				worklist,
			)
		}
	}
}

// Creates the initial lockset for a function, according to the Requires
// tag that is provided
func createInitialLockset(fn *ssa.Function, contract *ir.FunctionContract) LockSet {
	// Setup initial state
	initialLockset := make(LockSet)

	if contract != nil {
		for _, expectation := range contract.Expectations {
			// Handle annotation "requires" expectations
			if expectation.Kind == ir.Requires {
				obj := resolveObjectInScope(fn, expectation.Target)
				if obj != nil {
					initialLockset[obj] = true
					logger.Debugf("Initialized path with lock: %v\n", obj.Name())
				} else {
					logger.Debugf("Warning: Could not resolve lock target '%s' in function %s\n",
						expectation.Target, fn.Name())
				}
			}
		}
	}

	return initialLockset
}

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
