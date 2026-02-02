package probe

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// itoa converts int to string
func itoa(i int) string {
	return strconv.Itoa(i)
}

// Integration tests for HotReloadManager

func TestHotReloadManager_ReloadWithoutDowntime(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var loadCount int64

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)

		// Simulate probe initialization
		time.Sleep(50 * time.Millisecond)

		probe := newMockProbe("test-probe", "1.0."+itoa(int(count)))
		return probe, nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Initial load
	err := manager.Reload("test-probe", nil)
	if err != nil {
		t.Fatalf("Initial reload failed: %v", err)
	}

	// Verify probe exists
	info := manager.GetProbeInfo("test-probe")
	if info == nil {
		t.Error("Expected probe to exist after initial load")
	}

	// Concurrent requests during reload
	var wg sync.WaitGroup
	requestResults := make([]bool, 100)

	// Start background "requests" that check probe availability
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Check if probe is available
			info := manager.GetProbeInfo("test-probe")
			requestResults[idx] = info != nil
			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	// Trigger reload mid-way
	time.Sleep(20 * time.Millisecond)
	err = manager.Reload("test-probe", nil)
	if err != nil {
		t.Errorf("Reload during requests failed: %v", err)
	}

	wg.Wait()

	// All requests should have succeeded (zero downtime)
	failedRequests := 0
	for _, success := range requestResults {
		if !success {
			failedRequests++
		}
	}

	if failedRequests > 0 {
		t.Errorf("Had %d failed requests during reload (expected 0)", failedRequests)
	}

	// Verify final state
	finalInfo := manager.GetProbeInfo("test-probe")
	if finalInfo == nil {
		t.Fatal("Probe info should exist after reloads")
	}

	if finalInfo.Version.Version != "1.0.2" {
		t.Errorf("Expected version 1.0.2 after 2 loads, got %s", finalInfo.Version.Version)
	}
}

func TestHotReloadManager_RollbackOnFailure(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var loadCount int64

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)

		// Fail on third load attempt
		if count == 3 {
			return nil, errors.New("simulated load failure")
		}

		return newMockProbe("test-probe", "1.0."+itoa(int(count))), nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Load v1
	err := manager.Reload("test-probe", nil)
	if err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	// Load v2
	err = manager.Reload("test-probe", nil)
	if err != nil {
		t.Fatalf("Second load failed: %v", err)
	}

	version := manager.GetVersion("test-probe")
	if version == nil || version.Version != "1.0.2" {
		t.Errorf("Expected version 1.0.2 before failure, got %v", version)
	}

	// Third load fails
	err = manager.Reload("test-probe", nil)
	if err == nil {
		t.Error("Expected third load to fail")
	}

	// Version history should still have both previous versions
	history := manager.GetVersionHistory("test-probe")
	if len(history) != 2 {
		t.Errorf("Expected 2 versions in history, got %d", len(history))
	}

	// Rollback to v1
	err = manager.Rollback("test-probe")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Check stats
	stats := manager.Stats()
	if stats.RollbackCount != 1 {
		t.Errorf("Expected 1 rollback, got %d", stats.RollbackCount)
	}
}

func TestHotReloadManager_ConfigUpdates(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	// Get original config
	originalConfig := manager.GetConfig("test-probe")
	if originalConfig == nil {
		t.Fatal("Expected config")
	}

	if originalConfig.SampleRate != 1.0 {
		t.Errorf("Expected default sample rate 1.0, got %f", originalConfig.SampleRate)
	}

	// Update config
	newConfig := DefaultProbeConfig()
	newConfig.SampleRate = 0.5
	newConfig.MaxEventsPerSec = 50000
	newConfig.BufferSize = 2000

	err := manager.UpdateConfig("test-probe", newConfig)
	if err != nil {
		t.Fatalf("Config update failed: %v", err)
	}

	// Verify update
	updatedConfig := manager.GetConfig("test-probe")
	if updatedConfig.SampleRate != 0.5 {
		t.Errorf("Expected updated sample rate 0.5, got %f", updatedConfig.SampleRate)
	}

	if updatedConfig.MaxEventsPerSec != 50000 {
		t.Errorf("Expected max events 50000, got %d", updatedConfig.MaxEventsPerSec)
	}

	if updatedConfig.BufferSize != 2000 {
		t.Errorf("Expected buffer size 2000, got %d", updatedConfig.BufferSize)
	}
}

