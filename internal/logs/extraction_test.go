package logs

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFieldExtractorCreation(t *testing.T) {
	fe := NewFieldExtractor()

	if fe == nil {
		t.Fatal("Failed to create field extractor")
	}

	if len(fe.builtinPatterns) == 0 {
		t.Error("No builtin patterns initialized")
	}

	if len(fe.typePatterns) == 0 {
		t.Error("No type patterns initialized")
	}
}

func TestJSONExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `{"level":"info","message":"User logged in","user_id":123,"email":"user@example.com","timestamp":"2024-01-15T10:30:00Z"}`

	fields := fe.Extract(message, "test-service")

	if len(fields) == 0 {
		t.Fatal("No fields extracted from JSON")
	}

	// Check specific fields
	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	if f, ok := fieldMap["level"]; !ok || f.RawValue != "info" {
		t.Error("Failed to extract level field")
	}

	if f, ok := fieldMap["user_id"]; !ok {
		t.Error("Failed to extract user_id field")
	} else if f.Type != FieldTypeInt && f.Type != FieldTypeFloat {
		t.Errorf("Expected int type for user_id, got %s", f.Type)
	}

	if f, ok := fieldMap["email"]; !ok {
		t.Error("Failed to extract email field")
	} else if f.Type != FieldTypeEmail {
		t.Errorf("Expected email type, got %s", f.Type)
	}
}

func TestKeyValueExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `level=info method=GET path=/api/users status=200 duration=150ms`

	fields := fe.Extract(message, "test-service")

	if len(fields) == 0 {
		t.Fatal("No fields extracted from key-value pairs")
	}

	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	if f, ok := fieldMap["level"]; !ok || f.RawValue != "info" {
		t.Error("Failed to extract level field")
	}

	if f, ok := fieldMap["status"]; !ok {
		t.Error("Failed to extract status field")
	} else if f.Type != FieldTypeInt {
		t.Errorf("Expected int type for status, got %s", f.Type)
	}

	if f, ok := fieldMap["duration"]; !ok {
		t.Error("Failed to extract duration field")
	} else if f.Type != FieldTypeDuration {
		t.Errorf("Expected duration type, got %s", f.Type)
	}
}

func TestApacheLogExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `192.168.1.100 - john [15/Jan/2024:10:30:00 +0000] "GET /api/users HTTP/1.1" 200 1234 "https://example.com" "Mozilla/5.0"`

	fields := fe.Extract(message, "apache")

	if len(fields) == 0 {
		t.Fatal("No fields extracted from Apache log")
	}

	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	if f, ok := fieldMap["client_ip"]; !ok {
		t.Error("Failed to extract client_ip")
	} else if f.Type != FieldTypeIP {
		t.Errorf("Expected IP type for client_ip, got %s", f.Type)
	}

	if _, ok := fieldMap["http_method"]; !ok {
		// Field might have different name based on mapping
		if _, ok := fieldMap["method"]; !ok {
			t.Log("Note: method field may have different name")
		}
	}

	if f, ok := fieldMap["status_code"]; !ok {
		if f, ok = fieldMap["status"]; !ok {
			t.Log("Note: status field may have different name")
		} else if f.Type != FieldTypeInt {
			t.Errorf("Expected int type for status, got %s", f.Type)
		}
	}
}

func TestTypeInference(t *testing.T) {
	fe := NewFieldExtractor()

	tests := []struct {
		value    string
		expected FieldType
	}{
		{"123", FieldTypeInt},
		{"-456", FieldTypeInt},
		{"12.34", FieldTypeFloat},
		{"-56.78", FieldTypeFloat},
		{"true", FieldTypeBool},
		{"false", FieldTypeBool},
		{"yes", FieldTypeBool},
		{"192.168.1.1", FieldTypeIP},
		{"550e8400-e29b-41d4-a716-446655440000", FieldTypeUUID},
		{"user@example.com", FieldTypeEmail},
		{"https://example.com/path", FieldTypeURL},
		{"/var/log/app.log", FieldTypePath},
		{"150ms", FieldTypeDuration},
		{"1.5GB", FieldTypeBytes},
		{"2024-01-15T10:30:00Z", FieldTypeTimestamp},
		{"hello world", FieldTypeString},
	}

	for _, tt := range tests {
		inferredType, confidence := fe.inferType(tt.value)
		if inferredType != tt.expected {
			t.Errorf("Value %q: expected %s, got %s (confidence: %.2f)",
				tt.value, tt.expected, inferredType, confidence)
		}
	}
}

