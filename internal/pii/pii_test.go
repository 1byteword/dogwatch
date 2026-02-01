package pii

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestDetector_Email(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		input    string
		expected bool
		count    int
	}{
		{"Contact us at user@example.com", true, 1},
		{"Multiple: a@b.co and c@d.org", true, 2},
		{"Email: test.user+tag@subdomain.example.com", true, 1},
		{"No email here", false, 0},
		{"Invalid @email.com", false, 0},
		{"Also invalid user@", false, 0},
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		emailCount := 0
		for _, det := range detections {
			if det.Type == TypeEmail {
				emailCount++
			}
		}

		if (emailCount > 0) != tt.expected {
			t.Errorf("Detect(%q) expected email=%v, got %d detections", tt.input, tt.expected, emailCount)
		}
		if emailCount != tt.count {
			t.Errorf("Detect(%q) expected %d emails, got %d", tt.input, tt.count, emailCount)
		}
	}
}

func TestDetector_Phone(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		input    string
		expected bool
	}{
		{"Call 555-123-4567", true},
		{"Phone: (555) 123-4567", true},
		{"Tel: 1-555-123-4567", true},
		{"+1 555 123 4567", true},
		{"+14155551234", true}, // International format
		{"Short: 123-456", false},
		{"Not a phone: 12345", false},
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		found := false
		for _, det := range detections {
			if det.Type == TypePhone {
				found = true
				break
			}
		}

		if found != tt.expected {
			t.Errorf("Detect(%q) phone detection expected=%v, got=%v", tt.input, tt.expected, found)
		}
	}
}

func TestDetector_SSN(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		input    string
		expected bool
	}{
		{"SSN: 123-45-6789", true},
		{"My SSN is 078-05-1120", true},
		// Invalid SSNs (000 prefix, 666 prefix, 9xx prefix)
		{"Invalid: 000-12-3456", false},
		{"Invalid: 666-12-3456", false},
		{"Invalid: 900-12-3456", false},
		{"Not SSN: 12-34-5678", false},
		{"Not SSN: 1234-56-7890", false},
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		found := false
		for _, det := range detections {
			if det.Type == TypeSSN {
				found = true
				break
			}
		}

		if found != tt.expected {
			t.Errorf("Detect(%q) SSN detection expected=%v, got=%v", tt.input, tt.expected, found)
		}
	}
}

func TestDetector_CreditCard(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Valid Visa", "Card: 4111111111111111", true},
		{"Valid MC", "MC: 5500000000000004", true},
		{"Valid Amex", "Amex: 378282246310005", true},
		{"Valid Discover", "Disc: 6011111111111117", true},
		{"Valid with dashes", "Card: 4111-1111-1111-1111", true},
		{"Valid with spaces", "Card: 4111 1111 1111 1111", true},
		{"Invalid Luhn", "Card: 4111111111111112", false},
		{"Too short", "Card: 411111111111", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detections := d.Detect(tt.input)
			found := false
			for _, det := range detections {
				if det.Type == TypeCreditCard {
					found = true
					break
				}
			}

			if found != tt.expected {
				t.Errorf("Detect(%q) credit card detection expected=%v, got=%v", tt.input, tt.expected, found)
			}
		})
	}
}

func TestDetector_IPAddress(t *testing.T) {
	// Enable IP detection
	config := DefaultConfig()
	config.IncludeIPAddresses = true
	config.SetTypeConfig(TypeIPAddress, true, StrategyMask)
	d := NewDetector(config)

	tests := []struct {
		input          string
		expected       bool
		expectedConf   Confidence
	}{
		{"Server: 192.168.1.1", true, ConfidenceLow}, // Private IP
		{"Server: 10.0.0.1", true, ConfidenceLow},    // Private IP
		{"Server: 8.8.8.8", true, ConfidenceMedium},  // Public IP
		{"Server: 256.1.2.3", false, ""},              // Invalid
		{"Server: 1.2.3", false, ""},                  // Incomplete
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		found := false
		var conf Confidence
		for _, det := range detections {
			if det.Type == TypeIPAddress {
				found = true
				conf = det.Confidence
				break
			}
		}

		if found != tt.expected {
			t.Errorf("Detect(%q) IP detection expected=%v, got=%v", tt.input, tt.expected, found)
		}
		if found && conf != tt.expectedConf {
			t.Errorf("Detect(%q) IP confidence expected=%v, got=%v", tt.input, tt.expectedConf, conf)
		}
	}
}

