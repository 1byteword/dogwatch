package federation

import (
	"sync"
	"time"
)

// SharedState manages cluster-wide state with CRDT-like semantics
// Uses last-writer-wins for most data, with special handling for incidents
type SharedState struct {
	localNodeID string

	// Node states - LWW map keyed by node ID
	nodeStates map[string]*NodeState
	nodeMu     sync.RWMutex

	// Incidents - LWW with tombstones for resolved incidents
	incidents   map[string]*IncidentEvent
	incidentMu  sync.RWMutex

	// On-call state - LWW per schedule
	onCall     map[string]*OnCallChange
	onCallMu   sync.RWMutex

	// Metrics summaries - circular buffer per node
	metrics    map[string][]*MetricsSummary
	metricsMu  sync.RWMutex

	// Callbacks for state changes
	onIncidentChange func(*IncidentEvent)
	onNodeChange     func(*NodeState)

	// Tombstones for deleted incidents (keep for 24h to prevent resurrection)
	tombstones   map[string]time.Time
	tombstoneMu  sync.RWMutex
}

// NewSharedState creates a new shared state manager
func NewSharedState(localNodeID string) *SharedState {
	s := &SharedState{
		localNodeID: localNodeID,
		nodeStates:  make(map[string]*NodeState),
		incidents:   make(map[string]*IncidentEvent),
		onCall:      make(map[string]*OnCallChange),
		metrics:     make(map[string][]*MetricsSummary),
		tombstones:  make(map[string]time.Time),
	}

	// Start tombstone cleanup goroutine
	go s.cleanupTombstones()

	return s
}

// SetOnIncidentChange sets a callback for incident changes
func (s *SharedState) SetOnIncidentChange(fn func(*IncidentEvent)) {
	s.incidentMu.Lock()
	defer s.incidentMu.Unlock()
	s.onIncidentChange = fn
}

// SetOnNodeChange sets a callback for node state changes
func (s *SharedState) SetOnNodeChange(fn func(*NodeState)) {
	s.nodeMu.Lock()
	defer s.nodeMu.Unlock()
	s.onNodeChange = fn
}

// UpdateLocalNode updates the local node's state
func (s *SharedState) UpdateLocalNode(cpu, mem float64, incidents int, requests int64, errorRate float64) {
	state := &NodeState{
		NodeID:          s.localNodeID,
		Timestamp:       time.Now(),
		CPUPercent:      cpu,
		MemPercent:      mem,
		ActiveIncidents: incidents,
		TotalRequests:   requests,
		ErrorRate:       errorRate,
	}

	s.nodeMu.Lock()
	s.nodeStates[s.localNodeID] = state
	s.nodeMu.Unlock()
}

// GetLocalState returns the local node's state
func (s *SharedState) GetLocalState() *NodeState {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	return s.nodeStates[s.localNodeID]
}

// GetNodeState returns a specific node's state
func (s *SharedState) GetNodeState(nodeID string) *NodeState {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()
	return s.nodeStates[nodeID]
}

// GetAllNodeStates returns all known node states
func (s *SharedState) GetAllNodeStates() map[string]*NodeState {
	s.nodeMu.RLock()
	defer s.nodeMu.RUnlock()

	result := make(map[string]*NodeState, len(s.nodeStates))
	for k, v := range s.nodeStates {
		result[k] = v
	}
	return result
}

// MergeNodeState merges a remote node's state using LWW semantics
func (s *SharedState) MergeNodeState(nodeID string, state *NodeState) {
	s.nodeMu.Lock()
	existing := s.nodeStates[nodeID]
	if existing == nil || state.Timestamp.After(existing.Timestamp) {
		s.nodeStates[nodeID] = state
		callback := s.onNodeChange
		s.nodeMu.Unlock()

		if callback != nil {
			callback(state)
		}
		return
	}
	s.nodeMu.Unlock()
}

