package lessSimple

import "sync"

type Vault struct {
	mu sync.RWMutex
	// @guarded_by(mu)
	totalAssets int
}

type Account struct {
	mu sync.Mutex
	// @guarded_by(mu)
	balance int
	id      int
}

type Bank struct {
	registryMu sync.Mutex
	// @guarded_by(registryMu)
	accounts map[int]*Account

	vault *Vault
}

// @requires(a.mu)
func (b *Bank) internalAudit(a *Account) {
	// 'a' is a pointer.
	_ = a.balance
}

// Transfer moves money between accounts, could lead to a deadlock.
// @requires(from.mu)
// @requires(to.mu)
func (b *Bank) TransferUnsafe(from *Account, to *Account, amount int) {
	from.balance -= amount
	to.balance += amount
}

// AuditAndLog checks assets and performs conditional locking.
// @acquires(b.vault.mu)
func (b *Bank) AuditAndLog() int {
	b.vault.mu.RLock()
	defer b.vault.mu.RUnlock()

	if b.vault.totalAssets < 0 {
		return -1
	}

	return b.vault.totalAssets
}

// Testing to see missing @requires(from.mu) and @requires(to.mu)
func (b *Bank) BadTransfer(from *Account, to *Account, amount int) {
	// This should fail because TransferUnsafe requires locks we haven't acquired
	b.TransferUnsafe(from, to, amount)
}

// No release of registryMu if account not found
// @acquires(b.registryMu)
func (b *Bank) InconsistentLock(id int) *Account {
	b.registryMu.Lock()
	acc, ok := b.accounts[id]
	if !ok {
		// returning without unlocking registryMu
		return nil
	}
	b.registryMu.Unlock()
	return acc
}

func main() {
	bank := &Bank{
		accounts: make(map[int]*Account),
		vault:    &Vault{},
	}

	acc1 := &Account{id: 1, balance: 500}

	// Should warn: calling a function that modifies guarded state without locking
	bank.internalAudit(acc1)
}
