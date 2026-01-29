package testutil

import (
	"errors"
	"testing"
	"time"
)

func TestTestDB(t *testing.T) {
	t.Run("creates and closes database", func(t *testing.T) {
		db := NewTestDB(t)
		if db.DB == nil {
			t.Error("DB should not be nil")
		}
		if db.Path == "" {
			t.Error("Path should not be empty")
		}
		// Close is handled by t.Cleanup
	})

	t.Run("executes SQL", func(t *testing.T) {
		db := NewTestDB(t)
		db.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
		db.Exec("INSERT INTO test (name) VALUES (?)", "test1")
		db.AssertRowCount("test", 1)
	})
}

func TestAssertNoError(t *testing.T) {
	// This should not fail
	AssertNoError(t, nil)
}

func TestAssertError(t *testing.T) {
	AssertError(t, errors.New("expected error"))
}

func TestAssertEqual(t *testing.T) {
	AssertEqual(t, 5, 5)
	AssertEqual(t, "hello", "hello")
	AssertEqual(t, true, true)
}

func TestAssertNotEqual(t *testing.T) {
	AssertNotEqual(t, 5, 10)
	AssertNotEqual(t, "hello", "world")
}

func TestAssertTrue(t *testing.T) {
	AssertTrue(t, true, "should be true")
	AssertTrue(t, 5 > 3, "5 should be greater than 3")
}

func TestAssertFalse(t *testing.T) {
	AssertFalse(t, false, "should be false")
	AssertFalse(t, 3 > 5, "3 should not be greater than 5")
}

func TestAssertContains(t *testing.T) {
	AssertContains(t, "hello world", "world")
	AssertContains(t, "hello world", "hello")
	AssertContains(t, "test", "test")
}

func TestWaitFor(t *testing.T) {
	t.Run("succeeds when condition becomes true", func(t *testing.T) {
		counter := 0
		err := WaitFor(time.Second, 10*time.Millisecond, func() bool {
			counter++
			return counter >= 3
		})
		if err != nil {
			t.Errorf("WaitFor should have succeeded: %v", err)
		}
	})

	t.Run("times out when condition never true", func(t *testing.T) {
		err := WaitFor(50*time.Millisecond, 10*time.Millisecond, func() bool {
			return false
		})
		if err == nil {
			t.Error("WaitFor should have timed out")
		}
	})
}

func TestMockTime(t *testing.T) {
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	mock := NewMockTime(start)

	t.Run("returns initial time", func(t *testing.T) {
		if !mock.Now().Equal(start) {
			t.Errorf("expected %v, got %v", start, mock.Now())
		}
	})

	t.Run("advances time", func(t *testing.T) {
		mock.Advance(time.Hour)
		expected := start.Add(time.Hour)
		if !mock.Now().Equal(expected) {
			t.Errorf("expected %v, got %v", expected, mock.Now())
		}
	})

	t.Run("sets time", func(t *testing.T) {
		newTime := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		mock.Set(newTime)
		if !mock.Now().Equal(newTime) {
			t.Errorf("expected %v, got %v", newTime, mock.Now())
		}
	})
}

func TestTempDir(t *testing.T) {
	dir := TempDir(t)
	if dir == "" {
		t.Error("TempDir should return non-empty path")
	}
}

func TestTempFile(t *testing.T) {
	content := "test content"
	path := TempFile(t, content)
	if path == "" {
		t.Error("TempFile should return non-empty path")
	}
}
