package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/quotas"
)

var quotaStore *quotas.Store
var quotaTracker *quotas.Tracker

// SetQuotaStore sets the quota store for handlers
func SetQuotaStore(store *quotas.Store, tracker *quotas.Tracker) {
	quotaStore = store
	quotaTracker = tracker
}

// RegisterQuotaRoutes registers quota API routes
func RegisterQuotaRoutes(mux *http.ServeMux) {
	// Team management
	mux.HandleFunc("/api/quotas/teams", handleQuotaTeams)
	mux.HandleFunc("/api/quotas/teams/", handleQuotaTeam)

	// Quota management
	mux.HandleFunc("/api/quotas", handleQuotas)
	mux.HandleFunc("/api/quotas/", handleQuota)

	// Usage and status
	mux.HandleFunc("/api/quotas/usage", handleQuotaUsage)
	mux.HandleFunc("/api/quotas/status", handleQuotaStatus)
	mux.HandleFunc("/api/quotas/violations", handleQuotaViolations)

	// Chargeback
	mux.HandleFunc("/api/chargeback/report", handleChargebackReport)
	mux.HandleFunc("/api/chargeback/summary", handleChargebackSummary)
	mux.HandleFunc("/api/chargeback/export", handleChargebackExport)

	// Team summary
	mux.HandleFunc("/api/quotas/team-summary", handleTeamSummary)
}

