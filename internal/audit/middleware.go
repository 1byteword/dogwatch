package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"dogwatch/internal/rbac"
	"github.com/google/uuid"
)

// Middleware provides HTTP middleware for audit logging
type Middleware struct {
	store *Store
}

// NewMiddleware creates a new audit middleware
func NewMiddleware(store *Store) *Middleware {
	return &Middleware{store: store}
}

// contextKey is used for storing audit context
type contextKey string

const (
	auditContextKey contextKey = "audit_context"
	requestIDKey    contextKey = "request_id"
)

// AuditContext holds audit information for the current request
type AuditContext struct {
	RequestID    string
	UserID       string
	UserEmail    string
	OrgID        string
	SessionID    string
	UserIP       string
	UserAgent    string
	Method       string
	Path         string
	ResourceType ResourceType
	ResourceID   string
	Action       ActionType
}

// GetAuditContext retrieves audit context from request context
func GetAuditContext(ctx context.Context) *AuditContext {
	if ac, ok := ctx.Value(auditContextKey).(*AuditContext); ok {
		return ac
	}
	return nil
}

// WithAuditContext adds audit context to request context
func WithAuditContext(ctx context.Context, ac *AuditContext) context.Context {
	return context.WithValue(ctx, auditContextKey, ac)
}

// Wrap wraps an http.Handler with audit logging
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()[:8]
		}

		// Create audit context
		ac := &AuditContext{
			RequestID: requestID,
			UserIP:    getClientIP(r),
			UserAgent: r.UserAgent(),
			Method:    r.Method,
			Path:      r.URL.Path,
		}

		// Extract user info from RBAC context
		if user := rbac.GetUserFromContext(r.Context()); user != nil {
			ac.UserID = user.ID
			ac.UserEmail = user.Email
			ac.OrgID = user.OrgID
		}

		// Determine resource type and action from path and method
		ac.ResourceType, ac.ResourceID = parseResourceFromPath(r.URL.Path)
		ac.Action = methodToAction(r.Method)

		// Add to context
		ctx := WithAuditContext(r.Context(), ac)
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		// Wrap response writer to capture status
		rw := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		// Add request ID to response
		w.Header().Set("X-Request-ID", requestID)

		// Call next handler
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Log based on endpoint sensitivity
		if shouldAudit(r.URL.Path, r.Method) {
			go m.logRequest(ac, rw.statusCode, rw.body.Bytes())
		}
	})
}

// responseRecorder captures response status and body
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rw *responseRecorder) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseRecorder) Write(b []byte) (int, error) {
	// Capture body for audit (limited)
	if rw.body.Len() < 4096 {
		rw.body.Write(b)
	}
	return rw.ResponseWriter.Write(b)
}

// logRequest creates an audit log entry from the request
func (m *Middleware) logRequest(ac *AuditContext, statusCode int, body []byte) {
	outcome := OutcomeSuccess
	var errorMsg string

	if statusCode >= 400 {
		if statusCode == 401 || statusCode == 403 {
			outcome = OutcomeDenied
		} else {
			outcome = OutcomeFailure
		}

		// Try to extract error message from response body
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			errorMsg = errResp.Error
		}
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		OrgID:        ac.OrgID,
		UserID:       ac.UserID,
		UserEmail:    ac.UserEmail,
		UserIP:       ac.UserIP,
		UserAgent:    ac.UserAgent,
		Action:       ac.Action,
		ResourceType: ac.ResourceType,
		ResourceID:   ac.ResourceID,
		Outcome:      outcome,
		ErrorMessage: errorMsg,
		RequestID:    ac.RequestID,
		SessionID:    ac.SessionID,
		Details: map[string]interface{}{
			"method":      ac.Method,
			"path":        ac.Path,
			"status_code": statusCode,
		},
	}

	if err := m.store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log: %v", err)
	}
}

// LogAction manually logs an action (for use in handlers)
func LogAction(ctx context.Context, store *Store, action ActionType, resourceType ResourceType, resourceID, resourceName string, details map[string]interface{}) {
	ac := GetAuditContext(ctx)
	if ac == nil {
		ac = &AuditContext{}
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		OrgID:        ac.OrgID,
		UserID:       ac.UserID,
		UserEmail:    ac.UserEmail,
		UserIP:       ac.UserIP,
		UserAgent:    ac.UserAgent,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Outcome:      OutcomeSuccess,
		RequestID:    ac.RequestID,
		SessionID:    ac.SessionID,
		Details:      details,
	}

	if err := store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log action: %v", err)
	}
}

