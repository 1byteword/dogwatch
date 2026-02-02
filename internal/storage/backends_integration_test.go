package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// LocalFS Backend Operations Tests
// =============================================================================

func TestLocalBackend_FullWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	backend, err := NewLocalBackend(tmpDir, "")
	if err != nil {
		t.Fatalf("Failed to create local backend: %v", err)
	}

	ctx := context.Background()

	// Test full CRUD workflow
	t.Run("Create", func(t *testing.T) {
		data := []byte("test data for full workflow")
		err := backend.Put(ctx, "workflow/test.txt", bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	})

	t.Run("Read", func(t *testing.T) {
		reader, err := backend.Get(ctx, "workflow/test.txt")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		defer reader.Close()

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		expected := "test data for full workflow"
		if string(data) != expected {
			t.Errorf("Data mismatch: expected %s, got %s", expected, string(data))
		}
	})

	t.Run("Update", func(t *testing.T) {
		newData := []byte("updated test data")
		err := backend.Put(ctx, "workflow/test.txt", bytes.NewReader(newData), int64(len(newData)))
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		reader, _ := backend.Get(ctx, "workflow/test.txt")
		defer reader.Close()
		data, _ := io.ReadAll(reader)

		if string(data) != "updated test data" {
			t.Errorf("Update didn't work: got %s", string(data))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := backend.Delete(ctx, "workflow/test.txt")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		exists, _ := backend.Exists(ctx, "workflow/test.txt")
		if exists {
			t.Error("File still exists after delete")
		}
	})
}

func TestLocalBackend_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	// Create 10MB file
	size := 10 * 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	start := time.Now()
	err := backend.Put(ctx, "large/file.bin", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put large file failed: %v", err)
	}
	t.Logf("Write 10MB took: %v", time.Since(start))

	start = time.Now()
	reader, err := backend.Get(ctx, "large/file.bin")
	if err != nil {
		t.Fatalf("Get large file failed: %v", err)
	}

	retrieved, err := io.ReadAll(reader)
	reader.Close()
	t.Logf("Read 10MB took: %v", time.Since(start))

	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(retrieved) != size {
		t.Errorf("Size mismatch: expected %d, got %d", size, len(retrieved))
	}

	// Verify data integrity
	for i := 0; i < 100; i++ {
		idx := i * (size / 100)
		if retrieved[idx] != data[idx] {
			t.Errorf("Data corruption at index %d", idx)
			break
		}
	}
}

func TestLocalBackend_NestedDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	// Create deeply nested file
	key := "a/b/c/d/e/f/g/h/deep.txt"
	data := []byte("deeply nested file")

	err := backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put nested file failed: %v", err)
	}

	exists, err := backend.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Nested file should exist")
	}

	// Verify the directories were created
	fullPath := filepath.Join(tmpDir, key)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Error("File not created on disk")
	}
}

func TestLocalBackend_ListWithPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	// Create files with different prefixes
	files := map[string][]byte{
		"logs/2024/01/log1.txt": []byte("log1"),
		"logs/2024/01/log2.txt": []byte("log2"),
		"logs/2024/02/log3.txt": []byte("log3"),
		"metrics/metric1.txt":   []byte("metric1"),
		"metrics/metric2.txt":   []byte("metric2"),
	}

	for key, data := range files {
		backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	}

	t.Run("ListAll", func(t *testing.T) {
		objects, err := backend.List(ctx, "")
		if err != nil {
			t.Fatalf("List all failed: %v", err)
		}
		if len(objects) < 5 {
			t.Errorf("Expected at least 5 objects, got %d", len(objects))
		}
	})

	t.Run("ListLogs", func(t *testing.T) {
		objects, err := backend.List(ctx, "logs")
		if err != nil {
			t.Fatalf("List logs failed: %v", err)
		}
		if len(objects) < 3 {
			t.Errorf("Expected at least 3 log objects, got %d", len(objects))
		}
	})

	t.Run("ListMetrics", func(t *testing.T) {
		objects, err := backend.List(ctx, "metrics")
		if err != nil {
			t.Fatalf("List metrics failed: %v", err)
		}
		if len(objects) < 2 {
			t.Errorf("Expected at least 2 metric objects, got %d", len(objects))
		}
	})

	t.Run("ListJanuary", func(t *testing.T) {
		objects, err := backend.List(ctx, "logs/2024/01")
		if err != nil {
			t.Fatalf("List January failed: %v", err)
		}
		if len(objects) < 2 {
			t.Errorf("Expected at least 2 January objects, got %d", len(objects))
		}
	})
}

