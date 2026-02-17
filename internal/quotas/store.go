package quotas

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Store persists team quotas and usage data
type Store struct {
	db *sql.DB
	mu sync.RWMutex

	// In-memory cache for fast quota checks
	quotaCache map[string][]*Quota // teamID -> quotas
	cacheMu    sync.RWMutex
}

// NewStore creates a new quota store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{
		db:         db,
		quotaCache: make(map[string][]*Quota),
	}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	// Load quotas into cache
	if err := store.refreshCache(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) createTables() error {
	schema := `
	-- Teams table
	CREATE TABLE IF NOT EXISTS teams (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		org_id TEXT NOT NULL,
		cost_center TEXT,
		owner_email TEXT,
		tags TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_teams_org ON teams(org_id);
	CREATE INDEX IF NOT EXISTS idx_teams_cost_center ON teams(cost_center);

	-- Quotas table
	CREATE TABLE IF NOT EXISTS quotas (
		id TEXT PRIMARY KEY,
		team_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		resource_type TEXT NOT NULL,
		unit TEXT NOT NULL,
		limit_value INTEGER NOT NULL,
		warn_at REAL DEFAULT 0.8,
		enforcement TEXT DEFAULT 'warn',
		period TEXT DEFAULT 'monthly',
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_quotas_team ON quotas(team_id);
	CREATE INDEX IF NOT EXISTS idx_quotas_resource ON quotas(resource_type);

	-- Usage tracking table (per period)
	CREATE TABLE IF NOT EXISTS usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		unit TEXT NOT NULL,
		amount INTEGER NOT NULL,
		period TEXT NOT NULL,
		period_start DATETIME NOT NULL,
		period_end DATETIME NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(team_id, resource_type, period_start)
	);

	CREATE INDEX IF NOT EXISTS idx_usage_team_period ON usage(team_id, period_start);
	CREATE INDEX IF NOT EXISTS idx_usage_resource ON usage(resource_type);

	-- Usage events (detailed tracking)
	CREATE TABLE IF NOT EXISTS usage_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		amount INTEGER NOT NULL,
		unit TEXT NOT NULL,
		source TEXT,
		metadata TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_usage_events_team_time ON usage_events(team_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_usage_events_source ON usage_events(source);

	-- Quota violations
	CREATE TABLE IF NOT EXISTS quota_violations (
		id TEXT PRIMARY KEY,
		quota_id TEXT NOT NULL,
		team_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		usage_amount INTEGER NOT NULL,
		limit_value INTEGER NOT NULL,
		percentage REAL NOT NULL,
		action_taken TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		resolved INTEGER DEFAULT 0,
		resolved_at DATETIME,
		FOREIGN KEY (quota_id) REFERENCES quotas(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_violations_team ON quota_violations(team_id);
	CREATE INDEX IF NOT EXISTS idx_violations_time ON quota_violations(timestamp);

	-- Chargeback reports (cached)
	CREATE TABLE IF NOT EXISTS chargeback_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT NOT NULL,
		period TEXT NOT NULL,
		period_start DATETIME NOT NULL,
		period_end DATETIME NOT NULL,
		total_cost REAL NOT NULL,
		breakdown TEXT NOT NULL,
		comparisons TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_chargeback_team_period ON chargeback_reports(team_id, period_start);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *Store) refreshCache() error {
	rows, err := s.db.Query(`
		SELECT id, team_id, name, description, resource_type, unit, limit_value,
		       warn_at, enforcement, period, enabled, created_at, updated_at
		FROM quotas WHERE enabled = 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cache := make(map[string][]*Quota)
	for rows.Next() {
		var q Quota
		var enabled int
		err := rows.Scan(
			&q.ID, &q.TeamID, &q.Name, &q.Description, &q.ResourceType,
			&q.Unit, &q.Limit, &q.WarnAt, &q.Enforcement, &q.Period,
			&enabled, &q.CreatedAt, &q.UpdatedAt,
		)
		if err != nil {
			return err
		}
		q.Enabled = enabled == 1
		cache[q.TeamID] = append(cache[q.TeamID], &q)
	}

	s.cacheMu.Lock()
	s.quotaCache = cache
	s.cacheMu.Unlock()

	return nil
}

