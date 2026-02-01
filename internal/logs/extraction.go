// Package logs provides automatic log field extraction with pattern learning,
// type inference, and Grok pattern support.
package logs

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FieldType represents the inferred type of a field
type FieldType string

const (
	FieldTypeString    FieldType = "string"
	FieldTypeInt       FieldType = "int"
	FieldTypeFloat     FieldType = "float"
	FieldTypeBool      FieldType = "bool"
	FieldTypeTimestamp FieldType = "timestamp"
	FieldTypeIP        FieldType = "ip"
	FieldTypeURL       FieldType = "url"
	FieldTypeEmail     FieldType = "email"
	FieldTypePath      FieldType = "path"
	FieldTypeUUID      FieldType = "uuid"
	FieldTypeDuration  FieldType = "duration"
	FieldTypeBytes     FieldType = "bytes"
	FieldTypeJSON      FieldType = "json"
	FieldTypeUnknown   FieldType = "unknown"
)

// ExtractedField represents a field extracted from a log message
type ExtractedField struct {
	Name       string      `json:"name"`
	Value      interface{} `json:"value"`
	RawValue   string      `json:"raw_value"`
	Type       FieldType   `json:"type"`
	Confidence float64     `json:"confidence"` // 0-1 confidence in type inference
	Source     string      `json:"source"`     // extraction method: json, kv, grok, regex
}

// ExtractionPattern represents a learned or configured extraction pattern
type ExtractionPattern struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Type          string            `json:"type"` // json, kv, grok, apache, nginx, syslog, custom
	Pattern       string            `json:"pattern,omitempty"` // Regex or Grok pattern
	FieldMappings map[string]string `json:"field_mappings,omitempty"` // Pattern group -> field name
	Enabled       bool              `json:"enabled"`
	Priority      int               `json:"priority"` // Higher = tried first
	MatchCount    int64             `json:"match_count"`
	LastMatch     time.Time         `json:"last_match"`
	CreatedAt     time.Time         `json:"created_at"`
	Source        string            `json:"source"` // auto-learned, user-defined, builtin
}

// SourcePatternStats tracks pattern usage per log source
type SourcePatternStats struct {
	Source         string    `json:"source"`
	PatternID      string    `json:"pattern_id"`
	MatchCount     int64     `json:"match_count"`
	SuccessRate    float64   `json:"success_rate"`
	AvgFieldsFound float64   `json:"avg_fields_found"`
	LastUsed       time.Time `json:"last_used"`
}

// FieldExtractor handles automatic field extraction
type FieldExtractor struct {
	// Pattern storage
	patterns       map[string]*ExtractionPattern
	sourcePatterns map[string][]string // source -> ordered pattern IDs

	// Compiled patterns
	compiledGrok    map[string]*regexp.Regexp
	compiledRegex   map[string]*regexp.Regexp

	// Learning store
	learningStore   *PatternLearningStore

	// Built-in patterns
	builtinPatterns map[string]*ExtractionPattern

	// Type inference patterns
	typePatterns map[FieldType]*regexp.Regexp

	// Stats
	extractionsTotal int64
	fieldsExtracted  int64

	mu sync.RWMutex
}

// GrokPattern represents a Grok pattern definition
type GrokPattern struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

// NewFieldExtractor creates a new field extractor
func NewFieldExtractor() *FieldExtractor {
	fe := &FieldExtractor{
		patterns:        make(map[string]*ExtractionPattern),
		sourcePatterns:  make(map[string][]string),
		compiledGrok:    make(map[string]*regexp.Regexp),
		compiledRegex:   make(map[string]*regexp.Regexp),
		builtinPatterns: make(map[string]*ExtractionPattern),
		typePatterns:    make(map[FieldType]*regexp.Regexp),
	}

	// Initialize type inference patterns
	fe.initTypePatterns()

	// Initialize built-in patterns
	fe.initBuiltinPatterns()

	return fe
}

