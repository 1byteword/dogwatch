package alerting

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"sync"
	"time"

)

// RuleType defines the type of alert rule
type RuleType string

const (
	RuleTypeThreshold RuleType = "threshold"
	RuleTypeAnomaly   RuleType = "anomaly"
	RuleTypeComposite RuleType = "composite"
	RuleTypeChange    RuleType = "change"
	RuleTypeAbsence   RuleType = "absence"
)

// Severity levels
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// AlertState represents the current state of an alert
type AlertState string

const (
	StateInactive AlertState = "inactive" // Condition not met
	StatePending  AlertState = "pending"  // Condition met, waiting for duration
	StateFiring   AlertState = "firing"   // Alert is active
	StateResolved AlertState = "resolved" // Was firing, now resolved
)

// Rule defines an alert rule
type Rule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Type        RuleType          `json:"type"`
	Enabled     bool              `json:"enabled"`
	Labels      map[string]string `json:"labels"`      // For routing
	Annotations map[string]string `json:"annotations"` // For display

	// Query configuration
	Query    string `json:"query"`     // WatchQL query
	Metric   string `json:"metric"`    // Or simple metric name
	GroupBy  []string `json:"group_by"` // Group alerts by these labels

	// Threshold configuration
	Condition  string  `json:"condition"`  // gt, lt, gte, lte, eq, neq
	Threshold  float64 `json:"threshold"`  // Value to compare
	Threshold2 float64 `json:"threshold2"` // For range conditions

	// Anomaly configuration
	Sensitivity float64 `json:"sensitivity"` // 0-1, for anomaly detection
	Algorithm   string  `json:"algorithm"`   // zscore, isolation_forest

	// Change detection
	ChangeType    string  `json:"change_type"`    // percent, absolute
	ChangeWindow  string  `json:"change_window"`  // Compare to this time ago
	ChangeThreshold float64 `json:"change_threshold"` // Amount of change

	// Composite configuration
	Expression string   `json:"expression"` // Boolean expression of other rules
	SubRules   []string `json:"sub_rules"`  // Rule IDs for composite

	// Timing
	EvalInterval  time.Duration `json:"eval_interval"`   // How often to evaluate
	ForDuration   time.Duration `json:"for_duration"`    // Must be true for this long
	KeepFiringFor time.Duration `json:"keep_firing_for"` // Keep firing after resolve

	// Routing
	NotifyChannels []string `json:"notify_channels"` // Channels to notify
	EscalationID   string   `json:"escalation_id"`   // Escalation policy

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
}

// Alert represents an active or recent alert instance
type Alert struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	State       AlertState        `json:"state"`
	Severity    Severity          `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	Value       float64           `json:"value"`     // Current value
	Threshold   float64           `json:"threshold"` // Threshold that was crossed

	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	FiredAt     *time.Time `json:"fired_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	LastEvalAt  time.Time  `json:"last_eval_at"`

	// Notification tracking
	NotifiedAt    *time.Time `json:"notified_at,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string    `json:"acknowledged_by,omitempty"`

	// Fingerprint for deduplication
	Fingerprint string `json:"fingerprint"`
}

// Silence suppresses alerts matching certain labels
type Silence struct {
	ID        string            `json:"id"`
	Matchers  []Matcher         `json:"matchers"`
	StartsAt  time.Time         `json:"starts_at"`
	EndsAt    time.Time         `json:"ends_at"`
	CreatedBy string            `json:"created_by"`
	Comment   string            `json:"comment"`
	CreatedAt time.Time         `json:"created_at"`
}

// Matcher defines a label matcher for silences
type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"is_regex"`
	IsEqual bool   `json:"is_equal"` // true = match, false = not match
}

// InhibitRule suppresses alerts when other alerts are firing
type InhibitRule struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SourceMatch  []Matcher `json:"source_match"`  // Match source alert
	TargetMatch  []Matcher `json:"target_match"`  // Match target to inhibit
	EqualLabels  []string  `json:"equal_labels"`  // Labels that must match
	CreatedAt    time.Time `json:"created_at"`
}

// Route defines how to route alerts
type Route struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Matchers       []Matcher `json:"matchers"`
	Receiver       string    `json:"receiver"`        // Channel or escalation
	GroupBy        []string  `json:"group_by"`        // Group alerts
	GroupWait      time.Duration `json:"group_wait"`      // Wait before sending
	GroupInterval  time.Duration `json:"group_interval"`  // Interval between sends
	RepeatInterval time.Duration `json:"repeat_interval"` // Repeat if still firing
	MuteTimeIntervals []string `json:"mute_time_intervals"` // Times to mute
	Continue       bool      `json:"continue"`        // Continue to next route
	Children       []Route   `json:"children"`        // Sub-routes
}

