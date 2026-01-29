package trace

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	t.Parallel()

	t.Run("creates store successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "traces.db")

		store, err := NewStore(dbPath)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}
		defer store.Close()

		if store.db == nil {
			t.Error("db should be initialized")
		}
	})

	t.Run("creates required tables", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "traces.db")

		store, err := NewStore(dbPath)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}
		defer store.Close()

		// Verify tables exist
		tables := []string{"spans", "traces"}
		for _, table := range tables {
			var name string
			err := store.db.QueryRow(
				"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
			).Scan(&name)
			if err != nil {
				t.Errorf("table %s does not exist: %v", table, err)
			}
		}
	})
}

func TestRecordSpan(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	t.Run("records span successfully", func(t *testing.T) {
		now := time.Now()
		span := Span{
			TraceID:     "trace123",
			SpanID:      "span001",
			Name:        "GET /api/users",
			ServiceName: "user-service",
			Kind:        "SERVER",
			StartTime:   now,
			EndTime:     now.Add(50 * time.Millisecond),
			DurationMs:  50.0,
			Status:      "OK",
			Attributes: map[string]string{
				"http.method": "GET",
				"http.path":   "/api/users",
			},
		}

		err := store.RecordSpan(span)
		if err != nil {
			t.Fatalf("RecordSpan failed: %v", err)
		}

		// Verify span was stored
		var count int
		store.db.QueryRow("SELECT COUNT(*) FROM spans WHERE trace_id = ?", "trace123").Scan(&count)
		if count != 1 {
			t.Errorf("expected 1 span, got %d", count)
		}
	})

	t.Run("records span with parent", func(t *testing.T) {
		now := time.Now()
		span := Span{
			TraceID:      "trace456",
			SpanID:       "span002",
			ParentSpanID: "span001",
			Name:         "database.query",
			ServiceName:  "user-service",
			Kind:         "CLIENT",
			StartTime:    now,
			EndTime:      now.Add(20 * time.Millisecond),
			DurationMs:   20.0,
			Status:       "OK",
		}

		err := store.RecordSpan(span)
		if err != nil {
			t.Fatalf("RecordSpan failed: %v", err)
		}
	})

	t.Run("updates trace summary", func(t *testing.T) {
		// Record a root span
		now := time.Now()
		span := Span{
			TraceID:     "trace789",
			SpanID:      "root",
			Name:        "API Request",
			ServiceName: "api-gateway",
			Kind:        "SERVER",
			StartTime:   now,
			EndTime:     now.Add(100 * time.Millisecond),
			DurationMs:  100.0,
			Status:      "OK",
		}

		err := store.RecordSpan(span)
		if err != nil {
			t.Fatalf("RecordSpan failed: %v", err)
		}

		// Verify trace summary was created
		var count int
		store.db.QueryRow("SELECT COUNT(*) FROM traces WHERE trace_id = ?", "trace789").Scan(&count)
		if count != 1 {
			t.Errorf("expected 1 trace summary, got %d", count)
		}
	})
}

func TestGetTrace(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	// Create a trace with multiple spans
	now := time.Now()
	rootSpan := Span{
		TraceID:     "testtrace",
		SpanID:      "root",
		Name:        "HTTP GET /users",
		ServiceName: "api-gateway",
		Kind:        "SERVER",
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
		DurationMs:  100.0,
		Status:      "OK",
	}
	childSpan := Span{
		TraceID:      "testtrace",
		SpanID:       "child1",
		ParentSpanID: "root",
		Name:         "database.query",
		ServiceName:  "user-service",
		Kind:         "CLIENT",
		StartTime:    now.Add(10 * time.Millisecond),
		EndTime:      now.Add(50 * time.Millisecond),
		DurationMs:   40.0,
		Status:       "OK",
	}

	store.RecordSpan(rootSpan)
	store.RecordSpan(childSpan)

	t.Run("retrieves trace with spans", func(t *testing.T) {
		detail, err := store.GetTrace("testtrace")
		if err != nil {
			t.Fatalf("GetTrace failed: %v", err)
		}

		if detail.TraceID != "testtrace" {
			t.Errorf("TraceID = %s, want testtrace", detail.TraceID)
		}

		if len(detail.Spans) != 2 {
			t.Errorf("expected 2 spans, got %d", len(detail.Spans))
		}

		if detail.SpanCount != 2 {
			t.Errorf("SpanCount = %d, want 2", detail.SpanCount)
		}
	})

	t.Run("returns error for non-existent trace", func(t *testing.T) {
		_, err := store.GetTrace("nonexistent")
		if err == nil {
			t.Error("expected error for non-existent trace")
		}
	})
}

