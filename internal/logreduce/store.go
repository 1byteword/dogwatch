package logreduce

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists log patterns and provides analysis
type Store struct {
	db    *sql.DB
	miner *Miner
	mu    sync.RWMutex
}

// NewStore creates a new pattern store
func NewStore(dbPath string, config MinerConfig) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:    db,
		miner: NewMiner(config),
	}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	// Load existing patterns
	if err := store.loadPatterns(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) createTables() error {
	schema := `
	-- Patterns table
	CREATE TABLE IF NOT EXISTS log_patterns (
		id TEXT PRIMARY KEY,
		template TEXT NOT NULL,
		tokens TEXT NOT NULL,
		sample_message TEXT,
		count INTEGER DEFAULT 0,
		first_seen DATETIME,
		last_seen DATETIME,
		level TEXT,
		service TEXT,
		variable_count INTEGER DEFAULT 0,
		variability REAL DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_patterns_count ON log_patterns(count DESC);
	CREATE INDEX IF NOT EXISTS idx_patterns_level ON log_patterns(level);
	CREATE INDEX IF NOT EXISTS idx_patterns_service ON log_patterns(service);
	CREATE INDEX IF NOT EXISTS idx_patterns_last_seen ON log_patterns(last_seen DESC);

	-- Hourly pattern counts for trend analysis
	CREATE TABLE IF NOT EXISTS pattern_counts (
		pattern_id TEXT NOT NULL,
		hour DATETIME NOT NULL,
		count INTEGER DEFAULT 0,
		PRIMARY KEY (pattern_id, hour),
		FOREIGN KEY (pattern_id) REFERENCES log_patterns(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_pattern_counts_hour ON pattern_counts(hour);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) loadPatterns() error {
	rows, err := s.db.Query(`
		SELECT id, template, tokens, sample_message, count, first_seen, last_seen,
		       level, service, variable_count, variability
		FROM log_patterns
		ORDER BY count DESC
		LIMIT 10000
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var patterns []*Pattern
	for rows.Next() {
		var p Pattern
		var tokensJSON string
		var level, service sql.NullString

		err := rows.Scan(
			&p.ID, &p.Template, &tokensJSON, &p.SampleMessage,
			&p.Count, &p.FirstSeen, &p.LastSeen,
			&level, &service, &p.VariableCount, &p.Variability,
		)
		if err != nil {
			return err
		}

		json.Unmarshal([]byte(tokensJSON), &p.Tokens)
		if level.Valid {
			p.Level = level.String
		}
		if service.Valid {
			p.Service = service.String
		}

		patterns = append(patterns, &p)
	}

	s.miner.Import(patterns)
	return nil
}

// Close closes the store
func (s *Store) Close() error {
	// Save patterns before closing
	s.savePatterns()
	return s.db.Close()
}

