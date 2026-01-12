package query

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Row represents a single result row
type Row map[string]interface{}

// Result represents query execution results
type Result struct {
	Columns []string
	Rows    []Row
	Stats   ExecutionStats
}

// ExecutionStats contains execution statistics
type ExecutionStats struct {
	RowsScanned    int64
	RowsReturned   int64
	ExecutionTime  time.Duration
	BytesProcessed int64
}

// DataSource interface for pluggable data backends
type DataSource interface {
	// Scan reads data from the source
	Scan(ctx context.Context, source string, metric string, timeRange TimeRangeSpec, predicates []Expr) ([]Row, error)
}

// Executor executes query plans
type Executor struct {
	metrics DataSource
	logs    DataSource
	traces  DataSource
	events  DataSource

	// Function registry
	functions map[string]Function
}

// Function represents a built-in function
type Function func(args ...interface{}) (interface{}, error)

// NewExecutor creates a new query executor
func NewExecutor() *Executor {
	e := &Executor{
		functions: make(map[string]Function),
	}
	e.registerBuiltins()
	return e
}

// SetMetricsSource sets the metrics data source
func (e *Executor) SetMetricsSource(ds DataSource) {
	e.metrics = ds
}

// SetLogsSource sets the logs data source
func (e *Executor) SetLogsSource(ds DataSource) {
	e.logs = ds
}

// SetTracesSource sets the traces data source
func (e *Executor) SetTracesSource(ds DataSource) {
	e.traces = ds
}

// SetEventsSource sets the events data source
func (e *Executor) SetEventsSource(ds DataSource) {
	e.events = ds
}

// Execute runs a query and returns results
func (e *Executor) Execute(ctx context.Context, query string) (*Result, error) {
	startTime := time.Now()

	// Parse query
	ast, err := Parse(query)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Create plan
	planner := NewPlanner()
	plan, err := planner.Plan(ast)
	if err != nil {
		return nil, fmt.Errorf("planning error: %w", err)
	}

	// Execute plan
	rows, err := e.executePlan(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("execution error: %w", err)
	}

	// Build result
	result := &Result{
		Rows: rows,
		Stats: ExecutionStats{
			RowsReturned:  int64(len(rows)),
			ExecutionTime: time.Since(startTime),
		},
	}

	// Extract columns from first row
	if len(rows) > 0 {
		for col := range rows[0] {
			result.Columns = append(result.Columns, col)
		}
		sort.Strings(result.Columns)
	}

	return result, nil
}

// ExecutePlan executes a pre-built plan
func (e *Executor) ExecutePlan(ctx context.Context, plan *Plan) (*Result, error) {
	startTime := time.Now()

	rows, err := e.executePlan(ctx, plan)
	if err != nil {
		return nil, err
	}

	result := &Result{
		Rows: rows,
		Stats: ExecutionStats{
			RowsReturned:  int64(len(rows)),
			ExecutionTime: time.Since(startTime),
		},
	}

	if len(rows) > 0 {
		for col := range rows[0] {
			result.Columns = append(result.Columns, col)
		}
		sort.Strings(result.Columns)
	}

	return result, nil
}

func (e *Executor) executePlan(ctx context.Context, plan *Plan) ([]Row, error) {
	return e.executeNode(ctx, plan.Root, plan.TimeRange)
}

func (e *Executor) executeNode(ctx context.Context, node PlanNode, timeRange TimeRangeSpec) ([]Row, error) {
	switch n := node.(type) {
	case *PlanScanNode:
		return e.executeScan(ctx, n, timeRange)
	case *PlanFilterNode:
		return e.executeFilter(ctx, n, timeRange)
	case *PlanProjectNode:
		return e.executeProject(ctx, n, timeRange)
	case *PlanAggregateNode:
		return e.executeAggregate(ctx, n, timeRange)
	case *PlanSortNode:
		return e.executeSort(ctx, n, timeRange)
	case *PlanLimitNode:
		return e.executeLimit(ctx, n, timeRange)
	case *PlanWindowNode:
		return e.executeWindow(ctx, n, timeRange)
	case *PlanJoinNode:
		return e.executeJoin(ctx, n, timeRange)
	case *PlanAnomalyNode:
		return e.executeAnomaly(ctx, n, timeRange)
	case *PlanExtractNode:
		return e.executeExtract(ctx, n, timeRange)
	case *PlanHistogramNode:
		return e.executeHistogram(ctx, n, timeRange)
	default:
		return nil, fmt.Errorf("unsupported plan node: %T", node)
	}
}

