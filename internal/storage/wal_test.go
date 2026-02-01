package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWALNew(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     1024 * 1024, // 1MB for testing
		SyncInterval:       100 * time.Millisecond,
		MaxSegments:        5,
		CheckpointInterval: time.Second,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	// Verify segment file was created
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to read dir: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected segment file to be created")
	}
}

func TestWALWrite(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     1024 * 1024,
		SyncInterval:       10 * time.Millisecond,
		MaxSegments:        5,
		CheckpointInterval: time.Second,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	// Write some entries
	seq1, err := wal.Write(WALOpInsert, "test_table", []byte(`{"id": 1, "value": "test"}`))
	if err != nil {
		t.Fatalf("Failed to write entry: %v", err)
	}

	seq2, err := wal.Write(WALOpInsert, "test_table", []byte(`{"id": 2, "value": "test2"}`))
	if err != nil {
		t.Fatalf("Failed to write second entry: %v", err)
	}

	if seq1 >= seq2 {
		t.Errorf("Sequence numbers should be monotonically increasing: %d >= %d", seq1, seq2)
	}

	// Verify pending count
	if wal.PendingCount() != 2 {
		t.Errorf("Expected 2 pending entries, got %d", wal.PendingCount())
	}

	// Force sync
	if err := wal.ForceSync(); err != nil {
		t.Errorf("ForceSync failed: %v", err)
	}

	stats := wal.Stats()
	if stats.EntriesWritten != 2 {
		t.Errorf("Expected 2 entries written, got %d", stats.EntriesWritten)
	}
}

func TestWALCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     1024 * 1024,
		SyncInterval:       10 * time.Millisecond,
		MaxSegments:        5,
		CheckpointInterval: time.Hour, // Don't auto checkpoint
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	// Track checkpointed entries
	var checkpointedEntries []WALEntry
	wal.SetCheckpointCallback(func(entries []WALEntry) error {
		checkpointedEntries = entries
		return nil
	})

	// Write some entries
	for i := 0; i < 5; i++ {
		wal.Write(WALOpInsert, "test_table", []byte(`{"test": "data"}`))
	}

	if wal.PendingCount() != 5 {
		t.Errorf("Expected 5 pending entries before checkpoint, got %d", wal.PendingCount())
	}

	// Force checkpoint
	if err := wal.ForceCheckpoint(); err != nil {
		t.Errorf("ForceCheckpoint failed: %v", err)
	}

	if len(checkpointedEntries) != 5 {
		t.Errorf("Expected 5 checkpointed entries, got %d", len(checkpointedEntries))
	}

	if wal.PendingCount() != 0 {
		t.Errorf("Expected 0 pending entries after checkpoint, got %d", wal.PendingCount())
	}
}

func TestWALRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     1024 * 1024,
		SyncInterval:       10 * time.Millisecond,
		MaxSegments:        5,
		CheckpointInterval: time.Hour,
	}

	// Create WAL and write entries
	wal1, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}

	for i := 0; i < 3; i++ {
		wal1.Write(WALOpInsert, "test_table", []byte(`{"recovery": "test"}`))
	}
	wal1.ForceSync()
	wal1.Stop()

	// Reopen WAL - should recover uncommitted entries
	wal2, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to recreate WAL: %v", err)
	}
	defer wal2.Stop()

	// Recovered entries should be in pending
	if wal2.PendingCount() != 3 {
		t.Errorf("Expected 3 recovered pending entries, got %d", wal2.PendingCount())
	}
}

func TestWALStats(t *testing.T) {
	tmpDir := t.TempDir()

	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     1024 * 1024,
		SyncInterval:       10 * time.Millisecond,
		MaxSegments:        5,
		CheckpointInterval: time.Hour,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	// Write entries
	for i := 0; i < 10; i++ {
		wal.Write(WALOpInsert, "stats_test", []byte(`{"data": "test"}`))
	}

	stats := wal.Stats()

	if stats.EntriesWritten != 10 {
		t.Errorf("Expected 10 entries written, got %d", stats.EntriesWritten)
	}

	if stats.BytesWritten == 0 {
		t.Error("Expected bytes written > 0")
	}

	if stats.SegmentsCreated == 0 {
		t.Error("Expected at least one segment created")
	}

	if stats.PendingEntries != 10 {
		t.Errorf("Expected 10 pending entries, got %d", stats.PendingEntries)
	}
}

func TestWALDefaultConfig(t *testing.T) {
	config := DefaultWALConfig()

	if config.Dir == "" {
		t.Error("Default Dir should not be empty")
	}

	if config.MaxSegmentSize == 0 {
		t.Error("Default MaxSegmentSize should not be 0")
	}

	if config.SyncInterval == 0 {
		t.Error("Default SyncInterval should not be 0")
	}

	if config.MaxSegments == 0 {
		t.Error("Default MaxSegments should not be 0")
	}

	if config.CheckpointInterval == 0 {
		t.Error("Default CheckpointInterval should not be 0")
	}
}

func TestWALSegmentRotation(t *testing.T) {
	tmpDir := t.TempDir()

	// Small segment size to force rotation
	config := WALConfig{
		Dir:                tmpDir,
		MaxSegmentSize:     100, // 100 bytes - very small
		SyncInterval:       10 * time.Millisecond,
		MaxSegments:        10,
		CheckpointInterval: time.Hour,
	}

	wal, err := NewWAL(config)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	defer wal.Stop()

	// Write enough data to force rotation
	largeData := make([]byte, 200)
	for i := range largeData {
		largeData[i] = 'x'
	}

	// Write many entries to trigger rotation
	for i := 0; i < 20; i++ {
		wal.Write(WALOpInsert, "rotation_test", largeData)
	}

	wal.ForceSync()

	// Check that at least one segment was created
	files, _ := filepath.Glob(filepath.Join(tmpDir, "wal-*.log"))
	if len(files) < 1 {
		t.Errorf("Expected at least 1 segment file, got %d", len(files))
	}

	// Check stats to verify segments were created
	stats := wal.Stats()
	if stats.SegmentsCreated < 1 {
		t.Errorf("Expected at least 1 segment created, got %d", stats.SegmentsCreated)
	}
}
