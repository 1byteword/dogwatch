package siem

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store handles SIEM configuration persistence
type Store struct {
	db *sql.DB
}

func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// init creates the necessary tables
func (s *Store) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS siem_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			format TEXT NOT NULL,
			exporter_type TEXT NOT NULL,
			config_json TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS siem_export_history (
			id TEXT PRIMARY KEY,
			config_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			events_count INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			duration_ms INTEGER NOT NULL,
			FOREIGN KEY (config_id) REFERENCES siem_configs(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_siem_export_history_config
			ON siem_export_history(config_id, timestamp);
	`)
	return err
}

// SaveConfig saves or updates a SIEM configuration
func (s *Store) SaveConfig(config Config) error {
	now := time.Now()

	if config.ID == "" {
		config.ID = uuid.New().String()
		config.CreatedAt = now
	}
	config.UpdatedAt = now

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO siem_configs (id, name, enabled, format, exporter_type, config_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			enabled = excluded.enabled,
			format = excluded.format,
			exporter_type = excluded.exporter_type,
			config_json = excluded.config_json,
			updated_at = excluded.updated_at
	`, config.ID, config.Name, config.Enabled, config.Format, config.ExporterType,
		string(configJSON), config.CreatedAt, config.UpdatedAt)

	return err
}

// GetConfig retrieves a SIEM configuration by ID
func (s *Store) GetConfig(id string) (*Config, error) {
	var configJSON string
	err := s.db.QueryRow(`
		SELECT config_json FROM siem_configs WHERE id = ?
	`, id).Scan(&configJSON)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("config not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &config, nil
}

// ListConfigs lists all SIEM configurations
func (s *Store) ListConfigs() ([]Config, error) {
	rows, err := s.db.Query(`
		SELECT config_json FROM siem_configs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []Config
	for rows.Next() {
		var configJSON string
		if err := rows.Scan(&configJSON); err != nil {
			return nil, err
		}

		var config Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			continue
		}
		configs = append(configs, config)
	}

	return configs, rows.Err()
}

// DeleteConfig deletes a SIEM configuration
func (s *Store) DeleteConfig(id string) error {
	result, err := s.db.Exec(`DELETE FROM siem_configs WHERE id = ?`, id)
	if err != nil {
		return err
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("config not found: %s", id)
	}

	return nil
}

// RecordExport records an export attempt
func (s *Store) RecordExport(configID string, eventsCount int, success bool, errMsg string, durationMs int64) error {
	_, err := s.db.Exec(`
		INSERT INTO siem_export_history (id, config_id, timestamp, events_count, success, error_message, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, uuid.New().String(), configID, time.Now(), eventsCount, success, errMsg, durationMs)

	return err
}

// ExportHistoryEntry represents a historical export record
type ExportHistoryEntry struct {
	ID           string    `json:"id"`
	ConfigID     string    `json:"config_id"`
	Timestamp    time.Time `json:"timestamp"`
	EventsCount  int       `json:"events_count"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
}

// GetExportHistory retrieves export history for a configuration
func (s *Store) GetExportHistory(configID string, limit int) ([]ExportHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, config_id, timestamp, events_count, success, error_message, duration_ms
		FROM siem_export_history
		WHERE config_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, configID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []ExportHistoryEntry
	for rows.Next() {
		var entry ExportHistoryEntry
		var errMsg sql.NullString
		if err := rows.Scan(&entry.ID, &entry.ConfigID, &entry.Timestamp, &entry.EventsCount,
			&entry.Success, &errMsg, &entry.DurationMs); err != nil {
			return nil, err
		}
		entry.ErrorMessage = errMsg.String
		history = append(history, entry)
	}

	return history, rows.Err()
}

// GetExportStats returns aggregated export statistics
func (s *Store) GetExportStats(configID string, since time.Time) (*ExportStatsAggregated, error) {
	var stats ExportStatsAggregated

	err := s.db.QueryRow(`
		SELECT
			COUNT(*) as total_exports,
			SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END) as successful_exports,
			SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END) as failed_exports,
			SUM(events_count) as total_events,
			AVG(duration_ms) as avg_duration_ms,
			MAX(timestamp) as last_export
		FROM siem_export_history
		WHERE config_id = ? AND timestamp >= ?
	`, configID, since).Scan(
		&stats.TotalExports,
		&stats.SuccessfulExports,
		&stats.FailedExports,
		&stats.TotalEvents,
		&stats.AvgDurationMs,
		&stats.LastExport,
	)

	if err != nil {
		return nil, err
	}

	stats.ConfigID = configID
	stats.Since = since
	return &stats, nil
}

// ExportStatsAggregated contains aggregated export statistics
type ExportStatsAggregated struct {
	ConfigID          string    `json:"config_id"`
	Since             time.Time `json:"since"`
	TotalExports      int       `json:"total_exports"`
	SuccessfulExports int       `json:"successful_exports"`
	FailedExports     int       `json:"failed_exports"`
	TotalEvents       int64     `json:"total_events"`
	AvgDurationMs     float64   `json:"avg_duration_ms"`
	LastExport        time.Time `json:"last_export"`
}

// CleanupHistory removes old export history records
func (s *Store) CleanupHistory(olderThan time.Time) (int64, error) {
	result, err := s.db.Exec(`
		DELETE FROM siem_export_history WHERE timestamp < ?
	`, olderThan)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}
