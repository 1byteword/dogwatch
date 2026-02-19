package compliance

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store handles compliance data persistence
type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
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

func (s *Store) Close() error {
	return s.db.Close()
}

// init creates the necessary tables
func (s *Store) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS compliance_reports (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			period_start DATETIME NOT NULL,
			period_end DATETIME NOT NULL,
			generated_at DATETIME NOT NULL,
			generated_by TEXT NOT NULL,
			status TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			report_json TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_compliance_reports_type
			ON compliance_reports(type, generated_at);

		CREATE TABLE IF NOT EXISTS compliance_evidence (
			id TEXT PRIMARY KEY,
			report_id TEXT,
			control_id TEXT,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			data_json TEXT,
			data_summary TEXT,
			collected_at DATETIME NOT NULL,
			source TEXT,
			hash TEXT,
			FOREIGN KEY (report_id) REFERENCES compliance_reports(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_compliance_evidence_report
			ON compliance_evidence(report_id);

		CREATE INDEX IF NOT EXISTS idx_compliance_evidence_control
			ON compliance_evidence(control_id);

		CREATE TABLE IF NOT EXISTS compliance_findings (
			id TEXT PRIMARY KEY,
			report_id TEXT,
			control_id TEXT NOT NULL,
			severity TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			impact TEXT,
			remediation TEXT,
			status TEXT NOT NULL DEFAULT 'open',
			due_date DATETIME,
			resolved_at DATETIME,
			resolved_by TEXT,
			assigned_to TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (report_id) REFERENCES compliance_reports(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_compliance_findings_status
			ON compliance_findings(status);

		CREATE TABLE IF NOT EXISTS compliance_schedules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			schedule TEXT NOT NULL,
			period_days INTEGER NOT NULL DEFAULT 90,
			enabled INTEGER NOT NULL DEFAULT 1,
			recipients TEXT,
			last_run DATETIME,
			next_run DATETIME,
			created_at DATETIME NOT NULL,
			created_by TEXT NOT NULL
		);
	`)
	return err
}

// SaveReport saves a compliance report
func (s *Store) SaveReport(report *ComplianceReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO compliance_reports (id, type, title, period_start, period_end, generated_at, generated_by, status, version, report_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			version = excluded.version,
			report_json = excluded.report_json
	`, report.ID, report.Type, report.Title, report.Period.Start, report.Period.End,
		report.GeneratedAt, report.GeneratedBy, report.Status, report.Version, string(reportJSON))

	if err != nil {
		return err
	}

	// Save evidence
	for _, evidence := range report.Evidence {
		if err := s.SaveEvidence(&evidence, report.ID); err != nil {
			return fmt.Errorf("save evidence: %w", err)
		}
	}

	// Save findings from sections
	for _, section := range report.Sections {
		for _, finding := range section.Findings {
			finding.ControlID = section.ID
			if err := s.saveFinding(&finding, report.ID); err != nil {
				return fmt.Errorf("save finding: %w", err)
			}
		}
	}

	return nil
}

// GetReport retrieves a compliance report by ID
func (s *Store) GetReport(id string) (*ComplianceReport, error) {
	var reportJSON string
	err := s.db.QueryRow(`
		SELECT report_json FROM compliance_reports WHERE id = ?
	`, id).Scan(&reportJSON)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("report not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	var report ComplianceReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return nil, fmt.Errorf("unmarshal report: %w", err)
	}

	return &report, nil
}

// ListReports lists compliance reports with optional filters
func (s *Store) ListReports(filter ReportFilter) ([]ComplianceReport, error) {
	query := `SELECT report_json FROM compliance_reports WHERE 1=1`
	var args []interface{}

	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.StartTime != nil {
		query += " AND generated_at >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND generated_at <= ?"
		args = append(args, *filter.EndTime)
	}

	query += " ORDER BY generated_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var reportJSON string
		if err := rows.Scan(&reportJSON); err != nil {
			return nil, err
		}

		var report ComplianceReport
		if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
			continue
		}
		reports = append(reports, report)
	}

	return reports, rows.Err()
}

// DeleteReport deletes a compliance report
func (s *Store) DeleteReport(id string) error {
	result, err := s.db.Exec(`DELETE FROM compliance_reports WHERE id = ?`, id)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("report not found: %s", id)
	}

	return nil
}

