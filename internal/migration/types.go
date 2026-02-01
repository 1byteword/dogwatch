// Package migration provides tools for importing dashboards and alerts
// from other observability platforms (Datadog, Grafana, Prometheus).
package migration

import (
	"time"

	"dogwatch/internal/alerting"
	"dogwatch/internal/dashboard"
)

// SourcePlatform identifies the source observability platform
type SourcePlatform string

const (
	PlatformDatadog    SourcePlatform = "datadog"
	PlatformGrafana    SourcePlatform = "grafana"
	PlatformPrometheus SourcePlatform = "prometheus"
	PlatformUnknown    SourcePlatform = "unknown"
)

// ImportResult contains the result of an import operation
type ImportResult struct {
	ID            string           `json:"id"`
	Success       bool             `json:"success"`
	Source        SourcePlatform   `json:"source"`
	SourceName    string           `json:"source_name"`
	ItemType      string           `json:"item_type"` // dashboard, alert
	ItemsImported int              `json:"items_imported"`
	ItemsSkipped  int              `json:"items_skipped"`
	ItemsFailed   int              `json:"items_failed"`
	Warnings      []string         `json:"warnings,omitempty"`
	Errors        []ImportError    `json:"errors,omitempty"`
	Duration      time.Duration    `json:"duration"`
	CreatedAt     time.Time        `json:"created_at"`
}

