package migration

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dogwatch/internal/alerting"
	"dogwatch/internal/dashboard"

	"github.com/google/uuid"
)

// DatadogDashboard represents a Datadog dashboard JSON export
type DatadogDashboard struct {
	ID                 string              `json:"id,omitempty"`
	Title              string              `json:"title"`
	Description        string              `json:"description,omitempty"`
	LayoutType         string              `json:"layout_type"` // ordered, free
	Widgets            []DatadogWidget     `json:"widgets"`
	TemplateVariables  []DatadogVariable   `json:"template_variables,omitempty"`
	NotifyList         []string            `json:"notify_list,omitempty"`
	ReflowType         string              `json:"reflow_type,omitempty"`
	RestrictedRoles    []string            `json:"restricted_roles,omitempty"`
	AuthorHandle       string              `json:"author_handle,omitempty"`
	AuthorName         string              `json:"author_name,omitempty"`
	CreatedAt          string              `json:"created_at,omitempty"`
	ModifiedAt         string              `json:"modified_at,omitempty"`
	IsReadOnly         bool                `json:"is_read_only,omitempty"`
}

// DatadogWidget represents a widget in a Datadog dashboard
type DatadogWidget struct {
	ID         int64               `json:"id,omitempty"`
	Definition DatadogWidgetDef    `json:"definition"`
	Layout     *DatadogWidgetLayout `json:"layout,omitempty"`
}

// DatadogWidgetDef represents the definition of a Datadog widget
type DatadogWidgetDef struct {
	Type            string               `json:"type"`
	Title           string               `json:"title,omitempty"`
	TitleSize       string               `json:"title_size,omitempty"`
	TitleAlign      string               `json:"title_align,omitempty"`
	Requests        []DatadogRequest     `json:"requests,omitempty"`
	Autoscale       bool                 `json:"autoscale,omitempty"`
	Precision       int                  `json:"precision,omitempty"`
	Time            *DatadogTimeRange    `json:"time,omitempty"`
	CustomLinks     []DatadogCustomLink  `json:"custom_links,omitempty"`
	Text            string               `json:"text,omitempty"`          // For note/free_text
	BackgroundColor string               `json:"background_color,omitempty"`
	FontSize        string               `json:"font_size,omitempty"`
	TextAlign       string               `json:"text_align,omitempty"`
	ShowLegend      bool                 `json:"show_legend,omitempty"`
	LegendSize      string               `json:"legend_size,omitempty"`
	Markers         []DatadogMarker      `json:"markers,omitempty"`
	YAxis           *DatadogYAxis        `json:"yaxis,omitempty"`
	// Group widget fields
	Widgets         []DatadogWidget      `json:"widgets,omitempty"`
	LayoutType      string               `json:"layout_type,omitempty"`
}

// DatadogRequest represents a query request in a widget
type DatadogRequest struct {
	Q               string               `json:"q,omitempty"`      // Datadog query
	Query           *DatadogQueryDef     `json:"query,omitempty"`  // Alternative query format
	Queries         []DatadogQueryDef    `json:"queries,omitempty"`
	ResponseFormat  string               `json:"response_format,omitempty"`
	Style           *DatadogStyle        `json:"style,omitempty"`
	DisplayType     string               `json:"display_type,omitempty"`
	Formulas        []DatadogFormula     `json:"formulas,omitempty"`
	ConditionalFormats []DatadogConditionalFormat `json:"conditional_formats,omitempty"`
	Aggregator      string               `json:"aggregator,omitempty"`
	Comparator      string               `json:"comparator,omitempty"`
	Value           float64              `json:"value,omitempty"`
	Order           string               `json:"order,omitempty"`
	Limit           int                  `json:"limit,omitempty"`
}

// DatadogQueryDef represents a query definition
type DatadogQueryDef struct {
	Name        string `json:"name,omitempty"`
	DataSource  string `json:"data_source,omitempty"`
	Query       string `json:"query,omitempty"`
	Aggregator  string `json:"aggregator,omitempty"`
}

// DatadogFormula represents a formula in Datadog
type DatadogFormula struct {
	Formula string `json:"formula"`
	Alias   string `json:"alias,omitempty"`
}

