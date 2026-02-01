package logs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// LogLevel represents log severity
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
	LevelFatal LogLevel = "fatal"
)

// LogEntry represents a single log entry
type LogEntry struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Level     LogLevel          `json:"level"`
	Message   string            `json:"message"`
	Service   string            `json:"service,omitempty"`
	Host      string            `json:"host,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	SpanID    string            `json:"span_id,omitempty"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

// SortOrder specifies how search results should be sorted
type SortOrder string

const (
	SortByTime      SortOrder = "time"      // Sort by timestamp (default)
	SortByRelevance SortOrder = "relevance" // Sort by BM25 relevance score
)

// SearchQuery represents a log search query
type SearchQuery struct {
	Query     string    // Full-text search query
	Level     LogLevel  // Filter by level (or empty for all)
	Service   string    // Filter by service
	TraceID   string    // Filter by trace ID
	StartTime time.Time // Start of time range
	EndTime   time.Time // End of time range
	Limit     int       // Max results (default 100)
	Offset    int       // Pagination offset
	SortBy    SortOrder // Sort order (time or relevance)
}

// ScoredLogEntry is a log entry with a relevance score
type ScoredLogEntry struct {
	LogEntry
	Score float64 `json:"score,omitempty"` // BM25 relevance score (lower is more relevant)
}

// SearchResult contains search results with metadata
type SearchResult struct {
	Entries    []ScoredLogEntry `json:"entries"`
	TotalCount int              `json:"total_count"`
	HasMore    bool             `json:"has_more"`
}

// LogEntries returns the entries as plain LogEntry slice (without scores)
func (r *SearchResult) LogEntries() []LogEntry {
	entries := make([]LogEntry, len(r.Entries))
	for i, e := range r.Entries {
		entries[i] = e.LogEntry
	}
	return entries
}

// Store handles log persistence with full-text search
type Store struct {
	db              *sql.DB
	patternDetector *PatternDetector
}

// NewStore creates a new log store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create tables with FTS5 for full-text search
	schema := `
	-- Main logs table
	CREATE TABLE IF NOT EXISTS logs (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		service TEXT,
		host TEXT,
		trace_id TEXT,
		span_id TEXT,
		attrs TEXT
	);

	-- Indexes for common queries
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_logs_level ON logs(level);
	CREATE INDEX IF NOT EXISTS idx_logs_service ON logs(service);
	CREATE INDEX IF NOT EXISTS idx_logs_trace_id ON logs(trace_id);

	-- FTS5 virtual table for full-text search
	CREATE VIRTUAL TABLE IF NOT EXISTS logs_fts USING fts5(
		id,
		message,
		service,
		content='logs',
		content_rowid='rowid'
	);

	-- Triggers to keep FTS in sync
	CREATE TRIGGER IF NOT EXISTS logs_ai AFTER INSERT ON logs BEGIN
		INSERT INTO logs_fts(rowid, id, message, service)
		VALUES (NEW.rowid, NEW.id, NEW.message, NEW.service);
	END;

	CREATE TRIGGER IF NOT EXISTS logs_ad AFTER DELETE ON logs BEGIN
		INSERT INTO logs_fts(logs_fts, rowid, id, message, service)
		VALUES('delete', OLD.rowid, OLD.id, OLD.message, OLD.service);
	END;

	CREATE TRIGGER IF NOT EXISTS logs_au AFTER UPDATE ON logs BEGIN
		INSERT INTO logs_fts(logs_fts, rowid, id, message, service)
		VALUES('delete', OLD.rowid, OLD.id, OLD.message, OLD.service);
		INSERT INTO logs_fts(rowid, id, message, service)
		VALUES (NEW.rowid, NEW.id, NEW.message, NEW.service);
	END;
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{
		db:              db,
		patternDetector: NewPatternDetector(),
	}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for query builder
func (s *Store) DB() *sql.DB {
	return s.db
}

// Insert adds a log entry
func (s *Store) Insert(entry *LogEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Level == "" {
		entry.Level = LevelInfo
	}

	var attrsJSON []byte
	if len(entry.Attrs) > 0 {
		attrsJSON, _ = json.Marshal(entry.Attrs)
	}

	_, err := s.db.Exec(`
		INSERT INTO logs (id, timestamp, level, message, service, host, trace_id, span_id, attrs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp.UTC().Format(time.RFC3339Nano), entry.Level, entry.Message,
		entry.Service, entry.Host, entry.TraceID, entry.SpanID, string(attrsJSON),
	)
	if err == nil && s.patternDetector != nil {
		s.patternDetector.Process(entry)
	}
	return err
}

