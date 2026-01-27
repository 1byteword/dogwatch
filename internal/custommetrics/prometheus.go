package custommetrics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

// PrometheusReceiver handles Prometheus remote write requests
type PrometheusReceiver struct {
	store *Store

	// Stats tracking
	mu             sync.RWMutex
	totalRequests  int64
	totalSamples   int64
	totalSeries    int64
	totalErrors    int64
	lastReceived   time.Time
	seriesSeen     map[string]struct{}
	recentErrors   []string
}

// NewPrometheusReceiver creates a new Prometheus remote write receiver
func NewPrometheusReceiver(store *Store) *PrometheusReceiver {
	return &PrometheusReceiver{
		store:      store,
		seriesSeen: make(map[string]struct{}),
	}
}

// HandleRemoteWrite handles POST /api/v1/write from Prometheus
func (r *PrometheusReceiver) HandleRemoteWrite(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	atomic.AddInt64(&r.totalRequests, 1)

	// Read compressed body
	compressed, err := io.ReadAll(req.Body)
	if err != nil {
		r.recordError(fmt.Sprintf("read body: %v", err))
		http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Decompress with snappy
	decompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		r.recordError(fmt.Sprintf("decompress: %v", err))
		http.Error(w, fmt.Sprintf("Failed to decompress: %v", err), http.StatusBadRequest)
		return
	}

	// Parse protobuf
	var writeReq prompb.WriteRequest
	if err := writeReq.Unmarshal(decompressed); err != nil {
		r.recordError(fmt.Sprintf("parse protobuf: %v", err))
		http.Error(w, fmt.Sprintf("Failed to parse protobuf: %v", err), http.StatusBadRequest)
		return
	}

	// Convert to DataPoints
	points, seriesKeys := r.convertTimeSeriesWithKeys(writeReq.Timeseries)

	// Track series
	r.mu.Lock()
	for _, key := range seriesKeys {
		r.seriesSeen[key] = struct{}{}
	}
	r.totalSeries = int64(len(r.seriesSeen))
	r.lastReceived = time.Now()
	r.mu.Unlock()

	// Store
	if len(points) > 0 {
		if err := r.store.RecordBatch(points); err != nil {
			r.recordError(fmt.Sprintf("store: %v", err))
			log.Printf("Failed to store prometheus metrics: %v", err)
			http.Error(w, fmt.Sprintf("Failed to store: %v", err), http.StatusInternalServerError)
			return
		}
		atomic.AddInt64(&r.totalSamples, int64(len(points)))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (r *PrometheusReceiver) recordError(msg string) {
	atomic.AddInt64(&r.totalErrors, 1)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recentErrors = append(r.recentErrors, fmt.Sprintf("%s: %s", time.Now().Format(time.RFC3339), msg))
	if len(r.recentErrors) > 10 {
		r.recentErrors = r.recentErrors[1:]
	}
}

// convertTimeSeries converts Prometheus TimeSeries to DataPoints
func (r *PrometheusReceiver) convertTimeSeries(timeseries []prompb.TimeSeries) []DataPoint {
	points, _ := r.convertTimeSeriesWithKeys(timeseries)
	return points
}

// convertTimeSeriesWithKeys converts Prometheus TimeSeries to DataPoints and returns series keys
func (r *PrometheusReceiver) convertTimeSeriesWithKeys(timeseries []prompb.TimeSeries) ([]DataPoint, []string) {
	var points []DataPoint
	var seriesKeys []string

	for _, ts := range timeseries {
		// Extract metric name and labels
		var name string
		tags := make(map[string]string)

		for _, label := range ts.Labels {
			if label.Name == "__name__" {
				name = label.Value
			} else {
				tags[label.Name] = label.Value
			}
		}

		if name == "" {
			continue
		}

		// Build series key for tracking unique series
		seriesKey := buildSeriesKey(name, tags)
		seriesKeys = append(seriesKeys, seriesKey)

		// Determine metric type from name suffix
		metricType := guessMetricType(name)

		// Convert samples
		for _, sample := range ts.Samples {
			// Prometheus timestamps are in milliseconds
			ts := time.Unix(0, sample.Timestamp*int64(time.Millisecond))

			points = append(points, DataPoint{
				Timestamp: ts,
				Name:      name,
				Type:      metricType,
				Value:     sample.Value,
				Tags:      copyTags(tags),
			})
		}

		// Handle histograms if present
		for _, h := range ts.Histograms {
			ts := time.Unix(0, h.Timestamp*int64(time.Millisecond))

			// Store count
			countTags := copyTags(tags)
			countTags["le"] = "+Inf"
			points = append(points, DataPoint{
				Timestamp: ts,
				Name:      name + "_bucket",
				Type:      Counter,
				Value:     float64(h.Count.(*prompb.Histogram_CountInt).CountInt),
				Tags:      countTags,
			})

			// Store sum
			points = append(points, DataPoint{
				Timestamp: ts,
				Name:      name + "_sum",
				Type:      Counter,
				Value:     h.Sum,
				Tags:      copyTags(tags),
			})
		}
	}

	return points, seriesKeys
}

func buildSeriesKey(name string, tags map[string]string) string {
	// Simple key: name + sorted tags
	key := name
	for k, v := range tags {
		key += "|" + k + "=" + v
	}
	return key
}

// guessMetricType guesses the metric type from the name
func guessMetricType(name string) MetricType {
	// Common suffixes
	switch {
	case endsWith(name, "_total"):
		return Counter
	case endsWith(name, "_count"):
		return Counter
	case endsWith(name, "_sum"):
		return Counter
	case endsWith(name, "_bucket"):
		return Counter
	case endsWith(name, "_created"):
		return Gauge
	case endsWith(name, "_info"):
		return Gauge
	default:
		return Gauge
	}
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func copyTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}
	result := make(map[string]string, len(tags))
	for k, v := range tags {
		result[k] = v
	}
	return result
}

// PrometheusStats returns stats about received metrics
type PrometheusStats struct {
	TotalRequests  int64     `json:"total_requests"`
	TotalSamples   int64     `json:"total_samples"`
	TotalSeries    int64     `json:"total_series"`
	TotalErrors    int64     `json:"total_errors"`
	LastReceived   time.Time `json:"last_received,omitempty"`
	RecentErrors   []string  `json:"recent_errors,omitempty"`
}

// GetStats returns Prometheus receiver statistics
func (r *PrometheusReceiver) GetStats() PrometheusStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return PrometheusStats{
		TotalRequests:  atomic.LoadInt64(&r.totalRequests),
		TotalSamples:   atomic.LoadInt64(&r.totalSamples),
		TotalSeries:    r.totalSeries,
		TotalErrors:    atomic.LoadInt64(&r.totalErrors),
		LastReceived:   r.lastReceived,
		RecentErrors:   r.recentErrors,
	}
}

// HandleStats handles GET /api/v1/write/stats
func (r *PrometheusReceiver) HandleStats(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := r.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
