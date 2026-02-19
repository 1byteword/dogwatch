package web

import (
	"log"
	"net/http"
	"runtime/debug"
)

// RecoveryMiddleware catches panics in HTTP handlers, logs the stack trace,
// and returns a 500 Internal Server Error instead of crashing the process.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				stack := debug.Stack()
				log.Printf("[PANIC] %s %s: %v\n%s", r.Method, r.URL.Path, err, stack)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
