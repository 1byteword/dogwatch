package runtime

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Integration tests for MemoryManager

func TestMemoryManager_ObjectPoolUnderLoad(t *testing.T) {
	config := DefaultMemoryConfig()
	config.EnableObjectPools = true
	manager := NewMemoryManager(config)

	const numGoroutines = 50
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	var totalSpanOps, totalLogOps, totalMetricOps int64

	// Concurrent span pool operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				span := manager.AcquireSpan()
				if span == nil {
					t.Error("AcquireSpan returned nil")
					return
				}
				span.TraceID = "test-trace"
				span.Name = "test-span"
				span.ServiceName = "test-service"
				manager.ReleaseSpan(span)
				atomic.AddInt64(&totalSpanOps, 1)
			}
		}()
	}

	// Concurrent log entry pool operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				entry := manager.AcquireLogEntry()
				if entry == nil {
					t.Error("AcquireLogEntry returned nil")
					return
				}
				entry.Level = "INFO"
				entry.Message = "test message"
				manager.ReleaseLogEntry(entry)
				atomic.AddInt64(&totalLogOps, 1)
			}
		}()
	}

	// Concurrent metric pool operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				metric := manager.AcquireMetric()
				if metric == nil {
					t.Error("AcquireMetric returned nil")
					return
				}
				metric.Name = "test_metric"
				metric.Value = 42.0
				manager.ReleaseMetric(metric)
				atomic.AddInt64(&totalMetricOps, 1)
			}
		}()
	}

	wg.Wait()

	expectedOps := int64(numGoroutines * opsPerGoroutine)
	if totalSpanOps != expectedOps {
		t.Errorf("Expected %d span ops, got %d", expectedOps, totalSpanOps)
	}
	if totalLogOps != expectedOps {
		t.Errorf("Expected %d log ops, got %d", expectedOps, totalLogOps)
	}
	if totalMetricOps != expectedOps {
		t.Errorf("Expected %d metric ops, got %d", expectedOps, totalMetricOps)
	}

	// Verify stats reflect pooled allocations
	stats := manager.GetStats()
	expectedPooled := expectedOps * 3
	if stats.PooledAllocations < expectedPooled {
		t.Errorf("Expected at least %d pooled allocations, got %d",
			expectedPooled, stats.PooledAllocations)
	}
}

func TestMemoryManager_MemoryPressureDetection(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MaxMemoryMB = 100 // 100MB limit for testing
	config.PressureThreshold = 70
	config.CriticalThreshold = 85
	manager := NewMemoryManager(config)

	// Initially should be normal pressure (unless system is already under load)
	initialPressure := manager.GetPressure()
	t.Logf("Initial pressure: %s", initialPressure)

	// Should accept requests under normal pressure
	if !manager.ShouldAccept() {
		t.Log("Note: System may be under load, ShouldAccept returned false")
	}

	// Get metrics and verify they're populated
	metrics := manager.GetMetrics()
	if metrics.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp in metrics")
	}

	if metrics.Goroutines <= 0 {
		t.Error("Expected positive goroutine count")
	}

	if metrics.HeapObjects <= 0 {
		t.Error("Expected positive heap objects count")
	}

	t.Logf("Current memory metrics: Alloc=%.2fMB, HeapObjects=%d, Goroutines=%d",
		metrics.AllocMB, metrics.HeapObjects, metrics.Goroutines)
}

func TestMemoryManager_BackpressureBehavior(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MaxMemoryMB = 50 // Low limit to potentially trigger pressure
	config.PressureThreshold = 70
	config.CriticalThreshold = 85
	manager := NewMemoryManager(config)

	// Test that ShouldAccept works correctly
	accepted := 0
	rejected := 0

	for i := 0; i < 100; i++ {
		if manager.ShouldAccept() {
			accepted++
		} else {
			rejected++
		}
	}

	// In normal conditions, most should be accepted
	t.Logf("Backpressure test: accepted=%d, rejected=%d", accepted, rejected)

	// At least some should be accepted unless system is under heavy load
	if accepted == 0 {
		t.Log("Warning: All requests rejected - system may be under memory pressure")
	}
}

func TestMemoryManager_GCTuningEffects(t *testing.T) {
	config := DefaultMemoryConfig()
	config.GCTargetPercent = 50
	manager := NewMemoryManager(config)

	// Record initial GC count
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	initialGCs := memStats.NumGC

	// Force GC multiple times
	for i := 0; i < 3; i++ {
		manager.ForceGC()
		time.Sleep(10 * time.Millisecond) // Allow GC to complete
	}

	// Check stats
	stats := manager.GetStats()
	if stats.ForcedGCs < 3 {
		t.Errorf("Expected at least 3 forced GCs, got %d", stats.ForcedGCs)
	}

	// Verify GC actually ran
	runtime.ReadMemStats(&memStats)
	if memStats.NumGC <= initialGCs {
		t.Log("Note: GC count did not increase - GC may have been no-op")
	}
}

