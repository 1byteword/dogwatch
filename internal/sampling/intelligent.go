// Package sampling provides intelligent tail sampling with retroactive sampling,
// anomaly awareness, cost tracking, and learning capabilities.
package sampling

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"dogwatch/internal/trace"
)

// IntelligentSampler extends tail sampling with advanced capabilities
type IntelligentSampler struct {
	config IntelligentSamplerConfig

	// Base tail sampler
	baseSampler *TailSampler

	// Retroactive sampling - ring buffer of recent trace decisions
	recentDecisions   *ring.Ring
	recentDecisionsMu sync.RWMutex
	decisionIndex     map[string]*TraceDecision

	// Anomaly detection
	normalPatterns   *PatternLearner
	anomalyThreshold float64

	// Cost tracking
	costTracker   *CostTracker
	budgetControl *BudgetController

	// Learning mode
	learner        *SamplingLearner
	learningActive bool

	// Stats
	totalSpans          int64
	retroactiveCaptures int64
	anomalySamples      int64
	budgetAdjustments   int64
	learnedAdjustments  int64

	// Callbacks
	onRetroactiveCapture func(traceID string, spans []trace.Span, reason string)
	onAnomalyDetected    func(span *trace.Span, anomalyScore float64)

	mu     sync.RWMutex
	stopCh chan struct{}
}

// IntelligentSamplerConfig configures the intelligent sampler
type IntelligentSamplerConfig struct {
	// Base tail sampler config
	TailConfig TailSamplerConfig `json:"tail_config"`

	// Retroactive sampling
	RetroactiveEnabled      bool          `json:"retroactive_enabled"`
	RetroactiveWindowSize   int           `json:"retroactive_window_size"`   // Number of recent traces to track
	RetroactiveWindowTime   time.Duration `json:"retroactive_window_time"`   // Time window for retroactive
	RelatedTraceDepth       int           `json:"related_trace_depth"`       // Depth to search for related traces
	RelatedTraceTime        time.Duration `json:"related_trace_time"`        // Time window for related traces

	// Anomaly awareness
	AnomalyEnabled          bool    `json:"anomaly_enabled"`
	AnomalyThreshold        float64 `json:"anomaly_threshold"`         // Z-score threshold for anomaly
	AnomalySampleRate       float64 `json:"anomaly_sample_rate"`       // Rate for anomalous traces (usually 1.0)
	AnomalyLearningPeriod   time.Duration `json:"anomaly_learning_period"` // Period to learn normal patterns

	// Cost awareness
	CostEnabled             bool    `json:"cost_enabled"`
	CostPerSpan             float64 `json:"cost_per_span"`             // Estimated cost per span (e.g., $0.000001)
	DailyBudget             float64 `json:"daily_budget"`              // Daily budget limit
	BudgetAdjustmentPeriod  time.Duration `json:"budget_adjustment_period"`

	// Learning mode
	LearningEnabled         bool          `json:"learning_enabled"`
	LearningWindowDays      int           `json:"learning_window_days"`
	LearningMinSamples      int           `json:"learning_min_samples"`
	MinSamplesForLearning   int           `json:"min_samples_for_learning"` // Deprecated: use LearningMinSamples
}

// DefaultIntelligentConfig returns sensible defaults
func DefaultIntelligentConfig() IntelligentSamplerConfig {
	return IntelligentSamplerConfig{
		TailConfig: TailSamplerConfig{
			Enabled:            true,
			BufferTimeout:      30 * time.Second,
			MaxBufferedTraces:  10000,
			MaxSpansPerTrace:   1000,
			ErrorSampleRate:    1.0,
			LatencyThresholdMs: 1000,
			LatencySampleRate:  1.0,
			DefaultSampleRate:  0.1,
		},
		RetroactiveEnabled:     true,
		RetroactiveWindowSize:  10000,
		RetroactiveWindowTime:  5 * time.Minute,
		RelatedTraceDepth:      3,
		RelatedTraceTime:       5 * time.Second,
		AnomalyEnabled:         true,
		AnomalyThreshold:       3.0, // 3 standard deviations
		AnomalySampleRate:      1.0,
		AnomalyLearningPeriod:  1 * time.Hour,
		CostEnabled:            true,
		CostPerSpan:            0.000001, // $0.000001 per span
		DailyBudget:            100.0,    // $100/day default
		BudgetAdjustmentPeriod: 1 * time.Minute,
		LearningEnabled:        true,
		LearningWindowDays:     7,
		LearningMinSamples:     1000,
		MinSamplesForLearning:  1000,
	}
}

