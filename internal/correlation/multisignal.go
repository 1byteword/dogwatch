package correlation

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"
)

// MultiSignalCorrelator provides advanced cross-signal correlation
type MultiSignalCorrelator struct {
	traceStore   *trace.Store
	logStore     *logs.Store
	metricsStore *custommetrics.Store

	// Exemplar storage: links metric points to trace IDs
	exemplars   map[string][]Exemplar // metric_key -> exemplars
	exemplarMu  sync.RWMutex
	maxExemplars int

	// Configuration
	config MultiSignalConfig
}

// MultiSignalConfig configures the correlator
type MultiSignalConfig struct {
	// FuzzyMatchWindow is the time window for fuzzy matching (default 5s)
	FuzzyMatchWindow time.Duration
	// MaxExemplarsPerMetric limits stored exemplars per metric series
	MaxExemplarsPerMetric int
	// TimelineMaxEvents limits events in cross-signal timeline
	TimelineMaxEvents int
}

// DefaultMultiSignalConfig returns sensible defaults
func DefaultMultiSignalConfig() MultiSignalConfig {
	return MultiSignalConfig{
		FuzzyMatchWindow:      5 * time.Second,
		MaxExemplarsPerMetric: 100,
		TimelineMaxEvents:     1000,
	}
}

// Exemplar links a metric point to a trace
type Exemplar struct {
	TraceID   string            `json:"trace_id"`
	SpanID    string            `json:"span_id,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// MetricToTraceResult contains traces related to a metric spike
type MetricToTraceResult struct {
	MetricName   string        `json:"metric_name"`
	MetricLabels map[string]string `json:"metric_labels,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	Value        float64       `json:"value"`
	Exemplars    []Exemplar    `json:"exemplars,omitempty"`
	FuzzyMatches []FuzzyMatch  `json:"fuzzy_matches,omitempty"`
}

// FuzzyMatch represents a correlation found via fuzzy matching
type FuzzyMatch struct {
	SignalType  string      `json:"signal_type"` // trace, log, metric
	ID          string      `json:"id"`
	Timestamp   time.Time   `json:"timestamp"`
	Service     string      `json:"service"`
	Operation   string      `json:"operation,omitempty"`
	Confidence  float64     `json:"confidence"` // 0-1
	MatchReason string      `json:"match_reason"`
	Data        interface{} `json:"data,omitempty"`
}

// LogToTraceResult contains traces related to a log entry
type LogToTraceResult struct {
	LogEntry      *logs.LogEntry `json:"log_entry"`
	ExactTrace    *trace.Trace   `json:"exact_trace,omitempty"`    // Found via trace_id
	FuzzyMatches  []FuzzyMatch   `json:"fuzzy_matches,omitempty"`  // Found via fuzzy matching
	RelatedTraces []trace.Trace  `json:"related_traces,omitempty"` // Same service + time window
}

// CrossSignalTimeline provides a unified view across all signals
type CrossSignalTimeline struct {
	Service   string              `json:"service,omitempty"`
	StartTime time.Time           `json:"start_time"`
	EndTime   time.Time           `json:"end_time"`
	Events    []TimelineEvent     `json:"events"`
	Summary   *TimelineSummaryExt `json:"summary"`
}

// TimelineEvent represents any event in the unified timeline
type TimelineEvent struct {
	Type       string            `json:"type"` // trace, log, metric_anomaly, deploy
	ID         string            `json:"id"`
	Timestamp  time.Time         `json:"timestamp"`
	EndTime    time.Time         `json:"end_time,omitempty"`
	Service    string            `json:"service"`
	Operation  string            `json:"operation,omitempty"`
	Message    string            `json:"message,omitempty"`
	Level      string            `json:"level,omitempty"` // For logs: error, warn, info
	Value      float64           `json:"value,omitempty"` // For metrics
	Status     string            `json:"status,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
	SpanID     string            `json:"span_id,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Severity   int               `json:"severity"` // 0=info, 1=warning, 2=error, 3=critical
}