func TestDetector_APIKeys(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		name     string
		input    string
		expected PIIType
	}{
		{"AWS Key", "AKIAIOSFODNN7EXAMPLE", TypeAWSKey},
		{"GitHub PAT", "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", TypeGitHubToken},
		{"GitHub OAuth", "gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", TypeGitHubToken},
		{"Stripe Live", "sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx", TypeStripeKey},
		{"Stripe Test", "pk_test_xxxxxxxxxxxxxxxxxxxxxxxxxxxx", TypeStripeKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detections := d.Detect(tt.input)
			found := false
			for _, det := range detections {
				if det.Type == tt.expected {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Detect(%q) expected to find %s", tt.input, tt.expected)
			}
		})
	}
}

func TestDetector_JWT(t *testing.T) {
	d := NewDetector(DefaultConfig())

	// Valid JWT structure (not cryptographically valid, just structure)
	validJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	tests := []struct {
		input    string
		expected bool
	}{
		{"Token: " + validJWT, true},
		{"Bearer " + validJWT, true},
		{"Not a JWT: abc.def.ghi", false},
		{"Missing part: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0", false},
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		found := false
		for _, det := range detections {
			if det.Type == TypeJWT {
				found = true
				break
			}
		}

		if found != tt.expected {
			t.Errorf("Detect(%q) JWT detection expected=%v, got=%v", tt.input[:50], tt.expected, found)
		}
	}
}

func TestDetector_Password(t *testing.T) {
	d := NewDetector(DefaultConfig())

	tests := []struct {
		input    string
		expected bool
	}{
		{"password=secret123", true},
		{"password:mysecretpass", true},
		{"pwd=abc123", true},
		{"url?password=test&user=admin", true},
		{"Nothing sensitive here", false},
	}

	for _, tt := range tests {
		detections := d.Detect(tt.input)
		found := false
		for _, det := range detections {
			if det.Type == TypePassword {
				found = true
				break
			}
		}

		if found != tt.expected {
			t.Errorf("Detect(%q) password detection expected=%v, got=%v", tt.input, tt.expected, found)
		}
	}
}

func TestRedactor_Mask(t *testing.T) {
	config := DefaultConfig()
	config.SetTypeConfig(TypeEmail, true, StrategyMask)
	r := NewRedactor(config)

	tests := []struct {
		input    string
		contains string
	}{
		{"Email: john@example.com", "j****@example.com"},
		{"Call 555-123-4567 now", "***-***-4567"},
		{"SSN: 123-45-6789", "***-**-6789"},
		{"Card: 4111111111111111", "****-****-****-1111"},
	}

	for _, tt := range tests {
		result := r.Redact(tt.input)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("Redact(%q) expected to contain %q, got %q", tt.input, tt.contains, result)
		}
	}
}

func TestRedactor_Hash(t *testing.T) {
	config := DefaultConfig()
	config.SetTypeConfig(TypeEmail, true, StrategyHash)
	r := NewRedactor(config)

	input := "Email: test@example.com"
	result := r.Redact(input)

	if !strings.Contains(result, "[HASH:") {
		t.Errorf("Redact with hash strategy should contain [HASH:], got %q", result)
	}
}

func TestRedactor_Format(t *testing.T) {
	config := DefaultConfig()
	config.SetTypeConfig(TypeEmail, true, StrategyFormat)
	r := NewRedactor(config)

	input := "Email: test@example.com"
	result := r.Redact(input)

	if !strings.Contains(result, "[EMAIL]") {
		t.Errorf("Redact with format strategy should contain [EMAIL], got %q", result)
	}
}

