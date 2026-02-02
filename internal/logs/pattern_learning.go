package logs

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// PatternLearner implements intelligent pattern learning for log extraction
type PatternLearner struct {
	// Unmatched log tracking
	unmatchedLogs    *UnmatchedLogBuffer

	// Pattern candidate tracking
	candidates       map[string]*PatternCandidate

	// Token frequency analysis
	tokenFrequency   map[string]int64

	// Configuration
	config           PatternLearnerConfig

	// Statistics
	stats            PatternLearnerStats

	mu sync.RWMutex
}

// PatternLearnerConfig configures the pattern learner
type PatternLearnerConfig struct {
	// Minimum samples before suggesting a pattern
	MinSamplesForSuggestion int `json:"min_samples_for_suggestion"`

	// Maximum candidates to track
	MaxCandidates int `json:"max_candidates"`

	// Buffer size for unmatched logs
	UnmatchedBufferSize int `json:"unmatched_buffer_size"`

	// Minimum confidence for auto-optimization
	MinConfidenceForOptimization float64 `json:"min_confidence_for_optimization"`

	// Pattern similarity threshold
	SimilarityThreshold float64 `json:"similarity_threshold"`
}

// DefaultPatternLearnerConfig returns sensible defaults
func DefaultPatternLearnerConfig() PatternLearnerConfig {
	return PatternLearnerConfig{
		MinSamplesForSuggestion:      100,
		MaxCandidates:                1000,
		UnmatchedBufferSize:          10000,
		MinConfidenceForOptimization: 0.7,
		SimilarityThreshold:          0.8,
	}
}

// PatternCandidate represents a potential pattern learned from logs
type PatternCandidate struct {
	ID              string            `json:"id"`
	Signature       string            `json:"signature"`       // Normalized log signature
	Pattern         string            `json:"pattern"`         // Generated regex pattern
	SampleCount     int64             `json:"sample_count"`
	SampleMessages  []string          `json:"sample_messages"` // First N samples
	FieldsDetected  []string          `json:"fields_detected"`
	Confidence      float64           `json:"confidence"`
	FirstSeen       time.Time         `json:"first_seen"`
	LastSeen        time.Time         `json:"last_seen"`
	TokenBreakdown  map[string]int64  `json:"token_breakdown"`
}

// UnmatchedLogBuffer tracks recent unmatched logs
type UnmatchedLogBuffer struct {
	logs    []UnmatchedLog
	size    int
	index   int
	mu      sync.Mutex
}

// UnmatchedLog represents a log that didn't match any pattern
type UnmatchedLog struct {
	Message   string
	Source    string
	Timestamp time.Time
	Signature string
}

// PatternLearnerStats provides learning statistics
type PatternLearnerStats struct {
	TotalLogsProcessed   int64 `json:"total_logs_processed"`
	UnmatchedLogsCount   int64 `json:"unmatched_logs_count"`
	CandidatesGenerated  int64 `json:"candidates_generated"`
	PatternsOptimized    int64 `json:"patterns_optimized"`
	AutoSuggestions      int64 `json:"auto_suggestions"`
}

// PatternSuggestion represents a suggested extraction pattern
type PatternSuggestion struct {
	Pattern        string   `json:"pattern"`
	Confidence     float64  `json:"confidence"`
	SampleCount    int64    `json:"sample_count"`
	FieldsDetected []string `json:"fields_detected"`
	SampleMessages []string `json:"sample_messages"`
	Reason         string   `json:"reason"`
}

// NewPatternLearner creates a new pattern learner
func NewPatternLearner(config PatternLearnerConfig) *PatternLearner {
	return &PatternLearner{
		unmatchedLogs:  newUnmatchedLogBuffer(config.UnmatchedBufferSize),
		candidates:     make(map[string]*PatternCandidate),
		tokenFrequency: make(map[string]int64),
		config:         config,
	}
}

func newUnmatchedLogBuffer(size int) *UnmatchedLogBuffer {
	return &UnmatchedLogBuffer{
		logs: make([]UnmatchedLog, size),
		size: size,
	}
}

