package web

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dogwatch/internal/aggregator"
	"dogwatch/internal/alerting"
	"dogwatch/internal/anomaly"
	"dogwatch/internal/bubbleup"
	"dogwatch/internal/catalog"
	"dogwatch/internal/correlation"
	"dogwatch/internal/query"
	"dogwatch/internal/containers"
	"dogwatch/internal/custommetrics"
	"dogwatch/internal/deploys"
	"dogwatch/internal/dashboard"
	"dogwatch/internal/federation"
	"dogwatch/internal/incidents"
	"dogwatch/internal/kubernetes"
	"dogwatch/internal/logs"
	"dogwatch/internal/oncall"
	"dogwatch/internal/metrics"
	"dogwatch/internal/slo"
	"dogwatch/internal/storage"
	"dogwatch/internal/synthetics"
	"dogwatch/internal/statuspage"
	"dogwatch/internal/trace"
	"dogwatch/internal/watch"

	"github.com/google/uuid"
)

//go:embed all:static
var staticFiles embed.FS

// FlameGraphProvider interface for getting flame graph data
type FlameGraphProvider interface {
	GetFlameGraph() (interface{}, error)
	ClearSamples() error
}

// Server serves the web dashboard
type Server struct {
	agg                 *aggregator.Aggregator
	metrics             *metrics.Collector
	store               *storage.Store
	traceStore          *trace.Store
	otlpReceiver        *trace.OTLPReceiver
	watchStore          *watch.Store
	watchEngine         *watch.Engine
	dashboardStore      *dashboard.Store
	logStore            *logs.Store
	customMetricsStore  *custommetrics.Store
	otlpMetricsReceiver *custommetrics.OTLPMetricsReceiver
	prometheusReceiver  *custommetrics.PrometheusReceiver
	syntheticsStore     *synthetics.Store
	syntheticsRunner    *synthetics.Runner
	sloStore            *slo.Store
	sloCalculator       *slo.Calculator
	containerCollector  *containers.Collector
	deployStore         *deploys.Store
	incidentStore       *incidents.Store
	pager               *incidents.Pager
	cluster             *federation.Cluster
	k8sCollector        *kubernetes.Collector
	anomalyService      *anomaly.Service
	queryExecutor       *query.Executor
	oncallStore         *oncall.Store
	oncallCalculator    *oncall.Calculator
	oncallEscalation    *oncall.EscalationEngine
	alertManager        *alerting.AlertManager
	profiler            FlameGraphProvider
	statusPageStore     *statuspage.Store
	statusPageHandlers  *StatusPageHandlers
	catalogStore        *catalog.Store
	catalogHandlers     *CatalogHandlers
	catalogDiscovery    *catalog.DiscoveryService
	correlationEngine   *correlation.Engine
	correlationHandlers *CorrelationHandlers
	bubbleupAnalyzer    *bubbleup.Analyzer
	bubbleupHandlers    *BubbleUpHandlers
	myServicesHandlers  *MyServicesHandlers
	wsHub               *Hub
	server              *http.Server
	mux                 *http.ServeMux
	mu                  sync.RWMutex
}

// Mux returns the underlying HTTP mux for registering additional routes
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// gzipResponseWriter wraps http.ResponseWriter with gzip compression
type gzipResponseWriter struct {
	http.ResponseWriter
	Writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// gzipMiddleware compresses responses for compressible content types
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Add cache headers for static assets
		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") {
			// Cache JS/CSS for 1 hour, allow stale-while-revalidate
			w.Header().Set("Cache-Control", "public, max-age=3600, stale-while-revalidate=86400")
		} else if strings.HasSuffix(path, ".woff2") || strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".ttf") {
			// Cache fonts for 1 year (they rarely change)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".ico") {
			// Cache images for 1 day
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}

		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Only compress certain file types
		compress := strings.HasSuffix(path, ".js") ||
			strings.HasSuffix(path, ".css") ||
			strings.HasSuffix(path, ".html") ||
			strings.HasSuffix(path, ".json") ||
			strings.HasSuffix(path, ".svg")

		if !compress {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzip.NewWriter(w)
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length changes with compression

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, Writer: gz}, r)
	})
}

// New creates a new web server
func New(agg *aggregator.Aggregator, port int) *Server {
	s := &Server{
		agg:     agg,
		metrics: metrics.NewCollector(),
	}

	// Initialize WebSocket hub for real-time updates
	s.wsHub = NewHub()
	go s.wsHub.Run()

	mux := http.NewServeMux()
	s.mux = mux

	// Serve static files with gzip compression
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", gzipMiddleware(http.FileServer(http.FS(staticFS))))

	// WebSocket endpoint for real-time updates
	mux.HandleFunc("/api/ws", s.wsHub.ServeWS)

	// API endpoints
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/endpoints", s.handleEndpoints)
	mux.HandleFunc("/api/connections", s.handleConnections)
	mux.HandleFunc("/api/system", s.handleSystem)
	mux.HandleFunc("/api/servicemap", s.handleServiceMap)
	mux.HandleFunc("/api/processes", s.handleProcesses)
	mux.HandleFunc("/api/flamegraph", s.handleFlameGraph)
	mux.HandleFunc("/api/flamegraph/clear", s.handleFlameGraphClear)
	mux.HandleFunc("/api/history/system", s.handleHistorySystem)
	mux.HandleFunc("/api/history/connections", s.handleHistoryConnections)
	mux.HandleFunc("/api/history/info", s.handleHistoryInfo)

	// Trace endpoints
	mux.HandleFunc("/v1/traces", s.handleOTLPTraces)
	mux.HandleFunc("/v1/trace", s.handleSimpleTrace)
	mux.HandleFunc("/api/traces", s.handleListTraces)
	mux.HandleFunc("/api/traces/", s.handleGetTrace)
	mux.HandleFunc("/api/trace/services", s.handleTraceServices)
	mux.HandleFunc("/api/trace/dependencies", s.handleTraceDependencies)

	// Watch endpoints
	mux.HandleFunc("/api/watches", s.handleWatches)
	mux.HandleFunc("/api/watches/", s.handleWatch)
	mux.HandleFunc("/api/watch/channels", s.handleChannels)
	mux.HandleFunc("/api/watch/channels/", s.handleChannel)
	mux.HandleFunc("/api/watch/events", s.handleWatchEvents)
	mux.HandleFunc("/api/watch/metrics", s.handleWatchMetrics)

	// Dashboard endpoints
	mux.HandleFunc("/api/dashboards", s.handleDashboards)
	mux.HandleFunc("/api/dashboards/", s.handleDashboard)
	mux.HandleFunc("/api/dashboards/default", s.handleDefaultDashboard)

	// Log endpoints
	mux.HandleFunc("/api/logs", s.handleLogs)
	mux.HandleFunc("/api/logs/ingest", s.handleLogIngest)
	mux.HandleFunc("/api/logs/services", s.handleLogServices)
	mux.HandleFunc("/api/logs/stats", s.handleLogStats)
	// Note: /api/logs/patterns routes registered via RegisterLogReduceRoutes()

	// Custom metrics endpoints
	mux.HandleFunc("/v1/metrics", s.handleOTLPMetrics)
	mux.HandleFunc("/api/metrics/push", s.handleMetricsPush)
	mux.HandleFunc("/api/metrics/query", s.handleMetricsQuery)
	mux.HandleFunc("/api/metrics/list", s.handleMetricsList)
	mux.HandleFunc("/api/metrics/histogram", s.handleMetricsHistogram)

	// Prometheus remote write endpoint
	mux.HandleFunc("/api/v1/write", s.handlePrometheusRemoteWrite)
	mux.HandleFunc("/api/v1/write/stats", s.handlePrometheusStats)

	// Synthetics endpoints
	mux.HandleFunc("/api/synthetics/checks", s.handleSyntheticsChecks)
	mux.HandleFunc("/api/synthetics/checks/", s.handleSyntheticsCheck)
	mux.HandleFunc("/api/synthetics/results/", s.handleSyntheticsResults)
	mux.HandleFunc("/api/synthetics/uptime/", s.handleSyntheticsUptime)

	// SLO endpoints
	mux.HandleFunc("/api/slos", s.handleSLOs)
	mux.HandleFunc("/api/slos/", s.handleSLO)

	// Container endpoints
	mux.HandleFunc("/api/containers", s.handleContainers)
	mux.HandleFunc("/api/containers/", s.handleContainer)
	mux.HandleFunc("/api/containers/summary", s.handleContainerSummary)

	// Deployment endpoints
	mux.HandleFunc("/api/deploys", s.handleDeploys)
	mux.HandleFunc("/api/deploys/", s.handleDeploy)
	mux.HandleFunc("/api/deploys/stats", s.handleDeployStats)
	mux.HandleFunc("/api/deploys/services", s.handleDeployServices)

	// Incidents / Paging
	mux.HandleFunc("/api/incidents", s.handleIncidents)
	mux.HandleFunc("/api/incidents/", s.handleIncident)
	mux.HandleFunc("/api/incidents/stats", s.handleIncidentStats)
	mux.HandleFunc("/api/oncall", s.handleOnCallSchedules)
	mux.HandleFunc("/api/oncall/", s.handleOnCallSchedule)
	mux.HandleFunc("/api/oncall/current", s.handleCurrentOnCall)
	mux.HandleFunc("/api/oncall/my", s.handleMyOnCall)
	mux.HandleFunc("/my-oncall", s.handleMyOnCallPage)
	mux.HandleFunc("/api/escalation", s.handleEscalationPolicies)
	mux.HandleFunc("/api/escalation/", s.handleEscalationPolicy)

	// Federation / Cluster endpoints
	mux.HandleFunc("/api/cluster", s.handleCluster)
	mux.HandleFunc("/api/cluster/nodes", s.handleClusterNodes)
	mux.HandleFunc("/api/cluster/join", s.handleClusterJoin)
	mux.HandleFunc("/api/cluster/incidents", s.handleClusterIncidents)
	mux.HandleFunc("/api/cluster/metrics", s.handleClusterMetrics)
	mux.HandleFunc("/api/cluster/state", s.handleClusterState)

	// Kubernetes endpoints
	mux.HandleFunc("/api/k8s", s.handleK8sInfo)
	mux.HandleFunc("/api/k8s/summary", s.handleK8sSummary)
	mux.HandleFunc("/api/k8s/nodes", s.handleK8sNodes)
	mux.HandleFunc("/api/k8s/namespaces", s.handleK8sNamespaces)
	mux.HandleFunc("/api/k8s/pods", s.handleK8sPods)
	mux.HandleFunc("/api/k8s/pods/", s.handleK8sPod)
	mux.HandleFunc("/api/k8s/deployments", s.handleK8sDeployments)
	mux.HandleFunc("/api/k8s/services", s.handleK8sServices)
	mux.HandleFunc("/api/k8s/daemonsets", s.handleK8sDaemonSets)
	mux.HandleFunc("/api/k8s/statefulsets", s.handleK8sStatefulSets)
	mux.HandleFunc("/api/k8s/jobs", s.handleK8sJobs)
	mux.HandleFunc("/api/k8s/cronjobs", s.handleK8sCronJobs)
	mux.HandleFunc("/api/k8s/ingresses", s.handleK8sIngresses)
	mux.HandleFunc("/api/k8s/events", s.handleK8sEvents)
	mux.HandleFunc("/api/k8s/workloads", s.handleK8sWorkloads)

	// Anomaly detection endpoints
	mux.HandleFunc("/api/anomaly/stats", s.handleAnomalyStats)
	mux.HandleFunc("/api/anomaly/recent", s.handleAnomalyRecent)
	mux.HandleFunc("/api/anomaly/metrics", s.handleAnomalyMetrics)
	mux.HandleFunc("/api/anomaly/push", s.handleAnomalyPush)
	mux.HandleFunc("/api/anomaly/train", s.handleAnomalyTrain)
	mux.HandleFunc("/api/anomaly/subscribe", s.handleAnomalySubscribe)

	// WatchQL query endpoints
	mux.HandleFunc("/api/query", s.handleQuery)
	mux.HandleFunc("/api/query/explain", s.handleQueryExplain)
	mux.HandleFunc("/api/query/validate", s.handleQueryValidate)

	// On-call scheduling endpoints
	mux.HandleFunc("/api/oncall/schedules", s.handleOncallSchedules)
	mux.HandleFunc("/api/oncall/schedules/", s.handleOncallSchedule)
	mux.HandleFunc("/api/oncall/current/", s.handleOncallCurrent)
	mux.HandleFunc("/api/oncall/calendar/", s.handleOncallCalendar)
	mux.HandleFunc("/api/oncall/overrides", s.handleOncallOverrides)
	mux.HandleFunc("/api/oncall/overrides/", s.handleOncallOverride)
	mux.HandleFunc("/api/oncall/policies", s.handleOncallPolicies)
	mux.HandleFunc("/api/oncall/policies/", s.handleOncallPolicy)
	mux.HandleFunc("/api/oncall/escalate", s.handleOncallEscalate)
	mux.HandleFunc("/api/oncall/acknowledge", s.handleOncallAcknowledge)

	// Alerting endpoints
	RegisterAlertingRoutes(mux)

	// RBAC / Authentication endpoints
	RegisterRBACRoutes(mux)

	// SSO / OAuth2 / SAML endpoints
	RegisterSSORoutes(mux)

	// Notification channel endpoints
	mux.HandleFunc("/api/notify/channels", s.handleNotifyChannels)
	mux.HandleFunc("/api/notify/channels/", s.handleNotifyChannel)
	mux.HandleFunc("/api/notify/history", s.handleNotifyHistory)

	// Audit log endpoints - enhanced for compliance
	RegisterAuditRoutes(mux, s)

	// Status page endpoints
	s.initStatusPages()
	if s.statusPageHandlers != nil {
		s.statusPageHandlers.RegisterRoutes(mux)
	}

	// Service catalog endpoints
	s.initCatalog()
	if s.catalogHandlers != nil {
		s.catalogHandlers.RegisterRoutes(mux)
	}

	// Correlation engine endpoints
	s.initCorrelation()
	if s.correlationHandlers != nil {
		s.correlationHandlers.RegisterRoutes(mux)
	}

	// BubbleUp endpoints
	s.initBubbleUp()
	if s.bubbleupHandlers != nil {
		s.bubbleupHandlers.RegisterRoutes(mux)
	}

	// Database watch endpoints
	RegisterDBWatchRoutes(mux)

	// Backup/restore endpoints
	RegisterBackupRoutes(mux)

	// Storage architecture endpoints (WAL, tiering, backends)
	RegisterStorageRoutes(mux)

	// Cost intelligence endpoints
	RegisterCostIntelRoutes(mux)

	// Cardinality explorer endpoints
	RegisterCardinalityRoutes(mux)

	// Cardinality controller endpoints registered in main.go (needs store setup)

	// Usage analytics endpoints
	RegisterUsageRoutes(mux)

	// Data shaping endpoints
	RegisterDataShapingRoutes(mux)

	// Quota and chargeback endpoints
	RegisterQuotaRoutes(mux)

	// Cost recommendations endpoints
	RegisterRecommendationRoutes(mux)

	// Log comparison endpoints
	RegisterLogCompareRoutes(mux)

	// Log pattern mining endpoints
	RegisterLogReduceRoutes(mux)

	// Log field extraction endpoints
	RegisterExtractionRoutes(mux)

	// Migration endpoints registered in main.go (needs store setup)

	// Migration fidelity analysis endpoints
	RegisterFidelityRoutes(mux)

	// Query builder endpoints
	RegisterQueryRoutes(mux, s)

	// Health check endpoints (no auth required)
	// Standard paths
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/ready", s.handleReady)
	// Kubernetes standard paths
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/livez", s.handleLivez)

	// Apply security middleware (headers + CSRF protection)
	securityConfig := DefaultSecurityConfig()
	securedHandler := SecurityMiddleware(securityConfig)(mux)

	// Apply rate limiting middleware
	rateLimitConfig := DefaultRateLimitConfig()
	rateLimitedHandler := RateLimitMiddleware(rateLimitConfig)(securedHandler)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: rateLimitedHandler,
	}

	// Start periodic WebSocket broadcasts for real-time updates
	s.wsHub.StartPeriodicBroadcasts(
		func() interface{} {
			// System stats
			if s.metrics != nil {
				return s.metrics.Collect()
			}
			return nil
		},
		func() interface{} {
			// Service map
			if s.agg != nil {
				return s.buildServiceMapData()
			}
			return nil
		},
	)

	return s
}

