package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"dogwatch/internal/costintel"
)

var (
	recommendationEngine   *costintel.RecommendationEngine
	recommendationProvider costintel.UsageDataProvider
	cachedReport           *costintel.RecommendationReport
	reportCacheMu          sync.RWMutex
	reportCacheTime        time.Time
	reportCacheTTL         = 5 * time.Minute
)

// SetRecommendationEngine sets the recommendation engine and provider
func SetRecommendationEngine(engine *costintel.RecommendationEngine, provider costintel.UsageDataProvider) {
	recommendationEngine = engine
	recommendationProvider = provider
}

// RegisterRecommendationRoutes registers cost recommendation API routes
func RegisterRecommendationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cost/recommendations", handleCostRecommendations)
	mux.HandleFunc("/api/cost/recommendations/", handleCostRecommendation)
	mux.HandleFunc("/api/cost/quick-wins", handleQuickWins)
	mux.HandleFunc("/api/cost/savings-summary", handleSavingsSummary)
}

// getOrGenerateReport returns cached report or generates a new one
func getOrGenerateReport() *costintel.RecommendationReport {
	reportCacheMu.RLock()
	if cachedReport != nil && time.Since(reportCacheTime) < reportCacheTTL {
		report := cachedReport
		reportCacheMu.RUnlock()
		return report
	}
	reportCacheMu.RUnlock()

	// Generate new report
	reportCacheMu.Lock()
	defer reportCacheMu.Unlock()

	// Double-check after acquiring write lock
	if cachedReport != nil && time.Since(reportCacheTime) < reportCacheTTL {
		return cachedReport
	}

	if recommendationEngine == nil || recommendationProvider == nil {
		return &costintel.RecommendationReport{
			GeneratedAt:         time.Now(),
			Recommendations:     []costintel.Recommendation{},
			ByPriority:          make(map[string]int),
			ByType:              make(map[string]int),
		}
	}

	cachedReport = recommendationEngine.Analyze(recommendationProvider)
	reportCacheTime = time.Now()

	return cachedReport
}

// handleCostRecommendations returns all cost optimization recommendations
// GET /api/cost/recommendations - List all recommendations
// GET /api/cost/recommendations?type=drop_unused - Filter by type
// GET /api/cost/recommendations?priority=critical - Filter by priority
// POST /api/cost/recommendations/refresh - Force refresh
func handleCostRecommendations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		report := getOrGenerateReport()

		// Apply filters
		typeFilter := r.URL.Query().Get("type")
		priorityFilter := r.URL.Query().Get("priority")
		limitStr := r.URL.Query().Get("limit")

		recommendations := report.Recommendations

		if typeFilter != "" {
			recommendations = filterByType(recommendations, costintel.RecommendationType(typeFilter))
		}

		if priorityFilter != "" {
			recommendations = filterByPriority(recommendations, costintel.RecommendationPriority(priorityFilter))
		}

		if limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(recommendations) {
				recommendations = recommendations[:limit]
			}
		}

		// Calculate filtered totals
		var filteredSavings float64
		for _, rec := range recommendations {
			filteredSavings += rec.Impact.MonthlySavings
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"generated_at":         report.GeneratedAt,
			"total_recommendations": report.RecommendationCount,
			"total_monthly_savings": report.TotalSavings,
			"total_annual_savings":  report.AnnualSavings,
			"filtered_count":        len(recommendations),
			"filtered_savings":      filteredSavings,
			"by_priority":           report.ByPriority,
			"by_type":               report.ByType,
			"recommendations":       recommendations,
		})

	case http.MethodPost:
		// Force refresh
		reportCacheMu.Lock()
		cachedReport = nil
		reportCacheMu.Unlock()

		report := getOrGenerateReport()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"generated_at": report.GeneratedAt,
			"count":        report.RecommendationCount,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCostRecommendation handles individual recommendation operations
// GET /api/cost/recommendations/{id} - Get recommendation details
// POST /api/cost/recommendations/{id}/apply - Apply recommendation
// POST /api/cost/recommendations/{id}/dismiss - Dismiss recommendation
func handleCostRecommendation(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/cost/recommendations/"):]

	// Check for actions
	if len(path) > 0 {
		parts := splitPath(path)
		if len(parts) >= 2 {
			id := parts[0]
			action := parts[1]

			switch action {
			case "apply":
				handleApplyRecommendation(w, r, id)
				return
			case "dismiss":
				handleDismissRecommendation(w, r, id)
				return
			}
		}

		// Just an ID, get details
		if r.Method == http.MethodGet {
			id := parts[0]
			report := getOrGenerateReport()

			for _, rec := range report.Recommendations {
				if rec.ID == id {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(rec)
					return
				}
			}

			http.Error(w, "Recommendation not found", http.StatusNotFound)
			return
		}
	}

	http.Error(w, "Invalid request", http.StatusBadRequest)
}

