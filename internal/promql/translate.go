package promql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// MetricsStore provides access to the metrics database.
type MetricsStore interface {
	ScanRange(ctx context.Context, metric string, labels map[string]string, matchers []*LabelMatcher, start, end time.Time) ([]Series, error)
	ListMetrics(ctx context.Context) ([]string, error)
	ListLabels(ctx context.Context, metric string) ([]string, error)
	ListLabelValues(ctx context.Context, label string, metric string) ([]string, error)
}

// SQLMetricsStore implements MetricsStore using SQLite.
type SQLMetricsStore struct {
	db *sql.DB
}

// NewSQLMetricsStore creates a new SQL-based metrics store.
func NewSQLMetricsStore(db *sql.DB) *SQLMetricsStore {
	return &SQLMetricsStore{db: db}
}

// ScanRange retrieves time series data matching the selector.
// When series/label_index tables are available, uses SQL-level filtering
// instead of reading all rows and post-filtering in Go.
func (s *SQLMetricsStore) ScanRange(ctx context.Context, metric string, labels map[string]string, matchers []*LabelMatcher, start, end time.Time) ([]Series, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Try indexed path first if we have equality matchers we can push down
	eqMatchers := extractEqualityMatchers(matchers)
	if len(eqMatchers) > 0 && s.hasLabelIndex() {
		return s.scanRangeIndexed(ctx, metric, eqMatchers, matchers, start, end)
	}

	return s.scanRangeFull(ctx, metric, matchers, start, end)
}

// hasLabelIndex checks if the label_index table exists.
func (s *SQLMetricsStore) hasLabelIndex() bool {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='label_index'").Scan(&n)
	return err == nil && n > 0
}

// extractEqualityMatchers returns matchers that can be pushed down to SQL.
func extractEqualityMatchers(matchers []*LabelMatcher) []*LabelMatcher {
	var eq []*LabelMatcher
	for _, m := range matchers {
		if m.Type == MatchEqual && m.Name != "__name__" {
			eq = append(eq, m)
		}
	}
	return eq
}

// scanRangeIndexed uses the label_index to pre-filter at the SQL level.
func (s *SQLMetricsStore) scanRangeIndexed(ctx context.Context, metric string, eqMatchers, allMatchers []*LabelMatcher, start, end time.Time) ([]Series, error) {
	// Build a query that JOINs through label_index to find matching series IDs,
	// then only reads metric rows for those series.
	//
	// For N equality matchers, we intersect series that match ALL of them:
	//   SELECT series_id FROM label_index WHERE (key='k1' AND value='v1')
	//   INTERSECT
	//   SELECT series_id FROM label_index WHERE (key='k2' AND value='v2')
	var intersectParts []string
	var args []interface{}

	for _, m := range eqMatchers {
		intersectParts = append(intersectParts,
			"SELECT series_id FROM label_index WHERE label_key = ? AND label_value = ?")
		args = append(args, m.Name, m.Value)
	}

	seriesQuery := strings.Join(intersectParts, " INTERSECT ")

	// Also filter by metric name via the series table
	query := `SELECT m.timestamp, m.name, m.value, m.tags
		FROM metrics m
		INNER JOIN series s ON m.series_id = s.id
		WHERE m.series_id IN (` + seriesQuery + `)
		AND m.timestamp >= ? AND m.timestamp <= ?`
	args = append(args, start, end)

	if metric != "" {
		query += " AND s.name = ?"
		args = append(args, metric)
	}

	query += " ORDER BY m.name, m.timestamp"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		// Fall back to full scan if indexed query fails (e.g. old schema)
		return s.scanRangeFull(ctx, metric, allMatchers, start, end)
	}
	defer rows.Close()

	seriesMap := make(map[string]*Series)
	// Non-equality matchers still need Go-level filtering
	remainingMatchers := filterOutEquality(allMatchers, eqMatchers)

	for rows.Next() {
		var ts time.Time
		var name string
		var value float64
		var tagsJSON sql.NullString

		if err := rows.Scan(&ts, &name, &value, &tagsJSON); err != nil {
			continue
		}

		tags := make(map[string]string)
		tags["__name__"] = name
		if tagsJSON.Valid && tagsJSON.String != "" {
			json.Unmarshal([]byte(tagsJSON.String), &tags)
			tags["__name__"] = name
		}

		// Apply remaining non-equality matchers in Go
		if !matchLabels(tags, remainingMatchers) {
			continue
		}

		key := labelKey(tags)
		series, ok := seriesMap[key]
		if !ok {
			series = &Series{Labels: tags}
			seriesMap[key] = series
		}
		series.Samples = append(series.Samples, Sample{Timestamp: ts, Value: value})
	}

	result := make([]Series, 0, len(seriesMap))
	for _, series := range seriesMap {
		result = append(result, *series)
	}
	return result, nil
}

