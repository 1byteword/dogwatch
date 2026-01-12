package main

import (
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
	"dogwatch/internal/anomaly"
	"dogwatch/internal/containers"
	"dogwatch/internal/custommetrics"
	"dogwatch/internal/deploys"
	"dogwatch/internal/dashboard"
	"dogwatch/internal/federation"
	"dogwatch/internal/incidents"
	"dogwatch/internal/kubernetes"
	"dogwatch/internal/logs"
	"dogwatch/internal/oncall"
	"dogwatch/internal/probe"
	"dogwatch/internal/slo"
	"dogwatch/internal/storage"
	"dogwatch/internal/synthetics"
	"dogwatch/internal/trace"
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

	// Create aggregator
	agg := aggregator.New()

	// Create storage for historical metrics
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Printf("Warning: Could not create data directory: %v", err)
	}
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

	// SSL probe is disabled for MVP - see comments above
	_ = sslProbe         // silence unused variable warning
	_ = statsdReceiver   // StatsD receiver runs in background

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
