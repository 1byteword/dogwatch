package deploys

import (
	"database/sql"
	"dogwatch/internal/storage"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Deployment represents a deployment event
type Deployment struct {
	ID          string            `json:"id"`
	Service     string            `json:"service"`
	Version     string            `json:"version"`
	Environment string            `json:"environment"` // prod, staging, dev
	Timestamp   time.Time         `json:"timestamp"`
	User        string            `json:"user"`        // Who deployed
	Description string            `json:"description"` // Optional notes
	CommitSHA   string            `json:"commit_sha"`  // Git commit
	CommitURL   string            `json:"commit_url"`  // Link to commit
	Status      string            `json:"status"`      // success, failed, rolled_back
	Duration    int               `json:"duration_ms"` // Deploy duration in ms
	Tags        map[string]string `json:"tags"`        // Custom metadata
}

// DeploymentImpact shows metrics before/after a deployment
type DeploymentImpact struct {
	DeploymentID string  `json:"deployment_id"`

	// Error rates
	ErrorsBefore     int     `json:"errors_before"`
	ErrorsAfter      int     `json:"errors_after"`
	ErrorRateChange  float64 `json:"error_rate_change"` // Percentage change

	// Latency
	LatencyP50Before float64 `json:"latency_p50_before"`
	LatencyP50After  float64 `json:"latency_p50_after"`
	LatencyChange    float64 `json:"latency_change"` // Percentage change

	// Request volume
	RequestsBefore   int     `json:"requests_before"`
	RequestsAfter    int     `json:"requests_after"`

	// SLO impact
	SLOsBefore       int     `json:"slos_met_before"`
	SLOsAfter        int     `json:"slos_met_after"`

	// Overall assessment
	Impact           string  `json:"impact"` // positive, negative, neutral
}

// Store handles deployment persistence
type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS deployments (
		id TEXT PRIMARY KEY,
		service TEXT NOT NULL,
		version TEXT NOT NULL,
		environment TEXT DEFAULT 'prod',
		timestamp DATETIME NOT NULL,
		user TEXT,
		description TEXT,
		commit_sha TEXT,
		commit_url TEXT,
		status TEXT DEFAULT 'success',
		duration_ms INTEGER DEFAULT 0,
		tags TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_deploy_service ON deployments(service);
	CREATE INDEX IF NOT EXISTS idx_deploy_timestamp ON deployments(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_deploy_env ON deployments(environment);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Record stores a new deployment
func (s *Store) Record(d *Deployment) error {
	if d.ID == "" {
		d.ID = uuid.New().String()
	}
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}
	if d.Status == "" {
		d.Status = "success"
	}
	if d.Environment == "" {
		d.Environment = "prod"
	}

	tagsJSON := "{}"
	if len(d.Tags) > 0 {
		// Simple JSON encoding for tags
		tagsJSON = "{"
		first := true
		for k, v := range d.Tags {
			if !first {
				tagsJSON += ","
			}
			tagsJSON += fmt.Sprintf(`"%s":"%s"`, k, v)
			first = false
		}
		tagsJSON += "}"
	}

	_, err := s.db.Exec(`
		INSERT INTO deployments (id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Service, d.Version, d.Environment, d.Timestamp,
		d.User, d.Description, d.CommitSHA, d.CommitURL, d.Status, d.Duration, tagsJSON,
	)
	return err
}

// Get retrieves a deployment by ID
func (s *Store) Get(id string) (*Deployment, error) {
	row := s.db.QueryRow(`
		SELECT id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags
		FROM deployments WHERE id = ?`, id)
	return s.scanDeployment(row)
}

// List returns recent deployments
func (s *Store) List(limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags
		FROM deployments
		ORDER BY timestamp DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}

// ListByService returns deployments for a specific service
func (s *Store) ListByService(service string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`
		SELECT id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags
		FROM deployments
		WHERE service = ?
		ORDER BY timestamp DESC
		LIMIT ?`, service, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}

// ListByTimeRange returns deployments within a time range
func (s *Store) ListByTimeRange(start, end time.Time) ([]Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags
		FROM deployments
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanDeployments(rows)
}

// GetServices returns list of unique services with deployments
func (s *Store) GetServices() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT service FROM deployments ORDER BY service`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, nil
}

// GetLatest returns the most recent deployment for a service
func (s *Store) GetLatest(service string) (*Deployment, error) {
	row := s.db.QueryRow(`
		SELECT id, service, version, environment, timestamp, user, description, commit_sha, commit_url, status, duration_ms, tags
		FROM deployments
		WHERE service = ?
		ORDER BY timestamp DESC
		LIMIT 1`, service)
	return s.scanDeployment(row)
}

// UpdateStatus updates a deployment's status
func (s *Store) UpdateStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE deployments SET status = ? WHERE id = ?`, status, id)
	return err
}

// Delete removes a deployment
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM deployments WHERE id = ?`, id)
	return err
}

// Stats returns deployment statistics
type DeployStats struct {
	TotalDeployments   int            `json:"total_deployments"`
	DeploysToday       int            `json:"deploys_today"`
	DeploysThisWeek    int            `json:"deploys_this_week"`
	SuccessRate        float64        `json:"success_rate"`
	AvgDurationMs      float64        `json:"avg_duration_ms"`
	DeploysByService   map[string]int `json:"deploys_by_service"`
	DeploysByEnv       map[string]int `json:"deploys_by_env"`
	RecentFailures     int            `json:"recent_failures"`
}

func (s *Store) GetStats() (*DeployStats, error) {
	stats := &DeployStats{
		DeploysByService: make(map[string]int),
		DeploysByEnv:     make(map[string]int),
	}

	now := time.Now()
	today := now.Truncate(24 * time.Hour)
	weekAgo := now.Add(-7 * 24 * time.Hour)

	// Total count
	s.db.QueryRow(`SELECT COUNT(*) FROM deployments`).Scan(&stats.TotalDeployments)

	// Today's deploys
	s.db.QueryRow(`SELECT COUNT(*) FROM deployments WHERE timestamp >= ?`, today).Scan(&stats.DeploysToday)

	// This week's deploys
	s.db.QueryRow(`SELECT COUNT(*) FROM deployments WHERE timestamp >= ?`, weekAgo).Scan(&stats.DeploysThisWeek)

	// Success rate
	var successCount int
	s.db.QueryRow(`SELECT COUNT(*) FROM deployments WHERE status = 'success'`).Scan(&successCount)
	if stats.TotalDeployments > 0 {
		stats.SuccessRate = float64(successCount) / float64(stats.TotalDeployments) * 100
	}

	// Average duration
	s.db.QueryRow(`SELECT AVG(duration_ms) FROM deployments WHERE duration_ms > 0`).Scan(&stats.AvgDurationMs)

	// By service
	rows, _ := s.db.Query(`SELECT service, COUNT(*) FROM deployments GROUP BY service`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var svc string
			var count int
			rows.Scan(&svc, &count)
			stats.DeploysByService[svc] = count
		}
	}

	// By environment
	rows, _ = s.db.Query(`SELECT environment, COUNT(*) FROM deployments GROUP BY environment`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var env string
			var count int
			rows.Scan(&env, &count)
			stats.DeploysByEnv[env] = count
		}
	}

	// Recent failures (last 24h)
	s.db.QueryRow(`SELECT COUNT(*) FROM deployments WHERE status != 'success' AND timestamp >= ?`, today).Scan(&stats.RecentFailures)

	return stats, nil
}

