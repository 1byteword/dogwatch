package probe

import (
	"testing"
)

func TestPreparedStatementCache_Basic(t *testing.T) {
	cache := NewPreparedStatementCache()

	// Store a prepared statement
	cache.StorePrepared(1234, 5678, "stmt1", "SELECT * FROM users WHERE id = ?")

	// Retrieve it
	stmt := cache.GetPrepared(1234, 5678, "stmt1")
	if stmt == nil {
		t.Fatal("Expected to find prepared statement")
	}
	if stmt.SQL != "SELECT * FROM users WHERE id = ?" {
		t.Errorf("SQL mismatch: got %q", stmt.SQL)
	}
	if stmt.UseCount != 1 {
		t.Errorf("UseCount should be 1, got %d", stmt.UseCount)
	}

	// Get again - use count should increase
	stmt = cache.GetPrepared(1234, 5678, "stmt1")
	if stmt.UseCount != 2 {
		t.Errorf("UseCount should be 2, got %d", stmt.UseCount)
	}

	// Non-existent statement
	stmt = cache.GetPrepared(1234, 5678, "nonexistent")
	if stmt != nil {
		t.Error("Should not find non-existent statement")
	}

	// Wrong PID/TID
	stmt = cache.GetPrepared(9999, 5678, "stmt1")
	if stmt != nil {
		t.Error("Should not find statement for wrong PID")
	}
}

func TestPreparedStatementCache_MultipleStatements(t *testing.T) {
	cache := NewPreparedStatementCache()

	// Store multiple statements for same connection
	cache.StorePrepared(100, 200, "s1", "SELECT 1")
	cache.StorePrepared(100, 200, "s2", "SELECT 2")
	cache.StorePrepared(100, 200, "s3", "SELECT 3")

	// Store for different connection
	cache.StorePrepared(101, 200, "s1", "INSERT INTO t VALUES (?)")

	// Verify all are retrievable
	if stmt := cache.GetPrepared(100, 200, "s1"); stmt == nil || stmt.SQL != "SELECT 1" {
		t.Error("s1 for conn 100:200 not found or wrong")
	}
	if stmt := cache.GetPrepared(100, 200, "s2"); stmt == nil || stmt.SQL != "SELECT 2" {
		t.Error("s2 for conn 100:200 not found or wrong")
	}
	if stmt := cache.GetPrepared(101, 200, "s1"); stmt == nil || stmt.SQL != "INSERT INTO t VALUES (?)" {
		t.Error("s1 for conn 101:200 not found or wrong")
	}

	stats := cache.Stats()
	if stats["connections"].(int) != 2 {
		t.Errorf("Expected 2 connections, got %d", stats["connections"].(int))
	}
	if stats["total_statements"].(int) != 4 {
		t.Errorf("Expected 4 total statements, got %d", stats["total_statements"].(int))
	}
}

func TestPreparedStatementCache_Remove(t *testing.T) {
	cache := NewPreparedStatementCache()

	cache.StorePrepared(100, 200, "stmt1", "SELECT 1")
	cache.StorePrepared(100, 200, "stmt2", "SELECT 2")

	// Remove one
	cache.RemovePrepared(100, 200, "stmt1")

	if stmt := cache.GetPrepared(100, 200, "stmt1"); stmt != nil {
		t.Error("stmt1 should be removed")
	}
	if stmt := cache.GetPrepared(100, 200, "stmt2"); stmt == nil {
		t.Error("stmt2 should still exist")
	}
}

func TestPreparedStatementCache_ClearConnection(t *testing.T) {
	cache := NewPreparedStatementCache()

	cache.StorePrepared(100, 200, "stmt1", "SELECT 1")
	cache.StorePrepared(100, 200, "stmt2", "SELECT 2")
	cache.StorePrepared(101, 200, "stmt1", "SELECT 3")

	// Clear one connection
	cache.ClearConnection(100, 200)

	if stmt := cache.GetPrepared(100, 200, "stmt1"); stmt != nil {
		t.Error("Connection 100:200 should be cleared")
	}
	if stmt := cache.GetPrepared(101, 200, "stmt1"); stmt == nil {
		t.Error("Connection 101:200 should still exist")
	}
}

