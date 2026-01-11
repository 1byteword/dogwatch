package trace

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Span represents a single operation in a trace
type Span struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	ServiceName  string            `json:"service_name"`
	Kind         string            `json:"kind"` // CLIENT, SERVER, INTERNAL, PRODUCER, CONSUMER
	StartTime    time.Time         `json:"start_time"`
	EndTime      time.Time         `json:"end_time"`
	DurationMs   float64           `json:"duration_ms"`
	Status       string            `json:"status"` // OK, ERROR, UNSET
	StatusMsg    string            `json:"status_message,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

// Trace represents a complete distributed trace
type Trace struct {
	TraceID     string    `json:"trace_id"`
	RootSpan    string    `json:"root_span"`
	ServiceName string    `json:"service_name"`
	Name        string    `json:"name"`
	StartTime   time.Time `json:"start_time"`
	DurationMs  float64   `json:"duration_ms"`
	SpanCount   int       `json:"span_count"`
	Status      string    `json:"status"`
	Services    []string  `json:"services"`
}

// TraceDetail includes full span tree
type TraceDetail struct {
	Trace
	Spans []Span `json:"spans"`
}

// Store handles trace storage
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new trace store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening trace db: %w", err)
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
		`CREATE TABLE IF NOT EXISTS spans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id TEXT NOT NULL,
			span_id TEXT NOT NULL,
			parent_span_id TEXT,
			name TEXT NOT NULL,
			service_name TEXT NOT NULL,
			kind TEXT,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			duration_ms REAL,
			status TEXT,
			status_message TEXT,
			attributes TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON spans(trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_service ON spans(service_name)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_start_time ON spans(start_time)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_spans_span_id ON spans(trace_id, span_id)`,

		`CREATE TABLE IF NOT EXISTS traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			trace_id TEXT UNIQUE NOT NULL,
			root_span_id TEXT,
			service_name TEXT,
			name TEXT,
			start_time TEXT NOT NULL,
			duration_ms REAL,
			span_count INTEGER DEFAULT 0,
			status TEXT,
			services TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_start_time ON traces(start_time)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_service ON traces(service_name)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// RecordSpan stores a span and updates the trace
func (s *Store) RecordSpan(span Span) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	attrs, _ := json.Marshal(span.Attributes)

	// Insert or replace span
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO spans
		(trace_id, span_id, parent_span_id, name, service_name, kind, start_time, end_time, duration_ms, status, status_message, attributes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		span.TraceID,
		span.SpanID,
		span.ParentSpanID,
		span.Name,
		span.ServiceName,
		span.Kind,
		span.StartTime.UTC().Format(time.RFC3339Nano),
		span.EndTime.UTC().Format(time.RFC3339Nano),
		span.DurationMs,
		span.Status,
		span.StatusMsg,
		string(attrs),
	)
	if err != nil {
		return err
	}

	// Update trace summary
	return s.updateTraceSummary(span.TraceID)
}

func (s *Store) updateTraceSummary(traceID string) error {
	// Get trace stats
	var rootSpanID, serviceName, name, status sql.NullString
	var startTime string
	var durationMs float64
	var spanCount int

	row := s.db.QueryRow(`
		SELECT span_id, service_name, name, start_time, duration_ms, status
		FROM spans
		WHERE trace_id = ? AND (parent_span_id IS NULL OR parent_span_id = '')
		ORDER BY start_time ASC LIMIT 1
	`, traceID)
	row.Scan(&rootSpanID, &serviceName, &name, &startTime, &durationMs, &status)

	// If no root span, use earliest span
	if !rootSpanID.Valid {
		row = s.db.QueryRow(`
			SELECT span_id, service_name, name, start_time, duration_ms, status
			FROM spans WHERE trace_id = ? ORDER BY start_time ASC LIMIT 1
		`, traceID)
		row.Scan(&rootSpanID, &serviceName, &name, &startTime, &durationMs, &status)
	}

	// Count spans
	s.db.QueryRow(`SELECT COUNT(*) FROM spans WHERE trace_id = ?`, traceID).Scan(&spanCount)

	// Get total duration (from earliest start to latest end)
	var totalDuration float64
	s.db.QueryRow(`
		SELECT (julianday(MAX(end_time)) - julianday(MIN(start_time))) * 86400000
		FROM spans WHERE trace_id = ?
	`, traceID).Scan(&totalDuration)

	// Get unique services
	rows, _ := s.db.Query(`SELECT DISTINCT service_name FROM spans WHERE trace_id = ?`, traceID)
	var services []string
	for rows.Next() {
		var svc string
		rows.Scan(&svc)
		services = append(services, svc)
	}
	rows.Close()
	servicesJSON, _ := json.Marshal(services)

	// Upsert trace
	_, err := s.db.Exec(`
		INSERT INTO traces (trace_id, root_span_id, service_name, name, start_time, duration_ms, span_count, status, services)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trace_id) DO UPDATE SET
			root_span_id = excluded.root_span_id,
			service_name = excluded.service_name,
			name = excluded.name,
			start_time = excluded.start_time,
			duration_ms = excluded.duration_ms,
			span_count = excluded.span_count,
			status = excluded.status,
			services = excluded.services
	`,
		traceID,
		rootSpanID.String,
		serviceName.String,
		name.String,
		startTime,
		totalDuration,
		spanCount,
		status.String,
		string(servicesJSON),
	)
	return err
}