// handleHealth returns basic health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleReady returns readiness status with component checks
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	allReady := true

	// Check aggregator
	if s.agg != nil {
		checks["aggregator"] = "ok"
	} else {
		checks["aggregator"] = "not initialized"
		allReady = false
	}

	// Check storage
	if s.store != nil {
		checks["metrics_store"] = "ok"
	} else {
		checks["metrics_store"] = "not configured"
	}

	// Check trace store
	if s.traceStore != nil {
		checks["trace_store"] = "ok"
	} else {
		checks["trace_store"] = "not configured"
	}

	w.Header().Set("Content-Type", "application/json")
	if allReady {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":  allReady,
		"checks": checks,
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleHealthz is the Kubernetes-standard liveness probe
// Returns 200 if the process is alive and can handle requests
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// Check if verbose output requested
	verbose := r.URL.Query().Get("verbose") == "true"

	checks := make(map[string]string)
	healthy := true

	// Basic process health - if we're responding, we're alive
	checks["ping"] = "ok"

	// Check if server can accept connections
	if s.server != nil {
		checks["server"] = "ok"
	} else {
		checks["server"] = "not initialized"
		healthy = false
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if healthy {
		w.WriteHeader(http.StatusOK)
		if verbose {
			for name, status := range checks {
				fmt.Fprintf(w, "[+] %s %s\n", name, status)
			}
		} else {
			w.Write([]byte("ok"))
		}
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		for name, status := range checks {
			if status != "ok" {
				fmt.Fprintf(w, "[-] %s %s\n", name, status)
			} else if verbose {
				fmt.Fprintf(w, "[+] %s %s\n", name, status)
			}
		}
	}
}

// handleReadyz is the Kubernetes-standard readiness probe
// Returns 200 only if the service is ready to receive traffic
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	verbose := r.URL.Query().Get("verbose") == "true"

	checks := make(map[string]string)
	ready := true

	// Core components required for readiness
	if s.agg != nil {
		checks["aggregator"] = "ok"
	} else {
		checks["aggregator"] = "not initialized"
		ready = false
	}

	// Storage checks (optional but tracked)
	if s.store != nil {
		checks["metrics_store"] = "ok"
	} else {
		checks["metrics_store"] = "not configured"
	}

	if s.traceStore != nil {
		checks["trace_store"] = "ok"
	} else {
		checks["trace_store"] = "not configured"
	}

	if s.logStore != nil {
		checks["log_store"] = "ok"
	} else {
		checks["log_store"] = "not configured"
	}

	if s.customMetricsStore != nil {
		checks["custom_metrics_store"] = "ok"
	} else {
		checks["custom_metrics_store"] = "not configured"
	}

	// Kubernetes collector (optional)
	if s.k8sCollector != nil {
		checks["kubernetes"] = "ok"
	}

	// Cluster federation (optional)
	if s.cluster != nil {
		checks["federation"] = "ok"
	}

	// Alert manager (optional)
	if s.alertManager != nil {
		checks["alerting"] = "ok"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if ready {
		w.WriteHeader(http.StatusOK)
		if verbose {
			for name, status := range checks {
				fmt.Fprintf(w, "[+] %s %s\n", name, status)
			}
		} else {
			w.Write([]byte("ok"))
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		for name, status := range checks {
			if status != "ok" && status != "not configured" {
				fmt.Fprintf(w, "[-] %s %s\n", name, status)
			} else if verbose {
				fmt.Fprintf(w, "[+] %s %s\n", name, status)
			}
		}
	}
}

// handleLivez is an alias for healthz (Kubernetes liveness)
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	s.handleHealthz(w, r)
}

// SetProfiler sets the flame graph profiler
func (s *Server) SetProfiler(p FlameGraphProvider) {
	s.profiler = p
}

// SetStore sets the storage backend
func (s *Server) SetStore(st *storage.Store) {
	s.store = st
}

// initStatusPages initializes the status page store and handlers
func (s *Server) initStatusPages() {
	dbPath := os.Getenv("STATUSPAGE_DB")
	if dbPath == "" {
		dbPath = filepath.Join(os.TempDir(), "dogwatch_statuspage.db")
	}

	store, err := statuspage.NewStore(dbPath)
	if err != nil {
		fmt.Printf("Warning: Failed to initialize status page store: %v\n", err)
		return
	}

	s.statusPageStore = store
	s.statusPageHandlers = NewStatusPageHandlers(store, staticFiles)
}

// SetStatusPageStore sets the status page store
func (s *Server) SetStatusPageStore(store *statuspage.Store) {
	s.statusPageStore = store
	s.statusPageHandlers = NewStatusPageHandlers(store, staticFiles)
}

// initCatalog initializes the service catalog store and handlers
func (s *Server) initCatalog() {
	dbPath := os.Getenv("CATALOG_DB")
	if dbPath == "" {
		dbPath = filepath.Join(os.TempDir(), "dogwatch_catalog.db")
	}

	store, err := catalog.NewStore(dbPath)
	if err != nil {
		fmt.Printf("Warning: Failed to initialize catalog store: %v\n", err)
		return
	}

	s.catalogStore = store
	s.catalogHandlers = NewCatalogHandlers(store)
	s.catalogDiscovery = catalog.NewDiscoveryService(store, "default")
	s.catalogDiscovery.Start()

	// Wire up health callback if synthetics runner already exists
	if s.syntheticsRunner != nil {
		s.syntheticsRunner.SetHealthCallback(s.catalogDiscovery.UpdateServiceHealthFromSynthetics)
	}

	// Wire up existing stores for context API
	if s.incidentStore != nil {
		s.catalogHandlers.SetIncidentStore(s.incidentStore)
	}
	if s.syntheticsStore != nil {
		s.catalogHandlers.SetSyntheticsStore(s.syntheticsStore)
	}
}

// SetCatalogStore sets the catalog store
func (s *Server) SetCatalogStore(store *catalog.Store) {
	s.catalogStore = store
	s.catalogHandlers = NewCatalogHandlers(store)
	s.catalogDiscovery = catalog.NewDiscoveryService(store, "default")
	s.catalogDiscovery.Start()

	// Wire up health callback if synthetics runner already exists
	if s.syntheticsRunner != nil {
		s.syntheticsRunner.SetHealthCallback(s.catalogDiscovery.UpdateServiceHealthFromSynthetics)
	}

	// Wire up existing stores for context API
	if s.incidentStore != nil {
		s.catalogHandlers.SetIncidentStore(s.incidentStore)
	}
	if s.syntheticsStore != nil {
		s.catalogHandlers.SetSyntheticsStore(s.syntheticsStore)
	}
}

// GetCatalogDiscovery returns the catalog discovery service for trace integration
func (s *Server) GetCatalogDiscovery() *catalog.DiscoveryService {
	return s.catalogDiscovery
}

// SetMyServicesHandlers sets up the My Services handlers
func (s *Server) SetMyServicesHandlers(handlers *MyServicesHandlers) {
	s.myServicesHandlers = handlers

	// Wire up stores that the server has
	if s.deployStore != nil {
		handlers.SetDeploysStore(s.deployStore)
	}
	if s.incidentStore != nil {
		handlers.SetIncidentStore(s.incidentStore)
	}
	if s.oncallStore != nil && s.oncallCalculator != nil {
		handlers.SetOncallStore(s.oncallStore, s.oncallCalculator)
	}
	if s.sloStore != nil {
		handlers.SetSLOStore(s.sloStore)
	}

	// Register routes
	handlers.RegisterRoutes(s.mux)
}

// initCorrelation initializes the correlation engine
func (s *Server) initCorrelation() {
	s.correlationEngine = correlation.NewEngine()
	s.correlationHandlers = NewCorrelationHandlers(s.correlationEngine)

	// Wire up any existing stores
	if s.traceStore != nil {
		s.correlationEngine.SetTraceStore(s.traceStore)
	}
	if s.logStore != nil {
		s.correlationEngine.SetLogStore(s.logStore)
	}
	if s.incidentStore != nil {
		s.correlationEngine.SetIncidentStore(s.incidentStore)
	}
	if s.deployStore != nil {
		s.correlationEngine.SetDeployStore(s.deployStore)
	}
}

// initBubbleUp initializes the BubbleUp analyzer
func (s *Server) initBubbleUp() {
	s.bubbleupAnalyzer = bubbleup.NewAnalyzer(s.traceStore)
	s.bubbleupHandlers = NewBubbleUpHandlers(s.bubbleupAnalyzer)
}

// GetCorrelationEngine returns the correlation engine for external use
func (s *Server) GetCorrelationEngine() *correlation.Engine {
	return s.correlationEngine
}

// SetTraceStore sets the trace storage backend
func (s *Server) SetTraceStore(ts *trace.Store) {
	s.traceStore = ts
	s.otlpReceiver = trace.NewOTLPReceiver(ts)
	if s.correlationEngine != nil {
		s.correlationEngine.SetTraceStore(ts)
	}
	if s.bubbleupAnalyzer != nil {
		s.bubbleupAnalyzer.SetTraceStore(ts)
	}
}

// SetSpanCallback sets a callback for span processing (for entity synthesis)
func (s *Server) SetSpanCallback(cb trace.SpanCallback) {
	if s.otlpReceiver != nil {
		// Wrap callback to also broadcast to WebSocket subscribers
		wrappedCb := func(span trace.Span) {
			// Call the original callback (entity synthesis)
			if cb != nil {
				cb(span)
			}
			// Broadcast to WebSocket subscribers (real-time trace updates)
			if s.wsHub != nil && s.wsHub.TopicSubscriberCount(TopicTraces) > 0 {
				s.wsHub.BroadcastTrace(map[string]interface{}{
					"trace_id":     span.TraceID,
					"span_id":      span.SpanID,
					"service_name": span.ServiceName,
					"name":         span.Name,
					"duration_ms":  span.DurationMs,
					"status":       span.Status,
					"timestamp":    span.StartTime,
				})
			}
		}
		s.otlpReceiver.SetSpanCallback(wrappedCb)
	}
}

// SetWatchStore sets the watch storage and starts the engine
func (s *Server) SetWatchStore(ws *watch.Store) {
	s.watchStore = ws
	adapter := watch.NewMetricsAdapter(s.metrics, s.agg)
	notifier := watch.NewNotifier()
	s.watchEngine = watch.NewEngine(ws, adapter, notifier)

	// Connect pager if already set up
	if s.pager != nil {
		s.watchEngine.SetPager(s.pager)
	}

	// Connect WebSocket broadcaster for real-time updates
	if s.wsHub != nil {
		s.watchEngine.SetBroadcaster(s.wsHub)
	}

	s.watchEngine.Start()
}

// StopWatchEngine stops the watch evaluation engine
func (s *Server) StopWatchEngine() {
	if s.watchEngine != nil {
		s.watchEngine.Stop()
	}
}

// SetDashboardStore sets the dashboard storage backend
func (s *Server) SetDashboardStore(ds *dashboard.Store) {
	s.dashboardStore = ds
}

// SetLogStore sets the log storage backend
func (s *Server) SetLogStore(ls *logs.Store) {
	s.logStore = ls
	if s.correlationEngine != nil {
		s.correlationEngine.SetLogStore(ls)
	}
}

// SetCustomMetricsStore sets the custom metrics storage backend
func (s *Server) SetCustomMetricsStore(cms *custommetrics.Store) {
	s.customMetricsStore = cms
	s.otlpMetricsReceiver = custommetrics.NewOTLPMetricsReceiver(cms)
	s.prometheusReceiver = custommetrics.NewPrometheusReceiver(cms)
}

// SetSyntheticsStore sets the synthetics storage and starts the runner
func (s *Server) SetSyntheticsStore(ss *synthetics.Store, notifier *watch.Notifier) {
	s.syntheticsStore = ss
	s.syntheticsRunner = synthetics.NewRunner(ss, notifier)

	// Wire up health callback to update Service Catalog health from synthetics results
	if s.catalogDiscovery != nil {
		s.syntheticsRunner.SetHealthCallback(s.catalogDiscovery.UpdateServiceHealthFromSynthetics)
	}

	// Connect synthetics store to catalog handlers for context API
	if s.catalogHandlers != nil {
		s.catalogHandlers.SetSyntheticsStore(ss)
	}

	s.syntheticsRunner.Start()
}

// StopSyntheticsRunner stops the synthetics runner
func (s *Server) StopSyntheticsRunner() {
	if s.syntheticsRunner != nil {
		s.syntheticsRunner.Stop()
	}
}

// SetSLOStore sets the SLO storage and starts the calculator
func (s *Server) SetSLOStore(ss *slo.Store) {
	s.sloStore = ss
	s.sloCalculator = slo.NewCalculator(ss, s.syntheticsStore)

	// Connect pager if already set up
	if s.pager != nil {
		s.sloCalculator.SetPager(s.pager)
	}

	s.sloCalculator.Start()
}

// StopSLOCalculator stops the SLO calculator
func (s *Server) StopSLOCalculator() {
	if s.sloCalculator != nil {
		s.sloCalculator.Stop()
	}
}

// SetContainerCollector sets the container metrics collector
func (s *Server) SetContainerCollector(cc *containers.Collector) {
	s.containerCollector = cc
}

// StopContainerCollector stops the container collector
func (s *Server) StopContainerCollector() {
	if s.containerCollector != nil {
		s.containerCollector.Stop()
	}
}

// SetDeployStore sets the deployment store
func (s *Server) SetDeployStore(ds *deploys.Store) {
	s.deployStore = ds
	if s.correlationEngine != nil {
		s.correlationEngine.SetDeployStore(ds)
	}
}

// SetIncidentStore sets the incident store and starts the pager
func (s *Server) SetIncidentStore(is *incidents.Store) {
	s.incidentStore = is
	s.pager = incidents.NewPager(is)
	s.pager.Start()

	// Connect pager to watch engine if already set up
	if s.watchEngine != nil {
		s.watchEngine.SetPager(s.pager)
	}

	// Connect pager to SLO calculator if already set up
	if s.sloCalculator != nil {
		s.sloCalculator.SetPager(s.pager)
	}

	// Connect incident store to catalog handlers for context API
	if s.catalogHandlers != nil {
		s.catalogHandlers.SetIncidentStore(is)
	}

	// Connect incident store to correlation engine
	if s.correlationEngine != nil {
		s.correlationEngine.SetIncidentStore(is)
	}
}

// GetPager returns the pager for external triggering
func (s *Server) GetPager() *incidents.Pager {
	return s.pager
}

// StopPager stops the incident pager
func (s *Server) StopPager() {
	if s.pager != nil {
		s.pager.Stop()
	}
}

// SetCluster sets the federation cluster
func (s *Server) SetCluster(c *federation.Cluster) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cluster = c

	// Set up callbacks for incident synchronization
	if s.cluster != nil {
		s.cluster.GetState().SetOnIncidentChange(func(inc *federation.IncidentEvent) {
			// Sync federated incidents to local store if we have one
			if s.incidentStore != nil {
				s.syncFederatedIncident(inc)
			}
		})
	}
}

