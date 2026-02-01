package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/sampling"
)

var intelligentSampler *sampling.IntelligentSampler

// SetIntelligentSampler sets the intelligent sampler for API handlers
func SetIntelligentSampler(sampler *sampling.IntelligentSampler) {
	intelligentSampler = sampler
}

// RegisterIntelligentSamplingRoutes registers intelligent sampling API routes
func RegisterIntelligentSamplingRoutes(mux *http.ServeMux) {
	// Configuration
	mux.HandleFunc("/api/sampling/intelligent/config", handleIntelligentConfig)

	// Stats and monitoring
	mux.HandleFunc("/api/sampling/intelligent/stats", handleIntelligentStats)
	mux.HandleFunc("/api/sampling/intelligent/cost", handleIntelligentCost)
	mux.HandleFunc("/api/sampling/intelligent/patterns", handleIntelligentPatterns)

	// Retroactive sampling
	mux.HandleFunc("/api/sampling/intelligent/retroactive", handleRetroactiveConfig)
	mux.HandleFunc("/api/sampling/intelligent/decisions", handleRecentDecisions)

	// Anomaly detection
	mux.HandleFunc("/api/sampling/intelligent/anomaly", handleAnomalyConfig)
	mux.HandleFunc("/api/sampling/intelligent/anomaly/test", handleAnomalyTest)

	// Budget control
	mux.HandleFunc("/api/sampling/intelligent/budget", handleBudgetConfig)

	// Learning mode
	mux.HandleFunc("/api/sampling/intelligent/learning", handleLearningConfig)
	mux.HandleFunc("/api/sampling/intelligent/learning/rates", handleLearnedRates)
}

// handleIntelligentConfig handles GET/PUT for intelligent sampler configuration
// GET /api/sampling/intelligent/config - Get current configuration
// PUT /api/sampling/intelligent/config - Update configuration
func handleIntelligentConfig(w http.ResponseWriter, r *http.Request) {
	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := intelligentSampler.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case http.MethodPut:
		var config sampling.IntelligentSamplerConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		intelligentSampler.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config":  config,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleIntelligentStats returns intelligent sampler statistics
// GET /api/sampling/intelligent/stats
func handleIntelligentStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	stats := intelligentSampler.GetStats()

	// Calculate additional metrics
	var keepRate, dropRate, deferRate float64
	if stats.TotalSpans > 0 {
		keepRate = float64(stats.KeptSpans) / float64(stats.TotalSpans) * 100
		dropRate = float64(stats.DroppedSpans) / float64(stats.TotalSpans) * 100
		deferRate = float64(stats.DeferredSpans) / float64(stats.TotalSpans) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats":          stats,
		"keep_rate_pct":  keepRate,
		"drop_rate_pct":  dropRate,
		"defer_rate_pct": deferRate,
	})
}

// handleIntelligentCost returns cost breakdown and budget information
// GET /api/sampling/intelligent/cost
func handleIntelligentCost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	breakdown := intelligentSampler.GetCostBreakdown()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(breakdown)
}

// handleIntelligentPatterns returns learned patterns
// GET /api/sampling/intelligent/patterns
func handleIntelligentPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	patterns := intelligentSampler.GetLearnedPatterns()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"patterns": patterns,
	})
}

// handleRetroactiveConfig handles retroactive sampling configuration
// GET /api/sampling/intelligent/retroactive - Get retroactive config
// PUT /api/sampling/intelligent/retroactive - Update retroactive config
func handleRetroactiveConfig(w http.ResponseWriter, r *http.Request) {
	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := intelligentSampler.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":            config.RetroactiveEnabled,
			"window_size":        config.RetroactiveWindowSize,
			"window_time":        config.RetroactiveWindowTime.String(),
			"related_trace_time": config.RelatedTraceTime.String(),
		})

	case http.MethodPut:
		var req struct {
			Enabled          *bool   `json:"enabled,omitempty"`
			WindowSize       *int    `json:"window_size,omitempty"`
			WindowTime       *string `json:"window_time,omitempty"`
			RelatedTraceTime *string `json:"related_trace_time,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		config := intelligentSampler.GetConfig()

		if req.Enabled != nil {
			config.RetroactiveEnabled = *req.Enabled
		}
		if req.WindowSize != nil {
			config.RetroactiveWindowSize = *req.WindowSize
		}
		if req.WindowTime != nil {
			if d, err := time.ParseDuration(*req.WindowTime); err == nil {
				config.RetroactiveWindowTime = d
			}
		}
		if req.RelatedTraceTime != nil {
			if d, err := time.ParseDuration(*req.RelatedTraceTime); err == nil {
				config.RelatedTraceTime = d
			}
		}

		intelligentSampler.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config": map[string]interface{}{
				"enabled":            config.RetroactiveEnabled,
				"window_size":        config.RetroactiveWindowSize,
				"window_time":        config.RetroactiveWindowTime.String(),
				"related_trace_time": config.RelatedTraceTime.String(),
			},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRecentDecisions returns recent sampling decisions
// GET /api/sampling/intelligent/decisions?limit=100
func handleRecentDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	decisions := intelligentSampler.GetRecentDecisions(limit)

	// Summarize by reason
	reasonCounts := make(map[string]int)
	for _, d := range decisions {
		reasonCounts[d.Reason]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":         len(decisions),
		"decisions":     decisions,
		"reason_counts": reasonCounts,
	})
}

// handleAnomalyConfig handles anomaly detection configuration
// GET /api/sampling/intelligent/anomaly - Get anomaly config
// PUT /api/sampling/intelligent/anomaly - Update anomaly config
func handleAnomalyConfig(w http.ResponseWriter, r *http.Request) {
	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := intelligentSampler.GetConfig()
		stats := intelligentSampler.GetStats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":              config.AnomalyEnabled,
			"threshold":            config.AnomalyThreshold,
			"anomalies_detected":   stats.AnomaliesDetected,
			"anomalies_per_minute": float64(stats.AnomaliesDetected) / (float64(stats.TotalSpans) / 60.0),
		})

	case http.MethodPut:
		var req struct {
			Enabled   *bool    `json:"enabled,omitempty"`
			Threshold *float64 `json:"threshold,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		config := intelligentSampler.GetConfig()

		if req.Enabled != nil {
			config.AnomalyEnabled = *req.Enabled
		}
		if req.Threshold != nil {
			if *req.Threshold < 0 {
				http.Error(w, "Threshold must be non-negative", http.StatusBadRequest)
				return
			}
			config.AnomalyThreshold = *req.Threshold
		}

		intelligentSampler.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config": map[string]interface{}{
				"enabled":   config.AnomalyEnabled,
				"threshold": config.AnomalyThreshold,
			},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAnomalyTest tests anomaly detection on a sample span