func (e *Executor) executeScan(ctx context.Context, node *PlanScanNode, timeRange TimeRangeSpec) ([]Row, error) {
	var ds DataSource
	switch node.Source {
	case "metrics":
		ds = e.metrics
	case "logs":
		ds = e.logs
	case "traces":
		ds = e.traces
	case "events":
		ds = e.events
	default:
		return nil, fmt.Errorf("unknown source: %s", node.Source)
	}

	if ds == nil {
		return nil, fmt.Errorf("no data source configured for %s", node.Source)
	}

	return ds.Scan(ctx, node.Source, node.MetricName, timeRange, node.Predicates)
}

func (e *Executor) executeFilter(ctx context.Context, node *PlanFilterNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	var result []Row
	for _, row := range input {
		match, err := e.evalExpr(node.Condition, row)
		if err != nil {
			return nil, err
		}
		if toBool(match) {
			result = append(result, row)
		}
	}

	return result, nil
}

func (e *Executor) executeProject(ctx context.Context, node *PlanProjectNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	var result []Row
	for _, row := range input {
		newRow := make(Row)
		for _, field := range node.Fields {
			val, err := e.evalExpr(field.Expr, row)
			if err != nil {
				return nil, err
			}
			name := field.Alias
			if name == "" {
				name = field.Expr.String()
			}
			newRow[name] = val
		}
		result = append(result, newRow)
	}

	return result, nil
}

func (e *Executor) executeAggregate(ctx context.Context, node *PlanAggregateNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	// Group rows
	groups := make(map[string][]Row)
	for _, row := range input {
		key := e.groupKey(row, node.GroupBy)
		groups[key] = append(groups[key], row)
	}

	// Compute aggregations for each group
	var result []Row
	for _, groupRows := range groups {
		newRow := make(Row)

		// Add group by fields
		if len(groupRows) > 0 && len(node.GroupBy) > 0 {
			for _, field := range node.GroupBy {
				newRow[field] = groupRows[0][field]
			}
		}

		// Compute aggregations
		for _, agg := range node.Aggregations {
			val := e.computeAggregation(agg, groupRows)
			name := agg.Alias
			if name == "" {
				name = agg.Function
				if agg.Field != "" {
					name += "_" + agg.Field
				}
			}
			newRow[name] = val
		}

		result = append(result, newRow)
	}

	return result, nil
}

func (e *Executor) groupKey(row Row, fields []string) string {
	if len(fields) == 0 {
		return "__all__"
	}
	var parts []string
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%v", row[f]))
	}
	return strings.Join(parts, "|")
}

func (e *Executor) computeAggregation(agg AggSpec, rows []Row) interface{} {
	var values []float64
	for _, row := range rows {
		if agg.Field == "" {
			values = append(values, 1) // count without field
		} else if v, ok := row[agg.Field]; ok {
			values = append(values, toFloat(v))
		}
	}

	if len(values) == 0 {
		return nil
	}

	switch agg.Function {
	case "count":
		return len(values)
	case "sum":
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum
	case "avg":
		var sum float64
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values))
	case "min":
		min := values[0]
		for _, v := range values[1:] {
			if v < min {
				min = v
			}
		}
		return min
	case "max":
		max := values[0]
		for _, v := range values[1:] {
			if v > max {
				max = v
			}
		}
		return max
	case "p50":
		return percentile(values, 50)
	case "p90":
		return percentile(values, 90)
	case "p95":
		return percentile(values, 95)
	case "p99":
		return percentile(values, 99)
	default:
		return nil
	}
}