// InsertBatch adds multiple log entries efficiently
func (s *Store) InsertBatch(entries []LogEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO logs (id, timestamp, level, message, service, host, trace_id, span_id, attrs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range entries {
		entry := &entries[i]
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now()
		}
		if entry.Level == "" {
			entry.Level = LevelInfo
		}

		var attrsJSON []byte
		if len(entry.Attrs) > 0 {
			attrsJSON, _ = json.Marshal(entry.Attrs)
		}

		_, err = stmt.Exec(
			entry.ID, entry.Timestamp.UTC().Format(time.RFC3339Nano), entry.Level, entry.Message,
			entry.Service, entry.Host, entry.TraceID, entry.SpanID, string(attrsJSON),
		)
		if err != nil {
			return err
		}

		// Process for pattern detection
		if s.patternDetector != nil {
			s.patternDetector.Process(entry)
		}
	}

	return tx.Commit()
}

// GetPatterns returns all detected log patterns
func (s *Store) GetPatterns() []*Pattern {
	if s.patternDetector == nil {
		return nil
	}
	return s.patternDetector.GetPatterns()
}

// GetPattern returns a specific pattern by ID
func (s *Store) GetPattern(id string) *Pattern {
	if s.patternDetector == nil {
		return nil
	}
	return s.patternDetector.GetPattern(id)
}

// GetTopPatterns returns the N most frequent patterns
func (s *Store) GetTopPatterns(n int) []*Pattern {
	if s.patternDetector == nil {
		return nil
	}
	return s.patternDetector.GetTopPatterns(n)
}

// GetNewPatterns returns patterns first seen in the last duration
func (s *Store) GetNewPatterns(since time.Duration) []*Pattern {
	if s.patternDetector == nil {
		return nil
	}
	return s.patternDetector.GetNewPatterns(since)
}

// GetIncreasingPatterns returns patterns with increasing trend
func (s *Store) GetIncreasingPatterns() []*Pattern {
	if s.patternDetector == nil {
		return nil
	}
	return s.patternDetector.GetIncreasingPatterns()
}

// GetPatternStats returns pattern detection statistics
func (s *Store) GetPatternStats() PatternStats {
	if s.patternDetector == nil {
		return PatternStats{}
	}
	return s.patternDetector.Stats()
}

// Search queries logs with full-text search and filters
// When a query is provided, results can be sorted by BM25 relevance score
func (s *Store) Search(q SearchQuery) (*SearchResult, error) {
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}

	// Default to relevance sort when there's a query, time sort otherwise
	if q.SortBy == "" {
		if q.Query != "" {
			q.SortBy = SortByRelevance
		} else {
			q.SortBy = SortByTime
		}
	}

	// Use optimized BM25 query path when sorting by relevance with a search query
	if q.Query != "" && q.SortBy == SortByRelevance {
		return s.searchWithBM25(q)
	}

	return s.searchByTime(q)
}