// GetCluster returns the federation cluster
func (s *Server) GetCluster() *federation.Cluster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cluster
}

// StopCluster stops the federation cluster
func (s *Server) StopCluster() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cluster != nil {
		s.cluster.Stop()
	}
}

// SetK8sCollector sets the Kubernetes collector
func (s *Server) SetK8sCollector(k8s *kubernetes.Collector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.k8sCollector = k8s

	// Connect pager for K8s events
	if s.pager != nil && k8s != nil {
		k8s.SetPager(&k8sPagerAdapter{pager: s.pager})
	}
}

// GetK8sCollector returns the Kubernetes collector
func (s *Server) GetK8sCollector() *kubernetes.Collector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.k8sCollector
}

// StopK8sCollector stops the Kubernetes collector
func (s *Server) StopK8sCollector() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.k8sCollector != nil {
		s.k8sCollector.Stop()
	}
}

// SetAnomalyService sets the anomaly detection service
func (s *Server) SetAnomalyService(as *anomaly.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.anomalyService = as

	// Connect anomaly detection to incident triggering
	if s.pager != nil && as != nil {
		as.SetAnomalyCallback(func(result anomaly.AnomalyResult) {
			if result.IsCritical {
				s.pager.Trigger(&incidents.Incident{
					Title:       fmt.Sprintf("[Anomaly] %s", result.MetricName),
					Description: result.Reason,
					Severity:    incidents.SeverityHigh,
					Source:      "anomaly_detection",
					Service:     result.MetricName,
				})
			}
		})
	}
}

// GetAnomalyService returns the anomaly detection service
func (s *Server) GetAnomalyService() *anomaly.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.anomalyService
}

// SetAlertManager sets the alert manager
func (s *Server) SetAlertManager(am *alerting.AlertManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertManager = am
	SetAlertManager(am) // Set for handlers

	// Connect WebSocket broadcaster for real-time alert updates
	if s.wsHub != nil {
		am.SetBroadcaster(s.wsHub)
	}
}

// GetAlertManager returns the alert manager
func (s *Server) GetAlertManager() *alerting.AlertManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.alertManager
}

// StopAlertManager stops the alert manager
func (s *Server) StopAlertManager() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.alertManager != nil {
		s.alertManager.Stop()
	}
}

// k8sPagerAdapter adapts our incidents.Pager to the kubernetes.IncidentTrigger interface
type k8sPagerAdapter struct {
	pager *incidents.Pager
}

func (a *k8sPagerAdapter) TriggerFromK8s(eventType, namespace, name, kind, reason, message string) error {
	if a.pager == nil {
		return nil
	}

	title := fmt.Sprintf("[K8s] %s: %s/%s %s", kind, namespace, name, reason)
	severity := incidents.SeverityMedium
	if reason == "CrashLoopBackOff" || reason == "OOMKilled" || reason == "FailedScheduling" {
		severity = incidents.SeverityHigh
	}

	return a.pager.Trigger(&incidents.Incident{
		Title:       title,
		Description: message,
		Severity:    severity,
		Source:      "kubernetes",
		Service:     fmt.Sprintf("%s/%s", namespace, name),
	})
}

// syncFederatedIncident syncs a federated incident to the local store
func (s *Server) syncFederatedIncident(inc *federation.IncidentEvent) {
	// Only sync if we're not the originating node
	if s.cluster == nil {
		return
	}
	localNode := s.cluster.LocalNode()
	if localNode != nil && inc.NodeID == localNode.ID {
		return // We originated this, skip
	}

	// Log federated incident for visibility
	// In a real implementation, we might store these separately or merge
}

// GetMetricsCollector returns the metrics collector for external recording
func (s *Server) GetMetricsCollector() *metrics.Collector {
	return s.metrics
}

// Start begins serving HTTP
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// StatsResponse is the JSON response for /api/stats
type StatsResponse struct {
	UpdatedAt        string             `json:"updated_at"`
	TotalConnections int64              `json:"total_connections"`
	TotalRequests    int64              `json:"total_requests"`
	TotalErrors      int64              `json:"total_errors"`
	Endpoints        []EndpointResponse `json:"endpoints"`
	Connections      []ConnectionResponse `json:"connections"`
}