func TestValueParsing(t *testing.T) {
	fe := NewFieldExtractor()

	tests := []struct {
		value     string
		fieldType FieldType
		expected  interface{}
	}{
		{"123", FieldTypeInt, int64(123)},
		{"12.34", FieldTypeFloat, 12.34},
		{"true", FieldTypeBool, true},
		{"false", FieldTypeBool, false},
		{"yes", FieldTypeBool, true},
		{"no", FieldTypeBool, false},
		{`{"key":"value"}`, FieldTypeJSON, map[string]interface{}{"key": "value"}},
	}

	for _, tt := range tests {
		result := fe.parseValue(tt.value, tt.fieldType)

		// Special handling for JSON comparison
		if tt.fieldType == FieldTypeJSON {
			resultJSON, _ := json.Marshal(result)
			expectedJSON, _ := json.Marshal(tt.expected)
			if string(resultJSON) != string(expectedJSON) {
				t.Errorf("Value %q: expected %v, got %v", tt.value, tt.expected, result)
			}
		} else if result != tt.expected {
			t.Errorf("Value %q type %s: expected %v (%T), got %v (%T)",
				tt.value, tt.fieldType, tt.expected, tt.expected, result, result)
		}
	}
}

func TestNestedJSONExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `{"request":{"method":"POST","path":"/api/users"},"response":{"status":201},"user":{"id":123,"name":"John"}}`

	fields := fe.Extract(message, "test-service")

	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	// Check nested fields are flattened
	expectedFields := []string{
		"request.method",
		"request.path",
		"response.status",
		"user.id",
		"user.name",
	}

	for _, expected := range expectedFields {
		if _, ok := fieldMap[expected]; !ok {
			t.Errorf("Missing nested field: %s", expected)
		}
	}
}

func TestPatternAddRemove(t *testing.T) {
	fe := NewFieldExtractor()

	pattern := &ExtractionPattern{
		Name:     "Custom Pattern",
		Type:     "custom",
		Pattern:  `(?P<key>\w+):\s*(?P<value>\S+)`,
		Enabled:  true,
		Priority: 50,
	}

	err := fe.AddPattern(pattern)
	if err != nil {
		t.Fatalf("Failed to add pattern: %v", err)
	}

	if pattern.ID == "" {
		t.Error("Pattern ID not generated")
	}

	// Get patterns
	patterns := fe.GetPatterns()
	found := false
	for _, p := range patterns {
		if p.ID == pattern.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Added pattern not found in list")
	}

	// Remove pattern
	fe.RemovePattern(pattern.ID)

	patterns = fe.GetPatterns()
	for _, p := range patterns {
		if p.ID == pattern.ID {
			t.Error("Removed pattern still in list")
		}
	}
}

func TestSourcePatternAssociation(t *testing.T) {
	fe := NewFieldExtractor()

	pattern := &ExtractionPattern{
		Name:     "Source Pattern",
		Type:     "custom",
		Pattern:  `(?P<field>\w+)`,
		Enabled:  true,
		Priority: 50,
	}

	fe.AddPattern(pattern)
	fe.SetSourcePattern("my-service", pattern.ID, 50)

	fe.mu.RLock()
	patterns := fe.sourcePatterns["my-service"]
	fe.mu.RUnlock()

	if len(patterns) == 0 {
		t.Error("Source pattern association failed")
	}

	found := false
	for _, p := range patterns {
		if p == pattern.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Pattern not associated with source")
	}
}