func TestExtractMySQLStmtID(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		wantID   string
		wantOK   bool
	}{
		{
			name: "valid execute packet",
			// Header (4) + command (1) + stmt_id (4)
			payload: []byte{
				0x09, 0x00, 0x00, 0x00, // length (doesn't matter for test)
				0x17,                   // COM_STMT_EXECUTE
				0x01, 0x00, 0x00, 0x00, // stmt_id = 1
			},
			wantID: "\x01",
			wantOK: true,
		},
		{
			name: "stmt_id 42",
			payload: []byte{
				0x09, 0x00, 0x00, 0x00,
				0x17,
				0x2A, 0x00, 0x00, 0x00, // stmt_id = 42
			},
			wantID: "*", // rune(42) = '*'
			wantOK: true,
		},
		{
			name:    "too short",
			payload: []byte{0x09, 0x00, 0x00, 0x00, 0x17},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := ExtractMySQLStmtID(tt.payload)
			if gotOK != tt.wantOK {
				t.Errorf("ExtractMySQLStmtID() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && gotID != tt.wantID {
				t.Errorf("ExtractMySQLStmtID() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

func TestExtractMySQLPrepareStmtID(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		wantID  string
		wantOK  bool
	}{
		{
			name: "valid OK response",
			// Header (4) + status (1) + stmt_id (4)
			payload: []byte{
				0x0C, 0x00, 0x00, 0x01, // length + seq
				0x00,                   // OK status
				0x05, 0x00, 0x00, 0x00, // stmt_id = 5
				0x00, 0x00, // num columns
				0x01, 0x00, // num params
			},
			wantID: "\x05",
			wantOK: true,
		},
		{
			name: "error response",
			payload: []byte{
				0x05, 0x00, 0x00, 0x01,
				0xFF, // ERROR status
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00,
			},
			wantOK: false,
		},
		{
			name:    "too short",
			payload: []byte{0x05, 0x00, 0x00, 0x01, 0x00},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := ExtractMySQLPrepareStmtID(tt.payload)
			if gotOK != tt.wantOK {
				t.Errorf("ExtractMySQLPrepareStmtID() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && gotID != tt.wantID {
				t.Errorf("ExtractMySQLPrepareStmtID() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

func TestExtractPostgresStmtName(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		wantName  string
		wantQuery string
		wantOK    bool
	}{
		{
			name: "valid Parse message",
			// 'P' + length (4) + name (null-term) + query (null-term)
			payload: append(append(append(
				[]byte{'P', 0x00, 0x00, 0x00, 0x20}, // header
				[]byte("stmt1\x00")...),            // statement name
				[]byte("SELECT * FROM users\x00")...), // query
				[]byte{0x00, 0x00}...), // param count
			wantName:  "stmt1",
			wantQuery: "SELECT * FROM users",
			wantOK:    true,
		},
		{
			name: "unnamed statement",
			payload: append(append(
				[]byte{'P', 0x00, 0x00, 0x00, 0x15},
				[]byte("\x00")...), // empty name
				[]byte("SELECT 1\x00")...),
			wantName:  "",
			wantQuery: "SELECT 1",
			wantOK:    true,
		},
		{
			name:    "wrong message type",
			payload: []byte{'Q', 0x00, 0x00, 0x00, 0x10, 's', 't', 'm', 't', 0x00},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotQuery, gotOK := ExtractPostgresStmtName(tt.payload)
			if gotOK != tt.wantOK {
				t.Errorf("ExtractPostgresStmtName() ok = %v, want %v", gotOK, tt.wantOK)
				return
			}
			if !gotOK {
				return
			}
			if gotName != tt.wantName {
				t.Errorf("ExtractPostgresStmtName() name = %q, want %q", gotName, tt.wantName)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("ExtractPostgresStmtName() query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestExtractPostgresExecuteStmtName(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		wantID  string
		wantOK  bool
	}{
		{
			name: "valid Execute message",
			payload: append(
				[]byte{'E', 0x00, 0x00, 0x00, 0x0D},
				append([]byte("portal1\x00"), []byte{0x00, 0x00, 0x00, 0x00}...)...),
			wantID: "portal1",
			wantOK: true,
		},
		{
			name: "unnamed portal",
			payload: append(
				[]byte{'E', 0x00, 0x00, 0x00, 0x09},
				[]byte("\x00\x00\x00\x00\x00")...),
			wantID: "",
			wantOK: true,
		},
		{
			name:    "wrong message type",
			payload: []byte{'Q', 0x00, 0x00, 0x00, 0x05, 0x00},
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := ExtractPostgresExecuteStmtName(tt.payload)
			if gotOK != tt.wantOK {
				t.Errorf("ExtractPostgresExecuteStmtName() ok = %v, want %v", gotOK, tt.wantOK)
				return
			}
			if gotOK && gotID != tt.wantID {
				t.Errorf("ExtractPostgresExecuteStmtName() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}