// initTypePatterns initializes patterns for type inference
func (fe *FieldExtractor) initTypePatterns() {
	fe.typePatterns = map[FieldType]*regexp.Regexp{
		FieldTypeInt:       regexp.MustCompile(`^-?\d+$`),
		FieldTypeFloat:     regexp.MustCompile(`^-?\d+\.\d+$`),
		FieldTypeBool:      regexp.MustCompile(`^(?i)(true|false|yes|no|on|off|1|0)$`),
		FieldTypeIP:        regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`),
		FieldTypeUUID:      regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`),
		FieldTypeEmail:     regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`),
		FieldTypeURL:       regexp.MustCompile(`^https?://[^\s]+$`),
		FieldTypePath:      regexp.MustCompile(`^(/[^/\s]+)+/?$`),
		FieldTypeDuration:  regexp.MustCompile(`^\d+(?:\.\d+)?(?:ns|us|ms|s|m|h|d)$`),
		FieldTypeBytes:     regexp.MustCompile(`^\d+(?:\.\d+)?(?:B|KB|MB|GB|TB|PB)$`),
		FieldTypeTimestamp: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(?:T|\s)\d{2}:\d{2}:\d{2}`),
	}
}

// initBuiltinPatterns initializes built-in extraction patterns
func (fe *FieldExtractor) initBuiltinPatterns() {
	// JSON pattern
	fe.builtinPatterns["json"] = &ExtractionPattern{
		ID:       "builtin-json",
		Name:     "JSON",
		Type:     "json",
		Enabled:  true,
		Priority: 100,
		Source:   "builtin",
	}

	// Key-value pattern
	fe.builtinPatterns["kv"] = &ExtractionPattern{
		ID:       "builtin-kv",
		Name:     "Key-Value Pairs",
		Type:     "kv",
		Pattern:  `(\w+)=("[^"]*"|'[^']*'|[^\s]+)`,
		Enabled:  true,
		Priority: 90,
		Source:   "builtin",
	}
	fe.compiledRegex["builtin-kv"] = regexp.MustCompile(`(\w+)=("[^"]*"|'[^']*'|[^\s]+)`)

	// Apache Combined Log Format
	apachePattern := `^(?P<client_ip>\S+)\s+\S+\s+(?P<user>\S+)\s+\[(?P<timestamp>[^\]]+)\]\s+"(?P<method>\S+)\s+(?P<path>\S+)\s+(?P<protocol>[^"]+)"\s+(?P<status>\d+)\s+(?P<bytes>\d+|-)\s+"(?P<referer>[^"]*)"\s+"(?P<user_agent>[^"]*)"`
	fe.builtinPatterns["apache-combined"] = &ExtractionPattern{
		ID:       "builtin-apache-combined",
		Name:     "Apache Combined Log",
		Type:     "apache",
		Pattern:  apachePattern,
		Enabled:  true,
		Priority: 80,
		Source:   "builtin",
		FieldMappings: map[string]string{
			"client_ip":  "client_ip",
			"user":       "user",
			"timestamp":  "timestamp",
			"method":     "http_method",
			"path":       "request_path",
			"protocol":   "http_version",
			"status":     "status_code",
			"bytes":      "response_bytes",
			"referer":    "referer",
			"user_agent": "user_agent",
		},
	}
	fe.compiledRegex["builtin-apache-combined"] = regexp.MustCompile(apachePattern)

	// Nginx Log Format
	nginxPattern := `^(?P<client_ip>\S+)\s+-\s+(?P<user>\S+)\s+\[(?P<timestamp>[^\]]+)\]\s+"(?P<request>[^"]+)"\s+(?P<status>\d+)\s+(?P<bytes>\d+)\s+"(?P<referer>[^"]*)"\s+"(?P<user_agent>[^"]*)"\s*(?:"(?P<forwarded>[^"]*)")?`
	fe.builtinPatterns["nginx"] = &ExtractionPattern{
		ID:       "builtin-nginx",
		Name:     "Nginx Log",
		Type:     "nginx",
		Pattern:  nginxPattern,
		Enabled:  true,
		Priority: 80,
		Source:   "builtin",
	}
	fe.compiledRegex["builtin-nginx"] = regexp.MustCompile(nginxPattern)

	// Syslog RFC3164
	syslogPattern := `^<(?P<priority>\d+)>(?P<timestamp>\w{3}\s+\d+\s+\d+:\d+:\d+)\s+(?P<hostname>\S+)\s+(?P<program>[^\[:\s]+)(?:\[(?P<pid>\d+)\])?:\s*(?P<message>.*)`
	fe.builtinPatterns["syslog"] = &ExtractionPattern{
		ID:       "builtin-syslog",
		Name:     "Syslog RFC3164",
		Type:     "syslog",
		Pattern:  syslogPattern,
		Enabled:  true,
		Priority: 70,
		Source:   "builtin",
	}
	fe.compiledRegex["builtin-syslog"] = regexp.MustCompile(syslogPattern)

	// Common log patterns
	commonPatterns := []struct {
		id      string
		name    string
		pattern string
		ptype   string
	}{
		{
			id:      "builtin-logfmt",
			name:    "Logfmt",
			pattern: `(\w+)=(?:"([^"]*)"|(\S+))`,
			ptype:   "logfmt",
		},
		{
			id:      "builtin-error-trace",
			name:    "Error with Stack Trace",
			pattern: `(?P<level>ERROR|FATAL|WARN|WARNING)\s*(?:\[(?P<component>[^\]]+)\])?\s*(?P<message>[^:]+)(?::\s*(?P<detail>.+))?`,
			ptype:   "error",
		},
		{
			id:      "builtin-java-exception",
			name:    "Java Exception",
			pattern: `(?P<exception>[A-Za-z.]+Exception)(?::\s*(?P<message>[^\n]+))?\n?\s*at\s+(?P<location>[^\n]+)`,
			ptype:   "java-exception",
		},
	}

	for _, cp := range commonPatterns {
		fe.builtinPatterns[cp.id] = &ExtractionPattern{
			ID:       cp.id,
			Name:     cp.name,
			Type:     cp.ptype,
			Pattern:  cp.pattern,
			Enabled:  true,
			Priority: 60,
			Source:   "builtin",
		}
		if re, err := regexp.Compile(cp.pattern); err == nil {
			fe.compiledRegex[cp.id] = re
		}
	}
}