// DatadogConditionalFormat represents conditional formatting
type DatadogConditionalFormat struct {
	Comparator string  `json:"comparator"`
	Value      float64 `json:"value"`
	Palette    string  `json:"palette"`
}

// DatadogStyle represents styling options
type DatadogStyle struct {
	Palette     string `json:"palette,omitempty"`
	LineType    string `json:"line_type,omitempty"`
	LineWidth   string `json:"line_width,omitempty"`
	FillType    string `json:"fill_type,omitempty"`
}

// DatadogWidgetLayout represents widget positioning
type DatadogWidgetLayout struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// DatadogTimeRange represents a time range
type DatadogTimeRange struct {
	LiveSpan string `json:"live_span,omitempty"`
}

// DatadogCustomLink represents a custom link
type DatadogCustomLink struct {
	Label string `json:"label"`
	Link  string `json:"link"`
}

// DatadogMarker represents a marker/threshold line
type DatadogMarker struct {
	Value       string `json:"value"`
	DisplayType string `json:"display_type,omitempty"`
	Label       string `json:"label,omitempty"`
}

// DatadogYAxis represents Y-axis configuration
type DatadogYAxis struct {
	Min         string `json:"min,omitempty"`
	Max         string `json:"max,omitempty"`
	Scale       string `json:"scale,omitempty"`
	IncludeZero bool   `json:"include_zero,omitempty"`
}

// DatadogVariable represents a template variable
type DatadogVariable struct {
	Name             string   `json:"name"`
	Prefix           string   `json:"prefix,omitempty"`
	Default          string   `json:"default,omitempty"`
	AvailableValues  []string `json:"available_values,omitempty"`
}

// DatadogMonitor represents a Datadog monitor/alert
type DatadogMonitor struct {
	ID              int64               `json:"id,omitempty"`
	Name            string              `json:"name"`
	Type            string              `json:"type"`
	Query           string              `json:"query"`
	Message         string              `json:"message,omitempty"`
	Tags            []string            `json:"tags,omitempty"`
	Options         DatadogMonitorOpts  `json:"options"`
	Priority        int                 `json:"priority,omitempty"`
	RestrictedRoles []string            `json:"restricted_roles,omitempty"`
	CreatedAt       string              `json:"created_at,omitempty"`
	ModifiedAt      string              `json:"modified_at,omitempty"`
	Creator         *DatadogCreator     `json:"creator,omitempty"`
}

// DatadogMonitorOpts represents monitor options
type DatadogMonitorOpts struct {
	Thresholds          DatadogThresholds `json:"thresholds,omitempty"`
	NotifyNoData        bool              `json:"notify_no_data,omitempty"`
	NoDataTimeframe     int               `json:"no_data_timeframe,omitempty"`
	NotifyAudit         bool              `json:"notify_audit,omitempty"`
	TimeoutH            int               `json:"timeout_h,omitempty"`
	RenotifyInterval    int               `json:"renotify_interval,omitempty"`
	EscalationMessage   string            `json:"escalation_message,omitempty"`
	IncludeTags         bool              `json:"include_tags,omitempty"`
	RequireFullWindow   bool              `json:"require_full_window,omitempty"`
	NewHostDelay        int               `json:"new_host_delay,omitempty"`
	EvaluationDelay     int               `json:"evaluation_delay,omitempty"`
	Silenced            map[string]any    `json:"silenced,omitempty"`
}

// DatadogThresholds represents threshold values
type DatadogThresholds struct {
	Critical         *float64 `json:"critical,omitempty"`
	Warning          *float64 `json:"warning,omitempty"`
	OK               *float64 `json:"ok,omitempty"`
	CriticalRecovery *float64 `json:"critical_recovery,omitempty"`
	WarningRecovery  *float64 `json:"warning_recovery,omitempty"`
}

// DatadogCreator represents the creator of a resource
type DatadogCreator struct {
	Email  string `json:"email,omitempty"`
	Handle string `json:"handle,omitempty"`
	Name   string `json:"name,omitempty"`
}

