package alerting

import (
	"crypto/md5"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MetricsProvider interface for fetching metric values
type MetricsProvider interface {
	// Query executes a query and returns results
	Query(query string) ([]QueryResult, error)
	// GetMetric fetches a specific metric's current value
	GetMetric(name string, labels map[string]string) (float64, error)
}

// QueryResult represents a result from a metrics query
type QueryResult struct {
	Labels map[string]string
	Value  float64
	Time   time.Time
}

// NotifyFunc is called when an alert should be sent
type NotifyFunc func(alert *Alert, rule *Rule)

// Evaluator periodically evaluates alert rules
type Evaluator struct {
	store           *Store
	metricsProvider MetricsProvider
	onNotify        NotifyFunc

	// Active alert states (fingerprint -> alert)
	alerts map[string]*Alert
	mu     sync.RWMutex

	// Rule evaluation state
	evalState map[string]*RuleEvalState
	evalMu    sync.RWMutex

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// RuleEvalState tracks evaluation state for a rule
type RuleEvalState struct {
	LastEval      time.Time
	ActiveSince   map[string]time.Time // fingerprint -> when condition became true
	FiringAlerts  map[string]bool      // fingerprint -> is firing
	PendingAlerts map[string]time.Time // fingerprint -> pending since
}

// NewEvaluator creates a new alert evaluator
func NewEvaluator(store *Store, metricsProvider MetricsProvider) *Evaluator {
	return &Evaluator{
		store:           store,
		metricsProvider: metricsProvider,
		alerts:          make(map[string]*Alert),
		evalState:       make(map[string]*RuleEvalState),
		stopCh:          make(chan struct{}),
	}
}

// SetNotifyFunc sets the notification callback
func (e *Evaluator) SetNotifyFunc(fn NotifyFunc) {
	e.onNotify = fn
}

// Start begins the evaluation loop
func (e *Evaluator) Start() {
	e.wg.Add(1)
	go e.runLoop()
	log.Println("[alerting] Evaluator started")
}

// Stop stops the evaluator
func (e *Evaluator) Stop() {
	close(e.stopCh)
	e.wg.Wait()
	log.Println("[alerting] Evaluator stopped")
}

// runLoop is the main evaluation loop
func (e *Evaluator) runLoop() {
	defer e.wg.Done()

	// Evaluate every 15 seconds by default
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// Run initial evaluation
	e.evaluateAll()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.evaluateAll()
		}
	}
}

// evaluateAll evaluates all enabled rules
func (e *Evaluator) evaluateAll() {
	rules, err := e.store.ListEnabledRules()
	if err != nil {
		log.Printf("[alerting] Error listing rules: %v", err)
		return
	}

	for _, rule := range rules {
		e.evaluateRule(&rule)
	}

	// Clean up resolved alerts
	e.cleanupResolvedAlerts()
}

// evaluateRule evaluates a single rule
func (e *Evaluator) evaluateRule(rule *Rule) {
	// Check if it's time to evaluate this rule
	e.evalMu.Lock()
	state, exists := e.evalState[rule.ID]
	if !exists {
		state = &RuleEvalState{
			ActiveSince:   make(map[string]time.Time),
			FiringAlerts:  make(map[string]bool),
			PendingAlerts: make(map[string]time.Time),
		}
		e.evalState[rule.ID] = state
	}

	// Use rule's eval interval or default to 1 minute
	evalInterval := rule.EvalInterval
	if evalInterval == 0 {
		evalInterval = time.Minute
	}

	if time.Since(state.LastEval) < evalInterval {
		e.evalMu.Unlock()
		return
	}
	state.LastEval = time.Now()
	e.evalMu.Unlock()

	// Evaluate based on rule type
	var results []EvalResult
	var err error

	switch rule.Type {
	case RuleTypeThreshold:
		results, err = e.evaluateThreshold(rule)
	case RuleTypeAnomaly:
		results, err = e.evaluateAnomaly(rule)
	case RuleTypeChange:
		results, err = e.evaluateChange(rule)
	case RuleTypeAbsence:
		results, err = e.evaluateAbsence(rule)
	case RuleTypeComposite:
		results, err = e.evaluateComposite(rule)
	default:
		log.Printf("[alerting] Unknown rule type: %s", rule.Type)
		return
	}

	if err != nil {
		log.Printf("[alerting] Error evaluating rule %s: %v", rule.ID, err)
		return
	}

	// Process results
	e.processResults(rule, state, results)
}

