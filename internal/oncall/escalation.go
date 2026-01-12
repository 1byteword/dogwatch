package oncall

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// EscalationState tracks the escalation state of an incident
type EscalationState struct {
	IncidentID     string         `json:"incident_id"`
	PolicyID       string         `json:"policy_id"`
	CurrentLevel   int            `json:"current_level"`
	RepeatCount    int            `json:"repeat_count"`
	StartedAt      time.Time      `json:"started_at"`
	LastEscalation time.Time      `json:"last_escalation"`
	Acknowledged   bool           `json:"acknowledged"`
	AckedBy        string         `json:"acked_by,omitempty"`
	AckedAt        *time.Time     `json:"acked_at,omitempty"`
	Resolved       bool           `json:"resolved"`
	Notifications  []Notification `json:"notifications"`
}

// Notification represents a sent notification
type Notification struct {
	ID        string    `json:"id"`
	Level     int       `json:"level"`
	Target    Target    `json:"target"`
	User      *User     `json:"user,omitempty"`
	Channel   string    `json:"channel"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"` // pending, sent, delivered, failed
	Message   string    `json:"message,omitempty"`
}

// NotificationChannel interface for sending notifications
type NotificationChannel interface {
	Name() string
	Send(user *User, subject, message string) error
}

// EscalationEngine manages incident escalation
type EscalationEngine struct {
	store      *Store
	calculator *Calculator
	channels   map[string]NotificationChannel
	states     map[string]*EscalationState // incidentID -> state
	mu         sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Callbacks
	onNotify func(incidentID string, user *User, level int)
	onEscalate func(incidentID string, fromLevel, toLevel int)
}

// NewEscalationEngine creates a new escalation engine
func NewEscalationEngine(store *Store, calculator *Calculator) *EscalationEngine {
	return &EscalationEngine{
		store:      store,
		calculator: calculator,
		channels:   make(map[string]NotificationChannel),
		states:     make(map[string]*EscalationState),
		stopCh:     make(chan struct{}),
	}
}

// RegisterChannel registers a notification channel
func (e *EscalationEngine) RegisterChannel(ch NotificationChannel) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.channels[ch.Name()] = ch
}

// SetOnNotify sets a callback for when notifications are sent
func (e *EscalationEngine) SetOnNotify(fn func(incidentID string, user *User, level int)) {
	e.onNotify = fn
}

// SetOnEscalate sets a callback for when escalation occurs
func (e *EscalationEngine) SetOnEscalate(fn func(incidentID string, fromLevel, toLevel int)) {
	e.onEscalate = fn
}

// Start starts the escalation engine background loop
func (e *EscalationEngine) Start() {
	e.wg.Add(1)
	go e.runLoop()
}

// Stop stops the escalation engine
func (e *EscalationEngine) Stop() {
	close(e.stopCh)
	e.wg.Wait()
}

// runLoop checks for escalations periodically
func (e *EscalationEngine) runLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.checkEscalations()
		}
	}
}

// checkEscalations checks all active incidents for escalation
func (e *EscalationEngine) checkEscalations() {
	e.mu.RLock()
	states := make([]*EscalationState, 0, len(e.states))
	for _, state := range e.states {
		if !state.Acknowledged && !state.Resolved {
			states = append(states, state)
		}
	}
	e.mu.RUnlock()

	for _, state := range states {
		e.checkEscalation(state)
	}
}

