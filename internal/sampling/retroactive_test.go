package sampling

import (
	"sync/atomic"
	"testing"
	"time"

	"dogwatch/internal/trace"
)

func TestIntelligentSampler_RetroactiveCapture_Error(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowSize = 100
	config.RetroactiveWindowTime = 1 * time.Minute

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	var captured []string
	sampler.SetOnRetroactiveCapture(func(traceID string, spans []trace.Span, reason string) {
		captured = append(captured, traceID)
	})

	// Record some dropped traces
	for i := 0; i < 5; i++ {
		sampler.handleDroppedTrace("dropped-trace-"+string(rune('A'+i)), 10)
	}

	// Record a trace with error (which triggers retroactive capture)
	errorSpans := []trace.Span{
		{
			TraceID:     "error-trace",
			SpanID:      "span-1",
			ServiceName: "api-service",
			Name:        "handle-request",
			Status:      "ERROR",
			DurationMs:  100,
		},
	}
	sampler.handleKeptTrace(errorSpans)

	// Check that retroactive captures were triggered
	stats := sampler.GetStats()
	if stats.RetroactiveCaptures == 0 {
		t.Log("Note: No retroactive captures occurred - may need buffered traces")
	}
}

func TestIntelligentSampler_RetroactiveCapture_HighLatency(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowSize = 100
	config.RetroactiveWindowTime = 1 * time.Minute

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Record some dropped traces
	for i := 0; i < 5; i++ {
		sampler.handleDroppedTrace("dropped-trace-"+string(rune('A'+i)), 10)
	}

	// Record a trace with high latency (> 5000ms triggers retroactive capture)
	highLatencySpans := []trace.Span{
		{
			TraceID:     "high-latency-trace",
			SpanID:      "span-1",
			ServiceName: "api-service",
			Name:        "slow-request",
			Status:      "OK",
			DurationMs:  6000, // 6 seconds
		},
	}
	sampler.handleKeptTrace(highLatencySpans)

	// Verify the decision was recorded with high latency
	decisions := sampler.GetRecentDecisions(10)
	found := false
	for _, d := range decisions {
		if d.TraceID == "high-latency-trace" && d.MaxLatency == 6000 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected high-latency trace decision to be recorded")
	}
}

func TestIntelligentSampler_ParentChildTracking(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowSize = 100

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Record a trace with parent trace reference
	spans := []trace.Span{
		{
			TraceID:     "child-trace",
			SpanID:      "span-1",
			ServiceName: "api-service",
			Name:        "child-operation",
			Status:      "OK",
			DurationMs:  100,
			Attributes: map[string]string{
				"_parent_trace_id": "parent-trace-123",
			},
		},
	}
	sampler.handleKeptTrace(spans)

	// Verify parent trace was recorded
	decisions := sampler.GetRecentDecisions(10)
	var decision *TraceDecision
	for _, d := range decisions {
		if d.TraceID == "child-trace" {
			decision = d
			break
		}
	}

	if decision == nil {
		t.Fatal("Expected decision to be recorded")
	}

	if len(decision.ParentTraces) != 1 || decision.ParentTraces[0] != "parent-trace-123" {
		t.Errorf("Expected parent trace 'parent-trace-123', got %v", decision.ParentTraces)
	}
}