// handleQuotaTeams handles team CRUD
// GET /api/quotas/teams - List teams
// POST /api/quotas/teams - Create team
func handleQuotaTeams(w http.ResponseWriter, r *http.Request) {
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		orgID := r.URL.Query().Get("org_id")
		teams, err := quotaStore.ListTeams(orgID)
		if err != nil {
			http.Error(w, "Failed to list teams: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(teams),
			"teams": teams,
		})

	case http.MethodPost:
		var team quotas.Team
		if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if team.ID == "" {
			team.ID = fmt.Sprintf("team-%d", time.Now().UnixNano())
		}
		if team.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if team.OrgID == "" {
			team.OrgID = "default"
		}

		if err := quotaStore.CreateTeam(&team); err != nil {
			http.Error(w, "Failed to create team: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(team)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQuotaTeam handles individual team operations
// GET /api/quotas/teams/{id} - Get team
// PUT /api/quotas/teams/{id} - Update team
// DELETE /api/quotas/teams/{id} - Delete team
func handleQuotaTeam(w http.ResponseWriter, r *http.Request) {
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/quotas/teams/")
	if id == "" {
		http.Error(w, "Team ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		team, err := quotaStore.GetTeam(id)
		if err != nil {
			http.Error(w, "Failed to get team: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if team == nil {
			http.Error(w, "Team not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(team)

	case http.MethodPut:
		var team quotas.Team
		if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		team.ID = id

		if err := quotaStore.UpdateTeam(&team); err != nil {
			http.Error(w, "Failed to update team: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(team)

	case http.MethodDelete:
		if err := quotaStore.DeleteTeam(id); err != nil {
			http.Error(w, "Failed to delete team: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQuotas handles quota CRUD
// GET /api/quotas - List quotas
// POST /api/quotas - Create quota
func handleQuotas(w http.ResponseWriter, r *http.Request) {
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	// Avoid matching /api/quotas/... paths
	if r.URL.Path != "/api/quotas" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		teamID := r.URL.Query().Get("team_id")
		quotasList, err := quotaStore.ListQuotas(teamID)
		if err != nil {
			http.Error(w, "Failed to list quotas: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count":  len(quotasList),
			"quotas": quotasList,
		})

	case http.MethodPost:
		var quota quotas.Quota
		if err := json.NewDecoder(r.Body).Decode(&quota); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if quota.ID == "" {
			quota.ID = fmt.Sprintf("quota-%d", time.Now().UnixNano())
		}
		if quota.TeamID == "" {
			http.Error(w, "team_id is required", http.StatusBadRequest)
			return
		}
		if quota.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if quota.ResourceType == "" {
			http.Error(w, "resource_type is required", http.StatusBadRequest)
			return
		}
		if quota.Limit <= 0 {
			http.Error(w, "limit must be positive", http.StatusBadRequest)
			return
		}

		// Set defaults
		if quota.WarnAt == 0 {
			quota.WarnAt = 0.8
		}
		if quota.Enforcement == "" {
			quota.Enforcement = quotas.ActionWarn
		}
		if quota.Period == "" {
			quota.Period = "monthly"
		}
		if quota.Unit == "" {
			quota.Unit = quotas.UnitEvents
		}
		quota.Enabled = true

		if err := quotaStore.CreateQuota(&quota); err != nil {
			http.Error(w, "Failed to create quota: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(quota)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQuota handles individual quota operations
func handleQuota(w http.ResponseWriter, r *http.Request) {
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse path for sub-routes
	path := strings.TrimPrefix(r.URL.Path, "/api/quotas/")
	parts := strings.Split(path, "/")

	// Handle special endpoints
	switch parts[0] {
	case "teams":
		handleQuotaTeam(w, r)
		return
	case "usage", "status", "violations", "team-summary":
		return // Handled by specific handlers
	}

	id := parts[0]
	if id == "" {
		http.Error(w, "Quota ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		quota, err := quotaStore.GetQuota(id)
		if err != nil {
			http.Error(w, "Failed to get quota: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if quota == nil {
			http.Error(w, "Quota not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(quota)

	case http.MethodPut:
		var quota quotas.Quota
		if err := json.NewDecoder(r.Body).Decode(&quota); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		quota.ID = id

		if err := quotaStore.UpdateQuota(&quota); err != nil {
			http.Error(w, "Failed to update quota: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(quota)

	case http.MethodDelete:
		if err := quotaStore.DeleteQuota(id); err != nil {
			http.Error(w, "Failed to delete quota: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleQuotaUsage returns current usage for a team
// GET /api/quotas/usage?team_id=X&period=monthly
func handleQuotaUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	teamID := r.URL.Query().Get("team_id")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "monthly"
	}

	usages, err := quotaStore.GetTeamUsageSummary(teamID, period)
	if err != nil {
		http.Error(w, "Failed to get usage: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get service breakdown
	services, _ := quotaStore.GetUsageByService(teamID, period)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"team_id":  teamID,
		"period":   period,
		"usage":    usages,
		"services": services,
	})
}

// handleQuotaStatus returns quota status for a team
// GET /api/quotas/status?team_id=X
func handleQuotaStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaTracker == nil {
		http.Error(w, "Quota tracker not configured", http.StatusServiceUnavailable)
		return
	}

	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		http.Error(w, "team_id is required", http.StatusBadRequest)
		return
	}

	statuses := quotaTracker.GetStatus(teamID)

	// Count by status
	statusCounts := map[string]int{"ok": 0, "warning": 0, "exceeded": 0}
	for _, s := range statuses {
		statusCounts[s.Status]++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"team_id":  teamID,
		"statuses": statuses,
		"summary":  statusCounts,
	})
}

// handleQuotaViolations returns recent quota violations
// GET /api/quotas/violations?team_id=X&limit=50
func handleQuotaViolations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	teamID := r.URL.Query().Get("team_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	violations, err := quotaStore.GetViolations(teamID, limit)
	if err != nil {
		http.Error(w, "Failed to get violations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":      len(violations),
		"violations": violations,
	})
}

// handleChargebackReport returns a chargeback report for a team
// GET /api/chargeback/report?team_id=X&period=monthly
func handleChargebackReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		http.Error(w, "team_id is required", http.StatusBadRequest)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "monthly"
	}

	calculator := quotas.NewChargebackCalculator(quotaStore, quotas.DefaultPricing())
	start, end := quotas.GetPeriodBounds(period, time.Now())

	report, err := calculator.Calculate(teamID, period, start, end)
	if err != nil {
		http.Error(w, "Failed to calculate chargeback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// handleChargebackSummary returns org-wide chargeback summary
// GET /api/chargeback/summary?org_id=X
func handleChargebackSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	orgID := r.URL.Query().Get("org_id")

	calculator := quotas.NewChargebackCalculator(quotaStore, quotas.DefaultPricing())
	summary, err := calculator.GetOrgSummary(orgID)
	if err != nil {
		http.Error(w, "Failed to get summary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleChargebackExport exports chargeback data as CSV
// GET /api/chargeback/export?org_id=X&period=monthly
func handleChargebackExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaStore == nil {
		http.Error(w, "Quota system not configured", http.StatusServiceUnavailable)
		return
	}

	orgID := r.URL.Query().Get("org_id")
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "monthly"
	}

	calculator := quotas.NewChargebackCalculator(quotaStore, quotas.DefaultPricing())
	start, end := quotas.GetPeriodBounds(period, time.Now())

	csv, err := calculator.GenerateCSV(orgID, period, start, end)
	if err != nil {
		http.Error(w, "Failed to generate CSV: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("chargeback-%s-%s.csv", period, start.Format("2006-01"))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(csv)
}

// handleTeamSummary returns a complete summary for a team
// GET /api/quotas/team-summary?team_id=X
func handleTeamSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if quotaTracker == nil {
		http.Error(w, "Quota tracker not configured", http.StatusServiceUnavailable)
		return
	}

	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		http.Error(w, "team_id is required", http.StatusBadRequest)
		return
	}

	summary, err := quotaTracker.GetTeamSummary(teamID)
	if err != nil {
		http.Error(w, "Failed to get summary: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// RecordTeamUsage is called by ingest handlers to track usage per team
// This is a helper function for integration
func RecordTeamUsage(teamID string, resourceType quotas.ResourceType, amount int64, unit quotas.QuotaUnit, source string) (bool, quotas.EnforcementAction) {
	if quotaTracker == nil {
		return true, quotas.ActionWarn
	}
	return quotaTracker.Track(teamID, resourceType, amount, unit, source)
}
