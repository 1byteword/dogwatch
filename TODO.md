# TODO - Prioritized Roadmap

Brief checklist with links to [VISION.md](VISION.md) for full context.

---

## Current State

| Category | Status |
|----------|--------|
| **Codebase** | ~45,500 lines Go, 29 packages |
| **eBPF Probes** | TCP ✅, HTTP/1.1 ✅, CPU profiling ✅, SSL ✅ |
| **Storage** | SQLite for all stores (metrics, traces, logs, alerts, etc.) |
| **Auth** | RBAC ✅, API keys ✅, OAuth2 ✅, SAML ✅ |
| **Alerting** | Rules ✅, evaluation ✅, routing ✅, 7 notification channels ✅ |
| **Features** | Dashboards ✅, SLOs ✅, Synthetics ✅, Incidents ✅, On-call ✅, Catalog ✅, Anomaly ✅, K8s ✅, Federation ✅ |
| **NOT Built** | All 7 killer features, DB protocol parsing, Cost Intelligence, BubbleUp, backup/restore |

---

## Phase 1: Core Differentiators (Weeks 1-6)

### P0 - Zero-Config Database Tracing

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Redis eBPF protocol parsing~~ | ✅ Done | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [Redis Wire Format](VISION.md#redis-protocol-parsing) |
| ~~PostgreSQL eBPF protocol parsing~~ | ✅ Done | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [PostgreSQL Wire Format](VISION.md#postgresql-protocol-parsing) |
| MySQL eBPF protocol parsing | Test needed | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [MySQL Wire Format](VISION.md#mysql-protocol-parsing) |
| ~~Fix SSL/HTTPS probe~~ | ✅ Done | [TLS Interception](VISION.md#2-tls-interception) |

### P1 - DB Probe Refinements

| Task | Complexity | Notes |
|------|------------|-------|
| Reduce MySQL false positives | 2-3 days | Binary protocol matches gRPC/protobuf; add stricter validation |
| ~~Test PostgreSQL with real server~~ | ✅ Done | Verified: CREATE, INSERT, SELECT with latency tracking |
| Test MySQL with real server | 1 day | Code exists, needs verification |

### P0 - Production Essentials

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| OTLP receiver (gRPC + HTTP) | 1 week | [Ingest Protocol Support](VISION.md#ingest-protocol-support) |
| Backup & restore CLI | 1 week | [Backup & Disaster Recovery](VISION.md#2-backup--disaster-recovery) |
| Health check endpoints (`/healthz`, `/readyz`) | 2 days | [Production Essentials](VISION.md#production-essentials) |
| Prometheus remote write receiver | 3 days | [Prometheus Compatibility](VISION.md#prometheus-compatibility) |

---

## Phase 2: Make People Pay (Weeks 7-12)

### P0 - Cost Intelligence

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Datadog cost calculator | 1 week | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| New Relic cost calculator | 3 days | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| Cost trending dashboard | 1 week | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| Cost recommendations engine | 1 week | [Cost Intelligence](VISION.md#2-cost-intelligence) |

### P0 - Control Plane

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Usage analytics (what's queried vs wasted) | 2 weeks | [Control Plane](VISION.md#3-control-plane) |
| Cardinality explorer | 1 week | [Control Plane](VISION.md#3-control-plane), [Pain Point: Cardinality](VISION.md#pain-point-3-cardinality-explosions--high) |
| Data shaping rules (drop/aggregate at ingest) | 2 weeks | [Control Plane](VISION.md#3-control-plane) |
| Team quotas & chargeback | 1 week | [Control Plane](VISION.md#3-control-plane) |

### P1 - Log Analysis

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| LogCompare (time period comparison) | 2 weeks | [LogCompare](VISION.md#1-logcompare--steal-this) |
| Pattern detection / LogReduce | 3 weeks | [LogReduce](VISION.md#3-logreduce--pattern-detection--steal-this) |

---

## Phase 3: Root Cause & Correlation (Weeks 13-18)

### P0 - Automatic Root Cause

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| BubbleUp (statistical anomaly explanation) | 3 weeks | [BubbleUp](VISION.md#4-bubbleup-automatic-root-cause-analysis) |
| Change correlation engine | 2 weeks | [Change Correlation](VISION.md#5-change-correlation) |
| Alert auto-enrichment (deploys, related alerts) | 1 week | [Pain Point: Slow Incident Response](VISION.md#pain-point-5-slow-incident-response--high) |

### P1 - Dependencies

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Dependency graph from traces | 2 weeks | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |
| Dependency-aware alerting | 1 week | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |
| Blast radius estimation | 1 week | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |

---

## Phase 4: User Experience (Weeks 19-24)

### P0 - Homepage & Navigation

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Lookout homepage (anomalies at a glance) | 2 weeks | [Lookout](VISION.md#2-lookout-automatic-anomaly-overview--steal-this) |
| Entity synthesis (auto-discover services) | 3 weeks | [Entity Synthesis](VISION.md#1-entity-synthesis--steal-this) |
| Entity relationship mapping | 2 weeks | [Entity Synthesis](VISION.md#1-entity-synthesis--steal-this) |

### P1 - Query & Developer Experience

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Visual query builder | 3 weeks | [Query Builder UX](VISION.md#2-query-builder-ux--steal-this) |
| "My Services" developer view | 1 week | [Pain Point: Developer Experience](VISION.md#pain-point-6-developer-experience-gap--medium) |
| "My On-Call" view | 3 days | [Pain Point: On-Call Burnout](VISION.md#pain-point-7-on-call-burnout--medium) |

---

## Phase 5: Query & Analysis (Weeks 25-30)

### P1 - Query Language

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| DQL pipe-based query language | 4 weeks | [SPL / Query Language](VISION.md#1-spl-search-processing-language--steal-this) |
| Cross-signal joins (logs + traces) | 2 weeks | [SPL / Query Language](VISION.md#1-spl-search-processing-language--steal-this) |
| Full-text search with BM25 ranking | 2 weeks | [Full-Text Search](VISION.md#1-full-text-search-with-relevance--steal-this) |

### P2 - Advanced Queries

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Recording rules (pre-computed aggregations) | 1 week | [Chronosphere Features](VISION.md#6-recording-rules-at-scale) |
| Knowledge objects (reusable query components) | 2 weeks | [Knowledge Objects](VISION.md#2-knowledge-objects--steal-this) |

---

## Phase 6: Security & Compliance (Weeks 31-36)

### P1 - Security

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Threat detection rules (shell-in-container, cryptominers) | 2 weeks | [Security Observability](VISION.md#6-security-observability) |
| Security dashboard | 1 week | [Security Observability](VISION.md#6-security-observability) |
| SIEM export (CEF/LEEF) | 1 week | [Security Observability](VISION.md#6-security-observability) |

### P1 - Compliance

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| PII detection & redaction | 2 weeks | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |
| Enhanced audit logging (query audit) | 1 week | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |
| Compliance reports (SOC2 evidence) | 2 weeks | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |

---

## Phase 7: Enterprise Scale (Weeks 37+)

### P2 - Data Architecture

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Write-ahead log (WAL) | 2 weeks | [Architectural Gaps](VISION.md#1-write-ahead-log-wal) |
| Hot/warm/cold storage tiering | 3 weeks | [Architectural Gaps](VISION.md#2-hotwarmcold-tiering) |
| Pluggable storage backends | 4 weeks | [Pluggable Storage](VISION.md#pluggable-storage-architecture) |

### P2 - Sampling

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Head sampling with priority rules | 1 week | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |
| Tail sampling (keep traces with errors) | 2 weeks | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |
| Adaptive sampling | 2 weeks | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |

### P2 - Migration

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| Datadog dashboard import | 2 weeks | [Migration Assistant](VISION.md#7-migration-assistant) |
| Grafana dashboard import | 1 week | [Migration Assistant](VISION.md#7-migration-assistant) |
| Alert rule import | 1 week | [Migration Assistant](VISION.md#7-migration-assistant) |

### P3 - Future

| Task | VISION.md Reference |
|------|---------------------|
| Histograms (first-class, accurate p99.9+) | [Histograms](VISION.md#1-histograms-as-first-class-citizens--steal-this) |
| Delta counters (serverless/ephemeral) | [Delta Counters](VISION.md#2-delta-counters--steal-this) |
| Data pipeline routing | [Cribl Features](VISION.md#1-data-routing--transformation--steal-this) |
| Derived metrics | [Derived Metrics](VISION.md#3-derived-metrics--steal-this) |
| Canvas dashboards (NOC displays) | [Canvas](VISION.md#2-canvas-presentation-dashboards--steal-this) |
| SOAR playbooks | [SOAR](VISION.md#4-soar-security-orchestration-automation-response--partial-steal) |
| LLM integration (natural language queries) | [AI/ML Features](VISION.md#1-aiml-powered-features-critical-gap) |
| IDE integration (VS Code) | [IDE Integration](VISION.md#7-ide-integration) |
| Terraform provider | [Observability as Code](VISION.md#trend-8-observability-as-code--terraform-integration--partial) |
| Wasm plugin system | [WebAssembly](VISION.md#trend-7-webassembly-for-extensibility--opportunity) |
| Business context (revenue impact) | [Pain Point: Business Context](VISION.md#pain-point-14-lack-of-business-context--high) |
| Multi-cloud visibility | [Pain Point: Multi-Cloud](VISION.md#pain-point-11-multi-cloud--hybrid-visibility--high) |

---

## Completed ✅

| Category | Items |
|----------|-------|
| **eBPF** | TCP connections, HTTP/1.1 parsing, CPU profiling/flamegraphs, SSL/HTTPS (tracefs uprobes), Redis protocol, PostgreSQL protocol |
| **Storage** | All SQLite stores (metrics, traces, logs, dashboards, alerts, SLOs, synthetics, incidents, on-call, RBAC, audit, SSO, deploys, notify, catalog) |
| **Alerting** | Rules, evaluation, routing, inhibition |
| **Notifications** | Slack, PagerDuty, OpsGenie, Discord, MS Teams, Email, Webhook |
| **Auth** | RBAC (Owner/Admin/Editor/Viewer), API keys, JWT sessions, OAuth2, SAML SSO |
| **Features** | Dashboards, SLOs, Synthetics, Incidents, On-call, Service catalog, Anomaly detection (IForest), Deploy tracking |
| **Infrastructure** | Federation (gossip), K8s collector, Container monitoring, Rate limiting, Pagination |
| **APM SDK** | HTTP middleware, SQL/Redis/gRPC instrumentation |
| **UI** | Web server, Service map, History graphs, Demo data |

---

## Not Building ❌

| Item | Reason | VISION.md Reference |
|------|--------|---------------------|
| RUM / Browser SDK | Different product | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| Mobile SDK | Different domain | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| Full SIEM | Too broad | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| Custom ML models | Use LLM APIs instead | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| 600+ integrations | Focus on auto-discovery | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| Session replay | Different product | [Explicitly Skipped](VISION.md#explicitly-skipped) |
| HA clustering | Premature | [Do NOT Build Yet](VISION.md#do-not-build-yet) |

---

## Quick Reference

| Question | Answer |
|----------|--------|
| **Build first** | MySQL/PG/Redis parsing → OTLP → Backup |
| **Makes people pay** | Cost Intelligence, Control Plane, LogCompare |
| **Makes people stay** | BubbleUp, Lookout, Entity synthesis |
| **The moat** | Zero-config eBPF + cost transparency |
| **What's broken** | Nothing critical currently |
| **Full context** | [VISION.md](VISION.md) (22,400 lines) |

---

## Priority Legend

| Priority | Meaning |
|----------|---------|
| **P0** | Must have - blocks adoption or differentiation |
| **P1** | Should have - significant value |
| **P2** | Nice to have - do after P0/P1 |
| **P3** | Future - revisit later |
