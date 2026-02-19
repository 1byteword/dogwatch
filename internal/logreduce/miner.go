package logreduce

import (
	"sync"
	"time"
)

// Miner implements log pattern mining using a Drain-like algorithm
// Drain is a fixed-depth tree-based algorithm that efficiently parses logs
type Miner struct {
	mu sync.RWMutex

	// Pattern storage
	patterns    map[string]*Pattern // patternID -> pattern
	lengthIndex map[int][]*Pattern  // token count -> patterns

	// Configuration
	similarityThreshold float64 // 0-1, how similar tokens must be
	maxChildren         int     // Max patterns per length group
	maxPatterns         int     // Max total patterns to store

	// Statistics
	totalProcessed int64
	lastUpdate     time.Time
}

// MinerConfig configures the pattern miner
type MinerConfig struct {
	SimilarityThreshold float64 `json:"similarity_threshold"` // Default 0.5
	MaxChildren         int     `json:"max_children"`         // Default 100
	MaxPatterns         int     `json:"max_patterns"`         // Default 10000
}

// DefaultMinerConfig returns default configuration
func DefaultMinerConfig() MinerConfig {
	return MinerConfig{
		SimilarityThreshold: 0.5,
		MaxChildren:         100,
		MaxPatterns:         10000,
	}
}

func NewMiner(config MinerConfig) *Miner {
	if config.SimilarityThreshold == 0 {
		config.SimilarityThreshold = 0.5
	}
	if config.MaxChildren == 0 {
		config.MaxChildren = 100
	}
	if config.MaxPatterns == 0 {
		config.MaxPatterns = 10000
	}

	return &Miner{
		patterns:            make(map[string]*Pattern),
		lengthIndex:         make(map[int][]*Pattern),
		similarityThreshold: config.SimilarityThreshold,
		maxChildren:         config.MaxChildren,
		maxPatterns:         config.MaxPatterns,
	}
}

// Process processes a single log entry and returns the matched/created pattern
func (m *Miner) Process(entry LogEntry) *Pattern {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalProcessed++
	m.lastUpdate = time.Now()

	// Tokenize the message
	tokens := Tokenize(entry.Message)
	tokenCount := len(tokens)

	if tokenCount == 0 {
		return nil
	}

	// Look for existing pattern with same token count
	candidates := m.lengthIndex[tokenCount]

	var bestMatch *Pattern
	var bestSimilarity float64

	for _, candidate := range candidates {
		similarity := m.calculateSimilarity(tokens, candidate)
		if similarity > bestSimilarity && similarity >= m.similarityThreshold {
			bestMatch = candidate
			bestSimilarity = similarity
		}
	}

	if bestMatch != nil {
		// Update existing pattern
		bestMatch.Count++
		bestMatch.LastSeen = entry.Timestamp
		if entry.Timestamp.Before(bestMatch.FirstSeen) {
			bestMatch.FirstSeen = entry.Timestamp
		}

		// Merge template (make variable tokens more general if needed)
		m.mergeTemplate(bestMatch, tokens)

		return bestMatch
	}

	// Create new pattern
	if len(m.patterns) >= m.maxPatterns {
		// Evict least used pattern
		m.evictPattern()
	}

	template := CreateTemplate(tokens)
	patternID := HashTemplate(template)

	// Check if this exact template already exists
	if existing, ok := m.patterns[patternID]; ok {
		existing.Count++
		existing.LastSeen = entry.Timestamp
		return existing
	}

	pattern := &Pattern{
		ID:            patternID,
		Template:      template,
		Tokens:        m.createTokens(tokens),
		SampleMessage: entry.Message,
		Count:         1,
		FirstSeen:     entry.Timestamp,
		LastSeen:      entry.Timestamp,
		Level:         entry.Level,
		Service:       entry.Service,
		VariableCount: countVariables(template),
		Variability:   CalculateVariability(template),
		Trend:         TrendNew,
	}

	m.patterns[patternID] = pattern
	m.lengthIndex[tokenCount] = append(m.lengthIndex[tokenCount], pattern)

	return pattern
}

