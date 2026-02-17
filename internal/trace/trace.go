package trace

import (
	"context"
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
	ProcessID    uint32            `json:"process_id,omitempty"`    // Process ID for profile correlation
	Hostname     string            `json:"hostname,omitempty"`      // Host for correlation
	ContainerID  string            `json:"container_id,omitempty"`  // Container ID for correlation
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
	db, err := storage.OpenDB(dbPath)
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
	// Create base tables first (without process_id index for migration compatibility)
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
			process_id INTEGER,
			hostname TEXT,
			container_id TEXT,
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

	// Run migrations for existing databases (adds process_id etc if missing)
	s.runMigrations()

	// Now create the process_id index (after migration ensures column exists)
	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_spans_process_id ON spans(process_id)`)
	return nil
}

func (s *Store) runMigrations() {
	// Add columns if they don't exist (ignore errors for already-existing columns)
	migrations := []string{
		`ALTER TABLE spans ADD COLUMN process_id INTEGER`,
		`ALTER TABLE spans ADD COLUMN hostname TEXT`,
		`ALTER TABLE spans ADD COLUMN container_id TEXT`,
		`ALTER TABLE traces ADD COLUMN end_time TEXT`,
	}

	for _, m := range migrations {
		s.db.Exec(m)
	}

	s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_spans_process_id ON spans(process_id)`)
}

// RecordSpan stores a span and updates the trace
func (s *Store) RecordSpan(span Span) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Extract process metadata from attributes if not already set
	extractProcessMetadata(&span)

	attrs, _ := json.Marshal(span.Attributes)

	// Insert or replace span
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO spans
		(trace_id, span_id, parent_span_id, name, service_name, kind, start_time, end_time, duration_ms, status, status_message, attributes, process_id, hostname, container_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		nullableUint32(span.ProcessID),
		nullableString(span.Hostname),
		nullableString(span.ContainerID),
	)
	if err != nil {
		return err
	}

	// Update trace summary incrementally (1 SELECT + 1 UPSERT instead of 5-6 queries)
	return s.updateTraceSummaryIncremental(span)
}

// RecordSpans stores multiple spans in a single transaction.
// This is significantly faster than calling RecordSpan in a loop.
func (s *Store) RecordSpans(spans []Span) error {
	if len(spans) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	spanStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO spans
		(trace_id, span_id, parent_span_id, name, service_name, kind, start_time, end_time, duration_ms, status, status_message, attributes, process_id, hostname, container_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare span stmt: %w", err)
	}
	defer spanStmt.Close()

	// Group spans by trace for summary updates
	traceSpans := make(map[string][]Span)

	for i := range spans {
		extractProcessMetadata(&spans[i])
		attrs, _ := json.Marshal(spans[i].Attributes)

		_, err := spanStmt.Exec(
			spans[i].TraceID, spans[i].SpanID, spans[i].ParentSpanID,
			spans[i].Name, spans[i].ServiceName, spans[i].Kind,
			spans[i].StartTime.UTC().Format(time.RFC3339Nano),
			spans[i].EndTime.UTC().Format(time.RFC3339Nano),
			spans[i].DurationMs, spans[i].Status, spans[i].StatusMsg,
			string(attrs),
			nullableUint32(spans[i].ProcessID),
			nullableString(spans[i].Hostname),
			nullableString(spans[i].ContainerID),
		)
		if err != nil {
			return fmt.Errorf("insert span: %w", err)
		}
		traceSpans[spans[i].TraceID] = append(traceSpans[spans[i].TraceID], spans[i])
	}

	// Update trace summaries per unique trace
	for traceID, tSpans := range traceSpans {
		if err := s.updateTraceSummaryBatch(tx, traceID, tSpans); err != nil {
			return fmt.Errorf("update trace summary: %w", err)
		}
	}

	return tx.Commit()
}

// extractProcessMetadata extracts process-level metadata from span attributes
func extractProcessMetadata(span *Span) {
	if span.Attributes == nil {
		return
	}

	// Extract process.pid
	if span.ProcessID == 0 {
		if pidStr, ok := span.Attributes["process.pid"]; ok {
			var pid uint64
			fmt.Sscanf(pidStr, "%d", &pid)
			span.ProcessID = uint32(pid)
		}
	}

	// Extract host.name
	if span.Hostname == "" {
		if host, ok := span.Attributes["host.name"]; ok {
			span.Hostname = host
		} else if host, ok := span.Attributes["hostname"]; ok {
			span.Hostname = host
		}
	}

	// Extract container.id
	if span.ContainerID == "" {
		if cid, ok := span.Attributes["container.id"]; ok {
			span.ContainerID = cid
		}
	}
}