// LogActionWithChanges logs an action with before/after changes
func LogActionWithChanges(ctx context.Context, store *Store, action ActionType, resourceType ResourceType, resourceID, resourceName string, before, after interface{}) {
	ac := GetAuditContext(ctx)
	if ac == nil {
		ac = &AuditContext{}
	}

	var changes *ChangeSet
	if before != nil || after != nil {
		changes = &ChangeSet{}
		if before != nil {
			if m, ok := before.(map[string]interface{}); ok {
				changes.Before = m
			} else {
				// Convert to map
				b, _ := json.Marshal(before)
				json.Unmarshal(b, &changes.Before)
			}
		}
		if after != nil {
			if m, ok := after.(map[string]interface{}); ok {
				changes.After = m
			} else {
				b, _ := json.Marshal(after)
				json.Unmarshal(b, &changes.After)
			}
		}
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		OrgID:        ac.OrgID,
		UserID:       ac.UserID,
		UserEmail:    ac.UserEmail,
		UserIP:       ac.UserIP,
		UserAgent:    ac.UserAgent,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Outcome:      OutcomeSuccess,
		RequestID:    ac.RequestID,
		SessionID:    ac.SessionID,
		Changes:      changes,
	}

	if err := store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log action with changes: %v", err)
	}
}

// LogFailure logs a failed action
func LogFailure(ctx context.Context, store *Store, action ActionType, resourceType ResourceType, resourceID string, err error) {
	ac := GetAuditContext(ctx)
	if ac == nil {
		ac = &AuditContext{}
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		OrgID:        ac.OrgID,
		UserID:       ac.UserID,
		UserEmail:    ac.UserEmail,
		UserIP:       ac.UserIP,
		UserAgent:    ac.UserAgent,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Outcome:      OutcomeFailure,
		ErrorMessage: err.Error(),
		RequestID:    ac.RequestID,
		SessionID:    ac.SessionID,
	}

	if err := store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log failure: %v", err)
	}
}

// LogLogin logs a login attempt
func LogLogin(store *Store, userID, email, ip, userAgent string, success bool, errorMsg string) {
	outcome := OutcomeSuccess
	if !success {
		outcome = OutcomeFailure
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		UserID:       userID,
		UserEmail:    email,
		UserIP:       ip,
		UserAgent:    userAgent,
		Action:       ActionLogin,
		ResourceType: ResourceSession,
		Outcome:      outcome,
		ErrorMessage: errorMsg,
	}

	if err := store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log login: %v", err)
	}
}

// LogLogout logs a logout
func LogLogout(store *Store, userID, email, ip, userAgent, sessionID string) {
	entry := &AuditLog{
		Timestamp:    time.Now(),
		UserID:       userID,
		UserEmail:    email,
		UserIP:       ip,
		UserAgent:    userAgent,
		Action:       ActionLogout,
		ResourceType: ResourceSession,
		SessionID:    sessionID,
		Outcome:      OutcomeSuccess,
	}

	if err := store.Log(entry); err != nil {
		log.Printf("[audit] Failed to log logout: %v", err)
	}
}

// Helper functions

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

func methodToAction(method string) ActionType {
	switch method {
	case "POST":
		return ActionCreate
	case "PUT", "PATCH":
		return ActionUpdate
	case "DELETE":
		return ActionDelete
	default:
		return ActionRead
	}
}

func parseResourceFromPath(path string) (ResourceType, string) {
	// Remove query string
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return "", ""
	}

	// Skip "api" prefix
	if parts[0] == "api" {
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return "", ""
	}

	// Map path segments to resource types
	resourceMap := map[string]ResourceType{
		"users":       ResourceUser,
		"teams":       ResourceTeam,
		"orgs":        ResourceOrg,
		"apikeys":     ResourceAPIKey,
		"dashboards":  ResourceDashboard,
		"alerts":      ResourceAlert,
		"alerting":    ResourceAlertRule,
		"silences":    ResourceSilence,
		"notify":      ResourceNotifyChannel,
		"incidents":   ResourceIncident,
		"schedules":   ResourceSchedule,
		"policies":    ResourcePolicy,
		"watches":     ResourceWatch,
		"synthetics":  ResourceSynthetic,
		"slos":        ResourceSLO,
		"deploys":     ResourceDeployment,
		"settings":    ResourceSettings,
		"auth":        ResourceSession,
		"oncall":      ResourceSchedule,
	}

	resourceType, ok := resourceMap[parts[0]]
	if !ok {
		return "", ""
	}

	// Extract resource ID if present
	var resourceID string
	if len(parts) > 1 {
		resourceID = parts[1]
		// Skip IDs that look like sub-resources
		if strings.Contains(resourceID, "/") || resourceID == "channels" || resourceID == "history" {
			resourceID = ""
		}
	}

	return resourceType, resourceID
}

