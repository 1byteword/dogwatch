package security

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Store persists security events, alerts, and rules
type Store struct {
	db     *sql.DB
	mu     sync.RWMutex
	engine *RulesEngine
}

// EventFilter contains filters for listing events
type EventFilter struct {
	Severity  string
	EventType string
	StartTime time.Time
	EndTime   time.Time
	Service   string
	SourceIP  string
	RuleID    string
	Limit     int
	Offset    int
}

// AlertFilter contains filters for listing alerts
type AlertFilter struct {
	Status    string
	Severity  string
	StartTime time.Time
	EndTime   time.Time
	RuleID    string
	Limit     int
	Offset    int
}

// SecurityStats holds dashboard statistics
type SecurityStats struct {
	TotalEvents      int                 `json:"total_events"`
	TotalAlerts      int                 `json:"total_alerts"`
	OpenAlerts       int                 `json:"open_alerts"`
	CriticalAlerts   int                 `json:"critical_alerts"`
	HighAlerts       int                 `json:"high_alerts"`
	ThreatsToday     int                 `json:"threats_today"`
	EventsByType     map[string]int      `json:"events_by_type"`
	EventsBySeverity map[string]int      `json:"events_by_severity"`
	TopSources       []SourceStats       `json:"top_sources"`
	TopTargets       []TargetStats       `json:"top_targets"`
	Timeline         []TimelineBucket    `json:"timeline"`
	TrendData        []TrendPoint        `json:"trend_data"`
	TopRules         []RuleStats         `json:"top_rules"`
}

// SourceStats represents event count from a source
type SourceStats struct {
	SourceIP string `json:"source_ip"`
	Count    int    `json:"count"`
}

// TargetStats represents event count to a target
type TargetStats struct {
	Service string `json:"service"`
	Count   int    `json:"count"`
}

// TimelineBucket represents events in a time bucket
type TimelineBucket struct {
	Timestamp time.Time      `json:"timestamp"`
	Counts    map[string]int `json:"counts"`
}

// TrendPoint represents a point in the trend chart
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Events    int       `json:"events"`
	Alerts    int       `json:"alerts"`
}

// RuleStats represents rule match statistics
type RuleStats struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Count    int    `json:"count"`
}

// MitreMapping represents MITRE ATT&CK mapping data
type MitreMapping struct {
	Tactics []MitreTacticData `json:"tactics"`
}

// MitreTacticData represents a tactic with its techniques
type MitreTacticData struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Techniques []MitreTechniqueData `json:"techniques"`
}

// MitreTechniqueData represents a technique with its count
type MitreTechniqueData struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AlertInvestigation contains investigation details
type AlertInvestigation struct {
	Alert           *SecurityAlert   `json:"alert"`
	Events          []SecurityEvent  `json:"events"`
	RelatedAlerts   []SecurityAlert  `json:"related_alerts"`
	Timeline        []TimelineEntry  `json:"timeline"`
	Recommendations []string         `json:"recommendations"`
}

// TimelineEntry represents an entry in the investigation timeline
type TimelineEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	Actor       string    `json:"actor,omitempty"`
}

// NewStore creates a new security store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{
		db:     db,
		engine: NewRulesEngine(),
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	go store.cleanupLoop()
	return store, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// initSchema creates the database tables
