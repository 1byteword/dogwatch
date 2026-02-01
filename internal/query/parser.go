package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Parser parses WatchQL queries
type Parser struct {
	tokens  []Token
	pos     int
	errors  []ParseError
	defines map[string]*DefineNode
}

// ParseError represents a parsing error with context
type ParseError struct {
	Message  string
	Position Position
	Token    Token
	Expected string
	Got      string
	Context  string // Source snippet around error
	Hint     string // Suggestion for fixing
}

func (e ParseError) Error() string {
	s := fmt.Sprintf("parse error at %s: %s", e.Position, e.Message)
	if e.Expected != "" {
		s += fmt.Sprintf(" (expected %s, got %s)", e.Expected, e.Got)
	}
	if e.Context != "" {
		s += fmt.Sprintf("\n  %s", e.Context)
	}
	if e.Hint != "" {
		s += fmt.Sprintf("\n  Hint: %s", e.Hint)
	}
	return s
}

// NewParser creates a new parser
func NewParser(tokens []Token) *Parser {
	return &Parser{
		tokens:  tokens,
		defines: make(map[string]*DefineNode),
	}
}

// Parse parses the tokens into a Query AST
func Parse(input string) (*Query, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	parser := NewParser(tokens)
	return parser.Parse()
}

// Parse parses a query (SQL or pipe syntax)
func (p *Parser) Parse() (*Query, error) {
	// Handle define statements first
	for p.check(TokenDefine) {
		def, err := p.parseDefine()
		if err != nil {
			return nil, err
		}
		p.defines[def.Name] = def
	}

	// Detect SQL vs pipe syntax
	if p.check(TokenSelect) {
		return p.parseSQLQuery()
	}

	query, err := p.parsePipeQuery()
	if err != nil {
		return nil, err
	}

	query.Definitions = p.defines
	return query, nil
}

// parsePipeQuery parses pipe-based WatchQL syntax
func (p *Parser) parsePipeQuery() (*Query, error) {
	return p.parseQuery()
}

func (p *Parser) parseQuery() (*Query, error) {
	query := &Query{Pos: p.current().Pos}

	// Parse source
	source, err := p.parseSource()
	if err != nil {
		return nil, err
	}
	query.Source = source

	// Parse pipe operations
	for p.match(TokenPipe) {
		pipe, err := p.parsePipe()
		if err != nil {
			return nil, err
		}
		query.Pipes = append(query.Pipes, pipe)
	}

	// Parse optional time range at the end
	if p.check(TokenLast) || p.check(TokenBetween) {
		tr, err := p.parseTimeRange()
		if err != nil {
			return nil, err
		}
		query.TimeRange = tr
	}

	return query, nil
}

func (p *Parser) parseSource() (*SourceNode, error) {
	pos := p.current().Pos

	var sourceType string
	switch {
	case p.match(TokenMetrics):
		sourceType = "metrics"
	case p.match(TokenLogs):
		sourceType = "logs"
	case p.match(TokenTraces):
		sourceType = "traces"
	case p.match(TokenEvents):
		sourceType = "events"
	case p.check(TokenIdent):
		// Check if it's a defined query
		tok := p.advance()
		if def, ok := p.defines[tok.Value]; ok {
			return def.Query.Source, nil
		}
		return nil, p.error("unknown source '%s' (expected metrics, logs, traces, or events)", tok.Value)
	default:
		return nil, p.error("expected data source (metrics, logs, traces, or events)")
	}

	source := &SourceNode{
		SourceType: sourceType,
		Pos:        pos,
	}

	// Check for metric name: metrics.cpu_percent
	if p.match(TokenDot) {
		if !p.check(TokenIdent) {
			return nil, p.error("expected metric name after '.'")
		}
		source.MetricName = p.advance().Value
	}

	return source, nil
}

func (p *Parser) parsePipe() (PipeNode, error) {
	switch {
	case p.check(TokenWhere):
		return p.parseWhere()
	case p.check(TokenSelect):
		return p.parseSelect()
	case p.check(TokenOrder):
		return p.parseOrderBy()
	case p.check(TokenLimit):
		return p.parseLimit()
	case p.check(TokenTop):
		return p.parseTop()
	case p.check(TokenWindow):
		return p.parseWindow()
	case p.check(TokenCorrelate):
		return p.parseCorrelate()
	case p.check(TokenAnomalies):
		return p.parseAnomalies()
	case p.check(TokenExtract):
		return p.parseExtract()
	case p.check(TokenHistogram):
		return p.parseHistogram()
	case p.check(TokenDistinct):
		return p.parseDistinct()
	case p.checkAggregation():
		return p.parseGroupBy()
	default:
		return nil, p.error("expected pipe operation (where, select, avg, sum, count, etc.)")
	}
}

