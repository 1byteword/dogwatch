package promql

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseError represents a parsing error with context.
type ParseError struct {
	Pos     Pos
	Message string
	Query   string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Pos.Line, e.Pos.Column, e.Message)
}

// Parser parses PromQL expressions.
type Parser struct {
	lexer *Lexer
	token Item
	query string
}

// Parse parses a PromQL expression and returns the AST.
func Parse(query string) (Expr, error) {
	p := &Parser{
		lexer: NewLexer(query),
		query: query,
	}
	p.advance()
	return p.parseExpr()
}

func (p *Parser) advance() {
	p.token = p.lexer.NextItem()
}

func (p *Parser) expect(typ ItemType) error {
	if p.token.Typ != typ {
		return p.errorf("expected %s, got %s", typ, p.token.Typ)
	}
	p.advance()
	return nil
}

func (p *Parser) errorf(format string, args ...interface{}) error {
	return &ParseError{
		Pos:     p.token.Pos,
		Message: fmt.Sprintf(format, args...),
		Query:   p.query,
	}
}

// parseExpr parses an expression with operator precedence.
func (p *Parser) parseExpr() (Expr, error) {
	return p.parseOr()
}

func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.token.Typ == ItemOr {
		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             OpOr,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
	return left, nil
}

func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseUnless()
	if err != nil {
		return nil, err
	}

	for p.token.Typ == ItemAnd {
		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parseUnless()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             OpAnd,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
	return left, nil
}

func (p *Parser) parseUnless() (Expr, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}

	for p.token.Typ == ItemUnless {
		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             OpUnless,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
	return left, nil
}

func (p *Parser) parseComparison() (Expr, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}

	for {
		var op BinaryOp
		switch p.token.Typ {
		case ItemEql:
			op = OpEql
		case ItemNeq:
			op = OpNeq
		case ItemLss:
			op = OpLss
		case ItemGtr:
			op = OpGtr
		case ItemLte:
			op = OpLte
		case ItemGte:
			op = OpGte
		default:
			return left, nil
		}

		pos := p.token.Pos
		p.advance()

		// Check for bool modifier
		returnBool := false
		if p.token.Typ == ItemBool {
			returnBool = true
			p.advance()
		}

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             op,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			ReturnBool:     returnBool,
			PosRange:       pos,
		}
	}
}

func (p *Parser) parseAdditive() (Expr, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}

	for {
		var op BinaryOp
		switch p.token.Typ {
		case ItemAdd:
			op = OpAdd
		case ItemSub:
			op = OpSub
		default:
			return left, nil
		}

		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             op,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
}

func (p *Parser) parseMultiplicative() (Expr, error) {
	left, err := p.parsePower()
	if err != nil {
		return nil, err
	}

	for {
		var op BinaryOp
		switch p.token.Typ {
		case ItemMul:
			op = OpMul
		case ItemDiv:
			op = OpDiv
		case ItemMod:
			op = OpMod
		case ItemAtan2:
			op = OpAtan2
		default:
			return left, nil
		}

		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		right, err := p.parsePower()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             op,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
}

func (p *Parser) parsePower() (Expr, error) {
	left, err := p.parseUnaryExpr()
	if err != nil {
		return nil, err
	}

	if p.token.Typ == ItemPow {
		pos := p.token.Pos
		p.advance()

		vm, err := p.parseVectorMatching()
		if err != nil {
			return nil, err
		}

		// Power is right-associative
		right, err := p.parsePower()
		if err != nil {
			return nil, err
		}

		left = &BinaryExpr{
			Op:             OpPow,
			LHS:            left,
			RHS:            right,
			VectorMatching: vm,
			PosRange:       pos,
		}
	}
	return left, nil
}

func (p *Parser) parseUnaryExpr() (Expr, error) {
	switch p.token.Typ {
	case ItemAdd:
		pos := p.token.Pos
		p.advance()
		expr, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: ItemAdd, Expr: expr, PosRange: pos}, nil

	case ItemSub:
		pos := p.token.Pos
		p.advance()
		expr, err := p.parseUnaryExpr()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: ItemSub, Expr: expr, PosRange: pos}, nil
	}

	return p.parsePrimaryExpr()
}

func (p *Parser) parsePrimaryExpr() (Expr, error) {
	switch p.token.Typ {
	case ItemNumber:
		return p.parseNumber()

	case ItemString:
		return p.parseString()

	case ItemLeftParen:
		return p.parseParen()

	case ItemIdentifier:
		return p.parseVectorSelectorOrFunction()

	case ItemSum, ItemAvg, ItemMin, ItemMax, ItemCount,
		ItemStddev, ItemStdvar, ItemTopK, ItemBottomK,
		ItemCountValues, ItemQuantile, ItemGroup:
		return p.parseAggregateExpr()

	default:
		return nil, p.errorf("unexpected token %s", p.token.Typ)
	}
}

