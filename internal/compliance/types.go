// Package compliance provides SOC2, HIPAA, GDPR, and PCI compliance reporting
package compliance

import (
	"time"
)

// ReportType defines the type of compliance report
type ReportType string

const (
	ReportTypeSOC2   ReportType = "SOC2"
	ReportTypeHIPAA  ReportType = "HIPAA"
	ReportTypeGDPR   ReportType = "GDPR"
	ReportTypePCI    ReportType = "PCI"
	ReportTypeCustom ReportType = "Custom"
)

// ReportStatus defines the status of a compliance report
type ReportStatus string

const (
	StatusDraft    ReportStatus = "draft"
	StatusFinal    ReportStatus = "final"
	StatusArchived ReportStatus = "archived"
)

// ControlStatus defines the compliance status of a control
type ControlStatus string

const (
	ControlCompliant    ControlStatus = "compliant"
	ControlNonCompliant ControlStatus = "non_compliant"
	ControlPartial      ControlStatus = "partial"
	ControlNotApplicable ControlStatus = "n_a"
	ControlPending      ControlStatus = "pending"
)

// FindingSeverity defines the severity of a compliance finding
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// EvidenceType defines the type of evidence collected
type EvidenceType string

const (
	EvidenceScreenshot EvidenceType = "screenshot"
	EvidenceLog        EvidenceType = "log"
	EvidenceConfig     EvidenceType = "config"
	EvidenceMetric     EvidenceType = "metric"
	EvidenceReport     EvidenceType = "report"
	EvidenceDocument   EvidenceType = "document"
	EvidenceQuery      EvidenceType = "query"
)

