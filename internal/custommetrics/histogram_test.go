package custommetrics

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHistogramQuantile(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   HistogramSnapshot
		quantile   float64
		wantApprox float64
		tolerance  float64
	}{
		{
			name: "p50 uniform distribution",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{10, 20, 30, 40, 50},
				Counts:     []uint64{10, 20, 30, 40, 50},
				TotalCount: 50,
			},
			quantile:   0.50,
			wantApprox: 25,
			tolerance:  5,
		},
		{
			name: "p90 uniform distribution",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{10, 20, 30, 40, 50},
				Counts:     []uint64{10, 20, 30, 40, 50},
				TotalCount: 50,
			},
			quantile:   0.90,
			wantApprox: 45,
			tolerance:  5,
		},
		{
			name: "p99 near max",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{0.1, 0.5, 1.0, 2.0, 5.0},
				Counts:     []uint64{50, 80, 95, 99, 100},
				TotalCount: 100,
				Max:        5.0,
			},
			quantile:   0.99,
			wantApprox: 2.0,
			tolerance:  1.5,
		},
		{
			name: "p0 returns first bucket",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{10, 20, 30},
				Counts:     []uint64{10, 20, 30},
				TotalCount: 30,
			},
			quantile:   0.0,
			wantApprox: 0,
			tolerance:  1,
		},
		{
			name: "p100 returns max",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{10, 20, 30},
				Counts:     []uint64{10, 20, 30},
				TotalCount: 30,
				Max:        30,
			},
			quantile:   1.0,
			wantApprox: 30,
			tolerance:  5,
		},
		{
			name: "empty histogram returns 0",
			snapshot: HistogramSnapshot{
				Bounds:     []float64{},
				Counts:     []uint64{},
				TotalCount: 0,
			},
			quantile:   0.99,
			wantApprox: 0,
			tolerance:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.snapshot.Quantile(tt.quantile)
			diff := math.Abs(got - tt.wantApprox)
			if diff > tt.tolerance {
				t.Errorf("Quantile(%f) = %f, want approximately %f (tolerance %f, got diff %f)",
					tt.quantile, got, tt.wantApprox, tt.tolerance, diff)
			}
		})
	}
}

func TestHistogramMerge(t *testing.T) {
	s1 := &HistogramSnapshot{
		Bounds:     []float64{10, 20, 30},
		Counts:     []uint64{5, 10, 15},
		TotalCount: 15,
		Sum:        150,
		Min:        1,
		Max:        25,
	}

	s2 := &HistogramSnapshot{
		Bounds:     []float64{10, 20, 30},
		Counts:     []uint64{3, 8, 12},
		TotalCount: 12,
		Sum:        100,
		Min:        2,
		Max:        28,
	}

	s1.Merge(s2)

	if s1.TotalCount != 27 {
		t.Errorf("TotalCount = %d, want 27", s1.TotalCount)
	}
	if s1.Sum != 250 {
		t.Errorf("Sum = %f, want 250", s1.Sum)
	}
	if s1.Min != 1 {
		t.Errorf("Min = %f, want 1", s1.Min)
	}
	if s1.Max != 28 {
		t.Errorf("Max = %f, want 28", s1.Max)
	}
	if s1.Counts[0] != 8 {
		t.Errorf("Counts[0] = %d, want 8", s1.Counts[0])
	}
}

func TestHistogramDataPointToSnapshot(t *testing.T) {
	minVal := 1.0
	maxVal := 100.0

	hdp := HistogramDataPoint{
		Timestamp:      time.Now(),
		Name:           "test_histogram",
		Count:          1000,
		Sum:            50000,
		Min:            &minVal,
		Max:            &maxVal,
		ExplicitBounds: []float64{10, 25, 50, 75, 100},
		BucketCounts:   []uint64{100, 300, 600, 900, 1000},
	}

	snapshot := hdp.ToSnapshot()

	if snapshot.TotalCount != 1000 {
		t.Errorf("TotalCount = %d, want 1000", snapshot.TotalCount)
	}
	if snapshot.Sum != 50000 {
		t.Errorf("Sum = %f, want 50000", snapshot.Sum)
	}
	if snapshot.Min != 1.0 {
		t.Errorf("Min = %f, want 1.0", snapshot.Min)
	}
	if snapshot.Max != 100.0 {
		t.Errorf("Max = %f, want 100.0", snapshot.Max)
	}
	if len(snapshot.Bounds) != 5 {
		t.Errorf("len(Bounds) = %d, want 5", len(snapshot.Bounds))
	}
}

func TestAggregateHistograms(t *testing.T) {
	minVal := 0.5
	maxVal := 10.0

	points := []HistogramDataPoint{
		{
			Timestamp:      time.Now(),
			Name:           "latency",
			Count:          100,
			Sum:            500,
			Min:            &minVal,
			Max:            &maxVal,
			ExplicitBounds: []float64{1, 5, 10},
			BucketCounts:   []uint64{20, 80, 100},
		},
		{
			Timestamp:      time.Now().Add(-time.Second),
			Name:           "latency",
			Count:          150,
			Sum:            750,
			Min:            &minVal,
			Max:            &maxVal,
			ExplicitBounds: []float64{1, 5, 10},
			BucketCounts:   []uint64{30, 120, 150},
		},
	}

	snapshot := AggregateHistograms(points)

	if snapshot.TotalCount != 250 {
		t.Errorf("TotalCount = %d, want 250", snapshot.TotalCount)
	}
	if snapshot.Sum != 1250 {
		t.Errorf("Sum = %f, want 1250", snapshot.Sum)
	}
}

