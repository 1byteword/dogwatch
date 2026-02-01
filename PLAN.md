# BubbleUp Implementation Plan

## Overview

BubbleUp automatically identifies root causes by statistically comparing "bad" events (slow requests, errors) against "good" events to surface which dimensions are disproportionately represented.

**Example output:** "98% of slow requests hit shard 7, but only 8% of normal requests do"

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      BubbleUp Flow                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. Event Selection                                         │
│     ├── Manual: User selects slow/erroring spans            │
│     └── Auto: Triggered by anomaly detection                │
│                                                             │
│  2. Data Extraction                                         │
│     ├── Query spans matching criteria (latency > p99, etc.) │
│     └── Query baseline spans (same time window, normal)     │
│                                                             │
│  3. Statistical Analysis                                    │
│     ├── Extract all dimensions from both sets               │
│     ├── Calculate distribution per dimension                │
│     ├── Compute divergence (chi-squared)                    │
│     └── Calculate lift for top values                       │
│                                                             │
│  4. Results                                                 │
│     ├── Rank dimensions by divergence score                 │
│     ├── Filter by significance threshold                    │
│     └── Return top contributing dimensions                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Implementation Tasks

### Task 1: Core Algorithm (`internal/bubbleup/`)

Create new package with:

**bubbleup.go** - Core types and analysis
```go
type AnalysisRequest struct {
    AnomalousSpanIDs []string      // Specific spans to analyze
    BaselineSpanIDs  []string      // Comparison spans (optional, auto-select if empty)

    // OR filter-based selection:
    Service          string        // Filter by service
    TimeStart        time.Time     // Time window start
    TimeEnd          time.Time     // Time window end
    LatencyThreshold float64       // e.g., p99 threshold in ms
    ErrorsOnly       bool          // Only analyze error spans
}

type DimensionAnalysis struct {
    Dimension        string             // e.g., "db_shard", "region", "http.method"
    Divergence       float64            // Chi-squared statistic
    Significance     float64            // p-value (lower = more significant)
    TopValue         string             // Most overrepresented value
    TopValueLift     float64            // How much more likely in anomalous set
    AnomalousDistro  map[string]float64 // value -> percentage
    BaselineDistro   map[string]float64 // value -> percentage
}

type AnalysisResult struct {
    ID               string              // Unique analysis ID
    CreatedAt        time.Time
    AnomalousCount   int                 // Number of "bad" spans
    BaselineCount    int                 // Number of "good" spans
    Dimensions       []DimensionAnalysis // Ranked by divergence
    Confidence       float64             // Overall confidence score
    Summary          string              // Human-readable summary
}
```

**stats.go** - Statistical functions
- `ChiSquared(observed, expected []float64) float64`
- `KLDivergence(p, q []float64) float64` (alternative)
- `CalculateLift(anomalousRate, baselineRate float64) float64`
- `Significance(chiSquared float64, degreesOfFreedom int) float64`

**analyzer.go** - Main analysis logic
```go
type Analyzer struct {
    traceStore *trace.Store
}

func (a *Analyzer) Analyze(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error)
func (a *Analyzer) extractDimensions(spans []trace.Span) map[string]map[string]int
func (a *Analyzer) compareDistributions(anomalous, baseline map[string]int) DimensionAnalysis
```

### Task 2: Trace Store Query Extension

Add to `internal/trace/trace.go`:

```go
// QuerySpans returns spans matching the given criteria
func (s *Store) QuerySpans(ctx context.Context, opts SpanQueryOptions) ([]Span, error)

type SpanQueryOptions struct {
    Service          string
    TimeStart        time.Time
    TimeEnd          time.Time
    MinDurationMs    float64       // For slow span selection
    MaxDurationMs    float64       // For baseline selection
    Status           string        // "ERROR", "OK", or "" for all
    Limit            int
}

// GetLatencyPercentile returns the p99/p95/etc latency for a service
func (s *Store) GetLatencyPercentile(ctx context.Context, service string, percentile float64, since time.Duration) (float64, error)
```

### Task 3: API Endpoints

Add to `internal/web/server.go`:

```
POST /api/bubbleup/analyze    - Run BubbleUp analysis
GET  /api/bubbleup/results    - List recent analyses
GET  /api/bubbleup/{id}       - Get specific analysis result
```

Request body for analyze:
```json
{
  "service": "api-gateway",
  "time_start": "2024-01-15T10:00:00Z",
  "time_end": "2024-01-15T11:00:00Z",
  "mode": "latency",           // "latency" or "errors"
  "latency_percentile": 99,    // For latency mode
  "latency_threshold_ms": 500  // OR explicit threshold
}
```

Response:
```json
{
  "id": "bb-12345",
  "anomalous_count": 847,
  "baseline_count": 12432,
  "confidence": 0.982,
  "dimensions": [
    {
      "dimension": "db_shard",
      "divergence": 245.7,
      "top_value": "7",
      "top_value_lift": 12.25,
      "anomalous_distro": {"7": 0.98, "1": 0.01, "2": 0.01},
      "baseline_distro": {"7": 0.08, "1": 0.23, "2": 0.22, ...}
    }
  ],
  "summary": "98% of slow requests hit db_shard=7 (vs 8% baseline). 12.25x more likely."
}
```

### Task 4: Frontend Components

Add to `internal/web/static/`:

1. **BubbleUp Analysis Page** (`bubbleup.html`)
   - Service selector dropdown
   - Time range picker
   - Mode toggle (latency vs errors)
   - "Analyze" button
   - Results display with dimension cards

2. **Results Component**
   - Ranked list of dimensions
   - Bar charts showing distribution comparison
   - Lift indicators with color coding
   - Links to drill into traces

3. **Integration Points**
   - Add "BubbleUp" button on trace list page
   - Add "Analyze" action on anomaly alerts
   - Show BubbleUp results in incident detail

## File Changes Summary

| File | Change |
|------|--------|
| `internal/bubbleup/bubbleup.go` | **NEW** - Core types |
| `internal/bubbleup/stats.go` | **NEW** - Statistical functions |
| `internal/bubbleup/analyzer.go` | **NEW** - Analysis logic |
| `internal/trace/trace.go` | Add `QuerySpans`, `GetLatencyPercentile` |
| `internal/web/server.go` | Add BubbleUp handlers |
| `internal/web/static/bubbleup.html` | **NEW** - Analysis page |
| `internal/web/static/index.html` | Add nav link to BubbleUp |
| `cmd/dogwatch/main.go` | Wire up BubbleUp analyzer |

## Implementation Order

1. **Core algorithm** (Task 1) - Get the statistics working
2. **Trace query extension** (Task 2) - Enable filtering spans
3. **API endpoints** (Task 3) - Expose via HTTP
4. **Frontend** (Task 4) - Build the UI

## Testing Strategy

1. Unit tests for statistical functions (chi-squared, lift calculation)
2. Integration test with synthetic trace data
3. Test case from VISION.md: "98% of slow requests hit shard 7"

## Scope Notes

- MVP focuses on trace spans (not metrics or logs)
- No saved analyses in MVP (add later for Team tier)
- No automatic triggering from anomaly detection in MVP (add later)
- Chi-squared over KL divergence (simpler, well-understood)
