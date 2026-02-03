package promql

import (
	"context"
	"testing"
	"time"
)

// TestLexer tests the PromQL lexer
func TestLexer(t *testing.T) {
	tests := []struct {
		input    string
		expected []ItemType
	}{
		{
			input:    "up",
			expected: []ItemType{ItemIdentifier, ItemEOF},
		},
		{
			input:    "rate(http_requests_total[5m])",
			expected: []ItemType{ItemIdentifier, ItemLeftParen, ItemIdentifier, ItemLeftBracket, ItemDuration, ItemRightBracket, ItemRightParen, ItemEOF},
		},
		{
			input:    "sum by (job) (rate(http_requests_total[5m]))",
			expected: []ItemType{ItemSum, ItemBy, ItemLeftParen, ItemIdentifier, ItemRightParen, ItemLeftParen, ItemIdentifier, ItemLeftParen, ItemIdentifier, ItemLeftBracket, ItemDuration, ItemRightBracket, ItemRightParen, ItemRightParen, ItemEOF},
		},
		{
			input:    `metric{job="api",code="200"}`,
			expected: []ItemType{ItemIdentifier, ItemLeftBrace, ItemIdentifier, ItemAssign, ItemString, ItemComma, ItemIdentifier, ItemAssign, ItemString, ItemRightBrace, ItemEOF},
		},
		{
			input:    "1 + 2 * 3",
			expected: []ItemType{ItemNumber, ItemAdd, ItemNumber, ItemMul, ItemNumber, ItemEOF},
		},
		{
			input:    "metric == 1",
			expected: []ItemType{ItemIdentifier, ItemEql, ItemNumber, ItemEOF},
		},
		{
			input:    `metric{job=~"api.*"}`,
			expected: []ItemType{ItemIdentifier, ItemLeftBrace, ItemIdentifier, ItemEqlMatch, ItemString, ItemRightBrace, ItemEOF},
		},
		{
			input:    "topk(5, metric)",
			expected: []ItemType{ItemTopK, ItemLeftParen, ItemNumber, ItemComma, ItemIdentifier, ItemRightParen, ItemEOF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			items := Lex(tt.input)
			if len(items) != len(tt.expected) {
				t.Errorf("expected %d tokens, got %d", len(tt.expected), len(items))
				for i, item := range items {
					t.Logf("  [%d] %s: %q", i, item.Typ, item.Val)
				}
				return
			}
			for i, item := range items {
				if item.Typ != tt.expected[i] {
					t.Errorf("token %d: expected %s, got %s (%q)", i, tt.expected[i], item.Typ, item.Val)
				}
			}
		})
	}
}