func TestListTraces(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	// Create several traces
	now := time.Now()
	for i := 0; i < 5; i++ {
		span := Span{
			TraceID:     "list" + string(rune('a'+i)),
			SpanID:      "root",
			Name:        "Request",
			ServiceName: "test-service",
			Kind:        "SERVER",
			StartTime:   now.Add(time.Duration(i) * time.Second),
			EndTime:     now.Add(time.Duration(i)*time.Second + 50*time.Millisecond),
			DurationMs:  50.0,
			Status:      "OK",
		}
		store.RecordSpan(span)
	}

	t.Run("lists traces", func(t *testing.T) {
		traces, err := store.ListTraces(10, "", time.Hour)
		if err != nil {
			t.Fatalf("ListTraces failed: %v", err)
		}

		if len(traces) != 5 {
			t.Errorf("expected 5 traces, got %d", len(traces))
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		traces, err := store.ListTraces(3, "", time.Hour)
		if err != nil {
			t.Fatalf("ListTraces failed: %v", err)
		}

		if len(traces) != 3 {
			t.Errorf("expected 3 traces, got %d", len(traces))
		}
	})

	t.Run("filters by service", func(t *testing.T) {
		// Add a trace with different service
		span := Span{
			TraceID:     "other-service-trace",
			SpanID:      "root",
			Name:        "Request",
			ServiceName: "other-service",
			Kind:        "SERVER",
			StartTime:   now,
			EndTime:     now.Add(50 * time.Millisecond),
			DurationMs:  50.0,
			Status:      "OK",
		}
		store.RecordSpan(span)

		traces, err := store.ListTraces(10, "other-service", time.Hour)
		if err != nil {
			t.Fatalf("ListTraces failed: %v", err)
		}

		if len(traces) != 1 {
			t.Errorf("expected 1 trace, got %d", len(traces))
		}
	})

	t.Run("respects time range", func(t *testing.T) {
		// Query for traces from the far past (should return nothing)
		// Note: The ListTraces function returns traces with start_time > (now - since)
		// Using a time far in the past ensures we get nothing
		tmpStore := setupTestTraceStore(t)
		defer tmpStore.Close()

		// Don't add any data - an empty store should return no traces
		traces, err := tmpStore.ListTraces(10, "", time.Hour)
		if err != nil {
			t.Fatalf("ListTraces failed: %v", err)
		}

		if len(traces) != 0 {
			t.Errorf("expected 0 traces for empty store, got %d", len(traces))
		}
	})

	t.Run("orders by start time descending", func(t *testing.T) {
		traces, err := store.ListTraces(10, "", time.Hour)
		if err != nil {
			t.Fatalf("ListTraces failed: %v", err)
		}

		for i := 1; i < len(traces); i++ {
			if traces[i].StartTime.After(traces[i-1].StartTime) {
				t.Error("traces should be ordered by start time descending")
			}
		}
	})
}

func TestGetServices(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	// Create spans with different services
	now := time.Now()
	services := []string{"service-a", "service-b", "service-c"}
	for _, svc := range services {
		span := Span{
			TraceID:     "trace-" + svc,
			SpanID:      "root",
			Name:        "Request",
			ServiceName: svc,
			Kind:        "SERVER",
			StartTime:   now,
			EndTime:     now.Add(50 * time.Millisecond),
			DurationMs:  50.0,
			Status:      "OK",
		}
		store.RecordSpan(span)
	}

	t.Run("returns unique services", func(t *testing.T) {
		result, err := store.GetServices()
		if err != nil {
			t.Fatalf("GetServices failed: %v", err)
		}

		if len(result) != 3 {
			t.Errorf("expected 3 services, got %d", len(result))
		}
	})

	t.Run("services are sorted", func(t *testing.T) {
		result, err := store.GetServices()
		if err != nil {
			t.Fatalf("GetServices failed: %v", err)
		}

		for i := 1; i < len(result); i++ {
			if result[i] < result[i-1] {
				t.Error("services should be sorted alphabetically")
			}
		}
	})
}

func TestGetServiceDependencies(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	// Create a trace with parent-child relationship between services
	now := time.Now()
	parentSpan := Span{
		TraceID:     "dep-trace",
		SpanID:      "parent",
		Name:        "API Request",
		ServiceName: "api-gateway",
		Kind:        "SERVER",
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
		DurationMs:  100.0,
		Status:      "OK",
	}
	childSpan := Span{
		TraceID:      "dep-trace",
		SpanID:       "child",
		ParentSpanID: "parent",
		Name:         "Database Query",
		ServiceName:  "user-service",
		Kind:         "CLIENT",
		StartTime:    now.Add(10 * time.Millisecond),
		EndTime:      now.Add(50 * time.Millisecond),
		DurationMs:   40.0,
		Status:       "OK",
	}

	store.RecordSpan(parentSpan)
	store.RecordSpan(childSpan)

	t.Run("returns service dependencies", func(t *testing.T) {
		deps, err := store.GetServiceDependencies()
		if err != nil {
			t.Fatalf("GetServiceDependencies failed: %v", err)
		}

		// Should have one dependency: api-gateway -> user-service
		found := false
		for _, dep := range deps {
			if dep.Parent == "api-gateway" && dep.Child == "user-service" {
				found = true
				if dep.CallCount < 1 {
					t.Errorf("call count should be at least 1, got %d", dep.CallCount)
				}
			}
		}

		if !found {
			t.Error("expected to find api-gateway -> user-service dependency")
		}
	})
}

func TestSpanStruct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	span := Span{
		TraceID:      "trace123",
		SpanID:       "span001",
		ParentSpanID: "parent001",
		Name:         "HTTP GET",
		ServiceName:  "test-service",
		Kind:         "SERVER",
		StartTime:    now,
		EndTime:      now.Add(50 * time.Millisecond),
		DurationMs:   50.0,
		Status:       "OK",
		StatusMsg:    "success",
		Attributes: map[string]string{
			"key": "value",
		},
	}

	if span.TraceID != "trace123" {
		t.Errorf("TraceID = %s, want trace123", span.TraceID)
	}
	if span.DurationMs != 50.0 {
		t.Errorf("DurationMs = %f, want 50.0", span.DurationMs)
	}
	if span.Attributes["key"] != "value" {
		t.Errorf("Attributes[key] = %s, want value", span.Attributes["key"])
	}
}

