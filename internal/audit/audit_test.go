package audit

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestQueryAuditEntry(t *testing.T) {
	entry := NewQueryAuditEntry()

	if entry.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
	if !entry.Success {
		t.Error("Expected success to default to true")
	}

	// Test SetDuration
	entry.SetDuration(100 * time.Millisecond)
	if entry.Duration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", entry.Duration)
	}
	if entry.DurationMs < 99 || entry.DurationMs > 101 {
		t.Errorf("Expected DurationMs ~100, got %f", entry.DurationMs)
	}
}

func TestTimeRange(t *testing.T) {
	start := time.Now()
	end := start.Add(time.Hour)
	tr := TimeRange{Start: start, End: end}

	if tr.Duration() != time.Hour {
		t.Errorf("Expected duration 1 hour, got %v", tr.Duration())
	}
}

func TestStoreQueryAudit(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create a query audit entry
	entry := &QueryAuditEntry{
		Timestamp:   time.Now(),
		UserID:      "user_123",
		Username:    "testuser@example.com",
		IPAddress:   "192.168.1.1",
		OrgID:       "org_456",
		QueryType:   QueryTypeLogs,
		QueryText:   "logs | where service = 'api'",
		DataSource:  "logs",
		TimeRange:   TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()},
		RowsReturned: 100,
		Success:      true,
		AccessedPII:  false,
	}
	entry.SetDuration(50 * time.Millisecond)

	// Log the entry
	if err := store.LogQueryAudit(entry); err != nil {
		t.Fatalf("Failed to log query audit: %v", err)
	}

	// Retrieve the entry
	entries, err := store.ListQueryAudit(QueryAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list query audit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	retrieved := entries[0]
	if retrieved.UserID != entry.UserID {
		t.Errorf("Expected UserID %s, got %s", entry.UserID, retrieved.UserID)
	}
	if retrieved.QueryType != entry.QueryType {
		t.Errorf("Expected QueryType %s, got %s", entry.QueryType, retrieved.QueryType)
	}
	if retrieved.RowsReturned != entry.RowsReturned {
		t.Errorf("Expected RowsReturned %d, got %d", entry.RowsReturned, retrieved.RowsReturned)
	}
}

func TestStoreAuthAudit(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create auth audit entry
	entry := &AuthAuditEntry{
		Timestamp: time.Now(),
		EventType: AuthEventLogin,
		UserID:    "user_123",
		Username:  "testuser",
		Email:     "test@example.com",
		OrgID:     "org_456",
		IPAddress: "192.168.1.1",
		Success:   true,
		Method:    "password",
	}

	if err := store.LogAuthAudit(entry); err != nil {
		t.Fatalf("Failed to log auth audit: %v", err)
	}

	// Retrieve
	entries, err := store.ListAuthAudit(AuthAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list auth audit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].EventType != AuthEventLogin {
		t.Errorf("Expected event type %s, got %s", AuthEventLogin, entries[0].EventType)
	}
}

func TestStoreAdminAudit(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create admin audit entry
	entry := &AdminAuditEntry{
		Timestamp:    time.Now(),
		UserID:       "admin_123",
		Username:     "admin@example.com",
		OrgID:        "org_456",
		IPAddress:    "192.168.1.1",
		ActionType:   AdminActionUserCreate,
		ResourceType: "user",
		ResourceID:   "new_user_789",
		ResourceName: "New User",
		NewValue:     map[string]interface{}{"email": "newuser@example.com", "role": "viewer"},
		Success:      true,
	}

	if err := store.LogAdminAudit(entry); err != nil {
		t.Fatalf("Failed to log admin audit: %v", err)
	}

	// Retrieve
	entries, err := store.ListAdminAudit(AdminAuditFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list admin audit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].ActionType != AdminActionUserCreate {
		t.Errorf("Expected action type %s, got %s", AdminActionUserCreate, entries[0].ActionType)
	}
}

