package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/costintel"
	"dogwatch/internal/custommetrics"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"
)

// Package-level stores for cost intelligence
var (
	costTraceStore    *trace.Store
	costLogStore      *logs.Store
	costCustomMetrics *custommetrics.Store
	costCalculator    = costintel.NewCalculator()
	costStore         *costintel.Store
	costCollector     *costintel.Collector
)

// SetCostIntelStores sets the stores for cost intelligence calculations
func SetCostIntelStores(
	traceStore *trace.Store,
	logStore *logs.Store,
	customMetrics *custommetrics.Store,
) {
	costTraceStore = traceStore
	costLogStore = logStore
	costCustomMetrics = customMetrics
}

// SetCostIntelStore sets the cost intelligence store for trending
func SetCostIntelStore(store *costintel.Store, collector *costintel.Collector) {
	costStore = store
	costCollector = collector
}

// RegisterCostIntelRoutes registers cost intelligence API routes
func RegisterCostIntelRoutes(mux *http.ServeMux) {
	// Current estimates
	mux.HandleFunc("/api/cost/usage", handleCostUsage)
	mux.HandleFunc("/api/cost/estimate", handleCostEstimate)
	mux.HandleFunc("/api/cost/estimate/vendor", handleCostVendorEstimate)
	mux.HandleFunc("/api/cost/calculate", handleCostCalculate)
	mux.HandleFunc("/api/cost/quick", handleCostQuick)
	mux.HandleFunc("/api/cost/pricing", handleCostPricing)

	// Trending endpoints
	mux.HandleFunc("/api/cost/trend", handleCostTrend)
	mux.HandleFunc("/api/cost/trend/vendor", handleCostTrendVendor)
	mux.HandleFunc("/api/cost/summary", handleCostSummary)
	mux.HandleFunc("/api/cost/history", handleCostHistory)
	mux.HandleFunc("/api/cost/record", handleCostRecord)
}

// handleCostUsage returns current usage metrics collected from stores
func handleCostUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	usage := collectUsageMetrics()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usage)
}

// handleCostEstimate returns cost estimates for all vendors
func handleCostEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	usage := collectUsageMetrics()
	comparison := costCalculator.Compare(usage)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comparison)
}

// handleCostVendorEstimate returns cost estimate for a specific vendor
func handleCostVendorEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vendor := r.URL.Query().Get("vendor")
	if vendor == "" {
		http.Error(w, "vendor parameter required", http.StatusBadRequest)
		return
	}

	usage := collectUsageMetrics()
	var estimate *costintel.CostEstimate

	switch vendor {
	case "datadog":
		estimate = costCalculator.CalculateDatadog(usage)
	case "newrelic":
		estimate = costCalculator.CalculateNewRelic(usage)
	case "splunk":
		estimate = costCalculator.CalculateSplunk(usage)
	default:
		http.Error(w, "unknown vendor: "+vendor+". Valid: datadog, newrelic, splunk", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(estimate)
}

// handleCostCalculate calculates cost for custom usage parameters
func handleCostCalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var usage costintel.UsageMetrics
	if err := json.NewDecoder(r.Body).Decode(&usage); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	comparison := costCalculator.Compare(usage)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comparison)
}

// handleCostQuick provides quick estimate from simple parameters
// Example: /api/cost/quick?hosts=10&containers=50&custom_metrics=500&logs_gb_per_day=10&spans_per_second=100
func handleCostQuick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters with sensible defaults
	hosts := parseIntParam(r, "hosts", 10)
	containers := parseIntParam(r, "containers", 50)
	customMetrics := parseIntParam(r, "custom_metrics", 500)
	logsGBPerDay := parseFloatParam(r, "logs_gb_per_day", 10.0)
	spansPerSecond := parseFloatParam(r, "spans_per_second", 100.0)

	usage := costintel.EstimateFromScale(hosts, containers, customMetrics, logsGBPerDay, spansPerSecond)
	comparison := costCalculator.Compare(usage)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comparison)
}

// handleCostPricing returns current pricing models for all vendors
func handleCostPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pricing := map[string]interface{}{
		"datadog":  costintel.DefaultDatadogPricing(),
		"newrelic": costintel.DefaultNewRelicPricing(),
		"splunk":   costintel.DefaultSplunkPricing(),
		"note":     "Prices are estimates based on publicly available information (as of 2024). Actual costs may vary based on contracts, volume discounts, and current pricing.",
		"updated":  "2024-01",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pricing)
}

// handleCostTrend returns cost trends for all vendors
// GET /api/cost/trend?period=7d (default: 24h)
func handleCostTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if costStore == nil {
		http.Error(w, "Cost trending not configured", http.StatusServiceUnavailable)
		return
	}

	period := parseDurationParam(r, "period", 24*time.Hour)
	trends, err := costStore.GetAllTrends(period)
	if err != nil {
		http.Error(w, "Failed to get trends: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"period": period.String(),
		"trends": trends,
	})
}

