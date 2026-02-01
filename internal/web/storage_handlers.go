package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dogwatch/internal/storage"
)

var (
	walInstance      *storage.WAL
	tieringManager   *storage.TieringManager
	backendManager   *storage.BackendManager
)

// SetWAL sets the WAL instance for handlers
func SetWAL(wal *storage.WAL) {
	walInstance = wal
}

// SetTieringManager sets the tiering manager for handlers
func SetTieringManager(tm *storage.TieringManager) {
	tieringManager = tm
}

// SetBackendManager sets the backend manager for handlers
func SetBackendManager(bm *storage.BackendManager) {
	backendManager = bm
}

// RegisterStorageRoutes registers storage management API routes
func RegisterStorageRoutes(mux *http.ServeMux) {
	// WAL endpoints
	mux.HandleFunc("/api/storage/wal/stats", handleWALStats)
	mux.HandleFunc("/api/storage/wal/checkpoint", handleWALCheckpoint)
	mux.HandleFunc("/api/storage/wal/sync", handleWALSync)

	// Tiering endpoints
	mux.HandleFunc("/api/storage/tiering/stats", handleTieringStats)
	mux.HandleFunc("/api/storage/tiering/state", handleTieringState)
	mux.HandleFunc("/api/storage/tiering/compact", handleTieringCompact)
	mux.HandleFunc("/api/storage/tiering/tier", handleTieringTier)
	mux.HandleFunc("/api/storage/tiering/warm", handleTieringWarm)
	mux.HandleFunc("/api/storage/tiering/cold", handleTieringCold)
	mux.HandleFunc("/api/storage/tiering/restore", handleTieringRestore)

	// Backend endpoints
	mux.HandleFunc("/api/storage/backends", handleBackends)
	mux.HandleFunc("/api/storage/backends/test", handleBackendTest)
}

// WAL Handlers

func handleWALStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if walInstance == nil {
		http.Error(w, "WAL not configured", http.StatusServiceUnavailable)
		return
	}

	stats := walInstance.Stats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func handleWALCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if walInstance == nil {
		http.Error(w, "WAL not configured", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	pendingBefore := walInstance.PendingCount()

	if err := walInstance.ForceCheckpoint(); err != nil {
		http.Error(w, fmt.Sprintf("Checkpoint failed: %v", err), http.StatusInternalServerError)
		return
	}

	pendingAfter := walInstance.PendingCount()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"duration_ms":     time.Since(start).Milliseconds(),
		"entries_applied": pendingBefore - pendingAfter,
		"pending_before":  pendingBefore,
		"pending_after":   pendingAfter,
	})
}

func handleWALSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if walInstance == nil {
		http.Error(w, "WAL not configured", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	if err := walInstance.ForceSync(); err != nil {
		http.Error(w, fmt.Sprintf("Sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

// Tiering Handlers

func handleTieringStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	stats := tieringManager.Stats()

	// Add human-readable sizes
	response := map[string]interface{}{
		"hot_data_size":        stats.HotDataSize,
		"hot_data_size_human":  formatBytes(stats.HotDataSize),
		"warm_data_size":       stats.WarmDataSize,
		"warm_data_size_human": formatBytes(stats.WarmDataSize),
		"cold_data_size":       stats.ColdDataSize,
		"cold_data_size_human": formatBytes(stats.ColdDataSize),
		"total_compactions":    stats.TotalCompactions,
		"total_tierings":       stats.TotalTierings,
		"bytes_compacted":      stats.BytesCompacted,
		"bytes_tiered_to_warm": stats.BytesTieredToWarm,
		"bytes_tiered_to_cold": stats.BytesTieredToCold,
	}

	if stats.LastError != "" {
		response["last_error"] = stats.LastError
		response["last_error_time"] = stats.LastErrorTime
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTieringState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	state := tieringManager.State()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func handleTieringCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	if err := tieringManager.ForceCompact(); err != nil {
		http.Error(w, fmt.Sprintf("Compaction failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"duration_ms": time.Since(start).Milliseconds(),
	})
}

func handleTieringTier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	start := time.Now()
	statsBefore := tieringManager.Stats()

	if err := tieringManager.ForceTier(); err != nil {
		http.Error(w, fmt.Sprintf("Tiering failed: %v", err), http.StatusInternalServerError)
		return
	}

	statsAfter := tieringManager.Stats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":              true,
		"duration_ms":          time.Since(start).Milliseconds(),
		"bytes_moved_to_warm":  statsAfter.BytesTieredToWarm - statsBefore.BytesTieredToWarm,
		"bytes_moved_to_cold":  statsAfter.BytesTieredToCold - statsBefore.BytesTieredToCold,
	})
}

func handleTieringWarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	files := tieringManager.ListWarmFiles()

	// Enrich with human-readable sizes
	type enrichedFile struct {
		storage.WarmFile
		SizeHuman string `json:"size_human"`
	}

	response := make([]enrichedFile, len(files))
	for i, f := range files {
		response[i] = enrichedFile{
			WarmFile:  f,
			SizeHuman: formatBytes(f.Size),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTieringCold(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	archives := tieringManager.ListColdArchives()

	// Enrich with human-readable sizes
	type enrichedArchive struct {
		storage.ColdArchive
		SizeHuman string `json:"size_human"`
	}

	response := make([]enrichedArchive, len(archives))
	for i, a := range archives {
		response[i] = enrichedArchive{
			ColdArchive: a,
			SizeHuman:   formatBytes(a.Size),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleTieringRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if tieringManager == nil {
		http.Error(w, "Tiering not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Key string `json:"key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Key == "" {
		http.Error(w, "key is required", http.StatusBadRequest)
		return
	}

	start := time.Now()
	if err := tieringManager.RestoreFromCold(r.Context(), req.Key); err != nil {
		http.Error(w, fmt.Sprintf("Restore failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"duration_ms": time.Since(start).Milliseconds(),
		"key":         req.Key,
	})
}

// Backend Handlers

func handleBackends(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleBackendsList(w, r)
	case http.MethodPost:
		handleBackendsCreate(w, r)
	case http.MethodDelete:
		handleBackendsDelete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleBackendsList(w http.ResponseWriter, r *http.Request) {
	if backendManager == nil {
		http.Error(w, "Backend manager not configured", http.StatusServiceUnavailable)
		return
	}

	names := backendManager.List()

	type backendInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	response := make([]backendInfo, len(names))
	for i, name := range names {
		backend, _ := backendManager.Get(name)
		response[i] = backendInfo{
			Name: name,
			Type: backend.Name(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleBackendsCreate(w http.ResponseWriter, r *http.Request) {
	if backendManager == nil {
		http.Error(w, "Backend manager not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name   string               `json:"name"`
		Config storage.BackendConfig `json:"config"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	backend, err := storage.NewBackend(req.Config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create backend: %v", err), http.StatusBadRequest)
		return
	}

	backendManager.Register(req.Name, backend)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"name":    req.Name,
		"type":    backend.Name(),
	})
}

func handleBackendsDelete(w http.ResponseWriter, r *http.Request) {
	if backendManager == nil {
		http.Error(w, "Backend manager not configured", http.StatusServiceUnavailable)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "name query parameter is required", http.StatusBadRequest)
		return
	}

	backendManager.Remove(name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"name":    name,
	})
}

func handleBackendTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if backendManager == nil {
		http.Error(w, "Backend manager not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	backend, ok := backendManager.Get(req.Name)
	if !ok {
		http.Error(w, "Backend not found", http.StatusNotFound)
		return
	}

	// Test by listing root
	start := time.Now()
	_, err := backend.List(r.Context(), "")
	duration := time.Since(start)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     false,
			"name":        req.Name,
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"name":        req.Name,
		"duration_ms": duration.Milliseconds(),
	})
}

// Helper function to format bytes as human-readable
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