func shouldAudit(path, method string) bool {
	// Skip health checks and static files
	if strings.HasPrefix(path, "/health") ||
		strings.HasPrefix(path, "/metrics") ||
		strings.HasPrefix(path, "/static") ||
		path == "/favicon.ico" {
		return false
	}

	// Skip read-only requests to non-sensitive endpoints
	if method == "GET" {
		// Always audit auth-related reads
		if strings.Contains(path, "/auth") || strings.Contains(path, "/rbac") || strings.Contains(path, "/audit") {
			return true
		}
		// Skip other GET requests (can enable for compliance)
		return false
	}

	// Audit all mutations
	return true
}

// RequestBodyReader captures and returns request body while allowing re-reading
func RequestBodyReader(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

// QueryAuditHook is called when a query is executed
type QueryAuditHook struct {
	store  *Store
	logger *Logger
}

// NewQueryAuditHook creates a new query audit hook
func NewQueryAuditHook(store *Store, logger *Logger) *QueryAuditHook {
	return &QueryAuditHook{
		store:  store,
		logger: logger,
	}
}

// LogQueryExecution logs a query execution from an HTTP context
func (h *QueryAuditHook) LogQueryExecution(ctx context.Context, queryType QueryType, queryText, dataSource string, timeRange TimeRange, rowsReturned int64, duration time.Duration, success bool, errorMsg string, piiAccessed bool, piiTypes, services []string) {
	entry := &QueryAuditEntry{
		Timestamp:        time.Now(),
		QueryType:        queryType,
		QueryText:        queryText,
		DataSource:       dataSource,
		TimeRange:        timeRange,
		RowsReturned:     rowsReturned,
		Success:          success,
		ErrorMessage:     errorMsg,
		AccessedPII:      piiAccessed,
		PIITypesAccessed: piiTypes,
		ServicesAccessed: services,
	}
	entry.SetDuration(duration)

	// Extract user context
	if ac := GetAuditContext(ctx); ac != nil {
		entry.UserID = ac.UserID
		entry.Username = ac.UserEmail
		entry.SessionID = ac.SessionID
		entry.IPAddress = ac.UserIP
		entry.UserAgent = ac.UserAgent
		entry.OrgID = ac.OrgID
		entry.RequestID = ac.RequestID
	}

	// Use async logger if available, otherwise write directly
	if h.logger != nil {
		h.logger.LogQuery(entry)
	} else if h.store != nil {
		if err := h.store.LogQueryAudit(entry); err != nil {
			log.Printf("[audit] Failed to log query execution: %v", err)
		}
	}
}

// LogAuthEvent logs an authentication event
func LogAuthEvent(store *Store, logger *Logger, eventType string, userID, username, email, orgID, ip, userAgent, sessionID string, success bool, errorMsg, failureCode, method, provider string) {
	entry := &AuthAuditEntry{
		Timestamp:    time.Now(),
		EventType:    eventType,
		UserID:       userID,
		Username:     username,
		Email:        email,
		OrgID:        orgID,
		IPAddress:    ip,
		UserAgent:    userAgent,
		SessionID:    sessionID,
		Success:      success,
		ErrorMessage: errorMsg,
		FailureCode:  failureCode,
		Method:       method,
		Provider:     provider,
	}

	if logger != nil {
		logger.LogAuth(entry)
	} else if store != nil {
		if err := store.LogAuthAudit(entry); err != nil {
			log.Printf("[audit] Failed to log auth event: %v", err)
		}
	}
}

// LogAdminAction logs an administrative action
func LogAdminAction(ctx context.Context, store *Store, logger *Logger, actionType, resourceType, resourceID, resourceName string, prevValue, newValue interface{}, success bool, errorMsg string) {
	entry := &AdminAuditEntry{
		Timestamp:     time.Now(),
		ActionType:    actionType,
		ResourceType:  resourceType,
		ResourceID:    resourceID,
		ResourceName:  resourceName,
		PreviousValue: prevValue,
		NewValue:      newValue,
		Success:       success,
		ErrorMessage:  errorMsg,
	}

	// Extract user context
	if ac := GetAuditContext(ctx); ac != nil {
		entry.UserID = ac.UserID
		entry.Username = ac.UserEmail
		entry.OrgID = ac.OrgID
		entry.IPAddress = ac.UserIP
		entry.RequestID = ac.RequestID
	}

	if logger != nil {
		logger.LogAdmin(entry)
	} else if store != nil {
		if err := store.LogAdminAudit(entry); err != nil {
			log.Printf("[audit] Failed to log admin action: %v", err)
		}
	}
}

// LogDataExport logs a data export operation
func LogDataExport(ctx context.Context, store *Store, logger *Logger, exportType, dataType, query string, timeRange TimeRange, recordCount, fileSizeBytes int64, containsPII bool, piiTypes []string, success bool, errorMsg, downloadURL string) {
	entry := &DataExportEntry{
		Timestamp:        time.Now(),
		ExportType:       exportType,
		DataType:         dataType,
		Query:            query,
		TimeRange:        timeRange,
		RecordCount:      recordCount,
		FileSizeBytes:    fileSizeBytes,
		ContainsPII:      containsPII,
		PIITypesExported: piiTypes,
		Success:          success,
		ErrorMessage:     errorMsg,
		DownloadURL:      downloadURL,
	}

	// Extract user context
	if ac := GetAuditContext(ctx); ac != nil {
		entry.UserID = ac.UserID
		entry.Username = ac.UserEmail
		entry.OrgID = ac.OrgID
		entry.IPAddress = ac.UserIP
		entry.RequestID = ac.RequestID
	}

	if logger != nil {
		logger.LogExport(entry)
	} else if store != nil {
		if err := store.LogExportAudit(entry); err != nil {
			log.Printf("[audit] Failed to log data export: %v", err)
		}
	}
}

// APIAuditMiddleware creates an audit middleware specifically for API requests
type APIAuditMiddleware struct {
	store  *Store
	logger *Logger
}

// NewAPIAuditMiddleware creates a new API audit middleware
func NewAPIAuditMiddleware(store *Store, logger *Logger) *APIAuditMiddleware {
	return &APIAuditMiddleware{
		store:  store,
		logger: logger,
	}
}

// Wrap wraps an http.Handler with detailed API audit logging
func (m *APIAuditMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()[:8]
		}

		// Create audit context
		ac := &AuditContext{
			RequestID: requestID,
			UserIP:    getClientIP(r),
			UserAgent: r.UserAgent(),
			Method:    r.Method,
			Path:      r.URL.Path,
		}

		// Extract user info from RBAC context
		if user := rbac.GetUserFromContext(r.Context()); user != nil {
			ac.UserID = user.ID
			ac.UserEmail = user.Email
			ac.OrgID = user.OrgID
		}

		// Determine resource type and action from path and method
		ac.ResourceType, ac.ResourceID = parseResourceFromPath(r.URL.Path)
		ac.Action = methodToAction(r.Method)

		// Add to context
		ctx := WithAuditContext(r.Context(), ac)
		ctx = context.WithValue(ctx, requestIDKey, requestID)

		// Wrap response writer to capture status
		rw := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		// Add request ID to response
		w.Header().Set("X-Request-ID", requestID)

		// Call next handler
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Calculate duration
		duration := time.Since(startTime)

		// Log based on endpoint sensitivity
		if shouldAudit(r.URL.Path, r.Method) {
			go m.logAPIRequest(ac, rw.statusCode, duration, rw.body.Bytes())
		}
	})
}

