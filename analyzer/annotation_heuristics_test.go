package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"testing"
)

func TestDetectLikelyMissingLockAnnotationsClassifiesAcquiresCandidate(t *testing.T) {
	lockObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	evidence := map[types.Object]lockUsageEvidence{
		lockObj: {
			firstPos:    token.Pos(10),
			lockCalls:   1,
			unlockCalls: 1,
		},
	}

	kind, targetObj, ok := firstLikelyMissingAnnotation(
		evidence,
		func(types.Object) bool { return false },
		func(types.Object) bool { return false },
	)
	if !ok {
		t.Fatal("expected acquires candidate")
	}
	if kind != "acquires" {
		t.Fatalf("expected acquires, got %s", kind)
	}
	if targetObj != lockObj {
		t.Fatalf("expected lock object %v, got %v", lockObj, targetObj)
	}
}

func TestDetectLikelyMissingLockAnnotationsClassifiesRequiresCandidate(t *testing.T) {
	lockObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	evidence := map[types.Object]lockUsageEvidence{
		lockObj: {
			firstPos:    token.Pos(10),
			lockCalls:   0,
			unlockCalls: 1,
		},
	}

	kind, targetObj, ok := firstLikelyMissingAnnotation(
		evidence,
		func(types.Object) bool { return true },
		func(types.Object) bool { return false },
	)
	if !ok {
		t.Fatal("expected requires candidate")
	}
	if kind != "requires" {
		t.Fatalf("expected requires, got %s", kind)
	}
	if targetObj != lockObj {
		t.Fatalf("expected lock object %v, got %v", lockObj, targetObj)
	}
}

func TestDetectLikelyMissingLockAnnotationsNoCandidateWhenContractsPresent(t *testing.T) {
	lockObj := types.NewVar(token.NoPos, nil, "mu", types.Typ[types.Int])
	evidence := map[types.Object]lockUsageEvidence{
		lockObj: {
			firstPos:    token.Pos(10),
			lockCalls:   1,
			unlockCalls: 1,
		},
	}

	if _, _, ok := firstLikelyMissingAnnotation(
		evidence,
		func(types.Object) bool { return true },
		func(types.Object) bool { return true },
	); ok {
		t.Fatal("did not expect missing-annotation candidate")
	}
}

func TestDetectLikelyMissingLockAnnotationsPerLockCoverage(t *testing.T) {
	siMu := types.NewVar(token.NoPos, nil, "si.mu", types.Typ[types.Int])
	fiMu := types.NewVar(token.NoPos, nil, "fi.mu", types.Typ[types.Int])

	evidence := map[types.Object]lockUsageEvidence{
		siMu: {
			firstPos:    token.Pos(10),
			lockCalls:   1,
			unlockCalls: 1,
		},
		fiMu: {
			firstPos:    token.Pos(20),
			lockCalls:   1,
			unlockCalls: 1,
		},
	}

	kind, targetObj, ok := firstLikelyMissingAnnotation(
		evidence,
		func(obj types.Object) bool {
			return obj == siMu
		},
		func(types.Object) bool { return false },
	)

	if !ok {
		t.Fatal("expected acquires candidate for uncovered lock")
	}

	if kind != "acquires" {
		t.Fatalf("expected acquires, got %s", kind)
	}

	if targetObj != fiMu {
		t.Fatalf("expected fi.mu lock object, got %v", targetObj)
	}
}

func TestLastTargetSegment(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{target: "proxy.connTrackLock", want: "connTrackLock"},
		{target: "mu", want: "mu"},
		{target: "", want: ""},
		{target: "a.", want: ""},
	}

	for _, tc := range tests {
		got := lastTargetSegment(tc.target)
		if got != tc.want {
			t.Fatalf("lastTargetSegment(%q) = %q, want %q", tc.target, got, tc.want)
		}
	}
}

func TestLockCoveredByContractFallbackToLastSegment(t *testing.T) {
	lockObj := types.NewVar(token.NoPos, nil, "connTrackLock", types.Typ[types.Int])

	contract := &ir.FunctionContract{Expectations: make(map[ir.AnnotationKind][]ir.Requirement)}
	contract.Expectations[ir.Acquires] = []ir.Requirement{{Target: "proxy.connTrackLock"}}

	if !lockCoveredByContract(nil, contract, ir.Acquires, lockObj) {
		t.Fatal("expected fallback last-segment coverage to match connTrackLock")
	}
}

func TestTypeEmbedsLockAnonymousField(t *testing.T) {
	mutexType := types.NewNamed(types.NewTypeName(token.NoPos, nil, "Mutex", nil), types.NewStruct(nil, nil), nil)
	embeddedMutexField := types.NewField(token.NoPos, nil, "Mutex", mutexType, true)
	containerType := types.NewStruct([]*types.Var{embeddedMutexField}, nil)

	if !typeEmbedsLock(containerType, embeddedMutexField) {
		t.Fatal("expected anonymous embedded field to cover lock object")
	}
}

func TestTypeEmbedsLockRejectsUnrelatedField(t *testing.T) {
	mutexType := types.NewNamed(types.NewTypeName(token.NoPos, nil, "Mutex", nil), types.NewStruct(nil, nil), nil)
	embeddedMutexField := types.NewField(token.NoPos, nil, "Mutex", mutexType, true)
	containerType := types.NewStruct([]*types.Var{embeddedMutexField}, nil)

	otherMutexField := types.NewField(token.NoPos, nil, "Mutex", types.Typ[types.Int], true)
	if typeEmbedsLock(containerType, otherMutexField) {
		t.Fatal("expected unrelated anonymous field not to be considered covered")
	}
}
