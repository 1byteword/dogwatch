package watch

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MetricsProvider interface for getting current metric values
type MetricsProvider interface {
	GetMetricValue(metric MetricType) (float64, bool)
}

// IncidentTrigger interface for creating incidents from watch alerts
type IncidentTrigger interface {
	TriggerFromWatch(watchID, watchName, message string, severity string) error
}

// WatchBroadcaster interface for broadcasting watch state changes via WebSocket
type WatchBroadcaster interface {
	BroadcastWatchState(watchID, state string, value float64)
}

// Engine evaluates watches and fires notifications
type Engine struct {
	store       *Store
	metrics     MetricsProvider
	notifier    *Notifier
	pager       IncidentTrigger
	broadcaster WatchBroadcaster

	// Track pending states for duration requirements
	pendingStart map[string]time.Time
	// Track which watches have active incidents to avoid duplicates
	activeIncidents map[string]bool
	mu              sync.RWMutex

	checkInterval time.Duration
	cancel        context.CancelFunc
}

// NewEngine creates a new watch evaluation engine
func NewEngine(store *Store, metrics MetricsProvider, notifier *Notifier) *Engine {
	return &Engine{
		store:           store,
		metrics:         metrics,
		notifier:        notifier,
		pendingStart:    make(map[string]time.Time),
		activeIncidents: make(map[string]bool),
		checkInterval:   30 * time.Second,
	}
}

// SetPager sets the incident trigger for creating incidents from alerts
func (e *Engine) SetPager(pager IncidentTrigger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pager = pager
}

// SetBroadcaster sets the WebSocket broadcaster for real-time updates
func (e *Engine) SetBroadcaster(broadcaster WatchBroadcaster) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.broadcaster = broadcaster
}

// Start begins the evaluation loop
func (e *Engine) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	go e.run(ctx)
	log.Printf("[watches] Engine started, checking every %v", e.checkInterval)
}

// Stop halts the evaluation loop
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

func (e *Engine) run(ctx context.Context) {
	ticker := time.NewTicker(e.checkInterval)
	defer ticker.Stop()

	// Initial check
	e.evaluateAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluateAll()
		}
	}
}

func (e *Engine) evaluateAll() {
	watches, err := e.store.ListWatches()
	if err != nil {
		log.Printf("[watches] Error listing watches: %v", err)
		return
	}

	for _, w := range watches {
		if !w.Enabled {
			continue
		}

		// Check if muted
		if w.MutedUntil != nil && time.Now().Before(*w.MutedUntil) {
			continue
		}

		e.evaluate(w)
	}
}

func (e *Engine) evaluate(w *Watch) {
	value, ok := e.metrics.GetMetricValue(w.Metric)

	now := time.Now()
	w.LastCheck = now
	w.LastValue = value

	var newState State
	if !ok {
		newState = StateNoData
	} else if e.checkThreshold(value, w.Operator, w.Threshold) {
		// Threshold breached - check duration
		newState = e.handleBreach(w, now)
	} else {
		// Threshold OK
		newState = StateOK
		e.clearPending(w.ID)
	}

	// Handle state transition
	if newState != w.State {
		e.transition(w, newState, value)
	}

	// Save updated watch
	if err := e.store.SaveWatch(w); err != nil {
		log.Printf("[watches] Error saving watch %s: %v", w.ID, err)
	}
}

func (e *Engine) checkThreshold(value float64, op Operator, threshold float64) bool {
	switch op {
	case OpGreaterThan:
		return value > threshold
	case OpGreaterOrEqual:
		return value >= threshold
	case OpLessThan:
		return value < threshold
	case OpLessOrEqual:
		return value <= threshold
	case OpEqual:
		return value == threshold
	case OpNotEqual:
		return value != threshold
	default:
		return false
	}
}

func (e *Engine) handleBreach(w *Watch, now time.Time) State {
	duration, err := time.ParseDuration(w.Duration)
	if err != nil {
		duration = 0
	}

	e.mu.Lock()
	start, pending := e.pendingStart[w.ID]
	if !pending {
		e.pendingStart[w.ID] = now
		e.mu.Unlock()
		if duration > 0 {
			return StatePending
		}
		return StateAlerting
	}
	e.mu.Unlock()

	// Check if we've exceeded the duration
	if now.Sub(start) >= duration {
		return StateAlerting
	}
	return StatePending
}

