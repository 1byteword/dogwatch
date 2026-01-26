package otlp

import (
	"time"

	"dogwatch/internal/custommetrics"

	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

// processMetrics converts OTLP resource metrics and stores them
func processMetrics(metricsStore *custommetrics.Store, resourceMetrics []*metricspb.ResourceMetrics) error {
	if metricsStore == nil {
		return nil
	}

	var dataPoints []custommetrics.DataPoint

	for _, rm := range resourceMetrics {
		resourceTags := extractResourceTags(rm.Resource)

		for _, scopeMetrics := range rm.ScopeMetrics {
			for _, metric := range scopeMetrics.Metrics {
				points := convertMetric(metric, resourceTags)
				dataPoints = append(dataPoints, points...)
			}
		}
	}

	if len(dataPoints) > 0 {
		return metricsStore.RecordBatch(dataPoints)
	}
	return nil
}

// extractResourceTags extracts tags from resource attributes
func extractResourceTags(resource *resourcepb.Resource) map[string]string {
	if resource == nil {
		return nil
	}
	tags := make(map[string]string)
	for _, attr := range resource.Attributes {
		tags[attr.Key] = anyValueToString(attr.Value)
	}
	return tags
}

// convertMetric converts an OTLP Metric to internal DataPoints
func convertMetric(metric *metricspb.Metric, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint

	switch data := metric.Data.(type) {
	case *metricspb.Metric_Sum:
		points = convertSum(metric.Name, data.Sum, resourceTags)
	case *metricspb.Metric_Gauge:
		points = convertGauge(metric.Name, data.Gauge, resourceTags)
	case *metricspb.Metric_Histogram:
		points = convertHistogram(metric.Name, data.Histogram, resourceTags)
	case *metricspb.Metric_Summary:
		points = convertSummary(metric.Name, data.Summary, resourceTags)
	case *metricspb.Metric_ExponentialHistogram:
		points = convertExponentialHistogram(metric.Name, data.ExponentialHistogram, resourceTags)
	}

	return points
}

// convertSum converts Sum metric data
func convertSum(name string, sum *metricspb.Sum, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint

	metricType := custommetrics.Counter
	if !sum.IsMonotonic {
		metricType = custommetrics.Gauge
	}

	for _, dp := range sum.DataPoints {
		point := convertNumberDataPoint(name, dp, metricType, resourceTags)
		points = append(points, point)
	}
	return points
}

// convertGauge converts Gauge metric data
func convertGauge(name string, gauge *metricspb.Gauge, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint
	for _, dp := range gauge.DataPoints {
		point := convertNumberDataPoint(name, dp, custommetrics.Gauge, resourceTags)
		points = append(points, point)
	}
	return points
}

// convertHistogram converts Histogram metric data to multiple gauge points
func convertHistogram(name string, hist *metricspb.Histogram, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint

	for _, dp := range hist.DataPoints {
		ts := time.Unix(0, int64(dp.TimeUnixNano))
		tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

		// Store count
		countTags := copyTags(tags)
		countTags["aggregate"] = "count"
		points = append(points, custommetrics.DataPoint{
			Timestamp: ts,
			Name:      name,
			Type:      custommetrics.Counter,
			Value:     float64(dp.Count),
			Tags:      countTags,
		})

		// Store sum if available
		if dp.Sum != nil {
			sumTags := copyTags(tags)
			sumTags["aggregate"] = "sum"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Counter,
				Value:     *dp.Sum,
				Tags:      sumTags,
			})
		}

		// Store min/max if available
		if dp.Min != nil {
			minTags := copyTags(tags)
			minTags["aggregate"] = "min"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Gauge,
				Value:     *dp.Min,
				Tags:      minTags,
			})
		}
		if dp.Max != nil {
			maxTags := copyTags(tags)
			maxTags["aggregate"] = "max"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Gauge,
				Value:     *dp.Max,
				Tags:      maxTags,
			})
		}

		// Store bucket counts
		for i, count := range dp.BucketCounts {
			bucketTags := copyTags(tags)
			bucketTags["aggregate"] = "bucket"
			if i < len(dp.ExplicitBounds) {
				bucketTags["le"] = formatFloat(dp.ExplicitBounds[i])
			} else {
				bucketTags["le"] = "+Inf"
			}
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name + "_bucket",
				Type:      custommetrics.Counter,
				Value:     float64(count),
				Tags:      bucketTags,
			})
		}
	}
	return points
}

