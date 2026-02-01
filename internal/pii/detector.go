package pii

import (
	"regexp"
	"strings"
)

// PIIType represents the type of PII detected
type PIIType string

const (
	TypeEmail       PIIType = "email"
	TypePhone       PIIType = "phone"
	TypeSSN         PIIType = "ssn"
	TypeCreditCard  PIIType = "credit_card"
	TypeIPAddress   PIIType = "ip_address"
	TypeAWSKey      PIIType = "aws_key"
	TypeGitHubToken PIIType = "github_token"
	TypeStripeKey   PIIType = "stripe_key"
	TypeGenericKey  PIIType = "generic_api_key"
	TypeJWT         PIIType = "jwt"
	TypePassword    PIIType = "password"
)

// Confidence represents detection confidence level
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Detection represents a single PII detection
type Detection struct {
	Type       PIIType    `json:"type"`
	Value      string     `json:"value"`
	Redacted   string     `json:"redacted,omitempty"`
	Start      int        `json:"start"`
	End        int        `json:"end"`
	Confidence Confidence `json:"confidence"`
}

// Detector detects PII in text
type Detector struct {
	config   *Config
	patterns map[PIIType]*regexp.Regexp
}

// NewDetector creates a new PII detector
func NewDetector(config *Config) *Detector {
	if config == nil {
		config = DefaultConfig()
	}

	d := &Detector{
		config:   config,
		patterns: make(map[PIIType]*regexp.Regexp),
	}

	d.compilePatterns()
	return d
}

