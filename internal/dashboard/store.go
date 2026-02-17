package dashboard

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WidgetPosition represents a widget's position and size in the grid
type WidgetPosition struct {
	ID     string `json:"id"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
}

// Folder represents a dashboard folder for organization
type Folder struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	ParentID *string   `json:"parent_id,omitempty"`
	Position int       `json:"position"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

// Dashboard represents a saved dashboard layout
type Dashboard struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Layout       []WidgetPosition        `json:"layout"`
	WidgetConfig map[string]WidgetConfig `json:"widget_config,omitempty"`
	IsDefault    bool                    `json:"is_default"`
	FolderID     *string                 `json:"folder_id,omitempty"`
	Position     int                     `json:"position"`
	Created      time.Time               `json:"created"`
	Updated      time.Time               `json:"updated"`
}

// WidgetConfig stores optional per-widget dashboard settings.
type WidgetConfig struct {
	Service  string `json:"service,omitempty"`
	Since    string `json:"since,omitempty"`
	Severity string `json:"severity,omitempty"`
	Locked   bool   `json:"locked,omitempty"`
}

// FolderTree represents a folder with its children and dashboards
type FolderTree struct {
	Folder
	Children   []FolderTree `json:"children,omitempty"`
	Dashboards []Dashboard  `json:"dashboards,omitempty"`
}

