package analyzer

import (
	"go/token"
	"go/types"
	"testing"

	"gotsan/ir"

	"golang.org/x/tools/go/ssa"
)

func TestRecursionGraphIsRecursiveEdge(t *testing.T) {
	a := &ssa.Function{}
	b := &ssa.Function{}
	c := &ssa.Function{}
	d := &ssa.Function{}

	g := &recursionGraph{
		componentByFn: map[*ssa.Function]int{a: 1, b: 1, c: 2, d: 3},
		componentSize: map[int]int{1: 2, 2: 1, 3: 1},
		selfRecursive: map[*ssa.Function]bool{d: true},
	}

	if !g.isRecursiveEdge(a, b) {
		t.Fatal("expected mutual-recursion edge in same SCC")
	}

	if g.isRecursiveEdge(a, c) {
		t.Fatal("did not expect recursive edge across SCCs")
	}

	if g.isRecursiveEdge(c, c) {
		t.Fatal("single-node SCC without self-loop should not be recursive")
	}

	if !g.isRecursiveEdge(d, d) {
		t.Fatal("self-loop function should be treated as recursive")
	}
}

func TestMatchHeldLockFromEvidenceMatchesByObject(t *testing.T) {
	lockObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	state := &AnalysisState{
		HeldLocks:       LockSet{lockObj: true},
		MayHeldLocks:    LockSet{},
		DeferredLocks:   LockSet{},
		DeferredUnlocks: LockSet{},
	}

	evidence := map[types.Object]lockUsageEvidence{
		lockObj: {lockCalls: 1},
	}

	matched, ok := matchHeldLockFromEvidence(state, evidence)
	if !ok {
		t.Fatal("expected match for held lock object")
	}

	if matched != "mu" {
		t.Fatalf("expected matched lock mu, got %q", matched)
	}
}

func TestMatchHeldLockFromEvidenceMatchesByNameFallback(t *testing.T) {
	heldObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	inferredObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	state := &AnalysisState{
		HeldLocks:       LockSet{heldObj: true},
		MayHeldLocks:    LockSet{},
		DeferredLocks:   LockSet{},
		DeferredUnlocks: LockSet{},
	}

	evidence := map[types.Object]lockUsageEvidence{
		inferredObj: {lockCalls: 1},
	}

	matched, ok := matchHeldLockFromEvidence(state, evidence)
	if !ok {
		t.Fatal("expected match by lock name fallback")
	}

	if matched != "mu" {
		t.Fatalf("expected matched lock mu, got %q", matched)
	}
}

func TestCheckRecursiveCallLockReacquireHeuristicSkipsWhenAcquiresContractExists(t *testing.T) {
	caller := &ssa.Function{}
	callee := &ssa.Function{}
	callSite := &ssa.Call{}

	lockObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	state := &AnalysisState{
		HeldLocks:       LockSet{lockObj: true},
		MayHeldLocks:    LockSet{},
		DeferredLocks:   LockSet{},
		DeferredUnlocks: LockSet{},
	}

	recursion := &recursionGraph{
		componentByFn: map[*ssa.Function]int{caller: 1, callee: 1},
		componentSize: map[int]int{1: 2},
		selfRecursive: map[*ssa.Function]bool{},
	}

	contract := &ir.FunctionContract{Expectations: make(map[ir.AnnotationKind][]ir.Requirement)}
	contract.Expectations[ir.Acquires] = []ir.Requirement{{Target: "mu"}}
	registry := ir.NewContractRegistry()
	registry.Functions[callee.Name()] = contract

	checkRecursiveCallLockReacquireHeuristic(caller, callee, callSite, state, registry, recursion, nil, nil)
}

func TestMergeLockUsageEvidence(t *testing.T) {
	mu := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])

	into := map[types.Object]lockUsageEvidence{
		mu: {lockCalls: 1, unlockCalls: 1},
	}
	from := map[types.Object]lockUsageEvidence{
		mu: {lockCalls: 2, unlockCalls: 3},
	}

	mergeLockUsageEvidence(into, from)

	got := into[mu]
	if got.lockCalls != 3 || got.unlockCalls != 4 {
		t.Fatalf("expected merged counters 3/4, got %d/%d", got.lockCalls, got.unlockCalls)
	}
}

func TestCollectTransitiveLockUsageEvidenceHandlesCycles(t *testing.T) {
	fn := &ssa.Function{}
	active := map[*ssa.Function]bool{fn: true}

	evidence := collectTransitiveLockUsageEvidence(fn, active)
	if len(evidence) != 0 {
		t.Fatalf("expected no evidence when function is already active, got %d", len(evidence))
	}
}
