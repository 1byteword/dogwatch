package oncall

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"sync"
	"time"

)

// Schedule represents an on-call schedule
type Schedule struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Timezone    string      `json:"timezone"`
	Teams       []string    `json:"teams"`      // Teams this schedule belongs to
	Layers      []Layer     `json:"layers"`     // Rotation layers (can stack)
	Overrides   []Override  `json:"overrides"`  // Temporary overrides
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Layer represents a rotation layer in a schedule
// Multiple layers can stack - higher layers take precedence
type Layer struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Priority        int       `json:"priority"`       // Higher = takes precedence
	RotationType    string    `json:"rotation_type"`  // daily, weekly, custom
	HandoffTime     string    `json:"handoff_time"`   // HH:MM in schedule timezone
	HandoffDay      int       `json:"handoff_day"`    // 0=Sun, 1=Mon, ... (for weekly)
	ShiftDuration   Duration  `json:"shift_duration"` // How long each shift lasts
	StartDate       time.Time `json:"start_date"`     // When rotation starts
	EndDate         *time.Time `json:"end_date,omitempty"` // Optional end date
	Users           []User    `json:"users"`          // Users in rotation order
	Restrictions    []Restriction `json:"restrictions,omitempty"` // Time restrictions
}

// Duration is a JSON-serializable duration
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

// User represents a user in the rotation
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone,omitempty"`
}

// Restriction limits when a layer is active
type Restriction struct {
	Type      string `json:"type"`       // daily, weekly
	StartTime string `json:"start_time"` // HH:MM
	EndTime   string `json:"end_time"`   // HH:MM
	StartDay  int    `json:"start_day"`  // 0-6 for weekly
	EndDay    int    `json:"end_day"`    // 0-6 for weekly
}

// Override temporarily changes who is on-call
type Override struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Reason    string    `json:"reason"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// OnCallEntry represents who is on-call during a time period
type OnCallEntry struct {
	User      User      `json:"user"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	LayerID   string    `json:"layer_id,omitempty"`
	LayerName string    `json:"layer_name,omitempty"`
	IsOverride bool     `json:"is_override"`
}

// EscalationPolicy defines how incidents escalate
type EscalationPolicy struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Teams         []string         `json:"teams"`
	Rules         []EscalationRule `json:"rules"`
	RepeatEnabled bool             `json:"repeat_enabled"`
	RepeatLimit   int              `json:"repeat_limit"`   // Max times to repeat (0=infinite)
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

// EscalationRule defines one level of escalation
type EscalationRule struct {
	Level        int      `json:"level"`
	DelayMinutes int      `json:"delay_minutes"`
	Targets      []Target `json:"targets"`
}

// Target defines who to notify
type Target struct {
	Type string `json:"type"` // user, schedule, team
	ID   string `json:"id"`
}

