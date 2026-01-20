# TODO - Prioritized Roadmap

Based on [VISION.md](VISION.md) analysis. Items ordered by priority within each phase.

---

## Phase 1: Make It Work (Weeks 1-4)

### P0 - Core Functionality

- [ ] **MySQL eBPF Protocol Parsing**
  - Zero-config tracing for MySQL queries
  - [VISION.md: Zero-Config Distributed Tracing](VISION.md#1-zero-config-distributed-tracing)

- [ ] **PostgreSQL eBPF Protocol Parsing**
  - Zero-config tracing for PostgreSQL queries
  - [VISION.md: Zero-Config Distributed Tracing](VISION.md#1-zero-config-distributed-tracing)

- [ ] **Redis eBPF Protocol Parsing**
  - Zero-config tracing for Redis commands
  - [VISION.md: Zero-Config Distributed Tracing](VISION.md#1-zero-config-distributed-tracing)

- [ ] **OTLP Receiver (gRPC + HTTP)**
  - Accept OpenTelemetry data on ports 4317/4318
  - Table stakes - industry standard
  - [VISION.md: Ingest Protocol Support](VISION.md#ingest-protocol-support)

- [ ] **Backup & Restore**
  - `dogwatch backup` / `dogwatch restore` commands
  - Required for production use
  - [VISION.md: Backup & Disaster Recovery](VISION.md#2-backup--disaster-recovery)

- [ ] **Basic Sampling**
  - Head sampling with priority rules (keep errors, slow requests)
  - [VISION.md: Sampling & Data Reduction](VISION.md#3-sampling--data-reduction)

---

## Phase 2: Make People Pay (Weeks 5-8)

### P0 - Differentiation

- [ ] **Cost Intelligence Dashboard**
  - Show "this would cost $X on Datadog/New Relic/Splunk"
  - Real-time cost tracking and projections
  - Our #2 killer feature - no competitor does this
  - [VISION.md: Cost Intelligence](VISION.md#2-cost-intelligence)

- [ ] **LogCompare**
  - Compare logs between two time periods
  - "What changed between working and broken?"
  - Stolen from Sumo Logic
  - [VISION.md: LogCompare](VISION.md#1-logcompare--steal-this)

- [ ] **Pattern Detection (LogReduce)**
  - Auto-cluster 1M logs into 5 patterns
  - Surface NEW and INCREASING patterns
  - Stolen from Splunk
  - [VISION.md: LogReduce / Pattern Detection](VISION.md#3-logreduce--pattern-detection--steal-this)

### P1 - Pain Points

- [ ] **Alert Grouping & Context**
  - 50 alerts → 1 grouped alert with context
  - Include related alerts, recent deploys, suggested cause
  - [VISION.md: Alert Fatigue](VISION.md#pain-point-1-alert-fatigue--critical)

- [ ] **Incident Auto-Enrichment**
  - Auto-include related traces, logs, deploys, similar incidents
  - Build timeline automatically
  - [VISION.md: Slow Incident Response](VISION.md#pain-point-5-slow-incident-response--high)

---

## Phase 3: Make People Stay (Weeks 9-12)

### P0 - User Experience

- [ ] **Lookout Homepage**
  - Open dogwatch → see what's abnormal immediately
  - No query required, prioritized by impact
  - Stolen from New Relic
  - [VISION.md: Lookout (Automatic Anomaly Overview)](VISION.md#2-lookout-automatic-anomaly-overview--steal-this)

- [ ] **Entity Synthesis**
  - Auto-discover services, databases, queues, external APIs
  - Show relationships (calls, runs-on, depends-on)
  - Golden signals per entity
  - Stolen from New Relic
  - [VISION.md: Entity Synthesis](VISION.md#1-entity-synthesis--steal-this)

- [ ] **Query Builder UX**
  - Visual query construction with dropdowns
  - Field autocomplete from actual values
  - Shows generated DQL as you build
  - Stolen from Honeycomb
  - [VISION.md: Query Builder UX](VISION.md#2-query-builder-ux--steal-this)

### P1 - Developer Experience

- [ ] **"My Services" View**
  - Developer-centric: show services I own
  - My recent changes + their impact
  - [VISION.md: Developer Experience Gap](VISION.md#pain-point-6-developer-experience-gap--medium)

- [ ] **Dependency Tracking**
  - Show service dependencies from traces
  - Root cause through dependency graph
  - Alert on unexpected new dependencies
  - [VISION.md: Microservices Dependency Hell](VISION.md#pain-point-13-microservices-dependency-hell--high)

---

## Phase 4: Production Ready (Weeks 13-16)

### P1 - Enterprise Requirements

- [ ] **DQL Query Language**
  - Pipe-based query language like Splunk SPL
  - `logs | where service == "api" | stats count() by endpoint`
  - Cross-signal joins (logs + traces + metrics)
  - [VISION.md: SPL (Search Processing Language)](VISION.md#1-spl-search-processing-language--steal-this)

- [ ] **Full-Text Search**
  - BM25 relevance ranking
  - Natural language queries
  - "payment timeout" → ranked results
  - Stolen from Elasticsearch
  - [VISION.md: Full-Text Search with Relevance](VISION.md#1-full-text-search-with-relevance--steal-this)

- [ ] **Cardinality Management**
  - Analysis dashboard showing top cardinality contributors
  - Alerts on cardinality spikes
  - Controls to drop/limit high-cardinality labels
  - [VISION.md: Cardinality Explosions](VISION.md#pain-point-3-cardinality-explosions--high)

- [ ] **Kubernetes-Native Views**
  - Cluster overview, namespace view, pod detail
  - Events, logs, traces per pod
  - OOMKilled, CrashLoopBackOff detection
  - [VISION.md: Kubernetes Complexity](VISION.md#pain-point-4-kubernetes-complexity--high)

- [ ] **Audit Logging**
  - Who logged in, what queries ran, what changed
  - Required for SOC2/HIPAA
  - [VISION.md: Compliance & Audit Gaps](VISION.md#pain-point-10-compliance--audit-gaps--high)

---

## Phase 5: Scale & Polish (Weeks 17+)

### P1 - Advanced Features

- [ ] **Histograms (First-Class)**
  - Store full distributions, query any percentile
  - Accurate p99.9+ without pre-defined buckets
  - Stolen from Wavefront
  - [VISION.md: Histograms as First-Class Citizens](VISION.md#1-histograms-as-first-class-citizens--steal-this)

- [ ] **Delta Counters**
  - Correct counting for serverless/ephemeral compute
  - Works with Lambda, K8s pods that come and go
  - Stolen from Wavefront
  - [VISION.md: Delta Counters](VISION.md#2-delta-counters--steal-this)

- [ ] **Data Pipeline Routing**
  - Route different data to different destinations
  - Sample, redact, enrich in transit
  - Stolen from Cribl
  - [VISION.md: Data Routing & Transformation](VISION.md#1-data-routing--transformation--steal-this)

- [ ] **Business Context**
  - Revenue impact per incident
  - Affected customer count and tiers
  - SLA breach status
  - [VISION.md: Lack of Business Context](VISION.md#pain-point-14-lack-of-business-context--high)

- [ ] **Multi-Cloud Visibility**
  - Unified view across AWS/GCP/Azure/on-prem
  - Cross-cloud tracing
  - [VISION.md: Multi-Cloud / Hybrid Visibility](VISION.md#pain-point-11-multi-cloud--hybrid-visibility--high)

### P2 - Nice to Have

- [ ] **Knowledge Objects**
  - Saved searches that power dashboards + alerts + reports
  - Change once, update everywhere
  - Stolen from Splunk
  - [VISION.md: Knowledge Objects](VISION.md#2-knowledge-objects--steal-this)

- [ ] **Derived Metrics**
  - Define metrics as computations of other metrics
  - `error_rate = errors / requests`
  - Stolen from Wavefront
  - [VISION.md: Derived Metrics](VISION.md#3-derived-metrics--steal-this)

- [ ] **SOAR Playbooks**
  - Automated incident response
  - Gather context → check correlation → auto-remediate
  - [VISION.md: SOAR](VISION.md#4-soar-security-orchestration-automation-response--partial-steal)

- [ ] **IDE Integration (VS Code)**
  - Show errors/latency inline in code
  - Jump to traces from function
  - [VISION.md: IDE Integration](VISION.md#7-ide-integration)

- [ ] **Terraform Provider**
  - Manage dashboards, alerts, SLOs as code
  - [VISION.md: Observability as Code / Terraform Integration](VISION.md#trend-8-observability-as-code--terraform-integration--partial)

- [ ] **Wasm Plugin System**
  - Custom protocol parsers
  - PII redaction rules
  - Custom sampling logic
  - [VISION.md: WebAssembly for Extensibility](VISION.md#trend-7-webassembly-for-extensibility--opportunity)

### P3 - Future

- [ ] **LLM Integration**
  - Natural language queries
  - "Why is checkout slow?" → AI analysis
  - Use Claude/GPT APIs, not custom ML
  - [VISION.md: AI/ML-Powered Features](VISION.md#1-aiml-powered-features-critical-gap)

- [ ] **Canvas Dashboards**
  - Pixel-perfect NOC/exec displays
  - TV mode with rotation
  - Stolen from Elastic
  - [VISION.md: Canvas (Presentation Dashboards)](VISION.md#2-canvas-presentation-dashboards--steal-this)

- [ ] **High Availability**
  - Active-passive with shared storage
  - Later: active-active cluster
  - [VISION.md: High Availability](VISION.md#1-high-availability)

---

## Completed

### Service Map
- [x] Zoom transitions performance
- [x] Layout spacing
- [x] Pan interaction

### Widgets
- [x] CPU history graph
- [x] Memory history graph

### Demo Data
- [x] Service map nodes and connections
- [x] Traces with multiple spans
- [x] Logs with various levels
- [x] Incidents and alerts
- [x] Deployments
- [x] SLOs with error budgets
- [x] Synthetics checks
- [x] Flame graph samples
- [x] Anomaly detections
- [x] On-call schedules

---

## Not Building (Explicitly Skipped)

| Item | Reason | Alternative |
|------|--------|-------------|
| RUM / Browser SDK | Different product | Integrate with Sentry/LogRocket |
| Mobile SDK | Different domain | Accept OTLP from mobile |
| Full SIEM | Too broad | Focus on detection rules |
| Custom ML Models | Expensive, unreliable | Use LLM APIs |
| 600+ Integrations | Years of work | Focus on auto-discovery |
| Session Replay | Different product | Integrate with LogRocket |

---

## Quick Reference

**What to build first:** MySQL parsing → PostgreSQL → Redis → OTLP → Backup

**What makes people pay:** Cost Intelligence, LogCompare, Pattern Detection

**What makes people stay:** Lookout, Entity view, Query Builder

**Full details:** [VISION.md](VISION.md) (10,779 lines)