// TestParser tests the PromQL parser
func TestParser(t *testing.T) {
	tests := []struct {
		input string
		check func(t *testing.T, expr Expr)
	}{
		{
			input: "up",
			check: func(t *testing.T, expr Expr) {
				vs, ok := expr.(*VectorSelector)
				if !ok {
					t.Fatalf("expected VectorSelector, got %T", expr)
				}
				if vs.Name != "up" {
					t.Errorf("expected name 'up', got %q", vs.Name)
				}
			},
		},
		{
			input: "42",
			check: func(t *testing.T, expr Expr) {
				nl, ok := expr.(*NumberLiteral)
				if !ok {
					t.Fatalf("expected NumberLiteral, got %T", expr)
				}
				if nl.Val != 42 {
					t.Errorf("expected 42, got %f", nl.Val)
				}
			},
		},
		{
			input: `"hello"`,
			check: func(t *testing.T, expr Expr) {
				sl, ok := expr.(*StringLiteral)
				if !ok {
					t.Fatalf("expected StringLiteral, got %T", expr)
				}
				if sl.Val != "hello" {
					t.Errorf("expected 'hello', got %q", sl.Val)
				}
			},
		},
		{
			input: "http_requests_total[5m]",
			check: func(t *testing.T, expr Expr) {
				ms, ok := expr.(*MatrixSelector)
				if !ok {
					t.Fatalf("expected MatrixSelector, got %T", expr)
				}
				if ms.Range != 5*time.Minute {
					t.Errorf("expected 5m range, got %s", ms.Range)
				}
				if ms.VectorSelector.Name != "http_requests_total" {
					t.Errorf("expected 'http_requests_total', got %q", ms.VectorSelector.Name)
				}
			},
		},
		{
			input: `metric{job="api",code="200"}`,
			check: func(t *testing.T, expr Expr) {
				vs, ok := expr.(*VectorSelector)
				if !ok {
					t.Fatalf("expected VectorSelector, got %T", expr)
				}
				if len(vs.LabelMatchers) != 2 {
					t.Fatalf("expected 2 label matchers, got %d", len(vs.LabelMatchers))
				}
				if vs.LabelMatchers[0].Name != "job" || vs.LabelMatchers[0].Value != "api" {
					t.Errorf("first matcher: expected job=api, got %s=%s", vs.LabelMatchers[0].Name, vs.LabelMatchers[0].Value)
				}
				if vs.LabelMatchers[1].Name != "code" || vs.LabelMatchers[1].Value != "200" {
					t.Errorf("second matcher: expected code=200, got %s=%s", vs.LabelMatchers[1].Name, vs.LabelMatchers[1].Value)
				}
			},
		},
		{
			input: "1 + 2",
			check: func(t *testing.T, expr Expr) {
				be, ok := expr.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
				if be.Op != OpAdd {
					t.Errorf("expected OpAdd, got %s", be.Op)
				}
			},
		},
		{
			input: "1 + 2 * 3",
			check: func(t *testing.T, expr Expr) {
				be, ok := expr.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
				if be.Op != OpAdd {
					t.Errorf("expected OpAdd at top level, got %s", be.Op)
				}
				rhs, ok := be.RHS.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr on RHS, got %T", be.RHS)
				}
				if rhs.Op != OpMul {
					t.Errorf("expected OpMul on RHS, got %s", rhs.Op)
				}
			},
		},
		{
			input: "-metric",
			check: func(t *testing.T, expr Expr) {
				ue, ok := expr.(*UnaryExpr)
				if !ok {
					t.Fatalf("expected UnaryExpr, got %T", expr)
				}
				if ue.Op != ItemSub {
					t.Errorf("expected ItemSub, got %s", ue.Op)
				}
			},
		},
		{
			input: "rate(http_requests_total[5m])",
			check: func(t *testing.T, expr Expr) {
				call, ok := expr.(*Call)
				if !ok {
					t.Fatalf("expected Call, got %T", expr)
				}
				if call.Func.Name != "rate" {
					t.Errorf("expected 'rate', got %q", call.Func.Name)
				}
				if len(call.Args) != 1 {
					t.Fatalf("expected 1 arg, got %d", len(call.Args))
				}
				_, ok = call.Args[0].(*MatrixSelector)
				if !ok {
					t.Fatalf("expected MatrixSelector arg, got %T", call.Args[0])
				}
			},
		},
		{
			input: "sum by (job) (rate(http_requests_total[5m]))",
			check: func(t *testing.T, expr Expr) {
				ae, ok := expr.(*AggregateExpr)
				if !ok {
					t.Fatalf("expected AggregateExpr, got %T", expr)
				}
				if ae.Op != ItemSum {
					t.Errorf("expected ItemSum, got %s", ae.Op)
				}
				if len(ae.Grouping) != 1 || ae.Grouping[0] != "job" {
					t.Errorf("expected grouping [job], got %v", ae.Grouping)
				}
				if ae.Without {
					t.Error("expected by, got without")
				}
			},
		},
		{
			input: "sum without (instance) (rate(http_requests_total[5m]))",
			check: func(t *testing.T, expr Expr) {
				ae, ok := expr.(*AggregateExpr)
				if !ok {
					t.Fatalf("expected AggregateExpr, got %T", expr)
				}
				if !ae.Without {
					t.Error("expected without, got by")
				}
			},
		},
		{
			input: "topk(5, http_requests_total)",
			check: func(t *testing.T, expr Expr) {
				ae, ok := expr.(*AggregateExpr)
				if !ok {
					t.Fatalf("expected AggregateExpr, got %T", expr)
				}
				if ae.Op != ItemTopK {
					t.Errorf("expected ItemTopK, got %s", ae.Op)
				}
				if ae.Param == nil {
					t.Fatal("expected param")
				}
				param, ok := ae.Param.(*NumberLiteral)
				if !ok {
					t.Fatalf("expected NumberLiteral param, got %T", ae.Param)
				}
				if param.Val != 5 {
					t.Errorf("expected 5, got %f", param.Val)
				}
			},
		},
		{
			input: "histogram_quantile(0.99, sum by (le) (rate(request_latency_bucket[5m])))",
			check: func(t *testing.T, expr Expr) {
				call, ok := expr.(*Call)
				if !ok {
					t.Fatalf("expected Call, got %T", expr)
				}
				if call.Func.Name != "histogram_quantile" {
					t.Errorf("expected 'histogram_quantile', got %q", call.Func.Name)
				}
				if len(call.Args) != 2 {
					t.Fatalf("expected 2 args, got %d", len(call.Args))
				}
			},
		},
		{
			input: "metric1 * on(job) group_left(instance) metric2",
			check: func(t *testing.T, expr Expr) {
				be, ok := expr.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
				if be.VectorMatching == nil {
					t.Fatal("expected vector matching")
				}
				if !be.VectorMatching.On {
					t.Error("expected on(), got ignoring()")
				}
				if len(be.VectorMatching.MatchingLabels) != 1 || be.VectorMatching.MatchingLabels[0] != "job" {
					t.Errorf("expected matching labels [job], got %v", be.VectorMatching.MatchingLabels)
				}
				if be.VectorMatching.Card != CardManyToOne {
					t.Errorf("expected CardManyToOne, got %s", be.VectorMatching.Card)
				}
				if len(be.VectorMatching.Include) != 1 || be.VectorMatching.Include[0] != "instance" {
					t.Errorf("expected include [instance], got %v", be.VectorMatching.Include)
				}
			},
		},
		{
			input: "metric offset 5m",
			check: func(t *testing.T, expr Expr) {
				vs, ok := expr.(*VectorSelector)
				if !ok {
					t.Fatalf("expected VectorSelector, got %T", expr)
				}
				if vs.Offset != 5*time.Minute {
					t.Errorf("expected offset 5m, got %s", vs.Offset)
				}
			},
		},
		{
			input: "metric > 100",
			check: func(t *testing.T, expr Expr) {
				be, ok := expr.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
				if be.Op != OpGtr {
					t.Errorf("expected OpGtr, got %s", be.Op)
				}
			},
		},
		{
			input: "metric > bool 100",
			check: func(t *testing.T, expr Expr) {
				be, ok := expr.(*BinaryExpr)
				if !ok {
					t.Fatalf("expected BinaryExpr, got %T", expr)
				}
				if !be.ReturnBool {
					t.Error("expected ReturnBool to be true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			tt.check(t, expr)
		})
	}
}

// TestParseDuration tests duration parsing
func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"30s", 30 * time.Second},
		{"1d", 24 * time.Hour},
		{"1w", 7 * 24 * time.Hour},
		{"1h30m", 90 * time.Minute},
		{"2d12h", 60 * time.Hour},
		{"100ms", 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := ParseDuration(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if d != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, d)
			}
		})
	}
}

