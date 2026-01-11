package federation

import (
	"encoding/json"
	"time"
)

// MessageType identifies the type of gossip message
type MessageType uint8

const (
	MessageTypeNodeState MessageType = iota + 1
	MessageTypeIncident
	MessageTypeIncidentUpdate
	MessageTypeSyncRequest
	MessageTypeSyncResponse
	MessageTypeOnCallChange
	MessageTypeMetricsSummary
)

// Message is the envelope for all gossip messages
type Message struct {
	Type      MessageType     `json:"type"`
	NodeID    string          `json:"node_id"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// NodeState represents a node's current operational state
type NodeState struct {
	NodeID          string    `json:"node_id"`
	Timestamp       time.Time `json:"timestamp"`

	// Resource usage
	CPUPercent      float64   `json:"cpu_percent"`
	MemPercent      float64   `json:"mem_percent"`
	DiskPercent     float64   `json:"disk_percent"`
	LoadAverage     float64   `json:"load_average"`

	// Application metrics
	ActiveIncidents int       `json:"active_incidents"`
	TotalRequests   int64     `json:"total_requests"`
	ErrorRate       float64   `json:"error_rate"`

	// Health
	Uptime          int64     `json:"uptime_seconds"`
	Version         string    `json:"version"`
}

// IncidentEvent represents an incident broadcast to the cluster
type IncidentEvent struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Service     string            `json:"service,omitempty"`
	Source      string            `json:"source"` // "manual", "watch", "slo", "synthetic"
	SourceID    string            `json:"source_id,omitempty"`
	NodeID      string            `json:"node_id"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	AckedAt     *time.Time        `json:"acked_at,omitempty"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	AckedBy     string            `json:"acked_by,omitempty"`
	ResolvedBy  string            `json:"resolved_by,omitempty"`
	Resolution  string            `json:"resolution,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// IncidentUpdate represents a status change to an incident
type IncidentUpdate struct {
	IncidentID string    `json:"incident_id"`
	Status     string    `json:"status"`
	User       string    `json:"user,omitempty"`
	NodeID     string    `json:"node_id"`
	Timestamp  time.Time `json:"timestamp"`
	Resolution string    `json:"resolution,omitempty"`
}

// OnCallChange represents a change in on-call status
type OnCallChange struct {
	ScheduleID   string    `json:"schedule_id"`
	ScheduleName string    `json:"schedule_name"`
	CurrentUser  string    `json:"current_user"`
	NextUser     string    `json:"next_user,omitempty"`
	RotationTime time.Time `json:"rotation_time,omitempty"`
	NodeID       string    `json:"node_id"`
	Timestamp    time.Time `json:"timestamp"`
}

// MetricsSummary is a compact representation of cluster-wide metrics
type MetricsSummary struct {
	NodeID    string    `json:"node_id"`
	Timestamp time.Time `json:"timestamp"`

	// Aggregated metrics (last 5 minutes)
	HTTPRequests   int64   `json:"http_requests"`
	HTTPErrors     int64   `json:"http_errors"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	P95LatencyMs   float64 `json:"p95_latency_ms"`
	P99LatencyMs   float64 `json:"p99_latency_ms"`

	// Connection metrics
	ActiveConnections int `json:"active_connections"`
	NewConnections    int `json:"new_connections"`

	// Container summary
	ContainerCount  int     `json:"container_count"`
	ContainerCPU    float64 `json:"container_cpu_total"`
	ContainerMemory int64   `json:"container_memory_total"`
}

// FullState represents the complete state for sync operations
type FullState struct {
	NodeID     string                    `json:"node_id"`
	Timestamp  time.Time                 `json:"timestamp"`
	NodeStates map[string]*NodeState     `json:"node_states"`
	Incidents  map[string]*IncidentEvent `json:"incidents"`
	OnCall     map[string]*OnCallChange  `json:"on_call"`
}
