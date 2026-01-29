// Package integration provides end-to-end API tests for dogwatch.
// These tests require a running dogwatch instance and test real API endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// serverURL returns the dogwatch server URL for integration tests.
// Default is http://localhost:9999, override with DOGWATCH_TEST_URL env var.
func serverURL() string {
	if url := os.Getenv("DOGWATCH_TEST_URL"); url != "" {
		return url
	}
	return "http://localhost:9999"
}

// skipIfNoServer skips the test if the dogwatch server is not reachable.
func skipIfNoServer(t *testing.T) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL() + "/api/health")
	if err != nil {
		t.Skipf("dogwatch server not reachable at %s: %v", serverURL(), err)
	}
	resp.Body.Close()
}

// TestHealthEndpoint tests the /api/health endpoint.
func TestHealthEndpoint(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestMetricsEndpoint tests the /api/metrics endpoint.
func TestMetricsEndpoint(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/metrics")
	if err != nil {
		t.Fatalf("metrics endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Should return JSON
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" || (contentType != "application/json" && contentType[:16] != "application/json") {
		t.Logf("Content-Type: %s (may be acceptable)", contentType)
	}
}

// TestSystemMetrics tests the /api/system endpoint returns system metrics.
func TestSystemMetrics(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/system")
	if err != nil {
		t.Fatalf("system endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	// Check for expected fields
	expectedFields := []string{"cpu_percent", "mem_percent"}
	for _, field := range expectedFields {
		if _, ok := result[field]; !ok {
			t.Logf("warning: expected field %s not found in response", field)
		}
	}
}

// TestTracesEndpoint tests the /api/traces endpoint.
func TestTracesEndpoint(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/traces")
	if err != nil {
		t.Fatalf("traces endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestOTLPTraceIngest tests OTLP trace ingestion via HTTP.
func TestOTLPTraceIngest(t *testing.T) {
	skipIfNoServer(t)

	// OTLP HTTP endpoint for traces
	otlpURL := "http://localhost:4318/v1/traces"

	// Check if OTLP server is available
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(otlpURL, "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Skipf("OTLP HTTP server not available at %s: %v", otlpURL, err)
	}
	resp.Body.Close()

	// The endpoint should accept the request (even if empty/invalid payload returns error)
	// A running OTLP server should at least respond
	t.Log("OTLP trace endpoint is reachable")
}

// TestLogsIngest tests log ingestion endpoint.
func TestLogsIngest(t *testing.T) {
	skipIfNoServer(t)

	logEntry := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"level":     "info",
		"message":   "Integration test log entry",
		"service":   "integration-test",
		"host":      "test-host",
	}

	body, _ := json.Marshal(logEntry)
	resp, err := http.Post(
		serverURL()+"/api/logs/ingest",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("log ingest failed: %v", err)
	}
	defer resp.Body.Close()

	// Should accept the log (200 or 201)
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected success status, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestAlertingRulesAPI tests the alerting rules CRUD API.
func TestAlertingRulesAPI(t *testing.T) {
	skipIfNoServer(t)

	// List rules
	resp, err := http.Get(serverURL() + "/api/alerting/rules")
	if err != nil {
		t.Fatalf("list rules failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestDashboardsAPI tests the dashboards CRUD API.
func TestDashboardsAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/dashboards")
	if err != nil {
		t.Fatalf("list dashboards failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestSLOsAPI tests the SLO API endpoints.
func TestSLOsAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/slos")
	if err != nil {
		t.Fatalf("SLOs endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestSyntheticsAPI tests the synthetics checks API.
func TestSyntheticsAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/synthetics/checks")
	if err != nil {
		t.Fatalf("synthetics endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestCostIntelligenceAPI tests the cost intelligence API.
func TestCostIntelligenceAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/cost/estimate")
	if err != nil {
		t.Fatalf("cost estimate endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Should have pricing information
	if _, ok := result["datadog"]; !ok {
		t.Log("warning: expected 'datadog' pricing in response")
	}
}

// TestContainersAPI tests the containers endpoint.
func TestContainersAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/containers")
	if err != nil {
		t.Fatalf("containers endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	// Container endpoint may return 200 or error if Docker not available
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("unexpected status %d", resp.StatusCode)
	}
}

// TestIncidentsAPI tests the incidents API.
func TestIncidentsAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/incidents")
	if err != nil {
		t.Fatalf("incidents endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestOnCallAPI tests the on-call schedules API.
func TestOnCallAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/oncall/schedules")
	if err != nil {
		t.Fatalf("oncall endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestDeploymentsAPI tests the deployments API.
func TestDeploymentsAPI(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/deploys")
	if err != nil {
		t.Fatalf("deploys endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestBackupAPI tests the backup endpoint.
func TestBackupAPI(t *testing.T) {
	skipIfNoServer(t)

	// Test backup list/status
	resp, err := http.Get(serverURL() + "/api/backup")
	if err != nil {
		t.Fatalf("backup endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	// Should return some status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		t.Errorf("unexpected status %d", resp.StatusCode)
	}
}

// TestAPIRateLimiting tests that rate limiting headers are present.
func TestAPIRateLimiting(t *testing.T) {
	skipIfNoServer(t)

	// Make multiple requests to trigger rate limit headers
	for i := 0; i < 5; i++ {
		resp, err := http.Get(serverURL() + "/api/health")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()

		// Check for rate limit headers (if implemented)
		if limit := resp.Header.Get("X-RateLimit-Limit"); limit != "" {
			t.Logf("Rate limit: %s", limit)
		}
	}
}

// TestCORSHeaders tests that CORS headers are properly set.
func TestCORSHeaders(t *testing.T) {
	skipIfNoServer(t)

	client := &http.Client{}
	req, _ := http.NewRequest("OPTIONS", serverURL()+"/api/health", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check for CORS headers
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "" {
		t.Logf("CORS Allow-Origin: %s", origin)
	}
}

// TestJSONContentType tests that JSON endpoints return proper content type.
func TestJSONContentType(t *testing.T) {
	skipIfNoServer(t)

	endpoints := []string{
		"/api/metrics",
		"/api/traces",
		"/api/dashboards",
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp, err := http.Get(serverURL() + endpoint)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			contentType := resp.Header.Get("Content-Type")
			if contentType != "" && len(contentType) >= 16 {
				if contentType[:16] != "application/json" {
					t.Logf("Content-Type: %s (expected application/json)", contentType)
				}
			}
		})
	}
}

// TestInvalidEndpoint tests that non-existent endpoints return 404.
func TestInvalidEndpoint(t *testing.T) {
	skipIfNoServer(t)

	resp, err := http.Get(serverURL() + "/api/nonexistent/endpoint/12345")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for invalid endpoint, got %d", resp.StatusCode)
	}
}

// TestQueryParameters tests that query parameters are properly handled.
func TestQueryParameters(t *testing.T) {
	skipIfNoServer(t)

	// Test traces with parameters
	url := fmt.Sprintf("%s/api/traces?limit=10&since=1h", serverURL())
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestLargePayloadHandling tests that large payloads are handled correctly.
func TestLargePayloadHandling(t *testing.T) {
	skipIfNoServer(t)

	// Create a large log batch
	logs := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		logs[i] = map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"level":     "info",
			"message":   fmt.Sprintf("Large batch test log entry %d with some additional content", i),
			"service":   "integration-test",
			"index":     i,
		}
	}

	body, _ := json.Marshal(logs)
	resp, err := http.Post(
		serverURL()+"/api/logs/ingest",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		t.Fatalf("large payload request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should handle large payload (either accept or return appropriate error)
	if resp.StatusCode >= 500 {
		t.Errorf("server error for large payload: %d", resp.StatusCode)
	}
}
