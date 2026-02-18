package simple

import "sync"

// A simple example of a bank manager guarded by Mutex "mu" with corresponding annotations
// added to the program. Comments indicate areas that should eventually flag a warning when the
// the program is completed.

var (
	globalMu  sync.Mutex
	globalMu2 sync.Mutex
)

// @guarded_by(globalMu)
var (
	globalBalance int
	globalCount   int
)

var globalFlag bool // @guarded_by(globalMu2)

// Account represents a bank account.
type Account struct {
	mu  sync.Mutex
	mu2 sync.Mutex

	// @guarded_by(mu)
	balance int

	accountNumber int //@guarded_by(mu2)

	// A warning of an annotation that does not work in this context
	// @requires(mu)
	badExample int
}

// depositUnsafe adds money to balance without acquiring a lock.
//
// @requires(mu)
func (a *Account) depositUnsafe(amount int) {
	a.balance += amount
}

// Deposit safely adds money and releases the lock before returning.
//
// @acquires(mu)
func (a *Account) Deposit(amount int) {
	a.mu.Lock()         // acquires the lock
	defer a.mu.Unlock() // releases on function exit

	a.depositUnsafe(amount)
}

// DepositAndHold safely adds money and returns while keeping the lock held.
//
// @acquires(mu)
// @returns(mu)
func (a *Account) DepositAndHold(amount int) *Account {
	a.mu.Lock() // acquires the lock
	a.balance += amount
	return a // lock is still held for caller
}

// Balance safely returns the account balance, releasing the lock before returning.
//
// @acquires(mu)
func (a *Account) Balance() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.balance
}

// BadReadBalance demonstrates incorrect access to a guarded field.
//
// @acquires(mu)
func (a *Account) BadReadBalance() int {
	// Forgot to lock balance
	return a.balance // should trigger @guarded_by warning
}

// BadCallDepositUnsafe demonstrates calling a @requires function without holding the lock.
func (a *Account) BadCallDepositUnsafe(amount int) {
	// Forgot to lock before calling depositUnsafe
	a.depositUnsafe(amount) // should trigger @requires warning
}
