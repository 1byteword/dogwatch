package entity

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TraceSpan represents a span from trace data
type TraceSpan struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	ServiceName   string
	OperationName string
	SpanKind      string // CLIENT, SERVER, PRODUCER, CONSUMER, INTERNAL
	Duration      time.Duration
	StatusCode    int
	Timestamp     time.Time
	Attributes    map[string]string
}

// MetricData represents metric data for entity synthesis
type MetricData struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp time.Time
}

// K8sPod represents Kubernetes pod information
type K8sPod struct {
	Name       string
	Namespace  string
	NodeName   string
	Containers []string
	Labels     map[string]string
	Phase      string
	IP         string
}

// Synthesizer automatically discovers and synthesizes entities from telemetry
type Synthesizer struct {
	entities map[string]*Entity // id -> entity
	mu       sync.RWMutex

	// Configuration
	orgID           string
	signalWindow    time.Duration
	staleThreshold  time.Duration

	// Callbacks
	onEntityCreated func(*Entity)
	onEntityUpdated func(*Entity)

	// Running state
	running bool
	stopCh  chan struct{}
}

// Config for the synthesizer
type Config struct {
	OrgID          string
	SignalWindow   time.Duration // How long to aggregate signals
	StaleThreshold time.Duration // When to mark entities as stale
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		OrgID:          "default",
		SignalWindow:   5 * time.Minute,
		StaleThreshold: 24 * time.Hour,
	}
}

// NewSynthesizer creates a new entity synthesizer
func NewSynthesizer(cfg Config) *Synthesizer {
	if cfg.SignalWindow == 0 {
		cfg.SignalWindow = 5 * time.Minute
	}
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = 24 * time.Hour
	}

	return &Synthesizer{
		entities:       make(map[string]*Entity),
		orgID:          cfg.OrgID,
		signalWindow:   cfg.SignalWindow,
		staleThreshold: cfg.StaleThreshold,
		stopCh:         make(chan struct{}),
	}
}

// Start begins background processing
func (s *Synthesizer) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.runMaintenance()
	log.Printf("[entity] Synthesizer started")
}

// Stop stops the synthesizer
func (s *Synthesizer) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	close(s.stopCh)
	log.Printf("[entity] Synthesizer stopped")
}

// SetCallbacks sets callbacks for entity events
func (s *Synthesizer) SetCallbacks(onCreate, onUpdate func(*Entity)) {
	s.mu.Lock()
	s.onEntityCreated = onCreate
	s.onEntityUpdated = onUpdate
	s.mu.Unlock()
}

// ProcessSpan processes a trace span for entity discovery
func (s *Synthesizer) ProcessSpan(span TraceSpan) {
	// Discover service entity
	svcEntity := s.getOrCreateEntity(TypeService, span.ServiceName)
	svcEntity.LastSeen = time.Now()
	svcEntity.LastActivity = span.Timestamp

	// Update signals
	s.updateServiceSignals(svcEntity, span)

	// Discover relationships and other entities
	s.discoverFromSpan(svcEntity, span)
}

// ProcessMetric processes a metric for entity discovery
func (s *Synthesizer) ProcessMetric(metric MetricData) {
	// Discover host from system metrics
	if host, ok := metric.Labels["host"]; ok {
		hostEntity := s.getOrCreateEntity(TypeHost, host)
		hostEntity.LastSeen = time.Now()
		s.updateHostSignals(hostEntity, metric)
	}

	// Discover container
	if container, ok := metric.Labels["container"]; ok {
		containerEntity := s.getOrCreateEntity(TypeContainer, container)
		containerEntity.LastSeen = time.Now()

		// Add RUNS_ON relationship to host
		if host, ok := metric.Labels["host"]; ok {
			containerEntity.AddRelationship(RelRunsOn, makeID(TypeHost, host), host, TypeHost)
		}

		s.updateContainerSignals(containerEntity, metric)
	}

	// Discover service from metrics with service label
	if svc, ok := metric.Labels["service"]; ok {
		svcEntity := s.getOrCreateEntity(TypeService, svc)
		svcEntity.LastSeen = time.Now()
		s.updateServiceMetricSignals(svcEntity, metric)
	}
}

