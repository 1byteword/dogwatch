package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"dogwatch/internal/aggregator"
	"dogwatch/internal/alerting"
	"dogwatch/internal/anomaly"
	"dogwatch/internal/audit"
	"dogwatch/internal/backup"
	"dogwatch/internal/cardinality"
	"dogwatch/internal/containers"
	"dogwatch/internal/costintel"
	"dogwatch/internal/datashaping"
	"dogwatch/internal/dbwatch"
	"dogwatch/internal/sso"
	"dogwatch/internal/custommetrics"
	"dogwatch/internal/deploys"
	"dogwatch/internal/dashboard"
	"dogwatch/internal/federation"
	"dogwatch/internal/incidents"
	"dogwatch/internal/kubernetes"
	"dogwatch/internal/logreduce"
	"dogwatch/internal/logs"
	"dogwatch/internal/notify"
	"dogwatch/internal/oncall"
	"dogwatch/internal/otlp"
	"dogwatch/internal/probe"
	"dogwatch/internal/quotas"
	"dogwatch/internal/rbac"
	"dogwatch/internal/slo"
	"dogwatch/internal/storage"
	"dogwatch/internal/synthetics"
	"dogwatch/internal/trace"
	"dogwatch/internal/usage"
	"dogwatch/internal/watch"
	"dogwatch/internal/web"
)

