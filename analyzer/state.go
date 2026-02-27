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
