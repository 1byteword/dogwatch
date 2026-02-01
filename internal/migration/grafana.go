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

// GrafanaDashboard represents a Grafana dashboard JSON export
type GrafanaDashboard struct {
	ID            int64              `json:"id,omitempty"`
	UID           string             `json:"uid,omitempty"`
	Title         string             `json:"title"`
	Description   string             `json:"description,omitempty"`
	Tags          []string           `json:"tags,omitempty"`
	Style         string             `json:"style,omitempty"`
	Timezone      string             `json:"timezone,omitempty"`
	Editable      bool               `json:"editable,omitempty"`
	GraphTooltip  int                `json:"graphTooltip,omitempty"`
	Panels        []GrafanaPanel     `json:"panels,omitempty"`
	Rows          []GrafanaRow       `json:"rows,omitempty"` // Legacy format
	Templating    GrafanaTemplating  `json:"templating,omitempty"`
	Annotations   GrafanaAnnotations `json:"annotations,omitempty"`
	Time          GrafanaTime        `json:"time,omitempty"`
	TimePicker    GrafanaTimePicker  `json:"timepicker,omitempty"`
	Refresh       string             `json:"refresh,omitempty"`
	SchemaVersion int                `json:"schemaVersion,omitempty"`
	Version       int                `json:"version,omitempty"`
	Links         []GrafanaLink      `json:"links,omitempty"`
	GnetID        int64              `json:"gnetId,omitempty"`
	// Import metadata
	Inputs        []GrafanaInput     `json:"__inputs,omitempty"`
	Requires      []GrafanaRequire   `json:"__requires,omitempty"`
}

// GrafanaPanel represents a panel in Grafana
type GrafanaPanel struct {
	ID          int64             `json:"id,omitempty"`
	Title       string            `json:"title,omitempty"`
	Type        string            `json:"type"`
	Description string            `json:"description,omitempty"`
	Transparent bool              `json:"transparent,omitempty"`
	Datasource  any               `json:"datasource,omitempty"` // string or object
	GridPos     GrafanaGridPos    `json:"gridPos,omitempty"`
	Targets     []GrafanaTarget   `json:"targets,omitempty"`
	Options     map[string]any    `json:"options,omitempty"`
	FieldConfig *GrafanaFieldConfig `json:"fieldConfig,omitempty"`
	// Legacy panel fields
	XAxis       *GrafanaAxis      `json:"xaxis,omitempty"`
	YAxes       []GrafanaAxis     `json:"yaxes,omitempty"`
	Legend      *GrafanaLegend    `json:"legend,omitempty"`
	Lines       bool              `json:"lines,omitempty"`
	Fill        int               `json:"fill,omitempty"`
	PointRadius int               `json:"pointradius,omitempty"`
	// Specific panel type fields
	ColorMode   string            `json:"colorMode,omitempty"`
	GraphMode   string            `json:"graphMode,omitempty"`
	JustifyMode string            `json:"justifyMode,omitempty"`
	TextMode    string            `json:"textMode,omitempty"`
	Content     string            `json:"content,omitempty"` // For text panels
	Mode        string            `json:"mode,omitempty"`
	// Row/collapsed panels
	Panels      []GrafanaPanel    `json:"panels,omitempty"`
	Collapsed   bool              `json:"collapsed,omitempty"`
	// Alert configuration (legacy)
	Alert       *GrafanaPanelAlert `json:"alert,omitempty"`
}

// GrafanaGridPos represents panel positioning
type GrafanaGridPos struct {
	H int `json:"h"`
	W int `json:"w"`
	X int `json:"x"`
	Y int `json:"y"`
}

