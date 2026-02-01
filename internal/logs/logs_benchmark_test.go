package logs

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Helper to create a test store
func createTestLogStore(b *testing.B) (*Store, func()) {
	b.Helper()
	tmpDir, err := os.MkdirTemp("", "logs_bench_*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		b.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// Helper to generate test log entries
func generateLogEntries(count int) []LogEntry {
	services := []string{"api-gateway", "user-service", "order-service", "payment-service", "notification-service"}
	levels := []LogLevel{LevelDebug, LevelInfo, LevelWarn, LevelError}
	hosts := []string{"worker-1", "worker-2", "worker-3", "api-1", "api-2"}

	entries := make([]LogEntry, count)
	baseTime := time.Now()

	for i := 0; i < count; i++ {
		entries[i] = LogEntry{
			Timestamp: baseTime.Add(time.Duration(i) * time.Millisecond),
			Level:     levels[i%len(levels)],
			Message:   fmt.Sprintf("Log message %d: Processing request for user %d with correlation id %s", i, i*100, fmt.Sprintf("corr-%d", i)),
			Service:   services[i%len(services)],
			Host:      hosts[i%len(hosts)],
			TraceID:   fmt.Sprintf("trace-%08d", i),
			SpanID:    fmt.Sprintf("span-%08d", i),
			Attrs: map[string]string{
				"request_id": fmt.Sprintf("req-%d", i),
				"user_id":    fmt.Sprintf("%d", i*100),
				"method":     []string{"GET", "POST", "PUT", "DELETE"}[i%4],
			},
		}
	}
	return entries
}

func BenchmarkInsertSingle(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   "Test log message for benchmarking",
		Service:   "test-service",
		Host:      "test-host",
		TraceID:   "trace-123",
		SpanID:    "span-456",
		Attrs:     map[string]string{"key": "value"},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entry.ID = "" // Force new ID generation
		_ = store.Insert(entry)
	}
}

func BenchmarkInsertBatch(b *testing.B) {
	batchSizes := []int{10, 100, 500, 1000}

	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize%d", size), func(b *testing.B) {
			store, cleanup := createTestLogStore(b)
			defer cleanup()

			entries := generateLogEntries(size)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Reset IDs for each iteration
				for j := range entries {
					entries[j].ID = ""
				}
				_ = store.InsertBatch(entries)
			}
		})
	}
}

func BenchmarkSearchByTime(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	now := time.Now()
	query := SearchQuery{
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now,
		Limit:     100,
		SortBy:    SortByTime,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.Search(query)
	}
}

func BenchmarkSearchWithFilters(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	tests := []struct {
		name  string
		query SearchQuery
	}{
		{
			"ByLevel",
			SearchQuery{Level: LevelError, Limit: 100},
		},
		{
			"ByService",
			SearchQuery{Service: "api-gateway", Limit: 100},
		},
		{
			"ByTraceID",
			SearchQuery{TraceID: "trace-00001000", Limit: 100},
		},
		{
			"ByLevelAndService",
			SearchQuery{Level: LevelError, Service: "api-gateway", Limit: 100},
		},
		{
			"ByTimeRange",
			SearchQuery{
				StartTime: time.Now().Add(-30 * time.Minute),
				EndTime:   time.Now(),
				Limit:     100,
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = store.Search(tt.query)
			}
		})
	}
}

func BenchmarkBM25Search(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data containing searchable content
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	tests := []struct {
		name  string
		query string
	}{
		{"SingleWord", "Processing"},
		{"TwoWords", "Processing request"},
		{"Phrase", "correlation id"},
		{"ServiceName", "api-gateway"},
		{"Numeric", "user 1000"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			query := SearchQuery{
				Query:  tt.query,
				Limit:  100,
				SortBy: SortByRelevance,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = store.Search(query)
			}
		})
	}
}

func BenchmarkBM25SearchWithFilters(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	query := SearchQuery{
		Query:   "Processing",
		Level:   LevelError,
		Service: "api-gateway",
		Limit:   100,
		SortBy:  SortByRelevance,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.Search(query)
	}
}

func BenchmarkSearchPagination(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	offsets := []int{0, 100, 500, 1000, 5000}

	for _, offset := range offsets {
		b.Run(fmt.Sprintf("Offset%d", offset), func(b *testing.B) {
			query := SearchQuery{
				Limit:  100,
				Offset: offset,
				SortBy: SortByTime,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = store.Search(query)
			}
		})
	}
}