// checkEscalation checks if an incident needs to be escalated
func (e *EscalationEngine) checkEscalation(state *EscalationState) {
	policy, err := e.store.GetPolicy(state.PolicyID)
	if err != nil {
		return
	}

	// Find current rule
	var currentRule *EscalationRule
	for i := range policy.Rules {
		if policy.Rules[i].Level == state.CurrentLevel {
			currentRule = &policy.Rules[i]
			break
		}
	}

	if currentRule == nil {
		return
	}

	// Check if it's time to escalate
	escalateAfter := time.Duration(currentRule.DelayMinutes) * time.Minute
	if time.Since(state.LastEscalation) < escalateAfter {
		return
	}

	// Find next level
	var nextRule *EscalationRule
	for i := range policy.Rules {
		if policy.Rules[i].Level == state.CurrentLevel+1 {
			nextRule = &policy.Rules[i]
			break
		}
	}

	if nextRule != nil {
		// Escalate to next level
		e.escalateTo(state, policy, nextRule)
	} else if policy.RepeatEnabled {
		// No next level, check if we should repeat
		if policy.RepeatLimit == 0 || state.RepeatCount < policy.RepeatLimit {
			// Reset to level 1
			state.RepeatCount++
			state.CurrentLevel = 0
			if len(policy.Rules) > 0 {
				e.escalateTo(state, policy, &policy.Rules[0])
			}
		}
	}
}

// escalateTo escalates to a specific rule level
func (e *EscalationEngine) escalateTo(state *EscalationState, policy *EscalationPolicy, rule *EscalationRule) {
	oldLevel := state.CurrentLevel
	state.CurrentLevel = rule.Level
	state.LastEscalation = time.Now()

	// Notify targets at this level
	for _, target := range rule.Targets {
		e.notifyTarget(state, target, rule.Level)
	}

	if e.onEscalate != nil {
		e.onEscalate(state.IncidentID, oldLevel, rule.Level)
	}

	log.Printf("[oncall] Escalated incident %s from level %d to %d", state.IncidentID, oldLevel, rule.Level)
}

// notifyTarget notifies a target (user, schedule, or team)
func (e *EscalationEngine) notifyTarget(state *EscalationState, target Target, level int) {
	switch target.Type {
	case "user":
		// Direct user notification
		user := &User{ID: target.ID, Name: target.ID}
		e.notifyUser(state, user, level)

	case "schedule":
		// Get current on-call from schedule
		entry, err := e.calculator.GetCurrentOnCall(target.ID)
		if err != nil {
			log.Printf("[oncall] Error getting on-call for schedule %s: %v", target.ID, err)
			return
		}
		if entry != nil {
			e.notifyUser(state, &entry.User, level)
		}

	case "team":
		// Notify all members of a team (simplified - would need team store)
		log.Printf("[oncall] Team notifications not fully implemented for team %s", target.ID)
	}
}

// notifyUser sends notification to a user
func (e *EscalationEngine) notifyUser(state *EscalationState, user *User, level int) {
	e.mu.RLock()
	channels := make(map[string]NotificationChannel, len(e.channels))
	for k, v := range e.channels {
		channels[k] = v
	}
	e.mu.RUnlock()

	notification := Notification{
		ID:     fmt.Sprintf("%s-%d-%d", state.IncidentID, level, time.Now().UnixNano()),
		Level:  level,
		Target: Target{Type: "user", ID: user.ID},
		User:   user,
		SentAt: time.Now(),
		Status: "pending",
	}

	// Try each registered channel
	for name, ch := range channels {
		err := ch.Send(user, fmt.Sprintf("Incident: %s", state.IncidentID), "You are being paged for an incident")
		if err != nil {
			log.Printf("[oncall] Failed to send %s notification to %s: %v", name, user.ID, err)
			notification.Status = "failed"
		} else {
			notification.Status = "sent"
			notification.Channel = name
		}
	}

	e.mu.Lock()
	state.Notifications = append(state.Notifications, notification)
	e.mu.Unlock()

	if e.onNotify != nil {
		e.onNotify(state.IncidentID, user, level)
	}
}

// TriggerIncident starts escalation for a new incident
func (e *EscalationEngine) TriggerIncident(incidentID, policyID string) error {
	policy, err := e.store.GetPolicy(policyID)
	if err != nil {
		return fmt.Errorf("policy not found: %w", err)
	}

	if len(policy.Rules) == 0 {
		return fmt.Errorf("policy has no rules")
	}

	state := &EscalationState{
		IncidentID:     incidentID,
		PolicyID:       policyID,
		CurrentLevel:   policy.Rules[0].Level,
		StartedAt:      time.Now(),
		LastEscalation: time.Now(),
	}

	e.mu.Lock()
	e.states[incidentID] = state
	e.mu.Unlock()

	// Immediately notify first level
	e.notifyLevel(state, policy, &policy.Rules[0])

	log.Printf("[oncall] Triggered escalation for incident %s using policy %s", incidentID, policyID)
	return nil
}

