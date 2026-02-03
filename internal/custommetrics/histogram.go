package custommetrics

import (
	"time"
)

// HistogramDataPoint represents a native histogram data point
type HistogramDataPoint struct {
	Timestamp      time.Time         `json:"timestamp"`
	Name           string            `json:"name"`
	Tags           map[string]string `json:"tags,omitempty"`
	Count          uint64            `json:"count"`
	Sum            float64           `json:"sum"`
	Min            *float64          `json:"min,omitempty"`
	Max            *float64          `json:"max,omitempty"`
	ExplicitBounds []float64         `json:"explicit_bounds"` // Bucket upper bounds
	BucketCounts   []uint64          `json:"bucket_counts"`   // Cumulative counts per bucket
	Exemplars      []Exemplar        `json:"exemplars,omitempty"`
}

// HistogramSnapshot represents an aggregated view of histogram data for queries
type HistogramSnapshot struct {
	Bounds     []float64 `json:"bounds"`
	Counts     []uint64  `json:"counts"`
	TotalCount uint64    `json:"total_count"`
	Sum        float64   `json:"sum"`
	Min        float64   `json:"min"`
	Max        float64   `json:"max"`
}

// Exemplar represents a sample trace association with a histogram bucket
type Exemplar struct {
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	TraceID   string    `json:"trace_id,omitempty"`
	SpanID    string    `json:"span_id,omitempty"`
}

// Quantile computes the estimated quantile value from the histogram snapshot
// using Prometheus-compatible linear interpolation within buckets.
// quantile should be between 0.0 and 1.0 (e.g., 0.99 for p99)
func (s *HistogramSnapshot) Quantile(quantile float64) float64 {
	if quantile < 0 {
		quantile = 0
	}
	if quantile > 1 {
		quantile = 1
	}

	if s.TotalCount == 0 {
		return 0
	}

	// Target rank (1-indexed count we're looking for)
	rank := quantile * float64(s.TotalCount)

	// Find the bucket containing our target rank
	var prevCount uint64
	var prevBound float64

	for i, count := range s.Counts {
		if count == 0 {
			continue
		}

		// count is cumulative, so bucketCount = count - prevCount
		if float64(count) >= rank {
			// Found the bucket containing our quantile
			bucketCount := float64(count - prevCount)
			if bucketCount == 0 {
				prevCount = count
				if i < len(s.Bounds) {
					prevBound = s.Bounds[i]
				}
				continue
			}

			// Position within this bucket
			posInBucket := rank - float64(prevCount)
			fraction := posInBucket / bucketCount

			// Upper bound for this bucket
			var upperBound float64
			if i < len(s.Bounds) {
				upperBound = s.Bounds[i]
			} else {
				// +Inf bucket - use Max if available, otherwise extrapolate
				if s.Max > 0 {
					upperBound = s.Max
				} else if len(s.Bounds) > 0 {
					upperBound = s.Bounds[len(s.Bounds)-1] * 2
				} else {
					upperBound = 1
				}
			}

			// Linear interpolation within bucket
			return prevBound + fraction*(upperBound-prevBound)
		}

		prevCount = count
		if i < len(s.Bounds) {
			prevBound = s.Bounds[i]
		}
	}

	// If we get here, return the max bound
	if len(s.Bounds) > 0 {
		return s.Bounds[len(s.Bounds)-1]
	}
	return s.Max
}

// Merge combines another histogram snapshot into this one
// Both snapshots must have compatible bucket bounds
func (s *HistogramSnapshot) Merge(other *HistogramSnapshot) {
	if other == nil || other.TotalCount == 0 {
		return
	}

	// If this snapshot is empty, just copy from other
	if s.TotalCount == 0 {
		s.Bounds = append([]float64{}, other.Bounds...)
		s.Counts = append([]uint64{}, other.Counts...)
		s.TotalCount = other.TotalCount
		s.Sum = other.Sum
		s.Min = other.Min
		s.Max = other.Max
		return
	}

	// Merge counts (assumes compatible bounds)
	for i := range s.Counts {
		if i < len(other.Counts) {
			s.Counts[i] += other.Counts[i]
		}
	}

	s.TotalCount += other.TotalCount
	s.Sum += other.Sum

	if other.Min < s.Min {
		s.Min = other.Min
	}
	if other.Max > s.Max {
		s.Max = other.Max
	}
}

// ToSnapshot converts a HistogramDataPoint to a HistogramSnapshot
func (h *HistogramDataPoint) ToSnapshot() *HistogramSnapshot {
	snapshot := &HistogramSnapshot{
		Bounds:     append([]float64{}, h.ExplicitBounds...),
		Counts:     append([]uint64{}, h.BucketCounts...),
		TotalCount: h.Count,
		Sum:        h.Sum,
	}

	if h.Min != nil {
		snapshot.Min = *h.Min
	}
	if h.Max != nil {
		snapshot.Max = *h.Max
	}

	return snapshot
}

// AggregateHistograms combines multiple histogram data points into a single snapshot
func AggregateHistograms(points []HistogramDataPoint) *HistogramSnapshot {
	if len(points) == 0 {
		return &HistogramSnapshot{}
	}

	result := points[0].ToSnapshot()
	for i := 1; i < len(points); i++ {
		result.Merge(points[i].ToSnapshot())
	}
	return result
}
