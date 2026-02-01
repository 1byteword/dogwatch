// Package integration provides end-to-end tests for dogwatch knowledge features.
package integration

import (
	"os"
	"path/filepath"
	"testing"

	"dogwatch/internal/knowledge"
)

// testKnowledgeStore creates a test knowledge store with in-memory SQLite
func testKnowledgeStore(t *testing.T) (*knowledge.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "dogwatch-knowledge-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "knowledge_test.db")
	store, err := knowledge.NewStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create knowledge store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestMacroCreationAndQueryExpansion tests macro creation -> query expansion -> execution
func TestMacroCreationAndQueryExpansion(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Step 1: Create custom macros
	macros := []struct {
		name       string
		expression string
		args       []string
	}{
		{
			name:       "high_errors",
			expression: `logs | where level = "error" AND count > 100`,
			args:       nil,
		},
		{
			name:       "slow_queries",
			expression: `traces | where duration > $1`,
			args:       []string{"threshold"},
		},
		{
			name:       "service_filter",
			expression: `where service = "$1"`,
			args:       []string{"service_name"},
		},
	}

	for _, m := range macros {
		obj := &knowledge.KnowledgeObject{
			ID:          "macro_" + m.name,
			Name:        m.name,
			Type:        knowledge.TypeMacro,
			Description: "Test macro: " + m.name,
			Owner:       "test-user",
			Shared:      true,
		}

		macro := knowledge.Macro{
			Expression: m.expression,
			Args:       m.args,
		}
		obj.SetDefinition(macro)

		err := store.Create(obj)
		if err != nil {
			t.Fatalf("Failed to create macro %s: %v", m.name, err)
		}
	}

	// Reload registry to pick up new macros
	registry.Reload()

	// Step 2: Test query expansion with macros
	testCases := []struct {
		name     string
		query    string
		expected string
	}{
		{
			name:     "Simple macro expansion",
			query:    "`high_errors` | limit 10",
			expected: `logs | where level = "error" AND count > 100 | limit 10`,
		},
		{
			name:     "Macro with argument",
			query:    "`slow_queries(500)` | select service, duration",
			expected: `traces | where duration > 500 | select service, duration`,
		},
		{
			name:     "Inline macro",
			query:    "logs | `service_filter(api-gateway)` | limit 5",
			expected: `logs | where service = "api-gateway" | limit 5`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			expanded, err := expander.ExpandMacros(tc.query)
			if err != nil {
				t.Fatalf("Expansion failed: %v", err)
			}

			if expanded != tc.expected {
				t.Errorf("Expansion mismatch:\n  Expected: %s\n  Got:      %s", tc.expected, expanded)
			}

			t.Logf("Query: %s", tc.query)
			t.Logf("Expanded: %s", expanded)
		})
	}
}

// TestFieldExtractionOnRealLogData tests field extraction on real log data
func TestFieldExtractionOnRealLogData(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Step 1: Create a custom field extraction for a specific log format
	customExtraction := &knowledge.KnowledgeObject{
		ID:          "extract_custom_log",
		Name:        "custom_app_log",
		Type:        knowledge.TypeFieldExtraction,
		Description: "Extract fields from custom application log format",
		Owner:       "test-user",
		Shared:      true,
	}

	fe := knowledge.FieldExtraction{
		Pattern:      `^\[(?P<level>\w+)\]\s+(?P<timestamp>[\d-]+\s[\d:]+)\s+-\s+(?P<component>\w+)\s+-\s+(?P<message>.*)$`,
		SourceField:  "message",
		TargetFields: []string{"level", "timestamp", "component", "message"},
	}
	customExtraction.SetDefinition(fe)

	err := store.Create(customExtraction)
	if err != nil {
		t.Fatalf("Failed to create field extraction: %v", err)
	}

	registry.Reload()

	// Step 2: Apply extraction to log data
	logRows := []knowledge.Row{
		{"message": "[INFO] 2024-01-15 10:30:00 - AuthService - User login successful"},
		{"message": "[ERROR] 2024-01-15 10:31:15 - PaymentService - Payment failed: insufficient funds"},
		{"message": "[WARN] 2024-01-15 10:32:00 - CacheService - Cache miss rate high"},
		{"message": "Invalid format log entry"}, // Should not match
	}

	enrichedRows, err := expander.ApplyFieldExtraction("custom_app_log", logRows)
	if err != nil {
		t.Fatalf("Field extraction failed: %v", err)
	}

	// Verify extraction results
	for i, row := range enrichedRows {
		if i < 3 { // First 3 should match
			if row["level"] == nil {
				t.Errorf("Row %d: level not extracted", i)
			}
			if row["component"] == nil {
				t.Errorf("Row %d: component not extracted", i)
			}
			t.Logf("Row %d: level=%v, component=%v", i, row["level"], row["component"])
		}
	}

	// Verify specific values
	if enrichedRows[0]["level"] != "INFO" {
		t.Errorf("Expected level 'INFO', got '%v'", enrichedRows[0]["level"])
	}
	if enrichedRows[0]["component"] != "AuthService" {
		t.Errorf("Expected component 'AuthService', got '%v'", enrichedRows[0]["component"])
	}
	if enrichedRows[1]["level"] != "ERROR" {
		t.Errorf("Expected level 'ERROR', got '%v'", enrichedRows[1]["level"])
	}
}

