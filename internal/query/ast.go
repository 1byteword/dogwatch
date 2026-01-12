package query

import (
	"fmt"
	"time"
)

// NodeType represents the type of AST node
type NodeType int

const (
	NodeSource NodeType = iota
	NodePipe
	NodeWhere
	NodeSelect
	NodeGroupBy
	NodeOrderBy
	NodeLimit
	NodeWindow
	NodeCorrelate
	NodeAnomalies
	NodeExtract
	NodeHistogram
	NodeDefine
	NodeFunctionCall
	NodeBinaryExpr
	NodeUnaryExpr
	NodeIdentifier
	NodeLiteral
	NodeTimeRange
	NodePattern
)

// Node is the interface for all AST nodes
type Node interface {
	Type() NodeType
	String() string
	Position() Position
}

// Position tracks source location for error messages
type Position struct {
	Line   int
	Column int
	Offset int
}

func (p Position) String() string {
	return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
}

// Query represents a complete WatchQL query
type Query struct {
	Source     *SourceNode
	Pipes      []PipeNode
	TimeRange  *TimeRangeNode
	Pos        Position
	Definitions map[string]*DefineNode
}

func (q *Query) Type() NodeType    { return NodeSource }
func (q *Query) Position() Position { return q.Pos }
func (q *Query) String() string {
	s := q.Source.String()
	for _, p := range q.Pipes {
		s += " | " + p.String()
	}
	if q.TimeRange != nil {
		s += " " + q.TimeRange.String()
	}
	return s
}

// SourceNode represents the data source (metrics, logs, traces)
type SourceNode struct {
	SourceType string // "metrics", "logs", "traces", "events"
	MetricName string // optional: metrics.cpu_percent
	Pos        Position
}

func (n *SourceNode) Type() NodeType    { return NodeSource }
func (n *SourceNode) Position() Position { return n.Pos }
func (n *SourceNode) String() string {
	if n.MetricName != "" {
		return n.SourceType + "." + n.MetricName
	}
	return n.SourceType
}

// PipeNode is the interface for pipe operations
type PipeNode interface {
	Node
	isPipe()
}

// WhereNode represents a filter condition
type WhereNode struct {
	Condition Expr
	Pos       Position
}

func (n *WhereNode) Type() NodeType    { return NodeWhere }
func (n *WhereNode) Position() Position { return n.Pos }
func (n *WhereNode) String() string     { return "where " + n.Condition.String() }
func (n *WhereNode) isPipe()            {}

// SelectNode represents column selection
type SelectNode struct {
	Fields []SelectField
	Pos    Position
}

type SelectField struct {
	Expr  Expr
	Alias string
}

func (n *SelectNode) Type() NodeType    { return NodeSelect }
func (n *SelectNode) Position() Position { return n.Pos }
func (n *SelectNode) String() string {
	s := "select "
	for i, f := range n.Fields {
		if i > 0 {
			s += ", "
		}
		s += f.Expr.String()
		if f.Alias != "" {
			s += " as " + f.Alias
		}
	}
	return s
}
func (n *SelectNode) isPipe() {}

// GroupByNode represents aggregation
type GroupByNode struct {
	Aggregations []AggregationExpr
	GroupFields  []string
	Pos          Position
}

type AggregationExpr struct {
	Function string // avg, sum, count, min, max, p50, p95, p99
	Field    string
	Alias    string
}

func (n *GroupByNode) Type() NodeType    { return NodeGroupBy }
func (n *GroupByNode) Position() Position { return n.Pos }
func (n *GroupByNode) String() string {
	s := ""
	for i, agg := range n.Aggregations {
		if i > 0 {
			s += ", "
		}
		s += agg.Function
		if agg.Field != "" {
			s += "(" + agg.Field + ")"
		}
	}
	if len(n.GroupFields) > 0 {
		s += " by ("
		for i, f := range n.GroupFields {
			if i > 0 {
				s += ", "
			}
			s += f
		}
		s += ")"
	}
	return s
}
func (n *GroupByNode) isPipe() {}

// OrderByNode represents sorting
type OrderByNode struct {
	Fields []OrderField
	Pos    Position
}

type OrderField struct {
	Field string
	Desc  bool
}

func (n *OrderByNode) Type() NodeType    { return NodeOrderBy }
func (n *OrderByNode) Position() Position { return n.Pos }
func (n *OrderByNode) String() string {
	s := "order by "
	for i, f := range n.Fields {
		if i > 0 {
			s += ", "
		}
		s += f.Field
		if f.Desc {
			s += " desc"
		}
	}
	return s
}
func (n *OrderByNode) isPipe() {}