func (s *Store) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS security_events (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		type TEXT NOT NULL,
		severity TEXT DEFAULT '',
		title TEXT DEFAULT '',
		host_id TEXT DEFAULT '',
		hostname TEXT DEFAULT '',
		pid INTEGER DEFAULT 0,
		ppid INTEGER DEFAULT 0,
		uid INTEGER DEFAULT 0,
		gid INTEGER DEFAULT 0,
		comm TEXT DEFAULT '',
		cmdline TEXT DEFAULT '',
		exe_path TEXT DEFAULT '',
		parent_comm TEXT DEFAULT '',
		src_ip TEXT DEFAULT '',
		dst_ip TEXT DEFAULT '',
		src_port INTEGER DEFAULT 0,
		dst_port INTEGER DEFAULT 0,
		protocol TEXT DEFAULT '',
		file_path TEXT DEFAULT '',
		file_mode INTEGER DEFAULT 0,
		operation TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		container_name TEXT DEFAULT '',
		pod_name TEXT DEFAULT '',
		namespace TEXT DEFAULT '',
		image_name TEXT DEFAULT '',
		privileged INTEGER DEFAULT 0,
		rule_id TEXT DEFAULT '',
		rule_name TEXT DEFAULT '',
		mitre_tactic TEXT DEFAULT '',
		mitre_technique TEXT DEFAULT '',
		attributes TEXT DEFAULT '{}'
	);

	CREATE INDEX IF NOT EXISTS idx_events_timestamp ON security_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_events_type ON security_events(type);
	CREATE INDEX IF NOT EXISTS idx_events_severity ON security_events(severity);
	CREATE INDEX IF NOT EXISTS idx_events_src_ip ON security_events(src_ip);
	CREATE INDEX IF NOT EXISTS idx_events_container_id ON security_events(container_id);

	CREATE TABLE IF NOT EXISTS security_alerts (
		id TEXT PRIMARY KEY,
		rule_id TEXT NOT NULL,
		rule_name TEXT NOT NULL,
		severity TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'open',
		title TEXT NOT NULL,
		description TEXT DEFAULT '',
		mitre_tactic TEXT DEFAULT '',
		mitre_technique TEXT DEFAULT '',
		mitre_technique_id TEXT DEFAULT '',
		event_id TEXT DEFAULT '',
		host_id TEXT DEFAULT '',
		hostname TEXT DEFAULT '',
		container_id TEXT DEFAULT '',
		container_name TEXT DEFAULT '',
		pod_name TEXT DEFAULT '',
		namespace TEXT DEFAULT '',
		detected_at DATETIME NOT NULL,
		acknowledged_at DATETIME,
		resolved_at DATETIME,
		acknowledged_by TEXT DEFAULT '',
		resolved_by TEXT DEFAULT '',
		labels TEXT DEFAULT '{}',
		indicators TEXT DEFAULT '[]',
		notes TEXT DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_alerts_state ON security_alerts(state);
	CREATE INDEX IF NOT EXISTS idx_alerts_severity ON security_alerts(severity);
	CREATE INDEX IF NOT EXISTS idx_alerts_detected_at ON security_alerts(detected_at);
	CREATE INDEX IF NOT EXISTS idx_alerts_rule_id ON security_alerts(rule_id);

	CREATE TABLE IF NOT EXISTS security_rules (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		enabled INTEGER DEFAULT 1,
		type TEXT NOT NULL,
		severity TEXT NOT NULL,
		mitre_tactic TEXT DEFAULT '',
		mitre_technique TEXT DEFAULT '',
		mitre_technique_id TEXT DEFAULT '',
		conditions TEXT DEFAULT '[]',
		tags TEXT DEFAULT '[]',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		created_by TEXT DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_rules_enabled ON security_rules(enabled);
	CREATE INDEX IF NOT EXISTS idx_rules_type ON security_rules(type);
	`

	_, err := s.db.Exec(schema)
	return err
}

// StoreEvent stores a security event
func (s *Store) StoreEvent(event *SecurityEvent) error {
	attributes, _ := json.Marshal(event.Attributes)

	_, err := s.db.Exec(`
		INSERT INTO security_events (
			id, timestamp, type, host_id, hostname,
			pid, ppid, uid, gid, comm, cmdline, exe_path, parent_comm,
			src_ip, dst_ip, src_port, dst_port, protocol,
			file_path, file_mode, operation,
			container_id, container_name, pod_name, namespace, image_name, privileged,
			attributes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		event.ID, event.Timestamp, event.Type, event.HostID, event.Hostname,
		event.PID, event.PPID, event.UID, event.GID, event.Comm, event.Cmdline, event.ExePath, event.ParentComm,
		event.SrcIP, event.DstIP, event.SrcPort, event.DstPort, event.Protocol,
		event.FilePath, event.FileMode, event.Operation,
		event.ContainerID, event.ContainerName, event.PodName, event.Namespace, event.ImageName, event.Privileged,
		string(attributes),
	)
	return err
}

// ListEvents returns events matching the filter
func (s *Store) ListEvents(filter EventFilter) ([]SecurityEvent, error) {
	query := `SELECT
		id, timestamp, type, host_id, hostname,
		pid, ppid, uid, gid, comm, cmdline, exe_path, parent_comm,
		src_ip, dst_ip, src_port, dst_port, protocol,
		file_path, file_mode, operation,
		container_id, container_name, pod_name, namespace, image_name, privileged,
		COALESCE(rule_id, '') as rule_id, COALESCE(rule_name, '') as rule_name,
		COALESCE(mitre_tactic, '') as mitre_tactic, COALESCE(mitre_technique, '') as mitre_technique,
		attributes
	FROM security_events WHERE 1=1`

	args := []interface{}{}

	if filter.EventType != "" {
		query += " AND type = ?"
		args = append(args, filter.EventType)
	}

	if !filter.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filter.StartTime)
	}

	if !filter.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime)
	}

	if filter.SourceIP != "" {
		query += " AND src_ip = ?"
		args = append(args, filter.SourceIP)
	}

	if filter.RuleID != "" {
		query += " AND rule_id = ?"
		args = append(args, filter.RuleID)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		var attributesJSON, ruleID, ruleName, mitreTactic, mitreTechnique string
		var privileged int

		err := rows.Scan(
			&e.ID, &e.Timestamp, &e.Type, &e.HostID, &e.Hostname,
			&e.PID, &e.PPID, &e.UID, &e.GID, &e.Comm, &e.Cmdline, &e.ExePath, &e.ParentComm,
			&e.SrcIP, &e.DstIP, &e.SrcPort, &e.DstPort, &e.Protocol,
			&e.FilePath, &e.FileMode, &e.Operation,
			&e.ContainerID, &e.ContainerName, &e.PodName, &e.Namespace, &e.ImageName, &privileged,
			&ruleID, &ruleName, &mitreTactic, &mitreTechnique,
			&attributesJSON,
		)
		if err != nil {
			continue
		}
		e.Privileged = privileged == 1
		if attributesJSON != "" {
			json.Unmarshal([]byte(attributesJSON), &e.Attributes)
		}
		events = append(events, e)
	}

	return events, nil
}

