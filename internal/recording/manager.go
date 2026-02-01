package recording

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Manager runs recording rules in the background
type Manager struct {
	store     *Store
	evaluator *Evaluator

	// Scheduling state
	lastEval map[string]time.Time
	mu       sync.RWMutex

	// Statistics
	totalEvals    int64
	successEvals  int64
	failedEvals   int64
	lastDuration  time.Duration
	totalDuration time.Duration

	// Control
	stopCh    chan struct{}
	wg        sync.WaitGroup
	running   bool
	runningMu sync.Mutex

	// Configuration
	evalInterval   time.Duration // How often to check for rules to evaluate
	historyMaxAge  time.Duration // How long to keep evaluation history
	cleanupEnabled bool          // Whether to run periodic cleanup
}

// ManagerConfig holds configuration for the recording rule manager
type ManagerConfig struct {
	EvalInterval  time.Duration // Default evaluation check interval
	HistoryMaxAge time.Duration // Maximum age of evaluation history
	CleanupOnInit bool          // Run cleanup on initialization
}

// DefaultConfig returns default manager configuration
func DefaultConfig() ManagerConfig {
	return ManagerConfig{
		EvalInterval:  15 * time.Second, // Check every 15 seconds
		HistoryMaxAge: 7 * 24 * time.Hour, // Keep 7 days of history
		CleanupOnInit: true,
	}
}

// NewManager creates a new recording rule manager
func NewManager(store *Store, evaluator *Evaluator, config ManagerConfig) *Manager {
	m := &Manager{
		store:          store,
		evaluator:      evaluator,
		lastEval:       make(map[string]time.Time),
		stopCh:         make(chan struct{}),
		evalInterval:   config.EvalInterval,
		historyMaxAge:  config.HistoryMaxAge,
		cleanupEnabled: true,
	}

	// Ensure default rules exist
	if err := store.EnsureDefaultRules(); err != nil {
		log.Printf("[recording] Warning: could not create default rules: %v", err)
	}

	// Run cleanup on init if configured
	if config.CleanupOnInit {
		if deleted, err := store.CleanupHistory(config.HistoryMaxAge); err == nil && deleted > 0 {
			log.Printf("[recording] Cleaned up %d old evaluation history records", deleted)
		}
	}

	return m
}

// Start begins the background evaluation loop
func (m *Manager) Start() {
	m.runningMu.Lock()
	if m.running {
		m.runningMu.Unlock()
		return
	}
	m.running = true
	m.stopCh = make(chan struct{})
	m.runningMu.Unlock()

	m.wg.Add(1)
	go m.runLoop()

	// Start cleanup goroutine
	if m.cleanupEnabled {
		m.wg.Add(1)
		go m.runCleanup()
	}

	log.Println("[recording] Manager started")
}

// Stop gracefully stops the manager
func (m *Manager) Stop() {
	m.runningMu.Lock()
	if !m.running {
		m.runningMu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.runningMu.Unlock()

	m.wg.Wait()
	log.Println("[recording] Manager stopped")
}

// runLoop is the main evaluation loop
func (m *Manager) runLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.evalInterval)
	defer ticker.Stop()

	// Run initial evaluation
	m.evaluateAll()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.evaluateAll()
		}
	}
}

// evaluateAll evaluates all enabled rules that are due
func (m *Manager) evaluateAll() {
	rules, err := m.store.ListEnabledRules()
	if err != nil {
		log.Printf("[recording] Error listing rules: %v", err)
		return
	}

	for i := range rules {
		rule := &rules[i]

		// Check if it's time to evaluate this rule
		m.mu.RLock()
		lastEval := m.lastEval[rule.ID]
		m.mu.RUnlock()

		if time.Since(lastEval) < rule.Interval {
			continue
		}

		// Update last eval time
		m.mu.Lock()
		m.lastEval[rule.ID] = time.Now()
		m.mu.Unlock()

		// Evaluate the rule
		m.evaluateRule(rule)
	}
}

