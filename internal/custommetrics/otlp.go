package custommetrics

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

// OTLPMetricsReceiver handles OTLP metrics ingestion
type OTLPMetricsReceiver struct {
	store *Store
}

// NewOTLPMetricsReceiver creates a new OTLP metrics receiver
func NewOTLPMetricsReceiver(store *Store) *OTLPMetricsReceiver {
	return &OTLPMetricsReceiver{store: store}
}

// OTLP JSON structures (simplified for common use cases)

type otlpMetricsRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

type resourceMetrics struct {
	Resource     otlpResource    `json:"resource"`
	ScopeMetrics []scopeMetrics  `json:"scopeMetrics"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

type scopeMetrics struct {
	Scope   otlpScope      `json:"scope"`
	Metrics []otlpMetric   `json:"metrics"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type otlpMetric struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Unit        string       `json:"unit"`
	Sum         *otlpSum     `json:"sum,omitempty"`
	Gauge       *otlpGauge   `json:"gauge,omitempty"`
	Histogram   *otlpHist    `json:"histogram,omitempty"`
}

type otlpSum struct {
	DataPoints             []otlpNumberDataPoint `json:"dataPoints"`
	AggregationTemporality int                   `json:"aggregationTemporality"`
	IsMonotonic            bool                  `json:"isMonotonic"`
}

type otlpGauge struct {
	DataPoints []otlpNumberDataPoint `json:"dataPoints"`
}

type otlpHist struct {
	DataPoints []otlpHistogramDataPoint `json:"dataPoints"`
}

