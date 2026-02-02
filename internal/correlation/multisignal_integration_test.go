package correlation

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Integration tests for MultiSignalCorrelator

func TestMultiSignalCorrelator_ExemplarStorageAndRetrieval(t *testing.T) {
	config := DefaultMultiSignalConfig()
	config.MaxExemplarsPerMetric = 10
	correlator := NewMultiSignalCorrelator(config)

	metricKey := "http_request_duration|service=api|method=GET"
	baseTime := time.Now()

	// Store exemplars with different trace IDs
	for i := 0; i < 15; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   fmt.Sprintf("trace-%03d", i),
			SpanID:    fmt.Sprintf("span-%03d", i),
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Value:     float64(100 + i*10),
			Labels:    map[string]string{"path": "/api/users"},
		})
	}

	// Verify only last 10 are kept
	exemplars := correlator.GetExemplars(metricKey)
	if len(exemplars) != 10 {
		t.Errorf("Expected 10 exemplars after overflow, got %d", len(exemplars))
	}

	// First exemplar should be trace-005 (earliest of kept)
	if exemplars[0].TraceID != "trace-005" {
		t.Errorf("Expected oldest kept exemplar to be trace-005, got %s", exemplars[0].TraceID)
	}

	// Last exemplar should be trace-014 (most recent)
	if exemplars[9].TraceID != "trace-014" {
		t.Errorf("Expected newest exemplar to be trace-014, got %s", exemplars[9].TraceID)
	}

	// Verify exemplar values are preserved
	for i, ex := range exemplars {
		expectedValue := float64(100 + (i+5)*10)
		if ex.Value != expectedValue {
			t.Errorf("Exemplar %d: expected value %.0f, got %.0f", i, expectedValue, ex.Value)
		}
	}
}

func TestMultiSignalCorrelator_ExemplarsInRangeQuery(t *testing.T) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())

	metricKey := "request_latency_p99"
	baseTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Record exemplars at specific times
	times := []struct {
		offset  time.Duration
		traceID string
	}{
		{0 * time.Minute, "trace-00"},
		{5 * time.Minute, "trace-05"},
		{10 * time.Minute, "trace-10"},
		{15 * time.Minute, "trace-15"},
		{20 * time.Minute, "trace-20"},
		{25 * time.Minute, "trace-25"},
	}

	for _, tt := range times {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   tt.traceID,
			Timestamp: baseTime.Add(tt.offset),
			Value:     100.0,
		})
	}

	tests := []struct {
		name          string
		start         time.Time
		end           time.Time
		expectedCount int
		expectedFirst string
		expectedLast  string
	}{
		{
			name:          "full range",
			start:         baseTime,
			end:           baseTime.Add(30 * time.Minute),
			expectedCount: 6,
			expectedFirst: "trace-00",
			expectedLast:  "trace-25",
		},
		{
			name:          "middle range",
			start:         baseTime.Add(7 * time.Minute),
			end:           baseTime.Add(18 * time.Minute),
			expectedCount: 2,
			expectedFirst: "trace-10",
			expectedLast:  "trace-15",
		},
		{
			name:          "exact boundary",
			start:         baseTime.Add(10 * time.Minute),
			end:           baseTime.Add(20 * time.Minute),
			expectedCount: 3,
			expectedFirst: "trace-10",
			expectedLast:  "trace-20",
		},
		{
			name:          "empty range",
			start:         baseTime.Add(6 * time.Minute),
			end:           baseTime.Add(8 * time.Minute),
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exemplars := correlator.GetExemplarsInRange(metricKey, tt.start, tt.end)

			if len(exemplars) != tt.expectedCount {
				t.Errorf("Expected %d exemplars, got %d", tt.expectedCount, len(exemplars))
			}

			if tt.expectedCount > 0 {
				if exemplars[0].TraceID != tt.expectedFirst {
					t.Errorf("Expected first exemplar %s, got %s", tt.expectedFirst, exemplars[0].TraceID)
				}
				if exemplars[len(exemplars)-1].TraceID != tt.expectedLast {
					t.Errorf("Expected last exemplar %s, got %s", tt.expectedLast, exemplars[len(exemplars)-1].TraceID)
				}
			}
		})
	}
}