// EvalResult represents the result of evaluating a rule
type EvalResult struct {
	Labels    map[string]string
	Value     float64
	Firing    bool
	Timestamp time.Time
}

// evaluateThreshold evaluates a threshold-based rule
func (e *Evaluator) evaluateThreshold(rule *Rule) ([]EvalResult, error) {
	var queryResults []QueryResult
	var err error

	if rule.Query != "" {
		queryResults, err = e.metricsProvider.Query(rule.Query)
	} else if rule.Metric != "" {
		value, err := e.metricsProvider.GetMetric(rule.Metric, rule.Labels)
		if err != nil {
			return nil, err
		}
		queryResults = []QueryResult{{
			Labels: rule.Labels,
			Value:  value,
			Time:   time.Now(),
		}}
	} else {
		return nil, fmt.Errorf("rule has no query or metric")
	}

	if err != nil {
		return nil, err
	}

	var results []EvalResult
	for _, qr := range queryResults {
		firing := e.checkCondition(qr.Value, rule.Condition, rule.Threshold)
		results = append(results, EvalResult{
			Labels:    qr.Labels,
			Value:     qr.Value,
			Firing:    firing,
			Timestamp: qr.Time,
		})
	}

	return results, nil
}

// checkCondition checks if a value meets the threshold condition
func (e *Evaluator) checkCondition(value float64, condition string, threshold float64) bool {
	switch condition {
	case "gt", ">":
		return value > threshold
	case "lt", "<":
		return value < threshold
	case "gte", ">=":
		return value >= threshold
	case "lte", "<=":
		return value <= threshold
	case "eq", "==":
		return value == threshold
	case "neq", "!=":
		return value != threshold
	default:
		return false
	}
}

// evaluateAnomaly evaluates an anomaly detection rule
func (e *Evaluator) evaluateAnomaly(rule *Rule) ([]EvalResult, error) {
	if rule.Query == "" {
		return nil, fmt.Errorf("anomaly rule requires a query")
	}

	queryResults, err := e.metricsProvider.Query(rule.Query)
	if err != nil {
		return nil, err
	}

	// Sensitivity determines the threshold (0 = very sensitive, 1 = not sensitive)
	// Default sensitivity of 0.5 = 2 standard deviations
	sensitivity := rule.Sensitivity
	if sensitivity == 0 {
		sensitivity = 0.5
	}
	zThreshold := 1.0 + (1.0-sensitivity)*3.0 // Maps 0->4, 0.5->2.5, 1->1

	var results []EvalResult
	for _, qr := range queryResults {
		// In a real implementation, this would compare against historical baseline
		// For now, we'll use a simple approach
		firing := false
		if rule.Algorithm == "zscore" || rule.Algorithm == "" {
			// Would calculate z-score from historical data
			// Placeholder: mark as anomaly if value exceeds threshold
			if rule.Threshold > 0 {
				zScore := (qr.Value - rule.Threshold) / (rule.Threshold * 0.1)
				firing = zScore > zThreshold || zScore < -zThreshold
			}
		}

		results = append(results, EvalResult{
			Labels:    qr.Labels,
			Value:     qr.Value,
			Firing:    firing,
			Timestamp: qr.Time,
		})
	}

	return results, nil
}

// evaluateChange evaluates a change detection rule
func (e *Evaluator) evaluateChange(rule *Rule) ([]EvalResult, error) {
	if rule.Query == "" {
		return nil, fmt.Errorf("change rule requires a query")
	}

	// Get current value
	currentResults, err := e.metricsProvider.Query(rule.Query)
	if err != nil {
		return nil, err
	}

	// In a real implementation, we'd query historical data
	// For now, use threshold as the baseline
	baseline := rule.Threshold
	changeThreshold := rule.ChangeThreshold
	if changeThreshold == 0 {
		changeThreshold = 10.0 // Default 10% change
	}

	var results []EvalResult
	for _, qr := range currentResults {
		var change float64
		if rule.ChangeType == "percent" && baseline != 0 {
			change = ((qr.Value - baseline) / baseline) * 100
		} else {
			change = qr.Value - baseline
		}

		firing := false
		if change > changeThreshold || change < -changeThreshold {
			firing = true
		}

		results = append(results, EvalResult{
			Labels:    qr.Labels,
			Value:     qr.Value,
			Firing:    firing,
			Timestamp: qr.Time,
		})
	}

	return results, nil
}