func filterOutEquality(all, eq []*LabelMatcher) []*LabelMatcher {
	eqSet := make(map[*LabelMatcher]bool, len(eq))
	for _, m := range eq {
		eqSet[m] = true
	}
	var remaining []*LabelMatcher
	for _, m := range all {
		if !eqSet[m] {
			remaining = append(remaining, m)
		}
	}
	return remaining
}

// scanRangeFull is the original full-scan path for backward compatibility.
func (s *SQLMetricsStore) scanRangeFull(ctx context.Context, metric string, matchers []*LabelMatcher, start, end time.Time) ([]Series, error) {
	query := `SELECT timestamp, name, value, tags FROM metrics WHERE timestamp >= ? AND timestamp <= ?`
	args := []interface{}{start, end}

	if metric != "" {
		query += " AND name = ?"
		args = append(args, metric)
	}

	query += " ORDER BY name, timestamp"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	seriesMap := make(map[string]*Series)

	for rows.Next() {
		var ts time.Time
		var name string
		var value float64
		var tagsJSON sql.NullString

		if err := rows.Scan(&ts, &name, &value, &tagsJSON); err != nil {
			continue
		}

		tags := make(map[string]string)
		tags["__name__"] = name
		if tagsJSON.Valid && tagsJSON.String != "" {
			json.Unmarshal([]byte(tagsJSON.String), &tags)
			tags["__name__"] = name
		}

		if !matchLabels(tags, matchers) {
			continue
		}

		key := labelKey(tags)
		series, ok := seriesMap[key]
		if !ok {
			series = &Series{Labels: tags}
			seriesMap[key] = series
		}

		series.Samples = append(series.Samples, Sample{
			Timestamp: ts,
			Value:     value,
		})
	}

	result := make([]Series, 0, len(seriesMap))
	for _, series := range seriesMap {
		result = append(result, *series)
	}

	return result, nil
}

// ListMetrics returns all metric names.
func (s *SQLMetricsStore) ListMetrics(ctx context.Context) ([]string, error) {
	if s.db == nil {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT name FROM metrics ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		result = append(result, name)
	}
	return result, nil
}

// ListLabels returns all label names for a metric.
func (s *SQLMetricsStore) ListLabels(ctx context.Context, metric string) ([]string, error) {
	if s.db == nil {
		return nil, nil
	}

	// Try indexed path first
	if s.hasLabelIndex() {
		var query string
		var args []interface{}
		if metric != "" {
			query = `SELECT DISTINCT li.label_key FROM label_index li
				INNER JOIN series s ON li.series_id = s.id WHERE s.name = ?`
			args = append(args, metric)
		} else {
			query = `SELECT DISTINCT label_key FROM label_index`
		}

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err == nil {
			defer rows.Close()
			result := []string{"__name__"}
			for rows.Next() {
				var key string
				if rows.Scan(&key) == nil {
					result = append(result, key)
				}
			}
			sort.Strings(result)
			return result, nil
		}
	}

	// Fallback: scan JSON tags
	labelSet := make(map[string]bool)
	labelSet["__name__"] = true

	query := "SELECT DISTINCT tags FROM metrics"
	if metric != "" {
		query += " WHERE name = ?"
	}
	query += " LIMIT 1000"

	var rows *sql.Rows
	var err error
	if metric != "" {
		rows, err = s.db.QueryContext(ctx, query, metric)
	} else {
		rows, err = s.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tagsJSON sql.NullString
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			var tags map[string]string
			if json.Unmarshal([]byte(tagsJSON.String), &tags) == nil {
				for k := range tags {
					labelSet[k] = true
				}
			}
		}
	}

	result := make([]string, 0, len(labelSet))
	for k := range labelSet {
		result = append(result, k)
	}
	sort.Strings(result)
	return result, nil
}

