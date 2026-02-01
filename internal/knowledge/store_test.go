package knowledge

import (
	"os"
	"testing"
)

func TestStore_CRUD(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "knowledge_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create store
	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test Create
	obj := &KnowledgeObject{
		Name:        "test_macro",
		Type:        TypeMacro,
		Description: "Test macro",
		Tags:        []string{"test"},
		Owner:       "testuser",
		Shared:      true,
	}
	macro := Macro{
		Expression: "logs | where level = \"error\"",
		Args:       nil,
	}
	obj.SetDefinition(macro)

	err = store.Create(obj)
	if err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}
	if obj.ID == "" {
		t.Error("Expected ID to be set")
	}

	// Test Get
	retrieved, err := store.Get(obj.ID)
	if err != nil {
		t.Fatalf("Failed to get object: %v", err)
	}
	if retrieved.Name != obj.Name {
		t.Errorf("Expected name %s, got %s", obj.Name, retrieved.Name)
	}
	if retrieved.Type != TypeMacro {
		t.Errorf("Expected type %s, got %s", TypeMacro, retrieved.Type)
	}

	// Test GetByName
	byName, err := store.GetByName("test_macro")
	if err != nil {
		t.Fatalf("Failed to get by name: %v", err)
	}
	if byName.ID != obj.ID {
		t.Errorf("Expected ID %s, got %s", obj.ID, byName.ID)
	}

	// Test Update
	obj.Description = "Updated description"
	err = store.Update(obj)
	if err != nil {
		t.Fatalf("Failed to update object: %v", err)
	}

	updated, _ := store.Get(obj.ID)
	if updated.Description != "Updated description" {
		t.Errorf("Expected updated description, got %s", updated.Description)
	}

	// Test List
	objects, err := store.List(ListFilters{})
	if err != nil {
		t.Fatalf("Failed to list objects: %v", err)
	}
	// Should have builtins + our test object
	if len(objects) < 1 {
		t.Error("Expected at least 1 object")
	}

	// Test List with type filter
	macros, err := store.List(ListFilters{Type: TypeMacro})
	if err != nil {
		t.Fatalf("Failed to list macros: %v", err)
	}
	found := false
	for _, m := range macros {
		if m.ID == obj.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find test macro in list")
	}

	// Test Search
	results, err := store.Search("test")
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}
	found = false
	for _, r := range results {
		if r.ID == obj.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find test macro in search results")
	}

	// Test Delete
	err = store.Delete(obj.ID)
	if err != nil {
		t.Fatalf("Failed to delete object: %v", err)
	}

	_, err = store.Get(obj.ID)
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestStore_DuplicateName(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	obj1 := &KnowledgeObject{
		Name:       "duplicate_test",
		Type:       TypeMacro,
		Definition: `{"expression": "test"}`,
	}
	err = store.Create(obj1)
	if err != nil {
		t.Fatalf("Failed to create first object: %v", err)
	}

	obj2 := &KnowledgeObject{
		Name:       "duplicate_test",
		Type:       TypeMacro,
		Definition: `{"expression": "test2"}`,
	}
	err = store.Create(obj2)
	if err != ErrDuplicateName {
		t.Errorf("Expected ErrDuplicateName, got %v", err)
	}
}

func TestStore_Builtins(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Check builtin macros
	macros, err := store.GetAllMacros()
	if err != nil {
		t.Fatalf("Failed to get macros: %v", err)
	}

	expectedMacros := []string{"error_logs", "slow_requests", "error_rate"}
	for _, expected := range expectedMacros {
		found := false
		for _, m := range macros {
			if m.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected builtin macro %s not found", expected)
		}
	}

	// Check builtin field extractions
	extractions, err := store.GetAllFieldExtractions()
	if err != nil {
		t.Fatalf("Failed to get field extractions: %v", err)
	}

	expectedFE := []string{"apache_combined", "json_extract", "nginx_combined", "syslog"}
	for _, expected := range expectedFE {
		found := false
		for _, fe := range extractions {
			if fe.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected builtin field extraction %s not found", expected)
		}
	}

	// Check builtin lookups
	lookups, err := store.GetAllLookups()
	if err != nil {
		t.Fatalf("Failed to get lookups: %v", err)
	}

	expectedLookups := []string{"http_status_codes", "log_levels"}
	for _, expected := range expectedLookups {
		found := false
		for _, l := range lookups {
			if l.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected builtin lookup %s not found", expected)
		}
	}
}

func TestKnowledgeObject_ParseTypes(t *testing.T) {
	// Test Macro parsing
	macroObj := &KnowledgeObject{
		Type:       TypeMacro,
		Definition: `{"expression": "logs | where level = \"error\"", "args": ["threshold"]}`,
	}
	macro, err := macroObj.ParseMacro()
	if err != nil {
		t.Fatalf("Failed to parse macro: %v", err)
	}
	if macro.Expression != `logs | where level = "error"` {
		t.Errorf("Unexpected expression: %s", macro.Expression)
	}
	if len(macro.Args) != 1 || macro.Args[0] != "threshold" {
		t.Errorf("Unexpected args: %v", macro.Args)
	}

	// Test FieldExtraction parsing
	feObj := &KnowledgeObject{
		Type:       TypeFieldExtraction,
		Definition: `{"pattern": "^(?P<level>\\w+): (?P<msg>.*)$", "source_field": "message", "target_fields": ["level", "msg"]}`,
	}
	fe, err := feObj.ParseFieldExtraction()
	if err != nil {
		t.Fatalf("Failed to parse field extraction: %v", err)
	}
	if fe.SourceField != "message" {
		t.Errorf("Unexpected source field: %s", fe.SourceField)
	}

	// Test Lookup parsing
	lookupObj := &KnowledgeObject{
		Type:       TypeLookup,
		Definition: `{"key_field": "status", "output_fields": ["description"], "data": {"200": {"description": "OK"}}}`,
	}
	lookup, err := lookupObj.ParseLookup()
	if err != nil {
		t.Fatalf("Failed to parse lookup: %v", err)
	}
	if lookup.KeyField != "status" {
		t.Errorf("Unexpected key field: %s", lookup.KeyField)
	}
	if lookup.Data["200"]["description"] != "OK" {
		t.Errorf("Unexpected lookup data")
	}

	// Test SavedSearch parsing
	ssObj := &KnowledgeObject{
		Type:       TypeSavedSearch,
		Definition: `{"query": "logs | where level = \"error\"", "schedule": "0 * * * *", "alert_on": "count > 10"}`,
	}
	ss, err := ssObj.ParseSavedSearch()
	if err != nil {
		t.Fatalf("Failed to parse saved search: %v", err)
	}
	if ss.Schedule != "0 * * * *" {
		t.Errorf("Unexpected schedule: %s", ss.Schedule)
	}

	// Test type mismatch
	macroObj.Type = TypeLookup
	_, err = macroObj.ParseMacro()
	if err != ErrTypeMismatch {
		t.Errorf("Expected ErrTypeMismatch, got %v", err)
	}
}