func TestRedactor_Tokenize(t *testing.T) {
	config := DefaultConfig()
	config.SetTypeConfig(TypeEmail, true, StrategyTokenize)
	r := NewRedactor(config)

	input := "Email: test@example.com"
	result := r.Redact(input)

	if !strings.Contains(result, "[PII:EMAIL:") {
		t.Errorf("Redact with tokenize strategy should contain [PII:EMAIL:], got %q", result)
	}

	// Test detokenization
	detokenized := r.DetokenizeText(result)
	if detokenized != input {
		t.Errorf("DetokenizeText should restore original, got %q", detokenized)
	}
}

func TestRedactor_Remove(t *testing.T) {
	config := DefaultConfig()
	config.SetTypeConfig(TypeEmail, true, StrategyRemove)
	r := NewRedactor(config)

	input := "Email: test@example.com"
	result := r.Redact(input)

	if strings.Contains(result, "@") {
		t.Errorf("Redact with remove strategy should remove email, got %q", result)
	}
}

func TestRedactor_Attributes(t *testing.T) {
	r := NewRedactor(DefaultConfig())

	attrs := map[string]string{
		"user.email":   "test@example.com",
		"user.phone":   "555-123-4567",
		"service.name": "api-gateway",
		"http.url":     "https://api.com?password=secret",
	}

	result := r.RedactAttributes(attrs)

	// Email should be redacted (masked format replaces with t****@example.com)
	if result["user.email"] == "test@example.com" {
		t.Errorf("user.email should be redacted: %s", result["user.email"])
	}

	// Phone should be redacted
	if result["user.phone"] == "555-123-4567" {
		t.Errorf("user.phone should be redacted: %s", result["user.phone"])
	}

	// Service name should not be touched
	if result["service.name"] != "api-gateway" {
		t.Errorf("service.name should not be changed: %s", result["service.name"])
	}

	// Password in URL should be redacted
	if strings.Contains(result["http.url"], "secret") {
		t.Errorf("password in http.url should be redacted: %s", result["http.url"])
	}
}

func TestConfig_TypeEnable(t *testing.T) {
	config := DefaultConfig()

	// IP addresses disabled by default
	if config.IsTypeEnabled(TypeIPAddress) {
		t.Error("IP addresses should be disabled by default")
	}

	// Email enabled by default
	if !config.IsTypeEnabled(TypeEmail) {
		t.Error("Email should be enabled by default")
	}

	// Disable email
	config.SetTypeConfig(TypeEmail, false, StrategyMask)
	if config.IsTypeEnabled(TypeEmail) {
		t.Error("Email should be disabled after SetTypeConfig")
	}

	// Enable IP addresses
	config.IncludeIPAddresses = true
	config.SetTypeConfig(TypeIPAddress, true, StrategyMask)
	if !config.IsTypeEnabled(TypeIPAddress) {
		t.Error("IP addresses should be enabled")
	}
}

func TestConfig_Allowlist(t *testing.T) {
	config := DefaultConfig()
	config.AddToAllowlist("test@example.com")

	d := NewDetector(config)
	detections := d.Detect("Email: test@example.com")

	if len(detections) > 0 {
		t.Error("Allowlisted values should not be detected")
	}

	// Non-allowlisted email should still be detected
	detections = d.Detect("Email: other@example.com")
	if len(detections) == 0 {
		t.Error("Non-allowlisted emails should be detected")
	}
}

func TestConfig_CustomPattern(t *testing.T) {
	config := DefaultConfig()
	config.AddCustomPattern("employee_id", `EMP-[0-9]{6}`, StrategyMask)

	d := NewDetector(config)
	detections := d.Detect("Employee ID: EMP-123456")

	found := false
	for _, det := range detections {
		if det.Type == "employee_id" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Custom pattern should be detected")
	}
}

func TestLuhnValidation(t *testing.T) {
	tests := []struct {
		number   string
		expected bool
	}{
		{"4111111111111111", true},  // Valid Visa
		{"4111111111111112", false}, // Invalid
		{"5500000000000004", true},  // Valid MC
		{"378282246310005", true},   // Valid Amex
		{"6011111111111117", true},  // Valid Discover
		{"", false},
		{"123", false},
	}

	for _, tt := range tests {
		result := luhnCheck(tt.number)
		if result != tt.expected {
			t.Errorf("luhnCheck(%q) = %v, expected %v", tt.number, result, tt.expected)
		}
	}
}