func TestMultiSignalCorrelator_FuzzyMatchingAccuracy(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		logTime         time.Time
		traceTime       time.Time
		logService      string
		traceService    string
		expectedReason  string
		containsService bool
	}{
		{
			name:            "exact time match",
			logTime:         now,
			traceTime:       now.Add(100 * time.Millisecond),
			logService:      "api-gateway",
			traceService:    "api-gateway",
			expectedReason:  "time_exact",
			containsService: true,
		},
		{
			name:            "close time match",
			logTime:         now,
			traceTime:       now.Add(2 * time.Second),
			logService:      "user-service",
			traceService:    "user-service",
			expectedReason:  "time_close",
			containsService: true,
		},
		{
			name:            "window time match no service",
			logTime:         now,
			traceTime:       now.Add(6 * time.Second), // 6s is outside 5s "close" threshold
			logService:      "",
			traceService:    "order-service",
			expectedReason:  "time_window",
			containsService: false,
		},
		{
			name:            "different services",
			logTime:         now,
			traceTime:       now.Add(500 * time.Millisecond),
			logService:      "service-a",
			traceService:    "service-b",
			expectedReason:  "time_exact",
			containsService: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := fuzzyMatchReason(tt.logTime, tt.traceTime, tt.logService, tt.traceService)

			if !containsSimple(reason, tt.expectedReason) {
				t.Errorf("Expected reason to contain %q, got %q", tt.expectedReason, reason)
			}

			hasService := containsSimple(reason, "service_match")
			if hasService != tt.containsService {
				t.Errorf("Expected service_match=%v, got %v in reason %q",
					tt.containsService, hasService, reason)
			}
		})
	}
}

func TestMultiSignalCorrelator_CrossSignalTimelineBuilding(t *testing.T) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())

	ctx := context.Background()
	now := time.Now()
	start := now.Add(-1 * time.Hour)
	end := now

	// Without stores, should return empty but valid timeline
	timeline, err := correlator.GetCrossSignalTimeline(ctx, "test-service", start, end)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify structure
	if timeline == nil {
		t.Fatal("Timeline should not be nil")
	}

	if timeline.Service != "test-service" {
		t.Errorf("Expected service test-service, got %s", timeline.Service)
	}

	if timeline.StartTime != start {
		t.Errorf("Expected start time %v, got %v", start, timeline.StartTime)
	}

	if timeline.EndTime != end {
		t.Errorf("Expected end time %v, got %v", end, timeline.EndTime)
	}

	if timeline.Events == nil {
		t.Log("Note: Events is nil, not empty slice - implementation detail")
	}

	if timeline.Summary == nil {
		t.Error("Summary should be initialized")
	}

	// Summary stats should be zero
	if timeline.Summary.TotalEvents != 0 {
		t.Errorf("Expected 0 total events, got %d", timeline.Summary.TotalEvents)
	}
}

func TestMultiSignalCorrelator_BuildMetricKey(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			metric:   "http_requests_total",
			labels:   nil,
			expected: "http_requests_total",
		},
		{
			name:     "empty labels",
			metric:   "http_requests_total",
			labels:   map[string]string{},
			expected: "http_requests_total",
		},
		{
			name:     "single label",
			metric:   "http_requests_total",
			labels:   map[string]string{"method": "GET"},
			expected: "http_requests_total|method=GET",
		},
		{
			name:   "multiple labels sorted",
			metric: "http_requests_total",
			labels: map[string]string{
				"method":  "POST",
				"service": "api",
				"code":    "200",
			},
			expected: "http_requests_total|code=200|method=POST|service=api",
		},
		{
			name:     "label with special chars",
			metric:   "custom_metric",
			labels:   map[string]string{"path": "/api/v1/users"},
			expected: "custom_metric|path=/api/v1/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMetricKey(tt.metric, tt.labels)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestMultiSignalCorrelator_AttributeMatching(t *testing.T) {
	tests := []struct {
		name   string
		attrs  map[string]string
		query  map[string]string
		expect bool
	}{
		{
			name:   "exact match",
			attrs:  map[string]string{"env": "prod", "region": "us-east"},
			query:  map[string]string{"env": "prod", "region": "us-east"},
			expect: true,
		},
		{
			name:   "partial match",
			attrs:  map[string]string{"env": "prod", "region": "us-east", "app": "api"},
			query:  map[string]string{"env": "prod"},
			expect: true,
		},
		{
			name:   "no match",
			attrs:  map[string]string{"env": "prod"},
			query:  map[string]string{"env": "staging"},
			expect: false,
		},
		{
			name:   "missing key",
			attrs:  map[string]string{"env": "prod"},
			query:  map[string]string{"region": "us-east"},
			expect: false,
		},
		{
			name:   "empty query matches all",
			attrs:  map[string]string{"env": "prod"},
			query:  map[string]string{},
			expect: true,
		},
		{
			name:   "nil attrs with query",
			attrs:  nil,
			query:  map[string]string{"key": "value"},
			expect: false,
		},
		{
			name:   "both nil",
			attrs:  nil,
			query:  nil,
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesAttributes(tt.attrs, tt.query)
			if result != tt.expect {
				t.Errorf("Expected %v, got %v", tt.expect, result)
			}
		})
	}
}

