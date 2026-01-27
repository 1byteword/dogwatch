package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/datashaping"
)

var dataShapingStore *datashaping.Store

// SetDataShapingStore sets the data shaping store
func SetDataShapingStore(store *datashaping.Store) {
	dataShapingStore = store
}

// RegisterDataShapingRoutes registers data shaping API routes
func RegisterDataShapingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/shaping/rules", handleShapingRules)
	mux.HandleFunc("/api/shaping/rules/", handleShapingRule)
	mux.HandleFunc("/api/shaping/stats", handleShapingStats)
	mux.HandleFunc("/api/shaping/test", handleShapingTest)
	mux.HandleFunc("/api/shaping/presets", handleShapingPresets)
}

// handleShapingRules handles listing and creating rules
// GET /api/shaping/rules - List all rules
// POST /api/shaping/rules - Create a new rule
func handleShapingRules(w http.ResponseWriter, r *http.Request) {
	if dataShapingStore == nil {
		http.Error(w, "Data shaping not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules := dataShapingStore.GetRules()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(rules),
			"rules": rules,
		})

	case http.MethodPost:
		var rule datashaping.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate required fields
		if rule.ID == "" {
			rule.ID = generateRuleID()
		}
		if rule.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if rule.DataType == "" {
			http.Error(w, "data_type is required", http.StatusBadRequest)
			return
		}
		if rule.Action == "" {
			http.Error(w, "action is required", http.StatusBadRequest)
			return
		}

		// Validate action
		switch rule.Action {
		case datashaping.ActionDrop, datashaping.ActionSample, datashaping.ActionAggregate, datashaping.ActionKeep, datashaping.ActionTransform:
			// Valid
		default:
			http.Error(w, "invalid action: "+string(rule.Action), http.StatusBadRequest)
			return
		}

		// Validate sample rate
		if rule.Action == datashaping.ActionSample && (rule.SampleRate <= 0 || rule.SampleRate > 1) {
			http.Error(w, "sample_rate must be between 0 and 1", http.StatusBadRequest)
			return
		}

		if err := dataShapingStore.CreateRule(&rule); err != nil {
			http.Error(w, "Failed to create rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleShapingRule handles individual rule operations
// GET /api/shaping/rules/{id} - Get a rule
// PUT /api/shaping/rules/{id} - Update a rule
// DELETE /api/shaping/rules/{id} - Delete a rule
// POST /api/shaping/rules/{id}/enable - Enable/disable a rule
func handleShapingRule(w http.ResponseWriter, r *http.Request) {
	if dataShapingStore == nil {
		http.Error(w, "Data shaping not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract rule ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/shaping/rules/")
	parts := strings.Split(path, "/")
	id := parts[0]

	if id == "" {
		http.Error(w, "Rule ID required", http.StatusBadRequest)
		return
	}

	// Check for sub-actions
	if len(parts) > 1 && parts[1] == "enable" {
		handleShapingRuleEnable(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rule := dataShapingStore.GetRule(id)
		if rule == nil {
			http.Error(w, "Rule not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var rule datashaping.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		rule.ID = id
		if err := dataShapingStore.UpdateRule(&rule); err != nil {
			http.Error(w, "Failed to update rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	case http.MethodDelete:
		if err := dataShapingStore.DeleteRule(id); err != nil {
			http.Error(w, "Failed to delete rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleShapingRuleEnable enables or disables a rule
// POST /api/shaping/rules/{id}/enable
func handleShapingRuleEnable(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := dataShapingStore.EnableRule(id, req.Enabled); err != nil {
		http.Error(w, "Failed to update rule: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"enabled": req.Enabled,
	})
}

// handleShapingStats returns data shaping statistics
// GET /api/shaping/stats
// POST /api/shaping/stats/reset - Reset statistics
func handleShapingStats(w http.ResponseWriter, r *http.Request) {
	if dataShapingStore == nil {
		http.Error(w, "Data shaping not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := dataShapingStore.GetStats()

		// Calculate savings
		var dropRate float64
		if stats.TotalProcessed > 0 {
			dropRate = float64(stats.TotalDropped) / float64(stats.TotalProcessed) * 100
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stats":           stats,
			"drop_rate_pct":   dropRate,
			"bytes_saved_mb":  float64(stats.BytesSavedEst) / (1024 * 1024),
			"bytes_saved_gb":  float64(stats.BytesSavedEst) / (1024 * 1024 * 1024),
			"cost_saved_est":  float64(stats.BytesSavedEst) / (1024 * 1024 * 1024) * 0.30, // $0.30/GB
		})

	case http.MethodPost:
		// Check for reset action
		if strings.HasSuffix(r.URL.Path, "/reset") {
			dataShapingStore.ResetStats()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleShapingTest tests a rule against sample data
// POST /api/shaping/test
func handleShapingTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if dataShapingStore == nil {
		http.Error(w, "Data shaping not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		DataType  datashaping.DataType `json:"data_type"`
		Name      string               `json:"name"`
		Tags      map[string]string    `json:"tags"`
		Level     string               `json:"level"`
		Service   string               `json:"service"`
		SizeBytes int                  `json:"size_bytes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.SizeBytes == 0 {
		req.SizeBytes = 100 // Default size estimate
	}

	decision := dataShapingStore.Evaluate(req.DataType, req.Name, req.Tags, req.Level, req.Service, req.SizeBytes)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"input":    req,
		"decision": decision,
		"would_keep": decision.Action == datashaping.ActionKeep ||
			decision.Action == datashaping.ActionTransform ||
			decision.Action == datashaping.ActionAggregate,
	})
}

// handleShapingPresets returns common rule presets
// GET /api/shaping/presets - List available presets
// POST /api/shaping/presets/{name} - Apply a preset
func handleShapingPresets(w http.ResponseWriter, r *http.Request) {
	if dataShapingStore == nil {
		http.Error(w, "Data shaping not configured", http.StatusServiceUnavailable)
		return
	}

	presets := map[string]struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Rules       []datashaping.Rule `json:"rules"`
	}{
		"drop-debug-logs": {
			Name:        "Drop Debug Logs",
			Description: "Drop all debug-level log entries",
			Rules: []datashaping.Rule{
				{
					ID:          "preset-drop-debug",
					Name:        "Drop debug logs",
					Description: "Drops all logs with level=debug",
					Enabled:     true,
					Priority:    50,
					DataType:    datashaping.DataTypeLog,
					Action:      datashaping.ActionDrop,
					LevelMatch:  "debug",
				},
			},
		},
		"sample-traces": {
			Name:        "Sample Traces at 10%",
			Description: "Keep only 10% of traces to reduce volume",
			Rules: []datashaping.Rule{
				{
					ID:          "preset-sample-traces",
					Name:        "Sample traces 10%",
					Description: "Keeps 10% of all traces",
					Enabled:     true,
					Priority:    100,
					DataType:    datashaping.DataTypeTrace,
					Action:      datashaping.ActionSample,
					SampleRate:  0.10,
				},
			},
		},
		"drop-health-checks": {
			Name:        "Drop Health Check Metrics",
			Description: "Drop metrics from health check endpoints",
			Rules: []datashaping.Rule{
				{
					ID:          "preset-drop-health",
					Name:        "Drop health check metrics",
					Description: "Drops metrics matching health/ready/live patterns",
					Enabled:     true,
					Priority:    50,
					DataType:    datashaping.DataTypeMetric,
					Action:      datashaping.ActionDrop,
					NamePattern: ".*(health|ready|live|ping).*",
				},
			},
		},
		"drop-high-cardinality-tags": {
			Name:        "Drop High Cardinality Tags",
			Description: "Remove tags that cause cardinality explosion",
			Rules: []datashaping.Rule{
				{
					ID:          "preset-drop-request-id",
					Name:        "Drop request_id tags",
					Description: "Removes request_id, trace_id, span_id tags from metrics",
					Enabled:     true,
					Priority:    25,
					DataType:    datashaping.DataTypeMetric,
					Action:      datashaping.ActionTransform,
					DropTags:    []string{"request_id", "trace_id", "span_id", "correlation_id"},
				},
			},
		},
		"sample-verbose-services": {
			Name:        "Sample Verbose Services",
			Description: "Sample data from high-volume services",
			Rules: []datashaping.Rule{
				{
					ID:           "preset-sample-verbose",
					Name:         "Sample verbose services",
					Description:  "50% sampling for services matching 'internal-*'",
					Enabled:      true,
					Priority:     75,
					DataType:     datashaping.DataTypeAll,
					Action:       datashaping.ActionSample,
					ServiceMatch: "^internal-.*",
					SampleRate:   0.50,
				},
			},
		},
	}

	switch r.Method {
	case http.MethodGet:
		// List presets
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presets)

	case http.MethodPost:
		// Apply preset
		presetName := strings.TrimPrefix(r.URL.Path, "/api/shaping/presets/")
		if presetName == "" || presetName == "presets" {
			http.Error(w, "Preset name required", http.StatusBadRequest)
			return
		}

		preset, ok := presets[presetName]
		if !ok {
			http.Error(w, "Unknown preset: "+presetName, http.StatusNotFound)
			return
		}

		// Apply all rules from preset
		var applied []string
		for _, rule := range preset.Rules {
			rule.CreatedAt = time.Now()
			rule.UpdatedAt = time.Now()
			if err := dataShapingStore.CreateRule(&rule); err != nil {
				http.Error(w, "Failed to apply preset: "+err.Error(), http.StatusInternalServerError)
				return
			}
			applied = append(applied, rule.ID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       true,
			"preset":        presetName,
			"rules_applied": applied,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// generateRuleID generates a unique rule ID
func generateRuleID() string {
	return fmt.Sprintf("rule-%d", time.Now().UnixNano())
}

// ApplyDataShaping evaluates and applies data shaping rules
// Returns true if the data should be kept, false if it should be dropped
func ApplyDataShaping(dataType datashaping.DataType, name string, tags map[string]string, level string, service string, sizeBytes int) (bool, *datashaping.Decision) {
	if dataShapingStore == nil {
		return true, nil
	}

	decision := dataShapingStore.Evaluate(dataType, name, tags, level, service, sizeBytes)

	switch decision.Action {
	case datashaping.ActionDrop:
		return false, &decision
	case datashaping.ActionKeep, datashaping.ActionTransform, datashaping.ActionAggregate:
		return true, &decision
	default:
		return true, &decision
	}
}

// ApplyMetricShaping applies data shaping to a metric
func ApplyMetricShaping(name string, tags map[string]string, sizeBytes int) (bool, map[string]string) {
	keep, decision := ApplyDataShaping(datashaping.DataTypeMetric, name, tags, "", "", sizeBytes)
	if !keep {
		return false, nil
	}

	// Apply transformations
	if decision != nil && decision.Action == datashaping.ActionTransform {
		tags = applyTagTransforms(tags, decision.DropTags, decision.AddTags)
	}

	return true, tags
}

// ApplyLogShaping applies data shaping to a log entry
func ApplyLogShaping(service, level string, tags map[string]string, sizeBytes int) bool {
	keep, _ := ApplyDataShaping(datashaping.DataTypeLog, "", tags, level, service, sizeBytes)
	return keep
}

// ApplyTraceShaping applies data shaping to a trace/span
func ApplyTraceShaping(service string, tags map[string]string, sizeBytes int) bool {
	keep, _ := ApplyDataShaping(datashaping.DataTypeTrace, "", tags, "", service, sizeBytes)
	return keep
}

func applyTagTransforms(tags map[string]string, dropTags []string, addTags map[string]string) map[string]string {
	result := make(map[string]string)

	// Copy existing tags except dropped ones
	for k, v := range tags {
		drop := false
		for _, dt := range dropTags {
			if k == dt {
				drop = true
				break
			}
		}
		if !drop {
			result[k] = v
		}
	}

	// Add new tags
	for k, v := range addTags {
		result[k] = v
	}

	return result
}
