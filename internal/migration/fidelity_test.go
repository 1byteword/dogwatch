package migration

import (
	"encoding/json"
	"testing"
)

func TestFidelityScoreCalculation(t *testing.T) {
	analyzer := NewFidelityAnalyzer()

	result := &DashboardResult{
		SourceName:       "Test Dashboard",
		WidgetsTotal:     10,
		WidgetsConverted: 8,
		WidgetsSkipped:   2,
		Warnings:         []string{"Query conversion approximate"},
	}

	converted := &ConvertedDashboard{}

	score := analyzer.CalculateFidelityScore(nil, converted, result)

	if score.WidgetFidelity != 80.0 {
		t.Errorf("Expected widget fidelity 80, got %f", score.WidgetFidelity)
	}

	if score.Overall < 50 || score.Overall > 100 {
		t.Errorf("Overall score out of range: %f", score.Overall)
	}

	if len(score.Suggestions) == 0 {
		t.Error("Expected suggestions to be generated")
	}
}

func TestParseDatadogTemplateVariables(t *testing.T) {
	vars := []DatadogVariable{
		{
			Name:            "env",
			Prefix:          "env",
			Default:         "production",
			AvailableValues: []string{"production", "staging", "dev"},
		},
		{
			Name:    "host",
			Default: "*",
		},
	}

	enhanced := ParseDatadogTemplateVariables(vars)

	if len(enhanced) != 2 {
		t.Fatalf("Expected 2 variables, got %d", len(enhanced))
	}

	if enhanced[0].Type != VarTypeQuery {
		t.Errorf("Expected query type for env, got %s", enhanced[0].Type)
	}

	if enhanced[0].Query != "tag:env" {
		t.Errorf("Expected query 'tag:env', got %s", enhanced[0].Query)
	}

	if enhanced[1].Type != VarTypeTextbox {
		t.Errorf("Expected textbox type for host, got %s", enhanced[1].Type)
	}
}

func TestParseGrafanaTemplateVariables(t *testing.T) {
	vars := []GrafanaVariable{
		{
			Name:       "datasource",
			Label:      "Data Source",
			Type:       "datasource",
			Multi:      false,
			IncludeAll: false,
		},
		{
			Name:       "service",
			Label:      "Service",
			Type:       "query",
			Query:      "label_values(up, service)",
			Multi:      true,
			IncludeAll: true,
			AllValue:   ".*",
		},
		{
			Name:  "env",
			Label: "Environment",
			Type:  "custom",
			Options: []GrafanaVarOpt{
				{Text: "Production", Value: "prod", Selected: true},
				{Text: "Staging", Value: "staging"},
			},
		},
	}

	enhanced := ParseGrafanaTemplateVariables(vars)

	if len(enhanced) != 3 {
		t.Fatalf("Expected 3 variables, got %d", len(enhanced))
	}

	// Check datasource
	if enhanced[0].Type != VarTypeDatasource {
		t.Errorf("Expected datasource type, got %s", enhanced[0].Type)
	}

	// Check query variable
	if enhanced[1].Type != VarTypeQuery {
		t.Errorf("Expected query type, got %s", enhanced[1].Type)
	}
	if !enhanced[1].Multi {
		t.Error("Expected multi to be true")
	}
	if !enhanced[1].IncludeAll {
		t.Error("Expected includeAll to be true")
	}

	// Check custom variable
	if enhanced[2].Type != VarTypeCustom {
		t.Errorf("Expected custom type, got %s", enhanced[2].Type)
	}
	if len(enhanced[2].Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(enhanced[2].Options))
	}
}

func TestExtractVariableReferences(t *testing.T) {
	tests := []struct {
		query    string
		expected []string
	}{
		{"$env", []string{"env"}},
		{"${service}", []string{"service"}},
		{"[[host]]", []string{"host"}},
		{"{{namespace}}", []string{"namespace"}},
		{"$service and $env", []string{"env", "service"}},
		{"${service}_${env}_metric", []string{"env", "service"}},
		{"$__interval is builtin", []string{}},
		{"no variables here", []string{}},
	}

	for _, tt := range tests {
		result := extractVariableReferences(tt.query)
		if len(result) != len(tt.expected) {
			t.Errorf("Query %q: expected %d vars, got %d", tt.query, len(tt.expected), len(result))
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("Query %q: expected var %s at pos %d, got %s", tt.query, tt.expected[i], i, v)
			}
		}
	}
}

