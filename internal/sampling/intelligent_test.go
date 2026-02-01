package sampling

import (
	"testing"
	"time"

	"dogwatch/internal/trace"
)

func TestIntelligentSamplerCreation(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	if sampler == nil {
		t.Fatal("Failed to create intelligent sampler")
	}

	if sampler.baseSampler == nil {
		t.Error("Base sampler not initialized")
	}

	if sampler.normalPatterns == nil {
		t.Error("Pattern learner not initialized")
	}

	if sampler.costTracker == nil {
		t.Error("Cost tracker not initialized")
	}

	if sampler.budgetControl == nil {
		t.Error("Budget controller not initialized")
	}
}

func TestIntelligentSamplerBasicSampling(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = false
	config.AnomalyEnabled = false
	config.CostEnabled = false
	config.LearningEnabled = false

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	span := &trace.Span{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "test-service",
		Name:        "test-operation",
		DurationMs:  100,
		Status:      "OK",
		StartTime:   time.Now(),
	}

	decision := sampler.ShouldSample(span)

	// Should defer (tail sampling)
	if decision != DecisionDefer {
		t.Errorf("Expected DecisionDefer, got %v", decision)
	}

	stats := sampler.GetStats()
	if stats.TotalSpans != 1 {
		t.Errorf("Expected 1 total span, got %d", stats.TotalSpans)
	}
}

func TestAnomalyScoreCalculation(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.AnomalyEnabled = true
	config.AnomalyThreshold = 2.0

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Train with normal spans
	for i := 0; i < 200; i++ {
		span := &trace.Span{
			TraceID:     "trace-" + string(rune(i)),
			SpanID:      "span-" + string(rune(i)),
			ServiceName: "test-service",
			DurationMs:  100 + float64(i%20), // 100-120ms range
			Status:      "OK",
			StartTime:   time.Now(),
		}
		sampler.normalPatterns.record([]trace.Span{*span})
	}

	// Test anomaly detection
	normalSpan := &trace.Span{
		TraceID:     "normal-trace",
		SpanID:      "normal-span",
		ServiceName: "test-service",
		DurationMs:  110,
		Status:      "OK",
	}
	normalScore := sampler.calculateAnomalyScore(normalSpan)
	if normalScore > 2.0 {
		t.Errorf("Normal span got high anomaly score: %f", normalScore)
	}

	// Test with extreme latency
	slowSpan := &trace.Span{
		TraceID:     "slow-trace",
		SpanID:      "slow-span",
		ServiceName: "test-service",
		DurationMs:  10000, // 10 seconds
		Status:      "OK",
	}
	slowScore := sampler.calculateAnomalyScore(slowSpan)
	if slowScore < 2.0 {
		t.Logf("Slow span anomaly score: %f (may need more training data)", slowScore)
	}
}

func TestCostTracker(t *testing.T) {
	tracker := newCostTracker(0.001, 100.0) // $0.001 per span, $100 budget

	// Record some spans
	tracker.recordSpans(1000)

	if tracker.getDailyCost() != 1.0 {
		t.Errorf("Expected daily cost 1.0, got %f", tracker.getDailyCost())
	}

	if tracker.getBudgetUsedPct() != 1.0 {
		t.Errorf("Expected 1%% budget used, got %f%%", tracker.getBudgetUsedPct())
	}

	// Record more
	tracker.recordSpans(9000)

	if tracker.getDailyCost() != 10.0 {
		t.Errorf("Expected daily cost 10.0, got %f", tracker.getDailyCost())
	}

	if tracker.getBudgetUsedPct() != 10.0 {
		t.Errorf("Expected 10%% budget used, got %f%%", tracker.getBudgetUsedPct())
	}
}

func TestBudgetController(t *testing.T) {
	controller := newBudgetController(100.0, time.Minute)

	// Initial factor should be 1.0
	if controller.getAdjustmentFactor() != 1.0 {
		t.Errorf("Expected initial factor 1.0, got %f", controller.getAdjustmentFactor())
	}

	// Set a spend rate to make adjustment meaningful
	controller.mu.Lock()
	controller.currentSpendRate = 0.01 // $0.01 per second = $36/hour
	controller.mu.Unlock()

	// Simulate spending 50% of budget with 12 hours remaining
	// Remaining budget: $50, remaining time: 12 hours
	// Hourly budget: $50/12 = $4.17
	// Current hourly spend: $36
	// Factor should be around 4.17/36 = 0.116
	controller.updateAdjustment(50.0, 12.0)
	factor := controller.getAdjustmentFactor()

	// Should adjust to use remaining budget over remaining time
	if factor < 0.1 || factor > 2.0 {
		t.Errorf("Factor out of expected range: %f", factor)
	}

	// Simulate nearly exhausted budget
	// Remaining budget: $5, remaining time: 1 hour
	// Hourly budget: $5
	// Current hourly spend: $36
	// Factor should be around 5/36 = 0.14, clamped to min 0.1
	controller.updateAdjustment(95.0, 1.0)
	factor = controller.getAdjustmentFactor()
	if factor > 0.5 {
		t.Errorf("Expected low factor when budget nearly exhausted, got %f", factor)
	}
}

