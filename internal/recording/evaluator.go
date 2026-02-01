package recording

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/query"
)

// Evaluator executes recording rule expressions
type Evaluator struct {
	executor     *query.Executor
	metricsStore *custommetrics.Store
}

// EvaluationResult holds the result of evaluating a recording rule
type EvaluationResult struct {
	Values   []ResultValue
	Duration time.Duration
	Error    error
}

// ResultValue represents a single value from evaluation (may have labels)
type ResultValue struct {
	Value  float64
	Labels map[string]string
}

// NewEvaluator creates a new recording rule evaluator
func NewEvaluator(executor *query.Executor, metricsStore *custommetrics.Store) *Evaluator {
	return &Evaluator{
		executor:     executor,
		metricsStore: metricsStore,
	}
}

// Evaluate executes a recording rule expression and returns the result
func (e *Evaluator) Evaluate(ctx context.Context, rule *RecordingRule) *EvaluationResult {
	start := time.Now()

	result := &EvaluationResult{}

	// Parse and execute the expression
	values, err := e.executeExpression(ctx, rule)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	result.Values = values
	result.Duration = time.Since(start)

	return result
}

// EvaluateAndStore evaluates a rule and stores the result as a new metric
func (e *Evaluator) EvaluateAndStore(ctx context.Context, rule *RecordingRule) *EvaluationResult {
	result := e.Evaluate(ctx, rule)

	if result.Error != nil {
		return result
	}

	// Store each value as a metric data point
	for _, rv := range result.Values {
		// Merge rule labels with result labels
		tags := make(map[string]string)
		for k, v := range rule.Labels {
			tags[k] = v
		}
		for k, v := range rv.Labels {
			// Don't overwrite __name__ from rule labels
			if k != "__name__" {
				tags[k] = v
			}
		}

		dp := custommetrics.DataPoint{
			Timestamp: time.Now(),
			Name:      rule.Name,
			Type:      custommetrics.Gauge,
			Value:     rv.Value,
			Tags:      tags,
		}

		if err := e.metricsStore.Record(dp); err != nil {
			log.Printf("[recording] Error storing metric %s: %v", rule.Name, err)
		}
	}

	return result
}

// executeExpression parses and executes the rule expression
func (e *Evaluator) executeExpression(ctx context.Context, rule *RecordingRule) ([]ResultValue, error) {
	expr := rule.Expression

	// Handle PromQL-style expressions
	if isPromQLStyle(expr) {
		return e.executePromQL(ctx, expr)
	}

	// Handle SQL/DQL-style expressions
	return e.executeDQL(ctx, expr)
}

// executeDQL executes a DQL/SQL-style expression
func (e *Evaluator) executeDQL(ctx context.Context, expr string) ([]ResultValue, error) {
	if e.executor == nil {
		return nil, fmt.Errorf("query executor not configured")
	}

	// Replace now() with actual timestamp
	expr = strings.ReplaceAll(expr, "now()", fmt.Sprintf("'%s'", time.Now().UTC().Format(time.RFC3339)))

	// Parse relative time expressions like "now() - 1m"
	expr = expandTimeExpressions(expr)

	result, err := e.executor.Execute(ctx, expr)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}

	var values []ResultValue
	for _, row := range result.Rows {
		rv := ResultValue{
			Labels: make(map[string]string),
		}

		// Extract value
		if v, ok := row["value"]; ok {
			rv.Value = toFloat(v)
		} else {
			// Find first numeric column
			for k, v := range row {
				if f := toFloat(v); f != 0 || k == "value" || k == "count" || k == "sum" || k == "avg" {
					rv.Value = f
					break
				}
			}
		}

		// Extract labels from other columns
		for k, v := range row {
			if k == "value" || k == "count" || k == "sum" || k == "avg" || k == "min" || k == "max" {
				continue
			}
			if k == "timestamp" || k == "window_start" || k == "window_end" {
				continue
			}
			if s, ok := v.(string); ok && s != "" {
				rv.Labels[k] = s
			}
		}

		values = append(values, rv)
	}

	return values, nil
}

