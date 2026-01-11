package watch

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// State represents the current state of a watch
type State string

const (
	StateOK       State = "OK"
	StateAlerting State = "ALERTING"
	StatePending  State = "PENDING"
	StateNoData   State = "NO_DATA"
	StateMuted    State = "MUTED"
)

// Operator for threshold comparison
type Operator string

const (
	OpGreaterThan      Operator = ">"
	OpGreaterOrEqual   Operator = ">="
	OpLessThan         Operator = "<"
	OpLessOrEqual      Operator = "<="
	OpEqual            Operator = "=="
	OpNotEqual         Operator = "!="
)

// Metric types we can watch
type MetricType string

const (
	MetricCPU           MetricType = "cpu_percent"
	MetricMemory        MetricType = "mem_percent"
	MetricDiskRead      MetricType = "disk_read_ps"
	MetricDiskWrite     MetricType = "disk_write_ps"
	MetricNetRx         MetricType = "net_rx_ps"
	MetricNetTx         MetricType = "net_tx_ps"
	MetricLoad          MetricType = "load_1"
	MetricConnections   MetricType = "connections"
	MetricRequests      MetricType = "requests"
	MetricErrors        MetricType = "errors"
	MetricErrorRate     MetricType = "error_rate"
	MetricLatencyP50    MetricType = "latency_p50"
	MetricLatencyP99    MetricType = "latency_p99"
)

// Watch defines an alerting rule
type Watch struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Metric      MetricType `json:"metric"`
	Operator    Operator   `json:"operator"`
	Threshold   float64    `json:"threshold"`
	Duration    string     `json:"duration"` // e.g., "5m", "1h"
	State       State      `json:"state"`
	StateAt     time.Time  `json:"state_at"`
	LastValue   float64    `json:"last_value"`
	LastCheck   time.Time  `json:"last_check"`
	MutedUntil  *time.Time `json:"muted_until,omitempty"`
	Channels    []string   `json:"channels"` // notification channel IDs
	Created     time.Time  `json:"created"`
	Updated     time.Time  `json:"updated"`
	Enabled     bool       `json:"enabled"`
}

// Channel represents a notification channel
type Channel struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Type    string          `json:"type"` // "webhook", "slack"
	Config  json.RawMessage `json:"config"`
	Created time.Time       `json:"created"`
}

// WebhookConfig for webhook notifications
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // defaults to POST
	Headers map[string]string `json:"headers,omitempty"`
}

// SlackConfig for Slack notifications
type SlackConfig struct {
	WebhookURL string `json:"webhook_url"`
	Channel    string `json:"channel,omitempty"`
	Username   string `json:"username,omitempty"`
}

