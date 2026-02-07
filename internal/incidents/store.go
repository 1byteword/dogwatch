package incidents

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Severity levels
type Severity string

const (
	SeverityCritical Severity = "critical" // P1 - page immediately
	SeverityHigh     Severity = "high"     // P2 - page during business hours
	SeverityMedium   Severity = "medium"   // P3 - notify, no page
	SeverityLow      Severity = "low"      // P4 - informational
)

// Status represents incident lifecycle
type Status string

const (
	StatusTriggered    Status = "triggered"    // Initial state, actively paging
	StatusAcknowledged Status = "acknowledged" // Someone is working on it
	StatusResolved     Status = "resolved"     // Incident closed
)

// Incident represents a paging incident
type Incident struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Severity    Severity          `json:"severity"`
	Status      Status            `json:"status"`
	Service     string            `json:"service"`    // Affected service name (for display)
	ServiceID   string            `json:"service_id"` // Link to service catalog
	Source      string            `json:"source"`     // watch, slo, synthetic, manual
	SourceID    string            `json:"source_id"`  // ID of triggering alert
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	AckedAt     *time.Time        `json:"acked_at,omitempty"`
	AckedBy     string            `json:"acked_by,omitempty"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	ResolvedBy  string            `json:"resolved_by,omitempty"`
	AssignedTo  string            `json:"assigned_to,omitempty"` // Current assignee
	EscLevel    int               `json:"escalation_level"`      // Current escalation level
	Tags        map[string]string `json:"tags,omitempty"`
	Timeline    []TimelineEvent   `json:"timeline,omitempty"`
}

// TimelineEvent records incident history
type TimelineEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`    // created, acked, escalated, resolved, note
	User      string    `json:"user"`    // Who did it
	Message   string    `json:"message"` // Description
}

// OnCallSchedule defines who is on-call
type OnCallSchedule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Timezone    string     `json:"timezone"`
	Rotations   []Rotation `json:"rotations"`
	Overrides   []Override `json:"overrides"` // Temporary overrides
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Rotation defines a recurring on-call rotation
type Rotation struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`       // daily, weekly, custom
	StartTime string   `json:"start_time"` // HH:MM
	Duration  int      `json:"duration_hours"`
	Users     []string `json:"users"` // User IDs/names in rotation order
}

// Override for temporary schedule changes
type Override struct {
	ID        string    `json:"id"`
	User      string    `json:"user"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Reason    string    `json:"reason"`
}

// EscalationPolicy defines how to escalate
type EscalationPolicy struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Rules       []EscalationRule `json:"rules"`
	RepeatAfter int              `json:"repeat_after_minutes"` // 0 = don't repeat
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// EscalationRule defines one level of escalation
type EscalationRule struct {
	Level          int      `json:"level"`
	DelayMinutes   int      `json:"delay_minutes"`   // Wait before escalating
	Targets        []Target `json:"targets"`         // Who to notify
	NotifyChannels []string `json:"notify_channels"` // slack, webhook, email
}

// Target for notifications
type Target struct {
	Type string `json:"type"` // user, schedule, team
	ID   string `json:"id"`   // User name or schedule ID
}

// NotificationLog tracks sent notifications
type NotificationLog struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	Channel    string    `json:"channel"` // slack, webhook, email
	Target     string    `json:"target"`  // Where it was sent
	Status     string    `json:"status"`  // sent, failed, delivered
	SentAt     time.Time `json:"sent_at"`
	Message    string    `json:"message"`
}