func (p *Parser) parseWhere() (*WhereNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'where'

	cond, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	return &WhereNode{
		Condition: cond,
		Pos:       pos,
	}, nil
}

func (p *Parser) parseSelect() (*SelectNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'select'

	var fields []SelectField
	for {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}

		field := SelectField{Expr: expr}

		if p.match(TokenAs) {
			if !p.check(TokenIdent) {
				return nil, p.error("expected alias name after 'as'")
			}
			field.Alias = p.advance().Value
		}

		fields = append(fields, field)

		if !p.match(TokenComma) {
			break
		}
	}

	return &SelectNode{
		Fields: fields,
		Pos:    pos,
	}, nil
}

func (p *Parser) parseGroupBy() (*GroupByNode, error) {
	pos := p.current().Pos

	var aggs []AggregationExpr
	for p.checkAggregation() {
		agg, err := p.parseAggregation()
		if err != nil {
			return nil, err
		}
		aggs = append(aggs, agg)

		if !p.match(TokenComma) {
			break
		}
	}

	var groupFields []string
	if p.match(TokenBy) {
		if !p.match(TokenLParen) {
			return nil, p.error("expected '(' after 'by'")
		}

		for {
			if !p.check(TokenIdent) {
				return nil, p.error("expected field name in group by")
			}
			groupFields = append(groupFields, p.advance().Value)

			if !p.match(TokenComma) {
				break
			}
		}

		if !p.match(TokenRParen) {
			return nil, p.error("expected ')' to close group by")
		}
	}

	return &GroupByNode{
		Aggregations: aggs,
		GroupFields:  groupFields,
		Pos:          pos,
	}, nil
}

func (p *Parser) parseAggregation() (AggregationExpr, error) {
	agg := AggregationExpr{}

	switch {
	case p.match(TokenCount):
		agg.Function = "count"
	case p.match(TokenSum):
		agg.Function = "sum"
	case p.match(TokenAvg):
		agg.Function = "avg"
	case p.match(TokenMin):
		agg.Function = "min"
	case p.match(TokenMax):
		agg.Function = "max"
	case p.match(TokenP50):
		agg.Function = "p50"
	case p.match(TokenP90):
		agg.Function = "p90"
	case p.match(TokenP95):
		agg.Function = "p95"
	case p.match(TokenP99):
		agg.Function = "p99"
	default:
		return agg, p.error("expected aggregation function")
	}

	// Optional field in parentheses
	if p.match(TokenLParen) {
		if p.check(TokenIdent) {
			agg.Field = p.advance().Value
		}
		if !p.match(TokenRParen) {
			return agg, p.error("expected ')' after aggregation field")
		}
	}

	return agg, nil
}

func (p *Parser) parseOrderBy() (*OrderByNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'order'

	if !p.match(TokenBy) {
		return nil, p.error("expected 'by' after 'order'")
	}

	var fields []OrderField
	for {
		if !p.check(TokenIdent) {
			return nil, p.error("expected field name in order by")
		}

		field := OrderField{Field: p.advance().Value}

		if p.match(TokenDesc) {
			field.Desc = true
		} else {
			p.match(TokenAsc) // optional, default is asc
		}

		fields = append(fields, field)

		if !p.match(TokenComma) {
			break
		}
	}

	return &OrderByNode{
		Fields: fields,
		Pos:    pos,
	}, nil
}

func (p *Parser) parseLimit() (*LimitNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'limit'

	if !p.check(TokenNumber) {
		return nil, p.error("expected number after 'limit'")
	}

	count, err := strconv.Atoi(p.advance().Value)
	if err != nil {
		return nil, p.error("invalid limit: %s", err)
	}

	node := &LimitNode{
		Count: count,
		Pos:   pos,
	}

	if p.match(TokenOffset) {
		if !p.check(TokenNumber) {
			return nil, p.error("expected number after 'offset'")
		}
		node.Offset, _ = strconv.Atoi(p.advance().Value)
	}

	return node, nil
}