// DateRange represents a time period for reporting
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComplianceReport represents a full compliance report
type ComplianceReport struct {
	ID          string            `json:"id"`
	Type        ReportType        `json:"type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Period      DateRange         `json:"period"`
	GeneratedAt time.Time         `json:"generated_at"`
	GeneratedBy string            `json:"generated_by"`
	Status      ReportStatus      `json:"status"`
	Version     int               `json:"version"`
	Sections    []ReportSection   `json:"sections"`
	Evidence    []Evidence        `json:"evidence"`
	Summary     ComplianceSummary `json:"summary"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ReportSection represents a section/control in the report
type ReportSection struct {
	ID          string        `json:"id"`           // e.g., "CC6.1"
	Control     string        `json:"control"`      // Control identifier
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Category    string        `json:"category"`     // Trust Service Category
	Status      ControlStatus `json:"status"`
	Evidence    []Evidence    `json:"evidence"`
	Findings    []Finding     `json:"findings"`
	Remediation string        `json:"remediation,omitempty"`
	Notes       string        `json:"notes,omitempty"`
	TestResults []TestResult  `json:"test_results,omitempty"`
	LastTested  *time.Time    `json:"last_tested,omitempty"`
}

// Evidence represents collected evidence for a control
type Evidence struct {
	ID          string       `json:"id"`
	Type        EvidenceType `json:"type"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Data        interface{}  `json:"data,omitempty"`
	DataSummary string       `json:"data_summary,omitempty"`
	CollectedAt time.Time    `json:"collected_at"`
	Source      string       `json:"source"`
	ControlID   string       `json:"control_id,omitempty"`
	ReportID    string       `json:"report_id,omitempty"`
	Hash        string       `json:"hash,omitempty"` // SHA256 for integrity
}

// Finding represents a compliance finding/issue
type Finding struct {
	ID           string          `json:"id"`
	ControlID    string          `json:"control_id"`
	Severity     FindingSeverity `json:"severity"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	Impact       string          `json:"impact"`
	Remediation  string          `json:"remediation"`
	Status       string          `json:"status"` // open, in_progress, resolved, accepted
	DueDate      *time.Time      `json:"due_date,omitempty"`
	ResolvedAt   *time.Time      `json:"resolved_at,omitempty"`
	ResolvedBy   string          `json:"resolved_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	AssignedTo   string          `json:"assigned_to,omitempty"`
	EvidenceRefs []string        `json:"evidence_refs,omitempty"`
}

// TestResult represents the result of a control test
type TestResult struct {
	ID          string    `json:"id"`
	ControlID   string    `json:"control_id"`
	TestName    string    `json:"test_name"`
	Passed      bool      `json:"passed"`
	Details     string    `json:"details"`
	TestedAt    time.Time `json:"tested_at"`
	TestedBy    string    `json:"tested_by"` // "automated" or user ID
	EvidenceRef string    `json:"evidence_ref,omitempty"`
}

// ComplianceSummary provides an overview of compliance status
type ComplianceSummary struct {
	TotalControls      int                      `json:"total_controls"`
	CompliantControls  int                      `json:"compliant_controls"`
	NonCompliant       int                      `json:"non_compliant"`
	PartialControls    int                      `json:"partial_controls"`
	NotApplicable      int                      `json:"not_applicable"`
	PendingControls    int                      `json:"pending_controls"`
	ComplianceScore    float64                  `json:"compliance_score"` // Percentage
	CriticalFindings   int                      `json:"critical_findings"`
	HighFindings       int                      `json:"high_findings"`
	MediumFindings     int                      `json:"medium_findings"`
	LowFindings        int                      `json:"low_findings"`
	OpenFindings       int                      `json:"open_findings"`
	ControlsByCategory map[string]CategoryStats `json:"controls_by_category"`
	RiskScore          float64                  `json:"risk_score"` // 0-100
	TrendData          []TrendPoint             `json:"trend_data,omitempty"`
}

// CategoryStats provides statistics for a category
type CategoryStats struct {
	Total     int     `json:"total"`
	Compliant int     `json:"compliant"`
	Score     float64 `json:"score"`
}

// TrendPoint represents a point in compliance trend data
type TrendPoint struct {
	Date            time.Time `json:"date"`
	ComplianceScore float64   `json:"compliance_score"`
	OpenFindings    int       `json:"open_findings"`
}

// ScheduledReport represents a scheduled compliance report
type ScheduledReport struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         ReportType        `json:"type"`
	Schedule     string            `json:"schedule"` // cron expression
	PeriodDays   int               `json:"period_days"` // Report covers last N days
	Enabled      bool              `json:"enabled"`
	Recipients   []string          `json:"recipients"` // Email addresses
	LastRun      *time.Time        `json:"last_run,omitempty"`
	NextRun      *time.Time        `json:"next_run,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	CreatedBy    string            `json:"created_by"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// GapAnalysis represents analysis of compliance gaps
type GapAnalysis struct {
	ReportID        string        `json:"report_id"`
	GeneratedAt     time.Time     `json:"generated_at"`
	TotalGaps       int           `json:"total_gaps"`
	CriticalGaps    int           `json:"critical_gaps"`
	Gaps            []Gap         `json:"gaps"`
	Recommendations []string      `json:"recommendations"`
	EstimatedEffort string        `json:"estimated_effort"`
	PriorityActions []PriorityAction `json:"priority_actions"`
}

// Gap represents a compliance gap
type Gap struct {
	ControlID       string          `json:"control_id"`
	ControlTitle    string          `json:"control_title"`
	CurrentState    string          `json:"current_state"`
	RequiredState   string          `json:"required_state"`
	Severity        FindingSeverity `json:"severity"`
	Remediation     string          `json:"remediation"`
	EstimatedEffort string          `json:"estimated_effort"`
	Dependencies    []string        `json:"dependencies,omitempty"`
}

// PriorityAction represents a prioritized remediation action
type PriorityAction struct {
	Rank        int      `json:"rank"`
	ControlID   string   `json:"control_id"`
	Action      string   `json:"action"`
	Impact      string   `json:"impact"`
	Effort      string   `json:"effort"`
	Blocking    []string `json:"blocking,omitempty"` // Controls blocked by this gap
}

// EvidenceFilter contains filters for querying evidence
type EvidenceFilter struct {
	ReportID  string
	ControlID string
	Type      EvidenceType
	StartTime *time.Time
	EndTime   *time.Time
	Source    string
	Limit     int
	Offset    int
}

// ReportFilter contains filters for querying reports
type ReportFilter struct {
	Type      ReportType
	Status    ReportStatus
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

// UserAccessReport represents a user access audit for compliance
type UserAccessReport struct {
	GeneratedAt   time.Time              `json:"generated_at"`
	Period        DateRange              `json:"period"`
	TotalUsers    int                    `json:"total_users"`
	ActiveUsers   int                    `json:"active_users"`
	InactiveUsers int                    `json:"inactive_users"`
	UsersByRole   map[string]int         `json:"users_by_role"`
	AccessChanges []AccessChange         `json:"access_changes"`
	Anomalies     []AccessAnomaly        `json:"anomalies"`
}

// AccessChange represents a change in user access
type AccessChange struct {
	Timestamp  time.Time `json:"timestamp"`
	UserID     string    `json:"user_id"`
	UserEmail  string    `json:"user_email"`
	ChangeType string    `json:"change_type"` // created, updated, deleted, role_change
	Details    string    `json:"details"`
	ChangedBy  string    `json:"changed_by"`
}

// AccessAnomaly represents an anomalous access pattern
type AccessAnomaly struct {
	Timestamp   time.Time `json:"timestamp"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	AnomalyType string    `json:"anomaly_type"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
}

// AuthenticationReport represents authentication audit data
type AuthenticationReport struct {
	GeneratedAt    time.Time       `json:"generated_at"`
	Period         DateRange       `json:"period"`
	TotalLogins    int             `json:"total_logins"`
	SuccessLogins  int             `json:"success_logins"`
	FailedLogins   int             `json:"failed_logins"`
	UniqueUsers    int             `json:"unique_users"`
	LoginsByHour   map[int]int     `json:"logins_by_hour"`
	LoginsByDay    map[string]int  `json:"logins_by_day"`
	TopIPs         []IPStats       `json:"top_ips"`
	SuspiciousIPs  []SuspiciousIP  `json:"suspicious_ips"`
	MFAUsage       *MFAStats       `json:"mfa_usage,omitempty"`
}

// IPStats represents login statistics for an IP
type IPStats struct {
	IP      string `json:"ip"`
	Count   int    `json:"count"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
}

// SuspiciousIP represents a potentially suspicious IP
type SuspiciousIP struct {
	IP            string    `json:"ip"`
	FailedLogins  int       `json:"failed_logins"`
	UniqueUsers   int       `json:"unique_users"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	Reason        string    `json:"reason"`
}

// MFAStats represents MFA usage statistics
type MFAStats struct {
	Enabled       int     `json:"enabled"`
	Disabled      int     `json:"disabled"`
	EnforcedRate  float64 `json:"enforced_rate"`
}

// ConfigChangeReport represents configuration change audit
type ConfigChangeReport struct {
	GeneratedAt    time.Time      `json:"generated_at"`
	Period         DateRange      `json:"period"`
	TotalChanges   int            `json:"total_changes"`
	ChangesByType  map[string]int `json:"changes_by_type"`
	ChangesByUser  map[string]int `json:"changes_by_user"`
	CriticalChanges []ConfigChange `json:"critical_changes"`
	AllChanges     []ConfigChange `json:"all_changes"`
}

// ConfigChange represents a configuration change
type ConfigChange struct {
	Timestamp    time.Time `json:"timestamp"`
	UserID       string    `json:"user_id"`
	UserEmail    string    `json:"user_email"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Action       string    `json:"action"`
	Before       string    `json:"before,omitempty"`
	After        string    `json:"after,omitempty"`
	IsCritical   bool      `json:"is_critical"`
}

// IncidentReport represents incident response data for compliance
type IncidentReport struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	Period           DateRange         `json:"period"`
	TotalIncidents   int               `json:"total_incidents"`
	OpenIncidents    int               `json:"open_incidents"`
	ClosedIncidents  int               `json:"closed_incidents"`
	AvgResponseTime  float64           `json:"avg_response_time_minutes"`
	AvgResolutionTime float64          `json:"avg_resolution_time_hours"`
	IncidentsByPriority map[string]int `json:"incidents_by_priority"`
	SecurityIncidents []SecurityIncident `json:"security_incidents"`
}

// SecurityIncident represents a security incident
type SecurityIncident struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Priority     string     `json:"priority"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	ResponseTime float64    `json:"response_time_minutes"`
	Description  string     `json:"description"`
	RootCause    string     `json:"root_cause,omitempty"`
}

// SystemAvailabilityReport represents availability data
type SystemAvailabilityReport struct {
	GeneratedAt     time.Time        `json:"generated_at"`
	Period          DateRange        `json:"period"`
	OverallUptime   float64          `json:"overall_uptime_percent"`
	TotalDowntime   float64          `json:"total_downtime_minutes"`
	Incidents       int              `json:"incidents"`
	SLOCompliance   []SLOCompliance  `json:"slo_compliance"`
	UptimeByService map[string]float64 `json:"uptime_by_service"`
}

// SLOCompliance represents SLO compliance data
type SLOCompliance struct {
	SLOID       string  `json:"slo_id"`
	SLOName     string  `json:"slo_name"`
	Target      float64 `json:"target"`
	Actual      float64 `json:"actual"`
	Met         bool    `json:"met"`
	BudgetUsed  float64 `json:"budget_used_percent"`
}

// BackupReport represents backup verification data
type BackupReport struct {
	GeneratedAt      time.Time      `json:"generated_at"`
	Period           DateRange      `json:"period"`
	TotalBackups     int            `json:"total_backups"`
	SuccessfulBackups int           `json:"successful_backups"`
	FailedBackups    int            `json:"failed_backups"`
	LastBackup       *time.Time     `json:"last_backup,omitempty"`
	LastVerified     *time.Time     `json:"last_verified,omitempty"`
	BackupSize       int64          `json:"backup_size_bytes"`
	RecoveryTested   bool           `json:"recovery_tested"`
	RecoveryTestDate *time.Time     `json:"recovery_test_date,omitempty"`
	Backups          []BackupRecord `json:"backups"`
}

// BackupRecord represents a single backup record
type BackupRecord struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Size       int64     `json:"size_bytes"`
	Status     string    `json:"status"`
	Verified   bool      `json:"verified"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}
