package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"dogwatch/internal/dbwatch"
)

var dbwatchStore *dbwatch.Store

// SetDBWatchStore sets the database watch store
func SetDBWatchStore(store *dbwatch.Store) {
	dbwatchStore = store
}

// RegisterDBWatchRoutes registers database watch routes
func RegisterDBWatchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dbwatch/queries", handleDBQueries)
	mux.HandleFunc("/api/dbwatch/slow", handleDBSlowQueries)
	mux.HandleFunc("/api/dbwatch/stats", handleDBStats)
	mux.HandleFunc("/api/dbwatch/query-stats", handleDBQueryStats)
	mux.HandleFunc("/api/dbwatch/operations", handleDBOperations)
	mux.HandleFunc("/api/dbwatch/tables", handleDBTables)
}

// handleDBQueries returns recent database queries
func handleDBQueries(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	dbType := dbwatch.DBType(r.URL.Query().Get("db_type"))
	limitStr := r.URL.Query().Get("limit")
	sinceStr := r.URL.Query().Get("since")

	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	queries, err := dbwatchStore.GetRecentQueries(dbType, limit, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// handleDBSlowQueries returns slow database queries
func handleDBSlowQueries(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	dbType := dbwatch.DBType(r.URL.Query().Get("db_type"))
	limitStr := r.URL.Query().Get("limit")
	sinceStr := r.URL.Query().Get("since")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	queries, err := dbwatchStore.GetSlowQueries(dbType, limit, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(queries)
}

// handleDBStats returns per-database statistics
func handleDBStats(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	stats, err := dbwatchStore.GetDBStats(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDBQueryStats returns aggregated query statistics
func handleDBQueryStats(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	dbType := dbwatch.DBType(r.URL.Query().Get("db_type"))
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	stats, err := dbwatchStore.GetQueryStats(dbType, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDBOperations returns query counts by operation
func handleDBOperations(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	dbType := dbwatch.DBType(r.URL.Query().Get("db_type"))
	sinceStr := r.URL.Query().Get("since")

	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	breakdown, err := dbwatchStore.GetOperationBreakdown(dbType, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(breakdown)
}

// handleDBTables returns query counts by table
func handleDBTables(w http.ResponseWriter, r *http.Request) {
	if dbwatchStore == nil {
		http.Error(w, "Database watch not configured", http.StatusServiceUnavailable)
		return
	}

	dbType := dbwatch.DBType(r.URL.Query().Get("db_type"))
	sinceStr := r.URL.Query().Get("since")

	since := time.Hour
	if sinceStr != "" {
		if d, err := time.ParseDuration(sinceStr); err == nil {
			since = d
		}
	}

	breakdown, err := dbwatchStore.GetTableBreakdown(dbType, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(breakdown)
}
