// Package migration provides advanced migration fidelity features for
// importing dashboards and alerts from other observability platforms.
package migration

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// FidelityScore represents the quality of a migration conversion
type FidelityScore struct {
	Overall       float64              `json:"overall"`        // 0-100 percentage
	QueryFidelity float64              `json:"query_fidelity"` // Query conversion accuracy
	WidgetFidelity float64             `json:"widget_fidelity"` // Widget type preservation
	StyleFidelity float64              `json:"style_fidelity"` // Visual style preservation
	LinkFidelity  float64              `json:"link_fidelity"`  // Dashboard links preservation
	Details       []FidelityDetail     `json:"details,omitempty"`
	Suggestions   []string             `json:"suggestions,omitempty"`
}

// FidelityDetail provides detail on a specific aspect of fidelity
type FidelityDetail struct {
	Category    string  `json:"category"`
	Item        string  `json:"item"`
	Score       float64 `json:"score"`
	SourceValue string  `json:"source_value,omitempty"`
	TargetValue string  `json:"target_value,omitempty"`
	Note        string  `json:"note,omitempty"`
}

// TemplateVariableType represents different template variable types
type TemplateVariableType string

const (
	VarTypeQuery    TemplateVariableType = "query"
	VarTypeCustom   TemplateVariableType = "custom"
	VarTypeConstant TemplateVariableType = "constant"
	VarTypeTextbox  TemplateVariableType = "textbox"
	VarTypeInterval TemplateVariableType = "interval"
	VarTypeDatasource TemplateVariableType = "datasource"
	VarTypeAdhoc    TemplateVariableType = "adhoc"
)

// EnhancedTemplateVariable extends TemplateVariable with full fidelity
type EnhancedTemplateVariable struct {
	Name          string               `json:"name"`
	Label         string               `json:"label,omitempty"`
	Type          TemplateVariableType `json:"type"`
	Query         string               `json:"query,omitempty"`
	Regex         string               `json:"regex,omitempty"`
	Sort          int                  `json:"sort,omitempty"` // 0=disabled, 1=asc, 2=desc, 3=asc_num, 4=desc_num
	Refresh       int                  `json:"refresh,omitempty"` // 0=never, 1=on-load, 2=on-time-change
	Multi         bool                 `json:"multi"`
	IncludeAll    bool                 `json:"include_all"`
	AllValue      string               `json:"all_value,omitempty"`
	Current       string               `json:"current,omitempty"`
	Options       []VariableOption     `json:"options,omitempty"`
	Hide          int                  `json:"hide,omitempty"` // 0=show, 1=hide_label, 2=hide
	Datasource    string               `json:"datasource,omitempty"`
	ChainedVars   []string             `json:"chained_vars,omitempty"` // Variables this depends on
	SourceFormat  string               `json:"source_format,omitempty"` // Original platform format
}

// VariableOption represents a single option for a template variable
type VariableOption struct {
	Text     string `json:"text"`
	Value    string `json:"value"`
	Selected bool   `json:"selected,omitempty"`
}