func TestStoreHistogramRoundTrip(t *testing.T) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "histogram_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Record histogram
	minVal := 1.0
	maxVal := 100.0
	now := time.Now()

	hdp := HistogramDataPoint{
		Timestamp:      now,
		Name:           "http_request_duration_seconds",
		Tags:           map[string]string{"method": "GET", "path": "/api"},
		Count:          500,
		Sum:            250.5,
		Min:            &minVal,
		Max:            &maxVal,
		ExplicitBounds: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		BucketCounts:   []uint64{10, 50, 100, 200, 300, 400, 450, 480, 495, 500, 500},
		Exemplars: []Exemplar{
			{
				Value:     0.125,
				Timestamp: now,
				TraceID:   "abc123",
				SpanID:    "def456",
			},
		},
	}

	if err := store.RecordHistogram(hdp); err != nil {
		t.Fatalf("RecordHistogram failed: %v", err)
	}

	// Query back
	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)

	points, err := store.QueryHistogram("http_request_duration_seconds", map[string]string{"method": "GET"}, start, end)
	if err != nil {
		t.Fatalf("QueryHistogram failed: %v", err)
	}

	if len(points) != 1 {
		t.Fatalf("Expected 1 point, got %d", len(points))
	}

	result := points[0]
	if result.Name != "http_request_duration_seconds" {
		t.Errorf("Name = %s, want http_request_duration_seconds", result.Name)
	}
	if result.Count != 500 {
		t.Errorf("Count = %d, want 500", result.Count)
	}
	if result.Tags["method"] != "GET" {
		t.Errorf("Tags[method] = %s, want GET", result.Tags["method"])
	}
	if len(result.ExplicitBounds) != 11 {
		t.Errorf("len(ExplicitBounds) = %d, want 11", len(result.ExplicitBounds))
	}
	if len(result.Exemplars) != 1 {
		t.Errorf("len(Exemplars) = %d, want 1", len(result.Exemplars))
	}
}

func TestStoreHistogramBatch(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "histogram_batch_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	points := make([]HistogramDataPoint, 10)
	for i := 0; i < 10; i++ {
		points[i] = HistogramDataPoint{
			Timestamp:      now.Add(time.Duration(i) * time.Second),
			Name:           "batch_metric",
			Count:          uint64(i * 100),
			Sum:            float64(i * 50),
			ExplicitBounds: []float64{1, 5, 10},
			BucketCounts:   []uint64{uint64(i * 10), uint64(i * 50), uint64(i * 100)},
		}
	}

	if err := store.RecordHistogramBatch(points); err != nil {
		t.Fatalf("RecordHistogramBatch failed: %v", err)
	}

	// Query back
	results, err := store.QueryHistogram("batch_metric", nil, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryHistogram failed: %v", err)
	}

	if len(results) != 10 {
		t.Errorf("Expected 10 results, got %d", len(results))
	}
}

func TestQueryHistogramSnapshot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "histogram_snapshot_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Record multiple histograms
	now := time.Now()
	for i := 0; i < 5; i++ {
		hdp := HistogramDataPoint{
			Timestamp:      now.Add(time.Duration(i) * time.Second),
			Name:           "aggregated_metric",
			Count:          100,
			Sum:            500,
			ExplicitBounds: []float64{10, 25, 50, 100},
			BucketCounts:   []uint64{20, 50, 80, 100},
		}
		if err := store.RecordHistogram(hdp); err != nil {
			t.Fatalf("RecordHistogram failed: %v", err)
		}
	}

	// Query aggregated snapshot
	snapshot, err := store.QueryHistogramSnapshot("aggregated_metric", nil, now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryHistogramSnapshot failed: %v", err)
	}

	if snapshot == nil {
		t.Fatal("Expected snapshot, got nil")
	}

	if snapshot.TotalCount != 500 { // 5 * 100
		t.Errorf("TotalCount = %d, want 500", snapshot.TotalCount)
	}
	if snapshot.Sum != 2500 { // 5 * 500
		t.Errorf("Sum = %f, want 2500", snapshot.Sum)
	}

	// Test quantile computation on aggregated data
	p50 := snapshot.Quantile(0.50)
	if p50 <= 0 || p50 > 100 {
		t.Errorf("p50 = %f, expected between 0 and 100", p50)
	}
}

func TestQuantileEdgeCases(t *testing.T) {
	// Test with single bucket
	single := &HistogramSnapshot{
		Bounds:     []float64{100},
		Counts:     []uint64{50},
		TotalCount: 50,
		Max:        100,
	}

	p50 := single.Quantile(0.50)
	if p50 < 0 || p50 > 100 {
		t.Errorf("Single bucket p50 = %f, expected between 0 and 100", p50)
	}

	// Test quantile bounds clamping
	snapshot := &HistogramSnapshot{
		Bounds:     []float64{10, 20},
		Counts:     []uint64{5, 10},
		TotalCount: 10,
	}

	if snapshot.Quantile(-0.5) != snapshot.Quantile(0) {
		t.Error("Negative quantile should be clamped to 0")
	}

	if snapshot.Quantile(1.5) != snapshot.Quantile(1.0) {
		t.Error("Quantile > 1 should be clamped to 1")
	}
}
