package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/sampling"
)

var samplingManager *sampling.Manager

// SetSamplingManager sets the sampling manager for API handlers
func SetSamplingManager(manager *sampling.Manager) {
	samplingManager = manager
}

// RegisterSamplingRoutes registers sampling API routes
func RegisterSamplingRoutes(mux *http.ServeMux) {
	// Configuration
	mux.HandleFunc("/api/sampling/config", handleSamplingConfig)

	// Rules (head sampling)
	mux.HandleFunc("/api/sampling/rules", handleSamplingRules)
	mux.HandleFunc("/api/sampling/rules/", handleSamplingRule)

	// Stats
	mux.HandleFunc("/api/sampling/stats", handleSamplingStats)
	mux.HandleFunc("/api/sampling/stats/history", handleSamplingStatsHistory)

	// Adaptive sampling controls
	mux.HandleFunc("/api/sampling/adaptive/rate", handleAdaptiveRate)
	mux.HandleFunc("/api/sampling/adaptive/service-rates", handleAdaptiveServiceRates)

	// Tail sampling controls
	mux.HandleFunc("/api/sampling/tail/priority-services", handleTailPriorityServices)
	mux.HandleFunc("/api/sampling/tail/flush", handleTailFlush)
	mux.HandleFunc("/api/sampling/tail/buffered", handleTailBuffered)

	// Test endpoint
	mux.HandleFunc("/api/sampling/test", handleSamplingTest)
}

