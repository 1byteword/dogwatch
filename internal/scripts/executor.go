package scripts

import (
	"context"
	"fmt"
	"time"

	"dogwatch/internal/query"
)

// Runner executes scripts using the query executor
type Runner struct {
	executor *query.Executor
	registry *Registry
}

// NewRunner creates a new script runner
func NewRunner(executor *query.Executor, registry *Registry) *Runner {
	if registry == nil {
		registry = DefaultRegistry
	}
	return &Runner{
		executor: executor,
		registry: registry,
	}
}

// Result represents the output of a script execution
type Result struct {
	Script     *Script                  `json:"script"`
	Columns    []string                 `json:"columns"`
	Rows       []map[string]interface{} `json:"rows"`
	RowCount   int                      `json:"row_count"`
	ExecutedAt time.Time                `json:"executed_at"`
	Duration   time.Duration            `json:"duration"`
	Query      string                   `json:"query,omitempty"` // Expanded query (for debugging)
}

// RunOptions configures script execution
type RunOptions struct {
	Parameters map[string]string // User-provided parameters
	Timeout    time.Duration     // Execution timeout
	DebugQuery bool              // Include expanded query in result
}

// Run executes a script by name with the given options
func (r *Runner) Run(ctx context.Context, name string, opts RunOptions) (*Result, error) {
	startTime := time.Now()

	// Look up script
	script := r.registry.Get(name)
	if script == nil {
		return nil, fmt.Errorf("script not found: %s", name)
	}

	// Parse and validate parameters
	params, err := script.ParseParameters(opts.Parameters)
	if err != nil {
		return nil, fmt.Errorf("parameter error: %w", err)
	}

	// Expand query template
	expandedQuery := script.ExpandQuery(params)

	// Apply timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Execute query
	if r.executor == nil {
		return nil, fmt.Errorf("query executor not configured")
	}

	queryResult, err := r.executor.Execute(ctx, expandedQuery)
	if err != nil {
		return nil, fmt.Errorf("query execution error: %w", err)
	}

	// Convert query.Row to map[string]interface{}
	rows := make([]map[string]interface{}, len(queryResult.Rows))
	for i, row := range queryResult.Rows {
		rows[i] = map[string]interface{}(row)
	}

	// Build result
	result := &Result{
		Script:     script,
		Columns:    queryResult.Columns,
		Rows:       rows,
		RowCount:   len(queryResult.Rows),
		ExecutedAt: startTime,
		Duration:   time.Since(startTime),
	}

	if opts.DebugQuery {
		result.Query = expandedQuery
	}

	return result, nil
}

// RunScript executes a script with a simpler interface for CLI usage
func (r *Runner) RunScript(ctx context.Context, name string, params map[string]string) (*Result, error) {
	return r.Run(ctx, name, RunOptions{
		Parameters: params,
		Timeout:    30 * time.Second,
	})
}

// ListScripts returns all available scripts, optionally filtered by category
func (r *Runner) ListScripts(category string) []*Script {
	return r.registry.List(category)
}

// ListCategories returns all script categories
func (r *Runner) ListCategories() []CategoryInfo {
	return r.registry.Categories()
}

// GetScript returns a script by name
func (r *Runner) GetScript(name string) *Script {
	return r.registry.Get(name)
}
