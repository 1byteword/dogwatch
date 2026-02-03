package ingest

import (
	"dogwatch/internal/custommetrics"
)

// CustomMetricsAdapter adapts custommetrics.Store to implement MetricStore
type CustomMetricsAdapter struct {
	store *custommetrics.Store
}

// NewCustomMetricsAdapter creates a new adapter wrapping a custommetrics.Store
func NewCustomMetricsAdapter(store *custommetrics.Store) *CustomMetricsAdapter {
	return &CustomMetricsAdapter{store: store}
}

// WriteSamples converts ingest.Sample to custommetrics.DataPoint and stores them
func (a *CustomMetricsAdapter) WriteSamples(samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}

	points := make([]custommetrics.DataPoint, len(samples))
	for i, s := range samples {
		// Determine metric type from tags if present
		metricType := custommetrics.Gauge // Default to gauge
		if typeTag, ok := s.Tags["__dd_type"]; ok {
			switch typeTag {
			case "count":
				metricType = custommetrics.Counter
			case "rate":
				metricType = custommetrics.Counter
			case "histogram", "distribution":
				metricType = custommetrics.Histogram
			}
			// Remove internal tag
			delete(s.Tags, "__dd_type")
		}

		points[i] = custommetrics.DataPoint{
			Timestamp: s.Timestamp,
			Name:      s.Metric,
			Type:      metricType,
			Value:     s.Value,
			Tags:      s.Tags,
		}
	}

	return a.store.RecordBatch(points)
}
