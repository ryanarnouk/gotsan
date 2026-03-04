package analyzer

import (
	"go/token"
	"gotsan/utils/logger"
	"gotsan/utils/report"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// For unknown position (i.e., a void return)
// use this method to return a position in the file for the warning report
func returnDiagnosticPos(fn *ssa.Function, instr ssa.Instruction) token.Pos {
	if instr != nil && instr.Pos() != token.NoPos {
		return instr.Pos()
	}

	if fn != nil && fn.Syntax() != nil {
		end := fn.Syntax().End()
		if end != token.NoPos {
			if int(end) > 1 {
				return token.Pos(int(end) - 1)
			}
			return end
		}
	}

	if fn != nil {
		return fn.Pos()
	}

	return token.NoPos
}

// Analysis reporting helper functions

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

func reportGuardViolation(
	instr ssa.Instruction,
	dataName string,
	mutexName string,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if reporter == nil || fset == nil {
		logger.Infof("ERROR: Access to %s requires lock %s, but it's not held\n", dataName, mutexName)
		return
	}

	position := fset.Position(instr.Pos())
	reporter.Warn(report.Diagnostic{
		Pos:      instr.Pos(),
		File:     position.Filename,
		Line:     position.Line,
		Column:   position.Column,
		Severity: "warning",
		Message:  "Access to " + dataName + " requires lock " + mutexName + ", but it's not held",
	})
}

func reportAlreadyAcquiredLock(
	msg *ssa.Call,
	callee *ssa.Function,
	target string,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	if reporter == nil || fset == nil {
		logger.Infof("ERROR: Call to %s acquires lock %s, but it is already held\n", callee.Name(), target)
		return
	}

	position := fset.Position(msg.Pos())
	reporter.Warn(report.Diagnostic{
		Pos:      msg.Pos(),
		File:     position.Filename,
		Line:     position.Line,
		Column:   position.Column,
		Severity: "warning",
		Message:  "Call to " + callee.Name() + " acquires lock " + target + ", but it is already held",
	})
}

func reportReturnMissingLock(
	fn *ssa.Function,
	instr ssa.Instruction,
	target string,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	pos := returnDiagnosticPos(fn, instr)
	if pos == token.NoPos {
		return
	}

	if reporter == nil || fset == nil {
		logger.Infof("ERROR: Function %s returns without required lock %s held\n", fn.Name(), target)
		return
	}

	position := fset.Position(pos)
	reporter.Warn(report.Diagnostic{
		Pos:      pos,
		File:     position.Filename,
		Line:     position.Line,
		Column:   position.Column,
		Severity: "warning",
		Message:  "Function " + fn.Name() + " must return with lock " + target + " held",
	})
}

func reportUndeclaredReturnedLock(
	fn *ssa.Function,
	instr ssa.Instruction,
	heldLocks LockSet,
	reporter *report.Reporter,
	fset *token.FileSet,
) {
	pos := returnDiagnosticPos(fn, instr)
	if pos == token.NoPos {
		return
	}

	lockNames := make([]string, 0, len(heldLocks))
	for obj := range heldLocks {
		if obj == nil {
			continue
		}
		lockNames = append(lockNames, obj.Name())
	}
	sort.Strings(lockNames)

	locks := "unknown lock"
	if len(lockNames) > 0 {
		locks = strings.Join(lockNames, ", ")
	}

	if reporter == nil || fset == nil {
		logger.Infof("ERROR: Function %s returns lock(s) %s without declaring @returns(...)\n", fn.Name(), locks)
		return
	}

	position := fset.Position(pos)
	reporter.Warn(report.Diagnostic{
		Pos:      pos,
		File:     position.Filename,
		Line:     position.Line,
		Column:   position.Column,
		Severity: "warning",
		Message:  "Function " + fn.Name() + " returns lock(s) " + locks + " but no @returns(...) contract is declared",
	})
}
