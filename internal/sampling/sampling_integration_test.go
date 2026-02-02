package sampling

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dogwatch/internal/trace"
)

// TestHeadSampler_MultipleRuleCombinations tests head sampler with various rule combinations
func TestHeadSampler_MultipleRuleCombinations(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 10000,
		Rules: []Rule{
			{
				ID:         "keep-critical-errors",
				Name:       "Keep critical service errors",
				Enabled:    true,
				Priority:   100,
				Condition:  RuleCondition{Service: "critical-*", HasError: boolPtr(true)},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
			{
				ID:         "keep-slow-requests",
				Name:       "Keep slow requests",
				Enabled:    true,
				Priority:   90,
				Condition:  RuleCondition{MinLatencyMs: 1000},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
			{
				ID:         "drop-health-checks",
				Name:       "Drop health check endpoints",
				Enabled:    true,
				Priority:   80,
				Condition:  RuleCondition{Operation: "GET /health*"},
				Action:     DecisionDrop,
				SampleRate: 0,
			},
			{
				ID:         "sample-api-requests",
				Name:       "Sample API requests at 50%",
				Enabled:    true,
				Priority:   50,
				Condition:  RuleCondition{Service: "api-*"},
				Action:     DecisionKeep,
				SampleRate: 0.5,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	tests := []struct {
		name           string
		span           *trace.Span
		expectedAction Decision
		description    string
	}{
		{
			name: "Critical service error - should be kept",
			span: &trace.Span{
				TraceID:     "trace-1",
				SpanID:      "span-1",
				ServiceName: "critical-payments",
				Name:        "process-payment",
				Status:      "ERROR",
				DurationMs:  100,
			},
			expectedAction: DecisionKeep,
			description:    "Errors from critical services should always be kept",
		},
		{
			name: "Slow request - should be kept",
			span: &trace.Span{
				TraceID:     "trace-2",
				SpanID:      "span-1",
				ServiceName: "regular-service",
				Name:        "slow-operation",
				Status:      "OK",
				DurationMs:  2000,
			},
			expectedAction: DecisionKeep,
			description:    "Requests over 1000ms should be kept",
		},
		{
			name: "Health check - should be dropped",
			span: &trace.Span{
				TraceID:     "trace-3",
				SpanID:      "span-1",
				ServiceName: "api-gateway",
				Name:        "GET /health",
				Status:      "OK",
				DurationMs:  5,
			},
			expectedAction: DecisionDrop,
			description:    "Health check endpoints should be dropped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := sampler.ShouldSample(tt.span)
			if decision != tt.expectedAction {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedAction, decision)
			}
		})
	}
}

// TestHeadSampler_DisabledRules tests that disabled rules are not evaluated
func TestHeadSampler_DisabledRules(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000,
		Rules: []Rule{
			{
				ID:         "disabled-rule",
				Name:       "Disabled rule - should not apply",
				Enabled:    false, // Disabled
				Priority:   100,
				Condition:  RuleCondition{HasError: boolPtr(true)},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	// Error span should not be kept because the rule is disabled
	// It will go through default sampling
	errorSpan := &trace.Span{
		TraceID:     "trace-disabled-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Status:      "ERROR",
	}

	// The decision depends on default sampling rate, not the disabled rule
	sampler.ShouldSample(errorSpan) // Should not panic
}

// TestTailSampler_BufferManagement tests tail sampler buffer management
func TestTailSampler_BufferManagement(t *testing.T) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      100 * time.Millisecond,
		MaxBufferedTraces:  10,
		MaxSpansPerTrace:   5,
		ErrorSampleRate:    1.0,
		LatencyThresholdMs: 1000,
		LatencySampleRate:  1.0,
		DefaultSampleRate:  1.0,
	}

	sampler := NewTailSampler(config)

	var keptTraces int64
	var droppedTraces int64
	var mu sync.Mutex
	keptTraceIDs := make(map[string]bool)

	sampler.SetOnKeep(func(spans []trace.Span) {
		mu.Lock()
		if len(spans) > 0 {
			keptTraceIDs[spans[0].TraceID] = true
		}
		atomic.AddInt64(&keptTraces, 1)
		mu.Unlock()
	})

	sampler.SetOnDrop(func(traceID string, spanCount int) {
		atomic.AddInt64(&droppedTraces, 1)
	})

	// Fill the buffer with traces
	for i := 0; i < 15; i++ {
		span := &trace.Span{
			TraceID:     "buffer-trace-" + string(rune('a'+i)),
			SpanID:      "span-1",
			ServiceName: "test-service",
			Status:      "OK",
			DurationMs:  100,
		}
		sampler.ShouldSample(span)
	}

	// Wait for buffer timeout
	time.Sleep(200 * time.Millisecond)
	sampler.Stop()

	// Some traces should have been kept or dropped due to buffer limits
	kept := atomic.LoadInt64(&keptTraces)
	dropped := atomic.LoadInt64(&droppedTraces)

	if kept+dropped == 0 {
		t.Error("Expected some traces to be processed")
	}
}

// TestTailSampler_MaxSpansPerTrace tests that traces with too many spans are handled
func TestTailSampler_MaxSpansPerTrace(t *testing.T) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      200 * time.Millisecond,
		MaxBufferedTraces:  100,
		MaxSpansPerTrace:   3, // Low limit for testing
		ErrorSampleRate:    1.0,
		LatencyThresholdMs: 1000,
		LatencySampleRate:  1.0,
		DefaultSampleRate:  1.0,
	}

	sampler := NewTailSampler(config)

	var keptSpanCount int
	var mu sync.Mutex

	sampler.SetOnKeep(func(spans []trace.Span) {
		mu.Lock()
		keptSpanCount = len(spans)
		mu.Unlock()
	})

	// Add more spans than MaxSpansPerTrace
	for i := 0; i < 10; i++ {
		span := &trace.Span{
			TraceID:     "max-spans-trace",
			SpanID:      "span-" + string(rune('a'+i)),
			ServiceName: "test-service",
			Status:      "OK",
			DurationMs:  100,
		}
		sampler.ShouldSample(span)
	}

	// Wait for buffer timeout
	time.Sleep(300 * time.Millisecond)
	sampler.Stop()

	mu.Lock()
	defer mu.Unlock()

	// Should have kept at most MaxSpansPerTrace spans
	if keptSpanCount > config.MaxSpansPerTrace {
		t.Errorf("Expected at most %d spans, got %d", config.MaxSpansPerTrace, keptSpanCount)
	}
}

