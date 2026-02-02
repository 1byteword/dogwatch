package correlation

import (
	"fmt"
	"testing"
	"time"
)

func TestHeaderlessCorrelator_SocketBasedCorrelation(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Create an inbound request
	inbound := &RequestEvent{
		ID:           "inbound-1",
		Type:         EventTypeHTTP,
		ThreadKey:    ThreadKey{PID: 1000, TID: 2000},
		SocketCookie: 12345,
		Timestamp:    time.Now(),
		ServiceName:  "api-gateway",
		Operation:    "GET",
		Endpoint:     "/api/users",
	}
	correlator.RecordInboundRequest(inbound)

	// Same thread makes an outbound call
	outbound := &RequestEvent{
		ID:           "outbound-1",
		Type:         EventTypeDatabase,
		ThreadKey:    ThreadKey{PID: 1000, TID: 2000}, // Same thread
		SocketCookie: 67890,
		Timestamp:    time.Now().Add(10 * time.Millisecond),
		ServiceName:  "api-gateway",
		Operation:    "SELECT",
		Endpoint:     "users",
	}
	correlator.RecordOutboundRequest(outbound)

	// Check correlation was established
	correlations := correlator.GetCorrelations("inbound-1")
	if len(correlations) != 1 {
		t.Fatalf("Expected 1 correlation, got %d", len(correlations))
	}

	corr := correlations[0]
	if corr.Method != MethodSocket {
		t.Errorf("Expected method=socket, got %s", corr.Method)
	}
	if corr.Confidence < 0.8 {
		t.Errorf("Expected confidence >= 0.8, got %f", corr.Confidence)
	}
	if corr.OutboundRequestID != "outbound-1" {
		t.Errorf("Expected outbound-1, got %s", corr.OutboundRequestID)
	}
}

func TestHeaderlessCorrelator_TimingBasedCorrelation(t *testing.T) {
	config := DefaultHeaderlessConfig()
	config.TimingWindowMs = 1000 // 1 second window
	correlator := NewHeaderlessCorrelator(config)

	baseTime := time.Now()

	// Create an inbound request
	inbound := &RequestEvent{
		ID:           "inbound-1",
		Type:         EventTypeHTTP,
		ThreadKey:    ThreadKey{PID: 1000, TID: 2000},
		Timestamp:    baseTime,
		ServiceName:  "api-gateway",
	}
	correlator.RecordInboundRequest(inbound)

	// Outbound from different thread but same process
	outbound := &RequestEvent{
		ID:           "outbound-1",
		Type:         EventTypeDatabase,
		ThreadKey:    ThreadKey{PID: 1000, TID: 3000}, // Different thread, same PID
		Timestamp:    baseTime.Add(100 * time.Millisecond),
		ServiceName:  "api-gateway",
	}
	correlator.RecordOutboundRequest(outbound)

	// Complete the outbound to trigger timing correlation
	correlator.RecordOutboundComplete("outbound-1", baseTime.Add(200*time.Millisecond))

	// Check correlation
	correlations := correlator.GetCorrelations("inbound-1")
	if len(correlations) != 1 {
		t.Fatalf("Expected 1 correlation, got %d", len(correlations))
	}

	corr := correlations[0]
	if corr.Method != MethodTiming {
		t.Errorf("Expected method=timing, got %s", corr.Method)
	}
	// Timing-based should have lower confidence
	if corr.Confidence >= 0.85 {
		t.Errorf("Timing correlation should have confidence < 0.85, got %f", corr.Confidence)
	}

	// Should have async worker warning
	hasAsyncWarning := false
	for _, w := range corr.Warnings {
		if w == "async_worker_pattern" {
			hasAsyncWarning = true
			break
		}
	}
	if !hasAsyncWarning {
		t.Error("Expected async_worker_pattern warning")
	}
}

func TestHeaderlessCorrelator_NoCorrelationForDifferentProcess(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	baseTime := time.Now()

	// Inbound from process 1000
	inbound := &RequestEvent{
		ID:        "inbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: ThreadKey{PID: 1000, TID: 2000},
		Timestamp: baseTime,
	}
	correlator.RecordInboundRequest(inbound)

	// Outbound from different process
	outbound := &RequestEvent{
		ID:        "outbound-1",
		Type:      EventTypeDatabase,
		ThreadKey: ThreadKey{PID: 2000, TID: 3000}, // Different PID
		Timestamp: baseTime.Add(10 * time.Millisecond),
	}
	correlator.RecordOutboundRequest(outbound)
	correlator.RecordOutboundComplete("outbound-1", baseTime.Add(20*time.Millisecond))

	// Should have no correlation
	correlations := correlator.GetCorrelations("inbound-1")
	if len(correlations) != 0 {
		t.Errorf("Expected 0 correlations for different process, got %d", len(correlations))
	}
}

