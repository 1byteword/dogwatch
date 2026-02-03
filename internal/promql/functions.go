package promql

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ValueType represents the type of a PromQL value.
type ValueType int

const (
	ValueTypeNone ValueType = iota
	ValueTypeScalar
	ValueTypeVector
	ValueTypeMatrix
	ValueTypeString
)

func (v ValueType) String() string {
	switch v {
	case ValueTypeNone:
		return "none"
	case ValueTypeScalar:
		return "scalar"
	case ValueTypeVector:
		return "vector"
	case ValueTypeMatrix:
		return "matrix"
	case ValueTypeString:
		return "string"
	}
	return "?"
}

// Function represents a PromQL function definition.
type Function struct {
	Name       string
	ArgTypes   []ValueType
	Variadic   int      // -1 = any number, 0 = none, N = minimum required
	ReturnType ValueType
	Call       FunctionCall
}

// FunctionCall is the signature for function implementations.
type FunctionCall func(args []Value, ctx *EvalContext) (Value, error)

// EvalContext provides context for function evaluation.
type EvalContext struct {
	Start    time.Time
	End      time.Time
	Interval time.Duration
}

// Sample represents a single time series sample.
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// Series represents a time series with labels.
type Series struct {
	Labels  map[string]string
	Samples []Sample
}

// Value is the result of evaluating a PromQL expression.
type Value interface {
	Type() ValueType
}

// Scalar is a scalar value.
type Scalar struct {
	Val       float64
	Timestamp time.Time
}

func (s Scalar) Type() ValueType { return ValueTypeScalar }

// Vector is an instant vector (single sample per series).
type Vector []VectorSample

func (v Vector) Type() ValueType { return ValueTypeVector }

// VectorSample is a single sample in a vector.
type VectorSample struct {
	Labels    map[string]string
	Value     float64
	Timestamp time.Time
}

// Matrix is a range vector (multiple samples per series).
type Matrix []Series

func (m Matrix) Type() ValueType { return ValueTypeMatrix }

// String is a string value.
type String struct {
	Val       string
	Timestamp time.Time
}

func (s String) Type() ValueType { return ValueTypeString }

