// Package testutil provides test fixtures for dogwatch tests.
package testutil

import (
	"time"
)

// SystemMetricFixture represents test data for system metrics.
type SystemMetricFixture struct {
	CPUPercent  float64
	MemPercent  float64
	DiskReadPS  float64
	DiskWritePS float64
	NetRxPS     float64
	NetTxPS     float64
	Load1       float64
}

// DefaultSystemMetric returns a typical system metric fixture.
func DefaultSystemMetric() SystemMetricFixture {
	return SystemMetricFixture{
		CPUPercent:  45.5,
		MemPercent:  60.2,
		DiskReadPS:  1024.0,
		DiskWritePS: 512.0,
		NetRxPS:     2048.0,
		NetTxPS:     1024.0,
		Load1:       2.5,
	}
}

// HighLoadSystemMetric returns a high-load system metric fixture.
func HighLoadSystemMetric() SystemMetricFixture {
	return SystemMetricFixture{
		CPUPercent:  95.0,
		MemPercent:  90.0,
		DiskReadPS:  50000.0,
		DiskWritePS: 25000.0,
		NetRxPS:     100000.0,
		NetTxPS:     50000.0,
		Load1:       10.0,
	}
}

// SpanFixture represents test data for a trace span.
type SpanFixture struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	ServiceName  string
	Kind         string
	StartTime    time.Time
	EndTime      time.Time
	DurationMs   float64
	Status       string
	StatusMsg    string
	Attributes   map[string]string
}

// DefaultSpan returns a typical span fixture.
func DefaultSpan() SpanFixture {
	now := time.Now()
	return SpanFixture{
		TraceID:      "abc123def456",
		SpanID:       "span001",
		ParentSpanID: "",
		Name:         "GET /api/users",
		ServiceName:  "user-service",
		Kind:         "SERVER",
		StartTime:    now,
		EndTime:      now.Add(50 * time.Millisecond),
		DurationMs:   50.0,
		Status:       "OK",
		StatusMsg:    "",
		Attributes: map[string]string{
			"http.method":      "GET",
			"http.url":         "/api/users",
			"http.status_code": "200",
		},
	}
}

// ErrorSpan returns a span fixture representing an error.
func ErrorSpan() SpanFixture {
	now := time.Now()
	return SpanFixture{
		TraceID:      "abc123def456",
		SpanID:       "span002",
		ParentSpanID: "span001",
		Name:         "database.query",
		ServiceName:  "user-service",
		Kind:         "CLIENT",
		StartTime:    now,
		EndTime:      now.Add(100 * time.Millisecond),
		DurationMs:   100.0,
		Status:       "ERROR",
		StatusMsg:    "connection timeout",
		Attributes: map[string]string{
			"db.system":    "postgresql",
			"db.statement": "SELECT * FROM users WHERE id = ?",
			"error":        "true",
		},
	}
}

// AlertRuleFixture represents test data for an alert rule.
type AlertRuleFixture struct {
	ID          string
	Name        string
	Description string
	Type        string
	Enabled     bool
	Query       string
	Condition   string
	Threshold   float64
}

// DefaultAlertRule returns a typical alert rule fixture.
func DefaultAlertRule() AlertRuleFixture {
	return AlertRuleFixture{
		ID:          "rule_001",
		Name:        "High CPU Usage",
		Description: "Alert when CPU usage exceeds 90%",
		Type:        "threshold",
		Enabled:     true,
		Query:       "system.cpu.percent",
		Condition:   "gt",
		Threshold:   90.0,
	}
}

// DisabledAlertRule returns a disabled alert rule fixture.
func DisabledAlertRule() AlertRuleFixture {
	return AlertRuleFixture{
		ID:          "rule_002",
		Name:        "Memory Warning",
		Description: "Warn when memory usage exceeds 80%",
		Type:        "threshold",
		Enabled:     false,
		Query:       "system.memory.percent",
		Condition:   "gte",
		Threshold:   80.0,
	}
}

// UserFixture represents test data for a user.
type UserFixture struct {
	ID       string
	OrgID    string
	Email    string
	Password string
	Name     string
	Role     string
	IsActive bool
}

// DefaultUser returns a typical user fixture.
func DefaultUser() UserFixture {
	return UserFixture{
		ID:       "user_001",
		OrgID:    "org_001",
		Email:    "test@example.com",
		Password: "securepassword123",
		Name:     "Test User",
		Role:     "editor",
		IsActive: true,
	}
}

// AdminUser returns an admin user fixture.
func AdminUser() UserFixture {
	return UserFixture{
		ID:       "user_admin",
		OrgID:    "org_001",
		Email:    "admin@example.com",
		Password: "adminpassword456",
		Name:     "Admin User",
		Role:     "admin",
		IsActive: true,
	}
}

// OwnerUser returns an owner user fixture.
func OwnerUser() UserFixture {
	return UserFixture{
		ID:       "user_owner",
		OrgID:    "org_001",
		Email:    "owner@example.com",
		Password: "ownerpassword789",
		Name:     "Owner User",
		Role:     "owner",
		IsActive: true,
	}
}

// ViewerUser returns a viewer user fixture.
func ViewerUser() UserFixture {
	return UserFixture{
		ID:       "user_viewer",
		OrgID:    "org_001",
		Email:    "viewer@example.com",
		Password: "viewerpassword000",
		Name:     "Viewer User",
		Role:     "viewer",
		IsActive: true,
	}
}

