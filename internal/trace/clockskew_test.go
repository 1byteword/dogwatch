package trace

import (
	"testing"
	"time"
)

func TestClockSkewManager_ProcessSpanPair(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.MinSamplesForDetection = 2
	manager := NewClockSkewManager(config)

	// Create parent and child spans with some skew
	now := time.Now()
	parent := Span{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "service-a",
		Name:        "parent",
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
	}

	// Child starts 10ms after parent (normal case)
	child := Span{
		TraceID:      "trace-1",
		SpanID:       "span-2",
		ParentSpanID: "span-1",
		ServiceName:  "service-b",
		Name:         "child",
		StartTime:    now.Add(10 * time.Millisecond),
		EndTime:      now.Add(50 * time.Millisecond),
	}

	manager.ProcessSpanPair(parent, child)
	manager.ProcessSpanPair(parent, child) // Second sample to trigger stats

	stats := manager.GetStats()
	if stats.TotalSpansProcessed != 2 {
		t.Errorf("Expected 2 spans processed, got %d", stats.TotalSpansProcessed)
	}
}

func TestClockSkewManager_DetectViolation(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.MinSamplesForDetection = 1
	manager := NewClockSkewManager(config)

	now := time.Now()
	parent := Span{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "service-a",
		Name:        "parent",
		StartTime:   now,
		EndTime:     now.Add(100 * time.Millisecond),
	}

	// Child starts BEFORE parent (violation)
	child := Span{
		TraceID:      "trace-1",
		SpanID:       "span-2",
		ParentSpanID: "span-1",
		ServiceName:  "service-b",
		Name:         "child",
		StartTime:    now.Add(-10 * time.Millisecond), // Before parent!
		EndTime:      now.Add(50 * time.Millisecond),
	}

	manager.ProcessSpanPair(parent, child)

	stats := manager.GetStats()
	if stats.TotalViolationsFound != 1 {
		t.Errorf("Expected 1 violation found, got %d", stats.TotalViolationsFound)
	}
}

func TestClockSkewManager_CorrectSpan(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.EnableAutoCorrection = true
	manager := NewClockSkewManager(config)

	parentStart := time.Now()

	// Child starts before parent (needs correction)
	span := Span{
		TraceID:      "trace-1",
		SpanID:       "span-2",
		ParentSpanID: "span-1",
		ServiceName:  "service-b",
		Name:         "child",
		StartTime:    parentStart.Add(-50 * time.Millisecond),
		EndTime:      parentStart.Add(50 * time.Millisecond),
	}

	confidence, corrected := manager.CorrectSpan(&span, parentStart)

	if !corrected {
		t.Error("Expected span to be corrected")
	}

	if confidence == nil {
		t.Fatal("Expected confidence info")
	}

	if !confidence.CorrectionApplied {
		t.Error("Expected correction to be applied")
	}

	// Span should now start at parent start time
	if !span.StartTime.Equal(parentStart) {
		t.Errorf("Expected span start time %v, got %v", parentStart, span.StartTime)
	}
}

func TestClockSkewManager_ManualCorrection(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	manager.SetCorrection("service-a", 100*time.Millisecond)

	corrections := manager.GetServiceCorrections()
	if corrections["service-a"] != 100*time.Millisecond {
		t.Errorf("Expected correction 100ms, got %v", corrections["service-a"])
	}

	manager.ClearCorrection("service-a")

	corrections = manager.GetServiceCorrections()
	if _, exists := corrections["service-a"]; exists {
		t.Error("Expected correction to be cleared")
	}
}

func TestClockSkewManager_NTPDrift(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	// Record some drift samples
	manager.RecordNTPDrift("service-a", 10*time.Millisecond)
	manager.RecordNTPDrift("service-a", 15*time.Millisecond)
	manager.RecordNTPDrift("service-a", 12*time.Millisecond)

	stats := manager.GetNTPDriftStats("service-a")
	if stats == nil {
		t.Fatal("Expected NTP drift stats")
	}

	if stats.SampleCount != 3 {
		t.Errorf("Expected 3 samples, got %d", stats.SampleCount)
	}

	// Mean should be around 12.33ms
	expectedMean := 12 * time.Millisecond
	if stats.MeanDrift < expectedMean-time.Millisecond || stats.MeanDrift > expectedMean+2*time.Millisecond {
		t.Errorf("Expected mean drift around 12ms, got %v", stats.MeanDrift)
	}
}

