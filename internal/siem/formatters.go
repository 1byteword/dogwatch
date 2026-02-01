package siem

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/security"
)

// SecurityEvent wraps security.SecurityEvent and security.SecurityAlert for export
type SecurityEvent struct {
	ID          string
	Timestamp   time.Time
	EventType   string // "event" or "alert"
	Severity    string
	Title       string
	Description string

	// Source/Destination
	SrcIP   string
	SrcPort uint16
	DstIP   string
	DstPort uint16

	// Process context
	PID     uint32
	Comm    string
	Cmdline string

	// Network context
	Protocol string
	Action   string // allow, block, detect

	// Container context
	ContainerID   string
	ContainerName string
	PodName       string
	Namespace     string

	// Host context
	Hostname string
	HostID   string

	// MITRE ATT&CK
	MitreTactic      string
	MitreTechnique   string
	MitreTechniqueID string

	// Alert-specific
	RuleID   string
	RuleName string
	State    string

	// Additional
	Category   string
	Outcome    string
	Message    string
	Indicators []string
	Labels     map[string]string
}

// Formatter defines the interface for SIEM format conversion
type Formatter interface {
	Format(event SecurityEvent) string
	ContentType() string
}

// CEFFormatter formats events in Common Event Format
// CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
type CEFFormatter struct {
	DeviceVendor  string
	DeviceProduct string
	DeviceVersion string
}

// NewCEFFormatter creates a new CEF formatter with default values
func NewCEFFormatter() *CEFFormatter {
	return &CEFFormatter{
		DeviceVendor:  "Dogwatch",
		DeviceProduct: "Dogwatch",
		DeviceVersion: "1.0",
	}
}

// Format converts a security event to CEF format
func (f *CEFFormatter) Format(event SecurityEvent) string {
	// Map severity to CEF (0-10 scale)
	cefSeverity := mapSeverityToCEF(event.Severity)

	// Build extension fields
	ext := f.buildCEFExtension(event)

	// Escape special characters in name and extension
	name := escapeCEF(event.Title)
	if name == "" {
		name = event.EventType
	}

	signatureID := event.RuleID
	if signatureID == "" {
		signatureID = event.ID
	}

	return fmt.Sprintf("CEF:0|%s|%s|%s|%s|%s|%d|%s",
		escapeCEF(f.DeviceVendor),
		escapeCEF(f.DeviceProduct),
		escapeCEF(f.DeviceVersion),
		escapeCEF(signatureID),
		name,
		cefSeverity,
		ext,
	)
}

// ContentType returns the MIME type for CEF
func (f *CEFFormatter) ContentType() string {
	return "text/plain"
}

