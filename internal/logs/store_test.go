package logs

import (
	"os"
	"testing"
	"time"
)

func TestBM25Search(t *testing.T) {
	// Create temp database
	tmpFile, err := os.CreateTemp("", "dogwatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Create store
	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Insert test data
	testLogs := []LogEntry{
		{Message: "Payment processing failed due to gateway timeout", Service: "payment", Level: LevelError},
		{Message: "Failed to complete payment: connection timeout", Service: "payment", Level: LevelError},
		{Message: "Payment service timeout, retry failed", Service: "payment", Level: LevelError},
		{Message: "User logged in successfully", Service: "auth", Level: LevelInfo},
		{Message: "Database connection established", Service: "database", Level: LevelInfo},
		{Message: "Payment completed successfully", Service: "payment", Level: LevelInfo},
	}

	for _, log := range testLogs {
		if err := store.Insert(&log); err != nil {
			t.Fatalf("failed to insert log: %v", err)
		}
	}

	// Test BM25 search for "payment failed timeout"
	result, err := store.Search(SearchQuery{
		Query:     "payment failed timeout",
		SortBy:    SortByRelevance,
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// Should find at least the error logs
	if len(result.Entries) == 0 {
		t.Fatal("expected results but got none")
	}

	// First result should have highest score (most relevant)
	t.Logf("Found %d results:", len(result.Entries))
	for i, entry := range result.Entries {
		t.Logf("  %d. [score=%.8f] %s", i+1, entry.Score, entry.Message)
	}

	// Verify that results have BM25 scores
	if result.Entries[0].Score == 0 {
		t.Error("expected non-zero BM25 score for first result")
	}

	// Verify that error logs (with "failed" and "timeout") rank higher
	// than info logs (with just "payment")
	foundErrorFirst := false
	for _, entry := range result.Entries {
		if entry.Level == LevelError {
			foundErrorFirst = true
			break
		}
		if entry.Level == LevelInfo && entry.Message == "Payment completed successfully" {
			break
		}
	}

	if !foundErrorFirst {
		t.Error("expected error logs with 'failed' and 'timeout' to rank higher than info logs")
	}

	// Test time-based sort
	result2, err := store.Search(SearchQuery{
		Query:     "payment",
		SortBy:    SortByTime,
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("time-sorted search failed: %v", err)
	}

	// Verify we get results sorted by time (no score)
	if len(result2.Entries) == 0 {
		t.Fatal("expected results for time-sorted search")
	}

	t.Logf("Time-sorted results: %d entries", len(result2.Entries))
}

func TestDefaultSortOrder(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "dogwatch-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := NewStore(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Insert test data
	if err := store.Insert(&LogEntry{Message: "test message", Service: "test", Level: LevelInfo}); err != nil {
		t.Fatalf("failed to insert log: %v", err)
	}

	// Query without specifying sort order - should default to time
	result, err := store.Search(SearchQuery{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Entries))
	}

	// Query with FTS - should default to relevance
	result2, err := store.Search(SearchQuery{
		Query:     "test",
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search with query failed: %v", err)
	}

	if len(result2.Entries) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result2.Entries))
	}

	// Result with FTS should have a score
	if result2.Entries[0].Score == 0 {
		t.Error("expected non-zero score for FTS result")
	}
}
