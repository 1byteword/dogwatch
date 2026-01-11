package custommetrics

import (
	"encoding/json"
	"io"
	"net/http"
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
	Attributes        []otlpAttribute `json:"attributes"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	TimeUnixNano      string          `json:"timeUnixNano"`
	Count             uint64          `json:"count"`
	Sum               *float64        `json:"sum,omitempty"`
	BucketCounts      []uint64        `json:"bucketCounts"`
	ExplicitBounds    []float64       `json:"explicitBounds"`
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

	for _, rm := range otlpReq.ResourceMetrics {
		// Extract resource attributes as base tags
		resourceTags := attributesToMap(rm.Resource.Attributes)

		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				pts := r.convertMetric(m, resourceTags)
				points = append(points, pts...)
			}
		}
	}

	if len(points) > 0 {
		if err := r.store.RecordBatch(points); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"accepted": len(points)})
}

func (r *OTLPMetricsReceiver) convertMetric(m otlpMetric, baseTags map[string]string) []DataPoint {
	var points []DataPoint

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
			// For histograms, we store the sum/count as a gauge
			if dp.Sum != nil {
				p := DataPoint{
					Timestamp: parseNanoTime(dp.TimeUnixNano),
					Name:      m.Name + ".sum",
					Type:      Gauge,
					Value:     *dp.Sum,
					Tags:      mergeTags(baseTags, attributesToMap(dp.Attributes)),
				}
				points = append(points, p)
			}
			p := DataPoint{
				Timestamp: parseNanoTime(dp.TimeUnixNano),
				Name:      m.Name + ".count",
				Type:      Counter,
				Value:     float64(dp.Count),
				Tags:      mergeTags(baseTags, attributesToMap(dp.Attributes)),
			}
			points = append(points, p)
		}
	}

	return points
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
