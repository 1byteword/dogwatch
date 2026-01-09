package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"dogwatch/internal/aggregator"
)

//go:embed static/*
var staticFiles embed.FS

// Server serves the web dashboard
type Server struct {
	agg    *aggregator.Aggregator
	server *http.Server
	mu     sync.RWMutex
}

// New creates a new web server
func New(agg *aggregator.Aggregator, port int) *Server {
	s := &Server{
		agg: agg,
	}

	mux := http.NewServeMux()

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API endpoints
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/connections", s.handleConnections)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

// Start begins serving HTTP
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// StatsResponse is the JSON response for /api/stats
type StatsResponse struct {
	UpdatedAt        string             `json:"updated_at"`
	TotalConnections int64              `json:"total_connections"`
	TotalRequests    int64              `json:"total_requests"`
	TotalErrors      int64              `json:"total_errors"`
	Endpoints        []EndpointResponse `json:"endpoints"`
	Connections      []ConnectionResponse `json:"connections"`
}

// EndpointResponse represents an endpoint in the API
type EndpointResponse struct {
	Method       string  `json:"method"`
	Path         string  `json:"path"`
	RequestCount int64   `json:"request_count"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
	P50Ms        float64 `json:"p50_ms"`
	P99Ms        float64 `json:"p99_ms"`
	AvgMs        float64 `json:"avg_ms"`
}

// ConnectionResponse represents a connection in the API
type ConnectionResponse struct {
	Process string `json:"process"`
	PID     uint32 `json:"pid"`
	Remote  string `json:"remote"`
	Port    uint16 `json:"port"`
	Count   int64  `json:"count"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()

	resp := StatsResponse{
		UpdatedAt:        time.Now().Format(time.RFC3339),
		TotalConnections: stats.TotalConnections,
		TotalRequests:    stats.TotalRequests,
		TotalErrors:      stats.TotalErrors,
		Endpoints:        make([]EndpointResponse, 0, len(stats.Endpoints)),
		Connections:      make([]ConnectionResponse, 0, len(stats.Connections)),
	}

	for _, ep := range stats.Endpoints {
		errRate := float64(0)
		if ep.RequestCount > 0 {
			errRate = float64(ep.ErrorCount) / float64(ep.RequestCount) * 100
		}
		resp.Endpoints = append(resp.Endpoints, EndpointResponse{
			Method:       ep.Method,
			Path:         ep.Path,
			RequestCount: ep.RequestCount,
			ErrorCount:   ep.ErrorCount,
			ErrorRate:    errRate,
			P50Ms:        float64(ep.P50.Microseconds()) / 1000,
			P99Ms:        float64(ep.P99.Microseconds()) / 1000,
			AvgMs:        float64(ep.Avg.Microseconds()) / 1000,
		})
	}

	for _, conn := range stats.Connections {
		resp.Connections = append(resp.Connections, ConnectionResponse{
			Process: conn.Comm,
			PID:     conn.PID,
			Remote:  conn.Remote,
			Port:    conn.Port,
			Count:   conn.Count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()
	endpoints := make([]EndpointResponse, 0, len(stats.Endpoints))

	for _, ep := range stats.Endpoints {
		errRate := float64(0)
		if ep.RequestCount > 0 {
			errRate = float64(ep.ErrorCount) / float64(ep.RequestCount) * 100
		}
		endpoints = append(endpoints, EndpointResponse{
			Method:       ep.Method,
			Path:         ep.Path,
			RequestCount: ep.RequestCount,
			ErrorCount:   ep.ErrorCount,
			ErrorRate:    errRate,
			P50Ms:        float64(ep.P50.Microseconds()) / 1000,
			P99Ms:        float64(ep.P99.Microseconds()) / 1000,
			AvgMs:        float64(ep.Avg.Microseconds()) / 1000,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()
	connections := make([]ConnectionResponse, 0, len(stats.Connections))

	for _, conn := range stats.Connections {
		connections = append(connections, ConnectionResponse{
			Process: conn.Comm,
			PID:     conn.PID,
			Remote:  conn.Remote,
			Port:    conn.Port,
			Count:   conn.Count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connections)
}
