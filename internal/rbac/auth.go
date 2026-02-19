package rbac

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotActive      = errors.New("user account is not active")
	ErrSessionExpired     = errors.New("session has expired")
	ErrInvalidToken       = errors.New("invalid token")
	ErrAPIKeyExpired      = errors.New("API key has expired")
	ErrAPIKeyInactive     = errors.New("API key is not active")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrInvalidInvite      = errors.New("invalid or expired invitation")
)

const (
	defaultSessionExpiry = 7 * 24 * time.Hour // sessions last 7 days
	defaultBcryptCost    = 12                 // bcrypt work factor (OWASP minimum)
)

// Auth handles authentication operations
type Auth struct {
	store         *Store
	sessionExpiry time.Duration
	bcryptCost    int
}

func NewAuth(store *Store) *Auth {
	return &Auth{
		store:         store,
		sessionExpiry: defaultSessionExpiry,
		bcryptCost:    defaultBcryptCost,
	}
}

// SetSessionExpiry sets the session expiry duration
func (a *Auth) SetSessionExpiry(d time.Duration) {
	a.sessionExpiry = d
}

// HashPassword creates a bcrypt hash of a password
func (a *Auth) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), a.bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies a password against a hash
func (a *Auth) CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateToken generates a random token
func (a *Auth) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HashToken creates a SHA256 hash of a token for storage
func (a *Auth) HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GenerateAPIKey generates a new API key with prefix
func (a *Auth) GenerateAPIKey() (key, prefix string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", err
	}
	key = "dw_" + base64.URLEncoding.EncodeToString(bytes)
	prefix = key[:11] // "dw_" + first 8 chars
	return key, prefix, nil
}