func TestStoreExportAudit(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create export audit entry
	entry := &DataExportEntry{
		Timestamp:     time.Now(),
		UserID:        "user_123",
		Username:      "testuser@example.com",
		OrgID:         "org_456",
		IPAddress:     "192.168.1.1",
		ExportType:    "csv",
		DataType:      "logs",
		Query:         "logs | where level = 'error'",
		TimeRange:     TimeRange{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
		RecordCount:   5000,
		FileSizeBytes: 1024000,
		ContainsPII:   true,
		PIITypesExported: []string{"email", "ip_address"},
		Success:       true,
	}

	if err := store.LogExportAudit(entry); err != nil {
		t.Fatalf("Failed to log export audit: %v", err)
	}

	// Retrieve
	entries, err := store.ListExportAudit("", "", nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("Failed to list export audit: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if !entries[0].ContainsPII {
		t.Error("Expected ContainsPII to be true")
	}
	if len(entries[0].PIITypesExported) != 2 {
		t.Errorf("Expected 2 PII types, got %d", len(entries[0].PIITypesExported))
	}
}

func TestQueryAuditFilter(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create multiple entries
	for i := 0; i < 5; i++ {
		entry := &QueryAuditEntry{
			Timestamp:   time.Now(),
			UserID:      "user_A",
			QueryType:   QueryTypeLogs,
			QueryText:   "test query",
			DataSource:  "logs",
			Success:     true,
			AccessedPII: i%2 == 0,
		}
		store.LogQueryAudit(entry)
	}

	for i := 0; i < 3; i++ {
		entry := &QueryAuditEntry{
			Timestamp:  time.Now(),
			UserID:     "user_B",
			QueryType:  QueryTypeMetrics,
			QueryText:  "test query",
			DataSource: "metrics",
			Success:    false,
		}
		store.LogQueryAudit(entry)
	}

	// Test filtering by user
	entries, _ := store.ListQueryAudit(QueryAuditFilter{UserID: "user_A", Limit: 100})
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries for user_A, got %d", len(entries))
	}

	// Test filtering by query type
	entries, _ = store.ListQueryAudit(QueryAuditFilter{QueryType: QueryTypeMetrics, Limit: 100})
	if len(entries) != 3 {
		t.Errorf("Expected 3 metrics entries, got %d", len(entries))
	}

	// Test filtering by PII access
	piiFlag := true
	entries, _ = store.ListQueryAudit(QueryAuditFilter{AccessedPII: &piiFlag, Limit: 100})
	if len(entries) != 3 {
		t.Errorf("Expected 3 PII entries, got %d", len(entries))
	}

	// Test count
	count, _ := store.CountQueryAudit(QueryAuditFilter{UserID: "user_A"})
	if count != 5 {
		t.Errorf("Expected count 5, got %d", count)
	}
}

func TestRetentionPolicy(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Get default policy
	policy, err := store.GetRetentionPolicy()
	if err != nil {
		t.Fatalf("Failed to get retention policy: %v", err)
	}

	if policy.QueryAuditDays != 90 {
		t.Errorf("Expected default QueryAuditDays 90, got %d", policy.QueryAuditDays)
	}

	// Update policy
	newPolicy := &RetentionPolicy{
		QueryAuditDays:   30,
		AuthAuditDays:    180,
		AdminAuditDays:   365,
		ExportAuditDays:  180,
		GeneralAuditDays: 30,
	}

	if err := store.SetRetentionPolicy(newPolicy); err != nil {
		t.Fatalf("Failed to set retention policy: %v", err)
	}

	// Verify
	policy, _ = store.GetRetentionPolicy()
	if policy.QueryAuditDays != 30 {
		t.Errorf("Expected QueryAuditDays 30, got %d", policy.QueryAuditDays)
	}
}

func TestAuditSummary(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create some entries
	for i := 0; i < 10; i++ {
		store.LogQueryAudit(&QueryAuditEntry{
			Timestamp:   time.Now(),
			UserID:      "user_1",
			Username:    "testuser",
			QueryType:   QueryTypeLogs,
			QueryText:   "test",
			Success:     i%2 == 0,
			AccessedPII: i%3 == 0,
		})
	}

	for i := 0; i < 5; i++ {
		store.LogAuthAudit(&AuthAuditEntry{
			Timestamp: time.Now(),
			EventType: AuthEventLogin,
			UserID:    "user_1",
			Success:   true,
			Method:    "password",
		})
	}

	for i := 0; i < 3; i++ {
		store.LogAdminAudit(&AdminAuditEntry{
			Timestamp:    time.Now(),
			UserID:       "admin_1",
			Username:     "admin",
			ActionType:   AdminActionUserCreate,
			ResourceType: "user",
			Success:      true,
		})
	}

	// Get summary
	summary, err := store.GetAuditSummary("", 24*time.Hour)
	if err != nil {
		t.Fatalf("Failed to get audit summary: %v", err)
	}

	if summary.TotalQueries != 10 {
		t.Errorf("Expected 10 total queries, got %d", summary.TotalQueries)
	}
	if summary.SuccessfulQueries != 5 {
		t.Errorf("Expected 5 successful queries, got %d", summary.SuccessfulQueries)
	}
	if summary.TotalLogins != 5 {
		t.Errorf("Expected 5 logins, got %d", summary.TotalLogins)
	}
	if summary.TotalAdminActions != 3 {
		t.Errorf("Expected 3 admin actions, got %d", summary.TotalAdminActions)
	}
}

func TestAsyncLogger(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create logger with short flush interval
	config := DefaultLoggerConfig()
	config.FlushInterval = 100 * time.Millisecond
	config.BufferSize = 100

	logger := NewLogger(store, config)
	logger.Start()
	defer logger.Stop()

	// Log some entries
	for i := 0; i < 10; i++ {
		logger.LogQuery(&QueryAuditEntry{
			Timestamp: time.Now(),
			UserID:    "user_1",
			QueryType: QueryTypeLogs,
			QueryText: "test query",
			Success:   true,
		})
	}

	// Wait for flush
	time.Sleep(200 * time.Millisecond)

	// Check stats
	stats := logger.GetStats()
	if stats.QueriesLogged != 10 {
		t.Errorf("Expected 10 queries logged, got %d", stats.QueriesLogged)
	}

	// Verify in database
	entries, _ := store.ListQueryAudit(QueryAuditFilter{Limit: 100})
	if len(entries) != 10 {
		t.Errorf("Expected 10 entries in database, got %d", len(entries))
	}
}

func TestQueryAuditHook(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	hook := NewQueryAuditHook(store, nil)

	// Create context with audit context
	ctx := context.Background()
	ac := &AuditContext{
		UserID:    "user_123",
		UserEmail: "test@example.com",
		OrgID:     "org_456",
		UserIP:    "192.168.1.1",
		RequestID: "req_abc",
	}
	ctx = WithAuditContext(ctx, ac)

	// Log query execution
	hook.LogQueryExecution(
		ctx,
		QueryTypeLogs,
		"logs | where service = 'api'",
		"logs",
		TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()},
		100,
		50*time.Millisecond,
		true,
		"",
		false,
		nil,
		[]string{"api", "web"},
	)

	// Verify
	entries, _ := store.ListQueryAudit(QueryAuditFilter{Limit: 10})
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.UserID != "user_123" {
		t.Errorf("Expected UserID user_123, got %s", entry.UserID)
	}
	if entry.OrgID != "org_456" {
		t.Errorf("Expected OrgID org_456, got %s", entry.OrgID)
	}
	if len(entry.ServicesAccessed) != 2 {
		t.Errorf("Expected 2 services, got %d", len(entry.ServicesAccessed))
	}
}