// EndpointResponse represents an endpoint in the API
type EndpointResponse struct {
	Method       string  `json:"method"`
	Path         string  `json:"path"`
	RequestCount int64   `json:"request_count"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
	P50Ms        float64 `json:"p50_ms"`
	P99Ms        float64 `json:"p99_ms"`
	AvgMs        float64 `json:"avg_ms"`
}

// ConnectionResponse represents a connection in the API
type ConnectionResponse struct {
	Process string `json:"process"`
	PID     uint32 `json:"pid"`
	Remote  string `json:"remote"`
	Port    uint16 `json:"port"`
	Count   int64  `json:"count"`
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()

	resp := StatsResponse{
		UpdatedAt:        time.Now().Format(time.RFC3339),
		TotalConnections: stats.TotalConnections,
		TotalRequests:    stats.TotalRequests,
		TotalErrors:      stats.TotalErrors,
		Endpoints:        make([]EndpointResponse, 0, len(stats.Endpoints)),
		Connections:      make([]ConnectionResponse, 0, len(stats.Connections)),
	}

	for _, ep := range stats.Endpoints {
		errRate := float64(0)
		if ep.RequestCount > 0 {
			errRate = float64(ep.ErrorCount) / float64(ep.RequestCount) * 100
		}
		resp.Endpoints = append(resp.Endpoints, EndpointResponse{
			Method:       ep.Method,
			Path:         ep.Path,
			RequestCount: ep.RequestCount,
			ErrorCount:   ep.ErrorCount,
			ErrorRate:    errRate,
			P50Ms:        float64(ep.P50.Microseconds()) / 1000,
			P99Ms:        float64(ep.P99.Microseconds()) / 1000,
			AvgMs:        float64(ep.Avg.Microseconds()) / 1000,
		})
	}

	for _, conn := range stats.Connections {
		resp.Connections = append(resp.Connections, ConnectionResponse{
			Process: conn.Comm,
			PID:     conn.PID,
			Remote:  conn.Remote,
			Port:    conn.Port,
			Count:   conn.Count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleEndpoints(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()
	endpoints := make([]EndpointResponse, 0, len(stats.Endpoints))

	for _, ep := range stats.Endpoints {
		errRate := float64(0)
		if ep.RequestCount > 0 {
			errRate = float64(ep.ErrorCount) / float64(ep.RequestCount) * 100
		}
		endpoints = append(endpoints, EndpointResponse{
			Method:       ep.Method,
			Path:         ep.Path,
			RequestCount: ep.RequestCount,
			ErrorCount:   ep.ErrorCount,
			ErrorRate:    errRate,
			P50Ms:        float64(ep.P50.Microseconds()) / 1000,
			P99Ms:        float64(ep.P99.Microseconds()) / 1000,
			AvgMs:        float64(ep.Avg.Microseconds()) / 1000,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(endpoints)
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()
	connections := make([]ConnectionResponse, 0, len(stats.Connections))

	for _, conn := range stats.Connections {
		connections = append(connections, ConnectionResponse{
			Process: conn.Comm,
			PID:     conn.PID,
			Remote:  conn.Remote,
			Port:    conn.Port,
			Count:   conn.Count,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connections)
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	sysMetrics := s.metrics.Collect()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sysMetrics)
}

// ServiceMapNode represents a node in the service map
type ServiceMapNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "process" or "external"
}

// ServiceMapLink represents a connection between nodes
type ServiceMapLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Count  int64  `json:"count"`
}

// ServiceMapResponse is the response for the service map API
type ServiceMapResponse struct {
	Nodes []ServiceMapNode `json:"nodes"`
	Links []ServiceMapLink `json:"links"`
}

// buildServiceMapData constructs service map data for WebSocket broadcasts
func (s *Server) buildServiceMapData() *ServiceMapResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.agg.GetStats()

	nodeMap := make(map[string]ServiceMapNode)
	linkMap := make(map[string]ServiceMapLink)

	// Add nodes and links from TCP connections (eBPF data)
	for _, conn := range stats.Connections {
		sourceID := conn.Comm
		if _, exists := nodeMap[sourceID]; !exists {
			nodeMap[sourceID] = ServiceMapNode{
				ID:   sourceID,
				Name: conn.Comm,
				Type: "process",
			}
		}

		targetID := fmt.Sprintf("%s:%d", conn.Remote, conn.Port)
		if _, exists := nodeMap[targetID]; !exists {
			nodeMap[targetID] = ServiceMapNode{
				ID:   targetID,
				Name: targetID,
				Type: "external",
			}
		}

		linkKey := sourceID + "->" + targetID
		linkMap[linkKey] = ServiceMapLink{
			Source: sourceID,
			Target: targetID,
			Count:  conn.Count,
		}
	}

	// Add nodes and links from distributed traces (if available)
	if s.traceStore != nil {
		deps, err := s.traceStore.GetServiceDependencies()
		if err == nil {
			for _, dep := range deps {
				if _, exists := nodeMap[dep.Parent]; !exists {
					nodeMap[dep.Parent] = ServiceMapNode{
						ID:   dep.Parent,
						Name: dep.Parent,
						Type: "service",
					}
				}
				if _, exists := nodeMap[dep.Child]; !exists {
					nodeMap[dep.Child] = ServiceMapNode{
						ID:   dep.Child,
						Name: dep.Child,
						Type: "service",
					}
				}

				linkKey := dep.Parent + "->" + dep.Child
				if existing, exists := linkMap[linkKey]; exists {
					existing.Count += dep.CallCount
					linkMap[linkKey] = existing
				} else {
					linkMap[linkKey] = ServiceMapLink{
						Source: dep.Parent,
						Target: dep.Child,
						Count:  dep.CallCount,
					}
				}
			}
		}
	}

	nodes := make([]ServiceMapNode, 0, len(nodeMap))
	for _, node := range nodeMap {
		nodes = append(nodes, node)
	}

	links := make([]ServiceMapLink, 0, len(linkMap))
	for _, link := range linkMap {
		links = append(links, link)
	}

	return &ServiceMapResponse{
		Nodes: nodes,
		Links: links,
	}
}

func (s *Server) handleServiceMap(w http.ResponseWriter, r *http.Request) {
	resp := s.buildServiceMapData()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ProcessInfo holds information about a running process
type ProcessInfo struct {
	PID     int     `json:"pid"`
	Name    string  `json:"name"`
	CPUPct  float64 `json:"cpu_pct"`
	MemMB   float64 `json:"mem_mb"`
	State   string  `json:"state"`
	Threads int     `json:"threads"`
}

func (s *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	processes := getTopProcesses(20)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(processes)
}

func getTopProcesses(limit int) []ProcessInfo {
	var processes []ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return processes
	}

	clkTck := float64(100) // Usually 100 Hz on Linux

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc := readProcessInfo(pid, clkTck)
		if proc.Name != "" {
			processes = append(processes, proc)
		}
	}

	// Sort by CPU usage descending
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].CPUPct > processes[j].CPUPct
	})

	if len(processes) > limit {
		processes = processes[:limit]
	}

	return processes
}

func readProcessInfo(pid int, clkTck float64) ProcessInfo {
	proc := ProcessInfo{PID: pid}

	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return proc
	}

	statStr := string(statData)
	start := strings.Index(statStr, "(")
	end := strings.LastIndex(statStr, ")")
	if start == -1 || end == -1 {
		return proc
	}

	proc.Name = statStr[start+1 : end]
	fields := strings.Fields(statStr[end+2:])
	if len(fields) < 20 {
		return proc
	}

	proc.State = fields[0]
	utime, _ := strconv.ParseFloat(fields[11], 64)
	stime, _ := strconv.ParseFloat(fields[12], 64)
	proc.Threads, _ = strconv.Atoi(fields[17])

	statmPath := filepath.Join("/proc", strconv.Itoa(pid), "statm")
	statmData, err := os.ReadFile(statmPath)
	if err == nil {
		statmFields := strings.Fields(string(statmData))
		if len(statmFields) > 1 {
			rss, _ := strconv.ParseFloat(statmFields[1], 64)
			proc.MemMB = (rss * 4096) / (1024 * 1024)
		}
	}

	uptimeData, err := os.ReadFile("/proc/uptime")
	if err == nil {
		uptimeFields := strings.Fields(string(uptimeData))
		if len(uptimeFields) > 0 {
			uptime, _ := strconv.ParseFloat(uptimeFields[0], 64)
			starttime, _ := strconv.ParseFloat(fields[19], 64)
			processAge := uptime - (starttime / clkTck)
			if processAge > 0 {
				totalCPU := (utime + stime) / clkTck
				proc.CPUPct = (totalCPU / processAge) * 100
			}
		}
	}

	return proc
}

func (s *Server) handleFlameGraph(w http.ResponseWriter, r *http.Request) {
	if s.profiler == nil {
		http.Error(w, "Profiler not available", http.StatusServiceUnavailable)
		return
	}

	data, err := s.profiler.GetFlameGraph()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleFlameGraphClear(w http.ResponseWriter, r *http.Request) {
	if s.profiler == nil {
		http.Error(w, "Profiler not available", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.profiler.ClearSamples(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "cleared"})
}

func (s *Server) handleHistorySystem(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "Storage not available", http.StatusServiceUnavailable)
		return
	}

	// Parse duration from query param (default 1 hour)
	durationStr := r.URL.Query().Get("duration")
	duration := time.Hour
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}

	// Cap at 24 hours
	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}

	data, err := s.store.GetSystemMetrics(duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Downsample for performance - target ~100 points max
	data = downsampleSystemMetrics(data, 100)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// downsampleSystemMetrics reduces data points by averaging groups
func downsampleSystemMetrics(data []storage.SystemMetricPoint, maxPoints int) []storage.SystemMetricPoint {
	if len(data) <= maxPoints {
		return data
	}

	step := len(data) / maxPoints
	if step < 1 {
		step = 1
	}

	result := make([]storage.SystemMetricPoint, 0, maxPoints)
	for i := 0; i < len(data); i += step {
		end := i + step
		if end > len(data) {
			end = len(data)
		}

		// Average the group
		var p storage.SystemMetricPoint
		p.Timestamp = data[i].Timestamp // Use first timestamp
		count := float64(end - i)
		for j := i; j < end; j++ {
			p.CPUPercent += data[j].CPUPercent
			p.MemPercent += data[j].MemPercent
			p.DiskReadPS += data[j].DiskReadPS
			p.DiskWritePS += data[j].DiskWritePS
			p.NetRxPS += data[j].NetRxPS
			p.NetTxPS += data[j].NetTxPS
			p.Load1 += data[j].Load1
		}
		p.CPUPercent /= count
		p.MemPercent /= count
		p.DiskReadPS /= count
		p.DiskWritePS /= count
		p.NetRxPS /= count
		p.NetTxPS /= count
		p.Load1 /= count

		result = append(result, p)
	}
	return result
}

func (s *Server) handleHistoryConnections(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "Storage not available", http.StatusServiceUnavailable)
		return
	}

	durationStr := r.URL.Query().Get("duration")
	duration := time.Hour
	if durationStr != "" {
		if d, err := time.ParseDuration(durationStr); err == nil {
			duration = d
		}
	}

	if duration > 24*time.Hour {
		duration = 24 * time.Hour
	}

	data, err := s.store.GetConnectionMetrics(duration)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Downsample for performance
	data = downsampleConnectionMetrics(data, 100)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// downsampleConnectionMetrics reduces connection data points
func downsampleConnectionMetrics(data []storage.ConnectionMetricPoint, maxPoints int) []storage.ConnectionMetricPoint {
	if len(data) <= maxPoints {
		return data
	}

	step := len(data) / maxPoints
	if step < 1 {
		step = 1
	}

	result := make([]storage.ConnectionMetricPoint, 0, maxPoints)
	for i := 0; i < len(data); i += step {
		end := i + step
		if end > len(data) {
			end = len(data)
		}

		// Use last values in the group (cumulative metrics)
		result = append(result, data[end-1])
	}
	return result
}

func (s *Server) handleHistoryInfo(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		http.Error(w, "Storage not available", http.StatusServiceUnavailable)
		return
	}

	// Get the oldest data point to show when collection started
	data, err := s.store.GetSystemMetrics(24 * time.Hour)
	if err != nil || len(data) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"collecting": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"collecting":  true,
		"since":       data[0].Timestamp,
		"data_points": len(data),
	})
}

// Trace handlers

func (s *Server) handleOTLPTraces(w http.ResponseWriter, r *http.Request) {
	if s.otlpReceiver == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}
	s.otlpReceiver.HandleTraces(w, r)
}

func (s *Server) handleSimpleTrace(w http.ResponseWriter, r *http.Request) {
	if s.otlpReceiver == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}
	s.otlpReceiver.HandleSimpleTrace(w, r)
}

func (s *Server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	if s.traceStore == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}

	// Parse pagination params
	params := ParsePaginationParams(r)

	service := r.URL.Query().Get("service")

	since := time.Hour
	if d := r.URL.Query().Get("duration"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			since = parsed
		}
	}

	// Fetch one extra to determine if there are more
	traces, err := s.traceStore.ListTraces(params.Limit+1, service, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine if there are more results
	hasMore := len(traces) > params.Limit
	if hasMore {
		traces = traces[:params.Limit]
	}

	// Track usage
	svc := service
	if svc == "" {
		svc = "_all"
	}
	TrackTraceQuery(svc, "api", len(traces))

	// Set pagination headers
	w.Header().Set("X-Page-Size", strconv.Itoa(params.Limit))
	w.Header().Set("X-Has-More", strconv.FormatBool(hasMore))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": traces,
		"pagination": map[string]interface{}{
			"limit":    params.Limit,
			"has_more": hasMore,
		},
	})
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	if s.traceStore == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}

	// Extract trace ID from path /api/traces/{traceID}
	traceID := strings.TrimPrefix(r.URL.Path, "/api/traces/")
	if traceID == "" {
		http.Error(w, "Trace ID required", http.StatusBadRequest)
		return
	}

	trace, err := s.traceStore.GetTrace(traceID)
	if err != nil {
		http.Error(w, "Trace not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

func (s *Server) handleTraceServices(w http.ResponseWriter, r *http.Request) {
	if s.traceStore == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}

	services, err := s.traceStore.GetServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (s *Server) handleTraceDependencies(w http.ResponseWriter, r *http.Request) {
	if s.traceStore == nil {
		http.Error(w, "Tracing not enabled", http.StatusServiceUnavailable)
		return
	}

	deps, err := s.traceStore.GetServiceDependencies()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deps)
}

// Watch handlers

func (s *Server) handleWatches(w http.ResponseWriter, r *http.Request) {
	if s.watchStore == nil {
		http.Error(w, "Watches not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		watches, err := s.watchStore.ListWatches()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(watches)

	case http.MethodPost:
		var req watch.Watch
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			req.ID = uuid.New().String()
		}
		if req.State == "" {
			req.State = watch.StateNoData
		}
		if err := s.watchStore.SaveWatch(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Trigger immediate check
		if s.watchEngine != nil {
			s.watchEngine.ForceCheck()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWatch(w http.ResponseWriter, r *http.Request) {
	if s.watchStore == nil {
		http.Error(w, "Watches not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/watches/")
	if id == "" {
		http.Error(w, "Watch ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		watch, err := s.watchStore.GetWatch(id)
		if err != nil {
			http.Error(w, "Watch not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(watch)

	case http.MethodPut:
		var req watch.Watch
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.ID = id
		if err := s.watchStore.SaveWatch(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.watchEngine != nil {
			s.watchEngine.ForceCheck()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)

	case http.MethodDelete:
		if err := s.watchStore.DeleteWatch(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if s.watchStore == nil {
		http.Error(w, "Watches not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		channels, err := s.watchStore.ListChannels()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(channels)

	case http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		var req watch.Channel
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID == "" {
			req.ID = uuid.New().String()
		}
		if err := s.watchStore.SaveChannel(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleChannel(w http.ResponseWriter, r *http.Request) {
	if s.watchStore == nil {
		http.Error(w, "Watches not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/watch/channels/")

	// Handle test endpoint
	if strings.HasSuffix(id, "/test") {
		id = strings.TrimSuffix(id, "/test")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		channel, err := s.watchStore.GetChannel(id)
		if err != nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}
		notifier := watch.NewNotifier()
		if err := notifier.TestChannel(channel); err != nil {
			http.Error(w, "Test failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	if id == "" {
		http.Error(w, "Channel ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		channel, err := s.watchStore.GetChannel(id)
		if err != nil {
			http.Error(w, "Channel not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(channel)

	case http.MethodDelete:
		if err := s.watchStore.DeleteChannel(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWatchEvents(w http.ResponseWriter, r *http.Request) {
	if s.watchStore == nil {
		http.Error(w, "Watches not enabled", http.StatusServiceUnavailable)
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	watchID := r.URL.Query().Get("watch_id")

	events, err := s.watchStore.GetEvents(limit, watchID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *Server) handleWatchMetrics(w http.ResponseWriter, r *http.Request) {
	// Return list of available metrics for watches
	metrics := []map[string]string{
		{"id": "cpu_percent", "name": "CPU Usage", "unit": "%"},
		{"id": "mem_percent", "name": "Memory Usage", "unit": "%"},
		{"id": "disk_read_ps", "name": "Disk Read", "unit": "B/s"},
		{"id": "disk_write_ps", "name": "Disk Write", "unit": "B/s"},
		{"id": "net_rx_ps", "name": "Network RX", "unit": "B/s"},
		{"id": "net_tx_ps", "name": "Network TX", "unit": "B/s"},
		{"id": "load_1", "name": "Load Average (1m)", "unit": ""},
		{"id": "connections", "name": "Total Connections", "unit": ""},
		{"id": "requests", "name": "Total Requests", "unit": ""},
		{"id": "errors", "name": "Total Errors", "unit": ""},
		{"id": "error_rate", "name": "Error Rate", "unit": "%"},
		{"id": "latency_p50", "name": "Latency P50", "unit": "ms"},
		{"id": "latency_p99", "name": "Latency P99", "unit": "ms"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// Dashboard handlers

func (s *Server) handleDashboards(w http.ResponseWriter, r *http.Request) {
	if s.dashboardStore == nil {
		http.Error(w, "Dashboard storage not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		dashboards, err := s.dashboardStore.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dashboards)

	case http.MethodPost:
		var req struct {
			Name      string                     `json:"name"`
			Layout    []dashboard.WidgetPosition `json:"layout"`
			IsDefault bool                       `json:"is_default"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		dash, err := s.dashboardStore.Create(req.Name, req.Layout, req.IsDefault)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dash)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if s.dashboardStore == nil {
		http.Error(w, "Dashboard storage not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/dashboards/")

	// Handle set-default endpoint
	if strings.HasSuffix(id, "/default") {
		id = strings.TrimSuffix(id, "/default")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := s.dashboardStore.SetDefault(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	if id == "" {
		http.Error(w, "Dashboard ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		dash, err := s.dashboardStore.Get(id)
		if err != nil || dash == nil {
			http.Error(w, "Dashboard not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dash)

	case http.MethodPut:
		var req struct {
			Name   string                     `json:"name"`
			Layout []dashboard.WidgetPosition `json:"layout"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.dashboardStore.Update(id, req.Name, req.Layout); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dash, _ := s.dashboardStore.Get(id)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dash)

	case http.MethodDelete:
		if err := s.dashboardStore.Delete(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDefaultDashboard(w http.ResponseWriter, r *http.Request) {
	if s.dashboardStore == nil {
		http.Error(w, "Dashboard storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dash, err := s.dashboardStore.GetDefault()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if dash == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dash)
}

// Log handlers

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination params
	params := ParsePaginationParams(r)

	// Parse query parameters
	q := logs.SearchQuery{
		Query:   r.URL.Query().Get("q"),
		Service: r.URL.Query().Get("service"),
		TraceID: r.URL.Query().Get("trace_id"),
		Limit:   params.Limit + 1, // Fetch one extra to check has_more
		Offset:  params.Offset,
	}

	// Parse sort order (relevance or time)
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		switch sortBy {
		case "relevance":
			q.SortBy = logs.SortByRelevance
		case "time":
			q.SortBy = logs.SortByTime
		}
	}

	if level := r.URL.Query().Get("level"); level != "" {
		q.Level = logs.LogLevel(level)
	}

	// Time range - support both 'since'/'until' (duration) and 'start'/'end' (ISO timestamps)
	if start := r.URL.Query().Get("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			q.StartTime = t
		}
	} else if since := r.URL.Query().Get("since"); since != "" {
		if d, err := time.ParseDuration(since); err == nil {
			q.StartTime = time.Now().Add(-d)
		}
	} else {
		q.StartTime = time.Now().Add(-time.Hour)
	}

	if end := r.URL.Query().Get("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			q.EndTime = t
		}
	} else if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			q.EndTime = t
		}
	}

	result, err := s.logStore.Search(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Determine if there are more results
	hasMore := len(result.Entries) > params.Limit
	if hasMore {
		result.Entries = result.Entries[:params.Limit]
	}

	// Track usage
	service := q.Service
	if service == "" {
		service = "_all"
	}
	TrackLogQuery(service, "api", len(result.Entries))

	// Set pagination headers
	w.Header().Set("X-Total-Count", strconv.Itoa(result.TotalCount))
	w.Header().Set("X-Page-Size", strconv.Itoa(params.Limit))
	w.Header().Set("X-Has-More", strconv.FormatBool(hasMore))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data":  result.Entries,
		"total": result.TotalCount,
		"pagination": map[string]interface{}{
			"limit":    params.Limit,
			"offset":   params.Offset,
			"has_more": hasMore,
			"total":    result.TotalCount,
		},
	})
}

func (s *Server) handleLogIngest(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")

	// Handle batch ingestion (array of logs)
	if strings.Contains(contentType, "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		// Try to parse as array first
		var entries []logs.LogEntry
		if err := json.Unmarshal(body, &entries); err == nil && len(entries) > 0 {
			if err := s.logStore.InsertBatch(entries); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Process logs for pattern detection (LogReduce)
			ProcessLogsForPatterns(entries)
			// Broadcast to WebSocket subscribers (real-time log streaming)
			if s.wsHub != nil && s.wsHub.TopicSubscriberCount(TopicLogs) > 0 {
				for _, entry := range entries {
					s.wsHub.BroadcastLog(map[string]interface{}{
						"message":   entry.Message,
						"level":     entry.Level,
						"service":   entry.Service,
						"host":      entry.Host,
						"timestamp": entry.Timestamp,
					})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"ingested": len(entries)})
			return
		}

		// Try single entry
		var entry logs.LogEntry
		if err := json.Unmarshal(body, &entry); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := s.logStore.Insert(&entry); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Process log for pattern detection (LogReduce)
		ProcessLogsForPatterns([]logs.LogEntry{entry})
		// Broadcast to WebSocket subscribers
		if s.wsHub != nil && s.wsHub.TopicSubscriberCount(TopicLogs) > 0 {
			s.wsHub.BroadcastLog(map[string]interface{}{
				"message":   entry.Message,
				"level":     entry.Level,
				"service":   entry.Service,
				"host":      entry.Host,
				"timestamp": entry.Timestamp,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"ingested": 1})
		return
	}

	// Handle plain text (syslog-style)
	if strings.Contains(contentType, "text/plain") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		lines := strings.Split(string(body), "\n")
		var entries []logs.LogEntry
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			entry := logs.LogEntry{
				Message: line,
				Level:   logs.LevelInfo,
				Service: r.URL.Query().Get("service"),
				Host:    r.URL.Query().Get("host"),
			}

			// Try to detect log level from message
			lowerLine := strings.ToLower(line)
			if strings.Contains(lowerLine, "error") || strings.Contains(lowerLine, "err]") {
				entry.Level = logs.LevelError
			} else if strings.Contains(lowerLine, "warn") {
				entry.Level = logs.LevelWarn
			} else if strings.Contains(lowerLine, "debug") {
				entry.Level = logs.LevelDebug
			}

			entries = append(entries, entry)
		}

		if len(entries) > 0 {
			if err := s.logStore.InsertBatch(entries); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Broadcast to WebSocket subscribers
			if s.wsHub != nil && s.wsHub.TopicSubscriberCount(TopicLogs) > 0 {
				for _, entry := range entries {
					s.wsHub.BroadcastLog(map[string]interface{}{
						"message":   entry.Message,
						"level":     entry.Level,
						"service":   entry.Service,
						"host":      entry.Host,
						"timestamp": entry.Timestamp,
					})
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"ingested": len(entries)})
		return
	}

	http.Error(w, "Unsupported content type", http.StatusBadRequest)
}

func (s *Server) handleLogServices(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	services, err := s.logStore.GetServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (s *Server) handleLogStats(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	since := time.Hour
	if d := r.URL.Query().Get("since"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			since = parsed
		}
	}

	stats, err := s.logStore.GetStats(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleLogPatterns(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Query parameters
	filter := r.URL.Query().Get("filter") // "all", "new", "increasing"
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	var patterns []*logs.Pattern

	switch filter {
	case "new":
		since := 24 * time.Hour
		if d := r.URL.Query().Get("since"); d != "" {
			if parsed, err := time.ParseDuration(d); err == nil {
				since = parsed
			}
		}
		patterns = s.logStore.GetNewPatterns(since)
	case "increasing":
		patterns = s.logStore.GetIncreasingPatterns()
	default:
		patterns = s.logStore.GetTopPatterns(limit)
	}

	// Get stats too
	stats := s.logStore.GetPatternStats()

	response := map[string]interface{}{
		"patterns": patterns,
		"stats":    stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleLogPattern(w http.ResponseWriter, r *http.Request) {
	if s.logStore == nil {
		http.Error(w, "Log storage not enabled", http.StatusServiceUnavailable)
		return
	}

	// Extract pattern ID from path
	patternID := strings.TrimPrefix(r.URL.Path, "/api/logs/patterns/")
	if patternID == "" {
		http.Error(w, "Pattern ID required", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pattern := s.logStore.GetPattern(patternID)
	if pattern == nil {
		http.Error(w, "Pattern not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pattern)
}

// Container handlers

func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	if s.containerCollector == nil {
		http.Error(w, "Container monitoring not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	containerList := s.containerCollector.GetContainers()
	stats := s.containerCollector.GetStats()

	// Combine containers with their stats
	type ContainerWithStats struct {
		Container *containers.Container      `json:"container"`
		Stats     *containers.ContainerStats `json:"stats"`
	}

	result := make([]ContainerWithStats, 0, len(containerList))
	for _, c := range containerList {
		result = append(result, ContainerWithStats{
			Container: c,
			Stats:     stats[c.ID],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleContainer(w http.ResponseWriter, r *http.Request) {
	if s.containerCollector == nil {
		http.Error(w, "Container monitoring not enabled", http.StatusServiceUnavailable)
		return
	}

	// Extract container ID from path
	containerID := strings.TrimPrefix(r.URL.Path, "/api/containers/")
	if containerID == "" || containerID == "summary" {
		// Let summary handler handle /api/containers/summary
		if containerID == "summary" {
			s.handleContainerSummary(w, r)
			return
		}
		http.Error(w, "Container ID required", http.StatusBadRequest)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	container := s.containerCollector.GetContainer(containerID)
	if container == nil {
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	stats := s.containerCollector.GetContainerStats(containerID)

	// Get history if requested
	var history []containers.ContainerStats
	if historyParam := r.URL.Query().Get("history"); historyParam != "" {
		if duration, err := time.ParseDuration(historyParam); err == nil {
			history = s.containerCollector.GetHistory(containerID, duration)
		}
	}

	result := map[string]interface{}{
		"container": container,
		"stats":     stats,
	}
	if len(history) > 0 {
		result["history"] = history
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleContainerSummary(w http.ResponseWriter, r *http.Request) {
	if s.containerCollector == nil {
		http.Error(w, "Container monitoring not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	summary := s.containerCollector.GetSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

// Deployment handlers

func (s *Server) handleDeploys(w http.ResponseWriter, r *http.Request) {
	if s.deployStore == nil {
		http.Error(w, "Deployment tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List deployments
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		service := r.URL.Query().Get("service")
		var deployments []deploys.Deployment
		var err error

		if service != "" {
			deployments, err = s.deployStore.ListByService(service, limit)
		} else {
			deployments, err = s.deployStore.List(limit)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deployments)

	case http.MethodPost:
		// Record new deployment
		var deploy deploys.Deployment
		if err := json.NewDecoder(r.Body).Decode(&deploy); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if deploy.Service == "" {
			http.Error(w, "Service name is required", http.StatusBadRequest)
			return
		}
		if deploy.Version == "" {
			http.Error(w, "Version is required", http.StatusBadRequest)
			return
		}

		if err := s.deployStore.Record(&deploy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(deploy)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if s.deployStore == nil {
		http.Error(w, "Deployment tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	// Extract deploy ID from path
	deployID := strings.TrimPrefix(r.URL.Path, "/api/deploys/")
	if deployID == "" || deployID == "stats" || deployID == "services" {
		if deployID == "stats" {
			s.handleDeployStats(w, r)
			return
		}
		if deployID == "services" {
			s.handleDeployServices(w, r)
			return
		}
		http.Error(w, "Deploy ID required", http.StatusBadRequest)
		return
	}

	// Check for impact endpoint: /api/deploys/{id}/impact
	if strings.HasSuffix(deployID, "/impact") {
		deployID = strings.TrimSuffix(deployID, "/impact")
		s.handleDeployImpact(w, r, deployID)
		return
	}

	switch r.Method {
	case http.MethodGet:
		deploy, err := s.deployStore.Get(deployID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if deploy == nil {
			http.Error(w, "Deployment not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deploy)

	case http.MethodPatch:
		// Update status
		var update struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := s.deployStore.UpdateStatus(deployID, update.Status); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)

	case http.MethodDelete:
		if err := s.deployStore.Delete(deployID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDeployStats(w http.ResponseWriter, r *http.Request) {
	if s.deployStore == nil {
		http.Error(w, "Deployment tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.deployStore.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleDeployServices(w http.ResponseWriter, r *http.Request) {
	if s.deployStore == nil {
		http.Error(w, "Deployment tracking not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	services, err := s.deployStore.GetServices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// handleDeployImpact calculates the metric impact of a deployment
func (s *Server) handleDeployImpact(w http.ResponseWriter, r *http.Request, deployID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the deployment
	deploy, err := s.deployStore.Get(deployID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if deploy == nil {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Get window duration from query params (default 30 minutes)
	windowStr := r.URL.Query().Get("window")
	window := 30 * time.Minute
	if windowStr != "" {
		if d, err := time.ParseDuration(windowStr); err == nil {
			window = d
		}
	}

	// Calculate before/after time windows
	beforeStart := deploy.Timestamp.Add(-window)
	beforeEnd := deploy.Timestamp
	afterStart := deploy.Timestamp
	afterEnd := deploy.Timestamp.Add(window)

	impact := deploys.DeploymentImpact{
		DeploymentID: deployID,
		Impact:       "neutral",
	}

	// Get connection metrics if storage is available
	if s.store != nil {
		// Before metrics
		beforeMetrics, err := s.store.GetConnectionMetricsByTimeRange(beforeStart, beforeEnd)
		if err == nil && len(beforeMetrics) > 0 {
			// Sum up errors and requests for the before window
			var totalErrors, totalRequests int64
			for _, m := range beforeMetrics {
				totalErrors += m.TotalErrors
				totalRequests += m.TotalRequests
			}
			impact.ErrorsBefore = int(totalErrors)
			impact.RequestsBefore = int(totalRequests)
		}

		// After metrics
		afterMetrics, err := s.store.GetConnectionMetricsByTimeRange(afterStart, afterEnd)
		if err == nil && len(afterMetrics) > 0 {
			var totalErrors, totalRequests int64
			for _, m := range afterMetrics {
				totalErrors += m.TotalErrors
				totalRequests += m.TotalRequests
			}
			impact.ErrorsAfter = int(totalErrors)
			impact.RequestsAfter = int(totalRequests)
		}

		// Calculate error rate change
		if impact.RequestsBefore > 0 && impact.RequestsAfter > 0 {
			beforeRate := float64(impact.ErrorsBefore) / float64(impact.RequestsBefore) * 100
			afterRate := float64(impact.ErrorsAfter) / float64(impact.RequestsAfter) * 100
			if beforeRate > 0 {
				impact.ErrorRateChange = ((afterRate - beforeRate) / beforeRate) * 100
			} else if afterRate > 0 {
				impact.ErrorRateChange = 100 // Went from 0 to some errors
			}
		}

		// Get system metrics for latency-like info (using CPU as a proxy)
		beforeSys, _ := s.store.GetSystemMetricsByTimeRange(beforeStart, beforeEnd)
		afterSys, _ := s.store.GetSystemMetricsByTimeRange(afterStart, afterEnd)

		if len(beforeSys) > 0 {
			var sum float64
			for _, m := range beforeSys {
				sum += m.CPUPercent
			}
			impact.LatencyP50Before = sum / float64(len(beforeSys))
		}
		if len(afterSys) > 0 {
			var sum float64
			for _, m := range afterSys {
				sum += m.CPUPercent
			}
			impact.LatencyP50After = sum / float64(len(afterSys))
		}

		if impact.LatencyP50Before > 0 {
			impact.LatencyChange = ((impact.LatencyP50After - impact.LatencyP50Before) / impact.LatencyP50Before) * 100
		}

		// Determine overall impact
		if impact.ErrorRateChange > 10 || impact.LatencyChange > 20 {
			impact.Impact = "negative"
		} else if impact.ErrorRateChange < -10 || impact.LatencyChange < -10 {
			impact.Impact = "positive"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(impact)
}

// Custom metrics handlers

func (s *Server) handleOTLPMetrics(w http.ResponseWriter, r *http.Request) {
	if s.otlpMetricsReceiver == nil {
		http.Error(w, "Custom metrics not enabled", http.StatusServiceUnavailable)
		return
	}
	s.otlpMetricsReceiver.HandleMetrics(w, r)
}

func (s *Server) handleMetricsPush(w http.ResponseWriter, r *http.Request) {
	if s.customMetricsStore == nil {
		http.Error(w, "Custom metrics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// Try array first
	var points []custommetrics.DataPoint
	if err := json.Unmarshal(body, &points); err == nil && len(points) > 0 {
		if err := s.customMetricsStore.RecordBatch(points); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"accepted": len(points)})
		return
	}

	// Try single point
	var point custommetrics.DataPoint
	if err := json.Unmarshal(body, &point); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if point.Name == "" {
		http.Error(w, "Metric name is required", http.StatusBadRequest)
		return
	}

	if point.Type == "" {
		point.Type = custommetrics.Gauge
	}

	if err := s.customMetricsStore.Record(point); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"accepted": 1})
}

func (s *Server) handleMetricsQuery(w http.ResponseWriter, r *http.Request) {
	if s.customMetricsStore == nil {
		http.Error(w, "Custom metrics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Metric name is required", http.StatusBadRequest)
		return
	}

	since := time.Hour
	if d := r.URL.Query().Get("since"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			since = parsed
		}
	}

	series, err := s.customMetricsStore.Query(name, nil, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Track usage
	resultSize := 0
	if series != nil {
		resultSize = len(series.Points)
	}
	TrackMetricQuery(name, "api", resultSize)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(series)
}

func (s *Server) handleMetricsList(w http.ResponseWriter, r *http.Request) {
	if s.customMetricsStore == nil {
		http.Error(w, "Custom metrics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics, err := s.customMetricsStore.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// handleMetricsHistogram returns native histogram data for latency distribution visualization
func (s *Server) handleMetricsHistogram(w http.ResponseWriter, r *http.Request) {
	if s.customMetricsStore == nil {
		http.Error(w, "Custom metrics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "latency" // default metric
	}

	// Parse time range (default 1h)
	rangeStr := r.URL.Query().Get("range")
	duration := time.Hour
	if rangeStr != "" {
		if d, err := time.ParseDuration(rangeStr); err == nil {
			duration = d
		}
	}

	end := time.Now()
	start := end.Add(-duration)

	// Try to get native histogram data
	snapshot, err := s.customMetricsStore.QueryHistogramSnapshot(metric, nil, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response for the histogram-chart component
	response := struct {
		Buckets     []string           `json:"buckets"`
		Counts      []uint64           `json:"counts"`
		Percentiles map[string]float64 `json:"percentiles"`
		Total       uint64             `json:"total"`
	}{
		Buckets:     []string{},
		Counts:      []uint64{},
		Percentiles: make(map[string]float64),
		Total:       0,
	}

	if snapshot != nil && snapshot.TotalCount > 0 {
		// Convert bounds to bucket labels
		for i, bound := range snapshot.Bounds {
			var label string
			if i == 0 {
				label = formatDuration(0, bound)
			} else {
				label = formatDuration(snapshot.Bounds[i-1], bound)
			}
			response.Buckets = append(response.Buckets, label)
		}
		// Add +Inf bucket if there are more counts than bounds
		if len(snapshot.Counts) > len(snapshot.Bounds) {
			if len(snapshot.Bounds) > 0 {
				response.Buckets = append(response.Buckets, formatDuration(snapshot.Bounds[len(snapshot.Bounds)-1], 0)+" +")
			} else {
				response.Buckets = append(response.Buckets, "all")
			}
		}

		// Convert cumulative counts to per-bucket counts
		var prevCount uint64
		for _, count := range snapshot.Counts {
			bucketCount := count - prevCount
			response.Counts = append(response.Counts, bucketCount)
			prevCount = count
		}

		response.Total = snapshot.TotalCount

		// Compute percentiles from native histogram
		response.Percentiles["p50"] = snapshot.Quantile(0.50)
		response.Percentiles["p90"] = snapshot.Quantile(0.90)
		response.Percentiles["p95"] = snapshot.Quantile(0.95)
		response.Percentiles["p99"] = snapshot.Quantile(0.99)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// formatDuration formats a duration range for histogram bucket labels
func formatDuration(low, high float64) string {
	formatVal := func(v float64) string {
		if v < 0.001 {
			return fmt.Sprintf("%.0fus", v*1e6)
		} else if v < 1 {
			return fmt.Sprintf("%.0fms", v*1e3)
		} else {
			return fmt.Sprintf("%.1fs", v)
		}
	}

	if high == 0 {
		return formatVal(low)
	}
	return fmt.Sprintf("%s-%s", formatVal(low), formatVal(high))
}

// handlePrometheusRemoteWrite handles Prometheus remote write requests
func (s *Server) handlePrometheusRemoteWrite(w http.ResponseWriter, r *http.Request) {
	if s.prometheusReceiver == nil {
		http.Error(w, "Prometheus receiver not enabled", http.StatusServiceUnavailable)
		return
	}
	s.prometheusReceiver.HandleRemoteWrite(w, r)
}

// handlePrometheusStats returns Prometheus receiver statistics
func (s *Server) handlePrometheusStats(w http.ResponseWriter, r *http.Request) {
	if s.prometheusReceiver == nil {
		http.Error(w, "Prometheus receiver not enabled", http.StatusServiceUnavailable)
		return
	}
	s.prometheusReceiver.HandleStats(w, r)
}

// Synthetics handlers

func (s *Server) handleSyntheticsChecks(w http.ResponseWriter, r *http.Request) {
	if s.syntheticsStore == nil {
		http.Error(w, "Synthetics not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		checks, err := s.syntheticsStore.ListChecks()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(checks)

	case http.MethodPost:
		var check synthetics.Check
		if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if check.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		if check.URL == "" {
			http.Error(w, "URL is required", http.StatusBadRequest)
			return
		}
		if err := s.syntheticsStore.CreateCheck(&check); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(check)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSyntheticsCheck(w http.ResponseWriter, r *http.Request) {
	if s.syntheticsStore == nil {
		http.Error(w, "Synthetics not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/synthetics/checks/")

	// Handle run endpoint
	if strings.HasSuffix(id, "/run") {
		id = strings.TrimSuffix(id, "/run")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.syntheticsRunner == nil {
			http.Error(w, "Synthetics runner not enabled", http.StatusServiceUnavailable)
			return
		}
		result, err := s.syntheticsRunner.RunCheck(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	if id == "" {
		http.Error(w, "Check ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		check, err := s.syntheticsStore.GetCheck(id)
		if err != nil || check == nil {
			http.Error(w, "Check not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(check)

	case http.MethodPut:
		var check synthetics.Check
		if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		check.ID = id
		if err := s.syntheticsStore.UpdateCheck(&check); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(check)

	case http.MethodDelete:
		if err := s.syntheticsStore.DeleteCheck(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSyntheticsResults(w http.ResponseWriter, r *http.Request) {
	if s.syntheticsStore == nil {
		http.Error(w, "Synthetics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimPrefix(r.URL.Path, "/api/synthetics/results/")
	if checkID == "" {
		http.Error(w, "Check ID required", http.StatusBadRequest)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	results, err := s.syntheticsStore.GetResults(checkID, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) handleSyntheticsUptime(w http.ResponseWriter, r *http.Request) {
	if s.syntheticsStore == nil {
		http.Error(w, "Synthetics not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimPrefix(r.URL.Path, "/api/synthetics/uptime/")
	if checkID == "" {
		http.Error(w, "Check ID required", http.StatusBadRequest)
		return
	}

	since := 24 * time.Hour
	if d := r.URL.Query().Get("since"); d != "" {
		if parsed, err := time.ParseDuration(d); err == nil {
			since = parsed
		}
	}

	stats, err := s.syntheticsStore.GetUptime(checkID, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// SLO handlers

func (s *Server) handleSLOs(w http.ResponseWriter, r *http.Request) {
	if s.sloStore == nil {
		http.Error(w, "SLOs not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Return SLOs with their current states
		if s.sloCalculator != nil {
			slosWithState, err := s.sloCalculator.GetSLOsWithState()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(slosWithState)
		} else {
			slos, err := s.sloStore.ListSLOs()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(slos)
		}

	case http.MethodPost:
		var req slo.SLO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}
		if req.Target <= 0 || req.Target > 100 {
			http.Error(w, "Target must be between 0 and 100", http.StatusBadRequest)
			return
		}
		if err := s.sloStore.CreateSLO(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Trigger immediate calculation
		if s.sloCalculator != nil {
			s.sloCalculator.ForceCalculate(req.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSLO(w http.ResponseWriter, r *http.Request) {
	if s.sloStore == nil {
		http.Error(w, "SLOs not enabled", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/slos/")

	// Handle state endpoint
	if strings.HasSuffix(id, "/state") {
		id = strings.TrimSuffix(id, "/state")
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.sloCalculator == nil {
			http.Error(w, "SLO calculator not running", http.StatusServiceUnavailable)
			return
		}
		state := s.sloCalculator.ForceCalculate(id)
		if state == nil {
			http.Error(w, "SLO not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
		return
	}

	// Handle history endpoint
	if strings.HasSuffix(id, "/history") {
		id = strings.TrimSuffix(id, "/history")
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		since := 24 * time.Hour
		if d := r.URL.Query().Get("since"); d != "" {
			if parsed, err := time.ParseDuration(d); err == nil {
				since = parsed
			}
		}
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		snapshots, err := s.sloStore.GetSnapshots(id, since, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshots)
		return
	}

	if id == "" {
		http.Error(w, "SLO ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sloObj, err := s.sloStore.GetSLO(id)
		if err != nil || sloObj == nil {
			http.Error(w, "SLO not found", http.StatusNotFound)
			return
		}
		// Include current state if available
		var state *slo.SLOState
		if s.sloCalculator != nil {
			state = s.sloCalculator.GetState(id)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"slo":   sloObj,
			"state": state,
		})

	case http.MethodPut:
		var req slo.SLO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		req.ID = id
		if err := s.sloStore.UpdateSLO(&req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Trigger recalculation
		if s.sloCalculator != nil {
			s.sloCalculator.ForceCalculate(id)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(req)

	case http.MethodDelete:
		if err := s.sloStore.DeleteSLO(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Incident handlers

func (s *Server) handleIncidents(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		params := ParsePaginationParams(r)
		status := r.URL.Query().Get("status")

		// Fetch one extra to check for more
		incs, err := s.incidentStore.ListIncidents(status, params.Limit+1)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Determine if there are more results
		hasMore := len(incs) > params.Limit
		if hasMore {
			incs = incs[:params.Limit]
		}

		// Set pagination headers
		w.Header().Set("X-Page-Size", strconv.Itoa(params.Limit))
		w.Header().Set("X-Has-More", strconv.FormatBool(hasMore))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": incs,
			"pagination": map[string]interface{}{
				"limit":    params.Limit,
				"has_more": hasMore,
			},
		})

	case http.MethodPost:
		var inc incidents.Incident
		if err := json.NewDecoder(r.Body).Decode(&inc); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if inc.Title == "" {
			http.Error(w, "Title is required", http.StatusBadRequest)
			return
		}

		if s.pager != nil {
			if err := s.pager.Trigger(&inc); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			if err := s.incidentStore.CreateIncident(&inc); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(inc)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIncident(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/incidents/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		if parts[0] == "stats" {
			s.handleIncidentStats(w, r)
			return
		}
		http.Error(w, "Incident ID required", http.StatusBadRequest)
		return
	}

	incidentID := parts[0]

	// Handle sub-routes
	if len(parts) > 1 {
		switch parts[1] {
		case "ack":
			s.handleIncidentAck(w, r, incidentID)
			return
		case "resolve":
			s.handleIncidentResolve(w, r, incidentID)
			return
		case "note":
			s.handleIncidentNote(w, r, incidentID)
			return
		case "notifications":
			s.handleIncidentNotifications(w, r, incidentID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		inc, err := s.incidentStore.GetIncident(incidentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if inc == nil {
			http.Error(w, "Incident not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(inc)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIncidentAck(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.User = "api"
	}
	if req.User == "" {
		req.User = "api"
	}

	if err := s.incidentStore.AcknowledgeIncident(incidentID, req.User); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.pager != nil {
		s.pager.CancelEscalation(incidentID)
	}

	inc, _ := s.incidentStore.GetIncident(incidentID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

func (s *Server) handleIncidentResolve(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User       string `json:"user"`
		Resolution string `json:"resolution"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.User == "" {
		req.User = "api"
	}

	if err := s.incidentStore.ResolveIncident(incidentID, req.User, req.Resolution); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.pager != nil {
		s.pager.CancelEscalation(incidentID)
		inc, _ := s.incidentStore.GetIncident(incidentID)
		if inc != nil {
			s.pager.NotifyResolution(inc)
		}
	}

	inc, _ := s.incidentStore.GetIncident(incidentID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

func (s *Server) handleIncidentNote(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		User string `json:"user"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Note == "" {
		http.Error(w, "Note is required", http.StatusBadRequest)
		return
	}
	if req.User == "" {
		req.User = "api"
	}

	if err := s.incidentStore.AddNote(incidentID, req.User, req.Note); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	inc, _ := s.incidentStore.GetIncident(incidentID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

func (s *Server) handleIncidentNotifications(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logs, err := s.incidentStore.GetNotificationLogs(incidentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *Server) handleIncidentStats(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.incidentStore.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// On-call schedule handlers

func (s *Server) handleOnCallSchedules(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		schedules, err := s.incidentStore.ListSchedules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schedules)

	case http.MethodPost:
		var sched incidents.OnCallSchedule
		if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if sched.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		if err := s.incidentStore.CreateSchedule(&sched); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sched)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOnCallSchedule(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	schedID := strings.TrimPrefix(r.URL.Path, "/api/oncall/")
	if schedID == "" || schedID == "current" {
		s.handleCurrentOnCall(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sched, err := s.incidentStore.GetSchedule(schedID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if sched == nil {
			http.Error(w, "Schedule not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sched)

	case http.MethodDelete:
		if err := s.incidentStore.DeleteSchedule(schedID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleCurrentOnCall(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scheduleID := r.URL.Query().Get("schedule")
	if scheduleID == "" {
		// Return all schedules with current on-call
		schedules, err := s.incidentStore.ListSchedules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		result := make(map[string]string)
		for _, sched := range schedules {
			user, _ := s.incidentStore.GetCurrentOnCall(sched.ID)
			result[sched.Name] = user
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
		return
	}

	user, err := s.incidentStore.GetCurrentOnCall(scheduleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"user": user, "schedule_id": scheduleID})
}

// Escalation policy handlers

func (s *Server) handleEscalationPolicies(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		policies, err := s.incidentStore.ListPolicies()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policies)

	case http.MethodPost:
		var policy incidents.EscalationPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		if policy.Name == "" {
			http.Error(w, "Name is required", http.StatusBadRequest)
			return
		}

		if err := s.incidentStore.CreatePolicy(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(policy)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleEscalationPolicy(w http.ResponseWriter, r *http.Request) {
	if s.incidentStore == nil {
		http.Error(w, "Incident management not enabled", http.StatusServiceUnavailable)
		return
	}

	policyID := strings.TrimPrefix(r.URL.Path, "/api/escalation/")
	if policyID == "" {
		http.Error(w, "Policy ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		policy, err := s.incidentStore.GetPolicy(policyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if policy == nil {
			http.Error(w, "Policy not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)

	case http.MethodDelete:
		if err := s.incidentStore.DeletePolicy(policyID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// Federation / Cluster Handlers
// ============================================================================

// ClusterInfoResponse is the response for /api/cluster
type ClusterInfoResponse struct {
	Enabled     bool                       `json:"enabled"`
	NodeID      string                     `json:"node_id,omitempty"`
	NodeCount   int                        `json:"node_count"`
	GossipAddr  string                     `json:"gossip_addr,omitempty"`
	LocalNode   *federation.NodeStatus     `json:"local_node,omitempty"`
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	resp := ClusterInfoResponse{
		Enabled: cluster != nil,
	}

	if cluster != nil {
		resp.NodeCount = cluster.NumMembers()
		resp.GossipAddr = cluster.GetLocalAddr()
		resp.LocalNode = cluster.LocalNode()
		if resp.LocalNode != nil {
			resp.NodeID = resp.LocalNode.ID
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	if cluster == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	nodes := cluster.Members()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (s *Server) handleClusterJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	if cluster == nil {
		http.Error(w, "Cluster not enabled", http.StatusBadRequest)
		return
	}

	var req struct {
		Addresses []string `json:"addresses"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Addresses) == 0 {
		http.Error(w, "No addresses provided", http.StatusBadRequest)
		return
	}

	n, err := cluster.Join(req.Addresses)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to join: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"joined": n,
		"total":  cluster.NumMembers(),
	})
}

func (s *Server) handleClusterIncidents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	if cluster == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	incidents := cluster.GetState().GetAllIncidents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incidents)
}

func (s *Server) handleClusterMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	if cluster == nil {
		http.Error(w, "Cluster not enabled", http.StatusBadRequest)
		return
	}

	// Return aggregated cluster metrics
	metrics := cluster.GetState().GetClusterMetrics()
	nodeStates := cluster.GetState().GetAllNodeStates()

	resp := map[string]interface{}{
		"aggregate":   metrics,
		"node_states": nodeStates,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleClusterState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	cluster := s.cluster
	s.mu.RUnlock()

	if cluster == nil {
		http.Error(w, "Cluster not enabled", http.StatusBadRequest)
		return
	}

	state := cluster.GetState().GetFullState()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// ============================================================================
// Kubernetes Handlers
// ============================================================================

func (s *Server) handleK8sInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"message": "Kubernetes not connected. Ensure kubeconfig is available.",
		})
		return
	}

	info := k8s.GetClusterInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": true,
		"cluster": info,
	})
}