// ImportError represents an error during import
type ImportError struct {
	Item    string `json:"item"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// MigrationReport contains a full migration report
type MigrationReport struct {
	ID              string            `json:"id"`
	Source          SourcePlatform    `json:"source"`
	StartedAt       time.Time         `json:"started_at"`
	CompletedAt     time.Time         `json:"completed_at"`
	Duration        time.Duration     `json:"duration"`

	// Dashboard results
	DashboardsTotal     int              `json:"dashboards_total"`
	DashboardsImported  int              `json:"dashboards_imported"`
	DashboardsSkipped   int              `json:"dashboards_skipped"`
	DashboardsFailed    int              `json:"dashboards_failed"`
	DashboardResults    []DashboardResult `json:"dashboard_results,omitempty"`

	// Alert results
	AlertsTotal     int           `json:"alerts_total"`
	AlertsImported  int           `json:"alerts_imported"`
	AlertsSkipped   int           `json:"alerts_skipped"`
	AlertsFailed    int           `json:"alerts_failed"`
	AlertResults    []AlertResult `json:"alert_results,omitempty"`

	// Summary
	Warnings    []string      `json:"warnings,omitempty"`
	Errors      []ImportError `json:"errors,omitempty"`
	ManualSteps []string      `json:"manual_steps,omitempty"` // Things requiring manual intervention
}

// DashboardResult contains the result of a single dashboard import
type DashboardResult struct {
	SourceName     string    `json:"source_name"`
	SourceID       string    `json:"source_id,omitempty"`
	TargetID       string    `json:"target_id,omitempty"`
	Success        bool      `json:"success"`
	WidgetsTotal   int       `json:"widgets_total"`
	WidgetsConverted int     `json:"widgets_converted"`
	WidgetsSkipped int       `json:"widgets_skipped"`
	Warnings       []string  `json:"warnings,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// AlertResult contains the result of a single alert import
type AlertResult struct {
	SourceName  string   `json:"source_name"`
	SourceID    string   `json:"source_id,omitempty"`
	TargetID    string   `json:"target_id,omitempty"`
	Success     bool     `json:"success"`
	Warnings    []string `json:"warnings,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// WidgetMapping maps source widget types to dogwatch widget types
type WidgetMapping struct {
	SourceType    string `json:"source_type"`
	TargetType    string `json:"target_type"`
	IsSupported   bool   `json:"is_supported"`
	Notes         string `json:"notes,omitempty"`
}

// QueryMapping maps source query syntax to DQL
type QueryMapping struct {
	SourceQuery  string `json:"source_query"`
	TargetQuery  string `json:"target_query"`
	IsExact      bool   `json:"is_exact"`      // true if exact conversion, false if approximate
	Notes        string `json:"notes,omitempty"`
}

// DashboardImportOptions configures dashboard import behavior
type DashboardImportOptions struct {
	SkipUnsupportedWidgets bool   `json:"skip_unsupported_widgets"`
	PreserveOriginalIDs    bool   `json:"preserve_original_ids"`
	DashboardNamePrefix    string `json:"dashboard_name_prefix"`
	SetAsDefault           bool   `json:"set_as_default"`
}

// AlertImportOptions configures alert import behavior
type AlertImportOptions struct {
	EnableImportedAlerts   bool     `json:"enable_imported_alerts"`
	DefaultNotifyChannels  []string `json:"default_notify_channels,omitempty"`
	AlertNamePrefix        string   `json:"alert_name_prefix"`
	OverwriteExisting      bool     `json:"overwrite_existing"`
}

// ConvertedDashboard represents a dashboard converted to dogwatch format
type ConvertedDashboard struct {
	Dashboard      *dashboard.Dashboard    `json:"dashboard"`
	WidgetConfigs  map[string]WidgetConfig `json:"widget_configs"` // widget ID -> config
	Variables      []TemplateVariable      `json:"variables,omitempty"`
	Annotations    []Annotation            `json:"annotations,omitempty"`
}

// WidgetConfig contains the full configuration for a widget
type WidgetConfig struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // timeseries, stat, table, etc.
	Title       string            `json:"title"`
	Query       string            `json:"query"` // DQL query
	SourceQuery string            `json:"source_query,omitempty"` // Original query for reference
	Options     map[string]any    `json:"options,omitempty"`
}

// TemplateVariable represents a dashboard template variable
type TemplateVariable struct {
	Name       string   `json:"name"`
	Label      string   `json:"label,omitempty"`
	Type       string   `json:"type"` // query, custom, constant, textbox
	Query      string   `json:"query,omitempty"`
	Values     []string `json:"values,omitempty"`
	Default    string   `json:"default,omitempty"`
	Multi      bool     `json:"multi"`
	IncludeAll bool     `json:"include_all"`
}

// Annotation represents a dashboard annotation
type Annotation struct {
	Name      string `json:"name"`
	Query     string `json:"query,omitempty"`
	Enabled   bool   `json:"enabled"`
	Color     string `json:"color,omitempty"`
	IconColor string `json:"icon_color,omitempty"`
}

// ConvertedAlert represents an alert converted to dogwatch format
type ConvertedAlert struct {
	Rule           *alerting.Rule `json:"rule"`
	SourceQuery    string         `json:"source_query,omitempty"`
	ConversionNote string         `json:"conversion_note,omitempty"`
}

// DetectFormat attempts to detect the format of an import file
func DetectFormat(data []byte) SourcePlatform {
	// Quick heuristics based on JSON structure
	content := string(data)

	// Datadog dashboard indicators
	if contains(content, `"layout_type"`) && contains(content, `"widgets"`) {
		return PlatformDatadog
	}
	if contains(content, `"type": "monitor"`) || contains(content, `"thresholds"`) && contains(content, `"notify_no_data"`) {
		return PlatformDatadog
	}

	// Grafana dashboard indicators
	if contains(content, `"__inputs"`) || contains(content, `"schemaVersion"`) {
		return PlatformGrafana
	}
	if contains(content, `"panels"`) && (contains(content, `"datasource"`) || contains(content, `"gridPos"`)) {
		return PlatformGrafana
	}

	// Prometheus alerting rules indicators
	if contains(content, `groups:`) && contains(content, `rules:`) {
		return PlatformPrometheus
	}
	if contains(content, `"groups"`) && contains(content, `"rules"`) && contains(content, `"expr"`) {
		return PlatformPrometheus
	}

	return PlatformUnknown
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		findSubstring(s, substr) >= 0)
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
