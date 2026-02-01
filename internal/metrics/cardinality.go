package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// CardinalityController provides real-time cardinality monitoring and prevention
type CardinalityController struct {
	mu sync.RWMutex

	// Configuration
	config CardinalityConfig

	// Real-time tracking
	metricSeries  map[string]*MetricSeriesTracker // metric_name -> tracker
	globalLabels  map[string]*LabelTracker        // label_key -> tracker

	// Circuit breaker state
	circuitBreaker *CircuitBreaker

	// Quarantine
	quarantined map[string]*QuarantineEntry // metric_name -> entry

	// Alert state
	alertCallback AlertCallback
	recentAlerts  []CardinalityAlert
	maxAlerts     int

	// Stats
	stats CardinalityStats
}

// CardinalityConfig configures the controller
type CardinalityConfig struct {
	// MaxSeriesPerMetric limits unique series per metric name
	MaxSeriesPerMetric int
	// MaxLabelValues limits unique values per label key
	MaxLabelValues int
	// CircuitBreakerThreshold triggers circuit breaker when hit
	CircuitBreakerThreshold int
	// CircuitBreakerCooldown is how long to stay open
	CircuitBreakerCooldown time.Duration
	// AlertThreshold triggers alert when cardinality exceeds
	AlertThreshold int
	// RapidGrowthWindow is the time window to detect rapid growth
	RapidGrowthWindow time.Duration
	// RapidGrowthThreshold is % growth that triggers alert
	RapidGrowthThreshold float64
	// AutoAggregateThreshold automatically aggregates above this
	AutoAggregateThreshold int
	// ProblematicLabels are known high-cardinality label patterns
	ProblematicLabels []string
	// EnableQuarantine enables automatic quarantine of bad metrics
	EnableQuarantine bool
}

// DefaultCardinalityConfig returns sensible defaults
func DefaultCardinalityConfig() CardinalityConfig {
	return CardinalityConfig{
		MaxSeriesPerMetric:      10000,
		MaxLabelValues:          5000,
		CircuitBreakerThreshold: 50000,
		CircuitBreakerCooldown:  5 * time.Minute,
		AlertThreshold:          5000,
		RapidGrowthWindow:       5 * time.Minute,
		RapidGrowthThreshold:    50.0, // 50% growth
		AutoAggregateThreshold:  8000,
		ProblematicLabels: []string{
			"request_id", "trace_id", "span_id", "correlation_id",
			"uuid", "guid", "id", "session_id", "user_id",
			"timestamp", "time", "ts",
			"ip", "ip_address", "client_ip",
			"url", "uri", "path", "query",
		},
		EnableQuarantine: true,
	}
}

// MetricSeriesTracker tracks cardinality for a single metric
type MetricSeriesTracker struct {
	Name            string                  `json:"name"`
	TotalSeries     int                     `json:"total_series"`
	UniqueLabels    map[string]int          `json:"unique_labels"` // label_key -> count
	SeriesKeys      map[string]time.Time    `json:"series_keys,omitempty"` // series_key -> last_seen
	GrowthHistory   []CardinalitySnapshot   `json:"growth_history,omitempty"`
	FirstSeen       time.Time               `json:"first_seen"`
	LastSeen        time.Time               `json:"last_seen"`
	Blocked         int                     `json:"blocked"`
	AutoAggregated  bool                    `json:"auto_aggregated"`
}

// LabelTracker tracks cardinality for a single label key
type LabelTracker struct {
	Key          string           `json:"key"`
	UniqueValues int              `json:"unique_values"`
	Values       map[string]int   `json:"values,omitempty"` // value -> count
	Metrics      map[string]bool  `json:"metrics,omitempty"` // metric_name -> true
	FirstSeen    time.Time        `json:"first_seen"`
	LastSeen     time.Time        `json:"last_seen"`
}

// CardinalitySnapshot captures cardinality at a point in time
type CardinalitySnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	TotalSeries int       `json:"total_series"`
	GrowthRate  float64   `json:"growth_rate"` // series/minute
}

// CircuitBreaker prevents cardinality explosion
type CircuitBreaker struct {
	State       CircuitState  `json:"state"`
	TotalSeries int           `json:"total_series"`
	OpenedAt    time.Time     `json:"opened_at,omitempty"`
	Cooldown    time.Duration `json:"cooldown"`
	Threshold   int           `json:"threshold"`
	BlockedCount int64        `json:"blocked_count"`
}