// CompositeMonitor represents a Datadog composite monitor with full logic
type CompositeMonitor struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Expression    string              `json:"expression"` // e.g., "a && b || c"
	SubMonitors   []SubMonitorRef     `json:"sub_monitors"`
	LogicTree     *LogicNode          `json:"logic_tree,omitempty"`
	Priority      int                 `json:"priority,omitempty"`
	Message       string              `json:"message,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
}

// SubMonitorRef references a sub-monitor in a composite
type SubMonitorRef struct {
	Alias       string `json:"alias"` // e.g., "a", "b", "c"
	MonitorID   int64  `json:"monitor_id"`
	MonitorName string `json:"monitor_name,omitempty"`
	Query       string `json:"query,omitempty"` // Original query for reference
	Converted   bool   `json:"converted"`       // Whether it was successfully converted
}

// LogicNode represents a node in the composite logic tree
type LogicNode struct {
	Type      string       `json:"type"` // "and", "or", "not", "ref"
	Left      *LogicNode   `json:"left,omitempty"`
	Right     *LogicNode   `json:"right,omitempty"`
	Child     *LogicNode   `json:"child,omitempty"` // For "not"
	Reference string       `json:"reference,omitempty"` // Monitor alias for "ref"
}

// ConditionalFormatting represents color/style rules based on values
type ConditionalFormatting struct {
	Conditions []FormatCondition `json:"conditions"`
	Background bool              `json:"background"` // Apply to background vs text
}

// FormatCondition represents a single conditional format rule
type FormatCondition struct {
	Comparator  string  `json:"comparator"` // gt, lt, gte, lte, eq, range
	Value       float64 `json:"value"`
	Value2      float64 `json:"value2,omitempty"` // For range comparator
	Color       string  `json:"color"`
	ColorLight  string  `json:"color_light,omitempty"` // For themes
	CustomImage string  `json:"custom_image,omitempty"` // Icon or image URL
	Text        string  `json:"text,omitempty"` // Override display text
}

// WidgetLink represents a link within a dashboard widget
type WidgetLink struct {
	Title         string            `json:"title"`
	Type          string            `json:"type"` // dashboard, url, drilldown
	URL           string            `json:"url,omitempty"`
	DashboardID   string            `json:"dashboard_id,omitempty"`
	DashboardName string            `json:"dashboard_name,omitempty"`
	IncludeVars   bool              `json:"include_vars,omitempty"`
	IncludeTime   bool              `json:"include_time,omitempty"`
	TargetBlank   bool              `json:"target_blank,omitempty"`
	Params        map[string]string `json:"params,omitempty"` // Extra URL parameters
}

// EnhancedWidgetConfig extends WidgetConfig with full fidelity
type EnhancedWidgetConfig struct {
	WidgetConfig
	ConditionalFormats []ConditionalFormatting `json:"conditional_formats,omitempty"`
	Links              []WidgetLink             `json:"links,omitempty"`
	Thresholds         []ThresholdConfig        `json:"thresholds,omitempty"`
	ColorScheme        string                   `json:"color_scheme,omitempty"`
	LegendConfig       *LegendConfig            `json:"legend,omitempty"`
	AxisConfig         *AxisConfig              `json:"axis,omitempty"`
	DisplayOptions     map[string]interface{}   `json:"display_options,omitempty"`
}

// ThresholdConfig represents visual threshold lines
type ThresholdConfig struct {
	Value       float64 `json:"value"`
	Color       string  `json:"color"`
	Label       string  `json:"label,omitempty"`
	LineStyle   string  `json:"line_style,omitempty"` // solid, dashed, dotted
	Fill        string  `json:"fill,omitempty"` // above, below, none
}

// LegendConfig represents legend display options
type LegendConfig struct {
	Show       bool     `json:"show"`
	Position   string   `json:"position,omitempty"` // bottom, right, hidden
	Values     bool     `json:"values,omitempty"`
	Min        bool     `json:"min,omitempty"`
	Max        bool     `json:"max,omitempty"`
	Avg        bool     `json:"avg,omitempty"`
	Current    bool     `json:"current,omitempty"`
	Total      bool     `json:"total,omitempty"`
	SortBy     string   `json:"sort_by,omitempty"`
	SortDesc   bool     `json:"sort_desc,omitempty"`
	HiddenKeys []string `json:"hidden_keys,omitempty"`
}

// AxisConfig represents axis configuration
type AxisConfig struct {
	YAxisMin       *float64 `json:"y_min,omitempty"`
	YAxisMax       *float64 `json:"y_max,omitempty"`
	YAxisScale     string   `json:"y_scale,omitempty"` // linear, log, log2, log10
	YAxisLabel     string   `json:"y_label,omitempty"`
	YAxisUnit      string   `json:"y_unit,omitempty"`
	YAxisDecimals  int      `json:"y_decimals,omitempty"`
	Y2Enabled      bool     `json:"y2_enabled,omitempty"`
	Y2Scale        string   `json:"y2_scale,omitempty"`
	IncludeZero    bool     `json:"include_zero,omitempty"`
}

// FidelityAnalyzer analyzes and calculates migration fidelity
type FidelityAnalyzer struct {
	scores map[string]float64
	details []FidelityDetail
}

// NewFidelityAnalyzer creates a new fidelity analyzer
func NewFidelityAnalyzer() *FidelityAnalyzer {
	return &FidelityAnalyzer{
		scores: make(map[string]float64),
	}
}

// CalculateFidelityScore calculates the overall migration fidelity score
func (f *FidelityAnalyzer) CalculateFidelityScore(
	source interface{},
	converted *ConvertedDashboard,
	result *DashboardResult,
) *FidelityScore {
	score := &FidelityScore{
		Details: make([]FidelityDetail, 0),
	}

	// Calculate widget fidelity
	if result.WidgetsTotal > 0 {
		score.WidgetFidelity = float64(result.WidgetsConverted) / float64(result.WidgetsTotal) * 100
	} else {
		score.WidgetFidelity = 100
	}

	// Calculate query fidelity (estimate based on warnings)
	queryWarnings := 0
	for _, w := range result.Warnings {
		if strings.Contains(strings.ToLower(w), "query") ||
			strings.Contains(strings.ToLower(w), "expression") {
			queryWarnings++
		}
	}
	if result.WidgetsConverted > 0 {
		score.QueryFidelity = (1.0 - float64(queryWarnings)/float64(result.WidgetsConverted)) * 100
		if score.QueryFidelity < 0 {
			score.QueryFidelity = 0
		}
	} else {
		score.QueryFidelity = 100
	}

	// Calculate style fidelity (conditional formats, colors, etc.)
	styleScore := 80.0 // Base score - we preserve most styles
	for _, w := range result.Warnings {
		if strings.Contains(strings.ToLower(w), "style") ||
			strings.Contains(strings.ToLower(w), "color") ||
			strings.Contains(strings.ToLower(w), "format") {
			styleScore -= 5
		}
	}
	score.StyleFidelity = max(styleScore, 0)

	// Calculate link fidelity
	score.LinkFidelity = 75.0 // Base score - links often need adjustment

	// Calculate overall score (weighted average)
	score.Overall = (score.QueryFidelity*0.4 +
		score.WidgetFidelity*0.3 +
		score.StyleFidelity*0.2 +
		score.LinkFidelity*0.1)

	// Generate suggestions
	score.Suggestions = f.generateSuggestions(score, result)

	return score
}

func (f *FidelityAnalyzer) generateSuggestions(score *FidelityScore, result *DashboardResult) []string {
	var suggestions []string

	if score.WidgetFidelity < 90 {
		suggestions = append(suggestions,
			fmt.Sprintf("%.0f%% of widgets were converted. Review skipped widgets and recreate manually if needed.",
				score.WidgetFidelity))
	}

	if score.QueryFidelity < 80 {
		suggestions = append(suggestions,
			"Some queries may need manual adjustment. Test each panel to verify data accuracy.")
	}

	if len(result.Warnings) > 5 {
		suggestions = append(suggestions,
			fmt.Sprintf("%d warnings were generated. Review the warning list for specific issues.",
				len(result.Warnings)))
	}

	if score.LinkFidelity < 80 {
		suggestions = append(suggestions,
			"Dashboard links may need to be reconfigured to point to migrated dashboards.")
	}

	return suggestions
}

// ParseDatadogTemplateVariables parses Datadog template variables with full fidelity
func ParseDatadogTemplateVariables(vars []DatadogVariable) []EnhancedTemplateVariable {
	result := make([]EnhancedTemplateVariable, 0, len(vars))

	for _, v := range vars {
		enhanced := EnhancedTemplateVariable{
			Name:         v.Name,
			Label:        v.Name,
			Current:      v.Default,
			SourceFormat: "datadog",
		}

		// Datadog variables are typically query-based
		if v.Prefix != "" {
			enhanced.Type = VarTypeQuery
			enhanced.Query = fmt.Sprintf("tag:%s", v.Prefix)
		} else if len(v.AvailableValues) > 0 {
			enhanced.Type = VarTypeCustom
			for _, val := range v.AvailableValues {
				enhanced.Options = append(enhanced.Options, VariableOption{
					Text:     val,
					Value:    val,
					Selected: val == v.Default,
				})
			}
		} else {
			enhanced.Type = VarTypeTextbox
		}

		result = append(result, enhanced)
	}

	return result
}

// ParseGrafanaTemplateVariables parses Grafana template variables with full fidelity
func ParseGrafanaTemplateVariables(vars []GrafanaVariable) []EnhancedTemplateVariable {
	result := make([]EnhancedTemplateVariable, 0, len(vars))

	for _, v := range vars {
		enhanced := EnhancedTemplateVariable{
			Name:         v.Name,
			Label:        v.Label,
			Multi:        v.Multi,
			IncludeAll:   v.IncludeAll,
			AllValue:     v.AllValue,
			Regex:        v.Regex,
			Sort:         v.Sort,
			Refresh:      v.Refresh,
			Hide:         v.Hide,
			SourceFormat: "grafana",
		}

		// Convert type
		switch v.Type {
		case "query":
			enhanced.Type = VarTypeQuery
			// Handle query which can be string or object
			switch q := v.Query.(type) {
			case string:
				enhanced.Query = q
			case map[string]interface{}:
				if qry, ok := q["query"].(string); ok {
					enhanced.Query = qry
				}
			}
		case "custom":
			enhanced.Type = VarTypeCustom
			for _, opt := range v.Options {
				enhanced.Options = append(enhanced.Options, VariableOption{
					Text:     opt.Text,
					Value:    opt.Value,
					Selected: opt.Selected,
				})
			}
		case "constant":
			enhanced.Type = VarTypeConstant
		case "textbox":
			enhanced.Type = VarTypeTextbox
		case "interval":
			enhanced.Type = VarTypeInterval
		case "datasource":
			enhanced.Type = VarTypeDatasource
		case "adhoc":
			enhanced.Type = VarTypeAdhoc
		default:
			enhanced.Type = VarTypeCustom
		}

		// Extract current value
		if current := v.Current.Value; current != nil {
			switch cv := current.(type) {
			case string:
				enhanced.Current = cv
			case []interface{}:
				if len(cv) > 0 {
					if s, ok := cv[0].(string); ok {
						enhanced.Current = s
					}
				}
			}
		}

		// Detect chained variables
		enhanced.ChainedVars = extractVariableReferences(enhanced.Query)

		result = append(result, enhanced)
	}

	return result
}

// extractVariableReferences finds variable references in a query string
func extractVariableReferences(query string) []string {
	// Match $var, ${var}, [[var]], {{var}}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\$\{?(\w+)\}?`),
		regexp.MustCompile(`\[\[(\w+)\]\]`),
		regexp.MustCompile(`\{\{(\w+)\}\}`),
	}

	varSet := make(map[string]bool)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(query, -1)
		for _, match := range matches {
			if len(match) > 1 {
				varSet[match[1]] = true
			}
		}
	}

	vars := make([]string, 0, len(varSet))
	for v := range varSet {
		// Skip built-in variables
		if !isBuiltinVariable(v) {
			vars = append(vars, v)
		}
	}
	sort.Strings(vars)
	return vars
}