// UpdateReportStatus updates the status of a report
func (s *Store) UpdateReportStatus(id string, status ReportStatus) error {
	// Get current report
	report, err := s.GetReport(id)
	if err != nil {
		return err
	}

	report.Status = status
	if status == StatusFinal {
		report.Version++
	}

	return s.SaveReport(report)
}

// SaveEvidence saves evidence to the store
func (s *Store) SaveEvidence(evidence *Evidence, reportID string) error {
	if evidence.ID == "" {
		evidence.ID = uuid.New().String()
	}

	dataJSON, _ := json.Marshal(evidence.Data)

	_, err := s.db.Exec(`
		INSERT INTO compliance_evidence (id, report_id, control_id, type, title, description, data_json, data_summary, collected_at, source, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			data_json = excluded.data_json,
			data_summary = excluded.data_summary
	`, evidence.ID, reportID, evidence.ControlID, evidence.Type, evidence.Title,
		evidence.Description, string(dataJSON), evidence.DataSummary, evidence.CollectedAt, evidence.Source, evidence.Hash)

	return err
}

// ListEvidence lists evidence with optional filters
func (s *Store) ListEvidence(filter EvidenceFilter) ([]Evidence, error) {
	query := `SELECT id, report_id, control_id, type, title, description, data_json, data_summary, collected_at, source, hash
		FROM compliance_evidence WHERE 1=1`
	var args []interface{}

	if filter.ReportID != "" {
		query += " AND report_id = ?"
		args = append(args, filter.ReportID)
	}
	if filter.ControlID != "" {
		query += " AND control_id = ?"
		args = append(args, filter.ControlID)
	}
	if filter.Type != "" {
		query += " AND type = ?"
		args = append(args, filter.Type)
	}
	if filter.Source != "" {
		query += " AND source = ?"
		args = append(args, filter.Source)
	}
	if filter.StartTime != nil {
		query += " AND collected_at >= ?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND collected_at <= ?"
		args = append(args, *filter.EndTime)
	}

	query += " ORDER BY collected_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evidenceList []Evidence
	for rows.Next() {
		var e Evidence
		var reportID, controlID sql.NullString
		var dataJSON string
		if err := rows.Scan(&e.ID, &reportID, &controlID, &e.Type, &e.Title, &e.Description,
			&dataJSON, &e.DataSummary, &e.CollectedAt, &e.Source, &e.Hash); err != nil {
			return nil, err
		}
		e.ReportID = reportID.String
		e.ControlID = controlID.String
		json.Unmarshal([]byte(dataJSON), &e.Data)
		evidenceList = append(evidenceList, e)
	}

	return evidenceList, rows.Err()
}

// saveFinding saves a finding to the store
func (s *Store) saveFinding(finding *Finding, reportID string) error {
	if finding.ID == "" {
		finding.ID = uuid.New().String()
	}

	var dueDate, resolvedAt interface{}
	if finding.DueDate != nil {
		dueDate = *finding.DueDate
	}
	if finding.ResolvedAt != nil {
		resolvedAt = *finding.ResolvedAt
	}

	_, err := s.db.Exec(`
		INSERT INTO compliance_findings (id, report_id, control_id, severity, title, description, impact, remediation, status, due_date, resolved_at, resolved_by, assigned_to, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			resolved_at = excluded.resolved_at,
			resolved_by = excluded.resolved_by,
			updated_at = excluded.updated_at
	`, finding.ID, reportID, finding.ControlID, finding.Severity, finding.Title,
		finding.Description, finding.Impact, finding.Remediation, finding.Status,
		dueDate, resolvedAt, finding.ResolvedBy, finding.AssignedTo,
		finding.CreatedAt, finding.UpdatedAt)

	return err
}

