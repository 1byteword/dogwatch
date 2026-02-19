package web

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	buckets    map[string]*tokenBucket
	mu         sync.RWMutex
	rate       int           // Requests per window
	window     time.Duration // Time window
	burstSize  int           // Max burst size
	cleanupInt time.Duration // Cleanup interval
	stopCh     chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	mu         sync.Mutex
}

// RateLimitConfig holds rate limiter configuration
type RateLimitConfig struct {
	RequestsPerMinute int           // Default requests per minute
	BurstSize         int           // Max burst allowed
	Window            time.Duration // Time window for rate calculation
	SkipPaths         []string      // Paths to skip (e.g., /health)
	SkipIPs           []string      // IPs to skip (e.g., localhost)
}

// DefaultRateLimitConfig returns sensible defaults
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		RequestsPerMinute: 300,          // 5 req/sec average
		BurstSize:         50,           // Allow bursts up to 50
		Window:            time.Minute,
		SkipPaths:         []string{"/health", "/ready", "/metrics"},
		SkipIPs:           []string{"127.0.0.1", "::1"},
	}
}

func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	rl := &RateLimiter{
		buckets:    make(map[string]*tokenBucket),
		rate:       config.RequestsPerMinute,
		window:     config.Window,
		burstSize:  config.BurstSize,
		cleanupInt: 5 * time.Minute,
		stopCh:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	rl.mu.Lock()
	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     float64(rl.burstSize),
			lastUpdate: time.Now(),
		}
		rl.buckets[key] = bucket
	}
	rl.mu.Unlock()

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(bucket.lastUpdate)
	bucket.lastUpdate = now

	// Add tokens based on elapsed time
	tokensToAdd := elapsed.Seconds() * (float64(rl.rate) / rl.window.Seconds())
	bucket.tokens += tokensToAdd
	if bucket.tokens > float64(rl.burstSize) {
		bucket.tokens = float64(rl.burstSize)
	}

	// Calculate reset time
	resetAt = now.Add(rl.window)

	if bucket.tokens >= 1 {
		bucket.tokens--
		return true, int(bucket.tokens), resetAt
	}

	// Calculate time until one token is available
	timeUntilToken := time.Duration((1.0 - bucket.tokens) / (float64(rl.rate) / rl.window.Seconds()) * float64(time.Second))
	resetAt = now.Add(timeUntilToken)

	return false, 0, resetAt
}

// cleanup periodically removes old buckets
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInt)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, bucket := range rl.buckets {
				bucket.mu.Lock()
				// Remove buckets that haven't been used in 2x the window
				if now.Sub(bucket.lastUpdate) > 2*rl.window {
					delete(rl.buckets, key)
				}
				bucket.mu.Unlock()
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// RateLimitMiddleware creates HTTP middleware for rate limiting
func RateLimitMiddleware(config *RateLimitConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultRateLimitConfig()
	}
	limiter := NewRateLimiter(config)
	return RateLimitMiddlewareWith(limiter, config)
}

// RateLimitMiddlewareWith creates rate limiting middleware using an existing limiter,
// allowing the caller to retain a reference for lifecycle management (e.g., Stop).
func RateLimitMiddlewareWith(limiter *RateLimiter, config *RateLimitConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	// Build skip paths map for O(1) lookup
	skipPaths := make(map[string]bool)
	for _, path := range config.SkipPaths {
		skipPaths[path] = true
	}

	// Build skip IPs map
	skipIPs := make(map[string]bool)
	for _, ip := range config.SkipIPs {
		skipIPs[ip] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip certain paths
			if skipPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Get client IP
			ip := getClientIP(r)

			// Skip certain IPs
			if skipIPs[ip] {
				next.ServeHTTP(w, r)
				return
			}

			// Check rate limit
			allowed, remaining, resetAt := limiter.Allow(ip)

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.RequestsPerMinute))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if !allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(time.Until(resetAt).Seconds())+1, 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":       "rate limit exceeded",
					"retry_after": int(time.Until(resetAt).Seconds()) + 1,
					"limit":       config.RequestsPerMinute,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the real client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// APIKeyRateLimiter provides separate limits for API key vs IP-based access
