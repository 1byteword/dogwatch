package quotas

import (
	"time"
)

// ResourceType represents a type of telemetry resource
type ResourceType string

const (
	ResourceMetrics       ResourceType = "metrics"
	ResourceLogs          ResourceType = "logs"
	ResourceTraces        ResourceType = "traces"
	ResourceCustomMetrics ResourceType = "custom_metrics"
	ResourceSpans         ResourceType = "spans"
	ResourceAll           ResourceType = "all"
)

// QuotaUnit represents the unit of measurement for a quota
type QuotaUnit string

const (
	UnitBytes      QuotaUnit = "bytes"
	UnitEvents     QuotaUnit = "events"
	UnitSeries     QuotaUnit = "series"
	UnitSpans      QuotaUnit = "spans"
	UnitGBPerMonth QuotaUnit = "gb_per_month"
)

// EnforcementAction defines what happens when quota is exceeded
type EnforcementAction string

const (
	ActionWarn     EnforcementAction = "warn"      // Log warning, allow data
	ActionSample   EnforcementAction = "sample"    // Sample data to stay under quota
	ActionDrop     EnforcementAction = "drop"      // Drop new data
	ActionThrottle EnforcementAction = "throttle"  // Rate limit ingestion
)

// Team represents a team/group that has quotas
type Team struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	OrgID       string    `json:"org_id"`
	CostCenter  string    `json:"cost_center,omitempty"`
	OwnerEmail  string    `json:"owner_email,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Quota defines a resource limit for a team
type Quota struct {
	ID           string            `json:"id"`
	TeamID       string            `json:"team_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	ResourceType ResourceType      `json:"resource_type"`
	Unit         QuotaUnit         `json:"unit"`
	Limit        int64             `json:"limit"`
	WarnAt       float64           `json:"warn_at"`        // Percentage (0.8 = 80%)
	Enforcement  EnforcementAction `json:"enforcement"`
	Period       string            `json:"period"`         // "hourly", "daily", "monthly"
	Enabled      bool              `json:"enabled"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// Usage represents current resource usage for a team
type Usage struct {
	TeamID       string       `json:"team_id"`
	ResourceType ResourceType `json:"resource_type"`
	Unit         QuotaUnit    `json:"unit"`
	Current      int64        `json:"current"`
	Limit        int64        `json:"limit"`
	Percentage   float64      `json:"percentage"`
	Period       string       `json:"period"`
	PeriodStart  time.Time    `json:"period_start"`
	PeriodEnd    time.Time    `json:"period_end"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// UsageRecord is a single usage event for tracking
type UsageRecord struct {
	TeamID       string       `json:"team_id"`
	ResourceType ResourceType `json:"resource_type"`
	Amount       int64        `json:"amount"`
	Unit         QuotaUnit    `json:"unit"`
	Timestamp    time.Time    `json:"timestamp"`
	Source       string       `json:"source,omitempty"`   // Service name
	Metadata     string       `json:"metadata,omitempty"` // JSON metadata
}

// ChargebackReport represents a cost allocation report
type ChargebackReport struct {
	TeamID       string             `json:"team_id"`
	TeamName     string             `json:"team_name"`
	CostCenter   string             `json:"cost_center,omitempty"`
	Period       string             `json:"period"`
	PeriodStart  time.Time          `json:"period_start"`
	PeriodEnd    time.Time          `json:"period_end"`
	TotalCost    float64            `json:"total_cost"`
	Breakdown    []CostBreakdown    `json:"breakdown"`
	Comparisons  []VendorComparison `json:"comparisons,omitempty"`
}

// CostBreakdown shows cost per resource type
type CostBreakdown struct {
	ResourceType ResourceType `json:"resource_type"`
	Usage        int64        `json:"usage"`
	Unit         QuotaUnit    `json:"unit"`
	UnitCost     float64      `json:"unit_cost"`
	TotalCost    float64      `json:"total_cost"`
	Percentage   float64      `json:"percentage"` // Of total team cost
}

// VendorComparison shows equivalent vendor cost
type VendorComparison struct {
	Vendor       string  `json:"vendor"`
	EstimatedCost float64 `json:"estimated_cost"`
	Savings      float64 `json:"savings"`
	SavingsPercent float64 `json:"savings_percent"`
}

// QuotaStatus represents the current status of a quota
type QuotaStatus struct {
	QuotaID      string            `json:"quota_id"`
	TeamID       string            `json:"team_id"`
	ResourceType ResourceType      `json:"resource_type"`
	Current      int64             `json:"current"`
	Limit        int64             `json:"limit"`
	Percentage   float64           `json:"percentage"`
	Status       string            `json:"status"` // "ok", "warning", "exceeded"
	Enforcement  EnforcementAction `json:"enforcement"`
	Message      string            `json:"message,omitempty"`
}

