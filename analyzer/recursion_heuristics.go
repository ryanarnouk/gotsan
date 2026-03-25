package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

type recursionGraph struct {
	componentByFn map[*ssa.Function]int
	componentSize map[int]int
	selfRecursive map[*ssa.Function]bool
}

func buildRecursionGraph(pkg *ssa.Package) *recursionGraph {
	graph := &recursionGraph{
		componentByFn: make(map[*ssa.Function]int),
		componentSize: make(map[int]int),
		selfRecursive: make(map[*ssa.Function]bool),
	}

	if pkg == nil {
		return graph
	}

	functions := collectPackageFunctions(pkg)
	if len(functions) == 0 {
		return graph
	}

	adj := make(map[*ssa.Function][]*ssa.Function, len(functions))
	for fn := range functions {
		adj[fn] = nil
	}

	for fn := range functions {
		if len(fn.Blocks) == 0 {
			continue
		}

		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				callInstr, ok := instr.(*ssa.Call)
				if !ok {
					continue
				}

				targets := make([]*ssa.Function, 0, 1)
				if callee := callInstr.Call.StaticCallee(); callee != nil {
					targets = append(targets, callee)
				} else {
					targets = append(targets, resolveDynamicCallTargets(fn, callInstr)...)
				}

				for _, target := range targets {
					if target == nil {
						continue
					}

					if _, ok := functions[target]; !ok {
						continue
					}

					adj[fn] = append(adj[fn], target)
					if fn == target {
						graph.selfRecursive[fn] = true
					}
				}
			}
		}
	}

	index := 0
	stack := make([]*ssa.Function, 0, len(functions))
	onStack := make(map[*ssa.Function]bool, len(functions))
	indices := make(map[*ssa.Function]int, len(functions))
	lowlink := make(map[*ssa.Function]int, len(functions))
	nextComponentID := 0

	var strongConnect func(v *ssa.Function)
	strongConnect = func(v *ssa.Function) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, seen := indices[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
				continue
			}

			if onStack[w] && indices[w] < lowlink[v] {
				lowlink[v] = indices[w]
			}
		}

		if lowlink[v] != indices[v] {
			return
		}

		for {
			last := len(stack) - 1
			w := stack[last]
			stack = stack[:last]
			onStack[w] = false
			graph.componentByFn[w] = nextComponentID
			graph.componentSize[nextComponentID]++

			if w == v {
				break
			}
		}

		nextComponentID++
	}

	for fn := range functions {
		if _, seen := indices[fn]; !seen {
			strongConnect(fn)
		}
	}

	return graph
}

func collectPackageFunctions(pkg *ssa.Package) map[*ssa.Function]struct{} {
	functions := make(map[*ssa.Function]struct{})

	var addFunction func(*ssa.Function)
	addFunction = func(fn *ssa.Function) {
		if fn == nil {
			return
		}

		if _, seen := functions[fn]; seen {
			return
		}

		functions[fn] = struct{}{}
		for _, anon := range fn.AnonFuncs {
			addFunction(anon)
		}
	}

	for _, member := range pkg.Members {
		switch n := member.(type) {
		case *ssa.Function:
			addFunction(n)
		case *ssa.Type:
			addMethodsForType(pkg, n.Type(), addFunction)
		}
	}

	return functions
}

func addMethodsForType(pkg *ssa.Package, t types.Type, addFn func(*ssa.Function)) {
	if pkg == nil || t == nil || addFn == nil {
		return
	}

	methodSet := pkg.Prog.MethodSets.MethodSet(t)
	for i := range methodSet.Len() {
		selection := methodSet.At(i)
		fn := pkg.Prog.MethodValue(selection)
		if fn != nil && fn.Pkg == pkg {
			addFn(fn)
		}
	}

	ptrMethodSet := pkg.Prog.MethodSets.MethodSet(types.NewPointer(t))
	for i := range ptrMethodSet.Len() {
		fn := pkg.Prog.MethodValue(ptrMethodSet.At(i))
		if fn != nil && fn.Pkg == pkg {
			addFn(fn)
		}
	}
}

