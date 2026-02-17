package rbac

import (
	"database/sql"
	"dogwatch/internal/storage"
	"encoding/json"
	"fmt"
	"sync"
	"time"

)

// Store manages RBAC data persistence
type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewStore creates a new RBAC store
func NewStore(dbPath string) (*Store, error) {
	db, err := storage.OpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.init(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) init() error {
	schema := `
	CREATE TABLE IF NOT EXISTS organizations (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		slug TEXT UNIQUE NOT NULL,
		plan TEXT DEFAULT 'free',
		settings TEXT,
		created_at INTEGER,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		email TEXT NOT NULL,
		password_hash TEXT NOT NULL,
		name TEXT,
		role TEXT NOT NULL,
		team_ids TEXT,
		avatar_url TEXT,
		timezone TEXT,
		is_active INTEGER DEFAULT 1,
		last_login_at INTEGER,
		created_at INTEGER,
		updated_at INTEGER,
		UNIQUE(org_id, email),
		FOREIGN KEY (org_id) REFERENCES organizations(id)
	);

	CREATE TABLE IF NOT EXISTS teams (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		member_ids TEXT,
		created_at INTEGER,
		updated_at INTEGER,
		FOREIGN KEY (org_id) REFERENCES organizations(id)
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		org_id TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		user_agent TEXT,
		ip_address TEXT,
		expires_at INTEGER NOT NULL,
		created_at INTEGER,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		key_prefix TEXT NOT NULL,
		key_hash TEXT NOT NULL,
		permissions TEXT,
		last_used_at INTEGER,
		expires_at INTEGER,
		is_active INTEGER DEFAULT 1,
		created_at INTEGER,
		FOREIGN KEY (org_id) REFERENCES organizations(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS invites (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		email TEXT NOT NULL,
		role TEXT NOT NULL,
		token_hash TEXT NOT NULL,
		invited_by TEXT NOT NULL,
		expires_at INTEGER NOT NULL,
		created_at INTEGER,
		FOREIGN KEY (org_id) REFERENCES organizations(id)
	);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		org_id TEXT NOT NULL,
		user_id TEXT,
		action TEXT NOT NULL,
		resource TEXT,
		resource_id TEXT,
		details TEXT,
		ip_address TEXT,
		user_agent TEXT,
		created_at INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token_hash);
	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
	CREATE INDEX IF NOT EXISTS idx_audit_org ON audit_logs(org_id);
	CREATE INDEX IF NOT EXISTS idx_audit_user ON audit_logs(user_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Organization methods

func (s *Store) CreateOrganization(org *Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	org.CreatedAt = now
	org.UpdatedAt = now

	settings, _ := json.Marshal(org.Settings)

	_, err := s.db.Exec(`
		INSERT INTO organizations (id, name, slug, plan, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		org.ID, org.Name, org.Slug, org.Plan, string(settings),
		now.Unix(), now.Unix())

	return err
}

func (s *Store) GetOrganization(id string) (*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var org Organization
	var settings string
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, name, slug, plan, settings, created_at, updated_at
		FROM organizations WHERE id = ?`, id).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Plan, &settings,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(settings), &org.Settings)
	org.CreatedAt = time.Unix(createdAt, 0)
	org.UpdatedAt = time.Unix(updatedAt, 0)

	return &org, nil
}

func (s *Store) GetOrganizationBySlug(slug string) (*Organization, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var org Organization
	var settings string
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, name, slug, plan, settings, created_at, updated_at
		FROM organizations WHERE slug = ?`, slug).Scan(
		&org.ID, &org.Name, &org.Slug, &org.Plan, &settings,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(settings), &org.Settings)
	org.CreatedAt = time.Unix(createdAt, 0)
	org.UpdatedAt = time.Unix(updatedAt, 0)

	return &org, nil
}

func (s *Store) UpdateOrganization(org *Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	org.UpdatedAt = time.Now()
	settings, _ := json.Marshal(org.Settings)

	_, err := s.db.Exec(`
		UPDATE organizations SET name=?, slug=?, plan=?, settings=?, updated_at=?
		WHERE id=?`,
		org.Name, org.Slug, org.Plan, string(settings),
		org.UpdatedAt.Unix(), org.ID)

	return err
}

// User methods

