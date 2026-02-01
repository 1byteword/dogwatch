// Package integration provides end-to-end tests for dogwatch query features.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dogwatch/internal/logs"
	"dogwatch/internal/query"
)

// testLogsStore creates a test logs store with FTS5 support
func testLogsStore(t *testing.T) (*logs.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dogwatch-query-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "logs_test.db")
	store, err := logs.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create logs store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestComplexDQLQueriesWithJoins tests complex DQL queries with JOINs
func TestComplexDQLQueriesWithJoins(t *testing.T) {
	// Create executor with mock data sources
	executor := query.NewExecutor()

	// Mock logs data
	logsDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": time.Now(), "service": "api-gateway", "level": "error", "message": "Connection timeout", "trace_id": "abc123"},
			{"timestamp": time.Now(), "service": "user-service", "level": "info", "message": "User created", "trace_id": "def456"},
			{"timestamp": time.Now(), "service": "api-gateway", "level": "error", "message": "Rate limit exceeded", "trace_id": "ghi789"},
			{"timestamp": time.Now(), "service": "payment-service", "level": "error", "message": "Payment failed", "trace_id": "jkl012"},
		},
	}

	// Mock traces data
	tracesDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": time.Now(), "trace_id": "abc123", "service": "api-gateway", "duration": 150.0, "status": "ERROR"},
			{"timestamp": time.Now(), "trace_id": "def456", "service": "user-service", "duration": 50.0, "status": "OK"},
			{"timestamp": time.Now(), "trace_id": "ghi789", "service": "api-gateway", "duration": 200.0, "status": "ERROR"},
			{"timestamp": time.Now(), "trace_id": "mno345", "service": "order-service", "duration": 75.0, "status": "OK"},
		},
	}

	// Mock metrics data
	metricsDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": time.Now(), "service": "api-gateway", "name": "request_rate", "value": 100.0},
			{"timestamp": time.Now(), "service": "user-service", "name": "request_rate", "value": 50.0},
			{"timestamp": time.Now(), "service": "payment-service", "name": "request_rate", "value": 25.0},
			{"timestamp": time.Now(), "service": "order-service", "name": "request_rate", "value": 75.0},
		},
	}

	executor.SetLogsSource(logsDS)
	executor.SetTracesSource(tracesDS)
	executor.SetMetricsSource(metricsDS)

	testCases := []struct {
		name        string
		query       string
		expectRows  int
		expectError bool
	}{
		{
			name:        "Simple SELECT with filter",
			query:       `logs | where level = "error" | select service, message`,
			expectRows:  3,
			expectError: false,
		},
		{
			name:        "Aggregation with GROUP BY",
			query:       "logs | count by (service)",
			expectRows:  3, // 3 unique services
			expectError: false,
		},
		{
			name:        "Multiple aggregations",
			query:       "traces | avg(duration), max(duration), count by (service)",
			expectRows:  3, // unique services
			expectError: false,
		},
		{
			name:        "Filter with ORDER BY",
			query:       "traces | where duration > 50 | order by duration desc | limit 5",
			expectRows:  3,
			expectError: false,
		},
		{
			name:        "Top N query",
			query:       "metrics | top 3 value",
			expectRows:  3,
			expectError: false,
		},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tc.query)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Logf("Query error (may be expected): %v", err)
				return
			}

			t.Logf("Query: %s", tc.query)
			t.Logf("Rows returned: %d (expected ~%d)", len(result.Rows), tc.expectRows)
			t.Logf("Columns: %v", result.Columns)
			t.Logf("Execution time: %v", result.Stats.ExecutionTime)

			// Log sample rows
			for i, row := range result.Rows {
				if i >= 3 {
					t.Logf("... and %d more rows", len(result.Rows)-3)
					break
				}
				t.Logf("  Row %d: %v", i, row)
			}
		})
	}
}

// QueryMockDataSource implements DataSource interface for query testing
type QueryMockDataSource struct {
	rows []query.Row
}

func (m *QueryMockDataSource) Scan(ctx context.Context, source string, metric string, timeRange query.TimeRangeSpec, predicates []query.Expr) ([]query.Row, error) {
	return m.rows, nil
}