func isBuiltinVariable(name string) bool {
	builtins := map[string]bool{
		"__interval":    true,
		"__interval_ms": true,
		"__rate_interval": true,
		"__from":        true,
		"__to":          true,
		"__timeFilter":  true,
		"__name":        true,
		"__range":       true,
		"__range_s":     true,
		"__range_ms":    true,
	}
	return builtins[name]
}

// ParseDatadogCompositeMonitor parses a Datadog composite monitor
func ParseDatadogCompositeMonitor(monitor *DatadogMonitor) (*CompositeMonitor, error) {
	if monitor.Type != "composite" {
		return nil, fmt.Errorf("not a composite monitor")
	}

	composite := &CompositeMonitor{
		ID:        fmt.Sprintf("%d", monitor.ID),
		Name:      monitor.Name,
		Priority:  monitor.Priority,
		Message:   monitor.Message,
		Tags:      monitor.Tags,
	}

	// Parse the query to extract expression and sub-monitor references
	// Datadog composite query format: "a && b || c" where a, b, c are monitor IDs
	expr, subs := parseCompositeExpression(monitor.Query)
	composite.Expression = expr
	composite.SubMonitors = subs

	// Build logic tree
	tree, err := parseLogicExpression(expr)
	if err == nil {
		composite.LogicTree = tree
	}

	return composite, nil
}

