package storage

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WAL (Write-Ahead Log) provides durability for writes before they are committed to SQLite.
// It records all write operations to segment files, enabling recovery after crashes.

// WALConfig configures the write-ahead log
type WALConfig struct {
	// Dir is the directory where WAL segments are stored
	Dir string
	// MaxSegmentSize is the maximum size of a single segment file (default 64MB)
	MaxSegmentSize int64
	// SyncInterval is how often to fsync WAL to disk (default 100ms)
	SyncInterval time.Duration
	// MaxSegments is the maximum number of segments to keep (default 10)
	MaxSegments int
	// CheckpointInterval is how often to checkpoint and clean old segments (default 1m)
	CheckpointInterval time.Duration
}

// DefaultWALConfig returns sensible defaults
func DefaultWALConfig() WALConfig {
	return WALConfig{
		Dir:                "/var/lib/dogwatch/wal",
		MaxSegmentSize:     64 * 1024 * 1024, // 64MB
		SyncInterval:       100 * time.Millisecond,
		MaxSegments:        10,
		CheckpointInterval: time.Minute,
	}
}

// WALOperation represents a write operation type
type WALOperation uint8

const (
	WALOpInsert WALOperation = iota + 1
	WALOpUpdate
	WALOpDelete
	WALOpBatch
)

// WALEntry represents a single entry in the WAL
type WALEntry struct {
	// Sequence is a monotonically increasing sequence number
	Sequence uint64
	// Operation type
	Operation WALOperation
	// Table name
	Table string
	// Data is the serialized operation data (JSON or binary)
	Data []byte
	// Timestamp when the entry was created
	Timestamp time.Time
	// Committed indicates if this entry has been applied to SQLite
	Committed bool
}

// WAL is a write-ahead log for durability
type WAL struct {
	config WALConfig

	mu             sync.Mutex
	currentSegment *os.File
	segmentSeq     int64
	entrySeq       uint64
	writer         *bufio.Writer

	// Pending entries waiting to be checkpointed
	pending     []WALEntry
	pendingLock sync.RWMutex

	// Checkpoint callback - called to apply entries to the actual store
	checkpointFn func(entries []WALEntry) error

	// Background workers
	syncTicker       *time.Ticker
	checkpointTicker *time.Ticker
	stopCh           chan struct{}
	wg               sync.WaitGroup

	// Stats
	stats WALStats
}

// WALStats tracks WAL statistics
type WALStats struct {
	EntriesWritten   uint64    `json:"entries_written"`
	BytesWritten     uint64    `json:"bytes_written"`
	Checkpoints      uint64    `json:"checkpoints"`
	SegmentsCreated  uint64    `json:"segments_created"`
	SegmentsCleaned  uint64    `json:"segments_cleaned"`
	LastCheckpoint   time.Time `json:"last_checkpoint"`
	LastSync         time.Time `json:"last_sync"`
	PendingEntries   int       `json:"pending_entries"`
	CurrentSegmentID int64     `json:"current_segment_id"`
}

// NewWAL creates a new write-ahead log
func NewWAL(config WALConfig) (*WAL, error) {
	if config.Dir == "" {
		config.Dir = DefaultWALConfig().Dir
	}
	if config.MaxSegmentSize == 0 {
		config.MaxSegmentSize = DefaultWALConfig().MaxSegmentSize
	}
	if config.SyncInterval == 0 {
		config.SyncInterval = DefaultWALConfig().SyncInterval
	}
	if config.MaxSegments == 0 {
		config.MaxSegments = DefaultWALConfig().MaxSegments
	}
	if config.CheckpointInterval == 0 {
		config.CheckpointInterval = DefaultWALConfig().CheckpointInterval
	}

	// Create WAL directory
	if err := os.MkdirAll(config.Dir, 0755); err != nil {
		return nil, fmt.Errorf("creating WAL directory: %w", err)
	}

	w := &WAL{
		config:  config,
		pending: make([]WALEntry, 0, 1000),
		stopCh:  make(chan struct{}),
	}

	// Recover any uncommitted entries from existing segments
	if err := w.recover(); err != nil {
		return nil, fmt.Errorf("recovering WAL: %w", err)
	}

	// Open or create the current segment
	if err := w.openNextSegment(); err != nil {
		return nil, fmt.Errorf("opening segment: %w", err)
	}

	return w, nil
}

