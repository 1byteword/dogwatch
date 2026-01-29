package probe

import (
	"strings"
	"testing"
)

// Note: These tests focus on the protocol parsing functions which don't require
// eBPF/kernel access. The actual DBProbe that uses eBPF cannot be unit tested
// without root privileges and a running kernel with BPF support.

func TestLooksLikeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "printable ASCII",
			data:     []byte("SELECT * FROM users WHERE id = 1"),
			expected: true,
		},
		{
			name:     "SQL with whitespace",
			data:     []byte("SELECT\n\tid,\n\tname\nFROM users"),
			expected: true,
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: false,
		},
		{
			name:     "binary data",
			data:     []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07},
			expected: false,
		},
		{
			name:     "mostly printable with some control chars",
			data:     []byte("SELECT * FROM users\x00\x00"),
			expected: true, // 80% threshold should pass
		},
		{
			name:     "encrypted/random bytes",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, // PNG header
			expected: false,
		},
		{
			name:     "mixed printable threshold",
			data:     []byte("abc\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09"), // ~30% printable
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeText(tt.data)
			if got != tt.expected {
				t.Errorf("looksLikeText(%v) = %v, want %v", tt.data, got, tt.expected)
			}
		})
	}
}

func TestLooksLikeSQLQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		// Valid SQL queries
		{"SELECT query", []byte("SELECT * FROM users"), true},
		{"select lowercase", []byte("select id from orders"), true},
		{"INSERT query", []byte("INSERT INTO users VALUES (1, 'test')"), true},
		{"UPDATE query", []byte("UPDATE users SET name = 'foo'"), true},
		{"DELETE query", []byte("DELETE FROM orders WHERE id = 1"), true},
		{"CREATE query", []byte("CREATE TABLE test (id INT)"), true},
		{"ALTER query", []byte("ALTER TABLE users ADD COLUMN email TEXT"), true},
		{"DROP query", []byte("DROP TABLE temp"), true},
		{"BEGIN transaction", []byte("BEGIN TRANSACTION"), true},
		{"COMMIT", []byte("COMMIT"), true},
		{"ROLLBACK", []byte("ROLLBACK"), true},
		{"SET variable", []byte("SET autocommit = 1"), true},
		{"SHOW query", []byte("SHOW TABLES"), true},
		{"EXPLAIN query", []byte("EXPLAIN SELECT * FROM users"), true},
		{"TRUNCATE", []byte("TRUNCATE TABLE logs"), true},
		{"GRANT", []byte("GRANT SELECT ON users TO reader"), true},
		{"LOCK", []byte("LOCK TABLES users WRITE"), true},
		{"WITH CTE", []byte("WITH cte AS (SELECT 1) SELECT * FROM cte"), true},
		{"comment start", []byte("/* comment */ SELECT 1"), true},
		{"subquery", []byte("(SELECT id FROM users)"), true},
		{"USE database", []byte("USE production"), true},
		{"REPLACE", []byte("REPLACE INTO users VALUES (1, 'test')"), true},

		// Leading whitespace
		{"whitespace before SELECT", []byte("  SELECT 1"), true},
		{"newline before INSERT", []byte("\nINSERT INTO t VALUES (1)"), true},
		{"tab before UPDATE", []byte("\tUPDATE t SET x = 1"), true},

		// Invalid/non-SQL
		{"empty data", []byte(""), false},
		{"short data", []byte("AB"), false},
		{"binary data", []byte{0x00, 0x01, 0x02, 0x03}, false},
		{"random text", []byte("hello world"), false},
		{"number start", []byte("123 SELECT"), false},
		{"special char start", []byte("@variable"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeSQLQuery(tt.data)
			if got != tt.expected {
				t.Errorf("looksLikeSQLQuery(%q) = %v, want %v", string(tt.data), got, tt.expected)
			}
		})
	}
}

func TestIsAlphaOrUnderscore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		b        byte
		expected bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'_', true},
		{'0', false},
		{'9', false},
		{' ', false},
		{'-', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.b), func(t *testing.T) {
			got := isAlphaOrUnderscore(tt.b)
			if got != tt.expected {
				t.Errorf("isAlphaOrUnderscore(%c) = %v, want %v", tt.b, got, tt.expected)
			}
		})
	}
}