func main() {
	// Parse flags
	verbose := flag.Bool("v", false, "Verbose mode - show individual events")
	interval := flag.Int("i", 5, "Stats refresh interval in seconds")
	webPort := flag.Int("port", 9999, "Web UI port")
	noWeb := flag.Bool("no-web", false, "Disable web UI")
	dataDir := flag.String("data", "/var/lib/dogwatch", "Data directory for metrics storage")

	// Federation flags
	clusterEnabled := flag.Bool("cluster", false, "Enable multi-node federation")
	clusterName := flag.String("cluster-name", "", "Node name in cluster (default: hostname)")
	clusterBind := flag.String("cluster-bind", "0.0.0.0", "Bind address for gossip protocol")
	clusterPort := flag.Int("cluster-port", 7946, "Port for gossip protocol")
	clusterSeeds := flag.String("cluster-seeds", "", "Comma-separated list of seed node addresses (host:port)")
	clusterAdvertise := flag.String("cluster-advertise", "", "Address to advertise to other nodes")
	clusterKey := flag.String("cluster-key", "", "Encryption key for gossip (16/24/32 bytes for AES)")

	// OTLP flags
	otlpEnabled := flag.Bool("otlp", true, "Enable OTLP receivers")
	otlpGRPCPort := flag.Int("otlp-grpc-port", 4317, "OTLP gRPC port")
	otlpHTTPPort := flag.Int("otlp-http-port", 4318, "OTLP HTTP port")

	flag.Parse()

	if os.Geteuid() != 0 {
		log.Fatal("dogwatch must be run as root")
	}

	fmt.Println("🐕 dogwatch - eBPF observability")
	fmt.Println("================================")

	// Start TCP probe
	fmt.Println("Starting TCP connection probe...")
	tcpProbe, err := probe.New()
	if err != nil {
		log.Fatalf("Failed to start TCP probe: %v", err)
	}
	defer tcpProbe.Close()

	// Start HTTP probe (plain HTTP)
	fmt.Println("Starting HTTP probe...")
	httpProbe, err := probe.NewHTTPProbe()
	if err != nil {
		log.Printf("Warning: HTTP probe failed to start: %v", err)
		httpProbe = nil
	} else {
		defer httpProbe.Close()
	}

	// SSL probe (HTTPS) - DISABLED FOR MVP
	// The SSL uprobes attach successfully but never fire. See internal/probe/ssl.go
	// and bpf/ssl.c for detailed debugging notes. bpftrace CAN trace the same
	// functions, so this appears to be a cilium/ebpf uprobe compatibility issue.
	var sslProbe *probe.SSLProbe = nil
	fmt.Println("HTTPS/SSL probe: disabled for MVP (uprobe compatibility issue)")

	// Start CPU profiler for flame graph
	fmt.Println("Starting CPU profiler...")
	profileProbe, err := probe.NewProfileProbe()
	if err != nil {
		log.Printf("Warning: CPU profiler failed to start: %v", err)
		profileProbe = nil
	} else {
		defer profileProbe.Close()
		fmt.Println("  CPU profiler running at 49Hz sampling")
	}

	// Start database protocol probe (MySQL, PostgreSQL, Redis)
	fmt.Println("Starting database protocol probe...")
	dbProbe, err := probe.NewDBProbe()
	if err != nil {
		log.Printf("Warning: Database probe failed to start: %v", err)
		dbProbe = nil
	} else {
		defer dbProbe.Close()
		fmt.Println("  Database probe monitoring MySQL, PostgreSQL, Redis")
	}

	// Create aggregator
	agg := aggregator.New()

	// Create storage for historical metrics
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}

	// Configure backup with data directory
	web.SetBackupDataDir(*dataDir)
	fmt.Printf("Backup/restore: http://localhost:%d/api/backup\n", *webPort)

	// Initialize backup scheduler (disabled by default)
	backupScheduler := backup.NewScheduler(backup.SchedulerConfig{
		Enabled:   false, // Enable via API or env var
		DataDir:   *dataDir,
		OutputDir: *dataDir,
		Interval:  24 * time.Hour,
		Retention: backup.DefaultRetentionPolicy(),
		Compress:  true,
	})
	web.SetBackupScheduler(backupScheduler)
	// Check if auto-backup is enabled via env
	if os.Getenv("DOGWATCH_AUTO_BACKUP") == "true" {
		backupScheduler.UpdateConfig(backup.SchedulerConfig{
			Enabled:   true,
			DataDir:   *dataDir,
			OutputDir: *dataDir,
			Interval:  24 * time.Hour,
			Retention: backup.DefaultRetentionPolicy(),
			Compress:  true,
		})
		backupScheduler.Start()
		defer backupScheduler.Stop()
		fmt.Printf("Auto-backup: enabled (daily)\n")
	}
	fmt.Printf("Backup scheduler: http://localhost:%d/api/backup/scheduler\n", *webPort)

	dbPath := filepath.Join(*dataDir, "metrics.db")
	store, err := storage.New(dbPath)
	if err != nil {
		log.Printf("Warning: Could not create metrics storage: %v", err)
		store = nil
	} else {
		defer store.Close()
		fmt.Printf("Metrics storage: %s\n", dbPath)
	}

	// Create trace storage
	traceDbPath := filepath.Join(*dataDir, "traces.db")
	traceStore, err := trace.NewStore(traceDbPath)
	if err != nil {
		log.Printf("Warning: Could not create trace storage: %v", err)
		traceStore = nil
	} else {
		defer traceStore.Close()
		fmt.Printf("Trace storage: %s\n", traceDbPath)
		fmt.Println("OTLP trace receiver: http://localhost:9999/v1/traces")
	}

	// Create watch storage
	watchDbPath := filepath.Join(*dataDir, "watches.db")
	watchStore, err := watch.NewStore(watchDbPath)
	if err != nil {
		log.Printf("Warning: Could not create watch storage: %v", err)
		watchStore = nil
	} else {
		defer watchStore.Close()
		fmt.Printf("Watch storage: %s\n", watchDbPath)
	}

	// Create dashboard storage
	dashboardDbPath := filepath.Join(*dataDir, "dashboards.db")
	dashboardStore, err := dashboard.NewStore(dashboardDbPath)
	if err != nil {
		log.Printf("Warning: Could not create dashboard storage: %v", err)
		dashboardStore = nil
	} else {
		defer dashboardStore.Close()
		fmt.Printf("Dashboard storage: %s\n", dashboardDbPath)
	}

	// Create log storage
	logDbPath := filepath.Join(*dataDir, "logs.db")
	logStore, err := logs.NewStore(logDbPath)
	if err != nil {
		log.Printf("Warning: Could not create log storage: %v", err)
		logStore = nil
	} else {
		defer logStore.Close()
		fmt.Printf("Log storage: %s\n", logDbPath)
		fmt.Println("Log ingestion: http://localhost:9999/api/logs/ingest")

		// Set up log comparison
		web.SetLogCompareStore(logStore)
		fmt.Printf("Log comparison: http://localhost:%d/api/logs/compare\n", *webPort)
	}

	// Create log pattern mining storage (LogReduce)
	logReduceDbPath := filepath.Join(*dataDir, "logreduce.db")
	logReduceStore, err := logreduce.NewStore(logReduceDbPath, logreduce.DefaultMinerConfig())
	if err != nil {
		log.Printf("Warning: Could not create log pattern storage: %v", err)
	} else {
		defer logReduceStore.Close()
		web.SetLogReduceStore(logReduceStore)
		fmt.Printf("Log pattern mining: %s\n", logReduceDbPath)
		fmt.Printf("LogReduce: http://localhost:%d/api/logs/patterns\n", *webPort)
	}

	// Create custom metrics storage
	customMetricsDbPath := filepath.Join(*dataDir, "custom_metrics.db")
	customMetricsStore, err := custommetrics.NewStore(customMetricsDbPath)
	if err != nil {
		log.Printf("Warning: Could not create custom metrics storage: %v", err)
		customMetricsStore = nil
	} else {
		defer customMetricsStore.Close()
		fmt.Printf("Custom metrics storage: %s\n", customMetricsDbPath)
		fmt.Printf("Prometheus remote write: http://localhost:%d/api/v1/write\n", *webPort)
	}

	// Create cardinality explorer
	cardinalityExplorer := cardinality.NewExplorer()
	if customMetricsStore != nil {
		customMetricsStore.SetCardinalityHook(cardinalityExplorer)
	}
	web.SetCardinalityExplorer(cardinalityExplorer)
	fmt.Printf("Cardinality explorer: http://localhost:%d/api/cardinality/report\n", *webPort)

	// Create usage tracker for analytics
	usageTracker := usage.NewTracker(10000)
	web.SetUsageTracker(usageTracker)
	fmt.Printf("Usage analytics: http://localhost:%d/api/usage/report\n", *webPort)

	// Create data shaping store
	shapingDbPath := filepath.Join(*dataDir, "shaping.db")
	shapingStore, err := datashaping.NewStore(shapingDbPath)
	if err != nil {
		log.Printf("Warning: Could not create data shaping storage: %v", err)
		shapingStore = nil
	} else {
		defer shapingStore.Close()
		web.SetDataShapingStore(shapingStore)

		// Connect data shaping to custom metrics store
		if customMetricsStore != nil {
			customMetricsStore.SetDataShapingHook(&metricsShapingAdapter{store: shapingStore})
		}

		fmt.Printf("Data shaping: http://localhost:%d/api/shaping/rules\n", *webPort)
	}

	// Create quota and chargeback store
	quotaDbPath := filepath.Join(*dataDir, "quotas.db")
	quotaStore, err := quotas.NewStore(quotaDbPath)
	if err != nil {
		log.Printf("Warning: Could not create quota storage: %v", err)
		quotaStore = nil
	} else {
		defer quotaStore.Close()

		// Create quota tracker for real-time enforcement
		quotaTracker := quotas.NewTracker(quotaStore)
		quotaTracker.Start()
		defer quotaTracker.Stop()

		web.SetQuotaStore(quotaStore, quotaTracker)
		fmt.Printf("Quotas storage: %s\n", quotaDbPath)
		fmt.Printf("Team quotas: http://localhost:%d/api/quotas/teams\n", *webPort)
		fmt.Printf("Chargeback: http://localhost:%d/api/chargeback/summary\n", *webPort)
	}

	// Create database watch storage
	dbwatchDbPath := filepath.Join(*dataDir, "dbwatch.db")
	dbwatchStore, err := dbwatch.NewStore(dbwatchDbPath)
	if err != nil {
		log.Printf("Warning: Could not create database watch storage: %v", err)
		dbwatchStore = nil
	} else {
		defer dbwatchStore.Close()
		web.SetDBWatchStore(dbwatchStore)
		fmt.Printf("Database watch storage: %s\n", dbwatchDbPath)
		fmt.Println("Database monitoring: http://localhost:9999/api/dbwatch/queries")
	}

	// Configure cost intelligence
	web.SetCostIntelStores(traceStore, logStore, customMetricsStore)

	// Create cost intelligence store for trending
	costDbPath := filepath.Join(*dataDir, "costintel.db")
	costStore, err := costintel.NewStore(costDbPath)
	if err != nil {
		log.Printf("Warning: Could not create cost intelligence storage: %v", err)
		costStore = nil
	} else {
		defer costStore.Close()
		fmt.Printf("Cost Intelligence storage: %s\n", costDbPath)

		// Create usage provider adapter
		usageProvider := &costUsageProvider{
			traceStore:    traceStore,
			logStore:      logStore,
			customMetrics: customMetricsStore,
		}

		// Create collector for periodic cost tracking (hourly)
		costCollector := costintel.NewCollector(costStore, usageProvider, time.Hour)
		costCollector.Start()
		defer costCollector.Stop()

		web.SetCostIntelStore(costStore, costCollector)
		fmt.Printf("Cost trending: http://localhost:%d/api/cost/trend\n", *webPort)
	}

	fmt.Printf("Cost Intelligence: http://localhost:%d/api/cost/estimate\n", *webPort)

	// Set up cost recommendations engine
	recommendationEngine := costintel.NewRecommendationEngine()
	recommendationProvider := costintel.NewDataProvider(
		traceStore,
		logStore,
		customMetricsStore,
		cardinalityExplorer,
		usageTracker,
	)
	web.SetRecommendationEngine(recommendationEngine, recommendationProvider)
	fmt.Printf("Cost recommendations: http://localhost:%d/api/cost/recommendations\n", *webPort)

	// Start OTLP receivers
	var otlpServer *otlp.Server
	if *otlpEnabled && (traceStore != nil || customMetricsStore != nil || logStore != nil) {
		otlpConfig := otlp.Config{
			GRPCPort: *otlpGRPCPort,
			HTTPPort: *otlpHTTPPort,
		}
		otlpServer = otlp.NewServer(otlpConfig, traceStore, customMetricsStore, logStore)
		if err := otlpServer.Start(); err != nil {
			log.Printf("Warning: Could not start OTLP server: %v", err)
			otlpServer = nil
		} else {
			defer otlpServer.Stop()
		}
	}

	// Start StatsD receiver
	var statsdReceiver *custommetrics.StatsDReceiver
	if customMetricsStore != nil {
		statsdReceiver = custommetrics.NewStatsDReceiver(customMetricsStore, 8125)
		if err := statsdReceiver.Start(); err != nil {
			log.Printf("Warning: Could not start StatsD receiver: %v", err)
			statsdReceiver = nil
		} else {
			defer statsdReceiver.Stop()
		}
	}

	// Create synthetics storage
	syntheticsDbPath := filepath.Join(*dataDir, "synthetics.db")
	syntheticsStore, err := synthetics.NewStore(syntheticsDbPath)
	if err != nil {
		log.Printf("Warning: Could not create synthetics storage: %v", err)
		syntheticsStore = nil
	} else {
		defer syntheticsStore.Close()
		fmt.Printf("Synthetics storage: %s\n", syntheticsDbPath)
	}

	// Create SLO storage
	sloDbPath := filepath.Join(*dataDir, "slos.db")
	sloStore, err := slo.NewStore(sloDbPath)
	if err != nil {
		log.Printf("Warning: Could not create SLO storage: %v", err)
		sloStore = nil
	} else {
		defer sloStore.Close()
		fmt.Printf("SLO storage: %s\n", sloDbPath)
	}

	// Create deployment storage
	deployDbPath := filepath.Join(*dataDir, "deploys.db")
	deployStore, err := deploys.NewStore(deployDbPath)
	if err != nil {
		log.Printf("Warning: Could not create deployment storage: %v", err)
		deployStore = nil
	} else {
		defer deployStore.Close()
		fmt.Printf("Deployment storage: %s\n", deployDbPath)
	}

	// Create incident storage (PagerDuty-like)
	incidentDbPath := filepath.Join(*dataDir, "incidents.db")
	incidentStore, err := incidents.NewStore(incidentDbPath)
	if err != nil {
		log.Printf("Warning: Could not create incident storage: %v", err)
		incidentStore = nil
	} else {
		defer incidentStore.Close()
		fmt.Printf("Incident storage: %s\n", incidentDbPath)
	}

	// Create on-call storage
	oncallDbPath := filepath.Join(*dataDir, "oncall.db")
	oncallStore, err := oncall.NewStore(oncallDbPath)
	if err != nil {
		log.Printf("Warning: Could not create on-call storage: %v", err)
		oncallStore = nil
	} else {
		defer oncallStore.Close()
		fmt.Printf("On-call storage: %s\n", oncallDbPath)
	}

	// Create RBAC storage and auth
	rbacDbPath := filepath.Join(*dataDir, "rbac.db")
	rbacStore, err := rbac.NewStore(rbacDbPath)
	if err != nil {
		log.Printf("Warning: Could not create RBAC storage: %v", err)
	} else {
		defer rbacStore.Close()
		fmt.Printf("RBAC storage: %s\n", rbacDbPath)

		// Ensure default organization exists
		defaultOrg, _ := rbacStore.EnsureDefaultOrg("default", "Default Organization")
		if defaultOrg != nil {
			fmt.Printf("Default organization: %s (%s)\n", defaultOrg.Name, defaultOrg.ID)
		}

		// Create auth and middleware
		rbacAuth := rbac.NewAuth(rbacStore)
		rbacMiddleware := rbac.NewMiddleware(rbacAuth)

		// Ensure default admin user exists
		if defaultOrg != nil {
			adminEmail := os.Getenv("DOGWATCH_ADMIN_EMAIL")
			adminPassword := os.Getenv("DOGWATCH_ADMIN_PASSWORD")

			// Check if any users exist
			users, _ := rbacStore.ListUsers(defaultOrg.ID)
			if len(users) == 0 {
				// First boot - need to create admin
				if adminEmail == "" {
					adminEmail = "admin@localhost"
				}

				if adminPassword == "" {
					// Generate a secure random password
					adminPassword = generateSecurePassword()
					fmt.Println()
					fmt.Println("╔════════════════════════════════════════════════════════════╗")
					fmt.Println("║  FIRST-TIME SETUP - Admin credentials generated            ║")
					fmt.Println("╠════════════════════════════════════════════════════════════╣")
					fmt.Printf("║  Email:    %-48s║\n", adminEmail)
					fmt.Printf("║  Password: %-48s║\n", adminPassword)
					fmt.Println("╠════════════════════════════════════════════════════════════╣")
					fmt.Println("║  ⚠️  SAVE THIS PASSWORD - it will not be shown again!      ║")
					fmt.Println("║  Set DOGWATCH_ADMIN_EMAIL and DOGWATCH_ADMIN_PASSWORD      ║")
					fmt.Println("║  environment variables to use your own credentials.        ║")
					fmt.Println("╚════════════════════════════════════════════════════════════╝")
					fmt.Println()
				}

				admin, err := rbacAuth.CreateUser(defaultOrg.ID, &rbac.UserCreate{
					Email:    adminEmail,
					Password: adminPassword,
					Name:     "Admin",
					Role:     rbac.RoleOwner,
				})
				if err != nil {
					log.Printf("Warning: Could not create admin user: %v", err)
				} else {
					fmt.Printf("Admin user: %s (role: %s)\n", admin.Email, admin.Role)
				}
			} else {
				fmt.Printf("Admin user: %s (role: %s)\n", users[0].Email, users[0].Role)
			}
		}

		// Set RBAC for web handlers
		web.SetRBACAuth(rbacAuth, rbacMiddleware, rbacStore)
		fmt.Printf("Authentication: http://localhost:%d/api/auth/login\n", *webPort)
	}

	// Create notification service
	notifyDbPath := filepath.Join(*dataDir, "notify.db")
	notifyStore, err := notify.NewStore(notifyDbPath)
	var notifyService *notify.Service
	if err != nil {
		log.Printf("Warning: Could not create notification storage: %v", err)
	} else {
		defer notifyStore.Close()
		baseURL := fmt.Sprintf("http://localhost:%d", *webPort)
		notifyService = notify.NewService(notifyStore, baseURL)
		web.SetNotifyService(notifyService)
		fmt.Printf("Notification storage: %s\n", notifyDbPath)
		fmt.Printf("Notification channels: http://localhost:%d/api/notify/channels\n", *webPort)
	}

	// Create audit logging store
	auditDbPath := filepath.Join(*dataDir, "audit.db")
	auditStore, err := audit.NewStore(auditDbPath)
	if err != nil {
		log.Printf("Warning: Could not create audit storage: %v", err)
	} else {
		defer auditStore.Close()
		web.SetAuditStore(auditStore)
		fmt.Printf("Audit storage: %s\n", auditDbPath)
		fmt.Printf("Audit logs: http://localhost:%d/api/audit/logs\n", *webPort)
	}

	// Create SSO store and initialize SSO
	ssoDbPath := filepath.Join(*dataDir, "sso.db")
	ssoStore, err := sso.NewStore(ssoDbPath)
	if err != nil {
		log.Printf("Warning: Could not create SSO storage: %v", err)
	} else {
		defer ssoStore.Close()
		baseURL := fmt.Sprintf("http://localhost:%d", *webPort)
		web.InitSSO(ssoStore, baseURL)
		fmt.Printf("SSO storage: %s\n", ssoDbPath)
		fmt.Printf("OAuth2 providers: http://localhost:%d/api/auth/providers\n", *webPort)
		fmt.Printf("SAML SSO: http://localhost:%d/api/auth/saml/login?org=ORG_ID\n", *webPort)
	}

	// Initialize federation cluster if enabled
	var cluster *federation.Cluster
	if *clusterEnabled {
		clusterCfg := federation.DefaultConfig()
		clusterCfg.HTTPPort = *webPort

		if *clusterName != "" {
			clusterCfg.NodeName = *clusterName
		}
		clusterCfg.BindAddr = *clusterBind
		clusterCfg.BindPort = *clusterPort

		if *clusterAdvertise != "" {
			parts := strings.Split(*clusterAdvertise, ":")
			clusterCfg.AdvertiseAddr = parts[0]
			if len(parts) > 1 {
				fmt.Sscanf(parts[1], "%d", &clusterCfg.AdvertisePort)
			}
		}

		if *clusterSeeds != "" {
			clusterCfg.Seeds = strings.Split(*clusterSeeds, ",")
		}

		if *clusterKey != "" {
			clusterCfg.EncryptionKey = []byte(*clusterKey)
		}

		var clusterErr error
		cluster, clusterErr = federation.NewCluster(clusterCfg)
		if clusterErr != nil {
			log.Printf("Warning: Could not create cluster: %v", clusterErr)
			cluster = nil
		} else {
			fmt.Printf("Federation cluster: %s (gossip port %d)\n", clusterCfg.NodeName, clusterCfg.BindPort)
		}
	}

	// Start web UI
	var webServer *web.Server
	if !*noWeb {
		webServer = web.New(agg, *webPort)
		if profileProbe != nil {
			webServer.SetProfiler(profileProbe)
		}
		if store != nil {
			webServer.SetStore(store)
		}
		if traceStore != nil {
			webServer.SetTraceStore(traceStore)
		}
		if watchStore != nil {
			webServer.SetWatchStore(watchStore)
		}
		if dashboardStore != nil {
			webServer.SetDashboardStore(dashboardStore)
		}
		if logStore != nil {
			webServer.SetLogStore(logStore)
		}
		if customMetricsStore != nil {
			webServer.SetCustomMetricsStore(customMetricsStore)
		}
		if syntheticsStore != nil {
			// Create notifier for synthetics alerts (reuses watch infrastructure)
			notifier := watch.NewNotifier()
			webServer.SetSyntheticsStore(syntheticsStore, notifier)
			fmt.Println("Synthetics checks: http://localhost:9999/api/synthetics/checks")
		}
		if sloStore != nil {
			webServer.SetSLOStore(sloStore)
			fmt.Println("SLOs: http://localhost:9999/api/slos")
		}

		// Start container monitoring
		containerCollector := containers.NewCollector()
		if err := containerCollector.Start(); err != nil {
			log.Printf("Container monitoring disabled: %v", err)
		} else {
			webServer.SetContainerCollector(containerCollector)
			fmt.Println("Containers: http://localhost:9999/api/containers")
		}

		// Set deployment store
		if deployStore != nil {
			webServer.SetDeployStore(deployStore)
			fmt.Println("Deployments: http://localhost:9999/api/deploys")
		}

		// Set incident store (PagerDuty-like paging)
		if incidentStore != nil {
			webServer.SetIncidentStore(incidentStore)
			fmt.Println("Incidents: http://localhost:9999/api/incidents")
		}

		// Set on-call store (schedules, rotations, escalations)
		if oncallStore != nil {
			webServer.SetOncallStore(oncallStore)
			fmt.Println("On-call schedules: http://localhost:9999/api/oncall/schedules")
			fmt.Println("Escalation policies: http://localhost:9999/api/oncall/policies")
		}

		// Set up alerting system
		alertingDbPath := filepath.Join(*dataDir, "alerting.db")
		metricsProvider := alerting.NewSimpleMetricsProvider()
		alertManager, err := alerting.NewAlertManager(alertingDbPath, metricsProvider)
		if err != nil {
			log.Printf("Warning: Could not create alert manager: %v", err)
		} else {
			// Register default receivers
			alertManager.Router.RegisterReceiver(alerting.NewLogReceiver("default"))

			// Register notification service receiver if available
			if notifyService != nil {
				alertManager.Router.RegisterReceiver(alerting.NewNotifyServiceReceiver(
					"notify",
					func(group *alerting.AlertGroup) error {
						// Send to all enabled channels
						for _, alert := range group.Alerts {
							// Map alert severity
							var severity notify.Severity
							switch alert.Severity {
							case alerting.SeverityCritical:
								severity = notify.SeverityCritical
							case alerting.SeverityWarning:
								severity = notify.SeverityWarning
							default:
								severity = notify.SeverityInfo
							}

							// Get description from annotations if available
							message := alert.RuleName + " is firing"
							if desc, ok := alert.Annotations["description"]; ok {
								message = desc
							} else if summary, ok := alert.Annotations["summary"]; ok {
								message = summary
							}

							// Handle nil FiredAt
							timestamp := time.Now()
							if alert.FiredAt != nil {
								timestamp = *alert.FiredAt
							}

							notification := &notify.Notification{
								Type:      notify.NotificationAlert,
								Title:     alert.RuleName,
								Message:   message,
								Severity:  severity,
								Source:    "alerting",
								Value:     alert.Value,
								Threshold: alert.Threshold,
								Labels:    alert.Labels,
								Timestamp: timestamp,
							}

							// Send to all channels (type-agnostic broadcast)
							channels, _ := notifyService.GetStore().ListChannels("")
							for _, ch := range channels {
								if ch.Enabled {
									notifyService.Send(ch.ID, notification)
								}
							}
						}
						return nil
					},
				))
				log.Println("[alerting] Notification service receiver registered")
			}

			// Connect to on-call escalation if available
			if oncallStore != nil {
				oncallCalc := oncall.NewCalculator(oncallStore)
				escalationEngine := oncall.NewEscalationEngine(oncallStore, oncallCalc)
				escalationEngine.RegisterChannel(&oncall.LogChannel{})
				escalationEngine.Start()

				// Create on-call receiver that triggers escalation
				alertManager.Router.RegisterReceiver(alerting.NewOnCallReceiver(
					"oncall",
					"", // Default escalation ID
					func(incidentID, policyID string) error {
						return escalationEngine.TriggerIncident(incidentID, policyID)
					},
				))
			}

			alertManager.Start()
			webServer.SetAlertManager(alertManager)
			fmt.Printf("Alerting: http://localhost:%d/api/alerting\n", *webPort)
		}

		// Set up federation cluster
		if cluster != nil {
			webServer.SetCluster(cluster)
			if err := cluster.Start(); err != nil {
				log.Printf("Warning: Could not start cluster: %v", err)
			} else {
				fmt.Printf("Cluster: http://localhost:%d/api/cluster\n", *webPort)
				fmt.Printf("Cluster nodes: http://localhost:%d/api/cluster/nodes\n", *webPort)
			}
		}

		// Set up Kubernetes collector (optional - only if kubeconfig available)
		k8sCollector, err := kubernetes.NewCollector()
		if err != nil {
			log.Printf("Kubernetes monitoring disabled: %v", err)
		} else {
			webServer.SetK8sCollector(k8sCollector)
			if err := k8sCollector.Start(); err != nil {
				log.Printf("Warning: Could not start Kubernetes collector: %v", err)
			} else {
				fmt.Printf("Kubernetes: http://localhost:%d/api/k8s\n", *webPort)
			}
		}

		// Set up anomaly detection service
		anomalyService, err := anomaly.NewService(anomaly.ServiceConfig{
			DataDir:           filepath.Join(*dataDir, "anomaly"),
			AnomalyThreshold:  0.7,
			CriticalThreshold: 0.9,
			Algorithm:         "ensemble", // uses IForest + statistical
		})
		if err != nil {
			log.Printf("Warning: Could not start anomaly detection: %v", err)
		} else {
			if err := anomalyService.Start(); err != nil {
				log.Printf("Warning: Anomaly service start error: %v", err)
			} else {
				webServer.SetAnomalyService(anomalyService)
				fmt.Printf("Anomaly detection: http://localhost:%d/api/anomaly\n", *webPort)
			}
		}

		go func() {
			fmt.Printf("Web UI available at http://localhost:%d\n", *webPort)
			if err := webServer.Start(); err != nil && err != http.ErrServerClosed {
				log.Printf("Web server error: %v", err)
			}
		}()
	}

	fmt.Println()
	fmt.Println("Probes loaded. Collecting data...")
	fmt.Printf("Stats will refresh every %d seconds. Press Ctrl+C to exit.\n", *interval)
	if *verbose {
		fmt.Println("Verbose mode: showing individual events")
	}
	fmt.Println()

	// Handle Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// Start reading TCP events
	go func() {
		if err := tcpProbe.Run(); err != nil {
			log.Printf("TCP probe error: %v", err)
		}
	}()

	// Start reading HTTP events
	if httpProbe != nil {
		go func() {
			if err := httpProbe.Run(); err != nil {
				log.Printf("HTTP probe error: %v", err)
			}
		}()
	}

	// Start reading database protocol events
	if dbProbe != nil {
		go func() {
			if err := dbProbe.Run(); err != nil {
				log.Printf("Database probe error: %v", err)
			}
		}()
	}

	// SSL probe is disabled for MVP - see comments above
	_ = sslProbe         // silence unused variable warning
	_ = statsdReceiver   // StatsD receiver runs in background
	_ = otlpServer       // OTLP server runs in background
	_ = dbwatchStore     // dbwatch store used in event loop

	// Ticker for stats display
	statsTicker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer statsTicker.Stop()

	// Start metrics recording to storage (every 10 seconds)
	if store != nil && webServer != nil {
		go func() {
			recordTicker := time.NewTicker(10 * time.Second)
			defer recordTicker.Stop()
			for range recordTicker.C {
				// Record system metrics
				sysMetrics := webServer.GetMetricsCollector().Collect()
				store.RecordSystemMetrics(
					sysMetrics.CPUUsagePercent,
					sysMetrics.MemUsagePercent,
					sysMetrics.DiskReadPerSec,
					sysMetrics.DiskWritePerSec,
					sysMetrics.NetRxPerSec,
					sysMetrics.NetTxPerSec,
					sysMetrics.Load1,
				)

				// Record connection metrics
				stats := agg.GetStats()
				store.RecordConnectionMetrics(
					stats.TotalConnections,
					stats.TotalRequests,
					stats.TotalErrors,
				)

				// Push to anomaly detection
				if as := webServer.GetAnomalyService(); as != nil {
					now := time.Now()
					as.Push("cpu_percent", sysMetrics.CPUUsagePercent, now)
					as.Push("memory_percent", sysMetrics.MemUsagePercent, now)
					as.Push("disk_io_read", float64(sysMetrics.DiskReadPerSec), now)
					as.Push("disk_io_write", float64(sysMetrics.DiskWritePerSec), now)
					as.Push("network_rx_bytes", float64(sysMetrics.NetRxPerSec), now)
					as.Push("network_tx_bytes", float64(sysMetrics.NetTxPerSec), now)
					as.Push("tcp_connection_count", float64(stats.TotalConnections), now)
				}
			}
		}()
		fmt.Println("Historical metrics recording: enabled (10s intervals)")
	}

	// Event loop
	for {
		select {
		case event, ok := <-tcpProbe.Events():
			if !ok {
				continue
			}
			agg.RecordConnection(event.Comm, event.PID, event.DAddr.String(), event.DPort)

			if *verbose {
				src := fmt.Sprintf("%s:%d", event.SAddr, event.SPort)
				dst := fmt.Sprintf("%s:%d", event.DAddr, event.DPort)
				fmt.Printf("[TCP] %-8d %-16s %-21s → %-21s\n",
					event.PID, truncate(event.Comm, 16), src, dst)
			}

		case event, ok := <-getHTTPEvents(httpProbe):
			if !ok {
				continue
			}
			handleHTTPEvent(event, agg, *verbose, "HTTP")

		case event, ok := <-getSSLEvents(sslProbe):
			if !ok {
				continue
			}
			handleHTTPEvent(event, agg, *verbose, "HTTPS")

		case event, ok := <-getDBEvents(dbProbe):
			if !ok {
				continue
			}
			handleDBEvent(event, dbwatchStore, *verbose)

		case <-statsTicker.C:
			fmt.Print("\033[2J\033[H")
			fmt.Println("🐕 dogwatch - eBPF observability")
			fmt.Println("================================")
			fmt.Printf("Updated: %s\n", time.Now().Format("15:04:05"))
			fmt.Print(agg.Summary())

		case <-sig:
			fmt.Println("\n\nFinal Statistics:")
			fmt.Print(agg.Summary())
			fmt.Println("Shutting down...")
			if webServer != nil {
				webServer.StopAlertManager()
				webServer.StopK8sCollector()
				webServer.StopCluster()
				webServer.StopContainerCollector()
				webServer.StopSLOCalculator()
				webServer.StopPager()
				webServer.StopSyntheticsRunner()
				webServer.StopWatchEngine()
				webServer.Stop()
			}
			return
		}
	}
}

