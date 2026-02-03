package ingest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Handlers provides HTTP handlers for multi-protocol metric ingestion
type Handlers struct {
	store          MetricStore
	graphiteParser *GraphiteParser
	influxParser   *InfluxParser
	opentsdbParser *OpenTSDBParser
	datadogParser  *DataDogParser
	statsdParser   *StatsDParser
}

// NewHandlers creates new ingestion handlers
func NewHandlers(store MetricStore) *Handlers {
	return &Handlers{
		store:          store,
		graphiteParser: &GraphiteParser{},
		influxParser:   &InfluxParser{DefaultPrecision: "ns"},
		opentsdbParser: &OpenTSDBParser{},
		datadogParser:  &DataDogParser{},
		statsdParser:   NewStatsDParser(),
	}
}

// HandleGraphitePlaintext handles Graphite plaintext protocol
// POST /api/graphite/write
func (h *Handlers) HandleGraphitePlaintext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := h.graphiteParser.ParsePlaintext(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGraphitePickle handles Graphite pickle protocol
// POST /api/graphite/pickle
func (h *Handlers) HandleGraphitePickle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := h.graphiteParser.ParsePickle(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleInfluxWrite handles InfluxDB line protocol
// POST /api/influx/write
// Query params: precision (ns, us, ms, s), db, rp
func (h *Handlers) HandleInfluxWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	precision := r.URL.Query().Get("precision")
	if precision == "" {
		precision = "ns"
	}

	batch, err := h.influxParser.ParseLineProtocol(r.Body, precision)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleInfluxQuery handles InfluxDB query endpoint (returns empty for compatibility)
// GET /api/influx/query
func (h *Handlers) HandleInfluxQuery(w http.ResponseWriter, r *http.Request) {
	// Return empty results for compatibility - actual queries use our query engine
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"results": []interface{}{},
	})
}

// HandleOpenTSDBPut handles OpenTSDB HTTP JSON format
// POST /api/opentsdb/put
func (h *Handlers) HandleOpenTSDBPut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := h.opentsdbParser.ParseHTTP(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// OpenTSDB returns details on success with ?details query param
	if r.URL.Query().Get("details") == "true" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": len(batch.Samples),
			"failed":  0,
			"errors":  []string{},
		})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleOpenTSDBTelnet handles OpenTSDB telnet-style input over HTTP