// ProcessBatch processes multiple log entries
func (m *Miner) ProcessBatch(entries []LogEntry) *ReduceResult {
	start := time.Now()

	result := &ReduceResult{
		InputCount:  int64(len(entries)),
		ProcessedAt: start,
	}

	patternCounts := make(map[string]int64)

	for _, entry := range entries {
		pattern := m.Process(entry)
		if pattern != nil {
			patternCounts[pattern.ID]++
		} else {
			result.Unmatched++
		}
	}

	result.PatternCount = len(patternCounts)
	result.Patterns = m.GetPatterns()
	result.ProcessingMs = time.Since(start).Milliseconds()

	if result.PatternCount > 0 {
		result.Reduction = float64(result.InputCount) / float64(result.PatternCount)
	}

	return result
}

// calculateSimilarity calculates similarity between tokens and a pattern
func (m *Miner) calculateSimilarity(tokens []string, pattern *Pattern) float64 {
	tmplTokens := pattern.Tokens
	if len(tokens) != len(tmplTokens) {
		return 0
	}

	matches := 0
	for i, token := range tokens {
		if tmplTokens[i].Variable {
			// Variable tokens always match
			matches++
		} else if tmplTokens[i].Value == token {
			// Exact match
			matches++
		} else {
			// Check if token should be variable
			_, isVar := NormalizeToken(token)
			if isVar {
				matches++
			}
		}
	}

	return float64(matches) / float64(len(tokens))
}

// mergeTemplate updates a pattern's template with new information
func (m *Miner) mergeTemplate(pattern *Pattern, tokens []string) {
	if len(tokens) != len(pattern.Tokens) {
		return
	}

	changed := false
	for i, token := range tokens {
		if !pattern.Tokens[i].Variable {
			if pattern.Tokens[i].Value != token {
				// Token differs, make it variable
				pattern.Tokens[i].Variable = true
				pattern.Tokens[i].Value = "<*>"
				changed = true
			}
		}
	}

	if changed {
		// Rebuild template string
		var parts []string
		for _, t := range pattern.Tokens {
			parts = append(parts, t.Value)
		}
		pattern.Template = joinTokens(parts)
		pattern.VariableCount = countVariables(pattern.Template)
		pattern.Variability = CalculateVariability(pattern.Template)
	}
}

func (m *Miner) createTokens(tokens []string) []Token {
	result := make([]Token, len(tokens))
	for i, t := range tokens {
		norm, isVar := NormalizeToken(t)
		result[i] = Token{
			Value:    norm,
			Variable: isVar,
			Position: i,
		}
	}
	return result
}

func (m *Miner) evictPattern() {
	// Find pattern with lowest count
	var minPattern *Pattern
	var minCount int64 = -1

	for _, p := range m.patterns {
		if minCount < 0 || p.Count < minCount {
			minPattern = p
			minCount = p.Count
		}
	}

	if minPattern != nil {
		delete(m.patterns, minPattern.ID)
		// Remove from length index
		tokenCount := len(minPattern.Tokens)
		patterns := m.lengthIndex[tokenCount]
		for i, p := range patterns {
			if p.ID == minPattern.ID {
				m.lengthIndex[tokenCount] = append(patterns[:i], patterns[i+1:]...)
				break
			}
		}
	}
}

// GetPatterns returns all patterns
func (m *Miner) GetPatterns() []*Pattern {
	m.mu.RLock()
	defer m.mu.RUnlock()

	patterns := make([]*Pattern, 0, len(m.patterns))
	for _, p := range m.patterns {
		patterns = append(patterns, p)
	}

	SortPatternsByCount(patterns)
	return patterns
}

// GetPattern returns a specific pattern by ID
func (m *Miner) GetPattern(id string) *Pattern {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.patterns[id]
}