func TestStore_CRUD(t *testing.T) {
	// Create temp db
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "pii_test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test scan
	result := store.Scan("Email: test@example.com, SSN: 123-45-6789")
	if !result.HasPII {
		t.Error("Scan should detect PII")
	}
	if result.Stats.Total < 2 {
		t.Errorf("Scan should detect at least 2 PII items, got %d", result.Stats.Total)
	}

	// Test record detection
	err = store.RecordDetection("log", "log-123", Detection{
		Type:       TypeEmail,
		Value:      "test@example.com",
		Confidence: ConfidenceHigh,
	}, true)
	if err != nil {
		t.Errorf("RecordDetection failed: %v", err)
	}

	// Test stats
	stats, err := store.GetStats(time.Hour)
	if err != nil {
		t.Errorf("GetStats failed: %v", err)
	}
	if stats.TotalDetections != 1 {
		t.Errorf("Expected 1 detection, got %d", stats.TotalDetections)
	}

	// Test config update
	newConfig := DefaultConfig()
	newConfig.SetTypeConfig(TypeEmail, false, StrategyMask)

	err = store.UpdateConfig(newConfig)
	if err != nil {
		t.Errorf("UpdateConfig failed: %v", err)
	}

	// Verify config persisted
	loadedConfig := store.GetConfig()
	if loadedConfig.IsTypeEnabled(TypeEmail) {
		t.Error("Email should be disabled after config update")
	}
}

func TestTokenStore(t *testing.T) {
	ts := NewTokenStore(nil) // In-memory

	// Store a token
	token := ts.Store(TypeEmail, "test@example.com")
	if !strings.HasPrefix(token, "[PII:EMAIL:") {
		t.Errorf("Token format incorrect: %s", token)
	}

	// Retrieve token
	piiType, value, ok := ts.Retrieve(token)
	if !ok {
		t.Error("Token should be retrievable")
	}
	if piiType != TypeEmail {
		t.Errorf("Expected email type, got %s", piiType)
	}
	if value != "test@example.com" {
		t.Errorf("Expected test@example.com, got %s", value)
	}

	// Store same value should return same token
	token2 := ts.Store(TypeEmail, "test@example.com")
	if token != token2 {
		t.Error("Same value should return same token")
	}

	// Count
	if ts.Count() != 1 {
		t.Errorf("Expected 1 token, got %d", ts.Count())
	}

	// Detokenize text
	text := "Contact: " + token
	result := ts.DetokenizeText(text)
	if result != "Contact: test@example.com" {
		t.Errorf("Detokenize failed: %s", result)
	}
}

func TestEncryptor(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes for AES-128

	enc, err := NewEncryptor(key)
	if err != nil {
		t.Fatalf("NewEncryptor failed: %v", err)
	}

	plaintext := "test@example.com"

	// Encrypt
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Error("Encrypted value should differ from plaintext")
	}

	// Decrypt
	decrypted, err := enc.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted value should match plaintext: %s", decrypted)
	}

	// Invalid key size
	_, err = NewEncryptor([]byte("short"))
	if err == nil {
		t.Error("Should reject invalid key size")
	}
}

func TestTokenStoreWithPersistence(t *testing.T) {
	// Create temp db
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tokens_test.db")

	db, err := NewPersistentTokenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create token DB: %v", err)
	}

	key := []byte("0123456789abcdef")
	ts, err := NewTokenStoreWithEncryption(db, key)
	if err != nil {
		t.Fatalf("Failed to create token store: %v", err)
	}

	// Store token
	token := ts.Store(TypeSSN, "123-45-6789")

	// Retrieve
	piiType, value, ok := ts.Retrieve(token)
	if !ok || piiType != TypeSSN || value != "123-45-6789" {
		t.Errorf("Token retrieval failed: %v, %s, %v", piiType, value, ok)
	}

	// Close and reopen
	db.Close()

	db, err = NewPersistentTokenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to reopen token DB: %v", err)
	}
	defer db.Close()

	ts2, err := NewTokenStoreWithEncryption(db, key)
	if err != nil {
		t.Fatalf("Failed to recreate token store: %v", err)
	}

	// Should still be able to retrieve
	piiType, value, ok = ts2.Retrieve(token)
	if !ok || value != "123-45-6789" {
		t.Errorf("Token retrieval after reopen failed: %v, %s, %v", piiType, value, ok)
	}
}

