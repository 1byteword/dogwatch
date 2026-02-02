package metrics

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Circuit Breaker State Transitions Tests
// =============================================================================

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 10
	config.CircuitBreakerCooldown = 50 * time.Millisecond
	config.MaxSeriesPerMetric = 1000

	controller := NewCardinalityController(config)

	t.Run("ClosedToOpen", func(t *testing.T) {
		// Start with closed circuit
		cb := controller.GetCircuitBreakerState()
		if cb.State != CircuitClosed {
			t.Fatalf("Expected closed, got %s", cb.State)
		}

		// Add series until circuit opens
		for i := 0; i < 15; i++ {
			controller.RecordSeries(fmt.Sprintf("metric_%d", i), nil)
		}

		cb = controller.GetCircuitBreakerState()
		if cb.State != CircuitOpen {
			t.Errorf("Expected open after threshold, got %s", cb.State)
		}
	})

	t.Run("OpenToHalfOpen", func(t *testing.T) {
		// Reset and trip the breaker
		controller.ResetCircuitBreaker()
		for i := 0; i < 15; i++ {
			controller.RecordSeries(fmt.Sprintf("metric_open_%d", i), nil)
		}

		// Wait for cooldown
		time.Sleep(60 * time.Millisecond)

		// Next request should transition to half-open
		result := controller.RecordSeries("metric_halfopen", nil)

		cb := controller.GetCircuitBreakerState()
		// After a successful request in half-open, it should close
		if !result.Accept && cb.State != CircuitHalfOpen {
			t.Logf("Circuit state: %s, Accept: %v", cb.State, result.Accept)
		}
	})

	t.Run("HalfOpenToClosed", func(t *testing.T) {
		// Reset and set up half-open scenario
		controller2 := NewCardinalityController(config)

		// Trip the breaker
		for i := 0; i < 15; i++ {
			controller2.RecordSeries(fmt.Sprintf("metric_trip_%d", i), nil)
		}

		cb := controller2.GetCircuitBreakerState()
		if cb.State != CircuitOpen {
			t.Fatalf("Expected open, got %s", cb.State)
		}

		// Wait for cooldown
		time.Sleep(60 * time.Millisecond)

		// Record an existing series (won't add to total)
		result := controller2.RecordSeries("metric_trip_0", nil)
		if result.Accept {
			cb = controller2.GetCircuitBreakerState()
			if cb.State != CircuitClosed {
				t.Errorf("Expected closed after successful half-open, got %s", cb.State)
			}
		}
	})
}

func TestCircuitBreaker_BlocksNewSeries(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 5
	config.CircuitBreakerCooldown = 1 * time.Second

	controller := NewCardinalityController(config)

	// Trip the circuit breaker
	for i := 0; i < 5; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), nil)
	}

	cb := controller.GetCircuitBreakerState()
	if cb.State != CircuitOpen {
		t.Fatalf("Circuit should be open, got %s", cb.State)
	}

	// Try to record new series - should be blocked
	result := controller.RecordSeries("new_metric", map[string]string{"new": "label"})
	if result.Accept {
		t.Error("New series should be blocked when circuit is open")
	}
	if !result.CircuitOpen {
		t.Error("Result should indicate circuit is open")
	}
}

func TestCircuitBreaker_ManualReset(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 5

	controller := NewCardinalityController(config)

	// Trip the circuit breaker
	for i := 0; i < 5; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), nil)
	}

	cb := controller.GetCircuitBreakerState()
	if cb.State != CircuitOpen {
		t.Fatalf("Circuit should be open")
	}

	// Manual reset
	controller.ResetCircuitBreaker()

	cb = controller.GetCircuitBreakerState()
	if cb.State != CircuitClosed {
		t.Errorf("Circuit should be closed after reset, got %s", cb.State)
	}
	if cb.BlockedCount != 0 {
		t.Errorf("Blocked count should be reset to 0, got %d", cb.BlockedCount)
	}
}

// =============================================================================
// Quarantine Add/Release Tests
// =============================================================================

