package quotas

import (
	"time"
)

// ChargebackCalculator computes cost allocation per team
type ChargebackCalculator struct {
	store   *Store
	pricing PricingConfig
}

// NewChargebackCalculator creates a new chargeback calculator
func NewChargebackCalculator(store *Store, pricing PricingConfig) *ChargebackCalculator {
	return &ChargebackCalculator{
		store:   store,
		pricing: pricing,
	}
}

// Calculate computes a chargeback report for a team
func (c *ChargebackCalculator) Calculate(teamID, period string, start, end time.Time) (*ChargebackReport, error) {
	team, err := c.store.GetTeam(teamID)
	if err != nil {
		return nil, err
	}

	usages, err := c.store.GetTeamUsageSummary(teamID, period)
	if err != nil {
		return nil, err
	}

	report := &ChargebackReport{
		TeamID:      teamID,
		Period:      period,
		PeriodStart: start,
		PeriodEnd:   end,
	}

	if team != nil {
		report.TeamName = team.Name
		report.CostCenter = team.CostCenter
	}

	var totalCost float64
	var breakdown []CostBreakdown

	for _, usage := range usages {
		cost := c.calculateResourceCost(usage.ResourceType, usage.Unit, usage.Current)
		totalCost += cost

		breakdown = append(breakdown, CostBreakdown{
			ResourceType: usage.ResourceType,
			Usage:        usage.Current,
			Unit:         usage.Unit,
			UnitCost:     c.getUnitCost(usage.ResourceType, usage.Unit),
			TotalCost:    cost,
		})
	}

	// Calculate percentages
	for i := range breakdown {
		if totalCost > 0 {
			breakdown[i].Percentage = breakdown[i].TotalCost / totalCost * 100
		}
	}

	report.TotalCost = totalCost
	report.Breakdown = breakdown

	// Add vendor comparisons
	report.Comparisons = c.calculateVendorComparisons(usages, totalCost)

	return report, nil
}

func (c *ChargebackCalculator) calculateResourceCost(resourceType ResourceType, unit QuotaUnit, amount int64) float64 {
	switch resourceType {
	case ResourceMetrics:
		// Metrics charged per series per month
		return float64(amount) * c.pricing.MetricsPerSeriesMonth

	case ResourceLogs:
		// Logs charged per GB
		if unit == UnitBytes {
			gbAmount := float64(amount) / (1024 * 1024 * 1024)
			return gbAmount * c.pricing.LogsPerGBIngest
		}
		return float64(amount) * c.pricing.LogsPerGBIngest / 1000000 // events

	case ResourceTraces, ResourceSpans:
		// Traces charged per million spans
		return float64(amount) / 1000000 * c.pricing.TracesPerSpan

	case ResourceCustomMetrics:
		// Custom metrics per series
		return float64(amount) * c.pricing.CustomMetricsPerSeries

	default:
		return 0
	}
}

func (c *ChargebackCalculator) getUnitCost(resourceType ResourceType, unit QuotaUnit) float64 {
	switch resourceType {
	case ResourceMetrics:
		return c.pricing.MetricsPerSeriesMonth
	case ResourceLogs:
		if unit == UnitBytes {
			return c.pricing.LogsPerGBIngest
		}
		return c.pricing.LogsPerGBIngest / 1000000
	case ResourceTraces, ResourceSpans:
		return c.pricing.TracesPerSpan / 1000000
	case ResourceCustomMetrics:
		return c.pricing.CustomMetricsPerSeries
	default:
		return 0
	}
}