// searchWithBM25 performs full-text search with BM25 relevance ranking
func (s *Store) searchWithBM25(q SearchQuery) (*SearchResult, error) {
	var conditions []string
	var args []interface{}

	// Build filter conditions (applied after FTS match)
	if q.Level != "" {
		conditions = append(conditions, "l.level = ?")
		args = append(args, q.Level)
	}
	if q.Service != "" {
		conditions = append(conditions, "l.service = ?")
		args = append(args, q.Service)
	}
	if q.TraceID != "" {
		conditions = append(conditions, "l.trace_id = ?")
		args = append(args, q.TraceID)
	}
	if !q.StartTime.IsZero() {
		conditions = append(conditions, "l.timestamp >= ?")
		args = append(args, q.StartTime.UTC().Format(time.RFC3339Nano))
	}
	if !q.EndTime.IsZero() {
		conditions = append(conditions, "l.timestamp <= ?")
		args = append(args, q.EndTime.UTC().Format(time.RFC3339Nano))
	}

	filterClause := ""
	if len(conditions) > 0 {
		filterClause = " AND " + strings.Join(conditions, " AND ")
	}

	// Count total matching results
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM logs l
		INNER JOIN logs_fts fts ON l.id = fts.id
		WHERE logs_fts MATCH ?%s`, filterClause)
	countArgs := append([]interface{}{q.Query}, args...)

	var totalCount int
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count query: %w", err)
	}

	// Fetch results with BM25 ranking
	// bm25() returns negative scores where more negative = more relevant
	// We negate it so higher scores = more relevant (more intuitive)
	query := fmt.Sprintf(`
		SELECT l.id, l.timestamp, l.level, l.message, l.service, l.host,
		       l.trace_id, l.span_id, l.attrs, -bm25(logs_fts) as score
		FROM logs l
		INNER JOIN logs_fts fts ON l.id = fts.id
		WHERE logs_fts MATCH ?%s
		ORDER BY score DESC
		LIMIT ? OFFSET ?`, filterClause)

	queryArgs := append([]interface{}{q.Query}, args...)
	queryArgs = append(queryArgs, q.Limit+1, q.Offset)

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var entries []ScoredLogEntry
	for rows.Next() {
		var entry ScoredLogEntry
		var attrsJSON sql.NullString
		var service, host, traceID, spanID sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.Level, &entry.Message,
			&service, &host, &traceID, &spanID, &attrsJSON, &entry.Score,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		entry.Service = service.String
		entry.Host = host.String
		entry.TraceID = traceID.String
		entry.SpanID = spanID.String

		if attrsJSON.Valid && attrsJSON.String != "" {
			json.Unmarshal([]byte(attrsJSON.String), &entry.Attrs)
		}

		entries = append(entries, entry)
	}

	hasMore := len(entries) > q.Limit
	if hasMore {
		entries = entries[:q.Limit]
	}

	return &SearchResult{
		Entries:    entries,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// searchByTime performs search sorted by timestamp (default for non-FTS queries)
func (s *Store) searchByTime(q SearchQuery) (*SearchResult, error) {
	var conditions []string
	var args []interface{}

	// Full-text search filter
	if q.Query != "" {
		conditions = append(conditions, "l.id IN (SELECT id FROM logs_fts WHERE logs_fts MATCH ?)")
		args = append(args, q.Query)
	}

	// Level filter
	if q.Level != "" {
		conditions = append(conditions, "l.level = ?")
		args = append(args, q.Level)
	}

	// Service filter
	if q.Service != "" {
		conditions = append(conditions, "l.service = ?")
		args = append(args, q.Service)
	}

	// Trace ID filter
	if q.TraceID != "" {
		conditions = append(conditions, "l.trace_id = ?")
		args = append(args, q.TraceID)
	}

	// Time range
	if !q.StartTime.IsZero() {
		conditions = append(conditions, "l.timestamp >= ?")
		args = append(args, q.StartTime.UTC().Format(time.RFC3339Nano))
	}
	if !q.EndTime.IsZero() {
		conditions = append(conditions, "l.timestamp <= ?")
		args = append(args, q.EndTime.UTC().Format(time.RFC3339Nano))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching results
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM logs l %s", whereClause)
	var totalCount int
	if err := s.db.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count query: %w", err)
	}

	// Fetch results
	query := fmt.Sprintf(`
		SELECT id, timestamp, level, message, service, host, trace_id, span_id, attrs
		FROM logs l
		%s
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`, whereClause)

	args = append(args, q.Limit+1, q.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	var entries []ScoredLogEntry
	for rows.Next() {
		var entry ScoredLogEntry
		var attrsJSON sql.NullString
		var service, host, traceID, spanID sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.Level, &entry.Message,
			&service, &host, &traceID, &spanID, &attrsJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		entry.Service = service.String
		entry.Host = host.String
		entry.TraceID = traceID.String
		entry.SpanID = spanID.String

		if attrsJSON.Valid && attrsJSON.String != "" {
			json.Unmarshal([]byte(attrsJSON.String), &entry.Attrs)
		}

		entries = append(entries, entry)
	}

	hasMore := len(entries) > q.Limit
	if hasMore {
		entries = entries[:q.Limit]
	}

	return &SearchResult{
		Entries:    entries,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// GetByTraceID retrieves all logs for a specific trace
func (s *Store) GetByTraceID(traceID string) ([]LogEntry, error) {
	result, err := s.Search(SearchQuery{
		TraceID: traceID,
		Limit:   1000,
		SortBy:  SortByTime,
	})
	if err != nil {
		return nil, err
	}
	// Extract LogEntry from ScoredLogEntry
	entries := make([]LogEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = e.LogEntry
	}
	return entries, nil
}

// GetServices returns a list of unique services
func (s *Store) GetServices() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT service FROM logs
		WHERE service IS NOT NULL AND service != ''
		ORDER BY service`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, nil
}

// GetStats returns log statistics
func (s *Store) GetStats(since time.Duration) (map[string]interface{}, error) {
	cutoff := time.Now().Add(-since)

	stats := make(map[string]interface{})

	// Total count
	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM logs WHERE timestamp >= ?", cutoff).Scan(&total)
	stats["total"] = total

	// Count by level
	levelCounts := make(map[string]int)
	rows, err := s.db.Query(`
		SELECT level, COUNT(*) FROM logs
		WHERE timestamp >= ?
		GROUP BY level`, cutoff)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var level string
			var count int
			rows.Scan(&level, &count)
			levelCounts[level] = count
		}
	}
	stats["by_level"] = levelCounts

	// Count by service (top 10)
	serviceCounts := make(map[string]int)
	rows, err = s.db.Query(`
		SELECT service, COUNT(*) as cnt FROM logs
		WHERE timestamp >= ? AND service IS NOT NULL AND service != ''
		GROUP BY service
		ORDER BY cnt DESC
		LIMIT 10`, cutoff)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var svc string
			var count int
			rows.Scan(&svc, &count)
			serviceCounts[svc] = count
		}
	}
	stats["by_service"] = serviceCounts

	return stats, nil
}

// Cleanup removes logs older than the specified duration
func (s *Store) Cleanup(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.Exec("DELETE FROM logs WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
