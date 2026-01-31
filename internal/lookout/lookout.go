// Package lookout provides the "What's Different Right Now" homepage
// that shows all anomalies, alerts, and significant changes at a glance.
package lookout

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"dogwatch/internal/alerting"
	"dogwatch/internal/anomaly"
	"dogwatch/internal/catalog"
)

// Severity levels for Lookout items
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// ItemType categorizes Lookout items
type ItemType string

const (
	TypeAnomaly ItemType = "anomaly"   // ML-detected anomaly
	TypeAlert   ItemType = "alert"     // Firing alert rule
	TypeChange  ItemType = "change"    // Significant metric change
	TypeHealth  ItemType = "health"    // Service health change
)

// Item represents a single item in the Lookout view
type Item struct {
	ID          string    `json:"id"`
	Type        ItemType  `json:"type"`
	Severity    Severity  `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`

	// Entity information
	ServiceID   string `json:"service_id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	MetricName  string `json:"metric_name,omitempty"`

	// Values
	CurrentValue  float64 `json:"current_value,omitempty"`
	BaselineValue float64 `json:"baseline_value,omitempty"`
	Deviation     float64 `json:"deviation,omitempty"`      // Standard deviations
	ChangePercent float64 `json:"change_percent,omitempty"` // Percentage change

	// Impact assessment
	Impact       string   `json:"impact"`         // "critical", "high", "medium", "low"
	AffectedBy   []string `json:"affected_by,omitempty"`   // Upstream causes
	BlastRadius  int      `json:"blast_radius,omitempty"`  // Downstream count

	// Timing
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Duration  string    `json:"duration"` // Human-readable

	// Related items
	RelatedAlerts    []string `json:"related_alerts,omitempty"`
	RelatedAnomalies []string `json:"related_anomalies,omitempty"`
}

// Overview is the main Lookout response
type Overview struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalItems  int       `json:"total_items"`

	// Grouped by severity
	Critical []Item `json:"critical"`
	Warning  []Item `json:"warning"`
	Info     []Item `json:"info"`

	// Summary stats
	CriticalCount int `json:"critical_count"`
	WarningCount  int `json:"warning_count"`
	InfoCount     int `json:"info_count"`

	// Health summary
	UnhealthyServices int `json:"unhealthy_services"`
	DegradedServices  int `json:"degraded_services"`
	TotalServices     int `json:"total_services"`
}

// Engine is the main Lookout engine
type Engine struct {
	catalogStore   *catalog.Store
	anomalyService *anomaly.Service
	alertManager   *alerting.AlertManager

	// Cached overview
	lastOverview   *Overview
	lastUpdateAt   time.Time
	cacheTTL       time.Duration

	mu sync.RWMutex
}

// Config for the Lookout engine
type Config struct {
	CacheTTL time.Duration
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		CacheTTL: 10 * time.Second,
	}
}

// NewEngine creates a new Lookout engine
func NewEngine(cfg Config) *Engine {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = DefaultConfig().CacheTTL
	}

	return &Engine{
		cacheTTL: cfg.CacheTTL,
	}
}

// SetCatalogStore sets the catalog store
func (e *Engine) SetCatalogStore(store *catalog.Store) {
	e.mu.Lock()
	e.catalogStore = store
	e.mu.Unlock()
}

// SetAnomalyService sets the anomaly detection service
func (e *Engine) SetAnomalyService(svc *anomaly.Service) {
	e.mu.Lock()
	e.anomalyService = svc
	e.mu.Unlock()
}

// SetAlertManager sets the alert manager
func (e *Engine) SetAlertManager(am *alerting.AlertManager) {
	e.mu.Lock()
	e.alertManager = am
	e.mu.Unlock()
}

// GetOverview returns the current Lookout overview
func (e *Engine) GetOverview() *Overview {
	e.mu.RLock()
	if e.lastOverview != nil && time.Since(e.lastUpdateAt) < e.cacheTTL {
		overview := e.lastOverview
		e.mu.RUnlock()
		return overview
	}
	e.mu.RUnlock()

	// Need to refresh
	return e.refresh()
}

// refresh rebuilds the overview from all sources
func (e *Engine) refresh() *Overview {
	e.mu.Lock()
	defer e.mu.Unlock()

	overview := &Overview{
		Timestamp: time.Now(),
		Critical:  []Item{},
		Warning:   []Item{},
		Info:      []Item{},
	}

	// Collect items from all sources
	var items []Item

	// 1. Get anomalies from anomaly service
	items = append(items, e.collectAnomalies()...)

	// 2. Get firing alerts
	items = append(items, e.collectAlerts()...)

	// 3. Get service health issues
	items = append(items, e.collectHealthIssues()...)

	// Sort by severity and time
	sort.Slice(items, func(i, j int) bool {
		// First by severity
		if severityRank(items[i].Severity) != severityRank(items[j].Severity) {
			return severityRank(items[i].Severity) > severityRank(items[j].Severity)
		}
		// Then by impact
		if impactRank(items[i].Impact) != impactRank(items[j].Impact) {
			return impactRank(items[i].Impact) > impactRank(items[j].Impact)
		}
		// Then by time (most recent first)
		return items[i].LastSeen.After(items[j].LastSeen)
	})

	// Group by severity
	for _, item := range items {
		switch item.Severity {
		case SeverityCritical:
			overview.Critical = append(overview.Critical, item)
			overview.CriticalCount++
		case SeverityWarning:
			overview.Warning = append(overview.Warning, item)
			overview.WarningCount++
		default:
			overview.Info = append(overview.Info, item)
			overview.InfoCount++
		}
	}

	overview.TotalItems = len(items)

	// Get service health summary
	if e.catalogStore != nil {
		overview.UnhealthyServices, overview.DegradedServices, overview.TotalServices = e.getServiceHealthSummary()
	}

	e.lastOverview = overview
	e.lastUpdateAt = time.Now()

	return overview
}

// collectAnomalies gets recent anomalies from the anomaly service
func (e *Engine) collectAnomalies() []Item {
	if e.anomalyService == nil {
		return nil
	}

	var items []Item
	anomalies := e.anomalyService.GetRecentAnomalies(100)

	// Only include anomalies from last hour
	cutoff := time.Now().Add(-1 * time.Hour)

	for _, a := range anomalies {
		if a.Timestamp.Before(cutoff) {
			continue
		}

		severity := SeverityInfo
		if a.IsCritical {
			severity = SeverityCritical
		} else if a.IsAnomaly {
			severity = SeverityWarning
		}

		item := Item{
			ID:            a.MetricName + "-" + a.Timestamp.Format("150405"),
			Type:          TypeAnomaly,
			Severity:      severity,
			Title:         formatAnomalyTitle(a),
			Description:   a.Reason,
			MetricName:    a.MetricName,
			CurrentValue:  a.Value,
			BaselineValue: a.Prediction,
			Deviation:     a.Score * 3, // Approximate z-score
			Impact:        estimateImpact(a.Score, a.IsCritical),
			FirstSeen:     a.Timestamp,
			LastSeen:      a.Timestamp,
			Duration:      formatDuration(time.Since(a.Timestamp)),
		}

		// Try to link to service
		if e.catalogStore != nil {
			if svc := e.findServiceForMetric(a.MetricName); svc != nil {
				item.ServiceID = svc.ID
				item.ServiceName = svc.Name
			}
		}

		items = append(items, item)
	}

	return items
}

// collectAlerts gets firing alerts from the alert manager
func (e *Engine) collectAlerts() []Item {
	if e.alertManager == nil {
		return nil
	}

	var items []Item
	alerts := e.alertManager.GetFiringAlerts()

	for _, a := range alerts {
		severity := SeverityInfo
		switch a.Severity {
		case alerting.SeverityCritical:
			severity = SeverityCritical
		case alerting.SeverityWarning:
			severity = SeverityWarning
		}

		// Get summary from annotations, fallback to rule name
		summary := ""
		if s, ok := a.Annotations["summary"]; ok {
			summary = s
		} else if s, ok := a.Annotations["description"]; ok {
			summary = s
		}

		// Handle pointer time fields
		firstSeen := a.StartsAt
		if a.FiredAt != nil {
			firstSeen = *a.FiredAt
		}

		item := Item{
			ID:          a.ID,
			Type:        TypeAlert,
			Severity:    severity,
			Title:       a.RuleName,
			Description: summary,
			Impact:      string(a.Severity),
			FirstSeen:   firstSeen,
			LastSeen:    time.Now(),
			Duration:    formatDuration(time.Since(firstSeen)),
		}

		// Extract service from labels
		if svcID, ok := a.Labels["service"]; ok {
			item.ServiceID = svcID
		} else if svcID, ok := a.Labels["service_id"]; ok {
			item.ServiceID = svcID
		}
		if svcName, ok := a.Labels["service_name"]; ok {
			item.ServiceName = svcName
		}

		items = append(items, item)
	}

	return items
}

// collectHealthIssues gets services with health issues
func (e *Engine) collectHealthIssues() []Item {
	if e.catalogStore == nil {
		return nil
	}

	var items []Item

	// Get all services - use default org
	services, err := e.catalogStore.ListServices("default", catalog.ServiceFilters{})
	if err != nil {
		log.Printf("[lookout] Error listing services: %v", err)
		return nil
	}

	for _, svc := range services {
		if svc.Health == catalog.HealthUnhealthy || svc.Health == catalog.HealthDegraded {
			severity := SeverityWarning
			if svc.Health == catalog.HealthUnhealthy {
				severity = SeverityCritical
			}

			item := Item{
				ID:          "health-" + svc.ID,
				Type:        TypeHealth,
				Severity:    severity,
				Title:       svc.Name + " is " + string(svc.Health),
				Description: "Service health status is " + string(svc.Health),
				ServiceID:   svc.ID,
				ServiceName: svc.Name,
				Impact:      string(svc.Tier),
				FirstSeen:   svc.UpdatedAt,
				LastSeen:    time.Now(),
				Duration:    formatDuration(time.Since(svc.UpdatedAt)),
			}

			items = append(items, item)
		}
	}

	return items
}

// getServiceHealthSummary returns unhealthy, degraded, and total service counts
func (e *Engine) getServiceHealthSummary() (unhealthy, degraded, total int) {
	services, err := e.catalogStore.ListServices("default", catalog.ServiceFilters{})
	if err != nil {
		return 0, 0, 0
	}

	total = len(services)
	for _, svc := range services {
		switch svc.Health {
		case catalog.HealthUnhealthy:
			unhealthy++
		case catalog.HealthDegraded:
			degraded++
		}
	}

	return unhealthy, degraded, total
}

// findServiceForMetric tries to find a service associated with a metric
func (e *Engine) findServiceForMetric(metricName string) *catalog.Service {
	// This is a simple heuristic - could be improved with better metric-service mapping
	// For now, return nil and rely on explicit service labels in alerts
	return nil
}

// Helper functions

func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func impactRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func formatAnomalyTitle(a anomaly.AnomalyResult) string {
	if a.IsCritical {
		return a.MetricName + ": Critical anomaly"
	}
	return a.MetricName + ": Anomaly detected"
}

func estimateImpact(score float64, isCritical bool) string {
	if isCritical {
		return "critical"
	}
	if score >= 0.8 {
		return "high"
	}
	if score >= 0.6 {
		return "medium"
	}
	return "low"
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

