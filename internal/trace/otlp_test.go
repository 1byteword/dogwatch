package trace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestHandleTraces_ValidOTLP(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	payload := `{
		"resourceSpans": [{
			"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "test-svc"}}]},
			"scopeSpans": [{
				"scope": {"name": "test"},
				"spans": [{
					"traceId": "0123456789abcdef0123456789abcdef",
					"spanId": "0123456789abcdef",
					"name": "GET /api",
					"kind": 2,
					"startTimeUnixNano": "1700000000000000000",
					"endTimeUnixNano": "1700000000100000000",
					"attributes": [{"key": "http.method", "value": {"stringValue": "GET"}}],
					"status": {"code": 1}
				}]
			}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	receiver.HandleTraces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}

	ps, ok := resp["partialSuccess"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing partialSuccess")
	}
	if rejected := ps["rejectedSpans"]; rejected != float64(0) {
		t.Errorf("rejectedSpans = %v, want 0", rejected)
	}

	// Verify span was stored
	detail, err := store.GetTrace("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if len(detail.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(detail.Spans))
	}
	if detail.Spans[0].Name != "GET /api" {
		t.Errorf("span name = %q, want %q", detail.Spans[0].Name, "GET /api")
	}
	if detail.Spans[0].ServiceName != "test-svc" {
		t.Errorf("service = %q, want %q", detail.Spans[0].ServiceName, "test-svc")
	}
	if detail.Spans[0].Attributes["http.method"] != "GET" {
		t.Errorf("http.method attr = %q, want %q", detail.Spans[0].Attributes["http.method"], "GET")
	}
}

func TestHandleTraces_InvalidJSON(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(`{not valid json`))
	w := httptest.NewRecorder()

	receiver.HandleTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTraces_MethodNotAllowed(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	req := httptest.NewRequest(http.MethodGet, "/v1/traces", nil)
	w := httptest.NewRecorder()

	receiver.HandleTraces(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleTraces_SpanCallback(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	var mu sync.Mutex
	var received []Span
	receiver.SetSpanCallback(func(s Span) {
		mu.Lock()
		received = append(received, s)
		mu.Unlock()
	})

	payload := `{
		"resourceSpans": [{
			"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "cb-svc"}}]},
			"scopeSpans": [{
				"scope": {"name": "test"},
				"spans": [{
					"traceId": "aaaa456789abcdef0123456789abcdef",
					"spanId": "aaaa456789abcdef",
					"name": "POST /data",
					"kind": 2,
					"startTimeUnixNano": "1700000000000000000",
					"endTimeUnixNano": "1700000000200000000",
					"attributes": [],
					"status": {"code": 1}
				}]
			}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	w := httptest.NewRecorder()

	receiver.HandleTraces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("callback called %d times, want 1", len(received))
	}
	if received[0].Name != "POST /data" {
		t.Errorf("callback span name = %q, want %q", received[0].Name, "POST /data")
	}
	if received[0].ServiceName != "cb-svc" {
		t.Errorf("callback service = %q, want %q", received[0].ServiceName, "cb-svc")
	}
}

func TestHandleTraces_MultipleSpans(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	traceID := "bbbb456789abcdef0123456789abcdef"
	payload := fmt.Sprintf(`{
		"resourceSpans": [{
			"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "multi-svc"}}]},
			"scopeSpans": [{
				"scope": {"name": "test"},
				"spans": [
					{
						"traceId": %q,
						"spanId": "span000000000001",
						"name": "span-a",
						"kind": 2,
						"startTimeUnixNano": "1700000000000000000",
						"endTimeUnixNano": "1700000000100000000",
						"attributes": [],
						"status": {"code": 1}
					},
					{
						"traceId": %q,
						"spanId": "span000000000002",
						"parentSpanId": "span000000000001",
						"name": "span-b",
						"kind": 3,
						"startTimeUnixNano": "1700000000010000000",
						"endTimeUnixNano": "1700000000050000000",
						"attributes": [],
						"status": {"code": 1}
					},
					{
						"traceId": %q,
						"spanId": "span000000000003",
						"parentSpanId": "span000000000001",
						"name": "span-c",
						"kind": 1,
						"startTimeUnixNano": "1700000000050000000",
						"endTimeUnixNano": "1700000000090000000",
						"attributes": [],
						"status": {"code": 0}
					}
				]
			}]
		}]
	}`, traceID, traceID, traceID)

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(payload))
	w := httptest.NewRecorder()

	receiver.HandleTraces(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	detail, err := store.GetTrace(traceID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if len(detail.Spans) != 3 {
		t.Errorf("expected 3 spans, got %d", len(detail.Spans))
	}
	if detail.SpanCount != 3 {
		t.Errorf("trace span count = %d, want 3", detail.SpanCount)
	}
}

func TestHandleSimpleTrace_Valid(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	payload := `{
		"trace_id": "abc123",
		"span_id": "span1",
		"service": "my-svc",
		"operation": "GET /users",
		"start_time_ms": 1700000000000,
		"duration_ms": 50.5,
		"status": "ok",
		"tags": {"env": "prod"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/trace", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	receiver.HandleSimpleTrace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("response status = %q, want %q", resp["status"], "ok")
	}

	detail, err := store.GetTrace("abc123")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if len(detail.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(detail.Spans))
	}
	sp := detail.Spans[0]
	if sp.Name != "GET /users" {
		t.Errorf("span name = %q, want %q", sp.Name, "GET /users")
	}
	if sp.ServiceName != "my-svc" {
		t.Errorf("service = %q, want %q", sp.ServiceName, "my-svc")
	}
	if sp.DurationMs != 50.5 {
		t.Errorf("duration = %f, want 50.5", sp.DurationMs)
	}
	if sp.Status != "OK" {
		t.Errorf("status = %q, want %q", sp.Status, "OK")
	}
	if sp.Attributes["env"] != "prod" {
		t.Errorf("env tag = %q, want %q", sp.Attributes["env"], "prod")
	}
}

func TestHandleSimpleTrace_Error(t *testing.T) {
	store := setupTestTraceStore(t)
	defer store.Close()

	receiver := NewOTLPReceiver(store)

	payload := `{
		"trace_id": "err-trace",
		"span_id": "span-err",
		"service": "error-svc",
		"operation": "POST /fail",
		"start_time_ms": 1700000000000,
		"duration_ms": 10,
		"status": "error"
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/trace", strings.NewReader(payload))
	w := httptest.NewRecorder()

	receiver.HandleSimpleTrace(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	detail, err := store.GetTrace("err-trace")
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if len(detail.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(detail.Spans))
	}
	if detail.Spans[0].Status != "ERROR" {
		t.Errorf("status = %q, want %q", detail.Spans[0].Status, "ERROR")
	}
}

func TestConvertSpan(t *testing.T) {
	tests := []struct {
		name       string
		s          span
		svc        string
		wantKind   string
		wantStatus string
	}{
		{
			name:       "kind 0 unspecified, status 0 unset",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 0, Status: spanStatus{Code: 0}},
			svc:        "svc",
			wantKind:   "UNSPECIFIED",
			wantStatus: "UNSET",
		},
		{
			name:       "kind 1 internal, status 1 ok",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 1, Status: spanStatus{Code: 1}},
			svc:        "svc",
			wantKind:   "INTERNAL",
			wantStatus: "OK",
		},
		{
			name:       "kind 2 server, status 2 error",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 2, Status: spanStatus{Code: 2, Message: "fail"}},
			svc:        "svc",
			wantKind:   "SERVER",
			wantStatus: "ERROR",
		},
		{
			name:       "kind 3 client",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 3, Status: spanStatus{Code: 1}},
			svc:        "svc",
			wantKind:   "CLIENT",
			wantStatus: "OK",
		},
		{
			name:       "kind 4 producer",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 4, Status: spanStatus{Code: 0}},
			svc:        "svc",
			wantKind:   "PRODUCER",
			wantStatus: "UNSET",
		},
		{
			name:       "kind 5 consumer",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 5, Status: spanStatus{Code: 1}},
			svc:        "svc",
			wantKind:   "CONSUMER",
			wantStatus: "OK",
		},
		{
			name:       "unknown kind defaults to UNSPECIFIED",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 99, Status: spanStatus{Code: 0}},
			svc:        "svc",
			wantKind:   "UNSPECIFIED",
			wantStatus: "UNSET",
		},
		{
			name:       "unknown status defaults to UNSET",
			s:          span{TraceID: "aabb", SpanID: "cc", Name: "op", Kind: 2, Status: spanStatus{Code: 99}},
			svc:        "svc",
			wantKind:   "SERVER",
			wantStatus: "UNSET",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convertSpan(tc.s, tc.svc)
			if got.Kind != tc.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.ServiceName != tc.svc {
				t.Errorf("service = %q, want %q", got.ServiceName, tc.svc)
			}
		})
	}

	// Test timestamps and duration
	t.Run("computes duration from nano timestamps", func(t *testing.T) {
		s := span{
			TraceID:           "aabb",
			SpanID:            "cc",
			Name:              "op",
			Kind:              2,
			StartTimeUnixNano: "1700000000000000000",
			EndTimeUnixNano:   "1700000000100000000",
			Status:            spanStatus{Code: 1},
		}
		got := convertSpan(s, "svc")
		if got.DurationMs != 100.0 {
			t.Errorf("duration = %f ms, want 100.0", got.DurationMs)
		}
	})

	// Test attribute extraction
	t.Run("extracts string and int attributes", func(t *testing.T) {
		s := span{
			TraceID: "aabb",
			SpanID:  "cc",
			Name:    "op",
			Attributes: []attribute{
				{Key: "http.method", Value: attributeValue{StringValue: "GET"}},
				{Key: "http.status", Value: attributeValue{IntValue: "200"}},
			},
			Status: spanStatus{Code: 1},
		}
		got := convertSpan(s, "svc")
		if got.Attributes["http.method"] != "GET" {
			t.Errorf("http.method = %q, want GET", got.Attributes["http.method"])
		}
		if got.Attributes["http.status"] != "200" {
			t.Errorf("http.status = %q, want 200", got.Attributes["http.status"])
		}
	})

	// Test status message
	t.Run("preserves status message", func(t *testing.T) {
		s := span{
			TraceID: "aabb",
			SpanID:  "cc",
			Name:    "op",
			Status:  spanStatus{Code: 2, Message: "connection refused"},
		}
		got := convertSpan(s, "svc")
		if got.StatusMsg != "connection refused" {
			t.Errorf("statusMsg = %q, want %q", got.StatusMsg, "connection refused")
		}
	})
}

