package logcompare

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CompareRequest defines the comparison parameters
type CompareRequest struct {
	// Baseline period (the "good" or reference period)
	BaselineStart time.Time `json:"baseline_start"`
	BaselineEnd   time.Time `json:"baseline_end"`

	// Comparison period (the "bad" or investigation period)
	CompareStart time.Time `json:"compare_start"`
	CompareEnd   time.Time `json:"compare_end"`

	// Optional filters
	Service string `json:"service,omitempty"`
	Level   string `json:"level,omitempty"`

	// Analysis options
	MinOccurrences   int     `json:"min_occurrences,omitempty"`   // Min count to include pattern
	SignificanceThreshold float64 `json:"significance_threshold,omitempty"` // Min change % to flag
}

// CompareResult contains the comparison analysis
type CompareResult struct {
	Request          CompareRequest    `json:"request"`
	GeneratedAt      time.Time         `json:"generated_at"`

	// Summary stats
	BaselineStats    PeriodStats       `json:"baseline_stats"`
	CompareStats     PeriodStats       `json:"compare_stats"`

	// Key findings
	NewPatterns      []PatternChange   `json:"new_patterns"`       // Only in compare period
	MissingPatterns  []PatternChange   `json:"missing_patterns"`   // Only in baseline
	IncreasedPatterns []PatternChange  `json:"increased_patterns"` // Significant increase
	DecreasedPatterns []PatternChange  `json:"decreased_patterns"` // Significant decrease

	// Error analysis
	ErrorSpikes      []ErrorSpike      `json:"error_spikes"`

	// Top changes ranked by impact
	TopChanges       []PatternChange   `json:"top_changes"`

	// Overall assessment
	Severity         string            `json:"severity"` // critical, warning, info, ok
	Summary          string            `json:"summary"`
	Insights         []Insight         `json:"insights"`
}

// PeriodStats contains statistics for a time period
type PeriodStats struct {
	Start       time.Time         `json:"start"`
	End         time.Time         `json:"end"`
	Duration    string            `json:"duration"`
	TotalLogs   int64             `json:"total_logs"`
	UniquePatterns int            `json:"unique_patterns"`
	ByLevel     map[string]int64  `json:"by_level"`
	ByService   map[string]int64  `json:"by_service"`
	ErrorRate   float64           `json:"error_rate"`
}

// PatternChange represents a change in a log pattern
type PatternChange struct {
	PatternID       string            `json:"pattern_id"`
	Pattern         string            `json:"pattern"`
	SampleMessage   string            `json:"sample_message"`
	Level           string            `json:"level,omitempty"`
	Service         string            `json:"service,omitempty"`

	BaselineCount   int64             `json:"baseline_count"`
	CompareCount    int64             `json:"compare_count"`

	ChangeType      ChangeType        `json:"change_type"`
	ChangePercent   float64           `json:"change_percent"`
	ChangeAbsolute  int64             `json:"change_absolute"`

	Significance    float64           `json:"significance"` // 0-1 score
	Impact          string            `json:"impact"`       // critical, high, medium, low

	FirstSeen       *time.Time        `json:"first_seen,omitempty"`
	LastSeen        *time.Time        `json:"last_seen,omitempty"`
}

// ChangeType categorizes the type of change
type ChangeType string

const (
	ChangeNew       ChangeType = "new"       // Only in compare period
	ChangeMissing   ChangeType = "missing"   // Only in baseline
	ChangeIncreased ChangeType = "increased" // Count went up
	ChangeDecreased ChangeType = "decreased" // Count went down
	ChangeUnchanged ChangeType = "unchanged" // No significant change
)

// ErrorSpike represents a spike in error logs
type ErrorSpike struct {
	Service      string    `json:"service"`
	Level        string    `json:"level"`
	BaselineRate float64   `json:"baseline_rate"` // errors per minute
	CompareRate  float64   `json:"compare_rate"`
	SpikePercent float64   `json:"spike_percent"`
	TopPatterns  []string  `json:"top_patterns"`
}

// Insight is an actionable finding
type Insight struct {
	Type        string `json:"type"`        // new_errors, missing_logs, volume_spike, etc.
	Severity    string `json:"severity"`    // critical, warning, info
	Title       string `json:"title"`
	Description string `json:"description"`
	PatternIDs  []string `json:"pattern_ids,omitempty"`
}

// LogEntry represents a log entry for comparison
type LogEntry struct {
	Timestamp time.Time
	Level     string
	Service   string
	Message   string
}

