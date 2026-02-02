package sampling

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"dogwatch/internal/trace"

	_ "modernc.org/sqlite"
)

// Manager coordinates multiple sampling strategies
type Manager struct {
	config Config

	headSampler     *HeadSampler
	tailSampler     *TailSampler
	adaptiveSampler *AdaptiveSampler

	// Trace store for persisting kept traces (from tail sampling)
	traceStore *trace.Store

	// Database for persisting configuration and rules
	db *sql.DB

	// Stats aggregation
	totalProcessed int64
	totalKept      int64
	totalDropped   int64

	mu      sync.RWMutex
	started bool
}

// NewManager creates a new sampling manager
func NewManager(dbPath string, traceStore *trace.Store) (*Manager, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sampling db: %w", err)
	}

	m := &Manager{
		config:     DefaultConfig(),
		traceStore: traceStore,
		db:         db,
	}

	if err := m.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	// Load persisted configuration
	if err := m.loadConfig(); err != nil {
		log.Printf("[sampling] Warning: could not load config: %v", err)
	}

	return m, nil
}

func (m *Manager) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS sampling_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			config TEXT NOT NULL,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sampling_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			enabled INTEGER DEFAULT 1,
			priority INTEGER DEFAULT 0,
			condition TEXT NOT NULL,
			action INTEGER NOT NULL,
			sample_rate REAL DEFAULT 1.0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sampling_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			total_processed INTEGER,
			total_kept INTEGER,
			total_dropped INTEGER,
			head_stats TEXT,
			tail_stats TEXT,
			adaptive_stats TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sampling_stats_time ON sampling_stats(timestamp)`,
	}

	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Start initializes and starts all enabled samplers
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("sampling manager already started")
	}

	// Initialize head sampler
	if m.config.HeadSamplerConfig.Enabled {
		m.headSampler = NewHeadSampler(m.config.HeadSamplerConfig)
		log.Printf("[sampling] Head sampler started with %d rules", len(m.config.HeadSamplerConfig.Rules))
	}

	// Initialize tail sampler (mutually exclusive with head for now)
	if m.config.TailSamplerConfig.Enabled && !m.config.HeadSamplerConfig.Enabled {
		m.tailSampler = NewTailSampler(m.config.TailSamplerConfig)

		// Wire up trace persistence
		m.tailSampler.SetOnKeep(func(spans []trace.Span) {
			if m.traceStore != nil {
				for _, span := range spans {
					m.traceStore.RecordSpan(span)
				}
			}
		})

		log.Printf("[sampling] Tail sampler started (buffer timeout: %v)", m.config.TailSamplerConfig.BufferTimeout)
	}

	// Initialize adaptive sampler
	if m.config.AdaptiveSamplerConfig.Enabled {
		m.adaptiveSampler = NewAdaptiveSampler(m.config.AdaptiveSamplerConfig)
		log.Printf("[sampling] Adaptive sampler started (target TPS: %.1f)", m.config.AdaptiveSamplerConfig.TargetTracesPerSecond)
	}

	m.started = true

	// Start stats collection goroutine
	go m.statsLoop()

	return nil
}

// Stop gracefully stops all samplers
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	if m.headSampler != nil {
		m.headSampler.Stop()
		m.headSampler = nil
	}

	if m.tailSampler != nil {
		m.tailSampler.Stop()
		m.tailSampler = nil
	}

	if m.adaptiveSampler != nil {
		m.adaptiveSampler.Stop()
		m.adaptiveSampler = nil
	}

	m.started = false
	log.Printf("[sampling] Manager stopped")
	return nil
}

// ProcessSpan applies sampling to a span and returns the decision
func (m *Manager) ProcessSpan(span *trace.Span) Decision {
	if !m.config.Enabled {
		return DecisionKeep
	}

	atomic.AddInt64(&m.totalProcessed, 1)

	// Get decision from active samplers
	decision := m.getSamplingDecision(span)

	if decision == DecisionKeep {
		atomic.AddInt64(&m.totalKept, 1)
	} else if decision == DecisionDrop {
		atomic.AddInt64(&m.totalDropped, 1)
	}

	return decision
}

// getSamplingDecision applies the sampling chain
func (m *Manager) getSamplingDecision(span *trace.Span) Decision {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If tail sampling is enabled, buffer the span
	if m.tailSampler != nil {
		return m.tailSampler.ShouldSample(span)
	}

	// Apply head sampling
	if m.headSampler != nil {
		decision := m.headSampler.ShouldSample(span)
		if decision == DecisionDrop {
			return DecisionDrop
		}
	}

	// Apply adaptive sampling (if head kept it or no head sampler)
	if m.adaptiveSampler != nil {
		return m.adaptiveSampler.ShouldSample(span)
	}

	return DecisionKeep
}

// GetStats returns aggregated stats from all samplers
func (m *Manager) GetStats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := ManagerStats{
		TotalProcessed: atomic.LoadInt64(&m.totalProcessed),
		TotalKept:      atomic.LoadInt64(&m.totalKept),
		TotalDropped:   atomic.LoadInt64(&m.totalDropped),
	}

	if m.headSampler != nil {
		stats.HeadStats = m.headSampler.GetStats()
	}

	if m.tailSampler != nil {
		stats.TailStats = m.tailSampler.GetStats()
		stats.BufferedTraces = m.tailSampler.GetBufferedTraceCount()
	}

	if m.adaptiveSampler != nil {
		stats.AdaptiveStats = m.adaptiveSampler.GetStats()
		stats.CurrentRate = m.adaptiveSampler.GetCurrentRate()
		stats.ServiceRates = m.adaptiveSampler.GetServiceRates()
	}

	return stats
}

// ManagerStats provides aggregated sampling statistics
type ManagerStats struct {
	TotalProcessed int64              `json:"total_processed"`
	TotalKept      int64              `json:"total_kept"`
	TotalDropped   int64              `json:"total_dropped"`
	HeadStats      SamplerStats       `json:"head_stats,omitempty"`
	TailStats      SamplerStats       `json:"tail_stats,omitempty"`
	AdaptiveStats  SamplerStats       `json:"adaptive_stats,omitempty"`
	BufferedTraces int64              `json:"buffered_traces,omitempty"`
	CurrentRate    float64            `json:"current_rate,omitempty"`
	ServiceRates   map[string]float64 `json:"service_rates,omitempty"`
}

// UpdateConfig updates the sampling configuration
func (m *Manager) UpdateConfig(config Config) error {
	m.mu.Lock()
	m.config = config
	m.mu.Unlock()

	// Save to database
	if err := m.saveConfig(); err != nil {
		return err
	}

	// Restart samplers with new config
	if m.started {
		m.Stop()
		return m.Start()
	}

	return nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// saveConfig persists the configuration to the database
func (m *Manager) saveConfig() error {
	m.mu.RLock()
	configJSON, err := json.Marshal(m.config)
	m.mu.RUnlock()

	if err != nil {
		return err
	}

	return m.saveConfigJSON(configJSON)
}

// saveConfigLocked saves config when lock is already held (avoids deadlock)
func (m *Manager) saveConfigLocked() error {
	configJSON, err := json.Marshal(m.config)
	if err != nil {
		return err
	}
	return m.saveConfigJSON(configJSON)
}

func (m *Manager) saveConfigJSON(configJSON []byte) error {
	_, err := m.db.Exec(`
		INSERT INTO sampling_config (id, config, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			config = excluded.config,
			updated_at = excluded.updated_at
	`, string(configJSON), time.Now().UTC().Format(time.RFC3339))

	return err
}

// loadConfig loads the configuration from the database
func (m *Manager) loadConfig() error {
	var configJSON string
	err := m.db.QueryRow(`SELECT config FROM sampling_config WHERE id = 1`).Scan(&configJSON)
	if err == sql.ErrNoRows {
		return nil // No saved config, use defaults
	}
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return json.Unmarshal([]byte(configJSON), &m.config)
}

// AddRule adds a new sampling rule to the head sampler
func (m *Manager) AddRule(rule Rule) error {
	m.mu.Lock()
	if m.headSampler != nil {
		m.headSampler.AddRule(rule)
	}
	m.config.HeadSamplerConfig.Rules = append(m.config.HeadSamplerConfig.Rules, rule)
	m.mu.Unlock()

	return m.saveRule(rule)
}

// UpdateRule updates an existing sampling rule
func (m *Manager) UpdateRule(rule Rule) error {
	m.mu.Lock()
	if m.headSampler != nil {
		m.headSampler.UpdateRule(rule)
	}

	// Update in config
	for i, r := range m.config.HeadSamplerConfig.Rules {
		if r.ID == rule.ID {
			m.config.HeadSamplerConfig.Rules[i] = rule
			break
		}
	}
	m.mu.Unlock()

	return m.saveRule(rule)
}

// RemoveRule removes a sampling rule
func (m *Manager) RemoveRule(ruleID string) error {
	m.mu.Lock()
	if m.headSampler != nil {
		m.headSampler.RemoveRule(ruleID)
	}

	// Remove from config
	var newRules []Rule
	for _, r := range m.config.HeadSamplerConfig.Rules {
		if r.ID != ruleID {
			newRules = append(newRules, r)
		}
	}
	m.config.HeadSamplerConfig.Rules = newRules
	m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM sampling_rules WHERE id = ?`, ruleID)
	return err
}