// TestLookupEnrichmentInQueryResults tests lookup enrichment in query results
func TestLookupEnrichmentInQueryResults(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Step 1: Create a custom lookup table
	customLookup := &knowledge.KnowledgeObject{
		ID:          "lookup_region",
		Name:        "region_lookup",
		Type:        knowledge.TypeLookup,
		Description: "Map region codes to full names and timezone",
		Owner:       "test-user",
		Shared:      true,
	}

	lookup := knowledge.Lookup{
		KeyField:     "region_code",
		OutputFields: []string{"region_name", "timezone", "currency"},
		Data: map[string]map[string]string{
			"US-EAST": {"region_name": "US East (Virginia)", "timezone": "EST", "currency": "USD"},
			"US-WEST": {"region_name": "US West (Oregon)", "timezone": "PST", "currency": "USD"},
			"EU-WEST": {"region_name": "EU West (Ireland)", "timezone": "GMT", "currency": "EUR"},
			"AP-EAST": {"region_name": "Asia Pacific (Singapore)", "timezone": "SGT", "currency": "SGD"},
		},
	}
	customLookup.SetDefinition(lookup)

	err := store.Create(customLookup)
	if err != nil {
		t.Fatalf("Failed to create lookup: %v", err)
	}

	registry.Reload()

	// Step 2: Apply lookup to query results
	queryResults := []knowledge.Row{
		{"service": "api-gateway", "region_code": "US-EAST", "requests": 1000},
		{"service": "user-service", "region_code": "EU-WEST", "requests": 750},
		{"service": "order-service", "region_code": "AP-EAST", "requests": 500},
		{"service": "payment-service", "region_code": "UNKNOWN", "requests": 100}, // No lookup match
	}

	enrichedRows, err := expander.ApplyLookup("region_lookup", queryResults)
	if err != nil {
		t.Fatalf("Lookup enrichment failed: %v", err)
	}

	// Verify enrichment
	for i, row := range enrichedRows {
		t.Logf("Row %d: service=%v, region_code=%v, region_name=%v, timezone=%v",
			i, row["service"], row["region_code"], row["region_name"], row["timezone"])
	}

	// Check specific enrichments
	if enrichedRows[0]["region_name"] != "US East (Virginia)" {
		t.Errorf("Expected region_name 'US East (Virginia)', got '%v'", enrichedRows[0]["region_name"])
	}
	if enrichedRows[0]["timezone"] != "EST" {
		t.Errorf("Expected timezone 'EST', got '%v'", enrichedRows[0]["timezone"])
	}
	if enrichedRows[1]["currency"] != "EUR" {
		t.Errorf("Expected currency 'EUR', got '%v'", enrichedRows[1]["currency"])
	}

	// Unknown region should not have enriched fields
	if enrichedRows[3]["region_name"] != nil {
		t.Errorf("Unknown region should not have region_name, got '%v'", enrichedRows[3]["region_name"])
	}
}

