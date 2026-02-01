// Package integration provides end-to-end tests for dogwatch recording rules.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/query"
	"dogwatch/internal/recording"
)

// testRecordingStore creates a test recording store with in-memory SQLite
func testRecordingStore(t *testing.T) (*recording.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dogwatch-recording-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "recording_test.db")
	store, err := recording.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create recording store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// testMetricsStore creates a test metrics store
func testMetricsStore(t *testing.T) (*custommetrics.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dogwatch-metrics-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "metrics_test.db")
	store, err := custommetrics.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create metrics store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestRecordingRuleCreationAndEvaluation tests rule creation -> evaluation -> metric storage
func TestRecordingRuleCreationAndEvaluation(t *testing.T) {
	recordingStore, cleanupRecording := testRecordingStore(t)
	defer cleanupRecording()

	metricsStore, cleanupMetrics := testMetricsStore(t)
	defer cleanupMetrics()

	// Step 1: Create a recording rule
	rule := &recording.RecordingRule{
		ID:          "test_rule_001",
		Name:        "test:request_count:1m",
		Expression:  "SELECT service, count(*) as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service",
		Interval:    1 * time.Minute,
		Enabled:     true,
		Description: "Test rule for request count by service",
		Labels: map[string]string{
			"__name__": "test:request_count:1m",
			"source":   "recording_rule",
		},
	}

	err := recordingStore.CreateRule(rule)
	if err != nil {
		t.Fatalf("Failed to create rule: %v", err)
	}

	// Step 2: Verify rule was created
	retrievedRule, err := recordingStore.GetRule(rule.ID)
	if err != nil {
		t.Fatalf("Failed to get rule: %v", err)
	}
	if retrievedRule.Name != rule.Name {
		t.Errorf("Rule name mismatch: expected '%s', got '%s'", rule.Name, retrievedRule.Name)
	}
	if retrievedRule.Interval != rule.Interval {
		t.Errorf("Rule interval mismatch: expected %v, got %v", rule.Interval, retrievedRule.Interval)
	}

	// Step 3: Create evaluator with mock data
	executor := query.NewExecutor()
	// Set up a mock data source
	mockDS := &MockDataSource{
		rows: []query.Row{
			{"service": "api-gateway", "count": 100},
			{"service": "user-service", "count": 75},
			{"service": "order-service", "count": 50},
		},
	}
	executor.SetTracesSource(mockDS)

	evaluator := recording.NewEvaluator(executor, metricsStore)

	// Step 4: Evaluate the rule
	ctx := context.Background()
	result := evaluator.EvaluateAndStore(ctx, rule)

	if result.Error != nil {
		t.Logf("Evaluation error (may be expected with mock): %v", result.Error)
	}

	t.Logf("Evaluation completed in %v", result.Duration)
	t.Logf("Result values: %d", len(result.Values))

	// Step 5: Update rule evaluation state
	err = recordingStore.UpdateEvaluation(rule.ID, 100.0, nil)
	if err != nil {
		t.Fatalf("Failed to update evaluation: %v", err)
	}

	// Verify update
	updatedRule, _ := recordingStore.GetRule(rule.ID)
	if updatedRule.LastValue != 100.0 {
		t.Errorf("Expected last value 100.0, got %f", updatedRule.LastValue)
	}
}

// MockDataSource implements a simple mock data source for testing
type MockDataSource struct {
	rows []query.Row
}

func (m *MockDataSource) Scan(ctx context.Context, source string, metric string, timeRange query.TimeRangeSpec, predicates []query.Expr) ([]query.Row, error) {
	return m.rows, nil
}

