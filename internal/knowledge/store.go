package knowledge

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store handles persistent storage of knowledge objects
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new knowledge store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	store := &Store{db: db}

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	// Initialize built-in objects
	if err := store.initBuiltins(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing builtins: %w", err)
	}

	return store, nil
}

func (s *Store) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS knowledge_objects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			definition TEXT NOT NULL,
			description TEXT,
			tags TEXT,
			owner TEXT,
			shared INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_name ON knowledge_objects(name)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_type ON knowledge_objects(type)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_owner ON knowledge_objects(owner)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("executing %q: %w", q[:50], err)
		}
	}

	return nil
}

// Create creates a new knowledge object
func (s *Store) Create(obj *KnowledgeObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if obj.ID == "" {
		obj.ID = fmt.Sprintf("ko_%d", time.Now().UnixNano())
	}

	now := time.Now()
	obj.CreatedAt = now
	obj.UpdatedAt = now

	tagsJSON, _ := json.Marshal(obj.Tags)

	_, err := s.db.Exec(`
		INSERT INTO knowledge_objects (id, name, type, definition, description, tags, owner, shared, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, obj.ID, obj.Name, obj.Type, obj.Definition, obj.Description, string(tagsJSON), obj.Owner, obj.Shared, now, now)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return ErrDuplicateName
		}
		return err
	}

	return nil
}

// Get retrieves a knowledge object by ID
func (s *Store) Get(id string) (*KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getByID(id)
}

func (s *Store) getByID(id string) (*KnowledgeObject, error) {
	var obj KnowledgeObject
	var tagsJSON string
	var createdAt, updatedAt string

	err := s.db.QueryRow(`
		SELECT id, name, type, definition, description, tags, owner, shared, created_at, updated_at
		FROM knowledge_objects
		WHERE id = ?
	`, id).Scan(&obj.ID, &obj.Name, &obj.Type, &obj.Definition, &obj.Description, &tagsJSON, &obj.Owner, &obj.Shared, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(tagsJSON), &obj.Tags)
	obj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	obj.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &obj, nil
}

// GetByName retrieves a knowledge object by name
func (s *Store) GetByName(name string) (*KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getByName(name)
}

func (s *Store) getByName(name string) (*KnowledgeObject, error) {
	var obj KnowledgeObject
	var tagsJSON string
	var createdAt, updatedAt string

	err := s.db.QueryRow(`
		SELECT id, name, type, definition, description, tags, owner, shared, created_at, updated_at
		FROM knowledge_objects
		WHERE name = ?
	`, name).Scan(&obj.ID, &obj.Name, &obj.Type, &obj.Definition, &obj.Description, &tagsJSON, &obj.Owner, &obj.Shared, &createdAt, &updatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(tagsJSON), &obj.Tags)
	obj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	obj.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return &obj, nil
}

// Update updates an existing knowledge object
func (s *Store) Update(obj *KnowledgeObject) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	obj.UpdatedAt = time.Now()
	tagsJSON, _ := json.Marshal(obj.Tags)

	result, err := s.db.Exec(`
		UPDATE knowledge_objects
		SET name = ?, type = ?, definition = ?, description = ?, tags = ?, owner = ?, shared = ?, updated_at = ?
		WHERE id = ?
	`, obj.Name, obj.Type, obj.Definition, obj.Description, string(tagsJSON), obj.Owner, obj.Shared, obj.UpdatedAt, obj.ID)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return ErrDuplicateName
		}
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// Delete removes a knowledge object
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec("DELETE FROM knowledge_objects WHERE id = ?", id)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// ListFilters contains filters for listing knowledge objects
type ListFilters struct {
	Type   ObjectType
	Owner  string
	Shared *bool
	Tags   []string
}

// List returns knowledge objects matching filters
func (s *Store) List(filters ListFilters) ([]*KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT id, name, type, definition, description, tags, owner, shared, created_at, updated_at FROM knowledge_objects WHERE 1=1"
	var args []interface{}

	if filters.Type != "" {
		query += " AND type = ?"
		args = append(args, filters.Type)
	}

	if filters.Owner != "" {
		query += " AND owner = ?"
		args = append(args, filters.Owner)
	}

	if filters.Shared != nil {
		query += " AND shared = ?"
		args = append(args, *filters.Shared)
	}

	query += " ORDER BY name ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var objects []*KnowledgeObject
	for rows.Next() {
		var obj KnowledgeObject
		var tagsJSON string
		var createdAt, updatedAt string

		if err := rows.Scan(&obj.ID, &obj.Name, &obj.Type, &obj.Definition, &obj.Description, &tagsJSON, &obj.Owner, &obj.Shared, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(tagsJSON), &obj.Tags)
		obj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		obj.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		// Filter by tags if specified
		if len(filters.Tags) > 0 {
			matched := false
			for _, ft := range filters.Tags {
				for _, t := range obj.Tags {
					if t == ft {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		objects = append(objects, &obj)
	}

	return objects, nil
}

// Search searches knowledge objects by name or description
func (s *Store) Search(term string) ([]*KnowledgeObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchTerm := "%" + term + "%"

	rows, err := s.db.Query(`
		SELECT id, name, type, definition, description, tags, owner, shared, created_at, updated_at
		FROM knowledge_objects
		WHERE name LIKE ? OR description LIKE ?
		ORDER BY name ASC
	`, searchTerm, searchTerm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var objects []*KnowledgeObject
	for rows.Next() {
		var obj KnowledgeObject
		var tagsJSON string
		var createdAt, updatedAt string

		if err := rows.Scan(&obj.ID, &obj.Name, &obj.Type, &obj.Definition, &obj.Description, &tagsJSON, &obj.Owner, &obj.Shared, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(tagsJSON), &obj.Tags)
		obj.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		obj.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		objects = append(objects, &obj)
	}

	return objects, nil
}

// GetAllMacros returns all macro objects for query expansion
func (s *Store) GetAllMacros() ([]*KnowledgeObject, error) {
	return s.List(ListFilters{Type: TypeMacro})
}

// GetAllFieldExtractions returns all field extraction objects
func (s *Store) GetAllFieldExtractions() ([]*KnowledgeObject, error) {
	return s.List(ListFilters{Type: TypeFieldExtraction})
}

// GetAllLookups returns all lookup objects
func (s *Store) GetAllLookups() ([]*KnowledgeObject, error) {
	return s.List(ListFilters{Type: TypeLookup})
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// initBuiltins initializes built-in knowledge objects
func (s *Store) initBuiltins() error {
	builtins := []KnowledgeObject{
		// Macros
		{
			ID:          "builtin_error_logs",
			Name:        "error_logs",
			Type:        TypeMacro,
			Description: "Filter for error level logs",
			Tags:        []string{"builtin", "logs"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_slow_requests",
			Name:        "slow_requests",
			Type:        TypeMacro,
			Description: "Filter for slow requests (threshold in ms as $1)",
			Tags:        []string{"builtin", "traces", "performance"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_error_rate",
			Name:        "error_rate",
			Type:        TypeMacro,
			Description: "Calculate error rate by service",
			Tags:        []string{"builtin", "metrics", "errors"},
			Owner:       "system",
			Shared:      true,
		},
		// Field Extractions
		{
			ID:          "builtin_apache_combined",
			Name:        "apache_combined",
			Type:        TypeFieldExtraction,
			Description: "Parse Apache combined log format",
			Tags:        []string{"builtin", "logs", "apache", "webserver"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_json_extract",
			Name:        "json_extract",
			Type:        TypeFieldExtraction,
			Description: "Extract JSON fields from log messages",
			Tags:        []string{"builtin", "logs", "json"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_nginx_combined",
			Name:        "nginx_combined",
			Type:        TypeFieldExtraction,
			Description: "Parse Nginx combined log format",
			Tags:        []string{"builtin", "logs", "nginx", "webserver"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_syslog",
			Name:        "syslog",
			Type:        TypeFieldExtraction,
			Description: "Parse syslog format messages",
			Tags:        []string{"builtin", "logs", "syslog", "system"},
			Owner:       "system",
			Shared:      true,
		},
		// Lookups
		{
			ID:          "builtin_http_status_codes",
			Name:        "http_status_codes",
			Type:        TypeLookup,
			Description: "Map HTTP status codes to descriptions",
			Tags:        []string{"builtin", "http", "lookup"},
			Owner:       "system",
			Shared:      true,
		},
		{
			ID:          "builtin_log_levels",
			Name:        "log_levels",
			Type:        TypeLookup,
			Description: "Map log level abbreviations to full names",
			Tags:        []string{"builtin", "logs", "lookup"},
			Owner:       "system",
			Shared:      true,
		},
	}

	// Set definitions
	errorLogsMacro := Macro{Expression: `logs | where level = "error"`, Args: nil}
	builtins[0].SetDefinition(errorLogsMacro)

	slowRequestsMacro := Macro{Expression: `traces | where duration > $1`, Args: []string{"threshold"}}
	builtins[1].SetDefinition(slowRequestsMacro)

	errorRateMacro := Macro{
		Expression: `logs | where level = "error" | count by (service) | select service, count as errors`,
		Args:       nil,
	}
	builtins[2].SetDefinition(errorRateMacro)

	apacheFE := FieldExtraction{
		Pattern:      `^(?P<remote_addr>\S+) \S+ (?P<remote_user>\S+) \[(?P<time_local>[^\]]+)\] "(?P<method>\S+) (?P<request>\S+) (?P<protocol>\S+)" (?P<status>\d+) (?P<body_bytes_sent>\d+) "(?P<http_referer>[^"]*)" "(?P<http_user_agent>[^"]*)"`,
		SourceField:  "message",
		TargetFields: []string{"remote_addr", "remote_user", "time_local", "method", "request", "protocol", "status", "body_bytes_sent", "http_referer", "http_user_agent"},
	}
	builtins[3].SetDefinition(apacheFE)

	jsonFE := FieldExtraction{
		Pattern:      `\{[^}]+\}`,
		SourceField:  "message",
		TargetFields: []string{}, // Dynamic based on JSON content
	}
	builtins[4].SetDefinition(jsonFE)

	nginxFE := FieldExtraction{
		Pattern:      `^(?P<remote_addr>\S+) - (?P<remote_user>\S+) \[(?P<time_local>[^\]]+)\] "(?P<request>[^"]*)" (?P<status>\d+) (?P<body_bytes_sent>\d+) "(?P<http_referer>[^"]*)" "(?P<http_user_agent>[^"]*)"`,
		SourceField:  "message",
		TargetFields: []string{"remote_addr", "remote_user", "time_local", "request", "status", "body_bytes_sent", "http_referer", "http_user_agent"},
	}
	builtins[5].SetDefinition(nginxFE)

	syslogFE := FieldExtraction{
		Pattern:      `^(?P<timestamp>\w+\s+\d+\s+\d+:\d+:\d+)\s+(?P<hostname>\S+)\s+(?P<program>\S+?)(?:\[(?P<pid>\d+)\])?:\s+(?P<message>.*)$`,
		SourceField:  "message",
		TargetFields: []string{"timestamp", "hostname", "program", "pid", "message"},
	}
	builtins[6].SetDefinition(syslogFE)

	httpStatusLookup := Lookup{
		KeyField:     "status",
		OutputFields: []string{"status_text", "status_category"},
		Data: map[string]map[string]string{
			"200": {"status_text": "OK", "status_category": "success"},
			"201": {"status_text": "Created", "status_category": "success"},
			"204": {"status_text": "No Content", "status_category": "success"},
			"301": {"status_text": "Moved Permanently", "status_category": "redirect"},
			"302": {"status_text": "Found", "status_category": "redirect"},
			"304": {"status_text": "Not Modified", "status_category": "redirect"},
			"400": {"status_text": "Bad Request", "status_category": "client_error"},
			"401": {"status_text": "Unauthorized", "status_category": "client_error"},
			"403": {"status_text": "Forbidden", "status_category": "client_error"},
			"404": {"status_text": "Not Found", "status_category": "client_error"},
			"405": {"status_text": "Method Not Allowed", "status_category": "client_error"},
			"429": {"status_text": "Too Many Requests", "status_category": "client_error"},
			"500": {"status_text": "Internal Server Error", "status_category": "server_error"},
			"502": {"status_text": "Bad Gateway", "status_category": "server_error"},
			"503": {"status_text": "Service Unavailable", "status_category": "server_error"},
			"504": {"status_text": "Gateway Timeout", "status_category": "server_error"},
		},
	}
	builtins[7].SetDefinition(httpStatusLookup)

	logLevelsLookup := Lookup{
		KeyField:     "level",
		OutputFields: []string{"level_name", "level_severity"},
		Data: map[string]map[string]string{
			"D":     {"level_name": "DEBUG", "level_severity": "1"},
			"DBG":   {"level_name": "DEBUG", "level_severity": "1"},
			"DEBUG": {"level_name": "DEBUG", "level_severity": "1"},
			"I":     {"level_name": "INFO", "level_severity": "2"},
			"INF":   {"level_name": "INFO", "level_severity": "2"},
			"INFO":  {"level_name": "INFO", "level_severity": "2"},
			"W":     {"level_name": "WARN", "level_severity": "3"},
			"WRN":   {"level_name": "WARN", "level_severity": "3"},
			"WARN":  {"level_name": "WARN", "level_severity": "3"},
			"WARNING": {"level_name": "WARN", "level_severity": "3"},
			"E":     {"level_name": "ERROR", "level_severity": "4"},
			"ERR":   {"level_name": "ERROR", "level_severity": "4"},
			"ERROR": {"level_name": "ERROR", "level_severity": "4"},
			"F":     {"level_name": "FATAL", "level_severity": "5"},
			"FATAL": {"level_name": "FATAL", "level_severity": "5"},
		},
	}
	builtins[8].SetDefinition(logLevelsLookup)

	// Insert builtins if they don't exist
	for _, b := range builtins {
		existing, err := s.getByID(b.ID)
		if err == ErrNotFound {
			s.db.Exec(`
				INSERT INTO knowledge_objects (id, name, type, definition, description, tags, owner, shared, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, b.ID, b.Name, b.Type, b.Definition, b.Description, mustJSON(b.Tags), b.Owner, b.Shared, time.Now(), time.Now())
		} else if err == nil && existing.Owner == "system" {
			// Update system builtins
			s.db.Exec(`
				UPDATE knowledge_objects
				SET definition = ?, description = ?, tags = ?, updated_at = ?
				WHERE id = ?
			`, b.Definition, b.Description, mustJSON(b.Tags), time.Now(), b.ID)
		}
	}

	return nil
}

func mustJSON(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}