// Close closes the store
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateTeam creates a new team
func (s *Store) CreateTeam(team *Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if team.CreatedAt.IsZero() {
		team.CreatedAt = time.Now()
	}
	team.UpdatedAt = time.Now()

	tagsJSON, _ := json.Marshal(team.Tags)

	_, err := s.db.Exec(`
		INSERT INTO teams (id, name, description, org_id, cost_center, owner_email, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, team.ID, team.Name, team.Description, team.OrgID, team.CostCenter,
		team.OwnerEmail, string(tagsJSON), team.CreatedAt, team.UpdatedAt)

	return err
}

// UpdateTeam updates a team
func (s *Store) UpdateTeam(team *Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	team.UpdatedAt = time.Now()
	tagsJSON, _ := json.Marshal(team.Tags)

	_, err := s.db.Exec(`
		UPDATE teams SET name = ?, description = ?, cost_center = ?,
		       owner_email = ?, tags = ?, updated_at = ?
		WHERE id = ?
	`, team.Name, team.Description, team.CostCenter,
		team.OwnerEmail, string(tagsJSON), team.UpdatedAt, team.ID)

	return err
}

// DeleteTeam deletes a team
func (s *Store) DeleteTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM teams WHERE id = ?`, id)
	if err != nil {
		return err
	}

	// Clear from cache
	s.cacheMu.Lock()
	delete(s.quotaCache, id)
	s.cacheMu.Unlock()

	return nil
}

// GetTeam returns a team by ID
func (s *Store) GetTeam(id string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var team Team
	var tagsJSON sql.NullString

	err := s.db.QueryRow(`
		SELECT id, name, description, org_id, cost_center, owner_email, tags, created_at, updated_at
		FROM teams WHERE id = ?
	`, id).Scan(
		&team.ID, &team.Name, &team.Description, &team.OrgID,
		&team.CostCenter, &team.OwnerEmail, &tagsJSON,
		&team.CreatedAt, &team.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if tagsJSON.Valid {
		json.Unmarshal([]byte(tagsJSON.String), &team.Tags)
	}

	return &team, nil
}

// ListTeams returns all teams for an org
func (s *Store) ListTeams(orgID string) ([]*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, name, description, org_id, cost_center, owner_email, tags, created_at, updated_at FROM teams`
	args := []interface{}{}

	if orgID != "" {
		query += ` WHERE org_id = ?`
		args = append(args, orgID)
	}
	query += ` ORDER BY name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var team Team
		var tagsJSON sql.NullString
		err := rows.Scan(
			&team.ID, &team.Name, &team.Description, &team.OrgID,
			&team.CostCenter, &team.OwnerEmail, &tagsJSON,
			&team.CreatedAt, &team.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if tagsJSON.Valid {
			json.Unmarshal([]byte(tagsJSON.String), &team.Tags)
		}
		teams = append(teams, &team)
	}

	return teams, nil
}

// CreateQuota creates a new quota
func (s *Store) CreateQuota(quota *Quota) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if quota.CreatedAt.IsZero() {
		quota.CreatedAt = time.Now()
	}
	quota.UpdatedAt = time.Now()

	enabled := 0
	if quota.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO quotas (id, team_id, name, description, resource_type, unit,
		                   limit_value, warn_at, enforcement, period, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, quota.ID, quota.TeamID, quota.Name, quota.Description, quota.ResourceType,
		quota.Unit, quota.Limit, quota.WarnAt, quota.Enforcement, quota.Period,
		enabled, quota.CreatedAt, quota.UpdatedAt)

	if err != nil {
		return err
	}

	// Update cache
	return s.refreshCache()
}

// UpdateQuota updates a quota
func (s *Store) UpdateQuota(quota *Quota) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	quota.UpdatedAt = time.Now()
	enabled := 0
	if quota.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(`
		UPDATE quotas SET name = ?, description = ?, resource_type = ?, unit = ?,
		       limit_value = ?, warn_at = ?, enforcement = ?, period = ?,
		       enabled = ?, updated_at = ?
		WHERE id = ?
	`, quota.Name, quota.Description, quota.ResourceType, quota.Unit,
		quota.Limit, quota.WarnAt, quota.Enforcement, quota.Period,
		enabled, quota.UpdatedAt, quota.ID)

	if err != nil {
		return err
	}

	return s.refreshCache()
}