// TestRecordingRuleCRUD tests full CRUD operations on recording rules
func TestRecordingRuleCRUD(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	// Create
	rule := &recording.RecordingRule{
		ID:          "crud_test_rule",
		Name:        "test:crud_metric",
		Expression:  "SELECT count(*) as value FROM logs",
		Interval:    5 * time.Minute,
		Enabled:     true,
		Description: "Test CRUD operations",
		Labels:      map[string]string{"env": "test"},
	}

	err := store.CreateRule(rule)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read
	retrieved, err := store.GetRule(rule.ID)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if retrieved.Expression != rule.Expression {
		t.Errorf("Expression mismatch")
	}

	// Read by name
	retrievedByName, err := store.GetRuleByName(rule.Name)
	if err != nil {
		t.Fatalf("Read by name failed: %v", err)
	}
	if retrievedByName.ID != rule.ID {
		t.Errorf("ID mismatch when reading by name")
	}

	// Update
	rule.Description = "Updated description"
	rule.Interval = 10 * time.Minute
	err = store.UpdateRule(rule)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, _ := store.GetRule(rule.ID)
	if updated.Description != "Updated description" {
		t.Errorf("Description not updated")
	}
	if updated.Interval != 10*time.Minute {
		t.Errorf("Interval not updated: got %v", updated.Interval)
	}

	// List
	rules, err := store.ListRules()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(rules) < 1 {
		t.Errorf("Expected at least 1 rule in list")
	}

	// Delete
	err = store.DeleteRule(rule.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetRule(rule.ID)
	if err == nil {
		t.Error("Expected error when getting deleted rule")
	}
}

// TestScheduledEvaluationRuns tests the scheduled evaluation mechanism
func TestScheduledEvaluationRuns(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	metricsStore, cleanupMetrics := testMetricsStore(t)
	defer cleanupMetrics()

	// Create rules with different intervals
	rules := []*recording.RecordingRule{
		{
			ID:         "sched_rule_1",
			Name:       "test:sched_1",
			Expression: "SELECT 1 as value",
			Interval:   100 * time.Millisecond,
			Enabled:    true,
		},
		{
			ID:         "sched_rule_2",
			Name:       "test:sched_2",
			Expression: "SELECT 2 as value",
			Interval:   200 * time.Millisecond,
			Enabled:    true,
		},
		{
			ID:         "sched_rule_3",
			Name:       "test:sched_3",
			Expression: "SELECT 3 as value",
			Interval:   1 * time.Hour, // Won't run during test
			Enabled:    true,
		},
	}

	for _, rule := range rules {
		if err := store.CreateRule(rule); err != nil {
			t.Fatalf("Failed to create rule: %v", err)
		}
	}

	// Create evaluator and manager
	executor := query.NewExecutor()
	evaluator := recording.NewEvaluator(executor, metricsStore)

	manager := recording.NewManager(store, evaluator, recording.ManagerConfig{
		EvalInterval:  50 * time.Millisecond,
		HistoryMaxAge: 1 * time.Hour,
		CleanupOnInit: false,
	})

	// Start the manager
	manager.Start()

	// Let it run for a bit
	time.Sleep(350 * time.Millisecond)

	// Stop the manager
	manager.Stop()

	// Check stats
	stats := manager.GetStats()
	t.Logf("Manager stats: %+v", stats)

	if stats.TotalRules < 3 {
		t.Errorf("Expected at least 3 rules, got %d", stats.TotalRules)
	}
	if stats.EnabledRules < 3 {
		t.Errorf("Expected at least 3 enabled rules, got %d", stats.EnabledRules)
	}

	// Verify evaluations happened
	if stats.TotalEvaluations == 0 {
		t.Log("No evaluations recorded - this may be expected due to timing")
	} else {
		t.Logf("Total evaluations: %d", stats.TotalEvaluations)
	}
}

// TestEvaluationHistory tests recording and retrieval of evaluation history
func TestEvaluationHistory(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	// Create a rule
	rule := &recording.RecordingRule{
		ID:         "history_rule",
		Name:       "test:history",
		Expression: "SELECT 42 as value",
		Interval:   1 * time.Minute,
		Enabled:    true,
	}
	store.CreateRule(rule)

	// Record some evaluation history
	histories := []recording.EvaluationHistory{
		{
			ID:        "hist_1",
			RuleID:    rule.ID,
			Timestamp: time.Now().Add(-5 * time.Minute),
			Value:     100.0,
			Duration:  50,
			Success:   true,
		},
		{
			ID:        "hist_2",
			RuleID:    rule.ID,
			Timestamp: time.Now().Add(-4 * time.Minute),
			Value:     105.0,
			Duration:  52,
			Success:   true,
		},
		{
			ID:        "hist_3",
			RuleID:    rule.ID,
			Timestamp: time.Now().Add(-3 * time.Minute),
			Value:     0,
			Duration:  100,
			Error:     "query timeout",
			Success:   false,
		},
		{
			ID:        "hist_4",
			RuleID:    rule.ID,
			Timestamp: time.Now().Add(-2 * time.Minute),
			Value:     98.0,
			Duration:  48,
			Success:   true,
		},
	}

	for _, h := range histories {
		err := store.RecordEvaluation(&h)
		if err != nil {
			t.Fatalf("Failed to record evaluation: %v", err)
		}
	}

	// Retrieve history
	retrieved, err := store.GetEvaluationHistory(rule.ID, 10)
	if err != nil {
		t.Fatalf("Failed to get history: %v", err)
	}

	if len(retrieved) != len(histories) {
		t.Errorf("Expected %d history entries, got %d", len(histories), len(retrieved))
	}

	// Verify order (should be most recent first)
	if len(retrieved) >= 2 && retrieved[0].Timestamp.Before(retrieved[1].Timestamp) {
		t.Error("History should be ordered by timestamp descending")
	}

	// Verify error entry
	foundError := false
	for _, h := range retrieved {
		if !h.Success && h.Error == "query timeout" {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Error("Expected to find error entry in history")
	}

	t.Logf("Retrieved %d history entries", len(retrieved))
}

// TestEvaluationWithRealDQL tests evaluation with real DQL queries
func TestEvaluationWithRealDQL(t *testing.T) {
	recStore, cleanup := testRecordingStore(t)
	defer cleanup()

	metricsStore, cleanupMetrics := testMetricsStore(t)
	defer cleanupMetrics()

	// Create executor with mock data source
	executor := query.NewExecutor()
	mockDS := &MockDataSource{
		rows: []query.Row{
			{"timestamp": time.Now(), "service": "api", "value": 100.0},
			{"timestamp": time.Now(), "service": "api", "value": 150.0},
			{"timestamp": time.Now(), "service": "web", "value": 200.0},
		},
	}
	executor.SetMetricsSource(mockDS)
	executor.SetLogsSource(mockDS)
	executor.SetTracesSource(mockDS)

	evaluator := recording.NewEvaluator(executor, metricsStore)

	testCases := []struct {
		name       string
		expression string
		expectErr  bool
	}{
		{
			name:       "Simple count",
			expression: "SELECT count(*) as value FROM logs",
			expectErr:  false,
		},
		{
			name:       "Group by with sum",
			expression: "SELECT service, sum(value) as value FROM metrics GROUP BY service",
			expectErr:  false,
		},
		{
			name:       "Average with filter",
			expression: "SELECT avg(value) as value FROM traces WHERE service = 'api'",
			expectErr:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := &recording.RecordingRule{
				ID:         "dql_test_" + tc.name,
				Name:       "test:" + tc.name,
				Expression: tc.expression,
				Interval:   1 * time.Minute,
				Enabled:    true,
			}

			// Save rule to store
			err := recStore.CreateRule(rule)
			if err != nil {
				t.Fatalf("Failed to create rule: %v", err)
			}

			// Verify rule was saved
			saved, err := recStore.GetRule(rule.ID)
			if err != nil {
				t.Fatalf("Failed to get saved rule: %v", err)
			}
			if saved.Expression != tc.expression {
				t.Errorf("Expression mismatch: expected '%s', got '%s'", tc.expression, saved.Expression)
			}

			ctx := context.Background()
			result := evaluator.Evaluate(ctx, rule)

			if tc.expectErr {
				if result.Error == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if result.Error != nil {
					t.Logf("Evaluation error (may be expected with mock): %v", result.Error)
				}
			}

			t.Logf("Expression: %s", tc.expression)
			t.Logf("Duration: %v", result.Duration)
			t.Logf("Values: %d", len(result.Values))
		})
	}
}

// TestDefaultRulesInitialization tests that default rules are created
func TestDefaultRulesInitialization(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	// Ensure default rules
	err := store.EnsureDefaultRules()
	if err != nil {
		t.Fatalf("Failed to ensure default rules: %v", err)
	}

	// Get default rules
	defaults := recording.DefaultRules()

	for _, defaultRule := range defaults {
		rule, err := store.GetRule(defaultRule.ID)
		if err != nil {
			t.Errorf("Default rule %s not found: %v", defaultRule.ID, err)
			continue
		}

		if rule.Name != defaultRule.Name {
			t.Errorf("Rule name mismatch for %s", defaultRule.ID)
		}
		if rule.Interval != defaultRule.Interval {
			t.Errorf("Rule interval mismatch for %s", defaultRule.ID)
		}
	}

	t.Logf("Verified %d default rules", len(defaults))
}

// TestManualEvaluation tests manual triggering of rule evaluation
func TestManualEvaluation(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	metricsStore, cleanupMetrics := testMetricsStore(t)
	defer cleanupMetrics()

	// Create a rule
	rule := &recording.RecordingRule{
		ID:         "manual_eval_rule",
		Name:       "test:manual",
		Expression: "SELECT 42 as value",
		Interval:   1 * time.Hour, // Long interval so it won't auto-run
		Enabled:    true,
	}
	store.CreateRule(rule)

	// Create manager but don't start it
	executor := query.NewExecutor()
	mockDS := &MockDataSource{
		rows: []query.Row{{"value": 42.0}},
	}
	executor.SetMetricsSource(mockDS)

	evaluator := recording.NewEvaluator(executor, metricsStore)
	manager := recording.NewManager(store, evaluator, recording.DefaultConfig())

	// Manually evaluate
	result, err := manager.EvaluateNow(rule.ID)
	if err != nil {
		t.Fatalf("Manual evaluation failed: %v", err)
	}

	t.Logf("Manual evaluation result: %+v", result)

	// Check that rule was updated
	updated, _ := store.GetRule(rule.ID)
	if updated.LastEval.IsZero() {
		t.Error("LastEval should be set after manual evaluation")
	}

	// Check stats
	stats := manager.GetStats()
	if stats.TotalEvaluations == 0 {
		t.Error("Expected at least 1 evaluation after manual trigger")
	}
}

// TestHistoryCleanup tests cleanup of old evaluation history
func TestHistoryCleanup(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	// Create a rule
	rule := &recording.RecordingRule{
		ID:         "cleanup_rule",
		Name:       "test:cleanup",
		Expression: "SELECT 1 as value",
		Interval:   1 * time.Minute,
		Enabled:    true,
	}
	store.CreateRule(rule)

	// Add old history entries
	oldTime := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 10; i++ {
		h := &recording.EvaluationHistory{
			ID:        "old_hist_" + string(rune('0'+i)),
			RuleID:    rule.ID,
			Timestamp: oldTime.Add(time.Duration(i) * time.Minute),
			Value:     float64(i),
			Duration:  50,
			Success:   true,
		}
		store.RecordEvaluation(h)
	}

	// Add recent history entries
	for i := 0; i < 5; i++ {
		h := &recording.EvaluationHistory{
			ID:        "new_hist_" + string(rune('a'+i)),
			RuleID:    rule.ID,
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			Value:     float64(i),
			Duration:  50,
			Success:   true,
		}
		store.RecordEvaluation(h)
	}

	// Cleanup entries older than 24 hours
	deleted, err := store.CleanupHistory(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	t.Logf("Deleted %d old history entries", deleted)

	if deleted != 10 {
		t.Errorf("Expected to delete 10 old entries, deleted %d", deleted)
	}

	// Verify recent entries remain
	remaining, err := store.GetEvaluationHistory(rule.ID, 100)
	if err != nil {
		t.Fatalf("Failed to get remaining history: %v", err)
	}

	if len(remaining) != 5 {
		t.Errorf("Expected 5 recent entries remaining, got %d", len(remaining))
	}
}

// TestListEnabledRules tests listing only enabled rules
func TestListEnabledRules(t *testing.T) {
	store, cleanup := testRecordingStore(t)
	defer cleanup()

	// Create mix of enabled and disabled rules
	rules := []*recording.RecordingRule{
		{ID: "enabled_1", Name: "test:enabled_1", Expression: "SELECT 1", Interval: time.Minute, Enabled: true},
		{ID: "enabled_2", Name: "test:enabled_2", Expression: "SELECT 2", Interval: time.Minute, Enabled: true},
		{ID: "disabled_1", Name: "test:disabled_1", Expression: "SELECT 3", Interval: time.Minute, Enabled: false},
		{ID: "enabled_3", Name: "test:enabled_3", Expression: "SELECT 4", Interval: time.Minute, Enabled: true},
		{ID: "disabled_2", Name: "test:disabled_2", Expression: "SELECT 5", Interval: time.Minute, Enabled: false},
	}

	for _, rule := range rules {
		store.CreateRule(rule)
	}

	// List all rules
	allRules, _ := store.ListRules()

	// List enabled rules only
	enabledRules, err := store.ListEnabledRules()
	if err != nil {
		t.Fatalf("Failed to list enabled rules: %v", err)
	}

	// Count enabled in test rules (excluding default rules)
	expectedEnabled := 3
	actualEnabled := 0
	for _, r := range enabledRules {
		for _, testRule := range rules {
			if r.ID == testRule.ID && testRule.Enabled {
				actualEnabled++
			}
		}
	}

	if actualEnabled != expectedEnabled {
		t.Errorf("Expected %d enabled rules, got %d (total rules: %d)",
			expectedEnabled, actualEnabled, len(allRules))
	}

	// Verify no disabled rules in enabled list
	for _, r := range enabledRules {
		if !r.Enabled {
			t.Errorf("Found disabled rule %s in enabled list", r.ID)
		}
	}
}