func BenchmarkGetByTraceID(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.GetByTraceID("trace-00001000")
	}
}

func BenchmarkGetServices(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.GetServices()
	}
}

func BenchmarkGetStats(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with data
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	durations := []time.Duration{
		1 * time.Hour,
		24 * time.Hour,
		7 * 24 * time.Hour,
	}

	for _, d := range durations {
		b.Run(d.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = store.GetStats(d)
			}
		})
	}
}

func BenchmarkCleanup(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate with old data
	oldEntries := make([]LogEntry, 1000)
	oldTime := time.Now().Add(-48 * time.Hour)
	for i := range oldEntries {
		oldEntries[i] = LogEntry{
			Timestamp: oldTime.Add(time.Duration(i) * time.Second),
			Level:     LevelInfo,
			Message:   fmt.Sprintf("Old log message %d", i),
			Service:   "old-service",
		}
	}
	_ = store.InsertBatch(oldEntries)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = store.Cleanup(24 * time.Hour)
	}
}

// Benchmark with different data sizes
func BenchmarkSearchScaling(b *testing.B) {
	dataSizes := []int{1000, 10000, 50000}

	for _, size := range dataSizes {
		b.Run(fmt.Sprintf("DataSize%d", size), func(b *testing.B) {
			store, cleanup := createTestLogStore(b)
			defer cleanup()

			entries := generateLogEntries(size)
			_ = store.InsertBatch(entries)

			query := SearchQuery{
				Query:  "Processing",
				Limit:  100,
				SortBy: SortByRelevance,
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, _ = store.Search(query)
			}
		})
	}
}

// Benchmark concurrent access
func BenchmarkConcurrentInsert(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		entry := &LogEntry{
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   "Concurrent insert test",
			Service:   "test-service",
		}
		for pb.Next() {
			entry.ID = ""
			_ = store.Insert(entry)
		}
	})
}

func BenchmarkConcurrentSearch(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate
	entries := generateLogEntries(10000)
	_ = store.InsertBatch(entries)

	queries := []SearchQuery{
		{Query: "Processing", Limit: 100, SortBy: SortByRelevance},
		{Level: LevelError, Limit: 100},
		{Service: "api-gateway", Limit: 100},
		{StartTime: time.Now().Add(-1 * time.Hour), Limit: 100},
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = store.Search(queries[i%len(queries)])
			i++
		}
	})
}

func BenchmarkConcurrentReadWrite(b *testing.B) {
	store, cleanup := createTestLogStore(b)
	defer cleanup()

	// Pre-populate
	entries := generateLogEntries(5000)
	_ = store.InsertBatch(entries)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		entry := &LogEntry{
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   "Concurrent test",
			Service:   "test-service",
		}
		query := SearchQuery{Query: "Processing", Limit: 10}
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				entry.ID = ""
				_ = store.Insert(entry)
			} else {
				_, _ = store.Search(query)
			}
			i++
		}
	})
}

// Benchmark log entry creation
func BenchmarkLogEntryCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = LogEntry{
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   "Test message with some content",
			Service:   "test-service",
			Host:      "test-host",
			TraceID:   "trace-123456",
			SpanID:    "span-789012",
			Attrs: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		}
	}
}

// Benchmark search query creation
func BenchmarkSearchQueryCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SearchQuery{
			Query:     "test search",
			Level:     LevelError,
			Service:   "api-gateway",
			TraceID:   "trace-123",
			StartTime: time.Now().Add(-1 * time.Hour),
			EndTime:   time.Now(),
			Limit:     100,
			Offset:    0,
			SortBy:    SortByRelevance,
		}
	}
}

// Benchmark result processing
func BenchmarkSearchResultProcessing(b *testing.B) {
	result := &SearchResult{
		Entries: make([]ScoredLogEntry, 100),
		TotalCount: 1000,
		HasMore: true,
	}
	for i := range result.Entries {
		result.Entries[i] = ScoredLogEntry{
			LogEntry: LogEntry{
				ID:        fmt.Sprintf("id-%d", i),
				Timestamp: time.Now(),
				Level:     LevelInfo,
				Message:   fmt.Sprintf("Message %d", i),
				Service:   "test-service",
			},
			Score: float64(i) * 0.1,
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = result.LogEntries()
	}
}
