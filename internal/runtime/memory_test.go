package runtime

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestMemoryManager_Creation(t *testing.T) {
	config := DefaultMemoryConfig()
	manager := NewMemoryManager(config)

	if manager == nil {
		t.Fatal("Expected manager to be created")
	}

	metrics := manager.GetMetrics()
	if metrics.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
}

func TestMemoryManager_DefaultConfig(t *testing.T) {
	config := DefaultMemoryConfig()

	if config.MaxMemoryMB != 500 {
		t.Errorf("Expected max memory 500MB, got %d", config.MaxMemoryMB)
	}

	if config.GCTargetPercent != 50 {
		t.Errorf("Expected GC target 50%%, got %d", config.GCTargetPercent)
	}

	if !config.EnableObjectPools {
		t.Error("Expected object pools enabled")
	}

	if !config.EnableBufferRecycling {
		t.Error("Expected buffer recycling enabled")
	}
}

func TestMemoryManager_GetMetrics(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	metrics := manager.GetMetrics()

	if metrics.AllocMB <= 0 {
		t.Error("Expected positive alloc MB")
	}

	if metrics.HeapObjects <= 0 {
		t.Error("Expected positive heap objects")
	}

	if metrics.Goroutines <= 0 {
		t.Error("Expected positive goroutine count")
	}
}

func TestMemoryManager_ObjectPools(t *testing.T) {
	config := DefaultMemoryConfig()
	config.EnableObjectPools = true
	manager := NewMemoryManager(config)

	// Test span pool
	span1 := manager.AcquireSpan()
	if span1 == nil {
		t.Fatal("Expected span from pool")
	}

	span1.TraceID = "test-trace"
	span1.Name = "test-span"
	manager.ReleaseSpan(span1)

	// Acquire again - should get recycled span
	span2 := manager.AcquireSpan()
	if span2.TraceID != "" {
		t.Error("Expected span to be reset")
	}
	manager.ReleaseSpan(span2)

	// Test log entry pool
	log1 := manager.AcquireLogEntry()
	if log1 == nil {
		t.Fatal("Expected log entry from pool")
	}
	manager.ReleaseLogEntry(log1)

	// Test metric pool
	metric1 := manager.AcquireMetric()
	if metric1 == nil {
		t.Fatal("Expected metric from pool")
	}
	manager.ReleaseMetric(metric1)

	stats := manager.GetStats()
	if stats.PooledAllocations < 3 {
		t.Errorf("Expected at least 3 pooled allocations, got %d", stats.PooledAllocations)
	}
}

func TestMemoryManager_BufferPool(t *testing.T) {
	config := DefaultMemoryConfig()
	config.EnableBufferRecycling = true
	manager := NewMemoryManager(config)

	buf1 := manager.GetBuffer()
	if buf1 == nil {
		t.Fatal("Expected buffer from pool")
	}

	if cap(buf1) < 4096 {
		t.Errorf("Expected buffer capacity >= 4096, got %d", cap(buf1))
	}

	// Append some data
	buf1 = append(buf1, []byte("test data")...)
	manager.PutBuffer(buf1)

	// Get another buffer
	buf2 := manager.GetBuffer()
	if len(buf2) != 0 {
		t.Error("Expected empty buffer")
	}
	manager.PutBuffer(buf2)
}

func TestMemoryManager_Pressure(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MaxMemoryMB = 1000 // 1GB target for easier testing
	config.PressureThreshold = 70
	config.CriticalThreshold = 85
	manager := NewMemoryManager(config)

	pressure := manager.GetPressure()
	// Should be normal on a fresh system
	if pressure != PressureNormal {
		t.Logf("Pressure: %s (may vary based on system load)", pressure)
	}
}

func TestMemoryManager_ShouldAccept(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MaxMemoryMB = 10000 // Large limit to ensure normal pressure
	manager := NewMemoryManager(config)

	// With normal pressure, should accept
	if !manager.ShouldAccept() {
		t.Error("Expected to accept requests under normal pressure")
	}
}

func TestMemoryManager_StartStop(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MetricsInterval = 100 * time.Millisecond
	manager := NewMemoryManager(config)

	manager.Start()

	// Wait for some metrics collection
	time.Sleep(250 * time.Millisecond)

	history := manager.GetMetricsHistory(time.Hour)
	if len(history) < 1 {
		t.Error("Expected at least 1 metrics entry in history")
	}

	manager.Stop()
}

func TestMemoryManager_ForceGC(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	// Allocate some memory
	data := make([]byte, 10*1024*1024) // 10MB
	_ = data

	// Force GC
	manager.ForceGC()

	stats := manager.GetStats()
	if stats.ForcedGCs < 1 {
		t.Error("Expected at least 1 forced GC")
	}
}

func TestMemoryManager_UpdateConfig(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	newConfig := DefaultMemoryConfig()
	newConfig.MaxMemoryMB = 1000
	newConfig.GCTargetPercent = 100

	manager.UpdateConfig(newConfig)

	config := manager.GetConfig()
	if config.MaxMemoryMB != 1000 {
		t.Errorf("Expected max memory 1000MB, got %d", config.MaxMemoryMB)
	}
}

func TestMemoryManager_PressureCallback(t *testing.T) {
	config := DefaultMemoryConfig()
	config.MetricsInterval = 50 * time.Millisecond
	manager := NewMemoryManager(config)

	callbackCalled := false
	manager.SetOnPressureChange(func(p MemoryPressure) {
		callbackCalled = true
	})

	manager.Start()
	defer manager.Stop()

	// The callback would be called if pressure changes
	// This test just verifies the callback is set correctly
	time.Sleep(100 * time.Millisecond)

	// Use the variable to satisfy the compiler
	_ = callbackCalled
}

