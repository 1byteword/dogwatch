package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"dogwatch/internal/rbac"
	"dogwatch/internal/sso"
)

var (
	ssoStore      *sso.Store
	jwtManager    *sso.JWTManager
	oauthManager  *sso.OAuthManager
	samlManager   *sso.SAMLManager
)

// SetSSOStore sets the SSO store for handlers
func SetSSOStore(store *sso.Store) {
	ssoStore = store
}

// SetJWTManager sets the JWT manager
func SetJWTManager(manager *sso.JWTManager) {
	jwtManager = manager
}

// SetOAuthManager sets the OAuth manager
func SetOAuthManager(manager *sso.OAuthManager) {
	oauthManager = manager
}

// SetSAMLManager sets the SAML manager
func SetSAMLManager(manager *sso.SAMLManager) {
	samlManager = manager
}

// InitSSO initializes SSO components
func InitSSO(store *sso.Store, baseURL string) {
	ssoStore = store

	// Initialize JWT manager
	secret := os.Getenv("DOGWATCH_JWT_SECRET")
	config := sso.DefaultJWTConfig()
	if secret != "" {
		config.SecretKey = []byte(secret)
	} else {
		log.Println("WARNING: DOGWATCH_JWT_SECRET not set - using random secret")
		log.Println("         Sessions will not persist across restarts")
		log.Println("         Set DOGWATCH_JWT_SECRET for production use")
	}
	jwtManager = sso.NewJWTManager(config)

	// Initialize OAuth manager
	oauthManager = sso.NewOAuthManager()

	// Register Google provider if configured
	if clientID := os.Getenv("DOGWATCH_GOOGLE_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("DOGWATCH_GOOGLE_CLIENT_SECRET")
		redirectURL := baseURL + "/api/auth/callback/google"
		oauthManager.RegisterProvider(sso.NewGoogleProvider(clientID, clientSecret, redirectURL))
		log.Println("[sso] Google OAuth provider registered")
	}

	// Register GitHub provider if configured
	if clientID := os.Getenv("DOGWATCH_GITHUB_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("DOGWATCH_GITHUB_CLIENT_SECRET")
		redirectURL := baseURL + "/api/auth/callback/github"
		oauthManager.RegisterProvider(sso.NewGitHubProvider(clientID, clientSecret, redirectURL))
		log.Println("[sso] GitHub OAuth provider registered")
	}

	// Register Microsoft provider if configured
	if clientID := os.Getenv("DOGWATCH_MICROSOFT_CLIENT_ID"); clientID != "" {
		clientSecret := os.Getenv("DOGWATCH_MICROSOFT_CLIENT_SECRET")
		tenantID := os.Getenv("DOGWATCH_MICROSOFT_TENANT_ID")
		redirectURL := baseURL + "/api/auth/callback/microsoft"
		oauthManager.RegisterProvider(sso.NewMicrosoftProvider(clientID, clientSecret, redirectURL, tenantID))
		log.Println("[sso] Microsoft OAuth provider registered")
	}

	// Initialize SAML manager
	samlManager = sso.NewSAMLManager()
}

// RegisterSSORoutes registers SSO API routes
func RegisterSSORoutes(mux *http.ServeMux) {
	// OAuth2 routes
	mux.HandleFunc("/api/auth/providers", handleListProviders)
	mux.HandleFunc("/api/auth/oauth/", handleOAuthStart)
	mux.HandleFunc("/api/auth/callback/", handleOAuthCallback)

	// SAML routes
	mux.HandleFunc("/api/auth/saml/login", handleSAMLLogin)
	mux.HandleFunc("/api/auth/saml/acs", handleSAMLACS)
	mux.HandleFunc("/api/auth/saml/metadata", handleSAMLMetadata)

	// JWT token routes
	mux.HandleFunc("/api/auth/token/refresh", handleTokenRefresh)
	mux.HandleFunc("/api/auth/token/revoke", handleTokenRevoke)

	// Linked accounts routes
	mux.HandleFunc("/api/auth/linked", handleLinkedAccounts)
	mux.HandleFunc("/api/auth/linked/", handleUnlinkAccount)

	// SSO configuration routes (admin only)
	mux.HandleFunc("/api/auth/sso/config", handleSSOConfig)
}

