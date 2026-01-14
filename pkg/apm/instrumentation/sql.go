// Package instrumentation provides APM wrappers for common libraries
package instrumentation

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"dogwatch/pkg/apm"
)

// TracedDB wraps a sql.DB with APM instrumentation
type TracedDB struct {
	*sql.DB
	dbType   string
	dbName   string
	connInfo string
}

// WrapDB wraps a sql.DB for tracing
func WrapDB(db *sql.DB, dbType, dbName string) *TracedDB {
	return &TracedDB{
		DB:     db,
		dbType: dbType,
		dbName: dbName,
	}
}

// OpenDB opens a traced database connection
func OpenDB(driverName, dsn string) (*TracedDB, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	// Parse database name from DSN (simplified)
	dbName := extractDBName(dsn)

	return &TracedDB{
		DB:       db,
		dbType:   driverName,
		dbName:   dbName,
		connInfo: maskDSN(dsn),
	}, nil
}

// Query executes a query with tracing
func (db *TracedDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	span, ctx := db.startSpan(ctx, "db.query", query)
	defer span.Finish()

	rows, err := db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		span.SetError(err)
	}
	return rows, err
}

// QueryRow executes a query that returns a single row with tracing
func (db *TracedDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	span, ctx := db.startSpan(ctx, "db.query", query)
	defer span.Finish()

	return db.DB.QueryRowContext(ctx, query, args...)
}

// Exec executes a query with tracing
func (db *TracedDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	span, ctx := db.startSpan(ctx, "db.exec", query)
	defer span.Finish()

	result, err := db.DB.ExecContext(ctx, query, args...)
	if err != nil {
		span.SetError(err)
	} else {
		if rowsAffected, err := result.RowsAffected(); err == nil {
			span.SetMetric("db.rows_affected", float64(rowsAffected))
		}
	}
	return result, err
}

// Prepare creates a prepared statement with tracing
func (db *TracedDB) PrepareContext(ctx context.Context, query string) (*TracedStmt, error) {
	span, ctx := db.startSpan(ctx, "db.prepare", query)
	defer span.Finish()

	stmt, err := db.DB.PrepareContext(ctx, query)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return &TracedStmt{Stmt: stmt, query: query, db: db}, nil
}

// Begin starts a transaction with tracing
func (db *TracedDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*TracedTx, error) {
	span, ctx := db.startSpan(ctx, "db.begin", "BEGIN")
	defer span.Finish()

	tx, err := db.DB.BeginTx(ctx, opts)
	if err != nil {
		span.SetError(err)
		return nil, err
	}
	return &TracedTx{Tx: tx, db: db, ctx: ctx}, nil
}

func (db *TracedDB) startSpan(ctx context.Context, opName, query string) (*apm.Span, context.Context) {
	resource := normalizeQuery(query)
	span, ctx := apm.StartSpanFromContext(ctx, opName,
		apm.WithSpanType(apm.SpanTypeSQL),
		apm.WithResource(resource),
		apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
	)

	span.SetTag(apm.TagDBType, db.dbType)
	span.SetTag(apm.TagDBInstance, db.dbName)
	span.SetTag(apm.TagDBStatement, truncateQuery(query, 1000))
	span.SetTag(apm.TagComponent, "database/sql")

	return span, ctx
}

// TracedStmt wraps a sql.Stmt with tracing
type TracedStmt struct {
	*sql.Stmt
	query string
	db    *TracedDB
}

// QueryContext executes the prepared query with tracing
func (s *TracedStmt) QueryContext(ctx context.Context, args ...interface{}) (*sql.Rows, error) {
	span, ctx := s.db.startSpan(ctx, "db.query", s.query)
	defer span.Finish()

	rows, err := s.Stmt.QueryContext(ctx, args...)
	if err != nil {
		span.SetError(err)
	}
	return rows, err
}

// ExecContext executes the prepared statement with tracing
func (s *TracedStmt) ExecContext(ctx context.Context, args ...interface{}) (sql.Result, error) {
	span, ctx := s.db.startSpan(ctx, "db.exec", s.query)
	defer span.Finish()

	result, err := s.Stmt.ExecContext(ctx, args...)
	if err != nil {
		span.SetError(err)
	}
	return result, err
}

// TracedTx wraps a sql.Tx with tracing
type TracedTx struct {
	*sql.Tx
	db  *TracedDB
	ctx context.Context
}

// QueryContext executes a query within the transaction with tracing
func (tx *TracedTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	span, ctx := tx.db.startSpan(ctx, "db.query", query)
	defer span.Finish()

	rows, err := tx.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		span.SetError(err)
	}
	return rows, err
}

// ExecContext executes a query within the transaction with tracing
func (tx *TracedTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	span, ctx := tx.db.startSpan(ctx, "db.exec", query)
	defer span.Finish()

	result, err := tx.Tx.ExecContext(ctx, query, args...)
	if err != nil {
		span.SetError(err)
	}
	return result, err
}

