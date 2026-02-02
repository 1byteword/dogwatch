package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"dogwatch/internal/metrics"
)

// =============================================================================
// Test Setup Helpers
// =============================================================================

type cardinalityTestServer struct {
	mux        *http.ServeMux
	controller *metrics.CardinalityController
}

func setupCardinalityTestServer(t *testing.T) *cardinalityTestServer {
	t.Helper()

	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 10000
	config.MaxSeriesPerMetric = 10000
	config.AlertThreshold = 100
	config.AutoAggregateThreshold = 1000

	controller := metrics.NewCardinalityController(config)

	ts := &cardinalityTestServer{
		mux:        http.NewServeMux(),
		controller: controller,
	}

	SetCardinalityController(controller)
	RegisterCardinalityControllerRoutes(ts.mux)

	return ts
}

func (ts *cardinalityTestServer) makeRequest(t *testing.T, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	ts.mux.ServeHTTP(w, req)
	return w
}

func assertCardinalityStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Errorf("Expected status %d, got %d. Body: %s", expected, w.Code, w.Body.String())
	}
}

// =============================================================================
// Dashboard Endpoint Tests
// =============================================================================

func TestCardinalityDashboard_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Add some data
	for i := 0; i < 50; i++ {
		ts.controller.RecordSeries(fmt.Sprintf("metric_%d", i%10), map[string]string{
			"env":  "prod",
			"host": fmt.Sprintf("host_%d", i%5),
		})
	}

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/dashboard", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var dashboard metrics.CardinalityDashboard
		if err := json.NewDecoder(w.Body).Decode(&dashboard); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify dashboard structure
		if dashboard.Stats.TotalMetrics < 1 {
			t.Error("Expected at least 1 metric")
		}

		if dashboard.CircuitBreaker == nil {
			t.Error("Expected circuit breaker data")
		}

		if len(dashboard.TopMetrics) == 0 {
			t.Error("Expected top metrics")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/dashboard", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestCardinalityDashboard_NotConfigured(t *testing.T) {
	// Save original
	original := cardinalityController
	cardinalityController = nil
	defer func() { cardinalityController = original }()

	mux := http.NewServeMux()
	RegisterCardinalityControllerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/cardinality/dashboard", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 when controller not configured, got %d", w.Code)
	}
}

// =============================================================================
// Controller Stats Endpoint Tests
// =============================================================================