// PatternStats tracks statistics for a pattern
type PatternStats struct {
	Pattern       string
	SampleMessage string
	Level         string
	Service       string
	Count         int64
	FirstSeen     time.Time
	LastSeen      time.Time
}

// Comparer performs log comparison analysis
type Comparer struct {
	minOccurrences      int
	significanceThreshold float64
}

// NewComparer creates a new log comparer
func NewComparer() *Comparer {
	return &Comparer{
		minOccurrences:      5,
		significanceThreshold: 50.0, // 50% change is significant
	}
}

// Compare analyzes two sets of logs
func (c *Comparer) Compare(baselineLogs, compareLogs []LogEntry, req CompareRequest) *CompareResult {
	if req.MinOccurrences > 0 {
		c.minOccurrences = req.MinOccurrences
	}
	if req.SignificanceThreshold > 0 {
		c.significanceThreshold = req.SignificanceThreshold
	}

	result := &CompareResult{
		Request:     req,
		GeneratedAt: time.Now(),
	}

	// Extract patterns from each period
	baselinePatterns := c.extractPatterns(baselineLogs)
	comparePatterns := c.extractPatterns(compareLogs)

	// Calculate stats
	result.BaselineStats = c.calculateStats(baselineLogs, baselinePatterns, req.BaselineStart, req.BaselineEnd)
	result.CompareStats = c.calculateStats(compareLogs, comparePatterns, req.CompareStart, req.CompareEnd)

	// Find changes
	result.NewPatterns = c.findNewPatterns(baselinePatterns, comparePatterns)
	result.MissingPatterns = c.findMissingPatterns(baselinePatterns, comparePatterns)
	result.IncreasedPatterns, result.DecreasedPatterns = c.findChangedPatterns(baselinePatterns, comparePatterns)

	// Analyze error spikes
	result.ErrorSpikes = c.findErrorSpikes(baselineLogs, compareLogs, req)

	// Rank top changes
	result.TopChanges = c.rankTopChanges(result)

	// Generate insights
	result.Insights = c.generateInsights(result)

	// Determine overall severity
	result.Severity, result.Summary = c.assessSeverity(result)

	return result
}

func (c *Comparer) extractPatterns(logs []LogEntry) map[string]*PatternStats {
	patterns := make(map[string]*PatternStats)

	for _, entry := range logs {
		pattern := normalizeMessage(entry.Message)
		patternID := hashPattern(pattern)

		if stats, exists := patterns[patternID]; exists {
			stats.Count++
			if entry.Timestamp.Before(stats.FirstSeen) {
				stats.FirstSeen = entry.Timestamp
			}
			if entry.Timestamp.After(stats.LastSeen) {
				stats.LastSeen = entry.Timestamp
			}
		} else {
			patterns[patternID] = &PatternStats{
				Pattern:       pattern,
				SampleMessage: entry.Message,
				Level:         entry.Level,
				Service:       entry.Service,
				Count:         1,
				FirstSeen:     entry.Timestamp,
				LastSeen:      entry.Timestamp,
			}
		}
	}

	return patterns
}

func (c *Comparer) calculateStats(logs []LogEntry, patterns map[string]*PatternStats, start, end time.Time) PeriodStats {
	stats := PeriodStats{
		Start:          start,
		End:            end,
		Duration:       end.Sub(start).String(),
		TotalLogs:      int64(len(logs)),
		UniquePatterns: len(patterns),
		ByLevel:        make(map[string]int64),
		ByService:      make(map[string]int64),
	}

	var errorCount int64
	for _, entry := range logs {
		stats.ByLevel[entry.Level]++
		if entry.Service != "" {
			stats.ByService[entry.Service]++
		}
		if entry.Level == "error" || entry.Level == "fatal" {
			errorCount++
		}
	}

	if stats.TotalLogs > 0 {
		stats.ErrorRate = float64(errorCount) / float64(stats.TotalLogs) * 100
	}

	return stats
}

