package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SecurityConfig holds security middleware configuration
type SecurityConfig struct {
	// Content Security Policy
	CSPDirectives map[string][]string

	// Frame Options (DENY, SAMEORIGIN, or ALLOW-FROM uri)
	FrameOptions string

	// Enable XSS protection header
	XSSProtection bool

	// Enable content type sniffing protection
	NoSniff bool

	// Referrer Policy
	ReferrerPolicy string

	// CSRF Protection
	CSRFEnabled   bool
	CSRFTokenName string
	CSRFSecret    []byte

	// Paths to skip CSRF protection (e.g., webhooks, API endpoints with API key auth)
	CSRFSkipPaths []string

	// HTTP methods that require CSRF protection
	CSRFMethods []string
}

// DefaultSecurityConfig returns sensible security defaults
func DefaultSecurityConfig() *SecurityConfig {
	secret := make([]byte, 32)
	rand.Read(secret)

	return &SecurityConfig{
		CSPDirectives: map[string][]string{
			"default-src": {"'self'"},
			"script-src":  {"'self'", "'unsafe-inline'", "'unsafe-eval'"}, // Required for Chart.js, D3, GridStack
			"style-src":   {"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"},
			"font-src":    {"'self'", "https://fonts.gstatic.com"},
			"img-src":     {"'self'", "data:", "blob:"},
			"connect-src": {"'self'", "ws:", "wss:"},
			"frame-ancestors": {"'none'"},
		},
		FrameOptions:   "DENY",
		XSSProtection:  true,
		NoSniff:        true,
		ReferrerPolicy: "strict-origin-when-cross-origin",
		CSRFEnabled:    true,
		CSRFTokenName:  "X-CSRF-Token",
		CSRFSecret:     secret,
		CSRFSkipPaths: []string{
			"/health",
			"/ready",
			"/healthz",
			"/readyz",
			"/livez",
			"/api/health",
			"/api/ready",
			"/v1/traces",      // OTLP endpoint
			"/v1/metrics",     // OTLP metrics endpoint
			"/api/v1/write",   // Prometheus remote write
			"/api/logs/ingest", // Log ingestion
		},
		CSRFMethods: []string{"POST", "PUT", "DELETE", "PATCH"},
	}
}

// CSRFTokenStore manages CSRF tokens
type CSRFTokenStore struct {
	tokens map[string]time.Time
	mu     sync.RWMutex
	ttl    time.Duration
}

// NewCSRFTokenStore creates a new token store
func NewCSRFTokenStore(ttl time.Duration) *CSRFTokenStore {
	store := &CSRFTokenStore{
		tokens: make(map[string]time.Time),
		ttl:    ttl,
	}
	go store.cleanup()
	return store
}

// Generate creates a new CSRF token
func (s *CSRFTokenStore) Generate() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := base64.URLEncoding.EncodeToString(b)

	s.mu.Lock()
	s.tokens[token] = time.Now().Add(s.ttl)
	s.mu.Unlock()

	return token
}

// Validate checks if a token is valid
func (s *CSRFTokenStore) Validate(token string) bool {
	s.mu.RLock()
	expiry, exists := s.tokens[token]
	s.mu.RUnlock()

	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
		return false
	}

	return true
}

// Invalidate removes a token (for single-use tokens)
func (s *CSRFTokenStore) Invalidate(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// cleanup periodically removes expired tokens
func (s *CSRFTokenStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, expiry := range s.tokens {
			if now.After(expiry) {
				delete(s.tokens, token)
			}
		}
		s.mu.Unlock()
	}
}

// buildCSP builds the Content-Security-Policy header value
func buildCSP(directives map[string][]string) string {
	var parts []string
	for directive, values := range directives {
		parts = append(parts, directive+" "+strings.Join(values, " "))
	}
	return strings.Join(parts, "; ")
}

// SecurityMiddleware creates HTTP middleware for security headers and CSRF protection
func SecurityMiddleware(config *SecurityConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	csp := buildCSP(config.CSPDirectives)

	// Build skip paths map for O(1) lookup
	skipPaths := make(map[string]bool)
	for _, path := range config.CSRFSkipPaths {
		skipPaths[path] = true
	}

	// Build CSRF methods map
	csrfMethods := make(map[string]bool)
	for _, method := range config.CSRFMethods {
		csrfMethods[method] = true
	}

	// Create token store with 4-hour TTL
	tokenStore := NewCSRFTokenStore(4 * time.Hour)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set security headers on all responses
			w.Header().Set("Content-Security-Policy", csp)
			w.Header().Set("X-Frame-Options", config.FrameOptions)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", config.ReferrerPolicy)

			if config.XSSProtection {
				w.Header().Set("X-XSS-Protection", "1; mode=block")
			}

			// Skip CSRF for certain paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Skip CSRF for paths with API key authentication
			if hasAPIKeyAuth(r) {
				next.ServeHTTP(w, r)
				return
			}

			// CSRF protection for state-changing methods
			if config.CSRFEnabled && csrfMethods[r.Method] {
				token := r.Header.Get(config.CSRFTokenName)
				if token == "" {
					// Also check form value
					token = r.FormValue("csrf_token")
				}

				if token == "" || !tokenStore.Validate(token) {
					http.Error(w, `{"error": "invalid or missing CSRF token"}`, http.StatusForbidden)
					return
				}
			}

			// For GET requests to HTML pages, set a CSRF token cookie
			if r.Method == "GET" && isHTMLRequest(r) {
				token := tokenStore.Generate()
				http.SetCookie(w, &http.Cookie{
					Name:     "csrf_token",
					Value:    token,
					Path:     "/",
					HttpOnly: false, // JS needs to read this
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteStrictMode,
					MaxAge:   14400, // 4 hours
				})
			}

			next.ServeHTTP(w, r)
		})
	}
}

// hasAPIKeyAuth checks if the request has API key authentication
func hasAPIKeyAuth(r *http.Request) bool {
	if r.Header.Get("X-API-Key") != "" {
		return true
	}
	if auth := r.Header.Get("Authorization"); auth != "" {
		authLower := strings.ToLower(auth)
		if strings.HasPrefix(authLower, "apikey ") || strings.HasPrefix(authLower, "bearer dw_") {
			return true
		}
	}
	return false
}

// isHTMLRequest checks if the request is for an HTML page
func isHTMLRequest(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	path := r.URL.Path

	// Root path or paths ending with .html
	if path == "/" || strings.HasSuffix(path, ".html") {
		return true
	}

	// Accepts HTML
	if strings.Contains(accept, "text/html") {
		// Exclude API requests
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/v1/") {
			return true
		}
	}

	return false
}

// GetCSRFToken is a helper to get the CSRF token from the request cookie
func GetCSRFToken(r *http.Request) string {
	cookie, err := r.Cookie("csrf_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}

// SecurityHeadersOnly creates a simpler middleware that only adds security headers without CSRF
// Useful when CSRF is handled elsewhere or not needed
func SecurityHeadersOnly(config *SecurityConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	csp := buildCSP(config.CSPDirectives)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", csp)
			w.Header().Set("X-Frame-Options", config.FrameOptions)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", config.ReferrerPolicy)

			if config.XSSProtection {
				w.Header().Set("X-XSS-Protection", "1; mode=block")
			}

			next.ServeHTTP(w, r)
		})
	}
}
