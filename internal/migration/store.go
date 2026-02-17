package migration

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Store handles migration report persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new migration store
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
	CREATE TABLE IF NOT EXISTS migration_reports (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		completed_at INTEGER,
		duration_ms INTEGER,
		dashboards_total INTEGER DEFAULT 0,
		dashboards_imported INTEGER DEFAULT 0,
		dashboards_skipped INTEGER DEFAULT 0,
		dashboards_failed INTEGER DEFAULT 0,
		alerts_total INTEGER DEFAULT 0,
		alerts_imported INTEGER DEFAULT 0,
		alerts_skipped INTEGER DEFAULT 0,
		alerts_failed INTEGER DEFAULT 0,
		details TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now'))
	);

	CREATE TABLE IF NOT EXISTS imported_dashboards (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		source_id TEXT,
		source_name TEXT NOT NULL,
		target_id TEXT NOT NULL,
		widgets_total INTEGER DEFAULT 0,
		widgets_converted INTEGER DEFAULT 0,
		widgets_skipped INTEGER DEFAULT 0,
		warnings TEXT,
		error TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (report_id) REFERENCES migration_reports(id)
	);

	CREATE TABLE IF NOT EXISTS imported_alerts (
		id TEXT PRIMARY KEY,
		report_id TEXT NOT NULL,
		source_id TEXT,
		source_name TEXT NOT NULL,
		target_id TEXT NOT NULL,
		source_query TEXT,
		warnings TEXT,
		error TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY (report_id) REFERENCES migration_reports(id)
	);

	CREATE INDEX IF NOT EXISTS idx_reports_source ON migration_reports(source);
	CREATE INDEX IF NOT EXISTS idx_reports_created ON migration_reports(created_at);
	CREATE INDEX IF NOT EXISTS idx_dashboards_report ON imported_dashboards(report_id);
	CREATE INDEX IF NOT EXISTS idx_alerts_report ON imported_alerts(report_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveReport saves a migration report
func (s *Store) SaveReport(report *MigrationReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize details
	details := map[string]any{
		"warnings":     report.Warnings,
		"errors":       report.Errors,
		"manual_steps": report.ManualSteps,
	}
	detailsJSON, _ := json.Marshal(details)

	_, err := s.db.Exec(`
		INSERT INTO migration_reports (
			id, source, started_at, completed_at, duration_ms,
			dashboards_total, dashboards_imported, dashboards_skipped, dashboards_failed,
			alerts_total, alerts_imported, alerts_skipped, alerts_failed, details
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.Source, report.StartedAt.Unix(), report.CompletedAt.Unix(),
		report.Duration.Milliseconds(),
		report.DashboardsTotal, report.DashboardsImported, report.DashboardsSkipped, report.DashboardsFailed,
		report.AlertsTotal, report.AlertsImported, report.AlertsSkipped, report.AlertsFailed,
		string(detailsJSON))

	if err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}

	// Save dashboard results
	for _, dr := range report.DashboardResults {
		warnings, _ := json.Marshal(dr.Warnings)
		_, err := s.db.Exec(`
			INSERT INTO imported_dashboards (
				id, report_id, source_id, source_name, target_id,
				widgets_total, widgets_converted, widgets_skipped, warnings, error
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			generateID(), report.ID, dr.SourceID, dr.SourceName, dr.TargetID,
			dr.WidgetsTotal, dr.WidgetsConverted, dr.WidgetsSkipped,
			string(warnings), dr.Error)
		if err != nil {
			return fmt.Errorf("failed to save dashboard result: %w", err)
		}
	}

	// Save alert results
	for _, ar := range report.AlertResults {
		warnings, _ := json.Marshal(ar.Warnings)
		_, err := s.db.Exec(`
			INSERT INTO imported_alerts (
				id, report_id, source_id, source_name, target_id, warnings, error
			) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			generateID(), report.ID, ar.SourceID, ar.SourceName, ar.TargetID,
			string(warnings), ar.Error)
		if err != nil {
			return fmt.Errorf("failed to save alert result: %w", err)
		}
	}

	return nil
}

// GetReport retrieves a migration report by ID
func (s *Store) GetReport(id string) (*MigrationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := &MigrationReport{}
	var startedAt, completedAt int64
	var durationMs int64
	var detailsJSON string

	err := s.db.QueryRow(`
		SELECT id, source, started_at, completed_at, duration_ms,
			dashboards_total, dashboards_imported, dashboards_skipped, dashboards_failed,
			alerts_total, alerts_imported, alerts_skipped, alerts_failed, details
		FROM migration_reports WHERE id = ?`, id).Scan(
		&report.ID, &report.Source, &startedAt, &completedAt, &durationMs,
		&report.DashboardsTotal, &report.DashboardsImported, &report.DashboardsSkipped, &report.DashboardsFailed,
		&report.AlertsTotal, &report.AlertsImported, &report.AlertsSkipped, &report.AlertsFailed,
		&detailsJSON)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	report.StartedAt = time.Unix(startedAt, 0)
	report.CompletedAt = time.Unix(completedAt, 0)
	report.Duration = time.Duration(durationMs) * time.Millisecond

	// Parse details
	var details struct {
		Warnings    []string      `json:"warnings"`
		Errors      []ImportError `json:"errors"`
		ManualSteps []string      `json:"manual_steps"`
	}
	if err := json.Unmarshal([]byte(detailsJSON), &details); err == nil {
		report.Warnings = details.Warnings
		report.Errors = details.Errors
		report.ManualSteps = details.ManualSteps
	}

	// Load dashboard results
	dashRows, err := s.db.Query(`
		SELECT source_id, source_name, target_id, widgets_total, widgets_converted,
			widgets_skipped, warnings, error
		FROM imported_dashboards WHERE report_id = ?`, id)
	if err == nil {
		defer dashRows.Close()
		for dashRows.Next() {
			var dr DashboardResult
			var warnings string
			if err := dashRows.Scan(&dr.SourceID, &dr.SourceName, &dr.TargetID,
				&dr.WidgetsTotal, &dr.WidgetsConverted, &dr.WidgetsSkipped,
				&warnings, &dr.Error); err == nil {
				json.Unmarshal([]byte(warnings), &dr.Warnings)
				dr.Success = dr.Error == ""
				report.DashboardResults = append(report.DashboardResults, dr)
			}
		}
	}

	// Load alert results
	alertRows, err := s.db.Query(`
		SELECT source_id, source_name, target_id, warnings, error
		FROM imported_alerts WHERE report_id = ?`, id)
	if err == nil {
		defer alertRows.Close()
		for alertRows.Next() {
			var ar AlertResult
			var warnings string
			if err := alertRows.Scan(&ar.SourceID, &ar.SourceName, &ar.TargetID,
				&warnings, &ar.Error); err == nil {
				json.Unmarshal([]byte(warnings), &ar.Warnings)
				ar.Success = ar.Error == ""
				report.AlertResults = append(report.AlertResults, ar)
			}
		}
	}

	return report, nil
}

// ListReports lists all migration reports
func (s *Store) ListReports(limit int) ([]*MigrationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, source, started_at, completed_at, duration_ms,
			dashboards_total, dashboards_imported, dashboards_skipped, dashboards_failed,
			alerts_total, alerts_imported, alerts_skipped, alerts_failed
		FROM migration_reports ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	defer rows.Close()

	var reports []*MigrationReport
	for rows.Next() {
		report := &MigrationReport{}
		var startedAt, completedAt int64
		var durationMs int64

		if err := rows.Scan(
			&report.ID, &report.Source, &startedAt, &completedAt, &durationMs,
			&report.DashboardsTotal, &report.DashboardsImported, &report.DashboardsSkipped, &report.DashboardsFailed,
			&report.AlertsTotal, &report.AlertsImported, &report.AlertsSkipped, &report.AlertsFailed,
		); err != nil {
			continue
		}

		report.StartedAt = time.Unix(startedAt, 0)
		report.CompletedAt = time.Unix(completedAt, 0)
		report.Duration = time.Duration(durationMs) * time.Millisecond

		reports = append(reports, report)
	}

	return reports, nil
}

// DeleteReport deletes a migration report
func (s *Store) DeleteReport(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete related records first
	s.db.Exec("DELETE FROM imported_dashboards WHERE report_id = ?", id)
	s.db.Exec("DELETE FROM imported_alerts WHERE report_id = ?", id)

	_, err := s.db.Exec("DELETE FROM migration_reports WHERE id = ?", id)
	return err
}

// GetReportsBySource gets reports filtered by source platform
func (s *Store) GetReportsBySource(source SourcePlatform, limit int) ([]*MigrationReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, source, started_at, completed_at, duration_ms,
			dashboards_total, dashboards_imported, dashboards_skipped, dashboards_failed,
			alerts_total, alerts_imported, alerts_skipped, alerts_failed
		FROM migration_reports WHERE source = ? ORDER BY created_at DESC LIMIT ?`,
		source, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	defer rows.Close()

	var reports []*MigrationReport
	for rows.Next() {
		report := &MigrationReport{}
		var startedAt, completedAt int64
		var durationMs int64

		if err := rows.Scan(
			&report.ID, &report.Source, &startedAt, &completedAt, &durationMs,
			&report.DashboardsTotal, &report.DashboardsImported, &report.DashboardsSkipped, &report.DashboardsFailed,
			&report.AlertsTotal, &report.AlertsImported, &report.AlertsSkipped, &report.AlertsFailed,
		); err != nil {
			continue
		}

		report.StartedAt = time.Unix(startedAt, 0)
		report.CompletedAt = time.Unix(completedAt, 0)
		report.Duration = time.Duration(durationMs) * time.Millisecond

		reports = append(reports, report)
	}

	return reports, nil
}

// GetStats returns migration statistics
func (s *Store) GetStats() (*MigrationStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &MigrationStats{
		BySource: make(map[SourcePlatform]SourceStats),
	}

	// Get total counts
	row := s.db.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM(dashboards_imported), 0),
			COALESCE(SUM(alerts_imported), 0)
		FROM migration_reports`)
	row.Scan(&stats.TotalMigrations, &stats.TotalDashboards, &stats.TotalAlerts)

	// Get counts by source
	rows, err := s.db.Query(`
		SELECT source, COUNT(*),
			COALESCE(SUM(dashboards_imported), 0),
			COALESCE(SUM(alerts_imported), 0)
		FROM migration_reports GROUP BY source`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var source SourcePlatform
			var ss SourceStats
			if err := rows.Scan(&source, &ss.Migrations, &ss.Dashboards, &ss.Alerts); err == nil {
				stats.BySource[source] = ss
			}
		}
	}

	// Get last migration time
	var lastMigration sql.NullInt64
	s.db.QueryRow("SELECT MAX(completed_at) FROM migration_reports").Scan(&lastMigration)
	if lastMigration.Valid {
		t := time.Unix(lastMigration.Int64, 0)
		stats.LastMigration = &t
	}

	return stats, nil
}

// MigrationStats contains aggregate migration statistics
type MigrationStats struct {
	TotalMigrations int                          `json:"total_migrations"`
	TotalDashboards int                          `json:"total_dashboards"`
	TotalAlerts     int                          `json:"total_alerts"`
	BySource        map[SourcePlatform]SourceStats `json:"by_source"`
	LastMigration   *time.Time                   `json:"last_migration,omitempty"`
}

// SourceStats contains statistics for a source platform
type SourceStats struct {
	Migrations int `json:"migrations"`
	Dashboards int `json:"dashboards"`
	Alerts     int `json:"alerts"`
}

func generateID() string {
	return fmt.Sprintf("imp_%d", time.Now().UnixNano())
}
