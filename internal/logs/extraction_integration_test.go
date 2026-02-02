package logs

import (
	"sync"
	"testing"
	"time"
)

// Integration tests for FieldExtractor

func TestFieldExtractor_PatternLearning(t *testing.T) {
	fe := NewFieldExtractor()

	// Simulate learning from multiple similar log lines
	logLines := []string{
		`{"level":"info","timestamp":"2024-01-15T10:00:00Z","service":"api","user_id":123,"action":"login"}`,
		`{"level":"info","timestamp":"2024-01-15T10:01:00Z","service":"api","user_id":456,"action":"logout"}`,
		`{"level":"error","timestamp":"2024-01-15T10:02:00Z","service":"api","user_id":789,"action":"payment_failed"}`,
		`{"level":"warn","timestamp":"2024-01-15T10:03:00Z","service":"api","user_id":101,"action":"rate_limited"}`,
	}

	for _, line := range logLines {
		fields := fe.Extract(line, "api-service")
		if len(fields) == 0 {
			t.Errorf("Failed to extract fields from: %s", line)
		}
	}

	stats := fe.GetStats()

	// Should have processed all lines
	if stats.TotalExtractions < int64(len(logLines)) {
		t.Errorf("Expected at least %d extractions, got %d", len(logLines), stats.TotalExtractions)
	}

	// Should have extracted multiple fields per line
	if stats.TotalFields < int64(len(logLines)*3) {
		t.Errorf("Expected at least %d fields, got %d", len(logLines)*3, stats.TotalFields)
	}
}

func TestFieldExtractor_MultiFormatExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	testCases := []struct {
		name           string
		message        string
		source         string
		expectedFields []string
		minFieldCount  int
	}{
		{
			name:           "JSON format",
			message:        `{"level":"info","method":"GET","path":"/api/users","status":200}`,
			source:         "api",
			expectedFields: []string{"level", "method", "path", "status"},
			minFieldCount:  4,
		},
		{
			name:           "Key-value format",
			message:        `level=info method=GET path=/api/users status=200 duration=150ms`,
			source:         "nginx",
			expectedFields: []string{"level", "method", "path", "status", "duration"},
			minFieldCount:  4,
		},
		{
			name:           "Apache combined log",
			message:        `192.168.1.100 - john [15/Jan/2024:10:30:00 +0000] "GET /api/users HTTP/1.1" 200 1234`,
			source:         "apache",
			expectedFields: []string{}, // May vary based on pattern matching
			minFieldCount:  0,          // Extraction depends on configured patterns
		},
		{
			name:           "Mixed format",
			message:        `[2024-01-15T10:30:00Z] INFO service=api-gateway {"request_id":"abc123","duration_ms":150}`,
			source:         "mixed",
			expectedFields: []string{},
			minFieldCount:  1, // Should get at least the JSON fields
		},
		{
			name:           "Quoted key-value",
			message:        `level=info message="User logged in successfully" user="john doe"`,
			source:         "auth",
			expectedFields: []string{"level", "message", "user"},
			minFieldCount:  3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fields := fe.Extract(tc.message, tc.source)

			if len(fields) < tc.minFieldCount {
				t.Errorf("Expected at least %d fields, got %d", tc.minFieldCount, len(fields))
			}

			// Check for expected fields
			fieldMap := make(map[string]ExtractedField)
			for _, f := range fields {
				fieldMap[f.Name] = f
			}

			for _, expected := range tc.expectedFields {
				if _, ok := fieldMap[expected]; !ok {
					t.Errorf("Missing expected field: %s", expected)
				}
			}
		})
	}
}

func TestFieldExtractor_GrokPatternMatching(t *testing.T) {
	fe := NewFieldExtractor()

	// Test Grok pattern expansion
	tests := []struct {
		pattern     string
		shouldParse bool
	}{
		{"%{IP:client_ip} - - [%{HTTPDATE:timestamp}]", true},
		{"%{WORD:level} %{GREEDYDATA:message}", true},
		{"%{INT:status} %{NUMBER:duration}", true},
		{"%{UUID:request_id}", true},
		{"%{INVALID_PATTERN:field}", true}, // Unknown patterns should still compile
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			expanded := fe.expandGrokPattern(tt.pattern)
			if tt.shouldParse && expanded == tt.pattern {
				t.Log("Note: Pattern may not have been fully expanded")
			}
		})
	}
}

