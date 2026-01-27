package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/usage"
)

var usageTracker *usage.Tracker

// SetUsageTracker sets the usage tracker instance
func SetUsageTracker(t *usage.Tracker) {
	usageTracker = t
}

// RegisterUsageRoutes registers usage analytics API routes
func RegisterUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/usage/report", handleUsageReport)
	mux.HandleFunc("/api/usage/top", handleUsageTop)
	mux.HandleFunc("/api/usage/wasted", handleUsageWasted)
	mux.HandleFunc("/api/usage/stats", handleUsageStats)
	mux.HandleFunc("/api/usage/item", handleUsageItem)
	mux.HandleFunc("/api/usage/events", handleUsageEvents)
	mux.HandleFunc("/api/usage/inventory", handleUsageInventory)
}

// handleUsageReport returns comprehensive usage analysis
// GET /api/usage/report?period=168h (default: 720h = 30 days)
func handleUsageReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	period := parseDuration(r, "period", 30*24*time.Hour)
	report := usageTracker.GetReport(period)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleUsageTop returns most queried data items
// GET /api/usage/top?type=metric&limit=20
func handleUsageTop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	dataType := usage.DataType(r.URL.Query().Get("type"))
	limit := parseInt(r, "limit", 20)

	top := usageTracker.GetTopQueried(dataType, limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":  dataType,
		"limit": limit,
		"items": top,
	})
}

// handleUsageWasted returns data that isn't being queried
// GET /api/usage/wasted?period=720h
func handleUsageWasted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	period := parseDuration(r, "period", 30*24*time.Hour)
	wasted := usageTracker.GetWastedData(period)

	// Calculate total potential savings
	var totalSavings float64
	for _, w := range wasted {
		totalSavings += w.EstimatedCost
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"period":                period.String(),
		"wasted_items":          len(wasted),
		"total_savings_monthly": totalSavings,
		"items":                 wasted,
	})
}

// handleUsageStats returns basic usage statistics
// GET /api/usage/stats
func handleUsageStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	stats := usageTracker.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleUsageItem returns usage stats for a specific item
// GET /api/usage/item?type=metric&name=http_requests_total
func handleUsageItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	dataType := usage.DataType(r.URL.Query().Get("type"))
	name := r.URL.Query().Get("name")

	if dataType == "" || name == "" {
		http.Error(w, "type and name parameters required", http.StatusBadRequest)
		return
	}

	stats := usageTracker.GetStats(dataType, name)
	if stats == nil {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleUsageEvents returns recent query events
// GET /api/usage/events?limit=100
func handleUsageEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	limit := parseInt(r, "limit", 100)
	events := usageTracker.GetRecentEvents(limit)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(events),
		"events": events,
	})
}

// handleUsageInventory records data inventory (POST) or returns inventory summary (GET)
// POST /api/usage/inventory - Record inventory item
// GET /api/usage/inventory - Get inventory summary
func handleUsageInventory(w http.ResponseWriter, r *http.Request) {
	if usageTracker == nil {
		http.Error(w, "Usage tracking not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := usageTracker.Stats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"inventory_items": stats["inventory_items"],
			"tracked_items":   stats["tracked_items"],
		})

	case http.MethodPost:
		var req struct {
			Type       string            `json:"type"`
			Name       string            `json:"name"`
			Tags       map[string]string `json:"tags"`
			DataPoints int64             `json:"data_points"`
			SizeBytes  int64             `json:"size_bytes"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Type == "" || req.Name == "" {
			http.Error(w, "type and name are required", http.StatusBadRequest)
			return
		}

		usageTracker.RecordDataItem(usage.DataType(req.Type), req.Name, req.Tags, req.DataPoints, req.SizeBytes)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// TrackQuery is a helper to track queries from handlers
func TrackQuery(dataType usage.DataType, name string, source string, resultSize int, duration time.Duration) {
	if usageTracker != nil {
		usageTracker.RecordQuery(usage.QueryEvent{
			Timestamp:  time.Now(),
			DataType:   dataType,
			Name:       name,
			Source:     source,
			ResultSize: resultSize,
			Duration:   duration,
		})
	}
}

// TrackMetricQuery tracks a metric query
func TrackMetricQuery(name string, source string, resultSize int) {
	TrackQuery(usage.DataTypeMetric, name, source, resultSize, 0)
}

// TrackLogQuery tracks a log query
func TrackLogQuery(service string, source string, resultSize int) {
	TrackQuery(usage.DataTypeLog, service, source, resultSize, 0)
}

// TrackTraceQuery tracks a trace query
func TrackTraceQuery(service string, source string, resultSize int) {
	TrackQuery(usage.DataTypeTrace, service, source, resultSize, 0)
}

func parseDuration(r *http.Request, name string, defaultVal time.Duration) time.Duration {
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

func parseInt(r *http.Request, name string, defaultVal int) int {
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
