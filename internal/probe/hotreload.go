package probe

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// ProbeVersion tracks a probe's version and metadata
type ProbeVersion struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	LoadedAt    time.Time `json:"loaded_at"`
	Checksum    string    `json:"checksum,omitempty"`
	Description string    `json:"description,omitempty"`
}

// ProbeHealth represents the health status of a probe
type ProbeHealth struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Status      string    `json:"status"` // "healthy", "degraded", "failed"
	LastCheck   time.Time `json:"last_check"`
	EventsTotal int64     `json:"events_total"`
	EventsRate  float64   `json:"events_per_second"`
	ErrorCount  int64     `json:"error_count"`
	Message     string    `json:"message,omitempty"`
}

// ProbeConfig represents configuration for a probe
type ProbeConfig struct {
	Enabled          bool              `json:"enabled"`
	SampleRate       float64           `json:"sample_rate"` // 0.0 to 1.0
	BufferSize       int               `json:"buffer_size"`
	Filters          map[string]string `json:"filters,omitempty"`
	CustomSettings   map[string]string `json:"custom_settings,omitempty"`
	MaxEventsPerSec  int64             `json:"max_events_per_sec"`
	DrainTimeoutSecs int               `json:"drain_timeout_secs"`
}

// DefaultProbeConfig returns default probe configuration
func DefaultProbeConfig() ProbeConfig {
	return ProbeConfig{
		Enabled:          true,
		SampleRate:       1.0,
		BufferSize:       1000,
		MaxEventsPerSec:  100000,
		DrainTimeoutSecs: 5,
	}
}

// ReloadableProbe defines the interface for probes that support hot reload
type ReloadableProbe interface {
	Name() string
	Version() ProbeVersion
	Health() ProbeHealth
	Config() ProbeConfig
	UpdateConfig(ProbeConfig) error
	Close() error
}

// ProbeLoader is a function that loads a new probe instance
type ProbeLoader func(config ProbeConfig) (ReloadableProbe, error)

// HotReloadManager manages hot reloading of eBPF probes
type HotReloadManager struct {
	mu sync.RWMutex

	// Registered probe loaders
	loaders map[string]ProbeLoader

	// Active probes by name
	probes map[string]ReloadableProbe

	// Probe configurations
	configs map[string]ProbeConfig

	// Version history for rollback
	versionHistory map[string][]ProbeVersion

	// Statistics
	reloadCount   int64
	rollbackCount int64
	failureCount  int64

	// Health check interval
	healthInterval time.Duration
	healthStop     chan struct{}

	// Callbacks
	onReload   func(name string, version ProbeVersion)
	onRollback func(name string, from, to ProbeVersion, err error)
	onHealth   func(name string, health ProbeHealth)
}

// HotReloadConfig configures the hot reload manager
type HotReloadConfig struct {
	HealthCheckInterval time.Duration
	MaxVersionHistory   int
}

// DefaultHotReloadConfig returns sensible defaults
func DefaultHotReloadConfig() HotReloadConfig {
	return HotReloadConfig{
		HealthCheckInterval: 10 * time.Second,
		MaxVersionHistory:   10,
	}
}

// NewHotReloadManager creates a new hot reload manager
func NewHotReloadManager(config HotReloadConfig) *HotReloadManager {
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 10 * time.Second
	}
	if config.MaxVersionHistory == 0 {
		config.MaxVersionHistory = 10
	}

	return &HotReloadManager{
		loaders:        make(map[string]ProbeLoader),
		probes:         make(map[string]ReloadableProbe),
		configs:        make(map[string]ProbeConfig),
		versionHistory: make(map[string][]ProbeVersion),
		healthInterval: config.HealthCheckInterval,
		healthStop:     make(chan struct{}),
	}
}

// RegisterLoader registers a probe loader function
func (m *HotReloadManager) RegisterLoader(name string, loader ProbeLoader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.loaders[name] = loader
	m.configs[name] = DefaultProbeConfig()
}

// RegisterProbe registers an already-loaded probe
func (m *HotReloadManager) RegisterProbe(name string, probe ReloadableProbe) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.probes[name] = probe
	m.configs[name] = probe.Config()

	// Record initial version
	version := probe.Version()
	m.versionHistory[name] = append(m.versionHistory[name], version)
}

