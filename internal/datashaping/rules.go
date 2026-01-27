package datashaping

import (
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// RuleAction defines what to do with matching data
type RuleAction string

const (
	ActionDrop      RuleAction = "drop"      // Discard the data entirely
	ActionSample    RuleAction = "sample"    // Keep only a percentage
	ActionAggregate RuleAction = "aggregate" // Combine into aggregated form
	ActionKeep      RuleAction = "keep"      // Explicitly keep (useful for exceptions)
	ActionTransform RuleAction = "transform" // Modify tags/fields
)

// DataType for rule matching
type DataType string

const (
	DataTypeMetric DataType = "metric"
	DataTypeLog    DataType = "log"
	DataTypeTrace  DataType = "trace"
	DataTypeAll    DataType = "all"
)

// Rule defines a data shaping rule
type Rule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	Priority    int               `json:"priority"` // Lower = higher priority
	DataType    DataType          `json:"data_type"`
	Action      RuleAction        `json:"action"`

	// Matching conditions (all must match)
	NamePattern   string            `json:"name_pattern,omitempty"`   // Regex for metric/log name
	TagMatches    map[string]string `json:"tag_matches,omitempty"`    // Exact tag matches
	TagPatterns   map[string]string `json:"tag_patterns,omitempty"`   // Regex tag matches
	LevelMatch    string            `json:"level_match,omitempty"`    // For logs: debug, info, warn, error
	ServiceMatch  string            `json:"service_match,omitempty"`  // Service name pattern

	// Action parameters
	SampleRate    float64           `json:"sample_rate,omitempty"`    // For sample action (0.0-1.0)
	AggregateBy   []string          `json:"aggregate_by,omitempty"`   // Tags to keep when aggregating
	DropTags      []string          `json:"drop_tags,omitempty"`      // Tags to remove
	AddTags       map[string]string `json:"add_tags,omitempty"`       // Tags to add

	// Metadata
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by,omitempty"`

	// Compiled patterns (not serialized)
	nameRegex    *regexp.Regexp
	tagRegexes   map[string]*regexp.Regexp
	serviceRegex *regexp.Regexp
}

// RuleStats tracks rule effectiveness
type RuleStats struct {
	RuleID        string    `json:"rule_id"`
	RuleName      string    `json:"rule_name"`
	TotalMatched  int64     `json:"total_matched"`
	TotalDropped  int64     `json:"total_dropped"`
	TotalSampled  int64     `json:"total_sampled"`
	TotalKept     int64     `json:"total_kept"`
	BytesSaved    int64     `json:"bytes_saved_estimate"`
	LastMatched   time.Time `json:"last_matched,omitempty"`
}

// EngineStats provides overall statistics
type EngineStats struct {
	TotalProcessed   int64                `json:"total_processed"`
	TotalDropped     int64                `json:"total_dropped"`
	TotalSampled     int64                `json:"total_sampled"`
	TotalTransformed int64                `json:"total_transformed"`
	TotalKept        int64                `json:"total_kept"`
	BytesSavedEst    int64                `json:"bytes_saved_estimate"`
	RuleStats        map[string]*RuleStats `json:"rule_stats"`
	ProcessedByType  map[DataType]int64   `json:"processed_by_type"`
	DroppedByType    map[DataType]int64   `json:"dropped_by_type"`
}

// Decision represents the engine's decision for a data point
type Decision struct {
	Action      RuleAction
	RuleID      string
	RuleName    string
	SampleRate  float64
	DropTags    []string
	AddTags     map[string]string
	AggregateBy []string
}

// Engine evaluates data shaping rules
type Engine struct {
	mu    sync.RWMutex
	rules []*Rule // Sorted by priority

	// Statistics
	stats       EngineStats
	ruleStats   map[string]*RuleStats

	// Sampling state
	sampleCounter uint64
}

// NewEngine creates a new data shaping engine
func NewEngine() *Engine {
	return &Engine{
		rules:     make([]*Rule, 0),
		ruleStats: make(map[string]*RuleStats),
		stats: EngineStats{
			RuleStats:       make(map[string]*RuleStats),
			ProcessedByType: make(map[DataType]int64),
			DroppedByType:   make(map[DataType]int64),
		},
	}
}

