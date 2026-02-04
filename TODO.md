# TODO - Prioritized Roadmap

Brief checklist with links to [VISION.md](VISION.md) for full context.

---

## Current State

| Category | Status |
|----------|--------|
| **Codebase** | ~45,500 lines Go, 29 packages |
| **eBPF Probes** | TCP ✅, HTTP/1.1 ✅, CPU profiling ✅, SSL ✅, Redis ✅, PostgreSQL ✅, MySQL ✅ |
| **Storage** | SQLite for all stores (metrics, traces, logs, alerts, etc.) |
| **Auth** | RBAC ✅, API keys ✅, OAuth2 ✅, SAML ✅ |
| **Alerting** | Rules ✅, evaluation ✅, routing ✅, 7 notification channels ✅ |
| **Features** | Dashboards ✅, SLOs ✅, Synthetics ✅, Incidents ✅, On-call ✅, Catalog ✅, Anomaly ✅, K8s ✅, Federation ✅ |
| **Cost Intel** | Cost calculators ✅, Cardinality ✅, Usage analytics ✅, Data shaping ✅, Quotas ✅ |
| **Correlation** | Change correlation ✅, Alert enrichment ✅, Dependency graph ✅ |
| **NOT Built** | - |

---

## Phase 1: Core Differentiators (Weeks 1-6)