// DatadogImporter handles importing from Datadog
type DatadogImporter struct {
	options DashboardImportOptions
}

// NewDatadogImporter creates a new Datadog importer
func NewDatadogImporter(opts DashboardImportOptions) *DatadogImporter {
	return &DatadogImporter{options: opts}
}

// ParseDashboard parses a Datadog dashboard JSON
func (d *DatadogImporter) ParseDashboard(data []byte) (*DatadogDashboard, error) {
	var dash DatadogDashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		return nil, fmt.Errorf("failed to parse Datadog dashboard: %w", err)
	}
	return &dash, nil
}

// ConvertDashboard converts a Datadog dashboard to dogwatch format
func (d *DatadogImporter) ConvertDashboard(dd *DatadogDashboard) (*ConvertedDashboard, *DashboardResult) {
	result := &DashboardResult{
		SourceName: dd.Title,
		SourceID:   dd.ID,
		Success:    true,
	}

	// Generate dashboard ID
	dashID := uuid.New().String()
	result.TargetID = dashID

	// Convert widgets
	var layout []dashboard.WidgetPosition
	widgetConfigs := make(map[string]WidgetConfig)

	// Track grid position for ordered layouts
	gridY := 0
	gridX := 0
	maxWidth := 12 // Standard 12-column grid

	for _, widget := range dd.Widgets {
		result.WidgetsTotal++

		// Handle group widgets recursively
		if widget.Definition.Type == "group" {
			for _, subWidget := range widget.Definition.Widgets {
				result.WidgetsTotal++
				pos, config, supported := d.convertWidget(subWidget, &gridX, &gridY, maxWidth)
				if supported {
					layout = append(layout, pos)
					widgetConfigs[pos.ID] = config
					result.WidgetsConverted++
				} else {
					result.WidgetsSkipped++
					if !d.options.SkipUnsupportedWidgets {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("Unsupported widget type: %s", subWidget.Definition.Type))
					}
				}
			}
			continue
		}

		pos, config, supported := d.convertWidget(widget, &gridX, &gridY, maxWidth)
		if supported {
			layout = append(layout, pos)
			widgetConfigs[pos.ID] = config
			result.WidgetsConverted++
		} else {
			result.WidgetsSkipped++
			if !d.options.SkipUnsupportedWidgets {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Unsupported widget type: %s", widget.Definition.Type))
			}
		}
	}

	// Convert template variables
	var variables []TemplateVariable
	for _, v := range dd.TemplateVariables {
		variables = append(variables, TemplateVariable{
			Name:    v.Name,
			Label:   v.Name,
			Type:    "query",
			Default: v.Default,
			Values:  v.AvailableValues,
		})
	}

	// Build dashboard name
	name := dd.Title
	if d.options.DashboardNamePrefix != "" {
		name = d.options.DashboardNamePrefix + name
	}

	now := time.Now()
	converted := &ConvertedDashboard{
		Dashboard: &dashboard.Dashboard{
			ID:        dashID,
			Name:      name,
			Layout:    layout,
			IsDefault: d.options.SetAsDefault,
			Created:   now,
			Updated:   now,
		},
		WidgetConfigs: widgetConfigs,
		Variables:     variables,
	}

	return converted, result
}

