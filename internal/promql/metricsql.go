package promql

import (
	"fmt"
	"math"
	"regexp"
	"sort"
)

// MetricsQL extensions - VictoriaMetrics compatible functions
// These extend the standard PromQL with useful utilities

func init() {
	// Label manipulation functions
	Functions["label_set"] = &Function{
		Name:       "label_set",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		Variadic:   -1, // label, value pairs
		ReturnType: ValueTypeVector,
		Call:       funcLabelSet,
	}
	Functions["label_del"] = &Function{
		Name:       "label_del",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString},
		Variadic:   -1,
		ReturnType: ValueTypeVector,
		Call:       funcLabelDel,
	}
	Functions["label_keep"] = &Function{
		Name:       "label_keep",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString},
		Variadic:   -1,
		ReturnType: ValueTypeVector,
		Call:       funcLabelKeep,
	}
	Functions["label_copy"] = &Function{
		Name:       "label_copy",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		Variadic:   -1, // src, dst pairs
		ReturnType: ValueTypeVector,
		Call:       funcLabelCopy,
	}
	Functions["label_move"] = &Function{
		Name:       "label_move",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		Variadic:   -1, // src, dst pairs
		ReturnType: ValueTypeVector,
		Call:       funcLabelMove,
	}
	Functions["label_transform"] = &Function{
		Name:       "label_transform",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString, ValueTypeString},
		ReturnType: ValueTypeVector,
		Call:       funcLabelTransform,
	}
	Functions["label_match"] = &Function{
		Name:       "label_match",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		ReturnType: ValueTypeVector,
		Call:       funcLabelMatch,
	}
	Functions["label_mismatch"] = &Function{
		Name:       "label_mismatch",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		ReturnType: ValueTypeVector,
		Call:       funcLabelMismatch,
	}

	// Union function
	Functions["union"] = &Function{
		Name:       "union",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   -1,
		ReturnType: ValueTypeVector,
		Call:       funcUnion,
	}

	// Resource utilization
	Functions["ru"] = &Function{
		Name:       "ru",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcRU,
	}

	// Time-to-fill prediction
	Functions["ttf"] = &Function{
		Name:       "ttf",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcTTF,
	}

	// Range functions
	Functions["range_median"] = &Function{
		Name:       "range_median",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRangeMedian,
	}
	Functions["range_first"] = &Function{
		Name:       "range_first",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRangeFirst,
	}
	Functions["range_last"] = &Function{
		Name:       "range_last",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRangeLast,
	}

	// Running aggregations (cumulative)
	Functions["running_sum"] = &Function{
		Name:       "running_sum",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeMatrix,
		Call:       funcRunningSum,
	}
	Functions["running_max"] = &Function{
		Name:       "running_max",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeMatrix,
		Call:       funcRunningMax,
	}
	Functions["running_min"] = &Function{
		Name:       "running_min",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeMatrix,
		Call:       funcRunningMin,
	}
	Functions["running_avg"] = &Function{
		Name:       "running_avg",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeMatrix,
		Call:       funcRunningAvg,
	}

	// Smoothing
	Functions["smooth_exponential"] = &Function{
		Name:       "smooth_exponential",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeMatrix,
		Call:       funcSmoothExponential,
	}

	// Outlier detection
	Functions["outliers_mad"] = &Function{
		Name:       "outliers_mad",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcOutliersMAD,
	}
	Functions["outliers_iqr"] = &Function{
		Name:       "outliers_iqr",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcOutliersIQR,
	}

	// Histogram utilities
	Functions["histogram_share"] = &Function{
		Name:       "histogram_share",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcHistogramShare,
	}
	Functions["histogram_avg"] = &Function{
		Name:       "histogram_avg",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcHistogramAvg,
	}
	Functions["histogram_stdvar"] = &Function{
		Name:       "histogram_stdvar",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcHistogramStdvar,
	}

	// Threshold-based functions
	Functions["duration_over_time"] = &Function{
		Name:       "duration_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcDurationOverTime,
	}
	Functions["share_gt_over_time"] = &Function{
		Name:       "share_gt_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcShareGTOverTime,
	}
	Functions["share_le_over_time"] = &Function{
		Name:       "share_le_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcShareLEOverTime,
	}
	Functions["count_gt_over_time"] = &Function{
		Name:       "count_gt_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcCountGTOverTime,
	}
	Functions["count_le_over_time"] = &Function{
		Name:       "count_le_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcCountLEOverTime,
	}

	// Time/interval functions
	Functions["lag"] = &Function{
		Name:       "lag",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcLag,
	}
	Functions["lifetime"] = &Function{
		Name:       "lifetime",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcLifetime,
	}
	Functions["scrape_interval"] = &Function{
		Name:       "scrape_interval",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcScrapeInterval,
	}

	// Aliases for common operations
	Functions["first_over_time"] = &Function{
		Name:       "first_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRangeFirst,
	}
	Functions["median_over_time"] = &Function{
		Name:       "median_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRangeMedian,
	}
}