// POST /api/sampling/intelligent/anomaly/test
func handleAnomalyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		ServiceName string  `json:"service_name"`
		DurationMs  float64 `json:"duration_ms"`
		Status      string  `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get learned patterns for context
	patterns := intelligentSampler.GetLearnedPatterns()

	// Check if we have enough data for the service
	serviceLatency, ok := patterns["service_latency"].(map[string]interface{})
	hasLearned := ok && serviceLatency[req.ServiceName] != nil

	config := intelligentSampler.GetConfig()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":         req.ServiceName,
		"duration_ms":     req.DurationMs,
		"has_learned":     hasLearned,
		"anomaly_enabled": config.AnomalyEnabled,
		"threshold":       config.AnomalyThreshold,
		"message":         "Submit spans via the sampler to calculate actual anomaly scores",
	})
}

// handleBudgetConfig handles budget configuration
// GET /api/sampling/intelligent/budget - Get budget config
// PUT /api/sampling/intelligent/budget - Update budget config
func handleBudgetConfig(w http.ResponseWriter, r *http.Request) {
	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := intelligentSampler.GetConfig()
		breakdown := intelligentSampler.GetCostBreakdown()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"cost_enabled":   config.CostEnabled,
			"cost_per_span":  config.CostPerSpan,
			"daily_budget":   config.DailyBudget,
			"current_usage":  breakdown,
		})

	case http.MethodPut:
		var req struct {
			Enabled     *bool    `json:"enabled,omitempty"`
			CostPerSpan *float64 `json:"cost_per_span,omitempty"`
			DailyBudget *float64 `json:"daily_budget,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		config := intelligentSampler.GetConfig()

		if req.Enabled != nil {
			config.CostEnabled = *req.Enabled
		}
		if req.CostPerSpan != nil {
			if *req.CostPerSpan < 0 {
				http.Error(w, "Cost per span must be non-negative", http.StatusBadRequest)
				return
			}
			config.CostPerSpan = *req.CostPerSpan
		}
		if req.DailyBudget != nil {
			if *req.DailyBudget <= 0 {
				http.Error(w, "Daily budget must be positive", http.StatusBadRequest)
				return
			}
			config.DailyBudget = *req.DailyBudget
		}

		intelligentSampler.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config": map[string]interface{}{
				"enabled":       config.CostEnabled,
				"cost_per_span": config.CostPerSpan,
				"daily_budget":  config.DailyBudget,
			},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLearningConfig handles learning mode configuration
// GET /api/sampling/intelligent/learning - Get learning config
// PUT /api/sampling/intelligent/learning - Update learning config
func handleLearningConfig(w http.ResponseWriter, r *http.Request) {
	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config := intelligentSampler.GetConfig()
		stats := intelligentSampler.GetStats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":          config.LearningEnabled,
			"min_samples":      config.LearningMinSamples,
			"samples_observed": stats.TotalSpans,
		})

	case http.MethodPut:
		var req struct {
			Enabled    *bool `json:"enabled,omitempty"`
			MinSamples *int  `json:"min_samples,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		config := intelligentSampler.GetConfig()

		if req.Enabled != nil {
			config.LearningEnabled = *req.Enabled
		}
		if req.MinSamples != nil {
			if *req.MinSamples < 1 {
				http.Error(w, "Min samples must be at least 1", http.StatusBadRequest)
				return
			}
			config.LearningMinSamples = *req.MinSamples
		}

		intelligentSampler.UpdateConfig(config)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"config": map[string]interface{}{
				"enabled":     config.LearningEnabled,
				"min_samples": config.LearningMinSamples,
			},
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLearnedRates returns learned sampling rates per service/operation
// GET /api/sampling/intelligent/learning/rates
func handleLearnedRates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if intelligentSampler == nil {
		http.Error(w, "Intelligent sampling not configured", http.StatusServiceUnavailable)
		return
	}

	patterns := intelligentSampler.GetLearnedPatterns()

	// Extract learned rates if available
	learnedRates := make(map[string]interface{})
	if rates, ok := patterns["learned_rates"]; ok {
		learnedRates = rates.(map[string]interface{})
	}

	// Extract service latency stats
	serviceStats := make(map[string]interface{})
	if latency, ok := patterns["service_latency"]; ok {
		serviceStats = latency.(map[string]interface{})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"learned_rates": learnedRates,
		"service_stats": serviceStats,
	})
}