// RemoveNode removes a node from the state (when it leaves the cluster)
func (s *SharedState) RemoveNode(nodeID string) {
	s.nodeMu.Lock()
	delete(s.nodeStates, nodeID)
	s.nodeMu.Unlock()

	s.metricsMu.Lock()
	delete(s.metrics, nodeID)
	s.metricsMu.Unlock()
}

// AddIncident adds or updates an incident with CRDT semantics
func (s *SharedState) AddIncident(inc *IncidentEvent) {
	// Check tombstones first
	s.tombstoneMu.RLock()
	if _, tombstoned := s.tombstones[inc.ID]; tombstoned {
		s.tombstoneMu.RUnlock()
		return // Don't resurrect tombstoned incidents
	}
	s.tombstoneMu.RUnlock()

	s.incidentMu.Lock()
	existing := s.incidents[inc.ID]

	// Use UpdatedAt for LWW comparison
	if existing == nil || inc.UpdatedAt.After(existing.UpdatedAt) {
		s.incidents[inc.ID] = inc
		callback := s.onIncidentChange
		s.incidentMu.Unlock()

		if callback != nil {
			callback(inc)
		}
		return
	}
	s.incidentMu.Unlock()
}

// UpdateIncident updates an incident's status
func (s *SharedState) UpdateIncident(update *IncidentUpdate) {
	s.incidentMu.Lock()
	defer s.incidentMu.Unlock()

	inc := s.incidents[update.IncidentID]
	if inc == nil {
		return
	}

	// Only apply if this update is newer
	if update.Timestamp.After(inc.UpdatedAt) {
		inc.Status = update.Status
		inc.UpdatedAt = update.Timestamp

		switch update.Status {
		case "acknowledged":
			inc.AckedBy = update.User
			t := update.Timestamp
			inc.AckedAt = &t
		case "resolved":
			inc.ResolvedBy = update.User
			inc.Resolution = update.Resolution
			t := update.Timestamp
			inc.ResolvedAt = &t

			// Add tombstone for resolved incidents
			s.tombstoneMu.Lock()
			s.tombstones[update.IncidentID] = update.Timestamp
			s.tombstoneMu.Unlock()
		}

		if s.onIncidentChange != nil {
			s.onIncidentChange(inc)
		}
	}
}

// GetIncident returns a specific incident
func (s *SharedState) GetIncident(id string) *IncidentEvent {
	s.incidentMu.RLock()
	defer s.incidentMu.RUnlock()
	return s.incidents[id]
}

// GetAllIncidents returns all active incidents cluster-wide
func (s *SharedState) GetAllIncidents() []*IncidentEvent {
	s.incidentMu.RLock()
	defer s.incidentMu.RUnlock()

	result := make([]*IncidentEvent, 0, len(s.incidents))
	for _, inc := range s.incidents {
		// Only include non-resolved or recently resolved
		if inc.Status != "resolved" || (inc.ResolvedAt != nil && time.Since(*inc.ResolvedAt) < 24*time.Hour) {
			result = append(result, inc)
		}
	}
	return result
}

// GetActiveIncidentCount returns the count of active incidents cluster-wide
func (s *SharedState) GetActiveIncidentCount() int {
	s.incidentMu.RLock()
	defer s.incidentMu.RUnlock()

	count := 0
	for _, inc := range s.incidents {
		if inc.Status == "triggered" || inc.Status == "acknowledged" {
			count++
		}
	}
	return count
}

// UpdateOnCall updates on-call state for a schedule
func (s *SharedState) UpdateOnCall(change *OnCallChange) {
	s.onCallMu.Lock()
	defer s.onCallMu.Unlock()

	existing := s.onCall[change.ScheduleID]
	if existing == nil || change.Timestamp.After(existing.Timestamp) {
		s.onCall[change.ScheduleID] = change
	}
}

// GetOnCall returns the current on-call state for all schedules
func (s *SharedState) GetOnCall() map[string]*OnCallChange {
	s.onCallMu.RLock()
	defer s.onCallMu.RUnlock()

	result := make(map[string]*OnCallChange, len(s.onCall))
	for k, v := range s.onCall {
		result[k] = v
	}
	return result
}

