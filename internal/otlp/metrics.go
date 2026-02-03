package otlp

import (
	"math"
	"time"

	"dogwatch/internal/custommetrics"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
)

// metricsConversionResult holds both decomposed and native histogram data
type metricsConversionResult struct {
	dataPoints []custommetrics.DataPoint
	histograms []custommetrics.HistogramDataPoint
}

// processMetrics converts OTLP resource metrics and stores them
func processMetrics(metricsStore *custommetrics.Store, resourceMetrics []*metricspb.ResourceMetrics) error {
	if metricsStore == nil {
		return nil
	}

	var result metricsConversionResult

	for _, rm := range resourceMetrics {
		resourceTags := extractResourceTags(rm.Resource)

		for _, scopeMetrics := range rm.ScopeMetrics {
			for _, metric := range scopeMetrics.Metrics {
				convertMetricWithHistograms(metric, resourceTags, &result)
			}
		}
	}

	// Record decomposed metrics (backward compatibility)
	if len(result.dataPoints) > 0 {
		if err := metricsStore.RecordBatch(result.dataPoints); err != nil {
			return err
		}
	}

	// Record native histograms (dual-write)
	if len(result.histograms) > 0 {
		if err := metricsStore.RecordHistogramBatch(result.histograms); err != nil {
			return err
		}
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

// convertMetric converts an OTLP Metric to internal DataPoints (for backward compatibility)
func convertMetric(metric *metricspb.Metric, resourceTags map[string]string) []custommetrics.DataPoint {
	var result metricsConversionResult
	convertMetricWithHistograms(metric, resourceTags, &result)
	return result.dataPoints
}

// convertMetricWithHistograms converts an OTLP Metric to internal DataPoints and native histograms
func convertMetricWithHistograms(metric *metricspb.Metric, resourceTags map[string]string, result *metricsConversionResult) {
	switch data := metric.Data.(type) {
	case *metricspb.Metric_Sum:
		result.dataPoints = append(result.dataPoints, convertSum(metric.Name, data.Sum, resourceTags)...)
	case *metricspb.Metric_Gauge:
		result.dataPoints = append(result.dataPoints, convertGauge(metric.Name, data.Gauge, resourceTags)...)
	case *metricspb.Metric_Histogram:
		points, histograms := convertHistogramDualWrite(metric.Name, data.Histogram, resourceTags)
		result.dataPoints = append(result.dataPoints, points...)
		result.histograms = append(result.histograms, histograms...)
	case *metricspb.Metric_Summary:
		result.dataPoints = append(result.dataPoints, convertSummary(metric.Name, data.Summary, resourceTags)...)
	case *metricspb.Metric_ExponentialHistogram:
		points, histograms := convertExponentialHistogramDualWrite(metric.Name, data.ExponentialHistogram, resourceTags)
		result.dataPoints = append(result.dataPoints, points...)
		result.histograms = append(result.histograms, histograms...)
	}
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

// convertHistogram converts Histogram metric data to multiple gauge points (backward compatibility)
func convertHistogram(name string, hist *metricspb.Histogram, resourceTags map[string]string) []custommetrics.DataPoint {
	points, _ := convertHistogramDualWrite(name, hist, resourceTags)
	return points
}

// convertHistogramDualWrite converts Histogram metric data to both decomposed metrics and native histograms
func convertHistogramDualWrite(name string, hist *metricspb.Histogram, resourceTags map[string]string) ([]custommetrics.DataPoint, []custommetrics.HistogramDataPoint) {
	var points []custommetrics.DataPoint
	var histograms []custommetrics.HistogramDataPoint

	for _, dp := range hist.DataPoints {
		ts := time.Unix(0, int64(dp.TimeUnixNano))
		tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

		// Create native histogram data point
		hdp := custommetrics.HistogramDataPoint{
			Timestamp:      ts,
			Name:           name,
			Tags:           copyTags(tags),
			Count:          dp.Count,
			ExplicitBounds: append([]float64{}, dp.ExplicitBounds...),
			BucketCounts:   append([]uint64{}, dp.BucketCounts...),
		}
		if dp.Sum != nil {
			hdp.Sum = *dp.Sum
		}
		if dp.Min != nil {
			minVal := *dp.Min
			hdp.Min = &minVal
		}
		if dp.Max != nil {
			maxVal := *dp.Max
			hdp.Max = &maxVal
		}

		// Convert exemplars if present
		for _, ex := range dp.Exemplars {
			exemplar := custommetrics.Exemplar{
				Timestamp: time.Unix(0, int64(ex.TimeUnixNano)),
			}
			// Handle oneof Value field
			switch v := ex.Value.(type) {
			case *metricspb.Exemplar_AsDouble:
				exemplar.Value = v.AsDouble
			case *metricspb.Exemplar_AsInt:
				exemplar.Value = float64(v.AsInt)
			}
			if len(ex.TraceId) > 0 {
				exemplar.TraceID = bytesToHex(ex.TraceId)
			}
			if len(ex.SpanId) > 0 {
				exemplar.SpanID = bytesToHex(ex.SpanId)
			}
			hdp.Exemplars = append(hdp.Exemplars, exemplar)
		}

		histograms = append(histograms, hdp)

		// Backward compatibility: Store count
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
	return points, histograms
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

// convertExponentialHistogram converts ExponentialHistogram metric data (backward compatibility)
func convertExponentialHistogram(name string, hist *metricspb.ExponentialHistogram, resourceTags map[string]string) []custommetrics.DataPoint {
	points, _ := convertExponentialHistogramDualWrite(name, hist, resourceTags)
	return points
}

// convertExponentialHistogramDualWrite converts ExponentialHistogram metric data to both decomposed metrics and native histograms
func convertExponentialHistogramDualWrite(name string, hist *metricspb.ExponentialHistogram, resourceTags map[string]string) ([]custommetrics.DataPoint, []custommetrics.HistogramDataPoint) {
	var points []custommetrics.DataPoint
	var histograms []custommetrics.HistogramDataPoint

	for _, dp := range hist.DataPoints {
		ts := time.Unix(0, int64(dp.TimeUnixNano))
		tags := mergeTags(resourceTags, convertAttributes(dp.Attributes))

		// Convert exponential histogram buckets to explicit bounds for native storage
		// This approximates the exponential buckets as explicit bounds
		bounds, counts := convertExpBucketsToExplicit(dp)

		// Create native histogram data point
		hdp := custommetrics.HistogramDataPoint{
			Timestamp:      ts,
			Name:           name,
			Tags:           copyTags(tags),
			Count:          dp.Count,
			ExplicitBounds: bounds,
			BucketCounts:   counts,
		}
		if dp.Sum != nil {
			hdp.Sum = *dp.Sum
		}
		if dp.Min != nil {
			minVal := *dp.Min
			hdp.Min = &minVal
		}
		if dp.Max != nil {
			maxVal := *dp.Max
			hdp.Max = &maxVal
		}

		// Convert exemplars if present
		for _, ex := range dp.Exemplars {
			exemplar := custommetrics.Exemplar{
				Timestamp: time.Unix(0, int64(ex.TimeUnixNano)),
			}
			// Handle oneof Value field
			switch v := ex.Value.(type) {
			case *metricspb.Exemplar_AsDouble:
				exemplar.Value = v.AsDouble
			case *metricspb.Exemplar_AsInt:
				exemplar.Value = float64(v.AsInt)
			}
			if len(ex.TraceId) > 0 {
				exemplar.TraceID = bytesToHex(ex.TraceId)
			}
			if len(ex.SpanId) > 0 {
				exemplar.SpanID = bytesToHex(ex.SpanId)
			}
			hdp.Exemplars = append(hdp.Exemplars, exemplar)
		}

		histograms = append(histograms, hdp)

		// Backward compatibility: Store count
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
	return points, histograms
}

// convertExpBucketsToExplicit converts exponential histogram buckets to explicit bounds
// This is an approximation that preserves the histogram shape for quantile computation
func convertExpBucketsToExplicit(dp *metricspb.ExponentialHistogramDataPoint) ([]float64, []uint64) {
	scale := dp.Scale
	base := math.Pow(2, math.Pow(2, float64(-scale)))

	var bounds []float64
	var counts []uint64

	// Process positive buckets
	if dp.Positive != nil && len(dp.Positive.BucketCounts) > 0 {
		offset := dp.Positive.Offset
		for i, count := range dp.Positive.BucketCounts {
			idx := offset + int32(i)
			upperBound := math.Pow(base, float64(idx+1))
			bounds = append(bounds, upperBound)
			counts = append(counts, count)
		}
	}

	// Add zero count if significant
	if dp.ZeroCount > 0 {
		// Insert zero bucket at the beginning
		bounds = append([]float64{0}, bounds...)
		counts = append([]uint64{dp.ZeroCount}, counts...)
	}

	return bounds, counts
}

// bytesToHex converts a byte slice to a hex string
func bytesToHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
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