func (s *Server) handleK8sSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	summary := k8s.GetSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}

func (s *Server) handleK8sNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	nodes := k8s.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (s *Server) handleK8sNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespaces := k8s.GetNamespaces()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(namespaces)
}

func (s *Server) handleK8sPods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	pods := k8s.GetPods(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pods)
}

func (s *Server) handleK8sPod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	// Extract namespace/name from path: /api/k8s/pods/{namespace}/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/k8s/pods/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid path, expected /api/k8s/pods/{namespace}/{name}", http.StatusBadRequest)
		return
	}

	pod := k8s.GetPod(parts[0], parts[1])
	if pod == nil {
		http.Error(w, "Pod not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pod)
}

func (s *Server) handleK8sDeployments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	deployments := k8s.GetDeployments(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deployments)
}

func (s *Server) handleK8sServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	services := k8s.GetServices(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

func (s *Server) handleK8sDaemonSets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	daemonsets := k8s.GetDaemonSets(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(daemonsets)
}

func (s *Server) handleK8sStatefulSets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	statefulsets := k8s.GetStatefulSets(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statefulsets)
}

func (s *Server) handleK8sJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	jobs := k8s.GetJobs(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}

func (s *Server) handleK8sCronJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	cronjobs := k8s.GetCronJobs(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cronjobs)
}

