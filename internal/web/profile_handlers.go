package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/profile"
)

// ProfileHandler provides HTTP endpoints for profile-trace linking.
type ProfileHandler struct {
	linker *profile.Linker
	store  *profile.Store
}

// NewProfileHandler creates a new profile handler.
func NewProfileHandler(linker *profile.Linker, store *profile.Store) *ProfileHandler {
	return &ProfileHandler{
		linker: linker,
		store:  store,
	}
}

// RegisterRoutes registers all profile-related routes.
func (h *ProfileHandler) RegisterRoutes(mux *http.ServeMux) {
	// Profile endpoints
	mux.HandleFunc("GET /api/profiles", h.handleListProfiles)
	mux.HandleFunc("GET /api/profiles/hotspots", h.handleHotspots)
	mux.HandleFunc("GET /api/profiles/{id}", h.handleGetProfile)

	// Profile-trace linking endpoints
	mux.HandleFunc("GET /api/profiles/{id}/traces", h.handleProfileTraces)
	mux.HandleFunc("GET /api/profiles/function/{name}/traces", h.handleFunctionTraces)
	mux.HandleFunc("GET /api/traces/{traceId}/profiles", h.handleTraceProfiles)
	mux.HandleFunc("GET /api/traces/{traceId}/spans/{spanId}/profiles", h.handleSpanProfiles)

	// Linking management
	mux.HandleFunc("POST /api/profiles/link", h.handleAutoLink)
	mux.HandleFunc("GET /api/profiles/stats", h.handleStats)
}

// handleListProfiles returns profile samples within a time range.
func (h *ProfileHandler) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	start, end := parseTimeRange(r)

	samples, err := h.store.QueryByTimeRange(start, end)
	if err != nil {
		writeProfileError(w, "failed to query profiles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, map[string]interface{}{
		"samples": samples,
		"count":   len(samples),
	})
}

// handleHotspots returns the top CPU-consuming functions.
func (h *ProfileHandler) handleHotspots(w http.ResponseWriter, r *http.Request) {
	start, end := parseTimeRange(r)

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	hotspots, err := h.store.GetHotspots(start, end, limit)
	if err != nil {
		writeProfileError(w, "failed to get hotspots: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, map[string]interface{}{
		"hotspots": hotspots,
		"start":    start,
		"end":      end,
	})
}

// handleGetProfile returns a specific profile sample.
func (h *ProfileHandler) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeProfileError(w, "invalid profile id", http.StatusBadRequest)
		return
	}

	sample, err := h.store.GetSampleByID(id)
	if err != nil {
		writeProfileError(w, "profile not found", http.StatusNotFound)
		return
	}

	writeProfileJSON(w, sample)
}

// handleProfileTraces returns traces linked to a specific profile sample.
func (h *ProfileHandler) handleProfileTraces(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeProfileError(w, "invalid profile id", http.StatusBadRequest)
		return
	}

	sample, err := h.store.GetSampleByID(id)
	if err != nil {
		writeProfileError(w, "profile not found", http.StatusNotFound)
		return
	}

	opts := parseLinkOptions(r)
	result, err := h.linker.LinkSampleToTraces(r.Context(), sample, opts)
	if err != nil {
		writeProfileError(w, "failed to link traces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, result)
}

// handleFunctionTraces returns traces related to a specific function hotspot.
func (h *ProfileHandler) handleFunctionTraces(w http.ResponseWriter, r *http.Request) {
	functionName := r.PathValue("name")
	if functionName == "" {
		writeProfileError(w, "missing function name", http.StatusBadRequest)
		return
	}

	start, end := parseTimeRange(r)
	opts := parseLinkOptions(r)

	profile, err := h.linker.GetTracesForHotspot(r.Context(), functionName, start, end, opts)
	if err != nil {
		writeProfileError(w, "failed to get function traces: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, profile)
}

// handleTraceProfiles returns profile samples related to a trace.
func (h *ProfileHandler) handleTraceProfiles(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceId")
	if traceID == "" {
		writeProfileError(w, "missing trace id", http.StatusBadRequest)
		return
	}

	samples, err := h.linker.GetProfilesForTrace(r.Context(), traceID)
	if err != nil {
		writeProfileError(w, "failed to get trace profiles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, map[string]interface{}{
		"trace_id": traceID,
		"samples":  samples,
		"count":    len(samples),
	})
}

// handleSpanProfiles returns profile samples related to a specific span.
func (h *ProfileHandler) handleSpanProfiles(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("traceId")
	spanID := r.PathValue("spanId")
	if traceID == "" || spanID == "" {
		writeProfileError(w, "missing trace or span id", http.StatusBadRequest)
		return
	}

	samples, err := h.linker.GetProfilesForSpan(r.Context(), traceID, spanID)
	if err != nil {
		writeProfileError(w, "failed to get span profiles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, map[string]interface{}{
		"trace_id": traceID,
		"span_id":  spanID,
		"samples":  samples,
		"count":    len(samples),
	})
}

// handleAutoLink triggers automatic profile-trace linking.
func (h *ProfileHandler) handleAutoLink(w http.ResponseWriter, r *http.Request) {
	lookback := time.Hour
	if lookbackStr := r.URL.Query().Get("lookback"); lookbackStr != "" {
		if d, err := time.ParseDuration(lookbackStr); err == nil {
			lookback = d
		}
	}

	linked, err := h.linker.AutoLink(r.Context(), lookback)
	if err != nil {
		writeProfileError(w, "auto-link failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, map[string]interface{}{
		"linked":   linked,
		"lookback": lookback.String(),
	})
}

// handleStats returns profile-trace linking statistics.
func (h *ProfileHandler) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.linker.GetStats(r.Context())
	if err != nil {
		writeProfileError(w, "failed to get stats: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeProfileJSON(w, stats)
}

// Helper functions

func parseTimeRange(r *http.Request) (start, end time.Time) {
	end = time.Now()
	start = end.Add(-time.Hour)

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		} else if ts, err := strconv.ParseInt(startStr, 10, 64); err == nil {
			start = time.Unix(ts, 0)
		}
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		} else if ts, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = time.Unix(ts, 0)
		}
	}

	return start, end
}

func parseLinkOptions(r *http.Request) profile.LinkOptions {
	opts := profile.DefaultLinkOptions()

	if windowStr := r.URL.Query().Get("time_window"); windowStr != "" {
		if d, err := time.ParseDuration(windowStr); err == nil {
			opts.TimeWindow = d
		}
	}

	if confStr := r.URL.Query().Get("min_confidence"); confStr != "" {
		if c, err := strconv.ParseFloat(confStr, 64); err == nil {
			opts.MinConfidence = c
		}
	}

	if limitStr := r.URL.Query().Get("max_links"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			opts.MaxLinks = l
		}
	}

	if r.URL.Query().Get("include_kernel") == "true" {
		opts.IncludeKernelFunctions = true
	}

	return opts
}

func writeProfileJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeProfileError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
