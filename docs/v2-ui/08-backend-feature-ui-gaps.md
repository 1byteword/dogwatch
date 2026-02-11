# V2 UI — Feature Gaps

Two sections:
1. **UI gaps** — backend is built, UI is missing
2. **Backend gaps** — not built yet at all

---

# Part A: Backend Built, Needs UI

Backend has 470+ API endpoints; the V2 UI currently covers ~50.
This is the backlog of implemented backend features that need UI.

---

## Tier 1 — Table-stakes (users expect these day one)

### ~~Alert Rule Management (Monitors)~~ DONE
- **Backend:** Full CRUD `/api/alerting/rules` — 5 rule types, label routing, ForDuration, dependency-aware blast radius
- **UI:** `MonitorsPage` — list, create (10 templates + 3-step wizard), edit, delete, enable/disable, search. Route: `/app/detect/monitors`

### ~~SLO Management~~ DONE
- **Backend:** Full CRUD `/api/slos`
- **UI:** `SloManagementPage` — list, create, delete. Budget remaining + burn rate cards. Route: `/app/configure/slos`

### ~~Synthetics Management~~ DONE
- **Backend:** Full CRUD `/api/synthetics/checks`, results, uptime stats
- **UI:** `SyntheticsManagementPage` — list, create (HTTP/browser/gRPC/DNS/TCP), delete. Uptime/latency cards, failure log. Route: `/app/configure/synthetics`

### ~~Query Explorer~~ DONE
- **Backend:** `/api/query/execute`, saved queries CRUD
- **UI:** `QueryExplorerPage` — PromQL/DQL editor, Ctrl+Enter run, time range, results table, saved queries. Route: `/app/explore/query`

### ~~Recording Rules~~ DONE
- **Backend:** CRUD `/api/recording-rules`, evaluation status
- **UI:** `RecordingRulesPage` — list, create, edit, delete, enable/disable, evaluation history, status bar. Route: `/app/configure/recording-rules`

---

## Tier 2 — High-value differentiators

### Security & Threat Detection
- **Backend:** `/api/security/*` — alerts, events, detection rules CRUD, MITRE ATT&CK mapping
- **Needed:** Security page — alert feed, detection rule builder, MITRE matrix view, event timeline
- **Endpoints:** `GET /api/security/alerts`, `GET /api/security/events`, `GET/POST/PUT/DELETE /api/security/rules/*`, `GET /api/security/stats`, `GET /api/security/mitre`

### Compliance
- **Backend:** `/api/compliance/*` — reports, controls, findings, scheduled scans
- **Needed:** Compliance page — report generator, control status matrix, findings list, gap analysis, schedule config
- **Endpoints:** `GET/POST /api/compliance/reports`, `GET /api/compliance/controls`, `GET /api/compliance/findings`, `GET/POST /api/compliance/schedules`, `GET /api/compliance/summary`

### Status Pages
- **Backend:** `/api/statuspages/*`, `/api/statuspage/components/*`, `/api/statuspage/incidents/*`, public page at `/status/:id`
- **Needed:** Status page builder — create/edit pages, add components, post incidents/updates. Preview public page.
- **Endpoints:** Full CRUD on statuspages, components, and incidents

### Knowledge Base
- **Backend:** `/api/knowledge/*` — entries, search, validation, graph expansion
- **Needed:** Knowledge page — create/edit runbooks and knowledge entries, search, link to services/alerts
- **Endpoints:** `GET/POST/PUT/DELETE /api/knowledge/*`, `GET /api/knowledge/search`

### BubbleUp Analysis
- **Backend:** `/api/bubbleup/analyze` — automated root cause analysis
- **Needed:** Widget or page — trigger analysis on a metric/timerange, show contributing dimensions. Like Honeycomb BubbleUp.
- **Endpoints:** `POST /api/bubbleup/analyze`, `GET /api/bubbleup/results`, `GET /api/bubbleup/:id`

### Lookout (Anomaly Overview)
- **Backend:** `/api/lookout/*` — anomaly overview
- **Needed:** Lookout page or widget — heatmap/grid of all metrics with anomaly scores. Like New Relic Lookout.
- **Endpoints:** `GET /api/lookout`, `GET /api/lookout/overview`