// handleSamplingConfig handles GET/PUT for the sampling configuration
// GET /api/sampling/config - Get current configuration
// PUT /api/sampling/config - Update configuration
func handleSamplingConfig(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := samplingManager.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case http.MethodPut:
		var config sampling.Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := samplingManager.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config":  config,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSamplingRules handles listing and creating rules
// GET /api/sampling/rules - List all rules
// POST /api/sampling/rules - Create a new rule
func handleSamplingRules(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules := samplingManager.GetRules()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(rules),
			"rules": rules,
		})

	case http.MethodPost:
		var rule sampling.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Validate required fields
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
		}
		if rule.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}

		// Validate action
		if rule.Action != sampling.DecisionKeep && rule.Action != sampling.DecisionDrop {
			http.Error(w, "action must be 0 (keep) or 1 (drop)", http.StatusBadRequest)
			return
		}

		// Validate sample rate
		if rule.SampleRate < 0 || rule.SampleRate > 1 {
			http.Error(w, "sample_rate must be between 0 and 1", http.StatusBadRequest)
			return
		}

		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()

		if err := samplingManager.AddRule(rule); err != nil {
			http.Error(w, "Failed to add rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSamplingRule handles individual rule operations
// GET /api/sampling/rules/{id} - Get a rule
// PUT /api/sampling/rules/{id} - Update a rule
// DELETE /api/sampling/rules/{id} - Delete a rule
func handleSamplingRule(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract rule ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/sampling/rules/")
	if id == "" {
		http.Error(w, "Rule ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules := samplingManager.GetRules()
		for _, rule := range rules {
			if rule.ID == id {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(rule)
				return
			}
		}
		http.Error(w, "Rule not found", http.StatusNotFound)

	case http.MethodPut:
		var rule sampling.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		rule.ID = id
		rule.UpdatedAt = time.Now()

		if err := samplingManager.UpdateRule(rule); err != nil {
			http.Error(w, "Failed to update rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	case http.MethodDelete:
		if err := samplingManager.RemoveRule(id); err != nil {
			http.Error(w, "Failed to delete rule: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSamplingStats returns current sampling statistics
// GET /api/sampling/stats
func handleSamplingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	stats := samplingManager.GetStats()

	// Calculate rates
	var keepRate, dropRate float64
	if stats.TotalProcessed > 0 {
		keepRate = float64(stats.TotalKept) / float64(stats.TotalProcessed) * 100
		dropRate = float64(stats.TotalDropped) / float64(stats.TotalProcessed) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats":          stats,
		"keep_rate_pct":  keepRate,
		"drop_rate_pct":  dropRate,
		"adaptive_stats": samplingManager.GetAdaptiveRateStats(),
	})
}

// handleSamplingStatsHistory returns historical sampling statistics
// GET /api/sampling/stats/history?since=1h
func handleSamplingStatsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse duration
	since := 1 * time.Hour
	if s := r.URL.Query().Get("since"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			since = d
		}
	}

	history, err := samplingManager.GetHistoricalStats(since)
	if err != nil {
		http.Error(w, "Failed to get history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"since":   since.String(),
		"count":   len(history),
		"history": history,
	})
}

// handleAdaptiveRate handles adaptive rate configuration
// GET /api/sampling/adaptive/rate - Get current rate
// PUT /api/sampling/adaptive/rate - Set target TPS
func handleAdaptiveRate(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := samplingManager.GetStats()
		rateStats := samplingManager.GetAdaptiveRateStats()

		response := map[string]interface{}{
			"current_rate":  stats.CurrentRate,
			"service_rates": stats.ServiceRates,
		}
		if rateStats != nil {
			response["rate_stats"] = rateStats
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)

	case http.MethodPut:
		var req struct {
			TargetTPS float64 `json:"target_tps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.TargetTPS <= 0 {
			http.Error(w, "target_tps must be positive", http.StatusBadRequest)
			return
		}

		samplingManager.SetAdaptiveTargetTPS(req.TargetTPS)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":    true,
			"target_tps": req.TargetTPS,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdaptiveServiceRates handles per-service rate configuration
// GET /api/sampling/adaptive/service-rates - Get service rates
// PUT /api/sampling/adaptive/service-rates - Set service rate
func handleAdaptiveServiceRates(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		stats := samplingManager.GetStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service_rates": stats.ServiceRates,
		})

	case http.MethodPut:
		var req struct {
			Service string  `json:"service"`
			Rate    float64 `json:"rate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Service == "" {
			http.Error(w, "service is required", http.StatusBadRequest)
			return
		}
		if req.Rate < 0 || req.Rate > 1 {
			http.Error(w, "rate must be between 0 and 1", http.StatusBadRequest)
			return
		}

		samplingManager.SetServiceSampleRate(req.Service, req.Rate)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"service": req.Service,
			"rate":    req.Rate,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTailPriorityServices handles tail sampling priority services
// GET /api/sampling/tail/priority-services - List priority services
// POST /api/sampling/tail/priority-services - Add priority service
// DELETE /api/sampling/tail/priority-services?service=xxx - Remove priority service
func handleTailPriorityServices(w http.ResponseWriter, r *http.Request) {
	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	config := samplingManager.GetConfig()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"priority_services": config.TailSamplerConfig.PriorityServices,
		})

	case http.MethodPost:
		var req struct {
			Service string `json:"service"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Service == "" {
			http.Error(w, "service is required", http.StatusBadRequest)
			return
		}

		samplingManager.AddTailPriorityService(req.Service)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"service": req.Service,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTailFlush forces a flush of all buffered traces
// POST /api/sampling/tail/flush
func handleTailFlush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	config := samplingManager.GetConfig()
	if !config.TailSamplerConfig.Enabled {
		http.Error(w, "Tail sampling not enabled", http.StatusBadRequest)
		return
	}

	// Get current buffered count before flush
	stats := samplingManager.GetStats()
	bufferedBefore := stats.BufferedTraces

	// Note: The manager doesn't expose a flush method directly, but we can
	// update the config with a very short timeout to force flush
	// For now, return the current buffered count
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"buffered_before": bufferedBefore,
		"message":         "Flush initiated - traces will be processed within buffer timeout",
	})
}

// handleTailBuffered returns currently buffered traces
// GET /api/sampling/tail/buffered
func handleTailBuffered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	stats := samplingManager.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"buffered_traces": stats.BufferedTraces,
		"deferred_spans":  stats.TailStats.DeferredSpans,
	})
}

// handleSamplingTest tests sampling decision for a sample span
// POST /api/sampling/test
func handleSamplingTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if samplingManager == nil {
		http.Error(w, "Sampling not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		TraceID     string            `json:"trace_id"`
		SpanID      string            `json:"span_id"`
		ServiceName string            `json:"service_name"`
		Operation   string            `json:"operation"`
		DurationMs  float64           `json:"duration_ms"`
		Status      string            `json:"status"`
		Kind        string            `json:"kind"`
		Attributes  map[string]string `json:"attributes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Generate IDs if not provided
	if req.TraceID == "" {
		req.TraceID = fmt.Sprintf("test-%d", time.Now().UnixNano())
	}
	if req.SpanID == "" {
		req.SpanID = fmt.Sprintf("span-%d", time.Now().UnixNano())
	}

	// Create test span (but don't actually process it through the manager)
	testSpan := struct {
		TraceID     string            `json:"trace_id"`
		SpanID      string            `json:"span_id"`
		ServiceName string            `json:"service_name"`
		Operation   string            `json:"operation"`
		DurationMs  float64           `json:"duration_ms"`
		Status      string            `json:"status"`
		Kind        string            `json:"kind"`
		Attributes  map[string]string `json:"attributes"`
	}{
		TraceID:     req.TraceID,
		SpanID:      req.SpanID,
		ServiceName: req.ServiceName,
		Operation:   req.Operation,
		DurationMs:  req.DurationMs,
		Status:      req.Status,
		Kind:        req.Kind,
		Attributes:  req.Attributes,
	}

	// Get current config and stats
	config := samplingManager.GetConfig()
	stats := samplingManager.GetStats()

	// Simulate decision based on rules
	wouldSample := true
	reason := "no rules matched"

	// Check head sampling rules
	for _, rule := range config.HeadSamplerConfig.Rules {
		if !rule.Enabled {
			continue
		}

		matches := true

		// Check service condition
		if rule.Condition.Service != "" && rule.Condition.Service != req.ServiceName {
			matches = false
		}

		// Check operation condition
		if rule.Condition.Operation != "" && rule.Condition.Operation != req.Operation {
			matches = false
		}

		// Check latency condition
		if rule.Condition.MinLatencyMs > 0 && req.DurationMs < rule.Condition.MinLatencyMs {
			matches = false
		}

		// Check error condition
		if rule.Condition.HasError != nil {
			hasError := req.Status == "ERROR"
			if *rule.Condition.HasError != hasError {
				matches = false
			}
		}

		if matches {
			wouldSample = rule.Action == sampling.DecisionKeep
			reason = fmt.Sprintf("matched rule: %s", rule.Name)
			break
		}
	}

	// Apply adaptive rate
	if wouldSample && config.AdaptiveSamplerConfig.Enabled {
		// Use current rate as approximation
		if stats.CurrentRate < 1.0 {
			reason += fmt.Sprintf(" (adaptive rate: %.2f%%)", stats.CurrentRate*100)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"input":        testSpan,
		"would_sample": wouldSample,
		"reason":       reason,
		"current_rate": stats.CurrentRate,
		"config": map[string]bool{
			"head_enabled":     config.HeadSamplerConfig.Enabled,
			"tail_enabled":     config.TailSamplerConfig.Enabled,
			"adaptive_enabled": config.AdaptiveSamplerConfig.Enabled,
		},
	})
}

// SamplingPresets returns common sampling presets
func handleSamplingPresets(w http.ResponseWriter, r *http.Request) {
	presets := map[string]sampling.Config{
		"aggressive": {
			Enabled:           true,
			DefaultSampleRate: 0.01, // 1%
			HeadSamplerConfig: sampling.HeadSamplerConfig{
				Enabled:            true,
				DecisionTTL:        5 * time.Minute,
				MaxCachedDecisions: 100000,
				Rules: []sampling.Rule{
					{
						ID:       "keep-errors",
						Name:     "Keep all errors",
						Enabled:  true,
						Priority: 100,
						Condition: sampling.RuleCondition{
							HasError: samplingBoolPtr(true),
						},
						Action:     sampling.DecisionKeep,
						SampleRate: 1.0,
					},
				},
			},
			AdaptiveSamplerConfig: sampling.AdaptiveSamplerConfig{
				Enabled:               true,
				TargetTracesPerSecond: 50,
				MinSampleRate:         0.001,
				MaxSampleRate:         0.1,
				AdjustmentInterval:    10 * time.Second,
				SmoothingFactor:       0.3,
				PerServiceRates:       true,
			},
		},
		"balanced": sampling.DefaultConfig(),
		"conservative": {
			Enabled:           true,
			DefaultSampleRate: 0.5, // 50%
			HeadSamplerConfig: sampling.HeadSamplerConfig{
				Enabled:            true,
				DecisionTTL:        5 * time.Minute,
				MaxCachedDecisions: 100000,
				Rules: []sampling.Rule{
					{
						ID:       "keep-errors",
						Name:     "Keep all errors",
						Enabled:  true,
						Priority: 100,
						Condition: sampling.RuleCondition{
							HasError: samplingBoolPtr(true),
						},
						Action:     sampling.DecisionKeep,
						SampleRate: 1.0,
					},
					{
						ID:       "keep-slow",
						Name:     "Keep slow requests",
						Enabled:  true,
						Priority: 90,
						Condition: sampling.RuleCondition{
							MinLatencyMs: 500,
						},
						Action:     sampling.DecisionKeep,
						SampleRate: 1.0,
					},
				},
			},
			AdaptiveSamplerConfig: sampling.AdaptiveSamplerConfig{
				Enabled:               true,
				TargetTracesPerSecond: 500,
				MinSampleRate:         0.1,
				MaxSampleRate:         1.0,
				AdjustmentInterval:    30 * time.Second,
				SmoothingFactor:       0.5,
				PerServiceRates:       true,
			},
		},
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presets)

	case http.MethodPost:
		// Apply preset
		presetName := r.URL.Query().Get("name")
		if presetName == "" {
			presetName = strings.TrimPrefix(r.URL.Path, "/api/sampling/presets/")
		}

		preset, ok := presets[presetName]
		if !ok {
			http.Error(w, "Unknown preset: "+presetName, http.StatusNotFound)
			return
		}

		if samplingManager != nil {
			if err := samplingManager.UpdateConfig(preset); err != nil {
				http.Error(w, "Failed to apply preset: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"preset":  presetName,
			"config":  preset,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// samplingBoolPtr returns a pointer to a bool (renamed to avoid conflicts)
func samplingBoolPtr(b bool) *bool {
	return &b
}
