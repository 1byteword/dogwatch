// Package testutil provides test utilities and helpers for dogwatch tests.
package testutil

import (
	"database/sql"
	"dogwatch/internal/storage"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

)

// TestDB wraps a SQLite database for testing purposes.
// It provides automatic cleanup and helper methods for test assertions.
type TestDB struct {
	DB     *sql.DB
	Path   string
	t      *testing.T
	closed bool
}

// NewTestDB creates a new temporary SQLite database for testing.
// The database is automatically cleaned up when the test completes.
func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	tdb := &TestDB{
		DB:   db,
		Path: dbPath,
		t:    t,
	}

	t.Cleanup(func() {
		if !tdb.closed {
			tdb.Close()
		}
	})

	return tdb
}

// Close closes the database connection.
func (tdb *TestDB) Close() error {
	if tdb.closed {
		return nil
	}
	tdb.closed = true
	return tdb.DB.Close()
}

// Exec executes a SQL statement and fails the test on error.
func (tdb *TestDB) Exec(query string, args ...interface{}) sql.Result {
	tdb.t.Helper()
	result, err := tdb.DB.Exec(query, args...)
	if err != nil {
		tdb.t.Fatalf("failed to execute SQL: %v\nquery: %s", err, query)
	}
	return result
}

// Query executes a query and fails the test on error.
func (tdb *TestDB) Query(query string, args ...interface{}) *sql.Rows {
	tdb.t.Helper()
	rows, err := tdb.DB.Query(query, args...)
	if err != nil {
		tdb.t.Fatalf("failed to query: %v\nquery: %s", err, query)
	}
	return rows
}

// QueryRow executes a query that returns a single row.
func (tdb *TestDB) QueryRow(query string, args ...interface{}) *sql.Row {
	tdb.t.Helper()
	return tdb.DB.QueryRow(query, args...)
}

// AssertRowCount checks that a table has the expected number of rows.
func (tdb *TestDB) AssertRowCount(table string, expected int) {
	tdb.t.Helper()
	var count int
	err := tdb.DB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	if err != nil {
		tdb.t.Fatalf("failed to count rows in %s: %v", table, err)
	}
	if count != expected {
		tdb.t.Errorf("expected %d rows in %s, got %d", expected, table, count)
	}
}

// TestServer wraps httptest.Server for API testing.
type TestServer struct {
	*httptest.Server
	t *testing.T
}

// NewTestServer creates a new test HTTP server.
func NewTestServer(t *testing.T, handler http.Handler) *TestServer {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return &TestServer{
		Server: ts,
		t:      t,
	}
}

// URL returns the server URL with an optional path appended.
func (ts *TestServer) URL(path string) string {
	return ts.Server.URL + path
}

// Get performs a GET request to the server.
func (ts *TestServer) Get(path string) *http.Response {
	ts.t.Helper()
	resp, err := http.Get(ts.URL(path))
	if err != nil {
		ts.t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

// TempDir creates a temporary directory for testing.
func TempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// TempFile creates a temporary file with the given content.
func TempFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected an error, got nil")
	}
}

// AssertEqual fails the test if got != want.
func AssertEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// AssertNotEqual fails the test if got == want.
func AssertNotEqual[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got == want {
		t.Errorf("got %v, want different value", got)
	}
}

// AssertTrue fails the test if condition is false.
func AssertTrue(t *testing.T, condition bool, msg string) {
	t.Helper()
	if !condition {
		t.Errorf("assertion failed: %s", msg)
	}
}

// AssertFalse fails the test if condition is true.
func AssertFalse(t *testing.T, condition bool, msg string) {
	t.Helper()
	if condition {
		t.Errorf("assertion failed (expected false): %s", msg)
	}
}

// AssertContains fails the test if substr is not in s.
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if len(s) == 0 || len(substr) == 0 {
		if substr != "" && s == "" {
			t.Errorf("expected string to contain %q, but string is empty", substr)
		}
		return
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Errorf("expected string to contain %q, got %q", substr, s)
}

// WaitFor waits for a condition to become true, checking every interval.
// Returns an error if the timeout is reached.
func WaitFor(timeout, interval time.Duration, condition func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("timeout waiting for condition")
}

// Eventually retries a function until it succeeds or times out.
func Eventually(t *testing.T, timeout time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		lastErr = fn()
		if lastErr == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("condition not met within %v: %v", timeout, lastErr)
}

// MockTime provides a controllable time source for testing.
type MockTime struct {
	now time.Time
}

// NewMockTime creates a new MockTime set to the given time.
func NewMockTime(t time.Time) *MockTime {
	return &MockTime{now: t}
}

// Now returns the current mock time.
func (m *MockTime) Now() time.Time {
	return m.now
}

// Advance moves the mock time forward by d.
func (m *MockTime) Advance(d time.Duration) {
	m.now = m.now.Add(d)
}

// Set sets the mock time to t.
func (m *MockTime) Set(t time.Time) {
	m.now = t
}