// convertSummary converts Summary metric data
func convertSummary(name string, summary *metricspb.Summary, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint

	for _, dp := range summary.DataPoints {
		ts := time.Unix(0, int64(dp.TimeUnixNano))
		tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

		// Store count
		countTags := copyTags(tags)
		countTags["aggregate"] = "count"
		points = append(points, custommetrics.DataPoint{
			Timestamp: ts,
			Name:      name,
			Type:      custommetrics.Counter,
			Value:     float64(dp.Count),
			Tags:      countTags,
		})

		// Store sum
		sumTags := copyTags(tags)
		sumTags["aggregate"] = "sum"
		points = append(points, custommetrics.DataPoint{
			Timestamp: ts,
			Name:      name,
			Type:      custommetrics.Counter,
			Value:     dp.Sum,
			Tags:      sumTags,
		})

		// Store quantiles
		for _, q := range dp.QuantileValues {
			qTags := copyTags(tags)
			qTags["quantile"] = formatFloat(q.Quantile)
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Gauge,
				Value:     q.Value,
				Tags:      qTags,
			})
		}
	}
	return points
}

// convertExponentialHistogram converts ExponentialHistogram metric data
func convertExponentialHistogram(name string, hist *metricspb.ExponentialHistogram, resourceTags map[string]string) []custommetrics.DataPoint {
	var points []custommetrics.DataPoint

	for _, dp := range hist.DataPoints {
		ts := time.Unix(0, int64(dp.TimeUnixNano))
		tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

		// Store count
		countTags := copyTags(tags)
		countTags["aggregate"] = "count"
		points = append(points, custommetrics.DataPoint{
			Timestamp: ts,
			Name:      name,
			Type:      custommetrics.Counter,
			Value:     float64(dp.Count),
			Tags:      countTags,
		})

		// Store sum if available
		if dp.Sum != nil {
			sumTags := copyTags(tags)
			sumTags["aggregate"] = "sum"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Counter,
				Value:     *dp.Sum,
				Tags:      sumTags,
			})
		}

		// Store min/max if available
		if dp.Min != nil {
			minTags := copyTags(tags)
			minTags["aggregate"] = "min"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Gauge,
				Value:     *dp.Min,
				Tags:      minTags,
			})
		}
		if dp.Max != nil {
			maxTags := copyTags(tags)
			maxTags["aggregate"] = "max"
			points = append(points, custommetrics.DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      custommetrics.Gauge,
				Value:     *dp.Max,
				Tags:      maxTags,
			})
		}
	}
	return points
}

// convertNumberDataPoint converts a NumberDataPoint to internal DataPoint
func convertNumberDataPoint(name string, dp *metricspb.NumberDataPoint, metricType custommetrics.MetricType, resourceTags map[string]string) custommetrics.DataPoint {
	ts := time.Unix(0, int64(dp.TimeUnixNano))
	tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

	var value float64
	switch v := dp.Value.(type) {
	case *metricspb.NumberDataPoint_AsDouble:
		value = v.AsDouble
	case *metricspb.NumberDataPoint_AsInt:
		value = float64(v.AsInt)
	}

	return custommetrics.DataPoint{
		Timestamp: ts,
		Name:      name,
		Type:      metricType,
		Value:     value,
		Tags:      tags,
	}
}

// mergeTags merges two tag maps
func mergeTags(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	result := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// copyTags creates a copy of tags map
func copyTags(tags map[string]string) map[string]string {
	if tags == nil {
		return make(map[string]string)
	}
	result := make(map[string]string, len(tags))
	for k, v := range tags {
		result[k] = v
	}
	return result
}