// ProcessK8sPod processes Kubernetes pod for entity discovery
func (s *Synthesizer) ProcessK8sPod(pod K8sPod) {
	// Create container entities for each container
	for _, container := range pod.Containers {
		containerID := fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, container)
		containerEntity := s.getOrCreateEntity(TypeContainer, containerID)
		containerEntity.LastSeen = time.Now()
		containerEntity.Tags["namespace"] = pod.Namespace
		containerEntity.Tags["pod"] = pod.Name
		containerEntity.Tags["node"] = pod.NodeName

		// Add RUNS_ON relationship to node (host)
		containerEntity.AddRelationship(RelRunsOn, makeID(TypeHost, pod.NodeName), pod.NodeName, TypeHost)

		// Copy labels
		for k, v := range pod.Labels {
			containerEntity.Tags[k] = v
		}

		// If there's an app or service label, link to service
		if svc, ok := pod.Labels["app"]; ok {
			svcEntity := s.getOrCreateEntity(TypeService, svc)
			svcEntity.AddRelationship(RelContains, containerEntity.ID, containerEntity.Name, TypeContainer)
		}
	}

	// Create host entity for the node
	nodeEntity := s.getOrCreateEntity(TypeHost, pod.NodeName)
	nodeEntity.LastSeen = time.Now()
	nodeEntity.Tags["kubernetes"] = "true"
}

