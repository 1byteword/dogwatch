package storage

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestLocalBackend(t *testing.T) {
	tmpDir := t.TempDir()

	backend, err := NewLocalBackend(tmpDir, "")
	if err != nil {
		t.Fatalf("Failed to create local backend: %v", err)
	}

	ctx := context.Background()

	t.Run("Put and Get", func(t *testing.T) {
		data := []byte("test data for local backend")
		key := "test/file.txt"

		err := backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		reader, err := backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		defer reader.Close()

		retrieved, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if !bytes.Equal(data, retrieved) {
			t.Errorf("Data mismatch: expected %s, got %s", data, retrieved)
		}
	})

	t.Run("Exists", func(t *testing.T) {
		key := "exists/test.txt"
		data := []byte("exists test")

		exists, _ := backend.Exists(ctx, key)
		if exists {
			t.Error("Expected key to not exist initially")
		}

		backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))

		exists, err := backend.Exists(ctx, key)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("Expected key to exist after Put")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := "delete/test.txt"
		data := []byte("delete test")

		backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))

		err := backend.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		exists, _ := backend.Exists(ctx, key)
		if exists {
			t.Error("Expected key to not exist after Delete")
		}
	})

	t.Run("List", func(t *testing.T) {
		// Put some files
		backend.Put(ctx, "list/a.txt", bytes.NewReader([]byte("a")), 1)
		backend.Put(ctx, "list/b.txt", bytes.NewReader([]byte("bb")), 2)
		backend.Put(ctx, "list/sub/c.txt", bytes.NewReader([]byte("ccc")), 3)

		objects, err := backend.List(ctx, "list")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		if len(objects) < 3 {
			t.Errorf("Expected at least 3 objects, got %d", len(objects))
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		_, err := backend.Get(ctx, "nonexistent/file.txt")
		if err == nil {
			t.Error("Expected error for nonexistent key")
		}
	})
}

func TestLocalBackendWithPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	backend, err := NewLocalBackend(tmpDir, "myprefix")
	if err != nil {
		t.Fatalf("Failed to create local backend: %v", err)
	}

	ctx := context.Background()
	key := "prefixed.txt"
	data := []byte("prefixed data")

	err = backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	reader, err := backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()

	retrieved, _ := io.ReadAll(reader)
	if !bytes.Equal(data, retrieved) {
		t.Errorf("Data mismatch with prefix")
	}
}

func TestBackendConfig(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("LocalBackend", func(t *testing.T) {
		config := BackendConfig{
			Type: "local",
			Path: tmpDir,
		}

		backend, err := NewBackend(config)
		if err != nil {
			t.Fatalf("NewBackend failed: %v", err)
		}

		if backend.Name() != "local" {
			t.Errorf("Expected name 'local', got %s", backend.Name())
		}
	})

	t.Run("EmptyTypeDefaultsToLocal", func(t *testing.T) {
		config := BackendConfig{
			Type: "",
			Path: tmpDir,
		}

		backend, err := NewBackend(config)
		if err != nil {
			t.Fatalf("NewBackend failed: %v", err)
		}

		if backend.Name() != "local" {
			t.Errorf("Expected name 'local', got %s", backend.Name())
		}
	})

	t.Run("UnknownTypeError", func(t *testing.T) {
		config := BackendConfig{
			Type: "unknown",
		}

		_, err := NewBackend(config)
		if err == nil {
			t.Error("Expected error for unknown backend type")
		}
	})
}

func TestS3BackendConfig(t *testing.T) {
	t.Run("MissingBucket", func(t *testing.T) {
		config := BackendConfig{
			Type: "s3",
		}

		_, err := NewS3Backend(config)
		if err == nil {
			t.Error("Expected error for missing bucket")
		}
	})

	t.Run("ValidConfig", func(t *testing.T) {
		config := BackendConfig{
			Type:      "s3",
			Bucket:    "test-bucket",
			Region:    "us-west-2",
			AccessKey: "test-key",
			SecretKey: "test-secret",
		}

		backend, err := NewS3Backend(config)
		if err != nil {
			t.Fatalf("NewS3Backend failed: %v", err)
		}

		if backend.Name() != "s3" {
			t.Errorf("Expected name 's3', got %s", backend.Name())
		}
	})

	t.Run("DefaultRegion", func(t *testing.T) {
		config := BackendConfig{
			Type:   "s3",
			Bucket: "test-bucket",
			// No region specified
		}

		backend, err := NewS3Backend(config)
		if err != nil {
			t.Fatalf("NewS3Backend failed: %v", err)
		}

		if backend.region != "us-east-1" {
			t.Errorf("Expected default region 'us-east-1', got %s", backend.region)
		}
	})
}