// Extract extracts fields from a log message
func (fe *FieldExtractor) Extract(message string, source string) []ExtractedField {
	fe.mu.RLock()
	defer fe.mu.RUnlock()

	fe.extractionsTotal++

	var fields []ExtractedField

	// Try source-specific patterns first
	if patterns, ok := fe.sourcePatterns[source]; ok {
		for _, patternID := range patterns {
			if pattern, ok := fe.patterns[patternID]; ok && pattern.Enabled {
				extracted := fe.applyPattern(message, pattern)
				if len(extracted) > 0 {
					fields = append(fields, extracted...)
					break // Use first matching pattern
				}
			}
		}
	}

	// Fall back to built-in patterns if no fields extracted
	if len(fields) == 0 {
		fields = fe.extractWithBuiltins(message)
	}

	// Infer types for all extracted fields
	for i := range fields {
		fields[i].Type, fields[i].Confidence = fe.inferType(fields[i].RawValue)
		fields[i].Value = fe.parseValue(fields[i].RawValue, fields[i].Type)
	}

	fe.fieldsExtracted += int64(len(fields))

	return fields
}

// extractWithBuiltins tries all built-in patterns
func (fe *FieldExtractor) extractWithBuiltins(message string) []ExtractedField {
	// Try JSON first
	if fields := fe.extractJSON(message); len(fields) > 0 {
		return fields
	}

	// Try key-value pairs
	if fields := fe.extractKeyValue(message); len(fields) > 0 {
		return fields
	}

	// Try built-in patterns in priority order
	patterns := make([]*ExtractionPattern, 0, len(fe.builtinPatterns))
	for _, p := range fe.builtinPatterns {
		if p.Enabled && p.Type != "json" && p.Type != "kv" {
			patterns = append(patterns, p)
		}
	}
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Priority > patterns[j].Priority
	})

	for _, pattern := range patterns {
		if fields := fe.applyPattern(message, pattern); len(fields) > 0 {
			return fields
		}
	}

	return nil
}

// extractJSON extracts fields from JSON-formatted messages
func (fe *FieldExtractor) extractJSON(message string) []ExtractedField {
	// Quick check for JSON-like structure
	message = strings.TrimSpace(message)
	if !strings.HasPrefix(message, "{") || !strings.HasSuffix(message, "}") {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(message), &data); err != nil {
		return nil
	}

	return fe.flattenJSON(data, "", "json")
}

// flattenJSON flattens nested JSON into fields
func (fe *FieldExtractor) flattenJSON(data map[string]interface{}, prefix, source string) []ExtractedField {
	var fields []ExtractedField

	for key, value := range data {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// Recursively flatten nested objects
			fields = append(fields, fe.flattenJSON(v, fieldName, source)...)
		case []interface{}:
			// Store arrays as JSON
			rawValue, _ := json.Marshal(v)
			fields = append(fields, ExtractedField{
				Name:     fieldName,
				RawValue: string(rawValue),
				Source:   source,
			})
		default:
			rawValue := fmt.Sprintf("%v", value)
			fields = append(fields, ExtractedField{
				Name:     fieldName,
				Value:    value,
				RawValue: rawValue,
				Source:   source,
			})
		}
	}

	return fields
}

// extractKeyValue extracts key=value pairs from messages
func (fe *FieldExtractor) extractKeyValue(message string) []ExtractedField {
	re := fe.compiledRegex["builtin-kv"]
	if re == nil {
		return nil
	}

	matches := re.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return nil
	}

	fields := make([]ExtractedField, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 3 {
			key := match[1]
			value := match[2]

			// Remove quotes if present
			value = strings.Trim(value, `"'`)

			fields = append(fields, ExtractedField{
				Name:     key,
				RawValue: value,
				Source:   "kv",
			})
		}
	}

	return fields
}

// applyPattern applies a specific pattern to extract fields
func (fe *FieldExtractor) applyPattern(message string, pattern *ExtractionPattern) []ExtractedField {
	re, ok := fe.compiledRegex[pattern.ID]
	if !ok {
		// Try to compile it
		var err error
		re, err = regexp.Compile(pattern.Pattern)
		if err != nil {
			return nil
		}
		fe.compiledRegex[pattern.ID] = re
	}

	match := re.FindStringSubmatch(message)
	if match == nil {
		return nil
	}

	names := re.SubexpNames()
	fields := make([]ExtractedField, 0, len(names)-1)

	for i, name := range names {
		if i == 0 || name == "" {
			continue
		}

		fieldName := name
		if mapping, ok := pattern.FieldMappings[name]; ok {
			fieldName = mapping
		}

		fields = append(fields, ExtractedField{
			Name:     fieldName,
			RawValue: match[i],
			Source:   pattern.Type,
		})
	}

	// Update pattern stats
	pattern.MatchCount++
	pattern.LastMatch = time.Now()

	return fields
}