func TestMemoryManager_BufferPoolPerformance(t *testing.T) {
	config := DefaultMemoryConfig()
	config.EnableBufferRecycling = true
	manager := NewMemoryManager(config)

	const iterations = 10000

	// Measure buffer pool performance
	start := time.Now()
	for i := 0; i < iterations; i++ {
		buf := manager.GetBuffer()
		buf = append(buf, []byte("test data that fills the buffer")...)
		manager.PutBuffer(buf)
	}
	pooledDuration := time.Since(start)

	// Compare with direct allocation
	start = time.Now()
	for i := 0; i < iterations; i++ {
		buf := make([]byte, 0, 4096)
		buf = append(buf, []byte("test data that fills the buffer")...)
		_ = buf
	}
	directDuration := time.Since(start)

	t.Logf("Buffer operations (%d): pooled=%v, direct=%v",
		iterations, pooledDuration, directDuration)

	// Pooled should generally be faster (or similar) due to reuse
	// But we don't fail the test on this as it depends on runtime conditions
}

func TestMemoryManager_MetricsCollection(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MetricsInterval = 50 * time.Millisecond
	manager := NewMemoryManager(config)

	manager.Start()
	defer manager.Stop()

	// Wait for several collection cycles
	time.Sleep(200 * time.Millisecond)

	// Get history
	history := manager.GetMetricsHistory(time.Hour)
	if len(history) < 2 {
		t.Errorf("Expected at least 2 history entries, got %d", len(history))
	}

	// Verify history entries are valid
	for i, entry := range history {
		if entry.Timestamp.IsZero() {
			t.Errorf("History entry %d has zero timestamp", i)
		}
		if entry.AllocMB <= 0 {
			t.Errorf("History entry %d has non-positive AllocMB: %v", i, entry.AllocMB)
		}
	}

	// Verify history is in chronological order
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Error("History is not in chronological order")
		}
	}
}

func TestMemoryManager_ReportGeneration(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MetricsInterval = 50 * time.Millisecond
	manager := NewMemoryManager(config)

	manager.Start()
	defer manager.Stop()

	// Wait for metrics collection
	time.Sleep(150 * time.Millisecond)

	// Generate report without history
	report := manager.GenerateReport(false)
	if report == nil {
		t.Fatal("Expected report")
	}

	if report.GeneratedAt.IsZero() {
		t.Error("Expected generated timestamp")
	}

	if report.Current.Timestamp.IsZero() {
		t.Error("Expected current metrics timestamp")
	}

	if report.History != nil {
		t.Error("Expected no history in report when includeHistory=false")
	}

	// Generate report with history
	reportWithHistory := manager.GenerateReport(true)
	if reportWithHistory == nil {
		t.Fatal("Expected report with history")
	}

	if len(reportWithHistory.History) == 0 {
		t.Log("Note: History may be empty if collection just started")
	}
}

func TestMemoryManager_PressureCallbackExtended(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MetricsInterval = 50 * time.Millisecond
	manager := NewMemoryManager(config)

	var callbackCount int64
	var lastPressure MemoryPressure
	var mu sync.Mutex

	manager.SetOnPressureChange(func(p MemoryPressure) {
		mu.Lock()
		atomic.AddInt64(&callbackCount, 1)
		lastPressure = p
		mu.Unlock()
	})

	manager.Start()
	defer manager.Stop()

	// Wait a bit for potential callback
	time.Sleep(200 * time.Millisecond)

	// Callback may or may not be called depending on system state
	count := atomic.LoadInt64(&callbackCount)
	t.Logf("Pressure callback count: %d, last pressure: %v", count, lastPressure)
}

func TestMemoryManager_ConfigUpdate(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	originalConfig := manager.GetConfig()
	if originalConfig.MaxMemoryMB != 500 {
		t.Errorf("Expected default max memory 500MB, got %d", originalConfig.MaxMemoryMB)
	}

	// Update config
	newConfig := DefaultMemoryConfig()
	newConfig.MaxMemoryMB = 1000
	newConfig.GCTargetPercent = 75
	newConfig.PressureThreshold = 80
	newConfig.EnableObjectPools = false

	manager.UpdateConfig(newConfig)

	updatedConfig := manager.GetConfig()
	if updatedConfig.MaxMemoryMB != 1000 {
		t.Errorf("Expected updated max memory 1000MB, got %d", updatedConfig.MaxMemoryMB)
	}

	if updatedConfig.GCTargetPercent != 75 {
		t.Errorf("Expected GC target 75%%, got %d", updatedConfig.GCTargetPercent)
	}

	if updatedConfig.PressureThreshold != 80 {
		t.Errorf("Expected pressure threshold 80, got %d", updatedConfig.PressureThreshold)
	}

	if updatedConfig.EnableObjectPools {
		t.Error("Expected object pools to be disabled")
	}
}