func getHTTPEvents(p *probe.HTTPProbe) <-chan probe.HTTPEvent {
	if p == nil {
		return nil
	}
	return p.Events()
}

func getSSLEvents(p *probe.SSLProbe) <-chan probe.HTTPEvent {
	if p == nil {
		return nil
	}
	return p.Events()
}

func getDBEvents(p *probe.DBProbe) <-chan probe.DBEvent {
	if p == nil {
		return nil
	}
	return p.Events()
}

func handleDBEvent(event probe.DBEvent, store *dbwatch.Store, verbose bool) {
	if store == nil {
		return
	}

	// Only record queries with latency (response events with timing info)
	if event.EventType == "response" && event.Latency > 0 {
		record := &dbwatch.QueryRecord{
			Timestamp:    event.Timestamp,
			DBType:       dbwatch.DBType(event.DBType),
			PID:          event.PID,
			Comm:         event.Comm,
			Operation:    event.Operation,
			Query:        event.Query,
			Table:        event.Table,
			Key:          event.Key,
			LatencyMs:    float64(event.Latency.Microseconds()) / 1000.0,
			RowsAffected: event.RowsAffected,
			Error:        event.Error,
		}
		store.Record(record)
	}

	if verbose {
		dbColor := dbTypeColor(event.DBType)
		if event.EventType == "query" {
			query := event.Query
			if len(query) > 60 {
				query = query[:60] + "..."
			}
			fmt.Printf("[%s%s\033[0m] PID:%-6d %s %s\n", dbColor, strings.ToUpper(event.DBType), event.PID, event.Operation, query)
		} else if event.Latency > 0 {
			fmt.Printf("[%s%s\033[0m] PID:%-6d %s %.2fms\n", dbColor, strings.ToUpper(event.DBType), event.PID, event.Operation, float64(event.Latency.Microseconds())/1000.0)
		}
	}
}

