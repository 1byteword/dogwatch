package catalog

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// TraceData represents trace data for service discovery
type TraceData struct {
	ServiceName   string
	SpanName      string
	ParentService string // For dependency detection
	TraceID       string
	SpanID        string
	Duration      time.Duration
	StatusCode    int
	Timestamp     time.Time
}

// K8sServiceInfo represents Kubernetes service information
type K8sServiceInfo struct {
	Name       string
	Namespace  string
	Deployment string
	Labels     map[string]string
	Ports      []int
}

// DiscoveryService automatically discovers services and dependencies
type DiscoveryService struct {
	store           *Store
	orgID           string
	mu              sync.Mutex
	serviceCache    map[string]time.Time // service name -> last seen
	dependencyCache map[string]time.Time // "source:target" -> last seen
	ticker          *time.Ticker
	done            chan bool
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(store *Store, orgID string) *DiscoveryService {
	return &DiscoveryService{
		store:           store,
		orgID:           orgID,
		serviceCache:    make(map[string]time.Time),
		dependencyCache: make(map[string]time.Time),
		done:            make(chan bool),
	}
}

// Start begins the discovery service
func (d *DiscoveryService) Start() {
	d.ticker = time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-d.ticker.C:
				d.cleanupStaleData()
			case <-d.done:
				return
			}
		}
	}()
}

// Stop stops the discovery service
func (d *DiscoveryService) Stop() {
	if d.ticker != nil {
		d.ticker.Stop()
	}
	close(d.done)
}

// ProcessTrace processes trace data for service discovery
func (d *DiscoveryService) ProcessTrace(trace TraceData) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Discover/update service
	if _, exists := d.serviceCache[trace.ServiceName]; !exists {
		d.ensureService(trace.ServiceName)
	}
	d.serviceCache[trace.ServiceName] = now

	// Discover dependency if parent service exists
	if trace.ParentService != "" && trace.ParentService != trace.ServiceName {
		depKey := trace.ParentService + ":" + trace.ServiceName
		if _, exists := d.dependencyCache[depKey]; !exists {
			d.ensureDependency(trace.ParentService, trace.ServiceName)
		}
		d.dependencyCache[depKey] = now
	}
}

// ensureService creates a service if it doesn't exist
func (d *DiscoveryService) ensureService(serviceName string) {
	// Check if already exists
	_, err := d.store.GetServiceByName(d.orgID, serviceName)
	if err == nil {
		return // Already exists
	}

	// Create new service
	svc := &Service{
		ID:          fmt.Sprintf("svc_%d", time.Now().UnixNano()),
		OrgID:       d.orgID,
		Name:        serviceName,
		DisplayName: formatServiceName(serviceName),
		Tier:        TierMedium,
		Lifecycle:   LifecycleActive,
		Health:      HealthUnknown,
		Tags:        []string{"auto-discovered"},
	}

	if err := d.store.CreateService(svc); err != nil {
		log.Printf("[catalog] Failed to auto-create service %s: %v", serviceName, err)
	} else {
		log.Printf("[catalog] Auto-discovered service: %s", serviceName)
	}
}

// ensureDependency creates a dependency if it doesn't exist
func (d *DiscoveryService) ensureDependency(sourceService, targetService string) {
	// Get service IDs
	source, err := d.store.GetServiceByName(d.orgID, sourceService)
	if err != nil {
		d.ensureService(sourceService)
		source, _ = d.store.GetServiceByName(d.orgID, sourceService)
	}

	target, err := d.store.GetServiceByName(d.orgID, targetService)
	if err != nil {
		d.ensureService(targetService)
		target, _ = d.store.GetServiceByName(d.orgID, targetService)
	}

	if source == nil || target == nil {
		return
	}

	dep := &Dependency{
		ID:             fmt.Sprintf("dep_%d", time.Now().UnixNano()),
		SourceService:  source.ID,
		TargetService:  target.ID,
		DependencyType: DepTypeSync,
		IsAutoDetected: true,
		Confidence:     0.8,
	}

	if err := d.store.AddDependency(dep); err != nil {
		log.Printf("[catalog] Failed to auto-create dependency %s->%s: %v",
			sourceService, targetService, err)
	} else {
		log.Printf("[catalog] Auto-discovered dependency: %s -> %s",
			sourceService, targetService)
	}
}

