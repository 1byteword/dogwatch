package incidents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Pager handles incident notifications and escalations
type Pager struct {
	store          *Store
	defaultPolicy  string                // Default escalation policy ID
	slackWebhook   string                // Default Slack webhook
	webhooks       map[string]string     // Named webhooks
	stopChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.RWMutex
	escalationJobs map[string]*time.Timer // Track pending escalations
}

// NewPager creates a new pager
func NewPager(store *Store) *Pager {
	return &Pager{
		store:          store,
		webhooks:       make(map[string]string),
		stopChan:       make(chan struct{}),
		escalationJobs: make(map[string]*time.Timer),
	}
}

// SetSlackWebhook sets the default Slack webhook URL
func (p *Pager) SetSlackWebhook(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slackWebhook = url
}

// AddWebhook adds a named webhook
func (p *Pager) AddWebhook(name, url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.webhooks[name] = url
}

// SetDefaultPolicy sets the default escalation policy
func (p *Pager) SetDefaultPolicy(policyID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultPolicy = policyID
}

// Start begins the escalation monitoring loop
func (p *Pager) Start() {
	p.wg.Add(1)
	go p.runLoop()
	log.Println("[pager] Started incident pager")
}

// Stop stops the pager
func (p *Pager) Stop() {
	close(p.stopChan)
	p.wg.Wait()

	// Cancel all pending escalations
	p.mu.Lock()
	for _, timer := range p.escalationJobs {
		timer.Stop()
	}
	p.mu.Unlock()

	log.Println("[pager] Stopped incident pager")
}

func (p *Pager) runLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopChan:
			return
		case <-ticker.C:
			p.checkEscalations()
		}
	}
}

// checkEscalations checks for incidents that need escalation
func (p *Pager) checkEscalations() {
	incidents, err := p.store.ListActiveIncidents()
	if err != nil {
		log.Printf("[pager] Error listing active incidents: %v", err)
		return
	}

	for _, inc := range incidents {
		if inc.Status != StatusTriggered {
			continue
		}

		// Check if this incident needs escalation
		p.maybeEscalate(&inc)
	}
}

// maybeEscalate checks and performs escalation if needed
func (p *Pager) maybeEscalate(inc *Incident) {
	p.mu.RLock()
	policyID := p.defaultPolicy
	p.mu.RUnlock()

	if policyID == "" {
		return
	}

	policy, err := p.store.GetPolicy(policyID)
	if err != nil || policy == nil {
		return
	}

	currentLevel := inc.EscLevel
	if currentLevel >= len(policy.Rules) {
		// Already at max escalation
		if policy.RepeatAfter > 0 {
			// Check if we should repeat from level 0
			timeSinceCreated := time.Since(inc.CreatedAt)
			cycleTime := time.Duration(policy.RepeatAfter) * time.Minute
			if int(timeSinceCreated/cycleTime) > currentLevel/len(policy.Rules) {
				// Time to repeat
				p.store.EscalateIncident(inc.ID, 0, "")
				p.notifyLevel(inc, &policy.Rules[0])
			}
		}
		return
	}

	rule := policy.Rules[currentLevel]
	escalationTime := inc.CreatedAt.Add(time.Duration(rule.DelayMinutes) * time.Minute)

	// Adjust for previous levels
	for i := 0; i < currentLevel; i++ {
		escalationTime = escalationTime.Add(time.Duration(policy.Rules[i].DelayMinutes) * time.Minute)
	}

	if time.Now().After(escalationTime) {
		// Time to escalate
		nextLevel := currentLevel + 1
		var assignTo string

		if nextLevel < len(policy.Rules) {
			nextRule := policy.Rules[nextLevel]
			assignTo = p.resolveTargets(nextRule.Targets)
			p.notifyLevel(inc, &nextRule)
		}

		p.store.EscalateIncident(inc.ID, nextLevel, assignTo)
		log.Printf("[pager] Escalated incident %s to level %d", inc.ID, nextLevel)
	}
}

// resolveTargets gets the actual user from targets (resolves schedules)
func (p *Pager) resolveTargets(targets []Target) string {
	for _, target := range targets {
		switch target.Type {
		case "user":
			return target.ID
		case "schedule":
			user, err := p.store.GetCurrentOnCall(target.ID)
			if err == nil && user != "" {
				return user
			}
		}
	}
	return ""
}

