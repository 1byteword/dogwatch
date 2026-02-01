// Package audit provides async audit logging functionality
package audit

import (
	"context"
	"log"
	"sync"
	"time"
)

// LogLevel represents the audit log level
type LogLevel int

const (
	LogLevelQuery  LogLevel = iota // Query execution events
	LogLevelAuth                   // Authentication events
	LogLevelAdmin                  // Admin action events
	LogLevelSystem                 // System events
	LogLevelAll                    // All events
)

// LoggerConfig configures the async audit logger
type LoggerConfig struct {
	// BufferSize is the size of the async write buffer
	BufferSize int

	// FlushInterval is how often to flush buffered entries
	FlushInterval time.Duration

	// MaxBatchSize is the maximum entries to write in a single batch
	MaxBatchSize int

	// EnabledLevels controls which log levels are enabled
	EnabledLevels []LogLevel

	// DropOnFull determines behavior when buffer is full
	// If true, new entries are dropped; if false, block until space available
	DropOnFull bool
}

// DefaultLoggerConfig returns sensible defaults
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		BufferSize:    10000,
		FlushInterval: 5 * time.Second,
		MaxBatchSize:  100,
		EnabledLevels: []LogLevel{LogLevelQuery, LogLevelAuth, LogLevelAdmin, LogLevelSystem},
		DropOnFull:    true, // Don't block app if audit is slow
	}
}

// Logger provides async audit logging to avoid performance impact
type Logger struct {
	store   *Store
	config  LoggerConfig

	// Channels for different entry types
	queryEntries  chan *QueryAuditEntry
	authEntries   chan *AuthAuditEntry
	adminEntries  chan *AdminAuditEntry
	exportEntries chan *DataExportEntry

	// Control
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	started    bool
	mu         sync.Mutex

	// Stats
	stats      LoggerStats
	statsMu    sync.RWMutex
}

// LoggerStats tracks logger performance
type LoggerStats struct {
	QueriesLogged     int64 `json:"queries_logged"`
	AuthEventsLogged  int64 `json:"auth_events_logged"`
	AdminActionsLogged int64 `json:"admin_actions_logged"`
	ExportsLogged     int64 `json:"exports_logged"`
	DroppedEntries    int64 `json:"dropped_entries"`
	FlushCount        int64 `json:"flush_count"`
	LastFlush         time.Time `json:"last_flush"`
	AvgFlushDurationMs float64 `json:"avg_flush_duration_ms"`
}

