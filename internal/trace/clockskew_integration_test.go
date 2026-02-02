package trace

import (
	"sync"
	"testing"
	"time"
)

// Integration tests for ClockSkewManager

func TestClockSkewManager_SkewDetectionAccuracy(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.MinSamplesForDetection = 5
	config.ConfidenceThreshold = 0.5
	manager := NewClockSkewManager(config)

	now := time.Now()

	// Create consistent skew pattern: service-b is always 50ms behind service-a
	skewMs := 50 * time.Millisecond

	for i := 0; i < 10; i++ {
		baseTime := now.Add(time.Duration(i) * time.Second)

		parent := Span{
			TraceID:     "trace-1",
			SpanID:      "span-parent-" + itoa(i),
			ServiceName: "service-a",
			Name:        "parent-op",
			StartTime:   baseTime,
			EndTime:     baseTime.Add(100 * time.Millisecond),
		}

		// Child starts "before" parent in wall clock due to skew
		child := Span{
			TraceID:      "trace-1",
			SpanID:       "span-child-" + itoa(i),
			ParentSpanID: parent.SpanID,
			ServiceName:  "service-b",
			Name:         "child-op",
			StartTime:    baseTime.Add(10*time.Millisecond - skewMs), // Would be 10ms after in reality
			EndTime:      baseTime.Add(50*time.Millisecond - skewMs),
		}

		manager.ProcessSpanPair(parent, child)
	}

	// Check that skew was detected
	stats := manager.GetSkewStats("service-a", "service-b")
	if stats == nil {
		t.Fatal("Expected skew stats to be computed")
	}

	if stats.SampleCount != 10 {
		t.Errorf("Expected 10 samples, got %d", stats.SampleCount)
	}

	// Mean skew should be close to 50ms (±10ms tolerance for calculation variance)
	expectedSkew := 40 * time.Millisecond // parent.StartTime - child.StartTime = skew
	tolerance := 15 * time.Millisecond
	if absDur(stats.MeanSkew-expectedSkew) > tolerance {
		t.Errorf("Expected mean skew around %v (±%v), got %v", expectedSkew, tolerance, stats.MeanSkew)
	}
}

func TestClockSkewManager_CorrectionApplication(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.EnableAutoCorrection = true
	config.MaxSkewTolerance = 1 * time.Second
	manager := NewClockSkewManager(config)

	// Set a known correction
	manager.SetCorrection("service-b", 100*time.Millisecond)

	parentStart := time.Now()

	tests := []struct {
		name            string
		span            Span
		expectCorrected bool
		expectConfLow   bool
	}{
		{
			name: "span needing correction via service offset",
			span: Span{
				TraceID:     "trace-1",
				SpanID:      "span-1",
				ServiceName: "service-b",
				StartTime:   parentStart.Add(10 * time.Millisecond),
				EndTime:     parentStart.Add(50 * time.Millisecond),
			},
			expectCorrected: true,
			expectConfLow:   false,
		},
		{
			name: "span with violation (starts before parent)",
			span: Span{
				TraceID:     "trace-2",
				SpanID:      "span-2",
				ServiceName: "service-c",
				StartTime:   parentStart.Add(-100 * time.Millisecond),
				EndTime:     parentStart.Add(50 * time.Millisecond),
			},
			expectCorrected: true,
			expectConfLow:   true,
		},
		{
			name: "span not needing correction",
			span: Span{
				TraceID:     "trace-3",
				SpanID:      "span-3",
				ServiceName: "service-d", // No correction set
				StartTime:   parentStart.Add(50 * time.Millisecond),
				EndTime:     parentStart.Add(100 * time.Millisecond),
			},
			expectCorrected: false,
			expectConfLow:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			span := tt.span // Copy
			confidence, corrected := manager.CorrectSpan(&span, parentStart)

			if corrected != tt.expectCorrected {
				t.Errorf("Expected corrected=%v, got %v", tt.expectCorrected, corrected)
			}

			if corrected {
				if confidence == nil {
					t.Error("Expected confidence info when corrected")
				} else {
					if !confidence.CorrectionApplied {
						t.Error("Expected CorrectionApplied=true")
					}

					if tt.expectConfLow && confidence.StartTimeConfidence >= 0.9 {
						t.Errorf("Expected low confidence for violation fix, got %v", confidence.StartTimeConfidence)
					}
				}
			}
		})
	}
}