func (p *Parser) parseTop() (*TopNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'top'

	if !p.check(TokenNumber) {
		return nil, p.error("expected number after 'top'")
	}

	count, _ := strconv.Atoi(p.advance().Value)

	node := &TopNode{
		Count: count,
		Pos:   pos,
	}

	// Optional field to sort by
	if p.check(TokenIdent) {
		node.Field = p.advance().Value
	}

	return node, nil
}

func (p *Parser) parseWindow() (*WindowNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'window'

	node := &WindowNode{Pos: pos}

	// Parse window type and duration
	switch {
	case p.match(TokenTumbling):
		node.WindowType = "tumbling"
		if !p.match(TokenEq) {
			return nil, p.error("expected '=' after 'tumbling'")
		}
	case p.match(TokenSliding):
		node.WindowType = "sliding"
		if !p.match(TokenEq) {
			return nil, p.error("expected '=' after 'sliding'")
		}
	case p.match(TokenSession):
		node.WindowType = "session"
		if !p.match(TokenEq) {
			return nil, p.error("expected '=' after 'session'")
		}
	default:
		return nil, p.error("expected window type (tumbling, sliding, session)")
	}

	if !p.check(TokenDuration) {
		return nil, p.error("expected duration after window type")
	}

	dur, err := parseDuration(p.advance().Value)
	if err != nil {
		return nil, p.error("invalid duration: %s", err)
	}
	node.Duration = dur

	return node, nil
}

func (p *Parser) parseCorrelate() (*CorrelateNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'correlate'

	node := &CorrelateNode{Pos: pos}

	// Parse target (logs, traces, metrics)
	switch {
	case p.match(TokenLogs):
		node.Target = "logs"
	case p.match(TokenTraces):
		node.Target = "traces"
	case p.match(TokenMetrics):
		node.Target = "metrics"
	default:
		return nil, p.error("expected correlation target (logs, traces, metrics)")
	}

	// Parse 'on' clause
	if !p.match(TokenOn) {
		return nil, p.error("expected 'on' clause in correlate")
	}

	if !p.match(TokenLParen) {
		return nil, p.error("expected '(' after 'on'")
	}

	// Parse join fields
	for {
		if p.check(TokenTime) {
			// Handle time ± tolerance
			p.advance()
			if p.match(TokenPlusMinus) {
				if !p.check(TokenDuration) {
					return nil, p.error("expected duration after '±'")
				}
				dur, err := parseDuration(p.advance().Value)
				if err != nil {
					return nil, p.error("invalid time tolerance: %s", err)
				}
				node.TimeTolerance = dur
			}
		} else if p.check(TokenIdent) {
			node.JoinFields = append(node.JoinFields, p.advance().Value)
		}

		if !p.match(TokenComma) {
			break
		}
	}

	if !p.match(TokenRParen) {
		return nil, p.error("expected ')' to close correlation fields")
	}

	return node, nil
}

func (p *Parser) parseAnomalies() (*AnomaliesNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'anomalies'

	node := &AnomaliesNode{
		Algorithm:   "ensemble",
		Sensitivity: 0.8,
		Pos:         pos,
	}

	// Parse optional parameters
	for {
		if p.match(TokenAlgorithm) {
			if !p.match(TokenEq) {
				return nil, p.error("expected '=' after 'algorithm'")
			}
			if !p.check(TokenIdent) {
				return nil, p.error("expected algorithm name")
			}
			node.Algorithm = p.advance().Value
		} else if p.match(TokenSensitivity) {
			if !p.match(TokenEq) {
				return nil, p.error("expected '=' after 'sensitivity'")
			}
			if !p.check(TokenNumber) {
				return nil, p.error("expected number for sensitivity")
			}
			sens, _ := strconv.ParseFloat(p.advance().Value, 64)
			node.Sensitivity = sens
		} else {
			break
		}
	}

	return node, nil
}

func (p *Parser) parseExtract() (*ExtractNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'extract'

	node := &ExtractNode{Pos: pos}

	if p.match(TokenAuto) {
		node.Auto = true
		return node, nil
	}

	if p.match(TokenPattern) {
		if !p.match(TokenEq) {
			return nil, p.error("expected '=' after 'pattern'")
		}
		if !p.check(TokenString) {
			return nil, p.error("expected pattern string")
		}
		node.Pattern = p.advance().Value
	} else if p.check(TokenString) {
		node.Pattern = p.advance().Value
	} else {
		return nil, p.error("expected 'auto' or pattern string")
	}

	return node, nil
}

