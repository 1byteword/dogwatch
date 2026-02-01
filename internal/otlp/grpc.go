package otlp

import (
	"context"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

// TraceService implements the OTLP trace collector service
type TraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	store        *trace.Store
	spanCallback SpanCallback
	spanSampler  SpanSampler
}

// Export receives trace data from OTLP clients
func (s *TraceService) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if err := processTracesWithSampler(s.store, req.ResourceSpans, s.spanCallback, s.spanSampler); err != nil {
		return &coltracepb.ExportTraceServiceResponse{}, err
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// MetricsService implements the OTLP metrics collector service
type MetricsService struct {
	colmetricspb.UnimplementedMetricsServiceServer
	store *custommetrics.Store
}

// Export receives metrics data from OTLP clients
func (s *MetricsService) Export(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) (*colmetricspb.ExportMetricsServiceResponse, error) {
	if err := processMetrics(s.store, req.ResourceMetrics); err != nil {
		return &colmetricspb.ExportMetricsServiceResponse{}, err
	}
	return &colmetricspb.ExportMetricsServiceResponse{}, nil
}

// LogsService implements the OTLP logs collector service
type LogsService struct {
	collogspb.UnimplementedLogsServiceServer
	store *logs.Store
}

// Export receives logs data from OTLP clients
func (s *LogsService) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	if err := processLogs(s.store, req.ResourceLogs); err != nil {
		return &collogspb.ExportLogsServiceResponse{}, err
	}
	return &collogspb.ExportLogsServiceResponse{}, nil
}

// registerTraceService registers the trace service with gRPC server
func registerTraceService(server *grpc.Server, store *trace.Store, spanCallback SpanCallback) {
	registerTraceServiceWithSampler(server, store, spanCallback, nil)
}

// registerTraceServiceWithSampler registers the trace service with gRPC server and sampler
func registerTraceServiceWithSampler(server *grpc.Server, store *trace.Store, spanCallback SpanCallback, sampler SpanSampler) {
	coltracepb.RegisterTraceServiceServer(server, &TraceService{store: store, spanCallback: spanCallback, spanSampler: sampler})
}

// registerMetricsService registers the metrics service with gRPC server
func registerMetricsService(server *grpc.Server, store *custommetrics.Store) {
	colmetricspb.RegisterMetricsServiceServer(server, &MetricsService{store: store})
}

// registerLogsService registers the logs service with gRPC server
func registerLogsService(server *grpc.Server, store *logs.Store) {
	collogspb.RegisterLogsServiceServer(server, &LogsService{store: store})
}
