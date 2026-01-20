package rbac

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Context keys for storing auth info
type contextKey string

const (
	ContextKeyUser    contextKey = "user"
	ContextKeySession contextKey = "session"
	ContextKeyAPIKey  contextKey = "apikey"
	ContextKeyOrgID   contextKey = "org_id"
)

// Middleware handles authentication and authorization
type Middleware struct {
	auth *Auth
}

// NewMiddleware creates a new auth middleware
func NewMiddleware(auth *Auth) *Middleware {
	return &Middleware{auth: auth}
}

// Authenticate validates the request and adds user info to context
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, session, apiKey, err := m.extractAuth(r)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// Add to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, ContextKeyUser, user)
		ctx = context.WithValue(ctx, ContextKeyOrgID, user.OrgID)

		if session != nil {
			ctx = context.WithValue(ctx, ContextKeySession, session)
		}
		if apiKey != nil {
			ctx = context.WithValue(ctx, ContextKeyAPIKey, apiKey)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AuthenticateOptional tries to authenticate but allows anonymous access
func (m *Middleware) AuthenticateOptional(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, session, apiKey, err := m.extractAuth(r)
		if err == nil && user != nil {
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyUser, user)
			ctx = context.WithValue(ctx, ContextKeyOrgID, user.OrgID)

			if session != nil {
				ctx = context.WithValue(ctx, ContextKeySession, session)
			}
			if apiKey != nil {
				ctx = context.WithValue(ctx, ContextKeyAPIKey, apiKey)
			}

			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

// RequirePermission checks if the user has a specific permission
func (m *Middleware) RequirePermission(resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Check if using API key - use API key permissions
			apiKey := GetAPIKeyFromContext(r.Context())
			if apiKey != nil {
				if !m.auth.CheckAPIKeyPermission(apiKey, resource, action) {
					http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
					return
				}
			} else {
				// Use role-based permissions
				if !m.auth.CheckPermission(user, resource, action) {
					http.Error(w, `{"error":"permission denied"}`, http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole checks if the user has at least the specified role
func (m *Middleware) RequireRole(minRole Role) func(http.Handler) http.Handler {
	roleHierarchy := map[Role]int{
		RoleViewer: 1,
		RoleEditor: 2,
		RoleAdmin:  3,
		RoleOwner:  4,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r.Context())
			if user == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			if roleHierarchy[user.Role] < roleHierarchy[minRole] {
				http.Error(w, `{"error":"insufficient role"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwner ensures only org owners can access
func (m *Middleware) RequireOwner(next http.Handler) http.Handler {
	return m.RequireRole(RoleOwner)(next)
}

// RequireAdmin ensures only admins or above can access
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return m.RequireRole(RoleAdmin)(next)
}

// extractAuth extracts and validates authentication from the request
func (m *Middleware) extractAuth(r *http.Request) (*User, *Session, *APIKey, error) {
	// Try Authorization header first
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		return m.validateAuthHeader(authHeader)
	}

	// Try X-API-Key header
	apiKeyHeader := r.Header.Get("X-API-Key")
	if apiKeyHeader != "" {
		apiKey, user, err := m.auth.ValidateAPIKey(apiKeyHeader)
		if err != nil {
			return nil, nil, nil, err
		}
		return user, nil, apiKey, nil
	}

	// Try cookie
	cookie, err := r.Cookie("session_token")
	if err == nil && cookie.Value != "" {
		user, session, err := m.auth.ValidateSession(cookie.Value)
		if err != nil {
			return nil, nil, nil, err
		}
		return user, session, nil, nil
	}

	return nil, nil, nil, ErrInvalidToken
}

// validateAuthHeader validates Bearer token or API key from Authorization header
func (m *Middleware) validateAuthHeader(header string) (*User, *Session, *APIKey, error) {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, nil, nil, ErrInvalidToken
	}

	scheme := strings.ToLower(parts[0])
	token := parts[1]

	switch scheme {
	case "bearer":
		user, session, err := m.auth.ValidateSession(token)
		if err != nil {
			return nil, nil, nil, err
		}
		return user, session, nil, nil

	case "apikey":
		apiKey, user, err := m.auth.ValidateAPIKey(token)
		if err != nil {
			return nil, nil, nil, err
		}
		return user, nil, apiKey, nil

	default:
		// Try as API key if it starts with dw_
		if strings.HasPrefix(token, "dw_") {
			apiKey, user, err := m.auth.ValidateAPIKey(token)
			if err != nil {
				return nil, nil, nil, err
			}
			return user, nil, apiKey, nil
		}
		return nil, nil, nil, ErrInvalidToken
	}
}

// Helper functions to extract from context

// GetUserFromContext returns the authenticated user from context
func GetUserFromContext(ctx context.Context) *User {
	user, ok := ctx.Value(ContextKeyUser).(*User)
	if !ok {
		return nil
	}
	return user
}

// GetSessionFromContext returns the session from context
func GetSessionFromContext(ctx context.Context) *Session {
	session, ok := ctx.Value(ContextKeySession).(*Session)
	if !ok {
		return nil
	}
	return session
}

// GetAPIKeyFromContext returns the API key from context
func GetAPIKeyFromContext(ctx context.Context) *APIKey {
	apiKey, ok := ctx.Value(ContextKeyAPIKey).(*APIKey)
	if !ok {
		return nil
	}
	return apiKey
}

// GetOrgIDFromContext returns the organization ID from context
func GetOrgIDFromContext(ctx context.Context) string {
	orgID, ok := ctx.Value(ContextKeyOrgID).(string)
	if !ok {
		return ""
	}
	return orgID
}

// MustGetUser returns the user or an error if not authenticated
// Prefer this over GetUserFromContext when authentication is required
func MustGetUser(ctx context.Context) (*User, error) {
	user := GetUserFromContext(ctx)
	if user == nil {
		return nil, ErrInvalidToken
	}
	return user, nil
}

// CORS adds CORS headers for API requests
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit provides basic rate limiting (should use redis in production)
type RateLimiter struct {
	mu         sync.RWMutex
	requests   map[string][]int64
	limit      int
	window     int64 // seconds
	maxEntries int   // max unique keys to track (memory bound)
}

// NewRateLimiter creates a rate limiter
func NewRateLimiter(limit int, windowSeconds int64) *RateLimiter {
	rl := &RateLimiter{
		requests:   make(map[string][]int64),
		limit:      limit,
		window:     windowSeconds,
		maxEntries: 10000, // prevent memory exhaustion
	}

	// Start background cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// cleanupLoop periodically removes stale entries
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Duration(rl.window) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup removes entries with no recent requests
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().Unix()
	cutoff := now - rl.window

	for key, timestamps := range rl.requests {
		var recent []int64
		for _, ts := range timestamps {
			if ts > cutoff {
				recent = append(recent, ts)
			}
		}
		if len(recent) == 0 {
			delete(rl.requests, key)
		} else {
			rl.requests[key] = recent
		}
	}
}

// Limit applies rate limiting middleware
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use API key or IP as identifier
		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.RemoteAddr
		}

		now := time.Now().Unix()
		cutoff := now - rl.window

		rl.mu.Lock()

		// Check if we're at max entries and this is a new key
		if _, exists := rl.requests[key]; !exists && len(rl.requests) >= rl.maxEntries {
			rl.mu.Unlock()
			// At capacity - reject new clients to prevent memory exhaustion
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		// Clean old entries and count recent
		var recent []int64
		for _, ts := range rl.requests[key] {
			if ts > cutoff {
				recent = append(recent, ts)
			}
		}

		if len(recent) >= rl.limit {
			rl.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		recent = append(recent, now)
		rl.requests[key] = recent
		rl.mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