// NewLogger creates a new async audit logger
func NewLogger(store *Store, config LoggerConfig) *Logger {
	ctx, cancel := context.WithCancel(context.Background())

	return &Logger{
		store:         store,
		config:        config,
		queryEntries:  make(chan *QueryAuditEntry, config.BufferSize),
		authEntries:   make(chan *AuthAuditEntry, config.BufferSize),
		adminEntries:  make(chan *AdminAuditEntry, config.BufferSize),
		exportEntries: make(chan *DataExportEntry, config.BufferSize),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins the async logging goroutines
func (l *Logger) Start() {
	l.mu.Lock()
	if l.started {
		l.mu.Unlock()
		return
	}
	l.started = true
	l.mu.Unlock()

	// Start flush workers
	l.wg.Add(4)
	go l.queryFlushWorker()
	go l.authFlushWorker()
	go l.adminFlushWorker()
	go l.exportFlushWorker()

	log.Printf("[audit] Async logger started (buffer: %d, flush interval: %v)",
		l.config.BufferSize, l.config.FlushInterval)
}

// Stop gracefully stops the logger, flushing remaining entries
func (l *Logger) Stop() {
	l.mu.Lock()
	if !l.started {
		l.mu.Unlock()
		return
	}
	l.started = false
	l.mu.Unlock()

	l.cancel()
	l.wg.Wait()

	// Final flush of any remaining entries
	l.flushAllSync()

	log.Printf("[audit] Async logger stopped")
}

// LogQuery logs a query audit entry asynchronously
func (l *Logger) LogQuery(entry *QueryAuditEntry) {
	if !l.isLevelEnabled(LogLevelQuery) {
		return
	}

	if l.config.DropOnFull {
		select {
		case l.queryEntries <- entry:
			// Sent successfully
		default:
			l.incrementDropped()
		}
	} else {
		l.queryEntries <- entry
	}
}

// LogAuth logs an auth audit entry asynchronously
func (l *Logger) LogAuth(entry *AuthAuditEntry) {
	if !l.isLevelEnabled(LogLevelAuth) {
		return
	}

	if l.config.DropOnFull {
		select {
		case l.authEntries <- entry:
		default:
			l.incrementDropped()
		}
	} else {
		l.authEntries <- entry
	}
}

// LogAdmin logs an admin audit entry asynchronously
func (l *Logger) LogAdmin(entry *AdminAuditEntry) {
	if !l.isLevelEnabled(LogLevelAdmin) {
		return
	}

	if l.config.DropOnFull {
		select {
		case l.adminEntries <- entry:
		default:
			l.incrementDropped()
		}
	} else {
		l.adminEntries <- entry
	}
}

// LogExport logs a data export audit entry asynchronously
func (l *Logger) LogExport(entry *DataExportEntry) {
	if !l.isLevelEnabled(LogLevelAdmin) {
		return
	}

	if l.config.DropOnFull {
		select {
		case l.exportEntries <- entry:
		default:
			l.incrementDropped()
		}
	} else {
		l.exportEntries <- entry
	}
}

// GetStats returns logger statistics
func (l *Logger) GetStats() LoggerStats {
	l.statsMu.RLock()
	defer l.statsMu.RUnlock()
	return l.stats
}

// isLevelEnabled checks if a log level is enabled
func (l *Logger) isLevelEnabled(level LogLevel) bool {
	for _, enabled := range l.config.EnabledLevels {
		if enabled == LogLevelAll || enabled == level {
			return true
		}
	}
	return false
}

// incrementDropped safely increments the dropped counter
func (l *Logger) incrementDropped() {
	l.statsMu.Lock()
	l.stats.DroppedEntries++
	l.statsMu.Unlock()
}

// queryFlushWorker handles periodic flushing of query entries
func (l *Logger) queryFlushWorker() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	var batch []*QueryAuditEntry

	for {
		select {
		case <-l.ctx.Done():
			// Flush remaining on shutdown
			l.flushQueryBatch(batch)
			return

		case entry := <-l.queryEntries:
			batch = append(batch, entry)
			if len(batch) >= l.config.MaxBatchSize {
				l.flushQueryBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushQueryBatch(batch)
				batch = nil
			}
		}
	}
}

// authFlushWorker handles periodic flushing of auth entries
func (l *Logger) authFlushWorker() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	var batch []*AuthAuditEntry

	for {
		select {
		case <-l.ctx.Done():
			l.flushAuthBatch(batch)
			return

		case entry := <-l.authEntries:
			batch = append(batch, entry)
			if len(batch) >= l.config.MaxBatchSize {
				l.flushAuthBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushAuthBatch(batch)
				batch = nil
			}
		}
	}
}

// adminFlushWorker handles periodic flushing of admin entries
func (l *Logger) adminFlushWorker() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	var batch []*AdminAuditEntry

	for {
		select {
		case <-l.ctx.Done():
			l.flushAdminBatch(batch)
			return

		case entry := <-l.adminEntries:
			batch = append(batch, entry)
			if len(batch) >= l.config.MaxBatchSize {
				l.flushAdminBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushAdminBatch(batch)
				batch = nil
			}
		}
	}
}

// exportFlushWorker handles periodic flushing of export entries
func (l *Logger) exportFlushWorker() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.FlushInterval)
	defer ticker.Stop()

	var batch []*DataExportEntry

	for {
		select {
		case <-l.ctx.Done():
			l.flushExportBatch(batch)
			return

		case entry := <-l.exportEntries:
			batch = append(batch, entry)
			if len(batch) >= l.config.MaxBatchSize {
				l.flushExportBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushExportBatch(batch)
				batch = nil
			}
		}
	}
}

