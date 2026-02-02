package logs

import (
	"strings"
	"testing"
)

func TestPatternLearner_ComputeSignature(t *testing.T) {
	pl := NewPatternLearner(DefaultPatternLearnerConfig())

	tests := []struct {
		name     string
		messages []string
		shouldMatch bool
	}{
		{
			name: "similar messages with different UUIDs",
			messages: []string{
				"Request 550e8400-e29b-41d4-a716-446655440000 completed",
				"Request 550e8400-e29b-41d4-a716-446655440001 completed",
			},
			shouldMatch: true,
		},
		{
			name: "similar messages with different IPs",
			messages: []string{
				"Connection from 192.168.1.1 accepted",
				"Connection from 10.0.0.5 accepted",
			},
			shouldMatch: true,
		},
		{
			name: "similar messages with different numbers",
			messages: []string{
				"Processed 100 items in 50ms",
				"Processed 200 items in 75ms",
			},
			shouldMatch: true,
		},
		{
			name: "different messages",
			messages: []string{
				"User logged in successfully",
				"Connection timeout error",
			},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sigs := make([]string, len(tt.messages))
			for i, msg := range tt.messages {
				sigs[i] = pl.computeSignature(msg)
			}

			allMatch := true
			for i := 1; i < len(sigs); i++ {
				if sigs[i] != sigs[0] {
					allMatch = false
					break
				}
			}

			if allMatch != tt.shouldMatch {
				t.Errorf("Expected signatures to match: %v, got %v", tt.shouldMatch, allMatch)
				for i, sig := range sigs {
					t.Logf("Message %d: %s -> %s", i, tt.messages[i], sig)
				}
			}
		})
	}
}

func TestPatternLearner_RecordUnmatched(t *testing.T) {
	config := DefaultPatternLearnerConfig()
	config.MinSamplesForSuggestion = 5
	pl := NewPatternLearner(config)

	// Record similar messages
	for i := 0; i < 10; i++ {
		pl.RecordUnmatched("Error processing request 123 from user test", "api-service")
	}

	stats := pl.GetStats()
	if stats.UnmatchedLogsCount != 10 {
		t.Errorf("Expected 10 unmatched logs, got %d", stats.UnmatchedLogsCount)
	}

	// Should have generated a candidate
	candidates := pl.GetCandidates()
	if len(candidates) != 1 {
		t.Errorf("Expected 1 candidate, got %d", len(candidates))
	}

	// Candidate should have a pattern after enough samples
	if len(candidates) > 0 && candidates[0].Pattern == "" {
		t.Error("Expected candidate to have a generated pattern")
	}
}

func TestPatternLearner_GetSuggestions(t *testing.T) {
	config := DefaultPatternLearnerConfig()
	config.MinSamplesForSuggestion = 3
	pl := NewPatternLearner(config)

	// Record messages with a consistent pattern
	messages := []string{
		"User abc123 logged in from 192.168.1.1",
		"User def456 logged in from 10.0.0.1",
		"User ghi789 logged in from 172.16.0.1",
		"User jkl012 logged in from 192.168.2.1",
		"User mno345 logged in from 10.10.10.1",
	}

	for _, msg := range messages {
		pl.RecordUnmatched(msg, "auth-service")
	}

	// Get suggestions
	suggestions := pl.GetSuggestions(0.3)

	if len(suggestions) == 0 {
		t.Log("No suggestions generated - may need more samples")
	} else {
		t.Logf("Generated %d suggestions", len(suggestions))
		for i, s := range suggestions {
			t.Logf("Suggestion %d: confidence=%.2f, fields=%v", i, s.Confidence, s.FieldsDetected)
		}
	}
}

func TestPatternLearner_AnalyzeLogStructure(t *testing.T) {
	pl := NewPatternLearner(DefaultPatternLearnerConfig())

	tests := []struct {
		message      string
		expectedType string
	}{
		{
			message:      `{"level":"info","msg":"Request processed","duration":123}`,
			expectedType: "json",
		},
		{
			message:      `level=info msg="Request processed" duration=123ms`,
			expectedType: "key-value",
		},
		{
			message:      `<165>Oct 12 12:34:56 myhost myapp[1234]: Message here`,
			expectedType: "syslog",
		},
		{
			message:      `This is a plain log message without structure`,
			expectedType: "unstructured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expectedType, func(t *testing.T) {
			analysis := pl.AnalyzeLogStructure(tt.message)

			if analysis.StructureType != tt.expectedType {
				t.Errorf("Expected structure type %s, got %s", tt.expectedType, analysis.StructureType)
			}
		})
	}
}