// parseCompositeExpression parses Datadog composite monitor expression
func parseCompositeExpression(query string) (string, []SubMonitorRef) {
	var subs []SubMonitorRef

	// Extract monitor IDs from query
	// Format can be: "123 && 456 || 789" or with aliases
	idPattern := regexp.MustCompile(`\b(\d+)\b`)
	aliasPattern := regexp.MustCompile(`(\w+)\s*:\s*(\d+)`)

	// First check for alias format
	aliasMatches := aliasPattern.FindAllStringSubmatch(query, -1)
	if len(aliasMatches) > 0 {
		for _, m := range aliasMatches {
			var id int64
			fmt.Sscanf(m[2], "%d", &id)
			subs = append(subs, SubMonitorRef{
				Alias:     m[1],
				MonitorID: id,
			})
		}
	} else {
		// Simple ID format
		matches := idPattern.FindAllStringSubmatch(query, -1)
		for i, m := range matches {
			var id int64
			fmt.Sscanf(m[1], "%d", &id)
			alias := string('a' + byte(i))
			subs = append(subs, SubMonitorRef{
				Alias:     alias,
				MonitorID: id,
			})
		}
	}

	// Clean up expression
	expr := query
	for _, sub := range subs {
		expr = strings.ReplaceAll(expr, fmt.Sprintf("%d", sub.MonitorID), sub.Alias)
	}

	return expr, subs
}

