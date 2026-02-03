// Package promql implements a PromQL parser and query engine for Prometheus compatibility.
package promql

import (
	"fmt"
	"strings"
	"time"
)

// NodeType identifies the type of AST node.
type NodeType int

const (
	NodeVectorSelector NodeType = iota
	NodeMatrixSelector
	NodeNumberLiteral
	NodeStringLiteral
	NodeUnaryExpr
	NodeBinaryExpr
	NodeAggregateExpr
	NodeCall
	NodeSubquery
	NodeParenExpr
)

// Node is the interface implemented by all AST nodes.
type Node interface {
	Type() NodeType
	String() string
	Position() Pos
}

// Pos represents a position in the input string.
type Pos struct {
	Offset int
	Line   int
	Column int
}

// Expr is a PromQL expression node.
type Expr interface {
	Node
	exprNode()
}

// LabelMatchType represents the type of label matching operation.
type LabelMatchType int

const (
	MatchEqual      LabelMatchType = iota // =
	MatchNotEqual                         // !=
	MatchRegexp                           // =~
	MatchNotRegexp                        // !~
)

func (m LabelMatchType) String() string {
	switch m {
	case MatchEqual:
		return "="
	case MatchNotEqual:
		return "!="
	case MatchRegexp:
		return "=~"
	case MatchNotRegexp:
		return "!~"
	default:
		return "?"
	}
}

// LabelMatcher represents a label matching condition.
type LabelMatcher struct {
	Type  LabelMatchType
	Name  string
	Value string
}

func (m *LabelMatcher) String() string {
	return fmt.Sprintf("%s%s%q", m.Name, m.Type, m.Value)
}

// VectorSelector represents a metric selector with optional label matchers.
// Example: http_requests_total{job="api",code="200"}
type VectorSelector struct {
	Name          string
	LabelMatchers []*LabelMatcher
	Offset        time.Duration
	Timestamp     *int64 // @ modifier
	StartOrEnd    int    // 0 = none, 1 = start(), 2 = end()
	PosRange      Pos
}

func (v *VectorSelector) Type() NodeType   { return NodeVectorSelector }
func (v *VectorSelector) Position() Pos    { return v.PosRange }
func (v *VectorSelector) exprNode()        {}
func (v *VectorSelector) String() string {
	var b strings.Builder
	b.WriteString(v.Name)
	if len(v.LabelMatchers) > 0 {
		b.WriteByte('{')
		for i, m := range v.LabelMatchers {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(m.String())
		}
		b.WriteByte('}')
	}
	if v.Offset != 0 {
		b.WriteString(" offset ")
		b.WriteString(formatDuration(v.Offset))
	}
	if v.Timestamp != nil {
		b.WriteString(fmt.Sprintf(" @ %d", *v.Timestamp))
	}
	return b.String()
}

// MatrixSelector represents a range vector selector.
// Example: http_requests_total{job="api"}[5m]
type MatrixSelector struct {
	VectorSelector *VectorSelector
	Range          time.Duration
	PosRange       Pos
}

func (m *MatrixSelector) Type() NodeType   { return NodeMatrixSelector }
func (m *MatrixSelector) Position() Pos    { return m.PosRange }
func (m *MatrixSelector) exprNode()        {}
func (m *MatrixSelector) String() string {
	return fmt.Sprintf("%s[%s]", m.VectorSelector.String(), formatDuration(m.Range))
}

// NumberLiteral represents a numeric constant.
type NumberLiteral struct {
	Val      float64
	PosRange Pos
}

func (n *NumberLiteral) Type() NodeType   { return NodeNumberLiteral }
func (n *NumberLiteral) Position() Pos    { return n.PosRange }
func (n *NumberLiteral) exprNode()        {}
func (n *NumberLiteral) String() string   { return fmt.Sprintf("%g", n.Val) }

// StringLiteral represents a string constant.
type StringLiteral struct {
	Val      string
	PosRange Pos
}

func (s *StringLiteral) Type() NodeType   { return NodeStringLiteral }
func (s *StringLiteral) Position() Pos    { return s.PosRange }
func (s *StringLiteral) exprNode()        {}
func (s *StringLiteral) String() string   { return fmt.Sprintf("%q", s.Val) }

// UnaryExpr represents a unary operation on a single expression.
type UnaryExpr struct {
	Op       ItemType // + or -
	Expr     Expr
	PosRange Pos
}

func (u *UnaryExpr) Type() NodeType   { return NodeUnaryExpr }
func (u *UnaryExpr) Position() Pos    { return u.PosRange }
func (u *UnaryExpr) exprNode()        {}
func (u *UnaryExpr) String() string {
	return fmt.Sprintf("%s%s", u.Op, u.Expr.String())
}

// BinaryOp represents a binary operator.
type BinaryOp int

