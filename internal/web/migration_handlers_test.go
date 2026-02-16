package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dogwatch/internal/migration"
)

// setupMigrationTestServer sets up a test server with migration routes
func setupMigrationTestServer(t *testing.T) (*httptest.Server, string, func()) {
	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "migration-handler-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "migration.db")
	store, err := migration.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	SetMigrationStore(store)

	mux := http.NewServeMux()
	RegisterMigrationRoutes(mux)

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return httptest.NewServer(mux), tmpDir, cleanup
}

// TestDatadogDashboardImportEndpoint tests POST /api/migration/datadog/dashboard
func TestDatadogDashboardImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test valid dashboard import
	t.Run("Valid dashboard import", func(t *testing.T) {
		dashJSON := `{
			"title": "Test Dashboard",
			"layout_type": "ordered",
			"widgets": [
				{
					"id": 1,
					"definition": {
						"type": "timeseries",
						"title": "CPU Usage",
						"requests": [{"q": "avg:system.cpu{*}"}]
					}
				}
			],
			"template_variables": [
				{"name": "env", "default": "prod"}
			]
		}`

		resp, err := http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
		if result["report_id"] == nil {
			t.Error("Expected report_id in response")
		}
		if result["dashboard"] == nil {
			t.Error("Expected dashboard in response")
		}
	})

	// Test with skip_unsupported query param
	t.Run("Dashboard import with skip unsupported", func(t *testing.T) {
		dashJSON := `{
			"title": "Dashboard with Unsupported",
			"layout_type": "ordered",
			"widgets": [
				{"id": 1, "definition": {"type": "unsupported_type", "title": "Unknown"}}
			]
		}`

		resp, err := http.Post(server.URL+"/api/migration/datadog/dashboard?skip_unsupported=true", "application/json", strings.NewReader(dashJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should succeed with warnings
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}
	})

	// Test invalid JSON
	t.Run("Invalid JSON", func(t *testing.T) {
		resp, err := http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader("{invalid}"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	// Test method not allowed
	t.Run("GET not allowed", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/datadog/dashboard")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", resp.StatusCode)
		}
	})
}

// TestGrafanaDashboardImportEndpoint tests POST /api/migration/grafana/dashboard
func TestGrafanaDashboardImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test valid Grafana dashboard import
	t.Run("Valid Grafana dashboard import", func(t *testing.T) {
		dashJSON := `{
			"uid": "test-dash",
			"title": "Grafana Test Dashboard",
			"schemaVersion": 27,
			"panels": [
				{
					"id": 1,
					"title": "Request Rate",
					"type": "timeseries",
					"gridPos": {"x": 0, "y": 0, "w": 12, "h": 8},
					"targets": [
						{"refId": "A", "expr": "rate(http_requests_total[5m])"}
					]
				}
			],
			"templating": {
				"list": [
					{"name": "namespace", "type": "query", "query": "label_values(namespace)"}
				]
			}
		}`

		resp, err := http.Post(server.URL+"/api/migration/grafana/dashboard", "application/json", strings.NewReader(dashJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
		if result["variables"] == nil {
			t.Error("Expected variables in response")
		}
	})
}

// TestAlertsImportEndpoint tests POST /api/migration/alerts
func TestAlertsImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test Prometheus rules import through auto-detect endpoint
	t.Run("Prometheus rules import", func(t *testing.T) {
		rulesYAML := `
groups:
  - name: test-group
    rules:
      - alert: HighCPU
        expr: cpu_usage > 80
        for: 5m
        labels:
          severity: warning
`

		resp, err := http.Post(server.URL+"/api/migration/alerts", "application/json", strings.NewReader(rulesYAML))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["alerts"] == nil {
			t.Error("Expected alerts in response")
		}
	})
}

// TestDatadogAlertsImportEndpoint tests POST /api/migration/datadog/alerts
func TestDatadogAlertsImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test Datadog monitor import
	t.Run("Datadog monitor import", func(t *testing.T) {
		monitorJSON := `{
			"id": 12345,
			"name": "High CPU Alert",
			"type": "metric alert",
			"query": "avg(last_5m):avg:system.cpu.user{*} > 80",
			"message": "CPU usage is above 80%",
			"options": {
				"thresholds": {"critical": 80, "warning": 70}
			}
		}`

		resp, err := http.Post(server.URL+"/api/migration/datadog/alerts?enable=true&prefix=[DD]", "application/json", strings.NewReader(monitorJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}
	})
}

// TestGrafanaAlertsImportEndpoint tests POST /api/migration/grafana/alerts
func TestGrafanaAlertsImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test Grafana alert rules import
	t.Run("Grafana alert rules import", func(t *testing.T) {
		alertsJSON := `{
			"groups": [
				{
					"name": "test-group",
					"rules": [
						{
							"title": "High Memory",
							"condition": "A",
							"for": "5m",
							"data": [
								{
									"refId": "A",
									"model": {
										"expr": "memory_usage > 90"
									}
								}
							]
						}
					]
				}
			]
		}`

		resp, err := http.Post(server.URL+"/api/migration/grafana/alerts", "application/json", strings.NewReader(alertsJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}
	})
}

