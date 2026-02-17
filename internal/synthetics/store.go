package synthetics

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CheckType represents the type of synthetic check
type CheckType string

const (
	CheckHTTP CheckType = "http"
	CheckTCP  CheckType = "tcp"
	CheckDNS  CheckType = "dns"
)

// CheckStatus represents the current status of a check
type CheckStatus string

const (
	StatusUp      CheckStatus = "up"
	StatusDown    CheckStatus = "down"
	StatusDegraded CheckStatus = "degraded"
	StatusUnknown CheckStatus = "unknown"
)

// Check represents a synthetic monitoring check
type Check struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	ServiceID   string            `json:"service_id,omitempty"` // Link to service catalog
	Type        CheckType         `json:"type"`
	URL         string            `json:"url"`
	Method      string            `json:"method,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	Interval    int               `json:"interval"`         // seconds between checks
	Timeout     int               `json:"timeout"`          // request timeout in seconds
	Assertions  []Assertion       `json:"assertions"`       // validation rules
	Channels    []string          `json:"channels"`         // notification channel IDs
	Enabled     bool              `json:"enabled"`
	Status      CheckStatus       `json:"status"`
	LastCheck   time.Time         `json:"last_check"`
	LastLatency int64             `json:"last_latency_ms"`  // last response time in ms
	Created     time.Time         `json:"created"`
	Updated     time.Time         `json:"updated"`
}

// Assertion defines a validation rule for check responses
type Assertion struct {
	Type     string `json:"type"`     // status_code, body_contains, response_time, header
	Operator string `json:"operator"` // equals, contains, less_than, greater_than
	Target   string `json:"target"`   // header name for header assertions
	Value    string `json:"value"`    // expected value
}

// CheckResult represents a single check execution result
type CheckResult struct {
	ID         string      `json:"id"`
	CheckID    string      `json:"check_id"`
	Timestamp  time.Time   `json:"timestamp"`
	Status     CheckStatus `json:"status"`
	LatencyMs  int64       `json:"latency_ms"`
	StatusCode int         `json:"status_code,omitempty"`
	Error      string      `json:"error,omitempty"`
	Body       string      `json:"body,omitempty"` // truncated response body
}

// UptimeStats contains uptime statistics for a check
type UptimeStats struct {
	CheckID       string  `json:"check_id"`
	Period        string  `json:"period"`
	TotalChecks   int     `json:"total_checks"`
	SuccessCount  int     `json:"success_count"`
	FailureCount  int     `json:"failure_count"`
	UptimePercent float64 `json:"uptime_percent"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	P95LatencyMs  float64 `json:"p95_latency_ms"`
}