// GrafanaTarget represents a query target
type GrafanaTarget struct {
	RefID         string         `json:"refId,omitempty"`
	Datasource    any            `json:"datasource,omitempty"`
	Expr          string         `json:"expr,omitempty"`     // Prometheus
	Query         string         `json:"query,omitempty"`    // Generic/SQL
	RawQuery      bool           `json:"rawQuery,omitempty"`
	Format        string         `json:"format,omitempty"`
	LegendFormat  string         `json:"legendFormat,omitempty"`
	Interval      string         `json:"interval,omitempty"`
	IntervalMs    int            `json:"intervalMs,omitempty"`
	Step          int            `json:"step,omitempty"`
	Instant       bool           `json:"instant,omitempty"`
	Range         bool           `json:"range,omitempty"`
	Hide          bool           `json:"hide,omitempty"`
	// InfluxDB specific
	Measurement   string         `json:"measurement,omitempty"`
	// Elasticsearch/Loki specific
	QueryType     string         `json:"queryType,omitempty"`
	// CloudWatch specific
	Namespace     string         `json:"namespace,omitempty"`
	MetricName    string         `json:"metricName,omitempty"`
}

// GrafanaFieldConfig represents field configuration
type GrafanaFieldConfig struct {
	Defaults  GrafanaFieldDefaults `json:"defaults,omitempty"`
	Overrides []GrafanaOverride    `json:"overrides,omitempty"`
}

// GrafanaFieldDefaults represents default field settings
type GrafanaFieldDefaults struct {
	Unit       string              `json:"unit,omitempty"`
	Decimals   int                 `json:"decimals,omitempty"`
	Min        *float64            `json:"min,omitempty"`
	Max        *float64            `json:"max,omitempty"`
	Color      map[string]any      `json:"color,omitempty"`
	Thresholds *GrafanaThresholds  `json:"thresholds,omitempty"`
	Custom     map[string]any      `json:"custom,omitempty"`
	Mappings   []GrafanaMapping    `json:"mappings,omitempty"`
}

// GrafanaThresholds represents threshold configuration
type GrafanaThresholds struct {
	Mode  string               `json:"mode,omitempty"`
	Steps []GrafanaThresholdStep `json:"steps,omitempty"`
}

// GrafanaThresholdStep represents a threshold step
type GrafanaThresholdStep struct {
	Color string   `json:"color"`
	Value *float64 `json:"value"`
}

// GrafanaMapping represents a value mapping
type GrafanaMapping struct {
	Type    string         `json:"type"`
	Options map[string]any `json:"options,omitempty"`
}

// GrafanaOverride represents a field override
type GrafanaOverride struct {
	Matcher    GrafanaMatcher      `json:"matcher"`
	Properties []GrafanaProperty   `json:"properties"`
}

// GrafanaMatcher represents a field matcher
type GrafanaMatcher struct {
	ID      string `json:"id"`
	Options any    `json:"options,omitempty"`
}

// GrafanaProperty represents a property override
type GrafanaProperty struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
}

