package dashboard

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// WidgetPosition represents a widget's position and size in the grid
type WidgetPosition struct {
	ID     string `json:"id"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
}

// Dashboard represents a saved dashboard layout
type Dashboard struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Layout    []WidgetPosition `json:"layout"`
	IsDefault bool             `json:"is_default"`
	Created   time.Time        `json:"created"`
	Updated   time.Time        `json:"updated"`
}

// Store handles dashboard persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new dashboard store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS dashboards (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		layout TEXT NOT NULL,
		is_default INTEGER DEFAULT 0,
		created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_dashboards_default ON dashboards(is_default);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Create creates a new dashboard
func (s *Store) Create(name string, layout []WidgetPosition, isDefault bool) (*Dashboard, error) {
	id := uuid.New().String()
	now := time.Now()

	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return nil, fmt.Errorf("marshal layout: %w", err)
	}

	// If setting as default, clear other defaults first
	if isDefault {
		if _, err := s.db.Exec("UPDATE dashboards SET is_default = 0"); err != nil {
			return nil, fmt.Errorf("clear defaults: %w", err)
		}
	}

	_, err = s.db.Exec(
		"INSERT INTO dashboards (id, name, layout, is_default, created, updated) VALUES (?, ?, ?, ?, ?, ?)",
		id, name, string(layoutJSON), isDefault, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert dashboard: %w", err)
	}

	return &Dashboard{
		ID:        id,
		Name:      name,
		Layout:    layout,
		IsDefault: isDefault,
		Created:   now,
		Updated:   now,
	}, nil
}

// Get retrieves a dashboard by ID
func (s *Store) Get(id string) (*Dashboard, error) {
	row := s.db.QueryRow(
		"SELECT id, name, layout, is_default, created, updated FROM dashboards WHERE id = ?",
		id,
	)

	var d Dashboard
	var layoutJSON string
	var isDefault int

	err := row.Scan(&d.ID, &d.Name, &layoutJSON, &isDefault, &d.Created, &d.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan dashboard: %w", err)
	}

	d.IsDefault = isDefault == 1

	if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
		return nil, fmt.Errorf("unmarshal layout: %w", err)
	}

	return &d, nil
}

// GetDefault retrieves the default dashboard
func (s *Store) GetDefault() (*Dashboard, error) {
	row := s.db.QueryRow(
		"SELECT id, name, layout, is_default, created, updated FROM dashboards WHERE is_default = 1",
	)

	var d Dashboard
	var layoutJSON string
	var isDefault int

	err := row.Scan(&d.ID, &d.Name, &layoutJSON, &isDefault, &d.Created, &d.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan dashboard: %w", err)
	}

	d.IsDefault = true

	if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
		return nil, fmt.Errorf("unmarshal layout: %w", err)
	}

	return &d, nil
}

// List returns all dashboards
func (s *Store) List() ([]Dashboard, error) {
	rows, err := s.db.Query(
		"SELECT id, name, layout, is_default, created, updated FROM dashboards ORDER BY name",
	)
	if err != nil {
		return nil, fmt.Errorf("query dashboards: %w", err)
	}
	defer rows.Close()

	var dashboards []Dashboard
	for rows.Next() {
		var d Dashboard
		var layoutJSON string
		var isDefault int

		if err := rows.Scan(&d.ID, &d.Name, &layoutJSON, &isDefault, &d.Created, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan dashboard: %w", err)
		}

		d.IsDefault = isDefault == 1

		if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
			return nil, fmt.Errorf("unmarshal layout: %w", err)
		}

		dashboards = append(dashboards, d)
	}

	return dashboards, nil
}

// Update updates a dashboard's name and layout
func (s *Store) Update(id string, name string, layout []WidgetPosition) error {
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("marshal layout: %w", err)
	}

	result, err := s.db.Exec(
		"UPDATE dashboards SET name = ?, layout = ?, updated = ? WHERE id = ?",
		name, string(layoutJSON), time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update dashboard: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("dashboard not found")
	}

	return nil
}

// SetDefault sets a dashboard as the default
func (s *Store) SetDefault(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear all defaults
	if _, err := tx.Exec("UPDATE dashboards SET is_default = 0"); err != nil {
		return fmt.Errorf("clear defaults: %w", err)
	}

	// Set new default
	result, err := tx.Exec("UPDATE dashboards SET is_default = 1 WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("set default: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("dashboard not found")
	}

	return tx.Commit()
}

// Delete removes a dashboard
func (s *Store) Delete(id string) error {
	result, err := s.db.Exec("DELETE FROM dashboards WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete dashboard: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("dashboard not found")
	}

	return nil
}