// Store manages alert rules and state
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS alert_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		type TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		config TEXT NOT NULL,
		created_at INTEGER,
		updated_at INTEGER,
		created_by TEXT
	);

	CREATE TABLE IF NOT EXISTS alerts (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL,
		fingerprint TEXT NOT NULL,
		state TEXT NOT NULL,
		severity TEXT,
		labels TEXT,
		annotations TEXT,
		value REAL,
		starts_at INTEGER,
		ends_at INTEGER,
		fired_at INTEGER,
		resolved_at INTEGER,
		last_eval_at INTEGER,
		notified_at INTEGER,
		acknowledged_at INTEGER,
		acknowledged_by TEXT,
		FOREIGN KEY (rule_id) REFERENCES alert_rules(id)
	);

	CREATE TABLE IF NOT EXISTS silences (
		id TEXT PRIMARY KEY,
		matchers TEXT NOT NULL,
		starts_at INTEGER NOT NULL,
		ends_at INTEGER NOT NULL,
		created_by TEXT,
		comment TEXT,
		created_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS inhibit_rules (
		id TEXT PRIMARY KEY,
		name TEXT,
		source_match TEXT,
		target_match TEXT,
		equal_labels TEXT,
		created_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS routes (
		id TEXT PRIMARY KEY,
		name TEXT,
		config TEXT NOT NULL,
		created_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_alerts_rule ON alerts(rule_id);
	CREATE INDEX IF NOT EXISTS idx_alerts_state ON alerts(state);
	CREATE INDEX IF NOT EXISTS idx_alerts_fingerprint ON alerts(fingerprint);
	CREATE INDEX IF NOT EXISTS idx_silences_time ON silences(starts_at, ends_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Rule CRUD operations

func (s *Store) CreateRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	config, _ := json.Marshal(rule)

	_, err := s.db.Exec(`
		INSERT INTO alert_rules (id, name, description, type, enabled, config, created_at, updated_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, rule.Description, rule.Type, rule.Enabled,
		string(config), now.Unix(), now.Unix(), rule.CreatedBy)

	return err
}

func (s *Store) GetRule(id string) (*Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var config string
	err := s.db.QueryRow("SELECT config FROM alert_rules WHERE id = ?", id).Scan(&config)
	if err != nil {
		return nil, err
	}

	var rule Rule
	if err := json.Unmarshal([]byte(config), &rule); err != nil {
		return nil, err
	}

	return &rule, nil
}

func (s *Store) UpdateRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rule.UpdatedAt = time.Now()
	config, _ := json.Marshal(rule)

	_, err := s.db.Exec(`
		UPDATE alert_rules SET name=?, description=?, type=?, enabled=?, config=?, updated_at=?
		WHERE id=?`,
		rule.Name, rule.Description, rule.Type, rule.Enabled,
		string(config), rule.UpdatedAt.Unix(), rule.ID)

	return err
}

func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM alert_rules WHERE id = ?", id)
	return err
}

func (s *Store) ListRules() ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT config FROM alert_rules ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var config string
		if err := rows.Scan(&config); err != nil {
			continue
		}

		var rule Rule
		if err := json.Unmarshal([]byte(config), &rule); err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func (s *Store) ListEnabledRules() ([]Rule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT config FROM alert_rules WHERE enabled = 1 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule
	for rows.Next() {
		var config string
		if err := rows.Scan(&config); err != nil {
			continue
		}

		var rule Rule
		if err := json.Unmarshal([]byte(config), &rule); err != nil {
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// Alert operations

func (s *Store) SaveAlert(alert *Alert) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	labels, _ := json.Marshal(alert.Labels)
	annotations, _ := json.Marshal(alert.Annotations)

	var endsAt, firedAt, resolvedAt, notifiedAt, ackedAt *int64
	if alert.EndsAt != nil {
		t := alert.EndsAt.Unix()
		endsAt = &t
	}
	if alert.FiredAt != nil {
		t := alert.FiredAt.Unix()
		firedAt = &t
	}
	if alert.ResolvedAt != nil {
		t := alert.ResolvedAt.Unix()
		resolvedAt = &t
	}
	if alert.NotifiedAt != nil {
		t := alert.NotifiedAt.Unix()
		notifiedAt = &t
	}
	if alert.AcknowledgedAt != nil {
		t := alert.AcknowledgedAt.Unix()
		ackedAt = &t
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO alerts
		(id, rule_id, fingerprint, state, severity, labels, annotations, value,
		 starts_at, ends_at, fired_at, resolved_at, last_eval_at, notified_at,
		 acknowledged_at, acknowledged_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		alert.ID, alert.RuleID, alert.Fingerprint, alert.State, alert.Severity,
		string(labels), string(annotations), alert.Value,
		alert.StartsAt.Unix(), endsAt, firedAt, resolvedAt, alert.LastEvalAt.Unix(),
		notifiedAt, ackedAt, alert.AcknowledgedBy)

	return err
}

func (s *Store) GetAlert(id string) (*Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAlert(id)
}

func (s *Store) getAlert(id string) (*Alert, error) {
	var alert Alert
	var labels, annotations string
	var startsAt, lastEvalAt int64
	var endsAt, firedAt, resolvedAt, notifiedAt, ackedAt sql.NullInt64
	var ackedBy sql.NullString

	err := s.db.QueryRow(`
		SELECT id, rule_id, fingerprint, state, severity, labels, annotations, value,
		       starts_at, ends_at, fired_at, resolved_at, last_eval_at, notified_at,
		       acknowledged_at, acknowledged_by
		FROM alerts WHERE id = ?`, id).Scan(
		&alert.ID, &alert.RuleID, &alert.Fingerprint, &alert.State, &alert.Severity,
		&labels, &annotations, &alert.Value, &startsAt, &endsAt, &firedAt,
		&resolvedAt, &lastEvalAt, &notifiedAt, &ackedAt, &ackedBy)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(labels), &alert.Labels)
	json.Unmarshal([]byte(annotations), &alert.Annotations)
	alert.StartsAt = time.Unix(startsAt, 0)
	alert.LastEvalAt = time.Unix(lastEvalAt, 0)

	if endsAt.Valid {
		t := time.Unix(endsAt.Int64, 0)
		alert.EndsAt = &t
	}
	if firedAt.Valid {
		t := time.Unix(firedAt.Int64, 0)
		alert.FiredAt = &t
	}
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0)
		alert.ResolvedAt = &t
	}
	if notifiedAt.Valid {
		t := time.Unix(notifiedAt.Int64, 0)
		alert.NotifiedAt = &t
	}
	if ackedAt.Valid {
		t := time.Unix(ackedAt.Int64, 0)
		alert.AcknowledgedAt = &t
	}
	if ackedBy.Valid {
		alert.AcknowledgedBy = ackedBy.String
	}

	return &alert, nil
}

func (s *Store) GetAlertByFingerprint(fingerprint string) (*Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var id string
	err := s.db.QueryRow("SELECT id FROM alerts WHERE fingerprint = ? ORDER BY starts_at DESC LIMIT 1", fingerprint).Scan(&id)
	if err != nil {
		return nil, err
	}

	return s.getAlert(id)
}

func (s *Store) ListAlerts(state AlertState) ([]Alert, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT id FROM alerts"
	var args []interface{}

	if state != "" {
		query += " WHERE state = ?"
		args = append(args, state)
	}
	query += " ORDER BY starts_at DESC LIMIT 1000"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		alert, err := s.getAlert(id)
		if err != nil {
			continue
		}
		alerts = append(alerts, *alert)
	}

	return alerts, nil
}

func (s *Store) ListFiringAlerts() ([]Alert, error) {
	return s.ListAlerts(StateFiring)
}

// Silence operations

func (s *Store) CreateSilence(silence *Silence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	silence.CreatedAt = time.Now()
	matchers, _ := json.Marshal(silence.Matchers)

	_, err := s.db.Exec(`
		INSERT INTO silences (id, matchers, starts_at, ends_at, created_by, comment, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		silence.ID, string(matchers), silence.StartsAt.Unix(), silence.EndsAt.Unix(),
		silence.CreatedBy, silence.Comment, silence.CreatedAt.Unix())

	return err
}

func (s *Store) DeleteSilence(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM silences WHERE id = ?", id)
	return err
}

func (s *Store) ListActiveSilences() ([]Silence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now().Unix()
	rows, err := s.db.Query(`
		SELECT id, matchers, starts_at, ends_at, created_by, comment, created_at
		FROM silences WHERE starts_at <= ? AND ends_at > ?`, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var silences []Silence
	for rows.Next() {
		var s Silence
		var matchers string
		var startsAt, endsAt, createdAt int64

		if err := rows.Scan(&s.ID, &matchers, &startsAt, &endsAt, &s.CreatedBy, &s.Comment, &createdAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(matchers), &s.Matchers)
		s.StartsAt = time.Unix(startsAt, 0)
		s.EndsAt = time.Unix(endsAt, 0)
		s.CreatedAt = time.Unix(createdAt, 0)

		silences = append(silences, s)
	}

	return silences, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