func (c *ChargebackCalculator) calculateVendorComparisons(usages []*Usage, dogwatchCost float64) []VendorComparison {
	// Calculate Datadog equivalent cost
	ddPricing := DatadogPricing()
	var ddCost float64

	for _, usage := range usages {
		switch usage.ResourceType {
		case ResourceMetrics:
			ddCost += float64(usage.Current) * ddPricing.MetricsPerSeriesMonth
		case ResourceLogs:
			if usage.Unit == UnitBytes {
				gbAmount := float64(usage.Current) / (1024 * 1024 * 1024)
				ddCost += gbAmount * ddPricing.LogsPerGBIngest
			} else {
				ddCost += float64(usage.Current) * ddPricing.LogsPerGBIngest / 1000000
			}
		case ResourceTraces, ResourceSpans:
			ddCost += float64(usage.Current) / 1000000 * ddPricing.TracesPerSpan
		case ResourceCustomMetrics:
			ddCost += float64(usage.Current) * ddPricing.CustomMetricsPerSeries
		}
	}

	var comparisons []VendorComparison

	if ddCost > 0 {
		savings := ddCost - dogwatchCost
		savingsPercent := 0.0
		if ddCost > 0 {
			savingsPercent = savings / ddCost * 100
		}

		comparisons = append(comparisons, VendorComparison{
			Vendor:         "Datadog",
			EstimatedCost:  ddCost,
			Savings:        savings,
			SavingsPercent: savingsPercent,
		})
	}

	// New Relic comparison (similar pricing structure)
	nrCost := ddCost * 0.85 // NR is roughly 15% cheaper than DD
	if nrCost > 0 {
		savings := nrCost - dogwatchCost
		savingsPercent := 0.0
		if nrCost > 0 {
			savingsPercent = savings / nrCost * 100
		}

		comparisons = append(comparisons, VendorComparison{
			Vendor:         "New Relic",
			EstimatedCost:  nrCost,
			Savings:        savings,
			SavingsPercent: savingsPercent,
		})
	}

	// Splunk comparison (typically more expensive)
	splunkCost := ddCost * 1.2 // Splunk is roughly 20% more
	if splunkCost > 0 {
		savings := splunkCost - dogwatchCost
		savingsPercent := 0.0
		if splunkCost > 0 {
			savingsPercent = savings / splunkCost * 100
		}

		comparisons = append(comparisons, VendorComparison{
			Vendor:         "Splunk",
			EstimatedCost:  splunkCost,
			Savings:        savings,
			SavingsPercent: savingsPercent,
		})
	}

	return comparisons
}