func (p *Parser) parseHistogram() (*HistogramNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'histogram'

	node := &HistogramNode{
		Buckets: 20,
		Pos:     pos,
	}

	if !p.check(TokenIdent) {
		return nil, p.error("expected field name for histogram")
	}
	node.Field = p.advance().Value

	// Parse optional parameters
	for {
		if p.match(TokenBuckets) {
			if !p.match(TokenEq) {
				return nil, p.error("expected '=' after 'buckets'")
			}
			if !p.check(TokenNumber) {
				return nil, p.error("expected number of buckets")
			}
			node.Buckets, _ = strconv.Atoi(p.advance().Value)
		} else if p.match(TokenBy) {
			if !p.match(TokenLParen) {
				return nil, p.error("expected '(' after 'by'")
			}
			for {
				if !p.check(TokenIdent) {
					return nil, p.error("expected field name in group by")
				}
				node.GroupBy = append(node.GroupBy, p.advance().Value)
				if !p.match(TokenComma) {
					break
				}
			}
			if !p.match(TokenRParen) {
				return nil, p.error("expected ')' to close group by")
			}
		} else {
			break
		}
	}

	return node, nil
}

func (p *Parser) parseDistinct() (*SelectNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'distinct'

	if !p.check(TokenIdent) {
		return nil, p.error("expected field name after 'distinct'")
	}

	field := p.advance().Value

	return &SelectNode{
		Fields: []SelectField{{
			Expr: &FunctionCallExpr{
				Name: "distinct",
				Args: []Expr{&IdentifierExpr{Name: field, Pos: pos}},
				Pos:  pos,
			},
		}},
		Pos: pos,
	}, nil
}

func (p *Parser) parseTimeRange() (*TimeRangeNode, error) {
	pos := p.current().Pos
	node := &TimeRangeNode{Pos: pos}

	if p.match(TokenLast) {
		if !p.check(TokenDuration) {
			return nil, p.error("expected duration after 'last'")
		}
		dur, err := parseDuration(p.advance().Value)
		if err != nil {
			return nil, p.error("invalid duration: %s", err)
		}
		node.Relative = dur
	} else if p.match(TokenBetween) {
		// Parse absolute time range
		if !p.check(TokenString) {
			return nil, p.error("expected start time string")
		}
		start, err := time.Parse(time.RFC3339, p.advance().Value)
		if err != nil {
			return nil, p.error("invalid start time: %s", err)
		}
		node.Start = start

		if !p.match(TokenAnd) {
			return nil, p.error("expected 'and' in between clause")
		}

		if !p.check(TokenString) {
			return nil, p.error("expected end time string")
		}
		end, err := time.Parse(time.RFC3339, p.advance().Value)
		if err != nil {
			return nil, p.error("invalid end time: %s", err)
		}
		node.End = end
	}

	// Optional shift
	if p.match(TokenShift) {
		if !p.check(TokenDuration) && !p.check(TokenMinus) {
			return nil, p.error("expected duration after 'shift'")
		}
		negative := p.match(TokenMinus)
		if !p.check(TokenDuration) {
			return nil, p.error("expected duration after 'shift'")
		}
		dur, err := parseDuration(p.advance().Value)
		if err != nil {
			return nil, p.error("invalid shift duration: %s", err)
		}
		if negative {
			dur = -dur
		}
		node.Shift = dur
	}

	return node, nil
}

func (p *Parser) parseDefine() (*DefineNode, error) {
	pos := p.current().Pos
	p.advance() // consume 'define'

	if !p.check(TokenIdent) {
		return nil, p.error("expected name after 'define'")
	}
	name := p.advance().Value

	if !p.match(TokenEq) {
		return nil, p.error("expected '=' after define name")
	}

	if !p.match(TokenLParen) {
		return nil, p.error("expected '(' to start query definition")
	}

	query, err := p.parseQuery()
	if err != nil {
		return nil, err
	}

	if !p.match(TokenRParen) {
		return nil, p.error("expected ')' to end query definition")
	}

	return &DefineNode{
		Name:  name,
		Query: query,
		Pos:   pos,
	}, nil
}

// Expression parsing with precedence climbing
func (p *Parser) parseExpression() (Expr, error) {
	return p.parseOr()
}

