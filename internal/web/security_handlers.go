package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/security"

	"github.com/google/uuid"
)

// securityStore holds the security store instance (set via SetSecurityStore)
var securityStore *security.Store

// SetSecurityStore sets the security store for the handlers
func SetSecurityStore(s *security.Store) {
	securityStore = s
}

// RegisterSecurityRoutes registers security API routes
func RegisterSecurityRoutes(mux *http.ServeMux) {
	// Events
	mux.HandleFunc("/api/security/events", handleSecurityEvents)

	// Alerts
	mux.HandleFunc("/api/security/alerts", handleSecurityAlerts)
	mux.HandleFunc("/api/security/alerts/", handleSecurityAlert)

	// Rules
	mux.HandleFunc("/api/security/rules", handleSecurityRules)
	mux.HandleFunc("/api/security/rules/", handleSecurityRule)

	// Stats
	mux.HandleFunc("/api/security/stats", handleSecurityStats)

	// MITRE ATT&CK
	mux.HandleFunc("/api/security/mitre", handleSecurityMitre)
}

// handleSecurityEvents handles GET /api/security/events
func handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse filters
	filter := security.EventFilter{}

	// Severity filter
	if severity := r.URL.Query().Get("severity"); severity != "" {
		filter.Severity = severity
	}

	// Event type filter
	if eventType := r.URL.Query().Get("type"); eventType != "" {
		filter.EventType = eventType
	}

	// Time range
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if start, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = start
		}
	} else {
		// Default to last 24 hours
		filter.StartTime = time.Now().Add(-24 * time.Hour)
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if end, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = end
		}
	} else {
		filter.EndTime = time.Now()
	}

	// Service filter
	if service := r.URL.Query().Get("service"); service != "" {
		filter.Service = service
	}

	// Source IP filter
	if sourceIP := r.URL.Query().Get("source_ip"); sourceIP != "" {
		filter.SourceIP = sourceIP
	}

	// Limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 100
	}

	// Offset for pagination
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			filter.Offset = offset
		}
	}

	events, err := securityStore.ListEvents(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// handleSecurityAlerts handles GET /api/security/alerts
func handleSecurityAlerts(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := security.AlertFilter{}

	// Status filter (open, acknowledged, resolved)
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = status
	}

	// Severity filter
	if severity := r.URL.Query().Get("severity"); severity != "" {
		filter.Severity = severity
	}

	// Time range
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if start, err := time.Parse(time.RFC3339, startStr); err == nil {
			filter.StartTime = start
		}
	}
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if end, err := time.Parse(time.RFC3339, endStr); err == nil {
			filter.EndTime = end
		}
	}

	// Limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}

	alerts, err := securityStore.ListAlerts(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

// handleSecurityAlert handles GET/POST for a specific alert
func handleSecurityAlert(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract alert ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/security/alerts/")
	parts := strings.Split(path, "/")
	alertID := parts[0]

	// Check for actions
	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "acknowledge":
			handleSecurityAlertAcknowledge(w, r, alertID)
			return
		case "resolve":
			handleSecurityAlertResolve(w, r, alertID)
			return
		case "investigate":
			handleSecurityAlertInvestigate(w, r, alertID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		alert, err := securityStore.GetAlert(alertID)
		if err != nil {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alert)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityAlertAcknowledge acknowledges a security alert
func handleSecurityAlertAcknowledge(w http.ResponseWriter, r *http.Request, alertID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID  string `json:"user_id"`
		Comment string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.UserID = "api"
	}

	if err := securityStore.AcknowledgeAlert(alertID, req.UserID, req.Comment); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
}

// handleSecurityAlertResolve resolves a security alert
func handleSecurityAlertResolve(w http.ResponseWriter, r *http.Request, alertID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID     string `json:"user_id"`
		Resolution string `json:"resolution"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.UserID = "api"
	}

	if err := securityStore.ResolveAlert(alertID, req.UserID, req.Resolution); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "resolved"})
}

// handleSecurityAlertInvestigate returns investigation details for an alert
func handleSecurityAlertInvestigate(w http.ResponseWriter, r *http.Request, alertID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	investigation, err := securityStore.GetAlertInvestigation(alertID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(investigation)
}

// handleSecurityRules handles GET/POST for detection rules
func handleSecurityRules(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Filter by enabled status
		enabled := r.URL.Query().Get("enabled")
		category := r.URL.Query().Get("category")

		rules, err := securityStore.ListRules(enabled, category)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rules)

	case http.MethodPost:
		var rule security.ThreatRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}
		if rule.CreatedAt.IsZero() {
			rule.CreatedAt = time.Now()
		}
		rule.UpdatedAt = time.Now()

		if err := securityStore.CreateRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityRule handles GET/PUT/DELETE for a specific rule
func handleSecurityRule(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract rule ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/security/rules/")
	ruleID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		rule, err := securityStore.GetRule(ruleID)
		if err != nil {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var rule security.ThreatRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rule.ID = ruleID
		rule.UpdatedAt = time.Now()

		if err := securityStore.UpdateRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)

	case http.MethodDelete:
		if err := securityStore.DeleteRule(ruleID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSecurityStats handles GET /api/security/stats
func handleSecurityStats(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Time range for stats
	hours := 24
	if hoursStr := r.URL.Query().Get("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	stats, err := securityStore.GetStats(hours)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleSecurityMitre returns MITRE ATT&CK mapping data
func handleSecurityMitre(w http.ResponseWriter, r *http.Request) {
	if securityStore == nil {
		http.Error(w, "security not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Time range
	hours := 24
	if hoursStr := r.URL.Query().Get("hours"); hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil {
			hours = h
		}
	}

	mitreData, err := securityStore.GetMitreMapping(hours)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mitreData)
}
