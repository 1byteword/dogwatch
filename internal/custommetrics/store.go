package custommetrics

import (
	"crypto/sha256"
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
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

// CardinalityHook is called when metrics are recorded for cardinality tracking
type CardinalityHook interface {
	RecordSeries(name string, tags map[string]string)
}

// DataShapingHook evaluates data shaping rules
type DataShapingHook interface {
	// EvaluateMetric returns (shouldKeep, transformedTags)
	EvaluateMetric(name string, tags map[string]string, sizeBytes int) (bool, map[string]string)
}

// Store handles custom metrics persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex

	// In-memory aggregation for counters
	counters map[string]float64
	counterMu sync.Mutex

	// Cardinality tracking hook
	cardinalityHook CardinalityHook

	// Data shaping hook
	shapingHook DataShapingHook
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
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

	CREATE TABLE IF NOT EXISTS histograms (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		name TEXT NOT NULL,
		tags TEXT,
		count INTEGER NOT NULL,
		sum REAL NOT NULL,
		min_val REAL,
		max_val REAL,
		bounds BLOB NOT NULL,
		bucket_counts BLOB NOT NULL,
		exemplars BLOB
	);
	CREATE INDEX IF NOT EXISTS idx_histograms_name_time ON histograms(name, timestamp DESC);

	CREATE TABLE IF NOT EXISTS metric_info (
		name TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		tag_keys TEXT,
		last_value REAL,
		last_seen DATETIME
	);

	CREATE TABLE IF NOT EXISTS series (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		fingerprint TEXT NOT NULL UNIQUE,
		tags_json TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX IF NOT EXISTS idx_series_name ON series(name);

	CREATE TABLE IF NOT EXISTS label_index (
		series_id INTEGER NOT NULL,
		label_key TEXT NOT NULL,
		label_value TEXT NOT NULL,
		UNIQUE(series_id, label_key)
	);
	CREATE INDEX IF NOT EXISTS idx_label_kv ON label_index(label_key, label_value);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Add series_id column to metrics if missing (migration)
	db.Exec("ALTER TABLE metrics ADD COLUMN series_id INTEGER REFERENCES series(id)")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_metrics_series_time ON metrics(series_id, timestamp DESC)")

	return &Store{
		db:       db,
		counters: make(map[string]float64),
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for query builder
func (s *Store) DB() *sql.DB {
	return s.db
}

// SetCardinalityHook sets a hook for cardinality tracking
func (s *Store) SetCardinalityHook(hook CardinalityHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cardinalityHook = hook
}

// SetDataShapingHook sets a hook for data shaping
func (s *Store) SetDataShapingHook(hook DataShapingHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shapingHook = hook
}

// seriesFingerprint returns a stable hash for a metric name + tag set.
func seriesFingerprint(name string, tags map[string]string) string {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		if k == "__name__" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	h.Write([]byte(name))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(tags[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// internSeries returns the series ID for a name+tags combination,
// creating the series and label index entries if they don't exist yet.
func (s *Store) internSeries(tx *sql.Tx, name string, tags map[string]string) (int64, error) {
	fp := seriesFingerprint(name, tags)

	var id int64
	err := tx.QueryRow("SELECT id FROM series WHERE fingerprint = ?", fp).Scan(&id)
	if err == nil {
		return id, nil
	}

	tagsJSON, _ := json.Marshal(tags)
	res, err := tx.Exec("INSERT OR IGNORE INTO series (name, fingerprint, tags_json) VALUES (?, ?, ?)",
		name, fp, string(tagsJSON))
	if err != nil {
		return 0, err
	}

	id, _ = res.LastInsertId()
	if id == 0 {
		// INSERT OR IGNORE hit a conflict; re-read the ID
		tx.QueryRow("SELECT id FROM series WHERE fingerprint = ?", fp).Scan(&id)
		return id, nil
	}

	// Populate the label index for this new series
	for k, v := range tags {
		if k == "__name__" {
			continue
		}
		tx.Exec("INSERT OR IGNORE INTO label_index (series_id, label_key, label_value) VALUES (?, ?, ?)",
			id, k, v)
	}

	return id, nil
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

	// Apply data shaping rules
	if s.shapingHook != nil {
		keep, transformedTags := s.shapingHook.EvaluateMetric(dp.Name, dp.Tags, 100)
		if !keep {
			return nil // Drop the metric
		}
		if transformedTags != nil {
			dp.Tags = transformedTags
		}
	}

	// Track cardinality
	if s.cardinalityHook != nil {
		s.cardinalityHook.RecordSeries(dp.Name, dp.Tags)
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

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Intern the series for indexed lookups
	var seriesID *int64
	if len(dp.Tags) > 0 {
		sid, err := s.internSeries(tx, dp.Name, dp.Tags)
		if err == nil && sid > 0 {
			seriesID = &sid
		}
	}

	_, err = tx.Exec(`
		INSERT INTO metrics (timestamp, name, type, value, tags, series_id)
		VALUES (?, ?, ?, ?, ?, ?)`,
		dp.Timestamp, dp.Name, dp.Type, dp.Value, string(tagsJSON), seriesID,
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

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO metric_info (name, type, tag_keys, last_value, last_seen)
		VALUES (?, ?, ?, ?, ?)`,
		dp.Name, dp.Type, string(tagKeysJSON), dp.Value, dp.Timestamp,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// RecordBatch stores multiple data points efficiently
func (s *Store) RecordBatch(points []DataPoint) error {
	// Track cardinality outside the lock
	if s.cardinalityHook != nil {
		for _, dp := range points {
			s.cardinalityHook.RecordSeries(dp.Name, dp.Tags)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO metrics (timestamp, name, type, value, tags, series_id)
		VALUES (?, ?, ?, ?, ?, ?)`)
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

		// Intern the series
		var seriesID *int64
		if len(dp.Tags) > 0 {
			sid, err := s.internSeries(tx, dp.Name, dp.Tags)
			if err == nil && sid > 0 {
				seriesID = &sid
			}
		}

		_, err = stmt.Exec(dp.Timestamp, dp.Name, dp.Type, dp.Value, string(tagsJSON), seriesID)
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

	// Also cleanup histograms
	s.db.Exec("DELETE FROM histograms WHERE timestamp < ?", cutoff)

	return result.RowsAffected()
}

// RecordHistogram stores a native histogram data point
func (s *Store) RecordHistogram(hdp HistogramDataPoint) error {
	if hdp.Timestamp.IsZero() {
		hdp.Timestamp = time.Now()
	}

	// Track cardinality
	if s.cardinalityHook != nil {
		s.cardinalityHook.RecordSeries(hdp.Name, hdp.Tags)
	}

	var tagsJSON []byte
	if len(hdp.Tags) > 0 {
		tagsJSON, _ = json.Marshal(hdp.Tags)
	}

	boundsJSON, _ := json.Marshal(hdp.ExplicitBounds)
	countsJSON, _ := json.Marshal(hdp.BucketCounts)

	var exemplarsJSON []byte
	if len(hdp.Exemplars) > 0 {
		exemplarsJSON, _ = json.Marshal(hdp.Exemplars)
	}

	var minVal, maxVal *float64
	if hdp.Min != nil {
		minVal = hdp.Min
	}
	if hdp.Max != nil {
		maxVal = hdp.Max
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO histograms (timestamp, name, tags, count, sum, min_val, max_val, bounds, bucket_counts, exemplars)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		hdp.Timestamp, hdp.Name, string(tagsJSON), hdp.Count, hdp.Sum, minVal, maxVal,
		boundsJSON, countsJSON, string(exemplarsJSON),
	)
	return err
}

// RecordHistogramBatch stores multiple histogram data points efficiently
func (s *Store) RecordHistogramBatch(points []HistogramDataPoint) error {
	if len(points) == 0 {
		return nil
	}

	// Track cardinality outside the lock
	if s.cardinalityHook != nil {
		for _, hdp := range points {
			s.cardinalityHook.RecordSeries(hdp.Name, hdp.Tags)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO histograms (timestamp, name, tags, count, sum, min_val, max_val, bounds, bucket_counts, exemplars)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, hdp := range points {
		if hdp.Timestamp.IsZero() {
			hdp.Timestamp = time.Now()
		}

		var tagsJSON []byte
		if len(hdp.Tags) > 0 {
			tagsJSON, _ = json.Marshal(hdp.Tags)
		}

		boundsJSON, _ := json.Marshal(hdp.ExplicitBounds)
		countsJSON, _ := json.Marshal(hdp.BucketCounts)

		var exemplarsJSON []byte
		if len(hdp.Exemplars) > 0 {
			exemplarsJSON, _ = json.Marshal(hdp.Exemplars)
		}

		_, err = stmt.Exec(
			hdp.Timestamp, hdp.Name, string(tagsJSON), hdp.Count, hdp.Sum,
			hdp.Min, hdp.Max, boundsJSON, countsJSON, string(exemplarsJSON),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// QueryHistogram retrieves histogram data points for a metric in a time range
func (s *Store) QueryHistogram(name string, tags map[string]string, start, end time.Time) ([]HistogramDataPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT timestamp, name, tags, count, sum, min_val, max_val, bounds, bucket_counts, exemplars
		FROM histograms WHERE name = ? AND timestamp >= ? AND timestamp <= ? ORDER BY timestamp`
	rows, err := s.db.Query(query, name, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []HistogramDataPoint
	for rows.Next() {
		var hdp HistogramDataPoint
		var tagsJSON, boundsJSON, countsJSON, exemplarsJSON sql.NullString
		var minVal, maxVal sql.NullFloat64

		if err := rows.Scan(&hdp.Timestamp, &hdp.Name, &tagsJSON, &hdp.Count, &hdp.Sum,
			&minVal, &maxVal, &boundsJSON, &countsJSON, &exemplarsJSON); err != nil {
			return nil, err
		}

		if tagsJSON.Valid && tagsJSON.String != "" {
			json.Unmarshal([]byte(tagsJSON.String), &hdp.Tags)
		}
		if boundsJSON.Valid {
			json.Unmarshal([]byte(boundsJSON.String), &hdp.ExplicitBounds)
		}
		if countsJSON.Valid {
			json.Unmarshal([]byte(countsJSON.String), &hdp.BucketCounts)
		}
		if exemplarsJSON.Valid && exemplarsJSON.String != "" {
			json.Unmarshal([]byte(exemplarsJSON.String), &hdp.Exemplars)
		}
		if minVal.Valid {
			v := minVal.Float64
			hdp.Min = &v
		}
		if maxVal.Valid {
			v := maxVal.Float64
			hdp.Max = &v
		}

		// Filter by tags if specified
		if len(tags) > 0 {
			match := true
			for k, v := range tags {
				if hdp.Tags[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}

		points = append(points, hdp)
	}

	return points, nil
}

// QueryHistogramSnapshot retrieves and aggregates histogram data into a snapshot
func (s *Store) QueryHistogramSnapshot(name string, tags map[string]string, start, end time.Time) (*HistogramSnapshot, error) {
	points, err := s.QueryHistogram(name, tags, start, end)
	if err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, nil
	}
	return AggregateHistograms(points), nil
}
