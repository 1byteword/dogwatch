package web

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"dogwatch/internal/storage"

	_ "modernc.org/sqlite"
)

// =============================================================================
// Test Setup Helpers
// =============================================================================

type storageTestServer struct {
	mux            *http.ServeMux
	tieringManager *storage.TieringManager
	backendManager *storage.BackendManager
	wal            *storage.WAL
	db             *sql.DB
	tmpDir         string
	cleanupFuncs   []func()
}

func setupStorageTestServer(t *testing.T) *storageTestServer {
	t.Helper()

	ts := &storageTestServer{
		mux:          http.NewServeMux(),
		cleanupFuncs: make([]func(), 0),
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	ts.tmpDir = tmpDir
	ts.cleanupFuncs = append(ts.cleanupFuncs, func() { os.RemoveAll(tmpDir) })

	// Create test database
	dbPath := tmpDir + "/test.db"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	ts.db = db
	ts.cleanupFuncs = append(ts.cleanupFuncs, func() { db.Close() })

	// Create test table
	db.Exec(`CREATE TABLE IF NOT EXISTS test_metrics (
		id INTEGER PRIMARY KEY,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		value REAL
	)`)

	// Initialize tiering manager
	tieringConfig := storage.TieringConfig{
		DataDir:      tmpDir,
		HotRetention: 24 * time.Hour,
	}
	tieringManager, err := storage.NewTieringManager(tieringConfig, db)
	if err != nil {
		t.Fatalf("Failed to create tiering manager: %v", err)
	}
	ts.tieringManager = tieringManager
	ts.cleanupFuncs = append(ts.cleanupFuncs, func() { tieringManager.Stop() })

	// Initialize backend manager
	backendManager := storage.NewBackendManager()
	backend, _ := storage.NewLocalBackend(tmpDir+"/cold", "")
	backendManager.Register("test-local", backend)
	ts.backendManager = backendManager

	// Initialize WAL
	walConfig := storage.WALConfig{
		Dir:            tmpDir + "/wal",
		MaxSegmentSize: 1024 * 1024,
	}
	wal, err := storage.NewWAL(walConfig)
	if err != nil {
		t.Fatalf("Failed to create WAL: %v", err)
	}
	ts.wal = wal
	ts.cleanupFuncs = append(ts.cleanupFuncs, func() { wal.Stop() })

	// Set global handlers
	SetWAL(wal)
	SetTieringManager(tieringManager)
	SetBackendManager(backendManager)

	// Register routes
	RegisterStorageRoutes(ts.mux)

	return ts
}

func (ts *storageTestServer) cleanup() {
	for i := len(ts.cleanupFuncs) - 1; i >= 0; i-- {
		ts.cleanupFuncs[i]()
	}
}

func (ts *storageTestServer) makeRequest(t *testing.T, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	ts.mux.ServeHTTP(w, req)
	return w
}

func assertStorageStatus(t *testing.T, w *httptest.ResponseRecorder, expected int) {
	t.Helper()
	if w.Code != expected {
		t.Errorf("Expected status %d, got %d. Body: %s", expected, w.Code, w.Body.String())
	}
}

// =============================================================================
// WAL Endpoint Tests
// =============================================================================

func TestWALStats_Get(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	// Write some entries to WAL
	for i := 0; i < 10; i++ {
		ts.wal.Write(storage.WALOpInsert, "test_table", []byte("test data"))
	}

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/wal/stats", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var stats storage.WALStats
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if stats.EntriesWritten < 10 {
			t.Errorf("Expected at least 10 entries written, got %d", stats.EntriesWritten)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/wal/stats", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestWALCheckpoint_Post(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	// Write some entries
	for i := 0; i < 5; i++ {
		ts.wal.Write(storage.WALOpInsert, "test_table", []byte("test data"))
	}

	t.Run("successful checkpoint", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/wal/checkpoint", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}

		if _, ok := result["duration_ms"]; !ok {
			t.Error("Expected duration_ms in response")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/wal/checkpoint", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestWALSync_Post(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful sync", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/wal/sync", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/wal/sync", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestWAL_NotConfigured(t *testing.T) {
	// Save original
	originalWAL := walInstance
	walInstance = nil
	defer func() { walInstance = originalWAL }()

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/wal/stats", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 when WAL not configured, got %d", w.Code)
	}
}

// =============================================================================
// Tiering Endpoint Tests
// =============================================================================

func TestTieringStats_Get(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/stats", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var stats map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Check for human-readable sizes
		if _, ok := stats["hot_data_size_human"]; !ok {
			t.Error("Expected hot_data_size_human in response")
		}

		if _, ok := stats["warm_data_size_human"]; !ok {
			t.Error("Expected warm_data_size_human in response")
		}

		if _, ok := stats["cold_data_size_human"]; !ok {
			t.Error("Expected cold_data_size_human in response")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/stats", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringState_Get(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful get", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/state", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var state storage.TieringState
		if err := json.NewDecoder(w.Body).Decode(&state); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// State should be valid (possibly empty)
		if state.WarmFiles == nil {
			// That's OK, just checking structure
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/state", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringCompact_Post(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful compact", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/compact", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/compact", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringTier_Post(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful tier", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/tier", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}

		if _, ok := result["bytes_moved_to_warm"]; !ok {
			t.Error("Expected bytes_moved_to_warm in response")
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/tier", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringWarm_Get(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful get (empty)", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/warm", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var files []interface{}
		if err := json.NewDecoder(w.Body).Decode(&files); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should be empty initially
		if len(files) != 0 {
			t.Logf("Found %d warm files", len(files))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/warm", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringCold_Get(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful get (empty)", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/cold", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var archives []interface{}
		if err := json.NewDecoder(w.Body).Decode(&archives); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should be empty initially
		if len(archives) != 0 {
			t.Logf("Found %d cold archives", len(archives))
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/cold", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTieringRestore_Post(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("missing key", func(t *testing.T) {
		body := map[string]string{}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/restore", body)
		assertStorageStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/storage/tiering/restore", bytes.NewBuffer([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		ts.mux.ServeHTTP(w, req)
		assertStorageStatus(t, w, http.StatusBadRequest)
	})

	t.Run("nonexistent key", func(t *testing.T) {
		body := map[string]string{"key": "nonexistent/key.db.gz"}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/tiering/restore", body)
		// Should fail because the key doesn't exist in cold storage
		assertStorageStatus(t, w, http.StatusInternalServerError)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/tiering/restore", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestTiering_NotConfigured(t *testing.T) {
	// Save original
	originalManager := tieringManager
	tieringManager = nil
	defer func() { tieringManager = originalManager }()

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	endpoints := []string{
		"/api/storage/tiering/stats",
		"/api/storage/tiering/state",
		"/api/storage/tiering/warm",
		"/api/storage/tiering/cold",
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: Expected 503 when tiering not configured, got %d", ep, w.Code)
		}
	}
}

// =============================================================================
// Backend Management Endpoint Tests
// =============================================================================

func TestBackends_List(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful list", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/backends", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var backends []map[string]string
		if err := json.NewDecoder(w.Body).Decode(&backends); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Should have at least the test backend
		if len(backends) < 1 {
			t.Error("Expected at least 1 backend")
		}

		// Check structure
		found := false
		for _, b := range backends {
			if b["name"] == "test-local" {
				found = true
				if b["type"] != "local" {
					t.Errorf("Expected type 'local', got '%s'", b["type"])
				}
			}
		}

		if !found {
			t.Error("Expected to find 'test-local' backend")
		}
	})
}

func TestBackends_Create(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful create", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "new-backend",
			"config": map[string]interface{}{
				"type": "local",
				"path": ts.tmpDir + "/new-backend",
			},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/backends", body)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}

		if result["name"] != "new-backend" {
			t.Errorf("Expected name 'new-backend', got '%v'", result["name"])
		}
	})

	t.Run("missing name", func(t *testing.T) {
		body := map[string]interface{}{
			"config": map[string]interface{}{
				"type": "local",
				"path": ts.tmpDir + "/no-name",
			},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/backends", body)
		assertStorageStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid config", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "invalid-backend",
			"config": map[string]interface{}{
				"type": "unknown-type",
			},
		}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/backends", body)
		assertStorageStatus(t, w, http.StatusBadRequest)
	})
}

func TestBackends_Delete(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	// First create a backend to delete
	createBody := map[string]interface{}{
		"name": "to-delete",
		"config": map[string]interface{}{
			"type": "local",
			"path": ts.tmpDir + "/to-delete",
		},
	}
	ts.makeRequest(t, http.MethodPost, "/api/storage/backends", createBody)

	t.Run("successful delete", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodDelete, "/api/storage/backends?name=to-delete", nil)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Error("Expected success to be true")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodDelete, "/api/storage/backends", nil)
		assertStorageStatus(t, w, http.StatusBadRequest)
	})
}