// MySQLQueryFixture represents test data for a MySQL query event.
type MySQLQueryFixture struct {
	Query     string
	Operation string
	Table     string
	Payload   []byte
}

// SimpleMySQLSelect returns a simple SELECT query fixture.
func SimpleMySQLSelect() MySQLQueryFixture {
	query := "SELECT * FROM users WHERE id = 123"
	// MySQL wire protocol: 3-byte length + 1-byte seq + 1-byte cmd + query
	payload := make([]byte, 5+len(query))
	// Length (little-endian, 3 bytes)
	pktLen := 1 + len(query) // cmd byte + query
	payload[0] = byte(pktLen & 0xFF)
	payload[1] = byte((pktLen >> 8) & 0xFF)
	payload[2] = byte((pktLen >> 16) & 0xFF)
	// Sequence number
	payload[3] = 0
	// COM_QUERY command
	payload[4] = 0x03
	// Query string
	copy(payload[5:], query)

	return MySQLQueryFixture{
		Query:     query,
		Operation: "SELECT",
		Table:     "USERS",
		Payload:   payload,
	}
}

// MySQLInsert returns an INSERT query fixture.
func MySQLInsert() MySQLQueryFixture {
	query := "INSERT INTO orders (user_id, amount) VALUES (1, 100.00)"
	payload := make([]byte, 5+len(query))
	pktLen := 1 + len(query)
	payload[0] = byte(pktLen & 0xFF)
	payload[1] = byte((pktLen >> 8) & 0xFF)
	payload[2] = byte((pktLen >> 16) & 0xFF)
	payload[3] = 0
	payload[4] = 0x03
	copy(payload[5:], query)

	return MySQLQueryFixture{
		Query:     query,
		Operation: "INSERT",
		Table:     "ORDERS",
		Payload:   payload,
	}
}

// PostgresQueryFixture represents test data for a PostgreSQL query event.
type PostgresQueryFixture struct {
	Query     string
	Operation string
	Table     string
	Payload   []byte
}

// SimplePostgresSelect returns a simple SELECT query fixture for PostgreSQL.
func SimplePostgresSelect() PostgresQueryFixture {
	query := "SELECT * FROM products WHERE price > 50"
	// PostgreSQL wire protocol: 'Q' + 4-byte length (big-endian) + query + null
	queryWithNull := query + "\x00"
	msgLen := 4 + len(queryWithNull) // length includes itself
	payload := make([]byte, 1+4+len(queryWithNull))
	payload[0] = 'Q'
	payload[1] = byte((msgLen >> 24) & 0xFF)
	payload[2] = byte((msgLen >> 16) & 0xFF)
	payload[3] = byte((msgLen >> 8) & 0xFF)
	payload[4] = byte(msgLen & 0xFF)
	copy(payload[5:], queryWithNull)

	return PostgresQueryFixture{
		Query:     query,
		Operation: "SELECT",
		Table:     "PRODUCTS",
		Payload:   payload,
	}
}

// RedisCommandFixture represents test data for a Redis command event.
type RedisCommandFixture struct {
	Command   string
	Key       string
	Operation string
	Payload   []byte
}

// RedisGet returns a GET command fixture.
func RedisGet() RedisCommandFixture {
	// RESP array format: *2\r\n$3\r\nGET\r\n$8\r\nuser:123\r\n
	payload := []byte("*2\r\n$3\r\nGET\r\n$8\r\nuser:123\r\n")
	return RedisCommandFixture{
		Command:   "GET user:123",
		Key:       "user:123",
		Operation: "GET",
		Payload:   payload,
	}
}

// RedisSet returns a SET command fixture.
func RedisSet() RedisCommandFixture {
	// RESP array format: *3\r\n$3\r\nSET\r\n$8\r\nuser:123\r\n$5\r\nhello\r\n
	payload := []byte("*3\r\n$3\r\nSET\r\n$8\r\nuser:123\r\n$5\r\nhello\r\n")
	return RedisCommandFixture{
		Command:   "SET user:123 hello",
		Key:       "user:123",
		Operation: "SET",
		Payload:   payload,
	}
}

// LogEntryFixture represents test data for a log entry.
type LogEntryFixture struct {
	Timestamp time.Time
	Level     string
	Service   string
	Message   string
	Host      string
	Tags      map[string]string
}

// InfoLog returns an INFO log entry fixture.
func InfoLog() LogEntryFixture {
	return LogEntryFixture{
		Timestamp: time.Now(),
		Level:     "info",
		Service:   "api-gateway",
		Message:   "Request processed successfully",
		Host:      "server-01",
		Tags: map[string]string{
			"request_id": "req-12345",
			"method":     "GET",
			"path":       "/api/health",
		},
	}
}

// ErrorLog returns an ERROR log entry fixture.
func ErrorLog() LogEntryFixture {
	return LogEntryFixture{
		Timestamp: time.Now(),
		Level:     "error",
		Service:   "payment-service",
		Message:   "Failed to process payment: card declined",
		Host:      "server-02",
		Tags: map[string]string{
			"request_id": "req-67890",
			"error_code": "CARD_DECLINED",
			"user_id":    "usr-456",
		},
	}
}