// TestPrometheusAlertsImportEndpoint tests POST /api/migration/prometheus/alerts
func TestPrometheusAlertsImportEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test Prometheus rules import
	t.Run("Prometheus rules import", func(t *testing.T) {
		rulesYAML := `
groups:
  - name: node-alerts
    interval: 1m
    rules:
      - alert: NodeDown
        expr: up == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: Node is down
      - alert: HighLoad
        expr: load1 > 10
        for: 10m
        labels:
          severity: warning
`

		resp, err := http.Post(server.URL+"/api/migration/prometheus/alerts", "application/json", strings.NewReader(rulesYAML))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		alerts := result["alerts"].(map[string]interface{})
		if alerts["total"].(float64) != 2 {
			t.Errorf("Expected 2 alerts total, got %v", alerts["total"])
		}
	})
}

// TestMigrationReportsEndpoint tests GET /api/migration/reports
func TestMigrationReportsEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// First, create some reports by doing imports
	dashJSON := `{"title": "Test", "layout_type": "ordered", "widgets": []}`
	http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))

	// Test GET reports
	t.Run("GET reports", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/reports")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var reports []migration.MigrationReport
		if err := json.NewDecoder(resp.Body).Decode(&reports); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should have at least one report from the import above
		if len(reports) < 1 {
			t.Error("Expected at least 1 report")
		}
	})

	// Test GET reports with source filter
	t.Run("GET reports with source filter", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/reports?source=datadog")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

// TestMigrationReportByIDEndpoint tests GET/DELETE /api/migration/report/:id
func TestMigrationReportByIDEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Create a report
	dashJSON := `{"title": "Test", "layout_type": "ordered", "widgets": []}`
	importResp, _ := http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))
	var importResult map[string]interface{}
	json.NewDecoder(importResp.Body).Decode(&importResult)
	importResp.Body.Close()
	reportID := importResult["report_id"].(string)

	// Test GET report by ID
	t.Run("GET report by ID", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/report/" + reportID)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var report migration.MigrationReport
		if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if report.ID != reportID {
			t.Errorf("Expected report ID %s, got %s", reportID, report.ID)
		}
	})

	// Test GET non-existent report
	t.Run("GET non-existent report", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/report/non-existent-id")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", resp.StatusCode)
		}
	})

	// Test DELETE report
	t.Run("DELETE report", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/migration/report/"+reportID, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("Expected 204, got %d", resp.StatusCode)
		}

		// Verify deletion
		getResp, err := http.Get(server.URL + "/api/migration/report/" + reportID)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer getResp.Body.Close()
		if getResp.StatusCode != http.StatusNotFound {
			t.Error("Report should have been deleted")
		}
	})
}

// TestMigrationStatsEndpoint tests GET /api/migration/stats
func TestMigrationStatsEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Create some reports first
	dashJSON := `{"title": "Test", "layout_type": "ordered", "widgets": []}`
	http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))
	http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))

	// Test GET stats
	t.Run("GET stats", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/stats")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var stats migration.MigrationStats
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if stats.TotalMigrations < 2 {
			t.Errorf("Expected at least 2 migrations, got %d", stats.TotalMigrations)
		}
	})
}

// TestMigrationFormatsEndpoint tests GET /api/migration/formats
func TestMigrationFormatsEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test GET formats
	t.Run("GET formats", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/api/migration/formats")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var response map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		formats, ok := response["formats"].([]any)
		if !ok {
			t.Fatal("Expected formats key in response")
		}
		if len(formats) < 3 {
			t.Errorf("Expected at least 3 formats, got %d", len(formats))
		}
	})
}