// ListTraces returns recent traces
func (s *Store) ListTraces(limit int, service string, since time.Duration) ([]Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	query := `
		SELECT trace_id, root_span_id, service_name, name, start_time, duration_ms, span_count, status, services
		FROM traces
		WHERE start_time > ?
	`
	args := []interface{}{cutoff}

	if service != "" {
		query += ` AND service_name = ?`
		args = append(args, service)
	}

	query += ` ORDER BY start_time DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []Trace
	for rows.Next() {
		var t Trace
		var startTime, servicesJSON string
		var rootSpan, svcName, name, status sql.NullString

		if err := rows.Scan(&t.TraceID, &rootSpan, &svcName, &name, &startTime, &t.DurationMs, &t.SpanCount, &status, &servicesJSON); err != nil {
			continue
		}

		t.RootSpan = rootSpan.String
		t.ServiceName = svcName.String
		t.Name = name.String
		t.Status = status.String
		t.StartTime, _ = time.Parse(time.RFC3339, startTime)
		json.Unmarshal([]byte(servicesJSON), &t.Services)

		traces = append(traces, t)
	}

	return traces, nil
}

// GetTrace returns full trace detail with all spans
func (s *Store) GetTrace(traceID string) (*TraceDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get trace summary
	var detail TraceDetail
	var startTime, servicesJSON string
	var rootSpan, svcName, name, status sql.NullString

	row := s.db.QueryRow(`
		SELECT trace_id, root_span_id, service_name, name, start_time, duration_ms, span_count, status, services
		FROM traces WHERE trace_id = ?
	`, traceID)

	if err := row.Scan(&detail.TraceID, &rootSpan, &svcName, &name, &startTime, &detail.DurationMs, &detail.SpanCount, &status, &servicesJSON); err != nil {
		return nil, err
	}

	detail.RootSpan = rootSpan.String
	detail.ServiceName = svcName.String
	detail.Name = name.String
	detail.Status = status.String
	detail.StartTime, _ = time.Parse(time.RFC3339, startTime)
	json.Unmarshal([]byte(servicesJSON), &detail.Services)

	// Get all spans
	rows, err := s.db.Query(`
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind, start_time, end_time, duration_ms, status, status_message, attributes
		FROM spans WHERE trace_id = ? ORDER BY start_time ASC
	`, traceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var span Span
		var startTime, endTime, attrs string
		var parentSpanID, kind, status, statusMsg sql.NullString

		if err := rows.Scan(&span.TraceID, &span.SpanID, &parentSpanID, &span.Name, &span.ServiceName, &kind, &startTime, &endTime, &span.DurationMs, &status, &statusMsg, &attrs); err != nil {
			continue
		}

		span.ParentSpanID = parentSpanID.String
		span.Kind = kind.String
		span.Status = status.String
		span.StatusMsg = statusMsg.String
		span.StartTime, _ = time.Parse(time.RFC3339Nano, startTime)
		span.EndTime, _ = time.Parse(time.RFC3339Nano, endTime)
		json.Unmarshal([]byte(attrs), &span.Attributes)

		detail.Spans = append(detail.Spans, span)
	}

	return &detail, nil
}

// GetServices returns list of unique services from traces
func (s *Store) GetServices() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT DISTINCT service_name FROM spans WHERE service_name != '' ORDER BY service_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var svc string
		rows.Scan(&svc)
		services = append(services, svc)
	}
	return services, nil
}

// GetServiceDependencies returns service-to-service call relationships
func (s *Store) GetServiceDependencies() ([]ServiceDependency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find parent-child service relationships from spans
	rows, err := s.db.Query(`
		SELECT
			p.service_name as parent_service,
			c.service_name as child_service,
			COUNT(*) as call_count
		FROM spans c
		JOIN spans p ON c.trace_id = p.trace_id AND c.parent_span_id = p.span_id
		WHERE p.service_name != c.service_name
		GROUP BY p.service_name, c.service_name
		ORDER BY call_count DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []ServiceDependency
	for rows.Next() {
		var d ServiceDependency
		rows.Scan(&d.Parent, &d.Child, &d.CallCount)
		deps = append(deps, d)
	}
	return deps, nil
}

// ServiceDependency represents a call from one service to another
type ServiceDependency struct {
	Parent    string `json:"parent"`
	Child     string `json:"child"`
	CallCount int64  `json:"call_count"`
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

	// Keep 24 hours of traces
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	s.db.Exec(`DELETE FROM spans WHERE start_time < ?`, cutoff)
	s.db.Exec(`DELETE FROM traces WHERE start_time < ?`, cutoff)
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}