type APIKeyRateLimiter struct {
	ipLimiter     *RateLimiter
	apiKeyLimiter *RateLimiter
}

// NewAPIKeyRateLimiter creates a rate limiter with different limits for API keys
func NewAPIKeyRateLimiter(ipConfig, apiKeyConfig *RateLimitConfig) *APIKeyRateLimiter {
	if ipConfig == nil {
		ipConfig = DefaultRateLimitConfig()
	}
	if apiKeyConfig == nil {
		// API keys get higher limits
		apiKeyConfig = &RateLimitConfig{
			RequestsPerMinute: 1000, // Higher limit for API keys
			BurstSize:         100,
			Window:            time.Minute,
		}
	}

	return &APIKeyRateLimiter{
		ipLimiter:     NewRateLimiter(ipConfig),
		apiKeyLimiter: NewRateLimiter(apiKeyConfig),
	}
}

// Allow checks rate limit based on whether request has API key
func (rl *APIKeyRateLimiter) Allow(r *http.Request) (allowed bool, remaining int, resetAt time.Time, limit int) {
	// Check for API key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "apikey ") {
			apiKey = strings.TrimPrefix(auth, "apikey ")
			apiKey = strings.TrimPrefix(apiKey, "ApiKey ")
		}
	}

	if apiKey != "" {
		// Use API key as the rate limit key with higher limits
		allowed, remaining, resetAt = rl.apiKeyLimiter.Allow("apikey:" + apiKey)
		return allowed, remaining, resetAt, 1000
	}

	// Fall back to IP-based limiting
	ip := getClientIP(r)
	allowed, remaining, resetAt = rl.ipLimiter.Allow(ip)
	return allowed, remaining, resetAt, 300
}

// Stop stops both limiters
func (rl *APIKeyRateLimiter) Stop() {
	rl.ipLimiter.Stop()
	rl.apiKeyLimiter.Stop()
}

// EndpointRateLimiter allows different limits per endpoint
type EndpointRateLimiter struct {
	limiters map[string]*RateLimiter
	default_ *RateLimiter
	mu       sync.RWMutex
}

// EndpointLimit defines rate limit for a specific endpoint pattern
type EndpointLimit struct {
	Pattern           string // Path prefix to match
	RequestsPerMinute int
	BurstSize         int
}

// NewEndpointRateLimiter creates an endpoint-aware rate limiter
func NewEndpointRateLimiter(defaultConfig *RateLimitConfig, endpoints []EndpointLimit) *EndpointRateLimiter {
	erl := &EndpointRateLimiter{
		limiters: make(map[string]*RateLimiter),
		default_: NewRateLimiter(defaultConfig),
	}

	for _, ep := range endpoints {
		erl.limiters[ep.Pattern] = NewRateLimiter(&RateLimitConfig{
			RequestsPerMinute: ep.RequestsPerMinute,
			BurstSize:         ep.BurstSize,
			Window:            time.Minute,
		})
	}

	return erl
}

// GetLimiter returns the appropriate limiter for a path
func (erl *EndpointRateLimiter) GetLimiter(path string) *RateLimiter {
	erl.mu.RLock()
	defer erl.mu.RUnlock()

	// Find matching endpoint (longest prefix match)
	var bestMatch string
	var bestLimiter *RateLimiter
	for pattern, limiter := range erl.limiters {
		if strings.HasPrefix(path, pattern) && len(pattern) > len(bestMatch) {
			bestMatch = pattern
			bestLimiter = limiter
		}
	}

	if bestLimiter != nil {
		return bestLimiter
	}
	return erl.default_
}

// Stop stops all limiters
func (erl *EndpointRateLimiter) Stop() {
	erl.mu.Lock()
	defer erl.mu.Unlock()

	erl.default_.Stop()
	for _, limiter := range erl.limiters {
		limiter.Stop()
	}
}
