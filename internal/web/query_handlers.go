package web

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"dogwatch/internal/logs"

	"github.com/google/uuid"
)

// QueryMetadata describes available fields and functions for query building
type QueryMetadata struct {
	Fields    map[string][]FieldInfo `json:"fields"`
	Functions []FunctionInfo         `json:"functions"`
	Operators []string               `json:"operators"`
	Syntax    SyntaxInfo             `json:"syntax"`
}

// SyntaxInfo describes supported query syntaxes
type SyntaxInfo struct {
	Pipe PipeSyntaxInfo `json:"pipe"`
	SQL  SQLSyntaxInfo  `json:"sql"`
}

// PipeSyntaxInfo describes pipe-based query syntax
type PipeSyntaxInfo struct {
	Example     string   `json:"example"`
	Description string   `json:"description"`
	Operations  []string `json:"operations"`
}

// SQLSyntaxInfo describes SQL query syntax
type SQLSyntaxInfo struct {
	Example     string   `json:"example"`
	Description string   `json:"description"`
	JoinTypes   []string `json:"joinTypes"`
	Clauses     []string `json:"clauses"`
}

// FieldInfo describes a queryable field
type FieldInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Desc string `json:"description,omitempty"`
}

// FunctionInfo describes an aggregation function
type FunctionInfo struct {
	Name   string `json:"name"`
	Syntax string `json:"syntax"`
	Desc   string `json:"description"`
}

