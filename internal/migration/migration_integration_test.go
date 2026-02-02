package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDatadogDashboardImport_EndToEnd tests complete Datadog dashboard import flow
func TestDatadogDashboardImport_EndToEnd(t *testing.T) {
	dashJSON := `{
		"id": "abc-123",
		"title": "Production Overview Dashboard",
		"layout_type": "ordered",
		"widgets": [
			{
				"id": 1,
				"layout": {"x": 0, "y": 0, "width": 6, "height": 4},
				"definition": {
					"type": "timeseries",
					"title": "Request Rate",
					"requests": [
						{
							"q": "avg:nginx.requests{env:prod}by{service}",
							"display_type": "line",
							"style": {"palette": "dog_classic"}
						}
					],
					"yaxis": {"min": "0"},
					"markers": [
						{"value": "y = 1000", "display_type": "error dashed"}
					]
				}
			},
			{
				"id": 2,
				"layout": {"x": 6, "y": 0, "width": 6, "height": 4},
				"definition": {
					"type": "query_value",
					"title": "Active Users",
					"requests": [
						{"q": "sum:app.users.active{*}"}
					],
					"precision": 0,
					"conditional_formats": [
						{"comparator": ">", "value": 1000, "palette": "white_on_green"},
						{"comparator": "<", "value": 100, "palette": "white_on_red"}
					]
				}
			},
			{
				"id": 3,
				"layout": {"x": 0, "y": 4, "width": 12, "height": 3},
				"definition": {
					"type": "toplist",
					"title": "Top Services by Errors",
					"requests": [
						{"q": "top(sum:errors{*}by{service},10,'sum','desc')"}
					]
				}
			},
			{
				"id": 4,
				"layout": {"x": 0, "y": 7, "width": 12, "height": 4},
				"definition": {
					"type": "heatmap",
					"title": "Request Latency Distribution",
					"requests": [
						{"q": "avg:trace.request.duration{*}"}
					]
				}
			},
			{
				"id": 5,
				"layout": {"x": 0, "y": 11, "width": 4, "height": 2},
				"definition": {
					"type": "note",
					"content": "This dashboard shows production metrics.\n\n**Important:** Check alerts for anomalies.",
					"background_color": "gray",
					"font_size": "14",
					"text_align": "left"
				}
			},
			{
				"id": 6,
				"layout": {"x": 4, "y": 11, "width": 8, "height": 4},
				"definition": {
					"type": "group",
					"title": "Database Metrics",
					"layout_type": "ordered",
					"widgets": [
						{
							"id": 61,
							"definition": {
								"type": "timeseries",
								"title": "Query Duration",
								"requests": [{"q": "avg:db.query.duration{*}"}]
							}
						},
						{
							"id": 62,
							"definition": {
								"type": "query_value",
								"title": "Connections",
								"requests": [{"q": "sum:db.connections{*}"}]
							}
						}
					]
				}
			}
		],
		"template_variables": [
			{"name": "env", "default": "prod", "prefix": "environment"},
			{"name": "service", "default": "*", "prefix": "service"},
			{"name": "region", "default": "us-east-1", "available_values": ["us-east-1", "us-west-2", "eu-west-1"]}
		]
	}`

	opts := DashboardImportOptions{
		SkipUnsupportedWidgets: true,
		DashboardNamePrefix:    "[Datadog] ",
	}

	importer := NewDatadogImporter(opts)
	converted, result, err := importer.ImportDashboard([]byte(dashJSON))

	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify result
	if !result.Success {
		t.Error("Expected success")
	}

	// Count total widgets including nested
	expectedWidgets := 6 + 2 // 6 top-level + 2 nested in group
	if result.WidgetsTotal != expectedWidgets {
		t.Errorf("Expected %d widgets, got %d", expectedWidgets, result.WidgetsTotal)
	}

	// Verify dashboard name
	if converted.Dashboard.Name != "[Datadog] Production Overview Dashboard" {
		t.Errorf("Unexpected dashboard name: %s", converted.Dashboard.Name)
	}

	// Verify variables
	if len(converted.Variables) != 3 {
		t.Errorf("Expected 3 variables, got %d", len(converted.Variables))
	}

	// Check variable types
	for _, v := range converted.Variables {
		if v.Name == "region" && len(v.Values) != 3 {
			t.Errorf("Expected region variable to have 3 values, got %d", len(v.Values))
		}
	}

	// Verify widget configs
	if len(converted.WidgetConfigs) == 0 {
		t.Error("Expected widget configs to be populated")
	}
}