// evaluateRule evaluates a single rule
func (m *Manager) evaluateRule(rule *RecordingRule) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	result := m.evaluator.EvaluateAndStore(ctx, rule)
	duration := time.Since(start)

	// Update statistics
	atomic.AddInt64(&m.totalEvals, 1)
	if result.Error != nil {
		atomic.AddInt64(&m.failedEvals, 1)
	} else {
		atomic.AddInt64(&m.successEvals, 1)
	}

	m.mu.Lock()
	m.lastDuration = duration
	m.totalDuration += duration
	m.mu.Unlock()

	// Calculate aggregate value for logging
	var aggregateValue float64
	if len(result.Values) > 0 {
		for _, v := range result.Values {
			aggregateValue += v.Value
		}
		aggregateValue /= float64(len(result.Values))
	}

	// Update rule in store
	if result.Error != nil {
		if err := m.store.UpdateEvaluation(rule.ID, 0, result.Error); err != nil {
			log.Printf("[recording] Error updating rule %s: %v", rule.ID, err)
		}
		log.Printf("[recording] Rule %s failed: %v (took %v)", rule.Name, result.Error, duration)
	} else {
		if err := m.store.UpdateEvaluation(rule.ID, aggregateValue, nil); err != nil {
			log.Printf("[recording] Error updating rule %s: %v", rule.ID, err)
		}
	}

	// Record evaluation history
	history := &EvaluationHistory{
		ID:        uuid.New().String(),
		RuleID:    rule.ID,
		Timestamp: time.Now(),
		Value:     aggregateValue,
		Duration:  duration.Milliseconds(),
		Success:   result.Error == nil,
	}
	if result.Error != nil {
		history.Error = result.Error.Error()
	}
	if err := m.store.RecordEvaluation(history); err != nil {
		log.Printf("[recording] Error recording history for %s: %v", rule.ID, err)
	}
}

// EvaluateNow manually triggers evaluation of a specific rule
func (m *Manager) EvaluateNow(ruleID string) (*EvaluationResult, error) {
	rule, err := m.store.GetRule(ruleID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := m.evaluator.EvaluateAndStore(ctx, rule)

	// Calculate aggregate value
	var aggregateValue float64
	if len(result.Values) > 0 {
		for _, v := range result.Values {
			aggregateValue += v.Value
		}
		aggregateValue /= float64(len(result.Values))
	}

	// Update rule in store
	if err := m.store.UpdateEvaluation(rule.ID, aggregateValue, result.Error); err != nil {
		log.Printf("[recording] Error updating rule %s: %v", rule.ID, err)
	}

	// Update statistics
	atomic.AddInt64(&m.totalEvals, 1)
	if result.Error != nil {
		atomic.AddInt64(&m.failedEvals, 1)
	} else {
		atomic.AddInt64(&m.successEvals, 1)
	}

	// Update last eval time
	m.mu.Lock()
	m.lastEval[rule.ID] = time.Now()
	m.mu.Unlock()

	// Record evaluation history
	history := &EvaluationHistory{
		ID:        uuid.New().String(),
		RuleID:    rule.ID,
		Timestamp: time.Now(),
		Value:     aggregateValue,
		Duration:  result.Duration.Milliseconds(),
		Success:   result.Error == nil,
	}
	if result.Error != nil {
		history.Error = result.Error.Error()
	}
	m.store.RecordEvaluation(history)

	return result, nil
}

// GetStats returns manager statistics
func (m *Manager) GetStats() ManagerStats {
	rules, _ := m.store.ListRules()
	enabledRules, _ := m.store.ListEnabledRules()

	m.mu.RLock()
	lastDuration := m.lastDuration
	totalDuration := m.totalDuration
	m.mu.RUnlock()

	totalEvals := atomic.LoadInt64(&m.totalEvals)
	var avgDuration time.Duration
	if totalEvals > 0 {
		avgDuration = totalDuration / time.Duration(totalEvals)
	}

	return ManagerStats{
		TotalRules:          len(rules),
		EnabledRules:        len(enabledRules),
		TotalEvaluations:    totalEvals,
		SuccessfulEvals:     atomic.LoadInt64(&m.successEvals),
		FailedEvals:         atomic.LoadInt64(&m.failedEvals),
		LastEvalDuration:    lastDuration,
		AverageEvalDuration: avgDuration,
	}
}

// runCleanup periodically cleans up old evaluation history
func (m *Manager) runCleanup() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			if deleted, err := m.store.CleanupHistory(m.historyMaxAge); err == nil && deleted > 0 {
				log.Printf("[recording] Cleaned up %d old evaluation history records", deleted)
			}
		}
	}
}

// IsRunning returns whether the manager is running
func (m *Manager) IsRunning() bool {
	m.runningMu.Lock()
	defer m.runningMu.Unlock()
	return m.running
}