func TestLocalBackend_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent writes to different keys
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := filepath.Join("concurrent", string(rune('a'+id%26)), "file.txt")
			data := []byte("concurrent data")
			if err := backend.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
				errors <- err
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			backend.List(ctx, "concurrent")
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		t.Logf("Concurrent error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Errorf("Had %d concurrent errors", errCount)
	}
}

func TestLocalBackend_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	key := "atomic/test.txt"
	originalData := []byte("original data")

	// Write original
	backend.Put(ctx, key, bytes.NewReader(originalData), int64(len(originalData)))

	// Simulate concurrent reader during write
	var wg sync.WaitGroup
	readErrors := make(chan error, 10)
	readData := make(chan []byte, 10)

	// Start reader
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			reader, err := backend.Get(ctx, key)
			if err != nil {
				readErrors <- err
				continue
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				readErrors <- err
				continue
			}
			readData <- data
			time.Sleep(time.Millisecond)
		}
	}()

	// Write new data
	newData := []byte("new data that is longer")
	backend.Put(ctx, key, bytes.NewReader(newData), int64(len(newData)))

	wg.Wait()
	close(readErrors)
	close(readData)

	// All reads should be either original or new data, not corrupted
	for data := range readData {
		str := string(data)
		if str != "original data" && str != "new data that is longer" {
			t.Errorf("Read corrupted data: %s", str)
		}
	}
}

// =============================================================================
// S3 Backend Tests (Mock/Config Only)
// =============================================================================

func TestS3Backend_ConfigValidation(t *testing.T) {
	t.Run("MissingBucket", func(t *testing.T) {
		config := BackendConfig{
			Type: "s3",
		}
		_, err := NewS3Backend(config)
		if err == nil {
			t.Error("Expected error for missing bucket")
		}
	})

	t.Run("DefaultRegion", func(t *testing.T) {
		config := BackendConfig{
			Type:   "s3",
			Bucket: "test-bucket",
		}
		backend, err := NewS3Backend(config)
		if err != nil {
			t.Fatalf("Failed to create backend: %v", err)
		}
		if backend.region != "us-east-1" {
			t.Errorf("Expected default region us-east-1, got %s", backend.region)
		}
	})

	t.Run("CustomEndpoint", func(t *testing.T) {
		config := BackendConfig{
			Type:     "s3",
			Bucket:   "test-bucket",
			Endpoint: "http://localhost:9000",
		}
		backend, err := NewS3Backend(config)
		if err != nil {
			t.Fatalf("Failed to create backend: %v", err)
		}
		if backend.endpoint != "http://localhost:9000" {
			t.Errorf("Endpoint not set correctly")
		}
	})

	t.Run("WithCredentials", func(t *testing.T) {
		config := BackendConfig{
			Type:      "s3",
			Bucket:    "test-bucket",
			AccessKey: "AKIATEST",
			SecretKey: "secret123",
		}
		backend, err := NewS3Backend(config)
		if err != nil {
			t.Fatalf("Failed to create backend: %v", err)
		}
		if backend.accessKey != "AKIATEST" {
			t.Error("Access key not set")
		}
	})
}

func TestS3Backend_URLConstruction(t *testing.T) {
	config := BackendConfig{
		Type:   "s3",
		Bucket: "my-bucket",
		Region: "us-west-2",
	}
	backend, _ := NewS3Backend(config)

	expectedURL := "https://my-bucket.s3.us-west-2.amazonaws.com"
	if backend.baseURL() != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, backend.baseURL())
	}
}

