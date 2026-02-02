package security

import (
	"sync"
	"time"
)

// BaselineState represents the learning state
type BaselineState string

const (
	BaselineStateLearning BaselineState = "learning"
	BaselineStateActive   BaselineState = "active"
	BaselineStatePaused   BaselineState = "paused"
)

// ContainerBaseline tracks the normal behavior of a container
type ContainerBaseline struct {
	ID            string        `json:"id"`
	ContainerID   string        `json:"container_id"`
	ContainerName string        `json:"container_name"`
	ImageName     string        `json:"image_name"`
	Namespace     string        `json:"namespace"`
	PodName       string        `json:"pod_name"`
	State         BaselineState `json:"state"`

	// Learning metadata
	LearningStarted  time.Time `json:"learning_started"`
	LearningEnded    time.Time `json:"learning_ended,omitempty"`
	EventsObserved   int64     `json:"events_observed"`
	LastEventTime    time.Time `json:"last_event_time"`

	// Process baseline
	ProcessBaseline *ProcessBaseline `json:"process_baseline,omitempty"`

	// Network baseline
	NetworkBaseline *NetworkBaseline `json:"network_baseline,omitempty"`

	// File access baseline
	FileBaseline *FileBaseline `json:"file_baseline,omitempty"`

	mu sync.RWMutex `json:"-"`
}

// ProcessBaseline tracks normal process behavior
type ProcessBaseline struct {
	// Allowed processes (by name)
	AllowedProcesses map[string]*ProcessInfo `json:"allowed_processes"`

	// Parent-child relationships
	AllowedParentChild map[string][]string `json:"allowed_parent_child"` // parent -> allowed children

	// Maximum observed process count
	MaxConcurrentProcesses int `json:"max_concurrent_processes"`

	// Process creation rate (per minute)
	AvgProcessCreationRate float64 `json:"avg_process_creation_rate"`
	MaxProcessCreationRate float64 `json:"max_process_creation_rate"`
}

// ProcessInfo holds information about a known process
type ProcessInfo struct {
	Name          string    `json:"name"`
	ExePath       string    `json:"exe_path"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	SeenCount     int64     `json:"seen_count"`
	AllowedArgs   []string  `json:"allowed_args,omitempty"`   // Common argument patterns
	RunsAsRoot    bool      `json:"runs_as_root"`
	Whitelisted   bool      `json:"whitelisted"`
}

// NetworkBaseline tracks normal network behavior
type NetworkBaseline struct {
	// Allowed outbound destinations
	AllowedDestinations map[string]*DestinationInfo `json:"allowed_destinations"` // ip:port -> info

	// Allowed listening ports
	AllowedListenPorts map[uint16]*PortInfo `json:"allowed_listen_ports"`

	// DNS query patterns
	AllowedDNSDomains map[string]int64 `json:"allowed_dns_domains"` // domain -> count

	// Traffic statistics
	AvgOutboundConnsPerMinute float64 `json:"avg_outbound_conns_per_minute"`
	MaxOutboundConnsPerMinute float64 `json:"max_outbound_conns_per_minute"`
	AvgBytesPerMinute         float64 `json:"avg_bytes_per_minute"`
}

// DestinationInfo holds information about a known destination
type DestinationInfo struct {
	IP        string    `json:"ip"`
	Port      uint16    `json:"port"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	ConnCount int64     `json:"conn_count"`
	Hostname  string    `json:"hostname,omitempty"` // If resolved
}