func (s *Store) CreateUser(user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	teamIDs, _ := json.Marshal(user.TeamIDs)

	_, err := s.db.Exec(`
		INSERT INTO users (id, org_id, email, password_hash, name, role, team_ids,
		                   avatar_url, timezone, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.OrgID, user.Email, user.PasswordHash, user.Name, user.Role,
		string(teamIDs), user.AvatarURL, user.Timezone, user.IsActive,
		now.Unix(), now.Unix())

	return err
}

func (s *Store) GetUser(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getUser(id)
}

func (s *Store) getUser(id string) (*User, error) {
	var user User
	var teamIDs string
	var createdAt, updatedAt int64
	var lastLoginAt sql.NullInt64
	var avatarURL, timezone sql.NullString

	err := s.db.QueryRow(`
		SELECT id, org_id, email, password_hash, name, role, team_ids,
		       avatar_url, timezone, is_active, last_login_at, created_at, updated_at
		FROM users WHERE id = ?`, id).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.PasswordHash, &user.Name, &user.Role,
		&teamIDs, &avatarURL, &timezone, &user.IsActive, &lastLoginAt,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(teamIDs), &user.TeamIDs)
	user.CreatedAt = time.Unix(createdAt, 0)
	user.UpdatedAt = time.Unix(updatedAt, 0)
	if lastLoginAt.Valid {
		t := time.Unix(lastLoginAt.Int64, 0)
		user.LastLoginAt = &t
	}
	if avatarURL.Valid {
		user.AvatarURL = avatarURL.String
	}
	if timezone.Valid {
		user.Timezone = timezone.String
	}

	return &user, nil
}

func (s *Store) GetUserByEmail(orgID, email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var id string
	err := s.db.QueryRow("SELECT id FROM users WHERE org_id = ? AND email = ?",
		orgID, email).Scan(&id)
	if err != nil {
		return nil, err
	}

	return s.getUser(id)
}

func (s *Store) GetUserByEmailAnyOrg(email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var id string
	err := s.db.QueryRow("SELECT id FROM users WHERE email = ? LIMIT 1", email).Scan(&id)
	if err != nil {
		return nil, err
	}

	return s.getUser(id)
}

func (s *Store) UpdateUser(user *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user.UpdatedAt = time.Now()
	teamIDs, _ := json.Marshal(user.TeamIDs)

	var lastLoginAt *int64
	if user.LastLoginAt != nil {
		t := user.LastLoginAt.Unix()
		lastLoginAt = &t
	}

	_, err := s.db.Exec(`
		UPDATE users SET email=?, name=?, role=?, team_ids=?, avatar_url=?,
		                 timezone=?, is_active=?, last_login_at=?, updated_at=?
		WHERE id=?`,
		user.Email, user.Name, user.Role, string(teamIDs), user.AvatarURL,
		user.Timezone, user.IsActive, lastLoginAt, user.UpdatedAt.Unix(), user.ID)

	return err
}

func (s *Store) UpdateUserPassword(userID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE users SET password_hash=?, updated_at=? WHERE id=?",
		passwordHash, time.Now().Unix(), userID)
	return err
}

func (s *Store) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete sessions first
	s.db.Exec("DELETE FROM sessions WHERE user_id = ?", id)
	// Delete API keys
	s.db.Exec("DELETE FROM api_keys WHERE user_id = ?", id)
	// Delete user
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *Store) ListUsers(orgID string) ([]User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id FROM users WHERE org_id = ? ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		user, err := s.getUser(id)
		if err != nil {
			continue
		}
		users = append(users, *user)
	}

	return users, nil
}

// Session methods

func (s *Store) CreateSession(session *Session, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.CreatedAt = time.Now()

	_, err := s.db.Exec(`
		INSERT INTO sessions (id, user_id, org_id, token_hash, user_agent, ip_address, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, session.OrgID, tokenHash, session.UserAgent,
		session.IPAddress, session.ExpiresAt.Unix(), session.CreatedAt.Unix())

	return err
}

func (s *Store) GetSessionByToken(tokenHash string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var session Session
	var expiresAt, createdAt int64

	err := s.db.QueryRow(`
		SELECT id, user_id, org_id, user_agent, ip_address, expires_at, created_at
		FROM sessions WHERE token_hash = ?`, tokenHash).Scan(
		&session.ID, &session.UserID, &session.OrgID, &session.UserAgent,
		&session.IPAddress, &expiresAt, &createdAt)

	if err != nil {
		return nil, err
	}

	session.ExpiresAt = time.Unix(expiresAt, 0)
	session.CreatedAt = time.Unix(createdAt, 0)

	return &session, nil
}

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	return err
}