func (p *Parser) parseNumber() (Expr, error) {
	pos := p.token.Pos
	val, err := strconv.ParseFloat(p.token.Val, 64)
	if err != nil {
		// Handle Inf and NaN
		lower := strings.ToLower(p.token.Val)
		if lower == "inf" || lower == "+inf" {
			val = positiveInf
		} else if lower == "-inf" {
			val = negativeInf
		} else if lower == "nan" {
			val = nan
		} else {
			return nil, p.errorf("invalid number: %s", p.token.Val)
		}
	}
	p.advance()
	return &NumberLiteral{Val: val, PosRange: pos}, nil
}

var (
	positiveInf = math.Inf(1)
	negativeInf = math.Inf(-1)
	nan         = math.NaN()
)

func (p *Parser) parseString() (Expr, error) {
	pos := p.token.Pos
	val := p.token.Val
	p.advance()
	return &StringLiteral{Val: val, PosRange: pos}, nil
}

func (p *Parser) parseParen() (Expr, error) {
	pos := p.token.Pos
	if err := p.expect(ItemLeftParen); err != nil {
		return nil, err
	}

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if err := p.expect(ItemRightParen); err != nil {
		return nil, err
	}

	return &ParenExpr{Expr: expr, PosRange: pos}, nil
}

func (p *Parser) parseVectorSelectorOrFunction() (Expr, error) {
	name := p.token.Val
	pos := p.token.Pos
	p.advance()

	// Check if this is a function call
	if p.token.Typ == ItemLeftParen {
		return p.parseFunctionCall(name, pos)
	}

	// It's a vector selector
	return p.parseVectorSelector(name, pos)
}

func (p *Parser) parseFunctionCall(name string, pos Pos) (Expr, error) {
	fn, ok := GetFunction(name)
	if !ok {
		return nil, p.errorf("unknown function: %s", name)
	}

	if err := p.expect(ItemLeftParen); err != nil {
		return nil, err
	}

	args, err := p.parseCallArgs()
	if err != nil {
		return nil, err
	}

	if err := p.expect(ItemRightParen); err != nil {
		return nil, err
	}

	return &Call{
		Func:     fn,
		Args:     args,
		PosRange: pos,
	}, nil
}

func (p *Parser) parseCallArgs() ([]Expr, error) {
	var args []Expr

	if p.token.Typ == ItemRightParen {
		return args, nil
	}

	for {
		arg, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)

		if p.token.Typ != ItemComma {
			break
		}
		p.advance()
	}

	return args, nil
}

func (p *Parser) parseVectorSelector(name string, pos Pos) (Expr, error) {
	vs := &VectorSelector{
		Name:     name,
		PosRange: pos,
	}

	// Parse label matchers
	if p.token.Typ == ItemLeftBrace {
		matchers, err := p.parseLabelMatchers()
		if err != nil {
			return nil, err
		}
		vs.LabelMatchers = matchers
	}

	// Check for range selector [5m]
	if p.token.Typ == ItemLeftBracket {
		return p.parseMatrixSelector(vs)
	}

	// Parse offset modifier
	if p.token.Typ == ItemOffset {
		p.advance()
		offset, err := p.parseDuration()
		if err != nil {
			return nil, err
		}
		vs.Offset = offset
	}

	// Parse @ modifier
	if p.token.Typ == ItemAt {
		p.advance()
		if p.token.Typ == ItemStart {
			vs.StartOrEnd = 1
			p.advance()
			if err := p.expect(ItemLeftParen); err != nil {
				return nil, err
			}
			if err := p.expect(ItemRightParen); err != nil {
				return nil, err
			}
		} else if p.token.Typ == ItemEnd {
			vs.StartOrEnd = 2
			p.advance()
			if err := p.expect(ItemLeftParen); err != nil {
				return nil, err
			}
			if err := p.expect(ItemRightParen); err != nil {
				return nil, err
			}
		} else if p.token.Typ == ItemNumber {
			ts, err := strconv.ParseInt(p.token.Val, 10, 64)
			if err != nil {
				return nil, p.errorf("invalid timestamp: %s", p.token.Val)
			}
			vs.Timestamp = &ts
			p.advance()
		} else {
			return nil, p.errorf("expected timestamp or start()/end() after @")
		}
	}

	return vs, nil
}

