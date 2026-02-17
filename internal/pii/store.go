package pii

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TokenStore manages reversible tokenization
type TokenStore struct {
	db        *sql.DB
	encryptor *Encryptor
	cache     map[string]TokenEntry
	mu        sync.RWMutex
}

// TokenEntry represents a stored token mapping
type TokenEntry struct {
	Token     string    `json:"token"`
	Type      PIIType   `json:"type"`
	Value     string    `json:"value"` // Encrypted in DB
	CreatedAt time.Time `json:"created_at"`
}

// NewTokenStore creates a new token store
func NewTokenStore(db *sql.DB) *TokenStore {
	ts := &TokenStore{
		db:    db,
		cache: make(map[string]TokenEntry),
	}

	if db != nil {
		ts.createTables()
	}

	return ts
}

// NewTokenStoreWithEncryption creates a token store with encryption
func NewTokenStoreWithEncryption(db *sql.DB, encryptionKey []byte) (*TokenStore, error) {
	enc, err := NewEncryptor(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("creating encryptor: %w", err)
	}

	ts := &TokenStore{
		db:        db,
		encryptor: enc,
		cache:     make(map[string]TokenEntry),
	}

	if db != nil {
		if err := ts.createTables(); err != nil {
			return nil, fmt.Errorf("creating tables: %w", err)
		}
	}

	return ts, nil
}

func (ts *TokenStore) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS pii_tokens (
		token TEXT PRIMARY KEY,
		pii_type TEXT NOT NULL,
		encrypted_value TEXT NOT NULL,
		created_at TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_pii_tokens_type ON pii_tokens(pii_type);
	CREATE INDEX IF NOT EXISTS idx_pii_tokens_created ON pii_tokens(created_at);
	`

	_, err := ts.db.Exec(schema)
	return err
}

// Store creates a token for a PII value
func (ts *TokenStore) Store(piiType PIIType, value string) string {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Check if value already has a token
	for token, entry := range ts.cache {
		if entry.Type == piiType && entry.Value == value {
			return token
		}
	}

	// Generate new token
	token := fmt.Sprintf("[PII:%s:%s]", strings.ToUpper(string(piiType)), uuid.New().String()[:8])

	entry := TokenEntry{
		Token:     token,
		Type:      piiType,
		Value:     value,
		CreatedAt: time.Now(),
	}

	ts.cache[token] = entry

	// Persist to database if available
	if ts.db != nil {
		ts.persistToken(entry)
	}

	return token
}

func (ts *TokenStore) persistToken(entry TokenEntry) {
	encryptedValue := entry.Value
	if ts.encryptor != nil {
		var err error
		encryptedValue, err = ts.encryptor.Encrypt(entry.Value)
		if err != nil {
			// Log error but continue with plaintext (in-memory only)
			return
		}
	}

	_, err := ts.db.Exec(`
		INSERT OR REPLACE INTO pii_tokens (token, pii_type, encrypted_value, created_at)
		VALUES (?, ?, ?, ?)
	`, entry.Token, entry.Type, encryptedValue, entry.CreatedAt.Format(time.RFC3339))

	if err != nil {
		// Log error
	}
}

// Retrieve gets the original value for a token
func (ts *TokenStore) Retrieve(token string) (PIIType, string, bool) {
	ts.mu.RLock()

	// Check cache first
	if entry, ok := ts.cache[token]; ok {
		ts.mu.RUnlock()
		return entry.Type, entry.Value, true
	}
	ts.mu.RUnlock()

	// Check database
	if ts.db != nil {
		var piiType string
		var encryptedValue string
		var createdAt string

		err := ts.db.QueryRow(`
			SELECT pii_type, encrypted_value, created_at
			FROM pii_tokens WHERE token = ?
		`, token).Scan(&piiType, &encryptedValue, &createdAt)

		if err == nil {
			value := encryptedValue
			if ts.encryptor != nil {
				decrypted, err := ts.encryptor.Decrypt(encryptedValue)
				if err == nil {
					value = decrypted
				}
			}

			ts.mu.Lock()
			ts.cache[token] = TokenEntry{
				Token: token,
				Type:  PIIType(piiType),
				Value: value,
			}
			ts.mu.Unlock()

			return PIIType(piiType), value, true
		}
	}

	return "", "", false
}

// DetokenizeText replaces all tokens in text with original values
func (ts *TokenStore) DetokenizeText(text string) string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	result := text
	for token, entry := range ts.cache {
		result = strings.ReplaceAll(result, token, entry.Value)
	}

	return result
}

// Count returns the number of stored tokens
func (ts *TokenStore) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return len(ts.cache)
}

// Clear removes all tokens
func (ts *TokenStore) Clear() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.cache = make(map[string]TokenEntry)

	if ts.db != nil {
		ts.db.Exec("DELETE FROM pii_tokens")
	}
}

// Cleanup removes tokens older than the specified duration
func (ts *TokenStore) Cleanup(maxAge time.Duration) int {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for token, entry := range ts.cache {
		if entry.CreatedAt.Before(cutoff) {
			delete(ts.cache, token)
			removed++
		}
	}

	if ts.db != nil {
		ts.db.Exec("DELETE FROM pii_tokens WHERE created_at < ?", cutoff.Format(time.RFC3339))
	}

	return removed
}

// Store is the main PII store for tracking detections and compliance
type Store struct {
	db       *sql.DB
	config   *Config
	redactor *Redactor
	mu       sync.RWMutex
}

// DetectionRecord represents a logged PII detection event
type DetectionRecord struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string    `json:"source"` // "log", "trace", "scan"
	SourceID    string    `json:"source_id"`
	PIIType     PIIType   `json:"pii_type"`
	Confidence  string    `json:"confidence"`
	Redacted    bool      `json:"redacted"`
	RedactedVal string    `json:"redacted_value,omitempty"`
}

// Stats represents PII detection statistics
type Stats struct {
	TotalDetections    int                  `json:"total_detections"`
	DetectionsByType   map[PIIType]int      `json:"detections_by_type"`
	DetectionsBySource map[string]int       `json:"detections_by_source"`
	LastDetection      time.Time            `json:"last_detection,omitempty"`
	Period             string               `json:"period"`
	HighConfidence     int                  `json:"high_confidence"`
	MediumConfidence   int                  `json:"medium_confidence"`
	LowConfidence      int                  `json:"low_confidence"`
}

// NewStore creates a new PII store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	store := &Store{
		db:     db,
		config: DefaultConfig(),
	}
	store.redactor = NewRedactor(store.config)

	if err := store.createTables(); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating tables: %w", err)
	}

	return store, nil
}

func (s *Store) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS pii_detections (
		id TEXT PRIMARY KEY,
		timestamp TEXT NOT NULL,
		source TEXT NOT NULL,
		source_id TEXT,
		pii_type TEXT NOT NULL,
		confidence TEXT NOT NULL,
		redacted INTEGER NOT NULL DEFAULT 1,
		redacted_value TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_pii_detections_timestamp ON pii_detections(timestamp);
	CREATE INDEX IF NOT EXISTS idx_pii_detections_type ON pii_detections(pii_type);
	CREATE INDEX IF NOT EXISTS idx_pii_detections_source ON pii_detections(source);

	CREATE TABLE IF NOT EXISTS pii_config (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		config_json TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	`

	_, err := s.db.Exec(schema)
	return err
}