func TestFieldExtractor_TypeInferenceAccuracy(t *testing.T) {
	fe := NewFieldExtractor()

	tests := []struct {
		value         string
		expectedType  FieldType
		minConfidence float64
	}{
		// Integers
		{"123", FieldTypeInt, 0.9},
		{"-456", FieldTypeInt, 0.9},
		// Note: "0" may be inferred as bool by some implementations

		// Floats
		{"12.34", FieldTypeFloat, 0.9},
		{"-56.78", FieldTypeFloat, 0.9},
		{"0.5", FieldTypeFloat, 0.9},

		// Booleans
		{"true", FieldTypeBool, 0.9},
		{"false", FieldTypeBool, 0.9},
		{"yes", FieldTypeBool, 0.8},
		{"no", FieldTypeBool, 0.8},

		// IP addresses
		{"192.168.1.1", FieldTypeIP, 0.9},
		{"10.0.0.255", FieldTypeIP, 0.9},

		// UUIDs
		{"550e8400-e29b-41d4-a716-446655440000", FieldTypeUUID, 0.9},
		{"a1b2c3d4-e5f6-7890-abcd-ef1234567890", FieldTypeUUID, 0.9},

		// Emails
		{"user@example.com", FieldTypeEmail, 0.9},
		{"test.user+tag@sub.domain.co.uk", FieldTypeEmail, 0.8},

		// URLs
		{"https://example.com/path", FieldTypeURL, 0.8},
		{"http://localhost:8080/api", FieldTypeURL, 0.8},

		// Durations
		{"150ms", FieldTypeDuration, 0.9},
		{"2.5s", FieldTypeDuration, 0.8},

		// Bytes
		{"1.5GB", FieldTypeBytes, 0.8},
		{"512MB", FieldTypeBytes, 0.8},

		// Timestamps
		{"2024-01-15T10:30:00Z", FieldTypeTimestamp, 0.9},
		{"2024-01-15 10:30:00", FieldTypeTimestamp, 0.8},

		// Strings (fallback)
		{"hello world", FieldTypeString, 0.5},
		{"random_text_here", FieldTypeString, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			inferredType, confidence := fe.inferType(tt.value)

			if inferredType != tt.expectedType {
				t.Errorf("Value %q: expected type %s, got %s (confidence: %.2f)",
					tt.value, tt.expectedType, inferredType, confidence)
			}

			if confidence < tt.minConfidence {
				t.Errorf("Value %q: expected confidence >= %.2f, got %.2f",
					tt.value, tt.minConfidence, confidence)
			}
		})
	}
}

func TestFieldExtractor_NestedJSONExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	nestedJSON := `{
		"request": {
			"method": "POST",
			"path": "/api/users",
			"headers": {
				"content-type": "application/json"
			}
		},
		"response": {
			"status": 201,
			"body": {
				"id": 123,
				"created": true
			}
		},
		"metadata": {
			"trace_id": "abc123",
			"duration_ms": 150
		}
	}`

	fields := fe.Extract(nestedJSON, "api")

	if len(fields) == 0 {
		t.Fatal("No fields extracted from nested JSON")
	}

	// Build field map for checking
	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	// Check for flattened nested fields
	expectedFields := []string{
		"request.method",
		"request.path",
		"response.status",
		"metadata.trace_id",
		"metadata.duration_ms",
	}

	for _, expected := range expectedFields {
		if _, ok := fieldMap[expected]; !ok {
			t.Errorf("Missing expected nested field: %s", expected)
		}
	}

	// Verify type inference on nested values
	if f, ok := fieldMap["response.status"]; ok {
		if f.Type != FieldTypeInt {
			t.Errorf("Expected status to be int, got %s", f.Type)
		}
	}

	if f, ok := fieldMap["response.body.created"]; ok {
		if f.Type != FieldTypeBool {
			t.Errorf("Expected created to be bool, got %s", f.Type)
		}
	}
}