// StoreAlert stores a security alert
func (s *Store) StoreAlert(alert *SecurityAlert) error {
	labels, _ := json.Marshal(alert.Labels)
	indicators, _ := json.Marshal(alert.Indicators)

	_, err := s.db.Exec(`
		INSERT INTO security_alerts (
			id, rule_id, rule_name, severity, state, title, description,
			mitre_tactic, mitre_technique, mitre_technique_id,
			event_id, host_id, hostname,
			container_id, container_name, pod_name, namespace,
			detected_at, labels, indicators, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		alert.ID, alert.RuleID, alert.RuleName, alert.Severity, alert.State, alert.Title, alert.Description,
		alert.MitreTactic, alert.MitreTechnique, alert.MitreTechniqueID,
		alert.EventID, alert.HostID, alert.Hostname,
		alert.ContainerID, alert.ContainerName, alert.PodName, alert.Namespace,
		alert.DetectedAt, string(labels), string(indicators), alert.Notes,
	)
	return err
}

// ListAlerts returns alerts matching the filter
func (s *Store) ListAlerts(filter AlertFilter) ([]SecurityAlert, error) {
	query := `SELECT
		id, rule_id, rule_name, severity, state, title, description,
		mitre_tactic, mitre_technique, mitre_technique_id,
		event_id, host_id, hostname,
		container_id, container_name, pod_name, namespace,
		detected_at, acknowledged_at, resolved_at,
		acknowledged_by, resolved_by, labels, indicators, notes
	FROM security_alerts WHERE 1=1`

	args := []interface{}{}

	if filter.Status != "" {
		query += " AND state = ?"
		args = append(args, filter.Status)
	}

	if filter.Severity != "" {
		query += " AND severity = ?"
		args = append(args, filter.Severity)
	}

	if !filter.StartTime.IsZero() {
		query += " AND detected_at >= ?"
		args = append(args, filter.StartTime)
	}

	if !filter.EndTime.IsZero() {
		query += " AND detected_at <= ?"
		args = append(args, filter.EndTime)
	}

	if filter.RuleID != "" {
		query += " AND rule_id = ?"
		args = append(args, filter.RuleID)
	}

	query += " ORDER BY detected_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []SecurityAlert
	for rows.Next() {
		var a SecurityAlert
		var acknowledgedAt, resolvedAt sql.NullTime
		var labelsJSON, indicatorsJSON string

		err := rows.Scan(
			&a.ID, &a.RuleID, &a.RuleName, &a.Severity, &a.State, &a.Title, &a.Description,
			&a.MitreTactic, &a.MitreTechnique, &a.MitreTechniqueID,
			&a.EventID, &a.HostID, &a.Hostname,
			&a.ContainerID, &a.ContainerName, &a.PodName, &a.Namespace,
			&a.DetectedAt, &acknowledgedAt, &resolvedAt,
			&a.AcknowledgedBy, &a.ResolvedBy, &labelsJSON, &indicatorsJSON, &a.Notes,
		)
		if err != nil {
			continue
		}

		if acknowledgedAt.Valid {
			a.AcknowledgedAt = &acknowledgedAt.Time
		}
		if resolvedAt.Valid {
			a.ResolvedAt = &resolvedAt.Time
		}
		json.Unmarshal([]byte(labelsJSON), &a.Labels)
		json.Unmarshal([]byte(indicatorsJSON), &a.Indicators)

		alerts = append(alerts, a)
	}

	return alerts, nil
}

// GetAlert returns a specific alert
func (s *Store) GetAlert(id string) (*SecurityAlert, error) {
	var a SecurityAlert
	var acknowledgedAt, resolvedAt sql.NullTime
	var labelsJSON, indicatorsJSON string

	err := s.db.QueryRow(`
		SELECT
			id, rule_id, rule_name, severity, state, title, description,
			mitre_tactic, mitre_technique, mitre_technique_id,
			event_id, host_id, hostname,
			container_id, container_name, pod_name, namespace,
			detected_at, acknowledged_at, resolved_at,
			acknowledged_by, resolved_by, labels, indicators, notes
		FROM security_alerts WHERE id = ?
	`, id).Scan(
		&a.ID, &a.RuleID, &a.RuleName, &a.Severity, &a.State, &a.Title, &a.Description,
		&a.MitreTactic, &a.MitreTechnique, &a.MitreTechniqueID,
		&a.EventID, &a.HostID, &a.Hostname,
		&a.ContainerID, &a.ContainerName, &a.PodName, &a.Namespace,
		&a.DetectedAt, &acknowledgedAt, &resolvedAt,
		&a.AcknowledgedBy, &a.ResolvedBy, &labelsJSON, &indicatorsJSON, &a.Notes,
	)
	if err != nil {
		return nil, err
	}

	if acknowledgedAt.Valid {
		a.AcknowledgedAt = &acknowledgedAt.Time
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	json.Unmarshal([]byte(labelsJSON), &a.Labels)
	json.Unmarshal([]byte(indicatorsJSON), &a.Indicators)

	return &a, nil
}

// AcknowledgeAlert acknowledges an alert
func (s *Store) AcknowledgeAlert(id, userID, comment string) error {
	now := time.Now()
	notes := ""
	if comment != "" {
		notes = fmt.Sprintf("\nAcknowledged: %s", comment)
	}

	_, err := s.db.Exec(`
		UPDATE security_alerts
		SET state = 'acknowledged', acknowledged_at = ?, acknowledged_by = ?, notes = COALESCE(notes, '') || ?
		WHERE id = ?
	`, now, userID, notes, id)
	return err
}

// ResolveAlert resolves an alert
func (s *Store) ResolveAlert(id, userID, resolution string) error {
	now := time.Now()

	_, err := s.db.Exec(`
		UPDATE security_alerts
		SET state = 'resolved', resolved_at = ?, resolved_by = ?, notes = COALESCE(notes, '') || ?
		WHERE id = ?
	`, now, userID, "\nResolution: "+resolution, id)
	return err
}

// GetAlertInvestigation returns investigation details for an alert
func (s *Store) GetAlertInvestigation(alertID string) (*AlertInvestigation, error) {
	alert, err := s.GetAlert(alertID)
	if err != nil {
		return nil, err
	}

	// Get related events (events from same host/container within 1 hour)
	eventFilter := EventFilter{
		StartTime: alert.DetectedAt.Add(-1 * time.Hour),
		EndTime:   alert.DetectedAt.Add(1 * time.Hour),
		Limit:     50,
	}
	events, _ := s.ListEvents(eventFilter)

	// Get related alerts (same rule or same host)
	relatedFilter := AlertFilter{
		RuleID: alert.RuleID,
		Limit:  10,
	}
	relatedAlerts, _ := s.ListAlerts(relatedFilter)

	// Build timeline
	timeline := []TimelineEntry{
		{
			Timestamp:   alert.DetectedAt,
			Action:      "detected",
			Description: "Alert detected",
		},
	}
	if alert.AcknowledgedAt != nil {
		timeline = append(timeline, TimelineEntry{
			Timestamp:   *alert.AcknowledgedAt,
			Action:      "acknowledged",
			Description: "Alert acknowledged",
			Actor:       alert.AcknowledgedBy,
		})
	}
	if alert.ResolvedAt != nil {
		timeline = append(timeline, TimelineEntry{
			Timestamp:   *alert.ResolvedAt,
			Action:      "resolved",
			Description: "Alert resolved",
			Actor:       alert.ResolvedBy,
		})
	}

	// Generate recommendations based on alert type
	recommendations := s.getRecommendations(alert)

	return &AlertInvestigation{
		Alert:           alert,
		Events:          events,
		RelatedAlerts:   relatedAlerts,
		Timeline:        timeline,
		Recommendations: recommendations,
	}, nil
}

func (s *Store) getRecommendations(alert *SecurityAlert) []string {
	recommendations := []string{}

	switch alert.RuleID {
	case "shell_in_container":
		recommendations = append(recommendations,
			"Investigate why a shell was spawned in the container",
			"Check if this is expected debug activity",
			"Review container security policies",
			"Consider using distroless or minimal images",
		)
	case "cryptominer_process":
		recommendations = append(recommendations,
			"IMMEDIATE: Kill the mining process",
			"Investigate how the miner was deployed",
			"Check for other compromised containers/hosts",
			"Rotate all credentials on affected systems",
			"Scan container images for vulnerabilities",
		)
	case "reverse_shell":
		recommendations = append(recommendations,
			"IMMEDIATE: Isolate the affected system",
			"Investigate the initial access vector",
			"Check for persistence mechanisms",
			"Review all network connections from the host",
			"Rotate all credentials",
		)
	case "privileged_container":
		recommendations = append(recommendations,
			"Review if privileged mode is necessary",
			"Consider using specific capabilities instead",
			"Implement Pod Security Policies/Standards",
		)
	case "sensitive_file_access":
		recommendations = append(recommendations,
			"Identify the process accessing sensitive files",
			"Verify if access is legitimate",
			"Review file permissions",
			"Consider using secrets management",
		)
	default:
		recommendations = append(recommendations,
			"Investigate the triggering event",
			"Review related events and alerts",
			"Check for indicators of compromise",
		)
	}

	return recommendations
}

// ListRules returns detection rules
func (s *Store) ListRules(enabled, category string) ([]*ThreatRule, error) {
	// Return built-in rules plus any custom rules from DB
	rules := s.engine.GetRules()

	// Filter if needed
	if enabled != "" {
		wantEnabled := enabled == "true"
		filtered := make([]*ThreatRule, 0)
		for _, r := range rules {
			if r.Enabled == wantEnabled {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	if category != "" {
		filtered := make([]*ThreatRule, 0)
		for _, r := range rules {
			if string(r.Type) == category {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	return rules, nil
}

// GetRule returns a specific rule
func (s *Store) GetRule(id string) (*ThreatRule, error) {
	rule := s.engine.GetRule(id)
	if rule == nil {
		return nil, fmt.Errorf("rule not found")
	}
	return rule, nil
}

// CreateRule creates a custom detection rule
func (s *Store) CreateRule(rule *ThreatRule) error {
	conditions, _ := json.Marshal(rule.Conditions)
	tags, _ := json.Marshal(rule.Tags)

	_, err := s.db.Exec(`
		INSERT INTO security_rules (
			id, name, description, enabled, type, severity,
			mitre_tactic, mitre_technique, mitre_technique_id,
			conditions, tags, created_at, updated_at, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		rule.ID, rule.Name, rule.Description, rule.Enabled, rule.Type, rule.Severity,
		rule.MitreTactic, rule.MitreTechnique, rule.MitreTechniqueID,
		string(conditions), string(tags), time.Now(), time.Now(), "",
	)

	if err == nil {
		// Add to engine
		s.engine.AddRule(rule)
	}

	return err
}

// UpdateRule updates a detection rule
func (s *Store) UpdateRule(rule *ThreatRule) error {
	conditions, _ := json.Marshal(rule.Conditions)
	tags, _ := json.Marshal(rule.Tags)

	_, err := s.db.Exec(`
		UPDATE security_rules SET
			name = ?, description = ?, enabled = ?, type = ?, severity = ?,
			mitre_tactic = ?, mitre_technique = ?, mitre_technique_id = ?,
			conditions = ?, tags = ?, updated_at = ?
		WHERE id = ?
	`,
		rule.Name, rule.Description, rule.Enabled, rule.Type, rule.Severity,
		rule.MitreTactic, rule.MitreTechnique, rule.MitreTechniqueID,
		string(conditions), string(tags), time.Now(),
		rule.ID,
	)

	if err == nil {
		// Update in engine
		if r := s.engine.GetRule(rule.ID); r != nil {
			r.Name = rule.Name
			r.Description = rule.Description
			r.Enabled = rule.Enabled
			r.Severity = rule.Severity
			r.Conditions = rule.Conditions
		}
	}

	return err
}

// DeleteRule deletes a detection rule
func (s *Store) DeleteRule(id string) error {
	_, err := s.db.Exec("DELETE FROM security_rules WHERE id = ?", id)
	return err
}

// GetStats returns dashboard statistics
func (s *Store) GetStats(hours int) (*SecurityStats, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	stats := &SecurityStats{
		EventsByType:     make(map[string]int),
		EventsBySeverity: make(map[string]int),
		TopSources:       make([]SourceStats, 0),
		TopTargets:       make([]TargetStats, 0),
		Timeline:         make([]TimelineBucket, 0),
		TrendData:        make([]TrendPoint, 0),
		TopRules:         make([]RuleStats, 0),
	}

	// Total events
	s.db.QueryRow("SELECT COUNT(*) FROM security_events WHERE timestamp >= ?", since).Scan(&stats.TotalEvents)

	// Total alerts
	s.db.QueryRow("SELECT COUNT(*) FROM security_alerts WHERE detected_at >= ?", since).Scan(&stats.TotalAlerts)

	// Open alerts
	s.db.QueryRow("SELECT COUNT(*) FROM security_alerts WHERE state = 'open'").Scan(&stats.OpenAlerts)

	// Critical alerts
	s.db.QueryRow("SELECT COUNT(*) FROM security_alerts WHERE severity = 'critical' AND state = 'open'").Scan(&stats.CriticalAlerts)

	// High alerts
	s.db.QueryRow("SELECT COUNT(*) FROM security_alerts WHERE severity = 'high' AND state = 'open'").Scan(&stats.HighAlerts)

	// Threats today
	today := time.Now().Truncate(24 * time.Hour)
	s.db.QueryRow("SELECT COUNT(*) FROM security_alerts WHERE detected_at >= ?", today).Scan(&stats.ThreatsToday)

	// Events by type
	rows, err := s.db.Query("SELECT type, COUNT(*) FROM security_events WHERE timestamp >= ? GROUP BY type", since)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t string
			var c int
			if rows.Scan(&t, &c) == nil {
				stats.EventsByType[t] = c
			}
		}
	}

	// Events by severity (from alerts)
	rows2, err := s.db.Query("SELECT severity, COUNT(*) FROM security_alerts WHERE detected_at >= ? GROUP BY severity", since)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var sev string
			var c int
			if rows2.Scan(&sev, &c) == nil {
				stats.EventsBySeverity[sev] = c
			}
		}
	}

	// Top sources
	rows3, err := s.db.Query(`
		SELECT src_ip, COUNT(*) as cnt
		FROM security_events
		WHERE timestamp >= ? AND src_ip != ''
		GROUP BY src_ip
		ORDER BY cnt DESC
		LIMIT 10
	`, since)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var ss SourceStats
			if rows3.Scan(&ss.SourceIP, &ss.Count) == nil {
				stats.TopSources = append(stats.TopSources, ss)
			}
		}
	}

	// Top rules
	rows4, err := s.db.Query(`
		SELECT rule_id, rule_name, COUNT(*) as cnt
		FROM security_alerts
		WHERE detected_at >= ?
		GROUP BY rule_id, rule_name
		ORDER BY cnt DESC
		LIMIT 10
	`, since)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var r RuleStats
			if rows4.Scan(&r.RuleID, &r.RuleName, &r.Count) == nil {
				stats.TopRules = append(stats.TopRules, r)
			}
		}
	}

	return stats, nil
}