// TraceDecision records a sampling decision for retroactive lookup
type TraceDecision struct {
	TraceID      string        `json:"trace_id"`
	Decision     Decision      `json:"decision"`
	Reason       string        `json:"reason"`
	Timestamp    time.Time     `json:"timestamp"`
	SpanCount    int           `json:"span_count"`
	HasError     bool          `json:"has_error"`
	MaxLatency   float64       `json:"max_latency"`
	RootService  string        `json:"root_service"`
	ParentTraces []string      `json:"parent_traces,omitempty"`
	ChildTraces  []string      `json:"child_traces,omitempty"`
	AnomalyScore float64       `json:"anomaly_score,omitempty"`
}

// PatternLearner learns normal traffic patterns
type PatternLearner struct {
	// Per-service latency statistics
	serviceLatency   map[string]*LatencyStats
	serviceSpanCount map[string]*SpanCountStats

	// Time-based patterns (hourly)
	hourlyPatterns [24]*HourlyPattern

	// Learning state
	samplesCollected int64
	learningComplete bool
	lastUpdate       time.Time

	mu sync.RWMutex
}

// LatencyStats tracks latency distribution
type LatencyStats struct {
	Count       int64     `json:"count"`
	Sum         float64   `json:"sum"`
	SumSquares  float64   `json:"sum_squares"`
	Min         float64   `json:"min"`
	Max         float64   `json:"max"`
	LastUpdated time.Time `json:"last_updated"`
}

func (l *LatencyStats) Mean() float64 {
	if l.Count == 0 {
		return 0
	}
	return l.Sum / float64(l.Count)
}