// GrafanaAxis represents axis configuration
type GrafanaAxis struct {
	Format  string   `json:"format,omitempty"`
	Label   string   `json:"label,omitempty"`
	LogBase int      `json:"logBase,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Show    bool     `json:"show,omitempty"`
}

// GrafanaLegend represents legend configuration
type GrafanaLegend struct {
	Show    bool `json:"show,omitempty"`
	Values  bool `json:"values,omitempty"`
	Min     bool `json:"min,omitempty"`
	Max     bool `json:"max,omitempty"`
	Current bool `json:"current,omitempty"`
	Total   bool `json:"total,omitempty"`
	Avg     bool `json:"avg,omitempty"`
}

// GrafanaRow represents a legacy row
type GrafanaRow struct {
	Title     string         `json:"title,omitempty"`
	Collapse  bool           `json:"collapse,omitempty"`
	Editable  bool           `json:"editable,omitempty"`
	Height    string         `json:"height,omitempty"`
	Panels    []GrafanaPanel `json:"panels,omitempty"`
}

// GrafanaTemplating represents templating configuration
type GrafanaTemplating struct {
	List []GrafanaVariable `json:"list,omitempty"`
}

// GrafanaVariable represents a template variable
type GrafanaVariable struct {
	Name        string            `json:"name"`
	Label       string            `json:"label,omitempty"`
	Type        string            `json:"type"`
	Query       any               `json:"query,omitempty"` // string or object
	Datasource  any               `json:"datasource,omitempty"`
	Regex       string            `json:"regex,omitempty"`
	Sort        int               `json:"sort,omitempty"`
	Refresh     int               `json:"refresh,omitempty"`
	Multi       bool              `json:"multi,omitempty"`
	IncludeAll  bool              `json:"includeAll,omitempty"`
	AllValue    string            `json:"allValue,omitempty"`
	Current     GrafanaVarCurrent `json:"current,omitempty"`
	Options     []GrafanaVarOpt   `json:"options,omitempty"`
	Hide        int               `json:"hide,omitempty"`
}

// GrafanaVarCurrent represents current variable value
type GrafanaVarCurrent struct {
	Text  any `json:"text,omitempty"` // string or []string
	Value any `json:"value,omitempty"`
}

// GrafanaVarOpt represents a variable option
type GrafanaVarOpt struct {
	Text     string `json:"text"`
	Value    string `json:"value"`
	Selected bool   `json:"selected,omitempty"`
}

// GrafanaAnnotations represents annotations configuration
type GrafanaAnnotations struct {
	List []GrafanaAnnotation `json:"list,omitempty"`
}

// GrafanaAnnotation represents an annotation
type GrafanaAnnotation struct {
	Name       string `json:"name"`
	Datasource any    `json:"datasource,omitempty"`
	Enable     bool   `json:"enable"`
	Expr       string `json:"expr,omitempty"`
	IconColor  string `json:"iconColor,omitempty"`
	Hide       bool   `json:"hide,omitempty"`
	Type       string `json:"type,omitempty"`
	BuiltIn    int    `json:"builtIn,omitempty"`
}

// GrafanaTime represents time range
type GrafanaTime struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// GrafanaTimePicker represents time picker config
type GrafanaTimePicker struct {
	RefreshIntervals []string `json:"refresh_intervals,omitempty"`
	TimeOptions      []string `json:"time_options,omitempty"`
}

// GrafanaLink represents a dashboard link
type GrafanaLink struct {
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	URL         string   `json:"url,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	TargetBlank bool     `json:"targetBlank,omitempty"`
}

// GrafanaInput represents an input for import
type GrafanaInput struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	PluginID    string `json:"pluginId,omitempty"`
	PluginName  string `json:"pluginName,omitempty"`
}

// GrafanaRequire represents a requirement
type GrafanaRequire struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

// GrafanaPanelAlert represents legacy panel alert
type GrafanaPanelAlert struct {
	Name         string               `json:"name"`
	Message      string               `json:"message,omitempty"`
	Conditions   []GrafanaCondition   `json:"conditions,omitempty"`
	Frequency    string               `json:"frequency,omitempty"`
	For          string               `json:"for,omitempty"`
	Handler      int                  `json:"handler,omitempty"`
	Notifications []GrafanaNotification `json:"notifications,omitempty"`
}

// GrafanaCondition represents an alert condition
type GrafanaCondition struct {
	Type      string                `json:"type"`
	Query     GrafanaConditionQuery `json:"query,omitempty"`
	Reducer   GrafanaReducer        `json:"reducer,omitempty"`
	Evaluator GrafanaEvaluator      `json:"evaluator,omitempty"`
	Operator  GrafanaOperator       `json:"operator,omitempty"`
}

// GrafanaConditionQuery represents condition query
type GrafanaConditionQuery struct {
	Params []any `json:"params,omitempty"`
}

// GrafanaReducer represents a condition reducer
type GrafanaReducer struct {
	Type   string `json:"type"`
	Params []any  `json:"params,omitempty"`
}

// GrafanaEvaluator represents condition evaluator
type GrafanaEvaluator struct {
	Type   string `json:"type"`
	Params []any  `json:"params,omitempty"`
}

// GrafanaOperator represents condition operator
type GrafanaOperator struct {
	Type string `json:"type"`
}