// Label manipulation functions

func funcLabelSet(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))

	for i, s := range vec {
		labels := copyLabels(s.Labels)
		// Process label/value pairs
		for j := 1; j+1 < len(args); j += 2 {
			label := args[j].(String).Val
			value := args[j+1].(String).Val
			if value == "" {
				delete(labels, label)
			} else {
				labels[label] = value
			}
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelDel(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))

	labelsToDelete := make(map[string]bool)
	for j := 1; j < len(args); j++ {
		labelsToDelete[args[j].(String).Val] = true
	}

	for i, s := range vec {
		labels := make(map[string]string)
		for k, v := range s.Labels {
			if !labelsToDelete[k] {
				labels[k] = v
			}
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelKeep(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))

	labelsToKeep := make(map[string]bool)
	for j := 1; j < len(args); j++ {
		labelsToKeep[args[j].(String).Val] = true
	}
	// Always keep __name__
	labelsToKeep["__name__"] = true

	for i, s := range vec {
		labels := make(map[string]string)
		for k, v := range s.Labels {
			if labelsToKeep[k] {
				labels[k] = v
			}
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelCopy(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))

	for i, s := range vec {
		labels := copyLabels(s.Labels)
		// Process src/dst pairs
		for j := 1; j+1 < len(args); j += 2 {
			src := args[j].(String).Val
			dst := args[j+1].(String).Val
			if val, ok := labels[src]; ok {
				labels[dst] = val
			}
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelMove(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))

	for i, s := range vec {
		labels := copyLabels(s.Labels)
		// Process src/dst pairs - copy then delete src
		for j := 1; j+1 < len(args); j += 2 {
			src := args[j].(String).Val
			dst := args[j+1].(String).Val
			if val, ok := labels[src]; ok {
				labels[dst] = val
				delete(labels, src)
			}
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelTransform(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	label := args[1].(String).Val
	regexStr := args[2].(String).Val
	replacement := args[3].(String).Val

	re, err := regexp.Compile(regexStr)
	if err != nil {
		return nil, err
	}

	result := make(Vector, len(vec))
	for i, s := range vec {
		labels := copyLabels(s.Labels)
		if val, ok := labels[label]; ok {
			labels[label] = re.ReplaceAllString(val, replacement)
		}
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelMatch(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	label := args[1].(String).Val
	regexStr := args[2].(String).Val

	re, err := regexp.Compile(regexStr)
	if err != nil {
		return nil, err
	}

	result := make(Vector, 0, len(vec))
	for _, s := range vec {
		if val, ok := s.Labels[label]; ok {
			if re.MatchString(val) {
				result = append(result, s)
			}
		}
	}
	return result, nil
}

func funcLabelMismatch(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	label := args[1].(String).Val
	regexStr := args[2].(String).Val

	re, err := regexp.Compile(regexStr)
	if err != nil {
		return nil, err
	}

	result := make(Vector, 0, len(vec))
	for _, s := range vec {
		val, ok := s.Labels[label]
		if !ok || !re.MatchString(val) {
			result = append(result, s)
		}
	}
	return result, nil
}

// Union function - merge multiple vectors
func funcUnion(args []Value, ctx *EvalContext) (Value, error) {
	result := make(Vector, 0)
	seen := make(map[string]bool)

	for _, arg := range args {
		vec := arg.(Vector)
		for _, s := range vec {
			key := labelsToKey(s.Labels)
			if !seen[key] {
				seen[key] = true
				result = append(result, s)
			}
		}
	}
	return result, nil
}

func labelsToKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := ""
	for _, k := range keys {
		key += k + "=" + labels[k] + ","
	}
	return key
}

// Resource utilization: ru(free, max) = (max - free) / max
func funcRU(args []Value, ctx *EvalContext) (Value, error) {
	freeVec := args[0].(Vector)
	maxVec := args[1].(Vector)

	// Match by labels
	maxByKey := make(map[string]VectorSample)
	for _, s := range maxVec {
		maxByKey[labelsToKey(s.Labels)] = s
	}

	result := make(Vector, 0, len(freeVec))
	for _, free := range freeVec {
		key := labelsToKey(free.Labels)
		if max, ok := maxByKey[key]; ok && max.Value != 0 {
			ru := (max.Value - free.Value) / max.Value
			result = append(result, VectorSample{
				Labels:    free.Labels,
				Value:     ru,
				Timestamp: free.Timestamp,
			})
		}
	}
	return result, nil
}

// Time-to-fill: predict when resource will be exhausted
// Uses linear regression to predict when value reaches 0 (or max)
func funcTTF(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))

	for _, series := range matrix {
		if len(series.Samples) < 2 {
			continue
		}

		slope, intercept := linearRegression(series.Samples)
		if slope >= 0 {
			// Not decreasing, return +Inf
			result = append(result, VectorSample{
				Labels:    series.Labels,
				Value:     math.Inf(1),
				Timestamp: series.Samples[len(series.Samples)-1].Timestamp,
			})
			continue
		}

		// Time to reach 0: solve for t where slope*t + intercept = 0
		// t = -intercept / slope
		ttf := -intercept / slope
		if ttf < 0 {
			ttf = 0
		}

		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     ttf,
			Timestamp: series.Samples[len(series.Samples)-1].Timestamp,
		})
	}
	return result, nil
}

// Range functions

func funcRangeMedian(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		values := make([]float64, len(samples))
		for i, s := range samples {
			values[i] = s.Value
		}
		sort.Float64s(values)
		return quantile(0.5, values)
	}), nil
}

func funcRangeFirst(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		return samples[0].Value
	}), nil
}