func nullableUint32(v uint32) interface{} {
	if v == 0 {
		return nil
	}
	return v
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// updateTraceSummaryIncremental updates the trace summary using only the incoming
// span's data. This reduces from 5-6 queries to 1 SELECT + 1 UPSERT per span.
func (s *Store) updateTraceSummaryIncremental(span Span) error {
	isRoot := span.ParentSpanID == ""
	startTimeStr := span.StartTime.UTC().Format(time.RFC3339Nano)
	endTimeStr := span.EndTime.UTC().Format(time.RFC3339Nano)

	// Merge services list (single PK lookup on existing trace)
	services := s.mergeServices(span.TraceID, span.ServiceName)
	servicesJSON, _ := json.Marshal(services)

	// For root spans, pass data to update trace identity fields
	rootSpanID := ""
	svcName := ""
	spanName := ""
	if isRoot {
		rootSpanID = span.SpanID
		svcName = span.ServiceName
		spanName = span.Name
	}

	_, err := s.db.Exec(`
		INSERT INTO traces (trace_id, root_span_id, service_name, name, start_time, end_time, duration_ms, span_count, status, services)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(trace_id) DO UPDATE SET
			root_span_id = CASE WHEN excluded.root_span_id != '' THEN excluded.root_span_id ELSE traces.root_span_id END,
			service_name = CASE WHEN excluded.root_span_id != '' THEN excluded.service_name ELSE traces.service_name END,
			name = CASE WHEN excluded.root_span_id != '' THEN excluded.name ELSE traces.name END,
			start_time = MIN(traces.start_time, excluded.start_time),
			end_time = MAX(COALESCE(traces.end_time, traces.start_time), excluded.end_time),
			duration_ms = (julianday(MAX(COALESCE(traces.end_time, traces.start_time), excluded.end_time)) - julianday(MIN(traces.start_time, excluded.start_time))) * 86400000,
			span_count = traces.span_count + 1,
			status = CASE WHEN excluded.status = 'ERROR' THEN 'ERROR'
			              WHEN excluded.root_span_id != '' THEN excluded.status
			              ELSE traces.status END,
			services = excluded.services
	`,
		span.TraceID, rootSpanID, svcName, spanName,
		startTimeStr, endTimeStr, span.DurationMs, span.Status,
		string(servicesJSON),
	)
	return err
}

// updateTraceSummaryBatch updates the trace summary for a batch of spans in a single transaction.
func (s *Store) updateTraceSummaryBatch(tx *sql.Tx, traceID string, spans []Span) error {
	// Merge services from existing trace + all new spans
	var existingServicesJSON sql.NullString
	tx.QueryRow(`SELECT services FROM traces WHERE trace_id = ?`, traceID).Scan(&existingServicesJSON)

	svcSet := make(map[string]bool)
	if existingServicesJSON.Valid {
		var existing []string
		json.Unmarshal([]byte(existingServicesJSON.String), &existing)
		for _, svc := range existing {
			svcSet[svc] = true
		}
	}

	// Find root span, earliest start, latest end from batch
	var rootSpanID, svcName, spanName, spanStatus string
	var minStart, maxEnd time.Time
	spanCount := len(spans)
	hasError := false

	for i, sp := range spans {
		svcSet[sp.ServiceName] = true
		if sp.ParentSpanID == "" {
			rootSpanID = sp.SpanID
			svcName = sp.ServiceName
			spanName = sp.Name
			spanStatus = sp.Status
		}
		if sp.Status == "ERROR" {
			hasError = true
		}
		if i == 0 || sp.StartTime.Before(minStart) {
			minStart = sp.StartTime
		}
		if i == 0 || sp.EndTime.After(maxEnd) {
			maxEnd = sp.EndTime
		}
	}

	services := make([]string, 0, len(svcSet))
	for svc := range svcSet {
		services = append(services, svc)
	}
	servicesJSON, _ := json.Marshal(services)

	status := spanStatus
	if hasError {
		status = "ERROR"
	}

	startTimeStr := minStart.UTC().Format(time.RFC3339Nano)
	endTimeStr := maxEnd.UTC().Format(time.RFC3339Nano)

	_, err := tx.Exec(`
		INSERT INTO traces (trace_id, root_span_id, service_name, name, start_time, end_time, duration_ms, span_count, status, services)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(trace_id) DO UPDATE SET
			root_span_id = CASE WHEN excluded.root_span_id != '' THEN excluded.root_span_id ELSE traces.root_span_id END,
			service_name = CASE WHEN excluded.root_span_id != '' THEN excluded.service_name ELSE traces.service_name END,
			name = CASE WHEN excluded.root_span_id != '' THEN excluded.name ELSE traces.name END,
			start_time = MIN(traces.start_time, excluded.start_time),
			end_time = MAX(COALESCE(traces.end_time, traces.start_time), excluded.end_time),
			duration_ms = (julianday(MAX(COALESCE(traces.end_time, traces.start_time), excluded.end_time)) - julianday(MIN(traces.start_time, excluded.start_time))) * 86400000,
			span_count = traces.span_count + excluded.span_count,
			status = CASE WHEN excluded.status = 'ERROR' THEN 'ERROR'
			              WHEN excluded.root_span_id != '' THEN excluded.status
			              ELSE traces.status END,
			services = excluded.services
	`,
		traceID, rootSpanID, svcName, spanName,
		startTimeStr, endTimeStr,
		maxEnd.Sub(minStart).Seconds()*1000,
		spanCount, status, string(servicesJSON),
	)
	return err
}

// mergeServices merges a new service name into the existing services list for a trace.
func (s *Store) mergeServices(traceID, serviceName string) []string {
	var existingJSON sql.NullString
	s.db.QueryRow(`SELECT services FROM traces WHERE trace_id = ?`, traceID).Scan(&existingJSON)

	var services []string
	if existingJSON.Valid {
		json.Unmarshal([]byte(existingJSON.String), &services)
	}
	for _, svc := range services {
		if svc == serviceName {
			return services
		}
	}
	return append(services, serviceName)
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

// SpanQueryOptions for querying spans with filters
type SpanQueryOptions struct {
	Service       string
	TimeStart     time.Time
	TimeEnd       time.Time
	MinDurationMs float64
	MaxDurationMs float64
	Status        string // "ERROR", "OK", or "" for all
	Limit         int
}

// QuerySpans returns spans matching the given criteria
func (s *Store) QuerySpans(ctx context.Context, opts SpanQueryOptions) ([]Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind,
			   start_time, end_time, duration_ms, status, status_message, attributes
		FROM spans WHERE 1=1
	`
	var args []interface{}

	if opts.Service != "" {
		query += ` AND service_name = ?`
		args = append(args, opts.Service)
	}
	if !opts.TimeStart.IsZero() {
		query += ` AND start_time >= ?`
		args = append(args, opts.TimeStart.UTC().Format(time.RFC3339Nano))
	}
	if !opts.TimeEnd.IsZero() {
		query += ` AND start_time <= ?`
		args = append(args, opts.TimeEnd.UTC().Format(time.RFC3339Nano))
	}
	if opts.MinDurationMs > 0 {
		query += ` AND duration_ms >= ?`
		args = append(args, opts.MinDurationMs)
	}
	if opts.MaxDurationMs > 0 {
		query += ` AND duration_ms < ?`
		args = append(args, opts.MaxDurationMs)
	}
	if opts.Status != "" {
		query += ` AND status = ?`
		args = append(args, opts.Status)
	}

	query += ` ORDER BY start_time DESC`

	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSpans(rows)
}

// GetSpansByIDs retrieves specific spans by their IDs
func (s *Store) GetSpansByIDs(ctx context.Context, spanIDs []string) ([]Span, error) {
	if len(spanIDs) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	placeholders := make([]string, len(spanIDs))
	args := make([]interface{}, len(spanIDs))
	for i, id := range spanIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind,
			   start_time, end_time, duration_ms, status, status_message, attributes
		FROM spans WHERE span_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSpans(rows)
}

// GetLatencyPercentile calculates the latency at a given percentile for a service
func (s *Store) GetLatencyPercentile(ctx context.Context, service string, percentile float64, start, end time.Time) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT duration_ms FROM spans
		WHERE start_time >= ? AND start_time <= ?
	`
	args := []interface{}{
		start.UTC().Format(time.RFC3339Nano),
		end.UTC().Format(time.RFC3339Nano),
	}

	if service != "" {
		query += ` AND service_name = ?`
		args = append(args, service)
	}

	query += ` ORDER BY duration_ms ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var durations []float64
	for rows.Next() {
		var d float64
		if err := rows.Scan(&d); err != nil {
			continue
		}
		durations = append(durations, d)
	}

	if len(durations) == 0 {
		return 0, fmt.Errorf("no spans found")
	}

	// Calculate percentile index
	idx := int(float64(len(durations)) * percentile / 100.0)
	if idx >= len(durations) {
		idx = len(durations) - 1
	}

	return durations[idx], nil
}

func (s *Store) scanSpans(rows *sql.Rows) ([]Span, error) {
	var spans []Span
	for rows.Next() {
		var span Span
		var startTime, endTime, attrs string
		var parentSpanID, kind, status, statusMsg sql.NullString

		if err := rows.Scan(
			&span.TraceID, &span.SpanID, &parentSpanID, &span.Name, &span.ServiceName,
			&kind, &startTime, &endTime, &span.DurationMs, &status, &statusMsg, &attrs,
		); err != nil {
			continue
		}

		span.ParentSpanID = parentSpanID.String
		span.Kind = kind.String
		span.Status = status.String
		span.StatusMsg = statusMsg.String
		span.StartTime, _ = time.Parse(time.RFC3339Nano, startTime)
		span.EndTime, _ = time.Parse(time.RFC3339Nano, endTime)
		json.Unmarshal([]byte(attrs), &span.Attributes)

		spans = append(spans, span)
	}
	return spans, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for query builder
func (s *Store) DB() *sql.DB {
	return s.db
}

// QuerySpansByPIDAndTime returns spans for a specific process ID within a time range.
// This is used for correlating profile samples with trace spans.
func (s *Store) QuerySpansByPIDAndTime(pid uint32, start, end time.Time) ([]Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind,
		       start_time, end_time, duration_ms, status, status_message, attributes,
		       process_id, hostname, container_id
		FROM spans
		WHERE process_id = ?
		  AND start_time >= ? AND end_time <= ?
		ORDER BY start_time
	`, pid, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSpansWithProcess(rows)
}

// QuerySpansByTimeRange returns spans overlapping a time range.
func (s *Store) QuerySpansByTimeRange(start, end time.Time) ([]Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind,
		       start_time, end_time, duration_ms, status, status_message, attributes,
		       process_id, hostname, container_id
		FROM spans
		WHERE start_time <= ? AND end_time >= ?
		ORDER BY start_time
		LIMIT 1000
	`, end.UTC().Format(time.RFC3339Nano), start.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSpansWithProcess(rows)
}

// QuerySpansWithProcessID returns all spans that have a process ID set.
func (s *Store) QuerySpansWithProcessID(start, end time.Time) ([]Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT trace_id, span_id, parent_span_id, name, service_name, kind,
		       start_time, end_time, duration_ms, status, status_message, attributes,
		       process_id, hostname, container_id
		FROM spans
		WHERE process_id IS NOT NULL AND process_id > 0
		  AND start_time >= ? AND end_time <= ?
		ORDER BY start_time
		LIMIT 10000
	`, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSpansWithProcess(rows)
}

func (s *Store) scanSpansWithProcess(rows *sql.Rows) ([]Span, error) {
	var spans []Span
	for rows.Next() {
		var span Span
		var startTime, endTime, attrs string
		var parentSpanID, kind, status, statusMsg, hostname, containerID sql.NullString
		var processID sql.NullInt64

		if err := rows.Scan(
			&span.TraceID, &span.SpanID, &parentSpanID, &span.Name, &span.ServiceName,
			&kind, &startTime, &endTime, &span.DurationMs, &status, &statusMsg, &attrs,
			&processID, &hostname, &containerID,
		); err != nil {
			continue
		}

		span.ParentSpanID = parentSpanID.String
		span.Kind = kind.String
		span.Status = status.String
		span.StatusMsg = statusMsg.String
		span.StartTime, _ = time.Parse(time.RFC3339Nano, startTime)
		span.EndTime, _ = time.Parse(time.RFC3339Nano, endTime)
		json.Unmarshal([]byte(attrs), &span.Attributes)

		if processID.Valid {
			span.ProcessID = uint32(processID.Int64)
		}
		span.Hostname = hostname.String
		span.ContainerID = containerID.String

		spans = append(spans, span)
	}
	return spans, nil
}
