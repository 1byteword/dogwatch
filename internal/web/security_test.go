package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Wrap with security middleware
	config := DefaultSecurityConfig()
	secured := SecurityHeadersOnly(config)(handler)

	// Make a request
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	secured.ServeHTTP(rec, req)

	// Check headers
	tests := []struct {
		header   string
		expected string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"X-XSS-Protection", "1; mode=block"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := rec.Header().Get(tt.header)
			if got != tt.expected {
				t.Errorf("Header %s = %q, want %q", tt.header, got, tt.expected)
			}
		})
	}

	// Check CSP header exists
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header not set")
	}
}

func TestCSRFTokenStore(t *testing.T) {
	store := NewCSRFTokenStore(1 * 60 * 1000000000) // 1 minute TTL

	// Generate token
	token := store.Generate()
	if token == "" {
		t.Error("Generated token is empty")
	}

	// Validate token
	if !store.Validate(token) {
		t.Error("Valid token not accepted")
	}

	// Invalidate token
	store.Invalidate(token)
	if store.Validate(token) {
		t.Error("Invalidated token still valid")
	}

	// Test invalid token
	if store.Validate("invalid-token") {
		t.Error("Invalid token accepted")
	}
}

func TestBuildCSP(t *testing.T) {
	directives := map[string][]string{
		"default-src": {"'self'"},
		"script-src":  {"'self'", "'unsafe-inline'"},
	}

	csp := buildCSP(directives)

	// Should contain both directives
	if csp == "" {
		t.Error("CSP is empty")
	}
}

func TestSecurityMiddlewareAPIKeySkip(t *testing.T) {
	// Create a handler that sets a test header
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := DefaultSecurityConfig()
	secured := SecurityMiddleware(config)(handler)

	// Test that API key requests skip CSRF check for POST
	req := httptest.NewRequest("POST", "/api/metrics/push", nil)
	req.Header.Set("X-API-Key", "test-key")
	rec := httptest.NewRecorder()
	secured.ServeHTTP(rec, req)

	// Should not get 403 Forbidden for missing CSRF token
	if rec.Code == http.StatusForbidden {
		t.Error("API key request should skip CSRF check")
	}
}

func TestSecurityMiddlewareSkipPaths(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	config := DefaultSecurityConfig()
	secured := SecurityMiddleware(config)(handler)

	// Test health endpoint skips CSRF
	req := httptest.NewRequest("POST", "/health", nil)
	rec := httptest.NewRecorder()
	secured.ServeHTTP(rec, req)

	// Should not get 403 for skip paths
	if rec.Code == http.StatusForbidden {
		t.Error("Skip path should not require CSRF token")
	}
}

func TestHasAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		value    string
		expected bool
	}{
		{"X-API-Key header", "X-API-Key", "test-key", true},
		{"ApiKey auth", "Authorization", "ApiKey test-key", true},
		{"Bearer dw_ token", "Authorization", "Bearer dw_test123", true},
		{"No auth", "", "", false},
		{"Regular bearer", "Authorization", "Bearer regular-token", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}
			got := hasAPIKeyAuth(req)
			if got != tt.expected {
				t.Errorf("hasAPIKeyAuth() = %v, want %v", got, tt.expected)
			}
		})
	}
}
