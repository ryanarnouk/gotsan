package analyzer

import (
	"fmt"
	"go/types"
	"gotsan/ir"
	"gotsan/utils"
	"strings"

	"golang.org/x/tools/go/ssa"
)

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
	if len(parts) == 0 {
		return nil
	}

	if len(parts) > 1 {
		first := parts[0]

		for _, p := range fn.Params {
			if p.Name() == first {
				return findFieldPathInType(p.Type(), parts[1:])
			}
		}

		if len(fn.Params) > 0 {
			recv := fn.Params[0]
			if recv.Name() == first {
				return findFieldPathInType(recv.Type(), parts[1:])
			}

			return findFieldPathInType(recv.Type(), parts)
		}

		return nil
	}

	targetName = parts[0]

	// 1. Check Function Parameters (this includes the receiver 'a' for methods)
	// If the annotation says 'mu' and the param is 'mu', we found it.
	for _, p := range fn.Params {
		if p.Name() == targetName {
			return p.Object()
		}
	}

	// 2. Check Receiver Fields (The "Implicit This" Case)
	// If the function is (a *Account) and the annotation is 'mu',
	// the user likely means 'a.mu'.
	if len(fn.Params) > 0 {
		recv := fn.Params[0] // Standard Go SSA: first param is the receiver
		if ptr, ok := recv.Type().Underlying().(*types.Pointer); ok {
			if strct, ok := ptr.Elem().Underlying().(*types.Struct); ok {
				for i := 0; i < strct.NumFields(); i++ {
					field := strct.Field(i)
					if field.Name() == targetName {
						return field
					}
				}
			}
		}
	}

	// 3. Check Package-Level Globals
	// If 'mu' isn't local or in the struct, it might be a global mutex.
	if fn.Pkg != nil {
		if member, ok := fn.Pkg.Members[targetName]; ok {
			if obj := member.Object(); obj != nil {
				return obj
			}
		}
	}

	return nil
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

type AnalysisState struct {
	HeldLocks       LockSet
	DeferredLocks   LockSet
	DeferredUnlocks LockSet
}

func newAnalysisState(initial LockSet) AnalysisState {
	return AnalysisState{
		HeldLocks:       initial.Copy(),
		DeferredLocks:   make(LockSet),
		DeferredUnlocks: make(LockSet),
	}
}

func (s AnalysisState) Copy() AnalysisState {
	return AnalysisState{
		HeldLocks:       s.HeldLocks.Copy(),
		DeferredLocks:   s.DeferredLocks.Copy(),
		DeferredUnlocks: s.DeferredUnlocks.Copy(),
	}
}

func (s AnalysisState) Equals(other AnalysisState) bool {
	return s.HeldLocks.Equals(other.HeldLocks) &&
		s.DeferredLocks.Equals(other.DeferredLocks) &&
		s.DeferredUnlocks.Equals(other.DeferredUnlocks)
}

func (s AnalysisState) Intersect(other AnalysisState) AnalysisState {
	return AnalysisState{
		HeldLocks:       s.HeldLocks.Intersect(other.HeldLocks),
		DeferredLocks:   s.DeferredLocks.Intersect(other.DeferredLocks),
		DeferredUnlocks: s.DeferredUnlocks.Intersect(other.DeferredUnlocks),
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

// Analyze the instructions of a given block, updating lock/defer state in accordance with SSA side effects.
func analyzeInstructions(instrs []ssa.Instruction, state *AnalysisState, fn *ssa.Function, registry *ir.ContractRegistry) {
	for _, instr := range instrs {
		switch msg := instr.(type) {
		case *ssa.Call:
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
					if contract == nil {
						continue
					}
					for _, exp := range contract.Expectations {
						if exp.Kind == ir.Requires {
							// Map the requirement to the caller's objects
							reqObj := resolveObjectAtCallSite(msg, exp.Target)
							if !state.HeldLocks[reqObj] {
								fmt.Printf("ERROR: Call to %s requires lock %s, but it's not held!\n",
									callee.Name(), exp.Target)
							}
						}
					}
				}
			}
		case *ssa.Defer:
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
		case *ssa.RunDefers:
			applyDeferredEffects(state)
		}
	}
}

