package sso

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Store provides SSO data persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// LinkedAccount represents an OAuth/SAML linked account
type LinkedAccount struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	OrgID        string    `json:"org_id"`
	Provider     string    `json:"provider"`     // google, github, microsoft, saml
	ProviderID   string    `json:"provider_id"`  // Provider's user ID
	Email        string    `json:"email"`
	AccessToken  string    `json:"-"`            // Encrypted
	RefreshToken string    `json:"-"`            // Encrypted
	TokenExpiry  *time.Time `json:"token_expiry,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OrgSSOConfig stores SSO configuration for an organization
type OrgSSOConfig struct {
	OrgID           string          `json:"org_id"`
	SAMLEnabled     bool            `json:"saml_enabled"`
	SAMLConfig      *SAMLConfig     `json:"saml_config,omitempty"`
	OAuthProviders  []string        `json:"oauth_providers"`       // Enabled OAuth providers
	DefaultRole     string          `json:"default_role"`          // Role for new SSO users
	AutoProvision   bool            `json:"auto_provision"`        // Auto-create users
	RequireSSO      bool            `json:"require_sso"`           // Disable password login
	AllowedDomains  []string        `json:"allowed_domains"`       // Email domain restrictions
	UpdatedAt       time.Time       `json:"updated_at"`
}

// NewStore creates a new SSO store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
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

func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS linked_accounts (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		org_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		email TEXT NOT NULL,
		access_token TEXT,
		refresh_token TEXT,
		token_expiry INTEGER,
		metadata TEXT,
		created_at INTEGER,
		updated_at INTEGER,
		UNIQUE(provider, provider_id),
		UNIQUE(user_id, provider)
	);

	CREATE TABLE IF NOT EXISTS sso_configs (
		org_id TEXT PRIMARY KEY,
		saml_enabled INTEGER DEFAULT 0,
		saml_config TEXT,
		oauth_providers TEXT,
		default_role TEXT DEFAULT 'viewer',
		auto_provision INTEGER DEFAULT 0,
		require_sso INTEGER DEFAULT 0,
		allowed_domains TEXT,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS sso_sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		org_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		access_token TEXT,
		refresh_token TEXT,
		id_token TEXT,
		token_expiry INTEGER,
		created_at INTEGER,
		expires_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_linked_user ON linked_accounts(user_id);
	CREATE INDEX IF NOT EXISTS idx_linked_provider ON linked_accounts(provider, provider_id);
	CREATE INDEX IF NOT EXISTS idx_linked_email ON linked_accounts(email);
	CREATE INDEX IF NOT EXISTS idx_sso_sessions_user ON sso_sessions(user_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateLinkedAccount creates a new linked account
func (s *Store) CreateLinkedAccount(account *LinkedAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	account.CreatedAt = now
	account.UpdatedAt = now

	metadata, _ := json.Marshal(account.Metadata)

	var tokenExpiry *int64
	if account.TokenExpiry != nil {
		t := account.TokenExpiry.Unix()
		tokenExpiry = &t
	}

	_, err := s.db.Exec(`
		INSERT INTO linked_accounts (id, user_id, org_id, provider, provider_id, email,
			access_token, refresh_token, token_expiry, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		account.ID, account.UserID, account.OrgID, account.Provider, account.ProviderID,
		account.Email, account.AccessToken, account.RefreshToken, tokenExpiry,
		string(metadata), now.Unix(), now.Unix())

	return err
}

