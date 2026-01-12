package query

import (
	"fmt"
	"strings"
	"time"
)

// PlanNodeType represents the type of plan node
type PlanNodeType int

const (
	PlanScanType PlanNodeType = iota
	PlanFilterType
	PlanProjectType
	PlanAggregateType
	PlanSortType
	PlanLimitType
	PlanWindowType
	PlanJoinType
	PlanAnomalyType
	PlanExtractType
	PlanHistogramType
)

// PlanNode represents a node in the execution plan
type PlanNode interface {
	NodeType() PlanNodeType
	Children() []PlanNode
	String() string
	EstimatedCost() float64
}

// Plan represents a complete execution plan
type Plan struct {
	Root      PlanNode
	TimeRange TimeRangeSpec
	Query     *Query
}

// TimeRangeSpec specifies the time range for the query
type TimeRangeSpec struct {
	Start time.Time
	End   time.Time
	Shift time.Duration
}

// PlanScanNode reads data from a source
type PlanScanNode struct {
	Source     string   // metrics, logs, traces
	MetricName string   // optional specific metric
	Fields     []string // fields to read
	Predicates []Expr   // pushed-down predicates
}

func (n *PlanScanNode) NodeType() PlanNodeType { return PlanScanType }
func (n *PlanScanNode) Children() []PlanNode   { return nil }
func (n *PlanScanNode) EstimatedCost() float64 { return 100.0 }
func (n *PlanScanNode) String() string {
	s := fmt.Sprintf("Scan(%s", n.Source)
	if n.MetricName != "" {
		s += "." + n.MetricName
	}
	if len(n.Predicates) > 0 {
		s += fmt.Sprintf(", predicates=%d", len(n.Predicates))
	}
	return s + ")"
}

// PlanFilterNode filters rows based on a condition
type PlanFilterNode struct {
	Input     PlanNode
	Condition Expr
}

func (n *PlanFilterNode) NodeType() PlanNodeType { return PlanFilterType }
func (n *PlanFilterNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanFilterNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 0.5 }
func (n *PlanFilterNode) String() string {
	return fmt.Sprintf("Filter(%s)", n.Condition.String())
}

// PlanProjectNode selects and transforms columns
type PlanProjectNode struct {
	Input  PlanNode
	Fields []ProjectField
}

type ProjectField struct {
	Expr  Expr
	Alias string
}

func (n *PlanProjectNode) NodeType() PlanNodeType { return PlanProjectType }
func (n *PlanProjectNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanProjectNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 1.1 }
func (n *PlanProjectNode) String() string {
	return fmt.Sprintf("Project(%d fields)", len(n.Fields))
}

// PlanAggregateNode performs aggregation
type PlanAggregateNode struct {
	Input        PlanNode
	Aggregations []AggSpec
	GroupBy      []string
}

type AggSpec struct {
	Function string // avg, sum, count, min, max, p50, p95, p99
	Field    string
	Alias    string
}

func (n *PlanAggregateNode) NodeType() PlanNodeType { return PlanAggregateType }
func (n *PlanAggregateNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanAggregateNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 2.0 }
func (n *PlanAggregateNode) String() string {
	s := "Aggregate("
	for i, agg := range n.Aggregations {
		if i > 0 {
			s += ", "
		}
		s += agg.Function
		if agg.Field != "" {
			s += "(" + agg.Field + ")"
		}
	}
	if len(n.GroupBy) > 0 {
		s += fmt.Sprintf(" by %v", n.GroupBy)
	}
	return s + ")"
}

// PlanSortNode sorts results
type PlanSortNode struct {
	Input  PlanNode
	Fields []SortField
}

type SortField struct {
	Field string
	Desc  bool
}

func (n *PlanSortNode) NodeType() PlanNodeType { return PlanSortType }
func (n *PlanSortNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanSortNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 1.5 }
func (n *PlanSortNode) String() string {
	return fmt.Sprintf("Sort(%d fields)", len(n.Fields))
}

// PlanLimitNode limits results
type PlanLimitNode struct {
	Input  PlanNode
	Count  int
	Offset int
}

func (n *PlanLimitNode) NodeType() PlanNodeType { return PlanLimitType }
func (n *PlanLimitNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanLimitNode) EstimatedCost() float64 {
	cost := n.Input.EstimatedCost()
	if n.Count > 0 {
		return cost * (float64(n.Count) / 1000.0)
	}
	return cost
}
func (n *PlanLimitNode) String() string {
	return fmt.Sprintf("Limit(%d offset %d)", n.Count, n.Offset)
}

// PlanWindowNode applies windowing
type PlanWindowNode struct {
	Input      PlanNode
	WindowType string
	Duration   time.Duration
	Slide      time.Duration
}

func (n *PlanWindowNode) NodeType() PlanNodeType { return PlanWindowType }
func (n *PlanWindowNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanWindowNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 1.2 }
func (n *PlanWindowNode) String() string {
	return fmt.Sprintf("Window(%s=%s)", n.WindowType, n.Duration)
}