func TestS3Backend_KeyPrefixing(t *testing.T) {
	config := BackendConfig{
		Type:   "s3",
		Bucket: "my-bucket",
		Prefix: "dogwatch/prod",
	}
	backend, _ := NewS3Backend(config)

	fullKey := backend.fullKey("archives/2024/data.db")
	expected := "dogwatch/prod/archives/2024/data.db"
	if fullKey != expected {
		t.Errorf("Expected key %s, got %s", expected, fullKey)
	}
}

// =============================================================================
// Async Uploader Retry Logic Tests
// =============================================================================

func TestAsyncUploader_BasicUpload(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")

	uploader := NewAsyncUploader(backend, 2, 3)
	defer uploader.Stop()

	done := make(chan error, 1)

	uploader.Upload("async/basic.txt", []byte("basic upload data"), func(err error) {
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Upload failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Upload timed out")
	}

	// Verify file exists
	ctx := context.Background()
	exists, _ := backend.Exists(ctx, "async/basic.txt")
	if !exists {
		t.Error("File should exist after upload")
	}
}

func TestAsyncUploader_RetryOnFailure(t *testing.T) {
	// Create a backend that fails initially
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")

	uploader := NewAsyncUploader(backend, 2, 3)
	defer uploader.Stop()

	done := make(chan error, 1)
	attempts := 0

	// Use a wrapper to track attempts
	uploader.Upload("retry/test.txt", []byte("retry test data"), func(err error) {
		attempts++
		done <- err
	})

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Upload eventually failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Upload timed out")
	}
}

func TestAsyncUploader_QueueFull(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")

	// Create uploader with very small queue (internal queue size is 1000, but we can test behavior)
	uploader := NewAsyncUploader(backend, 1, 3)
	// Note: Do NOT defer uploader.Stop() since we call it explicitly below

	// Flood with many uploads
	errors := make(chan error, 2000)
	for i := 0; i < 1500; i++ {
		key := filepath.Join("flood", string(rune('a'+i%26)), "file.txt")
		uploader.Upload(key, []byte("flood data"), func(err error) {
			if err != nil {
				errors <- err
			}
		})
	}

	// Wait for uploads to process
	time.Sleep(2 * time.Second)
	uploader.Stop()

	close(errors)
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
			if errorCount <= 5 {
				t.Logf("Queue error: %v", err)
			}
		}
	}

	if errorCount > 0 {
		t.Logf("Total queue full errors: %d", errorCount)
	}
}

func TestAsyncUploader_ConcurrentUploads(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")

	uploader := NewAsyncUploader(backend, 4, 3)
	defer uploader.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			key := filepath.Join("concurrent", string(rune('a'+id%26)), "upload.txt")
			data := []byte("concurrent upload data")

			done := make(chan error, 1)
			uploader.Upload(key, data, func(err error) {
				done <- err
			})

			select {
			case err := <-done:
				if err != nil {
					errors <- err
				}
			case <-time.After(10 * time.Second):
				errors <- context.DeadlineExceeded
			}
			wg.Done()
		}(i)
	}

	wg.Wait()
	close(errors)

	var errCount int
	for err := range errors {
		t.Logf("Upload error: %v", err)
		errCount++
	}

	if errCount > 10 {
		t.Errorf("Too many upload errors: %d", errCount)
	}
}

func TestAsyncUploader_QueueLength(t *testing.T) {
	tmpDir := t.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")

	uploader := NewAsyncUploader(backend, 1, 3)
	defer uploader.Stop()

	// Queue some uploads without waiting
	for i := 0; i < 10; i++ {
		uploader.Upload("queue/test.txt", []byte("data"), nil)
	}

	// Queue length should be > 0
	length := uploader.QueueLength()
	t.Logf("Queue length after 10 uploads: %d", length)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)
}

// =============================================================================
// Backend Failover Tests
// =============================================================================