// parseLogicExpression parses a boolean expression into a tree
func parseLogicExpression(expr string) (*LogicNode, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}

	// Handle parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		// Check if these are matching parens
		depth := 0
		for i, c := range expr {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
			}
			if depth == 0 && i < len(expr)-1 {
				break
			}
			if depth == 0 && i == len(expr)-1 {
				return parseLogicExpression(expr[1 : len(expr)-1])
			}
		}
	}

	// Find lowest precedence operator (OR then AND)
	orPos := findOperator(expr, "||")
	if orPos >= 0 {
		left, _ := parseLogicExpression(expr[:orPos])
		right, _ := parseLogicExpression(expr[orPos+2:])
		return &LogicNode{
			Type:  "or",
			Left:  left,
			Right: right,
		}, nil
	}

	andPos := findOperator(expr, "&&")
	if andPos >= 0 {
		left, _ := parseLogicExpression(expr[:andPos])
		right, _ := parseLogicExpression(expr[andPos+2:])
		return &LogicNode{
			Type:  "and",
			Left:  left,
			Right: right,
		}, nil
	}

	// Handle NOT
	if strings.HasPrefix(expr, "!") {
		child, _ := parseLogicExpression(expr[1:])
		return &LogicNode{
			Type:  "not",
			Child: child,
		}, nil
	}

	// Must be a reference
	return &LogicNode{
		Type:      "ref",
		Reference: strings.TrimSpace(expr),
	}, nil
}

func findOperator(expr, op string) int {
	depth := 0
	for i := 0; i < len(expr)-len(op)+1; i++ {
		if expr[i] == '(' {
			depth++
		} else if expr[i] == ')' {
			depth--
		} else if depth == 0 && expr[i:i+len(op)] == op {
			return i
		}
	}
	return -1
}

// ConvertCompositeMonitor converts a Datadog composite monitor to dogwatch format
func ConvertCompositeMonitor(composite *CompositeMonitor, subAlerts map[string]string) (*ConvertedAlert, *AlertResult) {
	result := &AlertResult{
		SourceName: composite.Name,
		SourceID:   composite.ID,
		Success:    true,
	}

	// Build expression mapping sub-monitor aliases to converted rule IDs
	convertedExpr := composite.Expression
	for _, sub := range composite.SubMonitors {
		if ruleID, ok := subAlerts[sub.Alias]; ok {
			convertedExpr = strings.ReplaceAll(convertedExpr, sub.Alias, ruleID)
			sub.Converted = true
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Sub-monitor %s (ID: %d) not found in converted alerts", sub.Alias, sub.MonitorID))
		}
	}

	// For now, we'll store the composite as a rule with expression
	// The alerting engine will need to evaluate this
	ruleJSON, _ := json.Marshal(map[string]interface{}{
		"type":         "composite",
		"expression":   composite.Expression,
		"converted_expr": convertedExpr,
		"sub_monitors": composite.SubMonitors,
		"logic_tree":   composite.LogicTree,
	})

	// Note: ruleJSON used for storage in practice
	_ = ruleJSON

	return &ConvertedAlert{
		SourceQuery:    composite.Expression,
		ConversionNote: fmt.Sprintf("Composite monitor with %d sub-monitors", len(composite.SubMonitors)),
		Rule:           nil, // Will be set by caller with proper alerting.Rule
	}, result
}

// ParseDatadogConditionalFormats parses Datadog conditional formatting
func ParseDatadogConditionalFormats(formats []DatadogConditionalFormat) ConditionalFormatting {
	result := ConditionalFormatting{
		Conditions: make([]FormatCondition, 0, len(formats)),
	}

	for _, f := range formats {
		cond := FormatCondition{
			Comparator: f.Comparator,
			Value:      f.Value,
		}

		// Map Datadog palette to color
		cond.Color = mapDatadogPalette(f.Palette)

		result.Conditions = append(result.Conditions, cond)
	}

	// Sort by value for proper threshold evaluation
	sort.Slice(result.Conditions, func(i, j int) bool {
		return result.Conditions[i].Value < result.Conditions[j].Value
	})

	return result
}

func mapDatadogPalette(palette string) string {
	// Map Datadog palettes to standard colors
	palettes := map[string]string{
		"white_on_red":    "#ff4444",
		"red_on_white":    "#ff4444",
		"white_on_yellow": "#ffcc00",
		"yellow_on_white": "#ffcc00",
		"white_on_green":  "#22cc22",
		"green_on_white":  "#22cc22",
		"white_on_gray":   "#888888",
		"gray_on_white":   "#888888",
		"custom_bg":       "#4488ff",
		"custom_text":     "#4488ff",
	}

	if color, ok := palettes[palette]; ok {
		return color
	}
	return palette // Return as-is if not recognized
}