func percentile(values []float64, p int) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	idx := float64(len(sorted)-1) * float64(p) / 100.0
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func (e *Executor) executeSort(ctx context.Context, node *PlanSortNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	// Sort rows
	sort.Slice(input, func(i, j int) bool {
		for _, field := range node.Fields {
			vi := input[i][field.Field]
			vj := input[j][field.Field]
			cmp := compare(vi, vj)
			if cmp != 0 {
				if field.Desc {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return false
	})

	return input, nil
}

func compare(a, b interface{}) int {
	fa := toFloat(a)
	fb := toFloat(b)
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	// Fall back to string comparison
	sa := fmt.Sprintf("%v", a)
	sb := fmt.Sprintf("%v", b)
	return strings.Compare(sa, sb)
}

func (e *Executor) executeLimit(ctx context.Context, node *PlanLimitNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	start := node.Offset
	if start > len(input) {
		return []Row{}, nil
	}

	end := start + node.Count
	if end > len(input) {
		end = len(input)
	}

	return input[start:end], nil
}

func (e *Executor) executeWindow(ctx context.Context, node *PlanWindowNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	// Group by window
	type windowKey struct {
		bucket int64
	}

	bucketDur := node.Duration.Nanoseconds()
	windows := make(map[int64][]Row)

	for _, row := range input {
		ts, ok := row["timestamp"].(time.Time)
		if !ok {
			continue
		}
		bucket := ts.UnixNano() / bucketDur
		windows[bucket] = append(windows[bucket], row)
	}

	// Sort buckets and return one row per window
	var buckets []int64
	for b := range windows {
		buckets = append(buckets, b)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })

	var result []Row
	for _, bucket := range buckets {
		windowRows := windows[bucket]
		ts := time.Unix(0, bucket*bucketDur)
		row := Row{
			"window_start": ts,
			"window_end":   ts.Add(node.Duration),
			"count":        len(windowRows),
		}
		// Include aggregated values
		if len(windowRows) > 0 {
			for k := range windowRows[0] {
				if k == "timestamp" {
					continue
				}
				var sum float64
				for _, wr := range windowRows {
					sum += toFloat(wr[k])
				}
				row[k] = sum / float64(len(windowRows))
			}
		}
		result = append(result, row)
	}

	return result, nil
}

func (e *Executor) executeJoin(ctx context.Context, node *PlanJoinNode, timeRange TimeRangeSpec) ([]Row, error) {
	// Execute both sides in parallel
	var leftRows, rightRows []Row
	var leftErr, rightErr error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		leftRows, leftErr = e.executeNode(ctx, node.Left, timeRange)
	}()

	go func() {
		defer wg.Done()
		rightRows, rightErr = e.executeNode(ctx, node.Right, timeRange)
	}()

	wg.Wait()

	if leftErr != nil {
		return nil, leftErr
	}
	if rightErr != nil {
		return nil, rightErr
	}

	// Build hash index on right side
	rightIndex := make(map[string][]Row)
	for _, row := range rightRows {
		key := e.joinKey(row, node.JoinFields)
		rightIndex[key] = append(rightIndex[key], row)
	}

	// Join
	var result []Row
	for _, leftRow := range leftRows {
		key := e.joinKey(leftRow, node.JoinFields)
		matches := rightIndex[key]

		// Time tolerance matching
		if node.TimeTolerance > 0 && len(matches) > 0 {
			leftTs, ok := leftRow["timestamp"].(time.Time)
			if ok {
				var filtered []Row
				for _, m := range matches {
					rightTs, ok := m["timestamp"].(time.Time)
					if ok {
						diff := leftTs.Sub(rightTs)
						if diff < 0 {
							diff = -diff
						}
						if diff <= node.TimeTolerance {
							filtered = append(filtered, m)
						}
					}
				}
				matches = filtered
			}
		}

		for _, rightRow := range matches {
			// Merge rows
			merged := make(Row)
			for k, v := range leftRow {
				merged[k] = v
			}
			for k, v := range rightRow {
				if _, exists := merged[k]; !exists {
					merged[k] = v
				} else {
					// Prefix with right source
					merged["right_"+k] = v
				}
			}
			result = append(result, merged)
		}
	}

	return result, nil
}

func (e *Executor) joinKey(row Row, fields []string) string {
	var parts []string
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%v", row[f]))
	}
	return strings.Join(parts, "|")
}