// Trigger creates and notifies about a new incident
func (p *Pager) Trigger(inc *Incident) error {
	if err := p.store.CreateIncident(inc); err != nil {
		return err
	}

	// Notify based on severity
	p.notifyIncident(inc)

	// Schedule escalation if policy exists
	p.scheduleEscalation(inc)

	log.Printf("[pager] Triggered incident: %s (%s)", inc.Title, inc.Severity)
	return nil
}

// TriggerFromWatch creates an incident from a watch alert
func (p *Pager) TriggerFromWatch(watchID, watchName, message string, severity string) error {
	// Convert string severity to Severity type
	sev := Severity(severity)
	if sev != SeverityCritical && sev != SeverityHigh && sev != SeverityMedium && sev != SeverityLow {
		sev = SeverityHigh // Default to high for watch alerts
	}

	inc := &Incident{
		Title:       fmt.Sprintf("Watch Alert: %s", watchName),
		Description: message,
		Severity:    sev,
		Source:      "watch",
		SourceID:    watchID,
	}
	return p.Trigger(inc)
}

// TriggerFromSLO creates an incident from an SLO breach
func (p *Pager) TriggerFromSLO(sloID, sloName string, remaining float64) error {
	severity := SeverityMedium
	if remaining <= 0 {
		severity = SeverityCritical
	} else if remaining < 10 {
		severity = SeverityHigh
	}

	inc := &Incident{
		Title:       fmt.Sprintf("SLO Breach: %s", sloName),
		Description: fmt.Sprintf("Error budget remaining: %.2f%%", remaining),
		Severity:    severity,
		Source:      "slo",
		SourceID:    sloID,
	}
	return p.Trigger(inc)
}

// TriggerFromSynthetic creates an incident from a synthetic check failure
func (p *Pager) TriggerFromSynthetic(checkID, checkName, errorMsg string) error {
	inc := &Incident{
		Title:       fmt.Sprintf("Synthetic Check Failed: %s", checkName),
		Description: errorMsg,
		Severity:    SeverityHigh,
		Source:      "synthetic",
		SourceID:    checkID,
	}
	return p.Trigger(inc)
}

// notifyIncident sends notifications for a new incident
func (p *Pager) notifyIncident(inc *Incident) {
	// Send to Slack
	p.sendSlackNotification(inc, "triggered")

	// Send to webhooks
	p.sendWebhookNotification(inc, "triggered")
}

// notifyLevel sends notifications for an escalation level
func (p *Pager) notifyLevel(inc *Incident, rule *EscalationRule) {
	for _, channel := range rule.NotifyChannels {
		switch channel {
		case "slack":
			p.sendSlackNotification(inc, "escalated")
		case "webhook":
			p.sendWebhookNotification(inc, "escalated")
		}
	}
}