// ParseDatadogCustomLinks parses Datadog widget custom links
func ParseDatadogCustomLinks(links []DatadogCustomLink) []WidgetLink {
	result := make([]WidgetLink, 0, len(links))

	for _, l := range links {
		link := WidgetLink{
			Title: l.Label,
		}

		// Detect link type based on URL pattern
		url := l.Link
		if strings.Contains(url, "/dashboard/") {
			link.Type = "dashboard"
			// Extract dashboard ID if possible
			if parts := strings.Split(url, "/dashboard/"); len(parts) > 1 {
				idParts := strings.Split(parts[1], "?")
				link.DashboardID = idParts[0]
			}
		} else if strings.Contains(url, "{{") || strings.Contains(url, "$") {
			link.Type = "drilldown"
		} else {
			link.Type = "url"
		}

		link.URL = url

		// Parse URL parameters
		link.Params = make(map[string]string)
		if strings.Contains(url, "?") {
			parts := strings.SplitN(url, "?", 2)
			if len(parts) > 1 {
				for _, param := range strings.Split(parts[1], "&") {
					kv := strings.SplitN(param, "=", 2)
					if len(kv) == 2 {
						link.Params[kv[0]] = kv[1]
					}
				}
			}
		}

		// Check for time and variable inclusion
		link.IncludeVars = strings.Contains(url, "$")
		link.IncludeTime = strings.Contains(url, "from=") || strings.Contains(url, "to=")

		result = append(result, link)
	}

	return result
}

// ParseGrafanaLinks parses Grafana dashboard links
func ParseGrafanaLinks(links []GrafanaLink) []WidgetLink {
	result := make([]WidgetLink, 0, len(links))

	for _, l := range links {
		link := WidgetLink{
			Title:       l.Title,
			TargetBlank: l.TargetBlank,
		}

		switch l.Type {
		case "dashboards":
			link.Type = "dashboard"
			if len(l.Tags) > 0 {
				link.Params = map[string]string{
					"tags": strings.Join(l.Tags, ","),
				}
			}
		case "link":
			link.Type = "url"
			link.URL = l.URL
		default:
			link.Type = l.Type
			link.URL = l.URL
		}

		result = append(result, link)
	}

	return result
}

// VariableSubstitution handles variable substitution between platforms
type VariableSubstitution struct {
	patterns map[string]*regexp.Regexp
}

// NewVariableSubstitution creates a new variable substitution handler
func NewVariableSubstitution() *VariableSubstitution {
	return &VariableSubstitution{
		patterns: map[string]*regexp.Regexp{
			"grafana_bracket": regexp.MustCompile(`\[\[(\w+)\]\]`),
			"grafana_dollar":  regexp.MustCompile(`\$(\w+)`),
			"grafana_brace":   regexp.MustCompile(`\$\{(\w+)(?::\w+)?\}`),
			"datadog_tmpl":    regexp.MustCompile(`\$(\w+)\.value`),
			"prometheus":      regexp.MustCompile(`\{\{(\s*\.\w+\s*)\}\}`),
		},
	}
}

// ConvertVariableReferences converts variable references to dogwatch format
func (v *VariableSubstitution) ConvertVariableReferences(query string, platform SourcePlatform) string {
	result := query

	switch platform {
	case PlatformGrafana:
		// Convert [[var]] to ${var}
		result = v.patterns["grafana_bracket"].ReplaceAllString(result, "${$1}")
		// Normalize $var to ${var}
		result = v.patterns["grafana_dollar"].ReplaceAllStringFunc(result, func(m string) string {
			// Don't double-wrap if already ${var}
			if strings.HasPrefix(m, "${") {
				return m
			}
			name := m[1:]
			return fmt.Sprintf("${%s}", name)
		})
	case PlatformDatadog:
		// Convert $var.value to ${var}
		result = v.patterns["datadog_tmpl"].ReplaceAllString(result, "${$1}")
	case PlatformPrometheus:
		// Convert {{ .label }} to ${label}
		result = v.patterns["prometheus"].ReplaceAllStringFunc(result, func(m string) string {
			inner := strings.Trim(m, "{}")
			inner = strings.TrimPrefix(strings.TrimSpace(inner), ".")
			inner = strings.TrimSpace(inner)
			return fmt.Sprintf("${%s}", inner)
		})
	}

	return result
}