func (s *Store) DeleteUserSessions(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

func (s *Store) CleanupExpiredSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now().Unix())
	return err
}

// API Key methods

func (s *Store) CreateAPIKey(key *APIKey, keyHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key.CreatedAt = time.Now()
	permissions, _ := json.Marshal(key.Permissions)

	var expiresAt *int64
	if key.ExpiresAt != nil {
		t := key.ExpiresAt.Unix()
		expiresAt = &t
	}

	_, err := s.db.Exec(`
		INSERT INTO api_keys (id, org_id, user_id, name, key_prefix, key_hash,
		                      permissions, expires_at, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ID, key.OrgID, key.UserID, key.Name, key.KeyPrefix, keyHash,
		string(permissions), expiresAt, key.IsActive, key.CreatedAt.Unix())

	return err
}

func (s *Store) GetAPIKeyByHash(keyHash string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var key APIKey
	var permissions string
	var createdAt int64
	var lastUsedAt, expiresAt sql.NullInt64

	err := s.db.QueryRow(`
		SELECT id, org_id, user_id, name, key_prefix, permissions,
		       last_used_at, expires_at, is_active, created_at
		FROM api_keys WHERE key_hash = ?`, keyHash).Scan(
		&key.ID, &key.OrgID, &key.UserID, &key.Name, &key.KeyPrefix,
		&permissions, &lastUsedAt, &expiresAt, &key.IsActive, &createdAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(permissions), &key.Permissions)
	key.CreatedAt = time.Unix(createdAt, 0)
	if lastUsedAt.Valid {
		t := time.Unix(lastUsedAt.Int64, 0)
		key.LastUsedAt = &t
	}
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		key.ExpiresAt = &t
	}

	return &key, nil
}

func (s *Store) UpdateAPIKeyLastUsed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?",
		time.Now().Unix(), id)
	return err
}

func (s *Store) DeleteAPIKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	return err
}

func (s *Store) ListAPIKeys(orgID string) ([]APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, user_id, name, key_prefix, permissions,
		       last_used_at, expires_at, is_active, created_at
		FROM api_keys WHERE org_id = ? ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var key APIKey
		var permissions string
		var createdAt int64
		var lastUsedAt, expiresAt sql.NullInt64

		if err := rows.Scan(&key.ID, &key.OrgID, &key.UserID, &key.Name, &key.KeyPrefix,
			&permissions, &lastUsedAt, &expiresAt, &key.IsActive, &createdAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(permissions), &key.Permissions)
		key.CreatedAt = time.Unix(createdAt, 0)
		if lastUsedAt.Valid {
			t := time.Unix(lastUsedAt.Int64, 0)
			key.LastUsedAt = &t
		}
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			key.ExpiresAt = &t
		}

		keys = append(keys, key)
	}

	return keys, nil
}

// Team methods

func (s *Store) CreateTeam(team *Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	team.CreatedAt = now
	team.UpdatedAt = now

	memberIDs, _ := json.Marshal(team.MemberIDs)

	_, err := s.db.Exec(`
		INSERT INTO teams (id, org_id, name, description, member_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		team.ID, team.OrgID, team.Name, team.Description, string(memberIDs),
		now.Unix(), now.Unix())

	return err
}

func (s *Store) GetTeam(id string) (*Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var team Team
	var memberIDs string
	var createdAt, updatedAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, name, description, member_ids, created_at, updated_at
		FROM teams WHERE id = ?`, id).Scan(
		&team.ID, &team.OrgID, &team.Name, &team.Description, &memberIDs,
		&createdAt, &updatedAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(memberIDs), &team.MemberIDs)
	team.CreatedAt = time.Unix(createdAt, 0)
	team.UpdatedAt = time.Unix(updatedAt, 0)

	return &team, nil
}

func (s *Store) UpdateTeam(team *Team) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	team.UpdatedAt = time.Now()
	memberIDs, _ := json.Marshal(team.MemberIDs)

	_, err := s.db.Exec(`
		UPDATE teams SET name=?, description=?, member_ids=?, updated_at=?
		WHERE id=?`,
		team.Name, team.Description, string(memberIDs), team.UpdatedAt.Unix(), team.ID)

	return err
}

func (s *Store) DeleteTeam(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM teams WHERE id = ?", id)
	return err
}

func (s *Store) ListTeams(orgID string) ([]Team, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, name, description, member_ids, created_at, updated_at
		FROM teams WHERE org_id = ? ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []Team
	for rows.Next() {
		var team Team
		var memberIDs string
		var createdAt, updatedAt int64

		if err := rows.Scan(&team.ID, &team.OrgID, &team.Name, &team.Description,
			&memberIDs, &createdAt, &updatedAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(memberIDs), &team.MemberIDs)
		team.CreatedAt = time.Unix(createdAt, 0)
		team.UpdatedAt = time.Unix(updatedAt, 0)

		teams = append(teams, team)
	}

	return teams, nil
}

// Audit log methods

func (s *Store) CreateAuditLog(log *AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.CreatedAt = time.Now()
	details, _ := json.Marshal(log.Details)

	_, err := s.db.Exec(`
		INSERT INTO audit_logs (id, org_id, user_id, action, resource, resource_id,
		                        details, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.ID, log.OrgID, log.UserID, log.Action, log.Resource, log.ResourceID,
		string(details), log.IPAddress, log.UserAgent, log.CreatedAt.Unix())

	return err
}

func (s *Store) ListAuditLogs(orgID string, limit int) ([]AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, org_id, user_id, action, resource, resource_id,
		       details, ip_address, user_agent, created_at
		FROM audit_logs WHERE org_id = ? ORDER BY created_at DESC LIMIT ?`,
		orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var log AuditLog
		var details string
		var createdAt int64
		var userID, resource, resourceID, ipAddress, userAgent sql.NullString

		if err := rows.Scan(&log.ID, &log.OrgID, &userID, &log.Action, &resource,
			&resourceID, &details, &ipAddress, &userAgent, &createdAt); err != nil {
			continue
		}

		json.Unmarshal([]byte(details), &log.Details)
		log.CreatedAt = time.Unix(createdAt, 0)
		if userID.Valid {
			log.UserID = userID.String
		}
		if resource.Valid {
			log.Resource = resource.String
		}
		if resourceID.Valid {
			log.ResourceID = resourceID.String
		}
		if ipAddress.Valid {
			log.IPAddress = ipAddress.String
		}
		if userAgent.Valid {
			log.UserAgent = userAgent.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// Invite methods

func (s *Store) CreateInvite(invite *Invite, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	invite.CreatedAt = time.Now()

	_, err := s.db.Exec(`
		INSERT INTO invites (id, org_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		invite.ID, invite.OrgID, invite.Email, invite.Role, tokenHash,
		invite.InvitedBy, invite.ExpiresAt.Unix(), invite.CreatedAt.Unix())

	return err
}

func (s *Store) GetInviteByToken(tokenHash string) (*Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var invite Invite
	var expiresAt, createdAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, email, role, invited_by, expires_at, created_at
		FROM invites WHERE token_hash = ?`, tokenHash).Scan(
		&invite.ID, &invite.OrgID, &invite.Email, &invite.Role,
		&invite.InvitedBy, &expiresAt, &createdAt)

	if err != nil {
		return nil, err
	}

	invite.ExpiresAt = time.Unix(expiresAt, 0)
	invite.CreatedAt = time.Unix(createdAt, 0)

	return &invite, nil
}

func (s *Store) DeleteInvite(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM invites WHERE id = ?", id)
	return err
}

func (s *Store) ListPendingInvites(orgID string) ([]Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, email, role, invited_by, expires_at, created_at
		FROM invites WHERE org_id = ? AND expires_at > ? ORDER BY created_at DESC`,
		orgID, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var invite Invite
		var expiresAt, createdAt int64

		if err := rows.Scan(&invite.ID, &invite.OrgID, &invite.Email, &invite.Role,
			&invite.InvitedBy, &expiresAt, &createdAt); err != nil {
			continue
		}

		invite.ExpiresAt = time.Unix(expiresAt, 0)
		invite.CreatedAt = time.Unix(createdAt, 0)

		invites = append(invites, invite)
	}

	return invites, nil
}