// buildCEFExtension builds the CEF extension string with key=value pairs
func (f *CEFFormatter) buildCEFExtension(event SecurityEvent) string {
	var parts []string

	// Timestamp in CEF format: MMM dd yyyy HH:mm:ss or epoch millis
	if !event.Timestamp.IsZero() {
		parts = append(parts, fmt.Sprintf("rt=%d", event.Timestamp.UnixMilli()))
	}

	// Source address
	if event.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("src=%s", event.SrcIP))
	}
	if event.SrcPort > 0 {
		parts = append(parts, fmt.Sprintf("spt=%d", event.SrcPort))
	}

	// Destination address
	if event.DstIP != "" {
		parts = append(parts, fmt.Sprintf("dst=%s", event.DstIP))
	}
	if event.DstPort > 0 {
		parts = append(parts, fmt.Sprintf("dpt=%d", event.DstPort))
	}

	// Protocol
	if event.Protocol != "" {
		parts = append(parts, fmt.Sprintf("proto=%s", escapeCEFValue(event.Protocol)))
	}

	// Action
	if event.Action != "" {
		parts = append(parts, fmt.Sprintf("act=%s", escapeCEFValue(event.Action)))
	}

	// Process info
	if event.PID > 0 {
		parts = append(parts, fmt.Sprintf("sproc=%d", event.PID))
	}
	if event.Comm != "" {
		parts = append(parts, fmt.Sprintf("suser=%s", escapeCEFValue(event.Comm)))
	}
	if event.Cmdline != "" {
		// Truncate long command lines
		cmdline := event.Cmdline
		if len(cmdline) > 1000 {
			cmdline = cmdline[:1000] + "..."
		}
		parts = append(parts, fmt.Sprintf("msg=%s", escapeCEFValue(cmdline)))
	}

	// Host info
	if event.Hostname != "" {
		parts = append(parts, fmt.Sprintf("dhost=%s", escapeCEFValue(event.Hostname)))
	}

	// Category
	if event.Category != "" {
		parts = append(parts, fmt.Sprintf("cat=%s", escapeCEFValue(event.Category)))
	}

	// Outcome
	if event.Outcome != "" {
		parts = append(parts, fmt.Sprintf("outcome=%s", escapeCEFValue(event.Outcome)))
	}

	// Container context
	if event.ContainerID != "" {
		parts = append(parts, fmt.Sprintf("cs1=%s", escapeCEFValue(event.ContainerID)))
		parts = append(parts, "cs1Label=ContainerID")
	}
	if event.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("cs2=%s", escapeCEFValue(event.ContainerName)))
		parts = append(parts, "cs2Label=ContainerName")
	}
	if event.PodName != "" {
		parts = append(parts, fmt.Sprintf("cs3=%s", escapeCEFValue(event.PodName)))
		parts = append(parts, "cs3Label=PodName")
	}
	if event.Namespace != "" {
		parts = append(parts, fmt.Sprintf("cs4=%s", escapeCEFValue(event.Namespace)))
		parts = append(parts, "cs4Label=Namespace")
	}

	// MITRE ATT&CK
	if event.MitreTechniqueID != "" {
		parts = append(parts, fmt.Sprintf("cs5=%s", escapeCEFValue(event.MitreTechniqueID)))
		parts = append(parts, "cs5Label=MITRETechniqueID")
	}
	if event.MitreTactic != "" {
		parts = append(parts, fmt.Sprintf("cs6=%s", escapeCEFValue(event.MitreTactic)))
		parts = append(parts, "cs6Label=MITRETactic")
	}

	// Description/Message
	if event.Description != "" && event.Cmdline == "" {
		desc := event.Description
		if len(desc) > 1000 {
			desc = desc[:1000] + "..."
		}
		parts = append(parts, fmt.Sprintf("msg=%s", escapeCEFValue(desc)))
	}

	return strings.Join(parts, " ")
}

// LEEFFormatter formats events in Log Event Extended Format (IBM QRadar)
// LEEF:Version|Vendor|Product|Version|EventID|Extension
type LEEFFormatter struct {
	Vendor  string
	Product string
	Version string
}

// NewLEEFFormatter creates a new LEEF formatter with default values
func NewLEEFFormatter() *LEEFFormatter {
	return &LEEFFormatter{
		Vendor:  "Dogwatch",
		Product: "Dogwatch",
		Version: "1.0",
	}
}

// Format converts a security event to LEEF format
func (f *LEEFFormatter) Format(event SecurityEvent) string {
	// Build extension fields
	ext := f.buildLEEFExtension(event)

	eventID := event.RuleID
	if eventID == "" {
		eventID = event.ID
	}

	return fmt.Sprintf("LEEF:2.0|%s|%s|%s|%s|%s",
		escapeLEEF(f.Vendor),
		escapeLEEF(f.Product),
		escapeLEEF(f.Version),
		escapeLEEF(eventID),
		ext,
	)
}

// ContentType returns the MIME type for LEEF
func (f *LEEFFormatter) ContentType() string {
	return "text/plain"
}

