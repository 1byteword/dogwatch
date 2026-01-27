package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"dogwatch/internal/backup"
)

func main() {
	// Subcommands
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	restoreCmd := flag.NewFlagSet("restore", flag.ExitOnError)
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	infoCmd := flag.NewFlagSet("info", flag.ExitOnError)
	verifyCmd := flag.NewFlagSet("verify", flag.ExitOnError)
	cleanupCmd := flag.NewFlagSet("cleanup", flag.ExitOnError)

	// Create flags
	createDataDir := createCmd.String("data", "/var/lib/dogwatch", "Data directory to backup")
	createOutput := createCmd.String("output", "", "Output file path (default: auto-generated)")
	createNoCompress := createCmd.Bool("no-compress", false, "Disable gzip compression")

	// Restore flags
	restoreBackup := restoreCmd.String("backup", "", "Backup file to restore")
	restoreDataDir := restoreCmd.String("data", "/var/lib/dogwatch", "Data directory to restore to")
	restoreForce := restoreCmd.Bool("force", false, "Overwrite existing files")

	// List flags
	listDir := listCmd.String("dir", "/var/lib/dogwatch", "Directory to list backups from")

	// Info flags
	infoBackup := infoCmd.String("backup", "", "Backup file to inspect")

	// Verify flags
	verifyBackup := verifyCmd.String("backup", "", "Backup file to verify")

	// Cleanup flags
	cleanupDir := cleanupCmd.String("dir", "/var/lib/dogwatch", "Directory to clean up")
	cleanupMaxBackups := cleanupCmd.Int("max-backups", 10, "Maximum number of backups to keep")
	cleanupMaxDays := cleanupCmd.Int("max-days", 30, "Maximum age in days")
	cleanupMinBackups := cleanupCmd.Int("min-backups", 1, "Minimum backups to always keep")
	cleanupDryRun := cleanupCmd.Bool("dry-run", false, "Show what would be deleted without deleting")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "create":
		createCmd.Parse(os.Args[2:])
		doCreate(*createDataDir, *createOutput, !*createNoCompress)

	case "restore":
		restoreCmd.Parse(os.Args[2:])
		if *restoreBackup == "" {
			fmt.Fprintln(os.Stderr, "Error: --backup is required")
			restoreCmd.Usage()
			os.Exit(1)
		}
		doRestore(*restoreBackup, *restoreDataDir, *restoreForce)

	case "list":
		listCmd.Parse(os.Args[2:])
		doList(*listDir)

	case "info":
		infoCmd.Parse(os.Args[2:])
		if *infoBackup == "" {
			fmt.Fprintln(os.Stderr, "Error: --backup is required")
			infoCmd.Usage()
			os.Exit(1)
		}
		doInfo(*infoBackup)

	case "verify":
		verifyCmd.Parse(os.Args[2:])
		if *verifyBackup == "" {
			fmt.Fprintln(os.Stderr, "Error: --backup is required")
			verifyCmd.Usage()
			os.Exit(1)
		}
		doVerify(*verifyBackup)

	case "cleanup":
		cleanupCmd.Parse(os.Args[2:])
		doCleanup(*cleanupDir, *cleanupMaxBackups, *cleanupMaxDays, *cleanupMinBackups, *cleanupDryRun)

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`dogwatch-backup - Backup and restore dogwatch data

Usage:
  dogwatch-backup <command> [options]

Commands:
  create   Create a new backup
  restore  Restore from a backup
  list     List available backups
  info     Show backup information
  verify   Verify backup integrity
  cleanup  Apply retention policy to delete old backups

Examples:
  # Create a backup
  dogwatch-backup create --data /var/lib/dogwatch

  # Create backup with custom output path
  dogwatch-backup create --data /var/lib/dogwatch --output /backups/dogwatch.tar.gz

  # Restore from backup
  dogwatch-backup restore --backup /backups/dogwatch.tar.gz --data /var/lib/dogwatch

  # Force restore (overwrite existing)
  dogwatch-backup restore --backup backup.tar.gz --force

  # List backups in directory
  dogwatch-backup list --dir /var/lib/dogwatch

  # Show backup info
  dogwatch-backup info --backup backup.tar.gz

  # Verify backup integrity
  dogwatch-backup verify --backup backup.tar.gz

  # Clean up old backups (keep last 10, max 30 days)
  dogwatch-backup cleanup --dir /var/lib/dogwatch --max-backups 10 --max-days 30

  # Dry run cleanup (show what would be deleted)
  dogwatch-backup cleanup --dir /var/lib/dogwatch --dry-run`)
}