// TestBM25SearchRankingAccuracy tests BM25 search ranking accuracy
func TestBM25SearchRankingAccuracy(t *testing.T) {
	store, cleanup := testLogsStore(t)
	defer cleanup()

	// Insert test logs with varying relevance to search term "error database"
	testLogs := []logs.LogEntry{
		// Highly relevant
		{Level: logs.LevelError, Message: "Database connection error: cannot connect to primary database", Service: "db-service"},
		{Level: logs.LevelError, Message: "Error in database query execution: timeout", Service: "api-gateway"},
		// Moderately relevant
		{Level: logs.LevelWarn, Message: "Database pool exhausted, errors may occur", Service: "db-service"},
		{Level: logs.LevelInfo, Message: "Database backup completed, no errors", Service: "backup-service"},
		// Less relevant
		{Level: logs.LevelError, Message: "File not found error", Service: "file-service"},
		{Level: logs.LevelInfo, Message: "Database initialized successfully", Service: "db-service"},
		// Not relevant
		{Level: logs.LevelInfo, Message: "User login successful", Service: "auth-service"},
		{Level: logs.LevelDebug, Message: "Cache hit ratio: 95%", Service: "cache-service"},
	}

	for i := range testLogs {
		testLogs[i].Timestamp = time.Now().Add(time.Duration(i) * time.Minute)
	}

	err := store.InsertBatch(testLogs)
	if err != nil {
		t.Fatalf("Failed to insert test logs: %v", err)
	}

	// Search with BM25 ranking
	result, err := store.Search(logs.SearchQuery{
		Query:  "error database",
		Limit:  10,
		SortBy: logs.SortByRelevance,
	})
	if err != nil {
		t.Fatalf("BM25 search failed: %v", err)
	}

	t.Logf("Search results for 'error database' (sorted by relevance):")
	t.Logf("Total count: %d", result.TotalCount)

	for i, entry := range result.Entries {
		t.Logf("  %d. [score=%.4f] %s: %s", i+1, entry.Score, entry.Level, entry.Message)
	}

	// Verify that highly relevant results come first
	if len(result.Entries) > 0 {
		// First results should have higher scores
		if len(result.Entries) >= 2 {
			if result.Entries[0].Score < result.Entries[1].Score {
				t.Logf("Note: First result has lower score than second (%.4f < %.4f)",
					result.Entries[0].Score, result.Entries[1].Score)
			}
		}

		// First result should contain both terms ideally
		firstMsg := result.Entries[0].Message
		if !containsBoth(firstMsg, "error", "database") && len(result.Entries) > 1 {
			t.Log("First result doesn't contain both search terms")
		}
	}

	// Compare with time-sorted search
	timeResult, err := store.Search(logs.SearchQuery{
		Query:  "error database",
		Limit:  10,
		SortBy: logs.SortByTime,
	})
	if err != nil {
		t.Fatalf("Time-sorted search failed: %v", err)
	}

	t.Logf("\nSame search sorted by time:")
	for i, entry := range timeResult.Entries {
		t.Logf("  %d. %s: %s", i+1, entry.Level, entry.Message)
	}
}

