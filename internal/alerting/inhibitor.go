package alerting

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"
	"time"
)

// Inhibitor manages alert inhibition and silencing
type Inhibitor struct {
	store    *Store
	rules    []InhibitRule
	rulesMu  sync.RWMutex

	// Cache of firing alerts for inhibition checks
	firingAlerts map[string]*Alert
	alertsMu     sync.RWMutex

	// Dependency-aware alerting (optional)
	dependencyAlerting *DependencyAlerting
}

// NewInhibitor creates a new inhibitor
func NewInhibitor(store *Store) *Inhibitor {
	return &Inhibitor{
		store:        store,
		rules:        []InhibitRule{},
		firingAlerts: make(map[string]*Alert),
	}
}

// SetDependencyAlerting sets the dependency alerting instance for dependency-aware inhibition
func (i *Inhibitor) SetDependencyAlerting(da *DependencyAlerting) {
	i.dependencyAlerting = da
}

// SetRules sets the inhibition rules
func (i *Inhibitor) SetRules(rules []InhibitRule) {
	i.rulesMu.Lock()
	defer i.rulesMu.Unlock()
	i.rules = rules
}

// LoadRulesFromDB loads inhibition rules from database
func (i *Inhibitor) LoadRulesFromDB() error {
	rules, err := i.store.ListInhibitRules()
	if err != nil {
		return err
	}
	i.SetRules(rules)
	return nil
}

// UpdateFiringAlerts updates the cache of firing alerts
func (i *Inhibitor) UpdateFiringAlerts(alerts []*Alert) {
	i.alertsMu.Lock()
	defer i.alertsMu.Unlock()

	i.firingAlerts = make(map[string]*Alert)
	for _, alert := range alerts {
		if alert.State == StateFiring {
			i.firingAlerts[alert.Fingerprint] = alert
		}
	}
}

// IsInhibited checks if an alert should be inhibited
func (i *Inhibitor) IsInhibited(alert *Alert) bool {
	i.rulesMu.RLock()
	rules := i.rules
	i.rulesMu.RUnlock()

	i.alertsMu.RLock()
	firingAlerts := i.firingAlerts
	i.alertsMu.RUnlock()

	for _, rule := range rules {
		if i.isInhibitedByRule(alert, &rule, firingAlerts) {
			return true
		}
	}

	return false
}

// isInhibitedByRule checks if alert is inhibited by a specific rule
func (i *Inhibitor) isInhibitedByRule(target *Alert, rule *InhibitRule, firingAlerts map[string]*Alert) bool {
	// Check if target matches the target matchers
	if !matchesAllMatchers(target.Labels, rule.TargetMatch) {
		return false
	}

	// Look for a source alert that inhibits this target
	for _, source := range firingAlerts {
		// Skip self
		if source.Fingerprint == target.Fingerprint {
			continue
		}

		// Check if source matches source matchers
		if !matchesAllMatchers(source.Labels, rule.SourceMatch) {
			continue
		}

		// Check if equal labels match
		equalMatch := true
		for _, label := range rule.EqualLabels {
			if source.Labels[label] != target.Labels[label] {
				equalMatch = false
				break
			}
		}

		if equalMatch {
			return true
		}
	}

	return false
}

// IsSilenced checks if an alert is silenced
func (i *Inhibitor) IsSilenced(alert *Alert) (bool, *Silence) {
	silences, err := i.store.ListActiveSilences()
	if err != nil {
		return false, nil
	}

	for _, silence := range silences {
		if matchesAllMatchers(alert.Labels, silence.Matchers) {
			return true, &silence
		}
	}

	return false, nil
}

// ShouldSuppress checks if an alert should be suppressed (silenced or inhibited)
func (i *Inhibitor) ShouldSuppress(alert *Alert) (bool, string) {
	// Check silences first
	if silenced, silence := i.IsSilenced(alert); silenced {
		return true, "silenced:" + silence.ID
	}

	// Check label-based inhibition
	if i.IsInhibited(alert) {
		return true, "inhibited"
	}

	// Check dependency-based inhibition
	if i.dependencyAlerting != nil {
		if suppressed, reason := i.dependencyAlerting.IsInhibitedByDependency(alert); suppressed {
			return true, "dependency:" + reason
		}
	}

	return false, ""
}

