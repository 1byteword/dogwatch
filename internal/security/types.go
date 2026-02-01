package security

import (
	"time"
)

// Severity levels for security events and alerts
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// EventType categorizes the source of security events
type EventType string

const (
	EventTypeProcess   EventType = "process"
	EventTypeNetwork   EventType = "network"
	EventTypeFile      EventType = "file"
	EventTypeContainer EventType = "container"
)

// AlertState represents the current state of a security alert
type AlertState string

const (
	AlertStateOpen         AlertState = "open"
	AlertStateAcknowledged AlertState = "acknowledged"
	AlertStateResolved     AlertState = "resolved"
	AlertStateFalsePositive AlertState = "false_positive"
)

// SecurityEvent represents a raw security-relevant event from eBPF probes
type SecurityEvent struct {
	ID        string            `json:"id"`
	Timestamp time.Time         `json:"timestamp"`
	Type      EventType         `json:"type"`
	HostID    string            `json:"host_id"`
	Hostname  string            `json:"hostname"`

	// Process context
	PID         uint32 `json:"pid,omitempty"`
	PPID        uint32 `json:"ppid,omitempty"`
	UID         uint32 `json:"uid,omitempty"`
	GID         uint32 `json:"gid,omitempty"`
	Comm        string `json:"comm,omitempty"`
	Cmdline     string `json:"cmdline,omitempty"`
	ExePath     string `json:"exe_path,omitempty"`
	ParentComm  string `json:"parent_comm,omitempty"`

	// Network context
	SrcIP    string `json:"src_ip,omitempty"`
	DstIP    string `json:"dst_ip,omitempty"`
	SrcPort  uint16 `json:"src_port,omitempty"`
	DstPort  uint16 `json:"dst_port,omitempty"`
	Protocol string `json:"protocol,omitempty"`

	// File context
	FilePath  string `json:"file_path,omitempty"`
	FileMode  uint32 `json:"file_mode,omitempty"`
	Operation string `json:"operation,omitempty"` // open, read, write, exec

	// Container context
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	ImageName     string `json:"image_name,omitempty"`
	Privileged    bool   `json:"privileged,omitempty"`

	// Additional attributes
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SecurityAlert represents a detected security threat
type SecurityAlert struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"rule_id"`
	RuleName    string            `json:"rule_name"`
	Severity    Severity          `json:"severity"`
	State       AlertState        `json:"state"`
	Title       string            `json:"title"`
	Description string            `json:"description"`

	// MITRE ATT&CK mapping
	MitreTactic    string   `json:"mitre_tactic,omitempty"`
	MitreTechnique string   `json:"mitre_technique,omitempty"`
	MitreTechniqueID string `json:"mitre_technique_id,omitempty"`

	// Context from triggering event
	Event     *SecurityEvent    `json:"event,omitempty"`
	EventID   string            `json:"event_id"`
	HostID    string            `json:"host_id"`
	Hostname  string            `json:"hostname"`

	// Container context
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	Namespace     string `json:"namespace,omitempty"`

	// Timestamps
	DetectedAt     time.Time  `json:"detected_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	ResolvedBy     string     `json:"resolved_by,omitempty"`

	// Additional context
	Labels     map[string]string `json:"labels,omitempty"`
	Indicators []string          `json:"indicators,omitempty"` // IOCs found
	Notes      string            `json:"notes,omitempty"`
}

// ThreatIndicator represents an indicator of compromise (IOC)
type ThreatIndicator struct {
	Type  string `json:"type"`  // ip, domain, hash, path, process
	Value string `json:"value"`
}

// ProcessExecEvent is a specialized event for process execution
type ProcessExecEvent struct {
	SecurityEvent
	Args     []string `json:"args,omitempty"`
	Env      []string `json:"env,omitempty"`
	Cwd      string   `json:"cwd,omitempty"`
	TTY      bool     `json:"tty,omitempty"`
}

// NetworkConnectEvent is a specialized event for network connections
type NetworkConnectEvent struct {
	SecurityEvent
	Direction  string `json:"direction"` // inbound, outbound
	BytesSent  uint64 `json:"bytes_sent,omitempty"`
	BytesRecv  uint64 `json:"bytes_recv,omitempty"`
}

// FileAccessEvent is a specialized event for file operations
type FileAccessEvent struct {
	SecurityEvent
	Flags     uint32 `json:"flags,omitempty"`
	BytesRead uint64 `json:"bytes_read,omitempty"`
	BytesWritten uint64 `json:"bytes_written,omitempty"`
}

// AlertSummary provides aggregated alert statistics
type AlertSummary struct {
	TotalAlerts       int            `json:"total_alerts"`
	OpenAlerts        int            `json:"open_alerts"`
	CriticalCount     int            `json:"critical_count"`
	HighCount         int            `json:"high_count"`
	MediumCount       int            `json:"medium_count"`
	LowCount          int            `json:"low_count"`
	AlertsByRule      map[string]int `json:"alerts_by_rule"`
	AlertsByHost      map[string]int `json:"alerts_by_host"`
	AlertsByContainer map[string]int `json:"alerts_by_container"`
}

// EventCallback is called when a security event is detected
type EventCallback func(event *SecurityEvent)

// AlertCallback is called when a security alert is generated
type AlertCallback func(alert *SecurityAlert)
