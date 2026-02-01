package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"dogwatch/internal/logs"
)

var fieldExtractor *logs.FieldExtractor
var extractionStore *logs.PatternLearningStore

// SetFieldExtractor sets the field extractor for API handlers
func SetFieldExtractor(extractor *logs.FieldExtractor) {
	fieldExtractor = extractor
}

// SetExtractionStore sets the extraction pattern store for API handlers
func SetExtractionStore(store *logs.PatternLearningStore) {
	extractionStore = store
}

// RegisterExtractionRoutes registers log field extraction API routes
func RegisterExtractionRoutes(mux *http.ServeMux) {
	// Extract fields from log messages
	mux.HandleFunc("/api/logs/extraction/extract", handleExtract)

	// Pattern management
	mux.HandleFunc("/api/logs/extraction/patterns", handleExtractionPatterns)
	mux.HandleFunc("/api/logs/extraction/patterns/", handleExtractionPattern)

	// Source-specific patterns
	mux.HandleFunc("/api/logs/extraction/sources", handleExtractionSources)
	mux.HandleFunc("/api/logs/extraction/sources/", handleExtractionSource)

	// Grok patterns
	mux.HandleFunc("/api/logs/extraction/grok", handleGrokPatterns)
	mux.HandleFunc("/api/logs/extraction/grok/test", handleGrokTest)

	// Learning mode
	mux.HandleFunc("/api/logs/extraction/learn", handleExtractionLearn)
	mux.HandleFunc("/api/logs/extraction/stats", handleExtractionStats)
}

