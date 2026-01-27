package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DatabaseFiles lists all SQLite databases to backup
var DatabaseFiles = []string{
	"metrics.db",
	"traces.db",
	"watches.db",
	"dashboards.db",
	"logs.db",
	"custom_metrics.db",
	"dbwatch.db",
	"synthetics.db",
	"slos.db",
	"deploys.db",
	"incidents.db",
	"oncall.db",
	"rbac.db",
	"notify.db",
	"audit.db",
	"sso.db",
	"alerting.db",
	"costintel.db",
	"shaping.db",
	"quotas.db",
	"logreduce.db",
}

// Metadata contains backup metadata
type Metadata struct {
	Version     string    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	DataDir     string    `json:"data_dir"`
	Files       []FileInfo `json:"files"`
	TotalSize   int64     `json:"total_size"`
	Hostname    string    `json:"hostname,omitempty"`
}

// FileInfo describes a backed up file
type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// BackupOptions configures backup behavior
type BackupOptions struct {
	DataDir    string
	OutputPath string
	Compress   bool
}

// RestoreOptions configures restore behavior
type RestoreOptions struct {
	BackupPath string
	DataDir    string
	Force      bool // Overwrite existing files
}

// BackupResult contains backup operation results
type BackupResult struct {
	Path       string
	Size       int64
	FileCount  int
	Duration   time.Duration
	Metadata   *Metadata
}

// RestoreResult contains restore operation results
type RestoreResult struct {
	FileCount   int
	TotalSize   int64
	Duration    time.Duration
	Metadata    *Metadata
}

// Create creates a backup of all databases
func Create(opts BackupOptions) (*BackupResult, error) {
	start := time.Now()

	// Validate data directory
	if _, err := os.Stat(opts.DataDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("data directory does not exist: %s", opts.DataDir)
	}

	// Generate output path if not specified
	if opts.OutputPath == "" {
		timestamp := time.Now().Format("20060102-150405")
		if opts.Compress {
			opts.OutputPath = fmt.Sprintf("dogwatch-backup-%s.tar.gz", timestamp)
		} else {
			opts.OutputPath = fmt.Sprintf("dogwatch-backup-%s.tar", timestamp)
		}
	}

	// Collect files to backup
	var files []FileInfo
	var totalSize int64

	for _, dbFile := range DatabaseFiles {
		path := filepath.Join(opts.DataDir, dbFile)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue // Skip missing files
		}
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", dbFile, err)
		}

		files = append(files, FileInfo{
			Name:    dbFile,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
		totalSize += info.Size()
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no database files found in %s", opts.DataDir)
	}

	// Create metadata
	hostname, _ := os.Hostname()
	metadata := &Metadata{
		Version:   "1.0",
		CreatedAt: time.Now().UTC(),
		DataDir:   opts.DataDir,
		Files:     files,
		TotalSize: totalSize,
		Hostname:  hostname,
	}

	// Create output file
	outFile, err := os.Create(opts.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	var tw *tar.Writer

	if opts.Compress {
		gw := gzip.NewWriter(outFile)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(outFile)
	}
	defer tw.Close()

	// Write metadata first
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	err = tw.WriteHeader(&tar.Header{
		Name:    "metadata.json",
		Mode:    0644,
		Size:    int64(len(metadataJSON)),
		ModTime: time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("write metadata header: %w", err)
	}
	if _, err := tw.Write(metadataJSON); err != nil {
		return nil, fmt.Errorf("write metadata: %w", err)
	}

	// Write each database file
	for _, fileInfo := range files {
		srcPath := filepath.Join(opts.DataDir, fileInfo.Name)
		if err := addFileToTar(tw, srcPath, fileInfo.Name); err != nil {
			return nil, fmt.Errorf("add %s to archive: %w", fileInfo.Name, err)
		}
	}

	// Get final size
	outInfo, err := outFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat output file: %w", err)
	}

	return &BackupResult{
		Path:      opts.OutputPath,
		Size:      outInfo.Size(),
		FileCount: len(files),
		Duration:  time.Since(start),
		Metadata:  metadata,
	}, nil
}

// Restore restores databases from a backup
func Restore(opts RestoreOptions) (*RestoreResult, error) {
	start := time.Now()

	// Open backup file
	file, err := os.Open(opts.BackupPath)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()

	// Detect compression
	var tr *tar.Reader
	if strings.HasSuffix(opts.BackupPath, ".gz") || strings.HasSuffix(opts.BackupPath, ".tgz") {
		gr, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(file)
	}

	// Read metadata first
	header, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("read tar header: %w", err)
	}
	if header.Name != "metadata.json" {
		return nil, fmt.Errorf("invalid backup: expected metadata.json, got %s", header.Name)
	}

	metadataJSON := make([]byte, header.Size)
	if _, err := io.ReadFull(tr, metadataJSON); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(opts.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// Check for existing files if not forcing
	if !opts.Force {
		for _, fileInfo := range metadata.Files {
			destPath := filepath.Join(opts.DataDir, fileInfo.Name)
			if _, err := os.Stat(destPath); err == nil {
				return nil, fmt.Errorf("file exists: %s (use --force to overwrite)", destPath)
			}
		}
	}

	// Extract files
	var fileCount int
	var totalSize int64

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}

		// Skip metadata (already read)
		if header.Name == "metadata.json" {
			continue
		}

		// Validate filename (security: prevent path traversal)
		if strings.Contains(header.Name, "..") || filepath.IsAbs(header.Name) {
			return nil, fmt.Errorf("invalid file path in backup: %s", header.Name)
		}

		destPath := filepath.Join(opts.DataDir, header.Name)

		// Create destination file
		destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", header.Name, err)
		}

		n, err := io.Copy(destFile, tr)
		destFile.Close()
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", header.Name, err)
		}

		fileCount++
		totalSize += n
	}

	return &RestoreResult{
		FileCount: fileCount,
		TotalSize: totalSize,
		Duration:  time.Since(start),
		Metadata:  &metadata,
	}, nil
}

