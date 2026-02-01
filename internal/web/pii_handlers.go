package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/pii"
)

// PIIHandlers provides HTTP handlers for PII detection and redaction
type PIIHandlers struct {
	store *pii.Store
}

// NewPIIHandlers creates PII handlers
func NewPIIHandlers(store *pii.Store) *PIIHandlers {
	return &PIIHandlers{store: store}
}

// RegisterRoutes registers PII routes
func (h *PIIHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/pii/config", h.handleConfig)
	mux.HandleFunc("/api/pii/scan", h.handleScan)
	mux.HandleFunc("/api/pii/stats", h.handleStats)
	mux.HandleFunc("/api/pii/types", h.handleTypes)
	mux.HandleFunc("/api/pii/allowlist", h.handleAllowlist)
	mux.HandleFunc("/api/pii/denylist", h.handleDenylist)
	mux.HandleFunc("/api/pii/patterns", h.handlePatterns)
}

// handleConfig handles GET/PUT for PII configuration
func (h *PIIHandlers) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		config := h.store.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case http.MethodPut:
		var config pii.Config
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := config.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := h.store.UpdateConfig(&config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "updated"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleScan handles POST to scan text for PII
func (h *PIIHandlers) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req pii.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text is required", http.StatusBadRequest)
		return
	}

	result := h.store.Scan(req.Text)

	// Optionally record detections
	if req.Source != "" && len(result.Detections) > 0 {
		h.store.RecordDetections(req.Source, "", result.Detections, true)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleStats handles GET for PII detection statistics
func (h *PIIHandlers) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse duration parameter (default 24h)
	sinceStr := r.URL.Query().Get("since")
	since := 24 * time.Hour
	if sinceStr != "" {
		if parsed, err := time.ParseDuration(sinceStr); err == nil {
			since = parsed
		}
	}

	stats, err := h.store.GetStats(since)
	if err != nil {
		http.Error(w, "Failed to get stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleTypes handles GET for supported PII types
func (h *PIIHandlers) handleTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	types := []map[string]interface{}{
		{"type": pii.TypeEmail, "name": "Email Address", "description": "Email addresses in standard format"},
		{"type": pii.TypePhone, "name": "Phone Number", "description": "US and international phone numbers"},
		{"type": pii.TypeSSN, "name": "Social Security Number", "description": "US SSN in xxx-xx-xxxx format"},
		{"type": pii.TypeCreditCard, "name": "Credit Card", "description": "Visa, Mastercard, Amex, Discover with Luhn validation"},
		{"type": pii.TypeIPAddress, "name": "IP Address", "description": "IPv4 addresses (disabled by default)"},
		{"type": pii.TypeAWSKey, "name": "AWS Access Key", "description": "AWS Access Key IDs"},
		{"type": pii.TypeGitHubToken, "name": "GitHub Token", "description": "GitHub Personal Access Tokens"},
		{"type": pii.TypeStripeKey, "name": "Stripe API Key", "description": "Stripe live and test API keys"},
		{"type": pii.TypeGenericKey, "name": "Generic API Key", "description": "Generic API key patterns"},
		{"type": pii.TypeJWT, "name": "JWT Token", "description": "JSON Web Tokens"},
		{"type": pii.TypePassword, "name": "Password", "description": "Passwords in URLs or query strings"},
	}

	config := h.store.GetConfig()
	for i := range types {
		piiType := pii.PIIType(types[i]["type"].(pii.PIIType))
		types[i]["enabled"] = config.IsTypeEnabled(piiType)
		types[i]["strategy"] = config.GetStrategy(piiType)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types)
}

// handleAllowlist handles GET/POST/DELETE for allowlist management
func (h *PIIHandlers) handleAllowlist(w http.ResponseWriter, r *http.Request) {
	config := h.store.GetConfig()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config.Allowlist)

	case http.MethodPost:
		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		config.AddToAllowlist(req.Value)
		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})

	case http.MethodDelete:
		value := r.URL.Query().Get("value")
		if value == "" {
			http.Error(w, "value parameter required", http.StatusBadRequest)
			return
		}

		if !config.RemoveFromAllowlist(value) {
			http.Error(w, "Value not found", http.StatusNotFound)
			return
		}

		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleDenylist handles GET/POST/DELETE for denylist management
func (h *PIIHandlers) handleDenylist(w http.ResponseWriter, r *http.Request) {
	config := h.store.GetConfig()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config.Denylist)

	case http.MethodPost:
		var req struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		config.AddToDenylist(req.Value)
		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})

	case http.MethodDelete:
		value := r.URL.Query().Get("value")
		if value == "" {
			http.Error(w, "value parameter required", http.StatusBadRequest)
			return
		}

		if !config.RemoveFromDenylist(value) {
			http.Error(w, "Value not found", http.StatusNotFound)
			return
		}

		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePatterns handles GET/POST/DELETE for custom pattern management
