package metrics

import (
	"sync"
	"testing"
	"time"
)

func TestCardinalityController_RecordSeries(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 10
	config.AlertThreshold = 5
	config.CircuitBreakerThreshold = 100

	controller := NewCardinalityController(config)

	// Record some series
	for i := 0; i < 8; i++ {
		result := controller.RecordSeries("http_requests", map[string]string{
			"method": "GET",
			"path":   "/api/v" + itoa(i),
		})

		if !result.Accept {
			t.Errorf("Series %d should be accepted", i)
		}
	}

	// Check stats
	stats := controller.GetStats()
	if stats.TotalSeries != 8 {
		t.Errorf("Expected 8 total series, got %d", stats.TotalSeries)
	}
}

func TestCardinalityController_SeriesLimit(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 5
	config.CircuitBreakerThreshold = 1000

	controller := NewCardinalityController(config)

	// Fill up to limit
	for i := 0; i < 5; i++ {
		result := controller.RecordSeries("test_metric", map[string]string{
			"id": itoa(i),
		})
		if !result.Accept {
			t.Errorf("Series %d should be accepted", i)
		}
	}

	// Next one should be blocked
	result := controller.RecordSeries("test_metric", map[string]string{
		"id": "5",
	})
	if result.Accept {
		t.Error("Series should be blocked due to metric limit")
	}
	if result.Reason != "metric_cardinality_limit_5" {
		t.Errorf("Unexpected reason: %s", result.Reason)
	}
}

func TestCardinalityController_CircuitBreaker(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 1000
	config.CircuitBreakerThreshold = 10
	config.CircuitBreakerCooldown = 100 * time.Millisecond
	// Disable rapid growth alerts for this test
	config.RapidGrowthThreshold = 10000

	controller := NewCardinalityController(config)

	// Fill up to circuit breaker threshold
	for i := 0; i < 10; i++ {
		controller.RecordSeries("metric_"+itoa(i), nil)
	}

	// Check circuit breaker state
	cb := controller.GetCircuitBreakerState()
	if cb.State != CircuitOpen {
		t.Errorf("Expected circuit breaker to be open, got %s", cb.State)
	}

	// New series should be blocked
	result := controller.RecordSeries("blocked_metric", nil)
	if result.Accept {
		t.Error("Series should be blocked by circuit breaker")
	}
	if !result.CircuitOpen {
		t.Error("Result should indicate circuit is open")
	}

	// Wait for cooldown
	time.Sleep(150 * time.Millisecond)

	// Should be half-open now and accept existing series (not a new one)
	// We send an existing series key, which won't add to the total
	result = controller.RecordSeries("metric_0", nil)
	if !result.Accept {
		t.Error("Series should be accepted after cooldown")
	}

	// After accepting a request in half-open state, circuit should close
	// Note: The request succeeded so we transition to closed
	cb = controller.GetCircuitBreakerState()
	if cb.State != CircuitClosed {
		t.Errorf("Expected circuit breaker to be closed after successful request, got %s", cb.State)
	}
}

func TestCardinalityController_ProblematicLabels(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxLabelValues = 3
	config.MaxSeriesPerMetric = 100

	controller := NewCardinalityController(config)

	// Record series with problematic label
	for i := 0; i < 5; i++ {
		controller.RecordSeries("http_requests", map[string]string{
			"request_id": "req-" + itoa(i),
		})
	}

	// Check that label is tracked
	labelTracker := controller.GetLabelCardinality("request_id")
	if labelTracker == nil {
		t.Fatal("Expected label tracker for request_id")
	}

	if labelTracker.UniqueValues != 5 {
		t.Errorf("Expected 5 unique values, got %d", labelTracker.UniqueValues)
	}
}

func TestCardinalityController_Quarantine(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 100
	config.EnableQuarantine = true
	config.AutoAggregateThreshold = 5

	controller := NewCardinalityController(config)

	// Create high cardinality
	for i := 0; i < 6; i++ {
		controller.RecordSeries("bad_metric", map[string]string{
			"id": itoa(i),
		})
	}

	// Check quarantine
	quarantined := controller.GetQuarantinedMetrics()
	if len(quarantined) != 1 {
		t.Fatalf("Expected 1 quarantined metric, got %d", len(quarantined))
	}

	if quarantined[0].MetricName != "bad_metric" {
		t.Errorf("Expected bad_metric to be quarantined, got %s", quarantined[0].MetricName)
	}

	// Unquarantine
	controller.UnquarantineMetric("bad_metric")
	quarantined = controller.GetQuarantinedMetrics()
	if len(quarantined) != 0 {
		t.Error("Metric should be unquarantined")
	}
}

func TestCardinalityController_Alerts(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.AlertThreshold = 3
	config.MaxSeriesPerMetric = 100
	config.AutoAggregateThreshold = 100

	var receivedAlerts []CardinalityAlert
	var mu sync.Mutex

	controller := NewCardinalityController(config)
	controller.SetAlertCallback(func(alert CardinalityAlert) {
		mu.Lock()
		receivedAlerts = append(receivedAlerts, alert)
		mu.Unlock()
	})

	// Generate alert by exceeding threshold
	for i := 0; i < 5; i++ {
		controller.RecordSeries("alerting_metric", map[string]string{
			"id": itoa(i),
		})
	}

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if len(receivedAlerts) == 0 {
		t.Error("Expected to receive at least one alert")
	}
	mu.Unlock()

	// Check alerts via API
	alerts := controller.GetRecentAlerts(10)
	if len(alerts) == 0 {
		t.Error("Expected stored alerts")
	}
}

