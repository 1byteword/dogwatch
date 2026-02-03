package ingest

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// PrometheusParser handles Prometheus remote read/write protocols
type PrometheusParser struct {
	store MetricStore
}

// NewPrometheusParser creates a new Prometheus parser
func NewPrometheusParser(store MetricStore) *PrometheusParser {
	return &PrometheusParser{store: store}
}

// PrometheusWriteRequest represents the JSON remote write format
// (Note: production would use protobuf, but JSON is easier for testing)
type PrometheusWriteRequest struct {
	Timeseries []PrometheusTimeSeries `json:"timeseries"`
}

// PrometheusTimeSeries represents a single time series
type PrometheusTimeSeries struct {
	Labels  []PrometheusLabel `json:"labels"`
	Samples []PrometheusSample `json:"samples"`
}

// PrometheusLabel is a label key-value pair
type PrometheusLabel struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PrometheusSample is a timestamp-value pair
type PrometheusSample struct {
	Timestamp int64   `json:"timestamp"` // milliseconds
	Value     float64 `json:"value"`
}

// ParseRemoteWrite parses Prometheus remote write format (JSON version)
func (p *PrometheusParser) ParseRemoteWrite(r io.Reader, contentEncoding string) (*Batch, error) {
	reader := r
	if contentEncoding == "gzip" || contentEncoding == "snappy" {
		var err error
		reader, err = gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.(io.Closer).Close()
	}

	batch := &Batch{
		Samples: make([]Sample, 0),
		Source:  "prometheus-remote-write",
	}

	var req PrometheusWriteRequest
	if err := json.NewDecoder(reader).Decode(&req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	for _, ts := range req.Timeseries {
		// Extract metric name and labels
		var metricName string
		tags := make(map[string]string)

		for _, label := range ts.Labels {
			if label.Name == "__name__" {
				metricName = label.Value
			} else {
				tags[label.Name] = label.Value
			}
		}

		if metricName == "" {
			continue
		}

		for _, sample := range ts.Samples {
			batch.Samples = append(batch.Samples, Sample{
				Metric:    metricName,
				Value:     sample.Value,
				Timestamp: time.Unix(sample.Timestamp/1000, (sample.Timestamp%1000)*1e6),
				Tags:      copyTags(tags),
			})
		}
	}

	return batch, nil
}

// PrometheusReadRequest represents a remote read request
type PrometheusReadRequest struct {
	Queries []PrometheusQuery `json:"queries"`
}

// PrometheusQuery represents a single query in a read request
type PrometheusQuery struct {
	StartTimestampMs int64                `json:"start_timestamp_ms"`
	EndTimestampMs   int64                `json:"end_timestamp_ms"`
	Matchers         []PrometheusMatcher  `json:"matchers"`
}

// PrometheusMatcher represents a label matcher
type PrometheusMatcher struct {
	Type  int    `json:"type"` // 0=EQ, 1=NEQ, 2=RE, 3=NRE
	Name  string `json:"name"`
	Value string `json:"value"`
}

// PrometheusReadResponse represents a remote read response
type PrometheusReadResponse struct {
	Results []PrometheusQueryResult `json:"results"`
}

// PrometheusQueryResult is the result for a single query
type PrometheusQueryResult struct {
	Timeseries []PrometheusTimeSeries `json:"timeseries"`
}

// MetricQuerier interface for querying metrics
type MetricQuerier interface {
	QueryMetrics(metricName string, tags map[string]string, start, end time.Time) ([]Sample, error)
	ListMetrics() ([]string, error)
}

// HandleRemoteWrite handles POST /api/v1/write (Prometheus remote write)
func (p *PrometheusParser) HandleRemoteWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := p.ParseRemoteWrite(r.Body, r.Header.Get("Content-Encoding"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := p.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PrometheusAPIHandlers provides Prometheus-compatible API handlers
type PrometheusAPIHandlers struct {
	querier MetricQuerier
}

// NewPrometheusAPIHandlers creates new API handlers
func NewPrometheusAPIHandlers(querier MetricQuerier) *PrometheusAPIHandlers {
	return &PrometheusAPIHandlers{querier: querier}
}

// HandleQuery handles GET/POST /api/v1/query (instant query)
func (h *PrometheusAPIHandlers) HandleQuery(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" && r.Method == http.MethodPost {
		r.ParseForm()
		query = r.PostFormValue("query")
	}

	if query == "" {
		writePrometheusError(w, "missing query parameter", http.StatusBadRequest)
		return
	}

	timeStr := r.URL.Query().Get("time")
	evalTime := time.Now()
	if timeStr != "" {
		if t, err := parsePrometheusTime(timeStr); err == nil {
			evalTime = t
		}
	}

	// For now, just return empty result
	// Full query execution would require PromQL parser
	result := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "vector",
			"result":     []interface{}{},
		},
	}

	// Add warning that full PromQL is not implemented
	_ = evalTime // Use the time in actual implementation

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleQueryRange handles GET/POST /api/v1/query_range (range query)
func (h *PrometheusAPIHandlers) HandleQueryRange(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" && r.Method == http.MethodPost {
		r.ParseForm()
		query = r.PostFormValue("query")
	}

	if query == "" {
		writePrometheusError(w, "missing query parameter", http.StatusBadRequest)
		return
	}

	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	stepStr := r.URL.Query().Get("step")

	if startStr == "" || endStr == "" || stepStr == "" {
		writePrometheusError(w, "missing start, end, or step parameter", http.StatusBadRequest)
		return
	}

	// Parse times
	start, err := parsePrometheusTime(startStr)
	if err != nil {
		writePrometheusError(w, fmt.Sprintf("invalid start time: %v", err), http.StatusBadRequest)
		return
	}
	end, err := parsePrometheusTime(endStr)
	if err != nil {
		writePrometheusError(w, fmt.Sprintf("invalid end time: %v", err), http.StatusBadRequest)
		return
	}

	// For now, return empty matrix result
	result := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "matrix",
			"result":     []interface{}{},
		},
	}

	_ = start
	_ = end

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSeries handles GET/POST /api/v1/series
func (h *PrometheusAPIHandlers) HandleSeries(w http.ResponseWriter, r *http.Request) {
	// Return empty series list for now
	result := map[string]interface{}{
		"status": "success",
		"data":   []interface{}{},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleLabels handles GET /api/v1/labels
func (h *PrometheusAPIHandlers) HandleLabels(w http.ResponseWriter, r *http.Request) {
	// Return common label names
	labels := []string{"__name__", "instance", "job", "host", "service"}

	result := map[string]interface{}{
		"status": "success",
		"data":   labels,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleLabelValues handles GET /api/v1/label/{name}/values
func (h *PrometheusAPIHandlers) HandleLabelValues(w http.ResponseWriter, r *http.Request) {
	// Extract label name from path
	// Path format: /api/v1/label/{name}/values

	// Return empty values for now
	result := map[string]interface{}{
		"status": "success",
		"data":   []string{},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleMetadata handles GET /api/v1/metadata
func (h *PrometheusAPIHandlers) HandleMetadata(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"status": "success",
		"data":   map[string]interface{}{},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleTargets handles GET /api/v1/targets (stub)
func (h *PrometheusAPIHandlers) HandleTargets(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"activeTargets":  []interface{}{},
			"droppedTargets": []interface{}{},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleAlerts handles GET /api/v1/alerts (stub)
func (h *PrometheusAPIHandlers) HandleAlerts(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"alerts": []interface{}{},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleRules handles GET /api/v1/rules (stub)
func (h *PrometheusAPIHandlers) HandleRules(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"groups": []interface{}{},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// parsePrometheusTime parses Prometheus time formats
func parsePrometheusTime(s string) (time.Time, error) {
	// Try Unix timestamp (float seconds)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s", s)
}

func writePrometheusError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "error",
		"errorType": "bad_data",
		"error":     msg,
	})
}

// RegisterPrometheusRoutes registers all Prometheus-compatible API routes
func RegisterPrometheusRoutes(mux *http.ServeMux, parser *PrometheusParser, handlers *PrometheusAPIHandlers) {
	// Remote write (already handled by existing prometheus receiver, but add fallback)
	if parser != nil {
		mux.HandleFunc("/api/prometheus/write", parser.HandleRemoteWrite)
	}

	if handlers != nil {
		// Query API
		mux.HandleFunc("/api/prometheus/v1/query", handlers.HandleQuery)
		mux.HandleFunc("/api/prometheus/v1/query_range", handlers.HandleQueryRange)
		mux.HandleFunc("/api/prometheus/v1/series", handlers.HandleSeries)
		mux.HandleFunc("/api/prometheus/v1/labels", handlers.HandleLabels)
		mux.HandleFunc("/api/prometheus/v1/label/", handlers.HandleLabelValues) // Catch-all for label values
		mux.HandleFunc("/api/prometheus/v1/metadata", handlers.HandleMetadata)
		mux.HandleFunc("/api/prometheus/v1/targets", handlers.HandleTargets)
		mux.HandleFunc("/api/prometheus/v1/alerts", handlers.HandleAlerts)
		mux.HandleFunc("/api/prometheus/v1/rules", handlers.HandleRules)
	}
}

// SamplesByTime sorts samples by timestamp
type SamplesByTime []Sample

func (s SamplesByTime) Len() int           { return len(s) }
func (s SamplesByTime) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SamplesByTime) Less(i, j int) bool { return s[i].Timestamp.Before(s[j].Timestamp) }

// SortSamples sorts samples by timestamp
func SortSamples(samples []Sample) {
	sort.Sort(SamplesByTime(samples))
}