func (h *PIIHandlers) handlePatterns(w http.ResponseWriter, r *http.Request) {
	config := h.store.GetConfig()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config.CustomPatterns)

	case http.MethodPost:
		var pattern pii.CustomPattern
		if err := json.NewDecoder(r.Body).Decode(&pattern); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if pattern.Name == "" || pattern.Pattern == "" {
			http.Error(w, "Name and pattern are required", http.StatusBadRequest)
			return
		}

		config.AddCustomPattern(pattern.Name, pattern.Pattern, pattern.Strategy)
		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "added"})

	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "name parameter required", http.StatusBadRequest)
			return
		}

		if !config.RemoveCustomPattern(name) {
			http.Error(w, "Pattern not found", http.StatusNotFound)
			return
		}

		if err := h.store.UpdateConfig(config); err != nil {
			http.Error(w, "Failed to update config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Global PII store and handlers for middleware integration
var (
	globalPIIStore    *pii.Store
	globalPIIHandlers *PIIHandlers
)

// SetPIIStore sets the global PII store
func SetPIIStore(store *pii.Store) {
	globalPIIStore = store
	globalPIIHandlers = NewPIIHandlers(store)
}

// GetPIIStore returns the global PII store
func GetPIIStore() *pii.Store {
	return globalPIIStore
}

// GetPIIHandlers returns the global PII handlers
func GetPIIHandlers() *PIIHandlers {
	return globalPIIHandlers
}

// PIIMiddleware wraps a handler to redact PII from responses
func PIIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if PII store not configured
		if globalPIIStore == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Check if PII redaction is enabled
		config := globalPIIStore.GetConfig()
		if !config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Wrap response writer to capture and redact response
		rw := &piiResponseWriter{
			ResponseWriter: w,
			store:          globalPIIStore,
		}

		next.ServeHTTP(rw, r)
	})
}

// piiResponseWriter wraps http.ResponseWriter to redact PII
type piiResponseWriter struct {
	http.ResponseWriter
	store *pii.Store
}

func (w *piiResponseWriter) Write(data []byte) (int, error) {
	// Only process JSON responses
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		return w.ResponseWriter.Write(data)
	}

	redactor := w.store.GetRedactor()
	redacted := redactor.Redact(string(data))

	return w.ResponseWriter.Write([]byte(redacted))
}

// LogPIIMiddleware is middleware for log ingestion that applies PII redaction
type LogPIIMiddleware struct {
	store *pii.Store
}

// NewLogPIIMiddleware creates log PII middleware
func NewLogPIIMiddleware(store *pii.Store) *LogPIIMiddleware {
	return &LogPIIMiddleware{store: store}
}

// RedactLogEntry redacts PII from a log entry before storage
func (m *LogPIIMiddleware) RedactLogEntry(message string, attrs map[string]string) (string, map[string]string) {
	if m.store == nil {
		return message, attrs
	}

	config := m.store.GetConfig()
	if !config.Enabled {
		return message, attrs
	}

	redactor := m.store.GetRedactor()

	// Redact message
	redactedMsg, detections := redactor.RedactWithDetails(message)

	// Record detections
	if len(detections) > 0 {
		m.store.RecordDetections("log", "", detections, true)
	}

	// Redact attributes
	redactedAttrs := redactor.RedactAttributes(attrs)

	return redactedMsg, redactedAttrs
}

// TracePIIMiddleware is middleware for trace spans that applies PII redaction
type TracePIIMiddleware struct {
	store *pii.Store
}

// NewTracePIIMiddleware creates trace PII middleware
func NewTracePIIMiddleware(store *pii.Store) *TracePIIMiddleware {
	return &TracePIIMiddleware{store: store}
}

// RedactSpanAttributes redacts PII from span attributes before storage
func (m *TracePIIMiddleware) RedactSpanAttributes(attrs map[string]string) map[string]string {
	if m.store == nil {
		return attrs
	}

	config := m.store.GetConfig()
	if !config.Enabled {
		return attrs
	}

	redactor := m.store.GetRedactor()
	redactedAttrs := redactor.RedactAttributes(attrs)

	// Record any detections
	detector := redactor.GetDetector()
	for k, v := range attrs {
		detections := detector.Detect(v)
		if len(detections) > 0 {
			m.store.RecordDetections("trace", k, detections, true)
		}
	}

	return redactedAttrs
}

// ScanExistingData scans existing data for PII
type ScanResult struct {
	Source      string        `json:"source"`
	TotalScanned int          `json:"total_scanned"`
	PIIFound    int           `json:"pii_found"`
	ByType      map[string]int `json:"by_type"`
	Duration    string         `json:"duration"`
}

// ScanExistingLogs scans existing logs for PII (utility function)
func ScanExistingLogs(store *pii.Store, logDB interface{}, limit int) (*ScanResult, error) {
	// This would integrate with the logs store to scan existing logs
	// Implementation would query logs and scan each message
	result := &ScanResult{
		Source:       "logs",
		TotalScanned: 0,
		PIIFound:     0,
		ByType:       make(map[string]int),
	}

	return result, nil
}

// Helper function to parse int with default
func parseIntOrDefault(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
