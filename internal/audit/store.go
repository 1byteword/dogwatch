// Package audit provides audit logging for tracking user actions and changes
package audit

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store provides audit log persistence
type Store struct {
	db *sql.DB
}

// ActionType represents the type of action being audited
type ActionType string

const (
	ActionCreate ActionType = "create"
	ActionUpdate ActionType = "update"
	ActionDelete ActionType = "delete"
	ActionRead   ActionType = "read"
	ActionLogin  ActionType = "login"
	ActionLogout ActionType = "logout"
	ActionExport ActionType = "export"
	ActionImport ActionType = "import"
)

// ResourceType represents the type of resource being accessed
type ResourceType string

const (
	ResourceUser        ResourceType = "user"
	ResourceTeam        ResourceType = "team"
	ResourceOrg         ResourceType = "organization"
	ResourceAPIKey      ResourceType = "api_key"
	ResourceDashboard   ResourceType = "dashboard"
	ResourceAlert       ResourceType = "alert"
	ResourceAlertRule   ResourceType = "alert_rule"
	ResourceSilence     ResourceType = "silence"
	ResourceNotifyChannel ResourceType = "notify_channel"
	ResourceIncident    ResourceType = "incident"
	ResourceSchedule    ResourceType = "schedule"
	ResourcePolicy      ResourceType = "policy"
	ResourceWatch       ResourceType = "watch"
	ResourceSynthetic   ResourceType = "synthetic"
	ResourceSLO         ResourceType = "slo"
	ResourceDeployment  ResourceType = "deployment"
	ResourceSettings    ResourceType = "settings"
	ResourceSession     ResourceType = "session"
)

// Outcome represents the result of an action
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeDenied  Outcome = "denied"
)

// AuditLog represents a single audit log entry
type AuditLog struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	OrgID        string                 `json:"org_id,omitempty"`
	UserID       string                 `json:"user_id,omitempty"`
	UserEmail    string                 `json:"user_email,omitempty"`
	UserIP       string                 `json:"user_ip,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	Action       ActionType             `json:"action"`
	ResourceType ResourceType           `json:"resource_type"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	ResourceName string                 `json:"resource_name,omitempty"`
	Outcome      Outcome                `json:"outcome"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Changes      *ChangeSet             `json:"changes,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
	SessionID    string                 `json:"session_id,omitempty"`
}

// ChangeSet represents before/after values for updates
type ChangeSet struct {
	Before map[string]interface{} `json:"before,omitempty"`
	After  map[string]interface{} `json:"after,omitempty"`
}

// NewStore creates a new audit store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
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
	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		org_id TEXT,
		user_id TEXT,
		user_email TEXT,
		user_ip TEXT,
		user_agent TEXT,
		action TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT,
		resource_name TEXT,
		outcome TEXT NOT NULL,
		details TEXT,
		changes TEXT,
		error_message TEXT,
		request_id TEXT,
		session_id TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_logs(org_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type, resource_id);
	CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_logs(action, timestamp);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Log records an audit log entry
