package alerting

import (
	"log"
	"sync"
	"time"

	"dogwatch/internal/catalog"
)

// DependencyAlerting provides dependency-aware alerting capabilities
type DependencyAlerting struct {
	catalogStore *catalog.Store
	mu           sync.RWMutex

	// Cache of service health states derived from alerts
	serviceHealth map[string]ServiceAlertState

	// Cache of dependencies for quick lookup
	upstreamCache   map[string][]string // service -> services it depends on
	downstreamCache map[string][]string // service -> services that depend on it
	cacheUpdatedAt  time.Time
	cacheTTL        time.Duration
}

// ServiceAlertState tracks alerting state for a service
type ServiceAlertState struct {
	ServiceID       string
	ServiceName     string
	HasFiringAlerts bool
	FiringAlertIDs  []string
	Severity        Severity
	LastUpdated     time.Time
}

// BlastRadius represents the impact of a service failure
type BlastRadius struct {
	FailedService     string               `json:"failed_service"`
	FailedServiceName string               `json:"failed_service_name"`
	AffectedServices  []AffectedService    `json:"affected_services"`
	TotalAffected     int                  `json:"total_affected"`
	CriticalAffected  int                  `json:"critical_affected"`
	PropagationDepth  int                  `json:"propagation_depth"`
	EstimatedImpact   string               `json:"estimated_impact"` // "critical", "high", "medium", "low"
}

// AffectedService represents a service affected by a failure
type AffectedService struct {
	ServiceID   string              `json:"service_id"`
	ServiceName string              `json:"service_name"`
	Tier        catalog.ServiceTier `json:"tier"`
	Depth       int                 `json:"depth"` // How many hops from the failed service
	Path        []string            `json:"path"`  // Dependency path from failed service
}

// DependencyContext provides context about an alert's dependencies
type DependencyContext struct {
	// Upstream services that this alert's service depends on
	UpstreamServices []ServiceAlertState `json:"upstream_services"`

	// Is this alert likely a symptom of an upstream failure?
	IsLikelySymptom bool   `json:"is_likely_symptom"`
	RootCauseHint   string `json:"root_cause_hint,omitempty"`

	// Downstream impact
	BlastRadius *BlastRadius `json:"blast_radius,omitempty"`

	// Should this alert be suppressed due to upstream failure?
	ShouldSuppress   bool   `json:"should_suppress"`
	SuppressionReason string `json:"suppression_reason,omitempty"`
}

// NewDependencyAlerting creates a new dependency alerting instance
func NewDependencyAlerting(catalogStore *catalog.Store) *DependencyAlerting {
	return &DependencyAlerting{
		catalogStore:    catalogStore,
		serviceHealth:   make(map[string]ServiceAlertState),
		upstreamCache:   make(map[string][]string),
		downstreamCache: make(map[string][]string),
		cacheTTL:        5 * time.Minute,
	}
}

// severityRank returns the numeric rank of a severity (higher = more severe)
func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// UpdateServiceHealth updates the health state for a service based on alerts
func (d *DependencyAlerting) UpdateServiceHealth(serviceID, serviceName string, firingAlerts []*Alert) {
	d.mu.Lock()
	defer d.mu.Unlock()

	state := ServiceAlertState{
		ServiceID:       serviceID,
		ServiceName:     serviceName,
		HasFiringAlerts: len(firingAlerts) > 0,
		FiringAlertIDs:  make([]string, 0, len(firingAlerts)),
		Severity:        SeverityInfo,
		LastUpdated:     time.Now(),
	}

	for _, alert := range firingAlerts {
		state.FiringAlertIDs = append(state.FiringAlertIDs, alert.ID)
		if severityRank(alert.Severity) > severityRank(state.Severity) {
			state.Severity = alert.Severity
		}
	}

	d.serviceHealth[serviceID] = state

	// Also update the catalog health
	if d.catalogStore != nil {
		health := catalog.HealthHealthy
		if len(firingAlerts) > 0 {
			health = catalog.HealthUnhealthy
			for _, alert := range firingAlerts {
				if alert.Severity == SeverityWarning {
					health = catalog.HealthDegraded
					break
				}
			}
		}
		d.catalogStore.UpdateServiceHealth(serviceID, health)
	}
}

// refreshDependencyCache refreshes the dependency cache if stale
func (d *DependencyAlerting) refreshDependencyCache() {
	if d.catalogStore == nil {
		return
	}

	if time.Since(d.cacheUpdatedAt) < d.cacheTTL {
		return
	}

	// Get all services - use default org for now
	graph, err := d.catalogStore.GetServiceGraph("default")
	if err != nil {
		log.Printf("[dependency-alerting] Error getting service graph: %v", err)
		return
	}

	d.upstreamCache = make(map[string][]string)
	d.downstreamCache = make(map[string][]string)

	for _, dep := range graph.Dependencies {
		// Source depends on Target (Source -> Target)
		// So Target is upstream of Source
		d.upstreamCache[dep.SourceService] = append(d.upstreamCache[dep.SourceService], dep.TargetService)
		// And Source is downstream of Target
		d.downstreamCache[dep.TargetService] = append(d.downstreamCache[dep.TargetService], dep.SourceService)
	}

	d.cacheUpdatedAt = time.Now()
}