// convertWidget converts a single Datadog widget to dogwatch format
func (d *DatadogImporter) convertWidget(widget DatadogWidget, gridX, gridY *int, maxWidth int) (dashboard.WidgetPosition, WidgetConfig, bool) {
	widgetID := uuid.New().String()

	// Determine position
	var x, y, w, h int
	if widget.Layout != nil {
		// Free-form layout - use provided coordinates
		x = widget.Layout.X
		y = widget.Layout.Y
		w = widget.Layout.Width
		h = widget.Layout.Height
	} else {
		// Ordered layout - calculate grid position
		w = 4 // Default width
		h = 3 // Default height
		x = *gridX
		y = *gridY

		// Move to next position
		*gridX += w
		if *gridX >= maxWidth {
			*gridX = 0
			*gridY += h
		}
	}

	pos := dashboard.WidgetPosition{
		ID:     widgetID,
		X:      x,
		Y:      y,
		Width:  w,
		Height: h,
	}

	config := WidgetConfig{
		ID:    widgetID,
		Title: widget.Definition.Title,
	}

	// Convert widget type and query
	switch widget.Definition.Type {
	case "timeseries":
		config.Type = "timeseries"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "query_value":
		config.Type = "stat"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)
		config.Options = map[string]any{
			"precision": widget.Definition.Precision,
		}

	case "toplist":
		config.Type = "toplist"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)
		if len(widget.Definition.Requests) > 0 && widget.Definition.Requests[0].Limit > 0 {
			config.Options = map[string]any{
				"limit": widget.Definition.Requests[0].Limit,
			}
		}

	case "heatmap":
		config.Type = "heatmap"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "table", "query_table":
		config.Type = "table"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "note", "free_text":
		config.Type = "text"
		config.Options = map[string]any{
			"content": widget.Definition.Text,
		}
		return pos, config, true

	case "alert_value":
		config.Type = "stat"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "change":
		config.Type = "stat"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)
		config.Options = map[string]any{
			"showChange": true,
		}

	case "distribution":
		config.Type = "histogram"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "geomap":
		config.Type = "geomap"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "hostmap":
		config.Type = "hostmap"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "scatter":
		config.Type = "scatter"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "treemap":
		config.Type = "treemap"
		config.Query, config.SourceQuery = d.convertQueries(widget.Definition.Requests)

	case "log_stream":
		config.Type = "logs"
		if len(widget.Definition.Requests) > 0 {
			config.Query = d.convertLogQuery(widget.Definition.Requests[0].Q)
			config.SourceQuery = widget.Definition.Requests[0].Q
		}

	case "slo":
		config.Type = "slo"
		return pos, config, true

	case "service_map":
		config.Type = "servicemap"
		return pos, config, true

	case "image":
		config.Type = "image"
		return pos, config, true

	case "iframe":
		// Not supported for security reasons
		return pos, config, false

	default:
		// Unsupported widget type
		return pos, config, false
	}

	return pos, config, true
}

// convertQueries converts Datadog queries to DQL
func (d *DatadogImporter) convertQueries(requests []DatadogRequest) (string, string) {
	if len(requests) == 0 {
		return "", ""
	}

	var sourceQueries []string
	var dqlQueries []string

	for _, req := range requests {
		var q string
		if req.Q != "" {
			q = req.Q
		} else if req.Query != nil && req.Query.Query != "" {
			q = req.Query.Query
		} else if len(req.Queries) > 0 {
			for _, qd := range req.Queries {
				if qd.Query != "" {
					q = qd.Query
					break
				}
			}
		}

		if q != "" {
			sourceQueries = append(sourceQueries, q)
			dqlQueries = append(dqlQueries, convertDatadogQuery(q))
		}
	}

	if len(sourceQueries) == 0 {
		return "", ""
	}

	return strings.Join(dqlQueries, " | "), strings.Join(sourceQueries, "; ")
}