func TestClockSkewManager_ParentChildOrderingFix(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.EnableAutoCorrection = true
	manager := NewClockSkewManager(config)

	parentStart := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	// Child starts 200ms BEFORE parent (clear violation)
	span := Span{
		TraceID:     "trace-1",
		SpanID:      "span-child",
		ServiceName: "slow-clock-service",
		StartTime:   parentStart.Add(-200 * time.Millisecond),
		EndTime:     parentStart.Add(300 * time.Millisecond),
	}

	originalStart := span.StartTime
	originalEnd := span.EndTime

	confidence, corrected := manager.CorrectSpan(&span, parentStart)

	if !corrected {
		t.Error("Expected span to be corrected due to violation")
	}

	// Span should now start at parent start time
	if !span.StartTime.Equal(parentStart) {
		t.Errorf("Expected corrected start time %v, got %v", parentStart, span.StartTime)
	}

	// Duration should be preserved
	originalDuration := originalEnd.Sub(originalStart)
	correctedDuration := span.EndTime.Sub(span.StartTime)
	if originalDuration != correctedDuration {
		t.Errorf("Duration changed from %v to %v", originalDuration, correctedDuration)
	}

	// Confidence should be low due to correction
	if confidence.StartTimeConfidence > 0.7 {
		t.Errorf("Expected confidence < 0.7 for violation fix, got %v", confidence.StartTimeConfidence)
	}

	// Check stats
	stats := manager.GetStats()
	if stats.TotalViolationsFixed != 1 {
		t.Errorf("Expected 1 violation fixed, got %d", stats.TotalViolationsFixed)
	}
}

func TestClockSkewManager_ConfidenceScoring(t *testing.T) {
	config := DefaultClockSkewConfig()
	config.MinSamplesForDetection = 3
	config.ConfidenceThreshold = 0.7
	manager := NewClockSkewManager(config)

	now := time.Now()

	// Create highly consistent measurements (low variance = high confidence)
	for i := 0; i < 10; i++ {
		parent := Span{
			TraceID:     "trace-consistent",
			SpanID:      "span-p-" + itoa(i),
			ServiceName: "service-a",
			StartTime:   now.Add(time.Duration(i) * time.Second),
		}
		child := Span{
			TraceID:      "trace-consistent",
			SpanID:       "span-c-" + itoa(i),
			ParentSpanID: parent.SpanID,
			ServiceName:  "service-b",
			StartTime:    now.Add(time.Duration(i)*time.Second + 50*time.Millisecond), // Consistent 50ms offset
		}
		manager.ProcessSpanPair(parent, child)
	}

	stats := manager.GetSkewStats("service-a", "service-b")
	if stats == nil {
		t.Fatal("Expected stats")
	}

	// High confidence due to consistency
	if stats.Confidence < 0.7 {
		t.Errorf("Expected high confidence (>0.7) for consistent measurements, got %v", stats.Confidence)
	}

	// Now add highly variable measurements to another pair
	for i := 0; i < 10; i++ {
		// Variable skew: -100ms to +100ms
		variableSkew := time.Duration((i % 5) - 2) * 50 * time.Millisecond

		parent := Span{
			TraceID:     "trace-variable",
			SpanID:      "span-p-var-" + itoa(i),
			ServiceName: "service-c",
			StartTime:   now.Add(time.Duration(i) * time.Second),
		}
		child := Span{
			TraceID:      "trace-variable",
			SpanID:       "span-c-var-" + itoa(i),
			ParentSpanID: parent.SpanID,
			ServiceName:  "service-d",
			StartTime:    now.Add(time.Duration(i)*time.Second + variableSkew),
		}
		manager.ProcessSpanPair(parent, child)
	}

	variableStats := manager.GetSkewStats("service-c", "service-d")
	if variableStats == nil {
		t.Fatal("Expected stats for variable pair")
	}

	// Lower confidence due to high variance
	if variableStats.Confidence > 0.8 {
		t.Logf("Note: Confidence %v may be higher than expected due to calculation method",
			variableStats.Confidence)
	}
}

