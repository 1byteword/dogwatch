package profile

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestStoreCreateAndQuery(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "profile_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Test RecordSample
	now := time.Now()
	sample := &Sample{
		Timestamp:   now,
		PID:         1234,
		TGID:        1234,
		Comm:        "testapp",
		Count:       5,
		KernelStack: []string{"do_syscall_64", "entry_SYSCALL_64"},
		UserStack:   []string{"main.handleRequest", "net/http.ServeHTTP"},
	}

	if err := store.RecordSample(sample); err != nil {
		t.Fatalf("RecordSample failed: %v", err)
	}

	if sample.ID == 0 {
		t.Error("Expected sample ID to be set")
	}

	// Test QueryByTimeRange
	samples, err := store.QueryByTimeRange(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryByTimeRange failed: %v", err)
	}

	if len(samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(samples))
	}

	if samples[0].Comm != "testapp" {
		t.Errorf("Expected comm 'testapp', got '%s'", samples[0].Comm)
	}

	// Test QueryByPIDAndTime
	samples, err = store.QueryByPIDAndTime(1234, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryByPIDAndTime failed: %v", err)
	}

	if len(samples) != 1 {
		t.Errorf("Expected 1 sample, got %d", len(samples))
	}

	// Test QueryByFunction
	samples, err = store.QueryByFunction("handleRequest", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryByFunction failed: %v", err)
	}

	if len(samples) != 1 {
		t.Errorf("Expected 1 sample for function search, got %d", len(samples))
	}

	// Test GetSampleByID
	retrieved, err := store.GetSampleByID(sample.ID)
	if err != nil {
		t.Fatalf("GetSampleByID failed: %v", err)
	}

	if retrieved.PID != 1234 {
		t.Errorf("Expected PID 1234, got %d", retrieved.PID)
	}

	if len(retrieved.UserStack) != 2 {
		t.Errorf("Expected 2 user stack frames, got %d", len(retrieved.UserStack))
	}
}

func TestRecordSamplesBatch(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "profile_batch_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	now := time.Now()
	samples := []*Sample{
		{
			Timestamp: now,
			PID:       1001,
			TGID:      1001,
			Comm:      "app1",
			Count:     10,
			UserStack: []string{"func1"},
		},
		{
			Timestamp: now.Add(time.Millisecond),
			PID:       1002,
			TGID:      1002,
			Comm:      "app2",
			Count:     20,
			UserStack: []string{"func2"},
		},
		{
			Timestamp: now.Add(2 * time.Millisecond),
			PID:       1003,
			TGID:      1003,
			Comm:      "app3",
			Count:     30,
			UserStack: []string{"func3"},
		},
	}

	if err := store.RecordSamples(samples); err != nil {
		t.Fatalf("RecordSamples failed: %v", err)
	}

	// Verify all samples got IDs
	for i, s := range samples {
		if s.ID == 0 {
			t.Errorf("Sample %d did not get ID", i)
		}
	}

	// Query and verify
	retrieved, err := store.QueryByTimeRange(now.Add(-time.Second), now.Add(time.Second))
	if err != nil {
		t.Fatalf("QueryByTimeRange failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 samples, got %d", len(retrieved))
	}
}

func TestProfileTraceLinks(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "profile_links_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a sample
	now := time.Now()
	sample := &Sample{
		Timestamp: now,
		PID:       5000,
		TGID:      5000,
		Comm:      "webserver",
		Count:     100,
		UserStack: []string{"handleHTTP"},
	}
	store.RecordSample(sample)

	// Create links
	link1 := &ProfileTraceLink{
		TraceID:      "abc123",
		SpanID:       "span1",
		SampleID:     sample.ID,
		FunctionName: "handleHTTP",
		Confidence:   0.9,
	}
	if err := store.RecordLink(link1); err != nil {
		t.Fatalf("RecordLink failed: %v", err)
	}

	link2 := &ProfileTraceLink{
		TraceID:      "abc123",
		SpanID:       "span2",
		SampleID:     sample.ID,
		FunctionName: "handleHTTP",
		Confidence:   0.7,
	}
	store.RecordLink(link2)

	// Query links by trace
	links, err := store.GetLinksForTrace("abc123")
	if err != nil {
		t.Fatalf("GetLinksForTrace failed: %v", err)
	}

	if len(links) != 2 {
		t.Errorf("Expected 2 links, got %d", len(links))
	}

	// Links should be sorted by confidence descending
	if links[0].Confidence < links[1].Confidence {
		t.Error("Links should be sorted by confidence descending")
	}

	// Query links by span
	spanLinks, err := store.GetLinksForSpan("abc123", "span1")
	if err != nil {
		t.Fatalf("GetLinksForSpan failed: %v", err)
	}

	if len(spanLinks) != 1 {
		t.Errorf("Expected 1 link for span, got %d", len(spanLinks))
	}
}