// Login authenticates a user and creates a session
func (a *Auth) Login(orgID, email, password, userAgent, ipAddress string) (*LoginResponse, error) {
	// Get user
	user, err := a.store.GetUserByEmail(orgID, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Check password
	if !a.CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check if active
	if !user.IsActive {
		return nil, ErrUserNotActive
	}

	// Generate session token
	token, err := a.GenerateToken()
	if err != nil {
		return nil, err
	}

	// Create session
	session := &Session{
		ID:        fmt.Sprintf("sess_%d", time.Now().UnixNano()),
		UserID:    user.ID,
		OrgID:     user.OrgID,
		Token:     token,
		UserAgent: userAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(a.sessionExpiry),
	}

	if err := a.store.CreateSession(session, a.HashToken(token)); err != nil {
		return nil, err
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	a.store.UpdateUser(user)

	return &LoginResponse{
		Token:     token,
		ExpiresAt: session.ExpiresAt,
		User:      *user,
	}, nil
}

// LoginAnyOrg authenticates a user across all organizations
func (a *Auth) LoginAnyOrg(email, password, userAgent, ipAddress string) (*LoginResponse, error) {
	// Get user by email from any org
	user, err := a.store.GetUserByEmailAnyOrg(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Check password
	if !a.CheckPassword(password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Check if active
	if !user.IsActive {
		return nil, ErrUserNotActive
	}

	// Generate session token
	token, err := a.GenerateToken()
	if err != nil {
		return nil, err
	}

	// Create session
	session := &Session{
		ID:        fmt.Sprintf("sess_%d", time.Now().UnixNano()),
		UserID:    user.ID,
		OrgID:     user.OrgID,
		Token:     token,
		UserAgent: userAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(a.sessionExpiry),
	}

	if err := a.store.CreateSession(session, a.HashToken(token)); err != nil {
		return nil, err
	}

	// Update last login
	now := time.Now()
	user.LastLoginAt = &now
	a.store.UpdateUser(user)

	return &LoginResponse{
		Token:     token,
		ExpiresAt: session.ExpiresAt,
		User:      *user,
	}, nil
}

// ValidateSession validates a session token and returns the user
func (a *Auth) ValidateSession(token string) (*User, *Session, error) {
	session, err := a.store.GetSessionByToken(a.HashToken(token))
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	if time.Now().After(session.ExpiresAt) {
		a.store.DeleteSession(session.ID)
		return nil, nil, ErrSessionExpired
	}

	user, err := a.store.GetUser(session.UserID)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	if !user.IsActive {
		return nil, nil, ErrUserNotActive
	}

	return user, session, nil
}

// Logout invalidates a session
func (a *Auth) Logout(token string) error {
	session, err := a.store.GetSessionByToken(a.HashToken(token))
	if err != nil {
		return nil
	}
	return a.store.DeleteSession(session.ID)
}

// LogoutAll invalidates all sessions for a user
func (a *Auth) LogoutAll(userID string) error {
	return a.store.DeleteUserSessions(userID)
}

// ValidateAPIKey validates an API key and returns associated info
func (a *Auth) ValidateAPIKey(key string) (*APIKey, *User, error) {
	apiKey, err := a.store.GetAPIKeyByHash(a.HashToken(key))
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	if !apiKey.IsActive {
		return nil, nil, ErrAPIKeyInactive
	}

	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, nil, ErrAPIKeyExpired
	}

	user, err := a.store.GetUser(apiKey.UserID)
	if err != nil {
		return nil, nil, ErrInvalidToken
	}

	// Update last used
	a.store.UpdateAPIKeyLastUsed(apiKey.ID)

	return apiKey, user, nil
}

// CreateAPIKey creates a new API key
func (a *Auth) CreateAPIKey(orgID, userID, name string, permissions []Permission, expiresIn string) (*APIKeyCreated, error) {
	key, prefix, err := a.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	var expiresAt *time.Time
	if expiresIn != "" && expiresIn != "never" {
		d, err := parseDuration(expiresIn)
		if err == nil {
			t := time.Now().Add(d)
			expiresAt = &t
		}
	}

	apiKey := &APIKey{
		ID:          fmt.Sprintf("key_%d", time.Now().UnixNano()),
		OrgID:       orgID,
		UserID:      userID,
		Name:        name,
		KeyPrefix:   prefix,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	if err := a.store.CreateAPIKey(apiKey, a.HashToken(key)); err != nil {
		return nil, err
	}

	return &APIKeyCreated{
		APIKey: *apiKey,
		Key:    key,
	}, nil
}

// CreateUser creates a new user
func (a *Auth) CreateUser(orgID string, req *UserCreate) (*User, error) {
	hash, err := a.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:           fmt.Sprintf("user_%d", time.Now().UnixNano()),
		OrgID:        orgID,
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordHash: hash,
		Name:         req.Name,
		Role:         req.Role,
		IsActive:     true,
	}

	if err := a.store.CreateUser(user); err != nil {
		return nil, err
	}

	return user, nil
}

// ChangePassword changes a user's password
func (a *Auth) ChangePassword(userID, oldPassword, newPassword string) error {
	user, err := a.store.GetUser(userID)
	if err != nil {
		return err
	}

	if !a.CheckPassword(oldPassword, user.PasswordHash) {
		return ErrInvalidCredentials
	}

	hash, err := a.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return a.store.UpdateUserPassword(userID, hash)
}

// ResetPassword resets a user's password (admin action)
func (a *Auth) ResetPassword(userID, newPassword string) error {
	hash, err := a.HashPassword(newPassword)
	if err != nil {
		return err
	}

	// Invalidate all sessions
	a.store.DeleteUserSessions(userID)

	return a.store.UpdateUserPassword(userID, hash)
}

// CreateInvite creates an invitation to join an organization
func (a *Auth) CreateInvite(orgID, email string, role Role, invitedBy string) (*Invite, string, error) {
	token, err := a.GenerateToken()
	if err != nil {
		return nil, "", err
	}

	invite := &Invite{
		ID:        fmt.Sprintf("inv_%d", time.Now().UnixNano()),
		OrgID:     orgID,
		Email:     strings.ToLower(strings.TrimSpace(email)),
		Role:      role,
		InvitedBy: invitedBy,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := a.store.CreateInvite(invite, a.HashToken(token)); err != nil {
		return nil, "", err
	}

	return invite, token, nil
}

// AcceptInvite accepts an invitation and creates a user
func (a *Auth) AcceptInvite(token, password, name string) (*User, error) {
	invite, err := a.store.GetInviteByToken(a.HashToken(token))
	if err != nil {
		return nil, ErrInvalidInvite
	}

	if time.Now().After(invite.ExpiresAt) {
		a.store.DeleteInvite(invite.ID)
		return nil, ErrInvalidInvite
	}

	// Check if user already exists
	existing, _ := a.store.GetUserByEmail(invite.OrgID, invite.Email)
	if existing != nil {
		a.store.DeleteInvite(invite.ID)
		return nil, errors.New("user already exists")
	}

	// Create user
	user, err := a.CreateUser(invite.OrgID, &UserCreate{
		Email:    invite.Email,
		Password: password,
		Name:     name,
		Role:     invite.Role,
	})
	if err != nil {
		return nil, err
	}

	// Delete invite
	a.store.DeleteInvite(invite.ID)

	return user, nil
}

// CheckPermission checks if a user has a specific permission
func (a *Auth) CheckPermission(user *User, resource, action string) bool {
	return HasPermission(user.Role, resource, action)
}

// CheckAPIKeyPermission checks if an API key has a specific permission
func (a *Auth) CheckAPIKeyPermission(key *APIKey, resource, action string) bool {
	for _, p := range key.Permissions {
		if p.Resource == ResourceAll && p.Action == ActionAll {
			return true
		}
		if p.Resource == ResourceAll && p.Action == action {
			return true
		}
		if p.Resource == resource && p.Action == ActionAll {
			return true
		}
		if p.Resource == resource && p.Action == action {
			return true
		}
	}
	return false
}

// SecureCompare performs a constant-time comparison
func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// parseDuration parses a duration string like "30d", "1y", "2h"
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, errors.New("invalid duration")
	}

	value := s[:len(s)-1]
	unit := s[len(s)-1:]

	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, err
	}

	switch unit {
	case "s":
		return time.Duration(n) * time.Second, nil
	case "m":
		return time.Duration(n) * time.Minute, nil
	case "h":
		return time.Duration(n) * time.Hour, nil
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	case "w":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case "y":
		return time.Duration(n) * 365 * 24 * time.Hour, nil
	default:
		return 0, errors.New("unknown duration unit")
	}
}

// EnsureDefaultAdmin ensures there's at least one admin user
func (a *Auth) EnsureDefaultAdmin(orgID, email, password string) (*User, error) {
	// Check if any users exist in this org
	users, _ := a.store.ListUsers(orgID)
	if len(users) > 0 {
		return &users[0], nil
	}

	// Create admin user
	return a.CreateUser(orgID, &UserCreate{
		Email:    email,
		Password: password,
		Name:     "Admin",
		Role:     RoleOwner,
	})
}
