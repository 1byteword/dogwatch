package knowledge

import (
	"os"
	"testing"
)

func setupTestExpander(t *testing.T) (*Expander, func()) {
	tmpFile, err := os.CreateTemp("", "knowledge_expander_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("Failed to create store: %v", err)
	}

	registry := NewRegistry(store)
	expander := NewExpander(registry)

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return expander, cleanup
}

func TestExpander_ExpandMacros(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Test expanding builtin macro
	query := "`error_logs` | count by (service)"
	expanded, err := expander.ExpandMacros(query)
	if err != nil {
		t.Fatalf("Failed to expand macros: %v", err)
	}

	expected := `logs | where level = "error" | count by (service)`
	if expanded != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, expanded)
	}
}

func TestExpander_ExpandMacrosWithArgs(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Test expanding macro with arguments
	query := "`slow_requests(100)`"
	expanded, err := expander.ExpandMacros(query)
	if err != nil {
		t.Fatalf("Failed to expand macros: %v", err)
	}

	expected := "traces | where duration > 100"
	if expanded != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, expanded)
	}
}

func TestExpander_NoMacroExpansion(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Query without macros should be unchanged
	query := "logs | where level = \"info\""
	expanded, err := expander.ExpandMacros(query)
	if err != nil {
		t.Fatalf("Failed to expand macros: %v", err)
	}

	if expanded != query {
		t.Errorf("Expected unchanged query, got:\n%s", expanded)
	}
}

func TestExpander_UnknownMacro(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Unknown macro should be left unchanged
	query := "`unknown_macro` | count"
	expanded, err := expander.ExpandMacros(query)
	if err != nil {
		t.Fatalf("Failed to expand macros: %v", err)
	}

	if expanded != query {
		t.Errorf("Expected unchanged query for unknown macro, got:\n%s", expanded)
	}
}

func TestExpander_ApplyFieldExtraction(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Test Apache combined log format extraction
	rows := []Row{
		{
			"message": `192.168.1.1 - john [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326 "http://www.example.com/start.html" "Mozilla/4.08 [en] (Win98; I ;Nav)"`,
		},
	}

	result, err := expander.ApplyFieldExtraction("apache_combined", rows)
	if err != nil {
		t.Fatalf("Failed to apply field extraction: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 row, got %d", len(result))
	}

	row := result[0]
	if row["remote_addr"] != "192.168.1.1" {
		t.Errorf("Expected remote_addr 192.168.1.1, got %v", row["remote_addr"])
	}
	if row["status"] != "200" {
		t.Errorf("Expected status 200, got %v", row["status"])
	}
	if row["method"] != "GET" {
		t.Errorf("Expected method GET, got %v", row["method"])
	}
}

func TestExpander_ApplyLookup(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	rows := []Row{
		{"status": "200"},
		{"status": "404"},
		{"status": "500"},
		{"status": "999"}, // Unknown status
	}

	result, err := expander.ApplyLookup("http_status_codes", rows)
	if err != nil {
		t.Fatalf("Failed to apply lookup: %v", err)
	}

	if len(result) != 4 {
		t.Fatalf("Expected 4 rows, got %d", len(result))
	}

	// Check 200
	if result[0]["status_text"] != "OK" {
		t.Errorf("Expected status_text OK, got %v", result[0]["status_text"])
	}
	if result[0]["status_category"] != "success" {
		t.Errorf("Expected status_category success, got %v", result[0]["status_category"])
	}

	// Check 404
	if result[1]["status_text"] != "Not Found" {
		t.Errorf("Expected status_text Not Found, got %v", result[1]["status_text"])
	}
	if result[1]["status_category"] != "client_error" {
		t.Errorf("Expected status_category client_error, got %v", result[1]["status_category"])
	}

	// Check 500
	if result[2]["status_text"] != "Internal Server Error" {
		t.Errorf("Expected status_text Internal Server Error, got %v", result[2]["status_text"])
	}

	// Check unknown - should not have lookup fields
	if _, ok := result[3]["status_text"]; ok {
		t.Error("Expected unknown status to not have status_text")
	}
}