func (e *Executor) executeAnomaly(ctx context.Context, node *PlanAnomalyNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	// Simple Z-score based anomaly detection
	var values []float64
	for _, row := range input {
		if v, ok := row["value"]; ok {
			values = append(values, toFloat(v))
		}
	}

	mean, stddev := meanStddev(values)
	threshold := (1 - node.Sensitivity) * 3 // Convert sensitivity to z-score threshold

	var result []Row
	for i, row := range input {
		newRow := make(Row)
		for k, v := range row {
			newRow[k] = v
		}

		if i < len(values) {
			zscore := 0.0
			if stddev > 0 {
				zscore = math.Abs(values[i]-mean) / stddev
			}
			newRow["anomaly_score"] = zscore
			newRow["is_anomaly"] = zscore > threshold
		}

		result = append(result, newRow)
	}

	return result, nil
}

func meanStddev(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	return mean, math.Sqrt(variance)
}

func (e *Executor) executeExtract(ctx context.Context, node *PlanExtractNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	if node.Auto {
		// Auto extraction - simple key=value pattern
		return e.autoExtract(input)
	}

	// Pattern-based extraction
	return e.patternExtract(input, node.Pattern)
}

func (e *Executor) autoExtract(rows []Row) ([]Row, error) {
	// Simple key=value and JSON extraction
	kvRegex := regexp.MustCompile(`(\w+)=("[^"]*"|[^\s,]+)`)

	var result []Row
	for _, row := range rows {
		newRow := make(Row)
		for k, v := range row {
			newRow[k] = v
		}

		if msg, ok := row["message"].(string); ok {
			// Extract key=value pairs
			matches := kvRegex.FindAllStringSubmatch(msg, -1)
			for _, m := range matches {
				key := m[1]
				val := strings.Trim(m[2], "\"")
				newRow[key] = val
			}
		}

		result = append(result, newRow)
	}

	return result, nil
}