// Store handles dashboard persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new dashboard store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create tables - base schema without folder columns for backwards compatibility
	schema := `
	CREATE TABLE IF NOT EXISTS dashboard_folders (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		parent_id TEXT REFERENCES dashboard_folders(id),
		position INTEGER DEFAULT 0,
		created TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_folders_parent ON dashboard_folders(parent_id);

	CREATE TABLE IF NOT EXISTS dashboards (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		layout TEXT NOT NULL,
		widget_config TEXT NOT NULL DEFAULT '{}',
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

	// Add folder columns if they don't exist (migration for existing DBs)
	migrations := []string{
		"ALTER TABLE dashboards ADD COLUMN folder_id TEXT REFERENCES dashboard_folders(id)",
		"ALTER TABLE dashboards ADD COLUMN position INTEGER DEFAULT 0",
		"ALTER TABLE dashboards ADD COLUMN widget_config TEXT NOT NULL DEFAULT '{}'",
	}
	for _, m := range migrations {
		db.Exec(m) // Ignore errors - column may already exist
	}

	// Create index on folder_id (after migration ensures column exists)
	db.Exec("CREATE INDEX IF NOT EXISTS idx_dashboards_folder ON dashboards(folder_id)")

	return &Store{db: db}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// ==================== Folder Methods ====================

// CreateFolder creates a new folder
func (s *Store) CreateFolder(name string, parentID *string) (*Folder, error) {
	id := uuid.New().String()
	now := time.Now()

	// Get next position in parent
	var position int
	if parentID != nil {
		row := s.db.QueryRow("SELECT COALESCE(MAX(position), -1) + 1 FROM dashboard_folders WHERE parent_id = ?", *parentID)
		row.Scan(&position)
	} else {
		row := s.db.QueryRow("SELECT COALESCE(MAX(position), -1) + 1 FROM dashboard_folders WHERE parent_id IS NULL")
		row.Scan(&position)
	}

	_, err := s.db.Exec(
		"INSERT INTO dashboard_folders (id, name, parent_id, position, created, updated) VALUES (?, ?, ?, ?, ?, ?)",
		id, name, parentID, position, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert folder: %w", err)
	}

	return &Folder{
		ID:       id,
		Name:     name,
		ParentID: parentID,
		Position: position,
		Created:  now,
		Updated:  now,
	}, nil
}

// GetFolder retrieves a folder by ID
func (s *Store) GetFolder(id string) (*Folder, error) {
	row := s.db.QueryRow(
		"SELECT id, name, parent_id, position, created, updated FROM dashboard_folders WHERE id = ?",
		id,
	)

	var f Folder
	var parentID sql.NullString

	err := row.Scan(&f.ID, &f.Name, &parentID, &f.Position, &f.Created, &f.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan folder: %w", err)
	}

	if parentID.Valid {
		f.ParentID = &parentID.String
	}

	return &f, nil
}

// ListFolders returns all folders
func (s *Store) ListFolders() ([]Folder, error) {
	rows, err := s.db.Query(
		"SELECT id, name, parent_id, position, created, updated FROM dashboard_folders ORDER BY position, name",
	)
	if err != nil {
		return nil, fmt.Errorf("query folders: %w", err)
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var f Folder
		var parentID sql.NullString

		if err := rows.Scan(&f.ID, &f.Name, &parentID, &f.Position, &f.Created, &f.Updated); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}

		if parentID.Valid {
			f.ParentID = &parentID.String
		}

		folders = append(folders, f)
	}

	return folders, nil
}

// GetFolderTree returns the full folder tree with dashboards
func (s *Store) GetFolderTree() ([]FolderTree, error) {
	folders, err := s.ListFolders()
	if err != nil {
		return nil, err
	}

	dashboards, err := s.List()
	if err != nil {
		return nil, err
	}

	// Build folder map
	folderMap := make(map[string]*FolderTree)
	for _, f := range folders {
		ft := FolderTree{Folder: f}
		folderMap[f.ID] = &ft
	}

	// Assign dashboards to folders
	var rootDashboards []Dashboard
	for _, d := range dashboards {
		if d.FolderID != nil {
			if ft, ok := folderMap[*d.FolderID]; ok {
				ft.Dashboards = append(ft.Dashboards, d)
			}
		} else {
			rootDashboards = append(rootDashboards, d)
		}
	}

	// Build tree: first attach children to parents, then collect roots.
	// Two passes needed to avoid copying parent before children are attached.
	for _, f := range folders {
		if f.ParentID != nil {
			if parent, ok := folderMap[*f.ParentID]; ok {
				parent.Children = append(parent.Children, *folderMap[f.ID])
			}
		}
	}

	var rootFolders []FolderTree
	for _, f := range folders {
		if f.ParentID == nil {
			rootFolders = append(rootFolders, *folderMap[f.ID])
		}
	}

	return rootFolders, nil
}

// GetFolderTreeWithRootDashboards returns tree plus root dashboards
func (s *Store) GetFolderTreeWithRootDashboards() ([]FolderTree, []Dashboard, error) {
	folders, err := s.ListFolders()
	if err != nil {
		return nil, nil, err
	}

	dashboards, err := s.List()
	if err != nil {
		return nil, nil, err
	}

	// Build folder map
	folderMap := make(map[string]*FolderTree)
	for _, f := range folders {
		ft := FolderTree{Folder: f}
		folderMap[f.ID] = &ft
	}

	// Assign dashboards to folders
	var rootDashboards []Dashboard
	for _, d := range dashboards {
		if d.FolderID != nil {
			if ft, ok := folderMap[*d.FolderID]; ok {
				ft.Dashboards = append(ft.Dashboards, d)
			}
		} else {
			rootDashboards = append(rootDashboards, d)
		}
	}

	// Build tree structure
	var rootFolders []FolderTree
	for _, f := range folders {
		ft := folderMap[f.ID]
		if f.ParentID != nil {
			if parent, ok := folderMap[*f.ParentID]; ok {
				parent.Children = append(parent.Children, *ft)
			}
		} else {
			rootFolders = append(rootFolders, *ft)
		}
	}

	return rootFolders, rootDashboards, nil
}

// UpdateFolder updates a folder's name
func (s *Store) UpdateFolder(id string, name string) error {
	result, err := s.db.Exec(
		"UPDATE dashboard_folders SET name = ?, updated = ? WHERE id = ?",
		name, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("update folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder not found")
	}

	return nil
}

// DeleteFolder removes a folder (dashboards inside are moved to root)
func (s *Store) DeleteFolder(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Move dashboards to root
	if _, err := tx.Exec("UPDATE dashboards SET folder_id = NULL WHERE folder_id = ?", id); err != nil {
		return fmt.Errorf("move dashboards: %w", err)
	}

	// Move child folders to root
	if _, err := tx.Exec("UPDATE dashboard_folders SET parent_id = NULL WHERE parent_id = ?", id); err != nil {
		return fmt.Errorf("move child folders: %w", err)
	}

	// Delete folder
	result, err := tx.Exec("DELETE FROM dashboard_folders WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("folder not found")
	}

	return tx.Commit()
}

// ==================== Dashboard Methods ====================

// Create creates a new dashboard
func (s *Store) Create(name string, layout []WidgetPosition, widgetConfig map[string]WidgetConfig, isDefault bool) (*Dashboard, error) {
	id := uuid.New().String()
	now := time.Now()

	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return nil, fmt.Errorf("marshal layout: %w", err)
	}
	if widgetConfig == nil {
		widgetConfig = map[string]WidgetConfig{}
	}
	widgetConfigJSON, err := json.Marshal(widgetConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal widget config: %w", err)
	}

	// If setting as default, clear other defaults first
	if isDefault {
		if _, err := s.db.Exec("UPDATE dashboards SET is_default = 0"); err != nil {
			return nil, fmt.Errorf("clear defaults: %w", err)
		}
	}

	// Get next position
	var position int
	row := s.db.QueryRow("SELECT COALESCE(MAX(position), -1) + 1 FROM dashboards WHERE folder_id IS NULL")
	row.Scan(&position)

	_, err = s.db.Exec(
		"INSERT INTO dashboards (id, name, layout, widget_config, is_default, position, created, updated) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		id, name, string(layoutJSON), string(widgetConfigJSON), isDefault, position, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert dashboard: %w", err)
	}

	return &Dashboard{
		ID:           id,
		Name:         name,
		Layout:       layout,
		WidgetConfig: widgetConfig,
		IsDefault:    isDefault,
		Position:     position,
		Created:      now,
		Updated:      now,
	}, nil
}

// Get retrieves a dashboard by ID
func (s *Store) Get(id string) (*Dashboard, error) {
	row := s.db.QueryRow(
		"SELECT id, name, layout, widget_config, is_default, folder_id, position, created, updated FROM dashboards WHERE id = ?",
		id,
	)

	var d Dashboard
	var layoutJSON string
	var widgetConfigJSON string
	var isDefault int
	var folderID sql.NullString

	err := row.Scan(&d.ID, &d.Name, &layoutJSON, &widgetConfigJSON, &isDefault, &folderID, &d.Position, &d.Created, &d.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan dashboard: %w", err)
	}

	d.IsDefault = isDefault == 1
	if folderID.Valid {
		d.FolderID = &folderID.String
	}

	if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
		return nil, fmt.Errorf("unmarshal layout: %w", err)
	}
	if widgetConfigJSON != "" {
		if err := json.Unmarshal([]byte(widgetConfigJSON), &d.WidgetConfig); err != nil {
			return nil, fmt.Errorf("unmarshal widget config: %w", err)
		}
	}
	if d.WidgetConfig == nil {
		d.WidgetConfig = map[string]WidgetConfig{}
	}

	return &d, nil
}

// GetDefault retrieves the default dashboard
func (s *Store) GetDefault() (*Dashboard, error) {
	row := s.db.QueryRow(
		"SELECT id, name, layout, widget_config, is_default, folder_id, position, created, updated FROM dashboards WHERE is_default = 1",
	)

	var d Dashboard
	var layoutJSON string
	var widgetConfigJSON string
	var isDefault int
	var folderID sql.NullString

	err := row.Scan(&d.ID, &d.Name, &layoutJSON, &widgetConfigJSON, &isDefault, &folderID, &d.Position, &d.Created, &d.Updated)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan dashboard: %w", err)
	}

	d.IsDefault = true
	if folderID.Valid {
		d.FolderID = &folderID.String
	}

	if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
		return nil, fmt.Errorf("unmarshal layout: %w", err)
	}
	if widgetConfigJSON != "" {
		if err := json.Unmarshal([]byte(widgetConfigJSON), &d.WidgetConfig); err != nil {
			return nil, fmt.Errorf("unmarshal widget config: %w", err)
		}
	}
	if d.WidgetConfig == nil {
		d.WidgetConfig = map[string]WidgetConfig{}
	}

	return &d, nil
}

// List returns all dashboards
func (s *Store) List() ([]Dashboard, error) {
	rows, err := s.db.Query(
		"SELECT id, name, layout, widget_config, is_default, folder_id, position, created, updated FROM dashboards ORDER BY position, name",
	)
	if err != nil {
		return nil, fmt.Errorf("query dashboards: %w", err)
	}
	defer rows.Close()

	var dashboards []Dashboard
	for rows.Next() {
		var d Dashboard
		var layoutJSON string
		var widgetConfigJSON string
		var isDefault int
		var folderID sql.NullString

		if err := rows.Scan(&d.ID, &d.Name, &layoutJSON, &widgetConfigJSON, &isDefault, &folderID, &d.Position, &d.Created, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan dashboard: %w", err)
		}

		d.IsDefault = isDefault == 1
		if folderID.Valid {
			d.FolderID = &folderID.String
		}

		if err := json.Unmarshal([]byte(layoutJSON), &d.Layout); err != nil {
			return nil, fmt.Errorf("unmarshal layout: %w", err)
		}
		if widgetConfigJSON != "" {
			if err := json.Unmarshal([]byte(widgetConfigJSON), &d.WidgetConfig); err != nil {
				return nil, fmt.Errorf("unmarshal widget config: %w", err)
			}
		}
		if d.WidgetConfig == nil {
			d.WidgetConfig = map[string]WidgetConfig{}
		}

		dashboards = append(dashboards, d)
	}

	return dashboards, nil
}

// Update updates a dashboard's name and layout
func (s *Store) Update(id string, name string, layout []WidgetPosition, widgetConfig map[string]WidgetConfig) error {
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("marshal layout: %w", err)
	}
	if widgetConfig == nil {
		widgetConfig = map[string]WidgetConfig{}
	}
	widgetConfigJSON, err := json.Marshal(widgetConfig)
	if err != nil {
		return fmt.Errorf("marshal widget config: %w", err)
	}

	result, err := s.db.Exec(
		"UPDATE dashboards SET name = ?, layout = ?, widget_config = ?, updated = ? WHERE id = ?",
		name, string(layoutJSON), string(widgetConfigJSON), time.Now(), id,
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

// MoveDashboard moves a dashboard to a folder
func (s *Store) MoveDashboard(id string, folderID *string) error {
	// Get next position in target folder
	var position int
	if folderID != nil {
		row := s.db.QueryRow("SELECT COALESCE(MAX(position), -1) + 1 FROM dashboards WHERE folder_id = ?", *folderID)
		row.Scan(&position)
	} else {
		row := s.db.QueryRow("SELECT COALESCE(MAX(position), -1) + 1 FROM dashboards WHERE folder_id IS NULL")
		row.Scan(&position)
	}

	result, err := s.db.Exec(
		"UPDATE dashboards SET folder_id = ?, position = ?, updated = ? WHERE id = ?",
		folderID, position, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("move dashboard: %w", err)
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