func TestParseCompositeExpression(t *testing.T) {
	tests := []struct {
		query    string
		expected string
		subCount int
	}{
		{"123 && 456", "a && b", 2},
		{"a: 123 && b: 456 || c: 789", "a && b || c", 3},
		{"100", "a", 1},
	}

	for _, tt := range tests {
		expr, subs := parseCompositeExpression(tt.query)
		if len(subs) != tt.subCount {
			t.Errorf("Query %q: expected %d subs, got %d", tt.query, tt.subCount, len(subs))
		}
		// Expression should contain expected operators
		if tt.subCount > 1 {
			if !containsSubstring(expr, "&&") && containsSubstring(tt.expected, "&&") {
				t.Errorf("Query %q: expected && in expression", tt.query)
			}
			if !containsSubstring(expr, "||") && containsSubstring(tt.expected, "||") {
				t.Errorf("Query %q: expected || in expression", tt.query)
			}
		}
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr) >= 0)
}

func TestParseLogicExpression(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{"a", "ref"},
		{"a && b", "and"},
		{"a || b", "or"},
		{"!a", "not"},
		{"(a && b) || c", "or"},
	}

	for _, tt := range tests {
		tree, err := parseLogicExpression(tt.expr)
		if err != nil {
			t.Errorf("Expression %q: unexpected error %v", tt.expr, err)
			continue
		}
		if tree.Type != tt.expected {
			t.Errorf("Expression %q: expected type %s, got %s", tt.expr, tt.expected, tree.Type)
		}
	}
}

func TestParseDatadogConditionalFormats(t *testing.T) {
	formats := []DatadogConditionalFormat{
		{Comparator: "gt", Value: 90, Palette: "white_on_red"},
		{Comparator: "gt", Value: 70, Palette: "white_on_yellow"},
		{Comparator: "gt", Value: 0, Palette: "white_on_green"},
	}

	result := ParseDatadogConditionalFormats(formats)

	if len(result.Conditions) != 3 {
		t.Fatalf("Expected 3 conditions, got %d", len(result.Conditions))
	}

	// Should be sorted by value
	if result.Conditions[0].Value != 0 {
		t.Error("Conditions not sorted by value")
	}

	if result.Conditions[2].Color != "#ff4444" {
		t.Errorf("Expected red color, got %s", result.Conditions[2].Color)
	}
}

func TestParseDatadogCustomLinks(t *testing.T) {
	links := []DatadogCustomLink{
		{Label: "Dashboard", Link: "/dashboard/abc-123?var=value"},
		{Label: "External", Link: "https://example.com/page"},
		{Label: "Drilldown", Link: "/logs?service={{service}}"},
	}

	result := ParseDatadogCustomLinks(links)

	if len(result) != 3 {
		t.Fatalf("Expected 3 links, got %d", len(result))
	}

	if result[0].Type != "dashboard" {
		t.Errorf("Expected dashboard type, got %s", result[0].Type)
	}
	if result[0].DashboardID != "abc-123" {
		t.Errorf("Expected dashboard ID abc-123, got %s", result[0].DashboardID)
	}

	if result[1].Type != "url" {
		t.Errorf("Expected url type, got %s", result[1].Type)
	}

	if result[2].Type != "drilldown" {
		t.Errorf("Expected drilldown type, got %s", result[2].Type)
	}
}

func TestVariableSubstitution(t *testing.T) {
	sub := NewVariableSubstitution()

	tests := []struct {
		input    string
		platform SourcePlatform
		expected string
	}{
		{"$env", PlatformGrafana, "${env}"},
		{"[[host]]", PlatformGrafana, "${host}"},
		{"$env.value", PlatformDatadog, "${env}"},
		{"{{ .label }}", PlatformPrometheus, "${label}"},
	}

	for _, tt := range tests {
		result := sub.ConvertVariableReferences(tt.input, tt.platform)
		if result != tt.expected {
			t.Errorf("Input %q platform %s: expected %q, got %q",
				tt.input, tt.platform, tt.expected, result)
		}
	}
}