// Store manages on-call data
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new on-call store
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
	CREATE TABLE IF NOT EXISTS schedules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		timezone TEXT DEFAULT 'UTC',
		teams TEXT,
		layers TEXT,
		overrides TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS escalation_policies (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		teams TEXT,
		rules TEXT,
		repeat_enabled INTEGER DEFAULT 0,
		repeat_limit INTEGER DEFAULT 0,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS overrides (
		id TEXT PRIMARY KEY,
		schedule_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		user_name TEXT,
		start_time INTEGER NOT NULL,
		end_time INTEGER NOT NULL,
		reason TEXT,
		created_by TEXT,
		created_at INTEGER,
		FOREIGN KEY (schedule_id) REFERENCES schedules(id)
	);

	CREATE INDEX IF NOT EXISTS idx_overrides_schedule ON overrides(schedule_id);
	CREATE INDEX IF NOT EXISTS idx_overrides_time ON overrides(start_time, end_time);

	CREATE TABLE IF NOT EXISTS escalation_states (
		incident_id TEXT PRIMARY KEY,
		policy_id TEXT NOT NULL,
		current_level INTEGER DEFAULT 0,
		repeat_count INTEGER DEFAULT 0,
		started_at INTEGER NOT NULL,
		last_escalation INTEGER NOT NULL,
		acknowledged INTEGER DEFAULT 0,
		acked_by TEXT,
		acked_at INTEGER,
		resolved INTEGER DEFAULT 0,
		notifications TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_esc_states_resolved ON escalation_states(resolved);
	`

	_, err := s.db.Exec(schema)
	return err
}

// CreateSchedule creates a new on-call schedule
func (s *Store) CreateSchedule(sched *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	sched.CreatedAt = now
	sched.UpdatedAt = now

	teamsJSON, _ := json.Marshal(sched.Teams)
	layersJSON, _ := json.Marshal(sched.Layers)
	overridesJSON, _ := json.Marshal(sched.Overrides)

	_, err := s.db.Exec(`
		INSERT INTO schedules (id, name, description, timezone, teams, layers, overrides, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sched.ID, sched.Name, sched.Description, sched.Timezone,
		string(teamsJSON), string(layersJSON), string(overridesJSON),
		now.Unix(), now.Unix())

	return err
}

// GetSchedule retrieves a schedule by ID
func (s *Store) GetSchedule(id string) (*Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sched Schedule
	var teamsJSON, layersJSON, overridesJSON string
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, name, description, timezone, teams, layers, overrides, created_at, updated_at
		FROM schedules WHERE id = ?`, id).Scan(
		&sched.ID, &sched.Name, &sched.Description, &sched.Timezone,
		&teamsJSON, &layersJSON, &overridesJSON, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(teamsJSON), &sched.Teams)
	json.Unmarshal([]byte(layersJSON), &sched.Layers)
	json.Unmarshal([]byte(overridesJSON), &sched.Overrides)
	sched.CreatedAt = time.Unix(createdAt, 0)
	sched.UpdatedAt = time.Unix(updatedAt, 0)

	// Load overrides from separate table
	overrides, _ := s.getOverridesForSchedule(id)
	sched.Overrides = append(sched.Overrides, overrides...)

	return &sched, nil
}

// UpdateSchedule updates a schedule
func (s *Store) UpdateSchedule(sched *Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sched.UpdatedAt = time.Now()

	teamsJSON, _ := json.Marshal(sched.Teams)
	layersJSON, _ := json.Marshal(sched.Layers)
	overridesJSON, _ := json.Marshal(sched.Overrides)

	_, err := s.db.Exec(`
		UPDATE schedules
		SET name=?, description=?, timezone=?, teams=?, layers=?, overrides=?, updated_at=?
		WHERE id=?`,
		sched.Name, sched.Description, sched.Timezone,
		string(teamsJSON), string(layersJSON), string(overridesJSON),
		sched.UpdatedAt.Unix(), sched.ID)

	return err
}

// DeleteSchedule removes a schedule and its associated overrides
func (s *Store) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete child overrides first to avoid FK constraint violation
	_, err := s.db.Exec("DELETE FROM overrides WHERE schedule_id = ?", id)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("DELETE FROM schedules WHERE id = ?", id)
	return err
}

// ListSchedules returns all schedules
func (s *Store) ListSchedules() ([]Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, description, timezone, teams, layers, overrides, created_at, updated_at
		FROM schedules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sched Schedule
		var teamsJSON, layersJSON, overridesJSON string
		var createdAt, updatedAt int64

		if err := rows.Scan(&sched.ID, &sched.Name, &sched.Description, &sched.Timezone,
			&teamsJSON, &layersJSON, &overridesJSON, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(teamsJSON), &sched.Teams)
		json.Unmarshal([]byte(layersJSON), &sched.Layers)
		json.Unmarshal([]byte(overridesJSON), &sched.Overrides)
		sched.CreatedAt = time.Unix(createdAt, 0)
		sched.UpdatedAt = time.Unix(updatedAt, 0)

		schedules = append(schedules, sched)
	}

	return schedules, nil
}

// CreateOverride creates a temporary override
func (s *Store) CreateOverride(scheduleID string, override *Override) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	override.CreatedAt = time.Now()

	_, err := s.db.Exec(`
		INSERT INTO overrides (id, schedule_id, user_id, user_name, start_time, end_time, reason, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		override.ID, scheduleID, override.UserID, override.UserName,
		override.StartTime.Unix(), override.EndTime.Unix(),
		override.Reason, override.CreatedBy, override.CreatedAt.Unix())

	return err
}

// DeleteOverride removes an override
func (s *Store) DeleteOverride(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM overrides WHERE id = ?", id)
	return err
}

// getOverridesForSchedule returns active/future overrides for a schedule
func (s *Store) getOverridesForSchedule(scheduleID string) ([]Override, error) {
	now := time.Now().Unix()

	rows, err := s.db.Query(`
		SELECT id, user_id, user_name, start_time, end_time, reason, created_by, created_at
		FROM overrides
		WHERE schedule_id = ? AND end_time > ?
		ORDER BY start_time`, scheduleID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []Override
	for rows.Next() {
		var o Override
		var startTime, endTime, createdAt int64

		if err := rows.Scan(&o.ID, &o.UserID, &o.UserName, &startTime, &endTime,
			&o.Reason, &o.CreatedBy, &createdAt); err != nil {
			continue
		}

		o.StartTime = time.Unix(startTime, 0)
		o.EndTime = time.Unix(endTime, 0)
		o.CreatedAt = time.Unix(createdAt, 0)
		overrides = append(overrides, o)
	}

	return overrides, nil
}

// Escalation Policy methods

// CreatePolicy creates a new escalation policy
func (s *Store) CreatePolicy(policy *EscalationPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	teamsJSON, _ := json.Marshal(policy.Teams)
	rulesJSON, _ := json.Marshal(policy.Rules)

	_, err := s.db.Exec(`
		INSERT INTO escalation_policies (id, name, description, teams, rules, repeat_enabled, repeat_limit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		policy.ID, policy.Name, policy.Description,
		string(teamsJSON), string(rulesJSON),
		policy.RepeatEnabled, policy.RepeatLimit,
		now.Unix(), now.Unix())

	return err
}

// GetPolicy retrieves an escalation policy
func (s *Store) GetPolicy(id string) (*EscalationPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var policy EscalationPolicy
	var teamsJSON, rulesJSON string
	var createdAt, updatedAt int64
	var repeatEnabled int

	err := s.db.QueryRow(`
		SELECT id, name, description, teams, rules, repeat_enabled, repeat_limit, created_at, updated_at
		FROM escalation_policies WHERE id = ?`, id).Scan(
		&policy.ID, &policy.Name, &policy.Description,
		&teamsJSON, &rulesJSON,
		&repeatEnabled, &policy.RepeatLimit,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(teamsJSON), &policy.Teams)
	json.Unmarshal([]byte(rulesJSON), &policy.Rules)
	policy.RepeatEnabled = repeatEnabled == 1
	policy.CreatedAt = time.Unix(createdAt, 0)
	policy.UpdatedAt = time.Unix(updatedAt, 0)

	return &policy, nil
}

// UpdatePolicy updates an escalation policy
func (s *Store) UpdatePolicy(policy *EscalationPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	policy.UpdatedAt = time.Now()

	teamsJSON, _ := json.Marshal(policy.Teams)
	rulesJSON, _ := json.Marshal(policy.Rules)

	repeatEnabled := 0
	if policy.RepeatEnabled {
		repeatEnabled = 1
	}

	_, err := s.db.Exec(`
		UPDATE escalation_policies
		SET name=?, description=?, teams=?, rules=?, repeat_enabled=?, repeat_limit=?, updated_at=?
		WHERE id=?`,
		policy.Name, policy.Description,
		string(teamsJSON), string(rulesJSON),
		repeatEnabled, policy.RepeatLimit,
		policy.UpdatedAt.Unix(), policy.ID)

	return err
}

// DeletePolicy removes an escalation policy
func (s *Store) DeletePolicy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM escalation_policies WHERE id = ?", id)
	return err
}

// ListPolicies returns all escalation policies
func (s *Store) ListPolicies() ([]EscalationPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, description, teams, rules, repeat_enabled, repeat_limit, created_at, updated_at
		FROM escalation_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []EscalationPolicy
	for rows.Next() {
		var policy EscalationPolicy
		var teamsJSON, rulesJSON string
		var createdAt, updatedAt int64
		var repeatEnabled int

		if err := rows.Scan(&policy.ID, &policy.Name, &policy.Description,
			&teamsJSON, &rulesJSON, &repeatEnabled, &policy.RepeatLimit,
			&createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(teamsJSON), &policy.Teams)
		json.Unmarshal([]byte(rulesJSON), &policy.Rules)
		policy.RepeatEnabled = repeatEnabled == 1
		policy.CreatedAt = time.Unix(createdAt, 0)
		policy.UpdatedAt = time.Unix(updatedAt, 0)

		policies = append(policies, policy)
	}

	return policies, nil
}

// SaveEscalationState persists an escalation state to the database.
func (s *Store) SaveEscalationState(state *EscalationState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notificationsJSON, _ := json.Marshal(state.Notifications)

	var ackedAt int64
	if state.AckedAt != nil {
		ackedAt = state.AckedAt.Unix()
	}
	acked := 0
	if state.Acknowledged {
		acked = 1
	}
	resolved := 0
	if state.Resolved {
		resolved = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO escalation_states (incident_id, policy_id, current_level, repeat_count,
			started_at, last_escalation, acknowledged, acked_by, acked_at, resolved, notifications)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(incident_id) DO UPDATE SET
			current_level = excluded.current_level,
			repeat_count = excluded.repeat_count,
			last_escalation = excluded.last_escalation,
			acknowledged = excluded.acknowledged,
			acked_by = excluded.acked_by,
			acked_at = excluded.acked_at,
			resolved = excluded.resolved,
			notifications = excluded.notifications
	`,
		state.IncidentID, state.PolicyID, state.CurrentLevel, state.RepeatCount,
		state.StartedAt.Unix(), state.LastEscalation.Unix(),
		acked, state.AckedBy, ackedAt, resolved,
		string(notificationsJSON),
	)
	return err
}

// LoadActiveEscalationStates loads all unresolved escalation states from the database.
func (s *Store) LoadActiveEscalationStates() ([]*EscalationState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT incident_id, policy_id, current_level, repeat_count,
			started_at, last_escalation, acknowledged, acked_by, acked_at, resolved, notifications
		FROM escalation_states WHERE resolved = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var states []*EscalationState
	for rows.Next() {
		var state EscalationState
		var startedAt, lastEsc, ackedAt int64
		var acked, resolved int
		var notifJSON sql.NullString

		if err := rows.Scan(&state.IncidentID, &state.PolicyID, &state.CurrentLevel,
			&state.RepeatCount, &startedAt, &lastEsc, &acked, &state.AckedBy,
			&ackedAt, &resolved, &notifJSON); err != nil {
			continue
		}

		state.StartedAt = time.Unix(startedAt, 0)
		state.LastEscalation = time.Unix(lastEsc, 0)
		state.Acknowledged = acked == 1
		state.Resolved = resolved == 1
		if ackedAt > 0 {
			t := time.Unix(ackedAt, 0)
			state.AckedAt = &t
		}
		if notifJSON.Valid {
			json.Unmarshal([]byte(notifJSON.String), &state.Notifications)
		}

		states = append(states, &state)
	}
	return states, nil
}

// DeleteEscalationState removes a resolved escalation state.
func (s *Store) DeleteEscalationState(incidentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM escalation_states WHERE incident_id = ?`, incidentID)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}
