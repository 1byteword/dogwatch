package instrumentation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dogwatch/pkg/apm"
)

// GRPCServerOptions configures gRPC server interceptor
type GRPCServerOptions struct {
	// ServiceName overrides the default service name
	ServiceName string

	// IgnoreMethods are methods to skip tracing
	IgnoreMethods []string

	// Tags to add to all spans
	Tags map[string]string
}

// GRPCClientOptions configures gRPC client interceptor
type GRPCClientOptions struct {
	// ServiceName for client-side tracing
	ServiceName string

	// Tags to add to all spans
	Tags map[string]string
}

// UnaryServerInterceptor returns a gRPC unary server interceptor for tracing
// This returns a function compatible with grpc.UnaryServerInterceptor
func UnaryServerInterceptor(opts ...GRPCServerOptions) interface{} {
	opt := GRPCServerOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Return a function that matches grpc.UnaryServerInterceptor signature
	// func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error)
	return func(ctx context.Context, req interface{}, info interface{}, handler interface{}) (interface{}, error) {
		// In a real implementation, info would be *grpc.UnaryServerInfo
		// and handler would be grpc.UnaryHandler

		// Extract method name from info
		fullMethod := extractMethodFromInfo(info)

		// Check if method should be ignored
		for _, m := range opt.IgnoreMethods {
			if m == fullMethod || strings.HasSuffix(fullMethod, m) {
				// Call handler directly
				if h, ok := handler.(func(context.Context, interface{}) (interface{}, error)); ok {
					return h(ctx, req)
				}
				return nil, fmt.Errorf("invalid handler type")
			}
		}

		// Parse service and method
		service, method := parseMethod(fullMethod)

		span, ctx := apm.StartSpanFromContext(ctx, "grpc.server",
			apm.WithSpanType(apm.SpanTypeGRPC),
			apm.WithResource(fullMethod),
			apm.WithTag(apm.TagSpanKind, apm.SpanKindServer),
		)

		span.SetTag(apm.TagGRPCService, service)
		span.SetTag(apm.TagGRPCMethod, method)
		span.SetTag(apm.TagComponent, "grpc")

		for k, v := range opt.Tags {
			span.SetTag(k, v)
		}

		start := time.Now()

		// Call the handler
		var resp interface{}
		var err error
		if h, ok := handler.(func(context.Context, interface{}) (interface{}, error)); ok {
			resp, err = h(ctx, req)
		}

		duration := time.Since(start)
		span.SetMetric("grpc.duration_ms", float64(duration.Milliseconds()))

		// Set status code
		code := "OK"
		if err != nil {
			span.SetError(err)
			code = extractGRPCCode(err)
		}
		span.SetTag(apm.TagGRPCCode, code)

		span.Finish()

		// Record metrics
		tags := map[string]string{
			"service": service,
			"method":  method,
			"code":    code,
		}
		apm.RecordHistogram("grpc.server.duration_ms", float64(duration.Milliseconds()), tags)
		apm.RecordCounter("grpc.server.request.count", 1, tags)
		if err != nil {
			apm.RecordCounter("grpc.server.errors", 1, tags)
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a gRPC stream server interceptor for tracing
func StreamServerInterceptor(opts ...GRPCServerOptions) interface{} {
	opt := GRPCServerOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(srv interface{}, ss interface{}, info interface{}, handler interface{}) error {
		fullMethod := extractMethodFromInfo(info)
		service, method := parseMethod(fullMethod)

		span := apm.StartSpan("grpc.server.stream",
			apm.WithSpanType(apm.SpanTypeGRPC),
			apm.WithResource(fullMethod),
			apm.WithTag(apm.TagSpanKind, apm.SpanKindServer),
		)

		span.SetTag(apm.TagGRPCService, service)
		span.SetTag(apm.TagGRPCMethod, method)
		span.SetTag("grpc.stream", "true")

		for k, v := range opt.Tags {
			span.SetTag(k, v)
		}

		start := time.Now()

		var err error
		if h, ok := handler.(func(interface{}, interface{}) error); ok {
			err = h(srv, ss)
		}

		duration := time.Since(start)
		span.SetMetric("grpc.duration_ms", float64(duration.Milliseconds()))

		code := "OK"
		if err != nil {
			span.SetError(err)
			code = extractGRPCCode(err)
		}
		span.SetTag(apm.TagGRPCCode, code)

		span.Finish()

		return err
	}
}

// UnaryClientInterceptor returns a gRPC unary client interceptor for tracing
func UnaryClientInterceptor(opts ...GRPCClientOptions) interface{} {
	opt := GRPCClientOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(ctx context.Context, method string, req, reply interface{}, cc interface{}, invoker interface{}, callOpts ...interface{}) error {
		service, methodName := parseMethod(method)

		span, ctx := apm.StartSpanFromContext(ctx, "grpc.client",
			apm.WithSpanType(apm.SpanTypeGRPC),
			apm.WithResource(method),
			apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
		)

		span.SetTag(apm.TagGRPCService, service)
		span.SetTag(apm.TagGRPCMethod, methodName)
		span.SetTag(apm.TagPeerService, service)
		span.SetTag(apm.TagComponent, "grpc")

		for k, v := range opt.Tags {
			span.SetTag(k, v)
		}

		start := time.Now()

		var err error
		if inv, ok := invoker.(func(context.Context, string, interface{}, interface{}, interface{}, ...interface{}) error); ok {
			err = inv(ctx, method, req, reply, cc, callOpts...)
		}

		duration := time.Since(start)
		span.SetMetric("grpc.duration_ms", float64(duration.Milliseconds()))

		code := "OK"
		if err != nil {
			span.SetError(err)
			code = extractGRPCCode(err)
		}
		span.SetTag(apm.TagGRPCCode, code)

		span.Finish()

		// Record metrics
		tags := map[string]string{
			"service": service,
			"method":  methodName,
			"code":    code,
		}
		apm.RecordHistogram("grpc.client.duration_ms", float64(duration.Milliseconds()), tags)
		apm.RecordCounter("grpc.client.request.count", 1, tags)
		if err != nil {
			apm.RecordCounter("grpc.client.errors", 1, tags)
		}

		return err
	}
}

// StreamClientInterceptor returns a gRPC stream client interceptor for tracing
func StreamClientInterceptor(opts ...GRPCClientOptions) interface{} {
	opt := GRPCClientOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(ctx context.Context, desc interface{}, cc interface{}, method string, streamer interface{}, callOpts ...interface{}) (interface{}, error) {
		service, methodName := parseMethod(method)

		span, ctx := apm.StartSpanFromContext(ctx, "grpc.client.stream",
			apm.WithSpanType(apm.SpanTypeGRPC),
			apm.WithResource(method),
			apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
		)

		span.SetTag(apm.TagGRPCService, service)
		span.SetTag(apm.TagGRPCMethod, methodName)
		span.SetTag("grpc.stream", "true")

		for k, v := range opt.Tags {
			span.SetTag(k, v)
		}

		var stream interface{}
		var err error
		if s, ok := streamer.(func(context.Context, interface{}, interface{}, string, ...interface{}) (interface{}, error)); ok {
			stream, err = s(ctx, desc, cc, method, callOpts...)
		}

		if err != nil {
			span.SetError(err)
			span.SetTag(apm.TagGRPCCode, extractGRPCCode(err))
			span.Finish()
			return nil, err
		}

		// For streams, we finish the span when the stream is created
		// Individual message tracing would require wrapping the stream
		span.SetTag(apm.TagGRPCCode, "OK")
		span.Finish()

		return stream, nil
	}
}

// Helper functions

// parseMethod extracts service and method from full gRPC method name
func parseMethod(fullMethod string) (service, method string) {
	// Format: /package.Service/Method
	fullMethod = strings.TrimPrefix(fullMethod, "/")
	if idx := strings.LastIndex(fullMethod, "/"); idx >= 0 {
		return fullMethod[:idx], fullMethod[idx+1:]
	}
	return fullMethod, ""
}

// extractMethodFromInfo extracts the method from server info (placeholder)
func extractMethodFromInfo(info interface{}) string {
	// In real implementation, cast to *grpc.UnaryServerInfo and return FullMethod
	return "/unknown.Service/UnknownMethod"
}

// extractGRPCCode extracts the gRPC status code from an error (placeholder)
func extractGRPCCode(err error) string {
	// In real implementation, use status.FromError(err)
	if err == nil {
		return "OK"
	}
	return "UNKNOWN"
}

// TraceGRPCCall is a helper for manual gRPC tracing
func TraceGRPCCall(ctx context.Context, service, method string) (*apm.Span, context.Context) {
	fullMethod := "/" + service + "/" + method

	span, ctx := apm.StartSpanFromContext(ctx, "grpc.call",
		apm.WithSpanType(apm.SpanTypeGRPC),
		apm.WithResource(fullMethod),
		apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
	)

	span.SetTag(apm.TagGRPCService, service)
	span.SetTag(apm.TagGRPCMethod, method)

	return span, ctx
}
