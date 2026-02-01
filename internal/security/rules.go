package security

import (
	"regexp"
	"strings"
	"time"
)

// RuleType categorizes security detection rules
type RuleType string

const (
	RuleTypeProcess   RuleType = "process"
	RuleTypeNetwork   RuleType = "network"
	RuleTypeFile      RuleType = "file"
	RuleTypeContainer RuleType = "container"
)

// ThreatRule defines a security detection rule
type ThreatRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Type        RuleType `json:"type"`
	Severity    Severity `json:"severity"`

	// MITRE ATT&CK mapping
	MitreTactic      string `json:"mitre_tactic,omitempty"`
	MitreTechnique   string `json:"mitre_technique,omitempty"`
	MitreTechniqueID string `json:"mitre_technique_id,omitempty"`

	// Conditions
	Conditions []RuleCondition `json:"conditions"`

	// Tags for categorization
	Tags []string `json:"tags,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`

	// Custom matcher function (set by rule initialization)
	matchFunc func(*SecurityEvent) bool `json:"-"`
}

// RuleCondition defines a single condition within a rule
type RuleCondition struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"` // eq, neq, contains, regex, in, gt, lt
	Value    string   `json:"value"`
	Values   []string `json:"values,omitempty"` // for "in" operator
}

// Match checks if an event matches the rule
func (r *ThreatRule) Match(event *SecurityEvent) bool {
	if !r.Enabled {
		return false
	}

	// Check event type matches rule type
	if !r.matchesType(event.Type) {
		return false
	}

	// Use custom matcher if defined
	if r.matchFunc != nil {
		return r.matchFunc(event)
	}

	// Check all conditions (AND logic)
	for _, cond := range r.Conditions {
		if !r.matchCondition(event, &cond) {
			return false
		}
	}

	return len(r.Conditions) > 0
}

func (r *ThreatRule) matchesType(eventType EventType) bool {
	switch r.Type {
	case RuleTypeProcess:
		return eventType == EventTypeProcess
	case RuleTypeNetwork:
		return eventType == EventTypeNetwork
	case RuleTypeFile:
		return eventType == EventTypeFile
	case RuleTypeContainer:
		return eventType == EventTypeContainer || eventType == EventTypeProcess
	}
	return true
}

func (r *ThreatRule) matchCondition(event *SecurityEvent, cond *RuleCondition) bool {
	value := r.getFieldValue(event, cond.Field)

	switch cond.Operator {
	case "eq":
		return value == cond.Value
	case "neq":
		return value != cond.Value
	case "contains":
		return strings.Contains(strings.ToLower(value), strings.ToLower(cond.Value))
	case "regex":
		matched, _ := regexp.MatchString(cond.Value, value)
		return matched
	case "in":
		for _, v := range cond.Values {
			if value == v {
				return true
			}
		}
		return false
	case "not_in":
		for _, v := range cond.Values {
			if value == v {
				return false
			}
		}
		return true
	case "exists":
		return value != ""
	case "not_exists":
		return value == ""
	}

	return false
}

func (r *ThreatRule) getFieldValue(event *SecurityEvent, field string) string {
	switch field {
	case "comm":
		return event.Comm
	case "cmdline":
		return event.Cmdline
	case "exe_path":
		return event.ExePath
	case "parent_comm":
		return event.ParentComm
	case "file_path":
		return event.FilePath
	case "operation":
		return event.Operation
	case "src_ip":
		return event.SrcIP
	case "dst_ip":
		return event.DstIP
	case "protocol":
		return event.Protocol
	case "container_id":
		return event.ContainerID
	case "container_name":
		return event.ContainerName
	case "image_name":
		return event.ImageName
	case "namespace":
		return event.Namespace
	case "hostname":
		return event.Hostname
	default:
		if event.Attributes != nil {
			return event.Attributes[field]
		}
	}
	return ""
}

// RulesEngine manages threat detection rules
type RulesEngine struct {
	rules []*ThreatRule
}

