package security

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Detector performs real-time security event analysis
type Detector struct {
	mu            sync.RWMutex
	engine        *RulesEngine
	store         *Store
	alertCallback AlertCallback
	eventCallback EventCallback

	// Deduplication
	recentAlerts map[string]time.Time
	dedupWindow  time.Duration

	// Stats
	eventsProcessed uint64
	alertsGenerated uint64
}

// DetectorConfig configures the detector
type DetectorConfig struct {
	// DedupWindow is how long to suppress duplicate alerts
	DedupWindow time.Duration

	// Store for persisting events and alerts
	Store *Store

	// Callbacks for alerting
	AlertCallback AlertCallback
	EventCallback EventCallback
}

// DefaultDetectorConfig returns default configuration
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		DedupWindow: 5 * time.Minute,
	}
}

// NewDetector creates a new security detector
func NewDetector(config DetectorConfig) *Detector {
	if config.DedupWindow == 0 {
		config.DedupWindow = 5 * time.Minute
	}

	d := &Detector{
		engine:        NewRulesEngine(),
		store:         config.Store,
		alertCallback: config.AlertCallback,
		eventCallback: config.EventCallback,
		recentAlerts:  make(map[string]time.Time),
		dedupWindow:   config.DedupWindow,
	}

	// Start cleanup goroutine
	go d.cleanupLoop()

	return d
}

// ProcessEvent analyzes a security event and generates alerts if rules match
func (d *Detector) ProcessEvent(event *SecurityEvent) []*SecurityAlert {
	d.mu.Lock()
	d.eventsProcessed++
	d.mu.Unlock()

	// Generate event ID if not set
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Store the event if store is configured
	if d.store != nil {
		if err := d.store.RecordEvent(event); err != nil {
			log.Printf("[security] Failed to store event: %v", err)
		}
	}

	// Notify event callback
	if d.eventCallback != nil {
		d.eventCallback(event)
	}

	// Match against rules
	matchedRules := d.engine.Match(event)
	if len(matchedRules) == 0 {
		return nil
	}

	var alerts []*SecurityAlert
	for _, rule := range matchedRules {
		alert := d.createAlert(event, rule)
		if alert != nil {
			alerts = append(alerts, alert)
		}
	}

	return alerts
}

// createAlert creates a security alert from an event and matched rule
func (d *Detector) createAlert(event *SecurityEvent, rule *ThreatRule) *SecurityAlert {
	// Generate fingerprint for deduplication
	fingerprint := d.generateFingerprint(event, rule)

	// Check for duplicate within window
	d.mu.Lock()
	if lastSeen, exists := d.recentAlerts[fingerprint]; exists {
		if time.Since(lastSeen) < d.dedupWindow {
			d.mu.Unlock()
			return nil
		}
	}
	d.recentAlerts[fingerprint] = time.Now()
	d.alertsGenerated++
	d.mu.Unlock()

	alert := &SecurityAlert{
		ID:               uuid.New().String(),
		RuleID:           rule.ID,
		RuleName:         rule.Name,
		Severity:         rule.Severity,
		State:            AlertStateOpen,
		Title:            d.generateTitle(event, rule),
		Description:      d.generateDescription(event, rule),
		MitreTactic:      rule.MitreTactic,
		MitreTechnique:   rule.MitreTechnique,
		MitreTechniqueID: rule.MitreTechniqueID,
		Event:            event,
		EventID:          event.ID,
		HostID:           event.HostID,
		Hostname:         event.Hostname,
		ContainerID:      event.ContainerID,
		ContainerName:    event.ContainerName,
		PodName:          event.PodName,
		Namespace:        event.Namespace,
		DetectedAt:       time.Now(),
		Labels:           make(map[string]string),
		Indicators:       d.extractIndicators(event),
	}

	// Add labels
	alert.Labels["rule_id"] = rule.ID
	alert.Labels["severity"] = string(rule.Severity)
	if event.ContainerID != "" {
		alert.Labels["container_id"] = event.ContainerID
	}
	if event.Namespace != "" {
		alert.Labels["namespace"] = event.Namespace
	}
	for _, tag := range rule.Tags {
		alert.Labels["tag:"+tag] = "true"
	}

	// Store the alert
	if d.store != nil {
		if err := d.store.RecordAlert(alert); err != nil {
			log.Printf("[security] Failed to store alert: %v", err)
		}
	}

	// Notify callback
	if d.alertCallback != nil {
		d.alertCallback(alert)
	}

	log.Printf("[security] Alert: %s - %s (severity: %s)", rule.Name, alert.Title, rule.Severity)

	return alert
}

