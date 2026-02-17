package costintel

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"sync"
	"time"

)

// Store persists cost estimates over time for trending
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// HistoricalEstimate represents a cost estimate at a point in time
type HistoricalEstimate struct {
	ID           int64              `json:"id"`
	Timestamp    time.Time          `json:"timestamp"`
	Vendor       string             `json:"vendor"`
	TotalMonthly float64            `json:"total_monthly"`
	TotalAnnual  float64            `json:"total_annual"`
	Breakdown    map[string]float64 `json:"breakdown"`
	Usage        UsageMetrics       `json:"usage"`
}

// CostTrend represents cost changes over time
type CostTrend struct {
	Vendor       string           `json:"vendor"`
	CurrentCost  float64          `json:"current_cost"`
	PreviousCost float64          `json:"previous_cost"`
	Change       float64          `json:"change"`
	ChangePercent float64         `json:"change_percent"`
	Trend        string           `json:"trend"` // "up", "down", "stable"
	DataPoints   []TrendDataPoint `json:"data_points"`
}

// TrendDataPoint represents a single point in time
type TrendDataPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	TotalMonthly float64   `json:"total_monthly"`
}

// CostSummary provides aggregated cost statistics
type CostSummary struct {
	Period          string             `json:"period"`
	StartTime       time.Time          `json:"start_time"`
	EndTime         time.Time          `json:"end_time"`
	AverageCost     map[string]float64 `json:"average_cost"`
	MinCost         map[string]float64 `json:"min_cost"`
	MaxCost         map[string]float64 `json:"max_cost"`
	TotalSavings    map[string]float64 `json:"total_savings"`
	ProjectedAnnual map[string]float64 `json:"projected_annual"`
}

// NewStore creates a new cost intelligence store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS cost_estimates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			vendor TEXT NOT NULL,
			total_monthly REAL NOT NULL,
			total_annual REAL NOT NULL,
			breakdown TEXT,
			usage TEXT,
			INDEX idx_vendor_time (vendor, timestamp)
		);

		CREATE TABLE IF NOT EXISTS cost_alerts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			vendor TEXT NOT NULL,
			alert_type TEXT NOT NULL,
			threshold REAL,
			current_value REAL,
			message TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_cost_estimates_vendor_time
			ON cost_estimates(vendor, timestamp);
		CREATE INDEX IF NOT EXISTS idx_cost_estimates_timestamp
			ON cost_estimates(timestamp);
	`)
	return err
}

// RecordEstimate stores a cost estimate
func (s *Store) RecordEstimate(estimate *CostEstimate, usage UsageMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	breakdownJSON, err := json.Marshal(estimate.Breakdown)
	if err != nil {
		return err
	}

	usageJSON, err := json.Marshal(usage)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO cost_estimates (timestamp, vendor, total_monthly, total_annual, breakdown, usage)
		VALUES (?, ?, ?, ?, ?, ?)
	`, time.Now(), estimate.Vendor, estimate.TotalMonthly, estimate.TotalAnnual, breakdownJSON, usageJSON)

	return err
}

// RecordComparison stores estimates for all vendors
func (s *Store) RecordComparison(comparison *CostComparison) error {
	for _, estimate := range comparison.Estimates {
		if err := s.RecordEstimate(estimate, comparison.Usage); err != nil {
			return err
		}
	}
	return nil
}

