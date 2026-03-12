package loop_returns

import "sync"

var mu sync.Mutex
var mu2 sync.Mutex
var mu3 sync.Mutex
var mu4 sync.Mutex

func keepLooping(i int, limit int) bool {
	return i < limit
}

// loopBreakOrUnlock demonstrates loop iterations where some paths break with the lock held,
// while other paths unlock before an early return.
//
// @returns(mu)
func loopBreakOrUnlock(flag int) *sync.Mutex {
	for i := 0; i < 4; i++ {
		mu.Lock()

		if i == flag {
			break
		}

		mu.Unlock()

		if i == 2 {
			return &mu
		}
	}

	return &mu
}

// loopBreakWithLockHeld demonstrates a loop that breaks while preserving the lock state
// expected by the return contract.
//
// @returns(mu2)
func loopBreakWithLockHeld() *sync.Mutex {
	mu2.Lock()

	for i := 0; i < 3; i++ {
		if i == 1 {
			break
		}
	}

	return &mu2
}

// loopDynamicCondition demonstrates an unknown compile-time iteration count
// (`limit` is runtime input). Some paths unlock before returning, violating @returns.
//
// @returns(mu3)
func loopDynamicCondition(limit int, releaseEarly bool) *sync.Mutex {
	i := 0
	for keepLooping(i, limit) {
		mu3.Lock()

		if releaseEarly && i%2 == 0 {
			mu3.Unlock()
			return &mu3
		}

		if i > 100 {
			break
		}

		i++
	}

	return &mu3
}

// loopDynamicConditionSafe demonstrates unknown iteration count where all return
// paths preserve the lock required by @returns.
//
// @returns(mu4)
func loopDynamicConditionSafe(limit int) *sync.Mutex {
	mu4.Lock()

	i := 0
	for keepLooping(i, limit) {
		if i > 100 {
			break
		}
		i++
	}

	return &mu4
}