// DeleteQuota deletes a quota
func (s *Store) DeleteQuota(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM quotas WHERE id = ?`, id)
	if err != nil {
		return err
	}

	return s.refreshCache()
}

// GetQuota returns a quota by ID
func (s *Store) GetQuota(id string) (*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var quota Quota
	var enabled int

	err := s.db.QueryRow(`
		SELECT id, team_id, name, description, resource_type, unit, limit_value,
		       warn_at, enforcement, period, enabled, created_at, updated_at
		FROM quotas WHERE id = ?
	`, id).Scan(
		&quota.ID, &quota.TeamID, &quota.Name, &quota.Description,
		&quota.ResourceType, &quota.Unit, &quota.Limit, &quota.WarnAt,
		&quota.Enforcement, &quota.Period, &enabled,
		&quota.CreatedAt, &quota.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	quota.Enabled = enabled == 1

	return &quota, nil
}

// ListQuotas returns all quotas for a team
func (s *Store) ListQuotas(teamID string) ([]*Quota, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, team_id, name, description, resource_type, unit, limit_value,
	          warn_at, enforcement, period, enabled, created_at, updated_at
	          FROM quotas`
	args := []interface{}{}

	if teamID != "" {
		query += ` WHERE team_id = ?`
		args = append(args, teamID)
	}
	query += ` ORDER BY resource_type, name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotas []*Quota
	for rows.Next() {
		var q Quota
		var enabled int
		err := rows.Scan(
			&q.ID, &q.TeamID, &q.Name, &q.Description, &q.ResourceType,
			&q.Unit, &q.Limit, &q.WarnAt, &q.Enforcement, &q.Period,
			&enabled, &q.CreatedAt, &q.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		q.Enabled = enabled == 1
		quotas = append(quotas, &q)
	}

	return quotas, nil
}

// GetQuotasForTeam returns cached quotas for fast checking
func (s *Store) GetQuotasForTeam(teamID string) []*Quota {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.quotaCache[teamID]
}

// RecordUsage records a usage event and updates period totals
func (s *Store) RecordUsage(record *UsageRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	// Insert usage event
	_, err := s.db.Exec(`
		INSERT INTO usage_events (team_id, resource_type, amount, unit, source, metadata, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, record.TeamID, record.ResourceType, record.Amount, record.Unit,
		record.Source, record.Metadata, record.Timestamp)
	if err != nil {
		return err
	}

	// Update period totals for multiple periods
	periods := []string{"hourly", "daily", "monthly"}
	for _, period := range periods {
		start, end := GetPeriodBounds(period, record.Timestamp)

		_, err = s.db.Exec(`
			INSERT INTO usage (team_id, resource_type, unit, amount, period, period_start, period_end, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(team_id, resource_type, period_start) DO UPDATE SET
				amount = amount + excluded.amount,
				updated_at = excluded.updated_at
		`, record.TeamID, record.ResourceType, record.Unit, record.Amount,
			period, start, end, time.Now())
		if err != nil {
			return err
		}
	}

	return nil
}