// AddMetricsSummary adds a metrics summary for a node
func (s *SharedState) AddMetricsSummary(summary *MetricsSummary) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()

	nodeMetrics := s.metrics[summary.NodeID]

	// Keep last 12 summaries (1 hour at 5-minute intervals)
	if len(nodeMetrics) >= 12 {
		nodeMetrics = nodeMetrics[1:]
	}

	s.metrics[summary.NodeID] = append(nodeMetrics, summary)
}

// GetClusterMetrics returns aggregated metrics across all nodes
func (s *SharedState) GetClusterMetrics() *MetricsSummary {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()

	aggregate := &MetricsSummary{
		NodeID:    "cluster",
		Timestamp: time.Now(),
	}

	latencyCount := 0
	for _, nodeMetrics := range s.metrics {
		if len(nodeMetrics) == 0 {
			continue
		}

		// Use most recent summary for each node
		latest := nodeMetrics[len(nodeMetrics)-1]
		aggregate.HTTPRequests += latest.HTTPRequests
		aggregate.HTTPErrors += latest.HTTPErrors
		aggregate.ActiveConnections += latest.ActiveConnections
		aggregate.NewConnections += latest.NewConnections
		aggregate.ContainerCount += latest.ContainerCount
		aggregate.ContainerCPU += latest.ContainerCPU
		aggregate.ContainerMemory += latest.ContainerMemory

		if latest.AvgLatencyMs > 0 {
			aggregate.AvgLatencyMs += latest.AvgLatencyMs
			latencyCount++
		}
		if latest.P95LatencyMs > aggregate.P95LatencyMs {
			aggregate.P95LatencyMs = latest.P95LatencyMs
		}
		if latest.P99LatencyMs > aggregate.P99LatencyMs {
			aggregate.P99LatencyMs = latest.P99LatencyMs
		}
	}

	if latencyCount > 0 {
		aggregate.AvgLatencyMs /= float64(latencyCount)
	}

	return aggregate
}

// GetFullState returns the complete state for sync operations
func (s *SharedState) GetFullState() *FullState {
	s.nodeMu.RLock()
	nodeStates := make(map[string]*NodeState, len(s.nodeStates))
	for k, v := range s.nodeStates {
		nodeStates[k] = v
	}
	s.nodeMu.RUnlock()

	s.incidentMu.RLock()
	incidents := make(map[string]*IncidentEvent, len(s.incidents))
	for k, v := range s.incidents {
		incidents[k] = v
	}
	s.incidentMu.RUnlock()

	s.onCallMu.RLock()
	onCall := make(map[string]*OnCallChange, len(s.onCall))
	for k, v := range s.onCall {
		onCall[k] = v
	}
	s.onCallMu.RUnlock()

	return &FullState{
		NodeID:     s.localNodeID,
		Timestamp:  time.Now(),
		NodeStates: nodeStates,
		Incidents:  incidents,
		OnCall:     onCall,
	}
}

// MergeFullState merges a complete state from another node
func (s *SharedState) MergeFullState(state *FullState) {
	// Merge node states
	for nodeID, nodeState := range state.NodeStates {
		s.MergeNodeState(nodeID, nodeState)
	}

	// Merge incidents
	for _, inc := range state.Incidents {
		s.AddIncident(inc)
	}

	// Merge on-call
	for _, oc := range state.OnCall {
		s.UpdateOnCall(oc)
	}
}

// cleanupTombstones periodically removes old tombstones
func (s *SharedState) cleanupTombstones() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.tombstoneMu.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)
		for id, ts := range s.tombstones {
			if ts.Before(cutoff) {
				delete(s.tombstones, id)
			}
		}
		s.tombstoneMu.Unlock()

		// Also cleanup old resolved incidents
		s.incidentMu.Lock()
		for id, inc := range s.incidents {
			if inc.Status == "resolved" && inc.ResolvedAt != nil && inc.ResolvedAt.Before(cutoff) {
				delete(s.incidents, id)
			}
		}
		s.incidentMu.Unlock()
	}
}