func TestClockSkewManager_GetAllSkewStats(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.MinSamplesForDetection = 2
	manager := NewClockSkewManager(config)

	now := time.Now()

	// Create spans for two different service pairs
	for i := 0; i < 3; i++ {
		parent := Span{
			TraceID:     "trace-1",
			SpanID:      "span-p",
			ServiceName: "service-a",
			StartTime:   now.Add(time.Duration(i) * time.Second),
		}
		child := Span{
			TraceID:      "trace-1",
			SpanID:       "span-c",
			ParentSpanID: "span-p",
			ServiceName:  "service-b",
			StartTime:    now.Add(time.Duration(i)*time.Second + 10*time.Millisecond),
		}
		manager.ProcessSpanPair(parent, child)
	}

	allStats := manager.GetAllSkewStats()
	if len(allStats) != 1 {
		t.Errorf("Expected 1 service pair stats, got %d", len(allStats))
	}
}

func TestClockSkewManager_GenerateReport(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())
	manager.Start()
	defer manager.Stop()

	// Record some data
	manager.RecordNTPDrift("service-a", 10*time.Millisecond)
	manager.SetCorrection("service-b", 20*time.Millisecond)

	report := manager.GenerateReport()
	if report == nil {
		t.Fatal("Expected report")
	}

	if report.GeneratedAt.IsZero() {
		t.Error("Expected generated timestamp")
	}

	if len(report.ActiveCorrections) != 1 {
		t.Errorf("Expected 1 active correction, got %d", len(report.ActiveCorrections))
	}
}

func TestClockSkewManager_UpdateConfig(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	newConfig := DefaultClockSkewConfig()
	newConfig.MaxSkewTolerance = 10 * time.Second
	newConfig.EnableAutoCorrection = false

	manager.UpdateConfig(newConfig)

	config := manager.GetConfig()
	if config.MaxSkewTolerance != 10*time.Second {
		t.Errorf("Expected max skew tolerance 10s, got %v", config.MaxSkewTolerance)
	}

	if config.EnableAutoCorrection {
		t.Error("Expected auto correction to be disabled")
	}
}

func TestDefaultClockSkewConfig(t *testing.T) {
	config := DefaultClockSkewConfig()

	if config.MaxSkewTolerance != 5*time.Second {
		t.Errorf("Expected max skew tolerance 5s, got %v", config.MaxSkewTolerance)
	}

	if config.DetectionWindow != 5*time.Minute {
		t.Errorf("Expected detection window 5m, got %v", config.DetectionWindow)
	}

	if !config.EnableAutoCorrection {
		t.Error("Expected auto correction enabled by default")
	}
}

func TestServicePair(t *testing.T) {
	pair1 := ServicePair{Source: "a", Target: "b"}
	pair2 := ServicePair{Source: "a", Target: "b"}
	pair3 := ServicePair{Source: "b", Target: "a"}

	if pair1 != pair2 {
		t.Error("Expected equal pairs to be equal")
	}

	if pair1 == pair3 {
		t.Error("Expected different pairs to be unequal")
	}
}

func BenchmarkClockSkewManager_ProcessSpanPair(b *testing.B) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	now := time.Now()
	parent := Span{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		ServiceName: "service-a",
		StartTime:   now,
	}
	child := Span{
		TraceID:      "trace-1",
		SpanID:       "span-2",
		ParentSpanID: "span-1",
		ServiceName:  "service-b",
		StartTime:    now.Add(10 * time.Millisecond),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.ProcessSpanPair(parent, child)
	}
}

func BenchmarkClockSkewManager_CorrectSpan(b *testing.B) {
	config := DefaultClockSkewConfig()
	config.EnableAutoCorrection = true
	manager := NewClockSkewManager(config)
	manager.SetCorrection("service-b", 10*time.Millisecond)

	now := time.Now()
	parentStart := now

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := Span{
			TraceID:     "trace-1",
			SpanID:      "span-2",
			ServiceName: "service-b",
			StartTime:   now.Add(-5 * time.Millisecond),
			EndTime:     now.Add(50 * time.Millisecond),
		}
		manager.CorrectSpan(&span, parentStart)
	}
}