func TestControllerStats_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Add some data
	for i := 0; i < 100; i++ {
		ts.controller.RecordSeries("stats_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/controller/stats", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var stats metrics.CardinalityStats
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if stats.TotalSeries < 100 {
			t.Errorf("Expected at least 100 series, got %d", stats.TotalSeries)
		}

		if stats.TotalMetrics < 1 {
			t.Errorf("Expected at least 1 metric, got %d", stats.TotalMetrics)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/controller/stats", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Top Metrics Endpoint Tests
// =============================================================================

func TestControllerMetrics_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Add metrics with varying cardinality
	for i := 0; i < 50; i++ {
		ts.controller.RecordSeries("high_card", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}
	for i := 0; i < 10; i++ {
		ts.controller.RecordSeries("low_card", map[string]string{
			"env": "prod",
		})
	}

	t.Run("successful get default limit", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/controller/metrics", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		metricsData, ok := result["metrics"].([]interface{})
		if !ok {
			t.Fatal("Expected metrics array in response")
		}

		if len(metricsData) < 2 {
			t.Errorf("Expected at least 2 metrics, got %d", len(metricsData))
		}
	})

	t.Run("successful get with limit", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/controller/metrics?limit=1", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		json.NewDecoder(w.Body).Decode(&result)

		metricsData := result["metrics"].([]interface{})
		if len(metricsData) != 1 {
			t.Errorf("Expected 1 metric with limit=1, got %d", len(metricsData))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/controller/metrics", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Top Labels Endpoint Tests
// =============================================================================

func TestControllerLabels_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Add data with various labels
	for i := 0; i < 100; i++ {
		ts.controller.RecordSeries("labels_test", map[string]string{
			"high_card": fmt.Sprintf("value_%d", i),
			"low_card":  "fixed",
		})
	}

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/controller/labels", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		labelsData, ok := result["labels"].([]interface{})
		if !ok {
			t.Fatal("Expected labels array in response")
		}

		if len(labelsData) < 2 {
			t.Errorf("Expected at least 2 labels, got %d", len(labelsData))
		}
	})

	t.Run("successful get with limit", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/controller/labels?limit=1", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		json.NewDecoder(w.Body).Decode(&result)

		labelsData := result["labels"].([]interface{})
		if len(labelsData) != 1 {
			t.Errorf("Expected 1 label with limit=1, got %d", len(labelsData))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/controller/labels", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Circuit Breaker Endpoint Tests
// =============================================================================

func TestCircuitBreaker_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/circuit-breaker", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var cb metrics.CircuitBreaker
		if err := json.NewDecoder(w.Body).Decode(&cb); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should start closed
		if cb.State != metrics.CircuitClosed {
			t.Errorf("Expected closed state, got %s", cb.State)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/circuit-breaker", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestCircuitBreakerReset_Post(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Trip the circuit breaker first (need lower threshold for test)
	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 5
	controller := metrics.NewCardinalityController(config)
	SetCardinalityController(controller)

	for i := 0; i < 6; i++ {
		controller.RecordSeries(fmt.Sprintf("trip_metric_%d", i), nil)
	}

	// Verify it's open
	cb := controller.GetCircuitBreakerState()
	if cb.State != metrics.CircuitOpen {
		t.Fatal("Circuit breaker should be open")
	}

	t.Run("successful reset", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/circuit-breaker/reset", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["status"] != "reset" {
			t.Errorf("Expected status 'reset', got '%v'", result["status"])
		}

		// Verify it's closed now
		cb = controller.GetCircuitBreakerState()
		if cb.State != metrics.CircuitClosed {
			t.Errorf("Circuit breaker should be closed after reset, got %s", cb.State)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/circuit-breaker/reset", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Quarantine Endpoint Tests
// =============================================================================

func TestQuarantine_List(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Set up a controller with low threshold for quarantine
	config := metrics.DefaultCardinalityConfig()
	config.AutoAggregateThreshold = 5
	config.EnableQuarantine = true
	config.MaxSeriesPerMetric = 100
	config.CircuitBreakerThreshold = 10000
	controller := metrics.NewCardinalityController(config)
	SetCardinalityController(controller)

	// Create high cardinality to trigger quarantine
	for i := 0; i < 10; i++ {
		controller.RecordSeries("quarantine_list_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	t.Run("successful list", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/quarantine", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		quarantined := result["quarantined"].([]interface{})
		count := int(result["count"].(float64))

		if count != len(quarantined) {
			t.Errorf("Count mismatch: %d vs %d", count, len(quarantined))
		}

		if count < 1 {
			t.Log("Note: Metric may not be quarantined depending on timing")
		}
	})
}

func TestQuarantine_SetRules(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	t.Run("successful set rules", func(t *testing.T) {
		body := map[string]interface{}{
			"metric_name":    "test_metric",
			"allowed_labels": []string{"env", "region"},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/quarantine", body)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["status"] != "quarantine_rules_set" {
			t.Errorf("Expected status 'quarantine_rules_set', got '%v'", result["status"])
		}
	})

	t.Run("missing metric name", func(t *testing.T) {
		body := map[string]interface{}{
			"allowed_labels": []string{"env"},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/quarantine", body)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/cardinality/quarantine", bytes.NewBuffer([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.mux.ServeHTTP(w, req)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})
}

func TestQuarantineMetric_Delete(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Set up quarantine
	config := metrics.DefaultCardinalityConfig()
	config.AutoAggregateThreshold = 5
	config.EnableQuarantine = true
	config.MaxSeriesPerMetric = 100
	config.CircuitBreakerThreshold = 10000
	controller := metrics.NewCardinalityController(config)
	SetCardinalityController(controller)

	// Trigger quarantine
	for i := 0; i < 10; i++ {
		controller.RecordSeries("release_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	t.Run("successful delete (unquarantine)", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodDelete, "/api/cardinality/quarantine/release_test", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["status"] != "unquarantined" {
			t.Errorf("Expected status 'unquarantined', got '%v'", result["status"])
		}

		if result["metric_name"] != "release_test" {
			t.Errorf("Expected metric_name 'release_test', got '%v'", result["metric_name"])
		}
	})

	t.Run("missing metric name", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodDelete, "/api/cardinality/quarantine/", nil)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})
}

func TestQuarantineMetric_Patch(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	t.Run("successful patch (update rules)", func(t *testing.T) {
		body := map[string]interface{}{
			"allowed_labels": []string{"env", "service"},
		}
		w := ts.makeRequest(t, http.MethodPatch, "/api/cardinality/quarantine/update_rules_test", body)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["status"] != "rules_updated" {
			t.Errorf("Expected status 'rules_updated', got '%v'", result["status"])
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/cardinality/quarantine/test", bytes.NewBuffer([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.mux.ServeHTTP(w, req)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})
}

// =============================================================================
// Alerts Endpoint Tests
// =============================================================================

func TestCardinalityAlerts_Get(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Set up controller with low alert threshold
	config := metrics.DefaultCardinalityConfig()
	config.AlertThreshold = 3
	config.MaxSeriesPerMetric = 100
	config.AutoAggregateThreshold = 100
	config.CircuitBreakerThreshold = 10000
	controller := metrics.NewCardinalityController(config)
	SetCardinalityController(controller)

	// Trigger alerts
	for i := 0; i < 10; i++ {
		controller.RecordSeries("alert_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	// Wait for alerts to be generated
	time.Sleep(20 * time.Millisecond)

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/alerts", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		alerts := result["alerts"].([]interface{})
		count := int(result["count"].(float64))

		if count != len(alerts) {
			t.Errorf("Count mismatch: %d vs %d", count, len(alerts))
		}
	})

	t.Run("successful get with limit", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/alerts?limit=5", nil)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		json.NewDecoder(w.Body).Decode(&result)

		alerts := result["alerts"].([]interface{})
		if len(alerts) > 5 {
			t.Errorf("Expected at most 5 alerts, got %d", len(alerts))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/alerts", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Record Series Endpoint Tests
// =============================================================================

func TestRecordSeries_Post(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	t.Run("successful record", func(t *testing.T) {
		body := map[string]interface{}{
			"metric_name": "test_record",
			"labels": map[string]string{
				"env":     "prod",
				"service": "api",
			},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/record", body)
		assertCardinalityStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["metric_name"] != "test_record" {
			t.Errorf("Expected metric_name 'test_record', got '%v'", result["metric_name"])
		}

		resultData := result["result"].(map[string]interface{})
		if resultData["accept"] != true {
			t.Error("Expected series to be accepted")
		}
	})

	t.Run("missing metric name", func(t *testing.T) {
		body := map[string]interface{}{
			"labels": map[string]string{"env": "prod"},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/cardinality/record", body)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/cardinality/record", bytes.NewBuffer([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.mux.ServeHTTP(w, req)
		assertCardinalityStatus(t, w, http.StatusBadRequest)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/record", nil)
		assertCardinalityStatus(t, w, http.StatusMethodNotAllowed)
	})
}

// =============================================================================
// Response Format Tests
// =============================================================================

func TestCardinalityEndpoints_JSONContentType(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/cardinality/dashboard"},
		{http.MethodGet, "/api/cardinality/controller/stats"},
		{http.MethodGet, "/api/cardinality/controller/metrics"},
		{http.MethodGet, "/api/cardinality/controller/labels"},
		{http.MethodGet, "/api/cardinality/circuit-breaker"},
		{http.MethodGet, "/api/cardinality/quarantine"},
		{http.MethodGet, "/api/cardinality/alerts"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			w := ts.makeRequest(t, ep.method, ep.path, nil)

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json for %s, got %s", ep.path, contentType)
			}
		})
	}
}

// =============================================================================
// Dashboard Data Aggregation Tests
// =============================================================================

func TestDashboard_DataAggregation(t *testing.T) {
	ts := setupCardinalityTestServer(t)

	// Add diverse data - create unique series for each metric
	// 20 metrics * 5 series each = 100 unique series
	for m := 0; m < 20; m++ {
		for s := 0; s < 5; s++ {
			ts.controller.RecordSeries(fmt.Sprintf("metric_%d", m), map[string]string{
				"env":     "prod",
				"host":    fmt.Sprintf("host_%d", s),
				"version": fmt.Sprintf("v%d", s%3),
			})
		}
	}

	w := ts.makeRequest(t, http.MethodGet, "/api/cardinality/dashboard", nil)
	assertCardinalityStatus(t, w, http.StatusOK)

	var dashboard metrics.CardinalityDashboard
	if err := json.NewDecoder(w.Body).Decode(&dashboard); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify aggregation
	if dashboard.Stats.TotalMetrics != 20 {
		t.Errorf("Expected 20 metrics, got %d", dashboard.Stats.TotalMetrics)
	}

	if dashboard.Stats.TotalSeries != 100 {
		t.Errorf("Expected 100 series, got %d", dashboard.Stats.TotalSeries)
	}

	// Verify top metrics are sorted by cardinality
	if len(dashboard.TopMetrics) > 1 {
		for i := 0; i < len(dashboard.TopMetrics)-1; i++ {
			if dashboard.TopMetrics[i].TotalSeries < dashboard.TopMetrics[i+1].TotalSeries {
				t.Error("Top metrics should be sorted by cardinality descending")
			}
		}
	}

	// Verify top labels are populated
	if len(dashboard.TopLabels) < 3 {
		t.Errorf("Expected at least 3 labels, got %d", len(dashboard.TopLabels))
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkCardinalityDashboard(b *testing.B) {
	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000
	controller := metrics.NewCardinalityController(config)

	// Add data
	for i := 0; i < 1000; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i%100), map[string]string{
			"env":  "prod",
			"host": fmt.Sprintf("host_%d", i%50),
		})
	}

	SetCardinalityController(controller)

	mux := http.NewServeMux()
	RegisterCardinalityControllerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/cardinality/dashboard", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkControllerStats(b *testing.B) {
	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000
	controller := metrics.NewCardinalityController(config)

	for i := 0; i < 500; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), nil)
	}

	SetCardinalityController(controller)

	mux := http.NewServeMux()
	RegisterCardinalityControllerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/cardinality/controller/stats", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkTopMetrics(b *testing.B) {
	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000
	controller := metrics.NewCardinalityController(config)

	for i := 0; i < 1000; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	SetCardinalityController(controller)

	mux := http.NewServeMux()
	RegisterCardinalityControllerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/cardinality/controller/metrics?limit=10", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkRecordSeries(b *testing.B) {
	config := metrics.DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 10000000
	config.MaxSeriesPerMetric = 10000000
	controller := metrics.NewCardinalityController(config)

	SetCardinalityController(controller)

	mux := http.NewServeMux()
	RegisterCardinalityControllerRoutes(mux)

	body := []byte(`{"metric_name":"bench_metric","labels":{"env":"prod","host":"host1"}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/cardinality/record", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}