func TestBackends_Test(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	t.Run("successful test", func(t *testing.T) {
		body := map[string]string{"name": "test-local"}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/backends/test", body)
		assertStorageStatus(t, w, http.StatusOK)

		var result map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if result["success"] != true {
			t.Errorf("Expected success to be true, got %v", result)
		}

		if result["name"] != "test-local" {
			t.Errorf("Expected name 'test-local', got '%v'", result["name"])
		}
	})

	t.Run("backend not found", func(t *testing.T) {
		body := map[string]string{"name": "nonexistent"}
		w := ts.makeRequest(t, http.MethodPost, "/api/storage/backends/test", body)
		assertStorageStatus(t, w, http.StatusNotFound)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := ts.makeRequest(t, http.MethodGet, "/api/storage/backends/test", nil)
		assertStorageStatus(t, w, http.StatusMethodNotAllowed)
	})
}

func TestBackends_NotConfigured(t *testing.T) {
	// Save original
	originalManager := backendManager
	backendManager = nil
	defer func() { backendManager = originalManager }()

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/backends", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 when backend manager not configured, got %d", w.Code)
	}
}

// =============================================================================
// Response Format Tests
// =============================================================================

func TestStorageEndpoints_JSONContentType(t *testing.T) {
	ts := setupStorageTestServer(t)
	defer ts.cleanup()

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/storage/wal/stats"},
		{http.MethodGet, "/api/storage/tiering/stats"},
		{http.MethodGet, "/api/storage/tiering/state"},
		{http.MethodGet, "/api/storage/tiering/warm"},
		{http.MethodGet, "/api/storage/tiering/cold"},
		{http.MethodGet, "/api/storage/backends"},
	}

	for _, ep := range endpoints {
		t.Run(ep.path, func(t *testing.T) {
			w := ts.makeRequest(t, ep.method, ep.path, nil)

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json for %s, got %s", ep.path, contentType)
			}
		})
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkWALStats(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench_*")
	defer os.RemoveAll(tmpDir)

	walConfig := storage.WALConfig{Dir: tmpDir + "/wal"}
	wal, _ := storage.NewWAL(walConfig)
	defer wal.Stop()

	SetWAL(wal)

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/wal/stats", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkTieringStats(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench_*")
	defer os.RemoveAll(tmpDir)

	db, _ := sql.Open("sqlite", tmpDir+"/test.db")
	defer db.Close()

	config := storage.TieringConfig{DataDir: tmpDir}
	manager, _ := storage.NewTieringManager(config, db)
	defer manager.Stop()

	SetTieringManager(manager)

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/tiering/stats", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}

func BenchmarkBackendsList(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "bench_*")
	defer os.RemoveAll(tmpDir)

	manager := storage.NewBackendManager()
	for i := 0; i < 10; i++ {
		backend, _ := storage.NewLocalBackend(tmpDir+"/"+string(rune('a'+i)), "")
		manager.Register(string(rune('a'+i)), backend)
	}

	SetBackendManager(manager)

	mux := http.NewServeMux()
	RegisterStorageRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/storage/backends", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
	}
}