// GetMitreMapping returns MITRE ATT&CK mapping data
func (s *Store) GetMitreMapping(hours int) (*MitreMapping, error) {
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Get technique counts from alerts
	techniqueMap := make(map[string]map[string]int) // tactic -> technique -> count

	rows, err := s.db.Query(`
		SELECT mitre_tactic, mitre_technique, COUNT(*) as cnt
		FROM security_alerts
		WHERE detected_at >= ? AND mitre_tactic != ''
		GROUP BY mitre_tactic, mitre_technique
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tactic, technique string
		var count int
		if rows.Scan(&tactic, &technique, &count) == nil {
			if techniqueMap[tactic] == nil {
				techniqueMap[tactic] = make(map[string]int)
			}
			techniqueMap[tactic][technique] = count
		}
	}

	// Build MITRE mapping structure
	// Using common MITRE ATT&CK tactics
	tacticOrder := []string{
		"Initial Access", "Execution", "Persistence", "Privilege Escalation",
		"Defense Evasion", "Credential Access", "Discovery", "Lateral Movement",
		"Collection", "Command and Control", "Exfiltration", "Impact",
	}

	mapping := &MitreMapping{
		Tactics: make([]MitreTacticData, 0),
	}

	for _, tacticName := range tacticOrder {
		techniques, ok := techniqueMap[tacticName]
		if !ok {
			continue
		}

		tactic := MitreTacticData{
			ID:         tacticToID(tacticName),
			Name:       tacticName,
			Techniques: make([]MitreTechniqueData, 0),
		}

		for techName, count := range techniques {
			tactic.Techniques = append(tactic.Techniques, MitreTechniqueData{
				Name:  techName,
				Count: count,
			})
		}

		mapping.Tactics = append(mapping.Tactics, tactic)
	}

	return mapping, nil
}

func tacticToID(name string) string {
	ids := map[string]string{
		"Initial Access":       "TA0001",
		"Execution":            "TA0002",
		"Persistence":          "TA0003",
		"Privilege Escalation": "TA0004",
		"Defense Evasion":      "TA0005",
		"Credential Access":    "TA0006",
		"Discovery":            "TA0007",
		"Lateral Movement":     "TA0008",
		"Collection":           "TA0009",
		"Command and Control":  "TA0011",
		"Exfiltration":         "TA0010",
		"Impact":               "TA0040",
	}
	if id, ok := ids[name]; ok {
		return id
	}
	return ""
}

// GetEngine returns the rules engine
func (s *Store) GetEngine() *RulesEngine {
	return s.engine
}

// RecordEvent stores a security event (alias for StoreEvent for detector compatibility)
func (s *Store) RecordEvent(event *SecurityEvent) error {
	return s.StoreEvent(event)
}

// RecordAlert stores a security alert (alias for StoreAlert for detector compatibility)
func (s *Store) RecordAlert(alert *SecurityAlert) error {
	return s.StoreAlert(alert)
}

// ListOpenAlerts returns all open alerts
func (s *Store) ListOpenAlerts() ([]SecurityAlert, error) {
	return s.ListAlerts(AlertFilter{Status: "open"})
}

// GetAlertSummary returns aggregated statistics (alias for GetStats for detector compatibility)
func (s *Store) GetAlertSummary(since time.Duration) (*AlertSummary, error) {
	hours := int(since.Hours())
	if hours < 1 {
		hours = 24
	}
	stats, err := s.GetStats(hours)
	if err != nil {
		return nil, err
	}

	summary := &AlertSummary{
		TotalAlerts:       stats.TotalAlerts,
		OpenAlerts:        stats.OpenAlerts,
		CriticalCount:     stats.CriticalAlerts,
		HighCount:         stats.HighAlerts,
		AlertsByRule:      make(map[string]int),
		AlertsByHost:      make(map[string]int),
		AlertsByContainer: make(map[string]int),
	}

	for _, r := range stats.TopRules {
		summary.AlertsByRule[r.RuleName] = r.Count
	}

	return summary, nil
}

// GetEvent retrieves an event by ID
func (s *Store) GetEvent(id string) (*SecurityEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var e SecurityEvent
	var attributesJSON string
	var privileged int

	err := s.db.QueryRow(`
		SELECT id, timestamp, type, host_id, hostname,
			   pid, ppid, uid, gid, comm, cmdline, exe_path, parent_comm,
			   src_ip, dst_ip, src_port, dst_port, protocol,
			   file_path, file_mode, operation,
			   container_id, container_name, pod_name, namespace, image_name, privileged,
			   attributes
		FROM security_events WHERE id = ?
	`, id).Scan(
		&e.ID, &e.Timestamp, &e.Type, &e.HostID, &e.Hostname,
		&e.PID, &e.PPID, &e.UID, &e.GID, &e.Comm, &e.Cmdline, &e.ExePath, &e.ParentComm,
		&e.SrcIP, &e.DstIP, &e.SrcPort, &e.DstPort, &e.Protocol,
		&e.FilePath, &e.FileMode, &e.Operation,
		&e.ContainerID, &e.ContainerName, &e.PodName, &e.Namespace, &e.ImageName, &privileged,
		&attributesJSON,
	)
	if err != nil {
		return nil, err
	}

	e.Privileged = privileged == 1
	if attributesJSON != "" {
		json.Unmarshal([]byte(attributesJSON), &e.Attributes)
	}

	return &e, nil
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.cleanup()
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep 7 days of events, 30 days of alerts
	eventCutoff := time.Now().Add(-7 * 24 * time.Hour)
	alertCutoff := time.Now().Add(-30 * 24 * time.Hour)

	s.db.Exec(`DELETE FROM security_events WHERE timestamp < ?`, eventCutoff)
	s.db.Exec(`DELETE FROM security_alerts WHERE detected_at < ? AND state IN ('resolved', 'false_positive')`, alertCutoff)
}

// MarkFalsePositive marks an alert as a false positive
func (s *Store) MarkFalsePositive(id, userID, notes string) error {
	now := time.Now()
	_, err := s.db.Exec(`
		UPDATE security_alerts
		SET state = 'false_positive', resolved_at = ?, resolved_by = ?, notes = COALESCE(notes, '') || ?
		WHERE id = ?
	`, now, userID, "\nFalse Positive: "+notes, id)
	return err
}
