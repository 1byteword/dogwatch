package dbwatch

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// DBType represents a database type
type DBType string

const (
	DBTypeMySQL    DBType = "mysql"
	DBTypePostgres DBType = "postgres"
	DBTypeRedis    DBType = "redis"
)

// QueryRecord represents a captured database query
type QueryRecord struct {
	ID          int64             `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	DBType      DBType            `json:"db_type"`
	PID         uint32            `json:"pid"`
	Comm        string            `json:"comm"`
	Operation   string            `json:"operation"`
	Query       string            `json:"query"`
	Table       string            `json:"table,omitempty"`
	Key         string            `json:"key,omitempty"`
	LatencyMs   float64           `json:"latency_ms"`
	RowsAffected int              `json:"rows_affected,omitempty"`
	Error       string            `json:"error,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
}

// QueryStats represents aggregated query statistics
type QueryStats struct {
	Query       string  `json:"query"`
	Operation   string  `json:"operation"`
	Table       string  `json:"table,omitempty"`
	DBType      DBType  `json:"db_type"`
	Count       int64   `json:"count"`
	TotalTimeMs float64 `json:"total_time_ms"`
	AvgTimeMs   float64 `json:"avg_time_ms"`
	MinTimeMs   float64 `json:"min_time_ms"`
	MaxTimeMs   float64 `json:"max_time_ms"`
	ErrorCount  int64   `json:"error_count"`
}

// DBStats represents per-database statistics
type DBStats struct {
	DBType       DBType  `json:"db_type"`
	TotalQueries int64   `json:"total_queries"`
	TotalErrors  int64   `json:"total_errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	QPS          float64 `json:"qps"`
}

// Store handles database query storage
type Store struct {
	db *sql.DB
	mu sync.RWMutex

	// In-memory stats for recent activity
	recentStats map[DBType]*recentDBStats
	statsMu     sync.RWMutex
}

type recentDBStats struct {
	queries    int64
	errors     int64
	totalMs    float64
	lastUpdate time.Time
}

// NewStore creates a new database watch store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}

	store := &Store{
		db:          db,
		recentStats: make(map[DBType]*recentDBStats),
	}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	go store.cleanupLoop()
	return store, nil
}

func (s *Store) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS db_queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		db_type TEXT NOT NULL,
		pid INTEGER,
		comm TEXT,
		operation TEXT,
		query TEXT,
		tbl TEXT,
		key TEXT,
		latency_ms REAL,
		rows_affected INTEGER,
		error TEXT,
		attrs TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_db_queries_timestamp ON db_queries(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_db_queries_db_type ON db_queries(db_type);
	CREATE INDEX IF NOT EXISTS idx_db_queries_operation ON db_queries(operation);
	CREATE INDEX IF NOT EXISTS idx_db_queries_table ON db_queries(tbl);

	-- Aggregated stats table for slow query analysis
	CREATE TABLE IF NOT EXISTS db_query_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		query_hash TEXT UNIQUE,
		query_sample TEXT,
		operation TEXT,
		tbl TEXT,
		db_type TEXT,
		count INTEGER DEFAULT 0,
		total_time_ms REAL DEFAULT 0,
		min_time_ms REAL DEFAULT 0,
		max_time_ms REAL DEFAULT 0,
		error_count INTEGER DEFAULT 0,
		last_seen DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_db_query_stats_db_type ON db_query_stats(db_type);
	CREATE INDEX IF NOT EXISTS idx_db_query_stats_total_time ON db_query_stats(total_time_ms DESC);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Record stores a database query
func (s *Store) Record(r *QueryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var attrsJSON []byte
	if len(r.Attrs) > 0 {
		attrsJSON, _ = json.Marshal(r.Attrs)
	}

	_, err := s.db.Exec(`
		INSERT INTO db_queries (timestamp, db_type, pid, comm, operation, query, tbl, key, latency_ms, rows_affected, error, attrs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Timestamp, r.DBType, r.PID, r.Comm, r.Operation, r.Query, r.Table, r.Key, r.LatencyMs, r.RowsAffected, r.Error, string(attrsJSON),
	)
	if err != nil {
		return err
	}

	// Update query stats
	s.updateQueryStats(r)

	// Update in-memory stats
	s.statsMu.Lock()
	if s.recentStats[r.DBType] == nil {
		s.recentStats[r.DBType] = &recentDBStats{}
	}
	stats := s.recentStats[r.DBType]
	stats.queries++
	stats.totalMs += r.LatencyMs
	if r.Error != "" {
		stats.errors++
	}
	stats.lastUpdate = time.Now()
	s.statsMu.Unlock()

	return nil
}