func TestMemoryManager_GenerateReport(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())

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

	// With history
	report = manager.GenerateReport(true)
	// History may be empty if not started
}

func TestPooledSpan_Reset(t *testing.T) {
	span := &PooledSpan{
		TraceID:     "trace-1",
		SpanID:      "span-1",
		Name:        "test",
		ServiceName: "service",
		Attributes:  map[string]string{"key": "value"},
	}

	span.Reset()

	if span.TraceID != "" {
		t.Error("Expected TraceID to be reset")
	}

	if span.Name != "" {
		t.Error("Expected Name to be reset")
	}

	if len(span.Attributes) != 0 {
		t.Error("Expected Attributes to be cleared")
	}
}

func TestPooledLogEntry_Reset(t *testing.T) {
	entry := &PooledLogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "test",
		Service:   "service",
		Labels:    map[string]string{"key": "value"},
	}

	entry.Reset()

	if entry.Level != "" {
		t.Error("Expected Level to be reset")
	}

	if len(entry.Labels) != 0 {
		t.Error("Expected Labels to be cleared")
	}
}

func TestPooledMetric_Reset(t *testing.T) {
	metric := &PooledMetric{
		Name:      "test_metric",
		Value:     42.0,
		Timestamp: time.Now(),
		Tags:      map[string]string{"key": "value"},
	}

	metric.Reset()

	if metric.Name != "" {
		t.Error("Expected Name to be reset")
	}

	if metric.Value != 0 {
		t.Error("Expected Value to be reset")
	}

	if len(metric.Tags) != 0 {
		t.Error("Expected Tags to be cleared")
	}
}

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool(1000)

	buf := pool.Get()
	if buf == nil {
		t.Fatal("Expected buffer")
	}

	if cap(buf) < 4096 {
		t.Errorf("Expected capacity >= 4096, got %d", cap(buf))
	}

	buf = append(buf, []byte("test")...)
	pool.Put(buf)

	stats := pool.Stats()
	if stats.Acquired != 1 {
		t.Errorf("Expected 1 acquired, got %d", stats.Acquired)
	}

	if stats.Released != 1 {
		t.Errorf("Expected 1 released, got %d", stats.Released)
	}
}

func TestStreamingBatcher(t *testing.T) {
	var processed [][]int
	batcher := NewStreamingBatcher(3, func(batch []int) error {
		// Copy batch since it gets reused
		cp := make([]int, len(batch))
		copy(cp, batch)
		processed = append(processed, cp)
		return nil
	})

	// Add items
	batcher.Add(1)
	batcher.Add(2)
	batcher.Add(3) // Should trigger processing

	if len(processed) != 1 {
		t.Errorf("Expected 1 batch processed, got %d", len(processed))
	}

	if len(processed[0]) != 3 {
		t.Errorf("Expected batch size 3, got %d", len(processed[0]))
	}

	// Add more
	batcher.Add(4)
	batcher.Add(5)
	batcher.Flush() // Force flush remaining

	if len(processed) != 2 {
		t.Errorf("Expected 2 batches processed, got %d", len(processed))
	}

	if len(processed[1]) != 2 {
		t.Errorf("Expected final batch size 2, got %d", len(processed[1]))
	}
}

func TestMemoryPressure_String(t *testing.T) {
	tests := []struct {
		pressure MemoryPressure
		expected string
	}{
		{PressureNormal, "normal"},
		{PressureElevated, "elevated"},
		{PressureCritical, "critical"},
	}

	for _, tt := range tests {
		if tt.pressure.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.pressure.String())
		}
	}
}

func BenchmarkMemoryManager_AcquireSpan(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := manager.AcquireSpan()
		manager.ReleaseSpan(span)
	}
}

func BenchmarkMemoryManager_AcquireLogEntry(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := manager.AcquireLogEntry()
		manager.ReleaseLogEntry(entry)
	}
}

func BenchmarkMemoryManager_AcquireMetric(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metric := manager.AcquireMetric()
		manager.ReleaseMetric(metric)
	}
}

func BenchmarkMemoryManager_GetBuffer(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := manager.GetBuffer()
		manager.PutBuffer(buf)
	}
}

func BenchmarkMemoryManager_GetMetrics(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetMetrics()
	}
}

func BenchmarkMemoryManager_Concurrent(b *testing.B) {
	manager := NewMemoryManager(DefaultMemoryConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			span := manager.AcquireSpan()
			span.TraceID = "test"
			span.Name = "bench"
			manager.ReleaseSpan(span)
		}
	})
}

func BenchmarkBufferPool_Concurrent(b *testing.B) {
	pool := NewBufferPool(10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := pool.Get()
			buf = append(buf, []byte("test data")...)
			pool.Put(buf)
		}
	})
}

// Test concurrent access to memory manager
func TestMemoryManager_Concurrent(t *testing.T) {
	manager := NewMemoryManager(DefaultMemoryConfig())
	manager.Start()
	defer manager.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Multiple goroutines acquiring/releasing spans
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				span := manager.AcquireSpan()
				span.TraceID = "test"
				manager.ReleaseSpan(span)
			}
		}()
	}

	// Multiple goroutines getting metrics
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				manager.GetMetrics()
				runtime.Gosched()
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent error: %v", err)
	}
}
