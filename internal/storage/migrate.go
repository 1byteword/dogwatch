package storage

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
)

// Migration represents a single schema migration step.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// MigrationRunner applies schema migrations to a SQLite database using a
// version table. Migrations are applied in order and are idempotent (already-applied
// versions are skipped).
type MigrationRunner struct {
	db         *sql.DB
	table      string
	migrations []Migration
}

// NewMigrationRunner creates a runner that tracks applied versions in the given table.
// The table defaults to "schema_migrations" if empty.
func NewMigrationRunner(db *sql.DB, table string) *MigrationRunner {
	if table == "" {
		table = "schema_migrations"
	}
	return &MigrationRunner{
		db:    db,
		table: table,
	}
}

// Add registers a migration. Migrations must have unique, monotonically increasing versions.
func (r *MigrationRunner) Add(version int, description, sql string) {
	r.migrations = append(r.migrations, Migration{
		Version:     version,
		Description: description,
		SQL:         sql,
	})
}

// Run applies all pending migrations in a transaction per migration.
func (r *MigrationRunner) Run() error {
	// Ensure version tracking table exists
	_, err := r.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`, r.table))
	if err != nil {
		return fmt.Errorf("creating migration table: %w", err)
	}

	// Get already applied versions
	applied := make(map[int]bool)
	rows, err := r.db.Query(fmt.Sprintf("SELECT version FROM %s", r.table))
	if err != nil {
		return fmt.Errorf("reading applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scanning migration version: %w", err)
		}
		applied[v] = true
	}

	// Sort migrations by version
	sort.Slice(r.migrations, func(i, j int) bool {
		return r.migrations[i].Version < r.migrations[j].Version
	})

	// Apply pending migrations
	for _, m := range r.migrations {
		if applied[m.Version] {
			continue
		}

		tx, err := r.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning migration %d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("applying migration %d (%s): %w", m.Version, m.Description, err)
		}

		if _, err := tx.Exec(
			fmt.Sprintf("INSERT INTO %s (version, description) VALUES (?, ?)", r.table),
			m.Version, m.Description,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.Version, err)
		}

		log.Printf("[migrate] Applied migration %d: %s", m.Version, m.Description)
	}

	return nil
}

// CurrentVersion returns the highest applied migration version, or 0 if none.
func (r *MigrationRunner) CurrentVersion() (int, error) {
	var version int
	err := r.db.QueryRow(
		fmt.Sprintf("SELECT COALESCE(MAX(version), 0) FROM %s", r.table),
	).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}