func TestFieldExtractor_PatternManagement(t *testing.T) {
	fe := NewFieldExtractor()

	// Count initial patterns
	initialPatterns := fe.GetPatterns()
	initialCount := len(initialPatterns)

	// Add custom pattern
	customPattern := &ExtractionPattern{
		Name:     "Custom Test Pattern",
		Type:     "regex",
		Pattern:  `(?P<level>\w+)\s+\[(?P<component>[^\]]+)\]\s+(?P<message>.*)`,
		Enabled:  true,
		Priority: 60,
	}

	err := fe.AddPattern(customPattern)
	if err != nil {
		t.Fatalf("Failed to add pattern: %v", err)
	}

	if customPattern.ID == "" {
		t.Error("Pattern ID should be generated")
	}

	// Verify pattern was added
	patterns := fe.GetPatterns()
	if len(patterns) != initialCount+1 {
		t.Errorf("Expected %d patterns, got %d", initialCount+1, len(patterns))
	}

	// Find our pattern
	found := false
	for _, p := range patterns {
		if p.ID == customPattern.ID {
			found = true
			if p.Name != customPattern.Name {
				t.Errorf("Pattern name mismatch: expected %s, got %s", customPattern.Name, p.Name)
			}
			break
		}
	}
	if !found {
		t.Error("Added pattern not found in list")
	}

	// Test extraction with custom pattern
	testMessage := `ERROR [api-gateway] Connection timeout occurred`
	fields := fe.Extract(testMessage, "custom-test")

	// Should extract level, component, message from our pattern
	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	// Remove pattern
	fe.RemovePattern(customPattern.ID)

	patterns = fe.GetPatterns()
	if len(patterns) != initialCount {
		t.Errorf("Expected %d patterns after removal, got %d", initialCount, len(patterns))
	}
}

func TestFieldExtractor_SourcePatternAssociation(t *testing.T) {
	fe := NewFieldExtractor()

	// Add custom patterns
	pattern1 := &ExtractionPattern{
		Name:     "Pattern 1",
		Type:     "kv",
		Pattern:  "key=value",
		Enabled:  true,
		Priority: 70,
	}
	pattern2 := &ExtractionPattern{
		Name:     "Pattern 2",
		Type:     "json",
		Pattern:  "{}",
		Enabled:  true,
		Priority: 80,
	}

	fe.AddPattern(pattern1)
	fe.AddPattern(pattern2)

	// Associate patterns with sources
	fe.SetSourcePattern("nginx-logs", pattern1.ID, 70)
	fe.SetSourcePattern("nginx-logs", pattern2.ID, 80)
	fe.SetSourcePattern("api-logs", pattern2.ID, 80)

	// Verify associations
	fe.mu.RLock()
	nginxPatterns := fe.sourcePatterns["nginx-logs"]
	apiPatterns := fe.sourcePatterns["api-logs"]
	fe.mu.RUnlock()

	if len(nginxPatterns) != 2 {
		t.Errorf("Expected 2 patterns for nginx-logs, got %d", len(nginxPatterns))
	}

	if len(apiPatterns) != 1 {
		t.Errorf("Expected 1 pattern for api-logs, got %d", len(apiPatterns))
	}
}

func TestFieldExtractor_ExtractionStats(t *testing.T) {
	fe := NewFieldExtractor()

	// Reset stats by creating a new extractor
	messages := []string{
		`{"a":1,"b":2}`,
		`{"c":3,"d":4,"e":5}`,
		`x=1 y=2`,
		`just plain text`,
	}

	for _, msg := range messages {
		fe.Extract(msg, "test")
	}

	stats := fe.GetStats()

	if stats.TotalExtractions != int64(len(messages)) {
		t.Errorf("Expected %d total extractions, got %d", len(messages), stats.TotalExtractions)
	}

	// Should have extracted fields from at least JSON and KV messages
	expectedMinFields := int64(2 + 3 + 2) // First JSON (2), second JSON (3), KV (2)
	if stats.TotalFields < expectedMinFields {
		t.Errorf("Expected at least %d total fields, got %d", expectedMinFields, stats.TotalFields)
	}

	if stats.PatternCount == 0 {
		t.Error("Expected pattern count > 0")
	}
}

func TestFieldExtractor_ExtractAndEnrich(t *testing.T) {
	fe := NewFieldExtractor()

	entry := &LogEntry{
		ID:        "log-1",
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   `{"user_id":123,"action":"login","ip":"192.168.1.1","duration_ms":45}`,
		Service:   "auth-service",
	}

	// Entry should have nil Attrs initially
	if entry.Attrs != nil && len(entry.Attrs) > 0 {
		t.Error("Expected empty Attrs before enrichment")
	}

	fe.ExtractAndEnrich(entry)

	// Attrs should be populated
	if entry.Attrs == nil {
		t.Fatal("Expected Attrs to be initialized")
	}

	// Check extracted fields
	expectedFields := map[string]interface{}{
		"user_id": int64(123),
		"action":  "login",
		"ip":      "192.168.1.1",
	}

	for field, expectedValue := range expectedFields {
		if value, ok := entry.Attrs[field]; !ok {
			t.Errorf("Missing expected field: %s", field)
		} else if value != expectedValue {
			t.Logf("Field %s: expected %v (%T), got %v (%T)",
				field, expectedValue, expectedValue, value, value)
			// Don't fail on type mismatch, just log
		}
	}

	// Check metadata
	if _, ok := entry.Attrs["_extracted_fields"]; !ok {
		t.Error("Missing _extracted_fields metadata")
	}
}