func TestHeaderlessCorrelator_MultipleOutboundCalls(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Create an inbound request
	inbound := &RequestEvent{
		ID:        "inbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: ThreadKey{PID: 1000, TID: 2000},
		Timestamp: time.Now(),
	}
	correlator.RecordInboundRequest(inbound)

	// Multiple outbound calls from same thread
	for i := 1; i <= 3; i++ {
		outbound := &RequestEvent{
			ID:        fmt.Sprintf("outbound-%d", i),
			Type:      EventTypeDatabase,
			ThreadKey: ThreadKey{PID: 1000, TID: 2000},
			Timestamp: time.Now().Add(time.Duration(i*10) * time.Millisecond),
		}
		correlator.RecordOutboundRequest(outbound)
	}

	// Should have 3 correlations
	correlations := correlator.GetCorrelations("inbound-1")
	if len(correlations) != 3 {
		t.Errorf("Expected 3 correlations, got %d", len(correlations))
	}
}

func TestHeaderlessCorrelator_ConnectionPoolingDetection(t *testing.T) {
	config := DefaultHeaderlessConfig()
	config.DetectConnectionPooling = true
	correlator := NewHeaderlessCorrelator(config)

	baseTime := time.Now()
	socket := uint64(99999)

	// Multiple requests on same socket (simulating connection pooling)
	for i := 1; i <= 5; i++ {
		inbound := &RequestEvent{
			ID:           fmt.Sprintf("inbound-%d", i),
			Type:         EventTypeHTTP,
			ThreadKey:    ThreadKey{PID: 1000, TID: uint32(2000 + i)},
			SocketCookie: socket,
			Timestamp:    baseTime.Add(time.Duration(i*100) * time.Millisecond),
		}
		correlator.RecordInboundRequest(inbound)

		outbound := &RequestEvent{
			ID:           fmt.Sprintf("outbound-%d", i),
			Type:         EventTypeDatabase,
			ThreadKey:    ThreadKey{PID: 1000, TID: uint32(2000 + i)},
			SocketCookie: socket,
			Timestamp:    baseTime.Add(time.Duration(i*100+10) * time.Millisecond),
		}
		correlator.RecordOutboundRequest(outbound)
		correlator.RecordInboundComplete(fmt.Sprintf("inbound-%d", i),
			baseTime.Add(time.Duration(i*100+50)*time.Millisecond))
	}

	// Check that connection pooling was detected
	stats := correlator.Stats()
	if stats.ConnectionPoolingDetected == 0 {
		t.Error("Expected connection pooling to be detected")
	}
}

func TestHeaderlessCorrelator_ContentBasedCorrelation(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Create a pending outbound with correlation header
	outbound := &RequestEvent{
		ID:        "outbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: ThreadKey{PID: 2000, TID: 3000}, // Different process
		Timestamp: time.Now(),
		Attributes: map[string]string{
			"X-Request-ID": "req-abc-123",
		},
	}
	correlator.RecordOutboundRequest(outbound)

	// Try to correlate by content
	result := correlator.CorrelateByContent("inbound-1", map[string]string{
		"X-Request-ID": "req-abc-123",
	})

	if result == nil {
		t.Fatal("Expected correlation result")
	}
	if result.Method != MethodContent {
		t.Errorf("Expected method=content, got %s", result.Method)
	}
	if result.OutboundRequestID != "outbound-1" {
		t.Errorf("Expected outbound-1, got %s", result.OutboundRequestID)
	}
}

func TestHeaderlessCorrelator_ThreadState(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	threadKey := ThreadKey{PID: 1000, TID: 2000}

	// Initially no state
	state := correlator.GetThreadState(threadKey)
	if state != nil {
		t.Error("Expected nil state for unknown thread")
	}

	// Record inbound
	inbound := &RequestEvent{
		ID:        "inbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: threadKey,
		Timestamp: time.Now(),
	}
	correlator.RecordInboundRequest(inbound)

	// Check state
	state = correlator.GetThreadState(threadKey)
	if state == nil {
		t.Fatal("Expected state after recording request")
	}
	if state.State != ActivityProcessing {
		t.Errorf("Expected state=processing, got %s", state.State)
	}
	if state.CurrentRequest == nil {
		t.Error("Expected current request to be set")
	}
}

func TestHeaderlessCorrelator_Stats(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Record some activity
	for i := 0; i < 10; i++ {
		inbound := &RequestEvent{
			ID:        fmt.Sprintf("inbound-%d", i),
			Type:      EventTypeHTTP,
			ThreadKey: ThreadKey{PID: 1000, TID: uint32(2000 + i)},
			Timestamp: time.Now(),
		}
		correlator.RecordInboundRequest(inbound)

		outbound := &RequestEvent{
			ID:        fmt.Sprintf("outbound-%d", i),
			Type:      EventTypeDatabase,
			ThreadKey: ThreadKey{PID: 1000, TID: uint32(2000 + i)},
			Timestamp: time.Now(),
		}
		correlator.RecordOutboundRequest(outbound)
	}

	stats := correlator.Stats()
	if stats.TotalInboundRequests != 10 {
		t.Errorf("Expected 10 inbound, got %d", stats.TotalInboundRequests)
	}
	if stats.TotalOutboundRequests != 10 {
		t.Errorf("Expected 10 outbound, got %d", stats.TotalOutboundRequests)
	}
	if stats.CorrelationsFound != 10 {
		t.Errorf("Expected 10 correlations, got %d", stats.CorrelationsFound)
	}
	if stats.CorrelationsByMethod[MethodSocket] != 10 {
		t.Errorf("Expected 10 socket correlations, got %d", stats.CorrelationsByMethod[MethodSocket])
	}
}

