package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/logreduce"
	"dogwatch/internal/logs"
)

var logReduceStore *logreduce.Store

// SetLogReduceStore sets the log reduce store
func SetLogReduceStore(store *logreduce.Store) {
	logReduceStore = store
}

// ProcessLogsForPatterns processes log entries through the pattern miner
// Call this when logs are ingested to build up patterns
func ProcessLogsForPatterns(entries []logs.LogEntry) {
	if logReduceStore == nil {
		return
	}
	// Convert to logreduce.LogEntry format
	reduceEntries := make([]logreduce.LogEntry, len(entries))
	for i, e := range entries {
		reduceEntries[i] = logreduce.LogEntry{
			Message:   e.Message,
			Level:     string(e.Level),
			Service:   e.Service,
			Timestamp: e.Timestamp,
		}
	}
	logReduceStore.ProcessBatch(reduceEntries)
}

// RegisterLogReduceRoutes registers log reduction API routes
func RegisterLogReduceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/logs/patterns", handleLogPatterns)
	mux.HandleFunc("/api/logs/patterns/", handleLogPattern)
	mux.HandleFunc("/api/logs/reduce", handleLogReduce)
	mux.HandleFunc("/api/logs/patterns/stats", handlePatternStats)
	mux.HandleFunc("/api/logs/patterns/search", handlePatternSearch)
	mux.HandleFunc("/api/logs/patterns/match", handlePatternMatch)
	mux.HandleFunc("/api/logs/patterns/trending", handleTrendingPatterns)
	mux.HandleFunc("/api/logs/patterns/new", handleNewPatterns)
}

// handleLogPatterns returns all discovered patterns
// GET /api/logs/patterns - List patterns
// GET /api/logs/patterns?level=error - Filter by level
// GET /api/logs/patterns?service=X - Filter by service
// GET /api/logs/patterns?limit=50 - Limit results
func handleLogPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	level := r.URL.Query().Get("level")
	service := r.URL.Query().Get("service")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	var patterns []*logreduce.Pattern

	if level != "" {
		patterns = logReduceStore.GetPatternsByLevel(level)
	} else if service != "" {
		patterns = logReduceStore.GetPatternsByService(service)
	} else {
		patterns = logReduceStore.GetTopPatterns(limit)
	}

	// Apply limit
	if len(patterns) > limit {
		patterns = patterns[:limit]
	}

	// Calculate total count
	var totalCount int64
	for _, p := range patterns {
		totalCount += p.Count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":       len(patterns),
		"total_logs":  totalCount,
		"patterns":    patterns,
		"level":       level,
		"service":     service,
	})
}

// handleLogPattern handles individual pattern operations
// GET /api/logs/patterns/{id} - Get pattern details
// GET /api/logs/patterns/{id}/history - Get pattern history
// GET /api/logs/patterns/{id}/examples - Get example logs
func handleLogPattern(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Path[len("/api/logs/patterns/"):]
	parts := splitPath(path)

	if len(parts) == 0 {
		http.Error(w, "Pattern ID required", http.StatusBadRequest)
		return
	}

	patternID := parts[0]

	// Check for sub-routes
	if len(parts) > 1 {
		switch parts[1] {
		case "history":
			handlePatternHistory(w, r, patternID)
			return
		case "examples":
			handlePatternExamples(w, r, patternID)
			return
		}
	}

	// Get pattern details
	pattern := logReduceStore.GetPattern(patternID)
	if pattern == nil {
		http.Error(w, "Pattern not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pattern)
}

func handlePatternHistory(w http.ResponseWriter, r *http.Request, patternID string) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 24
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}

	history, err := logReduceStore.GetPatternHistory(patternID, hours)
	if err != nil {
		http.Error(w, "Failed to get history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pattern_id": patternID,
		"hours":      hours,
		"history":    history,
	})
}