// Commit commits the transaction with tracing
func (tx *TracedTx) Commit() error {
	span, _ := tx.db.startSpan(tx.ctx, "db.commit", "COMMIT")
	defer span.Finish()

	err := tx.Tx.Commit()
	if err != nil {
		span.SetError(err)
	}
	return err
}

// Rollback rolls back the transaction with tracing
func (tx *TracedTx) Rollback() error {
	span, _ := tx.db.startSpan(tx.ctx, "db.rollback", "ROLLBACK")
	defer span.Finish()

	err := tx.Tx.Rollback()
	if err != nil && err != sql.ErrTxDone {
		span.SetError(err)
	}
	return err
}

// Helper functions

// normalizeQuery creates a normalized query for resource naming
func normalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "unknown"
	}

	// Get operation type (SELECT, INSERT, UPDATE, DELETE, etc.)
	upper := strings.ToUpper(query)
	var op string
	switch {
	case strings.HasPrefix(upper, "SELECT"):
		op = "SELECT"
	case strings.HasPrefix(upper, "INSERT"):
		op = "INSERT"
	case strings.HasPrefix(upper, "UPDATE"):
		op = "UPDATE"
	case strings.HasPrefix(upper, "DELETE"):
		op = "DELETE"
	case strings.HasPrefix(upper, "BEGIN"):
		op = "BEGIN"
	case strings.HasPrefix(upper, "COMMIT"):
		op = "COMMIT"
	case strings.HasPrefix(upper, "ROLLBACK"):
		op = "ROLLBACK"
	case strings.HasPrefix(upper, "CREATE"):
		op = "CREATE"
	case strings.HasPrefix(upper, "ALTER"):
		op = "ALTER"
	case strings.HasPrefix(upper, "DROP"):
		op = "DROP"
	default:
		op = "QUERY"
	}

	// Try to extract table name
	table := extractTableName(query)
	if table != "" {
		return fmt.Sprintf("%s %s", op, table)
	}
	return op
}

// extractTableName attempts to extract the table name from a query
func extractTableName(query string) string {
	query = strings.ToUpper(query)

	// Common patterns
	patterns := []string{
		"FROM ", "INTO ", "UPDATE ", "TABLE ", "JOIN ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(query, pattern); idx != -1 {
			rest := query[idx+len(pattern):]
			// Get the next word
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				table := strings.Trim(fields[0], "`\"[](),")
				// Remove schema prefix if present
				if parts := strings.Split(table, "."); len(parts) > 1 {
					return parts[len(parts)-1]
				}
				return table
			}
		}
	}
	return ""
}

// truncateQuery truncates a query to a maximum length
func truncateQuery(query string, maxLen int) string {
	if len(query) <= maxLen {
		return query
	}
	return query[:maxLen-3] + "..."
}

// extractDBName extracts database name from DSN
func extractDBName(dsn string) string {
	// Handle common DSN formats
	// postgres: postgres://user:pass@host/dbname
	// mysql: user:pass@tcp(host)/dbname
	// sqlite: file.db

	if strings.Contains(dsn, "://") {
		parts := strings.Split(dsn, "/")
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			// Remove query string
			if idx := strings.Index(last, "?"); idx != -1 {
				return last[:idx]
			}
			return last
		}
	}

	// For MySQL-style DSN
	if idx := strings.LastIndex(dsn, "/"); idx != -1 {
		dbName := dsn[idx+1:]
		if idx := strings.Index(dbName, "?"); idx != -1 {
			return dbName[:idx]
		}
		return dbName
	}

	return "unknown"
}

// maskDSN removes sensitive info from DSN
func maskDSN(dsn string) string {
	// Replace password with ****
	if idx := strings.Index(dsn, "://"); idx != -1 {
		// URL-style DSN
		if atIdx := strings.Index(dsn[idx+3:], "@"); atIdx != -1 {
			prefix := dsn[:idx+3]
			rest := dsn[idx+3:]
			// Find : after user
			if colonIdx := strings.Index(rest[:atIdx], ":"); colonIdx != -1 {
				return prefix + rest[:colonIdx+1] + "****" + rest[atIdx:]
			}
		}
	}
	return dsn
}

// TracedDriver wraps a sql driver with tracing (for custom integrations)
type TracedDriver struct {
	driver.Driver
	dbType string
}

// WrapDriver wraps a database driver for tracing
func WrapDriver(d driver.Driver, dbType string) *TracedDriver {
	return &TracedDriver{Driver: d, dbType: dbType}
}

// RecordDBMetric records a database metric
func RecordDBMetric(operation, dbType, table string, duration time.Duration, err error) {
	tags := map[string]string{
		"operation": operation,
		"db_type":   dbType,
		"table":     table,
	}
	if err != nil {
		tags["error"] = "true"
		apm.RecordCounter("db.errors", 1, tags)
	}
	apm.RecordHistogram("db.query.duration_ms", float64(duration.Milliseconds()), tags)
	apm.RecordCounter("db.query.count", 1, tags)
}