func TestHeaderlessCorrelator_Reset(t *testing.T) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Add some state
	inbound := &RequestEvent{
		ID:        "inbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: ThreadKey{PID: 1000, TID: 2000},
		Timestamp: time.Now(),
	}
	correlator.RecordInboundRequest(inbound)

	// Reset
	correlator.Reset()

	// Verify cleared
	state := correlator.GetThreadState(ThreadKey{PID: 1000, TID: 2000})
	if state != nil {
		t.Error("Expected nil state after reset")
	}

	stats := correlator.Stats()
	if stats.TotalInboundRequests != 0 {
		t.Error("Expected stats to be reset")
	}
}

func TestCorrelationMethod_Confidence(t *testing.T) {
	tests := []struct {
		method     CorrelationMethod
		minConf    float64
		maxConf    float64
	}{
		{MethodHeader, 0.90, 1.0},
		{MethodSocket, 0.80, 0.90},
		{MethodContent, 0.75, 0.85},
		{MethodTiming, 0.65, 0.75},
		{MethodUnknown, 0.40, 0.60},
	}

	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			conf := tt.method.Confidence()
			if conf < tt.minConf || conf > tt.maxConf {
				t.Errorf("Confidence for %s = %f, expected between %f and %f",
					tt.method, conf, tt.minConf, tt.maxConf)
			}
		})
	}
}

func TestHeaderlessCorrelator_OutOfOrderOutbound(t *testing.T) {
	config := DefaultHeaderlessConfig()
	config.TimingWindowMs = 5000
	correlator := NewHeaderlessCorrelator(config)

	baseTime := time.Now()

	// Outbound request happens BEFORE inbound (should not correlate)
	outbound := &RequestEvent{
		ID:        "outbound-1",
		Type:      EventTypeDatabase,
		ThreadKey: ThreadKey{PID: 1000, TID: 3000},
		Timestamp: baseTime,
	}
	correlator.RecordOutboundRequest(outbound)

	// Inbound comes later
	inbound := &RequestEvent{
		ID:        "inbound-1",
		Type:      EventTypeHTTP,
		ThreadKey: ThreadKey{PID: 1000, TID: 2000},
		Timestamp: baseTime.Add(100 * time.Millisecond),
	}
	correlator.RecordInboundRequest(inbound)

	// Complete outbound
	correlator.RecordOutboundComplete("outbound-1", baseTime.Add(50*time.Millisecond))

	// Should not correlate (outbound started before inbound)
	correlations := correlator.GetCorrelations("inbound-1")
	if len(correlations) != 0 {
		t.Errorf("Expected 0 correlations for out-of-order, got %d", len(correlations))
	}
}

func BenchmarkHeaderlessCorrelator_SocketCorrelation(b *testing.B) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		threadKey := ThreadKey{PID: 1000, TID: uint32(2000 + i%100)}

		inbound := &RequestEvent{
			ID:        fmt.Sprintf("inbound-%d", i),
			Type:      EventTypeHTTP,
			ThreadKey: threadKey,
			Timestamp: time.Now(),
		}
		correlator.RecordInboundRequest(inbound)

		outbound := &RequestEvent{
			ID:        fmt.Sprintf("outbound-%d", i),
			Type:      EventTypeDatabase,
			ThreadKey: threadKey,
			Timestamp: time.Now(),
		}
		correlator.RecordOutboundRequest(outbound)

		correlator.RecordInboundComplete(fmt.Sprintf("inbound-%d", i), time.Now())
	}
}

func BenchmarkHeaderlessCorrelator_TimingCorrelation(b *testing.B) {
	config := DefaultHeaderlessConfig()
	correlator := NewHeaderlessCorrelator(config)

	// Pre-populate with some inbound requests
	for i := 0; i < 100; i++ {
		inbound := &RequestEvent{
			ID:        fmt.Sprintf("inbound-%d", i),
			Type:      EventTypeHTTP,
			ThreadKey: ThreadKey{PID: 1000, TID: uint32(2000 + i)},
			Timestamp: time.Now(),
		}
		correlator.RecordInboundRequest(inbound)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outbound := &RequestEvent{
			ID:        fmt.Sprintf("outbound-%d", i),
			Type:      EventTypeDatabase,
			ThreadKey: ThreadKey{PID: 1000, TID: uint32(3000 + i%50)}, // Different thread
			Timestamp: time.Now(),
		}
		correlator.RecordOutboundRequest(outbound)
		correlator.RecordOutboundComplete(fmt.Sprintf("outbound-%d", i), time.Now())
	}
}
