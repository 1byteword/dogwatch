package recording

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()

	if len(rules) != 3 {
		t.Errorf("Expected 3 default rules, got %d", len(rules))
	}

	// Check that all default rules have required fields
	for _, rule := range rules {
		if rule.ID == "" {
			t.Error("Default rule missing ID")
		}
		if rule.Name == "" {
			t.Error("Default rule missing Name")
		}
		if rule.Expression == "" {
			t.Error("Default rule missing Expression")
		}
		if rule.Interval == 0 {
			t.Error("Default rule missing Interval")
		}
		if !rule.Enabled {
			t.Errorf("Default rule %s should be enabled", rule.Name)
		}
	}
}

func TestStore(t *testing.T) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "recording_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test CreateRule
	rule := &RecordingRule{
		ID:          "test-rule-1",
		Name:        "test:metric:1m",
		Expression:  "SELECT count(*) as value FROM metrics",
		Interval:    time.Minute,
		Labels:      map[string]string{"env": "test"},
		Enabled:     true,
		Description: "Test rule",
	}

	if err := store.CreateRule(rule); err != nil {
		t.Fatalf("Failed to create rule: %v", err)
	}

	// Test GetRule
	retrieved, err := store.GetRule("test-rule-1")
	if err != nil {
		t.Fatalf("Failed to get rule: %v", err)
	}

	if retrieved.Name != rule.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, rule.Name)
	}
	if retrieved.Expression != rule.Expression {
		t.Errorf("Expression mismatch: got %s, want %s", retrieved.Expression, rule.Expression)
	}
	if retrieved.Interval != rule.Interval {
		t.Errorf("Interval mismatch: got %v, want %v", retrieved.Interval, rule.Interval)
	}
	if retrieved.Labels["env"] != "test" {
		t.Errorf("Labels mismatch: got %v, want env=test", retrieved.Labels)
	}

	// Test GetRuleByName
	byName, err := store.GetRuleByName("test:metric:1m")
	if err != nil {
		t.Fatalf("Failed to get rule by name: %v", err)
	}
	if byName.ID != "test-rule-1" {
		t.Errorf("GetByName returned wrong rule: got %s, want test-rule-1", byName.ID)
	}

	// Test ListRules
	rules, err := store.ListRules()
	if err != nil {
		t.Fatalf("Failed to list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}

	// Test UpdateRule
	rule.Description = "Updated description"
	if err := store.UpdateRule(rule); err != nil {
		t.Fatalf("Failed to update rule: %v", err)
	}

	updated, _ := store.GetRule("test-rule-1")
	if updated.Description != "Updated description" {
		t.Errorf("Description not updated: got %s", updated.Description)
	}

	// Test UpdateEvaluation
	if err := store.UpdateEvaluation("test-rule-1", 42.0, nil); err != nil {
		t.Fatalf("Failed to update evaluation: %v", err)
	}

	evaluated, _ := store.GetRule("test-rule-1")
	if evaluated.LastValue != 42.0 {
		t.Errorf("LastValue not updated: got %f, want 42.0", evaluated.LastValue)
	}
	if evaluated.LastError != "" {
		t.Errorf("LastError should be empty: got %s", evaluated.LastError)
	}

	// Test ListEnabledRules
	enabled, err := store.ListEnabledRules()
	if err != nil {
		t.Fatalf("Failed to list enabled rules: %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("Expected 1 enabled rule, got %d", len(enabled))
	}

	// Disable the rule
	rule.Enabled = false
	store.UpdateRule(rule)

	enabled, _ = store.ListEnabledRules()
	if len(enabled) != 0 {
		t.Errorf("Expected 0 enabled rules, got %d", len(enabled))
	}

	// Test DeleteRule
	if err := store.DeleteRule("test-rule-1"); err != nil {
		t.Fatalf("Failed to delete rule: %v", err)
	}

	_, err = store.GetRule("test-rule-1")
	if err == nil {
		t.Error("Expected error getting deleted rule")
	}
}

func TestEvaluationHistory(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "recording_history_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create a rule first
	rule := &RecordingRule{
		ID:         "test-rule",
		Name:       "test:metric",
		Expression: "SELECT 1",
		Interval:   time.Minute,
		Enabled:    true,
	}
	store.CreateRule(rule)

	// Record some evaluations
	for i := 0; i < 5; i++ {
		history := &EvaluationHistory{
			ID:        "eval-" + string(rune('a'+i)),
			RuleID:    "test-rule",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Value:     float64(i * 10),
			Duration:  100,
			Success:   true,
		}
		if err := store.RecordEvaluation(history); err != nil {
			t.Fatalf("Failed to record evaluation: %v", err)
		}
	}

	// Test GetEvaluationHistory
	history, err := store.GetEvaluationHistory("test-rule", 10)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}
	if len(history) != 5 {
		t.Errorf("Expected 5 history records, got %d", len(history))
	}

	// Verify order (should be newest first)
	if history[0].Value != 0 {
		t.Errorf("Expected newest record first (value=0), got value=%f", history[0].Value)
	}

	// Test limit
	limited, _ := store.GetEvaluationHistory("test-rule", 3)
	if len(limited) != 3 {
		t.Errorf("Expected 3 history records with limit, got %d", len(limited))
	}

	// Test CleanupHistory
	deleted, err := store.CleanupHistory(30 * time.Second)
	if err != nil {
		t.Fatalf("Failed to cleanup history: %v", err)
	}
	// All records should be older than 30 seconds except the first one
	if deleted < 4 {
		t.Errorf("Expected at least 4 deleted records, got %d", deleted)
	}
}

func TestEnsureDefaultRules(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "recording_defaults_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Ensure defaults
	if err := store.EnsureDefaultRules(); err != nil {
		t.Fatalf("Failed to ensure default rules: %v", err)
	}

	rules, _ := store.ListRules()
	if len(rules) != 3 {
		t.Errorf("Expected 3 default rules, got %d", len(rules))
	}

	// Call again - should not create duplicates
	if err := store.EnsureDefaultRules(); err != nil {
		t.Fatalf("Failed second ensure: %v", err)
	}

	rules, _ = store.ListRules()
	if len(rules) != 3 {
		t.Errorf("Expected still 3 rules after second ensure, got %d", len(rules))
	}
}

func TestIsPromQLStyle(t *testing.T) {
	tests := []struct {
		expr     string
		expected bool
	}{
		{"rate(http_requests_total[5m])", true},
		{"sum(request_count) by (service)", true},
		{"avg(latency) by (endpoint)", true},
		{"count(errors)", true},
		{"histogram_quantile(0.99, request_latency)", true},
		{"SELECT count(*) FROM traces", false},
		{"SELECT service, avg(duration) FROM spans GROUP BY service", false},
		{"some_metric > 100", false},
	}

	for _, tt := range tests {
		result := isPromQLStyle(tt.expr)
		if result != tt.expected {
			t.Errorf("isPromQLStyle(%q) = %v, want %v", tt.expr, result, tt.expected)
		}
	}
}

func TestDurationToSeconds(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"30s", "30"},
		{"5m", "300"},
		{"1h", "3600"},
		{"1d", "86400"},
		{"invalid", "60"},
	}

	for _, tt := range tests {
		result := durationToSeconds(tt.input)
		if result != tt.expected {
			t.Errorf("durationToSeconds(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected float64
	}{
		{42.5, 42.5},
		{float32(42.5), 42.5},
		{42, 42.0},
		{int64(42), 42.0},
		{int32(42), 42.0},
		{"42.5", 42.5},
		{"invalid", 0.0},
		{nil, 0.0},
	}

	for _, tt := range tests {
		result := toFloat(tt.input)
		if result != tt.expected {
			t.Errorf("toFloat(%v) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestEvaluatorWithoutExecutor(t *testing.T) {
	evaluator := NewEvaluator(nil, nil)

	rule := &RecordingRule{
		ID:         "test",
		Name:       "test:rule",
		Expression: "SELECT 1",
		Interval:   time.Minute,
	}

	result := evaluator.Evaluate(context.Background(), rule)
	if result.Error == nil {
		t.Error("Expected error when executor is nil")
	}
}

func TestManagerStats(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "recording_manager_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	evaluator := NewEvaluator(nil, nil)
	manager := NewManager(store, evaluator, DefaultConfig())

	stats := manager.GetStats()
	if stats.TotalRules != 3 {
		t.Errorf("Expected 3 total rules (defaults), got %d", stats.TotalRules)
	}
	if stats.EnabledRules != 3 {
		t.Errorf("Expected 3 enabled rules (defaults), got %d", stats.EnabledRules)
	}
	if stats.TotalEvaluations != 0 {
		t.Errorf("Expected 0 evaluations, got %d", stats.TotalEvaluations)
	}
}

func TestManagerStartStop(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "recording_startstop_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	evaluator := NewEvaluator(nil, nil)
	config := DefaultConfig()
	config.EvalInterval = 100 * time.Millisecond // Fast for testing
	manager := NewManager(store, evaluator, config)

	// Test initial state
	if manager.IsRunning() {
		t.Error("Manager should not be running initially")
	}

	// Test Start
	manager.Start()
	if !manager.IsRunning() {
		t.Error("Manager should be running after Start")
	}

	// Double start should be safe
	manager.Start()
	if !manager.IsRunning() {
		t.Error("Manager should still be running after double Start")
	}

	// Let it run briefly
	time.Sleep(50 * time.Millisecond)

	// Test Stop
	manager.Stop()
	if manager.IsRunning() {
		t.Error("Manager should not be running after Stop")
	}

	// Double stop should be safe
	manager.Stop()
	if manager.IsRunning() {
		t.Error("Manager should still not be running after double Stop")
	}
}