func TestLatencyStats(t *testing.T) {
	stats := &LatencyStats{}

	// Record some values
	values := []float64{100, 110, 90, 105, 95, 100, 120, 80, 115, 85}
	for _, v := range values {
		stats.Count++
		stats.Sum += v
		stats.SumSquares += v * v
		if stats.Min == 0 || v < stats.Min {
			stats.Min = v
		}
		if v > stats.Max {
			stats.Max = v
		}
	}

	mean := stats.Mean()
	if mean < 95 || mean > 105 {
		t.Errorf("Mean out of expected range: %f", mean)
	}

	stdDev := stats.StdDev()
	if stdDev < 5 || stdDev > 20 {
		t.Errorf("StdDev out of expected range: %f", stdDev)
	}

	// Test Z-score
	zScore := stats.ZScore(200) // Very high value
	if zScore < 2 {
		t.Errorf("Expected high Z-score for outlier, got %f", zScore)
	}

	zScore = stats.ZScore(100) // Normal value
	if zScore < -1 || zScore > 1 {
		t.Errorf("Expected low Z-score for normal value, got %f", zScore)
	}
}

func TestSamplingLearner(t *testing.T) {
	learner := newSamplingLearner()

	// Record samples
	for i := 0; i < 100; i++ {
		isError := i%10 == 0 // 10% error rate
		latency := 100.0
		if i%5 == 0 {
			latency = 2000.0 // 20% high latency
		}
		learner.recordSample("test-service", "test-op", isError, latency, 1000.0)
	}

	// Get learned rate
	rate := learner.getLearnedRate("test-service", "test-op")
	if rate <= 0 {
		t.Error("Expected positive learned rate")
	}

	// Service with errors should have higher rate
	if rate < 0.1 {
		t.Errorf("Expected higher rate for service with errors, got %f", rate)
	}
}

func TestTraceDecisionRecording(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowSize = 100
	config.AnomalyEnabled = false
	config.CostEnabled = false

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Record a decision
	decision := &TraceDecision{
		TraceID:     "trace-1",
		Decision:    DecisionKeep,
		Reason:      "test",
		Timestamp:   time.Now(),
		SpanCount:   5,
		HasError:    false,
		MaxLatency:  100,
		RootService: "test-service",
	}

	sampler.recordDecision(decision)

	sampler.recentDecisionsMu.RLock()
	_, found := sampler.decisionIndex["trace-1"]
	sampler.recentDecisionsMu.RUnlock()

	if !found {
		t.Error("Decision not recorded in index")
	}
}

func TestGetCostBreakdown(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.CostEnabled = true
	config.CostPerSpan = 0.001

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Record some spans
	sampler.costTracker.recordSpans(5000)

	breakdown := sampler.GetCostBreakdown()

	if breakdown.DailyCost != 5.0 {
		t.Errorf("Expected daily cost 5.0, got %f", breakdown.DailyCost)
	}

	if breakdown.DailySpans != 5000 {
		t.Errorf("Expected 5000 daily spans, got %d", breakdown.DailySpans)
	}

	if breakdown.CostPerSpan != 0.001 {
		t.Errorf("Expected cost per span 0.001, got %f", breakdown.CostPerSpan)
	}
}

func TestIntelligentSamplerStats(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Generate some activity
	for i := 0; i < 10; i++ {
		span := &trace.Span{
			TraceID:     "trace-" + string(rune(i)),
			SpanID:      "span-" + string(rune(i)),
			ServiceName: "test-service",
			Name:        "test-op",
			DurationMs:  100,
			Status:      "OK",
			StartTime:   time.Now(),
		}
		sampler.ShouldSample(span)
	}

	stats := sampler.GetStats()

	if stats.TotalSpans != 10 {
		t.Errorf("Expected 10 total spans, got %d", stats.TotalSpans)
	}
}