// Functions is the registry of all PromQL functions.
var Functions = map[string]*Function{
	// Rate functions
	"rate": {
		Name:       "rate",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcRate,
	},
	"irate": {
		Name:       "irate",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIrate,
	},
	"increase": {
		Name:       "increase",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIncrease,
	},
	"delta": {
		Name:       "delta",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcDelta,
	},
	"idelta": {
		Name:       "idelta",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcIdelta,
	},
	"deriv": {
		Name:       "deriv",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcDeriv,
	},
	"predict_linear": {
		Name:       "predict_linear",
		ArgTypes:   []ValueType{ValueTypeMatrix, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcPredictLinear,
	},
	"resets": {
		Name:       "resets",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcResets,
	},
	"changes": {
		Name:       "changes",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcChanges,
	},

	// Over time aggregations
	"avg_over_time": {
		Name:       "avg_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcAvgOverTime,
	},
	"sum_over_time": {
		Name:       "sum_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcSumOverTime,
	},
	"min_over_time": {
		Name:       "min_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcMinOverTime,
	},
	"max_over_time": {
		Name:       "max_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcMaxOverTime,
	},
	"count_over_time": {
		Name:       "count_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcCountOverTime,
	},
	"stddev_over_time": {
		Name:       "stddev_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcStddevOverTime,
	},
	"stdvar_over_time": {
		Name:       "stdvar_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcStdvarOverTime,
	},
	"quantile_over_time": {
		Name:       "quantile_over_time",
		ArgTypes:   []ValueType{ValueTypeScalar, ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcQuantileOverTime,
	},
	"last_over_time": {
		Name:       "last_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcLastOverTime,
	},
	"present_over_time": {
		Name:       "present_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcPresentOverTime,
	},

	// Histogram
	"histogram_quantile": {
		Name:       "histogram_quantile",
		ArgTypes:   []ValueType{ValueTypeScalar, ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcHistogramQuantile,
	},

	// Label functions
	"label_replace": {
		Name:       "label_replace",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString, ValueTypeString, ValueTypeString},
		ReturnType: ValueTypeVector,
		Call:       funcLabelReplace,
	},
	"label_join": {
		Name:       "label_join",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeString, ValueTypeString},
		Variadic:   -1,
		ReturnType: ValueTypeVector,
		Call:       funcLabelJoin,
	},

	// Selection/filtering
	"absent": {
		Name:       "absent",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcAbsent,
	},
	"absent_over_time": {
		Name:       "absent_over_time",
		ArgTypes:   []ValueType{ValueTypeMatrix},
		ReturnType: ValueTypeVector,
		Call:       funcAbsentOverTime,
	},

	// Math functions
	"abs": {
		Name:       "abs",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcAbs,
	},
	"ceil": {
		Name:       "ceil",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcCeil,
	},
	"floor": {
		Name:       "floor",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcFloor,
	},
	"round": {
		Name:       "round",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcRound,
	},
	"sqrt": {
		Name:       "sqrt",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSqrt,
	},
	"ln": {
		Name:       "ln",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLn,
	},
	"log2": {
		Name:       "log2",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLog2,
	},
	"log10": {
		Name:       "log10",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcLog10,
	},
	"exp": {
		Name:       "exp",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcExp,
	},
	"sgn": {
		Name:       "sgn",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSgn,
	},

	// Clamping
	"clamp": {
		Name:       "clamp",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcClamp,
	},
	"clamp_min": {
		Name:       "clamp_min",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcClampMin,
	},
	"clamp_max": {
		Name:       "clamp_max",
		ArgTypes:   []ValueType{ValueTypeVector, ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcClampMax,
	},

	// Type conversion
	"scalar": {
		Name:       "scalar",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeScalar,
		Call:       funcScalar,
	},
	"vector": {
		Name:       "vector",
		ArgTypes:   []ValueType{ValueTypeScalar},
		ReturnType: ValueTypeVector,
		Call:       funcVector,
	},

	// Time functions
	"time": {
		Name:       "time",
		ArgTypes:   []ValueType{},
		ReturnType: ValueTypeScalar,
		Call:       funcTime,
	},
	"timestamp": {
		Name:       "timestamp",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcTimestamp,
	},
	"day_of_month": {
		Name:       "day_of_month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDayOfMonth,
	},
	"day_of_week": {
		Name:       "day_of_week",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDayOfWeek,
	},
	"day_of_year": {
		Name:       "day_of_year",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDayOfYear,
	},
	"days_in_month": {
		Name:       "days_in_month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcDaysInMonth,
	},
	"hour": {
		Name:       "hour",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcHour,
	},
	"minute": {
		Name:       "minute",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcMinute,
	},
	"month": {
		Name:       "month",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcMonth,
	},
	"year": {
		Name:       "year",
		ArgTypes:   []ValueType{ValueTypeVector},
		Variadic:   1,
		ReturnType: ValueTypeVector,
		Call:       funcYear,
	},

	// Sorting
	"sort": {
		Name:       "sort",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSort,
	},
	"sort_desc": {
		Name:       "sort_desc",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       funcSortDesc,
	},

	// Trigonometric functions
	"acos": {
		Name:       "acos",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Acos),
	},
	"acosh": {
		Name:       "acosh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Acosh),
	},
	"asin": {
		Name:       "asin",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Asin),
	},
	"asinh": {
		Name:       "asinh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Asinh),
	},
	"atan": {
		Name:       "atan",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Atan),
	},
	"atanh": {
		Name:       "atanh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Atanh),
	},
	"cos": {
		Name:       "cos",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Cos),
	},
	"cosh": {
		Name:       "cosh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Cosh),
	},
	"sin": {
		Name:       "sin",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Sin),
	},
	"sinh": {
		Name:       "sinh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Sinh),
	},
	"tan": {
		Name:       "tan",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Tan),
	},
	"tanh": {
		Name:       "tanh",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(math.Tanh),
	},
	"deg": {
		Name:       "deg",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(func(v float64) float64 { return v * 180 / math.Pi }),
	},
	"rad": {
		Name:       "rad",
		ArgTypes:   []ValueType{ValueTypeVector},
		ReturnType: ValueTypeVector,
		Call:       makeSimpleMathFunc(func(v float64) float64 { return v * math.Pi / 180 }),
	},
	"pi": {
		Name:       "pi",
		ArgTypes:   []ValueType{},
		ReturnType: ValueTypeScalar,
		Call:       funcPi,
	},
}

