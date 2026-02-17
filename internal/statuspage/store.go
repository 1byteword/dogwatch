// Package statuspage provides public status page functionality
package statuspage

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// ServiceStatus represents the current status of a service
type ServiceStatus string

const (
	StatusOperational ServiceStatus = "operational"
	StatusDegraded    ServiceStatus = "degraded"
	StatusPartialOutage ServiceStatus = "partial_outage"
	StatusMajorOutage ServiceStatus = "major_outage"
	StatusMaintenance ServiceStatus = "maintenance"
	StatusUnknown     ServiceStatus = "unknown"
)

// Component represents a service/component on the status page
type Component struct {
	ID          string            `json:"id"`
	OrgID       string            `json:"org_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Group       string            `json:"group,omitempty"`       // Group components together
	Order       int               `json:"order"`                 // Display order
	Status      ServiceStatus     `json:"status"`
	Enabled     bool              `json:"enabled"`
	Public      bool              `json:"public"`                // Show on public status page

	// Data sources for status
	SyntheticCheckIDs []string    `json:"synthetic_check_ids,omitempty"`
	SLOID             string      `json:"slo_id,omitempty"`

	// Computed metrics
	UptimeDay     float64         `json:"uptime_24h"`           // Last 24 hours
	UptimeWeek    float64         `json:"uptime_7d"`            // Last 7 days
	UptimeMonth   float64         `json:"uptime_30d"`           // Last 30 days
	UptimeQuarter float64         `json:"uptime_90d"`           // Last 90 days
	ResponseTime  float64         `json:"response_time_ms"`     // Average response time
	LastChecked   time.Time       `json:"last_checked"`

	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// StatusPage represents a status page configuration
type StatusPage struct {
	ID            string            `json:"id"`
	OrgID         string            `json:"org_id"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`              // URL-friendly name
	Description   string            `json:"description,omitempty"`
	LogoURL       string            `json:"logo_url,omitempty"`
	FaviconURL    string            `json:"favicon_url,omitempty"`
	CustomDomain  string            `json:"custom_domain,omitempty"`
	Theme         StatusPageTheme   `json:"theme"`
	Public        bool              `json:"public"`
	ShowUptime    bool              `json:"show_uptime"`
	ShowIncidents bool              `json:"show_incidents"`
	ComponentIDs  []string          `json:"component_ids"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// StatusPageTheme holds theming options
type StatusPageTheme struct {
	PrimaryColor   string `json:"primary_color,omitempty"`
	BackgroundColor string `json:"background_color,omitempty"`
	TextColor      string `json:"text_color,omitempty"`
	CustomCSS      string `json:"custom_css,omitempty"`
}

// StatusHistory records historical status for a component
type StatusHistory struct {
	ID          string        `json:"id"`
	ComponentID string        `json:"component_id"`
	Status      ServiceStatus `json:"status"`
	ResponseTime float64      `json:"response_time_ms"`
	Timestamp   time.Time     `json:"timestamp"`
}

// StatusIncident represents an incident shown on the status page
type StatusIncident struct {
	ID           string           `json:"id"`
	OrgID        string           `json:"org_id"`
	Title        string           `json:"title"`
	Status       IncidentStatus   `json:"status"`
	Impact       IncidentImpact   `json:"impact"`
	Message      string           `json:"message"`
	ComponentIDs []string         `json:"component_ids"`
	Updates      []IncidentUpdate `json:"updates"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	ResolvedAt   *time.Time       `json:"resolved_at,omitempty"`
}

type IncidentStatus string

const (
	IncidentInvestigating IncidentStatus = "investigating"
	IncidentIdentified    IncidentStatus = "identified"
	IncidentMonitoring    IncidentStatus = "monitoring"
	IncidentResolved      IncidentStatus = "resolved"
)

type IncidentImpact string

const (
	ImpactNone     IncidentImpact = "none"
	ImpactMinor    IncidentImpact = "minor"
	ImpactMajor    IncidentImpact = "major"
	ImpactCritical IncidentImpact = "critical"
)

type IncidentUpdate struct {
	ID        string         `json:"id"`
	Status    IncidentStatus `json:"status"`
	Message   string         `json:"message"`
	CreatedAt time.Time      `json:"created_at"`
}

// MaintenanceWindow represents scheduled maintenance
type MaintenanceWindow struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ComponentIDs []string  `json:"component_ids"`
	ScheduledStart time.Time `json:"scheduled_start"`
	ScheduledEnd   time.Time `json:"scheduled_end"`
	CreatedAt    time.Time `json:"created_at"`
}

// Store provides status page data persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new status page store
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
	CREATE TABLE IF NOT EXISTS components (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		group_name TEXT,
		display_order INTEGER DEFAULT 0,
		status TEXT DEFAULT 'operational',
		enabled INTEGER DEFAULT 1,
		public INTEGER DEFAULT 1,
		synthetic_check_ids TEXT,
		slo_id TEXT,
		uptime_day REAL DEFAULT 100,
		uptime_week REAL DEFAULT 100,
		uptime_month REAL DEFAULT 100,
		uptime_quarter REAL DEFAULT 100,
		response_time REAL DEFAULT 0,
		last_checked INTEGER,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS status_pages (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		name TEXT NOT NULL,
		slug TEXT UNIQUE NOT NULL,
		description TEXT,
		logo_url TEXT,
		favicon_url TEXT,
		custom_domain TEXT,
		theme TEXT,
		public INTEGER DEFAULT 0,
		show_uptime INTEGER DEFAULT 1,
		show_incidents INTEGER DEFAULT 1,
		component_ids TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS status_history (
		id TEXT PRIMARY KEY,
		component_id TEXT NOT NULL,
		status TEXT NOT NULL,
		response_time REAL,
		timestamp INTEGER NOT NULL,
		FOREIGN KEY (component_id) REFERENCES components(id)
	);

	CREATE TABLE IF NOT EXISTS status_incidents (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		impact TEXT NOT NULL,
		message TEXT,
		component_ids TEXT,
		updates TEXT,
		created_at INTEGER,
		updated_at INTEGER,
		resolved_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS maintenance_windows (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		component_ids TEXT,
		scheduled_start INTEGER,
		scheduled_end INTEGER,
		created_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_components_org ON components(org_id);
	CREATE INDEX IF NOT EXISTS idx_status_pages_org ON status_pages(org_id);
	CREATE INDEX IF NOT EXISTS idx_status_pages_slug ON status_pages(slug);
	CREATE INDEX IF NOT EXISTS idx_status_history_component ON status_history(component_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_status_incidents_org ON status_incidents(org_id);
	CREATE INDEX IF NOT EXISTS idx_maintenance_org ON maintenance_windows(org_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateComponent creates a new component
func (s *Store) CreateComponent(c *Component) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = StatusOperational
	}

	checkIDs, _ := json.Marshal(c.SyntheticCheckIDs)

	_, err := s.db.Exec(`
		INSERT INTO components (id, org_id, name, description, group_name, display_order,
			status, enabled, public, synthetic_check_ids, slo_id, uptime_day, uptime_week,
			uptime_month, uptime_quarter, response_time, last_checked, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.OrgID, c.Name, c.Description, c.Group, c.Order,
		c.Status, c.Enabled, c.Public, string(checkIDs), c.SLOID,
		c.UptimeDay, c.UptimeWeek, c.UptimeMonth, c.UptimeQuarter,
		c.ResponseTime, c.LastChecked.Unix(), now.Unix(), now.Unix())

	return err
}

// GetComponent gets a component by ID
func (s *Store) GetComponent(id string) (*Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var c Component
	var checkIDs string
	var lastChecked, createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, name, description, group_name, display_order, status,
			enabled, public, synthetic_check_ids, slo_id, uptime_day, uptime_week,
			uptime_month, uptime_quarter, response_time, last_checked, created_at, updated_at
		FROM components WHERE id = ?`, id).Scan(
		&c.ID, &c.OrgID, &c.Name, &c.Description, &c.Group, &c.Order, &c.Status,
		&c.Enabled, &c.Public, &checkIDs, &c.SLOID, &c.UptimeDay, &c.UptimeWeek,
		&c.UptimeMonth, &c.UptimeQuarter, &c.ResponseTime, &lastChecked, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(checkIDs), &c.SyntheticCheckIDs)
	c.LastChecked = time.Unix(lastChecked, 0)
	c.CreatedAt = time.Unix(createdAt, 0)
	c.UpdatedAt = time.Unix(updatedAt, 0)

	return &c, nil
}

// ListComponents lists all components for an organization
func (s *Store) ListComponents(orgID string) ([]*Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, name, description, group_name, display_order, status,
			enabled, public, synthetic_check_ids, slo_id, uptime_day, uptime_week,
			uptime_month, uptime_quarter, response_time, last_checked, created_at, updated_at
		FROM components WHERE org_id = ? ORDER BY display_order, name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var components []*Component
	for rows.Next() {
		var c Component
		var checkIDs string
		var lastChecked, createdAt, updatedAt int64

		if err := rows.Scan(&c.ID, &c.OrgID, &c.Name, &c.Description, &c.Group, &c.Order,
			&c.Status, &c.Enabled, &c.Public, &checkIDs, &c.SLOID, &c.UptimeDay,
			&c.UptimeWeek, &c.UptimeMonth, &c.UptimeQuarter, &c.ResponseTime,
			&lastChecked, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(checkIDs), &c.SyntheticCheckIDs)
		c.LastChecked = time.Unix(lastChecked, 0)
		c.CreatedAt = time.Unix(createdAt, 0)
		c.UpdatedAt = time.Unix(updatedAt, 0)
		components = append(components, &c)
	}

	return components, nil
}

// UpdateComponent updates a component
func (s *Store) UpdateComponent(c *Component) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c.UpdatedAt = time.Now()
	checkIDs, _ := json.Marshal(c.SyntheticCheckIDs)

	_, err := s.db.Exec(`
		UPDATE components SET name=?, description=?, group_name=?, display_order=?,
			status=?, enabled=?, public=?, synthetic_check_ids=?, slo_id=?,
			uptime_day=?, uptime_week=?, uptime_month=?, uptime_quarter=?,
			response_time=?, last_checked=?, updated_at=?
		WHERE id=?`,
		c.Name, c.Description, c.Group, c.Order, c.Status, c.Enabled, c.Public,
		string(checkIDs), c.SLOID, c.UptimeDay, c.UptimeWeek, c.UptimeMonth,
		c.UptimeQuarter, c.ResponseTime, c.LastChecked.Unix(), c.UpdatedAt.Unix(), c.ID)

	return err
}

// DeleteComponent deletes a component
func (s *Store) DeleteComponent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM components WHERE id = ?", id)
	return err
}

// RecordStatus records a status check for a component
func (s *Store) RecordStatus(componentID string, status ServiceStatus, responseTime float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("sh_%d", time.Now().UnixNano())
	now := time.Now()

	_, err := s.db.Exec(`
		INSERT INTO status_history (id, component_id, status, response_time, timestamp)
		VALUES (?, ?, ?, ?, ?)`,
		id, componentID, status, responseTime, now.Unix())

	if err != nil {
		return err
	}

	// Update component's current status
	_, err = s.db.Exec(`
		UPDATE components SET status=?, response_time=?, last_checked=?, updated_at=?
		WHERE id=?`, status, responseTime, now.Unix(), now.Unix(), componentID)

	return err
}

// GetStatusHistory gets status history for a component
func (s *Store) GetStatusHistory(componentID string, since time.Duration, limit int) ([]StatusHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	startTime := time.Now().Add(-since).Unix()

	rows, err := s.db.Query(`
		SELECT id, component_id, status, response_time, timestamp
		FROM status_history
		WHERE component_id = ? AND timestamp >= ?
		ORDER BY timestamp DESC LIMIT ?`,
		componentID, startTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []StatusHistory
	for rows.Next() {
		var h StatusHistory
		var ts int64
		if err := rows.Scan(&h.ID, &h.ComponentID, &h.Status, &h.ResponseTime, &ts); err != nil {
			continue
		}
		h.Timestamp = time.Unix(ts, 0)
		history = append(history, h)
	}

	return history, nil
}

// CalculateUptime calculates uptime percentage for a component
func (s *Store) CalculateUptime(componentID string, duration time.Duration) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startTime := time.Now().Add(-duration).Unix()

	var total, operational int
	rows, err := s.db.Query(`
		SELECT status FROM status_history
		WHERE component_id = ? AND timestamp >= ?`,
		componentID, startTime)
	if err != nil {
		return 100.0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status ServiceStatus
		if err := rows.Scan(&status); err != nil {
			continue
		}
		total++
		if status == StatusOperational || status == StatusMaintenance {
			operational++
		}
	}

	if total == 0 {
		return 100.0, nil
	}

	return float64(operational) / float64(total) * 100, nil
}

// CreateStatusPage creates a new status page
func (s *Store) CreateStatusPage(page *StatusPage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	page.CreatedAt = now
	page.UpdatedAt = now

	theme, _ := json.Marshal(page.Theme)
	componentIDs, _ := json.Marshal(page.ComponentIDs)

	_, err := s.db.Exec(`
		INSERT INTO status_pages (id, org_id, name, slug, description, logo_url,
			favicon_url, custom_domain, theme, public, show_uptime, show_incidents,
			component_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		page.ID, page.OrgID, page.Name, page.Slug, page.Description, page.LogoURL,
		page.FaviconURL, page.CustomDomain, string(theme), page.Public,
		page.ShowUptime, page.ShowIncidents, string(componentIDs),
		now.Unix(), now.Unix())

	return err
}

// GetStatusPage gets a status page by ID
func (s *Store) GetStatusPage(id string) (*StatusPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getStatusPage("id", id)
}

// GetStatusPageBySlug gets a status page by slug
func (s *Store) GetStatusPageBySlug(slug string) (*StatusPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getStatusPage("slug", slug)
}

func (s *Store) getStatusPage(field, value string) (*StatusPage, error) {
	var page StatusPage
	var theme, componentIDs string
	var createdAt, updatedAt int64

	query := fmt.Sprintf(`
		SELECT id, org_id, name, slug, description, logo_url, favicon_url,
			custom_domain, theme, public, show_uptime, show_incidents,
			component_ids, created_at, updated_at
		FROM status_pages WHERE %s = ?`, field)

	err := s.db.QueryRow(query, value).Scan(
		&page.ID, &page.OrgID, &page.Name, &page.Slug, &page.Description,
		&page.LogoURL, &page.FaviconURL, &page.CustomDomain, &theme,
		&page.Public, &page.ShowUptime, &page.ShowIncidents,
		&componentIDs, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(theme), &page.Theme)
	json.Unmarshal([]byte(componentIDs), &page.ComponentIDs)
	page.CreatedAt = time.Unix(createdAt, 0)
	page.UpdatedAt = time.Unix(updatedAt, 0)

	return &page, nil
}

// ListStatusPages lists all status pages for an organization
func (s *Store) ListStatusPages(orgID string) ([]*StatusPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, name, slug, description, logo_url, favicon_url,
			custom_domain, theme, public, show_uptime, show_incidents,
			component_ids, created_at, updated_at
		FROM status_pages WHERE org_id = ? ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []*StatusPage
	for rows.Next() {
		var page StatusPage
		var theme, componentIDs string
		var createdAt, updatedAt int64

		if err := rows.Scan(&page.ID, &page.OrgID, &page.Name, &page.Slug,
			&page.Description, &page.LogoURL, &page.FaviconURL, &page.CustomDomain,
			&theme, &page.Public, &page.ShowUptime, &page.ShowIncidents,
			&componentIDs, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(theme), &page.Theme)
		json.Unmarshal([]byte(componentIDs), &page.ComponentIDs)
		page.CreatedAt = time.Unix(createdAt, 0)
		page.UpdatedAt = time.Unix(updatedAt, 0)
		pages = append(pages, &page)
	}

	return pages, nil
}

// UpdateStatusPage updates a status page
func (s *Store) UpdateStatusPage(page *StatusPage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	page.UpdatedAt = time.Now()
	theme, _ := json.Marshal(page.Theme)
	componentIDs, _ := json.Marshal(page.ComponentIDs)

	_, err := s.db.Exec(`
		UPDATE status_pages SET name=?, slug=?, description=?, logo_url=?,
			favicon_url=?, custom_domain=?, theme=?, public=?, show_uptime=?,
			show_incidents=?, component_ids=?, updated_at=?
		WHERE id=?`,
		page.Name, page.Slug, page.Description, page.LogoURL, page.FaviconURL,
		page.CustomDomain, string(theme), page.Public, page.ShowUptime,
		page.ShowIncidents, string(componentIDs), page.UpdatedAt.Unix(), page.ID)

	return err
}

// DeleteStatusPage deletes a status page
func (s *Store) DeleteStatusPage(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM status_pages WHERE id = ?", id)
	return err
}

// CreateIncident creates a status incident
func (s *Store) CreateIncident(inc *StatusIncident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	inc.CreatedAt = now
	inc.UpdatedAt = now

	componentIDs, _ := json.Marshal(inc.ComponentIDs)
	updates, _ := json.Marshal(inc.Updates)

	_, err := s.db.Exec(`
		INSERT INTO status_incidents (id, org_id, title, status, impact, message,
			component_ids, updates, created_at, updated_at, resolved_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inc.ID, inc.OrgID, inc.Title, inc.Status, inc.Impact, inc.Message,
		string(componentIDs), string(updates), now.Unix(), now.Unix(), nil)

	return err
}

// GetIncident gets an incident by ID
func (s *Store) GetIncident(id string) (*StatusIncident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var inc StatusIncident
	var componentIDs, updates string
	var createdAt, updatedAt int64
	var resolvedAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, org_id, title, status, impact, message, component_ids,
			updates, created_at, updated_at, resolved_at
		FROM status_incidents WHERE id = ?`, id).Scan(
		&inc.ID, &inc.OrgID, &inc.Title, &inc.Status, &inc.Impact, &inc.Message,
		&componentIDs, &updates, &createdAt, &updatedAt, &resolvedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(componentIDs), &inc.ComponentIDs)
	json.Unmarshal([]byte(updates), &inc.Updates)
	inc.CreatedAt = time.Unix(createdAt, 0)
	inc.UpdatedAt = time.Unix(updatedAt, 0)
	if resolvedAt.Valid {
		t := time.Unix(resolvedAt.Int64, 0)
		inc.ResolvedAt = &t
	}

	return &inc, nil
}

// ListIncidents lists incidents for an organization
func (s *Store) ListIncidents(orgID string, limit int, includeResolved bool) ([]*StatusIncident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, org_id, title, status, impact, message, component_ids,
			updates, created_at, updated_at, resolved_at
		FROM status_incidents WHERE org_id = ?`

	if !includeResolved {
		query += " AND status != 'resolved'"
	}

	query += " ORDER BY created_at DESC LIMIT ?"

	rows, err := s.db.Query(query, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []*StatusIncident
	for rows.Next() {
		var inc StatusIncident
		var componentIDs, updates string
		var createdAt, updatedAt int64
		var resolvedAt sql.NullInt64

		if err := rows.Scan(&inc.ID, &inc.OrgID, &inc.Title, &inc.Status, &inc.Impact,
			&inc.Message, &componentIDs, &updates, &createdAt, &updatedAt, &resolvedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(componentIDs), &inc.ComponentIDs)
		json.Unmarshal([]byte(updates), &inc.Updates)
		inc.CreatedAt = time.Unix(createdAt, 0)
		inc.UpdatedAt = time.Unix(updatedAt, 0)
		if resolvedAt.Valid {
			t := time.Unix(resolvedAt.Int64, 0)
			inc.ResolvedAt = &t
		}
		incidents = append(incidents, &inc)
	}

	return incidents, nil
}

// UpdateIncident updates an incident
func (s *Store) UpdateIncident(inc *StatusIncident) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	inc.UpdatedAt = time.Now()
	componentIDs, _ := json.Marshal(inc.ComponentIDs)
	updates, _ := json.Marshal(inc.Updates)

	var resolvedAt *int64
	if inc.ResolvedAt != nil {
		t := inc.ResolvedAt.Unix()
		resolvedAt = &t
	}

	_, err := s.db.Exec(`
		UPDATE status_incidents SET title=?, status=?, impact=?, message=?,
			component_ids=?, updates=?, updated_at=?, resolved_at=?
		WHERE id=?`,
		inc.Title, inc.Status, inc.Impact, inc.Message,
		string(componentIDs), string(updates), inc.UpdatedAt.Unix(),
		resolvedAt, inc.ID)

	return err
}

// CleanupOldHistory removes old status history entries
func (s *Store) CleanupOldHistory(retention time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-retention).Unix()
	_, err := s.db.Exec("DELETE FROM status_history WHERE timestamp < ?", cutoff)
	return err
}

// GetOverallStatus calculates overall status from all components
func (s *Store) GetOverallStatus(orgID string) ServiceStatus {
	components, err := s.ListComponents(orgID)
	if err != nil || len(components) == 0 {
		return StatusUnknown
	}

	hasOutage := false
	hasDegraded := false

	for _, c := range components {
		if !c.Enabled {
			continue
		}
		switch c.Status {
		case StatusMajorOutage:
			return StatusMajorOutage
		case StatusPartialOutage:
			hasOutage = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasOutage {
		return StatusPartialOutage
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusOperational
}
