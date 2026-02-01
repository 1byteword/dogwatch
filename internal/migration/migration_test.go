package migration

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected SourcePlatform
	}{
		{
			name:     "Datadog dashboard",
			data:     `{"layout_type": "ordered", "widgets": []}`,
			expected: PlatformDatadog,
		},
		{
			name:     "Datadog monitor",
			data:     `{"type": "monitor", "thresholds": {}, "notify_no_data": true}`,
			expected: PlatformDatadog,
		},
		{
			name:     "Grafana dashboard with inputs",
			data:     `{"__inputs": [], "panels": []}`,
			expected: PlatformGrafana,
		},
		{
			name:     "Grafana dashboard with schemaVersion",
			data:     `{"schemaVersion": 27, "panels": []}`,
			expected: PlatformGrafana,
		},
		{
			name:     "Grafana dashboard with datasource",
			data:     `{"panels": [{"datasource": "prometheus"}]}`,
			expected: PlatformGrafana,
		},
		{
			name:     "Prometheus rules YAML style",
			data:     `groups:\n  - rules:\n      - expr: up == 0`,
			expected: PlatformPrometheus,
		},
		{
			name:     "Prometheus rules JSON",
			data:     `{"groups": [{"rules": [{"expr": "up == 0"}]}]}`,
			expected: PlatformPrometheus,
		},
		{
			name:     "Unknown format",
			data:     `{"foo": "bar"}`,
			expected: PlatformUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFormat([]byte(tt.data))
			if got != tt.expected {
				t.Errorf("DetectFormat() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDatadogDashboardImport(t *testing.T) {
	dashJSON := `{
		"title": "Test Dashboard",
		"layout_type": "ordered",
		"widgets": [
			{
				"id": 1,
				"definition": {
					"type": "timeseries",
					"title": "CPU Usage",
					"requests": [
						{"q": "avg:system.cpu.user{*}by{host}"}
					]
				}
			},
			{
				"id": 2,
				"definition": {
					"type": "query_value",
					"title": "Request Count",
					"requests": [
						{"q": "sum:http.requests{*}"}
					],
					"precision": 2
				}
			},
			{
				"id": 3,
				"definition": {
					"type": "note",
					"text": "This is a note"
				}
			}
		],
		"template_variables": [
			{
				"name": "env",
				"default": "prod",
				"prefix": "environment"
			}
		]
	}`

	importer := NewDatadogImporter(DashboardImportOptions{
		SkipUnsupportedWidgets: true,
		DashboardNamePrefix:    "[DD] ",
	})

	converted, result, err := importer.ImportDashboard([]byte(dashJSON))
	if err != nil {
		t.Fatalf("ImportDashboard failed: %v", err)
	}

	// Check result
	if !result.Success {
		t.Errorf("Expected success, got failure")
	}
	if result.WidgetsTotal != 3 {
		t.Errorf("Expected 3 widgets total, got %d", result.WidgetsTotal)
	}
	if result.WidgetsConverted != 3 {
		t.Errorf("Expected 3 widgets converted, got %d", result.WidgetsConverted)
	}

	// Check dashboard
	if converted.Dashboard.Name != "[DD] Test Dashboard" {
		t.Errorf("Expected dashboard name '[DD] Test Dashboard', got '%s'", converted.Dashboard.Name)
	}
	if len(converted.Dashboard.Layout) != 3 {
		t.Errorf("Expected 3 widgets in layout, got %d", len(converted.Dashboard.Layout))
	}

	// Check variables
	if len(converted.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(converted.Variables))
	}
	if converted.Variables[0].Name != "env" {
		t.Errorf("Expected variable name 'env', got '%s'", converted.Variables[0].Name)
	}
}

func TestDatadogMonitorImport(t *testing.T) {
	monitorJSON := `{
		"id": 12345,
		"name": "High CPU Alert",
		"type": "metric alert",
		"query": "avg(last_5m):avg:system.cpu.user{*} > 80",
		"message": "CPU usage is above 80%",
		"tags": ["env:prod", "team:platform"],
		"priority": 2,
		"options": {
			"thresholds": {
				"critical": 80,
				"warning": 70
			},
			"notify_no_data": true,
			"evaluation_delay": 60
		}
	}`

	importer := NewDatadogImporter(DashboardImportOptions{})
	alerts, results, err := importer.ImportMonitors([]byte(monitorJSON), AlertImportOptions{
		EnableImportedAlerts: true,
		AlertNamePrefix:      "[DD] ",
	})

	if err != nil {
		t.Fatalf("ImportMonitors failed: %v", err)
	}

	if len(alerts) != 1 {
		t.Fatalf("Expected 1 alert, got %d", len(alerts))
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	alert := alerts[0]
	result := results[0]

	if !result.Success {
		t.Errorf("Expected success, got failure")
	}
	if alert.Rule.Name != "[DD] High CPU Alert" {
		t.Errorf("Expected alert name '[DD] High CPU Alert', got '%s'", alert.Rule.Name)
	}
	if alert.Rule.Threshold != 80 {
		t.Errorf("Expected threshold 80, got %f", alert.Rule.Threshold)
	}
	if !alert.Rule.Enabled {
		t.Errorf("Expected alert to be enabled")
	}
}

func TestGrafanaDashboardImport(t *testing.T) {
	dashJSON := `{
		"uid": "abc123",
		"title": "Test Grafana Dashboard",
		"schemaVersion": 27,
		"panels": [
			{
				"id": 1,
				"title": "Request Rate",
				"type": "timeseries",
				"gridPos": {"x": 0, "y": 0, "w": 12, "h": 8},
				"targets": [
					{
						"refId": "A",
						"expr": "rate(http_requests_total[5m])"
					}
				]
			},
			{
				"id": 2,
				"title": "Current Users",
				"type": "stat",
				"gridPos": {"x": 12, "y": 0, "w": 6, "h": 4},
				"targets": [
					{
						"refId": "A",
						"expr": "sum(active_users)"
					}
				]
			},
			{
				"id": 3,
				"title": "Notes",
				"type": "text",
				"gridPos": {"x": 0, "y": 8, "w": 24, "h": 2},
				"content": "Dashboard documentation"
			}
		],
		"templating": {
			"list": [
				{
					"name": "namespace",
					"type": "query",
					"query": "label_values(namespace)",
					"multi": true
				}
			]
		},
		"annotations": {
			"list": [
				{
					"name": "Deployments",
					"expr": "changes(deployment_info[1m])",
					"enable": true,
					"iconColor": "green"
				}
			]
		}
	}`

	importer := NewGrafanaImporter(DashboardImportOptions{
		SkipUnsupportedWidgets: true,
	})

	converted, result, err := importer.ImportDashboard([]byte(dashJSON))
	if err != nil {
		t.Fatalf("ImportDashboard failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got failure")
	}
	if result.WidgetsTotal != 3 {
		t.Errorf("Expected 3 widgets total, got %d", result.WidgetsTotal)
	}
	if result.WidgetsConverted != 3 {
		t.Errorf("Expected 3 widgets converted, got %d", result.WidgetsConverted)
	}

	if converted.Dashboard.Name != "Test Grafana Dashboard" {
		t.Errorf("Expected dashboard name 'Test Grafana Dashboard', got '%s'", converted.Dashboard.Name)
	}

	// Check variables
	if len(converted.Variables) != 1 {
		t.Errorf("Expected 1 variable, got %d", len(converted.Variables))
	}
	if converted.Variables[0].Name != "namespace" {
		t.Errorf("Expected variable name 'namespace', got '%s'", converted.Variables[0].Name)
	}

	// Check annotations
	if len(converted.Annotations) != 1 {
		t.Errorf("Expected 1 annotation, got %d", len(converted.Annotations))
	}
}

func TestPrometheusRulesImport(t *testing.T) {
	rulesYAML := `
groups:
  - name: example
    interval: 30s
    rules:
      - alert: HighCPU
        expr: avg(cpu_usage) > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: High CPU usage detected
      - alert: DiskFull
        expr: disk_usage > 0.9
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: Disk is almost full
`

	importer := NewAlertImporter()
	alerts, results, err := importer.ImportPrometheusRules([]byte(rulesYAML), AlertImportOptions{
		EnableImportedAlerts: true,
	})

	if err != nil {
		t.Fatalf("ImportPrometheusRules failed: %v", err)
	}

	if len(alerts) != 2 {
		t.Fatalf("Expected 2 alerts, got %d", len(alerts))
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Check first alert
	if alerts[0].Rule.Name != "HighCPU" {
		t.Errorf("Expected alert name 'HighCPU', got '%s'", alerts[0].Rule.Name)
	}
	if alerts[0].Rule.ForDuration != 5*time.Minute {
		t.Errorf("Expected for duration 5m, got %v", alerts[0].Rule.ForDuration)
	}

	// Check second alert
	if alerts[1].Rule.Name != "DiskFull" {
		t.Errorf("Expected alert name 'DiskFull', got '%s'", alerts[1].Rule.Name)
	}
}

func TestDatadogQueryConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple metric",
			input:    "system.cpu.user",
			expected: "metric(system.cpu.user) | avg()", // Default aggregator is avg
		},
		{
			name:     "Metric with aggregator",
			input:    "avg:system.cpu.user{*}",
			expected: "metric(system.cpu.user) | avg()",
		},
		{
			name:     "Metric with filter",
			input:    "avg:system.cpu.user{host:web01}",
			expected: "metric(system.cpu.user) | filter host=\"web01\" | avg()",
		},
		{
			name:     "Metric with group by",
			input:    "avg:system.cpu.user{*}by{host}",
			expected: "metric(system.cpu.user) | avg() | group_by(host)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertDatadogQuery(tt.input)
			if got != tt.expected {
				t.Errorf("convertDatadogQuery(%s) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPromQLConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple metric",
			input:    "http_requests_total",
			expected: "metric(http_requests_total)",
		},
		{
			name:     "Rate function",
			input:    "rate(http_requests_total[5m])",
			expected: "metric(http_requests_total) | rate()",
		},
		{
			name:     "Sum with labels",
			input:    `sum(rate(http_requests_total{job="api"}[5m]))`,
			expected: `metric(http_requests_total) | filter job="api" | sum()`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPromQL(tt.input)
			if got != tt.expected {
				t.Errorf("convertPromQL(%s) = %s, want %s", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMigrationStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "migration-test-*")
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

	// Create a report
	report := &MigrationReport{
		ID:                 "test-123",
		Source:             PlatformDatadog,
		StartedAt:          time.Now().Add(-1 * time.Minute),
		CompletedAt:        time.Now(),
		Duration:           1 * time.Minute,
		DashboardsTotal:    2,
		DashboardsImported: 2,
		AlertsTotal:        3,
		AlertsImported:     2,
		AlertsFailed:       1,
		DashboardResults: []DashboardResult{
			{
				SourceName:       "Dashboard 1",
				TargetID:         "dash-1",
				Success:          true,
				WidgetsTotal:     5,
				WidgetsConverted: 5,
			},
		},
		AlertResults: []AlertResult{
			{
				SourceName: "Alert 1",
				TargetID:   "alert-1",
				Success:    true,
			},
		},
		Warnings: []string{"Some warning"},
	}

	// Save report
	if err := store.SaveReport(report); err != nil {
		t.Fatalf("Failed to save report: %v", err)
	}

	// Get report
	retrieved, err := store.GetReport("test-123")
	if err != nil {
		t.Fatalf("Failed to get report: %v", err)
	}
	if retrieved == nil {
		t.Fatalf("Report not found")
	}
	if retrieved.DashboardsImported != 2 {
		t.Errorf("Expected 2 dashboards imported, got %d", retrieved.DashboardsImported)
	}
	if retrieved.AlertsFailed != 1 {
		t.Errorf("Expected 1 alert failed, got %d", retrieved.AlertsFailed)
	}

	// List reports
	reports, err := store.ListReports(10)
	if err != nil {
		t.Fatalf("Failed to list reports: %v", err)
	}
	if len(reports) != 1 {
		t.Errorf("Expected 1 report, got %d", len(reports))
	}

	// Get stats
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats.TotalMigrations != 1 {
		t.Errorf("Expected 1 total migration, got %d", stats.TotalMigrations)
	}
	if stats.TotalDashboards != 2 {
		t.Errorf("Expected 2 total dashboards, got %d", stats.TotalDashboards)
	}

	// Delete report
	if err := store.DeleteReport("test-123"); err != nil {
		t.Fatalf("Failed to delete report: %v", err)
	}

	// Verify deleted
	deleted, err := store.GetReport("test-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if deleted != nil {
		t.Errorf("Expected report to be deleted")
	}
}

func TestExtractThresholdFromPromQL(t *testing.T) {
	tests := []struct {
		expr              string
		expectedThreshold float64
		expectedCondition string
	}{
		{"metric > 80", 80, "gt"},
		{"metric >= 0.95", 0.95, "gte"},
		{"metric < 10", 10, "lt"},
		{"metric <= 100", 100, "lte"},
		{"metric == 0", 0, "eq"},
		{"metric != 1", 1, "neq"},
		{"metric", 0, "gt"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			threshold, condition := extractThresholdFromPromQL(tt.expr)
			if threshold != tt.expectedThreshold {
				t.Errorf("threshold = %f, want %f", threshold, tt.expectedThreshold)
			}
			if condition != tt.expectedCondition {
				t.Errorf("condition = %s, want %s", condition, tt.expectedCondition)
			}
		})
	}
}

func TestGenerateMigrationReport(t *testing.T) {
	dashResults := []DashboardResult{
		{SourceName: "Dash1", Success: true, WidgetsTotal: 5, WidgetsConverted: 5},
		{SourceName: "Dash2", Success: false, Error: "Failed to parse"},
	}
	alertResults := []AlertResult{
		{SourceName: "Alert1", Success: true},
		{SourceName: "Alert2", Success: true},
		{SourceName: "Alert3", Success: false, Error: "Invalid expression"},
	}

	startTime := time.Now().Add(-1 * time.Second)
	report := GenerateMigrationReport(dashResults, alertResults, PlatformGrafana, startTime)

	if report.Source != PlatformGrafana {
		t.Errorf("Expected source PlatformGrafana, got %v", report.Source)
	}
	if report.DashboardsTotal != 2 {
		t.Errorf("Expected 2 dashboards total, got %d", report.DashboardsTotal)
	}
	if report.DashboardsImported != 1 {
		t.Errorf("Expected 1 dashboard imported, got %d", report.DashboardsImported)
	}
	if report.DashboardsFailed != 1 {
		t.Errorf("Expected 1 dashboard failed, got %d", report.DashboardsFailed)
	}
	if report.AlertsTotal != 3 {
		t.Errorf("Expected 3 alerts total, got %d", report.AlertsTotal)
	}
	if report.AlertsImported != 2 {
		t.Errorf("Expected 2 alerts imported, got %d", report.AlertsImported)
	}
	if report.AlertsFailed != 1 {
		t.Errorf("Expected 1 alert failed, got %d", report.AlertsFailed)
	}
	if len(report.ManualSteps) == 0 {
		t.Errorf("Expected manual steps to be populated")
	}
}