func (c *Comparer) findNewPatterns(baseline, compare map[string]*PatternStats) []PatternChange {
	var newPatterns []PatternChange

	for patternID, stats := range compare {
		if _, exists := baseline[patternID]; !exists {
			if stats.Count >= int64(c.minOccurrences) {
				firstSeen := stats.FirstSeen
				lastSeen := stats.LastSeen
				change := PatternChange{
					PatternID:      patternID,
					Pattern:        stats.Pattern,
					SampleMessage:  stats.SampleMessage,
					Level:          stats.Level,
					Service:        stats.Service,
					BaselineCount:  0,
					CompareCount:   stats.Count,
					ChangeType:     ChangeNew,
					ChangePercent:  100,
					ChangeAbsolute: stats.Count,
					Significance:   c.calculateSignificance(0, stats.Count),
					Impact:         c.determineImpact(0, stats.Count, stats.Level),
					FirstSeen:      &firstSeen,
					LastSeen:       &lastSeen,
				}
				newPatterns = append(newPatterns, change)
			}
		}
	}

	// Sort by count descending
	sort.Slice(newPatterns, func(i, j int) bool {
		return newPatterns[i].CompareCount > newPatterns[j].CompareCount
	})

	return newPatterns
}

func (c *Comparer) findMissingPatterns(baseline, compare map[string]*PatternStats) []PatternChange {
	var missingPatterns []PatternChange

	for patternID, stats := range baseline {
		if _, exists := compare[patternID]; !exists {
			if stats.Count >= int64(c.minOccurrences) {
				lastSeen := stats.LastSeen
				change := PatternChange{
					PatternID:      patternID,
					Pattern:        stats.Pattern,
					SampleMessage:  stats.SampleMessage,
					Level:          stats.Level,
					Service:        stats.Service,
					BaselineCount:  stats.Count,
					CompareCount:   0,
					ChangeType:     ChangeMissing,
					ChangePercent:  -100,
					ChangeAbsolute: -stats.Count,
					Significance:   c.calculateSignificance(stats.Count, 0),
					Impact:         "low", // Missing logs are usually less critical
					LastSeen:       &lastSeen,
				}
				missingPatterns = append(missingPatterns, change)
			}
		}
	}

	sort.Slice(missingPatterns, func(i, j int) bool {
		return missingPatterns[i].BaselineCount > missingPatterns[j].BaselineCount
	})

	return missingPatterns
}

func (c *Comparer) findChangedPatterns(baseline, compare map[string]*PatternStats) ([]PatternChange, []PatternChange) {
	var increased, decreased []PatternChange

	for patternID, compareStats := range compare {
		baselineStats, exists := baseline[patternID]
		if !exists {
			continue
		}

		baseCount := baselineStats.Count
		compCount := compareStats.Count

		if baseCount < int64(c.minOccurrences) && compCount < int64(c.minOccurrences) {
			continue
		}

		var changePercent float64
		if baseCount > 0 {
			changePercent = float64(compCount-baseCount) / float64(baseCount) * 100
		}

		// Check if change is significant
		if absFloat(changePercent) < c.significanceThreshold {
			continue
		}

		lastSeen := compareStats.LastSeen
		change := PatternChange{
			PatternID:      patternID,
			Pattern:        compareStats.Pattern,
			SampleMessage:  compareStats.SampleMessage,
			Level:          compareStats.Level,
			Service:        compareStats.Service,
			BaselineCount:  baseCount,
			CompareCount:   compCount,
			ChangePercent:  changePercent,
			ChangeAbsolute: compCount - baseCount,
			Significance:   c.calculateSignificance(baseCount, compCount),
			Impact:         c.determineImpact(baseCount, compCount, compareStats.Level),
			LastSeen:       &lastSeen,
		}

		if changePercent > 0 {
			change.ChangeType = ChangeIncreased
			increased = append(increased, change)
		} else {
			change.ChangeType = ChangeDecreased
			decreased = append(decreased, change)
		}
	}

	// Sort by absolute change
	sort.Slice(increased, func(i, j int) bool {
		return increased[i].ChangeAbsolute > increased[j].ChangeAbsolute
	})
	sort.Slice(decreased, func(i, j int) bool {
		return decreased[i].ChangeAbsolute < decreased[j].ChangeAbsolute
	})

	return increased, decreased
}

