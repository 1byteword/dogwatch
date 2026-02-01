package trace

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SpanCallback is called for each span received
type SpanCallback func(span Span)

// OTLPReceiver handles OpenTelemetry trace ingestion
type OTLPReceiver struct {
	store        *Store
	spanCallback SpanCallback
}

// NewOTLPReceiver creates a new OTLP receiver
func NewOTLPReceiver(store *Store) *OTLPReceiver {
	return &OTLPReceiver{store: store}
}

// SetSpanCallback sets a callback to be invoked for each span received
func (r *OTLPReceiver) SetSpanCallback(cb SpanCallback) {
	r.spanCallback = cb
}

// OTLP JSON structures (simplified)
type otlpExportRequest struct {
	ResourceSpans []resourceSpans `json:"resourceSpans"`
}

type resourceSpans struct {
	Resource   resource     `json:"resource"`
	ScopeSpans []scopeSpans `json:"scopeSpans"`
}

type resource struct {
	Attributes []attribute `json:"attributes"`
}

type scopeSpans struct {
	Scope scope  `json:"scope"`
	Spans []span `json:"spans"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type span struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	ParentSpanID      string      `json:"parentSpanId"`
	Name              string      `json:"name"`
	Kind              int         `json:"kind"`
	StartTimeUnixNano string      `json:"startTimeUnixNano"`
	EndTimeUnixNano   string      `json:"endTimeUnixNano"`
	Attributes        []attribute `json:"attributes"`
	Status            spanStatus  `json:"status"`
}

type attribute struct {
	Key   string         `json:"key"`
	Value attributeValue `json:"value"`
}

type attributeValue struct {
	StringValue string `json:"stringValue,omitempty"`
	IntValue    string `json:"intValue,omitempty"`
	BoolValue   bool   `json:"boolValue,omitempty"`
}

type spanStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HandleTraces handles POST /v1/traces
func (r *OTLPReceiver) HandleTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var otlpReq otlpExportRequest
	if err := json.Unmarshal(body, &otlpReq); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	spanCount := 0
	for _, rs := range otlpReq.ResourceSpans {
		serviceName := extractServiceName(rs.Resource.Attributes)

		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				span := convertSpan(s, serviceName)
				if err := r.store.RecordSpan(span); err != nil {
					// Log but continue
					continue
				}
				spanCount++

				// Call the span callback for entity synthesis
				if r.spanCallback != nil {
					r.spanCallback(span)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"partialSuccess": map[string]int{
			"rejectedSpans": 0,
		},
	})
}

func extractServiceName(attrs []attribute) string {
	for _, a := range attrs {
		if a.Key == "service.name" {
			return a.Value.StringValue
		}
	}
	return "unknown"
}

func convertSpan(s span, serviceName string) Span {
	// Parse timestamps (nanoseconds since epoch)
	startNano := parseNanoTime(s.StartTimeUnixNano)
	endNano := parseNanoTime(s.EndTimeUnixNano)

	startTime := time.Unix(0, startNano)
	endTime := time.Unix(0, endNano)
	durationMs := float64(endNano-startNano) / 1e6

	// Convert trace/span IDs from hex
	traceID := s.TraceID
	spanID := s.SpanID
	parentSpanID := s.ParentSpanID

	// If IDs are base64, decode them
	if len(s.TraceID) == 32 {
		// Already hex
	} else if decoded, err := hex.DecodeString(s.TraceID); err == nil {
		traceID = hex.EncodeToString(decoded)
	}

	// Convert kind
	kindMap := map[int]string{
		0: "UNSPECIFIED",
		1: "INTERNAL",
		2: "SERVER",
		3: "CLIENT",
		4: "PRODUCER",
		5: "CONSUMER",
	}
	kind := kindMap[s.Kind]
	if kind == "" {
		kind = "UNSPECIFIED"
	}

	// Convert status
	statusMap := map[int]string{
		0: "UNSET",
		1: "OK",
		2: "ERROR",
	}
	status := statusMap[s.Status.Code]
	if status == "" {
		status = "UNSET"
	}

	// Extract attributes
	attrs := make(map[string]string)
	for _, a := range s.Attributes {
		if a.Value.StringValue != "" {
			attrs[a.Key] = a.Value.StringValue
		} else if a.Value.IntValue != "" {
			attrs[a.Key] = a.Value.IntValue
		}
	}

	return Span{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Name:         s.Name,
		ServiceName:  serviceName,
		Kind:         kind,
		StartTime:    startTime,
		EndTime:      endTime,
		DurationMs:   durationMs,
		Status:       status,
		StatusMsg:    s.Status.Message,
		Attributes:   attrs,
	}
}

func parseNanoTime(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// HandleSimpleTrace handles a simpler trace format for easy integration
// POST /v1/trace with JSON body
type SimpleTraceRequest struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Service      string            `json:"service"`
	Operation    string            `json:"operation"`
	StartTime    int64             `json:"start_time_ms"`
	Duration     float64           `json:"duration_ms"`
	Status       string            `json:"status,omitempty"` // ok, error
	Tags         map[string]string `json:"tags,omitempty"`
}

func (r *OTLPReceiver) HandleSimpleTrace(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var simple SimpleTraceRequest
	if err := json.NewDecoder(req.Body).Decode(&simple); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	startTime := time.UnixMilli(simple.StartTime)
	endTime := startTime.Add(time.Duration(simple.Duration * float64(time.Millisecond)))

	status := "OK"
	if simple.Status == "error" {
		status = "ERROR"
	}

	span := Span{
		TraceID:      simple.TraceID,
		SpanID:       simple.SpanID,
		ParentSpanID: simple.ParentSpanID,
		Name:         simple.Operation,
		ServiceName:  simple.Service,
		Kind:         "SERVER",
		StartTime:    startTime,
		EndTime:      endTime,
		DurationMs:   simple.Duration,
		Status:       status,
		Attributes:   simple.Tags,
	}

	if err := r.store.RecordSpan(span); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Call the span callback for entity synthesis
	if r.spanCallback != nil {
		r.spanCallback(span)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