// TestMigrationPreviewEndpoint tests POST /api/migration/preview
func TestMigrationPreviewEndpoint(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Test Datadog dashboard preview
	t.Run("Datadog dashboard preview", func(t *testing.T) {
		dashJSON := `{
			"title": "Preview Test",
			"layout_type": "ordered",
			"widgets": [
				{"id": 1, "definition": {"type": "timeseries", "title": "Widget 1"}}
			]
		}`

		resp, err := http.Post(server.URL+"/api/migration/preview", "application/json", strings.NewReader(dashJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["format"] != "datadog" {
			t.Errorf("Expected format 'datadog', got %v", result["format"])
		}
		if result["items"] == nil {
			t.Error("Expected items in preview")
		}
	})

	// Test Grafana dashboard preview
	t.Run("Grafana dashboard preview", func(t *testing.T) {
		dashJSON := `{
			"schemaVersion": 27,
			"panels": [
				{"id": 1, "type": "timeseries", "title": "Panel 1", "gridPos": {"x": 0, "y": 0, "w": 12, "h": 8}}
			]
		}`

		resp, err := http.Post(server.URL+"/api/migration/preview", "application/json", strings.NewReader(dashJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["format"] != "grafana" {
			t.Errorf("Expected format 'grafana', got %v", result["format"])
		}
	})

	// Test Prometheus rules preview
	t.Run("Prometheus rules preview", func(t *testing.T) {
		rulesYAML := `
groups:
  - name: test
    rules:
      - alert: TestAlert
        expr: up == 0
`

		resp, err := http.Post(server.URL+"/api/migration/preview", "application/json", strings.NewReader(rulesYAML))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["format"] != "prometheus" {
			t.Errorf("Expected format 'prometheus', got %v", result["format"])
		}
	})

	// Test unknown format
	t.Run("Unknown format preview", func(t *testing.T) {
		unknownJSON := `{"random": "data", "no": "recognizable", "fields": true}`

		resp, err := http.Post(server.URL+"/api/migration/preview", "application/json", strings.NewReader(unknownJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["format"] != "unknown" {
			t.Errorf("Expected format 'unknown', got %v", result["format"])
		}
		warnings := result["warnings"].([]interface{})
		if len(warnings) == 0 {
			t.Error("Expected warnings for unknown format")
		}
	})
}

// TestMigrationStoreNotConfigured tests endpoints when store is nil
func TestMigrationStoreNotConfigured(t *testing.T) {
	// Reset global store to nil
	SetMigrationStore(nil)

	mux := http.NewServeMux()
	RegisterMigrationRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	endpoints := []string{
		"/api/migration/reports",
		"/api/migration/stats",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp, err := http.Get(server.URL + endpoint)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("Expected 503, got %d", resp.StatusCode)
			}
		})
	}
}

// TestLargeFileHandling tests handling of large import files
func TestLargeFileHandling(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Generate a large dashboard with many widgets
	widgets := make([]map[string]interface{}, 500)
	for i := 0; i < 500; i++ {
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

	largeDash := map[string]interface{}{
		"title":       "Large Dashboard",
		"layout_type": "ordered",
		"widgets":     widgets,
	}

	body, _ := json.Marshal(largeDash)

	t.Run("Large dashboard import", func(t *testing.T) {
		resp, err := http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Should handle large files without error
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		dashboard := result["dashboard"].(map[string]interface{})
		if dashboard["widgets_total"].(float64) != 500 {
			t.Errorf("Expected 500 widgets, got %v", dashboard["widgets_total"])
		}
	})
}

// TestFileUploadHandling tests multipart file upload
func TestFileUploadHandling(t *testing.T) {
	server, _, cleanup := setupMigrationTestServer(t)
	defer cleanup()
	defer server.Close()

	// Create a multipart form with a file
	dashJSON := `{"title": "Upload Test", "layout_type": "ordered", "widgets": []}`

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "dashboard.json")
	part.Write([]byte(dashJSON))
	writer.Close()

	// Note: The current handlers don't support multipart uploads directly,
	// but this test verifies the server doesn't crash on such requests
	t.Run("Multipart upload (not supported)", func(t *testing.T) {
		resp, err := http.Post(server.URL+"/api/migration/datadog/dashboard", writer.FormDataContentType(), body)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Will likely fail since we expect JSON, but shouldn't crash
		// The error handling should return a proper error response
	})
}

// BenchmarkDashboardImportHandler benchmarks dashboard import
func BenchmarkDashboardImportHandler(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "migration-bench-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "migration.db")
	store, _ := migration.NewStore(dbPath)
	defer store.Close()
	SetMigrationStore(store)

	mux := http.NewServeMux()
	RegisterMigrationRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	dashJSON := `{
		"title": "Benchmark Dashboard",
		"layout_type": "ordered",
		"widgets": [
			{"id": 1, "definition": {"type": "timeseries", "title": "W1", "requests": [{"q": "avg:metric{*}"}]}},
			{"id": 2, "definition": {"type": "query_value", "title": "W2", "requests": [{"q": "sum:metric{*}"}]}},
			{"id": 3, "definition": {"type": "toplist", "title": "W3", "requests": [{"q": "top(metric{*})"}]}}
		]
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := http.Post(server.URL+"/api/migration/datadog/dashboard", "application/json", strings.NewReader(dashJSON))
		resp.Body.Close()
	}
}

// BenchmarkPreviewHandler benchmarks preview generation
func BenchmarkPreviewHandler(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "migration-bench-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "migration.db")
	store, _ := migration.NewStore(dbPath)
	defer store.Close()
	SetMigrationStore(store)

	mux := http.NewServeMux()
	RegisterMigrationRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	dashJSON := `{
		"title": "Preview Test",
		"layout_type": "ordered",
		"widgets": [{"id": 1, "definition": {"type": "timeseries"}}]
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := http.Post(server.URL+"/api/migration/preview", "application/json", strings.NewReader(dashJSON))
		resp.Body.Close()
	}
}