func TestMultiSignalCorrelator_StringHelpers(t *testing.T) {
	// Test truncateString
	truncateTests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello world", 20, "hello world"},
		{"hello world", 8, "hello..."},
		{"hello world", 5, "he..."},
		{"", 10, ""},
		{"ab", 5, "ab"},
		{"abc", 3, "abc"},
	}

	for _, tt := range truncateTests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d): expected %q, got %q",
				tt.input, tt.maxLen, tt.expected, result)
		}
	}

	// Test containsAny
	containsTests := []struct {
		s       string
		substrs []string
		expect  bool
	}{
		{"http_request_latency", []string{"latency", "duration"}, true},
		{"error_count_total", []string{"error", "failure"}, true},
		{"cpu_usage_percent", []string{"memory", "disk"}, false},
		{"", []string{"test"}, false},
		{"test", []string{}, false},
		{"HTTP_LATENCY", []string{"latency"}, true}, // case insensitive
	}

	for _, tt := range containsTests {
		result := containsAny(tt.s, tt.substrs)
		if result != tt.expect {
			t.Errorf("containsAny(%q, %v): expected %v, got %v",
				tt.s, tt.substrs, tt.expect, result)
		}
	}

	// Test absDuration
	durationTests := []struct {
		input  time.Duration
		expect time.Duration
	}{
		{5 * time.Second, 5 * time.Second},
		{-5 * time.Second, 5 * time.Second},
		{0, 0},
		{-1 * time.Millisecond, 1 * time.Millisecond},
	}

	for _, tt := range durationTests {
		result := absDuration(tt.input)
		if result != tt.expect {
			t.Errorf("absDuration(%v): expected %v, got %v", tt.input, tt.expect, result)
		}
	}
}

func TestMultiSignalCorrelator_ConcurrentExemplarAccess(t *testing.T) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())

	metricKey := "concurrent_test_metric"
	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 5
	operationsPerGoroutine := 100

	// Writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				correlator.RecordExemplar(metricKey, Exemplar{
					TraceID:   fmt.Sprintf("trace-%d-%d", writerID, j),
					Timestamp: time.Now(),
					Value:     float64(j),
				})
			}
		}(i)
	}

	// Readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = correlator.GetExemplars(metricKey)
			}
		}()
	}

	wg.Wait()

	// Verify final state is valid
	exemplars := correlator.GetExemplars(metricKey)
	if len(exemplars) == 0 {
		t.Error("Expected some exemplars after concurrent writes")
	}

	// Verify no nil or corrupt entries
	for i, ex := range exemplars {
		if ex.TraceID == "" {
			t.Errorf("Exemplar %d has empty TraceID", i)
		}
		if ex.Timestamp.IsZero() {
			t.Errorf("Exemplar %d has zero timestamp", i)
		}
	}
}

// Benchmark tests

func BenchmarkMultiSignalCorrelator_RecordExemplar(b *testing.B) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())
	metricKey := "bench_metric|service=api"
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   fmt.Sprintf("trace-%d", i),
			Timestamp: now,
			Value:     float64(i),
		})
	}
}

func BenchmarkMultiSignalCorrelator_GetExemplars(b *testing.B) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())
	metricKey := "bench_metric"

	// Pre-populate
	for i := 0; i < 100; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   fmt.Sprintf("trace-%d", i),
			Timestamp: time.Now(),
			Value:     float64(i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = correlator.GetExemplars(metricKey)
	}
}

func BenchmarkMultiSignalCorrelator_GetExemplarsInRange(b *testing.B) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())
	metricKey := "bench_metric"
	baseTime := time.Now()

	// Pre-populate
	for i := 0; i < 100; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   fmt.Sprintf("trace-%d", i),
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Value:     float64(i),
		})
	}

	start := baseTime.Add(20 * time.Second)
	end := baseTime.Add(60 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = correlator.GetExemplarsInRange(metricKey, start, end)
	}
}

func BenchmarkMultiSignalCorrelator_BuildMetricKey(b *testing.B) {
	labels := map[string]string{
		"service": "api-gateway",
		"method":  "POST",
		"path":    "/api/v1/users",
		"status":  "200",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildMetricKey("http_request_duration_seconds", labels)
	}
}

func BenchmarkMultiSignalCorrelator_FuzzyMatchReason(b *testing.B) {
	now := time.Now()
	logTime := now
	traceTime := now.Add(2 * time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fuzzyMatchReason(logTime, traceTime, "api-gateway", "api-gateway")
	}
}

func BenchmarkMultiSignalCorrelator_ConcurrentAccess(b *testing.B) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())
	metricKey := "concurrent_bench"

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				correlator.RecordExemplar(metricKey, Exemplar{
					TraceID:   fmt.Sprintf("trace-%d", i),
					Timestamp: time.Now(),
					Value:     float64(i),
				})
			} else {
				_ = correlator.GetExemplars(metricKey)
			}
			i++
		}
	})
}