func TestGenerateMigrationPreview(t *testing.T) {
	// Test with Grafana dashboard
	dashboardJSON := `{
		"title": "Test Dashboard",
		"uid": "test-123",
		"schemaVersion": 30,
		"panels": [
			{"type": "graph", "title": "CPU", "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8}},
			{"type": "stat", "title": "Memory", "gridPos": {"x": 12, "y": 0, "w": 12, "h": 8}}
		],
		"templating": {
			"list": [
				{"name": "env", "type": "custom"}
			]
		}
	}`

	preview, err := GenerateMigrationPreview([]byte(dashboardJSON))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if preview.Source != PlatformGrafana {
		t.Errorf("Expected Grafana source, got %s", preview.Source)
	}

	if len(preview.Dashboards) != 1 {
		t.Fatalf("Expected 1 dashboard, got %d", len(preview.Dashboards))
	}

	if preview.Dashboards[0].WidgetCount != 2 {
		t.Errorf("Expected 2 widgets, got %d", preview.Dashboards[0].WidgetCount)
	}

	if preview.EstimatedFidelity == nil {
		t.Error("Expected fidelity score")
	}
}

func TestDatadogCompositeMonitorParsing(t *testing.T) {
	monitor := &DatadogMonitor{
		ID:    100,
		Name:  "Composite Alert",
		Type:  "composite",
		Query: "123 && 456 || 789",
	}

	composite, err := ParseDatadogCompositeMonitor(monitor)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(composite.SubMonitors) != 3 {
		t.Errorf("Expected 3 sub-monitors, got %d", len(composite.SubMonitors))
	}

	// Verify sub-monitor IDs
	expectedIDs := []int64{123, 456, 789}
	for i, sub := range composite.SubMonitors {
		if sub.MonitorID != expectedIDs[i] {
			t.Errorf("Expected monitor ID %d, got %d", expectedIDs[i], sub.MonitorID)
		}
	}

	// Verify logic tree was built
	if composite.LogicTree == nil {
		t.Error("Expected logic tree to be built")
	}
}

func TestEnhancedWidgetConfigSerialization(t *testing.T) {
	config := EnhancedWidgetConfig{
		WidgetConfig: WidgetConfig{
			ID:    "widget-1",
			Type:  "timeseries",
			Title: "CPU Usage",
			Query: "metric(cpu.usage)",
		},
		ConditionalFormats: []ConditionalFormatting{
			{
				Conditions: []FormatCondition{
					{Comparator: "gt", Value: 80, Color: "#ff0000"},
				},
			},
		},
		Links: []WidgetLink{
			{Title: "Details", Type: "dashboard", DashboardID: "dash-1"},
		},
		Thresholds: []ThresholdConfig{
			{Value: 80, Color: "#ff0000", Label: "Critical"},
		},
	}

	// Test serialization
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Test deserialization
	var parsed EnhancedWidgetConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Title != "CPU Usage" {
		t.Error("Title not preserved")
	}

	if len(parsed.ConditionalFormats) != 1 {
		t.Error("Conditional formats not preserved")
	}

	if len(parsed.Links) != 1 {
		t.Error("Links not preserved")
	}

	if len(parsed.Thresholds) != 1 {
		t.Error("Thresholds not preserved")
	}
}

func TestMapDatadogPalette(t *testing.T) {
	tests := []struct {
		palette  string
		expected string
	}{
		{"white_on_red", "#ff4444"},
		{"white_on_yellow", "#ffcc00"},
		{"white_on_green", "#22cc22"},
		{"unknown_palette", "unknown_palette"},
	}

	for _, tt := range tests {
		result := mapDatadogPalette(tt.palette)
		if result != tt.expected {
			t.Errorf("Palette %s: expected %s, got %s", tt.palette, tt.expected, result)
		}
	}
}
