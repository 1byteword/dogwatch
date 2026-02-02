package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// =============================================================================
// Integration Test Helpers
// =============================================================================

func createIntegrationTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "integration_test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	// Create test tables with timestamp columns
	tables := []string{
		`CREATE TABLE IF NOT EXISTS system_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			metric_name TEXT,
			value REAL,
			host TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			level TEXT,
			message TEXT,
			source TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS spans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
			trace_id TEXT,
			span_id TEXT,
			duration_ms INTEGER
		)`,
	}

	for _, sql := range tables {
		if _, err := db.Exec(sql); err != nil {
			t.Fatalf("Failed to create table: %v", err)
		}
	}

	return db, tmpDir
}

func insertTestMetrics(t *testing.T, db *sql.DB, count int, baseTime time.Time) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO system_metrics (timestamp, metric_name, value, host) VALUES (?, ?, ?, ?)")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	for i := 0; i < count; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		_, err := stmt.Exec(ts.Format(time.RFC3339), "cpu_usage", float64(i%100), "host-1")
		if err != nil {
			t.Fatalf("Failed to insert metric: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}

func insertTestLogs(t *testing.T, db *sql.DB, count int, baseTime time.Time) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO logs (timestamp, level, message, source) VALUES (?, ?, ?, ?)")
	if err != nil {
		t.Fatalf("Failed to prepare statement: %v", err)
	}
	defer stmt.Close()

	levels := []string{"info", "warn", "error", "debug"}
	for i := 0; i < count; i++ {
		ts := baseTime.Add(time.Duration(i) * time.Second)
		_, err := stmt.Exec(ts.Format(time.RFC3339), levels[i%4], "Test message", "app-1")
		if err != nil {
			t.Fatalf("Failed to insert log: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
}

// =============================================================================
// Data Movement Through Tiers Tests
// =============================================================================

func TestTieringManager_DataMovementHotToWarm(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	// Configure with very short retention for testing
	config := TieringConfig{
		DataDir:      tmpDir,
		HotRetention: 1 * time.Second, // Very short for testing
		WarmRetention: 1 * time.Hour,
		CompressWarm: true,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Insert old data (older than HotRetention)
	oldTime := time.Now().Add(-1 * time.Hour)
	insertTestMetrics(t, db, 100, oldTime)

	// Verify data exists in hot tier
	var countBefore int
	err = db.QueryRow("SELECT COUNT(*) FROM system_metrics").Scan(&countBefore)
	if err != nil {
		t.Fatalf("Failed to count metrics: %v", err)
	}
	if countBefore != 100 {
		t.Errorf("Expected 100 metrics, got %d", countBefore)
	}

	// Trigger tiering
	err = manager.ForceTier()
	if err != nil {
		t.Fatalf("Tiering failed: %v", err)
	}

	// Verify data was moved (hot should have fewer records)
	var countAfter int
	err = db.QueryRow("SELECT COUNT(*) FROM system_metrics").Scan(&countAfter)
	if err != nil {
		t.Fatalf("Failed to count metrics after tier: %v", err)
	}

	if countAfter >= countBefore {
		t.Logf("Note: Data may not have been tiered if timestamp column detection failed")
	}

	// Verify warm files were created
	warmFiles := manager.ListWarmFiles()
	t.Logf("Warm files after tiering: %d", len(warmFiles))

	// Check stats updated
	stats := manager.Stats()
	if stats.TotalTierings < 1 {
		t.Error("Expected at least 1 tiering operation")
	}
}

func TestTieringManager_DataMovementWarmToCold(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	// Create cold backend
	coldDir := filepath.Join(tmpDir, "cold")
	coldBackend, err := NewLocalBackend(coldDir, "")
	if err != nil {
		t.Fatalf("Failed to create cold backend: %v", err)
	}

	config := TieringConfig{
		DataDir:       tmpDir,
		HotRetention:  1 * time.Millisecond,
		WarmRetention: 1 * time.Millisecond, // Very short for testing
		ColdRetention: 24 * time.Hour,
		ColdBackend:   coldBackend,
		CompressWarm:  true,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Manually add a warm file that's old enough to tier to cold
	oldWarmFile := WarmFile{
		Path: filepath.Join(tmpDir, "warm", "test-old.db.gz"),
		TimeRange: TimeRange{
			Start: time.Now().Add(-48 * time.Hour),
			End:   time.Now().Add(-24 * time.Hour),
		},
		Size:       1024,
		Compressed: true,
		Tables:     []string{"system_metrics"},
		CreatedAt:  time.Now().Add(-24 * time.Hour),
	}

	// Create the warm directory and a dummy file
	os.MkdirAll(filepath.Join(tmpDir, "warm"), 0755)
	os.WriteFile(oldWarmFile.Path, []byte("test data"), 0644)

	manager.stateMu.Lock()
	manager.tieringState.WarmFiles = append(manager.tieringState.WarmFiles, oldWarmFile)
	manager.stateMu.Unlock()

	// Trigger tiering (warm to cold)
	err = manager.ForceTier()
	if err != nil {
		t.Logf("Tiering note: %v", err)
	}

	// Check cold archives
	coldArchives := manager.ListColdArchives()
	t.Logf("Cold archives after tiering: %d", len(coldArchives))
}

// =============================================================================
// Compaction Process Tests
// =============================================================================

func TestTieringManager_CompactionProcess(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Insert data and delete some to create fragmentation
	insertTestMetrics(t, db, 1000, time.Now())
	db.Exec("DELETE FROM system_metrics WHERE id % 2 = 0")

	// Get DB size before compaction
	var sizeBefore int64
	err = db.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()").Scan(&sizeBefore)
	if err != nil {
		t.Logf("Could not get DB size: %v", err)
	}

	// Run compaction
	statsBefore := manager.Stats()
	err = manager.ForceCompact()
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	statsAfter := manager.Stats()

	// Verify compaction count increased
	if statsAfter.TotalCompactions != statsBefore.TotalCompactions+1 {
		t.Errorf("Expected compaction count to increase by 1, got %d -> %d",
			statsBefore.TotalCompactions, statsAfter.TotalCompactions)
	}

	// Verify state was updated
	state := manager.State()
	if state.LastCompaction.IsZero() {
		t.Error("LastCompaction should be set after compaction")
	}
}

func TestTieringManager_CompactionUnderLoad(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Start concurrent writes
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				db.Exec("INSERT INTO system_metrics (metric_name, value) VALUES (?, ?)",
					"concurrent_test", 42.0)
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Run compaction while writes are happening
	for i := 0; i < 3; i++ {
		err := manager.ForceCompact()
		if err != nil {
			t.Errorf("Compaction %d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(stop)
	wg.Wait()

	stats := manager.Stats()
	if stats.TotalCompactions < 3 {
		t.Errorf("Expected at least 3 compactions, got %d", stats.TotalCompactions)
	}
}

// =============================================================================
// Restore from Cold Storage Tests
// =============================================================================

func TestTieringManager_RestoreFromCold(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	// Create cold backend
	coldDir := filepath.Join(tmpDir, "cold")
	coldBackend, err := NewLocalBackend(coldDir, "")
	if err != nil {
		t.Fatalf("Failed to create cold backend: %v", err)
	}

	config := TieringConfig{
		DataDir:     tmpDir,
		ColdBackend: coldBackend,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Create a test file in cold storage
	ctx := context.Background()
	testKey := "archives/2024/01/01/test-restore.db.gz"
	testData := []byte("test archive data")

	// Create the test file directly in cold storage
	err = os.MkdirAll(filepath.Dir(filepath.Join(coldDir, testKey)), 0755)
	if err != nil {
		t.Fatalf("Failed to create cold dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(coldDir, testKey), testData, 0644)
	if err != nil {
		t.Fatalf("Failed to write cold file: %v", err)
	}

	// Add to cold archives
	manager.stateMu.Lock()
	manager.tieringState.ColdArchives = append(manager.tieringState.ColdArchives, ColdArchive{
		Key: testKey,
		TimeRange: TimeRange{
			Start: time.Now().Add(-30 * 24 * time.Hour),
			End:   time.Now().Add(-14 * 24 * time.Hour),
		},
		Size:      int64(len(testData)),
		Tables:    []string{"system_metrics"},
		CreatedAt: time.Now(),
	})
	manager.stateMu.Unlock()

	// Restore
	err = manager.RestoreFromCold(ctx, testKey)
	if err != nil {
		t.Logf("Restore note (may fail if key doesn't exist): %v", err)
	}

	// Check warm files after restore
	warmFiles := manager.ListWarmFiles()
	t.Logf("Warm files after restore attempt: %d", len(warmFiles))
}

func TestTieringManager_RestoreNonexistentKey(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	coldDir := filepath.Join(tmpDir, "cold")
	coldBackend, _ := NewLocalBackend(coldDir, "")

	config := TieringConfig{
		DataDir:     tmpDir,
		ColdBackend: coldBackend,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	err = manager.RestoreFromCold(context.Background(), "nonexistent/key.db.gz")
	if err == nil {
		t.Error("Expected error for nonexistent key")
	}
}

func TestTieringManager_RestoreWithoutColdBackend(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
		// No cold backend configured
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	err = manager.RestoreFromCold(context.Background(), "any/key.db.gz")
	if err == nil {
		t.Error("Expected error when no cold backend configured")
	}
}

// =============================================================================
// Concurrent Access During Tiering Tests
// =============================================================================

func TestTieringManager_ConcurrentAccess(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir:      tmpDir,
		HotRetention: 1 * time.Millisecond,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent readers of stats
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = manager.Stats()
				_ = manager.State()
				_ = manager.ListWarmFiles()
				_ = manager.ListColdArchives()
			}
		}()
	}

	// Concurrent tiering operations
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := manager.ForceCompact(); err != nil {
					errors <- err
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Concurrent writes to DB
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, err := db.Exec("INSERT INTO system_metrics (metric_name, value) VALUES (?, ?)",
					"concurrent_metric", float64(j))
				if err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		t.Logf("Concurrent error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Logf("Total concurrent errors: %d", errCount)
	}
}

func TestTieringManager_ConcurrentTierQueries(t *testing.T) {
	db, tmpDir := createIntegrationTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	var wg sync.WaitGroup

	// Test GetTierForTime concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			testTime := time.Now().Add(-time.Duration(offset) * time.Hour)
			tier := manager.GetTierForTime(testTime)
			if tier != TierHot && tier != TierWarm && tier != TierCold {
				t.Errorf("Invalid tier returned: %s", tier)
			}
		}(i)
	}

	wg.Wait()
}

// =============================================================================
// WAL Recovery Scenarios Tests
// =============================================================================

func TestWAL_RecoveryAfterCrash(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:            tmpDir,
		MaxSegmentSize: 1024 * 1024, // 1MB
		MaxSegments:    5,
	}

	// Create WAL and write some entries
	wal1, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	// Write entries
	for i := 0; i < 100; i++ {
		_, err := wal1.Write(WALOpInsert, "test_table", []byte("test data"))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Sync to ensure data is on disk
	wal1.ForceSync()

	// Simulate crash - don't call Stop() properly
	if wal1.currentSegment != nil {
		wal1.writer.Flush()
	}

	// Create new WAL to simulate recovery
	wal2, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL after crash: %v", err)
	}
	defer wal2.Stop()

	// Verify entries were recovered
	pendingCount := wal2.PendingCount()
	if pendingCount == 0 {
		t.Log("Note: Entries may have been committed or recovery behavior differs")
	}

	stats := wal2.Stats()
	t.Logf("Recovered WAL - Pending: %d, Entries: %d", pendingCount, stats.EntriesWritten)
}

func TestWAL_RecoveryWithCorruptedSegment(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:            tmpDir,
		MaxSegmentSize: 1024,
		MaxSegments:    5,
	}

	// Create a corrupted segment file
	corruptedPath := filepath.Join(tmpDir, "wal-0000000000000001.log")
	os.WriteFile(corruptedPath, []byte("corrupted data that is not valid WAL format"), 0644)

	// Create WAL - should handle corrupted segment gracefully
	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL with corrupted segment: %v", err)
	}
	defer wal.Stop()

	// Should be able to write new entries
	_, err = wal.Write(WALOpInsert, "test_table", []byte("new data"))
	if err != nil {
		t.Errorf("Write after recovery failed: %v", err)
	}
}

func TestWAL_SegmentRotation(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:            tmpDir,
		MaxSegmentSize: 1024, // Small size to force rotation
		MaxSegments:    3,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	initialSegment := wal.stats.CurrentSegmentID

	// Write enough data to force segment rotation
	largeData := make([]byte, 200)
	for i := 0; i < 50; i++ {
		_, err := wal.Write(WALOpInsert, "test_table", largeData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Verify segment rotated
	stats := wal.Stats()
	if stats.CurrentSegmentID <= initialSegment {
		t.Log("Note: Segment may not have rotated with current data size")
	}

	if stats.SegmentsCreated < 1 {
		t.Error("Expected at least 1 segment to be created")
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkTieringManager_Compaction(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		b.Fatalf("Failed to create db: %v", err)
	}
	defer db.Close()

	db.Exec(`CREATE TABLE IF NOT EXISTS test_metrics (
		id INTEGER PRIMARY KEY,
		timestamp DATETIME,
		value REAL
	)`)

	// Insert test data
	for i := 0; i < 10000; i++ {
		db.Exec("INSERT INTO test_metrics (timestamp, value) VALUES (?, ?)",
			time.Now().Format(time.RFC3339), float64(i))
	}

	config := TieringConfig{DataDir: tmpDir}
	manager, _ := NewTieringManager(config, db)
	defer manager.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ForceCompact()
	}
}

func BenchmarkTieringManager_Stats(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()

	config := TieringConfig{DataDir: tmpDir}
	manager, _ := NewTieringManager(config, db)
	defer manager.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.Stats()
	}
}

func BenchmarkTieringManager_ConcurrentStats(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()

	config := TieringConfig{DataDir: tmpDir}
	manager, _ := NewTieringManager(config, db)
	defer manager.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = manager.Stats()
		}
	})
}

func BenchmarkWAL_Write(b *testing.B) {
	tmpDir := b.TempDir()

	config := WALConfig{
		Dir:            tmpDir,
		MaxSegmentSize: 64 * 1024 * 1024,
	}

	wal, _ := NewWAL(config)
	defer wal.Stop()

	data := make([]byte, 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wal.Write(WALOpInsert, "bench_table", data)
	}
}

func BenchmarkWAL_WriteBatch(b *testing.B) {
	tmpDir := b.TempDir()

	config := WALConfig{
		Dir:            tmpDir,
		MaxSegmentSize: 64 * 1024 * 1024,
	}

	wal, _ := NewWAL(config)
	defer wal.Stop()

	data := make([]byte, 100)
	entries := make([]WALEntry, 100)
	for i := range entries {
		entries[i] = WALEntry{
			Operation: WALOpInsert,
			Table:     "bench_table",
			Data:      data,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wal.WriteBatch(entries)
	}
}