func (d *Detector) compilePatterns() {
	// Email pattern - RFC 5322 simplified
	d.patterns[TypeEmail] = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

	// Phone patterns - US and international
	d.patterns[TypePhone] = regexp.MustCompile(`\b(?:\+?1[-.\s]?)?(?:\(?[0-9]{3}\)?[-.\s]?)?[0-9]{3}[-.\s]?[0-9]{4}\b|\b\+[0-9]{1,3}[-.\s]?[0-9]{1,4}[-.\s]?[0-9]{1,4}[-.\s]?[0-9]{1,9}\b`)

	// SSN pattern - xxx-xx-xxxx
	d.patterns[TypeSSN] = regexp.MustCompile(`\b[0-9]{3}-[0-9]{2}-[0-9]{4}\b`)

	// Credit card patterns - Visa, MC, Amex, Discover
	d.patterns[TypeCreditCard] = regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12}|(?:4[0-9]{3}|5[1-5][0-9]{2}|6011|3[47][0-9]{2})[-\s]?[0-9]{4}[-\s]?[0-9]{4}[-\s]?[0-9]{4,7})\b`)

	// IP address pattern (IPv4)
	d.patterns[TypeIPAddress] = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

	// AWS Access Key ID
	d.patterns[TypeAWSKey] = regexp.MustCompile(`\b(?:AKIA|ABIA|ACCA|ASIA)[0-9A-Z]{16}\b`)

	// GitHub token patterns
	d.patterns[TypeGitHubToken] = regexp.MustCompile(`\b(?:ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|ghu_[a-zA-Z0-9]{36}|ghs_[a-zA-Z0-9]{36}|ghr_[a-zA-Z0-9]{36}|github_pat_[a-zA-Z0-9]{22}_[a-zA-Z0-9]{59})\b`)

	// Stripe API keys
	d.patterns[TypeStripeKey] = regexp.MustCompile(`\b(?:sk_live_[0-9a-zA-Z]{24,}|pk_live_[0-9a-zA-Z]{24,}|sk_test_[0-9a-zA-Z]{24,}|pk_test_[0-9a-zA-Z]{24,})\b`)

	// Generic API key patterns
	d.patterns[TypeGenericKey] = regexp.MustCompile(`\b(?:api[_-]?key|apikey|secret[_-]?key|access[_-]?token)[=:\s]["']?([a-zA-Z0-9_-]{20,64})["']?`)

	// JWT token pattern
	d.patterns[TypeJWT] = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*\b`)

	// Password in URLs or query strings
	d.patterns[TypePassword] = regexp.MustCompile(`(?i)(?:password|passwd|pwd)[=:\s]["']?([^&\s"']{1,64})["']?`)
}

// Detect scans text for PII and returns all detections
func (d *Detector) Detect(text string) []Detection {
	var detections []Detection

	for piiType, pattern := range d.patterns {
		// Skip disabled types
		if !d.config.IsTypeEnabled(piiType) {
			continue
		}

		matches := pattern.FindAllStringIndex(text, -1)
		for _, match := range matches {
			value := text[match[0]:match[1]]

			// Additional validation for certain types
			confidence := d.validateMatch(piiType, value)
			if confidence == "" {
				continue
			}

			// Check allowlist
			if d.config.IsAllowed(value) {
				continue
			}

			// Check denylist (auto-detect)
			if d.config.IsDenied(value) {
				confidence = ConfidenceHigh
			}

			detections = append(detections, Detection{
				Type:       piiType,
				Value:      value,
				Start:      match[0],
				End:        match[1],
				Confidence: confidence,
			})
		}
	}

	// Add custom pattern detections
	for _, cp := range d.config.CustomPatterns {
		if !cp.Enabled {
			continue
		}

		re, err := regexp.Compile(cp.Pattern)
		if err != nil {
			continue
		}

		matches := re.FindAllStringIndex(text, -1)
		for _, match := range matches {
			value := text[match[0]:match[1]]

			detections = append(detections, Detection{
				Type:       PIIType(cp.Name),
				Value:      value,
				Start:      match[0],
				End:        match[1],
				Confidence: ConfidenceMedium,
			})
		}
	}

	return detections
}

// validateMatch performs additional validation on matches
func (d *Detector) validateMatch(piiType PIIType, value string) Confidence {
	switch piiType {
	case TypeCreditCard:
		// Validate with Luhn algorithm
		if !luhnCheck(value) {
			return ""
		}
		return ConfidenceHigh

	case TypeSSN:
		// SSN should not start with 000, 666, or 900-999
		if strings.HasPrefix(value, "000") || strings.HasPrefix(value, "666") {
			return ""
		}
		if value[0] == '9' {
			return ""
		}
		return ConfidenceHigh

	case TypeIPAddress:
		// Check if it's a private IP (might want to skip in some cases)
		if isPrivateIP(value) {
			return ConfidenceLow
		}
		return ConfidenceMedium

	case TypePhone:
		// Basic validation - at least 10 digits
		digits := countDigits(value)
		if digits < 10 {
			return ""
		}
		if digits == 10 || digits == 11 {
			return ConfidenceHigh
		}
		return ConfidenceMedium

	case TypeEmail:
		return ConfidenceHigh

	case TypeAWSKey, TypeGitHubToken, TypeStripeKey:
		return ConfidenceHigh

	case TypeJWT:
		// Validate JWT structure
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return ""
		}
		return ConfidenceHigh

	case TypePassword, TypeGenericKey:
		return ConfidenceMedium

	default:
		return ConfidenceMedium
	}
}

// luhnCheck validates a credit card number using the Luhn algorithm
func luhnCheck(number string) bool {
	// Remove spaces and dashes
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "-", "")

	if len(number) < 13 || len(number) > 19 {
		return false
	}

	var sum int
	alternate := false

	for i := len(number) - 1; i >= 0; i-- {
		digit := int(number[i] - '0')
		if digit < 0 || digit > 9 {
			return false
		}

		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		alternate = !alternate
	}

	return sum%10 == 0
}

// isPrivateIP checks if an IP address is private
func isPrivateIP(ip string) bool {
	privateRanges := []string{
		"10.",
		"192.168.",
		"127.",
	}

	for _, prefix := range privateRanges {
		if strings.HasPrefix(ip, prefix) {
			return true
		}
	}

	// Check 172.16.0.0 - 172.31.255.255
	if strings.HasPrefix(ip, "172.") {
		parts := strings.Split(ip, ".")
		if len(parts) >= 2 {
			second := parseIntFromString(parts[1])
			if second >= 16 && second <= 31 {
				return true
			}
		}
	}

	return false
}

func parseIntFromString(s string) int {
	result := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return result
		}
		result = result*10 + int(c-'0')
	}
	return result
}

func countDigits(s string) int {
	count := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			count++
		}
	}
	return count
}

// DetectWithContext returns detections with surrounding context
func (d *Detector) DetectWithContext(text string, contextSize int) []Detection {
	detections := d.Detect(text)

	for i := range detections {
		start := detections[i].Start - contextSize
		if start < 0 {
			start = 0
		}
		end := detections[i].End + contextSize
		if end > len(text) {
			end = len(text)
		}

		// Store context in the value field with markers
		detections[i].Value = text[start:end]
	}

	return detections
}

// HasPII returns true if text contains any PII
func (d *Detector) HasPII(text string) bool {
	return len(d.Detect(text)) > 0
}

// HasHighConfidencePII returns true if text contains high-confidence PII
func (d *Detector) HasHighConfidencePII(text string) bool {
	detections := d.Detect(text)
	for _, d := range detections {
		if d.Confidence == ConfidenceHigh {
			return true
		}
	}
	return false
}
