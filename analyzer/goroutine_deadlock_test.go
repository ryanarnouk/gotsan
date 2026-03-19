package analyzer

import (
	"go/token"
	"go/types"
	"testing"

	"gotsan/ir"

	"golang.org/x/tools/go/ssa"
)

func makeVar(name string) types.Object {
	return types.NewVar(token.NoPos, nil, name, types.Typ[types.Int])
}

func TestLockDisplayName(t *testing.T) {
	obj := makeVar("mu")
	if got := lockDisplayName(lockRef{Obj: obj}); got != "mu" {
		t.Errorf("expected mu, got %q", got)
	}

	if got := lockDisplayName(lockRef{Name: "foo"}); got != "foo" {
		t.Errorf("expected foo, got %q", got)
	}

	if got := lockDisplayName(lockRef{}); got != "<unknown lock>" {
		t.Errorf("expected <unknown lock>, got %q", got)
	}
}

func TestSameLock(t *testing.T) {
	obj1 := makeVar("x")
	obj2 := makeVar("x") // different object with same name
	a := lockRef{Obj: obj1}
	b := lockRef{Obj: obj1}
	c := lockRef{Obj: obj2}

	if !sameLock(a, b) {
		t.Error("expected identical object locks to be same")
	}
	if sameLock(a, c) {
		t.Error("different objects should not be same even if names match")
	}

	n1 := lockRef{Name: "foo"}
	n2 := lockRef{Name: "foo"}
	n3 := lockRef{Name: "bar"}
	if !sameLock(n1, n2) {
		t.Error("locks with same Name should be same")
	}
	if sameLock(n1, n3) {
		t.Error("locks with different Names should not be same")
	}

	// mixed cases
	if sameLock(lockRef{Obj: obj1}, lockRef{Name: "x"}) {
		t.Error("a non-nil Obj should not equal a Name-only lock")
	}
}

func TestIndexOfLock(t *testing.T) {
	a := lockRef{Name: "a"}
	b := lockRef{Name: "b"}
	c := lockRef{Name: "c"}
	arr := []lockRef{a, b}

	if got := indexOfLock(arr, a); got != 0 {
		t.Errorf("expected index 0, got %d", got)
	}
	if got := indexOfLock(arr, b); got != 1 {
		t.Errorf("expected index 1, got %d", got)
	}
	if got := indexOfLock(arr, c); got != -1 {
		t.Errorf("expected -1 for missing lock, got %d", got)
	}
}

func TestFindOrderInversion(t *testing.T) {
	a := lockRef{Name: "a"}
	b := lockRef{Name: "b"}
	c := lockRef{Name: "c"}

	// basic inversion: [a,b] vs [b,a]
	f1, f2, ok := findOrderInversion([]lockRef{a, b}, []lockRef{b, a})
	if !ok || !sameLock(f1, a) || !sameLock(f2, b) {
		t.Errorf("unexpected result: %v %v %v", f1, f2, ok)
	}

	// no inversion
	_, _, ok = findOrderInversion([]lockRef{a, b}, []lockRef{a, b})
	if ok {
		t.Error("found inversion when none expected")
	}

	// missing lock in second sequence
	_, _, ok = findOrderInversion([]lockRef{a, b}, []lockRef{a, c})
	if ok {
		t.Error("should not report inversion if one lock is missing")
	}

	// repeated locks should be skipped
	list1 := []lockRef{a, a, b}
	list2 := []lockRef{b, a}
	f1, f2, ok = findOrderInversion(list1, list2)
	if !ok || !sameLock(f1, a) || !sameLock(f2, b) {
		t.Errorf("expected inversion with duplicates, got %v %v %v", f1, f2, ok)
	}
}

func TestFirstRepeatedLock(t *testing.T) {
	a := lockRef{Name: "a"}
	b := lockRef{Name: "b"}

	if _, ok := firstRepeatedLock([]lockRef{a, b}); ok {
		t.Fatal("did not expect repeated lock")
	}

	repeated, ok := firstRepeatedLock([]lockRef{a, b, a})
	if !ok {
		t.Fatal("expected repeated lock")
	}

	if !sameLock(repeated, a) {
		t.Fatalf("expected repeated lock a, got %v", repeated)
	}
}

func TestContainsLock(t *testing.T) {
	a := lockRef{Name: "a"}
	b := lockRef{Name: "b"}
	c := lockRef{Name: "c"}

	order := []lockRef{a, b}
	if !containsLock(order, a) {
		t.Fatal("expected to find lock a")
	}
	if !containsLock(order, b) {
		t.Fatal("expected to find lock b")
	}
	if containsLock(order, c) {
		t.Fatal("did not expect to find lock c")
	}
}

func TestAcquireOrderHelpers(t *testing.T) {
	// nil inputs
	if ord := acquireOrderForGoCall(nil, nil, nil); ord != nil {
		t.Errorf("expected nil for nil inputs, got %v", ord)
	}
	if ord := acquireOrderForCall(nil, nil, nil); ord != nil {
		t.Errorf("expected nil for nil inputs, got %v", ord)
	}

	// contract with no acquires
	empty := &ir.FunctionContract{Expectations: make(map[ir.AnnotationKind][]ir.Requirement)}
	g := &ssa.Go{Call: ssa.CallCommon{Args: nil}}
	f := &ssa.Function{}
	if ord := acquireOrderForGoCall(g, f, empty); ord != nil {
		t.Errorf("expected nil when contract has no acquires")
	}
	call := &ssa.Call{Call: ssa.CallCommon{Args: nil}}
	if ord := acquireOrderForCall(call, f, empty); ord != nil {
		t.Errorf("expected nil when contract has no acquires")
	}

	// contract with some acquires; resolver returns nil so we only get Names
	contract := &ir.FunctionContract{Expectations: make(map[ir.AnnotationKind][]ir.Requirement)}
	contract.Expectations[ir.Acquires] = []ir.Requirement{{Target: "foo"}, {Target: "bar"}}

	ord := acquireOrderForGoCall(g, f, contract)
	if len(ord) != 2 || ord[0].Name != "foo" || ord[1].Name != "bar" {
		t.Errorf("unexpected order for go call: %v", ord)
	}

	ord = acquireOrderForCall(call, f, contract)
	if len(ord) != 2 || ord[0].Name != "foo" || ord[1].Name != "bar" {
		t.Errorf("unexpected order for call: %v", ord)
	}
}
