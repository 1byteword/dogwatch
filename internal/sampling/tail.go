package sampling

import (
	"hash/fnv"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"dogwatch/internal/trace"
)

// TailSampler implements tail-based sampling
// Spans are buffered until the trace completes or times out, then a decision is made
type TailSampler struct {
	config TailSamplerConfig

	// Buffered traces waiting for completion
	traces   map[string]*TraceBuffer
	tracesMu sync.RWMutex

	// Priority services set for O(1) lookup
	priorityServices map[string]bool

	// Stats
	totalSpans    int64
	sampledSpans  int64
	droppedSpans  int64
	deferredSpans int64
	bufferedTraces int64

	// Callbacks for when traces are finalized
	onKeep func(spans []trace.Span)
	onDrop func(traceID string, spanCount int)

	// Cleanup
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

// NewTailSampler creates a new tail-based sampler
func NewTailSampler(config TailSamplerConfig) *TailSampler {
	ts := &TailSampler{
		config:           config,
		traces:           make(map[string]*TraceBuffer),
		priorityServices: make(map[string]bool),
		stopCh:           make(chan struct{}),
	}

	// Build priority services set
	for _, svc := range config.PriorityServices {
		ts.priorityServices[svc] = true
	}

	// Start cleanup goroutine
	ts.cleanupTicker = time.NewTicker(time.Second * 5)
	go ts.cleanupLoop()

	return ts
}

// SetOnKeep sets the callback for when a trace is kept
func (ts *TailSampler) SetOnKeep(fn func(spans []trace.Span)) {
	ts.onKeep = fn
}

// SetOnDrop sets the callback for when a trace is dropped
func (ts *TailSampler) SetOnDrop(fn func(traceID string, spanCount int)) {
	ts.onDrop = fn
}

// ShouldSample buffers the span and returns DecisionDefer
// The actual decision is made when the trace completes or times out
func (ts *TailSampler) ShouldSample(span *trace.Span) Decision {
	if !ts.config.Enabled {
		return DecisionKeep
	}

	atomic.AddInt64(&ts.totalSpans, 1)
	atomic.AddInt64(&ts.deferredSpans, 1)

	ts.bufferSpan(span)

	return DecisionDefer
}

// bufferSpan adds a span to its trace buffer
func (ts *TailSampler) bufferSpan(span *trace.Span) {
	ts.tracesMu.Lock()
	defer ts.tracesMu.Unlock()

	buffer, exists := ts.traces[span.TraceID]
	if !exists {
		// Check if we've exceeded max buffered traces
		if len(ts.traces) >= ts.config.MaxBufferedTraces {
			// Evict oldest trace
			ts.evictOldestTrace()
		}

		buffer = &TraceBuffer{
			TraceID:   span.TraceID,
			Spans:     make([]trace.Span, 0, 10),
			FirstSeen: time.Now(),
		}
		ts.traces[span.TraceID] = buffer
		atomic.AddInt64(&ts.bufferedTraces, 1)
	}

	// Check span limit per trace
	if buffer.SpanCount() >= ts.config.MaxSpansPerTrace {
		return
	}

	buffer.AddSpan(*span)
}

// evictOldestTrace removes the oldest trace from the buffer
func (ts *TailSampler) evictOldestTrace() {
	var oldestID string
	var oldestTime time.Time

	for id, buffer := range ts.traces {
		if oldestID == "" || buffer.FirstSeen.Before(oldestTime) {
			oldestID = id
			oldestTime = buffer.FirstSeen
		}
	}

	if oldestID != "" {
		buffer := ts.traces[oldestID]
		delete(ts.traces, oldestID)
		atomic.AddInt64(&ts.bufferedTraces, -1)

		// Make decision for evicted trace
		ts.finalizeTrace(buffer)
	}
}

// cleanupLoop periodically checks for timed-out traces
func (ts *TailSampler) cleanupLoop() {
	for {
		select {
		case <-ts.cleanupTicker.C:
			ts.processTimedOutTraces()
		case <-ts.stopCh:
			return
		}
	}
}

// processTimedOutTraces finalizes traces that have timed out
func (ts *TailSampler) processTimedOutTraces() {
	ts.tracesMu.Lock()

	now := time.Now()
	var toFinalize []*TraceBuffer

	for id, buffer := range ts.traces {
		if now.Sub(buffer.LastUpdated) > ts.config.BufferTimeout {
			toFinalize = append(toFinalize, buffer)
			delete(ts.traces, id)
			atomic.AddInt64(&ts.bufferedTraces, -1)
		}
	}

	ts.tracesMu.Unlock()

	// Finalize outside the lock
	for _, buffer := range toFinalize {
		ts.finalizeTrace(buffer)
	}
}

// finalizeTrace makes the final sampling decision for a complete trace
func (ts *TailSampler) finalizeTrace(buffer *TraceBuffer) {
	buffer.mu.Lock()
	spans := buffer.Spans
	hasError := buffer.HasError
	maxLatency := buffer.MaxLatency
	rootService := buffer.RootService
	traceID := buffer.TraceID
	buffer.mu.Unlock()

	decision, reason := ts.makeDecision(hasError, maxLatency, rootService, traceID)

	if decision == DecisionKeep {
		atomic.AddInt64(&ts.sampledSpans, int64(len(spans)))
		atomic.AddInt64(&ts.deferredSpans, -int64(len(spans)))

		if ts.onKeep != nil {
			ts.onKeep(spans)
		}

		log.Printf("[tail-sampler] Keeping trace %s: %s (%d spans)", traceID, reason, len(spans))
	} else {
		atomic.AddInt64(&ts.droppedSpans, int64(len(spans)))
		atomic.AddInt64(&ts.deferredSpans, -int64(len(spans)))

		if ts.onDrop != nil {
			ts.onDrop(traceID, len(spans))
		}

		log.Printf("[tail-sampler] Dropping trace %s: %s (%d spans)", traceID, reason, len(spans))
	}
}

// makeDecision determines whether to keep or drop a trace
func (ts *TailSampler) makeDecision(hasError bool, maxLatency float64, rootService string, traceID string) (Decision, string) {
	// Rule 1: Keep all traces with errors
	if hasError && ts.config.ErrorSampleRate > 0 {
		if ts.shouldSampleByRate(traceID+"error", ts.config.ErrorSampleRate) {
			return DecisionKeep, "has_error"
		}
	}

	// Rule 2: Keep high-latency traces
	if maxLatency >= ts.config.LatencyThresholdMs && ts.config.LatencySampleRate > 0 {
		if ts.shouldSampleByRate(traceID+"latency", ts.config.LatencySampleRate) {
			return DecisionKeep, "high_latency"
		}
	}

	// Rule 3: Keep traces from priority services
	if ts.priorityServices[rootService] {
		return DecisionKeep, "priority_service"
	}

	// Rule 4: Apply default sampling rate
	if ts.shouldSampleByRate(traceID, ts.config.DefaultSampleRate) {
		return DecisionKeep, "default_sample"
	}

	return DecisionDrop, "not_sampled"
}

// shouldSampleByRate uses consistent hashing for deterministic sampling
func (ts *TailSampler) shouldSampleByRate(key string, rate float64) bool {
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

// GetStats returns current sampler statistics
func (ts *TailSampler) GetStats() SamplerStats {
	total := atomic.LoadInt64(&ts.totalSpans)
	sampled := atomic.LoadInt64(&ts.sampledSpans)
	dropped := atomic.LoadInt64(&ts.droppedSpans)
	deferred := atomic.LoadInt64(&ts.deferredSpans)

	var rate float64
	if total > 0 {
		rate = float64(sampled) / float64(total)
	}

	return SamplerStats{
		TotalSpans:    total,
		SampledSpans:  sampled,
		DroppedSpans:  dropped,
		DeferredSpans: deferred,
		CurrentRate:   rate,
	}
}

// GetBufferedTraceCount returns the number of traces currently buffered
func (ts *TailSampler) GetBufferedTraceCount() int64 {
	return atomic.LoadInt64(&ts.bufferedTraces)
}

// UpdateConfig updates the sampler configuration
func (ts *TailSampler) UpdateConfig(config TailSamplerConfig) {
	ts.tracesMu.Lock()
	ts.config = config

	// Rebuild priority services set
	ts.priorityServices = make(map[string]bool)
	for _, svc := range config.PriorityServices {
		ts.priorityServices[svc] = true
	}
	ts.tracesMu.Unlock()
}

// GetConfig returns the current configuration
func (ts *TailSampler) GetConfig() TailSamplerConfig {
	ts.tracesMu.RLock()
	defer ts.tracesMu.RUnlock()
	return ts.config
}

// ForceFlush immediately finalizes all buffered traces
func (ts *TailSampler) ForceFlush() {
	ts.tracesMu.Lock()

	var toFinalize []*TraceBuffer
	for id, buffer := range ts.traces {
		toFinalize = append(toFinalize, buffer)
		delete(ts.traces, id)
		atomic.AddInt64(&ts.bufferedTraces, -1)
	}

	ts.tracesMu.Unlock()

	for _, buffer := range toFinalize {
		ts.finalizeTrace(buffer)
	}
}

// GetBufferedTraces returns a copy of all buffered trace IDs and their span counts
func (ts *TailSampler) GetBufferedTraces() map[string]int {
	ts.tracesMu.RLock()
	defer ts.tracesMu.RUnlock()

	result := make(map[string]int)
	for id, buffer := range ts.traces {
		result[id] = buffer.SpanCount()
	}
	return result
}

// Stop stops the tail sampler cleanup goroutine
func (ts *TailSampler) Stop() {
	// Flush remaining traces
	ts.ForceFlush()

	close(ts.stopCh)
	ts.cleanupTicker.Stop()
}

// AddPriorityService adds a service to the priority list
func (ts *TailSampler) AddPriorityService(service string) {
	ts.tracesMu.Lock()
	ts.priorityServices[service] = true
	ts.config.PriorityServices = append(ts.config.PriorityServices, service)
	ts.tracesMu.Unlock()
}

// RemovePriorityService removes a service from the priority list
func (ts *TailSampler) RemovePriorityService(service string) {
	ts.tracesMu.Lock()
	delete(ts.priorityServices, service)

	var newServices []string
	for _, svc := range ts.config.PriorityServices {
		if svc != service {
			newServices = append(newServices, svc)
		}
	}
	ts.config.PriorityServices = newServices
	ts.tracesMu.Unlock()
}

// GetPriorityServices returns the list of priority services
func (ts *TailSampler) GetPriorityServices() []string {
	ts.tracesMu.RLock()
	defer ts.tracesMu.RUnlock()
	return append([]string{}, ts.config.PriorityServices...)
}