// Start begins background sync and checkpoint workers
func (w *WAL) Start() {
	w.syncTicker = time.NewTicker(w.config.SyncInterval)
	w.checkpointTicker = time.NewTicker(w.config.CheckpointInterval)

	w.wg.Add(2)

	// Background sync worker
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.syncTicker.C:
				w.sync()
			case <-w.stopCh:
				return
			}
		}
	}()

	// Background checkpoint worker
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-w.checkpointTicker.C:
				w.checkpoint()
			case <-w.stopCh:
				return
			}
		}
	}()
}

// Stop gracefully shuts down the WAL
func (w *WAL) Stop() error {
	close(w.stopCh)

	if w.syncTicker != nil {
		w.syncTicker.Stop()
	}
	if w.checkpointTicker != nil {
		w.checkpointTicker.Stop()
	}

	w.wg.Wait()

	// Final sync and close
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		w.writer.Flush()
	}
	if w.currentSegment != nil {
		w.currentSegment.Sync()
		w.currentSegment.Close()
	}

	return nil
}

// SetCheckpointCallback sets the function called to apply entries to the store
func (w *WAL) SetCheckpointCallback(fn func(entries []WALEntry) error) {
	w.checkpointFn = fn
}

// Write adds an entry to the WAL
func (w *WAL) Write(op WALOperation, table string, data []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.entrySeq++
	entry := WALEntry{
		Sequence:  w.entrySeq,
		Operation: op,
		Table:     table,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Serialize and write entry
	if err := w.writeEntry(entry); err != nil {
		return 0, err
	}

	// Add to pending list
	w.pendingLock.Lock()
	w.pending = append(w.pending, entry)
	w.pendingLock.Unlock()

	w.stats.EntriesWritten++

	// Check if we need to rotate segment
	if w.currentSegment != nil {
		info, err := w.currentSegment.Stat()
		if err == nil && info.Size() >= w.config.MaxSegmentSize {
			if err := w.rotateSegment(); err != nil {
				return entry.Sequence, fmt.Errorf("rotating segment: %w", err)
			}
		}
	}

	return entry.Sequence, nil
}

// WriteBatch writes multiple entries atomically
func (w *WAL) WriteBatch(entries []WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range entries {
		w.entrySeq++
		entries[i].Sequence = w.entrySeq
		entries[i].Timestamp = time.Now()

		if err := w.writeEntry(entries[i]); err != nil {
			return err
		}

		w.stats.EntriesWritten++
	}

	// Add all to pending
	w.pendingLock.Lock()
	w.pending = append(w.pending, entries...)
	w.pendingLock.Unlock()

	return nil
}

// writeEntry serializes and writes a single entry to the current segment
func (w *WAL) writeEntry(entry WALEntry) error {
	if w.writer == nil {
		return fmt.Errorf("WAL not initialized")
	}

	// Entry format:
	// [4 bytes: entry length (not including this field)]
	// [8 bytes: sequence number]
	// [1 byte: operation]
	// [2 bytes: table name length]
	// [N bytes: table name]
	// [8 bytes: timestamp unix nano]
	// [4 bytes: data length]
	// [N bytes: data]
	// [4 bytes: CRC32 checksum]

	tableBytes := []byte(entry.Table)
	entryLen := 8 + 1 + 2 + len(tableBytes) + 8 + 4 + len(entry.Data) + 4

	// Write length
	if err := binary.Write(w.writer, binary.LittleEndian, uint32(entryLen)); err != nil {
		return err
	}

	// Calculate CRC as we write
	crc := crc32.NewIEEE()
	mw := io.MultiWriter(w.writer, crc)

	// Write sequence
	if err := binary.Write(mw, binary.LittleEndian, entry.Sequence); err != nil {
		return err
	}

	// Write operation
	if err := binary.Write(mw, binary.LittleEndian, entry.Operation); err != nil {
		return err
	}

	// Write table name
	if err := binary.Write(mw, binary.LittleEndian, uint16(len(tableBytes))); err != nil {
		return err
	}
	if _, err := mw.Write(tableBytes); err != nil {
		return err
	}

	// Write timestamp
	if err := binary.Write(mw, binary.LittleEndian, entry.Timestamp.UnixNano()); err != nil {
		return err
	}

	// Write data
	if err := binary.Write(mw, binary.LittleEndian, uint32(len(entry.Data))); err != nil {
		return err
	}
	if _, err := mw.Write(entry.Data); err != nil {
		return err
	}

	// Write checksum
	if err := binary.Write(w.writer, binary.LittleEndian, crc.Sum32()); err != nil {
		return err
	}

	w.stats.BytesWritten += uint64(4 + entryLen)

	return nil
}

// sync flushes the WAL buffer to disk
func (w *WAL) sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		if err := w.writer.Flush(); err != nil {
			return err
		}
	}
	if w.currentSegment != nil {
		if err := w.currentSegment.Sync(); err != nil {
			return err
		}
	}

	w.stats.LastSync = time.Now()
	return nil
}

