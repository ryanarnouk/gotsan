package analyzer

import (
	"go/token"
	"go/types"
	"gotsan/ir"
	"gotsan/utils/report"
	"strings"

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

	kind, lockObj, ok := firstLikelyMissingAnnotation(
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

	evidence := evidenceByLock[lockObj]
	pos := evidence.firstPos
	lockName := preferredLockDisplayName(fn, contract, kind, lockObj)
	if lockName == "" {
		lockName = lockObj.Name()
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
) (string, types.Object, bool) {
	for lockObj, evidence := range evidenceByLock {
		if lockObj == nil {
			continue
		}

		if evidence.lockCalls > 0 && evidence.unlockCalls > 0 && !hasAcquiresForLock(lockObj) {
			return "acquires", lockObj, true
		}
	}

	for lockObj, evidence := range evidenceByLock {
		if lockObj == nil {
			continue
		}

		if evidence.lockCalls == 0 && evidence.unlockCalls > 0 && !hasRequiresForLock(lockObj) {
			return "requires", lockObj, true
		}
	}

	return "", nil, false
}

func preferredLockDisplayName(
	fn *ssa.Function,
	contract *ir.FunctionContract,
	kind string,
	lockObj types.Object,
) string {
	if lockObj == nil {
		return ""
	}

	annotationKind, ok := annotationKindFromString(kind)
	if ok && fn != nil && contract != nil {
		requirements := contract.Expectations[annotationKind]
		for _, req := range requirements {
			if resolved := resolveObjectInScope(fn, req.Target); resolved == lockObj {
				return req.Target
			}
		}
	}

	if fn != nil {
		if field, ok := lockObj.(*types.Var); ok && field.Anonymous() && len(fn.Params) > 0 {
			recvName := fn.Params[0].Name()
			if recvName != "" {
				return recvName + "." + lockObj.Name()
			}
		}
	}

	return lockObj.Name()
}

func annotationKindFromString(kind string) (ir.AnnotationKind, bool) {
	switch kind {
	case "acquires":
		return ir.Acquires, true
	case "requires":
		return ir.Requires, true
	default:
		return ir.Acquires, false
	}
}

func lockCoveredByContract(
	fn *ssa.Function,
	contract *ir.FunctionContract,
	kind ir.AnnotationKind,
	lockObj types.Object,
) bool {
	if contract == nil || lockObj == nil {
		return false
	}

	requirements := contract.Expectations[kind]
	for _, req := range requirements {
		if fn != nil {
			resolved := resolveObjectInScope(fn, req.Target)
			if resolved != nil {
				if resolved == lockObj {
					return true
				}
				continue
			}
		}

		if req.Target == lockObj.Name() || lastTargetSegment(req.Target) == lockObj.Name() {
			return true
		}
	}

	return false
}

func lastTargetSegment(target string) string {
	if target == "" {
		return ""
	}

	idx := strings.LastIndex(target, ".")
	if idx == -1 {
		return target
	}

	if idx+1 >= len(target) {
		return ""
	}

	return target[idx+1:]
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