// EnhancedConvertedDashboard extends ConvertedDashboard with fidelity info
type EnhancedConvertedDashboard struct {
	*ConvertedDashboard
	FidelityScore     *FidelityScore             `json:"fidelity_score,omitempty"`
	EnhancedVariables []EnhancedTemplateVariable `json:"enhanced_variables,omitempty"`
	EnhancedWidgets   []EnhancedWidgetConfig     `json:"enhanced_widgets,omitempty"`
	DashboardLinks    []WidgetLink               `json:"dashboard_links,omitempty"`
}

// MigrationPreview provides a detailed preview of what will be migrated
type MigrationPreview struct {
	Source            SourcePlatform           `json:"source"`
	EstimatedFidelity *FidelityScore           `json:"estimated_fidelity"`
	Dashboards        []DashboardPreview       `json:"dashboards,omitempty"`
	Alerts            []AlertPreview           `json:"alerts,omitempty"`
	Variables         []VariablePreview        `json:"variables,omitempty"`
	RequiresManual    []string                 `json:"requires_manual,omitempty"`
	Timestamp         time.Time                `json:"timestamp"`
}

// DashboardPreview provides preview info for a dashboard
type DashboardPreview struct {
	SourceName    string   `json:"source_name"`
	SourceID      string   `json:"source_id,omitempty"`
	WidgetCount   int      `json:"widget_count"`
	SupportedPct  float64  `json:"supported_pct"`
	Unsupported   []string `json:"unsupported,omitempty"`
	VariableCount int      `json:"variable_count"`
	LinkCount     int      `json:"link_count"`
}

// AlertPreview provides preview info for an alert
type AlertPreview struct {
	SourceName  string   `json:"source_name"`
	SourceID    string   `json:"source_id,omitempty"`
	AlertType   string   `json:"alert_type"`
	QueryType   string   `json:"query_type"`
	Convertible bool     `json:"convertible"`
	Warnings    []string `json:"warnings,omitempty"`
}

// VariablePreview provides preview info for a template variable
type VariablePreview struct {
	Name         string `json:"name"`
	SourceType   string `json:"source_type"`
	TargetType   string `json:"target_type"`
	Convertible  bool   `json:"convertible"`
	ChainedTo    []string `json:"chained_to,omitempty"`
}

// GenerateMigrationPreview creates a detailed preview of a migration
func GenerateMigrationPreview(data []byte) (*MigrationPreview, error) {
	format := DetectFormat(data)
	if format == PlatformUnknown {
		return nil, fmt.Errorf("unknown format")
	}

	preview := &MigrationPreview{
		Source:    format,
		Timestamp: time.Now(),
	}

	switch format {
	case PlatformDatadog:
		// Try parsing as dashboard
		importer := NewDatadogImporter(DashboardImportOptions{})
		if dash, err := importer.ParseDashboard(data); err == nil {
			dp := DashboardPreview{
				SourceName:    dash.Title,
				SourceID:      dash.ID,
				WidgetCount:   countDatadogWidgets(dash.Widgets),
				VariableCount: len(dash.TemplateVariables),
			}
			dp.SupportedPct = estimateDatadogWidgetSupport(dash.Widgets)
			preview.Dashboards = append(preview.Dashboards, dp)

			for _, v := range dash.TemplateVariables {
				preview.Variables = append(preview.Variables, VariablePreview{
					Name:        v.Name,
					SourceType:  "datadog_tag",
					TargetType:  "query",
					Convertible: true,
				})
			}
		}

		// Try parsing as monitors
		if monitors, err := importer.ParseMonitors(data); err == nil {
			for _, m := range monitors {
				preview.Alerts = append(preview.Alerts, AlertPreview{
					SourceName:  m.Name,
					SourceID:    fmt.Sprintf("%d", m.ID),
					AlertType:   m.Type,
					QueryType:   "metric",
					Convertible: m.Type != "rum" && m.Type != "synthetics",
				})
			}
		}

	case PlatformGrafana:
		importer := NewGrafanaImporter(DashboardImportOptions{})
		if dash, err := importer.ParseDashboard(data); err == nil {
			dp := DashboardPreview{
				SourceName:    dash.Title,
				SourceID:      dash.UID,
				WidgetCount:   len(dash.Panels),
				VariableCount: len(dash.Templating.List),
				LinkCount:     len(dash.Links),
			}
			dp.SupportedPct = estimateGrafanaPanelSupport(dash.Panels)
			preview.Dashboards = append(preview.Dashboards, dp)

			enhancedVars := ParseGrafanaTemplateVariables(dash.Templating.List)
			for _, v := range enhancedVars {
				preview.Variables = append(preview.Variables, VariablePreview{
					Name:        v.Name,
					SourceType:  string(v.Type),
					TargetType:  string(v.Type),
					Convertible: v.Type != VarTypeAdhoc,
					ChainedTo:   v.ChainedVars,
				})
			}
		}

	case PlatformPrometheus:
		if rules, err := ParsePrometheusRules(data); err == nil {
			for _, g := range rules.Groups {
				for _, r := range g.Rules {
					if r.Alert != "" {
						preview.Alerts = append(preview.Alerts, AlertPreview{
							SourceName:  r.Alert,
							AlertType:   "prometheus",
							QueryType:   "promql",
							Convertible: true,
						})
					}
				}
			}
		}
	}

	// Estimate overall fidelity
	preview.EstimatedFidelity = estimateOverallFidelity(preview)

	return preview, nil
}