// ProcessK8sService processes Kubernetes service info
func (d *DiscoveryService) ProcessK8sService(info K8sServiceInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if service exists
	svc, err := d.store.GetServiceByName(d.orgID, info.Name)
	if err != nil {
		// Create new service from K8s info
		svc = &Service{
			ID:            fmt.Sprintf("svc_%d", time.Now().UnixNano()),
			OrgID:         d.orgID,
			Name:          info.Name,
			DisplayName:   formatServiceName(info.Name),
			Tier:          TierMedium,
			Lifecycle:     LifecycleActive,
			Health:        HealthUnknown,
			K8sNamespace:  info.Namespace,
			K8sDeployment: info.Deployment,
			K8sService:    info.Name,
			Tags:          []string{"kubernetes", info.Namespace},
		}

		// Extract team from labels if present
		if team, ok := info.Labels["team"]; ok {
			svc.TeamName = team
		}
		if owner, ok := info.Labels["owner"]; ok {
			svc.OwnerEmail = owner
		}

		if err := d.store.CreateService(svc); err != nil {
			log.Printf("[catalog] Failed to create K8s service %s: %v", info.Name, err)
		} else {
			log.Printf("[catalog] Discovered K8s service: %s/%s", info.Namespace, info.Name)
		}
	} else {
		// Update existing service with K8s info
		svc.K8sNamespace = info.Namespace
		svc.K8sDeployment = info.Deployment
		svc.K8sService = info.Name

		if err := d.store.UpdateService(svc); err != nil {
			log.Printf("[catalog] Failed to update K8s service %s: %v", info.Name, err)
		}
	}
}

// SyncFromTraces syncs services from trace data (call periodically)
func (d *DiscoveryService) SyncFromTraces(getServices func() []string, getDeps func() [][2]string) {
	// Get all services from traces
	for _, svc := range getServices() {
		d.mu.Lock()
		if _, exists := d.serviceCache[svc]; !exists {
			d.ensureService(svc)
		}
		d.serviceCache[svc] = time.Now()
		d.mu.Unlock()
	}

	// Get all dependencies from traces
	for _, dep := range getDeps() {
		d.mu.Lock()
		depKey := dep[0] + ":" + dep[1]
		if _, exists := d.dependencyCache[depKey]; !exists {
			d.ensureDependency(dep[0], dep[1])
		}
		d.dependencyCache[depKey] = time.Now()
		d.mu.Unlock()
	}
}

// cleanupStaleData removes services/dependencies not seen recently
func (d *DiscoveryService) cleanupStaleData() {
	d.mu.Lock()
	defer d.mu.Unlock()

	staleThreshold := time.Now().Add(-24 * time.Hour)

	// Clean up stale service cache entries
	for name, lastSeen := range d.serviceCache {
		if lastSeen.Before(staleThreshold) {
			delete(d.serviceCache, name)
		}
	}

	// Clean up stale dependency cache entries
	for key, lastSeen := range d.dependencyCache {
		if lastSeen.Before(staleThreshold) {
			delete(d.dependencyCache, key)
		}
	}
}

// UpdateServiceHealthFromSynthetics updates service health based on synthetic checks
func (d *DiscoveryService) UpdateServiceHealthFromSynthetics(serviceID string, passing bool, responseTime float64) {
	health := HealthHealthy
	if !passing {
		health = HealthUnhealthy
	} else if responseTime > 1000 { // > 1 second is degraded
		health = HealthDegraded
	}

	if err := d.store.UpdateServiceHealth(serviceID, health); err != nil {
		log.Printf("[catalog] Failed to update service health: %v", err)
	}
}

// formatServiceName formats a service name for display
func formatServiceName(name string) string {
	// Convert kebab-case or snake_case to Title Case
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// GetServiceByName is a helper to look up a service by name
func (d *DiscoveryService) GetServiceByName(name string) (*Service, error) {
	return d.store.GetServiceByName(d.orgID, name)
}

// EnrichIncidentWithService enriches an incident with service context
func (d *DiscoveryService) EnrichIncidentWithService(serviceName string) (*ServiceContext, error) {
	svc, err := d.store.GetServiceByName(d.orgID, serviceName)
	if err != nil {
		return nil, err
	}

	upstream, downstream, _ := d.store.GetDependencies(svc.ID)
	runbooks, _ := d.store.GetRunbooksForService(svc.ID)

	return &ServiceContext{
		Service:    svc,
		Upstream:   upstream,
		Downstream: downstream,
		Runbooks:   runbooks,
	}, nil
}

// ServiceContext provides full context about a service for incident response
type ServiceContext struct {
	Service    *Service      `json:"service"`
	Upstream   []*Dependency `json:"upstream_dependencies"`
	Downstream []*Dependency `json:"downstream_dependencies"`
	Runbooks   []*Runbook    `json:"runbooks"`
}
