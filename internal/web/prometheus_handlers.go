package web

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/promql"
)

// PrometheusHandler provides Prometheus-compatible HTTP API endpoints.
type PrometheusHandler struct {
	engine *promql.Engine
}

// NewPrometheusHandler creates a new Prometheus API handler.
func NewPrometheusHandler(db *sql.DB) *PrometheusHandler {
	store := promql.NewSQLMetricsStore(db)
	engine := promql.NewEngine(store)
	return &PrometheusHandler{engine: engine}
}

// RegisterRoutes registers all Prometheus API routes.
func (h *PrometheusHandler) RegisterRoutes(mux *http.ServeMux) {
	// Query endpoints
	mux.HandleFunc("GET /api/v1/query", h.handleQuery)
	mux.HandleFunc("POST /api/v1/query", h.handleQuery)
	mux.HandleFunc("GET /api/v1/query_range", h.handleQueryRange)
	mux.HandleFunc("POST /api/v1/query_range", h.handleQueryRange)

	// Metadata endpoints
	mux.HandleFunc("GET /api/v1/series", h.handleSeries)
	mux.HandleFunc("POST /api/v1/series", h.handleSeries)
	mux.HandleFunc("GET /api/v1/labels", h.handleLabels)
	mux.HandleFunc("POST /api/v1/labels", h.handleLabels)
	mux.HandleFunc("GET /api/v1/label/{name}/values", h.handleLabelValues)
	mux.HandleFunc("GET /api/v1/metadata", h.handleMetadata)

	// Status endpoints (stubs for compatibility)
	mux.HandleFunc("GET /api/v1/status/config", h.handleStatusConfig)
	mux.HandleFunc("GET /api/v1/status/flags", h.handleStatusFlags)
	mux.HandleFunc("GET /api/v1/status/runtimeinfo", h.handleRuntimeInfo)
	mux.HandleFunc("GET /api/v1/status/buildinfo", h.handleBuildInfo)
	mux.HandleFunc("GET /api/v1/targets", h.handleTargets)
	mux.HandleFunc("GET /api/v1/alerts", h.handleAlerts)
	mux.HandleFunc("GET /api/v1/rules", h.handleRules)
}