// CalculateAll generates chargeback reports for all teams
func (c *ChargebackCalculator) CalculateAll(orgID, period string, start, end time.Time) ([]*ChargebackReport, error) {
	teams, err := c.store.ListTeams(orgID)
	if err != nil {
		return nil, err
	}

	var reports []*ChargebackReport
	for _, team := range teams {
		report, err := c.Calculate(team.ID, period, start, end)
		if err != nil {
			continue
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// GetOrgSummary returns a summary of all team costs for an org
func (c *ChargebackCalculator) GetOrgSummary(orgID string) (*OrgChargeSummary, error) {
	start, end := GetPeriodBounds("monthly", time.Now())

	reports, err := c.CalculateAll(orgID, "monthly", start, end)
	if err != nil {
		return nil, err
	}

	summary := &OrgChargeSummary{
		OrgID:       orgID,
		Period:      "monthly",
		PeriodStart: start,
		PeriodEnd:   end,
		Teams:       make([]TeamCostSummary, 0, len(reports)),
	}

	var totalCost float64
	resourceTotals := make(map[ResourceType]float64)

	for _, report := range reports {
		totalCost += report.TotalCost

		teamSummary := TeamCostSummary{
			TeamID:     report.TeamID,
			TeamName:   report.TeamName,
			CostCenter: report.CostCenter,
			TotalCost:  report.TotalCost,
		}
		summary.Teams = append(summary.Teams, teamSummary)

		for _, bd := range report.Breakdown {
			resourceTotals[bd.ResourceType] += bd.TotalCost
		}
	}

	summary.TotalCost = totalCost

	// Calculate resource breakdown
	for rt, cost := range resourceTotals {
		pct := 0.0
		if totalCost > 0 {
			pct = cost / totalCost * 100
		}
		summary.ResourceBreakdown = append(summary.ResourceBreakdown, ResourceCostSummary{
			ResourceType: rt,
			TotalCost:    cost,
			Percentage:   pct,
		})
	}

	// Calculate team percentages
	for i := range summary.Teams {
		if totalCost > 0 {
			summary.Teams[i].Percentage = summary.Teams[i].TotalCost / totalCost * 100
		}
	}

	return summary, nil
}

// OrgChargeSummary represents organization-wide cost summary
type OrgChargeSummary struct {
	OrgID             string                `json:"org_id"`
	Period            string                `json:"period"`
	PeriodStart       time.Time             `json:"period_start"`
	PeriodEnd         time.Time             `json:"period_end"`
	TotalCost         float64               `json:"total_cost"`
	Teams             []TeamCostSummary     `json:"teams"`
	ResourceBreakdown []ResourceCostSummary `json:"resource_breakdown"`
}

// TeamCostSummary is a brief cost summary for a team
type TeamCostSummary struct {
	TeamID     string  `json:"team_id"`
	TeamName   string  `json:"team_name"`
	CostCenter string  `json:"cost_center,omitempty"`
	TotalCost  float64 `json:"total_cost"`
	Percentage float64 `json:"percentage"`
}

// ResourceCostSummary shows cost per resource type
type ResourceCostSummary struct {
	ResourceType ResourceType `json:"resource_type"`
	TotalCost    float64      `json:"total_cost"`
	Percentage   float64      `json:"percentage"`
}

// GenerateCSV generates a CSV export of chargeback data
func (c *ChargebackCalculator) GenerateCSV(orgID, period string, start, end time.Time) ([]byte, error) {
	reports, err := c.CalculateAll(orgID, period, start, end)
	if err != nil {
		return nil, err
	}

	// Build CSV
	csv := "Team ID,Team Name,Cost Center,Resource Type,Usage,Unit,Unit Cost,Total Cost,Vendor Comparison (Datadog)\n"

	for _, report := range reports {
		ddCost := 0.0
		for _, comp := range report.Comparisons {
			if comp.Vendor == "Datadog" {
				ddCost = comp.EstimatedCost
				break
			}
		}

		for _, bd := range report.Breakdown {
			csv += stringf("%s,%s,%s,%s,%d,%s,%.4f,%.2f,%.2f\n",
				report.TeamID, report.TeamName, report.CostCenter,
				bd.ResourceType, bd.Usage, bd.Unit,
				bd.UnitCost, bd.TotalCost, ddCost)
		}
	}

	return []byte(csv), nil
}

func stringf(format string, args ...interface{}) string {
	return sprintf(format, args...)
}

func sprintf(format string, args ...interface{}) string {
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = replaceFirst(result, "%s", v)
		case int:
			result = replaceFirst(result, "%d", itoa(v))
		case int64:
			result = replaceFirst(result, "%d", itoa64(v))
		case float64:
			result = replaceFirst(result, "%f", ftoa(v))
			result = replaceFirst(result, "%.2f", ftoa2(v))
			result = replaceFirst(result, "%.4f", ftoa4(v))
		case ResourceType:
			result = replaceFirst(result, "%s", string(v))
		case QuotaUnit:
			result = replaceFirst(result, "%s", string(v))
		}
	}
	return result
}

func replaceFirst(s, old, new string) string {
	for i := 0; i <= len(s)-len(old); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func itoa64(n int64) string {
	return itoa(int(n))
}

func ftoa(f float64) string {
	return ftoa2(f)
}

func ftoa2(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	whole := int64(f)
	frac := int64((f - float64(whole)) * 100)
	result := itoa64(whole) + "." + padLeft(itoa64(frac), 2, '0')
	if neg {
		result = "-" + result
	}
	return result
}

func ftoa4(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	whole := int64(f)
	frac := int64((f - float64(whole)) * 10000)
	result := itoa64(whole) + "." + padLeft(itoa64(frac), 4, '0')
	if neg {
		result = "-" + result
	}
	return result
}

func padLeft(s string, n int, c byte) string {
	for len(s) < n {
		s = string(c) + s
	}
	return s
}
