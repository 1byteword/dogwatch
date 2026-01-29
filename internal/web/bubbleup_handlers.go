package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/bubbleup"
)

// BubbleUpHandlers provides HTTP handlers for BubbleUp analysis
type BubbleUpHandlers struct {
	analyzer *bubbleup.Analyzer
}

// NewBubbleUpHandlers creates BubbleUp handlers
func NewBubbleUpHandlers(analyzer *bubbleup.Analyzer) *BubbleUpHandlers {
	return &BubbleUpHandlers{analyzer: analyzer}
}

// RegisterRoutes registers BubbleUp routes
func (h *BubbleUpHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/bubbleup/analyze", h.handleAnalyze)
	mux.HandleFunc("/api/bubbleup/results", h.handleListResults)
	mux.HandleFunc("/api/bubbleup/", h.handleGetResult)
}

// analyzeRequest is the JSON request body for analysis
type analyzeRequest struct {
	Service            string    `json:"service"`
	TimeStart          time.Time `json:"time_start"`
	TimeEnd            time.Time `json:"time_end"`
	Mode               string    `json:"mode"`
	LatencyPercentile  float64   `json:"latency_percentile"`
	LatencyThresholdMs float64   `json:"latency_threshold_ms"`
	AnomalousSpanIDs   []string  `json:"anomalous_span_ids"`
	BaselineSpanIDs    []string  `json:"baseline_span_ids"`
}

// handleAnalyze runs BubbleUp analysis
func (h *BubbleUpHandlers) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Build analysis request
	analysisReq := bubbleup.AnalysisRequest{
		Service:           req.Service,
		TimeStart:         req.TimeStart,
		TimeEnd:           req.TimeEnd,
		Mode:              req.Mode,
		LatencyPercentile: req.LatencyPercentile,
		LatencyThreshold:  req.LatencyThresholdMs,
		AnomalousSpanIDs:  req.AnomalousSpanIDs,
		BaselineSpanIDs:   req.BaselineSpanIDs,
	}

	// Default time range to last hour
	if analysisReq.TimeStart.IsZero() {
		analysisReq.TimeEnd = time.Now().UTC()
		analysisReq.TimeStart = analysisReq.TimeEnd.Add(-1 * time.Hour)
	}
	if analysisReq.TimeEnd.IsZero() {
		analysisReq.TimeEnd = time.Now().UTC()
	}

	result, err := h.analyzer.Analyze(r.Context(), analysisReq)
	if err != nil {
		http.Error(w, "Analysis failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleListResults returns recent analysis results
func (h *BubbleUpHandlers) handleListResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	results := h.analyzer.ListResults(50)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleGetResult returns a specific analysis result
func (h *BubbleUpHandlers) handleGetResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/bubbleup/")
	if id == "" {
		http.Error(w, "Analysis ID required", http.StatusBadRequest)
		return
	}

	result, ok := h.analyzer.GetResult(id)
	if !ok {
		http.Error(w, "Analysis not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
