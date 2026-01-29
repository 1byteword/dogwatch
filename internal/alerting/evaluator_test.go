package alerting

import (
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestNewEvaluator(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()

	evaluator := NewEvaluator(store, provider)
	if evaluator == nil {
		t.Fatal("NewEvaluator returned nil")
	}

	if evaluator.alerts == nil {
		t.Error("alerts map should be initialized")
	}

	if evaluator.evalState == nil {
		t.Error("evalState map should be initialized")
	}
}

func TestCheckCondition(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	tests := []struct {
		name      string
		value     float64
		condition string
		threshold float64
		expected  bool
	}{
		// Greater than
		{"gt - above threshold", 100, "gt", 90, true},
		{"gt - equal threshold", 90, "gt", 90, false},
		{"gt - below threshold", 80, "gt", 90, false},
		{"> alias - above", 100, ">", 90, true},

		// Less than
		{"lt - below threshold", 80, "lt", 90, true},
		{"lt - equal threshold", 90, "lt", 90, false},
		{"lt - above threshold", 100, "lt", 90, false},
		{"< alias - below", 80, "<", 90, true},

		// Greater than or equal
		{"gte - above threshold", 100, "gte", 90, true},
		{"gte - equal threshold", 90, "gte", 90, true},
		{"gte - below threshold", 80, "gte", 90, false},
		{">= alias - equal", 90, ">=", 90, true},

		// Less than or equal
		{"lte - below threshold", 80, "lte", 90, true},
		{"lte - equal threshold", 90, "lte", 90, true},
		{"lte - above threshold", 100, "lte", 90, false},
		{"<= alias - equal", 90, "<=", 90, true},

		// Equal
		{"eq - equal values", 90, "eq", 90, true},
		{"eq - different values", 89, "eq", 90, false},
		{"== alias - equal", 90, "==", 90, true},

		// Not equal
		{"neq - different values", 89, "neq", 90, true},
		{"neq - equal values", 90, "neq", 90, false},
		{"!= alias - different", 89, "!=", 90, true},

		// Invalid condition
		{"invalid condition", 100, "invalid", 90, false},
		{"empty condition", 100, "", 90, false},

		// Edge cases
		{"zero threshold gt", 0.001, "gt", 0, true},
		{"negative values lt", -100, "lt", -50, true},
		// Note: 0.1 + 0.2 == 0.3 in Go due to compiler optimizations in some cases
		// The classic float precision issue may or may not manifest
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.checkCondition(tt.value, tt.condition, tt.threshold)
			if result != tt.expected {
				t.Errorf("checkCondition(%f, %q, %f) = %v, want %v",
					tt.value, tt.condition, tt.threshold, result, tt.expected)
			}
		})
	}
}

