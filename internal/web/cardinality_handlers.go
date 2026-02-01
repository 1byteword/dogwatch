package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/cardinality"
)

var cardinalityExplorer *cardinality.Explorer

// SetCardinalityExplorer sets the cardinality explorer instance
func SetCardinalityExplorer(e *cardinality.Explorer) {
	cardinalityExplorer = e
}

// RegisterCardinalityRoutes registers cardinality explorer API routes
func RegisterCardinalityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cardinality/report", handleCardinalityReport)
	mux.HandleFunc("/api/cardinality/metrics", handleCardinalityMetrics)
	mux.HandleFunc("/api/cardinality/metric", handleCardinalityMetric)
	mux.HandleFunc("/api/cardinality/tags", handleCardinalityTags)
	mux.HandleFunc("/api/cardinality/tag", handleCardinalityTag)
	mux.HandleFunc("/api/cardinality/high", handleHighCardinality)
	mux.HandleFunc("/api/cardinality/stats", handleCardinalityStats)
}

// handleCardinalityReport returns full cardinality analysis
// GET /api/cardinality/report?limit=20
func handleCardinalityReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseQueryInt(r, "limit", 20)
	report := cardinalityExplorer.GetReport(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleCardinalityMetrics returns cardinality for all metrics
// GET /api/cardinality/metrics?limit=50&sort=series
func handleCardinalityMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseQueryInt(r, "limit", 50)
	report := cardinalityExplorer.GetReport(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":   report.TotalMetrics,
		"metrics": report.TopMetrics,
	})
}

// handleCardinalityMetric returns cardinality for a specific metric
// GET /api/cardinality/metric?name=http_requests_total
func handleCardinalityMetric(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name parameter required", http.StatusBadRequest)
		return
	}

	mc := cardinalityExplorer.GetMetricCardinality(name)
	if mc == nil {
		http.Error(w, "metric not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mc)
}

// handleCardinalityTags returns cardinality for all tag keys
// GET /api/cardinality/tags?limit=20
func handleCardinalityTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseQueryInt(r, "limit", 20)
	report := cardinalityExplorer.GetReport(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total": report.TotalTagKeys,
		"tags":  report.TopTags,
	})
}

// handleCardinalityTag returns cardinality for a specific tag
// GET /api/cardinality/tag?key=host
func handleCardinalityTag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key parameter required", http.StatusBadRequest)
		return
	}

	tc := cardinalityExplorer.GetTagCardinality(key)
	if tc == nil {
		http.Error(w, "tag not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tc)
}

// handleHighCardinality returns metrics exceeding cardinality threshold
// GET /api/cardinality/high?threshold=1000
func handleHighCardinality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	threshold := parseQueryInt(r, "threshold", 1000)
	metrics := cardinalityExplorer.GetHighCardinalityMetrics(threshold)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"threshold": threshold,
		"count":     len(metrics),
		"metrics":   metrics,
	})
}

// handleCardinalityStats returns basic cardinality statistics
// GET /api/cardinality/stats
func handleCardinalityStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if cardinalityExplorer == nil {
		http.Error(w, "Cardinality explorer not configured", http.StatusServiceUnavailable)
		return
	}

	stats := cardinalityExplorer.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func parseQueryInt(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func parseQueryDuration(r *http.Request, name string, defaultVal time.Duration) time.Duration {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}
