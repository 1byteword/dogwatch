package security

import (
	"testing"
	"time"
)

func TestBaselineManager_LearningMode(t *testing.T) {
	// Create manager with short learning period for testing
	manager := NewBaselineManager(100 * time.Millisecond)

	event := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		ContainerName: "test-container",
		Timestamp:   time.Now(),
		Comm:        "nginx",
		ExePath:     "/usr/sbin/nginx",
		ParentComm:  "systemd",
	}

	// First event should be learned, not flagged as anomaly
	anomaly := manager.ProcessEvent(event)
	if anomaly != nil {
		t.Error("Expected no anomaly during learning period")
	}

	// Check baseline was created
	baseline := manager.GetContainerBaseline("container-1")
	if baseline == nil {
		t.Fatal("Expected baseline to be created")
	}
	if baseline.State != BaselineStateLearning {
		t.Errorf("Expected state=learning, got %s", baseline.State)
	}

	// Check process was recorded
	if _, ok := baseline.ProcessBaseline.AllowedProcesses["nginx"]; !ok {
		t.Error("Expected nginx to be in allowed processes")
	}
}

func TestBaselineManager_ActiveMode(t *testing.T) {
	// Create manager with very short learning period
	manager := NewBaselineManager(1 * time.Millisecond)

	// Create baseline and let it learn
	learnEvent := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
		ExePath:     "/usr/sbin/nginx",
	}
	manager.ProcessEvent(learnEvent)

	// Wait for learning period to expire
	time.Sleep(10 * time.Millisecond)

	// Send known process - should not be anomaly
	knownEvent := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	}
	anomaly := manager.ProcessEvent(knownEvent)
	if anomaly != nil {
		t.Error("Expected no anomaly for known process")
	}

	// Send unknown process - should be anomaly
	unknownEvent := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "cryptominer",
		ExePath:     "/tmp/cryptominer",
	}
	anomaly = manager.ProcessEvent(unknownEvent)
	if anomaly == nil {
		t.Error("Expected anomaly for unknown process")
	}
	if anomaly.Type != "process" {
		t.Errorf("Expected type=process, got %s", anomaly.Type)
	}
	if anomaly.Confidence < 0.5 {
		t.Errorf("Expected confidence > 0.5, got %f", anomaly.Confidence)
	}
}

func TestBaselineManager_NetworkBaseline(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Learn a destination
	learnEvent := &SecurityEvent{
		Type:        EventTypeNetwork,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		DstIP:       "10.0.0.1",
		DstPort:     443,
	}
	manager.ProcessEvent(learnEvent)

	time.Sleep(10 * time.Millisecond)

	// Known destination - no anomaly
	knownEvent := &SecurityEvent{
		Type:        EventTypeNetwork,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		DstIP:       "10.0.0.1",
		DstPort:     443,
	}
	anomaly := manager.ProcessEvent(knownEvent)
	if anomaly != nil {
		t.Error("Expected no anomaly for known destination")
	}

	// Unknown destination - anomaly
	unknownEvent := &SecurityEvent{
		Type:        EventTypeNetwork,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		DstIP:       "evil.com",
		DstPort:     4444,
	}
	anomaly = manager.ProcessEvent(unknownEvent)
	if anomaly == nil {
		t.Error("Expected anomaly for unknown destination")
	}
	if anomaly.Type != "network" {
		t.Errorf("Expected type=network, got %s", anomaly.Type)
	}
}

func TestBaselineManager_FileBaseline(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Learn a file access
	learnEvent := &SecurityEvent{
		Type:        EventTypeFile,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		FilePath:    "/var/log/app.log",
		Operation:   "write",
		Comm:        "app",
	}
	manager.ProcessEvent(learnEvent)

	time.Sleep(10 * time.Millisecond)

	// Known file access - no anomaly
	knownEvent := &SecurityEvent{
		Type:        EventTypeFile,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		FilePath:    "/var/log/app.log",
		Operation:   "write",
		Comm:        "app",
	}
	anomaly := manager.ProcessEvent(knownEvent)
	if anomaly != nil {
		t.Error("Expected no anomaly for known file access")
	}

	// Sensitive file access - should be high severity anomaly
	sensitiveEvent := &SecurityEvent{
		Type:        EventTypeFile,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		FilePath:    "/etc/shadow",
		Operation:   "read",
		Comm:        "app",
	}
	anomaly = manager.ProcessEvent(sensitiveEvent)
	if anomaly == nil {
		t.Error("Expected anomaly for sensitive file access")
	}
	if anomaly.Severity != SeverityHigh {
		t.Errorf("Expected severity=high, got %s", anomaly.Severity)
	}
}