// GetTrend returns cost trend for a vendor over a time period
func (s *Store) GetTrend(vendor string, since time.Duration) (*CostTrend, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startTime := time.Now().Add(-since)

	rows, err := s.db.Query(`
		SELECT timestamp, total_monthly
		FROM cost_estimates
		WHERE vendor = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, vendor, startTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dataPoints []TrendDataPoint
	for rows.Next() {
		var dp TrendDataPoint
		if err := rows.Scan(&dp.Timestamp, &dp.TotalMonthly); err != nil {
			return nil, err
		}
		dataPoints = append(dataPoints, dp)
	}

	if len(dataPoints) == 0 {
		return &CostTrend{
			Vendor:     vendor,
			Trend:      "unknown",
			DataPoints: []TrendDataPoint{},
		}, nil
	}

	trend := &CostTrend{
		Vendor:      vendor,
		DataPoints:  dataPoints,
		CurrentCost: dataPoints[len(dataPoints)-1].TotalMonthly,
	}

	if len(dataPoints) > 1 {
		trend.PreviousCost = dataPoints[0].TotalMonthly
		trend.Change = trend.CurrentCost - trend.PreviousCost
		if trend.PreviousCost > 0 {
			trend.ChangePercent = (trend.Change / trend.PreviousCost) * 100
		}

		if trend.Change > 0.01 {
			trend.Trend = "up"
		} else if trend.Change < -0.01 {
			trend.Trend = "down"
		} else {
			trend.Trend = "stable"
		}
	} else {
		trend.Trend = "stable"
	}

	return trend, nil
}

// GetAllTrends returns trends for all vendors
func (s *Store) GetAllTrends(since time.Duration) (map[string]*CostTrend, error) {
	vendors := []string{"Datadog", "New Relic", "Splunk"}
	trends := make(map[string]*CostTrend)

	for _, vendor := range vendors {
		trend, err := s.GetTrend(vendor, since)
		if err != nil {
			return nil, err
		}
		trends[vendor] = trend
	}

	return trends, nil
}

// GetSummary returns aggregated cost statistics for a period
func (s *Store) GetSummary(since time.Duration) (*CostSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	startTime := time.Now().Add(-since)
	endTime := time.Now()

	summary := &CostSummary{
		Period:          since.String(),
		StartTime:       startTime,
		EndTime:         endTime,
		AverageCost:     make(map[string]float64),
		MinCost:         make(map[string]float64),
		MaxCost:         make(map[string]float64),
		TotalSavings:    make(map[string]float64),
		ProjectedAnnual: make(map[string]float64),
	}

	rows, err := s.db.Query(`
		SELECT vendor,
			   AVG(total_monthly) as avg_cost,
			   MIN(total_monthly) as min_cost,
			   MAX(total_monthly) as max_cost
		FROM cost_estimates
		WHERE timestamp >= ?
		GROUP BY vendor
	`, startTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var vendor string
		var avgCost, minCost, maxCost float64
		if err := rows.Scan(&vendor, &avgCost, &minCost, &maxCost); err != nil {
			return nil, err
		}
		summary.AverageCost[vendor] = avgCost
		summary.MinCost[vendor] = minCost
		summary.MaxCost[vendor] = maxCost
		summary.TotalSavings[vendor] = avgCost * 12 // Annual savings vs dogwatch
		summary.ProjectedAnnual[vendor] = avgCost * 12
	}

	return summary, nil
}

// GetLatestEstimates returns the most recent estimate for each vendor
func (s *Store) GetLatestEstimates() (map[string]*HistoricalEstimate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	estimates := make(map[string]*HistoricalEstimate)
	vendors := []string{"Datadog", "New Relic", "Splunk"}

	for _, vendor := range vendors {
		row := s.db.QueryRow(`
			SELECT id, timestamp, vendor, total_monthly, total_annual, breakdown, usage
			FROM cost_estimates
			WHERE vendor = ?
			ORDER BY timestamp DESC
			LIMIT 1
		`, vendor)

		var est HistoricalEstimate
		var breakdownJSON, usageJSON string
		err := row.Scan(&est.ID, &est.Timestamp, &est.Vendor, &est.TotalMonthly, &est.TotalAnnual, &breakdownJSON, &usageJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(breakdownJSON), &est.Breakdown)
		json.Unmarshal([]byte(usageJSON), &est.Usage)
		estimates[vendor] = &est
	}

	return estimates, nil
}

// GetHistory returns historical estimates for a vendor
func (s *Store) GetHistory(vendor string, limit int) ([]HistoricalEstimate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, timestamp, vendor, total_monthly, total_annual, breakdown, usage
		FROM cost_estimates
		WHERE vendor = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, vendor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var estimates []HistoricalEstimate
	for rows.Next() {
		var est HistoricalEstimate
		var breakdownJSON, usageJSON string
		if err := rows.Scan(&est.ID, &est.Timestamp, &est.Vendor, &est.TotalMonthly, &est.TotalAnnual, &breakdownJSON, &usageJSON); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(breakdownJSON), &est.Breakdown)
		json.Unmarshal([]byte(usageJSON), &est.Usage)
		estimates = append(estimates, est)
	}

	return estimates, nil
}

// Cleanup removes old records
func (s *Store) Cleanup(maxAge time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	result, err := s.db.Exec(`DELETE FROM cost_estimates WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}
