package runtime

import (
	"encoding/json"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// MemoryConfig configures the memory management system
type MemoryConfig struct {
	// MaxMemoryMB is the target maximum memory usage in MB
	MaxMemoryMB int64

	// GCTargetPercent is the GOGC target percentage
	GCTargetPercent int

	// EnableObjectPools enables object pooling for common types
	EnableObjectPools bool

	// EnableBufferRecycling enables buffer recycling
	EnableBufferRecycling bool

	// PressureThreshold is the percentage at which to start shedding load (0-100)
	PressureThreshold int

	// CriticalThreshold is the percentage at which aggressive measures kick in (0-100)
	CriticalThreshold int

	// MetricsInterval is how often to collect memory metrics
	MetricsInterval time.Duration

	// StreamingBatchSize is the batch size for streaming processing
	StreamingBatchSize int

	// MaxBufferPoolSize is the max size of the buffer pool
	MaxBufferPoolSize int
}

// DefaultMemoryConfig returns sensible defaults for 500MB target
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		MaxMemoryMB:           500,
		GCTargetPercent:       50,  // More aggressive GC
		EnableObjectPools:     true,
		EnableBufferRecycling: true,
		PressureThreshold:     70,
		CriticalThreshold:     85,
		MetricsInterval:       5 * time.Second,
		StreamingBatchSize:    1000,
		MaxBufferPoolSize:     10000,
	}
}

// MemoryPressure represents the current memory pressure level
type MemoryPressure int

const (
	PressureNormal MemoryPressure = iota
	PressureElevated
	PressureCritical
)

func (p MemoryPressure) String() string {
	switch p {
	case PressureNormal:
		return "normal"
	case PressureElevated:
		return "elevated"
	case PressureCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// MemoryMetrics holds current memory statistics
type MemoryMetrics struct {
	Timestamp        time.Time      `json:"timestamp"`
	AllocMB          float64        `json:"alloc_mb"`
	TotalAllocMB     float64        `json:"total_alloc_mb"`
	SysMB            float64        `json:"sys_mb"`
	HeapAllocMB      float64        `json:"heap_alloc_mb"`
	HeapSysMB        float64        `json:"heap_sys_mb"`
	HeapIdleMB       float64        `json:"heap_idle_mb"`
	HeapInuseMB      float64        `json:"heap_inuse_mb"`
	HeapReleasedMB   float64        `json:"heap_released_mb"`
	HeapObjects      uint64         `json:"heap_objects"`
	StackInuseMB     float64        `json:"stack_inuse_mb"`
	NumGC            uint32         `json:"num_gc"`
	GCPauseTotalMs   float64        `json:"gc_pause_total_ms"`
	LastGCPauseMs    float64        `json:"last_gc_pause_ms"`
	GCCPUPercent     float64        `json:"gc_cpu_percent"`
	Pressure         MemoryPressure `json:"pressure"`
	PressureStr      string         `json:"pressure_str"`
	UsagePercent     float64        `json:"usage_percent"`
	Goroutines       int            `json:"goroutines"`
}

// MemoryManager manages memory usage and provides backpressure
type MemoryManager struct {
	config MemoryConfig

	// Current state
	pressure     atomic.Int32
	usagePercent atomic.Int64

	// Metrics history
	metricsHistory []MemoryMetrics
	metricsLimit   int
	metricsMu      sync.RWMutex

	// Object pools
	spanPool       sync.Pool
	logEntryPool   sync.Pool
	metricPool     sync.Pool
	bufferPool     *BufferPool
	bufferPoolSize atomic.Int64

	// Backpressure callbacks
	onPressureChange func(MemoryPressure)

	// Statistics
	totalAllocations   int64
	pooledAllocations  int64
	droppedDuePressure int64
	forcedGCs          int64

	// Lifecycle
	stopCh  chan struct{}
	started bool
	mu      sync.Mutex
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(config MemoryConfig) *MemoryManager {
	m := &MemoryManager{
		config:         config,
		metricsLimit:   1000,
		metricsHistory: make([]MemoryMetrics, 0, 1000),
		stopCh:         make(chan struct{}),
	}

	// Configure GC
	if config.GCTargetPercent > 0 {
		debug.SetGCPercent(config.GCTargetPercent)
	}

	// Set memory limit if specified
	if config.MaxMemoryMB > 0 {
		debug.SetMemoryLimit(config.MaxMemoryMB * 1024 * 1024)
	}

	// Initialize object pools if enabled
	if config.EnableObjectPools {
		m.initPools()
	}

	// Initialize buffer pool if enabled
	if config.EnableBufferRecycling {
		m.bufferPool = NewBufferPool(config.MaxBufferPoolSize)
	}

	return m
}

// initPools initializes sync.Pools for common objects
func (m *MemoryManager) initPools() {
	// Span pool (adjust based on your Span struct size)
	m.spanPool = sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&m.totalAllocations, 1)
			return &PooledSpan{
				Attributes: make(map[string]string, 8),
			}
		},
	}

	// Log entry pool
	m.logEntryPool = sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&m.totalAllocations, 1)
			return &PooledLogEntry{
				Labels: make(map[string]string, 8),
			}
		},
	}

	// Metric pool
	m.metricPool = sync.Pool{
		New: func() interface{} {
			atomic.AddInt64(&m.totalAllocations, 1)
			return &PooledMetric{
				Tags: make(map[string]string, 8),
			}
		},
	}
}