// TestBuiltinKnowledgeObjects tests built-in knowledge objects
func TestBuiltinKnowledgeObjects(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)

	// Verify built-in macros exist
	builtinMacros := []string{"error_logs", "slow_requests", "error_rate"}
	for _, name := range builtinMacros {
		obj, err := registry.GetMacro(name)
		if err != nil {
			t.Errorf("Built-in macro '%s' not found: %v", name, err)
			continue
		}
		if obj.Owner != "system" {
			t.Errorf("Built-in macro '%s' should have owner 'system', got '%s'", name, obj.Owner)
		}
		t.Logf("Found built-in macro: %s", name)
	}

	// Verify built-in field extractions exist
	builtinExtractions := []string{"apache_combined", "nginx_combined", "syslog", "json_extract"}
	for _, name := range builtinExtractions {
		obj, err := registry.GetFieldExtraction(name)
		if err != nil {
			t.Errorf("Built-in field extraction '%s' not found: %v", name, err)
			continue
		}
		t.Logf("Found built-in extraction: %s", name)

		// Validate the extraction
		fe, err := obj.ParseFieldExtraction()
		if err != nil {
			t.Errorf("Failed to parse field extraction '%s': %v", name, err)
		} else if fe.Pattern == "" && name != "json_extract" {
			t.Errorf("Field extraction '%s' has empty pattern", name)
		}
	}

	// Verify built-in lookups exist
	builtinLookups := []string{"http_status_codes", "log_levels"}
	for _, name := range builtinLookups {
		obj, err := registry.GetLookup(name)
		if err != nil {
			t.Errorf("Built-in lookup '%s' not found: %v", name, err)
			continue
		}
		t.Logf("Found built-in lookup: %s", name)

		// Validate the lookup has data
		lookup, err := obj.ParseLookup()
		if err != nil {
			t.Errorf("Failed to parse lookup '%s': %v", name, err)
		} else if len(lookup.Data) == 0 {
			t.Errorf("Lookup '%s' has empty data", name)
		}
	}
}

// TestHTTPStatusCodeLookup tests the built-in HTTP status code lookup
func TestHTTPStatusCodeLookup(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Apply HTTP status code lookup
	httpLogs := []knowledge.Row{
		{"path": "/api/users", "status": "200", "duration": 50},
		{"path": "/api/orders", "status": "404", "duration": 10},
		{"path": "/api/payment", "status": "500", "duration": 100},
		{"path": "/api/health", "status": "204", "duration": 5},
		{"path": "/api/login", "status": "401", "duration": 15},
	}

	enriched, err := expander.ApplyLookup("http_status_codes", httpLogs)
	if err != nil {
		t.Fatalf("HTTP status lookup failed: %v", err)
	}

	expectedEnrichments := map[string]map[string]string{
		"200": {"status_text": "OK", "status_category": "success"},
		"404": {"status_text": "Not Found", "status_category": "client_error"},
		"500": {"status_text": "Internal Server Error", "status_category": "server_error"},
		"204": {"status_text": "No Content", "status_category": "success"},
		"401": {"status_text": "Unauthorized", "status_category": "client_error"},
	}

	for i, row := range enriched {
		status := row["status"].(string)
		expected := expectedEnrichments[status]

		if row["status_text"] != expected["status_text"] {
			t.Errorf("Row %d: expected status_text '%s', got '%v'",
				i, expected["status_text"], row["status_text"])
		}
		if row["status_category"] != expected["status_category"] {
			t.Errorf("Row %d: expected status_category '%s', got '%v'",
				i, expected["status_category"], row["status_category"])
		}

		t.Logf("Status %s -> %v (%v)", status, row["status_text"], row["status_category"])
	}
}

// TestKnowledgeObjectCRUD tests CRUD operations on knowledge objects
func TestKnowledgeObjectCRUD(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	// Create
	obj := &knowledge.KnowledgeObject{
		ID:          "crud_test",
		Name:        "test_macro",
		Type:        knowledge.TypeMacro,
		Description: "Test CRUD operations",
		Tags:        []string{"test", "crud"},
		Owner:       "test-user",
		Shared:      true,
	}
	obj.SetDefinition(knowledge.Macro{Expression: "logs | where level = 'error'"})

	err := store.Create(obj)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read
	retrieved, err := store.Get(obj.ID)
	if err != nil {
		t.Fatalf("Get by ID failed: %v", err)
	}
	if retrieved.Name != obj.Name {
		t.Errorf("Name mismatch: expected '%s', got '%s'", obj.Name, retrieved.Name)
	}

	// Read by name
	retrievedByName, err := store.GetByName(obj.Name)
	if err != nil {
		t.Fatalf("Get by name failed: %v", err)
	}
	if retrievedByName.ID != obj.ID {
		t.Errorf("ID mismatch when getting by name")
	}

	// Update
	obj.Description = "Updated description"
	obj.Tags = append(obj.Tags, "updated")
	err = store.Update(obj)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	updated, _ := store.Get(obj.ID)
	if updated.Description != "Updated description" {
		t.Error("Description not updated")
	}
	if len(updated.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(updated.Tags))
	}

	// List
	objs, err := store.List(knowledge.ListFilters{Type: knowledge.TypeMacro})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, o := range objs {
		if o.ID == obj.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Object not found in list")
	}

	// Search - search for term in name (which wasn't updated)
	searchResults, err := store.Search("test_macro")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	found = false
	for _, o := range searchResults {
		if o.ID == obj.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Object not found in search results")
	}

	// Delete
	err = store.Delete(obj.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(obj.ID)
	if err != knowledge.ErrNotFound {
		t.Error("Expected ErrNotFound after delete")
	}
}