// GrafanaNotification represents alert notification
type GrafanaNotification struct {
	UID string `json:"uid,omitempty"`
	ID  int64  `json:"id,omitempty"`
}

// GrafanaAlertRule represents a Grafana alerting rule (v8+)
type GrafanaAlertRule struct {
	UID          string               `json:"uid,omitempty"`
	Title        string               `json:"title"`
	Condition    string               `json:"condition"`
	Data         []GrafanaAlertQuery  `json:"data,omitempty"`
	NoDataState  string               `json:"noDataState,omitempty"`
	ExecErrState string               `json:"execErrState,omitempty"`
	For          string               `json:"for,omitempty"`
	Annotations  map[string]string    `json:"annotations,omitempty"`
	Labels       map[string]string    `json:"labels,omitempty"`
	IsPaused     bool                 `json:"isPaused,omitempty"`
}

// GrafanaAlertQuery represents an alert query
type GrafanaAlertQuery struct {
	RefID         string         `json:"refId"`
	DatasourceUID string         `json:"datasourceUid,omitempty"`
	Model         map[string]any `json:"model,omitempty"`
	QueryType     string         `json:"queryType,omitempty"`
	RelativeRange *GrafanaRange  `json:"relativeTimeRange,omitempty"`
}

// GrafanaRange represents a time range
type GrafanaRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// GrafanaAlertGroup represents a group of alert rules
type GrafanaAlertGroup struct {
	Name     string             `json:"name"`
	Folder   string             `json:"folder,omitempty"`
	Interval string             `json:"interval,omitempty"`
	Rules    []GrafanaAlertRule `json:"rules,omitempty"`
}

// GrafanaImporter handles importing from Grafana
type GrafanaImporter struct {
	options DashboardImportOptions
}

// NewGrafanaImporter creates a new Grafana importer
func NewGrafanaImporter(opts DashboardImportOptions) *GrafanaImporter {
	return &GrafanaImporter{options: opts}
}

// ParseDashboard parses a Grafana dashboard JSON
func (g *GrafanaImporter) ParseDashboard(data []byte) (*GrafanaDashboard, error) {
	var dash GrafanaDashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		return nil, fmt.Errorf("failed to parse Grafana dashboard: %w", err)
	}
	return &dash, nil
}

// ConvertDashboard converts a Grafana dashboard to dogwatch format
func (g *GrafanaImporter) ConvertDashboard(gd *GrafanaDashboard) (*ConvertedDashboard, *DashboardResult) {
	result := &DashboardResult{
		SourceName: gd.Title,
		SourceID:   gd.UID,
		Success:    true,
	}

	dashID := uuid.New().String()
	result.TargetID = dashID

	var layout []dashboard.WidgetPosition
	widgetConfigs := make(map[string]WidgetConfig)

	// Process panels
	panels := gd.Panels

	// Handle legacy row format
	if len(panels) == 0 && len(gd.Rows) > 0 {
		for _, row := range gd.Rows {
			panels = append(panels, row.Panels...)
		}
	}

	for _, panel := range panels {
		result.WidgetsTotal++

		// Handle row panels with collapsed content
		if panel.Type == "row" && len(panel.Panels) > 0 {
			for _, subPanel := range panel.Panels {
				result.WidgetsTotal++
				pos, config, supported := g.convertPanel(subPanel)
				if supported {
					layout = append(layout, pos)
					widgetConfigs[pos.ID] = config
					result.WidgetsConverted++
				} else {
					result.WidgetsSkipped++
					if !g.options.SkipUnsupportedWidgets {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("Unsupported panel type: %s", subPanel.Type))
					}
				}
			}
			continue
		}

		pos, config, supported := g.convertPanel(panel)
		if supported {
			layout = append(layout, pos)
			widgetConfigs[pos.ID] = config
			result.WidgetsConverted++
		} else {
			result.WidgetsSkipped++
			if !g.options.SkipUnsupportedWidgets {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("Unsupported panel type: %s", panel.Type))
			}
		}
	}

	// Convert template variables
	var variables []TemplateVariable
	for _, v := range gd.Templating.List {
		varType := v.Type
		if varType == "datasource" {
			varType = "constant"
		}

		queryStr := ""
		switch q := v.Query.(type) {
		case string:
			queryStr = q
		case map[string]any:
			if qry, ok := q["query"].(string); ok {
				queryStr = qry
			}
		}

		variables = append(variables, TemplateVariable{
			Name:       v.Name,
			Label:      v.Label,
			Type:       varType,
			Query:      queryStr,
			Multi:      v.Multi,
			IncludeAll: v.IncludeAll,
		})
	}

	// Convert annotations
	var annotations []Annotation
	for _, a := range gd.Annotations.List {
		if a.BuiltIn > 0 {
			continue // Skip built-in annotations
		}
		annotations = append(annotations, Annotation{
			Name:      a.Name,
			Query:     a.Expr,
			Enabled:   a.Enable,
			IconColor: a.IconColor,
		})
	}

	// Build dashboard name
	name := gd.Title
	if g.options.DashboardNamePrefix != "" {
		name = g.options.DashboardNamePrefix + name
	}

	now := time.Now()
	converted := &ConvertedDashboard{
		Dashboard: &dashboard.Dashboard{
			ID:        dashID,
			Name:      name,
			Layout:    layout,
			IsDefault: g.options.SetAsDefault,
			Created:   now,
			Updated:   now,
		},
		WidgetConfigs: widgetConfigs,
		Variables:     variables,
		Annotations:   annotations,
	}

	return converted, result
}