func (l *LatencyStats) StdDev() float64 {
	if l.Count < 2 {
		return 0
	}
	mean := l.Mean()
	variance := (l.SumSquares / float64(l.Count)) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

func (l *LatencyStats) ZScore(value float64) float64 {
	stdDev := l.StdDev()
	if stdDev == 0 {
		return 0
	}
	return (value - l.Mean()) / stdDev
}

// SpanCountStats tracks span count distribution
type SpanCountStats struct {
	Count      int64   `json:"count"`
	Sum        int64   `json:"sum"`
	SumSquares int64   `json:"sum_squares"`
}

func (s *SpanCountStats) Mean() float64 {
	if s.Count == 0 {
		return 0
	}
	return float64(s.Sum) / float64(s.Count)
}

func (s *SpanCountStats) StdDev() float64 {
	if s.Count < 2 {
		return 0
	}
	mean := s.Mean()
	variance := (float64(s.SumSquares) / float64(s.Count)) - (mean * mean)
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

// HourlyPattern tracks patterns for a specific hour
type HourlyPattern struct {
	Hour          int     `json:"hour"`
	AvgSpansPerSec float64 `json:"avg_spans_per_sec"`
	AvgLatency    float64 `json:"avg_latency"`
	ErrorRate     float64 `json:"error_rate"`
	SampleCount   int64   `json:"sample_count"`
}

// CostTracker tracks sampling costs
type CostTracker struct {
	costPerSpan     float64
	dailyBudget     float64

	// Current day tracking
	currentDay      time.Time
	dailyCost       float64
	dailySpans      int64

	// Hourly breakdown
	hourlyCosts [24]float64
	hourlySpans [24]int64

	mu sync.RWMutex
}

// BudgetController adjusts sampling based on budget
type BudgetController struct {
	budget             float64
	currentSpendRate   float64 // Spend rate per second
	adjustmentFactor   float64 // Current rate adjustment

	lastAdjustment     time.Time
	adjustmentInterval time.Duration

	mu sync.RWMutex
}

// SamplingLearner learns optimal sampling decisions
type SamplingLearner struct {
	// Service-specific learned rates
	serviceRates map[string]*LearnedRate

	// Operation-specific learned rates
	operationRates map[string]*LearnedRate

	// Pattern-based learning
	patternRates map[string]*LearnedRate

	// Learning window
	windowStart time.Time
	windowEnd   time.Time

	mu sync.RWMutex
}

// LearnedRate represents a learned sampling rate
type LearnedRate struct {
	Key             string    `json:"key"`
	CurrentRate     float64   `json:"current_rate"`
	RecommendedRate float64   `json:"recommended_rate"`
	ErrorsSeen      int64     `json:"errors_seen"`
	TotalSeen       int64     `json:"total_seen"`
	HighLatencyPct  float64   `json:"high_latency_pct"`
	LastUpdated     time.Time `json:"last_updated"`
}

// NewIntelligentSampler creates a new intelligent sampler
func NewIntelligentSampler(config IntelligentSamplerConfig) *IntelligentSampler {
	s := &IntelligentSampler{
		config:          config,
		baseSampler:     NewTailSampler(config.TailConfig),
		decisionIndex:   make(map[string]*TraceDecision),
		normalPatterns:  newPatternLearner(),
		anomalyThreshold: config.AnomalyThreshold,
		costTracker:     newCostTracker(config.CostPerSpan, config.DailyBudget),
		budgetControl:   newBudgetController(config.DailyBudget, config.BudgetAdjustmentPeriod),
		learner:         newSamplingLearner(),
		stopCh:          make(chan struct{}),
	}

	// Initialize retroactive decision ring
	if config.RetroactiveEnabled && config.RetroactiveWindowSize > 0 {
		s.recentDecisions = ring.New(config.RetroactiveWindowSize)
	}

	// Wire up base sampler callbacks
	s.baseSampler.SetOnKeep(s.handleKeptTrace)
	s.baseSampler.SetOnDrop(s.handleDroppedTrace)

	// Start background tasks
	go s.maintenanceLoop()

	return s
}

func newPatternLearner() *PatternLearner {
	p := &PatternLearner{
		serviceLatency:   make(map[string]*LatencyStats),
		serviceSpanCount: make(map[string]*SpanCountStats),
	}
	for i := range p.hourlyPatterns {
		p.hourlyPatterns[i] = &HourlyPattern{Hour: i}
	}
	return p
}

func newCostTracker(costPerSpan, dailyBudget float64) *CostTracker {
	return &CostTracker{
		costPerSpan: costPerSpan,
		dailyBudget: dailyBudget,
		currentDay:  time.Now().Truncate(24 * time.Hour),
	}
}

func newBudgetController(budget float64, adjustInterval time.Duration) *BudgetController {
	return &BudgetController{
		budget:             budget,
		adjustmentFactor:   1.0,
		lastAdjustment:     time.Now(),
		adjustmentInterval: adjustInterval,
	}
}

func newSamplingLearner() *SamplingLearner {
	return &SamplingLearner{
		serviceRates:   make(map[string]*LearnedRate),
		operationRates: make(map[string]*LearnedRate),
		patternRates:   make(map[string]*LearnedRate),
		windowStart:    time.Now(),
	}
}

// SetOnRetroactiveCapture sets the callback for retroactive captures
func (s *IntelligentSampler) SetOnRetroactiveCapture(fn func(traceID string, spans []trace.Span, reason string)) {
	s.onRetroactiveCapture = fn
}

// SetOnAnomalyDetected sets the callback for anomaly detection
func (s *IntelligentSampler) SetOnAnomalyDetected(fn func(span *trace.Span, anomalyScore float64)) {
	s.onAnomalyDetected = fn
}

// ShouldSample processes a span with intelligent sampling
func (s *IntelligentSampler) ShouldSample(span *trace.Span) Decision {
	atomic.AddInt64(&s.totalSpans, 1)

	// Check anomaly first
	if s.config.AnomalyEnabled {
		anomalyScore := s.calculateAnomalyScore(span)
		if anomalyScore > s.anomalyThreshold {
			atomic.AddInt64(&s.anomalySamples, 1)
			if s.onAnomalyDetected != nil {
				s.onAnomalyDetected(span, anomalyScore)
			}
			// Mark span for anomaly sampling
			if span.Attributes == nil {
				span.Attributes = make(map[string]string)
			}
			span.Attributes["_anomaly_score"] = fmt.Sprintf("%.2f", anomalyScore)

			// High anomaly scores get sampled at anomaly rate
			if s.shouldSampleByRate(span.TraceID+"anomaly", s.config.AnomalySampleRate) {
				return DecisionKeep
			}
		}
	}

	// Apply budget adjustment
	if s.config.CostEnabled {
		adjustmentFactor := s.budgetControl.getAdjustmentFactor()
		if adjustmentFactor < 1.0 {
			// Reduce effective sample rate based on budget
			effectiveRate := s.config.TailConfig.DefaultSampleRate * adjustmentFactor
			if !s.shouldSampleByRate(span.TraceID+"budget", effectiveRate) {
				atomic.AddInt64(&s.budgetAdjustments, 1)
				return DecisionDrop
			}
		}
	}

	// Apply learned adjustments
	if s.config.LearningEnabled && s.learner != nil {
		learnedRate := s.learner.getLearnedRate(span.ServiceName, span.Name)
		if learnedRate > 0 && learnedRate != s.config.TailConfig.DefaultSampleRate {
			if !s.shouldSampleByRate(span.TraceID+"learned", learnedRate) {
				atomic.AddInt64(&s.learnedAdjustments, 1)
				return DecisionDrop
			}
		}
	}

	// Delegate to base tail sampler
	return s.baseSampler.ShouldSample(span)
}

// handleKeptTrace processes traces that were kept
func (s *IntelligentSampler) handleKeptTrace(spans []trace.Span) {
	if len(spans) == 0 {
		return
	}

	traceID := spans[0].TraceID

	// Record decision
	decision := &TraceDecision{
		TraceID:   traceID,
		Decision:  DecisionKeep,
		Reason:    "kept",
		Timestamp: time.Now(),
		SpanCount: len(spans),
	}

	// Analyze spans and extract relationships
	parentTraces := make(map[string]bool)
	childTraces := make(map[string]bool)

	for _, span := range spans {
		if span.Status == "ERROR" {
			decision.HasError = true
		}
		if span.DurationMs > decision.MaxLatency {
			decision.MaxLatency = span.DurationMs
		}
		if span.ParentSpanID == "" && decision.RootService == "" {
			decision.RootService = span.ServiceName
		}

		// Calculate anomaly score for this span
		anomalyScore := s.calculateAnomalyScore(&span)
		if anomalyScore > decision.AnomalyScore {
			decision.AnomalyScore = anomalyScore
		}

		// Extract parent/child trace relationships from attributes
		if span.Attributes != nil {
			// Check for parent trace references
			for _, key := range []string{"_parent_trace_id", "parent_trace_id", "uber-trace-parent"} {
				if parentID, ok := span.Attributes[key]; ok && parentID != "" && parentID != traceID {
					parentTraces[parentID] = true
				}
			}
			// Check for linked/child traces
			for _, key := range []string{"_linked_traces", "linked_trace_id", "child_trace_id"} {
				if linkedID, ok := span.Attributes[key]; ok && linkedID != "" && linkedID != traceID {
					childTraces[linkedID] = true
				}
			}
		}
	}

	// Convert maps to slices
	for id := range parentTraces {
		decision.ParentTraces = append(decision.ParentTraces, id)
	}
	for id := range childTraces {
		decision.ChildTraces = append(decision.ChildTraces, id)
	}

	s.recordDecision(decision)

	// Update pattern learner
	if s.config.AnomalyEnabled {
		s.normalPatterns.record(spans)
	}

	// Update cost tracker
	if s.config.CostEnabled {
		s.costTracker.recordSpans(len(spans))
	}

	// Check for retroactive capture triggers
	if s.config.RetroactiveEnabled {
		// Trigger on error
		if decision.HasError {
			s.triggerRetroactiveCapture(traceID, "error_in_trace")
		}

		// Trigger on high latency (> 5 seconds)
		if decision.MaxLatency > 5000 {
			s.triggerRetroactiveCapture(traceID, "high_latency")
		}

		// Trigger on anomaly detection
		if decision.AnomalyScore > s.anomalyThreshold {
			s.triggerRetroactiveCapture(traceID, "anomaly_detected")
		}
	}
}

// handleDroppedTrace processes traces that were dropped
func (s *IntelligentSampler) handleDroppedTrace(traceID string, spanCount int) {
	decision := &TraceDecision{
		TraceID:   traceID,
		Decision:  DecisionDrop,
		Reason:    "dropped",
		Timestamp: time.Now(),
		SpanCount: spanCount,
	}

	s.recordDecision(decision)
}

// recordDecision records a trace decision in the ring buffer
func (s *IntelligentSampler) recordDecision(decision *TraceDecision) {
	if s.recentDecisions == nil {
		return
	}

	s.recentDecisionsMu.Lock()
	defer s.recentDecisionsMu.Unlock()

	// Remove old decision from index if we're overwriting
	if old := s.recentDecisions.Value; old != nil {
		if oldDecision, ok := old.(*TraceDecision); ok {
			delete(s.decisionIndex, oldDecision.TraceID)
		}
	}

	// Add new decision
	s.recentDecisions.Value = decision
	s.recentDecisions = s.recentDecisions.Next()
	s.decisionIndex[decision.TraceID] = decision
}

// triggerRetroactiveCapture attempts to recover related dropped traces
func (s *IntelligentSampler) triggerRetroactiveCapture(triggerTraceID, reason string) {
	s.recentDecisionsMu.RLock()
	triggerDecision, exists := s.decisionIndex[triggerTraceID]
	s.recentDecisionsMu.RUnlock()

	if !exists || triggerDecision == nil {
		return
	}

	// Find related traces that were dropped
	relatedTraces := s.findRelatedDroppedTraces(triggerDecision)

	for _, relatedTraceID := range relatedTraces {
		// Try to recover from buffered traces
		if s.baseSampler != nil {
			s.baseSampler.tracesMu.RLock()
			buffer, found := s.baseSampler.traces[relatedTraceID]
			s.baseSampler.tracesMu.RUnlock()

			if found && buffer != nil {
				atomic.AddInt64(&s.retroactiveCaptures, 1)
				buffer.mu.Lock()
				spans := make([]trace.Span, len(buffer.Spans))
				copy(spans, buffer.Spans)
				buffer.mu.Unlock()

				if s.onRetroactiveCapture != nil {
					s.onRetroactiveCapture(relatedTraceID, spans,
						fmt.Sprintf("retroactive_capture:related_to:%s:%s", triggerTraceID, reason))
				}

				log.Printf("[intelligent-sampler] Retroactively captured trace %s (related to error trace %s)",
					relatedTraceID, triggerTraceID)
			}
		}
	}
}

// findRelatedDroppedTraces finds traces related to the trigger
func (s *IntelligentSampler) findRelatedDroppedTraces(trigger *TraceDecision) []string {
	s.recentDecisionsMu.RLock()
	defer s.recentDecisionsMu.RUnlock()

	var related []string
	cutoff := trigger.Timestamp.Add(-s.config.RetroactiveWindowTime)

	s.recentDecisions.Do(func(v interface{}) {
		if v == nil {
			return
		}
		decision, ok := v.(*TraceDecision)
		if !ok || decision == nil {
			return
		}

		// Skip if not dropped or too old
		if decision.Decision != DecisionDrop || decision.Timestamp.Before(cutoff) {
			return
		}

		// Check if related (same service, similar timing)
		if s.isRelatedTrace(trigger, decision) {
			related = append(related, decision.TraceID)
		}
	})

	// Limit depth
	if len(related) > s.config.RelatedTraceDepth {
		related = related[:s.config.RelatedTraceDepth]
	}

	return related
}

// isRelatedTrace checks if two traces are related
func (s *IntelligentSampler) isRelatedTrace(trigger, candidate *TraceDecision) bool {
	// Same service
	if trigger.RootService == candidate.RootService {
		return true
	}

	// Close in time (within 1 second)
	timeDiff := trigger.Timestamp.Sub(candidate.Timestamp)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff < time.Second {
		return true
	}

	// Parent/child relationship
	for _, parent := range trigger.ParentTraces {
		if parent == candidate.TraceID {
			return true
		}
	}
	for _, child := range trigger.ChildTraces {
		if child == candidate.TraceID {
			return true
		}
	}

	return false
}

// calculateAnomalyScore calculates how anomalous a span is
func (s *IntelligentSampler) calculateAnomalyScore(span *trace.Span) float64 {
	s.normalPatterns.mu.RLock()
	defer s.normalPatterns.mu.RUnlock()

	score := 0.0
	factors := 0

	// Latency anomaly
	if stats, ok := s.normalPatterns.serviceLatency[span.ServiceName]; ok && stats.Count > 100 {
		latencyZ := stats.ZScore(span.DurationMs)
		if math.Abs(latencyZ) > score {
			score = math.Abs(latencyZ)
		}
		factors++
	}

	// If no learned patterns, use default threshold
	if factors == 0 {
		// Use high latency as simple anomaly indicator
		if span.DurationMs > 5000 { // > 5 seconds
			return 3.0
		}
		if span.Status == "ERROR" {
			return 2.0
		}
		return 0
	}

	return score
}

// shouldSampleByRate uses consistent hashing for deterministic sampling
func (s *IntelligentSampler) shouldSampleByRate(key string, rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	h := fnv.New64a()
	h.Write([]byte(key))
	hashValue := h.Sum64()

	normalized := float64(hashValue) / float64(^uint64(0))
	return normalized < rate
}

// Pattern learner methods

func (p *PatternLearner) record(spans []trace.Span) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	hour := now.Hour()
	var totalLatency float64
	var errorCount int

	for _, span := range spans {
		// Update service latency stats
		stats, ok := p.serviceLatency[span.ServiceName]
		if !ok {
			stats = &LatencyStats{Min: span.DurationMs, Max: span.DurationMs}
			p.serviceLatency[span.ServiceName] = stats
		}
		stats.Count++
		stats.Sum += span.DurationMs
		stats.SumSquares += span.DurationMs * span.DurationMs
		if span.DurationMs < stats.Min {
			stats.Min = span.DurationMs
		}
		if span.DurationMs > stats.Max {
			stats.Max = span.DurationMs
		}
		stats.LastUpdated = now

		// Update span count stats
		scStats, ok := p.serviceSpanCount[span.ServiceName]
		if !ok {
			scStats = &SpanCountStats{}
			p.serviceSpanCount[span.ServiceName] = scStats
		}
		scStats.Count++
		scStats.Sum += 1
		scStats.SumSquares += 1

		totalLatency += span.DurationMs
		if span.Status == "ERROR" {
			errorCount++
		}
	}

	if len(spans) > 0 {
		// Update hourly pattern with moving average
		pattern := p.hourlyPatterns[hour]
		pattern.SampleCount++

		// Calculate averages using exponential moving average for smoothing
		alpha := 0.1 // Smoothing factor
		avgLatency := totalLatency / float64(len(spans))
		if pattern.SampleCount == 1 {
			pattern.AvgLatency = avgLatency
		} else {
			pattern.AvgLatency = alpha*avgLatency + (1-alpha)*pattern.AvgLatency
		}

		// Update error rate
		traceErrorRate := float64(errorCount) / float64(len(spans))
		if pattern.SampleCount == 1 {
			pattern.ErrorRate = traceErrorRate
		} else {
			pattern.ErrorRate = alpha*traceErrorRate + (1-alpha)*pattern.ErrorRate
		}
	}

	atomic.AddInt64(&p.samplesCollected, int64(len(spans)))
	p.lastUpdate = now

	// Mark learning complete after sufficient samples
	if !p.learningComplete && p.samplesCollected > 10000 {
		p.learningComplete = true
	}
}

// isLearningComplete returns whether baseline learning is done
func (p *PatternLearner) isLearningComplete() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.learningComplete
}