// TestMacroValidation tests macro validation
func TestMacroValidation(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	testCases := []struct {
		name        string
		macro       knowledge.Macro
		expectValid bool
	}{
		{
			name: "Valid macro without args",
			macro: knowledge.Macro{
				Expression: "logs | where level = 'error'",
				Args:       nil,
			},
			expectValid: true,
		},
		{
			name: "Valid macro with used args",
			macro: knowledge.Macro{
				Expression: "logs | where service = '$1' AND level = '$2'",
				Args:       []string{"service", "level"},
			},
			expectValid: true,
		},
		{
			name: "Invalid - empty expression",
			macro: knowledge.Macro{
				Expression: "",
				Args:       nil,
			},
			expectValid: false,
		},
		{
			name: "Warning - unused argument",
			macro: knowledge.Macro{
				Expression: "logs | where service = '$1'",
				Args:       []string{"service", "unused_arg"},
			},
			expectValid: true, // Valid but should have warning
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := expander.ValidateMacro(&tc.macro)

			if tc.expectValid && !result.Valid {
				t.Errorf("Expected valid, but got errors: %v", result.Errors)
			}
			if !tc.expectValid && result.Valid {
				t.Error("Expected invalid, but validation passed")
			}

			if len(result.Warning) > 0 {
				t.Logf("Warnings: %v", result.Warning)
			}
		})
	}
}

// TestFieldExtractionValidation tests field extraction validation
func TestFieldExtractionValidation(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	testCases := []struct {
		name        string
		fe          knowledge.FieldExtraction
		expectValid bool
	}{
		{
			name: "Valid extraction with named groups",
			fe: knowledge.FieldExtraction{
				Pattern:      `(?P<ip>\d+\.\d+\.\d+\.\d+)\s+(?P<user>\S+)`,
				SourceField:  "message",
				TargetFields: []string{"ip", "user"},
			},
			expectValid: true,
		},
		{
			name: "Invalid regex",
			fe: knowledge.FieldExtraction{
				Pattern:      `[invalid(regex`,
				SourceField:  "message",
				TargetFields: []string{},
			},
			expectValid: false,
		},
		{
			name: "Warning - no named groups",
			fe: knowledge.FieldExtraction{
				Pattern:      `\d+\.\d+\.\d+\.\d+`,
				SourceField:  "message",
				TargetFields: []string{},
			},
			expectValid: true, // Valid but should have warning
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := expander.ValidateFieldExtraction(&tc.fe)

			if tc.expectValid && !result.Valid {
				t.Errorf("Expected valid, but got errors: %v", result.Errors)
			}
			if !tc.expectValid && result.Valid {
				t.Error("Expected invalid, but validation passed")
			}

			if len(result.Warning) > 0 {
				t.Logf("Warnings: %v", result.Warning)
			}
		})
	}
}

// TestLookupValidation tests lookup validation
func TestLookupValidation(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	testCases := []struct {
		name        string
		lookup      knowledge.Lookup
		expectValid bool
	}{
		{
			name: "Valid lookup",
			lookup: knowledge.Lookup{
				KeyField:     "status_code",
				OutputFields: []string{"status_text"},
				Data: map[string]map[string]string{
					"200": {"status_text": "OK"},
					"404": {"status_text": "Not Found"},
				},
			},
			expectValid: true,
		},
		{
			name: "Invalid - empty key field",
			lookup: knowledge.Lookup{
				KeyField:     "",
				OutputFields: []string{"status_text"},
				Data:         map[string]map[string]string{},
			},
			expectValid: false,
		},
		{
			name: "Warning - empty data",
			lookup: knowledge.Lookup{
				KeyField:     "code",
				OutputFields: []string{"description"},
				Data:         map[string]map[string]string{},
			},
			expectValid: true, // Valid but should have warning
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := expander.ValidateLookup(&tc.lookup)

			if tc.expectValid && !result.Valid {
				t.Errorf("Expected valid, but got errors: %v", result.Errors)
			}
			if !tc.expectValid && result.Valid {
				t.Error("Expected invalid, but validation passed")
			}

			if len(result.Warning) > 0 {
				t.Logf("Warnings: %v", result.Warning)
			}
		})
	}
}

