package analyzer

import (
	"go/token"
	"gotsan/utils/logger"
	"gotsan/utils/report"

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