func (s *Server) handleK8sIngresses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	ingresses := k8s.GetIngresses(namespace)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ingresses)
}

func (s *Server) handleK8sEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	namespace := r.URL.Query().Get("namespace")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	events := k8s.GetEvents(namespace, limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

func (s *Server) handleK8sWorkloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	k8s := s.k8sCollector
	s.mu.RUnlock()

	if k8s == nil {
		http.Error(w, "Kubernetes not connected", http.StatusServiceUnavailable)
		return
	}

	workloads := k8s.GetWorkloadHealth()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(workloads)
}

// ============ Anomaly Detection Handlers ============

func (s *Server) handleAnomalyStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	stats := as.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleAnomalyRecent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	anomalies := as.GetRecentAnomalies(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anomalies)
}

func (s *Server) handleAnomalyMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	metricName := r.URL.Query().Get("name")
	if metricName != "" {
		stats := as.GetMetricStats(metricName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
		return
	}

	// Return list of subscribed metrics
	metrics := as.GetSubscribedMetrics()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleAnomalyPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Metric string  `json:"metric"`
		Value  float64 `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result := as.Push(req.Metric, req.Value, time.Now())

	w.Header().Set("Content-Type", "application/json")
	if result != nil {
		json.NewEncoder(w).Encode(result)
	} else {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "data point recorded"})
	}
}

func (s *Server) handleAnomalyTrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Metric string `json:"metric"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := as.TrainModel(req.Metric); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "model trained"})
}

