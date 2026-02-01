package pii

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// RedactionStrategy defines how PII should be redacted
type RedactionStrategy string

const (
	StrategyMask     RedactionStrategy = "mask"     // Replace with ****
	StrategyHash     RedactionStrategy = "hash"     // Replace with hash
	StrategyRemove   RedactionStrategy = "remove"   // Remove entirely
	StrategyTokenize RedactionStrategy = "tokenize" // Replace with reversible token
	StrategyFormat   RedactionStrategy = "format"   // Preserve format e.g., [EMAIL]
)

// Redactor handles PII redaction
type Redactor struct {
	detector   *Detector
	config     *Config
	tokenStore *TokenStore
	mu         sync.RWMutex
}

// NewRedactor creates a new PII redactor
func NewRedactor(config *Config) *Redactor {
	if config == nil {
		config = DefaultConfig()
	}

	return &Redactor{
		detector:   NewDetector(config),
		config:     config,
		tokenStore: NewTokenStore(nil), // In-memory by default
	}
}

// NewRedactorWithTokenStore creates a redactor with a persistent token store
func NewRedactorWithTokenStore(config *Config, store *TokenStore) *Redactor {
	if config == nil {
		config = DefaultConfig()
	}

	return &Redactor{
		detector:   NewDetector(config),
		config:     config,
		tokenStore: store,
	}
}

// Redact processes text and returns redacted version
func (r *Redactor) Redact(text string) string {
	detections := r.detector.Detect(text)
	return r.applyRedactions(text, detections)
}

// RedactWithDetails returns redacted text along with detection details
func (r *Redactor) RedactWithDetails(text string) (string, []Detection) {
	detections := r.detector.Detect(text)
	redacted := r.applyRedactions(text, detections)

	// Update detections with redacted values
	for i := range detections {
		detections[i].Redacted = r.redactValue(detections[i].Type, detections[i].Value)
	}

	return redacted, detections
}

// applyRedactions applies all detections to the text
func (r *Redactor) applyRedactions(text string, detections []Detection) string {
	if len(detections) == 0 {
		return text
	}

	// Sort detections by start position (descending) to process from end to start
	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Start > detections[j].Start
	})

	result := text
	for _, d := range detections {
		replacement := r.redactValue(d.Type, d.Value)
		result = result[:d.Start] + replacement + result[d.End:]
	}

	return result
}

// redactValue applies the appropriate redaction strategy
func (r *Redactor) redactValue(piiType PIIType, value string) string {
	strategy := r.config.GetStrategy(piiType)

	switch strategy {
	case StrategyMask:
		return r.maskValue(piiType, value)
	case StrategyHash:
		return r.hashValue(value)
	case StrategyRemove:
		return ""
	case StrategyTokenize:
		return r.tokenizeValue(piiType, value)
	case StrategyFormat:
		return r.formatValue(piiType)
	default:
		return r.maskValue(piiType, value)
	}
}

// maskValue replaces value with asterisks
func (r *Redactor) maskValue(piiType PIIType, value string) string {
	switch piiType {
	case TypeEmail:
		// Show first char and domain
		parts := strings.Split(value, "@")
		if len(parts) == 2 && len(parts[0]) > 0 {
			return string(parts[0][0]) + "****@" + parts[1]
		}
		return "****@****.***"

	case TypePhone:
		// Show last 4 digits
		if len(value) >= 4 {
			return "***-***-" + value[len(value)-4:]
		}
		return "***-***-****"

	case TypeSSN:
		// Show last 4 digits
		if len(value) >= 4 {
			return "***-**-" + value[len(value)-4:]
		}
		return "***-**-****"

	case TypeCreditCard:
		// Show last 4 digits
		cleaned := strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), "-", "")
		if len(cleaned) >= 4 {
			return "****-****-****-" + cleaned[len(cleaned)-4:]
		}
		return "****-****-****-****"

	case TypeIPAddress:
		// Show network portion
		parts := strings.Split(value, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1] + ".***.***.***"
		}
		return "***.***.***.***"

	case TypeAWSKey, TypeGitHubToken, TypeStripeKey, TypeGenericKey:
		// Show prefix only
		if len(value) > 8 {
			return value[:8] + "************************"
		}
		return "********************************"

	case TypeJWT:
		return "[JWT_TOKEN_REDACTED]"

	case TypePassword:
		return "********"

	default:
		return strings.Repeat("*", len(value))
	}
}

// hashValue replaces value with its SHA256 hash
func (r *Redactor) hashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return "[HASH:" + hex.EncodeToString(hash[:8]) + "]"
}

// tokenizeValue creates a reversible token
func (r *Redactor) tokenizeValue(piiType PIIType, value string) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	token := r.tokenStore.Store(piiType, value)
	return token
}

// formatValue returns a format placeholder
func (r *Redactor) formatValue(piiType PIIType) string {
	return fmt.Sprintf("[%s]", strings.ToUpper(string(piiType)))
}

// DetokenizeToken retrieves the original value for a token
func (r *Redactor) DetokenizeToken(token string) (PIIType, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.tokenStore.Retrieve(token)
}

// DetokenizeText replaces all tokens in text with original values
func (r *Redactor) DetokenizeText(text string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.tokenStore.DetokenizeText(text)
}

// RedactMap redacts PII from a map of string values
func (r *Redactor) RedactMap(data map[string]string) map[string]string {
	result := make(map[string]string, len(data))
	for k, v := range data {
		result[k] = r.Redact(v)
	}
	return result
}

// RedactAttributes redacts span/log attributes
func (r *Redactor) RedactAttributes(attrs map[string]string) map[string]string {
	// Special handling for known attribute names that typically contain PII
	sensitiveKeys := map[string]bool{
		"user.email":          true,
		"user.phone":          true,
		"user.name":           true,
		"user.id":             true,
		"customer.email":      true,
		"customer.phone":      true,
		"http.request.header": true,
		"db.statement":        true,
		"http.url":            true,
		"message":             true,
		"error.message":       true,
	}

	result := make(map[string]string, len(attrs))
	for k, v := range attrs {
		// Always redact sensitive keys
		if sensitiveKeys[k] {
			result[k] = r.Redact(v)
			continue
		}

		// For other keys, only redact if PII detected
		if r.detector.HasPII(v) {
			result[k] = r.Redact(v)
		} else {
			result[k] = v
		}
	}

	return result
}

// GetDetector returns the underlying detector
func (r *Redactor) GetDetector() *Detector {
	return r.detector
}

// UpdateConfig updates the redactor configuration
func (r *Redactor) UpdateConfig(config *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = config
	r.detector = NewDetector(config)
}

// GetConfig returns the current configuration
func (r *Redactor) GetConfig() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.config
}

// Stats returns redaction statistics
func (r *Redactor) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"token_count": r.tokenStore.Count(),
		"config":      r.config,
	}
}
