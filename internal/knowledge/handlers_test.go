package knowledge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// MockKnowledgeHandlers provides a simple HTTP handler for testing
type MockKnowledgeHandlers struct {
	store    *Store
	registry *Registry
	expander *Expander
}

func newMockHandlers(t *testing.T) (*MockKnowledgeHandlers, func()) {
	tmpFile, err := os.CreateTemp("", "knowledge_handlers_test_*.db")
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

	h := &MockKnowledgeHandlers{
		store:    store,
		registry: registry,
		expander: expander,
	}

	cleanup := func() {
		store.Close()
		os.Remove(tmpFile.Name())
	}

	return h, cleanup
}

func (h *MockKnowledgeHandlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/knowledge" && r.Method == http.MethodGet:
		h.listKnowledge(w, r)
	case r.URL.Path == "/api/knowledge" && r.Method == http.MethodPost:
		h.createKnowledge(w, r)
	case r.URL.Path == "/api/knowledge/expand" && r.Method == http.MethodPost:
		h.expandMacros(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *MockKnowledgeHandlers) listKnowledge(w http.ResponseWriter, r *http.Request) {
	filters := ListFilters{}
	if typeFilter := r.URL.Query().Get("type"); typeFilter != "" {
		filters.Type = ObjectType(typeFilter)
	}

	objects, err := h.store.List(filters)
	if err != nil {
		http.Error(w, "Failed to list", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(objects)
}

func (h *MockKnowledgeHandlers) createKnowledge(w http.ResponseWriter, r *http.Request) {
	var obj KnowledgeObject
	if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.store.Create(&obj); err != nil {
		if err == ErrDuplicateName {
			http.Error(w, "Duplicate name", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create", http.StatusInternalServerError)
		return
	}

	h.registry.InvalidateCache()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(obj)
}

func (h *MockKnowledgeHandlers) expandMacros(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	expanded, err := h.expander.ExpandMacros(req.Query)
	resp := map[string]string{
		"original": req.Query,
		"expanded": expanded,
	}
	if err != nil {
		resp["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func TestAPI_ListKnowledge(t *testing.T) {
	h, cleanup := newMockHandlers(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var objects []*KnowledgeObject
	if err := json.NewDecoder(w.Body).Decode(&objects); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have builtins
	if len(objects) == 0 {
		t.Error("Expected builtins to be returned")
	}
}

func TestAPI_ListKnowledgeByType(t *testing.T) {
	h, cleanup := newMockHandlers(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge?type=macro", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var objects []*KnowledgeObject
	if err := json.NewDecoder(w.Body).Decode(&objects); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// All should be macros
	for _, obj := range objects {
		if obj.Type != TypeMacro {
			t.Errorf("Expected type macro, got %s", obj.Type)
		}
	}
}

func TestAPI_CreateKnowledge(t *testing.T) {
	h, cleanup := newMockHandlers(t)
	defer cleanup()

	obj := KnowledgeObject{
		Name:        "test_api_macro",
		Type:        TypeMacro,
		Definition:  `{"expression": "logs | count"}`,
		Description: "Test macro",
	}
	body, _ := json.Marshal(obj)

	req := httptest.NewRequest(http.MethodPost, "/api/knowledge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var created KnowledgeObject
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if created.ID == "" {
		t.Error("Expected ID to be set")
	}
	if created.Name != "test_api_macro" {
		t.Errorf("Expected name test_api_macro, got %s", created.Name)
	}
}

func TestAPI_CreateDuplicate(t *testing.T) {
	h, cleanup := newMockHandlers(t)
	defer cleanup()

	obj := KnowledgeObject{
		Name:       "dup_test",
		Type:       TypeMacro,
		Definition: `{"expression": "logs | count"}`,
	}
	body, _ := json.Marshal(obj)

	// Create first
	req := httptest.NewRequest(http.MethodPost, "/api/knowledge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("First create failed: %d", w.Code)
	}

	// Create duplicate
	body, _ = json.Marshal(obj)
	req = httptest.NewRequest(http.MethodPost, "/api/knowledge", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("Expected status 409 for duplicate, got %d", w.Code)
	}
}

func TestAPI_ExpandMacros(t *testing.T) {
	h, cleanup := newMockHandlers(t)
	defer cleanup()

	reqBody := map[string]string{
		"query": "`error_logs` | count by (service)",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/knowledge/expand", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["original"] != "`error_logs` | count by (service)" {
		t.Errorf("Unexpected original: %s", resp["original"])
	}

	expected := `logs | where level = "error" | count by (service)`
	if resp["expanded"] != expected {
		t.Errorf("Expected expanded:\n%s\nGot:\n%s", expected, resp["expanded"])
	}
}