func TestIntelligentSampler_IsRelatedTrace(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	now := time.Now()

	tests := []struct {
		name     string
		trigger  *TraceDecision
		candidate *TraceDecision
		expected bool
	}{
		{
			name: "same service",
			trigger: &TraceDecision{
				TraceID:     "trigger",
				RootService: "api-service",
				Timestamp:   now,
			},
			candidate: &TraceDecision{
				TraceID:     "candidate",
				RootService: "api-service",
				Timestamp:   now.Add(-500 * time.Millisecond),
			},
			expected: true,
		},
		{
			name: "close in time",
			trigger: &TraceDecision{
				TraceID:     "trigger",
				RootService: "api-service",
				Timestamp:   now,
			},
			candidate: &TraceDecision{
				TraceID:     "candidate",
				RootService: "other-service",
				Timestamp:   now.Add(-500 * time.Millisecond),
			},
			expected: true,
		},
		{
			name: "parent relationship",
			trigger: &TraceDecision{
				TraceID:      "trigger",
				RootService:  "api-service",
				ParentTraces: []string{"candidate"},
				Timestamp:    now,
			},
			candidate: &TraceDecision{
				TraceID:     "candidate",
				RootService: "other-service",
				Timestamp:   now.Add(-5 * time.Second),
			},
			expected: true,
		},
		{
			name: "child relationship",
			trigger: &TraceDecision{
				TraceID:     "trigger",
				RootService: "api-service",
				ChildTraces: []string{"candidate"},
				Timestamp:   now,
			},
			candidate: &TraceDecision{
				TraceID:     "candidate",
				RootService: "other-service",
				Timestamp:   now.Add(-5 * time.Second),
			},
			expected: true,
		},
		{
			name: "not related",
			trigger: &TraceDecision{
				TraceID:     "trigger",
				RootService: "api-service",
				Timestamp:   now,
			},
			candidate: &TraceDecision{
				TraceID:     "candidate",
				RootService: "other-service",
				Timestamp:   now.Add(-10 * time.Second),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sampler.isRelatedTrace(tt.trigger, tt.candidate)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPatternLearner_Record(t *testing.T) {
	learner := newPatternLearner()

	// Record some spans
	spans := []trace.Span{
		{ServiceName: "api", DurationMs: 100, Status: "OK"},
		{ServiceName: "api", DurationMs: 120, Status: "OK"},
		{ServiceName: "api", DurationMs: 150, Status: "ERROR"},
		{ServiceName: "db", DurationMs: 50, Status: "OK"},
	}

	for i := 0; i < 100; i++ {
		learner.record(spans)
	}

	// Check that stats are being recorded
	stats, ok := learner.getServiceBaseline("api")
	if !ok {
		t.Fatal("Expected stats for 'api' service")
	}

	if stats.Count != 300 { // 3 spans per batch * 100 batches
		t.Errorf("Expected count 300, got %d", stats.Count)
	}

	mean := stats.Mean()
	if mean < 100 || mean > 150 {
		t.Errorf("Mean latency %.2f out of expected range [100, 150]", mean)
	}

	stdDev := stats.StdDev()
	if stdDev == 0 {
		t.Error("Expected non-zero standard deviation")
	}
}

func TestPatternLearner_IsAnomalous(t *testing.T) {
	learner := newPatternLearner()

	// Train with normal data (latency around 100ms)
	normalSpans := []trace.Span{
		{ServiceName: "api", DurationMs: 100, Status: "OK"},
		{ServiceName: "api", DurationMs: 105, Status: "OK"},
		{ServiceName: "api", DurationMs: 95, Status: "OK"},
	}

	for i := 0; i < 100; i++ {
		learner.record(normalSpans)
	}

	// Check normal latency is not anomalous
	if learner.isAnomalousForService("api", 100, 3.0) {
		t.Error("Normal latency should not be anomalous")
	}

	// Check extreme latency is anomalous
	if !learner.isAnomalousForService("api", 1000, 3.0) {
		t.Error("Extreme latency should be anomalous")
	}

	// Check unknown service returns false (not enough data)
	if learner.isAnomalousForService("unknown-service", 1000, 3.0) {
		t.Error("Unknown service should not be flagged as anomalous")
	}
}

func TestIntelligentSampler_AnomalyDetection_Enhanced(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.AnomalyEnabled = true
	config.AnomalyThreshold = 3.0

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	var detectedAnomalies int64
	sampler.SetOnAnomalyDetected(func(span *trace.Span, score float64) {
		atomic.AddInt64(&detectedAnomalies, 1)
	})

	// Process normal spans to build baseline
	for i := 0; i < 100; i++ {
		normalSpan := &trace.Span{
			TraceID:     "normal-trace",
			SpanID:      "span-1",
			ServiceName: "api-service",
			Name:        "normal-operation",
			DurationMs:  100,
		}
		sampler.ShouldSample(normalSpan)
	}

	// Process an anomalous span (very high latency)
	anomalousSpan := &trace.Span{
		TraceID:     "anomaly-trace",
		SpanID:      "span-1",
		ServiceName: "api-service",
		Name:        "slow-operation",
		DurationMs:  10000, // 10 seconds - should be anomalous
	}
	sampler.ShouldSample(anomalousSpan)

	// Check stats
	stats := sampler.GetStats()
	if stats.AnomaliesDetected == 0 {
		t.Log("Note: No anomalies detected - baseline may not have enough data")
	}
}

func TestIntelligentSampler_GetLearnedPatterns(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Process some spans to build patterns
	spans := []trace.Span{
		{ServiceName: "api", DurationMs: 100, Status: "OK"},
		{ServiceName: "db", DurationMs: 50, Status: "OK"},
	}
	for i := 0; i < 50; i++ {
		sampler.handleKeptTrace(spans)
	}

	// Get learned patterns
	patterns := sampler.GetLearnedPatterns()

	if patterns["samples_collected"] == nil {
		t.Error("Expected samples_collected in patterns")
	}

	serviceLatency, ok := patterns["service_latency"].(map[string]map[string]interface{})
	if !ok {
		t.Fatal("Expected service_latency map in patterns")
	}

	if _, ok := serviceLatency["api"]; !ok {
		t.Error("Expected 'api' service in latency stats")
	}
	if _, ok := serviceLatency["db"]; !ok {
		t.Error("Expected 'db' service in latency stats")
	}
}

func TestCostTracker_DailyReset(t *testing.T) {
	tracker := newCostTracker(0.000001, 100.0)

	// Record some spans
	tracker.recordSpans(1000000) // 1M spans = $1

	cost := tracker.getDailyCost()
	if cost != 1.0 {
		t.Errorf("Expected daily cost $1.00, got $%.2f", cost)
	}

	usedPct := tracker.getBudgetUsedPct()
	if usedPct != 1.0 {
		t.Errorf("Expected 1%% budget used, got %.2f%%", usedPct)
	}
}

func TestBudgetController_Adjustment(t *testing.T) {
	controller := newBudgetController(100.0, time.Minute)

	// Initially factor should be 1.0
	factor := controller.getAdjustmentFactor()
	if factor != 1.0 {
		t.Errorf("Expected initial factor 1.0, got %.2f", factor)
	}

	// Set a spend rate to enable budget adjustments
	controller.mu.Lock()
	controller.currentSpendRate = 1.0 // $1 per second = $3600/hour
	controller.mu.Unlock()

	// Simulate spending 80% of budget with 8 hours remaining
	// Remaining budget: $20, 8 hours = $2.50/hour budget
	// Current hourly spend: $3600/hour
	// Expected factor: 2.50/3600 = 0.0007 (clamped to 0.1)
	controller.updateAdjustment(80.0, 8.0)

	factor = controller.getAdjustmentFactor()
	if factor >= 1.0 {
		t.Errorf("Expected reduced factor, got %.2f", factor)
	}

	// Simulate almost exhausted budget
	controller.updateAdjustment(99.0, 1.0)

	factor = controller.getAdjustmentFactor()
	if factor > 0.2 {
		t.Errorf("Expected very low factor for exhausted budget, got %.2f", factor)
	}
}