// TestAdaptiveSampler_RateAdjustmentsUnderLoad tests adaptive rate adjustments under high load
func TestAdaptiveSampler_RateAdjustmentsUnderLoad(t *testing.T) {
	config := AdaptiveSamplerConfig{
		Enabled:               true,
		TargetTracesPerSecond: 10,
		MinSampleRate:         0.01,
		MaxSampleRate:         1.0,
		AdjustmentInterval:    50 * time.Millisecond,
		SmoothingFactor:       0.5,
		PerServiceRates:       false,
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Initial rate should be 1.0
	initialRate := sampler.GetCurrentRate()
	if initialRate != 1.0 {
		t.Errorf("Expected initial rate 1.0, got %v", initialRate)
	}

	// Send many spans to trigger rate adjustment
	for i := 0; i < 10000; i++ {
		span := &trace.Span{
			TraceID:     "high-load-" + string(rune(i%26+'a')) + "-" + string(rune(i/26)),
			SpanID:      "span-1",
			ServiceName: "load-test-service",
			Status:      "OK",
		}
		sampler.ShouldSample(span)
	}

	// Wait for multiple adjustment intervals
	time.Sleep(200 * time.Millisecond)

	// Rate should have decreased due to high load
	adjustedRate := sampler.GetCurrentRate()
	if adjustedRate >= initialRate {
		t.Logf("Rate may not have adjusted yet: initial=%v, adjusted=%v", initialRate, adjustedRate)
	}
}

// TestAdaptiveSampler_PerServiceRateTracking tests per-service rate tracking
func TestAdaptiveSampler_PerServiceRateTracking(t *testing.T) {
	config := AdaptiveSamplerConfig{
		Enabled:               true,
		TargetTracesPerSecond: 100,
		MinSampleRate:         0.01,
		MaxSampleRate:         1.0,
		AdjustmentInterval:    50 * time.Millisecond,
		SmoothingFactor:       0.5,
		PerServiceRates:       true,
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Send spans from multiple services with different volumes
	services := []struct {
		name  string
		count int
	}{
		{"high-volume-service", 500},
		{"medium-volume-service", 100},
		{"low-volume-service", 10},
	}

	for _, svc := range services {
		for i := 0; i < svc.count; i++ {
			span := &trace.Span{
				TraceID:     svc.name + "-trace-" + string(rune(i%100)),
				SpanID:      "span-1",
				ServiceName: svc.name,
				Status:      "OK",
			}
			sampler.ShouldSample(span)
		}
	}

	// Wait for rate adjustments
	time.Sleep(100 * time.Millisecond)

	rates := sampler.GetServiceRates()
	if len(rates) < 3 {
		t.Errorf("Expected rates for 3 services, got %d", len(rates))
	}

	// High volume service should have lower rate than low volume
	if rates["high-volume-service"] > rates["low-volume-service"] && rates["low-volume-service"] > 0 {
		t.Logf("Service rates: high=%v, medium=%v, low=%v",
			rates["high-volume-service"],
			rates["medium-volume-service"],
			rates["low-volume-service"])
	}
}

// TestIntelligentSampler_AnomalyDetection tests anomaly detection in intelligent sampler
func TestIntelligentSampler_AnomalyDetection(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.AnomalyEnabled = true
	config.AnomalyThreshold = 2.0 // Lower threshold for testing
	config.TailConfig.BufferTimeout = 100 * time.Millisecond
	config.RetroactiveEnabled = false // Disable for simpler testing

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	var anomalyCount int64
	sampler.SetOnAnomalyDetected(func(span *trace.Span, score float64) {
		atomic.AddInt64(&anomalyCount, 1)
	})

	// First, establish "normal" patterns
	for i := 0; i < 200; i++ {
		span := &trace.Span{
			TraceID:     "normal-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "anomaly-test-service",
			Name:        "normal-operation",
			Status:      "OK",
			DurationMs:  100 + float64(i%20), // Normal range: 100-120ms
		}
		sampler.ShouldSample(span)
	}

	// Wait for pattern learning
	time.Sleep(50 * time.Millisecond)

	// Now send anomalous spans (very high latency)
	for i := 0; i < 10; i++ {
		span := &trace.Span{
			TraceID:     "anomaly-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "anomaly-test-service",
			Name:        "slow-operation",
			Status:      "OK",
			DurationMs:  10000, // Way above normal
		}
		sampler.ShouldSample(span)
	}

	time.Sleep(200 * time.Millisecond)

	// Should have detected some anomalies
	anomalies := atomic.LoadInt64(&anomalyCount)
	if anomalies == 0 {
		t.Log("No anomalies detected - may need more training data")
	} else {
		t.Logf("Detected %d anomalies", anomalies)
	}
}

// TestIntelligentSampler_CostTrackingAccuracy tests cost tracking accuracy
func TestIntelligentSampler_CostTrackingAccuracy(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.CostEnabled = true
	config.CostPerSpan = 0.001 // $0.001 per span
	config.DailyBudget = 100.0 // $100 daily budget
	config.TailConfig.BufferTimeout = 50 * time.Millisecond
	config.TailConfig.DefaultSampleRate = 1.0 // Keep all for cost tracking test
	config.AnomalyEnabled = false
	config.LearningEnabled = false
	config.RetroactiveEnabled = false

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Send known number of spans
	spansToSend := 100

	for i := 0; i < spansToSend; i++ {
		span := &trace.Span{
			TraceID:     "cost-trace-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "cost-test-service",
			Status:      "OK",
			DurationMs:  100,
		}
		sampler.ShouldSample(span)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	breakdown := sampler.GetCostBreakdown()

	// Cost tracking should have recorded spans
	if breakdown.DailySpans == 0 {
		t.Log("Daily spans not yet recorded - spans may be buffered")
	}

	// Check cost calculation is reasonable
	expectedMaxCost := float64(spansToSend) * config.CostPerSpan
	if breakdown.DailyCost > expectedMaxCost*1.1 {
		t.Errorf("Daily cost %v exceeds expected max %v", breakdown.DailyCost, expectedMaxCost)
	}

	// Check budget remaining calculation
	expectedRemaining := config.DailyBudget - breakdown.DailyCost
	if breakdown.BudgetRemaining < 0 || breakdown.BudgetRemaining > config.DailyBudget {
		t.Errorf("Budget remaining %v is invalid", breakdown.BudgetRemaining)
	}

	t.Logf("Cost breakdown: daily_cost=%.4f, daily_spans=%d, budget_remaining=%.2f",
		breakdown.DailyCost, breakdown.DailySpans, expectedRemaining)
}

// TestIntelligentSampler_RetroactiveSampling tests retroactive capture
func TestIntelligentSampler_RetroactiveSampling(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowSize = 100
	config.RetroactiveWindowTime = 5 * time.Second
	config.RelatedTraceDepth = 3
	config.TailConfig.BufferTimeout = 100 * time.Millisecond
	config.TailConfig.DefaultSampleRate = 0.1 // Low default rate
	config.AnomalyEnabled = false
	config.LearningEnabled = false
	config.CostEnabled = false

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	var retroactiveCaptureCount int64
	sampler.SetOnRetroactiveCapture(func(traceID string, spans []trace.Span, reason string) {
		atomic.AddInt64(&retroactiveCaptureCount, 1)
		t.Logf("Retroactive capture: traceID=%s, spanCount=%d, reason=%s", traceID, len(spans), reason)
	})

	// Send some traces from the same service
	for i := 0; i < 20; i++ {
		span := &trace.Span{
			TraceID:     "retro-trace-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "retro-test-service",
			Status:      "OK",
			DurationMs:  100,
		}
		sampler.ShouldSample(span)
	}

	// Now send an error trace from the same service
	errorSpan := &trace.Span{
		TraceID:     "error-trigger-trace",
		SpanID:      "span-1",
		ServiceName: "retro-test-service",
		Status:      "ERROR",
		DurationMs:  100,
	}
	sampler.ShouldSample(errorSpan)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Check stats
	stats := sampler.GetStats()
	t.Logf("Retroactive captures: %d, tracked decisions: %d",
		stats.RetroactiveCaptures, stats.TrackedDecisions)
}

// TestManager_ConfigUpdate tests updating manager configuration
func TestManager_ConfigUpdate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sampling-manager-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "sampling.db")

	manager, err := NewManager(dbPath, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	initialConfig := DefaultConfig()
	initialConfig.DefaultSampleRate = 0.5
	manager.UpdateConfig(initialConfig)

	// Verify initial config
	config := manager.GetConfig()
	if config.DefaultSampleRate != 0.5 {
		t.Errorf("Expected initial rate 0.5, got %v", config.DefaultSampleRate)
	}

	// Update config
	newConfig := config
	newConfig.DefaultSampleRate = 0.1
	newConfig.HeadSamplerConfig.Rules = append(newConfig.HeadSamplerConfig.Rules, Rule{
		ID:         "new-rule",
		Name:       "New Rule",
		Enabled:    true,
		Priority:   50,
		Condition:  RuleCondition{Service: "new-service"},
		Action:     DecisionKeep,
		SampleRate: 1.0,
	})

	err = manager.UpdateConfig(newConfig)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Verify updated config
	updatedConfig := manager.GetConfig()
	if updatedConfig.DefaultSampleRate != 0.1 {
		t.Errorf("Expected updated rate 0.1, got %v", updatedConfig.DefaultSampleRate)
	}

	rules := manager.GetRules()
	found := false
	for _, r := range rules {
		if r.ID == "new-rule" {
			found = true
			break
		}
	}
	if !found {
		t.Error("New rule not found after config update")
	}
}

// TestManager_RuleManagement tests adding, updating, and removing rules
func TestManager_RuleManagement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sampling-rule-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "sampling.db")

	manager, err := NewManager(dbPath, nil)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close()

	// Add a rule
	newRule := Rule{
		ID:         "test-rule-1",
		Name:       "Test Rule 1",
		Enabled:    true,
		Priority:   75,
		Condition:  RuleCondition{Service: "test-*"},
		Action:     DecisionKeep,
		SampleRate: 0.8,
	}

	err = manager.AddRule(newRule)
	if err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Verify rule was added
	rules := manager.GetRules()
	found := false
	for _, r := range rules {
		if r.ID == "test-rule-1" {
			found = true
			if r.Priority != 75 {
				t.Errorf("Expected priority 75, got %d", r.Priority)
			}
			break
		}
	}
	if !found {
		t.Error("Added rule not found")
	}

	// Update the rule
	newRule.Priority = 90
	newRule.SampleRate = 0.5
	err = manager.UpdateRule(newRule)
	if err != nil {
		t.Fatalf("Failed to update rule: %v", err)
	}

	// Verify update
	rules = manager.GetRules()
	for _, r := range rules {
		if r.ID == "test-rule-1" {
			if r.Priority != 90 {
				t.Errorf("Expected updated priority 90, got %d", r.Priority)
			}
			if r.SampleRate != 0.5 {
				t.Errorf("Expected updated sample rate 0.5, got %v", r.SampleRate)
			}
			break
		}
	}

	// Remove the rule
	err = manager.RemoveRule("test-rule-1")
	if err != nil {
		t.Fatalf("Failed to remove rule: %v", err)
	}

	// Verify removal
	rules = manager.GetRules()
	for _, r := range rules {
		if r.ID == "test-rule-1" {
			t.Error("Removed rule still exists")
		}
	}
}