func TestMergeLabels(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	tests := []struct {
		name     string
		base     map[string]string
		overlay  map[string]string
		expected map[string]string
	}{
		{
			name:     "empty maps",
			base:     map[string]string{},
			overlay:  map[string]string{},
			expected: map[string]string{},
		},
		{
			name:     "base only",
			base:     map[string]string{"a": "1", "b": "2"},
			overlay:  map[string]string{},
			expected: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:     "overlay only",
			base:     map[string]string{},
			overlay:  map[string]string{"x": "9", "y": "8"},
			expected: map[string]string{"x": "9", "y": "8"},
		},
		{
			name:     "merge without conflict",
			base:     map[string]string{"a": "1"},
			overlay:  map[string]string{"b": "2"},
			expected: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:     "overlay overrides base",
			base:     map[string]string{"a": "1", "b": "2"},
			overlay:  map[string]string{"b": "3", "c": "4"},
			expected: map[string]string{"a": "1", "b": "3", "c": "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.mergeLabels(tt.base, tt.overlay)

			if len(result) != len(tt.expected) {
				t.Errorf("result length = %d, want %d", len(result), len(tt.expected))
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("result[%q] = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestGetSeverity(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	tests := []struct {
		name     string
		labels   map[string]string
		expected Severity
	}{
		{
			name:     "critical severity",
			labels:   map[string]string{"severity": "critical"},
			expected: SeverityCritical,
		},
		{
			name:     "warning severity",
			labels:   map[string]string{"severity": "warning"},
			expected: SeverityWarning,
		},
		{
			name:     "info severity",
			labels:   map[string]string{"severity": "info"},
			expected: SeverityInfo,
		},
		{
			name:     "default severity (no label)",
			labels:   map[string]string{},
			expected: SeverityWarning,
		},
		{
			name:     "unknown severity defaults to warning",
			labels:   map[string]string{"severity": "unknown"},
			expected: SeverityWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &Rule{Labels: tt.labels}
			result := evaluator.getSeverity(rule)
			if result != tt.expected {
				t.Errorf("getSeverity() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateFingerprint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ruleID string
		labels map[string]string
	}{
		{
			name:   "simple fingerprint",
			ruleID: "rule1",
			labels: map[string]string{"host": "server1"},
		},
		{
			name:   "multiple labels",
			ruleID: "rule2",
			labels: map[string]string{"host": "server1", "service": "api", "env": "prod"},
		},
		{
			name:   "empty labels",
			ruleID: "rule3",
			labels: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp1 := generateFingerprint(tt.ruleID, tt.labels)
			fp2 := generateFingerprint(tt.ruleID, tt.labels)

			// Should be deterministic
			if fp1 != fp2 {
				t.Error("fingerprint should be deterministic")
			}

			// Should be non-empty
			if fp1 == "" {
				t.Error("fingerprint should not be empty")
			}

			// Should be hex string (MD5 = 32 hex chars)
			if len(fp1) != 32 {
				t.Errorf("fingerprint length = %d, want 32", len(fp1))
			}
		})
	}

	t.Run("different inputs produce different fingerprints", func(t *testing.T) {
		fp1 := generateFingerprint("rule1", map[string]string{"a": "1"})
		fp2 := generateFingerprint("rule2", map[string]string{"a": "1"})
		fp3 := generateFingerprint("rule1", map[string]string{"a": "2"})

		if fp1 == fp2 {
			t.Error("different rule IDs should produce different fingerprints")
		}
		if fp1 == fp3 {
			t.Error("different labels should produce different fingerprints")
		}
	})

	t.Run("label order does not affect fingerprint", func(t *testing.T) {
		labels1 := map[string]string{"a": "1", "b": "2", "c": "3"}
		labels2 := map[string]string{"c": "3", "a": "1", "b": "2"}

		fp1 := generateFingerprint("rule1", labels1)
		fp2 := generateFingerprint("rule1", labels2)

		if fp1 != fp2 {
			t.Error("label order should not affect fingerprint")
		}
	})
}

func TestSimpleMetricsProvider(t *testing.T) {
	t.Parallel()

	provider := NewSimpleMetricsProvider()

	t.Run("set and get metric", func(t *testing.T) {
		provider.SetMetric("cpu.usage", 45.5)

		value, err := provider.GetMetric("cpu.usage", nil)
		if err != nil {
			t.Fatalf("GetMetric failed: %v", err)
		}
		if value != 45.5 {
			t.Errorf("value = %f, want 45.5", value)
		}
	})

	t.Run("get non-existent metric", func(t *testing.T) {
		_, err := provider.GetMetric("nonexistent", nil)
		if err == nil {
			t.Error("expected error for non-existent metric")
		}
	})

	t.Run("query returns results", func(t *testing.T) {
		provider.SetMetric("memory.usage", 60.0)

		results, err := provider.Query("memory.usage")
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].Value != 60.0 {
			t.Errorf("value = %f, want 60.0", results[0].Value)
		}
	})

	t.Run("query non-existent metric returns nil", func(t *testing.T) {
		results, err := provider.Query("nonexistent")
		if err != nil {
			t.Fatalf("Query should not error: %v", err)
		}
		if results != nil {
			t.Error("expected nil results for non-existent metric")
		}
	})

	t.Run("concurrent access", func(t *testing.T) {
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(n int) {
				for j := 0; j < 100; j++ {
					provider.SetMetric("concurrent.metric", float64(n*100+j))
					provider.GetMetric("concurrent.metric", nil)
				}
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

func TestEvaluateThreshold(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	provider.SetMetric("test.metric", 95.0)

	tests := []struct {
		name           string
		rule           *Rule
		expectedFiring bool
	}{
		{
			name: "threshold exceeded",
			rule: &Rule{
				ID:        "rule1",
				Type:      RuleTypeThreshold,
				Query:     "test.metric",
				Condition: "gt",
				Threshold: 90.0,
			},
			expectedFiring: true,
		},
		{
			name: "threshold not exceeded",
			rule: &Rule{
				ID:        "rule2",
				Type:      RuleTypeThreshold,
				Query:     "test.metric",
				Condition: "gt",
				Threshold: 100.0,
			},
			expectedFiring: false,
		},
		{
			name: "metric name query",
			rule: &Rule{
				ID:        "rule3",
				Type:      RuleTypeThreshold,
				Metric:    "test.metric",
				Condition: "gte",
				Threshold: 95.0,
			},
			expectedFiring: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := evaluator.evaluateThreshold(tt.rule)
			if err != nil {
				t.Fatalf("evaluateThreshold failed: %v", err)
			}

			if len(results) == 0 {
				if tt.expectedFiring {
					t.Error("expected firing result, got none")
				}
				return
			}

			if results[0].Firing != tt.expectedFiring {
				t.Errorf("Firing = %v, want %v", results[0].Firing, tt.expectedFiring)
			}
		})
	}
}

func TestEvaluateAbsence(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	provider.SetMetric("existing.metric", 50.0)

	tests := []struct {
		name           string
		rule           *Rule
		expectedFiring bool
	}{
		{
			name: "metric exists - not firing",
			rule: &Rule{
				ID:     "rule1",
				Type:   RuleTypeAbsence,
				Metric: "existing.metric",
			},
			expectedFiring: false,
		},
		{
			name: "metric absent - firing",
			rule: &Rule{
				ID:     "rule2",
				Type:   RuleTypeAbsence,
				Metric: "nonexistent.metric",
			},
			expectedFiring: true,
		},
		{
			name: "query with results - not firing",
			rule: &Rule{
				ID:    "rule3",
				Type:  RuleTypeAbsence,
				Query: "existing.metric",
			},
			expectedFiring: false,
		},
		{
			name: "query without results - firing",
			rule: &Rule{
				ID:    "rule4",
				Type:  RuleTypeAbsence,
				Query: "nonexistent.query",
			},
			expectedFiring: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := evaluator.evaluateAbsence(tt.rule)
			if err != nil {
				t.Fatalf("evaluateAbsence failed: %v", err)
			}

			if len(results) == 0 {
				t.Fatal("expected at least one result")
			}

			if results[0].Firing != tt.expectedFiring {
				t.Errorf("Firing = %v, want %v", results[0].Firing, tt.expectedFiring)
			}
		})
	}
}

func TestAlertStates(t *testing.T) {
	t.Parallel()

	// Ensure alert state constants have expected values
	tests := []struct {
		state    AlertState
		expected string
	}{
		{StateInactive, "inactive"},
		{StatePending, "pending"},
		{StateFiring, "firing"},
		{StateResolved, "resolved"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("AlertState = %q, want %q", tt.state, tt.expected)
			}
		})
	}
}

func TestSeverityConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityCritical, "critical"},
		{SeverityWarning, "warning"},
		{SeverityInfo, "info"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			if string(tt.severity) != tt.expected {
				t.Errorf("Severity = %q, want %q", tt.severity, tt.expected)
			}
		})
	}
}

func TestRuleTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ruleType RuleType
		expected string
	}{
		{RuleTypeThreshold, "threshold"},
		{RuleTypeAnomaly, "anomaly"},
		{RuleTypeComposite, "composite"},
		{RuleTypeChange, "change"},
		{RuleTypeAbsence, "absence"},
	}

	for _, tt := range tests {
		t.Run(string(tt.ruleType), func(t *testing.T) {
			if string(tt.ruleType) != tt.expected {
				t.Errorf("RuleType = %q, want %q", tt.ruleType, tt.expected)
			}
		})
	}
}