func countDatadogWidgets(widgets []DatadogWidget) int {
	count := 0
	for _, w := range widgets {
		count++
		if w.Definition.Type == "group" {
			count += len(w.Definition.Widgets)
		}
	}
	return count
}

func estimateDatadogWidgetSupport(widgets []DatadogWidget) float64 {
	if len(widgets) == 0 {
		return 100
	}

	supported := 0
	total := 0

	for _, w := range widgets {
		total++
		if isDatadogWidgetSupported(w.Definition.Type) {
			supported++
		}
		for _, sub := range w.Definition.Widgets {
			total++
			if isDatadogWidgetSupported(sub.Definition.Type) {
				supported++
			}
		}
	}

	return float64(supported) / float64(total) * 100
}

func isDatadogWidgetSupported(widgetType string) bool {
	supported := map[string]bool{
		"timeseries":   true,
		"query_value":  true,
		"toplist":      true,
		"heatmap":      true,
		"table":        true,
		"query_table":  true,
		"note":         true,
		"free_text":    true,
		"alert_value":  true,
		"change":       true,
		"distribution": true,
		"geomap":       true,
		"hostmap":      true,
		"scatter":      true,
		"treemap":      true,
		"log_stream":   true,
		"slo":          true,
		"service_map":  true,
		"image":        true,
		"group":        true,
	}
	return supported[widgetType]
}

func estimateGrafanaPanelSupport(panels []GrafanaPanel) float64 {
	if len(panels) == 0 {
		return 100
	}

	supported := 0
	total := 0

	for _, p := range panels {
		total++
		if isGrafanaPanelSupported(p.Type) {
			supported++
		}
		for range p.Panels {
			total++
			supported++ // Nested panels are usually supported
		}
	}

	return float64(supported) / float64(total) * 100
}

func isGrafanaPanelSupported(panelType string) bool {
	supported := map[string]bool{
		"graph":          true,
		"timeseries":     true,
		"stat":           true,
		"singlestat":     true,
		"gauge":          true,
		"bargauge":       true,
		"table":          true,
		"table-old":      true,
		"heatmap":        true,
		"heatmap-new":    true,
		"piechart":       true,
		"barchart":       true,
		"histogram":      true,
		"text":           true,
		"logs":           true,
		"alertlist":      true,
		"dashlist":       true,
		"geomap":         true,
		"nodeGraph":      true,
		"state-timeline": true,
		"status-history": true,
		"candlestick":    true,
		"row":            true,
	}
	return supported[panelType]
}

func estimateOverallFidelity(preview *MigrationPreview) *FidelityScore {
	score := &FidelityScore{
		Overall:        80, // Base estimate
		QueryFidelity:  75,
		WidgetFidelity: 85,
		StyleFidelity:  70,
		LinkFidelity:   60,
	}

	// Adjust based on dashboard support
	for _, d := range preview.Dashboards {
		if d.SupportedPct < 80 {
			score.WidgetFidelity -= 10
		}
	}

	// Adjust based on variable complexity
	for _, v := range preview.Variables {
		if !v.Convertible {
			score.QueryFidelity -= 5
		}
		if len(v.ChainedTo) > 0 {
			score.QueryFidelity -= 2 // Chained variables are tricky
		}
	}

	// Recalculate overall
	score.Overall = (score.QueryFidelity*0.4 +
		score.WidgetFidelity*0.3 +
		score.StyleFidelity*0.2 +
		score.LinkFidelity*0.1)

	return score
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
