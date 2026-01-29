package bubbleup

import "time"

// AnalysisRequest specifies what to analyze
type AnalysisRequest struct {
	// Direct span selection
	AnomalousSpanIDs []string `json:"anomalous_span_ids,omitempty"`
	BaselineSpanIDs  []string `json:"baseline_span_ids,omitempty"`

	// Filter-based selection
	Service           string    `json:"service,omitempty"`
	TimeStart         time.Time `json:"time_start"`
	TimeEnd           time.Time `json:"time_end"`
	Mode              string    `json:"mode"` // "latency" or "errors"
	LatencyPercentile float64   `json:"latency_percentile,omitempty"`
	LatencyThreshold  float64   `json:"latency_threshold_ms,omitempty"`
}

// DimensionAnalysis shows how a single dimension differs between anomalous and baseline sets
type DimensionAnalysis struct {
	Dimension        string             `json:"dimension"`
	Divergence       float64            `json:"divergence"`
	Significance     float64            `json:"significance"`
	TopValue         string             `json:"top_value"`
	TopValueLift     float64            `json:"top_value_lift"`
	AnomalousDistro  map[string]float64 `json:"anomalous_distro"`
	BaselineDistro   map[string]float64 `json:"baseline_distro"`
}

// AnalysisResult contains the full BubbleUp analysis
type AnalysisResult struct {
	ID             string              `json:"id"`
	CreatedAt      time.Time           `json:"created_at"`
	Service        string              `json:"service,omitempty"`
	Mode           string              `json:"mode"`
	TimeStart      time.Time           `json:"time_start"`
	TimeEnd        time.Time           `json:"time_end"`
	AnomalousCount int                 `json:"anomalous_count"`
	BaselineCount  int                 `json:"baseline_count"`
	Dimensions     []DimensionAnalysis `json:"dimensions"`
	Confidence     float64             `json:"confidence"`
	Summary        string              `json:"summary"`
}

