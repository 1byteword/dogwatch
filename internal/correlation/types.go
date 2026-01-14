package correlation

import (
	"time"

	"dogwatch/internal/deploys"
	"dogwatch/internal/incidents"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"
)

// CorrelationType identifies the type of correlation
type CorrelationType string

const (
	CorrelationTraceLog      CorrelationType = "trace_log"       // Trace and its logs
	CorrelationDeployIncident CorrelationType = "deploy_incident" // Deploy followed by incident
	CorrelationAlertTrace    CorrelationType = "alert_trace"     // Alert with triggering traces
	CorrelationMetricTrace   CorrelationType = "metric_trace"    // Metric anomaly with traces
	CorrelationServiceEvent  CorrelationType = "service_event"   // All events for a service
)

// CorrelatedEvent represents any event that can be correlated
type CorrelatedEvent struct {
	Type      string      `json:"type"`      // trace, log, incident, deploy, alert, metric
	ID        string      `json:"id"`
	Timestamp time.Time   `json:"timestamp"`
	Service   string      `json:"service,omitempty"`
	Summary   string      `json:"summary"`
	Severity  string      `json:"severity,omitempty"` // For incidents/alerts
	Data      interface{} `json:"data,omitempty"`     // Full object if needed
}

// TraceContext contains all data correlated to a trace
type TraceContext struct {
	TraceID    string            `json:"trace_id"`
	Trace      *trace.Trace      `json:"trace,omitempty"`
	Spans      []trace.Span      `json:"spans,omitempty"`
	Logs       []logs.LogEntry   `json:"logs,omitempty"`
	Service    string            `json:"service,omitempty"`
	Duration   float64           `json:"duration_ms,omitempty"`
	Status     string            `json:"status,omitempty"`
	ErrorCount int               `json:"error_count"`
}

// IncidentContext contains all data correlated to an incident
type IncidentContext struct {
	Incident         *incidents.Incident   `json:"incident"`
	RelatedTraces    []trace.Trace         `json:"related_traces,omitempty"`
	RelatedLogs      []logs.LogEntry       `json:"related_logs,omitempty"`
	PrecedingDeploys []deploys.Deployment  `json:"preceding_deploys,omitempty"`
	RelatedIncidents []incidents.Incident  `json:"related_incidents,omitempty"` // Same service, recent
	ProbableCause    *DeployCorrelation    `json:"probable_cause,omitempty"`
	Timeline         []CorrelatedEvent     `json:"timeline,omitempty"`
}

// DeployContext contains all data correlated to a deployment
type DeployContext struct {
	Deployment        *deploys.Deployment   `json:"deployment"`
	FollowingIncidents []incidents.Incident `json:"following_incidents,omitempty"`
	ErrorsBeforeAfter  *ErrorComparison     `json:"errors_comparison,omitempty"`
	LatencyBeforeAfter *LatencyComparison   `json:"latency_comparison,omitempty"`
	Impact             string               `json:"impact,omitempty"` // positive, negative, neutral
}

// DeployCorrelation links a deploy to an incident
type DeployCorrelation struct {
	Deployment   *deploys.Deployment `json:"deployment"`
	TimeDelta    time.Duration       `json:"time_delta"`    // Time between deploy and incident
	Confidence   float64             `json:"confidence"`    // 0-1 confidence score
	Reason       string              `json:"reason"`        // Why we think this is related
	ServiceMatch bool                `json:"service_match"` // Same service as incident
}

// ErrorComparison compares error rates before/after an event
type ErrorComparison struct {
	Before      int     `json:"errors_before"`
	After       int     `json:"errors_after"`
	BeforeRate  float64 `json:"rate_before"`  // errors per minute
	AfterRate   float64 `json:"rate_after"`
	ChangePercent float64 `json:"change_percent"`
}

// LatencyComparison compares latency before/after an event
type LatencyComparison struct {
	P50Before     float64 `json:"p50_before_ms"`
	P50After      float64 `json:"p50_after_ms"`
	P99Before     float64 `json:"p99_before_ms"`
	P99After      float64 `json:"p99_after_ms"`
	ChangePercent float64 `json:"change_percent"`
}

// ServiceTimeline contains all events for a service in time order
type ServiceTimeline struct {
	Service    string            `json:"service"`
	ServiceID  string            `json:"service_id,omitempty"`
	StartTime  time.Time         `json:"start_time"`
	EndTime    time.Time         `json:"end_time"`
	Events     []CorrelatedEvent `json:"events"`
	Summary    *TimelineSummary  `json:"summary"`
}

// TimelineSummary provides aggregate stats for a timeline
type TimelineSummary struct {
	TotalEvents   int `json:"total_events"`
	TraceCount    int `json:"trace_count"`
	LogCount      int `json:"log_count"`
	ErrorLogCount int `json:"error_log_count"`
	IncidentCount int `json:"incident_count"`
	DeployCount   int `json:"deploy_count"`
	AlertCount    int `json:"alert_count"`
}

// AlertContext contains all data correlated to an alert
type AlertContext struct {
	AlertID       string          `json:"alert_id"`
	AlertName     string          `json:"alert_name"`
	Service       string          `json:"service"`
	TriggeredAt   time.Time       `json:"triggered_at"`
	TriggerTraces []trace.Trace   `json:"trigger_traces,omitempty"` // Traces around trigger time
	TriggerLogs   []logs.LogEntry `json:"trigger_logs,omitempty"`   // Logs around trigger time
	RecentDeploys []deploys.Deployment `json:"recent_deploys,omitempty"`
}

// CorrelationQuery specifies what to correlate
type CorrelationQuery struct {
	// Time range for correlation
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// Filters
	Service   string `json:"service,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`

	// What to include
	IncludeTraces    bool `json:"include_traces"`
	IncludeLogs      bool `json:"include_logs"`
	IncludeIncidents bool `json:"include_incidents"`
	IncludeDeploys   bool `json:"include_deploys"`

	// Limits
	MaxTraces    int `json:"max_traces"`
	MaxLogs      int `json:"max_logs"`
	MaxIncidents int `json:"max_incidents"`
}
