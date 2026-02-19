package bubbleup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dogwatch/internal/trace"

	"github.com/google/uuid"
)

// Analyzer performs BubbleUp analysis on trace data
type Analyzer struct {
	traceStore *trace.Store

	// Store recent results
	results   map[string]*AnalysisResult
	resultsMu sync.RWMutex
	maxResults int
}

func NewAnalyzer(traceStore *trace.Store) *Analyzer {
	return &Analyzer{
		traceStore: traceStore,
		results:    make(map[string]*AnalysisResult),
		maxResults: 100,
	}
}

// SetTraceStore sets or updates the trace store
func (a *Analyzer) SetTraceStore(ts *trace.Store) {
	a.traceStore = ts
}

// Analyze runs BubbleUp analysis on the given request
func (a *Analyzer) Analyze(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error) {
	if a.traceStore == nil {
		return nil, fmt.Errorf("trace store not configured")
	}

	// Validate request
	if req.TimeStart.IsZero() || req.TimeEnd.IsZero() {
		return nil, fmt.Errorf("time_start and time_end are required")
	}
	if req.Mode == "" {
		req.Mode = "latency"
	}
	if req.Mode != "latency" && req.Mode != "errors" {
		return nil, fmt.Errorf("mode must be 'latency' or 'errors'")
	}

	// Get anomalous spans
	var anomalousSpans []trace.Span
	var baselineSpans []trace.Span
	var err error

	if len(req.AnomalousSpanIDs) > 0 {
		// Direct span selection
		anomalousSpans, err = a.traceStore.GetSpansByIDs(ctx, req.AnomalousSpanIDs)
		if err != nil {
			return nil, fmt.Errorf("fetching anomalous spans: %w", err)
		}
		if len(req.BaselineSpanIDs) > 0 {
			baselineSpans, err = a.traceStore.GetSpansByIDs(ctx, req.BaselineSpanIDs)
			if err != nil {
				return nil, fmt.Errorf("fetching baseline spans: %w", err)
			}
		}
	} else {
		// Filter-based selection
		if req.Mode == "latency" {
			anomalousSpans, baselineSpans, err = a.selectByLatency(ctx, req)
		} else {
			anomalousSpans, baselineSpans, err = a.selectByErrors(ctx, req)
		}
		if err != nil {
			return nil, err
		}
	}

	if len(anomalousSpans) == 0 {
		return nil, fmt.Errorf("no anomalous spans found matching criteria")
	}
	if len(baselineSpans) == 0 {
		return nil, fmt.Errorf("no baseline spans found for comparison")
	}

	// Extract dimensions from both sets
	anomalousDims := extractDimensions(anomalousSpans)
	baselineDims := extractDimensions(baselineSpans)

	// Compare each dimension
	var dimensions []DimensionAnalysis
	for dimName := range anomalousDims {
		analysis := compareDimension(
			dimName,
			anomalousDims[dimName],
			baselineDims[dimName],
			len(anomalousSpans),
			len(baselineSpans),
		)
		// Only include significant dimensions
		if analysis.Divergence > 5.0 && analysis.TopValueLift > 1.5 {
			dimensions = append(dimensions, analysis)
		}
	}

	// Sort by divergence (highest first)
	sort.Slice(dimensions, func(i, j int) bool {
		return dimensions[i].Divergence > dimensions[j].Divergence
	})

	// Limit to top 20 dimensions
	if len(dimensions) > 20 {
		dimensions = dimensions[:20]
	}

	// Calculate overall confidence
	confidence := calculateConfidence(dimensions, len(anomalousSpans), len(baselineSpans))

	// Generate summary
	summary := generateSummary(dimensions, req.Mode)

	result := &AnalysisResult{
		ID:             uuid.New().String()[:8],
		CreatedAt:      time.Now().UTC(),
		Service:        req.Service,
		Mode:           req.Mode,
		TimeStart:      req.TimeStart,
		TimeEnd:        req.TimeEnd,
		AnomalousCount: len(anomalousSpans),
		BaselineCount:  len(baselineSpans),
		Dimensions:     dimensions,
		Confidence:     confidence,
		Summary:        summary,
	}

	// Store result
	a.resultsMu.Lock()
	a.results[result.ID] = result
	// Cleanup old results
	if len(a.results) > a.maxResults {
		var oldest string
		var oldestTime time.Time
		for id, r := range a.results {
			if oldest == "" || r.CreatedAt.Before(oldestTime) {
				oldest = id
				oldestTime = r.CreatedAt
			}
		}
		delete(a.results, oldest)
	}
	a.resultsMu.Unlock()

	return result, nil
}

// GetResult retrieves a previous analysis result
func (a *Analyzer) GetResult(id string) (*AnalysisResult, bool) {
	a.resultsMu.RLock()
	defer a.resultsMu.RUnlock()
	r, ok := a.results[id]
	return r, ok
}