// POST /api/opentsdb/telnet
func (h *Handlers) HandleOpenTSDBTelnet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := h.opentsdbParser.ParseTelnet(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleDataDogV1Series handles DataDog /api/v1/series endpoint
// POST /api/datadog/v1/series
func (h *Handlers) HandleDataDogV1Series(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentEncoding := r.Header.Get("Content-Encoding")
	batch, err := h.datadogParser.ParseV1Series(r.Body, contentEncoding)
	if err != nil {
		writeDataDogError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		writeDataDogError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// DataDog expects JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// HandleDataDogV2Series handles DataDog /api/v2/series endpoint
// POST /api/datadog/v2/series
func (h *Handlers) HandleDataDogV2Series(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentEncoding := r.Header.Get("Content-Encoding")
	batch, err := h.datadogParser.ParseV2Series(r.Body, contentEncoding)
	if err != nil {
		writeDataDogError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		writeDataDogError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// HandleDataDogCheckRun handles DataDog /api/v1/check_run endpoint
// POST /api/datadog/v1/check_run
func (h *Handlers) HandleDataDogCheckRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentEncoding := r.Header.Get("Content-Encoding")
	batch, err := h.datadogParser.ParseCheckRun(r.Body, contentEncoding)
	if err != nil {
		writeDataDogError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		writeDataDogError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// HandleDataDogDistribution handles DataDog /api/v1/distribution_points endpoint
// POST /api/datadog/v1/distribution_points
func (h *Handlers) HandleDataDogDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentEncoding := r.Header.Get("Content-Encoding")
	batch, err := h.datadogParser.ParseDistributionPoints(r.Body, contentEncoding)
	if err != nil {
		writeDataDogError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		writeDataDogError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// HandleDataDogIntake handles DataDog /intake endpoint (agent metadata)
// POST /api/datadog/intake
func (h *Handlers) HandleDataDogIntake(w http.ResponseWriter, r *http.Request) {
	// Accept but ignore agent metadata
	io.Copy(io.Discard, r.Body)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// HandleDataDogValidate handles DataDog /api/v1/validate endpoint
// GET /api/datadog/v1/validate
func (h *Handlers) HandleDataDogValidate(w http.ResponseWriter, r *http.Request) {
	// Always return valid - we accept all API keys
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid": true,
	})
}

func writeDataDogError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []string{msg},
	})
}

// HandleStatsD handles StatsD protocol over HTTP
// POST /api/statsd/write
func (h *Handlers) HandleStatsD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batch, err := h.statsdParser.Parse(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.store.WriteSamples(batch.Samples); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RegisterRoutes registers all ingestion routes on a ServeMux
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	// Graphite endpoints
	mux.HandleFunc("/api/graphite/write", h.HandleGraphitePlaintext)
	mux.HandleFunc("/api/graphite/pickle", h.HandleGraphitePickle)

	// InfluxDB endpoints
	mux.HandleFunc("/api/influx/write", h.HandleInfluxWrite)
	mux.HandleFunc("/api/influx/query", h.HandleInfluxQuery)
	// Also support standard InfluxDB paths
	mux.HandleFunc("/write", h.HandleInfluxWrite)
	mux.HandleFunc("/api/v2/write", h.HandleInfluxWrite)

	// OpenTSDB endpoints
	mux.HandleFunc("/api/opentsdb/put", h.HandleOpenTSDBPut)
	mux.HandleFunc("/api/opentsdb/telnet", h.HandleOpenTSDBTelnet)
	// Standard OpenTSDB path
	mux.HandleFunc("/api/put", h.HandleOpenTSDBPut)

	// StatsD endpoint (HTTP bridge for StatsD format)
	mux.HandleFunc("/api/statsd/write", h.HandleStatsD)

	// DataDog endpoints
	mux.HandleFunc("/api/datadog/v1/series", h.HandleDataDogV1Series)
	mux.HandleFunc("/api/datadog/v2/series", h.HandleDataDogV2Series)
	mux.HandleFunc("/api/datadog/v1/check_run", h.HandleDataDogCheckRun)
	mux.HandleFunc("/api/datadog/v1/distribution_points", h.HandleDataDogDistribution)
	mux.HandleFunc("/api/datadog/intake", h.HandleDataDogIntake)
	mux.HandleFunc("/api/datadog/v1/validate", h.HandleDataDogValidate)
	// Standard DataDog paths (for direct agent compatibility)
	mux.HandleFunc("/api/v1/series", h.wrapDataDogV1Series)
	mux.HandleFunc("/api/v2/series", h.wrapDataDogV2Series)
	mux.HandleFunc("/api/v1/check_run", h.wrapDataDogCheckRun)
	mux.HandleFunc("/api/v1/distribution_points", h.wrapDataDogDistribution)
	mux.HandleFunc("/intake", h.HandleDataDogIntake)
}

// wrapDataDogV1Series checks for DD-API-KEY header to identify DataDog requests
func (h *Handlers) wrapDataDogV1Series(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("DD-API-KEY") != "" || strings.Contains(r.Header.Get("User-Agent"), "datadog") {
		h.HandleDataDogV1Series(w, r)
		return
	}
	// Not a DataDog request - return 404 or handle differently
	http.NotFound(w, r)
}

func (h *Handlers) wrapDataDogV2Series(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("DD-API-KEY") != "" || strings.Contains(r.Header.Get("User-Agent"), "datadog") {
		h.HandleDataDogV2Series(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *Handlers) wrapDataDogCheckRun(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("DD-API-KEY") != "" || strings.Contains(r.Header.Get("User-Agent"), "datadog") {
		h.HandleDataDogCheckRun(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *Handlers) wrapDataDogDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("DD-API-KEY") != "" || strings.Contains(r.Header.Get("User-Agent"), "datadog") {
		h.HandleDataDogDistribution(w, r)
		return
	}
	http.NotFound(w, r)
}