type otlpNumberDataPoint struct {
	Attributes        []otlpAttribute `json:"attributes"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	TimeUnixNano      string          `json:"timeUnixNano"`
	AsInt             *int64          `json:"asInt,omitempty"`
	AsDouble          *float64        `json:"asDouble,omitempty"`
}

type otlpHistogramDataPoint struct {
	Attributes        []otlpAttribute  `json:"attributes"`
	StartTimeUnixNano string           `json:"startTimeUnixNano"`
	TimeUnixNano      string           `json:"timeUnixNano"`
	Count             uint64           `json:"count"`
	Sum               *float64         `json:"sum,omitempty"`
	Min               *float64         `json:"min,omitempty"`
	Max               *float64         `json:"max,omitempty"`
	BucketCounts      []uint64         `json:"bucketCounts"`
	ExplicitBounds    []float64        `json:"explicitBounds"`
	Exemplars         []otlpExemplar   `json:"exemplars,omitempty"`
}

type otlpExemplar struct {
	FilteredAttributes []otlpAttribute `json:"filteredAttributes,omitempty"`
	TimeUnixNano       string          `json:"timeUnixNano"`
	AsDouble           *float64        `json:"asDouble,omitempty"`
	AsInt              *int64          `json:"asInt,omitempty"`
	SpanID             string          `json:"spanId,omitempty"`
	TraceID            string          `json:"traceId,omitempty"`
}

type otlpAttribute struct {
	Key   string         `json:"key"`
	Value otlpAttrValue  `json:"value"`
}

type otlpAttrValue struct {
	StringValue string `json:"stringValue,omitempty"`
	IntValue    string `json:"intValue,omitempty"`
	BoolValue   bool   `json:"boolValue,omitempty"`
}

// HandleMetrics handles POST /v1/metrics
func (r *OTLPMetricsReceiver) HandleMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var otlpReq otlpMetricsRequest
	if err := json.Unmarshal(body, &otlpReq); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	var points []DataPoint
	var histograms []HistogramDataPoint

	for _, rm := range otlpReq.ResourceMetrics {
		// Extract resource attributes as base tags
		resourceTags := attributesToMap(rm.Resource.Attributes)

		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				pts, hists := r.convertMetricWithHistograms(m, resourceTags)
				points = append(points, pts...)
				histograms = append(histograms, hists...)
			}
		}
	}

	// Record decomposed metrics (backward compatibility)
	if len(points) > 0 {
		if err := r.store.RecordBatch(points); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Record native histograms (dual-write)
	if len(histograms) > 0 {
		if err := r.store.RecordHistogramBatch(histograms); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"accepted":   len(points),
		"histograms": len(histograms),
	})
}

func (r *OTLPMetricsReceiver) convertMetric(m otlpMetric, baseTags map[string]string) []DataPoint {
	points, _ := r.convertMetricWithHistograms(m, baseTags)
	return points
}

func (r *OTLPMetricsReceiver) convertMetricWithHistograms(m otlpMetric, baseTags map[string]string) ([]DataPoint, []HistogramDataPoint) {
	var points []DataPoint
	var histograms []HistogramDataPoint

	if m.Sum != nil {
		metricType := Counter
		if !m.Sum.IsMonotonic {
			metricType = Gauge
		}
		for _, dp := range m.Sum.DataPoints {
			points = append(points, r.convertNumberDataPoint(m.Name, metricType, dp, baseTags))
		}
	}

	if m.Gauge != nil {
		for _, dp := range m.Gauge.DataPoints {
			points = append(points, r.convertNumberDataPoint(m.Name, Gauge, dp, baseTags))
		}
	}

	if m.Histogram != nil {
		for _, dp := range m.Histogram.DataPoints {
			ts := parseNanoTime(dp.TimeUnixNano)
			tags := mergeTags(baseTags, attributesToMap(dp.Attributes))

			// Create native histogram data point
			hdp := HistogramDataPoint{
				Timestamp:      ts,
				Name:           m.Name,
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

			// Convert exemplars
			for _, ex := range dp.Exemplars {
				exemplar := Exemplar{
					Timestamp: parseNanoTime(ex.TimeUnixNano),
					TraceID:   ex.TraceID,
					SpanID:    ex.SpanID,
				}
				if ex.AsDouble != nil {
					exemplar.Value = *ex.AsDouble
				} else if ex.AsInt != nil {
					exemplar.Value = float64(*ex.AsInt)
				}
				hdp.Exemplars = append(hdp.Exemplars, exemplar)
			}

			histograms = append(histograms, hdp)

			// Backward compatibility: store decomposed metrics
			if dp.Sum != nil {
				p := DataPoint{
					Timestamp: ts,
					Name:      m.Name + ".sum",
					Type:      Gauge,
					Value:     *dp.Sum,
					Tags:      copyTags(tags),
				}
				points = append(points, p)
			}
			p := DataPoint{
				Timestamp: ts,
				Name:      m.Name + ".count",
				Type:      Counter,
				Value:     float64(dp.Count),
				Tags:      copyTags(tags),
			}
			points = append(points, p)

			// Store bucket counts for backward compatibility
			for i, count := range dp.BucketCounts {
				bucketTags := copyTags(tags)
				bucketTags["aggregate"] = "bucket"
				if i < len(dp.ExplicitBounds) {
					bucketTags["le"] = formatFloatOTLP(dp.ExplicitBounds[i])
				} else {
					bucketTags["le"] = "+Inf"
				}
				points = append(points, DataPoint{
					Timestamp: ts,
					Name:      m.Name + "_bucket",
					Type:      Counter,
					Value:     float64(count),
					Tags:      bucketTags,
				})
			}
		}
	}

	return points, histograms
}

func formatFloatOTLP(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (r *OTLPMetricsReceiver) convertNumberDataPoint(name string, mtype MetricType, dp otlpNumberDataPoint, baseTags map[string]string) DataPoint {
	var value float64
	if dp.AsDouble != nil {
		value = *dp.AsDouble
	} else if dp.AsInt != nil {
		value = float64(*dp.AsInt)
	}

	return DataPoint{
		Timestamp: parseNanoTime(dp.TimeUnixNano),
		Name:      name,
		Type:      mtype,
		Value:     value,
		Tags:      mergeTags(baseTags, attributesToMap(dp.Attributes)),
	}
}

func attributesToMap(attrs []otlpAttribute) map[string]string {
	m := make(map[string]string)
	for _, a := range attrs {
		if a.Value.StringValue != "" {
			m[a.Key] = a.Value.StringValue
		} else if a.Value.IntValue != "" {
			m[a.Key] = a.Value.IntValue
		}
	}
	return m
}

func mergeTags(base, overlay map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

func parseNanoTime(nanoStr string) time.Time {
	if nanoStr == "" {
		return time.Now()
	}
	// Parse nanoseconds since epoch
	var nanos int64
	json.Unmarshal([]byte(nanoStr), &nanos)
	if nanos == 0 {
		return time.Now()
	}
	return time.Unix(0, nanos)
}
