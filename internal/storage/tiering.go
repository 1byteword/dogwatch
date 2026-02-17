package storage

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// StorageTier represents a data storage tier
type StorageTier string

const (
	// TierHot is for recent, frequently accessed data (in active SQLite)
	TierHot StorageTier = "hot"
	// TierWarm is for older data that may still be queried (compressed SQLite)
	TierWarm StorageTier = "warm"
	// TierCold is for archived data (in object storage)
	TierCold StorageTier = "cold"
)

// TieringConfig configures storage tiering
type TieringConfig struct {
	// DataDir is the base directory for data storage
	DataDir string
	// HotRetention is how long data stays in hot tier (default 24h)
	HotRetention time.Duration
	// WarmRetention is how long data stays in warm tier before cold (default 7d)
	WarmRetention time.Duration
	// ColdRetention is how long to keep data in cold storage (default 90d)
	ColdRetention time.Duration
	// CompactionInterval is how often to run compaction (default 1h)
	CompactionInterval time.Duration
	// TieringInterval is how often to move data between tiers (default 6h)
	TieringInterval time.Duration
	// ColdBackend is the backend for cold storage
	ColdBackend StorageBackend
	// CompressWarm enables compression for warm tier
	CompressWarm bool
}

// DefaultTieringConfig returns sensible defaults
func DefaultTieringConfig() TieringConfig {
	return TieringConfig{
		DataDir:            "/var/lib/dogwatch",
		HotRetention:       24 * time.Hour,
		WarmRetention:      7 * 24 * time.Hour,
		ColdRetention:      90 * 24 * time.Hour,
		CompactionInterval: time.Hour,
		TieringInterval:    6 * time.Hour,
		CompressWarm:       true,
	}
}

// TieringManager manages hot/warm/cold storage tiers
type TieringManager struct {
	config TieringConfig

	// Hot tier is the active SQLite database
	hotDB *sql.DB

	// Warm tier stores compressed SQLite files by time range
	warmDir string

	// Cold tier uses the configured backend
	coldBackend StorageBackend

	// Track tiering state
	tieringState *TieringState
	stateMu      sync.RWMutex

	// Background workers
	compactionTicker *time.Ticker
	tieringTicker    *time.Ticker
	stopCh           chan struct{}
	wg               sync.WaitGroup

	// Stats
	stats TieringStats
}

// TieringState tracks what data is in which tier
type TieringState struct {
	// HotRanges are time ranges currently in hot tier
	HotRanges []TimeRange `json:"hot_ranges"`
	// WarmFiles are compressed SQLite files in warm tier
	WarmFiles []WarmFile `json:"warm_files"`
	// ColdArchives are archives in cold storage
	ColdArchives []ColdArchive `json:"cold_archives"`
	// LastCompaction is when compaction last ran
	LastCompaction time.Time `json:"last_compaction"`
	// LastTiering is when tiering last ran
	LastTiering time.Time `json:"last_tiering"`
}

// TimeRange represents a time range
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// WarmFile represents a compressed SQLite file in warm tier
type WarmFile struct {
	Path       string    `json:"path"`
	TimeRange  TimeRange `json:"time_range"`
	Size       int64     `json:"size"`
	Compressed bool      `json:"compressed"`
	Tables     []string  `json:"tables"`
	CreatedAt  time.Time `json:"created_at"`
}

// ColdArchive represents an archive in cold storage
type ColdArchive struct {
	Key       string    `json:"key"`
	TimeRange TimeRange `json:"time_range"`
	Size      int64     `json:"size"`
	Tables    []string  `json:"tables"`
	CreatedAt time.Time `json:"created_at"`
}