// handleListProviders returns available authentication providers
func handleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	providers := []map[string]interface{}{}

	// Add password auth
	providers = append(providers, map[string]interface{}{
		"id":   "password",
		"name": "Email & Password",
		"type": "password",
	})

	// Add OAuth providers
	if oauthManager != nil {
		for _, name := range oauthManager.ListProviders() {
			displayName := strings.Title(name)
			providers = append(providers, map[string]interface{}{
				"id":   name,
				"name": displayName,
				"type": "oauth2",
			})
		}
	}

	// Check for org-specific SAML
	orgID := r.URL.Query().Get("org")
	if orgID != "" && ssoStore != nil {
		config, err := ssoStore.GetOrgSSOConfig(orgID)
		if err == nil && config.SAMLEnabled {
			providers = append(providers, map[string]interface{}{
				"id":   "saml",
				"name": "Enterprise SSO",
				"type": "saml",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	})
}

// handleOAuthStart initiates OAuth flow
func handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if oauthManager == nil {
		http.Error(w, `{"error":"oauth not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract provider from path: /api/auth/oauth/{provider}
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/oauth/")
	provider := strings.Split(path, "/")[0]

	if provider == "" {
		http.Error(w, `{"error":"provider required"}`, http.StatusBadRequest)
		return
	}

	// Get redirect URI
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		// Build from request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		redirectURI = fmt.Sprintf("%s://%s/api/auth/callback/%s", scheme, r.Host, provider)
	}

	// Optional org ID for pre-selecting organization
	orgID := r.URL.Query().Get("org")

	// Create auth URL
	authURL, state, err := oauthManager.CreateAuthURL(provider, redirectURI, orgID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"provider not available: %s"}`, err), http.StatusBadRequest)
		return
	}

	// Store state in cookie for callback
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	// Redirect to provider
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOAuthCallback handles OAuth callback
func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if oauthManager == nil || rbacAuth == nil || rbacStore == nil {
		http.Error(w, `{"error":"oauth not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract provider from path
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/callback/")
	provider := strings.Split(path, "/")[0]

	// Get code and state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		// Check for error
		errMsg := r.URL.Query().Get("error")
		errDesc := r.URL.Query().Get("error_description")
		if errMsg != "" {
			http.Error(w, fmt.Sprintf(`{"error":"%s: %s"}`, errMsg, errDesc), http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"authorization code required"}`, http.StatusBadRequest)
		return
	}

	// Handle callback
	oauthUser, oauthToken, orgID, err := oauthManager.HandleCallback(r.Context(), provider, code, state)
	if err != nil {
		log.Printf("[sso] OAuth callback error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"authentication failed: %s"}`, err), http.StatusUnauthorized)
		return
	}

	// Process the OAuth user
	user, err := processOAuthUser(oauthUser, oauthToken, orgID)
	if err != nil {
		log.Printf("[sso] Failed to process OAuth user: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}

	// Generate JWT tokens
	sessionID := fmt.Sprintf("sso_%d", time.Now().UnixNano())
	tokenPair, err := jwtManager.GenerateTokenPair(user.ID, user.Email, user.OrgID, string(user.Role), sessionID)
	if err != nil {
		http.Error(w, `{"error":"failed to generate tokens"}`, http.StatusInternalServerError)
		return
	}

	// Set cookies
	setAuthCookies(w, r, tokenPair)

	// Clear OAuth state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	// Redirect to app or return JSON
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  tokenPair.AccessToken,
			"refresh_token": tokenPair.RefreshToken,
			"token_type":    tokenPair.TokenType,
			"expires_in":    tokenPair.ExpiresIn,
			"user":          user,
		})
	} else {
		// Redirect to dashboard
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// processOAuthUser creates or updates a user from OAuth
func processOAuthUser(oauthUser *sso.OAuthUser, oauthToken *sso.OAuthToken, orgID string) (*rbac.User, error) {
	// Try to find existing linked account
	linkedAccount, err := ssoStore.GetLinkedAccount(oauthUser.Provider, oauthUser.ID)
	if err == nil && linkedAccount != nil {
		// Update tokens
		ssoStore.UpdateLinkedAccountTokens(linkedAccount.ID, oauthToken.AccessToken,
			oauthToken.RefreshToken, &oauthToken.ExpiresAt)

		// Return existing user
		return rbacStore.GetUser(linkedAccount.UserID)
	}

	// Try to find user by email
	user, err := rbacStore.GetUserByEmailAnyOrg(oauthUser.Email)
	if err == nil && user != nil {
		// Link account to existing user
		account := &sso.LinkedAccount{
			ID:           fmt.Sprintf("link_%d", time.Now().UnixNano()),
			UserID:       user.ID,
			OrgID:        user.OrgID,
			Provider:     oauthUser.Provider,
			ProviderID:   oauthUser.ID,
			Email:        oauthUser.Email,
			AccessToken:  oauthToken.AccessToken,
			RefreshToken: oauthToken.RefreshToken,
			TokenExpiry:  &oauthToken.ExpiresAt,
		}
		ssoStore.CreateLinkedAccount(account)
		return user, nil
	}

	// Determine organization
	if orgID == "" {
		// Get default org
		org, err := rbacStore.EnsureDefaultOrg("default", "Default Organization")
		if err != nil {
			return nil, fmt.Errorf("no organization available")
		}
		orgID = org.ID
	}

	// Check SSO config for auto-provisioning
	ssoConfig, _ := ssoStore.GetOrgSSOConfig(orgID)
	if ssoConfig != nil && !ssoConfig.AutoProvision {
		return nil, fmt.Errorf("user not found and auto-provisioning disabled")
	}

	// Check allowed domains
	if ssoConfig != nil && len(ssoConfig.AllowedDomains) > 0 {
		emailParts := strings.Split(oauthUser.Email, "@")
		if len(emailParts) != 2 {
			return nil, fmt.Errorf("invalid email address")
		}
		domain := emailParts[1]
		allowed := false
		for _, d := range ssoConfig.AllowedDomains {
			if strings.EqualFold(d, domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("email domain not allowed")
		}
	}

	// Create new user
	role := rbac.RoleViewer
	if ssoConfig != nil && ssoConfig.DefaultRole != "" {
		role = rbac.Role(ssoConfig.DefaultRole)
	}

	user = &rbac.User{
		ID:       fmt.Sprintf("user_%d", time.Now().UnixNano()),
		OrgID:    orgID,
		Email:    oauthUser.Email,
		Name:     oauthUser.Name,
		Role:     role,
		IsActive: true,
		AvatarURL: oauthUser.Picture,
	}

	if err := rbacStore.CreateUser(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Link account
	account := &sso.LinkedAccount{
		ID:           fmt.Sprintf("link_%d", time.Now().UnixNano()),
		UserID:       user.ID,
		OrgID:        user.OrgID,
		Provider:     oauthUser.Provider,
		ProviderID:   oauthUser.ID,
		Email:        oauthUser.Email,
		AccessToken:  oauthToken.AccessToken,
		RefreshToken: oauthToken.RefreshToken,
		TokenExpiry:  &oauthToken.ExpiresAt,
	}
	ssoStore.CreateLinkedAccount(account)

	return user, nil
}

// handleSAMLLogin initiates SAML login
func handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	if samlManager == nil {
		http.Error(w, `{"error":"saml not configured"}`, http.StatusServiceUnavailable)
		return
	}

	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		http.Error(w, `{"error":"org parameter required"}`, http.StatusBadRequest)
		return
	}

	// Build ACS URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	acsURL := fmt.Sprintf("%s://%s/api/auth/saml/acs", scheme, r.Host)

	// Optional relay state for post-login redirect
	relayState := r.URL.Query().Get("redirect")
	if relayState == "" {
		relayState = "/"
	}

	// Create AuthnRequest
	redirectURL, _, err := samlManager.CreateAuthnRequest(orgID, acsURL, relayState)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"saml not configured for org: %s"}`, err), http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// handleSAMLACS handles SAML Assertion Consumer Service
func handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	if samlManager == nil || rbacAuth == nil || rbacStore == nil {
		http.Error(w, `{"error":"saml not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}

	samlResponse := r.FormValue("SAMLResponse")
	relayState := r.FormValue("RelayState")

	if samlResponse == "" {
		http.Error(w, `{"error":"SAMLResponse required"}`, http.StatusBadRequest)
		return
	}

	// Process SAML response
	samlUser, orgID, err := samlManager.ProcessResponse(samlResponse, relayState)
	if err != nil {
		log.Printf("[sso] SAML response error: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"authentication failed: %s"}`, err), http.StatusUnauthorized)
		return
	}

	// Process SAML user
	user, err := processSAMLUser(samlUser, orgID)
	if err != nil {
		log.Printf("[sso] Failed to process SAML user: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err), http.StatusInternalServerError)
		return
	}

	// Generate JWT tokens
	sessionID := fmt.Sprintf("saml_%d", time.Now().UnixNano())
	tokenPair, err := jwtManager.GenerateTokenPair(user.ID, user.Email, user.OrgID, string(user.Role), sessionID)
	if err != nil {
		http.Error(w, `{"error":"failed to generate tokens"}`, http.StatusInternalServerError)
		return
	}

	// Set cookies
	setAuthCookies(w, r, tokenPair)

	// Redirect to relay state or home
	redirectTo := "/"
	if relayState != "" && strings.HasPrefix(relayState, "/") {
		redirectTo = relayState
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// processSAMLUser creates or updates a user from SAML
func processSAMLUser(samlUser *sso.SAMLUser, orgID string) (*rbac.User, error) {
	// Try to find existing linked account
	linkedAccount, err := ssoStore.GetLinkedAccountByEmail(samlUser.Email, "saml")
	if err == nil && linkedAccount != nil {
		// Return existing user
		return rbacStore.GetUser(linkedAccount.UserID)
	}

	// Try to find user by email
	user, err := rbacStore.GetUserByEmail(orgID, samlUser.Email)
	if err == nil && user != nil {
		// Link account to existing user
		account := &sso.LinkedAccount{
			ID:         fmt.Sprintf("link_%d", time.Now().UnixNano()),
			UserID:     user.ID,
			OrgID:      user.OrgID,
			Provider:   "saml",
			ProviderID: samlUser.NameID,
			Email:      samlUser.Email,
		}
		ssoStore.CreateLinkedAccount(account)
		return user, nil
	}

	// Check SSO config for auto-provisioning
	ssoConfig, _ := ssoStore.GetOrgSSOConfig(orgID)
	if ssoConfig != nil && !ssoConfig.AutoProvision {
		return nil, fmt.Errorf("user not found and auto-provisioning disabled")
	}

	// Create new user
	role := rbac.RoleViewer
	if ssoConfig != nil && ssoConfig.DefaultRole != "" {
		role = rbac.Role(ssoConfig.DefaultRole)
	}

	name := samlUser.FirstName
	if samlUser.LastName != "" {
		name += " " + samlUser.LastName
	}
	if name == "" {
		name = strings.Split(samlUser.Email, "@")[0]
	}

	user = &rbac.User{
		ID:       fmt.Sprintf("user_%d", time.Now().UnixNano()),
		OrgID:    orgID,
		Email:    samlUser.Email,
		Name:     name,
		Role:     role,
		IsActive: true,
	}

	if err := rbacStore.CreateUser(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Link account
	account := &sso.LinkedAccount{
		ID:         fmt.Sprintf("link_%d", time.Now().UnixNano()),
		UserID:     user.ID,
		OrgID:      user.OrgID,
		Provider:   "saml",
		ProviderID: samlUser.NameID,
		Email:      samlUser.Email,
	}
	ssoStore.CreateLinkedAccount(account)

	return user, nil
}

// handleSAMLMetadata returns SP metadata
func handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	if samlManager == nil {
		http.Error(w, `{"error":"saml not configured"}`, http.StatusServiceUnavailable)
		return
	}

	orgID := r.URL.Query().Get("org")
	if orgID == "" {
		orgID = "default"
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	acsURL := fmt.Sprintf("%s://%s/api/auth/saml/acs", scheme, r.Host)
	entityID := fmt.Sprintf("%s://%s/api/auth/saml/metadata?org=%s", scheme, r.Host, orgID)

	metadata, err := samlManager.GetMetadata(orgID, acsURL, entityID)
	if err != nil {
		http.Error(w, `{"error":"failed to generate metadata"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(metadata))
}

// handleTokenRefresh refreshes an access token
func handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	if jwtManager == nil {
		http.Error(w, `{"error":"jwt not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Also try cookie
	if req.RefreshToken == "" {
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			req.RefreshToken = cookie.Value
		}
	}

	if req.RefreshToken == "" {
		http.Error(w, `{"error":"refresh token required"}`, http.StatusBadRequest)
		return
	}

	tokenPair, err := jwtManager.RefreshAccessToken(req.RefreshToken)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"refresh failed: %s"}`, err), http.StatusUnauthorized)
		return
	}

	// Set cookies
	setAuthCookies(w, r, tokenPair)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
		"token_type":    tokenPair.TokenType,
		"expires_in":    tokenPair.ExpiresIn,
	})
}

// handleTokenRevoke revokes tokens
func handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if jwtManager == nil {
		http.Error(w, `{"error":"jwt not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Get token from body or header
	var req struct {
		Token string `json:"token"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Token == "" {
		if auth := r.Header.Get("Authorization"); auth != "" {
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) == 2 {
				req.Token = parts[1]
			}
		}
	}

	if req.Token != "" {
		jwtManager.RevokeToken(req.Token)
	}

	// Clear cookies
	clearAuthCookies(w)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

// handleLinkedAccounts returns user's linked accounts
func handleLinkedAccounts(w http.ResponseWriter, r *http.Request) {
	if ssoStore == nil {
		http.Error(w, `{"error":"sso not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	accounts, err := ssoStore.GetUserLinkedAccounts(user.ID)
	if err != nil {
		http.Error(w, `{"error":"failed to get linked accounts"}`, http.StatusInternalServerError)
		return
	}

	if accounts == nil {
		accounts = []sso.LinkedAccount{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// handleUnlinkAccount removes a linked account
func handleUnlinkAccount(w http.ResponseWriter, r *http.Request) {
	if ssoStore == nil {
		http.Error(w, `{"error":"sso not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Extract account ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/auth/linked/")
	accountID := strings.Split(path, "/")[0]

	// Verify ownership
	accounts, _ := ssoStore.GetUserLinkedAccounts(user.ID)
	found := false
	for _, a := range accounts {
		if a.ID == accountID {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, `{"error":"account not found"}`, http.StatusNotFound)
		return
	}

	if err := ssoStore.DeleteLinkedAccount(accountID); err != nil {
		http.Error(w, `{"error":"failed to unlink account"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleSSOConfig handles SSO configuration for an organization
func handleSSOConfig(w http.ResponseWriter, r *http.Request) {
	if ssoStore == nil {
		http.Error(w, `{"error":"sso not configured"}`, http.StatusServiceUnavailable)
		return
	}

	user := getUserFromRequest(r)
	if user == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	// Only admins can manage SSO config
	if user.Role != rbac.RoleAdmin && user.Role != rbac.RoleOwner {
		http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		config, err := ssoStore.GetOrgSSOConfig(user.OrgID)
		if err != nil {
			http.Error(w, `{"error":"failed to get config"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	case http.MethodPut:
		var config sso.OrgSSOConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
			return
		}

		config.OrgID = user.OrgID

		// If SAML is being enabled, configure the SAML manager
		if config.SAMLEnabled && config.SAMLConfig != nil {
			if err := samlManager.ConfigureOrg(user.OrgID, config.SAMLConfig); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"invalid saml config: %s"}`, err), http.StatusBadRequest)
				return
			}
		} else if !config.SAMLEnabled {
			samlManager.RemoveConfig(user.OrgID)
		}

		if err := ssoStore.SaveOrgSSOConfig(&config); err != nil {
			http.Error(w, `{"error":"failed to save config"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// setAuthCookies sets authentication cookies
func setAuthCookies(w http.ResponseWriter, r *http.Request, tokenPair *sso.TokenPair) {
	secure := r.TLS != nil

	// Access token cookie (short-lived, httpOnly)
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    tokenPair.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   tokenPair.ExpiresIn,
	})

	// Refresh token cookie (long-lived, httpOnly)
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokenPair.RefreshToken,
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   7 * 24 * 60 * 60, // 7 days
	})

	// Also set the session_token for backward compatibility
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    tokenPair.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   tokenPair.ExpiresIn,
	})
}

// clearAuthCookies clears authentication cookies
func clearAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{"access_token", "refresh_token", "session_token"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
	}
}
