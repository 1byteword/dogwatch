package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/siem"
)

// Package-level variables for SIEM handlers
var (
	siemStore   *siem.Store
	siemManager *siem.Manager
)

// SetSIEMManager sets the SIEM manager and store for the handlers
func SetSIEMManager(manager *siem.Manager, store *siem.Store) {
	siemManager = manager
	siemStore = store
}

// RegisterSIEMRoutes registers SIEM API routes at the package level
func RegisterSIEMRoutes(mux *http.ServeMux) {
	if siemStore == nil {
		return
	}
	handlers := NewSIEMHandlers(siemStore, siemManager)
	handlers.RegisterSIEMRoutes(mux)
}

// SIEMHandlers handles SIEM-related HTTP requests
type SIEMHandlers struct {
	store   *siem.Store
	manager *siem.Manager
}

// NewSIEMHandlers creates new SIEM handlers
func NewSIEMHandlers(store *siem.Store, manager *siem.Manager) *SIEMHandlers {
	return &SIEMHandlers{
		store:   store,
		manager: manager,
	}
}

// RegisterSIEMRoutes registers SIEM routes
func (h *SIEMHandlers) RegisterSIEMRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/siem/configs", h.ListConfigs)
	mux.HandleFunc("POST /api/siem/configs", h.CreateConfig)
	mux.HandleFunc("GET /api/siem/configs/{id}", h.GetConfig)
	mux.HandleFunc("PUT /api/siem/configs/{id}", h.UpdateConfig)
	mux.HandleFunc("DELETE /api/siem/configs/{id}", h.DeleteConfig)
	mux.HandleFunc("POST /api/siem/configs/{id}/test", h.TestConfig)
	mux.HandleFunc("POST /api/siem/configs/{id}/export", h.ManualExport)
	mux.HandleFunc("GET /api/siem/configs/{id}/stats", h.GetConfigStats)
	mux.HandleFunc("GET /api/siem/configs/{id}/history", h.GetExportHistory)
	mux.HandleFunc("GET /api/siem/stats", h.GetAllStats)
	mux.HandleFunc("GET /api/siem/formats", h.ListFormats)
}

// ListConfigs lists all SIEM configurations
func (h *SIEMHandlers) ListConfigs(w http.ResponseWriter, r *http.Request) {
	configs, err := h.store.ListConfigs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if configs == nil {
		configs = []siem.Config{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(configs)
}

// CreateConfig creates a new SIEM configuration
func (h *SIEMHandlers) CreateConfig(w http.ResponseWriter, r *http.Request) {
	var config siem.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := config.Validate(); err != nil {
		http.Error(w, "validation error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.SaveConfig(config); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Start worker if enabled
	if config.Enabled && h.manager != nil {
		h.manager.StartWorker(config)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(config)
}

// GetConfig retrieves a SIEM configuration
func (h *SIEMHandlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	config, err := h.store.GetConfig(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdateConfig updates a SIEM configuration
func (h *SIEMHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var config siem.Config
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	config.ID = id
	if err := config.Validate(); err != nil {
		http.Error(w, "validation error: "+err.Error(), http.StatusBadRequest)
		return
	}

	if h.manager != nil {
		if err := h.manager.UpdateConfig(config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.store.SaveConfig(config); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// DeleteConfig deletes a SIEM configuration
func (h *SIEMHandlers) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	// Stop worker first
	if h.manager != nil {
		h.manager.StopWorker(id)
	}

	if err := h.store.DeleteConfig(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestConfig tests a SIEM configuration
func (h *SIEMHandlers) TestConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	config, err := h.store.GetConfig(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Test the exporter
	if err := siem.TestExporter(*config); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Test event sent successfully",
	})
}

// ManualExport triggers a manual export
func (h *SIEMHandlers) ManualExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	// Parse time range from query params
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	var startTime, endTime time.Time
	var err error

	if startStr != "" {
		startTime, err = time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, "invalid start time: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		startTime = time.Now().Add(-24 * time.Hour)
	}

	if endStr != "" {
		endTime, err = time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, "invalid end time: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		endTime = time.Now()
	}

	if h.manager == nil {
		http.Error(w, "SIEM manager not initialized", http.StatusServiceUnavailable)
		return
	}

	count, err := h.manager.ManualExport(id, startTime, endTime)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"events_queued":  count,
		"time_range": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
	})
}

// GetConfigStats returns stats for a specific config
func (h *SIEMHandlers) GetConfigStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	// Get real-time stats from manager
	var realtimeStats *siem.ExportStats
	if h.manager != nil {
		realtimeStats = h.manager.GetConfigStats(id)
	}

	// Get historical stats from store
	since := time.Now().Add(-24 * time.Hour)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = t
		}
	}

	historicalStats, err := h.store.GetExportStats(id, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"historical": historicalStats,
	}
	if realtimeStats != nil {
		response["realtime"] = realtimeStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetExportHistory returns export history for a config
func (h *SIEMHandlers) GetExportHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	history, err := h.store.GetExportHistory(id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if history == nil {
		history = []siem.ExportHistoryEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// GetAllStats returns stats for all configurations
func (h *SIEMHandlers) GetAllStats(w http.ResponseWriter, r *http.Request) {
	stats := make(map[string]interface{})

	if h.manager != nil {
		stats["realtime"] = h.manager.GetStats()
	}

	// Get configs for historical stats
	configs, err := h.store.ListConfigs()
	if err == nil {
		since := time.Now().Add(-24 * time.Hour)
		historical := make(map[string]*siem.ExportStatsAggregated)
		for _, config := range configs {
			if s, err := h.store.GetExportStats(config.ID, since); err == nil {
				historical[config.ID] = s
			}
		}
		stats["historical"] = historical
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ListFormats returns supported export formats
func (h *SIEMHandlers) ListFormats(w http.ResponseWriter, r *http.Request) {
	formats := []map[string]interface{}{
		{
			"id":          "cef",
			"name":        "CEF (Common Event Format)",
			"description": "ArcSight/HP standard format for SIEM integration",
			"example":     "CEF:0|Dogwatch|Dogwatch|1.0|rule-id|Alert Name|8|src=192.168.1.1 dst=10.0.0.1",
		},
		{
			"id":          "leef",
			"name":        "LEEF (Log Event Extended Format)",
			"description": "IBM QRadar standard format",
			"example":     "LEEF:2.0|Dogwatch|Dogwatch|1.0|rule-id|sev=8\tsrc=192.168.1.1",
		},
		{
			"id":          "json",
			"name":        "JSON",
			"description": "Structured JSON format for flexible parsing",
			"example":     `{"id":"...", "timestamp":"...", "severity":"high", ...}`,
		},
	}

	exporterTypes := []map[string]interface{}{
		{
			"id":          "syslog",
			"name":        "Syslog",
			"description": "Send events via syslog protocol (UDP/TCP/TLS)",
			"protocols":   []string{"udp", "tcp", "tls"},
		},
		{
			"id":          "file",
			"name":        "File",
			"description": "Write events to rotating log files",
		},
		{
			"id":          "http",
			"name":        "HTTP Webhook",
			"description": "Send events to HTTP/HTTPS endpoints",
			"auth_types":  []string{"none", "basic", "bearer", "api_key"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"formats":        formats,
		"exporter_types": exporterTypes,
	})
}

