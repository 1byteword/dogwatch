package otlp

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/logs"
	"dogwatch/internal/trace"

	"google.golang.org/grpc"
)

// Config holds OTLP receiver configuration
type Config struct {
	GRPCPort int
	HTTPPort int
}

// DefaultConfig returns default OTLP configuration
func DefaultConfig() Config {
	return Config{
		GRPCPort: 4317,
		HTTPPort: 4318,
	}
}

// SpanCallback is called for each span processed
type SpanCallback func(span trace.Span)

// SpanSampler decides whether to keep or drop a span
type SpanSampler interface {
	// ShouldSample returns true if the span should be kept
	ShouldSample(span *trace.Span) bool
}

// Server is the unified OTLP receiver server
type Server struct {
	config       Config
	traceStore   *trace.Store
	metricsStore *custommetrics.Store
	logStore     *logs.Store

	spanCallback SpanCallback
	spanSampler  SpanSampler

	grpcServer *grpc.Server
	httpServer *http.Server

	mu      sync.Mutex
	running bool
}

// NewServer creates a new OTLP receiver server
func NewServer(cfg Config, traceStore *trace.Store, metricsStore *custommetrics.Store, logStore *logs.Store) *Server {
	return &Server{
		config:       cfg,
		traceStore:   traceStore,
		metricsStore: metricsStore,
		logStore:     logStore,
	}
}

// SetSpanCallback sets a callback to be invoked for each span received
func (s *Server) SetSpanCallback(cb SpanCallback) {
	s.mu.Lock()
	s.spanCallback = cb
	s.mu.Unlock()
}

// SetSpanSampler sets a sampler to filter spans before storage
func (s *Server) SetSpanSampler(sampler SpanSampler) {
	s.mu.Lock()
	s.spanSampler = sampler
	s.mu.Unlock()
}

// Start starts both gRPC and HTTP OTLP receivers
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("OTLP server already running")
	}

	// Start gRPC server
	if err := s.startGRPC(); err != nil {
		return fmt.Errorf("starting gRPC server: %w", err)
	}

	// Start HTTP server
	if err := s.startHTTP(); err != nil {
		s.grpcServer.GracefulStop()
		return fmt.Errorf("starting HTTP server: %w", err)
	}

	s.running = true
	log.Printf("OTLP gRPC receiver: localhost:%d", s.config.GRPCPort)
	log.Printf("OTLP HTTP receiver: http://localhost:%d/v1/{traces,metrics,logs}", s.config.HTTPPort)
	return nil
}

// Stop gracefully stops both servers
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Stop gRPC server
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Stop HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	s.running = false
	log.Println("OTLP server stopped")
	return nil
}

func (s *Server) startGRPC() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen on port %d: %w", s.config.GRPCPort, err)
	}

	s.grpcServer = grpc.NewServer()

	// Register OTLP services
	registerTraceServiceWithSampler(s.grpcServer, s.traceStore, s.spanCallback, s.spanSampler)
	registerMetricsService(s.grpcServer, s.metricsStore)
	registerLogsService(s.grpcServer, s.logStore)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("OTLP gRPC server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) startHTTP() error {
	mux := http.NewServeMux()

	// Register OTLP HTTP endpoints
	mux.HandleFunc("/v1/traces", s.handleHTTPTraces)
	mux.HandleFunc("/v1/metrics", s.handleHTTPMetrics)
	mux.HandleFunc("/v1/logs", s.handleHTTPLogs)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.HTTPPort),
		Handler: mux,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("OTLP HTTP server error: %v", err)
		}
	}()

	return nil
}