func funcRangeLast(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		return samples[len(samples)-1].Value
	}), nil
}

// Running (cumulative) aggregations

func funcRunningSum(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Matrix, len(matrix))

	for i, series := range matrix {
		newSamples := make([]Sample, len(series.Samples))
		sum := 0.0
		for j, s := range series.Samples {
			sum += s.Value
			newSamples[j] = Sample{
				Timestamp: s.Timestamp,
				Value:     sum,
			}
		}
		result[i] = Series{
			Labels:  series.Labels,
			Samples: newSamples,
		}
	}
	return result, nil
}

func funcRunningMax(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Matrix, len(matrix))

	for i, series := range matrix {
		newSamples := make([]Sample, len(series.Samples))
		maxVal := math.Inf(-1)
		for j, s := range series.Samples {
			if s.Value > maxVal {
				maxVal = s.Value
			}
			newSamples[j] = Sample{
				Timestamp: s.Timestamp,
				Value:     maxVal,
			}
		}
		result[i] = Series{
			Labels:  series.Labels,
			Samples: newSamples,
		}
	}
	return result, nil
}

func funcRunningMin(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Matrix, len(matrix))

	for i, series := range matrix {
		newSamples := make([]Sample, len(series.Samples))
		minVal := math.Inf(1)
		for j, s := range series.Samples {
			if s.Value < minVal {
				minVal = s.Value
			}
			newSamples[j] = Sample{
				Timestamp: s.Timestamp,
				Value:     minVal,
			}
		}
		result[i] = Series{
			Labels:  series.Labels,
			Samples: newSamples,
		}
	}
	return result, nil
}

