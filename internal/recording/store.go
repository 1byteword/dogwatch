package recording

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store manages recording rule persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new recording rules store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS recording_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		expression TEXT NOT NULL,
		interval_ns INTEGER NOT NULL,
		labels TEXT,
		enabled INTEGER DEFAULT 1,
		description TEXT,
		last_eval INTEGER,
		last_error TEXT,
		last_value REAL,
		created_at INTEGER,
		updated_at INTEGER,
		created_by TEXT
	);

	CREATE TABLE IF NOT EXISTS evaluation_history (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL,
		timestamp INTEGER NOT NULL,
		value REAL,
		duration_ms INTEGER,
		error TEXT,
		success INTEGER,
		FOREIGN KEY (rule_id) REFERENCES recording_rules(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_eval_history_rule ON evaluation_history(rule_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_recording_rules_name ON recording_rules(name);
	CREATE INDEX IF NOT EXISTS idx_recording_rules_enabled ON recording_rules(enabled);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database for query execution
func (s *Store) DB() *sql.DB {
	return s.db
}

// CreateRule creates a new recording rule
func (s *Store) CreateRule(rule *RecordingRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	labels, _ := json.Marshal(rule.Labels)

	_, err := s.db.Exec(`
		INSERT INTO recording_rules
		(id, name, expression, interval_ns, labels, enabled, description, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, rule.Expression, rule.Interval.Nanoseconds(),
		string(labels), rule.Enabled, rule.Description,
		now.Unix(), now.Unix(), rule.CreatedBy,
	)

	return err
}

// GetRule retrieves a rule by ID
func (s *Store) GetRule(id string) (*RecordingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getRule(id)
}

func (s *Store) getRule(id string) (*RecordingRule, error) {
	var rule RecordingRule
	var labelsJSON sql.NullString
	var intervalNs int64
	var createdAt, updatedAt sql.NullInt64
	var lastEval sql.NullInt64
	var lastError sql.NullString
	var lastValue sql.NullFloat64
	var description, createdBy sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, expression, interval_ns, labels, enabled, description,
		       last_eval, last_error, last_value, created_at, updated_at, created_by
		FROM recording_rules WHERE id = ?`, id).Scan(
		&rule.ID, &rule.Name, &rule.Expression, &intervalNs,
		&labelsJSON, &rule.Enabled, &description,
		&lastEval, &lastError, &lastValue,
		&createdAt, &updatedAt, &createdBy,
	)
	if err != nil {
		return nil, err
	}

	rule.Interval = time.Duration(intervalNs)
	if labelsJSON.Valid && labelsJSON.String != "" {
		json.Unmarshal([]byte(labelsJSON.String), &rule.Labels)
	}
	if rule.Labels == nil {
		rule.Labels = make(map[string]string)
	}
	if description.Valid {
		rule.Description = description.String
	}
	if lastEval.Valid {
		rule.LastEval = time.Unix(lastEval.Int64, 0)
	}
	if lastError.Valid {
		rule.LastError = lastError.String
	}
	if lastValue.Valid {
		rule.LastValue = lastValue.Float64
	}
	if createdAt.Valid {
		rule.CreatedAt = time.Unix(createdAt.Int64, 0)
	}
	if updatedAt.Valid {
		rule.UpdatedAt = time.Unix(updatedAt.Int64, 0)
	}
	if createdBy.Valid {
		rule.CreatedBy = createdBy.String
	}

	return &rule, nil
}

// GetRuleByName retrieves a rule by name
func (s *Store) GetRuleByName(name string) (*RecordingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var id string
	err := s.db.QueryRow("SELECT id FROM recording_rules WHERE name = ?", name).Scan(&id)
	if err != nil {
		return nil, err
	}

	return s.getRule(id)
}

// UpdateRule updates an existing rule
func (s *Store) UpdateRule(rule *RecordingRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rule.UpdatedAt = time.Now()
	labels, _ := json.Marshal(rule.Labels)

	_, err := s.db.Exec(`
		UPDATE recording_rules
		SET name=?, expression=?, interval_ns=?, labels=?, enabled=?,
		    description=?, updated_at=?
		WHERE id=?`,
		rule.Name, rule.Expression, rule.Interval.Nanoseconds(),
		string(labels), rule.Enabled, rule.Description,
		rule.UpdatedAt.Unix(), rule.ID,
	)

	return err
}

// DeleteRule deletes a rule
func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete evaluation history first
	s.db.Exec("DELETE FROM evaluation_history WHERE rule_id = ?", id)

	_, err := s.db.Exec("DELETE FROM recording_rules WHERE id = ?", id)
	return err
}

// ListRules returns all recording rules
func (s *Store) ListRules() ([]RecordingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, expression, interval_ns, labels, enabled, description,
		       last_eval, last_error, last_value, created_at, updated_at, created_by
		FROM recording_rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RecordingRule
	for rows.Next() {
		var rule RecordingRule
		var labelsJSON sql.NullString
		var intervalNs int64
		var createdAt, updatedAt sql.NullInt64
		var lastEval sql.NullInt64
		var lastError sql.NullString
		var lastValue sql.NullFloat64
		var description, createdBy sql.NullString

		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Expression, &intervalNs,
			&labelsJSON, &rule.Enabled, &description,
			&lastEval, &lastError, &lastValue,
			&createdAt, &updatedAt, &createdBy,
		); err != nil {
			continue
		}

		rule.Interval = time.Duration(intervalNs)
		if labelsJSON.Valid && labelsJSON.String != "" {
			json.Unmarshal([]byte(labelsJSON.String), &rule.Labels)
		}
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}
		if description.Valid {
			rule.Description = description.String
		}
		if lastEval.Valid {
			rule.LastEval = time.Unix(lastEval.Int64, 0)
		}
		if lastError.Valid {
			rule.LastError = lastError.String
		}
		if lastValue.Valid {
			rule.LastValue = lastValue.Float64
		}
		if createdAt.Valid {
			rule.CreatedAt = time.Unix(createdAt.Int64, 0)
		}
		if updatedAt.Valid {
			rule.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}
		if createdBy.Valid {
			rule.CreatedBy = createdBy.String
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// ListEnabledRules returns only enabled rules
func (s *Store) ListEnabledRules() ([]RecordingRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, expression, interval_ns, labels, enabled, description,
		       last_eval, last_error, last_value, created_at, updated_at, created_by
		FROM recording_rules WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []RecordingRule
	for rows.Next() {
		var rule RecordingRule
		var labelsJSON sql.NullString
		var intervalNs int64
		var createdAt, updatedAt sql.NullInt64
		var lastEval sql.NullInt64
		var lastError sql.NullString
		var lastValue sql.NullFloat64
		var description, createdBy sql.NullString

		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Expression, &intervalNs,
			&labelsJSON, &rule.Enabled, &description,
			&lastEval, &lastError, &lastValue,
			&createdAt, &updatedAt, &createdBy,
		); err != nil {
			continue
		}

		rule.Interval = time.Duration(intervalNs)
		if labelsJSON.Valid && labelsJSON.String != "" {
			json.Unmarshal([]byte(labelsJSON.String), &rule.Labels)
		}
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}
		if description.Valid {
			rule.Description = description.String
		}
		if lastEval.Valid {
			rule.LastEval = time.Unix(lastEval.Int64, 0)
		}
		if lastError.Valid {
			rule.LastError = lastError.String
		}
		if lastValue.Valid {
			rule.LastValue = lastValue.Float64
		}
		if createdAt.Valid {
			rule.CreatedAt = time.Unix(createdAt.Int64, 0)
		}
		if updatedAt.Valid {
			rule.UpdatedAt = time.Unix(updatedAt.Int64, 0)
		}
		if createdBy.Valid {
			rule.CreatedBy = createdBy.String
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// UpdateEvaluation updates evaluation state for a rule
func (s *Store) UpdateEvaluation(id string, value float64, evalErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var errStr *string
	if evalErr != nil {
		e := evalErr.Error()
		errStr = &e
	}

	_, err := s.db.Exec(`
		UPDATE recording_rules
		SET last_eval=?, last_value=?, last_error=?
		WHERE id=?`,
		now.Unix(), value, errStr, id,
	)

	return err
}

// RecordEvaluation records an evaluation in history
func (s *Store) RecordEvaluation(history *EvaluationHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var errStr *string
	if history.Error != "" {
		errStr = &history.Error
	}

	_, err := s.db.Exec(`
		INSERT INTO evaluation_history
		(id, rule_id, timestamp, value, duration_ms, error, success)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		history.ID, history.RuleID, history.Timestamp.Unix(),
		history.Value, history.Duration, errStr, history.Success,
	)

	return err
}

// GetEvaluationHistory returns evaluation history for a rule
func (s *Store) GetEvaluationHistory(ruleID string, limit int) ([]EvaluationHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, rule_id, timestamp, value, duration_ms, error, success
		FROM evaluation_history
		WHERE rule_id = ?
		ORDER BY timestamp DESC
		LIMIT ?`, ruleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []EvaluationHistory
	for rows.Next() {
		var h EvaluationHistory
		var ts int64
		var errStr sql.NullString

		if err := rows.Scan(&h.ID, &h.RuleID, &ts, &h.Value, &h.Duration, &errStr, &h.Success); err != nil {
			continue
		}

		h.Timestamp = time.Unix(ts, 0)
		if errStr.Valid {
			h.Error = errStr.String
		}

		history = append(history, h)
	}

	return history, nil
}

// CleanupHistory removes old evaluation history
func (s *Store) CleanupHistory(maxAge time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Unix()
	result, err := s.db.Exec("DELETE FROM evaluation_history WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EnsureDefaultRules creates the default rules if they don't exist
func (s *Store) EnsureDefaultRules() error {
	defaults := DefaultRules()

	for _, rule := range defaults {
		_, err := s.GetRule(rule.ID)
		if err == sql.ErrNoRows {
			rule.CreatedAt = time.Now()
			rule.UpdatedAt = time.Now()
			if err := s.CreateRule(&rule); err != nil {
				return fmt.Errorf("create default rule %s: %w", rule.Name, err)
			}
		}
	}

	return nil
}
