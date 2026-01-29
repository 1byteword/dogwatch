# Dogwatch UI Reference

Web interface available at `http://localhost:9999`

---

## Dashboard Widgets

### System Monitoring

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **CPU** | Real-time CPU usage percentage | `/api/system` |
| **Memory** | Real-time memory usage | `/api/system` |
| **Disk I/O** | Read/write throughput | `/api/system` |
| **Network** | RX/TX throughput | `/api/system` |
| **Total Connections** | Active connection count | `/api/stats` |
| **Total Requests** | HTTP request count | `/api/stats` |
| **Total Errors** | Error count | `/api/stats` |

### Service Discovery

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Service Map** | Visual topology of services | `/api/servicemap` |
| **HTTP Endpoints** | Endpoint stats (count, latency, errors) | `/api/endpoints` |
| **Connections** | Active connections by process | `/api/connections` |
| **Top Processes** | Processes by CPU usage | `/api/processes` |

### Distributed Tracing

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Traces** | List of traces with latency/status | `/api/traces` |
| **Trace Detail** | Span waterfall with timing | `/api/traces/{id}` |
| **Services** | Services discovered from traces | `/api/trace/services` |
| **Dependencies** | Service dependency graph | `/api/trace/dependencies` |

### Logs & Patterns

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Logs** | Real-time log stream | `/api/logs` |
| **Log Search** | Full-text log search | `/api/logs?q=...` |
| **Log Patterns** | Discovered patterns (LogReduce) | `/api/logs/patterns` |
| **Trending Patterns** | Patterns gaining frequency | `/api/logs/patterns/trending` |
| **Log Compare** | Compare logs across time periods | `/api/logs/compare` |

### Alerting

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Watches** | Configured watch rules | `/api/watches` |
| **Alerts** | Active firing alerts | `/api/alerting/alerts` |
| **Alert Groups** | Alerts grouped by labels | `/api/alerting/groups` |
| **Silences** | Active alert silences | `/api/alerting/silences` |

### Synthetics & SLOs

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Synthetic Checks** | HTTP/TCP check status | `/api/synthetics/checks` |
| **Check Results** | Historical check results | `/api/synthetics/results/{id}` |
| **Uptime** | Uptime percentage | `/api/synthetics/uptime/{id}` |
| **SLOs** | Service Level Objectives | `/api/slos` |

### Infrastructure

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Containers** | Running containers | `/api/containers` |
| **Kubernetes** | Cluster overview | `/api/k8s/summary` |
| **K8s Pods** | Pod list with status | `/api/k8s/pods` |
| **K8s Deployments** | Deployment status | `/api/k8s/deployments` |
| **K8s Events** | Cluster events | `/api/k8s/events` |

### Incidents & On-Call

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Incidents** | Active/recent incidents | `/api/incidents` |
| **On-Call** | Current on-call user | `/api/oncall/current` |
| **Schedule** | On-call schedules | `/api/oncall/schedules` |
| **Escalations** | Escalation policies | `/api/oncall/policies` |

### Deployments

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Deployments** | Recent deploys | `/api/deploys` |
| **Deploy Stats** | Deployment statistics | `/api/deploys/stats` |
| **Impact Analysis** | Deploy-to-incident correlation | `/api/correlate/deploy/{id}` |

### Performance

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Flame Graph** | CPU profiling visualization | `/api/flamegraph` |
| **Anomalies** | Detected anomalies | `/api/anomaly/recent` |
| **DB Queries** | Database query monitoring | `/api/dbwatch/queries` |
| **Slow Queries** | Slow query detection | `/api/dbwatch/slow` |

### Cost & Usage

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Cost Estimate** | Datadog-equivalent cost | `/api/cost/estimate` |
| **Recommendations** | Cost optimization tips | `/api/cost/recommendations` |
| **Quick Wins** | Easy savings opportunities | `/api/cost/quick-wins` |
| **Cardinality** | High-cardinality metrics | `/api/cardinality/high` |
| **Usage** | Resource usage report | `/api/usage/report` |

### Administration

| Widget | Description | Data Source |
|--------|-------------|-------------|
| **Users** | User management | `/api/users` |
| **Teams** | Team management | `/api/teams` |
| **API Keys** | API key management | `/api/apikeys` |
| **Audit Logs** | User action history | `/api/audit/logs` |
| **Backups** | Backup management | `/api/backup/list` |

---

## Pages

### Main Views

| Page | Path | Description |
|------|------|-------------|
| Dashboard | `/` | Customizable widget dashboard |
| Login | `/login` | Authentication page |
| Status Page | `/status/{pageId}` | Public status page |

### Management UIs (via modals)

- User Settings
- User/Team Management
- API Key Management
- Notification Channels
- Dashboard Manager
- Watch/Alert Rule Editor
- SLO Editor
- Incident Management

---

## API Data Ingestion

| Protocol | Port | Endpoints |
|----------|------|-----------|
| OTLP gRPC | 4317 | traces, metrics, logs |
| OTLP HTTP | 4318 | `/v1/traces`, `/v1/metrics`, `/v1/logs` |
| Prometheus | 9999 | `/api/v1/write` |
| StatsD | 8125 | UDP metrics |
| HTTP | 9999 | `/api/logs/ingest`, `/api/metrics/push` |

---

## Widget Categories Summary

| Category | Widget Count |
|----------|--------------|
| System Monitoring | 7 |
| Service Discovery | 4 |
| Distributed Tracing | 4 |
| Logs & Patterns | 5 |
| Alerting | 4 |
| Synthetics & SLOs | 4 |
| Infrastructure | 5 |
| Incidents & On-Call | 4 |
| Deployments | 3 |
| Performance | 4 |
| Cost & Usage | 5 |
| Administration | 5 |
| **Total** | **54** |

---

## Notification Channels

| Channel | Description |
|---------|-------------|
| Slack | Webhook integration |
| PagerDuty | Incident routing |
| OpsGenie | Alert management |
| Discord | Webhook messages |
| MS Teams | Webhook integration |
| Email | SMTP delivery |
| Webhook | Custom HTTP callbacks |

---

## Authentication Methods

| Method | Description |
|--------|-------------|
| Username/Password | Built-in authentication |
| OAuth2 | Google, GitHub, etc. |
| SAML SSO | Enterprise SSO |
| API Keys | Programmatic access (`dw_` prefix) |

---

## Frontend Stack

- **HTML/CSS/JS** - Single-page application
- **Chart.js** - Metrics visualization
- **Custom components** - Grid layout, modals, forms
- **WebSocket** - Real-time updates (anomaly detection)

---

## Screenshots

*TODO: Add screenshots of major widgets*

---

## Related Docs

- [VISION.md](VISION.md) - Product vision and roadmap
- [TODO.md](TODO.md) - Development priorities
- [CLAUDE.md](CLAUDE.md) - Development guide
