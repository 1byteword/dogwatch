package logreduce

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Pattern represents a discovered log pattern (template)
type Pattern struct {
	ID            string            `json:"id"`
	Template      string            `json:"template"`       // Pattern with <*> placeholders
	Tokens        []Token           `json:"tokens"`         // Parsed tokens
	SampleMessage string            `json:"sample_message"` // Original example

	// Statistics
	Count         int64             `json:"count"`
	FirstSeen     time.Time         `json:"first_seen"`
	LastSeen      time.Time         `json:"last_seen"`

	// Metadata extracted from samples
	Level         string            `json:"level,omitempty"`
	Service       string            `json:"service,omitempty"`

	// Trend information
	CountLastHour int64             `json:"count_last_hour"`
	CountLastDay  int64             `json:"count_last_day"`
	Trend         TrendDirection    `json:"trend"`
	TrendPercent  float64           `json:"trend_percent"`

	// Variability info
	VariableCount int               `json:"variable_count"` // Number of <*> in template
	Variability   float64           `json:"variability"`    // 0-1 how variable the pattern is
}

// Token represents a part of a log pattern
type Token struct {
	Value    string `json:"value"`
	Variable bool   `json:"variable"` // true if this is a <*> placeholder
	Position int    `json:"position"`
}

// TrendDirection indicates pattern trend
type TrendDirection string

const (
	TrendNew        TrendDirection = "new"        // First seen recently
	TrendIncreasing TrendDirection = "increasing" // Count going up
	TrendStable     TrendDirection = "stable"     // No significant change
	TrendDecreasing TrendDirection = "decreasing" // Count going down
	TrendGone       TrendDirection = "gone"       // Stopped appearing
)

// PatternMatch represents a log entry matched to a pattern
type PatternMatch struct {
	PatternID  string            `json:"pattern_id"`
	Message    string            `json:"message"`
	Variables  map[string]string `json:"variables"` // Extracted variable values
	Timestamp  time.Time         `json:"timestamp"`
	Confidence float64           `json:"confidence"` // 0-1 match confidence
}

// PatternGroup represents patterns grouped by some criteria
type PatternGroup struct {
	Name     string     `json:"name"`
	Patterns []*Pattern `json:"patterns"`
	Count    int64      `json:"total_count"`
}

// PatternStats provides overall statistics
type PatternStats struct {
	TotalPatterns   int            `json:"total_patterns"`
	TotalLogs       int64          `json:"total_logs"`
	NewPatterns     int            `json:"new_patterns"`      // Last hour
	TopPatterns     []*Pattern     `json:"top_patterns"`      // By count
	TrendingUp      []*Pattern     `json:"trending_up"`       // Increasing
	TrendingDown    []*Pattern     `json:"trending_down"`     // Decreasing
	ByLevel         map[string]int `json:"by_level"`
	ByService       map[string]int `json:"by_service"`
	CoveragePercent float64        `json:"coverage_percent"`  // % of logs matched
	AnalyzedAt      time.Time      `json:"analyzed_at"`
}

// ReduceResult is the result of log reduction
type ReduceResult struct {
	InputCount    int64      `json:"input_count"`
	PatternCount  int        `json:"pattern_count"`
	Reduction     float64    `json:"reduction_ratio"` // e.g., 1000:1
	Patterns      []*Pattern `json:"patterns"`
	Unmatched     int64      `json:"unmatched_count"`
	ProcessedAt   time.Time  `json:"processed_at"`
	ProcessingMs  int64      `json:"processing_ms"`
}

// LogEntry represents a log entry for pattern mining
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Service   string
	Message   string
}

// Common variable patterns to normalize
var variablePatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"UUID", regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)},
	{"HEX32", regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`)},
	{"IP", regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?\b`)},
	{"TIMESTAMP", regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`)},
	{"DURATION", regexp.MustCompile(`\b\d+(\.\d+)?(ms|s|m|h|ns|µs)\b`)},
	{"BYTES", regexp.MustCompile(`\b\d+(\.\d+)?\s*(B|KB|MB|GB|TB|bytes)\b`)},
	{"PATH", regexp.MustCompile(`(/[a-zA-Z0-9._-]+)+`)},
	{"URL", regexp.MustCompile(`https?://[^\s]+`)},
	{"EMAIL", regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)},
	{"NUMBER", regexp.MustCompile(`\b\d+\b`)},
}

// Tokenize splits a message into tokens
func Tokenize(message string) []string {
	// Split on whitespace and common delimiters
	re := regexp.MustCompile(`[\s,;:=\[\]{}()|]+`)
	parts := re.Split(message, -1)

	var tokens []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tokens = append(tokens, p)
		}
	}
	return tokens
}

