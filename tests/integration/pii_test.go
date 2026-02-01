// Package integration provides end-to-end tests for dogwatch PII features.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dogwatch/internal/pii"
)

// testPIIStore creates a test PII store with in-memory SQLite
func testPIIStore(t *testing.T) (*pii.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dogwatch-pii-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "pii_test.db")
	store, err := pii.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create PII store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestLogIngestionWithPIIRedaction tests log ingestion with automatic PII redaction
func TestLogIngestionWithPIIRedaction(t *testing.T) {
	store, cleanup := testPIIStore(t)
	defer cleanup()

	testCases := []struct {
		name           string
		input          string
		shouldRedact   bool
		expectedTypes  []pii.PIIType
		notContains    []string // Strings that should NOT appear in redacted output
	}{
		{
			name:          "Email address in log",
			input:         "User john.doe@example.com logged in from IP 192.168.1.100",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeEmail},
			notContains:   []string{"john.doe@example.com"},
		},
		{
			name:          "Credit card number",
			input:         "Payment processed for card 4111-1111-1111-1111",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeCreditCard},
			notContains:   []string{"4111-1111-1111-1111", "4111111111111111"},
		},
		{
			name:          "SSN in log message",
			input:         "Customer SSN: 123-45-6789 verified",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeSSN},
			notContains:   []string{"123-45-6789"},
		},
		{
			name:          "Phone number",
			input:         "Contact phone: (555) 123-4567 for support",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypePhone},
			notContains:   []string{"(555) 123-4567", "5551234567"},
		},
		{
			name:          "AWS access key",
			input:         "Using key AKIAIOSFODNN7EXAMPLE for S3 access",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeAWSKey},
			notContains:   []string{"AKIAIOSFODNN7EXAMPLE"},
		},
		{
			name:          "JWT token",
			input:         "Auth token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeJWT},
			notContains:   []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"},
		},
		{
			name:          "No PII present",
			input:         "System started successfully at 2024-01-15 10:30:00",
			shouldRedact:  false,
			expectedTypes: []pii.PIIType{},
			notContains:   []string{},
		},
		{
			name:          "Multiple PII types",
			input:         "User alice@company.com called from 555-987-6543",
			shouldRedact:  true,
			expectedTypes: []pii.PIIType{pii.TypeEmail, pii.TypePhone},
			notContains:   []string{"alice@company.com", "555-987-6543"},
		},
	}

	redactor := store.GetRedactor()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Perform redaction
			redacted, detections := redactor.RedactWithDetails(tc.input)

			// Verify detection occurred when expected
			if tc.shouldRedact && len(detections) == 0 {
				t.Errorf("Expected PII to be detected in: %s", tc.input)
			}
			if !tc.shouldRedact && len(detections) > 0 {
				t.Errorf("Did not expect PII in: %s, but found %d detections", tc.input, len(detections))
			}

			// Verify expected PII types were detected
			for _, expectedType := range tc.expectedTypes {
				found := false
				for _, d := range detections {
					if d.Type == expectedType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected PII type %s not detected in: %s", expectedType, tc.input)
				}
			}

			// Verify redacted output doesn't contain sensitive data
			for _, sensitive := range tc.notContains {
				if strings.Contains(redacted, sensitive) {
					t.Errorf("Redacted output still contains sensitive data '%s': %s", sensitive, redacted)
				}
			}

			// Record detection for stats
			if len(detections) > 0 {
				err := store.RecordDetections("log", "test-log-id", detections, true)
				if err != nil {
					t.Errorf("Failed to record detections: %v", err)
				}
			}

			t.Logf("Input: %s", tc.input)
			t.Logf("Redacted: %s", redacted)
			t.Logf("Detections: %d", len(detections))
		})
	}
}