// evaluateAbsence evaluates a data absence rule
func (e *Evaluator) evaluateAbsence(rule *Rule) ([]EvalResult, error) {
	if rule.Query == "" && rule.Metric == "" {
		return nil, fmt.Errorf("absence rule requires a query or metric")
	}

	var queryResults []QueryResult
	var err error

	if rule.Query != "" {
		queryResults, err = e.metricsProvider.Query(rule.Query)
	} else {
		_, err = e.metricsProvider.GetMetric(rule.Metric, rule.Labels)
		if err == nil {
			// Data exists, not firing
			return []EvalResult{{
				Labels:    rule.Labels,
				Value:     0,
				Firing:    false,
				Timestamp: time.Now(),
			}}, nil
		}
		// Data is absent
		return []EvalResult{{
			Labels:    rule.Labels,
			Value:     0,
			Firing:    true,
			Timestamp: time.Now(),
		}}, nil
	}

	if err != nil {
		// Query failed, consider as absence
		return []EvalResult{{
			Labels:    rule.Labels,
			Value:     0,
			Firing:    true,
			Timestamp: time.Now(),
		}}, nil
	}

	// If we got results, data exists
	if len(queryResults) > 0 {
		return []EvalResult{{
			Labels:    rule.Labels,
			Value:     queryResults[0].Value,
			Firing:    false,
			Timestamp: time.Now(),
		}}, nil
	}

	// No results = data absent
	return []EvalResult{{
		Labels:    rule.Labels,
		Value:     0,
		Firing:    true,
		Timestamp: time.Now(),
	}}, nil
}

// evaluateComposite evaluates a composite rule based on other rules
func (e *Evaluator) evaluateComposite(rule *Rule) ([]EvalResult, error) {
	if len(rule.SubRules) == 0 {
		return nil, fmt.Errorf("composite rule has no sub-rules")
	}

	// Get the state of all sub-rules
	subRuleFiring := make(map[string]bool)
	for _, subRuleID := range rule.SubRules {
		e.evalMu.RLock()
		state, exists := e.evalState[subRuleID]
		e.evalMu.RUnlock()

		if exists && len(state.FiringAlerts) > 0 {
			subRuleFiring[subRuleID] = true
		} else {
			subRuleFiring[subRuleID] = false
		}
	}

	// Evaluate expression (simple AND/OR for now)
	firing := false
	expr := strings.ToLower(rule.Expression)
	if strings.Contains(expr, " and ") {
		// All must be firing
		firing = true
		for _, isFiring := range subRuleFiring {
			if !isFiring {
				firing = false
				break
			}
		}
	} else if strings.Contains(expr, " or ") {
		// Any must be firing
		for _, isFiring := range subRuleFiring {
			if isFiring {
				firing = true
				break
			}
		}
	} else {
		// Default: any firing
		for _, isFiring := range subRuleFiring {
			if isFiring {
				firing = true
				break
			}
		}
	}

	return []EvalResult{{
		Labels:    rule.Labels,
		Value:     0,
		Firing:    firing,
		Timestamp: time.Now(),
	}}, nil
}

// processResults processes evaluation results and manages alert states
func (e *Evaluator) processResults(rule *Rule, state *RuleEvalState, results []EvalResult) {
	now := time.Now()

	// Track which fingerprints we've seen
	seenFingerprints := make(map[string]bool)

	for _, result := range results {
		// Generate fingerprint for this alert instance
		fingerprint := generateFingerprint(rule.ID, result.Labels)
		seenFingerprints[fingerprint] = true

		if result.Firing {
			e.handleFiringResult(rule, state, fingerprint, result, now)
		} else {
			e.handleResolvedResult(rule, state, fingerprint, now)
		}
	}

	// Mark any alerts that weren't seen as resolved
	e.evalMu.Lock()
	for fingerprint := range state.FiringAlerts {
		if !seenFingerprints[fingerprint] {
			delete(state.FiringAlerts, fingerprint)
			delete(state.ActiveSince, fingerprint)
			delete(state.PendingAlerts, fingerprint)

			// Resolve the alert
			e.mu.Lock()
			if alert, exists := e.alerts[fingerprint]; exists {
				if alert.State == StateFiring || alert.State == StatePending {
					alert.State = StateResolved
					resolvedAt := time.Now()
					alert.ResolvedAt = &resolvedAt
					alert.EndsAt = &resolvedAt
					e.store.SaveAlert(alert)
				}
			}
			e.mu.Unlock()
		}
	}
	e.evalMu.Unlock()
}