// inferType infers the type of a value
func (fe *FieldExtractor) inferType(value string) (FieldType, float64) {
	value = strings.TrimSpace(value)
	if value == "" {
		return FieldTypeString, 1.0
	}

	// Check specific types in order of specificity
	typeOrder := []FieldType{
		FieldTypeUUID,
		FieldTypeEmail,
		FieldTypeURL,
		FieldTypeIP,
		FieldTypeTimestamp,
		FieldTypeDuration,
		FieldTypeBytes,
		FieldTypeBool,
		FieldTypeFloat,
		FieldTypeInt,
		FieldTypePath,
	}

	for _, t := range typeOrder {
		if pattern, ok := fe.typePatterns[t]; ok {
			if pattern.MatchString(value) {
				return t, 0.95
			}
		}
	}

	// Check for JSON
	if (strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}")) ||
		(strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]")) {
		var js interface{}
		if json.Unmarshal([]byte(value), &js) == nil {
			return FieldTypeJSON, 0.9
		}
	}

	return FieldTypeString, 1.0
}

// parseValue parses a string value into its typed value
func (fe *FieldExtractor) parseValue(value string, fieldType FieldType) interface{} {
	switch fieldType {
	case FieldTypeInt:
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			return v
		}
	case FieldTypeFloat:
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	case FieldTypeBool:
		lower := strings.ToLower(value)
		return lower == "true" || lower == "yes" || lower == "on" || lower == "1"
	case FieldTypeJSON:
		var v interface{}
		if json.Unmarshal([]byte(value), &v) == nil {
			return v
		}
	}
	return value
}

// AddPattern adds a new extraction pattern
func (fe *FieldExtractor) AddPattern(pattern *ExtractionPattern) error {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	if pattern.ID == "" {
		pattern.ID = fe.generatePatternID(pattern.Pattern)
	}
	pattern.CreatedAt = time.Now()

	// Compile the pattern
	if pattern.Pattern != "" {
		grokExpanded := fe.expandGrokPattern(pattern.Pattern)
		re, err := regexp.Compile(grokExpanded)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
		fe.compiledRegex[pattern.ID] = re
	}

	fe.patterns[pattern.ID] = pattern

	return nil
}

// RemovePattern removes an extraction pattern
func (fe *FieldExtractor) RemovePattern(id string) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	delete(fe.patterns, id)
	delete(fe.compiledRegex, id)

	// Remove from source mappings
	for source, patterns := range fe.sourcePatterns {
		newPatterns := make([]string, 0, len(patterns))
		for _, p := range patterns {
			if p != id {
				newPatterns = append(newPatterns, p)
			}
		}
		fe.sourcePatterns[source] = newPatterns
	}
}

