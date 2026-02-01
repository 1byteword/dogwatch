package query

import (
	"testing"
)

// Test queries for benchmarks
var (
	simplePipeQuery     = "logs | where level = 'error'"
	mediumPipeQuery     = "logs | where level = 'error' and service = 'api-gateway' | select message, timestamp"
	complexPipeQuery    = "logs | where level = 'error' and service = 'api-gateway' | avg(duration) by (service, method) | order by value desc | limit 10"
	simpleSQLQuery      = "SELECT * FROM logs WHERE level = 'error'"
	mediumSQLQuery      = "SELECT message, timestamp FROM logs WHERE level = 'error' AND service = 'api-gateway'"
	complexSQLQuery     = "SELECT service, method, avg(duration) as avg_duration FROM traces WHERE status = 'ERROR' GROUP BY service, method ORDER BY avg_duration DESC LIMIT 10"
	joinSQLQuery        = "SELECT l.message, t.name FROM logs l JOIN traces t ON l.trace_id = t.trace_id WHERE l.level = 'error'"
	aggregationQuery    = "traces | count, sum(duration), avg(duration), p99(duration) by (service)"
	windowQuery         = "metrics | window tumbling=5m | avg(value) by (host)"
	correlateQuery      = "traces | correlate logs on (trace_id, time ± 1m)"
	extractQuery        = "logs | extract pattern='(?P<ip>\\d+\\.\\d+\\.\\d+\\.\\d+)'"
	histogramQuery      = "traces | histogram duration buckets=20 by (service)"
	timeRangeQuery      = "logs | where level = 'error' last 1h"
	defineQuery         = "define errors = (logs | where level = 'error') errors | count by (service)"
)

func BenchmarkLexer(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"SimplePipe", simplePipeQuery},
		{"MediumPipe", mediumPipeQuery},
		{"ComplexPipe", complexPipeQuery},
		{"SimpleSQL", simpleSQLQuery},
		{"MediumSQL", mediumSQLQuery},
		{"ComplexSQL", complexSQLQuery},
		{"JoinSQL", joinSQLQuery},
		{"Aggregation", aggregationQuery},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.query)
				_, _ = lexer.Tokenize()
			}
		})
	}
}

func BenchmarkLexerTokenTypes(b *testing.B) {
	// Test specific token type parsing speed
	inputs := []struct {
		name  string
		input string
	}{
		{"Identifier", "service_name"},
		{"String", "'error message'"},
		{"Number", "12345"},
		{"Duration", "5m"},
		{"Operators", "= != < <= > >= + - * /"},
		{"Keywords", "select from where and or not in like"},
		{"Mixed", "logs | where level = 'error' and duration > 100"},
	}

	for _, tt := range inputs {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.input)
				_, _ = lexer.Tokenize()
			}
		})
	}
}

func BenchmarkParsePipeQuery(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"Simple", simplePipeQuery},
		{"Medium", mediumPipeQuery},
		{"Complex", complexPipeQuery},
		{"Aggregation", aggregationQuery},
		{"Window", windowQuery},
		{"Correlate", correlateQuery},
		{"Extract", extractQuery},
		{"Histogram", histogramQuery},
		{"TimeRange", timeRangeQuery},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

func BenchmarkParseSQLQuery(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"Simple", simpleSQLQuery},
		{"Medium", mediumSQLQuery},
		{"Complex", complexSQLQuery},
		{"WithJoin", joinSQLQuery},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

func BenchmarkParseExpressions(b *testing.B) {
	expressions := []struct {
		name  string
		query string
	}{
		{"SimpleComparison", "logs | where level = 'error'"},
		{"And", "logs | where level = 'error' and service = 'api'"},
		{"Or", "logs | where level = 'error' or level = 'warn'"},
		{"Not", "logs | where not level = 'debug'"},
		{"Nested", "logs | where (level = 'error' or level = 'warn') and service = 'api'"},
		{"Arithmetic", "logs | where duration + 100 > 500"},
		{"In", "logs | where level in ('error', 'warn', 'fatal')"},
		{"Like", "logs | where message like '%error%'"},
		{"Matches", "logs | where message matches '/error/i'"},
		{"FunctionCall", "logs | where length(message) > 100"},
	}

	for _, tt := range expressions {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

func BenchmarkParseAggregations(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"Count", "logs | count"},
		{"CountBy", "logs | count by (service)"},
		{"Sum", "traces | sum(duration)"},
		{"Avg", "traces | avg(duration)"},
		{"Multiple", "traces | count, sum(duration), avg(duration)"},
		{"MultipleBy", "traces | count, avg(duration), p99(duration) by (service, method)"},
		{"Percentiles", "traces | p50(duration), p90(duration), p95(duration), p99(duration)"},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

func BenchmarkParseDuration(b *testing.B) {
	durations := []string{"1s", "5m", "1h", "24h", "7d", "1w", "100ms", "500us"}

	for _, d := range durations {
		b.Run(d, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = parseDuration(d)
			}
		})
	}
}

func BenchmarkTokenName(b *testing.B) {
	tokens := []TokenType{
		TokenSelect, TokenFrom, TokenWhere, TokenAnd, TokenOr,
		TokenIdent, TokenString, TokenNumber, TokenDuration,
		TokenLParen, TokenRParen, TokenEq, TokenNeq,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, t := range tokens {
			_ = TokenName(t)
		}
	}
}