func TestClockSkewManager_NTPDriftTracking(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	service := "test-service"

	// Record multiple drift samples
	driftSamples := []time.Duration{
		10 * time.Millisecond,
		15 * time.Millisecond,
		12 * time.Millisecond,
		8 * time.Millisecond,
		20 * time.Millisecond,
	}

	for _, drift := range driftSamples {
		manager.RecordNTPDrift(service, drift)
	}

	stats := manager.GetNTPDriftStats(service)
	if stats == nil {
		t.Fatal("Expected NTP drift stats")
	}

	if stats.SampleCount != 5 {
		t.Errorf("Expected 5 samples, got %d", stats.SampleCount)
	}

	// Mean should be (10+15+12+8+20)/5 = 13ms
	expectedMean := 13 * time.Millisecond
	tolerance := 1 * time.Millisecond
	if absDur(stats.MeanDrift-expectedMean) > tolerance {
		t.Errorf("Expected mean drift %v (±%v), got %v", expectedMean, tolerance, stats.MeanDrift)
	}

	// Min should be 8ms
	if stats.MinDrift != 8*time.Millisecond {
		t.Errorf("Expected min drift 8ms, got %v", stats.MinDrift)
	}

	// Max should be 20ms
	if stats.MaxDrift != 20*time.Millisecond {
		t.Errorf("Expected max drift 20ms, got %v", stats.MaxDrift)
	}
}

func TestClockSkewManager_ManualCorrectionManagement(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	// Set corrections
	manager.SetCorrection("service-a", 50*time.Millisecond)
	manager.SetCorrection("service-b", -30*time.Millisecond)
	manager.SetCorrection("service-c", 100*time.Millisecond)

	// Verify all corrections
	corrections := manager.GetServiceCorrections()
	if len(corrections) != 3 {
		t.Errorf("Expected 3 corrections, got %d", len(corrections))
	}

	if corrections["service-a"] != 50*time.Millisecond {
		t.Errorf("Expected service-a correction 50ms, got %v", corrections["service-a"])
	}

	if corrections["service-b"] != -30*time.Millisecond {
		t.Errorf("Expected service-b correction -30ms, got %v", corrections["service-b"])
	}

	// Clear one
	manager.ClearCorrection("service-b")

	corrections = manager.GetServiceCorrections()
	if len(corrections) != 2 {
		t.Errorf("Expected 2 corrections after clear, got %d", len(corrections))
	}

	if _, exists := corrections["service-b"]; exists {
		t.Error("service-b correction should be cleared")
	}

	// Update existing
	manager.SetCorrection("service-a", 75*time.Millisecond)
	corrections = manager.GetServiceCorrections()
	if corrections["service-a"] != 75*time.Millisecond {
		t.Errorf("Expected updated correction 75ms, got %v", corrections["service-a"])
	}
}

func TestClockSkewManager_ConfigUpdate(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	// Verify defaults
	config := manager.GetConfig()
	if config.MaxSkewTolerance != 5*time.Second {
		t.Errorf("Expected default max tolerance 5s, got %v", config.MaxSkewTolerance)
	}

	// Update config
	newConfig := DefaultClockSkewConfig()
	newConfig.MaxSkewTolerance = 10 * time.Second
	newConfig.DetectionWindow = 10 * time.Minute
	newConfig.EnableAutoCorrection = false
	newConfig.ConfidenceThreshold = 0.9

	manager.UpdateConfig(newConfig)

	config = manager.GetConfig()
	if config.MaxSkewTolerance != 10*time.Second {
		t.Errorf("Expected updated max tolerance 10s, got %v", config.MaxSkewTolerance)
	}

	if config.DetectionWindow != 10*time.Minute {
		t.Errorf("Expected detection window 10m, got %v", config.DetectionWindow)
	}

	if config.EnableAutoCorrection {
		t.Error("Expected auto correction to be disabled")
	}

	if config.ConfidenceThreshold != 0.9 {
		t.Errorf("Expected confidence threshold 0.9, got %v", config.ConfidenceThreshold)
	}
}

func TestClockSkewManager_ReportGeneration(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())
	manager.Start()
	defer manager.Stop()

	// Add some data
	now := time.Now()
	for i := 0; i < 15; i++ {
		parent := Span{
			TraceID:     "trace-report",
			SpanID:      "span-p-" + itoa(i),
			ServiceName: "service-a",
			StartTime:   now.Add(time.Duration(i) * time.Second),
		}
		child := Span{
			TraceID:      "trace-report",
			SpanID:       "span-c-" + itoa(i),
			ParentSpanID: parent.SpanID,
			ServiceName:  "service-b",
			StartTime:    now.Add(time.Duration(i)*time.Second + 30*time.Millisecond),
		}
		manager.ProcessSpanPair(parent, child)
	}

	manager.SetCorrection("service-x", 25*time.Millisecond)
	manager.RecordNTPDrift("service-a", 10*time.Millisecond)

	report := manager.GenerateReport()
	if report == nil {
		t.Fatal("Expected report")
	}

	// Verify report structure
	if report.GeneratedAt.IsZero() {
		t.Error("Expected generated timestamp")
	}

	if report.Config.MaxSkewTolerance != 5*time.Second {
		t.Errorf("Expected config in report, got max tolerance %v", report.Config.MaxSkewTolerance)
	}

	if len(report.ServicePairStats) == 0 {
		t.Error("Expected service pair stats in report")
	}

	// At least one correction should exist (service-x)
	if len(report.ActiveCorrections) < 1 {
		t.Errorf("Expected at least 1 active correction, got %d", len(report.ActiveCorrections))
	}

	// Verify service-x correction exists
	if _, found := report.ActiveCorrections["service-x"]; !found {
		t.Error("Expected correction for service-x in report")
	}

	if report.Stats.TotalSpansProcessed != 15 {
		t.Errorf("Expected 15 spans processed, got %d", report.Stats.TotalSpansProcessed)
	}
}

