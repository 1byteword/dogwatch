package costintel

import (
	"time"

	"dogwatch/internal/cardinality"
	"dogwatch/internal/custommetrics"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"
	"dogwatch/internal/usage"
)

// DataProvider implements UsageDataProvider by connecting to actual stores
type DataProvider struct {
	traceStore      *trace.Store
	logStore        *logs.Store
	metricsStore    *custommetrics.Store
	cardinalityExp  *cardinality.Explorer
	usageTracker    *usage.Tracker
}

// NewDataProvider creates a new data provider
func NewDataProvider(
	traceStore *trace.Store,
	logStore *logs.Store,
	metricsStore *custommetrics.Store,
	cardinalityExp *cardinality.Explorer,
	usageTracker *usage.Tracker,
) *DataProvider {
	return &DataProvider{
		traceStore:     traceStore,
		logStore:       logStore,
		metricsStore:   metricsStore,
		cardinalityExp: cardinalityExp,
		usageTracker:   usageTracker,
	}
}

// GetUnusedMetrics returns metrics not queried in the given duration
func (p *DataProvider) GetUnusedMetrics(since time.Duration) []UnusedMetric {
	var unused []UnusedMetric

	if p.metricsStore == nil || p.usageTracker == nil {
		return unused
	}

	// Get usage report to find wasted data
	report := p.usageTracker.GetReport(since)

	// NeverQueried contains data that exists but was never queried
	for _, wasted := range report.NeverQueried {
		if wasted.Inventory.DataType == usage.DataTypeMetric {
			unused = append(unused, UnusedMetric{
				Name:        wasted.Inventory.Name,
				LastQueried: time.Time{}, // Never queried
				DataPoints:  wasted.Inventory.DataPoints,
				BytesStored: wasted.Inventory.SizeBytes,
				MonthlyCost: wasted.EstimatedCost,
			})
		}
	}

	// RarelyQueried contains data queried very infrequently
	for _, wasted := range report.RarelyQueried {
		if wasted.Inventory.DataType == usage.DataTypeMetric {
			lastQueried := wasted.Inventory.LastSeen
			if wasted.DaysSinceQuery > 0 {
				lastQueried = time.Now().AddDate(0, 0, -wasted.DaysSinceQuery)
			}

			unused = append(unused, UnusedMetric{
				Name:        wasted.Inventory.Name,
				LastQueried: lastQueried,
				DataPoints:  wasted.Inventory.DataPoints,
				BytesStored: wasted.Inventory.SizeBytes,
				MonthlyCost: wasted.EstimatedCost,
			})
		}
	}

	return unused
}

// GetHighVolumeServices returns services with high data volume
func (p *DataProvider) GetHighVolumeServices(threshold int64) []HighVolumeService {
	var highVolume []HighVolumeService

	// Check traces
	if p.traceStore != nil {
		services, err := p.traceStore.GetServices()
		if err == nil {
			for _, svc := range services {
				// Get trace count for this service
				traces, err := p.traceStore.ListTraces(10000, svc, 24*time.Hour)
				if err != nil {
					continue
				}

				// Estimate bytes (avg 5KB per span, 5 spans per trace)
				bytesPerDay := int64(len(traces)) * 5 * 5000

				if bytesPerDay >= threshold {
					// $1.70 per million spans
					monthlyCost := float64(len(traces)*5*30) / 1000000 * 1.70

					highVolume = append(highVolume, HighVolumeService{
						Service:      svc,
						EventsPerDay: int64(len(traces) * 5),
						BytesPerDay:  bytesPerDay,
						MonthlyCost:  monthlyCost,
						DataType:     "traces",
					})
				}
			}
		}
	}

	// Check logs
	if p.logStore != nil {
		end := time.Now()
		start := end.Add(-24 * time.Hour)

		// Get log services by querying with empty filter
		query := logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Limit:     1000,
		}

		results, err := p.logStore.Search(query)
		if err == nil && results != nil {
			// Aggregate by service
			serviceBytes := make(map[string]int64)
			serviceCounts := make(map[string]int64)

			for _, entry := range results.Entries {
				svc := entry.Service
				if svc == "" {
					svc = "unknown"
				}
				serviceCounts[svc]++
				// Estimate 500 bytes per log entry
				serviceBytes[svc] += 500
			}

			for svc, bytes := range serviceBytes {
				if bytes >= threshold {
					// $0.10 per GB
					monthlyCost := float64(bytes*30) / (1024 * 1024 * 1024) * 0.10

					highVolume = append(highVolume, HighVolumeService{
						Service:      svc,
						EventsPerDay: serviceCounts[svc],
						BytesPerDay:  bytes,
						MonthlyCost:  monthlyCost,
						DataType:     "logs",
					})
				}
			}
		}
	}

	return highVolume
}