// GetUsage returns current usage for a team/resource/period
func (s *Store) GetUsage(teamID string, resourceType ResourceType, period string) (*Usage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start, end := GetPeriodBounds(period, time.Now())

	var usage Usage
	err := s.db.QueryRow(`
		SELECT team_id, resource_type, unit, amount, period, period_start, period_end, updated_at
		FROM usage
		WHERE team_id = ? AND resource_type = ? AND period_start = ?
	`, teamID, resourceType, start).Scan(
		&usage.TeamID, &usage.ResourceType, &usage.Unit, &usage.Current,
		&usage.Period, &usage.PeriodStart, &usage.PeriodEnd, &usage.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return &Usage{
			TeamID:       teamID,
			ResourceType: resourceType,
			Period:       period,
			PeriodStart:  start,
			PeriodEnd:    end,
			Current:      0,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return &usage, nil
}

// GetTeamUsageSummary returns usage across all resource types for a team
func (s *Store) GetTeamUsageSummary(teamID string, period string) ([]*Usage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start, _ := GetPeriodBounds(period, time.Now())

	rows, err := s.db.Query(`
		SELECT team_id, resource_type, unit, amount, period, period_start, period_end, updated_at
		FROM usage
		WHERE team_id = ? AND period = ? AND period_start = ?
	`, teamID, period, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []*Usage
	for rows.Next() {
		var u Usage
		err := rows.Scan(
			&u.TeamID, &u.ResourceType, &u.Unit, &u.Current,
			&u.Period, &u.PeriodStart, &u.PeriodEnd, &u.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		usages = append(usages, &u)
	}

	return usages, nil
}

// RecordViolation records a quota violation
func (s *Store) RecordViolation(violation *QuotaViolation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if violation.Timestamp.IsZero() {
		violation.Timestamp = time.Now()
	}

	_, err := s.db.Exec(`
		INSERT INTO quota_violations (id, quota_id, team_id, resource_type, usage_amount,
		                             limit_value, percentage, action_taken, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, violation.ID, violation.QuotaID, violation.TeamID, violation.ResourceType,
		violation.Usage, violation.Limit, violation.Percentage,
		violation.Action, violation.Timestamp)

	return err
}

// GetViolations returns recent quota violations
func (s *Store) GetViolations(teamID string, limit int) ([]*QuotaViolation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, quota_id, team_id, resource_type, usage_amount, limit_value,
	          percentage, action_taken, timestamp, resolved, resolved_at
	          FROM quota_violations`
	args := []interface{}{}

	if teamID != "" {
		query += ` WHERE team_id = ?`
		args = append(args, teamID)
	}
	query += ` ORDER BY timestamp DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []*QuotaViolation
	for rows.Next() {
		var v QuotaViolation
		var resolved int
		var resolvedAt sql.NullTime
		err := rows.Scan(
			&v.ID, &v.QuotaID, &v.TeamID, &v.ResourceType, &v.Usage,
			&v.Limit, &v.Percentage, &v.Action, &v.Timestamp,
			&resolved, &resolvedAt,
		)
		if err != nil {
			return nil, err
		}
		v.Resolved = resolved == 1
		if resolvedAt.Valid {
			v.ResolvedAt = &resolvedAt.Time
		}
		violations = append(violations, &v)
	}

	return violations, nil
}

// SaveChargebackReport saves a chargeback report
func (s *Store) SaveChargebackReport(report *ChargebackReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	breakdownJSON, _ := json.Marshal(report.Breakdown)
	comparisonsJSON, _ := json.Marshal(report.Comparisons)

	_, err := s.db.Exec(`
		INSERT INTO chargeback_reports (team_id, period, period_start, period_end, total_cost, breakdown, comparisons)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, report.TeamID, report.Period, report.PeriodStart, report.PeriodEnd,
		report.TotalCost, string(breakdownJSON), string(comparisonsJSON))

	return err
}

// GetChargebackReport retrieves a chargeback report
func (s *Store) GetChargebackReport(teamID string, periodStart time.Time) (*ChargebackReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var report ChargebackReport
	var breakdownJSON, comparisonsJSON string

	err := s.db.QueryRow(`
		SELECT team_id, period, period_start, period_end, total_cost, breakdown, comparisons
		FROM chargeback_reports
		WHERE team_id = ? AND period_start = ?
	`, teamID, periodStart).Scan(
		&report.TeamID, &report.Period, &report.PeriodStart, &report.PeriodEnd,
		&report.TotalCost, &breakdownJSON, &comparisonsJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(breakdownJSON), &report.Breakdown)
	json.Unmarshal([]byte(comparisonsJSON), &report.Comparisons)

	return &report, nil
}

// GetUsageByService returns usage breakdown by service for a team
func (s *Store) GetUsageByService(teamID string, period string) ([]ServiceUsage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start, end := GetPeriodBounds(period, time.Now())

	rows, err := s.db.Query(`
		SELECT source, resource_type, SUM(amount) as total
		FROM usage_events
		WHERE team_id = ? AND timestamp >= ? AND timestamp < ? AND source != ''
		GROUP BY source, resource_type
		ORDER BY total DESC
	`, teamID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []ServiceUsage
	for rows.Next() {
		var u ServiceUsage
		var source sql.NullString
		err := rows.Scan(&source, &u.ResourceType, &u.Usage)
		if err != nil {
			return nil, err
		}
		if source.Valid {
			u.Service = source.String
		}
		usages = append(usages, u)
	}

	return usages, nil
}