func TestTrimLeadingNulls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected []byte
	}{
		{"no nulls", []byte("SELECT 1"), []byte("SELECT 1")},
		{"leading nulls", []byte{0x00, 0x00, 'S', 'E', 'L'}, []byte("SEL")},
		{"control chars", []byte{0x01, 0x02, 0x03, 'A', 'B'}, []byte("AB")},
		{"all nulls", []byte{0x00, 0x00, 0x00}, []byte{}},
		{"empty", []byte{}, []byte{}},
		{"preserve tabs and newlines", []byte{'\t', 'A'}, []byte{'\t', 'A'}},
		{"preserve newlines", []byte{'\n', 'A'}, []byte{'\n', 'A'}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimLeadingNulls(tt.data)
			if string(got) != string(tt.expected) {
				t.Errorf("trimLeadingNulls(%v) = %v, want %v", tt.data, got, tt.expected)
			}
		})
	}
}

func TestExtractSQLOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query    string
		expected string
	}{
		{"SELECT * FROM users", "SELECT"},
		{"select id from orders", "SELECT"},
		{"INSERT INTO users VALUES (1)", "INSERT"},
		{"UPDATE users SET name = 'test'", "UPDATE"},
		{"DELETE FROM orders", "DELETE"},
		{"CREATE TABLE test (id INT)", "CREATE"},
		{"DROP TABLE temp", "DROP"},
		{"ALTER TABLE users ADD COLUMN x INT", "ALTER"},
		{"TRUNCATE TABLE logs", "TRUNCATE"},
		{"BEGIN", "BEGIN"},
		{"COMMIT", "COMMIT"},
		{"ROLLBACK", "ROLLBACK"},
		{"SET autocommit = 1", "SET"},
		{"SHOW TABLES", "SHOW"},
		{"EXPLAIN SELECT 1", "EXPLAIN"},
		{"USE database", "USE"},
		{"unknown query type", "QUERY"},
		{"", "QUERY"},
		{"  SELECT 1", "SELECT"}, // with leading whitespace
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := extractSQLOperation(tt.query)
			if got != tt.expected {
				t.Errorf("extractSQLOperation(%q) = %q, want %q", tt.query, got, tt.expected)
			}
		})
	}
}

func TestExtractTableName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		query    string
		expected string
	}{
		// FROM clause
		{"SELECT * FROM users", "USERS"},
		{"SELECT * FROM users WHERE id = 1", "USERS"},
		{"SELECT * FROM `users`", "USERS"},
		{"SELECT * FROM \"users\"", "USERS"},
		{"select id from orders join items", "ORDERS"},

		// INTO clause
		{"INSERT INTO users VALUES (1)", "USERS"},
		{"INSERT INTO `orders` (id) VALUES (1)", "ORDERS"},

		// UPDATE clause
		{"UPDATE users SET name = 'test'", "USERS"},
		{"UPDATE `products` SET price = 10", "PRODUCTS"},

		// No table detected
		{"SELECT 1", ""},
		{"SET autocommit = 1", ""},
		{"COMMIT", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := extractTableName(tt.query)
			if got != tt.expected {
				t.Errorf("extractTableName(%q) = %q, want %q", tt.query, got, tt.expected)
			}
		})
	}
}

func TestSanitizeQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "trim null bytes",
			query:    "SELECT 1\x00\x00",
			expected: "SELECT 1",
		},
		{
			name:     "trim whitespace",
			query:    "  SELECT 1  ",
			expected: "SELECT 1",
		},
		{
			name:     "collapse multiple spaces",
			query:    "SELECT  *   FROM    users",
			expected: "SELECT * FROM users",
		},
		{
			name:     "truncate long query",
			query:    strings.Repeat("x", 600),
			expected: strings.Repeat("x", 500) + "...",
		},
		{
			name:     "normal query unchanged",
			query:    "SELECT id, name FROM users WHERE active = 1",
			expected: "SELECT id, name FROM users WHERE active = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeQuery(tt.query)
			if got != tt.expected {
				t.Errorf("sanitizeQuery() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLooksLikeMySQLResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  []byte
		expected bool
	}{
		{
			name: "OK packet",
			// Length: 7, Seq: 1, Status: 0x00 (OK)
			payload:  []byte{0x07, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00},
			expected: true,
		},
		{
			name: "EOF packet",
			// Length: 5, Seq: 5, Status: 0xfe (EOF)
			payload:  []byte{0x05, 0x00, 0x00, 0x05, 0xfe, 0x00, 0x00},
			expected: true,
		},
		{
			name:     "too short",
			payload:  []byte{0x01, 0x00, 0x00},
			expected: false,
		},
		{
			name: "invalid packet length (too large)",
			// Length: max (0xFFFFFF), Seq: 0
			payload:  []byte{0xFF, 0xFF, 0xFF, 0x00, 0x00},
			expected: true, // This is actually valid in MySQL protocol
		},
		{
			name: "high sequence number",
			// Length: 1, Seq: 200
			payload:  []byte{0x01, 0x00, 0x00, 0xC8, 0x00},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeMySQLResponse(tt.payload)
			if got != tt.expected {
				t.Errorf("looksLikeMySQLResponse(%v) = %v, want %v", tt.payload, got, tt.expected)
			}
		})
	}
}

func TestLooksLikePostgresMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		payload  []byte
		expected bool
	}{
		{
			name: "Query message",
			// 'Q' + length (big-endian) + query
			payload:  []byte{'Q', 0x00, 0x00, 0x00, 0x10, 'S', 'E', 'L', 'E', 'C', 'T'},
			expected: true,
		},
		{
			name: "CommandComplete message",
			// 'C' + length
			payload:  []byte{'C', 0x00, 0x00, 0x00, 0x0B},
			expected: true,
		},
		{
			name: "RowDescription message",
			// 'T' + length
			payload:  []byte{'T', 0x00, 0x00, 0x00, 0x20},
			expected: true,
		},
		{
			name: "DataRow message",
			// 'D' + length
			payload:  []byte{'D', 0x00, 0x00, 0x00, 0x15},
			expected: true,
		},
		{
			name: "ErrorResponse message",
			// 'E' + length
			payload:  []byte{'E', 0x00, 0x00, 0x00, 0x50},
			expected: true,
		},
		{
			name: "ReadyForQuery message",
			// 'Z' + length
			payload:  []byte{'Z', 0x00, 0x00, 0x00, 0x05},
			expected: true,
		},
		{
			name:     "too short",
			payload:  []byte{'Q', 0x00, 0x00},
			expected: false,
		},
		{
			name: "invalid message type",
			// 'X' is not a valid message type
			payload:  []byte{'X', 0x00, 0x00, 0x00, 0x10},
			expected: false,
		},
		{
			name: "zero length",
			// Length of 0 is invalid
			payload:  []byte{'Q', 0x00, 0x00, 0x00, 0x00},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikePostgresMessage(tt.payload)
			if got != tt.expected {
				t.Errorf("looksLikePostgresMessage(%v) = %v, want %v", tt.payload, got, tt.expected)
			}
		})
	}
}