// Store handles synthetic check persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new synthetics store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS checks (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		service_id TEXT,
		type TEXT NOT NULL,
		url TEXT NOT NULL,
		method TEXT DEFAULT 'GET',
		headers TEXT,
		body TEXT,
		interval_secs INTEGER DEFAULT 60,
		timeout_secs INTEGER DEFAULT 10,
		assertions TEXT,
		channels TEXT,
		enabled INTEGER DEFAULT 1,
		status TEXT DEFAULT 'unknown',
		last_check DATETIME,
		last_latency_ms INTEGER,
		created DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_check_service ON checks(service_id);

	CREATE TABLE IF NOT EXISTS check_results (
		id TEXT PRIMARY KEY,
		check_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		status TEXT NOT NULL,
		latency_ms INTEGER,
		status_code INTEGER,
		error TEXT,
		body TEXT,
		FOREIGN KEY (check_id) REFERENCES checks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_results_check_time ON check_results(check_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_results_time ON check_results(timestamp DESC);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateCheck creates a new synthetic check
func (s *Store) CreateCheck(c *Check) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.Type == "" {
		c.Type = CheckHTTP
	}
	if c.Method == "" {
		c.Method = "GET"
	}
	if c.Interval == 0 {
		c.Interval = 60
	}
	if c.Timeout == 0 {
		c.Timeout = 10
	}
	if c.Status == "" {
		c.Status = StatusUnknown
	}
	c.Created = time.Now()
	c.Updated = time.Now()

	headersJSON, _ := json.Marshal(c.Headers)
	assertionsJSON, _ := json.Marshal(c.Assertions)
	channelsJSON, _ := json.Marshal(c.Channels)

	_, err := s.db.Exec(`
		INSERT INTO checks (id, name, service_id, type, url, method, headers, body, interval_secs, timeout_secs, assertions, channels, enabled, status, created, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.ServiceID, c.Type, c.URL, c.Method, string(headersJSON), c.Body,
		c.Interval, c.Timeout, string(assertionsJSON), string(channelsJSON),
		c.Enabled, c.Status, c.Created, c.Updated,
	)
	return err
}

// GetCheck retrieves a check by ID
func (s *Store) GetCheck(id string) (*Check, error) {
	row := s.db.QueryRow(`
		SELECT id, name, service_id, type, url, method, headers, body, interval_secs, timeout_secs,
		       assertions, channels, enabled, status, last_check, last_latency_ms, created, updated
		FROM checks WHERE id = ?`, id)

	return s.scanCheck(row)
}

// ListChecks returns all checks
func (s *Store) ListChecks() ([]Check, error) {
	rows, err := s.db.Query(`
		SELECT id, name, service_id, type, url, method, headers, body, interval_secs, timeout_secs,
		       assertions, channels, enabled, status, last_check, last_latency_ms, created, updated
		FROM checks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		c, err := s.scanCheckRows(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, *c)
	}
	return checks, nil
}

// ListEnabledChecks returns all enabled checks
func (s *Store) ListEnabledChecks() ([]Check, error) {
	rows, err := s.db.Query(`
		SELECT id, name, service_id, type, url, method, headers, body, interval_secs, timeout_secs,
		       assertions, channels, enabled, status, last_check, last_latency_ms, created, updated
		FROM checks WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		c, err := s.scanCheckRows(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, *c)
	}
	return checks, nil
}

// UpdateCheck updates a check
func (s *Store) UpdateCheck(c *Check) error {
	c.Updated = time.Now()

	headersJSON, _ := json.Marshal(c.Headers)
	assertionsJSON, _ := json.Marshal(c.Assertions)
	channelsJSON, _ := json.Marshal(c.Channels)

	_, err := s.db.Exec(`
		UPDATE checks SET name=?, service_id=?, type=?, url=?, method=?, headers=?, body=?,
		       interval_secs=?, timeout_secs=?, assertions=?, channels=?, enabled=?, updated=?
		WHERE id=?`,
		c.Name, c.ServiceID, c.Type, c.URL, c.Method, string(headersJSON), c.Body,
		c.Interval, c.Timeout, string(assertionsJSON), string(channelsJSON),
		c.Enabled, c.Updated, c.ID,
	)
	return err
}

// ListChecksByService returns all checks for a given service
func (s *Store) ListChecksByService(serviceID string) ([]Check, error) {
	rows, err := s.db.Query(`
		SELECT id, name, service_id, type, url, method, headers, body, interval_secs, timeout_secs,
		       assertions, channels, enabled, status, last_check, last_latency_ms, created, updated
		FROM checks WHERE service_id = ? ORDER BY name`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		c, err := s.scanCheckRows(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, *c)
	}
	return checks, nil
}

// UpdateCheckStatus updates the status and last check time
func (s *Store) UpdateCheckStatus(id string, status CheckStatus, latencyMs int64) error {
	_, err := s.db.Exec(`
		UPDATE checks SET status=?, last_check=?, last_latency_ms=?, updated=?
		WHERE id=?`,
		status, time.Now(), latencyMs, time.Now(), id,
	)
	return err
}

// DeleteCheck removes a check
func (s *Store) DeleteCheck(id string) error {
	_, err := s.db.Exec("DELETE FROM checks WHERE id = ?", id)
	return err
}

// RecordResult stores a check result
func (s *Store) RecordResult(r *CheckResult) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}

	// Truncate body if too long
	body := r.Body
	if len(body) > 1000 {
		body = body[:1000] + "..."
	}

	_, err := s.db.Exec(`
		INSERT INTO check_results (id, check_id, timestamp, status, latency_ms, status_code, error, body)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.CheckID, r.Timestamp, r.Status, r.LatencyMs, r.StatusCode, r.Error, body,
	)
	return err
}

// GetResults retrieves recent results for a check
func (s *Store) GetResults(checkID string, limit int) ([]CheckResult, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, check_id, timestamp, status, latency_ms, status_code, error, body
		FROM check_results
		WHERE check_id = ?
		ORDER BY timestamp DESC
		LIMIT ?`, checkID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CheckResult
	for rows.Next() {
		var r CheckResult
		var statusCode sql.NullInt64
		var errStr, body sql.NullString

		err := rows.Scan(&r.ID, &r.CheckID, &r.Timestamp, &r.Status, &r.LatencyMs,
			&statusCode, &errStr, &body)
		if err != nil {
			return nil, err
		}

		r.StatusCode = int(statusCode.Int64)
		r.Error = errStr.String
		r.Body = body.String
		results = append(results, r)
	}
	return results, nil
}

// GetUptime calculates uptime statistics for a check
func (s *Store) GetUptime(checkID string, since time.Duration) (*UptimeStats, error) {
	cutoff := time.Now().Add(-since)

	var stats UptimeStats
	stats.CheckID = checkID
	stats.Period = since.String()

	row := s.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN status != 'up' THEN 1 ELSE 0 END) as failure,
			AVG(latency_ms) as avg_latency
		FROM check_results
		WHERE check_id = ? AND timestamp >= ?`, checkID, cutoff)

	var avgLatency sql.NullFloat64
	err := row.Scan(&stats.TotalChecks, &stats.SuccessCount, &stats.FailureCount, &avgLatency)
	if err != nil {
		return nil, err
	}

	if stats.TotalChecks > 0 {
		stats.UptimePercent = float64(stats.SuccessCount) / float64(stats.TotalChecks) * 100
	}
	stats.AvgLatencyMs = avgLatency.Float64

	// Calculate P95 latency
	row = s.db.QueryRow(`
		SELECT latency_ms FROM check_results
		WHERE check_id = ? AND timestamp >= ? AND status = 'up'
		ORDER BY latency_ms DESC
		LIMIT 1 OFFSET (
			SELECT CAST(COUNT(*) * 0.05 AS INTEGER) FROM check_results
			WHERE check_id = ? AND timestamp >= ? AND status = 'up'
		)`, checkID, cutoff, checkID, cutoff)

	var p95 sql.NullInt64
	row.Scan(&p95)
	stats.P95LatencyMs = float64(p95.Int64)

	return &stats, nil
}