// getServiceBaseline returns the baseline stats for a service
func (p *PatternLearner) getServiceBaseline(service string) (*LatencyStats, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats, ok := p.serviceLatency[service]
	if !ok || stats.Count < 100 {
		return nil, false
	}
	return stats, true
}

// getHourlyPattern returns the pattern for a specific hour
func (p *PatternLearner) getHourlyPattern(hour int) *HourlyPattern {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if hour < 0 || hour > 23 {
		return nil
	}
	return p.hourlyPatterns[hour]
}

// isAnomalousForService checks if a latency is anomalous for a given service
func (p *PatternLearner) isAnomalousForService(service string, latencyMs float64, threshold float64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats, ok := p.serviceLatency[service]
	if !ok || stats.Count < 100 {
		// Not enough data to determine
		return false
	}

	zScore := stats.ZScore(latencyMs)
	return math.Abs(zScore) > threshold
}

// Cost tracker methods

func (c *CostTracker) recordSpans(count int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	today := now.Truncate(24 * time.Hour)

	// Reset if new day
	if today.After(c.currentDay) {
		c.currentDay = today
		c.dailyCost = 0
		c.dailySpans = 0
		for i := range c.hourlyCosts {
			c.hourlyCosts[i] = 0
			c.hourlySpans[i] = 0
		}
	}

	cost := float64(count) * c.costPerSpan
	c.dailyCost += cost
	c.dailySpans += int64(count)

	hour := now.Hour()
	c.hourlyCosts[hour] += cost
	c.hourlySpans[hour] += int64(count)
}

