// Package entity provides automatic entity synthesis from telemetry data.
// Entities are the logical units in your infrastructure: services, hosts,
// containers, databases, queues, and external APIs.
package entity

import (
	"sync"
	"time"
)

// Type represents the type of entity
type Type string

const (
	TypeService     Type = "SERVICE"
	TypeHost        Type = "HOST"
	TypeContainer   Type = "CONTAINER"
	TypeDatabase    Type = "DATABASE"
	TypeQueue       Type = "QUEUE"
	TypeExternalAPI Type = "EXTERNAL_API"
	TypeLoadBalancer Type = "LOAD_BALANCER"
	TypeCustom      Type = "CUSTOM"
)

// RelationType represents the type of relationship between entities
type RelationType string

const (
	RelCalls     RelationType = "CALLS"      // Service A calls Service B
	RelRunsOn    RelationType = "RUNS_ON"    // Container runs on Host
	RelContains  RelationType = "CONTAINS"   // Service contains endpoints
	RelDependsOn RelationType = "DEPENDS_ON" // Service depends on Database
	RelConnectsTo RelationType = "CONNECTS_TO" // Service connects to Queue
)

// HealthStatus represents entity health
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// Entity represents a discovered entity in the infrastructure
type Entity struct {
	ID          string            `json:"id"`
	Type        Type              `json:"type"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Domain      string            `json:"domain,omitempty"` // Logical grouping

	// Golden signals (auto-calculated)
	Signals GoldenSignals `json:"signals"`

	// Relationships
	Relationships []Relationship `json:"relationships,omitempty"`

	// Metadata
	Tags       map[string]string `json:"tags,omitempty"`
	Team       string            `json:"team,omitempty"`
	Owner      string            `json:"owner,omitempty"`
	Repository string            `json:"repository,omitempty"`
	OnCall     string            `json:"oncall,omitempty"`

	// Current status
	Health       HealthStatus  `json:"health"`
	AlertCount   int           `json:"alert_count"`
	LastActivity time.Time     `json:"last_activity"`

	// Source information
	Source     string    `json:"source"` // traces, metrics, k8s, manual
	Confidence float64   `json:"confidence"` // 0-1, how certain are we this entity exists
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`

	mu sync.RWMutex
}

// GoldenSignals represents the four golden signals for any entity
type GoldenSignals struct {
	// Throughput - requests/operations per second
	Throughput     float64   `json:"throughput"`
	ThroughputUnit string    `json:"throughput_unit"` // "req/s", "ops/s", "msg/s"

	// Error rate - percentage of failed requests
	ErrorRate float64 `json:"error_rate"`
	ErrorCount int64  `json:"error_count"`
	TotalCount int64  `json:"total_count"`

	// Latency - response time percentiles in milliseconds
	LatencyP50 float64 `json:"latency_p50"`
	LatencyP95 float64 `json:"latency_p95"`
	LatencyP99 float64 `json:"latency_p99"`

	// Saturation - resource utilization percentage
	Saturation     float64 `json:"saturation"`
	SaturationUnit string  `json:"saturation_unit"` // "cpu", "memory", "disk", "connections"

	// Time window for these signals
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}

// Relationship represents a connection between two entities
type Relationship struct {
	Type        RelationType      `json:"type"`
	TargetID    string            `json:"target_id"`
	TargetName  string            `json:"target_name"`
	TargetType  Type              `json:"target_type"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	LastSeen    time.Time         `json:"last_seen"`
	CallCount   int64             `json:"call_count,omitempty"`   // For CALLS relationships
	ErrorCount  int64             `json:"error_count,omitempty"`  // For CALLS relationships
	AvgLatency  float64           `json:"avg_latency,omitempty"`  // For CALLS relationships
}

// UpdateSignals updates the golden signals with new data
func (e *Entity) UpdateSignals(throughput, errorRate float64, latencies []float64, saturation float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Signals.Throughput = throughput
	e.Signals.ErrorRate = errorRate
	e.Signals.Saturation = saturation

	if len(latencies) > 0 {
		e.Signals.LatencyP50 = percentile(latencies, 50)
		e.Signals.LatencyP95 = percentile(latencies, 95)
		e.Signals.LatencyP99 = percentile(latencies, 99)
	}

	e.Signals.WindowEnd = time.Now()
	if e.Signals.WindowStart.IsZero() {
		e.Signals.WindowStart = time.Now().Add(-5 * time.Minute)
	}
}

// AddRelationship adds or updates a relationship
func (e *Entity) AddRelationship(relType RelationType, targetID, targetName string, targetType Type) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if relationship exists
	for i, r := range e.Relationships {
		if r.Type == relType && r.TargetID == targetID {
			e.Relationships[i].LastSeen = time.Now()
			e.Relationships[i].CallCount++
			return
		}
	}

	// Add new relationship
	e.Relationships = append(e.Relationships, Relationship{
		Type:       relType,
		TargetID:   targetID,
		TargetName: targetName,
		TargetType: targetType,
		LastSeen:   time.Now(),
		CallCount:  1,
	})
}

// GetSignals returns a copy of the current signals
func (e *Entity) GetSignals() GoldenSignals {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Signals
}

// GetRelationships returns a copy of the relationships
func (e *Entity) GetRelationships() []Relationship {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Relationship, len(e.Relationships))
	copy(result, e.Relationships)
	return result
}

// ComputeHealth computes health based on golden signals
func (e *Entity) ComputeHealth() HealthStatus {
	e.mu.RLock()
	signals := e.Signals
	e.mu.RUnlock()

	// Critical thresholds
	if signals.ErrorRate > 5 || signals.Saturation > 95 {
		return HealthUnhealthy
	}

	// Warning thresholds
	if signals.ErrorRate > 1 || signals.Saturation > 80 || signals.LatencyP99 > 1000 {
		return HealthDegraded
	}

	if signals.TotalCount == 0 {
		return HealthUnknown
	}

	return HealthHealthy
}

// percentile calculates a percentile from a slice of values
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Simple implementation - for production, use a proper percentile algorithm
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sortFloat64s(sorted)

	idx := int(float64(len(sorted)-1) * p / 100)
	return sorted[idx]
}

// sortFloat64s sorts a slice of float64s in place
func sortFloat64s(s []float64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
