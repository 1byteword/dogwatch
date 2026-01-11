package logs

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Pattern represents a detected log pattern
type Pattern struct {
	ID          string    `json:"id"`
	Signature   string    `json:"signature"`   // Normalized pattern with placeholders
	Count       int64     `json:"count"`       // Total occurrences
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Level       string    `json:"level"`       // Most common level
	Service     string    `json:"service"`     // Most common service
	Examples    []string  `json:"examples"`    // Sample messages (max 3)
	Trend       string    `json:"trend"`       // "increasing", "stable", "decreasing"
	CountLastHr int64     `json:"count_last_hr"`
}

// PatternMatch links a log entry to its pattern
type PatternMatch struct {
	LogID     string
	PatternID string
	Timestamp time.Time
}

// PatternDetector identifies and tracks log patterns
type PatternDetector struct {
	patterns   map[string]*Pattern
	matches    []PatternMatch // Rolling window of recent matches
	mu         sync.RWMutex

	// Compiled regexes for tokenization
	tokenizers []*tokenizer
}

type tokenizer struct {
	regex       *regexp.Regexp
	placeholder string
}

// NewPatternDetector creates a new pattern detector
func NewPatternDetector() *PatternDetector {
	pd := &PatternDetector{
		patterns: make(map[string]*Pattern),
		matches:  make([]PatternMatch, 0, 10000),
	}

	// Define tokenizers to normalize variable parts
	pd.tokenizers = []*tokenizer{
		// UUIDs
		{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<UUID>"},
		// Hex strings (32+ chars, like hashes)
		{regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`), "<HEX>"},
		// IP addresses
		{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "<IP>"},
		// IPv6
		{regexp.MustCompile(`\b[0-9a-fA-F:]{15,39}\b`), "<IPV6>"},
		// Timestamps (ISO 8601)
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?`), "<TIMESTAMP>"},
		// Dates
		{regexp.MustCompile(`\b\d{4}[-/]\d{2}[-/]\d{2}\b`), "<DATE>"},
		// Times
		{regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:\.\d+)?\b`), "<TIME>"},
		// URLs
		{regexp.MustCompile(`https?://[^\s]+`), "<URL>"},
		// Email addresses
		{regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`), "<EMAIL>"},
		// File paths
		{regexp.MustCompile(`(?:/[\w.-]+)+`), "<PATH>"},
		// Numbers with units (ms, s, KB, MB, GB, etc.)
		{regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ms|s|ns|us|m|h|d|B|KB|MB|GB|TB|%)\b`), "<NUM>"},
		// Numbers after underscore or dash (IDs like user_123, order-456)
		{regexp.MustCompile(`[_-]\d+\b`), "_<NUM>"},
		// Numbers (including decimals, negative, scientific)
		{regexp.MustCompile(`\b-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b`), "<NUM>"},
		// Quoted strings
		{regexp.MustCompile(`"[^"]*"`), `"<STR>"`},
		{regexp.MustCompile(`'[^']*'`), `'<STR>'`},
		// Request IDs and trace IDs (alphanumeric 16+ chars)
		{regexp.MustCompile(`\b[A-Za-z0-9]{16,}\b`), "<ID>"},
	}

	return pd
}

// Process analyzes a log entry and returns its pattern ID
func (pd *PatternDetector) Process(entry *LogEntry) string {
	pd.mu.Lock()
	defer pd.mu.Unlock()

	// Normalize the message to get signature
	signature := pd.normalize(entry.Message)
	patternID := pd.hashSignature(signature)

	pattern, exists := pd.patterns[patternID]
	if !exists {
		pattern = &Pattern{
			ID:        patternID,
			Signature: signature,
			FirstSeen: entry.Timestamp,
			LastSeen:  entry.Timestamp,
			Level:     string(entry.Level),
			Service:   entry.Service,
			Examples:  []string{entry.Message},
			Count:     1,
		}
		pd.patterns[patternID] = pattern
	} else {
		pattern.Count++
		pattern.LastSeen = entry.Timestamp
		// Keep up to 3 diverse examples
		if len(pattern.Examples) < 3 && !pd.containsSimilar(pattern.Examples, entry.Message) {
			pattern.Examples = append(pattern.Examples, entry.Message)
		}
	}

	// Track match for trend analysis
	pd.matches = append(pd.matches, PatternMatch{
		LogID:     entry.ID,
		PatternID: patternID,
		Timestamp: entry.Timestamp,
	})

	// Trim old matches (keep last hour for trend calculation)
	pd.trimMatches()

	return patternID
}