// PlanJoinNode correlates data from multiple sources
type PlanJoinNode struct {
	Left          PlanNode
	Right         PlanNode
	JoinFields    []string
	TimeTolerance time.Duration
}

func (n *PlanJoinNode) NodeType() PlanNodeType { return PlanJoinType }
func (n *PlanJoinNode) Children() []PlanNode   { return []PlanNode{n.Left, n.Right} }
func (n *PlanJoinNode) EstimatedCost() float64 {
	return (n.Left.EstimatedCost() + n.Right.EstimatedCost()) * 3.0
}
func (n *PlanJoinNode) String() string {
	return fmt.Sprintf("Join(on=%v, time±%s)", n.JoinFields, n.TimeTolerance)
}

// PlanAnomalyNode detects anomalies
type PlanAnomalyNode struct {
	Input       PlanNode
	Algorithm   string
	Sensitivity float64
}

func (n *PlanAnomalyNode) NodeType() PlanNodeType { return PlanAnomalyType }
func (n *PlanAnomalyNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanAnomalyNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 5.0 }
func (n *PlanAnomalyNode) String() string {
	return fmt.Sprintf("Anomaly(algo=%s, sens=%.2f)", n.Algorithm, n.Sensitivity)
}

// PlanExtractNode extracts fields from logs
type PlanExtractNode struct {
	Input   PlanNode
	Pattern string
	Auto    bool
}

func (n *PlanExtractNode) NodeType() PlanNodeType { return PlanExtractType }
func (n *PlanExtractNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanExtractNode) EstimatedCost() float64 {
	cost := n.Input.EstimatedCost()
	if n.Auto {
		return cost * 10.0 // ML extraction is expensive
	}
	return cost * 1.5
}
func (n *PlanExtractNode) String() string {
	if n.Auto {
		return "Extract(auto)"
	}
	return fmt.Sprintf("Extract(%q)", n.Pattern)
}

// PlanHistogramNode computes histograms
type PlanHistogramNode struct {
	Input   PlanNode
	Field   string
	Buckets int
	GroupBy []string
}

func (n *PlanHistogramNode) NodeType() PlanNodeType { return PlanHistogramType }
func (n *PlanHistogramNode) Children() []PlanNode   { return []PlanNode{n.Input} }
func (n *PlanHistogramNode) EstimatedCost() float64 { return n.Input.EstimatedCost() * 1.5 }
func (n *PlanHistogramNode) String() string {
	return fmt.Sprintf("Histogram(%s, buckets=%d)", n.Field, n.Buckets)
}

// Planner creates execution plans from AST
type Planner struct {
	optimizations []Optimization
}

// Optimization is a plan optimization pass
type Optimization interface {
	Name() string
	Optimize(plan *Plan) *Plan
}

// NewPlanner creates a new query planner
func NewPlanner() *Planner {
	return &Planner{
		optimizations: []Optimization{
			&PredicatePushdown{},
			&LimitPushdown{},
		},
	}
}

// Plan creates an execution plan from a query
func (p *Planner) Plan(query *Query) (*Plan, error) {
	// Build initial plan
	root, err := p.buildPlan(query)
	if err != nil {
		return nil, err
	}

	// Determine time range
	timeRange := p.resolveTimeRange(query.TimeRange)

	plan := &Plan{
		Root:      root,
		TimeRange: timeRange,
		Query:     query,
	}

	// Apply optimizations
	for _, opt := range p.optimizations {
		plan = opt.Optimize(plan)
	}

	return plan, nil
}

func (p *Planner) buildPlan(query *Query) (PlanNode, error) {
	// Start with scan
	scan := &PlanScanNode{
		Source:     query.Source.SourceType,
		MetricName: query.Source.MetricName,
	}

	var node PlanNode = scan

	// Build pipeline from pipes
	for _, pipe := range query.Pipes {
		var err error
		node, err = p.buildPipeNode(node, pipe)
		if err != nil {
			return nil, err
		}
	}

	return node, nil
}