// PooledSpan is a poolable span object
type PooledSpan struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Name         string
	ServiceName  string
	Kind         string
	StartTime    time.Time
	EndTime      time.Time
	DurationMs   float64
	Status       string
	StatusMsg    string
	Attributes   map[string]string
}

// Reset clears the span for reuse
func (s *PooledSpan) Reset() {
	s.TraceID = ""
	s.SpanID = ""
	s.ParentSpanID = ""
	s.Name = ""
	s.ServiceName = ""
	s.Kind = ""
	s.StartTime = time.Time{}
	s.EndTime = time.Time{}
	s.DurationMs = 0
	s.Status = ""
	s.StatusMsg = ""
	for k := range s.Attributes {
		delete(s.Attributes, k)
	}
}

// PooledLogEntry is a poolable log entry
type PooledLogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Service   string
	Labels    map[string]string
}

// Reset clears the log entry for reuse
func (l *PooledLogEntry) Reset() {
	l.Timestamp = time.Time{}
	l.Level = ""
	l.Message = ""
	l.Service = ""
	for k := range l.Labels {
		delete(l.Labels, k)
	}
}

// PooledMetric is a poolable metric
type PooledMetric struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Tags      map[string]string
}

// Reset clears the metric for reuse
func (m *PooledMetric) Reset() {
	m.Name = ""
	m.Value = 0
	m.Timestamp = time.Time{}
	for k := range m.Tags {
		delete(m.Tags, k)
	}
}

// AcquireSpan gets a span from the pool
func (m *MemoryManager) AcquireSpan() *PooledSpan {
	if !m.config.EnableObjectPools {
		return &PooledSpan{Attributes: make(map[string]string, 8)}
	}
	atomic.AddInt64(&m.pooledAllocations, 1)
	return m.spanPool.Get().(*PooledSpan)
}

// ReleaseSpan returns a span to the pool
func (m *MemoryManager) ReleaseSpan(s *PooledSpan) {
	if !m.config.EnableObjectPools || s == nil {
		return
	}
	s.Reset()
	m.spanPool.Put(s)
}

// AcquireLogEntry gets a log entry from the pool
func (m *MemoryManager) AcquireLogEntry() *PooledLogEntry {
	if !m.config.EnableObjectPools {
		return &PooledLogEntry{Labels: make(map[string]string, 8)}
	}
	atomic.AddInt64(&m.pooledAllocations, 1)
	return m.logEntryPool.Get().(*PooledLogEntry)
}

// ReleaseLogEntry returns a log entry to the pool
func (m *MemoryManager) ReleaseLogEntry(l *PooledLogEntry) {
	if !m.config.EnableObjectPools || l == nil {
		return
	}
	l.Reset()
	m.logEntryPool.Put(l)
}

