package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/correlation"
)

// CorrelationHandlers provides HTTP handlers for correlation queries
type CorrelationHandlers struct {
	engine *correlation.Engine
}

// NewCorrelationHandlers creates correlation handlers
func NewCorrelationHandlers(engine *correlation.Engine) *CorrelationHandlers {
	return &CorrelationHandlers{engine: engine}
}

// RegisterRoutes registers correlation routes
func (h *CorrelationHandlers) RegisterRoutes(mux *http.ServeMux) {
	// Trace correlations
	mux.HandleFunc("/api/correlate/trace/", h.handleTraceContext)

	// Incident correlations
	mux.HandleFunc("/api/correlate/incident/", h.handleIncidentContext)

	// Deploy correlations
	mux.HandleFunc("/api/correlate/deploy/", h.handleDeployContext)

	// Service timeline
	mux.HandleFunc("/api/correlate/service/", h.handleServiceTimeline)

	// Alert context
	mux.HandleFunc("/api/correlate/alert", h.handleAlertContext)

	// Deploy-incident correlations
	mux.HandleFunc("/api/correlate/deploy-incidents", h.handleDeployIncidentCorrelations)
}

// handleTraceContext returns all data correlated to a trace
func (h *CorrelationHandlers) handleTraceContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	traceID := strings.TrimPrefix(r.URL.Path, "/api/correlate/trace/")
	if traceID == "" {
		http.Error(w, "Trace ID required", http.StatusBadRequest)
		return
	}

	ctx, err := h.engine.GetTraceContext(traceID)
	if err != nil {
		http.Error(w, "Failed to get trace context", http.StatusInternalServerError)
		return
	}

	if ctx == nil || ctx.Trace == nil {
		http.Error(w, "Trace not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}

// handleIncidentContext returns all data correlated to an incident
func (h *CorrelationHandlers) handleIncidentContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incidentID := strings.TrimPrefix(r.URL.Path, "/api/correlate/incident/")
	if incidentID == "" {
		http.Error(w, "Incident ID required", http.StatusBadRequest)
		return
	}

	ctx, err := h.engine.GetIncidentContext(incidentID)
	if err != nil {
		http.Error(w, "Failed to get incident context", http.StatusInternalServerError)
		return
	}

	if ctx == nil || ctx.Incident == nil {
		http.Error(w, "Incident not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}

// handleDeployContext returns all data correlated to a deployment
func (h *CorrelationHandlers) handleDeployContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deployID := strings.TrimPrefix(r.URL.Path, "/api/correlate/deploy/")
	if deployID == "" {
		http.Error(w, "Deploy ID required", http.StatusBadRequest)
		return
	}

	ctx, err := h.engine.GetDeployContext(deployID)
	if err != nil {
		http.Error(w, "Failed to get deploy context", http.StatusInternalServerError)
		return
	}

	if ctx == nil || ctx.Deployment == nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}

// handleServiceTimeline returns all events for a service
func (h *CorrelationHandlers) handleServiceTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/correlate/service/")
	parts := strings.SplitN(path, "/", 2)
	service := parts[0]

	if service == "" {
		http.Error(w, "Service required", http.StatusBadRequest)
		return
	}

	// Parse time range from query params
	start := time.Now().Add(-1 * time.Hour) // Default: last hour
	end := time.Now()

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

	// Also support "since" parameter for relative time
	if since := r.URL.Query().Get("since"); since != "" {
		if d, err := time.ParseDuration(since); err == nil {
			start = time.Now().Add(-d)
			end = time.Now()
		}
	}

	timeline, err := h.engine.GetServiceTimeline(service, start, end)
	if err != nil {
		http.Error(w, "Failed to get service timeline", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(timeline)
}

// handleAlertContext returns context for an alert
func (h *CorrelationHandlers) handleAlertContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	alertID := r.URL.Query().Get("alert_id")
	service := r.URL.Query().Get("service")
	triggeredAtStr := r.URL.Query().Get("triggered_at")

	if alertID == "" {
		http.Error(w, "alert_id required", http.StatusBadRequest)
		return
	}

	triggeredAt := time.Now()
	if triggeredAtStr != "" {
		if t, err := time.Parse(time.RFC3339, triggeredAtStr); err == nil {
			triggeredAt = t
		}
	}

	ctx, err := h.engine.GetAlertContext(alertID, service, triggeredAt)
	if err != nil {
		http.Error(w, "Failed to get alert context", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}

// handleDeployIncidentCorrelations returns all deploy->incident correlations
func (h *CorrelationHandlers) handleDeployIncidentCorrelations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Default to last 24 hours
	since := 24 * time.Hour
	if sinceParam := r.URL.Query().Get("since"); sinceParam != "" {
		if d, err := time.ParseDuration(sinceParam); err == nil {
			since = d
		}
	}

	correlations, err := h.engine.FindDeployIncidentCorrelations(since)
	if err != nil {
		http.Error(w, "Failed to find correlations", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"correlations": correlations,
		"since":        since.String(),
	})
}
