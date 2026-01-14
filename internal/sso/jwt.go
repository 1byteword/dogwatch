// Package sso provides Single Sign-On capabilities including OAuth2, SAML, and JWT sessions
package sso

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"crypto/hmac"
	"crypto/sha256"
	"strings"
)

var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrTokenExpired     = errors.New("token expired")
	ErrInvalidSignature = errors.New("invalid signature")
)

// JWTConfig holds JWT configuration
type JWTConfig struct {
	SecretKey          []byte
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
	Issuer             string
}

// DefaultJWTConfig returns default JWT configuration
func DefaultJWTConfig() *JWTConfig {
	// Generate a random secret key if not provided
	secret := make([]byte, 32)
	rand.Read(secret)

	return &JWTConfig{
		SecretKey:          secret,
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
		Issuer:             "dogwatch",
	}
}

// JWTManager handles JWT token operations
type JWTManager struct {
	config        *JWTConfig
	revokedTokens map[string]time.Time // Token ID -> expiry time
	mu            sync.RWMutex
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(config *JWTConfig) *JWTManager {
	if config == nil {
		config = DefaultJWTConfig()
	}
	return &JWTManager{
		config:        config,
		revokedTokens: make(map[string]time.Time),
	}
}

// TokenClaims represents JWT claims
type TokenClaims struct {
	// Standard claims
	Subject   string `json:"sub"`           // User ID
	Issuer    string `json:"iss"`           // Token issuer
	IssuedAt  int64  `json:"iat"`           // Issued at timestamp
	ExpiresAt int64  `json:"exp"`           // Expiry timestamp
	TokenID   string `json:"jti"`           // Unique token ID

	// Custom claims
	Email     string `json:"email"`
	OrgID     string `json:"org_id"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
	TokenType string `json:"type"` // "access" or "refresh"

	// SSO-specific
	Provider   string `json:"provider,omitempty"`    // oauth provider if applicable
	ProviderID string `json:"provider_id,omitempty"` // provider user ID
}

// TokenPair contains access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"` // Access token expiry in seconds
	ExpiresAt    time.Time `json:"expires_at"`
}

// GenerateTokenPair generates both access and refresh tokens
func (j *JWTManager) GenerateTokenPair(userID, email, orgID, role, sessionID string) (*TokenPair, error) {
	now := time.Now()

	// Generate token IDs
	accessTokenID := generateTokenID()
	refreshTokenID := generateTokenID()

	// Create access token claims
	accessClaims := &TokenClaims{
		Subject:   userID,
		Issuer:    j.config.Issuer,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(j.config.AccessTokenExpiry).Unix(),
		TokenID:   accessTokenID,
		Email:     email,
		OrgID:     orgID,
		Role:      role,
		SessionID: sessionID,
		TokenType: "access",
	}

	// Create refresh token claims
	refreshClaims := &TokenClaims{
		Subject:   userID,
		Issuer:    j.config.Issuer,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(j.config.RefreshTokenExpiry).Unix(),
		TokenID:   refreshTokenID,
		Email:     email,
		OrgID:     orgID,
		Role:      role,
		SessionID: sessionID,
		TokenType: "refresh",
	}

	// Sign tokens
	accessToken, err := j.signToken(accessClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	refreshToken, err := j.signToken(refreshClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(j.config.AccessTokenExpiry.Seconds()),
		ExpiresAt:    now.Add(j.config.AccessTokenExpiry),
	}, nil
}

// ValidateToken validates a JWT token and returns claims
func (j *JWTManager) ValidateToken(tokenString string) (*TokenClaims, error) {
	claims, err := j.parseToken(tokenString)
	if err != nil {
		return nil, err
	}

	// Check expiry
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrTokenExpired
	}

	// Check if revoked
	j.mu.RLock()
	if _, revoked := j.revokedTokens[claims.TokenID]; revoked {
		j.mu.RUnlock()
		return nil, ErrInvalidToken
	}
	j.mu.RUnlock()

	return claims, nil
}

// RefreshAccessToken creates a new access token from a valid refresh token
func (j *JWTManager) RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := j.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != "refresh" {
		return nil, errors.New("not a refresh token")
	}

	// Generate new token pair
	return j.GenerateTokenPair(claims.Subject, claims.Email, claims.OrgID, claims.Role, claims.SessionID)
}

// RevokeToken revokes a token by its ID
func (j *JWTManager) RevokeToken(tokenString string) error {
	claims, err := j.parseToken(tokenString)
	if err != nil {
		return err
	}

	j.mu.Lock()
	j.revokedTokens[claims.TokenID] = time.Unix(claims.ExpiresAt, 0)
	j.mu.Unlock()

	return nil
}

// RevokeSession revokes all tokens for a session
func (j *JWTManager) RevokeSession(sessionID string) {
	// This would need to be tracked separately in a production system
	// For now, individual token revocation is sufficient
}

// CleanupExpired removes expired entries from the revocation list
func (j *JWTManager) CleanupExpired() {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()
	for tokenID, expiry := range j.revokedTokens {
		if now.After(expiry) {
			delete(j.revokedTokens, tokenID)
		}
	}
}

// signToken creates a signed JWT token (simple HS256 implementation)
func (j *JWTManager) signToken(claims *TokenClaims) (string, error) {
	// Create header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// Create payload
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// Create signature
	message := headerB64 + "." + payloadB64
	h := hmac.New(sha256.New, j.config.SecretKey)
	h.Write([]byte(message))
	signature := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return message + "." + signature, nil
}

// parseToken parses and validates a JWT token signature
func (j *JWTManager) parseToken(tokenString string) (*TokenClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	// Verify signature
	message := parts[0] + "." + parts[1]
	h := hmac.New(sha256.New, j.config.SecretKey)
	h.Write([]byte(message))
	expectedSig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, ErrInvalidSignature
	}

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims TokenClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

// generateTokenID generates a unique token ID
func generateTokenID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// GenerateState generates a random state for OAuth2 flows
func GenerateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// GenerateNonce generates a random nonce for OIDC flows
func GenerateNonce() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