// AddRule adds a rule to the engine
func (e *Engine) AddRule(rule *Rule) error {
	if err := rule.compile(); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Remove existing rule with same ID
	e.removeRuleLocked(rule.ID)

	// Add and sort by priority
	e.rules = append(e.rules, rule)
	e.sortRulesLocked()

	// Initialize stats
	e.ruleStats[rule.ID] = &RuleStats{
		RuleID:   rule.ID,
		RuleName: rule.Name,
	}
	e.stats.RuleStats[rule.ID] = e.ruleStats[rule.ID]

	return nil
}

// RemoveRule removes a rule from the engine
func (e *Engine) RemoveRule(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.removeRuleLocked(id)
}

func (e *Engine) removeRuleLocked(id string) bool {
	for i, r := range e.rules {
		if r.ID == id {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			delete(e.ruleStats, id)
			delete(e.stats.RuleStats, id)
			return true
		}
	}
	return false
}

// GetRules returns all rules
func (e *Engine) GetRules() []*Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	rules := make([]*Rule, len(e.rules))
	copy(rules, e.rules)
	return rules
}

// GetRule returns a specific rule
func (e *Engine) GetRule(id string) *Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, r := range e.rules {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// Evaluate evaluates a data point against all rules
func (e *Engine) Evaluate(dataType DataType, name string, tags map[string]string, level string, service string, sizeBytes int) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Update processed count
	atomic.AddInt64(&e.stats.TotalProcessed, 1)
	e.stats.ProcessedByType[dataType]++

	// Default decision: keep
	decision := Decision{Action: ActionKeep}

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		if rule.matches(dataType, name, tags, level, service) {
			decision = e.applyRule(rule, sizeBytes)

			// Update rule stats
			if stats, ok := e.ruleStats[rule.ID]; ok {
				atomic.AddInt64(&stats.TotalMatched, 1)
				stats.LastMatched = time.Now()

				switch decision.Action {
				case ActionDrop:
					atomic.AddInt64(&stats.TotalDropped, 1)
					atomic.AddInt64(&stats.BytesSaved, int64(sizeBytes))
				case ActionSample:
					atomic.AddInt64(&stats.TotalSampled, 1)
				case ActionKeep:
					atomic.AddInt64(&stats.TotalKept, 1)
				}
			}

			// First matching rule wins
			break
		}
	}

	// Update global stats
	switch decision.Action {
	case ActionDrop:
		atomic.AddInt64(&e.stats.TotalDropped, 1)
		atomic.AddInt64(&e.stats.BytesSavedEst, int64(sizeBytes))
		e.stats.DroppedByType[dataType]++
	case ActionSample:
		atomic.AddInt64(&e.stats.TotalSampled, 1)
	case ActionTransform:
		atomic.AddInt64(&e.stats.TotalTransformed, 1)
	case ActionKeep:
		atomic.AddInt64(&e.stats.TotalKept, 1)
	}

	return decision
}

// EvaluateMetric is a convenience method for metrics
func (e *Engine) EvaluateMetric(name string, tags map[string]string, sizeBytes int) Decision {
	return e.Evaluate(DataTypeMetric, name, tags, "", "", sizeBytes)
}

// EvaluateLog is a convenience method for logs
func (e *Engine) EvaluateLog(service string, level string, tags map[string]string, sizeBytes int) Decision {
	return e.Evaluate(DataTypeLog, "", tags, level, service, sizeBytes)
}

// EvaluateTrace is a convenience method for traces
func (e *Engine) EvaluateTrace(service string, tags map[string]string, sizeBytes int) Decision {
	return e.Evaluate(DataTypeTrace, "", tags, "", service, sizeBytes)
}

// ShouldSample returns true if the data should be kept based on sample rate
func (e *Engine) ShouldSample(rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	counter := atomic.AddUint64(&e.sampleCounter, 1)
	threshold := uint64(rate * 100)
	return (counter % 100) < threshold
}

// GetStats returns engine statistics
func (e *Engine) GetStats() EngineStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Deep copy stats
	stats := EngineStats{
		TotalProcessed:   atomic.LoadInt64(&e.stats.TotalProcessed),
		TotalDropped:     atomic.LoadInt64(&e.stats.TotalDropped),
		TotalSampled:     atomic.LoadInt64(&e.stats.TotalSampled),
		TotalTransformed: atomic.LoadInt64(&e.stats.TotalTransformed),
		TotalKept:        atomic.LoadInt64(&e.stats.TotalKept),
		BytesSavedEst:    atomic.LoadInt64(&e.stats.BytesSavedEst),
		RuleStats:        make(map[string]*RuleStats),
		ProcessedByType:  make(map[DataType]int64),
		DroppedByType:    make(map[DataType]int64),
	}

	for k, v := range e.stats.ProcessedByType {
		stats.ProcessedByType[k] = v
	}
	for k, v := range e.stats.DroppedByType {
		stats.DroppedByType[k] = v
	}
	for k, v := range e.ruleStats {
		statsCopy := *v
		stats.RuleStats[k] = &statsCopy
	}

	return stats
}

