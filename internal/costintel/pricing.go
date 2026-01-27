package costintel

// Pricing models based on publicly available information (as of 2024)
// These are estimates and may not reflect current or promotional pricing

// DatadogPricing contains Datadog's pricing tiers
type DatadogPricing struct {
	// Infrastructure monitoring (per host/month)
	InfraProHost       float64 // Pro tier
	InfraEnterpriseHost float64 // Enterprise tier

	// APM (per host/month)
	APMProHost         float64
	APMEnterpriseHost  float64

	// APM Spans (per million spans ingested)
	APMSpansPerMillion float64

	// Logs
	LogsIngestPerGB     float64 // Per GB ingested
	LogsIndexPerMillion float64 // Per million events indexed (15-day retention)
	LogsRetention30Day  float64 // Multiplier for 30-day retention

	// Custom Metrics (per 100 custom metrics/month)
	CustomMetricsPer100 float64
	CustomMetricsFree   int // Free tier

	// Synthetics (per 10k test runs)
	SyntheticsAPIPer10k    float64
	SyntheticsBrowserPer1k float64

	// RUM (per 1000 sessions)
	RUMPer1000Sessions float64
}

// NewRelicPricing contains New Relic's pricing
type NewRelicPricing struct {
	// Users (per user/month)
	FullUserStandard float64
	FullUserPro      float64
	CoreUser         float64
	BasicUserFree    bool

	// Data Ingest (per GB after free tier)
	DataIngestPerGB   float64
	DataIngestFreeGB  float64 // Free tier per month

	// Data Plus (additional features)
	DataPlusPerGB float64
}

// SplunkPricing contains Splunk's pricing
type SplunkPricing struct {
	// Workload pricing (per GB/day ingested)
	IngestPerGBDay float64

	// Infrastructure monitoring (per host/month)
	InfraPerHost float64

	// APM (per host/month)
	APMPerHost float64
}

// DefaultDatadogPricing returns current Datadog pricing estimates
func DefaultDatadogPricing() DatadogPricing {
	return DatadogPricing{
		// Infrastructure
		InfraProHost:        15.00,
		InfraEnterpriseHost: 23.00,

		// APM
		APMProHost:         31.00,
		APMEnterpriseHost:  40.00,
		APMSpansPerMillion: 0.10,

		// Logs
		LogsIngestPerGB:     0.10,
		LogsIndexPerMillion: 1.70,
		LogsRetention30Day:  1.5,

		// Custom Metrics
		CustomMetricsPer100: 5.00, // $0.05 per metric
		CustomMetricsFree:   100,

		// Synthetics
		SyntheticsAPIPer10k:    5.00,
		SyntheticsBrowserPer1k: 12.00,

		// RUM
		RUMPer1000Sessions: 1.50,
	}
}

// DefaultNewRelicPricing returns current New Relic pricing estimates
func DefaultNewRelicPricing() NewRelicPricing {
	return NewRelicPricing{
		// Users
		FullUserStandard: 99.00,
		FullUserPro:      349.00,
		CoreUser:         49.00,
		BasicUserFree:    true,

		// Data
		DataIngestPerGB:  0.30,
		DataIngestFreeGB: 100.0,

		// Data Plus
		DataPlusPerGB: 0.50,
	}
}

// DefaultSplunkPricing returns current Splunk pricing estimates
func DefaultSplunkPricing() SplunkPricing {
	return SplunkPricing{
		IngestPerGBDay: 2.00,  // Workload pricing
		InfraPerHost:   15.00,
		APMPerHost:     55.00,
	}
}