// PortInfo holds information about a listening port
type PortInfo struct {
	Port      uint16    `json:"port"`
	Process   string    `json:"process"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// FileBaseline tracks normal file access patterns
type FileBaseline struct {
	// Allowed file access patterns
	AllowedPaths map[string]*FileAccessInfo `json:"allowed_paths"`

	// Sensitive paths that should never be accessed
	SensitivePaths []string `json:"sensitive_paths"`

	// Write locations
	AllowedWritePaths map[string]int64 `json:"allowed_write_paths"`
}

// FileAccessInfo holds information about file access patterns
type FileAccessInfo struct {
	Path       string    `json:"path"`
	Operation  string    `json:"operation"` // read, write, exec
	Process    string    `json:"process"`   // Which process accesses
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	AccessCount int64    `json:"access_count"`
}

// BaselineManager manages container baselines
type BaselineManager struct {
	mu             sync.RWMutex
	baselines      map[string]*ContainerBaseline // containerID -> baseline
	hostBaseline   *ContainerBaseline            // For host-level events
	learningPeriod time.Duration

	// Callbacks for anomaly detection
	onAnomaly func(anomaly *BaselineAnomaly)
}

// BaselineAnomaly represents a deviation from baseline
type BaselineAnomaly struct {
	Type          string            `json:"type"` // process, network, file
	Severity      Severity          `json:"severity"`
	ContainerID   string            `json:"container_id,omitempty"`
	Description   string            `json:"description"`
	Event         *SecurityEvent    `json:"event,omitempty"`
	BaselineValue interface{}       `json:"baseline_value,omitempty"`
	ObservedValue interface{}       `json:"observed_value,omitempty"`
	Confidence    float64           `json:"confidence"` // 0-1, how confident in anomaly
	Timestamp     time.Time         `json:"timestamp"`
}

// NewBaselineManager creates a new baseline manager
func NewBaselineManager(learningPeriod time.Duration) *BaselineManager {
	if learningPeriod == 0 {
		learningPeriod = 24 * time.Hour // Default 24-hour learning period
	}

	return &BaselineManager{
		baselines:      make(map[string]*ContainerBaseline),
		learningPeriod: learningPeriod,
		hostBaseline: &ContainerBaseline{
			ID:              "host",
			ContainerID:     "host",
			ContainerName:   "host",
			State:           BaselineStateLearning,
			LearningStarted: time.Now(),
			ProcessBaseline: &ProcessBaseline{
				AllowedProcesses:   make(map[string]*ProcessInfo),
				AllowedParentChild: make(map[string][]string),
			},
			NetworkBaseline: &NetworkBaseline{
				AllowedDestinations: make(map[string]*DestinationInfo),
				AllowedListenPorts:  make(map[uint16]*PortInfo),
				AllowedDNSDomains:   make(map[string]int64),
			},
			FileBaseline: &FileBaseline{
				AllowedPaths:      make(map[string]*FileAccessInfo),
				AllowedWritePaths: make(map[string]int64),
			},
		},
	}
}

// SetAnomalyCallback sets the callback for anomaly detection
func (m *BaselineManager) SetAnomalyCallback(cb func(*BaselineAnomaly)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAnomaly = cb
}

// GetOrCreateBaseline gets or creates a baseline for a container
func (m *BaselineManager) GetOrCreateBaseline(event *SecurityEvent) *ContainerBaseline {
	m.mu.Lock()
	defer m.mu.Unlock()

	// For non-container events, use host baseline
	if event.ContainerID == "" {
		return m.hostBaseline
	}

	baseline, ok := m.baselines[event.ContainerID]
	if !ok {
		baseline = &ContainerBaseline{
			ID:              event.ContainerID,
			ContainerID:     event.ContainerID,
			ContainerName:   event.ContainerName,
			ImageName:       event.ImageName,
			Namespace:       event.Namespace,
			PodName:         event.PodName,
			State:           BaselineStateLearning,
			LearningStarted: time.Now(),
			ProcessBaseline: &ProcessBaseline{
				AllowedProcesses:   make(map[string]*ProcessInfo),
				AllowedParentChild: make(map[string][]string),
			},
			NetworkBaseline: &NetworkBaseline{
				AllowedDestinations: make(map[string]*DestinationInfo),
				AllowedListenPorts:  make(map[uint16]*PortInfo),
				AllowedDNSDomains:   make(map[string]int64),
			},
			FileBaseline: &FileBaseline{
				AllowedPaths:      make(map[string]*FileAccessInfo),
				AllowedWritePaths: make(map[string]int64),
			},
		}
		m.baselines[event.ContainerID] = baseline
	}

	return baseline
}

// ProcessEvent processes a security event and updates/checks baseline
func (m *BaselineManager) ProcessEvent(event *SecurityEvent) *BaselineAnomaly {
	baseline := m.GetOrCreateBaseline(event)

	baseline.mu.Lock()
	defer baseline.mu.Unlock()

	baseline.EventsObserved++
	baseline.LastEventTime = event.Timestamp

	// Check if learning period is over
	if baseline.State == BaselineStateLearning {
		if time.Since(baseline.LearningStarted) > m.learningPeriod {
			baseline.State = BaselineStateActive
			baseline.LearningEnded = time.Now()
		}
	}

	// Process based on event type
	var anomaly *BaselineAnomaly
	switch event.Type {
	case EventTypeProcess:
		anomaly = m.processProcessEvent(baseline, event)
	case EventTypeNetwork:
		anomaly = m.processNetworkEvent(baseline, event)
	case EventTypeFile:
		anomaly = m.processFileEvent(baseline, event)
	}

	if anomaly != nil && m.onAnomaly != nil {
		m.onAnomaly(anomaly)
	}

	return anomaly
}

func (m *BaselineManager) processProcessEvent(baseline *ContainerBaseline, event *SecurityEvent) *BaselineAnomaly {
	if baseline.ProcessBaseline == nil {
		return nil
	}

	pb := baseline.ProcessBaseline
	procName := event.Comm

	if baseline.State == BaselineStateLearning {
		// Learning mode: record process
		if _, ok := pb.AllowedProcesses[procName]; !ok {
			pb.AllowedProcesses[procName] = &ProcessInfo{
				Name:      procName,
				ExePath:   event.ExePath,
				FirstSeen: event.Timestamp,
				SeenCount: 1,
				RunsAsRoot: event.UID == 0,
			}
		} else {
			pb.AllowedProcesses[procName].LastSeen = event.Timestamp
			pb.AllowedProcesses[procName].SeenCount++
		}

		// Record parent-child relationship
		if event.ParentComm != "" {
			children := pb.AllowedParentChild[event.ParentComm]
			found := false
			for _, c := range children {
				if c == procName {
					found = true
					break
				}
			}
			if !found {
				pb.AllowedParentChild[event.ParentComm] = append(children, procName)
			}
		}

		return nil
	}

	// Active mode: check for anomalies
	info, known := pb.AllowedProcesses[procName]
	if !known {
		return &BaselineAnomaly{
			Type:          "process",
			Severity:      SeverityMedium,
			ContainerID:   event.ContainerID,
			Description:   "Unknown process executed: " + procName,
			Event:         event,
			BaselineValue: "not in baseline",
			ObservedValue: procName,
			Confidence:    0.8,
			Timestamp:     event.Timestamp,
		}
	}

	// Check parent-child relationship
	if event.ParentComm != "" {
		allowedChildren := pb.AllowedParentChild[event.ParentComm]
		childAllowed := false
		for _, c := range allowedChildren {
			if c == procName {
				childAllowed = true
				break
			}
		}
		if !childAllowed && len(allowedChildren) > 0 {
			return &BaselineAnomaly{
				Type:          "process",
				Severity:      SeverityMedium,
				ContainerID:   event.ContainerID,
				Description:   "Unexpected parent-child relationship: " + event.ParentComm + " -> " + procName,
				Event:         event,
				BaselineValue: allowedChildren,
				ObservedValue: procName,
				Confidence:    0.7,
				Timestamp:     event.Timestamp,
			}
		}
	}

	// Update last seen even when known
	info.LastSeen = event.Timestamp
	info.SeenCount++

	return nil
}

func (m *BaselineManager) processNetworkEvent(baseline *ContainerBaseline, event *SecurityEvent) *BaselineAnomaly {
	if baseline.NetworkBaseline == nil {
		return nil
	}

	nb := baseline.NetworkBaseline

	// Create destination key
	destKey := event.DstIP + ":" + string(rune(event.DstPort))

	if baseline.State == BaselineStateLearning {
		// Learning mode: record destination
		if _, ok := nb.AllowedDestinations[destKey]; !ok {
			nb.AllowedDestinations[destKey] = &DestinationInfo{
				IP:        event.DstIP,
				Port:      event.DstPort,
				FirstSeen: event.Timestamp,
				ConnCount: 1,
			}
		} else {
			nb.AllowedDestinations[destKey].LastSeen = event.Timestamp
			nb.AllowedDestinations[destKey].ConnCount++
		}

		return nil
	}

	// Active mode: check for anomalies
	_, known := nb.AllowedDestinations[destKey]
	if !known && event.DstIP != "" && event.DstPort != 0 {
		// Check if the IP is known but different port
		anyPortKnown := false
		for key := range nb.AllowedDestinations {
			if len(key) > len(event.DstIP) && key[:len(event.DstIP)] == event.DstIP {
				anyPortKnown = true
				break
			}
		}

		severity := SeverityMedium
		confidence := 0.7
		if !anyPortKnown {
			severity = SeverityHigh
			confidence = 0.85
		}

		return &BaselineAnomaly{
			Type:          "network",
			Severity:      severity,
			ContainerID:   event.ContainerID,
			Description:   "Connection to unknown destination: " + destKey,
			Event:         event,
			BaselineValue: "not in baseline",
			ObservedValue: destKey,
			Confidence:    confidence,
			Timestamp:     event.Timestamp,
		}
	}

	if known {
		nb.AllowedDestinations[destKey].LastSeen = event.Timestamp
		nb.AllowedDestinations[destKey].ConnCount++
	}

	return nil
}

func (m *BaselineManager) processFileEvent(baseline *ContainerBaseline, event *SecurityEvent) *BaselineAnomaly {
	if baseline.FileBaseline == nil {
		return nil
	}

	fb := baseline.FileBaseline
	path := event.FilePath

	// Create path key with operation
	pathKey := path + ":" + event.Operation

	if baseline.State == BaselineStateLearning {
		// Learning mode: record file access
		if _, ok := fb.AllowedPaths[pathKey]; !ok {
			fb.AllowedPaths[pathKey] = &FileAccessInfo{
				Path:        path,
				Operation:   event.Operation,
				Process:     event.Comm,
				FirstSeen:   event.Timestamp,
				AccessCount: 1,
			}
		} else {
			fb.AllowedPaths[pathKey].LastSeen = event.Timestamp
			fb.AllowedPaths[pathKey].AccessCount++
		}

		if event.Operation == "write" {
			fb.AllowedWritePaths[path]++
		}

		return nil
	}

	// Active mode: check for anomalies
	_, known := fb.AllowedPaths[pathKey]
	if !known && path != "" {
		// Higher severity for sensitive paths
		severity := SeverityLow
		confidence := 0.6
		if isSensitivePath(path) {
			severity = SeverityHigh
			confidence = 0.9
		} else if event.Operation == "write" || event.Operation == "exec" {
			severity = SeverityMedium
			confidence = 0.75
		}

		return &BaselineAnomaly{
			Type:          "file",
			Severity:      severity,
			ContainerID:   event.ContainerID,
			Description:   "Unusual file access: " + event.Operation + " on " + path,
			Event:         event,
			BaselineValue: "not in baseline",
			ObservedValue: pathKey,
			Confidence:    confidence,
			Timestamp:     event.Timestamp,
		}
	}

	if known {
		fb.AllowedPaths[pathKey].LastSeen = event.Timestamp
		fb.AllowedPaths[pathKey].AccessCount++
	}

	return nil
}

// isSensitivePath checks if a path is considered sensitive
func isSensitivePath(path string) bool {
	sensitivePaths := []string{
		"/etc/shadow",
		"/etc/passwd",
		"/etc/sudoers",
		"/root/.ssh",
		"/.ssh/",
		"/var/run/docker.sock",
		"/var/run/secrets",
		"/proc/sys",
		"/sys/kernel",
	}

	for _, sp := range sensitivePaths {
		if len(path) >= len(sp) && path[:len(sp)] == sp {
			return true
		}
	}
	return false
}

// GetBaselineStats returns statistics about baselines
func (m *BaselineManager) GetBaselineStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	learningCount := 0
	activeCount := 0
	totalProcesses := 0
	totalDestinations := 0

	for _, b := range m.baselines {
		if b.State == BaselineStateLearning {
			learningCount++
		} else if b.State == BaselineStateActive {
			activeCount++
		}
		if b.ProcessBaseline != nil {
			totalProcesses += len(b.ProcessBaseline.AllowedProcesses)
		}
		if b.NetworkBaseline != nil {
			totalDestinations += len(b.NetworkBaseline.AllowedDestinations)
		}
	}

	// Include host baseline
	if m.hostBaseline.ProcessBaseline != nil {
		totalProcesses += len(m.hostBaseline.ProcessBaseline.AllowedProcesses)
	}
	if m.hostBaseline.NetworkBaseline != nil {
		totalDestinations += len(m.hostBaseline.NetworkBaseline.AllowedDestinations)
	}

	return map[string]interface{}{
		"total_containers":      len(m.baselines),
		"learning_containers":   learningCount,
		"active_containers":     activeCount,
		"total_known_processes": totalProcesses,
		"total_known_destinations": totalDestinations,
		"learning_period_hours": m.learningPeriod.Hours(),
		"host_baseline_state":   string(m.hostBaseline.State),
	}
}

// GetContainerBaseline returns the baseline for a specific container
func (m *BaselineManager) GetContainerBaseline(containerID string) *ContainerBaseline {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if containerID == "" || containerID == "host" {
		return m.hostBaseline
	}

	return m.baselines[containerID]
}

// MarkFalsePositive records that an alert was a false positive
// This can be used to adjust the baseline
func (m *BaselineManager) MarkFalsePositive(event *SecurityEvent) {
	baseline := m.GetOrCreateBaseline(event)

	baseline.mu.Lock()
	defer baseline.mu.Unlock()

	// Add the event to the baseline
	switch event.Type {
	case EventTypeProcess:
		if baseline.ProcessBaseline != nil {
			baseline.ProcessBaseline.AllowedProcesses[event.Comm] = &ProcessInfo{
				Name:        event.Comm,
				ExePath:     event.ExePath,
				FirstSeen:   event.Timestamp,
				LastSeen:    event.Timestamp,
				SeenCount:   1,
				Whitelisted: true,
			}
		}
	case EventTypeNetwork:
		if baseline.NetworkBaseline != nil {
			destKey := event.DstIP + ":" + string(rune(event.DstPort))
			baseline.NetworkBaseline.AllowedDestinations[destKey] = &DestinationInfo{
				IP:        event.DstIP,
				Port:      event.DstPort,
				FirstSeen: event.Timestamp,
				LastSeen:  event.Timestamp,
				ConnCount: 1,
			}
		}
	case EventTypeFile:
		if baseline.FileBaseline != nil {
			pathKey := event.FilePath + ":" + event.Operation
			baseline.FileBaseline.AllowedPaths[pathKey] = &FileAccessInfo{
				Path:        event.FilePath,
				Operation:   event.Operation,
				Process:     event.Comm,
				FirstSeen:   event.Timestamp,
				LastSeen:    event.Timestamp,
				AccessCount: 1,
			}
		}
	}
}

// ResetBaseline resets the baseline for a container to learning mode
func (m *BaselineManager) ResetBaseline(containerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if containerID == "" || containerID == "host" {
		m.hostBaseline.State = BaselineStateLearning
		m.hostBaseline.LearningStarted = time.Now()
		m.hostBaseline.ProcessBaseline = &ProcessBaseline{
			AllowedProcesses:   make(map[string]*ProcessInfo),
			AllowedParentChild: make(map[string][]string),
		}
		m.hostBaseline.NetworkBaseline = &NetworkBaseline{
			AllowedDestinations: make(map[string]*DestinationInfo),
			AllowedListenPorts:  make(map[uint16]*PortInfo),
			AllowedDNSDomains:   make(map[string]int64),
		}
		m.hostBaseline.FileBaseline = &FileBaseline{
			AllowedPaths:      make(map[string]*FileAccessInfo),
			AllowedWritePaths: make(map[string]int64),
		}
		return
	}

	delete(m.baselines, containerID)
}
