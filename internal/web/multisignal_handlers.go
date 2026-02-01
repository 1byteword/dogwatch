package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/correlation"
	"dogwatch/internal/logs"
)

var multiSignalCorrelator *correlation.MultiSignalCorrelator

// SetMultiSignalCorrelator sets the multi-signal correlator instance
func SetMultiSignalCorrelator(c *correlation.MultiSignalCorrelator) {
	multiSignalCorrelator = c
}

// MultiSignalHandlers provides HTTP handlers for multi-signal correlation
type MultiSignalHandlers struct {
	correlator *correlation.MultiSignalCorrelator
}

// NewMultiSignalHandlers creates new multi-signal handlers
func NewMultiSignalHandlers(c *correlation.MultiSignalCorrelator) *MultiSignalHandlers {
	return &MultiSignalHandlers{correlator: c}
}

// RegisterRoutes registers multi-signal correlation routes
func (h *MultiSignalHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Cross-signal timeline
	mux.HandleFunc("/api/multisignal/timeline", h.handleTimeline)
	mux.HandleFunc("/api/multisignal/timeline/", h.handleServiceTimeline)

	// Log to trace correlation
	mux.HandleFunc("/api/multisignal/log-to-trace", h.handleLogToTrace)

	// Metric to trace correlation
	mux.HandleFunc("/api/multisignal/metric-to-trace", h.handleMetricToTrace)

	// Find all signals for a trace
	mux.HandleFunc("/api/multisignal/trace/", h.handleCorrelatedSignals)

	// Exemplars
	mux.HandleFunc("/api/multisignal/exemplars", h.handleExemplars)
	mux.HandleFunc("/api/multisignal/exemplar", h.handleRecordExemplar)

	// Attribute correlation
	mux.HandleFunc("/api/multisignal/correlate-attributes", h.handleCorrelateByAttributes)
}

