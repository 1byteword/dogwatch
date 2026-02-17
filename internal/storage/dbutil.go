package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenDB opens a SQLite database with production-ready PRAGMAs:
// WAL mode, busy timeout, synchronous=NORMAL, and mmap.
// All stores should use this instead of sql.Open directly.
func OpenDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", dbPath, err)
	}

	if err := ConfigureDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("configuring database %s: %w", dbPath, err)
	}

	return db, nil
}

// ConfigureDB applies production PRAGMAs to an existing database connection.
func ConfigureDB(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",          // 5-10x write throughput; readers don't block writers
		"PRAGMA busy_timeout=5000",         // Retry on SQLITE_BUSY for up to 5s instead of failing immediately
		"PRAGMA synchronous=NORMAL",        // Safe with WAL; avoids full fsync on every write
		"PRAGMA cache_size=-64000",         // 64MB page cache (negative = KB)
		"PRAGMA mmap_size=268435456",       // 256MB memory-mapped I/O
		"PRAGMA foreign_keys=ON",           // Enforce FK constraints
		"PRAGMA auto_vacuum=INCREMENTAL",   // Allow database to shrink
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("executing %s: %w", p, err)
		}
	}

	// Limit to a single writer connection to avoid SQLITE_BUSY from concurrent writers.
	// WAL mode allows unlimited concurrent readers regardless of this setting.
	db.SetMaxOpenConns(4)

	return nil
}