// ListLabelValues returns all values for a label.
func (s *SQLMetricsStore) ListLabelValues(ctx context.Context, label string, metric string) ([]string, error) {
	if s.db == nil {
		return nil, nil
	}

	if label == "__name__" {
		return s.ListMetrics(ctx)
	}

	// Try indexed path first
	if s.hasLabelIndex() {
		var query string
		var args []interface{}
		if metric != "" {
			query = `SELECT DISTINCT li.label_value FROM label_index li
				INNER JOIN series s ON li.series_id = s.id
				WHERE li.label_key = ? AND s.name = ?`
			args = append(args, label, metric)
		} else {
			query = `SELECT DISTINCT label_value FROM label_index WHERE label_key = ?`
			args = append(args, label)
		}

		rows, err := s.db.QueryContext(ctx, query, args...)
		if err == nil {
			defer rows.Close()
			var result []string
			for rows.Next() {
				var val string
				if rows.Scan(&val) == nil {
					result = append(result, val)
				}
			}
			sort.Strings(result)
			return result, nil
		}
	}

	// Fallback: scan JSON tags
	valueSet := make(map[string]bool)

	query := "SELECT tags FROM metrics"
	if metric != "" {
		query += " WHERE name = ?"
	}
	query += " LIMIT 10000"

	var rows *sql.Rows
	var err error
	if metric != "" {
		rows, err = s.db.QueryContext(ctx, query, metric)
	} else {
		rows, err = s.db.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var tagsJSON sql.NullString
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			var tags map[string]string
			if json.Unmarshal([]byte(tagsJSON.String), &tags) == nil {
				if v, ok := tags[label]; ok {
					valueSet[v] = true
				}
			}
		}
	}

	result := make([]string, 0, len(valueSet))
	for v := range valueSet {
		result = append(result, v)
	}
	sort.Strings(result)
	return result, nil
}

func matchLabels(labels map[string]string, matchers []*LabelMatcher) bool {
	for _, m := range matchers {
		if !m.Matches(labels[m.Name]) {
			return false
		}
	}
	return true
}

func labelKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte(',')
	}
	return b.String()
}

// Evaluator evaluates PromQL expressions.
type Evaluator struct {
	store MetricsStore
}

// NewEvaluator creates a new PromQL evaluator.
func NewEvaluator(store MetricsStore) *Evaluator {
	return &Evaluator{store: store}
}

// EvalOptions contains options for query evaluation.
type EvalOptions struct {
	Start    time.Time
	End      time.Time
	Interval time.Duration // step for range queries
}

// Eval evaluates a PromQL expression.
func (e *Evaluator) Eval(ctx context.Context, expr Expr, opts *EvalOptions) (Value, error) {
	evalCtx := &EvalContext{
		Start:    opts.Start,
		End:      opts.End,
		Interval: opts.Interval,
	}
	return e.eval(ctx, expr, evalCtx)
}

