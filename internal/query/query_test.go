package query

import (
	"testing"
	"time"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{
			input:    "SELECT * FROM logs",
			expected: []TokenType{TokenSelect, TokenStar, TokenFrom, TokenLogs, TokenEOF},
		},
		{
			input:    "logs | where service = 'api'",
			expected: []TokenType{TokenLogs, TokenPipe, TokenWhere, TokenIdent, TokenEq, TokenString, TokenEOF},
		},
		{
			input:    "LEFT JOIN traces ON t.id = l.trace_id",
			expected: []TokenType{TokenLeft, TokenJoin, TokenTraces, TokenOn, TokenIdent, TokenDot, TokenIdent, TokenEq, TokenIdent, TokenDot, TokenIdent, TokenEOF},
		},
		{
			input:    "GROUP BY service HAVING count(*) > 10",
			expected: []TokenType{TokenGroup, TokenBy, TokenIdent, TokenHaving, TokenCount, TokenLParen, TokenStar, TokenRParen, TokenGt, TokenNumber, TokenEOF},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			if err != nil {
				t.Fatalf("lexer error: %v", err)
			}

			if len(tokens) != len(tt.expected) {
				t.Errorf("expected %d tokens, got %d", len(tt.expected), len(tokens))
				for i, tok := range tokens {
					t.Logf("  token %d: %v", i, tok)
				}
				return
			}

			for i, expected := range tt.expected {
				if tokens[i].Type != expected {
					t.Errorf("token %d: expected %v, got %v", i, expected, tokens[i].Type)
				}
			}
		})
	}
}

