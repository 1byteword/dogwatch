package cardinality

import (
	"sort"
	"sync"
	"time"
)

// MetricCardinality represents cardinality info for a metric
type MetricCardinality struct {
	Name              string            `json:"name"`
	TotalSeries       int               `json:"total_series"`
	UniqueTagKeys     int               `json:"unique_tag_keys"`
	TagCardinality    map[string]int    `json:"tag_cardinality"`
	TopTagValues      map[string][]string `json:"top_tag_values,omitempty"`
	EstimatedCost     float64           `json:"estimated_cost_monthly"`
	Recommendation    string            `json:"recommendation,omitempty"`
	SampleTags        []map[string]string `json:"sample_tags,omitempty"`
	LastSeen          time.Time         `json:"last_seen"`
}

// CardinalityReport provides overall cardinality analysis
type CardinalityReport struct {
	TotalMetrics       int                  `json:"total_metrics"`
	TotalSeries        int                  `json:"total_series"`
	TotalTagKeys       int                  `json:"total_tag_keys"`
	HighCardinalityCount int               `json:"high_cardinality_count"`
	EstimatedMonthlyCost float64           `json:"estimated_monthly_cost"`
	TopMetrics         []MetricCardinality  `json:"top_metrics"`
	TopTags            []TagCardinality     `json:"top_tags"`
	Recommendations    []Recommendation     `json:"recommendations"`
	AnalyzedAt         time.Time            `json:"analyzed_at"`
}

// TagCardinality represents cardinality for a specific tag key
type TagCardinality struct {
	Key           string   `json:"key"`
	UniqueValues  int      `json:"unique_values"`
	MetricsUsing  int      `json:"metrics_using"`
	TopValues     []string `json:"top_values,omitempty"`
	IsHighCardinality bool `json:"is_high_cardinality"`
}