// generateFingerprint creates a unique fingerprint for deduplication
func (d *Detector) generateFingerprint(event *SecurityEvent, rule *ThreatRule) string {
	// Include rule ID, host, container, and key event attributes
	data := fmt.Sprintf("%s:%s:%s:%s:%s:%d",
		rule.ID,
		event.HostID,
		event.ContainerID,
		event.Comm,
		event.FilePath,
		event.DstPort,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

// generateTitle creates a human-readable alert title
func (d *Detector) generateTitle(event *SecurityEvent, rule *ThreatRule) string {
	switch rule.Type {
	case RuleTypeProcess:
		if event.ContainerID != "" {
			return fmt.Sprintf("%s: %s in container %s", rule.Name, event.Comm, d.truncate(event.ContainerName, 20))
		}
		return fmt.Sprintf("%s: %s on %s", rule.Name, event.Comm, event.Hostname)

	case RuleTypeNetwork:
		return fmt.Sprintf("%s: %s:%d -> %s:%d", rule.Name, event.SrcIP, event.SrcPort, event.DstIP, event.DstPort)

	case RuleTypeFile:
		return fmt.Sprintf("%s: %s accessing %s", rule.Name, event.Comm, d.truncate(event.FilePath, 40))

	case RuleTypeContainer:
		return fmt.Sprintf("%s: container %s", rule.Name, d.truncate(event.ContainerName, 30))

	default:
		return rule.Name
	}
}

// generateDescription creates a detailed alert description
func (d *Detector) generateDescription(event *SecurityEvent, rule *ThreatRule) string {
	desc := rule.Description + "\n\n"

	if event.Comm != "" {
		desc += fmt.Sprintf("Process: %s (PID: %d)\n", event.Comm, event.PID)
	}
	if event.Cmdline != "" {
		desc += fmt.Sprintf("Command: %s\n", d.truncate(event.Cmdline, 200))
	}
	if event.ParentComm != "" {
		desc += fmt.Sprintf("Parent: %s (PPID: %d)\n", event.ParentComm, event.PPID)
	}
	if event.FilePath != "" {
		desc += fmt.Sprintf("File: %s\n", event.FilePath)
	}
	if event.DstIP != "" {
		desc += fmt.Sprintf("Network: %s:%d -> %s:%d\n", event.SrcIP, event.SrcPort, event.DstIP, event.DstPort)
	}
	if event.ContainerID != "" {
		desc += fmt.Sprintf("Container: %s (%s)\n", event.ContainerName, d.truncate(event.ContainerID, 12))
	}
	if event.PodName != "" {
		desc += fmt.Sprintf("Pod: %s/%s\n", event.Namespace, event.PodName)
	}
	if rule.MitreTechniqueID != "" {
		desc += fmt.Sprintf("\nMITRE ATT&CK: %s - %s (%s)", rule.MitreTactic, rule.MitreTechnique, rule.MitreTechniqueID)
	}

	return desc
}

// extractIndicators extracts IOCs from the event
func (d *Detector) extractIndicators(event *SecurityEvent) []string {
	var indicators []string

	if event.DstIP != "" && event.DstIP != "127.0.0.1" && event.DstIP != "::1" {
		indicators = append(indicators, "ip:"+event.DstIP)
	}
	if event.FilePath != "" {
		indicators = append(indicators, "path:"+event.FilePath)
	}
	if event.Comm != "" {
		indicators = append(indicators, "process:"+event.Comm)
	}
	if event.ContainerID != "" {
		indicators = append(indicators, "container:"+event.ContainerID)
	}

	return indicators
}

// truncate shortens a string to max length
func (d *Detector) truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// cleanupLoop periodically cleans up the deduplication cache
func (d *Detector) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		d.cleanup()
	}
}

