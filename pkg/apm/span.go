package apm

import (
	"context"
	"fmt"
	"time"
)

// SpanType represents the type of span
type SpanType string

const (
	SpanTypeWeb      SpanType = "web"
	SpanTypeHTTP     SpanType = "http"
	SpanTypeSQL      SpanType = "sql"
	SpanTypeCache    SpanType = "cache"
	SpanTypeQueue    SpanType = "queue"
	SpanTypeGRPC     SpanType = "grpc"
	SpanTypeGraphQL  SpanType = "graphql"
	SpanTypeCustom   SpanType = "custom"
	SpanTypeInternal SpanType = "internal"
)

// Span represents a unit of work
type Span struct {
	agent *Agent

	TraceID       string            `json:"trace_id"`
	SpanID        string            `json:"span_id"`
	ParentID      string            `json:"parent_id,omitempty"`
	OperationName string            `json:"operation_name"`
	Service       string            `json:"service"`
	Resource      string            `json:"resource"`
	Type          SpanType          `json:"type,omitempty"`
	StartTime     time.Time         `json:"start_time"`
	Duration      time.Duration     `json:"duration"`
	Error         bool              `json:"error"`
	Tags          map[string]string `json:"tags,omitempty"`
	Metrics       map[string]float64 `json:"metrics,omitempty"`

	// For internal tracking
	finished bool
}

// SpanOption configures a span
type SpanOption func(*Span)

// WithParent sets the parent span
func WithParent(parent *Span) SpanOption {
	return func(s *Span) {
		if parent != nil {
			s.TraceID = parent.TraceID
			s.ParentID = parent.SpanID
		}
	}
}

// WithSpanType sets the span type
func WithSpanType(t SpanType) SpanOption {
	return func(s *Span) {
		s.Type = t
	}
}

// WithResource sets the resource name
func WithResource(resource string) SpanOption {
	return func(s *Span) {
		s.Resource = resource
	}
}

// WithTag adds a tag to the span
func WithTag(key, value string) SpanOption {
	return func(s *Span) {
		s.Tags[key] = value
	}
}

// WithTags adds multiple tags
func WithTags(tags map[string]string) SpanOption {
	return func(s *Span) {
		for k, v := range tags {
			s.Tags[k] = v
		}
	}
}

// Finish completes the span and records it
func (s *Span) Finish() {
	if s == nil || s.finished {
		return
	}
	s.finished = true
	s.Duration = time.Since(s.StartTime)

	if s.agent != nil {
		s.agent.recordSpan(s)
	}
}

// FinishWithError completes the span with an error
func (s *Span) FinishWithError(err error) {
	if s == nil {
		return
	}
	if err != nil {
		s.SetError(err)
	}
	s.Finish()
}

// SetTag sets a tag on the span
func (s *Span) SetTag(key, value string) *Span {
	if s == nil || s.Tags == nil {
		return s
	}
	s.Tags[key] = value
	return s
}

// SetMetric sets a metric on the span
func (s *Span) SetMetric(key string, value float64) *Span {
	if s == nil || s.Metrics == nil {
		return s
	}
	s.Metrics[key] = value
	return s
}

// SetError marks the span as errored
func (s *Span) SetError(err error) *Span {
	if s == nil || err == nil {
		return s
	}
	s.Error = true
	s.Tags["error.message"] = err.Error()
	s.Tags["error.type"] = fmt.Sprintf("%T", err)
	return s
}

// SetResource sets the resource name
func (s *Span) SetResource(resource string) *Span {
	if s == nil {
		return s
	}
	s.Resource = resource
	return s
}

// SetOperationName sets the operation name
func (s *Span) SetOperationName(name string) *Span {
	if s == nil {
		return s
	}
	s.OperationName = name
	return s
}

// Context key for spans
type spanContextKey struct{}

// ContextWithSpan returns a new context with the span
func ContextWithSpan(ctx context.Context, span *Span) context.Context {
	return context.WithValue(ctx, spanContextKey{}, span)
}

// SpanFromContext retrieves a span from context
func SpanFromContext(ctx context.Context) *Span {
	if ctx == nil {
		return nil
	}
	if span, ok := ctx.Value(spanContextKey{}).(*Span); ok {
		return span
	}
	return nil
}

// Common tag keys
const (
	TagHTTPMethod     = "http.method"
	TagHTTPURL        = "http.url"
	TagHTTPStatusCode = "http.status_code"
	TagHTTPHost       = "http.host"
	TagHTTPPath       = "http.path"
	TagHTTPUserAgent  = "http.user_agent"
	TagHTTPRequestID  = "http.request_id"

	TagDBType      = "db.type"
	TagDBInstance  = "db.instance"
	TagDBStatement = "db.statement"
	TagDBUser      = "db.user"
	TagDBRowCount  = "db.row_count"

	TagGRPCService   = "grpc.service"
	TagGRPCMethod    = "grpc.method"
	TagGRPCCode      = "grpc.code"

	TagCacheType = "cache.type"
	TagCacheHit  = "cache.hit"
	TagCacheKey  = "cache.key"

	TagQueueType    = "queue.type"
	TagQueueName    = "queue.name"
	TagMessageCount = "queue.message_count"

	TagPeerService  = "peer.service"
	TagPeerHostname = "peer.hostname"
	TagPeerPort     = "peer.port"

	TagComponent = "component"
	TagSpanKind  = "span.kind"
)

// Span kinds
const (
	SpanKindServer   = "server"
	SpanKindClient   = "client"
	SpanKindProducer = "producer"
	SpanKindConsumer = "consumer"
	SpanKindInternal = "internal"
)