func (p *Planner) buildPipeNode(input PlanNode, pipe PipeNode) (PlanNode, error) {
	switch n := pipe.(type) {
	case *WhereNode:
		return &PlanFilterNode{
			Input:     input,
			Condition: n.Condition,
		}, nil

	case *SelectNode:
		fields := make([]ProjectField, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = ProjectField{Expr: f.Expr, Alias: f.Alias}
		}
		return &PlanProjectNode{
			Input:  input,
			Fields: fields,
		}, nil

	case *GroupByNode:
		specs := make([]AggSpec, len(n.Aggregations))
		for i, agg := range n.Aggregations {
			specs[i] = AggSpec{
				Function: agg.Function,
				Field:    agg.Field,
				Alias:    agg.Alias,
			}
		}
		return &PlanAggregateNode{
			Input:        input,
			Aggregations: specs,
			GroupBy:      n.GroupFields,
		}, nil

	case *OrderByNode:
		fields := make([]SortField, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = SortField{Field: f.Field, Desc: f.Desc}
		}
		return &PlanSortNode{
			Input:  input,
			Fields: fields,
		}, nil

	case *LimitNode:
		return &PlanLimitNode{
			Input:  input,
			Count:  n.Count,
			Offset: n.Offset,
		}, nil

	case *TopNode:
		// Top is sort desc + limit
		sorted := &PlanSortNode{
			Input:  input,
			Fields: []SortField{{Field: n.Field, Desc: true}},
		}
		return &PlanLimitNode{
			Input: sorted,
			Count: n.Count,
		}, nil

	case *WindowNode:
		return &PlanWindowNode{
			Input:      input,
			WindowType: n.WindowType,
			Duration:   n.Duration,
			Slide:      n.Slide,
		}, nil

	case *CorrelateNode:
		// Create a join with a new scan for the target
		rightScan := &PlanScanNode{Source: n.Target}
		return &PlanJoinNode{
			Left:          input,
			Right:         rightScan,
			JoinFields:    n.JoinFields,
			TimeTolerance: n.TimeTolerance,
		}, nil

	case *AnomaliesNode:
		return &PlanAnomalyNode{
			Input:       input,
			Algorithm:   n.Algorithm,
			Sensitivity: n.Sensitivity,
		}, nil

	case *ExtractNode:
		return &PlanExtractNode{
			Input:   input,
			Pattern: n.Pattern,
			Auto:    n.Auto,
		}, nil

	case *HistogramNode:
		return &PlanHistogramNode{
			Input:   input,
			Field:   n.Field,
			Buckets: n.Buckets,
			GroupBy: n.GroupBy,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported pipe type: %T", pipe)
	}
}

func (p *Planner) resolveTimeRange(tr *TimeRangeNode) TimeRangeSpec {
	spec := TimeRangeSpec{
		End: time.Now(),
	}

	if tr == nil {
		// Default to last 1 hour
		spec.Start = spec.End.Add(-time.Hour)
		return spec
	}

	if tr.Relative > 0 {
		spec.Start = spec.End.Add(-tr.Relative)
	} else {
		spec.Start = tr.Start
		spec.End = tr.End
	}

	spec.Shift = tr.Shift
	return spec
}

// PredicatePushdown pushes filters closer to the scan
type PredicatePushdown struct{}

func (o *PredicatePushdown) Name() string { return "predicate_pushdown" }

func (o *PredicatePushdown) Optimize(plan *Plan) *Plan {
	plan.Root = o.pushdown(plan.Root)
	return plan
}

func (o *PredicatePushdown) pushdown(node PlanNode) PlanNode {
	switch n := node.(type) {
	case *PlanFilterNode:
		// Try to push predicate into scan
		n.Input = o.pushdown(n.Input)
		if scan, ok := n.Input.(*PlanScanNode); ok {
			if o.canPush(n.Condition) {
				scan.Predicates = append(scan.Predicates, n.Condition)
				return scan
			}
		}
		return n

	case *PlanProjectNode:
		n.Input = o.pushdown(n.Input)
		return n

	case *PlanAggregateNode:
		n.Input = o.pushdown(n.Input)
		return n

	case *PlanSortNode:
		n.Input = o.pushdown(n.Input)
		return n

	case *PlanLimitNode:
		n.Input = o.pushdown(n.Input)
		return n

	case *PlanJoinNode:
		n.Left = o.pushdown(n.Left)
		n.Right = o.pushdown(n.Right)
		return n

	default:
		return node
	}
}

func (o *PredicatePushdown) canPush(expr Expr) bool {
	// Simple heuristic: push equality and comparison predicates
	switch e := expr.(type) {
	case *BinaryExpr:
		switch e.Operator {
		case "=", "!=", "<", "<=", ">", ">=", "like":
			return true
		case "and":
			return o.canPush(e.Left) && o.canPush(e.Right)
		}
	}
	return false
}

// LimitPushdown pushes limits closer to the source
type LimitPushdown struct{}

func (o *LimitPushdown) Name() string { return "limit_pushdown" }

func (o *LimitPushdown) Optimize(plan *Plan) *Plan {
	// This is a simplified version - real implementation would be more sophisticated
	return plan
}

// ExplainPlan returns a string representation of the plan
func ExplainPlan(plan *Plan) string {
	var sb strings.Builder
	sb.WriteString("Query Plan:\n")
	sb.WriteString(fmt.Sprintf("  Time Range: %s to %s\n",
		plan.TimeRange.Start.Format(time.RFC3339),
		plan.TimeRange.End.Format(time.RFC3339)))
	if plan.TimeRange.Shift != 0 {
		sb.WriteString(fmt.Sprintf("  Shift: %s\n", plan.TimeRange.Shift))
	}
	sb.WriteString("\n")
	explainNode(&sb, plan.Root, 0)
	sb.WriteString(fmt.Sprintf("\nEstimated Cost: %.2f\n", plan.Root.EstimatedCost()))
	return sb.String()
}

func explainNode(sb *strings.Builder, node PlanNode, indent int) {
	prefix := strings.Repeat("  ", indent)
	sb.WriteString(prefix)
	sb.WriteString("→ ")
	sb.WriteString(node.String())
	sb.WriteString("\n")

	for _, child := range node.Children() {
		explainNode(sb, child, indent+1)
	}
}
