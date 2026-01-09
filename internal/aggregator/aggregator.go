package aggregator

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// EndpointStats holds statistics for a single endpoint
type EndpointStats struct {
	Method       string
	Path         string
	RequestCount int64
	ErrorCount   int64 // 4xx and 5xx responses
	Latencies    []time.Duration
	LastSeen     time.Time

	// Computed stats
	P50 time.Duration
	P99 time.Duration
	Avg time.Duration
}

// ConnectionStats tracks connections between services
type ConnectionStats struct {
	SourceComm string
	SourcePID  uint32
	DestAddr   string
	DestPort   uint16
	Count      int64
	LastSeen   time.Time
}

// PendingRequest tracks a request waiting for response
type PendingRequest struct {
	Method    string
	Path      string
	Timestamp time.Time
	PID       uint32
	TID       uint32
}

// Aggregator collects and summarizes observability data
type Aggregator struct {
	mu sync.RWMutex

	// Endpoint stats keyed by "METHOD /path"
	endpoints map[string]*EndpointStats

	// Pending requests keyed by "pid:tid"
	pending map[string]*PendingRequest

	// Connection stats keyed by "comm:pid -> addr:port"
	connections map[string]*ConnectionStats

	// Config
	maxLatencySamples int
	requestTimeout    time.Duration
}

// New creates a new Aggregator
func New() *Aggregator {
	a := &Aggregator{
		endpoints:         make(map[string]*EndpointStats),
		pending:           make(map[string]*PendingRequest),
		connections:       make(map[string]*ConnectionStats),
		maxLatencySamples: 1000, // Keep last 1000 latency samples per endpoint
		requestTimeout:    30 * time.Second,
	}

	// Start cleanup goroutine
	go a.cleanupLoop()

	return a
}

// RecordRequest records an HTTP request
func (a *Aggregator) RecordRequest(pid, tid uint32, method, path string, ts time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%d:%d", pid, tid)
	a.pending[key] = &PendingRequest{
		Method:    method,
		Path:      path,
		Timestamp: ts,
		PID:       pid,
		TID:       tid,
	}

	// Also increment request count
	endpointKey := fmt.Sprintf("%s %s", method, path)
	stats := a.getOrCreateEndpoint(endpointKey, method, path)
	stats.RequestCount++
	stats.LastSeen = ts
}

// RecordResponse records an HTTP response and calculates latency
func (a *Aggregator) RecordResponse(pid, tid uint32, statusCode int, ts time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%d:%d", pid, tid)
	req, exists := a.pending[key]
	if !exists {
		return
	}
	delete(a.pending, key)

	latency := ts.Sub(req.Timestamp)
	if latency < 0 {
		latency = 0
	}

	endpointKey := fmt.Sprintf("%s %s", req.Method, req.Path)
	stats := a.getOrCreateEndpoint(endpointKey, req.Method, req.Path)

	// Record latency
	if len(stats.Latencies) >= a.maxLatencySamples {
		// Remove oldest sample
		stats.Latencies = stats.Latencies[1:]
	}
	stats.Latencies = append(stats.Latencies, latency)

	// Record errors (4xx and 5xx)
	if statusCode >= 400 {
		stats.ErrorCount++
	}

	stats.LastSeen = ts
	a.computePercentiles(stats)
}

// RecordConnection records a TCP connection
func (a *Aggregator) RecordConnection(comm string, pid uint32, destAddr string, destPort uint16) {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%s:%d -> %s:%d", comm, pid, destAddr, destPort)
	conn, exists := a.connections[key]
	if !exists {
		conn = &ConnectionStats{
			SourceComm: comm,
			SourcePID:  pid,
			DestAddr:   destAddr,
			DestPort:   destPort,
		}
		a.connections[key] = conn
	}
	conn.Count++
	conn.LastSeen = time.Now()
}

func (a *Aggregator) getOrCreateEndpoint(key, method, path string) *EndpointStats {
	stats, exists := a.endpoints[key]
	if !exists {
		stats = &EndpointStats{
			Method:    method,
			Path:      path,
			Latencies: make([]time.Duration, 0, a.maxLatencySamples),
		}
		a.endpoints[key] = stats
	}
	return stats
}

func (a *Aggregator) computePercentiles(stats *EndpointStats) {
	n := len(stats.Latencies)
	if n == 0 {
		return
	}

	// Make a sorted copy
	sorted := make([]time.Duration, n)
	copy(sorted, stats.Latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Compute average
	var total time.Duration
	for _, l := range sorted {
		total += l
	}
	stats.Avg = total / time.Duration(n)

	// Compute percentiles
	stats.P50 = sorted[n*50/100]
	stats.P99 = sorted[n*99/100]
}

// GetEndpointStats returns a copy of all endpoint stats
func (a *Aggregator) GetEndpointStats() []EndpointStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]EndpointStats, 0, len(a.endpoints))
	for _, stats := range a.endpoints {
		// Recompute percentiles
		a.computePercentiles(stats)
		result = append(result, *stats)
	}

	// Sort by request count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].RequestCount > result[j].RequestCount
	})

	return result
}

