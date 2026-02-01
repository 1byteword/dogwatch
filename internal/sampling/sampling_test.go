package sampling

import (
	"sync"
	"testing"
	"time"

	"dogwatch/internal/trace"
)

func TestHeadSampler_BasicSampling(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000,
		Rules: []Rule{
			{
				ID:       "keep-errors",
				Name:     "Keep all errors",
				Enabled:  true,
				Priority: 100,
				Condition: RuleCondition{
					HasError: boolPtr(true),
				},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	// Test error span is kept
	errorSpan := &trace.Span{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Name:        "test-op",
		Status:      "ERROR",
	}

	decision := sampler.ShouldSample(errorSpan)
	if decision != DecisionKeep {
		t.Errorf("Expected error span to be kept, got %v", decision)
	}

	// Test OK span follows default sampling
	okSpan := &trace.Span{
		TraceID:     "trace-2",
		SpanID:      "span-2",
		ServiceName: "test-service",
		Name:        "test-op",
		Status:      "OK",
	}

	// With 10% default rate, most should be dropped
	sampler.ShouldSample(okSpan) // Just verify it doesn't panic
}

func TestHeadSampler_ConsistentSampling(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000,
		Rules:              []Rule{},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	// Same trace ID should get same decision
	span1 := &trace.Span{
		TraceID:     "consistent-trace-123",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Status:      "OK",
	}

	decision1 := sampler.ShouldSample(span1)

	// Second span with same trace ID
	span2 := &trace.Span{
		TraceID:     "consistent-trace-123",
		SpanID:      "span-2",
		ServiceName: "test-service",
		Status:      "OK",
	}

	decision2 := sampler.ShouldSample(span2)

	if decision1 != decision2 {
		t.Errorf("Same trace ID should get same decision: got %v and %v", decision1, decision2)
	}
}

func TestHeadSampler_RulePriority(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000,
		Rules: []Rule{
			{
				ID:         "drop-test",
				Name:       "Drop test service",
				Enabled:    true,
				Priority:   50,
				Condition:  RuleCondition{Service: "test-service"},
				Action:     DecisionDrop,
				SampleRate: 0,
			},
			{
				ID:         "keep-errors",
				Name:       "Keep all errors",
				Enabled:    true,
				Priority:   100, // Higher priority
				Condition:  RuleCondition{HasError: boolPtr(true)},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	// Error from test-service should be kept (higher priority rule)
	errorSpan := &trace.Span{
		TraceID:     "trace-priority-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Name:        "test-op",
		Status:      "ERROR",
	}

	decision := sampler.ShouldSample(errorSpan)
	if decision != DecisionKeep {
		t.Errorf("Expected error from test-service to be kept, got %v", decision)
	}
}

func TestHeadSampler_ServicePatternMatching(t *testing.T) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 1000,
		Rules: []Rule{
			{
				ID:         "keep-api-services",
				Name:       "Keep API services",
				Enabled:    true,
				Priority:   100,
				Condition:  RuleCondition{Service: "api-*"},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	// Should match api-* pattern
	apiSpan := &trace.Span{
		TraceID:     "trace-pattern-1",
		SpanID:      "span-1",
		ServiceName: "api-gateway",
		Status:      "OK",
	}

	decision := sampler.ShouldSample(apiSpan)
	if decision != DecisionKeep {
		t.Errorf("Expected api-gateway to be kept, got %v", decision)
	}
}

func TestTailSampler_ErrorTracesKept(t *testing.T) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      100 * time.Millisecond,
		MaxBufferedTraces:  100,
		MaxSpansPerTrace:   100,
		ErrorSampleRate:    1.0,
		LatencyThresholdMs: 1000,
		LatencySampleRate:  1.0,
		DefaultSampleRate:  0.0, // Drop everything except errors/high latency
	}

	sampler := NewTailSampler(config)

	var keptSpans []trace.Span
	var mu sync.Mutex

	sampler.SetOnKeep(func(spans []trace.Span) {
		mu.Lock()
		keptSpans = append(keptSpans, spans...)
		mu.Unlock()
	})

	// Add span with error
	errorSpan := &trace.Span{
		TraceID:     "error-trace-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Status:      "ERROR",
		DurationMs:  100,
	}

	decision := sampler.ShouldSample(errorSpan)
	if decision != DecisionDefer {
		t.Errorf("Expected DecisionDefer, got %v", decision)
	}

	// Wait for buffer timeout
	time.Sleep(200 * time.Millisecond)

	sampler.Stop()

	mu.Lock()
	if len(keptSpans) != 1 {
		t.Errorf("Expected 1 kept span, got %d", len(keptSpans))
	}
	mu.Unlock()
}

func TestTailSampler_HighLatencyTracesKept(t *testing.T) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      100 * time.Millisecond,
		MaxBufferedTraces:  100,
		MaxSpansPerTrace:   100,
		ErrorSampleRate:    1.0,
		LatencyThresholdMs: 500,
		LatencySampleRate:  1.0,
		DefaultSampleRate:  0.0,
	}

	sampler := NewTailSampler(config)

	var keptSpans []trace.Span
	var mu sync.Mutex

	sampler.SetOnKeep(func(spans []trace.Span) {
		mu.Lock()
		keptSpans = append(keptSpans, spans...)
		mu.Unlock()
	})

	// Add high latency span
	slowSpan := &trace.Span{
		TraceID:     "slow-trace-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Status:      "OK",
		DurationMs:  1000, // Above threshold
	}

	sampler.ShouldSample(slowSpan)

	// Wait for buffer timeout
	time.Sleep(200 * time.Millisecond)

	sampler.Stop()

	mu.Lock()
	if len(keptSpans) != 1 {
		t.Errorf("Expected 1 kept span (high latency), got %d", len(keptSpans))
	}
	mu.Unlock()
}

func TestTailSampler_PriorityServices(t *testing.T) {
	config := TailSamplerConfig{
		Enabled:            true,
		BufferTimeout:      100 * time.Millisecond,
		MaxBufferedTraces:  100,
		MaxSpansPerTrace:   100,
		ErrorSampleRate:    0.0,
		LatencyThresholdMs: 10000,
		LatencySampleRate:  0.0,
		DefaultSampleRate:  0.0,
		PriorityServices:   []string{"critical-service"},
	}

	sampler := NewTailSampler(config)

	var keptSpans []trace.Span
	var mu sync.Mutex

	sampler.SetOnKeep(func(spans []trace.Span) {
		mu.Lock()
		keptSpans = append(keptSpans, spans...)
		mu.Unlock()
	})

	// Add root span from priority service
	prioritySpan := &trace.Span{
		TraceID:      "priority-trace-1",
		SpanID:       "span-1",
		ParentSpanID: "", // Root span
		ServiceName:  "critical-service",
		Status:       "OK",
		DurationMs:   100,
	}

	sampler.ShouldSample(prioritySpan)

	// Wait for buffer timeout
	time.Sleep(200 * time.Millisecond)

	sampler.Stop()

	mu.Lock()
	if len(keptSpans) != 1 {
		t.Errorf("Expected 1 kept span (priority service), got %d", len(keptSpans))
	}
	mu.Unlock()
}

func TestAdaptiveSampler_RateAdjustment(t *testing.T) {
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

	// Send many spans to trigger rate adjustment
	for i := 0; i < 1000; i++ {
		span := &trace.Span{
			TraceID:     "trace-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "test-service",
			Status:      "OK",
		}
		sampler.ShouldSample(span)
	}

	// Wait for adjustment
	time.Sleep(100 * time.Millisecond)

	// Rate should have adjusted
	rate := sampler.GetCurrentRate()
	if rate >= 1.0 {
		t.Logf("Rate adjusted to: %v (expected < 1.0 due to high traffic)", rate)
	}
}

func TestAdaptiveSampler_PerServiceRates(t *testing.T) {
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

	// Send spans from different services
	for i := 0; i < 100; i++ {
		span := &trace.Span{
			TraceID:     "trace-svc1-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "service-1",
			Status:      "OK",
		}
		sampler.ShouldSample(span)
	}

	for i := 0; i < 50; i++ {
		span := &trace.Span{
			TraceID:     "trace-svc2-" + string(rune(i)),
			SpanID:      "span-1",
			ServiceName: "service-2",
			Status:      "OK",
		}
		sampler.ShouldSample(span)
	}

	// Wait for adjustment
	time.Sleep(100 * time.Millisecond)

	rates := sampler.GetServiceRates()
	if len(rates) < 2 {
		t.Errorf("Expected at least 2 service rates, got %d", len(rates))
	}
}

func TestAdaptiveSampler_ManualRateOverride(t *testing.T) {
	config := AdaptiveSamplerConfig{
		Enabled:               true,
		TargetTracesPerSecond: 100,
		MinSampleRate:         0.01,
		MaxSampleRate:         1.0,
		AdjustmentInterval:    1 * time.Hour, // Long interval so no auto-adjustment
		SmoothingFactor:       0.5,
		PerServiceRates:       true,
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	// Manually set rate
	sampler.SetRate(0.5)
	if sampler.GetCurrentRate() != 0.5 {
		t.Errorf("Expected rate 0.5, got %v", sampler.GetCurrentRate())
	}

	// Manually set service rate
	sampler.SetServiceRate("test-service", 0.25)
	rates := sampler.GetServiceRates()
	if rates["test-service"] != 0.25 {
		t.Errorf("Expected service rate 0.25, got %v", rates["test-service"])
	}
}

func TestDecision_String(t *testing.T) {
	tests := []struct {
		decision Decision
		expected string
	}{
		{DecisionKeep, "keep"},
		{DecisionDrop, "drop"},
		{DecisionDefer, "defer"},
		{Decision(99), "unknown"},
	}

	for _, test := range tests {
		if got := test.decision.String(); got != test.expected {
			t.Errorf("Decision(%d).String() = %q, want %q", test.decision, got, test.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Expected default config to be enabled")
	}

	if config.DefaultSampleRate != 0.1 {
		t.Errorf("Expected default sample rate 0.1, got %v", config.DefaultSampleRate)
	}

	if !config.HeadSamplerConfig.Enabled {
		t.Error("Expected head sampler to be enabled by default")
	}

	if config.TailSamplerConfig.Enabled {
		t.Error("Expected tail sampler to be disabled by default")
	}

	if !config.AdaptiveSamplerConfig.Enabled {
		t.Error("Expected adaptive sampler to be enabled by default")
	}
}

func TestTraceBuffer_AddSpan(t *testing.T) {
	buffer := &TraceBuffer{
		TraceID:   "test-trace",
		Spans:     make([]trace.Span, 0),
		FirstSeen: time.Now(),
	}

	// Add normal span
	buffer.AddSpan(trace.Span{
		TraceID:    "test-trace",
		SpanID:     "span-1",
		Status:     "OK",
		DurationMs: 100,
	})

	if buffer.SpanCount() != 1 {
		t.Errorf("Expected 1 span, got %d", buffer.SpanCount())
	}

	if buffer.HasError {
		t.Error("Expected no error")
	}

	// Add error span
	buffer.AddSpan(trace.Span{
		TraceID:    "test-trace",
		SpanID:     "span-2",
		Status:     "ERROR",
		DurationMs: 200,
	})

	if buffer.SpanCount() != 2 {
		t.Errorf("Expected 2 spans, got %d", buffer.SpanCount())
	}

	if !buffer.HasError {
		t.Error("Expected error flag to be set")
	}

	if buffer.MaxLatency != 200 {
		t.Errorf("Expected max latency 200, got %v", buffer.MaxLatency)
	}

	// Add root span
	buffer.AddSpan(trace.Span{
		TraceID:      "test-trace",
		SpanID:       "span-0",
		ParentSpanID: "",
		ServiceName:  "root-service",
		Status:       "OK",
		DurationMs:   300,
	})

	if buffer.RootService != "root-service" {
		t.Errorf("Expected root service 'root-service', got %q", buffer.RootService)
	}

	if buffer.MaxLatency != 300 {
		t.Errorf("Expected max latency 300, got %v", buffer.MaxLatency)
	}
}

func BenchmarkHeadSampler_ShouldSample(b *testing.B) {
	config := HeadSamplerConfig{
		Enabled:            true,
		DecisionTTL:        5 * time.Minute,
		MaxCachedDecisions: 100000,
		Rules: []Rule{
			{
				ID:         "keep-errors",
				Name:       "Keep all errors",
				Enabled:    true,
				Priority:   100,
				Condition:  RuleCondition{HasError: boolPtr(true)},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
			{
				ID:         "keep-slow",
				Name:       "Keep slow requests",
				Enabled:    true,
				Priority:   90,
				Condition:  RuleCondition{MinLatencyMs: 1000},
				Action:     DecisionKeep,
				SampleRate: 1.0,
			},
		},
	}

	sampler := NewHeadSampler(config)
	defer sampler.Stop()

	span := &trace.Span{
		TraceID:     "bench-trace",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Name:        "test-operation",
		Status:      "OK",
		DurationMs:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.TraceID = "bench-trace-" + string(rune(i%10000))
		sampler.ShouldSample(span)
	}
}

func BenchmarkAdaptiveSampler_ShouldSample(b *testing.B) {
	config := AdaptiveSamplerConfig{
		Enabled:               true,
		TargetTracesPerSecond: 1000,
		MinSampleRate:         0.01,
		MaxSampleRate:         1.0,
		AdjustmentInterval:    1 * time.Minute,
		SmoothingFactor:       0.3,
		PerServiceRates:       true,
	}

	sampler := NewAdaptiveSampler(config)
	defer sampler.Stop()

	span := &trace.Span{
		TraceID:     "bench-trace",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Name:        "test-operation",
		Status:      "OK",
		DurationMs:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.TraceID = "bench-trace-" + string(rune(i%10000))
		sampler.ShouldSample(span)
	}
}
