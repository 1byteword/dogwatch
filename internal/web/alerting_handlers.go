package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/alerting"

	"github.com/google/uuid"
)

// alertManager holds the alert manager instance (set via SetAlertManager)
var serverAlertManager *alerting.AlertManager

// SetAlertManager sets the alert manager for the handlers
func SetAlertManager(am *alerting.AlertManager) {
	serverAlertManager = am
}

// RegisterAlertingRoutes registers alerting API routes
func RegisterAlertingRoutes(mux *http.ServeMux) {
	// Rules
	mux.HandleFunc("/api/alerting/rules", handleAlertRules)
	mux.HandleFunc("/api/alerting/rules/", handleAlertRule)

	// Alerts
	mux.HandleFunc("/api/alerting/alerts", handleAlerts)
	mux.HandleFunc("/api/alerting/alerts/", handleAlert)

	// Silences
	mux.HandleFunc("/api/alerting/silences", handleSilences)
	mux.HandleFunc("/api/alerting/silences/", handleSilence)

	// Inhibitions
	mux.HandleFunc("/api/alerting/inhibitions", handleInhibitions)
	mux.HandleFunc("/api/alerting/inhibitions/", handleInhibition)

	// Groups
	mux.HandleFunc("/api/alerting/groups", handleAlertGroups)

	// Status
	mux.HandleFunc("/api/alerting/status", handleAlertingStatus)

	// Dependency-aware alerting
	mux.HandleFunc("/api/alerting/blast-radius/", handleBlastRadius)
	mux.HandleFunc("/api/alerting/dependency-context/", handleDependencyContext)
}

// handleAlertRules handles GET/POST for alert rules
func handleAlertRules(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules, err := serverAlertManager.Store.ListRules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(rules)

	case http.MethodPost:
		var rule alerting.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}
		if rule.Labels == nil {
			rule.Labels = make(map[string]string)
		}
		if rule.Annotations == nil {
			rule.Annotations = make(map[string]string)
		}

		if err := serverAlertManager.Store.CreateRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAlertRule handles GET/PUT/DELETE for a specific rule
