package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dogwatch/internal/aggregator"
	"dogwatch/internal/probe"
	"dogwatch/internal/web"
)

func main() {
	// Parse flags
	verbose := flag.Bool("v", false, "Verbose mode - show individual events")
	interval := flag.Int("i", 5, "Stats refresh interval in seconds")
	webPort := flag.Int("port", 9999, "Web UI port")
	noWeb := flag.Bool("no-web", false, "Disable web UI")
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

	// Start web UI
	var webServer *web.Server
	if !*noWeb {
		webServer = web.New(agg, *webPort)
		if profileProbe != nil {
			webServer.SetProfiler(profileProbe)
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
	_ = sslProbe // silence unused variable warning

	// Ticker for stats display
	statsTicker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer statsTicker.Stop()

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
