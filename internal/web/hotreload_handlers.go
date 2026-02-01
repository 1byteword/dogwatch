package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"dogwatch/internal/probe"
)

// HotReloadHandlers handles HTTP requests for probe hot reload
type HotReloadHandlers struct {
	manager *probe.HotReloadManager
}

// NewHotReloadHandlers creates new hot reload handlers
func NewHotReloadHandlers(manager *probe.HotReloadManager) *HotReloadHandlers {
	return &HotReloadHandlers{manager: manager}
}

// RegisterRoutes registers hot reload API routes
func (h *HotReloadHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/probes", h.handleListProbes)
	mux.HandleFunc("/api/probes/", h.handleProbe)
	mux.HandleFunc("/api/probes/reload", h.handleReload)
	mux.HandleFunc("/api/probes/rollback", h.handleRollback)
	mux.HandleFunc("/api/probes/health", h.handleHealth)
	mux.HandleFunc("/api/probes/stats", h.handleStats)
}

// handleListProbes returns list of all probes with their status
func (h *HotReloadHandlers) handleListProbes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	probeNames := h.manager.ListProbes()
	probes := make([]map[string]interface{}, 0, len(probeNames))

	for _, name := range probeNames {
		info := h.manager.GetProbeInfo(name)
		if info != nil {
			probes = append(probes, map[string]interface{}{
				"name":    info.Name,
				"version": info.Version,
				"health":  info.Health,
				"config":  info.Config,
			})
		}
	}

	writeJSON(w, map[string]interface{}{
		"probes": probes,
		"stats":  h.manager.Stats(),
	})
}

// handleProbe handles individual probe operations
func (h *HotReloadHandlers) handleProbe(w http.ResponseWriter, r *http.Request) {
	// Extract probe name from path: /api/probes/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/probes/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Probe name required", http.StatusBadRequest)
		return
	}

	name := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		h.getProbe(w, name)
	case http.MethodPut:
		if action == "config" {
			h.updateProbeConfig(w, r, name)
		} else {
			h.getProbe(w, name)
		}
	case http.MethodPost:
		switch action {
		case "reload":
			h.reloadProbe(w, r, name)
		case "rollback":
			h.rollbackProbe(w, name)
		default:
			http.Error(w, "Unknown action", http.StatusBadRequest)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getProbe returns detailed info about a probe
func (h *HotReloadHandlers) getProbe(w http.ResponseWriter, name string) {
	info := h.manager.GetProbeInfo(name)
	if info == nil {
		http.Error(w, "Probe not found", http.StatusNotFound)
		return
	}

	writeJSON(w, info)
}

// updateProbeConfig updates a probe's configuration
func (h *HotReloadHandlers) updateProbeConfig(w http.ResponseWriter, r *http.Request, name string) {
	var config probe.ProbeConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.manager.UpdateConfig(name, config); err != nil {
		http.Error(w, "Update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Configuration updated",
		"config":  h.manager.GetConfig(name),
	})
}

// reloadProbe reloads a specific probe
func (h *HotReloadHandlers) reloadProbe(w http.ResponseWriter, r *http.Request, name string) {
	var config *probe.ProbeConfig

	// Check if config is provided in request body
	if r.ContentLength > 0 {
		var c probe.ProbeConfig
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		config = &c
	}

	if err := h.manager.Reload(name, config); err != nil {
		http.Error(w, "Reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Probe reloaded",
		"version": h.manager.GetVersion(name),
	})
}

// rollbackProbe rolls back a probe to previous version
func (h *HotReloadHandlers) rollbackProbe(w http.ResponseWriter, name string) {
	if err := h.manager.Rollback(name); err != nil {
		http.Error(w, "Rollback failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Probe rolled back",
		"version": h.manager.GetVersion(name),
	})
}

// handleReload reloads all probes or specific probe
func (h *HotReloadHandlers) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if specific probe requested
	probeName := r.URL.Query().Get("probe")
	if probeName != "" {
		h.reloadProbe(w, r, probeName)
		return
	}

	// Reload all probes
	results := make(map[string]string)
	for _, name := range h.manager.ListProbes() {
		if err := h.manager.Reload(name, nil); err != nil {
			results[name] = "error: " + err.Error()
		} else {
			results[name] = "reloaded"
		}
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"results": results,
	})
}

// handleRollback rolls back a probe
func (h *HotReloadHandlers) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	probeName := r.URL.Query().Get("probe")
	if probeName == "" {
		http.Error(w, "Probe name required", http.StatusBadRequest)
		return
	}

	h.rollbackProbe(w, probeName)
}

// handleHealth returns health status for all or specific probes
func (h *HotReloadHandlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	probeName := r.URL.Query().Get("probe")
	if probeName != "" {
		health := h.manager.GetHealth(probeName)
		if health == nil {
			http.Error(w, "Probe not found", http.StatusNotFound)
			return
		}
		writeJSON(w, health)
		return
	}

	writeJSON(w, h.manager.GetAllHealth())
}

// handleStats returns hot reload statistics
func (h *HotReloadHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, h.manager.Stats())
}

// Package-level handler registration
var hotReloadHandlers *HotReloadHandlers

// SetHotReloadManager sets the hot reload manager for handlers
func SetHotReloadManager(manager *probe.HotReloadManager) {
	hotReloadHandlers = NewHotReloadHandlers(manager)
}

// RegisterHotReloadRoutes registers hot reload routes on a mux
func RegisterHotReloadRoutes(mux *http.ServeMux) {
	if hotReloadHandlers != nil {
		hotReloadHandlers.RegisterRoutes(mux)
	}
}