const (
	OpAdd BinaryOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpPow
	OpEql
	OpNeq
	OpLss
	OpGtr
	OpLte
	OpGte
	OpAnd
	OpOr
	OpUnless
	OpAtan2
)

func (op BinaryOp) String() string {
	switch op {
	case OpAdd:
		return "+"
	case OpSub:
		return "-"
	case OpMul:
		return "*"
	case OpDiv:
		return "/"
	case OpMod:
		return "%"
	case OpPow:
		return "^"
	case OpEql:
		return "=="
	case OpNeq:
		return "!="
	case OpLss:
		return "<"
	case OpGtr:
		return ">"
	case OpLte:
		return "<="
	case OpGte:
		return ">="
	case OpAnd:
		return "and"
	case OpOr:
		return "or"
	case OpUnless:
		return "unless"
	case OpAtan2:
		return "atan2"
	default:
		return "?"
	}
}

// IsComparisonOp returns true if the operator is a comparison operator.
func (op BinaryOp) IsComparisonOp() bool {
	switch op {
	case OpEql, OpNeq, OpLss, OpGtr, OpLte, OpGte:
		return true
	}
	return false
}

// IsSetOp returns true if the operator is a set operator.
func (op BinaryOp) IsSetOp() bool {
	switch op {
	case OpAnd, OpOr, OpUnless:
		return true
	}
	return false
}

// VectorMatchCardinality represents the cardinality of a vector matching operation.
type VectorMatchCardinality int

const (
	CardOneToOne VectorMatchCardinality = iota
	CardManyToOne
	CardOneToMany
	CardManyToMany
)

func (c VectorMatchCardinality) String() string {
	switch c {
	case CardOneToOne:
		return "one-to-one"
	case CardManyToOne:
		return "many-to-one"
	case CardOneToMany:
		return "one-to-many"
	case CardManyToMany:
		return "many-to-many"
	default:
		return "?"
	}
}

// VectorMatching describes how vector matching is performed in binary operations.
type VectorMatching struct {
	Card           VectorMatchCardinality
	MatchingLabels []string
	On             bool // true for on(), false for ignoring()
	Include        []string // group_left/group_right labels
}

// BinaryExpr represents a binary operation between two expressions.
type BinaryExpr struct {
	Op             BinaryOp
	LHS            Expr
	RHS            Expr
	VectorMatching *VectorMatching
	ReturnBool     bool // bool modifier for comparison ops
	PosRange       Pos
}

func (b *BinaryExpr) Type() NodeType   { return NodeBinaryExpr }
func (b *BinaryExpr) Position() Pos    { return b.PosRange }
func (b *BinaryExpr) exprNode()        {}
func (b *BinaryExpr) String() string {
	var matching string
	if b.VectorMatching != nil {
		if b.VectorMatching.On {
			matching = fmt.Sprintf(" on(%s)", strings.Join(b.VectorMatching.MatchingLabels, ","))
		} else if len(b.VectorMatching.MatchingLabels) > 0 {
			matching = fmt.Sprintf(" ignoring(%s)", strings.Join(b.VectorMatching.MatchingLabels, ","))
		}
		if len(b.VectorMatching.Include) > 0 {
			switch b.VectorMatching.Card {
			case CardManyToOne:
				matching += fmt.Sprintf(" group_left(%s)", strings.Join(b.VectorMatching.Include, ","))
			case CardOneToMany:
				matching += fmt.Sprintf(" group_right(%s)", strings.Join(b.VectorMatching.Include, ","))
			}
		}
	}
	boolMod := ""
	if b.ReturnBool {
		boolMod = " bool"
	}
	return fmt.Sprintf("%s %s%s%s %s", b.LHS.String(), b.Op, boolMod, matching, b.RHS.String())
}

// AggregateModifier specifies whether aggregation uses by or without.
type AggregateModifier int

const (
	AggregateModifierNone AggregateModifier = iota
	AggregateModifierBy
	AggregateModifierWithout
)

// AggregateExpr represents an aggregation operation.
// Example: sum(rate(http_requests_total[5m])) by (job)
type AggregateExpr struct {
	Op       ItemType // aggregation operator (sum, avg, etc.)
	Expr     Expr
	Param    Expr     // parameter for topk, bottomk, quantile, count_values
	Grouping []string // labels for by/without
	Without  bool     // true for without, false for by
	PosRange Pos
}

func (a *AggregateExpr) Type() NodeType   { return NodeAggregateExpr }
func (a *AggregateExpr) Position() Pos    { return a.PosRange }
func (a *AggregateExpr) exprNode()        {}
func (a *AggregateExpr) String() string {
	var b strings.Builder
	b.WriteString(a.Op.String())
	if len(a.Grouping) > 0 {
		if a.Without {
			b.WriteString(" without(")
		} else {
			b.WriteString(" by(")
		}
		b.WriteString(strings.Join(a.Grouping, ","))
		b.WriteByte(')')
	}
	b.WriteByte('(')
	if a.Param != nil {
		b.WriteString(a.Param.String())
		b.WriteByte(',')
	}
	b.WriteString(a.Expr.String())
	b.WriteByte(')')
	return b.String()
}