### Entity Explorer
- **Backend:** `/api/entities/*` — auto-discovered entities with graph
- **Needed:** Entity page — searchable list, entity detail with related signals, graph visualization
- **Endpoints:** `GET /api/entities`, `GET /api/entities/:id`, `GET /api/entities/stats`, `GET /api/entities/graph`

---

## Tier 3 — Power-user & admin features

### User & Team Management
- **Backend:** `/api/users/*`, `/api/teams/*`, `/api/auth/invites/*`, SSO/OAuth config
- **Needed:** Admin settings page — user list, invite, role assignment, team CRUD, SSO config
- **Endpoints:** CRUD on users, teams, invites, `/api/auth/providers`, `/api/auth/sso/config`

### PII Detection
- **Backend:** `/api/pii/*` — scan, config, allow/deny lists, custom patterns
- **Needed:** PII config page — toggle scanning, manage patterns, view detections
- **Endpoints:** `GET/PUT /api/pii/config`, `POST /api/pii/scan`, allow/deny list CRUD

### SIEM Export
- **Backend:** `/api/siem/*` — config CRUD, test, manual export, history
- **Needed:** SIEM integration page — add Splunk/Sentinel/etc, test connection, view export history
- **Endpoints:** CRUD on `/api/siem/configs/*`, `GET /api/siem/stats`

### Sampling Config
- **Backend:** `/api/sampling/*` — adaptive, tail, intelligent sampling with ML
- **Needed:** Sampling page — config editor, rule CRUD, stats dashboard, budget/cost impact view
- **Endpoints:** `GET/PUT /api/sampling/config`, CRUD on rules, adaptive/tail/intelligent stats

### Data Shaping
- **Backend:** `/api/shaping/*` — transform/filter rules, presets
- **Needed:** Data shaping page — rule builder, test against sample data, presets
- **Endpoints:** CRUD on `/api/shaping/rules/*`, `POST /api/shaping/test`, `GET /api/shaping/presets`

### Quotas & Chargeback
- **Backend:** `/api/quotas/*`, `/api/chargeback/*`
- **Needed:** Quotas page — team quota CRUD, usage view, violations. Chargeback report export.
- **Endpoints:** CRUD on quotas, `GET /api/chargeback/summary`, `GET /api/chargeback/report`

### Log Field Extraction
- **Backend:** `/api/logs/extraction/*` — Grok patterns, auto-learn
- **Needed:** Field extraction page — test Grok patterns, manage extraction rules, view extracted fields
- **Endpoints:** CRUD on patterns/sources, `POST /api/logs/extraction/grok/test`

### Migration Wizard
- **Backend:** `/api/migration/*` — import dashboards/alerts from Datadog, Grafana, Prometheus
- **Needed:** Migration wizard — upload JSON, preview conversion, import with fidelity report
- **Endpoints:** `POST /api/migration/preview`, `POST /api/migration/datadog/*`, `POST /api/migration/grafana/*`

### Scripts Engine
- **Backend:** `/api/scripts/*` — script library, execute, categories
- **Needed:** Scripts page — browse library by category, run with output, view script source
- **Endpoints:** `GET /api/scripts`, `POST /api/scripts`, `GET /api/scripts/categories`

### Storage Management
- **Backend:** `/api/storage/*` — WAL, tiering (hot/warm/cold), compaction
- **Needed:** Storage admin page — tier stats, trigger compaction, view WAL health
- **Endpoints:** `GET /api/storage/tiering/stats`, `GET /api/storage/wal/stats`, etc.

### eBPF Probe Management
- **Backend:** `/api/probes/*` — list, health, hot reload, rollback
- **Needed:** Probes admin page — probe list with health status, reload/rollback controls
- **Endpoints:** `GET /api/probes`, `GET /api/probes/health`, `POST /api/probes/reload`

