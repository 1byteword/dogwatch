// Package audit provides enhanced audit logging for compliance
package audit

import (
	"time"
)

// QueryType represents the type of query being audited
type QueryType string

const (
	QueryTypeLogs    QueryType = "logs"
	QueryTypeTraces  QueryType = "traces"
	QueryTypeMetrics QueryType = "metrics"
	QueryTypeDQL     QueryType = "dql"
	QueryTypeSQL     QueryType = "sql"
	QueryTypeAPI     QueryType = "api"
)

// TimeRange represents the time range of a query
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Duration returns the duration of the time range
func (tr TimeRange) Duration() time.Duration {
	return tr.End.Sub(tr.Start)
}

// QueryAuditEntry represents a single query audit log entry
type QueryAuditEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`

	// User context
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	SessionID string `json:"session_id,omitempty"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent,omitempty"`
	OrgID     string `json:"org_id,omitempty"`

	// Query details
	QueryType  QueryType `json:"query_type"`
	QueryText  string    `json:"query_text"`
	DataSource string    `json:"data_source"` // What data was accessed
	TimeRange  TimeRange `json:"time_range"`

	// Results
	RowsReturned int64         `json:"rows_returned"`
	Duration     time.Duration `json:"duration_ns"`
	DurationMs   float64       `json:"duration_ms"`
	Success      bool          `json:"success"`
	ErrorMessage string        `json:"error_message,omitempty"`

	// Sensitive data flags
	AccessedPII      bool     `json:"accessed_pii"`
	PIITypesAccessed []string `json:"pii_types_accessed,omitempty"`
	ServicesAccessed []string `json:"services_accessed,omitempty"`

	// Additional metadata
	RequestID string                 `json:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NewQueryAuditEntry creates a new query audit entry with defaults
func NewQueryAuditEntry() *QueryAuditEntry {
	return &QueryAuditEntry{
		Timestamp: time.Now(),
		Success:   true,
	}
}

// SetDuration sets the duration and calculates DurationMs
func (e *QueryAuditEntry) SetDuration(d time.Duration) {
	e.Duration = d
	e.DurationMs = float64(d.Nanoseconds()) / 1e6
}

// AuthAuditEntry represents an authentication audit log entry
type AuthAuditEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`

	// Event details
	EventType string `json:"event_type"` // login, logout, login_failed, token_refresh, api_key_used
	UserID    string `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Email     string `json:"email,omitempty"`
	OrgID     string `json:"org_id,omitempty"`

	// Request context
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`

	// Result
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	FailureCode  string `json:"failure_code,omitempty"` // invalid_password, account_locked, etc.

	// Additional context
	Method    string                 `json:"method,omitempty"` // password, sso, api_key
	Provider  string                 `json:"provider,omitempty"` // For SSO: google, okta, etc.
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AuthEventType constants
const (
	AuthEventLogin          = "login"
	AuthEventLogout         = "logout"
	AuthEventLoginFailed    = "login_failed"
	AuthEventTokenRefresh   = "token_refresh"
	AuthEventAPIKeyUsed     = "api_key_used"
	AuthEventAPIKeyCreated  = "api_key_created"
	AuthEventAPIKeyRevoked  = "api_key_revoked"
	AuthEventPasswordChange = "password_change"
	AuthEventPasswordReset  = "password_reset"
	AuthEventMFAEnabled     = "mfa_enabled"
	AuthEventMFADisabled    = "mfa_disabled"
)