// TieringStats tracks tiering statistics
type TieringStats struct {
	HotDataSize       int64     `json:"hot_data_size"`
	WarmDataSize      int64     `json:"warm_data_size"`
	ColdDataSize      int64     `json:"cold_data_size"`
	TotalCompactions  int64     `json:"total_compactions"`
	TotalTierings     int64     `json:"total_tierings"`
	BytesCompacted    int64     `json:"bytes_compacted"`
	BytesTieredToWarm int64     `json:"bytes_tiered_to_warm"`
	BytesTieredToCold int64     `json:"bytes_tiered_to_cold"`
	LastError         string    `json:"last_error,omitempty"`
	LastErrorTime     time.Time `json:"last_error_time,omitempty"`
}

// NewTieringManager creates a new tiering manager
func NewTieringManager(config TieringConfig, hotDB *sql.DB) (*TieringManager, error) {
	if config.DataDir == "" {
		config.DataDir = DefaultTieringConfig().DataDir
	}
	if config.HotRetention == 0 {
		config.HotRetention = DefaultTieringConfig().HotRetention
	}
	if config.WarmRetention == 0 {
		config.WarmRetention = DefaultTieringConfig().WarmRetention
	}
	if config.ColdRetention == 0 {
		config.ColdRetention = DefaultTieringConfig().ColdRetention
	}
	if config.CompactionInterval == 0 {
		config.CompactionInterval = DefaultTieringConfig().CompactionInterval
	}
	if config.TieringInterval == 0 {
		config.TieringInterval = DefaultTieringConfig().TieringInterval
	}

	warmDir := filepath.Join(config.DataDir, "warm")
	if err := os.MkdirAll(warmDir, 0755); err != nil {
		return nil, fmt.Errorf("creating warm directory: %w", err)
	}

	m := &TieringManager{
		config:       config,
		hotDB:        hotDB,
		warmDir:      warmDir,
		coldBackend:  config.ColdBackend,
		tieringState: &TieringState{},
		stopCh:       make(chan struct{}),
	}

	// Load existing state
	if err := m.loadState(); err != nil {
		// Not fatal - just start fresh
	}

	return m, nil
}

