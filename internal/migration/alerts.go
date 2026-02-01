package migration

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dogwatch/internal/alerting"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// PrometheusAlertRules represents Prometheus alerting rules YAML
type PrometheusAlertRules struct {
	Groups []PrometheusRuleGroup `yaml:"groups" json:"groups"`
}

// PrometheusRuleGroup represents a group of Prometheus rules
type PrometheusRuleGroup struct {
	Name     string           `yaml:"name" json:"name"`
	Interval string           `yaml:"interval,omitempty" json:"interval,omitempty"`
	Rules    []PrometheusRule `yaml:"rules" json:"rules"`
}

// PrometheusRule represents a single Prometheus alerting or recording rule
type PrometheusRule struct {
	// Alert rule fields
	Alert       string            `yaml:"alert,omitempty" json:"alert,omitempty"`
	Expr        string            `yaml:"expr" json:"expr"`
	For         string            `yaml:"for,omitempty" json:"for,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`

	// Recording rule fields
	Record string `yaml:"record,omitempty" json:"record,omitempty"`
}

// AlertImporter handles importing alerts from various sources
type AlertImporter struct {
	datadogImporter *DatadogImporter
	grafanaImporter *GrafanaImporter
}

// NewAlertImporter creates a new alert importer
func NewAlertImporter() *AlertImporter {
	return &AlertImporter{
		datadogImporter: NewDatadogImporter(DashboardImportOptions{}),
		grafanaImporter: NewGrafanaImporter(DashboardImportOptions{}),
	}
}

// ImportAlerts auto-detects format and imports alerts
func (a *AlertImporter) ImportAlerts(data []byte, opts AlertImportOptions) ([]*ConvertedAlert, *ImportResult, error) {
	startTime := time.Now()

	// Detect format
	format := DetectFormat(data)

	result := &ImportResult{
		ID:        uuid.New().String(),
		Source:    format,
		ItemType:  "alert",
		CreatedAt: time.Now(),
	}

	var alerts []*ConvertedAlert
	var alertResults []AlertResult

	switch format {
	case PlatformDatadog:
		result.SourceName = "Datadog Monitors"
		imported, results, err := a.datadogImporter.ImportMonitors(data, opts)
		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, ImportError{
				Item:    "monitors",
				Message: err.Error(),
			})
			return nil, result, err
		}
		alerts = imported
		alertResults = results

	case PlatformGrafana:
		result.SourceName = "Grafana Alerts"
		imported, results, err := a.grafanaImporter.ImportAlertRules(data, opts)
		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, ImportError{
				Item:    "alerts",
				Message: err.Error(),
			})
			return nil, result, err
		}
		alerts = imported
		alertResults = results

	case PlatformPrometheus:
		result.SourceName = "Prometheus Alerting Rules"
		imported, results, err := a.ImportPrometheusRules(data, opts)
		if err != nil {
			result.Success = false
			result.Errors = append(result.Errors, ImportError{
				Item:    "rules",
				Message: err.Error(),
			})
			return nil, result, err
		}
		alerts = imported
		alertResults = results

	default:
		result.Success = false
		result.Errors = append(result.Errors, ImportError{
			Item:    "format",
			Message: "Unknown alert format. Supported: Datadog, Grafana, Prometheus",
		})
		return nil, result, fmt.Errorf("unknown alert format")
	}

	// Compile results
	for _, ar := range alertResults {
		if ar.Success {
			result.ItemsImported++
		} else {
			result.ItemsFailed++
			if ar.Error != "" {
				result.Errors = append(result.Errors, ImportError{
					Item:    ar.SourceName,
					Message: ar.Error,
				})
			}
		}
		result.Warnings = append(result.Warnings, ar.Warnings...)
	}

	result.Success = result.ItemsFailed == 0
	result.Duration = time.Since(startTime)

	return alerts, result, nil
}

// ImportPrometheusRules imports Prometheus alerting rules
func (a *AlertImporter) ImportPrometheusRules(data []byte, opts AlertImportOptions) ([]*ConvertedAlert, []AlertResult, error) {
	rules, err := ParsePrometheusRules(data)
	if err != nil {
		return nil, nil, err
	}

	var alerts []*ConvertedAlert
	var results []AlertResult

	for _, group := range rules.Groups {
		for _, rule := range group.Rules {
			// Skip recording rules
			if rule.Record != "" {
				continue
			}

			alert, result := ConvertPrometheusRule(rule, group.Name, opts)
			alerts = append(alerts, alert)
			results = append(results, *result)
		}
	}

	return alerts, results, nil
}

