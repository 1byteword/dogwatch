package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dogwatch/internal/audit"
	"dogwatch/internal/rbac"
)

var auditStore *audit.Store
var auditLogger *audit.Logger
var queryAuditHook *audit.QueryAuditHook

// SetAuditStore sets the audit store for handlers
func SetAuditStore(store *audit.Store) {
	auditStore = store
}

// GetAuditStore returns the audit store
func GetAuditStore() *audit.Store {
	return auditStore
}

// SetAuditLogger sets the audit logger for async logging
func SetAuditLogger(logger *audit.Logger) {
	auditLogger = logger
}

// GetAuditLogger returns the audit logger
func GetAuditLogger() *audit.Logger {
	return auditLogger
}

// SetQueryAuditHook sets the query audit hook for query execution tracking
func SetQueryAuditHook(hook *audit.QueryAuditHook) {
	queryAuditHook = hook
}

// GetQueryAuditHook returns the query audit hook
func GetQueryAuditHook() *audit.QueryAuditHook {
	return queryAuditHook
}

// handleAuditLogs handles /api/audit/logs
func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	opts := audit.QueryOptions{
		OrgID: user.OrgID,
	}

	if v := r.URL.Query().Get("user_id"); v != "" {
		opts.UserID = v
	}
	if v := r.URL.Query().Get("action"); v != "" {
		opts.Action = audit.ActionType(v)
	}
	if v := r.URL.Query().Get("resource_type"); v != "" {
		opts.ResourceType = audit.ResourceType(v)
	}
	if v := r.URL.Query().Get("resource_id"); v != "" {
		opts.ResourceID = v
	}
	if v := r.URL.Query().Get("outcome"); v != "" {
		opts.Outcome = audit.Outcome(v)
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.EndTime = &t
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			opts.Offset = n
		}
	}

	logs, err := auditStore.List(opts)
	if err != nil {
		http.Error(w, `{"error":"failed to list audit logs"}`, http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []*audit.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// handleAuditStats handles /api/audit/stats
func (s *Server) handleAuditStats(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	stats, err := auditStore.GetStats(user.OrgID)
	if err != nil {
		http.Error(w, `{"error":"failed to get audit stats"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleAuditExport handles /api/audit/export
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse time range
	opts := audit.QueryOptions{
		OrgID: user.OrgID,
		Limit: 10000, // Max export size
	}

	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			opts.EndTime = &t
		}
	}

	logs, err := auditStore.List(opts)
	if err != nil {
		http.Error(w, `{"error":"failed to export audit logs"}`, http.StatusInternalServerError)
		return
	}

	// Log the export action
	audit.LogAction(r.Context(), auditStore, audit.ActionExport, audit.ResourceSettings, "", "audit_logs", map[string]interface{}{
		"count": len(logs),
	})

	// Return as JSON download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit-logs.json")
	json.NewEncoder(w).Encode(logs)
}

// AuditLogResponse wraps audit logs with metadata
type AuditLogResponse struct {
	Logs       []*audit.AuditLog `json:"logs"`
	Total      int64             `json:"total"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
	HasMore    bool              `json:"has_more"`
}

// handleAuditLogsPaginated handles /api/audit/logs/paginated
func (s *Server) handleAuditLogsPaginated(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	opts := audit.QueryOptions{
		OrgID: user.OrgID,
		Limit: 50,
	}

	// Parse query parameters
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			opts.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			opts.Offset = n
		}
	}
	if v := r.URL.Query().Get("user_id"); v != "" {
		opts.UserID = v
	}
	if v := r.URL.Query().Get("action"); v != "" {
		opts.Action = audit.ActionType(v)
	}
	if v := r.URL.Query().Get("resource_type"); v != "" {
		opts.ResourceType = audit.ResourceType(v)
	}

	// Get logs and count
	logs, err := auditStore.List(opts)
	if err != nil {
		http.Error(w, `{"error":"failed to list audit logs"}`, http.StatusInternalServerError)
		return
	}

	total, _ := auditStore.Count(opts)

	resp := AuditLogResponse{
		Logs:    logs,
		Total:   total,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
		HasMore: int64(opts.Offset+len(logs)) < total,
	}

	if resp.Logs == nil {
		resp.Logs = []*audit.AuditLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// QueryAuditResponse wraps query audit entries with metadata
type QueryAuditResponse struct {
	Entries []*audit.QueryAuditEntry `json:"entries"`
	Total   int64                    `json:"total"`
	Limit   int                      `json:"limit"`
	Offset  int                      `json:"offset"`
	HasMore bool                     `json:"has_more"`
}

// handleQueryAudit handles /api/audit/queries
func (s *Server) handleQueryAudit(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	filter := audit.QueryAuditFilter{
		OrgID: user.OrgID,
		Limit: 50,
	}

	// Parse query parameters
	if v := r.URL.Query().Get("user_id"); v != "" {
		filter.UserID = v
	}
	if v := r.URL.Query().Get("username"); v != "" {
		filter.Username = v
	}
	if v := r.URL.Query().Get("query_type"); v != "" {
		filter.QueryType = audit.QueryType(v)
	}
	if v := r.URL.Query().Get("data_source"); v != "" {
		filter.DataSource = v
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.EndTime = &t
		}
	}
	if v := r.URL.Query().Get("success"); v != "" {
		success := v == "true" || v == "1"
		filter.SuccessOnly = &success
	}
	if v := r.URL.Query().Get("pii"); v != "" {
		pii := v == "true" || v == "1"
		filter.AccessedPII = &pii
	}
	if v := r.URL.Query().Get("service"); v != "" {
		filter.ServiceName = v
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	entries, err := auditStore.ListQueryAudit(filter)
	if err != nil {
		http.Error(w, `{"error":"failed to list query audit entries"}`, http.StatusInternalServerError)
		return
	}

	total, _ := auditStore.CountQueryAudit(filter)

	resp := QueryAuditResponse{
		Entries: entries,
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		HasMore: int64(filter.Offset+len(entries)) < total,
	}

	if resp.Entries == nil {
		resp.Entries = []*audit.QueryAuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleQueryAuditDetail handles /api/audit/queries/:id
func (s *Server) handleQueryAuditDetail(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from path
	id := strings.TrimPrefix(r.URL.Path, "/api/audit/queries/")
	if id == "" {
		http.Error(w, `{"error":"query id required"}`, http.StatusBadRequest)
		return
	}

	entry, err := auditStore.GetQueryAudit(id)
	if err != nil {
		http.Error(w, `{"error":"failed to get query audit entry"}`, http.StatusInternalServerError)
		return
	}
	if entry == nil {
		http.Error(w, `{"error":"query audit entry not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

// handleUserQueryAudit handles /api/audit/users/:id/queries
func (s *Server) handleUserQueryAudit(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract user ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/audit/users/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "queries" {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}
	userID := parts[0]

	filter := audit.QueryAuditFilter{
		UserID: userID,
		OrgID:  user.OrgID,
		Limit:  50,
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	entries, err := auditStore.ListQueryAudit(filter)
	if err != nil {
		http.Error(w, `{"error":"failed to list user query audit entries"}`, http.StatusInternalServerError)
		return
	}

	total, _ := auditStore.CountQueryAudit(filter)

	resp := QueryAuditResponse{
		Entries: entries,
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		HasMore: int64(filter.Offset+len(entries)) < total,
	}

	if resp.Entries == nil {
		resp.Entries = []*audit.QueryAuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAdminAudit handles /api/audit/admin
func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	filter := audit.AdminAuditFilter{
		OrgID: user.OrgID,
		Limit: 50,
	}

	if v := r.URL.Query().Get("user_id"); v != "" {
		filter.UserID = v
	}
	if v := r.URL.Query().Get("action_type"); v != "" {
		filter.ActionType = v
	}
	if v := r.URL.Query().Get("resource_type"); v != "" {
		filter.ResourceType = v
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.EndTime = &t
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	entries, err := auditStore.ListAdminAudit(filter)
	if err != nil {
		http.Error(w, `{"error":"failed to list admin audit entries"}`, http.StatusInternalServerError)
		return
	}

	if entries == nil {
		entries = []*audit.AdminAuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handleAuthAudit handles /api/audit/auth
func (s *Server) handleAuthAudit(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	filter := audit.AuthAuditFilter{
		OrgID: user.OrgID,
		Limit: 50,
	}

	if v := r.URL.Query().Get("user_id"); v != "" {
		filter.UserID = v
	}
	if v := r.URL.Query().Get("email"); v != "" {
		filter.Email = v
	}
	if v := r.URL.Query().Get("event_type"); v != "" {
		filter.EventType = v
	}
	if v := r.URL.Query().Get("ip_address"); v != "" {
		filter.IPAddress = v
	}
	if v := r.URL.Query().Get("success"); v != "" {
		success := v == "true" || v == "1"
		filter.SuccessOnly = &success
	}
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.StartTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.EndTime = &t
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			filter.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filter.Offset = n
		}
	}

	entries, err := auditStore.ListAuthAudit(filter)
	if err != nil {
		http.Error(w, `{"error":"failed to list auth audit entries"}`, http.StatusInternalServerError)
		return
	}

	if entries == nil {
		entries = []*audit.AuthAuditEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// handleAuditSummary handles /api/audit/stats (enhanced stats)
func (s *Server) handleAuditSummary(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse period
	period := 24 * time.Hour // Default to 24 hours
	if v := r.URL.Query().Get("period"); v != "" {
		switch v {
		case "1h":
			period = time.Hour
		case "6h":
			period = 6 * time.Hour
		case "24h":
			period = 24 * time.Hour
		case "7d":
			period = 7 * 24 * time.Hour
		case "30d":
			period = 30 * 24 * time.Hour
		}
	}

	summary, err := auditStore.GetAuditSummary(user.OrgID, period)
	if err != nil {
		http.Error(w, `{"error":"failed to get audit summary"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// handleAuditExportFull handles /api/audit/export with format options
func (s *Server) handleAuditExportFull(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse parameters
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	auditType := r.URL.Query().Get("type")
	if auditType == "" {
		auditType = "all"
	}

	var startTime, endTime *time.Time
	if v := r.URL.Query().Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			startTime = &t
		}
	}
	if v := r.URL.Query().Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			endTime = &t
		}
	}

	// Collect data based on type
	var exportData interface{}
	var recordCount int64

	switch auditType {
	case "queries":
		filter := audit.QueryAuditFilter{
			OrgID:     user.OrgID,
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		}
		entries, _ := auditStore.ListQueryAudit(filter)
		exportData = entries
		recordCount = int64(len(entries))
	case "auth":
		filter := audit.AuthAuditFilter{
			OrgID:     user.OrgID,
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		}
		entries, _ := auditStore.ListAuthAudit(filter)
		exportData = entries
		recordCount = int64(len(entries))
	case "admin":
		filter := audit.AdminAuditFilter{
			OrgID:     user.OrgID,
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		}
		entries, _ := auditStore.ListAdminAudit(filter)
		exportData = entries
		recordCount = int64(len(entries))
	default:
		// Export all types
		opts := audit.QueryOptions{
			OrgID:     user.OrgID,
			StartTime: startTime,
			EndTime:   endTime,
			Limit:     10000,
		}
		logs, _ := auditStore.List(opts)
		exportData = logs
		recordCount = int64(len(logs))
	}

	// Log the export action
	timeRange := audit.TimeRange{}
	if startTime != nil {
		timeRange.Start = *startTime
	}
	if endTime != nil {
		timeRange.End = *endTime
	}
	audit.LogDataExport(r.Context(), auditStore, auditLogger, format, "audit_"+auditType, "",
		timeRange, recordCount, 0, false, nil, true, "", "")

	// Return in requested format
	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=audit-%s-%s.csv", auditType, time.Now().Format("2006-01-02")))
		writeCSVExport(w, auditType, exportData)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=audit-%s-%s.json", auditType, time.Now().Format("2006-01-02")))
		json.NewEncoder(w).Encode(exportData)
	}
}

// writeCSVExport writes audit data as CSV
func writeCSVExport(w http.ResponseWriter, auditType string, data interface{}) {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	switch auditType {
	case "queries":
		entries, ok := data.([]*audit.QueryAuditEntry)
		if !ok {
			return
		}
		writer.Write([]string{"ID", "Timestamp", "UserID", "Username", "IPAddress", "QueryType", "QueryText", "DataSource", "RowsReturned", "DurationMs", "Success", "AccessedPII"})
		for _, e := range entries {
			writer.Write([]string{
				e.ID,
				e.Timestamp.Format(time.RFC3339),
				e.UserID,
				e.Username,
				e.IPAddress,
				string(e.QueryType),
				e.QueryText,
				e.DataSource,
				strconv.FormatInt(e.RowsReturned, 10),
				fmt.Sprintf("%.2f", e.DurationMs),
				strconv.FormatBool(e.Success),
				strconv.FormatBool(e.AccessedPII),
			})
		}
	case "auth":
		entries, ok := data.([]*audit.AuthAuditEntry)
		if !ok {
			return
		}
		writer.Write([]string{"ID", "Timestamp", "EventType", "UserID", "Email", "IPAddress", "Success", "FailureCode", "Method", "Provider"})
		for _, e := range entries {
			writer.Write([]string{
				e.ID,
				e.Timestamp.Format(time.RFC3339),
				e.EventType,
				e.UserID,
				e.Email,
				e.IPAddress,
				strconv.FormatBool(e.Success),
				e.FailureCode,
				e.Method,
				e.Provider,
			})
		}
	case "admin":
		entries, ok := data.([]*audit.AdminAuditEntry)
		if !ok {
			return
		}
		writer.Write([]string{"ID", "Timestamp", "UserID", "Username", "ActionType", "ResourceType", "ResourceID", "Success"})
		for _, e := range entries {
			writer.Write([]string{
				e.ID,
				e.Timestamp.Format(time.RFC3339),
				e.UserID,
				e.Username,
				e.ActionType,
				e.ResourceType,
				e.ResourceID,
				strconv.FormatBool(e.Success),
			})
		}
	default:
		logs, ok := data.([]*audit.AuditLog)
		if !ok {
			return
		}
		writer.Write([]string{"ID", "Timestamp", "UserID", "UserEmail", "Action", "ResourceType", "ResourceID", "Outcome"})
		for _, l := range logs {
			writer.Write([]string{
				l.ID,
				l.Timestamp.Format(time.RFC3339),
				l.UserID,
				l.UserEmail,
				string(l.Action),
				string(l.ResourceType),
				l.ResourceID,
				string(l.Outcome),
			})
		}
	}
}

// handleAuditPurge handles /api/audit/purge
func (s *Server) handleAuditPurge(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require owner role for purge
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || user.Role != "owner" {
		http.Error(w, `{"error":"owner access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "DELETE" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Purge based on retention policy
	results, err := auditStore.PurgeOldRecords()
	if err != nil {
		http.Error(w, `{"error":"failed to purge old records"}`, http.StatusInternalServerError)
		return
	}

	// Log the purge action
	audit.LogAdminAction(r.Context(), auditStore, auditLogger, audit.AdminActionDataPurge, "audit", "", "audit_logs", nil, results, true, "")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"purged":  results,
	})
}

// handleAuditRetention handles /api/audit/retention
func (s *Server) handleAuditRetention(w http.ResponseWriter, r *http.Request) {
	if auditStore == nil {
		http.Error(w, `{"error":"audit logging not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	switch r.Method {
	case "GET":
		policy, err := auditStore.GetRetentionPolicy()
		if err != nil {
			http.Error(w, `{"error":"failed to get retention policy"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)

	case "PUT":
		// Require owner role to update
		if user.Role != "owner" {
			http.Error(w, `{"error":"owner access required to update retention policy"}`, http.StatusForbidden)
			return
		}

		var policy audit.RetentionPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		if err := auditStore.SetRetentionPolicy(&policy); err != nil {
			http.Error(w, `{"error":"failed to update retention policy"}`, http.StatusInternalServerError)
			return
		}

		// Log the policy change
		audit.LogAdminAction(r.Context(), auditStore, auditLogger, audit.AdminActionConfigUpdate, "retention_policy", "", "audit_retention", nil, policy, true, "")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleAuditLoggerStats handles /api/audit/logger/stats
func (s *Server) handleAuditLoggerStats(w http.ResponseWriter, r *http.Request) {
	// Require admin role
	user := rbac.GetUserFromContext(r.Context())
	if user == nil || (user.Role != "admin" && user.Role != "owner") {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	if r.Method != "GET" {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var stats audit.LoggerStats
	if auditLogger != nil {
		stats = auditLogger.GetStats()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// RegisterAuditRoutes registers all audit-related API routes
func RegisterAuditRoutes(mux *http.ServeMux, s *Server) {
	// Existing routes
	mux.HandleFunc("/api/audit/logs", s.handleAuditLogs)
	mux.HandleFunc("/api/audit/logs/paginated", s.handleAuditLogsPaginated)
	mux.HandleFunc("/api/audit/stats", s.handleAuditStats)
	mux.HandleFunc("/api/audit/export", s.handleAuditExport)

	// New query audit routes
	mux.HandleFunc("/api/audit/queries", s.handleQueryAudit)
	mux.HandleFunc("/api/audit/queries/", s.handleQueryAuditDetail)
	mux.HandleFunc("/api/audit/users/", s.handleUserQueryAudit)

	// New admin/auth audit routes
	mux.HandleFunc("/api/audit/admin", s.handleAdminAudit)
	mux.HandleFunc("/api/audit/auth", s.handleAuthAudit)

	// Enhanced stats/summary
	mux.HandleFunc("/api/audit/summary", s.handleAuditSummary)

	// Export with format options
	mux.HandleFunc("/api/audit/export/full", s.handleAuditExportFull)

	// Admin operations
	mux.HandleFunc("/api/audit/purge", s.handleAuditPurge)
	mux.HandleFunc("/api/audit/retention", s.handleAuditRetention)

	// Logger stats
	mux.HandleFunc("/api/audit/logger/stats", s.handleAuditLoggerStats)
}