func (e *Executor) patternExtract(rows []Row, pattern string) ([]Row, error) {
	// Convert pattern like "{timestamp} {level} {msg}" to regex
	// Named groups: {name} or {name:type}
	groupRegex := regexp.MustCompile(`\{(\w+)(?::(\w+))?\}`)
	regexPattern := groupRegex.ReplaceAllStringFunc(pattern, func(m string) string {
		parts := groupRegex.FindStringSubmatch(m)
		name := parts[1]
		typ := parts[2]

		var expr string
		switch typ {
		case "int":
			expr = `(-?\d+)`
		case "float":
			expr = `(-?\d+\.?\d*)`
		case "ip":
			expr = `(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`
		default:
			expr = `(\S+)`
		}
		return fmt.Sprintf("(?P<%s>%s)", name, expr[1:len(expr)-1])
	})

	re, err := regexp.Compile(regexPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	names := re.SubexpNames()

	var result []Row
	for _, row := range rows {
		newRow := make(Row)
		for k, v := range row {
			newRow[k] = v
		}

		if msg, ok := row["message"].(string); ok {
			match := re.FindStringSubmatch(msg)
			if match != nil {
				for i, name := range names {
					if i > 0 && name != "" {
						newRow[name] = match[i]
					}
				}
			}
		}

		result = append(result, newRow)
	}

	return result, nil
}

func (e *Executor) executeHistogram(ctx context.Context, node *PlanHistogramNode, timeRange TimeRangeSpec) ([]Row, error) {
	input, err := e.executeNode(ctx, node.Input, timeRange)
	if err != nil {
		return nil, err
	}

	// Collect values and find range
	var values []float64
	for _, row := range input {
		if v, ok := row[node.Field]; ok {
			values = append(values, toFloat(v))
		}
	}

	if len(values) == 0 {
		return []Row{}, nil
	}

	// Find min/max
	minVal, maxVal := values[0], values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Create buckets
	bucketSize := (maxVal - minVal) / float64(node.Buckets)
	if bucketSize == 0 {
		bucketSize = 1
	}

	buckets := make([]int, node.Buckets)
	for _, v := range values {
		idx := int((v - minVal) / bucketSize)
		if idx >= node.Buckets {
			idx = node.Buckets - 1
		}
		if idx < 0 {
			idx = 0
		}
		buckets[idx]++
	}

	// Create result rows
	var result []Row
	for i := 0; i < node.Buckets; i++ {
		low := minVal + float64(i)*bucketSize
		high := low + bucketSize
		result = append(result, Row{
			"bucket":     i,
			"low":        low,
			"high":       high,
			"count":      buckets[i],
			"percentage": float64(buckets[i]) / float64(len(values)) * 100,
		})
	}

	return result, nil
}

// Expression evaluation
func (e *Executor) evalExpr(expr Expr, row Row) (interface{}, error) {
	switch n := expr.(type) {
	case *LiteralExpr:
		return n.Value, nil

	case *IdentifierExpr:
		if n.Name == "*" {
			return row, nil
		}
		val := row[n.Name]
		for _, p := range n.Path {
			if m, ok := val.(map[string]interface{}); ok {
				val = m[p]
			} else if m, ok := val.(Row); ok {
				val = m[p]
			}
		}
		return val, nil

	case *BinaryExpr:
		left, err := e.evalExpr(n.Left, row)
		if err != nil {
			return nil, err
		}
		right, err := e.evalExpr(n.Right, row)
		if err != nil {
			return nil, err
		}
		return e.evalBinaryOp(n.Operator, left, right)

	case *UnaryExpr:
		val, err := e.evalExpr(n.Operand, row)
		if err != nil {
			return nil, err
		}
		return e.evalUnaryOp(n.Operator, val)

	case *FunctionCallExpr:
		var args []interface{}
		for _, arg := range n.Args {
			val, err := e.evalExpr(arg, row)
			if err != nil {
				return nil, err
			}
			args = append(args, val)
		}
		return e.callFunction(n.Name, args)

	case *ListExpr:
		var values []interface{}
		for _, v := range n.Values {
			val, err := e.evalExpr(v, row)
			if err != nil {
				return nil, err
			}
			values = append(values, val)
		}
		return values, nil

	default:
		return nil, fmt.Errorf("unsupported expression: %T", expr)
	}
}

func (e *Executor) evalBinaryOp(op string, left, right interface{}) (interface{}, error) {
	switch op {
	case "=":
		return equals(left, right), nil
	case "!=":
		return !equals(left, right), nil
	case "<":
		return toFloat(left) < toFloat(right), nil
	case "<=":
		return toFloat(left) <= toFloat(right), nil
	case ">":
		return toFloat(left) > toFloat(right), nil
	case ">=":
		return toFloat(left) >= toFloat(right), nil
	case "+":
		return toFloat(left) + toFloat(right), nil
	case "-":
		return toFloat(left) - toFloat(right), nil
	case "*":
		return toFloat(left) * toFloat(right), nil
	case "/":
		r := toFloat(right)
		if r == 0 {
			return nil, nil
		}
		return toFloat(left) / r, nil
	case "%":
		return int(toFloat(left)) % int(toFloat(right)), nil
	case "and":
		return toBool(left) && toBool(right), nil
	case "or":
		return toBool(left) || toBool(right), nil
	case "like":
		pattern := fmt.Sprintf("%v", right)
		pattern = strings.ReplaceAll(pattern, "%", ".*")
		pattern = strings.ReplaceAll(pattern, "_", ".")
		re, err := regexp.Compile("^" + pattern + "$")
		if err != nil {
			return false, nil
		}
		return re.MatchString(fmt.Sprintf("%v", left)), nil
	case "matches":
		re, err := regexp.Compile(fmt.Sprintf("%v", right))
		if err != nil {
			return false, nil
		}
		return re.MatchString(fmt.Sprintf("%v", left)), nil
	case "in":
		list, ok := right.([]interface{})
		if !ok {
			return false, nil
		}
		for _, v := range list {
			if equals(left, v) {
				return true, nil
			}
		}
		return false, nil
	default:
		return nil, fmt.Errorf("unknown operator: %s", op)
	}
}

func (e *Executor) evalUnaryOp(op string, val interface{}) (interface{}, error) {
	switch op {
	case "not":
		return !toBool(val), nil
	case "-":
		return -toFloat(val), nil
	default:
		return nil, fmt.Errorf("unknown unary operator: %s", op)
	}
}

func (e *Executor) callFunction(name string, args []interface{}) (interface{}, error) {
	fn, ok := e.functions[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("unknown function: %s", name)
	}
	return fn(args...)
}

func (e *Executor) registerBuiltins() {
	e.functions["lower"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return strings.ToLower(fmt.Sprintf("%v", args[0])), nil
	}

	e.functions["upper"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return strings.ToUpper(fmt.Sprintf("%v", args[0])), nil
	}

	e.functions["length"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return 0, nil
		}
		return len(fmt.Sprintf("%v", args[0])), nil
	}

	e.functions["abs"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Abs(toFloat(args[0])), nil
	}

	e.functions["round"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Round(toFloat(args[0])), nil
	}

	e.functions["floor"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Floor(toFloat(args[0])), nil
	}

	e.functions["ceil"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Ceil(toFloat(args[0])), nil
	}

	e.functions["coalesce"] = func(args ...interface{}) (interface{}, error) {
		for _, arg := range args {
			if arg != nil {
				return arg, nil
			}
		}
		return nil, nil
	}

	e.functions["now"] = func(args ...interface{}) (interface{}, error) {
		return time.Now(), nil
	}

	e.functions["substring"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, nil
		}
		s := fmt.Sprintf("%v", args[0])
		start := int(toFloat(args[1]))
		if start < 0 || start >= len(s) {
			return "", nil
		}
		if len(args) >= 3 {
			length := int(toFloat(args[2]))
			end := start + length
			if end > len(s) {
				end = len(s)
			}
			return s[start:end], nil
		}
		return s[start:], nil
	}

	e.functions["concat"] = func(args ...interface{}) (interface{}, error) {
		var result strings.Builder
		for _, arg := range args {
			result.WriteString(fmt.Sprintf("%v", arg))
		}
		return result.String(), nil
	}

	e.functions["split"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, nil
		}
		s := fmt.Sprintf("%v", args[0])
		sep := fmt.Sprintf("%v", args[1])
		return strings.Split(s, sep), nil
	}

	e.functions["contains"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return false, nil
		}
		s := fmt.Sprintf("%v", args[0])
		substr := fmt.Sprintf("%v", args[1])
		return strings.Contains(s, substr), nil
	}

	e.functions["replace"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 3 {
			return nil, nil
		}
		s := fmt.Sprintf("%v", args[0])
		old := fmt.Sprintf("%v", args[1])
		new := fmt.Sprintf("%v", args[2])
		return strings.ReplaceAll(s, old, new), nil
	}

	e.functions["log"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Log(toFloat(args[0])), nil
	}

	e.functions["log10"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Log10(toFloat(args[0])), nil
	}

	e.functions["pow"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, nil
		}
		return math.Pow(toFloat(args[0]), toFloat(args[1])), nil
	}

	e.functions["sqrt"] = func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, nil
		}
		return math.Sqrt(toFloat(args[0])), nil
	}
}

// Helper functions
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
	case time.Duration:
		return float64(n)
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int:
		return b != 0
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		return b != "" && b != "false" && b != "0"
	default:
		return v != nil
	}
}

func equals(a, b interface{}) bool {
	// Handle wildcards
	if s, ok := b.(string); ok && strings.Contains(s, "*") {
		pattern := strings.ReplaceAll(s, "*", ".*")
		re, err := regexp.Compile("^" + pattern + "$")
		if err == nil {
			return re.MatchString(fmt.Sprintf("%v", a))
		}
	}

	// Try numeric comparison
	fa := toFloat(a)
	fb := toFloat(b)
	if fa != 0 || fb != 0 {
		return fa == fb
	}

	// String comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
