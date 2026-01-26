package otlp

import (
	"encoding/hex"
	"fmt"
	"time"

	"dogwatch/internal/trace"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// processTraces converts OTLP resource spans and stores them
func processTraces(traceStore *trace.Store, resourceSpans []*tracepb.ResourceSpans) error {
	if traceStore == nil {
		return nil
	}

	for _, rs := range resourceSpans {
		serviceName := extractServiceName(rs.Resource)

		for _, scopeSpans := range rs.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				internalSpan := convertProtoSpan(span, serviceName)
				if err := traceStore.RecordSpan(internalSpan); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// convertProtoSpan converts an OTLP Span to internal trace.Span
func convertProtoSpan(span *tracepb.Span, serviceName string) trace.Span {
	startTime := time.Unix(0, int64(span.StartTimeUnixNano))
	endTime := time.Unix(0, int64(span.EndTimeUnixNano))
	durationMs := float64(span.EndTimeUnixNano-span.StartTimeUnixNano) / 1e6

	return trace.Span{
		TraceID:      traceIDToHex(span.TraceId),
		SpanID:       spanIDToHex(span.SpanId),
		ParentSpanID: spanIDToHex(span.ParentSpanId),
		Name:         span.Name,
		ServiceName:  serviceName,
		Kind:         convertSpanKind(span.Kind),
		StartTime:    startTime,
		EndTime:      endTime,
		DurationMs:   durationMs,
		Status:       convertSpanStatus(span.Status),
		StatusMsg:    getStatusMessage(span.Status),
		Attributes:   convertAttributes(span.Attributes),
	}
}

// extractServiceName gets service.name from resource attributes
func extractServiceName(resource *resourcepb.Resource) string {
	if resource == nil {
		return "unknown"
	}
	for _, attr := range resource.Attributes {
		if attr.Key == "service.name" {
			if sv := attr.Value.GetStringValue(); sv != "" {
				return sv
			}
		}
	}
	return "unknown"
}

// convertSpanKind converts OTLP SpanKind to string
func convertSpanKind(kind tracepb.Span_SpanKind) string {
	switch kind {
	case tracepb.Span_SPAN_KIND_CLIENT:
		return "CLIENT"
	case tracepb.Span_SPAN_KIND_SERVER:
		return "SERVER"
	case tracepb.Span_SPAN_KIND_PRODUCER:
		return "PRODUCER"
	case tracepb.Span_SPAN_KIND_CONSUMER:
		return "CONSUMER"
	case tracepb.Span_SPAN_KIND_INTERNAL:
		return "INTERNAL"
	default:
		return "UNSPECIFIED"
	}
}

// convertSpanStatus converts OTLP Status to string
func convertSpanStatus(status *tracepb.Status) string {
	if status == nil {
		return "UNSET"
	}
	switch status.Code {
	case tracepb.Status_STATUS_CODE_OK:
		return "OK"
	case tracepb.Status_STATUS_CODE_ERROR:
		return "ERROR"
	default:
		return "UNSET"
	}
}

// getStatusMessage extracts status message
func getStatusMessage(status *tracepb.Status) string {
	if status == nil {
		return ""
	}
	return status.Message
}

// convertAttributes converts OTLP KeyValue slice to map
func convertAttributes(attrs []*commonpb.KeyValue) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	result := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		result[kv.Key] = anyValueToString(kv.Value)
	}
	return result
}

// anyValueToString converts OTLP AnyValue to string
func anyValueToString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch val := v.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return formatInt(val.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return formatFloat(val.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		if val.BoolValue {
			return "true"
		}
		return "false"
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(val.BytesValue)
	default:
		return ""
	}
}

// traceIDToHex converts 16-byte trace ID to hex string
func traceIDToHex(id []byte) string {
	if len(id) == 0 {
		return ""
	}
	return hex.EncodeToString(id)
}

// spanIDToHex converts 8-byte span ID to hex string
func spanIDToHex(id []byte) string {
	if len(id) == 0 {
		return ""
	}
	return hex.EncodeToString(id)
}

// formatInt formats int64 to string
func formatInt(i int64) string {
	return fmt.Sprintf("%d", i)
}

// formatFloat formats float64 to string
func formatFloat(f float64) string {
	return fmt.Sprintf("%g", f)
}