// getOrCreateEntity gets or creates an entity
func (s *Synthesizer) getOrCreateEntity(entityType Type, name string) *Entity {
	id := makeID(entityType, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if entity, exists := s.entities[id]; exists {
		return entity
	}

	entity := &Entity{
		ID:          id,
		Type:        entityType,
		Name:        name,
		DisplayName: formatDisplayName(name),
		Tags:        make(map[string]string),
		Health:      HealthUnknown,
		Source:      "auto-discovered",
		Confidence:  0.8,
		FirstSeen:   time.Now(),
		LastSeen:    time.Now(),
		Signals: GoldenSignals{
			ThroughputUnit: getDefaultThroughputUnit(entityType),
			SaturationUnit: getDefaultSaturationUnit(entityType),
		},
	}

	s.entities[id] = entity

	if s.onEntityCreated != nil {
		go s.onEntityCreated(entity)
	}

	log.Printf("[entity] Discovered %s: %s", entityType, name)
	return entity
}

// discoverFromSpan discovers entities and relationships from a span
func (s *Synthesizer) discoverFromSpan(svcEntity *Entity, span TraceSpan) {
	// Discover database from span attributes
	if dbSystem, ok := span.Attributes["db.system"]; ok {
		dbName := span.Attributes["db.name"]
		if dbName == "" {
			dbName = dbSystem
		}
		dbEntity := s.getOrCreateEntity(TypeDatabase, dbName)
		dbEntity.LastSeen = time.Now()
		dbEntity.Tags["db.system"] = dbSystem

		// Add DEPENDS_ON relationship
		svcEntity.AddRelationship(RelDependsOn, dbEntity.ID, dbEntity.Name, TypeDatabase)
	}

	// Discover message queue
	if msgSystem, ok := span.Attributes["messaging.system"]; ok {
		queueName := span.Attributes["messaging.destination"]
		if queueName == "" {
			queueName = msgSystem
		}
		queueEntity := s.getOrCreateEntity(TypeQueue, queueName)
		queueEntity.LastSeen = time.Now()
		queueEntity.Tags["messaging.system"] = msgSystem

		// Add relationship based on span kind
		if span.SpanKind == "PRODUCER" {
			svcEntity.AddRelationship(RelConnectsTo, queueEntity.ID, queueEntity.Name, TypeQueue)
		} else if span.SpanKind == "CONSUMER" {
			queueEntity.AddRelationship(RelConnectsTo, svcEntity.ID, svcEntity.Name, TypeService)
		}
	}

	// Discover external API calls
	if span.SpanKind == "CLIENT" {
		if httpHost, ok := span.Attributes["http.host"]; ok {
			// Check if it's an external service
			if isExternalHost(httpHost) {
				extEntity := s.getOrCreateEntity(TypeExternalAPI, httpHost)
				extEntity.LastSeen = time.Now()
				svcEntity.AddRelationship(RelCalls, extEntity.ID, extEntity.Name, TypeExternalAPI)
			}
		}

		// Discover downstream service calls
		if peerService, ok := span.Attributes["peer.service"]; ok {
			targetEntity := s.getOrCreateEntity(TypeService, peerService)
			targetEntity.LastSeen = time.Now()
			svcEntity.AddRelationship(RelCalls, targetEntity.ID, targetEntity.Name, TypeService)
		}
	}
}

// updateServiceSignals updates service signals from a span
func (s *Synthesizer) updateServiceSignals(entity *Entity, span TraceSpan) {
	entity.mu.Lock()
	defer entity.mu.Unlock()

	entity.Signals.TotalCount++
	if span.StatusCode >= 400 {
		entity.Signals.ErrorCount++
	}
	entity.Signals.ErrorRate = float64(entity.Signals.ErrorCount) / float64(entity.Signals.TotalCount) * 100

	// Update latency (simple moving average for now)
	latency := float64(span.Duration.Milliseconds())
	if entity.Signals.LatencyP50 == 0 {
		entity.Signals.LatencyP50 = latency
		entity.Signals.LatencyP95 = latency
		entity.Signals.LatencyP99 = latency
	} else {
		// Exponential moving average
		alpha := 0.1
		entity.Signals.LatencyP50 = entity.Signals.LatencyP50*(1-alpha) + latency*alpha
		if latency > entity.Signals.LatencyP95 {
			entity.Signals.LatencyP95 = entity.Signals.LatencyP95*(1-alpha) + latency*alpha
		}
		if latency > entity.Signals.LatencyP99 {
			entity.Signals.LatencyP99 = entity.Signals.LatencyP99*(1-alpha) + latency*alpha
		}
	}

	entity.Signals.WindowEnd = time.Now()
}

// updateHostSignals updates host signals from metrics
func (s *Synthesizer) updateHostSignals(entity *Entity, metric MetricData) {
	entity.mu.Lock()
	defer entity.mu.Unlock()

	switch {
	case strings.Contains(metric.Name, "cpu"):
		entity.Signals.Saturation = metric.Value
		entity.Signals.SaturationUnit = "cpu"
	case strings.Contains(metric.Name, "memory"):
		if metric.Value > entity.Signals.Saturation {
			entity.Signals.Saturation = metric.Value
			entity.Signals.SaturationUnit = "memory"
		}
	}

	entity.Signals.WindowEnd = time.Now()
}

// updateContainerSignals updates container signals from metrics
func (s *Synthesizer) updateContainerSignals(entity *Entity, metric MetricData) {
	entity.mu.Lock()
	defer entity.mu.Unlock()

	switch {
	case strings.Contains(metric.Name, "cpu"):
		entity.Signals.Saturation = metric.Value
		entity.Signals.SaturationUnit = "cpu"
	case strings.Contains(metric.Name, "memory"):
		if metric.Value > entity.Signals.Saturation {
			entity.Signals.Saturation = metric.Value
			entity.Signals.SaturationUnit = "memory"
		}
	}

	entity.Signals.WindowEnd = time.Now()
}

// updateServiceMetricSignals updates service signals from metrics
func (s *Synthesizer) updateServiceMetricSignals(entity *Entity, metric MetricData) {
	entity.mu.Lock()
	defer entity.mu.Unlock()

	switch {
	case strings.Contains(metric.Name, "request") && strings.Contains(metric.Name, "total"):
		entity.Signals.Throughput = metric.Value
	case strings.Contains(metric.Name, "error"):
		entity.Signals.ErrorRate = metric.Value
	case strings.Contains(metric.Name, "latency") || strings.Contains(metric.Name, "duration"):
		if strings.Contains(metric.Name, "p99") {
			entity.Signals.LatencyP99 = metric.Value
		} else if strings.Contains(metric.Name, "p95") {
			entity.Signals.LatencyP95 = metric.Value
		} else if strings.Contains(metric.Name, "p50") {
			entity.Signals.LatencyP50 = metric.Value
		}
	}

	entity.Signals.WindowEnd = time.Now()
}

// runMaintenance runs periodic maintenance tasks
func (s *Synthesizer) runMaintenance() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.computeHealth()
			s.cleanupStale()
		case <-s.stopCh:
			return
		}
	}
}

