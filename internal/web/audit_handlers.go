package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/audit"
	"dogwatch/internal/rbac"
)

var auditStore *audit.Store

// SetAuditStore sets the audit store for handlers
func SetAuditStore(store *audit.Store) {
	auditStore = store
}

// GetAuditStore returns the audit store
func GetAuditStore() *audit.Store {
	return auditStore
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
