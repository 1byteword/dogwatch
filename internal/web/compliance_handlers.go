package web

import (
	"encoding/json"
	"net/http"
	"time"

	"dogwatch/internal/compliance"
)

// Package-level variables for compliance handlers
var (
	complianceStore     *compliance.Store
	complianceGenerator *compliance.ReportGenerator
)

// SetComplianceStore sets the compliance store and generator for handlers
func SetComplianceStore(store *compliance.Store, generator *compliance.ReportGenerator) {
	complianceStore = store
	complianceGenerator = generator
}

// RegisterComplianceRoutes registers compliance API routes at the package level
func RegisterComplianceRoutes(mux *http.ServeMux) {
	if complianceStore == nil {
		return
	}
	handlers := NewComplianceHandlers(complianceStore, complianceGenerator)
	handlers.RegisterComplianceRoutes(mux)
}

// ComplianceHandlers handles compliance-related HTTP requests
type ComplianceHandlers struct {
	store     *compliance.Store
	generator *compliance.ReportGenerator
}

// NewComplianceHandlers creates new compliance handlers
func NewComplianceHandlers(store *compliance.Store, generator *compliance.ReportGenerator) *ComplianceHandlers {
	return &ComplianceHandlers{
		store:     store,
		generator: generator,
	}
}

// RegisterComplianceRoutes registers compliance routes
func (h *ComplianceHandlers) RegisterComplianceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/compliance/reports", h.ListReports)
	mux.HandleFunc("POST /api/compliance/reports", h.GenerateReport)
	mux.HandleFunc("GET /api/compliance/reports/{id}", h.GetReport)
	mux.HandleFunc("DELETE /api/compliance/reports/{id}", h.DeleteReport)
	mux.HandleFunc("PUT /api/compliance/reports/{id}/status", h.UpdateReportStatus)
	mux.HandleFunc("GET /api/compliance/reports/{id}/evidence", h.GetReportEvidence)
	mux.HandleFunc("GET /api/compliance/reports/{id}/gaps", h.GetGapAnalysis)
	mux.HandleFunc("GET /api/compliance/controls", h.ListControls)
	mux.HandleFunc("GET /api/compliance/controls/{id}", h.GetControlStatus)
	mux.HandleFunc("GET /api/compliance/findings", h.ListFindings)
	mux.HandleFunc("PUT /api/compliance/findings/{id}", h.UpdateFinding)
	mux.HandleFunc("GET /api/compliance/schedules", h.ListSchedules)
	mux.HandleFunc("POST /api/compliance/schedules", h.CreateSchedule)
	mux.HandleFunc("DELETE /api/compliance/schedules/{id}", h.DeleteSchedule)
	mux.HandleFunc("GET /api/compliance/summary", h.GetComplianceSummary)
}