// sendSlackNotification sends a Slack notification
func (p *Pager) sendSlackNotification(inc *Incident, action string) {
	p.mu.RLock()
	webhook := p.slackWebhook
	p.mu.RUnlock()

	if webhook == "" {
		return
	}

	color := "#ff9800" // warning
	switch inc.Severity {
	case SeverityCritical:
		color = "#f44336" // red
	case SeverityHigh:
		color = "#ff5722" // deep orange
	case SeverityLow:
		color = "#2196f3" // blue
	}

	emoji := "🔔"
	if action == "escalated" {
		emoji = "🚨"
	}

	statusText := string(inc.Status)
	if action == "escalated" {
		statusText = fmt.Sprintf("escalated (level %d)", inc.EscLevel+1)
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"blocks": []map[string]interface{}{
					{
						"type": "header",
						"text": map[string]string{
							"type": "plain_text",
							"text": fmt.Sprintf("%s Incident %s: %s", emoji, statusText, inc.Title),
						},
					},
					{
						"type": "section",
						"fields": []map[string]string{
							{"type": "mrkdwn", "text": fmt.Sprintf("*Severity:*\n%s", inc.Severity)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Service:*\n%s", inc.Service)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Source:*\n%s", inc.Source)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Assigned:*\n%s", inc.AssignedTo)},
						},
					},
					{
						"type": "section",
						"text": map[string]string{
							"type": "mrkdwn",
							"text": inc.Description,
						},
					},
					{
						"type": "actions",
						"elements": []map[string]interface{}{
							{
								"type": "button",
								"text": map[string]string{"type": "plain_text", "text": "Acknowledge"},
								"style": "primary",
								"url":  fmt.Sprintf("http://localhost:9999/incidents?ack=%s", inc.ID),
							},
							{
								"type": "button",
								"text": map[string]string{"type": "plain_text", "text": "View Details"},
								"url":  fmt.Sprintf("http://localhost:9999/incidents?id=%s", inc.ID),
							},
						},
					},
				},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := http.Post(webhook, "application/json", bytes.NewBuffer(jsonPayload))

	status := "sent"
	if err != nil {
		status = "failed"
		log.Printf("[pager] Slack notification failed: %v", err)
	} else {
		resp.Body.Close()
	}

	p.store.LogNotification(&NotificationLog{
		IncidentID: inc.ID,
		Channel:    "slack",
		Target:     webhook,
		Status:     status,
		Message:    inc.Title,
	})
}

// sendWebhookNotification sends a generic webhook notification
func (p *Pager) sendWebhookNotification(inc *Incident, action string) {
	p.mu.RLock()
	webhooks := make(map[string]string)
	for k, v := range p.webhooks {
		webhooks[k] = v
	}
	p.mu.RUnlock()

	payload := map[string]interface{}{
		"incident_id": inc.ID,
		"title":       inc.Title,
		"description": inc.Description,
		"severity":    inc.Severity,
		"status":      inc.Status,
		"service":     inc.Service,
		"source":      inc.Source,
		"source_id":   inc.SourceID,
		"action":      action,
		"assigned_to": inc.AssignedTo,
		"created_at":  inc.CreatedAt,
		"esc_level":   inc.EscLevel,
	}

	jsonPayload, _ := json.Marshal(payload)

	for name, url := range webhooks {
		go func(name, url string) {
			resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
			status := "sent"
			if err != nil {
				status = "failed"
				log.Printf("[pager] Webhook %s failed: %v", name, err)
			} else {
				resp.Body.Close()
			}

			p.store.LogNotification(&NotificationLog{
				IncidentID: inc.ID,
				Channel:    "webhook",
				Target:     name,
				Status:     status,
				Message:    inc.Title,
			})
		}(name, url)
	}
}

// scheduleEscalation sets up escalation timers for an incident
func (p *Pager) scheduleEscalation(inc *Incident) {
	p.mu.RLock()
	policyID := p.defaultPolicy
	p.mu.RUnlock()

	if policyID == "" {
		return
	}

	policy, err := p.store.GetPolicy(policyID)
	if err != nil || policy == nil {
		return
	}

	if len(policy.Rules) == 0 {
		return
	}

	// Schedule first escalation
	firstRule := policy.Rules[0]
	delay := time.Duration(firstRule.DelayMinutes) * time.Minute

	p.mu.Lock()
	p.escalationJobs[inc.ID] = time.AfterFunc(delay, func() {
		p.checkEscalations()
	})
	p.mu.Unlock()
}

// CancelEscalation cancels pending escalations for an incident
func (p *Pager) CancelEscalation(incidentID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if timer, ok := p.escalationJobs[incidentID]; ok {
		timer.Stop()
		delete(p.escalationJobs, incidentID)
	}
}

// NotifyResolution sends resolution notification
func (p *Pager) NotifyResolution(inc *Incident) {
	p.mu.RLock()
	webhook := p.slackWebhook
	p.mu.RUnlock()

	if webhook == "" {
		return
	}

	duration := ""
	if inc.ResolvedAt != nil {
		d := inc.ResolvedAt.Sub(inc.CreatedAt)
		if d.Hours() >= 1 {
			duration = fmt.Sprintf("%.1f hours", d.Hours())
		} else {
			duration = fmt.Sprintf("%.0f minutes", d.Minutes())
		}
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color": "#4caf50", // green
				"blocks": []map[string]interface{}{
					{
						"type": "header",
						"text": map[string]string{
							"type": "plain_text",
							"text": fmt.Sprintf("✅ Incident Resolved: %s", inc.Title),
						},
					},
					{
						"type": "section",
						"fields": []map[string]string{
							{"type": "mrkdwn", "text": fmt.Sprintf("*Resolved by:*\n%s", inc.ResolvedBy)},
							{"type": "mrkdwn", "text": fmt.Sprintf("*Duration:*\n%s", duration)},
						},
					},
				},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	resp, err := http.Post(webhook, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("[pager] Slack resolution notification failed: %v", err)
	} else {
		resp.Body.Close()
	}
}