// Cleanup removes old results
func (s *Store) Cleanup(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.Exec("DELETE FROM check_results WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Helper functions

func (s *Store) scanCheck(row *sql.Row) (*Check, error) {
	var c Check
	var serviceID sql.NullString
	var headersJSON, assertionsJSON, channelsJSON sql.NullString
	var lastCheck sql.NullTime
	var lastLatency sql.NullInt64

	err := row.Scan(
		&c.ID, &c.Name, &serviceID, &c.Type, &c.URL, &c.Method, &headersJSON, &c.Body,
		&c.Interval, &c.Timeout, &assertionsJSON, &channelsJSON,
		&c.Enabled, &c.Status, &lastCheck, &lastLatency, &c.Created, &c.Updated,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	c.ServiceID = serviceID.String
	if headersJSON.Valid {
		json.Unmarshal([]byte(headersJSON.String), &c.Headers)
	}
	if assertionsJSON.Valid {
		json.Unmarshal([]byte(assertionsJSON.String), &c.Assertions)
	}
	if channelsJSON.Valid {
		json.Unmarshal([]byte(channelsJSON.String), &c.Channels)
	}
	c.LastCheck = lastCheck.Time
	c.LastLatency = lastLatency.Int64

	return &c, nil
}

func (s *Store) scanCheckRows(rows *sql.Rows) (*Check, error) {
	var c Check
	var serviceID sql.NullString
	var headersJSON, assertionsJSON, channelsJSON sql.NullString
	var lastCheck sql.NullTime
	var lastLatency sql.NullInt64

	err := rows.Scan(
		&c.ID, &c.Name, &serviceID, &c.Type, &c.URL, &c.Method, &headersJSON, &c.Body,
		&c.Interval, &c.Timeout, &assertionsJSON, &channelsJSON,
		&c.Enabled, &c.Status, &lastCheck, &lastLatency, &c.Created, &c.Updated,
	)
	if err != nil {
		return nil, err
	}

	c.ServiceID = serviceID.String
	if headersJSON.Valid {
		json.Unmarshal([]byte(headersJSON.String), &c.Headers)
	}
	if assertionsJSON.Valid {
		json.Unmarshal([]byte(assertionsJSON.String), &c.Assertions)
	}
	if channelsJSON.Valid {
		json.Unmarshal([]byte(channelsJSON.String), &c.Channels)
	}
	c.LastCheck = lastCheck.Time
	c.LastLatency = lastLatency.Int64

	return &c, nil
}
