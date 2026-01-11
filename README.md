# dogwatch

**eBPF-powered observability platform for Linux.** A Datadog/Grafana/PagerDuty alternative in a single binary.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Linux](https://img.shields.io/badge/Linux-5.8+-FCC624?style=flat&logo=linux&logoColor=black)](https://kernel.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  🐕 dogwatch - full-stack observability: metrics, traces, logs, incidents   │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Features

### Infrastructure Monitoring
| Feature | Description |
|---------|-------------|
| **TCP Connection Tracking** | See every connection with process name, PID, destination |
| **HTTP Tracing** | Auto-discover endpoints, request counts, error rates, latency |
| **System Metrics** | CPU, memory, disk I/O, network throughput, load average |
| **Container Metrics** | Docker container CPU, memory, network, block I/O via Unix socket |
| **Process Profiling** | CPU flame graphs for performance analysis |

### Alerting & Incidents
| Feature | Description |
|---------|-------------|
| **Watch Alerts** | Threshold-based alerting on any metric (CPU > 80%, error rate > 5%) |
| **Incident Management** | PagerDuty-like incident lifecycle (triggered → acked → resolved) |
| **On-Call Schedules** | Weekly/daily rotations with override support |
| **Escalation Policies** | Multi-level escalation with configurable delays |
| **Slack & Webhooks** | Notify via Slack or custom webhooks |

### Reliability
| Feature | Description |
|---------|-------------|
| **SLOs** | Define Service Level Objectives with error budgets |
| **Error Budget Tracking** | Real-time budget consumption and burn rate |
| **Synthetic Checks** | HTTP endpoint monitoring with success rate tracking |
| **Auto-Incident Creation** | SLO breaches and watch alerts auto-create incidents |

### Observability
| Feature | Description |
|---------|-------------|
| **Distributed Tracing** | OpenTelemetry (OTLP) trace ingestion and visualization |
| **Log Management** | Full-text search with automatic pattern detection |
| **Custom Metrics** | OTLP and StatsD protocol support |
| **Deployment Markers** | Track deployments with before/after metric correlation |

### Dashboard
| Feature | Description |
|---------|-------------|
| **Real-time Web UI** | Draggable, resizable GridStack widgets |
| **Persistent Layouts** | Save and restore dashboard configurations |
| **Dark Theme** | Easy on the eyes, built for NOCs |

---

## Quick Start

### Install

```bash
# One-line install
curl -fsSL https://raw.githubusercontent.com/1byteword/dogwatch/main/install.sh | sudo bash
```

### Run

```bash
sudo dogwatch
# Open http://localhost:9999
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              dogwatch                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   eBPF      │  │   OTLP      │  │  Docker     │  │  HTTP       │        │
│  │   Probes    │  │  Receiver   │  │  Collector  │  │  Synthetics │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                │                │                │               │
│         ▼                ▼                ▼                ▼               │
│  ┌─────────────────────────────────────────────────────────────────┐       │
│  │                      Aggregator & Storage                        │       │
│  │   (SQLite: metrics, traces, logs, incidents, SLOs, watches)     │       │
│  └─────────────────────────────────────────────────────────────────┘       │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────┐       │
│  │                         Evaluation Engines                       │       │
│  │   Watch Engine │ SLO Calculator │ Pager │ Synthetic Runner      │       │
│  └─────────────────────────────────────────────────────────────────┘       │
│         │                                                                   │
│         ▼                                                                   │
│  ┌─────────────────────────────────────────────────────────────────┐       │
│  │                      Web UI & REST API                           │       │
│  │                      http://localhost:9999                       │       │
│  └─────────────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## API Reference

### Incidents

```bash
# List incidents
curl http://localhost:9999/api/incidents

# Create incident
curl -X POST http://localhost:9999/api/incidents \
  -H "Content-Type: application/json" \
  -d '{"title": "Database down", "severity": "critical", "service": "postgres"}'

# Acknowledge
curl -X POST http://localhost:9999/api/incidents/{id}/ack \
  -d '{"user": "alice"}'

# Resolve
curl -X POST http://localhost:9999/api/incidents/{id}/resolve \
  -d '{"user": "alice", "resolution": "Restarted service"}'

# Get stats (MTTA, MTTR)
curl http://localhost:9999/api/incidents/stats
```

### On-Call & Escalation

```bash
# Create on-call schedule
curl -X POST http://localhost:9999/api/oncall \
  -d '{"name": "Primary", "rotations": [{"type": "weekly", "users": ["alice", "bob"]}]}'

# Who's on-call now?
curl http://localhost:9999/api/oncall/current

# Create escalation policy
curl -X POST http://localhost:9999/api/escalation \
  -d '{"name": "Default", "rules": [{"level": 0, "delay_minutes": 5, "notify_channels": ["slack"]}]}'
```

### SLOs

```bash
# Create SLO
curl -X POST http://localhost:9999/api/slos \
  -d '{"name": "API Availability", "target": 99.9, "window": "30d", "source": {"type": "synthetics", "id": "check-id"}}'

# Get SLO status
curl http://localhost:9999/api/slos/{id}/status

# Get historical snapshots
curl http://localhost:9999/api/slos/{id}/history
```

### Watches (Alerts)

```bash
# Create watch
curl -X POST http://localhost:9999/api/watches \
  -d '{"name": "High CPU", "metric": "cpu", "operator": ">", "threshold": 80, "duration": "5m"}'

# Create notification channel
curl -X POST http://localhost:9999/api/watches/channels \
  -d '{"name": "Slack Ops", "type": "slack", "config": {"webhook_url": "https://hooks.slack.com/..."}}'
```

### Synthetic Checks

```bash
# Create HTTP check
curl -X POST http://localhost:9999/api/synthetics/checks \
  -d '{"name": "Homepage", "url": "https://example.com", "interval": "1m", "timeout": "10s"}'

# Get check results
curl http://localhost:9999/api/synthetics/checks/{id}/results
```

### Deployments

```bash
# Record deployment
curl -X POST http://localhost:9999/api/deploys \
  -d '{"service": "api", "version": "v1.2.3", "environment": "prod", "user": "deploy-bot"}'

# Get deployment impact (before/after metrics)
curl http://localhost:9999/api/deploys/{id}/impact?window=30m
```

### Logs

```bash
# Search logs (FTS5)
curl "http://localhost:9999/api/logs?q=error+timeout&limit=100"

# Get log patterns
curl http://localhost:9999/api/logs/patterns
```

### Traces (OTLP)

```bash
# Send traces via OTLP HTTP
curl -X POST http://localhost:9999/v1/traces \
  -H "Content-Type: application/x-protobuf" \
  --data-binary @trace.pb

# Query traces
curl http://localhost:9999/api/traces?service=api&limit=50
```

### Custom Metrics

```bash
# Push metrics (simple JSON)
curl -X POST http://localhost:9999/api/metrics/push \
  -d '[{"name": "orders.count", "value": 42, "tags": {"region": "us-east"}}]'

# Query metrics
curl "http://localhost:9999/api/metrics/query?name=orders.count&since=1h"

# OTLP metrics endpoint
POST http://localhost:9999/v1/metrics
```

### Containers

```bash
# List containers with metrics
curl http://localhost:9999/api/containers

# Get container summary
curl http://localhost:9999/api/containers/summary
```

---

## Configuration

### CLI Flags

```bash
sudo dogwatch [flags]

Flags:
  -port int       Web UI port (default 9999)
  -data string    Data directory for SQLite databases (default "./data")
  -i int          Stats refresh interval in seconds (default 5)
  -v              Verbose mode - show individual events
  -no-web         Disable web UI, CLI only
```

### Environment Variables

```bash
SLACK_WEBHOOK_URL=https://hooks.slack.com/...   # Default Slack webhook for alerts
```

---

## Requirements

| Requirement | Version |
|-------------|---------|
| Linux Kernel | 5.8+ (BPF ring buffers) |
| Architecture | x86_64 (ARM64 coming soon) |
| Privileges | Root (for eBPF) |
| Docker | Optional (for container metrics) |

Check your kernel:
```bash
uname -r  # Should be >= 5.8
```

---

## Building from Source

```bash
git clone https://github.com/1byteword/dogwatch.git
cd dogwatch
go build ./cmd/dogwatch
sudo ./dogwatch
```

Regenerate BPF bindings (requires clang, libbpf):
```bash
go generate ./internal/probe/...
go build ./cmd/dogwatch
```

---

## How It Works

### eBPF Probes

dogwatch uses eBPF to hook into kernel functions with minimal overhead:

| Probe | Purpose |
|-------|---------|
| `kprobe/tcp_connect` | Outbound TCP connections |
| `kretprobe/inet_csk_accept` | Inbound connections |
| `tracepoint/syscalls/sys_enter_write` | HTTP request data |
| `tracepoint/syscalls/sys_exit_read` | HTTP response data |

### Data Flow

1. **eBPF probes** capture events in-kernel
2. **Ring buffers** efficiently transfer to userspace
3. **Aggregator** computes metrics (counts, latencies, percentiles)
4. **SQLite** persists historical data (24h retention)
5. **Evaluation engines** check watches, SLOs, escalations
6. **Web UI** displays real-time dashboards

---

## Comparison

| Feature | dogwatch | Datadog | Grafana + Prometheus |
|---------|----------|---------|----------------------|
| Single binary | Yes | No | No |
| Zero config | Yes | No | No |
| eBPF-native | Yes | Agent | No |
| Incidents/Paging | Yes | Yes | Requires OnCall |
| SLOs | Yes | Yes | Plugin |
| Cost | Free | $$$$ | Free* |

---

## Roadmap

- [ ] ARM64 support
- [ ] HTTPS/TLS interception (via eBPF SSL probes)
- [ ] Kubernetes integration
- [ ] PagerDuty/Opsgenie integration
- [ ] Prometheus remote write
- [ ] Multi-node federation

---

## License

MIT

---

## Contributing

PRs welcome! See [issues](https://github.com/1byteword/dogwatch/issues) for ideas.

```
        ╱|、
       (˚ˎ 。7
        |、˜〵
       じしˍ,)ノ   woof! happy monitoring!
```
