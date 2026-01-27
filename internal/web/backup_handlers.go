package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"dogwatch/internal/backup"
)

var backupDataDir string

// SetBackupDataDir sets the data directory for backup operations
func SetBackupDataDir(dataDir string) {
	backupDataDir = dataDir
}

// RegisterBackupRoutes registers backup/restore routes
func RegisterBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/backup", handleBackup)
	mux.HandleFunc("/api/backup/list", handleBackupList)
	mux.HandleFunc("/api/backup/download/", handleBackupDownload)
	mux.HandleFunc("/api/restore", handleRestore)
}

// handleBackup creates a new backup
func handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if backupDataDir == "" {
		http.Error(w, "Backup not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse options from request
	var opts struct {
		OutputDir string `json:"output_dir"`
		Compress  bool   `json:"compress"`
	}
	opts.Compress = true // Default to compressed

	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Default output directory to data directory
	outputDir := opts.OutputDir
	if outputDir == "" {
		outputDir = backupDataDir
	}

	// Generate output path
	timestamp := time.Now().Format("20060102-150405")
	var outputPath string
	if opts.Compress {
		outputPath = filepath.Join(outputDir, fmt.Sprintf("dogwatch-backup-%s.tar.gz", timestamp))
	} else {
		outputPath = filepath.Join(outputDir, fmt.Sprintf("dogwatch-backup-%s.tar", timestamp))
	}

	// Create backup
	result, err := backup.Create(backup.BackupOptions{
		DataDir:    backupDataDir,
		OutputPath: outputPath,
		Compress:   opts.Compress,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Backup failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"path":       result.Path,
		"size":       result.Size,
		"size_human": backup.FormatSize(result.Size),
		"file_count": result.FileCount,
		"duration":   result.Duration.String(),
		"metadata":   result.Metadata,
	})
}

// handleBackupList lists available backups
func handleBackupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if backupDataDir == "" {
		http.Error(w, "Backup not configured", http.StatusServiceUnavailable)
		return
	}

	// Check query param for directory
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = backupDataDir
	}

	backups, err := backup.ListBackups(dir)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list backups: %v", err), http.StatusInternalServerError)
		return
	}

	// Add human-readable sizes
	type backupResponse struct {
		backup.BackupInfo
		SizeHuman string `json:"size_human"`
	}

	var response []backupResponse
	for _, b := range backups {
		response = append(response, backupResponse{
			BackupInfo: b,
			SizeHuman:  backup.FormatSize(b.Size),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleBackupDownload downloads a backup file
func handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract filename from path
	filename := filepath.Base(r.URL.Path)
	if filename == "" || filename == "download" {
		http.Error(w, "Filename required", http.StatusBadRequest)
		return
	}

	// Security: only allow backup files
	if !isValidBackupFilename(filename) {
		http.Error(w, "Invalid backup filename", http.StatusBadRequest)
		return
	}

	backupPath := filepath.Join(backupDataDir, filename)

	// Check file exists
	info, err := os.Stat(backupPath)
	if os.IsNotExist(err) {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
		return
	}

	// Serve file
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	http.ServeFile(w, r, backupPath)
}

// handleRestore restores from a backup
func handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if backupDataDir == "" {
		http.Error(w, "Backup not configured", http.StatusServiceUnavailable)
		return
	}

	var opts struct {
		BackupPath string `json:"backup_path"`
		Force      bool   `json:"force"`
	}

	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if opts.BackupPath == "" {
		http.Error(w, "backup_path is required", http.StatusBadRequest)
		return
	}

	// Security: validate path
	if !isValidBackupPath(opts.BackupPath) {
		http.Error(w, "Invalid backup path", http.StatusBadRequest)
		return
	}

	// Restore
	result, err := backup.Restore(backup.RestoreOptions{
		BackupPath: opts.BackupPath,
		DataDir:    backupDataDir,
		Force:      opts.Force,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Restore failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"file_count":   result.FileCount,
		"total_size":   result.TotalSize,
		"size_human":   backup.FormatSize(result.TotalSize),
		"duration":     result.Duration.String(),
		"metadata":     result.Metadata,
		"note":         "Restart dogwatch to use restored data",
	})
}

// isValidBackupFilename checks if filename is a valid backup file
func isValidBackupFilename(filename string) bool {
	if len(filename) < 20 || len(filename) > 50 {
		return false
	}
	if !hasPrefix(filename, "dogwatch-backup-") {
		return false
	}
	if !hasSuffix(filename, ".tar") && !hasSuffix(filename, ".tar.gz") {
		return false
	}
	// Check for path traversal
	if containsAny(filename, "/\\..") {
		return false
	}
	return true
}

// isValidBackupPath checks if path is safe for restore
func isValidBackupPath(path string) bool {
	// Must be absolute or relative without traversal
	if containsAny(path, "..") {
		return false
	}
	// Must end with valid extension
	if !hasSuffix(path, ".tar") && !hasSuffix(path, ".tar.gz") {
		return false
	}
	return true
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func containsAny(s, chars string) bool {
	for _, c := range chars {
		for _, sc := range s {
			if c == sc {
				return true
			}
		}
	}
	return false
}
