package recording

import (
	"time"
)

// RecordingRule defines a pre-computed aggregation rule
type RecordingRule struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`       // e.g., "service:request_rate:5m"
	Expression string            `json:"expression"` // DQL/PromQL expression to evaluate
	Interval   time.Duration     `json:"interval"`   // Evaluation interval (e.g., 1m, 5m)
	Labels     map[string]string `json:"labels"`     // Additional labels to add to result
	Enabled    bool              `json:"enabled"`
	LastEval   time.Time         `json:"last_eval"`
	LastError  string            `json:"last_error,omitempty"`
	LastValue  float64           `json:"last_value,omitempty"`

	// Metadata
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by,omitempty"`
}

// EvaluationHistory tracks the history of rule evaluations
type EvaluationHistory struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Duration  int64     `json:"duration_ms"` // Evaluation duration in milliseconds
	Error     string    `json:"error,omitempty"`
	Success   bool      `json:"success"`
}

// ManagerStats holds metrics about the recording rule manager
type ManagerStats struct {
	TotalRules          int           `json:"total_rules"`
	EnabledRules        int           `json:"enabled_rules"`
	TotalEvaluations    int64         `json:"total_evaluations"`
	SuccessfulEvals     int64         `json:"successful_evaluations"`
	FailedEvals         int64         `json:"failed_evaluations"`
	LastEvalDuration    time.Duration `json:"last_eval_duration"`
	AverageEvalDuration time.Duration `json:"avg_eval_duration"`
}

// DefaultRules returns the built-in recording rules
func DefaultRules() []RecordingRule {
	return []RecordingRule{
		{
			ID:          "builtin:request_rate:1m",
			Name:        "service:request_rate:1m",
			Expression:  "SELECT service, count(*) / 60.0 as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service",
			Interval:    time.Minute,
			Enabled:     true,
			Description: "Requests per second by service (1 minute window)",
			Labels: map[string]string{
				"__name__": "service:request_rate:1m",
				"source":   "recording_rule",
			},
		},
		{
			ID:          "builtin:error_rate:1m",
			Name:        "service:error_rate:1m",
			Expression:  "SELECT service, sum(CASE WHEN status = 'ERROR' THEN 1 ELSE 0 END) * 100.0 / count(*) as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service",
			Interval:    time.Minute,
			Enabled:     true,
			Description: "Error percentage by service (1 minute window)",
			Labels: map[string]string{
				"__name__": "service:error_rate:1m",
				"source":   "recording_rule",
			},
		},
		{
			ID:          "builtin:latency_p99:1m",
			Name:        "service:latency_p99:1m",
			Expression:  "SELECT service, p99(duration_ms) as value FROM traces WHERE timestamp >= now() - 1m GROUP BY service",
			Interval:    time.Minute,
			Enabled:     true,
			Description: "P99 latency in milliseconds by service (1 minute window)",
			Labels: map[string]string{
				"__name__": "service:latency_p99:1m",
				"source":   "recording_rule",
			},
		},
	}
}