// handleQuery handles instant query requests.
func (h *PrometheusHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePrometheusError(w, "bad_data", "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	query := r.FormValue("query")
	if query == "" {
		writePrometheusError(w, "bad_data", "missing query parameter", http.StatusBadRequest)
		return
	}

	// Parse time parameter
	ts := time.Now()
	if timeParam := r.FormValue("time"); timeParam != "" {
		var err error
		ts, err = promql.ParseTime(timeParam)
		if err != nil {
			writePrometheusError(w, "bad_data", "invalid time parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Parse timeout
	timeout := 2 * time.Minute
	if timeoutParam := r.FormValue("timeout"); timeoutParam != "" {
		d, err := promql.ParseDurationParam(timeoutParam, timeout)
		if err != nil {
			writePrometheusError(w, "bad_data", "invalid timeout parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		timeout = d
	}

	ctx := r.Context()
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := h.engine.Query(ctx, query, ts)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusBadRequest)
		return
	}

	writePrometheusJSON(w, promql.FormatQueryResult(result))
}

// handleQueryRange handles range query requests.
func (h *PrometheusHandler) handleQueryRange(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePrometheusError(w, "bad_data", "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	query := r.FormValue("query")
	if query == "" {
		writePrometheusError(w, "bad_data", "missing query parameter", http.StatusBadRequest)
		return
	}

	// Parse start time
	start, err := promql.ParseTime(r.FormValue("start"))
	if err != nil {
		writePrometheusError(w, "bad_data", "invalid start parameter: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse end time
	end, err := promql.ParseTime(r.FormValue("end"))
	if err != nil {
		writePrometheusError(w, "bad_data", "invalid end parameter: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Parse step
	step, err := promql.ParseDurationParam(r.FormValue("step"), time.Minute)
	if err != nil {
		writePrometheusError(w, "bad_data", "invalid step parameter: "+err.Error(), http.StatusBadRequest)
		return
	}
	if step <= 0 {
		writePrometheusError(w, "bad_data", "zero or negative query resolution step width", http.StatusBadRequest)
		return
	}

	// Limit the number of points
	maxPoints := int64(11000)
	numPoints := int64(end.Sub(start) / step)
	if numPoints > maxPoints {
		writePrometheusError(w, "bad_data", "exceeded maximum resolution of 11,000 points per timeseries", http.StatusBadRequest)
		return
	}

	// Parse timeout
	timeout := 2 * time.Minute
	if timeoutParam := r.FormValue("timeout"); timeoutParam != "" {
		d, err := promql.ParseDurationParam(timeoutParam, timeout)
		if err != nil {
			writePrometheusError(w, "bad_data", "invalid timeout parameter: "+err.Error(), http.StatusBadRequest)
			return
		}
		timeout = d
	}

	ctx := r.Context()
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := h.engine.QueryRange(ctx, query, start, end, step)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusBadRequest)
		return
	}

	writePrometheusJSON(w, promql.FormatQueryResult(result))
}

// handleSeries handles series metadata requests.
func (h *PrometheusHandler) handleSeries(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePrometheusError(w, "bad_data", "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	matchers := r.Form["match[]"]
	if len(matchers) == 0 {
		writePrometheusError(w, "bad_data", "missing match[] parameter", http.StatusBadRequest)
		return
	}

	// Parse time range
	start, _ := promql.ParseTime(r.FormValue("start"))
	if start.IsZero() {
		start = time.Now().Add(-time.Hour)
	}
	end, _ := promql.ParseTime(r.FormValue("end"))
	if end.IsZero() {
		end = time.Now()
	}

	series, err := h.engine.Series(r.Context(), matchers, start, end)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data:   series,
	})
}

// handleLabels handles label names requests.
func (h *PrometheusHandler) handleLabels(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writePrometheusError(w, "bad_data", "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	matchers := r.Form["match[]"]

	start, _ := promql.ParseTime(r.FormValue("start"))
	if start.IsZero() {
		start = time.Now().Add(-time.Hour)
	}
	end, _ := promql.ParseTime(r.FormValue("end"))
	if end.IsZero() {
		end = time.Now()
	}

	labels, err := h.engine.LabelNames(r.Context(), matchers, start, end)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data:   labels,
	})
}

// handleLabelValues handles label values requests.
func (h *PrometheusHandler) handleLabelValues(w http.ResponseWriter, r *http.Request) {
	// Extract label name from path
	labelName := r.PathValue("name")
	if labelName == "" {
		writePrometheusError(w, "bad_data", "missing label name", http.StatusBadRequest)
		return
	}

	if err := r.ParseForm(); err != nil {
		writePrometheusError(w, "bad_data", "failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	matchers := r.Form["match[]"]

	start, _ := promql.ParseTime(r.FormValue("start"))
	if start.IsZero() {
		start = time.Now().Add(-time.Hour)
	}
	end, _ := promql.ParseTime(r.FormValue("end"))
	if end.IsZero() {
		end = time.Now()
	}

	values, err := h.engine.LabelValues(r.Context(), labelName, matchers, start, end)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data:   values,
	})
}

// handleMetadata handles metric metadata requests.
func (h *PrometheusHandler) handleMetadata(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	limit := 0
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, _ = strconv.Atoi(limitStr)
	}

	metadata, err := h.engine.Metadata(r.Context(), metric, limit)
	if err != nil {
		writePrometheusError(w, "execution", err.Error(), http.StatusInternalServerError)
		return
	}

	// Format as Prometheus expects: map[metric][]metadata
	result := make(map[string][]map[string]string)
	for _, m := range metadata {
		result[m.Metric] = append(result[m.Metric], map[string]string{
			"type": m.Type,
			"help": m.Help,
			"unit": m.Unit,
		})
	}

	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data:   result,
	})
}

// Status and other endpoints (stubs for Grafana compatibility)

func (h *PrometheusHandler) handleStatusConfig(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"yaml": "",
		},
	})
}

func (h *PrometheusHandler) handleStatusFlags(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data:   map[string]string{},
	})
}

func (h *PrometheusHandler) handleRuntimeInfo(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"startTime":           time.Now().Format(time.RFC3339),
			"CWD":                 "/",
			"reloadConfigSuccess": true,
			"lastConfigTime":      time.Now().Format(time.RFC3339),
			"corruptionCount":     0,
			"goroutineCount":      10,
			"GOMAXPROCS":          4,
			"GOGC":                "",
			"GODEBUG":             "",
			"storageRetention":    "15d",
		},
	})
}

func (h *PrometheusHandler) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"version":   "2.45.0",
			"revision":  "dogwatch",
			"branch":    "main",
			"buildUser": "dogwatch",
			"buildDate": time.Now().Format("20060102-15:04:05"),
			"goVersion": "go1.21",
		},
	})
}

func (h *PrometheusHandler) handleTargets(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"activeTargets":  []interface{}{},
			"droppedTargets": []interface{}{},
		},
	})
}

func (h *PrometheusHandler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"alerts": []interface{}{},
		},
	})
}

func (h *PrometheusHandler) handleRules(w http.ResponseWriter, r *http.Request) {
	writePrometheusJSON(w, &promql.PrometheusResponse{
		Status: "success",
		Data: map[string]interface{}{
			"groups": []interface{}{},
		},
	})
}

// Helper functions

func writePrometheusJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	// Enable CORS for Grafana
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writePrometheusError(w http.ResponseWriter, errorType, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(promql.ErrorResponse(errorType, msg))
}

// SanitizeLabelName ensures a label name is valid.
func SanitizeLabelName(name string) string {
	// Replace invalid characters with underscore
	result := strings.Builder{}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			result.WriteRune(r)
		} else if (r >= '0' && r <= '9') && i > 0 {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}
