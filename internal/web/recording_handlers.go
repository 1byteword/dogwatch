package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/recording"

	"github.com/google/uuid"
)

// recordingManager holds the recording rule manager instance
var recordingManager *recording.Manager
var recordingStore *recording.Store

// SetRecordingManager sets the recording manager for the handlers
func SetRecordingManager(manager *recording.Manager, store *recording.Store) {
	recordingManager = manager
	recordingStore = store
}

// RegisterRecordingRoutes registers recording rule API routes
func RegisterRecordingRoutes(mux *http.ServeMux) {
	// Rules
	mux.HandleFunc("/api/recording-rules", handleRecordingRules)
	mux.HandleFunc("/api/recording-rules/", handleRecordingRule)

	// Status
	mux.HandleFunc("/api/recording-rules-status", handleRecordingStatus)
}

// handleRecordingRules handles GET/POST for recording rules
func handleRecordingRules(w http.ResponseWriter, r *http.Request) {
	if recordingStore == nil {
		http.Error(w, "recording rules not configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// Optional filter by enabled
		enabledOnly := r.URL.Query().Get("enabled") == "true"

		var rules []recording.RecordingRule
		var err error

		if enabledOnly {
			rules, err = recordingStore.ListEnabledRules()
		} else {
			rules, err = recordingStore.ListRules()
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if rules == nil {
			rules = []recording.RecordingRule{}
		}

		json.NewEncoder(w).Encode(rules)

	case http.MethodPost:
		var rule recording.RecordingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validate required fields
		if rule.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if rule.Expression == "" {
			http.Error(w, "expression is required", http.StatusBadRequest)
			return
		}

		// Set defaults
		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}
		if rule.Interval == 0 {
			rule.Interval = time.Minute
		}
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}

		// Set __name__ label to match rule name
		if _, ok := rule.Labels["__name__"]; !ok {
			rule.Labels["__name__"] = rule.Name
		}
		rule.Labels["source"] = "recording_rule"

		rule.Enabled = true
		rule.CreatedAt = time.Now()
		rule.UpdatedAt = time.Now()

		if err := recordingStore.CreateRule(&rule); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				http.Error(w, "a rule with this name already exists", http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRecordingRule handles GET/PUT/DELETE for a specific rule
func handleRecordingRule(w http.ResponseWriter, r *http.Request) {
	if recordingStore == nil {
		http.Error(w, "recording rules not configured", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Extract rule ID from path: /api/recording-rules/{id}[/{action}]
	path := strings.TrimPrefix(r.URL.Path, "/api/recording-rules/")
	parts := strings.Split(path, "/")
	ruleID := parts[0]

	if ruleID == "" {
		http.Error(w, "rule ID required", http.StatusBadRequest)
		return
	}

	// Check for actions
	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "evaluate":
			handleRecordingEvaluate(w, r, ruleID)
			return
		case "history":
			handleRecordingHistory(w, r, ruleID)
			return
		case "enable":
			handleRecordingEnable(w, r, ruleID, true)
			return
		case "disable":
			handleRecordingEnable(w, r, ruleID, false)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		rule, err := recordingStore.GetRule(ruleID)
		if err != nil {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var rule recording.RecordingRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Preserve ID
		rule.ID = ruleID

		// Validate required fields
		if rule.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if rule.Expression == "" {
			http.Error(w, "expression is required", http.StatusBadRequest)
			return
		}

		// Set defaults
		if rule.Interval == 0 {
			rule.Interval = time.Minute
		}
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}

		// Ensure source label
		rule.Labels["source"] = "recording_rule"
		if _, ok := rule.Labels["__name__"]; !ok {
			rule.Labels["__name__"] = rule.Name
		}

		rule.UpdatedAt = time.Now()

		if err := recordingStore.UpdateRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(rule)

	case http.MethodDelete:
		// Prevent deleting built-in rules
		rule, err := recordingStore.GetRule(ruleID)
		if err != nil {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}

		if strings.HasPrefix(rule.ID, "builtin:") {
			http.Error(w, "cannot delete built-in rules", http.StatusForbidden)
			return
		}

		if err := recordingStore.DeleteRule(ruleID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRecordingEvaluate manually triggers evaluation of a rule
func handleRecordingEvaluate(w http.ResponseWriter, r *http.Request, ruleID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if recordingManager == nil {
		http.Error(w, "recording manager not running", http.StatusServiceUnavailable)
		return
	}

	result, err := recordingManager.EvaluateNow(ruleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"rule_id":     ruleID,
		"evaluated":   true,
		"duration_ms": result.Duration.Milliseconds(),
		"values":      result.Values,
	}

	if result.Error != nil {
		response["error"] = result.Error.Error()
		response["success"] = false
	} else {
		response["success"] = true
	}

	json.NewEncoder(w).Encode(response)
}

// handleRecordingHistory returns evaluation history for a rule
func handleRecordingHistory(w http.ResponseWriter, r *http.Request, ruleID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history, err := recordingStore.GetEvaluationHistory(ruleID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if history == nil {
		history = []recording.EvaluationHistory{}
	}

	json.NewEncoder(w).Encode(history)
}

// handleRecordingEnable enables or disables a rule
func handleRecordingEnable(w http.ResponseWriter, r *http.Request, ruleID string, enabled bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rule, err := recordingStore.GetRule(ruleID)
	if err != nil {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	rule.Enabled = enabled
	rule.UpdatedAt = time.Now()

	if err := recordingStore.UpdateRule(rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status := "disabled"
	if enabled {
		status = "enabled"
	}

	json.NewEncoder(w).Encode(map[string]string{
		"status":  status,
		"rule_id": ruleID,
	})
}

// handleRecordingStatus returns the recording rules system status
func handleRecordingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if recordingManager == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not_configured",
		})
		return
	}

	stats := recordingManager.GetStats()

	response := map[string]interface{}{
		"status":                  "running",
		"running":                 recordingManager.IsRunning(),
		"total_rules":             stats.TotalRules,
		"enabled_rules":           stats.EnabledRules,
		"total_evaluations":       stats.TotalEvaluations,
		"successful_evaluations":  stats.SuccessfulEvals,
		"failed_evaluations":      stats.FailedEvals,
		"last_eval_duration_ms":   stats.LastEvalDuration.Milliseconds(),
		"avg_eval_duration_ms":    stats.AverageEvalDuration.Milliseconds(),
	}

	json.NewEncoder(w).Encode(response)
}
