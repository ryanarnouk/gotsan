package analyzer

import "go/types"

type LockSet map[types.Object]bool

func (ls LockSet) Copy() LockSet {
	newSet := make(LockSet)
	for k, v := range ls {
		newSet[k] = v
	}
	return newSet
}

func (ls LockSet) Equals(other LockSet) bool {
	if len(ls) != len(other) {
		return false
	}
	for k := range ls {
		if !other[k] {
			return false
		}
	}
	return true
}

func (ls LockSet) Intersect(other LockSet) LockSet {
	result := make(LockSet)
	for k := range ls {
		if other[k] {
			result[k] = true
		}
	}
	return result
}

type AnalysisState struct {
	HeldLocks       LockSet
	DeferredLocks   LockSet
	DeferredUnlocks LockSet
}

func newAnalysisState(initial LockSet) AnalysisState {
	return AnalysisState{
		HeldLocks:       initial.Copy(),
		DeferredLocks:   make(LockSet),
		DeferredUnlocks: make(LockSet),
	}
}

func (s AnalysisState) Copy() AnalysisState {
	return AnalysisState{
		HeldLocks:       s.HeldLocks.Copy(),
		DeferredLocks:   s.DeferredLocks.Copy(),
		DeferredUnlocks: s.DeferredUnlocks.Copy(),
	}
}

func (s AnalysisState) Equals(other AnalysisState) bool {
	return s.HeldLocks.Equals(other.HeldLocks) &&
		s.DeferredLocks.Equals(other.DeferredLocks) &&
		s.DeferredUnlocks.Equals(other.DeferredUnlocks)
}

func (s AnalysisState) Intersect(other AnalysisState) AnalysisState {
	return AnalysisState{
		HeldLocks:       s.HeldLocks.Intersect(other.HeldLocks),
		DeferredLocks:   s.DeferredLocks.Intersect(other.DeferredLocks),
		DeferredUnlocks: s.DeferredUnlocks.Intersect(other.DeferredUnlocks),
	}
}