func (e *Evaluator) eval(ctx context.Context, expr Expr, evalCtx *EvalContext) (Value, error) {
	switch node := expr.(type) {
	case *VectorSelector:
		return e.evalVectorSelector(ctx, node, evalCtx)

	case *MatrixSelector:
		return e.evalMatrixSelector(ctx, node, evalCtx)

	case *NumberLiteral:
		return Scalar{Val: node.Val, Timestamp: evalCtx.End}, nil

	case *StringLiteral:
		return String{Val: node.Val, Timestamp: evalCtx.End}, nil

	case *UnaryExpr:
		return e.evalUnary(ctx, node, evalCtx)

	case *BinaryExpr:
		return e.evalBinary(ctx, node, evalCtx)

	case *AggregateExpr:
		return e.evalAggregate(ctx, node, evalCtx)

	case *Call:
		return e.evalCall(ctx, node, evalCtx)

	case *ParenExpr:
		return e.eval(ctx, node.Expr, evalCtx)

	case *Subquery:
		return e.evalSubquery(ctx, node, evalCtx)

	default:
		return nil, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

func (e *Evaluator) evalVectorSelector(ctx context.Context, vs *VectorSelector, evalCtx *EvalContext) (Value, error) {
	// Adjust time for offset
	queryEnd := evalCtx.End
	if vs.Offset != 0 {
		queryEnd = queryEnd.Add(-vs.Offset)
	}

	// Handle @ modifier
	if vs.Timestamp != nil {
		queryEnd = time.Unix(*vs.Timestamp, 0)
	} else if vs.StartOrEnd == 1 {
		queryEnd = evalCtx.Start
	} else if vs.StartOrEnd == 2 {
		queryEnd = evalCtx.End
	}

	// For instant queries, use a small window to find the most recent sample
	queryStart := queryEnd.Add(-5 * time.Minute) // Look back 5 minutes for stale data

	series, err := e.store.ScanRange(ctx, vs.Name, nil, vs.LabelMatchers, queryStart, queryEnd)
	if err != nil {
		return nil, err
	}

	// Get the most recent sample for each series
	result := make(Vector, 0, len(series))
	for _, s := range series {
		if len(s.Samples) == 0 {
			continue
		}
		// Get the last sample
		lastSample := s.Samples[len(s.Samples)-1]
		result = append(result, VectorSample{
			Labels:    s.Labels,
			Value:     lastSample.Value,
			Timestamp: lastSample.Timestamp,
		})
	}

	return result, nil
}

func (e *Evaluator) evalMatrixSelector(ctx context.Context, ms *MatrixSelector, evalCtx *EvalContext) (Value, error) {
	vs := ms.VectorSelector

	// Adjust time for offset
	queryEnd := evalCtx.End
	if vs.Offset != 0 {
		queryEnd = queryEnd.Add(-vs.Offset)
	}

	// Handle @ modifier
	if vs.Timestamp != nil {
		queryEnd = time.Unix(*vs.Timestamp, 0)
	} else if vs.StartOrEnd == 1 {
		queryEnd = evalCtx.Start
	} else if vs.StartOrEnd == 2 {
		queryEnd = evalCtx.End
	}

	queryStart := queryEnd.Add(-ms.Range)

	series, err := e.store.ScanRange(ctx, vs.Name, nil, vs.LabelMatchers, queryStart, queryEnd)
	if err != nil {
		return nil, err
	}

	return Matrix(series), nil
}

func (e *Evaluator) evalUnary(ctx context.Context, node *UnaryExpr, evalCtx *EvalContext) (Value, error) {
	val, err := e.eval(ctx, node.Expr, evalCtx)
	if err != nil {
		return nil, err
	}

	if node.Op == ItemAdd {
		return val, nil
	}

	// Negate values
	switch v := val.(type) {
	case Scalar:
		return Scalar{Val: -v.Val, Timestamp: v.Timestamp}, nil
	case Vector:
		result := make(Vector, len(v))
		for i, s := range v {
			result[i] = VectorSample{
				Labels:    s.Labels,
				Value:     -s.Value,
				Timestamp: s.Timestamp,
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot negate %T", val)
	}
}

func (e *Evaluator) evalBinary(ctx context.Context, node *BinaryExpr, evalCtx *EvalContext) (Value, error) {
	lhs, err := e.eval(ctx, node.LHS, evalCtx)
	if err != nil {
		return nil, err
	}

	rhs, err := e.eval(ctx, node.RHS, evalCtx)
	if err != nil {
		return nil, err
	}

	// Handle different type combinations
	switch l := lhs.(type) {
	case Scalar:
		switch r := rhs.(type) {
		case Scalar:
			return e.scalarBinop(node.Op, l, r, node.ReturnBool), nil
		case Vector:
			return e.scalarVectorBinop(node.Op, l, r, true, node.ReturnBool), nil
		}
	case Vector:
		switch r := rhs.(type) {
		case Scalar:
			return e.scalarVectorBinop(node.Op, r, l, false, node.ReturnBool), nil
		case Vector:
			return e.vectorVectorBinop(node.Op, l, r, node.VectorMatching, node.ReturnBool), nil
		}
	}

	return nil, fmt.Errorf("invalid operand types: %T and %T", lhs, rhs)
}

func (e *Evaluator) scalarBinop(op BinaryOp, lhs, rhs Scalar, returnBool bool) Scalar {
	val := computeBinop(op, lhs.Val, rhs.Val)
	if op.IsComparisonOp() && !returnBool {
		// Comparison without bool modifier filters
		return Scalar{Val: val, Timestamp: lhs.Timestamp}
	}
	return Scalar{Val: val, Timestamp: lhs.Timestamp}
}

func (e *Evaluator) scalarVectorBinop(op BinaryOp, scalar Scalar, vec Vector, scalarOnLeft bool, returnBool bool) Vector {
	result := make(Vector, 0, len(vec))
	for _, s := range vec {
		var val float64
		if scalarOnLeft {
			val = computeBinop(op, scalar.Val, s.Value)
		} else {
			val = computeBinop(op, s.Value, scalar.Val)
		}

		if op.IsComparisonOp() && !returnBool {
			if val == 0 {
				continue // Filter out non-matching
			}
			result = append(result, VectorSample{
				Labels:    s.Labels,
				Value:     s.Value, // Keep original value
				Timestamp: s.Timestamp,
			})
		} else {
			result = append(result, VectorSample{
				Labels:    s.Labels,
				Value:     val,
				Timestamp: s.Timestamp,
			})
		}
	}
	return result
}

func (e *Evaluator) vectorVectorBinop(op BinaryOp, lhs, rhs Vector, matching *VectorMatching, returnBool bool) Vector {
	if matching == nil {
		matching = &VectorMatching{Card: CardOneToOne}
	}

	// Build a map of RHS samples by matching labels
	rhsMap := make(map[string][]VectorSample)
	for _, s := range rhs {
		key := matchingKey(s.Labels, matching)
		rhsMap[key] = append(rhsMap[key], s)
	}

	result := make(Vector, 0)
	for _, l := range lhs {
		key := matchingKey(l.Labels, matching)
		rhsSamples := rhsMap[key]

		for _, r := range rhsSamples {
			val := computeBinop(op, l.Value, r.Value)

			if op.IsComparisonOp() && !returnBool {
				if val == 0 {
					continue
				}
				result = append(result, VectorSample{
					Labels:    mergeLabels(l.Labels, r.Labels, matching),
					Value:     l.Value,
					Timestamp: l.Timestamp,
				})
			} else if op.IsSetOp() {
				if val != 0 {
					result = append(result, VectorSample{
						Labels:    l.Labels,
						Value:     l.Value,
						Timestamp: l.Timestamp,
					})
				}
			} else {
				result = append(result, VectorSample{
					Labels:    mergeLabels(l.Labels, r.Labels, matching),
					Value:     val,
					Timestamp: l.Timestamp,
				})
			}
		}
	}

	return result
}

func matchingKey(labels map[string]string, matching *VectorMatching) string {
	if matching.On {
		// Only use specified labels
		keys := make([]string, len(matching.MatchingLabels))
		copy(keys, matching.MatchingLabels)
		sort.Strings(keys)

		var b strings.Builder
		for _, k := range keys {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(labels[k])
			b.WriteByte(',')
		}
		return b.String()
	}

	// Use all labels except ignored ones
	ignored := make(map[string]bool)
	for _, l := range matching.MatchingLabels {
		ignored[l] = true
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		if !ignored[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte(',')
	}
	return b.String()
}

func mergeLabels(lhs, rhs map[string]string, matching *VectorMatching) map[string]string {
	result := make(map[string]string)
	for k, v := range lhs {
		result[k] = v
	}

	// Add group_left/group_right labels
	for _, l := range matching.Include {
		if v, ok := rhs[l]; ok {
			result[l] = v
		}
	}

	return result
}

func computeBinop(op BinaryOp, lhs, rhs float64) float64 {
	switch op {
	case OpAdd:
		return lhs + rhs
	case OpSub:
		return lhs - rhs
	case OpMul:
		return lhs * rhs
	case OpDiv:
		return lhs / rhs
	case OpMod:
		return float64(int64(lhs) % int64(rhs))
	case OpPow:
		return pow(lhs, rhs)
	case OpEql:
		if lhs == rhs {
			return 1
		}
		return 0
	case OpNeq:
		if lhs != rhs {
			return 1
		}
		return 0
	case OpLss:
		if lhs < rhs {
			return 1
		}
		return 0
	case OpGtr:
		if lhs > rhs {
			return 1
		}
		return 0
	case OpLte:
		if lhs <= rhs {
			return 1
		}
		return 0
	case OpGte:
		if lhs >= rhs {
			return 1
		}
		return 0
	case OpAnd:
		return lhs // and returns lhs if both exist
	case OpOr:
		return lhs // or prefers lhs
	case OpUnless:
		return 0 // unless removes matches
	}
	return 0
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func (e *Evaluator) evalAggregate(ctx context.Context, node *AggregateExpr, evalCtx *EvalContext) (Value, error) {
	val, err := e.eval(ctx, node.Expr, evalCtx)
	if err != nil {
		return nil, err
	}

	vec, ok := val.(Vector)
	if !ok {
		return nil, fmt.Errorf("aggregate requires vector input, got %T", val)
	}

	// Group samples
	groups := make(map[string][]VectorSample)
	groupLabels := make(map[string]map[string]string)

	for _, s := range vec {
		key := groupKey(s.Labels, node.Grouping, node.Without)
		groups[key] = append(groups[key], s)

		if _, ok := groupLabels[key]; !ok {
			groupLabels[key] = groupLabelSet(s.Labels, node.Grouping, node.Without)
		}
	}

	// Apply aggregation
	result := make(Vector, 0, len(groups))
	for key, samples := range groups {
		var aggVal float64
		var ts time.Time
		if len(samples) > 0 {
			ts = samples[0].Timestamp
		}

		switch node.Op {
		case ItemSum:
			for _, s := range samples {
				aggVal += s.Value
			}
		case ItemAvg:
			for _, s := range samples {
				aggVal += s.Value
			}
			aggVal /= float64(len(samples))
		case ItemMin:
			aggVal = samples[0].Value
			for _, s := range samples[1:] {
				if s.Value < aggVal {
					aggVal = s.Value
				}
			}
		case ItemMax:
			aggVal = samples[0].Value
			for _, s := range samples[1:] {
				if s.Value > aggVal {
					aggVal = s.Value
				}
			}
		case ItemCount:
			aggVal = float64(len(samples))
		case ItemStddev, ItemStdvar:
			mean := 0.0
			for _, s := range samples {
				mean += s.Value
			}
			mean /= float64(len(samples))
			variance := 0.0
			for _, s := range samples {
				d := s.Value - mean
				variance += d * d
			}
			variance /= float64(len(samples))
			if node.Op == ItemStddev {
				aggVal = sqrt(variance)
			} else {
				aggVal = variance
			}
		case ItemTopK, ItemBottomK:
			// Get k from param
			k := 1
			if node.Param != nil {
				if kVal, err := e.eval(ctx, node.Param, evalCtx); err == nil {
					if scalar, ok := kVal.(Scalar); ok {
						k = int(scalar.Val)
					}
				}
			}
			sort.Slice(samples, func(i, j int) bool {
				if node.Op == ItemTopK {
					return samples[i].Value > samples[j].Value
				}
				return samples[i].Value < samples[j].Value
			})
			if k > len(samples) {
				k = len(samples)
			}
			for _, s := range samples[:k] {
				result = append(result, VectorSample{
					Labels:    s.Labels,
					Value:     s.Value,
					Timestamp: s.Timestamp,
				})
			}
			continue
		case ItemQuantile:
			q := 0.5
			if node.Param != nil {
				if qVal, err := e.eval(ctx, node.Param, evalCtx); err == nil {
					if scalar, ok := qVal.(Scalar); ok {
						q = scalar.Val
					}
				}
			}
			values := make([]float64, len(samples))
			for i, s := range samples {
				values[i] = s.Value
			}
			sort.Float64s(values)
			aggVal = quantile(q, values)
		case ItemCountValues:
			// Count occurrences of each value
			valueCounts := make(map[float64]int)
			for _, s := range samples {
				valueCounts[s.Value]++
			}
			for val, count := range valueCounts {
				labels := copyLabels(groupLabels[key])
				if node.Param != nil {
					if strLit, ok := node.Param.(*StringLiteral); ok {
						labels[strLit.Val] = fmt.Sprintf("%g", val)
					}
				}
				result = append(result, VectorSample{
					Labels:    labels,
					Value:     float64(count),
					Timestamp: ts,
				})
			}
			continue
		case ItemGroup:
			aggVal = 1
		}

		result = append(result, VectorSample{
			Labels:    groupLabels[key],
			Value:     aggVal,
			Timestamp: ts,
		})
	}

	return result, nil
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func groupKey(labels map[string]string, grouping []string, without bool) string {
	var keys []string
	if without {
		excluded := make(map[string]bool)
		for _, l := range grouping {
			excluded[l] = true
		}
		for k := range labels {
			if !excluded[k] {
				keys = append(keys, k)
			}
		}
	} else if len(grouping) > 0 {
		keys = grouping
	} else {
		// No grouping - all in one group
		return ""
	}

	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte(',')
	}
	return b.String()
}

func groupLabelSet(labels map[string]string, grouping []string, without bool) map[string]string {
	result := make(map[string]string)
	if without {
		excluded := make(map[string]bool)
		for _, l := range grouping {
			excluded[l] = true
		}
		for k, v := range labels {
			if !excluded[k] {
				result[k] = v
			}
		}
	} else if len(grouping) > 0 {
		for _, l := range grouping {
			if v, ok := labels[l]; ok {
				result[l] = v
			}
		}
	}
	return result
}

func (e *Evaluator) evalCall(ctx context.Context, node *Call, evalCtx *EvalContext) (Value, error) {
	// Evaluate arguments
	args := make([]Value, len(node.Args))
	for i, arg := range node.Args {
		val, err := e.eval(ctx, arg, evalCtx)
		if err != nil {
			return nil, err
		}
		args[i] = val
	}

	// Call the function
	if node.Func.Call == nil {
		return nil, fmt.Errorf("function %s not implemented", node.Func.Name)
	}

	return node.Func.Call(args, evalCtx)
}

func (e *Evaluator) evalSubquery(ctx context.Context, node *Subquery, evalCtx *EvalContext) (Value, error) {
	step := node.Step
	if step == 0 {
		step = evalCtx.Interval
		if step == 0 {
			step = time.Minute
		}
	}

	end := evalCtx.End
	if node.Offset != 0 {
		end = end.Add(-node.Offset)
	}
	start := end.Add(-node.Range)

	// Evaluate at each step
	var allSeries []Series
	seriesMap := make(map[string]*Series)

	for t := start; !t.After(end); t = t.Add(step) {
		subCtx := &EvalContext{
			Start:    t.Add(-step),
			End:      t,
			Interval: step,
		}

		val, err := e.eval(ctx, node.Expr, subCtx)
		if err != nil {
			return nil, err
		}

		vec, ok := val.(Vector)
		if !ok {
			continue
		}

		for _, s := range vec {
			key := labelKey(s.Labels)
			series, exists := seriesMap[key]
			if !exists {
				series = &Series{Labels: s.Labels}
				seriesMap[key] = series
			}
			series.Samples = append(series.Samples, Sample{
				Timestamp: s.Timestamp,
				Value:     s.Value,
			})
		}
	}

	for _, s := range seriesMap {
		allSeries = append(allSeries, *s)
	}

	return Matrix(allSeries), nil
}
