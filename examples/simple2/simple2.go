package main

import "sync"

type Account struct {
	mu sync.Mutex
	// @guarded_by(mu)
	balance int
}

type Registry struct {
	mu       sync.Mutex
	accounts map[int]*Account
}

// Case 1: The "Happy Path"
// Should produce NO errors.
func (a *Account) Deposit(amount int) {
	a.mu.Lock()
	a.balance += amount // Safe: mu is held
	a.mu.Unlock()
}

// Case 2: The "Inconsistent Path" (The SSA example we discussed)
// Should error: "Lock registryMu not released on return path"
func (r *Registry) GetAccount(id int) *Account {
	r.mu.Lock()
	acc, ok := r.accounts[id]
	if !ok {
		// BUG: Returns without unlocking!
		return nil
	}
	r.mu.Unlock()
	return acc
}

// Case 3: The "Helper Function"
// @requires(mu)
// Should produce NO errors because initialLS pre-loads the lock.
func (a *Account) updateBalanceUnsafe(amount int) {
	a.balance += amount // Safe ONLY if @requires(mu) works
}

// Case 4: The "Bad Caller"
// Should error: "Function updateBalanceUnsafe requires lock mu which is not held"
func (a *Account) RiskyUpdate(amount int) {
	// BUG: Calls a @requires function without locking first
	a.updateBalanceUnsafe(amount)
}

func main() {}