// TestGrafanaDashboardImport_EndToEnd tests complete Grafana dashboard import flow
func TestGrafanaDashboardImport_EndToEnd(t *testing.T) {
	dashJSON := `{
		"uid": "grafana-dash-123",
		"title": "Kubernetes Cluster Metrics",
		"schemaVersion": 27,
		"tags": ["kubernetes", "monitoring"],
		"timezone": "browser",
		"panels": [
			{
				"id": 1,
				"title": "CPU Usage by Namespace",
				"type": "timeseries",
				"gridPos": {"x": 0, "y": 0, "w": 12, "h": 8},
				"targets": [
					{
						"refId": "A",
						"expr": "sum(rate(container_cpu_usage_seconds_total{namespace!=\"\"}[5m])) by (namespace)",
						"legendFormat": "{{namespace}}"
					}
				],
				"fieldConfig": {
					"defaults": {
						"unit": "percentunit",
						"min": 0,
						"max": 1
					}
				},
				"options": {
					"legend": {"displayMode": "table", "placement": "right"}
				}
			},
			{
				"id": 2,
				"title": "Memory Usage",
				"type": "stat",
				"gridPos": {"x": 12, "y": 0, "w": 6, "h": 4},
				"targets": [
					{
						"refId": "A",
						"expr": "sum(container_memory_usage_bytes) / sum(machine_memory_bytes) * 100"
					}
				],
				"fieldConfig": {
					"defaults": {
						"unit": "percent",
						"thresholds": {
							"mode": "absolute",
							"steps": [
								{"value": null, "color": "green"},
								{"value": 70, "color": "yellow"},
								{"value": 90, "color": "red"}
							]
						}
					}
				}
			},
			{
				"id": 3,
				"title": "Pod Status",
				"type": "table",
				"gridPos": {"x": 18, "y": 0, "w": 6, "h": 8},
				"targets": [
					{
						"refId": "A",
						"expr": "kube_pod_status_phase{phase!=\"Running\"}",
						"instant": true,
						"format": "table"
					}
				],
				"transformations": [
					{"id": "organize", "options": {"excludeByName": {"Time": true}}}
				]
			},
			{
				"id": 4,
				"title": "Logs",
				"type": "logs",
				"gridPos": {"x": 0, "y": 8, "w": 24, "h": 6},
				"targets": [
					{
						"refId": "A",
						"expr": "{namespace=~\"$namespace\", pod=~\"$pod\"}",
						"datasource": "loki"
					}
				]
			},
			{
				"id": 5,
				"title": "Row - Network",
				"type": "row",
				"gridPos": {"x": 0, "y": 14, "w": 24, "h": 1},
				"collapsed": false,
				"panels": [
					{
						"id": 51,
						"title": "Network I/O",
						"type": "timeseries",
						"gridPos": {"x": 0, "y": 15, "w": 12, "h": 6},
						"targets": [
							{"refId": "A", "expr": "rate(container_network_receive_bytes_total[5m])"}
						]
					}
				]
			}
		],
		"templating": {
			"list": [
				{
					"name": "namespace",
					"label": "Namespace",
					"type": "query",
					"query": "label_values(kube_pod_info, namespace)",
					"multi": true,
					"includeAll": true,
					"allValue": ".*",
					"current": {"text": "All", "value": ["$__all"]},
					"refresh": 2
				},
				{
					"name": "pod",
					"label": "Pod",
					"type": "query",
					"query": "label_values(kube_pod_info{namespace=~\"$namespace\"}, pod)",
					"multi": true,
					"includeAll": true,
					"refresh": 2
				},
				{
					"name": "interval",
					"type": "interval",
					"options": [
						{"text": "1m", "value": "1m"},
						{"text": "5m", "value": "5m"},
						{"text": "15m", "value": "15m"}
					],
					"current": {"text": "5m", "value": "5m"}
				}
			]
		},
		"annotations": {
			"list": [
				{
					"name": "Deployments",
					"expr": "changes(kube_deployment_status_observed_generation[1m]) > 0",
					"enable": true,
					"iconColor": "blue"
				}
			]
		},
		"links": [
			{
				"title": "Grafana Docs",
				"type": "link",
				"url": "https://grafana.com/docs",
				"targetBlank": true
			}
		]
	}`

	opts := DashboardImportOptions{
		SkipUnsupportedWidgets: true,
	}

	importer := NewGrafanaImporter(opts)
	converted, result, err := importer.ImportDashboard([]byte(dashJSON))

	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	// Verify dashboard
	if converted.Dashboard.Name != "Kubernetes Cluster Metrics" {
		t.Errorf("Unexpected dashboard name: %s", converted.Dashboard.Name)
	}

	// Verify variables
	if len(converted.Variables) != 3 {
		t.Errorf("Expected 3 variables, got %d", len(converted.Variables))
	}

	// Check chained variable
	for _, v := range converted.Variables {
		if v.Name == "pod" && v.Query == "" {
			t.Error("Pod variable should have query referencing namespace")
		}
	}

	// Verify annotations
	if len(converted.Annotations) != 1 {
		t.Errorf("Expected 1 annotation, got %d", len(converted.Annotations))
	}
}

