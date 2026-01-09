package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dogwatch/internal/aggregator"
	"dogwatch/internal/metrics"
)

//go:embed static/*
var staticFiles embed.FS

// Server serves the web dashboard
type Server struct {
	agg     *aggregator.Aggregator
	metrics *metrics.Collector
	server  *http.Server
	mu      sync.RWMutex
}

// New creates a new web server
func New(agg *aggregator.Aggregator, port int) *Server {
	s := &Server{
		agg:     agg,
		metrics: metrics.NewCollector(),
	}

	mux := http.NewServeMux()

	// Serve static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// API endpoints
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/connections", s.handleConnections)
	mux.HandleFunc("/api/system", s.handleSystem)
	mux.HandleFunc("/api/servicemap", s.handleServiceMap)
	mux.HandleFunc("/api/processes", s.handleProcesses)

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

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	sysMetrics := s.metrics.Collect()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sysMetrics)
}

// ServiceMapNode represents a node in the service map
type ServiceMapNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "process" or "external"
}

// ServiceMapLink represents a connection between nodes
type ServiceMapLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Count  int64  `json:"count"`
}

// ServiceMapResponse is the response for the service map API
type ServiceMapResponse struct {
	Nodes []ServiceMapNode `json:"nodes"`
	Links []ServiceMapLink `json:"links"`
}

func (s *Server) handleServiceMap(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()

	nodeMap := make(map[string]ServiceMapNode)
	var links []ServiceMapLink

	for _, conn := range stats.Connections {
		// Source node (process)
		sourceID := conn.Comm
		if _, exists := nodeMap[sourceID]; !exists {
			nodeMap[sourceID] = ServiceMapNode{
				ID:   sourceID,
				Name: conn.Comm,
				Type: "process",
			}
		}

		// Target node (remote)
		targetID := fmt.Sprintf("%s:%d", conn.Remote, conn.Port)
		if _, exists := nodeMap[targetID]; !exists {
			nodeMap[targetID] = ServiceMapNode{
				ID:   targetID,
				Name: targetID,
				Type: "external",
			}
		}

		links = append(links, ServiceMapLink{
			Source: sourceID,
			Target: targetID,
			Count:  conn.Count,
		})
	}

	nodes := make([]ServiceMapNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, node)
	}

	resp := ServiceMapResponse{
		Nodes: nodes,
		Links: links,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ProcessInfo holds information about a running process
type ProcessInfo struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	CPUPct  float64 `json:"cpu_pct"`
	MemMB   float64 `json:"mem_mb"`
	State   string  `json:"state"`
	Threads int     `json:"threads"`
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	processes := getTopProcesses(20)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

func getTopProcesses(limit int) []ProcessInfo {
	var processes []ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return processes
	}

	clkTck := float64(100) // Usually 100 Hz on Linux

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc := readProcessInfo(pid, clkTck)
		if proc.Name != "" {
			processes = append(processes, proc)
		}
	}

	// Sort by CPU usage descending
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].CPUPct > processes[j].CPUPct
	})

	if len(processes) > limit {
		processes = processes[:limit]
	}

	return processes
}

func readProcessInfo(pid int, clkTck float64) ProcessInfo {
	proc := ProcessInfo{PID: pid}

	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return proc
	}

	statStr := string(statData)
	start := strings.Index(statStr, "(")
	end := strings.LastIndex(statStr, ")")
	if start == -1 || end == -1 {
		return proc
	}

	proc.Name = statStr[start+1 : end]
	fields := strings.Fields(statStr[end+2:])
	if len(fields) < 20 {
		return proc
	}

	proc.State = fields[0]
	utime, _ := strconv.ParseFloat(fields[11], 64)
	stime, _ := strconv.ParseFloat(fields[12], 64)
	proc.Threads, _ = strconv.Atoi(fields[17])

	statmPath := filepath.Join("/proc", strconv.Itoa(pid), "statm")
	statmData, err := os.ReadFile(statmPath)
	if err == nil {
		statmFields := strings.Fields(string(statmData))
		if len(statmFields) > 1 {
			rss, _ := strconv.ParseFloat(statmFields[1], 64)
			proc.MemMB = (rss * 4096) / (1024 * 1024)
		}
	}

	uptimeData, err := os.ReadFile("/proc/uptime")
	if err == nil {
		uptimeFields := strings.Fields(string(uptimeData))
		if len(uptimeFields) > 0 {
			uptime, _ := strconv.ParseFloat(uptimeFields[0], 64)
			starttime, _ := strconv.ParseFloat(fields[19], 64)
			processAge := uptime - (starttime / clkTck)
			if processAge > 0 {
				totalCPU := (utime + stime) / clkTck
				proc.CPUPct = (totalCPU / processAge) * 100
			}
		}
	}

	return proc
}