// GetConnectionStats returns a copy of all connection stats
func (a *Aggregator) GetConnectionStats() []ConnectionStats {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]ConnectionStats, 0, len(a.connections))
	for _, conn := range a.connections {
		result = append(result, *conn)
	}

	// Sort by count descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// Stats holds all aggregated statistics for the web API
type Stats struct {
	TotalConnections int64
	TotalRequests    int64
	TotalErrors      int64
	Endpoints        []EndpointStats
	Connections      []ConnectionStatsView
}

// ConnectionStatsView is a simplified view for the web API
type ConnectionStatsView struct {
	Comm   string
	PID    uint32
	Remote string
	Port   uint16
	Count  int64
}

// GetStats returns all stats for the web API
func (a *Aggregator) GetStats() Stats {
	endpoints := a.GetEndpointStats()
	connections := a.GetConnectionStats()

	var totalRequests, totalErrors int64
	for _, ep := range endpoints {
		totalRequests += ep.RequestCount
		totalErrors += ep.ErrorCount
	}

	var totalConnections int64
	connViews := make([]ConnectionStatsView, 0, len(connections))
	for _, conn := range connections {
		totalConnections += conn.Count
		connViews = append(connViews, ConnectionStatsView{
			Comm:   conn.SourceComm,
			PID:    conn.SourcePID,
			Remote: conn.DestAddr,
			Port:   conn.DestPort,
			Count:  conn.Count,
		})
	}

	return Stats{
		TotalConnections: totalConnections,
		TotalRequests:    totalRequests,
		TotalErrors:      totalErrors,
		Endpoints:        endpoints,
		Connections:      connViews,
	}
}

// Summary returns a formatted summary of current stats
func (a *Aggregator) Summary() string {
	endpoints := a.GetEndpointStats()
	connections := a.GetConnectionStats()

	var result string

	result += "\n╔══════════════════════════════════════════════════════════════════════╗\n"
	result += "║                         ENDPOINT STATS                               ║\n"
	result += "╠══════════════════════════════════════════════════════════════════════╣\n"

	if len(endpoints) == 0 {
		result += "║  No HTTP endpoints recorded yet                                     ║\n"
	} else {
		result += fmt.Sprintf("║ %-7s %-30s %6s %6s %8s %8s ║\n", "METHOD", "PATH", "COUNT", "ERR", "P50", "P99")
		result += "╟──────────────────────────────────────────────────────────────────────╢\n"
		for i, ep := range endpoints {
			if i >= 10 {
				result += fmt.Sprintf("║  ... and %d more endpoints                                          ║\n", len(endpoints)-10)
				break
			}
			path := ep.Path
			if len(path) > 30 {
				path = path[:27] + "..."
			}
			result += fmt.Sprintf("║ %-7s %-30s %6d %6d %8s %8s ║\n",
				ep.Method, path, ep.RequestCount, ep.ErrorCount,
				formatDuration(ep.P50), formatDuration(ep.P99))
		}
	}

	result += "╠══════════════════════════════════════════════════════════════════════╣\n"
	result += "║                        CONNECTION MAP                                ║\n"
	result += "╠══════════════════════════════════════════════════════════════════════╣\n"

	if len(connections) == 0 {
		result += "║  No connections recorded yet                                         ║\n"
	} else {
		result += fmt.Sprintf("║ %-16s %-25s %8s                   ║\n", "SOURCE", "DESTINATION", "COUNT")
		result += "╟──────────────────────────────────────────────────────────────────────╢\n"
		for i, conn := range connections {
			if i >= 10 {
				result += fmt.Sprintf("║  ... and %d more connections                                        ║\n", len(connections)-10)
				break
			}
			dest := fmt.Sprintf("%s:%d", conn.DestAddr, conn.DestPort)
			result += fmt.Sprintf("║ %-16s %-25s %8d                   ║\n",
				truncate(conn.SourceComm, 16), truncate(dest, 25), conn.Count)
		}
	}

	result += "╚══════════════════════════════════════════════════════════════════════╝\n"

	return result
}

func (a *Aggregator) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		a.cleanup()
	}
}

func (a *Aggregator) cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	// Clean up stale pending requests
	for key, req := range a.pending {
		if now.Sub(req.Timestamp) > a.requestTimeout {
			delete(a.pending, key)
		}
	}

	// Clean up old connections (older than 5 minutes)
	for key, conn := range a.connections {
		if now.Sub(conn.LastSeen) > 5*time.Minute {
			delete(a.connections, key)
		}
	}
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