func TestExtractServiceName(t *testing.T) {
	t.Run("finds service.name attribute", func(t *testing.T) {
		attrs := []attribute{
			{Key: "deployment.env", Value: attributeValue{StringValue: "prod"}},
			{Key: "service.name", Value: attributeValue{StringValue: "my-service"}},
			{Key: "host.name", Value: attributeValue{StringValue: "host1"}},
		}
		got := extractServiceName(attrs)
		if got != "my-service" {
			t.Errorf("extractServiceName = %q, want %q", got, "my-service")
		}
	})

	t.Run("returns unknown when service.name missing", func(t *testing.T) {
		attrs := []attribute{
			{Key: "deployment.env", Value: attributeValue{StringValue: "prod"}},
		}
		got := extractServiceName(attrs)
		if got != "unknown" {
			t.Errorf("extractServiceName = %q, want %q", got, "unknown")
		}
	})

	t.Run("returns unknown for empty attributes", func(t *testing.T) {
		got := extractServiceName(nil)
		if got != "unknown" {
			t.Errorf("extractServiceName = %q, want %q", got, "unknown")
		}
	})
}

func TestParseNanoTime(t *testing.T) {
	t.Run("valid nanosecond timestamp", func(t *testing.T) {
		got := parseNanoTime("1700000000000000000")
		if got != 1700000000000000000 {
			t.Errorf("parseNanoTime = %d, want 1700000000000000000", got)
		}
	})

	t.Run("empty string returns zero", func(t *testing.T) {
		got := parseNanoTime("")
		if got != 0 {
			t.Errorf("parseNanoTime(\"\") = %d, want 0", got)
		}
	})

	t.Run("non-numeric returns zero", func(t *testing.T) {
		got := parseNanoTime("not-a-number")
		if got != 0 {
			t.Errorf("parseNanoTime(\"not-a-number\") = %d, want 0", got)
		}
	})
}