// Start begins background compaction and tiering workers
func (m *TieringManager) Start() {
	m.compactionTicker = time.NewTicker(m.config.CompactionInterval)
	m.tieringTicker = time.NewTicker(m.config.TieringInterval)

	m.wg.Add(2)

	// Compaction worker
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-m.compactionTicker.C:
				if err := m.Compact(); err != nil {
					m.recordError(err)
				}
			case <-m.stopCh:
				return
			}
		}
	}()

	// Tiering worker
	go func() {
		defer m.wg.Done()
		for {
			select {
			case <-m.tieringTicker.C:
				if err := m.TierData(); err != nil {
					m.recordError(err)
				}
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop gracefully shuts down the tiering manager
func (m *TieringManager) Stop() {
	close(m.stopCh)

	if m.compactionTicker != nil {
		m.compactionTicker.Stop()
	}
	if m.tieringTicker != nil {
		m.tieringTicker.Stop()
	}

	m.wg.Wait()
	m.saveState()
}

// Compact runs compaction on hot tier data
func (m *TieringManager) Compact() error {
	m.stats.TotalCompactions++

	// For SQLite, compaction is VACUUM
	_, err := m.hotDB.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("vacuuming database: %w", err)
	}

	m.stateMu.Lock()
	m.tieringState.LastCompaction = time.Now()
	m.stateMu.Unlock()

	return m.saveState()
}

// TierData moves old data from hot to warm, and warm to cold
func (m *TieringManager) TierData() error {
	m.stats.TotalTierings++

	// Move hot to warm
	if err := m.tierHotToWarm(); err != nil {
		return fmt.Errorf("tiering hot to warm: %w", err)
	}

	// Move warm to cold
	if err := m.tierWarmToCold(); err != nil {
		return fmt.Errorf("tiering warm to cold: %w", err)
	}

	// Clean expired cold data
	if err := m.cleanExpiredCold(); err != nil {
		return fmt.Errorf("cleaning expired cold: %w", err)
	}

	m.stateMu.Lock()
	m.tieringState.LastTiering = time.Now()
	m.stateMu.Unlock()

	return m.saveState()
}

// tierHotToWarm moves old hot data to warm tier
func (m *TieringManager) tierHotToWarm() error {
	cutoff := time.Now().Add(-m.config.HotRetention)

	// Get tables that support tiering
	tables := m.getTierableTables()

	for _, table := range tables {
		// Check if table has timestamp column
		hasTimestamp, tsCol := m.hasTimestampColumn(table)
		if !hasTimestamp {
			continue
		}

		// Count rows to tier
		var count int64
		row := m.hotDB.QueryRow(fmt.Sprintf(
			"SELECT COUNT(*) FROM %s WHERE %s < ?",
			table, tsCol), cutoff.Format(time.RFC3339))
		if err := row.Scan(&count); err != nil || count == 0 {
			continue
		}

		// Export to warm tier
		warmFile, err := m.exportToWarm(table, tsCol, cutoff)
		if err != nil {
			return fmt.Errorf("exporting %s to warm: %w", table, err)
		}

		// Delete from hot after successful export
		_, err = m.hotDB.Exec(fmt.Sprintf(
			"DELETE FROM %s WHERE %s < ?",
			table, tsCol), cutoff.Format(time.RFC3339))
		if err != nil {
			return fmt.Errorf("deleting from hot: %w", err)
		}

		// Update state
		m.stateMu.Lock()
		m.tieringState.WarmFiles = append(m.tieringState.WarmFiles, warmFile)
		m.stats.BytesTieredToWarm += warmFile.Size
		m.stateMu.Unlock()
	}

	return nil
}

// tierWarmToCold moves old warm data to cold tier
func (m *TieringManager) tierWarmToCold() error {
	if m.coldBackend == nil {
		return nil // No cold backend configured
	}

	cutoff := time.Now().Add(-m.config.WarmRetention)

	m.stateMu.Lock()
	var remaining []WarmFile
	var toArchive []WarmFile

	for _, wf := range m.tieringState.WarmFiles {
		if wf.TimeRange.End.Before(cutoff) {
			toArchive = append(toArchive, wf)
		} else {
			remaining = append(remaining, wf)
		}
	}
	m.tieringState.WarmFiles = remaining
	m.stateMu.Unlock()

	for _, wf := range toArchive {
		archive, err := m.archiveToCold(wf)
		if err != nil {
			// Re-add to warm on failure
			m.stateMu.Lock()
			m.tieringState.WarmFiles = append(m.tieringState.WarmFiles, wf)
			m.stateMu.Unlock()
			return fmt.Errorf("archiving to cold: %w", err)
		}

		// Remove warm file
		os.Remove(wf.Path)

		// Update state
		m.stateMu.Lock()
		m.tieringState.ColdArchives = append(m.tieringState.ColdArchives, archive)
		m.stats.BytesTieredToCold += archive.Size
		m.stateMu.Unlock()
	}

	return nil
}

// cleanExpiredCold removes cold archives past retention
func (m *TieringManager) cleanExpiredCold() error {
	if m.coldBackend == nil {
		return nil
	}

	cutoff := time.Now().Add(-m.config.ColdRetention)

	m.stateMu.Lock()
	var remaining []ColdArchive
	var toDelete []ColdArchive

	for _, ca := range m.tieringState.ColdArchives {
		if ca.TimeRange.End.Before(cutoff) {
			toDelete = append(toDelete, ca)
		} else {
			remaining = append(remaining, ca)
		}
	}
	m.tieringState.ColdArchives = remaining
	m.stateMu.Unlock()

	ctx := context.Background()
	for _, ca := range toDelete {
		if err := m.coldBackend.Delete(ctx, ca.Key); err != nil {
			// Re-add on failure
			m.stateMu.Lock()
			m.tieringState.ColdArchives = append(m.tieringState.ColdArchives, ca)
			m.stateMu.Unlock()
			continue
		}
	}

	return nil
}

// exportToWarm exports table data to a warm tier file
func (m *TieringManager) exportToWarm(table, tsCol string, before time.Time) (WarmFile, error) {
	// Generate warm file path
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.db", table, timestamp)
	if m.config.CompressWarm {
		filename += ".gz"
	}
	path := filepath.Join(m.warmDir, filename)

	// Create new SQLite database for warm storage
	warmDBPath := path
	if m.config.CompressWarm {
		warmDBPath = strings.TrimSuffix(path, ".gz")
	}

	warmDB, err := OpenDB(warmDBPath)
	if err != nil {
		return WarmFile{}, err
	}
	defer warmDB.Close()

	// Get table schema
	var schema string
	row := m.hotDB.QueryRow(fmt.Sprintf(
		"SELECT sql FROM sqlite_master WHERE type='table' AND name='%s'", table))
	if err := row.Scan(&schema); err != nil {
		return WarmFile{}, fmt.Errorf("getting schema: %w", err)
	}

	// Create table in warm DB
	if _, err := warmDB.Exec(schema); err != nil {
		return WarmFile{}, fmt.Errorf("creating table: %w", err)
	}

	// Copy data
	rows, err := m.hotDB.Query(fmt.Sprintf(
		"SELECT * FROM %s WHERE %s < ?",
		table, tsCol), before.Format(time.RFC3339))
	if err != nil {
		return WarmFile{}, fmt.Errorf("querying data: %w", err)
	}
	defer rows.Close()

	// Get column names
	cols, _ := rows.Columns()
	colPlaceholders := strings.Repeat("?,", len(cols))
	colPlaceholders = colPlaceholders[:len(colPlaceholders)-1]

	insertStmt, err := warmDB.Prepare(fmt.Sprintf(
		"INSERT INTO %s VALUES (%s)", table, colPlaceholders))
	if err != nil {
		return WarmFile{}, fmt.Errorf("preparing insert: %w", err)
	}
	defer insertStmt.Close()

	var minTime, maxTime time.Time
	first := true

	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Track time range
		for i, col := range cols {
			if col == tsCol {
				if ts, ok := values[i].(string); ok {
					if t, err := time.Parse(time.RFC3339, ts); err == nil {
						if first || t.Before(minTime) {
							minTime = t
						}
						if first || t.After(maxTime) {
							maxTime = t
						}
						first = false
					}
				}
			}
		}

		if _, err := insertStmt.Exec(values...); err != nil {
			continue
		}
	}

	warmDB.Close()

	// Compress if configured
	var finalSize int64
	if m.config.CompressWarm {
		if err := compressFile(warmDBPath, path); err != nil {
			os.Remove(warmDBPath)
			return WarmFile{}, fmt.Errorf("compressing: %w", err)
		}
		os.Remove(warmDBPath)
		info, _ := os.Stat(path)
		finalSize = info.Size()
	} else {
		info, _ := os.Stat(path)
		finalSize = info.Size()
	}

	return WarmFile{
		Path: path,
		TimeRange: TimeRange{
			Start: minTime,
			End:   maxTime,
		},
		Size:       finalSize,
		Compressed: m.config.CompressWarm,
		Tables:     []string{table},
		CreatedAt:  time.Now(),
	}, nil
}

// archiveToCold uploads a warm file to cold storage
func (m *TieringManager) archiveToCold(wf WarmFile) (ColdArchive, error) {
	ctx := context.Background()

	// Generate cold key
	key := fmt.Sprintf("archives/%s/%s",
		wf.TimeRange.Start.Format("2006/01/02"),
		filepath.Base(wf.Path))

	// Open warm file
	f, err := os.Open(wf.Path)
	if err != nil {
		return ColdArchive{}, err
	}
	defer f.Close()

	info, _ := f.Stat()

	// Upload to cold backend
	if err := m.coldBackend.Put(ctx, key, f, info.Size()); err != nil {
		return ColdArchive{}, err
	}

	return ColdArchive{
		Key:       key,
		TimeRange: wf.TimeRange,
		Size:      info.Size(),
		Tables:    wf.Tables,
		CreatedAt: time.Now(),
	}, nil
}

// QueryWarm queries data from warm tier
func (m *TieringManager) QueryWarm(ctx context.Context, table string, start, end time.Time) (*sql.Rows, error) {
	m.stateMu.RLock()
	var relevantFiles []WarmFile
	for _, wf := range m.tieringState.WarmFiles {
		// Check if time range overlaps
		if wf.TimeRange.Start.Before(end) && wf.TimeRange.End.After(start) {
			for _, t := range wf.Tables {
				if t == table {
					relevantFiles = append(relevantFiles, wf)
					break
				}
			}
		}
	}
	m.stateMu.RUnlock()

	if len(relevantFiles) == 0 {
		return nil, fmt.Errorf("no warm data found for range")
	}

	// Query from first matching file (for simplicity)
	// In production, would merge results from multiple files
	wf := relevantFiles[0]

	dbPath := wf.Path
	if wf.Compressed {
		// Decompress to temp file
		tmpPath, err := decompressToTemp(wf.Path)
		if err != nil {
			return nil, err
		}
		dbPath = tmpPath
		// Note: caller responsible for cleanup
	}

	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	return db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", table))
}

// RestoreFromCold restores data from cold storage to warm tier
func (m *TieringManager) RestoreFromCold(ctx context.Context, key string) error {
	if m.coldBackend == nil {
		return fmt.Errorf("no cold backend configured")
	}

	// Download from cold
	reader, err := m.coldBackend.Get(ctx, key)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Write to warm directory
	filename := filepath.Base(key)
	path := filepath.Join(m.warmDir, filename)

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	size, err := io.Copy(f, reader)
	f.Close()
	if err != nil {
		os.Remove(path)
		return err
	}

	// Find archive info
	m.stateMu.Lock()
	var archive ColdArchive
	var remaining []ColdArchive
	for _, ca := range m.tieringState.ColdArchives {
		if ca.Key == key {
			archive = ca
		} else {
			remaining = append(remaining, ca)
		}
	}

	if archive.Key != "" {
		// Add to warm files
		m.tieringState.WarmFiles = append(m.tieringState.WarmFiles, WarmFile{
			Path:       path,
			TimeRange:  archive.TimeRange,
			Size:       size,
			Compressed: strings.HasSuffix(path, ".gz"),
			Tables:     archive.Tables,
			CreatedAt:  time.Now(),
		})
		// Remove from cold archives
		m.tieringState.ColdArchives = remaining
	}
	m.stateMu.Unlock()

	return m.saveState()
}

// Stats returns current tiering statistics
func (m *TieringManager) Stats() TieringStats {
	m.updateSizeStats()
	return m.stats
}

// State returns current tiering state
func (m *TieringManager) State() TieringState {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return *m.tieringState
}

// updateSizeStats updates size statistics
func (m *TieringManager) updateSizeStats() {
	// Hot tier size
	var hotSize int64
	row := m.hotDB.QueryRow("SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()")
	row.Scan(&hotSize)
	m.stats.HotDataSize = hotSize

	// Warm tier size
	m.stateMu.RLock()
	var warmSize int64
	for _, wf := range m.tieringState.WarmFiles {
		warmSize += wf.Size
	}
	var coldSize int64
	for _, ca := range m.tieringState.ColdArchives {
		coldSize += ca.Size
	}
	m.stateMu.RUnlock()

	m.stats.WarmDataSize = warmSize
	m.stats.ColdDataSize = coldSize
}

// getTierableTables returns tables that support tiering
func (m *TieringManager) getTierableTables() []string {
	// Tables with timestamp columns that can be tiered
	return []string{
		"system_metrics",
		"endpoint_metrics",
		"connection_metrics",
		"logs",
		"spans",
		"traces",
	}
}

// hasTimestampColumn checks if a table has a timestamp column
func (m *TieringManager) hasTimestampColumn(table string) (bool, string) {
	// Check common timestamp column names
	tsColumns := []string{"timestamp", "start_time", "created_at", "time"}

	rows, err := m.hotDB.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, ""
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			continue
		}

		for _, tsCol := range tsColumns {
			if strings.EqualFold(name, tsCol) {
				return true, name
			}
		}
	}

	return false, ""
}

