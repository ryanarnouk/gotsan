package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/report"

	"golang.org/x/tools/go/ssa"
)

type lockUsageEvidence struct {
	firstPos    token.Pos
	lockCalls   int
	unlockCalls int
}

func detectLikelyMissingLockAnnotations(
	fn *ssa.Function,
	contract *ir.FunctionContract,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if fn == nil || len(fn.Blocks) == 0 {
		return
	}

	evidenceByLock := collectLockUsageEvidence(fn)
	if len(evidenceByLock) == 0 {
		return
	}

	kind, lockName, ok := firstLikelyMissingAnnotation(
		evidenceByLock,
		func(lockObj types.Object) bool {
			return lockCoveredByContract(fn, contract, ir.Acquires, lockObj)
		},
		func(lockObj types.Object) bool {
			return lockCoveredByContract(fn, contract, ir.Requires, lockObj)
		},
	)
	if !ok {
		return
	}

	var pos token.Pos
	for lockObj, evidence := range evidenceByLock {
		if lockObj == nil || lockObj.Name() != lockName {
			continue
		}
		pos = evidence.firstPos
		break
	}

	reportLikelyMissingAnnotation(
		fn,
		pos,
		kind,
		lockName,
		reporter,
		fset,
	)
}

func firstLikelyMissingAnnotation(
	evidenceByLock map[types.Object]lockUsageEvidence,
	hasAcquiresForLock func(types.Object) bool,
	hasRequiresForLock func(types.Object) bool,
) (string, string, bool) {
	for lockObj, evidence := range evidenceByLock {
		if lockObj == nil {
			continue
		}

		if evidence.lockCalls > 0 && evidence.unlockCalls > 0 && !hasAcquiresForLock(lockObj) {
			return "acquires", lockObj.Name(), true
		}
	}

	for lockObj, evidence := range evidenceByLock {
		if lockObj == nil {
			continue
		}

		if evidence.lockCalls == 0 && evidence.unlockCalls > 0 && !hasRequiresForLock(lockObj) {
			return "requires", lockObj.Name(), true
		}
	}

	return "", "", false
}

func lockCoveredByContract(
	fn *ssa.Function,
	contract *ir.FunctionContract,
	kind ir.AnnotationKind,
	lockObj types.Object,
) bool {
	if fn == nil || contract == nil || lockObj == nil {
		return false
	}

	requirements := contract.Expectations[kind]
	for _, req := range requirements {
		resolved := resolveObjectInScope(fn, req.Target)
		if resolved != nil {
			if resolved == lockObj {
				return true
			}
			continue
		}

		if req.Target == lockObj.Name() {
			return true
		}
	}

	return false
}

func collectLockUsageEvidence(fn *ssa.Function) map[types.Object]lockUsageEvidence {
	evidenceByLock := make(map[types.Object]lockUsageEvidence)

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			switch msg := instr.(type) {
			case *ssa.Call:
				if isLockCall(msg) {
					obj := getLockObject(msg)
					if obj == nil {
						continue
					}

					ev := evidenceByLock[obj]
					if ev.firstPos == token.NoPos {
						ev.firstPos = msg.Pos()
					}
					ev.lockCalls++
					evidenceByLock[obj] = ev
					continue
				}

				if isUnlockCall(msg) {
					obj := getLockObject(msg)
					if obj == nil {
						continue
					}

					ev := evidenceByLock[obj]
					if ev.firstPos == token.NoPos {
						ev.firstPos = msg.Pos()
					}
					ev.unlockCalls++
					evidenceByLock[obj] = ev
				}
			case *ssa.Defer:
				if !isUnlockCallCommon(&msg.Call) {
					continue
				}

				obj := getLockObjectFromCallCommon(&msg.Call)
				if obj == nil {
					continue
				}

				ev := evidenceByLock[obj]
				if ev.firstPos == token.NoPos {
					ev.firstPos = msg.Pos()
				}
				ev.unlockCalls++
				evidenceByLock[obj] = ev
			}
		}
	}

	return evidenceByLock
}