func (s *Store) Log(entry *AuditLog) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var detailsJSON, changesJSON []byte
	var err error

	if entry.Details != nil {
		detailsJSON, err = json.Marshal(entry.Details)
		if err != nil {
			return fmt.Errorf("failed to marshal details: %w", err)
		}
	}

	if entry.Changes != nil {
		changesJSON, err = json.Marshal(entry.Changes)
		if err != nil {
			return fmt.Errorf("failed to marshal changes: %w", err)
		}
	}

	_, err = s.db.Exec(`
		INSERT INTO audit_logs (
			id, timestamp, org_id, user_id, user_email, user_ip, user_agent,
			action, resource_type, resource_id, resource_name, outcome,
			details, changes, error_message, request_id, session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.OrgID, entry.UserID, entry.UserEmail,
		entry.UserIP, entry.UserAgent, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.ResourceName, entry.Outcome,
		string(detailsJSON), string(changesJSON), entry.ErrorMessage,
		entry.RequestID, entry.SessionID,
	)

	return err
}

// QueryOptions for listing audit logs
type QueryOptions struct {
	OrgID        string
	UserID       string
	Action       ActionType
	ResourceType ResourceType
	ResourceID   string
	Outcome      Outcome
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}

// List retrieves audit logs matching the query options
func (s *Store) List(opts QueryOptions) ([]*AuditLog, error) {
	query := `SELECT
		id, timestamp, org_id, user_id, user_email, user_ip, user_agent,
		action, resource_type, resource_id, resource_name, outcome,
		details, changes, error_message, request_id, session_id
		FROM audit_logs WHERE 1=1`

	args := []interface{}{}

	if opts.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, opts.OrgID)
	}
	if opts.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, opts.UserID)
	}
	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}
	if opts.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, opts.ResourceType)
	}
	if opts.ResourceID != "" {
		query += " AND resource_id = ?"
		args = append(args, opts.ResourceID)
	}
	if opts.Outcome != "" {
		query += " AND outcome = ?"
		args = append(args, opts.Outcome)
	}
	if opts.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *opts.StartTime)
	}
	if opts.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *opts.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	} else {
		query += " LIMIT 100"
	}
	if opts.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		log := &AuditLog{}
		var detailsJSON, changesJSON sql.NullString
		var orgID, userID, userEmail, userIP, userAgent sql.NullString
		var resourceID, resourceName, errorMsg, requestID, sessionID sql.NullString

		err := rows.Scan(
			&log.ID, &log.Timestamp, &orgID, &userID, &userEmail,
			&userIP, &userAgent, &log.Action, &log.ResourceType,
			&resourceID, &resourceName, &log.Outcome,
			&detailsJSON, &changesJSON, &errorMsg, &requestID, &sessionID,
		)
		if err != nil {
			return nil, err
		}

		log.OrgID = orgID.String
		log.UserID = userID.String
		log.UserEmail = userEmail.String
		log.UserIP = userIP.String
		log.UserAgent = userAgent.String
		log.ResourceID = resourceID.String
		log.ResourceName = resourceName.String
		log.ErrorMessage = errorMsg.String
		log.RequestID = requestID.String
		log.SessionID = sessionID.String

		if detailsJSON.Valid && detailsJSON.String != "" {
			json.Unmarshal([]byte(detailsJSON.String), &log.Details)
		}
		if changesJSON.Valid && changesJSON.String != "" {
			log.Changes = &ChangeSet{}
			json.Unmarshal([]byte(changesJSON.String), log.Changes)
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// Get retrieves a single audit log by ID
func (s *Store) Get(id string) (*AuditLog, error) {
	logs, err := s.List(QueryOptions{Limit: 1})
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRow(`SELECT
		id, timestamp, org_id, user_id, user_email, user_ip, user_agent,
		action, resource_type, resource_id, resource_name, outcome,
		details, changes, error_message, request_id, session_id
		FROM audit_logs WHERE id = ?`, id)

	log := &AuditLog{}
	var detailsJSON, changesJSON sql.NullString
	var orgID, userID, userEmail, userIP, userAgent sql.NullString
	var resourceID, resourceName, errorMsg, requestID, sessionID sql.NullString

	err = row.Scan(
		&log.ID, &log.Timestamp, &orgID, &userID, &userEmail,
		&userIP, &userAgent, &log.Action, &log.ResourceType,
		&resourceID, &resourceName, &log.Outcome,
		&detailsJSON, &changesJSON, &errorMsg, &requestID, &sessionID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	log.OrgID = orgID.String
	log.UserID = userID.String
	log.UserEmail = userEmail.String
	log.UserIP = userIP.String
	log.UserAgent = userAgent.String
	log.ResourceID = resourceID.String
	log.ResourceName = resourceName.String
	log.ErrorMessage = errorMsg.String
	log.RequestID = requestID.String
	log.SessionID = sessionID.String

	if detailsJSON.Valid && detailsJSON.String != "" {
		json.Unmarshal([]byte(detailsJSON.String), &log.Details)
	}
	if changesJSON.Valid && changesJSON.String != "" {
		log.Changes = &ChangeSet{}
		json.Unmarshal([]byte(changesJSON.String), log.Changes)
	}

	_ = logs // silence unused variable warning from incorrect code above
	return log, nil
}

// Count returns the total number of logs matching the query
func (s *Store) Count(opts QueryOptions) (int64, error) {
	query := `SELECT COUNT(*) FROM audit_logs WHERE 1=1`
	args := []interface{}{}

	if opts.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, opts.OrgID)
	}
	if opts.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, opts.UserID)
	}
	if opts.Action != "" {
		query += " AND action = ?"
		args = append(args, opts.Action)
	}
	if opts.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, opts.ResourceType)
	}
	if opts.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *opts.StartTime)
	}
	if opts.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *opts.EndTime)
	}

	var count int64
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// Cleanup removes audit logs older than the specified duration
func (s *Store) Cleanup(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := s.db.Exec(`DELETE FROM audit_logs WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Stats returns audit log statistics
type AuditStats struct {
	TotalLogs      int64            `json:"total_logs"`
	LogsToday      int64            `json:"logs_today"`
	LogsThisWeek   int64            `json:"logs_this_week"`
	ByAction       map[string]int64 `json:"by_action"`
	ByResource     map[string]int64 `json:"by_resource"`
	ByOutcome      map[string]int64 `json:"by_outcome"`
	TopUsers       []UserActivity   `json:"top_users"`
	RecentFailures int64            `json:"recent_failures"`
}

// UserActivity represents a user's activity count
type UserActivity struct {
	UserID    string `json:"user_id"`
	UserEmail string `json:"user_email"`
	Count     int64  `json:"count"`
}

// GetStats returns audit log statistics
func (s *Store) GetStats(orgID string) (*AuditStats, error) {
	stats := &AuditStats{
		ByAction:   make(map[string]int64),
		ByResource: make(map[string]int64),
		ByOutcome:  make(map[string]int64),
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -7)

	baseWhere := "WHERE 1=1"
	args := []interface{}{}
	if orgID != "" {
		baseWhere += " AND org_id = ?"
		args = append(args, orgID)
	}

	// Total logs
	s.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+baseWhere, args...).Scan(&stats.TotalLogs)

	// Logs today
	todayArgs := append(args, todayStart)
	s.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+baseWhere+" AND timestamp >= ?", todayArgs...).Scan(&stats.LogsToday)

	// Logs this week
	weekArgs := append(args, weekStart)
	s.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+baseWhere+" AND timestamp >= ?", weekArgs...).Scan(&stats.LogsThisWeek)

	// By action
	rows, _ := s.db.Query("SELECT action, COUNT(*) FROM audit_logs "+baseWhere+" GROUP BY action", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var action string
			var count int64
			rows.Scan(&action, &count)
			stats.ByAction[action] = count
		}
	}

	// By resource
	rows, _ = s.db.Query("SELECT resource_type, COUNT(*) FROM audit_logs "+baseWhere+" GROUP BY resource_type", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var resource string
			var count int64
			rows.Scan(&resource, &count)
			stats.ByResource[resource] = count
		}
	}

	// By outcome
	rows, _ = s.db.Query("SELECT outcome, COUNT(*) FROM audit_logs "+baseWhere+" GROUP BY outcome", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var outcome string
			var count int64
			rows.Scan(&outcome, &count)
			stats.ByOutcome[outcome] = count
		}
	}

	// Top users
	rows, _ = s.db.Query(`
		SELECT user_id, user_email, COUNT(*) as cnt
		FROM audit_logs `+baseWhere+` AND user_id != ''
		GROUP BY user_id ORDER BY cnt DESC LIMIT 10`, args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var ua UserActivity
			rows.Scan(&ua.UserID, &ua.UserEmail, &ua.Count)
			stats.TopUsers = append(stats.TopUsers, ua)
		}
	}

	// Recent failures (last 24 hours)
	failArgs := append(args, todayStart.Add(-24*time.Hour))
	s.db.QueryRow("SELECT COUNT(*) FROM audit_logs "+baseWhere+" AND outcome = 'failure' AND timestamp >= ?", failArgs...).Scan(&stats.RecentFailures)

	return stats, nil
}