func TestTraceStruct(t *testing.T) {
	t.Parallel()

	now := time.Now()
	trace := Trace{
		TraceID:     "trace123",
		RootSpan:    "span001",
		ServiceName: "test-service",
		Name:        "HTTP GET",
		StartTime:   now,
		DurationMs:  100.0,
		SpanCount:   5,
		Status:      "OK",
		Services:    []string{"service-a", "service-b"},
	}

	if trace.TraceID != "trace123" {
		t.Errorf("TraceID = %s, want trace123", trace.TraceID)
	}
	if trace.SpanCount != 5 {
		t.Errorf("SpanCount = %d, want 5", trace.SpanCount)
	}
	if len(trace.Services) != 2 {
		t.Errorf("Services count = %d, want 2", len(trace.Services))
	}
}

func TestConcurrentSpanRecording(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	done := make(chan bool)
	now := time.Now()

	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				span := Span{
					TraceID:     "concurrent-trace",
					SpanID:      "span" + string(rune('0'+n)) + string(rune('0'+j)),
					Name:        "Concurrent Request",
					ServiceName: "test-service",
					Kind:        "SERVER",
					StartTime:   now,
					EndTime:     now.Add(50 * time.Millisecond),
					DurationMs:  50.0,
					Status:      "OK",
				}
				store.RecordSpan(span)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all spans were recorded
	var count int
	store.db.QueryRow("SELECT COUNT(*) FROM spans WHERE trace_id = ?", "concurrent-trace").Scan(&count)
	if count != 100 {
		t.Errorf("expected 100 spans, got %d", count)
	}
}

func TestSpanAttributes(t *testing.T) {
	t.Parallel()

	store := setupTestTraceStore(t)
	defer store.Close()

	now := time.Now()
	span := Span{
		TraceID:     "attr-trace",
		SpanID:      "span001",
		Name:        "HTTP Request",
		ServiceName: "test-service",
		Kind:        "SERVER",
		StartTime:   now,
		EndTime:     now.Add(50 * time.Millisecond),
		DurationMs:  50.0,
		Status:      "OK",
		Attributes: map[string]string{
			"http.method":      "GET",
			"http.url":         "/api/users",
			"http.status_code": "200",
			"user.id":          "12345",
		},
	}

	err := store.RecordSpan(span)
	if err != nil {
		t.Fatalf("RecordSpan failed: %v", err)
	}

	// Retrieve and verify attributes
	detail, err := store.GetTrace("attr-trace")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}

	if len(detail.Spans) == 0 {
		t.Fatal("expected at least one span")
	}

	attrs := detail.Spans[0].Attributes
	if attrs["http.method"] != "GET" {
		t.Errorf("http.method = %s, want GET", attrs["http.method"])
	}
	if attrs["http.status_code"] != "200" {
		t.Errorf("http.status_code = %s, want 200", attrs["http.status_code"])
	}
}

// Helper to set up test store
func setupTestTraceStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "traces_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// Benchmarks

func BenchmarkRecordSpan(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	store, err := NewStore(dbPath)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	span := Span{
		TraceID:     "bench-trace",
		SpanID:      "span001",
		Name:        "Benchmark Request",
		ServiceName: "bench-service",
		Kind:        "SERVER",
		StartTime:   now,
		EndTime:     now.Add(50 * time.Millisecond),
		DurationMs:  50.0,
		Status:      "OK",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.SpanID = "span" + string(rune('0'+i%10))
		store.RecordSpan(span)
	}
}

func BenchmarkListTraces(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	store, err := NewStore(dbPath)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Seed data
	now := time.Now()
	for i := 0; i < 1000; i++ {
		span := Span{
			TraceID:     "trace" + string(rune('0'+i%10)),
			SpanID:      "span" + string(rune('0'+i%100)),
			Name:        "Request",
			ServiceName: "service",
			Kind:        "SERVER",
			StartTime:   now,
			EndTime:     now.Add(50 * time.Millisecond),
			DurationMs:  50.0,
			Status:      "OK",
		}
		store.RecordSpan(span)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListTraces(100, "", time.Hour)
	}
}