// convertPanel converts a single Grafana panel to dogwatch format
func (g *GrafanaImporter) convertPanel(panel GrafanaPanel) (dashboard.WidgetPosition, WidgetConfig, bool) {
	widgetID := uuid.New().String()

	pos := dashboard.WidgetPosition{
		ID:     widgetID,
		X:      panel.GridPos.X,
		Y:      panel.GridPos.Y,
		Width:  panel.GridPos.W,
		Height: panel.GridPos.H,
	}

	config := WidgetConfig{
		ID:    widgetID,
		Title: panel.Title,
	}

	// Convert panel type
	switch panel.Type {
	case "graph", "timeseries":
		config.Type = "timeseries"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "stat", "singlestat":
		config.Type = "stat"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)
		config.Options = map[string]any{
			"colorMode": panel.ColorMode,
			"graphMode": panel.GraphMode,
		}

	case "gauge":
		config.Type = "gauge"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)
		if panel.FieldConfig != nil && panel.FieldConfig.Defaults.Thresholds != nil {
			config.Options = map[string]any{
				"thresholds": g.convertThresholds(panel.FieldConfig.Defaults.Thresholds),
			}
		}

	case "bargauge":
		config.Type = "bargauge"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "table", "table-old":
		config.Type = "table"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "heatmap", "heatmap-new":
		config.Type = "heatmap"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "piechart":
		config.Type = "piechart"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "barchart":
		config.Type = "barchart"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "histogram":
		config.Type = "histogram"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "text":
		config.Type = "text"
		config.Options = map[string]any{
			"content": panel.Content,
			"mode":    panel.Mode,
		}
		return pos, config, true

	case "logs":
		config.Type = "logs"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "alertlist":
		config.Type = "alertlist"
		return pos, config, true

	case "dashlist":
		config.Type = "dashlist"
		return pos, config, true

	case "news":
		config.Type = "text"
		return pos, config, true

	case "geomap":
		config.Type = "geomap"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "nodeGraph":
		config.Type = "servicemap"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "state-timeline", "status-history":
		config.Type = "timeline"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "candlestick":
		config.Type = "candlestick"
		config.Query, config.SourceQuery = g.convertTargets(panel.Targets)

	case "row":
		// Row panels are containers, skip them
		return pos, config, false

	default:
		// Unknown panel type
		return pos, config, false
	}

	return pos, config, true
}

