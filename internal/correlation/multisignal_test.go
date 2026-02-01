package correlation

import (
	"context"
	"testing"
	"time"
)

func TestMultiSignalCorrelator_Exemplars(t *testing.T) {
	config := DefaultMultiSignalConfig()
	config.MaxExemplarsPerMetric = 5
	correlator := NewMultiSignalCorrelator(config)

	metricKey := "http_request_duration|service=api"
	now := time.Now()

	// Record exemplars
	for i := 0; i < 10; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   "trace-" + itoa(i),
			SpanID:    "span-" + itoa(i),
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Value:     float64(i * 100),
		})
	}

	// Should only keep last 5
	exemplars := correlator.GetExemplars(metricKey)
	if len(exemplars) != 5 {
		t.Errorf("Expected 5 exemplars, got %d", len(exemplars))
	}

	// First exemplar should be trace-5 (oldest kept)
	if exemplars[0].TraceID != "trace-5" {
		t.Errorf("Expected trace-5, got %s", exemplars[0].TraceID)
	}
}

func TestMultiSignalCorrelator_ExemplarsInRange(t *testing.T) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())

	metricKey := "request_count"
	baseTime := time.Now()

	// Record exemplars at different times
	for i := 0; i < 5; i++ {
		correlator.RecordExemplar(metricKey, Exemplar{
			TraceID:   "trace-" + itoa(i),
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Value:     float64(i),
		})
	}

	// Query range that should include exemplars 1-3
	start := baseTime.Add(30 * time.Second)
	end := baseTime.Add(3*time.Minute + 30*time.Second)

	exemplars := correlator.GetExemplarsInRange(metricKey, start, end)
	if len(exemplars) != 3 {
		t.Errorf("Expected 3 exemplars in range, got %d", len(exemplars))
	}
}

func TestMultiSignalCorrelator_GetCrossSignalTimeline(t *testing.T) {
	correlator := NewMultiSignalCorrelator(DefaultMultiSignalConfig())

	// Without stores, should still return empty timeline
	ctx := context.Background()
	now := time.Now()
	start := now.Add(-1 * time.Hour)
	end := now

	timeline, err := correlator.GetCrossSignalTimeline(ctx, "test-service", start, end)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if timeline.Service != "test-service" {
		t.Errorf("Expected service test-service, got %s", timeline.Service)
	}

	// Events can be empty but should be accessible
	if len(timeline.Events) != 0 {
		t.Errorf("Expected 0 events without stores, got %d", len(timeline.Events))
	}

	if timeline.Summary == nil {
		t.Error("Summary should be initialized")
	}
}

func TestBuildMetricKey(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "http_requests",
			labels: nil,
			want:   "http_requests",
		},
		{
			name:   "http_requests",
			labels: map[string]string{},
			want:   "http_requests",
		},
		{
			name:   "http_requests",
			labels: map[string]string{"method": "GET"},
			want:   "http_requests|method=GET",
		},
		{
			name:   "http_requests",
			labels: map[string]string{"method": "GET", "path": "/api"},
			want:   "http_requests|method=GET|path=/api",
		},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := buildMetricKey(tt.name, tt.labels)
			if got != tt.want {
				t.Errorf("buildMetricKey(%s, %v) = %s, want %s", tt.name, tt.labels, got, tt.want)
			}
		})
	}
}

func TestFuzzyMatchReason(t *testing.T) {
	now := time.Now()

	tests := []struct {
		logTime      time.Time
		traceTime    time.Time
		logService   string
		traceService string
		wantContains string
	}{
		{
			logTime:      now,
			traceTime:    now.Add(500 * time.Millisecond),
			logService:   "api",
			traceService: "api",
			wantContains: "time_exact",
		},
		{
			logTime:      now,
			traceTime:    now.Add(3 * time.Second),
			logService:   "api",
			traceService: "api",
			wantContains: "time_close",
		},
		{
			logTime:      now,
			traceTime:    now.Add(10 * time.Second),
			logService:   "",
			traceService: "api",
			wantContains: "time_window",
		},
	}

	for i, tt := range tests {
		reason := fuzzyMatchReason(tt.logTime, tt.traceTime, tt.logService, tt.traceService)
		if !containsSimple(reason, tt.wantContains) {
			t.Errorf("Test %d: expected reason to contain %s, got %s", i, tt.wantContains, reason)
		}
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s      string
		substr []string
		want   bool
	}{
		{"http_latency", []string{"latency", "duration"}, true},
		{"request_error_count", []string{"error", "failure"}, true},
		{"cpu_usage", []string{"latency", "error"}, false},
		{"", []string{"a"}, false},
	}

	for _, tt := range tests {
		got := containsAny(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsAny(%s, %v) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestAbsDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want time.Duration
	}{
		{5 * time.Second, 5 * time.Second},
		{-5 * time.Second, 5 * time.Second},
		{0, 0},
	}

	for _, tt := range tests {
		got := absDuration(tt.d)
		if got != tt.want {
			t.Errorf("absDuration(%v) = %v, want %v", tt.d, got, tt.want)
		}
	}
}

func TestMatchesAttributes(t *testing.T) {
	tests := []struct {
		attrs map[string]string
		query map[string]string
		want  bool
	}{
		{
			attrs: map[string]string{"env": "prod", "region": "us-east"},
			query: map[string]string{"env": "prod"},
			want:  true,
		},
		{
			attrs: map[string]string{"env": "prod"},
			query: map[string]string{"env": "staging"},
			want:  false,
		},
		{
			attrs: map[string]string{"env": "prod"},
			query: map[string]string{},
			want:  true,
		},
		{
			attrs: nil,
			query: nil,
			want:  true,
		},
	}

	for i, tt := range tests {
		got := matchesAttributes(tt.attrs, tt.query)
		if got != tt.want {
			t.Errorf("Test %d: matchesAttributes = %v, want %v", i, got, tt.want)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncateString(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}

	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		b[pos] = '-'
	}

	return string(b[pos:])
}