// handleTimeline returns a cross-signal timeline
// GET /api/multisignal/timeline?service=my-service&start=...&end=...&since=1h
func (h *MultiSignalHandlers) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	service := r.URL.Query().Get("service")

	// Parse time range
	start, end := parseTimeRangeMS(r)

	timeline, err := h.correlator.GetCrossSignalTimeline(r.Context(), service, start, end)
	if err != nil {
		http.Error(w, "Failed to get timeline: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeline)
}

// handleServiceTimeline returns timeline for a specific service
// GET /api/multisignal/timeline/{service}?since=1h
func (h *MultiSignalHandlers) handleServiceTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	service := strings.TrimPrefix(r.URL.Path, "/api/multisignal/timeline/")
	if service == "" {
		http.Error(w, "Service required", http.StatusBadRequest)
		return
	}

	start, end := parseTimeRangeMS(r)

	timeline, err := h.correlator.GetCrossSignalTimeline(r.Context(), service, start, end)
	if err != nil {
		http.Error(w, "Failed to get timeline: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeline)
}

// handleLogToTrace correlates a log entry to traces
// POST /api/multisignal/log-to-trace
func (h *MultiSignalHandlers) handleLogToTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	var req LogToTraceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.correlator.LogToTrace(r.Context(), req.ToLogEntry())
	if err != nil {
		http.Error(w, "Failed to correlate: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleMetricToTrace correlates a metric spike to traces
// POST /api/multisignal/metric-to-trace
func (h *MultiSignalHandlers) handleMetricToTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	var req MetricToTraceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ts := req.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	result, err := h.correlator.MetricToTrace(r.Context(), req.MetricName, req.Labels, ts, req.Value)
	if err != nil {
		http.Error(w, "Failed to correlate: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleCorrelatedSignals returns all signals for a trace ID
// GET /api/multisignal/trace/{trace_id}
func (h *MultiSignalHandlers) handleCorrelatedSignals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	traceID := strings.TrimPrefix(r.URL.Path, "/api/multisignal/trace/")
	if traceID == "" {
		http.Error(w, "Trace ID required", http.StatusBadRequest)
		return
	}

	result, err := h.correlator.FindCorrelatedSignals(r.Context(), traceID)
	if err != nil {
		http.Error(w, "Failed to find signals: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleExemplars returns exemplars for a metric
// GET /api/multisignal/exemplars?metric=http_request_duration&start=...&end=...
func (h *MultiSignalHandlers) handleExemplars(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	metricKey := r.URL.Query().Get("metric")
	if metricKey == "" {
		http.Error(w, "metric parameter required", http.StatusBadRequest)
		return
	}

	start, end := parseTimeRangeMS(r)

	var exemplars []correlation.Exemplar
	if !start.IsZero() && !end.IsZero() {
		exemplars = h.correlator.GetExemplarsInRange(metricKey, start, end)
	} else {
		exemplars = h.correlator.GetExemplars(metricKey)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metric_key": metricKey,
		"exemplars":  exemplars,
		"count":      len(exemplars),
	})
}

// handleRecordExemplar records a new exemplar
// POST /api/multisignal/exemplar
func (h *MultiSignalHandlers) handleRecordExemplar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	var req RecordExemplarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MetricKey == "" || req.TraceID == "" {
		http.Error(w, "metric_key and trace_id required", http.StatusBadRequest)
		return
	}

	ts := req.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	exemplar := correlation.Exemplar{
		TraceID:   req.TraceID,
		SpanID:    req.SpanID,
		Timestamp: ts,
		Value:     req.Value,
		Labels:    req.Labels,
	}

	h.correlator.RecordExemplar(req.MetricKey, exemplar)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "recorded",
		"exemplar": exemplar,
	})
}

// handleCorrelateByAttributes correlates signals by matching attributes
// POST /api/multisignal/correlate-attributes
func (h *MultiSignalHandlers) handleCorrelateByAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.correlator == nil {
		http.Error(w, "Multi-signal correlator not configured", http.StatusServiceUnavailable)
		return
	}

	var req AttributeCorrelationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Attributes) == 0 {
		http.Error(w, "attributes required", http.StatusBadRequest)
		return
	}

	start := req.StartTime
	end := req.EndTime
	if start.IsZero() {
		start = time.Now().Add(-1 * time.Hour)
	}
	if end.IsZero() {
		end = time.Now()
	}

	result, err := h.correlator.CorrelateByAttributes(r.Context(), req.Attributes, start, end)
	if err != nil {
		http.Error(w, "Failed to correlate: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Request types

// LogToTraceRequest is the request for log-to-trace correlation
type LogToTraceRequest struct {
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	Service    string            `json:"service"`
	Level      string            `json:"level"`
	Message    string            `json:"message"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ToLogEntry converts the request to a log entry
func (r LogToTraceRequest) ToLogEntry() *logs.LogEntry {
	return &logs.LogEntry{
		ID:        r.ID,
		Timestamp: r.Timestamp,
		Service:   r.Service,
		Level:     logs.LogLevel(r.Level),
		Message:   r.Message,
		TraceID:   r.TraceID,
		SpanID:    r.SpanID,
		Attrs:     r.Attributes,
	}
}

// MetricToTraceRequest is the request for metric-to-trace correlation
type MetricToTraceRequest struct {
	MetricName string            `json:"metric_name"`
	Labels     map[string]string `json:"labels,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Value      float64           `json:"value"`
}

// RecordExemplarRequest is the request for recording an exemplar
type RecordExemplarRequest struct {
	MetricKey string            `json:"metric_key"`
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// AttributeCorrelationRequest is the request for attribute correlation
type AttributeCorrelationRequest struct {
	Attributes map[string]string `json:"attributes"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
}

// parseTimeRangeMS parses start/end or since from query params
func parseTimeRangeMS(r *http.Request) (start, end time.Time) {
	end = time.Now()
	start = end.Add(-1 * time.Hour) // Default: last hour

	if startParam := r.URL.Query().Get("start"); startParam != "" {
		if t, err := time.Parse(time.RFC3339, startParam); err == nil {
			start = t
		}
	}

	if endParam := r.URL.Query().Get("end"); endParam != "" {
		if t, err := time.Parse(time.RFC3339, endParam); err == nil {
			end = t
		}
	}

	if since := r.URL.Query().Get("since"); since != "" {
		if d, err := time.ParseDuration(since); err == nil {
			start = time.Now().Add(-d)
			end = time.Now()
		}
	}

	return start, end
}