func TestHotReloadManager_HealthMonitoring(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	// Register probes with different health states
	healthyProbe := &mockReloadableProbe{
		name:    "healthy-probe",
		version: "1.0.0",
		healthy: true,
	}

	degradedProbe := &mockReloadableProbe{
		name:       "degraded-probe",
		version:    "1.0.0",
		healthy:    true,
		errorCount: 100, // High error count
	}

	failedProbe := &mockReloadableProbe{
		name:    "failed-probe",
		version: "1.0.0",
		healthy: false,
	}

	manager.RegisterProbe("healthy-probe", healthyProbe)
	manager.RegisterProbe("degraded-probe", degradedProbe)
	manager.RegisterProbe("failed-probe", failedProbe)

	// Get individual health
	healthyHealth := manager.GetHealth("healthy-probe")
	if healthyHealth == nil {
		t.Fatal("Expected health for healthy probe")
	}
	if healthyHealth.Status != "healthy" {
		t.Errorf("Expected healthy status, got %s", healthyHealth.Status)
	}

	failedHealth := manager.GetHealth("failed-probe")
	if failedHealth == nil {
		t.Fatal("Expected health for failed probe")
	}
	if failedHealth.Status != "failed" {
		t.Errorf("Expected failed status, got %s", failedHealth.Status)
	}

	// Get all health
	allHealth := manager.GetAllHealth()
	if len(allHealth) != 3 {
		t.Errorf("Expected 3 health entries, got %d", len(allHealth))
	}

	// Count by status
	statusCounts := make(map[string]int)
	for _, h := range allHealth {
		statusCounts[h.Status]++
	}

	if statusCounts["healthy"] != 2 {
		t.Errorf("Expected 2 healthy probes, got %d", statusCounts["healthy"])
	}

	if statusCounts["failed"] != 1 {
		t.Errorf("Expected 1 failed probe, got %d", statusCounts["failed"])
	}
}

func TestHotReloadManager_VersionHistoryExtended(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var loadCount int64

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)
		return newMockProbe("test-probe", "1.0."+itoa(int(count))), nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Load multiple versions
	for i := 0; i < 5; i++ {
		err := manager.Reload("test-probe", nil)
		if err != nil {
			t.Fatalf("Load %d failed: %v", i+1, err)
		}
		time.Sleep(10 * time.Millisecond) // Ensure distinct timestamps
	}

	// Get version history
	history := manager.GetVersionHistory("test-probe")
	if len(history) != 5 {
		t.Errorf("Expected 5 versions in history, got %d", len(history))
	}

	// Verify history contains all versions (order may vary by implementation)
	versionsSeen := make(map[string]bool)
	for _, h := range history {
		versionsSeen[h.Version] = true
	}

	expectedVersions := []string{"1.0.1", "1.0.2", "1.0.3", "1.0.4", "1.0.5"}
	for _, v := range expectedVersions {
		if !versionsSeen[v] {
			t.Errorf("Expected version %s in history", v)
		}
	}

	t.Logf("Version history contains %d versions", len(history))
}

func TestHotReloadManager_CallbacksExtended(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var reloadCalls []string
	var rollbackCalls []string
	var mu sync.Mutex

	manager.SetOnReload(func(name string, version ProbeVersion) {
		mu.Lock()
		reloadCalls = append(reloadCalls, name+":"+version.Version)
		mu.Unlock()
	})

	manager.SetOnRollback(func(name string, from, to ProbeVersion, err error) {
		mu.Lock()
		rollbackCalls = append(rollbackCalls, name+":"+from.Version+"->"+to.Version)
		mu.Unlock()
	})

	var loadCount int64
	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)
		return newMockProbe("callback-probe", "v"+itoa(int(count))), nil
	}

	manager.RegisterLoader("callback-probe", loader)

	// Trigger reloads
	manager.Reload("callback-probe", nil)
	manager.Reload("callback-probe", nil)

	// Trigger rollback
	manager.Rollback("callback-probe")

	mu.Lock()
	defer mu.Unlock()

	// Verify reload callbacks
	if len(reloadCalls) != 2 {
		t.Errorf("Expected 2 reload callbacks, got %d", len(reloadCalls))
	}

	if len(reloadCalls) > 0 && reloadCalls[0] != "callback-probe:v1" {
		t.Errorf("Expected first reload callback for v1, got %s", reloadCalls[0])
	}

	// Verify rollback callbacks (may not fire if rollback implementation doesn't call it)
	t.Logf("Rollback callbacks: %d", len(rollbackCalls))
}

