// Package middleware provides APM instrumentation middleware for common frameworks
package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"dogwatch/pkg/apm"
)

// HTTPOptions configures HTTP middleware behavior
type HTTPOptions struct {
	// ServiceName overrides the agent's service name for this handler
	ServiceName string

	// IgnorePaths are URL paths to skip tracing (e.g., /health, /metrics)
	IgnorePaths []string

	// ResourceNamer customizes the resource name for a request
	ResourceNamer func(r *http.Request) string

	// SpanNamer customizes the span operation name
	SpanNamer func(r *http.Request) string

	// ErrorHandler is called when an error occurs
	ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

	// Tags are added to all spans
	Tags map[string]string
}

// DefaultHTTPOptions returns default options
func DefaultHTTPOptions() HTTPOptions {
	return HTTPOptions{
		IgnorePaths: []string{"/health", "/healthz", "/ready", "/readyz", "/metrics", "/favicon.ico"},
		ResourceNamer: func(r *http.Request) string {
			return r.Method + " " + r.URL.Path
		},
		SpanNamer: func(r *http.Request) string {
			return "http.request"
		},
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int64
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += int64(n)
	return n, err
}

// HTTP returns middleware for net/http handlers
func HTTP(opts ...HTTPOptions) func(http.Handler) http.Handler {
	opt := DefaultHTTPOptions()
	if len(opts) > 0 {
		opt = mergeHTTPOptions(opt, opts[0])
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip ignored paths
			for _, path := range opt.IgnorePaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Create span
			spanName := opt.SpanNamer(r)
			resource := opt.ResourceNamer(r)

			span, ctx := apm.StartSpanFromContext(r.Context(), spanName,
				apm.WithSpanType(apm.SpanTypeWeb),
				apm.WithResource(resource),
				apm.WithTag(apm.TagSpanKind, apm.SpanKindServer),
			)

			// Add HTTP tags
			span.SetTag(apm.TagHTTPMethod, r.Method)
			span.SetTag(apm.TagHTTPURL, r.URL.String())
			span.SetTag(apm.TagHTTPPath, r.URL.Path)
			span.SetTag(apm.TagHTTPHost, r.Host)

			if ua := r.UserAgent(); ua != "" {
				span.SetTag(apm.TagHTTPUserAgent, ua)
			}

			// Add custom tags
			for k, v := range opt.Tags {
				span.SetTag(k, v)
			}

			// Propagate trace context in response headers
			w.Header().Set("X-Trace-ID", span.TraceID)
			w.Header().Set("X-Span-ID", span.SpanID)

			// Wrap response writer
			rw := newResponseWriter(w)

			// Call next handler
			start := time.Now()
			next.ServeHTTP(rw, r.WithContext(ctx))
			duration := time.Since(start)

			// Record status and metrics
			span.SetTag(apm.TagHTTPStatusCode, fmt.Sprintf("%d", rw.statusCode))
			span.SetMetric("http.response_size", float64(rw.written))
			span.SetMetric("http.duration_ms", float64(duration.Milliseconds()))

			// Mark as error if 5xx
			if rw.statusCode >= 500 {
				span.SetError(fmt.Errorf("HTTP %d", rw.statusCode))
			}

			span.Finish()

			// Record metrics
			tags := map[string]string{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": fmt.Sprintf("%d", rw.statusCode),
			}
			apm.RecordHistogram("http.request.duration_ms", float64(duration.Milliseconds()), tags)
			apm.RecordCounter("http.request.count", 1, tags)
			if rw.statusCode >= 400 {
				apm.RecordCounter("http.request.errors", 1, tags)
			}
		})
	}
}

// HTTPHandler wraps an http.Handler with APM instrumentation
func HTTPHandler(handler http.Handler, opts ...HTTPOptions) http.Handler {
	return HTTP(opts...)(handler)
}

// HTTPFunc wraps an http.HandlerFunc with APM instrumentation
func HTTPFunc(handler http.HandlerFunc, opts ...HTTPOptions) http.HandlerFunc {
	wrapped := HTTP(opts...)(handler)
	return func(w http.ResponseWriter, r *http.Request) {
		wrapped.ServeHTTP(w, r)
	}
}

func mergeHTTPOptions(base, override HTTPOptions) HTTPOptions {
	if override.ServiceName != "" {
		base.ServiceName = override.ServiceName
	}
	if override.IgnorePaths != nil {
		base.IgnorePaths = append(base.IgnorePaths, override.IgnorePaths...)
	}
	if override.ResourceNamer != nil {
		base.ResourceNamer = override.ResourceNamer
	}
	if override.SpanNamer != nil {
		base.SpanNamer = override.SpanNamer
	}
	if override.ErrorHandler != nil {
		base.ErrorHandler = override.ErrorHandler
	}
	if override.Tags != nil {
		if base.Tags == nil {
			base.Tags = make(map[string]string)
		}
		for k, v := range override.Tags {
			base.Tags[k] = v
		}
	}
	return base
}

// HTTPClient wraps an http.Client with APM instrumentation
type HTTPClient struct {
	client *http.Client
	opts   HTTPClientOptions
}

// HTTPClientOptions configures HTTP client instrumentation
type HTTPClientOptions struct {
	// ServiceName for the traced service
	ServiceName string

	// Tags to add to all spans
	Tags map[string]string
}

// WrapHTTPClient wraps an http.Client for tracing
func WrapHTTPClient(client *http.Client, opts ...HTTPClientOptions) *HTTPClient {
	if client == nil {
		client = http.DefaultClient
	}
	opt := HTTPClientOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &HTTPClient{client: client, opts: opt}
}

// Do executes an HTTP request with tracing
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	spanName := "http.client.request"
	resource := req.Method + " " + req.URL.Host + req.URL.Path

	span, ctx := apm.StartSpanFromContext(req.Context(), spanName,
		apm.WithSpanType(apm.SpanTypeHTTP),
		apm.WithResource(resource),
		apm.WithTag(apm.TagSpanKind, apm.SpanKindClient),
	)

	// Add HTTP tags
	span.SetTag(apm.TagHTTPMethod, req.Method)
	span.SetTag(apm.TagHTTPURL, req.URL.String())
	span.SetTag(apm.TagPeerHostname, req.URL.Host)

	// Add custom tags
	for k, v := range c.opts.Tags {
		span.SetTag(k, v)
	}

	// Inject trace context into request headers
	req.Header.Set("X-Trace-ID", span.TraceID)
	req.Header.Set("X-Parent-Span-ID", span.SpanID)

	// Execute request
	start := time.Now()
	resp, err := c.client.Do(req.WithContext(ctx))
	duration := time.Since(start)

	span.SetMetric("http.duration_ms", float64(duration.Milliseconds()))

	if err != nil {
		span.SetError(err)
		span.Finish()
		return nil, err
	}

	span.SetTag(apm.TagHTTPStatusCode, fmt.Sprintf("%d", resp.StatusCode))

	if resp.StatusCode >= 400 {
		span.SetError(fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	span.Finish()

	// Record metrics
	tags := map[string]string{
		"method":      req.Method,
		"host":        req.URL.Host,
		"status_code": fmt.Sprintf("%d", resp.StatusCode),
	}
	apm.RecordHistogram("http.client.duration_ms", float64(duration.Milliseconds()), tags)
	apm.RecordCounter("http.client.request.count", 1, tags)

	return resp, nil
}

// Get performs a GET request with tracing
func (c *HTTPClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request with tracing
func (c *HTTPClient) Post(url, contentType string, body interface{}) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}