// ListFindings lists findings with optional filters
func (s *Store) ListFindings(reportID string, status string) ([]Finding, error) {
	query := `SELECT id, control_id, severity, title, description, impact, remediation, status, due_date, resolved_at, resolved_by, assigned_to, created_at, updated_at
		FROM compliance_findings WHERE 1=1`
	var args []interface{}

	if reportID != "" {
		query += " AND report_id = ?"
		args = append(args, reportID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY severity DESC, created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var findings []Finding
	for rows.Next() {
		var f Finding
		var dueDate, resolvedAt sql.NullTime
		var resolvedBy, assignedTo sql.NullString
		if err := rows.Scan(&f.ID, &f.ControlID, &f.Severity, &f.Title, &f.Description,
			&f.Impact, &f.Remediation, &f.Status, &dueDate, &resolvedAt, &resolvedBy, &assignedTo,
			&f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		if dueDate.Valid {
			f.DueDate = &dueDate.Time
		}
		if resolvedAt.Valid {
			f.ResolvedAt = &resolvedAt.Time
		}
		f.ResolvedBy = resolvedBy.String
		f.AssignedTo = assignedTo.String
		findings = append(findings, f)
	}

	return findings, rows.Err()
}

// UpdateFinding updates a finding
func (s *Store) UpdateFinding(finding *Finding) error {
	finding.UpdatedAt = time.Now()

	var dueDate, resolvedAt interface{}
	if finding.DueDate != nil {
		dueDate = *finding.DueDate
	}
	if finding.ResolvedAt != nil {
		resolvedAt = *finding.ResolvedAt
	}

	_, err := s.db.Exec(`
		UPDATE compliance_findings SET
			status = ?, due_date = ?, resolved_at = ?, resolved_by = ?, assigned_to = ?, updated_at = ?
		WHERE id = ?
	`, finding.Status, dueDate, resolvedAt, finding.ResolvedBy, finding.AssignedTo, finding.UpdatedAt, finding.ID)

	return err
}

// SaveSchedule saves a scheduled report configuration
func (s *Store) SaveSchedule(schedule *ScheduledReport) error {
	if schedule.ID == "" {
		schedule.ID = uuid.New().String()
		schedule.CreatedAt = time.Now()
	}

	recipientsJSON, _ := json.Marshal(schedule.Recipients)

	var lastRun, nextRun interface{}
	if schedule.LastRun != nil {
		lastRun = *schedule.LastRun
	}
	if schedule.NextRun != nil {
		nextRun = *schedule.NextRun
	}

	_, err := s.db.Exec(`
		INSERT INTO compliance_schedules (id, name, type, schedule, period_days, enabled, recipients, last_run, next_run, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			schedule = excluded.schedule,
			period_days = excluded.period_days,
			enabled = excluded.enabled,
			recipients = excluded.recipients,
			last_run = excluded.last_run,
			next_run = excluded.next_run
	`, schedule.ID, schedule.Name, schedule.Type, schedule.Schedule, schedule.PeriodDays,
		schedule.Enabled, string(recipientsJSON), lastRun, nextRun, schedule.CreatedAt, schedule.CreatedBy)

	return err
}

// ListSchedules lists all scheduled reports
func (s *Store) ListSchedules() ([]ScheduledReport, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, schedule, period_days, enabled, recipients, last_run, next_run, created_at, created_by
		FROM compliance_schedules ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []ScheduledReport
	for rows.Next() {
		var sched ScheduledReport
		var recipientsJSON string
		var lastRun, nextRun sql.NullTime
		if err := rows.Scan(&sched.ID, &sched.Name, &sched.Type, &sched.Schedule, &sched.PeriodDays,
			&sched.Enabled, &recipientsJSON, &lastRun, &nextRun, &sched.CreatedAt, &sched.CreatedBy); err != nil {
			return nil, err
		}
		if lastRun.Valid {
			sched.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			sched.NextRun = &nextRun.Time
		}
		json.Unmarshal([]byte(recipientsJSON), &sched.Recipients)
		schedules = append(schedules, sched)
	}

	return schedules, rows.Err()
}

// DeleteSchedule deletes a scheduled report
func (s *Store) DeleteSchedule(id string) error {
	_, err := s.db.Exec(`DELETE FROM compliance_schedules WHERE id = ?`, id)
	return err
}

// GetControlStatus returns the latest status for a control
func (s *Store) GetControlStatus(controlID string) (*ReportSection, error) {
	var reportJSON string
	err := s.db.QueryRow(`
		SELECT report_json FROM compliance_reports
		WHERE status IN ('draft', 'final')
		ORDER BY generated_at DESC LIMIT 1
	`).Scan(&reportJSON)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no reports found")
	}
	if err != nil {
		return nil, err
	}

	var report ComplianceReport
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return nil, err
	}

	for _, section := range report.Sections {
		if section.ID == controlID || section.Control == controlID {
			return &section, nil
		}
	}

	return nil, fmt.Errorf("control not found: %s", controlID)
}
