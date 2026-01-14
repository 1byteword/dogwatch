// Package catalog provides service catalog functionality for ownership,
// dependencies, and service metadata management.
package catalog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ServiceTier represents the criticality level of a service
type ServiceTier string

const (
	TierCritical ServiceTier = "critical" // P0 - Revenue impacting, customer facing
	TierHigh     ServiceTier = "high"     // P1 - Important but not critical
	TierMedium   ServiceTier = "medium"   // P2 - Internal tools, batch jobs
	TierLow      ServiceTier = "low"      // P3 - Development, testing
)

// ServiceLifecycle represents the lifecycle state of a service
type ServiceLifecycle string

const (
	LifecycleActive     ServiceLifecycle = "active"
	LifecycleDegraded   ServiceLifecycle = "degraded"
	LifecycleDeprecated ServiceLifecycle = "deprecated"
	LifecycleSunset     ServiceLifecycle = "sunset"
)

// ServiceHealth represents the current health status
type ServiceHealth string

const (
	HealthHealthy  ServiceHealth = "healthy"
	HealthDegraded ServiceHealth = "degraded"
	HealthUnhealthy ServiceHealth = "unhealthy"
	HealthUnknown  ServiceHealth = "unknown"
)

// Service represents a service in the catalog
type Service struct {
	ID          string           `json:"id"`
	OrgID       string           `json:"org_id"`
	Name        string           `json:"name"`
	DisplayName string           `json:"display_name,omitempty"`
	Description string           `json:"description,omitempty"`
	Tier        ServiceTier      `json:"tier"`
	Lifecycle   ServiceLifecycle `json:"lifecycle"`
	Health      ServiceHealth    `json:"health"`

	// Ownership
	TeamID      string   `json:"team_id,omitempty"`
	TeamName    string   `json:"team_name,omitempty"`
	OwnerEmail  string   `json:"owner_email,omitempty"`
	OnCallID    string   `json:"oncall_schedule_id,omitempty"`
	EscalationID string  `json:"escalation_policy_id,omitempty"`
	SlackChannel string  `json:"slack_channel,omitempty"`

	// Links
	RepoURL       string `json:"repo_url,omitempty"`
	DocsURL       string `json:"docs_url,omitempty"`
	DashboardID   string `json:"dashboard_id,omitempty"`
	RunbookURL    string `json:"runbook_url,omitempty"`
	StatusPageID  string `json:"statuspage_component_id,omitempty"`

	// Technical metadata
	Language    string            `json:"language,omitempty"`
	Framework   string            `json:"framework,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`

	// Kubernetes metadata (auto-discovered)
	K8sNamespace  string `json:"k8s_namespace,omitempty"`
	K8sDeployment string `json:"k8s_deployment,omitempty"`
	K8sService    string `json:"k8s_service,omitempty"`

	// Linked monitoring
	SLOID           string   `json:"slo_id,omitempty"`
	SyntheticIDs    []string `json:"synthetic_check_ids,omitempty"`
	AlertRuleIDs    []string `json:"alert_rule_ids,omitempty"`

	// Computed fields
	IncidentCount   int       `json:"incident_count_30d"`
	LastIncident    *time.Time `json:"last_incident,omitempty"`
	UptimePercent   float64   `json:"uptime_percent_30d"`
	AvgResponseTime float64   `json:"avg_response_time_ms"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Dependency represents a dependency between services
type Dependency struct {
	ID             string         `json:"id"`
	SourceService  string         `json:"source_service_id"`  // The service that depends
	TargetService  string         `json:"target_service_id"`  // The service being depended on
	DependencyType DependencyType `json:"type"`
	Description    string         `json:"description,omitempty"`
	IsAutoDetected bool           `json:"is_auto_detected"`   // From traces vs manual
	Confidence     float64        `json:"confidence"`         // For auto-detected (0-1)
	LastSeenAt     time.Time      `json:"last_seen_at"`
	CreatedAt      time.Time      `json:"created_at"`
}

// DependencyType represents the type of dependency
type DependencyType string

const (
	DepTypeSync      DependencyType = "sync"      // Synchronous HTTP/gRPC call
	DepTypeAsync     DependencyType = "async"     // Async messaging (Kafka, etc)
	DepTypeDatabase  DependencyType = "database"  // Database connection
	DepTypeCache     DependencyType = "cache"     // Cache (Redis, Memcached)
	DepTypeExternal  DependencyType = "external"  // External API
)

// Team represents a team that owns services
type Team struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	SlackChannel string   `json:"slack_channel,omitempty"`
	Email       string    `json:"email,omitempty"`
	OnCallID    string    `json:"oncall_schedule_id,omitempty"`
	Members     []string  `json:"member_ids,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Runbook represents a runbook for a service
type Runbook struct {
	ID          string    `json:"id"`
	ServiceID   string    `json:"service_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Content     string    `json:"content"`           // Markdown content
	ExternalURL string    `json:"external_url,omitempty"` // Or link to external
	AlertTags   []string  `json:"alert_tags,omitempty"`   // Auto-attach to alerts with these tags
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store provides service catalog persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new catalog store
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
	CREATE TABLE IF NOT EXISTS services (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		name TEXT NOT NULL,
		display_name TEXT,
		description TEXT,
		tier TEXT DEFAULT 'medium',
		lifecycle TEXT DEFAULT 'active',
		health TEXT DEFAULT 'unknown',
		team_id TEXT,
		team_name TEXT,
		owner_email TEXT,
		oncall_schedule_id TEXT,
		escalation_policy_id TEXT,
		slack_channel TEXT,
		repo_url TEXT,
		docs_url TEXT,
		dashboard_id TEXT,
		runbook_url TEXT,
		statuspage_component_id TEXT,
		language TEXT,
		framework TEXT,
		tags TEXT,
		metadata TEXT,
		k8s_namespace TEXT,
		k8s_deployment TEXT,
		k8s_service TEXT,
		slo_id TEXT,
		synthetic_check_ids TEXT,
		alert_rule_ids TEXT,
		incident_count_30d INTEGER DEFAULT 0,
		last_incident INTEGER,
		uptime_percent_30d REAL DEFAULT 100,
		avg_response_time_ms REAL DEFAULT 0,
		created_at INTEGER,
		updated_at INTEGER,
		UNIQUE(org_id, name)
	);

	CREATE TABLE IF NOT EXISTS dependencies (
		id TEXT PRIMARY KEY,
		source_service_id TEXT NOT NULL,
		target_service_id TEXT NOT NULL,
		dependency_type TEXT DEFAULT 'sync',
		description TEXT,
		is_auto_detected INTEGER DEFAULT 0,
		confidence REAL DEFAULT 1.0,
		last_seen_at INTEGER,
		created_at INTEGER,
		UNIQUE(source_service_id, target_service_id)
	);

	CREATE TABLE IF NOT EXISTS teams (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		slack_channel TEXT,
		email TEXT,
		oncall_schedule_id TEXT,
		member_ids TEXT,
		created_at INTEGER,
		updated_at INTEGER,
		UNIQUE(org_id, name)
	);

	CREATE TABLE IF NOT EXISTS runbooks (
		id TEXT PRIMARY KEY,
		service_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		content TEXT,
		external_url TEXT,
		alert_tags TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_services_org ON services(org_id);
	CREATE INDEX IF NOT EXISTS idx_services_team ON services(team_id);
	CREATE INDEX IF NOT EXISTS idx_services_name ON services(org_id, name);
	CREATE INDEX IF NOT EXISTS idx_deps_source ON dependencies(source_service_id);
	CREATE INDEX IF NOT EXISTS idx_deps_target ON dependencies(target_service_id);
	CREATE INDEX IF NOT EXISTS idx_teams_org ON teams(org_id);
	CREATE INDEX IF NOT EXISTS idx_runbooks_service ON runbooks(service_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateService creates a new service
func (s *Store) CreateService(svc *Service) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	svc.CreatedAt = now
	svc.UpdatedAt = now

	if svc.Tier == "" {
		svc.Tier = TierMedium
	}
	if svc.Lifecycle == "" {
		svc.Lifecycle = LifecycleActive
	}
	if svc.Health == "" {
		svc.Health = HealthUnknown
	}

	tags, _ := json.Marshal(svc.Tags)
	metadata, _ := json.Marshal(svc.Metadata)
	syntheticIDs, _ := json.Marshal(svc.SyntheticIDs)
	alertRuleIDs, _ := json.Marshal(svc.AlertRuleIDs)

	_, err := s.db.Exec(`
		INSERT INTO services (
			id, org_id, name, display_name, description, tier, lifecycle, health,
			team_id, team_name, owner_email, oncall_schedule_id, escalation_policy_id,
			slack_channel, repo_url, docs_url, dashboard_id, runbook_url,
			statuspage_component_id, language, framework, tags, metadata,
			k8s_namespace, k8s_deployment, k8s_service, slo_id, synthetic_check_ids,
			alert_rule_ids, incident_count_30d, last_incident, uptime_percent_30d,
			avg_response_time_ms, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		svc.ID, svc.OrgID, svc.Name, svc.DisplayName, svc.Description,
		svc.Tier, svc.Lifecycle, svc.Health, svc.TeamID, svc.TeamName,
		svc.OwnerEmail, svc.OnCallID, svc.EscalationID, svc.SlackChannel,
		svc.RepoURL, svc.DocsURL, svc.DashboardID, svc.RunbookURL,
		svc.StatusPageID, svc.Language, svc.Framework, string(tags),
		string(metadata), svc.K8sNamespace, svc.K8sDeployment, svc.K8sService,
		svc.SLOID, string(syntheticIDs), string(alertRuleIDs),
		svc.IncidentCount, nil, svc.UptimePercent, svc.AvgResponseTime,
		now.Unix(), now.Unix())

	return err
}

// GetService gets a service by ID
func (s *Store) GetService(id string) (*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.scanService(s.db.QueryRow(`
		SELECT id, org_id, name, display_name, description, tier, lifecycle, health,
			team_id, team_name, owner_email, oncall_schedule_id, escalation_policy_id,
			slack_channel, repo_url, docs_url, dashboard_id, runbook_url,
			statuspage_component_id, language, framework, tags, metadata,
			k8s_namespace, k8s_deployment, k8s_service, slo_id, synthetic_check_ids,
			alert_rule_ids, incident_count_30d, last_incident, uptime_percent_30d,
			avg_response_time_ms, created_at, updated_at
		FROM services WHERE id = ?`, id))
}

// GetServiceByName gets a service by name within an org
func (s *Store) GetServiceByName(orgID, name string) (*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.scanService(s.db.QueryRow(`
		SELECT id, org_id, name, display_name, description, tier, lifecycle, health,
			team_id, team_name, owner_email, oncall_schedule_id, escalation_policy_id,
			slack_channel, repo_url, docs_url, dashboard_id, runbook_url,
			statuspage_component_id, language, framework, tags, metadata,
			k8s_namespace, k8s_deployment, k8s_service, slo_id, synthetic_check_ids,
			alert_rule_ids, incident_count_30d, last_incident, uptime_percent_30d,
			avg_response_time_ms, created_at, updated_at
		FROM services WHERE org_id = ? AND name = ?`, orgID, name))
}

func (s *Store) scanService(row *sql.Row) (*Service, error) {
	var svc Service
	var tags, metadata, syntheticIDs, alertRuleIDs string
	var lastIncident sql.NullInt64
	var createdAt, updatedAt int64

	err := row.Scan(
		&svc.ID, &svc.OrgID, &svc.Name, &svc.DisplayName, &svc.Description,
		&svc.Tier, &svc.Lifecycle, &svc.Health, &svc.TeamID, &svc.TeamName,
		&svc.OwnerEmail, &svc.OnCallID, &svc.EscalationID, &svc.SlackChannel,
		&svc.RepoURL, &svc.DocsURL, &svc.DashboardID, &svc.RunbookURL,
		&svc.StatusPageID, &svc.Language, &svc.Framework, &tags, &metadata,
		&svc.K8sNamespace, &svc.K8sDeployment, &svc.K8sService, &svc.SLOID,
		&syntheticIDs, &alertRuleIDs, &svc.IncidentCount, &lastIncident,
		&svc.UptimePercent, &svc.AvgResponseTime, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(tags), &svc.Tags)
	json.Unmarshal([]byte(metadata), &svc.Metadata)
	json.Unmarshal([]byte(syntheticIDs), &svc.SyntheticIDs)
	json.Unmarshal([]byte(alertRuleIDs), &svc.AlertRuleIDs)

	if lastIncident.Valid {
		t := time.Unix(lastIncident.Int64, 0)
		svc.LastIncident = &t
	}

	svc.CreatedAt = time.Unix(createdAt, 0)
	svc.UpdatedAt = time.Unix(updatedAt, 0)

	return &svc, nil
}

// ListServices lists all services for an organization
func (s *Store) ListServices(orgID string, filters ServiceFilters) ([]*Service, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, org_id, name, display_name, description, tier, lifecycle, health,
			team_id, team_name, owner_email, oncall_schedule_id, escalation_policy_id,
			slack_channel, repo_url, docs_url, dashboard_id, runbook_url,
			statuspage_component_id, language, framework, tags, metadata,
			k8s_namespace, k8s_deployment, k8s_service, slo_id, synthetic_check_ids,
			alert_rule_ids, incident_count_30d, last_incident, uptime_percent_30d,
			avg_response_time_ms, created_at, updated_at
		FROM services WHERE org_id = ?`

	args := []interface{}{orgID}

	if filters.TeamID != "" {
		query += " AND team_id = ?"
		args = append(args, filters.TeamID)
	}
	if filters.Tier != "" {
		query += " AND tier = ?"
		args = append(args, filters.Tier)
	}
	if filters.Lifecycle != "" {
		query += " AND lifecycle = ?"
		args = append(args, filters.Lifecycle)
	}
	if filters.Health != "" {
		query += " AND health = ?"
		args = append(args, filters.Health)
	}

	query += " ORDER BY tier, name"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []*Service
	for rows.Next() {
		var svc Service
		var tags, metadata, syntheticIDs, alertRuleIDs string
		var lastIncident sql.NullInt64
		var createdAt, updatedAt int64

		if err := rows.Scan(
			&svc.ID, &svc.OrgID, &svc.Name, &svc.DisplayName, &svc.Description,
			&svc.Tier, &svc.Lifecycle, &svc.Health, &svc.TeamID, &svc.TeamName,
			&svc.OwnerEmail, &svc.OnCallID, &svc.EscalationID, &svc.SlackChannel,
			&svc.RepoURL, &svc.DocsURL, &svc.DashboardID, &svc.RunbookURL,
			&svc.StatusPageID, &svc.Language, &svc.Framework, &tags, &metadata,
			&svc.K8sNamespace, &svc.K8sDeployment, &svc.K8sService, &svc.SLOID,
			&syntheticIDs, &alertRuleIDs, &svc.IncidentCount, &lastIncident,
			&svc.UptimePercent, &svc.AvgResponseTime, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(tags), &svc.Tags)
		json.Unmarshal([]byte(metadata), &svc.Metadata)
		json.Unmarshal([]byte(syntheticIDs), &svc.SyntheticIDs)
		json.Unmarshal([]byte(alertRuleIDs), &svc.AlertRuleIDs)

		if lastIncident.Valid {
			t := time.Unix(lastIncident.Int64, 0)
			svc.LastIncident = &t
		}

		svc.CreatedAt = time.Unix(createdAt, 0)
		svc.UpdatedAt = time.Unix(updatedAt, 0)

		services = append(services, &svc)
	}

	return services, nil
}

// ServiceFilters for listing services
type ServiceFilters struct {
	TeamID    string
	Tier      ServiceTier
	Lifecycle ServiceLifecycle
	Health    ServiceHealth
}

// UpdateService updates a service
func (s *Store) UpdateService(svc *Service) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	svc.UpdatedAt = time.Now()

	tags, _ := json.Marshal(svc.Tags)
	metadata, _ := json.Marshal(svc.Metadata)
	syntheticIDs, _ := json.Marshal(svc.SyntheticIDs)
	alertRuleIDs, _ := json.Marshal(svc.AlertRuleIDs)

	var lastIncident *int64
	if svc.LastIncident != nil {
		t := svc.LastIncident.Unix()
		lastIncident = &t
	}

	_, err := s.db.Exec(`
		UPDATE services SET
			display_name=?, description=?, tier=?, lifecycle=?, health=?,
			team_id=?, team_name=?, owner_email=?, oncall_schedule_id=?,
			escalation_policy_id=?, slack_channel=?, repo_url=?, docs_url=?,
			dashboard_id=?, runbook_url=?, statuspage_component_id=?,
			language=?, framework=?, tags=?, metadata=?, k8s_namespace=?,
			k8s_deployment=?, k8s_service=?, slo_id=?, synthetic_check_ids=?,
			alert_rule_ids=?, incident_count_30d=?, last_incident=?,
			uptime_percent_30d=?, avg_response_time_ms=?, updated_at=?
		WHERE id=?`,
		svc.DisplayName, svc.Description, svc.Tier, svc.Lifecycle, svc.Health,
		svc.TeamID, svc.TeamName, svc.OwnerEmail, svc.OnCallID,
		svc.EscalationID, svc.SlackChannel, svc.RepoURL, svc.DocsURL,
		svc.DashboardID, svc.RunbookURL, svc.StatusPageID,
		svc.Language, svc.Framework, string(tags), string(metadata),
		svc.K8sNamespace, svc.K8sDeployment, svc.K8sService, svc.SLOID,
		string(syntheticIDs), string(alertRuleIDs), svc.IncidentCount,
		lastIncident, svc.UptimePercent, svc.AvgResponseTime,
		svc.UpdatedAt.Unix(), svc.ID)

	return err
}

// DeleteService deletes a service
func (s *Store) DeleteService(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete dependencies first
	s.db.Exec("DELETE FROM dependencies WHERE source_service_id = ? OR target_service_id = ?", id, id)
	s.db.Exec("DELETE FROM runbooks WHERE service_id = ?", id)

	_, err := s.db.Exec("DELETE FROM services WHERE id = ?", id)
	return err
}

// UpdateServiceHealth updates just the health status
func (s *Store) UpdateServiceHealth(id string, health ServiceHealth) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE services SET health = ?, updated_at = ? WHERE id = ?",
		health, time.Now().Unix(), id)
	return err
}

// AddDependency adds a dependency between services
func (s *Store) AddDependency(dep *Dependency) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	dep.CreatedAt = now
	dep.LastSeenAt = now

	if dep.DependencyType == "" {
		dep.DependencyType = DepTypeSync
	}

	_, err := s.db.Exec(`
		INSERT INTO dependencies (id, source_service_id, target_service_id,
			dependency_type, description, is_auto_detected, confidence,
			last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_service_id, target_service_id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			confidence = MAX(confidence, excluded.confidence)`,
		dep.ID, dep.SourceService, dep.TargetService, dep.DependencyType,
		dep.Description, dep.IsAutoDetected, dep.Confidence,
		now.Unix(), now.Unix())

	return err
}

// GetDependencies gets all dependencies for a service
func (s *Store) GetDependencies(serviceID string) (upstream, downstream []*Dependency, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Upstream (services this service depends on)
	rows, err := s.db.Query(`
		SELECT id, source_service_id, target_service_id, dependency_type,
			description, is_auto_detected, confidence, last_seen_at, created_at
		FROM dependencies WHERE source_service_id = ?`, serviceID)
	if err != nil {
		return nil, nil, err
	}

	upstream, err = s.scanDependencies(rows)
	if err != nil {
		return nil, nil, err
	}

	// Downstream (services that depend on this service)
	rows, err = s.db.Query(`
		SELECT id, source_service_id, target_service_id, dependency_type,
			description, is_auto_detected, confidence, last_seen_at, created_at
		FROM dependencies WHERE target_service_id = ?`, serviceID)
	if err != nil {
		return nil, nil, err
	}

	downstream, err = s.scanDependencies(rows)
	return upstream, downstream, err
}

func (s *Store) scanDependencies(rows *sql.Rows) ([]*Dependency, error) {
	defer rows.Close()

	var deps []*Dependency
	for rows.Next() {
		var dep Dependency
		var lastSeenAt, createdAt int64

		if err := rows.Scan(&dep.ID, &dep.SourceService, &dep.TargetService,
			&dep.DependencyType, &dep.Description, &dep.IsAutoDetected,
			&dep.Confidence, &lastSeenAt, &createdAt); err != nil {
			continue
		}

		dep.LastSeenAt = time.Unix(lastSeenAt, 0)
		dep.CreatedAt = time.Unix(createdAt, 0)
		deps = append(deps, &dep)
	}

	return deps, nil
}

// GetServiceGraph gets all services and dependencies for visualization
func (s *Store) GetServiceGraph(orgID string) (*ServiceGraph, error) {
	services, err := s.ListServices(orgID, ServiceFilters{})
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT d.id, d.source_service_id, d.target_service_id, d.dependency_type,
			d.description, d.is_auto_detected, d.confidence, d.last_seen_at, d.created_at
		FROM dependencies d
		JOIN services s1 ON d.source_service_id = s1.id
		JOIN services s2 ON d.target_service_id = s2.id
		WHERE s1.org_id = ? OR s2.org_id = ?`, orgID, orgID)
	if err != nil {
		return nil, err
	}

	deps, err := s.scanDependencies(rows)
	if err != nil {
		return nil, err
	}

	return &ServiceGraph{
		Services:     services,
		Dependencies: deps,
	}, nil
}

// ServiceGraph represents the full service dependency graph
type ServiceGraph struct {
	Services     []*Service    `json:"services"`
	Dependencies []*Dependency `json:"dependencies"`
}

// CreateTeam creates a new team
func (s *Store) CreateTeam(team *Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	team.CreatedAt = now
	team.UpdatedAt = now

	members, _ := json.Marshal(team.Members)

	_, err := s.db.Exec(`
		INSERT INTO teams (id, org_id, name, description, slack_channel,
			email, oncall_schedule_id, member_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		team.ID, team.OrgID, team.Name, team.Description, team.SlackChannel,
		team.Email, team.OnCallID, string(members), now.Unix(), now.Unix())

	return err
}

// GetTeam gets a team by ID
func (s *Store) GetTeam(id string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var team Team
	var members string
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, name, description, slack_channel, email,
			oncall_schedule_id, member_ids, created_at, updated_at
		FROM teams WHERE id = ?`, id).Scan(
		&team.ID, &team.OrgID, &team.Name, &team.Description,
		&team.SlackChannel, &team.Email, &team.OnCallID, &members,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(members), &team.Members)
	team.CreatedAt = time.Unix(createdAt, 0)
	team.UpdatedAt = time.Unix(updatedAt, 0)

	return &team, nil
}

// ListTeams lists all teams for an organization
func (s *Store) ListTeams(orgID string) ([]*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, name, description, slack_channel, email,
			oncall_schedule_id, member_ids, created_at, updated_at
		FROM teams WHERE org_id = ? ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var team Team
		var members string
		var createdAt, updatedAt int64

		if err := rows.Scan(&team.ID, &team.OrgID, &team.Name, &team.Description,
			&team.SlackChannel, &team.Email, &team.OnCallID, &members,
			&createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(members), &team.Members)
		team.CreatedAt = time.Unix(createdAt, 0)
		team.UpdatedAt = time.Unix(updatedAt, 0)
		teams = append(teams, &team)
	}

	return teams, nil
}