// handleCostTrendVendor returns cost trend for a specific vendor
// GET /api/cost/trend/vendor?vendor=datadog&period=7d
func handleCostTrendVendor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if costStore == nil {
		http.Error(w, "Cost trending not configured", http.StatusServiceUnavailable)
		return
	}

	vendor := r.URL.Query().Get("vendor")
	if vendor == "" {
		http.Error(w, "vendor parameter required", http.StatusBadRequest)
		return
	}

	// Map lowercase to display names
	vendorMap := map[string]string{
		"datadog":  "Datadog",
		"newrelic": "New Relic",
		"splunk":   "Splunk",
	}
	displayName, ok := vendorMap[vendor]
	if !ok {
		http.Error(w, "unknown vendor: "+vendor+". Valid: datadog, newrelic, splunk", http.StatusBadRequest)
		return
	}

	period := parseDurationParam(r, "period", 24*time.Hour)
	trend, err := costStore.GetTrend(displayName, period)
	if err != nil {
		http.Error(w, "Failed to get trend: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trend)
}

// handleCostSummary returns aggregated cost statistics
// GET /api/cost/summary?period=30d
func handleCostSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if costStore == nil {
		http.Error(w, "Cost trending not configured", http.StatusServiceUnavailable)
		return
	}

	period := parseDurationParam(r, "period", 30*24*time.Hour)
	summary, err := costStore.GetSummary(period)
	if err != nil {
		http.Error(w, "Failed to get summary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleCostHistory returns historical estimates for a vendor
// GET /api/cost/history?vendor=datadog&limit=100
func handleCostHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if costStore == nil {
		http.Error(w, "Cost trending not configured", http.StatusServiceUnavailable)
		return
	}

	vendor := r.URL.Query().Get("vendor")
	if vendor == "" {
		// Return latest for all vendors
		estimates, err := costStore.GetLatestEstimates()
		if err != nil {
			http.Error(w, "Failed to get estimates: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(estimates)
		return
	}

	// Map lowercase to display names
	vendorMap := map[string]string{
		"datadog":  "Datadog",
		"newrelic": "New Relic",
		"splunk":   "Splunk",
	}
	displayName, ok := vendorMap[vendor]
	if !ok {
		http.Error(w, "unknown vendor: "+vendor, http.StatusBadRequest)
		return
	}

	limit := parseIntParam(r, "limit", 100)
	history, err := costStore.GetHistory(displayName, limit)
	if err != nil {
		http.Error(w, "Failed to get history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// handleCostRecord records current cost estimates (manual trigger)
// POST /api/cost/record
func handleCostRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if costStore == nil {
		http.Error(w, "Cost trending not configured", http.StatusServiceUnavailable)
		return
	}

	// Collect current usage and record estimates
	usage := collectUsageMetrics()
	comparison := costCalculator.Compare(usage)

	if err := costStore.RecordComparison(comparison); err != nil {
		http.Error(w, "Failed to record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"recorded":   time.Now(),
		"comparison": comparison,
	})
}

func parseDurationParam(r *http.Request, name string, defaultVal time.Duration) time.Duration {
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

// collectUsageMetrics gathers current usage from all stores
func collectUsageMetrics() costintel.UsageMetrics {
	usage := costintel.UsageMetrics{
		CollectedAt: time.Now(),
		PeriodDays:  30,
	}

	// Collect trace metrics (APM data)
	if costTraceStore != nil {
		services, err := costTraceStore.GetServices()
		if err == nil {
			// Use service count as proxy for APM host count
			usage.APMHostCount = len(services)
			usage.HostCount = len(services) // Also use as general host estimate

			// Estimate spans from recent traces
			for _, svc := range services {
				traces, err := costTraceStore.ListTraces(1000, svc, 24*time.Hour)
				if err == nil {
					// Rough estimate: 5 spans per trace average, extrapolate to month
					usage.SpansPerMonth += int64(len(traces) * 5 * 30)
				}
			}
		}
	}

	// Collect log metrics
	if costLogStore != nil {
		end := time.Now()
		start := end.Add(-24 * time.Hour)

		query := logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Limit:     1, // We just need count
		}

		results, err := costLogStore.Search(query)
		if err == nil && results != nil {
			dailyLogs := int64(results.TotalCount)
			usage.LogEventsPerMonth = dailyLogs * 30

			// Estimate GB: ~1KB per log entry average
			usage.LogsGBPerMonth = float64(dailyLogs*30) * 0.001 / 1024
		}
	}

	// Collect custom metrics count
	if costCustomMetrics != nil {
		metricInfos, err := costCustomMetrics.List()
		if err == nil {
			usage.CustomMetricsCount = len(metricInfos)

			// Estimate data points: 1 per minute per metric
			usage.MetricDataPoints = int64(len(metricInfos)) * 60 * 24 * 30
		}
	}

	return usage
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
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

func parseFloatParam(r *http.Request, name string, defaultVal float64) float64 {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return defaultVal
	}
	return f
}
