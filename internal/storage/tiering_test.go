package storage

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func createTestDB(t *testing.T) (*sql.DB, string) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	// Create a test table with timestamp column
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS test_metrics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			value REAL
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create test table: %v", err)
	}

	return db, tmpDir
}

func TestTieringManagerNew(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir:            tmpDir,
		HotRetention:       24 * time.Hour,
		WarmRetention:      7 * 24 * time.Hour,
		ColdRetention:      90 * 24 * time.Hour,
		CompactionInterval: time.Hour,
		TieringInterval:    6 * time.Hour,
		CompressWarm:       true,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	if manager.hotDB == nil {
		t.Error("Expected hotDB to be set")
	}
}

func TestTieringManagerStats(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir:      tmpDir,
		HotRetention: 24 * time.Hour,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	stats := manager.Stats()

	// Initial stats should have zero or minimal values
	if stats.TotalCompactions < 0 {
		t.Error("TotalCompactions should not be negative")
	}

	if stats.TotalTierings < 0 {
		t.Error("TotalTierings should not be negative")
	}
}

func TestTieringManagerState(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	state := manager.State()

	// Initial state should be empty
	if len(state.WarmFiles) != 0 {
		t.Errorf("Expected 0 warm files, got %d", len(state.WarmFiles))
	}

	if len(state.ColdArchives) != 0 {
		t.Errorf("Expected 0 cold archives, got %d", len(state.ColdArchives))
	}
}

func TestTieringManagerCompact(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Insert some test data
	for i := 0; i < 10; i++ {
		db.Exec("INSERT INTO test_metrics (value) VALUES (?)", float64(i))
	}

	// Run compaction
	err = manager.ForceCompact()
	if err != nil {
		t.Fatalf("Compaction failed: %v", err)
	}

	stats := manager.Stats()
	if stats.TotalCompactions != 1 {
		t.Errorf("Expected 1 compaction, got %d", stats.TotalCompactions)
	}
}

func TestTieringManagerGetTierForTime(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir:       tmpDir,
		HotRetention:  24 * time.Hour,
		WarmRetention: 7 * 24 * time.Hour,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	t.Run("RecentTimeIsHot", func(t *testing.T) {
		tier := manager.GetTierForTime(time.Now())
		if tier != TierHot {
			t.Errorf("Expected hot tier for recent time, got %s", tier)
		}
	})

	t.Run("YesterdayIsHot", func(t *testing.T) {
		yesterday := time.Now().Add(-12 * time.Hour)
		tier := manager.GetTierForTime(yesterday)
		if tier != TierHot {
			t.Errorf("Expected hot tier for yesterday, got %s", tier)
		}
	})
}

func TestTieringConfigDefaults(t *testing.T) {
	config := DefaultTieringConfig()

	if config.DataDir == "" {
		t.Error("Default DataDir should not be empty")
	}

	if config.HotRetention != 24*time.Hour {
		t.Errorf("Expected HotRetention of 24h, got %v", config.HotRetention)
	}

	if config.WarmRetention != 7*24*time.Hour {
		t.Errorf("Expected WarmRetention of 7d, got %v", config.WarmRetention)
	}

	if config.ColdRetention != 90*24*time.Hour {
		t.Errorf("Expected ColdRetention of 90d, got %v", config.ColdRetention)
	}

	if config.CompactionInterval != time.Hour {
		t.Errorf("Expected CompactionInterval of 1h, got %v", config.CompactionInterval)
	}

	if config.TieringInterval != 6*time.Hour {
		t.Errorf("Expected TieringInterval of 6h, got %v", config.TieringInterval)
	}
}

func TestStorageTierConstants(t *testing.T) {
	if TierHot != "hot" {
		t.Errorf("Expected TierHot to be 'hot', got '%s'", TierHot)
	}

	if TierWarm != "warm" {
		t.Errorf("Expected TierWarm to be 'warm', got '%s'", TierWarm)
	}

	if TierCold != "cold" {
		t.Errorf("Expected TierCold to be 'cold', got '%s'", TierCold)
	}
}

func TestTimeRange(t *testing.T) {
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()

	tr := TimeRange{
		Start: start,
		End:   end,
	}

	if !tr.Start.Before(tr.End) {
		t.Error("Start should be before End")
	}
}

func TestWarmFile(t *testing.T) {
	wf := WarmFile{
		Path: "/tmp/warm/test.db.gz",
		TimeRange: TimeRange{
			Start: time.Now().Add(-48 * time.Hour),
			End:   time.Now().Add(-24 * time.Hour),
		},
		Size:       1024 * 1024,
		Compressed: true,
		Tables:     []string{"metrics", "logs"},
		CreatedAt:  time.Now(),
	}

	if wf.Path == "" {
		t.Error("Path should not be empty")
	}

	if !wf.Compressed {
		t.Error("Expected file to be compressed")
	}

	if len(wf.Tables) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(wf.Tables))
	}
}

func TestColdArchive(t *testing.T) {
	ca := ColdArchive{
		Key: "archives/2024/01/01/metrics.db.gz",
		TimeRange: TimeRange{
			Start: time.Now().Add(-30 * 24 * time.Hour),
			End:   time.Now().Add(-14 * 24 * time.Hour),
		},
		Size:      10 * 1024 * 1024,
		Tables:    []string{"metrics"},
		CreatedAt: time.Now(),
	}

	if ca.Key == "" {
		t.Error("Key should not be empty")
	}

	if ca.Size == 0 {
		t.Error("Size should not be zero")
	}
}

func TestTieringManagerListWarmFiles(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	files := manager.ListWarmFiles()

	// Should be empty initially
	if len(files) != 0 {
		t.Errorf("Expected 0 warm files initially, got %d", len(files))
	}
}

func TestTieringManagerListColdArchives(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	archives := manager.ListColdArchives()

	// Should be empty initially
	if len(archives) != 0 {
		t.Errorf("Expected 0 cold archives initially, got %d", len(archives))
	}
}

func TestTieringManagerSetColdBackend(t *testing.T) {
	db, tmpDir := createTestDB(t)
	defer db.Close()

	config := TieringConfig{
		DataDir: tmpDir,
	}

	manager, err := NewTieringManager(config, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	defer manager.Stop()

	// Create a local backend
	backend, _ := NewLocalBackend(tmpDir+"/cold", "")

	// Set cold backend
	manager.SetColdBackend(backend)

	// Verify it's set
	if manager.coldBackend == nil {
		t.Error("Cold backend should be set")
	}
}