// Stats

func (s *Store) GetOrgStats(orgID string) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]int)

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM users WHERE org_id = ?", orgID).Scan(&count)
	stats["users"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM teams WHERE org_id = ?", orgID).Scan(&count)
	stats["teams"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM api_keys WHERE org_id = ? AND is_active = 1", orgID).Scan(&count)
	stats["api_keys"] = count

	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE org_id = ? AND expires_at > ?",
		orgID, time.Now().Unix()).Scan(&count)
	stats["active_sessions"] = count

	return stats, nil
}

// EnsureDefaultOrg creates a default organization if none exists
func (s *Store) EnsureDefaultOrg(slug, name string) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if any org exists
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM organizations").Scan(&count)
	if count > 0 {
		// Return first org
		var id string
		s.db.QueryRow("SELECT id FROM organizations LIMIT 1").Scan(&id)
		s.mu.Unlock()
		org, err := s.GetOrganization(id)
		s.mu.Lock()
		return org, err
	}

	// Create default org
	org := &Organization{
		ID:       fmt.Sprintf("org_%d", time.Now().UnixNano()),
		Name:     name,
		Slug:     slug,
		Plan:     "free",
		Settings: make(map[string]string),
	}

	now := time.Now()
	org.CreatedAt = now
	org.UpdatedAt = now

	settings, _ := json.Marshal(org.Settings)

	_, err := s.db.Exec(`
		INSERT INTO organizations (id, name, slug, plan, settings, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		org.ID, org.Name, org.Slug, org.Plan, string(settings),
		now.Unix(), now.Unix())

	if err != nil {
		return nil, err
	}

	return org, nil
}

// GetAPIKey gets an API key by ID
func (s *Store) GetAPIKey(id string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var key APIKey
	var permissionsJSON string
	var expiresAt, lastUsedAt, createdAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, user_id, name, key_prefix, permissions,
			   expires_at, last_used_at, is_active, created_at
		FROM api_keys WHERE id = ?`, id).Scan(
		&key.ID, &key.OrgID, &key.UserID, &key.Name, &key.KeyPrefix,
		&permissionsJSON, &expiresAt, &lastUsedAt, &key.IsActive, &createdAt)

	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(permissionsJSON), &key.Permissions)
	key.CreatedAt = time.Unix(createdAt, 0)

	if expiresAt > 0 {
		t := time.Unix(expiresAt, 0)
		key.ExpiresAt = &t
	}
	if lastUsedAt > 0 {
		t := time.Unix(lastUsedAt, 0)
		key.LastUsedAt = &t
	}

	return &key, nil
}

