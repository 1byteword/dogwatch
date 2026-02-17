package web

import (
	"net/http"
	"strings"
)

// BodyLimitMiddleware enforces a maximum request body size.
// Requests exceeding the limit receive a 413 Request Entity Too Large response.
func BodyLimitMiddleware(defaultLimit, ingestLimit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip body limiting for GET, HEAD, OPTIONS (no body)
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Ingest endpoints get a higher limit for batched telemetry data
			limit := defaultLimit
			path := r.URL.Path
			if strings.HasPrefix(path, "/v1/") ||
				path == "/api/logs/ingest" ||
				path == "/api/v1/write" ||
				strings.HasPrefix(path, "/api/graphite/") ||
				strings.HasPrefix(path, "/api/influx/") ||
				strings.HasPrefix(path, "/api/opentsdb/") ||
				strings.HasPrefix(path, "/api/statsd/") ||
				strings.HasPrefix(path, "/api/datadog/") {
				limit = ingestLimit
			}

			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
