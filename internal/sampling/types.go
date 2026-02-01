package sampling

import (
	"sync"
	"time"

	"dogwatch/internal/trace"
)

// Decision represents the sampling decision for a trace
type Decision int

const (
	// DecisionKeep indicates the trace should be kept
	DecisionKeep Decision = iota
	// DecisionDrop indicates the trace should be dropped
	DecisionDrop
	// DecisionDefer indicates the decision should be deferred (for tail sampling)
	DecisionDefer
)

func (d Decision) String() string {
	switch d {
	case DecisionKeep:
		return "keep"
	case DecisionDrop:
		return "drop"
	case DecisionDefer:
		return "defer"
	default:
		return "unknown"
	}
}

// RuleCondition defines what a rule matches against
type RuleCondition struct {
	// Service name to match (exact or pattern with * wildcard)
	Service string `json:"service,omitempty"`

	// Operation name to match (exact or pattern with * wildcard)
	Operation string `json:"operation,omitempty"`

	// MinLatencyMs - match spans with latency >= this value
	MinLatencyMs float64 `json:"min_latency_ms,omitempty"`

	// HasError - match spans with error status
	HasError *bool `json:"has_error,omitempty"`

	// SpanKind - match specific span kinds (CLIENT, SERVER, INTERNAL, etc.)
	SpanKind string `json:"span_kind,omitempty"`

	// Attributes - match spans with specific attributes
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Rule defines a sampling rule
type Rule struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Enabled     bool          `json:"enabled"`
	Priority    int           `json:"priority"` // Higher priority rules evaluated first
	Condition   RuleCondition `json:"condition"`
	Action      Decision      `json:"action"`      // Keep or Drop
	SampleRate  float64       `json:"sample_rate"` // 0.0-1.0, only used if Action != Keep
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Config holds sampler configuration
type Config struct {
	// Enabled controls whether sampling is active
	Enabled bool `json:"enabled"`

	// DefaultSampleRate is the rate for traces that don't match any rules (0.0-1.0)
	DefaultSampleRate float64 `json:"default_sample_rate"`

	// HeadSamplerConfig for head-based sampling
	HeadSamplerConfig HeadSamplerConfig `json:"head_sampler"`

	// TailSamplerConfig for tail-based sampling
	TailSamplerConfig TailSamplerConfig `json:"tail_sampler"`

	// AdaptiveSamplerConfig for adaptive sampling
	AdaptiveSamplerConfig AdaptiveSamplerConfig `json:"adaptive_sampler"`
}

// HeadSamplerConfig configures head-based sampling
type HeadSamplerConfig struct {
	// Enabled controls whether head sampling is active
	Enabled bool `json:"enabled"`

	// Rules are evaluated in priority order
	Rules []Rule `json:"rules"`

	// DecisionTTL is how long to cache sampling decisions
	DecisionTTL time.Duration `json:"decision_ttl"`

	// MaxCachedDecisions limits memory usage for cached decisions
	MaxCachedDecisions int `json:"max_cached_decisions"`
}

// TailSamplerConfig configures tail-based sampling
type TailSamplerConfig struct {
	// Enabled controls whether tail sampling is active
	Enabled bool `json:"enabled"`

	// BufferTimeout is max time to wait for trace completion
	BufferTimeout time.Duration `json:"buffer_timeout"`

	// MaxBufferedTraces limits memory usage
	MaxBufferedTraces int `json:"max_buffered_traces"`

	// MaxSpansPerTrace limits spans stored per trace
	MaxSpansPerTrace int `json:"max_spans_per_trace"`

	// ErrorSampleRate is the rate for traces containing errors (usually 1.0)
	ErrorSampleRate float64 `json:"error_sample_rate"`

	// LatencyThresholdMs - traces exceeding this are kept at LatencySampleRate
	LatencyThresholdMs float64 `json:"latency_threshold_ms"`

	// LatencySampleRate is the rate for high-latency traces (usually 1.0)
	LatencySampleRate float64 `json:"latency_sample_rate"`

	// PriorityServices always have their root spans kept
	PriorityServices []string `json:"priority_services"`

	// DefaultSampleRate for traces that don't match special criteria
	DefaultSampleRate float64 `json:"default_sample_rate"`
}

// AdaptiveSamplerConfig configures adaptive sampling
type AdaptiveSamplerConfig struct {
	// Enabled controls whether adaptive sampling is active
	Enabled bool `json:"enabled"`

	// TargetTracesPerSecond is the desired throughput
	TargetTracesPerSecond float64 `json:"target_traces_per_second"`

	// MinSampleRate is the floor for sampling rate
	MinSampleRate float64 `json:"min_sample_rate"`

	// MaxSampleRate is the ceiling for sampling rate (usually 1.0)
	MaxSampleRate float64 `json:"max_sample_rate"`

	// AdjustmentInterval is how often to recalculate rates
	AdjustmentInterval time.Duration `json:"adjustment_interval"`

	// SmoothingFactor controls rate change smoothness (0.0-1.0)
	// Lower values = smoother changes but slower adaptation
	SmoothingFactor float64 `json:"smoothing_factor"`

	// PerServiceRates enables tracking rates per service
	PerServiceRates bool `json:"per_service_rates"`
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		DefaultSampleRate: 0.1, // 10% default

		HeadSamplerConfig: HeadSamplerConfig{
			Enabled:            true,
			DecisionTTL:        5 * time.Minute,
			MaxCachedDecisions: 100000,
			Rules: []Rule{
				{
					ID:       "default-errors",
					Name:     "Keep all errors",
					Enabled:  true,
					Priority: 100,
					Condition: RuleCondition{
						HasError: boolPtr(true),
					},
					Action:     DecisionKeep,
					SampleRate: 1.0,
				},
			},
		},

		TailSamplerConfig: TailSamplerConfig{
			Enabled:            false, // Off by default, mutually exclusive with head
			BufferTimeout:      30 * time.Second,
			MaxBufferedTraces:  10000,
			MaxSpansPerTrace:   1000,
			ErrorSampleRate:    1.0,
			LatencyThresholdMs: 1000, // 1 second
			LatencySampleRate:  1.0,
			DefaultSampleRate:  0.1,
		},

		AdaptiveSamplerConfig: AdaptiveSamplerConfig{
			Enabled:               true,
			TargetTracesPerSecond: 100,
			MinSampleRate:         0.01, // 1% minimum
			MaxSampleRate:         1.0,
			AdjustmentInterval:    10 * time.Second,
			SmoothingFactor:       0.3,
			PerServiceRates:       true,
		},
	}
}

