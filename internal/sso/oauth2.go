package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	ErrProviderNotConfigured = errors.New("oauth provider not configured")
	ErrInvalidState          = errors.New("invalid oauth state")
	ErrOAuthFailed           = errors.New("oauth authentication failed")
)

// OAuthProvider represents an OAuth2 provider
type OAuthProvider interface {
	// Name returns the provider name (google, github, etc.)
	Name() string

	// AuthURL returns the authorization URL for the OAuth flow
	AuthURL(state, redirectURI string) string

	// Exchange exchanges an authorization code for tokens
	Exchange(ctx context.Context, code, redirectURI string) (*OAuthToken, error)

	// GetUserInfo retrieves user information using the access token
	GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error)

	// RefreshToken refreshes an expired access token
	RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error)
}

// OAuthToken represents OAuth2 tokens
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int       `json:"expires_in,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	IDToken      string    `json:"id_token,omitempty"` // For OIDC providers
}

// OAuthUser represents user information from OAuth provider
type OAuthUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture,omitempty"`
	Provider      string `json:"provider"`
}

// OAuthConfig holds common OAuth2 configuration
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

// OAuthManager manages multiple OAuth providers
type OAuthManager struct {
	providers map[string]OAuthProvider
	states    map[string]*oauthState // state -> metadata
	mu        sync.RWMutex
}

type oauthState struct {
	Provider    string
	RedirectURI string
	CreatedAt   time.Time
	Nonce       string
	OrgID       string // Pre-selected org for login
}

// NewOAuthManager creates a new OAuth manager
func NewOAuthManager() *OAuthManager {
	m := &OAuthManager{
		providers: make(map[string]OAuthProvider),
		states:    make(map[string]*oauthState),
	}

	// Start cleanup goroutine
	go m.cleanupStates()

	return m
}

// RegisterProvider registers an OAuth provider
func (m *OAuthManager) RegisterProvider(provider OAuthProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[provider.Name()] = provider
}

// GetProvider returns a registered provider
func (m *OAuthManager) GetProvider(name string) (OAuthProvider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, ok := m.providers[name]
	if !ok {
		return nil, ErrProviderNotConfigured
	}
	return provider, nil
}

// ListProviders returns all registered provider names
func (m *OAuthManager) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// CreateAuthURL creates an authorization URL for the specified provider
func (m *OAuthManager) CreateAuthURL(providerName, redirectURI, orgID string) (string, string, error) {
	provider, err := m.GetProvider(providerName)
	if err != nil {
		return "", "", err
	}

	state := GenerateState()
	nonce := GenerateNonce()

	m.mu.Lock()
	m.states[state] = &oauthState{
		Provider:    providerName,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now(),
		Nonce:       nonce,
		OrgID:       orgID,
	}
	m.mu.Unlock()

	authURL := provider.AuthURL(state, redirectURI)
	return authURL, state, nil
}

// ValidateState validates an OAuth state and returns metadata
func (m *OAuthManager) ValidateState(state string) (*oauthState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.states[state]
	if !ok {
		return nil, ErrInvalidState
	}

	// Check expiry (10 minutes)
	if time.Since(s.CreatedAt) > 10*time.Minute {
		delete(m.states, state)
		return nil, ErrInvalidState
	}

	// Delete state after use (one-time use)
	delete(m.states, state)

	return s, nil
}

// HandleCallback processes the OAuth callback
func (m *OAuthManager) HandleCallback(ctx context.Context, providerName, code, state string) (*OAuthUser, *OAuthToken, string, error) {
	// Validate state
	stateInfo, err := m.ValidateState(state)
	if err != nil {
		return nil, nil, "", err
	}

	// Get provider
	provider, err := m.GetProvider(providerName)
	if err != nil {
		return nil, nil, "", err
	}

	// Exchange code for tokens
	token, err := provider.Exchange(ctx, code, stateInfo.RedirectURI)
	if err != nil {
		return nil, nil, "", fmt.Errorf("token exchange failed: %w", err)
	}

	// Get user info
	user, err := provider.GetUserInfo(ctx, token)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get user info: %w", err)
	}

	return user, token, stateInfo.OrgID, nil
}

// cleanupStates periodically cleans up expired states
func (m *OAuthManager) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for state, s := range m.states {
			if now.Sub(s.CreatedAt) > 10*time.Minute {
				delete(m.states, state)
			}
		}
		m.mu.Unlock()
	}
}

// =============================================================================
// Google OAuth2 Provider
// =============================================================================

// GoogleProvider implements OAuth2 for Google
type GoogleProvider struct {
	config OAuthConfig
}

// NewGoogleProvider creates a new Google OAuth provider
func NewGoogleProvider(clientID, clientSecret, redirectURL string) *GoogleProvider {
	return &GoogleProvider{
		config: OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
		},
	}
}

func (g *GoogleProvider) Name() string {
	return "google"
}

