package promql

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Engine is the PromQL query engine.
type Engine struct {
	store MetricsStore
}

// NewEngine creates a new PromQL engine.
func NewEngine(store MetricsStore) *Engine {
	return &Engine{store: store}
}

// QueryResult is the result of a query.
type QueryResult struct {
	ResultType string
	Result     Value
}

// Query executes an instant query at a single point in time.
func (e *Engine) Query(ctx context.Context, query string, ts time.Time) (*QueryResult, error) {
	expr, err := Parse(query)
	if err != nil {
		return nil, err
	}

	evaluator := NewEvaluator(e.store)
	opts := &EvalOptions{
		Start: ts,
		End:   ts,
	}

	result, err := evaluator.Eval(ctx, expr, opts)
	if err != nil {
		return nil, err
	}

	return &QueryResult{
		ResultType: resultType(result),
		Result:     result,
	}, nil
}

// QueryRange executes a range query over a time range.
func (e *Engine) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) (*QueryResult, error) {
	expr, err := Parse(query)
	if err != nil {
		return nil, err
	}

	evaluator := NewEvaluator(e.store)

	// For range queries, we evaluate at each step and collect the results
	seriesMap := make(map[string]*Series)

	for t := start; !t.After(end); t = t.Add(step) {
		opts := &EvalOptions{
			Start:    start,
			End:      t,
			Interval: step,
		}

		result, err := evaluator.Eval(ctx, expr, opts)
		if err != nil {
			return nil, err
		}

		// Collect results into matrix
		switch v := result.(type) {
		case Scalar:
			key := ""
			series, ok := seriesMap[key]
			if !ok {
				series = &Series{Labels: map[string]string{}}
				seriesMap[key] = series
			}
			series.Samples = append(series.Samples, Sample{
				Timestamp: t,
				Value:     v.Val,
			})

		case Vector:
			for _, sample := range v {
				key := labelKey(sample.Labels)
				series, ok := seriesMap[key]
				if !ok {
					series = &Series{Labels: sample.Labels}
					seriesMap[key] = series
				}
				series.Samples = append(series.Samples, Sample{
					Timestamp: t,
					Value:     sample.Value,
				})
			}
		}
	}

	// Convert to matrix
	matrix := make(Matrix, 0, len(seriesMap))
	for _, series := range seriesMap {
		matrix = append(matrix, *series)
	}

	return &QueryResult{
		ResultType: "matrix",
		Result:     matrix,
	}, nil
}

// Series returns series matching the label selectors.
func (e *Engine) Series(ctx context.Context, matchers []string, start, end time.Time) ([]map[string]string, error) {
	var allMatchers []*LabelMatcher
	metricName := ""

	for _, matcherStr := range matchers {
		// Parse the matcher string: metric{label="value"}
		expr, err := Parse(matcherStr)
		if err != nil {
			continue
		}

		if vs, ok := expr.(*VectorSelector); ok {
			if metricName == "" {
				metricName = vs.Name
			}
			allMatchers = append(allMatchers, vs.LabelMatchers...)
		}
	}

	series, err := e.store.ScanRange(ctx, metricName, nil, allMatchers, start, end)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]string, len(series))
	for i, s := range series {
		result[i] = s.Labels
	}
	return result, nil
}

// LabelNames returns all label names.
func (e *Engine) LabelNames(ctx context.Context, matchers []string, start, end time.Time) ([]string, error) {
	metric := ""
	for _, m := range matchers {
		if expr, err := Parse(m); err == nil {
			if vs, ok := expr.(*VectorSelector); ok {
				metric = vs.Name
				break
			}
		}
	}
	return e.store.ListLabels(ctx, metric)
}

// LabelValues returns all values for a label.
func (e *Engine) LabelValues(ctx context.Context, label string, matchers []string, start, end time.Time) ([]string, error) {
	metric := ""
	for _, m := range matchers {
		if expr, err := Parse(m); err == nil {
			if vs, ok := expr.(*VectorSelector); ok {
				metric = vs.Name
				break
			}
		}
	}
	return e.store.ListLabelValues(ctx, label, metric)
}