// GetFunction returns the function with the given name.
func GetFunction(name string) (*Function, bool) {
	f, ok := Functions[name]
	return f, ok
}

// Helper to create simple math functions
func makeSimpleMathFunc(f func(float64) float64) FunctionCall {
	return func(args []Value, ctx *EvalContext) (Value, error) {
		vec := args[0].(Vector)
		result := make(Vector, len(vec))
		for i, s := range vec {
			result[i] = VectorSample{
				Labels:    s.Labels,
				Value:     f(s.Value),
				Timestamp: s.Timestamp,
			}
		}
		return result, nil
	}
}

// Function implementations

func funcRate(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return extrapolatedRate(matrix, true, true), nil
}

func funcIrate(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		if len(samples) < 2 {
			continue
		}
		last := samples[len(samples)-1]
		prev := samples[len(samples)-2]
		dt := last.Timestamp.Sub(prev.Timestamp).Seconds()
		if dt == 0 {
			continue
		}
		var rate float64
		if last.Value < prev.Value {
			rate = last.Value / dt
		} else {
			rate = (last.Value - prev.Value) / dt
		}
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     rate,
			Timestamp: last.Timestamp,
		})
	}
	return result, nil
}

func funcIncrease(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return extrapolatedRate(matrix, true, false), nil
}

func funcDelta(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return extrapolatedRate(matrix, false, false), nil
}

func funcIdelta(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		if len(samples) < 2 {
			continue
		}
		last := samples[len(samples)-1]
		prev := samples[len(samples)-2]
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     last.Value - prev.Value,
			Timestamp: last.Timestamp,
		})
	}
	return result, nil
}

func extrapolatedRate(matrix Matrix, isCounter bool, isRate bool) Vector {
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		if len(samples) < 2 {
			continue
		}

		first := samples[0]
		last := samples[len(samples)-1]

		counterCorrection := 0.0
		if isCounter {
			for i := 1; i < len(samples); i++ {
				if samples[i].Value < samples[i-1].Value {
					counterCorrection += samples[i-1].Value
				}
			}
		}

		durationToStart := first.Timestamp.Sub(first.Timestamp).Seconds()
		durationToEnd := last.Timestamp.Sub(first.Timestamp).Seconds()
		sampledInterval := last.Timestamp.Sub(first.Timestamp).Seconds()
		averageDuration := sampledInterval / float64(len(samples)-1)

		if isCounter && last.Value < first.Value {
			durationToEnd = averageDuration
		}

		extrapolateToInterval := sampledInterval
		if durationToStart < averageDuration {
			extrapolateToInterval += durationToStart
		} else {
			extrapolateToInterval += averageDuration / 2
		}
		if durationToEnd < averageDuration {
			extrapolateToInterval += durationToEnd
		} else {
			extrapolateToInterval += averageDuration / 2
		}

		factor := extrapolateToInterval / sampledInterval
		if isRate {
			factor /= sampledInterval
		}

		resultVal := (last.Value - first.Value + counterCorrection) * factor

		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     resultVal,
			Timestamp: last.Timestamp,
		})
	}
	return result
}