func TestPatternLearner_DetectPotentialFields(t *testing.T) {
	pl := NewPatternLearner(DefaultPatternLearnerConfig())

	tests := []struct {
		message  string
		expected []string
	}{
		{
			message:  "Request from 192.168.1.1 completed",
			expected: []string{"ip_address"},
		},
		{
			message:  "User user@example.com logged in",
			expected: []string{"email"},
		},
		{
			message:  "Trace ID: 550e8400-e29b-41d4-a716-446655440000",
			expected: []string{"uuid"},
		},
		{
			message:  "key1=value1 key2=value2",
			expected: []string{"key1", "key2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.message[:20]+"...", func(t *testing.T) {
			fields := pl.detectPotentialFields(tt.message)

			for _, exp := range tt.expected {
				found := false
				for _, f := range fields {
					if f == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected field %s not found in %v", exp, fields)
				}
			}
		})
	}
}

func TestPatternLearner_GeneratePattern(t *testing.T) {
	pl := NewPatternLearner(DefaultPatternLearnerConfig())

	samples := []string{
		"Request 550e8400-e29b-41d4-a716-446655440000 from 192.168.1.1 completed in 100ms",
		"Request 550e8400-e29b-41d4-a716-446655440001 from 10.0.0.1 completed in 200ms",
	}

	pattern := pl.generatePattern(samples)

	if pattern == "" {
		t.Fatal("Expected pattern to be generated")
	}

	// Pattern should contain capture groups
	if !strings.Contains(pattern, "?P<") {
		t.Error("Expected pattern to contain named capture groups")
	}

	t.Logf("Generated pattern: %s", pattern)
}

func TestUnmatchedLogBuffer(t *testing.T) {
	buffer := newUnmatchedLogBuffer(5)

	// Add more logs than buffer size
	for i := 0; i < 10; i++ {
		buffer.add(UnmatchedLog{
			Message:   "Log message " + string(rune('0'+i)),
			Source:    "test",
		})
	}

	// Get recent logs
	recent := buffer.getRecent(5)

	// Should only get 5 (buffer size)
	if len(recent) != 5 {
		t.Errorf("Expected 5 recent logs, got %d", len(recent))
	}

	// Most recent should be "Log message 9"
	if len(recent) > 0 && recent[0].Message != "Log message 9" {
		t.Errorf("Expected most recent to be 'Log message 9', got %s", recent[0].Message)
	}
}

func TestPatternLearner_CandidateEviction(t *testing.T) {
	config := DefaultPatternLearnerConfig()
	config.MaxCandidates = 3
	pl := NewPatternLearner(config)

	// Add different patterns (different signatures)
	messages := []string{
		"Type A: message 1",
		"Type B: message 2",
		"Type C: message 3",
		"Type D: message 4",
	}

	for _, msg := range messages {
		pl.RecordUnmatched(msg, "test")
	}

	candidates := pl.GetCandidates()
	if len(candidates) > 3 {
		t.Errorf("Expected max 3 candidates, got %d", len(candidates))
	}
}

func TestPatternLearner_CalculateConfidence(t *testing.T) {
	pl := NewPatternLearner(DefaultPatternLearnerConfig())

	// Low sample count = low confidence
	lowCandidate := &PatternCandidate{
		SampleCount:    10,
		Pattern:        "^test$",
		FieldsDetected: []string{"field1"},
	}
	lowConfidence := pl.calculateConfidence(lowCandidate)

	// High sample count = higher confidence
	highCandidate := &PatternCandidate{
		SampleCount:    1000,
		Pattern:        "^test$",
		FieldsDetected: []string{"field1", "field2", "field3"},
	}
	highConfidence := pl.calculateConfidence(highCandidate)

	if highConfidence <= lowConfidence {
		t.Errorf("Expected high sample candidate to have higher confidence (%.2f <= %.2f)",
			highConfidence, lowConfidence)
	}

	t.Logf("Low confidence: %.2f, High confidence: %.2f", lowConfidence, highConfidence)
}