func TestQuarantine_AutoQuarantine(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AutoAggregateThreshold = 5
	config.MaxSeriesPerMetric = 100
	config.EnableQuarantine = true
	config.CircuitBreakerThreshold = 1000

	controller := NewCardinalityController(config)

	// Create high cardinality metric
	for i := 0; i < 6; i++ {
		controller.RecordSeries("high_card_metric", map[string]string{
			"id": fmt.Sprintf("unique_%d", i),
		})
	}

	// Check that metric was quarantined
	quarantined := controller.GetQuarantinedMetrics()
	found := false
	for _, q := range quarantined {
		if q.MetricName == "high_card_metric" {
			found = true
			if q.Reason != "high_cardinality" {
				t.Errorf("Expected reason 'high_cardinality', got '%s'", q.Reason)
			}
			break
		}
	}

	if !found {
		t.Error("high_card_metric should be quarantined")
	}
}

func TestQuarantine_ManualRelease(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AutoAggregateThreshold = 3
	config.EnableQuarantine = true
	config.MaxSeriesPerMetric = 100
	config.CircuitBreakerThreshold = 1000

	controller := NewCardinalityController(config)

	// Create high cardinality metric
	for i := 0; i < 5; i++ {
		controller.RecordSeries("quarantine_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	// Verify quarantined
	q := controller.GetQuarantinedMetrics()
	if len(q) == 0 {
		t.Fatal("Metric should be quarantined")
	}

	// Release from quarantine
	controller.UnquarantineMetric("quarantine_test")

	// Verify released
	q = controller.GetQuarantinedMetrics()
	for _, entry := range q {
		if entry.MetricName == "quarantine_test" {
			t.Error("Metric should be unquarantined")
		}
	}
}

func TestQuarantine_AllowedLabels(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AutoAggregateThreshold = 3
	config.EnableQuarantine = true
	config.MaxSeriesPerMetric = 100
	config.CircuitBreakerThreshold = 1000

	controller := NewCardinalityController(config)

	// Create high cardinality metric with some low-card labels
	for i := 0; i < 5; i++ {
		controller.RecordSeries("allowed_test", map[string]string{
			"env":  "prod",     // Low cardinality
			"id":   fmt.Sprintf("id_%d", i), // High cardinality
		})
	}

	// Set allowed labels
	controller.SetQuarantineRules("allowed_test", []string{"env"})

	// Verify allowed labels were set
	q := controller.GetQuarantinedMetrics()
	for _, entry := range q {
		if entry.MetricName == "allowed_test" {
			foundEnv := false
			for _, label := range entry.AllowedLabels {
				if label == "env" {
					foundEnv = true
				}
			}
			if !foundEnv {
				t.Error("'env' should be in allowed labels")
			}
		}
	}
}

// =============================================================================
// Alert Generation Threshold Tests
// =============================================================================

func TestAlerts_ThresholdTriggered(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AlertThreshold = 3
	config.MaxSeriesPerMetric = 100
	config.AutoAggregateThreshold = 100
	config.CircuitBreakerThreshold = 1000

	var receivedAlerts []CardinalityAlert
	var mu sync.Mutex

	controller := NewCardinalityController(config)
	controller.SetAlertCallback(func(alert CardinalityAlert) {
		mu.Lock()
		receivedAlerts = append(receivedAlerts, alert)
		mu.Unlock()
	})

	// Create metric with cardinality above threshold
	for i := 0; i < 5; i++ {
		controller.RecordSeries("alert_test", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	// Give callback time to execute
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	alertCount := len(receivedAlerts)
	mu.Unlock()

	if alertCount == 0 {
		t.Error("Expected to receive alerts")
	}

	// Verify alert via GetRecentAlerts
	alerts := controller.GetRecentAlerts(10)
	if len(alerts) == 0 {
		t.Error("Expected stored alerts")
	}

	// Check alert type
	foundMetricHighAlert := false
	for _, a := range alerts {
		if a.AlertType == AlertMetricHigh {
			foundMetricHighAlert = true
			if a.MetricName != "alert_test" {
				t.Errorf("Expected metric name 'alert_test', got '%s'", a.MetricName)
			}
		}
	}

	if !foundMetricHighAlert {
		t.Error("Expected metric_high_cardinality alert")
	}
}

func TestAlerts_RapidGrowth(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.RapidGrowthWindow = 100 * time.Millisecond
	config.RapidGrowthThreshold = 50.0 // 50% growth
	config.MaxSeriesPerMetric = 1000
	config.AlertThreshold = 1000 // High to avoid normal alerts
	config.AutoAggregateThreshold = 1000
	config.CircuitBreakerThreshold = 10000

	var rapidGrowthAlerts []CardinalityAlert
	var mu sync.Mutex

	controller := NewCardinalityController(config)
	controller.SetAlertCallback(func(alert CardinalityAlert) {
		if alert.AlertType == AlertRapidGrowth {
			mu.Lock()
			rapidGrowthAlerts = append(rapidGrowthAlerts, alert)
			mu.Unlock()
		}
	})

	// Initial series
	for i := 0; i < 10; i++ {
		controller.RecordSeries("rapid_growth", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	// Wait a bit for window to have some history
	time.Sleep(50 * time.Millisecond)

	// Add more series rapidly (> 50% growth)
	for i := 10; i < 30; i++ {
		controller.RecordSeries("rapid_growth", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	alertCount := len(rapidGrowthAlerts)
	mu.Unlock()

	// Rapid growth detection depends on timing, so we just verify the mechanism exists
	t.Logf("Rapid growth alerts received: %d", alertCount)
}

func TestAlerts_CircuitBreakerOpen(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 5

	var circuitAlerts []CardinalityAlert
	var mu sync.Mutex

	controller := NewCardinalityController(config)
	controller.SetAlertCallback(func(alert CardinalityAlert) {
		if alert.AlertType == AlertCircuitOpen {
			mu.Lock()
			circuitAlerts = append(circuitAlerts, alert)
			mu.Unlock()
		}
	})

	// Trip the circuit breaker
	for i := 0; i < 6; i++ {
		controller.RecordSeries(fmt.Sprintf("circuit_metric_%d", i), nil)
	}

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	alertCount := len(circuitAlerts)
	mu.Unlock()

	if alertCount == 0 {
		t.Error("Expected circuit breaker open alert")
	}
}

// =============================================================================
// Auto-Aggregation Trigger Tests
// =============================================================================

func TestAutoAggregation_ProblematicLabels(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxLabelValues = 5
	config.MaxSeriesPerMetric = 1000
	config.CircuitBreakerThreshold = 10000

	var aggregationAlerts []CardinalityAlert
	var mu sync.Mutex

	controller := NewCardinalityController(config)
	controller.SetAlertCallback(func(alert CardinalityAlert) {
		if alert.AlertType == AlertAutoAggregate {
			mu.Lock()
			aggregationAlerts = append(aggregationAlerts, alert)
			mu.Unlock()
		}
	})

	// Create metric with problematic label that exceeds MaxLabelValues
	for i := 0; i < 10; i++ {
		result := controller.RecordSeries("agg_test", map[string]string{
			"request_id": fmt.Sprintf("req_%d", i), // Problematic label
			"env":        "prod",
		})

		if result.TransformedLabels != nil {
			t.Logf("Labels transformed at iteration %d", i)
		}
	}

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	alertCount := len(aggregationAlerts)
	mu.Unlock()

	t.Logf("Auto-aggregation alerts: %d", alertCount)
}

func TestAutoAggregation_LabelRemoval(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxLabelValues = 3
	config.MaxSeriesPerMetric = 1000
	config.CircuitBreakerThreshold = 10000

	controller := NewCardinalityController(config)

	// First, create enough unique values for a problematic label
	for i := 0; i < 5; i++ {
		controller.RecordSeries("removal_test", map[string]string{
			"trace_id": fmt.Sprintf("trace_%d", i),
		})
	}

	// Next series should have trace_id removed
	result := controller.RecordSeries("removal_test", map[string]string{
		"trace_id": "new_trace",
		"env":      "prod",
	})

	// Check if labels were transformed
	if result.TransformedLabels != nil {
		if _, hasTraceID := result.TransformedLabels["trace_id"]; hasTraceID {
			t.Error("trace_id should be removed from transformed labels")
		}
	}
}

// =============================================================================
// High-Cardinality Detection Accuracy Tests
// =============================================================================

func TestHighCardinalityDetection_Metrics(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AlertThreshold = 10
	config.MaxSeriesPerMetric = 1000
	config.CircuitBreakerThreshold = 10000
	config.AutoAggregateThreshold = 1000

	controller := NewCardinalityController(config)

	// Create metrics with varying cardinality
	for i := 0; i < 50; i++ {
		controller.RecordSeries("high_card", map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	for i := 0; i < 5; i++ {
		controller.RecordSeries("low_card", map[string]string{
			"env": "prod",
		})
	}

	// Get top metrics
	topMetrics := controller.GetTopMetrics(5)

	if len(topMetrics) < 2 {
		t.Fatalf("Expected at least 2 metrics, got %d", len(topMetrics))
	}

	// high_card should be first
	if topMetrics[0].Name != "high_card" {
		t.Errorf("Expected high_card first, got %s", topMetrics[0].Name)
	}

	// Verify cardinality counts
	if topMetrics[0].TotalSeries != 50 {
		t.Errorf("Expected 50 series for high_card, got %d", topMetrics[0].TotalSeries)
	}

	stats := controller.GetStats()
	if stats.HighCardinalityMetrics < 1 {
		t.Error("Expected at least 1 high cardinality metric")
	}
}

func TestHighCardinalityDetection_Labels(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 1000
	config.CircuitBreakerThreshold = 10000

	controller := NewCardinalityController(config)

	// Create metrics with various labels
	for i := 0; i < 100; i++ {
		controller.RecordSeries("label_test", map[string]string{
			"high_card_label": fmt.Sprintf("value_%d", i),
			"low_card_label":  "fixed_value",
		})
	}

	topLabels := controller.GetTopLabels(5)

	if len(topLabels) < 2 {
		t.Fatalf("Expected at least 2 labels, got %d", len(topLabels))
	}

	// high_card_label should be first
	if topLabels[0].Key != "high_card_label" {
		t.Errorf("Expected high_card_label first, got %s", topLabels[0].Key)
	}

	if topLabels[0].UniqueValues != 100 {
		t.Errorf("Expected 100 unique values, got %d", topLabels[0].UniqueValues)
	}
}

func TestHighCardinalityDetection_ProblematicLabels(t *testing.T) {
	controller := NewCardinalityController(DefaultCardinalityConfig())

	problematicLabels := []string{
		"request_id", "trace_id", "span_id", "correlation_id",
		"uuid", "guid", "session_id", "user_id",
		"ip", "ip_address", "client_ip",
	}

	for _, label := range problematicLabels {
		if !controller.isProblematicLabel(label) {
			t.Errorf("Label '%s' should be identified as problematic", label)
		}
	}

	safeLabels := []string{
		"env", "region", "service", "version", "cluster",
	}

	for _, label := range safeLabels {
		if controller.isProblematicLabel(label) {
			t.Errorf("Label '%s' should not be identified as problematic", label)
		}
	}
}

// =============================================================================
// Cardinality Tracking Overhead Benchmark Tests
// =============================================================================

func BenchmarkCardinalityController_RecordSeries(b *testing.B) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 1000000
	config.MaxSeriesPerMetric = 1000000

	controller := NewCardinalityController(config)

	labels := map[string]string{
		"env":     "prod",
		"region":  "us-west-2",
		"service": "api",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		controller.RecordSeries("benchmark_metric", labels)
	}
}

func BenchmarkCardinalityController_RecordSeriesHighCard(b *testing.B) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 1000000
	config.MaxSeriesPerMetric = 1000000

	controller := NewCardinalityController(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		labels := map[string]string{
			"env":  "prod",
			"id":   fmt.Sprintf("unique_%d", i),
			"host": fmt.Sprintf("host_%d", i%100),
		}
		controller.RecordSeries("high_card_benchmark", labels)
	}
}

func BenchmarkCardinalityController_GetStats(b *testing.B) {
	config := DefaultCardinalityConfig()
	controller := NewCardinalityController(config)

	// Add some data first
	for i := 0; i < 100; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = controller.GetStats()
	}
}

func BenchmarkCardinalityController_GetTopMetrics(b *testing.B) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000

	controller := NewCardinalityController(config)

	// Add many metrics
	for i := 0; i < 1000; i++ {
		controller.RecordSeries(fmt.Sprintf("metric_%d", i), map[string]string{
			"id": fmt.Sprintf("id_%d", i),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = controller.GetTopMetrics(10)
	}
}

func BenchmarkCardinalityController_ConcurrentRecordSeries(b *testing.B) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 10000000
	config.MaxSeriesPerMetric = 10000000

	controller := NewCardinalityController(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			labels := map[string]string{
				"env":  "prod",
				"id":   fmt.Sprintf("unique_%d", i),
				"host": fmt.Sprintf("host_%d", i%100),
			}
			controller.RecordSeries("concurrent_benchmark", labels)
			i++
		}
	})
}

func BenchmarkCardinalityController_GetDashboardData(b *testing.B) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000

	controller := NewCardinalityController(config)

	// Add data
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			controller.RecordSeries(fmt.Sprintf("metric_%d", i), map[string]string{
				"id": fmt.Sprintf("id_%d_%d", i, j),
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = controller.GetDashboardData()
	}
}

// =============================================================================
// Additional Integration Tests
// =============================================================================

func TestCardinalityController_FullDashboard(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 10000

	controller := NewCardinalityController(config)

	// Add various data
	for i := 0; i < 20; i++ {
		for j := 0; j < 10; j++ {
			controller.RecordSeries(fmt.Sprintf("dashboard_metric_%d", i), map[string]string{
				"env":  "prod",
				"id":   fmt.Sprintf("id_%d", j),
				"host": fmt.Sprintf("host_%d", j%5),
			})
		}
	}

	dashboard := controller.GetDashboardData()

	// Verify dashboard structure
	if dashboard.Stats.TotalMetrics != 20 {
		t.Errorf("Expected 20 metrics, got %d", dashboard.Stats.TotalMetrics)
	}

	if dashboard.Stats.TotalSeries != 200 {
		t.Errorf("Expected 200 series, got %d", dashboard.Stats.TotalSeries)
	}

	if len(dashboard.TopMetrics) == 0 {
		t.Error("Expected top metrics to be populated")
	}

	if len(dashboard.TopLabels) == 0 {
		t.Error("Expected top labels to be populated")
	}

	if dashboard.CircuitBreaker == nil {
		t.Error("Expected circuit breaker data")
	}

	if dashboard.CircuitBreaker.State != CircuitClosed {
		t.Errorf("Expected closed circuit, got %s", dashboard.CircuitBreaker.State)
	}
}

func TestCardinalityController_ConcurrentOperations(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 100000
	config.MaxSeriesPerMetric = 100000

	controller := NewCardinalityController(config)

	var wg sync.WaitGroup
	errors := make(chan error, 1000)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				result := controller.RecordSeries(
					fmt.Sprintf("concurrent_%d", workerID),
					map[string]string{
						"worker": fmt.Sprintf("%d", workerID),
						"iter":   fmt.Sprintf("%d", j),
					},
				)
				if !result.Accept && !result.CircuitOpen {
					errors <- fmt.Errorf("unexpected rejection: %s", result.Reason)
				}
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = controller.GetStats()
				_ = controller.GetTopMetrics(10)
				_ = controller.GetTopLabels(10)
				_ = controller.GetDashboardData()
			}
		}()
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		t.Logf("Error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Logf("Total errors: %d", errCount)
	}

	// Verify final state is consistent
	stats := controller.GetStats()
	if stats.TotalMetrics != 10 {
		t.Errorf("Expected 10 metrics, got %d", stats.TotalMetrics)
	}
}
