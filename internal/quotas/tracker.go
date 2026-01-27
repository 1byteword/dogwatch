package quotas

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Tracker monitors resource usage and enforces quotas in real-time
type Tracker struct {
	store *Store
	mu    sync.RWMutex

	// In-memory usage counters for fast tracking (flushed periodically)
	counters   map[string]*usageCounter
	countersMu sync.Mutex

	// Alert callbacks
	onWarning   func(status *QuotaStatus)
	onExceeded  func(status *QuotaStatus)
	onViolation func(violation *QuotaViolation)

	// Background flush
	flushInterval time.Duration
	stopCh        chan struct{}
	wg            sync.WaitGroup
}

type usageCounter struct {
	TeamID       string
	ResourceType ResourceType
	Unit         QuotaUnit
	Amount       int64
	LastFlush    time.Time
}

func counterKey(teamID string, resourceType ResourceType) string {
	return teamID + ":" + string(resourceType)
}

// NewTracker creates a new quota tracker
func NewTracker(store *Store) *Tracker {
	return &Tracker{
		store:         store,
		counters:      make(map[string]*usageCounter),
		flushInterval: 10 * time.Second,
		stopCh:        make(chan struct{}),
	}
}

// Start begins background processing
func (t *Tracker) Start() {
	t.wg.Add(1)
	go t.flushLoop()
}

// Stop halts background processing
func (t *Tracker) Stop() {
	close(t.stopCh)
	t.wg.Wait()
	// Final flush
	t.flush()
}

func (t *Tracker) flushLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.flush()
		case <-t.stopCh:
			return
		}
	}
}

func (t *Tracker) flush() {
	t.countersMu.Lock()
	counters := t.counters
	t.counters = make(map[string]*usageCounter)
	t.countersMu.Unlock()

	for _, counter := range counters {
		if counter.Amount > 0 {
			record := &UsageRecord{
				TeamID:       counter.TeamID,
				ResourceType: counter.ResourceType,
				Amount:       counter.Amount,
				Unit:         counter.Unit,
				Timestamp:    time.Now(),
			}
			if err := t.store.RecordUsage(record); err != nil {
				log.Printf("[quotas] flush error: %v", err)
			}
		}
	}
}

// SetWarningCallback sets the callback for quota warnings
func (t *Tracker) SetWarningCallback(fn func(*QuotaStatus)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onWarning = fn
}

// SetExceededCallback sets the callback for quota exceeded events
func (t *Tracker) SetExceededCallback(fn func(*QuotaStatus)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onExceeded = fn
}

// SetViolationCallback sets the callback for quota violations
func (t *Tracker) SetViolationCallback(fn func(*QuotaViolation)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onViolation = fn
}

// Track records resource usage and checks quotas
// Returns: shouldAllow, action to take
func (t *Tracker) Track(teamID string, resourceType ResourceType, amount int64, unit QuotaUnit, source string) (bool, EnforcementAction) {
	// Update in-memory counter
	key := counterKey(teamID, resourceType)
	t.countersMu.Lock()
	counter, exists := t.counters[key]
	if !exists {
		counter = &usageCounter{
			TeamID:       teamID,
			ResourceType: resourceType,
			Unit:         unit,
			LastFlush:    time.Now(),
		}
		t.counters[key] = counter
	}
	counter.Amount += amount
	currentAmount := counter.Amount
	t.countersMu.Unlock()

	// Check quotas
	quotas := t.store.GetQuotasForTeam(teamID)
	for _, quota := range quotas {
		if quota.ResourceType != resourceType && quota.ResourceType != ResourceAll {
			continue
		}
		if !quota.Enabled {
			continue
		}

		// Get current period usage
		usage, err := t.store.GetUsage(teamID, resourceType, quota.Period)
		if err != nil {
			log.Printf("[quotas] usage query error: %v", err)
			continue
		}

		// Include buffered amount
		totalUsage := usage.Current + currentAmount
		percentage := float64(totalUsage) / float64(quota.Limit) * 100

		status := &QuotaStatus{
			QuotaID:      quota.ID,
			TeamID:       teamID,
			ResourceType: resourceType,
			Current:      totalUsage,
			Limit:        quota.Limit,
			Percentage:   percentage,
			Enforcement:  quota.Enforcement,
		}

		// Check thresholds
		if percentage >= 100 {
			status.Status = "exceeded"
			status.Message = fmt.Sprintf("Quota exceeded: %.1f%% of limit", percentage)

			// Record violation
			violation := &QuotaViolation{
				ID:           fmt.Sprintf("v-%d", time.Now().UnixNano()),
				QuotaID:      quota.ID,
				TeamID:       teamID,
				ResourceType: resourceType,
				Usage:        totalUsage,
				Limit:        quota.Limit,
				Percentage:   percentage,
				Action:       quota.Enforcement,
				Timestamp:    time.Now(),
			}
			t.store.RecordViolation(violation)

			t.mu.RLock()
			if t.onExceeded != nil {
				go t.onExceeded(status)
			}
			if t.onViolation != nil {
				go t.onViolation(violation)
			}
			t.mu.RUnlock()

			// Apply enforcement
			switch quota.Enforcement {
			case ActionDrop:
				return false, ActionDrop
			case ActionSample:
				// Sample at 10% when exceeded
				if time.Now().UnixNano()%10 != 0 {
					return false, ActionSample
				}
			case ActionThrottle:
				// Delay but allow
				time.Sleep(100 * time.Millisecond)
			case ActionWarn:
				// Log but allow
			}
		} else if percentage >= quota.WarnAt*100 {
			status.Status = "warning"
			status.Message = fmt.Sprintf("Approaching quota: %.1f%% of limit", percentage)

			t.mu.RLock()
			if t.onWarning != nil {
				go t.onWarning(status)
			}
			t.mu.RUnlock()
		}
	}

	return true, ActionWarn
}