// TestPrometheusRulesImport_EndToEnd tests complete Prometheus rules import
func TestPrometheusRulesImport_EndToEnd(t *testing.T) {
	rulesYAML := `
groups:
  - name: node-alerts
    interval: 1m
    rules:
      - alert: NodeMemoryPressure
        expr: |
          (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100 > 85
        for: 5m
        labels:
          severity: warning
          team: platform
        annotations:
          summary: "Node {{ $labels.instance }} memory pressure"
          description: "Memory usage is above 85% for more than 5 minutes"
          runbook_url: "https://wiki.example.com/runbooks/node-memory"

      - alert: NodeDiskFull
        expr: |
          (1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100 > 90
        for: 10m
        labels:
          severity: critical
          team: platform
        annotations:
          summary: "Node {{ $labels.instance }} disk almost full"
          description: "Root filesystem is above 90% capacity"

      - alert: NodeCPUThrottled
        expr: rate(container_cpu_cfs_throttled_seconds_total[5m]) > 0.1
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "Container {{ $labels.container }} is being CPU throttled"

  - name: service-alerts
    interval: 30s
    rules:
      - alert: HighErrorRate
        expr: |
          sum(rate(http_requests_total{status=~"5.."}[5m])) by (service)
          / sum(rate(http_requests_total[5m])) by (service) * 100 > 5
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate for {{ $labels.service }}"

      - alert: HighLatency
        expr: |
          histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service)) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P99 latency > 1s for {{ $labels.service }}"

      - record: job:http_requests:rate5m
        expr: sum(rate(http_requests_total[5m])) by (job)
        labels:
          aggregation: "5m"
`

	importer := NewAlertImporter()
	alerts, results, err := importer.ImportPrometheusRules([]byte(rulesYAML), AlertImportOptions{
		EnableImportedAlerts: true,
	})

	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Should have 5 alerts (recording rules are skipped)
	if len(alerts) != 5 {
		t.Errorf("Expected 5 alerts, got %d", len(alerts))
	}

	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// Verify specific alerts
	alertNames := make(map[string]bool)
	for _, alert := range alerts {
		alertNames[alert.Rule.Name] = true

		// Check ForDuration was parsed
		if alert.Rule.Name == "NodeMemoryPressure" && alert.Rule.ForDuration != 5*time.Minute {
			t.Errorf("Expected ForDuration 5m for NodeMemoryPressure, got %v", alert.Rule.ForDuration)
		}

		// Check labels/annotations
		if alert.Rule.Name == "NodeDiskFull" && alert.Rule.Description == "" {
			t.Error("Expected description to be set from annotation")
		}
	}

	// Verify all expected alerts are present
	expectedAlerts := []string{
		"NodeMemoryPressure",
		"NodeDiskFull",
		"NodeCPUThrottled",
		"HighErrorRate",
		"HighLatency",
	}

	for _, name := range expectedAlerts {
		if !alertNames[name] {
			t.Errorf("Expected alert %s not found", name)
		}
	}
}

