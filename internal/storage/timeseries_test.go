package storage

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestTimeSeriesOptimizer_BatchInsert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	config := DefaultTimeSeriesConfig()
	config.MaxBatchSize = 3
	config.FlushInterval = 100 * time.Millisecond

	opt := NewTimeSeriesOptimizer(db, config)

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE test_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT,
			value REAL,
			name TEXT
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	now := time.Now()

	// Insert rows one at a time
	for i := 0; i < 5; i++ {
		row := BatchRow{
			Table:     "test_metrics",
			Columns:   []string{"timestamp", "value", "name"},
			Values:    []interface{}{now.Format(time.RFC3339), float64(i), "metric-" + string(rune('a'+i))},
			Timestamp: now,
		}
		if err := opt.BatchInsert(row); err != nil {
			t.Fatalf("BatchInsert failed: %v", err)
		}
	}

	// Flush remaining
	opt.FlushBatches()

	// Verify all rows were inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_metrics").Scan(&count)
	if err != nil {
		t.Fatalf("Count query failed: %v", err)
	}

	if count != 5 {
		t.Errorf("Expected 5 rows, got %d", count)
	}
}

func TestTimeSeriesOptimizer_QueryWithTimeRange(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	opt := NewTimeSeriesOptimizer(db, DefaultTimeSeriesConfig())

	// Create test table with data
	_, err := db.Exec(`
		CREATE TABLE metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT,
			value REAL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Insert test data
	now := time.Now()
	for i := -5; i <= 5; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		_, err := db.Exec("INSERT INTO metrics (timestamp, value) VALUES (?, ?)",
			ts.Format(time.RFC3339), float64(i+10))
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	// Query with time range
	ctx := context.Background()
	start := now.Add(-2 * time.Hour)
	end := now.Add(2 * time.Hour)

	rows, plan, err := opt.QueryWithTimeRange(ctx, TimeRangeQuery{
		Table:     "metrics",
		StartTime: start,
		EndTime:   end,
		Columns:   []string{"value"},
	})
	if err != nil {
		t.Fatalf("QueryWithTimeRange failed: %v", err)
	}
	defer rows.Close()

	// Count results
	var count int
	for rows.Next() {
		count++
	}

	// Should get rows for -2, -1, 0, 1 hours (4 or 5 rows depending on boundary)
	if count < 4 || count > 5 {
		t.Errorf("Expected 4-5 rows, got %d", count)
	}

	// Check plan has optimizations
	if len(plan.Optimizations) == 0 {
		t.Error("Expected optimizations in query plan")
	}
}

func TestTimeSeriesOptimizer_DownsampleRule(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	opt := NewTimeSeriesOptimizer(db, DefaultTimeSeriesConfig())

	// Add a rule
	rule := DownsampleRule{
		SourceTable:     "raw_metrics",
		TargetTable:     "hourly_metrics",
		SourceInterval:  time.Minute,
		TargetInterval:  time.Hour,
		AggregateColumn: "value",
		AggregateFunc:   "avg",
		GroupByColumns:  []string{"service"},
		Enabled:         true,
	}

	opt.AddDownsampleRule(rule)

	// Verify rule was added
	rules := opt.ListDownsampleRules()
	if len(rules) != 1 {
		t.Fatalf("Expected 1 rule, got %d", len(rules))
	}

	if rules[0].SourceTable != "raw_metrics" {
		t.Errorf("Expected source table raw_metrics, got %s", rules[0].SourceTable)
	}

	// Disable rule
	opt.SetDownsampleRuleEnabled("raw_metrics", false)
	rules = opt.ListDownsampleRules()
	if rules[0].Enabled {
		t.Error("Rule should be disabled")
	}

	// Remove rule
	opt.RemoveDownsampleRule("raw_metrics")
	rules = opt.ListDownsampleRules()
	if len(rules) != 0 {
		t.Errorf("Expected 0 rules after removal, got %d", len(rules))
	}
}

func TestTimeSeriesOptimizer_GetPartitionForTime(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	config := DefaultTimeSeriesConfig()
	config.PartitionInterval = time.Hour
	opt := NewTimeSeriesOptimizer(db, config)

	// Get partition for a specific time
	ts := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
	partName := opt.GetPartitionForTime("metrics", ts)

	expected := "metrics_p20240115_140000"
	if partName != expected {
		t.Errorf("Expected partition %s, got %s", expected, partName)
	}

	// Same hour should get same partition
	ts2 := time.Date(2024, 1, 15, 14, 45, 0, 0, time.UTC)
	partName2 := opt.GetPartitionForTime("metrics", ts2)
	if partName != partName2 {
		t.Errorf("Same hour should get same partition: %s vs %s", partName, partName2)
	}

	// Different hour should get different partition
	ts3 := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)
	partName3 := opt.GetPartitionForTime("metrics", ts3)
	if partName == partName3 {
		t.Error("Different hour should get different partition")
	}
}

func TestTimeSeriesOptimizer_OptimizeForTimeSeries(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	opt := NewTimeSeriesOptimizer(db, DefaultTimeSeriesConfig())

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE test_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT,
			value REAL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	// Apply optimizations
	err = opt.OptimizeForTimeSeries("test_data", "timestamp")
	if err != nil {
		t.Fatalf("OptimizeForTimeSeries failed: %v", err)
	}

	// Verify index was created
	var indexCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND name LIKE 'idx_test_data_timestamp%'
	`).Scan(&indexCount)
	if err != nil {
		t.Fatalf("Index check failed: %v", err)
	}

	if indexCount == 0 {
		t.Error("Expected time index to be created")
	}
}