// ListReports lists all compliance reports
func (h *ComplianceHandlers) ListReports(w http.ResponseWriter, r *http.Request) {
	filter := compliance.ReportFilter{
		Limit: 50,
	}

	if t := r.URL.Query().Get("type"); t != "" {
		filter.Type = compliance.ReportType(t)
	}
	if s := r.URL.Query().Get("status"); s != "" {
		filter.Status = compliance.ReportStatus(s)
	}

	reports, err := h.store.ListReports(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if reports == nil {
		reports = []compliance.ComplianceReport{}
	}

	// Return summary only, not full report JSON
	summaries := make([]map[string]interface{}, len(reports))
	for i, report := range reports {
		summaries[i] = map[string]interface{}{
			"id":               report.ID,
			"type":             report.Type,
			"title":            report.Title,
			"period":           report.Period,
			"generated_at":     report.GeneratedAt,
			"generated_by":     report.GeneratedBy,
			"status":           report.Status,
			"compliance_score": report.Summary.ComplianceScore,
			"open_findings":    report.Summary.OpenFindings,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}

// GenerateReport generates a new compliance report
func (h *ComplianceHandlers) GenerateReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type       string `json:"type"`
		OrgID      string `json:"org_id"`
		PeriodDays int    `json:"period_days"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		req.Type = "SOC2"
	}
	if req.PeriodDays <= 0 {
		req.PeriodDays = 90
	}

	period := compliance.DateRange{
		Start: time.Now().AddDate(0, 0, -req.PeriodDays),
		End:   time.Now(),
	}

	// Get user from context (simplified)
	generatedBy := "system"

	var report *compliance.ComplianceReport
	var err error

	switch compliance.ReportType(req.Type) {
	case compliance.ReportTypeSOC2:
		report, err = h.generator.GenerateSOC2Report(req.OrgID, period, generatedBy)
	default:
		http.Error(w, "unsupported report type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(report)
}

// GetReport retrieves a compliance report
func (h *ComplianceHandlers) GetReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	report, err := h.store.GetReport(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// DeleteReport deletes a compliance report
func (h *ComplianceHandlers) DeleteReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := h.store.DeleteReport(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateReportStatus updates a report's status
func (h *ComplianceHandlers) UpdateReportStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	status := compliance.ReportStatus(req.Status)
	if status != compliance.StatusDraft && status != compliance.StatusFinal && status != compliance.StatusArchived {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateReportStatus(id, status); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": string(status)})
}

// GetReportEvidence retrieves evidence for a report
func (h *ComplianceHandlers) GetReportEvidence(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	filter := compliance.EvidenceFilter{
		ReportID: id,
		Limit:    1000,
	}

	if controlID := r.URL.Query().Get("control_id"); controlID != "" {
		filter.ControlID = controlID
	}

	evidence, err := h.store.ListEvidence(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if evidence == nil {
		evidence = []compliance.Evidence{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evidence)
}

// GetGapAnalysis generates and returns gap analysis for a report
func (h *ComplianceHandlers) GetGapAnalysis(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	analysis, err := h.generator.GenerateGapAnalysis(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

// ListControls lists all SOC2 controls
func (h *ComplianceHandlers) ListControls(w http.ResponseWriter, r *http.Request) {
	controls := compliance.SOC2Controls
	categories := compliance.GetSOC2Categories()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"controls":   controls,
		"categories": categories,
	})
}

// GetControlStatus returns the status of a specific control
func (h *ComplianceHandlers) GetControlStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	// Get control definition
	control := compliance.GetSOC2Control(id)
	if control == nil {
		http.Error(w, "control not found", http.StatusNotFound)
		return
	}

	// Get latest status from reports
	section, err := h.store.GetControlStatus(id)

	response := map[string]interface{}{
		"control": control,
	}

	if err == nil && section != nil {
		response["status"] = section.Status
		response["last_tested"] = section.LastTested
		response["findings_count"] = len(section.Findings)
	} else {
		response["status"] = "not_evaluated"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListFindings lists compliance findings
func (h *ComplianceHandlers) ListFindings(w http.ResponseWriter, r *http.Request) {
	reportID := r.URL.Query().Get("report_id")
	status := r.URL.Query().Get("status")

	findings, err := h.store.ListFindings(reportID, status)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if findings == nil {
		findings = []compliance.Finding{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(findings)
}

// UpdateFinding updates a finding
func (h *ComplianceHandlers) UpdateFinding(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var finding compliance.Finding
	if err := json.NewDecoder(r.Body).Decode(&finding); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	finding.ID = id

	if err := h.store.UpdateFinding(&finding); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(finding)
}

// ListSchedules lists scheduled reports
func (h *ComplianceHandlers) ListSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.store.ListSchedules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if schedules == nil {
		schedules = []compliance.ScheduledReport{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schedules)
}

// CreateSchedule creates a scheduled report
func (h *ComplianceHandlers) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule compliance.ScheduledReport
	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if schedule.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if schedule.Schedule == "" {
		http.Error(w, "schedule is required", http.StatusBadRequest)
		return
	}

	schedule.CreatedBy = "system" // Simplified

	if err := h.store.SaveSchedule(&schedule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(schedule)
}

// DeleteSchedule deletes a scheduled report
func (h *ComplianceHandlers) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	if err := h.store.DeleteSchedule(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetComplianceSummary returns an overall compliance summary
func (h *ComplianceHandlers) GetComplianceSummary(w http.ResponseWriter, r *http.Request) {
	// Get most recent report
	reports, err := h.store.ListReports(compliance.ReportFilter{Limit: 1})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"total_controls": len(compliance.SOC2Controls),
		"categories":     compliance.GetSOC2Categories(),
	}

	if len(reports) > 0 {
		report := reports[0]
		response["latest_report"] = map[string]interface{}{
			"id":               report.ID,
			"type":             report.Type,
			"generated_at":     report.GeneratedAt,
			"status":           report.Status,
			"compliance_score": report.Summary.ComplianceScore,
			"risk_score":       report.Summary.RiskScore,
		}
		response["summary"] = report.Summary
	} else {
		response["latest_report"] = nil
		response["summary"] = compliance.ComplianceSummary{
			TotalControls: len(compliance.SOC2Controls),
		}
	}

	// Get open findings count
	findings, _ := h.store.ListFindings("", "open")
	response["open_findings"] = len(findings)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
