package costintel

import (
	"time"
)

// UsageMetrics represents current system usage
type UsageMetrics struct {
	// Infrastructure
	HostCount       int     `json:"host_count"`
	ContainerCount  int     `json:"container_count"`
	K8sNodeCount    int     `json:"k8s_node_count"`
	K8sPodCount     int     `json:"k8s_pod_count"`

	// APM / Traces
	APMHostCount    int     `json:"apm_host_count"`
	SpansPerMonth   int64   `json:"spans_per_month"`
	TracesPerMonth  int64   `json:"traces_per_month"`

	// Logs
	LogsGBPerMonth  float64 `json:"logs_gb_per_month"`
	LogEventsPerMonth int64 `json:"log_events_per_month"`

	// Custom Metrics
	CustomMetricsCount int `json:"custom_metrics_count"`
	MetricDataPoints   int64 `json:"metric_data_points_per_month"`

	// Synthetics
	SyntheticsAPIChecks     int `json:"synthetics_api_checks"`
	SyntheticsBrowserChecks int `json:"synthetics_browser_checks"`
	SyntheticsRunsPerMonth  int64 `json:"synthetics_runs_per_month"`

	// Users (for New Relic pricing)
	FullPlatformUsers int `json:"full_platform_users"`
	CoreUsers         int `json:"core_users"`

	// Collection period
	CollectedAt time.Time `json:"collected_at"`
	PeriodDays  int       `json:"period_days"`
}

// CostEstimate represents estimated costs for a vendor
type CostEstimate struct {
	Vendor          string             `json:"vendor"`
	TotalMonthly    float64            `json:"total_monthly"`
	TotalAnnual     float64            `json:"total_annual"`
	Breakdown       map[string]float64 `json:"breakdown"`
	Notes           []string           `json:"notes,omitempty"`
	CalculatedAt    time.Time          `json:"calculated_at"`
}

// CostComparison compares costs across vendors
type CostComparison struct {
	Usage           UsageMetrics              `json:"usage"`
	Estimates       map[string]*CostEstimate  `json:"estimates"`
	DogwatchSavings map[string]float64        `json:"dogwatch_savings"`
	CalculatedAt    time.Time                 `json:"calculated_at"`
}

// Calculator calculates cost estimates
type Calculator struct {
	datadogPricing  DatadogPricing
	newrelicPricing NewRelicPricing
	splunkPricing   SplunkPricing
}

// NewCalculator creates a new cost calculator
func NewCalculator() *Calculator {
	return &Calculator{
		datadogPricing:  DefaultDatadogPricing(),
		newrelicPricing: DefaultNewRelicPricing(),
		splunkPricing:   DefaultSplunkPricing(),
	}
}

// CalculateDatadog estimates Datadog costs
func (c *Calculator) CalculateDatadog(usage UsageMetrics) *CostEstimate {
	breakdown := make(map[string]float64)
	var notes []string

	// Infrastructure (use Pro tier as baseline)
	hostCount := usage.HostCount
	if usage.K8sNodeCount > hostCount {
		hostCount = usage.K8sNodeCount
	}
	if hostCount > 0 {
		infraCost := float64(hostCount) * c.datadogPricing.InfraProHost
		breakdown["infrastructure"] = infraCost
		notes = append(notes, "Infrastructure: Pro tier pricing")
	}

	// APM
	if usage.APMHostCount > 0 {
		apmHostCost := float64(usage.APMHostCount) * c.datadogPricing.APMProHost
		breakdown["apm_hosts"] = apmHostCost
	}
	if usage.SpansPerMonth > 0 {
		spanCost := (float64(usage.SpansPerMonth) / 1_000_000) * c.datadogPricing.APMSpansPerMillion
		breakdown["apm_spans"] = spanCost
	}

	// Logs
	if usage.LogsGBPerMonth > 0 {
		ingestCost := usage.LogsGBPerMonth * c.datadogPricing.LogsIngestPerGB
		breakdown["logs_ingest"] = ingestCost
	}
	if usage.LogEventsPerMonth > 0 {
		indexCost := (float64(usage.LogEventsPerMonth) / 1_000_000) * c.datadogPricing.LogsIndexPerMillion
		breakdown["logs_index"] = indexCost
		notes = append(notes, "Logs: 15-day retention")
	}

	// Custom Metrics
	if usage.CustomMetricsCount > c.datadogPricing.CustomMetricsFree {
		billableMetrics := usage.CustomMetricsCount - c.datadogPricing.CustomMetricsFree
		metricsCost := (float64(billableMetrics) / 100) * c.datadogPricing.CustomMetricsPer100
		breakdown["custom_metrics"] = metricsCost
		notes = append(notes, "First 100 custom metrics free")
	}

	// Synthetics
	if usage.SyntheticsRunsPerMonth > 0 {
		// Estimate split between API and browser
		apiRuns := usage.SyntheticsRunsPerMonth
		if usage.SyntheticsBrowserChecks > 0 {
			apiRuns = usage.SyntheticsRunsPerMonth / 2
		}
		apiCost := (float64(apiRuns) / 10_000) * c.datadogPricing.SyntheticsAPIPer10k
		breakdown["synthetics"] = apiCost
	}

	// Calculate total
	var total float64
	for _, v := range breakdown {
		total += v
	}

	return &CostEstimate{
		Vendor:       "Datadog",
		TotalMonthly: total,
		TotalAnnual:  total * 12,
		Breakdown:    breakdown,
		Notes:        notes,
		CalculatedAt: time.Now(),
	}
}