func (p *Parser) parseMatrixSelector(vs *VectorSelector) (Expr, error) {
	pos := p.token.Pos
	if err := p.expect(ItemLeftBracket); err != nil {
		return nil, err
	}

	// Parse range duration
	rangeDur, err := p.parseDuration()
	if err != nil {
		return nil, err
	}

	// Check for subquery step [:1m]
	if p.token.Typ == ItemColon {
		p.advance()
		step := time.Duration(0)
		if p.token.Typ == ItemDuration {
			step, err = p.parseDuration()
			if err != nil {
				return nil, err
			}
		}

		if err := p.expect(ItemRightBracket); err != nil {
			return nil, err
		}

		subquery := &Subquery{
			Expr:     vs,
			Range:    rangeDur,
			Step:     step,
			PosRange: pos,
		}

		// Parse offset for subquery
		if p.token.Typ == ItemOffset {
			p.advance()
			offset, err := p.parseDuration()
			if err != nil {
				return nil, err
			}
			subquery.Offset = offset
		}

		return subquery, nil
	}

	if err := p.expect(ItemRightBracket); err != nil {
		return nil, err
	}

	ms := &MatrixSelector{
		VectorSelector: vs,
		Range:          rangeDur,
		PosRange:       pos,
	}

	// Parse offset modifier after matrix selector
	if p.token.Typ == ItemOffset {
		p.advance()
		offset, err := p.parseDuration()
		if err != nil {
			return nil, err
		}
		vs.Offset = offset
	}

	// Parse @ modifier
	if p.token.Typ == ItemAt {
		p.advance()
		if p.token.Typ == ItemStart {
			vs.StartOrEnd = 1
			p.advance()
			if err := p.expect(ItemLeftParen); err != nil {
				return nil, err
			}
			if err := p.expect(ItemRightParen); err != nil {
				return nil, err
			}
		} else if p.token.Typ == ItemEnd {
			vs.StartOrEnd = 2
			p.advance()
			if err := p.expect(ItemLeftParen); err != nil {
				return nil, err
			}
			if err := p.expect(ItemRightParen); err != nil {
				return nil, err
			}
		} else if p.token.Typ == ItemNumber {
			ts, err := strconv.ParseInt(p.token.Val, 10, 64)
			if err != nil {
				return nil, p.errorf("invalid timestamp: %s", p.token.Val)
			}
			vs.Timestamp = &ts
			p.advance()
		} else {
			return nil, p.errorf("expected timestamp or start()/end() after @")
		}
	}

	return ms, nil
}

func (p *Parser) parseLabelMatchers() ([]*LabelMatcher, error) {
	if err := p.expect(ItemLeftBrace); err != nil {
		return nil, err
	}

	var matchers []*LabelMatcher

	if p.token.Typ == ItemRightBrace {
		p.advance()
		return matchers, nil
	}

	for {
		m, err := p.parseLabelMatcher()
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, m)

		if p.token.Typ != ItemComma {
			break
		}
		p.advance()

		// Allow trailing comma
		if p.token.Typ == ItemRightBrace {
			break
		}
	}

	if err := p.expect(ItemRightBrace); err != nil {
		return nil, err
	}

	return matchers, nil
}

func (p *Parser) parseLabelMatcher() (*LabelMatcher, error) {
	if p.token.Typ != ItemIdentifier {
		return nil, p.errorf("expected label name, got %s", p.token.Typ)
	}

	name := p.token.Val
	p.advance()

	var matchType LabelMatchType
	switch p.token.Typ {
	case ItemAssign:
		matchType = MatchEqual
	case ItemNeq:
		matchType = MatchNotEqual
	case ItemEqlMatch:
		matchType = MatchRegexp
	case ItemNeqMatch:
		matchType = MatchNotRegexp
	default:
		return nil, p.errorf("expected label match operator, got %s", p.token.Typ)
	}
	p.advance()

	if p.token.Typ != ItemString {
		return nil, p.errorf("expected string, got %s", p.token.Typ)
	}

	value := p.token.Val
	p.advance()

	return &LabelMatcher{
		Type:  matchType,
		Name:  name,
		Value: value,
	}, nil
}

func (p *Parser) parseDuration() (time.Duration, error) {
	if p.token.Typ != ItemDuration && p.token.Typ != ItemNumber {
		return 0, p.errorf("expected duration, got %s", p.token.Typ)
	}

	val := p.token.Val
	p.advance()

	return ParseDuration(val)
}

