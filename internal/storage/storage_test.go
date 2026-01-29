package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("creates store with valid path", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := New(dbPath)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}
		defer store.Close()

		if store.db == nil {
			t.Error("expected db to be initialized")
		}
	})

	t.Run("creates tables on initialization", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.db")

		store, err := New(dbPath)
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}
		defer store.Close()

		// Check that tables exist
		tables := []string{"system_metrics", "endpoint_metrics", "connection_metrics"}
		for _, table := range tables {
			var count int
			err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
			if err != nil {
				t.Errorf("table %s does not exist: %v", table, err)
			}
		}
	})

	t.Run("fails with invalid path", func(t *testing.T) {
		// Try to create a database in a non-existent directory
		_, err := New("/nonexistent/path/that/should/fail/test.db")
		if err == nil {
			t.Error("expected error with invalid path")
		}
	})
}

func TestRecordSystemMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	tests := []struct {
		name        string
		cpu         float64
		mem         float64
		diskRead    float64
		diskWrite   float64
		netRx       float64
		netTx       float64
		load1       float64
		expectError bool
	}{
		{
			name:      "typical values",
			cpu:       45.5,
			mem:       60.2,
			diskRead:  1024.0,
			diskWrite: 512.0,
			netRx:     2048.0,
			netTx:     1024.0,
			load1:     2.5,
		},
		{
			name:      "zero values",
			cpu:       0.0,
			mem:       0.0,
			diskRead:  0.0,
			diskWrite: 0.0,
			netRx:     0.0,
			netTx:     0.0,
			load1:     0.0,
		},
		{
			name:      "high values",
			cpu:       100.0,
			mem:       99.9,
			diskRead:  1000000.0,
			diskWrite: 500000.0,
			netRx:     10000000.0,
			netTx:     5000000.0,
			load1:     50.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.RecordSystemMetrics(tt.cpu, tt.mem, tt.diskRead, tt.diskWrite, tt.netRx, tt.netTx, tt.load1)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetSystemMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	// Record some test data
	testData := []struct {
		cpu, mem, diskRead, diskWrite, netRx, netTx, load1 float64
	}{
		{10.0, 20.0, 100.0, 50.0, 200.0, 100.0, 1.0},
		{20.0, 30.0, 200.0, 100.0, 400.0, 200.0, 2.0},
		{30.0, 40.0, 300.0, 150.0, 600.0, 300.0, 3.0},
	}

	for _, d := range testData {
		err := store.RecordSystemMetrics(d.cpu, d.mem, d.diskRead, d.diskWrite, d.netRx, d.netTx, d.load1)
		if err != nil {
			t.Fatalf("failed to record metrics: %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("retrieves metrics within time range", func(t *testing.T) {
		points, err := store.GetSystemMetrics(time.Hour)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 3 {
			t.Errorf("expected 3 points, got %d", len(points))
		}
	})

	t.Run("empty result for future time range", func(t *testing.T) {
		// Using a very short duration that excludes all data
		// Since data was just recorded, using a tiny duration should work
		points, err := store.GetSystemMetrics(time.Nanosecond)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 0 {
			t.Errorf("expected 0 points for tiny duration, got %d", len(points))
		}
	})

	t.Run("verifies metric values", func(t *testing.T) {
		points, err := store.GetSystemMetrics(time.Hour)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		// Check first point values
		if len(points) > 0 {
			p := points[0]
			if p.CPUPercent != 10.0 {
				t.Errorf("expected CPU 10.0, got %f", p.CPUPercent)
			}
			if p.MemPercent != 20.0 {
				t.Errorf("expected Mem 20.0, got %f", p.MemPercent)
			}
		}
	})
}

func TestRecordEndpointMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	tests := []struct {
		name     string
		method   string
		path     string
		reqCount int64
		errCount int64
		p50      float64
		p99      float64
	}{
		{
			name:     "GET request",
			method:   "GET",
			path:     "/api/users",
			reqCount: 100,
			errCount: 5,
			p50:      10.5,
			p99:      50.2,
		},
		{
			name:     "POST request",
			method:   "POST",
			path:     "/api/orders",
			reqCount: 50,
			errCount: 0,
			p50:      20.0,
			p99:      100.0,
		},
		{
			name:     "empty path",
			method:   "DELETE",
			path:     "",
			reqCount: 10,
			errCount: 2,
			p50:      5.0,
			p99:      15.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.RecordEndpointMetrics(tt.method, tt.path, tt.reqCount, tt.errCount, tt.p50, tt.p99)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetEndpointMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	// Record test data - note: the GetEndpointMetrics query filters by timestamp > cutoff
	// We need to ensure the timestamp in the record is after the cutoff
	err := store.RecordEndpointMetrics("GET", "/api/health", 100, 0, 5.0, 20.0)
	if err != nil {
		t.Fatalf("failed to record metrics: %v", err)
	}
	// Small delay to ensure timestamp is recorded
	time.Sleep(50 * time.Millisecond)

	t.Run("retrieves matching endpoint", func(t *testing.T) {
		// Query with a duration that definitely includes our data
		points, err := store.GetEndpointMetrics(24*time.Hour, "GET", "/api/health")
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) < 1 {
			t.Errorf("expected at least 1 point, got %d", len(points))
		}

		if len(points) > 0 {
			if points[0].Method != "GET" {
				t.Errorf("expected method GET, got %s", points[0].Method)
			}
			if points[0].RequestCount != 100 {
				t.Errorf("expected request count 100, got %d", points[0].RequestCount)
			}
		}
	})

	t.Run("empty result for non-matching endpoint", func(t *testing.T) {
		points, err := store.GetEndpointMetrics(24*time.Hour, "POST", "/api/other")
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 0 {
			t.Errorf("expected 0 points, got %d", len(points))
		}
	})
}

func TestRecordConnectionMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	tests := []struct {
		name       string
		totalConns int64
		totalReqs  int64
		totalErrs  int64
	}{
		{"typical load", 100, 1000, 10},
		{"zero values", 0, 0, 0},
		{"high load", 10000, 1000000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.RecordConnectionMetrics(tt.totalConns, tt.totalReqs, tt.totalErrs)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetConnectionMetrics(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	// Record test data
	for i := 0; i < 5; i++ {
		err := store.RecordConnectionMetrics(int64(i*10), int64(i*100), int64(i))
		if err != nil {
			t.Fatalf("failed to record metrics: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("retrieves all metrics within range", func(t *testing.T) {
		points, err := store.GetConnectionMetrics(time.Hour)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 5 {
			t.Errorf("expected 5 points, got %d", len(points))
		}
	})

	t.Run("metrics are ordered by timestamp", func(t *testing.T) {
		points, err := store.GetConnectionMetrics(time.Hour)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		for i := 1; i < len(points); i++ {
			if points[i].Timestamp.Before(points[i-1].Timestamp) {
				t.Error("metrics not ordered by timestamp")
			}
		}
	})
}

func TestGetConnectionMetricsByTimeRange(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	// Record test data with known timestamps
	baseTime := time.Now()
	for i := 0; i < 3; i++ {
		err := store.RecordConnectionMetrics(int64(i*10), int64(i*100), int64(i))
		if err != nil {
			t.Fatalf("failed to record metrics: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Run("retrieves metrics in time range", func(t *testing.T) {
		start := baseTime.Add(-time.Second)
		end := time.Now().Add(time.Second)

		points, err := store.GetConnectionMetricsByTimeRange(start, end)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 3 {
			t.Errorf("expected 3 points, got %d", len(points))
		}
	})

	t.Run("empty result for past time range", func(t *testing.T) {
		start := baseTime.Add(-2 * time.Hour)
		end := baseTime.Add(-time.Hour)

		points, err := store.GetConnectionMetricsByTimeRange(start, end)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 0 {
			t.Errorf("expected 0 points, got %d", len(points))
		}
	})
}

func TestGetSystemMetricsByTimeRange(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	baseTime := time.Now()
	for i := 0; i < 3; i++ {
		err := store.RecordSystemMetrics(float64(i*10), float64(i*20), 0, 0, 0, 0, 0)
		if err != nil {
			t.Fatalf("failed to record metrics: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Run("retrieves metrics in time range", func(t *testing.T) {
		start := baseTime.Add(-time.Second)
		end := time.Now().Add(time.Second)

		points, err := store.GetSystemMetricsByTimeRange(start, end)
		if err != nil {
			t.Fatalf("failed to get metrics: %v", err)
		}

		if len(points) != 3 {
			t.Errorf("expected 3 points, got %d", len(points))
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	store := setupTestStore(t)
	defer store.Close()

	// Test concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 10; j++ {
				store.RecordSystemMetrics(float64(n), float64(j), 0, 0, 0, 0, 0)
				store.RecordConnectionMetrics(int64(n), int64(j), 0)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data was written
	points, err := store.GetSystemMetrics(time.Hour)
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}

	if len(points) != 100 {
		t.Errorf("expected 100 points, got %d", len(points))
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// Close should succeed
	err = store.Close()
	if err != nil {
		t.Errorf("close failed: %v", err)
	}

	// Operations after close should fail
	err = store.RecordSystemMetrics(1, 2, 3, 4, 5, 6, 7)
	if err == nil {
		t.Error("expected error after close")
	}
}

// Helper function to set up a test store
func setupTestStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// Benchmarks

func BenchmarkRecordSystemMetrics(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	store, err := New(dbPath)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.RecordSystemMetrics(45.5, 60.2, 1024.0, 512.0, 2048.0, 1024.0, 2.5)
	}
}

func BenchmarkGetSystemMetrics(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	store, err := New(dbPath)
	if err != nil {
		b.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Seed data
	for i := 0; i < 1000; i++ {
		store.RecordSystemMetrics(float64(i), float64(i), 0, 0, 0, 0, 0)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.GetSystemMetrics(time.Hour)
	}
}