func (d *Detector) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for fingerprint, lastSeen := range d.recentAlerts {
		if now.Sub(lastSeen) > d.dedupWindow {
			delete(d.recentAlerts, fingerprint)
		}
	}
}

// GetRulesEngine returns the underlying rules engine
func (d *Detector) GetRulesEngine() *RulesEngine {
	return d.engine
}

// GetStats returns detector statistics
func (d *Detector) GetStats() DetectorStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return DetectorStats{
		EventsProcessed:  d.eventsProcessed,
		AlertsGenerated:  d.alertsGenerated,
		RulesLoaded:      len(d.engine.rules),
		DedupCacheSize:   len(d.recentAlerts),
	}
}

// DetectorStats contains detector statistics
type DetectorStats struct {
	EventsProcessed uint64 `json:"events_processed"`
	AlertsGenerated uint64 `json:"alerts_generated"`
	RulesLoaded     int    `json:"rules_loaded"`
	DedupCacheSize  int    `json:"dedup_cache_size"`
}

// ProcessProcessExec handles process execution events (from eBPF)
func (d *Detector) ProcessProcessExec(pid, ppid, uid, gid uint32, comm, cmdline, exePath, parentComm string,
	containerID, containerName, podName, namespace, imageName string, privileged bool,
	hostID, hostname string) []*SecurityAlert {

	event := &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeProcess,
		HostID:        hostID,
		Hostname:      hostname,
		PID:           pid,
		PPID:          ppid,
		UID:           uid,
		GID:           gid,
		Comm:          comm,
		Cmdline:       cmdline,
		ExePath:       exePath,
		ParentComm:    parentComm,
		ContainerID:   containerID,
		ContainerName: containerName,
		PodName:       podName,
		Namespace:     namespace,
		ImageName:     imageName,
		Privileged:    privileged,
	}

	return d.ProcessEvent(event)
}

// ProcessNetworkConnect handles network connection events (from eBPF)
func (d *Detector) ProcessNetworkConnect(pid uint32, comm string, srcIP string, srcPort uint16,
	dstIP string, dstPort uint16, protocol string,
	containerID, containerName string, hostID, hostname string) []*SecurityAlert {

	event := &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeNetwork,
		HostID:        hostID,
		Hostname:      hostname,
		PID:           pid,
		Comm:          comm,
		SrcIP:         srcIP,
		SrcPort:       srcPort,
		DstIP:         dstIP,
		DstPort:       dstPort,
		Protocol:      protocol,
		ContainerID:   containerID,
		ContainerName: containerName,
	}

	return d.ProcessEvent(event)
}

// ProcessFileAccess handles file access events (from eBPF)
func (d *Detector) ProcessFileAccess(pid uint32, comm, filePath, operation string, fileMode uint32,
	containerID, containerName string, hostID, hostname string) []*SecurityAlert {

	event := &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeFile,
		HostID:        hostID,
		Hostname:      hostname,
		PID:           pid,
		Comm:          comm,
		FilePath:      filePath,
		Operation:     operation,
		FileMode:      fileMode,
		ContainerID:   containerID,
		ContainerName: containerName,
	}

	return d.ProcessEvent(event)
}

// SetAlertCallback sets the callback for new alerts
func (d *Detector) SetAlertCallback(cb AlertCallback) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.alertCallback = cb
}

// SetEventCallback sets the callback for processed events
func (d *Detector) SetEventCallback(cb EventCallback) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.eventCallback = cb
}