func TestGCSBackendConfig(t *testing.T) {
	t.Run("MissingBucket", func(t *testing.T) {
		config := BackendConfig{
			Type: "gcs",
		}

		_, err := NewGCSBackend(config)
		if err == nil {
			t.Error("Expected error for missing bucket")
		}
	})

	t.Run("ValidConfig", func(t *testing.T) {
		config := BackendConfig{
			Type:      "gcs",
			Bucket:    "test-bucket",
			AccessKey: "test-key",
			SecretKey: "test-secret",
		}

		backend, err := NewGCSBackend(config)
		if err != nil {
			t.Fatalf("NewGCSBackend failed: %v", err)
		}

		if backend.Name() != "gcs" {
			t.Errorf("Expected name 'gcs', got %s", backend.Name())
		}
	})
}

func TestAsyncUploader(t *testing.T) {
	tmpDir := t.TempDir()

	backend, _ := NewLocalBackend(tmpDir, "")
	uploader := NewAsyncUploader(backend, 2, 3)
	defer uploader.Stop()

	ctx := context.Background()

	t.Run("Upload", func(t *testing.T) {
		done := make(chan error, 1)

		uploader.Upload("async/test.txt", []byte("async data"), func(err error) {
			done <- err
		})

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Upload timed out")
		}

		// Verify upload
		exists, _ := backend.Exists(ctx, "async/test.txt")
		if !exists {
			t.Error("Expected file to exist after async upload")
		}
	})

	t.Run("QueueLength", func(t *testing.T) {
		initial := uploader.QueueLength()
		if initial < 0 {
			t.Error("Queue length should not be negative")
		}
	})
}

func TestBackendManager(t *testing.T) {
	tmpDir := t.TempDir()

	manager := NewBackendManager()

	t.Run("RegisterAndGet", func(t *testing.T) {
		backend, _ := NewLocalBackend(tmpDir, "")
		manager.Register("test-local", backend)

		retrieved, ok := manager.Get("test-local")
		if !ok {
			t.Error("Expected to find registered backend")
		}
		if retrieved.Name() != "local" {
			t.Errorf("Expected name 'local', got %s", retrieved.Name())
		}
	})

	t.Run("List", func(t *testing.T) {
		backend, _ := NewLocalBackend(tmpDir, "")
		manager.Register("backend-a", backend)
		manager.Register("backend-b", backend)

		names := manager.List()
		if len(names) < 2 {
			t.Errorf("Expected at least 2 backends, got %d", len(names))
		}
	})

	t.Run("Remove", func(t *testing.T) {
		backend, _ := NewLocalBackend(tmpDir, "")
		manager.Register("to-remove", backend)

		manager.Remove("to-remove")

		_, ok := manager.Get("to-remove")
		if ok {
			t.Error("Expected backend to be removed")
		}
	})

	t.Run("GetNonexistent", func(t *testing.T) {
		_, ok := manager.Get("nonexistent")
		if ok {
			t.Error("Expected to not find nonexistent backend")
		}
	})
}

func TestStorageObject(t *testing.T) {
	obj := StorageObject{
		Key:          "test/key.txt",
		Size:         1024,
		LastModified: time.Now(),
		ETag:         "abc123",
	}

	if obj.Key != "test/key.txt" {
		t.Errorf("Expected key 'test/key.txt', got %s", obj.Key)
	}

	if obj.Size != 1024 {
		t.Errorf("Expected size 1024, got %d", obj.Size)
	}
}