// TestRedactionStrategies tests different redaction strategies end-to-end
func TestRedactionStrategies(t *testing.T) {
	testCases := []struct {
		name     string
		strategy pii.RedactionStrategy
		input    string
		piiType  pii.PIIType
		validate func(t *testing.T, redacted string)
	}{
		{
			name:     "Mask strategy for email",
			strategy: pii.StrategyMask,
			input:    "Email: john.doe@example.com",
			piiType:  pii.TypeEmail,
			validate: func(t *testing.T, redacted string) {
				if !strings.Contains(redacted, "****@example.com") {
					t.Errorf("Mask strategy should show partial email, got: %s", redacted)
				}
			},
		},
		{
			name:     "Mask strategy for phone",
			strategy: pii.StrategyMask,
			input:    "Phone: 555-123-4567",
			piiType:  pii.TypePhone,
			validate: func(t *testing.T, redacted string) {
				// Should show last 4 digits
				if !strings.Contains(redacted, "4567") {
					t.Errorf("Mask strategy should show last 4 digits for phone, got: %s", redacted)
				}
				if strings.Contains(redacted, "555") {
					t.Errorf("Mask strategy should hide first digits for phone, got: %s", redacted)
				}
			},
		},
		{
			name:     "Mask strategy for SSN",
			strategy: pii.StrategyMask,
			input:    "SSN: 123-45-6789",
			piiType:  pii.TypeSSN,
			validate: func(t *testing.T, redacted string) {
				if !strings.Contains(redacted, "6789") {
					t.Errorf("Mask strategy should show last 4 digits for SSN, got: %s", redacted)
				}
			},
		},
		{
			name:     "Mask strategy for credit card",
			strategy: pii.StrategyMask,
			input:    "Card: 4111111111111111",
			piiType:  pii.TypeCreditCard,
			validate: func(t *testing.T, redacted string) {
				if !strings.Contains(redacted, "1111") {
					t.Errorf("Mask strategy should show last 4 digits for CC, got: %s", redacted)
				}
				if strings.Contains(redacted, "4111111111111111") {
					t.Errorf("Should not contain full credit card number")
				}
			},
		},
		{
			name:     "Hash strategy for email",
			strategy: pii.StrategyHash,
			input:    "User: user@test.com",
			piiType:  pii.TypeEmail,
			validate: func(t *testing.T, redacted string) {
				if !strings.Contains(redacted, "[HASH:") {
					t.Errorf("Hash strategy should produce [HASH:...], got: %s", redacted)
				}
			},
		},
		{
			name:     "Format strategy for JWT",
			strategy: pii.StrategyFormat,
			input:    "Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
			piiType:  pii.TypeJWT,
			validate: func(t *testing.T, redacted string) {
				if !strings.Contains(redacted, "[JWT") {
					t.Errorf("Format strategy should produce [JWT...], got: %s", redacted)
				}
			},
		},
		{
			name:     "Remove strategy",
			strategy: pii.StrategyRemove,
			input:    "Config: password=secretvalue123",
			piiType:  pii.TypePassword,
			validate: func(t *testing.T, redacted string) {
				// With remove strategy, the value should be completely gone
				// The pattern captures the password value after password=
				if strings.Contains(redacted, "secretvalue123") {
					t.Errorf("Remove strategy should completely remove the value, got: %s", redacted)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create config with specific strategy
			config := pii.DefaultConfig()
			config.SetTypeConfig(tc.piiType, true, tc.strategy)

			redactor := pii.NewRedactor(config)
			redacted := redactor.Redact(tc.input)

			tc.validate(t, redacted)
			t.Logf("Input: %s", tc.input)
			t.Logf("Strategy: %s", tc.strategy)
			t.Logf("Redacted: %s", redacted)
		})
	}
}

// TestTokenizationRoundTrip tests tokenization and detokenization
func TestTokenizationRoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dogwatch-pii-token-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config with tokenize strategy for emails
	config := pii.DefaultConfig()
	config.SetTypeConfig(pii.TypeEmail, true, pii.StrategyTokenize)
	config.SetTypeConfig(pii.TypePhone, true, pii.StrategyTokenize)

	redactor := pii.NewRedactor(config)

	testCases := []struct {
		name     string
		input    string
		piiValue string
		piiType  pii.PIIType
	}{
		{
			name:     "Email tokenization",
			input:    "Contact: alice@example.com for help",
			piiValue: "alice@example.com",
			piiType:  pii.TypeEmail,
		},
		{
			name:     "Phone tokenization",
			input:    "Call 555-123-4567 for support",
			piiValue: "555-123-4567",
			piiType:  pii.TypePhone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Step 1: Tokenize
			redacted := redactor.Redact(tc.input)

			// Verify original value is not in redacted output
			if strings.Contains(redacted, tc.piiValue) {
				t.Errorf("Tokenized output should not contain original value: %s", redacted)
			}

			// Verify token format
			if !strings.Contains(redacted, "[PII:") {
				t.Errorf("Tokenized output should contain [PII: token, got: %s", redacted)
			}

			t.Logf("Original: %s", tc.input)
			t.Logf("Tokenized: %s", redacted)

			// Step 2: Detokenize
			detokenized := redactor.DetokenizeText(redacted)

			// Verify original value is restored
			if !strings.Contains(detokenized, tc.piiValue) {
				t.Errorf("Detokenized output should contain original value '%s', got: %s",
					tc.piiValue, detokenized)
			}

			// Verify token is removed
			if strings.Contains(detokenized, "[PII:") {
				t.Errorf("Detokenized output should not contain token, got: %s", detokenized)
			}

			t.Logf("Detokenized: %s", detokenized)
		})
	}
}