// logAPIRequest creates a detailed API audit log entry
func (m *APIAuditMiddleware) logAPIRequest(ac *AuditContext, statusCode int, duration time.Duration, body []byte) {
	outcome := OutcomeSuccess
	var errorMsg string

	if statusCode >= 400 {
		if statusCode == 401 || statusCode == 403 {
			outcome = OutcomeDenied
		} else {
			outcome = OutcomeFailure
		}

		// Try to extract error message from response body
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			errorMsg = errResp.Error
		}
	}

	entry := &AuditLog{
		Timestamp:    time.Now(),
		OrgID:        ac.OrgID,
		UserID:       ac.UserID,
		UserEmail:    ac.UserEmail,
		UserIP:       ac.UserIP,
		UserAgent:    ac.UserAgent,
		Action:       ac.Action,
		ResourceType: ac.ResourceType,
		ResourceID:   ac.ResourceID,
		Outcome:      outcome,
		ErrorMessage: errorMsg,
		RequestID:    ac.RequestID,
		SessionID:    ac.SessionID,
		Details: map[string]interface{}{
			"method":      ac.Method,
			"path":        ac.Path,
			"status_code": statusCode,
			"duration_ms": float64(duration.Nanoseconds()) / 1e6,
		},
	}

	if m.store != nil {
		if err := m.store.Log(entry); err != nil {
			log.Printf("[audit] Failed to log API request: %v", err)
		}
	}
}