func handleAlertRule(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract rule ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/rules/")
	ruleID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodGet:
		rule, err := serverAlertManager.Store.GetRule(ruleID)
		if err != nil {
			http.Error(w, "rule not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var rule alerting.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rule.ID = ruleID

		if err := serverAlertManager.Store.UpdateRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(rule)

	case http.MethodDelete:
		if err := serverAlertManager.Store.DeleteRule(ruleID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAlerts handles GET for alerts
func handleAlerts(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Filter by state
	state := r.URL.Query().Get("state")
	var alerts []alerting.Alert
	var err error

	if state != "" {
		alerts, err = serverAlertManager.Store.ListAlerts(alerting.AlertState(state))
	} else {
		alerts, err = serverAlertManager.Store.ListAlerts("")
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(alerts)
}

// handleAlert handles GET/POST for a specific alert
func handleAlert(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract alert ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/alerts/")
	parts := strings.Split(path, "/")
	alertID := parts[0]

	// Check for action (acknowledge, silence)
	if len(parts) > 1 {
		action := parts[1]
		switch action {
		case "acknowledge":
			handleAlertAcknowledge(w, r, alertID)
			return
		case "silence":
			handleAlertSilence(w, r, alertID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		alert, err := serverAlertManager.Store.GetAlert(alertID)
		if err != nil {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(alert)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAlertAcknowledge acknowledges an alert
func handleAlertAcknowledge(w http.ResponseWriter, r *http.Request, alertID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.UserID = "api"
	}

	if err := serverAlertManager.Evaluator.AcknowledgeAlert(alertID, req.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
}

// handleAlertSilence creates a silence for an alert
func handleAlertSilence(w http.ResponseWriter, r *http.Request, alertID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	alert, err := serverAlertManager.Store.GetAlert(alertID)
	if err != nil {
		http.Error(w, "alert not found", http.StatusNotFound)
		return
	}

	var req struct {
		Duration  string `json:"duration"`
		Comment   string `json:"comment"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	duration := time.Hour
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err == nil {
			duration = d
		}
	}

	// Create silence from alert labels
	matchers := make([]alerting.Matcher, 0)
	for k, v := range alert.Labels {
		matchers = append(matchers, alerting.Matcher{
			Name:    k,
			Value:   v,
			IsRegex: false,
			IsEqual: true,
		})
	}

	silence := &alerting.Silence{
		ID:        uuid.New().String(),
		Matchers:  matchers,
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(duration),
		CreatedBy: req.CreatedBy,
		Comment:   req.Comment,
	}

	if err := serverAlertManager.Store.CreateSilence(silence); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(silence)
}

// handleSilences handles GET/POST for silences
func handleSilences(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		showAll := r.URL.Query().Get("all") == "true"
		var silences interface{}
		var err error

		if showAll {
			silences, err = serverAlertManager.Store.ListSilencesWithState()
		} else {
			silences, err = serverAlertManager.Store.ListActiveSilences()
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(silences)

	case http.MethodPost:
		var silence alerting.Silence
		if err := json.NewDecoder(r.Body).Decode(&silence); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if silence.ID == "" {
			silence.ID = uuid.New().String()
		}
		if silence.StartsAt.IsZero() {
			silence.StartsAt = time.Now()
		}
		if silence.EndsAt.IsZero() {
			silence.EndsAt = time.Now().Add(time.Hour)
		}

		if err := serverAlertManager.Store.CreateSilence(&silence); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(silence)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSilence handles GET/PUT/DELETE for a specific silence
func handleSilence(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract silence ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/silences/")
	parts := strings.Split(path, "/")
	silenceID := parts[0]

	// Check for expire action
	if len(parts) > 1 && parts[1] == "expire" {
		handleSilenceExpire(w, r, silenceID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		silence, err := serverAlertManager.Store.GetSilence(silenceID)
		if err != nil {
			http.Error(w, "silence not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(silence)

	case http.MethodPut:
		var silence alerting.Silence
		if err := json.NewDecoder(r.Body).Decode(&silence); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		silence.ID = silenceID

		if err := serverAlertManager.Store.UpdateSilence(&silence); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(silence)

	case http.MethodDelete:
		if err := serverAlertManager.Store.DeleteSilence(silenceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSilenceExpire immediately expires a silence
func handleSilenceExpire(w http.ResponseWriter, r *http.Request, silenceID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := serverAlertManager.Store.ExpireSilence(silenceID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "expired"})
}

// handleInhibitions handles GET/POST for inhibition rules
func handleInhibitions(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		rules, err := serverAlertManager.Store.ListInhibitRules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(rules)

	case http.MethodPost:
		var rule alerting.InhibitRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}

		if err := serverAlertManager.Store.CreateInhibitRule(&rule); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Reload rules in inhibitor
		serverAlertManager.Inhibitor.LoadRulesFromDB()

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(rule)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleInhibition handles GET/DELETE for a specific inhibition rule
func handleInhibition(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract rule ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/inhibitions/")
	ruleID := strings.Split(path, "/")[0]

	switch r.Method {
	case http.MethodDelete:
		if err := serverAlertManager.Store.DeleteInhibitRule(ruleID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Reload rules in inhibitor
		serverAlertManager.Inhibitor.LoadRulesFromDB()

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAlertGroups returns alert groups
func handleAlertGroups(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groups := serverAlertManager.Router.GetGroups()
	json.NewEncoder(w).Encode(groups)
}

// handleAlertingStatus returns alerting system status
func handleAlertingStatus(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get counts
	rules, _ := serverAlertManager.Store.ListRules()
	enabledRules, _ := serverAlertManager.Store.ListEnabledRules()
	firingAlerts := serverAlertManager.Evaluator.GetFiringAlerts()
	pendingAlerts := serverAlertManager.Evaluator.GetPendingAlerts()
	activeSilences, _ := serverAlertManager.Store.ListActiveSilences()
	inhibitRules, _ := serverAlertManager.Store.ListInhibitRules()
	groups := serverAlertManager.Router.GetGroups()

	status := map[string]interface{}{
		"total_rules":              len(rules),
		"enabled_rules":            len(enabledRules),
		"firing_alerts":            len(firingAlerts),
		"pending_alerts":           len(pendingAlerts),
		"active_silences":          len(activeSilences),
		"inhibition_rules":         len(inhibitRules),
		"alert_groups":             len(groups),
		"uptime":                   "running",
		"dependency_alerting":      serverAlertManager.DependencyAlerting != nil,
	}

	json.NewEncoder(w).Encode(status)
}

// handleBlastRadius returns the blast radius for a service
func handleBlastRadius(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service ID from path: /api/alerting/blast-radius/{service_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/blast-radius/")
	serviceID := strings.TrimSuffix(path, "/")

	if serviceID == "" {
		http.Error(w, "service ID required", http.StatusBadRequest)
		return
	}

	blastRadius := serverAlertManager.GetBlastRadius(serviceID)
	if blastRadius == nil {
		// Return empty blast radius if dependency alerting not enabled
		blastRadius = &alerting.BlastRadius{
			FailedService:    serviceID,
			AffectedServices: []alerting.AffectedService{},
			TotalAffected:    0,
			EstimatedImpact:  "unknown",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(blastRadius)
}

// handleDependencyContext returns the dependency context for an alert
func handleDependencyContext(w http.ResponseWriter, r *http.Request) {
	if serverAlertManager == nil {
		http.Error(w, "alerting not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract alert ID from path: /api/alerting/dependency-context/{alert_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/alerting/dependency-context/")
	alertID := strings.TrimSuffix(path, "/")

	if alertID == "" {
		http.Error(w, "alert ID required", http.StatusBadRequest)
		return
	}

	// Find the alert
	var targetAlert *alerting.Alert
	for _, alert := range serverAlertManager.Evaluator.GetFiringAlerts() {
		if alert.ID == alertID {
			targetAlert = alert
			break
		}
	}

	if targetAlert == nil {
		// Check pending alerts too
		for _, alert := range serverAlertManager.Evaluator.GetPendingAlerts() {
			if alert.ID == alertID {
				targetAlert = alert
				break
			}
		}
	}

	if targetAlert == nil {
		http.Error(w, "alert not found", http.StatusNotFound)
		return
	}

	ctx := serverAlertManager.GetDependencyContext(targetAlert)
	if ctx == nil {
		// Return empty context if dependency alerting not enabled
		ctx = &alerting.DependencyContext{
			UpstreamServices: []alerting.ServiceAlertState{},
			IsLikelySymptom:  false,
			ShouldSuppress:   false,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ctx)
}