// GetLinkedAccount gets a linked account by provider and provider ID
func (s *Store) GetLinkedAccount(provider, providerID string) (*LinkedAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var account LinkedAccount
	var metadata string
	var tokenExpiry sql.NullInt64
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, user_id, org_id, provider, provider_id, email,
			access_token, refresh_token, token_expiry, metadata, created_at, updated_at
		FROM linked_accounts WHERE provider = ? AND provider_id = ?`,
		provider, providerID).Scan(
		&account.ID, &account.UserID, &account.OrgID, &account.Provider, &account.ProviderID,
		&account.Email, &account.AccessToken, &account.RefreshToken, &tokenExpiry,
		&metadata, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(metadata), &account.Metadata)
	account.CreatedAt = time.Unix(createdAt, 0)
	account.UpdatedAt = time.Unix(updatedAt, 0)
	if tokenExpiry.Valid {
		t := time.Unix(tokenExpiry.Int64, 0)
		account.TokenExpiry = &t
	}

	return &account, nil
}

// GetLinkedAccountByEmail gets a linked account by email and provider
func (s *Store) GetLinkedAccountByEmail(email, provider string) (*LinkedAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var account LinkedAccount
	var metadata string
	var tokenExpiry sql.NullInt64
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, user_id, org_id, provider, provider_id, email,
			access_token, refresh_token, token_expiry, metadata, created_at, updated_at
		FROM linked_accounts WHERE email = ? AND provider = ?`,
		email, provider).Scan(
		&account.ID, &account.UserID, &account.OrgID, &account.Provider, &account.ProviderID,
		&account.Email, &account.AccessToken, &account.RefreshToken, &tokenExpiry,
		&metadata, &createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(metadata), &account.Metadata)
	account.CreatedAt = time.Unix(createdAt, 0)
	account.UpdatedAt = time.Unix(updatedAt, 0)
	if tokenExpiry.Valid {
		t := time.Unix(tokenExpiry.Int64, 0)
		account.TokenExpiry = &t
	}

	return &account, nil
}

// GetUserLinkedAccounts gets all linked accounts for a user
func (s *Store) GetUserLinkedAccounts(userID string) ([]LinkedAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, user_id, org_id, provider, provider_id, email,
			access_token, refresh_token, token_expiry, metadata, created_at, updated_at
		FROM linked_accounts WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []LinkedAccount
	for rows.Next() {
		var account LinkedAccount
		var metadata string
		var tokenExpiry sql.NullInt64
		var createdAt, updatedAt int64

		if err := rows.Scan(&account.ID, &account.UserID, &account.OrgID, &account.Provider,
			&account.ProviderID, &account.Email, &account.AccessToken, &account.RefreshToken,
			&tokenExpiry, &metadata, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(metadata), &account.Metadata)
		account.CreatedAt = time.Unix(createdAt, 0)
		account.UpdatedAt = time.Unix(updatedAt, 0)
		if tokenExpiry.Valid {
			t := time.Unix(tokenExpiry.Int64, 0)
			account.TokenExpiry = &t
		}

		accounts = append(accounts, account)
	}

	return accounts, nil
}

// UpdateLinkedAccountTokens updates OAuth tokens for a linked account
func (s *Store) UpdateLinkedAccountTokens(id, accessToken, refreshToken string, expiry *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var tokenExpiry *int64
	if expiry != nil {
		t := expiry.Unix()
		tokenExpiry = &t
	}

	_, err := s.db.Exec(`
		UPDATE linked_accounts
		SET access_token = ?, refresh_token = ?, token_expiry = ?, updated_at = ?
		WHERE id = ?`,
		accessToken, refreshToken, tokenExpiry, time.Now().Unix(), id)

	return err
}

// DeleteLinkedAccount removes a linked account
func (s *Store) DeleteLinkedAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM linked_accounts WHERE id = ?", id)
	return err
}

// GetOrgSSOConfig gets SSO configuration for an organization
func (s *Store) GetOrgSSOConfig(orgID string) (*OrgSSOConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var config OrgSSOConfig
	var samlConfig, oauthProviders, allowedDomains string
	var updatedAt int64

	err := s.db.QueryRow(`
		SELECT org_id, saml_enabled, saml_config, oauth_providers, default_role,
			auto_provision, require_sso, allowed_domains, updated_at
		FROM sso_configs WHERE org_id = ?`, orgID).Scan(
		&config.OrgID, &config.SAMLEnabled, &samlConfig, &oauthProviders,
		&config.DefaultRole, &config.AutoProvision, &config.RequireSSO,
		&allowedDomains, &updatedAt)

	if err == sql.ErrNoRows {
		// Return default config
		return &OrgSSOConfig{
			OrgID:          orgID,
			OAuthProviders: []string{},
			DefaultRole:    "viewer",
		}, nil
	}
	if err != nil {
		return nil, err
	}

	if samlConfig != "" {
		json.Unmarshal([]byte(samlConfig), &config.SAMLConfig)
	}
	json.Unmarshal([]byte(oauthProviders), &config.OAuthProviders)
	json.Unmarshal([]byte(allowedDomains), &config.AllowedDomains)
	config.UpdatedAt = time.Unix(updatedAt, 0)

	return &config, nil
}