func TestFingerprintSorting(t *testing.T) {
	t.Parallel()

	// Test that fingerprint generation handles label sorting correctly
	labels := map[string]string{
		"z": "last",
		"a": "first",
		"m": "middle",
	}

	// Generate fingerprint multiple times
	fingerprints := make([]string, 10)
	for i := 0; i < 10; i++ {
		fingerprints[i] = generateFingerprint("test", labels)
	}

	// All should be identical
	for i := 1; i < 10; i++ {
		if fingerprints[i] != fingerprints[0] {
			t.Error("fingerprints should be identical regardless of iteration")
		}
	}
}

func TestEvaluatorGetAlerts(t *testing.T) {
	t.Parallel()

	store := setupTestAlertStore(t)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	// Add some test alerts
	now := time.Now()
	firingAlert := &Alert{
		ID:          "alert1",
		Fingerprint: "fp1",
		State:       StateFiring,
		StartsAt:    now,
	}
	pendingAlert := &Alert{
		ID:          "alert2",
		Fingerprint: "fp2",
		State:       StatePending,
		StartsAt:    now,
	}

	evaluator.mu.Lock()
	evaluator.alerts["fp1"] = firingAlert
	evaluator.alerts["fp2"] = pendingAlert
	evaluator.mu.Unlock()

	t.Run("GetFiringAlerts", func(t *testing.T) {
		alerts := evaluator.GetFiringAlerts()
		if len(alerts) != 1 {
			t.Errorf("expected 1 firing alert, got %d", len(alerts))
		}
		if len(alerts) > 0 && alerts[0].ID != "alert1" {
			t.Errorf("expected alert1, got %s", alerts[0].ID)
		}
	})

	t.Run("GetPendingAlerts", func(t *testing.T) {
		alerts := evaluator.GetPendingAlerts()
		if len(alerts) != 1 {
			t.Errorf("expected 1 pending alert, got %d", len(alerts))
		}
		if len(alerts) > 0 && alerts[0].ID != "alert2" {
			t.Errorf("expected alert2, got %s", alerts[0].ID)
		}
	})

	t.Run("GetAlert by fingerprint", func(t *testing.T) {
		alert := evaluator.GetAlert("fp1")
		if alert == nil {
			t.Fatal("expected to find alert")
		}
		if alert.ID != "alert1" {
			t.Errorf("expected alert1, got %s", alert.ID)
		}
	})

	t.Run("GetAlert non-existent", func(t *testing.T) {
		alert := evaluator.GetAlert("nonexistent")
		if alert != nil {
			t.Error("expected nil for non-existent fingerprint")
		}
	})
}

// Helper to set up test alert store
func setupTestAlertStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "alerting_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// Benchmarks

func BenchmarkCheckCondition(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, _ := NewStore(dbPath)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.checkCondition(95.5, "gt", 90.0)
	}
}

func BenchmarkGenerateFingerprint(b *testing.B) {
	labels := map[string]string{
		"host":    "server1",
		"service": "api",
		"env":     "prod",
		"region":  "us-east-1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		generateFingerprint("rule1", labels)
	}
}

func BenchmarkMergeLabels(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	store, _ := NewStore(dbPath)
	defer store.Close()
	provider := NewSimpleMetricsProvider()
	evaluator := NewEvaluator(store, provider)

	base := map[string]string{"a": "1", "b": "2", "c": "3"}
	overlay := map[string]string{"d": "4", "e": "5", "c": "override"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		evaluator.mergeLabels(base, overlay)
	}
}

// Test to ensure label keys are sorted (for fingerprint consistency)
func TestLabelKeysSorted(t *testing.T) {
	labels := map[string]string{
		"zebra": "1",
		"apple": "2",
		"mango": "3",
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	expected := []string{"apple", "mango", "zebra"}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("sorted key %d = %s, want %s", i, k, expected[i])
		}
	}
}
