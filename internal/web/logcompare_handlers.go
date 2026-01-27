package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dogwatch/internal/logcompare"
	"dogwatch/internal/logs"
)

// Package-level store for log comparison
var logCompareStore *logs.Store

// SetLogCompareStore sets the log store for comparison operations
func SetLogCompareStore(store *logs.Store) {
	logCompareStore = store
}

// RegisterLogCompareRoutes registers log comparison API routes
func RegisterLogCompareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/logs/compare", handleLogCompare)
	mux.HandleFunc("/api/logs/compare/quick", handleQuickCompare)
	mux.HandleFunc("/api/logs/compare/presets", handleComparePresets)
}

// handleLogCompare performs a log comparison between two time periods
// POST /api/logs/compare
func handleLogCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logCompareStore == nil {
		http.Error(w, "Log store not configured", http.StatusServiceUnavailable)
		return
	}

	var req logcompare.CompareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.BaselineStart.IsZero() || req.BaselineEnd.IsZero() {
		http.Error(w, "baseline_start and baseline_end are required", http.StatusBadRequest)
		return
	}
	if req.CompareStart.IsZero() || req.CompareEnd.IsZero() {
		http.Error(w, "compare_start and compare_end are required", http.StatusBadRequest)
		return
	}

	// Fetch logs for baseline period
	baselineLogs, err := fetchLogs(logCompareStore, req.BaselineStart, req.BaselineEnd, req.Service, req.Level)
	if err != nil {
		http.Error(w, "Failed to fetch baseline logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch logs for comparison period
	compareLogs, err := fetchLogs(logCompareStore, req.CompareStart, req.CompareEnd, req.Service, req.Level)
	if err != nil {
		http.Error(w, "Failed to fetch comparison logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Perform comparison
	comparer := logcompare.NewComparer()
	result := comparer.Compare(baselineLogs, compareLogs, req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleQuickCompare performs a quick comparison using preset time ranges
// GET /api/logs/compare/quick?preset=last_hour_vs_previous&service=X
func handleQuickCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logCompareStore == nil {
		http.Error(w, "Log store not configured", http.StatusServiceUnavailable)
		return
	}

	preset := r.URL.Query().Get("preset")
	if preset == "" {
		preset = "last_hour_vs_previous"
	}

	service := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")

	req := buildPresetRequest(preset, service, level)

	// Fetch logs for baseline period
	baselineLogs, err := fetchLogs(logCompareStore, req.BaselineStart, req.BaselineEnd, req.Service, req.Level)
	if err != nil {
		http.Error(w, "Failed to fetch baseline logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch logs for comparison period
	compareLogs, err := fetchLogs(logCompareStore, req.CompareStart, req.CompareEnd, req.Service, req.Level)
	if err != nil {
		http.Error(w, "Failed to fetch comparison logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Perform comparison
	comparer := logcompare.NewComparer()
	result := comparer.Compare(baselineLogs, compareLogs, req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"preset": preset,
		"result": result,
	})
}

// handleComparePresets returns available comparison presets
// GET /api/logs/compare/presets
func handleComparePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	presets := []map[string]interface{}{
		{
			"id":          "last_hour_vs_previous",
			"name":        "Last Hour vs Previous Hour",
			"description": "Compare the last hour against the hour before that",
			"baseline":    "1-2 hours ago",
			"compare":     "0-1 hours ago",
		},
		{
			"id":          "last_15min_vs_previous",
			"name":        "Last 15 Minutes vs Previous 15 Minutes",
			"description": "Quick comparison for recent issues",
			"baseline":    "15-30 minutes ago",
			"compare":     "0-15 minutes ago",
		},
		{
			"id":          "today_vs_yesterday",
			"name":        "Today vs Yesterday (Same Time)",
			"description": "Compare today's logs against the same time window yesterday",
			"baseline":    "Yesterday same time",
			"compare":     "Today",
		},
		{
			"id":          "last_6h_vs_previous",
			"name":        "Last 6 Hours vs Previous 6 Hours",
			"description": "Compare the last 6 hours against the previous 6 hours",
			"baseline":    "6-12 hours ago",
			"compare":     "0-6 hours ago",
		},
		{
			"id":          "this_week_vs_last",
			"name":        "This Week vs Last Week (Same Day)",
			"description": "Compare this day's logs against the same day last week",
			"baseline":    "Last week same day",
			"compare":     "Today",
		},
		{
			"id":          "deploy_window",
			"name":        "Post-Deploy vs Pre-Deploy",
			"description": "Compare 30 minutes after deploy against 30 minutes before",
			"baseline":    "30 min before reference time",
			"compare":     "30 min after reference time",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"presets": presets,
	})
}

func buildPresetRequest(preset, service, level string) logcompare.CompareRequest {
	now := time.Now()

	req := logcompare.CompareRequest{
		Service: service,
		Level:   level,
	}

	switch preset {
	case "last_hour_vs_previous":
		req.CompareStart = now.Add(-1 * time.Hour)
		req.CompareEnd = now
		req.BaselineStart = now.Add(-2 * time.Hour)
		req.BaselineEnd = now.Add(-1 * time.Hour)

	case "last_15min_vs_previous":
		req.CompareStart = now.Add(-15 * time.Minute)
		req.CompareEnd = now
		req.BaselineStart = now.Add(-30 * time.Minute)
		req.BaselineEnd = now.Add(-15 * time.Minute)

	case "today_vs_yesterday":
		// Get start of current hour
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		req.CompareStart = todayStart
		req.CompareEnd = now
		// Yesterday same window
		duration := now.Sub(todayStart)
		req.BaselineStart = todayStart.AddDate(0, 0, -1)
		req.BaselineEnd = req.BaselineStart.Add(duration)

	case "last_6h_vs_previous":
		req.CompareStart = now.Add(-6 * time.Hour)
		req.CompareEnd = now
		req.BaselineStart = now.Add(-12 * time.Hour)
		req.BaselineEnd = now.Add(-6 * time.Hour)

	case "this_week_vs_last":
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		req.CompareStart = todayStart
		req.CompareEnd = now
		duration := now.Sub(todayStart)
		req.BaselineStart = todayStart.AddDate(0, 0, -7)
		req.BaselineEnd = req.BaselineStart.Add(duration)

	case "deploy_window":
		// Default: use 1 hour ago as the reference point
		ref := now.Add(-1 * time.Hour)
		req.BaselineStart = ref.Add(-30 * time.Minute)
		req.BaselineEnd = ref
		req.CompareStart = ref
		req.CompareEnd = ref.Add(30 * time.Minute)

	default:
		// Default to last hour vs previous
		req.CompareStart = now.Add(-1 * time.Hour)
		req.CompareEnd = now
		req.BaselineStart = now.Add(-2 * time.Hour)
		req.BaselineEnd = now.Add(-1 * time.Hour)
	}

	return req
}

func fetchLogs(store *logs.Store, start, end time.Time, service, level string) ([]logcompare.LogEntry, error) {
	query := logs.SearchQuery{
		StartTime: start,
		EndTime:   end,
		Limit:     10000, // Max logs to fetch for comparison
	}

	if service != "" {
		query.Service = service
	}
	if level != "" {
		query.Level = logs.LogLevel(level)
	}

	results, err := store.Search(query)
	if err != nil {
		return nil, err
	}

	var entries []logcompare.LogEntry
	for _, entry := range results.Entries {
		entries = append(entries, logcompare.LogEntry{
			Timestamp: entry.Timestamp,
			Level:     string(entry.Level),
			Service:   entry.Service,
			Message:   entry.Message,
		})
	}

	return entries, nil
}

// LogCompareHandler provides programmatic access to log comparison
type LogCompareHandler struct {
	store *logs.Store
}

// NewLogCompareHandler creates a new log compare handler
func NewLogCompareHandler(store *logs.Store) *LogCompareHandler {
	return &LogCompareHandler{store: store}
}

// Compare performs a log comparison
func (h *LogCompareHandler) Compare(req logcompare.CompareRequest) (*logcompare.CompareResult, error) {
	if h.store == nil {
		return nil, fmt.Errorf("log store not available")
	}

	baselineLogs, err := fetchLogs(h.store, req.BaselineStart, req.BaselineEnd, req.Service, req.Level)
	if err != nil {
		return nil, fmt.Errorf("fetch baseline logs: %w", err)
	}

	compareLogs, err := fetchLogs(h.store, req.CompareStart, req.CompareEnd, req.Service, req.Level)
	if err != nil {
		return nil, fmt.Errorf("fetch comparison logs: %w", err)
	}

	comparer := logcompare.NewComparer()
	return comparer.Compare(baselineLogs, compareLogs, req), nil
}

// QuickCompare performs a quick comparison using a preset
func (h *LogCompareHandler) QuickCompare(preset, service, level string) (*logcompare.CompareResult, error) {
	req := buildPresetRequest(preset, service, level)
	return h.Compare(req)
}
