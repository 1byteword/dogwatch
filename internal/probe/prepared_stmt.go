package probe

import (
	"sync"
	"time"
)

// PreparedStatement represents a prepared statement
type PreparedStatement struct {
	ID        string    // Statement ID (MySQL: numeric, PostgreSQL: name)
	SQL       string    // The prepared SQL with placeholders
	CreatedAt time.Time
	UseCount  int64
}

// ConnectionPreparedStmts holds prepared statements for a connection
type ConnectionPreparedStmts struct {
	PID        uint32
	TID        uint32
	Statements map[string]*PreparedStatement // ID -> Statement
	LastUsed   time.Time
}

// PreparedStatementCache caches prepared statements per connection
type PreparedStatementCache struct {
	mu          sync.RWMutex
	connections map[string]*ConnectionPreparedStmts // "pid:tid" -> stmts
	maxAge      time.Duration
	maxConns    int
}

// NewPreparedStatementCache creates a new prepared statement cache
func NewPreparedStatementCache() *PreparedStatementCache {
	cache := &PreparedStatementCache{
		connections: make(map[string]*ConnectionPreparedStmts),
		maxAge:      30 * time.Minute, // Expire connections after 30 min idle
		maxConns:    10000,            // Max connections to track
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// connKey generates a unique key for a connection
func connKey(pid, tid uint32) string {
	return string(rune(pid)) + ":" + string(rune(tid))
}

// StorePrepared stores a prepared statement
func (c *PreparedStatementCache) StorePrepared(pid, tid uint32, stmtID, sql string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := connKey(pid, tid)
	conn, ok := c.connections[key]
	if !ok {
		conn = &ConnectionPreparedStmts{
			PID:        pid,
			TID:        tid,
			Statements: make(map[string]*PreparedStatement),
		}
		c.connections[key] = conn
	}

	conn.Statements[stmtID] = &PreparedStatement{
		ID:        stmtID,
		SQL:       sql,
		CreatedAt: time.Now(),
	}
	conn.LastUsed = time.Now()
}

// GetPrepared retrieves a prepared statement
func (c *PreparedStatementCache) GetPrepared(pid, tid uint32, stmtID string) *PreparedStatement {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := connKey(pid, tid)
	conn, ok := c.connections[key]
	if !ok {
		return nil
	}

	stmt := conn.Statements[stmtID]
	if stmt != nil {
		stmt.UseCount++
		conn.LastUsed = time.Now()
	}

	return stmt
}

// RemovePrepared removes a prepared statement (e.g., DEALLOCATE)
func (c *PreparedStatementCache) RemovePrepared(pid, tid uint32, stmtID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := connKey(pid, tid)
	conn, ok := c.connections[key]
	if ok {
		delete(conn.Statements, stmtID)
	}
}

// ClearConnection removes all statements for a connection
func (c *PreparedStatementCache) ClearConnection(pid, tid uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := connKey(pid, tid)
	delete(c.connections, key)
}

// Stats returns cache statistics
func (c *PreparedStatementCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	totalStmts := 0
	for _, conn := range c.connections {
		totalStmts += len(conn.Statements)
	}

	return map[string]interface{}{
		"connections":         len(c.connections),
		"total_statements":    totalStmts,
		"max_connections":     c.maxConns,
		"max_age_minutes":     c.maxAge.Minutes(),
	}
}

// cleanupLoop periodically removes old connections
func (c *PreparedStatementCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

func (c *PreparedStatementCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, conn := range c.connections {
		if now.Sub(conn.LastUsed) > c.maxAge {
			delete(c.connections, key)
		}
	}

	// If still over limit, remove oldest
	if len(c.connections) > c.maxConns {
		// Find and remove oldest
		var oldestKey string
		var oldestTime time.Time
		first := true

		for key, conn := range c.connections {
			if first || conn.LastUsed.Before(oldestTime) {
				oldestKey = key
				oldestTime = conn.LastUsed
				first = false
			}
		}

		if oldestKey != "" {
			delete(c.connections, oldestKey)
		}
	}
}

// ExtractMySQLStmtID extracts statement ID from COM_STMT_EXECUTE payload
// MySQL statement IDs are 4-byte little-endian integers
func ExtractMySQLStmtID(payload []byte) (string, bool) {
	// COM_STMT_EXECUTE format:
	// 1 byte: command (0x17)
	// 4 bytes: statement ID (little-endian)
	// 1 byte: flags
	// 4 bytes: iteration count
	// ... params if any
	if len(payload) < 9 {
		return "", false
	}

	// Skip the packet header (3 bytes length + 1 byte seq) and command byte
	stmtID := uint32(payload[5]) | uint32(payload[6])<<8 | uint32(payload[7])<<16 | uint32(payload[8])<<24
	return string(rune(stmtID)), true
}

// ExtractMySQLPrepareStmtID extracts statement ID from OK response to COM_STMT_PREPARE
// Response format: OK packet with statement ID
func ExtractMySQLPrepareStmtID(payload []byte) (string, bool) {
	// COM_STMT_PREPARE response:
	// 3 bytes: packet length
	// 1 byte: sequence
	// 1 byte: status (0x00 for OK)
	// 4 bytes: statement ID
	// 2 bytes: num columns
	// 2 bytes: num params
	// 1 byte: reserved
	// 2 bytes: warning count
	if len(payload) < 13 {
		return "", false
	}

	if payload[4] != 0x00 { // Not OK
		return "", false
	}

	stmtID := uint32(payload[5]) | uint32(payload[6])<<8 | uint32(payload[7])<<16 | uint32(payload[8])<<24
	return string(rune(stmtID)), true
}

// ExtractPostgresStmtName extracts statement name from Parse message
// PostgreSQL uses named prepared statements
func ExtractPostgresStmtName(payload []byte) (string, string, bool) {
	// Parse message format:
	// 1 byte: 'P'
	// 4 bytes: length
	// string: statement name (null-terminated)
	// string: query (null-terminated)
	// 2 bytes: num params
	// ... param types
	if len(payload) < 7 || payload[0] != 'P' {
		return "", "", false
	}

	// Find statement name (null-terminated)
	nameStart := 5
	nameEnd := nameStart
	for nameEnd < len(payload) && payload[nameEnd] != 0 {
		nameEnd++
	}
	if nameEnd >= len(payload) {
		return "", "", false
	}

	name := string(payload[nameStart:nameEnd])

	// Find query (null-terminated, starts after name's null terminator)
	queryStart := nameEnd + 1
	queryEnd := queryStart
	for queryEnd < len(payload) && payload[queryEnd] != 0 {
		queryEnd++
	}

	query := ""
	if queryEnd > queryStart {
		query = string(payload[queryStart:queryEnd])
	}

	return name, query, true
}

// ExtractPostgresExecuteStmtName extracts statement name from Execute message
func ExtractPostgresExecuteStmtName(payload []byte) (string, bool) {
	// Execute message format:
	// 1 byte: 'E'
	// 4 bytes: length
	// string: portal name (null-terminated)
	// 4 bytes: max rows
	if len(payload) < 6 || payload[0] != 'E' {
		return "", false
	}

	// Portal name is null-terminated
	nameStart := 5
	nameEnd := nameStart
	for nameEnd < len(payload) && payload[nameEnd] != 0 {
		nameEnd++
	}

	return string(payload[nameStart:nameEnd]), true
}
