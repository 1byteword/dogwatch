package recording

import (
	"regexp"
	"testing"
	"time"
)

// Helper to create test recording rules
func createTestRules() []RecordingRule {
	return []RecordingRule{
		{
			ID:         "test:request_rate:1m",
			Name:       "service:request_rate:1m",
			Expression: "SELECT service, count(*) / 60.0 as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service",
			Interval:   time.Minute,
			Enabled:    true,
			Labels: map[string]string{
				"__name__": "service:request_rate:1m",
				"source":   "recording_rule",
			},
		},
		{
			ID:         "test:error_rate:1m",
			Name:       "service:error_rate:1m",
			Expression: "rate(errors_total[5m]) by (service)",
			Interval:   time.Minute,
			Enabled:    true,
			Labels: map[string]string{
				"__name__": "service:error_rate:1m",
			},
		},
		{
			ID:         "test:latency_p99:1m",
			Name:       "service:latency_p99:1m",
			Expression: "histogram_quantile(0.99, request_duration) by (service)",
			Interval:   time.Minute,
			Enabled:    true,
			Labels:     map[string]string{},
		},
		{
			ID:         "test:sum_by_service",
			Name:       "service:total_requests",
			Expression: "sum(requests_total) by (service, method)",
			Interval:   time.Minute,
			Enabled:    true,
			Labels:     map[string]string{},
		},
		{
			ID:         "test:avg_by_service",
			Name:       "service:avg_duration",
			Expression: "avg(duration_ms) by (service)",
			Interval:   time.Minute,
			Enabled:    true,
			Labels:     map[string]string{},
		},
	}
}

func BenchmarkRuleEvaluation(b *testing.B) {
	// Note: This benchmarks the evaluation logic without an actual executor
	// In production, the executor would be the bottleneck
	rules := createTestRules()

	b.Run("DQLExpression", func(b *testing.B) {
		rule := rules[0] // DQL-style
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = isPromQLStyle(rule.Expression)
		}
	})

	b.Run("PromQLExpression", func(b *testing.B) {
		rule := rules[1] // PromQL-style
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = isPromQLStyle(rule.Expression)
		}
	})
}

func BenchmarkExpressionParsing(b *testing.B) {
	expressions := []struct {
		name string
		expr string
	}{
		{"DQL_Simple", "SELECT count(*) FROM logs"},
		{"DQL_GroupBy", "SELECT service, count(*) FROM traces GROUP BY service"},
		{"DQL_Complex", "SELECT service, count(*) / 60.0 as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service"},
		{"PromQL_Rate", "rate(http_requests_total[5m])"},
		{"PromQL_Sum", "sum(requests_total) by (service)"},
		{"PromQL_Histogram", "histogram_quantile(0.99, http_request_duration_bucket)"},
		{"PromQL_Complex", "sum(rate(errors_total[5m])) by (service) / sum(rate(requests_total[5m])) by (service)"},
	}

	for _, tt := range expressions {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = isPromQLStyle(tt.expr)
			}
		})
	}
}

func BenchmarkIsPromQLStyle(b *testing.B) {
	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{"rate", "rate(metric[5m])", true},
		{"sum", "sum(metric) by (label)", true},
		{"avg", "avg(metric) by (label)", true},
		{"count", "count(metric)", true},
		{"histogram", "histogram_quantile(0.99, metric)", true},
		{"SELECT", "SELECT * FROM logs", false},
		{"Plain", "metric_name", false},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := isPromQLStyle(tt.expr)
				if result != tt.expected {
					b.Errorf("isPromQLStyle(%q) = %v, want %v", tt.expr, result, tt.expected)
				}
			}
		})
	}
}

func BenchmarkDurationParsing(b *testing.B) {
	durations := []string{"5s", "1m", "5m", "1h", "24h", "7d"}

	for _, d := range durations {
		b.Run(d, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = durationToSeconds(d)
			}
		})
	}
}

func BenchmarkExpandTimeExpressions(b *testing.B) {
	expressions := []string{
		"timestamp >= '2024-01-01T00:00:00Z' - 1h",
		"timestamp >= '2024-01-01T00:00:00Z' - 5m",
		"timestamp >= '2024-01-01T00:00:00Z' - 24h",
		"timestamp BETWEEN '2024-01-01T00:00:00Z' - 1d AND '2024-01-01T00:00:00Z'",
	}

	for i, expr := range expressions {
		b.Run("Expr"+string(rune('A'+i)), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				_ = expandTimeExpressions(expr)
			}
		})
	}
}