// AlertThreshold defines when to alert about quota usage
type AlertThreshold struct {
	Percentage float64 `json:"percentage"`
	Action     string  `json:"action"` // "notify", "escalate"
}

// TeamSummary provides an overview of a team's usage
type TeamSummary struct {
	Team         *Team          `json:"team"`
	Quotas       []*Quota       `json:"quotas"`
	UsageStatus  []*QuotaStatus `json:"usage_status"`
	TotalCost    float64        `json:"total_cost_mtd"`
	CostTrend    float64        `json:"cost_trend"` // % change from last period
	TopServices  []ServiceUsage `json:"top_services"`
}

// ServiceUsage shows usage by service within a team
type ServiceUsage struct {
	Service      string       `json:"service"`
	ResourceType ResourceType `json:"resource_type"`
	Usage        int64        `json:"usage"`
	Percentage   float64      `json:"percentage"`
	Cost         float64      `json:"cost"`
}

// QuotaViolation records when a quota was exceeded
type QuotaViolation struct {
	ID           string            `json:"id"`
	QuotaID      string            `json:"quota_id"`
	TeamID       string            `json:"team_id"`
	ResourceType ResourceType      `json:"resource_type"`
	Usage        int64             `json:"usage"`
	Limit        int64             `json:"limit"`
	Percentage   float64           `json:"percentage"`
	Action       EnforcementAction `json:"action_taken"`
	Timestamp    time.Time         `json:"timestamp"`
	Resolved     bool              `json:"resolved"`
	ResolvedAt   *time.Time        `json:"resolved_at,omitempty"`
}

// PricingConfig holds per-unit costs for chargeback calculation
type PricingConfig struct {
	MetricsPerSeriesMonth  float64 `json:"metrics_per_series_month"`   // $/series/month
	LogsPerGBIngest        float64 `json:"logs_per_gb_ingest"`         // $/GB ingested
	LogsPerGBStorage       float64 `json:"logs_per_gb_storage"`        // $/GB/month stored
	TracesPerSpan          float64 `json:"traces_per_span"`            // $/million spans
	CustomMetricsPerSeries float64 `json:"custom_metrics_per_series"`  // $/series/month
	APMPerHost             float64 `json:"apm_per_host"`               // $/host/month
}

// DefaultPricing returns dogwatch internal pricing (cost-based, not market-based)
func DefaultPricing() PricingConfig {
	return PricingConfig{
		MetricsPerSeriesMonth:  0.01,  // $0.01/series/month (vs $0.05 Datadog)
		LogsPerGBIngest:        0.05,  // $0.05/GB (vs $0.10 Datadog)
		LogsPerGBStorage:       0.02,  // $0.02/GB/month stored
		TracesPerSpan:          0.50,  // $0.50/million spans (vs $1.70 Datadog)
		CustomMetricsPerSeries: 0.02,  // $0.02/series/month
		APMPerHost:             5.00,  // $5/host/month (vs $31 Datadog)
	}
}

// DatadogPricing returns Datadog equivalent pricing for comparison
func DatadogPricing() PricingConfig {
	return PricingConfig{
		MetricsPerSeriesMonth:  0.05,
		LogsPerGBIngest:        0.10,
		LogsPerGBStorage:       0.05,
		TracesPerSpan:          1.70,
		CustomMetricsPerSeries: 0.05,
		APMPerHost:             31.00,
	}
}

// CalculateQuotaStatus computes the current status for a quota
func CalculateQuotaStatus(quota *Quota, current int64) *QuotaStatus {
	percentage := float64(current) / float64(quota.Limit) * 100

	status := &QuotaStatus{
		QuotaID:      quota.ID,
		TeamID:       quota.TeamID,
		ResourceType: quota.ResourceType,
		Current:      current,
		Limit:        quota.Limit,
		Percentage:   percentage,
		Enforcement:  quota.Enforcement,
	}

	if percentage >= 100 {
		status.Status = "exceeded"
		status.Message = "Quota exceeded - enforcement action active"
	} else if percentage >= quota.WarnAt*100 {
		status.Status = "warning"
		status.Message = "Approaching quota limit"
	} else {
		status.Status = "ok"
	}

	return status
}

// GetPeriodBounds returns the start and end times for a quota period
func GetPeriodBounds(period string, now time.Time) (time.Time, time.Time) {
	switch period {
	case "hourly":
		start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
		return start, start.Add(time.Hour)
	case "daily":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 0, 1)
	case "monthly":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0)
	default:
		// Default to monthly
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0)
	}
}