// convertDatadogQuery converts a Datadog metric query to DQL
func convertDatadogQuery(ddQuery string) string {
	// Datadog query format: aggregator:metric{tag:value,...}by{groupby}
	// Example: avg:system.cpu.user{host:*}by{host}

	// Parse the query
	q := ddQuery

	// Extract aggregator
	aggMatch := regexp.MustCompile(`^(\w+):`).FindStringSubmatch(q)
	agg := "avg"
	if len(aggMatch) > 1 {
		agg = aggMatch[1]
		q = q[len(aggMatch[0]):]
	}

	// Extract metric name
	metricMatch := regexp.MustCompile(`^([^{]+)`).FindStringSubmatch(q)
	metric := ""
	if len(metricMatch) > 1 {
		metric = strings.TrimSpace(metricMatch[1])
		q = q[len(metricMatch[0]):]
	}

	// Extract filters (tags in curly braces)
	filters := ""
	filtersMatch := regexp.MustCompile(`\{([^}]*)\}`).FindStringSubmatch(q)
	if len(filtersMatch) > 1 {
		filters = filtersMatch[1]
		q = q[len(filtersMatch[0]):]
	}

	// Extract group by
	groupBy := ""
	groupByMatch := regexp.MustCompile(`by\{([^}]*)\}`).FindStringSubmatch(q)
	if len(groupByMatch) > 1 {
		groupBy = groupByMatch[1]
	}

	// Build DQL query
	dql := fmt.Sprintf("metric(%s)", metric)

	// Add filters
	if filters != "" && filters != "*" {
		// Convert tag:value to tag="value"
		filterParts := strings.Split(filters, ",")
		var dqlFilters []string
		for _, f := range filterParts {
			f = strings.TrimSpace(f)
			if f == "" || f == "*" {
				continue
			}
			parts := strings.SplitN(f, ":", 2)
			if len(parts) == 2 {
				dqlFilters = append(dqlFilters, fmt.Sprintf("%s=\"%s\"", parts[0], parts[1]))
			}
		}
		if len(dqlFilters) > 0 {
			dql += fmt.Sprintf(" | filter %s", strings.Join(dqlFilters, " AND "))
		}
	}

	// Add aggregation
	if agg != "" {
		dql += fmt.Sprintf(" | %s()", agg)
	}

	// Add group by
	if groupBy != "" {
		dql += fmt.Sprintf(" | group_by(%s)", groupBy)
	}

	return dql
}

// convertLogQuery converts a Datadog log query to DQL
func (d *DatadogImporter) convertLogQuery(ddQuery string) string {
	if ddQuery == "" {
		return "logs(*)"
	}
	// Simple conversion - Datadog log queries are similar to Lucene
	// Convert to DQL logs query
	return fmt.Sprintf("logs(%s)", ddQuery)
}

// ParseMonitor parses a Datadog monitor JSON
func (d *DatadogImporter) ParseMonitor(data []byte) (*DatadogMonitor, error) {
	var monitor DatadogMonitor
	if err := json.Unmarshal(data, &monitor); err != nil {
		return nil, fmt.Errorf("failed to parse Datadog monitor: %w", err)
	}
	return &monitor, nil
}

// ParseMonitors parses multiple Datadog monitors from JSON array
func (d *DatadogImporter) ParseMonitors(data []byte) ([]DatadogMonitor, error) {
	var monitors []DatadogMonitor
	if err := json.Unmarshal(data, &monitors); err != nil {
		// Try single monitor
		var monitor DatadogMonitor
		if err2 := json.Unmarshal(data, &monitor); err2 != nil {
			return nil, fmt.Errorf("failed to parse Datadog monitors: %w", err)
		}
		return []DatadogMonitor{monitor}, nil
	}
	return monitors, nil
}

// ConvertMonitor converts a Datadog monitor to dogwatch alert rule
func (d *DatadogImporter) ConvertMonitor(monitor *DatadogMonitor, opts AlertImportOptions) (*ConvertedAlert, *AlertResult) {
	return d.ConvertMonitorWithContext(monitor, opts, nil)
}