// handleFiringResult handles a result where the condition is true
func (e *Evaluator) handleFiringResult(rule *Rule, state *RuleEvalState, fingerprint string, result EvalResult, now time.Time) {
	e.evalMu.Lock()

	// Track when condition started
	if _, exists := state.ActiveSince[fingerprint]; !exists {
		state.ActiveSince[fingerprint] = now
	}

	activeSince := state.ActiveSince[fingerprint]
	e.evalMu.Unlock()

	// Get or create alert
	e.mu.Lock()
	alert, exists := e.alerts[fingerprint]
	if !exists {
		alert = &Alert{
			ID:          uuid.New().String(),
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			Fingerprint: fingerprint,
			State:       StatePending,
			Severity:    e.getSeverity(rule),
			Labels:      e.mergeLabels(rule.Labels, result.Labels),
			Annotations: rule.Annotations,
			Value:       result.Value,
			Threshold:   rule.Threshold,
			StartsAt:    activeSince,
			LastEvalAt:  now,
		}
		e.alerts[fingerprint] = alert
	}

	alert.Value = result.Value
	alert.LastEvalAt = now
	e.mu.Unlock()

	// Check ForDuration
	pendingDuration := now.Sub(activeSince)
	shouldFire := pendingDuration >= rule.ForDuration

	if shouldFire && alert.State != StateFiring {
		// Transition to firing
		e.mu.Lock()
		alert.State = StateFiring
		firedAt := now
		alert.FiredAt = &firedAt
		e.mu.Unlock()

		e.evalMu.Lock()
		state.FiringAlerts[fingerprint] = true
		delete(state.PendingAlerts, fingerprint)
		e.evalMu.Unlock()

		// Check if silenced
		if !e.isSilenced(alert) {
			// Send notification
			if e.onNotify != nil {
				e.onNotify(alert, rule)
			}
			notifiedAt := now
			alert.NotifiedAt = &notifiedAt
		}

		log.Printf("[alerting] Alert %s firing: %s (value: %.2f, threshold: %.2f)",
			alert.ID, rule.Name, result.Value, rule.Threshold)
	} else if !shouldFire && alert.State == StateInactive {
		// Transition to pending
		e.mu.Lock()
		alert.State = StatePending
		e.mu.Unlock()

		e.evalMu.Lock()
		state.PendingAlerts[fingerprint] = activeSince
		e.evalMu.Unlock()
	}

	// Save alert state
	e.store.SaveAlert(alert)
}

// handleResolvedResult handles a result where the condition is false
func (e *Evaluator) handleResolvedResult(rule *Rule, state *RuleEvalState, fingerprint string, now time.Time) {
	e.evalMu.Lock()
	wasActive := state.ActiveSince[fingerprint]
	delete(state.ActiveSince, fingerprint)
	delete(state.PendingAlerts, fingerprint)
	e.evalMu.Unlock()

	if wasActive.IsZero() {
		return // Was never active
	}

	e.mu.Lock()
	alert, exists := e.alerts[fingerprint]
	if !exists {
		e.mu.Unlock()
		return
	}

	wasFiring := alert.State == StateFiring

	// Check KeepFiringFor duration
	if wasFiring && rule.KeepFiringFor > 0 {
		if alert.FiredAt != nil && now.Sub(*alert.FiredAt) < rule.KeepFiringFor {
			// Keep firing for a bit longer
			e.mu.Unlock()
			return
		}
	}

	if alert.State == StateFiring || alert.State == StatePending {
		alert.State = StateResolved
		resolvedAt := now
		alert.ResolvedAt = &resolvedAt
		alert.EndsAt = &resolvedAt
		e.store.SaveAlert(alert)

		if wasFiring {
			log.Printf("[alerting] Alert %s resolved: %s", alert.ID, rule.Name)
		}
	}
	e.mu.Unlock()

	e.evalMu.Lock()
	delete(state.FiringAlerts, fingerprint)
	e.evalMu.Unlock()
}