// convertTargets converts Grafana targets/queries to DQL
func (g *GrafanaImporter) convertTargets(targets []GrafanaTarget) (string, string) {
	if len(targets) == 0 {
		return "", ""
	}

	var sourceQueries []string
	var dqlQueries []string

	for _, target := range targets {
		if target.Hide {
			continue
		}

		var q string
		if target.Expr != "" {
			// PromQL query
			q = target.Expr
			sourceQueries = append(sourceQueries, q)
			dqlQueries = append(dqlQueries, convertPromQL(q))
		} else if target.Query != "" {
			// Generic query
			q = target.Query
			sourceQueries = append(sourceQueries, q)
			dqlQueries = append(dqlQueries, q) // Keep as-is for now
		}
	}

	if len(sourceQueries) == 0 {
		return "", ""
	}

	return strings.Join(dqlQueries, " | "), strings.Join(sourceQueries, "; ")
}

// convertPromQL converts PromQL to DQL
func convertPromQL(promQL string) string {
	// PromQL format examples:
	// rate(http_requests_total{job="api"}[5m])
	// sum(rate(http_requests_total[5m])) by (service)
	// histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))

	q := promQL

	// Handle common PromQL functions
	// rate() -> rate()
	// sum() -> sum()
	// avg() -> avg()
	// These are similar in DQL

	// Extract metric name and labels
	metricMatch := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\{([^}]*)\}`).FindStringSubmatch(q)
	if metricMatch == nil {
		// Try without labels
		metricMatch = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\[`).FindStringSubmatch(q)
		if metricMatch == nil {
			// Try just metric name
			metricMatch = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)`).FindStringSubmatch(q)
		}
	}

	metric := ""
	labels := ""
	if len(metricMatch) > 1 {
		metric = metricMatch[1]
	}
	if len(metricMatch) > 2 {
		labels = metricMatch[2]
	}

	// Build DQL
	dql := fmt.Sprintf("metric(%s)", metric)

	// Convert labels to DQL filters
	if labels != "" {
		filters := convertPromLabels(labels)
		if filters != "" {
			dql += fmt.Sprintf(" | filter %s", filters)
		}
	}

	// Detect aggregation function
	if strings.HasPrefix(q, "sum(") {
		dql += " | sum()"
	} else if strings.HasPrefix(q, "avg(") {
		dql += " | avg()"
	} else if strings.HasPrefix(q, "max(") {
		dql += " | max()"
	} else if strings.HasPrefix(q, "min(") {
		dql += " | min()"
	} else if strings.HasPrefix(q, "count(") {
		dql += " | count()"
	} else if strings.HasPrefix(q, "rate(") {
		dql += " | rate()"
	} else if strings.HasPrefix(q, "irate(") {
		dql += " | irate()"
	} else if strings.HasPrefix(q, "increase(") {
		dql += " | increase()"
	} else if strings.Contains(q, "histogram_quantile") {
		// Extract quantile value
		qMatch := regexp.MustCompile(`histogram_quantile\(([0-9.]+)`).FindStringSubmatch(q)
		if len(qMatch) > 1 {
			dql += fmt.Sprintf(" | quantile(%s)", qMatch[1])
		}
	}

	// Handle by() clause for grouping
	byMatch := regexp.MustCompile(`by\s*\(([^)]+)\)`).FindStringSubmatch(q)
	if len(byMatch) > 1 {
		dql += fmt.Sprintf(" | group_by(%s)", byMatch[1])
	}

	return dql
}

// convertPromLabels converts PromQL label selectors to DQL filters
func convertPromLabels(labels string) string {
	if labels == "" {
		return ""
	}

	var filters []string
	// Split by comma, handling quoted values
	parts := splitPromLabels(labels)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle different operators: =, !=, =~, !~
		var op, key, value string
		if strings.Contains(part, "=~") {
			parts := strings.SplitN(part, "=~", 2)
			key = strings.TrimSpace(parts[0])
			value = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			filters = append(filters, fmt.Sprintf("%s =~ \"%s\"", key, value))
		} else if strings.Contains(part, "!~") {
			parts := strings.SplitN(part, "!~", 2)
			key = strings.TrimSpace(parts[0])
			value = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			filters = append(filters, fmt.Sprintf("%s !~ \"%s\"", key, value))
		} else if strings.Contains(part, "!=") {
			parts := strings.SplitN(part, "!=", 2)
			key = strings.TrimSpace(parts[0])
			value = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			filters = append(filters, fmt.Sprintf("%s != \"%s\"", key, value))
		} else if strings.Contains(part, "=") {
			parts := strings.SplitN(part, "=", 2)
			key = strings.TrimSpace(parts[0])
			value = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			op = "="
			filters = append(filters, fmt.Sprintf("%s%s\"%s\"", key, op, value))
		}
	}

	return strings.Join(filters, " AND ")
}

// splitPromLabels splits PromQL label selectors respecting quotes
func splitPromLabels(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)

	for _, c := range s {
		switch {
		case (c == '"' || c == '\'') && !inQuotes:
			inQuotes = true
			quoteChar = c
			current.WriteRune(c)
		case c == quoteChar && inQuotes:
			inQuotes = false
			current.WriteRune(c)
		case c == ',' && !inQuotes:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// convertThresholds converts Grafana thresholds to dogwatch format
func (g *GrafanaImporter) convertThresholds(t *GrafanaThresholds) []map[string]any {
	if t == nil {
		return nil
	}

	var result []map[string]any
	for _, step := range t.Steps {
		threshold := map[string]any{
			"color": step.Color,
		}
		if step.Value != nil {
			threshold["value"] = *step.Value
		}
		result = append(result, threshold)
	}
	return result
}

// ParseAlertRules parses Grafana alert rules (v8+)
func (g *GrafanaImporter) ParseAlertRules(data []byte) ([]GrafanaAlertGroup, error) {
	// Try parsing as array of groups
	var groups []GrafanaAlertGroup
	if err := json.Unmarshal(data, &groups); err == nil {
		return groups, nil
	}

	// Try parsing as single group
	var group GrafanaAlertGroup
	if err := json.Unmarshal(data, &group); err == nil {
		return []GrafanaAlertGroup{group}, nil
	}

	// Try parsing as array of rules
	var rules []GrafanaAlertRule
	if err := json.Unmarshal(data, &rules); err == nil {
		return []GrafanaAlertGroup{{Name: "imported", Rules: rules}}, nil
	}

	return nil, fmt.Errorf("failed to parse Grafana alert rules")
}

// ConvertAlertRule converts a Grafana alert rule to dogwatch format
func (g *GrafanaImporter) ConvertAlertRule(rule GrafanaAlertRule, opts AlertImportOptions) (*ConvertedAlert, *AlertResult) {
	result := &AlertResult{
		SourceName: rule.Title,
		SourceID:   rule.UID,
		Success:    true,
	}

	ruleID := uuid.New().String()
	result.TargetID = ruleID

	// Build rule name
	name := rule.Title
	if opts.AlertNamePrefix != "" {
		name = opts.AlertNamePrefix + name
	}

	// Convert queries to DQL
	var sourceQueries []string
	var dqlQuery string
	for _, data := range rule.Data {
		if model, ok := data.Model["expr"].(string); ok {
			sourceQueries = append(sourceQueries, model)
			dqlQuery = convertPromQL(model)
		}
	}

	// Determine threshold and condition
	threshold := 0.0
	condition := "gt"

	// Labels and annotations
	labels := rule.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["source"] = "grafana"

	annotations := rule.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	if rule.UID != "" {
		annotations["source_id"] = rule.UID
	}

	// Parse for duration
	forDuration := time.Duration(0)
	if rule.For != "" {
		if d, err := time.ParseDuration(rule.For); err == nil {
			forDuration = d
		}
	}

	// Determine severity from labels (stored in labels for later use)
	if sev, ok := labels["severity"]; ok {
		switch sev {
		case "critical":
			labels["_severity"] = string(alerting.SeverityCritical)
		case "warning":
			labels["_severity"] = string(alerting.SeverityWarning)
		case "info":
			labels["_severity"] = string(alerting.SeverityInfo)
		}
	}

	alertRule := &alerting.Rule{
		ID:             ruleID,
		Name:           name,
		Description:    annotations["description"],
		Type:           alerting.RuleTypeThreshold,
		Enabled:        opts.EnableImportedAlerts && !rule.IsPaused,
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

	converted := &ConvertedAlert{
		Rule:           alertRule,
		SourceQuery:    strings.Join(sourceQueries, "; "),
		ConversionNote: "Converted from Grafana alerting rule",
	}

	return converted, result
}

// ConvertLegacyPanelAlert converts a legacy Grafana panel alert
func (g *GrafanaImporter) ConvertLegacyPanelAlert(panel GrafanaPanel, opts AlertImportOptions) (*ConvertedAlert, *AlertResult) {
	if panel.Alert == nil {
		return nil, nil
	}

	alert := panel.Alert
	result := &AlertResult{
		SourceName: alert.Name,
		Success:    true,
	}

	ruleID := uuid.New().String()
	result.TargetID = ruleID

	name := alert.Name
	if opts.AlertNamePrefix != "" {
		name = opts.AlertNamePrefix + name
	}

	// Convert query from panel targets
	dqlQuery, sourceQuery := g.convertTargets(panel.Targets)

	// Parse threshold from conditions
	threshold := 0.0
	condition := "gt"
	if len(alert.Conditions) > 0 {
		cond := alert.Conditions[0]
		if cond.Evaluator.Type == "gt" {
			condition = "gt"
		} else if cond.Evaluator.Type == "lt" {
			condition = "lt"
		}
		if len(cond.Evaluator.Params) > 0 {
			if v, ok := cond.Evaluator.Params[0].(float64); ok {
				threshold = v
			}
		}
	}

	// Parse for duration
	forDuration := time.Duration(0)
	if alert.For != "" {
		if d, err := time.ParseDuration(alert.For); err == nil {
			forDuration = d
		}
	}

	alertRule := &alerting.Rule{
		ID:          ruleID,
		Name:        name,
		Description: alert.Message,
		Type:        alerting.RuleTypeThreshold,
		Enabled:     opts.EnableImportedAlerts,
		Labels: map[string]string{
			"source": "grafana",
		},
		Annotations: map[string]string{
			"description": alert.Message,
		},
		Query:          dqlQuery,
		Condition:      condition,
		Threshold:      threshold,
		ForDuration:    forDuration,
		EvalInterval:   1 * time.Minute,
		NotifyChannels: opts.DefaultNotifyChannels,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	converted := &ConvertedAlert{
		Rule:           alertRule,
		SourceQuery:    sourceQuery,
		ConversionNote: "Converted from Grafana legacy panel alert",
	}

	return converted, result
}

// ImportDashboard is a convenience method that parses and converts in one call
func (g *GrafanaImporter) ImportDashboard(data []byte) (*ConvertedDashboard, *DashboardResult, error) {
	dash, err := g.ParseDashboard(data)
	if err != nil {
		return nil, nil, err
	}
	converted, result := g.ConvertDashboard(dash)
	return converted, result, nil
}

// ImportAlertRules is a convenience method that parses and converts alert rules
func (g *GrafanaImporter) ImportAlertRules(data []byte, opts AlertImportOptions) ([]*ConvertedAlert, []AlertResult, error) {
	groups, err := g.ParseAlertRules(data)
	if err != nil {
		return nil, nil, err
	}

	var alerts []*ConvertedAlert
	var results []AlertResult

	for _, group := range groups {
		for _, rule := range group.Rules {
			alert, result := g.ConvertAlertRule(rule, opts)
			alerts = append(alerts, alert)
			results = append(results, *result)
		}
	}

	return alerts, results, nil
}