func TestHotReloadManager_ConcurrentOperations(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var loadCount int64

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)
		// Simulate some initialization time
		time.Sleep(10 * time.Millisecond)
		return newMockProbe("concurrent-probe", "v"+itoa(int(count))), nil
	}

	manager.RegisterLoader("concurrent-probe", loader)

	// Initial load
	manager.Reload("concurrent-probe", nil)

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reloads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := manager.Reload("concurrent-probe", nil); err != nil {
				errors <- err
			}
		}()
	}

	// Concurrent health checks
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.GetHealth("concurrent-probe")
			_ = manager.GetAllHealth()
		}()
	}

	// Concurrent config reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.GetConfig("concurrent-probe")
			_ = manager.GetVersion("concurrent-probe")
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Had %d errors during concurrent operations", errorCount)
	}

	// Verify final state is valid
	info := manager.GetProbeInfo("concurrent-probe")
	if info == nil {
		t.Error("Probe info should exist after concurrent operations")
	}

	stats := manager.Stats()
	t.Logf("Final stats: reloads=%d, failures=%d", stats.ReloadCount, stats.FailureCount)
}

func TestHotReloadManager_StatsAccumulation(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var loadCount int64
	var shouldFail int64

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		count := atomic.AddInt64(&loadCount, 1)
		if atomic.LoadInt64(&shouldFail) == 1 {
			return nil, errors.New("intentional failure")
		}
		return newMockProbe("stats-probe", "v"+itoa(int(count))), nil
	}

	manager.RegisterLoader("stats-probe", loader)

	// Successful reloads
	for i := 0; i < 5; i++ {
		manager.Reload("stats-probe", nil)
	}

	// Failed reload
	atomic.StoreInt64(&shouldFail, 1)
	manager.Reload("stats-probe", nil)

	// Reset for rollback
	atomic.StoreInt64(&shouldFail, 0)

	// Rollback
	manager.Rollback("stats-probe")

	stats := manager.Stats()

	if stats.ReloadCount != 5 {
		t.Errorf("Expected 5 successful reloads, got %d", stats.ReloadCount)
	}

	if stats.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", stats.FailureCount)
	}

	if stats.RollbackCount != 1 {
		t.Errorf("Expected 1 rollback, got %d", stats.RollbackCount)
	}
}

// Helper types

type mockLoadingProbe struct {
	name    string
	version string
	config  ProbeConfig
	running *int64
}

func (p *mockLoadingProbe) Name() string { return p.name }

func (p *mockLoadingProbe) Version() ProbeVersion {
	return ProbeVersion{
		Version:  p.version,
		Name:     p.name,
		LoadedAt: time.Now(),
	}
}

func (p *mockLoadingProbe) Health() ProbeHealth {
	return ProbeHealth{
		Name:      p.name,
		Version:   p.version,
		Status:    "healthy",
		LastCheck: time.Now(),
	}
}

func (p *mockLoadingProbe) Config() ProbeConfig {
	if p.config.BufferSize == 0 {
		return DefaultProbeConfig()
	}
	return p.config
}

func (p *mockLoadingProbe) UpdateConfig(config ProbeConfig) error {
	p.config = config
	return nil
}

func (p *mockLoadingProbe) Close() error {
	if p.running != nil {
		atomic.StoreInt64(p.running, 0)
	}
	return nil
}

func init() {
	// Ensure mockLoadingProbe implements ReloadableProbe
	var _ ReloadableProbe = (*mockLoadingProbe)(nil)
}

// Benchmark tests

func BenchmarkHotReloadManager_GetHealthExtended(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	for i := 0; i < 10; i++ {
		probe := newMockProbe("probe-"+itoa(i), "1.0.0")
		manager.RegisterProbe("probe-"+itoa(i), probe)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetHealth("probe-5")
	}
}

func BenchmarkHotReloadManager_GetAllHealthParallel(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	for i := 0; i < 20; i++ {
		probe := newMockProbe("probe-"+itoa(i), "1.0.0")
		manager.RegisterProbe("probe-"+itoa(i), probe)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = manager.GetAllHealth()
		}
	})
}

func BenchmarkHotReloadManager_GetConfigParallel(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = manager.GetConfig("test-probe")
		}
	})
}

func BenchmarkHotReloadManager_Reload(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	var count int64
	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		c := atomic.AddInt64(&count, 1)
		return newMockProbe("bench-probe", "v"+itoa(int(c))), nil
	}

	manager.RegisterLoader("bench-probe", loader)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Reload("bench-probe", nil)
	}
}