// TestFunctions tests PromQL function implementations
func TestFunctions(t *testing.T) {
	ctx := &EvalContext{
		Start: time.Now().Add(-5 * time.Minute),
		End:   time.Now(),
	}

	t.Run("rate", func(t *testing.T) {
		// Create a matrix with counter values
		matrix := Matrix{
			Series{
				Labels: map[string]string{"job": "api"},
				Samples: []Sample{
					{Timestamp: time.Now().Add(-4 * time.Minute), Value: 100},
					{Timestamp: time.Now().Add(-3 * time.Minute), Value: 200},
					{Timestamp: time.Now().Add(-2 * time.Minute), Value: 300},
					{Timestamp: time.Now().Add(-1 * time.Minute), Value: 400},
					{Timestamp: time.Now(), Value: 500},
				},
			},
		}

		result, err := funcRate([]Value{matrix}, ctx)
		if err != nil {
			t.Fatalf("rate error: %v", err)
		}

		vec, ok := result.(Vector)
		if !ok {
			t.Fatalf("expected Vector, got %T", result)
		}
		if len(vec) != 1 {
			t.Fatalf("expected 1 sample, got %d", len(vec))
		}
		// Rate should be approximately 100/min = 1.67/s
		if vec[0].Value < 1 || vec[0].Value > 2 {
			t.Errorf("unexpected rate value: %f", vec[0].Value)
		}
	})

	t.Run("sum_over_time", func(t *testing.T) {
		matrix := Matrix{
			Series{
				Labels: map[string]string{"job": "api"},
				Samples: []Sample{
					{Timestamp: time.Now(), Value: 10},
					{Timestamp: time.Now(), Value: 20},
					{Timestamp: time.Now(), Value: 30},
				},
			},
		}

		result, err := funcSumOverTime([]Value{matrix}, ctx)
		if err != nil {
			t.Fatalf("sum_over_time error: %v", err)
		}

		vec, ok := result.(Vector)
		if !ok {
			t.Fatalf("expected Vector, got %T", result)
		}
		if len(vec) != 1 {
			t.Fatalf("expected 1 sample, got %d", len(vec))
		}
		if vec[0].Value != 60 {
			t.Errorf("expected 60, got %f", vec[0].Value)
		}
	})

	t.Run("avg_over_time", func(t *testing.T) {
		matrix := Matrix{
			Series{
				Labels: map[string]string{"job": "api"},
				Samples: []Sample{
					{Timestamp: time.Now(), Value: 10},
					{Timestamp: time.Now(), Value: 20},
					{Timestamp: time.Now(), Value: 30},
				},
			},
		}

		result, err := funcAvgOverTime([]Value{matrix}, ctx)
		if err != nil {
			t.Fatalf("avg_over_time error: %v", err)
		}

		vec, ok := result.(Vector)
		if !ok {
			t.Fatalf("expected Vector, got %T", result)
		}
		if vec[0].Value != 20 {
			t.Errorf("expected 20, got %f", vec[0].Value)
		}
	})

	t.Run("abs", func(t *testing.T) {
		vec := Vector{
			{Labels: map[string]string{}, Value: -5, Timestamp: time.Now()},
			{Labels: map[string]string{}, Value: 10, Timestamp: time.Now()},
		}

		result, err := funcAbs([]Value{vec}, ctx)
		if err != nil {
			t.Fatalf("abs error: %v", err)
		}

		resVec := result.(Vector)
		if resVec[0].Value != 5 {
			t.Errorf("expected 5, got %f", resVec[0].Value)
		}
		if resVec[1].Value != 10 {
			t.Errorf("expected 10, got %f", resVec[1].Value)
		}
	})

	t.Run("clamp", func(t *testing.T) {
		vec := Vector{
			{Labels: map[string]string{}, Value: 5, Timestamp: time.Now()},
			{Labels: map[string]string{}, Value: 50, Timestamp: time.Now()},
			{Labels: map[string]string{}, Value: 150, Timestamp: time.Now()},
		}

		result, err := funcClamp([]Value{vec, Scalar{Val: 10}, Scalar{Val: 100}}, ctx)
		if err != nil {
			t.Fatalf("clamp error: %v", err)
		}

		resVec := result.(Vector)
		if resVec[0].Value != 10 {
			t.Errorf("expected 10, got %f", resVec[0].Value)
		}
		if resVec[1].Value != 50 {
			t.Errorf("expected 50, got %f", resVec[1].Value)
		}
		if resVec[2].Value != 100 {
			t.Errorf("expected 100, got %f", resVec[2].Value)
		}
	})
}