func dbTypeColor(dbType string) string {
	switch dbType {
	case "mysql":
		return "\033[34m" // blue
	case "postgres":
		return "\033[36m" // cyan
	case "redis":
		return "\033[31m" // red
	default:
		return ""
	}
}

func handleHTTPEvent(event probe.HTTPEvent, agg *aggregator.Aggregator, verbose bool, proto string) {
	if event.EventType == "request" && event.Method != "" {
		agg.RecordRequest(event.PID, event.TID, event.Method, event.Path, event.Timestamp)
		if verbose {
			fmt.Printf("[%s REQ] PID:%-6d %s %s\n", proto, event.PID, event.Method, event.Path)
		}
	} else if event.EventType == "response" && event.StatusCode > 0 {
		agg.RecordResponse(event.PID, event.TID, event.StatusCode, event.Timestamp)
		if verbose {
			color := statusColor(event.StatusCode)
			fmt.Printf("[%s RES] PID:%-6d %s%d\033[0m\n", proto, event.PID, color, event.StatusCode)
		}
	}
}

func statusColor(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "\033[32m"
	case code >= 300 && code < 400:
		return "\033[33m"
	case code >= 400:
		return "\033[31m"
	default:
		return ""
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// generateSecurePassword generates a cryptographically secure random password
func generateSecurePassword() string {
	const (
		length  = 24
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	)

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// Fallback to less secure but still random
		for i := range b {
			b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		}
	} else {
		for i := range b {
			b[i] = charset[int(b[i])%len(charset)]
		}
	}
	return string(b)
}