// Perform analysis for a given function using depth first search
// to uncover every possible program path
func functionDepthFirstSearch(fn *ssa.Function, initialLS LockSet, registry *ir.ContractRegistry) {
	if len(fn.Blocks) == 0 {
		return
	}

	blockEntryStates := make(map[int]AnalysisState)
	worklist := []*ssa.BasicBlock{fn.Blocks[0]}
	inQueue := map[int]bool{fn.Blocks[0].Index: true}
	blockEntryStates[fn.Blocks[0].Index] = newAnalysisState(initialLS)

	for len(worklist) > 0 {
		curr := worklist[0]
		worklist = worklist[1:]
		inQueue[curr.Index] = false

		entryState := blockEntryStates[curr.Index]
		currentState := entryState.Copy()
		analyzeInstructions(curr.Instrs, &currentState, fn, registry)

		utils.PrintSSABlock(curr)

		for _, succ := range curr.Succs {
			existingState, seen := blockEntryStates[succ.Index]

			if !seen {
				blockEntryStates[succ.Index] = currentState.Copy()
				if !inQueue[succ.Index] {
					worklist = append(worklist, succ)
					inQueue[succ.Index] = true
				}
				continue
			}

			merged := existingState.Intersect(currentState)
			if !existingState.Equals(merged) {
				blockEntryStates[succ.Index] = merged
				if !inQueue[succ.Index] {
					worklist = append(worklist, succ)
					inQueue[succ.Index] = true
				}
			}
		}
	}
}

// Analyze a function, recursively handling any anonymous functinos
// within it's body
func analyzeFunction(fn *ssa.Function, registry *ir.ContractRegistry) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}

	// Setup initial state
	initialLockset := make(LockSet)
	contract := contractForFunction(fn, registry)

	if contract != nil {
		for _, expectation := range contract.Expectations {
			// Handle annotation "requires" expectations
			if expectation.Kind == ir.Requires {
				obj := resolveObjectInScope(fn, expectation.Target)
				if obj != nil {
					initialLockset[obj] = true
					fmt.Printf("Initialized path with lock: %v\n", obj.Name())
				} else {
					fmt.Printf("Warning: Could not resolve lock target '%s' in function %s\n",
						expectation.Target, fn.Name())
				}
			}
		}
	}

	fmt.Println("Function analyzed: ", fn.Name(), contract)
	// Begin DFS through function
	functionDepthFirstSearch(fn, initialLockset, registry)

	// Recurse through any anonymous functions
	for _, anon := range fn.AnonFuncs {
		analyzeFunction(anon, registry)
	}
}

func findMethodsForType(pkg *ssa.Package, t types.Type, registry *ir.ContractRegistry) {
	// Check methods/interface implementing a type
	methodSet := pkg.Prog.MethodSets.MethodSet(t)
	for i := range methodSet.Len() {
		selection := methodSet.At(i)
		fn := pkg.Prog.MethodValue(selection)
		if fn != nil && fn.Pkg == pkg {
			analyzeFunction(fn, registry)
		}
	}

	// Check the pointer to the type
	ptrMset := pkg.Prog.MethodSets.MethodSet(types.NewPointer(t))
	for i := range ptrMset.Len() {
		if fn := pkg.Prog.MethodValue(ptrMset.At(i)); fn != nil {
			analyzeFunction(fn, registry)
		}
	}
}

func Run(pkg *ssa.Package, registry *ir.ContractRegistry) {
	for _, member := range pkg.Members {
		switch n := member.(type) {
		case *ssa.Function:
			analyzeFunction(n, registry)
		case *ssa.Type:
			// Check if the type has any methods
			// This appears when using an interface
			findMethodsForType(pkg, n.Type(), registry)
		}
	}
}
