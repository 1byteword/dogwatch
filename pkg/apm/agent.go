package apm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Agent is the main APM agent
type Agent struct {
	config    *Config
	transport *Transport

	// Span management
	spans      []*Span
	spansMu    sync.Mutex
	spanCount  int64
	sampleSeed uint64

	// Metrics
	metrics   *MetricsCollector
	metricsMu sync.Mutex

	// Runtime
	startTime time.Time
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// Global agent instance
var (
	globalAgent *Agent
	globalMu    sync.RWMutex
)

// Start initializes the global APM agent
func Start(config *Config) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalAgent != nil {
		return fmt.Errorf("agent already started")
	}

	agent, err := NewAgent(config)
	if err != nil {
		return err
	}

	globalAgent = agent
	return nil
}

// Stop shuts down the global agent
func Stop() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalAgent != nil {
		globalAgent.Close()
		globalAgent = nil
	}
}

// GetAgent returns the global agent instance
func GetAgent() *Agent {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalAgent
}

// NewAgent creates a new APM agent
func NewAgent(config *Config) (*Agent, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.ServiceName == "" {
		return nil, fmt.Errorf("service name is required")
	}
	if config.Disabled {
		if config.Debug {
			log.Println("[apm] Agent disabled")
		}
		return &Agent{config: config}, nil
	}

	transport, err := NewTransport(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	agent := &Agent{
		config:    config,
		transport: transport,
		spans:     make([]*Span, 0, 1000),
		startTime: time.Now(),
		stopCh:    make(chan struct{}),
	}

	if config.EnableMetrics {
		agent.metrics = NewMetricsCollector(config.ServiceName)
	}

	// Start background flush loop
	agent.wg.Add(1)
	go agent.flushLoop()

	// Start metrics collection if enabled
	if config.EnableMetrics {
		agent.wg.Add(1)
		go agent.collectMetricsLoop()
	}

	if config.Debug {
		log.Printf("[apm] Agent started for service %s (endpoint: %s)", config.ServiceName, config.AgentEndpoint)
	}

	return agent, nil
}

// Close shuts down the agent
func (a *Agent) Close() {
	if a.config.Disabled {
		return
	}

	close(a.stopCh)
	a.wg.Wait()

	// Final flush
	a.flush()

	if a.config.Debug {
		log.Printf("[apm] Agent stopped (sent %d spans)", atomic.LoadInt64(&a.spanCount))
	}
}

// StartSpan creates a new span
func (a *Agent) StartSpan(operationName string, opts ...SpanOption) *Span {
	if a.config.Disabled {
		return &Span{agent: a}
	}

	span := &Span{
		agent:         a,
		TraceID:       generateTraceID(),
		SpanID:        generateSpanID(),
		OperationName: operationName,
		Service:       a.config.ServiceName,
		Resource:      operationName,
		StartTime:     time.Now(),
		Tags:          make(map[string]string),
		Metrics:       make(map[string]float64),
	}

	// Apply global tags
	for k, v := range a.config.Tags {
		span.Tags[k] = v
	}

	// Apply environment
	if a.config.Environment != "" {
		span.Tags["env"] = a.config.Environment
	}
	if a.config.ServiceVersion != "" {
		span.Tags["version"] = a.config.ServiceVersion
	}

	// Apply options
	for _, opt := range opts {
		opt(span)
	}

	return span
}

// StartSpanFromContext creates a child span from context
func (a *Agent) StartSpanFromContext(ctx context.Context, operationName string, opts ...SpanOption) (*Span, context.Context) {
	var parentSpan *Span
	if parent := SpanFromContext(ctx); parent != nil {
		parentSpan = parent
	}

	span := a.StartSpan(operationName, opts...)

	if parentSpan != nil {
		span.TraceID = parentSpan.TraceID
		span.ParentID = parentSpan.SpanID
	}

	return span, ContextWithSpan(ctx, span)
}

// recordSpan adds a span to the buffer
func (a *Agent) recordSpan(span *Span) {
	if a.config.Disabled || span == nil {
		return
	}

	// Sampling
	if a.config.SampleRate < 1.0 {
		if !a.shouldSample(span.TraceID) {
			return
		}
	}

	a.spansMu.Lock()
	a.spans = append(a.spans, span)
	a.spansMu.Unlock()

	atomic.AddInt64(&a.spanCount, 1)
}

// shouldSample determines if a trace should be sampled
func (a *Agent) shouldSample(traceID string) bool {
	// Simple deterministic sampling based on trace ID
	if len(traceID) < 4 {
		return true
	}
	// Use last 4 chars of trace ID as sampling key
	b, _ := hex.DecodeString(traceID[len(traceID)-4:])
	if len(b) < 2 {
		return true
	}
	val := float64(uint16(b[0])<<8|uint16(b[1])) / 65535.0
	return val < a.config.SampleRate
}

// flushLoop periodically flushes spans to the server
func (a *Agent) flushLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.flush()
		}
	}
}

// flush sends buffered spans to the server
func (a *Agent) flush() {
	a.spansMu.Lock()
	if len(a.spans) == 0 {
		a.spansMu.Unlock()
		return
	}
	spans := a.spans
	a.spans = make([]*Span, 0, 1000)
	a.spansMu.Unlock()

	if err := a.transport.SendSpans(spans); err != nil {
		if a.config.Debug {
			log.Printf("[apm] Failed to send spans: %v", err)
		}
	} else if a.config.Debug {
		log.Printf("[apm] Flushed %d spans", len(spans))
	}
}

// collectMetricsLoop collects runtime metrics
func (a *Agent) collectMetricsLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.collectRuntimeMetrics()
		}
	}
}

// collectRuntimeMetrics gathers Go runtime metrics
func (a *Agent) collectRuntimeMetrics() {
	if a.metrics == nil {
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	a.metrics.Gauge("runtime.num_goroutines", float64(runtime.NumGoroutine()))
	a.metrics.Gauge("runtime.heap_alloc", float64(mem.HeapAlloc))
	a.metrics.Gauge("runtime.heap_sys", float64(mem.HeapSys))
	a.metrics.Gauge("runtime.heap_objects", float64(mem.HeapObjects))
	a.metrics.Gauge("runtime.gc_pause_ns", float64(mem.PauseNs[(mem.NumGC+255)%256]))
	a.metrics.Counter("runtime.gc_runs", float64(mem.NumGC))
	a.metrics.Gauge("runtime.num_cpu", float64(runtime.NumCPU()))

	// Send metrics
	metrics := a.metrics.Flush()
	if len(metrics) > 0 {
		if err := a.transport.SendMetrics(metrics); err != nil {
			if a.config.Debug {
				log.Printf("[apm] Failed to send metrics: %v", err)
			}
		}
	}
}

// Helper functions

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Convenience functions using global agent

// StartSpan starts a span using the global agent
func StartSpan(operationName string, opts ...SpanOption) *Span {
	agent := GetAgent()
	if agent == nil {
		return &Span{} // Return no-op span
	}
	return agent.StartSpan(operationName, opts...)
}

// StartSpanFromContext starts a child span using the global agent
func StartSpanFromContext(ctx context.Context, operationName string, opts ...SpanOption) (*Span, context.Context) {
	agent := GetAgent()
	if agent == nil {
		return &Span{}, ctx
	}
	return agent.StartSpanFromContext(ctx, operationName, opts...)
}