// TestConfigChangesAffectRedaction tests that config changes affect redaction behavior
func TestConfigChangesAffectRedaction(t *testing.T) {
	store, cleanup := testPIIStore(t)
	defer cleanup()

	input := "Email: test@example.com, Phone: 555-123-4567"

	// Get initial config
	config := store.GetConfig()
	redactor := pii.NewRedactor(config)

	// Both should be redacted by default
	redacted1 := redactor.Redact(input)
	if strings.Contains(redacted1, "test@example.com") {
		t.Error("Email should be redacted by default")
	}
	if strings.Contains(redacted1, "555-123-4567") {
		t.Error("Phone should be redacted by default")
	}

	// Disable email redaction
	newConfig := config.Clone()
	newConfig.SetTypeConfig(pii.TypeEmail, false, pii.StrategyMask)

	err := store.UpdateConfig(newConfig)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Reload and test
	redactor2 := store.GetRedactor()
	redacted2 := redactor2.Redact(input)

	// Email should NOT be redacted now
	if !strings.Contains(redacted2, "test@example.com") {
		t.Error("Email should NOT be redacted after disabling")
	}
	// Phone should still be redacted
	if strings.Contains(redacted2, "555-123-4567") {
		t.Error("Phone should still be redacted")
	}

	t.Logf("Before config change: %s", redacted1)
	t.Logf("After config change: %s", redacted2)
}

// TestAllowlistDenylist tests allowlist and denylist functionality
func TestAllowlistDenylist(t *testing.T) {
	config := pii.DefaultConfig()

	// Add to allowlist - this value should NOT be redacted
	config.AddToAllowlist("test@company.com")

	// Add to denylist - this value should ALWAYS be detected
	config.AddToDenylist("secret-internal-value")

	redactor := pii.NewRedactor(config)

	// Test allowlist
	t.Run("Allowlist prevents redaction", func(t *testing.T) {
		input := "Contact test@company.com for help"
		redacted := redactor.Redact(input)

		if !strings.Contains(redacted, "test@company.com") {
			t.Errorf("Allowlisted email should NOT be redacted, got: %s", redacted)
		}
	})

	// Test that non-allowlisted emails are still redacted
	t.Run("Non-allowlisted emails are redacted", func(t *testing.T) {
		input := "Contact other@example.com for help"
		redacted := redactor.Redact(input)

		if strings.Contains(redacted, "other@example.com") {
			t.Errorf("Non-allowlisted email should be redacted, got: %s", redacted)
		}
	})
}