// AdminAuditEntry represents an admin action audit log entry
type AdminAuditEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`

	// Actor
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	OrgID     string `json:"org_id,omitempty"`
	IPAddress string `json:"ip_address"`

	// Action
	ActionType   string `json:"action_type"`   // user_create, user_delete, role_change, config_update, etc.
	ResourceType string `json:"resource_type"` // user, team, org, alert_rule, dashboard, etc.
	ResourceID   string `json:"resource_id"`
	ResourceName string `json:"resource_name,omitempty"`

	// Changes
	PreviousValue interface{} `json:"previous_value,omitempty"`
	NewValue      interface{} `json:"new_value,omitempty"`

	// Result
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`

	// Context
	RequestID string                 `json:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AdminActionType constants
const (
	AdminActionUserCreate     = "user_create"
	AdminActionUserUpdate     = "user_update"
	AdminActionUserDelete     = "user_delete"
	AdminActionUserDisable    = "user_disable"
	AdminActionRoleChange     = "role_change"
	AdminActionTeamCreate     = "team_create"
	AdminActionTeamUpdate     = "team_update"
	AdminActionTeamDelete     = "team_delete"
	AdminActionOrgCreate      = "org_create"
	AdminActionOrgUpdate      = "org_update"
	AdminActionConfigUpdate   = "config_update"
	AdminActionAlertRuleCreate = "alert_rule_create"
	AdminActionAlertRuleUpdate = "alert_rule_update"
	AdminActionAlertRuleDelete = "alert_rule_delete"
	AdminActionDashboardShare = "dashboard_share"
	AdminActionDataExport     = "data_export"
	AdminActionDataPurge      = "data_purge"
	AdminActionBackupCreate   = "backup_create"
	AdminActionBackupRestore  = "backup_restore"
)

// DataExportEntry represents a data export audit log entry
type DataExportEntry struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`

	// Actor
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	OrgID     string `json:"org_id,omitempty"`
	IPAddress string `json:"ip_address"`

	// Export details
	ExportType    string    `json:"export_type"`    // csv, json, pdf
	DataType      string    `json:"data_type"`      // logs, traces, metrics, audit, dashboard
	Query         string    `json:"query,omitempty"`
	TimeRange     TimeRange `json:"time_range"`
	RecordCount   int64     `json:"record_count"`
	FileSizeBytes int64     `json:"file_size_bytes,omitempty"`

	// Sensitive data flags
	ContainsPII      bool     `json:"contains_pii"`
	PIITypesExported []string `json:"pii_types_exported,omitempty"`

	// Result
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`

	// Context
	RequestID string                 `json:"request_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// QueryAuditFilter represents filters for query audit log searches
type QueryAuditFilter struct {
	UserID       string
	Username     string
	OrgID        string
	QueryType    QueryType
	DataSource   string
	StartTime    *time.Time
	EndTime      *time.Time
	SuccessOnly  *bool
	AccessedPII  *bool
	MinDuration  *time.Duration
	MaxDuration  *time.Duration
	ServiceName  string
	Limit        int
	Offset       int
}

// AuthAuditFilter represents filters for auth audit log searches
type AuthAuditFilter struct {
	UserID      string
	Email       string
	OrgID       string
	EventType   string
	StartTime   *time.Time
	EndTime     *time.Time
	SuccessOnly *bool
	IPAddress   string
	Limit       int
	Offset      int
}

// AdminAuditFilter represents filters for admin audit log searches
type AdminAuditFilter struct {
	UserID       string
	OrgID        string
	ActionType   string
	ResourceType string
	ResourceID   string
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}

// AuditSummary provides aggregated audit statistics
type AuditSummary struct {
	Period    string    `json:"period"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// Query stats
	TotalQueries       int64            `json:"total_queries"`
	SuccessfulQueries  int64            `json:"successful_queries"`
	FailedQueries      int64            `json:"failed_queries"`
	QueriesByType      map[string]int64 `json:"queries_by_type"`
	QueriesByUser      map[string]int64 `json:"queries_by_user"`
	AvgQueryDurationMs float64          `json:"avg_query_duration_ms"`
	P95QueryDurationMs float64          `json:"p95_query_duration_ms"`
	QueriesWithPII     int64            `json:"queries_with_pii"`

	// Auth stats
	TotalLogins        int64            `json:"total_logins"`
	SuccessfulLogins   int64            `json:"successful_logins"`
	FailedLogins       int64            `json:"failed_logins"`
	UniqueUsers        int64            `json:"unique_users"`
	LoginsByMethod     map[string]int64 `json:"logins_by_method"`

	// Admin stats
	TotalAdminActions   int64            `json:"total_admin_actions"`
	AdminActionsByType  map[string]int64 `json:"admin_actions_by_type"`
	AdminActionsByUser  map[string]int64 `json:"admin_actions_by_user"`

	// Export stats
	TotalExports       int64 `json:"total_exports"`
	ExportsWithPII     int64 `json:"exports_with_pii"`
	TotalBytesExported int64 `json:"total_bytes_exported"`
}