// ConvertMonitorWithContext converts a Datadog monitor with composite monitor context
func (d *DatadogImporter) ConvertMonitorWithContext(monitor *DatadogMonitor, opts AlertImportOptions, ctx *CompositeMonitorContext) (*ConvertedAlert, *AlertResult) {
	result := &AlertResult{
		SourceName: monitor.Name,
		SourceID:   fmt.Sprintf("%d", monitor.ID),
		Success:    true,
	}

	ruleID := uuid.New().String()
	result.TargetID = ruleID

	// Register this monitor in the composite context for later resolution
	if ctx != nil {
		ctx.RegisterMonitor(monitor.ID, ruleID)
	}

	// Build rule name
	name := monitor.Name
	if opts.AlertNamePrefix != "" {
		name = opts.AlertNamePrefix + name
	}

	// Determine rule type based on monitor type
	ruleType := alerting.RuleTypeThreshold
	isComposite := IsCompositeMonitor(monitor)

	switch monitor.Type {
	case "metric alert":
		ruleType = alerting.RuleTypeThreshold
	case "query alert":
		ruleType = alerting.RuleTypeThreshold
	case "service check":
		ruleType = alerting.RuleTypeThreshold
	case "event alert":
		ruleType = alerting.RuleTypeChange
	case "log alert":
		ruleType = alerting.RuleTypeThreshold
	case "composite":
		ruleType = alerting.RuleTypeComposite
		isComposite = true
	case "anomaly":
		ruleType = alerting.RuleTypeAnomaly
	default:
		// Check if query looks like a composite query even if type doesn't say so
		if isComposite {
			ruleType = alerting.RuleTypeComposite
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Unknown monitor type '%s', defaulting to threshold", monitor.Type))
		}
	}

	// Add composite monitor to pending resolution if context provided
	if isComposite && ctx != nil {
		ctx.AddPendingComposite(monitor, ruleID)
		result.Warnings = append(result.Warnings,
			"Composite monitor - sub-rule references will be resolved after all monitors are imported")
	}

	// Determine severity based on thresholds (stored in labels)
	severityStr := string(alerting.SeverityWarning)
	if monitor.Priority <= 2 {
		severityStr = string(alerting.SeverityCritical)
	}

	// Convert query
	dqlQuery := convertDatadogMonitorQuery(monitor.Query)

	// Get threshold value
	var threshold float64
	condition := "gt"
	if monitor.Options.Thresholds.Critical != nil {
		threshold = *monitor.Options.Thresholds.Critical
	} else if monitor.Options.Thresholds.Warning != nil {
		threshold = *monitor.Options.Thresholds.Warning
		severityStr = string(alerting.SeverityWarning)
	}

	// Parse condition from query if present
	if strings.Contains(monitor.Query, " > ") {
		condition = "gt"
	} else if strings.Contains(monitor.Query, " >= ") {
		condition = "gte"
	} else if strings.Contains(monitor.Query, " < ") {
		condition = "lt"
	} else if strings.Contains(monitor.Query, " <= ") {
		condition = "lte"
	} else if strings.Contains(monitor.Query, " == ") {
		condition = "eq"
	}

	// Convert labels from tags
	labels := make(map[string]string)
	for _, tag := range monitor.Tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			labels[parts[0]] = parts[1]
		} else {
			labels[tag] = "true"
		}
	}
	labels["_severity"] = severityStr
	labels["source"] = "datadog"

	// Build annotations
	annotations := map[string]string{
		"description": monitor.Message,
		"source":      "datadog",
	}
	if monitor.ID > 0 {
		annotations["source_id"] = fmt.Sprintf("%d", monitor.ID)
	}

	// Determine eval interval
	evalInterval := 1 * time.Minute
	if monitor.Options.EvaluationDelay > 0 {
		evalInterval = time.Duration(monitor.Options.EvaluationDelay) * time.Second
	}

	// Convert notify channels
	notifyChannels := opts.DefaultNotifyChannels

	rule := &alerting.Rule{
		ID:             ruleID,
		Name:           name,
		Description:    monitor.Message,
		Type:           ruleType,
		Enabled:        opts.EnableImportedAlerts,
		Labels:         labels,
		Annotations:    annotations,
		Query:          dqlQuery,
		Condition:      condition,
		Threshold:      threshold,
		EvalInterval:   evalInterval,
		NotifyChannels: notifyChannels,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Set anomaly-specific fields
	if ruleType == alerting.RuleTypeAnomaly {
		rule.Sensitivity = 0.7
		rule.Algorithm = "zscore"
	}

	// Handle no-data configuration
	if monitor.Options.NotifyNoData {
		result.Warnings = append(result.Warnings,
			"No-data alerting converted - verify evaluation behavior")
	}

	converted := &ConvertedAlert{
		Rule:           rule,
		SourceQuery:    monitor.Query,
		ConversionNote: fmt.Sprintf("Converted from Datadog %s monitor", monitor.Type),
	}

	return converted, result
}