func (c *Comparer) findErrorSpikes(baseline, compare []LogEntry, req CompareRequest) []ErrorSpike {
	var spikes []ErrorSpike

	// Calculate error rates by service
	baselineErrors := make(map[string]int64)
	compareErrors := make(map[string]int64)

	for _, entry := range baseline {
		if entry.Level == "error" || entry.Level == "fatal" {
			key := entry.Service
			if key == "" {
				key = "unknown"
			}
			baselineErrors[key]++
		}
	}

	for _, entry := range compare {
		if entry.Level == "error" || entry.Level == "fatal" {
			key := entry.Service
			if key == "" {
				key = "unknown"
			}
			compareErrors[key]++
		}
	}

	baselineDuration := req.BaselineEnd.Sub(req.BaselineStart).Minutes()
	compareDuration := req.CompareEnd.Sub(req.CompareStart).Minutes()

	if baselineDuration == 0 {
		baselineDuration = 1
	}
	if compareDuration == 0 {
		compareDuration = 1
	}

	// Find services with error spikes
	allServices := make(map[string]bool)
	for svc := range baselineErrors {
		allServices[svc] = true
	}
	for svc := range compareErrors {
		allServices[svc] = true
	}

	for svc := range allServices {
		baseRate := float64(baselineErrors[svc]) / baselineDuration
		compRate := float64(compareErrors[svc]) / compareDuration

		if compRate > baseRate && compRate > 0.1 { // At least 0.1 errors/min
			spikePercent := 0.0
			if baseRate > 0 {
				spikePercent = (compRate - baseRate) / baseRate * 100
			} else {
				spikePercent = 100
			}

			if spikePercent >= 50 { // 50% increase threshold
				spikes = append(spikes, ErrorSpike{
					Service:      svc,
					Level:        "error",
					BaselineRate: baseRate,
					CompareRate:  compRate,
					SpikePercent: spikePercent,
				})
			}
		}
	}

	sort.Slice(spikes, func(i, j int) bool {
		return spikes[i].SpikePercent > spikes[j].SpikePercent
	})

	return spikes
}

func (c *Comparer) rankTopChanges(result *CompareResult) []PatternChange {
	var allChanges []PatternChange

	allChanges = append(allChanges, result.NewPatterns...)
	allChanges = append(allChanges, result.IncreasedPatterns...)

	// Sort by significance and count
	sort.Slice(allChanges, func(i, j int) bool {
		// Prioritize by impact, then significance, then count
		impactOrder := map[string]int{"critical": 4, "high": 3, "medium": 2, "low": 1}
		if impactOrder[allChanges[i].Impact] != impactOrder[allChanges[j].Impact] {
			return impactOrder[allChanges[i].Impact] > impactOrder[allChanges[j].Impact]
		}
		if allChanges[i].Significance != allChanges[j].Significance {
			return allChanges[i].Significance > allChanges[j].Significance
		}
		return allChanges[i].CompareCount > allChanges[j].CompareCount
	})

	if len(allChanges) > 10 {
		return allChanges[:10]
	}
	return allChanges
}

func (c *Comparer) generateInsights(result *CompareResult) []Insight {
	var insights []Insight

	// Check for new error patterns
	newErrors := 0
	var newErrorPatterns []string
	for _, p := range result.NewPatterns {
		if p.Level == "error" || p.Level == "fatal" {
			newErrors++
			newErrorPatterns = append(newErrorPatterns, p.PatternID)
		}
	}
	if newErrors > 0 {
		insights = append(insights, Insight{
			Type:        "new_errors",
			Severity:    "critical",
			Title:       fmt.Sprintf("%d new error patterns detected", newErrors),
			Description: "These error patterns did not appear in the baseline period and may indicate new issues.",
			PatternIDs:  newErrorPatterns,
		})
	}

	// Check for error rate increase
	if result.CompareStats.ErrorRate > result.BaselineStats.ErrorRate*1.5 {
		insights = append(insights, Insight{
			Type:        "error_rate_spike",
			Severity:    "critical",
			Title:       fmt.Sprintf("Error rate increased %.0f%%", (result.CompareStats.ErrorRate/result.BaselineStats.ErrorRate-1)*100),
			Description: fmt.Sprintf("Error rate went from %.2f%% to %.2f%%", result.BaselineStats.ErrorRate, result.CompareStats.ErrorRate),
		})
	}

	// Check for volume spike
	if result.CompareStats.TotalLogs > result.BaselineStats.TotalLogs*2 {
		insights = append(insights, Insight{
			Type:        "volume_spike",
			Severity:    "warning",
			Title:       "Log volume doubled",
			Description: fmt.Sprintf("Log count increased from %d to %d", result.BaselineStats.TotalLogs, result.CompareStats.TotalLogs),
		})
	}

	// Check for missing patterns (potential silent failures)
	if len(result.MissingPatterns) > 5 {
		var missingIDs []string
		for _, p := range result.MissingPatterns[:5] {
			missingIDs = append(missingIDs, p.PatternID)
		}
		insights = append(insights, Insight{
			Type:        "missing_logs",
			Severity:    "warning",
			Title:       fmt.Sprintf("%d log patterns stopped appearing", len(result.MissingPatterns)),
			Description: "These patterns were present in baseline but missing in comparison period. This could indicate silent failures or service issues.",
			PatternIDs:  missingIDs,
		})
	}

	// Check for error spikes per service
	for _, spike := range result.ErrorSpikes {
		if spike.SpikePercent > 200 {
			insights = append(insights, Insight{
				Type:        "service_error_spike",
				Severity:    "critical",
				Title:       fmt.Sprintf("Service '%s' error rate spiked %.0f%%", spike.Service, spike.SpikePercent),
				Description: fmt.Sprintf("Error rate increased from %.2f/min to %.2f/min", spike.BaselineRate, spike.CompareRate),
			})
		}
	}

	return insights
}