// GetUpstreamServices returns services that the given service depends on
func (d *DependencyAlerting) GetUpstreamServices(serviceID string) []string {
	d.mu.Lock()
	d.refreshDependencyCache()
	d.mu.Unlock()

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.upstreamCache[serviceID]
}

// GetDownstreamServices returns services that depend on the given service
func (d *DependencyAlerting) GetDownstreamServices(serviceID string) []string {
	d.mu.Lock()
	d.refreshDependencyCache()
	d.mu.Unlock()

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.downstreamCache[serviceID]
}

// GetDependencyContext returns dependency context for an alert
func (d *DependencyAlerting) GetDependencyContext(alert *Alert) *DependencyContext {
	serviceID := alert.Labels["service"]
	if serviceID == "" {
		serviceID = alert.Labels["service_id"]
	}
	if serviceID == "" {
		// No service label, can't determine dependencies
		return &DependencyContext{}
	}

	ctx := &DependencyContext{
		UpstreamServices: []ServiceAlertState{},
	}

	// Get upstream services
	upstreams := d.GetUpstreamServices(serviceID)

	d.mu.RLock()
	for _, upstreamID := range upstreams {
		if state, exists := d.serviceHealth[upstreamID]; exists {
			ctx.UpstreamServices = append(ctx.UpstreamServices, state)

			// Check if upstream is unhealthy
			if state.HasFiringAlerts {
				ctx.IsLikelySymptom = true
				if ctx.RootCauseHint == "" {
					ctx.RootCauseHint = "Upstream service '" + state.ServiceName + "' has active alerts"
				}
			}
		}
	}
	d.mu.RUnlock()

	// Determine if we should suppress
	ctx.ShouldSuppress, ctx.SuppressionReason = d.shouldSuppressDueToUpstream(alert, ctx)

	// Calculate blast radius if this is a root cause alert
	if !ctx.IsLikelySymptom {
		ctx.BlastRadius = d.CalculateBlastRadius(serviceID)
	}

	return ctx
}

// shouldSuppressDueToUpstream determines if an alert should be suppressed
func (d *DependencyAlerting) shouldSuppressDueToUpstream(alert *Alert, ctx *DependencyContext) (bool, string) {
	if !ctx.IsLikelySymptom {
		return false, ""
	}

	// Find the most severe upstream alert
	var worstUpstream *ServiceAlertState
	for i := range ctx.UpstreamServices {
		upstream := &ctx.UpstreamServices[i]
		if upstream.HasFiringAlerts {
			if worstUpstream == nil || severityRank(upstream.Severity) > severityRank(worstUpstream.Severity) {
				worstUpstream = upstream
			}
		}
	}

	if worstUpstream == nil {
		return false, ""
	}

	// Suppress if:
	// 1. Upstream has critical alert and this is warning
	// 2. Upstream has same or higher severity
	// 3. Alert is likely caused by the upstream (connection/timeout type)
	alertType := alert.Labels["alertname"]
	isConnectionAlert := containsAny(alertType, []string{"timeout", "connection", "unavailable", "error_rate"})

	if severityRank(worstUpstream.Severity) >= severityRank(alert.Severity) && isConnectionAlert {
		return true, "Upstream service '" + worstUpstream.ServiceName + "' has " +
			string(worstUpstream.Severity) + " alerts - this alert is likely a symptom"
	}

	// For critical upstream failures, always suppress downstream warnings
	if worstUpstream.Severity == SeverityCritical && severityRank(alert.Severity) <= severityRank(SeverityWarning) {
		return true, "Upstream service '" + worstUpstream.ServiceName + "' has critical alert"
	}

	return false, ""
}