// Call represents a function call.
// Example: rate(http_requests_total[5m])
type Call struct {
	Func     *Function
	Args     []Expr
	PosRange Pos
}

func (c *Call) Type() NodeType   { return NodeCall }
func (c *Call) Position() Pos    { return c.PosRange }
func (c *Call) exprNode()        {}
func (c *Call) String() string {
	var args []string
	for _, arg := range c.Args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("%s(%s)", c.Func.Name, strings.Join(args, ","))
}

// Subquery represents a subquery expression.
// Example: http_requests_total[5m:1m]
type Subquery struct {
	Expr     Expr
	Range    time.Duration
	Step     time.Duration
	Offset   time.Duration
	PosRange Pos
}

func (s *Subquery) Type() NodeType   { return NodeSubquery }
func (s *Subquery) Position() Pos    { return s.PosRange }
func (s *Subquery) exprNode()        {}
func (s *Subquery) String() string {
	step := ""
	if s.Step > 0 {
		step = ":" + formatDuration(s.Step)
	}
	offset := ""
	if s.Offset > 0 {
		offset = " offset " + formatDuration(s.Offset)
	}
	return fmt.Sprintf("%s[%s%s]%s", s.Expr.String(), formatDuration(s.Range), step, offset)
}

// ParenExpr represents a parenthesized expression.
type ParenExpr struct {
	Expr     Expr
	PosRange Pos
}

func (p *ParenExpr) Type() NodeType   { return NodeParenExpr }
func (p *ParenExpr) Position() Pos    { return p.PosRange }
func (p *ParenExpr) exprNode()        {}
func (p *ParenExpr) String() string   { return fmt.Sprintf("(%s)", p.Expr.String()) }

// formatDuration formats a duration in PromQL style (e.g., 5m, 1h30m).
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	var b strings.Builder

	if d >= 365*24*time.Hour {
		years := d / (365 * 24 * time.Hour)
		b.WriteString(fmt.Sprintf("%dy", years))
		d -= years * 365 * 24 * time.Hour
	}
	if d >= 7*24*time.Hour {
		weeks := d / (7 * 24 * time.Hour)
		b.WriteString(fmt.Sprintf("%dw", weeks))
		d -= weeks * 7 * 24 * time.Hour
	}
	if d >= 24*time.Hour {
		days := d / (24 * time.Hour)
		b.WriteString(fmt.Sprintf("%dd", days))
		d -= days * 24 * time.Hour
	}
	if d >= time.Hour {
		hours := d / time.Hour
		b.WriteString(fmt.Sprintf("%dh", hours))
		d -= hours * time.Hour
	}
	if d >= time.Minute {
		minutes := d / time.Minute
		b.WriteString(fmt.Sprintf("%dm", minutes))
		d -= minutes * time.Minute
	}
	if d >= time.Second {
		seconds := d / time.Second
		b.WriteString(fmt.Sprintf("%ds", seconds))
		d -= seconds * time.Second
	}
	if d >= time.Millisecond {
		ms := d / time.Millisecond
		b.WriteString(fmt.Sprintf("%dms", ms))
	}

	return b.String()
}

// Walk traverses an AST in depth-first order.
func Walk(v Visitor, node Node) {
	if v = v.Visit(node); v == nil {
		return
	}

	switch n := node.(type) {
	case *VectorSelector:
		// leaf node
	case *MatrixSelector:
		Walk(v, n.VectorSelector)
	case *NumberLiteral:
		// leaf node
	case *StringLiteral:
		// leaf node
	case *UnaryExpr:
		Walk(v, n.Expr)
	case *BinaryExpr:
		Walk(v, n.LHS)
		Walk(v, n.RHS)
	case *AggregateExpr:
		if n.Param != nil {
			Walk(v, n.Param)
		}
		Walk(v, n.Expr)
	case *Call:
		for _, arg := range n.Args {
			Walk(v, arg)
		}
	case *Subquery:
		Walk(v, n.Expr)
	case *ParenExpr:
		Walk(v, n.Expr)
	}
}

// Visitor is the interface for AST visitors.
type Visitor interface {
	Visit(node Node) Visitor
}

// Inspector is a function that visits AST nodes.
type Inspector func(Node) bool

// Visit implements the Visitor interface.
func (f Inspector) Visit(node Node) Visitor {
	if f(node) {
		return f
	}
	return nil
}

// Inspect traverses an AST in depth-first order, calling f for each node.
func Inspect(node Node, f func(Node) bool) {
	Walk(Inspector(f), node)
}