// ParseDuration parses a PromQL duration string.
func ParseDuration(s string) (time.Duration, error) {
	var total time.Duration
	i := 0

	for i < len(s) {
		// Parse number
		j := i
		for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == '.') {
			j++
		}
		if j == i {
			return 0, fmt.Errorf("invalid duration: %s", s)
		}

		numStr := s[i:j]
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration number: %s", numStr)
		}

		if j >= len(s) {
			// Just a number - assume seconds
			return time.Duration(num * float64(time.Second)), nil
		}

		// Parse unit
		unit := s[j]
		j++

		// Check for 'ms'
		if unit == 'm' && j < len(s) && s[j] == 's' {
			j++
			total += time.Duration(num * float64(time.Millisecond))
			i = j
			continue
		}

		var d time.Duration
		switch unit {
		case 'y':
			d = time.Duration(num * float64(365*24*time.Hour))
		case 'w':
			d = time.Duration(num * float64(7*24*time.Hour))
		case 'd':
			d = time.Duration(num * float64(24*time.Hour))
		case 'h':
			d = time.Duration(num * float64(time.Hour))
		case 'm':
			d = time.Duration(num * float64(time.Minute))
		case 's':
			d = time.Duration(num * float64(time.Second))
		default:
			return 0, fmt.Errorf("invalid duration unit: %c", unit)
		}

		total += d
		i = j
	}

	return total, nil
}

func (p *Parser) parseAggregateExpr() (Expr, error) {
	op := p.token.Typ
	pos := p.token.Pos
	p.advance()

	// Parse optional grouping before args: sum by (labels) (expr)
	var grouping []string
	without := false

	if p.token.Typ == ItemBy || p.token.Typ == ItemWithout {
		without = p.token.Typ == ItemWithout
		p.advance()

		var err error
		grouping, err = p.parseGrouping()
		if err != nil {
			return nil, err
		}
	}

	if err := p.expect(ItemLeftParen); err != nil {
		return nil, err
	}

	// Parse optional parameter for topk, bottomk, quantile, count_values
	var param Expr
	if op == ItemTopK || op == ItemBottomK || op == ItemQuantile {
		var err error
		param, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(ItemComma); err != nil {
			return nil, err
		}
	} else if op == ItemCountValues {
		var err error
		param, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
		if err := p.expect(ItemComma); err != nil {
			return nil, err
		}
	}

	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}

	if err := p.expect(ItemRightParen); err != nil {
		return nil, err
	}

	// Parse optional grouping after args: sum(expr) by (labels)
	if grouping == nil && (p.token.Typ == ItemBy || p.token.Typ == ItemWithout) {
		without = p.token.Typ == ItemWithout
		p.advance()

		grouping, err = p.parseGrouping()
		if err != nil {
			return nil, err
		}
	}

	return &AggregateExpr{
		Op:       op,
		Expr:     expr,
		Param:    param,
		Grouping: grouping,
		Without:  without,
		PosRange: pos,
	}, nil
}

func (p *Parser) parseGrouping() ([]string, error) {
	if err := p.expect(ItemLeftParen); err != nil {
		return nil, err
	}

	var labels []string
	if p.token.Typ == ItemRightParen {
		p.advance()
		return labels, nil
	}

	for {
		if p.token.Typ != ItemIdentifier {
			return nil, p.errorf("expected label name, got %s", p.token.Typ)
		}
		labels = append(labels, p.token.Val)
		p.advance()

		if p.token.Typ != ItemComma {
			break
		}
		p.advance()
	}

	if err := p.expect(ItemRightParen); err != nil {
		return nil, err
	}

	return labels, nil
}

func (p *Parser) parseVectorMatching() (*VectorMatching, error) {
	if p.token.Typ != ItemOn && p.token.Typ != ItemIgnoring {
		return nil, nil
	}

	vm := &VectorMatching{
		Card: CardOneToOne,
	}

	vm.On = p.token.Typ == ItemOn
	p.advance()

	labels, err := p.parseGrouping()
	if err != nil {
		return nil, err
	}
	vm.MatchingLabels = labels

	// Check for group_left/group_right
	if p.token.Typ == ItemGroupLeft {
		vm.Card = CardManyToOne
		p.advance()
		if p.token.Typ == ItemLeftParen {
			include, err := p.parseGrouping()
			if err != nil {
				return nil, err
			}
			vm.Include = include
		}
	} else if p.token.Typ == ItemGroupRight {
		vm.Card = CardOneToMany
		p.advance()
		if p.token.Typ == ItemLeftParen {
			include, err := p.parseGrouping()
			if err != nil {
				return nil, err
			}
			vm.Include = include
		}
	}

	return vm, nil
}