func handlePatternExamples(w http.ResponseWriter, r *http.Request, patternID string) {
	pattern := logReduceStore.GetPattern(patternID)
	if pattern == nil {
		http.Error(w, "Pattern not found", http.StatusNotFound)
		return
	}

	// For now, just return the sample message
	// In a full implementation, we'd query the log store for matching logs
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pattern_id": patternID,
		"template":   pattern.Template,
		"examples":   []string{pattern.SampleMessage},
	})
}

// handleLogReduce processes logs and reduces them to patterns
// POST /api/logs/reduce
func handleLogReduce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Messages []string `json:"messages"`
		Level    string   `json:"level,omitempty"`
		Service  string   `json:"service,omitempty"`
		// Or fetch from log store
		StartTime *time.Time `json:"start_time,omitempty"`
		EndTime   *time.Time `json:"end_time,omitempty"`
		Query     string     `json:"query,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	var result *logreduce.ReduceResult

	if len(req.Messages) > 0 {
		// Process provided messages
		result = logReduceStore.Reduce(req.Messages, req.Level, req.Service)
	} else if req.StartTime != nil && req.EndTime != nil && logCompareStore != nil {
		// Fetch from log store and process
		entries, err := fetchLogsForReduce(logCompareStore, *req.StartTime, *req.EndTime, req.Service, req.Level, req.Query)
		if err != nil {
			http.Error(w, "Failed to fetch logs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		result = logReduceStore.ProcessBatch(entries)
	} else {
		http.Error(w, "Provide either messages array or start_time/end_time", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func fetchLogsForReduce(store *logs.Store, start, end time.Time, service, level, query string) ([]logreduce.LogEntry, error) {
	q := logs.SearchQuery{
		StartTime: start,
		EndTime:   end,
		Limit:     10000,
	}
	if service != "" {
		q.Service = service
	}
	if level != "" {
		q.Level = logs.LogLevel(level)
	}
	if query != "" {
		q.Query = query
	}

	results, err := store.Search(q)
	if err != nil {
		return nil, err
	}

	var entries []logreduce.LogEntry
	for _, entry := range results.Entries {
		entries = append(entries, logreduce.LogEntry{
			Timestamp: entry.Timestamp,
			Level:     string(entry.Level),
			Service:   entry.Service,
			Message:   entry.Message,
		})
	}

	return entries, nil
}

// handlePatternStats returns pattern mining statistics
// GET /api/logs/patterns/stats
func handlePatternStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	stats := logReduceStore.GetStats()

	// Add service summary
	serviceSummary := logReduceStore.GetServicePatternSummary()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats":           stats,
		"service_summary": serviceSummary,
	})
}

// handlePatternSearch searches for patterns matching a query
// GET /api/logs/patterns/search?q=error
func handlePatternSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	patterns := logReduceStore.Search(query)

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if len(patterns) > limit {
		patterns = patterns[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"query":    query,
		"count":    len(patterns),
		"patterns": patterns,
	})
}

// handlePatternMatch finds the pattern that matches a message
// POST /api/logs/patterns/match
func handlePatternMatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	match, found := logReduceStore.Match(req.Message)

	w.Header().Set("Content-Type", "application/json")
	if found {
		pattern := logReduceStore.GetPattern(match.PatternID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"matched":   true,
			"match":     match,
			"pattern":   pattern,
		})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"matched": false,
			"message": "No matching pattern found",
		})
	}
}

// handleTrendingPatterns returns patterns with increasing frequency
// GET /api/logs/patterns/trending
func handleTrendingPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	patterns := logReduceStore.GetTrendingPatterns()

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if len(patterns) > limit {
		patterns = patterns[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    len(patterns),
		"patterns": patterns,
	})
}

// handleNewPatterns returns recently discovered patterns
// GET /api/logs/patterns/new?since=1h
func handleNewPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if logReduceStore == nil {
		http.Error(w, "Log pattern mining not configured", http.StatusServiceUnavailable)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	patterns := logReduceStore.GetNewPatterns(since)

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if len(patterns) > limit {
		patterns = patterns[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"since":    since.String(),
		"count":    len(patterns),
		"patterns": patterns,
	})
}