### Cardinality Deep Dive
- **Backend:** Full cardinality dashboard, quarantine, circuit breaker (beyond current widget)
- **Needed:** Expand cardinality into a page — metric drill-down, quarantine management, circuit breaker controls
- **Endpoints:** `GET /api/cardinality/dashboard`, `GET /api/cardinality/quarantine`, `GET /api/cardinality/circuit-breaker`

### CPU Profile Explorer
- **Backend:** `/api/profiles/*` — profile list, profile-trace linking, function-to-trace
- **Needed:** Profiles page — flamegraph viewer, profile list, click-to-trace linking
- **Endpoints:** `GET /api/profiles`, `GET /api/profiles/:id`, `GET /api/profiles/:id/traces`

### Containers (non-K8s)
- **Backend:** `/api/containers/*` — list, details, summary
- **Needed:** Containers page or widget — container list with resource usage
- **Endpoints:** `GET /api/containers`, `GET /api/containers/summary`

### Dashboard Folders
- **Backend:** `/api/folders/*` — CRUD, tree structure
- **Needed:** Folder support in dashboard page — organize dashboards into folders
- **Endpoints:** CRUD on `/api/folders/*`, `GET /api/folders/tree`

---

# Part B: Not Built Yet (Backend Gaps)

55 packages in `internal/`, but these features are still missing entirely.

---

## High impact — blocks production credibility

### Error Tracking
- **What:** Sentry-style automatic error grouping, stack trace fingerprinting, frequency tracking, link errors to deploys that introduced them, assignment/triage workflow
- **Why:** Every competitor has this (Datadog Error Tracking, Sentry, New Relic Errors Inbox). Without it, users still need a separate tool for errors.
- **Scope:** New `internal/errortracking/` package. Ingest from OTLP span errors + log errors, fingerprint/group, store, expose CRUD API. UI: error list, group detail, stack trace view, frequency chart, deploy correlation.

### Additional eBPF Protocol Parsers
- **What:** The eBPF layer currently parses HTTP/1.1, MySQL, PostgreSQL, Redis. Missing:
  - **HTTP/2** — frame parsing, multiplexing, HPACK decompression
  - **gRPC** — built on HTTP/2, needs proto reflection for method names
  - **HTTPS/TLS** — SSL_read/SSL_write uprobes for OpenSSL/BoringSSL/GnuTLS
  - **DNS** — often-hidden latency source, trivial to parse
  - **Kafka** — message queue tracing
  - **MongoDB** — wire protocol (OP_MSG)
  - **Memcached** — text + binary protocol
  - **RabbitMQ/AMQP** — message queue tracing
- **Why:** gRPC is standard in modern microservices. Without HTTP/2+TLS parsing, dogwatch is blind to most production traffic. DNS is low-effort, high-value.
- **Priority:** TLS + HTTP/2 + gRPC first (unlocks real-world tracing), then DNS, then the rest.

### Additional Ingest Protocol Receivers
- **What:** OTLP (gRPC + HTTP) is done. Multi-protocol HTTP bridges exist for Graphite/InfluxDB/StatsD/Datadog. Still missing native receivers for:
  - **Fluent Forward** (port 24224) — FluentBit is THE K8s log collector. Without this, K8s log ingestion requires extra hops.
  - **Jaeger** (Thrift compact/binary + gRPC) — many existing apps already instrumented with Jaeger SDKs
  - **Zipkin** (JSON v2) — same reason, existing instrumentation
  - **Syslog** (RFC5424 + RFC3164, UDP/TCP) — network devices, firewalls, routers. Enterprise must-have.
  - **Loki push API** — Grafana ecosystem log ingestion
  - **StatsD native UDP** (port 8125) — HTTP bridge exists but real StatsD is UDP. Dead simple for devs.
- **Why:** Reduces migration friction. "Just point your existing FluentBit/Jaeger at dogwatch" is a powerful adoption story.

---

## Medium impact — competitive gaps

### LogQL / TraceQL Query Languages
- **What:** PromQL is done and Grafana-compatible. No equivalent for:
  - **LogQL** — Loki-style log query language (`{service="checkout"} |= "error" | json | duration > 500ms`)
  - **TraceQL** — Tempo-style trace query language (`{span.http.status_code >= 500 && duration > 1s}`)