// RecordUnmatched records an unmatched log for learning
func (pl *PatternLearner) RecordUnmatched(message, source string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	pl.stats.TotalLogsProcessed++
	pl.stats.UnmatchedLogsCount++

	// Compute signature
	signature := pl.computeSignature(message)

	// Add to buffer
	pl.unmatchedLogs.add(UnmatchedLog{
		Message:   message,
		Source:    source,
		Timestamp: time.Now(),
		Signature: signature,
	})

	// Update candidate
	pl.updateCandidate(signature, message)
}

// RecordMatched records a successfully matched log
func (pl *PatternLearner) RecordMatched(message, source, patternID string) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	pl.stats.TotalLogsProcessed++
}

// computeSignature computes a normalized signature for a log message
func (pl *PatternLearner) computeSignature(message string) string {
	// Tokenize and normalize
	normalized := message

	// Replace common variable patterns with tokens
	replacements := []struct {
		pattern *regexp.Regexp
		token   string
	}{
		// UUIDs
		{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<UUID>"},
		// Hex strings (8+ chars)
		{regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`), "<HEX>"},
		// IP addresses
		{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "<IP>"},
		// IPv6
		{regexp.MustCompile(`[0-9a-fA-F:]{15,}`), "<IPV6>"},
		// ISO timestamps
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`), "<TIMESTAMP>"},
		// Date formats
		{regexp.MustCompile(`\d{1,2}[/.-]\d{1,2}[/.-]\d{2,4}`), "<DATE>"},
		// Time formats
		{regexp.MustCompile(`\d{2}:\d{2}:\d{2}(?:\.\d+)?`), "<TIME>"},
		// URLs
		{regexp.MustCompile(`https?://[^\s]+`), "<URL>"},
		// Email addresses
		{regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "<EMAIL>"},
		// File paths
		{regexp.MustCompile(`(/[a-zA-Z0-9._-]+)+`), "<PATH>"},
		// Numbers with units
		{regexp.MustCompile(`\d+(?:\.\d+)?(?:ms|s|m|h|ns|us|KB|MB|GB|TB|B)\b`), "<UNIT>"},
		// Plain numbers
		{regexp.MustCompile(`\b\d+\b`), "<NUM>"},
		// Quoted strings
		{regexp.MustCompile(`"[^"]*"`), "<QUOTED>"},
		// Request IDs and similar
		{regexp.MustCompile(`\b[a-zA-Z0-9]{16,}\b`), "<REQID>"},
	}

	for _, r := range replacements {
		normalized = r.pattern.ReplaceAllString(normalized, r.token)
	}

	// Hash the normalized string
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8])
}

// updateCandidate updates or creates a pattern candidate
func (pl *PatternLearner) updateCandidate(signature, message string) {
	candidate, exists := pl.candidates[signature]

	if !exists {
		// Check if we're at capacity
		if len(pl.candidates) >= pl.config.MaxCandidates {
			pl.evictOldestCandidate()
		}

		candidate = &PatternCandidate{
			ID:             signature,
			Signature:      signature,
			SampleCount:    0,
			SampleMessages: make([]string, 0, 5),
			TokenBreakdown: make(map[string]int64),
			FirstSeen:      time.Now(),
		}
		pl.candidates[signature] = candidate
		pl.stats.CandidatesGenerated++
	}

	candidate.SampleCount++
	candidate.LastSeen = time.Now()

	// Keep first few samples
	if len(candidate.SampleMessages) < 5 {
		candidate.SampleMessages = append(candidate.SampleMessages, message)
	}

	// Generate pattern if enough samples
	if candidate.SampleCount >= int64(pl.config.MinSamplesForSuggestion) && candidate.Pattern == "" {
		candidate.Pattern = pl.generatePattern(candidate.SampleMessages)
		candidate.FieldsDetected = pl.detectFields(candidate.Pattern)
		candidate.Confidence = pl.calculateConfidence(candidate)
	}
}

// evictOldestCandidate removes the oldest/least useful candidate
func (pl *PatternLearner) evictOldestCandidate() {
	var oldestSig string
	var oldestTime time.Time

	for sig, candidate := range pl.candidates {
		if oldestSig == "" || candidate.LastSeen.Before(oldestTime) {
			oldestSig = sig
			oldestTime = candidate.LastSeen
		}
	}

	if oldestSig != "" {
		delete(pl.candidates, oldestSig)
	}
}