func TestHotspots(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "profile_hotspots_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	now := time.Now()

	// Create samples with different functions and counts
	samples := []*Sample{
		{
			Timestamp: now,
			PID:       1000,
			Comm:      "app",
			Count:     100,
			UserStack: []string{"hotFunction", "middleFunction", "baseFunction"},
		},
		{
			Timestamp: now.Add(time.Millisecond),
			PID:       1000,
			Comm:      "app",
			Count:     50,
			UserStack: []string{"hotFunction", "baseFunction"},
		},
		{
			Timestamp: now.Add(2 * time.Millisecond),
			PID:       1000,
			Comm:      "app",
			Count:     30,
			UserStack: []string{"middleFunction", "baseFunction"},
		},
	}

	store.RecordSamples(samples)

	hotspots, err := store.GetHotspots(now.Add(-time.Second), now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("GetHotspots failed: %v", err)
	}

	if len(hotspots) == 0 {
		t.Fatal("Expected hotspots, got none")
	}

	// Verify hotspots are sorted by count
	for i := 1; i < len(hotspots); i++ {
		if hotspots[i].SampleCount > hotspots[i-1].SampleCount {
			t.Error("Hotspots should be sorted by sample count descending")
		}
	}

	// hotFunction should appear with highest count (100 + 50 = 150)
	found := false
	for _, h := range hotspots {
		if h.Function == "hotFunction" && h.SampleCount == 150 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected hotFunction with count 150")
	}
}

func TestLinkOptions(t *testing.T) {
	opts := DefaultLinkOptions()

	if opts.TimeWindow != 100*time.Millisecond {
		t.Errorf("Expected default TimeWindow 100ms, got %v", opts.TimeWindow)
	}

	if opts.MinConfidence != 0.3 {
		t.Errorf("Expected default MinConfidence 0.3, got %v", opts.MinConfidence)
	}

	if opts.MaxLinks != 10 {
		t.Errorf("Expected default MaxLinks 10, got %d", opts.MaxLinks)
	}
}

func TestSortLinksByConfidence(t *testing.T) {
	links := []SpanLink{
		{TraceID: "a", Confidence: 0.5},
		{TraceID: "b", Confidence: 0.9},
		{TraceID: "c", Confidence: 0.3},
		{TraceID: "d", Confidence: 0.7},
	}

	sortLinksByConfidence(links)

	expected := []float64{0.9, 0.7, 0.5, 0.3}
	for i, link := range links {
		if link.Confidence != expected[i] {
			t.Errorf("Position %d: expected confidence %v, got %v", i, expected[i], link.Confidence)
		}
	}
}

func TestLinkerCalculateTimeOverlap(t *testing.T) {
	linker := &Linker{}

	// Sample inside span
	sampleTime := time.Now()
	spanStart := sampleTime.Add(-50 * time.Millisecond)
	spanDurationMs := float64(100) // 100 milliseconds
	window := 100 * time.Millisecond

	score := linker.calculateTimeOverlap(sampleTime, spanStart, spanDurationMs, window)
	if score != 1.0 {
		t.Errorf("Sample inside span should have score 1.0, got %v", score)
	}

	// Sample just outside span
	sampleTime2 := spanStart.Add(-10 * time.Millisecond)
	score2 := linker.calculateTimeOverlap(sampleTime2, spanStart, spanDurationMs, window)
	if score2 <= 0 || score2 >= 1.0 {
		t.Errorf("Sample just outside span should have score between 0 and 1, got %v", score2)
	}

	// Sample far outside span
	sampleTime3 := spanStart.Add(-200 * time.Millisecond)
	score3 := linker.calculateTimeOverlap(sampleTime3, spanStart, spanDurationMs, window)
	if score3 != 0.0 {
		t.Errorf("Sample far outside span should have score 0, got %v", score3)
	}
}

func TestMatchesHTTPPattern(t *testing.T) {
	tests := []struct {
		fn        string
		operation string
		expected  bool
	}{
		{"net/http.ServeHTTP", "GET /api/users", true},
		{"handleRequest", "POST /api/data", true},
		{"sqlExec", "GET /api/users", false},
		{"main.httpHandler", "HTTP request", true},
	}

	for _, tt := range tests {
		result := matchesHTTPPattern(tt.fn, tt.operation)
		if result != tt.expected {
			t.Errorf("matchesHTTPPattern(%q, %q) = %v, want %v",
				tt.fn, tt.operation, result, tt.expected)
		}
	}
}

func TestMatchesDBPattern(t *testing.T) {
	tests := []struct {
		fn        string
		operation string
		expected  bool
	}{
		{"database/sql.Query", "mysql SELECT", true},
		{"redis.Get", "redis GET", true},
		{"main.handleHTTP", "HTTP request", false},
		{"postgres.Exec", "postgres INSERT", true},
	}

	for _, tt := range tests {
		result := matchesDBPattern(tt.fn, tt.operation)
		if result != tt.expected {
			t.Errorf("matchesDBPattern(%q, %q) = %v, want %v",
				tt.fn, tt.operation, result, tt.expected)
		}
	}
}

func TestLinkerGetStats(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "profile_stats_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create some test data
	now := time.Now()
	sample := &Sample{
		Timestamp: now,
		PID:       1234,
		Comm:      "test",
		Count:     10,
	}
	store.RecordSample(sample)

	link := &ProfileTraceLink{
		TraceID:    "trace1",
		SpanID:     "span1",
		SampleID:   sample.ID,
		Confidence: 0.8,
	}
	store.RecordLink(link)

	linker := &Linker{profileStore: store}
	stats, err := linker.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalSamples != 1 {
		t.Errorf("Expected 1 sample, got %d", stats.TotalSamples)
	}

	if stats.TotalLinks != 1 {
		t.Errorf("Expected 1 link, got %d", stats.TotalLinks)
	}

	if stats.HighConfidenceLinks != 1 {
		t.Errorf("Expected 1 high confidence link, got %d", stats.HighConfidenceLinks)
	}
}