func TestCardinalityController_GetTopMetrics(t *testing.T) {
	controller := NewCardinalityController(DefaultCardinalityConfig())

	// Create metrics with different cardinalities
	for i := 0; i < 10; i++ {
		controller.RecordSeries("metric_a", map[string]string{"id": itoa(i)})
	}
	for i := 0; i < 5; i++ {
		controller.RecordSeries("metric_b", map[string]string{"id": itoa(i)})
	}
	for i := 0; i < 15; i++ {
		controller.RecordSeries("metric_c", map[string]string{"id": itoa(i)})
	}

	top := controller.GetTopMetrics(2)
	if len(top) != 2 {
		t.Fatalf("Expected 2 top metrics, got %d", len(top))
	}

	// metric_c should be first (highest cardinality)
	if top[0].Name != "metric_c" {
		t.Errorf("Expected metric_c first, got %s", top[0].Name)
	}
	if top[1].Name != "metric_a" {
		t.Errorf("Expected metric_a second, got %s", top[1].Name)
	}
}

func TestCardinalityController_GetTopLabels(t *testing.T) {
	controller := NewCardinalityController(DefaultCardinalityConfig())

	// Create labels with different cardinalities
	for i := 0; i < 10; i++ {
		controller.RecordSeries("metric", map[string]string{
			"env":       "prod",
			"region":    "us-" + itoa(i%3),
			"unique_id": itoa(i),
		})
	}

	top := controller.GetTopLabels(2)
	if len(top) != 2 {
		t.Fatalf("Expected 2 top labels, got %d", len(top))
	}

	// unique_id should be first (10 unique values)
	if top[0].Key != "unique_id" {
		t.Errorf("Expected unique_id first, got %s", top[0].Key)
	}
}

func TestCardinalityController_Dashboard(t *testing.T) {
	controller := NewCardinalityController(DefaultCardinalityConfig())

	// Add some data
	for i := 0; i < 5; i++ {
		controller.RecordSeries("metric_"+itoa(i), map[string]string{
			"env": "prod",
		})
	}

	dashboard := controller.GetDashboardData()

	if dashboard.Stats.TotalMetrics != 5 {
		t.Errorf("Expected 5 metrics, got %d", dashboard.Stats.TotalMetrics)
	}

	if dashboard.CircuitBreaker == nil {
		t.Error("Expected circuit breaker data")
	}

	if dashboard.CircuitBreaker.State != CircuitClosed {
		t.Errorf("Expected closed circuit, got %s", dashboard.CircuitBreaker.State)
	}
}

func TestCardinalityController_ResetCircuitBreaker(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.CircuitBreakerThreshold = 5

	controller := NewCardinalityController(config)

	// Trip the circuit breaker
	for i := 0; i < 5; i++ {
		controller.RecordSeries("metric_"+itoa(i), nil)
	}

	cb := controller.GetCircuitBreakerState()
	if cb.State != CircuitOpen {
		t.Fatal("Circuit breaker should be open")
	}

	// Reset
	controller.ResetCircuitBreaker()

	cb = controller.GetCircuitBreakerState()
	if cb.State != CircuitClosed {
		t.Errorf("Expected closed after reset, got %s", cb.State)
	}
}

func TestCardinalityController_Concurrent(t *testing.T) {
	config := DefaultCardinalityConfig()
	config.MaxSeriesPerMetric = 10000
	config.CircuitBreakerThreshold = 100000

	controller := NewCardinalityController(config)

	var wg sync.WaitGroup
	workers := 10
	iterations := 100

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				controller.RecordSeries("concurrent_metric", map[string]string{
					"worker": itoa(worker),
					"iter":   itoa(i),
				})
			}
		}(w)
	}

	wg.Wait()

	stats := controller.GetStats()
	expected := workers * iterations
	if stats.TotalSeries != expected {
		t.Errorf("Expected %d series, got %d", expected, stats.TotalSeries)
	}
}

func TestBuildSeriesKey(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "metric",
			labels: nil,
			want:   "metric",
		},
		{
			name:   "metric",
			labels: map[string]string{},
			want:   "metric",
		},
		{
			name:   "metric",
			labels: map[string]string{"a": "1"},
			want:   "metric|a=1",
		},
		{
			name:   "metric",
			labels: map[string]string{"b": "2", "a": "1"},
			want:   "metric|a=1|b=2",
		},
	}

	for _, tt := range tests {
		got := buildSeriesKey(tt.name, tt.labels)
		if got != tt.want {
			t.Errorf("buildSeriesKey(%s, %v) = %s, want %s", tt.name, tt.labels, got, tt.want)
		}
	}
}

func TestSeverityForCount(t *testing.T) {
	config := DefaultCardinalityConfig()

	tests := []struct {
		count int
		want  string
	}{
		{1000, "info"},
		{config.AutoAggregateThreshold, "warning"},
		{config.CircuitBreakerThreshold, "critical"},
	}

	for _, tt := range tests {
		got := severityForCount(tt.count, config)
		if got != tt.want {
			t.Errorf("severityForCount(%d) = %s, want %s", tt.count, got, tt.want)
		}
	}
}

func TestIsProblematicLabel(t *testing.T) {
	controller := NewCardinalityController(DefaultCardinalityConfig())

	tests := []struct {
		label string
		want  bool
	}{
		{"request_id", true},
		{"trace_id", true},
		{"user_id", true},
		{"ip_address", true},
		{"env", false},
		{"service", false},
		{"region", false},
		{"my_custom_uuid", true}, // Contains "uuid"
	}

	for _, tt := range tests {
		got := controller.isProblematicLabel(tt.label)
		if got != tt.want {
			t.Errorf("isProblematicLabel(%s) = %v, want %v", tt.label, got, tt.want)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}

	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		b[pos] = '-'
	}

	return string(b[pos:])
}