// SaveOrgSSOConfig saves SSO configuration for an organization
func (s *Store) SaveOrgSSOConfig(config *OrgSSOConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	config.UpdatedAt = time.Now()

	samlConfig, _ := json.Marshal(config.SAMLConfig)
	oauthProviders, _ := json.Marshal(config.OAuthProviders)
	allowedDomains, _ := json.Marshal(config.AllowedDomains)

	_, err := s.db.Exec(`
		INSERT INTO sso_configs (org_id, saml_enabled, saml_config, oauth_providers,
			default_role, auto_provision, require_sso, allowed_domains, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(org_id) DO UPDATE SET
			saml_enabled = ?, saml_config = ?, oauth_providers = ?,
			default_role = ?, auto_provision = ?, require_sso = ?,
			allowed_domains = ?, updated_at = ?`,
		config.OrgID, config.SAMLEnabled, string(samlConfig), string(oauthProviders),
		config.DefaultRole, config.AutoProvision, config.RequireSSO,
		string(allowedDomains), config.UpdatedAt.Unix(),
		config.SAMLEnabled, string(samlConfig), string(oauthProviders),
		config.DefaultRole, config.AutoProvision, config.RequireSSO,
		string(allowedDomains), config.UpdatedAt.Unix())

	return err
}

// SSOSession represents an active SSO session
type SSOSession struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	OrgID        string    `json:"org_id"`
	Provider     string    `json:"provider"`
	AccessToken  string    `json:"-"`
	RefreshToken string    `json:"-"`
	IDToken      string    `json:"-"`
	TokenExpiry  *time.Time `json:"token_expiry,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// CreateSSOSession creates a new SSO session
func (s *Store) CreateSSOSession(session *SSOSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.CreatedAt = time.Now()

	var tokenExpiry *int64
	if session.TokenExpiry != nil {
		t := session.TokenExpiry.Unix()
		tokenExpiry = &t
	}

	_, err := s.db.Exec(`
		INSERT INTO sso_sessions (id, user_id, org_id, provider, access_token,
			refresh_token, id_token, token_expiry, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.OrgID, session.Provider,
		session.AccessToken, session.RefreshToken, session.IDToken,
		tokenExpiry, session.CreatedAt.Unix(), session.ExpiresAt.Unix())

	return err
}

// GetSSOSession gets an SSO session by ID
func (s *Store) GetSSOSession(id string) (*SSOSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var session SSOSession
	var tokenExpiry sql.NullInt64
	var createdAt, expiresAt int64

	err := s.db.QueryRow(`
		SELECT id, user_id, org_id, provider, access_token, refresh_token,
			id_token, token_expiry, created_at, expires_at
		FROM sso_sessions WHERE id = ?`, id).Scan(
		&session.ID, &session.UserID, &session.OrgID, &session.Provider,
		&session.AccessToken, &session.RefreshToken, &session.IDToken,
		&tokenExpiry, &createdAt, &expiresAt)

	if err != nil {
		return nil, err
	}

	session.CreatedAt = time.Unix(createdAt, 0)
	session.ExpiresAt = time.Unix(expiresAt, 0)
	if tokenExpiry.Valid {
		t := time.Unix(tokenExpiry.Int64, 0)
		session.TokenExpiry = &t
	}

	return &session, nil
}

// DeleteSSOSession removes an SSO session
func (s *Store) DeleteSSOSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sso_sessions WHERE id = ?", id)
	return err
}

// DeleteUserSSOSessions removes all SSO sessions for a user
func (s *Store) DeleteUserSSOSessions(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sso_sessions WHERE user_id = ?", userID)
	return err
}

// CleanupExpiredSessions removes expired SSO sessions
func (s *Store) CleanupExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sso_sessions WHERE expires_at < ?", time.Now().Unix())
	return err
}