// TestCustomPatterns tests custom PII patterns
func TestCustomPatterns(t *testing.T) {
	config := pii.DefaultConfig()

	// Add custom pattern for employee IDs (format: EMP-XXXXX)
	config.AddCustomPattern("employee_id", `EMP-\d{5}`, pii.StrategyMask)

	redactor := pii.NewRedactor(config)

	input := "Employee EMP-12345 submitted request"
	redacted, detections := redactor.RedactWithDetails(input)

	// Check if custom pattern was detected
	customDetected := false
	for _, d := range detections {
		if d.Type == pii.PIIType("employee_id") {
			customDetected = true
			break
		}
	}

	if !customDetected {
		t.Error("Custom pattern 'employee_id' should be detected")
	}

	if strings.Contains(redacted, "EMP-12345") {
		t.Errorf("Custom pattern should be redacted, got: %s", redacted)
	}

	t.Logf("Input: %s", input)
	t.Logf("Redacted: %s", redacted)
}

// TestPIIStats tests PII detection statistics
func TestPIIStats(t *testing.T) {
	store, cleanup := testPIIStore(t)
	defer cleanup()

	redactor := store.GetRedactor()

	// Process multiple logs with different PII types
	logs := []string{
		"User alice@example.com logged in",
		"User bob@test.com made purchase",
		"Card 4111111111111111 charged",
		"SSN 123-45-6789 verified",
		"No PII in this log",
	}

	for _, log := range logs {
		_, detections := redactor.RedactWithDetails(log)
		if len(detections) > 0 {
			store.RecordDetections("log", "log-"+log[:10], detections, true)
		}
	}

	// Get stats
	stats, err := store.GetStats(24 * 60 * 60 * 1000000000) // 24 hours in nanoseconds (as Duration)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.TotalDetections < 4 {
		t.Errorf("Expected at least 4 detections, got %d", stats.TotalDetections)
	}

	t.Logf("Total detections: %d", stats.TotalDetections)
	t.Logf("Detections by type: %v", stats.DetectionsByType)
	t.Logf("High confidence: %d, Medium: %d, Low: %d",
		stats.HighConfidence, stats.MediumConfidence, stats.LowConfidence)
}

// TestPIIScan tests the scan functionality
func TestPIIScan(t *testing.T) {
	store, cleanup := testPIIStore(t)
	defer cleanup()

	input := "User john@example.com called from 555-123-4567, card: 4111111111111111"

	result := store.Scan(input)

	if !result.HasPII {
		t.Error("Expected PII to be detected")
	}

	if result.Stats.Total < 3 {
		t.Errorf("Expected at least 3 PII detections, got %d", result.Stats.Total)
	}

	// Verify different types were detected
	if result.Stats.ByType["email"] < 1 {
		t.Error("Expected email to be detected")
	}
	if result.Stats.ByType["phone"] < 1 {
		t.Error("Expected phone to be detected")
	}
	if result.Stats.ByType["credit_card"] < 1 {
		t.Error("Expected credit card to be detected")
	}

	t.Logf("Scan result: %+v", result.Stats)
	t.Logf("Redacted: %s", result.Redacted)
}

// TestPIIDetectorConfidence tests confidence level assignment
func TestPIIDetectorConfidence(t *testing.T) {
	config := pii.DefaultConfig()
	detector := pii.NewDetector(config)

	testCases := []struct {
		name               string
		input              string
		expectedConfidence pii.Confidence
		piiType            pii.PIIType
	}{
		{
			name:               "Valid credit card - high confidence",
			input:              "Card: 4111111111111111",
			expectedConfidence: pii.ConfidenceHigh,
			piiType:            pii.TypeCreditCard,
		},
		{
			name:               "Email - high confidence",
			input:              "Email: user@example.com",
			expectedConfidence: pii.ConfidenceHigh,
			piiType:            pii.TypeEmail,
		},
		{
			name:               "AWS Key - high confidence",
			input:              "Key: AKIAIOSFODNN7EXAMPLE",
			expectedConfidence: pii.ConfidenceHigh,
			piiType:            pii.TypeAWSKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			detections := detector.Detect(tc.input)

			found := false
			for _, d := range detections {
				if d.Type == tc.piiType {
					found = true
					if d.Confidence != tc.expectedConfidence {
						t.Errorf("Expected confidence %s for %s, got %s",
							tc.expectedConfidence, tc.piiType, d.Confidence)
					}
					break
				}
			}

			if !found {
				t.Errorf("PII type %s not detected in: %s", tc.piiType, tc.input)
			}
		})
	}
}
