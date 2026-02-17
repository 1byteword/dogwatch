// Package audit provides audit logging for tracking user actions and changes
package audit

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	db, err := storage.OpenDB(dbPath)
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

	-- Query audit table for compliance tracking
	CREATE TABLE IF NOT EXISTS query_audit (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		user_id TEXT,
		username TEXT,
		session_id TEXT,
		ip_address TEXT,
		user_agent TEXT,
		org_id TEXT,
		query_type TEXT NOT NULL,
		query_text TEXT NOT NULL,
		data_source TEXT,
		time_range_start DATETIME,
		time_range_end DATETIME,
		rows_returned INTEGER DEFAULT 0,
		duration_ns INTEGER DEFAULT 0,
		duration_ms REAL DEFAULT 0,
		success INTEGER DEFAULT 1,
		error_message TEXT,
		accessed_pii INTEGER DEFAULT 0,
		pii_types_accessed TEXT,
		services_accessed TEXT,
		request_id TEXT,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_query_audit_timestamp ON query_audit(timestamp);
	CREATE INDEX IF NOT EXISTS idx_query_audit_user ON query_audit(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_query_audit_org ON query_audit(org_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_query_audit_type ON query_audit(query_type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_query_audit_pii ON query_audit(accessed_pii, timestamp);

	-- Auth audit table for authentication events
	CREATE TABLE IF NOT EXISTS auth_audit (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		event_type TEXT NOT NULL,
		user_id TEXT,
		username TEXT,
		email TEXT,
		org_id TEXT,
		ip_address TEXT,
		user_agent TEXT,
		session_id TEXT,
		success INTEGER DEFAULT 1,
		error_message TEXT,
		failure_code TEXT,
		method TEXT,
		provider TEXT,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_auth_audit_timestamp ON auth_audit(timestamp);
	CREATE INDEX IF NOT EXISTS idx_auth_audit_user ON auth_audit(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_auth_audit_event ON auth_audit(event_type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_auth_audit_ip ON auth_audit(ip_address, timestamp);

	-- Admin audit table for administrative actions
	CREATE TABLE IF NOT EXISTS admin_audit (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		user_id TEXT NOT NULL,
		username TEXT,
		org_id TEXT,
		ip_address TEXT,
		action_type TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT,
		resource_name TEXT,
		previous_value TEXT,
		new_value TEXT,
		success INTEGER DEFAULT 1,
		error_message TEXT,
		request_id TEXT,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_admin_audit_timestamp ON admin_audit(timestamp);
	CREATE INDEX IF NOT EXISTS idx_admin_audit_user ON admin_audit(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_admin_audit_action ON admin_audit(action_type, timestamp);
	CREATE INDEX IF NOT EXISTS idx_admin_audit_resource ON admin_audit(resource_type, resource_id);

	-- Export audit table for data export tracking
	CREATE TABLE IF NOT EXISTS export_audit (
		id TEXT PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		user_id TEXT NOT NULL,
		username TEXT,
		org_id TEXT,
		ip_address TEXT,
		export_type TEXT NOT NULL,
		data_type TEXT NOT NULL,
		query TEXT,
		time_range_start DATETIME,
		time_range_end DATETIME,
		record_count INTEGER DEFAULT 0,
		file_size_bytes INTEGER DEFAULT 0,
		contains_pii INTEGER DEFAULT 0,
		pii_types_exported TEXT,
		success INTEGER DEFAULT 1,
		error_message TEXT,
		download_url TEXT,
		request_id TEXT,
		metadata TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_export_audit_timestamp ON export_audit(timestamp);
	CREATE INDEX IF NOT EXISTS idx_export_audit_user ON export_audit(user_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_export_audit_pii ON export_audit(contains_pii, timestamp);

	-- Retention policy table
	CREATE TABLE IF NOT EXISTS audit_retention_policy (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		query_audit_days INTEGER DEFAULT 90,
		auth_audit_days INTEGER DEFAULT 365,
		admin_audit_days INTEGER DEFAULT 730,
		export_audit_days INTEGER DEFAULT 365,
		general_audit_days INTEGER DEFAULT 90,
		updated_at DATETIME NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// DB returns the underlying database connection
func (s *Store) DB() *sql.DB {
	return s.db
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

// LogQueryAudit logs a query audit entry
func (s *Store) LogQueryAudit(entry *QueryAuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var piiTypesJSON, servicesJSON, metadataJSON []byte
	var err error

	if len(entry.PIITypesAccessed) > 0 {
		piiTypesJSON, _ = json.Marshal(entry.PIITypesAccessed)
	}
	if len(entry.ServicesAccessed) > 0 {
		servicesJSON, _ = json.Marshal(entry.ServicesAccessed)
	}
	if entry.Metadata != nil {
		metadataJSON, _ = json.Marshal(entry.Metadata)
	}

	_, err = s.db.Exec(`
		INSERT INTO query_audit (
			id, timestamp, user_id, username, session_id, ip_address, user_agent, org_id,
			query_type, query_text, data_source, time_range_start, time_range_end,
			rows_returned, duration_ns, duration_ms, success, error_message,
			accessed_pii, pii_types_accessed, services_accessed, request_id, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.UserID, entry.Username, entry.SessionID,
		entry.IPAddress, entry.UserAgent, entry.OrgID,
		entry.QueryType, entry.QueryText, entry.DataSource,
		entry.TimeRange.Start, entry.TimeRange.End,
		entry.RowsReturned, entry.Duration.Nanoseconds(), entry.DurationMs,
		entry.Success, entry.ErrorMessage,
		entry.AccessedPII, string(piiTypesJSON), string(servicesJSON),
		entry.RequestID, string(metadataJSON),
	)

	return err
}

// ListQueryAudit retrieves query audit entries matching the filter
func (s *Store) ListQueryAudit(filter QueryAuditFilter) ([]*QueryAuditEntry, error) {
	query := `SELECT
		id, timestamp, user_id, username, session_id, ip_address, user_agent, org_id,
		query_type, query_text, data_source, time_range_start, time_range_end,
		rows_returned, duration_ns, duration_ms, success, error_message,
		accessed_pii, pii_types_accessed, services_accessed, request_id, metadata
		FROM query_audit WHERE 1=1`

	args := []interface{}{}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.Username != "" {
		query += " AND username = ?"
		args = append(args, filter.Username)
	}
	if filter.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, filter.OrgID)
	}
	if filter.QueryType != "" {
		query += " AND query_type = ?"
		args = append(args, filter.QueryType)
	}
	if filter.DataSource != "" {
		query += " AND data_source = ?"
		args = append(args, filter.DataSource)
	}
	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}
	if filter.SuccessOnly != nil {
		if *filter.SuccessOnly {
			query += " AND success = 1"
		} else {
			query += " AND success = 0"
		}
	}
	if filter.AccessedPII != nil && *filter.AccessedPII {
		query += " AND accessed_pii = 1"
	}
	if filter.MinDuration != nil {
		query += " AND duration_ns >= ?"
		args = append(args, filter.MinDuration.Nanoseconds())
	}
	if filter.MaxDuration != nil {
		query += " AND duration_ns <= ?"
		args = append(args, filter.MaxDuration.Nanoseconds())
	}
	if filter.ServiceName != "" {
		query += " AND services_accessed LIKE ?"
		args = append(args, "%"+filter.ServiceName+"%")
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	} else {
		query += " LIMIT 100"
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*QueryAuditEntry
	for rows.Next() {
		entry := &QueryAuditEntry{}
		var sessionID, userAgent, orgID, dataSource sql.NullString
		var errorMsg, requestID, piiTypesJSON, servicesJSON, metadataJSON sql.NullString
		var durationNs int64
		var success int

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.UserID, &entry.Username,
			&sessionID, &entry.IPAddress, &userAgent, &orgID,
			&entry.QueryType, &entry.QueryText, &dataSource,
			&entry.TimeRange.Start, &entry.TimeRange.End,
			&entry.RowsReturned, &durationNs, &entry.DurationMs,
			&success, &errorMsg,
			&entry.AccessedPII, &piiTypesJSON, &servicesJSON,
			&requestID, &metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		entry.SessionID = sessionID.String
		entry.UserAgent = userAgent.String
		entry.OrgID = orgID.String
		entry.DataSource = dataSource.String
		entry.Duration = time.Duration(durationNs)
		entry.Success = success == 1
		entry.ErrorMessage = errorMsg.String
		entry.RequestID = requestID.String

		if piiTypesJSON.Valid && piiTypesJSON.String != "" {
			json.Unmarshal([]byte(piiTypesJSON.String), &entry.PIITypesAccessed)
		}
		if servicesJSON.Valid && servicesJSON.String != "" {
			json.Unmarshal([]byte(servicesJSON.String), &entry.ServicesAccessed)
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// GetQueryAudit retrieves a single query audit entry by ID
func (s *Store) GetQueryAudit(id string) (*QueryAuditEntry, error) {
	entries, err := s.ListQueryAudit(QueryAuditFilter{Limit: 1})
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRow(`SELECT
		id, timestamp, user_id, username, session_id, ip_address, user_agent, org_id,
		query_type, query_text, data_source, time_range_start, time_range_end,
		rows_returned, duration_ns, duration_ms, success, error_message,
		accessed_pii, pii_types_accessed, services_accessed, request_id, metadata
		FROM query_audit WHERE id = ?`, id)

	entry := &QueryAuditEntry{}
	var sessionID, userAgent, orgID, dataSource sql.NullString
	var errorMsg, requestID, piiTypesJSON, servicesJSON, metadataJSON sql.NullString
	var durationNs int64
	var success int

	err = row.Scan(
		&entry.ID, &entry.Timestamp, &entry.UserID, &entry.Username,
		&sessionID, &entry.IPAddress, &userAgent, &orgID,
		&entry.QueryType, &entry.QueryText, &dataSource,
		&entry.TimeRange.Start, &entry.TimeRange.End,
		&entry.RowsReturned, &durationNs, &entry.DurationMs,
		&success, &errorMsg,
		&entry.AccessedPII, &piiTypesJSON, &servicesJSON,
		&requestID, &metadataJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	entry.SessionID = sessionID.String
	entry.UserAgent = userAgent.String
	entry.OrgID = orgID.String
	entry.DataSource = dataSource.String
	entry.Duration = time.Duration(durationNs)
	entry.Success = success == 1
	entry.ErrorMessage = errorMsg.String
	entry.RequestID = requestID.String

	if piiTypesJSON.Valid && piiTypesJSON.String != "" {
		json.Unmarshal([]byte(piiTypesJSON.String), &entry.PIITypesAccessed)
	}
	if servicesJSON.Valid && servicesJSON.String != "" {
		json.Unmarshal([]byte(servicesJSON.String), &entry.ServicesAccessed)
	}
	if metadataJSON.Valid && metadataJSON.String != "" {
		json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
	}

	_ = entries // silence unused variable warning
	return entry, nil
}

// CountQueryAudit returns the count of query audit entries matching the filter
func (s *Store) CountQueryAudit(filter QueryAuditFilter) (int64, error) {
	query := `SELECT COUNT(*) FROM query_audit WHERE 1=1`
	args := []interface{}{}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, filter.OrgID)
	}
	if filter.QueryType != "" {
		query += " AND query_type = ?"
		args = append(args, filter.QueryType)
	}
	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}
	if filter.AccessedPII != nil && *filter.AccessedPII {
		query += " AND accessed_pii = 1"
	}

	var count int64
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// LogAuthAudit logs an authentication audit entry
func (s *Store) LogAuthAudit(entry *AuthAuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var metadataJSON []byte
	if entry.Metadata != nil {
		metadataJSON, _ = json.Marshal(entry.Metadata)
	}

	_, err := s.db.Exec(`
		INSERT INTO auth_audit (
			id, timestamp, event_type, user_id, username, email, org_id,
			ip_address, user_agent, session_id, success, error_message,
			failure_code, method, provider, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.EventType, entry.UserID, entry.Username,
		entry.Email, entry.OrgID, entry.IPAddress, entry.UserAgent, entry.SessionID,
		entry.Success, entry.ErrorMessage, entry.FailureCode, entry.Method,
		entry.Provider, string(metadataJSON),
	)

	return err
}

// ListAuthAudit retrieves auth audit entries matching the filter
func (s *Store) ListAuthAudit(filter AuthAuditFilter) ([]*AuthAuditEntry, error) {
	query := `SELECT
		id, timestamp, event_type, user_id, username, email, org_id,
		ip_address, user_agent, session_id, success, error_message,
		failure_code, method, provider, metadata
		FROM auth_audit WHERE 1=1`

	args := []interface{}{}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.Email != "" {
		query += " AND email = ?"
		args = append(args, filter.Email)
	}
	if filter.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, filter.OrgID)
	}
	if filter.EventType != "" {
		query += " AND event_type = ?"
		args = append(args, filter.EventType)
	}
	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}
	if filter.SuccessOnly != nil {
		if *filter.SuccessOnly {
			query += " AND success = 1"
		} else {
			query += " AND success = 0"
		}
	}
	if filter.IPAddress != "" {
		query += " AND ip_address = ?"
		args = append(args, filter.IPAddress)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	} else {
		query += " LIMIT 100"
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuthAuditEntry
	for rows.Next() {
		entry := &AuthAuditEntry{}
		var userID, username, email, orgID sql.NullString
		var userAgent, sessionID, errorMsg, failureCode sql.NullString
		var method, provider, metadataJSON sql.NullString
		var success int

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.EventType,
			&userID, &username, &email, &orgID,
			&entry.IPAddress, &userAgent, &sessionID, &success, &errorMsg,
			&failureCode, &method, &provider, &metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		entry.UserID = userID.String
		entry.Username = username.String
		entry.Email = email.String
		entry.OrgID = orgID.String
		entry.UserAgent = userAgent.String
		entry.SessionID = sessionID.String
		entry.Success = success == 1
		entry.ErrorMessage = errorMsg.String
		entry.FailureCode = failureCode.String
		entry.Method = method.String
		entry.Provider = provider.String

		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// LogAdminAudit logs an admin action audit entry
func (s *Store) LogAdminAudit(entry *AdminAuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var prevValueJSON, newValueJSON, metadataJSON []byte
	if entry.PreviousValue != nil {
		prevValueJSON, _ = json.Marshal(entry.PreviousValue)
	}
	if entry.NewValue != nil {
		newValueJSON, _ = json.Marshal(entry.NewValue)
	}
	if entry.Metadata != nil {
		metadataJSON, _ = json.Marshal(entry.Metadata)
	}

	_, err := s.db.Exec(`
		INSERT INTO admin_audit (
			id, timestamp, user_id, username, org_id, ip_address,
			action_type, resource_type, resource_id, resource_name,
			previous_value, new_value, success, error_message,
			request_id, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.UserID, entry.Username, entry.OrgID,
		entry.IPAddress, entry.ActionType, entry.ResourceType, entry.ResourceID,
		entry.ResourceName, string(prevValueJSON), string(newValueJSON),
		entry.Success, entry.ErrorMessage, entry.RequestID, string(metadataJSON),
	)

	return err
}

// ListAdminAudit retrieves admin audit entries matching the filter
func (s *Store) ListAdminAudit(filter AdminAuditFilter) ([]*AdminAuditEntry, error) {
	query := `SELECT
		id, timestamp, user_id, username, org_id, ip_address,
		action_type, resource_type, resource_id, resource_name,
		previous_value, new_value, success, error_message,
		request_id, metadata
		FROM admin_audit WHERE 1=1`

	args := []interface{}{}

	if filter.UserID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.UserID)
	}
	if filter.OrgID != "" {
		query += " AND org_id = ?"
		args = append(args, filter.OrgID)
	}
	if filter.ActionType != "" {
		query += " AND action_type = ?"
		args = append(args, filter.ActionType)
	}
	if filter.ResourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, filter.ResourceType)
	}
	if filter.ResourceID != "" {
		query += " AND resource_id = ?"
		args = append(args, filter.ResourceID)
	}
	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *filter.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	} else {
		query += " LIMIT 100"
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AdminAuditEntry
	for rows.Next() {
		entry := &AdminAuditEntry{}
		var username, orgID, resourceID, resourceName sql.NullString
		var prevValueJSON, newValueJSON, errorMsg, requestID, metadataJSON sql.NullString
		var success int

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.UserID, &username, &orgID,
			&entry.IPAddress, &entry.ActionType, &entry.ResourceType,
			&resourceID, &resourceName, &prevValueJSON, &newValueJSON,
			&success, &errorMsg, &requestID, &metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		entry.Username = username.String
		entry.OrgID = orgID.String
		entry.ResourceID = resourceID.String
		entry.ResourceName = resourceName.String
		entry.Success = success == 1
		entry.ErrorMessage = errorMsg.String
		entry.RequestID = requestID.String

		if prevValueJSON.Valid && prevValueJSON.String != "" {
			json.Unmarshal([]byte(prevValueJSON.String), &entry.PreviousValue)
		}
		if newValueJSON.Valid && newValueJSON.String != "" {
			json.Unmarshal([]byte(newValueJSON.String), &entry.NewValue)
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// LogExportAudit logs a data export audit entry
func (s *Store) LogExportAudit(entry *DataExportEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var piiTypesJSON, metadataJSON []byte
	if len(entry.PIITypesExported) > 0 {
		piiTypesJSON, _ = json.Marshal(entry.PIITypesExported)
	}
	if entry.Metadata != nil {
		metadataJSON, _ = json.Marshal(entry.Metadata)
	}

	_, err := s.db.Exec(`
		INSERT INTO export_audit (
			id, timestamp, user_id, username, org_id, ip_address,
			export_type, data_type, query, time_range_start, time_range_end,
			record_count, file_size_bytes, contains_pii, pii_types_exported,
			success, error_message, download_url, request_id, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Timestamp, entry.UserID, entry.Username, entry.OrgID,
		entry.IPAddress, entry.ExportType, entry.DataType, entry.Query,
		entry.TimeRange.Start, entry.TimeRange.End, entry.RecordCount,
		entry.FileSizeBytes, entry.ContainsPII, string(piiTypesJSON),
		entry.Success, entry.ErrorMessage, entry.DownloadURL,
		entry.RequestID, string(metadataJSON),
	)

	return err
}

// ListExportAudit retrieves export audit entries
func (s *Store) ListExportAudit(userID, orgID string, startTime, endTime *time.Time, limit, offset int) ([]*DataExportEntry, error) {
	query := `SELECT
		id, timestamp, user_id, username, org_id, ip_address,
		export_type, data_type, query, time_range_start, time_range_end,
		record_count, file_size_bytes, contains_pii, pii_types_exported,
		success, error_message, download_url, request_id, metadata
		FROM export_audit WHERE 1=1`

	args := []interface{}{}

	if userID != "" {
		query += " AND user_id = ?"
		args = append(args, userID)
	}
	if orgID != "" {
		query += " AND org_id = ?"
		args = append(args, orgID)
	}
	if startTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, *startTime)
	}
	if endTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, *endTime)
	}

	query += " ORDER BY timestamp DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	} else {
		query += " LIMIT 100"
	}
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*DataExportEntry
	for rows.Next() {
		entry := &DataExportEntry{}
		var username, orgID, queryStr, errorMsg, downloadURL, requestID sql.NullString
		var piiTypesJSON, metadataJSON sql.NullString
		var success int

		err := rows.Scan(
			&entry.ID, &entry.Timestamp, &entry.UserID, &username, &orgID,
			&entry.IPAddress, &entry.ExportType, &entry.DataType, &queryStr,
			&entry.TimeRange.Start, &entry.TimeRange.End, &entry.RecordCount,
			&entry.FileSizeBytes, &entry.ContainsPII, &piiTypesJSON,
			&success, &errorMsg, &downloadURL, &requestID, &metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		entry.Username = username.String
		entry.OrgID = orgID.String
		entry.Query = queryStr.String
		entry.Success = success == 1
		entry.ErrorMessage = errorMsg.String
		entry.DownloadURL = downloadURL.String
		entry.RequestID = requestID.String

		if piiTypesJSON.Valid && piiTypesJSON.String != "" {
			json.Unmarshal([]byte(piiTypesJSON.String), &entry.PIITypesExported)
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// GetAuditSummary returns aggregated audit statistics for a time period
func (s *Store) GetAuditSummary(orgID string, since time.Duration) (*AuditSummary, error) {
	now := time.Now()
	startTime := now.Add(-since)

	summary := &AuditSummary{
		Period:              since.String(),
		StartTime:           startTime,
		EndTime:             now,
		QueriesByType:       make(map[string]int64),
		QueriesByUser:       make(map[string]int64),
		LoginsByMethod:      make(map[string]int64),
		AdminActionsByType:  make(map[string]int64),
		AdminActionsByUser:  make(map[string]int64),
	}

	baseWhere := "WHERE timestamp >= ?"
	args := []interface{}{startTime}
	if orgID != "" {
		baseWhere += " AND org_id = ?"
		args = append(args, orgID)
	}

	// Query audit stats
	s.db.QueryRow("SELECT COUNT(*) FROM query_audit "+baseWhere, args...).Scan(&summary.TotalQueries)
	s.db.QueryRow("SELECT COUNT(*) FROM query_audit "+baseWhere+" AND success = 1", args...).Scan(&summary.SuccessfulQueries)
	summary.FailedQueries = summary.TotalQueries - summary.SuccessfulQueries
	s.db.QueryRow("SELECT COUNT(*) FROM query_audit "+baseWhere+" AND accessed_pii = 1", args...).Scan(&summary.QueriesWithPII)
	s.db.QueryRow("SELECT AVG(duration_ms) FROM query_audit "+baseWhere, args...).Scan(&summary.AvgQueryDurationMs)

	// Queries by type
	rows, _ := s.db.Query("SELECT query_type, COUNT(*) FROM query_audit "+baseWhere+" GROUP BY query_type", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var qType string
			var count int64
			rows.Scan(&qType, &count)
			summary.QueriesByType[qType] = count
		}
	}

	// Queries by user
	rows, _ = s.db.Query("SELECT username, COUNT(*) FROM query_audit "+baseWhere+" AND username != '' GROUP BY username ORDER BY COUNT(*) DESC LIMIT 10", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var username string
			var count int64
			rows.Scan(&username, &count)
			summary.QueriesByUser[username] = count
		}
	}

	// Auth stats
	s.db.QueryRow("SELECT COUNT(*) FROM auth_audit "+baseWhere+" AND event_type = 'login'", args...).Scan(&summary.TotalLogins)
	s.db.QueryRow("SELECT COUNT(*) FROM auth_audit "+baseWhere+" AND event_type = 'login' AND success = 1", args...).Scan(&summary.SuccessfulLogins)
	summary.FailedLogins = summary.TotalLogins - summary.SuccessfulLogins
	s.db.QueryRow("SELECT COUNT(DISTINCT user_id) FROM auth_audit "+baseWhere+" AND success = 1", args...).Scan(&summary.UniqueUsers)

	// Logins by method
	rows, _ = s.db.Query("SELECT COALESCE(method, 'password'), COUNT(*) FROM auth_audit "+baseWhere+" AND event_type = 'login' GROUP BY method", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var count int64
			rows.Scan(&method, &count)
			summary.LoginsByMethod[method] = count
		}
	}

	// Admin stats
	s.db.QueryRow("SELECT COUNT(*) FROM admin_audit "+baseWhere, args...).Scan(&summary.TotalAdminActions)

	// Admin actions by type
	rows, _ = s.db.Query("SELECT action_type, COUNT(*) FROM admin_audit "+baseWhere+" GROUP BY action_type", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var actionType string
			var count int64
			rows.Scan(&actionType, &count)
			summary.AdminActionsByType[actionType] = count
		}
	}

	// Admin actions by user
	rows, _ = s.db.Query("SELECT username, COUNT(*) FROM admin_audit "+baseWhere+" AND username != '' GROUP BY username ORDER BY COUNT(*) DESC LIMIT 10", args...)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var username string
			var count int64
			rows.Scan(&username, &count)
			summary.AdminActionsByUser[username] = count
		}
	}

	// Export stats
	s.db.QueryRow("SELECT COUNT(*) FROM export_audit "+baseWhere, args...).Scan(&summary.TotalExports)
	s.db.QueryRow("SELECT COUNT(*) FROM export_audit "+baseWhere+" AND contains_pii = 1", args...).Scan(&summary.ExportsWithPII)
	s.db.QueryRow("SELECT COALESCE(SUM(file_size_bytes), 0) FROM export_audit "+baseWhere, args...).Scan(&summary.TotalBytesExported)

	return summary, nil
}

// RetentionPolicy represents audit log retention settings
type RetentionPolicy struct {
	QueryAuditDays   int       `json:"query_audit_days"`
	AuthAuditDays    int       `json:"auth_audit_days"`
	AdminAuditDays   int       `json:"admin_audit_days"`
	ExportAuditDays  int       `json:"export_audit_days"`
	GeneralAuditDays int       `json:"general_audit_days"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// GetRetentionPolicy returns the current retention policy
func (s *Store) GetRetentionPolicy() (*RetentionPolicy, error) {
	row := s.db.QueryRow(`SELECT query_audit_days, auth_audit_days, admin_audit_days,
		export_audit_days, general_audit_days, updated_at FROM audit_retention_policy WHERE id = 1`)

	policy := &RetentionPolicy{}
	var updatedAt string
	err := row.Scan(&policy.QueryAuditDays, &policy.AuthAuditDays, &policy.AdminAuditDays,
		&policy.ExportAuditDays, &policy.GeneralAuditDays, &updatedAt)
	if err == sql.ErrNoRows {
		// Return defaults
		return &RetentionPolicy{
			QueryAuditDays:   90,
			AuthAuditDays:    365,
			AdminAuditDays:   730,
			ExportAuditDays:  365,
			GeneralAuditDays: 90,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	policy.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return policy, nil
}

// SetRetentionPolicy updates the retention policy
func (s *Store) SetRetentionPolicy(policy *RetentionPolicy) error {
	policy.UpdatedAt = time.Now()

	_, err := s.db.Exec(`
		INSERT INTO audit_retention_policy (id, query_audit_days, auth_audit_days, admin_audit_days,
			export_audit_days, general_audit_days, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			query_audit_days = excluded.query_audit_days,
			auth_audit_days = excluded.auth_audit_days,
			admin_audit_days = excluded.admin_audit_days,
			export_audit_days = excluded.export_audit_days,
			general_audit_days = excluded.general_audit_days,
			updated_at = excluded.updated_at`,
		policy.QueryAuditDays, policy.AuthAuditDays, policy.AdminAuditDays,
		policy.ExportAuditDays, policy.GeneralAuditDays, policy.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

// PurgeOldRecords removes audit records based on retention policy
func (s *Store) PurgeOldRecords() (map[string]int64, error) {
	policy, err := s.GetRetentionPolicy()
	if err != nil {
		return nil, err
	}

	results := make(map[string]int64)

	// Purge query audit
	cutoff := time.Now().AddDate(0, 0, -policy.QueryAuditDays)
	result, _ := s.db.Exec("DELETE FROM query_audit WHERE timestamp < ?", cutoff)
	if result != nil {
		results["query_audit"], _ = result.RowsAffected()
	}

	// Purge auth audit
	cutoff = time.Now().AddDate(0, 0, -policy.AuthAuditDays)
	result, _ = s.db.Exec("DELETE FROM auth_audit WHERE timestamp < ?", cutoff)
	if result != nil {
		results["auth_audit"], _ = result.RowsAffected()
	}

	// Purge admin audit
	cutoff = time.Now().AddDate(0, 0, -policy.AdminAuditDays)
	result, _ = s.db.Exec("DELETE FROM admin_audit WHERE timestamp < ?", cutoff)
	if result != nil {
		results["admin_audit"], _ = result.RowsAffected()
	}

	// Purge export audit
	cutoff = time.Now().AddDate(0, 0, -policy.ExportAuditDays)
	result, _ = s.db.Exec("DELETE FROM export_audit WHERE timestamp < ?", cutoff)
	if result != nil {
		results["export_audit"], _ = result.RowsAffected()
	}

	// Purge general audit logs
	cutoff = time.Now().AddDate(0, 0, -policy.GeneralAuditDays)
	result, _ = s.db.Exec("DELETE FROM audit_logs WHERE timestamp < ?", cutoff)
	if result != nil {
		results["audit_logs"], _ = result.RowsAffected()
	}

	return results, nil
}