// loadState loads tiering state from disk
func (m *TieringManager) loadState() error {
	path := filepath.Join(m.config.DataDir, "tiering_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return json.Unmarshal(data, m.tieringState)
}

// saveState saves tiering state to disk
func (m *TieringManager) saveState() error {
	m.stateMu.RLock()
	data, err := json.MarshalIndent(m.tieringState, "", "  ")
	m.stateMu.RUnlock()
	if err != nil {
		return err
	}

	path := filepath.Join(m.config.DataDir, "tiering_state.json")
	return os.WriteFile(path, data, 0644)
}

// recordError records an error in stats
func (m *TieringManager) recordError(err error) {
	m.stats.LastError = err.Error()
	m.stats.LastErrorTime = time.Now()
}

// Helper functions

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gw := gzip.NewWriter(out)
	defer gw.Close()

	_, err = io.Copy(gw, in)
	return err
}

func decompressToTemp(src string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	gr, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gr.Close()

	tmpFile, err := os.CreateTemp("", "dogwatch-warm-*.db")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, gr); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// ListWarmFiles returns all warm tier files
func (m *TieringManager) ListWarmFiles() []WarmFile {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	files := make([]WarmFile, len(m.tieringState.WarmFiles))
	copy(files, m.tieringState.WarmFiles)
	sort.Slice(files, func(i, j int) bool {
		return files[i].TimeRange.Start.Before(files[j].TimeRange.Start)
	})
	return files
}