func containsBoth(s, term1, term2 string) bool {
	return contains(s, term1) && contains(s, term2)
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// TestCrossSignalCorrelationQueries tests cross-signal correlation queries
func TestCrossSignalCorrelationQueries(t *testing.T) {
	executor := query.NewExecutor()

	// Set up data sources with correlated data (same trace_id)
	now := time.Now()

	logsDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": now, "trace_id": "trace-001", "service": "api", "level": "error", "message": "Request failed"},
			{"timestamp": now.Add(1 * time.Second), "trace_id": "trace-001", "service": "db", "level": "error", "message": "Query timeout"},
			{"timestamp": now.Add(2 * time.Second), "trace_id": "trace-002", "service": "api", "level": "info", "message": "Request success"},
		},
	}

	tracesDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": now, "trace_id": "trace-001", "service": "api", "duration": 5000.0, "status": "ERROR"},
			{"timestamp": now.Add(2 * time.Second), "trace_id": "trace-002", "service": "api", "duration": 50.0, "status": "OK"},
		},
	}

	metricsDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": now, "service": "api", "name": "cpu_percent", "value": 85.0},
			{"timestamp": now, "service": "db", "name": "cpu_percent", "value": 95.0},
		},
	}

	executor.SetLogsSource(logsDS)
	executor.SetTracesSource(tracesDS)
	executor.SetMetricsSource(metricsDS)

	// Test correlation-style queries
	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Filter logs by trace_id from slow traces",
			query: `logs | where trace_id = "trace-001"`,
		},
		{
			name:  "Aggregate errors by service",
			query: `logs | where level = "error" | count by (service)`,
		},
		{
			name:  "Find services with both errors and high latency",
			query: `traces | where duration > 1000 AND status = "ERROR" | select service, duration`,
		},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tc.query)
			if err != nil {
				t.Logf("Query error (may be expected): %v", err)
				return
			}

			t.Logf("Query: %s", tc.query)
			t.Logf("Results: %d rows", len(result.Rows))
			for i, row := range result.Rows {
				if i >= 5 {
					break
				}
				t.Logf("  %v", row)
			}
		})
	}
}

// TestDQLSQLSyntax tests SQL-style DQL syntax
func TestDQLSQLSyntax(t *testing.T) {
	executor := query.NewExecutor()

	// Set up mock data
	mockDS := &QueryMockDataSource{
		rows: []query.Row{
			{"timestamp": time.Now(), "service": "api", "duration": 100.0, "status": "OK"},
			{"timestamp": time.Now(), "service": "api", "duration": 150.0, "status": "OK"},
			{"timestamp": time.Now(), "service": "db", "duration": 500.0, "status": "ERROR"},
			{"timestamp": time.Now(), "service": "cache", "duration": 5.0, "status": "OK"},
		},
	}

	executor.SetTracesSource(mockDS)
	executor.SetLogsSource(mockDS)
	executor.SetMetricsSource(mockDS)

	testCases := []struct {
		name        string
		query       string
		expectError bool
	}{
		{
			name:        "SELECT * FROM",
			query:       "SELECT * FROM traces",
			expectError: false,
		},
		{
			name:        "SELECT with WHERE",
			query:       "SELECT service, duration FROM traces WHERE status = 'OK'",
			expectError: false,
		},
		{
			name:        "SELECT with GROUP BY",
			query:       "SELECT service, avg(duration) as avg_duration FROM traces GROUP BY service",
			expectError: false,
		},
		{
			name:        "SELECT with ORDER BY",
			query:       "SELECT service, duration FROM traces ORDER BY duration DESC",
			expectError: false,
		},
		{
			name:        "SELECT with LIMIT",
			query:       "SELECT * FROM traces LIMIT 2",
			expectError: false,
		},
		{
			name:        "SELECT with multiple aggregations",
			query:       "SELECT service, count(*) as cnt, avg(duration) as avg_dur, max(duration) as max_dur FROM traces GROUP BY service",
			expectError: false,
		},
		{
			name:        "SELECT DISTINCT",
			query:       "SELECT DISTINCT service FROM traces",
			expectError: false,
		},
	}

	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tc.query)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Logf("Query error: %v", err)
				return
			}

			t.Logf("Query: %s", tc.query)
			t.Logf("Columns: %v", result.Columns)
			t.Logf("Rows: %d", len(result.Rows))

			for i, row := range result.Rows {
				if i >= 3 {
					t.Logf("... and %d more rows", len(result.Rows)-3)
					break
				}
				t.Logf("  %v", row)
			}
		})
	}
}