// checkpoint applies pending entries and cleans old segments
func (w *WAL) checkpoint() error {
	w.pendingLock.Lock()
	if len(w.pending) == 0 {
		w.pendingLock.Unlock()
		return nil
	}

	// Copy pending entries
	entries := make([]WALEntry, len(w.pending))
	copy(entries, w.pending)
	w.pendingLock.Unlock()

	// Apply entries via callback
	if w.checkpointFn != nil {
		if err := w.checkpointFn(entries); err != nil {
			return fmt.Errorf("checkpoint callback: %w", err)
		}
	}

	// Mark all as committed and clear pending
	w.pendingLock.Lock()
	// Only clear entries that were checkpointed (based on sequence)
	lastSeq := entries[len(entries)-1].Sequence
	newPending := make([]WALEntry, 0)
	for _, e := range w.pending {
		if e.Sequence > lastSeq {
			newPending = append(newPending, e)
		}
	}
	w.pending = newPending
	w.pendingLock.Unlock()

	w.stats.Checkpoints++
	w.stats.LastCheckpoint = time.Now()

	// Clean old segments
	return w.cleanOldSegments()
}

// rotateSegment closes the current segment and opens a new one
func (w *WAL) rotateSegment() error {
	// Flush and close current segment
	if w.writer != nil {
		w.writer.Flush()
	}
	if w.currentSegment != nil {
		w.currentSegment.Sync()
		w.currentSegment.Close()
	}

	return w.openNextSegment()
}

// openNextSegment creates a new segment file
func (w *WAL) openNextSegment() error {
	w.segmentSeq++
	w.stats.SegmentsCreated++

	filename := filepath.Join(w.config.Dir, fmt.Sprintf("wal-%016d.log", w.segmentSeq))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	w.currentSegment = f
	w.writer = bufio.NewWriterSize(f, 256*1024) // 256KB buffer
	w.stats.CurrentSegmentID = w.segmentSeq

	return nil
}

// recover reads existing WAL segments and replays uncommitted entries
func (w *WAL) recover() error {
	segments, err := w.listSegments()
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		return nil
	}

	// Find the highest sequence number and recover entries
	for _, seg := range segments {
		entries, maxSeq, err := w.readSegment(seg)
		if err != nil {
			// Log but continue - corrupted segments shouldn't stop recovery
			continue
		}

		if maxSeq > w.entrySeq {
			w.entrySeq = maxSeq
		}

		// Add uncommitted entries to pending
		for _, e := range entries {
			if !e.Committed {
				w.pending = append(w.pending, e)
			}
		}

		// Extract segment sequence from filename
		base := filepath.Base(seg)
		if seqStr := strings.TrimPrefix(strings.TrimSuffix(base, ".log"), "wal-"); seqStr != "" {
			if seq, err := strconv.ParseInt(seqStr, 10, 64); err == nil && seq > w.segmentSeq {
				w.segmentSeq = seq
			}
		}
	}

	return nil
}