### P0 - Zero-Config Database Tracing

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Redis eBPF protocol parsing~~ | ✅ Done | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [Redis Wire Format](VISION.md#redis-protocol-parsing) |
| ~~PostgreSQL eBPF protocol parsing~~ | ✅ Done | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [PostgreSQL Wire Format](VISION.md#postgresql-protocol-parsing) |
| ~~MySQL eBPF protocol parsing~~ | ✅ Done | [Zero-Config Tracing](VISION.md#1-zero-config-distributed-tracing), [MySQL Wire Format](VISION.md#mysql-protocol-parsing) |
| ~~Fix SSL/HTTPS probe~~ | ✅ Done | [TLS Interception](VISION.md#2-tls-interception) |

### P1 - DB Probe Refinements

| Task | Complexity | Notes |
|------|------------|-------|
| ~~Fix MySQL protocol detection~~ | ✅ Done | Stricter BPF validation, handles MySQL 8 query attributes, reduced false positives |
| ~~Test PostgreSQL with real server~~ | ✅ Done | Verified: CREATE, INSERT, SELECT with latency tracking |
| ~~Test MySQL with real server~~ | ✅ Done | Verified: SELECT, INSERT, UPDATE with plaintext connections (TLS requires SSL probe) |

### P0 - Production Essentials

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~OTLP receiver (gRPC + HTTP)~~ | ✅ Done | [Ingest Protocol Support](VISION.md#ingest-protocol-support) |
| ~~Backup & restore CLI~~ | ✅ Done | [Backup & Disaster Recovery](VISION.md#2-backup--disaster-recovery) |
| ~~Health check endpoints (`/healthz`, `/readyz`)~~ | ✅ Done | [Production Essentials](VISION.md#production-essentials) |
| ~~Prometheus remote write receiver~~ | ✅ Done | [Prometheus Compatibility](VISION.md#prometheus-compatibility) |

---

## Phase 2: Make People Pay (Weeks 7-12)

### P0 - Cost Intelligence

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Datadog cost calculator~~ | ✅ Done | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| ~~New Relic cost calculator~~ | ✅ Done | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| ~~Splunk cost calculator~~ | ✅ Done | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| ~~Cost trending dashboard~~ | ✅ Done | [Cost Intelligence](VISION.md#2-cost-intelligence) |
| ~~Cost recommendations engine~~ | ✅ Done | [Cost Intelligence](VISION.md#2-cost-intelligence) |

### P0 - Control Plane

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Usage analytics (what's queried vs wasted)~~ | ✅ Done | [Control Plane](VISION.md#3-control-plane) |
| ~~Cardinality explorer~~ | ✅ Done | [Control Plane](VISION.md#3-control-plane), [Pain Point: Cardinality](VISION.md#pain-point-3-cardinality-explosions--high) |
| ~~Data shaping rules (drop/aggregate at ingest)~~ | ✅ Done | [Control Plane](VISION.md#3-control-plane) |
| ~~Team quotas & chargeback~~ | ✅ Done | [Control Plane](VISION.md#3-control-plane) |

### P1 - Log Analysis

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~LogCompare (time period comparison)~~ | ✅ Done | [LogCompare](VISION.md#1-logcompare--steal-this) |
| ~~Pattern detection / LogReduce~~ | ✅ Done | [LogReduce](VISION.md#3-logreduce--pattern-detection--steal-this) |

---

## Phase 3: Root Cause & Correlation (Weeks 13-18)

### P0 - Automatic Root Cause

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~BubbleUp (statistical anomaly explanation)~~ | ✅ Done | [BubbleUp](VISION.md#4-bubbleup-automatic-root-cause-analysis) |
| ~~Change correlation engine~~ | ✅ Done | [Change Correlation](VISION.md#5-change-correlation) |
| ~~Alert auto-enrichment (deploys, related alerts)~~ | ✅ Done | [Pain Point: Slow Incident Response](VISION.md#pain-point-5-slow-incident-response--high) |

### P1 - Dependencies

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Dependency graph from traces~~ | ✅ Done | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |
| ~~Dependency-aware alerting~~ | ✅ Done | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |
| ~~Blast radius estimation~~ | ✅ Done | [Pain Point: Microservices Dependencies](VISION.md#pain-point-13-microservices-dependency-hell--high) |

---

## Phase 4: User Experience (Weeks 19-24)

### P0 - Homepage & Navigation

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Lookout homepage (anomalies at a glance)~~ | ✅ Done | [Lookout](VISION.md#2-lookout-automatic-anomaly-overview--steal-this) |
| ~~Entity synthesis (auto-discover services)~~ | ✅ Done | [Entity Synthesis](VISION.md#1-entity-synthesis--steal-this) |
| ~~Entity relationship mapping~~ | ✅ Done | [Entity Synthesis](VISION.md#1-entity-synthesis--steal-this) |

### P1 - Query & Developer Experience

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Visual query builder~~ | ✅ Done | [Query Builder UX](VISION.md#2-query-builder-ux--steal-this) |
| ~~"My Services" developer view~~ | ✅ Done | [Pain Point: Developer Experience](VISION.md#pain-point-6-developer-experience-gap--medium) |
| ~~"My On-Call" view~~ | ✅ Done | [Pain Point: On-Call Burnout](VISION.md#pain-point-7-on-call-burnout--medium) |

---

## Phase 5: Query & Analysis (Weeks 25-30)

### P1 - Query Language

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~DQL pipe-based query language~~ | ✅ Done | [SPL / Query Language](VISION.md#1-spl-search-processing-language--steal-this) |
| ~~Cross-signal joins (logs + traces)~~ | ✅ Done | [SPL / Query Language](VISION.md#1-spl-search-processing-language--steal-this) |
| ~~Full-text search with BM25 ranking~~ | ✅ Done | [Full-Text Search](VISION.md#1-full-text-search-with-relevance--steal-this) |

### P2 - Advanced Queries

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Recording rules (pre-computed aggregations)~~ | ✅ Done | [Chronosphere Features](VISION.md#6-recording-rules-at-scale) |
| ~~Knowledge objects (reusable query components)~~ | ✅ Done | [Knowledge Objects](VISION.md#2-knowledge-objects--steal-this) |

---

## Phase 6: Security & Compliance (Weeks 31-36)

### P1 - Security

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Threat detection rules (shell-in-container, cryptominers)~~ | ✅ Done | [Security Observability](VISION.md#6-security-observability) |
| ~~Security dashboard~~ | ✅ Done | [Security Observability](VISION.md#6-security-observability) |
| ~~SIEM export (CEF/LEEF)~~ | ✅ Done | [Security Observability](VISION.md#6-security-observability) |

### P1 - Compliance

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~PII detection & redaction~~ | ✅ Done | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |
| ~~Enhanced audit logging (query audit)~~ | ✅ Done | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |
| ~~Compliance reports (SOC2 evidence)~~ | ✅ Done | [Pain Point: Compliance](VISION.md#pain-point-10-compliance--audit-gaps--high) |

---

## Phase 7: Enterprise Scale (Weeks 37+)

### P2 - Data Architecture

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Write-ahead log (WAL)~~ | ✅ Done | [Architectural Gaps](VISION.md#1-write-ahead-log-wal) |
| ~~Hot/warm/cold storage tiering~~ | ✅ Done | [Architectural Gaps](VISION.md#2-hotwarmcold-tiering) |
| ~~Pluggable storage backends~~ | ✅ Done | [Pluggable Storage](VISION.md#pluggable-storage-architecture) |

### P2 - Sampling

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Head sampling with priority rules~~ | ✅ Done | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |
| ~~Tail sampling (keep traces with errors)~~ | ✅ Done | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |
| ~~Adaptive sampling~~ | ✅ Done | [Sampling & Data Reduction](VISION.md#3-sampling--data-reduction) |

### P2 - Migration

| Task | Complexity | VISION.md Reference |
|------|------------|---------------------|
| ~~Datadog dashboard import~~ | ✅ Done | [Migration Assistant](VISION.md#7-migration-assistant) |
| ~~Grafana dashboard import~~ | ✅ Done | [Migration Assistant](VISION.md#7-migration-assistant) |
| ~~Alert rule import~~ | ✅ Done | [Migration Assistant](VISION.md#7-migration-assistant) |

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

## Phase 8: Beat VictoriaMetrics (Protocol & Scale Parity)

Goal: Close every feature gap with VictoriaMetrics so there's no reason to choose them over us.

### P0 - Protocol Ingestion (Make Migration Trivial)

| Task | Complexity | Notes |
|------|------------|-------|
| Graphite plaintext protocol (`/api/graphite/write`) | 1-2 days | Line format: `metric.path value timestamp` |
| Graphite pickle protocol | 1 day | Python pickle batched format |
| InfluxDB line protocol (`/api/influx/write`) | 1-2 days | Format: `measurement,tag=val field=val timestamp` |
| OpenTSDB HTTP (`/api/opentsdb/write`) | 1 day | JSON array of datapoints |
| OpenTSDB telnet protocol (port 4242) | 1 day | `put metric timestamp value tags` |
| Prometheus remote read (`/api/v1/read`) | 2-3 days | Return metrics for external Prometheus queries |
| DataDog agent protocol (`/api/v1/series`) | 1-2 days | Native DD agent compatibility |

### P0 - MetricsQL Extensions

| Task | Complexity | Notes |
|------|------------|-------|
| `label_set(q, "label", "value")` | 1 day | Add/override labels |
| `label_del(q, "label1", "label2")` | 1 day | Remove labels |
| `label_keep(q, "label1", "label2")` | 1 day | Keep only specified labels |
| `label_copy(q, "src", "dst")` | 1 day | Copy label value |
| `label_move(q, "src", "dst")` | 1 day | Move label (copy + delete) |
| `label_transform(q, "label", "regex", "replacement")` | 1 day | Regex transform |
| `label_match(q, "label", "regex")` | 1 day | Filter by label regex |
| `label_mismatch(q, "label", "regex")` | 1 day | Filter by label not matching |
| `union(q1, q2, ...)` | 1 day | Merge multiple queries |
| `ru(free, max)` | 1 day | Resource utilization: `(max - free) / max` |
| `ttf(q)` | 2 days | Time-to-fill: predict when resource exhausts |
| `range_median(q)` | 1 day | Median over range |
| `range_first(q)` | 1 day | First value in range |
| `range_last(q)` | 1 day | Last value in range |
| `running_sum(q)` | 1 day | Cumulative sum |
| `running_max(q)` | 1 day | Cumulative max |
| `running_min(q)` | 1 day | Cumulative min |
| `running_avg(q)` | 1 day | Cumulative average |
| `smooth_exponential(q, sf)` | 1 day | Exponential smoothing |
| `outliers_mad(q, threshold)` | 2 days | MAD-based outlier detection |
| `outliers_iqr(q, threshold)` | 2 days | IQR-based outlier detection |
| `histogram_share(q, le)` | 1 day | Fraction of histogram <= le |
| `histogram_avg(q)` | 1 day | Average from histogram |
| `histogram_stdvar(q)` | 1 day | Variance from histogram |
| `duration_over_time(q, threshold)` | 2 days | Time spent over threshold |
| `share_gt_over_time(q, threshold)` | 1 day | Fraction of time > threshold |
| `share_le_over_time(q, threshold)` | 1 day | Fraction of time <= threshold |
| `count_gt_over_time(q, threshold)` | 1 day | Count of samples > threshold |
| `count_le_over_time(q, threshold)` | 1 day | Count of samples <= threshold |
| `lag(q)` | 1 day | Time since last sample |
| `lifetime(q)` | 1 day | Duration from first to last sample |
| `scrape_interval(q)` | 1 day | Detected scrape interval |

### P1 - Data Processing

| Task | Complexity | Notes |
|------|------------|-------|
| Downsampling at ingest | 3-4 days | Reduce resolution for old data automatically |
| Configurable downsampling rules | 2 days | Per-metric downsampling policies |
| Streaming aggregation | 1 week | Pre-aggregate at ingest time (sum, count, min, max, avg) |
| Deduplication | 2-3 days | Detect and remove duplicate samples |
| Relabeling at ingest | 2-3 days | Transform labels before storage |

### P2 - Scale Features (When Needed)

| Task | Complexity | Notes |
|------|------------|-------|
| Multi-tenancy | 2-3 weeks | Isolated data per tenant, per-tenant auth |
| Clustering (vmselect/vminsert/vmstorage model) | 4-6 weeks | Distributed storage and query |
| Cross-tenant queries | 1 week | Admin queries across tenants |
| Per-tenant retention | 1 week | Different retention per tenant |
| Per-tenant rate limits | 3-4 days | Protect against noisy neighbors |

### P2 - Operational

| Task | Complexity | Notes |
|------|------------|-------|
| Kubernetes operator | 2-3 weeks | CRDs for dogwatch resources |
| Helm chart | 3-4 days | Production-ready Helm deployment |
| vmctl-equivalent migration tool | 1 week | Migrate data from Prometheus/VictoriaMetrics/Thanos |

---

## Technical Moats (Competitive Differentiators)

These are the deep technical capabilities that create defensible competitive advantage. See [VISION.md Technical Moats](VISION.md#technical-moats) for full context.

### Moat 1-3: eBPF Deep Compatibility (Done)

| Task | Complexity | Status | VISION.md Reference |
|------|------------|--------|---------------------|
| ~~Kernel version compatibility (4.x - 6.x)~~ | 4-6 weeks | ✅ Done | [Kernel Compatibility](VISION.md#moat-1-kernel-version-compatibility) |
| ~~TLS interception (OpenSSL, BoringSSL, Go crypto)~~ | 6-8 weeks | ✅ Done | [TLS Interception](VISION.md#moat-2-tls-interception) |
| ~~Protocol edge cases (prepared statements, pipelining)~~ | 3-4 weeks | ✅ Done | [Protocol Edge Cases](VISION.md#moat-3-protocol-edge-cases) |

*Features: Kernel version detection (4.4-6.x), BTF availability check, BPF feature probing, perf/ring buffer detection, OpenSSL 1.1/3.x uprobes, Go crypto/tls symbol extraction, prepared statement tracking (MySQL/PostgreSQL), connection state caching.*

### Moat 4-6: Intelligent Analysis (Done)

| Task | Complexity | Status | VISION.md Reference |
|------|------------|--------|---------------------|
| ~~Correlation without trace headers (socket/timing-based)~~ | 4-6 weeks | ✅ Done | [Headerless Correlation](VISION.md#moat-4-correlation-without-trace-headers) |
| ~~Low-overhead continuous profiling (all languages)~~ | 3-4 weeks | ✅ Done | [Continuous Profiling](VISION.md#moat-5-low-overhead-continuous-profiling) |
| ~~Security detection without false positives (baseline learning)~~ | 2-3 weeks | ✅ Done | [Security Detection](VISION.md#moat-6-security-detection-without-false-positives) |

*Features: Headerless correlation via socket-based (same thread inbound→outbound), timing-based (same PID within time window), and content-based (X-Request-ID headers) methods; connection pooling detection; async worker pattern detection; confidence scoring by method. Full symbol resolution (kernel + userspace ELF), /proc/[pid]/maps parsing, symbol caching, readable flame graphs. Security baseline learning per-container, process/network/file pattern learning, anomaly detection after warmup, false positive feedback handling, parent-child relationship tracking.*

### Moat 7-9: Data Intelligence (Done)

| Task | Complexity | Status | VISION.md Reference |
|------|------------|--------|---------------------|
| ~~Migration fidelity (template variables, composite monitors)~~ | 4-6 weeks | ✅ Done | [Migration Fidelity](VISION.md#moat-7-migration-fidelity) |
| ~~Intelligent tail sampling (adaptive, retroactive for errors)~~ | 3-4 weeks | ✅ Done | [Tail Sampling](VISION.md#moat-8-intelligent-tail-sampling) |
| ~~Automatic log field extraction (pattern learning)~~ | 3-4 weeks | ✅ Done | [Field Extraction](VISION.md#moat-9-automatic-log-field-extraction) |

*Features: Composite monitor query parsing (AND/OR/NOT operators, monitor reference resolution), batch import with composite resolution, retroactive trace capture (error/high-latency triggers, parent/child relationship tracking), anomaly detection callbacks, pattern learning for log extraction (signature computation, candidate generation, confidence scoring), log structure analysis (JSON/KV/syslog detection), field detection (IP/email/UUID/key-value patterns).*

### Moat 10-12: Scale & Efficiency (Done)

| Task | Complexity | Status | VISION.md Reference |
|------|------------|--------|---------------------|
| Multi-signal correlation engine (exemplars, fuzzy matching) | 2-3 weeks | ✅ Done | [Multi-Signal Correlation](VISION.md#moat-10-multi-signal-correlation-engine) |
| SQLite time-series optimization | 2-3 weeks | ✅ Done | [SQLite Optimization](VISION.md#moat-11-sqlite-time-series-optimization) |
| Cardinality bomb prevention (auto-detection, circuit breaker) | 2 weeks | ✅ Done | [Cardinality Prevention](VISION.md#moat-12-cardinality-bomb-prevention) |

*Features: Exemplars linking metrics to traces, fuzzy matching via time/service/operation, log-to-trace correlation, metric-to-trace correlation, cross-signal timeline, time-based partitioning, downsampling rules, batch insert buffering, circuit breaker, quarantine system, cardinality alerts.*

### Moat 13-15: Operational Excellence (Done)

| Task | Complexity | Status | VISION.md Reference |
|------|------------|--------|---------------------|
| Hot probe reload (update eBPF without restart) | 2 weeks | ✅ Done | [Hot Reload](VISION.md#moat-13-hot-probe-reload) |
| Clock skew tolerance (distributed clock handling) | 1-2 weeks | ✅ Done | [Clock Skew](VISION.md#moat-14-clock-skew-tolerance) |
| Memory-efficient operation (<500MB for 10K eps) | 2-3 weeks | ✅ Done | [Memory Efficiency](VISION.md#moat-15-memory-efficient-operation) |

### Technical Moats Summary

| Category | Status | Total Effort |
|----------|--------|--------------|
| eBPF Compatibility (1-3) | ✅ 100% | ~14 weeks |
| Intelligent Analysis (4-6) | ✅ 100% | ~10 weeks |
| Data Intelligence (7-9) | ✅ 100% | ~11 weeks |
| Scale & Efficiency (10-12) | ✅ 100% | ~7 weeks |
| Operational Excellence (13-15) | ✅ 100% | ~6 weeks |
| **Total** | **✅ 100%** | **~48 weeks** |

---

## Completed ✅

| Category | Items |
|----------|-------|
| **eBPF** | TCP connections, HTTP/1.1 parsing, CPU profiling/flamegraphs, SSL/HTTPS (tracefs uprobes), Redis protocol, PostgreSQL protocol |
| **OTLP** | gRPC (4317) + HTTP (4318) receivers for traces, metrics, logs |
| **Backup** | CLI + API, scheduler, retention policy, verification |
| **Storage** | All SQLite stores (metrics, traces, logs, dashboards, alerts, SLOs, synthetics, incidents, on-call, RBAC, audit, SSO, deploys, notify, catalog) |
| **Alerting** | Rules, evaluation, routing, inhibition |
| **Notifications** | Slack, PagerDuty, OpsGenie, Discord, MS Teams, Email, Webhook |
| **Auth** | RBAC (Owner/Admin/Editor/Viewer), API keys, JWT sessions, OAuth2, SAML SSO |
| **Features** | Dashboards, SLOs, Synthetics, Incidents, On-call, Service catalog, Anomaly detection (IForest), Deploy tracking |
| **Infrastructure** | Federation (gossip), K8s collector, Container monitoring, Rate limiting, Pagination, Health checks |
| **APM SDK** | HTTP middleware, SQL/Redis/gRPC instrumentation |
| **UI** | Web server, Service map, History graphs, Demo data, Lookout homepage, Entity synthesis/mapping, My On-Call dashboard, My Services developer view |
| **Cost Intel** | Datadog/New Relic/Splunk calculators, Cost trending, Recommendations, Cardinality explorer, Usage analytics, Data shaping, Team quotas, Cardinality bomb prevention (circuit breaker, quarantine, alerts, auto-aggregation) |
| **Correlation** | Change correlation engine, Alert auto-enrichment, Deploy-incident correlation, Dependency graph, Multi-signal correlation (exemplars, fuzzy matching, log-to-trace, metric-to-trace, cross-signal timeline), Headerless correlation (socket/timing/content-based, connection pooling detection, async worker detection) |
| **Root Cause** | BubbleUp (chi-squared analysis, lift calculation, dimension ranking) |
| **Logs** | LogCompare (time period comparison), LogReduce (pattern detection), BM25 relevance search |
| **Production** | Prometheus remote write receiver, Health check endpoints (/healthz, /readyz, /livez) |
| **Query** | DQL (pipe + SQL syntax), Cross-signal JOINs, BM25 full-text search, Recording rules, Knowledge objects (macros, field extractions, lookups) |
| **Security** | Threat detection (13 rules), Security dashboard, MITRE ATT&CK mapping, Alert investigation |
| **Compliance** | PII detection (email, phone, SSN, credit cards, API keys, JWT), Redaction strategies (mask, hash, tokenize) |
| **Data Arch** | WAL (write-ahead log), Hot/warm/cold tiering, Pluggable backends (LocalFS, S3, GCS), SQLite time-series optimization (partitioning, downsampling, batch inserts, covering indexes) |
| **Sampling** | Head sampling (priority rules), Tail sampling (error/latency detection), Adaptive sampling (per-service rates), Retroactive capture (error/high-latency triggers, parent/child tracking), Anomaly detection (Z-score, baseline learning) |
| **Migration** | Datadog import (dashboards, monitors), Grafana import (dashboards, alerts), Prometheus rules, Auto-format detection, Composite monitor parsing (AND/OR/NOT, batch resolution), Template variables |
| **Log Intelligence** | Pattern learning (signature computation, candidate generation), Log structure analysis (JSON/KV/syslog detection), Field extraction (IP/email/UUID/key-value patterns), Confidence scoring |
| **Operational** | Hot probe reload (versioning, graceful swap, rollback), Clock skew tolerance (detection, correction, NTP drift), Memory efficiency (object pools, buffer recycling, backpressure) |

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

---

## Frontend Performance Refactoring ✅

Goal: Transform 557KB monolithic frontend into modular, real-time system.

| Task | Status |
|------|--------|
| CSS extraction (variables, base, components, layout, bundle) | ✅ Done |
| WebSocket backend (websocket.go) | ✅ Done |
| WebSocket client (js/websocket.js) | ✅ Done |
| Wire WebSocket hub to server.go | ✅ Done |
| Wire WebSocket to watch engine | ✅ Done |
| Add /api/ws route | ✅ Done |
| Lazy loader (js/loader.js) | ✅ Done |
| Web Components (status-badge, metrics-card, service-map, trace-viewer, log-viewer) | ✅ Done |
| Update index.html (CSS extraction) | ✅ Done |
| Frontend WebSocket subscriptions (8 topics) | ✅ Done |
| Polling fallback for robustness | ✅ Done |

### Results
| Metric | Before | After | Target |
|--------|--------|-------|--------|
| Initial HTML | 557KB | **40KB** | <100KB ✅ |
| Real-time updates | Polling only | **WebSocket primary** | WebSocket ✅ |

---

## Quick Reference

| Question | Answer |
|----------|--------|
| **Build first** | DQL query language (Phase 5) or Security features (Phase 6) |
| **Makes people pay** | ✅ Cost Intelligence, Control Plane, LogCompare - all done |
| **Makes people stay** | ✅ BubbleUp, Lookout, Entity synthesis - all done |
| **The moat** | Zero-config eBPF + cost transparency |
| **What's broken** | Nothing critical (MySQL works with plaintext, TLS needs SSL probe) |
| **Full context** | [VISION.md](VISION.md) (22,400 lines) |

---

## Priority Legend

| Priority | Meaning |
|----------|---------|
| **P0** | Must have - blocks adoption or differentiation |
| **P1** | Should have - significant value |
| **P2** | Nice to have - do after P0/P1 |
| **P3** | Future - revisit later |
