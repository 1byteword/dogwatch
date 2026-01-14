package apm

import (
	"sync"
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	MetricTypeGauge   MetricType = "gauge"
	MetricTypeCounter MetricType = "counter"
	MetricTypeHist    MetricType = "histogram"
)

// Metric represents a single metric point
type Metric struct {
	Name      string            `json:"name"`
	Type      MetricType        `json:"type"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// MetricsCollector collects and aggregates metrics
type MetricsCollector struct {
	service  string
	metrics  []*Metric
	counters map[string]float64
	mu       sync.Mutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(service string) *MetricsCollector {
	return &MetricsCollector{
		service:  service,
		metrics:  make([]*Metric, 0),
		counters: make(map[string]float64),
	}
}

// Gauge records a gauge metric
func (m *MetricsCollector) Gauge(name string, value float64, tags ...map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeGauge,
		Value:     value,
		Tags:      mergeTags(tags...),
		Timestamp: time.Now(),
	}
	metric.Tags["service"] = m.service
	m.metrics = append(m.metrics, metric)
}

// Counter increments a counter metric
func (m *MetricsCollector) Counter(name string, value float64, tags ...map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := metricKey(name, tags...)
	m.counters[key] += value

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeCounter,
		Value:     m.counters[key],
		Tags:      mergeTags(tags...),
		Timestamp: time.Now(),
	}
	metric.Tags["service"] = m.service
	m.metrics = append(m.metrics, metric)
}

// Histogram records a histogram metric (distribution)
func (m *MetricsCollector) Histogram(name string, value float64, tags ...map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	metric := &Metric{
		Name:      name,
		Type:      MetricTypeHist,
		Value:     value,
		Tags:      mergeTags(tags...),
		Timestamp: time.Now(),
	}
	metric.Tags["service"] = m.service
	m.metrics = append(m.metrics, metric)
}

// Timing records a timing metric in milliseconds
func (m *MetricsCollector) Timing(name string, duration time.Duration, tags ...map[string]string) {
	m.Histogram(name, float64(duration.Milliseconds()), tags...)
}

// Flush returns and clears all collected metrics
func (m *MetricsCollector) Flush() []*Metric {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := m.metrics
	m.metrics = make([]*Metric, 0)
	return metrics
}

// Helper functions

func mergeTags(tagMaps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, tags := range tagMaps {
		for k, v := range tags {
			result[k] = v
		}
	}
	return result
}

func metricKey(name string, tags ...map[string]string) string {
	key := name
	merged := mergeTags(tags...)
	for k, v := range merged {
		key += "|" + k + "=" + v
	}
	return key
}

// Global metrics functions using the global agent

// RecordGauge records a gauge metric
func RecordGauge(name string, value float64, tags ...map[string]string) {
	agent := GetAgent()
	if agent == nil || agent.metrics == nil {
		return
	}
	agent.metrics.Gauge(name, value, tags...)
}

// RecordCounter increments a counter
func RecordCounter(name string, value float64, tags ...map[string]string) {
	agent := GetAgent()
	if agent == nil || agent.metrics == nil {
		return
	}
	agent.metrics.Counter(name, value, tags...)
}

// RecordHistogram records a histogram value
func RecordHistogram(name string, value float64, tags ...map[string]string) {
	agent := GetAgent()
	if agent == nil || agent.metrics == nil {
		return
	}
	agent.metrics.Histogram(name, value, tags...)
}

// RecordTiming records a timing in milliseconds
func RecordTiming(name string, duration time.Duration, tags ...map[string]string) {
	agent := GetAgent()
	if agent == nil || agent.metrics == nil {
		return
	}
	agent.metrics.Timing(name, duration, tags...)
}

// Timer is a convenience for timing operations
type Timer struct {
	name    string
	start   time.Time
	tags    map[string]string
	stopped bool
}

// NewTimer creates a new timer
func NewTimer(name string, tags ...map[string]string) *Timer {
	return &Timer{
		name:  name,
		start: time.Now(),
		tags:  mergeTags(tags...),
	}
}

// Stop stops the timer and records the duration
func (t *Timer) Stop() time.Duration {
	if t.stopped {
		return 0
	}
	t.stopped = true
	duration := time.Since(t.start)
	RecordTiming(t.name, duration, t.tags)
	return duration
}