// List returns information about a backup without extracting
func List(backupPath string) (*Metadata, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()

	var tr *tar.Reader
	if strings.HasSuffix(backupPath, ".gz") || strings.HasSuffix(backupPath, ".tgz") {
		gr, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(file)
	}

	header, err := tr.Next()
	if err != nil {
		return nil, fmt.Errorf("read tar header: %w", err)
	}
	if header.Name != "metadata.json" {
		return nil, fmt.Errorf("invalid backup: expected metadata.json, got %s", header.Name)
	}

	metadataJSON := make([]byte, header.Size)
	if _, err := io.ReadFull(tr, metadataJSON); err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	return &metadata, nil
}

// ListBackups returns all backups in a directory
func ListBackups(dir string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "dogwatch-backup-") {
			continue
		}
		if !strings.HasSuffix(name, ".tar") && !strings.HasSuffix(name, ".tar.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backupPath := filepath.Join(dir, name)
		metadata, _ := List(backupPath) // Ignore errors for listing

		backups = append(backups, BackupInfo{
			Path:     backupPath,
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			Metadata: metadata,
		})
	}

	// Sort by modification time, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.After(backups[j].ModTime)
	})

	return backups, nil
}

// BackupInfo describes a backup file
type BackupInfo struct {
	Path     string    `json:"path"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Metadata *Metadata `json:"metadata,omitempty"`
}

// addFileToTar adds a file to a tar archive
func addFileToTar(tw *tar.Writer, srcPath, name string) error {
	file, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = name

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}

// FormatSize formats bytes as human readable
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Verify checks backup integrity without extracting
func Verify(backupPath string) (*VerifyResult, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return nil, fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()

	var tr *tar.Reader
	if strings.HasSuffix(backupPath, ".gz") || strings.HasSuffix(backupPath, ".tgz") {
		gr, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("invalid gzip: %w", err)
		}
		defer gr.Close()
		tr = tar.NewReader(gr)
	} else {
		tr = tar.NewReader(file)
	}

	result := &VerifyResult{
		Path:   backupPath,
		Valid:  true,
		Errors: []string{},
	}

	// Read and validate metadata
	header, err := tr.Next()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("read header: %v", err))
		return result, nil
	}
	if header.Name != "metadata.json" {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("expected metadata.json, got %s", header.Name))
		return result, nil
	}

	metadataJSON := make([]byte, header.Size)
	if _, err := io.ReadFull(tr, metadataJSON); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("read metadata: %v", err))
		return result, nil
	}

	var metadata Metadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("parse metadata: %v", err))
		return result, nil
	}

	result.Metadata = &metadata

	// Verify all files can be read
	expectedFiles := make(map[string]int64)
	for _, f := range metadata.Files {
		expectedFiles[f.Name] = f.Size
	}

	foundFiles := make(map[string]int64)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("read entry: %v", err))
			continue
		}

		// Skip metadata
		if header.Name == "metadata.json" {
			continue
		}

		// Verify we can read the entire file
		n, err := io.Copy(io.Discard, tr)
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("read %s: %v", header.Name, err))
			continue
		}

		foundFiles[header.Name] = n
		result.FileCount++
		result.TotalSize += n
	}

	// Check for missing files
	for name := range expectedFiles {
		if _, found := foundFiles[name]; !found {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("missing file: %s", name))
		}
	}

	return result, nil
}

// VerifyResult contains backup verification results
type VerifyResult struct {
	Path      string    `json:"path"`
	Valid     bool      `json:"valid"`
	FileCount int       `json:"file_count"`
	TotalSize int64     `json:"total_size"`
	Metadata  *Metadata `json:"metadata,omitempty"`
	Errors    []string  `json:"errors,omitempty"`
}

// RetentionPolicy defines backup retention rules
type RetentionPolicy struct {
	MaxBackups int           // Maximum number of backups to keep (0 = unlimited)
	MaxAge     time.Duration // Maximum age of backups (0 = unlimited)
	MinBackups int           // Always keep at least this many (default 1)
}

// DefaultRetentionPolicy returns sensible defaults
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxBackups: 10,
		MaxAge:     30 * 24 * time.Hour, // 30 days
		MinBackups: 1,
	}
}

// ApplyRetention deletes old backups according to policy
func ApplyRetention(dir string, policy RetentionPolicy) (*RetentionResult, error) {
	backups, err := ListBackups(dir)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	if policy.MinBackups < 1 {
		policy.MinBackups = 1
	}

	result := &RetentionResult{
		TotalBefore: len(backups),
	}

	// Determine which backups to delete
	var toDelete []BackupInfo
	cutoff := time.Now().Add(-policy.MaxAge)

	for i, b := range backups {
		// Always keep minimum number of backups
		if len(backups)-len(toDelete) <= policy.MinBackups {
			break
		}

		shouldDelete := false

		// Check max backups
		if policy.MaxBackups > 0 && i >= policy.MaxBackups {
			shouldDelete = true
		}

		// Check max age
		if policy.MaxAge > 0 && b.ModTime.Before(cutoff) {
			shouldDelete = true
		}

		if shouldDelete {
			toDelete = append(toDelete, b)
		}
	}

	// Delete selected backups
	for _, b := range toDelete {
		if err := os.Remove(b.Path); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", b.Path, err))
		} else {
			result.Deleted = append(result.Deleted, b.Path)
			result.BytesFreed += b.Size
		}
	}

	result.TotalAfter = result.TotalBefore - len(result.Deleted)
	return result, nil
}

// RetentionResult contains retention operation results
type RetentionResult struct {
	TotalBefore int      `json:"total_before"`
	TotalAfter  int      `json:"total_after"`
	Deleted     []string `json:"deleted"`
	BytesFreed  int64    `json:"bytes_freed"`
	Errors      []string `json:"errors,omitempty"`
}