// CreateRunbook creates a new runbook
func (s *Store) CreateRunbook(rb *Runbook) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	rb.CreatedAt = now
	rb.UpdatedAt = now

	alertTags, _ := json.Marshal(rb.AlertTags)

	_, err := s.db.Exec(`
		INSERT INTO runbooks (id, service_id, title, description, content,
			external_url, alert_tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rb.ID, rb.ServiceID, rb.Title, rb.Description, rb.Content,
		rb.ExternalURL, string(alertTags), now.Unix(), now.Unix())

	return err
}

// GetRunbooksForService gets all runbooks for a service
func (s *Store) GetRunbooksForService(serviceID string) ([]*Runbook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, service_id, title, description, content, external_url,
			alert_tags, created_at, updated_at
		FROM runbooks WHERE service_id = ? ORDER BY title`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runbooks []*Runbook
	for rows.Next() {
		var rb Runbook
		var alertTags string
		var createdAt, updatedAt int64

		if err := rows.Scan(&rb.ID, &rb.ServiceID, &rb.Title, &rb.Description,
			&rb.Content, &rb.ExternalURL, &alertTags, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(alertTags), &rb.AlertTags)
		rb.CreatedAt = time.Unix(createdAt, 0)
		rb.UpdatedAt = time.Unix(updatedAt, 0)
		runbooks = append(runbooks, &rb)
	}

	return runbooks, nil
}

