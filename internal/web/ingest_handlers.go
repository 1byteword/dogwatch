package web

import (
	"net/http"

	"dogwatch/internal/custommetrics"
	"dogwatch/internal/ingest"
)

var ingestHandlers *ingest.Handlers

// SetIngestStore initializes the ingestion handlers with a metrics store
func SetIngestStore(store *custommetrics.Store) {
	adapter := ingest.NewCustomMetricsAdapter(store)
	ingestHandlers = ingest.NewHandlers(adapter)
}

// RegisterIngestRoutes registers all multi-protocol ingestion routes
func RegisterIngestRoutes(mux *http.ServeMux) {
	if ingestHandlers == nil {
		return
	}

	// Graphite endpoints
	mux.HandleFunc("/api/graphite/write", withIngestCORS(ingestHandlers.HandleGraphitePlaintext))
	mux.HandleFunc("/api/graphite/pickle", withIngestCORS(ingestHandlers.HandleGraphitePickle))

	// InfluxDB endpoints
	mux.HandleFunc("/api/influx/write", withIngestCORS(ingestHandlers.HandleInfluxWrite))
	mux.HandleFunc("/api/influx/query", withIngestCORS(ingestHandlers.HandleInfluxQuery))
	mux.HandleFunc("/write", withIngestCORS(ingestHandlers.HandleInfluxWrite))          // Standard InfluxDB path
	mux.HandleFunc("/api/v2/write", withIngestCORS(ingestHandlers.HandleInfluxWrite))   // InfluxDB 2.x path

	// OpenTSDB endpoints
	mux.HandleFunc("/api/opentsdb/put", withIngestCORS(ingestHandlers.HandleOpenTSDBPut))
	mux.HandleFunc("/api/opentsdb/telnet", withIngestCORS(ingestHandlers.HandleOpenTSDBTelnet))
	mux.HandleFunc("/api/put", withIngestCORS(ingestHandlers.HandleOpenTSDBPut)) // Standard OpenTSDB path

	// StatsD endpoint (HTTP bridge)
	mux.HandleFunc("/api/statsd/write", withIngestCORS(ingestHandlers.HandleStatsD))

	// DataDog endpoints
	mux.HandleFunc("/api/datadog/v1/series", withIngestCORS(ingestHandlers.HandleDataDogV1Series))
	mux.HandleFunc("/api/datadog/v2/series", withIngestCORS(ingestHandlers.HandleDataDogV2Series))
	mux.HandleFunc("/api/datadog/v1/check_run", withIngestCORS(ingestHandlers.HandleDataDogCheckRun))
	mux.HandleFunc("/api/datadog/v1/distribution_points", withIngestCORS(ingestHandlers.HandleDataDogDistribution))
	mux.HandleFunc("/api/datadog/intake", withIngestCORS(ingestHandlers.HandleDataDogIntake))
	mux.HandleFunc("/api/datadog/v1/validate", withIngestCORS(ingestHandlers.HandleDataDogValidate))

	// Note: Standard DataDog paths (/api/v1/series etc.) are handled in handlers.go
	// with wrapping functions that check for DD-API-KEY header
}

// withIngestCORS adds CORS headers for ingestion endpoints
func withIngestCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow from any origin for metric ingestion
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, DD-API-KEY, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}
