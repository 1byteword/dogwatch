package migration

import (
	"testing"
)

func TestExtractMonitorRefs(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []MonitorRef
	}{
		{
			name:  "single monitor",
			query: "monitors.123",
			expected: []MonitorRef{
				{Original: "monitors.123", ID: 123, Negated: false},
			},
		},
		{
			name:  "negated monitor",
			query: "!monitors.456",
			expected: []MonitorRef{
				{Original: "!monitors.456", ID: 456, Negated: true},
			},
		},
		{
			name:  "AND expression",
			query: "monitors.123 && monitors.456",
			expected: []MonitorRef{
				{Original: "monitors.123", ID: 123, Negated: false},
				{Original: "monitors.456", ID: 456, Negated: false},
			},
		},
		{
			name:  "OR expression",
			query: "monitors.123 || monitors.456",
			expected: []MonitorRef{
				{Original: "monitors.123", ID: 123, Negated: false},
				{Original: "monitors.456", ID: 456, Negated: false},
			},
		},
		{
			name:  "mixed expression",
			query: "(monitors.123 && monitors.456) || !monitors.789",
			expected: []MonitorRef{
				{Original: "monitors.123", ID: 123, Negated: false},
				{Original: "monitors.456", ID: 456, Negated: false},
				{Original: "!monitors.789", ID: 789, Negated: true},
			},
		},
		{
			name:  "bracket notation",
			query: "monitors[123] && monitors[456]",
			expected: []MonitorRef{
				{Original: "monitors[123]", ID: 123, Negated: false},
				{Original: "monitors[456]", ID: 456, Negated: false},
			},
		},
		{
			name:  "colon notation",
			query: "monitor:123 && monitor:456",
			expected: []MonitorRef{
				{Original: "monitor:123", ID: 123, Negated: false},
				{Original: "monitor:456", ID: 456, Negated: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := extractMonitorRefs(tt.query)

			if len(refs) != len(tt.expected) {
				t.Fatalf("expected %d refs, got %d", len(tt.expected), len(refs))
			}

			for i, ref := range refs {
				if ref.ID != tt.expected[i].ID {
					t.Errorf("ref %d: expected ID %d, got %d", i, tt.expected[i].ID, ref.ID)
				}
				if ref.Negated != tt.expected[i].Negated {
					t.Errorf("ref %d: expected Negated %v, got %v", i, tt.expected[i].Negated, ref.Negated)
				}
			}
		})
	}
}