// TestFidelityScoringAccuracy tests fidelity scoring accuracy
func TestFidelityScoringAccuracy(t *testing.T) {
	tests := []struct {
		name            string
		widgetsTotal    int
		widgetsConverted int
		warnings        []string
		expectedWidgetFidelity float64
	}{
		{
			name:            "All widgets converted",
			widgetsTotal:    10,
			widgetsConverted: 10,
			warnings:        []string{},
			expectedWidgetFidelity: 100.0,
		},
		{
			name:            "Half widgets converted",
			widgetsTotal:    10,
			widgetsConverted: 5,
			warnings:        []string{},
			expectedWidgetFidelity: 50.0,
		},
		{
			name:            "Most widgets converted with query warnings",
			widgetsTotal:    10,
			widgetsConverted: 8,
			warnings: []string{
				"Query approximation for complex expression",
				"Query function not supported: someFunc",
			},
			expectedWidgetFidelity: 80.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewFidelityAnalyzer()
			result := &DashboardResult{
				WidgetsTotal:     tt.widgetsTotal,
				WidgetsConverted: tt.widgetsConverted,
				Warnings:         tt.warnings,
			}

			score := analyzer.CalculateFidelityScore(nil, nil, result)

			if score.WidgetFidelity != tt.expectedWidgetFidelity {
				t.Errorf("Expected widget fidelity %.1f, got %.1f",
					tt.expectedWidgetFidelity, score.WidgetFidelity)
			}

			// Verify overall score is calculated
			if score.Overall == 0 {
				t.Error("Expected non-zero overall score")
			}

			// Verify suggestions are generated for low fidelity
			if score.WidgetFidelity < 90 && len(score.Suggestions) == 0 {
				t.Error("Expected suggestions for low fidelity score")
			}
		})
	}
}

// TestTemplateVariableConversion tests template variable conversion accuracy
func TestTemplateVariableConversion(t *testing.T) {
	// Test Datadog variable conversion
	datadogVars := []DatadogVariable{
		{Name: "env", Default: "prod", Prefix: "environment"},
		{Name: "service", Default: "*", AvailableValues: []string{"api", "web", "worker"}},
		{Name: "host", Default: "host123"},
	}

	enhanced := ParseDatadogTemplateVariables(datadogVars)

	if len(enhanced) != 3 {
		t.Fatalf("Expected 3 variables, got %d", len(enhanced))
	}

	// Check env variable
	if enhanced[0].Type != VarTypeQuery {
		t.Errorf("Expected env variable type query, got %v", enhanced[0].Type)
	}

	// Check service variable with available values
	if enhanced[1].Type != VarTypeCustom {
		t.Errorf("Expected service variable type custom, got %v", enhanced[1].Type)
	}
	if len(enhanced[1].Options) != 3 {
		t.Errorf("Expected 3 options for service variable, got %d", len(enhanced[1].Options))
	}

	// Check host variable without prefix
	if enhanced[2].Type != VarTypeTextbox {
		t.Errorf("Expected host variable type textbox, got %v", enhanced[2].Type)
	}
}

// TestGrafanaTemplateVariableConversion tests Grafana variable conversion
func TestGrafanaTemplateVariableConversion(t *testing.T) {
	grafanaVars := []GrafanaVariable{
		{
			Name:       "namespace",
			Label:      "Namespace",
			Type:       "query",
			Query:      "label_values(kube_pod_info, namespace)",
			Multi:      true,
			IncludeAll: true,
			AllValue:   ".*",
			Refresh:    2,
		},
		{
			Name:  "pod",
			Label: "Pod",
			Type:  "query",
			Query: "label_values(kube_pod_info{namespace=~\"$namespace\"}, pod)",
			Multi: true,
		},
		{
			Name:  "interval",
			Type:  "interval",
			Options: []GrafanaVarOpt{
				{Text: "1m", Value: "1m"},
				{Text: "5m", Value: "5m"},
			},
		},
		{
			Name:  "datasource",
			Type:  "datasource",
			Query: "prometheus",
		},
	}

	enhanced := ParseGrafanaTemplateVariables(grafanaVars)

	if len(enhanced) != 4 {
		t.Fatalf("Expected 4 variables, got %d", len(enhanced))
	}

	// Check namespace variable
	if enhanced[0].Type != VarTypeQuery {
		t.Errorf("Expected namespace type query, got %v", enhanced[0].Type)
	}
	if enhanced[0].Refresh != 2 {
		t.Errorf("Expected refresh 2, got %d", enhanced[0].Refresh)
	}

	// Check pod variable has chained dependency
	if len(enhanced[1].ChainedVars) == 0 {
		t.Error("Expected pod variable to detect chained dependency on namespace")
	}

	// Check interval variable
	if enhanced[2].Type != VarTypeInterval {
		t.Errorf("Expected interval type, got %v", enhanced[2].Type)
	}

	// Check datasource variable
	if enhanced[3].Type != VarTypeDatasource {
		t.Errorf("Expected datasource type, got %v", enhanced[3].Type)
	}
}