// readSegment reads all entries from a segment file
func (w *WAL) readSegment(path string) ([]WALEntry, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var entries []WALEntry
	var maxSeq uint64

	for {
		entry, err := w.readEntry(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Corrupted entry - stop reading this segment
			break
		}

		entries = append(entries, entry)
		if entry.Sequence > maxSeq {
			maxSeq = entry.Sequence
		}
	}

	return entries, maxSeq, nil
}

// readEntry reads a single entry from a reader
func (w *WAL) readEntry(r *bufio.Reader) (WALEntry, error) {
	var entry WALEntry

	// Read length
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return entry, err
	}

	// Read entry data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return entry, err
	}

	// Verify CRC
	expectedCRC := binary.LittleEndian.Uint32(data[length-4:])
	actualCRC := crc32.ChecksumIEEE(data[:length-4])
	if expectedCRC != actualCRC {
		return entry, fmt.Errorf("CRC mismatch: expected %x, got %x", expectedCRC, actualCRC)
	}

	// Parse entry
	pos := 0

	// Sequence
	entry.Sequence = binary.LittleEndian.Uint64(data[pos:])
	pos += 8

	// Operation
	entry.Operation = WALOperation(data[pos])
	pos++

	// Table name
	tableLen := binary.LittleEndian.Uint16(data[pos:])
	pos += 2
	entry.Table = string(data[pos : pos+int(tableLen)])
	pos += int(tableLen)

	// Timestamp
	tsNano := binary.LittleEndian.Uint64(data[pos:])
	entry.Timestamp = time.Unix(0, int64(tsNano))
	pos += 8

	// Data
	dataLen := binary.LittleEndian.Uint32(data[pos:])
	pos += 4
	entry.Data = make([]byte, dataLen)
	copy(entry.Data, data[pos:pos+int(dataLen)])

	return entry, nil
}

// listSegments returns all WAL segment files sorted by sequence
func (w *WAL) listSegments() ([]string, error) {
	entries, err := os.ReadDir(w.config.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var segments []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "wal-") && strings.HasSuffix(e.Name(), ".log") {
			segments = append(segments, filepath.Join(w.config.Dir, e.Name()))
		}
	}

	sort.Strings(segments)
	return segments, nil
}

// cleanOldSegments removes segments beyond MaxSegments
func (w *WAL) cleanOldSegments() error {
	segments, err := w.listSegments()
	if err != nil {
		return err
	}

	// Keep only the most recent MaxSegments
	if len(segments) <= w.config.MaxSegments {
		return nil
	}

	toDelete := segments[:len(segments)-w.config.MaxSegments]
	for _, seg := range toDelete {
		if err := os.Remove(seg); err != nil {
			continue // Best effort
		}
		w.stats.SegmentsCleaned++
	}

	return nil
}

// Stats returns current WAL statistics
func (w *WAL) Stats() WALStats {
	w.pendingLock.RLock()
	stats := w.stats
	stats.PendingEntries = len(w.pending)
	w.pendingLock.RUnlock()
	return stats
}

// PendingCount returns the number of uncommitted entries
func (w *WAL) PendingCount() int {
	w.pendingLock.RLock()
	defer w.pendingLock.RUnlock()
	return len(w.pending)
}

// ForceCheckpoint triggers an immediate checkpoint
func (w *WAL) ForceCheckpoint() error {
	return w.checkpoint()
}

// ForceSync triggers an immediate sync
func (w *WAL) ForceSync() error {
	return w.sync()
}