// matchesAllMatchers checks if labels match all matchers
func matchesAllMatchers(labels map[string]string, matchers []Matcher) bool {
	for _, matcher := range matchers {
		labelValue, exists := labels[matcher.Name]
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

// Silence management methods for Store

// ListInhibitRules returns all inhibit rules
func (s *Store) ListInhibitRules() ([]InhibitRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, source_match, target_match, equal_labels, created_at
		FROM inhibit_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []InhibitRule
	for rows.Next() {
		var rule InhibitRule
		var sourceMatch, targetMatch, equalLabels string
		var createdAt int64

		if err := rows.Scan(&rule.ID, &rule.Name, &sourceMatch, &targetMatch, &equalLabels, &createdAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(sourceMatch), &rule.SourceMatch)
		json.Unmarshal([]byte(targetMatch), &rule.TargetMatch)
		json.Unmarshal([]byte(equalLabels), &rule.EqualLabels)
		rule.CreatedAt = time.Unix(createdAt, 0)

		rules = append(rules, rule)
	}

	return rules, nil
}

// CreateInhibitRule creates a new inhibit rule
func (s *Store) CreateInhibitRule(rule *InhibitRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rule.CreatedAt = time.Now()

	sourceMatch, _ := json.Marshal(rule.SourceMatch)
	targetMatch, _ := json.Marshal(rule.TargetMatch)
	equalLabels, _ := json.Marshal(rule.EqualLabels)

	_, err := s.db.Exec(`
		INSERT INTO inhibit_rules (id, name, source_match, target_match, equal_labels, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.Name, string(sourceMatch), string(targetMatch),
		string(equalLabels), rule.CreatedAt.Unix())

	return err
}

// DeleteInhibitRule removes an inhibit rule
func (s *Store) DeleteInhibitRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM inhibit_rules WHERE id = ?", id)
	return err
}

// GetSilence retrieves a silence by ID
func (s *Store) GetSilence(id string) (*Silence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var silence Silence
	var matchers string
	var startsAt, endsAt, createdAt int64

	err := s.db.QueryRow(`
		SELECT id, matchers, starts_at, ends_at, created_by, comment, created_at
		FROM silences WHERE id = ?`, id).Scan(
		&silence.ID, &matchers, &startsAt, &endsAt,
		&silence.CreatedBy, &silence.Comment, &createdAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(matchers), &silence.Matchers)
	silence.StartsAt = time.Unix(startsAt, 0)
	silence.EndsAt = time.Unix(endsAt, 0)
	silence.CreatedAt = time.Unix(createdAt, 0)

	return &silence, nil
}

// ListAllSilences returns all silences (including expired)
func (s *Store) ListAllSilences() ([]Silence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, matchers, starts_at, ends_at, created_by, comment, created_at
		FROM silences ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var silences []Silence
	for rows.Next() {
		var silence Silence
		var matchers string
		var startsAt, endsAt, createdAt int64

		if err := rows.Scan(&silence.ID, &matchers, &startsAt, &endsAt,
			&silence.CreatedBy, &silence.Comment, &createdAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(matchers), &silence.Matchers)
		silence.StartsAt = time.Unix(startsAt, 0)
		silence.EndsAt = time.Unix(endsAt, 0)
		silence.CreatedAt = time.Unix(createdAt, 0)

		silences = append(silences, silence)
	}

	return silences, nil
}

// UpdateSilence updates a silence
func (s *Store) UpdateSilence(silence *Silence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	matchers, _ := json.Marshal(silence.Matchers)

	_, err := s.db.Exec(`
		UPDATE silences SET matchers=?, starts_at=?, ends_at=?, comment=?
		WHERE id=?`,
		string(matchers), silence.StartsAt.Unix(), silence.EndsAt.Unix(),
		silence.Comment, silence.ID)

	return err
}

// ExpireSilence immediately expires a silence
func (s *Store) ExpireSilence(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	_, err := s.db.Exec("UPDATE silences SET ends_at = ? WHERE id = ? AND ends_at > ?",
		now, id, now)
	return err
}

// CleanupExpiredSilences removes silences that have been expired for a while
func (s *Store) CleanupExpiredSilences(olderThan time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan).Unix()
	_, err := s.db.Exec("DELETE FROM silences WHERE ends_at < ?", cutoff)
	return err
}

// SilenceState represents the state of a silence
type SilenceState string

const (
	SilenceStateActive   SilenceState = "active"
	SilenceStatePending  SilenceState = "pending"
	SilenceStateExpired  SilenceState = "expired"
)

// GetState returns the current state of a silence
func (silence *Silence) GetState() SilenceState {
	now := time.Now()
	if now.Before(silence.StartsAt) {
		return SilenceStatePending
	}
	if now.After(silence.EndsAt) {
		return SilenceStateExpired
	}
	return SilenceStateActive
}

// SilenceWithState includes state information
type SilenceWithState struct {
	Silence
	State SilenceState `json:"state"`
}

// ListSilencesWithState returns silences with their current state
func (s *Store) ListSilencesWithState() ([]SilenceWithState, error) {
	silences, err := s.ListAllSilences()
	if err != nil {
		return nil, err
	}

	result := make([]SilenceWithState, len(silences))
	for i, silence := range silences {
		result[i] = SilenceWithState{
			Silence: silence,
			State:   silence.GetState(),
		}
	}

	return result, nil
}

// AlertManager combines all alerting components
type AlertManager struct {
	Store              *Store
	Evaluator          *Evaluator
	Router             *Router
	Inhibitor          *Inhibitor
	DependencyAlerting *DependencyAlerting

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewAlertManager creates a new alert manager
func NewAlertManager(dbPath string, metricsProvider MetricsProvider) (*AlertManager, error) {
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, err
	}

	evaluator := NewEvaluator(store, metricsProvider)
	router := NewRouter(store)
	inhibitor := NewInhibitor(store)

	am := &AlertManager{
		Store:     store,
		Evaluator: evaluator,
		Router:    router,
		Inhibitor: inhibitor,
		stopCh:    make(chan struct{}),
	}

	// Wire up evaluator to router
	evaluator.SetNotifyFunc(func(alert *Alert, rule *Rule) {
		// Check suppression
		if suppressed, reason := inhibitor.ShouldSuppress(alert); suppressed {
			log.Printf("[alerting] Alert %s suppressed: %s", alert.ID, reason)
			return
		}
		router.Route(alert)
	})

	return am, nil
}

// SetCatalogStore sets the catalog store for dependency-aware alerting
func (am *AlertManager) SetCatalogStore(catalogStore interface{}) {
	// Type assert to catalog.Store
	if cs, ok := catalogStore.(interface {
		GetService(id string) (interface{}, error)
		GetServiceGraph(orgID string) (interface{}, error)
		UpdateServiceHealth(id string, health interface{}) error
	}); ok {
		// We need to use the actual catalog.Store type
		// This is a workaround to avoid import cycles
		log.Printf("[alerting] Catalog store connected for dependency-aware alerting")
		_ = cs // Will be used when we have proper type assertion
	}
}

// EnableDependencyAlerting enables dependency-aware alerting with the given catalog store
func (am *AlertManager) EnableDependencyAlerting(da *DependencyAlerting) {
	am.DependencyAlerting = da
	am.Inhibitor.SetDependencyAlerting(da)
	log.Printf("[alerting] Dependency-aware alerting enabled")
}

// Start starts all alerting components
func (am *AlertManager) Start() {
	// Load inhibition rules
	if err := am.Inhibitor.LoadRulesFromDB(); err != nil {
		log.Printf("[alerting] Error loading inhibit rules: %v", err)
	}

	am.Evaluator.Start()
	am.Router.Start()

	// Start maintenance loop
	am.wg.Add(1)
	go am.maintenanceLoop()

	log.Println("[alerting] AlertManager started")
}

// Stop stops all alerting components
func (am *AlertManager) Stop() {
	close(am.stopCh)
	am.wg.Wait()

	am.Evaluator.Stop()
	am.Router.Stop()
	am.Store.Close()

	log.Println("[alerting] AlertManager stopped")
}

// maintenanceLoop performs periodic maintenance
func (am *AlertManager) maintenanceLoop() {
	defer am.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopCh:
			return
		case <-ticker.C:
			// Update inhibitor's firing alerts cache
			firingAlerts := am.Evaluator.GetFiringAlerts()
			am.Inhibitor.UpdateFiringAlerts(firingAlerts)

			// Update service health states for dependency alerting
			if am.DependencyAlerting != nil {
				am.updateServiceHealthFromAlerts(firingAlerts)
			}

			// Cleanup old silences
			am.Store.CleanupExpiredSilences(7 * 24 * time.Hour)

			// Reload inhibit rules
			am.Inhibitor.LoadRulesFromDB()
		}
	}
}

// updateServiceHealthFromAlerts updates service health based on firing alerts
func (am *AlertManager) updateServiceHealthFromAlerts(alerts []*Alert) {
	if am.DependencyAlerting == nil {
		return
	}

	// Group alerts by service
	serviceAlerts := make(map[string][]*Alert)
	for _, alert := range alerts {
		serviceID := alert.Labels["service"]
		if serviceID == "" {
			serviceID = alert.Labels["service_id"]
		}
		if serviceID != "" {
			serviceAlerts[serviceID] = append(serviceAlerts[serviceID], alert)
		}
	}

	// Update health for each service
	for serviceID, svcAlerts := range serviceAlerts {
		serviceName := serviceID
		if len(svcAlerts) > 0 && svcAlerts[0].Labels["service_name"] != "" {
			serviceName = svcAlerts[0].Labels["service_name"]
		}
		am.DependencyAlerting.UpdateServiceHealth(serviceID, serviceName, svcAlerts)
	}
}

// GetBlastRadius returns the blast radius for a service
func (am *AlertManager) GetBlastRadius(serviceID string) *BlastRadius {
	if am.DependencyAlerting == nil {
		return nil
	}
	return am.DependencyAlerting.CalculateBlastRadius(serviceID)
}

// GetDependencyContext returns dependency context for an alert
func (am *AlertManager) GetDependencyContext(alert *Alert) *DependencyContext {
	if am.DependencyAlerting == nil {
		return nil
	}
	return am.DependencyAlerting.GetDependencyContext(alert)
}