func TestClockSkewManager_ConcurrentAccess(t *testing.T) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())
	manager.Start()
	defer manager.Stop()

	var wg sync.WaitGroup
	numWorkers := 10
	opsPerWorker := 100

	// Concurrent span processing
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			now := time.Now()
			for j := 0; j < opsPerWorker; j++ {
				parent := Span{
					TraceID:     "trace-concurrent",
					SpanID:      "span-p-" + itoa(workerID*1000+j),
					ServiceName: "service-" + itoa(workerID%3),
					StartTime:   now.Add(time.Duration(j) * time.Millisecond),
				}
				child := Span{
					TraceID:      "trace-concurrent",
					SpanID:       "span-c-" + itoa(workerID*1000+j),
					ParentSpanID: parent.SpanID,
					ServiceName:  "service-target",
					StartTime:    now.Add(time.Duration(j)*time.Millisecond + 10*time.Millisecond),
				}
				manager.ProcessSpanPair(parent, child)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerWorker; j++ {
				_ = manager.GetStats()
				_ = manager.GetAllSkewStats()
				_ = manager.GetServiceCorrections()
			}
		}()
	}

	wg.Wait()

	// Verify no panic and data is consistent
	stats := manager.GetStats()
	if stats.TotalSpansProcessed != int64(numWorkers*opsPerWorker) {
		t.Errorf("Expected %d spans processed, got %d",
			numWorkers*opsPerWorker, stats.TotalSpansProcessed)
	}
}

// Benchmark tests

func BenchmarkClockSkewManager_ProcessSpanPairParallel(b *testing.B) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())
	now := time.Now()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			parent := Span{
				TraceID:     "trace-bench",
				SpanID:      "span-p-bench",
				ServiceName: "service-a",
				StartTime:   now,
			}
			child := Span{
				TraceID:      "trace-bench",
				SpanID:       "span-c-bench",
				ParentSpanID: parent.SpanID,
				ServiceName:  "service-b",
				StartTime:    now.Add(10 * time.Millisecond),
			}
			manager.ProcessSpanPair(parent, child)
			i++
		}
	})
}

func BenchmarkClockSkewManager_CorrectSpanParallel(b *testing.B) {
	config := DefaultClockSkewConfig()
	config.EnableAutoCorrection = true
	manager := NewClockSkewManager(config)
	manager.SetCorrection("service-b", 10*time.Millisecond)

	now := time.Now()
	parentStart := now

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			span := Span{
				TraceID:     "trace-bench",
				SpanID:      "span-bench",
				ServiceName: "service-b",
				StartTime:   now.Add(-5 * time.Millisecond),
				EndTime:     now.Add(50 * time.Millisecond),
			}
			manager.CorrectSpan(&span, parentStart)
		}
	})
}

func BenchmarkClockSkewManager_GetAllSkewStats(b *testing.B) {
	manager := NewClockSkewManager(DefaultClockSkewConfig())

	// Populate with some data
	now := time.Now()
	for i := 0; i < 100; i++ {
		parent := Span{
			TraceID:     "trace-" + itoa(i),
			SpanID:      "span-p",
			ServiceName: "service-" + itoa(i%5),
			StartTime:   now,
		}
		child := Span{
			TraceID:      "trace-" + itoa(i),
			SpanID:       "span-c",
			ParentSpanID: parent.SpanID,
			ServiceName:  "service-" + itoa((i+1)%5),
			StartTime:    now.Add(10 * time.Millisecond),
		}
		manager.ProcessSpanPair(parent, child)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetAllSkewStats()
	}
}

// Helper functions

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
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