func TestConvertCompositeExpression(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		substitutions map[string]string
		expected      string
	}{
		{
			name:  "simple AND",
			query: "monitors.123 && monitors.456",
			substitutions: map[string]string{
				"monitors.123": "rule-aaa",
				"monitors.456": "rule-bbb",
			},
			expected: "rule-aaa AND rule-bbb",
		},
		{
			name:  "simple OR",
			query: "monitors.123 || monitors.456",
			substitutions: map[string]string{
				"monitors.123": "rule-aaa",
				"monitors.456": "rule-bbb",
			},
			expected: "rule-aaa OR rule-bbb",
		},
		{
			name:  "with negation",
			query: "monitors.123 && !monitors.456",
			substitutions: map[string]string{
				"monitors.123": "rule-aaa",
				"monitors.456": "rule-bbb",
			},
			expected: "rule-aaa AND NOT rule-bbb",
		},
		{
			name:  "complex expression",
			query: "(monitors.123 && monitors.456) || monitors.789",
			substitutions: map[string]string{
				"monitors.123": "rule-aaa",
				"monitors.456": "rule-bbb",
				"monitors.789": "rule-ccc",
			},
			expected: "(rule-aaa AND rule-bbb) OR rule-ccc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertCompositeExpression(tt.query, tt.substitutions)

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCompositeQueryParser_ParseCompositeQuery(t *testing.T) {
	parser := NewCompositeQueryParser()

	// Register some monitor mappings
	parser.RegisterMonitorMapping(123, "rule-aaa")
	parser.RegisterMonitorMapping(456, "rule-bbb")

	tests := []struct {
		name            string
		query           string
		expectedExpr    string
		expectedSubRules int
		expectedUnresolved int
	}{
		{
			name:            "all resolved",
			query:           "monitors.123 && monitors.456",
			expectedExpr:    "rule-aaa AND rule-bbb",
			expectedSubRules: 2,
			expectedUnresolved: 0,
		},
		{
			name:            "partial resolution",
			query:           "monitors.123 && monitors.789",
			expectedExpr:    "rule-aaa AND unresolved_789",
			expectedSubRules: 1,
			expectedUnresolved: 1,
		},
		{
			name:            "with negation",
			query:           "!monitors.123 || monitors.456",
			expectedExpr:    "NOT rule-aaa OR rule-bbb",
			expectedSubRules: 2,
			expectedUnresolved: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseCompositeQuery(tt.query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Expression != tt.expectedExpr {
				t.Errorf("expected expression %q, got %q", tt.expectedExpr, result.Expression)
			}

			if len(result.SubRuleIDs) != tt.expectedSubRules {
				t.Errorf("expected %d sub-rules, got %d", tt.expectedSubRules, len(result.SubRuleIDs))
			}

			if len(result.UnresolvedRefs) != tt.expectedUnresolved {
				t.Errorf("expected %d unresolved, got %d", tt.expectedUnresolved, len(result.UnresolvedRefs))
			}
		})
	}
}

func TestCompositeMonitorContext_FullWorkflow(t *testing.T) {
	ctx := NewCompositeMonitorContext()

	// Simulate importing monitors
	monitor1 := &DatadogMonitor{ID: 100, Name: "CPU High", Type: "metric alert"}
	monitor2 := &DatadogMonitor{ID: 200, Name: "Memory High", Type: "metric alert"}
	monitor3 := &DatadogMonitor{
		ID:    300,
		Name:  "Combined Alert",
		Type:  "composite",
		Query: "monitors.100 && monitors.200",
	}

	// Register non-composite monitors
	ctx.RegisterMonitor(100, "rule-cpu")
	ctx.RegisterMonitor(200, "rule-mem")
	ctx.RegisterMonitor(300, "rule-combined")

	// Add composite monitor to pending
	ctx.AddPendingComposite(monitor3, "rule-combined")

	// Resolve
	resolutions := ctx.ResolvePendingComposites()

	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}

	resolution := resolutions[0]
	if !resolution.Success {
		t.Fatalf("resolution failed: %s", resolution.Error)
	}

	if resolution.Expression != "rule-cpu AND rule-mem" {
		t.Errorf("expected expression 'rule-cpu AND rule-mem', got %q", resolution.Expression)
	}

	if len(resolution.SubRuleIDs) != 2 {
		t.Errorf("expected 2 sub-rule IDs, got %d", len(resolution.SubRuleIDs))
	}

	// Verify order/content
	hasRuleCPU := false
	hasRuleMem := false
	for _, id := range resolution.SubRuleIDs {
		if id == "rule-cpu" {
			hasRuleCPU = true
		}
		if id == "rule-mem" {
			hasRuleMem = true
		}
	}

	if !hasRuleCPU || !hasRuleMem {
		t.Error("missing expected sub-rule IDs")
	}

	// Verify monitor1 and monitor2 are not nil (they were created)
	if monitor1 == nil || monitor2 == nil {
		t.Error("monitors should not be nil")
	}
}

func TestIsCompositeMonitor(t *testing.T) {
	tests := []struct {
		name     string
		monitor  *DatadogMonitor
		expected bool
	}{
		{
			name:     "explicit composite type",
			monitor:  &DatadogMonitor{Type: "composite", Query: "monitors.123"},
			expected: true,
		},
		{
			name:     "metric alert",
			monitor:  &DatadogMonitor{Type: "metric alert", Query: "avg:system.cpu.user{*}"},
			expected: false,
		},
		{
			name:     "query with monitor reference",
			monitor:  &DatadogMonitor{Type: "metric alert", Query: "monitors.123 && monitors.456"},
			expected: true,
		},
		{
			name:     "query with bracket notation",
			monitor:  &DatadogMonitor{Type: "query alert", Query: "monitors[123]"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsCompositeMonitor(tt.monitor)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractCompositeOperator(t *testing.T) {
	tests := []struct {
		query    string
		expected string
	}{
		{"monitors.123", "single"},
		{"monitors.123 && monitors.456", "and"},
		{"monitors.123 || monitors.456", "or"},
		{"monitors.123 && monitors.456 || monitors.789", "mixed"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			result := ExtractCompositeOperator(tt.query)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