func funcDeriv(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		if len(samples) < 2 {
			continue
		}
		slope, _ := linearRegression(samples)
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     slope,
			Timestamp: samples[len(samples)-1].Timestamp,
		})
	}
	return result, nil
}

func funcPredictLinear(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	t := args[1].(Scalar).Val
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		if len(samples) < 2 {
			continue
		}
		slope, intercept := linearRegression(samples)
		predicted := slope*t + intercept
		result = append(result, VectorSample{
			Labels:    series.Labels,
			Value:     predicted,
			Timestamp: samples[len(samples)-1].Timestamp,
		})
	}
	return result, nil
}

func linearRegression(samples []Sample) (slope, intercept float64) {
	n := float64(len(samples))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	baseTime := samples[0].Timestamp
	for _, s := range samples {
		x := s.Timestamp.Sub(baseTime).Seconds()
		y := s.Value
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	slope = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	intercept = (sumY - slope*sumX) / n
	return
}

func funcResets(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		resets := 0.0
		for i := 1; i < len(samples); i++ {
			if samples[i].Value < samples[i-1].Value {
				resets++
			}
		}
		if len(samples) > 0 {
			result = append(result, VectorSample{
				Labels:    series.Labels,
				Value:     resets,
				Timestamp: samples[len(samples)-1].Timestamp,
			})
		}
	}
	return result, nil
}

func funcChanges(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		samples := series.Samples
		changes := 0.0
		for i := 1; i < len(samples); i++ {
			if samples[i].Value != samples[i-1].Value {
				changes++
			}
		}
		if len(samples) > 0 {
			result = append(result, VectorSample{
				Labels:    series.Labels,
				Value:     changes,
				Timestamp: samples[len(samples)-1].Timestamp,
			})
		}
	}
	return result, nil
}

// Over time functions

func funcAvgOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		sum := 0.0
		for _, s := range samples {
			sum += s.Value
		}
		return sum / float64(len(samples))
	}), nil
}

func funcSumOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		sum := 0.0
		for _, s := range samples {
			sum += s.Value
		}
		return sum
	}), nil
}

func funcMinOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		min := samples[0].Value
		for _, s := range samples[1:] {
			if s.Value < min {
				min = s.Value
			}
		}
		return min
	}), nil
}

func funcMaxOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		max := samples[0].Value
		for _, s := range samples[1:] {
			if s.Value > max {
				max = s.Value
			}
		}
		return max
	}), nil
}

func funcCountOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		return float64(len(samples))
	}), nil
}

func funcStddevOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) < 2 {
			return math.NaN()
		}
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
		return math.Sqrt(variance / float64(len(samples)))
	}), nil
}

func funcStdvarOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) < 2 {
			return math.NaN()
		}
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
		return variance / float64(len(samples))
	}), nil
}

func funcQuantileOverTime(args []Value, ctx *EvalContext) (Value, error) {
	q := args[0].(Scalar).Val
	matrix := args[1].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		values := make([]float64, len(samples))
		for i, s := range samples {
			values[i] = s.Value
		}
		sort.Float64s(values)
		return quantile(q, values)
	}), nil
}

func funcLastOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		return samples[len(samples)-1].Value
	}), nil
}

func funcPresentOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	return aggregateOverTime(matrix, func(samples []Sample) float64 {
		if len(samples) > 0 {
			return 1
		}
		return math.NaN()
	}), nil
}

func aggregateOverTime(matrix Matrix, f func([]Sample) float64) Vector {
	result := make(Vector, 0, len(matrix))
	for _, series := range matrix {
		val := f(series.Samples)
		if !math.IsNaN(val) {
			ts := time.Time{}
			if len(series.Samples) > 0 {
				ts = series.Samples[len(series.Samples)-1].Timestamp
			}
			result = append(result, VectorSample{
				Labels:    series.Labels,
				Value:     val,
				Timestamp: ts,
			})
		}
	}
	return result
}