func BenchmarkToFloat(b *testing.B) {
	values := []struct {
		name string
		val  interface{}
	}{
		{"Float64", float64(123.456)},
		{"Float32", float32(123.456)},
		{"Int", int(123)},
		{"Int64", int64(123)},
		{"Int32", int32(123)},
		{"String", "123.456"},
		{"StringInt", "123"},
		{"InvalidString", "not-a-number"},
		{"Nil", nil},
	}

	for _, tt := range values {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = toFloat(tt.val)
			}
		})
	}
}

// Benchmark regex pattern matching used in PromQL parsing
func BenchmarkPromQLPatternMatching(b *testing.B) {
	patterns := []struct {
		name    string
		pattern string
		input   string
	}{
		{
			"Rate",
			`rate\((\w+)\[(\d+[smhd])\]\)(?:\s+by\s+\(([^)]+)\))?`,
			"rate(http_requests_total[5m]) by (service, method)",
		},
		{
			"Sum",
			`sum\((\w+)\)(?:\s+by\s+\(([^)]+)\))?`,
			"sum(requests_total) by (service)",
		},
		{
			"Histogram",
			`histogram_quantile\(([\d.]+),\s*(\w+)\)(?:\s+by\s+\(([^)]+)\))?`,
			"histogram_quantile(0.99, http_request_duration_bucket) by (le)",
		},
		{
			"Avg",
			`avg\((\w+)\)(?:\s+by\s+\(([^)]+)\))?`,
			"avg(response_time) by (endpoint)",
		},
	}

	for _, tt := range patterns {
		re := regexp.MustCompile(tt.pattern)
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = re.FindStringSubmatch(tt.input)
			}
		})
	}
}

func BenchmarkRuleCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = RecordingRule{
			ID:         "test:metric:1m",
			Name:       "service:metric:1m",
			Expression: "SELECT service, count(*) FROM traces GROUP BY service",
			Interval:   time.Minute,
			Enabled:    true,
			Labels: map[string]string{
				"__name__": "service:metric:1m",
				"source":   "recording_rule",
			},
			Description: "Test rule for benchmarking",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}
}

func BenchmarkDefaultRules(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = DefaultRules()
	}
}

func BenchmarkResultValueCreation(b *testing.B) {
	labels := map[string]string{
		"service": "api-gateway",
		"method":  "GET",
		"status":  "200",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rv := ResultValue{
			Value:  123.456,
			Labels: make(map[string]string, len(labels)),
		}
		for k, v := range labels {
			rv.Labels[k] = v
		}
	}
}

// Benchmark concurrent rule checking
func BenchmarkConcurrentIsPromQLStyle(b *testing.B) {
	expressions := []string{
		"rate(metric[5m])",
		"sum(metric) by (label)",
		"SELECT * FROM logs",
		"histogram_quantile(0.99, metric)",
		"avg(duration) by (service)",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = isPromQLStyle(expressions[i%len(expressions)])
			i++
		}
	})
}

// Table-driven benchmark for various expression complexities
func BenchmarkExpressionComplexity(b *testing.B) {
	complexities := []struct {
		name string
		expr string
	}{
		{"Simple", "count(*)"},
		{"Medium", "sum(rate(requests[5m])) by (service)"},
		{"Complex", "sum(rate(http_requests_total{status=~\"5..\"}[5m])) by (service) / sum(rate(http_requests_total[5m])) by (service) * 100"},
	}

	for _, c := range complexities {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = isPromQLStyle(c.expr)
			}
		})
	}
}

func BenchmarkEvaluationResultCreation(b *testing.B) {
	values := []ResultValue{
		{Value: 100.0, Labels: map[string]string{"service": "api"}},
		{Value: 200.0, Labels: map[string]string{"service": "web"}},
		{Value: 50.0, Labels: map[string]string{"service": "worker"}},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result := &EvaluationResult{
			Values:   make([]ResultValue, len(values)),
			Duration: 100 * time.Millisecond,
		}
		copy(result.Values, values)
	}
}
