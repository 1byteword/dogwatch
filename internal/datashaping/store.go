package datashaping

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"sync"
	"time"

)

// Store persists data shaping rules
type Store struct {
	db     *sql.DB
	engine *Engine
	mu     sync.RWMutex
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:     db,
		engine: NewEngine(),
	}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	// Load existing rules
	if err := store.loadRules(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS shaping_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			enabled INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 100,
			data_type TEXT NOT NULL,
			action TEXT NOT NULL,
			config TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_by TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_shaping_rules_enabled
			ON shaping_rules(enabled);
		CREATE INDEX IF NOT EXISTS idx_shaping_rules_priority
			ON shaping_rules(priority);
	`)
	return err
}

func (s *Store) loadRules() error {
	rows, err := s.db.Query(`
		SELECT id, name, description, enabled, priority, data_type, action, config, created_at, updated_at, created_by
		FROM shaping_rules
		ORDER BY priority ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var rule Rule
		var configJSON string
		var enabled int

		err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Description, &enabled,
			&rule.Priority, &rule.DataType, &rule.Action, &configJSON,
			&rule.CreatedAt, &rule.UpdatedAt, &rule.CreatedBy,
		)
		if err != nil {
			return err
		}

		rule.Enabled = enabled == 1

		// Parse config JSON
		if configJSON != "" {
			var config ruleConfig
			if err := json.Unmarshal([]byte(configJSON), &config); err == nil {
				rule.NamePattern = config.NamePattern
				rule.TagMatches = config.TagMatches
				rule.TagPatterns = config.TagPatterns
				rule.LevelMatch = config.LevelMatch
				rule.ServiceMatch = config.ServiceMatch
				rule.SampleRate = config.SampleRate
				rule.AggregateBy = config.AggregateBy
				rule.DropTags = config.DropTags
				rule.AddTags = config.AddTags
			}
		}

		s.engine.AddRule(&rule)
	}

	return nil
}

// ruleConfig holds the JSON-serialized rule configuration
type ruleConfig struct {
	NamePattern  string            `json:"name_pattern,omitempty"`
	TagMatches   map[string]string `json:"tag_matches,omitempty"`
	TagPatterns  map[string]string `json:"tag_patterns,omitempty"`
	LevelMatch   string            `json:"level_match,omitempty"`
	ServiceMatch string            `json:"service_match,omitempty"`
	SampleRate   float64           `json:"sample_rate,omitempty"`
	AggregateBy  []string          `json:"aggregate_by,omitempty"`
	DropTags     []string          `json:"drop_tags,omitempty"`
	AddTags      map[string]string `json:"add_tags,omitempty"`
}

// CreateRule creates a new rule
func (s *Store) CreateRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	rule.UpdatedAt = time.Now()

	config := ruleConfig{
		NamePattern:  rule.NamePattern,
		TagMatches:   rule.TagMatches,
		TagPatterns:  rule.TagPatterns,
		LevelMatch:   rule.LevelMatch,
		ServiceMatch: rule.ServiceMatch,
		SampleRate:   rule.SampleRate,
		AggregateBy:  rule.AggregateBy,
		DropTags:     rule.DropTags,
		AddTags:      rule.AddTags,
	}
	configJSON, _ := json.Marshal(config)

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO shaping_rules (id, name, description, enabled, priority, data_type, action, config, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rule.ID, rule.Name, rule.Description, enabled, rule.Priority, rule.DataType, rule.Action, string(configJSON), rule.CreatedAt, rule.UpdatedAt, rule.CreatedBy)

	if err != nil {
		return err
	}

	return s.engine.AddRule(rule)
}

// UpdateRule updates an existing rule
func (s *Store) UpdateRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rule.UpdatedAt = time.Now()

	config := ruleConfig{
		NamePattern:  rule.NamePattern,
		TagMatches:   rule.TagMatches,
		TagPatterns:  rule.TagPatterns,
		LevelMatch:   rule.LevelMatch,
		ServiceMatch: rule.ServiceMatch,
		SampleRate:   rule.SampleRate,
		AggregateBy:  rule.AggregateBy,
		DropTags:     rule.DropTags,
		AddTags:      rule.AddTags,
	}
	configJSON, _ := json.Marshal(config)

	enabled := 0
	if rule.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(`
		UPDATE shaping_rules
		SET name = ?, description = ?, enabled = ?, priority = ?, data_type = ?, action = ?, config = ?, updated_at = ?
		WHERE id = ?
	`, rule.Name, rule.Description, enabled, rule.Priority, rule.DataType, rule.Action, string(configJSON), rule.UpdatedAt, rule.ID)

	if err != nil {
		return err
	}

	return s.engine.AddRule(rule)
}

// DeleteRule deletes a rule
func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM shaping_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}

	s.engine.RemoveRule(id)
	return nil
}

// GetRule returns a specific rule
func (s *Store) GetRule(id string) *Rule {
	return s.engine.GetRule(id)
}

// GetRules returns all rules
func (s *Store) GetRules() []*Rule {
	return s.engine.GetRules()
}

// EnableRule enables a rule
func (s *Store) EnableRule(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	_, err := s.db.Exec(`UPDATE shaping_rules SET enabled = ?, updated_at = ? WHERE id = ?`, enabledInt, time.Now(), id)
	if err != nil {
		return err
	}

	// Update in engine
	rule := s.engine.GetRule(id)
	if rule != nil {
		rule.Enabled = enabled
	}

	return nil
}

// GetEngine returns the shaping engine
func (s *Store) GetEngine() *Engine {
	return s.engine
}

// GetStats returns engine statistics
func (s *Store) GetStats() EngineStats {
	return s.engine.GetStats()
}

// ResetStats resets engine statistics
func (s *Store) ResetStats() {
	s.engine.ResetStats()
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Evaluate evaluates a data point against all rules
func (s *Store) Evaluate(dataType DataType, name string, tags map[string]string, level string, service string, sizeBytes int) Decision {
	return s.engine.Evaluate(dataType, name, tags, level, service, sizeBytes)
}

// EvaluateMetric evaluates a metric
func (s *Store) EvaluateMetric(name string, tags map[string]string, sizeBytes int) Decision {
	return s.engine.EvaluateMetric(name, tags, sizeBytes)
}

// EvaluateLog evaluates a log entry
func (s *Store) EvaluateLog(service string, level string, tags map[string]string, sizeBytes int) Decision {
	return s.engine.EvaluateLog(service, level, tags, sizeBytes)
}

// EvaluateTrace evaluates a trace/span
func (s *Store) EvaluateTrace(service string, tags map[string]string, sizeBytes int) Decision {
	return s.engine.EvaluateTrace(service, tags, sizeBytes)
}