// LimitNode represents result limiting
type LimitNode struct {
	Count  int
	Offset int
	Pos    Position
}

func (n *LimitNode) Type() NodeType     { return NodeLimit }
func (n *LimitNode) Position() Position { return n.Pos }
func (n *LimitNode) String() string {
	s := fmt.Sprintf("limit %d", n.Count)
	if n.Offset > 0 {
		s += fmt.Sprintf(" offset %d", n.Offset)
	}
	return s
}
func (n *LimitNode) isPipe() {}

// TopNode is syntactic sugar for order + limit
type TopNode struct {
	Count int
	Field string
	Pos   Position
}

func (n *TopNode) Type() NodeType     { return NodeLimit }
func (n *TopNode) Position() Position { return n.Pos }
func (n *TopNode) String() string     { return fmt.Sprintf("top %d", n.Count) }
func (n *TopNode) isPipe()            {}

// WindowNode represents windowing operations
type WindowNode struct {
	WindowType string        // "tumbling", "sliding", "session"
	Duration   time.Duration
	Slide      time.Duration // for sliding windows
	Pos        Position
}

func (n *WindowNode) Type() NodeType     { return NodeWindow }
func (n *WindowNode) Position() Position { return n.Pos }
func (n *WindowNode) String() string {
	return fmt.Sprintf("window %s=%s", n.WindowType, n.Duration)
}
func (n *WindowNode) isPipe() {}

// CorrelateNode represents cross-data-type correlation
type CorrelateNode struct {
	Target     string        // "logs", "traces", "metrics"
	JoinFields []string      // fields to join on
	TimeTolerance time.Duration // time ± tolerance
	Pos        Position
}

func (n *CorrelateNode) Type() NodeType     { return NodeCorrelate }
func (n *CorrelateNode) Position() Position { return n.Pos }
func (n *CorrelateNode) String() string {
	s := "correlate " + n.Target + " on ("
	for i, f := range n.JoinFields {
		if i > 0 {
			s += ", "
		}
		s += f
	}
	s += ")"
	if n.TimeTolerance > 0 {
		s += fmt.Sprintf(" time ± %s", n.TimeTolerance)
	}
	return s
}
func (n *CorrelateNode) isPipe() {}

// AnomaliesNode represents anomaly detection
type AnomaliesNode struct {
	Algorithm   string  // "zscore", "isolation_forest", "xgboost"
	Sensitivity float64
	Pos         Position
}

func (n *AnomaliesNode) Type() NodeType     { return NodeAnomalies }
func (n *AnomaliesNode) Position() Position { return n.Pos }
func (n *AnomaliesNode) String() string {
	return fmt.Sprintf("anomalies algorithm=%s sensitivity=%.2f", n.Algorithm, n.Sensitivity)
}
func (n *AnomaliesNode) isPipe() {}

// ExtractNode represents pattern extraction from logs
type ExtractNode struct {
	Pattern string
	Auto    bool // use ML-based extraction
	Pos     Position
}

func (n *ExtractNode) Type() NodeType     { return NodeExtract }
func (n *ExtractNode) Position() Position { return n.Pos }
func (n *ExtractNode) String() string {
	if n.Auto {
		return "extract auto"
	}
	return fmt.Sprintf("extract pattern=%q", n.Pattern)
}
func (n *ExtractNode) isPipe() {}

// HistogramNode represents histogram aggregation
type HistogramNode struct {
	Field   string
	Buckets int
	GroupBy []string
	Pos     Position
}

func (n *HistogramNode) Type() NodeType     { return NodeHistogram }
func (n *HistogramNode) Position() Position { return n.Pos }
func (n *HistogramNode) String() string {
	s := fmt.Sprintf("histogram %s buckets=%d", n.Field, n.Buckets)
	if len(n.GroupBy) > 0 {
		s += " by ("
		for i, f := range n.GroupBy {
			if i > 0 {
				s += ", "
			}
			s += f
		}
		s += ")"
	}
	return s
}
func (n *HistogramNode) isPipe() {}

// DefineNode represents a saved query definition
type DefineNode struct {
	Name  string
	Query *Query
	Pos   Position
}

func (n *DefineNode) Type() NodeType     { return NodeDefine }
func (n *DefineNode) Position() Position { return n.Pos }
func (n *DefineNode) String() string {
	return fmt.Sprintf("define %s = (%s)", n.Name, n.Query.String())
}