// flushQueryBatch writes query entries to storage
func (l *Logger) flushQueryBatch(batch []*QueryAuditEntry) {
	if len(batch) == 0 {
		return
	}

	start := time.Now()

	for _, entry := range batch {
		if err := l.store.LogQueryAudit(entry); err != nil {
			log.Printf("[audit] Failed to log query entry: %v", err)
		}
	}

	l.statsMu.Lock()
	l.stats.QueriesLogged += int64(len(batch))
	l.stats.FlushCount++
	l.stats.LastFlush = time.Now()
	// Update rolling average
	duration := time.Since(start).Seconds() * 1000
	l.stats.AvgFlushDurationMs = (l.stats.AvgFlushDurationMs*float64(l.stats.FlushCount-1) + duration) / float64(l.stats.FlushCount)
	l.statsMu.Unlock()
}

// flushAuthBatch writes auth entries to storage
func (l *Logger) flushAuthBatch(batch []*AuthAuditEntry) {
	if len(batch) == 0 {
		return
	}

	for _, entry := range batch {
		if err := l.store.LogAuthAudit(entry); err != nil {
			log.Printf("[audit] Failed to log auth entry: %v", err)
		}
	}

	l.statsMu.Lock()
	l.stats.AuthEventsLogged += int64(len(batch))
	l.statsMu.Unlock()
}

// flushAdminBatch writes admin entries to storage
func (l *Logger) flushAdminBatch(batch []*AdminAuditEntry) {
	if len(batch) == 0 {
		return
	}

	for _, entry := range batch {
		if err := l.store.LogAdminAudit(entry); err != nil {
			log.Printf("[audit] Failed to log admin entry: %v", err)
		}
	}

	l.statsMu.Lock()
	l.stats.AdminActionsLogged += int64(len(batch))
	l.statsMu.Unlock()
}

// flushExportBatch writes export entries to storage
func (l *Logger) flushExportBatch(batch []*DataExportEntry) {
	if len(batch) == 0 {
		return
	}

	for _, entry := range batch {
		if err := l.store.LogExportAudit(entry); err != nil {
			log.Printf("[audit] Failed to log export entry: %v", err)
		}
	}

	l.statsMu.Lock()
	l.stats.ExportsLogged += int64(len(batch))
	l.statsMu.Unlock()
}

// flushAllSync synchronously flushes all remaining entries
func (l *Logger) flushAllSync() {
	// Drain query entries
	for {
		select {
		case entry := <-l.queryEntries:
			if err := l.store.LogQueryAudit(entry); err != nil {
				log.Printf("[audit] Failed to log query entry on shutdown: %v", err)
			}
		default:
			goto authDrain
		}
	}

authDrain:
	for {
		select {
		case entry := <-l.authEntries:
			if err := l.store.LogAuthAudit(entry); err != nil {
				log.Printf("[audit] Failed to log auth entry on shutdown: %v", err)
			}
		default:
			goto adminDrain
		}
	}

adminDrain:
	for {
		select {
		case entry := <-l.adminEntries:
			if err := l.store.LogAdminAudit(entry); err != nil {
				log.Printf("[audit] Failed to log admin entry on shutdown: %v", err)
			}
		default:
			goto exportDrain
		}
	}

exportDrain:
	for {
		select {
		case entry := <-l.exportEntries:
			if err := l.store.LogExportAudit(entry); err != nil {
				log.Printf("[audit] Failed to log export entry on shutdown: %v", err)
			}
		default:
			return
		}
	}
}

// Flush forces an immediate flush of all buffered entries
func (l *Logger) Flush() {
	// Send flush signals by temporarily closing and recreating channels
	// This is a simplified approach - a more robust implementation would use
	// dedicated flush signals
	l.flushAllSync()
}
