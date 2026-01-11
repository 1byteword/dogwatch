package slo

import (
	"log"
	"sync"
	"time"

	"dogwatch/internal/synthetics"
)

// IncidentTrigger interface for creating incidents from SLO breaches
type IncidentTrigger interface {
	TriggerFromSLO(sloID, sloName string, remaining float64) error
}

// Calculator evaluates SLOs against their data sources
type Calculator struct {
	store           *Store
	syntheticsStore *synthetics.Store
	pager           IncidentTrigger
	stopChan        chan struct{}
	wg              sync.WaitGroup

	// Cache of current states
	states   map[string]*SLOState
	statesMu sync.RWMutex

	// Track which SLOs have active incidents to avoid duplicates
	activeIncidents map[string]bool
	incidentsMu     sync.RWMutex

	// Track last budget percentage to detect threshold crossings
	lastBudgetPct map[string]float64
}

// NewCalculator creates a new SLO calculator
func NewCalculator(store *Store, syntheticsStore *synthetics.Store) *Calculator {
	return &Calculator{
		store:           store,
		syntheticsStore: syntheticsStore,
		stopChan:        make(chan struct{}),
		states:          make(map[string]*SLOState),
		activeIncidents: make(map[string]bool),
		lastBudgetPct:   make(map[string]float64),
	}
}

// SetPager sets the incident trigger for SLO breaches
func (c *Calculator) SetPager(pager IncidentTrigger) {
	c.incidentsMu.Lock()
	defer c.incidentsMu.Unlock()
	c.pager = pager
}

// Start begins periodic SLO calculation
func (c *Calculator) Start() {
	c.wg.Add(1)
	go c.runLoop()
	log.Println("[slo] Calculator started")
}

// Stop stops the calculator
func (c *Calculator) Stop() {
	close(c.stopChan)
	c.wg.Wait()
	log.Println("[slo] Calculator stopped")
}

func (c *Calculator) runLoop() {
	defer c.wg.Done()

	// Calculate immediately on start
	c.calculateAll()

	// Then every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Record snapshots every 5 minutes
	snapshotTicker := time.NewTicker(5 * time.Minute)
	defer snapshotTicker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.calculateAll()
		case <-snapshotTicker.C:
			c.recordSnapshots()
		}
	}
}

// calculateAll calculates state for all enabled SLOs
func (c *Calculator) calculateAll() {
	slos, err := c.store.ListEnabledSLOs()
	if err != nil {
		log.Printf("[slo] Failed to list SLOs: %v", err)
		return
	}

	for _, slo := range slos {
		state := c.Calculate(slo)
		c.statesMu.Lock()
		c.states[slo.ID] = state
		c.statesMu.Unlock()

		// Check for budget breach and trigger incident
		c.checkAndTriggerIncident(slo, state)
	}
}

// checkAndTriggerIncident triggers an incident when SLO budget crosses critical thresholds
func (c *Calculator) checkAndTriggerIncident(slo SLO, state *SLOState) {
	c.incidentsMu.Lock()
	pager := c.pager
	hasActiveIncident := c.activeIncidents[slo.ID]
	lastPct := c.lastBudgetPct[slo.ID]
	c.incidentsMu.Unlock()

	if pager == nil || state.ErrorBudget <= 0 {
		return
	}

	// Calculate budget percentage remaining
	budgetPct := (state.BudgetRemaining / state.ErrorBudget) * 100
	if budgetPct < 0 {
		budgetPct = 0
	}

	// Trigger incident when budget crosses below 25% or is exhausted
	shouldTrigger := false
	if !hasActiveIncident {
		if budgetPct <= 0 && lastPct > 0 {
			// Budget exhausted
			shouldTrigger = true
			log.Printf("[slo] %s: Error budget exhausted!", slo.Name)
		} else if budgetPct <= 25 && lastPct > 25 {
			// Budget critically low
			shouldTrigger = true
			log.Printf("[slo] %s: Error budget critically low (%.1f%% remaining)", slo.Name, budgetPct)
		}
	}

	// Update last budget percentage
	c.incidentsMu.Lock()
	c.lastBudgetPct[slo.ID] = budgetPct
	c.incidentsMu.Unlock()

	if shouldTrigger {
		if err := pager.TriggerFromSLO(slo.ID, slo.Name, state.BudgetRemaining); err != nil {
			log.Printf("[slo] Failed to trigger incident for %s: %v", slo.Name, err)
		} else {
			c.incidentsMu.Lock()
			c.activeIncidents[slo.ID] = true
			c.incidentsMu.Unlock()
			log.Printf("[slo] Triggered incident for SLO %s", slo.Name)
		}
	}

	// Clear active incident tracking if budget recovered above 50%
	if hasActiveIncident && budgetPct > 50 {
		c.incidentsMu.Lock()
		delete(c.activeIncidents, slo.ID)
		c.incidentsMu.Unlock()
		log.Printf("[slo] %s: Error budget recovered (%.1f%%), incident tracking cleared", slo.Name, budgetPct)
	}
}

// recordSnapshots records current state for all SLOs
func (c *Calculator) recordSnapshots() {
	c.statesMu.RLock()
	defer c.statesMu.RUnlock()

	for sloID, state := range c.states {
		snap := &SLOSnapshot{
			SLOID:           sloID,
			Timestamp:       time.Now(),
			CurrentValue:    state.CurrentValue,
			BudgetRemaining: state.BudgetRemaining,
			Status:          state.Status,
		}
		if err := c.store.RecordSnapshot(snap); err != nil {
			log.Printf("[slo] Failed to record snapshot for %s: %v", sloID, err)
		}
	}
}