// TimeRangeNode represents time constraints
type TimeRangeNode struct {
	Relative   time.Duration // last 1h
	Start      time.Time     // absolute start
	End        time.Time     // absolute end
	Shift      time.Duration // shift -1w (compare to past)
	Pos        Position
}

func (n *TimeRangeNode) Type() NodeType     { return NodeTimeRange }
func (n *TimeRangeNode) Position() Position { return n.Pos }
func (n *TimeRangeNode) String() string {
	if n.Relative > 0 {
		return fmt.Sprintf("last %s", n.Relative)
	}
	return fmt.Sprintf("between %s and %s", n.Start.Format(time.RFC3339), n.End.Format(time.RFC3339))
}

// Expr is the interface for expressions
type Expr interface {
	Node
	isExpr()
}

// BinaryExpr represents binary operations (and, or, =, !=, <, >, etc.)
type BinaryExpr struct {
	Left     Expr
	Operator string
	Right    Expr
	Pos      Position
}

func (n *BinaryExpr) Type() NodeType     { return NodeBinaryExpr }
func (n *BinaryExpr) Position() Position { return n.Pos }
func (n *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", n.Left.String(), n.Operator, n.Right.String())
}
func (n *BinaryExpr) isExpr() {}

// UnaryExpr represents unary operations (not, -)
type UnaryExpr struct {
	Operator string
	Operand  Expr
	Pos      Position
}

func (n *UnaryExpr) Type() NodeType     { return NodeUnaryExpr }
func (n *UnaryExpr) Position() Position { return n.Pos }
func (n *UnaryExpr) String() string     { return n.Operator + n.Operand.String() }
func (n *UnaryExpr) isExpr()            {}

// IdentifierExpr represents field/column references
type IdentifierExpr struct {
	Name string
	Path []string // for nested: log.message, trace.spans[0].name
	Pos  Position
}

func (n *IdentifierExpr) Type() NodeType     { return NodeIdentifier }
func (n *IdentifierExpr) Position() Position { return n.Pos }
func (n *IdentifierExpr) String() string {
	if len(n.Path) == 0 {
		return n.Name
	}
	s := n.Name
	for _, p := range n.Path {
		s += "." + p
	}
	return s
}
func (n *IdentifierExpr) isExpr() {}

// LiteralExpr represents literal values
type LiteralExpr struct {
	Value     interface{} // string, int64, float64, bool, time.Duration
	ValueType LiteralType
	Pos       Position
}

type LiteralType int

const (
	LiteralString LiteralType = iota
	LiteralInt
	LiteralFloat
	LiteralBool
	LiteralDuration
	LiteralNull
	LiteralRegex
)

func (n *LiteralExpr) Type() NodeType     { return NodeLiteral }
func (n *LiteralExpr) Position() Position { return n.Pos }
func (n *LiteralExpr) String() string {
	switch n.ValueType {
	case LiteralString:
		return fmt.Sprintf("%q", n.Value)
	case LiteralDuration:
		return n.Value.(time.Duration).String()
	default:
		return fmt.Sprintf("%v", n.Value)
	}
}
func (n *LiteralExpr) isExpr() {}

// FunctionCallExpr represents function calls
type FunctionCallExpr struct {
	Name string
	Args []Expr
	Pos  Position
}

func (n *FunctionCallExpr) Type() NodeType     { return NodeFunctionCall }
func (n *FunctionCallExpr) Position() Position { return n.Pos }
func (n *FunctionCallExpr) String() string {
	s := n.Name + "("
	for i, arg := range n.Args {
		if i > 0 {
			s += ", "
		}
		s += arg.String()
	}
	return s + ")"
}
func (n *FunctionCallExpr) isExpr() {}

// ListExpr represents a list of values (for IN operator)
type ListExpr struct {
	Values []Expr
	Pos    Position
}

func (n *ListExpr) Type() NodeType     { return NodeLiteral }
func (n *ListExpr) Position() Position { return n.Pos }
func (n *ListExpr) String() string {
	s := "("
	for i, v := range n.Values {
		if i > 0 {
			s += ", "
		}
		s += v.String()
	}
	return s + ")"
}
func (n *ListExpr) isExpr() {}

// SubqueryExpr represents a subquery
type SubqueryExpr struct {
	Query *Query
	Pos   Position
}

func (n *SubqueryExpr) Type() NodeType     { return NodeSource }
func (n *SubqueryExpr) Position() Position { return n.Pos }
func (n *SubqueryExpr) String() string     { return "(" + n.Query.String() + ")" }
func (n *SubqueryExpr) isExpr()            {}