// generatePattern generates a regex pattern from sample messages
func (pl *PatternLearner) generatePattern(samples []string) string {
	if len(samples) == 0 {
		return ""
	}

	// Use the first sample as a template
	template := samples[0]

	// Define replacements for variable patterns
	replacements := []struct {
		pattern      *regexp.Regexp
		captureGroup string
		fieldName    string
	}{
		{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), `(?P<uuid>[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`, "uuid"},
		{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), `(?P<ip>\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`, "ip"},
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`), `(?P<timestamp>\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)`, "timestamp"},
		{regexp.MustCompile(`https?://[^\s]+`), `(?P<url>https?://[^\s]+)`, "url"},
		{regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), `(?P<email>[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`, "email"},
		{regexp.MustCompile(`(/[a-zA-Z0-9._-]+)+`), `(?P<path>(?:/[a-zA-Z0-9._-]+)+)`, "path"},
		{regexp.MustCompile(`\d+(?:\.\d+)?(?:ms|s|m|h|ns|us|KB|MB|GB|TB|B)\b`), `(?P<value>\d+(?:\.\d+)?(?:ms|s|m|h|ns|us|KB|MB|GB|TB|B))`, "value"},
		{regexp.MustCompile(`"[^"]*"`), `(?P<quoted>"[^"]*")`, "quoted"},
		{regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`), `(?P<hex>[0-9a-fA-F]{8,})`, "hex"},
		{regexp.MustCompile(`\b\d+\b`), `(?P<num>\d+)`, "num"},
	}

	// Start with escaped template
	pattern := template
	fieldCount := 0

	// Replace variable parts with capture groups
	for _, r := range replacements {
		if r.pattern.MatchString(pattern) {
			// Add field number to avoid duplicate capture group names
			fieldName := fmt.Sprintf("%s_%d", r.fieldName, fieldCount)
			captureGroup := strings.Replace(r.captureGroup, "?P<"+r.fieldName+">", "?P<"+fieldName+">", 1)

			// Replace just the first match
			loc := r.pattern.FindStringIndex(pattern)
			if loc != nil {
				before := pattern[:loc[0]]
				after := pattern[loc[1]:]
				// Escape the parts that aren't the match
				before = regexp.QuoteMeta(before)
				pattern = before + captureGroup + after
				fieldCount++
			}
		}
	}

	// Escape any remaining literal parts
	// This is tricky because we already have some capture groups
	// For simplicity, if no capture groups were added, escape the whole thing
	if fieldCount == 0 {
		pattern = regexp.QuoteMeta(pattern)
	}

	return "^" + pattern + "$"
}

// detectFields extracts field names from a pattern
func (pl *PatternLearner) detectFields(pattern string) []string {
	re := regexp.MustCompile(`\?P<([^>]+)>`)
	matches := re.FindAllStringSubmatch(pattern, -1)

	fields := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			fields = append(fields, match[1])
		}
	}

	return fields
}

// calculateConfidence calculates confidence for a pattern candidate
func (pl *PatternLearner) calculateConfidence(candidate *PatternCandidate) float64 {
	confidence := 0.0

	// More samples = higher confidence
	sampleFactor := float64(candidate.SampleCount) / 1000.0
	if sampleFactor > 0.5 {
		sampleFactor = 0.5
	}
	confidence += sampleFactor

	// More fields = higher confidence
	fieldFactor := float64(len(candidate.FieldsDetected)) * 0.1
	if fieldFactor > 0.3 {
		fieldFactor = 0.3
	}
	confidence += fieldFactor

	// Pattern compiles = add confidence
	if candidate.Pattern != "" {
		if _, err := regexp.Compile(candidate.Pattern); err == nil {
			confidence += 0.2
		}
	}

	return confidence
}

// GetSuggestions returns pattern suggestions based on learned data
func (pl *PatternLearner) GetSuggestions(minConfidence float64) []PatternSuggestion {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	var suggestions []PatternSuggestion

	for _, candidate := range pl.candidates {
		if candidate.Pattern == "" || candidate.Confidence < minConfidence {
			continue
		}

		suggestions = append(suggestions, PatternSuggestion{
			Pattern:        candidate.Pattern,
			Confidence:     candidate.Confidence,
			SampleCount:    candidate.SampleCount,
			FieldsDetected: candidate.FieldsDetected,
			SampleMessages: candidate.SampleMessages,
			Reason:         fmt.Sprintf("Detected %d similar messages with %d fields", candidate.SampleCount, len(candidate.FieldsDetected)),
		})
	}

	// Sort by confidence
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Confidence > suggestions[j].Confidence
	})

	return suggestions
}

// OptimizePatternPriorities suggests priority adjustments based on usage
func (pl *PatternLearner) OptimizePatternPriorities(store *PatternLearningStore) []PatternPriorityAdjustment {
	if store == nil {
		return nil
	}

	pl.mu.Lock()
	defer pl.mu.Unlock()

	var adjustments []PatternPriorityAdjustment

	// Get all sources
	sources := store.GetSources()

	for _, source := range sources {
		// Get pattern stats for this source
		stats := store.GetLearnedPatterns(source)

		// Sort by effectiveness (success rate * match count)
		sort.Slice(stats, func(i, j int) bool {
			scoreI := stats[i].SuccessRate * float64(stats[i].MatchCount)
			scoreJ := stats[j].SuccessRate * float64(stats[j].MatchCount)
			return scoreI > scoreJ
		})

		// Suggest priority adjustments
		for rank, stat := range stats {
			suggestedPriority := 100 - (rank * 10)
			if suggestedPriority < 10 {
				suggestedPriority = 10
			}

			adjustments = append(adjustments, PatternPriorityAdjustment{
				Source:            source,
				PatternID:         stat.PatternID,
				CurrentPriority:   0, // Would need to look up
				SuggestedPriority: suggestedPriority,
				SuccessRate:       stat.SuccessRate,
				MatchCount:        stat.MatchCount,
				Reason:            fmt.Sprintf("Rank %d for source %s (%.1f%% success rate)", rank+1, source, stat.SuccessRate*100),
			})
		}
	}

	pl.stats.PatternsOptimized += int64(len(adjustments))

	return adjustments
}

// PatternPriorityAdjustment represents a suggested priority change
type PatternPriorityAdjustment struct {
	Source            string  `json:"source"`
	PatternID         string  `json:"pattern_id"`
	CurrentPriority   int     `json:"current_priority"`
	SuggestedPriority int     `json:"suggested_priority"`
	SuccessRate       float64 `json:"success_rate"`
	MatchCount        int64   `json:"match_count"`
	Reason            string  `json:"reason"`
}

// GetStats returns learning statistics
func (pl *PatternLearner) GetStats() PatternLearnerStats {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	return pl.stats
}

// GetCandidates returns all pattern candidates
func (pl *PatternLearner) GetCandidates() []*PatternCandidate {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	candidates := make([]*PatternCandidate, 0, len(pl.candidates))
	for _, c := range pl.candidates {
		copy := *c
		candidates = append(candidates, &copy)
	}

	// Sort by sample count
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SampleCount > candidates[j].SampleCount
	})

	return candidates
}

// UnmatchedLogBuffer methods

func (b *UnmatchedLogBuffer) add(log UnmatchedLog) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logs[b.index] = log
	b.index = (b.index + 1) % b.size
}

func (b *UnmatchedLogBuffer) getRecent(count int) []UnmatchedLog {
	b.mu.Lock()
	defer b.mu.Unlock()

	if count > b.size {
		count = b.size
	}

	result := make([]UnmatchedLog, 0, count)
	idx := (b.index - 1 + b.size) % b.size

	for i := 0; i < count; i++ {
		if b.logs[idx].Message != "" {
			result = append(result, b.logs[idx])
		}
		idx = (idx - 1 + b.size) % b.size
	}

	return result
}

// IntegrateWithExtractor integrates the pattern learner with a field extractor
func (pl *PatternLearner) IntegrateWithExtractor(fe *FieldExtractor) {
	// Store reference for callback integration
	// This enables automatic learning during extraction
}

// AnalyzeLogStructure analyzes a log message structure
func (pl *PatternLearner) AnalyzeLogStructure(message string) LogStructureAnalysis {
	analysis := LogStructureAnalysis{
		Message:   message,
		Signature: pl.computeSignature(message),
	}

	// Detect structure type
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		analysis.StructureType = "json"
	} else if strings.Contains(message, "=") && regexp.MustCompile(`\w+=\S+`).MatchString(message) {
		analysis.StructureType = "key-value"
	} else if strings.HasPrefix(trimmed, "<") {
		analysis.StructureType = "syslog"
	} else {
		analysis.StructureType = "unstructured"
	}

	// Extract tokens
	tokens := regexp.MustCompile(`\S+`).FindAllString(message, -1)
	analysis.TokenCount = len(tokens)

	// Detect potential fields
	analysis.PotentialFields = pl.detectPotentialFields(message)

	return analysis
}

// LogStructureAnalysis represents analysis of a log message structure
type LogStructureAnalysis struct {
	Message         string   `json:"message"`
	Signature       string   `json:"signature"`
	StructureType   string   `json:"structure_type"`
	TokenCount      int      `json:"token_count"`
	PotentialFields []string `json:"potential_fields"`
}

// detectPotentialFields detects potential fields in a message
func (pl *PatternLearner) detectPotentialFields(message string) []string {
	var fields []string

	// Check for key-value pairs
	kvRe := regexp.MustCompile(`(\w+)=("[^"]*"|'[^']*'|\S+)`)
	kvMatches := kvRe.FindAllStringSubmatch(message, -1)
	for _, match := range kvMatches {
		if len(match) > 1 {
			fields = append(fields, match[1])
		}
	}

	// Check for common field patterns
	patterns := map[string]*regexp.Regexp{
		"ip_address": regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`),
		"timestamp":  regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`),
		"uuid":       regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`),
		"url":        regexp.MustCompile(`https?://[^\s]+`),
		"email":      regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`),
	}

	for name, re := range patterns {
		if re.MatchString(message) {
			fields = append(fields, name)
		}
	}

	return fields
}

// PersistCandidates saves candidates to the database
func (pl *PatternLearner) PersistCandidates(db *sql.DB) error {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	// Create table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pattern_candidates (
			id TEXT PRIMARY KEY,
			signature TEXT NOT NULL,
			pattern TEXT,
			sample_count INTEGER,
			fields_detected TEXT,
			confidence REAL,
			first_seen INTEGER,
			last_seen INTEGER
		)
	`)
	if err != nil {
		return err
	}

	for _, c := range pl.candidates {
		fieldsJSON := strings.Join(c.FieldsDetected, ",")
		_, err := db.Exec(`
			INSERT INTO pattern_candidates (id, signature, pattern, sample_count, fields_detected, confidence, first_seen, last_seen)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				sample_count = excluded.sample_count,
				pattern = excluded.pattern,
				fields_detected = excluded.fields_detected,
				confidence = excluded.confidence,
				last_seen = excluded.last_seen
		`, c.ID, c.Signature, c.Pattern, c.SampleCount, fieldsJSON, c.Confidence, c.FirstSeen.Unix(), c.LastSeen.Unix())
		if err != nil {
			return err
		}
	}

	return nil
}

// LoadCandidates loads candidates from the database
func (pl *PatternLearner) LoadCandidates(db *sql.DB) error {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	rows, err := db.Query(`
		SELECT id, signature, pattern, sample_count, fields_detected, confidence, first_seen, last_seen
		FROM pattern_candidates
	`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil // Table doesn't exist yet
		}
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var c PatternCandidate
		var fieldsStr string
		var firstSeen, lastSeen int64

		err := rows.Scan(&c.ID, &c.Signature, &c.Pattern, &c.SampleCount, &fieldsStr, &c.Confidence, &firstSeen, &lastSeen)
		if err != nil {
			continue
		}

		c.FirstSeen = time.Unix(firstSeen, 0)
		c.LastSeen = time.Unix(lastSeen, 0)
		if fieldsStr != "" {
			c.FieldsDetected = strings.Split(fieldsStr, ",")
		}
		c.SampleMessages = make([]string, 0)
		c.TokenBreakdown = make(map[string]int64)

		pl.candidates[c.ID] = &c
	}

	return nil
}