func (c *Comparer) assessSeverity(result *CompareResult) (string, string) {
	// Count critical issues
	criticalCount := 0
	for _, insight := range result.Insights {
		if insight.Severity == "critical" {
			criticalCount++
		}
	}

	newErrorCount := 0
	for _, p := range result.NewPatterns {
		if p.Level == "error" || p.Level == "fatal" {
			newErrorCount++
		}
	}

	if criticalCount > 0 || newErrorCount > 3 {
		return "critical", fmt.Sprintf("Found %d critical issues including %d new error patterns", criticalCount, newErrorCount)
	}

	if len(result.NewPatterns) > 10 || len(result.ErrorSpikes) > 0 {
		return "warning", fmt.Sprintf("Found %d new log patterns and %d error spikes", len(result.NewPatterns), len(result.ErrorSpikes))
	}

	if len(result.NewPatterns) > 0 || len(result.IncreasedPatterns) > 0 {
		return "info", fmt.Sprintf("Found %d new patterns and %d patterns with increased volume", len(result.NewPatterns), len(result.IncreasedPatterns))
	}

	return "ok", "No significant changes detected between the two periods"
}

func (c *Comparer) calculateSignificance(baseline, compare int64) float64 {
	if baseline == 0 && compare == 0 {
		return 0
	}
	if baseline == 0 {
		return 1.0 // New pattern is highly significant
	}

	change := absFloat(float64(compare-baseline) / float64(baseline))
	// Normalize to 0-1 scale
	if change > 1 {
		return 1.0
	}
	return change
}

func (c *Comparer) determineImpact(baseline, compare int64, level string) string {
	// Error/fatal logs have higher impact
	if level == "error" || level == "fatal" {
		if compare > baseline*2 || (baseline == 0 && compare > 10) {
			return "critical"
		}
		return "high"
	}

	// Warning logs
	if level == "warn" || level == "warning" {
		if compare > baseline*3 {
			return "high"
		}
		return "medium"
	}

	// Info/debug logs
	if compare > baseline*5 {
		return "medium"
	}
	return "low"
}

// normalizeMessage converts a log message to a pattern by replacing variable parts
func normalizeMessage(msg string) string {
	// Replace common variable patterns
	patterns := []struct {
		re   *regexp.Regexp
		repl string
	}{
		// UUIDs
		{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<UUID>"},
		// Hex IDs (32+ chars)
		{regexp.MustCompile(`[0-9a-fA-F]{32,}`), "<HEX_ID>"},
		// IP addresses
		{regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`), "<IP>"},
		// Timestamps
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}`), "<TIMESTAMP>"},
		// Numbers
		{regexp.MustCompile(`\b\d+\b`), "<NUM>"},
		// Quoted strings
		{regexp.MustCompile(`"[^"]*"`), `"<STR>"`},
		{regexp.MustCompile(`'[^']*'`), `'<STR>'`},
		// File paths
		{regexp.MustCompile(`/[a-zA-Z0-9_/.-]+`), "<PATH>"},
		// Email addresses
		{regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`), "<EMAIL>"},
	}

	result := msg
	for _, p := range patterns {
		result = p.re.ReplaceAllString(result, p.repl)
	}

	// Collapse multiple spaces
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// hashPattern creates a unique ID for a pattern
func hashPattern(pattern string) string {
	hash := md5.Sum([]byte(pattern))
	return fmt.Sprintf("%x", hash)[:12]
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
