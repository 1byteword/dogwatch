package usage

import (
	"sync"
	"time"
)

// DataType represents the type of observability data
type DataType string

const (
	DataTypeMetric DataType = "metric"
	DataTypeLog    DataType = "log"
	DataTypeTrace  DataType = "trace"
	DataTypeSpan   DataType = "span"
)

// QueryEvent represents a single query event
type QueryEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	DataType   DataType          `json:"data_type"`
	Name       string            `json:"name"`        // metric name, service name, etc.
	Tags       map[string]string `json:"tags,omitempty"`
	Query      string            `json:"query,omitempty"` // raw query if applicable
	Duration   time.Duration     `json:"duration"`
	ResultSize int               `json:"result_size"`
	Source     string            `json:"source"` // API endpoint, dashboard, alert, etc.
	UserID     string            `json:"user_id,omitempty"`
}

// UsageStats tracks usage statistics for a data item
type UsageStats struct {
	Name           string            `json:"name"`
	DataType       DataType          `json:"data_type"`
	Tags           map[string]string `json:"tags,omitempty"`
	QueryCount     int64             `json:"query_count"`
	LastQueried    time.Time         `json:"last_queried"`
	FirstQueried   time.Time         `json:"first_queried"`
	AvgResultSize  float64           `json:"avg_result_size"`
	TotalResultSize int64            `json:"total_result_size"`
	Sources        map[string]int64  `json:"sources"` // source -> count
}

// DataInventory represents known data items
type DataInventory struct {
	Name       string            `json:"name"`
	DataType   DataType          `json:"data_type"`
	Tags       map[string]string `json:"tags,omitempty"`
	FirstSeen  time.Time         `json:"first_seen"`
	LastSeen   time.Time         `json:"last_seen"`
	DataPoints int64             `json:"data_points"` // estimated count
	SizeBytes  int64             `json:"size_bytes"`  // estimated size
}

// WastedData represents data that exists but isn't queried
type WastedData struct {
	Inventory       DataInventory `json:"inventory"`
	DaysSinceQuery  int           `json:"days_since_query"`
	NeverQueried    bool          `json:"never_queried"`
	EstimatedCost   float64       `json:"estimated_cost_monthly"`
	Recommendation  string        `json:"recommendation"`
}

// UsageReport provides comprehensive usage analysis
type UsageReport struct {
	Period           string                   `json:"period"`
	StartTime        time.Time                `json:"start_time"`
	EndTime          time.Time                `json:"end_time"`
	TotalQueries     int64                    `json:"total_queries"`
	UniqueDataItems  int                      `json:"unique_data_items"`
	TopQueried       []UsageStats             `json:"top_queried"`
	NeverQueried     []WastedData             `json:"never_queried"`
	RarelyQueried    []WastedData             `json:"rarely_queried"`
	QueryByType      map[DataType]int64       `json:"query_by_type"`
	QueryBySource    map[string]int64         `json:"query_by_source"`
	QueryByHour      map[int]int64            `json:"query_by_hour"`
	WastedDataCost   float64                  `json:"wasted_data_cost_monthly"`
	Recommendations  []UsageRecommendation    `json:"recommendations"`
	GeneratedAt      time.Time                `json:"generated_at"`
}