func TestExpander_ApplyLogLevelsLookup(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	rows := []Row{
		{"level": "INFO"},
		{"level": "ERR"},
		{"level": "D"},
		{"level": "WARN"},
	}

	result, err := expander.ApplyLookup("log_levels", rows)
	if err != nil {
		t.Fatalf("Failed to apply lookup: %v", err)
	}

	if result[0]["level_name"] != "INFO" {
		t.Errorf("Expected level_name INFO, got %v", result[0]["level_name"])
	}
	if result[1]["level_name"] != "ERROR" {
		t.Errorf("Expected level_name ERROR, got %v", result[1]["level_name"])
	}
	if result[2]["level_name"] != "DEBUG" {
		t.Errorf("Expected level_name DEBUG, got %v", result[2]["level_name"])
	}
	if result[3]["level_name"] != "WARN" {
		t.Errorf("Expected level_name WARN, got %v", result[3]["level_name"])
	}
}

func TestExpander_ValidateFieldExtraction(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Valid extraction
	fe := &FieldExtraction{
		Pattern:      `^(?P<level>\w+): (?P<msg>.*)$`,
		SourceField:  "message",
		TargetFields: []string{"level", "msg"},
	}
	result := expander.ValidateFieldExtraction(fe)
	if !result.Valid {
		t.Errorf("Expected valid, got errors: %v", result.Errors)
	}

	// Invalid regex
	fe.Pattern = `^(?P<level>`
	result = expander.ValidateFieldExtraction(fe)
	if result.Valid {
		t.Error("Expected invalid for bad regex")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error message for bad regex")
	}

	// Warning for no named groups
	fe.Pattern = `^\w+: .*$`
	result = expander.ValidateFieldExtraction(fe)
	if !result.Valid {
		t.Error("Expected valid even without named groups")
	}
	if len(result.Warning) == 0 {
		t.Error("Expected warning for no named groups")
	}
}

func TestExpander_ValidateMacro(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Valid macro
	m := &Macro{
		Expression: "logs | where duration > $1",
		Args:       []string{"threshold"},
	}
	result := expander.ValidateMacro(m)
	if !result.Valid {
		t.Errorf("Expected valid, got errors: %v", result.Errors)
	}

	// Empty expression
	m.Expression = ""
	result = expander.ValidateMacro(m)
	if result.Valid {
		t.Error("Expected invalid for empty expression")
	}

	// Unused argument
	m.Expression = "logs | where level = \"error\""
	m.Args = []string{"unused"}
	result = expander.ValidateMacro(m)
	if !result.Valid {
		t.Error("Expected valid even with unused arg")
	}
	if len(result.Warning) == 0 {
		t.Error("Expected warning for unused argument")
	}
}

func TestExpander_ValidateLookup(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	// Valid lookup
	l := &Lookup{
		KeyField:     "status",
		OutputFields: []string{"description"},
		Data: map[string]map[string]string{
			"200": {"description": "OK"},
		},
	}
	result := expander.ValidateLookup(l)
	if !result.Valid {
		t.Errorf("Expected valid, got errors: %v", result.Errors)
	}

	// Empty key field
	l.KeyField = ""
	result = expander.ValidateLookup(l)
	if result.Valid {
		t.Error("Expected invalid for empty key field")
	}

	// Missing output field in data
	l.KeyField = "status"
	l.OutputFields = []string{"description", "missing"}
	result = expander.ValidateLookup(l)
	if !result.Valid {
		t.Error("Expected valid even with missing output field")
	}
	if len(result.Warning) == 0 {
		t.Error("Expected warning for missing output field")
	}
}

func TestExpander_JSONExtraction(t *testing.T) {
	expander, cleanup := setupTestExpander(t)
	defer cleanup()

	rows := []Row{
		{"message": `Processing request {"user_id": "123", "action": "login", "success": true}`},
	}

	result, err := expander.ApplyFieldExtraction("json_extract", rows)
	if err != nil {
		t.Fatalf("Failed to apply JSON extraction: %v", err)
	}

	row := result[0]
	if row["user_id"] != "123" {
		t.Errorf("Expected user_id 123, got %v", row["user_id"])
	}
	if row["action"] != "login" {
		t.Errorf("Expected action login, got %v", row["action"])
	}
	if row["success"] != true {
		t.Errorf("Expected success true, got %v", row["success"])
	}
}
