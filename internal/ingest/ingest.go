// Package ingest provides multi-protocol metric ingestion
// Supports: Graphite, InfluxDB, OpenTSDB, DataDog, Prometheus remote read
package ingest

import (
	"time"
)

// Sample represents a single metric sample
type Sample struct {
	Metric    string
	Value     float64
	Timestamp time.Time
	Tags      map[string]string
}

// Batch represents a batch of samples from any protocol
type Batch struct {
	Samples []Sample
	Source  string // Protocol source: graphite, influx, opentsdb, datadog
}

// MetricStore is the interface for storing ingested metrics
type MetricStore interface {
	WriteSamples(samples []Sample) error
}