// ResetStats resets all statistics
func (e *Engine) ResetStats() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.stats = EngineStats{
		RuleStats:       make(map[string]*RuleStats),
		ProcessedByType: make(map[DataType]int64),
		DroppedByType:   make(map[DataType]int64),
	}

	for id, rule := range e.ruleStats {
		e.ruleStats[id] = &RuleStats{
			RuleID:   rule.RuleID,
			RuleName: rule.RuleName,
		}
		e.stats.RuleStats[id] = e.ruleStats[id]
	}
}

func (e *Engine) applyRule(rule *Rule, sizeBytes int) Decision {
	decision := Decision{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		Action:   rule.Action,
	}

	switch rule.Action {
	case ActionSample:
		decision.SampleRate = rule.SampleRate
		if !e.ShouldSample(rule.SampleRate) {
			decision.Action = ActionDrop
		}
	case ActionTransform:
		decision.DropTags = rule.DropTags
		decision.AddTags = rule.AddTags
	case ActionAggregate:
		decision.AggregateBy = rule.AggregateBy
	}

	return decision
}

func (e *Engine) sortRulesLocked() {
	// Simple bubble sort (rules list is typically small)
	for i := 0; i < len(e.rules)-1; i++ {
		for j := i + 1; j < len(e.rules); j++ {
			if e.rules[j].Priority < e.rules[i].Priority {
				e.rules[i], e.rules[j] = e.rules[j], e.rules[i]
			}
		}
	}
}

// Rule methods

func (r *Rule) compile() error {
	if r.NamePattern != "" {
		regex, err := regexp.Compile(r.NamePattern)
		if err != nil {
			return err
		}
		r.nameRegex = regex
	}

	if r.ServiceMatch != "" {
		regex, err := regexp.Compile(r.ServiceMatch)
		if err != nil {
			return err
		}
		r.serviceRegex = regex
	}

	r.tagRegexes = make(map[string]*regexp.Regexp)
	for k, v := range r.TagPatterns {
		regex, err := regexp.Compile(v)
		if err != nil {
			return err
		}
		r.tagRegexes[k] = regex
	}

	return nil
}

func (r *Rule) matches(dataType DataType, name string, tags map[string]string, level string, service string) bool {
	// Check data type
	if r.DataType != DataTypeAll && r.DataType != dataType {
		return false
	}

	// Check name pattern
	if r.nameRegex != nil && !r.nameRegex.MatchString(name) {
		return false
	}

	// Check service pattern
	if r.serviceRegex != nil && !r.serviceRegex.MatchString(service) {
		return false
	}

	// Check log level
	if r.LevelMatch != "" && r.LevelMatch != level {
		return false
	}

	// Check exact tag matches
	for k, v := range r.TagMatches {
		if tags[k] != v {
			return false
		}
	}

	// Check tag patterns
	for k, regex := range r.tagRegexes {
		tagVal, ok := tags[k]
		if !ok || !regex.MatchString(tagVal) {
			return false
		}
	}

	return true
}

// Helper to create common rules

// NewDropRule creates a simple drop rule
func NewDropRule(id, name string, dataType DataType, namePattern string) *Rule {
	return &Rule{
		ID:          id,
		Name:        name,
		Enabled:     true,
		Priority:    100,
		DataType:    dataType,
		Action:      ActionDrop,
		NamePattern: namePattern,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewSampleRule creates a sampling rule
func NewSampleRule(id, name string, dataType DataType, namePattern string, rate float64) *Rule {
	return &Rule{
		ID:          id,
		Name:        name,
		Enabled:     true,
		Priority:    100,
		DataType:    dataType,
		Action:      ActionSample,
		NamePattern: namePattern,
		SampleRate:  rate,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// NewDropTagRule creates a rule that removes specific tags
func NewDropTagRule(id, name string, dataType DataType, namePattern string, dropTags []string) *Rule {
	return &Rule{
		ID:          id,
		Name:        name,
		Enabled:     true,
		Priority:    100,
		DataType:    dataType,
		Action:      ActionTransform,
		NamePattern: namePattern,
		DropTags:    dropTags,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