func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.match(TokenOr) {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: "or", Right: right, Pos: left.Position()}
	}

	return left, nil
}

func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.match(TokenAnd) {
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: "and", Right: right, Pos: left.Position()}
	}

	return left, nil
}

func (p *Parser) parseNot() (Expr, error) {
	if p.match(TokenNot) {
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Operator: "not", Operand: expr, Pos: expr.Position()}, nil
	}
	return p.parseComparison()
}

func (p *Parser) parseComparison() (Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}

	for {
		var op string
		switch {
		case p.match(TokenEq):
			op = "="
		case p.match(TokenNeq):
			op = "!="
		case p.match(TokenLt):
			op = "<"
		case p.match(TokenLte):
			op = "<="
		case p.match(TokenGt):
			op = ">"
		case p.match(TokenGte):
			op = ">="
		case p.match(TokenLike):
			op = "like"
		case p.match(TokenMatches):
			op = "matches"
		case p.match(TokenIn):
			op = "in"
			// Parse the list or subquery
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return &BinaryExpr{Left: left, Operator: op, Right: right, Pos: left.Position()}, nil
		default:
			return left, nil
		}

		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: op, Right: right, Pos: left.Position()}
	}
}

func (p *Parser) parseAdditive() (Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}

	for {
		var op string
		switch {
		case p.match(TokenPlus):
			op = "+"
		case p.match(TokenMinus):
			op = "-"
		default:
			return left, nil
		}

		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: op, Right: right, Pos: left.Position()}
	}
}

func (p *Parser) parseMultiplicative() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	for {
		var op string
		switch {
		case p.match(TokenStar):
			op = "*"
		case p.match(TokenSlash):
			op = "/"
		case p.match(TokenPercent):
			op = "%"
		default:
			return left, nil
		}

		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Operator: op, Right: right, Pos: left.Position()}
	}
}

func (p *Parser) parseUnary() (Expr, error) {
	if p.match(TokenMinus) {
		expr, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Operator: "-", Operand: expr, Pos: expr.Position()}, nil
	}
	return p.parsePrimary()
}

func (p *Parser) parsePrimary() (Expr, error) {
	pos := p.current().Pos

	switch {
	case p.check(TokenNumber):
		tok := p.advance()
		if strings.Contains(tok.Value, ".") {
			val, _ := strconv.ParseFloat(tok.Value, 64)
			return &LiteralExpr{Value: val, ValueType: LiteralFloat, Pos: pos}, nil
		}
		val, _ := strconv.ParseInt(tok.Value, 10, 64)
		return &LiteralExpr{Value: val, ValueType: LiteralInt, Pos: pos}, nil

	case p.check(TokenDuration):
		tok := p.advance()
		dur, err := parseDuration(tok.Value)
		if err != nil {
			return nil, p.error("invalid duration: %s", err)
		}
		return &LiteralExpr{Value: dur, ValueType: LiteralDuration, Pos: pos}, nil

	case p.check(TokenString):
		tok := p.advance()
		return &LiteralExpr{Value: tok.Value, ValueType: LiteralString, Pos: pos}, nil

	case p.check(TokenRegex):
		tok := p.advance()
		return &LiteralExpr{Value: tok.Value, ValueType: LiteralRegex, Pos: pos}, nil

	case p.match(TokenTrue):
		return &LiteralExpr{Value: true, ValueType: LiteralBool, Pos: pos}, nil

	case p.match(TokenFalse):
		return &LiteralExpr{Value: false, ValueType: LiteralBool, Pos: pos}, nil

	case p.match(TokenNull):
		return &LiteralExpr{Value: nil, ValueType: LiteralNull, Pos: pos}, nil

	case p.check(TokenIdent):
		return p.parseIdentifierOrCall()

	// Handle aggregation keywords as function calls in SQL expressions
	case p.checkAggregation():
		return p.parseAggregationAsFunction()

	case p.match(TokenLParen):
		// Could be a subquery or grouped expression or list
		if p.check(TokenMetrics) || p.check(TokenLogs) || p.check(TokenTraces) || p.check(TokenEvents) {
			query, err := p.parseQuery()
			if err != nil {
				return nil, err
			}
			if !p.match(TokenRParen) {
				return nil, p.error("expected ')' to close subquery")
			}
			return &SubqueryExpr{Query: query, Pos: pos}, nil
		}

		// Check if it's a list
		if p.check(TokenString) || p.check(TokenNumber) {
			return p.parseList(pos)
		}

		// Grouped expression
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if !p.match(TokenRParen) {
			return nil, p.error("expected ')' to close expression")
		}
		return expr, nil

	case p.match(TokenStar):
		return &IdentifierExpr{Name: "*", Pos: pos}, nil
	}

	return nil, p.error("unexpected token in expression")
}

