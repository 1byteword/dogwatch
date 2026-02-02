package correlation

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// CorrelationMethod indicates how a correlation was established
type CorrelationMethod string

const (
	MethodHeader  CorrelationMethod = "header"  // Explicit trace headers
	MethodSocket  CorrelationMethod = "socket"  // Socket + PID/TID tracking
	MethodTiming  CorrelationMethod = "timing"  // Time-window heuristics
	MethodContent CorrelationMethod = "content" // Request ID in body/headers
	MethodUnknown CorrelationMethod = "unknown"
)

// CorrelationConfidence returns the confidence score for a method
func (m CorrelationMethod) Confidence() float64 {
	switch m {
	case MethodHeader:
		return 0.95
	case MethodSocket:
		return 0.85
	case MethodContent:
		return 0.80
	case MethodTiming:
		return 0.70
	default:
		return 0.50
	}
}

// CorrelationResult represents a correlation finding
type CorrelationResult struct {
	InboundRequestID  string            `json:"inbound_request_id"`
	OutboundRequestID string            `json:"outbound_request_id"`
	Method            CorrelationMethod `json:"method"`
	Confidence        float64           `json:"confidence"`
	Warnings          []string          `json:"warnings,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

// ThreadKey uniquely identifies a thread
type ThreadKey struct {
	PID uint32
	TID uint32
}

func (k ThreadKey) String() string {
	return fmt.Sprintf("%d:%d", k.PID, k.TID)
}

// RequestEvent represents an observed request or response
type RequestEvent struct {
	ID           string
	Type         RequestEventType
	ThreadKey    ThreadKey
	SocketCookie uint64
	Timestamp    time.Time
	Duration     time.Duration // For completed requests
	ServiceName  string
	Operation    string // HTTP method, DB operation, etc.
	Endpoint     string // URL path, DB table, etc.
	Direction    RequestDirection
	Attributes   map[string]string
}

// RequestEventType categorizes the event
type RequestEventType string

const (
	EventTypeHTTP     RequestEventType = "http"
	EventTypeDatabase RequestEventType = "database"
	EventTypeGRPC     RequestEventType = "grpc"
	EventTypeRedis    RequestEventType = "redis"
	EventTypeTCP      RequestEventType = "tcp"
)

// RequestDirection indicates if request is inbound or outbound
type RequestDirection string

const (
	DirectionInbound  RequestDirection = "inbound"
	DirectionOutbound RequestDirection = "outbound"
)

// ThreadState tracks what a thread is currently doing
type ThreadState struct {
	ThreadKey       ThreadKey
	CurrentRequest  *RequestEvent   // Inbound request being handled
	OutboundCalls   []*RequestEvent // Outbound calls made during handling
	State           ThreadActivity
	LastActivity    time.Time
	ProcessedCount  int64
}

// ThreadActivity represents thread's current activity
type ThreadActivity string

const (
	ActivityIdle       ThreadActivity = "idle"
	ActivityProcessing ThreadActivity = "processing"
	ActivityWaiting    ThreadActivity = "waiting" // Waiting for outbound response
)

// HeaderlessCorrelator performs correlation without trace headers
type HeaderlessCorrelator struct {
	mu sync.RWMutex

	// Thread tracking
	threadStates map[ThreadKey]*ThreadState

	// Socket tracking: socket -> list of requests
	socketRequests map[uint64][]*RequestEvent

	// Pending outbound requests awaiting correlation
	pendingOutbound map[string]*RequestEvent // requestID -> event

	// Completed correlations
	correlations map[string][]*CorrelationResult // inboundID -> correlations

	// Configuration
	config HeaderlessConfig

	// Metrics
	stats HeaderlessStats
}

// HeaderlessConfig configures the correlator
type HeaderlessConfig struct {
	// Time window for timing-based correlation
	TimingWindowMs int64 // Default: 5000ms

	// Maximum time to track a thread's state
	ThreadStateMaxAge time.Duration // Default: 30s

	// Maximum pending outbound requests to track
	MaxPendingOutbound int // Default: 10000

	// Enable connection pooling detection
	DetectConnectionPooling bool // Default: true

	// Enable async worker detection
	DetectAsyncWorkers bool // Default: true

	// Content correlation headers to check
	CorrelationHeaders []string
}

// DefaultHeaderlessConfig returns sensible defaults
func DefaultHeaderlessConfig() HeaderlessConfig {
	return HeaderlessConfig{
		TimingWindowMs:          5000,
		ThreadStateMaxAge:       30 * time.Second,
		MaxPendingOutbound:      10000,
		DetectConnectionPooling: true,
		DetectAsyncWorkers:      true,
		CorrelationHeaders: []string{
			"X-Request-ID",
			"X-Correlation-ID",
			"X-Trace-ID",
			"Request-Id",
			"X-Amzn-Trace-Id",
		},
	}
}

// HeaderlessStats tracks correlation statistics
type HeaderlessStats struct {
	TotalInboundRequests  int64 `json:"total_inbound_requests"`
	TotalOutboundRequests int64 `json:"total_outbound_requests"`
	CorrelationsFound     int64 `json:"correlations_found"`
	CorrelationsByMethod  map[CorrelationMethod]int64 `json:"correlations_by_method"`
	AverageConfidence     float64 `json:"average_confidence"`
	ConnectionPoolingDetected int64 `json:"connection_pooling_detected"`
	AsyncWorkerDetected   int64 `json:"async_worker_detected"`
}

// NewHeaderlessCorrelator creates a new headerless correlator
func NewHeaderlessCorrelator(config HeaderlessConfig) *HeaderlessCorrelator {
	h := &HeaderlessCorrelator{
		threadStates:    make(map[ThreadKey]*ThreadState),
		socketRequests:  make(map[uint64][]*RequestEvent),
		pendingOutbound: make(map[string]*RequestEvent),
		correlations:    make(map[string][]*CorrelationResult),
		config:          config,
		stats: HeaderlessStats{
			CorrelationsByMethod: make(map[CorrelationMethod]int64),
		},
	}

	// Start cleanup goroutine
	go h.cleanupLoop()

	return h
}

// RecordInboundRequest records an inbound request starting
func (h *HeaderlessCorrelator) RecordInboundRequest(event *RequestEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event.Direction = DirectionInbound
	h.stats.TotalInboundRequests++

	// Update thread state
	state, ok := h.threadStates[event.ThreadKey]
	if !ok {
		state = &ThreadState{
			ThreadKey: event.ThreadKey,
		}
		h.threadStates[event.ThreadKey] = state
	}

	state.CurrentRequest = event
	state.State = ActivityProcessing
	state.LastActivity = event.Timestamp
	state.OutboundCalls = nil // Reset outbound calls for new request

	// Track by socket
	if event.SocketCookie != 0 {
		h.socketRequests[event.SocketCookie] = append(
			h.socketRequests[event.SocketCookie], event)
	}
}

// RecordInboundComplete records an inbound request completing
func (h *HeaderlessCorrelator) RecordInboundComplete(requestID string, endTime time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Find the thread handling this request
	for _, state := range h.threadStates {
		if state.CurrentRequest != nil && state.CurrentRequest.ID == requestID {
			state.CurrentRequest.Duration = endTime.Sub(state.CurrentRequest.Timestamp)
			state.State = ActivityIdle
			state.ProcessedCount++
			state.LastActivity = endTime

			// Finalize correlations for all outbound calls
			for _, outbound := range state.OutboundCalls {
				h.finalizeCorrelation(state.CurrentRequest, outbound)
			}

			state.CurrentRequest = nil
			state.OutboundCalls = nil
			break
		}
	}
}

// RecordOutboundRequest records an outbound request
func (h *HeaderlessCorrelator) RecordOutboundRequest(event *RequestEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	event.Direction = DirectionOutbound
	h.stats.TotalOutboundRequests++

	// Track by socket for connection pooling detection (always, not just for uncorrelated)
	if event.SocketCookie != 0 {
		h.socketRequests[event.SocketCookie] = append(
			h.socketRequests[event.SocketCookie], event)
	}

	// Try to correlate immediately via thread
	state, ok := h.threadStates[event.ThreadKey]
	// Allow correlation when processing OR waiting (multiple outbound calls from same request)
	if ok && state.CurrentRequest != nil && (state.State == ActivityProcessing || state.State == ActivityWaiting) {
		// Same thread is handling an inbound request - direct correlation
		state.OutboundCalls = append(state.OutboundCalls, event)
		state.State = ActivityWaiting

		var warnings []string

		// Check for connection pooling
		if h.config.DetectConnectionPooling && event.SocketCookie != 0 {
			socketReqs := h.socketRequests[event.SocketCookie]
			if len(socketReqs) > 2 {
				warnings = append(warnings, "connection_pooling_detected")
				h.stats.ConnectionPoolingDetected++
			}
		}

		result := &CorrelationResult{
			InboundRequestID:  state.CurrentRequest.ID,
			OutboundRequestID: event.ID,
			Method:            MethodSocket,
			Confidence:        MethodSocket.Confidence(),
			Warnings:          warnings,
			Metadata: map[string]string{
				"thread": event.ThreadKey.String(),
			},
		}

		h.addCorrelation(state.CurrentRequest.ID, result)
		return
	}

	// Can't correlate by thread - add to pending for timing-based correlation
	h.pendingOutbound[event.ID] = event
}

// RecordOutboundComplete records an outbound request completing
func (h *HeaderlessCorrelator) RecordOutboundComplete(requestID string, endTime time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Update pending outbound
	if event, ok := h.pendingOutbound[requestID]; ok {
		event.Duration = endTime.Sub(event.Timestamp)

		// Try timing-based correlation
		h.tryTimingCorrelation(event)
	}

	// Update thread state
	for _, state := range h.threadStates {
		if state.State == ActivityWaiting {
			for _, outbound := range state.OutboundCalls {
				if outbound.ID == requestID {
					outbound.Duration = endTime.Sub(outbound.Timestamp)
					state.State = ActivityProcessing
					state.LastActivity = endTime
					break
				}
			}
		}
	}
}

// tryTimingCorrelation attempts to correlate an outbound request by timing
func (h *HeaderlessCorrelator) tryTimingCorrelation(outbound *RequestEvent) {
	windowMs := h.config.TimingWindowMs
	window := time.Duration(windowMs) * time.Millisecond

	var candidates []*RequestEvent
	var warnings []string

	// Find inbound requests that could have triggered this outbound
	for _, state := range h.threadStates {
		if state.CurrentRequest == nil {
			continue
		}

		inbound := state.CurrentRequest

		// Check timing constraints:
		// 1. Outbound started after inbound started
		// 2. Outbound started within window of inbound start
		if outbound.Timestamp.Before(inbound.Timestamp) {
			continue
		}

		timeDiff := outbound.Timestamp.Sub(inbound.Timestamp)
		if timeDiff > window {
			continue
		}

		// Check if same PID (different thread but same process)
		if outbound.ThreadKey.PID == inbound.ThreadKey.PID {
			candidates = append(candidates, inbound)
		}
	}

	if len(candidates) == 0 {
		return
	}

	// Multiple candidates - lower confidence
	if len(candidates) > 1 {
		warnings = append(warnings, "multiple_candidates")
	}

	// Check for connection pooling
	if h.config.DetectConnectionPooling && outbound.SocketCookie != 0 {
		socketReqs := h.socketRequests[outbound.SocketCookie]
		if len(socketReqs) > 2 {
			warnings = append(warnings, "connection_pooling_likely")
			h.stats.ConnectionPoolingDetected++
		}
	}

	// Check for async worker pattern
	if h.config.DetectAsyncWorkers {
		for _, candidate := range candidates {
			if candidate.ThreadKey.TID != outbound.ThreadKey.TID {
				warnings = append(warnings, "async_worker_pattern")
				h.stats.AsyncWorkerDetected++
				break
			}
		}
	}

	// Sort by time proximity and pick best
	sort.Slice(candidates, func(i, j int) bool {
		diffI := outbound.Timestamp.Sub(candidates[i].Timestamp)
		diffJ := outbound.Timestamp.Sub(candidates[j].Timestamp)
		return diffI < diffJ
	})

	best := candidates[0]
	timeDiff := outbound.Timestamp.Sub(best.Timestamp)

	// Calculate confidence based on time proximity
	// Closer in time = higher confidence
	confidence := MethodTiming.Confidence()
	proximityFactor := 1.0 - (float64(timeDiff.Milliseconds()) / float64(windowMs))
	if proximityFactor < 0 {
		proximityFactor = 0
	}
	confidence *= (0.7 + 0.3*proximityFactor) // Scale between 70% and 100% of base

	if len(candidates) > 1 {
		confidence *= 0.8 // Reduce confidence for ambiguous cases
	}

	result := &CorrelationResult{
		InboundRequestID:  best.ID,
		OutboundRequestID: outbound.ID,
		Method:            MethodTiming,
		Confidence:        confidence,
		Warnings:          warnings,
		Metadata: map[string]string{
			"time_diff_ms":     fmt.Sprintf("%d", timeDiff.Milliseconds()),
			"candidates_count": fmt.Sprintf("%d", len(candidates)),
		},
	}

	h.addCorrelation(best.ID, result)
}

// finalizeCorrelation creates correlation result for socket-based correlation
func (h *HeaderlessCorrelator) finalizeCorrelation(inbound, outbound *RequestEvent) {
	// Check if we already have this correlation
	existing := h.correlations[inbound.ID]
	for _, c := range existing {
		if c.OutboundRequestID == outbound.ID {
			return // Already correlated
		}
	}

	var warnings []string

	// Check for connection pooling
	if h.config.DetectConnectionPooling && outbound.SocketCookie != 0 {
		socketReqs := h.socketRequests[outbound.SocketCookie]
		if len(socketReqs) > 2 {
			warnings = append(warnings, "connection_pooling_detected")
			h.stats.ConnectionPoolingDetected++
		}
	}

	result := &CorrelationResult{
		InboundRequestID:  inbound.ID,
		OutboundRequestID: outbound.ID,
		Method:            MethodSocket,
		Confidence:        MethodSocket.Confidence(),
		Warnings:          warnings,
		Metadata: map[string]string{
			"thread": outbound.ThreadKey.String(),
		},
	}

	h.addCorrelation(inbound.ID, result)
}

func (h *HeaderlessCorrelator) addCorrelation(inboundID string, result *CorrelationResult) {
	h.correlations[inboundID] = append(h.correlations[inboundID], result)
	h.stats.CorrelationsFound++
	h.stats.CorrelationsByMethod[result.Method]++

	// Update average confidence
	total := float64(h.stats.CorrelationsFound)
	h.stats.AverageConfidence = ((h.stats.AverageConfidence * (total - 1)) + result.Confidence) / total
}

// GetCorrelations returns all correlations for an inbound request
func (h *HeaderlessCorrelator) GetCorrelations(inboundRequestID string) []*CorrelationResult {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.correlations[inboundRequestID]
}

// GetThreadState returns the current state of a thread
func (h *HeaderlessCorrelator) GetThreadState(key ThreadKey) *ThreadState {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if state, ok := h.threadStates[key]; ok {
		// Return a copy to avoid race conditions
		copy := *state
		return &copy
	}
	return nil
}

// CorrelateByContent tries to correlate using request content
func (h *HeaderlessCorrelator) CorrelateByContent(inboundID string, headers map[string]string) *CorrelationResult {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check for correlation headers
	for _, headerName := range h.config.CorrelationHeaders {
		if value, ok := headers[headerName]; ok && value != "" {
			// Look for pending outbound with matching ID
			for outboundID, outbound := range h.pendingOutbound {
				if outboundAttr, ok := outbound.Attributes[headerName]; ok && outboundAttr == value {
					result := &CorrelationResult{
						InboundRequestID:  inboundID,
						OutboundRequestID: outboundID,
						Method:            MethodContent,
						Confidence:        MethodContent.Confidence(),
						Metadata: map[string]string{
							"header":       headerName,
							"header_value": value,
						},
					}
					h.addCorrelation(inboundID, result)
					delete(h.pendingOutbound, outboundID)
					return result
				}
			}
		}
	}

	return nil
}

// Stats returns current statistics
func (h *HeaderlessCorrelator) Stats() HeaderlessStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Return a copy
	statsCopy := h.stats
	statsCopy.CorrelationsByMethod = make(map[CorrelationMethod]int64)
	for k, v := range h.stats.CorrelationsByMethod {
		statsCopy.CorrelationsByMethod[k] = v
	}

	return statsCopy
}

// cleanupLoop periodically cleans up stale state
func (h *HeaderlessCorrelator) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.cleanup()
	}
}

func (h *HeaderlessCorrelator) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	maxAge := h.config.ThreadStateMaxAge

	// Clean up old thread states
	for key, state := range h.threadStates {
		if now.Sub(state.LastActivity) > maxAge {
			delete(h.threadStates, key)
		}
	}

	// Clean up old pending outbound
	for id, event := range h.pendingOutbound {
		if now.Sub(event.Timestamp) > maxAge {
			delete(h.pendingOutbound, id)
		}
	}

	// Clean up old socket requests
	for socket, requests := range h.socketRequests {
		var active []*RequestEvent
		for _, req := range requests {
			if now.Sub(req.Timestamp) <= maxAge {
				active = append(active, req)
			}
		}
		if len(active) == 0 {
			delete(h.socketRequests, socket)
		} else {
			h.socketRequests[socket] = active
		}
	}

	// Limit pending outbound size
	if len(h.pendingOutbound) > h.config.MaxPendingOutbound {
		// Remove oldest entries
		var oldest []struct {
			id   string
			time time.Time
		}
		for id, event := range h.pendingOutbound {
			oldest = append(oldest, struct {
				id   string
				time time.Time
			}{id, event.Timestamp})
		}
		sort.Slice(oldest, func(i, j int) bool {
			return oldest[i].time.Before(oldest[j].time)
		})

		toRemove := len(h.pendingOutbound) - h.config.MaxPendingOutbound
		for i := 0; i < toRemove; i++ {
			delete(h.pendingOutbound, oldest[i].id)
		}
	}
}

// Reset clears all state (useful for testing)
func (h *HeaderlessCorrelator) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.threadStates = make(map[ThreadKey]*ThreadState)
	h.socketRequests = make(map[uint64][]*RequestEvent)
	h.pendingOutbound = make(map[string]*RequestEvent)
	h.correlations = make(map[string][]*CorrelationResult)
	h.stats = HeaderlessStats{
		CorrelationsByMethod: make(map[CorrelationMethod]int64),
	}
}