// ListColdArchives returns all cold tier archives
func (m *TieringManager) ListColdArchives() []ColdArchive {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()

	archives := make([]ColdArchive, len(m.tieringState.ColdArchives))
	copy(archives, m.tieringState.ColdArchives)
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].TimeRange.Start.Before(archives[j].TimeRange.Start)
	})
	return archives
}

// SetColdBackend updates the cold storage backend
func (m *TieringManager) SetColdBackend(backend StorageBackend) {
	m.coldBackend = backend
}

// ForceCompact triggers immediate compaction
func (m *TieringManager) ForceCompact() error {
	return m.Compact()
}

// ForceTier triggers immediate tiering
func (m *TieringManager) ForceTier() error {
	return m.TierData()
}

// GetTierForTime returns which tier contains data for a given time
func (m *TieringManager) GetTierForTime(t time.Time) StorageTier {
	now := time.Now()

	if t.After(now.Add(-m.config.HotRetention)) {
		return TierHot
	}

	if t.After(now.Add(-m.config.WarmRetention)) {
		// Check if we have warm data for this time
		m.stateMu.RLock()
		for _, wf := range m.tieringState.WarmFiles {
			if t.After(wf.TimeRange.Start) && t.Before(wf.TimeRange.End) {
				m.stateMu.RUnlock()
				return TierWarm
			}
		}
		m.stateMu.RUnlock()
	}

	// Check cold
	m.stateMu.RLock()
	for _, ca := range m.tieringState.ColdArchives {
		if t.After(ca.TimeRange.Start) && t.Before(ca.TimeRange.End) {
			m.stateMu.RUnlock()
			return TierCold
		}
	}
	m.stateMu.RUnlock()

	return TierHot // Default to hot if not found elsewhere
}