// UsageRecommendation suggests actions based on usage
type UsageRecommendation struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"` // critical, warning, info
	DataType    DataType `json:"data_type"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
	Savings     float64 `json:"estimated_savings,omitempty"`
}

// Tracker tracks data usage patterns
type Tracker struct {
	mu sync.RWMutex

	// Query tracking
	stats     map[string]*UsageStats // key = datatype:name
	events    []QueryEvent           // recent events (ring buffer)
	eventPos  int
	eventSize int

	// Data inventory
	inventory map[string]*DataInventory

	// Aggregated stats
	queryByType   map[DataType]int64
	queryBySource map[string]int64
	queryByHour   map[int]int64

	// Cost estimation
	costPerGBMonth float64
}

// NewTracker creates a new usage tracker
func NewTracker(eventBufferSize int) *Tracker {
	if eventBufferSize <= 0 {
		eventBufferSize = 10000
	}

	return &Tracker{
		stats:         make(map[string]*UsageStats),
		events:        make([]QueryEvent, eventBufferSize),
		eventSize:     eventBufferSize,
		inventory:     make(map[string]*DataInventory),
		queryByType:   make(map[DataType]int64),
		queryBySource: make(map[string]int64),
		queryByHour:   make(map[int]int64),
		costPerGBMonth: 0.30, // Default: $0.30/GB/month (conservative)
	}
}

// RecordQuery records a query event
func (t *Tracker) RecordQuery(event QueryEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Store event in ring buffer
	t.events[t.eventPos] = event
	t.eventPos = (t.eventPos + 1) % t.eventSize

	// Update stats
	key := string(event.DataType) + ":" + event.Name
	stats, ok := t.stats[key]
	if !ok {
		stats = &UsageStats{
			Name:         event.Name,
			DataType:     event.DataType,
			Tags:         event.Tags,
			FirstQueried: event.Timestamp,
			Sources:      make(map[string]int64),
		}
		t.stats[key] = stats
	}

	stats.QueryCount++
	stats.LastQueried = event.Timestamp
	stats.TotalResultSize += int64(event.ResultSize)
	stats.AvgResultSize = float64(stats.TotalResultSize) / float64(stats.QueryCount)
	if event.Source != "" {
		stats.Sources[event.Source]++
	}

	// Update aggregates
	t.queryByType[event.DataType]++
	if event.Source != "" {
		t.queryBySource[event.Source]++
	}
	t.queryByHour[event.Timestamp.Hour()]++
}

// RecordDataItem records a data item in the inventory
func (t *Tracker) RecordDataItem(dataType DataType, name string, tags map[string]string, dataPoints int64, sizeBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := string(dataType) + ":" + name
	inv, ok := t.inventory[key]
	if !ok {
		inv = &DataInventory{
			Name:      name,
			DataType:  dataType,
			Tags:      tags,
			FirstSeen: time.Now(),
		}
		t.inventory[key] = inv
	}

	inv.LastSeen = time.Now()
	inv.DataPoints = dataPoints
	inv.SizeBytes = sizeBytes
}

// GetReport generates a usage report
func (t *Tracker) GetReport(since time.Duration) *UsageReport {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()
	startTime := now.Add(-since)

	report := &UsageReport{
		Period:        since.String(),
		StartTime:     startTime,
		EndTime:       now,
		QueryByType:   make(map[DataType]int64),
		QueryBySource: make(map[string]int64),
		QueryByHour:   make(map[int]int64),
		GeneratedAt:   now,
	}

	// Copy aggregates
	for k, v := range t.queryByType {
		report.QueryByType[k] = v
		report.TotalQueries += v
	}
	for k, v := range t.queryBySource {
		report.QueryBySource[k] = v
	}
	for k, v := range t.queryByHour {
		report.QueryByHour[k] = v
	}

	// Get top queried
	var allStats []UsageStats
	for _, s := range t.stats {
		allStats = append(allStats, *s)
	}
	sortStatsByQueryCount(allStats)
	if len(allStats) > 20 {
		report.TopQueried = allStats[:20]
	} else {
		report.TopQueried = allStats
	}
	report.UniqueDataItems = len(t.stats)

	// Find wasted data
	report.NeverQueried, report.RarelyQueried = t.findWastedData(since)

	// Calculate wasted cost
	for _, w := range report.NeverQueried {
		report.WastedDataCost += w.EstimatedCost
	}
	for _, w := range report.RarelyQueried {
		report.WastedDataCost += w.EstimatedCost * 0.5 // 50% weight for rarely queried
	}

	// Generate recommendations
	report.Recommendations = t.generateRecommendations(report)

	return report
}

// GetStats returns usage stats for a specific item
func (t *Tracker) GetStats(dataType DataType, name string) *UsageStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	key := string(dataType) + ":" + name
	if stats, ok := t.stats[key]; ok {
		copy := *stats
		return &copy
	}
	return nil
}

// GetTopQueried returns the most queried items
func (t *Tracker) GetTopQueried(dataType DataType, limit int) []UsageStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var filtered []UsageStats
	for _, s := range t.stats {
		if dataType == "" || s.DataType == dataType {
			filtered = append(filtered, *s)
		}
	}

	sortStatsByQueryCount(filtered)

	if len(filtered) > limit {
		return filtered[:limit]
	}
	return filtered
}

// GetWastedData returns data that isn't being queried
func (t *Tracker) GetWastedData(since time.Duration) []WastedData {
	t.mu.RLock()
	defer t.mu.RUnlock()

	never, rarely := t.findWastedData(since)
	return append(never, rarely...)
}

// GetRecentEvents returns recent query events
func (t *Tracker) GetRecentEvents(limit int) []QueryEvent {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if limit <= 0 || limit > t.eventSize {
		limit = t.eventSize
	}

	var events []QueryEvent
	pos := t.eventPos - 1
	for i := 0; i < limit; i++ {
		if pos < 0 {
			pos = t.eventSize - 1
		}
		if t.events[pos].Timestamp.IsZero() {
			break
		}
		events = append(events, t.events[pos])
		pos--
	}

	return events
}

// findWastedData identifies data that isn't being queried
func (t *Tracker) findWastedData(since time.Duration) ([]WastedData, []WastedData) {
	cutoff := time.Now().Add(-since)
	rarelyCutoff := time.Now().Add(-since / 4) // Rarely = queried less than 25% of period

	var neverQueried []WastedData
	var rarelyQueried []WastedData

	for key, inv := range t.inventory {
		stats := t.stats[key]

		wasted := WastedData{
			Inventory:     *inv,
			EstimatedCost: float64(inv.SizeBytes) / (1024 * 1024 * 1024) * t.costPerGBMonth,
		}

		if stats == nil {
			// Never queried
			wasted.NeverQueried = true
			wasted.DaysSinceQuery = int(time.Since(inv.FirstSeen).Hours() / 24)
			wasted.Recommendation = "Consider dropping - never queried since creation"
			neverQueried = append(neverQueried, wasted)
		} else if stats.LastQueried.Before(cutoff) {
			// Not queried in the analysis period
			wasted.DaysSinceQuery = int(time.Since(stats.LastQueried).Hours() / 24)
			wasted.Recommendation = "Consider archiving - not queried in " + since.String()
			neverQueried = append(neverQueried, wasted)
		} else if stats.LastQueried.Before(rarelyCutoff) && stats.QueryCount < 10 {
			// Rarely queried
			wasted.DaysSinceQuery = int(time.Since(stats.LastQueried).Hours() / 24)
			wasted.Recommendation = "Review retention - rarely queried"
			rarelyQueried = append(rarelyQueried, wasted)
		}
	}

	// Sort by cost
	sortWastedByCost(neverQueried)
	sortWastedByCost(rarelyQueried)

	return neverQueried, rarelyQueried
}

// generateRecommendations creates actionable recommendations
func (t *Tracker) generateRecommendations(report *UsageReport) []UsageRecommendation {
	var recs []UsageRecommendation

	// High wasted data cost
	if report.WastedDataCost > 100 {
		recs = append(recs, UsageRecommendation{
			Type:        "reduce_waste",
			Severity:    "critical",
			Description: "Significant unused data detected",
			Impact:      formatCurrency(report.WastedDataCost) + "/month in unused data storage",
			Savings:     report.WastedDataCost,
		})
	}

	// Many never-queried items
	if len(report.NeverQueried) > 50 {
		recs = append(recs, UsageRecommendation{
			Type:        "cleanup",
			Severity:    "warning",
			Description: formatInt(len(report.NeverQueried)) + " data items have never been queried",
			Impact:      "Review and drop unused metrics/logs to reduce costs",
		})
	}

	// Identify specific high-cost unused metrics
	for _, w := range report.NeverQueried {
		if w.EstimatedCost > 10 {
			recs = append(recs, UsageRecommendation{
				Type:        "drop_metric",
				Severity:    "warning",
				DataType:    w.Inventory.DataType,
				Name:        w.Inventory.Name,
				Description: "High-cost unused data: " + w.Inventory.Name,
				Impact:      "Never queried, costing " + formatCurrency(w.EstimatedCost) + "/month",
				Savings:     w.EstimatedCost,
			})
		}
	}

	// Query pattern insights
	if len(t.queryByHour) > 0 {
		// Find peak hours
		var maxHour int
		var maxCount int64
		for hour, count := range t.queryByHour {
			if count > maxCount {
				maxCount = count
				maxHour = hour
			}
		}
		recs = append(recs, UsageRecommendation{
			Type:        "insight",
			Severity:    "info",
			Description: "Peak query hour: " + formatInt(maxHour) + ":00 with " + formatInt64(maxCount) + " queries",
			Impact:      "Consider scheduling batch jobs outside peak hours",
		})
	}

	// Low utilization warning
	if report.UniqueDataItems > 0 {
		queriedRatio := float64(len(report.TopQueried)) / float64(report.UniqueDataItems)
		if queriedRatio < 0.2 {
			recs = append(recs, UsageRecommendation{
				Type:        "low_utilization",
				Severity:    "warning",
				Description: "Only " + formatPercent(queriedRatio*100) + " of data items are actively queried",
				Impact:      "Review data collection strategy - most data may be unnecessary",
			})
		}
	}

	return recs
}

// SetCostPerGB sets the cost estimation rate
func (t *Tracker) SetCostPerGB(cost float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.costPerGBMonth = cost
}

// Stats returns basic statistics
func (t *Tracker) Stats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return map[string]interface{}{
		"tracked_items":    len(t.stats),
		"inventory_items":  len(t.inventory),
		"total_queries":    sumMapInt64(t.queryByType),
		"query_by_type":    t.queryByType,
		"cost_per_gb":      t.costPerGBMonth,
	}
}

// Helper functions

func sortStatsByQueryCount(stats []UsageStats) {
	for i := 0; i < len(stats)-1; i++ {
		for j := i + 1; j < len(stats); j++ {
			if stats[j].QueryCount > stats[i].QueryCount {
				stats[i], stats[j] = stats[j], stats[i]
			}
		}
	}
}

func sortWastedByCost(wasted []WastedData) {
	for i := 0; i < len(wasted)-1; i++ {
		for j := i + 1; j < len(wasted); j++ {
			if wasted[j].EstimatedCost > wasted[i].EstimatedCost {
				wasted[i], wasted[j] = wasted[j], wasted[i]
			}
		}
	}
}

func sumMapInt64(m map[DataType]int64) int64 {
	var sum int64
	for _, v := range m {
		sum += v
	}
	return sum
}

func formatCurrency(amount float64) string {
	if amount >= 1000 {
		return "$" + formatFloat(amount/1000, 1) + "k"
	}
	return "$" + formatFloat(amount, 2)
}

func formatPercent(pct float64) string {
	return formatFloat(pct, 1) + "%"
}

func formatInt(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

func formatInt64(i int64) string {
	return formatInt(int(i))
}

func formatFloat(f float64, decimals int) string {
	if decimals <= 0 {
		return formatInt(int(f))
	}

	intPart := int(f)
	fracPart := f - float64(intPart)

	for i := 0; i < decimals; i++ {
		fracPart *= 10
	}

	fracInt := int(fracPart + 0.5)

	result := formatInt(intPart) + "."
	fracStr := formatInt(fracInt)
	for len(fracStr) < decimals {
		fracStr = "0" + fracStr
	}
	return result + fracStr
}
