package sampling

import (
	"encoding/binary"
	"hash/fnv"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dogwatch/internal/trace"
)

// HeadSampler implements head-based sampling with priority rules
// Decisions are made at trace start (first span) and cached for consistency
type HeadSampler struct {
	config HeadSamplerConfig

	// Cached decisions for trace IDs
	decisions     map[string]*cachedDecision
	decisionsMu   sync.RWMutex
	decisionCount int64

	// Stats
	totalSpans   int64
	sampledSpans int64
	droppedSpans int64

	// Compiled regex patterns for rules
	compiledRules []compiledRule

	// Cleanup ticker
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

type cachedDecision struct {
	decision  Decision
	reason    string
	ruleID    string
	timestamp time.Time
}

type compiledRule struct {
	rule            Rule
	servicePattern  *regexp.Regexp
	operationPattern *regexp.Regexp
}

// NewHeadSampler creates a new head-based sampler
func NewHeadSampler(config HeadSamplerConfig) *HeadSampler {
	hs := &HeadSampler{
		config:    config,
		decisions: make(map[string]*cachedDecision),
		stopCh:    make(chan struct{}),
	}

	// Compile rules
	hs.compileRules()

	// Start cleanup goroutine
	hs.cleanupTicker = time.NewTicker(time.Minute)
	go hs.cleanupLoop()

	return hs
}

// compileRules pre-compiles regex patterns for rules
func (hs *HeadSampler) compileRules() {
	var compiled []compiledRule

	// Sort rules by priority (higher first)
	rules := make([]Rule, len(hs.config.Rules))
	copy(rules, hs.config.Rules)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		cr := compiledRule{rule: rule}

		// Compile service pattern
		if rule.Condition.Service != "" {
			pattern := wildcardToRegex(rule.Condition.Service)
			cr.servicePattern, _ = regexp.Compile(pattern)
		}

		// Compile operation pattern
		if rule.Condition.Operation != "" {
			pattern := wildcardToRegex(rule.Condition.Operation)
			cr.operationPattern, _ = regexp.Compile(pattern)
		}

		compiled = append(compiled, cr)
	}

	hs.compiledRules = compiled
}

// wildcardToRegex converts a wildcard pattern to regex
func wildcardToRegex(pattern string) string {
	// Escape special regex characters except *
	escaped := regexp.QuoteMeta(pattern)
	// Replace escaped * with .*
	return "^" + strings.ReplaceAll(escaped, `\*`, ".*") + "$"
}

// ShouldSample returns the sampling decision for a span
func (hs *HeadSampler) ShouldSample(span *trace.Span) Decision {
	if !hs.config.Enabled {
		return DecisionKeep
	}

	atomic.AddInt64(&hs.totalSpans, 1)

	// Check cached decision for this trace
	hs.decisionsMu.RLock()
	cached, exists := hs.decisions[span.TraceID]
	hs.decisionsMu.RUnlock()

	if exists {
		if cached.decision == DecisionKeep {
			atomic.AddInt64(&hs.sampledSpans, 1)
		} else {
			atomic.AddInt64(&hs.droppedSpans, 1)
		}
		return cached.decision
	}

	// Make new decision (this is the first span of the trace)
	result := hs.makeDecision(span)

	// Cache the decision
	hs.cacheDecision(span.TraceID, result)

	if result.Decision == DecisionKeep {
		atomic.AddInt64(&hs.sampledSpans, 1)
	} else {
		atomic.AddInt64(&hs.droppedSpans, 1)
	}

	return result.Decision
}

// makeDecision evaluates rules and makes a sampling decision
func (hs *HeadSampler) makeDecision(span *trace.Span) SamplingResult {
	// Evaluate rules in priority order
	for _, cr := range hs.compiledRules {
		if hs.matchesRule(span, cr) {
			// Determine final decision based on rule action and sample rate
			decision := cr.rule.Action
			if decision == DecisionKeep && cr.rule.SampleRate < 1.0 {
				// Apply probabilistic sampling
				if !hs.shouldSampleByRate(span.TraceID, cr.rule.SampleRate) {
					decision = DecisionDrop
				}
			}

			return SamplingResult{
				Decision:   decision,
				Reason:     "rule:" + cr.rule.Name,
				RuleID:     cr.rule.ID,
				SampleRate: cr.rule.SampleRate,
				Timestamp:  time.Now(),
			}
		}
	}

	// No rule matched, use default sampling
	decision := DecisionKeep
	if !hs.shouldSampleByRate(span.TraceID, 0.1) { // 10% default
		decision = DecisionDrop
	}

	return SamplingResult{
		Decision:   decision,
		Reason:     "default",
		SampleRate: 0.1,
		Timestamp:  time.Now(),
	}
}

// matchesRule checks if a span matches a rule's conditions
func (hs *HeadSampler) matchesRule(span *trace.Span, cr compiledRule) bool {
	cond := cr.rule.Condition

	// Check service pattern
	if cr.servicePattern != nil {
		if !cr.servicePattern.MatchString(span.ServiceName) {
			return false
		}
	}

	// Check operation pattern
	if cr.operationPattern != nil {
		if !cr.operationPattern.MatchString(span.Name) {
			return false
		}
	}

	// Check latency threshold
	if cond.MinLatencyMs > 0 {
		if span.DurationMs < cond.MinLatencyMs {
			return false
		}
	}

	// Check error status
	if cond.HasError != nil {
		hasError := span.Status == "ERROR"
		if *cond.HasError != hasError {
			return false
		}
	}

	// Check span kind
	if cond.SpanKind != "" {
		if span.Kind != cond.SpanKind {
			return false
		}
	}

	// Check attributes
	if len(cond.Attributes) > 0 {
		for key, value := range cond.Attributes {
			if span.Attributes == nil {
				return false
			}
			if spanValue, ok := span.Attributes[key]; !ok || spanValue != value {
				return false
			}
		}
	}

	return true
}