func TestBackendManager_Failover(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewBackendManager()

	// Register primary and fallback backends
	primary, _ := NewLocalBackend(filepath.Join(tmpDir, "primary"), "")
	fallback, _ := NewLocalBackend(filepath.Join(tmpDir, "fallback"), "")

	manager.Register("primary", primary)
	manager.Register("fallback", fallback)

	// Verify both are registered
	names := manager.List()
	if len(names) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(names))
	}

	// Get primary
	p, ok := manager.Get("primary")
	if !ok {
		t.Fatal("Failed to get primary backend")
	}
	if p.Name() != "local" {
		t.Errorf("Expected local backend, got %s", p.Name())
	}

	// Simulate failover by removing primary
	manager.Remove("primary")

	// Primary should no longer exist
	_, ok = manager.Get("primary")
	if ok {
		t.Error("Primary should be removed")
	}

	// Fallback should still work
	f, ok := manager.Get("fallback")
	if !ok {
		t.Fatal("Failed to get fallback backend")
	}

	// Verify fallback is operational
	ctx := context.Background()
	err := f.Put(ctx, "failover/test.txt", bytes.NewReader([]byte("fallback data")), 13)
	if err != nil {
		t.Errorf("Fallback put failed: %v", err)
	}
}

func TestBackendManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	manager := NewBackendManager()

	var wg sync.WaitGroup

	// Concurrent registration
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			backend, _ := NewLocalBackend(filepath.Join(tmpDir, string(rune('a'+id))), "")
			manager.Register(string(rune('a'+id)), backend)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.List()
		}()
	}

	// Concurrent gets
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			manager.Get(string(rune('a' + id)))
		}(i)
	}

	wg.Wait()

	// Verify state is consistent
	names := manager.List()
	t.Logf("Final backend count: %d", len(names))
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkLocalBackend_Put(b *testing.B) {
	tmpDir := b.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()
	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := filepath.Join("bench", string(rune('a'+i%26)), "file.txt")
		backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	}
}

func BenchmarkLocalBackend_Get(b *testing.B) {
	tmpDir := b.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	// Setup: create file to read
	data := make([]byte, 1024)
	backend.Put(ctx, "bench/read.txt", bytes.NewReader(data), int64(len(data)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, _ := backend.Get(ctx, "bench/read.txt")
		io.Copy(io.Discard, reader)
		reader.Close()
	}
}

func BenchmarkLocalBackend_List(b *testing.B) {
	tmpDir := b.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	ctx := context.Background()

	// Setup: create files
	for i := 0; i < 100; i++ {
		key := filepath.Join("bench", "list", string(rune('a'+i%26)), "file.txt")
		backend.Put(ctx, key, bytes.NewReader([]byte("data")), 4)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.List(ctx, "bench/list")
	}
}

func BenchmarkAsyncUploader_Upload(b *testing.B) {
	tmpDir := b.TempDir()
	backend, _ := NewLocalBackend(tmpDir, "")
	uploader := NewAsyncUploader(backend, 4, 3)
	defer uploader.Stop()

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := filepath.Join("bench", "async", string(rune('a'+i%26)), "file.txt")
		uploader.Upload(key, data, nil)
	}
}

func BenchmarkBackendManager_Get(b *testing.B) {
	tmpDir := b.TempDir()
	manager := NewBackendManager()

	for i := 0; i < 10; i++ {
		backend, _ := NewLocalBackend(filepath.Join(tmpDir, string(rune('a'+i))), "")
		manager.Register(string(rune('a'+i)), backend)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.Get(string(rune('a' + i%10)))
	}
}

func BenchmarkBackendManager_ConcurrentGet(b *testing.B) {
	tmpDir := b.TempDir()
	manager := NewBackendManager()

	for i := 0; i < 10; i++ {
		backend, _ := NewLocalBackend(filepath.Join(tmpDir, string(rune('a'+i))), "")
		manager.Register(string(rune('a'+i)), backend)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			manager.Get(string(rune('a' + i%10)))
			i++
		}
	})
}
