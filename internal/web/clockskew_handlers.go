package web

import (
	"encoding/json"
	"net/http"
	"time"

	"dogwatch/internal/trace"
)

// ClockSkewHandlers handles HTTP requests for clock skew management
type ClockSkewHandlers struct {
	manager *trace.ClockSkewManager
}

// NewClockSkewHandlers creates new clock skew handlers
func NewClockSkewHandlers(manager *trace.ClockSkewManager) *ClockSkewHandlers {
	return &ClockSkewHandlers{manager: manager}
}

// RegisterRoutes registers clock skew API routes
func (h *ClockSkewHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/clockskew/stats", h.handleStats)
	mux.HandleFunc("/api/clockskew/config", h.handleConfig)
	mux.HandleFunc("/api/clockskew/report", h.handleReport)
	mux.HandleFunc("/api/clockskew/pairs", h.handlePairs)
	mux.HandleFunc("/api/clockskew/corrections", h.handleCorrections)
	mux.HandleFunc("/api/clockskew/ntp", h.handleNTP)
}

// handleStats returns clock skew statistics
func (h *ClockSkewHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, h.manager.GetStats())
}

// handleConfig handles clock skew configuration
func (h *ClockSkewHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, h.manager.GetConfig())

	case http.MethodPut:
		var config trace.ClockSkewConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		h.manager.UpdateConfig(config)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Configuration updated",
			"config":  h.manager.GetConfig(),
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleReport generates a comprehensive skew report
func (h *ClockSkewHandlers) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	report := h.manager.GenerateReport()
	writeJSON(w, report)
}

// handlePairs returns skew statistics for service pairs
func (h *ClockSkewHandlers) handlePairs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		source := r.URL.Query().Get("source")
		target := r.URL.Query().Get("target")

		if source != "" && target != "" {
			// Get specific pair
			stats := h.manager.GetSkewStats(source, target)
			if stats == nil {
				http.Error(w, "Service pair not found", http.StatusNotFound)
				return
			}
			writeJSON(w, stats)
		} else {
			// Get all pairs
			writeJSON(w, h.manager.GetAllSkewStats())
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCorrections manages manual corrections
func (h *ClockSkewHandlers) handleCorrections(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get all corrections
		writeJSON(w, h.manager.GetServiceCorrections())

	case http.MethodPost:
		// Set a correction
		var req struct {
			Service    string `json:"service"`
			CorrectionMs int64 `json:"correction_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Service == "" {
			http.Error(w, "Service name required", http.StatusBadRequest)
			return
		}

		correction := time.Duration(req.CorrectionMs) * time.Millisecond
		h.manager.SetCorrection(req.Service, correction)

		writeJSON(w, map[string]interface{}{
			"success":    true,
			"message":    "Correction set",
			"service":    req.Service,
			"correction": correction.String(),
		})

	case http.MethodDelete:
		// Clear a correction
		service := r.URL.Query().Get("service")
		if service == "" {
			http.Error(w, "Service name required", http.StatusBadRequest)
			return
		}

		h.manager.ClearCorrection(service)
		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Correction cleared",
			"service": service,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleNTP handles NTP drift reporting
func (h *ClockSkewHandlers) handleNTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get NTP drift stats for a service
		service := r.URL.Query().Get("service")
		if service == "" {
			http.Error(w, "Service name required", http.StatusBadRequest)
			return
		}

		stats := h.manager.GetNTPDriftStats(service)
		if stats == nil {
			http.Error(w, "No NTP drift data for service", http.StatusNotFound)
			return
		}
		writeJSON(w, stats)

	case http.MethodPost:
		// Record NTP drift measurement
		var req struct {
			Service string `json:"service"`
			DriftMs int64  `json:"drift_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Service == "" {
			http.Error(w, "Service name required", http.StatusBadRequest)
			return
		}

		drift := time.Duration(req.DriftMs) * time.Millisecond
		h.manager.RecordNTPDrift(req.Service, drift)

		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "NTP drift recorded",
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Package-level handler registration
var clockSkewHandlers *ClockSkewHandlers

// SetClockSkewManager sets the clock skew manager for handlers
func SetClockSkewManager(manager *trace.ClockSkewManager) {
	clockSkewHandlers = NewClockSkewHandlers(manager)
}

// RegisterClockSkewRoutes registers clock skew routes on a mux
func RegisterClockSkewRoutes(mux *http.ServeMux) {
	if clockSkewHandlers != nil {
		clockSkewHandlers.RegisterRoutes(mux)
	}
}