// Start begins health monitoring
func (m *HotReloadManager) Start() {
	go m.healthCheckLoop()
	log.Printf("[hotreload] Manager started with %d probes", len(m.probes))
}

// Stop halts health monitoring
func (m *HotReloadManager) Stop() {
	close(m.healthStop)
}

// healthCheckLoop periodically checks probe health
func (m *HotReloadManager) healthCheckLoop() {
	ticker := time.NewTicker(m.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkAllHealth()
		case <-m.healthStop:
			return
		}
	}
}

// checkAllHealth checks health of all probes
func (m *HotReloadManager) checkAllHealth() {
	m.mu.RLock()
	names := make([]string, 0, len(m.probes))
	for name := range m.probes {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		health := m.GetHealth(name)
		if health != nil && m.onHealth != nil {
			m.onHealth(name, *health)
		}
	}
}

// Reload reloads a probe with new configuration
func (m *HotReloadManager) Reload(name string, config *ProbeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	loader, ok := m.loaders[name]
	if !ok {
		return fmt.Errorf("no loader registered for probe: %s", name)
	}

	oldProbe := m.probes[name]
	var oldVersion ProbeVersion
	if oldProbe != nil {
		oldVersion = oldProbe.Version()
	}

	// Use provided config or existing config
	useConfig := m.configs[name]
	if config != nil {
		useConfig = *config
	}

	// Load new probe
	newProbe, err := loader(useConfig)
	if err != nil {
		atomic.AddInt64(&m.failureCount, 1)
		return fmt.Errorf("loading new probe: %w", err)
	}

	// Verify new probe is healthy before swap
	health := newProbe.Health()
	if health.Status == "failed" {
		newProbe.Close()
		atomic.AddInt64(&m.failureCount, 1)
		return fmt.Errorf("new probe failed health check: %s", health.Message)
	}

	// Drain old probe gracefully
	if oldProbe != nil {
		drainTimeout := time.Duration(useConfig.DrainTimeoutSecs) * time.Second
		if drainTimeout == 0 {
			drainTimeout = 5 * time.Second
		}

		// Give time for in-flight events to complete
		time.Sleep(drainTimeout)

		// Close old probe
		if err := oldProbe.Close(); err != nil {
			log.Printf("[hotreload] Warning: error closing old probe %s: %v", name, err)
		}
	}

	// Swap to new probe
	m.probes[name] = newProbe
	m.configs[name] = useConfig

	// Record version history
	newVersion := newProbe.Version()
	m.versionHistory[name] = append(m.versionHistory[name], newVersion)

	// Trim history if needed
	if len(m.versionHistory[name]) > 10 {
		m.versionHistory[name] = m.versionHistory[name][1:]
	}

	atomic.AddInt64(&m.reloadCount, 1)

	if m.onReload != nil {
		m.onReload(name, newVersion)
	}

	log.Printf("[hotreload] Reloaded probe %s: %s -> %s", name, oldVersion.Version, newVersion.Version)
	return nil
}

// Rollback reverts to the previous probe version
func (m *HotReloadManager) Rollback(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	history := m.versionHistory[name]
	if len(history) < 2 {
		return errors.New("no previous version to rollback to")
	}

	currentProbe := m.probes[name]
	if currentProbe == nil {
		return fmt.Errorf("probe %s not loaded", name)
	}

	currentVersion := currentProbe.Version()
	previousVersion := history[len(history)-2]

	loader, ok := m.loaders[name]
	if !ok {
		return fmt.Errorf("no loader registered for probe: %s", name)
	}

	// Load previous version
	config := m.configs[name]
	newProbe, err := loader(config)
	if err != nil {
		atomic.AddInt64(&m.failureCount, 1)
		if m.onRollback != nil {
			m.onRollback(name, currentVersion, previousVersion, err)
		}
		return fmt.Errorf("rollback failed: %w", err)
	}

	// Swap probes
	if err := currentProbe.Close(); err != nil {
		log.Printf("[hotreload] Warning: error closing current probe during rollback: %v", err)
	}

	m.probes[name] = newProbe
	m.versionHistory[name] = history[:len(history)-1]

	atomic.AddInt64(&m.rollbackCount, 1)

	if m.onRollback != nil {
		m.onRollback(name, currentVersion, previousVersion, nil)
	}

	log.Printf("[hotreload] Rolled back probe %s: %s -> %s", name, currentVersion.Version, previousVersion.Version)
	return nil
}

