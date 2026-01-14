package slo

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// SLOType represents the type of SLO measurement
type SLOType string

const (
	SLOAvailability SLOType = "availability" // Uptime percentage
	SLOLatency      SLOType = "latency"      // Response time percentile
	SLOErrorRate    SLOType = "error_rate"   // Error percentage
	SLOThroughput   SLOType = "throughput"   // Requests per second
)

// SLOStatus represents current SLO compliance status
type SLOStatus string

const (
	StatusMet      SLOStatus = "met"       // Within target
	StatusAtRisk   SLOStatus = "at_risk"   // Burning budget fast
	StatusBreached SLOStatus = "breached"  // Target missed
	StatusNoData   SLOStatus = "no_data"   // Insufficient data
)

// TimeWindow represents the SLO evaluation period
type TimeWindow string

const (
	Window7Days  TimeWindow = "7d"
	Window30Days TimeWindow = "30d"
	Window90Days TimeWindow = "90d"
)

// DataSource defines where SLO data comes from
type DataSource struct {
	Type       string `json:"type"`        // "synthetics", "metric", "logs"
	ID         string `json:"id"`          // Check ID, metric name, etc.
	Field      string `json:"field"`       // For metrics: "latency_p99", "error_rate"
	Percentile int    `json:"percentile"`  // For latency SLOs: 50, 95, 99
}

// SLO represents a Service Level Objective
type SLO struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	ServiceID   string     `json:"service_id,omitempty"` // Link to service catalog
	Type        SLOType    `json:"type"`
	Target      float64    `json:"target"`       // Target value (e.g., 99.9 for availability)
	Window      TimeWindow `json:"window"`       // Time window for evaluation
	Source      DataSource `json:"source"`       // Data source configuration
	Threshold   float64    `json:"threshold"`    // For latency: max acceptable ms
	Enabled     bool       `json:"enabled"`
	Created     time.Time  `json:"created"`
	Updated     time.Time  `json:"updated"`
}

// SLOState represents the current state of an SLO
type SLOState struct {
	SLOID           string    `json:"slo_id"`
	CurrentValue    float64   `json:"current_value"`     // Current measured value
	Target          float64   `json:"target"`            // Target value
	Status          SLOStatus `json:"status"`
	ErrorBudget     float64   `json:"error_budget"`      // Total allowed error (in appropriate units)
	BudgetRemaining float64   `json:"budget_remaining"`  // Remaining budget
	BudgetUsedPct   float64   `json:"budget_used_pct"`   // Percentage of budget used
	BurnRate        float64   `json:"burn_rate"`         // Current burn rate (1.0 = normal)
	TotalEvents     int64     `json:"total_events"`      // Total measured events
	GoodEvents      int64     `json:"good_events"`       // Events meeting criteria
	BadEvents       int64     `json:"bad_events"`        // Events failing criteria
	WindowStart     time.Time `json:"window_start"`
	WindowEnd       time.Time `json:"window_end"`
	LastUpdated     time.Time `json:"last_updated"`
}

// SLOSnapshot stores historical SLO state for trending
type SLOSnapshot struct {
	ID              string    `json:"id"`
	SLOID           string    `json:"slo_id"`
	Timestamp       time.Time `json:"timestamp"`
	CurrentValue    float64   `json:"current_value"`
	BudgetRemaining float64   `json:"budget_remaining"`
	Status          SLOStatus `json:"status"`
}