// Calculate computes the current state of an SLO
func (c *Calculator) Calculate(slo SLO) *SLOState {
	state := &SLOState{
		SLOID:       slo.ID,
		Target:      slo.Target,
		Status:      StatusNoData,
		WindowEnd:   time.Now(),
		LastUpdated: time.Now(),
	}

	windowDuration := GetWindowDuration(slo.Window)
	state.WindowStart = state.WindowEnd.Add(-windowDuration)

	// Get data based on source type
	switch slo.Source.Type {
	case "synthetics":
		c.calculateFromSynthetics(slo, state)
	default:
		// Future: support metrics, logs, etc.
		log.Printf("[slo] Unknown source type: %s", slo.Source.Type)
	}

	return state
}

// calculateFromSynthetics calculates SLO from synthetic check results
func (c *Calculator) calculateFromSynthetics(slo SLO, state *SLOState) {
	if c.syntheticsStore == nil {
		return
	}

	checkID := slo.Source.ID
	windowDuration := GetWindowDuration(slo.Window)

	// Get uptime stats from synthetics
	stats, err := c.syntheticsStore.GetUptime(checkID, windowDuration)
	if err != nil {
		log.Printf("[slo] Failed to get synthetics uptime for %s: %v", checkID, err)
		return
	}

	if stats.TotalChecks == 0 {
		state.Status = StatusNoData
		return
	}

	state.TotalEvents = int64(stats.TotalChecks)
	state.GoodEvents = int64(stats.SuccessCount)
	state.BadEvents = int64(stats.FailureCount)

	switch slo.Type {
	case SLOAvailability:
		// Availability: percentage of successful checks
		state.CurrentValue = stats.UptimePercent
		c.calculateErrorBudget(slo, state)

	case SLOLatency:
		// Latency: P95 or P99 must be under threshold
		// For this, we need to check what percentage of requests meet the latency target
		if slo.Threshold > 0 {
			// Use P95 latency as the measured value
			state.CurrentValue = stats.P95LatencyMs
			// For latency SLOs, we compare against the threshold
			// If P95 is under threshold, we're meeting the SLO
			if stats.P95LatencyMs <= slo.Threshold {
				state.Status = StatusMet
			} else {
				state.Status = StatusBreached
			}
		}

	case SLOErrorRate:
		// Error rate: percentage of failures must be under target
		if stats.TotalChecks > 0 {
			errorRate := float64(stats.FailureCount) / float64(stats.TotalChecks) * 100
			state.CurrentValue = errorRate
			// For error rate, lower is better
			if errorRate <= (100 - slo.Target) {
				state.Status = StatusMet
			} else {
				state.Status = StatusBreached
			}
		}
	}
}

// calculateErrorBudget computes error budget for availability SLOs
func (c *Calculator) calculateErrorBudget(slo SLO, state *SLOState) {
	// Error budget = allowed failures based on target
	// If target is 99.9%, error budget is 0.1% of the window

	windowDuration := GetWindowDuration(slo.Window)
	windowMinutes := windowDuration.Minutes()

	// Total allowed downtime in minutes
	allowedErrorPct := 100 - slo.Target
	state.ErrorBudget = windowMinutes * (allowedErrorPct / 100)

	// Calculate actual downtime
	if state.TotalEvents > 0 {
		actualErrorPct := float64(state.BadEvents) / float64(state.TotalEvents) * 100
		usedBudgetMinutes := windowMinutes * (actualErrorPct / 100)

		state.BudgetRemaining = state.ErrorBudget - usedBudgetMinutes
		if state.ErrorBudget > 0 {
			state.BudgetUsedPct = (usedBudgetMinutes / state.ErrorBudget) * 100
		}

		// Calculate burn rate
		// Burn rate = actual error rate / allowed error rate
		// 1.0 = burning at expected rate
		// >1.0 = burning faster than expected
		if allowedErrorPct > 0 {
			state.BurnRate = actualErrorPct / allowedErrorPct
		}

		// Determine status
		if state.CurrentValue >= slo.Target {
			if state.BurnRate > 2.0 {
				state.Status = StatusAtRisk // Meeting target but burning fast
			} else {
				state.Status = StatusMet
			}
		} else {
			state.Status = StatusBreached
		}
	}
}

// GetState returns the current state for an SLO
func (c *Calculator) GetState(sloID string) *SLOState {
	c.statesMu.RLock()
	defer c.statesMu.RUnlock()

	if state, ok := c.states[sloID]; ok {
		return state
	}
	return nil
}

// GetAllStates returns states for all SLOs
func (c *Calculator) GetAllStates() map[string]*SLOState {
	c.statesMu.RLock()
	defer c.statesMu.RUnlock()

	result := make(map[string]*SLOState)
	for k, v := range c.states {
		result[k] = v
	}
	return result
}

// ForceCalculate triggers immediate recalculation for an SLO
func (c *Calculator) ForceCalculate(sloID string) *SLOState {
	slo, err := c.store.GetSLO(sloID)
	if err != nil || slo == nil {
		return nil
	}

	state := c.Calculate(*slo)
	c.statesMu.Lock()
	c.states[sloID] = state
	c.statesMu.Unlock()

	return state
}

// SLOWithState combines an SLO definition with its current state
type SLOWithState struct {
	SLO   SLO       `json:"slo"`
	State *SLOState `json:"state"`
}

// GetSLOsWithState returns all SLOs with their current states
func (c *Calculator) GetSLOsWithState() ([]SLOWithState, error) {
	slos, err := c.store.ListSLOs()
	if err != nil {
		return nil, err
	}

	c.statesMu.RLock()
	defer c.statesMu.RUnlock()

	result := make([]SLOWithState, len(slos))
	for i, slo := range slos {
		result[i] = SLOWithState{
			SLO:   slo,
			State: c.states[slo.ID],
		}
	}
	return result, nil
}