// buildLEEFExtension builds the LEEF extension string
func (f *LEEFFormatter) buildLEEFExtension(event SecurityEvent) string {
	var parts []string

	// Timestamp as epoch seconds
	if !event.Timestamp.IsZero() {
		parts = append(parts, fmt.Sprintf("devTime=%d", event.Timestamp.Unix()))
	}

	// Severity (LEEF uses 1-10)
	leefSeverity := mapSeverityToLEEF(event.Severity)
	parts = append(parts, fmt.Sprintf("sev=%d", leefSeverity))

	// Source
	if event.SrcIP != "" {
		parts = append(parts, fmt.Sprintf("src=%s", event.SrcIP))
	}
	if event.SrcPort > 0 {
		parts = append(parts, fmt.Sprintf("srcPort=%d", event.SrcPort))
	}

	// Destination
	if event.DstIP != "" {
		parts = append(parts, fmt.Sprintf("dst=%s", event.DstIP))
	}
	if event.DstPort > 0 {
		parts = append(parts, fmt.Sprintf("dstPort=%d", event.DstPort))
	}

	// Protocol
	if event.Protocol != "" {
		parts = append(parts, fmt.Sprintf("proto=%s", escapeLEEFValue(event.Protocol)))
	}

	// Category
	if event.Category != "" {
		parts = append(parts, fmt.Sprintf("cat=%s", escapeLEEFValue(event.Category)))
	}

	// Action
	if event.Action != "" {
		parts = append(parts, fmt.Sprintf("action=%s", escapeLEEFValue(event.Action)))
	}

	// Process
	if event.Comm != "" {
		parts = append(parts, fmt.Sprintf("usrName=%s", escapeLEEFValue(event.Comm)))
	}

	// Host
	if event.Hostname != "" {
		parts = append(parts, fmt.Sprintf("identHostName=%s", escapeLEEFValue(event.Hostname)))
	}

	// Title/Name
	if event.Title != "" {
		parts = append(parts, fmt.Sprintf("name=%s", escapeLEEFValue(event.Title)))
	}

	// Description
	if event.Description != "" {
		desc := event.Description
		if len(desc) > 1000 {
			desc = desc[:1000] + "..."
		}
		parts = append(parts, fmt.Sprintf("msg=%s", escapeLEEFValue(desc)))
	}

	// Container context
	if event.ContainerID != "" {
		parts = append(parts, fmt.Sprintf("containerID=%s", escapeLEEFValue(event.ContainerID)))
	}
	if event.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("containerName=%s", escapeLEEFValue(event.ContainerName)))
	}
	if event.PodName != "" {
		parts = append(parts, fmt.Sprintf("podName=%s", escapeLEEFValue(event.PodName)))
	}
	if event.Namespace != "" {
		parts = append(parts, fmt.Sprintf("namespace=%s", escapeLEEFValue(event.Namespace)))
	}

	// MITRE
	if event.MitreTechniqueID != "" {
		parts = append(parts, fmt.Sprintf("mitreTechniqueID=%s", escapeLEEFValue(event.MitreTechniqueID)))
	}
	if event.MitreTactic != "" {
		parts = append(parts, fmt.Sprintf("mitreTactic=%s", escapeLEEFValue(event.MitreTactic)))
	}

	return strings.Join(parts, "\t")
}

// JSONFormatter formats events as JSON
type JSONFormatter struct{}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{}
}