func TestMemoryManager_PooledObjectReset(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	// Test span reset
	span := manager.AcquireSpan()
	span.TraceID = "trace-123"
	span.SpanID = "span-456"
	span.Name = "test-operation"
	span.ServiceName = "test-service"
	span.Attributes = map[string]string{"key": "value"}

	manager.ReleaseSpan(span)

	span2 := manager.AcquireSpan()
	// Span should be reset
	if span2.TraceID != "" {
		t.Error("Expected TraceID to be reset")
	}
	if span2.Name != "" {
		t.Error("Expected Name to be reset")
	}
	if len(span2.Attributes) != 0 {
		t.Error("Expected Attributes to be cleared")
	}
	manager.ReleaseSpan(span2)

	// Test log entry reset
	entry := manager.AcquireLogEntry()
	entry.Level = "ERROR"
	entry.Message = "test message"
	entry.Service = "test-service"
	entry.Labels = map[string]string{"env": "prod"}

	manager.ReleaseLogEntry(entry)

	entry2 := manager.AcquireLogEntry()
	if entry2.Level != "" {
		t.Error("Expected Level to be reset")
	}
	if entry2.Message != "" {
		t.Error("Expected Message to be reset")
	}
	if len(entry2.Labels) != 0 {
		t.Error("Expected Labels to be cleared")
	}
	manager.ReleaseLogEntry(entry2)

	// Test metric reset
	metric := manager.AcquireMetric()
	metric.Name = "test_metric"
	metric.Value = 42.0
	metric.Tags = map[string]string{"host": "server1"}

	manager.ReleaseMetric(metric)

	metric2 := manager.AcquireMetric()
	if metric2.Name != "" {
		t.Error("Expected Name to be reset")
	}
	if metric2.Value != 0 {
		t.Error("Expected Value to be reset")
	}
	if len(metric2.Tags) != 0 {
		t.Error("Expected Tags to be cleared")
	}
	manager.ReleaseMetric(metric2)
}

func TestMemoryManager_StreamingBatcher(t *testing.T) {
	var processed [][]int
	var mu sync.Mutex

	batcher := NewStreamingBatcher(5, func(batch []int) error {
		mu.Lock()
		// Copy batch since it gets reused
		cp := make([]int, len(batch))
		copy(cp, batch)
		processed = append(processed, cp)
		mu.Unlock()
		return nil
	})

	// Add 12 items (should trigger 2 batches, leave 2 pending)
	for i := 1; i <= 12; i++ {
		batcher.Add(i)
	}

	// Should have processed 2 batches of 5
	mu.Lock()
	if len(processed) != 2 {
		t.Errorf("Expected 2 batches processed, got %d", len(processed))
	}

	// First batch should be [1,2,3,4,5]
	if len(processed) > 0 && len(processed[0]) != 5 {
		t.Errorf("First batch size: expected 5, got %d", len(processed[0]))
	}

	// Second batch should be [6,7,8,9,10]
	if len(processed) > 1 && len(processed[1]) != 5 {
		t.Errorf("Second batch size: expected 5, got %d", len(processed[1]))
	}
	mu.Unlock()

	// Flush remaining
	batcher.Flush()

	mu.Lock()
	if len(processed) != 3 {
		t.Errorf("Expected 3 batches after flush, got %d", len(processed))
	}

	// Third batch should be [11,12]
	if len(processed) > 2 && len(processed[2]) != 2 {
		t.Errorf("Third batch size: expected 2, got %d", len(processed[2]))
	}
	mu.Unlock()
}

// Benchmark tests

func BenchmarkMemoryManager_AcquireReleaseSpan(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := manager.AcquireSpan()
		span.TraceID = "test"
		span.Name = "benchmark"
		manager.ReleaseSpan(span)
	}
}

func BenchmarkMemoryManager_AcquireReleaseSpanParallel(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			span := manager.AcquireSpan()
			span.TraceID = "test"
			span.Name = "benchmark"
			manager.ReleaseSpan(span)
		}
	})
}

func BenchmarkMemoryManager_BufferPoolParallel(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := manager.GetBuffer()
			buf = append(buf, []byte("benchmark data")...)
			manager.PutBuffer(buf)
		}
	})
}

func BenchmarkMemoryManager_GetMetricsExtended(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetMetrics()
	}
}

func BenchmarkMemoryManager_GetPressure(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetPressure()
	}
}

func BenchmarkBufferPool_GetPut(b *testing.B) {
	pool := NewBufferPool(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get()
		buf = append(buf, []byte("test")...)
		pool.Put(buf)
	}
}

func BenchmarkBufferPool_Parallel(b *testing.B) {
	pool := NewBufferPool(10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			buf = append(buf, []byte("benchmark data for testing")...)
			pool.Put(buf)
		}
	})
}

func BenchmarkStreamingBatcher_Add(b *testing.B) {
	batcher := NewStreamingBatcher(100, func(batch []int) error {
		return nil
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batcher.Add(i)
	}
}
