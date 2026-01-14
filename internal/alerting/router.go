package alerting

import (
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Router routes alerts to receivers and handles grouping
type Router struct {
	store    *Store
	routes   []Route
	routeMu  sync.RWMutex

	// Alert groups
	groups   map[string]*AlertGroup
	groupMu  sync.RWMutex

	// Receivers
	receivers map[string]Receiver
	recvMu    sync.RWMutex

	// Notification tracking
	lastNotify map[string]time.Time
	notifyMu   sync.RWMutex

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// AlertGroup represents a group of related alerts
type AlertGroup struct {
	Key       string            `json:"key"`
	Labels    map[string]string `json:"labels"`
	Alerts    []*Alert          `json:"alerts"`
	Receiver  string            `json:"receiver"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	NotifiedAt *time.Time       `json:"notified_at,omitempty"`
}

// Receiver handles alert notifications
type Receiver interface {
	Name() string
	Send(group *AlertGroup) error
}

// RouterConfig holds router configuration
type RouterConfig struct {
	DefaultReceiver string        `json:"default_receiver"`
	GroupWait       time.Duration `json:"group_wait"`
	GroupInterval   time.Duration `json:"group_interval"`
	RepeatInterval  time.Duration `json:"repeat_interval"`
	Routes          []Route       `json:"routes"`
}

// NewRouter creates a new alert router
func NewRouter(store *Store) *Router {
	return &Router{
		store:      store,
		routes:     []Route{},
		groups:     make(map[string]*AlertGroup),
		receivers:  make(map[string]Receiver),
		lastNotify: make(map[string]time.Time),
		stopCh:     make(chan struct{}),
	}
}

// RegisterReceiver registers a notification receiver
func (r *Router) RegisterReceiver(recv Receiver) {
	r.recvMu.Lock()
	defer r.recvMu.Unlock()
	r.receivers[recv.Name()] = recv
}

// SetRoutes sets the routing configuration
func (r *Router) SetRoutes(routes []Route) {
	r.routeMu.Lock()
	defer r.routeMu.Unlock()
	r.routes = routes
}

// Start begins the router background loop
func (r *Router) Start() {
	r.wg.Add(1)
	go r.runLoop()
	log.Println("[alerting] Router started")
}

// Stop stops the router
func (r *Router) Stop() {
	close(r.stopCh)
	r.wg.Wait()
	log.Println("[alerting] Router stopped")
}

// runLoop handles periodic notification dispatch
func (r *Router) runLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.processGroups()
		}
	}
}

// Route routes an alert to the appropriate receiver(s)
func (r *Router) Route(alert *Alert) {
	r.routeMu.RLock()
	routes := r.routes
	r.routeMu.RUnlock()

	// Find matching routes
	matchedRoutes := r.findMatchingRoutes(alert, routes)

	for _, route := range matchedRoutes {
		// Determine group key based on GroupBy labels
		groupKey := r.buildGroupKey(alert, route.GroupBy)

		// Add to group
		r.addToGroup(groupKey, alert, route)
	}
}

// findMatchingRoutes finds all routes that match an alert
func (r *Router) findMatchingRoutes(alert *Alert, routes []Route) []Route {
	var matched []Route

	for _, route := range routes {
		if r.matchesRoute(alert, &route) {
			matched = append(matched, route)

			// Check children
			if len(route.Children) > 0 {
				childMatches := r.findMatchingRoutes(alert, route.Children)
				if len(childMatches) > 0 {
					matched = append(matched, childMatches...)
				}
			}

			// Stop if not continuing
			if !route.Continue {
				break
			}
		}
	}

	// If no matches, use default route
	if len(matched) == 0 && len(routes) > 0 {
		matched = append(matched, routes[0])
	}

	return matched
}

// matchesRoute checks if an alert matches a route's matchers
func (r *Router) matchesRoute(alert *Alert, route *Route) bool {
	if len(route.Matchers) == 0 {
		return true // Empty matchers match all
	}

	for _, matcher := range route.Matchers {
		labelValue, exists := alert.Labels[matcher.Name]
		if !exists {
			labelValue = ""
		}

		matched := false
		if matcher.IsRegex {
			re, err := regexp.Compile(matcher.Value)
			if err != nil {
				continue
			}
			matched = re.MatchString(labelValue)
		} else {
			matched = labelValue == matcher.Value
		}

		if matcher.IsEqual && !matched {
			return false
		}
		if !matcher.IsEqual && matched {
			return false
		}
	}

	return true
}

// buildGroupKey creates a key for grouping alerts
func (r *Router) buildGroupKey(alert *Alert, groupBy []string) string {
	if len(groupBy) == 0 {
		// Default grouping: by alertname (rule name)
		return alert.RuleName
	}

	var parts []string
	for _, label := range groupBy {
		if value, ok := alert.Labels[label]; ok {
			parts = append(parts, label+"="+value)
		}
	}

	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// addToGroup adds an alert to a group
func (r *Router) addToGroup(groupKey string, alert *Alert, route Route) {
	r.groupMu.Lock()
	defer r.groupMu.Unlock()

	group, exists := r.groups[groupKey]
	if !exists {
		// Extract group labels
		groupLabels := make(map[string]string)
		for _, label := range route.GroupBy {
			if value, ok := alert.Labels[label]; ok {
				groupLabels[label] = value
			}
		}

		group = &AlertGroup{
			Key:       groupKey,
			Labels:    groupLabels,
			Alerts:    []*Alert{},
			Receiver:  route.Receiver,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		r.groups[groupKey] = group
	}

	// Check if alert already in group
	found := false
	for i, a := range group.Alerts {
		if a.Fingerprint == alert.Fingerprint {
			group.Alerts[i] = alert
			found = true
			break
		}
	}

	if !found {
		group.Alerts = append(group.Alerts, alert)
	}

	group.UpdatedAt = time.Now()
}

// processGroups processes alert groups and sends notifications
func (r *Router) processGroups() {
	r.groupMu.Lock()
	groups := make([]*AlertGroup, 0, len(r.groups))
	for _, g := range r.groups {
		groups = append(groups, g)
	}
	r.groupMu.Unlock()

	for _, group := range groups {
		r.processGroup(group)
	}
}

// processGroup processes a single alert group
func (r *Router) processGroup(group *AlertGroup) {
	// Filter to only firing alerts
	firingAlerts := make([]*Alert, 0)
	for _, alert := range group.Alerts {
		if alert.State == StateFiring {
			firingAlerts = append(firingAlerts, alert)
		}
	}

	if len(firingAlerts) == 0 {
		// No firing alerts, check if we need to send resolved notification
		r.handleResolvedGroup(group)
		return
	}

	// Get route config (use defaults if not specified)
	r.routeMu.RLock()
	var routeConfig *Route
	for i := range r.routes {
		if r.routes[i].Receiver == group.Receiver {
			routeConfig = &r.routes[i]
			break
		}
	}
	r.routeMu.RUnlock()

	groupWait := 30 * time.Second
	groupInterval := 5 * time.Minute
	repeatInterval := 4 * time.Hour

	if routeConfig != nil {
		if routeConfig.GroupWait > 0 {
			groupWait = routeConfig.GroupWait
		}
		if routeConfig.GroupInterval > 0 {
			groupInterval = routeConfig.GroupInterval
		}
		if routeConfig.RepeatInterval > 0 {
			repeatInterval = routeConfig.RepeatInterval
		}
	}

	r.notifyMu.RLock()
	lastNotify := r.lastNotify[group.Key]
	r.notifyMu.RUnlock()

	now := time.Now()
	shouldNotify := false

	if group.NotifiedAt == nil {
		// Never notified, wait GroupWait
		if now.Sub(group.CreatedAt) >= groupWait {
			shouldNotify = true
		}
	} else {
		// Check if group has changed
		if group.UpdatedAt.After(*group.NotifiedAt) {
			// Group changed, wait GroupInterval
			if now.Sub(*group.NotifiedAt) >= groupInterval {
				shouldNotify = true
			}
		} else {
			// Group unchanged, wait RepeatInterval
			if now.Sub(lastNotify) >= repeatInterval {
				shouldNotify = true
			}
		}
	}

	if shouldNotify {
		r.sendGroupNotification(group, firingAlerts)
	}
}

// handleResolvedGroup handles a group with no firing alerts
func (r *Router) handleResolvedGroup(group *AlertGroup) {
	r.groupMu.Lock()
	defer r.groupMu.Unlock()

	// If group was previously notified and has resolved, send resolution
	if group.NotifiedAt != nil {
		hasUnresolved := false
		for _, alert := range group.Alerts {
			if alert.State != StateResolved && alert.State != StateInactive {
				hasUnresolved = true
				break
			}
		}

		if !hasUnresolved {
			// All alerts resolved, clean up group after some time
			if time.Since(group.UpdatedAt) > 15*time.Minute {
				delete(r.groups, group.Key)
			}
		}
	} else {
		// Never notified and no firing alerts, clean up
		if time.Since(group.CreatedAt) > 5*time.Minute {
			delete(r.groups, group.Key)
		}
	}
}

// sendGroupNotification sends a notification for an alert group
func (r *Router) sendGroupNotification(group *AlertGroup, firingAlerts []*Alert) {
	r.recvMu.RLock()
	receiver, exists := r.receivers[group.Receiver]
	r.recvMu.RUnlock()

	if !exists {
		log.Printf("[alerting] Receiver not found: %s", group.Receiver)
		return
	}

	// Create notification group with only firing alerts
	notifyGroup := &AlertGroup{
		Key:       group.Key,
		Labels:    group.Labels,
		Alerts:    firingAlerts,
		Receiver:  group.Receiver,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}

	if err := receiver.Send(notifyGroup); err != nil {
		log.Printf("[alerting] Failed to send notification to %s: %v", group.Receiver, err)
		return
	}

	// Update notification tracking
	now := time.Now()
	r.groupMu.Lock()
	group.NotifiedAt = &now
	r.groupMu.Unlock()

	r.notifyMu.Lock()
	r.lastNotify[group.Key] = now
	r.notifyMu.Unlock()

	log.Printf("[alerting] Sent notification to %s for group %s (%d alerts)",
		group.Receiver, group.Key, len(firingAlerts))
}

// GetGroups returns all alert groups
func (r *Router) GetGroups() []*AlertGroup {
	r.groupMu.RLock()
	defer r.groupMu.RUnlock()

	groups := make([]*AlertGroup, 0, len(r.groups))
	for _, g := range r.groups {
		groups = append(groups, g)
	}
	return groups
}

// GetGroup returns a specific group by key
func (r *Router) GetGroup(key string) *AlertGroup {
	r.groupMu.RLock()
	defer r.groupMu.RUnlock()
	return r.groups[key]
}

// Built-in receivers

// LogReceiver logs notifications
type LogReceiver struct {
	name string
}

// NewLogReceiver creates a new log receiver
func NewLogReceiver(name string) *LogReceiver {
	return &LogReceiver{name: name}
}

func (r *LogReceiver) Name() string { return r.name }
func (r *LogReceiver) Send(group *AlertGroup) error {
	log.Printf("[alert-notify] Group: %s, Receiver: %s, Alerts: %d",
		group.Key, r.name, len(group.Alerts))
	for _, alert := range group.Alerts {
		log.Printf("[alert-notify]   - %s: %s (value: %.2f)",
			alert.ID[:8], alert.RuleName, alert.Value)
	}
	return nil
}

// WebhookReceiver sends alerts via webhook
type WebhookReceiver struct {
	name string
	url  string
}

// NewWebhookReceiver creates a new webhook receiver
func NewWebhookReceiver(name, url string) *WebhookReceiver {
	return &WebhookReceiver{name: name, url: url}
}

func (r *WebhookReceiver) Name() string { return r.name }
func (r *WebhookReceiver) Send(group *AlertGroup) error {
	// In production, POST to webhook URL with JSON payload
	log.Printf("[alert-webhook] Would POST to %s: %d alerts in group %s",
		r.url, len(group.Alerts), group.Key)
	return nil
}

// SlackReceiver sends alerts to Slack
type SlackReceiver struct {
	name    string
	token   string
	channel string
}

// NewSlackReceiver creates a new Slack receiver
func NewSlackReceiver(name, token, channel string) *SlackReceiver {
	return &SlackReceiver{name: name, token: token, channel: channel}
}

func (r *SlackReceiver) Name() string { return r.name }
func (r *SlackReceiver) Send(group *AlertGroup) error {
	// In production, use Slack API
	log.Printf("[alert-slack] Would send to #%s: %d alerts in group %s",
		r.channel, len(group.Alerts), group.Key)
	return nil
}

// EmailReceiver sends alerts via email
type EmailReceiver struct {
	name     string
	smtpHost string
	from     string
	to       []string
}

// NewEmailReceiver creates a new email receiver
func NewEmailReceiver(name, smtpHost, from string, to []string) *EmailReceiver {
	return &EmailReceiver{name: name, smtpHost: smtpHost, from: from, to: to}
}

func (r *EmailReceiver) Name() string { return r.name }
func (r *EmailReceiver) Send(group *AlertGroup) error {
	// In production, send email via SMTP
	log.Printf("[alert-email] Would email to %v: %d alerts in group %s",
		r.to, len(group.Alerts), group.Key)
	return nil
}

// PagerDutyReceiver integrates with PagerDuty
type PagerDutyReceiver struct {
	name        string
	routingKey  string
	serviceKey  string
}

// NewPagerDutyReceiver creates a new PagerDuty receiver
func NewPagerDutyReceiver(name, routingKey, serviceKey string) *PagerDutyReceiver {
	return &PagerDutyReceiver{name: name, routingKey: routingKey, serviceKey: serviceKey}
}

func (r *PagerDutyReceiver) Name() string { return r.name }
func (r *PagerDutyReceiver) Send(group *AlertGroup) error {
	// In production, use PagerDuty Events API v2
	log.Printf("[alert-pagerduty] Would trigger PD event: %d alerts in group %s",
		len(group.Alerts), group.Key)
	return nil
}

// OpsGenieReceiver integrates with OpsGenie
type OpsGenieReceiver struct {
	name   string
	apiKey string
}

// NewOpsGenieReceiver creates a new OpsGenie receiver
func NewOpsGenieReceiver(name, apiKey string) *OpsGenieReceiver {
	return &OpsGenieReceiver{name: name, apiKey: apiKey}
}

func (r *OpsGenieReceiver) Name() string { return r.name }
func (r *OpsGenieReceiver) Send(group *AlertGroup) error {
	log.Printf("[alert-opsgenie] Would create OpsGenie alert: %d alerts in group %s",
		len(group.Alerts), group.Key)
	return nil
}

// OnCallReceiver integrates with the on-call system
type OnCallReceiver struct {
	name         string
	escalationID string
	notifyFn     func(incidentID, policyID string) error
}

// NewOnCallReceiver creates a new on-call receiver
func NewOnCallReceiver(name, escalationID string, notifyFn func(incidentID, policyID string) error) *OnCallReceiver {
	return &OnCallReceiver{name: name, escalationID: escalationID, notifyFn: notifyFn}
}

func (r *OnCallReceiver) Name() string { return r.name }
func (r *OnCallReceiver) Send(group *AlertGroup) error {
	if r.notifyFn != nil {
		// Use the first alert ID as the incident ID
		if len(group.Alerts) > 0 {
			return r.notifyFn(group.Alerts[0].ID, r.escalationID)
		}
	}
	log.Printf("[alert-oncall] Would trigger escalation %s: %d alerts in group %s",
		r.escalationID, len(group.Alerts), group.Key)
	return nil
}

// NotifyServiceReceiver bridges alerting to the notification service
type NotifyServiceReceiver struct {
	name     string
	sendFunc func(group *AlertGroup) error
}

// NewNotifyServiceReceiver creates a receiver that uses the notification service
func NewNotifyServiceReceiver(name string, sendFunc func(group *AlertGroup) error) *NotifyServiceReceiver {
	return &NotifyServiceReceiver{name: name, sendFunc: sendFunc}
}

func (r *NotifyServiceReceiver) Name() string { return r.name }
func (r *NotifyServiceReceiver) Send(group *AlertGroup) error {
	if r.sendFunc != nil {
		return r.sendFunc(group)
	}
	return nil
}