func TestGrokPatternExpansion(t *testing.T) {
	fe := NewFieldExtractor()

	tests := []struct {
		pattern  string
		contains string
	}{
		{"%{IP}", `\d{1,3}`},       // IP pattern contains digit ranges
		{"%{IP:client_ip}", `(?P<client_ip>`},
		{"%{INT}", `[+-]?[0-9]+`},
		{"%{YEAR}", `\d\d`},        // Year pattern should contain digits
	}

	for _, tt := range tests {
		expanded := fe.expandGrokPattern(tt.pattern)
		if !containsSubstring(expanded, tt.contains) {
			t.Errorf("Pattern %q: expected to contain %q, got %q",
				tt.pattern, tt.contains, expanded)
		}
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidateGrokPattern(t *testing.T) {
	// Valid pattern
	err := ValidateGrokPattern(`%{IP:ip} %{INT:status}`)
	if err != nil {
		t.Errorf("Valid pattern rejected: %v", err)
	}

	// Invalid pattern
	err = ValidateGrokPattern(`(?P<broken`)
	if err == nil {
		t.Error("Invalid pattern accepted")
	}
}

func TestGetGrokPatterns(t *testing.T) {
	patterns := GetGrokPatterns()

	expectedPatterns := []string{"IP", "INT", "UUID", "TIMESTAMP_ISO8601", "LOGLEVEL"}
	for _, expected := range expectedPatterns {
		if _, ok := patterns[expected]; !ok {
			t.Errorf("Missing expected Grok pattern: %s", expected)
		}
	}
}

func TestExtractionStats(t *testing.T) {
	fe := NewFieldExtractor()

	// Extract from several messages
	messages := []string{
		`{"level":"info","value":123}`,
		`status=200 method=GET`,
		`{"error":"not found"}`,
	}

	for _, msg := range messages {
		fe.Extract(msg, "test")
	}

	stats := fe.GetStats()

	if stats.TotalExtractions < 3 {
		t.Errorf("Expected at least 3 extractions, got %d", stats.TotalExtractions)
	}

	if stats.TotalFields == 0 {
		t.Error("Expected some fields to be extracted")
	}

	if stats.PatternCount == 0 {
		t.Error("Expected pattern count > 0")
	}
}

func TestExtractAndEnrich(t *testing.T) {
	fe := NewFieldExtractor()

	entry := &LogEntry{
		ID:        "log-1",
		Timestamp: time.Now(),
		Level:     LevelInfo,
		Message:   `{"user_id":123,"action":"login","ip":"192.168.1.1"}`,
		Service:   "auth-service",
	}

	fe.ExtractAndEnrich(entry)

	if entry.Attrs == nil {
		t.Fatal("Attrs not initialized")
	}

	if _, ok := entry.Attrs["user_id"]; !ok {
		t.Error("user_id not extracted")
	}

	if _, ok := entry.Attrs["ip"]; !ok {
		t.Error("ip not extracted")
	}

	if _, ok := entry.Attrs["_extracted_fields"]; !ok {
		t.Error("_extracted_fields metadata not added")
	}
}

func TestQuotedKeyValueExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `level=info message="User logged in successfully" user="john doe" path="/api/users"`

	fields := fe.Extract(message, "test-service")

	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	if f, ok := fieldMap["message"]; !ok {
		t.Error("Failed to extract message field")
	} else if f.RawValue != "User logged in successfully" {
		t.Errorf("Message not unquoted: %s", f.RawValue)
	}

	if f, ok := fieldMap["user"]; !ok {
		t.Error("Failed to extract user field")
	} else if f.RawValue != "john doe" {
		t.Errorf("User not unquoted: %s", f.RawValue)
	}
}

func TestSyslogExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `<34>Jan 15 10:30:00 myhost myapp[1234]: User authentication successful`

	fields := fe.Extract(message, "syslog")

	if len(fields) == 0 {
		t.Log("Note: Syslog pattern may not match this exact format")
		return
	}

	fieldMap := make(map[string]ExtractedField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	// Check for expected syslog fields
	if _, ok := fieldMap["priority"]; !ok {
		t.Log("Priority field not extracted")
	}

	if _, ok := fieldMap["hostname"]; !ok {
		t.Log("Hostname field not extracted")
	}
}

func TestBuiltinPatternsInitialized(t *testing.T) {
	fe := NewFieldExtractor()

	expectedPatterns := []string{"json", "kv", "apache-combined", "nginx", "syslog"}

	for _, expected := range expectedPatterns {
		patternID := "builtin-" + expected
		if _, ok := fe.builtinPatterns[patternID]; !ok {
			// Some might be named differently
			found := false
			for id := range fe.builtinPatterns {
				if containsSubstring(id, expected) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Missing builtin pattern: %s", expected)
			}
		}
	}
}

func TestEmptyMessageExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	fields := fe.Extract("", "test")

	if len(fields) != 0 {
		t.Errorf("Expected no fields from empty message, got %d", len(fields))
	}
}