// Recommendation suggests how to reduce cardinality
type Recommendation struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"` // critical, warning, info
	Metric      string  `json:"metric,omitempty"`
	Tag         string  `json:"tag,omitempty"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
	Savings     float64 `json:"estimated_savings,omitempty"`
}

// SeriesInfo represents a unique time series
type SeriesInfo struct {
	Name     string
	Tags     map[string]string
	LastSeen time.Time
}

// Explorer analyzes metric cardinality
type Explorer struct {
	mu sync.RWMutex

	// Track unique series
	series map[string]*SeriesInfo // key = metric_name + sorted_tags

	// Track tag values per metric
	metricTags map[string]map[string]map[string]struct{} // metric -> tag_key -> tag_values

	// Global tag tracking
	globalTags map[string]map[string]struct{} // tag_key -> tag_values

	// Thresholds
	highCardinalityThreshold int
	costPerSeries           float64
}

// NewExplorer creates a new cardinality explorer
func NewExplorer() *Explorer {
	return &Explorer{
		series:                   make(map[string]*SeriesInfo),
		metricTags:              make(map[string]map[string]map[string]struct{}),
		globalTags:              make(map[string]map[string]struct{}),
		highCardinalityThreshold: 1000,
		costPerSeries:           0.05, // $0.05 per series per month (Datadog-like pricing)
	}
}

// RecordSeries records a metric series for cardinality tracking
func (e *Explorer) RecordSeries(name string, tags map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Generate series key
	key := e.seriesKey(name, tags)

	// Update or create series
	if existing, ok := e.series[key]; ok {
		existing.LastSeen = time.Now()
	} else {
		e.series[key] = &SeriesInfo{
			Name:     name,
			Tags:     copyTags(tags),
			LastSeen: time.Now(),
		}
	}

	// Track tag cardinality per metric
	if _, ok := e.metricTags[name]; !ok {
		e.metricTags[name] = make(map[string]map[string]struct{})
	}

	for k, v := range tags {
		if _, ok := e.metricTags[name][k]; !ok {
			e.metricTags[name][k] = make(map[string]struct{})
		}
		e.metricTags[name][k][v] = struct{}{}

		// Global tag tracking
		if _, ok := e.globalTags[k]; !ok {
			e.globalTags[k] = make(map[string]struct{})
		}
		e.globalTags[k][v] = struct{}{}
	}
}

// GetReport generates a full cardinality report
func (e *Explorer) GetReport(limit int) *CardinalityReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	report := &CardinalityReport{
		TotalMetrics: len(e.metricTags),
		TotalSeries:  len(e.series),
		AnalyzedAt:   time.Now(),
	}

	// Calculate total unique tag keys
	report.TotalTagKeys = len(e.globalTags)

	// Analyze each metric
	var metrics []MetricCardinality
	for name, tagMap := range e.metricTags {
		mc := e.analyzeMetric(name, tagMap)
		metrics = append(metrics, mc)

		if mc.TotalSeries >= e.highCardinalityThreshold {
			report.HighCardinalityCount++
		}
		report.EstimatedMonthlyCost += mc.EstimatedCost
	}

	// Sort by total series (highest first)
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].TotalSeries > metrics[j].TotalSeries
	})

	// Take top N
	if len(metrics) > limit {
		report.TopMetrics = metrics[:limit]
	} else {
		report.TopMetrics = metrics
	}

	// Analyze global tags
	report.TopTags = e.analyzeGlobalTags(limit)

	// Generate recommendations
	report.Recommendations = e.generateRecommendations(metrics)

	return report
}

// GetMetricCardinality returns cardinality for a specific metric
func (e *Explorer) GetMetricCardinality(name string) *MetricCardinality {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tagMap, ok := e.metricTags[name]
	if !ok {
		return nil
	}

	mc := e.analyzeMetric(name, tagMap)

	// Add sample tags
	mc.SampleTags = e.getSampleTags(name, 5)

	return &mc
}

// GetTagCardinality returns cardinality for a specific tag across all metrics
func (e *Explorer) GetTagCardinality(tagKey string) *TagCardinality {
	e.mu.RLock()
	defer e.mu.RUnlock()

	values, ok := e.globalTags[tagKey]
	if !ok {
		return nil
	}

	tc := &TagCardinality{
		Key:          tagKey,
		UniqueValues: len(values),
	}

	// Count metrics using this tag
	for _, tagMap := range e.metricTags {
		if _, ok := tagMap[tagKey]; ok {
			tc.MetricsUsing++
		}
	}

	// Get top values
	tc.TopValues = e.getTopTagValues(tagKey, 10)
	tc.IsHighCardinality = tc.UniqueValues >= e.highCardinalityThreshold

	return tc
}

// GetHighCardinalityMetrics returns metrics exceeding the threshold
func (e *Explorer) GetHighCardinalityMetrics(threshold int) []MetricCardinality {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if threshold <= 0 {
		threshold = e.highCardinalityThreshold
	}

	var result []MetricCardinality
	for name, tagMap := range e.metricTags {
		mc := e.analyzeMetric(name, tagMap)
		if mc.TotalSeries >= threshold {
			result = append(result, mc)
		}
	}

	// Sort by cardinality
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalSeries > result[j].TotalSeries
	})

	return result
}

// Cleanup removes stale series older than maxAge
func (e *Explorer) Cleanup(maxAge time.Duration) int {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for key, info := range e.series {
		if info.LastSeen.Before(cutoff) {
			delete(e.series, key)
			removed++
		}
	}

	return removed
}

// analyzeMetric calculates cardinality for a single metric
func (e *Explorer) analyzeMetric(name string, tagMap map[string]map[string]struct{}) MetricCardinality {
	mc := MetricCardinality{
		Name:           name,
		UniqueTagKeys:  len(tagMap),
		TagCardinality: make(map[string]int),
		TopTagValues:   make(map[string][]string),
		LastSeen:       time.Now(),
	}

	// Count series for this metric
	for key, info := range e.series {
		if info.Name == name {
			mc.TotalSeries++
			if info.LastSeen.After(mc.LastSeen) {
				mc.LastSeen = info.LastSeen
			}
		}
		_ = key
	}

	// Calculate per-tag cardinality
	maxCardinality := 0
	var highCardTag string

	for tagKey, values := range tagMap {
		cardinality := len(values)
		mc.TagCardinality[tagKey] = cardinality

		if cardinality > maxCardinality {
			maxCardinality = cardinality
			highCardTag = tagKey
		}

		// Get top values for high-cardinality tags
		if cardinality > 10 {
			mc.TopTagValues[tagKey] = e.getTopTagValuesForMetric(name, tagKey, 5)
		}
	}

	// Estimate cost
	mc.EstimatedCost = float64(mc.TotalSeries) * e.costPerSeries

	// Generate recommendation
	if mc.TotalSeries >= e.highCardinalityThreshold {
		if highCardTag != "" {
			mc.Recommendation = "Consider removing or aggregating tag '" + highCardTag + "' which has " +
				itoa(maxCardinality) + " unique values"
		} else {
			mc.Recommendation = "High cardinality metric - review tag combinations"
		}
	}

	return mc
}

// analyzeGlobalTags returns cardinality info for all tags
func (e *Explorer) analyzeGlobalTags(limit int) []TagCardinality {
	var tags []TagCardinality

	for tagKey, values := range e.globalTags {
		tc := TagCardinality{
			Key:          tagKey,
			UniqueValues: len(values),
		}

		// Count metrics using this tag
		for _, tagMap := range e.metricTags {
			if _, ok := tagMap[tagKey]; ok {
				tc.MetricsUsing++
			}
		}

		tc.IsHighCardinality = tc.UniqueValues >= e.highCardinalityThreshold
		tc.TopValues = e.getTopTagValues(tagKey, 5)
		tags = append(tags, tc)
	}

	// Sort by unique values
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].UniqueValues > tags[j].UniqueValues
	})

	if len(tags) > limit {
		return tags[:limit]
	}
	return tags
}

// generateRecommendations creates actionable recommendations
func (e *Explorer) generateRecommendations(metrics []MetricCardinality) []Recommendation {
	var recs []Recommendation

	// Check for extremely high cardinality metrics
	for _, mc := range metrics {
		if mc.TotalSeries >= 10000 {
			recs = append(recs, Recommendation{
				Type:        "drop_metric",
				Severity:    "critical",
				Metric:      mc.Name,
				Description: "Metric has extremely high cardinality (" + itoa(mc.TotalSeries) + " series)",
				Impact:      "Consider dropping this metric or reducing tag dimensions",
				Savings:     mc.EstimatedCost * 0.9, // 90% savings if addressed
			})
		} else if mc.TotalSeries >= 5000 {
			recs = append(recs, Recommendation{
				Type:        "reduce_cardinality",
				Severity:    "warning",
				Metric:      mc.Name,
				Description: "Metric has high cardinality (" + itoa(mc.TotalSeries) + " series)",
				Impact:      "Review and consolidate high-cardinality tags",
				Savings:     mc.EstimatedCost * 0.5,
			})
		}

		// Check for problematic tags
		for tag, cardinality := range mc.TagCardinality {
			if cardinality >= 1000 && isProblematicTag(tag) {
				recs = append(recs, Recommendation{
					Type:        "remove_tag",
					Severity:    "warning",
					Metric:      mc.Name,
					Tag:         tag,
					Description: "Tag '" + tag + "' appears to contain unique identifiers (" + itoa(cardinality) + " values)",
					Impact:      "Remove or hash this tag to reduce cardinality",
					Savings:     float64(cardinality) * e.costPerSeries * 0.8,
				})
			}
		}
	}

	// Check global tags
	for tagKey, values := range e.globalTags {
		if len(values) >= 5000 && isProblematicTag(tagKey) {
			recs = append(recs, Recommendation{
				Type:        "global_tag_issue",
				Severity:    "critical",
				Tag:         tagKey,
				Description: "Global tag '" + tagKey + "' has " + itoa(len(values)) + " unique values across all metrics",
				Impact:      "This tag is likely causing cardinality explosion across multiple metrics",
			})
		}
	}

	// Sort by severity
	severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.Slice(recs, func(i, j int) bool {
		return severityOrder[recs[i].Severity] < severityOrder[recs[j].Severity]
	})

	return recs
}

// getSampleTags returns sample tag combinations for a metric
func (e *Explorer) getSampleTags(name string, limit int) []map[string]string {
	var samples []map[string]string
	count := 0

	for _, info := range e.series {
		if info.Name == name {
			samples = append(samples, copyTags(info.Tags))
			count++
			if count >= limit {
				break
			}
		}
	}

	return samples
}

// getTopTagValues returns most common values for a tag
func (e *Explorer) getTopTagValues(tagKey string, limit int) []string {
	values, ok := e.globalTags[tagKey]
	if !ok {
		return nil
	}

	var result []string
	for v := range values {
		result = append(result, v)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// getTopTagValuesForMetric returns top values for a tag within a metric
func (e *Explorer) getTopTagValuesForMetric(metric, tagKey string, limit int) []string {
	tagMap, ok := e.metricTags[metric]
	if !ok {
		return nil
	}

	values, ok := tagMap[tagKey]
	if !ok {
		return nil
	}

	var result []string
	for v := range values {
		result = append(result, v)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// seriesKey generates a unique key for a series
func (e *Explorer) seriesKey(name string, tags map[string]string) string {
	// Sort tag keys for consistent ordering
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	key := name
	for _, k := range keys {
		key += "|" + k + "=" + tags[k]
	}
	return key
}

// SetThresholds configures cardinality thresholds
func (e *Explorer) SetThresholds(highCardinality int, costPerSeries float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if highCardinality > 0 {
		e.highCardinalityThreshold = highCardinality
	}
	if costPerSeries > 0 {
		e.costPerSeries = costPerSeries
	}
}

// Stats returns basic statistics
func (e *Explorer) Stats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]interface{}{
		"total_metrics":     len(e.metricTags),
		"total_series":      len(e.series),
		"total_tag_keys":    len(e.globalTags),
		"threshold":         e.highCardinalityThreshold,
		"cost_per_series":   e.costPerSeries,
	}
}

// Helper functions

func copyTags(tags map[string]string) map[string]string {
	result := make(map[string]string, len(tags))
	for k, v := range tags {
		result[k] = v
	}
	return result
}

func isProblematicTag(tag string) bool {
	problematic := []string{
		"request_id", "trace_id", "span_id", "correlation_id",
		"uuid", "guid", "id", "session_id", "user_id",
		"timestamp", "time", "ts",
		"ip", "ip_address", "client_ip",
		"url", "uri", "path", "query",
	}

	tagLower := toLower(tag)
	for _, p := range problematic {
		if tagLower == p || contains(tagLower, p) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}

	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		b[pos] = '-'
	}

	return string(b[pos:])
}