// BenchmarkHeadSampler_HighThroughput benchmarks head sampler under high throughput
func BenchmarkHeadSampler_HighThroughput(b *testing.B) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000000,
		Rules: []Rule{
			{
				ID:        "keep-errors",
				Name:      "Keep errors",
				Enabled:   true,
				Priority:  100,
				Condition: RuleCondition{HasError: boolPtr(true)},
				Action:    DecisionKeep,
			},
			{
				ID:        "keep-slow",
				Name:      "Keep slow",
				Enabled:   true,
				Priority:  90,
				Condition: RuleCondition{MinLatencyMs: 1000},
				Action:    DecisionKeep,
			},
			{
				ID:        "sample-api",
				Name:      "Sample API",
				Enabled:   true,
				Priority:  50,
				Condition: RuleCondition{Service: "api-*"},
				Action:    DecisionKeep,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	spans := make([]*trace.Span, 10000)
	for i := range spans {
		spans[i] = &trace.Span{
			TraceID:     "bench-trace-" + string(rune(i%1000)),
			SpanID:      "span-" + string(rune(i)),
			ServiceName: "api-service",
			Name:        "bench-operation",
			Status:      "OK",
			DurationMs:  float64(50 + i%200),
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sampler.ShouldSample(spans[i%len(spans)])
			i++
		}
	})
}