func (g *recursionGraph) isRecursiveEdge(caller *ssa.Function, callee *ssa.Function) bool {
	if g == nil || caller == nil || callee == nil {
		return false
	}

	callerComponent, ok := g.componentByFn[caller]
	if !ok {
		return false
	}

	calleeComponent, ok := g.componentByFn[callee]
	if !ok {
		return false
	}

	if callerComponent != calleeComponent {
		return false
	}

	if caller != callee {
		return true
	}

	return g.selfRecursive[caller]
}

func checkRecursiveCallLockReacquireHeuristic(
	callerFn *ssa.Function,
	calleeFn *ssa.Function,
	callSite *ssa.Call,
	state *AnalysisState,
	registry *ir.ContractRegistry,
	recursion *recursionGraph,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if callerFn == nil || calleeFn == nil || callSite == nil || state == nil {
		return
	}

	if !recursion.isRecursiveEdge(callerFn, calleeFn) {
		return
	}

	if len(state.HeldLocks) == 0 {
		return
	}

	contract := contractForFunction(calleeFn, registry)
	if contract != nil && len(contract.Expectations[ir.Acquires]) > 0 {
		// Contract-aware checks already report deterministic reacquire errors.
		return
	}

	evidenceByLock := collectTransitiveLockUsageEvidence(calleeFn, map[*ssa.Function]bool{})
	if len(evidenceByLock) == 0 {
		return
	}

	lockName, ok := matchHeldLockFromEvidence(state, evidenceByLock)
	if !ok {
		return
	}

	reportRecursiveCallMayReacquireLock(callSite, callerFn, calleeFn, lockName, reporter, fset)
}

func collectTransitiveLockUsageEvidence(
	fn *ssa.Function,
	active map[*ssa.Function]bool,
) map[types.Object]lockUsageEvidence {
	if fn == nil {
		return nil
	}

	if active[fn] {
		// Break call cycles while preserving evidence gathered so far.
		return nil
	}

	active[fn] = true
	defer delete(active, fn)

	evidenceByLock := collectLockUsageEvidence(fn)
	if evidenceByLock == nil {
		evidenceByLock = make(map[types.Object]lockUsageEvidence)
	}

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			callInstr, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}

			targets := make([]*ssa.Function, 0, 1)
			if nestedCallee := callInstr.Call.StaticCallee(); nestedCallee != nil {
				targets = append(targets, nestedCallee)
			} else {
				targets = append(targets, resolveDynamicCallTargets(fn, callInstr)...)
			}

			for _, target := range targets {
				nestedEvidence := collectTransitiveLockUsageEvidence(target, active)
				mergeLockUsageEvidence(evidenceByLock, nestedEvidence)
			}
		}
	}

	return evidenceByLock
}

func mergeLockUsageEvidence(
	into map[types.Object]lockUsageEvidence,
	from map[types.Object]lockUsageEvidence,
) {
	if len(from) == 0 {
		return
	}

	for lockObj, incoming := range from {
		if lockObj == nil {
			continue
		}

		curr := into[lockObj]
		if curr.firstPos == token.NoPos {
			curr.firstPos = incoming.firstPos
		}
		curr.lockCalls += incoming.lockCalls
		curr.unlockCalls += incoming.unlockCalls
		into[lockObj] = curr
	}
}

func matchHeldLockFromEvidence(state *AnalysisState, evidenceByLock map[types.Object]lockUsageEvidence) (string, bool) {
	if state == nil || len(state.HeldLocks) == 0 || len(evidenceByLock) == 0 {
		return "", false
	}

	for heldObj := range state.HeldLocks {
		if heldObj == nil {
			continue
		}

		ev, seen := evidenceByLock[heldObj]
		if !seen || ev.lockCalls == 0 {
			continue
		}

		return heldObj.Name(), true
	}

	heldByName := make(map[string]bool, len(state.HeldLocks))
	for heldObj := range state.HeldLocks {
		if heldObj == nil || heldObj.Name() == "" {
			continue
		}
		heldByName[heldObj.Name()] = true
	}

	for lockObj, ev := range evidenceByLock {
		if lockObj == nil || ev.lockCalls == 0 {
			continue
		}

		if heldByName[lockObj.Name()] {
			return lockObj.Name(), true
		}
	}

	return "", false
}