func (p *Parser) parseList(pos Position) (Expr, error) {
	var values []Expr

	for {
		expr, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		values = append(values, expr)

		if !p.match(TokenComma) {
			break
		}
	}

	if !p.match(TokenRParen) {
		return nil, p.error("expected ')' to close list")
	}

	return &ListExpr{Values: values, Pos: pos}, nil
}

// parseAggregationAsFunction parses aggregation keywords (count, sum, etc.) as function calls
func (p *Parser) parseAggregationAsFunction() (Expr, error) {
	pos := p.current().Pos
	tok := p.advance() // consume the aggregation keyword
	name := tok.Value

	// Must be followed by (
	if !p.match(TokenLParen) {
		return nil, p.error("expected '(' after %s", name)
	}

	// Parse arguments
	var args []Expr
	if !p.check(TokenRParen) {
		for {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if !p.match(TokenComma) {
				break
			}
		}
	}

	if !p.match(TokenRParen) {
		return nil, p.error("expected ')' after function arguments")
	}

	return &FunctionCallExpr{Name: name, Args: args, Pos: pos}, nil
}

func (p *Parser) parseIdentifierOrCall() (Expr, error) {
	pos := p.current().Pos
	name := p.advance().Value

	// Check for function call
	if p.match(TokenLParen) {
		var args []Expr
		if !p.check(TokenRParen) {
			for {
				arg, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if !p.match(TokenComma) {
					break
				}
			}
		}
		if !p.match(TokenRParen) {
			return nil, p.error("expected ')' after function arguments")
		}
		return &FunctionCallExpr{Name: name, Args: args, Pos: pos}, nil
	}

	// Check for path (a.b.c)
	ident := &IdentifierExpr{Name: name, Pos: pos}
	for p.match(TokenDot) {
		if !p.check(TokenIdent) && !p.check(TokenStar) {
			return nil, p.error("expected identifier after '.'")
		}
		ident.Path = append(ident.Path, p.advance().Value)
	}

	return ident, nil
}

// Helper methods
func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) check(t TokenType) bool {
	return p.current().Type == t
}

func (p *Parser) checkAggregation() bool {
	switch p.current().Type {
	case TokenCount, TokenSum, TokenAvg, TokenMin, TokenMax, TokenP50, TokenP90, TokenP95, TokenP99:
		return true
	}
	return false
}

func (p *Parser) match(t TokenType) bool {
	if p.check(t) {
		p.pos++
		return true
	}
	return false
}

func (p *Parser) advance() Token {
	tok := p.current()
	if tok.Type != TokenEOF {
		p.pos++
	}
	return tok
}

func (p *Parser) error(format string, args ...interface{}) error {
	tok := p.current()
	err := ParseError{
		Message:  fmt.Sprintf(format, args...),
		Position: tok.Pos,
		Token:    tok,
		Got:      TokenName(tok.Type),
	}

	// Add context and hints based on common errors
	err.Hint = p.suggestHint(tok, err.Message)

	return err
}

// suggestHint provides helpful suggestions for common errors
func (p *Parser) suggestHint(tok Token, msg string) string {
	switch {
	case strings.Contains(msg, "expected data source"):
		return "Query should start with 'metrics', 'logs', 'traces', 'events', or 'SELECT'"
	case strings.Contains(msg, "expected FROM"):
		return "SQL queries require a FROM clause after SELECT, e.g., 'SELECT * FROM logs'"
	case strings.Contains(msg, "expected BY after GROUP"):
		return "GROUP requires BY, e.g., 'GROUP BY service'"
	case strings.Contains(msg, "expected JOIN"):
		return "Use 'INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', or 'FULL JOIN'"
	case strings.Contains(msg, "expected '=' or BETWEEN"):
		return "JOIN conditions should use '=' for equality or 'BETWEEN' for time ranges"
	case strings.Contains(msg, "unexpected token"):
		if tok.Type == TokenIdent {
			return fmt.Sprintf("'%s' is not a recognized keyword at this position", tok.Value)
		}
	}
	return ""
}

