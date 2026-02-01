package probe

import (
	"sync/atomic"
	"testing"
	"time"
)

// mockReloadableProbe implements ReloadableProbe for testing
type mockReloadableProbe struct {
	name        string
	version     string
	config      ProbeConfig
	eventCount  int64
	errorCount  int64
	healthy     bool
	closeCalled bool
}

func newMockProbe(name, version string) *mockReloadableProbe {
	return &mockReloadableProbe{
		name:    name,
		version: version,
		config:  DefaultProbeConfig(),
		healthy: true,
	}
}

func (p *mockReloadableProbe) Name() string {
	return p.name
}

func (p *mockReloadableProbe) Version() ProbeVersion {
	return ProbeVersion{
		Version:  p.version,
		Name:     p.name,
		LoadedAt: time.Now(),
	}
}

func (p *mockReloadableProbe) Health() ProbeHealth {
	status := "healthy"
	if !p.healthy {
		status = "failed"
	}
	return ProbeHealth{
		Name:        p.name,
		Version:     p.version,
		Status:      status,
		LastCheck:   time.Now(),
		EventsTotal: atomic.LoadInt64(&p.eventCount),
		ErrorCount:  atomic.LoadInt64(&p.errorCount),
	}
}

func (p *mockReloadableProbe) Config() ProbeConfig {
	return p.config
}

func (p *mockReloadableProbe) UpdateConfig(config ProbeConfig) error {
	p.config = config
	return nil
}

func (p *mockReloadableProbe) Close() error {
	p.closeCalled = true
	return nil
}

func TestHotReloadManager_RegisterProbe(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	probes := manager.ListProbes()
	if len(probes) != 1 {
		t.Errorf("Expected 1 probe, got %d", len(probes))
	}

	if probes[0] != "test-probe" {
		t.Errorf("Expected probe name 'test-probe', got '%s'", probes[0])
	}
}

func TestHotReloadManager_GetProbeInfo(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	info := manager.GetProbeInfo("test-probe")
	if info == nil {
		t.Fatal("Expected probe info, got nil")
	}

	if info.Name != "test-probe" {
		t.Errorf("Expected name 'test-probe', got '%s'", info.Name)
	}

	if info.Version.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", info.Version.Version)
	}
}

func TestHotReloadManager_Reload(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	versionCounter := 0
	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		versionCounter++
		return newMockProbe("test-probe", "1.0." + string(rune('0'+versionCounter))), nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Initial load
	err := manager.Reload("test-probe", nil)
	if err != nil {
		t.Fatalf("Initial reload failed: %v", err)
	}

	version := manager.GetVersion("test-probe")
	if version == nil {
		t.Fatal("Expected version, got nil")
	}

	// Second reload
	err = manager.Reload("test-probe", nil)
	if err != nil {
		t.Fatalf("Second reload failed: %v", err)
	}

	stats := manager.Stats()
	if stats.ReloadCount != 2 {
		t.Errorf("Expected reload count 2, got %d", stats.ReloadCount)
	}
}

func TestHotReloadManager_Rollback(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	versionCounter := 0
	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		versionCounter++
		return newMockProbe("test-probe", "1.0." + string(rune('0'+versionCounter))), nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Load v1
	manager.Reload("test-probe", nil)
	// Load v2
	manager.Reload("test-probe", nil)

	// Rollback to v1
	err := manager.Rollback("test-probe")
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	stats := manager.Stats()
	if stats.RollbackCount != 1 {
		t.Errorf("Expected rollback count 1, got %d", stats.RollbackCount)
	}
}

func TestHotReloadManager_UpdateConfig(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	newConfig := DefaultProbeConfig()
	newConfig.SampleRate = 0.5
	newConfig.MaxEventsPerSec = 50000

	err := manager.UpdateConfig("test-probe", newConfig)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	config := manager.GetConfig("test-probe")
	if config == nil {
		t.Fatal("Expected config, got nil")
	}

	if config.SampleRate != 0.5 {
		t.Errorf("Expected sample rate 0.5, got %f", config.SampleRate)
	}
}

func TestHotReloadManager_Health(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe := newMockProbe("test-probe", "1.0.0")
	manager.RegisterProbe("test-probe", probe)

	health := manager.GetHealth("test-probe")
	if health == nil {
		t.Fatal("Expected health, got nil")
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", health.Status)
	}
}

func TestHotReloadManager_GetAllHealth(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	probe1 := newMockProbe("probe-1", "1.0.0")
	probe2 := newMockProbe("probe-2", "2.0.0")

	manager.RegisterProbe("probe-1", probe1)
	manager.RegisterProbe("probe-2", probe2)

	allHealth := manager.GetAllHealth()
	if len(allHealth) != 2 {
		t.Errorf("Expected 2 health entries, got %d", len(allHealth))
	}
}

func TestHotReloadManager_VersionHistory(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	versionCounter := 0
	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		versionCounter++
		return newMockProbe("test-probe", "1.0." + string(rune('0'+versionCounter))), nil
	}

	manager.RegisterLoader("test-probe", loader)

	// Load multiple versions
	manager.Reload("test-probe", nil)
	manager.Reload("test-probe", nil)
	manager.Reload("test-probe", nil)

	history := manager.GetVersionHistory("test-probe")
	if len(history) != 3 {
		t.Errorf("Expected 3 versions in history, got %d", len(history))
	}
}

func TestHotReloadManager_Callbacks(t *testing.T) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	reloadCalled := false
	manager.SetOnReload(func(name string, version ProbeVersion) {
		reloadCalled = true
	})

	loader := func(config ProbeConfig) (ReloadableProbe, error) {
		return newMockProbe("test-probe", "1.0.0"), nil
	}

	manager.RegisterLoader("test-probe", loader)
	manager.Reload("test-probe", nil)

	if !reloadCalled {
		t.Error("Expected reload callback to be called")
	}
}

func TestDefaultProbeConfig(t *testing.T) {
	config := DefaultProbeConfig()

	if !config.Enabled {
		t.Error("Expected enabled by default")
	}

	if config.SampleRate != 1.0 {
		t.Errorf("Expected sample rate 1.0, got %f", config.SampleRate)
	}

	if config.BufferSize != 1000 {
		t.Errorf("Expected buffer size 1000, got %d", config.BufferSize)
	}
}

func BenchmarkHotReloadManager_GetHealth(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())
	probe := newMockProbe("bench-probe", "1.0.0")
	manager.RegisterProbe("bench-probe", probe)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetHealth("bench-probe")
	}
}

func BenchmarkHotReloadManager_GetAllHealth(b *testing.B) {
	manager := NewHotReloadManager(DefaultHotReloadConfig())

	for i := 0; i < 10; i++ {
		probe := newMockProbe("probe-"+string(rune('0'+i)), "1.0.0")
		manager.RegisterProbe("probe-"+string(rune('0'+i)), probe)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetAllHealth()
	}
}