// AcquireMetric gets a metric from the pool
func (m *MemoryManager) AcquireMetric() *PooledMetric {
	if !m.config.EnableObjectPools {
		return &PooledMetric{Tags: make(map[string]string, 8)}
	}
	atomic.AddInt64(&m.pooledAllocations, 1)
	return m.metricPool.Get().(*PooledMetric)
}

// ReleaseMetric returns a metric to the pool
func (m *MemoryManager) ReleaseMetric(metric *PooledMetric) {
	if !m.config.EnableObjectPools || metric == nil {
		return
	}
	metric.Reset()
	m.metricPool.Put(metric)
}

// BufferPool manages a pool of byte buffers
type BufferPool struct {
	pool     sync.Pool
	maxSize  int
	acquired int64
	released int64
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(maxSize int) *BufferPool {
	return &BufferPool{
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 4096)
			},
		},
	}
}

// Get acquires a buffer from the pool
func (p *BufferPool) Get() []byte {
	atomic.AddInt64(&p.acquired, 1)
	buf := p.pool.Get().([]byte)
	return buf[:0]
}

// Put returns a buffer to the pool
func (p *BufferPool) Put(buf []byte) {
	// Don't pool very large buffers
	if cap(buf) > 1024*1024 {
		return
	}
	atomic.AddInt64(&p.released, 1)
	p.pool.Put(buf[:0])
}

// Stats returns buffer pool statistics
func (p *BufferPool) Stats() BufferPoolStats {
	return BufferPoolStats{
		Acquired:    atomic.LoadInt64(&p.acquired),
		Released:    atomic.LoadInt64(&p.released),
		Outstanding: atomic.LoadInt64(&p.acquired) - atomic.LoadInt64(&p.released),
	}
}

// BufferPoolStats holds buffer pool statistics
type BufferPoolStats struct {
	Acquired    int64 `json:"acquired"`
	Released    int64 `json:"released"`
	Outstanding int64 `json:"outstanding"`
}

// GetBuffer acquires a buffer from the pool
func (m *MemoryManager) GetBuffer() []byte {
	if m.bufferPool == nil {
		return make([]byte, 0, 4096)
	}
	return m.bufferPool.Get()
}

// PutBuffer returns a buffer to the pool
func (m *MemoryManager) PutBuffer(buf []byte) {
	if m.bufferPool == nil {
		return
	}
	m.bufferPool.Put(buf)
}

// Start begins memory monitoring
func (m *MemoryManager) Start() {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	m.mu.Unlock()

	go m.monitorLoop()
	log.Printf("[memory] Manager started (target: %dMB, GC: %d%%)",
		m.config.MaxMemoryMB, m.config.GCTargetPercent)
}

// Stop halts memory monitoring
func (m *MemoryManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return
	}

	close(m.stopCh)
	m.started = false
}

// monitorLoop periodically collects metrics and checks pressure
func (m *MemoryManager) monitorLoop() {
	ticker := time.NewTicker(m.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := m.collectMetrics()
			m.recordMetrics(metrics)
			m.updatePressure(metrics)
		case <-m.stopCh:
			return
		}
	}
}