// TrackMetrics is a convenience method for tracking metrics usage
func (t *Tracker) TrackMetrics(teamID string, seriesCount int64, source string) (bool, EnforcementAction) {
	return t.Track(teamID, ResourceMetrics, seriesCount, UnitSeries, source)
}

// TrackLogs is a convenience method for tracking log usage
func (t *Tracker) TrackLogs(teamID string, bytes int64, source string) (bool, EnforcementAction) {
	return t.Track(teamID, ResourceLogs, bytes, UnitBytes, source)
}

// TrackTraces is a convenience method for tracking trace usage
func (t *Tracker) TrackTraces(teamID string, spanCount int64, source string) (bool, EnforcementAction) {
	return t.Track(teamID, ResourceTraces, spanCount, UnitSpans, source)
}

// TrackCustomMetrics is a convenience method for tracking custom metrics
func (t *Tracker) TrackCustomMetrics(teamID string, dataPoints int64, source string) (bool, EnforcementAction) {
	return t.Track(teamID, ResourceCustomMetrics, dataPoints, UnitEvents, source)
}

// GetStatus returns the current quota status for a team
func (t *Tracker) GetStatus(teamID string) []*QuotaStatus {
	quotas := t.store.GetQuotasForTeam(teamID)
	var statuses []*QuotaStatus

	for _, quota := range quotas {
		usage, err := t.store.GetUsage(teamID, quota.ResourceType, quota.Period)
		if err != nil {
			continue
		}

		status := CalculateQuotaStatus(quota, usage.Current)
		statuses = append(statuses, status)
	}

	return statuses
}

// CheckQuota checks if a team can use more of a resource
func (t *Tracker) CheckQuota(teamID string, resourceType ResourceType, additionalAmount int64) *QuotaStatus {
	quotas := t.store.GetQuotasForTeam(teamID)

	for _, quota := range quotas {
		if quota.ResourceType != resourceType && quota.ResourceType != ResourceAll {
			continue
		}
		if !quota.Enabled {
			continue
		}

		usage, err := t.store.GetUsage(teamID, resourceType, quota.Period)
		if err != nil {
			continue
		}

		projectedUsage := usage.Current + additionalAmount
		status := CalculateQuotaStatus(quota, projectedUsage)
		if status.Status != "ok" {
			return status
		}
	}

	return nil // All quotas OK
}

// GetTeamSummary returns a complete summary for a team
func (t *Tracker) GetTeamSummary(teamID string) (*TeamSummary, error) {
	team, err := t.store.GetTeam(teamID)
	if err != nil {
		return nil, err
	}
	if team == nil {
		return nil, fmt.Errorf("team not found: %s", teamID)
	}

	quotas, err := t.store.ListQuotas(teamID)
	if err != nil {
		return nil, err
	}

	usageStatus := t.GetStatus(teamID)

	services, _ := t.store.GetUsageByService(teamID, "monthly")

	summary := &TeamSummary{
		Team:        team,
		Quotas:      quotas,
		UsageStatus: usageStatus,
		TopServices: services,
	}

	// Calculate costs
	calculator := NewChargebackCalculator(t.store, DefaultPricing())
	start, end := GetPeriodBounds("monthly", time.Now())
	report, err := calculator.Calculate(teamID, "monthly", start, end)
	if err == nil && report != nil {
		summary.TotalCost = report.TotalCost
	}

	return summary, nil
}