// Helper functions

func (s *Store) scanDeployment(row *sql.Row) (*Deployment, error) {
	var d Deployment
	var user, desc, sha, url, tags sql.NullString
	var duration sql.NullInt64

	err := row.Scan(&d.ID, &d.Service, &d.Version, &d.Environment, &d.Timestamp,
		&user, &desc, &sha, &url, &d.Status, &duration, &tags)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	d.User = user.String
	d.Description = desc.String
	d.CommitSHA = sha.String
	d.CommitURL = url.String
	d.Duration = int(duration.Int64)

	// Parse tags (simple approach)
	d.Tags = make(map[string]string)

	return &d, nil
}

func (s *Store) scanDeployments(rows *sql.Rows) ([]Deployment, error) {
	var deployments []Deployment
	for rows.Next() {
		var d Deployment
		var user, desc, sha, url, tags sql.NullString
		var duration sql.NullInt64

		err := rows.Scan(&d.ID, &d.Service, &d.Version, &d.Environment, &d.Timestamp,
			&user, &desc, &sha, &url, &d.Status, &duration, &tags)
		if err != nil {
			return nil, err
		}

		d.User = user.String
		d.Description = desc.String
		d.CommitSHA = sha.String
		d.CommitURL = url.String
		d.Duration = int(duration.Int64)
		d.Tags = make(map[string]string)

		deployments = append(deployments, d)
	}
	return deployments, nil
}