func TestBaselineManager_HostBaseline(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Event without container ID should use host baseline
	event := &SecurityEvent{
		Type:      EventTypeProcess,
		Timestamp: time.Now(),
		Comm:      "sshd",
	}
	manager.ProcessEvent(event)

	hostBaseline := manager.GetContainerBaseline("host")
	if hostBaseline == nil {
		t.Fatal("Expected host baseline to exist")
	}
	if _, ok := hostBaseline.ProcessBaseline.AllowedProcesses["sshd"]; !ok {
		t.Error("Expected sshd to be in host baseline")
	}
}

func TestBaselineManager_MarkFalsePositive(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Process event to create baseline
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	})

	time.Sleep(10 * time.Millisecond)

	// This would normally be an anomaly
	unknownEvent := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "worker",
		ExePath:     "/usr/bin/worker",
	}

	// Mark it as false positive
	manager.MarkFalsePositive(unknownEvent)

	// Now it should not be an anomaly
	anomaly := manager.ProcessEvent(unknownEvent)
	if anomaly != nil {
		t.Error("Expected no anomaly after marking false positive")
	}

	// Verify it was added to baseline
	baseline := manager.GetContainerBaseline("container-1")
	if info, ok := baseline.ProcessBaseline.AllowedProcesses["worker"]; !ok {
		t.Error("Expected worker to be in allowed processes")
	} else if !info.Whitelisted {
		t.Error("Expected worker to be marked as whitelisted")
	}
}

func TestBaselineManager_ResetBaseline(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Create baseline
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	})

	baseline := manager.GetContainerBaseline("container-1")
	if baseline == nil {
		t.Fatal("Expected baseline to exist")
	}

	// Reset it
	manager.ResetBaseline("container-1")

	baseline = manager.GetContainerBaseline("container-1")
	if baseline != nil {
		t.Error("Expected baseline to be deleted after reset")
	}
}

func TestBaselineManager_Stats(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Create a few baselines
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	})
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-2",
		Timestamp:   time.Now(),
		Comm:        "redis",
	})
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeNetwork,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		DstIP:       "10.0.0.1",
		DstPort:     80,
	})

	stats := manager.GetBaselineStats()

	if stats["total_containers"].(int) != 2 {
		t.Errorf("Expected 2 containers, got %d", stats["total_containers"].(int))
	}
	if stats["total_known_processes"].(int) < 2 {
		t.Errorf("Expected at least 2 processes, got %d", stats["total_known_processes"].(int))
	}
	if stats["total_known_destinations"].(int) < 1 {
		t.Errorf("Expected at least 1 destination, got %d", stats["total_known_destinations"].(int))
	}
}

func TestBaselineManager_ParentChildRelationship(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	// Learn parent-child relationship: nginx spawns worker
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "worker",
		ParentComm:  "nginx",
	})
	// Learn nginx as a known parent
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	})

	time.Sleep(10 * time.Millisecond)

	// Same relationship - no anomaly
	goodChild := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "worker",
		ParentComm:  "nginx",
	}
	anomaly := manager.ProcessEvent(goodChild)
	if anomaly != nil {
		t.Error("Expected no anomaly for known parent-child relationship")
	}

	// nginx spawning something unexpected - should be anomaly
	// (nginx has known children [worker], so spawning 'bash' is unexpected)
	unexpectedChild := &SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "bash",
		ParentComm:  "nginx",
	}
	anomaly = manager.ProcessEvent(unexpectedChild)
	// bash is unknown process, so it should be flagged
	if anomaly == nil {
		t.Error("Expected anomaly for unknown process (bash)")
	}
	if anomaly.Type != "process" {
		t.Errorf("Expected type=process, got %s", anomaly.Type)
	}
}

func TestIsSensitivePath(t *testing.T) {
	tests := []struct {
		path      string
		sensitive bool
	}{
		{"/etc/shadow", true},
		{"/etc/passwd", true},
		{"/root/.ssh/id_rsa", true},
		{"/var/run/docker.sock", true},
		{"/var/log/app.log", false},
		{"/tmp/foo", false},
		{"/home/user/app", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isSensitivePath(tt.path)
			if result != tt.sensitive {
				t.Errorf("isSensitivePath(%q) = %v, want %v", tt.path, result, tt.sensitive)
			}
		})
	}
}

func TestBaselineManager_AnomalyCallback(t *testing.T) {
	manager := NewBaselineManager(1 * time.Millisecond)

	var receivedAnomaly *BaselineAnomaly
	manager.SetAnomalyCallback(func(a *BaselineAnomaly) {
		receivedAnomaly = a
	})

	// Learn something
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "nginx",
	})

	time.Sleep(10 * time.Millisecond)

	// Trigger anomaly
	manager.ProcessEvent(&SecurityEvent{
		Type:        EventTypeProcess,
		ContainerID: "container-1",
		Timestamp:   time.Now(),
		Comm:        "cryptominer",
	})

	if receivedAnomaly == nil {
		t.Error("Expected anomaly callback to be called")
	}
	if receivedAnomaly.Type != "process" {
		t.Errorf("Expected anomaly type=process, got %s", receivedAnomaly.Type)
	}
}