func quantile(q float64, values []float64) float64 {
	if len(values) == 0 || math.IsNaN(q) || q < 0 || q > 1 {
		return math.NaN()
	}
	if q == 0 {
		return values[0]
	}
	if q == 1 {
		return values[len(values)-1]
	}
	rank := q * (float64(len(values)) - 1)
	lower := int(rank)
	upper := lower + 1
	if upper >= len(values) {
		return values[len(values)-1]
	}
	frac := rank - float64(lower)
	return values[lower]*(1-frac) + values[upper]*frac
}

func funcHistogramQuantile(args []Value, ctx *EvalContext) (Value, error) {
	q := args[0].(Scalar).Val
	vec := args[1].(Vector)

	// Group by labels excluding 'le'
	groups := make(map[string][]VectorSample)
	for _, s := range vec {
		key := labelsWithout(s.Labels, "le")
		groups[key] = append(groups[key], s)
	}

	result := make(Vector, 0, len(groups))
	for _, buckets := range groups {
		if len(buckets) == 0 {
			continue
		}
		val := histogramQuantile(q, buckets)
		labels := make(map[string]string)
		for k, v := range buckets[0].Labels {
			if k != "le" {
				labels[k] = v
			}
		}
		result = append(result, VectorSample{
			Labels:    labels,
			Value:     val,
			Timestamp: buckets[0].Timestamp,
		})
	}
	return result, nil
}