// errorWithHint creates an error with a specific hint
func (p *Parser) errorWithHint(hint string, format string, args ...interface{}) error {
	tok := p.current()
	return ParseError{
		Message:  fmt.Sprintf(format, args...),
		Position: tok.Pos,
		Token:    tok,
		Got:      TokenName(tok.Type),
		Hint:     hint,
	}
}

// parseSQLQuery parses SQL-style DQL query
func (p *Parser) parseSQLQuery() (*Query, error) {
	pos := p.current().Pos

	sql := &SQLQuery{Pos: pos}

	// Parse SELECT clause
	selectClause, err := p.parseSQLSelect()
	if err != nil {
		return nil, err
	}
	sql.Select = selectClause

	// Parse FROM clause
	if !p.match(TokenFrom) {
		return nil, p.error("expected FROM after SELECT")
	}
	fromClause, err := p.parseSQLFrom()
	if err != nil {
		return nil, err
	}
	sql.From = fromClause

	// Parse JOINs
	for p.checkJoin() {
		joinClause, err := p.parseSQLJoin()
		if err != nil {
			return nil, err
		}
		sql.Joins = append(sql.Joins, joinClause)
	}

	// Parse WHERE clause
	if p.match(TokenWhere) {
		cond, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		sql.Where = cond
	}

	// Parse GROUP BY clause
	if p.match(TokenGroup) {
		if !p.match(TokenBy) {
			return nil, p.error("expected BY after GROUP")
		}
		groupBy, err := p.parseSQLGroupBy()
		if err != nil {
			return nil, err
		}
		sql.GroupBy = groupBy
	}

	// Parse HAVING clause
	if p.match(TokenHaving) {
		having, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		sql.Having = having
	}

	// Parse ORDER BY clause
	if p.check(TokenOrder) {
		orderBy, err := p.parseOrderBy()
		if err != nil {
			return nil, err
		}
		sql.OrderBy = orderBy
	}

	// Parse LIMIT clause
	if p.check(TokenLimit) {
		limit, err := p.parseLimit()
		if err != nil {
			return nil, err
		}
		sql.Limit = limit
	}

	query := &Query{
		SQL:         sql,
		Pos:         pos,
		Definitions: p.defines,
	}

	return query, nil
}

// parseSQLSelect parses the SELECT clause
func (p *Parser) parseSQLSelect() (*SQLSelectClause, error) {
	pos := p.current().Pos
	p.advance() // consume SELECT

	clause := &SQLSelectClause{Pos: pos}

	// Check for DISTINCT
	if p.match(TokenDistinct) {
		clause.Distinct = true
	}

	// Parse fields
	for {
		field, err := p.parseSQLSelectField()
		if err != nil {
			return nil, err
		}
		clause.Fields = append(clause.Fields, field)

		if !p.match(TokenComma) {
			break
		}
	}

	return clause, nil
}

// parseSQLSelectField parses a single field in SELECT
func (p *Parser) parseSQLSelectField() (SQLSelectField, error) {
	expr, err := p.parseExpression()
	if err != nil {
		return SQLSelectField{}, err
	}

	field := SQLSelectField{Expr: expr}

	// Check for AS alias
	if p.match(TokenAs) {
		if !p.check(TokenIdent) {
			return field, p.error("expected alias name after AS")
		}
		field.Alias = p.advance().Value
	} else if p.check(TokenIdent) && !p.checkKeyword() {
		// Handle implicit alias (without AS)
		field.Alias = p.advance().Value
	}

	return field, nil
}

// parseSQLFrom parses the FROM clause
func (p *Parser) parseSQLFrom() (*SQLFromClause, error) {
	pos := p.current().Pos
	clause := &SQLFromClause{Pos: pos}

	// Check for subquery
	if p.match(TokenLParen) {
		if p.check(TokenSelect) {
			subquery, err := p.parseSQLQuery()
			if err != nil {
				return nil, err
			}
			clause.Subquery = subquery.SQL
			if !p.match(TokenRParen) {
				return nil, p.error("expected ')' after subquery")
			}
		} else {
			return nil, p.error("expected SELECT in subquery")
		}
	} else {
		// Parse source (metrics, logs, traces, events)
		source, err := p.parseSource()
		if err != nil {
			return nil, err
		}
		clause.Source = source
	}

	// Parse optional alias
	if p.check(TokenIdent) && !p.checkKeyword() {
		clause.Alias = p.advance().Value
	} else if p.match(TokenAs) {
		if !p.check(TokenIdent) {
			return nil, p.error("expected alias after AS")
		}
		clause.Alias = p.advance().Value
	}

	return clause, nil
}