// collectMetrics gathers current memory statistics
func (m *MemoryManager) collectMetrics() MemoryMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	usagePercent := float64(memStats.Alloc) / float64(m.config.MaxMemoryMB*1024*1024) * 100
	m.usagePercent.Store(int64(usagePercent * 100)) // Store as fixed point

	pressure := PressureNormal
	if usagePercent >= float64(m.config.CriticalThreshold) {
		pressure = PressureCritical
	} else if usagePercent >= float64(m.config.PressureThreshold) {
		pressure = PressureElevated
	}

	var lastGCPause float64
	if memStats.NumGC > 0 {
		lastGCPause = float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1e6
	}

	return MemoryMetrics{
		Timestamp:        time.Now(),
		AllocMB:          float64(memStats.Alloc) / 1024 / 1024,
		TotalAllocMB:     float64(memStats.TotalAlloc) / 1024 / 1024,
		SysMB:            float64(memStats.Sys) / 1024 / 1024,
		HeapAllocMB:      float64(memStats.HeapAlloc) / 1024 / 1024,
		HeapSysMB:        float64(memStats.HeapSys) / 1024 / 1024,
		HeapIdleMB:       float64(memStats.HeapIdle) / 1024 / 1024,
		HeapInuseMB:      float64(memStats.HeapInuse) / 1024 / 1024,
		HeapReleasedMB:   float64(memStats.HeapReleased) / 1024 / 1024,
		HeapObjects:      memStats.HeapObjects,
		StackInuseMB:     float64(memStats.StackInuse) / 1024 / 1024,
		NumGC:            memStats.NumGC,
		GCPauseTotalMs:   float64(memStats.PauseTotalNs) / 1e6,
		LastGCPauseMs:    lastGCPause,
		GCCPUPercent:     memStats.GCCPUFraction * 100,
		Pressure:         pressure,
		PressureStr:      pressure.String(),
		UsagePercent:     usagePercent,
		Goroutines:       runtime.NumGoroutine(),
	}
}

// recordMetrics stores metrics in history
func (m *MemoryManager) recordMetrics(metrics MemoryMetrics) {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()

	m.metricsHistory = append(m.metricsHistory, metrics)

	// Trim history if needed
	if len(m.metricsHistory) > m.metricsLimit {
		m.metricsHistory = m.metricsHistory[1:]
	}
}

// updatePressure checks and updates memory pressure state
func (m *MemoryManager) updatePressure(metrics MemoryMetrics) {
	oldPressure := MemoryPressure(m.pressure.Load())
	newPressure := metrics.Pressure

	if newPressure != oldPressure {
		m.pressure.Store(int32(newPressure))

		log.Printf("[memory] Pressure changed: %s -> %s (%.1f%% of %dMB)",
			oldPressure, newPressure, metrics.UsagePercent, m.config.MaxMemoryMB)

		if m.onPressureChange != nil {
			m.onPressureChange(newPressure)
		}

		// Take action on critical pressure
		if newPressure == PressureCritical {
			m.handleCriticalPressure()
		}
	}
}

// handleCriticalPressure takes action when memory is critically low
func (m *MemoryManager) handleCriticalPressure() {
	atomic.AddInt64(&m.forcedGCs, 1)

	// Force garbage collection
	runtime.GC()

	// Try to release memory back to OS
	debug.FreeOSMemory()

	log.Printf("[memory] Forced GC due to critical pressure")
}

// GetPressure returns the current memory pressure level
func (m *MemoryManager) GetPressure() MemoryPressure {
	return MemoryPressure(m.pressure.Load())
}

// ShouldAccept returns true if we should accept new work
// Use this for load shedding
func (m *MemoryManager) ShouldAccept() bool {
	pressure := m.GetPressure()

	if pressure == PressureCritical {
		atomic.AddInt64(&m.droppedDuePressure, 1)
		return false
	}

	if pressure == PressureElevated {
		// Accept 50% of requests under elevated pressure
		if time.Now().UnixNano()%2 == 0 {
			atomic.AddInt64(&m.droppedDuePressure, 1)
			return false
		}
	}

	return true
}

// GetMetrics returns the current memory metrics
func (m *MemoryManager) GetMetrics() MemoryMetrics {
	return m.collectMetrics()
}

// GetMetricsHistory returns historical metrics
func (m *MemoryManager) GetMetricsHistory(since time.Duration) []MemoryMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	cutoff := time.Now().Add(-since)
	var result []MemoryMetrics

	for _, metrics := range m.metricsHistory {
		if metrics.Timestamp.After(cutoff) {
			result = append(result, metrics)
		}
	}

	return result
}