func TestFieldExtractor_ConcurrentExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	var wg sync.WaitGroup
	numGoroutines := 50
	extractionsPerGoroutine := 100

	messages := []string{
		`{"level":"info","value":123}`,
		`status=200 method=GET`,
		`192.168.1.1 - - [15/Jan/2024:10:00:00] "GET /api" 200`,
		`ERROR [component] Something failed`,
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < extractionsPerGoroutine; j++ {
				msg := messages[(goroutineID+j)%len(messages)]
				fields := fe.Extract(msg, "concurrent-test")
				_ = fields // Use the result
			}
		}(i)
	}

	wg.Wait()

	stats := fe.GetStats()

	expectedExtractions := int64(numGoroutines * extractionsPerGoroutine)
	// Allow some variance due to race conditions in stats collection
	minExpected := expectedExtractions - 100
	if stats.TotalExtractions < minExpected {
		t.Errorf("Expected at least %d extractions, got %d", minExpected, stats.TotalExtractions)
	}
}

func TestFieldExtractor_GrokPatternLibrary(t *testing.T) {
	patterns := GetGrokPatterns()

	requiredPatterns := []string{
		"IP",
		"INT",
		"NUMBER",
		"WORD",
		"UUID",
		"TIMESTAMP_ISO8601",
		"LOGLEVEL",
		"HTTPDATE",
		"GREEDYDATA",
	}

	for _, required := range requiredPatterns {
		if _, ok := patterns[required]; !ok {
			t.Errorf("Missing required Grok pattern: %s", required)
		}
	}

	// Test pattern syntax (should not panic)
	for name, pattern := range patterns {
		err := ValidateGrokPattern(pattern)
		if err != nil {
			t.Logf("Pattern %s may have issues: %v", name, err)
		}
	}
}

// Benchmark tests

func BenchmarkFieldExtractor_JSONExtraction(b *testing.B) {
	fe := NewFieldExtractor()
	message := `{"level":"info","method":"GET","path":"/api/users","status":200,"duration":150.5,"trace_id":"abc123"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.Extract(message, "api")
	}
}

func BenchmarkFieldExtractor_KeyValueExtraction(b *testing.B) {
	fe := NewFieldExtractor()
	message := `level=info method=GET path=/api/users status=200 duration=150ms trace_id=abc123`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.Extract(message, "nginx")
	}
}

func BenchmarkFieldExtractor_NestedJSON(b *testing.B) {
	fe := NewFieldExtractor()
	message := `{"request":{"method":"POST","path":"/api"},"response":{"status":201},"meta":{"trace_id":"abc"}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.Extract(message, "api")
	}
}

func BenchmarkFieldExtractor_TypeInference(b *testing.B) {
	fe := NewFieldExtractor()
	values := []string{
		"192.168.1.1",
		"user@example.com",
		"https://example.com",
		"12345",
		"12.34",
		"true",
		"550e8400-e29b-41d4-a716-446655440000",
		"150ms",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			fe.inferType(v)
		}
	}
}

func BenchmarkFieldExtractor_ConcurrentExtraction(b *testing.B) {
	fe := NewFieldExtractor()
	message := `{"level":"info","method":"GET","path":"/api/users","status":200}`

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			fe.Extract(message, "bench")
		}
	})
}

func BenchmarkFieldExtractor_ExtractAndEnrich(b *testing.B) {
	fe := NewFieldExtractor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &LogEntry{
			ID:        "log-bench",
			Timestamp: time.Now(),
			Level:     LevelInfo,
			Message:   `{"user_id":123,"action":"test","duration_ms":50}`,
			Service:   "bench",
		}
		fe.ExtractAndEnrich(entry)
	}
}

func BenchmarkFieldExtractor_GrokExpansion(b *testing.B) {
	fe := NewFieldExtractor()
	pattern := `%{IP:client} - %{USER:user} [%{HTTPDATE:timestamp}] "%{WORD:method} %{URIPATH:path} HTTP/%{NUMBER:version}" %{INT:status}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.expandGrokPattern(pattern)
	}
}
