package trade_test

import (
	"context"
	"sync"
)

// Order represents a trade request.
type Order struct {
	ID     int
	Amount float64
}

// OrderBook manages active trades.
type OrderBook struct {
	mu sync.Mutex
	// @guarded_by(mu)
	orders map[int]*Order
}

// Engine handles the high-level logic.
type Engine struct {
	stateMu sync.RWMutex
	// @guarded_by(stateMu)
	isRunning bool

	book *OrderBook
}

// Mutex in a lambda

// @requires(e.book.mu)
func (e *Engine) ProcessWithCallback(o *Order, callback func(float64)) {
	e.book.orders[o.ID] = o

	// The closure captures 'o'. If 'callback' modifies 'o.Amount'
	// later without the book lock, is it still "safe"?
	go func() {
		callback(o.Amount)
	}()
}

// Interfaces have an unknown type at compile time, how to handle?

type Auditor interface {
	// @requires(mu)
	Audit(book *OrderBook)
}

type RiskAuditor struct {
	mu sync.Mutex
}

func (ra *RiskAuditor) Audit(ob *OrderBook) {
	// Warning for acessing ob.orders without ob.mu
	_ = len(ob.orders)
}

// Does the analyzer know that
// once an object is sent over a channel,
// the sender should no longer access its guarded fields

func (e *Engine) Pipeline(ch chan *Order) {
	e.book.mu.Lock()
	order := e.book.orders[1]

	ch <- order // Ownership transferred

	// Technically, 'order' might be being modified by the receiver of 'ch' now.
	order.Amount = 0
	e.book.mu.Unlock()
}

// Checking if the analyzer detects a function calling itself or
// another function that tries to acquire a lock it already holds.

// @acquires(e.stateMu)
func (e *Engine) RecursiveStateCheck(depth int) {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if depth > 0 {
		// Warning for a recursive call will attempt to Lock() stateMu again -> Deadlock.
		e.RecursiveStateCheck(depth - 1)
	}
}

// @requires(e.book.mu)
func (e *Engine) SelectAudit(ctx context.Context) {
	select {
	case <-ctx.Done():
		// Path 1: Exit without doing anything.
		return
	default:
		// Path 2: Access guarded data.
		for _, o := range e.book.orders {
			_ = o.ID
		}
	}
}

func main() {
	e := &Engine{
		book: &OrderBook{orders: make(map[int]*Order)},
	}

	e.ProcessWithCallback(&Order{ID: 1, Amount: 100.0}, func(f float64) {})

	// Should give a warning about passing a concrete implementation to an interface that
	// violates its own @requires contract.
	var auditor Auditor = &RiskAuditor{}
	auditor.Audit(e.book)
}