// executePromQL executes a PromQL-style expression
func (e *Evaluator) executePromQL(ctx context.Context, expr string) ([]ResultValue, error) {
	// Parse common PromQL patterns and convert to DQL

	// rate(metric[5m])
	if strings.HasPrefix(expr, "rate(") {
		return e.executeRate(ctx, expr)
	}

	// sum(metric) by (label)
	if strings.HasPrefix(expr, "sum(") {
		return e.executeSum(ctx, expr)
	}

	// avg(metric) by (label)
	if strings.HasPrefix(expr, "avg(") {
		return e.executeAvg(ctx, expr)
	}

	// count(metric) by (label)
	if strings.HasPrefix(expr, "count(") {
		return e.executeCount(ctx, expr)
	}

	// histogram_quantile(0.99, metric)
	if strings.HasPrefix(expr, "histogram_quantile(") {
		return e.executeQuantile(ctx, expr)
	}

	// Fallback: try as DQL
	return e.executeDQL(ctx, expr)
}

// executeRate handles rate() function
func (e *Evaluator) executeRate(ctx context.Context, expr string) ([]ResultValue, error) {
	// Parse rate(metric[duration])
	re := regexp.MustCompile(`rate\((\w+)\[(\d+[smhd])\]\)(?:\s+by\s+\(([^)]+)\))?`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid rate expression: %s", expr)
	}

	metric := matches[1]
	duration := matches[2]
	groupBy := ""
	if len(matches) > 3 {
		groupBy = matches[3]
	}

	// Convert to DQL
	dql := fmt.Sprintf("SELECT ")
	if groupBy != "" {
		dql += groupBy + ", "
	}
	dql += fmt.Sprintf("sum(value) / %s as value FROM metrics WHERE name = '%s' AND timestamp >= now() - %s",
		durationToSeconds(duration), metric, duration)
	if groupBy != "" {
		dql += " GROUP BY " + groupBy
	}

	return e.executeDQL(ctx, dql)
}

// executeSum handles sum() function
func (e *Evaluator) executeSum(ctx context.Context, expr string) ([]ResultValue, error) {
	re := regexp.MustCompile(`sum\((\w+)\)(?:\s+by\s+\(([^)]+)\))?`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid sum expression: %s", expr)
	}

	metric := matches[1]
	groupBy := ""
	if len(matches) > 2 {
		groupBy = matches[2]
	}

	dql := "SELECT "
	if groupBy != "" {
		dql += groupBy + ", "
	}
	dql += fmt.Sprintf("sum(value) as value FROM metrics WHERE name = '%s' AND timestamp >= now() - 5m", metric)
	if groupBy != "" {
		dql += " GROUP BY " + groupBy
	}

	return e.executeDQL(ctx, dql)
}

// executeAvg handles avg() function
func (e *Evaluator) executeAvg(ctx context.Context, expr string) ([]ResultValue, error) {
	re := regexp.MustCompile(`avg\((\w+)\)(?:\s+by\s+\(([^)]+)\))?`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid avg expression: %s", expr)
	}

	metric := matches[1]
	groupBy := ""
	if len(matches) > 2 {
		groupBy = matches[2]
	}

	dql := "SELECT "
	if groupBy != "" {
		dql += groupBy + ", "
	}
	dql += fmt.Sprintf("avg(value) as value FROM metrics WHERE name = '%s' AND timestamp >= now() - 5m", metric)
	if groupBy != "" {
		dql += " GROUP BY " + groupBy
	}

	return e.executeDQL(ctx, dql)
}