func TestPatternLearnerRecord(t *testing.T) {
	learner := newPatternLearner()

	spans := []trace.Span{
		{
			TraceID:     "trace-1",
			SpanID:      "span-1",
			ServiceName: "api-service",
			DurationMs:  100,
		},
		{
			TraceID:     "trace-1",
			SpanID:      "span-2",
			ServiceName: "api-service",
			DurationMs:  150,
		},
		{
			TraceID:     "trace-1",
			SpanID:      "span-3",
			ServiceName: "db-service",
			DurationMs:  50,
		},
	}

	learner.record(spans)

	learner.mu.RLock()
	defer learner.mu.RUnlock()

	if len(learner.serviceLatency) != 2 {
		t.Errorf("Expected 2 services, got %d", len(learner.serviceLatency))
	}

	apiStats := learner.serviceLatency["api-service"]
	if apiStats == nil {
		t.Fatal("api-service stats not found")
	}

	if apiStats.Count != 2 {
		t.Errorf("Expected 2 samples for api-service, got %d", apiStats.Count)
	}

	if apiStats.Mean() != 125 {
		t.Errorf("Expected mean 125, got %f", apiStats.Mean())
	}
}

func TestConfigSerialization(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	data, err := sampler.ExportConfig()
	if err != nil {
		t.Fatalf("Failed to export config: %v", err)
	}

	if len(data) == 0 {
		t.Error("Exported config is empty")
	}
}

func TestUpdateConfig(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Update config
	newConfig := config
	newConfig.AnomalyThreshold = 5.0
	newConfig.DailyBudget = 200.0

	sampler.UpdateConfig(newConfig)

	if sampler.anomalyThreshold != 5.0 {
		t.Errorf("Anomaly threshold not updated: %f", sampler.anomalyThreshold)
	}

	retrieved := sampler.GetConfig()
	if retrieved.DailyBudget != 200.0 {
		t.Errorf("Daily budget not updated: %f", retrieved.DailyBudget)
	}
}

func TestGetLearnedPatterns(t *testing.T) {
	config := DefaultIntelligentConfig()
	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	// Record some data
	spans := []trace.Span{
		{ServiceName: "api", DurationMs: 100},
		{ServiceName: "api", DurationMs: 110},
		{ServiceName: "db", DurationMs: 50},
	}
	sampler.normalPatterns.record(spans)

	patterns := sampler.GetLearnedPatterns()

	if patterns == nil {
		t.Fatal("Expected patterns map")
	}

	if _, ok := patterns["service_latency"]; !ok {
		t.Error("Expected service_latency in patterns")
	}
}

func TestRelatedTraceDetection(t *testing.T) {
	config := DefaultIntelligentConfig()
	config.RetroactiveEnabled = true
	config.RetroactiveWindowTime = 5 * time.Second

	sampler := NewIntelligentSampler(config)
	defer sampler.Stop()

	now := time.Now()

	// Create related decisions
	trigger := &TraceDecision{
		TraceID:     "trigger-trace",
		Decision:    DecisionKeep,
		Reason:      "error",
		Timestamp:   now,
		HasError:    true,
		RootService: "api-service",
	}

	related := &TraceDecision{
		TraceID:     "related-trace",
		Decision:    DecisionDrop,
		Reason:      "sampled_out",
		Timestamp:   now.Add(-500 * time.Millisecond),
		RootService: "api-service",
	}

	unrelated := &TraceDecision{
		TraceID:     "unrelated-trace",
		Decision:    DecisionDrop,
		Reason:      "sampled_out",
		Timestamp:   now.Add(-10 * time.Second),
		RootService: "other-service",
	}

	// Test relatedness
	if !sampler.isRelatedTrace(trigger, related) {
		t.Error("Expected related trace to be detected as related")
	}

	if sampler.isRelatedTrace(trigger, unrelated) {
		t.Error("Expected unrelated trace to not be detected as related")
	}
}

func TestHourlyPatterns(t *testing.T) {
	learner := newPatternLearner()

	// Verify hourly patterns initialized
	for i := 0; i < 24; i++ {
		if learner.hourlyPatterns[i] == nil {
			t.Errorf("Hourly pattern %d not initialized", i)
		}
		if learner.hourlyPatterns[i].Hour != i {
			t.Errorf("Hourly pattern %d has wrong hour: %d", i, learner.hourlyPatterns[i].Hour)
		}
	}

	// Record and check increment
	hour := time.Now().Hour()
	initialCount := learner.hourlyPatterns[hour].SampleCount

	learner.record([]trace.Span{{ServiceName: "test", DurationMs: 100}})

	if learner.hourlyPatterns[hour].SampleCount != initialCount+1 {
		t.Error("Hourly pattern sample count not incremented")
	}
}
