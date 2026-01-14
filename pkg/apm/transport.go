package apm

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Transport sends data to the dogwatch server
type Transport struct {
	config *Config
	client *http.Client
}

// NewTransport creates a new transport
func NewTransport(config *Config) (*Transport, error) {
	return &Transport{
		config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}, nil
}

// TracePayload represents the trace data sent to server
type TracePayload struct {
	Service     string  `json:"service"`
	Environment string  `json:"env,omitempty"`
	Version     string  `json:"version,omitempty"`
	Spans       []*Span `json:"spans"`
}

// SendSpans sends spans to the server
func (t *Transport) SendSpans(spans []*Span) error {
	if len(spans) == 0 {
		return nil
	}

	// Convert to OTLP-compatible format
	payload := t.convertToOTLP(spans)

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal spans: %w", err)
	}

	// Compress if large
	var reqBody io.Reader = bytes.NewReader(body)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if len(body) > 1024 {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(body); err == nil {
			gz.Close()
			reqBody = &buf
			headers["Content-Encoding"] = "gzip"
		}
	}

	req, err := http.NewRequest("POST", t.config.AgentEndpoint+"/v1/traces", reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// OTLPTrace represents OTLP trace format
type OTLPTrace struct {
	ResourceSpans []ResourceSpan `json:"resourceSpans"`
}

// ResourceSpan groups spans by resource
type ResourceSpan struct {
	Resource   Resource    `json:"resource"`
	ScopeSpans []ScopeSpan `json:"scopeSpans"`
}

// Resource describes the entity producing telemetry
type Resource struct {
	Attributes []Attribute `json:"attributes"`
}

// ScopeSpan groups spans by instrumentation scope
type ScopeSpan struct {
	Scope InstrumentationScope `json:"scope"`
	Spans []OTLPSpan           `json:"spans"`
}

// InstrumentationScope describes the instrumentation library
type InstrumentationScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// OTLPSpan represents a span in OTLP format
type OTLPSpan struct {
	TraceID           string      `json:"traceId"`
	SpanID            string      `json:"spanId"`
	ParentSpanID      string      `json:"parentSpanId,omitempty"`
	Name              string      `json:"name"`
	Kind              int         `json:"kind"`
	StartTimeUnixNano int64       `json:"startTimeUnixNano"`
	EndTimeUnixNano   int64       `json:"endTimeUnixNano"`
	Attributes        []Attribute `json:"attributes,omitempty"`
	Status            SpanStatus  `json:"status"`
}

// Attribute is a key-value pair
type Attribute struct {
	Key   string         `json:"key"`
	Value AttributeValue `json:"value"`
}

// AttributeValue holds the attribute value
type AttributeValue struct {
	StringValue string `json:"stringValue,omitempty"`
	IntValue    int64  `json:"intValue,omitempty"`
	DoubleValue float64 `json:"doubleValue,omitempty"`
	BoolValue   bool   `json:"boolValue,omitempty"`
}

// SpanStatus represents the status of a span
type SpanStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// convertToOTLP converts spans to OTLP format
func (t *Transport) convertToOTLP(spans []*Span) *OTLPTrace {
	// Build resource attributes
	resourceAttrs := []Attribute{
		{Key: "service.name", Value: AttributeValue{StringValue: t.config.ServiceName}},
	}
	if t.config.ServiceVersion != "" {
		resourceAttrs = append(resourceAttrs, Attribute{Key: "service.version", Value: AttributeValue{StringValue: t.config.ServiceVersion}})
	}
	if t.config.Environment != "" {
		resourceAttrs = append(resourceAttrs, Attribute{Key: "deployment.environment", Value: AttributeValue{StringValue: t.config.Environment}})
	}

	// Convert spans
	otlpSpans := make([]OTLPSpan, 0, len(spans))
	for _, span := range spans {
		// Convert tags to attributes
		attrs := make([]Attribute, 0, len(span.Tags))
		for k, v := range span.Tags {
			attrs = append(attrs, Attribute{Key: k, Value: AttributeValue{StringValue: v}})
		}

		// Determine span kind
		kind := 0 // INTERNAL
		if k, ok := span.Tags[TagSpanKind]; ok {
			switch k {
			case SpanKindServer:
				kind = 2
			case SpanKindClient:
				kind = 3
			case SpanKindProducer:
				kind = 4
			case SpanKindConsumer:
				kind = 5
			}
		}

		// Status
		status := SpanStatus{Code: 1} // OK
		if span.Error {
			status.Code = 2 // ERROR
			if msg, ok := span.Tags["error.message"]; ok {
				status.Message = msg
			}
		}

		otlpSpan := OTLPSpan{
			TraceID:           span.TraceID,
			SpanID:            span.SpanID,
			ParentSpanID:      span.ParentID,
			Name:              span.OperationName,
			Kind:              kind,
			StartTimeUnixNano: span.StartTime.UnixNano(),
			EndTimeUnixNano:   span.StartTime.Add(span.Duration).UnixNano(),
			Attributes:        attrs,
			Status:            status,
		}
		otlpSpans = append(otlpSpans, otlpSpan)
	}

	return &OTLPTrace{
		ResourceSpans: []ResourceSpan{
			{
				Resource: Resource{Attributes: resourceAttrs},
				ScopeSpans: []ScopeSpan{
					{
						Scope: InstrumentationScope{
							Name:    "dogwatch/apm",
							Version: "1.0.0",
						},
						Spans: otlpSpans,
					},
				},
			},
		},
	}
}

// MetricsPayload represents metrics data sent to server
type MetricsPayload struct {
	Service string    `json:"service"`
	Metrics []*Metric `json:"metrics"`
}

// SendMetrics sends metrics to the server
func (t *Transport) SendMetrics(metrics []*Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	payload := MetricsPayload{
		Service: t.config.ServiceName,
		Metrics: metrics,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics: %w", err)
	}

	req, err := http.NewRequest("POST", t.config.AgentEndpoint+"/api/metrics/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