// SavedQuery represents a user-saved query
type SavedQuery struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Query     string                 `json:"query"`
	Source    string                 `json:"source"`
	Pipeline  []map[string]interface{} `json:"pipeline,omitempty"`
	TimeRange string                 `json:"timeRange,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// QueryExecuteRequest is the request body for query execution
type QueryExecuteRequest struct {
	Query     string `json:"query"`
	TimeRange string `json:"timeRange"`
}

// QueryExecuteResponse is the response for query execution
type QueryExecuteResponse struct {
	Rows    []map[string]interface{} `json:"rows"`
	Columns []string                 `json:"columns,omitempty"`
	Count   int                      `json:"count"`
	Error   string                   `json:"error,omitempty"`
}

// In-memory saved queries store (in production, use database)
var savedQueries = make(map[string]*SavedQuery)
var savedQueriesMu sync.RWMutex

// handleQueryMetadata returns metadata about available fields and functions
func (s *Server) handleQueryMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := QueryMetadata{
		Fields: map[string][]FieldInfo{
			"metrics": {
				{Name: "timestamp", Type: "time", Desc: "Metric timestamp"},
				{Name: "name", Type: "string", Desc: "Metric name"},
				{Name: "value", Type: "number", Desc: "Metric value"},
				{Name: "host", Type: "string", Desc: "Host name"},
				{Name: "service", Type: "string", Desc: "Service name"},
				{Name: "labels", Type: "object", Desc: "Metric labels"},
			},
			"logs": {
				{Name: "timestamp", Type: "time", Desc: "Log timestamp"},
				{Name: "level", Type: "string", Desc: "Log level (info, warn, error)"},
				{Name: "service", Type: "string", Desc: "Service name"},
				{Name: "message", Type: "string", Desc: "Log message"},
				{Name: "trace_id", Type: "string", Desc: "Trace correlation ID"},
				{Name: "span_id", Type: "string", Desc: "Span correlation ID"},
				{Name: "host", Type: "string", Desc: "Host name"},
				{Name: "attributes", Type: "object", Desc: "Additional attributes"},
			},
			"traces": {
				{Name: "timestamp", Type: "time", Desc: "Span start time"},
				{Name: "trace_id", Type: "string", Desc: "Trace ID"},
				{Name: "span_id", Type: "string", Desc: "Span ID"},
				{Name: "parent_span_id", Type: "string", Desc: "Parent span ID"},
				{Name: "service", Type: "string", Desc: "Service name"},
				{Name: "operation", Type: "string", Desc: "Operation name"},
				{Name: "duration", Type: "number", Desc: "Duration in milliseconds"},
				{Name: "status", Type: "string", Desc: "Span status (ok, error)"},
				{Name: "attributes", Type: "object", Desc: "Span attributes"},
			},
			"events": {
				{Name: "timestamp", Type: "time", Desc: "Event timestamp"},
				{Name: "type", Type: "string", Desc: "Event type"},
				{Name: "source", Type: "string", Desc: "Event source"},
				{Name: "message", Type: "string", Desc: "Event message"},
				{Name: "severity", Type: "string", Desc: "Event severity"},
			},
		},
		Functions: []FunctionInfo{
			// Aggregation functions
			{Name: "avg", Syntax: "avg(field)", Desc: "Calculate average value"},
			{Name: "sum", Syntax: "sum(field)", Desc: "Calculate sum of values"},
			{Name: "count", Syntax: "count()", Desc: "Count number of rows"},
			{Name: "min", Syntax: "min(field)", Desc: "Find minimum value"},
			{Name: "max", Syntax: "max(field)", Desc: "Find maximum value"},
			{Name: "p50", Syntax: "p50(field)", Desc: "50th percentile (median)"},
			{Name: "p90", Syntax: "p90(field)", Desc: "90th percentile"},
			{Name: "p95", Syntax: "p95(field)", Desc: "95th percentile"},
			{Name: "p99", Syntax: "p99(field)", Desc: "99th percentile"},
			{Name: "rate", Syntax: "rate(field)", Desc: "Rate of change per second"},
			{Name: "stddev", Syntax: "stddev(field)", Desc: "Standard deviation"},
			{Name: "distinct", Syntax: "distinct(field)", Desc: "Count distinct values"},
			// Time functions
			{Name: "time_bucket", Syntax: "time_bucket('1h', timestamp)", Desc: "Truncate timestamp to interval (1m, 5m, 1h, 1d)"},
			{Name: "date_trunc", Syntax: "date_trunc('hour', timestamp)", Desc: "Truncate to precision (second, minute, hour, day, month, year)"},
			{Name: "extract", Syntax: "extract('hour', timestamp)", Desc: "Extract part from timestamp (year, month, day, hour, etc.)"},
			// String functions
			{Name: "lower", Syntax: "lower(string)", Desc: "Convert to lowercase"},
			{Name: "upper", Syntax: "upper(string)", Desc: "Convert to uppercase"},
			{Name: "concat", Syntax: "concat(a, b, ...)", Desc: "Concatenate strings"},
			{Name: "substring", Syntax: "substring(string, start, length)", Desc: "Extract substring"},
			{Name: "contains", Syntax: "contains(string, substr)", Desc: "Check if string contains substring"},
			{Name: "replace", Syntax: "replace(string, old, new)", Desc: "Replace occurrences"},
			// Math functions
			{Name: "abs", Syntax: "abs(number)", Desc: "Absolute value"},
			{Name: "round", Syntax: "round(number)", Desc: "Round to nearest integer"},
			{Name: "floor", Syntax: "floor(number)", Desc: "Round down"},
			{Name: "ceil", Syntax: "ceil(number)", Desc: "Round up"},
			{Name: "sqrt", Syntax: "sqrt(number)", Desc: "Square root"},
			{Name: "pow", Syntax: "pow(base, exp)", Desc: "Power function"},
			// Utility functions
			{Name: "coalesce", Syntax: "coalesce(a, b, ...)", Desc: "Return first non-null value"},
			{Name: "now", Syntax: "now()", Desc: "Current timestamp"},
		},
		Operators: []string{"=", "!=", ">", "<", ">=", "<=", "LIKE", "IN", "NOT IN", "BETWEEN", "AND", "OR", "NOT"},
		Syntax: SyntaxInfo{
			Pipe: PipeSyntaxInfo{
				Example:     "logs | where service = 'api' | avg(latency) by (endpoint) | order by avg_latency desc",
				Description: "Pipe-based syntax for streaming data transformations",
				Operations:  []string{"where", "select", "avg", "sum", "count", "min", "max", "order by", "limit", "window", "correlate", "histogram", "extract", "anomalies"},
			},
			SQL: SQLSyntaxInfo{
				Example:     "SELECT endpoint, avg(latency) as avg_latency FROM logs WHERE service = 'api' GROUP BY endpoint HAVING count(*) > 10 ORDER BY avg_latency DESC",
				Description: "SQL-style syntax with JOINs for cross-signal queries",
				JoinTypes:   []string{"INNER JOIN", "LEFT JOIN", "RIGHT JOIN", "FULL JOIN"},
				Clauses:     []string{"SELECT", "DISTINCT", "FROM", "JOIN", "ON", "WHERE", "GROUP BY", "HAVING", "ORDER BY", "LIMIT", "OFFSET"},
			},
		},
	}

	// Add dynamic fields if stores are available
	if s.customMetricsStore != nil {
		// Could fetch actual metric names
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// handleQueryExecute executes a WatchQL query
func (s *Server) handleQueryExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QueryExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("[query] Executing query: %s (timeRange: %s)", req.Query, req.TimeRange)

	// Parse time range
	duration := parseQueryTimeRange(req.TimeRange)
	startTime := time.Now().Add(-duration)
	endTime := time.Now()

	// Execute based on data source
	var rows []map[string]interface{}
	var err error

	// Simple query parsing - extract source
	source := extractSource(req.Query)

	switch source {
	case "metrics":
		rows, err = s.executeMetricsQuery(req.Query, startTime, endTime)
	case "logs":
		rows, err = s.executeLogsQuery(req.Query, startTime, endTime)
	case "traces":
		rows, err = s.executeTracesQuery(req.Query, startTime, endTime)
	case "events":
		rows, err = s.executeEventsQuery(req.Query, startTime, endTime)
	default:
		// Try metrics as default
		rows, err = s.executeMetricsQuery(req.Query, startTime, endTime)
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(QueryExecuteResponse{
			Error: err.Error(),
		})
		return
	}

	response := QueryExecuteResponse{
		Rows:  rows,
		Count: len(rows),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// executeMetricsQuery executes a query against metrics data
func (s *Server) executeMetricsQuery(query string, start, end time.Time) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}

	// If we have custom metrics store, use it
	if s.customMetricsStore != nil {
		metrics, err := s.customMetricsStore.List()
		if err == nil {
			for _, m := range metrics {
				rows = append(rows, map[string]interface{}{
					"name":      m.Name,
					"type":      m.Type,
					"timestamp": time.Now(),
				})
			}
		}
	}

	// Add system metrics from collector
	if s.metrics != nil {
		sysMetrics := s.metrics.Collect()
		now := time.Now()
		rows = append(rows, map[string]interface{}{
			"name":      "cpu_percent",
			"value":     sysMetrics.CPUUsagePercent,
			"timestamp": now,
		})
		rows = append(rows, map[string]interface{}{
			"name":      "memory_percent",
			"value":     sysMetrics.MemUsagePercent,
			"timestamp": now,
		})
		rows = append(rows, map[string]interface{}{
			"name":      "memory_used_bytes",
			"value":     float64(sysMetrics.MemUsedBytes),
			"timestamp": now,
		})
		rows = append(rows, map[string]interface{}{
			"name":      "memory_total_bytes",
			"value":     float64(sysMetrics.MemTotalBytes),
			"timestamp": now,
		})
	}

	return rows, nil
}

// executeLogsQuery executes a query against logs data
func (s *Server) executeLogsQuery(query string, start, end time.Time) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}

	if s.logStore != nil {
		searchQuery := logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Limit:     1000,
		}
		result, err := s.logStore.Search(searchQuery)
		if err == nil && result != nil {
			for _, e := range result.Entries {
				rows = append(rows, map[string]interface{}{
					"timestamp": e.Timestamp,
					"level":     string(e.Level),
					"service":   e.Service,
					"message":   e.Message,
					"trace_id":  e.TraceID,
				})
			}
		}
	}

	return rows, nil
}

// executeTracesQuery executes a query against traces data
func (s *Server) executeTracesQuery(query string, start, end time.Time) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}

	if s.traceStore != nil {
		since := time.Since(start)
		traces, err := s.traceStore.ListTraces(100, "", since)
		if err == nil {
			for _, t := range traces {
				rows = append(rows, map[string]interface{}{
					"trace_id":   t.TraceID,
					"service":    t.ServiceName,
					"operation":  t.RootSpan,
					"duration":   t.DurationMs,
					"timestamp":  t.StartTime,
					"span_count": t.SpanCount,
				})
			}
		}
	}

	return rows, nil
}

// executeEventsQuery executes a query against events data
func (s *Server) executeEventsQuery(query string, start, end time.Time) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}

	// Combine events from various sources
	if s.watchStore != nil {
		events, err := s.watchStore.GetEvents(100, "")
		if err == nil {
			for _, e := range events {
				rows = append(rows, map[string]interface{}{
					"timestamp": e.Timestamp,
					"type":      "watch",
					"source":    e.WatchName,
					"message":   e.Message,
					"severity":  string(e.ToState),
				})
			}
		}
	}

	if s.incidentStore != nil {
		incidents, err := s.incidentStore.ListIncidents("", 100)
		if err == nil {
			for _, i := range incidents {
				rows = append(rows, map[string]interface{}{
					"timestamp": i.CreatedAt,
					"type":      "incident",
					"source":    i.Title,
					"message":   i.Description,
					"severity":  i.Severity,
				})
			}
		}
	}

	return rows, nil
}

// handleSavedQueries handles CRUD for saved queries
func (s *Server) handleSavedQueries(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listSavedQueries(w, r)
	case http.MethodPost:
		s.createSavedQuery(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listSavedQueries returns all saved queries
func (s *Server) listSavedQueries(w http.ResponseWriter, r *http.Request) {
	savedQueriesMu.RLock()
	defer savedQueriesMu.RUnlock()

	queries := make([]*SavedQuery, 0, len(savedQueries))
	for _, q := range savedQueries {
		queries = append(queries, q)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// createSavedQuery creates a new saved query
func (s *Server) createSavedQuery(w http.ResponseWriter, r *http.Request) {
	var query SavedQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query.ID = uuid.New().String()
	query.CreatedAt = time.Now()
	query.UpdatedAt = time.Now()

	savedQueriesMu.Lock()
	savedQueries[query.ID] = &query
	savedQueriesMu.Unlock()

	log.Printf("[query] Saved query: %s (%s)", query.Name, query.ID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(query)
}

// handleSavedQuery handles operations on a single saved query
func (s *Server) handleSavedQuery(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/query/saved/"):]
	if id == "" {
		http.Error(w, "Query ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		savedQueriesMu.RLock()
		query, ok := savedQueries[id]
		savedQueriesMu.RUnlock()

		if !ok {
			http.Error(w, "Query not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(query)

	case http.MethodPut:
		var update SavedQuery
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		savedQueriesMu.Lock()
		query, ok := savedQueries[id]
		if !ok {
			savedQueriesMu.Unlock()
			http.Error(w, "Query not found", http.StatusNotFound)
			return
		}

		query.Name = update.Name
		query.Query = update.Query
		query.Source = update.Source
		query.Pipeline = update.Pipeline
		query.TimeRange = update.TimeRange
		query.UpdatedAt = time.Now()
		savedQueriesMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(query)

	case http.MethodDelete:
		savedQueriesMu.Lock()
		delete(savedQueries, id)
		savedQueriesMu.Unlock()

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Helper functions

func extractSource(query string) string {
	// Simple extraction - find first word for pipe syntax
	for _, source := range []string{"metrics", "logs", "traces", "events"} {
		if len(query) >= len(source) && query[:len(source)] == source {
			return source
		}
	}

	// Handle SQL syntax - extract FROM clause
	lowerQuery := strings.ToLower(query)
	if strings.HasPrefix(lowerQuery, "select") {
		fromIdx := strings.Index(lowerQuery, "from ")
		if fromIdx != -1 {
			remainder := strings.TrimSpace(lowerQuery[fromIdx+5:])
			for _, source := range []string{"metrics", "logs", "traces", "events"} {
				if strings.HasPrefix(remainder, source) {
					return source
				}
			}
		}
	}

	return "metrics"
}

func parseQueryTimeRange(s string) time.Duration {
	// Parse duration strings like "5m", "1h", "24h", "7d"
	if len(s) < 2 {
		return 15 * time.Minute
	}

	unit := s[len(s)-1]
	value := s[:len(s)-1]

	var num int
	for _, c := range value {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}

	if num == 0 {
		num = 15
	}

	switch unit {
	case 'm':
		return time.Duration(num) * time.Minute
	case 'h':
		return time.Duration(num) * time.Hour
	case 'd':
		return time.Duration(num) * 24 * time.Hour
	default:
		return time.Duration(num) * time.Minute
	}
}

// RegisterQueryRoutes registers the query builder API routes
func RegisterQueryRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/query/metadata", s.handleQueryMetadata)
	mux.HandleFunc("/api/query/execute", s.handleQueryExecute)
	mux.HandleFunc("/api/query/saved", s.handleSavedQueries)
	mux.HandleFunc("/api/query/saved/", s.handleSavedQuery)
}