// parseSQLJoin parses a JOIN clause
func (p *Parser) parseSQLJoin() (*SQLJoinClause, error) {
	pos := p.current().Pos
	clause := &SQLJoinClause{Pos: pos}

	// Determine join type
	if p.match(TokenInner) {
		clause.JoinType = JoinInner
		if !p.match(TokenJoin) {
			return nil, p.error("expected JOIN after INNER")
		}
	} else if p.match(TokenLeft) {
		clause.JoinType = JoinLeft
		p.match(TokenOuter) // optional OUTER
		if !p.match(TokenJoin) {
			return nil, p.error("expected JOIN after LEFT")
		}
	} else if p.match(TokenRight) {
		clause.JoinType = JoinRight
		p.match(TokenOuter) // optional OUTER
		if !p.match(TokenJoin) {
			return nil, p.error("expected JOIN after RIGHT")
		}
	} else if p.match(TokenFull) {
		clause.JoinType = JoinFull
		p.match(TokenOuter) // optional OUTER
		if !p.match(TokenJoin) {
			return nil, p.error("expected JOIN after FULL")
		}
	} else if p.match(TokenJoin) {
		clause.JoinType = JoinInner // Default to INNER JOIN
	} else {
		return nil, p.error("expected JOIN keyword")
	}

	// Parse source
	source, err := p.parseSQLFrom()
	if err != nil {
		return nil, err
	}
	clause.Source = source

	// Parse ON condition
	if p.match(TokenOn) {
		cond, err := p.parseSQLJoinCondition()
		if err != nil {
			return nil, err
		}
		clause.Condition = cond
	}

	return clause, nil
}

// parseSQLJoinCondition parses the ON condition for a JOIN
// It supports: expr = expr, expr BETWEEN expr AND expr, and compound conditions with AND
func (p *Parser) parseSQLJoinCondition() (*SQLJoinCondition, error) {
	// Parse the full expression - will handle =, BETWEEN, etc.
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}

	// Convert expression to join condition
	return p.exprToJoinCondition(expr)
}

// exprToJoinCondition converts a parsed expression into a SQLJoinCondition
func (p *Parser) exprToJoinCondition(expr Expr) (*SQLJoinCondition, error) {
	switch e := expr.(type) {
	case *BinaryExpr:
		switch e.Operator {
		case "=":
			return &SQLJoinCondition{
				Left:  e.Left,
				Op:    "=",
				Right: e.Right,
				Pos:   e.Pos,
			}, nil
		case "and":
			// For compound conditions, just use the first one for the join
			// TODO: support multiple join conditions
			return p.exprToJoinCondition(e.Left)
		default:
			return nil, p.errorWithHint(
				"Use '=' for equality joins or 'BETWEEN' for time-based joins",
				"unsupported join operator: %s", e.Operator)
		}
	default:
		return nil, p.errorWithHint(
			"JOIN conditions should be in the form: t1.column = t2.column",
			"invalid join condition")
	}
}

// parseSQLGroupBy parses the GROUP BY fields
func (p *Parser) parseSQLGroupBy() ([]Expr, error) {
	var fields []Expr

	for {
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		fields = append(fields, expr)

		if !p.match(TokenComma) {
			break
		}
	}

	return fields, nil
}

// checkJoin checks if the current token starts a JOIN clause
func (p *Parser) checkJoin() bool {
	switch p.current().Type {
	case TokenJoin, TokenInner, TokenLeft, TokenRight, TokenFull:
		return true
	}
	return false
}

// checkKeyword checks if the current token is a keyword (not an identifier to use as alias)
func (p *Parser) checkKeyword() bool {
	switch p.current().Type {
	case TokenFrom, TokenWhere, TokenGroup, TokenHaving, TokenOrder, TokenLimit, TokenJoin,
		TokenInner, TokenLeft, TokenRight, TokenFull, TokenOn, TokenAnd, TokenOr:
		return true
	}
	return false
}

// parseDuration parses duration strings like "1h", "30m", "1d"
func parseDuration(s string) (time.Duration, error) {
	// Handle day and week suffixes
	s = strings.ToLower(s)
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