// handleApplyRecommendation applies a recommendation by creating the suggested rule
func handleApplyRecommendation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	report := getOrGenerateReport()

	var rec *costintel.Recommendation
	for i := range report.Recommendations {
		if report.Recommendations[i].ID == id {
			rec = &report.Recommendations[i]
			break
		}
	}

	if rec == nil {
		http.Error(w, "Recommendation not found", http.StatusNotFound)
		return
	}

	// For now, return the action config so the client can apply it
	// In a full implementation, this would call the data shaping API directly
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"recommendation": rec.ID,
		"action":         rec.Action,
		"message":        "Use the action config to create a data shaping rule",
		"api_endpoint":   rec.Action.APIEndpoint,
		"rule_config":    rec.Action.RuleConfig,
	})
}

// handleDismissRecommendation marks a recommendation as dismissed
func handleDismissRecommendation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// In a full implementation, this would persist the dismissal
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"recommendation": id,
		"dismissed":      true,
		"reason":         req.Reason,
	})
}

// handleQuickWins returns the top recommendations with highest savings
// GET /api/cost/quick-wins?limit=5
func handleQuickWins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 5
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	report := getOrGenerateReport()
	quickWins := report.QuickWins(limit)

	var totalSavings float64
	for _, rec := range quickWins {
		totalSavings += rec.Impact.MonthlySavings
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":           len(quickWins),
		"monthly_savings": totalSavings,
		"annual_savings":  totalSavings * 12,
		"quick_wins":      quickWins,
	})
}

// handleSavingsSummary returns a summary of potential savings
// GET /api/cost/savings-summary
func handleSavingsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	report := getOrGenerateReport()

	// Calculate savings by category
	savingsByType := make(map[string]float64)
	savingsByPriority := make(map[string]float64)
	savingsByDataType := make(map[string]float64)

	for _, rec := range report.Recommendations {
		savingsByType[string(rec.Type)] += rec.Impact.MonthlySavings
		savingsByPriority[string(rec.Priority)] += rec.Impact.MonthlySavings
		savingsByDataType[rec.DataType] += rec.Impact.MonthlySavings
	}

	// Top actions to take
	type actionSummary struct {
		Type           string  `json:"type"`
		Description    string  `json:"description"`
		Count          int     `json:"count"`
		MonthlySavings float64 `json:"monthly_savings"`
	}

	typeDescriptions := map[string]string{
		"drop_unused":              "Drop metrics that are collected but never queried",
		"sample_high_volume":       "Sample high-volume services to reduce data without losing insights",
		"drop_debug_logs":          "Drop debug/trace level logs in production",
		"aggregate_metrics":        "Aggregate metrics to reduce cardinality",
		"drop_high_cardinality_tags": "Remove tags that cause cardinality explosion",
		"reduce_retention":         "Reduce retention period for less critical data",
		"set_quota":                "Set quotas to prevent cost overruns",
		"optimize_query":           "Optimize queries to reduce scan costs",
	}

	var actions []actionSummary
	for t, savings := range savingsByType {
		count := report.ByType[t]
		actions = append(actions, actionSummary{
			Type:           t,
			Description:    typeDescriptions[t],
			Count:          count,
			MonthlySavings: savings,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"generated_at":          report.GeneratedAt,
		"total_recommendations": report.RecommendationCount,
		"total_monthly_savings": report.TotalSavings,
		"total_annual_savings":  report.AnnualSavings,
		"savings_by_type":       savingsByType,
		"savings_by_priority":   savingsByPriority,
		"savings_by_data_type":  savingsByDataType,
		"recommended_actions":   actions,
		"priority_breakdown": map[string]interface{}{
			"critical": map[string]interface{}{
				"count":   report.ByPriority["critical"],
				"savings": savingsByPriority["critical"],
			},
			"high": map[string]interface{}{
				"count":   report.ByPriority["high"],
				"savings": savingsByPriority["high"],
			},
			"medium": map[string]interface{}{
				"count":   report.ByPriority["medium"],
				"savings": savingsByPriority["medium"],
			},
			"low": map[string]interface{}{
				"count":   report.ByPriority["low"],
				"savings": savingsByPriority["low"],
			},
		},
	})
}

func filterByType(recs []costintel.Recommendation, t costintel.RecommendationType) []costintel.Recommendation {
	var filtered []costintel.Recommendation
	for _, rec := range recs {
		if rec.Type == t {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func filterByPriority(recs []costintel.Recommendation, p costintel.RecommendationPriority) []costintel.Recommendation {
	var filtered []costintel.Recommendation
	for _, rec := range recs {
		if rec.Priority == p {
			filtered = append(filtered, rec)
		}
	}
	return filtered
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