// costUsageProvider adapts stores to the costintel.UsageProvider interface
type costUsageProvider struct {
	traceStore    *trace.Store
	logStore      *logs.Store
	customMetrics *custommetrics.Store
}

func (p *costUsageProvider) CollectUsage() costintel.UsageMetrics {
	usage := costintel.UsageMetrics{
		CollectedAt: time.Now(),
		PeriodDays:  30,
	}

	// Collect trace metrics (APM data)
	if p.traceStore != nil {
		services, err := p.traceStore.GetServices()
		if err == nil {
			usage.APMHostCount = len(services)
			usage.HostCount = len(services)

			for _, svc := range services {
				traces, err := p.traceStore.ListTraces(1000, svc, 24*time.Hour)
				if err == nil {
					usage.SpansPerMonth += int64(len(traces) * 5 * 30)
				}
			}
		}
	}

	// Collect log metrics
	if p.logStore != nil {
		end := time.Now()
		start := end.Add(-24 * time.Hour)

		query := logs.SearchQuery{
			StartTime: start,
			EndTime:   end,
			Limit:     1,
		}

		results, err := p.logStore.Search(query)
		if err == nil && results != nil {
			dailyLogs := int64(results.TotalCount)
			usage.LogEventsPerMonth = dailyLogs * 30
			usage.LogsGBPerMonth = float64(dailyLogs*30) * 0.001 / 1024
		}
	}

	// Collect custom metrics count
	if p.customMetrics != nil {
		metricInfos, err := p.customMetrics.List()
		if err == nil {
			usage.CustomMetricsCount = len(metricInfos)
			usage.MetricDataPoints = int64(len(metricInfos)) * 60 * 24 * 30
		}
	}

	return usage
}

// metricsShapingAdapter adapts datashaping.Store to the custommetrics.DataShapingHook interface
type metricsShapingAdapter struct {
	store *datashaping.Store
}

func (a *metricsShapingAdapter) EvaluateMetric(name string, tags map[string]string, sizeBytes int) (bool, map[string]string) {
	if a.store == nil {
		return true, tags
	}

	decision := a.store.EvaluateMetric(name, tags, sizeBytes)

	switch decision.Action {
	case datashaping.ActionDrop:
		return false, nil
	case datashaping.ActionTransform:
		// Apply tag transformations
		newTags := make(map[string]string)
		for k, v := range tags {
			drop := false
			for _, dt := range decision.DropTags {
				if k == dt {
					drop = true
					break
				}
			}
			if !drop {
				newTags[k] = v
			}
		}
		for k, v := range decision.AddTags {
			newTags[k] = v
		}
		return true, newTags
	default:
		return true, tags
	}
}