// NewRulesEngine creates a new rules engine with built-in rules
func NewRulesEngine() *RulesEngine {
	engine := &RulesEngine{
		rules: make([]*ThreatRule, 0),
	}
	engine.loadBuiltinRules()
	return engine
}

// AddRule adds a custom rule
func (e *RulesEngine) AddRule(rule *ThreatRule) {
	e.rules = append(e.rules, rule)
}

// GetRules returns all rules
func (e *RulesEngine) GetRules() []*ThreatRule {
	return e.rules
}

// GetRule returns a rule by ID
func (e *RulesEngine) GetRule(id string) *ThreatRule {
	for _, r := range e.rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// EnableRule enables a rule
func (e *RulesEngine) EnableRule(id string) {
	if r := e.GetRule(id); r != nil {
		r.Enabled = true
	}
}

// DisableRule disables a rule
func (e *RulesEngine) DisableRule(id string) {
	if r := e.GetRule(id); r != nil {
		r.Enabled = false
	}
}

// Match returns all rules that match an event
func (e *RulesEngine) Match(event *SecurityEvent) []*ThreatRule {
	var matched []*ThreatRule
	for _, rule := range e.rules {
		if rule.Match(event) {
			matched = append(matched, rule)
		}
	}
	return matched
}

// loadBuiltinRules initializes the built-in detection rules
func (e *RulesEngine) loadBuiltinRules() {
	// Shell spawned in container
	e.rules = append(e.rules, &ThreatRule{
		ID:               "shell_in_container",
		Name:             "Shell Spawned in Container",
		Description:      "A shell process (bash, sh, zsh, ash) was spawned inside a container",
		Enabled:          true,
		Type:             RuleTypeContainer,
		Severity:         SeverityHigh,
		MitreTactic:      "Execution",
		MitreTechnique:   "Command and Scripting Interpreter",
		MitreTechniqueID: "T1059",
		Tags:             []string{"container", "shell", "execution"},
		matchFunc: func(event *SecurityEvent) bool {
			if event.ContainerID == "" {
				return false
			}
			shells := []string{"bash", "sh", "zsh", "ash", "dash", "csh", "tcsh", "ksh"}
			comm := strings.ToLower(event.Comm)
			for _, shell := range shells {
				if comm == shell {
					return true
				}
			}
			return false
		},
	})

	// Cryptominer detection - known processes
	e.rules = append(e.rules, &ThreatRule{
		ID:               "cryptominer_process",
		Name:             "Cryptominer Process Detected",
		Description:      "A known cryptocurrency mining process was detected",
		Enabled:          true,
		Type:             RuleTypeProcess,
		Severity:         SeverityCritical,
		MitreTactic:      "Impact",
		MitreTechnique:   "Resource Hijacking",
		MitreTechniqueID: "T1496",
		Tags:             []string{"cryptominer", "malware"},
		matchFunc: func(event *SecurityEvent) bool {
			miners := []string{
				"xmrig", "minerd", "cpuminer", "cgminer", "bfgminer",
				"ethminer", "claymore", "phoenixminer", "t-rex", "gminer",
				"lolminer", "nbminer", "nanominer", "xmr-stak", "ccminer",
				"nicehash", "kryptex", "minergate", "coinhive",
			}
			comm := strings.ToLower(event.Comm)
			cmdline := strings.ToLower(event.Cmdline)
			for _, miner := range miners {
				if strings.Contains(comm, miner) || strings.Contains(cmdline, miner) {
					return true
				}
			}
			// Check for stratum protocol in cmdline (mining pools)
			if strings.Contains(cmdline, "stratum+tcp://") || strings.Contains(cmdline, "stratum+ssl://") {
				return true
			}
			return false
		},
	})

	// Reverse shell detection
	e.rules = append(e.rules, &ThreatRule{
		ID:               "reverse_shell",
		Name:             "Reverse Shell Detected",
		Description:      "A potential reverse shell connection was detected",
		Enabled:          true,
		Type:             RuleTypeProcess,
		Severity:         SeverityCritical,
		MitreTactic:      "Command and Control",
		MitreTechnique:   "Ingress Tool Transfer",
		MitreTechniqueID: "T1105",
		Tags:             []string{"reverse_shell", "c2", "persistence"},
		matchFunc: func(event *SecurityEvent) bool {
			cmdline := strings.ToLower(event.Cmdline)
			// Common reverse shell patterns
			patterns := []string{
				"bash -i",
				"/dev/tcp/",
				"/dev/udp/",
				"nc -e",
				"nc -c",
				"ncat -e",
				"netcat -e",
				"python -c 'import socket",
				"python3 -c 'import socket",
				"perl -e 'use socket",
				"ruby -rsocket",
				"php -r '$sock=fsockopen",
				"mkfifo",
				"socat tcp",
				"0<&196;exec 196<>/dev/tcp",
			}
			for _, pattern := range patterns {
				if strings.Contains(cmdline, pattern) {
					return true
				}
			}
			return false
		},
	})

	// Privileged container detection
	e.rules = append(e.rules, &ThreatRule{
		ID:               "privileged_container",
		Name:             "Privileged Container Detected",
		Description:      "A container running with privileged mode was detected",
		Enabled:          true,
		Type:             RuleTypeContainer,
		Severity:         SeverityHigh,
		MitreTactic:      "Privilege Escalation",
		MitreTechnique:   "Escape to Host",
		MitreTechniqueID: "T1611",
		Tags:             []string{"container", "privileged", "escape"},
		matchFunc: func(event *SecurityEvent) bool {
			return event.ContainerID != "" && event.Privileged
		},
	})

	// Sensitive file access
	e.rules = append(e.rules, &ThreatRule{
		ID:               "sensitive_file_access",
		Name:             "Sensitive File Access",
		Description:      "Access to sensitive system files was detected",
		Enabled:          true,
		Type:             RuleTypeFile,
		Severity:         SeverityHigh,
		MitreTactic:      "Credential Access",
		MitreTechnique:   "Unsecured Credentials",
		MitreTechniqueID: "T1552",
		Tags:             []string{"credential", "file_access"},
		matchFunc: func(event *SecurityEvent) bool {
			sensitiveFiles := []string{
				"/etc/shadow",
				"/etc/passwd",
				"/etc/sudoers",
				"/etc/ssh/sshd_config",
				"/.ssh/id_rsa",
				"/.ssh/id_dsa",
				"/.ssh/id_ecdsa",
				"/.ssh/id_ed25519",
				"/.ssh/authorized_keys",
				"/root/.ssh/",
				"/etc/kubernetes/",
				"/var/run/secrets/kubernetes.io",
				"/.aws/credentials",
				"/.docker/config.json",
				"/etc/ssl/private/",
			}
			path := event.FilePath
			for _, sensitive := range sensitiveFiles {
				if strings.Contains(path, sensitive) {
					return true
				}
			}
			return false
		},
	})

	// Suspicious outbound connection
	e.rules = append(e.rules, &ThreatRule{
		ID:               "suspicious_outbound",
		Name:             "Suspicious Outbound Connection",
		Description:      "Connection to a suspicious port or known malicious service",
		Enabled:          true,
		Type:             RuleTypeNetwork,
		Severity:         SeverityMedium,
		MitreTactic:      "Command and Control",
		MitreTechnique:   "Non-Standard Port",
		MitreTechniqueID: "T1571",
		Tags:             []string{"network", "c2", "exfiltration"},
		matchFunc: func(event *SecurityEvent) bool {
			// Suspicious ports commonly used by malware/miners
			suspiciousPorts := map[uint16]bool{
				4444:  true, // Metasploit default
				5555:  true, // Common backdoor
				6666:  true, // IRC/backdoors
				6667:  true, // IRC
				6697:  true, // IRC SSL
				8333:  true, // Bitcoin
				8545:  true, // Ethereum RPC
				9050:  true, // Tor SOCKS
				9150:  true, // Tor Browser
				14444: true, // Mining pool
				45700: true, // Mining pool
			}
			if suspiciousPorts[event.DstPort] {
				return true
			}
			// Mining pool ports typically 3333, 5555, 7777, 9999
			miningPorts := []uint16{3333, 5555, 7777, 9999, 14444, 45700}
			for _, port := range miningPorts {
				if event.DstPort == port {
					return true
				}
			}
			return false
		},
	})

	// Container escape attempts
	e.rules = append(e.rules, &ThreatRule{
		ID:               "container_escape",
		Name:             "Container Escape Attempt",
		Description:      "Potential container escape attempt detected",
		Enabled:          true,
		Type:             RuleTypeContainer,
		Severity:         SeverityCritical,
		MitreTactic:      "Privilege Escalation",
		MitreTechnique:   "Escape to Host",
		MitreTechniqueID: "T1611",
		Tags:             []string{"container", "escape", "privilege_escalation"},
		matchFunc: func(event *SecurityEvent) bool {
			if event.ContainerID == "" {
				return false
			}
			cmdline := strings.ToLower(event.Cmdline)
			// Check for common escape techniques
			escapePatterns := []string{
				"nsenter",
				"--mount=/proc/1/ns/mnt",
				"/proc/1/root",
				"docker.sock",
				"containerd.sock",
				"cri-dockerd.sock",
				"kubelet",
				"release_agent",
				"notify_on_release",
				"cgroup",
			}
			for _, pattern := range escapePatterns {
				if strings.Contains(cmdline, pattern) {
					return true
				}
			}
			// Check for capabilities abuse
			if strings.Contains(cmdline, "capsh") || strings.Contains(cmdline, "setcap") {
				return true
			}
			return false
		},
	})

	// Suspicious process ancestry
	e.rules = append(e.rules, &ThreatRule{
		ID:               "web_shell_execution",
		Name:             "Web Server Spawning Shell",
		Description:      "A web server process spawned a shell or command interpreter",
		Enabled:          true,
		Type:             RuleTypeProcess,
		Severity:         SeverityCritical,
		MitreTactic:      "Execution",
		MitreTechnique:   "Command and Scripting Interpreter",
		MitreTechniqueID: "T1059",
		Tags:             []string{"webshell", "execution"},
		matchFunc: func(event *SecurityEvent) bool {
			webServers := []string{"nginx", "apache", "httpd", "lighttpd", "caddy", "php-fpm", "php", "node", "python", "ruby", "java"}
			shells := []string{"bash", "sh", "zsh", "ash", "dash", "csh", "tcsh", "ksh", "cmd", "powershell"}

			parentComm := strings.ToLower(event.ParentComm)
			comm := strings.ToLower(event.Comm)

			isWebServer := false
			for _, ws := range webServers {
				if strings.Contains(parentComm, ws) {
					isWebServer = true
					break
				}
			}
			if !isWebServer {
				return false
			}

			for _, shell := range shells {
				if comm == shell {
					return true
				}
			}
			return false
		},
	})

	// Kernel module loading
	e.rules = append(e.rules, &ThreatRule{
		ID:               "kernel_module_load",
		Name:             "Kernel Module Loading",
		Description:      "Kernel module was loaded, which could be used for rootkit installation",
		Enabled:          true,
		Type:             RuleTypeProcess,
		Severity:         SeverityHigh,
		MitreTactic:      "Persistence",
		MitreTechnique:   "Kernel Modules and Extensions",
		MitreTechniqueID: "T1547.006",
		Tags:             []string{"rootkit", "persistence", "kernel"},
		matchFunc: func(event *SecurityEvent) bool {
			comm := strings.ToLower(event.Comm)
			cmdline := strings.ToLower(event.Cmdline)
			return comm == "insmod" || comm == "modprobe" ||
				strings.Contains(cmdline, "insmod") ||
				strings.Contains(cmdline, "modprobe")
		},
	})

	// Data exfiltration via DNS
	e.rules = append(e.rules, &ThreatRule{
		ID:               "dns_exfiltration",
		Name:             "Potential DNS Exfiltration",
		Description:      "Unusually long DNS query that may indicate data exfiltration",
		Enabled:          true,
		Type:             RuleTypeNetwork,
		Severity:         SeverityMedium,
		MitreTactic:      "Exfiltration",
		MitreTechnique:   "Exfiltration Over Alternative Protocol",
		MitreTechniqueID: "T1048",
		Tags:             []string{"dns", "exfiltration", "c2"},
		Conditions: []RuleCondition{
			{Field: "dst_port", Operator: "eq", Value: "53"},
		},
		// Note: Additional check for query length would need DNS payload inspection
	})

	// Scheduled task creation (cron)
	e.rules = append(e.rules, &ThreatRule{
		ID:               "scheduled_task",
		Name:             "Scheduled Task Creation",
		Description:      "A scheduled task (cron job) was created or modified",
		Enabled:          true,
		Type:             RuleTypeFile,
		Severity:         SeverityMedium,
		MitreTactic:      "Persistence",
		MitreTechnique:   "Scheduled Task/Job",
		MitreTechniqueID: "T1053",
		Tags:             []string{"persistence", "cron", "scheduled_task"},
		matchFunc: func(event *SecurityEvent) bool {
			path := event.FilePath
			cronPaths := []string{
				"/etc/cron",
				"/var/spool/cron",
				"/etc/crontab",
				"/etc/anacrontab",
			}
			for _, cronPath := range cronPaths {
				if strings.HasPrefix(path, cronPath) {
					return true
				}
			}
			return false
		},
	})

	// Systemd service creation
	e.rules = append(e.rules, &ThreatRule{
		ID:               "systemd_service_creation",
		Name:             "Systemd Service Creation",
		Description:      "A new systemd service was created, which could be used for persistence",
		Enabled:          true,
		Type:             RuleTypeFile,
		Severity:         SeverityMedium,
		MitreTactic:      "Persistence",
		MitreTechnique:   "Create or Modify System Process",
		MitreTechniqueID: "T1543.002",
		Tags:             []string{"persistence", "systemd", "service"},
		matchFunc: func(event *SecurityEvent) bool {
			path := event.FilePath
			operation := event.Operation
			if operation != "write" && operation != "create" {
				return false
			}
			systemdPaths := []string{
				"/etc/systemd/system/",
				"/usr/lib/systemd/system/",
				"/lib/systemd/system/",
				"/run/systemd/system/",
			}
			for _, sp := range systemdPaths {
				if strings.HasPrefix(path, sp) && strings.HasSuffix(path, ".service") {
					return true
				}
			}
			return false
		},
	})

	// Process injection tools
	e.rules = append(e.rules, &ThreatRule{
		ID:               "process_injection",
		Name:             "Process Injection Tool",
		Description:      "A tool commonly used for process injection was executed",
		Enabled:          true,
		Type:             RuleTypeProcess,
		Severity:         SeverityHigh,
		MitreTactic:      "Defense Evasion",
		MitreTechnique:   "Process Injection",
		MitreTechniqueID: "T1055",
		Tags:             []string{"injection", "evasion"},
		matchFunc: func(event *SecurityEvent) bool {
			cmdline := strings.ToLower(event.Cmdline)
			injectionPatterns := []string{
				"ptrace",
				"process_vm_writev",
				"/proc/*/mem",
				"/proc/*/maps",
				"ld_preload",
				"ld_library_path",
			}
			for _, pattern := range injectionPatterns {
				if strings.Contains(cmdline, pattern) {
					return true
				}
			}
			return false
		},
	})
}