func (s *Server) handleAnomalySubscribe(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	as := s.anomalyService
	s.mu.RUnlock()

	if as == nil {
		http.Error(w, "Anomaly detection not enabled", http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Metric string `json:"metric"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		as.Subscribe(req.Metric)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "subscribed to " + req.Metric})
		return
	}

	if r.Method == http.MethodDelete {
		metric := r.URL.Query().Get("metric")
		if metric == "" {
			http.Error(w, "metric parameter required", http.StatusBadRequest)
			return
		}

		as.Unsubscribe(metric)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "unsubscribed from " + metric})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// WatchQL Query Handlers

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	executor := s.queryExecutor
	s.mu.RUnlock()

	if executor == nil {
		// Create a default executor if not set
		executor = query.NewExecutor()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := executor.Execute(ctx, req.Query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"columns": result.Columns,
		"rows":    result.Rows,
		"stats": map[string]interface{}{
			"rows_returned":   result.Stats.RowsReturned,
			"execution_time":  result.Stats.ExecutionTime.String(),
			"execution_ms":    result.Stats.ExecutionTime.Milliseconds(),
		},
	})
}

func (s *Server) handleQueryExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	// Parse the query
	ast, err := query.Parse(req.Query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	// Create a plan
	planner := query.NewPlanner()
	plan, err := planner.Plan(ast)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	// Generate explain output
	explain := query.ExplainPlan(plan)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"query":          req.Query,
		"parsed":         ast.String(),
		"plan":           explain,
		"estimated_cost": plan.Root.EstimatedCost(),
	})
}

func (s *Server) handleQueryValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	// Try to parse
	ast, err := query.Parse(req.Query)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":   false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":      true,
		"normalized": ast.String(),
	})
}

// SetQueryExecutor sets the query executor
func (s *Server) SetQueryExecutor(e *query.Executor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryExecutor = e
}

// GetQueryExecutor returns the query executor
func (s *Server) GetQueryExecutor() *query.Executor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queryExecutor
}

// On-Call Scheduling Handlers

// SetOncallStore sets the on-call store and creates calculator/engine
func (s *Server) SetOncallStore(store *oncall.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oncallStore = store
	s.oncallCalculator = oncall.NewCalculator(store)
	s.oncallEscalation = oncall.NewEscalationEngine(store, s.oncallCalculator)
	s.oncallEscalation.RegisterChannel(&oncall.LogChannel{})
	s.oncallEscalation.Start()
}

func (s *Server) handleOncallSchedules(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		schedules, err := store.ListSchedules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(schedules)

	case http.MethodPost:
		var sched oncall.Schedule
		if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if sched.ID == "" {
			sched.ID = uuid.New().String()
		}
		if err := store.CreateSchedule(&sched); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sched)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOncallSchedule(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/oncall/schedules/")
	if id == "" {
		http.Error(w, "Schedule ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sched, err := store.GetSchedule(id)
		if err != nil {
			http.Error(w, "Schedule not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sched)

	case http.MethodPut:
		var sched oncall.Schedule
		if err := json.NewDecoder(r.Body).Decode(&sched); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		sched.ID = id
		if err := store.UpdateSchedule(&sched); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sched)

	case http.MethodDelete:
		if err := store.DeleteSchedule(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOncallCurrent(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	calc := s.oncallCalculator
	s.mu.RUnlock()

	if calc == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	scheduleID := strings.TrimPrefix(r.URL.Path, "/api/oncall/current/")
	if scheduleID == "" {
		http.Error(w, "Schedule ID required", http.StatusBadRequest)
		return
	}

	entry, err := calc.GetCurrentOnCall(scheduleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if entry == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"on_call": nil})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{"on_call": entry})
	}
}

func (s *Server) handleOncallCalendar(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	calc := s.oncallCalculator
	s.mu.RUnlock()

	if calc == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	scheduleID := strings.TrimPrefix(r.URL.Path, "/api/oncall/calendar/")
	if scheduleID == "" {
		http.Error(w, "Schedule ID required", http.StatusBadRequest)
		return
	}

	// Parse time range from query params
	start := time.Now()
	end := start.AddDate(0, 0, 14) // Default to 2 weeks

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}

	entries, err := calc.GetCalendar(scheduleID, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"schedule_id": scheduleID,
		"start":       start,
		"end":         end,
		"entries":     entries,
	})
}

func (s *Server) handleOncallOverrides(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ScheduleID string          `json:"schedule_id"`
			Override   oncall.Override `json:"override"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Override.ID == "" {
			req.Override.ID = uuid.New().String()
		}
		if err := store.CreateOverride(req.ScheduleID, &req.Override); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(req.Override)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleOncallOverride(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/oncall/overrides/")
	if id == "" {
		http.Error(w, "Override ID required", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodDelete {
		if err := store.DeleteOverride(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) handleOncallPolicies(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		policies, err := store.ListPolicies()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policies)

	case http.MethodPost:
		var policy oncall.EscalationPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if policy.ID == "" {
			policy.ID = uuid.New().String()
		}
		if err := store.CreatePolicy(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(policy)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOncallPolicy(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	store := s.oncallStore
	s.mu.RUnlock()

	if store == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/oncall/policies/")
	if id == "" {
		http.Error(w, "Policy ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		policy, err := store.GetPolicy(id)
		if err != nil {
			http.Error(w, "Policy not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)

	case http.MethodPut:
		var policy oncall.EscalationPolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		policy.ID = id
		if err := store.UpdatePolicy(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(policy)

	case http.MethodDelete:
		if err := store.DeletePolicy(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOncallEscalate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	engine := s.oncallEscalation
	s.mu.RUnlock()

	if engine == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		IncidentID string `json:"incident_id"`
		PolicyID   string `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := engine.TriggerIncident(req.IncidentID, req.PolicyID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Escalation triggered",
	})
}

func (s *Server) handleOncallAcknowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	engine := s.oncallEscalation
	s.mu.RUnlock()

	if engine == nil {
		http.Error(w, "On-call not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		IncidentID string `json:"incident_id"`
		UserID     string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := engine.AcknowledgeIncident(req.IncidentID, req.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Incident acknowledged",
	})
}