// GetRules returns all sampling rules
func (m *Manager) GetRules() []Rule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Rule{}, m.config.HeadSamplerConfig.Rules...)
}

// saveRule persists a rule to the database
func (m *Manager) saveRule(rule Rule) error {
	conditionJSON, err := json.Marshal(rule.Condition)
	if err != nil {
		return err
	}

	_, err = m.db.Exec(`
		INSERT INTO sampling_rules (id, name, description, enabled, priority, condition, action, sample_rate, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			enabled = excluded.enabled,
			priority = excluded.priority,
			condition = excluded.condition,
			action = excluded.action,
			sample_rate = excluded.sample_rate,
			updated_at = excluded.updated_at
	`,
		rule.ID,
		rule.Name,
		rule.Description,
		rule.Enabled,
		rule.Priority,
		string(conditionJSON),
		rule.Action,
		rule.SampleRate,
		rule.CreatedAt.UTC().Format(time.RFC3339),
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// statsLoop periodically saves stats to the database
func (m *Manager) statsLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.saveStats()
		case <-m.stopCh():
			return
		}
	}
}

func (m *Manager) stopCh() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		for {
			m.mu.RLock()
			started := m.started
			m.mu.RUnlock()
			if !started {
				close(ch)
				return
			}
			time.Sleep(time.Second)
		}
	}()
	return ch
}