func (s *Store) updateQueryStats(r *QueryRecord) {
	// Simple hash: first 100 chars of normalized query
	queryHash := normalizeQuery(r.Query)
	if len(queryHash) > 100 {
		queryHash = queryHash[:100]
	}

	// Try update first
	result, _ := s.db.Exec(`
		UPDATE db_query_stats SET
			count = count + 1,
			total_time_ms = total_time_ms + ?,
			min_time_ms = CASE WHEN min_time_ms = 0 OR ? < min_time_ms THEN ? ELSE min_time_ms END,
			max_time_ms = CASE WHEN ? > max_time_ms THEN ? ELSE max_time_ms END,
			error_count = error_count + CASE WHEN ? != '' THEN 1 ELSE 0 END,
			last_seen = ?
		WHERE query_hash = ?`,
		r.LatencyMs, r.LatencyMs, r.LatencyMs, r.LatencyMs, r.LatencyMs, r.Error, r.Timestamp, queryHash,
	)

	if rows, _ := result.RowsAffected(); rows == 0 {
		// Insert new
		s.db.Exec(`
			INSERT OR IGNORE INTO db_query_stats (query_hash, query_sample, operation, tbl, db_type, count, total_time_ms, min_time_ms, max_time_ms, error_count, last_seen)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?, CASE WHEN ? != '' THEN 1 ELSE 0 END, ?)`,
			queryHash, r.Query, r.Operation, r.Table, r.DBType, r.LatencyMs, r.LatencyMs, r.LatencyMs, r.Error, r.Timestamp,
		)
	}
}

// RecordBatch stores multiple queries efficiently
func (s *Store) RecordBatch(records []*QueryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO db_queries (timestamp, db_type, pid, comm, operation, query, tbl, key, latency_ms, rows_affected, error, attrs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		var attrsJSON []byte
		if len(r.Attrs) > 0 {
			attrsJSON, _ = json.Marshal(r.Attrs)
		}

		_, err = stmt.Exec(r.Timestamp, r.DBType, r.PID, r.Comm, r.Operation, r.Query, r.Table, r.Key, r.LatencyMs, r.RowsAffected, r.Error, string(attrsJSON))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetRecentQueries returns recent queries
func (s *Store) GetRecentQueries(dbType DBType, limit int, since time.Duration) ([]QueryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	cutoff := time.Now().Add(-since)

	query := `SELECT id, timestamp, db_type, pid, comm, operation, query, tbl, key, latency_ms, rows_affected, error, attrs
		FROM db_queries WHERE timestamp > ?`
	args := []interface{}{cutoff}

	if dbType != "" {
		query += ` AND db_type = ?`
		args = append(args, dbType)
	}

	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []QueryRecord
	for rows.Next() {
		var r QueryRecord
		var attrsJSON sql.NullString
		var tbl, key, errStr sql.NullString

		err := rows.Scan(&r.ID, &r.Timestamp, &r.DBType, &r.PID, &r.Comm, &r.Operation, &r.Query, &tbl, &key, &r.LatencyMs, &r.RowsAffected, &errStr, &attrsJSON)
		if err != nil {
			continue
		}

		r.Table = tbl.String
		r.Key = key.String
		r.Error = errStr.String
		if attrsJSON.Valid && attrsJSON.String != "" {
			json.Unmarshal([]byte(attrsJSON.String), &r.Attrs)
		}

		records = append(records, r)
	}

	return records, nil
}

// GetSlowQueries returns queries ordered by latency
func (s *Store) GetSlowQueries(dbType DBType, limit int, since time.Duration) ([]QueryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	cutoff := time.Now().Add(-since)

	query := `SELECT id, timestamp, db_type, pid, comm, operation, query, tbl, key, latency_ms, rows_affected, error, attrs
		FROM db_queries WHERE timestamp > ? AND latency_ms > 0`
	args := []interface{}{cutoff}

	if dbType != "" {
		query += ` AND db_type = ?`
		args = append(args, dbType)
	}

	query += ` ORDER BY latency_ms DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []QueryRecord
	for rows.Next() {
		var r QueryRecord
		var attrsJSON sql.NullString
		var tbl, key, errStr sql.NullString

		err := rows.Scan(&r.ID, &r.Timestamp, &r.DBType, &r.PID, &r.Comm, &r.Operation, &r.Query, &tbl, &key, &r.LatencyMs, &r.RowsAffected, &errStr, &attrsJSON)
		if err != nil {
			continue
		}

		r.Table = tbl.String
		r.Key = key.String
		r.Error = errStr.String
		if attrsJSON.Valid && attrsJSON.String != "" {
			json.Unmarshal([]byte(attrsJSON.String), &r.Attrs)
		}

		records = append(records, r)
	}

	return records, nil
}

// GetQueryStats returns aggregated query statistics
func (s *Store) GetQueryStats(dbType DBType, limit int) ([]QueryStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `SELECT query_sample, operation, tbl, db_type, count, total_time_ms, min_time_ms, max_time_ms, error_count
		FROM db_query_stats`
	args := []interface{}{}

	if dbType != "" {
		query += ` WHERE db_type = ?`
		args = append(args, dbType)
	}

	query += ` ORDER BY total_time_ms DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []QueryStats
	for rows.Next() {
		var qs QueryStats
		var tbl sql.NullString

		err := rows.Scan(&qs.Query, &qs.Operation, &tbl, &qs.DBType, &qs.Count, &qs.TotalTimeMs, &qs.MinTimeMs, &qs.MaxTimeMs, &qs.ErrorCount)
		if err != nil {
			continue
		}

		qs.Table = tbl.String
		if qs.Count > 0 {
			qs.AvgTimeMs = qs.TotalTimeMs / float64(qs.Count)
		}

		stats = append(stats, qs)
	}

	return stats, nil
}

// GetDBStats returns per-database statistics
func (s *Store) GetDBStats(since time.Duration) ([]DBStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since)

	rows, err := s.db.Query(`
		SELECT db_type,
			COUNT(*) as total_queries,
			SUM(CASE WHEN error != '' THEN 1 ELSE 0 END) as total_errors,
			AVG(latency_ms) as avg_latency_ms
		FROM db_queries
		WHERE timestamp > ?
		GROUP BY db_type`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DBStats
	durationSecs := since.Seconds()

	for rows.Next() {
		var ds DBStats
		var avgLatency sql.NullFloat64

		err := rows.Scan(&ds.DBType, &ds.TotalQueries, &ds.TotalErrors, &avgLatency)
		if err != nil {
			continue
		}

		ds.AvgLatencyMs = avgLatency.Float64
		if durationSecs > 0 {
			ds.QPS = float64(ds.TotalQueries) / durationSecs
		}

		stats = append(stats, ds)
	}

	return stats, nil
}

// GetOperationBreakdown returns query counts by operation
func (s *Store) GetOperationBreakdown(dbType DBType, since time.Duration) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since)

	query := `SELECT operation, COUNT(*) FROM db_queries WHERE timestamp > ?`
	args := []interface{}{cutoff}

	if dbType != "" {
		query += ` AND db_type = ?`
		args = append(args, dbType)
	}

	query += ` GROUP BY operation`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var op string
		var count int64
		rows.Scan(&op, &count)
		result[op] = count
	}

	return result, nil
}

