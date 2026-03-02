package analyzer

import (
	"go/types"

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