// notifyLevel notifies all targets at a level
func (e *EscalationEngine) notifyLevel(state *EscalationState, policy *EscalationPolicy, rule *EscalationRule) {
	for _, target := range rule.Targets {
		e.notifyTarget(state, target, rule.Level)
	}
}

// AcknowledgeIncident marks an incident as acknowledged
func (e *EscalationEngine) AcknowledgeIncident(incidentID, userID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, ok := e.states[incidentID]
	if !ok {
		return fmt.Errorf("incident not found")
	}

	if state.Acknowledged {
		return fmt.Errorf("incident already acknowledged")
	}

	now := time.Now()
	state.Acknowledged = true
	state.AckedBy = userID
	state.AckedAt = &now

	log.Printf("[oncall] Incident %s acknowledged by %s", incidentID, userID)
	return nil
}

// ResolveIncident marks an incident as resolved
func (e *EscalationEngine) ResolveIncident(incidentID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	state, ok := e.states[incidentID]
	if !ok {
		return fmt.Errorf("incident not found")
	}

	state.Resolved = true

	log.Printf("[oncall] Incident %s resolved", incidentID)
	return nil
}

// GetState returns the escalation state for an incident
func (e *EscalationEngine) GetState(incidentID string) (*EscalationState, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	state, ok := e.states[incidentID]
	if !ok {
		return nil, fmt.Errorf("incident not found")
	}

	return state, nil
}

// GetActiveIncidents returns all active (unresolved) incidents
func (e *EscalationEngine) GetActiveIncidents() []*EscalationState {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var active []*EscalationState
	for _, state := range e.states {
		if !state.Resolved {
			active = append(active, state)
		}
	}

	return active
}

// Built-in notification channels

// LogChannel logs notifications (for testing)
type LogChannel struct{}

func (c *LogChannel) Name() string { return "log" }
func (c *LogChannel) Send(user *User, subject, message string) error {
	log.Printf("[oncall-notify] To: %s (%s), Subject: %s, Message: %s",
		user.Name, user.Email, subject, message)
	return nil
}

// WebhookChannel sends notifications via webhook
type WebhookChannel struct {
	URL string
}

func (c *WebhookChannel) Name() string { return "webhook" }
func (c *WebhookChannel) Send(user *User, subject, message string) error {
	// In a real implementation, this would POST to the webhook URL
	log.Printf("[oncall-webhook] Would POST to %s: user=%s subject=%s",
		c.URL, user.ID, subject)
	return nil
}

// SlackChannel sends notifications to Slack
type SlackChannel struct {
	Token   string
	Default string // Default channel
}

func (c *SlackChannel) Name() string { return "slack" }
func (c *SlackChannel) Send(user *User, subject, message string) error {
	// In a real implementation, this would use the Slack API
	log.Printf("[oncall-slack] Would notify %s: %s", user.ID, subject)
	return nil
}

// EmailChannel sends email notifications
type EmailChannel struct {
	SMTPHost string
	From     string
}

func (c *EmailChannel) Name() string { return "email" }
func (c *EmailChannel) Send(user *User, subject, message string) error {
	if user.Email == "" {
		return fmt.Errorf("user has no email address")
	}
	// In a real implementation, this would send an email
	log.Printf("[oncall-email] Would email %s: %s", user.Email, subject)
	return nil
}

// PagerDutyChannel integrates with PagerDuty
type PagerDutyChannel struct {
	APIKey     string
	ServiceKey string
}

func (c *PagerDutyChannel) Name() string { return "pagerduty" }
func (c *PagerDutyChannel) Send(user *User, subject, message string) error {
	// In a real implementation, this would use the PagerDuty Events API
	log.Printf("[oncall-pagerduty] Would trigger PD event: %s", subject)
	return nil
}