func TestTimeSeriesOptimizer_Stats(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	config := DefaultTimeSeriesConfig()
	config.MaxBatchSize = 100
	config.FlushInterval = 50 * time.Millisecond

	opt := NewTimeSeriesOptimizer(db, config)

	stats := opt.GetStats()

	if stats.MaxBatchSize != 100 {
		t.Errorf("Expected max batch size 100, got %d", stats.MaxBatchSize)
	}

	if stats.FlushIntervalMs != 50 {
		t.Errorf("Expected flush interval 50ms, got %d", stats.FlushIntervalMs)
	}

	if stats.PartitionInterval != "1h0m0s" {
		t.Errorf("Expected partition interval 1h0m0s, got %s", stats.PartitionInterval)
	}
}

func TestTimeSeriesOptimizer_ListPartitions(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	opt := NewTimeSeriesOptimizer(db, DefaultTimeSeriesConfig())

	// Create some partitions
	baseTime := time.Now()
	for i := 0; i < 3; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Hour)
		opt.GetPartitionForTime("metrics", ts)
	}

	partitions := opt.ListPartitions("metrics")
	if len(partitions) != 3 {
		t.Errorf("Expected 3 partitions, got %d", len(partitions))
	}

	// Should be sorted by start time (descending)
	if len(partitions) >= 2 {
		if partitions[0].StartTime.Before(partitions[1].StartTime) {
			t.Error("Partitions should be sorted descending by start time")
		}
	}
}

func TestTimeSeriesOptimizer_CreateCoveringIndex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	opt := NewTimeSeriesOptimizer(db, DefaultTimeSeriesConfig())

	// Create test table
	_, err := db.Exec(`
		CREATE TABLE events (
			id INTEGER PRIMARY KEY,
			timestamp TEXT,
			service TEXT,
			value REAL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Create covering index
	err = opt.CreateCoveringIndex("events", []string{"timestamp", "service"}, []string{"value"})
	if err != nil {
		t.Fatalf("CreateCoveringIndex failed: %v", err)
	}

	// Verify index exists
	var indexName string
	err = db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='index' AND name LIKE 'idx_events%covering'
	`).Scan(&indexName)
	if err != nil {
		t.Fatalf("Index verification failed: %v", err)
	}

	if indexName == "" {
		t.Error("Covering index was not created")
	}
}

func TestBuildGroupBySchema(t *testing.T) {
	tests := []struct {
		cols []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"service"}, "service TEXT,"},
		{[]string{"service", "region"}, "service TEXT, region TEXT,"},
	}

	for _, tt := range tests {
		got := buildGroupBySchema(tt.cols)
		if got != tt.want {
			t.Errorf("buildGroupBySchema(%v) = %q, want %q", tt.cols, got, tt.want)
		}
	}
}

func TestBuildGroupByPrimaryKey(t *testing.T) {
	tests := []struct {
		cols []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"service"}, ", service"},
		{[]string{"service", "region"}, ", service, region"},
	}

	for _, tt := range tests {
		got := buildGroupByPrimaryKey(tt.cols)
		if got != tt.want {
			t.Errorf("buildGroupByPrimaryKey(%v) = %q, want %q", tt.cols, got, tt.want)
		}
	}
}

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "timeseries_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	db, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to open database: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}