// TimelineSummaryExt provides extended summary stats
type TimelineSummaryExt struct {
	TotalEvents     int     `json:"total_events"`
	TraceCount      int     `json:"trace_count"`
	SpanCount       int     `json:"span_count"`
	LogCount        int     `json:"log_count"`
	ErrorLogCount   int     `json:"error_log_count"`
	MetricCount     int     `json:"metric_count"`
	AnomalyCount    int     `json:"anomaly_count"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
	P99LatencyMs    float64 `json:"p99_latency_ms"`
	ErrorRate       float64 `json:"error_rate"`
	CorrelationHits int     `json:"correlation_hits"` // Events linked by trace ID
}

// NewMultiSignalCorrelator creates a new multi-signal correlator
func NewMultiSignalCorrelator(config MultiSignalConfig) *MultiSignalCorrelator {
	if config.FuzzyMatchWindow == 0 {
		config.FuzzyMatchWindow = DefaultMultiSignalConfig().FuzzyMatchWindow
	}
	if config.MaxExemplarsPerMetric == 0 {
		config.MaxExemplarsPerMetric = DefaultMultiSignalConfig().MaxExemplarsPerMetric
	}
	if config.TimelineMaxEvents == 0 {
		config.TimelineMaxEvents = DefaultMultiSignalConfig().TimelineMaxEvents
	}

	return &MultiSignalCorrelator{
		exemplars:    make(map[string][]Exemplar),
		maxExemplars: config.MaxExemplarsPerMetric,
		config:       config,
	}
}

// SetTraceStore sets the trace store
func (c *MultiSignalCorrelator) SetTraceStore(s *trace.Store) {
	c.traceStore = s
}

// SetLogStore sets the log store
func (c *MultiSignalCorrelator) SetLogStore(s *logs.Store) {
	c.logStore = s
}

// SetMetricsStore sets the custom metrics store
func (c *MultiSignalCorrelator) SetMetricsStore(s *custommetrics.Store) {
	c.metricsStore = s
}

// SetStores is a convenience method to set all stores at once
func (c *MultiSignalCorrelator) SetStores(traceStore *trace.Store, logStore *logs.Store, metricsStore *custommetrics.Store) {
	c.traceStore = traceStore
	c.logStore = logStore
	c.metricsStore = metricsStore
}

// RecordExemplar stores a metric-to-trace exemplar
func (c *MultiSignalCorrelator) RecordExemplar(metricKey string, exemplar Exemplar) {
	c.exemplarMu.Lock()
	defer c.exemplarMu.Unlock()

	exemplars := c.exemplars[metricKey]
	exemplars = append(exemplars, exemplar)

	// Limit size - keep most recent
	if len(exemplars) > c.maxExemplars {
		exemplars = exemplars[len(exemplars)-c.maxExemplars:]
	}

	c.exemplars[metricKey] = exemplars
}

// GetExemplars returns stored exemplars for a metric key
func (c *MultiSignalCorrelator) GetExemplars(metricKey string) []Exemplar {
	c.exemplarMu.RLock()
	defer c.exemplarMu.RUnlock()

	exemplars := c.exemplars[metricKey]
	result := make([]Exemplar, len(exemplars))
	copy(result, exemplars)
	return result
}

// GetExemplarsInRange returns exemplars within a time range
func (c *MultiSignalCorrelator) GetExemplarsInRange(metricKey string, start, end time.Time) []Exemplar {
	c.exemplarMu.RLock()
	defer c.exemplarMu.RUnlock()

	var result []Exemplar
	for _, ex := range c.exemplars[metricKey] {
		if (ex.Timestamp.Equal(start) || ex.Timestamp.After(start)) &&
			(ex.Timestamp.Equal(end) || ex.Timestamp.Before(end)) {
			result = append(result, ex)
		}
	}
	return result
}

// LogToTrace finds traces related to a log entry using both exact and fuzzy matching
func (c *MultiSignalCorrelator) LogToTrace(ctx context.Context, logEntry *logs.LogEntry) (*LogToTraceResult, error) {
	result := &LogToTraceResult{
		LogEntry: logEntry,
	}

	// 1. Exact match via trace_id
	if logEntry.TraceID != "" && c.traceStore != nil {
		detail, err := c.traceStore.GetTrace(logEntry.TraceID)
		if err == nil && detail != nil {
			result.ExactTrace = &detail.Trace
		}
	}

	// 2. Fuzzy matching: find traces in the same time window and service
	if c.traceStore != nil && logEntry.Service != "" {
		window := c.config.FuzzyMatchWindow
		startTime := logEntry.Timestamp.Add(-window)
		endTime := logEntry.Timestamp.Add(window)

		spans, err := c.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
			Service:   logEntry.Service,
			TimeStart: startTime,
			TimeEnd:   endTime,
			Limit:     50,
		})
		if err == nil {
			seenTraces := make(map[string]bool)
			for _, span := range spans {
				if seenTraces[span.TraceID] {
					continue
				}
				seenTraces[span.TraceID] = true

				// Calculate confidence based on time proximity
				timeDiff := absDuration(span.StartTime.Sub(logEntry.Timestamp))
				confidence := 1.0 - (float64(timeDiff) / float64(window))
				if confidence < 0 {
					confidence = 0
				}

				// Boost confidence if log level indicates error and span has error
				if (logEntry.Level == logs.LevelError || logEntry.Level == logs.LevelFatal) &&
					span.Status == "ERROR" {
					confidence = math.Min(1.0, confidence+0.2)
				}

				// Skip if exact match was already found
				if result.ExactTrace != nil && span.TraceID == result.ExactTrace.TraceID {
					continue
				}

				result.FuzzyMatches = append(result.FuzzyMatches, FuzzyMatch{
					SignalType:  "trace",
					ID:          span.TraceID,
					Timestamp:   span.StartTime,
					Service:     span.ServiceName,
					Operation:   span.Name,
					Confidence:  confidence,
					MatchReason: fuzzyMatchReason(logEntry.Timestamp, span.StartTime, logEntry.Service, span.ServiceName),
				})
			}

			// Sort by confidence
			sort.Slice(result.FuzzyMatches, func(i, j int) bool {
				return result.FuzzyMatches[i].Confidence > result.FuzzyMatches[j].Confidence
			})

			// Limit results
			if len(result.FuzzyMatches) > 10 {
				result.FuzzyMatches = result.FuzzyMatches[:10]
			}
		}
	}

	return result, nil
}

// MetricToTrace finds traces related to a metric spike or anomaly
func (c *MultiSignalCorrelator) MetricToTrace(ctx context.Context, metricName string, labels map[string]string, timestamp time.Time, value float64) (*MetricToTraceResult, error) {
	result := &MetricToTraceResult{
		MetricName:   metricName,
		MetricLabels: labels,
		Timestamp:    timestamp,
		Value:        value,
	}

	// 1. Check exemplars for direct links
	metricKey := buildMetricKey(metricName, labels)
	window := c.config.FuzzyMatchWindow
	exemplars := c.GetExemplarsInRange(metricKey, timestamp.Add(-window), timestamp.Add(window))
	result.Exemplars = exemplars

	// 2. Fuzzy matching based on service + time
	if c.traceStore != nil {
		service := ""
		if labels != nil {
			service = labels["service"]
			if service == "" {
				service = labels["service_name"]
			}
		}

		spans, err := c.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
			Service:   service,
			TimeStart: timestamp.Add(-window),
			TimeEnd:   timestamp.Add(window),
			Limit:     50,
		})
		if err == nil {
			seenTraces := make(map[string]bool)
			for _, span := range spans {
				if seenTraces[span.TraceID] {
					continue
				}
				seenTraces[span.TraceID] = true

				// Calculate confidence
				timeDiff := absDuration(span.StartTime.Sub(timestamp))
				confidence := 1.0 - (float64(timeDiff) / float64(window))
				if confidence < 0 {
					confidence = 0
				}

				// Boost for latency metrics if span duration is high
				if containsAny(metricName, []string{"latency", "duration", "response_time"}) &&
					span.DurationMs > 1000 {
					confidence = math.Min(1.0, confidence+0.15)
				}

				// Boost for error metrics if span has error
				if containsAny(metricName, []string{"error", "failure", "exception"}) &&
					span.Status == "ERROR" {
					confidence = math.Min(1.0, confidence+0.2)
				}

				result.FuzzyMatches = append(result.FuzzyMatches, FuzzyMatch{
					SignalType:  "trace",
					ID:          span.TraceID,
					Timestamp:   span.StartTime,
					Service:     span.ServiceName,
					Operation:   span.Name,
					Confidence:  confidence,
					MatchReason: "time_proximity",
				})
			}

			// Sort by confidence
			sort.Slice(result.FuzzyMatches, func(i, j int) bool {
				return result.FuzzyMatches[i].Confidence > result.FuzzyMatches[j].Confidence
			})

			if len(result.FuzzyMatches) > 10 {
				result.FuzzyMatches = result.FuzzyMatches[:10]
			}
		}
	}

	return result, nil
}

// GetCrossSignalTimeline builds a unified timeline of all signals
func (c *MultiSignalCorrelator) GetCrossSignalTimeline(ctx context.Context, service string, start, end time.Time) (*CrossSignalTimeline, error) {
	timeline := &CrossSignalTimeline{
		Service:   service,
		StartTime: start,
		EndTime:   end,
		Events:    make([]TimelineEvent, 0),
		Summary: &TimelineSummaryExt{},
	}

	var allEvents []TimelineEvent
	var totalLatency float64
	var latencies []float64
	var errorCount, totalTraces int

	// 1. Get traces
	if c.traceStore != nil {
		spans, err := c.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
			Service:   service,
			TimeStart: start,
			TimeEnd:   end,
			Limit:     c.config.TimelineMaxEvents,
		})
		if err == nil {
			seenTraces := make(map[string]bool)
			for _, span := range spans {
				timeline.Summary.SpanCount++
				latencies = append(latencies, span.DurationMs)
				totalLatency += span.DurationMs

				if span.Status == "ERROR" {
					errorCount++
				}

				// Add root spans or distinct traces
				if span.ParentSpanID == "" || !seenTraces[span.TraceID] {
					if !seenTraces[span.TraceID] {
						totalTraces++
						timeline.Summary.TraceCount++
						seenTraces[span.TraceID] = true
					}

					severity := 0
					if span.Status == "ERROR" {
						severity = 2
					}

					allEvents = append(allEvents, TimelineEvent{
						Type:       "trace",
						ID:         span.TraceID,
						Timestamp:  span.StartTime,
						EndTime:    span.EndTime,
						Service:    span.ServiceName,
						Operation:  span.Name,
						Status:     span.Status,
						TraceID:    span.TraceID,
						SpanID:     span.SpanID,
						Severity:   severity,
						Value:      span.DurationMs,
						Attributes: span.Attributes,
					})
				}
			}
		}
	}

	// 2. Get logs
	if c.logStore != nil {
		result, err := c.logStore.Search(logs.SearchQuery{
			Service:   service,
			StartTime: start,
			EndTime:   end,
			Limit:     c.config.TimelineMaxEvents,
		})
		if err == nil {
			for _, entry := range result.Entries {
				timeline.Summary.LogCount++

				severity := 0
				level := "info"
				switch entry.Level {
				case logs.LevelWarn:
					severity = 1
					level = "warn"
				case logs.LevelError:
					severity = 2
					level = "error"
					timeline.Summary.ErrorLogCount++
				case logs.LevelFatal:
					severity = 3
					level = "fatal"
					timeline.Summary.ErrorLogCount++
				}

				// Track correlation hits
				if entry.TraceID != "" {
					timeline.Summary.CorrelationHits++
				}

				allEvents = append(allEvents, TimelineEvent{
					Type:       "log",
					ID:         entry.ID,
					Timestamp:  entry.Timestamp,
					Service:    entry.Service,
					Message:    truncateString(entry.Message, 200),
					Level:      level,
					TraceID:    entry.TraceID,
					SpanID:     entry.SpanID,
					Severity:   severity,
					Attributes: entry.Attrs,
				})
			}
		}
	}

	// Sort all events by timestamp
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp.Before(allEvents[j].Timestamp)
	})

	// Limit total events
	if len(allEvents) > c.config.TimelineMaxEvents {
		allEvents = allEvents[:c.config.TimelineMaxEvents]
	}

	timeline.Events = allEvents
	timeline.Summary.TotalEvents = len(allEvents)

	// Calculate summary stats
	if len(latencies) > 0 {
		timeline.Summary.AvgLatencyMs = totalLatency / float64(len(latencies))

		// P99
		sort.Float64s(latencies)
		p99Idx := int(float64(len(latencies)) * 0.99)
		if p99Idx >= len(latencies) {
			p99Idx = len(latencies) - 1
		}
		timeline.Summary.P99LatencyMs = latencies[p99Idx]
	}

	if totalTraces > 0 {
		timeline.Summary.ErrorRate = float64(errorCount) / float64(totalTraces)
	}

	return timeline, nil
}

// FindCorrelatedSignals finds all signals correlated to a given trace ID
func (c *MultiSignalCorrelator) FindCorrelatedSignals(ctx context.Context, traceID string) (*CorrelatedSignals, error) {
	result := &CorrelatedSignals{
		TraceID: traceID,
	}

	// Get trace detail
	if c.traceStore != nil {
		detail, err := c.traceStore.GetTrace(traceID)
		if err == nil && detail != nil {
			result.Trace = detail
		}
	}

	// Get logs with this trace ID
	if c.logStore != nil {
		logEntries, err := c.logStore.GetByTraceID(traceID)
		if err == nil {
			result.Logs = logEntries
		}
	}

	// Get exemplars referencing this trace
	c.exemplarMu.RLock()
	for metricKey, exemplars := range c.exemplars {
		for _, ex := range exemplars {
			if ex.TraceID == traceID {
				result.Exemplars = append(result.Exemplars, MetricExemplar{
					MetricKey: metricKey,
					Exemplar:  ex,
				})
			}
		}
	}
	c.exemplarMu.RUnlock()

	return result, nil
}

// CorrelatedSignals contains all signals linked to a trace
type CorrelatedSignals struct {
	TraceID   string            `json:"trace_id"`
	Trace     *trace.TraceDetail `json:"trace,omitempty"`
	Logs      []logs.LogEntry    `json:"logs,omitempty"`
	Exemplars []MetricExemplar   `json:"metric_exemplars,omitempty"`
}

// MetricExemplar links a metric to an exemplar
type MetricExemplar struct {
	MetricKey string   `json:"metric_key"`
	Exemplar  Exemplar `json:"exemplar"`
}

// CorrelateByAttributes finds related signals by matching attributes
func (c *MultiSignalCorrelator) CorrelateByAttributes(ctx context.Context, attrs map[string]string, start, end time.Time) (*AttributeCorrelation, error) {
	result := &AttributeCorrelation{
		Attributes: attrs,
		StartTime:  start,
		EndTime:    end,
	}

	// Find traces with matching attributes
	if c.traceStore != nil {
		spans, err := c.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
			TimeStart: start,
			TimeEnd:   end,
			Limit:     100,
		})
		if err == nil {
			for _, span := range spans {
				if matchesAttributes(span.Attributes, attrs) {
					result.Traces = append(result.Traces, span)
				}
			}
		}
	}

	// Find logs with matching attributes
	if c.logStore != nil {
		searchResult, err := c.logStore.Search(logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Limit:     100,
		})
		if err == nil {
			for _, entry := range searchResult.Entries {
				if matchesAttributes(entry.Attrs, attrs) {
					result.Logs = append(result.Logs, entry.LogEntry)
				}
			}
		}
	}

	return result, nil
}

// AttributeCorrelation contains signals matching specific attributes
type AttributeCorrelation struct {
	Attributes map[string]string `json:"attributes"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Traces     []trace.Span      `json:"traces,omitempty"`
	Logs       []logs.LogEntry   `json:"logs,omitempty"`
}

// Helper functions

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func buildMetricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	key := name
	for _, k := range keys {
		key += "|" + k + "=" + labels[k]
	}
	return key
}

func fuzzyMatchReason(logTime, traceTime time.Time, logService, traceService string) string {
	reasons := []string{}

	timeDiff := absDuration(logTime.Sub(traceTime))
	if timeDiff < time.Second {
		reasons = append(reasons, "time_exact")
	} else if timeDiff < 5*time.Second {
		reasons = append(reasons, "time_close")
	} else {
		reasons = append(reasons, "time_window")
	}

	if logService != "" && logService == traceService {
		reasons = append(reasons, "service_match")
	}

	if len(reasons) == 0 {
		return "time_proximity"
	}

	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += "+" + reasons[i]
	}
	return result
}

func containsAny(s string, substrs []string) bool {
	sLower := toLowerSimple(s)
	for _, substr := range substrs {
		if containsSimple(sLower, toLowerSimple(substr)) {
			return true
		}
	}
	return false
}

func toLowerSimple(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func containsSimple(s, substr string) bool {
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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func matchesAttributes(attrs, query map[string]string) bool {
	if len(query) == 0 {
		return true
	}
	for k, v := range query {
		if attrs[k] != v {
			return false
		}
	}
	return true
}