func doCreate(dataDir, output string, compress bool) {
	fmt.Printf("Creating backup from %s...\n", dataDir)

	result, err := backup.Create(backup.BackupOptions{
		DataDir:    dataDir,
		OutputPath: output,
		Compress:   compress,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("Backup created successfully!\n")
	fmt.Printf("  Path:       %s\n", result.Path)
	fmt.Printf("  Size:       %s\n", backup.FormatSize(result.Size))
	fmt.Printf("  Files:      %d databases\n", result.FileCount)
	fmt.Printf("  Duration:   %s\n", result.Duration)
	fmt.Printf("  Created:    %s\n", result.Metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
}

func doRestore(backupPath, dataDir string, force bool) {
	// Show backup info first
	metadata, err := backup.List(backupPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading backup: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Restoring from: %s\n", backupPath)
	fmt.Printf("  Created:    %s\n", metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Files:      %d databases\n", len(metadata.Files))
	fmt.Printf("  Total size: %s\n", backup.FormatSize(metadata.TotalSize))
	fmt.Printf("  Target:     %s\n", dataDir)
	fmt.Println()

	if !force {
		fmt.Println("WARNING: This will restore database files to the target directory.")
		fmt.Println("         Stop dogwatch before restoring to avoid corruption.")
		fmt.Println("         Use --force to overwrite existing files.")
		fmt.Println()
	}

	result, err := backup.Restore(backup.RestoreOptions{
		BackupPath: backupPath,
		DataDir:    dataDir,
		Force:      force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Restore completed successfully!\n")
	fmt.Printf("  Files:      %d restored\n", result.FileCount)
	fmt.Printf("  Size:       %s\n", backup.FormatSize(result.TotalSize))
	fmt.Printf("  Duration:   %s\n", result.Duration)
	fmt.Println()
	fmt.Println("Restart dogwatch to use the restored data.")
}

func doList(dir string) {
	backups, err := backup.ListBackups(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(backups) == 0 {
		fmt.Printf("No backups found in %s\n", dir)
		return
	}

	fmt.Printf("Backups in %s:\n\n", dir)
	fmt.Printf("%-45s  %10s  %s\n", "FILENAME", "SIZE", "CREATED")
	fmt.Println(repeatString("-", 80))

	for _, b := range backups {
		created := "unknown"
		if b.Metadata != nil {
			created = b.Metadata.CreatedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-45s  %10s  %s\n",
			truncateString(b.Path, 45),
			backup.FormatSize(b.Size),
			created,
		)
	}
}

func doInfo(backupPath string) {
	metadata, err := backup.List(backupPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Backup: %s\n", backupPath)
	fmt.Println()
	fmt.Printf("Version:    %s\n", metadata.Version)
	fmt.Printf("Created:    %s\n", metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("Hostname:   %s\n", metadata.Hostname)
	fmt.Printf("Data dir:   %s\n", metadata.DataDir)
	fmt.Printf("Total size: %s\n", backup.FormatSize(metadata.TotalSize))
	fmt.Println()
	fmt.Println("Files:")
	fmt.Printf("  %-30s  %10s  %s\n", "NAME", "SIZE", "MODIFIED")
	fmt.Println("  " + repeatString("-", 60))
	for _, f := range metadata.Files {
		fmt.Printf("  %-30s  %10s  %s\n",
			f.Name,
			backup.FormatSize(f.Size),
			f.ModTime.Format("2006-01-02 15:04:05"),
		)
	}
}

func repeatString(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "..." + s[len(s)-max+3:]
}

func doVerify(backupPath string) {
	fmt.Printf("Verifying backup: %s\n", backupPath)

	result, err := backup.Verify(backupPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	if result.Valid {
		fmt.Println("✓ Backup is valid")
	} else {
		fmt.Println("✗ Backup is INVALID")
	}

	fmt.Printf("  Files:      %d\n", result.FileCount)
	fmt.Printf("  Total size: %s\n", backup.FormatSize(result.TotalSize))

	if result.Metadata != nil {
		fmt.Printf("  Created:    %s\n", result.Metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC"))
		fmt.Printf("  Hostname:   %s\n", result.Metadata.Hostname)
	}

	if len(result.Errors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
		os.Exit(1)
	}
}

func doCleanup(dir string, maxBackups, maxDays, minBackups int, dryRun bool) {
	fmt.Printf("Cleanup backups in: %s\n", dir)
	fmt.Printf("  Max backups: %d\n", maxBackups)
	fmt.Printf("  Max age:     %d days\n", maxDays)
	fmt.Printf("  Min keep:    %d\n", minBackups)

	if dryRun {
		fmt.Println("  Mode:        DRY RUN (no files will be deleted)")
	}
	fmt.Println()

	// List current backups
	backups, err := backup.ListBackups(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing backups: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d backups\n", len(backups))

	if len(backups) == 0 {
		fmt.Println("Nothing to clean up")
		return
	}

	policy := backup.RetentionPolicy{
		MaxBackups: maxBackups,
		MaxAge:     time.Duration(maxDays) * 24 * time.Hour,
		MinBackups: minBackups,
	}

	if dryRun {
		// Show what would be deleted
		cutoff := time.Now().Add(-policy.MaxAge)
		var toDelete []backup.BackupInfo

		for i, b := range backups {
			if len(backups)-len(toDelete) <= policy.MinBackups {
				break
			}

			shouldDelete := false
			if policy.MaxBackups > 0 && i >= policy.MaxBackups {
				shouldDelete = true
			}
			if policy.MaxAge > 0 && b.ModTime.Before(cutoff) {
				shouldDelete = true
			}

			if shouldDelete {
				toDelete = append(toDelete, b)
			}
		}

		if len(toDelete) == 0 {
			fmt.Println("\nNo backups would be deleted")
			return
		}

		fmt.Printf("\nWould delete %d backups:\n", len(toDelete))
		var totalSize int64
		for _, b := range toDelete {
			fmt.Printf("  - %s (%s)\n", b.Path, backup.FormatSize(b.Size))
			totalSize += b.Size
		}
		fmt.Printf("\nTotal space to be freed: %s\n", backup.FormatSize(totalSize))
		return
	}

	// Actually apply retention
	result, err := backup.ApplyRetention(dir, policy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	if len(result.Deleted) == 0 {
		fmt.Println("No backups deleted")
	} else {
		fmt.Printf("Deleted %d backups:\n", len(result.Deleted))
		for _, path := range result.Deleted {
			fmt.Printf("  - %s\n", path)
		}
		fmt.Printf("\nSpace freed: %s\n", backup.FormatSize(result.BytesFreed))
	}

	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	fmt.Printf("\nBackups remaining: %d\n", result.TotalAfter)
}
