package web

import (
	"encoding/json"
	"net/http"
	"time"

	"dogwatch/internal/storage"
)

var timeSeriesOptimizer *storage.TimeSeriesOptimizer

// SetTimeSeriesOptimizer sets the time-series optimizer instance
func SetTimeSeriesOptimizer(o *storage.TimeSeriesOptimizer) {
	timeSeriesOptimizer = o
}

// RegisterTimeSeriesRoutes registers time-series optimizer API routes
func RegisterTimeSeriesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/timeseries/stats", handleTimeSeriesStats)
	mux.HandleFunc("/api/timeseries/partitions", handleTimeSeriesPartitions)
	mux.HandleFunc("/api/timeseries/downsample", handleDownsampleRules)
	mux.HandleFunc("/api/timeseries/downsample/run", handleRunDownsample)
	mux.HandleFunc("/api/timeseries/flush", handleFlushBatches)
	mux.HandleFunc("/api/timeseries/optimize", handleOptimizeTable)
}

// handleTimeSeriesStats returns optimizer statistics
// GET /api/timeseries/stats
func handleTimeSeriesStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	stats := timeSeriesOptimizer.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleTimeSeriesPartitions returns partition information
// GET /api/timeseries/partitions?table=metrics
func handleTimeSeriesPartitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	table := r.URL.Query().Get("table")
	partitions := timeSeriesOptimizer.ListPartitions(table)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"table":      table,
		"partitions": partitions,
		"count":      len(partitions),
	})
}

// handleDownsampleRules manages downsampling rules
// GET /api/timeseries/downsample - list rules
// POST /api/timeseries/downsample - add rule
// DELETE /api/timeseries/downsample?source=table_name - remove rule
func handleDownsampleRules(w http.ResponseWriter, r *http.Request) {
	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules := timeSeriesOptimizer.ListDownsampleRules()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rules": rules,
			"count": len(rules),
		})

	case http.MethodPost:
		var req DownsampleRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		rule := storage.DownsampleRule{
			SourceTable:     req.SourceTable,
			TargetTable:     req.TargetTable,
			SourceInterval:  parseDurationDefault(req.SourceInterval, time.Minute),
			TargetInterval:  parseDurationDefault(req.TargetInterval, time.Hour),
			AggregateColumn: req.AggregateColumn,
			AggregateFunc:   req.AggregateFunc,
			GroupByColumns:  req.GroupByColumns,
			Enabled:         true,
		}

		timeSeriesOptimizer.AddDownsampleRule(rule)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "created",
			"rule":   rule,
		})

	case http.MethodDelete:
		source := r.URL.Query().Get("source")
		if source == "" {
			http.Error(w, "source parameter required", http.StatusBadRequest)
			return
		}

		timeSeriesOptimizer.RemoveDownsampleRule(source)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "deleted",
			"source": source,
		})

	case http.MethodPatch:
		// Enable/disable a rule
		var req struct {
			SourceTable string `json:"source_table"`
			Enabled     bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		timeSeriesOptimizer.SetDownsampleRuleEnabled(req.SourceTable, req.Enabled)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "updated",
			"source":  req.SourceTable,
			"enabled": req.Enabled,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRunDownsample triggers immediate downsampling
// POST /api/timeseries/downsample/run
func handleRunDownsample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	if err := timeSeriesOptimizer.RunDownsampling(r.Context()); err != nil {
		http.Error(w, "Downsampling failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "completed",
		"message": "Downsampling executed successfully",
	})
}

// handleFlushBatches triggers immediate batch flush
// POST /api/timeseries/flush
func handleFlushBatches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	timeSeriesOptimizer.FlushBatches()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "flushed",
		"message": "All pending batches flushed",
	})
}

// handleOptimizeTable applies time-series optimizations to a table
// POST /api/timeseries/optimize
func handleOptimizeTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if timeSeriesOptimizer == nil {
		http.Error(w, "Time-series optimizer not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Table           string `json:"table"`
		TimestampColumn string `json:"timestamp_column"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Table == "" {
		http.Error(w, "table required", http.StatusBadRequest)
		return
	}
	if req.TimestampColumn == "" {
		req.TimestampColumn = "timestamp"
	}

	if err := timeSeriesOptimizer.OptimizeForTimeSeries(req.Table, req.TimestampColumn); err != nil {
		http.Error(w, "Optimization failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":           "optimized",
		"table":            req.Table,
		"timestamp_column": req.TimestampColumn,
		"optimizations": []string{
			"time_index_created",
			"wal_mode_enabled",
			"mmap_enabled",
		},
	})
}

// DownsampleRuleRequest is the request body for adding a downsample rule
type DownsampleRuleRequest struct {
	SourceTable     string   `json:"source_table"`
	TargetTable     string   `json:"target_table"`
	SourceInterval  string   `json:"source_interval"` // e.g., "1m"
	TargetInterval  string   `json:"target_interval"` // e.g., "1h"
	AggregateColumn string   `json:"aggregate_column"`
	AggregateFunc   string   `json:"aggregate_func"` // avg, sum, min, max, count
	GroupByColumns  []string `json:"group_by_columns,omitempty"`
}

func parseDurationDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}
