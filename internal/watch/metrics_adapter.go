package watch

import (
	"dogwatch/internal/aggregator"
	"dogwatch/internal/metrics"
)

// MetricsAdapter bridges the metrics collector and aggregator to the watch engine
type MetricsAdapter struct {
	collector  *metrics.Collector
	aggregator *aggregator.Aggregator
}

// NewMetricsAdapter creates a new metrics adapter
func NewMetricsAdapter(collector *metrics.Collector, agg *aggregator.Aggregator) *MetricsAdapter {
	return &MetricsAdapter{
		collector:  collector,
		aggregator: agg,
	}
}

// GetMetricValue returns the current value for a metric type
func (m *MetricsAdapter) GetMetricValue(metric MetricType) (float64, bool) {
	switch metric {
	// System metrics from collector
	case MetricCPU:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return sys.CPUUsagePercent, true

	case MetricMemory:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return sys.MemUsagePercent, true

	case MetricDiskRead:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return float64(sys.DiskReadPerSec), true

	case MetricDiskWrite:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return float64(sys.DiskWritePerSec), true

	case MetricNetRx:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return float64(sys.NetRxPerSec), true

	case MetricNetTx:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return float64(sys.NetTxPerSec), true

	case MetricLoad:
		if m.collector == nil {
			return 0, false
		}
		sys := m.collector.Collect()
		return sys.Load1, true

	// Aggregator metrics
	case MetricConnections:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		return float64(stats.TotalConnections), true

	case MetricRequests:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		return float64(stats.TotalRequests), true

	case MetricErrors:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		return float64(stats.TotalErrors), true

	case MetricErrorRate:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		if stats.TotalRequests == 0 {
			return 0, true
		}
		return float64(stats.TotalErrors) / float64(stats.TotalRequests) * 100, true

	case MetricLatencyP50:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		// Get average P50 across all endpoints
		var total float64
		for _, ep := range stats.Endpoints {
			total += float64(ep.P50.Milliseconds())
		}
		if len(stats.Endpoints) == 0 {
			return 0, true
		}
		return total / float64(len(stats.Endpoints)), true

	case MetricLatencyP99:
		if m.aggregator == nil {
			return 0, false
		}
		stats := m.aggregator.GetStats()
		// Get max P99 across all endpoints (worst case)
		var max float64
		for _, ep := range stats.Endpoints {
			if ms := float64(ep.P99.Milliseconds()); ms > max {
				max = ms
			}
		}
		return max, true

	default:
		return 0, false
	}
}