// TestCompositeMonitorParsing tests Datadog composite monitor parsing
func TestCompositeMonitorParsing(t *testing.T) {
	monitor := &DatadogMonitor{
		ID:       12345,
		Name:     "Critical Service Health",
		Type:     "composite",
		Query:    "a && b || !c",
		Message:  "One or more critical checks failing",
		Priority: 1,
		Tags:     []string{"env:prod", "critical:true"},
	}

	composite, err := ParseDatadogCompositeMonitor(monitor)
	if err != nil {
		t.Fatalf("Failed to parse composite monitor: %v", err)
	}

	if composite.Name != "Critical Service Health" {
		t.Errorf("Unexpected name: %s", composite.Name)
	}

	if composite.Expression != "a && b || !c" {
		t.Errorf("Unexpected expression: %s", composite.Expression)
	}

	// Check logic tree was built
	if composite.LogicTree == nil {
		t.Error("Expected logic tree to be built")
	}

	// Verify logic tree structure (should be OR at root)
	if composite.LogicTree.Type != "or" {
		t.Errorf("Expected root node type 'or', got '%s'", composite.LogicTree.Type)
	}
}

// TestVariableSubstitution_Integration tests variable reference conversion between platforms
func TestVariableSubstitution_Integration(t *testing.T) {
	sub := NewVariableSubstitution()

	tests := []struct {
		name     string
		input    string
		platform SourcePlatform
		expected string
	}{
		{
			name:     "Grafana bracket notation",
			input:    "metric{namespace=[[namespace]]}",
			platform: PlatformGrafana,
			expected: "metric{namespace=${namespace}}",
		},
		{
			name:     "Grafana dollar notation",
			input:    "metric{pod=$pod}",
			platform: PlatformGrafana,
			expected: "metric{pod=${pod}}",
		},
		{
			name:     "Datadog template notation",
			input:    "metric{service:$service.value}",
			platform: PlatformDatadog,
			expected: "metric{service:${service}}",
		},
		{
			name:     "Prometheus label notation",
			input:    "{{ .instance }} is down",
			platform: PlatformPrometheus,
			expected: "${instance} is down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sub.ConvertVariableReferences(tt.input, tt.platform)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestErrorHandlingForMalformedInput tests error handling for various malformed inputs
func TestErrorHandlingForMalformedInput(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		importFunc  func([]byte) error
		expectError bool
	}{
		{
			name:  "Invalid JSON",
			input: `{invalid json`,
			importFunc: func(data []byte) error {
				importer := NewDatadogImporter(DashboardImportOptions{})
				_, _, err := importer.ImportDashboard(data)
				return err
			},
			expectError: true,
		},
		{
			name:  "Empty JSON object",
			input: `{}`,
			importFunc: func(data []byte) error {
				importer := NewDatadogImporter(DashboardImportOptions{})
				_, _, err := importer.ImportDashboard(data)
				return err
			},
			expectError: false, // Implementation is lenient with empty dashboard
		},
		{
			name:  "Valid JSON but wrong structure",
			input: `{"foo": "bar", "baz": 123}`,
			importFunc: func(data []byte) error {
				importer := NewGrafanaImporter(DashboardImportOptions{})
				_, _, err := importer.ImportDashboard(data)
				return err
			},
			expectError: false, // Implementation is lenient with minimal structure
		},
		{
			name:  "Invalid YAML for Prometheus rules",
			input: `invalid: yaml: content: [`,
			importFunc: func(data []byte) error {
				importer := NewAlertImporter()
				_, _, err := importer.ImportPrometheusRules(data, AlertImportOptions{})
				return err
			},
			expectError: true,
		},
		{
			name: "Prometheus YAML without groups",
			input: `
alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']
`,
			importFunc: func(data []byte) error {
				importer := NewAlertImporter()
				_, _, err := importer.ImportPrometheusRules(data, AlertImportOptions{})
				return err
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.importFunc([]byte(tt.input))
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestMigrationStoreOperations tests all migration store operations
func TestMigrationStoreOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migration-store-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "migration.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create multiple reports
	reports := []*MigrationReport{
		{
			ID:                 "report-1",
			Source:             PlatformDatadog,
			StartedAt:          time.Now().Add(-2 * time.Hour),
			CompletedAt:        time.Now().Add(-2 * time.Hour).Add(30 * time.Second),
			Duration:           30 * time.Second,
			DashboardsTotal:    3,
			DashboardsImported: 3,
			AlertsTotal:        5,
			AlertsImported:     4,
			AlertsFailed:       1,
		},
		{
			ID:                 "report-2",
			Source:             PlatformGrafana,
			StartedAt:          time.Now().Add(-1 * time.Hour),
			CompletedAt:        time.Now().Add(-1 * time.Hour).Add(15 * time.Second),
			Duration:           15 * time.Second,
			DashboardsTotal:    5,
			DashboardsImported: 5,
		},
		{
			ID:                 "report-3",
			Source:             PlatformPrometheus,
			StartedAt:          time.Now().Add(-30 * time.Minute),
			CompletedAt:        time.Now().Add(-30 * time.Minute).Add(5 * time.Second),
			Duration:           5 * time.Second,
			AlertsTotal:        10,
			AlertsImported:     10,
		},
	}

	// Save all reports
	for _, report := range reports {
		if err := store.SaveReport(report); err != nil {
			t.Fatalf("Failed to save report %s: %v", report.ID, err)
		}
	}

	// Test listing reports
	allReports, err := store.ListReports(10)
	if err != nil {
		t.Fatalf("Failed to list reports: %v", err)
	}
	if len(allReports) != 3 {
		t.Errorf("Expected 3 reports, got %d", len(allReports))
	}

	// Test filtering by source
	datadogReports, err := store.GetReportsBySource(PlatformDatadog, 10)
	if err != nil {
		t.Fatalf("Failed to get Datadog reports: %v", err)
	}
	if len(datadogReports) != 1 {
		t.Errorf("Expected 1 Datadog report, got %d", len(datadogReports))
	}

	// Test getting individual report
	report, err := store.GetReport("report-2")
	if err != nil {
		t.Fatalf("Failed to get report: %v", err)
	}
	if report.Source != PlatformGrafana {
		t.Errorf("Expected Grafana source, got %v", report.Source)
	}

	// Test stats
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats.TotalMigrations != 3 {
		t.Errorf("Expected 3 total migrations, got %d", stats.TotalMigrations)
	}
	if stats.TotalDashboards != 8 {
		t.Errorf("Expected 8 total dashboards, got %d", stats.TotalDashboards)
	}
	if stats.TotalAlerts != 14 {
		t.Errorf("Expected 14 total alerts, got %d", stats.TotalAlerts)
	}

	// Test deletion
	if err := store.DeleteReport("report-1"); err != nil {
		t.Fatalf("Failed to delete report: %v", err)
	}

	allReports, _ = store.ListReports(10)
	if len(allReports) != 2 {
		t.Errorf("Expected 2 reports after deletion, got %d", len(allReports))
	}

	// Test getting non-existent report
	report, err = store.GetReport("non-existent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if report != nil {
		t.Error("Expected nil for non-existent report")
	}
}

// TestGenerateMigrationPreview_Integration tests preview generation
func TestGenerateMigrationPreview_Integration(t *testing.T) {
	datadogDash := `{
		"title": "Test Dashboard",
		"layout_type": "ordered",
		"widgets": [
			{"id": 1, "definition": {"type": "timeseries", "title": "CPU"}},
			{"id": 2, "definition": {"type": "unsupported_widget_type", "title": "Unknown"}}
		],
		"template_variables": [{"name": "env"}]
	}`

	preview, err := GenerateMigrationPreview([]byte(datadogDash))
	if err != nil {
		t.Fatalf("Failed to generate preview: %v", err)
	}

	if preview.Source != PlatformDatadog {
		t.Errorf("Expected Datadog source, got %v", preview.Source)
	}

	if len(preview.Dashboards) != 1 {
		t.Errorf("Expected 1 dashboard in preview, got %d", len(preview.Dashboards))
	}

	// Should have estimated fidelity
	if preview.EstimatedFidelity == nil {
		t.Error("Expected fidelity estimate")
	}

	if preview.EstimatedFidelity.Overall == 0 {
		t.Error("Expected non-zero overall fidelity")
	}
}

// TestSupportedFormats tests the supported formats listing
func TestSupportedFormats(t *testing.T) {
	info := GetSupportedFormats()

	// Should have formats key
	formatsRaw, ok := info["formats"]
	if !ok {
		t.Fatal("Expected 'formats' key in supported formats")
	}

	formats, ok := formatsRaw.([]map[string]string)
	if !ok {
		t.Fatal("Expected formats to be []map[string]string")
	}

	// Should have at least Datadog, Grafana, Prometheus
	if len(formats) < 3 {
		t.Errorf("Expected at least 3 formats, got %d", len(formats))
	}

	// Check for expected format info
	foundDatadog := false
	foundGrafana := false
	foundPrometheus := false

	for _, f := range formats {
		switch f["name"] {
		case "datadog":
			foundDatadog = true
		case "grafana":
			foundGrafana = true
		case "prometheus":
			foundPrometheus = true
		}
	}

	if !foundDatadog {
		t.Error("Expected datadog in supported formats")
	}
	if !foundGrafana {
		t.Error("Expected grafana in supported formats")
	}
	if !foundPrometheus {
		t.Error("Expected prometheus in supported formats")
	}
}

// BenchmarkDatadogDashboardImport benchmarks Datadog dashboard import
func BenchmarkDatadogDashboardImport(b *testing.B) {
	dashJSON := generateLargeDashboard(100)

	importer := NewDatadogImporter(DashboardImportOptions{
		SkipUnsupportedWidgets: true,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = importer.ImportDashboard(dashJSON)
	}
}

// BenchmarkGrafanaDashboardImport benchmarks Grafana dashboard import
func BenchmarkGrafanaDashboardImport(b *testing.B) {
	dashJSON := generateLargeGrafanaDashboard(100)

	importer := NewGrafanaImporter(DashboardImportOptions{
		SkipUnsupportedWidgets: true,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = importer.ImportDashboard(dashJSON)
	}
}

// BenchmarkPrometheusRulesImport benchmarks Prometheus rules import
func BenchmarkPrometheusRulesImport(b *testing.B) {
	rules := generateLargePrometheusRules(100)

	importer := NewAlertImporter()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = importer.ImportPrometheusRules(rules, AlertImportOptions{})
	}
}

// Helper functions for generating test data

func generateLargeDashboard(widgetCount int) []byte {
	widgets := make([]map[string]interface{}, widgetCount)
	for i := 0; i < widgetCount; i++ {
		widgets[i] = map[string]interface{}{
			"id": i,
			"definition": map[string]interface{}{
				"type":  "timeseries",
				"title": "Widget " + string(rune(i)),
				"requests": []map[string]string{
					{"q": "avg:system.cpu{*}"},
				},
			},
		}
	}

	dash := map[string]interface{}{
		"title":       "Large Dashboard",
		"layout_type": "ordered",
		"widgets":     widgets,
	}

	data, _ := json.Marshal(dash)
	return data
}

func generateLargeGrafanaDashboard(panelCount int) []byte {
	panels := make([]map[string]interface{}, panelCount)
	for i := 0; i < panelCount; i++ {
		panels[i] = map[string]interface{}{
			"id":    i,
			"type":  "timeseries",
			"title": "Panel " + string(rune(i)),
			"gridPos": map[string]int{
				"x": (i % 4) * 6,
				"y": (i / 4) * 6,
				"w": 6,
				"h": 6,
			},
			"targets": []map[string]string{
				{"refId": "A", "expr": "rate(metric[5m])"},
			},
		}
	}

	dash := map[string]interface{}{
		"uid":           "bench-dash",
		"title":         "Large Grafana Dashboard",
		"schemaVersion": 27,
		"panels":        panels,
	}

	data, _ := json.Marshal(dash)
	return data
}

func generateLargePrometheusRules(alertCount int) []byte {
	rules := make([]string, alertCount)
	for i := 0; i < alertCount; i++ {
		rules[i] = `      - alert: Alert` + string(rune('A'+i%26)) + string(rune('0'+i/26)) + `
        expr: metric > ` + string(rune('0'+i%10)) + `
        for: 5m
        labels:
          severity: warning`
	}

	yaml := `groups:
  - name: benchmark-alerts
    rules:
` + stringJoin(rules, "\n")

	return []byte(yaml)
}

func stringJoin(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