// ListInvites lists all invites for an organization (including expired)
func (s *Store) ListInvites(orgID string) ([]Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, org_id, email, role, invited_by, expires_at, created_at
		FROM invites WHERE org_id = ?
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var inv Invite
		var expiresAt, createdAt int64

		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role,
			&inv.InvitedBy, &expiresAt, &createdAt); err != nil {
			continue
		}

		inv.ExpiresAt = time.Unix(expiresAt, 0)
		inv.CreatedAt = time.Unix(createdAt, 0)
		invites = append(invites, inv)
	}

	return invites, nil
}

// GetInvite gets an invite by ID
func (s *Store) GetInvite(id string) (*Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var inv Invite
	var expiresAt, createdAt int64

	err := s.db.QueryRow(`
		SELECT id, org_id, email, role, invited_by, expires_at, created_at
		FROM invites WHERE id = ?`, id).Scan(
		&inv.ID, &inv.OrgID, &inv.Email, &inv.Role,
		&inv.InvitedBy, &expiresAt, &createdAt)

	if err != nil {
		return nil, err
	}

	inv.ExpiresAt = time.Unix(expiresAt, 0)
	inv.CreatedAt = time.Unix(createdAt, 0)

	return &inv, nil
}

// ListUserSessions lists all sessions for a user
func (s *Store) ListUserSessions(userID string) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, user_id, org_id, user_agent, ip_address, expires_at, created_at
		FROM sessions WHERE user_id = ? AND expires_at > ?
		ORDER BY created_at DESC`, userID, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var expiresAt, createdAt int64

		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.OrgID,
			&sess.UserAgent, &sess.IPAddress, &expiresAt, &createdAt); err != nil {
			continue
		}

		sess.ExpiresAt = time.Unix(expiresAt, 0)
		sess.CreatedAt = time.Unix(createdAt, 0)
		sessions = append(sessions, sess)
	}

	return sessions, nil
}