// Format converts a security event to JSON format
func (f *JSONFormatter) Format(event SecurityEvent) string {
	// Build a clean JSON structure
	var sb strings.Builder
	sb.WriteString("{")

	// Core fields
	writeJSONString(&sb, "id", event.ID, true)
	writeJSONString(&sb, "timestamp", event.Timestamp.Format(time.RFC3339), false)
	writeJSONString(&sb, "event_type", event.EventType, false)
	writeJSONString(&sb, "severity", event.Severity, false)
	writeJSONString(&sb, "title", event.Title, false)

	if event.Description != "" {
		writeJSONString(&sb, "description", event.Description, false)
	}

	// Network
	if event.SrcIP != "" || event.DstIP != "" {
		sb.WriteString(`,"network":{`)
		first := true
		if event.SrcIP != "" {
			writeJSONString(&sb, "src_ip", event.SrcIP, first)
			first = false
		}
		if event.SrcPort > 0 {
			writeJSONInt(&sb, "src_port", int(event.SrcPort), first)
			first = false
		}
		if event.DstIP != "" {
			writeJSONString(&sb, "dst_ip", event.DstIP, first)
			first = false
		}
		if event.DstPort > 0 {
			writeJSONInt(&sb, "dst_port", int(event.DstPort), first)
			first = false
		}
		if event.Protocol != "" {
			writeJSONString(&sb, "protocol", event.Protocol, first)
		}
		sb.WriteString("}")
	}

	// Process
	if event.Comm != "" || event.PID > 0 {
		sb.WriteString(`,"process":{`)
		first := true
		if event.PID > 0 {
			writeJSONInt(&sb, "pid", int(event.PID), first)
			first = false
		}
		if event.Comm != "" {
			writeJSONString(&sb, "name", event.Comm, first)
			first = false
		}
		if event.Cmdline != "" {
			writeJSONString(&sb, "cmdline", event.Cmdline, first)
		}
		sb.WriteString("}")
	}

	// Host
	if event.Hostname != "" {
		writeJSONString(&sb, "hostname", event.Hostname, false)
	}

	// Container
	if event.ContainerID != "" {
		sb.WriteString(`,"container":{`)
		writeJSONString(&sb, "id", event.ContainerID, true)
		if event.ContainerName != "" {
			writeJSONString(&sb, "name", event.ContainerName, false)
		}
		if event.PodName != "" {
			writeJSONString(&sb, "pod", event.PodName, false)
		}
		if event.Namespace != "" {
			writeJSONString(&sb, "namespace", event.Namespace, false)
		}
		sb.WriteString("}")
	}

	// MITRE
	if event.MitreTechniqueID != "" {
		sb.WriteString(`,"mitre":{`)
		writeJSONString(&sb, "technique_id", event.MitreTechniqueID, true)
		if event.MitreTechnique != "" {
			writeJSONString(&sb, "technique", event.MitreTechnique, false)
		}
		if event.MitreTactic != "" {
			writeJSONString(&sb, "tactic", event.MitreTactic, false)
		}
		sb.WriteString("}")
	}

	// Rule info
	if event.RuleID != "" {
		writeJSONString(&sb, "rule_id", event.RuleID, false)
	}
	if event.RuleName != "" {
		writeJSONString(&sb, "rule_name", event.RuleName, false)
	}

	// Action/Outcome
	if event.Action != "" {
		writeJSONString(&sb, "action", event.Action, false)
	}
	if event.Outcome != "" {
		writeJSONString(&sb, "outcome", event.Outcome, false)
	}
	if event.Category != "" {
		writeJSONString(&sb, "category", event.Category, false)
	}

	sb.WriteString("}")
	return sb.String()
}

// ContentType returns the MIME type for JSON
func (f *JSONFormatter) ContentType() string {
	return "application/json"
}

// Helper functions for JSON building
func writeJSONString(sb *strings.Builder, key, value string, first bool) {
	if !first {
		sb.WriteString(",")
	}
	sb.WriteString(`"`)
	sb.WriteString(key)
	sb.WriteString(`":"`)
	sb.WriteString(escapeJSON(value))
	sb.WriteString(`"`)
}

func writeJSONInt(sb *strings.Builder, key string, value int, first bool) {
	if !first {
		sb.WriteString(",")
	}
	sb.WriteString(`"`)
	sb.WriteString(key)
	sb.WriteString(`":`)
	sb.WriteString(strconv.Itoa(value))
}

// Severity mapping functions

// mapSeverityToCEF maps dogwatch severity to CEF (0-10 scale)
func mapSeverityToCEF(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 10
	case "high":
		return 8
	case "medium":
		return 5
	case "low":
		return 3
	case "info":
		return 1
	default:
		return 0
	}
}