// CalculateBlastRadius calculates the impact of a service failure
func (d *DependencyAlerting) CalculateBlastRadius(serviceID string) *BlastRadius {
	if d.catalogStore == nil {
		return nil
	}

	d.mu.Lock()
	d.refreshDependencyCache()
	d.mu.Unlock()

	// Get service info
	service, err := d.catalogStore.GetService(serviceID)
	if err != nil {
		return nil
	}

	br := &BlastRadius{
		FailedService:     serviceID,
		FailedServiceName: service.Name,
		AffectedServices:  []AffectedService{},
	}

	// BFS to find all downstream services
	visited := make(map[string]bool)
	visited[serviceID] = true

	type queueItem struct {
		serviceID string
		depth     int
		path      []string
	}

	queue := []queueItem{{serviceID, 0, []string{service.Name}}}

	d.mu.RLock()
	defer d.mu.RUnlock()

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Get downstream services
		for _, downstreamID := range d.downstreamCache[current.serviceID] {
			if visited[downstreamID] {
				continue
			}
			visited[downstreamID] = true

			// Get service details
			downstreamSvc, err := d.catalogStore.GetService(downstreamID)
			if err != nil {
				continue
			}

			newPath := make([]string, len(current.path))
			copy(newPath, current.path)
			newPath = append(newPath, downstreamSvc.Name)

			affected := AffectedService{
				ServiceID:   downstreamID,
				ServiceName: downstreamSvc.Name,
				Tier:        downstreamSvc.Tier,
				Depth:       current.depth + 1,
				Path:        newPath,
			}

			br.AffectedServices = append(br.AffectedServices, affected)
			br.TotalAffected++

			if downstreamSvc.Tier == catalog.TierCritical {
				br.CriticalAffected++
			}

			if current.depth+1 > br.PropagationDepth {
				br.PropagationDepth = current.depth + 1
			}

			// Continue BFS
			queue = append(queue, queueItem{downstreamID, current.depth + 1, newPath})
		}
	}

	// Determine estimated impact
	br.EstimatedImpact = d.estimateImpact(service.Tier, br.CriticalAffected, br.TotalAffected)

	return br
}

// estimateImpact estimates the overall impact of a failure
func (d *DependencyAlerting) estimateImpact(failedTier catalog.ServiceTier, criticalAffected, totalAffected int) string {
	return d.estimateImpactByString(string(failedTier), criticalAffected, totalAffected)
}

// estimateImpactByString estimates impact using string tier (for testing)
func (d *DependencyAlerting) estimateImpactByString(failedTier string, criticalAffected, totalAffected int) string {
	// Critical service failure or affects critical services
	if failedTier == "critical" || criticalAffected > 0 {
		return "critical"
	}

	// High tier failure with multiple downstream services
	if failedTier == "high" && totalAffected > 3 {
		return "high"
	}

	// Multiple services affected
	if totalAffected > 5 {
		return "high"
	}

	if totalAffected > 2 {
		return "medium"
	}

	return "low"
}

// IsInhibitedByDependency checks if an alert should be inhibited due to upstream failures
func (d *DependencyAlerting) IsInhibitedByDependency(alert *Alert) (bool, string) {
	ctx := d.GetDependencyContext(alert)
	return ctx.ShouldSuppress, ctx.SuppressionReason
}

// GetServiceAlertState returns the current alert state for a service
func (d *DependencyAlerting) GetServiceAlertState(serviceID string) (ServiceAlertState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	state, exists := d.serviceHealth[serviceID]
	return state, exists
}

// GetUnhealthyUpstreams returns upstream services that have firing alerts
func (d *DependencyAlerting) GetUnhealthyUpstreams(serviceID string) []ServiceAlertState {
	upstreams := d.GetUpstreamServices(serviceID)
	var unhealthy []ServiceAlertState

	d.mu.RLock()
	defer d.mu.RUnlock()

	for _, upstreamID := range upstreams {
		if state, exists := d.serviceHealth[upstreamID]; exists && state.HasFiringAlerts {
			unhealthy = append(unhealthy, state)
		}
	}

	return unhealthy
}

// FindRootCause attempts to find the root cause service for an alert
func (d *DependencyAlerting) FindRootCause(serviceID string) *ServiceAlertState {
	// Refresh cache first
	d.mu.Lock()
	d.refreshDependencyCache()
	d.mu.Unlock()

	visited := make(map[string]bool)

	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.findRootCauseRecursive(serviceID, visited)
}

// findRootCauseRecursive finds the deepest unhealthy upstream (must be called with mu.RLock held)
func (d *DependencyAlerting) findRootCauseRecursive(serviceID string, visited map[string]bool) *ServiceAlertState {
	if visited[serviceID] {
		return nil
	}
	visited[serviceID] = true

	upstreams := d.upstreamCache[serviceID]

	var deepestUnhealthy *ServiceAlertState

	for _, upstreamID := range upstreams {
		// Check if this upstream has alerts
		state, exists := d.serviceHealth[upstreamID]
		if !exists || !state.HasFiringAlerts {
			continue
		}

		// Recursively check further upstream
		deeper := d.findRootCauseRecursive(upstreamID, visited)
		if deeper != nil {
			deepestUnhealthy = deeper
		} else {
			// This is the deepest unhealthy service in this path
			stateCopy := state
			deepestUnhealthy = &stateCopy
		}
	}

	return deepestUnhealthy
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