func (c *CostTracker) getDailyCost() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dailyCost
}

func (c *CostTracker) getBudgetUsedPct() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dailyBudget <= 0 {
		return 0
	}
	return (c.dailyCost / c.dailyBudget) * 100
}

// Budget controller methods

func (b *BudgetController) getAdjustmentFactor() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.adjustmentFactor
}

func (b *BudgetController) updateAdjustment(currentCost, hoursRemaining float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if hoursRemaining <= 0 {
		hoursRemaining = 1
	}

	remainingBudget := b.budget - currentCost
	if remainingBudget <= 0 {
		b.adjustmentFactor = 0.1 // Minimum sampling
		return
	}

	// Calculate required rate to stay within budget
	hourlyBudget := remainingBudget / hoursRemaining
	currentHourlySpend := b.currentSpendRate * 3600 // Convert per-second to hourly

	if currentHourlySpend > 0 {
		b.adjustmentFactor = hourlyBudget / currentHourlySpend
		// Clamp to reasonable range
		if b.adjustmentFactor > 2.0 {
			b.adjustmentFactor = 2.0
		}
		if b.adjustmentFactor < 0.1 {
			b.adjustmentFactor = 0.1
		}
	} else {
		b.adjustmentFactor = 1.0
	}

	b.lastAdjustment = time.Now()
}