func (a *Analyzer) ListResults(limit int) []*AnalysisResult {
	a.resultsMu.RLock()
	defer a.resultsMu.RUnlock()

	results := make([]*AnalysisResult, 0, len(a.results))
	for _, r := range a.results {
		results = append(results, r)
	}

	// Sort by creation time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func (a *Analyzer) selectByLatency(ctx context.Context, req AnalysisRequest) (anomalous, baseline []trace.Span, err error) {
	threshold := req.LatencyThreshold

	// If no threshold, calculate from percentile
	if threshold == 0 {
		percentile := req.LatencyPercentile
		if percentile == 0 {
			percentile = 99
		}
		threshold, err = a.traceStore.GetLatencyPercentile(ctx, req.Service, percentile, req.TimeStart, req.TimeEnd)
		if err != nil {
			return nil, nil, fmt.Errorf("calculating latency percentile: %w", err)
		}
	}

	// Get slow spans (anomalous)
	anomalous, err = a.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
		Service:       req.Service,
		TimeStart:     req.TimeStart,
		TimeEnd:       req.TimeEnd,
		MinDurationMs: threshold,
		Limit:         10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("querying slow spans: %w", err)
	}

	// Get normal spans (baseline) - spans below threshold
	baseline, err = a.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
		Service:       req.Service,
		TimeStart:     req.TimeStart,
		TimeEnd:       req.TimeEnd,
		MaxDurationMs: threshold,
		Limit:         10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("querying baseline spans: %w", err)
	}

	return anomalous, baseline, nil
}

func (a *Analyzer) selectByErrors(ctx context.Context, req AnalysisRequest) (anomalous, baseline []trace.Span, err error) {
	// Get error spans
	anomalous, err = a.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
		Service:   req.Service,
		TimeStart: req.TimeStart,
		TimeEnd:   req.TimeEnd,
		Status:    "ERROR",
		Limit:     10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("querying error spans: %w", err)
	}

	// Get successful spans (baseline)
	baseline, err = a.traceStore.QuerySpans(ctx, trace.SpanQueryOptions{
		Service:   req.Service,
		TimeStart: req.TimeStart,
		TimeEnd:   req.TimeEnd,
		Status:    "OK",
		Limit:     10000,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("querying baseline spans: %w", err)
	}

	return anomalous, baseline, nil
}

// extractDimensions collects all attribute values from spans
func extractDimensions(spans []trace.Span) map[string]map[string]int {
	dims := make(map[string]map[string]int)

	for _, span := range spans {
		// Standard span fields as dimensions
		addDimension(dims, "service", span.ServiceName)
		addDimension(dims, "operation", span.Name)
		addDimension(dims, "kind", span.Kind)
		addDimension(dims, "status", span.Status)

		// All attributes as dimensions
		for k, v := range span.Attributes {
			if v != "" {
				addDimension(dims, k, v)
			}
		}
	}

	return dims
}

func addDimension(dims map[string]map[string]int, dimension, value string) {
	if value == "" {
		return
	}
	if dims[dimension] == nil {
		dims[dimension] = make(map[string]int)
	}
	dims[dimension][value]++
}

// compareDimension analyzes how a dimension differs between sets
func compareDimension(name string, anomalous, baseline map[string]int, anomalousTotal, baselineTotal int) DimensionAnalysis {
	if anomalous == nil {
		anomalous = make(map[string]int)
	}
	if baseline == nil {
		baseline = make(map[string]int)
	}

	chi2 := ChiSquaredFromDistributions(anomalous, baseline, anomalousTotal, baselineTotal)
	df := len(anomalous) + len(baseline) - 1
	if df < 1 {
		df = 1
	}
	pValue := ChiSquaredPValue(chi2, df)

	topValue, topLift := FindTopValue(anomalous, baseline, anomalousTotal, baselineTotal)

	return DimensionAnalysis{
		Dimension:        name,
		Divergence:       chi2,
		Significance:     pValue,
		TopValue:         topValue,
		TopValueLift:     topLift,
		AnomalousDistro:  DistributionToRates(anomalous, anomalousTotal),
		BaselineDistro:   DistributionToRates(baseline, baselineTotal),
	}
}

func calculateConfidence(dimensions []DimensionAnalysis, anomalousCount, baselineCount int) float64 {
	if len(dimensions) == 0 {
		return 0
	}

	// Base confidence on sample sizes
	sampleConfidence := 1.0
	if anomalousCount < 30 {
		sampleConfidence = float64(anomalousCount) / 30.0
	}
	if baselineCount < 100 {
		sampleConfidence *= float64(baselineCount) / 100.0
	}

	// Boost based on top dimension's significance
	if dimensions[0].Significance < 0.01 {
		sampleConfidence *= 1.2
	}
	if dimensions[0].TopValueLift > 5 {
		sampleConfidence *= 1.1
	}

	if sampleConfidence > 1.0 {
		sampleConfidence = 1.0
	}
	return sampleConfidence
}

func generateSummary(dimensions []DimensionAnalysis, mode string) string {
	if len(dimensions) == 0 {
		return "No significant dimensional differences found."
	}

	top := dimensions[0]
	anomalousRate := top.AnomalousDistro[top.TopValue] * 100
	baselineRate := top.BaselineDistro[top.TopValue] * 100

	eventType := "slow requests"
	if mode == "errors" {
		eventType = "errors"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(
		"%.0f%% of %s have %s=%s (vs %.0f%% baseline). %.1fx more likely.",
		anomalousRate, eventType, top.Dimension, top.TopValue, baselineRate, top.TopValueLift,
	))

	// Add secondary factors
	if len(dimensions) > 1 {
		secondary := dimensions[1]
		if secondary.TopValueLift > 2 {
			parts = append(parts, fmt.Sprintf(
				"Also: %s=%s (%.1fx lift).",
				secondary.Dimension, secondary.TopValue, secondary.TopValueLift,
			))
		}
	}

	return strings.Join(parts, " ")
}