// TestEvaluator tests the PromQL evaluator
func TestEvaluator(t *testing.T) {
	// Create a mock store
	store := &mockMetricsStore{
		series: []Series{
			{
				Labels: map[string]string{"__name__": "up", "job": "api", "instance": "localhost:9090"},
				Samples: []Sample{
					{Timestamp: time.Now(), Value: 1},
				},
			},
			{
				Labels: map[string]string{"__name__": "up", "job": "api", "instance": "localhost:9091"},
				Samples: []Sample{
					{Timestamp: time.Now(), Value: 1},
				},
			},
			{
				Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "GET"},
				Samples: []Sample{
					{Timestamp: time.Now().Add(-4 * time.Minute), Value: 100},
					{Timestamp: time.Now().Add(-3 * time.Minute), Value: 200},
					{Timestamp: time.Now().Add(-2 * time.Minute), Value: 300},
					{Timestamp: time.Now().Add(-1 * time.Minute), Value: 400},
					{Timestamp: time.Now(), Value: 500},
				},
			},
		},
	}

	eval := NewEvaluator(store)
	ctx := context.Background()

	t.Run("vector selector", func(t *testing.T) {
		opts := &EvalOptions{
			Start: time.Now().Add(-5 * time.Minute),
			End:   time.Now(),
		}

		expr, _ := Parse("up")
		result, err := eval.Eval(ctx, expr, opts)
		if err != nil {
			t.Fatalf("eval error: %v", err)
		}

		vec, ok := result.(Vector)
		if !ok {
			t.Fatalf("expected Vector, got %T", result)
		}
		if len(vec) != 2 {
			t.Errorf("expected 2 samples, got %d", len(vec))
		}
	})

	t.Run("binary arithmetic", func(t *testing.T) {
		opts := &EvalOptions{End: time.Now()}

		expr, _ := Parse("1 + 2 * 3")
		result, err := eval.Eval(ctx, expr, opts)
		if err != nil {
			t.Fatalf("eval error: %v", err)
		}

		scalar, ok := result.(Scalar)
		if !ok {
			t.Fatalf("expected Scalar, got %T", result)
		}
		if scalar.Val != 7 {
			t.Errorf("expected 7, got %f", scalar.Val)
		}
	})

	t.Run("aggregation", func(t *testing.T) {
		opts := &EvalOptions{
			Start: time.Now().Add(-5 * time.Minute),
			End:   time.Now(),
		}

		expr, _ := Parse("count(up)")
		result, err := eval.Eval(ctx, expr, opts)
		if err != nil {
			t.Fatalf("eval error: %v", err)
		}

		vec, ok := result.(Vector)
		if !ok {
			t.Fatalf("expected Vector, got %T", result)
		}
		if len(vec) != 1 {
			t.Fatalf("expected 1 sample, got %d", len(vec))
		}
		if vec[0].Value != 2 {
			t.Errorf("expected 2, got %f", vec[0].Value)
		}
	})
}