func TestDetector_MultipleTypes(t *testing.T) {
	d := NewDetector(DefaultConfig())

	input := "User john@example.com called 555-123-4567 with card 4111111111111111"
	detections := d.Detect(input)

	typeCount := make(map[PIIType]int)
	for _, det := range detections {
		typeCount[det.Type]++
	}

	if typeCount[TypeEmail] != 1 {
		t.Errorf("Expected 1 email, got %d", typeCount[TypeEmail])
	}
	if typeCount[TypePhone] != 1 {
		t.Errorf("Expected 1 phone, got %d", typeCount[TypePhone])
	}
	if typeCount[TypeCreditCard] != 1 {
		t.Errorf("Expected 1 credit card, got %d", typeCount[TypeCreditCard])
	}
}

func TestHasPII(t *testing.T) {
	d := NewDetector(DefaultConfig())

	if !d.HasPII("Email: test@example.com") {
		t.Error("HasPII should return true for text with PII")
	}

	if d.HasPII("No PII here") {
		t.Error("HasPII should return false for text without PII")
	}
}

func TestHasHighConfidencePII(t *testing.T) {
	d := NewDetector(DefaultConfig())

	// Email is high confidence
	if !d.HasHighConfidencePII("Email: test@example.com") {
		t.Error("HasHighConfidencePII should return true for email")
	}

	// Only text without high confidence PII
	if d.HasHighConfidencePII("No PII here") {
		t.Error("HasHighConfidencePII should return false for plain text")
	}
}

func TestRedactor_MapRedaction(t *testing.T) {
	r := NewRedactor(DefaultConfig())

	data := map[string]string{
		"email":   "test@example.com",
		"message": "Call 555-123-4567",
		"safe":    "Hello world",
	}

	result := r.RedactMap(data)

	// Email should be redacted
	if strings.Contains(result["email"], "test@example.com") {
		t.Errorf("email should be redacted: %s", result["email"])
	}

	// Phone should be redacted
	if strings.Contains(result["message"], "555-123") {
		t.Errorf("phone should be redacted: %s", result["message"])
	}

	// Safe text unchanged
	if result["safe"] != "Hello world" {
		t.Errorf("safe text should be unchanged: %s", result["safe"])
	}
}

func TestConfig_Serialization(t *testing.T) {
	config := DefaultConfig()
	config.AddCustomPattern("test", `TEST-[0-9]+`, StrategyMask)
	config.AddToAllowlist("allowed@example.com")

	// Serialize
	json, err := config.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Deserialize
	config2, err := ConfigFromJSON(json)
	if err != nil {
		t.Fatalf("ConfigFromJSON failed: %v", err)
	}

	if len(config2.CustomPatterns) != 1 {
		t.Errorf("Expected 1 custom pattern, got %d", len(config2.CustomPatterns))
	}

	if len(config2.Allowlist) != 1 {
		t.Errorf("Expected 1 allowlist entry, got %d", len(config2.Allowlist))
	}
}

func NewPersistentTokenDB(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath)
}

func BenchmarkDetector(b *testing.B) {
	d := NewDetector(DefaultConfig())
	text := "User john@example.com called 555-123-4567 with card 4111111111111111 from 192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Detect(text)
	}
}

func BenchmarkRedactor(b *testing.B) {
	r := NewRedactor(DefaultConfig())
	text := "User john@example.com called 555-123-4567 with card 4111111111111111"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Redact(text)
	}
}

// Helper to clean up test databases
func cleanup(t *testing.T, paths ...string) {
	for _, p := range paths {
		os.Remove(p)
	}
}