// computeHealth computes health for all entities
func (s *Synthesizer) computeHealth() {
	s.mu.RLock()
	entities := make([]*Entity, 0, len(s.entities))
	for _, e := range s.entities {
		entities = append(entities, e)
	}
	s.mu.RUnlock()

	for _, entity := range entities {
		entity.mu.Lock()
		entity.Health = entity.ComputeHealth()
		entity.mu.Unlock()
	}
}

// cleanupStale removes stale entities
func (s *Synthesizer) cleanupStale() {
	s.mu.Lock()
	defer s.mu.Unlock()

	threshold := time.Now().Add(-s.staleThreshold)

	for id, entity := range s.entities {
		if entity.LastSeen.Before(threshold) {
			delete(s.entities, id)
			log.Printf("[entity] Removed stale entity: %s", entity.Name)
		}
	}
}

// GetEntity returns an entity by ID
func (s *Synthesizer) GetEntity(id string) *Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entities[id]
}

// GetEntityByName returns an entity by type and name
func (s *Synthesizer) GetEntityByName(entityType Type, name string) *Entity {
	return s.GetEntity(makeID(entityType, name))
}

// ListEntities returns all entities, optionally filtered by type
func (s *Synthesizer) ListEntities(entityType Type) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Entity
	for _, e := range s.entities {
		if entityType == "" || e.Type == entityType {
			result = append(result, e)
		}
	}
	return result
}

// GetRelatedEntities returns entities related to a given entity
func (s *Synthesizer) GetRelatedEntities(entityID string) []*Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entity, exists := s.entities[entityID]
	if !exists {
		return nil
	}

	var related []*Entity
	seenIDs := make(map[string]bool)

	// Get direct relationships
	for _, rel := range entity.Relationships {
		if target, exists := s.entities[rel.TargetID]; exists && !seenIDs[rel.TargetID] {
			related = append(related, target)
			seenIDs[rel.TargetID] = true
		}
	}

	// Get reverse relationships (entities that have relationships to this one)
	for _, e := range s.entities {
		if seenIDs[e.ID] || e.ID == entityID {
			continue
		}
		for _, rel := range e.Relationships {
			if rel.TargetID == entityID {
				related = append(related, e)
				seenIDs[e.ID] = true
				break
			}
		}
	}

	return related
}

// GetStats returns synthesizer statistics
func (s *Synthesizer) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[Type]int)
	healthCounts := make(map[HealthStatus]int)

	for _, e := range s.entities {
		counts[e.Type]++
		healthCounts[e.Health]++
	}

	return map[string]interface{}{
		"total_entities": len(s.entities),
		"by_type":        counts,
		"by_health":      healthCounts,
	}
}

// Helper functions

func makeID(entityType Type, name string) string {
	return fmt.Sprintf("%s:%s", entityType, name)
}

func formatDisplayName(name string) string {
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

func getDefaultThroughputUnit(t Type) string {
	switch t {
	case TypeService:
		return "req/s"
	case TypeDatabase:
		return "queries/s"
	case TypeQueue:
		return "msg/s"
	default:
		return "ops/s"
	}
}

func getDefaultSaturationUnit(t Type) string {
	switch t {
	case TypeHost, TypeContainer:
		return "cpu"
	case TypeDatabase:
		return "connections"
	case TypeQueue:
		return "queue_depth"
	default:
		return "utilization"
	}
}

// isExternalHost checks if a host is external (not internal service)
func isExternalHost(host string) bool {
	// Internal patterns
	internalPatterns := []string{
		`^localhost`,
		`^127\.`,
		`^10\.`,
		`^172\.(1[6-9]|2[0-9]|3[01])\.`,
		`^192\.168\.`,
		`\.local$`,
		`\.internal$`,
		`\.svc\.cluster\.local$`,
	}

	for _, pattern := range internalPatterns {
		if matched, _ := regexp.MatchString(pattern, host); matched {
			return false
		}
	}
	return true
}