// shouldSampleByRate uses consistent hashing based on trace ID
// This ensures the same trace ID always gets the same decision
func (hs *HeadSampler) shouldSampleByRate(traceID string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	// Hash the trace ID for consistent sampling
	h := fnv.New64a()
	h.Write([]byte(traceID))
	hashValue := h.Sum64()

	// Convert hash to a value between 0 and 1
	normalized := float64(hashValue) / float64(^uint64(0))

	return normalized < rate
}

// cacheDecision stores a sampling decision for a trace ID
func (hs *HeadSampler) cacheDecision(traceID string, result SamplingResult) {
	hs.decisionsMu.Lock()
	defer hs.decisionsMu.Unlock()

	// Check if we need to evict old entries
	if len(hs.decisions) >= hs.config.MaxCachedDecisions {
		hs.evictOldDecisions()
	}

	hs.decisions[traceID] = &cachedDecision{
		decision:  result.Decision,
		reason:    result.Reason,
		ruleID:    result.RuleID,
		timestamp: result.Timestamp,
	}
	atomic.AddInt64(&hs.decisionCount, 1)
}

// evictOldDecisions removes expired decisions
func (hs *HeadSampler) evictOldDecisions() {
	now := time.Now()
	ttl := hs.config.DecisionTTL

	for traceID, decision := range hs.decisions {
		if now.Sub(decision.timestamp) > ttl {
			delete(hs.decisions, traceID)
		}
	}
}

// cleanupLoop periodically cleans up expired decisions
func (hs *HeadSampler) cleanupLoop() {
	for {
		select {
		case <-hs.cleanupTicker.C:
			hs.decisionsMu.Lock()
			hs.evictOldDecisions()
			hs.decisionsMu.Unlock()
		case <-hs.stopCh:
			return
		}
	}
}

// GetStats returns current sampler statistics
func (hs *HeadSampler) GetStats() SamplerStats {
	total := atomic.LoadInt64(&hs.totalSpans)
	sampled := atomic.LoadInt64(&hs.sampledSpans)
	dropped := atomic.LoadInt64(&hs.droppedSpans)

	var rate float64
	if total > 0 {
		rate = float64(sampled) / float64(total)
	}

	return SamplerStats{
		TotalSpans:   total,
		SampledSpans: sampled,
		DroppedSpans: dropped,
		CurrentRate:  rate,
	}
}

// UpdateConfig updates the sampler configuration
func (hs *HeadSampler) UpdateConfig(config HeadSamplerConfig) {
	hs.decisionsMu.Lock()
	hs.config = config
	hs.compileRules()
	hs.decisionsMu.Unlock()
}

// GetRules returns the current rules
func (hs *HeadSampler) GetRules() []Rule {
	hs.decisionsMu.RLock()
	defer hs.decisionsMu.RUnlock()
	return append([]Rule{}, hs.config.Rules...)
}

// AddRule adds a new sampling rule
func (hs *HeadSampler) AddRule(rule Rule) {
	hs.decisionsMu.Lock()
	hs.config.Rules = append(hs.config.Rules, rule)
	hs.compileRules()
	hs.decisionsMu.Unlock()
}

// RemoveRule removes a rule by ID
func (hs *HeadSampler) RemoveRule(ruleID string) bool {
	hs.decisionsMu.Lock()
	defer hs.decisionsMu.Unlock()

	for i, rule := range hs.config.Rules {
		if rule.ID == ruleID {
			hs.config.Rules = append(hs.config.Rules[:i], hs.config.Rules[i+1:]...)
			hs.compileRules()
			return true
		}
	}
	return false
}

// UpdateRule updates an existing rule
func (hs *HeadSampler) UpdateRule(rule Rule) bool {
	hs.decisionsMu.Lock()
	defer hs.decisionsMu.Unlock()

	for i, r := range hs.config.Rules {
		if r.ID == rule.ID {
			hs.config.Rules[i] = rule
			hs.compileRules()
			return true
		}
	}
	return false
}

// GetDecision returns the cached decision for a trace ID
func (hs *HeadSampler) GetDecision(traceID string) (*SamplingResult, bool) {
	hs.decisionsMu.RLock()
	defer hs.decisionsMu.RUnlock()

	cached, exists := hs.decisions[traceID]
	if !exists {
		return nil, false
	}

	return &SamplingResult{
		Decision:  cached.decision,
		Reason:    cached.reason,
		RuleID:    cached.ruleID,
		Timestamp: cached.timestamp,
	}, true
}

// Stop stops the head sampler cleanup goroutine
func (hs *HeadSampler) Stop() {
	close(hs.stopCh)
	hs.cleanupTicker.Stop()
}

// traceIDToUint64 converts a hex trace ID to uint64 for hashing
func traceIDToUint64(traceID string) uint64 {
	if len(traceID) < 16 {
		return 0
	}
	// Take first 8 bytes (16 hex chars)
	b := make([]byte, 8)
	for i := 0; i < 8 && i*2+1 < len(traceID); i++ {
		b[i] = hexToByte(traceID[i*2], traceID[i*2+1])
	}
	return binary.BigEndian.Uint64(b)
}

func hexToByte(hi, lo byte) byte {
	return hexDigit(hi)<<4 | hexDigit(lo)
}

func hexDigit(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