- **Why:** Industry-standard query languages. Users migrating from Grafana/Loki/Tempo expect these. DQL exists in `internal/query/` but isn't compatible with either.
- **Scope:** New parsers + evaluators, wire to existing log/trace storage.

### Cloud Cost Integration
- **What:** Cost intelligence estimates self-hosted costs and compares to Datadog pricing. Missing: actual cloud spend import from AWS Cost Explorer, GCP Billing, Azure Cost Management APIs.
- **Why:** "You're spending $18K/month on Datadog AND $4K/month on the EC2 instances running dogwatch" is a more complete story than just the Datadog comparison.
- **Scope:** New `internal/cloudcost/` package with provider adapters, periodic sync, attribution to services.

### Real User Monitoring (RUM)
- **What:** No frontend performance monitoring. Missing: JS SDK for Core Web Vitals (LCP, FID, CLS), page load timing, JS error capture, session replay, user journey tracking.
- **Why:** Datadog RUM, New Relic Browser, Sentry all have this. Completes the "full stack" story from browser to backend to database.
- **Scope:** JS SDK (`dogwatch-rum.js`), new `internal/rum/` package for ingest + storage, session viewer UI.

### Exemplars (Prometheus-style)
- **What:** Click a metric data point → see the exact trace(s) that contributed to it. Prometheus exemplars standard stores trace_id with metric samples.
- **Why:** Bridges the metrics→traces gap. "P99 spiked at 14:32" → click → see the slow trace. Partial support in `internal/correlation/` but not the standard exemplars API.
- **Scope:** Store exemplars alongside metric samples, expose via `/api/v1/query` response, UI: clickable metric points.

### ~~Dashboard Variables~~ DONE
- **What:** Grafana-style template variables — `$service`, `$environment` dropdowns that filter all widgets.
- **UI:** Variables bar below toolbar, variable editor in edit mode, `$variable` binding in widget scope selectors. Types: service (auto-populated), custom, severity, timerange. DashboardVariable type in dashboards/types.ts.

---

## Lower impact — nice-to-have / enterprise

### Data at Rest Encryption
- **What:** SQLite files are stored unencrypted. Need AES-256 encryption for data at rest, encrypted backups, key rotation.
- **Why:** Required for SOC 2, HIPAA, many enterprise security policies. Currently a compliance blocker.
- **Scope:** SQLCipher or application-level encryption, key management, encrypted backup option.

### OpenAPI / Swagger Spec
- **What:** 470+ API endpoints with no generated documentation. No OpenAPI spec, no interactive explorer.
- **Why:** Developer experience. Third-party integrations. SDK generation. Currently users have to read Go source to understand the API.
- **Scope:** Generate OpenAPI 3.0 spec from handler registrations, serve Swagger UI at `/api/docs`.

### Helm Chart / K8s Operator
- **What:** No K8s-native deployment artifacts. Users must write their own manifests.
- **Why:** K8s is the primary deployment target. `helm install dogwatch` is expected. Operator enables auto-scaling, lifecycle management.
- **Scope:** `deploy/helm/` chart, optional `internal/operator/` for CRD-based management.

### Terraform Provider
- **What:** No infrastructure-as-code support. Can't manage alert rules, dashboards, SLOs, or teams via Terraform.
- **Why:** Platform engineering teams expect GitOps workflows. "All our monitoring config is in Terraform" is standard.
- **Scope:** Separate repo `terraform-provider-dogwatch`, covers rules, dashboards, SLOs, teams, notification channels.

### GitOps Config Sync
- **What:** No way to sync dashboards/alerts/SLOs from a Git repo. No config-as-code export/import beyond migration tools.
- **Why:** Teams want to version-control their observability config alongside application code.
- **Scope:** YAML/JSON config format, `dogwatch sync` CLI command, webhook for CI/CD integration.

### Full-Text Log Search (BM25)
- **What:** Current log search is pattern/filter based. No relevance-ranked full-text search.
- **Why:** Splunk's #1 feature. "Search for anything, get ranked results" is intuitive for operators.
- **Scope:** BM25 index on log messages, ranked results API, search-as-you-type UI.