func (g *GoogleProvider) AuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":     {g.config.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(g.config.Scopes, " ")},
		"state":         {state},
		"access_type":   {"offline"}, // Get refresh token
		"prompt":        {"consent"}, // Force consent to get refresh token
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

func (g *GoogleProvider) Exchange(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {g.config.ClientID},
		"client_secret": {g.config.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

func (g *GoogleProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %s", string(body))
	}

	var info struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &OAuthUser{
		ID:            info.ID,
		Email:         info.Email,
		EmailVerified: info.VerifiedEmail,
		Name:          info.Name,
		Picture:       info.Picture,
		Provider:      "google",
	}, nil
}

func (g *GoogleProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {g.config.ClientID},
		"client_secret": {g.config.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	token.RefreshToken = refreshToken // Preserve refresh token
	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

// =============================================================================
// GitHub OAuth2 Provider
// =============================================================================

// GitHubProvider implements OAuth2 for GitHub
type GitHubProvider struct {
	config OAuthConfig
}

// NewGitHubProvider creates a new GitHub OAuth provider
func NewGitHubProvider(clientID, clientSecret, redirectURL string) *GitHubProvider {
	return &GitHubProvider{
		config: OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
		},
	}
}

func (g *GitHubProvider) Name() string {
	return "github"
}

func (g *GitHubProvider) AuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":    {g.config.ClientID},
		"redirect_uri": {redirectURI},
		"scope":        {strings.Join(g.config.Scopes, " ")},
		"state":        {state},
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

func (g *GitHubProvider) Exchange(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {g.config.ClientID},
		"client_secret": {g.config.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://github.com/login/oauth/access_token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	// Check for error in response
	var errResp struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &errResp)
	if errResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.ErrorDescription)
	}

	return &token, nil
}

func (g *GitHubProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	// Get user profile
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var profile struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}

	// If email is not public, fetch from emails endpoint
	email := profile.Email
	if email == "" {
		email = g.fetchPrimaryEmail(ctx, token.AccessToken)
	}

	name := profile.Name
	if name == "" {
		name = profile.Login
	}

	return &OAuthUser{
		ID:            fmt.Sprintf("%d", profile.ID),
		Email:         email,
		EmailVerified: email != "", // GitHub emails are verified
		Name:          name,
		Picture:       profile.AvatarURL,
		Provider:      "github",
	}, nil
}

func (g *GitHubProvider) fetchPrimaryEmail(ctx context.Context, accessToken string) string {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}

	// Find primary verified email
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}

	// Fall back to any verified email
	for _, e := range emails {
		if e.Verified {
			return e.Email
		}
	}

	return ""
}

func (g *GitHubProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	// GitHub doesn't support refresh tokens for OAuth apps (only GitHub Apps)
	return nil, errors.New("github does not support token refresh")
}

// =============================================================================
// Microsoft (Azure AD) OAuth2 Provider
// =============================================================================

// MicrosoftProvider implements OAuth2 for Microsoft/Azure AD
type MicrosoftProvider struct {
	config   OAuthConfig
	TenantID string // "common", "organizations", "consumers", or specific tenant ID
}

// NewMicrosoftProvider creates a new Microsoft OAuth provider
func NewMicrosoftProvider(clientID, clientSecret, redirectURL, tenantID string) *MicrosoftProvider {
	if tenantID == "" {
		tenantID = "common"
	}
	return &MicrosoftProvider{
		config: OAuthConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile", "User.Read"},
		},
		TenantID: tenantID,
	}
}

func (m *MicrosoftProvider) Name() string {
	return "microsoft"
}

func (m *MicrosoftProvider) AuthURL(state, redirectURI string) string {
	params := url.Values{
		"client_id":     {m.config.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(m.config.Scopes, " ")},
		"state":         {state},
		"response_mode": {"query"},
	}
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?%s", m.TenantID, params.Encode())
}

func (m *MicrosoftProvider) Exchange(ctx context.Context, code, redirectURI string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
		"scope":         {strings.Join(m.config.Scopes, " ")},
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", m.TenantID)
	req, _ := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}

func (m *MicrosoftProvider) GetUserInfo(ctx context.Context, token *OAuthToken) (*OAuthUser, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user info: %s", string(body))
	}

	var info struct {
		ID                string `json:"id"`
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
		DisplayName       string `json:"displayName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	email := info.Mail
	if email == "" {
		email = info.UserPrincipalName
	}

	return &OAuthUser{
		ID:            info.ID,
		Email:         email,
		EmailVerified: true, // Microsoft accounts are verified
		Name:          info.DisplayName,
		Provider:      "microsoft",
	}, nil
}

func (m *MicrosoftProvider) RefreshToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	data := url.Values{
		"client_id":     {m.config.ClientID},
		"client_secret": {m.config.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {strings.Join(m.config.Scopes, " ")},
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", m.TenantID)
	req, _ := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var token OAuthToken
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return nil, err
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}

	return &token, nil
}