func TestParseRESPArray(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected []string
	}{
		{
			name:     "GET command",
			data:     []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"),
			expected: []string{"GET", "key"},
		},
		{
			name:     "SET command",
			data:     []byte("*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"),
			expected: []string{"SET", "key", "value"},
		},
		{
			name:     "HGET command",
			data:     []byte("*3\r\n$4\r\nHGET\r\n$4\r\nhash\r\n$5\r\nfield\r\n"),
			expected: []string{"HGET", "hash", "field"},
		},
		{
			name:     "PING command",
			data:     []byte("*1\r\n$4\r\nPING\r\n"),
			expected: []string{"PING"},
		},
		{
			name:     "empty array",
			data:     []byte("*0\r\n"),
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRESPArray(tt.data)
			if len(got) != len(tt.expected) {
				t.Errorf("parseRESPArray() len = %d, want %d", len(got), len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("parseRESPArray()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestParsePostgresError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name: "simple error message",
			// S (Severity) + null + M (Message) + null
			data:     []byte{'S', 'E', 'R', 'R', 'O', 'R', 0, 'M', 'd', 'i', 'v', 'i', 's', 'i', 'o', 'n', ' ', 'b', 'y', ' ', 'z', 'e', 'r', 'o', 0},
			expected: "division by zero",
		},
		{
			name:     "empty data",
			data:     []byte{},
			expected: "",
		},
		{
			name: "message field only",
			data: []byte{'M', 't', 'e', 's', 't', 0},
			expected: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePostgresError(tt.data)
			if got != tt.expected {
				t.Errorf("parsePostgresError() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestMySQLQueryParsing tests MySQL wire protocol query parsing
func TestMySQLQueryParsing(t *testing.T) {
	t.Parallel()

	// Create a mock probe just to access the parsing methods
	p := &DBProbe{}

	tests := []struct {
		name          string
		payload       []byte
		expectedOp    string
		expectedQuery string
		expectedTable string
	}{
		{
			name: "COM_QUERY SELECT",
			// Length: len("SELECT * FROM users") + 1, Seq: 0, Cmd: 0x03
			payload:       buildMySQLPacket(0x03, "SELECT * FROM users"),
			expectedOp:    "SELECT",
			expectedQuery: "SELECT * FROM users",
			expectedTable: "USERS",
		},
		{
			name:          "COM_QUERY INSERT",
			payload:       buildMySQLPacket(0x03, "INSERT INTO orders (id) VALUES (1)"),
			expectedOp:    "INSERT",
			expectedQuery: "INSERT INTO orders (id) VALUES (1)",
			expectedTable: "ORDERS",
		},
		{
			name:          "COM_STMT_PREPARE",
			payload:       buildMySQLPacket(0x16, "SELECT * FROM products WHERE id = ?"),
			expectedOp:    "PREPARE",
			expectedQuery: "SELECT * FROM products WHERE id = ?",
			expectedTable: "",
		},
		{
			name:          "COM_INIT_DB",
			payload:       buildMySQLPacket(0x02, "testdb"),
			expectedOp:    "USE",
			expectedQuery: "testdb",
			expectedTable: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &DBEvent{}
			raw := rawDBEvent{
				DBType:      DBTypeMySQL,
				PayloadSize: uint32(len(tt.payload)),
			}
			copy(raw.Payload[:], tt.payload)

			p.parseMySQLQuery(event, tt.payload)

			if event.Operation != tt.expectedOp {
				t.Errorf("Operation = %q, want %q", event.Operation, tt.expectedOp)
			}
			if event.Query != tt.expectedQuery {
				t.Errorf("Query = %q, want %q", event.Query, tt.expectedQuery)
			}
			if event.Table != tt.expectedTable {
				t.Errorf("Table = %q, want %q", event.Table, tt.expectedTable)
			}
		})
	}
}

// TestPostgresQueryParsing tests PostgreSQL wire protocol query parsing
func TestPostgresQueryParsing(t *testing.T) {
	t.Parallel()

	p := &DBProbe{}

	tests := []struct {
		name          string
		payload       []byte
		expectedOp    string
		expectedQuery string
		expectedTable string
	}{
		{
			name:          "Simple Query",
			payload:       buildPostgresPacket('Q', "SELECT * FROM users"),
			expectedOp:    "SELECT",
			expectedQuery: "SELECT * FROM users",
			expectedTable: "USERS",
		},
		{
			name:          "Insert Query",
			payload:       buildPostgresPacket('Q', "INSERT INTO orders VALUES (1)"),
			expectedOp:    "INSERT",
			expectedQuery: "INSERT INTO orders VALUES (1)",
			expectedTable: "ORDERS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &DBEvent{}

			p.parsePostgresQuery(event, tt.payload)

			if event.Operation != tt.expectedOp {
				t.Errorf("Operation = %q, want %q", event.Operation, tt.expectedOp)
			}
			if event.Query != tt.expectedQuery {
				t.Errorf("Query = %q, want %q", event.Query, tt.expectedQuery)
			}
			if event.Table != tt.expectedTable {
				t.Errorf("Table = %q, want %q", event.Table, tt.expectedTable)
			}
		})
	}
}

// TestRedisCommandParsing tests Redis RESP protocol command parsing
func TestRedisCommandParsing(t *testing.T) {
	t.Parallel()

	p := &DBProbe{}

	tests := []struct {
		name        string
		payload     []byte
		expectedOp  string
		expectedKey string
	}{
		{
			name:        "GET command",
			payload:     []byte("*2\r\n$3\r\nGET\r\n$8\r\nuser:123\r\n"),
			expectedOp:  "GET",
			expectedKey: "user:123",
		},
		{
			name:        "SET command",
			payload:     []byte("*3\r\n$3\r\nSET\r\n$8\r\nuser:123\r\n$5\r\nhello\r\n"),
			expectedOp:  "SET",
			expectedKey: "user:123",
		},
		{
			name:        "HGET command",
			payload:     []byte("*3\r\n$4\r\nHGET\r\n$6\r\nmyhash\r\n$5\r\nfield\r\n"),
			expectedOp:  "HGET",
			expectedKey: "myhash",
		},
		{
			name:        "inline PING",
			payload:     []byte("PING\r\n"),
			expectedOp:  "PING",
			expectedKey: "",
		},
		{
			name:        "inline GET",
			payload:     []byte("GET mykey\r\n"),
			expectedOp:  "GET",
			expectedKey: "mykey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &DBEvent{}

			p.parseRedisCommand(event, tt.payload)

			if event.Operation != tt.expectedOp {
				t.Errorf("Operation = %q, want %q", event.Operation, tt.expectedOp)
			}
			if event.Key != tt.expectedKey {
				t.Errorf("Key = %q, want %q", event.Key, tt.expectedKey)
			}
		})
	}
}

// TestRedisResponseParsing tests Redis RESP response parsing
func TestRedisResponseParsing(t *testing.T) {
	t.Parallel()

	p := &DBProbe{}

	tests := []struct {
		name       string
		payload    []byte
		expectedOp string
		expectErr  bool
	}{
		{
			name:       "OK response",
			payload:    []byte("+OK\r\n"),
			expectedOp: "OK",
		},
		{
			name:       "simple string",
			payload:    []byte("+PONG\r\n"),
			expectedOp: "OK",
		},
		{
			name:       "error response",
			payload:    []byte("-ERR unknown command\r\n"),
			expectedOp: "ERROR",
			expectErr:  true,
		},
		{
			name:       "integer response",
			payload:    []byte(":42\r\n"),
			expectedOp: "INT",
		},
		{
			name:       "bulk string",
			payload:    []byte("$6\r\nfoobar\r\n"),
			expectedOp: "BULK",
		},
		{
			name:       "array response",
			payload:    []byte("*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"),
			expectedOp: "ARRAY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &DBEvent{}

			p.parseRedisResponse(event, tt.payload)

			if event.Operation != tt.expectedOp {
				t.Errorf("Operation = %q, want %q", event.Operation, tt.expectedOp)
			}
			if tt.expectErr && event.Error == "" {
				t.Error("expected error message, got empty")
			}
		})
	}
}

// Helper functions to build test packets

func buildMySQLPacket(cmd byte, data string) []byte {
	pktLen := 1 + len(data) // cmd byte + data
	packet := make([]byte, 4+1+len(data))
	// Length (little-endian, 3 bytes)
	packet[0] = byte(pktLen & 0xFF)
	packet[1] = byte((pktLen >> 8) & 0xFF)
	packet[2] = byte((pktLen >> 16) & 0xFF)
	// Sequence number
	packet[3] = 0
	// Command
	packet[4] = cmd
	// Data
	copy(packet[5:], data)
	return packet
}

func buildPostgresPacket(msgType byte, query string) []byte {
	queryWithNull := query + "\x00"
	msgLen := 4 + len(queryWithNull) // length includes itself
	packet := make([]byte, 1+4+len(queryWithNull))
	packet[0] = msgType
	// Length (big-endian, 4 bytes)
	packet[1] = byte((msgLen >> 24) & 0xFF)
	packet[2] = byte((msgLen >> 16) & 0xFF)
	packet[3] = byte((msgLen >> 8) & 0xFF)
	packet[4] = byte(msgLen & 0xFF)
	copy(packet[5:], queryWithNull)
	return packet
}

// Benchmarks

func BenchmarkLooksLikeText(b *testing.B) {
	data := []byte("SELECT * FROM users WHERE id = 1 AND name = 'test' ORDER BY created_at DESC")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		looksLikeText(data)
	}
}

func BenchmarkLooksLikeSQLQuery(b *testing.B) {
	data := []byte("SELECT * FROM users WHERE id = 1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		looksLikeSQLQuery(data)
	}
}

func BenchmarkExtractTableName(b *testing.B) {
	query := "SELECT u.id, u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id WHERE o.status = 'active'"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractTableName(query)
	}
}

func BenchmarkParseRESPArray(b *testing.B) {
	data := []byte("*3\r\n$3\r\nSET\r\n$8\r\nuser:123\r\n$32\r\nsome-value-that-is-moderately-long\r\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseRESPArray(data)
	}
}