func (e *Engine) clearPending(watchID string) {
	e.mu.Lock()
	delete(e.pendingStart, watchID)
	e.mu.Unlock()
}

func (e *Engine) transition(w *Watch, newState State, value float64) {
	oldState := w.State
	w.State = newState
	w.StateAt = time.Now()

	// Clear pending if transitioning to OK or Alerting
	if newState == StateOK || newState == StateAlerting {
		e.clearPending(w.ID)
	}

	// Create event
	event := &Event{
		ID:        uuid.New().String(),
		WatchID:   w.ID,
		WatchName: w.Name,
		FromState: oldState,
		ToState:   newState,
		Value:     value,
		Threshold: w.Threshold,
		Message:   e.formatMessage(w, oldState, newState, value),
		Timestamp: time.Now(),
	}

	if err := e.store.RecordEvent(event); err != nil {
		log.Printf("[watches] Error recording event: %v", err)
	}

	// Notify on significant transitions
	if (newState == StateAlerting && oldState != StateAlerting) ||
		(newState == StateOK && oldState == StateAlerting) {
		e.notify(w, event)
	}

	log.Printf("[watches] %s: %s -> %s (value=%.2f, threshold=%.2f)",
		w.Name, oldState, newState, value, w.Threshold)

	// Broadcast state change via WebSocket
	if e.broadcaster != nil {
		e.broadcaster.BroadcastWatchState(w.ID, string(newState), value)
	}
}

func (e *Engine) formatMessage(w *Watch, from, to State, value float64) string {
	if to == StateAlerting {
		return fmt.Sprintf("%s is %s: %.2f %s %.2f",
			w.Name, "ALERTING", value, w.Operator, w.Threshold)
	}
	if to == StateOK && from == StateAlerting {
		return fmt.Sprintf("%s is %s: %.2f (threshold: %s %.2f)",
			w.Name, "RECOVERED", value, w.Operator, w.Threshold)
	}
	return fmt.Sprintf("%s changed to %s", w.Name, to)
}

func (e *Engine) notify(w *Watch, event *Event) {
	// Send to configured notification channels
	if e.notifier != nil && len(w.Channels) > 0 {
		for _, channelID := range w.Channels {
			channel, err := e.store.GetChannel(channelID)
			if err != nil {
				log.Printf("[watches] Channel %s not found: %v", channelID, err)
				continue
			}

			if err := e.notifier.Send(channel, w, event); err != nil {
				log.Printf("[watches] Failed to notify via %s: %v", channel.Name, err)
			}
		}
	}

	// Trigger incident on alerting, clear on recovery
	e.mu.Lock()
	pager := e.pager
	hasActiveIncident := e.activeIncidents[w.ID]
	e.mu.Unlock()

	if pager != nil {
		if event.ToState == StateAlerting && !hasActiveIncident {
			// Determine severity based on metric type or default to high
			severity := "high"
			if w.Metric == MetricCPU || w.Metric == MetricMemory {
				severity = "critical"
			}

			if err := pager.TriggerFromWatch(w.ID, w.Name, event.Message, severity); err != nil {
				log.Printf("[watches] Failed to trigger incident for %s: %v", w.Name, err)
			} else {
				e.mu.Lock()
				e.activeIncidents[w.ID] = true
				e.mu.Unlock()
				log.Printf("[watches] Triggered incident for watch %s", w.Name)
			}
		} else if event.ToState == StateOK && hasActiveIncident {
			// Clear the active incident tracking on recovery
			e.mu.Lock()
			delete(e.activeIncidents, w.ID)
			e.mu.Unlock()
			log.Printf("[watches] Watch %s recovered, incident tracking cleared", w.Name)
		}
	}
}

// ForceCheck triggers an immediate evaluation of all watches
func (e *Engine) ForceCheck() {
	go e.evaluateAll()
}