// savePatterns persists all patterns to database
func (s *Store) savePatterns() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	patterns := s.miner.GetPatterns()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO log_patterns
		(id, template, tokens, sample_message, count, first_seen, last_seen,
		 level, service, variable_count, variability, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, p := range patterns {
		tokensJSON, _ := json.Marshal(p.Tokens)
		_, err = stmt.Exec(
			p.ID, p.Template, string(tokensJSON), p.SampleMessage,
			p.Count, p.FirstSeen, p.LastSeen,
			p.Level, p.Service, p.VariableCount, p.Variability, now,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Process processes a log entry
func (s *Store) Process(entry LogEntry) *Pattern {
	return s.miner.Process(entry)
}

// ProcessBatch processes multiple log entries
func (s *Store) ProcessBatch(entries []LogEntry) *ReduceResult {
	result := s.miner.ProcessBatch(entries)

	// Periodically save to database
	if s.miner.totalProcessed%1000 == 0 {
		go s.savePatterns()
	}

	return result
}

// GetPatterns returns all patterns
func (s *Store) GetPatterns() []*Pattern {
	return s.miner.GetPatterns()
}

// GetPattern returns a specific pattern
func (s *Store) GetPattern(id string) *Pattern {
	return s.miner.GetPattern(id)
}

// GetStats returns pattern statistics
func (s *Store) GetStats() *PatternStats {
	s.miner.UpdateTrends(s.getHourlyCount)
	return s.miner.GetStats()
}

func (s *Store) getHourlyCount(patternID string) int64 {
	hourAgo := time.Now().Add(-time.Hour).Truncate(time.Hour)

	var count int64
	s.db.QueryRow(`
		SELECT COALESCE(SUM(count), 0) FROM pattern_counts
		WHERE pattern_id = ? AND hour >= ?
	`, patternID, hourAgo).Scan(&count)

	return count
}

// RecordHourlyCounts records current hour's counts
func (s *Store) RecordHourlyCounts() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hour := time.Now().Truncate(time.Hour)
	patterns := s.miner.GetPatterns()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO pattern_counts (pattern_id, hour, count)
		VALUES (?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range patterns {
		stmt.Exec(p.ID, hour, p.CountLastHour)
	}

	// Clean up old counts (keep 7 days)
	cutoff := time.Now().AddDate(0, 0, -7)
	tx.Exec(`DELETE FROM pattern_counts WHERE hour < ?`, cutoff)

	return tx.Commit()
}

// Match finds the pattern that matches a message
func (s *Store) Match(message string) (*PatternMatch, bool) {
	return s.miner.Match(message)
}

// Search finds patterns matching a query
func (s *Store) Search(query string) []*Pattern {
	return s.miner.Search(query)
}

// GetTopPatterns returns top N patterns by count
func (s *Store) GetTopPatterns(limit int) []*Pattern {
	patterns := s.miner.GetPatterns()
	return GetTopPatterns(patterns, limit)
}

// GetNewPatterns returns patterns first seen within duration
func (s *Store) GetNewPatterns(since time.Duration) []*Pattern {
	patterns := s.miner.GetPatterns()
	return GetNewPatterns(patterns, since)
}

// GetTrendingPatterns returns patterns with increasing counts
func (s *Store) GetTrendingPatterns() []*Pattern {
	s.miner.UpdateTrends(s.getHourlyCount)
	patterns := s.miner.GetPatterns()
	return GetTrendingPatterns(patterns)
}

// GetPatternsByLevel returns patterns for a specific level
func (s *Store) GetPatternsByLevel(level string) []*Pattern {
	patterns := s.miner.GetPatterns()
	return FilterPatternsByLevel(patterns, level)
}

// GetPatternsByService returns patterns for a specific service
func (s *Store) GetPatternsByService(service string) []*Pattern {
	patterns := s.miner.GetPatterns()
	return FilterPatternsByService(patterns, service)
}

// Reduce performs log reduction on a set of messages
func (s *Store) Reduce(messages []string, level, service string) *ReduceResult {
	entries := make([]LogEntry, len(messages))
	for i, msg := range messages {
		entries[i] = LogEntry{
			Timestamp: time.Now(),
			Level:     level,
			Service:   service,
			Message:   msg,
		}
	}
	return s.ProcessBatch(entries)
}

// GetPatternHistory returns hourly counts for a pattern
func (s *Store) GetPatternHistory(patternID string, hours int) ([]PatternHourlyCount, error) {
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)

	rows, err := s.db.Query(`
		SELECT hour, count FROM pattern_counts
		WHERE pattern_id = ? AND hour >= ?
		ORDER BY hour
	`, patternID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []PatternHourlyCount
	for rows.Next() {
		var h PatternHourlyCount
		if err := rows.Scan(&h.Hour, &h.Count); err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	return history, nil
}

// PatternHourlyCount represents hourly count for a pattern
type PatternHourlyCount struct {
	Hour  time.Time `json:"hour"`
	Count int64     `json:"count"`
}

// GetServicePatternSummary returns pattern summary by service
func (s *Store) GetServicePatternSummary() map[string]*ServicePatternSummary {
	patterns := s.miner.GetPatterns()
	summaries := make(map[string]*ServicePatternSummary)

	for _, p := range patterns {
		svc := p.Service
		if svc == "" {
			svc = "unknown"
		}

		if _, ok := summaries[svc]; !ok {
			summaries[svc] = &ServicePatternSummary{
				Service:       svc,
				ByLevel:       make(map[string]int),
			}
		}

		summary := summaries[svc]
		summary.PatternCount++
		summary.TotalLogs += p.Count
		if p.Level != "" {
			summary.ByLevel[p.Level]++
		}
	}

	return summaries
}

// ServicePatternSummary summarizes patterns for a service
type ServicePatternSummary struct {
	Service      string         `json:"service"`
	PatternCount int            `json:"pattern_count"`
	TotalLogs    int64          `json:"total_logs"`
	ByLevel      map[string]int `json:"by_level"`
}
