package custommetrics

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// MetricType represents the type of metric
type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

// DataPoint represents a single metric data point
type DataPoint struct {
	Timestamp time.Time         `json:"timestamp"`
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
}

// MetricSeries represents a time series for a metric
type MetricSeries struct {
	Name   string            `json:"name"`
	Type   MetricType        `json:"type"`
	Tags   map[string]string `json:"tags,omitempty"`
	Points []TimeValue       `json:"points"`
}

// TimeValue is a timestamp-value pair
type TimeValue struct {
	Timestamp time.Time `json:"t"`
	Value     float64   `json:"v"`
}

// MetricInfo describes a known metric
type MetricInfo struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	TagKeys   []string          `json:"tag_keys,omitempty"`
	LastValue float64           `json:"last_value"`
	LastSeen  time.Time         `json:"last_seen"`
}

// Store handles custom metrics persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex

	// In-memory aggregation for counters
	counters map[string]float64
	counterMu sync.Mutex
}

// NewStore creates a new custom metrics store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		value REAL NOT NULL,
		tags TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_metrics_name_time ON metrics(name, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_metrics_time ON metrics(timestamp DESC);

	CREATE TABLE IF NOT EXISTS metric_info (
		name TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		tag_keys TEXT,
		last_value REAL,
		last_seen DATETIME
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{
		db:       db,
		counters: make(map[string]float64),
	}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// tagsToKey creates a unique key for a metric + tags combination
func tagsToKey(name string, tags map[string]string) string {
	if len(tags) == 0 {
		return name
	}
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := []string{name}
	for _, k := range keys {
		parts = append(parts, k+"="+tags[k])
	}
	return strings.Join(parts, ",")
}

// Record stores a metric data point
func (s *Store) Record(dp DataPoint) error {
	if dp.Timestamp.IsZero() {
		dp.Timestamp = time.Now()
	}

	// For counters, we accumulate and periodically flush
	if dp.Type == Counter {
		s.counterMu.Lock()
		key := tagsToKey(dp.Name, dp.Tags)
		s.counters[key] += dp.Value
		s.counterMu.Unlock()

		// Also record the increment for historical purposes
		return s.recordPoint(dp)
	}

	return s.recordPoint(dp)
}

func (s *Store) recordPoint(dp DataPoint) error {
	var tagsJSON []byte
	if len(dp.Tags) > 0 {
		tagsJSON, _ = json.Marshal(dp.Tags)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO metrics (timestamp, name, type, value, tags)
		VALUES (?, ?, ?, ?, ?)`,
		dp.Timestamp, dp.Name, dp.Type, dp.Value, string(tagsJSON),
	)
	if err != nil {
		return err
	}

	// Update metric info
	tagKeys := make([]string, 0, len(dp.Tags))
	for k := range dp.Tags {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	tagKeysJSON, _ := json.Marshal(tagKeys)

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO metric_info (name, type, tag_keys, last_value, last_seen)
		VALUES (?, ?, ?, ?, ?)`,
		dp.Name, dp.Type, string(tagKeysJSON), dp.Value, dp.Timestamp,
	)

	return err
}

// RecordBatch stores multiple data points efficiently
func (s *Store) RecordBatch(points []DataPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO metrics (timestamp, name, type, value, tags)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	infoStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO metric_info (name, type, tag_keys, last_value, last_seen)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer infoStmt.Close()

	for _, dp := range points {
		if dp.Timestamp.IsZero() {
			dp.Timestamp = time.Now()
		}

		var tagsJSON []byte
		if len(dp.Tags) > 0 {
			tagsJSON, _ = json.Marshal(dp.Tags)
		}

		_, err = stmt.Exec(dp.Timestamp, dp.Name, dp.Type, dp.Value, string(tagsJSON))
		if err != nil {
			return err
		}

		// Update counter accumulator
		if dp.Type == Counter {
			s.counterMu.Lock()
			key := tagsToKey(dp.Name, dp.Tags)
			s.counters[key] += dp.Value
			s.counterMu.Unlock()
		}

		tagKeys := make([]string, 0, len(dp.Tags))
		for k := range dp.Tags {
			tagKeys = append(tagKeys, k)
		}
		sort.Strings(tagKeys)
		tagKeysJSON, _ := json.Marshal(tagKeys)

		infoStmt.Exec(dp.Name, dp.Type, string(tagKeysJSON), dp.Value, dp.Timestamp)
	}

	return tx.Commit()
}

// Query retrieves metric data for a time range
func (s *Store) Query(name string, tags map[string]string, since time.Duration) (*MetricSeries, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since)

	query := `SELECT timestamp, value, type FROM metrics WHERE name = ? AND timestamp >= ? ORDER BY timestamp`
	rows, err := s.db.Query(query, name, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	series := &MetricSeries{
		Name: name,
		Tags: tags,
	}

	for rows.Next() {
		var tv TimeValue
		var metricType string
		if err := rows.Scan(&tv.Timestamp, &tv.Value, &metricType); err != nil {
			return nil, err
		}
		series.Type = MetricType(metricType)
		series.Points = append(series.Points, tv)
	}

	return series, nil
}

// List returns all known metrics
func (s *Store) List() ([]MetricInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT name, type, tag_keys, last_value, last_seen
		FROM metric_info
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []MetricInfo
	for rows.Next() {
		var m MetricInfo
		var tagKeysJSON sql.NullString
		if err := rows.Scan(&m.Name, &m.Type, &tagKeysJSON, &m.LastValue, &m.LastSeen); err != nil {
			return nil, err
		}
		if tagKeysJSON.Valid {
			json.Unmarshal([]byte(tagKeysJSON.String), &m.TagKeys)
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

// GetCounterValue returns the current accumulated value of a counter
func (s *Store) GetCounterValue(name string, tags map[string]string) float64 {
	s.counterMu.Lock()
	defer s.counterMu.Unlock()
	key := tagsToKey(name, tags)
	return s.counters[key]
}

// GetLatest returns the most recent values for all metrics
func (s *Store) GetLatest(limit int) ([]DataPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT timestamp, name, type, value, tags
		FROM metrics
		ORDER BY timestamp DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []DataPoint
	for rows.Next() {
		var dp DataPoint
		var tagsJSON sql.NullString
		if err := rows.Scan(&dp.Timestamp, &dp.Name, &dp.Type, &dp.Value, &tagsJSON); err != nil {
			return nil, err
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			json.Unmarshal([]byte(tagsJSON.String), &dp.Tags)
		}
		points = append(points, dp)
	}

	return points, nil
}

// Cleanup removes old data points
func (s *Store) Cleanup(maxAge time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.Exec("DELETE FROM metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