// Sampling learner methods

func (l *SamplingLearner) getLearnedRate(service, operation string) float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Check operation-specific rate first
	opKey := fmt.Sprintf("%s:%s", service, operation)
	if rate, ok := l.operationRates[opKey]; ok {
		return rate.RecommendedRate
	}

	// Fall back to service rate
	if rate, ok := l.serviceRates[service]; ok {
		return rate.RecommendedRate
	}

	return 0 // No learned rate
}

func (l *SamplingLearner) recordSample(service, operation string, wasError bool, latencyMs float64, highLatencyThreshold float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Update service rate
	rate, ok := l.serviceRates[service]
	if !ok {
		rate = &LearnedRate{
			Key:         service,
			CurrentRate: 0.1, // Default
		}
		l.serviceRates[service] = rate
	}
	rate.TotalSeen++
	if wasError {
		rate.ErrorsSeen++
	}
	if latencyMs > highLatencyThreshold {
		rate.HighLatencyPct = float64(rate.HighLatencyPct*float64(rate.TotalSeen-1)+1) / float64(rate.TotalSeen)
	}
	rate.LastUpdated = time.Now()

	// Recommend rate based on characteristics
	// Higher error rates and latency -> higher sample rate
	errorRate := float64(rate.ErrorsSeen) / float64(rate.TotalSeen)
	rate.RecommendedRate = 0.1 + (errorRate * 0.5) + (rate.HighLatencyPct * 0.3)
	if rate.RecommendedRate > 1.0 {
		rate.RecommendedRate = 1.0
	}
}

