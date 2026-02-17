package web

import (
	"net/http"
	"strings"

	"dogwatch/internal/rbac"
)

// publicPrefixes are path prefixes that never require authentication.
var publicPrefixes = []string{
	"/health",
	"/ready",
	"/healthz",
	"/readyz",
	"/livez",
	"/app/",
	"/login",
	"/assets/",
	"/favicon.",
}

// publicExact are exact paths that never require authentication.
var publicExact = map[string]bool{
	"/":                       true,
	"/api/health":             true,
	"/api/ready":              true,
	"/api/auth/login":         true,
	"/api/auth/logout":        true,
	"/api/auth/invite/accept": true,
}

// ingestPaths use API-key-or-session auth (AuthenticateOptional) —
// they must accept unauthenticated writes for backward compatibility
// with agents that don't yet send credentials.
var ingestPaths = map[string]bool{
	"/v1/traces":       true,
	"/v1/trace":        true,
	"/v1/metrics":      true,
	"/api/v1/write":    true,
	"/api/logs/ingest": true,
}

// AuthMiddleware wraps an http.Handler and enforces authentication on all
// /api/* routes except those explicitly listed as public or ingest.
// Static assets, SPA routes, and health checks are never gated.
func AuthMiddleware(mw *rbac.Middleware) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if mw == nil {
			// RBAC not configured — pass through (development mode)
			return next
		}

		authed := mw.Authenticate(next)
		optional := mw.AuthenticateOptional(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// 1. Exact public paths — no auth
			if publicExact[path] {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Public prefixes — no auth
			for _, prefix := range publicPrefixes {
				if strings.HasPrefix(path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 3. Ingest endpoints — optional auth (accept API key if present)
			if ingestPaths[path] {
				optional.ServeHTTP(w, r)
				return
			}

			// 4. WebSocket — optional auth (upgrade must happen first)
			if path == "/api/ws" {
				optional.ServeHTTP(w, r)
				return
			}

			// 5. SSO callback paths — must complete before user has a session
			if strings.HasPrefix(path, "/api/auth/sso/") ||
				strings.HasPrefix(path, "/api/auth/saml/") ||
				strings.HasPrefix(path, "/api/auth/oauth/") ||
				strings.HasPrefix(path, "/api/auth/providers") {
				next.ServeHTTP(w, r)
				return
			}

			// 6. All other /api/* routes — require authentication
			if strings.HasPrefix(path, "/api/") {
				authed.ServeHTTP(w, r)
				return
			}

			// 7. Non-API routes (status pages, etc.) — pass through
			next.ServeHTTP(w, r)
		})
	}
}