// handleExtract extracts fields from log messages
// POST /api/logs/extraction/extract
func handleExtract(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if fieldExtractor == nil {
		http.Error(w, "Field extraction not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Messages []string `json:"messages"`
		Source   string   `json:"source,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		http.Error(w, "messages array is required", http.StatusBadRequest)
		return
	}

	results := make([]map[string]interface{}, len(req.Messages))

	source := req.Source
	for i, msg := range req.Messages {
		fields := fieldExtractor.Extract(msg, source)

		// Convert to map
		fieldMap := make(map[string]interface{})
		for _, f := range fields {
			fieldMap[f.Name] = map[string]interface{}{
				"value":      f.Value,
				"type":       f.Type,
				"source":     f.Source,
				"confidence": f.Confidence,
			}
		}

		results[i] = map[string]interface{}{
			"message":     msg,
			"field_count": len(fields),
			"fields":      fieldMap,
		}
	}

	// Learn from extraction if store is configured
	if extractionStore != nil && req.Source != "" {
		for _, msg := range req.Messages {
			fields := fieldExtractor.Extract(msg, req.Source)
			extractionStore.RecordExtraction(req.Source, msg, fields)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(results),
		"results": results,
	})
}

// handleExtractionPatterns handles listing and creating patterns
// GET /api/logs/extraction/patterns - List all patterns
// POST /api/logs/extraction/patterns - Create a new pattern
func handleExtractionPatterns(w http.ResponseWriter, r *http.Request) {
	if fieldExtractor == nil {
		http.Error(w, "Field extraction not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		allPatterns := fieldExtractor.GetPatterns()

		// Filter by type if requested
		patternType := r.URL.Query().Get("type")
		var patterns []*logs.ExtractionPattern
		if patternType != "" {
			for _, p := range allPatterns {
				if string(p.Type) == patternType {
					patterns = append(patterns, p)
				}
			}
		} else {
			patterns = allPatterns
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":    len(patterns),
			"patterns": patterns,
		})

	case http.MethodPost:
		var pattern logs.ExtractionPattern
		if err := json.NewDecoder(r.Body).Decode(&pattern); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if pattern.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if pattern.Pattern == "" {
			http.Error(w, "pattern is required", http.StatusBadRequest)
			return
		}

		if err := fieldExtractor.AddPatternValue(pattern); err != nil {
			http.Error(w, "Failed to add pattern: "+err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(pattern)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleExtractionPattern handles individual pattern operations
// GET /api/logs/extraction/patterns/{name} - Get pattern
// DELETE /api/logs/extraction/patterns/{name} - Remove pattern
func handleExtractionPattern(w http.ResponseWriter, r *http.Request) {
	if fieldExtractor == nil {
		http.Error(w, "Field extraction not configured", http.StatusServiceUnavailable)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/logs/extraction/patterns/")
	if name == "" {
		http.Error(w, "Pattern name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		patterns := fieldExtractor.GetPatterns()
		for _, p := range patterns {
			if p.Name == name {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(p)
				return
			}
		}
		http.Error(w, "Pattern not found", http.StatusNotFound)

	case http.MethodDelete:
		if err := fieldExtractor.RemovePatternByName(name); err != nil {
			http.Error(w, "Failed to remove pattern: "+err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleExtractionSources lists sources with learned patterns
// GET /api/logs/extraction/sources
func handleExtractionSources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if extractionStore == nil {
		http.Error(w, "Extraction store not configured", http.StatusServiceUnavailable)
		return
	}

	sources := extractionStore.GetSources()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(sources),
		"sources": sources,
	})
}

// handleExtractionSource handles source-specific operations
// GET /api/logs/extraction/sources/{source} - Get learned patterns for source
// GET /api/logs/extraction/sources/{source}/fields - Get common fields for source
func handleExtractionSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if extractionStore == nil {
		http.Error(w, "Extraction store not configured", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/logs/extraction/sources/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Source name required", http.StatusBadRequest)
		return
	}

	source := parts[0]

	// Check for sub-routes
	if len(parts) > 1 && parts[1] == "fields" {
		fields := extractionStore.GetCommonFields(source)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"source": source,
			"count":  len(fields),
			"fields": fields,
		})
		return
	}

	// Return learned patterns for source
	patterns := extractionStore.GetLearnedPatterns(source)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"source":   source,
		"count":    len(patterns),
		"patterns": patterns,
	})
}

// handleGrokPatterns returns available Grok patterns
// GET /api/logs/extraction/grok
func handleGrokPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if fieldExtractor == nil {
		http.Error(w, "Field extraction not configured", http.StatusServiceUnavailable)
		return
	}

	grokPatterns := fieldExtractor.GetGrokPatterns()

	// Group by category
	categories := map[string][]map[string]string{
		"network":   {},
		"datetime":  {},
		"common":    {},
		"log_level": {},
		"other":     {},
	}

	for name, pattern := range grokPatterns {
		entry := map[string]string{
			"name":    name,
			"pattern": pattern,
		}

		switch {
		case strings.Contains(name, "IP") || strings.Contains(name, "HOST") || strings.Contains(name, "URI"):
			categories["network"] = append(categories["network"], entry)
		case strings.Contains(name, "TIME") || strings.Contains(name, "DATE"):
			categories["datetime"] = append(categories["datetime"], entry)
		case strings.Contains(name, "LEVEL") || strings.Contains(name, "LOG"):
			categories["log_level"] = append(categories["log_level"], entry)
		case name == "UUID" || name == "NUMBER" || name == "WORD" || name == "DATA" || name == "GREEDYDATA":
			categories["common"] = append(categories["common"], entry)
		default:
			categories["other"] = append(categories["other"], entry)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":      len(grokPatterns),
		"categories": categories,
	})
}

// handleGrokTest tests a Grok pattern against sample messages
// POST /api/logs/extraction/grok/test
func handleGrokTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if fieldExtractor == nil {
		http.Error(w, "Field extraction not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Pattern  string   `json:"pattern"`
		Messages []string `json:"messages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Pattern == "" {
		http.Error(w, "pattern is required", http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages array is required", http.StatusBadRequest)
		return
	}

	// Test the pattern
	results, err := fieldExtractor.TestGrokPattern(req.Pattern, req.Messages)
	if err != nil {
		http.Error(w, "Pattern error: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pattern": req.Pattern,
		"results": results,
	})
}

// handleExtractionLearn triggers learning from existing logs
// POST /api/logs/extraction/learn
func handleExtractionLearn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if extractionStore == nil {
		http.Error(w, "Extraction store not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Source   string   `json:"source"`
		Messages []string `json:"messages"`
		Limit    int      `json:"limit,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}

	limit := req.Limit
	if limit == 0 {
		limit = 1000
	}

	if len(req.Messages) > 0 {
		// Learn from provided messages
		for _, msg := range req.Messages {
			if fieldExtractor != nil {
				fields := fieldExtractor.Extract(msg, req.Source)
				extractionStore.RecordExtraction(req.Source, msg, fields)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":          true,
			"source":           req.Source,
			"messages_learned": len(req.Messages),
		})
	} else {
		// Return instructions for learning from log store
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Provide messages array to learn from, or learning happens automatically on log ingestion",
			"source":  req.Source,
		})
	}
}

// handleExtractionStats returns extraction statistics
// GET /api/logs/extraction/stats
func handleExtractionStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := make(map[string]interface{})

	if fieldExtractor != nil {
		patterns := fieldExtractor.GetPatterns()
		stats["patterns_count"] = len(patterns)

		// Count by type
		typeCounts := make(map[string]int)
		for _, p := range patterns {
			typeCounts[string(p.Type)]++
		}
		stats["patterns_by_type"] = typeCounts

		// Get grok pattern count
		grokPatterns := fieldExtractor.GetGrokPatterns()
		stats["grok_patterns"] = len(grokPatterns)
	}

	if extractionStore != nil {
		sources := extractionStore.GetSources()
		stats["sources_count"] = len(sources)
		stats["sources"] = sources

		// Get stats per source
		sourceStats := make(map[string]interface{})
		for _, source := range sources {
			fields := extractionStore.GetCommonFields(source)
			learned := extractionStore.GetLearnedPatterns(source)
			sourceStats[source] = map[string]interface{}{
				"common_fields":    len(fields),
				"learned_patterns": len(learned),
			}
		}
		stats["source_stats"] = sourceStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// extractionLimitStr parses limit from query string
func extractionLimitStr(r *http.Request, defaultLimit int) int {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit
	}
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		return l
	}
	return defaultLimit
}
