package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/metrics"
)

var cardinalityController *metrics.CardinalityController

// SetCardinalityController sets the cardinality controller instance
func SetCardinalityController(c *metrics.CardinalityController) {
	cardinalityController = c
}

// RegisterCardinalityControllerRoutes registers cardinality controller API routes
func RegisterCardinalityControllerRoutes(mux *http.ServeMux) {
	// Dashboard and stats
	mux.HandleFunc("/api/cardinality/dashboard", handleCardinalityDashboard)
	mux.HandleFunc("/api/cardinality/controller/stats", handleControllerStats)

	// Top metrics and labels
	mux.HandleFunc("/api/cardinality/controller/metrics", handleControllerMetrics)
	mux.HandleFunc("/api/cardinality/controller/labels", handleControllerLabels)

	// Circuit breaker
	mux.HandleFunc("/api/cardinality/circuit-breaker", handleCircuitBreaker)
	mux.HandleFunc("/api/cardinality/circuit-breaker/reset", handleCircuitBreakerReset)

	// Quarantine management
	mux.HandleFunc("/api/cardinality/quarantine", handleQuarantine)
	mux.HandleFunc("/api/cardinality/quarantine/", handleQuarantineMetric)

	// Alerts
	mux.HandleFunc("/api/cardinality/alerts", handleCardinalityAlerts)

	// Test endpoint for recording series
	mux.HandleFunc("/api/cardinality/record", handleRecordSeries)
}

// handleCardinalityDashboard returns full dashboard data
// GET /api/cardinality/dashboard
func handleCardinalityDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	dashboard := cardinalityController.GetDashboardData()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard)
}

// handleControllerStats returns basic statistics
// GET /api/cardinality/controller/stats
func handleControllerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	stats := cardinalityController.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleControllerMetrics returns top metrics by cardinality
// GET /api/cardinality/controller/metrics?limit=20
func handleControllerMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	metrics := cardinalityController.GetTopMetrics(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

// handleControllerLabels returns top labels by cardinality
// GET /api/cardinality/controller/labels?limit=20
func handleControllerLabels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	labels := cardinalityController.GetTopLabels(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"labels": labels,
		"count":  len(labels),
	})
}

// handleCircuitBreaker returns circuit breaker state
// GET /api/cardinality/circuit-breaker
func handleCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	state := cardinalityController.GetCircuitBreakerState()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// handleCircuitBreakerReset manually resets the circuit breaker
// POST /api/cardinality/circuit-breaker/reset
func handleCircuitBreakerReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	cardinalityController.ResetCircuitBreaker()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "reset",
		"message": "Circuit breaker has been reset to closed state",
	})
}

// handleQuarantine manages quarantined metrics
// GET /api/cardinality/quarantine - list quarantined metrics
// POST /api/cardinality/quarantine - quarantine a metric manually
func handleQuarantine(w http.ResponseWriter, r *http.Request) {
	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		quarantined := cardinalityController.GetQuarantinedMetrics()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"quarantined": quarantined,
			"count":       len(quarantined),
		})

	case http.MethodPost:
		var req struct {
			MetricName    string   `json:"metric_name"`
			AllowedLabels []string `json:"allowed_labels,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.MetricName == "" {
			http.Error(w, "metric_name required", http.StatusBadRequest)
			return
		}

		// The controller doesn't expose direct quarantine, but we can set rules
		if len(req.AllowedLabels) > 0 {
			cardinalityController.SetQuarantineRules(req.MetricName, req.AllowedLabels)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "quarantine_rules_set",
			"metric_name":   req.MetricName,
			"allowed_labels": req.AllowedLabels,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQuarantineMetric manages a specific quarantined metric
// DELETE /api/cardinality/quarantine/{metric_name} - unquarantine
// PATCH /api/cardinality/quarantine/{metric_name} - update rules
func handleQuarantineMetric(w http.ResponseWriter, r *http.Request) {
	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	metricName := strings.TrimPrefix(r.URL.Path, "/api/cardinality/quarantine/")
	if metricName == "" {
		http.Error(w, "Metric name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		cardinalityController.UnquarantineMetric(metricName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":      "unquarantined",
			"metric_name": metricName,
		})

	case http.MethodPatch:
		var req struct {
			AllowedLabels []string `json:"allowed_labels"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		cardinalityController.SetQuarantineRules(metricName, req.AllowedLabels)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "rules_updated",
			"metric_name":    metricName,
			"allowed_labels": req.AllowedLabels,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCardinalityAlerts returns recent cardinality alerts
// GET /api/cardinality/alerts?limit=50
func handleCardinalityAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	alerts := cardinalityController.GetRecentAlerts(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

// handleRecordSeries allows testing the cardinality controller
// POST /api/cardinality/record
func handleRecordSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityController == nil {
		http.Error(w, "Cardinality controller not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		MetricName string            `json:"metric_name"`
		Labels     map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MetricName == "" {
		http.Error(w, "metric_name required", http.StatusBadRequest)
		return
	}

	result := cardinalityController.RecordSeries(req.MetricName, req.Labels)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metric_name": req.MetricName,
		"labels":      req.Labels,
		"result":      result,
		"timestamp":   time.Now(),
	})
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return val
}