// executeCount handles count() function
func (e *Evaluator) executeCount(ctx context.Context, expr string) ([]ResultValue, error) {
	re := regexp.MustCompile(`count\((\w+)\)(?:\s+by\s+\(([^)]+)\))?`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid count expression: %s", expr)
	}

	metric := matches[1]
	groupBy := ""
	if len(matches) > 2 {
		groupBy = matches[2]
	}

	dql := "SELECT "
	if groupBy != "" {
		dql += groupBy + ", "
	}
	dql += fmt.Sprintf("count(*) as value FROM metrics WHERE name = '%s' AND timestamp >= now() - 5m", metric)
	if groupBy != "" {
		dql += " GROUP BY " + groupBy
	}

	return e.executeDQL(ctx, dql)
}

// executeQuantile handles histogram_quantile() function
func (e *Evaluator) executeQuantile(ctx context.Context, expr string) ([]ResultValue, error) {
	re := regexp.MustCompile(`histogram_quantile\(([\d.]+),\s*(\w+)\)(?:\s+by\s+\(([^)]+)\))?`)
	matches := re.FindStringSubmatch(expr)

	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid histogram_quantile expression: %s", expr)
	}

	quantile, _ := strconv.ParseFloat(matches[1], 64)
	metric := matches[2]
	groupBy := ""
	if len(matches) > 3 {
		groupBy = matches[3]
	}

	// Map quantile to percentile function
	pct := int(quantile * 100)
	pctFunc := fmt.Sprintf("p%d", pct)
	if pct == 50 || pct == 90 || pct == 95 || pct == 99 {
		// Use built-in percentile functions
	} else {
		// Fallback to p99 for unsupported percentiles
		pctFunc = "p99"
	}

	dql := "SELECT "
	if groupBy != "" {
		dql += groupBy + ", "
	}
	dql += fmt.Sprintf("%s(value) as value FROM metrics WHERE name = '%s' AND timestamp >= now() - 5m", pctFunc, metric)
	if groupBy != "" {
		dql += " GROUP BY " + groupBy
	}

	return e.executeDQL(ctx, dql)
}

// isPromQLStyle checks if an expression looks like PromQL
func isPromQLStyle(expr string) bool {
	promqlPatterns := []string{
		`^rate\(`,
		`^sum\(`,
		`^avg\(`,
		`^count\(`,
		`^min\(`,
		`^max\(`,
		`^histogram_quantile\(`,
		`^increase\(`,
		`^irate\(`,
	}

	for _, pattern := range promqlPatterns {
		if matched, _ := regexp.MatchString(pattern, expr); matched {
			return true
		}
	}

	return false
}

// expandTimeExpressions expands relative time expressions
func expandTimeExpressions(expr string) string {
	// Replace patterns like "now() - 1m" with actual timestamps
	re := regexp.MustCompile(`'[^']+'\s*-\s*(\d+)([smhd])`)
	expr = re.ReplaceAllStringFunc(expr, func(match string) string {
		subMatches := re.FindStringSubmatch(match)
		if len(subMatches) < 3 {
			return match
		}
		n, _ := strconv.Atoi(subMatches[1])
		unit := subMatches[2]

		var d time.Duration
		switch unit {
		case "s":
			d = time.Duration(n) * time.Second
		case "m":
			d = time.Duration(n) * time.Minute
		case "h":
			d = time.Duration(n) * time.Hour
		case "d":
			d = time.Duration(n) * 24 * time.Hour
		}

		return fmt.Sprintf("'%s'", time.Now().Add(-d).UTC().Format(time.RFC3339))
	})

	return expr
}

// durationToSeconds converts a duration string to seconds
func durationToSeconds(d string) string {
	re := regexp.MustCompile(`(\d+)([smhd])`)
	matches := re.FindStringSubmatch(d)
	if len(matches) < 3 {
		return "60" // default to 60 seconds
	}

	n, _ := strconv.Atoi(matches[1])
	unit := matches[2]

	var seconds int
	switch unit {
	case "s":
		seconds = n
	case "m":
		seconds = n * 60
	case "h":
		seconds = n * 3600
	case "d":
		seconds = n * 86400
	}

	return strconv.Itoa(seconds)
}

// toFloat converts an interface to float64
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}