func funcRunningAvg(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Matrix, len(matrix))

	for i, series := range matrix {
		newSamples := make([]Sample, len(series.Samples))
		sum := 0.0
		for j, s := range series.Samples {
			sum += s.Value
			newSamples[j] = Sample{
				Timestamp: s.Timestamp,
				Value:     sum / float64(j+1),
			}
		}
		result[i] = Series{
			Labels:  series.Labels,
			Samples: newSamples,
		}
	}
	return result, nil
}

// Exponential smoothing
func funcSmoothExponential(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	sf := args[1].(Scalar).Val // Smoothing factor (0-1)

	if sf < 0 || sf > 1 {
		sf = 0.5 // Default
	}

	result := make(Matrix, len(matrix))
	for i, series := range matrix {
		newSamples := make([]Sample, len(series.Samples))
		if len(series.Samples) == 0 {
			result[i] = Series{Labels: series.Labels, Samples: newSamples}
			continue
		}

		smoothed := series.Samples[0].Value
		for j, s := range series.Samples {
			smoothed = sf*s.Value + (1-sf)*smoothed
			newSamples[j] = Sample{
				Timestamp: s.Timestamp,
				Value:     smoothed,
			}
		}
		result[i] = Series{
			Labels:  series.Labels,
			Samples: newSamples,
		}
	}
	return result, nil
}

// Outlier detection using MAD (Median Absolute Deviation)
func funcOutliersMAD(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	threshold := args[1].(Scalar).Val

	if len(vec) == 0 {
		return Vector{}, nil
	}

	// Calculate median
	values := make([]float64, len(vec))
	for i, s := range vec {
		values[i] = s.Value
	}
	sort.Float64s(values)
	median := quantile(0.5, values)

	// Calculate MAD
	deviations := make([]float64, len(values))
	for i, v := range values {
		deviations[i] = math.Abs(v - median)
	}
	sort.Float64s(deviations)
	mad := quantile(0.5, deviations)

	// MAD-based modified z-score: 0.6745 * (x - median) / MAD
	// Return samples that exceed threshold
	result := make(Vector, 0)
	for _, s := range vec {
		if mad == 0 {
			continue
		}
		score := 0.6745 * math.Abs(s.Value-median) / mad
		if score > threshold {
			result = append(result, s)
		}
	}
	return result, nil
}

// Outlier detection using IQR (Interquartile Range)
func funcOutliersIQR(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	threshold := args[1].(Scalar).Val

	if len(vec) == 0 {
		return Vector{}, nil
	}

	values := make([]float64, len(vec))
	for i, s := range vec {
		values[i] = s.Value
	}
	sort.Float64s(values)

	q1 := quantile(0.25, values)
	q3 := quantile(0.75, values)
	iqr := q3 - q1

	lower := q1 - threshold*iqr
	upper := q3 + threshold*iqr

	result := make(Vector, 0)
	for _, s := range vec {
		if s.Value < lower || s.Value > upper {
			result = append(result, s)
		}
	}
	return result, nil
}

// Histogram utilities

// histogram_share returns the fraction of observations <= le
func funcHistogramShare(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	le := args[1].(Scalar).Val

	// Group by labels excluding 'le'
	groups := make(map[string][]VectorSample)
	for _, s := range vec {
		key := labelsWithout(s.Labels, "le")
		groups[key] = append(groups[key], s)
	}

	result := make(Vector, 0, len(groups))
	for _, buckets := range groups {
		share := histogramShare(le, buckets)
		if !math.IsNaN(share) {
			labels := make(map[string]string)
			for k, v := range buckets[0].Labels {
				if k != "le" {
					labels[k] = v
				}
			}
			result = append(result, VectorSample{
				Labels:    labels,
				Value:     share,
				Timestamp: buckets[0].Timestamp,
			})
		}
	}
	return result, nil
}

