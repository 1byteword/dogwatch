package knowledge

import (
	"os"
	"testing"
	"time"
)

func TestRegistry_Cache(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_registry_test_*.db")
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

	registry := NewRegistry(store)

	// Should have builtins loaded
	macros := registry.ListMacros()
	if len(macros) == 0 {
		t.Error("Expected builtins to be loaded")
	}

	// Get specific macro
	errorLogs, err := registry.GetMacro("error_logs")
	if err != nil {
		t.Errorf("Failed to get error_logs macro: %v", err)
	}
	if errorLogs.Name != "error_logs" {
		t.Errorf("Expected error_logs, got %s", errorLogs.Name)
	}

	// Get field extraction
	apache, err := registry.GetFieldExtraction("apache_combined")
	if err != nil {
		t.Errorf("Failed to get apache_combined: %v", err)
	}
	if apache.Name != "apache_combined" {
		t.Errorf("Expected apache_combined, got %s", apache.Name)
	}

	// Get lookup
	httpStatus, err := registry.GetLookup("http_status_codes")
	if err != nil {
		t.Errorf("Failed to get http_status_codes: %v", err)
	}
	if httpStatus.Name != "http_status_codes" {
		t.Errorf("Expected http_status_codes, got %s", httpStatus.Name)
	}
}

func TestRegistry_NotFound(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_registry_test_*.db")
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

	registry := NewRegistry(store)

	_, err = registry.GetMacro("nonexistent")
	if err != ErrMacroNotFound {
		t.Errorf("Expected ErrMacroNotFound, got %v", err)
	}

	_, err = registry.GetFieldExtraction("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	_, err = registry.GetLookup("nonexistent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestRegistry_Reload(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_registry_test_*.db")
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

	registry := NewRegistry(store)

	// Add a new macro
	obj := &KnowledgeObject{
		Name:       "new_macro",
		Type:       TypeMacro,
		Definition: `{"expression": "logs | count"}`,
	}
	if err := store.Create(obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Should not be in cache yet
	_, err = registry.GetMacro("new_macro")
	if err != ErrMacroNotFound {
		t.Error("Expected macro to not be in cache before reload")
	}

	// Reload
	if err := registry.Reload(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	// Should be in cache now
	macro, err := registry.GetMacro("new_macro")
	if err != nil {
		t.Errorf("Expected macro to be in cache after reload: %v", err)
	}
	if macro.Name != "new_macro" {
		t.Errorf("Expected new_macro, got %s", macro.Name)
	}
}

func TestRegistry_CacheTTL(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_registry_test_*.db")
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

	registry := NewRegistry(store)

	// Set very short TTL
	registry.SetCacheTTL(50 * time.Millisecond)

	// Add a new macro
	obj := &KnowledgeObject{
		Name:       "ttl_test_macro",
		Type:       TypeMacro,
		Definition: `{"expression": "logs | count"}`,
	}
	if err := store.Create(obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Invalidate the cache so we can test TTL behavior
	registry.InvalidateCache()

	// Wait for TTL to trigger auto-reload on next access
	time.Sleep(60 * time.Millisecond)

	// Should be in cache now after TTL-triggered reload
	_, err = registry.GetMacro("ttl_test_macro")
	if err != nil {
		t.Errorf("Expected macro to be in cache after TTL reload: %v", err)
	}
}

func TestRegistry_InvalidateCache(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "knowledge_registry_test_*.db")
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

	registry := NewRegistry(store)

	// Add a new macro
	obj := &KnowledgeObject{
		Name:       "invalidate_test",
		Type:       TypeMacro,
		Definition: `{"expression": "logs | count"}`,
	}
	if err := store.Create(obj); err != nil {
		t.Fatalf("Failed to create object: %v", err)
	}

	// Invalidate cache
	registry.InvalidateCache()

	// Next access should trigger reload
	macro, err := registry.GetMacro("invalidate_test")
	if err != nil {
		t.Errorf("Expected macro after cache invalidation: %v", err)
	}
	if macro.Name != "invalidate_test" {
		t.Errorf("Expected invalidate_test, got %s", macro.Name)
	}
}
