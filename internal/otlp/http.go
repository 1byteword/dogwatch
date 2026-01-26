package otlp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

const (
	contentTypeProtobuf = "application/x-protobuf"
	contentTypeJSON     = "application/json"
)

// handleHTTPTraces handles POST /v1/traces
func (s *Server) handleHTTPTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req coltracepb.ExportTraceServiceRequest
	if err := unmarshalRequest(r, body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := processTraces(s.traceStore, req.ResourceSpans); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, r, &coltracepb.ExportTraceServiceResponse{})
}

// handleHTTPMetrics handles POST /v1/metrics
func (s *Server) handleHTTPMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req colmetricspb.ExportMetricsServiceRequest
	if err := unmarshalRequest(r, body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := processMetrics(s.metricsStore, req.ResourceMetrics); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, r, &colmetricspb.ExportMetricsServiceResponse{})
}

// handleHTTPLogs handles POST /v1/logs
func (s *Server) handleHTTPLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req collogspb.ExportLogsServiceRequest
	if err := unmarshalRequest(r, body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := processLogs(s.logStore, req.ResourceLogs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, r, &collogspb.ExportLogsServiceResponse{})
}

// unmarshalRequest detects content type and unmarshals accordingly
func unmarshalRequest(r *http.Request, body []byte, msg proto.Message) error {
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "protobuf") || strings.Contains(contentType, "x-protobuf") {
		return proto.Unmarshal(body, msg)
	}

	// Default to JSON
	return json.Unmarshal(body, msg)
}

// writeResponse writes response in the same format as request
func writeResponse(w http.ResponseWriter, r *http.Request, msg proto.Message) {
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "protobuf") || strings.Contains(contentType, "x-protobuf") {
		w.Header().Set("Content-Type", contentTypeProtobuf)
		data, _ := proto.Marshal(msg)
		w.Write(data)
		return
	}

	// Default to JSON
	w.Header().Set("Content-Type", contentTypeJSON)
	json.NewEncoder(w).Encode(msg)
}