// Store handles SLO persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new SLO store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS slos (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		service_id TEXT,
		type TEXT NOT NULL,
		target REAL NOT NULL,
		window TEXT NOT NULL,
		source TEXT NOT NULL,
		threshold REAL,
		enabled INTEGER DEFAULT 1,
		created DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_slo_service ON slos(service_id);

	CREATE TABLE IF NOT EXISTS slo_snapshots (
		id TEXT PRIMARY KEY,
		slo_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		current_value REAL,
		budget_remaining REAL,
		status TEXT,
		FOREIGN KEY (slo_id) REFERENCES slos(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_snapshots_slo_time ON slo_snapshots(slo_id, timestamp DESC);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateSLO creates a new SLO
func (s *Store) CreateSLO(slo *SLO) error {
	if slo.ID == "" {
		slo.ID = uuid.New().String()
	}
	if slo.Window == "" {
		slo.Window = Window30Days
	}
	slo.Created = time.Now()
	slo.Updated = time.Now()

	sourceJSON, _ := json.Marshal(slo.Source)

	_, err := s.db.Exec(`
		INSERT INTO slos (id, name, description, service_id, type, target, window, source, threshold, enabled, created, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		slo.ID, slo.Name, slo.Description, slo.ServiceID, slo.Type, slo.Target, slo.Window,
		string(sourceJSON), slo.Threshold, slo.Enabled, slo.Created, slo.Updated,
	)
	return err
}

// GetSLO retrieves an SLO by ID
func (s *Store) GetSLO(id string) (*SLO, error) {
	row := s.db.QueryRow(`
		SELECT id, name, description, service_id, type, target, window, source, threshold, enabled, created, updated
		FROM slos WHERE id = ?`, id)

	return s.scanSLO(row)
}

// ListSLOs returns all SLOs
func (s *Store) ListSLOs() ([]SLO, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, service_id, type, target, window, source, threshold, enabled, created, updated
		FROM slos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slos []SLO
	for rows.Next() {
		slo, err := s.scanSLORows(rows)
		if err != nil {
			return nil, err
		}
		slos = append(slos, *slo)
	}
	return slos, nil
}

// ListSLOsByService returns all SLOs for a given service
func (s *Store) ListSLOsByService(serviceID string) ([]SLO, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, service_id, type, target, window, source, threshold, enabled, created, updated
		FROM slos WHERE service_id = ? ORDER BY name`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slos []SLO
	for rows.Next() {
		slo, err := s.scanSLORows(rows)
		if err != nil {
			return nil, err
		}
		slos = append(slos, *slo)
	}
	return slos, nil
}

// ListEnabledSLOs returns all enabled SLOs
func (s *Store) ListEnabledSLOs() ([]SLO, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, service_id, type, target, window, source, threshold, enabled, created, updated
		FROM slos WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slos []SLO
	for rows.Next() {
		slo, err := s.scanSLORows(rows)
		if err != nil {
			return nil, err
		}
		slos = append(slos, *slo)
	}
	return slos, nil
}

// UpdateSLO updates an existing SLO
func (s *Store) UpdateSLO(slo *SLO) error {
	slo.Updated = time.Now()
	sourceJSON, _ := json.Marshal(slo.Source)

	_, err := s.db.Exec(`
		UPDATE slos SET name=?, description=?, service_id=?, type=?, target=?, window=?, source=?, threshold=?, enabled=?, updated=?
		WHERE id=?`,
		slo.Name, slo.Description, slo.ServiceID, slo.Type, slo.Target, slo.Window,
		string(sourceJSON), slo.Threshold, slo.Enabled, slo.Updated, slo.ID,
	)
	return err
}

// DeleteSLO removes an SLO
func (s *Store) DeleteSLO(id string) error {
	_, err := s.db.Exec("DELETE FROM slos WHERE id = ?", id)
	return err
}

// RecordSnapshot stores an SLO state snapshot
func (s *Store) RecordSnapshot(snap *SLOSnapshot) error {
	if snap.ID == "" {
		snap.ID = uuid.New().String()
	}
	if snap.Timestamp.IsZero() {
		snap.Timestamp = time.Now()
	}

	_, err := s.db.Exec(`
		INSERT INTO slo_snapshots (id, slo_id, timestamp, current_value, budget_remaining, status)
		VALUES (?, ?, ?, ?, ?, ?)`,
		snap.ID, snap.SLOID, snap.Timestamp, snap.CurrentValue, snap.BudgetRemaining, snap.Status,
	)
	return err
}

// GetSnapshots retrieves historical snapshots for an SLO
func (s *Store) GetSnapshots(sloID string, since time.Duration, limit int) ([]SLOSnapshot, error) {
	if limit <= 0 {
		limit = 100
	}
	cutoff := time.Now().Add(-since)

	rows, err := s.db.Query(`
		SELECT id, slo_id, timestamp, current_value, budget_remaining, status
		FROM slo_snapshots
		WHERE slo_id = ? AND timestamp >= ?
		ORDER BY timestamp DESC
		LIMIT ?`, sloID, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []SLOSnapshot
	for rows.Next() {
		var snap SLOSnapshot
		err := rows.Scan(&snap.ID, &snap.SLOID, &snap.Timestamp, &snap.CurrentValue, &snap.BudgetRemaining, &snap.Status)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// CleanupSnapshots removes old snapshots
func (s *Store) CleanupSnapshots(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.Exec("DELETE FROM slo_snapshots WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Helper scan functions
func (s *Store) scanSLO(row *sql.Row) (*SLO, error) {
	var slo SLO
	var sourceJSON string
	var threshold sql.NullFloat64
	var serviceID sql.NullString

	err := row.Scan(&slo.ID, &slo.Name, &slo.Description, &serviceID, &slo.Type, &slo.Target,
		&slo.Window, &sourceJSON, &threshold, &slo.Enabled, &slo.Created, &slo.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(sourceJSON), &slo.Source)
	slo.Threshold = threshold.Float64
	slo.ServiceID = serviceID.String
	return &slo, nil
}

func (s *Store) scanSLORows(rows *sql.Rows) (*SLO, error) {
	var slo SLO
	var sourceJSON string
	var threshold sql.NullFloat64
	var serviceID sql.NullString

	err := rows.Scan(&slo.ID, &slo.Name, &slo.Description, &serviceID, &slo.Type, &slo.Target,
		&slo.Window, &sourceJSON, &threshold, &slo.Enabled, &slo.Created, &slo.Updated)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(sourceJSON), &slo.Source)
	slo.Threshold = threshold.Float64
	slo.ServiceID = serviceID.String
	return &slo, nil
}

// GetWindowDuration converts TimeWindow to time.Duration
func GetWindowDuration(w TimeWindow) time.Duration {
	switch w {
	case Window7Days:
		return 7 * 24 * time.Hour
	case Window30Days:
		return 30 * 24 * time.Hour
	case Window90Days:
		return 90 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}