// maintenanceLoop runs periodic maintenance tasks
func (s *IntelligentSampler) maintenanceLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runMaintenance()
		case <-s.stopCh:
			return
		}
	}
}

func (s *IntelligentSampler) runMaintenance() {
	// Update budget controller
	if s.config.CostEnabled {
		now := time.Now()
		hoursRemaining := float64(24-now.Hour()) + float64(60-now.Minute())/60.0
		currentCost := s.costTracker.getDailyCost()
		s.budgetControl.updateAdjustment(currentCost, hoursRemaining)
	}

	// Clean old decisions from index
	s.recentDecisionsMu.Lock()
	cutoff := time.Now().Add(-s.config.RetroactiveWindowTime)
	for traceID, decision := range s.decisionIndex {
		if decision.Timestamp.Before(cutoff) {
			delete(s.decisionIndex, traceID)
		}
	}
	s.recentDecisionsMu.Unlock()
}

// GetStats returns current sampler statistics
func (s *IntelligentSampler) GetStats() IntelligentSamplerStats {
	stats := IntelligentSamplerStats{
		TotalSpans:          atomic.LoadInt64(&s.totalSpans),
		AnomaliesDetected:   atomic.LoadInt64(&s.anomalySamples),
		RetroactiveCaptures: atomic.LoadInt64(&s.retroactiveCaptures),
		AnomalySamples:      atomic.LoadInt64(&s.anomalySamples),
		BudgetAdjustments:   atomic.LoadInt64(&s.budgetAdjustments),
		LearnedAdjustments:  atomic.LoadInt64(&s.learnedAdjustments),
	}

	if s.baseSampler != nil {
		baseStats := s.baseSampler.GetStats()
		stats.BaseSamplerStats = baseStats
		stats.BufferedTraces = s.baseSampler.GetBufferedTraceCount()
		stats.KeptSpans = baseStats.SampledSpans
		stats.DroppedSpans = baseStats.DroppedSpans
		stats.DeferredSpans = baseStats.DeferredSpans
	}

	if s.costTracker != nil {
		stats.DailyCost = s.costTracker.getDailyCost()
		stats.BudgetUsedPct = s.costTracker.getBudgetUsedPct()
	}

	if s.budgetControl != nil {
		stats.AdjustmentFactor = s.budgetControl.getAdjustmentFactor()
	}

	s.recentDecisionsMu.RLock()
	stats.TrackedDecisions = len(s.decisionIndex)
	s.recentDecisionsMu.RUnlock()

	return stats
}

// IntelligentSamplerStats provides statistics for the intelligent sampler
type IntelligentSamplerStats struct {
	TotalSpans          int64        `json:"total_spans"`
	KeptSpans           int64        `json:"kept_spans"`
	DroppedSpans        int64        `json:"dropped_spans"`
	DeferredSpans       int64        `json:"deferred_spans"`
	AnomaliesDetected   int64        `json:"anomalies_detected"`
	RetroactiveCaptures int64        `json:"retroactive_captures"`
	AnomalySamples      int64        `json:"anomaly_samples"`
	BudgetAdjustments   int64        `json:"budget_adjustments"`
	LearnedAdjustments  int64        `json:"learned_adjustments"`
	BufferedTraces      int64        `json:"buffered_traces"`
	TrackedDecisions    int          `json:"tracked_decisions"`
	DailyCost           float64      `json:"daily_cost"`
	BudgetUsedPct       float64      `json:"budget_used_pct"`
	AdjustmentFactor    float64      `json:"adjustment_factor"`
	BaseSamplerStats    SamplerStats `json:"base_sampler_stats"`
}