// CalculateNewRelic estimates New Relic costs
func (c *Calculator) CalculateNewRelic(usage UsageMetrics) *CostEstimate {
	breakdown := make(map[string]float64)
	var notes []string

	// Users
	if usage.FullPlatformUsers > 0 {
		userCost := float64(usage.FullPlatformUsers) * c.newrelicPricing.FullUserStandard
		breakdown["full_users"] = userCost
		notes = append(notes, "Standard tier user pricing")
	}
	if usage.CoreUsers > 0 {
		coreCost := float64(usage.CoreUsers) * c.newrelicPricing.CoreUser
		breakdown["core_users"] = coreCost
	}

	// Data Ingest
	totalDataGB := usage.LogsGBPerMonth
	// Estimate trace data (roughly 1KB per span)
	if usage.SpansPerMonth > 0 {
		traceGB := float64(usage.SpansPerMonth) * 0.001 / 1024 // 1KB per span
		totalDataGB += traceGB
	}
	// Estimate metric data
	if usage.MetricDataPoints > 0 {
		metricGB := float64(usage.MetricDataPoints) * 0.0001 / 1024 // ~100 bytes per point
		totalDataGB += metricGB
	}

	if totalDataGB > c.newrelicPricing.DataIngestFreeGB {
		billableGB := totalDataGB - c.newrelicPricing.DataIngestFreeGB
		dataCost := billableGB * c.newrelicPricing.DataIngestPerGB
		breakdown["data_ingest"] = dataCost
		notes = append(notes, "100GB/month free tier applied")
	}

	// If no users specified, estimate based on host count
	if usage.FullPlatformUsers == 0 && usage.CoreUsers == 0 {
		// Assume 1 full user per 10 hosts, minimum 1
		estimatedUsers := (usage.HostCount / 10) + 1
		if estimatedUsers < 1 {
			estimatedUsers = 1
		}
		userCost := float64(estimatedUsers) * c.newrelicPricing.FullUserStandard
		breakdown["full_users_estimated"] = userCost
		notes = append(notes, "User count estimated from host count")
	}

	var total float64
	for _, v := range breakdown {
		total += v
	}

	return &CostEstimate{
		Vendor:       "New Relic",
		TotalMonthly: total,
		TotalAnnual:  total * 12,
		Breakdown:    breakdown,
		Notes:        notes,
		CalculatedAt: time.Now(),
	}
}

// CalculateSplunk estimates Splunk Observability costs
func (c *Calculator) CalculateSplunk(usage UsageMetrics) *CostEstimate {
	breakdown := make(map[string]float64)
	var notes []string

	// Infrastructure monitoring
	hostCount := usage.HostCount
	if usage.K8sNodeCount > hostCount {
		hostCount = usage.K8sNodeCount
	}
	if hostCount > 0 {
		infraCost := float64(hostCount) * c.splunkPricing.InfraPerHost
		breakdown["infrastructure"] = infraCost
	}

	// APM
	if usage.APMHostCount > 0 {
		apmCost := float64(usage.APMHostCount) * c.splunkPricing.APMPerHost
		breakdown["apm"] = apmCost
	}

	// Log Observer (workload pricing)
	if usage.LogsGBPerMonth > 0 {
		// Convert to daily average
		dailyGB := usage.LogsGBPerMonth / 30
		logCost := dailyGB * c.splunkPricing.IngestPerGBDay * 30
		breakdown["logs"] = logCost
		notes = append(notes, "Workload-based pricing")
	}

	var total float64
	for _, v := range breakdown {
		total += v
	}

	return &CostEstimate{
		Vendor:       "Splunk",
		TotalMonthly: total,
		TotalAnnual:  total * 12,
		Breakdown:    breakdown,
		Notes:        notes,
		CalculatedAt: time.Now(),
	}
}

// Compare calculates cost comparison across all vendors
func (c *Calculator) Compare(usage UsageMetrics) *CostComparison {
	estimates := map[string]*CostEstimate{
		"datadog":   c.CalculateDatadog(usage),
		"newrelic":  c.CalculateNewRelic(usage),
		"splunk":    c.CalculateSplunk(usage),
	}

	savings := make(map[string]float64)
	for vendor, estimate := range estimates {
		savings[vendor] = estimate.TotalAnnual // 100% savings since dogwatch is free/self-hosted
	}

	return &CostComparison{
		Usage:           usage,
		Estimates:       estimates,
		DogwatchSavings: savings,
		CalculatedAt:    time.Now(),
	}
}

// EstimateFromScale creates usage estimate from simple scale parameters
func EstimateFromScale(hosts, containers, customMetrics int, logsGBPerDay, spansPerSecond float64) UsageMetrics {
	return UsageMetrics{
		HostCount:          hosts,
		ContainerCount:     containers,
		APMHostCount:       hosts, // Assume APM on all hosts
		SpansPerMonth:      int64(spansPerSecond * 60 * 60 * 24 * 30),
		LogsGBPerMonth:     logsGBPerDay * 30,
		LogEventsPerMonth:  int64(logsGBPerDay * 1_000_000), // ~1M events per GB
		CustomMetricsCount: customMetrics,
		MetricDataPoints:   int64(customMetrics) * 60 * 24 * 30, // 1 point per minute
		FullPlatformUsers:  (hosts / 10) + 1,
		CollectedAt:        time.Now(),
		PeriodDays:         30,
	}
}
