package notify

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Store manages notification channel persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new notification store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// init creates the necessary tables
func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS notification_channels (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		last_used_at TEXT,
		last_error TEXT,
		success_count INTEGER DEFAULT 0,
		failure_count INTEGER DEFAULT 0
	);

	CREATE INDEX IF NOT EXISTS idx_channels_org ON notification_channels(org_id);
	CREATE INDEX IF NOT EXISTS idx_channels_type ON notification_channels(type);

	CREATE TABLE IF NOT EXISTS notification_logs (
		id TEXT PRIMARY KEY,
		channel_id TEXT NOT NULL,
		channel_name TEXT NOT NULL,
		channel_type TEXT NOT NULL,
		notification_type TEXT NOT NULL,
		title TEXT NOT NULL,
		message TEXT,
		severity TEXT,
		source TEXT,
		source_type TEXT,
		source_id TEXT,
		labels TEXT,
		status TEXT NOT NULL,
		error TEXT,
		sent_at TEXT NOT NULL,
		response_time_ms INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_logs_channel ON notification_logs(channel_id);
	CREATE INDEX IF NOT EXISTS idx_logs_sent_at ON notification_logs(sent_at);
	CREATE INDEX IF NOT EXISTS idx_logs_status ON notification_logs(status);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateChannel creates a new notification channel
func (s *Store) CreateChannel(channel *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel.ID == "" {
		channel.ID = uuid.New().String()
	}
	now := time.Now()
	channel.CreatedAt = now
	channel.UpdatedAt = now

	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO notification_channels
		(id, org_id, name, type, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, channel.ID, channel.OrgID, channel.Name, channel.Type, string(configJSON),
		channel.Enabled, now.Format(time.RFC3339), now.Format(time.RFC3339))

	return err
}

// GetChannel retrieves a channel by ID
func (s *Store) GetChannel(id string) (*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var channel Channel
	var configStr string
	var createdAt, updatedAt string
	var lastUsedAt, lastError sql.NullString
	var successCount, failureCount int

	err := s.db.QueryRow(`
		SELECT id, org_id, name, type, config, enabled, created_at, updated_at,
		       last_used_at, last_error, success_count, failure_count
		FROM notification_channels WHERE id = ?
	`, id).Scan(&channel.ID, &channel.OrgID, &channel.Name, &channel.Type, &configStr,
		&channel.Enabled, &createdAt, &updatedAt, &lastUsedAt, &lastError,
		&successCount, &failureCount)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	channel.Config = json.RawMessage(configStr)
	channel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	channel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	if lastUsedAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
		channel.LastUsedAt = &t
	}
	if lastError.Valid {
		channel.LastError = lastError.String
	}

	total := successCount + failureCount
	if total > 0 {
		channel.SuccessRate = float64(successCount) / float64(total) * 100
	}

	return &channel, nil
}

// UpdateChannel updates an existing channel
func (s *Store) UpdateChannel(channel *Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	channel.UpdatedAt = time.Now()

	configJSON, err := json.Marshal(channel.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	_, err = s.db.Exec(`
		UPDATE notification_channels
		SET name = ?, type = ?, config = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, channel.Name, channel.Type, string(configJSON), channel.Enabled,
		channel.UpdatedAt.Format(time.RFC3339), channel.ID)

	return err
}

// DeleteChannel deletes a channel
func (s *Store) DeleteChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM notification_channels WHERE id = ?", id)
	return err
}

// ListChannels returns all channels, optionally filtered by org
func (s *Store) ListChannels(orgID string) ([]*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, org_id, name, type, config, enabled, created_at, updated_at,
		       last_used_at, last_error, success_count, failure_count
		FROM notification_channels
	`
	args := []interface{}{}

	if orgID != "" {
		query += " WHERE org_id = ?"
		args = append(args, orgID)
	}
	query += " ORDER BY name"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var channel Channel
		var configStr string
		var createdAt, updatedAt string
		var lastUsedAt, lastError sql.NullString
		var successCount, failureCount int

		err := rows.Scan(&channel.ID, &channel.OrgID, &channel.Name, &channel.Type,
			&configStr, &channel.Enabled, &createdAt, &updatedAt, &lastUsedAt,
			&lastError, &successCount, &failureCount)
		if err != nil {
			return nil, err
		}

		channel.Config = json.RawMessage(configStr)
		channel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		channel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if lastUsedAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
			channel.LastUsedAt = &t
		}
		if lastError.Valid {
			channel.LastError = lastError.String
		}

		total := successCount + failureCount
		if total > 0 {
			channel.SuccessRate = float64(successCount) / float64(total) * 100
		}

		channels = append(channels, &channel)
	}

	return channels, nil
}

