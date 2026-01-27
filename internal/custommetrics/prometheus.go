package custommetrics

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

// PrometheusReceiver handles Prometheus remote write requests
type PrometheusReceiver struct {
	store *Store
}

// NewPrometheusReceiver creates a new Prometheus remote write receiver
func NewPrometheusReceiver(store *Store) *PrometheusReceiver {
	return &PrometheusReceiver{store: store}
}

// HandleRemoteWrite handles POST /api/v1/write from Prometheus
func (r *PrometheusReceiver) HandleRemoteWrite(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read compressed body
	compressed, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Decompress with snappy
	decompressed, err := snappy.Decode(nil, compressed)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to decompress: %v", err), http.StatusBadRequest)
		return
	}

	// Parse protobuf
	var writeReq prompb.WriteRequest
	if err := writeReq.Unmarshal(decompressed); err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse protobuf: %v", err), http.StatusBadRequest)
		return
	}

	// Convert to DataPoints
	points := r.convertTimeSeries(writeReq.Timeseries)

	// Store
	if len(points) > 0 {
		if err := r.store.RecordBatch(points); err != nil {
			log.Printf("Failed to store prometheus metrics: %v", err)
			http.Error(w, fmt.Sprintf("Failed to store: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// convertTimeSeries converts Prometheus TimeSeries to DataPoints
func (r *PrometheusReceiver) convertTimeSeries(timeseries []prompb.TimeSeries) []DataPoint {
	var points []DataPoint

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

	return points
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
	TotalSeries   int64 `json:"total_series"`
	TotalSamples  int64 `json:"total_samples"`
	LastReceived  time.Time `json:"last_received,omitempty"`
}