// Event records a watch state change
type Event struct {
	ID        string    `json:"id"`
	WatchID   string    `json:"watch_id"`
	WatchName string    `json:"watch_name"`
	FromState State     `json:"from_state"`
	ToState   State     `json:"to_state"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// Store manages watch persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new watch store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS watches (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT,
		metric TEXT NOT NULL,
		operator TEXT NOT NULL,
		threshold REAL NOT NULL,
		duration TEXT NOT NULL,
		state TEXT DEFAULT 'NO_DATA',
		state_at TEXT,
		last_value REAL,
		last_check TEXT,
		muted_until TEXT,
		channels TEXT,
		created TEXT NOT NULL,
		updated TEXT NOT NULL,
		enabled INTEGER DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS channels (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT NOT NULL,
		created TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS watch_events (
		id TEXT PRIMARY KEY,
		watch_id TEXT NOT NULL,
		watch_name TEXT NOT NULL,
		from_state TEXT,
		to_state TEXT NOT NULL,
		value REAL,
		threshold REAL,
		message TEXT,
		timestamp TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_events_watch ON watch_events(watch_id);
	CREATE INDEX IF NOT EXISTS idx_events_time ON watch_events(timestamp);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveWatch creates or updates a watch
func (s *Store) SaveWatch(w *Watch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channels, _ := json.Marshal(w.Channels)
	now := time.Now()

	if w.Created.IsZero() {
		w.Created = now
	}
	w.Updated = now

	var mutedUntil *string
	if w.MutedUntil != nil {
		t := w.MutedUntil.Format(time.RFC3339)
		mutedUntil = &t
	}

	var stateAt *string
	if !w.StateAt.IsZero() {
		t := w.StateAt.Format(time.RFC3339)
		stateAt = &t
	}

	var lastCheck *string
	if !w.LastCheck.IsZero() {
		t := w.LastCheck.Format(time.RFC3339)
		lastCheck = &t
	}

	_, err := s.db.Exec(`
		INSERT INTO watches (id, name, description, metric, operator, threshold, duration,
			state, state_at, last_value, last_check, muted_until, channels, created, updated, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, description=excluded.description, metric=excluded.metric,
			operator=excluded.operator, threshold=excluded.threshold, duration=excluded.duration,
			state=excluded.state, state_at=excluded.state_at, last_value=excluded.last_value,
			last_check=excluded.last_check, muted_until=excluded.muted_until, channels=excluded.channels,
			updated=excluded.updated, enabled=excluded.enabled
	`, w.ID, w.Name, w.Description, w.Metric, w.Operator, w.Threshold, w.Duration,
		w.State, stateAt, w.LastValue, lastCheck, mutedUntil, string(channels),
		w.Created.Format(time.RFC3339), w.Updated.Format(time.RFC3339), w.Enabled)

	return err
}

// GetWatch retrieves a watch by ID
func (s *Store) GetWatch(id string) (*Watch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRow(`SELECT id, name, description, metric, operator, threshold, duration,
		state, state_at, last_value, last_check, muted_until, channels, created, updated, enabled
		FROM watches WHERE id = ?`, id)

	return scanWatch(row)
}

// ListWatches returns all watches
func (s *Store) ListWatches() ([]*Watch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT id, name, description, metric, operator, threshold, duration,
		state, state_at, last_value, last_check, muted_until, channels, created, updated, enabled
		FROM watches ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var watches []*Watch
	for rows.Next() {
		w, err := scanWatch(rows)
		if err != nil {
			return nil, err
		}
		watches = append(watches, w)
	}
	return watches, rows.Err()
}

// DeleteWatch removes a watch
func (s *Store) DeleteWatch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM watches WHERE id = ?", id)
	return err
}

// SaveChannel creates or updates a notification channel
func (s *Store) SaveChannel(c *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.Created.IsZero() {
		c.Created = time.Now()
	}

	_, err := s.db.Exec(`
		INSERT INTO channels (id, name, type, config, created)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, type=excluded.type, config=excluded.config
	`, c.ID, c.Name, c.Type, string(c.Config), c.Created.Format(time.RFC3339))

	return err
}

// ListChannels returns all notification channels
func (s *Store) ListChannels() ([]*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT id, name, type, config, created FROM channels ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var c Channel
		var config, created string
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &config, &created); err != nil {
			return nil, err
		}
		c.Config = json.RawMessage(config)
		c.Created, _ = time.Parse(time.RFC3339, created)
		channels = append(channels, &c)
	}
	return channels, rows.Err()
}

// GetChannel retrieves a channel by ID
func (s *Store) GetChannel(id string) (*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var c Channel
	var config, created string
	err := s.db.QueryRow("SELECT id, name, type, config, created FROM channels WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.Type, &config, &created)
	if err != nil {
		return nil, err
	}
	c.Config = json.RawMessage(config)
	c.Created, _ = time.Parse(time.RFC3339, created)
	return &c, nil
}

// DeleteChannel removes a notification channel
func (s *Store) DeleteChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM channels WHERE id = ?", id)
	return err
}

// RecordEvent saves a watch event
func (s *Store) RecordEvent(e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO watch_events (id, watch_id, watch_name, from_state, to_state, value, threshold, message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.WatchID, e.WatchName, e.FromState, e.ToState, e.Value, e.Threshold, e.Message,
		e.Timestamp.Format(time.RFC3339))

	return err
}

// GetEvents returns recent events
func (s *Store) GetEvents(limit int, watchID string) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, watch_id, watch_name, from_state, to_state, value, threshold, message, timestamp
		FROM watch_events`
	args := []interface{}{}

	if watchID != "" {
		query += " WHERE watch_id = ?"
		args = append(args, watchID)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var e Event
		var ts, fromState string
		if err := rows.Scan(&e.ID, &e.WatchID, &e.WatchName, &fromState, &e.ToState,
			&e.Value, &e.Threshold, &e.Message, &ts); err != nil {
			return nil, err
		}
		e.FromState = State(fromState)
		e.Timestamp, _ = time.Parse(time.RFC3339, ts)
		events = append(events, &e)
	}
	return events, rows.Err()
}

// scanner interface for row/rows
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanWatch(s scanner) (*Watch, error) {
	var w Watch
	var stateAt, lastCheck, mutedUntil, channels, created, updated sql.NullString
	var enabled int

	err := s.Scan(&w.ID, &w.Name, &w.Description, &w.Metric, &w.Operator, &w.Threshold,
		&w.Duration, &w.State, &stateAt, &w.LastValue, &lastCheck, &mutedUntil,
		&channels, &created, &updated, &enabled)
	if err != nil {
		return nil, err
	}

	if stateAt.Valid {
		w.StateAt, _ = time.Parse(time.RFC3339, stateAt.String)
	}
	if lastCheck.Valid {
		w.LastCheck, _ = time.Parse(time.RFC3339, lastCheck.String)
	}
	if mutedUntil.Valid {
		t, _ := time.Parse(time.RFC3339, mutedUntil.String)
		w.MutedUntil = &t
	}
	if channels.Valid {
		json.Unmarshal([]byte(channels.String), &w.Channels)
	}
	if created.Valid {
		w.Created, _ = time.Parse(time.RFC3339, created.String)
	}
	if updated.Valid {
		w.Updated, _ = time.Parse(time.RFC3339, updated.String)
	}
	w.Enabled = enabled == 1

	return &w, nil
}