// Store handles incident persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new incident store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS incidents (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT,
		severity TEXT DEFAULT 'medium',
		status TEXT DEFAULT 'triggered',
		service TEXT,
		service_id TEXT,
		source TEXT,
		source_id TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		acked_at DATETIME,
		acked_by TEXT,
		resolved_at DATETIME,
		resolved_by TEXT,
		assigned_to TEXT,
		escalation_level INTEGER DEFAULT 0,
		tags TEXT,
		timeline TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_incident_status ON incidents(status);
	CREATE INDEX IF NOT EXISTS idx_incident_severity ON incidents(severity);
	CREATE INDEX IF NOT EXISTS idx_incident_created ON incidents(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_incident_service ON incidents(service);
	CREATE INDEX IF NOT EXISTS idx_incident_service_id ON incidents(service_id);

	CREATE TABLE IF NOT EXISTS oncall_schedules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		timezone TEXT DEFAULT 'UTC',
		rotations TEXT,
		overrides TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS escalation_policies (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		rules TEXT,
		repeat_after_minutes INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS notification_logs (
		id TEXT PRIMARY KEY,
		incident_id TEXT NOT NULL,
		channel TEXT NOT NULL,
		target TEXT,
		status TEXT,
		sent_at DATETIME NOT NULL,
		message TEXT,
		FOREIGN KEY (incident_id) REFERENCES incidents(id)
	);
	CREATE INDEX IF NOT EXISTS idx_notif_incident ON notification_logs(incident_id);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateIncident creates a new incident
func (s *Store) CreateIncident(inc *Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if inc.ID == "" {
		inc.ID = uuid.New().String()
	}
	now := time.Now()
	inc.CreatedAt = now
	inc.UpdatedAt = now
	if inc.Status == "" {
		inc.Status = StatusTriggered
	}
	if inc.Severity == "" {
		inc.Severity = SeverityMedium
	}

	// Add creation to timeline
	inc.Timeline = append(inc.Timeline, TimelineEvent{
		Timestamp: now,
		Type:      "created",
		Message:   fmt.Sprintf("Incident created: %s", inc.Title),
	})

	tagsJSON, _ := json.Marshal(inc.Tags)
	timelineJSON, _ := json.Marshal(inc.Timeline)

	_, err := s.db.Exec(`
		INSERT INTO incidents (id, title, description, severity, status, service, service_id, source, source_id,
			created_at, updated_at, assigned_to, escalation_level, tags, timeline)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.ID, inc.Title, inc.Description, inc.Severity, inc.Status, inc.Service, inc.ServiceID, inc.Source, inc.SourceID,
		inc.CreatedAt, inc.UpdatedAt, inc.AssignedTo, inc.EscLevel, string(tagsJSON), string(timelineJSON),
	)
	return err
}

// GetIncident retrieves an incident by ID
func (s *Store) GetIncident(id string) (*Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`
		SELECT id, title, description, severity, status, service, service_id, source, source_id,
			created_at, updated_at, acked_at, acked_by, resolved_at, resolved_by,
			assigned_to, escalation_level, tags, timeline
		FROM incidents WHERE id = ?`, id)

	return s.scanIncident(row)
}

// ListIncidents returns incidents with optional filters
func (s *Store) ListIncidents(status string, limit int) ([]Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if status != "" && status != "all" {
		rows, err = s.db.Query(`
			SELECT id, title, description, severity, status, service, service_id, source, source_id,
				created_at, updated_at, acked_at, acked_by, resolved_at, resolved_by,
				assigned_to, escalation_level, tags, timeline
			FROM incidents WHERE status = ?
			ORDER BY created_at DESC LIMIT ?`, status, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT id, title, description, severity, status, service, service_id, source, source_id,
				created_at, updated_at, acked_at, acked_by, resolved_at, resolved_by,
				assigned_to, escalation_level, tags, timeline
			FROM incidents ORDER BY created_at DESC LIMIT ?`, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanIncidents(rows)
}

// ListActiveIncidents returns triggered or acknowledged incidents
func (s *Store) ListActiveIncidents() ([]Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, title, description, severity, status, service, service_id, source, source_id,
			created_at, updated_at, acked_at, acked_by, resolved_at, resolved_by,
			assigned_to, escalation_level, tags, timeline
		FROM incidents WHERE status IN ('triggered', 'acknowledged')
		ORDER BY
			CASE severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				ELSE 4
			END,
			created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanIncidents(rows)
}

// ListIncidentsByService returns incidents for a specific service
func (s *Store) ListIncidentsByService(serviceID string, limit int) ([]Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, title, description, severity, status, service, service_id, source, source_id,
			created_at, updated_at, acked_at, acked_by, resolved_at, resolved_by,
			assigned_to, escalation_level, tags, timeline
		FROM incidents WHERE service_id = ?
		ORDER BY created_at DESC LIMIT ?`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanIncidents(rows)
}

// AcknowledgeIncident marks an incident as acknowledged
func (s *Store) AcknowledgeIncident(id, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Get current timeline
	var timelineJSON string
	err := s.db.QueryRow(`SELECT timeline FROM incidents WHERE id = ?`, id).Scan(&timelineJSON)
	if err != nil {
		return err
	}

	var timeline []TimelineEvent
	json.Unmarshal([]byte(timelineJSON), &timeline)
	timeline = append(timeline, TimelineEvent{
		Timestamp: now,
		Type:      "acknowledged",
		User:      user,
		Message:   fmt.Sprintf("Acknowledged by %s", user),
	})
	newTimelineJSON, _ := json.Marshal(timeline)

	_, err = s.db.Exec(`
		UPDATE incidents SET status = ?, acked_at = ?, acked_by = ?, assigned_to = ?, updated_at = ?, timeline = ?
		WHERE id = ?`,
		StatusAcknowledged, now, user, user, now, string(newTimelineJSON), id)
	return err
}

// ResolveIncident marks an incident as resolved
func (s *Store) ResolveIncident(id, user, resolution string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Get current timeline
	var timelineJSON string
	err := s.db.QueryRow(`SELECT timeline FROM incidents WHERE id = ?`, id).Scan(&timelineJSON)
	if err != nil {
		return err
	}

	var timeline []TimelineEvent
	json.Unmarshal([]byte(timelineJSON), &timeline)
	msg := fmt.Sprintf("Resolved by %s", user)
	if resolution != "" {
		msg += ": " + resolution
	}
	timeline = append(timeline, TimelineEvent{
		Timestamp: now,
		Type:      "resolved",
		User:      user,
		Message:   msg,
	})
	newTimelineJSON, _ := json.Marshal(timeline)

	_, err = s.db.Exec(`
		UPDATE incidents SET status = ?, resolved_at = ?, resolved_by = ?, updated_at = ?, timeline = ?
		WHERE id = ?`,
		StatusResolved, now, user, now, string(newTimelineJSON), id)
	return err
}

// EscalateIncident increases escalation level
func (s *Store) EscalateIncident(id string, newLevel int, assignTo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Get current timeline
	var timelineJSON string
	err := s.db.QueryRow(`SELECT timeline FROM incidents WHERE id = ?`, id).Scan(&timelineJSON)
	if err != nil {
		return err
	}

	var timeline []TimelineEvent
	json.Unmarshal([]byte(timelineJSON), &timeline)
	timeline = append(timeline, TimelineEvent{
		Timestamp: now,
		Type:      "escalated",
		Message:   fmt.Sprintf("Escalated to level %d, assigned to %s", newLevel, assignTo),
	})
	newTimelineJSON, _ := json.Marshal(timeline)

	_, err = s.db.Exec(`
		UPDATE incidents SET escalation_level = ?, assigned_to = ?, updated_at = ?, timeline = ?
		WHERE id = ?`,
		newLevel, assignTo, now, string(newTimelineJSON), id)
	return err
}

// AddNote adds a note to incident timeline
func (s *Store) AddNote(id, user, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	var timelineJSON string
	err := s.db.QueryRow(`SELECT timeline FROM incidents WHERE id = ?`, id).Scan(&timelineJSON)
	if err != nil {
		return err
	}

	var timeline []TimelineEvent
	json.Unmarshal([]byte(timelineJSON), &timeline)
	timeline = append(timeline, TimelineEvent{
		Timestamp: now,
		Type:      "note",
		User:      user,
		Message:   note,
	})
	newTimelineJSON, _ := json.Marshal(timeline)

	_, err = s.db.Exec(`UPDATE incidents SET timeline = ?, updated_at = ? WHERE id = ?`,
		string(newTimelineJSON), now, id)
	return err
}

// AssignIncident assigns an incident to a responder and appends a timeline event.
func (s *Store) AssignIncident(id, user, assignee string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	var timelineJSON string
	err := s.db.QueryRow(`SELECT timeline FROM incidents WHERE id = ?`, id).Scan(&timelineJSON)
	if err != nil {
		return err
	}

	var timeline []TimelineEvent
	json.Unmarshal([]byte(timelineJSON), &timeline)
	timeline = append(timeline, TimelineEvent{
		Timestamp: now,
		Type:      "assigned",
		User:      user,
		Message:   fmt.Sprintf("Assigned to %s by %s", assignee, user),
	})
	newTimelineJSON, _ := json.Marshal(timeline)

	_, err = s.db.Exec(`
		UPDATE incidents SET assigned_to = ?, updated_at = ?, timeline = ?
		WHERE id = ?`,
		assignee, now, string(newTimelineJSON), id)
	return err
}

// GetStats returns incident statistics
type IncidentStats struct {
	TotalIncidents    int            `json:"total_incidents"`
	ActiveIncidents   int            `json:"active_incidents"`
	TriggeredCount    int            `json:"triggered_count"`
	AcknowledgedCount int            `json:"acknowledged_count"`
	ResolvedToday     int            `json:"resolved_today"`
	MTTA              float64        `json:"mtta_minutes"` // Mean time to acknowledge
	MTTR              float64        `json:"mttr_minutes"` // Mean time to resolve
	BySeverity        map[string]int `json:"by_severity"`
	ByService         map[string]int `json:"by_service"`
}

func (s *Store) GetStats() (*IncidentStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &IncidentStats{
		BySeverity: make(map[string]int),
		ByService:  make(map[string]int),
	}

	today := time.Now().Truncate(24 * time.Hour)

	s.db.QueryRow(`SELECT COUNT(*) FROM incidents`).Scan(&stats.TotalIncidents)
	s.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE status IN ('triggered', 'acknowledged')`).Scan(&stats.ActiveIncidents)
	s.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE status = 'triggered'`).Scan(&stats.TriggeredCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE status = 'acknowledged'`).Scan(&stats.AcknowledgedCount)
	s.db.QueryRow(`SELECT COUNT(*) FROM incidents WHERE status = 'resolved' AND resolved_at >= ?`, today).Scan(&stats.ResolvedToday)

	// MTTA - average time from created to acknowledged
	s.db.QueryRow(`
		SELECT AVG((julianday(acked_at) - julianday(created_at)) * 24 * 60)
		FROM incidents WHERE acked_at IS NOT NULL`).Scan(&stats.MTTA)

	// MTTR - average time from created to resolved
	s.db.QueryRow(`
		SELECT AVG((julianday(resolved_at) - julianday(created_at)) * 24 * 60)
		FROM incidents WHERE resolved_at IS NOT NULL`).Scan(&stats.MTTR)

	// By severity
	rows, _ := s.db.Query(`SELECT severity, COUNT(*) FROM incidents WHERE status != 'resolved' GROUP BY severity`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var sev string
			var count int
			rows.Scan(&sev, &count)
			stats.BySeverity[sev] = count
		}
	}

	// By service
	rows, _ = s.db.Query(`SELECT service, COUNT(*) FROM incidents WHERE status != 'resolved' AND service != '' GROUP BY service`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var svc string
			var count int
			rows.Scan(&svc, &count)
			stats.ByService[svc] = count
		}
	}

	return stats, nil
}