func TestParsePipeQuery(t *testing.T) {
	tests := []struct {
		input string
		check func(*Query) error
	}{
		{
			input: "logs | where service = 'api'",
			check: func(q *Query) error {
				if q.Source.SourceType != "logs" {
					return errorf("expected source 'logs', got '%s'", q.Source.SourceType)
				}
				if len(q.Pipes) != 1 {
					return errorf("expected 1 pipe, got %d", len(q.Pipes))
				}
				if _, ok := q.Pipes[0].(*WhereNode); !ok {
					return errorf("expected WhereNode, got %T", q.Pipes[0])
				}
				return nil
			},
		},
		{
			input: "metrics | avg(latency) by (service)",
			check: func(q *Query) error {
				if q.Source.SourceType != "metrics" {
					return errorf("expected source 'metrics', got '%s'", q.Source.SourceType)
				}
				if len(q.Pipes) != 1 {
					return errorf("expected 1 pipe, got %d", len(q.Pipes))
				}
				gb, ok := q.Pipes[0].(*GroupByNode)
				if !ok {
					return errorf("expected GroupByNode, got %T", q.Pipes[0])
				}
				if len(gb.Aggregations) != 1 || gb.Aggregations[0].Function != "avg" {
					return errorf("expected avg aggregation")
				}
				if len(gb.GroupFields) != 1 || gb.GroupFields[0] != "service" {
					return errorf("expected group by service")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := tt.check(q); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestParseSQLQuery(t *testing.T) {
	tests := []struct {
		input string
		check func(*Query) error
	}{
		{
			input: "SELECT * FROM logs",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if q.SQL.From.Source.SourceType != "logs" {
					return errorf("expected FROM logs, got %s", q.SQL.From.Source.SourceType)
				}
				return nil
			},
		},
		{
			input: "SELECT service, count(*) as cnt FROM logs WHERE status = 'error' GROUP BY service",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if len(q.SQL.Select.Fields) != 2 {
					return errorf("expected 2 select fields, got %d", len(q.SQL.Select.Fields))
				}
				if q.SQL.Where == nil {
					return errorf("expected WHERE clause")
				}
				if len(q.SQL.GroupBy) != 1 {
					return errorf("expected 1 group by field, got %d", len(q.SQL.GroupBy))
				}
				return nil
			},
		},
		{
			input: "SELECT DISTINCT service FROM logs",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if !q.SQL.Select.Distinct {
					return errorf("expected DISTINCT")
				}
				return nil
			},
		},
		{
			input: "SELECT * FROM traces t LEFT JOIN logs l ON l.trace_id = t.trace_id",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if len(q.SQL.Joins) != 1 {
					return errorf("expected 1 join, got %d", len(q.SQL.Joins))
				}
				if q.SQL.Joins[0].JoinType != JoinLeft {
					return errorf("expected LEFT JOIN, got %v", q.SQL.Joins[0].JoinType)
				}
				return nil
			},
		},
		{
			input: "SELECT service, count(*) FROM logs GROUP BY service HAVING count(*) > 10",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if q.SQL.Having == nil {
					return errorf("expected HAVING clause")
				}
				return nil
			},
		},
		{
			input: "SELECT * FROM logs ORDER BY timestamp DESC LIMIT 100",
			check: func(q *Query) error {
				if q.SQL == nil {
					return errorf("expected SQL query")
				}
				if q.SQL.OrderBy == nil {
					return errorf("expected ORDER BY clause")
				}
				if q.SQL.Limit == nil {
					return errorf("expected LIMIT clause")
				}
				if q.SQL.Limit.Count != 100 {
					return errorf("expected LIMIT 100, got %d", q.SQL.Limit.Count)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if err := tt.check(q); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestPlanner(t *testing.T) {
	tests := []struct {
		input string
		check func(*Plan) error
	}{
		{
			input: "logs | where service = 'api' | limit 10",
			check: func(p *Plan) error {
				if p.Root == nil {
					return errorf("expected plan root")
				}
				if p.Root.NodeType() != PlanLimitType {
					return errorf("expected Limit at root, got %v", p.Root.NodeType())
				}
				return nil
			},
		},
		{
			input: "SELECT * FROM logs WHERE service = 'api'",
			check: func(p *Plan) error {
				if p.Root == nil {
					return errorf("expected plan root")
				}
				return nil
			},
		},
		{
			input: "SELECT * FROM traces t LEFT JOIN logs l ON l.trace_id = t.trace_id",
			check: func(p *Plan) error {
				if p.Root == nil {
					return errorf("expected plan root")
				}
				if p.Root.NodeType() != PlanJoinType {
					return errorf("expected Join at root, got %v", p.Root.NodeType())
				}
				join := p.Root.(*PlanJoinNode)
				if join.JoinType != JoinLeft {
					return errorf("expected LEFT join, got %v", join.JoinType)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}

			planner := NewPlanner()
			plan, err := planner.Plan(q)
			if err != nil {
				t.Fatalf("planner error: %v", err)
			}

			if err := tt.check(plan); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestTimeBucket(t *testing.T) {
	e := NewExecutor()

	// Test time_bucket function
	ts := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	result, err := e.callFunction("time_bucket", []interface{}{"1h", ts})
	if err != nil {
		t.Fatalf("time_bucket error: %v", err)
	}

	expected := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	if !result.(time.Time).Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}

	// Test with 5 minute bucket
	result, err = e.callFunction("time_bucket", []interface{}{"5m", ts})
	if err != nil {
		t.Fatalf("time_bucket error: %v", err)
	}

	expected = time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !result.(time.Time).Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestDateTrunc(t *testing.T) {
	e := NewExecutor()

	ts := time.Date(2024, 6, 15, 10, 30, 45, 123, time.UTC)

	tests := []struct {
		precision string
		expected  time.Time
	}{
		{"second", time.Date(2024, 6, 15, 10, 30, 45, 0, time.UTC)},
		{"minute", time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)},
		{"hour", time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)},
		{"day", time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)},
		{"month", time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)},
		{"year", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.precision, func(t *testing.T) {
			result, err := e.callFunction("date_trunc", []interface{}{tt.precision, ts})
			if err != nil {
				t.Fatalf("date_trunc error: %v", err)
			}

			if !result.(time.Time).Equal(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDualSyntaxEquivalence(t *testing.T) {
	// Test that equivalent pipe and SQL queries produce similar plans
	pipeQuery := "logs | where service = 'api' | limit 10"
	sqlQuery := "SELECT * FROM logs WHERE service = 'api' LIMIT 10"

	pipeAST, err := Parse(pipeQuery)
	if err != nil {
		t.Fatalf("pipe parse error: %v", err)
	}

	sqlAST, err := Parse(sqlQuery)
	if err != nil {
		t.Fatalf("sql parse error: %v", err)
	}

	planner := NewPlanner()

	pipePlan, err := planner.Plan(pipeAST)
	if err != nil {
		t.Fatalf("pipe planner error: %v", err)
	}

	sqlPlan, err := planner.Plan(sqlAST)
	if err != nil {
		t.Fatalf("sql planner error: %v", err)
	}

	// Both should have a limit node at root
	if pipePlan.Root.NodeType() != PlanLimitType {
		t.Errorf("pipe plan: expected Limit at root")
	}
	if sqlPlan.Root.NodeType() != PlanLimitType {
		t.Errorf("sql plan: expected Limit at root")
	}
}

func errorf(format string, args ...interface{}) error {
	return &testError{msg: format, args: args}
}

type testError struct {
	msg  string
	args []interface{}
}

func (e *testError) Error() string {
	return e.msg
}