// RecordDetection logs a PII detection event
func (s *Store) RecordDetection(source, sourceID string, detection Detection, redacted bool) error {
	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO pii_detections (id, timestamp, source, source_id, pii_type, confidence, redacted, redacted_value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		id,
		time.Now().UTC().Format(time.RFC3339),
		source,
		sourceID,
		detection.Type,
		detection.Confidence,
		redacted,
		detection.Redacted,
	)

	return err
}

// RecordDetections logs multiple PII detection events
func (s *Store) RecordDetections(source, sourceID string, detections []Detection, redacted bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO pii_detections (id, timestamp, source, source_id, pii_type, confidence, redacted, redacted_value)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range detections {
		_, err = stmt.Exec(
			uuid.New().String(),
			now,
			source,
			sourceID,
			d.Type,
			d.Confidence,
			redacted,
			d.Redacted,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetStats returns PII detection statistics
func (s *Store) GetStats(since time.Duration) (*Stats, error) {
	cutoff := time.Now().Add(-since).UTC().Format(time.RFC3339)

	stats := &Stats{
		DetectionsByType:   make(map[PIIType]int),
		DetectionsBySource: make(map[string]int),
		Period:             since.String(),
	}

	// Total and by type
	rows, err := s.db.Query(`
		SELECT pii_type, COUNT(*)
		FROM pii_detections
		WHERE timestamp >= ?
		GROUP BY pii_type
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var piiType string
		var count int
		if err := rows.Scan(&piiType, &count); err != nil {
			continue
		}
		stats.DetectionsByType[PIIType(piiType)] = count
		stats.TotalDetections += count
	}

	// By source
	rows, err = s.db.Query(`
		SELECT source, COUNT(*)
		FROM pii_detections
		WHERE timestamp >= ?
		GROUP BY source
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var source string
		var count int
		if err := rows.Scan(&source, &count); err != nil {
			continue
		}
		stats.DetectionsBySource[source] = count
	}

	// By confidence
	var high, medium, low int
	s.db.QueryRow(`SELECT COUNT(*) FROM pii_detections WHERE timestamp >= ? AND confidence = 'high'`, cutoff).Scan(&high)
	s.db.QueryRow(`SELECT COUNT(*) FROM pii_detections WHERE timestamp >= ? AND confidence = 'medium'`, cutoff).Scan(&medium)
	s.db.QueryRow(`SELECT COUNT(*) FROM pii_detections WHERE timestamp >= ? AND confidence = 'low'`, cutoff).Scan(&low)
	stats.HighConfidence = high
	stats.MediumConfidence = medium
	stats.LowConfidence = low

	// Last detection
	var lastDetection string
	if err := s.db.QueryRow(`SELECT MAX(timestamp) FROM pii_detections`).Scan(&lastDetection); err == nil {
		stats.LastDetection, _ = time.Parse(time.RFC3339, lastDetection)
	}

	return stats, nil
}

// GetConfig returns the current PII configuration
func (s *Store) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.config.Clone()
}

// UpdateConfig updates the PII configuration
func (s *Store) UpdateConfig(config *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	configJSON, err := config.ToJSON()
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO pii_config (id, config_json, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			config_json = excluded.config_json,
			updated_at = excluded.updated_at
	`, string(configJSON), time.Now().UTC().Format(time.RFC3339))

	if err != nil {
		return err
	}

	s.config = config
	s.redactor = NewRedactor(config)
	return nil
}

// LoadConfig loads configuration from the database
func (s *Store) LoadConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var configJSON string
	err := s.db.QueryRow(`SELECT config_json FROM pii_config WHERE id = 1`).Scan(&configJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // Use default config
		}
		return err
	}

	config, err := ConfigFromJSON([]byte(configJSON))
	if err != nil {
		return err
	}

	s.config = config
	s.redactor = NewRedactor(config)
	return nil
}

// GetRedactor returns the configured redactor
func (s *Store) GetRedactor() *Redactor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.redactor
}

// Cleanup removes old detection records
func (s *Store) Cleanup(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339)
	result, err := s.db.Exec("DELETE FROM pii_detections WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Encryptor handles AES encryption for token values
type Encryptor struct {
	block cipher.Block
}

// NewEncryptor creates a new encryptor with the given key
func NewEncryptor(key []byte) (*Encryptor, error) {
	// Key must be 16, 24, or 32 bytes
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("invalid key size: must be 16, 24, or 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return &Encryptor{block: block}, nil
}

// Encrypt encrypts a value using AES-GCM
func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	aead, err := cipher.NewGCM(e.block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a value using AES-GCM
func (e *Encryptor) Decrypt(encrypted string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	aead, err := cipher.NewGCM(e.block)
	if err != nil {
		return "", err
	}

	nonceSize := aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// ScanRequest represents a request to scan text for PII
type ScanRequest struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

// ScanResult represents the result of scanning text for PII
type ScanResult struct {
	HasPII     bool        `json:"has_pii"`
	Detections []Detection `json:"detections"`
	Redacted   string      `json:"redacted"`
	Stats      struct {
		Total      int            `json:"total"`
		ByType     map[string]int `json:"by_type"`
		Confidence struct {
			High   int `json:"high"`
			Medium int `json:"medium"`
			Low    int `json:"low"`
		} `json:"confidence"`
	} `json:"stats"`
}

// Scan scans text for PII and returns detailed results
func (s *Store) Scan(text string) *ScanResult {
	s.mu.RLock()
	redactor := s.redactor
	s.mu.RUnlock()

	redacted, detections := redactor.RedactWithDetails(text)

	result := &ScanResult{
		HasPII:     len(detections) > 0,
		Detections: detections,
		Redacted:   redacted,
	}

	result.Stats.Total = len(detections)
	result.Stats.ByType = make(map[string]int)

	for _, d := range detections {
		result.Stats.ByType[string(d.Type)]++
		switch d.Confidence {
		case ConfidenceHigh:
			result.Stats.Confidence.High++
		case ConfidenceMedium:
			result.Stats.Confidence.Medium++
		case ConfidenceLow:
			result.Stats.Confidence.Low++
		}
	}

	return result
}