// FindRunbookByAlertTags finds runbooks matching alert tags
func (s *Store) FindRunbookByAlertTags(orgID string, tags []string) ([]*Runbook, error) {
	// For simplicity, we'll do this in Go rather than complex SQL
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT r.id, r.service_id, r.title, r.description, r.content,
			r.external_url, r.alert_tags, r.created_at, r.updated_at
		FROM runbooks r
		JOIN services s ON r.service_id = s.id
		WHERE s.org_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}

	var matches []*Runbook
	for rows.Next() {
		var rb Runbook
		var alertTags string
		var createdAt, updatedAt int64

		if err := rows.Scan(&rb.ID, &rb.ServiceID, &rb.Title, &rb.Description,
			&rb.Content, &rb.ExternalURL, &alertTags, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(alertTags), &rb.AlertTags)
		rb.CreatedAt = time.Unix(createdAt, 0)
		rb.UpdatedAt = time.Unix(updatedAt, 0)

		// Check if any tags match
		for _, t := range rb.AlertTags {
			if tagSet[t] {
				matches = append(matches, &rb)
				break
			}
		}
	}

	return matches, nil
}

// GetServiceStats gets aggregated stats for services
func (s *Store) GetServiceStats(orgID string) (*ServiceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats ServiceStats

	// Count by tier
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND tier = 'critical'", orgID).Scan(&stats.Critical)
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND tier = 'high'", orgID).Scan(&stats.High)
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND tier = 'medium'", orgID).Scan(&stats.Medium)
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND tier = 'low'", orgID).Scan(&stats.Low)

	// Count by health
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND health = 'healthy'", orgID).Scan(&stats.Healthy)
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND health = 'degraded'", orgID).Scan(&stats.Degraded)
	s.db.QueryRow("SELECT COUNT(*) FROM services WHERE org_id = ? AND health = 'unhealthy'", orgID).Scan(&stats.Unhealthy)

	stats.Total = stats.Critical + stats.High + stats.Medium + stats.Low

	return &stats, nil
}

// ServiceStats represents aggregated service statistics
type ServiceStats struct {
	Total     int `json:"total"`
	Critical  int `json:"critical"`
	High      int `json:"high"`
	Medium    int `json:"medium"`
	Low       int `json:"low"`
	Healthy   int `json:"healthy"`
	Degraded  int `json:"degraded"`
	Unhealthy int `json:"unhealthy"`
}