// ParsePrometheusRules parses Prometheus alerting rules from YAML or JSON
func ParsePrometheusRules(data []byte) (*PrometheusAlertRules, error) {
	var rules PrometheusAlertRules

	// Try YAML first
	if err := yaml.Unmarshal(data, &rules); err == nil && len(rules.Groups) > 0 {
		return &rules, nil
	}

	// Try JSON
	if err := json.Unmarshal(data, &rules); err == nil && len(rules.Groups) > 0 {
		return &rules, nil
	}

	// Try single group
	var group PrometheusRuleGroup
	if err := yaml.Unmarshal(data, &group); err == nil && len(group.Rules) > 0 {
		return &PrometheusAlertRules{Groups: []PrometheusRuleGroup{group}}, nil
	}

	return nil, fmt.Errorf("failed to parse Prometheus rules")
}

// ConvertPrometheusRule converts a Prometheus alerting rule to dogwatch format
func ConvertPrometheusRule(rule PrometheusRule, groupName string, opts AlertImportOptions) (*ConvertedAlert, *AlertResult) {
	result := &AlertResult{
		SourceName: rule.Alert,
		Success:    true,
	}

	ruleID := uuid.New().String()
	result.TargetID = ruleID

	// Build rule name
	name := rule.Alert
	if opts.AlertNamePrefix != "" {
		name = opts.AlertNamePrefix + name
	}

	// Convert PromQL to DQL
	dqlQuery := convertPromQL(rule.Expr)

	// Parse threshold from expression
	threshold, condition := extractThresholdFromPromQL(rule.Expr)

	// Parse for duration
	forDuration := time.Duration(0)
	if rule.For != "" {
		if d, err := time.ParseDuration(rule.For); err == nil {
			forDuration = d
		}
	}

	// Build labels
	labels := make(map[string]string)
	for k, v := range rule.Labels {
		labels[k] = v
	}
	labels["source"] = "prometheus"
	labels["group"] = groupName

	// Determine severity (stored in labels for now)
	if sev, ok := labels["severity"]; ok {
		switch sev {
		case "critical", "page":
			labels["_severity"] = string(alerting.SeverityCritical)
		case "warning", "warn":
			labels["_severity"] = string(alerting.SeverityWarning)
		case "info", "informational":
			labels["_severity"] = string(alerting.SeverityInfo)
		}
	}

	// Build annotations
	annotations := make(map[string]string)
	for k, v := range rule.Annotations {
		annotations[k] = v
	}

	alertRule := &alerting.Rule{
		ID:             ruleID,
		Name:           name,
		Description:    annotations["description"],
		Type:           alerting.RuleTypeThreshold,
		Enabled:        opts.EnableImportedAlerts,
		Labels:         labels,
		Annotations:    annotations,
		Query:          dqlQuery,
		Condition:      condition,
		Threshold:      threshold,
		ForDuration:    forDuration,
		EvalInterval:   1 * time.Minute,
		NotifyChannels: opts.DefaultNotifyChannels,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Handle special cases
	if strings.Contains(strings.ToLower(rule.Expr), "absent") {
		alertRule.Type = alerting.RuleTypeAbsence
		result.Warnings = append(result.Warnings, "Converted absent() to absence rule type")
	}

	if strings.Contains(strings.ToLower(rule.Expr), "changes") ||
		strings.Contains(strings.ToLower(rule.Expr), "deriv") {
		alertRule.Type = alerting.RuleTypeChange
		result.Warnings = append(result.Warnings, "Converted to change detection rule")
	}

	converted := &ConvertedAlert{
		Rule:           alertRule,
		SourceQuery:    rule.Expr,
		ConversionNote: fmt.Sprintf("Converted from Prometheus alerting rule (group: %s)", groupName),
	}

	// Add warnings for complex expressions
	if strings.Count(rule.Expr, "(") > 3 {
		result.Warnings = append(result.Warnings, "Complex expression may need manual review")
	}

	return converted, result
}

// extractThresholdFromPromQL extracts threshold and comparison operator from PromQL
func extractThresholdFromPromQL(expr string) (float64, string) {
	// Common patterns:
	// metric > 80
	// metric >= 0.95
	// metric < 10
	// metric <= 100
	// metric == 0
	// metric != 1

	patterns := []struct {
		regex string
		cond  string
	}{
		{`>\s*([0-9.]+)\s*$`, "gt"},
		{`>=\s*([0-9.]+)\s*$`, "gte"},
		{`<\s*([0-9.]+)\s*$`, "lt"},
		{`<=\s*([0-9.]+)\s*$`, "lte"},
		{`==\s*([0-9.]+)\s*$`, "eq"},
		{`!=\s*([0-9.]+)\s*$`, "neq"},
	}

	for _, p := range patterns {
		re := regexp.MustCompile(p.regex)
		if matches := re.FindStringSubmatch(expr); len(matches) > 1 {
			var threshold float64
			fmt.Sscanf(matches[1], "%f", &threshold)
			return threshold, p.cond
		}
	}

	// Default to "gt 0" if no threshold found
	return 0, "gt"
}

// GenerateMigrationReport generates a comprehensive migration report
func GenerateMigrationReport(
	dashResults []DashboardResult,
	alertResults []AlertResult,
	source SourcePlatform,
	startTime time.Time,
) *MigrationReport {
	report := &MigrationReport{
		ID:        uuid.New().String(),
		Source:    source,
		StartedAt: startTime,
	}

	// Process dashboard results
	for _, dr := range dashResults {
		report.DashboardsTotal++
		if dr.Success {
			report.DashboardsImported++
		} else {
			report.DashboardsFailed++
			if dr.Error != "" {
				report.Errors = append(report.Errors, ImportError{
					Item:    dr.SourceName,
					Message: dr.Error,
				})
			}
		}
		report.DashboardResults = append(report.DashboardResults, dr)
		report.Warnings = append(report.Warnings, dr.Warnings...)
	}

	// Process alert results
	for _, ar := range alertResults {
		report.AlertsTotal++
		if ar.Success {
			report.AlertsImported++
		} else {
			report.AlertsFailed++
			if ar.Error != "" {
				report.Errors = append(report.Errors, ImportError{
					Item:    ar.SourceName,
					Message: ar.Error,
				})
			}
		}
		report.AlertResults = append(report.AlertResults, ar)
		report.Warnings = append(report.Warnings, ar.Warnings...)
	}

	// Generate manual steps
	if len(report.Errors) > 0 {
		report.ManualSteps = append(report.ManualSteps,
			"Review failed imports and recreate manually if needed")
	}
	if len(report.Warnings) > 0 {
		report.ManualSteps = append(report.ManualSteps,
			"Review warnings and verify converted queries work correctly")
	}
	report.ManualSteps = append(report.ManualSteps,
		"Verify notification channels are correctly mapped",
		"Test alert thresholds in your environment")

	report.CompletedAt = time.Now()
	report.Duration = report.CompletedAt.Sub(startTime)

	return report
}

// ValidateAlertRule validates a converted alert rule
func ValidateAlertRule(rule *alerting.Rule) []string {
	var errors []string

	if rule.Name == "" {
		errors = append(errors, "Rule name is required")
	}

	if rule.Query == "" && rule.Metric == "" {
		errors = append(errors, "Either query or metric is required")
	}

	if rule.Type == alerting.RuleTypeThreshold && rule.Condition == "" {
		errors = append(errors, "Threshold rule requires a condition (gt, lt, gte, lte, eq, neq)")
	}

	if rule.Type == alerting.RuleTypeComposite && rule.Expression == "" {
		errors = append(errors, "Composite rule requires an expression")
	}

	if rule.Type == alerting.RuleTypeAnomaly && rule.Sensitivity == 0 {
		errors = append(errors, "Anomaly rule requires sensitivity setting")
	}

	if rule.EvalInterval < time.Second {
		errors = append(errors, "Evaluation interval must be at least 1 second")
	}

	return errors
}

// GetSupportedFormats returns information about supported import formats
func GetSupportedFormats() map[string]any {
	return map[string]any{
		"formats": []map[string]string{
			{
				"name":        "datadog",
				"description": "Datadog dashboard JSON export and monitor JSON",
				"dashboards":  "Supports: timeseries, query_value, toplist, heatmap, table, distribution",
				"alerts":      "Supports: metric alert, query alert, service check, composite, anomaly",
			},
			{
				"name":        "grafana",
				"description": "Grafana dashboard JSON export and alerting rules (v8+)",
				"dashboards":  "Supports: graph, timeseries, stat, gauge, bargauge, table, heatmap, piechart",
				"alerts":      "Supports: Grafana alerting rules and legacy panel alerts",
			},
			{
				"name":        "prometheus",
				"description": "Prometheus alerting rules YAML/JSON",
				"dashboards":  "Not applicable",
				"alerts":      "Supports: Prometheus alerting rules with PromQL expressions",
			},
		},
		"query_conversion": map[string]string{
			"datadog_to_dql":    "Converts Datadog metric queries to DQL format",
			"promql_to_dql":     "Converts PromQL to DQL (common functions supported)",
			"grafana_to_dql":    "Converts Grafana queries based on datasource type",
		},
		"limitations": []string{
			"Complex nested queries may require manual adjustment",
			"Datasource-specific functions may not have direct equivalents",
			"Template variables need to be reconfigured for dogwatch",
			"Notification channels need manual mapping",
		},
	}
}