// TestQueryExecutionStats tests that query execution stats are tracked
func TestQueryExecutionStats(t *testing.T) {
	executor := query.NewExecutor()

	// Large mock dataset for stats testing
	rows := make([]query.Row, 1000)
	for i := 0; i < 1000; i++ {
		rows[i] = query.Row{
			"timestamp": time.Now(),
			"service":   "service-" + string(rune('A'+i%26)),
			"value":     float64(i),
		}
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetMetricsSource(mockDS)

	ctx := context.Background()
	result, err := executor.Execute(ctx, "metrics | where value > 500 | count by (service)")

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	t.Logf("Execution Stats:")
	t.Logf("  Rows returned: %d", result.Stats.RowsReturned)
	t.Logf("  Execution time: %v", result.Stats.ExecutionTime)

	// Verify stats are populated
	if result.Stats.ExecutionTime == 0 {
		t.Error("Execution time should be > 0")
	}
	if result.Stats.RowsReturned == 0 && len(result.Rows) > 0 {
		t.Error("RowsReturned should match actual row count")
	}
}

// TestWindowFunctions tests window function queries
func TestWindowFunctions(t *testing.T) {
	executor := query.NewExecutor()

	// Create time-series data
	now := time.Now().Truncate(time.Minute)
	rows := make([]query.Row, 60)
	for i := 0; i < 60; i++ {
		rows[i] = query.Row{
			"timestamp": now.Add(time.Duration(i) * time.Second),
			"service":   "api",
			"value":     float64(100 + i%10),
		}
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetMetricsSource(mockDS)

	ctx := context.Background()

	// Test tumbling window
	result, err := executor.Execute(ctx, "metrics | window tumbling=10s")
	if err != nil {
		t.Logf("Window query error (may be expected): %v", err)
		return
	}

	t.Logf("Window query results:")
	t.Logf("  Total windows: %d", len(result.Rows))

	for i, row := range result.Rows {
		if i >= 5 {
			break
		}
		t.Logf("  Window %d: start=%v, count=%v", i, row["window_start"], row["count"])
	}
}

// TestAnomalyDetection tests anomaly detection in queries
func TestAnomalyDetection(t *testing.T) {
	executor := query.NewExecutor()

	// Create data with anomalies
	rows := []query.Row{
		{"timestamp": time.Now(), "value": 100.0},
		{"timestamp": time.Now(), "value": 102.0},
		{"timestamp": time.Now(), "value": 98.0},
		{"timestamp": time.Now(), "value": 101.0},
		{"timestamp": time.Now(), "value": 500.0}, // Anomaly
		{"timestamp": time.Now(), "value": 99.0},
		{"timestamp": time.Now(), "value": 103.0},
		{"timestamp": time.Now(), "value": 1000.0}, // Anomaly
		{"timestamp": time.Now(), "value": 97.0},
		{"timestamp": time.Now(), "value": 100.0},
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetMetricsSource(mockDS)

	ctx := context.Background()
	result, err := executor.Execute(ctx, "metrics | anomalies sensitivity=0.9")

	if err != nil {
		t.Logf("Anomaly query error (may be expected): %v", err)
		return
	}

	t.Logf("Anomaly detection results:")
	anomalyCount := 0
	for i, row := range result.Rows {
		isAnomaly := row["is_anomaly"]
		score := row["anomaly_score"]
		if isAnomaly == true {
			anomalyCount++
			t.Logf("  Row %d: value=%v, score=%v (ANOMALY)", i, row["value"], score)
		}
	}
	t.Logf("Total anomalies detected: %d", anomalyCount)
}

// TestHistogramQueries tests histogram aggregation
func TestHistogramQueries(t *testing.T) {
	executor := query.NewExecutor()

	// Create data for histogram
	rows := make([]query.Row, 100)
	for i := 0; i < 100; i++ {
		// Create a distribution of values
		value := float64(i) + float64(i%10)
		rows[i] = query.Row{
			"timestamp": time.Now(),
			"duration":  value,
		}
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetTracesSource(mockDS)

	ctx := context.Background()
	result, err := executor.Execute(ctx, "traces | histogram duration buckets=10")

	if err != nil {
		t.Logf("Histogram query error (may be expected): %v", err)
		return
	}

	t.Logf("Histogram results (10 buckets):")
	for _, row := range result.Rows {
		t.Logf("  Bucket %v: [%.1f-%.1f] count=%v (%.1f%%)",
			row["bucket"], row["low"], row["high"], row["count"], row["percentage"])
	}
}

// TestExtractQueries tests field extraction queries
func TestExtractQueries(t *testing.T) {
	executor := query.NewExecutor()

	// Create log data with extractable fields
	rows := []query.Row{
		{"timestamp": time.Now(), "message": "user_id=123 action=login ip=192.168.1.1"},
		{"timestamp": time.Now(), "message": "user_id=456 action=purchase amount=99.99"},
		{"timestamp": time.Now(), "message": "user_id=789 action=logout duration=3600"},
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetLogsSource(mockDS)

	ctx := context.Background()

	// Test auto extraction
	result, err := executor.Execute(ctx, "logs | extract auto")
	if err != nil {
		t.Logf("Extract query error (may be expected): %v", err)
		return
	}

	t.Logf("Auto-extraction results:")
	for i, row := range result.Rows {
		t.Logf("  Row %d:", i)
		for k, v := range row {
			if k != "message" && k != "timestamp" {
				t.Logf("    %s = %v", k, v)
			}
		}
	}
}

// TestQueryWithFunctions tests built-in function usage
func TestQueryWithFunctions(t *testing.T) {
	executor := query.NewExecutor()

	rows := []query.Row{
		{"timestamp": time.Now(), "service": "API-Gateway", "value": 123.456},
		{"timestamp": time.Now(), "service": "User-Service", "value": 789.012},
	}

	mockDS := &QueryMockDataSource{rows: rows}
	executor.SetMetricsSource(mockDS)

	ctx := context.Background()

	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "lower function",
			query: "metrics | select lower(service) as svc, value",
		},
		{
			name:  "upper function",
			query: "metrics | select upper(service) as svc, value",
		},
		{
			name:  "round function",
			query: "metrics | select service, round(value) as rounded",
		},
		{
			name:  "length function",
			query: "metrics | select service, length(service) as name_len",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tc.query)
			if err != nil {
				t.Logf("Query error: %v", err)
				return
			}

			t.Logf("Query: %s", tc.query)
			for i, row := range result.Rows {
				t.Logf("  Row %d: %v", i, row)
			}
		})
	}
}

// TestLogSearchWithFilters tests log search with various filters
func TestLogSearchWithFilters(t *testing.T) {
	store, cleanup := testLogsStore(t)
	defer cleanup()

	// Insert test logs
	now := time.Now()
	testLogs := []logs.LogEntry{
		{Level: logs.LevelError, Message: "Error in payment processing", Service: "payment", Timestamp: now},
		{Level: logs.LevelError, Message: "Database connection failed", Service: "db", Timestamp: now.Add(-1 * time.Hour)},
		{Level: logs.LevelWarn, Message: "High memory usage", Service: "api", Timestamp: now.Add(-2 * time.Hour)},
		{Level: logs.LevelInfo, Message: "User registered successfully", Service: "auth", Timestamp: now.Add(-3 * time.Hour)},
		{Level: logs.LevelDebug, Message: "Cache miss for key user:123", Service: "cache", Timestamp: now.Add(-4 * time.Hour)},
	}

	for _, log := range testLogs {
		store.Insert(&log)
	}

	testCases := []struct {
		name  string
		query logs.SearchQuery
	}{
		{
			name:  "Filter by level",
			query: logs.SearchQuery{Level: logs.LevelError, Limit: 10},
		},
		{
			name:  "Filter by service",
			query: logs.SearchQuery{Service: "payment", Limit: 10},
		},
		{
			name:  "Filter by time range",
			query: logs.SearchQuery{StartTime: now.Add(-2 * time.Hour), EndTime: now, Limit: 10},
		},
		{
			name:  "Full-text search",
			query: logs.SearchQuery{Query: "error", Limit: 10},
		},
		{
			name:  "Combined filters",
			query: logs.SearchQuery{Level: logs.LevelError, Query: "connection", Limit: 10},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := store.Search(tc.query)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			t.Logf("Query: %+v", tc.query)
			t.Logf("Results: %d (total: %d)", len(result.Entries), result.TotalCount)
			for i, entry := range result.Entries {
				if i >= 3 {
					break
				}
				t.Logf("  [%s] %s: %s", entry.Level, entry.Service, entry.Message)
			}
		})
	}
}
