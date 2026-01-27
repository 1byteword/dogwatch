package main

import (
	"flag"
	"fmt"
	"os"

	"dogwatch/internal/backup"
)

func main() {
	// Subcommands
	createCmd := flag.NewFlagSet("create", flag.ExitOnError)
	restoreCmd := flag.NewFlagSet("restore", flag.ExitOnError)
	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	infoCmd := flag.NewFlagSet("info", flag.ExitOnError)

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
  dogwatch-backup info --backup backup.tar.gz`)
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
