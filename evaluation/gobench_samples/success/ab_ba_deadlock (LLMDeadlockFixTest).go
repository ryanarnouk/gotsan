/*
 * Example: AB-BA Deadlock Pattern
 * Description: Similar to cockroach#10214, this demonstrates an AB-BA deadlock
 * where two goroutines acquire locks in opposite order:
 * - Thread 1: Acquires LockA (database), then tries to acquire LockB (connection)
 * - Thread 2: Acquires LockB (connection), then tries to acquire LockA (database)
 */
package gobench_samples

import (
	"sync"
	"testing"
)

type Database struct {
	dbMu struct {
		sync.Mutex
		connections []*Connection
	}
}

type Connection struct {
	connMu sync.Mutex
	db     *Database
}

// Thread 1 path: acquires dbMu first, then tries to acquire connMu
// @acquires(d.dbMu)
func (d *Database) processConnections() {
	d.dbMu.Lock()         // LockA acquire
	defer d.dbMu.Unlock() // LockA release
	
	for _, conn := range d.dbMu.connections {
		conn.sendQuery() // tries to acquire connMu
	}
}

// @acquires(c.connMu)
func (c *Connection) sendQuery() {
	c.connMu.Lock() // LockB acquire
	defer c.connMu.Unlock()
	// Simulate query execution
}

// Thread 2 path: acquires connMu first, then tries to acquire dbMu
// @acquires(c.connMu)
func (c *Connection) handleResponse() {
	c.connMu.Lock() // LockB acquire
	defer c.connMu.Unlock()
	c.notifyDatabase() // tries to acquire dbMu
}

// @acquires(c.db.dbMu)
func (c *Connection) notifyDatabase() {
	c.db.dbMu.Lock() // LockA acquire
	// This will deadlock if processConnections holds dbMu
	defer c.db.dbMu.Unlock()
}

func TestABBADeadlock(t *testing.T) {
	db := &Database{}
	db.dbMu.connections = make([]*Connection, 2)
	
	conn1 := &Connection{
		db: db,
	}
	conn2 := &Connection{
		db: db,
	}
	db.dbMu.connections[0] = conn1
	db.dbMu.connections[1] = conn2

	// Thread 1: Acquires dbMu, then tries to acquire connMu
	go func() {
		db.processConnections()
	}()

	// Thread 2: Acquires connMu, then tries to acquire dbMu
	go func() {
		conn1.handleResponse()
	}()
}