// GetHighCardinalityMetrics returns metrics with high cardinality
func (p *DataProvider) GetHighCardinalityMetrics(threshold int) []HighCardinalityMetric {
	var highCardinality []HighCardinalityMetric

	if p.cardinalityExp == nil {
		return highCardinality
	}

	report := p.cardinalityExp.GetReport(100) // Get top 100 metrics

	for _, metric := range report.TopMetrics {
		if metric.TotalSeries >= threshold {
			// $0.05 per series per month
			monthlyCost := float64(metric.TotalSeries) * 0.05

			// Find problematic tags (those with high cardinality)
			var problematicTags []string
			for tagKey, cardinality := range metric.TagCardinality {
				if cardinality > 100 { // More than 100 unique values is problematic
					problematicTags = append(problematicTags, tagKey)
				}
			}

			// Common problematic tag patterns
			commonProblematic := []string{"request_id", "trace_id", "span_id", "correlation_id", "uuid", "session_id"}
			for tagKey := range metric.TagCardinality {
				for _, common := range commonProblematic {
					if tagKey == common {
						found := false
						for _, existing := range problematicTags {
							if existing == tagKey {
								found = true
								break
							}
						}
						if !found {
							problematicTags = append(problematicTags, tagKey)
						}
					}
				}
			}

			highCardinality = append(highCardinality, HighCardinalityMetric{
				Name:              metric.Name,
				UniqueSeriesCount: metric.TotalSeries,
				ProblematicTags:   problematicTags,
				MonthlyCost:       monthlyCost,
			})
		}
	}

	return highCardinality
}

// GetLogLevelDistribution returns log counts by level
func (p *DataProvider) GetLogLevelDistribution() map[string]int64 {
	distribution := make(map[string]int64)

	if p.logStore == nil {
		return distribution
	}

	end := time.Now()
	start := end.Add(-24 * time.Hour)

	// Query logs and aggregate by level
	levels := []logs.LogLevel{logs.LevelDebug, logs.LevelInfo, logs.LevelWarn, logs.LevelError, logs.LevelFatal}
	for _, level := range levels {
		query := logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Level:     level,
			Limit:     1,
		}

		results, err := p.logStore.Search(query)
		if err == nil && results != nil {
			distribution[string(level)] = int64(results.TotalCount)
		}
	}

	return distribution
}

// GetQueryPatterns returns what data is actually queried
func (p *DataProvider) GetQueryPatterns() []QueryPattern {
	var patterns []QueryPattern

	if p.usageTracker == nil {
		return patterns
	}

	report := p.usageTracker.GetReport(24 * time.Hour)

	// Aggregate by data type from QueryByType
	for dataType, count := range report.QueryByType {
		patterns = append(patterns, QueryPattern{
			DataType:    string(dataType),
			Pattern:     "*",
			QueryCount:  count,
			LastQueried: report.EndTime,
		})
	}

	return patterns
}

// SimpleDataProvider provides mock data for testing when stores aren't available
type SimpleDataProvider struct {
	unusedMetrics      []UnusedMetric
	highVolumeServices []HighVolumeService
	highCardinality    []HighCardinalityMetric
	logDistribution    map[string]int64
	queryPatterns      []QueryPattern
}

// NewSimpleDataProvider creates a simple provider with mock data
func NewSimpleDataProvider() *SimpleDataProvider {
	return &SimpleDataProvider{
		logDistribution: make(map[string]int64),
	}
}

func (p *SimpleDataProvider) GetUnusedMetrics(since time.Duration) []UnusedMetric {
	return p.unusedMetrics
}

func (p *SimpleDataProvider) GetHighVolumeServices(threshold int64) []HighVolumeService {
	return p.highVolumeServices
}

func (p *SimpleDataProvider) GetHighCardinalityMetrics(threshold int) []HighCardinalityMetric {
	return p.highCardinality
}

func (p *SimpleDataProvider) GetLogLevelDistribution() map[string]int64 {
	return p.logDistribution
}

func (p *SimpleDataProvider) GetQueryPatterns() []QueryPattern {
	return p.queryPatterns
}

// SetUnusedMetrics sets mock unused metrics
func (p *SimpleDataProvider) SetUnusedMetrics(metrics []UnusedMetric) {
	p.unusedMetrics = metrics
}

// SetHighVolumeServices sets mock high volume services
func (p *SimpleDataProvider) SetHighVolumeServices(services []HighVolumeService) {
	p.highVolumeServices = services
}

// SetHighCardinalityMetrics sets mock high cardinality metrics
func (p *SimpleDataProvider) SetHighCardinalityMetrics(metrics []HighCardinalityMetric) {
	p.highCardinality = metrics
}

// SetLogDistribution sets mock log distribution
func (p *SimpleDataProvider) SetLogDistribution(dist map[string]int64) {
	p.logDistribution = dist
}

// SetQueryPatterns sets mock query patterns
func (p *SimpleDataProvider) SetQueryPatterns(patterns []QueryPattern) {
	p.queryPatterns = patterns
}