// normalize extracts a pattern signature from a message
func (pd *PatternDetector) normalize(message string) string {
	result := message

	// Apply tokenizers in order
	for _, t := range pd.tokenizers {
		result = t.regex.ReplaceAllString(result, t.placeholder)
	}

	// Collapse multiple spaces
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// hashSignature creates a stable ID from a pattern signature
func (pd *PatternDetector) hashSignature(sig string) string {
	hash := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(hash[:8])
}

// containsSimilar checks if examples already has a similar message
func (pd *PatternDetector) containsSimilar(examples []string, msg string) bool {
	normalized := pd.normalize(msg)
	for _, ex := range examples {
		if pd.normalize(ex) == normalized {
			return true
		}
	}
	return false
}

// trimMatches removes matches older than 1 hour
func (pd *PatternDetector) trimMatches() {
	cutoff := time.Now().Add(-1 * time.Hour)
	firstValid := 0
	for i, m := range pd.matches {
		if m.Timestamp.After(cutoff) {
			firstValid = i
			break
		}
	}
	if firstValid > 0 {
		pd.matches = pd.matches[firstValid:]
	}
}

// GetPatterns returns all detected patterns sorted by count
func (pd *PatternDetector) GetPatterns() []*Pattern {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	// Calculate counts for last hour
	hourCounts := make(map[string]int64)
	cutoff := time.Now().Add(-1 * time.Hour)
	for _, m := range pd.matches {
		if m.Timestamp.After(cutoff) {
			hourCounts[m.PatternID]++
		}
	}

	patterns := make([]*Pattern, 0, len(pd.patterns))
	for _, p := range pd.patterns {
		// Copy pattern
		copy := *p
		copy.CountLastHr = hourCounts[p.ID]

		// Calculate trend (compare last hour to previous average)
		if copy.Count > 0 {
			duration := time.Since(copy.FirstSeen)
			if duration > time.Hour {
				hoursActive := duration.Hours()
				avgPerHour := float64(copy.Count) / hoursActive
				if float64(copy.CountLastHr) > avgPerHour*1.5 {
					copy.Trend = "increasing"
				} else if float64(copy.CountLastHr) < avgPerHour*0.5 {
					copy.Trend = "decreasing"
				} else {
					copy.Trend = "stable"
				}
			} else {
				copy.Trend = "new"
			}
		}

		patterns = append(patterns, &copy)
	}

	// Sort by count descending
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})

	return patterns
}

// GetPattern returns a specific pattern by ID
func (pd *PatternDetector) GetPattern(id string) *Pattern {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	if p, ok := pd.patterns[id]; ok {
		copy := *p
		return &copy
	}
	return nil
}

// GetTopPatterns returns the N most frequent patterns
func (pd *PatternDetector) GetTopPatterns(n int) []*Pattern {
	patterns := pd.GetPatterns()
	if len(patterns) > n {
		return patterns[:n]
	}
	return patterns
}

// GetNewPatterns returns patterns first seen in the last duration
func (pd *PatternDetector) GetNewPatterns(since time.Duration) []*Pattern {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	cutoff := time.Now().Add(-since)
	var newPatterns []*Pattern

	for _, p := range pd.patterns {
		if p.FirstSeen.After(cutoff) {
			copy := *p
			newPatterns = append(newPatterns, &copy)
		}
	}

	// Sort by first seen (newest first)
	sort.Slice(newPatterns, func(i, j int) bool {
		return newPatterns[i].FirstSeen.After(newPatterns[j].FirstSeen)
	})

	return newPatterns
}

// GetIncreasingPatterns returns patterns with increasing trend
func (pd *PatternDetector) GetIncreasingPatterns() []*Pattern {
	patterns := pd.GetPatterns()
	var increasing []*Pattern
	for _, p := range patterns {
		if p.Trend == "increasing" {
			increasing = append(increasing, p)
		}
	}
	return increasing
}

// Stats returns pattern detection statistics
type PatternStats struct {
	TotalPatterns    int   `json:"total_patterns"`
	TotalMatches     int64 `json:"total_matches"`
	MatchesLastHour  int64 `json:"matches_last_hour"`
	NewPatternsToday int   `json:"new_patterns_today"`
	IncreasingCount  int   `json:"increasing_count"`
}

func (pd *PatternDetector) Stats() PatternStats {
	pd.mu.RLock()
	defer pd.mu.RUnlock()

	stats := PatternStats{
		TotalPatterns: len(pd.patterns),
	}

	today := time.Now().Truncate(24 * time.Hour)
	hourAgo := time.Now().Add(-1 * time.Hour)

	for _, p := range pd.patterns {
		stats.TotalMatches += p.Count
		if p.FirstSeen.After(today) {
			stats.NewPatternsToday++
		}
	}

	for _, m := range pd.matches {
		if m.Timestamp.After(hourAgo) {
			stats.MatchesLastHour++
		}
	}

	return stats
}

// Clear resets all patterns (useful for testing)
func (pd *PatternDetector) Clear() {
	pd.mu.Lock()
	defer pd.mu.Unlock()
	pd.patterns = make(map[string]*Pattern)
	pd.matches = pd.matches[:0]
}
