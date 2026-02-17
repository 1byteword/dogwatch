package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// Store handles persistent storage of metrics
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// SystemMetricPoint represents a single system metric data point
type SystemMetricPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemPercent  float64   `json:"mem_percent"`
	DiskReadPS  float64   `json:"disk_read_ps"`
	DiskWritePS float64   `json:"disk_write_ps"`
	NetRxPS     float64   `json:"net_rx_ps"`
	NetTxPS     float64   `json:"net_tx_ps"`
	Load1       float64   `json:"load_1"`
}

// EndpointMetricPoint represents endpoint metrics at a point in time
type EndpointMetricPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	RequestCount int64     `json:"request_count"`
	ErrorCount   int64     `json:"error_count"`
	P50Ms        float64   `json:"p50_ms"`
	P99Ms        float64   `json:"p99_ms"`
}

// ConnectionMetricPoint represents connection count over time
type ConnectionMetricPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	TotalConnections int64     `json:"total_connections"`
	TotalRequests    int64     `json:"total_requests"`
	TotalErrors      int64     `json:"total_errors"`
}

// New creates a new storage instance
func New(dbPath string) (*Store, error) {
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	store := &Store{db: db}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	// Start cleanup routine
	go store.cleanupLoop()

	return store, nil
}

func (s *Store) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS system_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			cpu_percent REAL,
			mem_percent REAL,
			disk_read_ps REAL,
			disk_write_ps REAL,
			net_rx_ps REAL,
			net_tx_ps REAL,
			load_1 REAL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_metrics_ts ON system_metrics(timestamp)`,

		`CREATE TABLE IF NOT EXISTS endpoint_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			method TEXT,
			path TEXT,
			request_count INTEGER,
			error_count INTEGER,
			p50_ms REAL,
			p99_ms REAL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_endpoint_metrics_ts ON endpoint_metrics(timestamp)`,

		`CREATE TABLE IF NOT EXISTS connection_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			total_connections INTEGER,
			total_requests INTEGER,
			total_errors INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_metrics_ts ON connection_metrics(timestamp)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("executing %q: %w", q[:50], err)
		}
	}

	return nil
}

// RecordSystemMetrics stores a system metrics snapshot
func (s *Store) RecordSystemMetrics(cpu, mem, diskRead, diskWrite, netRx, netTx, load1 float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO system_metrics (timestamp, cpu_percent, mem_percent, disk_read_ps, disk_write_ps, net_rx_ps, net_tx_ps, load_1)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, now, cpu, mem, diskRead, diskWrite, netRx, netTx, load1)

	return err
}

// RecordEndpointMetrics stores endpoint metrics snapshot
func (s *Store) RecordEndpointMetrics(method, path string, reqCount, errCount int64, p50, p99 float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO endpoint_metrics (method, path, request_count, error_count, p50_ms, p99_ms)
		VALUES (?, ?, ?, ?, ?, ?)
	`, method, path, reqCount, errCount, p50, p99)

	return err
}

// RecordConnectionMetrics stores connection summary metrics
func (s *Store) RecordConnectionMetrics(totalConns, totalReqs, totalErrs int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO connection_metrics (timestamp, total_connections, total_requests, total_errors)
		VALUES (?, ?, ?, ?)
	`, now, totalConns, totalReqs, totalErrs)

	return err
}

// GetSystemMetrics retrieves system metrics for a time range
func (s *Store) GetSystemMetrics(since time.Duration) ([]SystemMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT timestamp, cpu_percent, mem_percent, disk_read_ps, disk_write_ps, net_rx_ps, net_tx_ps, load_1
		FROM system_metrics
		WHERE timestamp > ?
		ORDER BY timestamp ASC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SystemMetricPoint
	for rows.Next() {
		var p SystemMetricPoint
		var ts string
		if err := rows.Scan(&ts, &p.CPUPercent, &p.MemPercent, &p.DiskReadPS, &p.DiskWritePS, &p.NetRxPS, &p.NetTxPS, &p.Load1); err != nil {
			continue
		}
		p.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, p)
	}

	return points, nil
}

// GetConnectionMetrics retrieves connection metrics for a time range
func (s *Store) GetConnectionMetrics(since time.Duration) ([]ConnectionMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT timestamp, total_connections, total_requests, total_errors
		FROM connection_metrics
		WHERE timestamp > ?
		ORDER BY timestamp ASC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ConnectionMetricPoint
	for rows.Next() {
		var p ConnectionMetricPoint
		var ts string
		if err := rows.Scan(&ts, &p.TotalConnections, &p.TotalRequests, &p.TotalErrors); err != nil {
			continue
		}
		p.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, p)
	}

	return points, nil
}

// GetEndpointMetrics retrieves endpoint metrics for a time range
func (s *Store) GetEndpointMetrics(since time.Duration, method, path string) ([]EndpointMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT timestamp, method, path, request_count, error_count, p50_ms, p99_ms
		FROM endpoint_metrics
		WHERE timestamp > ? AND method = ? AND path = ?
		ORDER BY timestamp ASC
	`, cutoff, method, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []EndpointMetricPoint
	for rows.Next() {
		var p EndpointMetricPoint
		var ts string
		if err := rows.Scan(&ts, &p.Method, &p.Path, &p.RequestCount, &p.ErrorCount, &p.P50Ms, &p.P99Ms); err != nil {
			continue
		}
		p.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, p)
	}

	return points, nil
}

// cleanupLoop removes old data periodically
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

// cleanup removes data older than retention period (24 hours)
func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)

	tables := []string{"system_metrics", "endpoint_metrics", "connection_metrics"}
	for _, table := range tables {
		s.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE timestamp < ?", table), cutoff)
	}
}

// GetConnectionMetricsByTimeRange retrieves connection metrics for a specific time range
func (s *Store) GetConnectionMetricsByTimeRange(start, end time.Time) ([]ConnectionMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startStr := start.UTC().Format(time.RFC3339)
	endStr := end.UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT timestamp, total_connections, total_requests, total_errors
		FROM connection_metrics
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []ConnectionMetricPoint
	for rows.Next() {
		var p ConnectionMetricPoint
		var ts string
		if err := rows.Scan(&ts, &p.TotalConnections, &p.TotalRequests, &p.TotalErrors); err != nil {
			continue
		}
		p.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, p)
	}

	return points, nil
}

// GetSystemMetricsByTimeRange retrieves system metrics for a specific time range
func (s *Store) GetSystemMetricsByTimeRange(start, end time.Time) ([]SystemMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startStr := start.UTC().Format(time.RFC3339)
	endStr := end.UTC().Format(time.RFC3339)

	rows, err := s.db.Query(`
		SELECT timestamp, cpu_percent, mem_percent, disk_read_ps, disk_write_ps, net_rx_ps, net_tx_ps, load_1
		FROM system_metrics
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []SystemMetricPoint
	for rows.Next() {
		var p SystemMetricPoint
		var ts string
		if err := rows.Scan(&ts, &p.CPUPercent, &p.MemPercent, &p.DiskReadPS, &p.DiskWritePS, &p.NetRxPS, &p.NetTxPS, &p.Load1); err != nil {
			continue
		}
		p.Timestamp, _ = time.Parse(time.RFC3339, ts)
		points = append(points, p)
	}

	return points, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for tiering operations
func (s *Store) DB() *sql.DB {
	return s.db
}