func histogramShare(le float64, buckets []VectorSample) float64 {
	type bucket struct {
		bound float64
		count float64
	}

	b := make([]bucket, 0, len(buckets))
	var total float64
	for _, s := range buckets {
		leStr, ok := s.Labels["le"]
		if !ok {
			continue
		}
		var bound float64
		if leStr == "+Inf" {
			bound = math.Inf(1)
			total = s.Value
		} else {
			var n int
			n, _ = fmt.Sscanf(leStr, "%f", &bound)
			if n != 1 {
				continue
			}
		}
		b = append(b, bucket{bound: bound, count: s.Value})
	}

	if total == 0 {
		return math.NaN()
	}

	sort.Slice(b, func(i, j int) bool { return b[i].bound < b[j].bound })

	// Find bucket containing le and interpolate
	for i, bkt := range b {
		if bkt.bound >= le {
			if i == 0 {
				return bkt.count / total
			}
			prev := b[i-1]
			if bkt.bound == prev.bound {
				return bkt.count / total
			}
			// Linear interpolation
			fraction := (le - prev.bound) / (bkt.bound - prev.bound)
			count := prev.count + fraction*(bkt.count-prev.count)
			return count / total
		}
	}
	return 1.0 // le is beyond max bucket
}

// histogram_avg computes average from histogram buckets
func funcHistogramAvg(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)

	groups := make(map[string][]VectorSample)
	for _, s := range vec {
		key := labelsWithout(s.Labels, "le")
		groups[key] = append(groups[key], s)
	}

	result := make(Vector, 0, len(groups))
	for _, buckets := range groups {
		avg := histogramAverage(buckets)
		if !math.IsNaN(avg) {
			labels := make(map[string]string)
			for k, v := range buckets[0].Labels {
				if k != "le" {
					labels[k] = v
				}
			}
			result = append(result, VectorSample{
				Labels:    labels,
				Value:     avg,
				Timestamp: buckets[0].Timestamp,
			})
		}
	}
	return result, nil
}

func histogramAverage(buckets []VectorSample) float64 {
	type bucket struct {
		bound float64
		count float64
	}

	b := make([]bucket, 0, len(buckets))
	for _, s := range buckets {
		leStr, ok := s.Labels["le"]
		if !ok || leStr == "+Inf" {
			continue
		}
		var bound float64
		var n int
		n, _ = fmt.Sscanf(leStr, "%f", &bound)
		if n != 1 {
			continue
		}
		b = append(b, bucket{bound: bound, count: s.Value})
	}

	if len(b) == 0 {
		return math.NaN()
	}

	sort.Slice(b, func(i, j int) bool { return b[i].bound < b[j].bound })

	// Compute weighted average using bucket midpoints
	var sum, total float64
	prevBound := 0.0
	prevCount := 0.0
	for _, bkt := range b {
		countInBucket := bkt.count - prevCount
		midpoint := (prevBound + bkt.bound) / 2
		sum += midpoint * countInBucket
		total += countInBucket
		prevBound = bkt.bound
		prevCount = bkt.count
	}

	if total == 0 {
		return math.NaN()
	}
	return sum / total
}

// histogram_stdvar computes variance from histogram buckets
func funcHistogramStdvar(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)

	groups := make(map[string][]VectorSample)
	for _, s := range vec {
		key := labelsWithout(s.Labels, "le")
		groups[key] = append(groups[key], s)
	}

	result := make(Vector, 0, len(groups))
	for _, buckets := range groups {
		variance := histogramVariance(buckets)
		if !math.IsNaN(variance) {
			labels := make(map[string]string)
			for k, v := range buckets[0].Labels {
				if k != "le" {
					labels[k] = v
				}
			}
			result = append(result, VectorSample{
				Labels:    labels,
				Value:     variance,
				Timestamp: buckets[0].Timestamp,
			})
		}
	}
	return result, nil
}