// mapSeverityToLEEF maps dogwatch severity to LEEF (1-10 scale)
func mapSeverityToLEEF(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 10
	case "high":
		return 8
	case "medium":
		return 5
	case "low":
		return 3
	case "info":
		return 1
	default:
		return 1
	}
}

// Escape functions

// escapeCEF escapes special characters in CEF header fields
// CEF header uses | as delimiter, must escape: | \ newlines
func escapeCEF(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// escapeCEFValue escapes special characters in CEF extension values
// Extension uses = as key-value delimiter
func escapeCEFValue(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "=", "\\=")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// escapeLEEF escapes special characters in LEEF header fields
// LEEF header uses | as delimiter
func escapeLEEF(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// escapeLEEFValue escapes special characters in LEEF extension values
// Extension uses tab as field delimiter
func escapeLEEFValue(s string) string {
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// escapeJSON escapes special characters in JSON values
func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// ConvertSecurityEvent converts a security.SecurityEvent to SIEM SecurityEvent
func ConvertSecurityEvent(event *security.SecurityEvent) SecurityEvent {
	return SecurityEvent{
		ID:            event.ID,
		Timestamp:     event.Timestamp,
		EventType:     "event",
		Severity:      "", // Events don't have severity, alerts do
		Title:         event.Comm + " activity detected",
		Description:   "",
		SrcIP:         event.SrcIP,
		SrcPort:       event.SrcPort,
		DstIP:         event.DstIP,
		DstPort:       event.DstPort,
		PID:           event.PID,
		Comm:          event.Comm,
		Cmdline:       event.Cmdline,
		Protocol:      event.Protocol,
		Action:        "detect",
		ContainerID:   event.ContainerID,
		ContainerName: event.ContainerName,
		PodName:       event.PodName,
		Namespace:     event.Namespace,
		Hostname:      event.Hostname,
		HostID:        event.HostID,
		Category:      string(event.Type),
		Outcome:       "unknown",
	}
}

// ConvertSecurityAlert converts a security.SecurityAlert to SIEM SecurityEvent
func ConvertSecurityAlert(alert *security.SecurityAlert) SecurityEvent {
	return SecurityEvent{
		ID:               alert.ID,
		Timestamp:        alert.DetectedAt,
		EventType:        "alert",
		Severity:         string(alert.Severity),
		Title:            alert.Title,
		Description:      alert.Description,
		SrcIP:            "", // Populated from event if available
		DstIP:            "",
		ContainerID:      alert.ContainerID,
		ContainerName:    alert.ContainerName,
		PodName:          alert.PodName,
		Namespace:        alert.Namespace,
		Hostname:         alert.Hostname,
		HostID:           alert.HostID,
		MitreTactic:      alert.MitreTactic,
		MitreTechnique:   alert.MitreTechnique,
		MitreTechniqueID: alert.MitreTechniqueID,
		RuleID:           alert.RuleID,
		RuleName:         alert.RuleName,
		State:            string(alert.State),
		Action:           "alert",
		Category:         "security_alert",
		Outcome:          mapAlertState(string(alert.State)),
		Indicators:       alert.Indicators,
		Labels:           alert.Labels,
	}
}

// mapAlertState maps alert state to outcome
func mapAlertState(state string) string {
	switch state {
	case "open":
		return "pending"
	case "acknowledged":
		return "in_progress"
	case "resolved":
		return "success"
	case "false_positive":
		return "false_positive"
	default:
		return "unknown"
	}
}

// GetFormatter returns the appropriate formatter for the format name
func GetFormatter(format string) Formatter {
	switch strings.ToLower(format) {
	case "cef":
		return NewCEFFormatter()
	case "leef":
		return NewLEEFFormatter()
	case "json":
		return NewJSONFormatter()
	default:
		return NewJSONFormatter()
	}
}

// SupportedFormats returns the list of supported export formats
func SupportedFormats() []string {
	return []string{"cef", "leef", "json"}
}