// GetStats returns memory manager statistics
func (m *MemoryManager) GetStats() MemoryManagerStats {
	var bufferStats BufferPoolStats
	if m.bufferPool != nil {
		bufferStats = m.bufferPool.Stats()
	}

	return MemoryManagerStats{
		TotalAllocations:   atomic.LoadInt64(&m.totalAllocations),
		PooledAllocations:  atomic.LoadInt64(&m.pooledAllocations),
		DroppedDuePressure: atomic.LoadInt64(&m.droppedDuePressure),
		ForcedGCs:          atomic.LoadInt64(&m.forcedGCs),
		CurrentPressure:    m.GetPressure().String(),
		BufferPoolStats:    bufferStats,
	}
}

// MemoryManagerStats holds manager statistics
type MemoryManagerStats struct {
	TotalAllocations   int64           `json:"total_allocations"`
	PooledAllocations  int64           `json:"pooled_allocations"`
	DroppedDuePressure int64           `json:"dropped_due_pressure"`
	ForcedGCs          int64           `json:"forced_gcs"`
	CurrentPressure    string          `json:"current_pressure"`
	BufferPoolStats    BufferPoolStats `json:"buffer_pool_stats"`
}

// SetOnPressureChange sets the callback for pressure changes
func (m *MemoryManager) SetOnPressureChange(fn func(MemoryPressure)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPressureChange = fn
}

// UpdateConfig updates the memory configuration
func (m *MemoryManager) UpdateConfig(config MemoryConfig) {
	m.mu.Lock()
	m.config = config
	m.mu.Unlock()

	// Update GC settings
	if config.GCTargetPercent > 0 {
		debug.SetGCPercent(config.GCTargetPercent)
	}

	if config.MaxMemoryMB > 0 {
		debug.SetMemoryLimit(config.MaxMemoryMB * 1024 * 1024)
	}

	log.Printf("[memory] Config updated: target %dMB, GC %d%%",
		config.MaxMemoryMB, config.GCTargetPercent)
}

// GetConfig returns the current configuration
func (m *MemoryManager) GetConfig() MemoryConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// ForceGC triggers garbage collection
func (m *MemoryManager) ForceGC() {
	atomic.AddInt64(&m.forcedGCs, 1)
	runtime.GC()
}

// FreeOSMemory releases memory back to the OS
func (m *MemoryManager) FreeOSMemory() {
	debug.FreeOSMemory()
}

// StreamingBatcher helps process data in memory-efficient batches
type StreamingBatcher[T any] struct {
	batchSize int
	batch     []T
	process   func([]T) error
	mu        sync.Mutex
}

// NewStreamingBatcher creates a new streaming batcher
func NewStreamingBatcher[T any](batchSize int, process func([]T) error) *StreamingBatcher[T] {
	return &StreamingBatcher[T]{
		batchSize: batchSize,
		batch:     make([]T, 0, batchSize),
		process:   process,
	}
}

// Add adds an item to the batch, processing if batch is full
func (b *StreamingBatcher[T]) Add(item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.batch = append(b.batch, item)

	if len(b.batch) >= b.batchSize {
		return b.flush()
	}

	return nil
}

// Flush processes any remaining items
func (b *StreamingBatcher[T]) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

func (b *StreamingBatcher[T]) flush() error {
	if len(b.batch) == 0 {
		return nil
	}

	err := b.process(b.batch)
	b.batch = b.batch[:0]
	return err
}

// MarshalJSON for stats serialization
func (s MemoryManagerStats) MarshalJSON() ([]byte, error) {
	type Alias MemoryManagerStats
	return json.Marshal(Alias(s))
}

// MemoryReport provides a comprehensive memory report
type MemoryReport struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Config      MemoryConfig        `json:"config"`
	Current     MemoryMetrics       `json:"current"`
	Stats       MemoryManagerStats  `json:"stats"`
	History     []MemoryMetrics     `json:"history,omitempty"`
}

// GenerateReport creates a comprehensive memory report
func (m *MemoryManager) GenerateReport(includeHistory bool) *MemoryReport {
	report := &MemoryReport{
		GeneratedAt: time.Now(),
		Config:      m.GetConfig(),
		Current:     m.GetMetrics(),
		Stats:       m.GetStats(),
	}

	if includeHistory {
		report.History = m.GetMetricsHistory(time.Hour)
	}

	return report
}