func histogramVariance(buckets []VectorSample) float64 {
	avg := histogramAverage(buckets)
	if math.IsNaN(avg) {
		return math.NaN()
	}

	type bucket struct {
		bound float64
		count float64
	}

	b := make([]bucket, 0, len(buckets))
	for _, s := range buckets {
		leStr, ok := s.Labels["le"]
		if !ok || leStr == "+Inf" {
			continue
		}
		var bound float64
		var n int
		n, _ = fmt.Sscanf(leStr, "%f", &bound)
		if n != 1 {
			continue
		}
		b = append(b, bucket{bound: bound, count: s.Value})
	}

	sort.Slice(b, func(i, j int) bool { return b[i].bound < b[j].bound })

	// Compute variance using bucket midpoints
	var sumSqDiff, total float64
	prevBound := 0.0
	prevCount := 0.0
	for _, bkt := range b {
		countInBucket := bkt.count - prevCount
		midpoint := (prevBound + bkt.bound) / 2
		diff := midpoint - avg
		sumSqDiff += diff * diff * countInBucket
		total += countInBucket
		prevBound = bkt.bound
		prevCount = bkt.count
	}

	if total == 0 {
		return math.NaN()
	}
	return sumSqDiff / total
}

// Threshold-based functions

func funcDurationOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	threshold := args[1].(Scalar).Val

	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) < 2 {
			return 0
		}
		var duration float64
		for i := 1; i < len(samples); i++ {
			if samples[i-1].Value > threshold {
				dt := samples[i].Timestamp.Sub(samples[i-1].Timestamp).Seconds()
				duration += dt
			}
		}
		return duration
	}), nil
}

func funcShareGTOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	threshold := args[1].(Scalar).Val

	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		count := 0
		for _, s := range samples {
			if s.Value > threshold {
				count++
			}
		}
		return float64(count) / float64(len(samples))
	}), nil
}

func funcShareLEOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	threshold := args[1].(Scalar).Val

	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		count := 0
		for _, s := range samples {
			if s.Value <= threshold {
				count++
			}
		}
		return float64(count) / float64(len(samples))
	}), nil
}

func funcCountGTOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	threshold := args[1].(Scalar).Val

	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		count := 0
		for _, s := range samples {
			if s.Value > threshold {
				count++
			}
		}
		return float64(count)
	}), nil
}

func funcCountLEOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	threshold := args[1].(Scalar).Val

	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		count := 0
		for _, s := range samples {
			if s.Value <= threshold {
				count++
			}
		}
		return float64(count)
	}), nil
}

// Time/interval functions

// lag returns seconds since last sample
func funcLag(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))

	for _, series := range matrix {
		if len(series.Samples) == 0 {
			continue
		}
		last := series.Samples[len(series.Samples)-1]
		lag := ctx.End.Sub(last.Timestamp).Seconds()
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     lag,
			Timestamp: ctx.End,
		})
	}
	return result, nil
}

// lifetime returns duration from first to last sample
func funcLifetime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))

	for _, series := range matrix {
		if len(series.Samples) < 2 {
			continue
		}
		first := series.Samples[0]
		last := series.Samples[len(series.Samples)-1]
		lifetime := last.Timestamp.Sub(first.Timestamp).Seconds()
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     lifetime,
			Timestamp: last.Timestamp,
		})
	}
	return result, nil
}

// scrape_interval estimates the scrape interval from samples
func funcScrapeInterval(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))

	for _, series := range matrix {
		if len(series.Samples) < 2 {
			continue
		}
		// Calculate average interval between samples
		totalInterval := series.Samples[len(series.Samples)-1].Timestamp.Sub(series.Samples[0].Timestamp).Seconds()
		avgInterval := totalInterval / float64(len(series.Samples)-1)

		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     avgInterval,
			Timestamp: series.Samples[len(series.Samples)-1].Timestamp,
		})
	}
	return result, nil
}