// isSilenced checks if an alert is silenced
func (e *Evaluator) isSilenced(alert *Alert) bool {
	silences, err := e.store.ListActiveSilences()
	if err != nil {
		return false
	}

	for _, silence := range silences {
		if e.matchesSilence(alert, &silence) {
			return true
		}
	}

	return false
}

// matchesSilence checks if an alert matches a silence
func (e *Evaluator) matchesSilence(alert *Alert, silence *Silence) bool {
	for _, matcher := range silence.Matchers {
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

		// IsEqual = true means we want a match, false means we want no match
		if matcher.IsEqual && !matched {
			return false
		}
		if !matcher.IsEqual && matched {
			return false
		}
	}

	return true
}

// getSeverity determines severity from rule or defaults
func (e *Evaluator) getSeverity(rule *Rule) Severity {
	if sev, ok := rule.Labels["severity"]; ok {
		switch sev {
		case "critical":
			return SeverityCritical
		case "warning":
			return SeverityWarning
		case "info":
			return SeverityInfo
		}
	}
	return SeverityWarning
}

// mergeLabels merges two label maps
func (e *Evaluator) mergeLabels(base, overlay map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// cleanupResolvedAlerts removes old resolved alerts from memory
func (e *Evaluator) cleanupResolvedAlerts() {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-time.Hour)
	for fingerprint, alert := range e.alerts {
		if alert.State == StateResolved && alert.ResolvedAt != nil && alert.ResolvedAt.Before(cutoff) {
			delete(e.alerts, fingerprint)
		}
	}
}

// GetFiringAlerts returns all currently firing alerts
func (e *Evaluator) GetFiringAlerts() []*Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var alerts []*Alert
	for _, alert := range e.alerts {
		if alert.State == StateFiring {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// GetPendingAlerts returns all pending alerts
func (e *Evaluator) GetPendingAlerts() []*Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var alerts []*Alert
	for _, alert := range e.alerts {
		if alert.State == StatePending {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// GetAlert returns a specific alert by fingerprint
func (e *Evaluator) GetAlert(fingerprint string) *Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alerts[fingerprint]
}

// AcknowledgeAlert acknowledges an alert
func (e *Evaluator) AcknowledgeAlert(alertID, userID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, alert := range e.alerts {
		if alert.ID == alertID {
			now := time.Now()
			alert.AcknowledgedAt = &now
			alert.AcknowledgedBy = userID
			return e.store.SaveAlert(alert)
		}
	}

	return fmt.Errorf("alert not found: %s", alertID)
}

// generateFingerprint creates a unique fingerprint for an alert
func generateFingerprint(ruleID string, labels map[string]string) string {
	// Sort label keys for consistent fingerprinting
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	parts = append(parts, ruleID)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, labels[k]))
	}

	hash := md5.Sum([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", hash)
}

// SimpleMetricsProvider is a basic implementation for testing
type SimpleMetricsProvider struct {
	metrics map[string]float64
	mu      sync.RWMutex
}

// NewSimpleMetricsProvider creates a new simple metrics provider
func NewSimpleMetricsProvider() *SimpleMetricsProvider {
	return &SimpleMetricsProvider{
		metrics: make(map[string]float64),
	}
}

// SetMetric sets a metric value
func (p *SimpleMetricsProvider) SetMetric(name string, value float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metrics[name] = value
}

// Query implements MetricsProvider
func (p *SimpleMetricsProvider) Query(query string) ([]QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Simple implementation: query is metric name
	if value, ok := p.metrics[query]; ok {
		return []QueryResult{{
			Labels: map[string]string{"metric": query},
			Value:  value,
			Time:   time.Now(),
		}}, nil
	}

	return nil, nil
}

// GetMetric implements MetricsProvider
func (p *SimpleMetricsProvider) GetMetric(name string, labels map[string]string) (float64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if value, ok := p.metrics[name]; ok {
		return value, nil
	}

	return 0, fmt.Errorf("metric not found: %s", name)
}