func labelsWithout(labels map[string]string, exclude string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != exclude {
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

func histogramQuantile(q float64, buckets []VectorSample) float64 {
	if math.IsNaN(q) || q < 0 || q > 1 {
		return math.NaN()
	}

	type bucket struct {
		bound float64
		count float64
	}

	b := make([]bucket, 0, len(buckets))
	for _, s := range buckets {
		le, ok := s.Labels["le"]
		if !ok {
			continue
		}
		var bound float64
		if le == "+Inf" {
			bound = math.Inf(1)
		} else {
			fmt.Sscanf(le, "%f", &bound)
		}
		b = append(b, bucket{bound: bound, count: s.Value})
	}

	sort.Slice(b, func(i, j int) bool { return b[i].bound < b[j].bound })

	if len(b) == 0 {
		return math.NaN()
	}

	total := b[len(b)-1].count
	if total == 0 {
		return math.NaN()
	}

	rank := q * total
	for i, bkt := range b {
		if bkt.count >= rank {
			lower := 0.0
			lowerCount := 0.0
			if i > 0 {
				lower = b[i-1].bound
				lowerCount = b[i-1].count
			}
			if bkt.count == lowerCount {
				return lower
			}
			return lower + (bkt.bound-lower)*(rank-lowerCount)/(bkt.count-lowerCount)
		}
	}
	return b[len(b)-1].bound
}

func funcLabelReplace(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	dst := args[1].(String).Val
	replacement := args[2].(String).Val
	src := args[3].(String).Val
	regexStr := args[4].(String).Val

	result := make(Vector, len(vec))
	for i, s := range vec {
		labels := copyLabels(s.Labels)
		srcVal := labels[src]

		// Simple replacement (full regex support would need regexp package)
		if srcVal == regexStr || regexStr == ".*" {
			labels[dst] = replacement
		}

		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLabelJoin(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	dst := args[1].(String).Val
	sep := args[2].(String).Val

	srcLabels := make([]string, len(args)-3)
	for i := 3; i < len(args); i++ {
		srcLabels[i-3] = args[i].(String).Val
	}

	result := make(Vector, len(vec))
	for i, s := range vec {
		labels := copyLabels(s.Labels)
		var parts []string
		for _, src := range srcLabels {
			if v, ok := labels[src]; ok {
				parts = append(parts, v)
			}
		}
		labels[dst] = strings.Join(parts, sep)
		result[i] = VectorSample{
			Labels:    labels,
			Value:     s.Value,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcAbsent(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	if len(vec) > 0 {
		return Vector{}, nil
	}
	return Vector{{Value: 1, Timestamp: ctx.End}}, nil
}

func funcAbsentOverTime(args []Value, ctx *EvalContext) (Value, error) {
	matrix := args[0].(Matrix)
	if len(matrix) > 0 {
		for _, s := range matrix {
			if len(s.Samples) > 0 {
				return Vector{}, nil
			}
		}
	}
	return Vector{{Value: 1, Timestamp: ctx.End}}, nil
}

func funcAbs(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Abs(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcCeil(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Ceil(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcFloor(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Floor(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcRound(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	toNearest := 1.0
	if len(args) > 1 {
		toNearest = args[1].(Scalar).Val
	}
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Round(s.Value/toNearest) * toNearest,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcSqrt(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Sqrt(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLn(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Log(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLog2(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Log2(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcLog10(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Log10(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcExp(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     math.Exp(s.Value),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcSgn(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		var sgn float64
		if s.Value > 0 {
			sgn = 1
		} else if s.Value < 0 {
			sgn = -1
		}
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     sgn,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcClamp(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	min := args[1].(Scalar).Val
	max := args[2].(Scalar).Val
	result := make(Vector, len(vec))
	for i, s := range vec {
		val := s.Value
		if val < min {
			val = min
		}
		if val > max {
			val = max
		}
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     val,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcClampMin(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	min := args[1].(Scalar).Val
	result := make(Vector, len(vec))
	for i, s := range vec {
		val := s.Value
		if val < min {
			val = min
		}
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     val,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcClampMax(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	max := args[1].(Scalar).Val
	result := make(Vector, len(vec))
	for i, s := range vec {
		val := s.Value
		if val > max {
			val = max
		}
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     val,
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcScalar(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	if len(vec) != 1 {
		return Scalar{Val: math.NaN(), Timestamp: ctx.End}, nil
	}
	return Scalar{Val: vec[0].Value, Timestamp: vec[0].Timestamp}, nil
}

func funcVector(args []Value, ctx *EvalContext) (Value, error) {
	s := args[0].(Scalar)
	return Vector{{Value: s.Val, Timestamp: s.Timestamp}}, nil
}

func funcTime(args []Value, ctx *EvalContext) (Value, error) {
	return Scalar{Val: float64(ctx.End.Unix()), Timestamp: ctx.End}, nil
}

func funcTimestamp(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Unix()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcDayOfMonth(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Day()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcDayOfWeek(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Weekday()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcDayOfYear(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.YearDay()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcDaysInMonth(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		year, month, _ := s.Timestamp.Date()
		nextMonth := time.Date(year, month+1, 1, 0, 0, 0, 0, s.Timestamp.Location())
		days := nextMonth.AddDate(0, 0, -1).Day()
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(days),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcHour(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Hour()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcMinute(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Minute()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcMonth(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Month()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcYear(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	for i, s := range vec {
		result[i] = VectorSample{
			Labels:    s.Labels,
			Value:     float64(s.Timestamp.Year()),
			Timestamp: s.Timestamp,
		}
	}
	return result, nil
}

func funcSort(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	copy(result, vec)
	sort.Slice(result, func(i, j int) bool { return result[i].Value < result[j].Value })
	return result, nil
}

func funcSortDesc(args []Value, ctx *EvalContext) (Value, error) {
	vec := args[0].(Vector)
	result := make(Vector, len(vec))
	copy(result, vec)
	sort.Slice(result, func(i, j int) bool { return result[i].Value > result[j].Value })
	return result, nil
}

func funcPi(args []Value, ctx *EvalContext) (Value, error) {
	return Scalar{Val: math.Pi, Timestamp: ctx.End}, nil
}

func copyLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		result[k] = v
	}
	return result
}
