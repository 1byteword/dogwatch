// Package profile provides persistent storage for CPU profile samples
// to enable correlation with distributed traces.
package profile

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Sample represents a recorded profile sample with timing information.
type Sample struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	PID         uint32    `json:"pid"`
	TGID        uint32    `json:"tgid"`
	Comm        string    `json:"comm"`
	Count       uint64    `json:"count"`
	KernelStack []string  `json:"kernel_stack,omitempty"`
	UserStack   []string  `json:"user_stack,omitempty"`
}

// StackFrame represents a single frame in a stack trace.
type StackFrame struct {
	Address  uint64 `json:"address"`
	Function string `json:"function"`
	Module   string `json:"module,omitempty"`
	Offset   uint64 `json:"offset,omitempty"`
}

// Store persists profile samples for correlation with traces.
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening profile db: %w", err)
	}

	store := &Store{db: db}
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	go store.cleanupLoop()
	return store, nil
}

func (s *Store) createTables() error {
	queries := []string{
		// Profile samples table - stores aggregated samples
		`CREATE TABLE IF NOT EXISTS profile_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp INTEGER NOT NULL,
			pid INTEGER NOT NULL,
			tgid INTEGER NOT NULL,
			comm TEXT NOT NULL,
			count INTEGER NOT NULL,
			kernel_stack TEXT,
			user_stack TEXT,
			created_at INTEGER DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_timestamp ON profile_samples(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_pid ON profile_samples(pid)`,
		`CREATE INDEX IF NOT EXISTS idx_samples_pid_time ON profile_samples(pid, timestamp)`,

		// Profile-trace links table - stores correlations
		`CREATE TABLE IF NOT EXISTS profile_trace_links (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id TEXT NOT NULL,
			span_id TEXT NOT NULL,
			sample_id INTEGER NOT NULL,
			function_name TEXT,
			confidence REAL,
			created_at INTEGER DEFAULT (strftime('%s', 'now')),
			FOREIGN KEY (sample_id) REFERENCES profile_samples(id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_links_trace ON profile_trace_links(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_links_span ON profile_trace_links(span_id)`,
		`CREATE INDEX IF NOT EXISTS idx_links_sample ON profile_trace_links(sample_id)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("exec %q: %w", q[:50], err)
		}
	}
	return nil
}

// RecordSample stores a profile sample.
func (s *Store) RecordSample(sample *Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	kernelStack, _ := json.Marshal(sample.KernelStack)
	userStack, _ := json.Marshal(sample.UserStack)

	result, err := s.db.Exec(`
		INSERT INTO profile_samples (timestamp, pid, tgid, comm, count, kernel_stack, user_stack)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		sample.Timestamp.UnixMilli(),
		sample.PID,
		sample.TGID,
		sample.Comm,
		sample.Count,
		string(kernelStack),
		string(userStack),
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	sample.ID = id
	return nil
}

// RecordSamples stores multiple profile samples in a batch.
func (s *Store) RecordSamples(samples []*Sample) error {
	if len(samples) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO profile_samples (timestamp, pid, tgid, comm, count, kernel_stack, user_stack)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sample := range samples {
		kernelStack, _ := json.Marshal(sample.KernelStack)
		userStack, _ := json.Marshal(sample.UserStack)

		result, err := stmt.Exec(
			sample.Timestamp.UnixMilli(),
			sample.PID,
			sample.TGID,
			sample.Comm,
			sample.Count,
			string(kernelStack),
			string(userStack),
		)
		if err != nil {
			return err
		}

		id, _ := result.LastInsertId()
		sample.ID = id
	}

	return tx.Commit()
}

// QueryByTimeRange returns samples within a time range.
func (s *Store) QueryByTimeRange(start, end time.Time) ([]*Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, timestamp, pid, tgid, comm, count, kernel_stack, user_stack
		FROM profile_samples
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT 10000
	`, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

// QueryByPIDAndTime returns samples for a specific PID within a time range.
func (s *Store) QueryByPIDAndTime(pid uint32, start, end time.Time) ([]*Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, timestamp, pid, tgid, comm, count, kernel_stack, user_stack
		FROM profile_samples
		WHERE pid = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT 10000
	`, pid, start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

// QueryByFunction returns samples containing a specific function in the stack.
func (s *Store) QueryByFunction(functionName string, start, end time.Time) ([]*Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Use LIKE for substring matching in JSON stack traces
	pattern := "%" + functionName + "%"

	rows, err := s.db.Query(`
		SELECT id, timestamp, pid, tgid, comm, count, kernel_stack, user_stack
		FROM profile_samples
		WHERE timestamp >= ? AND timestamp <= ?
		  AND (kernel_stack LIKE ? OR user_stack LIKE ?)
		ORDER BY count DESC
		LIMIT 1000
	`, start.UnixMilli(), end.UnixMilli(), pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSamples(rows)
}

func (s *Store) scanSamples(rows *sql.Rows) ([]*Sample, error) {
	var samples []*Sample
	for rows.Next() {
		var sample Sample
		var tsMillis int64
		var kernelStackJSON, userStackJSON sql.NullString

		if err := rows.Scan(
			&sample.ID,
			&tsMillis,
			&sample.PID,
			&sample.TGID,
			&sample.Comm,
			&sample.Count,
			&kernelStackJSON,
			&userStackJSON,
		); err != nil {
			continue
		}

		sample.Timestamp = time.UnixMilli(tsMillis)

		if kernelStackJSON.Valid {
			json.Unmarshal([]byte(kernelStackJSON.String), &sample.KernelStack)
		}
		if userStackJSON.Valid {
			json.Unmarshal([]byte(userStackJSON.String), &sample.UserStack)
		}

		samples = append(samples, &sample)
	}
	return samples, rows.Err()
}

// ProfileTraceLink represents a link between a profile sample and a trace span.
type ProfileTraceLink struct {
	ID           int64   `json:"id"`
	TraceID      string  `json:"trace_id"`
	SpanID       string  `json:"span_id"`
	SampleID     int64   `json:"sample_id"`
	FunctionName string  `json:"function_name"`
	Confidence   float64 `json:"confidence"`
}

// RecordLink stores a profile-trace link.
func (s *Store) RecordLink(link *ProfileTraceLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		INSERT INTO profile_trace_links (trace_id, span_id, sample_id, function_name, confidence)
		VALUES (?, ?, ?, ?, ?)
	`,
		link.TraceID,
		link.SpanID,
		link.SampleID,
		link.FunctionName,
		link.Confidence,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	link.ID = id
	return nil
}

// GetLinksForTrace returns all profile links for a trace.
func (s *Store) GetLinksForTrace(traceID string) ([]*ProfileTraceLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trace_id, span_id, sample_id, function_name, confidence
		FROM profile_trace_links
		WHERE trace_id = ?
		ORDER BY confidence DESC
	`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*ProfileTraceLink
	for rows.Next() {
		var link ProfileTraceLink
		if err := rows.Scan(
			&link.ID,
			&link.TraceID,
			&link.SpanID,
			&link.SampleID,
			&link.FunctionName,
			&link.Confidence,
		); err != nil {
			continue
		}
		links = append(links, &link)
	}
	return links, rows.Err()
}

// GetLinksForSpan returns all profile links for a specific span.
func (s *Store) GetLinksForSpan(traceID, spanID string) ([]*ProfileTraceLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, trace_id, span_id, sample_id, function_name, confidence
		FROM profile_trace_links
		WHERE trace_id = ? AND span_id = ?
		ORDER BY confidence DESC
	`, traceID, spanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []*ProfileTraceLink
	for rows.Next() {
		var link ProfileTraceLink
		if err := rows.Scan(
			&link.ID,
			&link.TraceID,
			&link.SpanID,
			&link.SampleID,
			&link.FunctionName,
			&link.Confidence,
		); err != nil {
			continue
		}
		links = append(links, &link)
	}
	return links, rows.Err()
}

// GetSampleByID returns a sample by its ID.
func (s *Store) GetSampleByID(id int64) (*Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT id, timestamp, pid, tgid, comm, count, kernel_stack, user_stack
		FROM profile_samples
		WHERE id = ?
	`, id)

	var sample Sample
	var tsMillis int64
	var kernelStackJSON, userStackJSON sql.NullString

	if err := row.Scan(
		&sample.ID,
		&tsMillis,
		&sample.PID,
		&sample.TGID,
		&sample.Comm,
		&sample.Count,
		&kernelStackJSON,
		&userStackJSON,
	); err != nil {
		return nil, err
	}

	sample.Timestamp = time.UnixMilli(tsMillis)

	if kernelStackJSON.Valid {
		json.Unmarshal([]byte(kernelStackJSON.String), &sample.KernelStack)
	}
	if userStackJSON.Valid {
		json.Unmarshal([]byte(userStackJSON.String), &sample.UserStack)
	}

	return &sample, nil
}

// GetHotspots returns the top functions by sample count in a time range.
func (s *Store) GetHotspots(start, end time.Time, limit int) ([]Hotspot, error) {
	samples, err := s.QueryByTimeRange(start, end)
	if err != nil {
		return nil, err
	}

	// Aggregate by function name
	functionCounts := make(map[string]uint64)
	var totalCount uint64

	for _, sample := range samples {
		// Count user stack functions
		for _, fn := range sample.UserStack {
			functionCounts[fn] += sample.Count
			totalCount += sample.Count
		}
		// Count kernel stack functions
		for _, fn := range sample.KernelStack {
			functionCounts[fn] += sample.Count
			totalCount += sample.Count
		}
	}

	// Convert to slice and sort
	type fnCount struct {
		fn    string
		count uint64
	}
	var sorted []fnCount
	for fn, count := range functionCounts {
		sorted = append(sorted, fnCount{fn, count})
	}

	// Sort by count descending
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Take top N
	if limit > len(sorted) {
		limit = len(sorted)
	}

	hotspots := make([]Hotspot, limit)
	for i := 0; i < limit; i++ {
		hotspots[i] = Hotspot{
			Function:    sorted[i].fn,
			SampleCount: sorted[i].count,
			Percent:     float64(sorted[i].count) / float64(totalCount) * 100,
		}
	}

	return hotspots, nil
}

// Hotspot represents a function with its CPU usage.
type Hotspot struct {
	Function    string  `json:"function"`
	SampleCount uint64  `json:"sample_count"`
	Percent     float64 `json:"percent"`
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanup()
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep 24 hours of data
	cutoff := time.Now().Add(-24 * time.Hour).UnixMilli()

	// Delete old links first (foreign key)
	s.db.Exec(`
		DELETE FROM profile_trace_links
		WHERE sample_id IN (SELECT id FROM profile_samples WHERE timestamp < ?)
	`, cutoff)

	// Delete old samples
	s.db.Exec(`DELETE FROM profile_samples WHERE timestamp < ?`, cutoff)
}

func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection.
func (s *Store) DB() *sql.DB {
	return s.db
}