// UpdateConfig updates a probe's configuration without full reload
func (m *HotReloadManager) UpdateConfig(name string, config ProbeConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	probe, ok := m.probes[name]
	if !ok {
		return fmt.Errorf("probe not found: %s", name)
	}

	if err := probe.UpdateConfig(config); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	m.configs[name] = config
	log.Printf("[hotreload] Updated config for probe %s", name)
	return nil
}

// GetHealth returns health status for a probe
func (m *HotReloadManager) GetHealth(name string) *ProbeHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[name]
	if !ok {
		return nil
	}

	health := probe.Health()
	return &health
}

// GetAllHealth returns health status for all probes
func (m *HotReloadManager) GetAllHealth() map[string]ProbeHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ProbeHealth)
	for name, probe := range m.probes {
		result[name] = probe.Health()
	}
	return result
}

// GetVersion returns the current version of a probe
func (m *HotReloadManager) GetVersion(name string) *ProbeVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[name]
	if !ok {
		return nil
	}

	version := probe.Version()
	return &version
}

// GetVersionHistory returns version history for a probe
func (m *HotReloadManager) GetVersionHistory(name string) []ProbeVersion {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := m.versionHistory[name]
	if history == nil {
		return nil
	}

	// Return a copy
	result := make([]ProbeVersion, len(history))
	copy(result, history)
	return result
}

// GetConfig returns the current config for a probe
func (m *HotReloadManager) GetConfig(name string) *ProbeConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, ok := m.configs[name]
	if !ok {
		return nil
	}
	return &config
}

// ListProbes returns names of all registered probes
func (m *HotReloadManager) ListProbes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.probes))
	for name := range m.probes {
		names = append(names, name)
	}
	return names
}

// SetOnReload sets callback for reload events
func (m *HotReloadManager) SetOnReload(fn func(name string, version ProbeVersion)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onReload = fn
}

// SetOnRollback sets callback for rollback events
func (m *HotReloadManager) SetOnRollback(fn func(name string, from, to ProbeVersion, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRollback = fn
}

// SetOnHealth sets callback for health check events
func (m *HotReloadManager) SetOnHealth(fn func(name string, health ProbeHealth)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onHealth = fn
}

// Stats returns reload statistics
func (m *HotReloadManager) Stats() HotReloadStats {
	return HotReloadStats{
		ReloadCount:   atomic.LoadInt64(&m.reloadCount),
		RollbackCount: atomic.LoadInt64(&m.rollbackCount),
		FailureCount:  atomic.LoadInt64(&m.failureCount),
		ProbeCount:    len(m.probes),
	}
}

// HotReloadStats holds reload statistics
type HotReloadStats struct {
	ReloadCount   int64 `json:"reload_count"`
	RollbackCount int64 `json:"rollback_count"`
	FailureCount  int64 `json:"failure_count"`
	ProbeCount    int   `json:"probe_count"`
}

// ProbeInfo provides comprehensive info about a probe
type ProbeInfo struct {
	Name           string         `json:"name"`
	Version        ProbeVersion   `json:"version"`
	Config         ProbeConfig    `json:"config"`
	Health         ProbeHealth    `json:"health"`
	VersionHistory []ProbeVersion `json:"version_history"`
}

// GetProbeInfo returns comprehensive info about a probe
func (m *HotReloadManager) GetProbeInfo(name string) *ProbeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	probe, ok := m.probes[name]
	if !ok {
		return nil
	}

	return &ProbeInfo{
		Name:           name,
		Version:        probe.Version(),
		Config:         probe.Config(),
		Health:         probe.Health(),
		VersionHistory: m.versionHistory[name],
	}
}

// MarshalJSON for stats serialization
func (s HotReloadStats) MarshalJSON() ([]byte, error) {
	type Alias HotReloadStats
	return json.Marshal(Alias(s))
}