// On-call schedule methods

// CreateSchedule creates a new on-call schedule
func (s *Store) CreateSchedule(sched *OnCallSchedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sched.ID == "" {
		sched.ID = uuid.New().String()
	}
	now := time.Now()
	sched.CreatedAt = now
	sched.UpdatedAt = now

	rotationsJSON, _ := json.Marshal(sched.Rotations)
	overridesJSON, _ := json.Marshal(sched.Overrides)

	_, err := s.db.Exec(`
		INSERT INTO oncall_schedules (id, name, description, timezone, rotations, overrides, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sched.ID, sched.Name, sched.Description, sched.Timezone, string(rotationsJSON), string(overridesJSON),
		sched.CreatedAt, sched.UpdatedAt,
	)
	return err
}

// GetSchedule retrieves a schedule by ID
func (s *Store) GetSchedule(id string) (*OnCallSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sched OnCallSchedule
	var rotationsJSON, overridesJSON string

	err := s.db.QueryRow(`
		SELECT id, name, description, timezone, rotations, overrides, created_at, updated_at
		FROM oncall_schedules WHERE id = ?`, id).Scan(
		&sched.ID, &sched.Name, &sched.Description, &sched.Timezone,
		&rotationsJSON, &overridesJSON, &sched.CreatedAt, &sched.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(rotationsJSON), &sched.Rotations)
	json.Unmarshal([]byte(overridesJSON), &sched.Overrides)

	return &sched, nil
}

// ListSchedules returns all on-call schedules
func (s *Store) ListSchedules() ([]OnCallSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, description, timezone, rotations, overrides, created_at, updated_at
		FROM oncall_schedules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []OnCallSchedule
	for rows.Next() {
		var sched OnCallSchedule
		var rotationsJSON, overridesJSON string
		err := rows.Scan(&sched.ID, &sched.Name, &sched.Description, &sched.Timezone,
			&rotationsJSON, &overridesJSON, &sched.CreatedAt, &sched.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(rotationsJSON), &sched.Rotations)
		json.Unmarshal([]byte(overridesJSON), &sched.Overrides)
		schedules = append(schedules, sched)
	}

	return schedules, nil
}

// GetCurrentOnCall returns who is currently on-call for a schedule
func (s *Store) GetCurrentOnCall(scheduleID string) (string, error) {
	sched, err := s.GetSchedule(scheduleID)
	if err != nil || sched == nil {
		return "", err
	}

	now := time.Now()

	// Check overrides first
	for _, override := range sched.Overrides {
		if now.After(override.StartTime) && now.Before(override.EndTime) {
			return override.User, nil
		}
	}

	// Check rotations
	for _, rotation := range sched.Rotations {
		if len(rotation.Users) == 0 {
			continue
		}

		// Simple weekly rotation calculation
		if rotation.Type == "weekly" {
			weekNum := int(now.Unix() / (7 * 24 * 60 * 60))
			userIdx := weekNum % len(rotation.Users)
			return rotation.Users[userIdx], nil
		}

		// Daily rotation
		if rotation.Type == "daily" {
			dayNum := int(now.Unix() / (24 * 60 * 60))
			userIdx := dayNum % len(rotation.Users)
			return rotation.Users[userIdx], nil
		}
	}

	// Default to first user in first rotation
	if len(sched.Rotations) > 0 && len(sched.Rotations[0].Users) > 0 {
		return sched.Rotations[0].Users[0], nil
	}

	return "", nil
}

// Escalation policy methods

// CreatePolicy creates a new escalation policy
func (s *Store) CreatePolicy(policy *EscalationPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if policy.ID == "" {
		policy.ID = uuid.New().String()
	}
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now

	rulesJSON, _ := json.Marshal(policy.Rules)

	_, err := s.db.Exec(`
		INSERT INTO escalation_policies (id, name, description, rules, repeat_after_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		policy.ID, policy.Name, policy.Description, string(rulesJSON), policy.RepeatAfter,
		policy.CreatedAt, policy.UpdatedAt,
	)
	return err
}

// GetPolicy retrieves an escalation policy
func (s *Store) GetPolicy(id string) (*EscalationPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var policy EscalationPolicy
	var rulesJSON string

	err := s.db.QueryRow(`
		SELECT id, name, description, rules, repeat_after_minutes, created_at, updated_at
		FROM escalation_policies WHERE id = ?`, id).Scan(
		&policy.ID, &policy.Name, &policy.Description, &rulesJSON, &policy.RepeatAfter,
		&policy.CreatedAt, &policy.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(rulesJSON), &policy.Rules)
	return &policy, nil
}

// ListPolicies returns all escalation policies
func (s *Store) ListPolicies() ([]EscalationPolicy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, description, rules, repeat_after_minutes, created_at, updated_at
		FROM escalation_policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []EscalationPolicy
	for rows.Next() {
		var policy EscalationPolicy
		var rulesJSON string
		err := rows.Scan(&policy.ID, &policy.Name, &policy.Description, &rulesJSON, &policy.RepeatAfter,
			&policy.CreatedAt, &policy.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(rulesJSON), &policy.Rules)
		policies = append(policies, policy)
	}

	return policies, nil
}

// Notification log methods

// LogNotification records a sent notification
func (s *Store) LogNotification(log *NotificationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.ID == "" {
		log.ID = uuid.New().String()
	}
	log.SentAt = time.Now()

	_, err := s.db.Exec(`
		INSERT INTO notification_logs (id, incident_id, channel, target, status, sent_at, message)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.IncidentID, log.Channel, log.Target, log.Status, log.SentAt, log.Message,
	)
	return err
}

// GetNotificationLogs returns notification history for an incident
func (s *Store) GetNotificationLogs(incidentID string) ([]NotificationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, incident_id, channel, target, status, sent_at, message
		FROM notification_logs WHERE incident_id = ? ORDER BY sent_at DESC`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []NotificationLog
	for rows.Next() {
		var log NotificationLog
		err := rows.Scan(&log.ID, &log.IncidentID, &log.Channel, &log.Target, &log.Status, &log.SentAt, &log.Message)
		if err != nil {
			continue
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// Helper functions

func (s *Store) scanIncident(row *sql.Row) (*Incident, error) {
	var inc Incident
	var serviceID sql.NullString
	var ackedAt, resolvedAt sql.NullTime
	var ackedBy, resolvedBy, assignedTo, tagsJSON, timelineJSON sql.NullString

	err := row.Scan(&inc.ID, &inc.Title, &inc.Description, &inc.Severity, &inc.Status,
		&inc.Service, &serviceID, &inc.Source, &inc.SourceID, &inc.CreatedAt, &inc.UpdatedAt,
		&ackedAt, &ackedBy, &resolvedAt, &resolvedBy, &assignedTo, &inc.EscLevel,
		&tagsJSON, &timelineJSON)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	inc.ServiceID = serviceID.String
	if ackedAt.Valid {
		inc.AckedAt = &ackedAt.Time
	}
	inc.AckedBy = ackedBy.String
	if resolvedAt.Valid {
		inc.ResolvedAt = &resolvedAt.Time
	}
	inc.ResolvedBy = resolvedBy.String
	inc.AssignedTo = assignedTo.String

	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &inc.Tags)
	}
	if timelineJSON.Valid {
		json.Unmarshal([]byte(timelineJSON.String), &inc.Timeline)
	}

	return &inc, nil
}

func (s *Store) scanIncidents(rows *sql.Rows) ([]Incident, error) {
	var incidents []Incident
	for rows.Next() {
		var inc Incident
		var serviceID sql.NullString
		var ackedAt, resolvedAt sql.NullTime
		var ackedBy, resolvedBy, assignedTo, tagsJSON, timelineJSON sql.NullString

		err := rows.Scan(&inc.ID, &inc.Title, &inc.Description, &inc.Severity, &inc.Status,
			&inc.Service, &serviceID, &inc.Source, &inc.SourceID, &inc.CreatedAt, &inc.UpdatedAt,
			&ackedAt, &ackedBy, &resolvedAt, &resolvedBy, &assignedTo, &inc.EscLevel,
			&tagsJSON, &timelineJSON)
		if err != nil {
			continue
		}

		inc.ServiceID = serviceID.String
		if ackedAt.Valid {
			inc.AckedAt = &ackedAt.Time
		}
		inc.AckedBy = ackedBy.String
		if resolvedAt.Valid {
			inc.ResolvedAt = &resolvedAt.Time
		}
		inc.ResolvedBy = resolvedBy.String
		inc.AssignedTo = assignedTo.String

		if tagsJSON.Valid {
			json.Unmarshal([]byte(tagsJSON.String), &inc.Tags)
		}
		if timelineJSON.Valid {
			json.Unmarshal([]byte(timelineJSON.String), &inc.Timeline)
		}

		incidents = append(incidents, inc)
	}

	return incidents, nil
}

// DeleteSchedule removes a schedule
func (s *Store) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM oncall_schedules WHERE id = ?`, id)
	return err
}

// DeletePolicy removes an escalation policy
func (s *Store) DeletePolicy(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM escalation_policies WHERE id = ?`, id)
	return err
}