// saveStats persists current stats to the database
func (m *Manager) saveStats() {
	stats := m.GetStats()

	headJSON, _ := json.Marshal(stats.HeadStats)
	tailJSON, _ := json.Marshal(stats.TailStats)
	adaptiveJSON, _ := json.Marshal(stats.AdaptiveStats)

	m.db.Exec(`
		INSERT INTO sampling_stats (timestamp, total_processed, total_kept, total_dropped, head_stats, tail_stats, adaptive_stats)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		time.Now().UTC().Format(time.RFC3339),
		stats.TotalProcessed,
		stats.TotalKept,
		stats.TotalDropped,
		string(headJSON),
		string(tailJSON),
		string(adaptiveJSON),
	)

	// Clean up old stats (keep last 24 hours)
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	m.db.Exec(`DELETE FROM sampling_stats WHERE timestamp < ?`, cutoff)
}

// GetHistoricalStats returns historical sampling statistics
func (m *Manager) GetHistoricalStats(since time.Duration) ([]HistoricalStats, error) {
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	rows, err := m.db.Query(`
		SELECT timestamp, total_processed, total_kept, total_dropped, head_stats, tail_stats, adaptive_stats
		FROM sampling_stats
		WHERE timestamp > ?
		ORDER BY timestamp ASC
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HistoricalStats
	for rows.Next() {
		var hs HistoricalStats
		var timestamp, headJSON, tailJSON, adaptiveJSON string

		if err := rows.Scan(&timestamp, &hs.TotalProcessed, &hs.TotalKept, &hs.TotalDropped, &headJSON, &tailJSON, &adaptiveJSON); err != nil {
			continue
		}

		hs.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		json.Unmarshal([]byte(headJSON), &hs.HeadStats)
		json.Unmarshal([]byte(tailJSON), &hs.TailStats)
		json.Unmarshal([]byte(adaptiveJSON), &hs.AdaptiveStats)

		results = append(results, hs)
	}

	return results, nil
}

// HistoricalStats represents a point-in-time snapshot of sampling stats
type HistoricalStats struct {
	Timestamp      time.Time    `json:"timestamp"`
	TotalProcessed int64        `json:"total_processed"`
	TotalKept      int64        `json:"total_kept"`
	TotalDropped   int64        `json:"total_dropped"`
	HeadStats      SamplerStats `json:"head_stats,omitempty"`
	TailStats      SamplerStats `json:"tail_stats,omitempty"`
	AdaptiveStats  SamplerStats `json:"adaptive_stats,omitempty"`
}

// Close closes the database connection
func (m *Manager) Close() error {
	m.Stop()
	return m.db.Close()
}

// SetAdaptiveTargetTPS updates the adaptive sampler's target TPS
func (m *Manager) SetAdaptiveTargetTPS(targetTPS float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.adaptiveSampler != nil {
		config := m.adaptiveSampler.GetConfig()
		config.TargetTracesPerSecond = targetTPS
		m.adaptiveSampler.UpdateConfig(config)
	}

	m.config.AdaptiveSamplerConfig.TargetTracesPerSecond = targetTPS
	m.saveConfigLocked()
}

// SetServiceSampleRate sets a manual sample rate for a specific service
func (m *Manager) SetServiceSampleRate(service string, rate float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.adaptiveSampler != nil {
		m.adaptiveSampler.SetServiceRate(service, rate)
	}
}

// AddTailPriorityService adds a service to the tail sampler's priority list
func (m *Manager) AddTailPriorityService(service string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tailSampler != nil {
		m.tailSampler.AddPriorityService(service)
	}

	// Update config
	m.config.TailSamplerConfig.PriorityServices = append(
		m.config.TailSamplerConfig.PriorityServices,
		service,
	)
	m.saveConfigLocked()
}

// GetAdaptiveRateStats returns statistics about adaptive rate changes
func (m *Manager) GetAdaptiveRateStats() *RateStatistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.adaptiveSampler == nil {
		return nil
	}

	stats := m.adaptiveSampler.GetRateStatistics()
	return &stats
}