// mockMetricsStore is a mock implementation of MetricsStore for testing
type mockMetricsStore struct {
	series []Series
}

func (m *mockMetricsStore) ScanRange(ctx context.Context, metric string, labels map[string]string, matchers []*LabelMatcher, start, end time.Time) ([]Series, error) {
	var result []Series
	for _, s := range m.series {
		name := s.Labels["__name__"]
		if metric != "" && name != metric {
			continue
		}
		if !matchLabels(s.Labels, matchers) {
			continue
		}
		// Filter samples by time range
		filtered := Series{Labels: s.Labels}
		for _, sample := range s.Samples {
			if !sample.Timestamp.Before(start) && !sample.Timestamp.After(end) {
				filtered.Samples = append(filtered.Samples, sample)
			}
		}
		if len(filtered.Samples) > 0 {
			result = append(result, filtered)
		}
	}
	return result, nil
}

func (m *mockMetricsStore) ListMetrics(ctx context.Context) ([]string, error) {
	seen := make(map[string]bool)
	var result []string
	for _, s := range m.series {
		name := s.Labels["__name__"]
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result, nil
}

func (m *mockMetricsStore) ListLabels(ctx context.Context, metric string) ([]string, error) {
	seen := make(map[string]bool)
	for _, s := range m.series {
		if metric != "" && s.Labels["__name__"] != metric {
			continue
		}
		for k := range s.Labels {
			seen[k] = true
		}
	}
	var result []string
	for k := range seen {
		result = append(result, k)
	}
	return result, nil
}

func (m *mockMetricsStore) ListLabelValues(ctx context.Context, label string, metric string) ([]string, error) {
	seen := make(map[string]bool)
	for _, s := range m.series {
		if metric != "" && s.Labels["__name__"] != metric {
			continue
		}
		if v, ok := s.Labels[label]; ok {
			seen[v] = true
		}
	}
	var result []string
	for v := range seen {
		result = append(result, v)
	}
	return result, nil
}

// TestEngine tests the PromQL engine
func TestEngine(t *testing.T) {
	store := &mockMetricsStore{
		series: []Series{
			{
				Labels: map[string]string{"__name__": "up", "job": "api"},
				Samples: []Sample{
					{Timestamp: time.Now(), Value: 1},
				},
			},
		},
	}

	engine := NewEngine(store)
	ctx := context.Background()

	t.Run("instant query", func(t *testing.T) {
		result, err := engine.Query(ctx, "up", time.Now())
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
		if result.ResultType != "vector" {
			t.Errorf("expected vector, got %s", result.ResultType)
		}
	})

	t.Run("range query", func(t *testing.T) {
		end := time.Now()
		start := end.Add(-5 * time.Minute)
		step := time.Minute

		result, err := engine.QueryRange(ctx, "up", start, end, step)
		if err != nil {
			t.Fatalf("query range error: %v", err)
		}
		if result.ResultType != "matrix" {
			t.Errorf("expected matrix, got %s", result.ResultType)
		}
	})
}

// TestParseTime tests time parsing
func TestParseTime(t *testing.T) {
	tests := []struct {
		input string
		check func(t *testing.T, ts time.Time)
	}{
		{
			input: "1609459200",
			check: func(t *testing.T, ts time.Time) {
				if ts.Unix() != 1609459200 {
					t.Errorf("expected 1609459200, got %d", ts.Unix())
				}
			},
		},
		{
			input: "1609459200.5",
			check: func(t *testing.T, ts time.Time) {
				if ts.Unix() != 1609459200 {
					t.Errorf("expected 1609459200, got %d", ts.Unix())
				}
			},
		},
		{
			input: "2021-01-01T00:00:00Z",
			check: func(t *testing.T, ts time.Time) {
				if ts.Year() != 2021 || ts.Month() != 1 || ts.Day() != 1 {
					t.Errorf("unexpected date: %v", ts)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ts, err := ParseTime(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			tt.check(t, ts)
		})
	}
}

// TestFormatQueryResult tests result formatting
func TestFormatQueryResult(t *testing.T) {
	t.Run("vector", func(t *testing.T) {
		result := &QueryResult{
			ResultType: "vector",
			Result: Vector{
				{Labels: map[string]string{"job": "api"}, Value: 42, Timestamp: time.Unix(1609459200, 0)},
			},
		}

		resp := FormatQueryResult(result)
		if resp.Status != "success" {
			t.Errorf("expected success, got %s", resp.Status)
		}

		data, ok := resp.Data.(*QueryData)
		if !ok {
			t.Fatalf("expected QueryData, got %T", resp.Data)
		}
		if data.ResultType != "vector" {
			t.Errorf("expected vector, got %s", data.ResultType)
		}
		if len(data.Result) != 1 {
			t.Errorf("expected 1 result, got %d", len(data.Result))
		}
	})

	t.Run("scalar", func(t *testing.T) {
		result := &QueryResult{
			ResultType: "scalar",
			Result:     Scalar{Val: 42, Timestamp: time.Unix(1609459200, 0)},
		}

		resp := FormatQueryResult(result)
		if resp.Status != "success" {
			t.Errorf("expected success, got %s", resp.Status)
		}
	})
}