// ListChannelsByType returns channels of a specific type
func (s *Store) ListChannelsByType(channelType ChannelType) ([]*Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, name, type, config, enabled, created_at, updated_at,
		       last_used_at, last_error, success_count, failure_count
		FROM notification_channels
		WHERE type = ? AND enabled = 1
		ORDER BY name
	`, channelType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		var channel Channel
		var configStr string
		var createdAt, updatedAt string
		var lastUsedAt, lastError sql.NullString
		var successCount, failureCount int

		err := rows.Scan(&channel.ID, &channel.OrgID, &channel.Name, &channel.Type,
			&configStr, &channel.Enabled, &createdAt, &updatedAt, &lastUsedAt,
			&lastError, &successCount, &failureCount)
		if err != nil {
			return nil, err
		}

		channel.Config = json.RawMessage(configStr)
		channel.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		channel.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		if lastUsedAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastUsedAt.String)
			channel.LastUsedAt = &t
		}
		if lastError.Valid {
			channel.LastError = lastError.String
		}

		total := successCount + failureCount
		if total > 0 {
			channel.SuccessRate = float64(successCount) / float64(total) * 100
		}

		channels = append(channels, &channel)
	}

	return channels, nil
}

// RecordSuccess records a successful notification send
func (s *Store) RecordSuccess(channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE notification_channels
		SET last_used_at = ?, last_error = '', success_count = success_count + 1
		WHERE id = ?
	`, now, channelID)
	return err
}

// RecordFailure records a failed notification send
func (s *Store) RecordFailure(channelID, errorMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Format(time.RFC3339)
	_, err := s.db.Exec(`
		UPDATE notification_channels
		SET last_used_at = ?, last_error = ?, failure_count = failure_count + 1
		WHERE id = ?
	`, now, errorMsg, channelID)
	return err
}

// CreateLog creates a notification log entry
func (s *Store) CreateLog(log *NotificationLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if log.ID == "" {
		log.ID = uuid.New().String()
	}

	labelsJSON := "{}"
	if log.Notification != nil && log.Notification.Labels != nil {
		data, _ := json.Marshal(log.Notification.Labels)
		labelsJSON = string(data)
	}

	message := ""
	severity := ""
	source := ""
	sourceType := ""
	sourceID := ""
	notificationType := ""
	title := ""

	if log.Notification != nil {
		message = log.Notification.Message
		severity = string(log.Notification.Severity)
		source = log.Notification.Source
		sourceType = log.Notification.SourceType
		sourceID = log.Notification.SourceID
		notificationType = string(log.Notification.Type)
		title = log.Notification.Title
	}

	_, err := s.db.Exec(`
		INSERT INTO notification_logs
		(id, channel_id, channel_name, channel_type, notification_type, title, message,
		 severity, source, source_type, source_id, labels, status, error, sent_at, response_time_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.ChannelID, log.ChannelName, log.ChannelType, notificationType, title,
		message, severity, source, sourceType, sourceID, labelsJSON, log.Status, log.Error,
		log.SentAt.Format(time.RFC3339), log.ResponseTime)

	return err
}

// ListLogs returns recent notification logs
func (s *Store) ListLogs(limit int, channelID string) ([]*NotificationLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, channel_id, channel_name, channel_type, notification_type, title,
		       message, severity, source, source_type, source_id, labels, status,
		       error, sent_at, response_time_ms
		FROM notification_logs
	`
	args := []interface{}{}

	if channelID != "" {
		query += " WHERE channel_id = ?"
		args = append(args, channelID)
	}

	query += " ORDER BY sent_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*NotificationLog
	for rows.Next() {
		var log NotificationLog
		var notifType, title, message, severity, source, sourceType, sourceID string
		var labelsJSON string
		var sentAt string
		var errorStr sql.NullString

		err := rows.Scan(&log.ID, &log.ChannelID, &log.ChannelName, &log.ChannelType,
			&notifType, &title, &message, &severity, &source, &sourceType, &sourceID,
			&labelsJSON, &log.Status, &errorStr, &sentAt, &log.ResponseTime)
		if err != nil {
			return nil, err
		}

		log.Notification = &Notification{
			Type:       NotificationType(notifType),
			Title:      title,
			Message:    message,
			Severity:   Severity(severity),
			Source:     source,
			SourceType: sourceType,
			SourceID:   sourceID,
		}

		if labelsJSON != "" && labelsJSON != "{}" {
			json.Unmarshal([]byte(labelsJSON), &log.Notification.Labels)
		}

		log.SentAt, _ = time.Parse(time.RFC3339, sentAt)
		if errorStr.Valid {
			log.Error = errorStr.String
		}

		logs = append(logs, &log)
	}

	return logs, nil
}

// CleanupOldLogs removes logs older than the specified duration
func (s *Store) CleanupOldLogs(maxAge time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	result, err := s.db.Exec("DELETE FROM notification_logs WHERE sent_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetChannelStats returns statistics for a channel
func (s *Store) GetChannelStats(channelID string, since time.Time) (sent, failed int, avgResponseMs int64, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err = s.db.QueryRow(`
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed,
			AVG(response_time_ms) as avg_response
		FROM notification_logs
		WHERE channel_id = ? AND sent_at >= ?
	`, channelID, since.Format(time.RFC3339)).Scan(&sent, &failed, &avgResponseMs)

	return
}