// CircuitState represents the circuit breaker state
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// QuarantineEntry represents a quarantined metric
type QuarantineEntry struct {
	MetricName  string            `json:"metric_name"`
	Reason      string            `json:"reason"`
	QuarantinedAt time.Time       `json:"quarantined_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	OriginalLabels []string       `json:"original_labels"`
	AllowedLabels  []string       `json:"allowed_labels"`
	AutoExpire  bool              `json:"auto_expire"`
}

// CardinalityAlert represents a cardinality alert
type CardinalityAlert struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	AlertType   CardinalityAlertType `json:"alert_type"`
	Severity    string            `json:"severity"` // critical, warning, info
	MetricName  string            `json:"metric_name,omitempty"`
	LabelKey    string            `json:"label_key,omitempty"`
	CurrentValue int              `json:"current_value"`
	Threshold   int               `json:"threshold"`
	GrowthRate  float64           `json:"growth_rate,omitempty"`
	Message     string            `json:"message"`
	Action      string            `json:"action"` // What action was taken
}

// CardinalityAlertType defines alert types
type CardinalityAlertType string

const (
	AlertMetricHigh     CardinalityAlertType = "metric_high_cardinality"
	AlertLabelHigh      CardinalityAlertType = "label_high_cardinality"
	AlertRapidGrowth    CardinalityAlertType = "rapid_growth"
	AlertCircuitOpen    CardinalityAlertType = "circuit_breaker_open"
	AlertQuarantine     CardinalityAlertType = "metric_quarantined"
	AlertAutoAggregate  CardinalityAlertType = "auto_aggregated"
)

// AlertCallback is called when alerts are generated
type AlertCallback func(alert CardinalityAlert)

// CardinalityStats contains controller statistics
type CardinalityStats struct {
	TotalMetrics       int       `json:"total_metrics"`
	TotalSeries        int       `json:"total_series"`
	TotalLabelKeys     int       `json:"total_label_keys"`
	HighCardinalityMetrics int   `json:"high_cardinality_metrics"`
	QuarantinedMetrics int       `json:"quarantined_metrics"`
	BlockedRequests    int64     `json:"blocked_requests"`
	AlertsGenerated    int       `json:"alerts_generated"`
	CircuitBreakerState string   `json:"circuit_breaker_state"`
	LastUpdated        time.Time `json:"last_updated"`
}

// NewCardinalityController creates a new cardinality controller
func NewCardinalityController(config CardinalityConfig) *CardinalityController {
	return &CardinalityController{
		config:       config,
		metricSeries: make(map[string]*MetricSeriesTracker),
		globalLabels: make(map[string]*LabelTracker),
		quarantined:  make(map[string]*QuarantineEntry),
		maxAlerts:    100,
		circuitBreaker: &CircuitBreaker{
			State:     CircuitClosed,
			Cooldown:  config.CircuitBreakerCooldown,
			Threshold: config.CircuitBreakerThreshold,
		},
	}
}

// SetAlertCallback sets the alert callback function
func (c *CardinalityController) SetAlertCallback(cb AlertCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alertCallback = cb
}

// RecordSeriesResult indicates whether a series should be accepted
type RecordSeriesResult struct {
	Accept           bool              `json:"accept"`
	Reason           string            `json:"reason,omitempty"`
	TransformedLabels map[string]string `json:"transformed_labels,omitempty"`
	Quarantined      bool              `json:"quarantined"`
	CircuitOpen      bool              `json:"circuit_open"`
}

// RecordSeries evaluates and optionally records a metric series
func (c *CardinalityController) RecordSeries(name string, labels map[string]string) RecordSeriesResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	result := RecordSeriesResult{Accept: true}

	// Check circuit breaker first
	if c.circuitBreaker.State == CircuitOpen {
		// Check if cooldown has passed
		if time.Since(c.circuitBreaker.OpenedAt) > c.circuitBreaker.Cooldown {
			c.circuitBreaker.State = CircuitHalfOpen
		} else {
			c.circuitBreaker.BlockedCount++
			result.Accept = false
			result.Reason = "circuit_breaker_open"
			result.CircuitOpen = true
			return result
		}
	}

	// Check if metric is quarantined
	if entry, ok := c.quarantined[name]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			// Apply quarantine rules - only allow specific labels
			if len(entry.AllowedLabels) > 0 {
				newLabels := make(map[string]string)
				for _, key := range entry.AllowedLabels {
					if val, exists := labels[key]; exists {
						newLabels[key] = val
					}
				}
				result.TransformedLabels = newLabels
				result.Quarantined = true
			} else {
				c.stats.BlockedRequests++
				result.Accept = false
				result.Reason = "metric_quarantined"
				result.Quarantined = true
				return result
			}
		} else if entry.AutoExpire {
			delete(c.quarantined, name)
		}
	}

	// Get or create metric tracker
	tracker := c.metricSeries[name]
	if tracker == nil {
		tracker = &MetricSeriesTracker{
			Name:         name,
			UniqueLabels: make(map[string]int),
			SeriesKeys:   make(map[string]time.Time),
			FirstSeen:    time.Now(),
		}
		c.metricSeries[name] = tracker
	}
	tracker.LastSeen = time.Now()

	// Generate series key
	seriesKey := buildSeriesKey(name, labels)

	// Check if this is a new series
	if _, exists := tracker.SeriesKeys[seriesKey]; !exists {

		// Check metric cardinality limit
		if tracker.TotalSeries >= c.config.MaxSeriesPerMetric {
			tracker.Blocked++
			c.stats.BlockedRequests++
			result.Accept = false
			result.Reason = fmt.Sprintf("metric_cardinality_limit_%d", c.config.MaxSeriesPerMetric)
			return result
		}

		// Check for problematic labels
		for key := range labels {
			if c.isProblematicLabel(key) {
				if labelTracker, ok := c.globalLabels[key]; ok {
					if labelTracker.UniqueValues >= c.config.MaxLabelValues {
						// Auto-aggregate by removing problematic label
						if result.TransformedLabels == nil {
							result.TransformedLabels = make(map[string]string)
							for k, v := range labels {
								if k != key {
									result.TransformedLabels[k] = v
								}
							}
						} else {
							delete(result.TransformedLabels, key)
						}
						c.generateAlert(CardinalityAlert{
							AlertType:   AlertAutoAggregate,
							Severity:    "warning",
							MetricName:  name,
							LabelKey:    key,
							Message:     fmt.Sprintf("Auto-removed high-cardinality label %s from %s", key, name),
							Action:      "label_removed",
						})
					}
				}
			}
		}

		// Record the series
		tracker.SeriesKeys[seriesKey] = time.Now()
		tracker.TotalSeries++

		// Update label tracking
		for key, value := range labels {
			tracker.UniqueLabels[key]++

			if c.globalLabels[key] == nil {
				c.globalLabels[key] = &LabelTracker{
					Key:       key,
					Values:    make(map[string]int),
					Metrics:   make(map[string]bool),
					FirstSeen: time.Now(),
				}
			}
			lt := c.globalLabels[key]
			lt.LastSeen = time.Now()
			lt.Values[value]++
			lt.UniqueValues = len(lt.Values)
			lt.Metrics[name] = true
		}

		// Update circuit breaker total
		c.circuitBreaker.TotalSeries++

		// Check if we should open circuit breaker
		if c.circuitBreaker.State != CircuitOpen &&
			c.circuitBreaker.TotalSeries >= c.circuitBreaker.Threshold {
			c.circuitBreaker.State = CircuitOpen
			c.circuitBreaker.OpenedAt = time.Now()
			c.generateAlert(CardinalityAlert{
				AlertType:    AlertCircuitOpen,
				Severity:     "critical",
				CurrentValue: c.circuitBreaker.TotalSeries,
				Threshold:    c.circuitBreaker.Threshold,
				Message:      "Circuit breaker opened due to high total cardinality",
				Action:       "circuit_breaker_opened",
			})
		}

		// Check for rapid growth
		c.checkRapidGrowth(tracker)

		// Check if metric should be quarantined
		if c.config.EnableQuarantine && tracker.TotalSeries >= c.config.AutoAggregateThreshold {
			c.quarantineMetric(name, "high_cardinality", 1*time.Hour)
		}

		// Generate alerts if thresholds exceeded
		if tracker.TotalSeries >= c.config.AlertThreshold {
			c.generateAlert(CardinalityAlert{
				AlertType:    AlertMetricHigh,
				Severity:     severityForCount(tracker.TotalSeries, c.config),
				MetricName:   name,
				CurrentValue: tracker.TotalSeries,
				Threshold:    c.config.AlertThreshold,
				Message:      fmt.Sprintf("Metric %s has %d unique series", name, tracker.TotalSeries),
				Action:       "alert_generated",
			})
		}
	}

	// Update stats
	c.updateStats()

	// If half-open and request accepted, move to closed
	// (Any successful request in half-open transitions to closed)
	if c.circuitBreaker.State == CircuitHalfOpen {
		c.circuitBreaker.State = CircuitClosed
	}

	return result
}

// checkRapidGrowth checks for rapid cardinality growth
func (c *CardinalityController) checkRapidGrowth(tracker *MetricSeriesTracker) {
	now := time.Now()

	// Add snapshot
	tracker.GrowthHistory = append(tracker.GrowthHistory, CardinalitySnapshot{
		Timestamp:   now,
		TotalSeries: tracker.TotalSeries,
	})

	// Keep only recent history
	cutoff := now.Add(-c.config.RapidGrowthWindow)
	var recent []CardinalitySnapshot
	for _, snap := range tracker.GrowthHistory {
		if snap.Timestamp.After(cutoff) {
			recent = append(recent, snap)
		}
	}
	tracker.GrowthHistory = recent

	// Calculate growth rate
	if len(recent) >= 2 {
		oldest := recent[0]
		newest := recent[len(recent)-1]
		duration := newest.Timestamp.Sub(oldest.Timestamp)

		if duration > 0 {
			growth := float64(newest.TotalSeries-oldest.TotalSeries) / duration.Minutes()

			// Calculate percentage growth
			if oldest.TotalSeries > 0 {
				pctGrowth := float64(newest.TotalSeries-oldest.TotalSeries) / float64(oldest.TotalSeries) * 100

				if pctGrowth >= c.config.RapidGrowthThreshold {
					c.generateAlert(CardinalityAlert{
						AlertType:    AlertRapidGrowth,
						Severity:     "warning",
						MetricName:   tracker.Name,
						CurrentValue: tracker.TotalSeries,
						GrowthRate:   growth,
						Message:      fmt.Sprintf("Metric %s growing rapidly: %.1f%% in %s", tracker.Name, pctGrowth, c.config.RapidGrowthWindow),
						Action:       "rapid_growth_detected",
					})
				}
			}
		}
	}
}

// quarantineMetric adds a metric to quarantine
func (c *CardinalityController) quarantineMetric(name, reason string, duration time.Duration) {
	entry := &QuarantineEntry{
		MetricName:    name,
		Reason:        reason,
		QuarantinedAt: time.Now(),
		ExpiresAt:     time.Now().Add(duration),
		AutoExpire:    true,
	}

	// Find low-cardinality labels to allow
	if tracker, ok := c.metricSeries[name]; ok {
		for label, count := range tracker.UniqueLabels {
			if count < 100 && !c.isProblematicLabel(label) {
				entry.AllowedLabels = append(entry.AllowedLabels, label)
			}
		}
	}

	c.quarantined[name] = entry

	c.generateAlert(CardinalityAlert{
		AlertType:  AlertQuarantine,
		Severity:   "warning",
		MetricName: name,
		Message:    fmt.Sprintf("Metric %s quarantined for %s: %s", name, duration, reason),
		Action:     "quarantined",
	})

	log.Printf("[cardinality] Quarantined metric %s: %s (allowed labels: %v)", name, reason, entry.AllowedLabels)
}

// generateAlert creates and dispatches an alert
func (c *CardinalityController) generateAlert(alert CardinalityAlert) {
	alert.ID = fmt.Sprintf("card-%d", time.Now().UnixNano())
	alert.Timestamp = time.Now()

	c.recentAlerts = append(c.recentAlerts, alert)
	if len(c.recentAlerts) > c.maxAlerts {
		c.recentAlerts = c.recentAlerts[1:]
	}
	c.stats.AlertsGenerated++

	if c.alertCallback != nil {
		go c.alertCallback(alert)
	}

	log.Printf("[cardinality] Alert: %s - %s (metric=%s, value=%d)",
		alert.AlertType, alert.Message, alert.MetricName, alert.CurrentValue)
}

// updateStats updates controller statistics
func (c *CardinalityController) updateStats() {
	totalSeries := 0
	highCard := 0
	for _, tracker := range c.metricSeries {
		totalSeries += tracker.TotalSeries
		if tracker.TotalSeries >= c.config.AlertThreshold {
			highCard++
		}
	}

	c.stats.TotalMetrics = len(c.metricSeries)
	c.stats.TotalSeries = totalSeries
	c.stats.TotalLabelKeys = len(c.globalLabels)
	c.stats.HighCardinalityMetrics = highCard
	c.stats.QuarantinedMetrics = len(c.quarantined)
	c.stats.CircuitBreakerState = string(c.circuitBreaker.State)
	c.stats.LastUpdated = time.Now()
}

// isProblematicLabel checks if a label key is known to cause high cardinality
func (c *CardinalityController) isProblematicLabel(key string) bool {
	keyLower := toLowerSimple(key)
	for _, problematic := range c.config.ProblematicLabels {
		if keyLower == problematic || containsSimple(keyLower, problematic) {
			return true
		}
	}
	return false
}

// GetStats returns current statistics
func (c *CardinalityController) GetStats() CardinalityStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	c.updateStats()
	return c.stats
}

// GetMetricCardinality returns cardinality info for a metric
func (c *CardinalityController) GetMetricCardinality(name string) *MetricSeriesTracker {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metricSeries[name]
}

// GetTopMetrics returns metrics sorted by cardinality
func (c *CardinalityController) GetTopMetrics(limit int) []*MetricSeriesTracker {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var metrics []*MetricSeriesTracker
	for _, tracker := range c.metricSeries {
		metrics = append(metrics, tracker)
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].TotalSeries > metrics[j].TotalSeries
	})

	if limit > 0 && len(metrics) > limit {
		return metrics[:limit]
	}
	return metrics
}

// GetLabelCardinality returns cardinality info for a label
func (c *CardinalityController) GetLabelCardinality(key string) *LabelTracker {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.globalLabels[key]
}

// GetTopLabels returns labels sorted by cardinality
func (c *CardinalityController) GetTopLabels(limit int) []*LabelTracker {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var labels []*LabelTracker
	for _, tracker := range c.globalLabels {
		labels = append(labels, tracker)
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].UniqueValues > labels[j].UniqueValues
	})

	if limit > 0 && len(labels) > limit {
		return labels[:limit]
	}
	return labels
}

// GetQuarantinedMetrics returns all quarantined metrics
func (c *CardinalityController) GetQuarantinedMetrics() []*QuarantineEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var entries []*QuarantineEntry
	for _, entry := range c.quarantined {
		entries = append(entries, entry)
	}
	return entries
}

// GetRecentAlerts returns recent alerts
func (c *CardinalityController) GetRecentAlerts(limit int) []CardinalityAlert {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 || limit > len(c.recentAlerts) {
		limit = len(c.recentAlerts)
	}

	// Return most recent
	result := make([]CardinalityAlert, limit)
	copy(result, c.recentAlerts[len(c.recentAlerts)-limit:])
	return result
}

// GetCircuitBreakerState returns circuit breaker state
func (c *CardinalityController) GetCircuitBreakerState() *CircuitBreaker {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.circuitBreaker
}

// ResetCircuitBreaker manually resets the circuit breaker
func (c *CardinalityController) ResetCircuitBreaker() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.circuitBreaker.State = CircuitClosed
	c.circuitBreaker.BlockedCount = 0
	log.Printf("[cardinality] Circuit breaker manually reset")
}

// UnquarantineMetric removes a metric from quarantine
func (c *CardinalityController) UnquarantineMetric(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.quarantined, name)
	log.Printf("[cardinality] Metric %s removed from quarantine", name)
}

// SetQuarantineRules sets allowed labels for a quarantined metric
func (c *CardinalityController) SetQuarantineRules(name string, allowedLabels []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.quarantined[name]; ok {
		entry.AllowedLabels = allowedLabels
	}
}

// GetDashboardData returns data suitable for a cardinality dashboard
func (c *CardinalityController) GetDashboardData() *CardinalityDashboard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dashboard := &CardinalityDashboard{
		Stats:             c.stats,
		CircuitBreaker:    c.circuitBreaker,
		TopMetrics:        c.getTopMetricsLocked(10),
		TopLabels:         c.getTopLabelsLocked(10),
		QuarantinedCount:  len(c.quarantined),
		RecentAlerts:      c.getRecentAlertsLocked(10),
		Trends:            c.calculateTrends(),
	}
	return dashboard
}

func (c *CardinalityController) getTopMetricsLocked(limit int) []MetricSummary {
	var metrics []*MetricSeriesTracker
	for _, tracker := range c.metricSeries {
		metrics = append(metrics, tracker)
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].TotalSeries > metrics[j].TotalSeries
	})

	var result []MetricSummary
	for i, m := range metrics {
		if i >= limit {
			break
		}
		_, quarantined := c.quarantined[m.Name]
		result = append(result, MetricSummary{
			Name:        m.Name,
			TotalSeries: m.TotalSeries,
			LabelCount:  len(m.UniqueLabels),
			LastSeen:    m.LastSeen,
			Quarantined: quarantined,
			Blocked:     m.Blocked,
		})
	}
	return result
}

func (c *CardinalityController) getTopLabelsLocked(limit int) []LabelSummary {
	var labels []*LabelTracker
	for _, tracker := range c.globalLabels {
		labels = append(labels, tracker)
	}

	sort.Slice(labels, func(i, j int) bool {
		return labels[i].UniqueValues > labels[j].UniqueValues
	})

	var result []LabelSummary
	for i, l := range labels {
		if i >= limit {
			break
		}
		result = append(result, LabelSummary{
			Key:           l.Key,
			UniqueValues:  l.UniqueValues,
			MetricCount:   len(l.Metrics),
			IsProblematic: c.isProblematicLabel(l.Key),
		})
	}
	return result
}

func (c *CardinalityController) getRecentAlertsLocked(limit int) []CardinalityAlert {
	if limit > len(c.recentAlerts) {
		limit = len(c.recentAlerts)
	}
	result := make([]CardinalityAlert, limit)
	copy(result, c.recentAlerts[len(c.recentAlerts)-limit:])
	return result
}

func (c *CardinalityController) calculateTrends() CardinalityTrends {
	// Calculate growth trends from history
	trends := CardinalityTrends{
		GrowingMetrics:    []string{},
		StableMetrics:     0,
		DecliningMetrics:  0,
	}

	for name, tracker := range c.metricSeries {
		if len(tracker.GrowthHistory) >= 2 {
			oldest := tracker.GrowthHistory[0]
			newest := tracker.GrowthHistory[len(tracker.GrowthHistory)-1]
			growth := newest.TotalSeries - oldest.TotalSeries

			if growth > 100 {
				trends.GrowingMetrics = append(trends.GrowingMetrics, name)
			} else if growth < -10 {
				trends.DecliningMetrics++
			} else {
				trends.StableMetrics++
			}
		} else {
			trends.StableMetrics++
		}
	}

	return trends
}

// CardinalityDashboard contains all dashboard data
type CardinalityDashboard struct {
	Stats            CardinalityStats    `json:"stats"`
	CircuitBreaker   *CircuitBreaker     `json:"circuit_breaker"`
	TopMetrics       []MetricSummary     `json:"top_metrics"`
	TopLabels        []LabelSummary      `json:"top_labels"`
	QuarantinedCount int                 `json:"quarantined_count"`
	RecentAlerts     []CardinalityAlert  `json:"recent_alerts"`
	Trends           CardinalityTrends   `json:"trends"`
}

// MetricSummary is a summary of metric cardinality
type MetricSummary struct {
	Name        string    `json:"name"`
	TotalSeries int       `json:"total_series"`
	LabelCount  int       `json:"label_count"`
	LastSeen    time.Time `json:"last_seen"`
	Quarantined bool      `json:"quarantined"`
	Blocked     int       `json:"blocked"`
}

// LabelSummary is a summary of label cardinality
type LabelSummary struct {
	Key           string `json:"key"`
	UniqueValues  int    `json:"unique_values"`
	MetricCount   int    `json:"metric_count"`
	IsProblematic bool   `json:"is_problematic"`
}

// CardinalityTrends shows growth trends
type CardinalityTrends struct {
	GrowingMetrics   []string `json:"growing_metrics"`
	StableMetrics    int      `json:"stable_metrics"`
	DecliningMetrics int      `json:"declining_metrics"`
}

// MarshalJSON provides custom JSON marshaling
func (c *CardinalityController) MarshalJSON() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return json.Marshal(c.GetDashboardData())
}

// Helper functions

func buildSeriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	key := name
	for _, k := range keys {
		key += "|" + k + "=" + labels[k]
	}
	return key
}

func severityForCount(count int, config CardinalityConfig) string {
	if count >= config.CircuitBreakerThreshold {
		return "critical"
	}
	if count >= config.AutoAggregateThreshold {
		return "warning"
	}
	return "info"
}

func toLowerSimple(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsSimple(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