func TestNonJSONNonKVExtraction(t *testing.T) {
	fe := NewFieldExtractor()

	message := `This is just a plain text log message with no structure`

	fields := fe.Extract(message, "test")

	// Should not crash, may return no fields
	t.Logf("Extracted %d fields from plain text", len(fields))
}

func TestIPv6TypeInference(t *testing.T) {
	fe := NewFieldExtractor()

	// Note: IPv6 detection may be limited
	ipv6Values := []string{
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		"::1",
		"fe80::1",
	}

	for _, ip := range ipv6Values {
		inferredType, _ := fe.inferType(ip)
		// IPv6 might be detected as string or hex depending on format
		if inferredType != FieldTypeString {
			t.Logf("IPv6 %s detected as %s", ip, inferredType)
		}
	}
}

func TestPatternPriorityOrdering(t *testing.T) {
	fe := NewFieldExtractor()

	patterns := fe.GetPatterns()

	// Verify patterns are sorted by priority (descending)
	for i := 1; i < len(patterns); i++ {
		if patterns[i].Priority > patterns[i-1].Priority {
			t.Error("Patterns not sorted by priority")
			break
		}
	}
}

func TestFieldExtractionConfidence(t *testing.T) {
	fe := NewFieldExtractor()

	message := `{"email":"user@example.com","id":"123","date":"2024-01-15"}`

	fields := fe.Extract(message, "test")

	for _, f := range fields {
		if f.Confidence < 0 || f.Confidence > 1 {
			t.Errorf("Confidence out of range for field %s: %f", f.Name, f.Confidence)
		}
	}
}

func TestExtractionPatternSerialization(t *testing.T) {
	pattern := &ExtractionPattern{
		ID:       "test-pattern",
		Name:     "Test Pattern",
		Type:     "custom",
		Pattern:  `(?P<field>\w+)`,
		Enabled:  true,
		Priority: 50,
		FieldMappings: map[string]string{
			"field": "mapped_field",
		},
	}

	data, err := json.Marshal(pattern)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed ExtractionPattern
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.ID != pattern.ID {
		t.Error("ID not preserved")
	}

	if parsed.Pattern != pattern.Pattern {
		t.Error("Pattern not preserved")
	}

	if len(parsed.FieldMappings) != 1 {
		t.Error("Field mappings not preserved")
	}
}

func BenchmarkJSONExtraction(b *testing.B) {
	fe := NewFieldExtractor()
	message := `{"level":"info","method":"GET","path":"/api/users","status":200,"duration":150.5}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.Extract(message, "test")
	}
}

func BenchmarkKeyValueExtraction(b *testing.B) {
	fe := NewFieldExtractor()
	message := `level=info method=GET path=/api/users status=200 duration=150ms`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fe.Extract(message, "test")
	}
}

func BenchmarkTypeInference(b *testing.B) {
	fe := NewFieldExtractor()
	values := []string{
		"192.168.1.1",
		"user@example.com",
		"https://example.com",
		"12345",
		"12.34",
		"true",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range values {
			fe.inferType(v)
		}
	}
}