// Metadata returns metric metadata.
func (e *Engine) Metadata(ctx context.Context, metric string, limit int) ([]MetricMetadata, error) {
	metrics, err := e.store.ListMetrics(ctx)
	if err != nil {
		return nil, err
	}

	var result []MetricMetadata
	for _, m := range metrics {
		if metric != "" && m != metric {
			continue
		}
		result = append(result, MetricMetadata{
			Metric: m,
			Type:   inferMetricType(m),
			Help:   "",
		})
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// MetricMetadata contains metadata about a metric.
type MetricMetadata struct {
	Metric string
	Type   string
	Help   string
	Unit   string
}

func inferMetricType(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, "_total") || strings.HasSuffix(lower, "_count") {
		return "counter"
	}
	if strings.HasSuffix(lower, "_bucket") {
		return "histogram"
	}
	if strings.HasSuffix(lower, "_sum") {
		return "counter"
	}
	return "gauge"
}

func resultType(v Value) string {
	switch v.(type) {
	case Scalar:
		return "scalar"
	case Vector:
		return "vector"
	case Matrix:
		return "matrix"
	case String:
		return "string"
	default:
		return "unknown"
	}
}

// PrometheusResponse is the Prometheus API response format.
type PrometheusResponse struct {
	Status    string      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType string      `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
}

// QueryData is the data portion of a query response.
type QueryData struct {
	ResultType string        `json:"resultType"`
	Result     []interface{} `json:"result"`
}

// FormatQueryResult formats a query result for the Prometheus API.
func FormatQueryResult(result *QueryResult) *PrometheusResponse {
	data := &QueryData{
		ResultType: result.ResultType,
		Result:     formatValue(result.Result),
	}

	return &PrometheusResponse{
		Status: "success",
		Data:   data,
	}
}

func formatValue(v Value) []interface{} {
	switch val := v.(type) {
	case Scalar:
		return []interface{}{
			[]interface{}{
				float64(val.Timestamp.Unix()),
				formatFloat(val.Val),
			},
		}

	case Vector:
		result := make([]interface{}, len(val))
		for i, sample := range val {
			result[i] = map[string]interface{}{
				"metric": sample.Labels,
				"value": []interface{}{
					float64(sample.Timestamp.Unix()),
					formatFloat(sample.Value),
				},
			}
		}
		return result

	case Matrix:
		result := make([]interface{}, len(val))
		for i, series := range val {
			values := make([][]interface{}, len(series.Samples))
			for j, sample := range series.Samples {
				values[j] = []interface{}{
					float64(sample.Timestamp.Unix()),
					formatFloat(sample.Value),
				}
			}
			result[i] = map[string]interface{}{
				"metric": series.Labels,
				"values": values,
			}
		}
		return result

	case String:
		return []interface{}{
			[]interface{}{
				float64(val.Timestamp.Unix()),
				val.Val,
			},
		}
	}

	return nil
}

func formatFloat(v float64) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	if math.IsInf(v, 1) {
		return "+Inf"
	}
	if math.IsInf(v, -1) {
		return "-Inf"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ParseTime parses a Prometheus time string.
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}

	// Try Unix timestamp (float)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(f)
		nsec := int64((f - float64(sec)) * 1e9)
		return time.Unix(sec, nsec), nil
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try RFC3339 without timezone
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}

	// Try relative time
	if strings.HasPrefix(s, "-") {
		d, err := ParseDuration(s[1:])
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().Add(-d), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

// ParseDurationParam parses a step/duration parameter.
func ParseDurationParam(s string, defaultVal time.Duration) (time.Duration, error) {
	if s == "" {
		return defaultVal, nil
	}

	// Try as float seconds
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second)), nil
	}

	// Try as duration string
	return ParseDuration(s)
}

// ErrorResponse creates an error response.
func ErrorResponse(errorType, msg string) *PrometheusResponse {
	return &PrometheusResponse{
		Status:    "error",
		ErrorType: errorType,
		Error:     msg,
	}
}
