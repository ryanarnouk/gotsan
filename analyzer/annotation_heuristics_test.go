package analyzer

import (
	"go/token"
	"go/types"
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

	kind, target, ok := firstLikelyMissingAnnotation(
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
	if target != "mu" {
		t.Fatalf("expected mu, got %s", target)
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

	kind, target, ok := firstLikelyMissingAnnotation(
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
	if target != "mu" {
		t.Fatalf("expected mu, got %s", target)
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

	kind, target, ok := firstLikelyMissingAnnotation(
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

	if target != "fi.mu" {
		t.Fatalf("expected fi.mu target, got %s", target)
	}
}
