package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/runtime"
)

// MemoryHandlers handles HTTP requests for memory management
type MemoryHandlers struct {
	manager *runtime.MemoryManager
}

// NewMemoryHandlers creates new memory handlers
func NewMemoryHandlers(manager *runtime.MemoryManager) *MemoryHandlers {
	return &MemoryHandlers{manager: manager}
}

// RegisterRoutes registers memory management API routes
func (h *MemoryHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/memory/metrics", h.handleMetrics)
	mux.HandleFunc("/api/memory/config", h.handleConfig)
	mux.HandleFunc("/api/memory/stats", h.handleStats)
	mux.HandleFunc("/api/memory/history", h.handleHistory)
	mux.HandleFunc("/api/memory/report", h.handleReport)
	mux.HandleFunc("/api/memory/pressure", h.handlePressure)
	mux.HandleFunc("/api/memory/gc", h.handleGC)
}

// handleMetrics returns current memory metrics
func (h *MemoryHandlers) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, h.manager.GetMetrics())
}

// handleConfig handles memory configuration
func (h *MemoryHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, h.manager.GetConfig())

	case http.MethodPut:
		var config runtime.MemoryConfig
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

// handleStats returns memory manager statistics
func (h *MemoryHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, h.manager.GetStats())
}

// handleHistory returns historical memory metrics
func (h *MemoryHandlers) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse duration from query parameter
	sinceStr := r.URL.Query().Get("since")
	since := time.Hour // default

	if sinceStr != "" {
		// Try parsing as duration
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		} else {
			// Try parsing as minutes
			if mins, err := strconv.Atoi(sinceStr); err == nil {
				since = time.Duration(mins) * time.Minute
			}
		}
	}

	writeJSON(w, h.manager.GetMetricsHistory(since))
}

// handleReport generates a comprehensive memory report
func (h *MemoryHandlers) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	includeHistory := r.URL.Query().Get("history") == "true"
	report := h.manager.GenerateReport(includeHistory)
	writeJSON(w, report)
}

// handlePressure returns current memory pressure status
func (h *MemoryHandlers) handlePressure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pressure := h.manager.GetPressure()
	shouldAccept := h.manager.ShouldAccept()

	writeJSON(w, map[string]interface{}{
		"pressure":       pressure.String(),
		"accepting_load": shouldAccept,
	})
}

// handleGC handles garbage collection triggers
func (h *MemoryHandlers) handleGC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.URL.Query().Get("action")

	switch action {
	case "run", "":
		h.manager.ForceGC()
		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Garbage collection triggered",
		})

	case "free":
		h.manager.FreeOSMemory()
		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Memory released to OS",
		})

	case "full":
		h.manager.ForceGC()
		h.manager.FreeOSMemory()
		writeJSON(w, map[string]interface{}{
			"success": true,
			"message": "Full GC and memory release completed",
		})

	default:
		http.Error(w, "Unknown action: "+action, http.StatusBadRequest)
	}
}

// Package-level handler registration
var memoryHandlers *MemoryHandlers

// SetMemoryManager sets the memory manager for handlers
func SetMemoryManager(manager *runtime.MemoryManager) {
	memoryHandlers = NewMemoryHandlers(manager)
}

// RegisterMemoryRoutes registers memory routes on a mux
func RegisterMemoryRoutes(mux *http.ServeMux) {
	if memoryHandlers != nil {
		memoryHandlers.RegisterRoutes(mux)
	}
}

// MemoryMiddleware wraps handlers with memory pressure checks
func MemoryMiddleware(manager *runtime.MemoryManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if we should accept the request
			if !manager.ShouldAccept() {
				w.Header().Set("Retry-After", "5")
				http.Error(w, "Service under memory pressure, try again later", http.StatusServiceUnavailable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