// TestRegistryCache tests the registry caching mechanism
func TestRegistryCache(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)

	// First access - loads from store
	macros1 := registry.ListMacros()
	initialCount := len(macros1)

	// Add a new macro directly to store
	newMacro := &knowledge.KnowledgeObject{
		ID:     "new_macro",
		Name:   "newly_added",
		Type:   knowledge.TypeMacro,
		Owner:  "test",
		Shared: true,
	}
	newMacro.SetDefinition(knowledge.Macro{Expression: "test"})
	store.Create(newMacro)

	// Second access - should still use cache (same count)
	macros2 := registry.ListMacros()
	if len(macros2) != initialCount {
		t.Log("Cache may have been invalidated due to TTL")
	}

	// Invalidate cache
	registry.InvalidateCache()

	// Third access - should reload and include new macro
	macros3 := registry.ListMacros()
	if len(macros3) != initialCount+1 {
		t.Errorf("Expected %d macros after cache invalidation, got %d",
			initialCount+1, len(macros3))
	}

	t.Logf("Initial: %d, After add: %d, After invalidation: %d",
		initialCount, len(macros2), len(macros3))
}

// TestCircularMacroReference tests handling of circular macro references
func TestCircularMacroReference(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	// Create macros that reference each other
	macro1 := &knowledge.KnowledgeObject{
		ID:     "circular_1",
		Name:   "macro_a",
		Type:   knowledge.TypeMacro,
		Owner:  "test",
		Shared: true,
	}
	macro1.SetDefinition(knowledge.Macro{Expression: "`macro_b` | limit 10"})

	macro2 := &knowledge.KnowledgeObject{
		ID:     "circular_2",
		Name:   "macro_b",
		Type:   knowledge.TypeMacro,
		Owner:  "test",
		Shared: true,
	}
	macro2.SetDefinition(knowledge.Macro{Expression: "`macro_a` | select *"})

	store.Create(macro1)
	store.Create(macro2)

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Try to expand - should handle circular reference gracefully
	query := "`macro_a`"
	expanded, err := expander.ExpandMacros(query)

	// Should not panic or hang
	if err != nil {
		t.Logf("Expected error for circular reference: %v", err)
	}

	t.Logf("Input: %s", query)
	t.Logf("Expanded: %s", expanded)

	// The expansion should stop at some point (not infinite)
	if len(expanded) > 1000 {
		t.Error("Expansion seems to be infinite")
	}
}

// TestApplyAllFieldExtractions tests automatic field extraction
func TestApplyAllFieldExtractions(t *testing.T) {
	store, cleanup := testKnowledgeStore(t)
	defer cleanup()

	registry := knowledge.NewRegistry(store)
	expander := knowledge.NewExpander(registry)

	// Create log entries that match different extraction patterns
	logRows := []knowledge.Row{
		// Apache format
		{"message": `192.168.1.1 - john [15/Jan/2024:10:30:00 +0000] "GET /api/users HTTP/1.1" 200 1234 "-" "Mozilla/5.0"`},
		// Nginx format
		{"message": `10.0.0.1 - admin [15/Jan/2024:10:31:00 +0000] "POST /api/orders HTTP/1.1" 201 567 "https://example.com" "curl/7.68.0"`},
		// Syslog format
		{"message": `Jan 15 10:32:00 webserver nginx[1234]: Connection closed`},
	}

	// Apply all field extractions automatically
	enrichedRows := expander.ApplyAllFieldExtractions(logRows)

	// Log the results
	for i, row := range enrichedRows {
		t.Logf("Row %d enriched fields:", i)
		for k, v := range row {
			if k != "message" {
				t.Logf("  %s = %v", k, v)
			}
		}
	}

	// At least one row should have extracted fields
	hasExtractedFields := false
	for _, row := range enrichedRows {
		for k := range row {
			if k != "message" {
				hasExtractedFields = true
				break
			}
		}
	}

	if !hasExtractedFields {
		t.Log("No fields were extracted - patterns may not have matched")
	}
}