// SetSourcePattern associates a pattern with a log source
func (fe *FieldExtractor) SetSourcePattern(source string, patternID string, priority int) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	patterns := fe.sourcePatterns[source]

	// Remove if already exists
	newPatterns := make([]string, 0, len(patterns)+1)
	for _, p := range patterns {
		if p != patternID {
			newPatterns = append(newPatterns, p)
		}
	}

	// Insert at position based on priority (higher priority first)
	inserted := false
	for i, p := range newPatterns {
		if existing, ok := fe.patterns[p]; ok && existing.Priority < priority {
			// Insert before this one
			newPatterns = append(newPatterns[:i], append([]string{patternID}, newPatterns[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		newPatterns = append(newPatterns, patternID)
	}

	fe.sourcePatterns[source] = newPatterns
}

// GetPatterns returns all patterns
func (fe *FieldExtractor) GetPatterns() []*ExtractionPattern {
	fe.mu.RLock()
	defer fe.mu.RUnlock()

	result := make([]*ExtractionPattern, 0, len(fe.patterns)+len(fe.builtinPatterns))

	for _, p := range fe.builtinPatterns {
		copy := *p
		result = append(result, &copy)
	}

	for _, p := range fe.patterns {
		copy := *p
		result = append(result, &copy)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// generatePatternID generates a unique ID for a pattern
func (fe *FieldExtractor) generatePatternID(pattern string) string {
	hash := sha256.Sum256([]byte(pattern + time.Now().String()))
	return "pattern-" + hex.EncodeToString(hash[:8])
}

// GrokPatterns contains common Grok patterns
var GrokPatterns = map[string]string{
	"USERNAME":     `[a-zA-Z0-9._-]+`,
	"USER":         `%{USERNAME}`,
	"INT":          `[+-]?[0-9]+`,
	"BASE10NUM":    `[+-]?(?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+)`,
	"NUMBER":       `%{BASE10NUM}`,
	"POSINT":       `\b[1-9][0-9]*\b`,
	"NONNEGINT":    `\b[0-9]+\b`,
	"WORD":         `\w+`,
	"NOTSPACE":     `\S+`,
	"SPACE":        `\s*`,
	"DATA":         `.*?`,
	"GREEDYDATA":   `.*`,
	"QUOTEDSTRING": `"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`,
	"UUID":         `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`,
	"MAC":          `(?:[0-9a-fA-F]{2}:){5}[0-9a-fA-F]{2}`,
	"IP":           `(?:\d{1,3}\.){3}\d{1,3}`,
	"IPV6":         `[0-9a-fA-F:]{7,}`,
	"IPORHOST":     `%{IP}|%{HOSTNAME}`,
	"HOSTNAME":     `\b[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?\b`,
	"HOSTPORT":     `%{IPORHOST}:%{POSINT}`,
	"PATH":         `(?:/[^/\s]*)+`,
	"UNIXPATH":     `(?:/[\w_%!$@:.,-]?/?)(?:\S+)?`,
	"WINPATH":      `(?:[a-zA-Z]:\\[^\\?*]*)`,
	"URIPATH":      `(?:/[A-Za-z0-9$.+!*'(){},~:;=@#%&_\-]*)+`,
	"URIPARAM":     `\?[A-Za-z0-9$.+!*'|(){},~@#%&/=:;_?\-\[\]<>]*`,
	"URIPATHPARAM": `%{URIPATH}(?:%{URIPARAM})?`,
	"URI":          `%{URIPROTO}://(?:%{USER}(?::[^@]*)?@)?(?:%{URIHOST})?(?:%{URIPATHPARAM})?`,
	"URIPROTO":     `[a-zA-Z][a-zA-Z0-9+.-]*`,
	"URIHOST":      `%{IPORHOST}(?::%{POSINT})?`,
	"MONTH":        `Jan(?:uary)?|Feb(?:ruary)?|Mar(?:ch)?|Apr(?:il)?|May|Jun(?:e)?|Jul(?:y)?|Aug(?:ust)?|Sep(?:tember)?|Oct(?:ober)?|Nov(?:ember)?|Dec(?:ember)?`,
	"MONTHNUM":     `0?[1-9]|1[0-2]`,
	"MONTHDAY":     `(?:0?[1-9]|[12][0-9]|3[01])`,
	"DAY":          `Mon(?:day)?|Tue(?:sday)?|Wed(?:nesday)?|Thu(?:rsday)?|Fri(?:day)?|Sat(?:urday)?|Sun(?:day)?`,
	"YEAR":         `(?:\d\d){1,2}`,
	"HOUR":         `2[0123]|[01]?[0-9]`,
	"MINUTE":       `[0-5][0-9]`,
	"SECOND":       `(?:[0-5]?[0-9]|60)(?:[:.,][0-9]+)?`,
	"TIME":         `%{HOUR}:%{MINUTE}:%{SECOND}`,
	"DATE_US":      `%{MONTHNUM}[/-]%{MONTHDAY}[/-]%{YEAR}`,
	"DATE_EU":      `%{MONTHDAY}[./-]%{MONTHNUM}[./-]%{YEAR}`,
	"ISO8601_TIMEZONE": `Z|[+-]%{HOUR}(?::?%{MINUTE})?`,
	"ISO8601_SECOND":   `%{SECOND}|60`,
	"TIMESTAMP_ISO8601": `%{YEAR}-%{MONTHNUM}-%{MONTHDAY}[T ]%{HOUR}:?%{MINUTE}(?::?%{SECOND})?%{ISO8601_TIMEZONE}?`,
	"DATESTAMP":    `%{DATE_US}[- ]%{TIME}`,
	"LOGLEVEL":     `[Dd]ebug|DEBUG|[Ii]nfo|INFO|[Ww]arn(?:ing)?|WARN(?:ING)?|[Ee]rror|ERROR|[Ff]atal|FATAL|[Ss]evere|SEVERE|TRACE`,
	"HTTPDATE":     `%{MONTHDAY}/%{MONTH}/%{YEAR}:%{TIME} %{INT}`,
}

// expandGrokPattern expands Grok patterns to regex
func (fe *FieldExtractor) expandGrokPattern(pattern string) string {
	// Replace %{PATTERN_NAME:field_name} with (?P<field_name>pattern)
	// Replace %{PATTERN_NAME} with (pattern)

	re := regexp.MustCompile(`%\{(\w+)(?::(\w+))?\}`)

	maxIterations := 100 // Prevent infinite loops
	for i := 0; i < maxIterations; i++ {
		newPattern := re.ReplaceAllStringFunc(pattern, func(match string) string {
			parts := re.FindStringSubmatch(match)
			if len(parts) < 2 {
				return match
			}

			patternName := parts[1]
			fieldName := ""
			if len(parts) > 2 {
				fieldName = parts[2]
			}

			replacement, ok := GrokPatterns[patternName]
			if !ok {
				return match // Keep original if not found
			}

			if fieldName != "" {
				return fmt.Sprintf("(?P<%s>%s)", fieldName, replacement)
			}
			return fmt.Sprintf("(?:%s)", replacement)
		})

		if newPattern == pattern {
			break // No more substitutions
		}
		pattern = newPattern
	}

	return pattern
}

// PatternLearningStore persists learned patterns
type PatternLearningStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewPatternLearningStore creates a new pattern learning store
func NewPatternLearningStore(db *sql.DB) (*PatternLearningStore, error) {
	store := &PatternLearningStore{db: db}

	schema := `
	CREATE TABLE IF NOT EXISTS extraction_patterns (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		pattern TEXT,
		field_mappings TEXT,
		enabled INTEGER DEFAULT 1,
		priority INTEGER DEFAULT 0,
		match_count INTEGER DEFAULT 0,
		last_match INTEGER,
		created_at INTEGER,
		source TEXT
	);

	CREATE TABLE IF NOT EXISTS source_pattern_stats (
		source TEXT NOT NULL,
		pattern_id TEXT NOT NULL,
		match_count INTEGER DEFAULT 0,
		success_rate REAL DEFAULT 0,
		avg_fields REAL DEFAULT 0,
		last_used INTEGER,
		PRIMARY KEY (source, pattern_id)
	);

	CREATE TABLE IF NOT EXISTS learned_field_types (
		field_name TEXT NOT NULL,
		source TEXT NOT NULL,
		inferred_type TEXT NOT NULL,
		sample_count INTEGER DEFAULT 0,
		confidence REAL DEFAULT 0,
		last_updated INTEGER,
		PRIMARY KEY (field_name, source)
	);

	CREATE INDEX IF NOT EXISTS idx_patterns_source ON extraction_patterns(source);
	CREATE INDEX IF NOT EXISTS idx_stats_source ON source_pattern_stats(source);
	`

	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return store, nil
}

// SavePattern saves a pattern to the store
func (s *PatternLearningStore) SavePattern(pattern *ExtractionPattern) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mappingsJSON, _ := json.Marshal(pattern.FieldMappings)

	_, err := s.db.Exec(`
		INSERT INTO extraction_patterns
		(id, name, type, pattern, field_mappings, enabled, priority, match_count, last_match, created_at, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			pattern = excluded.pattern,
			field_mappings = excluded.field_mappings,
			enabled = excluded.enabled,
			priority = excluded.priority,
			match_count = excluded.match_count,
			last_match = excluded.last_match,
			source = excluded.source
	`,
		pattern.ID, pattern.Name, pattern.Type, pattern.Pattern, string(mappingsJSON),
		pattern.Enabled, pattern.Priority, pattern.MatchCount,
		pattern.LastMatch.Unix(), pattern.CreatedAt.Unix(), pattern.Source,
	)
	return err
}

// LoadPatterns loads all patterns from the store
func (s *PatternLearningStore) LoadPatterns() ([]*ExtractionPattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, type, pattern, field_mappings, enabled, priority,
		       match_count, last_match, created_at, source
		FROM extraction_patterns
		ORDER BY priority DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []*ExtractionPattern
	for rows.Next() {
		var p ExtractionPattern
		var mappingsJSON string
		var lastMatch, createdAt int64
		var enabled int

		err := rows.Scan(
			&p.ID, &p.Name, &p.Type, &p.Pattern, &mappingsJSON,
			&enabled, &p.Priority, &p.MatchCount, &lastMatch, &createdAt, &p.Source,
		)
		if err != nil {
			continue
		}

		p.Enabled = enabled == 1
		p.LastMatch = time.Unix(lastMatch, 0)
		p.CreatedAt = time.Unix(createdAt, 0)
		json.Unmarshal([]byte(mappingsJSON), &p.FieldMappings)

		patterns = append(patterns, &p)
	}

	return patterns, nil
}

// RecordSourcePatternUsage records usage of a pattern for a source
func (s *PatternLearningStore) RecordSourcePatternUsage(source, patternID string, fieldsFound int, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current stats
	var matchCount int64
	var successRate, avgFields float64
	err := s.db.QueryRow(`
		SELECT match_count, success_rate, avg_fields
		FROM source_pattern_stats
		WHERE source = ? AND pattern_id = ?
	`, source, patternID).Scan(&matchCount, &successRate, &avgFields)

	if err == sql.ErrNoRows {
		matchCount = 0
		successRate = 0
		avgFields = 0
	} else if err != nil {
		return err
	}

	// Update stats
	matchCount++
	if success {
		successRate = (successRate*float64(matchCount-1) + 1) / float64(matchCount)
	} else {
		successRate = (successRate * float64(matchCount-1)) / float64(matchCount)
	}
	avgFields = (avgFields*float64(matchCount-1) + float64(fieldsFound)) / float64(matchCount)

	_, err = s.db.Exec(`
		INSERT INTO source_pattern_stats (source, pattern_id, match_count, success_rate, avg_fields, last_used)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(source, pattern_id) DO UPDATE SET
			match_count = excluded.match_count,
			success_rate = excluded.success_rate,
			avg_fields = excluded.avg_fields,
			last_used = excluded.last_used
	`, source, patternID, matchCount, successRate, avgFields, time.Now().Unix())

	return err
}

// GetBestPatternForSource returns the best pattern for a given source
func (s *PatternLearningStore) GetBestPatternForSource(source string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var patternID string
	err := s.db.QueryRow(`
		SELECT pattern_id FROM source_pattern_stats
		WHERE source = ?
		ORDER BY success_rate DESC, match_count DESC
		LIMIT 1
	`, source).Scan(&patternID)

	if err == sql.ErrNoRows {
		return "", nil
	}
	return patternID, err
}

// SetLearningStore sets the pattern learning store
func (fe *FieldExtractor) SetLearningStore(store *PatternLearningStore) {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.learningStore = store

	// Load existing patterns
	if patterns, err := store.LoadPatterns(); err == nil {
		for _, p := range patterns {
			fe.patterns[p.ID] = p
			// Compile pattern
			if p.Pattern != "" {
				grokExpanded := fe.expandGrokPattern(p.Pattern)
				if re, err := regexp.Compile(grokExpanded); err == nil {
					fe.compiledRegex[p.ID] = re
				}
			}
		}
	}
}

// LearnFromMessage learns patterns from a message
func (fe *FieldExtractor) LearnFromMessage(message, source string) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	if fe.learningStore == nil {
		return
	}

	// Try to extract with current patterns and record success
	for patternID, pattern := range fe.patterns {
		if !pattern.Enabled {
			continue
		}
		fields := fe.applyPattern(message, pattern)
		success := len(fields) > 0
		fe.learningStore.RecordSourcePatternUsage(source, patternID, len(fields), success)
	}
}

// GetStats returns extraction statistics
func (fe *FieldExtractor) GetStats() ExtractionStats {
	fe.mu.RLock()
	defer fe.mu.RUnlock()

	stats := ExtractionStats{
		TotalExtractions: fe.extractionsTotal,
		TotalFields:      fe.fieldsExtracted,
		PatternCount:     len(fe.patterns) + len(fe.builtinPatterns),
		SourcePatterns:   make(map[string]int),
	}

	for source, patterns := range fe.sourcePatterns {
		stats.SourcePatterns[source] = len(patterns)
	}

	if fe.extractionsTotal > 0 {
		stats.AvgFieldsPerExtraction = float64(fe.fieldsExtracted) / float64(fe.extractionsTotal)
	}

	return stats
}

// ExtractionStats provides extraction statistics
type ExtractionStats struct {
	TotalExtractions       int64          `json:"total_extractions"`
	TotalFields            int64          `json:"total_fields"`
	AvgFieldsPerExtraction float64        `json:"avg_fields_per_extraction"`
	PatternCount           int            `json:"pattern_count"`
	SourcePatterns         map[string]int `json:"source_patterns"`
}

// ExtractAndEnrich extracts fields and enriches a log entry
func (fe *FieldExtractor) ExtractAndEnrich(entry *LogEntry) {
	fields := fe.Extract(entry.Message, entry.Service)

	if entry.Attrs == nil {
		entry.Attrs = make(map[string]string)
	}

	for _, field := range fields {
		// Don't overwrite existing attributes
		if _, exists := entry.Attrs[field.Name]; !exists {
			switch v := field.Value.(type) {
			case string:
				entry.Attrs[field.Name] = v
			default:
				if jsonBytes, err := json.Marshal(v); err == nil {
					entry.Attrs[field.Name] = string(jsonBytes)
				} else {
					entry.Attrs[field.Name] = field.RawValue
				}
			}
		}
	}

	// Add extraction metadata
	entry.Attrs["_extracted_fields"] = fmt.Sprintf("%d", len(fields))
}

// ValidateGrokPattern validates a Grok pattern
func ValidateGrokPattern(pattern string) error {
	fe := NewFieldExtractor()
	expanded := fe.expandGrokPattern(pattern)
	_, err := regexp.Compile(expanded)
	if err != nil {
		return fmt.Errorf("invalid pattern after expansion: %w", err)
	}
	return nil
}

// GetGrokPatterns returns available Grok patterns
func GetGrokPatterns() map[string]string {
	result := make(map[string]string, len(GrokPatterns))
	for k, v := range GrokPatterns {
		result[k] = v
	}
	return result
}

// AddPatternValue adds a new extraction pattern by value (for API)
func (fe *FieldExtractor) AddPatternValue(pattern ExtractionPattern) error {
	return fe.AddPattern(&pattern)
}

// RemovePatternByName removes an extraction pattern by name (for API)
func (fe *FieldExtractor) RemovePatternByName(name string) error {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	// Find pattern by name
	var foundID string
	for id, p := range fe.patterns {
		if p.Name == name || p.ID == name {
			foundID = id
			break
		}
	}

	if foundID == "" {
		return fmt.Errorf("pattern not found: %s", name)
	}

	delete(fe.patterns, foundID)
	delete(fe.compiledRegex, foundID)

	// Remove from source mappings
	for source, patterns := range fe.sourcePatterns {
		newPatterns := make([]string, 0, len(patterns))
		for _, p := range patterns {
			if p != foundID {
				newPatterns = append(newPatterns, p)
			}
		}
		fe.sourcePatterns[source] = newPatterns
	}

	return nil
}

// GetGrokPatterns returns available Grok patterns (method on FieldExtractor)
func (fe *FieldExtractor) GetGrokPatterns() map[string]string {
	return GetGrokPatterns()
}

// TestGrokPattern tests a Grok pattern against sample messages
func (fe *FieldExtractor) TestGrokPattern(pattern string, messages []string) ([]map[string]interface{}, error) {
	// Expand the grok pattern
	expanded := fe.expandGrokPattern(pattern)

	// Compile it
	re, err := regexp.Compile(expanded)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	results := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		match := re.FindStringSubmatch(msg)
		if match == nil {
			results[i] = map[string]interface{}{
				"message": msg,
				"matched": false,
				"fields":  nil,
			}
			continue
		}

		names := re.SubexpNames()
		fields := make(map[string]string)
		for j, name := range names {
			if j > 0 && name != "" && j < len(match) {
				fields[name] = match[j]
			}
		}

		results[i] = map[string]interface{}{
			"message":  msg,
			"matched":  true,
			"fields":   fields,
			"expanded": expanded,
		}
	}

	return results, nil
}

// Extract extracts fields from a log message (simplified API without source)
func (fe *FieldExtractor) ExtractSimple(message string) []ExtractedField {
	return fe.Extract(message, "")
}

// FieldInfo represents field information with Key alias for Name
type FieldInfo struct {
	Key        string      `json:"key"`
	Value      interface{} `json:"value"`
	Type       FieldType   `json:"type"`
	Source     string      `json:"source"`
	Confidence float64     `json:"confidence"`
}

// ToFieldInfo converts ExtractedField to FieldInfo with Key field
func (f *ExtractedField) ToFieldInfo() FieldInfo {
	return FieldInfo{
		Key:        f.Name,
		Value:      f.Value,
		Type:       f.Type,
		Source:     f.Source,
		Confidence: f.Confidence,
	}
}

// PatternLearningStore methods for API

// GetSources returns all sources with learned patterns
func (s *PatternLearningStore) GetSources() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT DISTINCT source FROM source_pattern_stats ORDER BY source`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var sources []string
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err == nil {
			sources = append(sources, source)
		}
	}
	return sources
}

// GetLearnedPatterns returns patterns for a specific source
func (s *PatternLearningStore) GetLearnedPatterns(source string) []SourcePatternStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT source, pattern_id, match_count, success_rate, avg_fields, last_used
		FROM source_pattern_stats
		WHERE source = ?
		ORDER BY success_rate DESC
	`, source)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var patterns []SourcePatternStats
	for rows.Next() {
		var p SourcePatternStats
		var lastUsed int64
		if err := rows.Scan(&p.Source, &p.PatternID, &p.MatchCount, &p.SuccessRate, &p.AvgFieldsFound, &lastUsed); err == nil {
			p.LastUsed = time.Unix(lastUsed, 0)
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// GetCommonFields returns the most common fields seen for a source
func (s *PatternLearningStore) GetCommonFields(source string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT field_name
		FROM learned_field_types
		WHERE source = ?
		ORDER BY sample_count DESC
		LIMIT 50
	`, source)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var fields []string
	for rows.Next() {
		var field string
		if err := rows.Scan(&field); err == nil {
			fields = append(fields, field)
		}
	}
	return fields
}

// RecordExtraction records an extraction for learning
func (s *PatternLearningStore) RecordExtraction(source, message string, fields []ExtractedField) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, field := range fields {
		_, _ = s.db.Exec(`
			INSERT INTO learned_field_types (field_name, source, inferred_type, sample_count, confidence, last_updated)
			VALUES (?, ?, ?, 1, ?, ?)
			ON CONFLICT(field_name, source) DO UPDATE SET
				sample_count = sample_count + 1,
				confidence = (confidence * sample_count + ?) / (sample_count + 1),
				last_updated = ?
		`, field.Name, source, field.Type, field.Confidence, time.Now().Unix(), field.Confidence, time.Now().Unix())
	}
}