// UpdateConfig updates the sampler configuration
func (s *IntelligentSampler) UpdateConfig(config IntelligentSamplerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = config
	s.anomalyThreshold = config.AnomalyThreshold

	if s.baseSampler != nil {
		s.baseSampler.UpdateConfig(config.TailConfig)
	}

	if s.costTracker != nil {
		s.costTracker.mu.Lock()
		s.costTracker.costPerSpan = config.CostPerSpan
		s.costTracker.dailyBudget = config.DailyBudget
		s.costTracker.mu.Unlock()
	}

	if s.budgetControl != nil {
		s.budgetControl.mu.Lock()
		s.budgetControl.budget = config.DailyBudget
		s.budgetControl.adjustmentInterval = config.BudgetAdjustmentPeriod
		s.budgetControl.mu.Unlock()
	}
}

// GetConfig returns the current configuration
func (s *IntelligentSampler) GetConfig() IntelligentSamplerConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// GetLearnedPatterns returns the current learned patterns
func (s *IntelligentSampler) GetLearnedPatterns() map[string]interface{} {
	result := make(map[string]interface{})

	if s.normalPatterns != nil {
		s.normalPatterns.mu.RLock()
		serviceStats := make(map[string]map[string]interface{})
		for svc, stats := range s.normalPatterns.serviceLatency {
			serviceStats[svc] = map[string]interface{}{
				"count":   stats.Count,
				"mean":    stats.Mean(),
				"std_dev": stats.StdDev(),
				"min":     stats.Min,
				"max":     stats.Max,
			}
		}
		result["service_latency"] = serviceStats
		result["samples_collected"] = s.normalPatterns.samplesCollected
		s.normalPatterns.mu.RUnlock()
	}

	if s.learner != nil {
		s.learner.mu.RLock()
		learnedRates := make(map[string]float64)
		for svc, rate := range s.learner.serviceRates {
			learnedRates[svc] = rate.RecommendedRate
		}
		result["learned_rates"] = learnedRates
		s.learner.mu.RUnlock()
	}

	return result
}

// ExportConfig exports the configuration as JSON
func (s *IntelligentSampler) ExportConfig() ([]byte, error) {
	return json.Marshal(s.config)
}

// Stop stops the intelligent sampler
func (s *IntelligentSampler) Stop() {
	close(s.stopCh)
	if s.baseSampler != nil {
		s.baseSampler.Stop()
	}
}

// ForceFlush forces the base sampler to flush
func (s *IntelligentSampler) ForceFlush() {
	if s.baseSampler != nil {
		s.baseSampler.ForceFlush()
	}
}

// GetCostBreakdown returns a breakdown of costs
func (s *IntelligentSampler) GetCostBreakdown() CostBreakdown {
	if s.costTracker == nil {
		return CostBreakdown{}
	}

	s.costTracker.mu.RLock()
	defer s.costTracker.mu.RUnlock()

	breakdown := CostBreakdown{
		DailyCost:    s.costTracker.dailyCost,
		DailyBudget:  s.costTracker.dailyBudget,
		DailySpans:   s.costTracker.dailySpans,
		CostPerSpan:  s.costTracker.costPerSpan,
		HourlyCosts:  s.costTracker.hourlyCosts,
		HourlySpans:  s.costTracker.hourlySpans,
	}

	if s.costTracker.dailyBudget > 0 {
		breakdown.BudgetUsedPct = (s.costTracker.dailyCost / s.costTracker.dailyBudget) * 100
		breakdown.BudgetRemaining = s.costTracker.dailyBudget - s.costTracker.dailyCost
	}

	return breakdown
}

// CostBreakdown provides detailed cost information
type CostBreakdown struct {
	DailyCost       float64      `json:"daily_cost"`
	DailyBudget     float64      `json:"daily_budget"`
	DailySpans      int64        `json:"daily_spans"`
	CostPerSpan     float64      `json:"cost_per_span"`
	BudgetUsedPct   float64      `json:"budget_used_pct"`
	BudgetRemaining float64      `json:"budget_remaining"`
	HourlyCosts     [24]float64  `json:"hourly_costs"`
	HourlySpans     [24]int64    `json:"hourly_spans"`
}

// GetRecentDecisions returns recent trace decisions
func (s *IntelligentSampler) GetRecentDecisions(limit int) []*TraceDecision {
	s.recentDecisionsMu.RLock()
	defer s.recentDecisionsMu.RUnlock()

	if s.recentDecisions == nil {
		return nil
	}

	var decisions []*TraceDecision
	s.recentDecisions.Do(func(v interface{}) {
		if v == nil {
			return
		}
		if decision, ok := v.(*TraceDecision); ok && decision != nil {
			decisions = append(decisions, decision)
		}
	})

	// Sort by timestamp descending (most recent first)
	sort.Slice(decisions, func(i, j int) bool {
		return decisions[i].Timestamp.After(decisions[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && len(decisions) > limit {
		decisions = decisions[:limit]
	}

	return decisions
}
