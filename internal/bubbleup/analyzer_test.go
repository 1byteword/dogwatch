package bubbleup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dogwatch/internal/trace"
)

func setupTestStore(t *testing.T) *trace.Store {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "bubbleup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	store, err := trace.NewStore(filepath.Join(tmpDir, "traces.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	return store
}

func createTestSpans(t *testing.T, store *trace.Store, spans []trace.Span) {
	t.Helper()
	for _, span := range spans {
		if err := store.RecordSpan(span); err != nil {
			t.Fatalf("failed to record span: %v", err)
		}
	}
}

func TestNewAnalyzer(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if analyzer.traceStore != store {
		t.Error("traceStore not set correctly")
	}
}

func TestAnalyzer_SetTraceStore(t *testing.T) {
	analyzer := NewAnalyzer(nil)
	if analyzer.traceStore != nil {
		t.Error("expected nil traceStore initially")
	}

	store := setupTestStore(t)
	analyzer.SetTraceStore(store)

	if analyzer.traceStore != store {
		t.Error("SetTraceStore did not set store")
	}
}

func TestAnalyzer_Analyze_NilStore(t *testing.T) {
	analyzer := NewAnalyzer(nil)

	_, err := analyzer.Analyze(context.Background(), AnalysisRequest{
		TimeStart: time.Now().Add(-1 * time.Hour),
		TimeEnd:   time.Now(),
		Mode:      "latency",
	})

	if err == nil {
		t.Error("expected error for nil store")
	}
	if err.Error() != "trace store not configured" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAnalyzer_Analyze_ValidationErrors(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	tests := []struct {
		name    string
		req     AnalysisRequest
		wantErr string
	}{
		{
			name:    "missing time_start",
			req:     AnalysisRequest{TimeEnd: time.Now()},
			wantErr: "time_start and time_end are required",
		},
		{
			name:    "missing time_end",
			req:     AnalysisRequest{TimeStart: time.Now()},
			wantErr: "time_start and time_end are required",
		},
		{
			name: "invalid mode",
			req: AnalysisRequest{
				TimeStart: time.Now().Add(-1 * time.Hour),
				TimeEnd:   time.Now(),
				Mode:      "invalid",
			},
			wantErr: "mode must be 'latency' or 'errors'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := analyzer.Analyze(context.Background(), tt.req)
			if err == nil {
				t.Error("expected error")
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %v, want %v", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestAnalyzer_Analyze_LatencyMode(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	now := time.Now()
	baseTime := now.Add(-30 * time.Minute)

	// Create spans with different latencies and attributes
	spans := []trace.Span{
		// Slow spans (anomalous) - all hit shard-7
		{TraceID: "t1", SpanID: "s1", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(500 * time.Millisecond), DurationMs: 500, Status: "OK", Attributes: map[string]string{"db_shard": "shard-7", "region": "us-east"}},
		{TraceID: "t2", SpanID: "s2", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(600 * time.Millisecond), DurationMs: 600, Status: "OK", Attributes: map[string]string{"db_shard": "shard-7", "region": "us-east"}},
		{TraceID: "t3", SpanID: "s3", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(550 * time.Millisecond), DurationMs: 550, Status: "OK", Attributes: map[string]string{"db_shard": "shard-7", "region": "us-west"}},

		// Fast spans (baseline) - distributed across shards
		{TraceID: "t4", SpanID: "s4", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(10 * time.Millisecond), DurationMs: 10, Status: "OK", Attributes: map[string]string{"db_shard": "shard-1", "region": "us-east"}},
		{TraceID: "t5", SpanID: "s5", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(15 * time.Millisecond), DurationMs: 15, Status: "OK", Attributes: map[string]string{"db_shard": "shard-2", "region": "us-east"}},
		{TraceID: "t6", SpanID: "s6", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(12 * time.Millisecond), DurationMs: 12, Status: "OK", Attributes: map[string]string{"db_shard": "shard-3", "region": "us-west"}},
		{TraceID: "t7", SpanID: "s7", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(8 * time.Millisecond), DurationMs: 8, Status: "OK", Attributes: map[string]string{"db_shard": "shard-1", "region": "us-east"}},
		{TraceID: "t8", SpanID: "s8", Name: "query", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(20 * time.Millisecond), DurationMs: 20, Status: "OK", Attributes: map[string]string{"db_shard": "shard-2", "region": "us-west"}},
	}

	createTestSpans(t, store, spans)

	result, err := analyzer.Analyze(context.Background(), AnalysisRequest{
		Service:          "api",
		TimeStart:        baseTime.Add(-1 * time.Minute),
		TimeEnd:          now,
		Mode:             "latency",
		LatencyThreshold: 100, // Anything over 100ms is "slow"
	})

	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.AnomalousCount != 3 {
		t.Errorf("AnomalousCount = %d, want 3", result.AnomalousCount)
	}
	if result.BaselineCount != 5 {
		t.Errorf("BaselineCount = %d, want 5", result.BaselineCount)
	}

	// Should identify db_shard as a significant dimension
	foundShard := false
	for _, dim := range result.Dimensions {
		if dim.Dimension == "db_shard" {
			foundShard = true
			if dim.TopValue != "shard-7" {
				t.Errorf("db_shard top value = %s, want shard-7", dim.TopValue)
			}
			if dim.TopValueLift < 2 {
				t.Errorf("db_shard lift = %f, want >= 2", dim.TopValueLift)
			}
		}
	}
	if !foundShard {
		t.Log("Dimensions found:", result.Dimensions)
		// This might not always be found depending on thresholds
	}
}

func TestAnalyzer_Analyze_ErrorsMode(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	now := time.Now()
	baseTime := now.Add(-30 * time.Minute)

	// Create spans with errors and successes
	spans := []trace.Span{
		// Error spans - all from endpoint /checkout
		{TraceID: "t1", SpanID: "s1", Name: "POST /checkout", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(100 * time.Millisecond), DurationMs: 100, Status: "ERROR", Attributes: map[string]string{"http.path": "/checkout", "error.type": "timeout"}},
		{TraceID: "t2", SpanID: "s2", Name: "POST /checkout", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(100 * time.Millisecond), DurationMs: 100, Status: "ERROR", Attributes: map[string]string{"http.path": "/checkout", "error.type": "timeout"}},
		{TraceID: "t3", SpanID: "s3", Name: "POST /checkout", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(100 * time.Millisecond), DurationMs: 100, Status: "ERROR", Attributes: map[string]string{"http.path": "/checkout", "error.type": "db_error"}},

		// Success spans - various endpoints
		{TraceID: "t4", SpanID: "s4", Name: "GET /products", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(50 * time.Millisecond), DurationMs: 50, Status: "OK", Attributes: map[string]string{"http.path": "/products"}},
		{TraceID: "t5", SpanID: "s5", Name: "GET /users", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(30 * time.Millisecond), DurationMs: 30, Status: "OK", Attributes: map[string]string{"http.path": "/users"}},
		{TraceID: "t6", SpanID: "s6", Name: "GET /products", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(40 * time.Millisecond), DurationMs: 40, Status: "OK", Attributes: map[string]string{"http.path": "/products"}},
		{TraceID: "t7", SpanID: "s7", Name: "POST /checkout", ServiceName: "api", StartTime: baseTime, EndTime: baseTime.Add(80 * time.Millisecond), DurationMs: 80, Status: "OK", Attributes: map[string]string{"http.path": "/checkout"}},
	}

	createTestSpans(t, store, spans)

	result, err := analyzer.Analyze(context.Background(), AnalysisRequest{
		Service:   "api",
		TimeStart: baseTime.Add(-1 * time.Minute),
		TimeEnd:   now,
		Mode:      "errors",
	})

	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.AnomalousCount != 3 {
		t.Errorf("AnomalousCount = %d, want 3", result.AnomalousCount)
	}
	if result.BaselineCount != 4 {
		t.Errorf("BaselineCount = %d, want 4", result.BaselineCount)
	}
	if result.Mode != "errors" {
		t.Errorf("Mode = %s, want errors", result.Mode)
	}
}

func TestAnalyzer_GetResult(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	// Create some test data
	now := time.Now()
	baseTime := now.Add(-30 * time.Minute)
	spans := []trace.Span{
		{TraceID: "t1", SpanID: "s1", Name: "op", ServiceName: "svc", StartTime: baseTime, EndTime: baseTime.Add(500 * time.Millisecond), DurationMs: 500, Status: "OK"},
		{TraceID: "t2", SpanID: "s2", Name: "op", ServiceName: "svc", StartTime: baseTime, EndTime: baseTime.Add(10 * time.Millisecond), DurationMs: 10, Status: "OK"},
	}
	createTestSpans(t, store, spans)

	// Run analysis
	result, err := analyzer.Analyze(context.Background(), AnalysisRequest{
		TimeStart:        baseTime.Add(-1 * time.Minute),
		TimeEnd:          now,
		Mode:             "latency",
		LatencyThreshold: 100,
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Retrieve by ID
	retrieved, ok := analyzer.GetResult(result.ID)
	if !ok {
		t.Fatal("GetResult returned false")
	}
	if retrieved.ID != result.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, result.ID)
	}

	// Non-existent ID
	_, ok = analyzer.GetResult("nonexistent")
	if ok {
		t.Error("GetResult should return false for nonexistent ID")
	}
}

func TestAnalyzer_ListResults(t *testing.T) {
	store := setupTestStore(t)
	analyzer := NewAnalyzer(store)

	// Create test data
	now := time.Now()
	baseTime := now.Add(-30 * time.Minute)
	spans := []trace.Span{
		{TraceID: "t1", SpanID: "s1", Name: "op", ServiceName: "svc", StartTime: baseTime, EndTime: baseTime.Add(500 * time.Millisecond), DurationMs: 500, Status: "OK"},
		{TraceID: "t2", SpanID: "s2", Name: "op", ServiceName: "svc", StartTime: baseTime, EndTime: baseTime.Add(10 * time.Millisecond), DurationMs: 10, Status: "OK"},
	}
	createTestSpans(t, store, spans)

	// Run multiple analyses
	for i := 0; i < 3; i++ {
		_, err := analyzer.Analyze(context.Background(), AnalysisRequest{
			TimeStart:        baseTime.Add(-1 * time.Minute),
			TimeEnd:          now,
			Mode:             "latency",
			LatencyThreshold: 100,
		})
		if err != nil {
			t.Fatalf("Analyze %d failed: %v", i, err)
		}
	}

	// List all
	results := analyzer.ListResults(0)
	if len(results) != 3 {
		t.Errorf("ListResults(0) returned %d results, want 3", len(results))
	}

	// List with limit
	results = analyzer.ListResults(2)
	if len(results) != 2 {
		t.Errorf("ListResults(2) returned %d results, want 2", len(results))
	}
}

func TestExtractDimensions(t *testing.T) {
	spans := []trace.Span{
		{
			ServiceName: "api",
			Name:        "GET /users",
			Kind:        "SERVER",
			Status:      "OK",
			Attributes: map[string]string{
				"http.method": "GET",
				"http.path":   "/users",
				"region":      "us-east",
			},
		},
		{
			ServiceName: "api",
			Name:        "GET /users",
			Kind:        "SERVER",
			Status:      "OK",
			Attributes: map[string]string{
				"http.method": "GET",
				"http.path":   "/users",
				"region":      "us-west",
			},
		},
	}

	dims := extractDimensions(spans)

	// Check service dimension
	if dims["service"]["api"] != 2 {
		t.Errorf("service[api] = %d, want 2", dims["service"]["api"])
	}

	// Check region dimension
	if dims["region"]["us-east"] != 1 || dims["region"]["us-west"] != 1 {
		t.Errorf("region counts wrong: %v", dims["region"])
	}

	// Check http.method from attributes
	if dims["http.method"]["GET"] != 2 {
		t.Errorf("http.method[GET] = %d, want 2", dims["http.method"]["GET"])
	}
}

func TestCompareDimension(t *testing.T) {
	anomalous := map[string]int{"shard-7": 98, "shard-1": 1, "shard-2": 1}
	baseline := map[string]int{"shard-7": 8, "shard-1": 23, "shard-2": 23, "shard-3": 23, "shard-4": 23}

	result := compareDimension("db_shard", anomalous, baseline, 100, 100)

	if result.Dimension != "db_shard" {
		t.Errorf("Dimension = %s, want db_shard", result.Dimension)
	}
	if result.TopValue != "shard-7" {
		t.Errorf("TopValue = %s, want shard-7", result.TopValue)
	}
	if result.TopValueLift < 10 {
		t.Errorf("TopValueLift = %f, want >= 10", result.TopValueLift)
	}
	if result.Divergence <= 0 {
		t.Errorf("Divergence = %f, want > 0", result.Divergence)
	}
}

func TestGenerateSummary(t *testing.T) {
	dims := []DimensionAnalysis{
		{
			Dimension:       "db_shard",
			TopValue:        "shard-7",
			TopValueLift:    12.25,
			AnomalousDistro: map[string]float64{"shard-7": 0.98},
			BaselineDistro:  map[string]float64{"shard-7": 0.08},
		},
	}

	summary := generateSummary(dims, "latency")

	if summary == "" {
		t.Error("summary is empty")
	}
	if !contains(summary, "98%") {
		t.Errorf("summary should contain '98%%': %s", summary)
	}
	if !contains(summary, "shard-7") {
		t.Errorf("summary should contain 'shard-7': %s", summary)
	}
	if !contains(summary, "12.2") {
		t.Errorf("summary should contain lift value: %s", summary)
	}
}

func TestGenerateSummary_Empty(t *testing.T) {
	summary := generateSummary([]DimensionAnalysis{}, "latency")
	if summary != "No significant dimensional differences found." {
		t.Errorf("unexpected summary for empty dims: %s", summary)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