// NormalizeToken replaces variable parts with placeholders
func NormalizeToken(token string) (string, bool) {
	for _, vp := range variablePatterns {
		if vp.pattern.MatchString(token) {
			return "<*>", true
		}
	}

	// Check if it's a quoted string
	if (strings.HasPrefix(token, `"`) && strings.HasSuffix(token, `"`)) ||
	   (strings.HasPrefix(token, `'`) && strings.HasSuffix(token, `'`)) {
		return "<*>", true
	}

	return token, false
}

// CreateTemplate creates a pattern template from tokens
func CreateTemplate(tokens []string) string {
	var normalized []string
	for _, t := range tokens {
		norm, _ := NormalizeToken(t)
		normalized = append(normalized, norm)
	}
	return strings.Join(normalized, " ")
}

// HashTemplate creates a unique ID for a template
func HashTemplate(template string) string {
	hash := md5.Sum([]byte(template))
	return fmt.Sprintf("%x", hash)[:16]
}

// MatchPattern checks if a message matches a pattern template
func MatchPattern(message string, template string) (bool, map[string]string, float64) {
	msgTokens := Tokenize(message)
	tmplTokens := strings.Split(template, " ")

	if len(msgTokens) != len(tmplTokens) {
		return false, nil, 0
	}

	variables := make(map[string]string)
	varCount := 0
	matchCount := 0

	for i, tmplToken := range tmplTokens {
		if tmplToken == "<*>" {
			variables[fmt.Sprintf("var%d", varCount)] = msgTokens[i]
			varCount++
			matchCount++
		} else if tmplToken == msgTokens[i] {
			matchCount++
		} else {
			// Check if normalized version matches
			norm, isVar := NormalizeToken(msgTokens[i])
			if isVar && tmplToken == "<*>" {
				variables[fmt.Sprintf("var%d", varCount)] = msgTokens[i]
				varCount++
				matchCount++
			} else if norm == tmplToken {
				matchCount++
			} else {
				return false, nil, 0
			}
		}
	}

	confidence := float64(matchCount) / float64(len(tmplTokens))
	return true, variables, confidence
}

// CalculateVariability computes how variable a pattern is
func CalculateVariability(template string) float64 {
	tokens := strings.Split(template, " ")
	if len(tokens) == 0 {
		return 0
	}

	varCount := 0
	for _, t := range tokens {
		if t == "<*>" {
			varCount++
		}
	}

	return float64(varCount) / float64(len(tokens))
}

// SortPatternsByCount sorts patterns by count descending
func SortPatternsByCount(patterns []*Pattern) {
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Count > patterns[j].Count
	})
}

// SortPatternsByTrend sorts patterns by trend (new/increasing first)
func SortPatternsByTrend(patterns []*Pattern) {
	trendOrder := map[TrendDirection]int{
		TrendNew:        0,
		TrendIncreasing: 1,
		TrendStable:     2,
		TrendDecreasing: 3,
		TrendGone:       4,
	}

	sort.Slice(patterns, func(i, j int) bool {
		if trendOrder[patterns[i].Trend] != trendOrder[patterns[j].Trend] {
			return trendOrder[patterns[i].Trend] < trendOrder[patterns[j].Trend]
		}
		return patterns[i].Count > patterns[j].Count
	})
}

// FilterPatternsByLevel returns patterns matching a level
func FilterPatternsByLevel(patterns []*Pattern, level string) []*Pattern {
	var filtered []*Pattern
	for _, p := range patterns {
		if p.Level == level {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// FilterPatternsByService returns patterns matching a service
func FilterPatternsByService(patterns []*Pattern, service string) []*Pattern {
	var filtered []*Pattern
	for _, p := range patterns {
		if p.Service == service {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// GetTopPatterns returns the top N patterns by count
func GetTopPatterns(patterns []*Pattern, n int) []*Pattern {
	sorted := make([]*Pattern, len(patterns))
	copy(sorted, patterns)
	SortPatternsByCount(sorted)

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// GetNewPatterns returns patterns first seen within duration
func GetNewPatterns(patterns []*Pattern, since time.Duration) []*Pattern {
	cutoff := time.Now().Add(-since)
	var newPatterns []*Pattern

	for _, p := range patterns {
		if p.FirstSeen.After(cutoff) {
			newPatterns = append(newPatterns, p)
		}
	}

	SortPatternsByCount(newPatterns)
	return newPatterns
}

// GetTrendingPatterns returns patterns with increasing counts
func GetTrendingPatterns(patterns []*Pattern) []*Pattern {
	var trending []*Pattern

	for _, p := range patterns {
		if p.Trend == TrendIncreasing || p.Trend == TrendNew {
			trending = append(trending, p)
		}
	}

	sort.Slice(trending, func(i, j int) bool {
		return trending[i].TrendPercent > trending[j].TrendPercent
	})

	return trending
}