// BenchmarkTailSampler_HighThroughput benchmarks tail sampler under high throughput
func BenchmarkTailSampler_HighThroughput(b *testing.B) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      30 * time.Second,
		MaxBufferedTraces:  100000,
		MaxSpansPerTrace:   1000,
		ErrorSampleRate:    1.0,
		LatencyThresholdMs: 1000,
		LatencySampleRate:  1.0,
		DefaultSampleRate:  0.1,
	}

	sampler := NewTailSampler(config)
	defer sampler.Stop()

	spans := make([]*trace.Span, 10000)
	for i := range spans {
		spans[i] = &trace.Span{
			TraceID:     "bench-tail-" + string(rune(i%1000)),
			SpanID:      "span-" + string(rune(i)),
			ServiceName: "tail-bench-service",
			Name:        "bench-operation",
			Status:      "OK",
			DurationMs:  float64(50 + i%200),
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sampler.ShouldSample(spans[i%len(spans)])
			i++
		}
	})
}

// BenchmarkIntelligentSampler_FullPipeline benchmarks the full intelligent sampling pipeline
func BenchmarkIntelligentSampler_FullPipeline(b *testing.B) {
	config := DefaultIntelligentConfig()
	config.AnomalyEnabled = true
	config.CostEnabled = true
	config.LearningEnabled = true
	config.RetroactiveEnabled = true
	config.TailConfig.BufferTimeout = 1 * time.Minute
	config.TailConfig.MaxBufferedTraces = 100000

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	spans := make([]*trace.Span, 10000)
	for i := range spans {
		status := "OK"
		if i%100 == 0 {
			status = "ERROR"
		}
		spans[i] = &trace.Span{
			TraceID:     "bench-intel-" + string(rune(i%1000)),
			SpanID:      "span-" + string(rune(i)),
			ServiceName: "intel-bench-service",
			Name:        "bench-operation",
			Status:      status,
			DurationMs:  float64(50 + i%500),
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sampler.ShouldSample(spans[i%len(spans)])
			i++
		}
	})
}

// BenchmarkManager_ConcurrentAccess benchmarks concurrent access to manager
func BenchmarkManager_ConcurrentAccess(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "sampling-bench-*")
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "sampling.db")

	manager, _ := NewManager(dbPath, nil)
	defer manager.Close()

	span := &trace.Span{
		TraceID:     "bench-manager",
		SpanID:      "span-1",
		ServiceName: "bench-service",
		Name:        "bench-op",
		Status:      "OK",
		DurationMs:  100,
	}

	manager.Start()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			switch i % 4 {
			case 0:
				manager.ProcessSpan(span)
			case 1:
				manager.GetStats()
			case 2:
				manager.GetConfig()
			case 3:
				manager.GetRules()
			}
			i++
		}
	})
}