// GetTableBreakdown returns query counts by table
func (s *Store) GetTableBreakdown(dbType DBType, since time.Duration) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since)

	query := `SELECT tbl, COUNT(*) FROM db_queries WHERE timestamp > ? AND tbl != ''`
	args := []interface{}{cutoff}

	if dbType != "" {
		query += ` AND db_type = ?`
		args = append(args, dbType)
	}

	query += ` GROUP BY tbl ORDER BY COUNT(*) DESC LIMIT 50`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var tbl string
		var count int64
		rows.Scan(&tbl, &count)
		result[tbl] = count
	}

	return result, nil
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep 24 hours of queries
	cutoff := time.Now().Add(-24 * time.Hour)
	s.db.Exec(`DELETE FROM db_queries WHERE timestamp < ?`, cutoff)

	// Keep query stats for 7 days based on last_seen
	statsCutoff := time.Now().Add(-7 * 24 * time.Hour)
	s.db.Exec(`DELETE FROM db_query_stats WHERE last_seen < ?`, statsCutoff)
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// normalizeQuery removes literals from query for grouping
func normalizeQuery(query string) string {
	// Simple normalization: replace numbers and quoted strings
	// This is a basic implementation - could be more sophisticated
	result := query

	// Replace numbers with ?
	inQuote := false
	quoteChar := byte(0)
	var normalized []byte

	for i := 0; i < len(result); i++ {
		c := result[i]

		if !inQuote {
			if c == '\'' || c == '"' {
				inQuote = true
				quoteChar = c
				normalized = append(normalized, '?')
				// Skip until closing quote
				continue
			}

			if c >= '0' && c <= '9' {
				// Skip number
				for i < len(result) && ((result[i] >= '0' && result[i] <= '9') || result[i] == '.') {
					i++
				}
				i--
				normalized = append(normalized, '?')
				continue
			}

			normalized = append(normalized, c)
		} else {
			if c == quoteChar {
				inQuote = false
			}
			// Skip quoted content
		}
	}

	return string(normalized)
}