// convertDatadogMonitorQuery converts a Datadog monitor query to DQL
func convertDatadogMonitorQuery(ddQuery string) string {
	// Monitor queries often have the format:
	// aggregator(timeframe):metric{filters}by{groupby} comparison threshold
	// Example: avg(last_5m):avg:system.cpu.user{*} > 80

	// Remove comparison part
	q := ddQuery
	for _, op := range []string{" > ", " >= ", " < ", " <= ", " == ", " != "} {
		if idx := strings.Index(q, op); idx > 0 {
			q = q[:idx]
		}
	}

	// Check for timeframe wrapper
	timeframeMatch := regexp.MustCompile(`^(\w+)\(([^)]+)\):`).FindStringSubmatch(q)
	if len(timeframeMatch) > 0 {
		// Skip the timeframe wrapper, take the inner query
		q = q[len(timeframeMatch[0]):]
	}

	return convertDatadogQuery(q)
}

// ImportDashboard is a convenience method that parses and converts in one call
func (d *DatadogImporter) ImportDashboard(data []byte) (*ConvertedDashboard, *DashboardResult, error) {
	dash, err := d.ParseDashboard(data)
	if err != nil {
		return nil, nil, err
	}
	converted, result := d.ConvertDashboard(dash)
	return converted, result, nil
}

// ImportMonitors is a convenience method that parses and converts monitors
func (d *DatadogImporter) ImportMonitors(data []byte, opts AlertImportOptions) ([]*ConvertedAlert, []AlertResult, error) {
	monitors, err := d.ParseMonitors(data)
	if err != nil {
		return nil, nil, err
	}

	var alerts []*ConvertedAlert
	var results []AlertResult

	for _, m := range monitors {
		alert, result := d.ConvertMonitor(&m, opts)
		alerts = append(alerts, alert)
		results = append(results, *result)
	}

	return alerts, results, nil
}

// ImportMonitorsWithCompositeResolution imports monitors and resolves composite references
// This is the preferred method when importing multiple monitors that may reference each other
func (d *DatadogImporter) ImportMonitorsWithCompositeResolution(data []byte, opts AlertImportOptions) (*MonitorImportResult, error) {
	monitors, err := d.ParseMonitors(data)
	if err != nil {
		return nil, err
	}

	result := &MonitorImportResult{
		Alerts:  make([]*ConvertedAlert, 0, len(monitors)),
		Results: make([]AlertResult, 0, len(monitors)),
	}

	// Create composite context
	ctx := NewCompositeMonitorContext()

	// First pass: convert all monitors and register IDs
	for i := range monitors {
		m := &monitors[i]
		alert, alertResult := d.ConvertMonitorWithContext(m, opts, ctx)
		result.Alerts = append(result.Alerts, alert)
		result.Results = append(result.Results, *alertResult)
	}

	// Second pass: resolve composite monitor references
	resolutions := ctx.ResolvePendingComposites()
	for _, resolution := range resolutions {
		result.CompositeResolutions = append(result.CompositeResolutions, resolution)

		// Find and update the corresponding rule
		for _, alert := range result.Alerts {
			if alert.Rule.ID == resolution.RuleID {
				if resolution.Success {
					alert.Rule.Expression = resolution.Expression
					alert.Rule.SubRules = resolution.SubRuleIDs
					alert.ConversionNote += fmt.Sprintf("; Composite: %s", resolution.Expression)
				} else {
					alert.ConversionNote += fmt.Sprintf("; Composite resolution failed: %s", resolution.Error)
				}

				// Add warnings to the corresponding result
				for i, r := range result.Results {
					if r.TargetID == resolution.RuleID {
						result.Results[i].Warnings = append(result.Results[i].Warnings, resolution.Warnings...)
						if !resolution.Success {
							result.Results[i].Warnings = append(result.Results[i].Warnings,
								fmt.Sprintf("Composite resolution failed: %s", resolution.Error))
						}
					}
				}
				break
			}
		}
	}

	return result, nil
}

// MonitorImportResult contains the full result of a monitor batch import
type MonitorImportResult struct {
	Alerts               []*ConvertedAlert     `json:"alerts"`
	Results              []AlertResult         `json:"results"`
	CompositeResolutions []CompositeResolution `json:"composite_resolutions,omitempty"`
}