// GetStats returns mining statistics
func (m *Miner) GetStats() *PatternStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &PatternStats{
		TotalPatterns: len(m.patterns),
		TotalLogs:     m.totalProcessed,
		ByLevel:       make(map[string]int),
		ByService:     make(map[string]int),
		AnalyzedAt:    time.Now(),
	}

	hourAgo := time.Now().Add(-time.Hour)
	var totalCount int64

	for _, p := range m.patterns {
		totalCount += p.Count

		if p.Level != "" {
			stats.ByLevel[p.Level]++
		}
		if p.Service != "" {
			stats.ByService[p.Service]++
		}
		if p.FirstSeen.After(hourAgo) {
			stats.NewPatterns++
		}
	}

	// Calculate coverage
	if m.totalProcessed > 0 {
		stats.CoveragePercent = float64(totalCount) / float64(m.totalProcessed) * 100
	}

	// Get top patterns
	patterns := m.GetPatterns()
	if len(patterns) > 10 {
		stats.TopPatterns = patterns[:10]
	} else {
		stats.TopPatterns = patterns
	}

	// Get trending
	stats.TrendingUp = GetTrendingPatterns(patterns)
	if len(stats.TrendingUp) > 5 {
		stats.TrendingUp = stats.TrendingUp[:5]
	}

	return stats
}

// UpdateTrends recalculates trend information for all patterns
func (m *Miner) UpdateTrends(hourlyCountGetter func(patternID string) int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hourAgo := time.Now().Add(-time.Hour)

	for _, p := range m.patterns {
		p.CountLastHour = hourlyCountGetter(p.ID)

		// Determine trend
		if p.FirstSeen.After(hourAgo) {
			p.Trend = TrendNew
			p.TrendPercent = 100
		} else if p.CountLastHour == 0 && p.LastSeen.Before(hourAgo) {
			p.Trend = TrendGone
			p.TrendPercent = -100
		} else {
			// Compare to average
			avgPerHour := float64(p.Count) / time.Since(p.FirstSeen).Hours()
			if avgPerHour > 0 {
				change := (float64(p.CountLastHour) - avgPerHour) / avgPerHour * 100
				p.TrendPercent = change

				if change > 50 {
					p.Trend = TrendIncreasing
				} else if change < -50 {
					p.Trend = TrendDecreasing
				} else {
					p.Trend = TrendStable
				}
			}
		}
	}
}

// Match finds the pattern that matches a message
func (m *Miner) Match(message string) (*PatternMatch, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens := Tokenize(message)
	tokenCount := len(tokens)

	candidates := m.lengthIndex[tokenCount]

	var bestMatch *Pattern
	var bestVars map[string]string
	var bestConfidence float64

	for _, candidate := range candidates {
		matches, vars, confidence := MatchPattern(message, candidate.Template)
		if matches && confidence > bestConfidence {
			bestMatch = candidate
			bestVars = vars
			bestConfidence = confidence
		}
	}

	if bestMatch != nil {
		return &PatternMatch{
			PatternID:  bestMatch.ID,
			Message:    message,
			Variables:  bestVars,
			Timestamp:  time.Now(),
			Confidence: bestConfidence,
		}, true
	}

	return nil, false
}

// Search finds patterns matching a query
func (m *Miner) Search(query string) []*Pattern {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*Pattern
	queryLower := toLower(query)

	for _, p := range m.patterns {
		if contains(toLower(p.Template), queryLower) ||
		   contains(toLower(p.SampleMessage), queryLower) {
			results = append(results, p)
		}
	}

	SortPatternsByCount(results)
	return results
}

// Clear removes all patterns
func (m *Miner) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.patterns = make(map[string]*Pattern)
	m.lengthIndex = make(map[int][]*Pattern)
	m.totalProcessed = 0
}

// Export exports patterns for persistence
func (m *Miner) Export() []*Pattern {
	return m.GetPatterns()
}

// Import imports patterns from persistence
func (m *Miner) Import(patterns []*Pattern) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range patterns {
		m.patterns[p.ID] = p
		tokenCount := len(p.Tokens)
		m.lengthIndex[tokenCount] = append(m.lengthIndex[tokenCount], p)
	}
}

func countVariables(template string) int {
	count := 0
	for _, c := range template {
		if c == '*' {
			count++
		}
	}
	return count
}

func joinTokens(tokens []string) string {
	return join(tokens, " ")
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