// Sampler is the interface for all samplers
type Sampler interface {
	// ShouldSample returns the sampling decision for a span
	ShouldSample(span *trace.Span) Decision

	// GetStats returns current sampler statistics
	GetStats() SamplerStats
}

// SamplerStats provides sampling statistics
type SamplerStats struct {
	TotalSpans    int64   `json:"total_spans"`
	SampledSpans  int64   `json:"sampled_spans"`
	DroppedSpans  int64   `json:"dropped_spans"`
	DeferredSpans int64   `json:"deferred_spans"`
	CurrentRate   float64 `json:"current_rate"`
}

// TraceBuffer holds spans for a single trace during tail sampling
type TraceBuffer struct {
	TraceID     string
	Spans       []trace.Span
	FirstSeen   time.Time
	LastUpdated time.Time
	HasError    bool
	MaxLatency  float64
	RootService string
	mu          sync.Mutex
}

// AddSpan adds a span to the trace buffer
func (tb *TraceBuffer) AddSpan(span trace.Span) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.Spans = append(tb.Spans, span)
	tb.LastUpdated = time.Now()

	// Track error status
	if span.Status == "ERROR" {
		tb.HasError = true
	}

	// Track max latency
	if span.DurationMs > tb.MaxLatency {
		tb.MaxLatency = span.DurationMs
	}

	// Track root service
	if span.ParentSpanID == "" && tb.RootService == "" {
		tb.RootService = span.ServiceName
	}
}

// SpanCount returns the number of spans in the buffer
func (tb *TraceBuffer) SpanCount() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return len(tb.Spans)
}

// SamplingResult holds the result of a sampling decision along with metadata
type SamplingResult struct {
	Decision   Decision          `json:"decision"`
	Reason     string            `json:"reason"`
	RuleID     string            `json:"rule_id,omitempty"`
	SampleRate float64           `json:"sample_rate"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}