// Benchmark concurrent parsing
func BenchmarkConcurrentParse(b *testing.B) {
	queries := []string{
		simplePipeQuery,
		mediumPipeQuery,
		complexPipeQuery,
		simpleSQLQuery,
		mediumSQLQuery,
		complexSQLQuery,
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = Parse(queries[i%len(queries)])
			i++
		}
	})
}

func BenchmarkConcurrentLex(b *testing.B) {
	queries := []string{
		simplePipeQuery,
		mediumPipeQuery,
		complexPipeQuery,
		simpleSQLQuery,
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			lexer := NewLexer(queries[i%len(queries)])
			_, _ = lexer.Tokenize()
			i++
		}
	})
}

// Benchmark query complexity scaling
func BenchmarkParseScaling(b *testing.B) {
	// Generate queries with increasing complexity
	generateQuery := func(conditions int) string {
		query := "logs | where level = 'error'"
		for i := 0; i < conditions; i++ {
			query += " and field" + string(rune('a'+i%26)) + " = 'value'"
		}
		return query
	}

	complexities := []int{1, 5, 10, 20, 50}

	for _, c := range complexities {
		query := generateQuery(c)
		b.Run(string(rune('0'+c/10))+string(rune('0'+c%10))+"Conditions", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(query)
			}
		})
	}
}

// Benchmark string handling in lexer
func BenchmarkLexerStrings(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"ShortString", `'short'`},
		{"MediumString", `'this is a medium length string value'`},
		{"LongString", `'this is a very long string that contains a lot of text and might appear in real world queries as search patterns or log messages'`},
		{"EscapedString", `'contains \'escaped\' quotes and \\ backslashes'`},
		{"DoubleQuoted", `"double quoted string"`},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.input)
				_, _ = lexer.Tokenize()
			}
		})
	}
}

// Benchmark number parsing
func BenchmarkLexerNumbers(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"Integer", "12345"},
		{"Float", "123.456"},
		{"Negative", "-12345"},
		{"NegativeFloat", "-123.456"},
		{"Duration", "5m"},
		{"DurationLong", "3600s"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.input)
				_, _ = lexer.Tokenize()
			}
		})
	}
}

// Benchmark whitespace handling
func BenchmarkLexerWhitespace(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{"NoWhitespace", "logs|where|level='error'"},
		{"Spaces", "logs | where level = 'error'"},
		{"Tabs", "logs\t|\twhere\tlevel\t=\t'error'"},
		{"Newlines", "logs\n|\nwhere\nlevel = 'error'"},
		{"Mixed", "logs  |  where\n\tlevel\t=   'error'"},
		{"Comments", "logs | where -- this is a comment\n level = 'error'"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lexer := NewLexer(tt.input)
				_, _ = lexer.Tokenize()
			}
		})
	}
}

// Benchmark parser error handling
func BenchmarkParseErrors(b *testing.B) {
	invalidQueries := []struct {
		name  string
		query string
	}{
		{"MissingSource", "| where level = 'error'"},
		{"InvalidOperator", "logs | where level == 'error'"},
		{"UnterminatedString", "logs | where level = 'error"},
		{"MissingParenClose", "logs | where (level = 'error'"},
		{"InvalidKeyword", "logs | invalid level = 'error'"},
	}

	for _, tt := range invalidQueries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

// Benchmark full pipeline: lex + parse
func BenchmarkFullPipeline(b *testing.B) {
	queries := []struct {
		name  string
		query string
	}{
		{"RealWorld_LogSearch", "logs | where level = 'error' and service = 'api-gateway' | select timestamp, message | order by timestamp desc | limit 100"},
		{"RealWorld_TraceAnalysis", "traces | where status = 'ERROR' | avg(duration), p99(duration) by (service, operation) | order by avg desc"},
		{"RealWorld_MetricAgg", "metrics.http_requests | window tumbling=5m | sum(value) by (host, method)"},
		{"SQL_Complex", "SELECT service, COUNT(*) as cnt, AVG(duration) as avg_dur FROM traces WHERE status = 'ERROR' GROUP BY service ORDER BY cnt DESC LIMIT 20"},
	}

	for _, tt := range queries {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = Parse(tt.query)
			}
		})
	}
}

// Benchmark Token creation
func BenchmarkTokenCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Token{
			Type:  TokenIdent,
			Value: "identifier_name",
			Pos:   Position{Line: 1, Column: 10, Offset: 10},
		}
	}
}

// Benchmark Parser helper methods
func BenchmarkParserHelpers(b *testing.B) {
	input := "logs | where level = 'error' and service = 'api'"
	lexer := NewLexer(input)
	tokens, _ := lexer.Tokenize()

	b.Run("Check", func(b *testing.B) {
		parser := NewParser(tokens)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.pos = 0
			_ = parser.check(TokenLogs)
		}
	})

	b.Run("Match", func(b *testing.B) {
		parser := NewParser(tokens)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.pos = 0
			_ = parser.match(TokenLogs)
		}
	})

	b.Run("Advance", func(b *testing.B) {
		parser := NewParser(tokens)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parser.pos = 0
			_ = parser.advance()
		}
	})
}
