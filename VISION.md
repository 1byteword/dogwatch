# dogwatch Vision

## The Pitch

**dogwatch** is observability that works out of the box.

Drop a single binary on your server. Within 60 seconds, see:
- Every service talking to every other service
- Every HTTP request, database query, and cache hit
- Distributed traces across your entire stack
- **No code changes. No SDKs. No configuration.**

Then show your boss: *"This would cost us $47,000/month on Datadog."*

---

## Glossary

Key terms used throughout this document:

| Term | Definition |
|------|------------|
| **Cardinality** | The number of unique combinations of label values for a metric. A metric with labels `{method, path, status}` where method has 5 values, path has 1000 values, and status has 10 values has cardinality of 5×1000×10 = 50,000 series. High cardinality (>100K series per metric) causes storage and query performance issues. |
| **Control Plane** | A data management layer that sits between data ingestion and storage. It analyzes, shapes, and optimizes observability data—aggregating unused labels, dropping never-queried metrics, and enforcing team quotas—before data hits permanent storage. Named after Kubernetes' control plane concept. |
| **BubbleUp** | An automated root cause analysis technique. When you select a set of "bad" requests (slow, erroring), BubbleUp statistically compares them against "good" requests to find which dimensions (service, endpoint, region, etc.) are disproportionately represented in the bad set. |
| **Exemplars** | Sample trace IDs attached to metric data points. When you see a latency spike in a graph, exemplars let you click through to an actual trace that contributed to that spike. Bridges metrics → traces. |
| **Entity** | A logical unit in your infrastructure: a service, database, queue, host, or container. Entity Synthesis automatically discovers these from telemetry data and tracks their relationships (service A calls service B, runs on host C). |
| **Span** | A single unit of work in a distributed trace. A trace is a tree of spans. Each span has: operation name, start time, duration, parent span ID, and attributes. Example: "HTTP GET /api/users" taking 45ms. |
| **Golden Signals** | The four key metrics for any service: latency, traffic, errors, and saturation (from Google SRE). If you only monitor four things, monitor these. |
| **Head Sampling** | Making the keep/drop decision at trace start (before you know if it's interesting). Opposite of tail sampling, which decides after the trace completes. Head sampling is simpler but can't keep "all errors" since you don't know yet if it will error. |
| **Tail Sampling** | Making the keep/drop decision after a trace completes, so you can keep interesting traces (errors, slow requests) and drop boring ones. Requires buffering all spans until the trace completes. |
| **Recording Rules** | Pre-computed aggregations that run continuously. Instead of computing `sum(rate(http_requests_total[5m])) by (service)` at query time, a recording rule computes it every 15s and stores the result. Faster queries, but uses storage. |
| **Uptime Probe / Synthetic Check** | An automated test that periodically hits an endpoint and records success/failure/latency. "Is my API responding?" checked every 60 seconds from multiple locations. |
| **SLO (Service Level Objective)** | A target for service reliability, expressed as a percentage over a time window. "99.9% of requests complete in <500ms over 30 days." SLOs have error budgets—if you're at 99.95%, you have budget to deploy risky changes. |
| **eBPF (Extended Berkeley Packet Filter)** | Linux kernel technology that lets you run sandboxed programs in kernel space. dogwatch uses eBPF to observe all network traffic, syscalls, and function calls without modifying applications. Requires Linux kernel 4.14+ (5.8+ recommended). |
| **Wire Format** | The raw bytes sent over the network for a protocol. Parsing wire format means reading TCP packets and understanding "these bytes are a MySQL query." |
| **uprobes** | eBPF hooks that attach to user-space functions. Used to intercept SSL/TLS libraries (like OpenSSL's `SSL_read`/`SSL_write`) to see encrypted traffic before/after encryption. |
| **Cardinality Explosion** | When a label has unbounded values (user IDs, request IDs, timestamps), causing metric series count to grow without limit. A single bad label can create millions of series and crash your monitoring system. |
| **OTLP (OpenTelemetry Protocol)** | The standard protocol for sending traces, metrics, and logs. gRPC on port 4317, HTTP on port 4318. Supporting OTLP means any OpenTelemetry-instrumented app can send data to dogwatch. |
| **PromQL** | Prometheus Query Language. The de facto standard for querying time-series metrics. `rate(http_requests_total[5m])` means "requests per second over the last 5 minutes." |

---

## Seven Killer Features

### 1. Zero-Config Distributed Tracing

**The problem:** Traditional APM requires instrumenting every service with SDKs, propagating trace headers, configuring exporters. Weeks of work. Every team has to cooperate.

**The dogwatch solution:** eBPF sees all network traffic at the kernel level. We parse protocols, correlate requests across services, and build traces automatically.

```
Traditional Tracing:
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  Service A  │───▶│  Service B  │───▶│  Service C  │
│ [add SDK]   │    │ [add SDK]   │    │ [add SDK]   │
│ [configure] │    │ [configure] │    │ [configure] │
└─────────────┘    └─────────────┘    └─────────────┘
        Weeks of integration work per service

dogwatch:
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  Service A  │───▶│  Service B  │───▶│  Database   │
└─────────────┘    └─────────────┘    └─────────────┘
        │                │                   │
        └────────────────┴───────────────────┘
                         │
                    eBPF kernel
                  (sees everything)
                         │
                         ▼
              Automatic distributed trace
              A (23ms) → B (18ms) → DB (12ms)
```

**How it works:**

1. **TCP tracking** - eBPF hooks `tcp_connect`, `inet_csk_accept` to see all connections
2. **Protocol parsing** - Detect HTTP, MySQL, PostgreSQL, Redis from wire format
3. **Request correlation** - Match requests to responses using socket + timing
4. **Cross-service linking** - Parse trace headers OR correlate by timing windows
5. **Service map generation** - Build topology from actual traffic, not configuration

**What we can trace without any instrumentation:**

| Protocol | Data Captured | Status |
|----------|---------------|--------|
| HTTP/1.1 | Method, path, status, latency, headers | ✅ Working |
| HTTPS | Same as HTTP (via SSL uprobes) | 🔧 In Progress |
| MySQL | Query text, latency, rows, errors | 🎯 Priority |
| PostgreSQL | Query text, latency, rows, errors | 🎯 Priority |
| Redis | Command, key, latency | 🎯 Priority |
| gRPC/HTTP2 | Method, status, latency | Planned |
| DNS | Query, response, latency | Planned |
| Kafka | Topic, partition, offset | Planned |

**The Pixie proof point:** New Relic acquired Pixie for this capability two months after launch. We're building the open-source, self-hosted alternative.

---

### 2. Cost Intelligence

**The problem:** Observability is expensive. Datadog bills shock teams every month. But the value is abstract - "we need monitoring" doesn't have a dollar figure.

**The dogwatch solution:** Show exactly what your usage would cost on Datadog, New Relic, or Splunk - in real time.

```
┌─────────────────────────────────────────────────────────────┐
│                    Cost Intelligence                         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Your dogwatch usage this month:                            │
│  ├── 847 hosts monitored                                    │
│  ├── 12.4M metric points/hour                               │
│  ├── 2.1TB logs ingested                                    │
│  ├── 94M trace spans                                        │
│  └── 1,247 synthetic checks                                 │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  Estimated costs on other platforms:                │    │
│  │                                                      │    │
│  │  Datadog:     $47,234/month                         │    │
│  │  New Relic:   $38,920/month                         │    │
│  │  Splunk:      $52,100/month                         │    │
│  │                                                      │    │
│  │  dogwatch:    $0 (self-hosted)                      │    │
│  │               or $299/month (supported)             │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  💰 You're saving approximately $46,935/month               │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Implementation:**

```go
type CostCalculator struct {
    // Track actual usage
    HostCount       int64
    MetricPoints    int64  // per hour
    LogBytesIngested int64
    TraceSpans      int64
    SyntheticChecks int64

    // Calculate competitor pricing
    DatadogEstimate()   float64  // $15-27/host + $0.10/GB logs + $1.70/M spans
    NewRelicEstimate()  float64  // $0.30/GB ingested (usage-based)
    SplunkEstimate()    float64  // $2-3/GB indexed
}
```

**Why this matters:**
- Makes the value proposition concrete and shareable
- CFOs understand "$47K/month savings"
- Creates viral screenshots for marketing
- Reinforces the "Datadog alternative" positioning

---

### 3. Control Plane (Data Shaping)

**The problem:** Observability data grows exponentially. Teams send 10x more metrics than they actually query. High-cardinality labels explode storage costs. One runaway service can blow your entire budget.

**The dogwatch solution:** Analyze what data is actually used, recommend optimizations, and let teams shape their data before it hits storage.

```
┌─────────────────────────────────────────────────────────────┐
│                      Control Plane                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ⚠️  High Cardinality Alert                                  │
│                                                              │
│  Metric: http_requests_total                                │
│  Labels: method, path, status, pod, node, region, az        │
│  Current cardinality: 2.4M unique series                    │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  Recommendation: Aggregate away 'pod' and 'node'    │    │
│  │                                                      │    │
│  │  → Reduces to 12K series (99.5% reduction)          │    │
│  │  → No dashboards use these labels                   │    │
│  │  → No alerts query these labels                     │    │
│  │  → Last queried: never                              │    │
│  │                                                      │    │
│  │  [Apply Rule]  [Ignore]  [Preview Impact]           │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  Unused Metrics (last 30 days):                             │
│  ├── go_gc_duration_seconds (847 series) - never queried   │
│  ├── process_open_fds (423 series) - never queried         │
│  └── ... 127 more metrics                                   │
│                                                              │
│  Potential reduction: 84% of current volume                 │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Key capabilities:**

| Feature | What it does |
|---------|--------------|
| **Cardinality Explorer** | Find which labels are exploding your metric count |
| **Usage Analysis** | Track which metrics are used in dashboards, alerts, queries |
| **Utility Scoring** | Auto-score each metric by value (queried often = high, never = low) |
| **Shaping Rules** | Aggregate, downsample, or drop metrics before storage |
| **Team Quotas** | Prevent one team from consuming all capacity |
| **Impact Preview** | "If we drop this, what breaks?" before applying rules |

**Implementation:**

```go
type ControlPlane struct {
    // Track metric usage
    MetricUsage map[string]*UsageStats

    // Cardinality analysis
    GetHighCardinalityMetrics(threshold int) []CardinalityReport

    // Recommendations engine
    GenerateRecommendations() []Recommendation

    // Shaping rules
    ApplyAggregationRule(metric string, dropLabels []string)
    ApplyDropRule(metric string)
    ApplyDownsampleRule(metric string, interval time.Duration)
}

type UsageStats struct {
    MetricName       string
    Cardinality      int64           // unique series count
    BytesStored      int64
    QueriesLast30d   int64           // how often queried
    DashboardsUsing  []string        // which dashboards reference it
    AlertsUsing      []string        // which alerts reference it
    LastQueried      time.Time
    UtilityScore     float64         // 0-100, computed from above
}

type Recommendation struct {
    Type        string  // "drop", "aggregate", "downsample"
    Metric      string
    Reason      string  // "never queried", "high cardinality label"
    Impact      string  // "reduces 2.4M series to 12K"
    SafeToApply bool    // true if nothing would break
}
```

**Why this matters:**

Chronosphere built this and got acquired for $3.35B. Their customers see **84% reduction in data volumes** on average.

For dogwatch, Control Plane directly supports the cost story:
- Cost Intelligence shows "you'd pay $47K on Datadog"
- Control Plane shows "and here's how to use 84% less data"
- Combined message: "dogwatch is free AND helps you be efficient"

**The Chronosphere proof point:** This feature was central to their value proposition of "observability at 1/3 the cost." It's not just about being cheaper - it's about being smarter with data.

---

### 4. BubbleUp (Automatic Root Cause)

**The problem:** Something's slow. You have millions of requests, hundreds of dimensions (user_id, region, endpoint, db_shard, pod, version...). Finding what's different about the slow requests takes hours of manual investigation.

**The dogwatch solution:** Automatically compare anomalous requests against the baseline. Find the dimensions that explain the difference. Surface root cause in seconds, not hours.

```
┌─────────────────────────────────────────────────────────────┐
│  🔍 BubbleUp: What's different about these slow requests?   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Analyzing: 847 slow requests (p99 > 2s)                    │
│  Baseline: 12,432 normal requests                           │
│                                                              │
│  Scanning 147 dimensions...                                 │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  🎯 Root Cause Identified (98.2% confidence)        │    │
│  │                                                      │    │
│  │  These requests are slow because:                   │    │
│  │                                                      │    │
│  │  1. db_shard = 7                                    │    │
│  │     → 98% of slow requests hit shard 7             │    │
│  │     → Only 8% of normal requests hit shard 7       │    │
│  │                                                      │    │
│  │  2. region = eu-west-1                              │    │
│  │     → 94% of slow requests from eu-west-1          │    │
│  │     → Only 12% of normal requests from eu-west-1   │    │
│  │                                                      │    │
│  │  Likely cause: DB shard 7 is degraded in eu-west-1 │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  [View shard 7 traces] [Check DB metrics] [Create incident] │
└─────────────────────────────────────────────────────────────┘
```

**How it works:**

1. **Select anomalous events** - slow requests, errors, timeouts
2. **Compare distributions** - for every dimension, compare anomalous vs baseline
3. **Statistical significance** - chi-squared test or KL divergence per dimension
4. **Rank by explanatory power** - which dimensions most separate anomalous from normal
5. **Surface insights** - "98% of slow requests hit shard 7, but only 8% of normal requests do"

**Implementation:**

```go
type BubbleUp struct {
    // Compare two sets of events
    Anomalous []Event  // e.g., requests where latency > p99
    Baseline  []Event  // e.g., all other requests in time window
}

type DimensionAnalysis struct {
    Dimension       string   // e.g., "db_shard"
    AnomalousDistro map[string]float64  // value → percentage
    BaselineDistro  map[string]float64
    Divergence      float64  // KL divergence or chi-squared
    TopValue        string   // e.g., "7"
    TopValueLift    float64  // e.g., 12.25x more likely in anomalous
}

func (b *BubbleUp) Analyze() []DimensionAnalysis {
    results := []DimensionAnalysis{}

    // For every dimension in the data
    for _, dim := range b.getAllDimensions() {
        analysis := b.compareDimension(dim)
        if analysis.Divergence > threshold {
            results = append(results, analysis)
        }
    }

    // Sort by divergence (most explanatory first)
    sort.Slice(results, func(i, j int) bool {
        return results[i].Divergence > results[j].Divergence
    })

    return results
}
```

**Why this matters:**

Honeycomb built their entire company around this concept. It's the "aha moment" that converts users - seeing root cause surface automatically instead of manually hunting through dashboards.

**The Honeycomb proof point:** They call it "observability 2.0" - the shift from "dashboards you stare at" to "questions the system answers." BubbleUp is why teams pay $50K+/year for Honeycomb.

---

### 5. Change Correlation (What Changed?)

**The problem:** Incident starts at 14:32. Was it a deploy? A config change? A feature flag? A dependency update? You spend 20 minutes asking "did anyone change anything?" in Slack.

**The dogwatch solution:** Automatically track all changes. Correlate them with incidents by timing. Surface "incident started 4 minutes after deploy X" without anyone asking.

```
┌─────────────────────────────────────────────────────────────┐
│  🔄 Change Correlation                                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Incident: API latency spike (started 14:32)                │
│                                                              │
│  Changes in 30-minute window before incident:               │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  🎯 HIGH CORRELATION (94% confidence)               │    │
│  │                                                      │    │
│  │  14:28 Deploy: api-service v2.4.1 → v2.4.2         │    │
│  │         ├── Commit: "Optimize search query"         │    │
│  │         ├── Author: alice@company.com               │    │
│  │         ├── Changed files:                          │    │
│  │         │   └── src/search/query.go (+847, -23)    │    │
│  │         └── Incident started 4 min after deploy    │    │
│  │                                                      │    │
│  │  [View diff] [View commit] [Rollback] [Page alice] │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  ⚪ LOW CORRELATION                                  │    │
│  │                                                      │    │
│  │  14:15 Config: Updated rate limits (bob)           │    │
│  │         └── 17 min before incident, different svc  │    │
│  │                                                      │    │
│  │  13:45 Feature flag: Enabled dark mode (carol)     │    │
│  │         └── 47 min before, UI only                 │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  No changes detected:                                       │
│  ├── Infrastructure (Terraform)                             │
│  ├── Database migrations                                    │
│  └── Kubernetes manifests                                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**What we track:**

| Change Type | Source | Data Captured |
|-------------|--------|---------------|
| **Deploys** | K8s, Docker, CI/CD webhooks | Image, commit SHA, author, diff |
| **Commits** | GitHub/GitLab webhooks | Files changed, author, message |
| **Config changes** | ConfigMaps, Consul, etcd | Key, old value, new value, who |
| **Feature flags** | LaunchDarkly, Split, custom | Flag, old state, new state |
| **DB migrations** | Flyway, Liquibase, custom | Migration name, tables affected |
| **Infra changes** | Terraform, Pulumi | Resources changed |

**Correlation scoring:**

```go
type ChangeCorrelation struct {
    Change      Change
    Incident    Incident
    Score       float64   // 0-100 confidence
    Factors     []string  // why we think it's related
}

func (c *Correlator) ScoreChange(change Change, incident Incident) float64 {
    score := 0.0

    // Time proximity (closer = higher score)
    timeDelta := incident.StartedAt.Sub(change.Timestamp)
    if timeDelta > 0 && timeDelta < 30*time.Minute {
        score += 40 * (1 - timeDelta.Minutes()/30)
    }

    // Service match (same service = higher score)
    if change.Service == incident.Service {
        score += 30
    }

    // Historical pattern (this change type caused incidents before)
    if c.hasHistoricalCorrelation(change.Type, incident.Type) {
        score += 20
    }

    // Change size (bigger changes = more likely to cause issues)
    if change.LinesChanged > 100 {
        score += 10
    }

    return score
}
```

**Auto-actions:**

- **High correlation detected** → Auto-add to incident timeline
- **Deploy + incident within 5 min** → Suggest rollback button
- **Same change caused incidents 3x** → Flag as high-risk change
- **Rollback fixes it** → Record as confirmed root cause

**Why this matters:**

"What changed?" is the first question in every incident. Auto-answering it saves 10-20 minutes per incident. At 50 incidents/month, that's 8-16 hours saved.

**The integration story:** This requires git/deploy webhooks, but it's worth it. Teams that connect deploys to incidents see 40% faster MTTR.

---

### 6. Security Observability (eBPF Threat Detection)

**The problem:** Security and observability are separate tools, separate teams, separate budgets. But the data overlaps - unusual processes, unexpected network connections, suspicious file access. Running Falco + Datadog + SIEM is expensive and fragmented.

**The dogwatch solution:** Since we're already in the kernel via eBPF, add security detection for free. One binary does observability AND runtime security.

```
┌─────────────────────────────────────────────────────────────┐
│  🛡️ Security Events (last 24h)                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  🔴 CRITICAL: Shell spawned in container                    │
│     Time: 14:32:07                                          │
│     Container: api-service-7d4f8b (pod api-service-abc123) │
│     Process: /bin/sh (PID 4847)                            │
│     Parent: python /app/main.py                            │
│     ⚠️ Unusual: This container has never spawned a shell   │
│                                                              │
│     [View process tree] [Kill container] [Create incident]  │
│                                                              │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  🟡 WARNING: Unusual outbound connection                    │
│     Time: 14:28:15                                          │
│     Source: api-service-7d4f8b                             │
│     Destination: 185.234.72.x:4444 (unknown, RU)           │
│     ⚠️ First connection to this IP from any service        │
│                                                              │
│     [View connection history] [Block IP] [Investigate]      │
│                                                              │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  🟡 WARNING: Sensitive file access                          │
│     Time: 13:45:22                                          │
│     File: /etc/shadow                                       │
│     Process: mystery-binary (PID 8923)                     │
│     ⚠️ Unknown binary accessing password file              │
│                                                              │
│     [View file access history] [Kill process] [Alert]       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**What we detect:**

| Threat | Detection Method | eBPF Hook |
|--------|------------------|-----------|
| **Shell in container** | Unexpected /bin/sh, /bin/bash execution | `sys_execve` |
| **Cryptominer** | Known miner processes, pool connections | `sys_execve`, `tcp_connect` |
| **Container escape** | Access to host namespaces, /proc/1 | `sys_setns`, file access |
| **Privilege escalation** | setuid calls, capability changes | `sys_setuid`, `cap_capable` |
| **Sensitive file access** | /etc/shadow, /etc/passwd, SSH keys | `sys_open`, `sys_read` |
| **Unusual network** | Connections to new IPs, known bad IPs | `tcp_connect` |
| **Reverse shell** | Shell with stdin/stdout to socket | `sys_dup2`, `sys_connect` |
| **Data exfiltration** | Large outbound transfers to new destinations | `tcp_sendmsg` |

**Implementation:**

```go
// Security event types
type SecurityEvent struct {
    Timestamp   time.Time
    Severity    Severity  // CRITICAL, WARNING, INFO
    Type        string    // shell_spawn, unusual_network, file_access, etc.
    Container   string
    Pod         string
    Process     ProcessInfo
    Details     map[string]interface{}
    Anomaly     AnomalyInfo  // why this is unusual
}

// Baseline learning - know what's "normal" for each container
type ContainerBaseline struct {
    ContainerImage  string
    NormalProcesses map[string]bool      // processes that normally run
    NormalPorts     map[int]bool         // ports that normally listen
    NormalOutbound  map[string]bool      // IPs/domains normally contacted
    NormalFiles     map[string]bool      // files normally accessed
    LearnedAt       time.Time
}

// Detection rules
type SecurityRule struct {
    Name        string
    Description string
    Severity    Severity
    Condition   func(event *eBPFEvent, baseline *ContainerBaseline) bool
    Message     func(event *eBPFEvent) string
}

var DefaultRules = []SecurityRule{
    {
        Name:        "shell_in_container",
        Description: "Shell spawned in container",
        Severity:    CRITICAL,
        Condition: func(e *eBPFEvent, b *ContainerBaseline) bool {
            return e.Type == "execve" &&
                   isShell(e.Comm) &&
                   e.InContainer &&
                   !b.NormalProcesses[e.Comm]
        },
    },
    {
        Name:        "unusual_outbound",
        Description: "Connection to unknown destination",
        Severity:    WARNING,
        Condition: func(e *eBPFEvent, b *ContainerBaseline) bool {
            return e.Type == "tcp_connect" &&
                   !b.NormalOutbound[e.DstIP] &&
                   !isInternalIP(e.DstIP)
        },
    },
    // ... more rules
}
```

**eBPF probes to add:**

```
bpf/
├── security/
│   ├── execve.c          # Process execution monitoring
│   ├── file_access.c     # Sensitive file tracking
│   ├── network.c         # Unusual connection detection
│   ├── privilege.c       # Privilege escalation detection
│   └── container.c       # Container escape detection
```

**Why this matters:**

1. **Bigger budgets** - Security spend > observability spend at most companies
2. **Unique positioning** - No one else does lightweight observability + security in one binary
3. **Natural extension** - We're already in the kernel, security is incremental
4. **Convergence trend** - Palo Alto bought Chronosphere; Cisco bought Splunk; security + observability are merging

**The competitive angle:**
- Falco/Tetragon do security but not observability
- Datadog does both but costs $$$$ and is SaaS
- dogwatch: both, lightweight, self-hosted, free

---

### 7. Migration Assistant (Zero-Friction Switching)

**The problem:** Switching costs kill adoption. Teams have years of dashboards, alerts, and SLOs in Datadog. Even if dogwatch is better, rebuilding everything is months of work.

**The dogwatch solution:** Import existing configurations. Show exactly what migrates, what needs adjustment, and what the savings will be.

```
┌─────────────────────────────────────────────────────────────┐
│  🔄 Migration Assistant                                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Import from: [Datadog ▼]                                   │
│                                                              │
│  Paste your Datadog API key: [dd-api-**********]           │
│                                                              │
│  [Scan Account]                                             │
│                                                              │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  📊 Scan Results                                            │
│                                                              │
│  Dashboards:                                                │
│  ├── ✅ 18 fully compatible                                │
│  ├── ⚠️  4 need minor adjustments (unsupported widgets)    │
│  └── ❌ 1 incompatible (APM-specific)                      │
│                                                              │
│  Alerts/Monitors:                                           │
│  ├── ✅ 42 fully compatible                                │
│  ├── ⚠️  8 need metric name mapping                        │
│  └── ❌ 3 use Datadog-specific features                    │
│                                                              │
│  SLOs:                                                      │
│  ├── ✅ 12 fully compatible                                │
│  └── ⚠️  2 need data source adjustment                     │
│                                                              │
│  Integrations:                                              │
│  ├── ✅ 8 supported (AWS, K8s, Docker, Postgres...)       │
│  ├── 🔧 4 partial (Slack, PagerDuty)                       │
│  └── ❌ 6 not yet supported (Snowflake, MongoDB...)        │
│                                                              │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  💰 Cost Analysis                                           │
│                                                              │
│  Your current Datadog bill: ~$47,000/month                 │
│  ├── Infrastructure (847 hosts): $28,000                   │
│  ├── APM (312 hosts): $12,000                              │
│  ├── Logs (2.4TB): $4,000                                  │
│  └── Synthetics + other: $3,000                            │
│                                                              │
│  dogwatch equivalent: $0 (self-hosted)                     │
│  Annual savings: $564,000                                   │
│                                                              │
│  ─────────────────────────────────────────────────────────  │
│                                                              │
│  [Import All Compatible] [Review Details] [Export Report]   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**What we import:**

| Source | What We Import | Compatibility |
|--------|---------------|---------------|
| **Datadog** | Dashboards, monitors, SLOs, notebooks | High |
| **Grafana** | Dashboards (JSON), alert rules | High |
| **Prometheus** | Alert rules, recording rules | High |
| **PagerDuty** | Escalation policies, schedules | Medium |
| **New Relic** | Dashboards, alerts, SLOs | Medium |

**Implementation:**

```go
type MigrationAssistant struct {
    Source      string  // "datadog", "grafana", "prometheus"
    Credentials map[string]string
}

type MigrationReport struct {
    Dashboards    []MigrationItem
    Alerts        []MigrationItem
    SLOs          []MigrationItem
    Integrations  []MigrationItem
    CostAnalysis  CostComparison

    TotalItems    int
    Compatible    int
    NeedsReview   int
    Incompatible  int
}

type MigrationItem struct {
    Name          string
    Type          string
    Status        MigrationStatus  // COMPATIBLE, NEEDS_REVIEW, INCOMPATIBLE
    Issues        []string         // what needs adjustment
    Imported      bool
}

// Datadog dashboard converter
func (m *MigrationAssistant) ImportDatadogDashboard(dd DatadogDashboard) (*Dashboard, []string) {
    dashboard := &Dashboard{
        Name:   dd.Title,
        Widgets: []Widget{},
    }

    issues := []string{}

    for _, widget := range dd.Widgets {
        converted, err := convertDatadogWidget(widget)
        if err != nil {
            issues = append(issues, fmt.Sprintf("Widget '%s': %v", widget.Title, err))
            continue
        }
        dashboard.Widgets = append(dashboard.Widgets, converted)
    }

    return dashboard, issues
}

// Query translation: Datadog → dogwatch
func translateDatadogQuery(ddQuery string) (string, error) {
    // avg:system.cpu.user{host:web-*} by {host}
    // → avg(system_cpu_user{host=~"web-.*"}) by (host)

    // Parse Datadog query syntax
    // Convert to PromQL-compatible syntax
    // Return translated query
}
```

**The migration flow:**

```
1. Connect to source (API key)
         │
         ▼
2. Scan all resources
         │
         ▼
3. Generate compatibility report
         │
         ▼
4. User reviews & adjusts
         │
         ▼
5. One-click import
         │
         ▼
6. Verify everything works
         │
         ▼
7. Show savings achieved
```

**Why this matters:**

Migration friction is the #1 reason people stay with expensive tools. By showing:
- Exactly what migrates
- Exactly what needs work
- Exactly how much they'll save

...you remove the fear of switching. The $564K/year savings number does the selling.

---

## Business Model & Pricing

### What Each Feature Is Worth

Based on competitor pricing and value delivered:

#### Zero-Config Tracing (eBPF Magic)

**Competitor pricing:**
- Datadog APM: $31-40/host/month
- New Relic APM: $0.25/GB ingested
- Pixie (before acquisition): Free, but now requires New Relic

**Our value:**
- No SDK integration (saves weeks of engineering time)
- No per-host pricing trap
- Works immediately

| What We Charge | Justification |
|----------------|---------------|
| **Free** | This is the hook. The magic that gets people in the door. |

**Why free:** If we charge for the core differentiator, we're just another APM. The zero-config magic is marketing.

---

#### Protocol Parsing (MySQL, PostgreSQL, Redis, etc.)

**Competitor pricing:**
- Datadog Database Monitoring: $70/host/month (on top of APM!)
- Most APMs: Require manual instrumentation

**Our value:**
- See every query without touching code
- Automatic slow query detection
- Query → trace correlation

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Part of zero-config magic. Core differentiator. |

---

#### Pre-Built Scripts

**Competitor pricing:**
- Pixie: Free (scripts were open source)
- Datadog Notebooks: Included in paid plans

**Our value:**
- Instant answers: `dogwatch run mysql/slow_queries`
- No dashboard building required
- Community contributions

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Core UX. Makes the product sticky. |

---

#### Continuous Profiling

**Competitor pricing:**
- Datadog Continuous Profiler: $12/host/month
- Pyroscope Cloud: $0.10/million profiles
- Parca: Free (open source)

**Our value:**
- CPU flamegraphs without code changes
- Profile → trace linking
- Always-on, low overhead

| What We Charge | Justification |
|----------------|---------------|
| **Free** (basic) | Differentiator if built-in |
| **$19/month** (Team) | Extended retention, advanced features |

---

#### Cost Intelligence

**Competitor pricing:**
- No direct competitor (Datadog won't show you how expensive they are!)
- Cloud cost tools (Vantage, CloudHealth): $500-5000/month

**Our value:**
- "You'd pay $47K/month on Datadog"
- Viral screenshots for marketing
- CFO-friendly justification

| What We Charge | Justification |
|----------------|---------------|
| **Free** | This is MARKETING. Every screenshot sells dogwatch. |

**This feature should be free forever.** It's an ad for the product.

---

#### Control Plane (Usage Analytics, Cardinality, Shaping)

**Competitor pricing:**
- Chronosphere: $$$$ (enterprise only, custom pricing)
- Estimated: $0.05-0.15 per million active series
- Typical customer: $10K-100K/month

**Our value:**
- "80% of your metrics are never queried"
- "Drop this label, save $18K/month"
- Prevents bill shock

| Tier | What's Included | Price |
|------|-----------------|-------|
| **Free** | Usage analytics (read-only), basic cardinality view | $0 |
| **Team** | Full cardinality explorer, cost attribution | $99/mo |
| **Business** | Shaping rules, recommendations, impact preview | $299/mo |
| **Enterprise** | Team quotas, advanced governance | Custom |

**This is the Chronosphere playbook.** Free to see the problem, pay to fix it.

---

#### BubbleUp (Automatic Root Cause)

**Competitor pricing:**
- Honeycomb: $100-500/month for teams, $1000+/month enterprise
- Honeycomb charges based on events ingested

**Our value:**
- "98% of slow requests hit shard 7"
- Saves hours of manual investigation
- Makes anyone look like a debugging expert

| What We Charge | Justification |
|----------------|---------------|
| **Free** (basic) | Limited to last 1 hour, 3 analyses/day |
| **Team $99/mo** | Unlimited analyses, 7-day lookback |
| **Business $299/mo** | 30-day lookback, saved analyses, sharing |

---

#### Change Correlation

**Competitor pricing:**
- Usually bundled with incident management
- Datadog: Included in APM
- Lightstep: Included

**Our value:**
- "Incident started 4 min after deploy v2.4.1"
- Automatic, no manual annotation
- Rollback suggestions

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Basic correlation, annotations on graphs |
| **Team $99/mo** | GitHub/GitLab integration, incident timeline |

---

#### Security Observability

**Competitor pricing:**
- Falco: Free (open source)
- Sysdig Secure: $25-50/host/month
- Datadog Security: $18/host/month (on top of infrastructure!)

**Our value:**
- Shell-in-container detection
- Unusual network connections
- No separate tool to deploy

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Basic detection (shell, crypto miner) |
| **Business $299/mo** | Full security rules, baseline learning, compliance |
| **Enterprise** | SIEM integration, audit logs, custom rules |

**Security sells to different buyers** (CISO vs SRE). Good upsell path.

---

#### Migration Assistant

**Competitor pricing:**
- Professional services: $10K-50K for migration projects
- Grafana Migration Service: Custom pricing

**Our value:**
- Scan Datadog account automatically
- "23 dashboards compatible, $564K/year savings"
- Reduces fear of switching

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Scan + compatibility report + savings estimate |
| **Business $299/mo** | Actual import of dashboards/alerts |
| **Enterprise** | White-glove migration service ($5K-20K one-time) |

**The scan should be free.** It's a sales tool.

---

#### SSO / SAML / OIDC

**Competitor pricing:**
- This is the classic "enterprise tax"
- Grafana Cloud: Free tier has no SSO, paid tiers include it
- Most SaaS: SSO only in enterprise tier

**Our value:**
- Required for enterprises
- Security/compliance checkbox

| What We Charge | Justification |
|----------------|---------------|
| **Team $99/mo** | OIDC (Google, GitHub, Okta via OIDC) |
| **Business $299/mo** | Full SAML 2.0, SCIM provisioning |

---

#### Advanced RBAC

**Competitor pricing:**
- Usually enterprise-only
- Part of "governance" packages

**Our value:**
- Team A can't see Team B's data
- Read-only roles for contractors
- Fine-grained permissions

| What We Charge | Justification |
|----------------|---------------|
| **Free** | Basic roles (admin, user, viewer) |
| **Business $299/mo** | Custom roles, resource-level permissions |
| **Enterprise** | Attribute-based access control |

---

#### Multi-Tenancy

**Competitor pricing:**
- Enterprise-only feature everywhere
- Required for MSPs, platform teams

**Our value:**
- Isolated organizations
- Per-tenant quotas
- Cross-tenant queries for admins

| What We Charge | Justification |
|----------------|---------------|
| **Enterprise only** | Custom pricing based on tenant count |

---

#### Support & SLA

**Competitor pricing:**
- Datadog: Support included, but response times vary by tier
- Grafana Enterprise: $$$
- Typical: $500-5000/month for priority support

**Our value:**
- Faster response times
- Direct access to engineers
- Guaranteed uptime

| Tier | Response Time | Price |
|------|---------------|-------|
| **Community** | Best effort (GitHub/Discord) | Free |
| **Team** | 24-hour response, business hours | $99/mo |
| **Business** | 4-hour response, extended hours | $299/mo |
| **Enterprise** | 1-hour response, 24/7, dedicated Slack | Custom ($1K+/mo) |

---

### Complete Pricing Grid

| Feature | Free | Team $99/mo | Business $299/mo | Enterprise |
|---------|------|-------------|------------------|------------|
| **eBPF Tracing** | ✅ Unlimited | ✅ | ✅ | ✅ |
| **Protocol Parsing** | ✅ All | ✅ | ✅ | ✅ |
| **Pre-Built Scripts** | ✅ All | ✅ | ✅ | ✅ |
| **Cost Intelligence** | ✅ Full | ✅ | ✅ | ✅ |
| **Dashboards** | ✅ 5 | ✅ Unlimited | ✅ | ✅ |
| **Alerts** | ✅ 10 | ✅ Unlimited | ✅ | ✅ |
| **Retention** | 7 days | 30 days | 90 days | Custom |
| **Users** | 3 | 10 | Unlimited | Unlimited |
| **Profiling** | ✅ Basic | ✅ Extended | ✅ Full | ✅ |
| **BubbleUp** | 3/day, 1hr | Unlimited, 7d | Unlimited, 30d | Unlimited |
| **Change Correlation** | ✅ Basic | ✅ + Git integration | ✅ | ✅ |
| **Control Plane** | Read-only | Full explorer | + Shaping rules | + Quotas |
| **Security** | Basic alerts | ✅ | Full + baseline | + Compliance |
| **Migration Scan** | ✅ | ✅ | + Import | + White-glove |
| **SSO** | ❌ | OIDC | SAML + SCIM | ✅ |
| **RBAC** | Basic | Basic | Advanced | Full ABAC |
| **Multi-tenant** | ❌ | ❌ | ❌ | ✅ |
| **Support** | Community | 24hr email | 4hr + Slack | 1hr + dedicated |
| **SLA** | ❌ | ❌ | 99.9% | 99.99% |

---

### Why This Pricing Works

**Free tier is genuinely useful:**
- All core features work
- No artificial limits on data
- Enough to run a small startup

**$99/mo Team tier ($1,188/year):**
- Longer retention (30 days)
- More users
- SSO (security checkbox)
- Git integration
- Compared to: Datadog at $400+/month for same coverage

**$299/mo Business tier ($3,588/year):**
- Control plane with shaping (save 70% on data)
- Full security
- Dashboard/alert import
- Compared to: Chronosphere at $10K+/month

**Enterprise (custom, $1K-10K/mo):**
- Multi-tenancy
- Compliance features
- Dedicated support
- Compared to: Datadog/Chronosphere at $50K+/month

---

### Revenue Projections

| Customers | Mix | MRR | ARR |
|-----------|-----|-----|-----|
| 50 Team | 100% Team | $4,950 | $59K |
| 40 Team + 10 Business | 80/20 | $6,950 | $83K |
| 30 Team + 15 Business + 5 Enterprise | 50/25/25 | $12,420 | $149K |
| 50 Team + 30 Business + 20 Enterprise | Mature | $29,850 | $358K |

**Path to $20K MRR:**
- 100 Team customers, or
- 50 Team + 25 Business, or
- 40 Team + 15 Business + 10 Enterprise

---

### What Competitors Charge (Reference)

| Tool | Pricing Model | Typical Cost (100 hosts) |
|------|---------------|-------------------------|
| **Datadog** | Per-host + usage | $15K-40K/month |
| **New Relic** | Per-GB ingested | $5K-20K/month |
| **Splunk** | Per-GB indexed | $10K-50K/month |
| **Grafana Cloud** | Per-series + GB | $2K-10K/month |
| **Chronosphere** | Per-series | $10K-100K/month |
| **Honeycomb** | Per-event | $2K-10K/month |
| **dogwatch** | Flat rate | $0-299/month |

**Our positioning:** Flat rate vs usage-based. No bill shock. No per-host trap.

---

### Pricing Philosophy

1. **Core magic is free** - eBPF tracing, cost intelligence, scripts
2. **Pay for depth** - More retention, more analysis, more governance
3. **Pay for enterprise** - SSO, RBAC, multi-tenant, support
4. **Never per-host** - This is our moat vs Datadog
5. **Never per-GB** - This is our moat vs everyone

**The pitch:** "Unlimited hosts, unlimited data, flat rate."

**What's always free:**
- All seven killer features (tracing, cost intel, control plane, BubbleUp, change correlation, security, migration)
- Unlimited hosts, metrics, traces, logs
- Self-hosted deployment
- Community support (GitHub, Discord)

**What's paid:**
- **Support SLA** - Guaranteed response times
- **SSO/SAML** - Enterprise auth
- **Advanced RBAC** - Fine-grained permissions
- **Migration service** - White-glove migration help
- **Training** - Team onboarding sessions
- **Custom integrations** - Build integrations for your stack

**Why this model:**

1. **Free tier is genuinely useful** - Not crippled, not time-limited. This drives adoption.
2. **Paid tier is support + enterprise features** - Things big companies need and will pay for.
3. **No per-host pricing** - The anti-Datadog positioning. "Unlimited hosts" is the headline.
4. **Self-hosted = no infrastructure costs** - Margins are high because we're not running their data.

**The competitive positioning:**

```
                    │ Datadog    │ Grafana Cloud │ dogwatch
────────────────────┼────────────┼───────────────┼──────────
100 hosts, full APM │ $4,000/mo  │ $2,000/mo     │ $0-99/mo
500 hosts, full APM │ $20,000/mo │ $8,000/mo     │ $0-99/mo
1000 hosts          │ $40,000/mo │ $15,000/mo    │ $0-299/mo
────────────────────┼────────────┼───────────────┼──────────
Self-hosted option  │ No         │ Yes           │ Yes
No per-host pricing │ No         │ No            │ Yes
eBPF auto-tracing   │ No         │ No            │ Yes
```

**Revenue targets:**

| Milestone | Timeline | How |
|-----------|----------|-----|
| $5K MRR | Month 6 | 50 Team customers |
| $20K MRR | Month 12 | 50 Team + 20 Business |
| $50K MRR | Month 18 | Mix + first Enterprise |
| $100K MRR | Month 24 | Ready for seed round or acquisition |

**The acquisition math:**

At $100K MRR ($1.2M ARR), typical SaaS multiples are 5-10x for growing companies. That's $6-12M valuation. With strong growth metrics and unique eBPF tech, could be higher.

Chronosphere: $160M ARR → $3.35B acquisition (21x multiple)
Smaller exit: $1.2M ARR → $6-12M acquisition (5-10x multiple)

---

## Positioning

### Tagline Options

1. **"See everything. Pay nothing."** - Emphasizes auto-instrumentation + self-hosted
2. **"Datadog visibility without the Datadog bill"** - Direct competitive positioning
3. **"Observability that installs in 60 seconds"** - Emphasizes simplicity

### Target Customer

**Primary:** Engineering teams at companies with 20-500 employees
- Running on Kubernetes or Linux VMs
- Feeling Datadog/New Relic pricing pain
- Want observability but can't justify $50K+/year
- Have ops capacity to run self-hosted software

**Secondary:** Platform teams at larger companies
- Looking for on-prem/air-gapped observability
- Data sovereignty requirements
- Want to standardize on one tool

### Competitive Positioning

| vs. Datadog | dogwatch |
|-------------|----------|
| $15-40/host/month | Self-hosted: $0 |
| Requires SDK integration | Zero-config eBPF |
| Data in Datadog's cloud | Data stays on your infra |
| 600+ integrations | Focused on what matters |
| Complex pricing | Simple pricing |

| vs. Grafana Stack | dogwatch |
|-------------------|----------|
| 5+ components to deploy | Single binary |
| Prometheus + Loki + Tempo + Grafana | All-in-one |
| Steep learning curve | Works out of the box |
| Manual correlation | Automatic correlation |

| vs. Pixie/New Relic | dogwatch |
|---------------------|----------|
| SaaS only | Self-hosted |
| New Relic pricing | Free or flat rate |
| Closed source | Open core |

---

## Architecture

### Single Binary Philosophy

Everything in one ~70MB binary:
- eBPF probes (compiled in)
- Web UI (embedded)
- SQLite storage
- No external dependencies

```
dogwatch binary
├── eBPF probes
│   ├── tcp.c      → Connection tracking
│   ├── http.c     → HTTP/1.1 parsing
│   ├── ssl.c      → TLS interception
│   ├── mysql.c    → MySQL protocol
│   ├── pgsql.c    → PostgreSQL protocol
│   ├── redis.c    → Redis protocol
│   └── profile.c  → CPU profiling
├── Aggregator     → In-memory metrics aggregation
├── Storage        → SQLite databases
├── Web UI         → Embedded React app
├── API Server     → REST + WebSocket
└── Federation     → Optional multi-node gossip
```

### Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│                     Your Infrastructure                       │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │Service A│  │Service B│  │ MySQL   │  │ Redis   │        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
│       │            │            │            │               │
│       └────────────┴────────────┴────────────┘               │
│                           │                                  │
│                    Linux Kernel                              │
│              ┌────────────┴────────────┐                     │
│              │      eBPF Probes        │                     │
│              │  (tcp, http, mysql...)  │                     │
│              └────────────┬────────────┘                     │
└───────────────────────────┼──────────────────────────────────┘
                            │ Ring buffer
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                       dogwatch                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                   Trace Assembler                     │   │
│  │  • Correlate requests across services                 │   │
│  │  • Parse trace headers (W3C, B3, X-Request-ID)       │   │
│  │  • Build distributed traces from eBPF events         │   │
│  └──────────────────────────────────────────────────────┘   │
│                            │                                 │
│  ┌─────────────┬───────────┴───────────┬─────────────┐     │
│  │   Traces    │       Metrics         │    Logs     │     │
│  │  traces.db  │      metrics.db       │   logs.db   │     │
│  └─────────────┴───────────────────────┴─────────────┘     │
│                            │                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Cost Intelligence Engine                 │   │
│  │  • Track usage (hosts, metrics, logs, spans)         │   │
│  │  • Calculate competitor pricing                       │   │
│  │  • Generate savings reports                           │   │
│  └──────────────────────────────────────────────────────┘   │
│                            │                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                   Control Plane                       │   │
│  │  • Cardinality analysis and alerts                   │   │
│  │  • Usage tracking (what's queried, what's not)       │   │
│  │  • Shaping rules (aggregate, drop, downsample)       │   │
│  │  • Recommendations engine                             │   │
│  └──────────────────────────────────────────────────────┘   │
│                            │                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                Security Detection                     │   │
│  │  • Container baseline learning                       │   │
│  │  • Anomaly detection (processes, network, files)     │   │
│  │  • Threat alerting (shell, cryptominer, exfil)       │   │
│  └──────────────────────────────────────────────────────┘   │
│                            │                                 │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                Migration Assistant                    │   │
│  │  • Import from Datadog, Grafana, Prometheus          │   │
│  │  • Query translation                                  │   │
│  │  • Cost comparison and savings calculator            │   │
│  └──────────────────────────────────────────────────────┘   │
│                            │                                 │
│                     Web UI (:9999)                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Technical Moats

These are the genuinely hard engineering challenges that create defensibility. Unlike features that can be copied in a weekend, these require months of painful iteration and real-world feedback. **This is where the engineering investment pays off.**

### Why Technical Depth Matters

| "Moat" | Reality | Defensibility |
|--------|---------|---------------|
| Cost calculator | Arithmetic with public pricing | ❌ None - anyone can copy |
| Single binary | Architecture choice | ❌ Low - just a build decision |
| Basic HTTP parsing | Documented protocol | ❌ Low - examples everywhere |
| Pretty dashboards | UI/UX work | ❌ Low - table stakes |
| **Kernel compatibility** | Months of testing, edge cases | ✅ High |
| **TLS interception** | Library-specific, breaks constantly | ✅ High |
| **Protocol edge cases** | Only found in production | ✅ High |
| **Correlation accuracy** | Probabilistic, needs tuning | ✅ High |
| **Migration fidelity** | Experience-based, 100 edge cases | ✅ High |

---

### 1. Kernel Version Compatibility Matrix

**The nightmare:**
```
Kernel 4.14    (old RHEL/CentOS, still in production)
Kernel 4.19    (Debian 10)
Kernel 5.4     (Ubuntu 20.04 LTS - huge install base)
Kernel 5.10    (Debian 11, Amazon Linux 2022)
Kernel 5.15    (Ubuntu 22.04 LTS)
Kernel 6.1     (Debian 12, latest LTS)
Kernel 6.5+    (Ubuntu 23.10+, bleeding edge)

Each has:
├── Different eBPF features available
├── Different verifier behavior (what passes/fails)
├── Different helper functions
├── Different struct layouts (task_struct changes!)
├── Different BTF availability
└── Different performance characteristics
```

**Why it's hard:**
- eBPF verifier rejects code that works on other kernels
- Struct field offsets change between versions
- Some features (ringbuf, BTF, CO-RE) only exist on newer kernels
- Can't just "test on CI" - need actual kernel diversity
- Users won't tell you their kernel version, just "it doesn't work"

**The moat:** An eBPF program that works reliably across all of these is months of painful testing. Most competitors pick 1-2 kernels and hope for the best.

**Implementation strategy:**

```go
// Build-time: Compile multiple variants
//go:embed bpf/probe_v4.14.o
//go:embed bpf/probe_v5.4.o
//go:embed bpf/probe_v5.10.o
//go:embed bpf/probe_v6.1.o

// Runtime: Detect and load appropriate variant
func loadProbe() (*ebpf.Collection, error) {
    version := detectKernelVersion()

    switch {
    case version.AtLeast(6, 1):
        return loadProbeV61()  // Full features, ringbuf, BTF
    case version.AtLeast(5, 10):
        return loadProbeV510() // CO-RE, most features
    case version.AtLeast(5, 4):
        return loadProbeV54()  // Reduced features
    case version.AtLeast(4, 14):
        return loadProbeV414() // Minimal, perf buffer
    default:
        return nil, ErrKernelTooOld
    }
}

// Graceful degradation
type ProbeCapabilities struct {
    HasRingbuf       bool  // 5.8+
    HasBTF           bool  // 5.2+ with config
    HasCORE          bool  // 5.2+
    HasKprobeMulti   bool  // 5.18+
    HasFentry        bool  // 5.5+
    MaxInstructions  int   // Varies by version
}
```

**Testing matrix (CI requirement):**

| Distro | Kernel | Priority | Notes |
|--------|--------|----------|-------|
| Ubuntu 20.04 | 5.4 | P0 | Huge install base |
| Ubuntu 22.04 | 5.15 | P0 | Current LTS |
| Debian 11 | 5.10 | P0 | Server standard |
| Amazon Linux 2023 | 6.1 | P0 | AWS default |
| RHEL 8 | 4.18 | P1 | Enterprise |
| RHEL 9 | 5.14 | P1 | Enterprise |
| Debian 12 | 6.1 | P1 | Latest stable |
| Fedora (latest) | 6.x | P2 | Bleeding edge testing |

**Effort estimate:** 4-6 weeks for comprehensive coverage + ongoing maintenance

**Deliverable:** `dogwatch doctor` command that reports kernel compatibility, missing features, and recommended actions.

---

### 2. TLS Interception Depth

**The nightmare:**
```
OpenSSL 1.1.x     (Ubuntu 20.04, older systems)
├── SSL_read/SSL_write symbols
├── Specific struct layouts
└── Different than 3.x!

OpenSSL 3.x       (Ubuntu 22.04+, modern systems)
├── New API, new symbols
├── Providers architecture
├── struct ssl_st layout changed!
└── BIO layer changes

BoringSSL         (Google's fork)
├── Used by: Go, Chrome, Android, gRPC
├── Different symbols entirely
├── Statically linked usually
└── Version detection is hard

LibreSSL          (OpenBSD's fork)
├── Used by: OpenBSD, some Alpine
├── API-compatible but different internals
└── Less common but exists

Go crypto/tls     (Go runtime)
├── Not a library - compiled into binary
├── Need to hook runtime functions
├── crypto/tls.(*Conn).Read
├── Offsets depend on Go version!
└── Go 1.20 vs 1.21 vs 1.22 differ

Java JSSE         (JVM TLS)
├── Need to hook JNI or Java methods
├── JVM version dependent
├── GraalVM native different again
└── Usually punt on this

Node.js           (V8 + OpenSSL)
├── Sometimes statically linked
├── Sometimes system OpenSSL
└── Need to detect which

Rust rustls       (Pure Rust TLS)
├── No OpenSSL at all
├── Need to hook Rust internals
└── Binary has no standard symbols
```

**Why it's hard:**
- Symbol names differ between libraries
- Struct layouts differ (can't just cast pointers)
- Static linking hides symbols
- Version detection requires reading binary headers
- One wrong offset = garbage data or crash
- Libraries update, breaking your probes

**Implementation strategy:**

```c
// Multiple uprobe targets for different libraries

// OpenSSL 1.1.x
SEC("uprobe/openssl_1_1_read")
int uprobe_ssl_read_1_1(struct pt_regs *ctx) {
    // OpenSSL 1.1.x struct layout
    struct ssl_st_1_1 *ssl = (void *)PT_REGS_PARM1(ctx);
    void *buf = (void *)PT_REGS_PARM2(ctx);
    // ... extract data
}

// OpenSSL 3.x
SEC("uprobe/openssl_3_read")
int uprobe_ssl_read_3(struct pt_regs *ctx) {
    // OpenSSL 3.x struct layout - DIFFERENT!
    struct ssl_st_3 *ssl = (void *)PT_REGS_PARM1(ctx);
    void *buf = (void *)PT_REGS_PARM2(ctx);
    // ... extract data
}

// Go crypto/tls
SEC("uprobe/go_tls_read")
int uprobe_go_tls_read(struct pt_regs *ctx) {
    // Go calling convention, different registers
    // Need to handle Go stack layout
}
```

```go
// Runtime library detection
type TLSLibrary struct {
    Type     string  // "openssl", "boringssl", "go", etc.
    Version  string  // "1.1.1", "3.0.2", etc.
    Path     string  // /usr/lib/libssl.so.1.1
    Symbols  map[string]uint64
}

func detectTLSLibraries(pid int) ([]TLSLibrary, error) {
    // 1. Read /proc/pid/maps
    // 2. Find loaded libraries
    // 3. Parse ELF headers for symbols
    // 4. Match against known signatures
    // 5. Return detected libraries with versions
}

func attachTLSProbes(lib TLSLibrary) error {
    switch lib.Type {
    case "openssl":
        if lib.Version.Major == 1 {
            return attachOpenSSL1Probes(lib)
        }
        return attachOpenSSL3Probes(lib)
    case "boringssl":
        return attachBoringSSlProbes(lib)
    case "go":
        return attachGoTLSProbes(lib)
    default:
        return ErrUnsupportedTLS
    }
}
```

**Coverage targets:**

| Library | Version | Priority | Coverage |
|---------|---------|----------|----------|
| OpenSSL | 1.1.x | P0 | 40% of traffic |
| OpenSSL | 3.x | P0 | 30% of traffic |
| Go crypto/tls | 1.20+ | P0 | 20% of traffic (growing) |
| BoringSSL | latest | P1 | gRPC, some apps |
| LibreSSL | 3.x | P2 | Rare |
| Java JSSE | 11+ | P2 | Enterprise apps |
| rustls | latest | P3 | Growing but rare |

**Effort estimate:** 6-8 weeks for OpenSSL + Go, 3-4 weeks per additional library

**The moat:** This is genuinely painful. Pixie had dedicated engineers on this. Getting it working across all variants requires constant maintenance as libraries update.

---

### 3. Protocol Edge Cases

**Basic parsing is easy. Production parsing is brutal.**

#### MySQL Protocol Deep Dive

```
Easy (what tutorials show):
  COM_QUERY: "SELECT * FROM users WHERE id = 1"
  → Parse packet, extract SQL string, done

Hard (what production sends):

Prepared statements (binary protocol):
  COM_STMT_PREPARE: "SELECT * FROM users WHERE id = ?"
  COM_STMT_EXECUTE: [stmt_id=1, params=[binary encoded 42]]
  → Need to track prepared statements per connection
  → Need to decode binary parameter encoding
  → Need to reconstruct full query for display

Pipelining:
  Client sends: QUERY1; QUERY2; QUERY3 (without waiting)
  → Can't assume request-response ordering
  → Need sequence number tracking

Compression (COMPRESS flag):
  Packets are zlib compressed after handshake
  → Need to detect compression negotiation
  → Need to decompress before parsing

Multi-statement:
  "SELECT 1; INSERT INTO t VALUES (1); SELECT 2"
  → Single packet, multiple queries
  → Multiple result sets

Connection state:
  USE database;  -- changes current database
  SET NAMES utf8mb4;  -- changes character set
  → Need to track per-connection state

SSL upgrade:
  Plain → COM_SSL → TLS handshake → encrypted
  → Need to switch from TCP parsing to TLS uprobes mid-connection

Character encoding:
  Query in latin1 vs utf8 vs utf8mb4
  → Wrong encoding = garbled query text

PgBouncer/ProxySQL:
  Connection poolers multiplex queries
  → Multiple logical connections over one TCP connection
  → Need to parse pooler protocol too
```

**Implementation for MySQL:**

```go
type MySQLConnection struct {
    // Connection state
    ConnectionID    uint32
    Database        string
    User            string
    CharacterSet    uint8
    Compressed      bool
    UsingSSL        bool

    // Prepared statement tracking
    PreparedStmts   map[uint32]*PreparedStatement

    // Query reassembly
    PendingPackets  [][]byte
    ExpectedSeq     uint8
}

type PreparedStatement struct {
    StmtID      uint32
    Query       string
    ParamCount  uint16
    ParamTypes  []uint8
}

func (c *MySQLConnection) ParsePacket(data []byte) (*MySQLEvent, error) {
    if c.Compressed {
        data = c.decompress(data)
    }

    cmd := data[0]
    switch cmd {
    case COM_QUERY:
        return c.parseQuery(data[1:])
    case COM_STMT_PREPARE:
        return c.parsePrepare(data[1:])
    case COM_STMT_EXECUTE:
        return c.parseExecute(data[1:]) // Needs PreparedStmts lookup
    case COM_STMT_CLOSE:
        return c.parseStmtClose(data[1:])
    // ... 20+ more command types
    }
}

func (c *MySQLConnection) parseExecute(data []byte) (*MySQLEvent, error) {
    stmtID := binary.LittleEndian.Uint32(data[0:4])

    stmt, ok := c.PreparedStmts[stmtID]
    if !ok {
        // Statement prepared before we started tracing
        return &MySQLEvent{Query: "<prepared statement, unknown>"}, nil
    }

    // Decode binary parameters
    params, err := c.decodeBinaryParams(data[5:], stmt.ParamTypes)
    if err != nil {
        return nil, err
    }

    // Reconstruct query with parameters
    query := c.reconstructQuery(stmt.Query, params)
    return &MySQLEvent{Query: query, Params: params}, nil
}
```

#### PostgreSQL Protocol Deep Dive

```
Similar complexity:
├── Simple query vs Extended query protocol
├── Parse/Bind/Execute message flow
├── Portal management
├── COPY protocol (bulk data)
├── SSL negotiation
├── SCRAM-SHA-256 auth parsing
└── Logical replication protocol
```

#### Redis Protocol Deep Dive

```
Easier but still has edge cases:
├── RESP2 vs RESP3 protocol
├── Pipelining (very common)
├── PubSub mode (changes parsing rules)
├── Cluster redirects (MOVED, ASK)
├── Transactions (MULTI/EXEC blocks)
├── Lua scripts (EVAL)
└── Module commands (custom protocols)
```

**Testing strategy:**

```go
// Capture real traffic for test cases
func TestMySQLParsing(t *testing.T) {
    // These are real packet captures from production
    testCases := []struct {
        name     string
        packets  [][]byte  // Captured from tcpdump
        expected []MySQLEvent
    }{
        {"simple_select", loadCapture("simple_select.pcap"), ...},
        {"prepared_stmt", loadCapture("prepared_stmt.pcap"), ...},
        {"compressed", loadCapture("compressed.pcap"), ...},
        {"pipelined", loadCapture("pipelined.pcap"), ...},
        {"ssl_upgrade", loadCapture("ssl_upgrade.pcap"), ...},
        {"pgbouncer_multiplex", loadCapture("pgbouncer.pcap"), ...},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // ...
        })
    }
}
```

**Effort estimate:**
- MySQL (solid): 3-4 weeks
- PostgreSQL (solid): 3-4 weeks
- Redis (solid): 1-2 weeks
- gRPC/HTTP2: 4-5 weeks
- Kafka: 3-4 weeks

**The moat:** Every production deployment finds new edge cases. Building a library of real-world packet captures from actual migrations = competitors can't match without the same experience.

---

### 4. Correlation Without Trace Headers

**The holy grail: distributed traces without any code changes.**

When services don't propagate W3C traceparent / B3 / X-Request-ID headers:

```
Request flow (what we see):
  10:00:00.000  Service A (pid 1234) → TCP connect to B:8080
  10:00:00.001  Service A → send HTTP request (247 bytes)
  10:00:00.023  Service B (pid 5678) → recv HTTP request
  10:00:00.025  Service B → TCP connect to MySQL:3306
  10:00:00.026  Service B → send MySQL query
  10:00:00.045  MySQL → send response
  10:00:00.046  Service B → recv MySQL response
  10:00:00.048  Service B → send HTTP response
  10:00:00.050  Service A → recv HTTP response

How do we KNOW the MySQL query belongs to that HTTP request?
```

**Correlation strategies:**

```go
// Strategy 1: Socket-to-process-to-socket tracking
type CorrelationEngine struct {
    // Track socket ownership
    SocketToProcess map[SocketKey]ProcessInfo

    // Track active requests per thread
    ThreadActiveRequest map[ThreadKey]*Request

    // Track pending requests awaiting response
    PendingRequests map[RequestKey]*Request
}

// When we see TCP connect from B to MySQL
func (e *CorrelationEngine) OnConnect(event *TCPConnectEvent) {
    // What thread made this connection?
    thread := ThreadKey{PID: event.PID, TID: event.TID}

    // Is this thread handling an inbound request?
    if req := e.ThreadActiveRequest[thread]; req != nil {
        // This outbound connection is part of that request's trace
        e.linkOutboundConnection(req, event)
    }
}

// Strategy 2: Timing-based correlation
type TimingCorrelator struct {
    // Window for matching requests
    MaxLatency time.Duration  // e.g., 100ms
}

func (c *TimingCorrelator) Correlate(
    inbound *Request,
    outbounds []*Request,
) []*Request {
    var correlated []*Request

    for _, out := range outbounds {
        // Outbound must start after inbound starts
        if out.StartTime.Before(inbound.StartTime) {
            continue
        }
        // Outbound must complete before inbound completes
        if out.EndTime.After(inbound.EndTime) {
            continue
        }
        // Same process
        if out.PID != inbound.PID {
            continue
        }

        correlated = append(correlated, out)
    }

    return correlated
}

// Strategy 3: Content-based correlation (best effort)
type ContentCorrelator struct{}

func (c *ContentCorrelator) FindRequestID(req *Request) string {
    // Check common header patterns
    for _, header := range []string{
        "X-Request-ID",
        "X-Correlation-ID",
        "X-Trace-ID",
        "Request-Id",
        "X-Amzn-Trace-Id",
    } {
        if val := req.Headers.Get(header); val != "" {
            return val
        }
    }

    // Check body for correlation IDs (JSON APIs)
    if id := extractJSONField(req.Body, "request_id"); id != "" {
        return id
    }

    return ""
}

// Combined: Use all strategies with confidence scoring
type TraceBuilder struct {
    strategies []CorrelationStrategy
}

func (b *TraceBuilder) BuildTrace(events []Event) (*Trace, float64) {
    trace := &Trace{}
    confidence := 0.0

    // Try content-based first (highest confidence)
    if linked := b.contentCorrelate(events); linked != nil {
        trace = linked
        confidence = 0.95
    }

    // Fall back to socket tracking
    if trace.IsEmpty() {
        if linked := b.socketCorrelate(events); linked != nil {
            trace = linked
            confidence = 0.85
        }
    }

    // Fall back to timing
    if trace.IsEmpty() {
        if linked := b.timingCorrelate(events); linked != nil {
            trace = linked
            confidence = 0.70
        }
    }

    return trace, confidence
}
```

**Edge cases that break correlation:**

| Scenario | Problem | Mitigation |
|----------|---------|------------|
| Connection pooling | One TCP connection, many requests | Track request/response pairing |
| Async workers | Request handled by different thread | Track goroutine/thread handoff |
| Queue-based | Request → Queue → Worker | Need queue protocol parsing |
| Long polling | Request open for minutes | Timeout handling |
| Multiplexing (HTTP/2) | Multiple requests on one connection | Stream ID tracking |
| Thread pools | Thread handles unrelated requests | Careful state clearing |

**Confidence reporting:**

```go
// Show users when correlation might be wrong
type Span struct {
    // ... normal span fields

    CorrelationConfidence float64  // 0.0 - 1.0
    CorrelationMethod     string   // "header", "socket", "timing"
    CorrelationWarnings   []string // ["timing-based, may be inaccurate"]
}
```

**Effort estimate:** 4-6 weeks for solid implementation, ongoing tuning based on real-world feedback

**The moat:** This is iterative. Every deployment teaches you new patterns. Competitors without production data can't tune their heuristics.

---

### 5. Low-Overhead Continuous Profiling

**The challenge:**
```
Requirements:
├── Sample CPU stacks at 100Hz (100 samples/second)
├── Across all processes on the system
├── Resolve symbols (function names, not addresses)
├── Handle multiple languages (Go, C, Java, Python, Node)
├── Store efficiently (deduplicate common stacks)
├── <1% CPU overhead
└── Don't drop samples under load
```

**Why symbol resolution is hell:**

```
Native binaries (C, C++, Rust):
├── Need DWARF debug info
├── Often stripped in production
├── Need to fetch debuginfo packages
└── Or use frame pointers (if compiled with -fno-omit-frame-pointer)

Go binaries:
├── Go has its own stack format
├── Goroutine stacks, not OS thread stacks
├── Need to parse Go runtime structures
├── gopclntab for symbol resolution
└── Changes between Go versions

JVM (Java, Kotlin, Scala):
├── JIT compiled - addresses change at runtime
├── Need to query JVM for method mappings
├── Perf-map-agent or AsyncProfiler integration
├── GraalVM native is different again

Python:
├── Interpreted - C stack shows interpreter, not Python
├── Need to read Python frame objects
├── py-spy approach: read PyFrameObject
└── Different for Python 3.10 vs 3.11 vs 3.12

Node.js:
├── V8 JIT compiled
├── Need V8 perf hooks
├── Or use --perf-basic-prof flag
└── Different for each Node version
```

**Implementation approach:**

```c
// eBPF: Capture stacks
SEC("perf_event")
int profile_cpu(struct bpf_perf_event_data *ctx) {
    u64 id = bpf_get_current_pid_tgid();
    u32 pid = id >> 32;

    // Get kernel stack
    int kernel_stack_id = bpf_get_stackid(ctx, &stack_traces, 0);

    // Get user stack
    int user_stack_id = bpf_get_stackid(ctx, &stack_traces, BPF_F_USER_STACK);

    struct profile_event event = {
        .pid = pid,
        .tid = id,
        .kernel_stack_id = kernel_stack_id,
        .user_stack_id = user_stack_id,
        .timestamp = bpf_ktime_get_ns(),
    };

    bpf_ringbuf_output(&events, &event, sizeof(event), 0);
    return 0;
}
```

```go
// Userspace: Symbol resolution
type SymbolResolver struct {
    // Per-process symbol tables
    processes map[uint32]*ProcessSymbols
}

type ProcessSymbols struct {
    PID         uint32
    Executable  string
    Mappings    []MemoryMapping
    Symbols     map[uint64]Symbol  // addr → symbol
    Language    string             // "go", "c", "java", etc.
}

func (r *SymbolResolver) Resolve(pid uint32, addr uint64) (Symbol, error) {
    proc := r.processes[pid]
    if proc == nil {
        proc = r.loadProcess(pid)
    }

    // Find which mapping contains this address
    mapping := proc.FindMapping(addr)
    if mapping == nil {
        return Symbol{Name: fmt.Sprintf("0x%x", addr)}, nil
    }

    // Adjust address for mapping offset
    offset := addr - mapping.Start + mapping.Offset

    // Look up symbol
    if sym, ok := proc.Symbols[offset]; ok {
        return sym, nil
    }

    // Language-specific resolution
    switch proc.Language {
    case "go":
        return r.resolveGoSymbol(proc, addr)
    case "java":
        return r.resolveJavaSymbol(proc, addr)
    default:
        return r.resolveDWARF(proc, offset)
    }
}
```

**Build vs Buy decision:**

| Option | Pros | Cons |
|--------|------|------|
| **Integrate Pyroscope** | Full-featured, maintained | Dependency, less control |
| **Use async-profiler** | Best for JVM | JVM only |
| **Build from scratch** | Full control | 10+ weeks, ongoing maintenance |

**Recommendation:** Start with Pyroscope integration (2-3 weeks). Build custom only if Pyroscope doesn't fit.

**Effort estimate:**
- Pyroscope integration: 2-3 weeks
- Custom (native + Go): 8-10 weeks
- Custom (+ JVM): +4 weeks
- Custom (+ Python/Node): +4 weeks each

---

### 6. Security Detection Without False Positive Hell

**The problem:**
```
Alert: "Shell spawned in container api-service-7f8d9"

Is this:
A) A health check: /bin/sh -c "curl localhost:8080/health"
B) A cron job: /bin/sh -c "/app/cleanup.sh"
C) An engineer debugging: kubectl exec -it api-service -- /bin/bash
D) An actual attack: reverse shell from compromised RCE
```

**All of them spawn /bin/sh in a container.** Alert on all = noise. Alert on none = miss attacks.

**Baseline learning approach:**

```go
type ContainerBaseline struct {
    // Key: image name (not container ID - containers are ephemeral)
    Image           string

    // Learned normal behavior
    NormalProcesses map[string]ProcessPattern  // process name → pattern
    NormalNetwork   map[string]NetworkPattern  // dest → pattern
    NormalFiles     map[string]FilePattern     // path → pattern

    // Learning state
    LearningStarted time.Time
    LearningComplete bool
    SampleCount     int
}

type ProcessPattern struct {
    ProcessName  string
    ParentName   string   // e.g., "containerd-shim"
    CommandLine  []string // common arguments
    Frequency    float64  // how often it runs
    SeenCount    int
}

func (b *ContainerBaseline) Learn(event *ProcessEvent) {
    key := event.ProcessName

    if pattern, exists := b.NormalProcesses[key]; exists {
        pattern.SeenCount++
        pattern.Frequency = b.calculateFrequency(pattern)
    } else {
        b.NormalProcesses[key] = &ProcessPattern{
            ProcessName: event.ProcessName,
            ParentName:  event.ParentName,
            CommandLine: event.Args,
            SeenCount:   1,
        }
    }
}

func (b *ContainerBaseline) IsAnomalous(event *ProcessEvent) (bool, string) {
    if !b.LearningComplete {
        return false, "still learning"
    }

    pattern, known := b.NormalProcesses[event.ProcessName]
    if !known {
        return true, fmt.Sprintf("process '%s' never seen in this image", event.ProcessName)
    }

    // Known process but unusual parent?
    if pattern.ParentName != "" && event.ParentName != pattern.ParentName {
        return true, fmt.Sprintf("unusual parent: expected '%s', got '%s'",
            pattern.ParentName, event.ParentName)
    }

    return false, ""
}
```

**Context-aware alerting:**

```go
type SecurityRule struct {
    Name        string
    Severity    Severity
    Condition   func(*Event, *ContainerBaseline) bool
    Context     func(*Event) []ContextInfo
}

var ShellInContainerRule = SecurityRule{
    Name:     "shell_in_container",
    Severity: WARNING,  // Not CRITICAL by default

    Condition: func(e *Event, b *ContainerBaseline) bool {
        if !isShell(e.ProcessName) {
            return false
        }
        if !e.InContainer {
            return false
        }
        // Check if this image normally runs shells
        if b.NormalProcesses[e.ProcessName] != nil {
            return false  // Normal for this image
        }
        return true
    },

    Context: func(e *Event) []ContextInfo {
        return []ContextInfo{
            checkKubectlExec(e),      // Is this a kubectl exec?
            checkHealthCheck(e),       // Health check pattern?
            checkCronJob(e),           // Scheduled job?
            checkSSHSession(e),        // SSH session?
            checkRecentDeploy(e),      // Recent deploy to this pod?
        }
    },
}

// Severity escalation based on context
func (r *SecurityRule) FinalSeverity(e *Event, contexts []ContextInfo) Severity {
    base := r.Severity

    for _, ctx := range contexts {
        if ctx.Type == "kubectl_exec" && ctx.UserKnown {
            // Known user doing kubectl exec = lower severity
            return INFO
        }
        if ctx.Type == "health_check" {
            // Definitely a health check
            return SUPPRESSED
        }
        if ctx.Type == "reverse_shell_pattern" {
            // stdin/stdout redirected to socket
            return CRITICAL  // Escalate!
        }
    }

    return base
}
```

**Known-good patterns to whitelist:**

```go
var KnownGoodPatterns = []WhitelistPattern{
    // Health checks
    {
        Parent:  "containerd-shim",
        Process: "sh",
        Args:    regexp.MustCompile(`curl.*localhost.*/health`),
        Reason:  "health check",
    },
    // Kubernetes probes
    {
        Parent:  "kubelet",
        Process: "*",
        Reason:  "kubernetes probe",
    },
    // Init containers
    {
        Container: regexp.MustCompile(`-init$`),
        Reason:    "init container",
    },
}
```

**Effort estimate:** 4-6 weeks for solid baseline learning + context-aware alerting

**The moat:** False positive tuning requires real deployments. Every customer teaches you new patterns. This knowledge compounds over time.

---

### 7. Migration Fidelity (Your Unique Advantage)

**You've done the Rivian migration. You know what actually breaks.**

```
Datadog Dashboard JSON → dogwatch

What tutorials show:
  timeseries widget → timeseries widget
  ✓ Easy!

What actually happens:

Template variables:
  $env = {prod, staging, dev}
  Query: avg:cpu{env:$env} by {host}
  → Need to parse and preserve variable syntax
  → Variables can have default values
  → Variables can depend on other variables

Datadog-specific functions:
  anomalies(avg:cpu{*}, 'agile', 2)
  forecast(avg:requests{*}, 'linear', 1)
  → No equivalent in PromQL
  → Need to either skip or implement equivalents

Composite monitors:
  (A && B) || (C && !D)
  Where A, B, C, D are other monitors
  → Need to recursively resolve

APM-specific widgets:
  Service Map widget (Datadog's trace data)
  Trace flamegraph widget
  → Need equivalent data in dogwatch

Dashboard links:
  "View in Logs" button → Datadog logs page
  → Need to rewrite to dogwatch equivalent

SLO references:
  Widget shows "SLO: api-latency-p99"
  → Need to import SLOs first

Conditional formatting:
  "Red if > 90%, yellow if > 70%"
  → Different syntax per platform

Annotations:
  "Deploy markers" on graphs
  → Need to import deploy data too
```

**Implementation:**

```go
type DatadogMigrator struct {
    // Track what we can and can't handle
    Capabilities MigrationCapabilities

    // Track issues found
    Issues []MigrationIssue
}

type MigrationCapabilities struct {
    WidgetTypes    map[string]WidgetConverter
    QueryFunctions map[string]QueryConverter
    MonitorTypes   map[string]MonitorConverter
}

type MigrationIssue struct {
    Severity    string  // "error", "warning", "info"
    Component   string  // "dashboard:main-overview"
    Field       string  // "widget[3].query"
    Original    string  // Original Datadog syntax
    Problem     string  // "anomalies() has no equivalent"
    Suggestion  string  // "Consider using threshold-based alerting instead"
}

func (m *DatadogMigrator) MigrateDashboard(dd DatadogDashboard) (*Dashboard, []MigrationIssue) {
    dashboard := &Dashboard{
        Name: dd.Title,
        Description: dd.Description,
    }

    for i, widget := range dd.Widgets {
        converted, issues := m.convertWidget(widget)

        for _, issue := range issues {
            issue.Component = fmt.Sprintf("dashboard:%s", dd.Title)
            issue.Field = fmt.Sprintf("widget[%d]", i)
            m.Issues = append(m.Issues, issue)
        }

        if converted != nil {
            dashboard.Widgets = append(dashboard.Widgets, converted)
        }
    }

    return dashboard, m.Issues
}

// Query translation with detailed error reporting
func (m *DatadogMigrator) TranslateQuery(ddQuery string) (string, []MigrationIssue) {
    issues := []MigrationIssue{}

    // Parse Datadog query
    parsed, err := parseDatadogQuery(ddQuery)
    if err != nil {
        return "", []MigrationIssue{{
            Severity: "error",
            Problem:  fmt.Sprintf("Cannot parse: %v", err),
            Original: ddQuery,
        }}
    }

    // Check for unsupported functions
    for _, fn := range parsed.Functions {
        if _, ok := m.Capabilities.QueryFunctions[fn.Name]; !ok {
            issues = append(issues, MigrationIssue{
                Severity:   "warning",
                Problem:    fmt.Sprintf("Function '%s' has no equivalent", fn.Name),
                Original:   ddQuery,
                Suggestion: suggestAlternative(fn.Name),
            })
        }
    }

    // Convert what we can
    promql := m.toPromQL(parsed)

    return promql, issues
}
```

**Migration report output:**

```
┌─────────────────────────────────────────────────────────────┐
│  📊 Migration Report: Datadog → dogwatch                     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Dashboards: 23 total                                       │
│  ├── ✅ 18 fully compatible                                 │
│  ├── ⚠️  4 need adjustments                                 │
│  └── ❌ 1 cannot migrate (APM-specific)                     │
│                                                              │
│  Detailed Issues:                                           │
│                                                              │
│  ⚠️  dashboard:api-overview / widget[3]                     │
│     Query: anomalies(avg:latency{*}, 'agile', 2)           │
│     Problem: anomalies() has no equivalent                  │
│     Suggestion: Use threshold alerting on p99 instead       │
│                                                              │
│  ⚠️  dashboard:infrastructure / widget[7]                   │
│     Query: forecast(sum:requests{*}, 'linear', 1)          │
│     Problem: forecast() has no equivalent                   │
│     Suggestion: Remove or use external forecasting tool     │
│                                                              │
│  ❌ dashboard:apm-services                                  │
│     Problem: Entire dashboard uses Datadog APM widgets      │
│     Suggestion: Skip - dogwatch has its own service views   │
│                                                              │
│  Monitors: 47 total                                         │
│  ├── ✅ 42 fully compatible                                 │
│  └── ⚠️  5 need adjustments                                 │
│                                                              │
│  💰 Cost Analysis:                                          │
│  Current Datadog: $47,234/month                             │
│  After migration: $0 (self-hosted)                          │
│  Annual savings: $566,808                                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Effort estimate:** 4-6 weeks for solid Datadog migration, 2-3 weeks for Grafana

**The moat:** You've done a real migration at scale. You know the 50 edge cases that blog posts don't mention. Every migration you do adds to this knowledge. Competitors without this experience will miss cases.

---

### 8. Intelligent Tail Sampling

**The problem:** You can't store every trace. But head sampling (decide at start) throws away errors and slow requests. Tail sampling (decide at end) requires buffering everything.

```
Head sampling problem:
  Request starts → flip coin → keep 10%
  → But what if this request will ERROR?
  → What if this request will be SLOW?
  → You don't know yet. You might throw away the interesting ones.

Tail sampling problem:
  Buffer ALL traces for 30 seconds
  Wait for trace to "complete"
  Then decide: keep errors, keep slow, sample rest
  → Requires massive memory
  → What is "complete"? Async traces never end cleanly.
  → Distributed: spans arrive at different collectors
```

**The moat: Hybrid intelligent sampling**

```go
// Adaptive sampling that doesn't require buffering everything
type IntelligentSampler struct {
    // Base sample rate
    BaseSampleRate float64  // e.g., 0.1 = 10%

    // Priority rules - these always get kept
    PriorityRules []SamplingRule

    // Adaptive rate based on throughput
    CurrentRate    float64
    TargetSpansPerSecond int64
}

type SamplingRule struct {
    Name       string
    Condition  func(*Span) bool
    Action     SampleAction  // ALWAYS_KEEP, ALWAYS_DROP, USE_BASE_RATE
    Priority   int           // Higher = evaluated first
}

var DefaultRules = []SamplingRule{
    // Always keep errors
    {
        Name:      "keep_errors",
        Condition: func(s *Span) bool { return s.Status == ERROR },
        Action:    ALWAYS_KEEP,
        Priority:  100,
    },
    // Always keep slow requests (>1s)
    {
        Name:      "keep_slow",
        Condition: func(s *Span) bool { return s.Duration > time.Second },
        Action:    ALWAYS_KEEP,
        Priority:  90,
    },
    // Always keep if parent was kept (trace coherence)
    {
        Name:      "keep_if_parent_kept",
        Condition: func(s *Span) bool { return s.ParentSampled },
        Action:    ALWAYS_KEEP,
        Priority:  80,
    },
    // Keep new endpoints (never seen before)
    {
        Name:      "keep_new_endpoints",
        Condition: func(s *Span) bool { return s.IsNewEndpoint },
        Action:    ALWAYS_KEEP,
        Priority:  70,
    },
    // Sample rest based on adaptive rate
    {
        Name:      "base_sample",
        Condition: func(s *Span) bool { return true },
        Action:    USE_BASE_RATE,
        Priority:  0,
    },
}

// The magic: consistent sampling for trace coherence
func (s *IntelligentSampler) ShouldSample(span *Span) bool {
    // Check priority rules first
    for _, rule := range s.PriorityRules {
        if rule.Condition(span) {
            switch rule.Action {
            case ALWAYS_KEEP:
                return true
            case ALWAYS_DROP:
                return false
            case USE_BASE_RATE:
                // Fall through to consistent sampling
            }
        }
    }

    // Consistent sampling: same trace ID = same decision everywhere
    // This is critical for distributed tracing
    hash := xxhash(span.TraceID)
    threshold := uint64(s.CurrentRate * math.MaxUint64)
    return hash < threshold
}

// Adaptive rate adjustment
func (s *IntelligentSampler) AdjustRate() {
    currentThroughput := s.measureSpansPerSecond()

    if currentThroughput > s.TargetSpansPerSecond * 1.2 {
        // Too much data, reduce sampling
        s.CurrentRate *= 0.9
    } else if currentThroughput < s.TargetSpansPerSecond * 0.8 {
        // Room for more, increase sampling
        s.CurrentRate = min(s.CurrentRate * 1.1, s.BaseSampleRate)
    }
}
```

**Advanced: Retroactive sampling**

```go
// When we see an error, go back and fetch related spans
type RetroactiveSampler struct {
    // Short-term buffer of span metadata (not full spans)
    RecentSpanIndex map[TraceID][]SpanMetadata

    // Storage for recently dropped spans (compressed, short TTL)
    DroppedSpanCache *LRUCache
}

func (s *RetroactiveSampler) OnError(errorSpan *Span) {
    // Find all spans in this trace from the cache
    traceID := errorSpan.TraceID

    // Retrieve dropped spans from cache
    droppedSpans := s.DroppedSpanCache.GetByTraceID(traceID)

    // Promote them to permanent storage
    for _, span := range droppedSpans {
        s.permanentStore.Write(span)
    }

    // Mark trace as "always keep future spans"
    s.RecentSpanIndex[traceID].AlwaysKeep = true
}
```

**Why it's hard:**
- Consistent sampling across distributed collectors
- Trace coherence (if parent sampled, children must be too)
- Adaptive rate without oscillation
- Low latency (can't wait to decide)
- Memory bounded

**Effort estimate:** 3-4 weeks for solid implementation

---

### 9. Automatic Log Field Extraction

**The problem:** Logs are unstructured text. Querying them means regex.

```
Raw logs:
2024-01-15 10:23:45.123 INFO  [api-service] user=12345 action=login ip=192.168.1.1 duration=45ms
2024-01-15 10:23:45.456 ERROR [api-service] user=12345 action=checkout error="payment failed" amount=99.99
{"timestamp": "2024-01-15T10:23:45Z", "level": "warn", "service": "worker", "msg": "queue full", "queue_size": 10000}
Jan 15 10:23:45 web-server nginx: 192.168.1.1 - - [15/Jan/2024:10:23:45 +0000] "GET /api/users HTTP/1.1" 200 1234
```

**The moat: Automatic structure detection**

```go
// Pattern detection engine
type LogPatternEngine struct {
    // Known patterns
    KnownPatterns []LogPattern

    // Learned patterns from data
    LearnedPatterns map[string]*LearnedPattern

    // Field extractors
    Extractors []FieldExtractor
}

type LogPattern struct {
    Name        string
    Regex       *regexp.Regexp
    Fields      []FieldDefinition
    Confidence  float64
}

var BuiltInPatterns = []LogPattern{
    // JSON logs
    {
        Name:   "json",
        Regex:  regexp.MustCompile(`^\s*\{.*\}\s*$`),
        Fields: nil,  // Dynamic from JSON
        Confidence: 0.99,
    },
    // Logfmt (key=value)
    {
        Name:   "logfmt",
        Regex:  regexp.MustCompile(`(\w+)=("[^"]*"|\S+)`),
        Fields: nil,  // Dynamic from matches
        Confidence: 0.95,
    },
    // Common Log Format (Apache/Nginx)
    {
        Name:  "clf",
        Regex: regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) (\S+) (\S+)" (\d+) (\d+)`),
        Fields: []FieldDefinition{
            {Name: "client_ip", Index: 1, Type: "ip"},
            {Name: "timestamp", Index: 2, Type: "timestamp"},
            {Name: "method", Index: 3, Type: "string"},
            {Name: "path", Index: 4, Type: "string"},
            {Name: "protocol", Index: 5, Type: "string"},
            {Name: "status", Index: 6, Type: "int"},
            {Name: "bytes", Index: 7, Type: "int"},
        },
        Confidence: 0.90,
    },
    // Syslog
    {
        Name:  "syslog",
        Regex: regexp.MustCompile(`^<(\d+)>(\w+ \d+ \d+:\d+:\d+) (\S+) (\S+): (.*)$`),
        Fields: []FieldDefinition{
            {Name: "priority", Index: 1, Type: "int"},
            {Name: "timestamp", Index: 2, Type: "timestamp"},
            {Name: "host", Index: 3, Type: "string"},
            {Name: "program", Index: 4, Type: "string"},
            {Name: "message", Index: 5, Type: "string"},
        },
        Confidence: 0.85,
    },
}

// Automatic pattern learning
type PatternLearner struct {
    // Sample buffer
    Samples []string

    // Token frequency analysis
    TokenFrequency map[string]int

    // Position analysis
    PositionPatterns map[int]map[string]int
}

func (l *PatternLearner) LearnPattern(samples []string) *LearnedPattern {
    // Tokenize all samples
    tokenized := make([][]Token, len(samples))
    for i, s := range samples {
        tokenized[i] = tokenize(s)
    }

    // Find common structure
    // "2024-01-15 ERROR user=123" → [TIMESTAMP, LEVEL, FIELD=VALUE]
    pattern := l.findCommonStructure(tokenized)

    // Identify variable vs constant parts
    // If position 0 is always a timestamp format → it's a timestamp field
    // If position 2 varies but matches \w+=\S+ → it's a key-value field
    for pos, tokens := range l.groupByPosition(tokenized) {
        pattern.Positions[pos] = l.classifyPosition(tokens)
    }

    return pattern
}

// Smart field type inference
func inferFieldType(values []string) FieldType {
    // Sample values and detect type
    var ints, floats, ips, timestamps, bools, strings int

    for _, v := range values {
        switch {
        case isInt(v):
            ints++
        case isFloat(v):
            floats++
        case isIP(v):
            ips++
        case isTimestamp(v):
            timestamps++
        case isBool(v):
            bools++
        default:
            strings++
        }
    }

    total := len(values)
    // Return most likely type (>80% match)
    if float64(ints)/float64(total) > 0.8 {
        return FieldTypeInt
    }
    if float64(ips)/float64(total) > 0.8 {
        return FieldTypeIP
    }
    // ... etc

    return FieldTypeString
}
```

**Query-time extraction (for unknown patterns):**

```go
// User can define extraction at query time
// logs | extract "user=(?P<user_id>\d+)" | where user_id = "12345"

type ExtractOperator struct {
    Pattern *regexp.Regexp
    Fields  []string  // Named capture groups
}

func (e *ExtractOperator) Process(log *Log) *Log {
    matches := e.Pattern.FindStringSubmatch(log.Message)
    if matches == nil {
        return log
    }

    // Add extracted fields
    for i, name := range e.Pattern.SubexpNames() {
        if name != "" && i < len(matches) {
            log.Fields[name] = matches[i]
        }
    }

    return log
}
```

**The UI: Field discovery**

```
┌─────────────────────────────────────────────────────────────┐
│  🔍 Discovered Fields (auto-extracted)                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  From 1,247,832 logs in last hour:                          │
│                                                              │
│  Detected pattern: logfmt (94% of logs)                     │
│                                                              │
│  Fields found:                                               │
│  ├── level      (string)  INFO: 89%, ERROR: 8%, WARN: 3%   │
│  ├── service    (string)  api: 45%, worker: 35%, web: 20%  │
│  ├── user_id    (int)     12,847 unique values             │
│  ├── duration   (duration) p50: 23ms, p99: 450ms           │
│  ├── status     (int)     200: 94%, 500: 4%, 404: 2%       │
│  └── trace_id   (string)  can link to traces ✓             │
│                                                              │
│  [Create Index] [Add to Dashboard] [Set Up Alert]           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Effort estimate:** 3-4 weeks for core extraction + learning

---

### 10. Multi-Signal Correlation Engine

**The problem:** Logs, traces, and metrics are collected separately. Connecting them requires manual work.

```
User sees: Latency spike in metrics graph
User wants: WHY? Show me the traces. Show me the error logs.

Today's workflow:
1. See spike in metrics at 10:23
2. Open traces tab, filter by time
3. Find slow traces manually
4. Copy trace ID
5. Open logs tab, search for trace ID
6. Hope they're connected

dogwatch workflow:
1. Click on spike
2. See correlated traces and logs automatically
```

**Implementation:**

```go
// Correlation engine maintains links between signals
type CorrelationEngine struct {
    // Index: trace_id → log entries
    TraceToLogs map[string][]LogRef

    // Index: trace_id → metric exemplars
    TraceToMetrics map[string][]MetricRef

    // Index: (service, time_bucket) → traces
    ServiceTimeToTraces map[ServiceTimeKey][]TraceRef

    // Index: (service, time_bucket) → logs
    ServiceTimeToLogs map[ServiceTimeKey][]LogRef
}

// When ingesting a log, extract correlation keys
func (e *CorrelationEngine) IndexLog(log *Log) {
    // Try to find trace ID in various formats
    traceID := e.extractTraceID(log)
    if traceID != "" {
        e.TraceToLogs[traceID] = append(e.TraceToLogs[traceID], log.Ref())
    }

    // Index by service + time for fuzzy correlation
    key := ServiceTimeKey{
        Service:    log.Service,
        TimeBucket: log.Timestamp.Truncate(time.Second),
    }
    e.ServiceTimeToLogs[key] = append(e.ServiceTimeToLogs[key], log.Ref())
}

// Extract trace ID from various formats
func (e *CorrelationEngine) extractTraceID(log *Log) string {
    // Check structured fields first
    for _, field := range []string{"trace_id", "traceId", "trace-id", "request_id", "X-Request-ID"} {
        if val, ok := log.Fields[field]; ok {
            return val
        }
    }

    // Check common patterns in message
    patterns := []*regexp.Regexp{
        regexp.MustCompile(`trace_id=([a-f0-9]{32})`),
        regexp.MustCompile(`traceId=([a-f0-9-]{36})`),
        regexp.MustCompile(`\[([a-f0-9]{32})\]`),  // [traceid] format
        regexp.MustCompile(`rid=([a-f0-9-]{36})`),
    }

    for _, p := range patterns {
        if m := p.FindStringSubmatch(log.Message); m != nil {
            return m[1]
        }
    }

    return ""
}

// Metric exemplars: link metrics to traces
type MetricPoint struct {
    Timestamp time.Time
    Value     float64
    Labels    map[string]string

    // Exemplar: a sample trace that contributed to this value
    Exemplar  *Exemplar
}

type Exemplar struct {
    TraceID   string
    SpanID    string
    Value     float64  // The specific value from this trace
    Timestamp time.Time
}

// When recording a metric, optionally attach an exemplar
func (r *MetricRecorder) RecordWithExemplar(name string, value float64, labels map[string]string, span *Span) {
    point := &MetricPoint{
        Timestamp: time.Now(),
        Value:     value,
        Labels:    labels,
    }

    // Attach exemplar with sampling
    if r.shouldAttachExemplar(name) {
        point.Exemplar = &Exemplar{
            TraceID:   span.TraceID,
            SpanID:    span.SpanID,
            Value:     value,
            Timestamp: span.StartTime,
        }
    }

    r.store.Write(point)
}
```

**Fuzzy correlation (when there's no trace ID):**

```go
// Correlate by service + time window + error signature
type FuzzyCorrelator struct {
    TimeWindow time.Duration  // e.g., 5 seconds
}

func (c *FuzzyCorrelator) FindRelatedLogs(span *Span) []*Log {
    // Find logs from same service in time window
    startTime := span.StartTime.Add(-c.TimeWindow)
    endTime := span.EndTime.Add(c.TimeWindow)

    logs := c.queryLogs(
        span.Service,
        startTime,
        endTime,
    )

    // Rank by relevance
    scored := make([]ScoredLog, len(logs))
    for i, log := range logs {
        scored[i] = ScoredLog{
            Log:   log,
            Score: c.scoreRelevance(span, log),
        }
    }

    sort.Slice(scored, func(i, j int) bool {
        return scored[i].Score > scored[j].Score
    })

    return top(scored, 10)
}

func (c *FuzzyCorrelator) scoreRelevance(span *Span, log *Log) float64 {
    score := 0.0

    // Time proximity (closer = higher score)
    timeDiff := log.Timestamp.Sub(span.StartTime).Abs()
    score += 1.0 / (1.0 + timeDiff.Seconds())

    // Same error type?
    if span.Status == ERROR && log.Level == "error" {
        score += 2.0
    }

    // Same endpoint?
    if strings.Contains(log.Message, span.Operation) {
        score += 1.5
    }

    // Same user/request context?
    if span.Tags["user_id"] != "" && strings.Contains(log.Message, span.Tags["user_id"]) {
        score += 3.0
    }

    return score
}
```

**The UI: Unified view**

```
┌─────────────────────────────────────────────────────────────┐
│  📊 Latency Spike Investigation                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Metric: http_request_duration_seconds (p99)                │
│  Time: 2024-01-15 10:23:00 - 10:28:00                       │
│  Spike: 450ms → 2.3s (5x increase)                          │
│                                                              │
│  ═══════════════════════════════════════════════════════    │
│                                                              │
│  🔗 Correlated Traces (47 found)                            │
│  ├── GET /api/checkout  2.4s  ERROR  trace:abc123          │
│  ├── GET /api/checkout  2.1s  ERROR  trace:def456          │
│  ├── GET /api/checkout  1.9s  OK     trace:ghi789          │
│  └── [View all 47 traces]                                   │
│                                                              │
│  📝 Correlated Logs (312 found)                             │
│  ├── ERROR [payment-service] timeout connecting to stripe   │
│  ├── ERROR [payment-service] retry 1/3 failed              │
│  ├── WARN  [api-gateway] upstream latency exceeded SLO      │
│  └── [View all 312 logs]                                    │
│                                                              │
│  🔄 Changes in Window                                       │
│  ├── 10:21 Deploy: payment-service v2.4.1 → v2.4.2         │
│  └── [View deploy diff]                                     │
│                                                              │
│  🎯 BubbleUp Analysis                                       │
│  └── 94% of slow requests hit payment-service               │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Effort estimate:** 4-5 weeks for full correlation engine

---

### 11. SQLite Time-Series Optimization

**The problem:** SQLite isn't designed for time-series. Naive implementation = slow queries.

```
Naive schema:
CREATE TABLE metrics (
    timestamp INTEGER,
    name TEXT,
    value REAL,
    labels TEXT  -- JSON blob
);

Query: "Get cpu_usage for service=api, last hour, 1-minute buckets"

SELECT
    (timestamp / 60000) * 60000 as bucket,
    AVG(value)
FROM metrics
WHERE name = 'cpu_usage'
  AND json_extract(labels, '$.service') = 'api'
  AND timestamp > ?
GROUP BY bucket;

Problem: Full table scan, JSON parsing on every row = SLOW
```

**The moat: Domain-specific optimizations**

```go
// Optimized schema for time-series
const schema = `
-- Metric names are interned (stored once)
CREATE TABLE metric_names (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

-- Label sets are interned and indexed
CREATE TABLE label_sets (
    id INTEGER PRIMARY KEY,
    hash INTEGER NOT NULL,           -- Fast lookup
    labels BLOB NOT NULL,            -- Compressed label key-values
    UNIQUE(hash, labels)
);

-- Separate tables per metric (Prometheus-style)
-- Created dynamically: metrics_cpu_usage, metrics_http_requests, etc.
-- This avoids the "name = ?" filter on every query

-- Time-partitioned tables (one per day)
-- metrics_cpu_usage_20240115, metrics_cpu_usage_20240116, etc.
-- Old partitions can be dropped without VACUUM

-- Actual data storage (per metric, per day)
CREATE TABLE metrics_cpu_usage_20240115 (
    timestamp INTEGER NOT NULL,
    label_set_id INTEGER NOT NULL,
    value REAL NOT NULL
);

-- Covering index: timestamp + labels for range queries
CREATE INDEX idx_metrics_cpu_usage_20240115_ts_labels
ON metrics_cpu_usage_20240115(timestamp, label_set_id);
`

// Write path: batch and compress
type MetricWriter struct {
    buffer    []MetricPoint
    batchSize int
    labelCache *LRUCache  // Avoid repeated label lookups
}

func (w *MetricWriter) Write(point MetricPoint) error {
    // Intern label set
    labelSetID := w.getOrCreateLabelSet(point.Labels)

    w.buffer = append(w.buffer, MetricPoint{
        Timestamp:   point.Timestamp,
        LabelSetID:  labelSetID,
        Value:       point.Value,
    })

    if len(w.buffer) >= w.batchSize {
        return w.flush()
    }
    return nil
}

func (w *MetricWriter) flush() error {
    // Sort by timestamp for sequential writes
    sort.Slice(w.buffer, func(i, j int) bool {
        return w.buffer[i].Timestamp < w.buffer[j].Timestamp
    })

    // Batch insert
    tx, _ := w.db.Begin()
    stmt, _ := tx.Prepare("INSERT INTO metrics_cpu_usage_? VALUES (?, ?, ?)")

    for _, p := range w.buffer {
        partition := p.Timestamp.Format("20060102")
        stmt.Exec(partition, p.Timestamp.UnixMilli(), p.LabelSetID, p.Value)
    }

    tx.Commit()
    w.buffer = w.buffer[:0]
    return nil
}
```

**Query optimizations:**

```go
// Query planner that uses partitions and indexes effectively
type QueryPlanner struct {
    db *sql.DB
}

func (p *QueryPlanner) PlanQuery(q *MetricQuery) (*QueryPlan, error) {
    plan := &QueryPlan{}

    // Determine which partitions to query
    startDay := q.Start.Truncate(24 * time.Hour)
    endDay := q.End.Truncate(24 * time.Hour)

    for day := startDay; !day.After(endDay); day = day.Add(24 * time.Hour) {
        partition := fmt.Sprintf("metrics_%s_%s", q.MetricName, day.Format("20060102"))
        if p.partitionExists(partition) {
            plan.Partitions = append(plan.Partitions, partition)
        }
    }

    // Resolve label filters to label_set_ids
    if len(q.LabelFilters) > 0 {
        plan.LabelSetIDs = p.resolveLabelSets(q.LabelFilters)
        // If we know the exact label sets, use IN clause (fast)
        // Otherwise, fall back to join
    }

    // Choose aggregation strategy
    if q.Step >= time.Minute {
        // Pre-aggregate at query time using SQLite window functions
        plan.Strategy = "window_agg"
    } else {
        // Raw data, aggregate in Go
        plan.Strategy = "raw_then_agg"
    }

    return plan, nil
}

// Example optimized query
func (p *QueryPlanner) BuildSQL(plan *QueryPlan) string {
    if len(plan.LabelSetIDs) > 0 && len(plan.LabelSetIDs) < 1000 {
        // Fast path: we know exactly which label sets
        return fmt.Sprintf(`
            SELECT
                (timestamp / %d) * %d as bucket,
                AVG(value) as value
            FROM %s
            WHERE timestamp BETWEEN ? AND ?
              AND label_set_id IN (%s)
            GROUP BY bucket
            ORDER BY bucket
        `, plan.BucketMs, plan.BucketMs,
           plan.Partitions[0],
           joinInts(plan.LabelSetIDs))
    }

    // Slower path: join with label_sets table
    return `...`
}
```

**Compression for older data:**

```go
// Downsample old data to save space
type RetentionManager struct {
    rules []RetentionRule
}

type RetentionRule struct {
    Age       time.Duration
    Action    RetentionAction
}

var DefaultRetention = []RetentionRule{
    // Last 24 hours: full resolution
    {Age: 24 * time.Hour, Action: KEEP_RAW},

    // 1-7 days: 1-minute resolution
    {Age: 7 * 24 * time.Hour, Action: DOWNSAMPLE_1M},

    // 7-30 days: 5-minute resolution
    {Age: 30 * 24 * time.Hour, Action: DOWNSAMPLE_5M},

    // 30-90 days: 1-hour resolution
    {Age: 90 * 24 * time.Hour, Action: DOWNSAMPLE_1H},

    // Older: delete
    {Age: 0, Action: DELETE},
}

func (m *RetentionManager) Compact(partition string) error {
    // Read old data
    // Aggregate into buckets
    // Write to new partition
    // Drop old partition
}
```

**Benchmarks to target:**

| Query | Naive | Optimized | Target |
|-------|-------|-----------|--------|
| Last hour, single metric | 2s | 50ms | <100ms |
| Last day, 10 metrics | 30s | 200ms | <500ms |
| Last week, aggregated | 5min | 1s | <2s |
| Last month, 1h buckets | timeout | 3s | <5s |

**Effort estimate:** 4-6 weeks for full optimization

---

### 12. Cardinality Bomb Prevention

**The problem:** One bad label can create millions of metric series and kill your storage.

```
Innocent-looking metric:
  http_requests_total{method="GET", path="/api/users", user_id="12345"}

Seems fine until:
  - 1 million users
  - 100 endpoints
  - 5 methods
  = 500 million series

Storage: explodes
Queries: timeout
Bill (if Datadog): $$$$$
```

**The moat: Real-time cardinality detection and prevention**

```go
// Cardinality tracker with bloom filters for efficiency
type CardinalityTracker struct {
    // Per-metric cardinality estimates
    Metrics map[string]*MetricCardinality

    // Global limits
    MaxSeriesPerMetric int64
    MaxTotalSeries     int64

    // Alert thresholds
    WarnThreshold float64  // 0.7 = warn at 70% of limit
}

type MetricCardinality struct {
    MetricName string

    // HyperLogLog for cardinality estimation (memory efficient)
    SeriesHLL *hyperloglog.Sketch

    // Per-label cardinality
    LabelCardinality map[string]*hyperloglog.Sketch

    // Exact count for small cardinalities
    ExactCount int64
    UseExact   bool  // Switch to HLL after threshold

    // Tracking
    FirstSeen time.Time
    LastSeen  time.Time

    // Alerts
    OverLimit bool
    Warnings  []CardinalityWarning
}

// On every metric write
func (t *CardinalityTracker) Track(metric string, labels map[string]string) error {
    mc := t.getOrCreate(metric)

    // Build series key
    seriesKey := buildSeriesKey(metric, labels)

    // Update HLL
    mc.SeriesHLL.Insert([]byte(seriesKey))
    estimatedCardinality := mc.SeriesHLL.Estimate()

    // Check limits
    if estimatedCardinality > float64(t.MaxSeriesPerMetric) {
        if !mc.OverLimit {
            mc.OverLimit = true
            t.alertCardinalityExplosion(mc)
        }
        return ErrCardinalityLimitExceeded
    }

    // Track per-label cardinality
    for label, value := range labels {
        if mc.LabelCardinality[label] == nil {
            mc.LabelCardinality[label] = hyperloglog.New14()
        }
        mc.LabelCardinality[label].Insert([]byte(value))
    }

    return nil
}

// Find the problematic labels
func (mc *MetricCardinality) FindHighCardinalityLabels() []LabelAnalysis {
    results := []LabelAnalysis{}

    totalCardinality := mc.SeriesHLL.Estimate()

    for label, hll := range mc.LabelCardinality {
        labelCardinality := hll.Estimate()

        // This label contributes to cardinality explosion if:
        // - It has many unique values
        // - Removing it would significantly reduce total cardinality
        results = append(results, LabelAnalysis{
            Label:           label,
            UniqueValues:    int64(labelCardinality),
            ContributionPct: (labelCardinality / totalCardinality) * 100,
            Recommendation:  recommendAction(label, labelCardinality),
        })
    }

    sort.Slice(results, func(i, j int) bool {
        return results[i].UniqueValues > results[j].UniqueValues
    })

    return results
}

func recommendAction(label string, cardinality float64) string {
    // Known high-cardinality labels that are usually mistakes
    badLabels := map[string]string{
        "user_id":     "Consider removing or using bucket ranges",
        "request_id":  "Remove - unique per request, useless for metrics",
        "timestamp":   "Remove - already in time dimension",
        "trace_id":    "Remove - use exemplars instead",
        "session_id":  "Remove or sample",
        "ip":          "Consider using IP ranges or geo",
        "url":         "Consider using path templates",
        "query":       "Remove - use logs for full queries",
    }

    if rec, ok := badLabels[label]; ok {
        return rec
    }

    if cardinality > 10000 {
        return "High cardinality - consider aggregating or removing"
    }

    return "OK"
}
```

**Prevention at ingest:**

```go
// Configurable cardinality limits
type CardinalityLimiter struct {
    Rules []CardinalityRule
}

type CardinalityRule struct {
    MetricMatch string           // Regex for metric name
    MaxSeries   int64            // Max series for this metric
    LabelRules  []LabelRule
    Action      LimitAction      // REJECT, DROP_LABELS, AGGREGATE
}

type LabelRule struct {
    Label       string
    MaxValues   int64            // Max unique values
    Action      LimitAction
}

func (l *CardinalityLimiter) Check(metric string, labels map[string]string) (*LimitResult, error) {
    for _, rule := range l.Rules {
        if !rule.Matches(metric) {
            continue
        }

        // Check metric-level limit
        if rule.MaxSeries > 0 {
            current := l.tracker.GetCardinality(metric)
            if current >= rule.MaxSeries {
                switch rule.Action {
                case REJECT:
                    return &LimitResult{Rejected: true, Reason: "series limit exceeded"}, nil
                case DROP_LABELS:
                    labels = l.dropHighCardinalityLabels(metric, labels)
                case AGGREGATE:
                    labels = l.aggregateLabels(metric, labels)
                }
            }
        }

        // Check label-level limits
        for _, lr := range rule.LabelRules {
            if val, ok := labels[lr.Label]; ok {
                labelCard := l.tracker.GetLabelCardinality(metric, lr.Label)
                if labelCard >= lr.MaxValues {
                    switch lr.Action {
                    case DROP_LABELS:
                        delete(labels, lr.Label)
                    case AGGREGATE:
                        labels[lr.Label] = l.bucketize(lr.Label, val)
                    }
                }
            }
        }
    }

    return &LimitResult{Labels: labels}, nil
}

// Auto-bucketize high-cardinality values
func (l *CardinalityLimiter) bucketize(label, value string) string {
    switch label {
    case "user_id":
        // user_id=12345 → user_id_bucket=12000-13000
        if id, err := strconv.ParseInt(value, 10, 64); err == nil {
            bucket := (id / 1000) * 1000
            return fmt.Sprintf("%d-%d", bucket, bucket+1000)
        }
    case "latency_ms":
        // latency_ms=347 → latency_bucket=200-500
        if ms, err := strconv.ParseInt(value, 10, 64); err == nil {
            for _, bound := range []int64{10, 50, 100, 200, 500, 1000, 5000} {
                if ms < bound {
                    return fmt.Sprintf("<%d", bound)
                }
            }
            return ">=5000"
        }
    }
    return value
}
```

**The UI: Cardinality dashboard**

```
┌─────────────────────────────────────────────────────────────┐
│  ⚠️ Cardinality Alert: http_requests_total                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Current series: 847,234 (limit: 1,000,000)                 │
│  Growth rate: +12,000/hour                                   │
│  Time to limit: ~12 hours                                    │
│                                                              │
│  High-cardinality labels:                                   │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  Label        Unique Values    Recommendation          │  │
│  │  ─────────────────────────────────────────────────────│  │
│  │  user_id      234,847          ⚠️ Remove or bucket     │  │
│  │  path         12,847           ⚠️ Use path templates   │  │
│  │  method       5                ✅ OK                   │  │
│  │  status       8                ✅ OK                   │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                              │
│  💡 Suggestion: Remove 'user_id' label                      │
│     Impact: Reduces to ~847 series (99.9% reduction)        │
│     Queries affected: None (label never used in dashboards) │
│                                                              │
│  [Apply Fix] [Ignore] [Set Up Alert Rule]                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Effort estimate:** 3-4 weeks

---

### 13. Hot Probe Reload

**The problem:** Updating eBPF probes requires restart. Restart = data gap.

```
Traditional flow:
1. New probe version ready
2. Stop dogwatch
3. Start dogwatch with new probes
4. Gap: 30-60 seconds of missing data
5. Users notice. Alerts fire. Panic.
```

**The moat: Zero-downtime probe updates**

```go
// Probe manager that supports hot reload
type ProbeManager struct {
    // Currently loaded probes
    active map[string]*LoadedProbe

    // Pending probes (being loaded)
    pending map[string]*LoadedProbe

    // BPF filesystem for pinning
    bpfFS string
}

type LoadedProbe struct {
    Name       string
    Version    string
    Collection *ebpf.Collection
    Links      []link.Link
    StartTime  time.Time
}

func (m *ProbeManager) HotReload(name string, newProbe []byte) error {
    old := m.active[name]

    // 1. Load new probe (doesn't attach yet)
    newCollection, err := ebpf.LoadCollectionSpecFromReader(bytes.NewReader(newProbe))
    if err != nil {
        return fmt.Errorf("failed to load new probe: %w", err)
    }

    // 2. Pin new maps to BPF filesystem (for persistence)
    for mapName, m := range newCollection.Maps {
        pinPath := filepath.Join(m.bpfFS, name, mapName)
        if err := m.Pin(pinPath); err != nil {
            return fmt.Errorf("failed to pin map %s: %w", mapName, err)
        }
    }

    // 3. Attach new probe (now collecting data)
    var newLinks []link.Link
    for progName, prog := range newCollection.Programs {
        l, err := m.attachProgram(prog, progName)
        if err != nil {
            // Rollback: close new links, keep old probe
            for _, l := range newLinks {
                l.Close()
            }
            return fmt.Errorf("failed to attach %s: %w", progName, err)
        }
        newLinks = append(newLinks, l)
    }

    // 4. Brief overlap: both probes collecting (dedup in userspace)
    m.pending[name] = &LoadedProbe{
        Name:       name,
        Version:    extractVersion(newProbe),
        Collection: newCollection,
        Links:      newLinks,
        StartTime:  time.Now(),
    }

    // 5. Drain old probe (wait for in-flight events)
    time.Sleep(100 * time.Millisecond)

    // 6. Detach old probe
    if old != nil {
        for _, l := range old.Links {
            l.Close()
        }
        old.Collection.Close()
    }

    // 7. Promote new probe to active
    m.active[name] = m.pending[name]
    delete(m.pending, name)

    log.Printf("Hot reloaded probe %s: %s → %s", name, old.Version, m.active[name].Version)
    return nil
}

// Map reuse: share maps between old and new probes
func (m *ProbeManager) reuseMaps(old, new *ebpf.CollectionSpec) {
    for mapName := range new.Maps {
        if oldMap, ok := old.Maps[mapName]; ok {
            // Reuse existing map (preserves data)
            pinPath := filepath.Join(m.bpfFS, mapName)
            if pinnedMap, err := ebpf.LoadPinnedMap(pinPath, nil); err == nil {
                new.Maps[mapName] = pinnedMap
            }
        }
    }
}
```

**Graceful version transition:**

```go
// During reload, both versions may send events
// Deduplicate based on event ID
type EventDeduplicator struct {
    // Recent event IDs (bloom filter for memory efficiency)
    seen *bloom.BloomFilter

    // Window for deduplication
    window time.Duration
}

func (d *EventDeduplicator) IsDuplicate(event *Event) bool {
    key := fmt.Sprintf("%d:%d:%d", event.PID, event.TID, event.Timestamp)

    if d.seen.Test([]byte(key)) {
        return true
    }

    d.seen.Add([]byte(key))
    return false
}
```

**API for remote reload:**

```go
// HTTP endpoint for triggering reload
func (s *Server) handleProbeReload(w http.ResponseWriter, r *http.Request) {
    probeName := r.URL.Query().Get("probe")

    // Option 1: Reload from embedded (new binary)
    if r.URL.Query().Get("source") == "embedded" {
        probeBytes := embeddedProbes[probeName]
        if err := s.probeManager.HotReload(probeName, probeBytes); err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
    }

    // Option 2: Reload from URL (remote update)
    if url := r.URL.Query().Get("url"); url != "" {
        probeBytes, err := fetchProbe(url)
        if err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        if err := s.probeManager.HotReload(probeName, probeBytes); err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
    }

    w.Write([]byte("OK"))
}
```

**Effort estimate:** 2-3 weeks

---

### 14. Clock Skew Tolerance

**The problem:** Distributed systems have clock drift. Traces look wrong when clocks disagree.

```
Reality:
  Host A clock: 10:00:00.000 (correct)
  Host B clock: 10:00:00.500 (500ms ahead)
  Host C clock: 09:59:59.200 (800ms behind)

What happened:
  A → B → C (sequential)

What trace shows with raw timestamps:
  C: 09:59:59.200 (appears first!)
  A: 10:00:00.000
  B: 10:00:00.500 (appears last!)

User sees: "C started before A even sent the request?!"
```

**The moat: Automatic clock skew detection and correction**

```go
// Clock skew detector using span relationships
type ClockSkewDetector struct {
    // Per-host estimated skew
    HostSkew map[string]*SkewEstimate

    // Reference host (assumed correct, or uses external NTP)
    ReferenceHost string
}

type SkewEstimate struct {
    Host      string
    Offset    time.Duration  // How far ahead/behind
    Confidence float64       // 0-1, based on sample count
    Samples   int
    LastUpdate time.Time
}

// Detect skew from parent-child span relationships
func (d *ClockSkewDetector) DetectSkew(parent, child *Span) {
    if parent.Host == child.Host {
        return  // Same host, no skew to detect
    }

    // Child should start AFTER parent starts
    // Child should end BEFORE parent ends (usually)

    // If child appears to start before parent sent request,
    // child's clock is behind
    expectedChildStart := parent.StartTime.Add(parent.NetworkLatency)
    observedChildStart := child.StartTime

    skew := observedChildStart.Sub(expectedChildStart)

    // Update estimate with exponential moving average
    d.updateSkew(child.Host, skew)
}

func (d *ClockSkewDetector) updateSkew(host string, observedSkew time.Duration) {
    est := d.HostSkew[host]
    if est == nil {
        est = &SkewEstimate{Host: host}
        d.HostSkew[host] = est
    }

    // Exponential moving average
    alpha := 0.1
    est.Offset = time.Duration(float64(est.Offset)*(1-alpha) + float64(observedSkew)*alpha)
    est.Samples++
    est.Confidence = math.Min(1.0, float64(est.Samples)/100)
    est.LastUpdate = time.Now()
}

// Apply correction when displaying traces
func (d *ClockSkewDetector) Correct(span *Span) *Span {
    corrected := *span

    if est, ok := d.HostSkew[span.Host]; ok && est.Confidence > 0.5 {
        corrected.StartTime = span.StartTime.Add(-est.Offset)
        corrected.EndTime = span.EndTime.Add(-est.Offset)
        corrected.ClockCorrected = true
        corrected.CorrectionApplied = est.Offset
    }

    return &corrected
}
```

**Causal ordering enforcement:**

```go
// Even after correction, enforce causal ordering
type TraceOrderer struct{}

func (o *TraceOrderer) EnforceOrdering(trace *Trace) *Trace {
    // Build parent-child relationships
    children := make(map[string][]*Span)
    for _, span := range trace.Spans {
        if span.ParentID != "" {
            children[span.ParentID] = append(children[span.ParentID], span)
        }
    }

    // Recursively fix ordering
    root := trace.RootSpan()
    o.fixOrdering(root, children)

    return trace
}

func (o *TraceOrderer) fixOrdering(parent *Span, children map[string][]*Span) {
    for _, child := range children[parent.SpanID] {
        // Child must start after parent starts
        if child.StartTime.Before(parent.StartTime) {
            child.StartTime = parent.StartTime.Add(1 * time.Microsecond)
            child.ClockAdjusted = true
        }

        // Child must end before parent ends (with some slack for async)
        if child.EndTime.After(parent.EndTime.Add(100 * time.Millisecond)) {
            // This might be legitimate async - flag but don't adjust
            child.MayBeAsync = true
        }

        // Recurse
        o.fixOrdering(child, children)
    }
}
```

**Effort estimate:** 2-3 weeks

---

### 15. Memory-Efficient Operation

**The problem:** Observability tools are memory hogs. Can't run on small instances.

```
Typical observability memory usage:
  Prometheus: 2-8 GB
  Elasticsearch: 4-32 GB
  Datadog Agent: 500 MB - 2 GB

dogwatch target:
  Base: 100-200 MB
  Per 1000 spans/sec: +50 MB
  Per 10000 metrics: +20 MB
```

**The moat: Aggressive memory optimization**

```go
// Object pooling to reduce GC pressure
var spanPool = sync.Pool{
    New: func() interface{} {
        return &Span{
            Tags: make(map[string]string, 8),  // Pre-allocate common size
        }
    },
}

func NewSpan() *Span {
    span := spanPool.Get().(*Span)
    // Reset fields
    span.TraceID = ""
    span.SpanID = ""
    span.ParentID = ""
    for k := range span.Tags {
        delete(span.Tags, k)
    }
    return span
}

func ReleaseSpan(span *Span) {
    spanPool.Put(span)
}

// String interning for repeated values
type StringInterner struct {
    mu      sync.RWMutex
    strings map[string]string
    size    int
    maxSize int
}

func (i *StringInterner) Intern(s string) string {
    i.mu.RLock()
    if interned, ok := i.strings[s]; ok {
        i.mu.RUnlock()
        return interned
    }
    i.mu.RUnlock()

    i.mu.Lock()
    defer i.mu.Unlock()

    // Double-check after acquiring write lock
    if interned, ok := i.strings[s]; ok {
        return interned
    }

    // Evict if too large
    if i.size >= i.maxSize {
        i.evictOldest()
    }

    i.strings[s] = s
    i.size++
    return s
}

// Use interning for common fields
func (p *Parser) parseSpan(data []byte) *Span {
    span := NewSpan()

    // These fields have low cardinality - intern them
    span.Service = p.interner.Intern(extractService(data))
    span.Operation = p.interner.Intern(extractOperation(data))

    // These fields are high cardinality - don't intern
    span.TraceID = extractTraceID(data)
    span.SpanID = extractSpanID(data)

    return span
}

// Streaming aggregation (don't buffer everything)
type StreamingAggregator struct {
    // Current bucket being aggregated
    currentBucket time.Time
    aggregates    map[string]*RunningStats

    // Flush completed buckets
    flushChan chan *CompletedBucket
}

type RunningStats struct {
    Count int64
    Sum   float64
    Min   float64
    Max   float64
    // For percentiles: t-digest or DDSketch (memory-bounded)
    Sketch *tdigest.TDigest
}

func (a *StreamingAggregator) Add(metric string, value float64, ts time.Time) {
    bucket := ts.Truncate(time.Minute)

    // Flush old bucket if we've moved on
    if bucket.After(a.currentBucket) {
        a.flush()
        a.currentBucket = bucket
    }

    // Update running stats (constant memory)
    stats := a.aggregates[metric]
    if stats == nil {
        stats = &RunningStats{
            Min:    math.MaxFloat64,
            Max:    -math.MaxFloat64,
            Sketch: tdigest.NewWithCompression(100),
        }
        a.aggregates[metric] = stats
    }

    stats.Count++
    stats.Sum += value
    stats.Min = math.Min(stats.Min, value)
    stats.Max = math.Max(stats.Max, value)
    stats.Sketch.Add(value, 1)
}

// Memory-mapped storage for large datasets
type MmapStorage struct {
    file *os.File
    mmap mmap.MMap
    size int64
}

func (s *MmapStorage) Read(offset, length int64) []byte {
    return s.mmap[offset : offset+length]
}

// Compaction to recover memory
func (s *Storage) Compact() error {
    // SQLite VACUUM
    _, err := s.db.Exec("VACUUM")
    return err
}

// Memory pressure response
type MemoryManager struct {
    limit     int64
    current   int64
    onPressure func()
}

func (m *MemoryManager) Monitor() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        var stats runtime.MemStats
        runtime.ReadMemStats(&stats)

        m.current = int64(stats.Alloc)

        if m.current > int64(float64(m.limit)*0.8) {
            // Memory pressure - take action
            log.Printf("Memory pressure: %d / %d", m.current, m.limit)

            // 1. Force GC
            runtime.GC()

            // 2. Clear caches
            m.onPressure()

            // 3. Increase sampling rate (ingest less)
            // 4. Drop oldest data
        }
    }
}
```

**Memory budget enforcement:**

```go
// Limit memory per component
type MemoryBudget struct {
    Total      int64
    Components map[string]int64
}

var DefaultBudget = MemoryBudget{
    Total: 500 * 1024 * 1024,  // 500 MB total
    Components: map[string]int64{
        "traces":     100 * 1024 * 1024,
        "metrics":    100 * 1024 * 1024,
        "logs":       100 * 1024 * 1024,
        "indexes":    50 * 1024 * 1024,
        "cache":      50 * 1024 * 1024,
        "buffers":    50 * 1024 * 1024,
        "overhead":   50 * 1024 * 1024,
    },
}
```

**Effort estimate:** 3-4 weeks for comprehensive optimization

---

### Summary: Technical Moat Investment (Updated)

| Moat Area | Effort | Defensibility | Priority |
|-----------|--------|---------------|----------|
| **Kernel compatibility** | 4-6 weeks | High | P1 - Do early |
| **TLS interception** | 6-8 weeks | Very High | P1 - Core feature |
| **Protocol edge cases** | 8-12 weeks | High | P0 - This IS the product |
| **Correlation accuracy** | 4-6 weeks | High | P0 - This IS the product |
| **Continuous profiling** | 2-3 weeks (integrate) | Medium | P2 - Can use Pyroscope |
| **Security false positives** | 4-6 weeks | High | P2 - After core works |
| **Migration fidelity** | 4-6 weeks | Very High | P1 - Your unique advantage |
| **Intelligent sampling** | 3-4 weeks | High | P1 - Required for scale |
| **Log field extraction** | 3-4 weeks | Medium | P2 - Nice to have |
| **Multi-signal correlation** | 4-5 weeks | High | P1 - Key differentiator |
| **SQLite optimization** | 4-6 weeks | High | P1 - Performance is table stakes |
| **Cardinality prevention** | 3-4 weeks | High | P1 - Saves users from themselves |
| **Hot probe reload** | 2-3 weeks | Medium | P2 - Nice for operations |
| **Clock skew tolerance** | 2-3 weeks | Medium | P2 - Polish |
| **Memory efficiency** | 3-4 weeks | High | P1 - Run on small instances |

**Total investment for deep technical moats: 55-85 weeks**

This isn't feature work. This is the hard engineering that makes competitors say "we'll just integrate with something else" instead of building it themselves.

---

## Competitive Weaknesses to Exploit

Every established player has pain points they can't easily fix due to architecture decisions, business model constraints, or organizational inertia. These are opportunities for dogwatch to iterate faster and win.

---

### Pixie (New Relic) Weaknesses

**Background:** Pixie was the eBPF tracing pioneer. Acquired by New Relic in 2021. Now tightly coupled to New Relic's ecosystem.

#### 1. Kubernetes-Only

```
Pixie's limitation:
├── ONLY works on Kubernetes
├── No bare metal support
├── No VM support (EC2, GCE without K8s)
├── No Docker Compose / Swarm
└── No edge devices / IoT

Reality check:
├── 40% of workloads still run on VMs
├── Many companies have mixed environments
├── Edge computing is growing
└── Not everyone is on K8s yet
```

**dogwatch advantage:**
```go
// Works everywhere Linux runs
func main() {
    // Detect environment automatically
    switch detectEnvironment() {
    case "kubernetes":
        enrichWithK8sMetadata()
    case "docker":
        enrichWithDockerMetadata()
    case "systemd":
        enrichWithSystemdMetadata()
    case "bare_metal":
        enrichWithHostMetadata()
    }

    // Same eBPF probes work everywhere
    startProbes()
}
```

**Iteration opportunity:**
- First-class support for non-K8s environments
- "Works on your $5 VPS" messaging
- Target the 40% not on Kubernetes
- Edge/IoT deployments (single binary is perfect)

#### 2. No Long-Term Retention

```
Pixie's architecture:
├── Data stays in-cluster (privacy feature)
├── Stored in memory only
├── Default retention: ~24 hours
├── Can't query last week's data
└── No historical analysis

Why they can't fix it:
├── "Data never leaves your cluster" is their pitch
├── Adding storage breaks the simplicity
├── New Relic wants you to export to their cloud ($$$)
└── Architectural decision baked in
```

**dogwatch advantage:**
```go
// SQLite = persistent by default
// Can query data from months ago
// Simple retention policies

type RetentionConfig struct {
    Traces   time.Duration  // default: 7 days
    Metrics  time.Duration  // default: 30 days
    Logs     time.Duration  // default: 14 days
}

// "What was happening last Tuesday at 3am?"
// Pixie: "Sorry, that data is gone"
// dogwatch: "Here you go"
```

**Iteration opportunity:**
- "Keep your data as long as you want"
- Historical analysis features (compare this week vs last week)
- Long-term trend detection
- Compliance requirements (some need 90+ days retention)

#### 3. Complex Query Language (PxL)

```python
# Pixie's PxL - it's... different
import px

df = px.DataFrame('http_events')
df = df[df['service'] == 'api-server']
df = df.groupby('req_path').agg(
    latency_p99=('latency', px.quantiles, 0.99),
    count=('latency', px.count),
)
px.display(df)
```

```
Problems:
├── Python-like but not Python
├── Can't use in dashboards easily
├── Learning curve for non-Python devs
├── No PromQL compatibility
├── Hard to share queries
└── IDE support is limited
```

**dogwatch advantage:**
```sql
-- DQL: Familiar SQL-like syntax
SELECT
    req_path,
    percentile(latency, 0.99) as p99,
    count(*) as count
FROM http_events
WHERE service = 'api-server'
GROUP BY req_path

-- Or use PromQL for metrics (everyone knows it)
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))
```

**Iteration opportunity:**
- PromQL compatibility (people already know it)
- SQL-like query language (everyone knows SQL)
- Copy-paste queries from Stack Overflow
- Better IDE/editor support

#### 4. Resource Hungry

```
Pixie resource usage:
├── Vizier (control plane): 2+ GB RAM
├── PEM (per-node agent): 500MB-2GB RAM per node
├── Requires dedicated resources
└── Doesn't work on small instances

Complaints from users:
├── "Pixie OOMKilled on my t3.small nodes"
├── "Had to add dedicated node pool just for Pixie"
├── "Using 20% of my cluster resources for observability"
```

**dogwatch advantage:**
```
dogwatch resource usage:
├── Base: 100-200 MB RAM
├── Scales with data volume, not node count
├── Works on t3.micro
├── No dedicated resources needed
└── Single process, not distributed system
```

**Iteration opportunity:**
- "Runs on a Raspberry Pi" marketing
- Cost-conscious customers (small clusters)
- Edge deployments where resources are limited
- Development environments

#### 5. New Relic Lock-In

```
Post-acquisition reality:
├── Pixie UI being deprecated
├── "Export to New Relic" is the path forward
├── Open source version getting less love
├── Community contributions slowing
└── Feature development focused on NR integration
```

**dogwatch advantage:**
- Truly independent, no corporate parent
- No hidden agenda to upsell
- Community-focused development
- Your data stays yours

---

### Grafana Stack Weaknesses

**Background:** Grafana Labs offers Loki (logs), Tempo (traces), Mimir (metrics), plus Grafana for visualization. Industry standard but operationally complex.

#### 1. Five Components to Deploy

```
Grafana Stack components:
├── Grafana (visualization)
├── Loki (logs)
├── Tempo (traces)
├── Mimir or Prometheus (metrics)
├── Grafana Agent or OTel Collector
└── Often also: Alertmanager, various exporters

Each component:
├── Has its own configuration
├── Has its own scaling concerns
├── Has its own storage backend
├── Has its own upgrade path
├── Can fail independently
└── Needs monitoring itself
```

```yaml
# Grafana stack Helm values - this is the SIMPLE version
grafana:
  enabled: true
  persistence:
    enabled: true
    size: 10Gi

loki:
  enabled: true
  persistence:
    enabled: true
    size: 50Gi
  config:
    chunk_store_config:
      max_look_back_period: 168h

tempo:
  enabled: true
  persistence:
    enabled: true
    size: 50Gi

mimir:
  enabled: true
  # ... 200 more lines of config
```

**dogwatch advantage:**
```bash
# dogwatch deployment
curl -L https://dogwatch.dev/install.sh | sh
./dogwatch

# That's it. One binary. One config file. Done.
```

**Iteration opportunity:**
- "One binary vs five services" messaging
- Time-to-value: 60 seconds vs hours
- Operational simplicity
- No "which component is broken?" debugging

#### 2. Three Query Languages

```
Grafana query languages:
├── PromQL for metrics
│   rate(http_requests_total{status="500"}[5m])
│
├── LogQL for logs
│   {service="api"} |= "error" | json | duration > 1s
│
└── TraceQL for traces
    {resource.service.name="api" && duration > 1s}

Problems:
├── Learn three syntaxes
├── Can't join across signals easily
├── Copy-paste between panels doesn't work
├── Documentation spread across three places
└── Mental context switching
```

**dogwatch advantage:**
```sql
-- One language for everything (DQL)

-- Logs
SELECT * FROM logs WHERE service = 'api' AND level = 'error'

-- Metrics
SELECT avg(value) FROM metrics
WHERE name = 'http_requests_total'
GROUP BY time_bucket('1m', timestamp)

-- Traces
SELECT * FROM traces WHERE service = 'api' AND duration > 1s

-- CROSS-SIGNAL JOIN (the magic)
SELECT
    t.trace_id,
    t.duration,
    l.message as error_message,
    m.value as concurrent_requests
FROM traces t
LEFT JOIN logs l ON l.trace_id = t.trace_id AND l.level = 'error'
LEFT JOIN metrics m ON m.timestamp BETWEEN t.start_time AND t.end_time
WHERE t.status = 'error'
```

**Iteration opportunity:**
- Single query language
- Cross-signal queries (join logs + traces + metrics)
- Familiar SQL-like syntax
- One set of documentation

#### 3. Correlation Is Manual

```
Grafana correlation workflow:
1. See spike in metrics dashboard
2. Mentally note the time range
3. Switch to Logs dashboard
4. Manually enter the same time range
5. Search for relevant logs
6. Find a trace ID in logs
7. Switch to Traces dashboard
8. Paste trace ID
9. Finally see the trace

Total clicks: ~15
Time: 2-5 minutes
User experience: Frustrating
```

**dogwatch advantage:**
```
dogwatch correlation workflow:
1. See spike in metrics
2. Click on spike
3. See correlated logs and traces immediately

Total clicks: 2
Time: 3 seconds
User experience: Magic
```

**Iteration opportunity:**
- Automatic correlation is THE feature
- Click-through from any signal to related data
- Unified timeline view
- "Show me everything related to this"

#### 4. Complex Storage Configuration

```yaml
# Loki storage config - and this is just ONE component
loki:
  storage:
    type: s3
    bucketNames:
      chunks: loki-chunks
      ruler: loki-ruler
      admin: loki-admin
    s3:
      endpoint: s3.amazonaws.com
      region: us-east-1
      accessKeyId: ${AWS_ACCESS_KEY_ID}
      secretAccessKey: ${AWS_SECRET_ACCESS_KEY}
      s3ForcePathStyle: false
    boltdb_shipper:
      active_index_directory: /loki/index
      shared_store: s3
      cache_ttl: 24h

# Now repeat for Tempo...
# Now repeat for Mimir...
# Now configure retention for each...
# Now configure compaction for each...
```

**dogwatch advantage:**
```yaml
# dogwatch config
storage:
  path: /var/lib/dogwatch  # SQLite, done
  retention:
    traces: 7d
    metrics: 30d
    logs: 14d
```

**Iteration opportunity:**
- Zero storage configuration
- Works out of the box
- Retention just works
- No S3/GCS/Azure setup required

#### 5. No Auto-Discovery

```
Grafana setup for a new service:
1. Add service to scrape config (or deploy exporter)
2. Reload Prometheus/Agent
3. Create dashboard for service
4. Create alert rules for service
5. Add service to service catalog (if you have one)
6. Configure log labels for service
7. Set up trace sampling for service

For 50 services: 50 × above = weeks of work
```

**dogwatch advantage:**
```
dogwatch setup for a new service:
1. Deploy the service
2. Done. dogwatch sees it automatically via eBPF.

For 50 services: deploy them. dogwatch handles the rest.
```

**Iteration opportunity:**
- Zero configuration for new services
- Automatic service discovery
- Automatic dashboard generation
- "It just works" experience

---

### OpenTelemetry Weaknesses

**Background:** OTel is the standard for instrumentation. But it's ONLY instrumentation - no storage, query, or visualization.

#### 1. Instrumentation Still Required

```
"Zero-code instrumentation" reality:

Java:
  java -javaagent:opentelemetry-javaagent.jar -jar app.jar
  # Plus: set OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_SERVICE_NAME, etc.

Python:
  opentelemetry-instrument python app.py
  # Plus: pip install opentelemetry-instrumentation-*
  # Plus: configure exporters

Node:
  node --require @opentelemetry/auto-instrumentations-node app.js
  # Plus: npm install 15 packages
  # Plus: configure

Go:
  # No auto-instrumentation. Manual only.
  # Add tracing to every function you care about.
```

```
Still required:
├── Modify deployment (add agent/flags)
├── Set environment variables
├── Install language-specific packages
├── Rebuild/redeploy containers
├── Configure exporters
└── Handle per-language quirks
```

**dogwatch advantage:**
```
eBPF instrumentation:
├── No code changes
├── No rebuilds
├── No redeployment
├── No per-language setup
├── Works on binaries you don't have source for
└── Deploy dogwatch, see everything
```

**Iteration opportunity:**
- "Truly zero-config" vs OTel's "mostly zero-config"
- Works with languages OTel doesn't auto-instrument well (Go, Rust)
- Works with third-party binaries
- No coordination needed across teams

#### 2. Backend Fragmentation

```
OTel gives you data. Now you need:
├── Somewhere to store traces (Jaeger? Tempo? Zipkin? X-Ray?)
├── Somewhere to store metrics (Prometheus? Mimir? Datadog?)
├── Somewhere to store logs (Loki? Elasticsearch? CloudWatch?)
├── Something to visualize (Grafana? Custom? Vendor UI?)
├── Something to alert (Alertmanager? PagerDuty integration?)
└── Something to correlate (???)

OTel explicitly does NOT solve:
├── Storage
├── Querying
├── Visualization
├── Alerting
└── Correlation
```

**dogwatch advantage:**
- All-in-one: collect, store, query, visualize, alert
- Plus we accept OTLP, so you get the best of both worlds
- No backend decision paralysis

**Iteration opportunity:**
- "OTel + dogwatch = complete solution"
- Accept OTLP for teams that have it
- eBPF fills gaps for teams that don't
- One tool instead of five

#### 3. Configuration Complexity

```yaml
# OTel Collector config - this is a "simple" example
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    timeout: 1s
    send_batch_size: 1024
  memory_limiter:
    check_interval: 1s
    limit_mib: 2000
  resource:
    attributes:
      - key: environment
        value: production
        action: upsert

exporters:
  otlp:
    endpoint: tempo:4317
    tls:
      insecure: true
  prometheus:
    endpoint: 0.0.0.0:8889
  loki:
    endpoint: http://loki:3100/loki/api/v1/push

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlp]
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [prometheus]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [loki]
```

**dogwatch advantage:**
```yaml
# dogwatch config
listen: :9999
```

**Iteration opportunity:**
- Sensible defaults that work
- Configuration optional, not required
- "It just works" out of the box

---

### Datadog Weaknesses

**Background:** Market leader, but expensive and closed.

#### 1. Pricing Trap

```
Datadog pricing reality:
├── Infrastructure: $15-23/host/month
├── APM: $31-40/host/month
├── Logs: $0.10/GB ingested + $1.70/million events
├── Custom metrics: $0.05 per 100 metrics
├── Synthetics: $5-12 per 10K runs
└── Everything adds up FAST

Real example:
├── 100 hosts with APM: $4,000/month
├── 500GB logs: $50/month + events
├── Custom metrics (50K): $25/month
├── Synthetics: $500/month
└── Total: $5,000+/month
└── Annual: $60,000+

And that's BEFORE:
├── Log retention beyond 15 days
├── High cardinality metrics
├── Trace retention
└── Additional users
```

**dogwatch advantage:**
```
dogwatch pricing:
├── Self-hosted: $0
├── Everything included
├── No per-host fees
├── No per-GB fees
├── No surprise bills
└── Your hardware, your cost
```

**Iteration opportunity:**
- Cost Intelligence showing "you'd pay $X on Datadog"
- Flat pricing (if you have a paid tier)
- "Migration savings" calculator
- Target companies with Datadog bill shock

#### 2. SaaS Only

```
Datadog limitations:
├── Data must leave your infrastructure
├── No on-premises option
├── No air-gapped deployment
├── Subject to their data retention policies
├── Dependent on their uptime
└── Compliance concerns for some industries
```

**dogwatch advantage:**
- Self-hosted: data never leaves
- Air-gapped deployment possible
- Full control over retention
- No dependency on external service
- Compliance-friendly (data sovereignty)

**Iteration opportunity:**
- Target regulated industries (finance, healthcare, government)
- Target security-conscious companies
- Target companies with data residency requirements
- "Your data stays yours" messaging

#### 3. SDK Required for APM

```
Datadog APM setup:
├── Install dd-trace-* for your language
├── Modify application startup
├── Set DD_* environment variables
├── Rebuild and redeploy
├── Configure sampling
├── Repeat for every service
└── Coordinate across teams
```

**dogwatch advantage:**
- eBPF sees everything without SDKs
- No code changes
- No team coordination
- Works immediately

---

### Iteration Opportunities Summary

**Where to focus for maximum competitive advantage:**

| Weakness | Who Has It | dogwatch Opportunity |
|----------|-----------|---------------------|
| **K8s-only** | Pixie | Support VMs, bare metal, edge |
| **No retention** | Pixie | Long-term storage, historical queries |
| **Complex query languages** | Pixie, Grafana | Single SQL-like language |
| **Resource hungry** | Pixie, Grafana | Run on t3.micro |
| **Many components** | Grafana | Single binary |
| **Manual correlation** | Grafana, most | Automatic cross-signal correlation |
| **Complex configuration** | Grafana, OTel | Sensible defaults, zero-config |
| **No auto-discovery** | Grafana, OTel | eBPF sees everything |
| **Still needs instrumentation** | OTel | True zero-code with eBPF |
| **Expensive** | Datadog | Self-hosted, flat pricing |
| **SaaS only** | Datadog | Self-hosted, air-gapped |
| **SDK required** | Datadog | eBPF, no SDK |

---

### Technical Iterations to Prioritize

Based on competitive weaknesses, prioritize these iterations:

#### Tier 1: Immediate Differentiation

| Iteration | Exploits Weakness In | Effort |
|-----------|---------------------|--------|
| **Non-K8s support** | Pixie | 2-3 weeks |
| **Single binary simplicity** | Grafana | Already done |
| **Auto-correlation** | Grafana, all | 4-5 weeks |
| **Low memory footprint** | Pixie, Grafana | 3-4 weeks |
| **Long-term retention** | Pixie | Already have (SQLite) |

#### Tier 2: Strong Differentiation

| Iteration | Exploits Weakness In | Effort |
|-----------|---------------------|--------|
| **Unified query language** | Grafana, Pixie | 4-6 weeks |
| **Zero-config auto-discovery** | Grafana, OTel | Core feature |
| **Cross-signal joins** | Everyone | 3-4 weeks |
| **Cost Intelligence** | Datadog | 1-2 weeks |
| **Migration tooling** | Everyone | 4-6 weeks |

#### Tier 3: Long-Term Moats

| Iteration | Exploits Weakness In | Effort |
|-----------|---------------------|--------|
| **Air-gapped deployment** | Datadog | 1-2 weeks |
| **Edge device support** | Pixie | 2-3 weeks |
| **Compliance features** | Datadog | 4-6 weeks |
| **PromQL compatibility** | None (table stakes) | 3-4 weeks |

---

### Messaging Against Each Competitor

**vs Pixie:**
> "Like Pixie, but works everywhere Linux runs—not just Kubernetes. Plus you keep your data forever, not just 24 hours."

**vs Grafana Stack:**
> "All of Grafana's capabilities in a single binary. No Loki + Tempo + Mimir + Grafana + Agent. Just dogwatch."

**vs OpenTelemetry:**
> "OTel still needs instrumentation. dogwatch sees everything via eBPF—even services you can't modify."

**vs Datadog:**
> "See what you're paying Datadog for. Then see the same data in dogwatch for free. We'll even migrate your dashboards."

---

## Roadmap

> **📋 AUTHORITATIVE PRIORITIES:** See [Summary: Cost vs Impact Matrix](#summary-cost-vs-impact-matrix) for the definitive build order. Other priority lists in this document provide context but defer to that matrix.

### Phase 1: Foundation (Now)

**Goal:** Production-ready basics. Can't demo without these.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| Health endpoints | `/healthz`, `/readyz` for K8s | Table stakes | 🎯 Must have |
| Graceful shutdown | Don't drop in-flight requests | Table stakes | 🎯 Must have |
| OTLP receiver | Accept OpenTelemetry data (gRPC + HTTP) | Table stakes | 🎯 Must have |
| Prometheus remote_write | Receive metrics from Prometheus | Chronosphere | 🎯 Must have |
| Basic retention | Per-signal retention policies | Table stakes | 🎯 Must have |
| Config validation | `dogwatch config validate` | Table stakes | 🎯 Must have |

**Success criteria:**
- Deploys cleanly on K8s with proper health checks
- Receives OTLP + Prometheus data
- Doesn't lose data on restart

### Phase 2: Zero-Config Magic (The Pixie Playbook)

**Goal:** Install → see everything in 60 seconds. This is the demo that sells.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| MySQL protocol parsing | Capture queries via eBPF | Pixie | 🎯 Priority |
| PostgreSQL protocol parsing | Capture queries via eBPF | Pixie | 🎯 Priority |
| Redis protocol parsing | Capture commands via eBPF | Pixie | 🎯 Priority |
| HTTP/2 + gRPC parsing | Modern microservices | Pixie | 🎯 Priority |
| DNS parsing | Often-hidden latency source | Pixie | 🎯 Priority |
| HTTPS/TLS interception | SSL uprobe for encrypted traffic | Pixie | 🔧 In Progress |
| Cross-service correlation | Link requests into distributed traces | Pixie | 🎯 Priority |
| K8s auto-discovery | Auto-detect pods, services, deployments | Pixie | 🎯 Priority |
| K8s auto-labeling | Every trace tagged with pod/namespace/etc | Pixie | 🎯 Priority |
| Service map | Auto-generate topology from traffic | Pixie | 🎯 Priority |

**Success criteria:**
- Demo: `kubectl apply -f dogwatch.yaml` → see distributed traces in 60 seconds
- Demo: Service map shows all services + dependencies automatically
- Demo: Click on service → see all database queries it makes

### Phase 3: Pre-Built Scripts (The Pixie Differentiator)

**Goal:** One command → instant analysis. Users productive immediately.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| Script engine | Execute pre-built analyses | Pixie | 🎯 Priority |
| HTTP scripts | `dogwatch run http/slow_requests` | Pixie | 🎯 Priority |
| Database scripts | `dogwatch run mysql/slow_queries` | Pixie | 🎯 Priority |
| K8s scripts | `dogwatch run k8s/pod_restarts` | Pixie | 🎯 Priority |
| Security scripts | `dogwatch run security/outbound_connections` | Pixie | 🎯 Priority |
| Script library | 20+ pre-built scripts | Pixie | 🎯 Priority |
| Custom scripts | User-defined analyses | Pixie | 📋 Planned |

**Success criteria:**
- Demo: `dogwatch run mysql/slow_queries` → instant results
- Demo: 20+ scripts available out of the box
- Demo: User can write custom script in 5 minutes

### Phase 4: Continuous Profiling (Complete Performance Picture)

**Goal:** Answer "why is this slow?" with CPU/memory profiles.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| CPU profiling | eBPF-based stack sampling | Pixie | 🎯 Priority |
| Flamegraph visualization | Interactive flamegraphs | Pixie | 🎯 Priority |
| Profile → trace linking | Click profile hotspot → see traces | Pixie | 🎯 Priority |
| Memory profiling | Allocation tracking | Pixie | 📋 Planned |
| Profile storage | Query historical profiles | Pixie | 📋 Planned |

**Success criteria:**
- Demo: See CPU flamegraph for any service, any time range
- Demo: Click hotspot → "These 3 traces spent 40% of CPU in json.Marshal"

### Phase 5: Control Plane (The Chronosphere $3.35B Feature)

**Goal:** Show what's wasted. Save customers 70% on observability costs.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| Usage analytics | Track what's queried vs never used | Chronosphere | 🎯 Priority |
| Cardinality explorer | Find labels exploding cardinality | Chronosphere | 🎯 Priority |
| Cost calculator | Show Datadog/NR equivalent pricing | Original | 🎯 Priority |
| Cost attribution | Per-team/service cost breakdown | Chronosphere | 🎯 Priority |
| Shaping rules UI | Drop/aggregate metrics at ingest | Chronosphere | 🎯 Priority |
| Recommendations engine | "Drop these 847 unused metrics" | Chronosphere | 🎯 Priority |
| Impact preview | "If we drop this, what breaks?" | Chronosphere | 🎯 Priority |
| Team quotas | Hard limits per team | Chronosphere | 📋 Planned |

**Success criteria:**
- Demo: "80% of your metrics are never queried"
- Demo: "Dropping pod_id label reduces cardinality by 99%"
- Demo: "This would cost $47K/month on Datadog, you're saving $46K"

### Phase 6: Investigation & Intelligence

**Goal:** Make dogwatch answer "why?" automatically.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| BubbleUp analysis | Statistical root cause detection | Honeycomb | 🎯 Priority |
| Change tracking | Ingest deploys, commits, config changes | Original | 🎯 Priority |
| Change correlation | Auto-correlate changes with incidents | Original | 🎯 Priority |
| GitHub/GitLab webhooks | Track deploys and commits | Original | 🎯 Priority |
| Error grouping | Sentry-like error aggregation | Table stakes | 🎯 Priority |
| Security baseline | Learn normal per-container behavior | Original | 🎯 Priority |
| Security detection | Shell-in-container, unusual network | Original | 🎯 Priority |

**Success criteria:**
- Demo: Select slow requests → "98% hit db_shard=7"
- Demo: Incident shows "started 4 min after deploy v2.4.2"
- Demo: Alert on "shell spawned in container that never runs shells"

### Phase 7: Migration & Production Hardening

**Goal:** Zero-friction switching. Stable enough to replace Datadog.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| Full PromQL support | 100% Prometheus compatibility | Chronosphere | 🎯 Priority |
| Datadog importer | Import dashboards, alerts, SLOs | Original | 🎯 Priority |
| Grafana importer | Import dashboards, alert rules | Original | 🎯 Priority |
| Query translator | Convert Datadog queries → PromQL | Original | 📋 Planned |
| Backup/restore | `dogwatch backup`, `dogwatch restore` | Table stakes | 🎯 Priority |
| Basic HA | Active-passive with shared storage | Table stakes | 🎯 Priority |
| Sampling | Head sampling with priority rules | Table stakes | 🎯 Priority |
| Recording rules | Pre-compute expensive queries | Chronosphere | 📋 Planned |

**Success criteria:**
- Demo: Scan Datadog → "23 dashboards importable, $564K/year savings"
- Demo: Prometheus queries work identically
- Demo: Backup + restore works

### Phase 8: Enterprise Features

**Goal:** Unlock larger customers and paid tiers.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| Pluggable storage | VictoriaMetrics, ClickHouse, S3 | Original | 📋 Planned |
| SAML/OIDC/SCIM | Enterprise SSO | Chronosphere | 📋 Planned |
| Advanced RBAC | Fine-grained permissions | Chronosphere | 📋 Planned |
| Multi-tenancy | Isolated organizations | Chronosphere | 📋 Planned |
| Audit logging | Who did what when | Compliance | 📋 Planned |
| Helm chart + Operator | K8s-native deployment | Table stakes | 📋 Planned |
| Edge mode | Data stays in cluster (Pixie-style) | Pixie | 📋 Planned |

### Phase 9: Growth Features

**Goal:** Features that create stickiness.

| Feature | Description | Source | Status |
|---------|-------------|--------|--------|
| AI investigation | Natural language queries | Trend | 📋 Planned |
| Auto post-mortems | Generate from incident timeline | Nice-to-have | 📋 Planned |
| Runbooks | Step-by-step response guides | Nice-to-have | 📋 Planned |
| Slack bot | Incident management in chat | Nice-to-have | 📋 Planned |
| Service catalog | Ownership, dependencies, SLOs | Nice-to-have | 📋 Planned |
| Dynamic logging | Add logs to running code (Pixie) | Pixie | 📋 Planned |

---

### Roadmap Summary: What Makes Us Special

| Phase | Core Idea | Inspiration |
|-------|-----------|-------------|
| 1-2 | **Zero-config magic** - Install → see everything | Pixie |
| 3-4 | **Instant answers** - Scripts + profiling | Pixie |
| 5 | **Cost control** - Show waste, save 70% | Chronosphere |
| 6 | **Auto-investigation** - Why is this broken? | Honeycomb |
| 7 | **Easy migration** - Switch from Datadog painlessly | Original |
| 8 | **Enterprise ready** - SSO, RBAC, compliance | Chronosphere |

**The pitch progression:**
1. "Install in 60 seconds, see everything" (Pixie magic)
2. "Here's exactly what's slow and why" (Profiling + BubbleUp)
3. "You're wasting $40K/month on unused metrics" (Chronosphere control plane)
4. "Import your Datadog dashboards, save $500K/year" (Migration)
5. "Enterprise-ready with SSO, audit logs, compliance" (Chronosphere enterprise)

---

## Security Audit

### Current Security Status: ⚠️ NEEDS WORK

The codebase has some good practices but several gaps that need addressing before production use.

### ✅ What's Good

| Area | Status | Details |
|------|--------|---------|
| **Password Hashing** | ✅ Secure | bcrypt with cost 12 |
| **JWT Implementation** | ✅ Decent | HMAC-SHA256, token revocation, expiry checks |
| **SQL Injection** | ✅ Mostly Safe | Parameterized queries with `?` placeholders |
| **XSS in Templates** | ✅ Safe | Uses `html/template` with auto-escaping |
| **Rate Limiting** | ✅ Good | Memory-bounded, cleanup goroutine |
| **RBAC** | ✅ Good | Role hierarchy, permission checks |
| **Session Management** | ✅ Decent | Secure random tokens, expiry |
| **API Key Hashing** | ✅ Good | SHA-256 hashed before storage |

### ❌ Critical Issues

#### 1. No CSRF Protection
```
Status: MISSING
Risk: HIGH

Any site can make authenticated requests on behalf of logged-in users.

Fix needed:
- Add CSRF tokens to all state-changing forms
- Validate Origin/Referer headers
- Use SameSite=Strict cookies
```

#### 2. No Security Headers
```
Status: MISSING
Risk: MEDIUM-HIGH

Missing headers:
- Content-Security-Policy
- X-Frame-Options
- X-Content-Type-Options
- Strict-Transport-Security
- X-XSS-Protection (legacy but still useful)

Fix:
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Content-Security-Policy", "default-src 'self'")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000")
        next.ServeHTTP(w, r)
    })
}
```

#### 3. TLS Certificate Verification Disabled
```
Location: internal/notify/webhook.go:108
Status: DANGEROUS

InsecureSkipVerify: true allows MITM attacks on webhook notifications.

Fix: Make configurable, default to secure. Only allow in dev/test.
```

#### 4. No Request Body Size Limits
```
Status: MISSING
Risk: MEDIUM

Large request bodies can cause OOM.

Fix: r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024) // 10MB
```

### ⚠️ Medium Issues

| Issue | Location | Risk | Fix |
|-------|----------|------|-----|
| JWT secret random if not set | sso_handlers.go | Medium | Refuse to start without explicit secret |
| Session cookie settings unverified | - | Medium | Ensure HttpOnly, Secure, SameSite=Strict |
| Minimal input validation | API handlers | Medium | Add go-playground/validator |
| Error info leakage | Various | Low | Sanitize error messages |
| No account lockout | auth.go | Medium | Lock after N failed attempts |

### 📋 Security Hardening Checklist

**Before Beta:**
- [ ] Add CSRF protection
- [ ] Add security headers middleware
- [ ] Fix InsecureSkipVerify defaults
- [ ] Add request body size limits
- [ ] Audit session cookie settings
- [ ] Add input validation

**Before Production:**
- [ ] Professional security audit
- [ ] Penetration testing
- [ ] `govulncheck ./...` for dependency vulnerabilities
- [ ] Rate limit auth endpoints specifically
- [ ] Account lockout after failed attempts
- [ ] Audit logging for security events

**For Enterprise:**
- [ ] SOC 2 compliance review
- [ ] Encryption at rest
- [ ] Key rotation
- [ ] IP allowlisting
- [ ] MFA support
- [ ] Session invalidation on password change

### Security Score: 6/10

**Good foundation, but not production-ready without fixes.**

The bcrypt, JWT, and RBAC implementations are solid. Main gaps are web security basics (CSRF, headers) that are straightforward to fix (~2-3 days of work).

---

## Engineering Cost Estimates

Reality check: What does it actually take to build these features?

### Feature Cost Breakdown

#### 1. Zero-Config Distributed Tracing (eBPF Protocol Parsing)

| Component | Effort | Complexity | Notes |
|-----------|--------|------------|-------|
| HTTP/1.1 (improve) | 1 week | Medium | Already have basic version |
| MySQL protocol | 2 weeks | Medium | Packet parsing, query extraction |
| PostgreSQL protocol | 2 weeks | Medium | Similar to MySQL |
| Redis protocol | 1 week | Low | Simple RESP protocol |
| HTTPS/TLS interception | 2-3 weeks | High | SSL uprobes, library detection |
| HTTP/2 framing | 3-4 weeks | High | Multiplexing, HPACK compression |
| gRPC parsing | 2 weeks | High | HTTP/2 + protobuf |
| DNS parsing | 1 week | Low | Simple UDP protocol |
| Kafka protocol | 2-3 weeks | High | Complex binary protocol |
| Cross-service correlation | 2-3 weeks | High | Timing + header parsing |
| Service map | 1-2 weeks | Medium | Graph from connections |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 6 weeks | MySQL + PostgreSQL + Redis |
| **Solid** | 12 weeks | + HTTPS + correlation + service map |
| **Full** | 20 weeks | + HTTP/2 + gRPC + Kafka + DNS |

---

#### 2. Pre-Built Scripts (Pixie-style)

| Component | Effort | Notes |
|-----------|--------|-------|
| Script engine | 2 weeks | YAML-based, not custom language |
| CLI integration | 1 week | `dogwatch run script_name` |
| 10 core scripts | 2 weeks | HTTP, MySQL, PG, Redis, K8s |
| Custom script support | 2 weeks | User-defined scripts |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 4 weeks | Engine + 10 scripts |
| **Full** | 7 weeks | + custom scripts + 20 more |

---

#### 3. Continuous Profiling

| Component | Effort | Notes |
|-----------|--------|-------|
| eBPF stack sampling | 2 weeks | perf_event hooks |
| Symbol resolution | 2-3 weeks | DWARF, Go, JIT - this is HARD |
| Flamegraph UI | 2 weeks | Interactive SVG |
| Profile storage | 1 week | Efficient storage |
| Profile → trace linking | 1-2 weeks | Correlate by time |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Integrate Pyroscope** | 2-3 weeks | Full profiling, less control |
| **Build from scratch** | 10 weeks | Full control, much harder |

**Recommendation:** Integrate Pyroscope. Symbol resolution alone is a 3-week rabbit hole.

---

#### 4. Cost Intelligence

| Component | Effort | Notes |
|-----------|--------|-------|
| Usage tracking | 3 days | Count hosts, metrics, spans |
| Datadog pricing | 2 days | Implement their formula |
| New Relic pricing | 2 days | Usage-based |
| UI dashboard | 1 week | Display + comparison |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 1 week | Datadog comparison only |
| **Full** | 3 weeks | All competitors + export |

**This is the highest ROI feature.** 1 week of work = viral screenshots.

---

#### 5. Control Plane (Chronosphere-style)

| Component | Effort | Notes |
|-----------|--------|-------|
| Usage analytics | 2 weeks | Track queries per metric |
| Cardinality explorer | 2 weeks | Drill down by label |
| Cost attribution | 1-2 weeks | Per-team breakdown |
| Shaping rules engine | 3-4 weeks | Drop/aggregate at ingest |
| Recommendations | 2-3 weeks | Suggest optimizations |
| Impact preview | 2 weeks | "What breaks if we drop?" |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 6 weeks | Usage + cardinality + attribution (read-only) |
| **Full** | 16 weeks | + shaping + recommendations + preview |

**This is Chronosphere's core IP.** The full version is expensive.

---

#### 6. BubbleUp (Automatic Root Cause)

| Component | Effort | Notes |
|-----------|--------|-------|
| Event selection UI | 1 week | Select anomalous events |
| Distribution comparison | 1-2 weeks | Compare vs baseline |
| Statistical analysis | 2 weeks | Chi-squared / KL divergence |
| Result ranking | 1 week | Most explanatory first |
| Results UI | 1-2 weeks | Dimension breakdowns |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 4 weeks | Core algorithm + basic UI |
| **Full** | 7 weeks | Polished UI + saved analyses |

**High ROI.** Honeycomb built a company on this concept.

---

#### 7. Change Correlation

| Component | Effort | Notes |
|-----------|--------|-------|
| Webhook receiver | 1 week | GitHub, GitLab, generic |
| Change storage | 3 days | Store deploys, commits |
| Correlation scoring | 1-2 weeks | Time proximity + service |
| Graph annotations | 1 week | Vertical lines on charts |
| Incident timeline | 1-2 weeks | Auto-add changes |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 3 weeks | Webhooks + annotations |
| **Full** | 5 weeks | + timeline + scoring |

**Cheap and useful.** Do this early.

---

#### 8. Security Observability

| Component | Effort | Notes |
|-----------|--------|-------|
| Shell-in-container | 1-2 weeks | execve tracking |
| Unusual network | 1-2 weeks | New outbound IPs |
| Container baseline | 2-3 weeks | Learn "normal" behavior |
| File access tracking | 1-2 weeks | Sensitive files |
| Rules engine | 2 weeks | Custom rules |
| Security UI | 2 weeks | Timeline + alerts |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 3 weeks | Shell detection + unusual network |
| **Full** | 13 weeks | Complete runtime security |

---

#### 9. Migration Assistant

| Component | Effort | Notes |
|-----------|--------|-------|
| Datadog API scan | 1-2 weeks | List dashboards, monitors |
| Compatibility report | 1 week | What works, what doesn't |
| Dashboard converter | 2-3 weeks | Widget type mapping |
| Query translator | 3-4 weeks | Datadog → PromQL (HARD) |
| Grafana import | 2 weeks | JSON dashboards |
| Cost savings report | 1 week | Show $ saved |

| Path | Effort | What You Get |
|------|--------|--------------|
| **Minimum** | 4 weeks | Scan + report + savings |
| **Full** | 13 weeks | + conversion + query translation |

**Query translation is the hard part.** Skip it initially.

---

### Production Essentials (Must Have)

| Feature | Effort | Notes |
|---------|--------|-------|
| Health endpoints | 2 days | `/healthz`, `/readyz` |
| Graceful shutdown | 3 days | Drain connections |
| OTLP receiver | 2 weeks | Use otel-collector libs |
| Prometheus remote_write | 1 week | Standard protocol |
| Retention policies | 1 week | Per-signal TTL |
| Backup/restore | 1-2 weeks | `dogwatch backup/restore` |
| Config validation | 3 days | `dogwatch validate` |
| Basic HA | 2-3 weeks | Active-passive |
| Head sampling | 1-2 weeks | Priority rules |

**Total: 10 weeks** - No shortcuts here, must build.

---

### Summary: Cost vs Impact Matrix

> **📋 THIS IS THE AUTHORITATIVE BUILD ORDER.** Other priority lists in this document provide context, but this matrix is the definitive sequence. When priorities conflict, follow this table.

| # | Feature | Effort | Impact | Rationale |
|---|---------|--------|--------|-----------|
| 1 | **Protocol Parsing (MySQL/PG/Redis)** | 6-20 wks | ★★★★★ | Core differentiator - can't demo without it |
| 2 | **Production Essentials** | 10 wks | ★★★★★ | OTLP, backup, health checks - blocks production |
| 3 | **Cost Intelligence** | 1-3 wks | ★★★★★ | Highest ROI - 1 week = viral screenshots |
| 4 | **LogCompare** | 2-3 wks | ★★★★☆ | Stolen from Sumo Logic, unique value |
| 5 | **Pattern Detection (LogReduce)** | 3-6 wks | ★★★★☆ | Stolen from Splunk, clusters logs |
| 6 | **BubbleUp** | 4-7 wks | ★★★★☆ | Automatic root cause analysis |
| 7 | **Change Correlation** | 3-5 wks | ★★★★☆ | Deploys on graphs, cheap win |
| 8 | **Entity Synthesis** | 4-6 wks | ★★★☆☆ | Auto service catalog from telemetry |
| 9 | **Control Plane (read-only)** | 6 wks | ★★★☆☆ | Cardinality explorer, usage analysis |
| 10 | **Migration Assistant** | 4-13 wks | ★★★☆☆ | Sales enabler, scan + report |
| 11 | **Continuous Profiling** | 3-10 wks | ★★☆☆☆ | Integrate Pyroscope, don't build |
| 12 | **Security Observability** | 3-13 wks | ★★☆☆☆ | Nice-to-have, not core mission |

**Reconciliation with other sections:**

| Document Section | Use It For | Defer to This Matrix For |
|------------------|------------|--------------------------|
| [Technical Gaps](#technical-gaps-vs-chronosphere-datadog-etc) | Understanding enterprise expectations | What to actually build next |
| [Modern Pain Points](#modern-pain-points-we-could-address) | Market context, positioning | Which pain points to address first |
| [Stealing Killer Features](#stealing-killer-features-from-competitors) | Feature specs and inspiration | When to build each feature |
| [Summary: Priority Ranking](#summary-priority-ranking) | Pain point analysis | Build sequence |

---

### 90-Day Plan (Solo Developer)

**Weeks 1-2: Foundation**
- Health endpoints + graceful shutdown
- Cost Intelligence v1 (Datadog comparison)

**Weeks 3-4: Data Ingestion**
- OTLP receiver (gRPC + HTTP)
- Prometheus remote_write

**Weeks 5-10: The Demo (Protocol Parsing)**
- MySQL protocol (2 weeks)
- PostgreSQL protocol (2 weeks)
- Redis protocol (1 week)
- Basic service map (1 week)

**Weeks 11-13: Intelligence**
- Change correlation (3 weeks, start earlier in parallel)

**After 90 days:**
```
✅ Install dogwatch → see database queries in 60 seconds
✅ "This would cost $47K/month on Datadog"
✅ Deploys show as annotations on graphs
✅ Accepts OTLP + Prometheus data
```

---

### Total Investment to MVP

| Scenario | Weeks | Calendar Time |
|----------|-------|---------------|
| Solo dev, minimum features | 25 weeks | 6 months |
| Solo dev, solid features | 40 weeks | 10 months |
| 2-person team, solid features | 20 weeks | 5 months |
| Solo dev, full product | 65 weeks | 16 months |

**Pixie had ~20 engineers for 2+ years. Chronosphere had ~100 engineers.**

The strategy: Build minimum viable → get paying customers → hire → build more.

---

## Technical Priorities

### Pluggable Storage Architecture

Different telemetry types need different storage engines at scale:

```
┌─────────────────────────────────────────────────────────────┐
│                    Storage Abstraction                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Default (works out of the box):                            │
│  └── SQLite for everything                                  │
│                                                              │
│  Scale (opt-in per pillar):                                 │
│  ├── Metrics → VictoriaMetrics, Prometheus, TimescaleDB    │
│  ├── Logs    → ClickHouse, Loki, S3/Minio                  │
│  └── Traces  → Jaeger, Tempo, S3/Minio                     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Configuration:**

```yaml
storage:
  # Default: SQLite (zero config, works immediately)
  default: sqlite

  # Per-pillar overrides
  metrics:
    backend: sqlite          # or: victoriametrics, prometheus
    retention: 30d
    max_size: 20GB
    downsampling:
      - after: 7d
        resolution: 5m

  logs:
    backend: sqlite          # or: clickhouse, loki, s3
    retention: 7d
    max_size: 50GB
    sampling: 1.0

  traces:
    backend: sqlite          # or: jaeger, tempo, s3
    retention: 3d
    sampling: 0.1            # Keep 10% of traces
    head_sampling: true      # Decide at trace start
```

**Interface design:**

```go
// Each pillar has its own storage interface
type MetricsStore interface {
    Write(metrics []Metric) error
    Query(query string, start, end time.Time) ([]Series, error)
}

type LogsStore interface {
    Write(logs []LogEntry) error
    Search(query string, start, end time.Time) ([]LogEntry, error)
}

type TracesStore interface {
    Write(spans []Span) error
    GetTrace(traceID string) (*Trace, error)
}

// Implementations per backend
type SQLiteMetricsStore struct { ... }
type VictoriaMetricsStore struct { ... }
type ClickHouseLogsStore struct { ... }
```

**Why this matters:**
- SQLite works for 80% of users (simple, zero-config)
- Power users can scale individual pillars independently
- No forced complexity - opt-in when needed
- Differentiator vs Grafana (which forces backend choice upfront)

---

### eBPF Protocol Parsing

The core differentiator. This section specifies exactly how we detect and parse each protocol.

#### Kernel Requirements

| Feature | Minimum Kernel | Recommended | Notes |
|---------|----------------|-------------|-------|
| Basic TCP tracking | 4.14 | 5.8+ | `tcp_connect`, `inet_csk_accept` kprobes |
| Protocol parsing | 4.18 | 5.8+ | BPF_MAP_TYPE_RINGBUF for efficient data transfer |
| SSL/TLS interception | 5.5 | 5.10+ | uprobes on OpenSSL/BoringSSL/GnuTLS |
| HTTP/2 parsing | 5.8 | 5.15+ | Requires larger BPF stack for frame parsing |
| BTF support | 5.2 | 5.8+ | CO-RE (Compile Once, Run Everywhere) |

**Startup compatibility check:**
```bash
# dogwatch runs these checks at startup and warns/exits if incompatible
uname -r                                    # Kernel version >= 4.14
cat /sys/kernel/btf/vmlinux 2>/dev/null     # BTF available (optional but recommended)
ls /sys/kernel/debug/tracing 2>/dev/null    # Tracing filesystem mounted
```

#### Protocol Detection State Machine

When TCP data arrives, identify protocol from first packet:

```go
// Detection runs once per connection, result cached
var defaultDetectors = []Detector{
    &MySQLDetector{},      // Binary: check packet header format
    &PostgreSQLDetector{}, // Binary: check message type + length
    &RedisDetector{},      // Text: check RESP prefix characters
    &HTTP2Detector{},      // Binary: check preface or SETTINGS frame
    &HTTPDetector{},       // Text: check for methods (GET/POST/etc.)
}

type ConnectionState struct {
    Tuple    ConnectionTuple
    Protocol Protocol         // Detected protocol
    State    ProtocolState    // Request sent, awaiting response, etc.
    Pending  []PendingRequest // For pipelined protocols
}
```

#### MySQL Protocol Parser

**Wire format:** https://dev.mysql.com/doc/dev/mysql-server/latest/page_protocol_basics.html

```c
// MySQL packet: [3-byte length][1-byte seq][payload]
// Detection: valid length + recognized command byte

#define COM_QUERY          0x03  // Text query
#define COM_STMT_PREPARE   0x16  // Prepared statement
#define COM_STMT_EXECUTE   0x17  // Execute prepared
#define COM_QUIT           0x01  // Connection close

struct mysql_event {
    u64 timestamp_ns;
    u32 pid;
    u64 conn_id;          // Hash of connection tuple
    u8  command;          // COM_QUERY, etc.
    u64 latency_ns;       // Request to first response byte
    u32 affected_rows;    // From OK packet
    u16 error_code;       // 0 = success
    char query[4096];     // Query text (truncated)
    char error_msg[256];  // Error message if error_code != 0
};

// Detection: first payload byte is valid MySQL command
static __always_inline bool is_mysql(const u8 *data, u32 len) {
    if (len < 5) return false;
    u32 pkt_len = data[0] | (data[1] << 8) | (data[2] << 16);
    u8 cmd = data[4];
    return (pkt_len <= 0xFFFFFF) &&
           (cmd == 0x03 || cmd == 0x16 || cmd == 0x17 || cmd == 0x01 ||
            cmd == 0x00 || cmd == 0xFF);  // OK/ERR responses
}
```

#### PostgreSQL Protocol Parser

**Wire format:** https://www.postgresql.org/docs/current/protocol-message-formats.html

```c
// PostgreSQL: [1-byte type][4-byte length BE][payload]
// Exception: startup message has no type byte

#define PG_QUERY           'Q'  // Simple query
#define PG_PARSE           'P'  // Prepared statement
#define PG_BIND            'B'  // Bind parameters
#define PG_EXECUTE         'E'  // Execute (also error response)
#define PG_COMMAND_COMPLETE 'C' // Success
#define PG_DATA_ROW        'D'  // Result row

struct pg_event {
    u64 timestamp_ns;
    u64 conn_id;
    u8  msg_type;         // 'Q', 'P', 'C', etc.
    u64 latency_ns;
    u32 row_count;        // Number of 'D' messages
    char sqlstate[6];     // Error code like "42P01"
    char query[4096];
    char error_msg[256];
};

// Detection: valid type byte + big-endian length
static __always_inline bool is_postgres(const u8 *data, u32 len) {
    if (len < 5) return false;
    u8 type = data[0];
    u32 msg_len = __builtin_bswap32(*(u32*)(data+1));
    return (type == 'Q' || type == 'P' || type == 'B' || type == 'E' ||
            type == 'C' || type == 'D' || type == 'Z') &&
           (msg_len >= 4 && msg_len <= 0x40000000);
}
```

#### Redis Protocol Parser

**Wire format:** https://redis.io/docs/reference/protocol-spec/ (RESP)

```c
// RESP: type prefix + data + \r\n
// '+' simple string, '-' error, ':' integer, '$' bulk, '*' array

struct redis_event {
    u64 timestamp_ns;
    u64 conn_id;
    char command[32];     // GET, SET, HGET, etc.
    char key[256];        // First key argument
    u64 latency_ns;
    bool is_error;        // Response starts with '-'
    char error_msg[256];
};

// Detection: first byte is RESP type indicator
static __always_inline bool is_redis(const u8 *data, u32 len) {
    if (len < 3) return false;
    u8 type = data[0];
    return (type == '+' || type == '-' || type == ':' ||
            type == '$' || type == '*');
}
```

#### HTTP/2 and gRPC Parser

```c
// HTTP/2 frame: [3-byte length BE][1-byte type][1-byte flags][4-byte stream_id]
// Connection preface: "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

struct h2_event {
    u64 timestamp_ns;
    u64 conn_id;
    u32 stream_id;
    char method[16];      // From :method header
    char path[256];       // From :path header (gRPC: /pkg.Svc/Method)
    u64 latency_ns;
    u32 status;           // From :status header
    i32 grpc_status;      // From grpc-status trailer (-1 if not gRPC)
};
```

#### Encrypted Traffic (TLS/HTTPS)

Three approaches, in order of preference:

```go
// 1. SSL library uprobes (recommended)
// Hook OpenSSL/BoringSSL SSL_read/SSL_write to capture plaintext
type SSLUprobe struct {
    Library  string // e.g., "/usr/lib/libssl.so.3"
    ReadSym  string // "SSL_read" or "SSL_read_ex"
    WriteSym string // "SSL_write" or "SSL_write_ex"
}

// Discover at startup by scanning /proc/*/maps
func discoverSSLLibraries() []SSLUprobe

// 2. kTLS interception (kernel 4.13+)
// If kernel TLS offload is used, hook tls_sw_sendmsg/recvmsg

// 3. Fallback: capture metadata only
// Connection timing, bytes transferred, TLS version, cipher suite
// Mark as "encrypted - content not visible" in UI
```

#### Pipelining and Multiplexing

```go
// MySQL: sequential (one request at a time)
// PostgreSQL: pipelined (multiple requests, responses in order)
// Redis: pipelined (multiple commands, responses in order)
// HTTP/2: multiplexed (multiple streams, out-of-order)

// For pipelined: queue pending requests, match responses in order
type PipelineTracker struct {
    pending []PendingRequest // FIFO queue
}

// For multiplexed: map stream ID to pending request
type MultiplexTracker struct {
    streams map[uint32]*PendingRequest
}
```

**Priority order:**
1. MySQL (most common database)
2. PostgreSQL (second most common)
3. Redis (simple protocol, high value)
4. HTTP/2 + gRPC (modern microservices)
5. DNS (often overlooked latency source)

### Cross-Service Trace Correlation

The magic that makes zero-config tracing work. Three methods, applied in priority order:

#### Method 1: Trace Header Parsing (Highest Confidence)

If services already propagate trace context, we extract it from HTTP headers:

```go
// Supported header formats (checked in order)
var traceHeaderParsers = []HeaderParser{
    W3CTraceContext{},  // traceparent: 00-{traceId}-{spanId}-{flags}
    B3Multi{},          // X-B3-TraceId, X-B3-SpanId, X-B3-ParentSpanId
    B3Single{},         // b3: {traceId}-{spanId}-{sampling}-{parentSpanId}
    Jaeger{},           // uber-trace-id: {traceId}:{spanId}:{parentSpanId}:{flags}
    XRequestID{},       // X-Request-ID (fallback, generates trace from request ID)
}

func (c *TraceCorrelator) ExtractTraceContext(headers http.Header) (*TraceContext, bool) {
    for _, parser := range traceHeaderParsers {
        if ctx, ok := parser.Parse(headers); ok {
            return ctx, true
        }
    }
    return nil, false
}
```

**Confidence: 100%** — If headers exist, correlation is definitive.

#### Method 2: Connection Tuple Matching (High Confidence)

When no headers exist, we correlate using TCP connection 5-tuples:

```go
// When Service A connects to Service B:
// A's outbound: (A_IP:ephemeral_port -> B_IP:service_port)
// B's inbound:  (A_IP:ephemeral_port -> B_IP:service_port)
// These MUST match exactly - enforced by TCP/IP.

type ConnectionTuple struct {
    SrcIP   net.IP
    SrcPort uint16
    DstIP   net.IP
    DstPort uint16
    // Tracked via eBPF tcp_connect (client) and inet_csk_accept (server)
}

func (c *TraceCorrelator) CorrelateByConnection(
    clientSpan *Span,
    serverSpans []*Span,
) (*Span, float64) {
    clientTuple := clientSpan.ConnectionTuple

    for _, serverSpan := range serverSpans {
        if serverSpan.ConnectionTuple.Matches(clientTuple) {
            // Exact match on all 4 fields
            return serverSpan, 1.0 // 100% confidence
        }
    }
    return nil, 0
}
```

**Confidence: 100%** — TCP guarantees tuple uniqueness for connection lifetime.

**Limitation:** Only works when both client and server run dogwatch eBPF probes.

#### Method 3: Timing-Window Correlation (Medium Confidence)

When we can't match connections directly (e.g., load balancer in between), use timing:

```go
const (
    // Timing window for correlation
    // Network latency is typically 0.1-5ms within a datacenter
    // We use 50ms to account for:
    // - Clock skew between machines (NTP typically <10ms)
    // - Kernel scheduling jitter
    // - Load balancer processing time
    MaxTimingWindow = 50 * time.Millisecond

    // Minimum overlap required to consider a match
    MinOverlapRatio = 0.8 // Child span must overlap 80%+ with potential parent
)

type TimingCorrelation struct {
    // Client span: started at T1, made outbound call at T2, received response at T3
    // Server span: received request at T4, sent response at T5
    // For valid correlation: T2 <= T4 <= T5 <= T3 (with timing window tolerance)
}

func (c *TraceCorrelator) CorrelateByTiming(
    clientSpan *Span,
    candidateServerSpans []*Span,
) (*Span, float64) {
    var bestMatch *Span
    var bestConfidence float64

    for _, server := range candidateServerSpans {
        // Check if server span fits within client span's timing window
        if !c.fitsTimingWindow(clientSpan, server) {
            continue
        }

        // Calculate confidence based on:
        // 1. How closely the timing aligns
        // 2. Whether endpoints match (A calls B's /api/users, B received /api/users)
        // 3. Whether the response size/status matches
        confidence := c.calculateTimingConfidence(clientSpan, server)

        if confidence > bestConfidence {
            bestMatch = server
            bestConfidence = confidence
        }
    }

    return bestMatch, bestConfidence
}

func (c *TraceCorrelator) calculateTimingConfidence(client, server *Span) float64 {
    confidence := 0.0

    // Base: timing alignment (0.0 - 0.4)
    timeDelta := abs(client.OutboundTime.Sub(server.StartTime))
    if timeDelta < 5*time.Millisecond {
        confidence += 0.4
    } else if timeDelta < 20*time.Millisecond {
        confidence += 0.3
    } else if timeDelta < MaxTimingWindow {
        confidence += 0.2
    }

    // Endpoint match (0.0 - 0.3)
    if client.TargetEndpoint == server.Endpoint {
        confidence += 0.3
    } else if pathMatches(client.TargetEndpoint, server.Endpoint) {
        confidence += 0.2 // Path matches but method differs
    }

    // Response correlation (0.0 - 0.2)
    if client.ResponseStatus == server.ResponseStatus {
        confidence += 0.1
    }
    if abs(client.ResponseSize-server.ResponseSize) < 100 {
        confidence += 0.1
    }

    // Service topology prior (0.0 - 0.1)
    // If we've seen A->B calls before, boost confidence
    if c.hasHistoricalConnection(client.Service, server.Service) {
        confidence += 0.1
    }

    return confidence
}
```

**Confidence: 50-90%** depending on timing alignment and endpoint matching.

**Thresholds:**
- `confidence >= 0.8`: Auto-link spans
- `0.5 <= confidence < 0.8`: Link with "inferred" marker in UI
- `confidence < 0.5`: Don't link, show as separate traces

#### Clock Skew Handling

```go
// We don't require synchronized clocks. Instead:
// 1. Record monotonic time (kernel timestamps) for ordering within a host
// 2. Use wall-clock time for cross-host correlation with tolerance window
// 3. Periodically estimate skew between hosts by comparing bidirectional call timings

type ClockSkewEstimator struct {
    // For each host pair, track observed skew
    skewEstimates map[HostPair]time.Duration
}

func (e *ClockSkewEstimator) EstimateSkew(hostA, hostB string, samples []RTTSample) time.Duration {
    // If A calls B and B responds:
    // A's view: sent at T1, received at T2 (RTT = T2 - T1)
    // B's view: received at T3, sent at T4
    // Skew ≈ T3 - T1 - (RTT/2)
    // Average over multiple samples to reduce noise

    var skewSum time.Duration
    for _, s := range samples {
        estimatedSkew := s.ServerReceiveTime.Sub(s.ClientSendTime) - (s.RTT / 2)
        skewSum += estimatedSkew
    }
    return skewSum / time.Duration(len(samples))
}
```

#### Algorithm Summary

```
For each outbound span from Service A:
1. Check for trace headers → if found, use those (done)
2. Find server spans where connection tuple matches → link with 100% confidence
3. If no tuple match, find candidate spans:
   a. Filter by target service (from DNS/connection)
   b. Filter by timing window (±50ms adjusted for estimated clock skew)
   c. Score by endpoint match, timing precision, response correlation
   d. Link if confidence >= threshold
```

### Cost Intelligence Implementation

```go
// Datadog pricing (as of 2024)
type DatadogPricing struct {
    InfrastructurePerHost  float64 // $15-23/host/month
    APMPerHost             float64 // $31-40/host/month
    LogsPerGBIngested      float64 // $0.10/GB
    LogsPerGBIndexed       float64 // $1.70/GB (15-day retention)
    TracesPerMillion       float64 // $1.70/M spans (after 150M free)
    SyntheticsAPIPerK      float64 // $5/10K runs
}

func (c *CostCalculator) CalculateDatadogCost() float64 {
    cost := 0.0
    cost += float64(c.HostCount) * DatadogPricing.APMPerHost
    cost += float64(c.LogBytesIngested/1e9) * DatadogPricing.LogsPerGBIngested
    cost += float64(c.TraceSpans/1e6) * DatadogPricing.TracesPerMillion
    // ... etc
    return cost
}
```

---

## Technical Gaps (vs Chronosphere, Datadog, etc.)

These are capabilities that enterprise observability platforms have that dogwatch currently lacks. Prioritize based on what blocks adoption.

### Critical: Must Have for Production

#### 1. High Availability

**Current:** Single binary = single point of failure. If dogwatch dies, you lose observability.

**What enterprises have:**
- Multiple instances behind load balancer
- Automatic failover
- No single point of failure
- Graceful degradation

**Minimum viable:**
```yaml
# Active-passive with shared storage
dogwatch:
  mode: primary
  storage: /shared/nfs/dogwatch/
  failover:
    peer: dogwatch-standby:9998
    heartbeat_interval: 5s
    takeover_after: 15s
```

**Full solution:**
```yaml
# Active-active cluster with gossip
cluster:
  enabled: true
  peers:
    - dogwatch-1:9998
    - dogwatch-2:9998
    - dogwatch-3:9998
  quorum: 2
  replication_factor: 2
```

#### 2. Backup & Disaster Recovery

**Current:** SQLite files with no backup strategy. Data loss = start over.

**What enterprises have:**
- Scheduled backups
- Point-in-time recovery
- Cross-region replication
- Backup validation

**Minimum viable:**
```bash
# Built-in backup command
dogwatch backup --output /backups/dogwatch-$(date +%Y%m%d).tar.gz

# Restore
dogwatch restore --from /backups/dogwatch-20240115.tar.gz
```

**Full solution:**
```yaml
backup:
  enabled: true
  schedule: "0 2 * * *"  # 2am daily
  retention: 30d
  destination:
    type: s3
    bucket: dogwatch-backups
    prefix: prod/
  validation:
    enabled: true
    restore_test: weekly
```

#### 3. Sampling & Data Reduction

**Current:** Keep everything. At scale, this explodes storage and costs.

**What enterprises have:**
- **Head sampling** - Decide at trace start (fast, simple, loses rare events)
- **Tail sampling** - Decide after trace completes (keeps errors/slow, more complex)
- **Adaptive sampling** - Adjust rate based on traffic
- **Priority sampling** - Always keep certain traces (errors, slow, specific users)

**Minimum viable:**
```yaml
sampling:
  traces:
    rate: 0.1  # Keep 10%
    always_keep:
      - error: true
      - latency_ms: ">2000"
      - user_id: "priority-customer-*"

  logs:
    rate: 1.0  # Keep all (logs are usually important)
    drop:
      - level: debug
      - message: "health check"

  metrics:
    # Metrics don't sample, but can downsample
    downsample:
      - after: 7d
        resolution: 5m
      - after: 30d
        resolution: 1h
```

#### 4. Backpressure & Rate Limiting

**Current:** If ingest exceeds capacity, behavior is undefined (likely OOM or data loss).

**What enterprises have:**
- Admission control at ingest
- Queue management with limits
- Graceful degradation (drop low-priority data first)
- Client feedback (slow down / retry)

**Minimum viable:**
```go
type IngestPipeline struct {
    queue        chan Event
    maxQueueSize int
    dropPolicy   DropPolicy  // DROP_OLDEST, DROP_NEWEST, DROP_LOW_PRIORITY

    // Metrics for observability of observability
    eventsReceived  counter
    eventsDropped   counter
    queueDepth      gauge
    ingestLatency   histogram
}

// Drop policy when queue is full
func (p *IngestPipeline) onQueueFull(event Event) {
    switch p.dropPolicy {
    case DROP_LOW_PRIORITY:
        // Drop debug logs before error logs
        // Drop sampled traces before kept ones
    case DROP_OLDEST:
        // Pop from front of queue
    case DROP_NEWEST:
        // Reject incoming event
    }
    p.eventsDropped.Inc()
}
```

#### 5. Data Export & Portability

**Current:** Data goes in, can't easily get it out. Lock-in concern.

**What enterprises have:**
- OTLP export (standard format)
- Bulk export to S3/GCS
- Query API for extracting data
- Archive to cold storage

**Minimum viable:**
```bash
# Export to OTLP for migration
dogwatch export --format otlp --start 2024-01-01 --end 2024-01-31 > traces.otlp

# Export to CSV for analysis
dogwatch export --format csv --query "service=api" --output metrics.csv
```

---

### High Priority: Expected by Serious Users

#### 6. Full Query Language Support

**Current:** Basic query support. No standard compatibility.

**What enterprises have:**
- Full PromQL for metrics
- LogQL for logs (Loki-style)
- TraceQL for traces (Tempo-style)

**Why it matters:**
- Teams have existing queries/dashboards
- Standard languages reduce learning curve
- Migration assistant needs query translation

**Example queries needed:**
```promql
# PromQL - complex aggregation
sum(rate(http_requests_total{status=~"5.."}[5m])) by (service)
  / sum(rate(http_requests_total[5m])) by (service) * 100

# LogQL - logs with metrics extraction
sum by (level) (count_over_time({service="api"} | json | level="error" [1h]))

# TraceQL - trace-specific queries
{span.http.status_code >= 500} | avg(duration) > 2s
```

#### 7. SLO/SLI Native Support

**Current:** Basic alerting only. No SLO tracking.

**What enterprises have:**
- Define SLOs (99.9% availability, p99 < 200ms)
- Track error budgets
- Burn rate alerts
- SLO dashboards

**Minimum viable:**
```yaml
slos:
  - name: api-availability
    service: api
    target: 0.999  # 99.9%
    indicator:
      type: availability
      good: 'http_requests{status!~"5.."}'
      total: 'http_requests{}'
    windows:
      - 7d
      - 30d
    alerts:
      - burn_rate: 14.4  # 1% budget in 1 hour
        severity: critical
      - burn_rate: 6     # 1% budget in 6 hours
        severity: warning
```

#### 8. Alert Correlation & Grouping

**Current:** Each alert fires independently. Alert fatigue.

**What enterprises have:**
- Group related alerts (same service, same time)
- Deduplicate repeated alerts
- Show alert timeline in incidents
- Suppress downstream alerts

**Minimum viable:**
```yaml
alerting:
  grouping:
    - by: [service, alertname]
      wait: 30s        # Wait for more alerts before sending
      interval: 5m     # How often to send grouped alerts
  suppression:
    - source_match: { alertname: "ServiceDown" }
      target_match: { service: "$service" }  # Suppress all alerts for down service
```

#### 9. Audit Logging

**Current:** No audit trail. Who changed what?

**What enterprises have:**
- Every action logged (dashboard edit, alert change, user action)
- Tamper-proof audit log
- Retention policies
- Compliance reports

**Minimum viable:**
```go
type AuditEvent struct {
    Timestamp   time.Time
    UserID      string
    UserEmail   string
    Action      string    // "dashboard.create", "alert.update", "user.invite"
    Resource    string    // Resource type
    ResourceID  string    // Resource identifier
    OldValue    any       // Previous state (for updates)
    NewValue    any       // New state
    IPAddress   string
    UserAgent   string
}

// Audit everything
func (h *DashboardHandler) Update(w http.ResponseWriter, r *http.Request) {
    // ... do the update
    h.audit.Log(AuditEvent{
        UserID:     user.ID,
        Action:     "dashboard.update",
        ResourceID: dashboard.ID,
        OldValue:   oldDashboard,
        NewValue:   newDashboard,
    })
}
```

#### 10. Observability of Observability

**Current:** Can't see if dogwatch itself is healthy or falling behind.

**What enterprises have:**
- Ingest rate dashboards
- Query performance metrics
- Storage utilization
- Pipeline lag indicators
- Self-alerting on health

**Minimum viable:**
```
dogwatch internal metrics:
├── dogwatch_ingest_events_total{type="trace|log|metric"}
├── dogwatch_ingest_lag_seconds
├── dogwatch_storage_bytes{pillar="traces|logs|metrics"}
├── dogwatch_query_duration_seconds{query_type="..."}
├── dogwatch_query_errors_total
├── dogwatch_ebpf_events_dropped_total
└── dogwatch_ebpf_buffer_usage_percent
```

Built-in `/internal/metrics` dashboard that shows all this.

---

### Medium Priority: Enterprise Requirements

#### 11. Multi-Tenancy

**Current:** Single org. Can't isolate different teams/customers.

**What enterprises have:**
- Multiple tenants in one deployment
- Data isolation (can't see other tenants)
- Per-tenant quotas
- Cross-tenant queries for platform teams

**When needed:** Platform teams, MSPs, SaaS providers running dogwatch for customers.

#### 12. PII Detection & Masking

**Current:** Logs and traces capture everything, including PII.

**What enterprises have:**
- Auto-detect PII patterns (emails, SSNs, credit cards)
- Mask at ingest time
- Audit who accessed unmasked data
- Retention policies per data type

**Minimum viable:**
```yaml
privacy:
  masking:
    - pattern: '\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b'
      replacement: "[EMAIL]"
    - pattern: '\b\d{3}-\d{2}-\d{4}\b'
      replacement: "[SSN]"
    - pattern: '\b\d{16}\b'
      replacement: "[CARD]"
  sensitive_fields:
    - password
    - secret
    - token
    - authorization
```

#### 13. Distributed Query Execution

**Current:** Single-node query. Can't scale queries across cluster.

**What enterprises have (Chronosphere, ClickHouse):**
- Query coordinator distributes work
- Parallel execution across nodes
- Result aggregation
- Query caching at multiple levels

**Why it matters:** Big queries (90-day aggregation) need parallelism.

#### 14. Maintenance Windows

**Current:** Alerts fire even during planned maintenance.

**What enterprises have:**
- Schedule maintenance windows
- Auto-suppress alerts during window
- Audit trail of suppressions

**Minimum viable:**
```yaml
maintenance_windows:
  - name: weekly-deploy
    schedule: "0 2 * * 3"  # 2am Wednesday
    duration: 2h
    services: [api, web]
    suppress_alerts: true
```

---

### Lower Priority: Nice to Have

#### 15. On-Call Scheduling

**Current:** Alerts go to fixed channels. No rotation.

**What enterprises have (PagerDuty, Opsgenie):**
- On-call schedules with rotations
- Escalation policies
- Override handling
- Handoff summaries

**Consider:** Integrate with PagerDuty/Opsgenie rather than rebuilding.

#### 16. SDK / Client Libraries

**Current:** eBPF only. Some use cases need instrumentation.

**What enterprises have:**
- OpenTelemetry SDK support
- Language-specific libraries
- Custom metric emission
- Manual span creation

**Minimum viable:**
```go
// Accept OTLP input for when eBPF isn't enough
otlp:
  grpc:
    enabled: true
    port: 4317
  http:
    enabled: true
    port: 4318
```

#### 17. Upgrade Path & Migrations

**Current:** No defined upgrade process. Schema changes = ???

**What enterprises have:**
- Semantic versioning
- Migration scripts between versions
- Rollback capability
- Zero-downtime upgrades

**Minimum viable:**
```bash
# Check upgrade compatibility
dogwatch upgrade --check

# Run with automatic migration
dogwatch upgrade --from v0.9.0 --to v1.0.0

# Rollback if needed
dogwatch rollback --to v0.9.0
```

#### 18. ARM64 / Multi-Platform

**Current:** Likely amd64 Linux only.

**What enterprises need:**
- ARM64 (Graviton, M1/M2)
- Multi-arch container images
- Windows support (for agents)

#### 19. Helm Chart & K8s Operator

**Current:** Manual deployment.

**What enterprises have:**
- Official Helm chart
- K8s Operator for lifecycle management
- CRDs for declarative config
- Auto-scaling

#### 20. Air-Gapped / Offline Support

**Current:** May phone home or require internet for features.

**What enterprises need:**
- Fully offline installation
- License validation without internet
- Update bundles for offline environments

---

## Gap Prioritization Matrix

| Gap | Blocks Adoption? | Effort | Priority |
|-----|------------------|--------|----------|
| High Availability | Yes (production) | High | P0 |
| Backup/Restore | Yes (production) | Medium | P0 |
| Sampling | Yes (scale) | Medium | P0 |
| Backpressure | Yes (scale) | Medium | P0 |
| PromQL Compat | Partial (migration) | High | P1 |
| SLO Support | No | Medium | P1 |
| Audit Logging | Yes (compliance) | Low | P1 |
| Self-Monitoring | No (but embarrassing) | Low | P1 |
| Data Export | Partial (lock-in fear) | Low | P1 |
| Alert Grouping | No | Medium | P2 |
| Multi-Tenancy | Only for platforms | High | P2 |
| PII Masking | Yes (compliance) | Medium | P2 |
| Helm/Operator | No | Medium | P2 |
| ARM64 | No | Low | P2 |
| Maintenance Windows | No | Low | P3 |
| On-Call | Integrate instead | N/A | P3 |
| Distributed Query | Only at massive scale | High | P3 |

---

## Future-Facing Assessment (2024-2026)

This section analyzes: (1) What competitors offer that we don't, (2) Whether we're positioned for future trends, (3) Modern pain points we could address.

### What Competitors Have That We Don't

#### 1. AI/ML-Powered Features (CRITICAL GAP)

This is the biggest gap. Every major platform now has AI:

| Platform | AI Feature | What It Does |
|----------|-----------|--------------|
| **Datadog** | Watchdog | Automatic anomaly detection, root cause analysis, correlation |
| **Dynatrace** | Davis AI | Causal AI, automatic problem detection, topology-aware |
| **New Relic** | AI Ops | LLM-powered queries, intelligent alerts, auto-insights |
| **Splunk** | AI Assistant | Natural language queries, pattern detection |
| **Honeycomb** | Query Assistant | LLM helps write queries, explains results |

**What they offer:**
```
User: "Why is checkout slow?"

AI: "Checkout latency increased 340% at 14:32.
     Root cause: PostgreSQL query on orders table taking 2.3s (usually 45ms).
     Correlation: New deployment (v2.4.1) at 14:30 added new JOIN.
     Impact: 12% of users seeing >3s load times.
     Suggested fix: Add index on orders.user_id"
```

**Our current state:** Zero AI. Manual investigation only.

**What we SHOULD do:**
```
Phase 1 (Now): Statistical anomaly detection
- Standard deviation alerts ("3σ from baseline")
- Rate-of-change detection
- No ML training needed

Phase 2 (6 months): BubbleUp on steroids
- Automatic dimension analysis
- "Top changes in last hour"
- Still rule-based, feels like AI

Phase 3 (12 months): LLM integration
- Natural language → query translation
- "Explain this trace" → structured summary
- Use Claude/GPT API (not custom ML)
```

**Honest take:** Skip custom ML (expensive, unreliable). Use LLM APIs for natural language. Focus on deterministic "AI-like" features (BubbleUp, change correlation) that are reliable.

---

#### 2. Real User Monitoring (RUM)

**What competitors have:**
- Browser JavaScript SDK
- Core Web Vitals (LCP, FID, CLS)
- User session tracking
- Error tracking with stack traces
- Rage click detection
- Session replay (some)

**Example from Datadog:**
```
Session: user@example.com
├── Page Load: /checkout (LCP: 2.3s ⚠️)
│   └── Long Task: payment.js blocking for 450ms
├── Click: "Submit Order" → 500 error
│   └── Stack trace: TypeError at checkout.js:142
└── Rage clicks detected on "Retry" button (7 clicks)
```

**Our position:** We explicitly said "integrate with Sentry/LogRocket" - this is correct.

**Why not build it:**
- Completely different tech (JavaScript SDK vs eBPF)
- Sentry/LogRocket are excellent and cheap
- Would distract from core value prop
- Browser observability ≠ backend observability

**What we SHOULD do:**
- Correlation hooks: Link RUM session ID to backend traces
- Error forwarding: Accept error webhooks from Sentry
- Dashboard embedding: Show Sentry/LogRocket widgets

---

#### 3. Mobile Observability

**What competitors have:**
- iOS/Android SDKs
- Crash reporting
- Network request tracing
- App startup time
- Battery/memory impact

**Example from New Relic:**
```
App: MyApp iOS
├── Crashes: 47 (0.3% of sessions)
│   └── Top: NSInvalidArgumentException in CheckoutVC
├── Network: 2.1s avg API latency
│   └── Slow: /api/products (3.4s on 3G)
├── Launch: 1.8s cold start
└── Battery: Network layer using 12% of battery
```

**Our position:** Not applicable to our value prop.

**Why not build it:**
- Mobile is completely different domain
- Crashlytics (Firebase) is free and excellent
- eBPF doesn't exist on mobile
- Would require dedicated mobile engineers

**What we SHOULD do:**
- Accept mobile app traces (OTLP)
- Show mobile → backend correlation
- Integrate with Crashlytics/Sentry for crash data

---

#### 4. Browser-Based Synthetic Monitoring

**What we have:** HTTP/TCP checks from our servers.

**What competitors have:**
```
Synthetic Browser Test: "Checkout Flow"
1. Navigate to https://shop.example.com
2. Click "Add to Cart" on product #123
3. Assert cart shows 1 item
4. Click "Checkout"
5. Fill form with test data
6. Click "Place Order"
7. Assert confirmation page shows order number

Run from: US-East, US-West, EU-West, APAC
Frequency: Every 5 minutes
Screenshot on failure: Yes
Video recording: Yes
```

**What this requires:**
- Headless Chrome (Playwright/Puppeteer)
- Global PoP infrastructure
- Video/screenshot storage
- DOM interaction scripting

**Our position:** Partial gap.

**What we SHOULD do:**
```yaml
# Phase 1 (Now): API-level multi-step
synthetics:
  - name: checkout-flow
    type: api
    steps:
      - request:
          method: POST
          url: /api/cart/add
          body: {product_id: 123}
        assertions:
          - status_code: 200
      - request:
          method: POST
          url: /api/checkout
          body: {cart_id: "{{steps[0].response.cart_id}}"}
        assertions:
          - status_code: 200
          - body_contains: order_id

# Phase 2 (Later): Playwright integration
  - name: visual-checkout
    type: browser
    script: |
      await page.goto('https://shop.example.com');
      await page.click('[data-test="add-cart"]');
      await expect(page.locator('.cart-count')).toHaveText('1');
```

**Honest assessment:** Multi-step API is enough for most users. Browser synthetics are nice-to-have.

---

#### 5. CI/CD Pipeline Observability

**What competitors have:**
- GitHub Actions integration
- GitLab CI metrics
- Deployment tracking
- Build time optimization
- Test flakiness detection

**Example from Datadog:**
```
Pipeline: main.yml (Run #4521)
├── Build: 3m 24s (↑12% from baseline)
│   └── Slow step: npm install (2m 10s)
├── Test: 8m 12s
│   └── Flaky: checkout.test.js (failed 3/10 runs)
├── Deploy: 45s
└── Total: 12m 21s

Deployed: v2.4.1 → production
Services affected: api, worker, web
```

**Our position:** We have deployment markers but not pipeline visibility.

**What we SHOULD do:**
```yaml
# Deployment events (we have this)
POST /api/events/deploy
{
  "version": "v2.4.1",
  "service": "api",
  "commit": "abc123",
  "pipeline_url": "https://github.com/..."
}

# Pipeline metrics (add this)
POST /api/pipelines/run
{
  "pipeline": "main.yml",
  "run_id": 4521,
  "status": "success",
  "duration_ms": 741000,
  "stages": [
    {"name": "build", "duration_ms": 204000},
    {"name": "test", "duration_ms": 492000},
    {"name": "deploy", "duration_ms": 45000}
  ]
}
```

Priority: Medium. Nice for correlation but not core.

---

#### 6. Cloud Cost Correlation

**What competitors have:**
- AWS/GCP/Azure spend data
- Cost per service/team
- Right-sizing recommendations
- Idle resource detection
- FinOps integration

**Example from CloudHealth/Datadog:**
```
Service: payment-api
├── Compute: $4,230/month (12 × m5.xlarge)
│   └── Utilization: 23% avg → Recommend: 6 × m5.large ($2,115)
├── Database: $890/month (RDS r5.large)
│   └── Utilization: 67% → Appropriate
├── Network: $340/month
└── Total: $5,460/month

Savings opportunity: $2,115/month (39%)
```

**Our position:** We don't have cloud billing integration.

**Why it matters:**
- Cost is #1 pain point (we know this)
- Showing "your infra costs X" + "your observability costs Y" = complete picture
- FinOps teams are a growing buyer

**What we SHOULD do:**
```yaml
# Phase 1: Manual cost tags
services:
  payment-api:
    cost_allocation:
      monthly_estimate: 5460
      team: payments
      cost_center: CC-1234

# Phase 2: Cloud provider integration
cloud_costs:
  aws:
    role_arn: arn:aws:iam::123456789:role/dogwatch-costs
    tag_mapping:
      service: "app:service"
      team: "team"
    sync_interval: 24h
```

Priority: High. Aligns with our cost-conscious positioning.

---

#### 7. IDE Integration

**What competitors have:**
- VS Code extension
- IntelliJ plugin
- "See traces for this function"
- "View logs for this file"
- CodeLens annotations

**Example from Rookout/Datadog:**
```
// In VS Code, you see:
function processOrder(order) {  // 🔴 2 errors in last hour
    const user = getUser(order.userId);  // ⚡ p99: 234ms
    const payment = chargeCard(user.card);  // 💸 12 traces
    // ...
}

// Click annotation → Opens Datadog with filtered traces
```

**Our position:** Zero IDE integration.

**What we SHOULD do:**
```
Phase 1 (Quick win): VS Code extension
- Show recent errors/logs for current file
- Jump to traces for selected function
- Link to dashboard from code

vs-code-extension/
├── src/
│   ├── extension.ts      # Main entry
│   ├── traces.ts         # Fetch traces for file/function
│   ├── codelens.ts       # Show annotations
│   └── sidebar.ts        # Live tail panel
└── package.json
```

Priority: Medium-high. Developer experience differentiator.

---

#### 8. SIEM / Security Convergence

**What competitors have:**
- Security event correlation
- Threat detection from observability data
- CSPM (Cloud Security Posture Management)
- RASP (Runtime Application Security)

**Example from Datadog Security:**
```
THREAT DETECTED
├── Attack: SQL Injection attempt
├── Target: /api/users?id=1'; DROP TABLE users;--
├── Source: 192.168.1.100 (known scanner)
├── Action: Blocked by WAF
├── Related: 47 similar attempts in last hour
└── Response: Auto-blocked IP for 24h
```

**Our position:** We have basic container security (shell-in-container). Missing broader security.

**What we SHOULD do:**
```yaml
# Leverage what we see via eBPF
security:
  detection_rules:
    - name: sql_injection_attempt
      condition: |
        http.path contains "'" OR
        http.path contains "--" OR
        http.path contains "UNION SELECT"
      severity: high

    - name: unusual_outbound
      condition: |
        network.destination NOT IN service.baseline.destinations
        AND network.destination.is_public
      severity: medium

    - name: sensitive_file_access
      condition: |
        file.path matches "/etc/passwd|/etc/shadow|.ssh/id_rsa"
      severity: critical

  integrations:
    - type: siem
      target: splunk
      forward_severity: high+
```

Priority: Medium. We already see the data - just need detection rules.

---

### Are We Future-Facing Enough?

Evaluating against major trends for 2024-2026:

#### Trend 1: LLM-Powered Observability ⚠️ GAP

**The trend:** Every vendor is adding "Ask AI about your system."

**Where it's going:**
```
2024: "Why is my app slow?" → AI suggests queries
2025: "Fix the problem" → AI generates runbooks, auto-remediates
2026: "Prevent this from happening" → AI predicts issues before they occur
```

**Our position:** No AI capabilities.

**Recommendation:**
```python
# Don't build ML. Use LLM APIs.
# Example integration:

def ask_claude(question: str, context: dict) -> str:
    """Use Claude to answer observability questions."""

    prompt = f"""
    You are an observability assistant for dogwatch.

    Current context:
    - Active alerts: {context['alerts']}
    - Recent deployments: {context['deploys']}
    - Top errors: {context['errors']}
    - Slow endpoints: {context['slow_endpoints']}

    User question: {question}

    Provide a concise, actionable answer.
    """

    response = anthropic.messages.create(
        model="claude-sonnet-4-20250514",
        messages=[{"role": "user", "content": prompt}]
    )
    return response.content[0].text
```

**Timeline:** Add basic LLM integration in Phase 3 (after core stability).

---

#### Trend 2: Platform Engineering / Internal Developer Platforms ✅ ALIGNED

**The trend:** Platform teams building self-service infrastructure.

**Key platforms:**
- Backstage (Spotify's IDP)
- Port (SaaS IDP)
- Humanitec
- Cortex

**Where it's going:**
```
Developer Portal
├── Service Catalog (← We have this!)
├── Templates ("Create new service")
├── Documentation
├── Observability (← We go here!)
│   ├── Health status
│   ├── Dependencies
│   ├── SLOs
│   └── On-call
└── Infrastructure
```

**Our position:** Good! Service Catalog is exactly what platform teams need.

**Recommendation:**
```yaml
# Add Backstage integration
integrations:
  backstage:
    enabled: true
    catalog_sync: true  # Sync services to Backstage
    annotations:
      health_url: "https://dogwatch.internal/api/services/{name}/health"
      dashboard_url: "https://dogwatch.internal/d/{name}"
```

---

#### Trend 3: OpenTelemetry Becoming Default ✅ ALIGNED

**The trend:** OTel is the standard. Everything speaks OTLP.

**Adoption:**
- All major cloud providers support OTel export
- All major APM vendors accept OTLP
- OTel Collector is default for most new setups

**Our position:** Good. We have OTLP planned as P0.

**Recommendation:** Ship OTLP receiver before anything else that accepts external data.

---

#### Trend 4: FinOps / Cost Management ✅ ALIGNED

**The trend:** Observability + Cost in one place.

**Players:**
- CloudHealth, Kubecost, CAST AI (cost)
- Datadog, New Relic (observability + cost)
- Converging into single platforms

**Our position:** Cost Intelligence is our #2 killer feature. Perfectly aligned.

**Recommendation:** Add cloud cost import (AWS Cost Explorer, GCP Billing) to complete the picture.

---

#### Trend 5: GitOps / Configuration as Code ⚠️ PARTIAL

**The trend:** Everything in Git, including observability config.

**What this means:**
```yaml
# alerts/production/api-alerts.yaml
apiVersion: dogwatch/v1
kind: Alert
metadata:
  name: api-error-rate
  labels:
    team: api-team
spec:
  query: rate(http_errors{service="api"}[5m]) > 0.05
  for: 5m
  channels: [slack-api-team]

# Managed by ArgoCD / Flux
# Changes trigger PRs, reviews, audit trail
```

**Our position:** We have config files but no Git-native workflow.

**Recommendation:**
```yaml
# Add these capabilities:
config:
  source: git
  repository: https://github.com/myorg/dogwatch-config
  branch: main
  sync_interval: 60s
  webhook_secret: ${GITHUB_WEBHOOK_SECRET}

# On change:
# 1. Validate config
# 2. Show diff in UI
# 3. Apply with rollback capability
```

---

#### Trend 6: Sustainability / Green Software ⚠️ NOT ADDRESSED

**The trend:** Carbon footprint of software becoming a metric.

**What's emerging:**
- Cloud Carbon Footprint (open source)
- AWS Customer Carbon Footprint Tool
- GCP Carbon Sense
- "Carbon cost per request"

**Example:**
```
Service: api-gateway
├── Compute: 1.2 kWh/day → 0.48 kg CO2/day
├── Network: 0.3 kWh/day → 0.12 kg CO2/day
├── Storage: 0.1 kWh/day → 0.04 kg CO2/day
└── Total: 0.64 kg CO2/day (234 kg CO2/year)

Compared to: 35 miles of driving
```

**Our position:** Not addressed at all.

**Recommendation:** Future nice-to-have. Not core value prop. Could add carbon estimate to cost calculator later.

---

#### Trend 7: WebAssembly for Extensibility ⚠️ OPPORTUNITY

**The trend:** Wasm as the universal plugin/extension runtime.

**Where Wasm is being used in observability:**

| Project | Wasm Use Case |
|---------|---------------|
| **OTel Collector** | Custom processors, transforms |
| **Envoy Proxy** | Custom filters, auth, observability |
| **Tremor** | User-defined data processing pipelines |
| **Vector** | Custom transforms and conditions |
| **Fluent Bit** | Plugin system for filters/outputs |

**Why Wasm matters for us:**

```
Problem: Users want custom behavior
- "I need to redact SSNs in a specific format"
- "I want to enrich logs with data from our internal API"
- "I need custom sampling logic for our use case"

Old solution: Fork the code, maintain forever
New solution: Write a Wasm plugin, drop it in

┌─────────────────────────────────────────────────────────┐
│                    dogwatch                              │
├─────────────────────────────────────────────────────────┤
│  Data Pipeline                                           │
│  ┌────────┐   ┌────────────┐   ┌────────────┐           │
│  │ Ingest │──▶│ Wasm Plugins│──▶│  Storage   │           │
│  └────────┘   └────────────┘   └────────────┘           │
│                     │                                    │
│               ┌─────┴─────┐                              │
│               ▼           ▼                              │
│          user1.wasm  user2.wasm                          │
│          (PII redact) (enrich)                           │
└─────────────────────────────────────────────────────────┘
```

**Concrete Wasm use cases for dogwatch:**

1. **Custom Protocol Parsers**
```rust
// user writes in Rust, compiles to Wasm
#[no_mangle]
pub fn parse_proprietary_protocol(data: &[u8]) -> ParsedMessage {
    // Parse company's internal RPC format
    // Return structured data dogwatch can trace
}
```

2. **PII Detection & Redaction**
```rust
#[no_mangle]
pub fn redact_pii(log_line: &str) -> String {
    // Company-specific PII patterns
    // SSN format, internal ID format, etc.
    log_line
        .replace_regex(r"\d{3}-\d{2}-\d{4}", "[SSN]")
        .replace_regex(r"EMP-\d{6}", "[EMPLOYEE_ID]")
}
```

3. **Custom Sampling Decisions**
```rust
#[no_mangle]
pub fn should_sample(span: &Span) -> bool {
    // Company-specific logic
    // "Always keep traces from VIP customers"
    // "Sample 100% of traces over $1000"
    span.attributes.get("customer.tier") == "enterprise" ||
    span.attributes.get("order.value").unwrap_or(0) > 1000
}
```

4. **Data Enrichment**
```rust
#[no_mangle]
pub fn enrich(event: &mut Event) {
    // Add business context from internal systems
    let user_id = event.get("user_id");
    let user_info = lookup_user_cache(user_id);
    event.set("user.tier", user_info.tier);
    event.set("user.region", user_info.region);
}
```

5. **Custom Alerting Conditions**
```rust
#[no_mangle]
pub fn evaluate_alert(metrics: &MetricWindow) -> bool {
    // Complex business logic
    // "Alert if error rate > 5% AND it's business hours AND revenue > $10k/hr"
    let error_rate = metrics.get("error_rate");
    let is_business_hours = is_between(9, 17);
    let hourly_revenue = metrics.get("revenue_per_hour");

    error_rate > 0.05 && is_business_hours && hourly_revenue > 10000.0
}
```

**Implementation approach:**

```yaml
# dogwatch.yaml
plugins:
  wasm:
    enabled: true
    sandbox:
      memory_limit: 64MB
      timeout: 100ms
      capabilities: [http_client, kv_store]  # Explicitly granted

    modules:
      - name: pii-redactor
        path: /etc/dogwatch/plugins/pii.wasm
        hook: log_transform

      - name: custom-sampler
        path: /etc/dogwatch/plugins/sampler.wasm
        hook: trace_sample

      - name: proprietary-parser
        path: /etc/dogwatch/plugins/parser.wasm
        hook: protocol_parse
        protocols: [mycompany-rpc]
```

**Why Wasm specifically:**

| Approach | Problem |
|----------|---------|
| **Lua scripts** | Slow, no type safety, security concerns |
| **Go plugins** | Must match exact Go version, platform-specific |
| **gRPC plugins** | Network overhead, complexity |
| **Fork the code** | Maintenance nightmare |
| **Wasm** | Fast, sandboxed, polyglot, portable ✅ |

**Technical requirements:**

```go
// Wasm runtime integration
import "github.com/tetratelabs/wazero"

type WasmPlugin struct {
    runtime wazero.Runtime
    module  wazero.CompiledModule

    // Resource limits
    memoryLimit uint32
    timeout     time.Duration
}

func (p *WasmPlugin) Execute(input []byte) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
    defer cancel()

    // Instantiate with memory limits
    instance, err := p.runtime.InstantiateModule(ctx, p.module,
        wazero.NewModuleConfig().
            WithMemoryLimitPages(p.memoryLimit / 65536))
    if err != nil {
        return nil, err
    }
    defer instance.Close(ctx)

    // Call the exported function
    fn := instance.ExportedFunction("process")
    results, err := fn.Call(ctx, ...)
    return results, err
}
```

**Competitive advantage:**

```
Datadog: No user-defined processing. You get what they give you.
Grafana: Lua in Loki, limited. No Wasm.
Vector:  Has Wasm, but it's a separate tool.
Us:      Built-in Wasm = "observability your way, safely"

Marketing: "Your rules. Your logic. Your observability."
```

**Roadmap:**

| Phase | What | Priority |
|-------|------|----------|
| 1 | Log transform plugins | P2 |
| 2 | Custom sampling plugins | P2 |
| 3 | Protocol parser plugins | P2 |
| 4 | Alert condition plugins | P3 |
| 5 | Full pipeline customization | P3 |

**Recommendation:** This is a real differentiator. Plan for Phase 2-3 (after core stability). Use wazero (pure Go, no CGO) for the runtime.

---

#### Trend 8: Observability as Code / Terraform Integration ⚠️ PARTIAL

**The trend:** Managing observability alongside infrastructure.

**What this means:**
```hcl
# terraform/monitoring.tf
resource "dogwatch_dashboard" "api" {
  name = "API Overview"

  panel {
    title = "Request Rate"
    query = "rate(http_requests_total{service=\"api\"}[5m])"
  }
}

resource "dogwatch_alert" "error_rate" {
  name      = "High Error Rate"
  query     = "http_errors / http_requests > 0.05"
  threshold = 0.05
  channels  = [dogwatch_channel.slack.id]
}

resource "dogwatch_slo" "availability" {
  name   = "API Availability"
  target = 99.9

  indicator {
    good  = "http_status < 500"
    total = "http_requests"
  }
}
```

**Why it matters:**
- Infrastructure team already uses Terraform
- Version control for observability config
- Review process for alert changes
- Reproducible across environments

**Our position:** No Terraform provider.

**What we SHOULD build:**
```go
// Terraform provider for dogwatch
// github.com/dogwatch/terraform-provider-dogwatch

provider "dogwatch" {
  endpoint = "https://dogwatch.internal:9999"
  api_key  = var.dogwatch_api_key
}

// Resources to implement:
// - dogwatch_dashboard
// - dogwatch_alert
// - dogwatch_channel
// - dogwatch_slo
// - dogwatch_service
// - dogwatch_synthetic_check

// Data sources:
// - dogwatch_services (list discovered services)
// - dogwatch_metrics (query current values)
```

Priority: **P2**. Not core, but expected by platform teams.

---

### Modern Pain Points We Could Address

These are the problems teams are struggling with RIGHT NOW. Each pain point is analyzed in depth with specific solutions.

#### Pain Point 1: Alert Fatigue ⭐ CRITICAL

**The problem:**
- Too many alerts
- Alerts without context
- Duplicate alerts for same issue
- "Alert and forget" - nobody responds

**Current state of art:**
```
Bad: 50 alerts → Team ignores all
Good: 1 grouped alert with context → Team investigates

Grouped Alert:
"API degradation affecting checkout"
├── 12 related alerts (4 services affected)
├── Started: 14:32 (correlates with deploy at 14:30)
├── Impact: 12% of users seeing errors
├── Suggested: Rollback deploy v2.4.1
└── [Acknowledge] [Escalate] [Rollback]
```

**What we have:** Basic alerting.

**What we SHOULD build:**
```yaml
alerting:
  grouping:
    enabled: true
    group_by: [service, environment]
    group_wait: 30s      # Wait before sending
    group_interval: 5m   # How often to send updates

  context:
    include_related: true     # Show related alerts
    include_deploys: true     # Show recent deploys
    include_changes: true     # Show config changes
    include_impact: true      # Show user impact estimate

  intelligence:
    auto_resolve: true        # Resolve when condition clears
    flap_detection: true      # Don't alert on flapping
    root_cause_hint: true     # Suggest likely cause
```

Priority: **P0**. Alert fatigue is universal.

---

#### Pain Point 2: Tool Sprawl / Context Switching ⭐ CRITICAL

**The problem:**
```
Incident investigation:
1. PagerDuty (get alert)
2. Datadog (check metrics)
3. Splunk (search logs)
4. Jaeger (find traces)
5. Grafana (check dashboard)
6. Slack (ask team)
7. GitHub (check recent commits)
8. AWS Console (check resources)

8 tools! 15 minutes just to understand what's happening.
```

**Our position:** This is our core value prop - single pane.

**What we SHOULD emphasize:**
```
dogwatch investigation:
1. Alert comes in with full context
2. See metrics, logs, traces in one view
3. Recent deploys shown inline
4. Related services highlighted
5. Suggested actions provided

1 tool. 2 minutes to understand.
```

**Recommendation:** Double down on the integrated experience. Every feature should reduce context switching, not add tabs.

---

#### Pain Point 3: Cardinality Explosions ⭐ HIGH

**The problem:**
```
Innocent metric:
  http_requests{user_id="..."}

With 1M users = 1M time series
Storage explodes. Queries slow. Bills spike.

Team doesn't know which metric caused it.
```

**What we have:** Control Plane (cardinality analysis) planned.

**What we SHOULD build:**
```yaml
cardinality:
  analysis:
    enabled: true
    schedule: hourly

  alerts:
    - name: cardinality_spike
      condition: series_count > previous_hour * 1.5
      channels: [platform-team]

  controls:
    - name: limit_user_id
      pattern: "*{user_id=*}"
      action: drop
      threshold: 10000 series

  dashboard:
    show_top_contributors: true
    show_growth_trends: true
    estimate_storage_cost: true
```

Priority: **P1**. Cardinality is how bills explode.

---

#### Pain Point 4: Kubernetes Complexity ⭐ HIGH

**The problem:**
```
"The pod is crashlooping"

Which pod? In which namespace? On which node?
What's the event log? What do the logs say?
Is it OOMKilled? Is there a liveness probe failing?
What's the deployment history?
```

**What we have:** Container/K8s basic support.

**What we SHOULD build:**
```
Kubernetes-native views:

Cluster Overview
├── Nodes (10/10 healthy)
│   └── worker-1: CPU 67%, Memory 45%, 23 pods
├── Namespaces
│   ├── production (142 pods, 3 alerts)
│   ├── staging (45 pods, 0 alerts)
│   └── monitoring (12 pods, 0 alerts)
└── Problems
    ├── CrashLoopBackOff: api-service-abc123
    ├── OOMKilled: worker-def456 (3 times in 1h)
    └── Pending: batch-job-ghi789 (no resources)

Pod Detail:
├── Events: ImagePullBackOff → Running → OOMKilled → CrashLoopBackOff
├── Resources: requests 256Mi, limits 512Mi, used 498Mi (97%)
├── Logs: [last 100 lines, searchable]
├── Traces: [requests handled by this pod]
└── Previous instances: [link to logs/traces]
```

Priority: **P1**. Everyone runs K8s. Everyone struggles with observability in K8s.

---

#### Pain Point 5: Slow Incident Response ⭐ HIGH

**The problem:**
```
Alert fires → 45 minutes to find root cause

Time breakdown:
- 5min: Notice alert, context switch
- 10min: Open tools, remember how to query
- 15min: Search for relevant data
- 10min: Correlate across tools
- 5min: Actually understand the problem
```

**What we SHOULD build:**
```yaml
# Incident auto-enrichment
incidents:
  auto_enrichment:
    enabled: true
    include:
      - recent_deploys        # What changed?
      - related_alerts        # What else is firing?
      - top_errors           # What's failing?
      - slow_queries         # What's slow?
      - similar_incidents    # When did this happen before?

  runbooks:
    enabled: true
    auto_suggest: true       # Based on alert type

  timeline:
    auto_build: true         # Show sequence of events

# Example incident view:
# "API Error Rate High"
# ├── Timeline
# │   ├── 14:30 - Deploy v2.4.1
# │   ├── 14:32 - Error rate started increasing
# │   ├── 14:35 - First alert fired
# │   └── 14:36 - Database connection errors spike
# ├── Likely cause: Deploy v2.4.1 (highest correlation)
# ├── Similar incidents: INC-1234 (2024-01-05, same pattern)
# └── Suggested action: Rollback deploy
```

Priority: **P0**. Reducing MTTR is measurable value.

---

#### Pain Point 6: Developer Experience Gap ⭐ MEDIUM

**The problem:**
```
Ops tools built for SREs, not developers.
Developers need:
- "Why is MY code slow?"
- "What's causing MY errors?"
- "How is MY service doing?"

Not:
- "Write PromQL to query metrics"
- "Navigate 50 dashboards"
- "Correlate 3 tools manually"
```

**What we SHOULD build:**
```
Developer-centric views:

My Services:
├── api-gateway (you own this)
│   ├── Health: 99.2% (target: 99.5%) ⚠️
│   ├── Latency: p99 234ms (target: 200ms) ⚠️
│   ├── Errors: 12 in last hour
│   └── [View details] [See my changes]
└── payment-service (you own this)
    ├── Health: 99.9% ✅
    └── [View details]

My Recent Changes:
├── PR #1234: "Add retry logic" (deployed 2h ago)
│   └── Impact: Error rate -23% ✅
└── PR #1198: "Optimize query" (deployed yesterday)
    └── Impact: p99 latency -45% ✅

My On-Call:
├── Next shift: Tomorrow 9am - 5pm
└── Current incidents: 0
```

Priority: **P1**. Developer adoption drives platform adoption.

---

#### Pain Point 7: On-Call Burnout ⭐ MEDIUM

**The problem:**
- Too many pages
- Pages for non-actionable issues
- No context in pages
- Same person always on-call

**Our position:** We said "integrate with PagerDuty, don't rebuild."

**What we SHOULD add:**
```yaml
on_call:
  integration: pagerduty

  noise_reduction:
    # Don't page for things that auto-resolve
    min_duration: 5m

    # Don't page for low-impact issues at night
    night_mode:
      hours: 22:00-07:00
      severity_threshold: critical

    # Group related alerts
    grouping: true

  context:
    # Include everything needed to investigate
    include_runbook: true
    include_related: true
    include_timeline: true

  metrics:
    # Track on-call health
    pages_per_shift: true
    mttr: true
    false_positive_rate: true
```

Priority: **P2**. Important but PagerDuty handles core functionality.

---

#### Pain Point 8: Observability Data Silos ⭐ CRITICAL

**The problem:**
```
Metrics in Prometheus
Logs in Elasticsearch
Traces in Jaeger
Errors in Sentry
Uptime in Pingdom
Costs in CloudHealth

"The dashboard shows high latency"
"Let me check the logs... different tool, different query language"
"Now let me find the trace... another tool, need the trace ID"
"Was there a deploy? Check yet another system..."

Result: 30 minutes to correlate what should take 30 seconds
```

**The deeper issue:**
```
Data exists but isn't connected:

Metric: api_latency_p99 = 2.3s (high!)
  └── Which requests? → Need to query logs
      └── What's the trace? → Need to query traces
          └── What changed? → Need to query deploys
              └── Who's affected? → Need to query business data

Each hop = context switch, different tool, lost time
```

**What we SHOULD build:**
```
One-click correlation:

┌─────────────────────────────────────────────────────────────┐
│ Metric: api_latency_p99 = 2.3s                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Related Traces (slowest):                                    │
│ ├── trace-abc123: GET /api/orders (2.8s) - user: john@...   │
│ ├── trace-def456: GET /api/orders (2.6s) - user: jane@...   │
│ └── [View all 847 slow traces]                              │
│                                                              │
│ Related Logs:                                                │
│ ├── 14:32:01 WARN Connection pool exhausted                  │
│ ├── 14:32:02 ERROR Query timeout after 2000ms               │
│ └── [View all 234 related logs]                             │
│                                                              │
│ Recent Changes:                                              │
│ ├── 14:30:00 Deploy v2.4.1 (api-service)                    │
│ └── 14:28:00 Config change: max_connections 50→25           │
│                                                              │
│ Likely Root Cause: Config change reduced connection pool     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Implementation:**
```go
// Unified correlation engine
type CorrelationEngine struct {
    metrics  MetricsStore
    logs     LogsStore
    traces   TracesStore
    events   EventsStore
}

func (c *CorrelationEngine) Correlate(ctx context.Context, anchor Anchor) *CorrelatedView {
    // Time window around the anchor
    window := TimeWindow{
        Start: anchor.Time.Add(-5 * time.Minute),
        End:   anchor.Time.Add(5 * time.Minute),
    }

    // Pull related data in parallel
    var wg sync.WaitGroup
    var traces []Trace
    var logs []Log
    var events []Event

    wg.Add(3)
    go func() { traces = c.traces.FindByService(anchor.Service, window); wg.Done() }()
    go func() { logs = c.logs.FindByService(anchor.Service, window); wg.Done() }()
    go func() { events = c.events.FindByService(anchor.Service, window); wg.Done() }()
    wg.Wait()

    // Build correlation links
    return &CorrelatedView{
        Anchor:       anchor,
        RelatedTraces: rankByRelevance(traces, anchor),
        RelatedLogs:   rankByRelevance(logs, anchor),
        RecentEvents:  events,
        LikelyCause:   inferCause(traces, logs, events),
    }
}
```

Priority: **P0**. This IS our core value prop. Every feature must strengthen correlation.

---

#### Pain Point 9: Cost Unpredictability ⭐ CRITICAL

**The problem:**
```
Month 1: "Observability costs $5,000"
Month 2: "Observability costs $12,000" 😱
Month 3: "Observability costs $28,000" 🔥

What happened?
- New team added logging → +$3,000
- Someone enabled debug logs → +$5,000
- Cardinality explosion in metrics → +$8,000
- New synthetic checks → +$2,000

Nobody knew until the bill arrived.
```

**The deeper issue:**
```
Cost is invisible until it's a problem:

Developer adds:
  logger.debug(f"Processing order {order_id} for user {user_id}")

Seems harmless. But:
  - 1M orders/day × 365 days = 365M log lines/year
  - At $0.10/GB ingestion = $3,650/year for ONE log line

Nobody told the developer. No feedback loop.
```

**What competitors do:**
- Datadog: Bill shock, then you scramble
- Grafana Cloud: Complex pricing calculator
- New Relic: Per-GB pricing, hard to predict

**What we SHOULD build:**
```
Real-time cost visibility:

┌─────────────────────────────────────────────────────────────┐
│ Cost Intelligence Dashboard                                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Current Month (Jan 15):                                      │
│ ├── Projected: $8,234 (at current rate)                     │
│ ├── Budget: $6,000                                          │
│ └── Status: ⚠️ 37% over budget                              │
│                                                              │
│ Top Cost Drivers:                                            │
│ ├── payment-service logs: $2,100 (+340% vs last month)      │
│ │   └── ⚠️ Debug logging enabled 3 days ago                 │
│ ├── api-gateway metrics: $1,800 (cardinality: 45,000)       │
│ │   └── ⚠️ user_id label causing explosion                  │
│ └── synthetic checks: $890 (247 checks @ $3.60 each)        │
│                                                              │
│ Recommendations:                                             │
│ ├── Disable debug logs on payment-service: -$1,800/month    │
│ ├── Drop user_id label from metrics: -$1,200/month          │
│ └── Reduce synthetic frequency (5m→15m): -$593/month        │
│                                                              │
│ If we did nothing: $12,400/month by end of month            │
│ If we follow recommendations: $5,800/month ✅                │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Cost attribution by team:**
```yaml
# Config
cost_allocation:
  enabled: true
  group_by: [team, service, environment]

  budgets:
    - team: payments
      monthly_limit: 2000
      alert_at: [80%, 100%, 120%]

    - team: platform
      monthly_limit: 3000
      alert_at: [80%, 100%]

  showback:
    # Weekly email to team leads
    enabled: true
    schedule: "0 9 * * 1"  # Monday 9am
```

**Developer feedback loop:**
```
# In CI/CD pipeline
$ dogwatch cost-estimate --diff HEAD~1

Cost Impact of This Change:
├── New metric: order_processing_time{user_id=*}
│   └── Estimated: +$1,200/month (high cardinality warning!)
├── New log statement: logger.info("Order processed")
│   └── Estimated: +$45/month (1M events × $0.045/M)
└── Total estimated impact: +$1,245/month

⚠️ Consider: Remove user_id label or use exemplars instead

[Block PR] [Approve with warning] [Ignore]
```

Priority: **P0**. Cost transparency is our #2 killer feature.

---

#### Pain Point 10: Compliance & Audit Gaps ⭐ HIGH

**The problem:**
```
Auditor: "Show me who accessed production data in the last 90 days"
Team: "Uh... we don't track that"

Auditor: "Show me all configuration changes to alerting"
Team: "We'd have to dig through git history... maybe"

Auditor: "Prove this PII was never logged"
Team: "We... can't prove a negative"

Result: Failed audit, scramble to implement, 6-month remediation
```

**What enterprises need:**
```
1. Access audit trail
   - Who logged in when
   - What queries they ran
   - What data they accessed
   - What changes they made

2. Configuration change history
   - All changes to alerts, dashboards, settings
   - Who made them, when, why
   - Ability to diff and rollback

3. Data governance
   - What data contains PII
   - Who can access it
   - Retention policies enforced
   - Proof of deletion

4. Export for compliance tools
   - SIEM integration
   - GRC platform integration
   - Audit report generation
```

**What we SHOULD build:**
```yaml
# Audit logging
audit:
  enabled: true
  retention: 2y  # Keep audit logs longer than data

  log_events:
    - user_login
    - user_logout
    - query_executed
    - dashboard_viewed
    - alert_modified
    - config_changed
    - data_exported
    - user_permission_changed

  export:
    - type: siem
      target: splunk
      filter: "severity >= warning"

    - type: s3
      bucket: audit-logs
      prefix: dogwatch/
      format: json

# Example audit entry
{
  "timestamp": "2024-01-15T14:32:01Z",
  "event": "query_executed",
  "user": "john@example.com",
  "ip": "10.0.1.50",
  "query": "logs{service=\"payment\"} |= \"credit_card\"",
  "result_count": 47,
  "duration_ms": 234,
  "data_scanned_bytes": 1048576
}
```

**Compliance reports:**
```
GET /api/compliance/report?type=access&start=2024-01-01&end=2024-03-31

Response:
{
  "report_type": "access_audit",
  "period": "2024-Q1",
  "summary": {
    "total_users": 45,
    "total_queries": 12847,
    "unique_data_accessed": ["logs", "traces", "metrics"],
    "pii_queries": 23,
    "after_hours_access": 156
  },
  "details": [
    {
      "user": "john@example.com",
      "role": "admin",
      "queries": 892,
      "last_access": "2024-03-31T16:42:00Z",
      "pii_access": false
    }
  ]
}
```

Priority: **P1** (P0 for enterprise sales). Required for SOC2/HIPAA customers.

---

#### Pain Point 11: Multi-Cloud / Hybrid Visibility ⭐ HIGH

**The problem:**
```
Reality of modern infrastructure:
├── AWS (us-east-1): Main workloads
├── AWS (eu-west-1): GDPR compliance
├── GCP: ML pipelines (better TPU pricing)
├── Azure: Microsoft integrations
├── On-prem: Legacy systems, data sovereignty
└── Edge: CDN, IoT devices

Each cloud has its own:
- Monitoring tools (CloudWatch, Stackdriver, Azure Monitor)
- Log format
- Metric naming conventions
- Authentication

Result: No unified view. Incidents spanning clouds = nightmare.
```

**Real scenario:**
```
User complaint: "The app is slow"

Investigation:
1. Frontend (Cloudflare) - looks fine
2. API Gateway (AWS) - latency spike!
3. But it's calling ML service (GCP) - where's that data?
4. ML service calls on-prem database - no observability there
5. 3 hours later: on-prem network switch was flapping

With unified observability: 10 minutes
Without: 3 hours
```

**What we SHOULD build:**
```yaml
# Multi-environment configuration
environments:
  aws-prod:
    type: aws
    region: us-east-1
    metrics_source: cloudwatch
    logs_source: cloudwatch_logs
    credentials:
      role_arn: arn:aws:iam::123456789:role/dogwatch

  gcp-ml:
    type: gcp
    project: ml-pipelines-prod
    metrics_source: cloud_monitoring
    logs_source: cloud_logging
    credentials:
      service_account: /etc/dogwatch/gcp-sa.json

  onprem-dc:
    type: custom
    endpoints:
      metrics: http://prometheus.internal:9090
      logs: http://loki.internal:3100
      traces: http://jaeger.internal:16686

# Unified service map
# dogwatch correlates across environments automatically:
#
# ┌─────────────────────────────────────────────────────────────┐
# │                     Unified Service Map                      │
# │                                                              │
# │  ┌──────────┐     ┌──────────┐     ┌──────────┐             │
# │  │ Frontend │────▶│   API    │────▶│    ML    │             │
# │  │ (CDN)    │     │  (AWS)   │     │  (GCP)   │             │
# │  └──────────┘     └────┬─────┘     └────┬─────┘             │
# │                        │                │                    │
# │                        ▼                ▼                    │
# │                   ┌──────────┐    ┌──────────┐              │
# │                   │ Database │    │  Legacy  │              │
# │                   │  (AWS)   │    │ (On-prem)│              │
# │                   └──────────┘    └──────────┘              │
# └─────────────────────────────────────────────────────────────┘
```

**Cross-cloud tracing:**
```go
// Trace that spans clouds
Trace: abc-123
├── Span: frontend (Cloudflare)
│   └── 45ms
├── Span: api-gateway (AWS us-east-1)
│   ├── 234ms
│   └── Child: database query (AWS RDS)
│       └── 89ms
├── Span: ml-inference (GCP us-central1)
│   ├── 567ms
│   └── Child: model-load (GCP)
│       └── 123ms
└── Span: legacy-validation (on-prem)
    └── 1.2s  ← BOTTLENECK IDENTIFIED

Total: 2.1s
Root cause: Legacy on-prem validation (57% of total)
```

Priority: **P1**. Modern architectures are multi-cloud. This is table stakes.

---

#### Pain Point 12: Ephemeral Infrastructure ⭐ HIGH

**The problem:**
```
Old world:
- 10 servers
- Each has a name (web-1, web-2, db-master)
- Servers live for years
- You can SSH in and debug

New world:
- 1000 containers
- Names are random (api-7d4f8b9c-xyz)
- Containers live for hours or minutes
- They're gone before you can debug
- Lambda functions exist for milliseconds
```

**Specific challenges:**
```
Serverless (Lambda/Cloud Functions):
├── Starts cold (1.2s)
├── Runs for 200ms
├── Dies
├── No persistent logs
├── No way to attach debugger
└── Problem: "Which invocation had the error?"

Kubernetes pods:
├── Pod starts
├── Runs for 2 hours
├── Gets OOMKilled
├── New pod starts (different name, different node)
└── Problem: "What happened to the old pod?"

Spot instances:
├── Running fine
├── AWS reclaims instance (2-minute warning)
├── Workload migrates
├── Logs/metrics gaps during migration
└── Problem: "Why is there missing data?"
```

**What we SHOULD build:**
```yaml
# Ephemeral-aware data model
entities:
  # Track entity lifecycle
  - type: container
    id: api-7d4f8b9c-xyz
    lifecycle:
      created: 2024-01-15T14:00:00Z
      terminated: 2024-01-15T16:32:00Z
      termination_reason: OOMKilled
    parent: pod/api-deployment-abc123
    # All data tagged with entity lifecycle

  - type: lambda
    id: process-orders-v3
    invocations:
      - request_id: abc-123
        started: 2024-01-15T14:32:01.234Z
        duration_ms: 456
        cold_start: true
        memory_used_mb: 234
        # Logs and traces linked to this invocation

# Query by entity, even after it's gone
GET /api/entities/container/api-7d4f8b9c-xyz

Response:
{
  "entity": {
    "type": "container",
    "id": "api-7d4f8b9c-xyz",
    "status": "terminated",
    "lifetime": "2h 32m",
    "termination": "OOMKilled"
  },
  "links": {
    "logs": "/api/logs?entity=api-7d4f8b9c-xyz",
    "traces": "/api/traces?entity=api-7d4f8b9c-xyz",
    "metrics": "/api/metrics?entity=api-7d4f8b9c-xyz",
    "previous": "/api/entities/container/api-6c3e7a8b-wxy",
    "replacement": "/api/entities/container/api-8e5f9c0d-zab"
  }
}
```

**Lambda-specific views:**
```
Lambda Function: process-orders

Invocations (last hour):
├── Total: 12,847
├── Errors: 23 (0.18%)
├── Cold starts: 89 (0.7%)
├── Avg duration: 234ms
├── p99 duration: 890ms
└── Timeouts: 2

Error breakdown:
├── ValidationError: 12 (52%)
├── TimeoutError: 2 (9%)
├── DatabaseError: 9 (39%)

Cold start analysis:
├── Avg cold start: 1.2s
├── Warm start: 45ms
├── Provisioned concurrency: Not enabled
└── Recommendation: Enable provisioned concurrency (saves ~$200/month in user-facing latency)

[View invocations] [View errors] [Compare versions]
```

Priority: **P1**. Serverless and K8s are the norm. Must handle ephemeral well.

---

#### Pain Point 13: Microservices Dependency Hell ⭐ HIGH

**The problem:**
```
api-gateway → user-service → auth-service → redis
          ↘ order-service → payment-service → stripe
                         → inventory-service → database
                         → shipping-service → fedex-api

"Why is checkout slow?"
- Is it order-service? Or something it calls?
- Is it payment-service? Or Stripe?
- Is it inventory? Or the database?
- Is it all of them?

100+ services = impossible to understand dependencies manually
```

**The cascade effect:**
```
14:30:00 - Redis latency increases (50ms → 500ms)
14:30:01 - auth-service slows down (calls Redis)
14:30:02 - user-service slows down (calls auth-service)
14:30:03 - api-gateway slows down (calls user-service)
14:30:04 - ALL endpoints slow

Alert: "api-gateway latency high"
Team debugs api-gateway for 30 minutes
Actual cause: Redis memory pressure

Without dependency awareness: 45 minutes
With dependency awareness: 2 minutes
```

**What we SHOULD build:**
```
Dependency-aware alerting:

┌─────────────────────────────────────────────────────────────┐
│ Alert: api-gateway latency high                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Dependency Analysis:                                         │
│                                                              │
│ api-gateway (this service)                                   │
│ ├── Self time: 12ms (normal)                                │
│ └── Waiting on dependencies: 1.8s (HIGH)                    │
│     ├── user-service: 800ms (usually 50ms) ⚠️               │
│     │   └── Waiting on: auth-service: 750ms ⚠️              │
│     │       └── Waiting on: redis: 700ms 🔴 ROOT CAUSE      │
│     └── order-service: 1.0s (usually 100ms) ⚠️              │
│         └── Also waiting on: auth-service (same issue)      │
│                                                              │
│ Root Cause: redis latency (700ms, usually 5ms)              │
│                                                              │
│ Affected Services: 8 (all depend on auth → redis)           │
│ Affected Endpoints: 23                                       │
│ Affected Users: ~12% seeing slow responses                  │
│                                                              │
│ [View Redis Dashboard] [View Dependency Map] [Create Incident]│
└─────────────────────────────────────────────────────────────┘
```

**Service health rollup:**
```
Service Health (real-time)

Healthy (72):
├── payment-service ✅
├── inventory-service ✅
└── ... 70 more

Degraded (5):
├── user-service ⚠️ (slow, caused by redis)
├── auth-service ⚠️ (slow, caused by redis)
├── api-gateway ⚠️ (slow, caused by redis)
├── order-service ⚠️ (slow, caused by redis)
└── checkout-service ⚠️ (slow, caused by redis)

Root Causes:
└── redis 🔴 (1 root cause affecting 5 services)
```

**Dependency change detection:**
```yaml
# Alert on unexpected dependencies
dependencies:
  change_detection:
    enabled: true
    alert_on:
      - new_dependency          # "frontend now calls database directly?!"
      - removed_dependency      # "why did auth stop calling redis?"
      - increased_call_rate     # "payment now calls stripe 10x more"
      - new_external_dependency # "who added call to unknown-api.com?"
```

Priority: **P0**. Microservices without dependency tracking is flying blind.

---

#### Pain Point 14: Lack of Business Context ⭐ HIGH

**The problem:**
```
Technical alert: "Error rate > 5%"

What the engineer sees:
- HTTP 500 errors
- Stack traces
- Service name

What they DON'T see:
- Is this affecting revenue?
- Which customers?
- How much money are we losing?
- Is this a $10 problem or a $100,000 problem?

CTO: "Is this critical?"
Engineer: "Uh... it's a 5% error rate?"
CTO: "What does that MEAN for the business?"
Engineer: "..."
```

**The gap:**
```
Technical Data              Business Data
─────────────              ─────────────
500 errors/min             ??? customers affected
p99 = 2.3s                 ??? revenue impacted
3 services degraded        ??? SLA breach?
Database at 90% CPU        ??? cost of downtime?

These SHOULD be connected but usually aren't.
```

**What we SHOULD build:**
```yaml
# Business context configuration
business_context:
  # Link services to business impact
  services:
    checkout-service:
      revenue_per_request: $45  # Average order value
      sla_tier: critical        # Business-critical
      customer_segment: all

    internal-tools:
      revenue_per_request: $0
      sla_tier: low
      customer_segment: internal

  # Alert enrichment
  enrichment:
    enabled: true
    include:
      - estimated_revenue_impact
      - affected_customer_count
      - affected_customer_tiers
      - sla_status
```

**Business-aware dashboards:**
```
┌─────────────────────────────────────────────────────────────┐
│ Business Impact Dashboard                                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Current Incidents:                                           │
│                                                              │
│ 🔴 CRITICAL: Checkout failing for 12% of users              │
│    ├── Revenue impact: -$4,230/hour                          │
│    ├── Affected customers: 847 (23 enterprise)              │
│    ├── Started: 14:32 (18 minutes ago)                      │
│    ├── Estimated loss so far: $1,269                        │
│    └── SLA status: BREACHING (99.9% target, currently 88%)  │
│                                                              │
│ ⚠️ WARNING: Search slow for mobile users                     │
│    ├── Revenue impact: -$890/hour (estimated)               │
│    ├── Affected customers: 2,341 (mobile only)              │
│    └── SLA status: OK (within targets)                      │
│                                                              │
│ Today's Summary:                                             │
│ ├── Revenue protected: $234,567                              │
│ ├── Revenue at risk: $5,120                                  │
│ ├── Incidents: 3 (2 resolved, 1 active)                     │
│ └── SLA status: 99.2% (target: 99.9%) ⚠️                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Revenue-aware alerting:**
```yaml
alerts:
  - name: checkout-errors
    condition: error_rate{service="checkout"} > 0.01
    severity: critical

    # Business context
    business_impact:
      revenue_per_error: $45
      calculate_impact: true

    # Alert includes:
    # "Checkout errors: 234/hour
    #  Estimated revenue impact: $10,530/hour
    #  Affected customers: 234 (12 enterprise tier)"

    # Escalation based on business impact
    escalation:
      - if: revenue_impact > 1000/hour
        then: page_oncall
      - if: revenue_impact > 10000/hour
        then: page_engineering_lead
      - if: revenue_impact > 50000/hour
        then: page_cto
```

Priority: **P1**. Connects technical work to business outcomes. Executives love this.

---

#### Pain Point 15: Security Blind Spots ⭐ HIGH

**The problem:**
```
Security team: "Was there any unusual access to the user database yesterday?"
Ops team: "We don't track that in our observability"
Security team: "Can you check for SQL injection attempts?"
Ops team: "We'd have to grep through logs manually"
Security team: "Were there any data exfiltration attempts?"
Ops team: "We have no way to know"

Security and observability are separate worlds.
But attackers don't care about organizational silos.
```

**What observability can see (but usually doesn't surface):**
```
eBPF sees EVERYTHING:
├── Every network connection (including unusual destinations)
├── Every file access (including /etc/passwd)
├── Every process spawn (including reverse shells)
├── Every syscall (including privilege escalation)

Logs contain:
├── Authentication attempts (including brute force)
├── SQL queries (including injection attempts)
├── API access patterns (including scraping)
├── Error messages (including path traversal attempts)

Traces show:
├── Request paths (including unusual patterns)
├── User sessions (including session hijacking)
├── Data access (including unauthorized access)
```

**What we SHOULD build:**
```yaml
# Security-focused observability
security:
  enabled: true

  # Threat detection from existing data
  detection_rules:
    - name: brute_force_login
      condition: |
        count(logs{message=~"login failed"}) by (source_ip) > 10
        within 5m
      severity: high
      action: alert

    - name: sql_injection_attempt
      condition: |
        http.path matches "('|--|UNION|SELECT|DROP)"
      severity: critical
      action: [alert, block_ip]

    - name: unusual_data_access
      condition: |
        database.rows_returned > 10000 AND
        user NOT IN known_batch_users
      severity: medium
      action: alert

    - name: lateral_movement
      condition: |
        new_internal_connection AND
        source.service NOT IN destination.allowed_callers
      severity: high
      action: alert

    - name: data_exfiltration
      condition: |
        outbound.bytes > 100MB AND
        destination NOT IN known_destinations
      severity: critical
      action: [alert, block]

  # Security dashboard
  dashboard:
    show_auth_failures: true
    show_unusual_access: true
    show_threat_map: true
    show_compliance_status: true

  # SIEM integration
  export:
    siem:
      enabled: true
      target: splunk
      events: [high, critical]
```

**Security incident view:**
```
┌─────────────────────────────────────────────────────────────┐
│ Security Alert: Potential Data Exfiltration                 │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Detected: 2024-01-15 14:32:01                               │
│ Severity: CRITICAL                                           │
│                                                              │
│ What happened:                                               │
│ ├── User: john@example.com (session: abc123)                │
│ ├── Query: SELECT * FROM users (returned 847,234 rows)      │
│ ├── Data size: 234MB                                        │
│ ├── Normal behavior: <1000 rows, <1MB                       │
│ └── Destination: Downloaded to client                        │
│                                                              │
│ Context:                                                     │
│ ├── User role: support (should not access all users)        │
│ ├── Time: 2:32 AM (outside normal hours)                    │
│ ├── Location: IP 45.67.89.12 (VPN: Russia)                  │
│ └── Recent activity: Password changed 2 hours ago           │
│                                                              │
│ Risk Assessment: HIGH                                        │
│ ├── Possible compromised account                            │
│ ├── Possible insider threat                                 │
│ └── Data may include PII                                    │
│                                                              │
│ [Block User] [Revoke Sessions] [Create Incident] [Ignore]   │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P1**. Security + observability convergence is a major trend.

---

#### Pain Point 16: Legacy System Visibility ⭐ MEDIUM

**The problem:**
```
Modern stack:
├── Kubernetes (full observability)
├── Cloud functions (full observability)
└── Easy to monitor

Legacy stack:
├── COBOL on mainframe (no agents allowed)
├── Java 6 on ancient Tomcat (can't update)
├── Windows Server 2008 (please don't ask)
└── "It works, don't touch it"

Reality: Legacy systems often handle critical business logic.
"The mainframe processes $2B in transactions daily."
"We can't see ANY of that in our dashboards."
```

**Approaches we SHOULD support:**
```yaml
# Non-invasive legacy monitoring
legacy_systems:

  # Option 1: Network-level observation
  - name: mainframe-transactions
    type: network_tap
    capture:
      interface: eth0
      filter: "host 10.0.1.50 and port 3270"  # TN3270
    parse:
      protocol: tn3270
      extract: [transaction_id, response_code, duration]

  # Option 2: Log file tailing
  - name: legacy-java-app
    type: file_tail
    paths:
      - /var/log/legacy-app/*.log
    parse:
      format: regex
      pattern: '(?P<timestamp>\d{4}-\d{2}-\d{2}) (?P<level>\w+) (?P<message>.*)'

  # Option 3: Database query (observe the data, not the app)
  - name: mainframe-batch-jobs
    type: database_poll
    connection: oracle://readonly:***@legacy-db:1521/prod
    query: |
      SELECT job_name, status, duration_seconds, error_message
      FROM batch_job_log
      WHERE completed_at > :last_poll
    interval: 60s

  # Option 4: SNMP for ancient infrastructure
  - name: legacy-load-balancer
    type: snmp
    host: 10.0.1.100
    community: public
    oids:
      - 1.3.6.1.2.1.2.2.1.10  # ifInOctets
      - 1.3.6.1.2.1.2.2.1.16  # ifOutOctets

  # Option 5: Synthetic probes
  - name: mainframe-health
    type: synthetic
    check:
      type: tcp
      host: 10.0.1.50
      port: 3270
    assertions:
      - response_time: <100ms
```

**Legacy in the service map:**
```
┌─────────────────────────────────────────────────────────────┐
│                      Service Map                             │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│    ┌──────────┐     ┌──────────┐     ┌──────────────────┐  │
│    │   API    │────▶│ Payment  │────▶│    Mainframe     │  │
│    │ Gateway  │     │ Service  │     │  (limited view)  │  │
│    └──────────┘     └──────────┘     └──────────────────┘  │
│         │                                     │             │
│         │           Observability:            │             │
│         │           ├── Full telemetry        │             │
│         │           └── eBPF tracing          │             │
│         │                                     │             │
│         │           Observability:            │             │
│         │           ├── Network latency only  │             │
│         │           ├── Success/failure       │             │
│         │           └── No internal visibility│             │
│                                                              │
│    Legend: ████ Full visibility  ░░░░ Limited visibility   │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P2**. Not everyone has legacy, but those who do really need this.

---

#### Pain Point 17: Observability of Observability ⭐ MEDIUM

**The problem:**
```
"Our monitoring is down"
"How do you know?"
"... I don't"

The irony: Observability systems need observability too.
And they're often the least monitored thing in the stack.
```

**What can go wrong:**
```
Scenario 1: Silent failure
├── Metrics ingestion stops
├── No alerts fire (because no metrics!)
├── Hours pass before someone notices
└── "Why didn't we get alerted about the outage?"
   "Because the alerting system was down"

Scenario 2: Data loss
├── Log buffer overflows
├── Logs dropped silently
├── Investigation fails ("where are the logs?!")
└── "They were dropped, we just didn't know"

Scenario 3: Delayed data
├── Processing lag increases
├── Dashboards show stale data
├── Decisions made on old information
└── "This says everything is fine" [narrator: it wasn't]
```

**What we SHOULD build:**
```yaml
# Self-monitoring
self_monitoring:
  enabled: true

  # Internal metrics (always on)
  metrics:
    - dogwatch_events_received_total
    - dogwatch_events_dropped_total
    - dogwatch_events_processed_total
    - dogwatch_processing_latency_seconds
    - dogwatch_storage_bytes
    - dogwatch_query_latency_seconds
    - dogwatch_active_connections
    - dogwatch_error_total

  # Health checks
  health:
    endpoints:
      - /health          # Basic liveness
      - /health/detailed # Component-level health
    components:
      - name: metrics_ingestion
        check: events_received > 0 in last 60s
      - name: log_ingestion
        check: logs_received > 0 in last 60s
      - name: storage
        check: disk_free > 10%
      - name: queries
        check: query_latency_p99 < 5s

  # External monitoring (push-based)
  external:
    enabled: true
    push_to:
      - type: heartbeat
        url: https://healthchecks.io/ping/abc123
        interval: 60s
      - type: metrics
        url: https://backup-metrics.example.com/push
        interval: 60s

  # Alerting on self
  alerts:
    - name: ingestion_stopped
      condition: rate(events_received[5m]) == 0
      severity: critical
      notify: [pagerduty, slack, email]  # ALL channels

    - name: high_drop_rate
      condition: rate(events_dropped[5m]) / rate(events_received[5m]) > 0.01
      severity: warning

    - name: storage_full
      condition: disk_free_percent < 10
      severity: critical

    - name: processing_lag
      condition: processing_lag_seconds > 60
      severity: warning
```

**Self-monitoring dashboard:**
```
┌─────────────────────────────────────────────────────────────┐
│ dogwatch Health                                              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Status: HEALTHY ✅                                           │
│                                                              │
│ Ingestion:                                                   │
│ ├── Events/sec: 12,847                                      │
│ ├── Dropped: 0 (0%)                                         │
│ └── Lag: 1.2s                                               │
│                                                              │
│ Storage:                                                     │
│ ├── Used: 234 GB / 500 GB (47%)                             │
│ ├── Write rate: 12 MB/s                                     │
│ └── Retention: 14 days                                      │
│                                                              │
│ Queries:                                                     │
│ ├── Active: 23                                              │
│ ├── p50 latency: 45ms                                       │
│ └── p99 latency: 890ms                                      │
│                                                              │
│ Components:                                                  │
│ ├── eBPF agent: ✅ (12 probes active)                       │
│ ├── Metrics engine: ✅                                      │
│ ├── Log processor: ✅                                       │
│ ├── Trace collector: ✅                                     │
│ ├── Alert evaluator: ✅ (47 rules, 2 firing)               │
│ └── Web UI: ✅                                              │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P1**. Embarrassing when your monitoring is unmonitored.

---

#### Pain Point 18: Scaling Observability Itself ⭐ MEDIUM

**The problem:**
```
Small scale: 10 services, 100 metrics, 1GB logs/day
→ Everything works fine

Medium scale: 100 services, 10,000 metrics, 100GB logs/day
→ Queries getting slow, storage filling up

Large scale: 1000 services, 1M metrics, 10TB logs/day
→ Everything is on fire

Observability tools become the bottleneck.
"We can't add more metrics, the system can't handle it"
"We had to disable debug logging, too expensive"
```

**Scaling challenges:**
```
1. Cardinality explosion
   └── 1000 services × 100 metrics × 10 labels = 1M time series

2. Storage growth
   └── 10TB/day × 30 days retention = 300TB storage

3. Query performance
   └── Aggregating 1M series = seconds or minutes

4. Ingestion throughput
   └── 1M events/second needs serious infrastructure

5. Cost explosion
   └── All the above = $$$
```

**What we SHOULD build:**
```yaml
# Scaling configurations
scaling:

  # Tier 1: Small (<100 services)
  small:
    deployment: single-binary
    storage: local-disk
    retention: 30d
    sampling: none
    resources:
      cpu: 2 cores
      memory: 4GB
      disk: 100GB

  # Tier 2: Medium (<1000 services)
  medium:
    deployment: clustered
    storage: s3-backed
    retention:
      hot: 7d (local SSD)
      warm: 30d (S3)
    sampling:
      traces: 10%
      logs: drop-debug
    resources:
      nodes: 3
      cpu: 8 cores each
      memory: 32GB each
      disk: 500GB SSD each

  # Tier 3: Large (1000+ services)
  large:
    deployment: distributed
    storage: tiered
    retention:
      hot: 1d (local NVMe)
      warm: 7d (S3 Standard)
      cold: 90d (S3 Glacier)
    sampling:
      traces: 1% (keep errors/slow)
      logs: drop-debug, sample-info
      metrics: downsample after 7d
    resources:
      ingest_nodes: 10
      query_nodes: 5
      storage_nodes: 20

# Automatic scaling signals
autoscaling:
  enabled: true
  signals:
    - metric: ingestion_lag_seconds
      scale_up_threshold: 30
      scale_down_threshold: 5

    - metric: query_latency_p99
      scale_up_threshold: 5s
      scale_down_threshold: 1s

    - metric: storage_used_percent
      scale_up_threshold: 80
      # No scale down - storage doesn't shrink
```

**Scaling dashboard:**
```
┌─────────────────────────────────────────────────────────────┐
│ Capacity Planning                                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Current Load:                                                │
│ ├── Events/sec: 234,567 (capacity: 500,000)                 │
│ ├── Active series: 847,234 (capacity: 2,000,000)            │
│ ├── Storage: 2.3TB / 5TB (46%)                              │
│ └── Headroom: 53%                                           │
│                                                              │
│ Growth Trends (30 days):                                     │
│ ├── Events: +12%/week                                       │
│ ├── Series: +8%/week                                        │
│ └── Storage: +15%/week                                      │
│                                                              │
│ Projections:                                                 │
│ ├── Storage full in: 47 days                                │
│ ├── Cardinality limit in: 89 days                           │
│ └── Ingestion limit in: 120 days                            │
│                                                              │
│ Recommendations:                                             │
│ ├── Add storage node in 30 days                             │
│ ├── Enable trace sampling at 10%: saves 40%                 │
│ └── Drop debug logs: saves 25%                              │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P2** initially (single binary scales far). **P0** when customers grow.

---

### Summary: Priority Ranking

> **Note:** This section analyzes pain points by urgency. For the actual build order, see [Summary: Cost vs Impact Matrix](#summary-cost-vs-impact-matrix) which is the authoritative priority list.

#### P0 - Must Have (Build First)

| Pain Point | Why Critical | Effort |
|------------|--------------|--------|
| **Data silos / correlation** | This IS our core value prop | Medium |
| **Cost unpredictability** | Our #2 killer feature | Medium |
| **Alert fatigue** | Universal pain, high visibility | Medium |
| **Incident response** | Directly reduces MTTR | Medium |
| **Microservices dependencies** | Can't debug without this | Medium |

#### P1 - High Priority (Build Next)

| Pain Point | Why Important | Effort |
|------------|---------------|--------|
| **Cardinality management** | How bills explode | Medium |
| **Kubernetes complexity** | Everyone runs K8s | High |
| **Developer experience** | Drives adoption | Medium |
| **Business context** | Connects tech to revenue | Medium |
| **Compliance/audit** | Required for enterprise | Medium |
| **Multi-cloud visibility** | Modern reality | High |
| **Ephemeral infra** | Serverless/K8s is norm | Medium |
| **Security blind spots** | Growing trend | Medium |
| **Self-monitoring** | Embarrassing if broken | Low |
| **LLM integration** | Table stakes by 2025 | Low |

#### P2 - Important (Build When Ready)

| Pain Point | Why | Effort |
|------------|-----|--------|
| **On-call burnout** | Integrate, don't build | Low |
| **Legacy visibility** | Niche but valuable | Medium |
| **Scaling** | Only at growth stage | High |
| **IDE integration** | Nice DX differentiator | Medium |
| **CI/CD visibility** | Correlation value | Low |
| **GitOps workflow** | Platform teams want | Medium |
| **Terraform provider** | IaC expectation | Medium |
| **Wasm plugins** | Extensibility win | High |

#### Future / Skip

| Item | Recommendation |
|------|----------------|
| **RUM** | Integrate with Sentry/LogRocket |
| **Mobile SDK** | Skip - use OTLP |
| **Browser synthetics** | Multi-step API first |
| **Custom ML** | Use LLM APIs instead |
| **Full SIEM** | Too broad - focus on detection |
| **Carbon footprint** | Future nice-to-have |

### The Bottom Line

**Where we're strong (double down):**
- Zero-config eBPF tracing (unique, hard to copy)
- Cost intelligence (unique positioning, no one else does this)
- Single binary simplicity (architectural decision, can't be changed)
- Service catalog (platform engineering aligned)
- Data correlation (metrics + logs + traces in one place)

**Where we need work (prioritize these):**
- Alert intelligence (grouping, context, noise reduction)
- Incident experience (auto-enrichment, timelines, runbooks)
- Dependency tracking (root cause across services)
- Cardinality control (analysis, limits, cost attribution)
- Kubernetes-native views (pods, nodes, namespaces)
- Business context (revenue impact, customer impact)
- Basic LLM integration (use APIs, not custom ML)
- Wasm extensibility (custom logic without forking)

**Where we should integrate (don't build):**
- RUM/Mobile → Sentry, LogRocket
- On-call → PagerDuty, OpsGenie
- Full SIEM → Splunk, Elastic Security
- Session replay → LogRocket, FullStory

**Our unfair advantages:**
1. **eBPF** - Kernel-level visibility no one else has in single binary
2. **Cost transparency** - We call out competitor pricing (they can't do this)
3. **Simplicity** - One binary vs 5+ components
4. **Data sovereignty** - Self-hosted, your data stays yours
5. **Wasm extensibility** - Your rules, your logic, safely sandboxed

---

## Stealing Killer Features from Competitors

Analyzing the unique strengths of major observability platforms and how to adopt them.

### Splunk: The Features That Made Them a $28B Company

Splunk's dominance came from several genuinely innovative features:

#### 1. SPL (Search Processing Language) ⭐ STEAL THIS

**What it is:** A pipeline-based query language that's incredibly expressive.

```spl
# Find slow requests, extract fields, calculate stats, alert
index=web sourcetype=access_log
| rex field=_raw "duration=(?<duration>\d+)"
| where duration > 2000
| stats count, avg(duration), max(duration) by endpoint, user
| where count > 10
| sort -avg(duration)
```

**Why it's killer:**
- Pipe-based (like Unix) - intuitive for engineers
- Field extraction on the fly (`rex`)
- Statistical commands built-in
- Transformations chain naturally
- Users can build complex analysis without programming

**What we should build:**

```
DogQuery Language (DQL):

# Same query in DQL
logs
| where service == "api" and latency_ms > 2000
| extract pattern="user=(?P<user>\w+)"
| group by endpoint, user
| stats count(), avg(latency_ms), max(latency_ms)
| having count() > 10
| sort avg_latency_ms desc

# Cross-signal queries (Splunk can't do this easily)
logs | where error == true
| join traces on trace_id
| join metrics on (service, timestamp ± 1m)
| correlate  # Find what's common across all three
```

**Implementation:**

```go
// DQL Parser
type DQLQuery struct {
    Source      DataSource    // logs, traces, metrics, events
    Pipes       []PipeStage   // Sequential transformations
}

type PipeStage interface {
    Execute(input Stream) Stream
}

// Pipe stages
type WhereStage struct { Condition Expression }
type ExtractStage struct { Pattern string; Fields []string }
type GroupByStage struct { Fields []string }
type StatsStage struct { Aggregations []Aggregation }
type JoinStage struct { Other DataSource; On JoinCondition }
type SortStage struct { Field string; Desc bool }
type LimitStage struct { N int }

// Example query execution
func (q *DQLQuery) Execute(ctx context.Context) (Results, error) {
    stream := q.Source.Stream(ctx)
    for _, pipe := range q.Pipes {
        stream = pipe.Execute(stream)
    }
    return stream.Collect()
}
```

Priority: **P1**. Query language is how power users live in the product.

---

#### 2. Knowledge Objects ⭐ STEAL THIS

**What it is:** Saved searches, reports, alerts, and dashboards that are first-class, reusable objects.

```
Knowledge Object Hierarchy:

Organization
├── Apps (logical groupings)
│   ├── Saved Searches
│   │   ├── "Slow API Requests" (used by 3 dashboards, 2 alerts)
│   │   ├── "Error Patterns" (used by 1 report)
│   │   └── "User Activity" (used by 2 dashboards)
│   ├── Reports (scheduled saved searches)
│   ├── Alerts (saved search + trigger + action)
│   ├── Dashboards (compositions of saved searches)
│   └── Lookups (enrichment data)
└── Permissions (who can use what)
```

**Why it's killer:**
- One search powers multiple dashboards
- Change the search, all dashboards update
- Alerts are just searches with triggers
- Reports are just searches with schedules
- Everything is composable

**What we should build:**

```yaml
# dogwatch knowledge objects
knowledge_objects:

  saved_queries:
    - id: slow-api-requests
      name: "Slow API Requests"
      query: |
        traces
        | where service == "api" and duration_ms > 2000
        | stats count(), p50(duration_ms), p99(duration_ms) by endpoint
      cache_ttl: 60s
      used_by:
        - dashboard: api-overview
        - alert: slow-endpoint-alert
        - report: weekly-performance

  alerts:
    - id: slow-endpoint-alert
      based_on: slow-api-requests  # Reference saved query
      trigger:
        condition: p99_duration_ms > 5000
        for: 5m
      actions:
        - slack: #api-team
        - pagerduty: api-oncall

  reports:
    - id: weekly-performance
      based_on: slow-api-requests
      schedule: "0 9 * * 1"  # Monday 9am
      format: pdf
      recipients: [engineering-leads@]

  dashboards:
    - id: api-overview
      panels:
        - query_ref: slow-api-requests
          visualization: table
        - query_ref: slow-api-requests
          visualization: timeseries
          field: p99_duration_ms
```

Priority: **P2**. Enables power users and reduces duplication.

---

#### 3. LogReduce / Pattern Detection ⭐ STEAL THIS

**What it is:** Automatically clusters log messages to find patterns.

```
Before LogReduce (1M log lines):
2024-01-15 14:32:01 ERROR Failed to connect to database: timeout
2024-01-15 14:32:01 ERROR Failed to connect to database: timeout
2024-01-15 14:32:02 ERROR Failed to connect to database: timeout
... (repeated 50,000 times)
2024-01-15 14:32:01 INFO User john logged in
2024-01-15 14:32:02 INFO User jane logged in
... (repeated 200,000 times)
2024-01-15 14:32:03 WARN Rate limit exceeded for IP 10.0.1.50
... (repeated 100 times)

After LogReduce (3 patterns):
┌─────────────────────────────────────────────────────────────┐
│ Pattern                                    │ Count │ Trend │
├────────────────────────────────────────────┼───────┼───────┤
│ ERROR Failed to connect to database: *     │ 50K   │ ↑ NEW │
│ INFO User * logged in                      │ 200K  │ →     │
│ WARN Rate limit exceeded for IP *          │ 100   │ →     │
└─────────────────────────────────────────────────────────────┘

Click pattern → See all matching logs
```

**Why it's killer:**
- Turns 1M lines into 3 patterns
- Immediately surfaces anomalies ("↑ NEW")
- No manual regex writing
- Works on ANY log format

**What we should build:**

```go
// Log pattern detection
type LogPattern struct {
    Template    string            // "ERROR Failed to connect to database: *"
    Signature   uint64            // Hash for fast matching
    Count       int64
    FirstSeen   time.Time
    LastSeen    time.Time
    Trend       Trend             // NEW, INCREASING, STABLE, DECREASING
    Examples    []string          // Sample matching logs
    Variables   []VariableSlot    // Extracted wildcards
}

// Pattern detection algorithm
func DetectPatterns(logs []LogLine) []LogPattern {
    // 1. Tokenize each log line
    // 2. Replace variable parts with wildcards:
    //    - Numbers → *
    //    - UUIDs → *
    //    - IPs → *
    //    - Timestamps → *
    //    - Quoted strings → *
    // 3. Hash the template
    // 4. Group by hash
    // 5. Calculate trends vs previous period

    patterns := make(map[uint64]*LogPattern)

    for _, log := range logs {
        template := tokenizeAndGeneralize(log.Message)
        sig := hash(template)

        if p, exists := patterns[sig]; exists {
            p.Count++
            p.LastSeen = log.Timestamp
        } else {
            patterns[sig] = &LogPattern{
                Template:  template,
                Signature: sig,
                Count:     1,
                FirstSeen: log.Timestamp,
                LastSeen:  log.Timestamp,
            }
        }
    }

    return rankByRelevance(patterns)
}
```

**UI for pattern detection:**

```
┌─────────────────────────────────────────────────────────────┐
│ Log Patterns (last 1 hour)                    [Auto-refresh]│
├─────────────────────────────────────────────────────────────┤
│ 🔴 NEW PATTERN (first seen 5 min ago)                       │
│ ├── "ERROR Connection refused to redis:6379"               │
│ ├── Count: 12,847                                          │
│ ├── Services: api, worker, scheduler                       │
│ └── [View logs] [Create alert] [Investigate]               │
│                                                              │
│ ⚠️ INCREASING (+340% vs last hour)                          │
│ ├── "WARN Request timeout after * ms"                      │
│ ├── Count: 8,234                                           │
│ └── [View logs] [Correlate]                                │
│                                                              │
│ ✅ NORMAL                                                   │
│ ├── "INFO User * logged in from *"                         │
│ ├── Count: 234,567                                         │
│ └── [View logs]                                            │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P1**. Transforms log analysis from "needle in haystack" to "here are the 5 things you should care about."

---

#### 4. SOAR (Security Orchestration, Automation, Response) ⭐ PARTIAL STEAL

**What it is:** Automated playbooks for security response.

```
Playbook: "Brute Force Response"

Trigger: > 10 failed logins from same IP in 5 min

Actions:
1. Block IP at firewall (automated)
2. Create ticket in ServiceNow (automated)
3. Enrich with threat intel (automated)
4. Notify security team (automated)
5. Wait for analyst decision
6. If malicious: add to permanent blocklist
7. Generate incident report
```

**Why it's killer:**
- Reduces response time from hours to seconds
- Consistent response (no human error)
- Audit trail of all actions
- Integrates everything

**What we should steal (simplified):**

```yaml
# Automated response playbooks
playbooks:
  - name: high-error-rate-response
    trigger:
      alert: error-rate-high
      severity: critical

    steps:
      - name: gather-context
        parallel:
          - action: query
            query: "traces | where error == true | limit 100"
            save_as: error_traces

          - action: query
            query: "events | where type == 'deploy' | last 1h"
            save_as: recent_deploys

          - action: query
            query: "logs | where level == 'error' | pattern_detect"
            save_as: error_patterns

      - name: check-for-deploy-correlation
        condition: recent_deploys.count > 0
        action: enrich
        add:
          likely_cause: "deploy"
          deploy_version: "{{recent_deploys[0].version}}"
          rollback_available: true

      - name: auto-rollback-if-enabled
        condition: config.auto_rollback_enabled AND likely_cause == "deploy"
        action: webhook
        url: "{{deploy_system}}/rollback"
        body:
          version: "{{recent_deploys[0].previous_version}}"

      - name: notify
        action: notify
        channels: [slack, pagerduty]
        message: |
          🔴 High Error Rate Detected

          Error patterns:
          {{error_patterns | format}}

          {{#if likely_cause == "deploy"}}
          Likely caused by deploy {{deploy_version}}
          {{#if auto_rollback_triggered}}
          ✅ Auto-rollback initiated
          {{else}}
          [Rollback] [Ignore]
          {{/if}}
          {{/if}}
```

Priority: **P2**. Powerful but complex. Start with simple auto-enrichment, add full playbooks later.

---

### VMware Wavefront (Aria): High-Cardinality Masters

Wavefront handles 100M+ time series. Here's how:

#### 1. Histograms as First-Class Citizens ⭐ STEAL THIS

**What it is:** Store full distributions, not just percentiles.

```
Traditional (Prometheus style):
  http_request_duration_seconds_bucket{le="0.1"} 24054
  http_request_duration_seconds_bucket{le="0.25"} 33444
  http_request_duration_seconds_bucket{le="0.5"} 100392
  http_request_duration_seconds_bucket{le="1"} 129389
  http_request_duration_seconds_bucket{le="+Inf"} 133988

  Problem: Fixed buckets chosen at instrumentation time
  Can't compute p99.9 if you didn't create that bucket

Wavefront histograms:
  http_request_duration stores ACTUAL distribution
  Query any percentile at query time: p50, p99, p99.9, p99.99
  Merge histograms across time/dimensions without losing accuracy
```

**Why it's killer:**
- No bucket pre-configuration
- Accurate tail latencies (p99.9+)
- Aggregate across dimensions without losing precision
- Storage efficient (t-digest or HDR histogram)

**What we should build:**

```go
// HDR Histogram storage
type HistogramPoint struct {
    Timestamp  time.Time
    Histogram  *hdrhistogram.Histogram  // Or t-digest
}

// Store full distribution, query any percentile
func (h *HistogramStore) RecordValue(metric string, labels Labels, value float64) {
    key := metricKey(metric, labels)
    hist := h.getOrCreate(key)
    hist.RecordValue(int64(value * 1000))  // Store as microseconds
}

func (h *HistogramStore) Query(metric string, labels Labels, percentile float64) float64 {
    key := metricKey(metric, labels)
    hist := h.get(key)
    return float64(hist.ValueAtPercentile(percentile)) / 1000
}

// Merge histograms across time windows
func (h *HistogramStore) Aggregate(metrics []string, window TimeWindow) *hdrhistogram.Histogram {
    result := hdrhistogram.New(1, 3600000000, 3)  // 1µs to 1hr, 3 sig figs
    for _, m := range metrics {
        points := h.getPoints(m, window)
        for _, p := range points {
            result.Merge(p.Histogram)
        }
    }
    return result
}
```

**Query examples:**

```
# Get p99.9 (not possible with fixed buckets)
percentile(99.9, http.duration{service="api"})

# Compare percentiles
percentile(99, http.duration) vs percentile(50, http.duration)

# Histogram over time
percentile(99, http.duration{service="api"}) [1h:1m]
```

Priority: **P1**. Tail latency accuracy is critical for SLOs.

---

#### 2. Delta Counters ⭐ STEAL THIS

**What it is:** Counters that work correctly in distributed/serverless environments.

```
Problem with regular counters:
  Lambda invocation 1: counter = 5
  Lambda invocation 2: counter = 3
  Lambda invocation 3: counter = 7

  How do you aggregate? They're independent!
  Regular counter assumes continuous increment.

Delta counters:
  Lambda 1 sends: Δ+5
  Lambda 2 sends: Δ+3
  Lambda 3 sends: Δ+7

  Server aggregates: total = 15
  Works for ANY ephemeral compute.
```

**Why it's killer:**
- Works with serverless (Lambda, Cloud Functions)
- Works with auto-scaling (pods come and go)
- Works with distributed systems (no coordination needed)
- No "counter reset" detection heuristics

**What we should build:**

```go
// Delta counter aggregation
type DeltaCounter struct {
    Name   string
    Labels Labels
}

// Ingest handles delta values
func (s *MetricsStore) IngestDelta(metric string, labels Labels, delta float64) {
    key := metricKey(metric, labels)
    bucket := s.getBucket(time.Now().Truncate(time.Minute))

    // Atomic add - no race conditions
    bucket.AddDelta(key, delta)
}

// Query returns aggregated value
func (s *MetricsStore) QueryDeltaCounter(metric string, labels Labels, window TimeWindow) float64 {
    var total float64
    for _, bucket := range s.getBuckets(window) {
        total += bucket.GetDelta(metricKey(metric, labels))
    }
    return total
}

// Rate calculation
func (s *MetricsStore) DeltaRate(metric string, labels Labels, window TimeWindow) float64 {
    total := s.QueryDeltaCounter(metric, labels, window)
    return total / window.Duration().Seconds()
}
```

**API:**

```
# Send delta
POST /api/v1/metrics/delta
{
  "metric": "function.invocations",
  "labels": {"function": "process-order"},
  "delta": 1
}

# Query (works correctly with ephemeral compute)
GET /api/v1/query?q=rate(function.invocations[5m])
```

Priority: **P1**. Essential for serverless/K8s environments.

---

#### 3. Derived Metrics ⭐ STEAL THIS

**What it is:** Define new metrics as computations of existing metrics.

```yaml
# Derived metric definitions
derived_metrics:
  # Error rate from raw counters
  - name: error_rate
    query: |
      rate(http_errors) / rate(http_requests) * 100
    interval: 1m

  # Cost per request
  - name: cost_per_request
    query: |
      sum(infrastructure_cost) / sum(http_requests)
    interval: 1h

  # Apdex score
  - name: apdex_score
    query: |
      (
        count(http_duration < 500) +
        count(http_duration >= 500 AND http_duration < 2000) * 0.5
      ) / count(http_duration)
    interval: 1m

  # Business metric: revenue per minute
  - name: revenue_per_minute
    query: |
      sum(order_value{status="completed"})
    interval: 1m
```

**Why it's killer:**
- Complex metrics without client-side computation
- Consistent definitions (everyone uses same formula)
- Alertable (alert on derived metrics)
- Historical (backfill when definition changes)

**What we should build:**

```go
// Derived metric engine
type DerivedMetric struct {
    Name       string
    Query      string        // DQL expression
    Interval   time.Duration // Computation frequency
    Labels     []string      // Preserved labels
    Enabled    bool
}

type DerivedMetricEngine struct {
    definitions []DerivedMetric
    store       MetricsStore
    queryEngine QueryEngine
}

func (e *DerivedMetricEngine) Run(ctx context.Context) {
    for _, dm := range e.definitions {
        go e.runDerivation(ctx, dm)
    }
}

func (e *DerivedMetricEngine) runDerivation(ctx context.Context, dm DerivedMetric) {
    ticker := time.NewTicker(dm.Interval)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Execute query
            result, err := e.queryEngine.Execute(dm.Query)
            if err != nil {
                log.Error("derived metric failed", "name", dm.Name, "err", err)
                continue
            }

            // Store as new metric
            for _, point := range result.Points {
                e.store.Record(dm.Name, point.Labels, point.Value)
            }
        }
    }
}
```

Priority: **P2**. Power feature for advanced users.

---

### Elastic/ELK: Search & Visualization Kings

#### 1. Full-Text Search with Relevance ⭐ STEAL THIS

**What it is:** Not just "contains" but "most relevant matches."

```
Query: "payment failed timeout"

Elasticsearch returns (ranked by relevance):
1. "Payment processing failed due to gateway timeout" (score: 9.2)
2. "Failed to complete payment: connection timeout" (score: 8.7)
3. "Payment service timeout, retry failed" (score: 8.1)
4. "Timeout waiting for payment confirmation" (score: 6.3)
5. "Failed health check (not payment related)" (score: 2.1)

Not just "contains all words" but "most relevant context"
```

**Why it's killer:**
- Natural language queries
- Finds what you mean, not just what you typed
- Handles typos, synonyms, word order
- Ranks results by usefulness

**What we should build:**

```go
// Full-text search with BM25 ranking
type LogSearchEngine struct {
    index *bluge.Index  // Or tantivy-go
}

func (e *LogSearchEngine) Search(query string, opts SearchOptions) ([]LogResult, error) {
    // Parse natural language query
    parsed := parseQuery(query)

    // Build search request
    req := bluge.NewSearchRequest(
        bluge.NewBooleanQuery().
            Should(bluge.NewMatchQuery(parsed.Terms).SetField("message")).
            Should(bluge.NewMatchPhraseQuery(query).SetField("message").SetBoost(2.0)).
            Filter(buildFilters(opts)),
    ).
        WithStandardAggregations().
        Size(opts.Limit).
        SortBy(bluge.SortBy{Field: "_score", Descending: true})

    // Execute and rank
    results, err := e.index.Search(req)
    if err != nil {
        return nil, err
    }

    return mapToLogResults(results), nil
}

// Query suggestions
func (e *LogSearchEngine) Suggest(partial string) []string {
    // Return common completions
    // "pay" → ["payment", "payment failed", "payment timeout"]
}
```

**UI experience:**

```
┌─────────────────────────────────────────────────────────────┐
│ 🔍 payment failed timeout                           [Search]│
├─────────────────────────────────────────────────────────────┤
│ Did you mean: "payment failure timeout" (23% more results)  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ 📄 1. payment-service | 14:32:01                            │
│    Payment processing failed due to gateway timeout         │
│    after 30000ms. Transaction ID: txn_abc123                │
│    [View context] [View trace]                              │
│                                                              │
│ 📄 2. checkout-service | 14:32:00                           │
│    Failed to complete payment: connection timeout to        │
│    payment gateway. Retrying...                             │
│    [View context] [View trace]                              │
│                                                              │
│ 💡 Related: Show traces with payment errors (47 found)      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P1**. This is how people actually want to search logs.

---

#### 2. Canvas (Presentation Dashboards) ⭐ STEAL THIS

**What it is:** Pixel-perfect dashboards for executives/TV displays.

```
Regular dashboard: Grids of charts, data-dense, for operators

Canvas dashboard:
- Custom backgrounds/branding
- Positioned elements (not grid)
- Large KPI numbers
- Status indicators
- Designed for TV displays / exec presentations
- Auto-refresh, auto-rotate
```

**Why it's killer:**
- NOC/SOC wall displays
- Executive dashboards (simple, pretty)
- Customer-facing status pages
- TV mode for office displays

**What we should build:**

```yaml
# Canvas dashboard
canvas:
  name: "NOC Overview"
  size: { width: 1920, height: 1080 }
  background:
    color: "#1a1a2e"
    # Or image: "/assets/company-bg.png"

  elements:
    - type: metric
      position: { x: 100, y: 100 }
      size: { width: 300, height: 200 }
      query: "sum(http_requests)"
      format: "0.0a"  # 1.2M
      label: "Requests/min"
      color:
        good: "#00ff00"
        warn: "#ffff00"
        bad: "#ff0000"
      thresholds:
        warn: 10000
        bad: 50000

    - type: status
      position: { x: 500, y: 100 }
      size: { width: 400, height: 50 }
      query: "min(up{job='api'})"
      states:
        1: { label: "API Healthy", color: "green" }
        0: { label: "API DOWN", color: "red", blink: true }

    - type: chart
      position: { x: 100, y: 350 }
      size: { width: 800, height: 300 }
      query: "rate(http_requests[5m])"
      style: "area"
      hideAxis: true
      hideLabels: true  # Just the shape

    - type: text
      position: { x: 1600, y: 50 }
      content: "{{now | format 'HH:mm'}}"
      size: 48
      color: "#ffffff"

  rotation:
    enabled: true
    interval: 30s
    dashboards: [canvas-1, canvas-2, canvas-3]
```

Priority: **P3**. Nice for enterprise sales demos, not core.

---

### New Relic: Entity-Centric Observability

#### 1. Entity Synthesis ⭐ STEAL THIS

**What it is:** Automatic discovery and relationship mapping of ALL entities.

```
Entity Types:
├── Services (from APM)
├── Hosts (from infrastructure)
├── Containers (from K8s)
├── Databases (from connections)
├── Load Balancers (from network)
├── Queues (from message traces)
├── External APIs (from outbound calls)
└── Custom (from your definitions)

Each entity has:
├── Golden signals (throughput, errors, latency, saturation)
├── Relationships (calls, runs-on, contains)
├── Ownership (team, repo, oncall)
└── Alerts (scoped to this entity)
```

**Why it's killer:**
- Everything is an entity with consistent metadata
- Navigate by relationship, not just by query
- "Show me everything related to this service"
- Automatic SLOs per entity

**What we should build:**

```go
// Entity model
type Entity struct {
    Type         EntityType    // SERVICE, HOST, CONTAINER, DATABASE, etc.
    ID           string        // Unique identifier
    Name         string        // Human-readable name
    Domain       string        // Logical grouping

    // Golden signals (auto-calculated)
    Signals      GoldenSignals

    // Relationships
    Relationships []Relationship

    // Metadata
    Tags         map[string]string
    Team         string
    Repository   string
    OnCall       string

    // Current status
    Health       HealthStatus
    Alerts       []Alert
}

type GoldenSignals struct {
    Throughput   float64  // Requests per second
    ErrorRate    float64  // Percentage
    Latency      Percentiles
    Saturation   float64  // Resource utilization
}

type Relationship struct {
    Type         RelationType  // CALLS, RUNS_ON, CONTAINS, DEPENDS_ON
    Target       string        // Target entity ID
    Metadata     map[string]interface{}
}

// Entity synthesizer
type EntitySynthesizer struct {
    // Discover entities from various signals
}

func (s *EntitySynthesizer) Synthesize(ctx context.Context) []Entity {
    entities := make(map[string]*Entity)

    // From traces: services, databases, external APIs
    for _, span := range s.traceStore.GetSpans(ctx, last1Hour) {
        svc := s.getOrCreateService(entities, span.ServiceName)
        svc.UpdateSignals(span)

        if span.SpanKind == CLIENT {
            // Creates relationship to target
            target := s.inferTarget(span)
            svc.AddRelationship(CALLS, target)
        }
    }

    // From metrics: hosts, containers
    for _, metric := range s.metricStore.GetHostMetrics(ctx) {
        host := s.getOrCreateHost(entities, metric.Host)
        host.UpdateSignals(metric)
    }

    // From K8s: pods, deployments, nodes
    for _, pod := range s.k8sStore.GetPods(ctx) {
        container := s.getOrCreateContainer(entities, pod)
        container.AddRelationship(RUNS_ON, pod.NodeName)
    }

    return toSlice(entities)
}
```

**Entity explorer UI:**

```
┌─────────────────────────────────────────────────────────────┐
│ Entity: payment-service                                      │
├─────────────────────────────────────────────────────────────┤
│ Type: SERVICE     Team: payments     OnCall: @alice         │
├─────────────────────────────────────────────────────────────┤
│ Golden Signals (last 5m):                                    │
│ ├── Throughput: 1,234 req/s                                 │
│ ├── Error Rate: 0.3%                                        │
│ ├── Latency: p50=45ms, p99=234ms                            │
│ └── Saturation: 67% CPU                                     │
├─────────────────────────────────────────────────────────────┤
│ Relationships:                                               │
│                                                              │
│ ← Called by:           → Calls:              Runs on:       │
│ ├── checkout-svc       ├── postgres-main     ├── node-1     │
│ ├── api-gateway        ├── redis-cache       ├── node-2     │
│ └── order-svc          ├── stripe-api        └── node-3     │
│                        └── kafka                             │
├─────────────────────────────────────────────────────────────┤
│ Active Alerts: 1                                             │
│ └── ⚠️ High latency (p99 > 200ms for 5m)                     │
├─────────────────────────────────────────────────────────────┤
│ [View Traces] [View Logs] [View Metrics] [View Dependencies]│
└─────────────────────────────────────────────────────────────┘
```

Priority: **P1**. This is how users want to navigate their systems.

---

#### 2. Lookout (Automatic Anomaly Overview) ⭐ STEAL THIS

**What it is:** At-a-glance view of everything that's abnormal.

```
┌─────────────────────────────────────────────────────────────┐
│ Lookout - What's Different Right Now                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ 🔴 Critical Deviations:                                      │
│ ├── payment-service: Error rate 5.2% (normally 0.1%)        │
│ ├── checkout-db: Query time 890ms (normally 45ms)           │
│ └── redis-main: Memory 94% (normally 60%)                   │
│                                                              │
│ ⚠️ Warnings:                                                 │
│ ├── api-gateway: Throughput -34% vs same time yesterday     │
│ ├── worker-pool: Queue depth 10x normal                     │
│ └── cdn: Cache hit rate 67% (normally 89%)                  │
│                                                              │
│ 📈 Unusual Growth:                                           │
│ ├── new-feature-service: +450% traffic (expected: launch)   │
│ └── logging: +230% volume (investigate)                     │
│                                                              │
│ Circle size = impact, Color = severity                      │
│ Click any item to investigate                               │
│                                                              │
│ ○ payment  ○ checkout-db  ○ redis                          │
│    ○ api-gateway  ○ worker  ○ cdn                          │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Why it's killer:**
- No query required
- Just open the page and see what's wrong
- Compares to baseline automatically
- Prioritized by impact

**What we should build:**

```go
// Anomaly detection across all entities
type LookoutEngine struct {
    entities    EntityStore
    baselines   BaselineStore
    anomalies   []Anomaly
}

type Anomaly struct {
    Entity      Entity
    Metric      string
    Current     float64
    Baseline    float64
    Deviation   float64   // How many standard deviations
    Impact      float64   // Estimated user/business impact
    Severity    Severity
    FirstSeen   time.Time
}

func (l *LookoutEngine) Scan(ctx context.Context) []Anomaly {
    var anomalies []Anomaly

    for _, entity := range l.entities.All() {
        for _, metric := range entity.GoldenSignals {
            baseline := l.baselines.Get(entity.ID, metric.Name)
            deviation := (metric.Value - baseline.Mean) / baseline.StdDev

            if abs(deviation) > 2 {  // More than 2 standard deviations
                anomalies = append(anomalies, Anomaly{
                    Entity:    entity,
                    Metric:    metric.Name,
                    Current:   metric.Value,
                    Baseline:  baseline.Mean,
                    Deviation: deviation,
                    Impact:    l.estimateImpact(entity, metric),
                    Severity:  l.classifySeverity(deviation),
                })
            }
        }
    }

    // Sort by impact
    sort.Slice(anomalies, func(i, j int) bool {
        return anomalies[i].Impact > anomalies[j].Impact
    })

    return anomalies
}
```

Priority: **P1**. This should be the homepage. "What's wrong right now?"

---

### Honeycomb: Query UX Masters

#### 1. BubbleUp (We Already Know This) ✅ HAVE IT

We've already documented this. Just ensure implementation is solid.

---

#### 2. Query Builder UX ⭐ STEAL THIS

**What it is:** Visual query construction for non-experts.

```
Instead of writing:
  logs | where service="api" and status>=500 | group by endpoint | count()

Build visually:
┌─────────────────────────────────────────────────────────────┐
│ Query Builder                                                │
├─────────────────────────────────────────────────────────────┤
│ WHERE:                                                       │
│ [service    ▼] [=    ▼] [api        ▼] [AND ▼]             │
│ [status     ▼] [>=   ▼] [500          ] [+Add]              │
│                                                              │
│ GROUP BY:                                                    │
│ [endpoint   ▼] [+Add]                                       │
│                                                              │
│ VISUALIZE:                                                   │
│ [COUNT      ▼] as [Request Count]                           │
│ [P99        ▼] of [duration] as [Latency]                   │
│                                                              │
│ [Run Query]                         Generated: logs | whe... │
└─────────────────────────────────────────────────────────────┘
```

**Why it's killer:**
- Discoverability (dropdown shows available fields)
- No syntax errors
- Learn the query language by seeing generated output
- Accessible to non-engineers

**What we should build:**

```typescript
// React query builder component
interface QueryBuilder {
  filters: Filter[];
  groupBy: string[];
  aggregations: Aggregation[];
  visualization: VisualizationType;
}

interface Filter {
  field: string;      // Autocomplete from schema
  operator: Operator; // =, !=, >, <, contains, regex
  value: string;      // Autocomplete from actual values
  combinator: 'AND' | 'OR';
}

// Real-time query generation
function generateDQL(builder: QueryBuilder): string {
  let query = builder.source;

  if (builder.filters.length > 0) {
    query += ' | where ' + builder.filters.map(f =>
      `${f.field} ${f.operator} "${f.value}"`
    ).join(` ${f.combinator} `);
  }

  if (builder.groupBy.length > 0) {
    query += ' | group by ' + builder.groupBy.join(', ');
  }

  if (builder.aggregations.length > 0) {
    query += ' | ' + builder.aggregations.map(a =>
      `${a.function}(${a.field}) as ${a.alias}`
    ).join(', ');
  }

  return query;
}

// Field value autocomplete
async function getFieldValues(field: string, prefix: string): Promise<string[]> {
  // Query for common values of this field matching prefix
  const values = await api.query(`
    logs | stats count() by ${field} | sort -count | limit 20
  `);
  return values.filter(v => v.startsWith(prefix));
}
```

Priority: **P1**. Critical for adoption by non-power-users.

---

### Sumo Logic: Log Intelligence

#### 1. LogCompare ⭐ STEAL THIS

**What it is:** Compare logs between two time periods to find what changed.

```
Time Period A: Today 14:00-15:00 (when things broke)
Time Period B: Yesterday 14:00-15:00 (when things worked)

LogCompare Result:
┌─────────────────────────────────────────────────────────────┐
│ New in Period A (didn't exist in B):                        │
├─────────────────────────────────────────────────────────────┤
│ 🆕 "Connection refused to redis:6379" (12,847 occurrences) │
│ 🆕 "Timeout waiting for lock" (3,421 occurrences)          │
│ 🆕 "Circuit breaker OPEN" (891 occurrences)                │
├─────────────────────────────────────────────────────────────┤
│ Gone from Period B (existed before, not now):               │
├─────────────────────────────────────────────────────────────┤
│ ❌ "Connected to redis:6379" (was 50,000/hr)               │
├─────────────────────────────────────────────────────────────┤
│ Changed Frequency:                                          │
├─────────────────────────────────────────────────────────────┤
│ ↑ "Request timeout" +2,340% (100 → 2,440)                  │
│ ↓ "Request completed" -89% (50,000 → 5,500)                │
└─────────────────────────────────────────────────────────────┘
```

**Why it's killer:**
- Instant root cause identification
- "What's different?" is the debugging question
- No hypothesis needed - just show the diff

**What we should build:**

```go
// Log comparison engine
type LogCompare struct {
    store LogStore
}

type CompareResult struct {
    NewPatterns      []PatternDiff  // Only in period A
    GonePatterns     []PatternDiff  // Only in period B
    IncreasedPatterns []PatternDiff // Higher count in A
    DecreasedPatterns []PatternDiff // Lower count in A
}

func (c *LogCompare) Compare(periodA, periodB TimeWindow) *CompareResult {
    patternsA := c.store.GetPatterns(periodA)
    patternsB := c.store.GetPatterns(periodB)

    result := &CompareResult{}

    // Find new patterns
    for sig, pattern := range patternsA {
        if _, exists := patternsB[sig]; !exists {
            result.NewPatterns = append(result.NewPatterns, PatternDiff{
                Pattern: pattern,
                CountA:  pattern.Count,
                CountB:  0,
                Change:  "NEW",
            })
        }
    }

    // Find gone patterns
    for sig, pattern := range patternsB {
        if _, exists := patternsA[sig]; !exists {
            result.GonePatterns = append(result.GonePatterns, PatternDiff{
                Pattern: pattern,
                CountA:  0,
                CountB:  pattern.Count,
                Change:  "GONE",
            })
        }
    }

    // Find frequency changes
    for sig, patternA := range patternsA {
        if patternB, exists := patternsB[sig]; exists {
            changePercent := (patternA.Count - patternB.Count) / patternB.Count * 100
            if abs(changePercent) > 50 {  // More than 50% change
                diff := PatternDiff{
                    Pattern:       patternA,
                    CountA:        patternA.Count,
                    CountB:        patternB.Count,
                    ChangePercent: changePercent,
                }
                if changePercent > 0 {
                    result.IncreasedPatterns = append(result.IncreasedPatterns, diff)
                } else {
                    result.DecreasedPatterns = append(result.DecreasedPatterns, diff)
                }
            }
        }
    }

    return result
}
```

**UI:**

```
┌─────────────────────────────────────────────────────────────┐
│ LogCompare                                                   │
├─────────────────────────────────────────────────────────────┤
│ Compare: [Today 14:00-15:00 ▼] vs [Yesterday 14:00-15:00 ▼] │
│                                                              │
│ Or: [Current hour] vs [Same hour last week]                 │
│     [After deploy] vs [Before deploy]                       │
│     [Incident window] vs [Normal baseline]                  │
│                                                              │
│ [Compare]                                                    │
└─────────────────────────────────────────────────────────────┘
```

Priority: **P0**. This is how people actually debug. "What changed?"

---

### Cribl: Data Pipeline Control

#### 1. Data Routing & Transformation ⭐ STEAL THIS

**What it is:** Control where data goes and transform it in transit.

```
┌─────────────────────────────────────────────────────────────┐
│                    Data Pipeline                             │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  Sources          Pipelines           Destinations          │
│  ────────         ─────────           ────────────          │
│  ┌────────┐       ┌─────────────┐     ┌─────────┐           │
│  │ eBPF   │──┬───▶│ Sample 10%  │────▶│ dogwatch│           │
│  └────────┘  │    │ (traces)    │     │ storage │           │
│              │    └─────────────┘     └─────────┘           │
│  ┌────────┐  │    ┌─────────────┐     ┌─────────┐           │
│  │ OTLP   │──┼───▶│ Redact PII  │────▶│ S3 cold │           │
│  └────────┘  │    │ (logs)      │     │ storage │           │
│              │    └─────────────┘     └─────────┘           │
│  ┌────────┐  │    ┌─────────────┐     ┌─────────┐           │
│  │ Prom   │──┴───▶│ Drop debug  │────▶│ SIEM    │           │
│  └────────┘       │ (all)       │     │ forward │           │
│                   └─────────────┘     └─────────┘           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Why it's killer:**
- Route different data to different destinations
- Transform in transit (redact, enrich, sample)
- Cost control (drop low-value data)
- Compliance (route PII differently)

**What we should build:**

```yaml
# Data pipeline configuration
pipelines:
  sources:
    - name: ebpf-traces
      type: internal

    - name: otlp-external
      type: otlp
      port: 4317

    - name: prometheus-scrape
      type: prometheus
      targets:
        - "app:9090"

  routes:
    # High-value traces: keep 100%, send to hot storage
    - name: important-traces
      source: ebpf-traces
      filter: |
        error == true OR
        duration_ms > 2000 OR
        service IN ["checkout", "payment"]
      transforms:
        - enrich:
            team: "{{lookup service_team_mapping}}"
      destination: hot-storage

    # Normal traces: sample to 10%
    - name: sampled-traces
      source: ebpf-traces
      filter: NOT (error == true OR duration_ms > 2000)
      transforms:
        - sample:
            rate: 0.1
      destination: hot-storage

    # Logs with PII: redact and route to compliance storage
    - name: pii-logs
      source: otlp-external
      filter: |
        pii_detected == true
      transforms:
        - redact:
            patterns:
              - '\d{3}-\d{2}-\d{4}'  # SSN
              - '\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b'  # Email
        - add_field:
            pii_redacted: true
      destination: [compliance-storage, siem]

    # Debug logs: drop entirely in production
    - name: drop-debug
      source: otlp-external
      filter: level == "debug" AND env == "production"
      destination: null  # /dev/null

    # All metrics: forward to both dogwatch and external
    - name: metrics-fanout
      source: prometheus-scrape
      transforms:
        - rename:
            old: http_requests_total
            new: http.requests
      destination: [hot-storage, datadog-exporter]

  destinations:
    - name: hot-storage
      type: internal

    - name: compliance-storage
      type: s3
      bucket: compliance-logs
      encryption: AES256

    - name: siem
      type: webhook
      url: https://siem.internal/api/events

    - name: datadog-exporter
      type: datadog
      api_key: ${DD_API_KEY}
```

Priority: **P1**. Essential for cost control and compliance.

---

### Summary: Features to Steal

#### P0 - Build These First

| Feature | From | Why Critical |
|---------|------|--------------|
| **LogCompare** | Sumo Logic | "What changed?" is THE debugging question |
| **Pattern Detection** | Splunk | Turns 1M logs into 5 patterns |
| **Lookout/Anomaly Overview** | New Relic | Homepage should show what's wrong |
| **Query Builder UX** | Honeycomb | Enables non-experts |
| **Entity Synthesis** | New Relic | How users navigate systems |

#### P1 - Build These Next

| Feature | From | Why Important |
|---------|------|---------------|
| **DQL (Pipeline Query Language)** | Splunk SPL | Power users live here |
| **Full-Text Search** | Elastic | Natural language log search |
| **Histograms** | Wavefront | Accurate tail latencies |
| **Delta Counters** | Wavefront | Serverless/K8s essential |
| **Data Pipeline Routing** | Cribl | Cost control, compliance |

#### P2 - Build When Ready

| Feature | From | Why |
|---------|------|-----|
| **Knowledge Objects** | Splunk | Reusable, composable queries |
| **Derived Metrics** | Wavefront | Complex metrics without code |
| **SOAR Playbooks** | Splunk | Automated response |
| **Canvas Dashboards** | Elastic | NOC/exec displays |

#### P3 / Skip

| Feature | Why Skip |
|---------|----------|
| **Full SIEM** | Too broad, integrate instead |
| **ML Anomaly Models** | Use LLM APIs or simple stats |
| **600 Integrations** | Focus on auto-discovery |

---

## What Pixie Did (That We're Not Doing)

### 1. Edge Computing Architecture

**Pixie's approach:**
```
Traditional (what we're doing):
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Cluster 1  │────▶│             │     │             │
└─────────────┘     │   Central   │────▶│  Dashboard  │
┌─────────────┐────▶│   Storage   │     │             │
│  Cluster 2  │     │             │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
     Data leaves cluster, stored centrally

Pixie's approach:
┌─────────────────────────────────────┐
│           Cluster 1                  │
│  ┌─────────┐  ┌─────────────────┐   │
│  │ Vizier  │  │ Local Storage   │   │    ┌─────────────┐
│  │ (query) │  │ (data stays     │   │◀──▶│  Dashboard  │
│  │         │  │  in cluster)    │   │    │  (queries   │
│  └─────────┘  └─────────────────┘   │    │   only)     │
└─────────────────────────────────────┘    └─────────────┘
     Data NEVER leaves cluster
     Queries execute at the edge
```

**Why this matters:**
- **Privacy/Compliance** - No sensitive data egress. HIPAA/PCI easier.
- **Cost** - No data transfer costs. No central storage costs.
- **Latency** - Queries execute locally, sub-second response.
- **Scale** - Each cluster is independent. Add clusters = linear scale.

**What dogwatch would need:**
```go
// Edge agent that stores and queries locally
type Vizier struct {
    storage    *LocalStorage      // In-cluster only
    queryEngine *PxLEngine        // Execute queries locally
    connector   *CloudConnector   // Only sends query results, not raw data
}

// Cloud only sees query results, not raw telemetry
type CloudConnector struct {
    // Receives: "Give me p99 latency for service X"
    // Sends back: "247ms"
    // Never sends: Raw traces, logs, spans
}
```

### 2. PxL - Custom Query Language

**Pixie built their own language:**
```python
# PxL script to find slow HTTP requests
import px

# Get HTTP events from eBPF
df = px.DataFrame(table='http_events')

# Filter to slow requests
df = df[df.latency > 100 * px.ms]

# Aggregate by service
df = df.groupby('service').agg(
    count=('latency', px.count),
    p99=('latency', px.quantile, 0.99),
)

# Display
px.display(df)
```

**Why a custom language:**
- More expressive than PromQL for traces/events
- Python-like (familiar to users)
- Can combine metrics, traces, logs in one query
- Enables community scripts

**What we're doing:** Basic query UI, maybe PromQL for metrics. No unified query language.

### 3. Community Scripts

**Pixie's killer feature:**
```
px/scripts/
├── http/
│   ├── http_requests.pxl        # All HTTP traffic
│   ├── slow_requests.pxl        # Requests > 100ms
│   └── http_errors.pxl          # 4xx/5xx responses
├── mysql/
│   ├── mysql_queries.pxl        # All queries
│   ├── slow_queries.pxl         # Slow queries
│   └── mysql_stats.pxl          # Query statistics
├── kubernetes/
│   ├── pod_stats.pxl            # Pod resource usage
│   ├── service_map.pxl          # Service dependencies
│   └── dns_queries.pxl          # DNS resolution
├── security/
│   ├── outbound_conns.pxl       # External connections
│   └── sensitive_data.pxl       # PII detection
└── community/                    # User-contributed
    ├── kafka_lag.pxl
    ├── redis_hotkeys.pxl
    └── grpc_errors.pxl
```

**User experience:**
```
$ px run px/http_requests
$ px run px/mysql_slow_queries
$ px run community/kafka_lag
```

One command → instant analysis. No dashboard building. No query writing.

**What we're missing:** Pre-built analyses. Users have to build everything from scratch.

### 4. Protocol Parsing Depth

**Pixie parsed these protocols via eBPF:**

| Protocol | Pixie | dogwatch |
|----------|-------|----------|
| HTTP/1.1 | ✅ Full | ✅ Basic |
| HTTP/2 | ✅ Full | ❌ |
| gRPC | ✅ Full (with proto reflection) | ❌ |
| MySQL | ✅ Full | 🎯 Planned |
| PostgreSQL | ✅ Full | 🎯 Planned |
| Cassandra | ✅ Full | ❌ |
| Redis | ✅ Full | 🎯 Planned |
| Kafka | ✅ Full | ❌ |
| NATS | ✅ Full | ❌ |
| DNS | ✅ Full | ❌ |
| AMQP | ✅ Full | ❌ |

**Pixie's "Stirling" collector:**
```cpp
// Pixie's protocol detection
void ProcessData(const SocketDataEvent& event) {
    // Detect protocol from first bytes
    Protocol proto = InferProtocol(event.data);

    switch (proto) {
        case kHTTP1:
            ParseHTTP1(event);
            break;
        case kHTTP2:
            ParseHTTP2Frame(event);
            break;
        case kMySQL:
            ParseMySQLPacket(event);
            break;
        case kPostgres:
            ParsePGMessage(event);
            break;
        case kRedis:
            ParseRESP(event);
            break;
        case kKafka:
            ParseKafkaMessage(event);
            break;
        // ... 10+ more protocols
    }
}
```

### 5. Continuous Profiling (Built-in)

**Pixie included CPU profiling:**
```
┌─────────────────────────────────────────────────────────────┐
│  CPU Flamegraph - api-service (last 5 minutes)              │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ ┌──────────────────────────────────────────────────────────┐│
│ │                      main.main                            ││
│ ├────────────────────────────────┬─────────────────────────┤│
│ │        http.Handler            │    background.Worker    ││
│ ├──────────────┬─────────────────┤                         ││
│ │ json.Marshal │  db.Query       │                         ││
│ │              ├────────┬────────┤                         ││
│ │              │ net.io │ encode │                         ││
│ └──────────────┴────────┴────────┴─────────────────────────┘│
│                                                              │
│  Top functions by CPU:                                      │
│  1. json.Marshal (23%)                                      │
│  2. runtime.mallocgc (18%)                                  │
│  3. db.(*Conn).Query (15%)                                  │
└─────────────────────────────────────────────────────────────┘
```

**eBPF-based, always-on:**
```c
// Sample stack traces at 100Hz
SEC("perf_event")
int sample_cpu(struct bpf_perf_event_data *ctx) {
    struct stack_key key = {};
    key.pid = bpf_get_current_pid_tgid() >> 32;
    key.user_stack = bpf_get_stackid(ctx, &stacks, BPF_F_USER_STACK);
    key.kernel_stack = bpf_get_stackid(ctx, &stacks, 0);

    u64 *count = bpf_map_lookup_elem(&stack_counts, &key);
    if (count) {
        (*count)++;
    } else {
        u64 one = 1;
        bpf_map_update_elem(&stack_counts, &key, &one, BPF_ANY);
    }
    return 0;
}
```

**What we're missing:** No profiling at all. Can't answer "why is this slow?"

### 6. Dynamic Logging

**Pixie could add logs to running code without redeployment:**
```python
# Add a log statement to a function without restarting the service
px.add_log(
    function="main.handleRequest",
    log="Request: method={method}, path={path}, user={user_id}",
    variables=["method", "path", "user_id"]
)
```

**How it works:**
- Uses eBPF uprobes to hook function entry/exit
- Reads function arguments from registers/stack
- No code change, no restart, no SDK

**What we're missing:** Can only see what's already instrumented.

### 7. Kubernetes-Native

**Pixie was deeply integrated with K8s:**

```
Automatic discovery:
- All pods, services, deployments
- Pod-to-pod traffic mapping
- Service mesh awareness (Istio, Linkerd)
- K8s events correlation

Query by K8s concepts:
px.run("http_requests", pod="api-*", namespace="production")

Automatic labeling:
- Every trace tagged with pod, node, deployment, namespace
- No configuration needed
```

**What we're missing:** Probably runs on K8s but doesn't deeply integrate with it.

### 8. Data Retention Strategy

**Pixie's approach:**
```
Short-term, high-resolution data:
├── Last 24 hours: Full resolution (every event)
├── Stored in: In-cluster memory + local disk
├── Size: ~50GB per node
└── No egress: Data never leaves cluster

Long-term export (optional):
├── Export aggregates to New Relic
├── Or export to your own S3/GCS
└── Raw data stays local
```

**Philosophy:** Keep detailed data short-term where it's useful (debugging recent issues). Export summaries for long-term trends.

**What we're missing:** Probably trying to keep everything forever, which doesn't scale.

---

### The Pixie Playbook (What Made Them Acquirable)

| Factor | What Pixie Did | Impact |
|--------|---------------|--------|
| **Zero config** | Install → instant visibility | 5-minute time-to-value |
| **No egress** | Data stays in cluster | Unlocks regulated industries |
| **Edge compute** | Queries run locally | Infinite scale, low latency |
| **Protocol depth** | 15+ protocols parsed | See everything without SDKs |
| **Scripts** | Pre-built analyses | Users productive immediately |
| **Profiling** | Built-in flamegraphs | Complete performance picture |
| **K8s native** | Deep integration | Natural fit for modern infra |

**What got them acquired:**
1. Unique tech (eBPF protocol parsing at scale)
2. Fast time-to-value (demo in 5 minutes)
3. Solves real pain (Datadog costs, instrumentation burden)
4. Open source momentum (Apache 2.0)
5. Team (ex-Google, deep systems expertise)

---

### What dogwatch Should Steal From Pixie

**High priority:**
1. **Pre-built scripts/analyses** - "px run slow_queries" equivalent
2. **Protocol depth** - HTTP/2, gRPC, Kafka, DNS
3. **Continuous profiling** - CPU flamegraphs
4. **K8s deep integration** - Auto-discover, auto-label

**Medium priority:**
5. **Edge-first option** - Data stays in cluster mode
6. **Query language** - Something better than raw SQL/PromQL

**Lower priority (complex):**
7. **Dynamic logging** - Add logs without restart

---

### Pixie's Limitations (Where We Can Be Better)

| Pixie Limitation | dogwatch Opportunity |
|------------------|----------------------|
| **K8s only** | Support VMs, bare metal, Docker |
| **Short retention** | Configurable long-term storage |
| **No alerting** | Built-in alerting |
| **Complex deployment** | Single binary simplicity |
| **Limited dashboards** | Better visualization |
| **New Relic lock-in** | Truly open, no vendor |

**Our positioning vs Pixie:**
- Pixie = K8s-only, edge-first, short-term, New Relic ecosystem
- dogwatch = Anywhere, simple, long-term option, independent

---

## What the Big Players Have (Beyond Chronosphere)

### Datadog (Market Leader, ~$2B ARR)

**Integrations at Scale:**
- 800+ integrations (AWS, Azure, GCP, every database, every framework)
- Turn-key dashboards for each integration
- Agent auto-discovery of services
- **Gap:** We have ~0 integrations. Manual setup only.

**APM Features:**
- Code-level profiling (continuous profiler)
- Deployment tracking built-in
- Error tracking with stack traces
- Service catalog with SLOs
- **Gap:** We have eBPF tracing but no profiling, no error grouping.

**Notebooks:**
- Interactive investigation documents
- Mix metrics, logs, traces in one view
- Shareable with team
- **Gap:** We have dashboards but no notebooks.

**Watchdog (ML):**
- Automatic anomaly detection
- Forecasting
- Root cause suggestions
- **Gap:** We have nothing ML-based yet.

### Honeycomb (Query Innovation)

**BubbleUp (We Planned This):**
- Statistical comparison of anomalous vs baseline
- Surface dimensions that explain difference
- **Status:** In our vision, not implemented

**Query Builder:**
- Visual query construction
- "What if" exploration
- Query history and sharing
- **Gap:** We need a better query UX.

**SLO Management:**
- Native SLO creation
- Burn rate dashboards
- Error budget alerts
- **Gap:** Not implemented yet.

**Team Collaboration:**
- Query permalinks
- Annotations on events
- Collaborative debugging
- **Gap:** Basic sharing only.

### Grafana Labs (Open Source Leader)

**Dashboard Excellence:**
- 12+ visualization types
- Panel plugins
- Variables and templating
- Dashboard as code (JSON)
- **Gap:** Our dashboards are basic.

**Alerting System:**
- Unified alerting across data sources
- Alert state history
- Silences and mute timings
- Contact point routing
- **Gap:** Basic alerting only.

**Loki (Logs):**
- LogQL query language
- Label-based indexing (not full-text)
- Extremely cost-efficient at scale
- **Gap:** Our logs are basic SQLite storage.

**Tempo (Traces):**
- TraceQL query language
- Massively scalable (object storage backend)
- Exemplars linking metrics to traces
- **Gap:** Our traces are SQLite, no advanced querying.

**Mimir (Metrics):**
- Global view across clusters
- Long-term storage (years)
- Cardinality limits and sharding
- **Gap:** Our metrics don't scale horizontally.

### New Relic (Full Stack)

**Errors Inbox:**
- Automatic error grouping
- Stack trace deduplication
- Assignment and triage workflow
- **Gap:** We don't group errors at all.

**Logs in Context:**
- Automatic correlation of logs to traces
- Click from span to see related logs
- **Gap:** Manual correlation only.

**Vulnerability Management:**
- CVE detection in dependencies
- Runtime vulnerability analysis
- **Gap:** We don't track vulnerabilities.

### Lightstep (ServiceNow, $500M acquisition)

**Change Intelligence:**
- Deep deployment correlation
- Regression detection after deploys
- **Status:** In our vision as Change Correlation.

**Service Health:**
- Automatic SLI detection
- Health scoring per service
- Dependency-aware health
- **Gap:** No health scoring.

### Dynatrace ($1B+ ARR)

**Automatic Root Cause (Davis AI):**
- Truly automatic (not just suggestions)
- Correlates across infrastructure, apps, users
- Natural language explanations
- **Gap:** Our BubbleUp is manual trigger, not automatic.

**Real User Monitoring:**
- Session replay
- User journey tracking
- Performance by geography
- **Gap:** We don't do RUM.

**Infrastructure as Code:**
- Monaco CLI for config management
- GitOps workflows
- **Gap:** No IaC story.

---

## What Chronosphere Did (That We're Not Doing)

Chronosphere raised $255M and got acquired for $3.35B. They focused on **metrics at massive scale** and **cost control**.

### 1. M3DB - Custom Time-Series Database

**The problem they solved:**
```
Prometheus works great at 1M time series.
At 100M series, it falls over.
At 1B series, nothing works.

Enterprise customers have 1B+ series.
```

**What they built:**
```
M3DB Architecture:
┌─────────────────────────────────────────────────────────────┐
│                        M3 Coordinator                        │
│            (Query routing, aggregation)                      │
└─────────────────────────┬───────────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        │                 │                 │
   ┌────▼────┐       ┌────▼────┐       ┌────▼────┐
   │  M3DB   │       │  M3DB   │       │  M3DB   │
   │ Node 1  │       │ Node 2  │       │ Node 3  │
   │         │       │         │       │         │
   │ Shard   │       │ Shard   │       │ Shard   │
   │ 0,3,6   │       │ 1,4,7   │       │ 2,5,8   │
   └─────────┘       └─────────┘       └─────────┘

Features:
- Consistent hashing for sharding
- Replication factor 3
- Automatic rebalancing
- Compression: 1.5 bytes/datapoint
- Query: Sub-second on billions of points
```

**Capabilities:**
| Feature | M3DB | SQLite (us) |
|---------|------|-------------|
| Max series | 10B+ | ~10M |
| Horizontal scale | Yes | No |
| Replication | Built-in | Manual |
| Downsampling | Automatic | Manual |
| Compression | 12:1 | Basic |

**What we'd need:**
```go
// Option 1: Build distributed layer on SQLite (hard)
// Option 2: Integrate VictoriaMetrics (easier)
// Option 3: Accept scale limit, focus on simplicity (pragmatic)

type MetricsBackend interface {
    Write(metrics []Metric) error
    Query(promql string, start, end time.Time) (Result, error)
}

// Default: SQLite (simple, works for 90% of users)
type SQLiteBackend struct{}

// Scale: VictoriaMetrics (for power users)
type VictoriaMetricsBackend struct {
    endpoint string
}
```

### 2. Control Plane - Their $3.35B Feature

**The core insight:**
```
Most observability data is WASTE:
- 80% of metrics are never queried
- 50% of cardinality comes from 3 labels
- 1 runaway service can blow the entire budget

Chronosphere's control plane:
- Shows exactly what's used vs wasted
- Lets you shape data BEFORE it's stored
- Attributes cost to teams
```

**What they built:**
```
┌─────────────────────────────────────────────────────────────┐
│                    Chronosphere Control Plane                │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  📊 Usage Analytics                                         │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Metric: http_request_duration_bucket                 │   │
│  │ Cardinality: 2.4M series                            │   │
│  │ Storage: 847GB                                       │   │
│  │ Queries (30d): 3                                     │   │
│  │ Dashboards: 0                                        │   │
│  │ Alerts: 0                                            │   │
│  │                                                       │   │
│  │ Recommendation: DROP (never used)                    │   │
│  │ Savings: $12,400/month                               │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  📈 Cardinality Explorer                                    │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ http_requests_total breakdown:                       │   │
│  │                                                       │   │
│  │ Label        Unique Values    Impact                 │   │
│  │ ─────────────────────────────────────                │   │
│  │ pod_name     12,847          92% of cardinality     │   │
│  │ request_id   ∞ (unique)      CRITICAL - drop this   │   │
│  │ method       5               <1%                     │   │
│  │ status       12              <1%                     │   │
│  │ path         234             2%                      │   │
│  │                                                       │   │
│  │ [Drop pod_name] [Drop request_id] [Preview]          │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  💰 Cost Attribution                                        │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Team             Series      Storage    Cost/mo     │   │
│  │ ─────────────────────────────────────────────       │   │
│  │ Platform         12.4M       2.1TB      $8,400      │   │
│  │ Payments         8.2M        1.4TB      $5,600      │   │
│  │ Search           45.8M       7.8TB      $31,200     │   │
│  │ ← WHY SO HIGH?                                       │   │
│  │                                                       │   │
│  │ Search team breakdown:                               │   │
│  │ └─ search_index_* metrics: 43M series (94%)         │   │
│  │    └─ shard_id label: 10,000 unique values          │   │
│  │    └─ Recommendation: Aggregate away shard_id       │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  🎛️ Shaping Rules                                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ Rule 1: Drop unused metrics                         │   │
│  │   match: {__name__=~"go_.*"}                        │   │
│  │   action: drop                                       │   │
│  │   savings: $4,200/mo                                │   │
│  │                                                       │   │
│  │ Rule 2: Aggregate high-cardinality                  │   │
│  │   match: {__name__="http_requests_total"}           │   │
│  │   action: aggregate_away(pod_name, instance)        │   │
│  │   savings: $18,000/mo                               │   │
│  │                                                       │   │
│  │ Rule 3: Downsample old data                         │   │
│  │   match: {__name__=~".*"}                           │   │
│  │   after: 7d → 5m resolution                         │   │
│  │   after: 30d → 1h resolution                        │   │
│  └─────────────────────────────────────────────────────┘   │
│                                                              │
│  Total savings from rules: $34,600/month                    │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**Why this is worth $3.35B:**
```
Customer: Paying $500K/year for Datadog
Chronosphere: "We'll cut that to $150K/year"
Customer: "How?"
Chronosphere: "You're storing 80% garbage. We'll show you what to drop."

Result: 70% cost reduction
Customers: Love it, never leave
Moat: Deep integration with their data, hard to switch
```

### 3. Fluent Bit / Collector Integration

**What they built:**
```
Traditional pipeline:
App → Prometheus → Remote Write → Storage
         ↓
    All data stored, then you figure out what's useful

Chronosphere pipeline:
App → Collector → Shaping Engine → Storage
                        ↓
              Drop/aggregate BEFORE storage
              Only store what's useful
```

**Their collector features:**
```yaml
# Chronosphere collector config
pipelines:
  metrics:
    processors:
      # Drop metrics that match patterns
      - drop:
          match: '{__name__=~"go_gc_.*"}'

      # Aggregate away high-cardinality labels
      - aggregate:
          match: '{__name__="http_requests_total"}'
          without: [pod, instance, request_id]

      # Rate limit per metric
      - rate_limit:
          match: '{team="search"}'
          max_series: 1000000

      # Add labels
      - relabel:
          source: [namespace]
          target: team
          mapping:
            payments-*: payments
            search-*: search
```

### 4. Prometheus Compatibility

**Why this matters:**
```
Every K8s cluster already has Prometheus.
Migration path:
1. Point Prometheus remote_write at Chronosphere
2. Dashboards/alerts keep working (PromQL compatible)
3. Add Chronosphere control plane
4. Start shaping data
5. Eventually: Replace Prometheus with Chronosphere collector
```

**What we need:**
- Full PromQL support (not subset)
- Remote write receiver
- Remote read support
- Drop-in Prometheus replacement

### 5. Multi-Tenancy at Scale

**What enterprises need:**
```
┌─────────────────────────────────────────────────────────────┐
│                     Chronosphere                             │
├─────────────────────────────────────────────────────────────┤
│  Tenant: Acme Corp                                          │
│  ├── Team: Platform                                         │
│  │   ├── Quota: 10M series                                 │
│  │   ├── Retention: 30d                                    │
│  │   └── Users: 12                                         │
│  ├── Team: Payments                                         │
│  │   ├── Quota: 5M series                                  │
│  │   ├── Retention: 90d (compliance)                       │
│  │   └── Users: 8                                          │
│  └── Team: Search                                           │
│      ├── Quota: 20M series                                 │
│      ├── Retention: 15d                                    │
│      └── Users: 15                                         │
│                                                              │
│  Cross-tenant (platform team only):                         │
│  └── Can query all teams for global dashboards             │
└─────────────────────────────────────────────────────────────┘
```

**Features:**
- Per-tenant quotas (hard limits, not just alerts)
- Per-tenant retention policies
- Tenant-aware query routing
- Cross-tenant queries for admins
- Chargeback reports per tenant

### 6. Recording Rules at Scale

**What they built:**
```yaml
# Pre-compute expensive queries
recording_rules:
  - name: api_latency_by_endpoint
    query: |
      histogram_quantile(0.99,
        sum(rate(http_request_duration_bucket[5m])) by (le, endpoint)
      )
    interval: 1m
    # Stored as new metric: api_latency_by_endpoint

  - name: error_rate_by_service
    query: |
      sum(rate(http_requests_total{status=~"5.."}[5m])) by (service)
      / sum(rate(http_requests_total[5m])) by (service)
    interval: 1m
```

**Why this matters:**
- Dashboard loads instantly (reads pre-computed value)
- Reduces query load by 100x
- Enables longer retention of aggregates

### 7. Governance & Compliance Features

**What enterprises need:**
```
Audit Trail:
- Who created/modified shaping rules
- Who queried what data
- Data retention compliance

Access Control:
- Team A can only see team A metrics
- Admins can see everything
- Read-only roles for contractors

Data Governance:
- PII detection in labels
- Automatic masking
- Retention enforcement
```

---

### Chronosphere's Playbook (What Made Them Worth $3.35B)

| Factor | What They Did | Why It Worked |
|--------|--------------|---------------|
| **Focus** | Metrics only (not traces/logs) | Did one thing extremely well |
| **Scale** | 10B+ series | Unlocked enterprises Prometheus couldn't handle |
| **Control plane** | Cost visibility + shaping | Saved customers 70% on observability |
| **Prometheus compat** | Drop-in replacement | Easy migration, keep existing dashboards |
| **Enterprise ready** | SOC2, HIPAA, RBAC | Unlocked regulated industries |
| **Team quotas** | Per-team limits | Platform teams love this |

**The Chronosphere formula:**
```
1. Take open source (Prometheus)
2. Make it scale (M3DB)
3. Add cost control (Control Plane)
4. Add enterprise features (SSO, RBAC, audit)
5. Sell to enterprises paying $500K+/year for Datadog
6. Show them 70% savings
7. Get acquired for $3.35B
```

---

### What dogwatch Should Steal From Chronosphere

**High priority:**
1. **Usage analytics** - Show what's queried vs wasted
2. **Cardinality explorer** - Find exploding labels
3. **Cost attribution** - Per-team/service cost breakdown
4. **Shaping rules** - Drop/aggregate at ingest
5. **Prometheus compatibility** - Full PromQL, remote write/read

**Medium priority:**
6. **Per-team quotas** - Hard limits, not just alerts
7. **Recording rules** - Pre-compute expensive queries
8. **Governance** - Audit trail, access control

**Lower priority (needs scale first):**
9. **Distributed storage** - We need to hit scale limits first
10. **Multi-tenancy** - Only matters for platform teams

---

### Chronosphere's Limitations (Where We Can Be Better)

| Chronosphere Limitation | dogwatch Opportunity |
|------------------------|----------------------|
| **Metrics only** | Full observability (traces, logs, metrics) |
| **SaaS only** | Self-hosted option |
| **Complex deployment** | Single binary |
| **Expensive** | Free tier |
| **Enterprise focus** | Works for startups too |
| **No eBPF** | Zero-config instrumentation |

**Our positioning vs Chronosphere:**
- Chronosphere = Metrics at massive scale, enterprise, SaaS, expensive
- dogwatch = Full observability, any scale, self-hosted, simple, free

---

## Additional Architectural Gaps

### Data Architecture

#### 1. Write-Ahead Log (WAL)

**Current:** Direct SQLite writes. Crash during write = potential corruption.

**What production systems have:**
```
Event arrives → Write to WAL → Acknowledge → Background flush to DB
```
- Durability guarantee before acknowledgment
- Recovery from crash by replaying WAL
- Better write throughput (sequential writes)

**Why it matters:** Without WAL, a crash during heavy ingest can corrupt data or lose recent events.

#### 2. Hot/Warm/Cold Tiering

**Current:** All data in SQLite. Same cost for recent and old data.

**What production systems have:**
```
┌─────────────────────────────────────────────────────────┐
│  Hot (0-24h)     │  Warm (1-30d)   │  Cold (30d+)       │
│  Fast SSD        │  Cheaper SSD    │  S3/Object storage │
│  Full resolution │  5m resolution  │  1h resolution     │
│  Query: <100ms   │  Query: <1s     │  Query: <10s       │
└─────────────────────────────────────────────────────────┘
```

**Minimum viable:**
```yaml
storage:
  tiers:
    hot:
      retention: 24h
      backend: sqlite
      path: /fast-ssd/dogwatch/
    warm:
      retention: 30d
      backend: sqlite
      path: /ssd/dogwatch/
      downsample: 5m
    cold:
      retention: 1y
      backend: s3
      bucket: dogwatch-archive
      downsample: 1h
```

#### 3. Compaction Strategy

**Current:** SQLite handles it (VACUUM). No control over compaction timing.

**What production systems have:**
- Scheduled compaction during low-traffic windows
- Incremental compaction (not stop-the-world)
- Compaction metrics (how much reclaimed, duration)

#### 4. Index Design

**Current:** SQLite indexes. May not be optimized for observability queries.

**What's needed for observability:**
```sql
-- Common query patterns need specific indexes:

-- "Show me traces for service X in last hour"
CREATE INDEX idx_traces_service_time ON traces(service, timestamp DESC);

-- "Find logs containing 'error' for trace Y"
CREATE INDEX idx_logs_trace_id ON logs(trace_id);
CREATE INDEX idx_logs_fulltext ON logs USING gin(to_tsvector(message));

-- "Aggregate metrics by label"
CREATE INDEX idx_metrics_labels ON metrics USING gin(labels);
```

**Consider:** Time-series specific storage (TimescaleDB, ClickHouse) has better index structures for this.

#### 5. Connection Pooling

**Current:** Unknown. Likely new connection per request.

**What production systems have:**
- Connection pool with min/max size
- Connection health checking
- Automatic reconnection
- Pool metrics (active, idle, waiting)

---

### Protocol & Format Support (Comprehensive)

**Current state:** eBPF captures wire traffic. No standard ingest APIs. Can't receive data from agents, SDKs, or other collectors.

#### Ingest Protocols - Traces

| Protocol | Port | Format | Who Uses It | Priority |
|----------|------|--------|-------------|----------|
| **OTLP gRPC** | 4317 | Protobuf | OpenTelemetry SDKs, collectors | P0 |
| **OTLP HTTP** | 4318 | JSON/Protobuf | Browser, serverless, firewalled envs | P0 |
| **Jaeger Thrift** | 14268 | Thrift/HTTP | Legacy Jaeger clients | P1 |
| **Jaeger gRPC** | 14250 | Protobuf | Jaeger agents | P1 |
| **Zipkin** | 9411 | JSON | Zipkin clients, Spring Cloud Sleuth | P1 |
| **X-Ray** | - | JSON | AWS Lambda, ECS | P2 |
| **OpenCensus** | 55678 | Protobuf | Legacy OC users (migrating to OTel) | P3 |

**Why OTLP is P0:**
```
The industry has converged on OpenTelemetry:
- All major languages have OTel SDKs
- Cloud providers support OTLP export
- Datadog, New Relic, Honeycomb all accept OTLP
- Not supporting OTLP = "we don't support the standard"
```

**Minimum OTLP implementation:**
```go
// OTLP gRPC receiver
type OTLPTraceReceiver struct {
    tracepb.UnimplementedTraceServiceServer
}

func (r *OTLPTraceReceiver) Export(ctx context.Context, req *tracepb.ExportTraceServiceRequest) (*tracepb.ExportTraceServiceResponse, error) {
    for _, resourceSpans := range req.ResourceSpans {
        resource := resourceSpans.Resource
        for _, scopeSpans := range resourceSpans.ScopeSpans {
            for _, span := range scopeSpans.Spans {
                // Convert OTLP span to internal format
                internalSpan := convertOTLPSpan(span, resource)
                r.storage.WriteSpan(internalSpan)
            }
        }
    }
    return &tracepb.ExportTraceServiceResponse{}, nil
}
```

#### Ingest Protocols - Metrics

| Protocol | Port | Format | Who Uses It | Priority |
|----------|------|--------|-------------|----------|
| **OTLP gRPC/HTTP** | 4317/4318 | Protobuf/JSON | OTel SDKs | P0 |
| **Prometheus remote_write** | 9090 | Protobuf (snappy) | Prometheus, Grafana Agent | P0 |
| **StatsD** | 8125 | UDP text | Apps, legacy systems | P1 |
| **Graphite** | 2003 | TCP text | Legacy monitoring | P2 |
| **InfluxDB line** | 8086 | Text | Telegraf, InfluxDB users | P2 |
| **Datadog agent** | 8126 | JSON | Migrating from Datadog | P2 |
| **Carbon** | 2003 | Text | Graphite ecosystem | P3 |

**Why Prometheus remote_write is P0:**
```
Most Kubernetes clusters already run Prometheus.
Remote_write lets them send metrics to dogwatch without replacing Prometheus.

Prometheus → remote_write → dogwatch (long-term storage, global view)
```

**StatsD is surprisingly important:**
```go
// Dead simple for developers to add custom metrics
import "github.com/DataDog/datadog-go/statsd"

client, _ := statsd.New("localhost:8125")
client.Incr("api.requests", []string{"endpoint:users"}, 1)
client.Timing("api.latency", 235*time.Millisecond, nil, 1)
```

Many apps already emit StatsD. Supporting it = instant custom metrics.

#### Ingest Protocols - Logs

| Protocol | Port | Format | Who Uses It | Priority |
|----------|------|--------|-------------|----------|
| **OTLP gRPC/HTTP** | 4317/4318 | Protobuf/JSON | OTel SDK logs | P0 |
| **Loki push API** | 3100 | JSON | Promtail, Grafana Agent | P1 |
| **Fluent Forward** | 24224 | MessagePack | FluentBit, Fluentd | P1 |
| **Syslog RFC5424** | 514/6514 | Text | Network devices, Unix systems | P1 |
| **Syslog RFC3164** | 514 | Text | Legacy syslog | P2 |
| **GELF** | 12201 | JSON | Graylog ecosystem | P2 |
| **HTTP JSON** | custom | JSON | Generic webhook logs | P1 |

**Why Fluent Forward matters:**
```
FluentBit is THE log collector for Kubernetes.
- DaemonSet on every node
- Lightweight (2MB binary)
- Already deployed in most clusters

If dogwatch speaks Fluent Forward, teams just change the output destination.
No new collector to deploy.
```

**Syslog for network devices:**
```
Routers, switches, firewalls, load balancers all speak syslog.
Without syslog support, you can't observe network infrastructure.
```

#### Wire Protocols (eBPF Parsing)

These are protocols we parse from network traffic via eBPF:

| Protocol | Current | Notes |
|----------|---------|-------|
| **HTTP/1.1** | ✅ Working | Method, path, status, latency |
| **HTTP/2** | ❌ Missing | Frame parsing needed, multiplexing |
| **gRPC** | ❌ Missing | HTTP/2 + protobuf, need proto registry |
| **HTTPS/TLS** | 🔧 Partial | Need SSL uprobes for decryption |
| **MySQL** | 🎯 Priority | Query text, latency, errors |
| **PostgreSQL** | 🎯 Priority | Query text, latency, errors |
| **Redis** | 🎯 Priority | Command, key, latency |
| **MongoDB** | ❌ Missing | BSON wire protocol |
| **Memcached** | ❌ Missing | Simple text protocol |
| **Kafka** | ❌ Missing | Producer/consumer, topic, partition |
| **RabbitMQ/AMQP** | ❌ Missing | Queue operations |
| **DNS** | ❌ Missing | Query, response, latency |
| **NATS** | ❌ Missing | Pub/sub protocol |

**HTTP/2 and gRPC are critical:**
```
Modern microservices = gRPC everywhere.
Without HTTP/2 parsing, we can't trace gRPC services.

Challenges:
- Multiplexed streams on single connection
- HPACK header compression
- Need to maintain stream state
```

**Database protocols are the killer feature:**
```
Most latency lives in database queries.
Auto-capturing queries without instrumentation = magic.

MySQL packet structure:
┌─────────┬────────┬──────────┬─────────────────────┐
│ Length  │ Seq ID │ Command  │ Payload (query)     │
│ 3 bytes │ 1 byte │ 1 byte   │ Variable            │
└─────────┴────────┴──────────┴─────────────────────┘
```

#### Export Protocols

| Protocol | Direction | Use Case | Priority |
|----------|-----------|----------|----------|
| **OTLP** | Push | Forward to other systems | P1 |
| **Prometheus /metrics** | Pull | Let Prometheus scrape dogwatch | P0 |
| **Prometheus remote_read** | Pull | Prometheus queries dogwatch | P2 |
| **S3/GCS/Azure Blob** | Push | Long-term archive | P1 |
| **Kafka** | Push | Stream to data lake | P2 |
| **Webhook** | Push | Alerts, events to external systems | P0 |

**Prometheus /metrics endpoint is P0:**
```
Users expect to scrape dogwatch itself:
- Internal metrics (ingest rate, storage, errors)
- Health monitoring
- Capacity planning

This is table stakes for any Kubernetes-native tool.
```

#### Protocol Priority Matrix

```
                    │ Traces │ Metrics │ Logs │
────────────────────┼────────┼─────────┼──────┤
OTLP gRPC           │   P0   │   P0    │  P0  │
OTLP HTTP           │   P0   │   P0    │  P0  │
Prometheus remote   │   -    │   P0    │  -   │
Jaeger              │   P1   │   -     │  -   │
Zipkin              │   P1   │   -     │  -   │
StatsD              │   -    │   P1    │  -   │
Fluent Forward      │   -    │   -     │  P1  │
Syslog              │   -    │   -     │  P1  │
Loki API            │   -    │   -     │  P1  │
```

#### Implementation Roadmap

**Week 1-2: OTLP Foundation**
```go
// Single receiver handles all three signals
type OTLPReceiver struct {
    traces  tracepb.TraceServiceServer
    metrics metricspb.MetricsServiceServer
    logs    logspb.LogsServiceServer
}

func NewOTLPReceiver(storage Storage) *OTLPReceiver {
    return &OTLPReceiver{
        traces:  &traceReceiver{storage: storage},
        metrics: &metricsReceiver{storage: storage},
        logs:    &logsReceiver{storage: storage},
    }
}

// Start both gRPC and HTTP
func (r *OTLPReceiver) Start() error {
    go r.startGRPC(":4317")
    go r.startHTTP(":4318")
    return nil
}
```

**Week 3-4: Prometheus Ecosystem**
```go
// Remote write receiver
func (r *PrometheusReceiver) RemoteWrite(w http.ResponseWriter, req *http.Request) {
    compressed, _ := io.ReadAll(req.Body)
    data, _ := snappy.Decode(nil, compressed)

    var writeReq prompb.WriteRequest
    proto.Unmarshal(data, &writeReq)

    for _, ts := range writeReq.Timeseries {
        r.storage.WriteTimeseries(ts)
    }

    w.WriteHeader(http.StatusNoContent)
}

// Expose /metrics for scraping
func (r *PrometheusReceiver) Metrics(w http.ResponseWriter, req *http.Request) {
    promhttp.Handler().ServeHTTP(w, req)
}
```

**Week 5-6: Log Collectors**
```go
// Fluent Forward receiver
type FluentReceiver struct {
    listener net.Listener
}

func (r *FluentReceiver) handleConnection(conn net.Conn) {
    decoder := msgpack.NewDecoder(conn)
    for {
        var tag string
        var entries []FluentEntry
        decoder.Decode(&tag, &entries)

        for _, entry := range entries {
            r.storage.WriteLog(Log{
                Timestamp: entry.Time,
                Message:   entry.Record["log"],
                Labels:    map[string]string{"fluent_tag": tag},
            })
        }
    }
}
```

**Week 7-8: Database eBPF Probes**
```c
// MySQL query capture
SEC("uprobe/mysql_dispatch_command")
int capture_mysql_query(struct pt_regs *ctx) {
    struct mysql_event event = {};
    event.timestamp = bpf_ktime_get_ns();
    event.pid = bpf_get_current_pid_tgid() >> 32;

    // Read command type (arg 1)
    event.command = (u8)PT_REGS_PARM1(ctx);

    // Read query string (arg 2)
    char *query = (char *)PT_REGS_PARM2(ctx);
    bpf_probe_read_str(&event.query, sizeof(event.query), query);

    bpf_perf_event_output(ctx, &mysql_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}
```

---

### eBPF Technical Gaps

#### 1. Kernel Compatibility (CO-RE / BTF)

**Current:** Likely compiled for specific kernel version. Different kernel = recompile or fail.

**The problem:**
```
Kernel 5.4 on Ubuntu 20.04 ≠ Kernel 5.15 on Ubuntu 22.04
Struct offsets change between versions
eBPF probe that works on one kernel crashes on another
```

**What production eBPF tools have:**
- **CO-RE (Compile Once, Run Everywhere)** - Uses BTF (BPF Type Format) to relocate struct offsets at load time
- Single binary works across kernel versions
- Graceful degradation when features unavailable

**Implementation:**
```c
// Instead of hardcoded offsets:
// struct task_struct has comm at offset 0x7b8 (WRONG - varies by kernel)

// Use CO-RE:
#include <bpf/bpf_core_read.h>

SEC("kprobe/tcp_connect")
int trace_connect(struct pt_regs *ctx) {
    struct sock *sk = (struct sock *)PT_REGS_PARM1(ctx);

    // CO-RE handles offset relocation automatically
    u16 dport = BPF_CORE_READ(sk, __sk_common.skc_dport);

    return 0;
}
```

**Minimum kernel versions:**
| Feature | Minimum Kernel | Notes |
|---------|---------------|-------|
| Basic eBPF | 4.4 | Ancient, limited |
| Kprobes | 4.14 | Function tracing |
| Uprobes | 4.17 | Userspace tracing |
| BTF | 5.2 | CO-RE support |
| Ring buffer | 5.8 | Better perf than perf_event |
| Signed eBPF | 5.14 | Security requirement |

**Graceful degradation:**
```go
func (e *EBPFLoader) Load() error {
    // Check kernel version and features
    features := e.detectFeatures()

    if !features.HasBTF {
        log.Warn("BTF not available, using legacy probes (may not work on all kernels)")
    }

    if !features.HasRingBuffer {
        log.Warn("Ring buffer not available, falling back to perf events")
    }

    // Load appropriate probes based on available features
    return e.loadProbes(features)
}
```

#### 2. eBPF Performance Overhead

**Current:** Unknown overhead. Could be stealing CPU from workloads.

**What needs measuring:**
- CPU overhead per syscall traced
- Memory for maps and ring buffers
- Impact on application latency (tail latency especially)

**Target overhead:**
```
CPU: < 1% overhead at 100K requests/sec
Memory: < 100MB for eBPF maps
Latency: < 1μs added per traced operation
```

**Measurement approach:**
```go
type EBPFMetrics struct {
    // Overhead metrics
    CPUTimeNs         prometheus.Counter   // Time spent in eBPF programs
    EventsProcessed   prometheus.Counter   // Events from ring buffer
    EventsDropped     prometheus.Counter   // Ring buffer overflow
    MapMemoryBytes    prometheus.Gauge     // Memory used by maps

    // Performance impact
    ProbeLatencyNs    prometheus.Histogram // Time per probe execution
}
```

#### 3. Container Runtime Support

**Current:** Likely works with Docker. May not work with containerd/CRI-O.

**Container runtimes to support:**
| Runtime | Market Share | Notes |
|---------|-------------|-------|
| containerd | 60%+ | Default in K8s 1.24+ |
| Docker | 30% | Legacy, declining |
| CRI-O | 10% | OpenShift default |
| Podman | Growing | Rootless containers |

**Challenges:**
- Different socket paths
- Different process hierarchies
- Different cgroup layouts (v1 vs v2)

**Container detection:**
```go
type ContainerRuntime interface {
    DetectRuntime() RuntimeType
    GetContainerID(pid int) string
    GetContainerName(id string) string
    GetPodInfo(id string) (*PodInfo, error)
}

// Implementations
type ContainerdRuntime struct { socket string }
type DockerRuntime struct { socket string }
type CRIORuntime struct { socket string }

func DetectRuntime() ContainerRuntime {
    if fileExists("/run/containerd/containerd.sock") {
        return &ContainerdRuntime{socket: "/run/containerd/containerd.sock"}
    }
    if fileExists("/var/run/docker.sock") {
        return &DockerRuntime{socket: "/var/run/docker.sock"}
    }
    // ... etc
}
```

#### 4. SSL/TLS Interception

**Current:** HTTPS traffic is encrypted. Can't see HTTP details inside TLS.

**The challenge:**
```
Without SSL interception:
- See: TCP connection to port 443
- Don't see: HTTP method, path, status, headers

With SSL interception:
- See: GET /api/users → 200 OK (47ms)
```

**Approaches:**

| Approach | Pros | Cons |
|----------|------|------|
| **Uprobe OpenSSL** | Works for most apps | Need to find SSL_read/SSL_write symbols |
| **Uprobe BoringSSL** | Works for Go, Chrome | Different symbols |
| **Uprobe GnuTLS** | Works for some apps | Less common |
| **kTLS interception** | Kernel-level | Requires kTLS enabled |
| **eBPF sockops** | No symbol hunting | Limited to newer kernels |

**OpenSSL uprobe implementation:**
```c
// Attach to SSL_read to capture decrypted data
SEC("uprobe/SSL_read")
int trace_ssl_read(struct pt_regs *ctx) {
    void *ssl = (void *)PT_REGS_PARM1(ctx);
    void *buf = (void *)PT_REGS_PARM2(ctx);
    int num = (int)PT_REGS_PARM3(ctx);

    // Store args for return probe
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct ssl_args args = {.ssl = ssl, .buf = buf};
    bpf_map_update_elem(&ssl_args_map, &pid_tgid, &args, BPF_ANY);

    return 0;
}

SEC("uretprobe/SSL_read")
int trace_ssl_read_ret(struct pt_regs *ctx) {
    int ret = PT_REGS_RC(ctx);
    if (ret <= 0) return 0;

    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct ssl_args *args = bpf_map_lookup_elem(&ssl_args_map, &pid_tgid);
    if (!args) return 0;

    // Read decrypted data
    struct ssl_event event = {};
    event.len = ret;
    bpf_probe_read(&event.data, min(ret, sizeof(event.data)), args->buf);

    bpf_perf_event_output(ctx, &ssl_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}
```

**Finding SSL library paths:**
```go
func findSSLLibraries(pid int) []string {
    // Parse /proc/{pid}/maps for loaded libraries
    maps, _ := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))

    var sslLibs []string
    for _, line := range strings.Split(string(maps), "\n") {
        if strings.Contains(line, "libssl") ||
           strings.Contains(line, "libcrypto") ||
           strings.Contains(line, "boringssl") {
            // Extract path
            path := extractPath(line)
            sslLibs = append(sslLibs, path)
        }
    }
    return sslLibs
}
```

---

### Query Engine Gaps

#### 1. Query Optimization

**Current:** Likely naive query execution. Full table scans.

**What production query engines have:**
- Query planning and optimization
- Index selection
- Predicate pushdown
- Parallel execution
- Result caching

**Example optimization:**
```sql
-- User query:
SELECT * FROM traces WHERE service = 'api' AND duration > 1000 ORDER BY timestamp DESC LIMIT 100

-- Naive execution:
-- 1. Full scan of traces table
-- 2. Filter by service
-- 3. Filter by duration
-- 4. Sort by timestamp
-- 5. Take 100
-- Time: 30 seconds on 100M rows

-- Optimized execution:
-- 1. Use index idx_traces_service_time (service, timestamp DESC)
-- 2. Seek to service='api'
-- 3. Scan backward (already sorted by timestamp)
-- 4. Filter duration > 1000 inline
-- 5. Stop after 100 matches
-- Time: 10ms
```

#### 2. Query Language

**Current:** Unknown. Probably basic filters.

**What's needed:**
```
Metrics: PromQL (or compatible subset)
  rate(http_requests_total{service="api"}[5m])
  histogram_quantile(0.99, sum(rate(http_request_duration_bucket[5m])) by (le))

Logs: LogQL (or compatible)
  {service="api"} |= "error" | json | duration > 1s

Traces: TraceQL (or compatible)
  {span.http.status_code >= 500} | count() > 10
```

**Minimum viable:**
```go
type QueryEngine struct {
    metrics *PromQLEngine    // Use existing prometheus/promql
    logs    *LogQLEngine     // Simpler - regex + JSON parsing
    traces  *TraceQueryEngine // Custom - span filtering
}

// For metrics, embed Prometheus query engine
import "github.com/prometheus/prometheus/promql"

func (q *QueryEngine) QueryMetrics(query string, start, end time.Time) (promql.Matrix, error) {
    eng := promql.NewEngine(promql.EngineOpts{})
    qry, _ := eng.NewRangeQuery(q.storage, nil, query, start, end, time.Minute)
    return qry.Exec(context.Background())
}
```

#### 3. Query Caching

**Current:** Every query hits storage. Repeated queries = repeated work.

**What production systems have:**
```
┌─────────────────────────────────────────────────┐
│                 Query Cache                      │
├─────────────────────────────────────────────────┤
│  L1: In-memory LRU (hot queries)                │
│      TTL: 10 seconds                            │
│      Size: 100MB                                │
│                                                  │
│  L2: Query result cache (recent queries)        │
│      TTL: 5 minutes                             │
│      Size: 1GB                                  │
│                                                  │
│  L3: Materialized views (common aggregations)   │
│      Refreshed: Every 1 minute                  │
│      Pre-computed: Top 20 dashboard queries     │
└─────────────────────────────────────────────────┘
```

---

### Data Model Gaps

#### 1. Trace Model Completeness

**Current:** Basic spans. Missing advanced features.

**Full OpenTelemetry trace model:**
```go
type Span struct {
    // Identity
    TraceID     [16]byte
    SpanID      [8]byte
    ParentID    [8]byte   // Optional, root spans have none
    TraceState  string    // W3C trace state

    // Core
    Name        string
    Kind        SpanKind  // CLIENT, SERVER, PRODUCER, CONSUMER, INTERNAL
    StartTime   time.Time
    EndTime     time.Time
    Status      Status    // OK, ERROR, UNSET

    // Context
    Attributes  map[string]any    // Key-value pairs
    Events      []SpanEvent       // Timestamped logs within span
    Links       []SpanLink        // Related traces (async, batch)

    // Resource (what generated this span)
    Resource    Resource          // service.name, host.name, k8s.pod, etc.

    // Instrumentation scope
    Scope       InstrumentationScope  // Library name/version
}

type SpanEvent struct {
    Name       string
    Timestamp  time.Time
    Attributes map[string]any
}

type SpanLink struct {
    TraceID    [16]byte
    SpanID     [8]byte
    TraceState string
    Attributes map[string]any
}
```

**Missing features:**
- **Span Events** - Logs attached to spans (exception stack traces)
- **Span Links** - Connect async operations, batch jobs
- **Resource attributes** - Where the span came from
- **Instrumentation scope** - Which library created it

#### 2. Metric Model Completeness

**Current:** Basic metrics. Probably missing histogram, summary, exemplars.

**Full metric types:**
```go
type Metric interface {
    Name() string
    Labels() map[string]string
}

// Counter - monotonically increasing
type Counter struct {
    Value float64
}

// Gauge - can go up or down
type Gauge struct {
    Value float64
}

// Histogram - distribution with buckets
type Histogram struct {
    Buckets     []Bucket  // {le: 0.1, count: 100}, {le: 0.5, count: 450}...
    Sum         float64
    Count       uint64
    Exemplars   []Exemplar  // Links to traces!
}

// Summary - pre-computed quantiles (less flexible than histogram)
type Summary struct {
    Quantiles   []Quantile  // {q: 0.5, v: 0.023}, {q: 0.99, v: 0.87}
    Sum         float64
    Count       uint64
}

// Exemplar - link metric to trace
type Exemplar struct {
    Value     float64
    Timestamp time.Time
    TraceID   string
    SpanID    string
    Labels    map[string]string
}
```

#### 3. Log Model Completeness

**Current:** Probably text + timestamp.

**Full OpenTelemetry log model:**
```go
type LogRecord struct {
    // Timing
    Timestamp         time.Time
    ObservedTimestamp time.Time  // When collector received it

    // Severity
    SeverityNumber    int32      // 1-24 (TRACE to FATAL)
    SeverityText      string     // "INFO", "ERROR", etc.

    // Content
    Body              any        // String, map, or array
    Attributes        map[string]any

    // Context
    TraceID           [16]byte   // Link to trace!
    SpanID            [8]byte    // Link to specific span!
    TraceFlags        byte

    // Resource
    Resource          Resource   // service.name, etc.
}
```

**Key missing feature: Trace correlation**
```
Every log should automatically get trace_id and span_id
if it's emitted within a traced request context.

Without this, you can't click from trace → logs or logs → trace.
```

---

### Scaling Gaps

#### 1. Horizontal Scaling

**Current:** Single node. Can't scale out.

**What's needed for scale:**
```
Write Path:
┌─────────┐    ┌─────────┐    ┌─────────┐
│ Ingest  │    │ Ingest  │    │ Ingest  │
│ Node 1  │    │ Node 2  │    │ Node 3  │
└────┬────┘    └────┬────┘    └────┬────┘
     │              │              │
     └──────────────┼──────────────┘
                    │
            ┌───────▼───────┐
            │   Consistent  │
            │    Hashing    │
            └───────┬───────┘
                    │
     ┌──────────────┼──────────────┐
     │              │              │
┌────▼────┐   ┌────▼────┐   ┌────▼────┐
│ Storage │   │ Storage │   │ Storage │
│ Node 1  │   │ Node 2  │   │ Node 3  │
└─────────┘   └─────────┘   └─────────┘

Read Path:
┌─────────┐
│  Query  │ ──► Scatter query to all storage nodes
│  Node   │ ◄── Gather and merge results
└─────────┘
```

**Consistent hashing for sharding:**
```go
type ShardRouter struct {
    ring      *hashring.HashRing
    shards    map[string]*ShardClient
}

func (r *ShardRouter) RouteWrite(metric Metric) *ShardClient {
    // Hash by metric name + some labels to distribute evenly
    key := metric.Name + metric.Labels["service"]
    shardID := r.ring.GetNode(key)
    return r.shards[shardID]
}
```

#### 2. Multi-Cluster Federation

**Current:** Single cluster view only.

**What enterprises need:**
```
┌─────────────────────────────────────────────────────────┐
│                    Global View                           │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐          │
│  │ US-East  │    │ US-West  │    │ EU-West  │          │
│  │ Cluster  │    │ Cluster  │    │ Cluster  │          │
│  │          │    │          │    │          │          │
│  │ dogwatch │    │ dogwatch │    │ dogwatch │          │
│  └────┬─────┘    └────┬─────┘    └────┬─────┘          │
│       │               │               │                 │
│       └───────────────┼───────────────┘                 │
│                       │                                  │
│                       ▼                                  │
│              ┌─────────────────┐                        │
│              │  dogwatch       │                        │
│              │  Federation     │                        │
│              │  (aggregates    │                        │
│              │   cross-cluster)│                        │
│              └─────────────────┘                        │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

**Federation capabilities:**
- Cross-cluster queries ("show me p99 latency across all regions")
- Global service map
- Cross-region trace stitching
- Centralized alerting with local evaluation

#### 3. Cardinality Explosion Handling

**Current:** Unknown. Probably crashes or OOMs.

**The problem:**
```
Innocent-looking metric:
  http_requests{user_id="...", request_id="...", timestamp="..."}

With 1M users × 100 requests/sec = 100M unique series
Each series = ~200 bytes metadata
Total = 20GB just for metric metadata
```

**What production systems do:**
```go
type CardinalityLimiter struct {
    maxSeriesPerMetric  int      // e.g., 10,000
    maxLabelsPerSeries  int      // e.g., 20
    maxLabelValueLength int      // e.g., 256

    // Track current cardinality
    seriesCount map[string]int   // metric name → series count
}

func (c *CardinalityLimiter) Check(metric Metric) error {
    // Reject if too many series for this metric
    if c.seriesCount[metric.Name] >= c.maxSeriesPerMetric {
        return ErrCardinalityLimitExceeded
    }

    // Reject if too many labels
    if len(metric.Labels) > c.maxLabelsPerSeries {
        return ErrTooManyLabels
    }

    // Reject if label value too long (often sign of high cardinality)
    for _, v := range metric.Labels {
        if len(v) > c.maxLabelValueLength {
            return ErrLabelValueTooLong
        }
    }

    return nil
}
```

**Alerting on cardinality:**
```yaml
alerts:
  - name: HighCardinalityMetric
    expr: dogwatch_metric_series_count > 50000
    for: 5m
    annotations:
      summary: "Metric {{ $labels.metric_name }} has {{ $value }} series"
      action: "Add aggregation rule or drop high-cardinality labels"
```

---

### Time & Ordering Gaps

#### 1. Clock Skew Handling

**Current:** Assumes clocks are synchronized. They're not.

**The problem:**
```
Service A (clock +2s fast):  Request sent at 14:00:02
Service B (clock correct):   Request received at 14:00:00

Trace shows: B received request BEFORE A sent it (impossible)
```

**Solutions:**
```go
type ClockSkewCorrector struct {
    // Track observed skew between services
    skew map[string]map[string]time.Duration  // source → dest → skew
}

func (c *ClockSkewCorrector) Correct(span Span) Span {
    // If parent span exists, ensure child starts after parent
    if span.ParentID != "" {
        parent := c.getSpan(span.ParentID)
        if span.StartTime.Before(parent.StartTime) {
            // Adjust child to start at parent start + small delta
            span.StartTime = parent.StartTime.Add(time.Microsecond)
        }
    }
    return span
}
```

**Better solution: Use monotonic trace-local time:**
```go
type Span struct {
    // Wall clock time (may have skew)
    StartTime time.Time
    EndTime   time.Time

    // Trace-local relative time (monotonic within trace)
    RelativeStartNs int64  // Nanoseconds since trace start
    RelativeEndNs   int64
}
```

#### 2. Late-Arriving Data

**Current:** Unknown. Probably drops or misorders.

**The problem:**
```
Normal: Events arrive in order, processed immediately
Reality: Network delays, retries, batch uploads

Event from 10 minutes ago arrives now.
If we've already compacted/aggregated that time window, what do we do?
```

**Solutions:**
```go
type LateArrivalHandler struct {
    maxLateArrival time.Duration  // e.g., 1 hour
    reprocessQueue chan Event     // Queue for late events
}

func (h *LateArrivalHandler) Handle(event Event) error {
    age := time.Since(event.Timestamp)

    if age > h.maxLateArrival {
        // Too old, drop with metric
        lateEventsDropped.Inc()
        return nil
    }

    if age > 5*time.Minute {
        // Late but acceptable, queue for reprocessing
        h.reprocessQueue <- event
        return nil
    }

    // Recent, process normally
    return h.process(event)
}
```

---

### Testing Gaps

#### 1. eBPF Testing

**Current:** Probably manual testing only.

**The challenge:**
- eBPF runs in kernel, hard to unit test
- Need real syscalls to trigger probes
- Different behavior on different kernels

**Testing approach:**
```go
// Integration test with real eBPF
func TestHTTPTracing(t *testing.T) {
    // Start dogwatch with eBPF
    dw := startDogwatch(t)
    defer dw.Stop()

    // Make HTTP request that will be traced
    resp, _ := http.Get("http://localhost:8080/test")

    // Wait for trace to be captured
    time.Sleep(100 * time.Millisecond)

    // Query for the trace
    traces := dw.QueryTraces("http.path = '/test'")
    assert.Len(t, traces, 1)
    assert.Equal(t, "GET", traces[0].Method)
    assert.Equal(t, 200, traces[0].Status)
}
```

**Kernel version matrix testing:**
```yaml
# CI matrix
test:
  strategy:
    matrix:
      kernel: ["5.4", "5.10", "5.15", "6.1", "6.5"]
      distro: ["ubuntu-20.04", "ubuntu-22.04", "debian-11", "rocky-8"]
```

#### 2. Load Testing

**Current:** Unknown performance characteristics.

**What's needed:**
```bash
# Synthetic load generator
dogwatch-bench \
  --traces-per-sec 10000 \
  --metrics-per-sec 100000 \
  --logs-per-sec 50000 \
  --duration 1h \
  --report performance-report.json
```

**Metrics to capture:**
- Max ingest rate before data loss
- Query latency at various loads
- Memory usage over time
- CPU usage breakdown
- Storage growth rate

---

### Deployment Model Gaps

#### 1. Kubernetes Deployment Modes

**Current:** Probably standalone binary only.

**What's needed:**

| Mode | Use Case | Implementation |
|------|----------|----------------|
| **DaemonSet** | One per node, eBPF collection | Needs hostPID, privileged |
| **Deployment** | Central aggregation/query | Stateless, scalable |
| **StatefulSet** | Storage nodes | Persistent volumes |
| **Sidecar** | Per-pod collection | Istio-style injection |

**DaemonSet for eBPF:**
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dogwatch-agent
spec:
  template:
    spec:
      hostPID: true      # Required for eBPF
      hostNetwork: true  # Required for network tracing
      containers:
      - name: dogwatch
        image: dogwatch:latest
        securityContext:
          privileged: true  # Required for eBPF
        volumeMounts:
        - name: sys
          mountPath: /sys
          readOnly: true
        - name: modules
          mountPath: /lib/modules
          readOnly: true
      volumes:
      - name: sys
        hostPath:
          path: /sys
      - name: modules
        hostPath:
          path: /lib/modules
```

#### 2. Sidecar vs DaemonSet

**DaemonSet (one per node):**
```
Pros: Less overhead, see all pods on node
Cons: Shared context, noisy neighbor issues
```

**Sidecar (one per pod):**
```
Pros: Isolated, per-pod configuration
Cons: More resource overhead, more complexity
```

**Hybrid approach:**
```
DaemonSet: eBPF collection (must be per-node)
Sidecar: Application-specific instrumentation (optional)
```

---

### Feature Gaps: Things We Don't Do At All

#### 1. Continuous Profiling

**What it is:** Always-on CPU/memory profiling at low overhead.

**Why it matters:**
```
Traditional profiling: Turn on when debugging, turn off after
Continuous profiling: Always on, see profile for any time range

"Why was the API slow yesterday at 3am?"
→ Pull up CPU profile from that exact time
→ See: 40% of CPU in json.Marshal, 30% in database driver
```

**Leaders:** Datadog, Pyroscope, Parca (open source)

**Implementation approach:**
```go
// eBPF-based stack sampling
SEC("perf_event")
int sample_stack(struct bpf_perf_event_data *ctx) {
    struct stack_event event = {};

    // Capture user + kernel stack
    event.user_stack_id = bpf_get_stackid(ctx, &stack_traces, BPF_F_USER_STACK);
    event.kernel_stack_id = bpf_get_stackid(ctx, &stack_traces, 0);
    event.pid = bpf_get_current_pid_tgid() >> 32;
    event.timestamp = bpf_ktime_get_ns();

    bpf_perf_event_output(ctx, &profile_events, BPF_F_CURRENT_CPU, &event, sizeof(event));
    return 0;
}
```

**Priority:** P2 - Differentiating but complex. Consider integrating Pyroscope instead of building.

#### 2. Synthetic Monitoring

**What it is:** Scheduled checks that probe your services from outside.

**Why it matters:**
```
Real user monitoring: See problems after users hit them
Synthetic monitoring: Detect problems before users do

Every 30 seconds:
- Hit /api/health from 5 regions
- Alert if latency > 500ms or status != 200
- Track uptime over time
```

**Minimum viable:**
```yaml
synthetics:
  - name: API Health Check
    type: http
    url: https://api.example.com/health
    interval: 30s
    timeout: 5s
    assertions:
      - type: status_code
        operator: equals
        value: 200
      - type: response_time
        operator: less_than
        value: 500ms
    locations:
      - us-east
      - us-west
      - eu-west
    alerts:
      - type: failure
        threshold: 2  # Alert after 2 consecutive failures
```

**Priority:** P2 - Expected feature, but can use external tools (Checkly, Uptime Robot) initially.

#### 3. Error Tracking

**What it is:** Automatic grouping and tracking of application errors with stack traces.

**What Sentry does:**
```
1. Capture exception with full stack trace
2. Group similar errors (same root cause)
3. Track error frequency over time
4. Link to deploys that introduced errors
5. Assign to team members
6. Track resolution status
```

**Current dogwatch:** Logs have errors, but no grouping, no stack traces, no workflow.

**Minimum viable:**
```go
type ErrorGroup struct {
    ID            string
    Fingerprint   string      // Hash of stack trace for grouping
    Message       string      // Error message pattern
    FirstSeen     time.Time
    LastSeen      time.Time
    Count         int64
    AffectedUsers int64
    Status        string      // "unresolved", "resolved", "ignored"
    AssignedTo    string
    Traces        []string    // Sample trace IDs
}

// Fingerprinting algorithm
func fingerprintError(err ErrorEvent) string {
    // Use top N frames of stack trace, normalized
    var frames []string
    for i, frame := range err.StackTrace[:min(5, len(err.StackTrace))] {
        // Normalize: remove line numbers, memory addresses
        normalized := fmt.Sprintf("%s:%s", frame.File, frame.Function)
        frames = append(frames, normalized)
    }
    return hash(strings.Join(frames, "|"))
}
```

**Priority:** P1 - High value, builds on existing log/trace data.

#### 4. Release Tracking

**What it is:** Track which version of code is running where, correlate with issues.

**Why it matters:**
```
Without release tracking:
- "There's an error spike"
- "Did we deploy something?"
- "Let me check the deploy log..."
- "Looks like we deployed at 2pm"
- "Was that before or after the spike?"

With release tracking:
- Error spike at 2:15pm
- Annotation on graph: "v2.4.1 deployed at 2:10pm"
- Click to see: 847 new errors since v2.4.1, 0 before
- One-click rollback button
```

**Data model:**
```go
type Release struct {
    ID           string
    Service      string
    Version      string
    Environment  string
    DeployedAt   time.Time
    DeployedBy   string
    CommitSHA    string
    CommitURL    string
    PreviousVersion string

    // Computed
    ErrorRate    float64  // Errors/sec since deploy
    ErrorDelta   float64  // Change from previous version
    LatencyP99   float64
    LatencyDelta float64
}

// Auto-detect from k8s
func (d *ReleaseDetector) WatchDeployments() {
    watcher := d.k8sClient.Watch("deployments")
    for event := range watcher.Events {
        if event.Type == "MODIFIED" {
            deploy := event.Object
            if isNewVersion(deploy) {
                d.recordRelease(Release{
                    Service: deploy.Name,
                    Version: deploy.Spec.Template.Labels["version"],
                    DeployedAt: time.Now(),
                    // ...
                })
            }
        }
    }
}
```

**Priority:** P1 - Essential for Change Correlation feature.

#### 5. Service Catalog

**What it is:** Registry of all services with ownership, dependencies, runbooks.

**Why it matters:**
```
Incident at 3am:
- Alert: "payment-service latency > 5s"
- Who owns payment-service? → Team: Payments, On-call: alice@
- What does it depend on? → stripe-api, postgres-payments, redis
- Runbook? → "Check Stripe status page, then check DB connections"
```

**Data model:**
```go
type ServiceDefinition struct {
    Name         string
    DisplayName  string
    Description  string
    Team         string
    Owner        string      // Primary contact
    Oncall       string      // PagerDuty schedule
    SlackChannel string
    Repository   string
    Runbook      string      // URL to runbook

    // Auto-discovered
    Dependencies []string    // Services this calls
    Dependents   []string    // Services that call this
    Endpoints    []Endpoint
    SLOs         []SLO

    // Classification
    Tier         string      // "critical", "standard", "experimental"
    DataClass    string      // "pii", "financial", "public"
}
```

**Auto-discovery from traces:**
```go
// Build dependency graph from actual traffic
func (c *ServiceCatalog) UpdateFromTraces(traces []Trace) {
    for _, trace := range traces {
        for _, span := range trace.Spans {
            if span.Kind == CLIENT {
                source := span.Resource.ServiceName
                dest := span.Attributes["peer.service"]
                c.recordDependency(source, dest)
            }
        }
    }
}
```

**Priority:** P2 - Nice to have, but can function without it initially.

#### 6. Runbooks & Automation

**What it is:** Step-by-step guides for responding to alerts, with automation.

**Basic runbooks:**
```yaml
runbooks:
  - alert: HighErrorRate
    service: api
    steps:
      - description: Check recent deployments
        query: "releases{service='api', time > now-1h}"
      - description: Check dependent services
        query: "errors{service=~'api-.*'}"
      - description: Check database
        query: "pg_connections{database='api'}"
    actions:
      - name: Rollback to previous version
        command: "kubectl rollout undo deployment/api"
        requires_approval: true
```

**Automated remediation:**
```yaml
automations:
  - trigger:
      alert: PodOOMKilled
      count: 3  # After 3 occurrences
    action:
      type: scale_up
      target: deployment/{{ .service }}
      increment: 1
      max: 10
    notification:
      channel: "#incidents"
      message: "Auto-scaled {{ .service }} due to OOM"
```

**Priority:** P3 - Advanced feature, integrate with existing tools first.

#### 7. Cost Attribution

**What it is:** Break down observability costs by team/service.

**Why it matters for self-hosted:**
```
Even though dogwatch is free, storage/compute cost money.

"Team A uses 80% of our observability storage"
"Service X emits 10M metrics/hour, 100x more than anything else"
"If we add this new instrumentation, it will cost $X/month in compute"
```

**Implementation:**
```go
type CostAttribution struct {
    // Track usage per service/team
    MetricSeries   map[string]int64   // service → series count
    LogBytes       map[string]int64   // service → bytes/day
    TraceSpans     map[string]int64   // service → spans/day
    StorageBytes   map[string]int64   // service → storage used
}

// Cost model for self-hosted
type SelfHostedCosts struct {
    ComputePerVCPU  float64  // $/month per vCPU
    StoragePerGB    float64  // $/month per GB
    NetworkPerGB    float64  // $/GB transferred
}

func (c *CostAttribution) CalculateServiceCost(service string, costs SelfHostedCosts) float64 {
    // Estimate based on usage
    storageCost := float64(c.StorageBytes[service]) / 1e9 * costs.StoragePerGB
    computeCost := estimateCPUUsage(service) * costs.ComputePerVCPU
    return storageCost + computeCost
}
```

**Priority:** P3 - Nice for large orgs, not essential initially.

---

### Build vs Buy vs Integrate

For each gap, decide:

| Feature | Build | Buy/SaaS | Integrate OSS |
|---------|-------|----------|---------------|
| HA/Clustering | Build | - | - |
| Backup | Build | - | - |
| OTLP Receiver | Build | - | Use otel-collector |
| PromQL | Integrate | - | prometheus/promql lib |
| Continuous Profiling | Integrate | - | Pyroscope/Parca |
| Synthetic Monitoring | Skip | Checkly | - |
| Error Tracking | Build (basic) | Sentry | - |
| On-Call | Skip | PagerDuty | - |
| Service Catalog | Build (basic) | - | - |
| Log Parsing | Build | - | Vector |
| Alerting | Build | - | Alertmanager patterns |

**The 80/20:**
- Build core differentiators (eBPF tracing, cost intelligence)
- Integrate commodity features (PromQL, profiling)
- Skip/partner for unrelated features (on-call, RUM)

---

### UI/UX Gaps

#### 1. Trace Visualization

**Current:** Probably basic list view.

**What production tools have:**

```
Waterfall View (Jaeger-style):
┌─────────────────────────────────────────────────────────────┐
│ api-gateway         ████████████████████████████  247ms     │
│  └─ auth-service      ████████  45ms                        │
│  └─ user-service           ██████████████  89ms             │
│      └─ postgres              ████████  67ms                │
│  └─ payment-service                 ██████████████  112ms   │
│      └─ stripe-api                     ████████████  98ms   │
└─────────────────────────────────────────────────────────────┘

Flame Graph (for CPU profiles):
┌─────────────────────────────────────────────────────────────┐
│                        main                                  │
├───────────────────────────────┬─────────────────────────────┤
│        handleRequest          │      backgroundJob          │
├───────────────┬───────────────┤                             │
│  parseJSON    │  queryDB      │                             │
│               ├───────┬───────┤                             │
│               │ encode│ query │                             │
└───────────────┴───────┴───────┴─────────────────────────────┘

Service Map (topology):
    ┌─────────┐
    │   web   │
    └────┬────┘
         │
    ┌────▼────┐
    │   api   │
    └────┬────┘
    ┌────┴────┬────────┐
    │         │        │
┌───▼───┐ ┌───▼───┐ ┌──▼──┐
│ users │ │orders │ │cache│
└───┬───┘ └───┬───┘ └─────┘
    │         │
┌───▼─────────▼───┐
│    postgres     │
└─────────────────┘
```

#### 2. Log Viewer

**Current:** Probably plain text list.

**What production tools have:**
- Syntax highlighting for JSON/structured logs
- Expand/collapse nested objects
- Field extraction and filtering
- Surrounding context (show logs before/after)
- Live tail with pause
- Histogram of log volume over time
- Saved searches

```
┌─────────────────────────────────────────────────────────────┐
│ [Search: level:error service:api] [Last 1h ▼] [Live ●]     │
├─────────────────────────────────────────────────────────────┤
│ ▃▃▅▇▇▅▃▂▁▁▂▃▅▇█▇▅▃▂▁  (log volume histogram)              │
├─────────────────────────────────────────────────────────────┤
│ 14:32:07 ERROR api [+] Request failed                      │
│   ├─ trace_id: abc123 (click to view trace)                │
│   ├─ user_id: user_456                                     │
│   ├─ error: "connection refused"                           │
│   └─ stack_trace: [expand]                                 │
│                                                             │
│ 14:32:05 ERROR api [+] Database timeout                    │
│   ├─ trace_id: def789                                      │
│   └─ query: "SELECT * FROM users..."                       │
└─────────────────────────────────────────────────────────────┘
```

#### 3. Dashboard Builder

**Current:** Probably hardcoded or basic.

**What production tools have:**
- Drag-and-drop panel arrangement
- Multiple visualization types (line, bar, gauge, table, heatmap, stat)
- Panel templates
- Variables (dropdown filters that update all panels)
- Time range sync across panels
- Annotations overlay
- Dashboard versioning
- Export/import (JSON, Terraform)

#### 4. Alerting UI

**Current:** Probably YAML/config only.

**What production tools have:**
- Visual alert rule builder
- Preview: "This would have fired 3 times in the last week"
- Test notification button
- Alert history and state timeline
- Silence management UI
- Escalation policy builder

#### 5. Mobile / Responsive

**Current:** Probably desktop only.

**What on-call engineers need:**
- Mobile-friendly alert notifications
- Quick acknowledge/resolve from phone
- Basic dashboards on mobile
- Push notifications

---

### Notification & Alerting Gaps

#### 1. Alert Channels

**Minimum channels to support:**

| Channel | Priority | Notes |
|---------|----------|-------|
| Webhook | P0 | Generic, enables anything |
| Slack | P0 | Most common for startups |
| Email | P0 | Universal fallback |
| PagerDuty | P1 | On-call integration |
| OpsGenie | P1 | Atlassian ecosystem |
| Microsoft Teams | P1 | Enterprise |
| Discord | P2 | Dev communities |
| Telegram | P2 | International teams |
| SMS (Twilio) | P2 | Critical alerts |

**Implementation:**
```go
type AlertChannel interface {
    Send(alert Alert) error
    Test() error
}

type SlackChannel struct {
    WebhookURL string
    Channel    string
    Username   string
}

func (s *SlackChannel) Send(alert Alert) error {
    payload := SlackMessage{
        Channel: s.Channel,
        Attachments: []Attachment{{
            Color:  severityColor(alert.Severity),
            Title:  alert.Name,
            Text:   alert.Message,
            Fields: alertFields(alert),
        }},
    }
    return httpPost(s.WebhookURL, payload)
}
```

#### 2. Alert Routing

**Current:** Probably all alerts go to same place.

**What production tools have:**
```yaml
routing:
  # Route by severity
  - match:
      severity: critical
    receiver: pagerduty-oncall
    continue: true  # Also send to Slack

  # Route by team
  - match:
      team: payments
    receiver: slack-payments

  # Route by service
  - match:
      service: api
    receiver: slack-api-alerts

  # Default
  - receiver: slack-general
```

#### 3. Alert Templates

**Current:** Probably hardcoded messages.

**What production tools have:**
```yaml
templates:
  slack:
    title: "{{ .Alert.Name }} - {{ .Alert.Severity | upper }}"
    body: |
      *Service:* {{ .Alert.Labels.service }}
      *Environment:* {{ .Alert.Labels.env }}
      *Value:* {{ .Alert.Value | printf "%.2f" }}
      *Since:* {{ .Alert.StartsAt | since }}

      {{ .Alert.Annotations.description }}

      <{{ .DashboardURL }}|View Dashboard> | <{{ .SilenceURL }}|Silence>
```

#### 4. Notification Digest

**Current:** Every alert = separate notification.

**What production tools have:**
```
Instead of 50 separate Slack messages:

┌─────────────────────────────────────────────────────────────┐
│ 🔴 Alert Summary (last 15 minutes)                          │
├─────────────────────────────────────────────────────────────┤
│ CRITICAL (3)                                                │
│ • APIHighLatency - api-service (firing for 12m)            │
│ • DatabaseConnectionsFull - postgres (firing for 8m)       │
│ • PaymentFailureRate - payments (firing for 5m)            │
│                                                             │
│ WARNING (7)                                                 │
│ • DiskSpaceWarning - 3 hosts                               │
│ • MemoryPressure - 4 hosts                                 │
│                                                             │
│ [View All] [Acknowledge All Critical]                       │
└─────────────────────────────────────────────────────────────┘
```

---

### API Design Gaps

#### 1. REST API Structure

**Current:** Probably ad-hoc endpoints.

**What production APIs have:**
```
/api/v1/
├── /traces
│   ├── GET    /traces                    # List/search traces
│   ├── GET    /traces/{traceId}          # Get single trace
│   └── GET    /traces/{traceId}/spans    # Get spans for trace
│
├── /metrics
│   ├── GET    /metrics/query             # PromQL query
│   ├── GET    /metrics/query_range       # PromQL range query
│   ├── GET    /metrics/labels            # List label names
│   ├── GET    /metrics/label/{name}/values  # List label values
│   └── GET    /metrics/series            # List series
│
├── /logs
│   ├── GET    /logs/query                # LogQL query
│   ├── GET    /logs/tail                 # WebSocket live tail
│   └── GET    /logs/labels               # List log labels
│
├── /alerts
│   ├── GET    /alerts                    # List alerts
│   ├── POST   /alerts                    # Create alert rule
│   ├── GET    /alerts/{id}               # Get alert rule
│   ├── PUT    /alerts/{id}               # Update alert rule
│   ├── DELETE /alerts/{id}               # Delete alert rule
│   └── GET    /alerts/{id}/history       # Alert state history
│
├── /dashboards
│   ├── GET    /dashboards                # List dashboards
│   ├── POST   /dashboards                # Create dashboard
│   ├── GET    /dashboards/{id}           # Get dashboard
│   ├── PUT    /dashboards/{id}           # Update dashboard
│   └── DELETE /dashboards/{id}           # Delete dashboard
│
└── /admin
    ├── GET    /admin/health              # Health check
    ├── GET    /admin/ready               # Readiness check
    └── GET    /admin/metrics             # Internal metrics
```

#### 2. API Versioning

**Current:** Probably unversioned.

**What production APIs have:**
```
/api/v1/traces  # Stable
/api/v2/traces  # New version with breaking changes

Headers:
Accept: application/vnd.dogwatch.v1+json
```

**Deprecation policy:**
```
v1 deprecated: 2024-06-01
v1 removed: 2025-01-01
Migration guide: /docs/migrating-v1-to-v2
```

#### 3. Pagination

**Current:** Probably returns everything or has hardcoded limits.

**What production APIs have:**
```json
GET /api/v1/traces?limit=100&cursor=eyJsYXN0X2lkIjoiYWJjMTIzIn0=

Response:
{
  "data": [...],
  "pagination": {
    "cursor": "eyJsYXN0X2lkIjoiZGVmNDU2In0=",
    "has_more": true,
    "total_count": 12847
  }
}
```

#### 4. Bulk Operations

**Current:** Probably single-item operations only.

**What production APIs need:**
```json
POST /api/v1/alerts/bulk
{
  "operation": "silence",
  "ids": ["alert-1", "alert-2", "alert-3"],
  "duration": "4h",
  "reason": "Planned maintenance"
}
```

#### 5. GraphQL (Optional)

**Why some tools add GraphQL:**
```graphql
# Single query gets everything needed for a page
query TraceDetail($traceId: ID!) {
  trace(id: $traceId) {
    id
    duration
    spans {
      id
      name
      duration
      service { name, team }
      logs { timestamp, message }
    }
    relatedAlerts {
      name
      severity
    }
  }
}
```

REST would need 4+ requests for the same data.

---

### Data Lifecycle Gaps

#### 1. Retention Policies

**Current:** Probably keeps everything forever or single global retention.

**What production tools have:**
```yaml
retention:
  # Per-signal defaults
  metrics:
    raw: 15d        # Full resolution
    5m: 90d         # Downsampled
    1h: 1y          # Long-term
  traces: 7d
  logs: 30d

  # Per-service overrides
  overrides:
    - match:
        service: payment
      logs: 90d        # Compliance requirement
    - match:
        environment: dev
      traces: 1d       # Save space
      logs: 3d
```

#### 2. Data Deletion (GDPR)

**Current:** Probably can't delete specific user's data.

**What GDPR requires:**
```bash
# Delete all data for a specific user
dogwatch delete --user-id "user_12345" --confirm

# Audit what would be deleted
dogwatch delete --user-id "user_12345" --dry-run
```

**Implementation:**
```go
func (s *Storage) DeleteUserData(userID string) error {
    // Find all traces with this user_id
    traces := s.QueryTraces(fmt.Sprintf("user_id = '%s'", userID))
    for _, t := range traces {
        s.DeleteTrace(t.ID)
    }

    // Find all logs with this user_id
    logs := s.QueryLogs(fmt.Sprintf("user_id = '%s'", userID))
    for _, l := range logs {
        s.DeleteLog(l.ID)
    }

    // Audit log the deletion (required for compliance)
    s.AuditLog("user_data_deleted", map[string]any{
        "user_id": userID,
        "traces_deleted": len(traces),
        "logs_deleted": len(logs),
        "requested_by": currentUser(),
    })

    return nil
}
```

#### 3. Data Export for Portability

**Current:** Probably no export.

**What regulations may require:**
```bash
# Export all data for a user (GDPR data portability)
dogwatch export --user-id "user_12345" --format json > user_data.json

# Export for migration to another system
dogwatch export --start 2024-01-01 --end 2024-01-31 --format otlp > january.otlp
```

---

### Compliance Gaps

#### 1. Audit Logging (Already Covered, But Critical)

Every compliance framework requires:
- Who did what, when
- Immutable audit trail
- Retention of audit logs (often 1+ year)

#### 2. Data Residency

**Current:** Data stored wherever dogwatch runs.

**What enterprises need:**
```yaml
# Ensure data stays in specific region
data_residency:
  region: eu-west-1
  enforce: true

# Some data may have different requirements
overrides:
  - match:
      data_class: pii
    region: eu-only
```

#### 3. Encryption

| What | Requirement | Status |
|------|-------------|--------|
| Data at rest | AES-256 | ❌ Missing |
| Data in transit | TLS 1.2+ | Probably ✅ |
| Backups | Encrypted | ❌ Missing |
| Encryption keys | Rotatable | ❌ Missing |

#### 4. Compliance Certifications

| Certification | What It Means | Effort |
|--------------|---------------|--------|
| SOC 2 Type I | Security controls exist | ~$50K, 3-6 months |
| SOC 2 Type II | Controls work over time | ~$80K, 6-12 months |
| ISO 27001 | Info security management | ~$100K, 12+ months |
| HIPAA | Healthcare data | BAA + controls |
| GDPR | EU data protection | Data handling + DPA |
| FedRAMP | US government | Very expensive, long |

**For a startup:** Skip certifications initially. Add SOC 2 when enterprise customers require it.

---

### Documentation Gaps

#### 1. User Documentation

**Current:** Probably minimal README.

**What production tools have:**
```
docs/
├── getting-started/
│   ├── quick-start.md         # 5-minute setup
│   ├── installation.md        # Detailed installation
│   └── first-dashboard.md     # Create first dashboard
│
├── concepts/
│   ├── traces.md
│   ├── metrics.md
│   ├── logs.md
│   └── alerting.md
│
├── guides/
│   ├── kubernetes.md          # K8s deployment
│   ├── docker.md
│   ├── scaling.md
│   ├── high-availability.md
│   └── troubleshooting.md
│
├── reference/
│   ├── api.md                 # Full API reference
│   ├── configuration.md       # All config options
│   ├── promql.md             # Query language
│   └── cli.md                # CLI reference
│
└── migration/
    ├── from-datadog.md
    ├── from-grafana.md
    └── from-prometheus.md
```

#### 2. API Documentation

**Current:** Probably undocumented.

**What production tools have:**
- OpenAPI/Swagger spec
- Interactive API explorer (try requests in browser)
- Code examples in multiple languages
- SDKs with documentation

#### 3. Runbook for dogwatch Itself

**Current:** Probably none.

**What ops teams need:**
```markdown
# dogwatch Operations Runbook

## High CPU Usage
1. Check `dogwatch_query_duration_seconds` for slow queries
2. Check `dogwatch_ingest_rate` for ingest spikes
3. Scale up query nodes if needed

## High Memory Usage
1. Check `dogwatch_storage_cache_size_bytes`
2. Reduce cache size in config
3. Check for cardinality explosion

## Data Loss / Gaps
1. Check `dogwatch_events_dropped_total`
2. Check disk space
3. Check backpressure metrics

## Recovery from Crash
1. Check WAL for uncommitted data
2. Run `dogwatch recover`
3. Verify data integrity
```

---

### Extensibility Gaps

#### 1. Plugin System

**Current:** Probably hardcoded functionality.

**What production tools have:**
```
plugins/
├── inputs/            # Data sources
│   ├── cloudwatch/
│   ├── stackdriver/
│   └── custom-api/
├── processors/        # Data transformation
│   ├── filter/
│   ├── aggregate/
│   └── enrich/
├── outputs/           # Destinations
│   ├── s3/
│   ├── kafka/
│   └── webhook/
└── panels/            # Dashboard visualizations
    ├── heatmap/
    ├── geomap/
    └── custom-chart/
```

**Plugin interface:**
```go
type InputPlugin interface {
    Name() string
    Collect() ([]Metric, error)
    Configure(config map[string]any) error
}

type ProcessorPlugin interface {
    Name() string
    Process(event Event) (Event, error)
}
```

#### 2. Webhooks (Inbound)

**Current:** Probably no way to receive arbitrary data.

**What production tools have:**
```
POST /api/v1/webhooks/ingest
{
  "source": "custom-app",
  "events": [
    {"type": "deployment", "service": "api", "version": "2.4.1"},
    {"type": "incident", "title": "Database slow", "severity": "high"}
  ]
}
```

This enables:
- Custom change tracking
- External incident creation
- Integration with any tool that can POST JSON

---

### Performance Optimization Gaps

#### 1. Lazy Loading

**Current:** Probably loads everything upfront.

**What production UIs have:**
- Load dashboard metadata first, panels on scroll
- Load trace summary first, spans on expand
- Infinite scroll for logs

#### 2. Streaming Responses

**Current:** Probably waits for full result before sending.

**What production tools have:**
```go
// Stream results as they're computed
func (h *QueryHandler) StreamQuery(w http.ResponseWriter, r *http.Request) {
    flusher := w.(http.Flusher)

    results := h.engine.StreamQuery(query)
    for result := range results {
        json.NewEncoder(w).Encode(result)
        flusher.Flush()  // Send immediately
    }
}
```

#### 3. Compression

| What | Compression | Savings |
|------|-------------|---------|
| API responses | gzip/brotli | 70-90% |
| Storage (metrics) | Gorilla/XOR | 90%+ |
| Storage (logs) | zstd/lz4 | 70-80% |
| Storage (traces) | zstd | 60-70% |

**Current:** Probably uncompressed.

---

### What's Actually Critical vs Nice-to-Have

**Critical (blocks production use):**
1. Health endpoints (/healthz, /readyz)
2. Graceful shutdown
3. OTLP receiver
4. Basic retention policies
5. Alert channels (webhook, Slack)
6. Basic trace visualization (waterfall)
7. Basic log viewer (search, filter)

**Important (blocks serious adoption):**
1. Dashboard variables
2. API versioning
3. Pagination
4. Audit logging
5. Backup/restore
6. Alert routing
7. Documentation

**Nice-to-have (growth features):**
1. Plugin system
2. GraphQL API
3. Mobile UI
4. Compliance certifications
5. Multi-tenancy
6. Digest notifications

#### What This Unlocks

With proper protocol support:

```
┌──────────────────────────────────────────────────────────────────┐
│                     dogwatch as universal receiver                │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  OTel SDK ──────┐                                                │
│  (any language) │                                                │
│                 │     ┌─────────────────┐                        │
│  Prometheus ────┼────▶│    dogwatch     │                        │
│  (metrics)      │     │                 │                        │
│                 │     │  OTLP:4317/4318 │                        │
│  FluentBit ─────┼────▶│  Prom:9090      │                        │
│  (logs)         │     │  Fluent:24224   │                        │
│                 │     │  StatsD:8125    │                        │
│  Jaeger agent ──┘     │  Jaeger:14268   │                        │
│                       └─────────────────┘                        │
│                                                                   │
│  Plus: eBPF captures everything else automatically               │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
```

**The pitch becomes:**
"dogwatch auto-captures everything via eBPF. AND it speaks every protocol, so you can send custom telemetry too. One binary, all your data."

---

### Correlation & Linking

#### 1. Exemplars (Metrics → Traces)

**Current:** Metrics and traces are separate. Can't jump from a metric spike to the specific traces.

**What production systems have:**
```
Graph showing latency spike at 14:32
    ↓ Click on spike
Shows: "Exemplar traces during this period"
    → trace_id: abc123 (2847ms)
    → trace_id: def456 (3102ms)
```

**Implementation:**
```go
type Metric struct {
    Name      string
    Value     float64
    Timestamp time.Time
    Labels    map[string]string
    Exemplar  *Exemplar  // Link to a representative trace
}

type Exemplar struct {
    TraceID   string
    SpanID    string
    Value     float64
    Timestamp time.Time
}
```

#### 2. Logs in Context

**Current:** Logs and traces are separate tables. Manual correlation.

**What production systems have:**
- Every log automatically gets trace_id if in request context
- Trace view shows associated logs inline
- Click log → see surrounding logs + trace

**eBPF approach:**
```c
// When we see a log write (write() to stdout/file),
// capture the current trace context from thread-local storage
struct log_event {
    u64 timestamp;
    u32 pid;
    u32 tid;
    char trace_id[32];  // From TLS or thread-local trace context
    char message[256];
};
```

#### 3. Infrastructure Correlation

**Current:** App-level observability. Don't know what host/container/pod things run on.

**What production systems have:**
```
Service: api-gateway
├── Pod: api-gateway-7d4f8b-abc12
│   ├── Container: api-gateway
│   ├── Node: worker-3
│   ├── CPU: 45%
│   ├── Memory: 1.2GB
│   └── Network: 150MB/s
└── Related:
    ├── 3 other pods of same deployment
    ├── Node metrics for worker-3
    └── K8s events (OOM, restarts)
```

**Resource detection:**
```go
type ResourceDetector interface {
    Detect() Resource
}

// Detect where we're running
type Resource struct {
    // Cloud
    CloudProvider  string  // aws, gcp, azure
    CloudRegion    string
    CloudZone      string
    CloudAccountID string

    // Kubernetes
    K8sCluster     string
    K8sNamespace   string
    K8sPod         string
    K8sContainer   string
    K8sNode        string
    K8sDeployment  string

    // Host
    HostName       string
    HostID         string
    OSType         string
    OSVersion      string
}
```

---

### Query & Visualization Gaps

#### 1. Dashboard Variables

**Current:** Static dashboards. Change service = edit dashboard.

**What production systems have:**
```
┌─────────────────────────────────────────────────────┐
│  Service: [api-gateway ▼]  Environment: [prod ▼]   │
├─────────────────────────────────────────────────────┤
│  [Dashboard adapts to selected values]              │
└─────────────────────────────────────────────────────┘
```

**Implementation:**
```yaml
dashboard:
  variables:
    - name: service
      type: query
      query: "label_values(service)"
    - name: environment
      type: custom
      values: [prod, staging, dev]

  panels:
    - title: "Latency for $service"
      query: "histogram_quantile(0.99, rate(http_duration{service='$service'}[5m]))"
```

#### 2. Annotations

**Current:** No way to mark events on graphs.

**What production systems have:**
- Vertical lines showing deploys, incidents, config changes
- Hover to see details
- Link to related data

**API:**
```
POST /api/annotations
{
  "time": 1705123456000,
  "text": "Deployed v2.4.1",
  "tags": ["deploy", "api-service"],
  "url": "https://github.com/org/repo/commit/abc123"
}
```

#### 3. Saved Queries & Query Library

**Current:** Write query from scratch every time.

**What production systems have:**
- Save queries with names
- Share queries with team
- Query history
- Suggested queries based on data

#### 4. Scheduled Reports

**Current:** Real-time only. No scheduled reports.

**What production systems have:**
- Daily/weekly email reports
- PDF export of dashboards
- Slack summary posts

---

### Operational Gaps

#### 1. Graceful Shutdown

**Current:** Kill signal = immediate stop. In-flight requests dropped.

**What production systems have:**
```go
func (s *Server) Shutdown(ctx context.Context) error {
    // 1. Stop accepting new connections
    s.listener.Close()

    // 2. Wait for in-flight requests (with timeout)
    s.wg.Wait()

    // 3. Flush buffers to storage
    s.storage.Flush()

    // 4. Close storage connections
    s.storage.Close()

    return nil
}
```

**K8s integration:**
```yaml
lifecycle:
  preStop:
    exec:
      command: ["/bin/sh", "-c", "sleep 5"]  # Let LB drain
terminationGracePeriodSeconds: 30
```

#### 2. Health Endpoints

**Current:** Unknown. Probably none.

**What K8s expects:**
```
GET /healthz        → Am I alive? (liveness)
GET /readyz         → Am I ready to serve? (readiness)
GET /metrics        → Prometheus metrics about myself
```

**Implementation:**
```go
func (s *Server) healthzHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}

func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
    // Check dependencies
    if err := s.storage.Ping(); err != nil {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    if s.ebpf.BufferUsage() > 0.9 {
        w.WriteHeader(http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
}
```

#### 3. Configuration Validation

**Current:** Bad config = crash at startup (or worse, silent misbehavior).

**What production systems have:**
```bash
# Validate before applying
dogwatch config validate --config dogwatch.yaml
# OK: Configuration is valid

# Diff against running config
dogwatch config diff --config dogwatch.yaml
# - retention: 7d
# + retention: 30d
```

#### 4. Database Migrations

**Current:** Unknown. Schema changes probably break things.

**What production systems have:**
```
migrations/
├── 001_initial_schema.sql
├── 002_add_trace_sampling.sql
├── 003_add_audit_log.sql
└── 004_add_slo_tables.sql
```

```bash
# Apply pending migrations
dogwatch migrate up

# Rollback last migration
dogwatch migrate down

# Show migration status
dogwatch migrate status
```

---

### Security Gaps

#### 1. Encryption at Rest

**Current:** SQLite files are plaintext. Disk access = data access.

**What production systems have:**
- SQLCipher for encrypted SQLite
- Or filesystem-level encryption
- Key rotation capability

#### 2. Secret Management

**Current:** Secrets in config file or env vars.

**What production systems have:**
- HashiCorp Vault integration
- AWS Secrets Manager
- K8s secrets with rotation
- No secrets in config files

**Minimum viable:**
```yaml
secrets:
  provider: vault  # or: env, file, aws-secrets-manager
  vault:
    address: https://vault.internal:8200
    path: secret/dogwatch
```

#### 3. mTLS Between Components

**Current:** Single binary, so N/A. But if we add workers/collectors...

**What production systems have:**
- All internal communication over mTLS
- Certificate rotation
- Service mesh integration (Istio sidecar)

#### 4. API Authentication Options

**Current:** Basic auth, API keys, maybe JWT.

**What production systems have:**
- API keys with scopes
- OAuth2/OIDC
- Service accounts for automation
- Short-lived tokens
- IP allowlisting

---

### Developer Experience Gaps

#### 1. CLI Tool

**Current:** Binary is server only.

**What production systems have:**
```bash
# Query from command line
dogwatch query 'rate(http_requests_total[5m])'

# Tail logs
dogwatch logs -f --service api

# Export data
dogwatch export --start 2024-01-01 --format json

# Check status
dogwatch status
```

#### 2. Local Development Mode

**Current:** Same config for dev and prod.

**What production systems have:**
```bash
# Start with sample data, lower resources
dogwatch --dev

# Includes:
# - Sample traces/logs/metrics
# - Lower memory limits
# - Debug logging
# - Hot reload of UI
```

#### 3. API Documentation

**Current:** Probably undocumented.

**What production systems have:**
- OpenAPI/Swagger spec
- Interactive API explorer
- Code examples in multiple languages
- Postman collection

---

### What You Should Actually Build Next

Given all these gaps, here's the prioritized order:

**This Week:**
1. Health endpoints (`/healthz`, `/readyz`)
2. Graceful shutdown
3. Config validation command

**Next 2 Weeks:**
4. OTLP receiver (gRPC + HTTP)
5. Basic WAL for durability
6. Database migration system

**Next Month:**
7. Dashboard variables
8. Exemplars (metrics → traces linking)
9. Logs in context (trace_id on every log)
10. CLI query tool

These are the "your users will hit these immediately" gaps that make dogwatch feel unproduction-ready.

---

## Realistic Path Forward

You're not going to out-build Datadog (4000+ engineers) or Grafana Labs (1000+ engineers). The goal isn't feature parity. It's **being better for a specific use case**.

### What dogwatch CAN Win On

| Advantage | Why They Can't Copy |
|-----------|---------------------|
| **Single binary simplicity** | Grafana has 5+ components. Datadog is SaaS. Architectural decisions. |
| **Zero-config eBPF tracing** | Requires kernel expertise. Most vendors bolt on APM as afterthought. |
| **Self-hosted, data-sovereign** | Datadog business model requires your data. Can't change. |
| **No per-host pricing** | Datadog revenue model. Would cannibalize themselves. |
| **Cost intelligence (calling them out)** | They won't build "look how expensive we are" features. |

### What to Build (Do This)

**Phase 1: Nail the Core Differentiator**
1. Zero-config tracing that actually works (MySQL, PG, Redis, HTTP)
2. Cost calculator that shows Datadog equivalent
3. Single command install, see traces in 60 seconds

**Phase 2: Production Ready**
4. Backup/restore (simple: `dogwatch backup`, `dogwatch restore`)
5. Basic HA (active-passive with shared storage)
6. Sampling (head sampling with priority rules)
7. Self-monitoring (internal metrics dashboard)

**Phase 3: Intelligence**
8. BubbleUp (automatic root cause)
9. Change correlation (deploy → incident)
10. Control plane (cardinality analysis)

**Phase 4: Migration**
11. Datadog/Grafana dashboard importer
12. Query translator
13. Cost savings report

### What to Skip (Don't Build)

| Feature | Why Skip | Alternative |
|---------|----------|-------------|
| **800+ integrations** | Years of work, not differentiating | Focus on auto-discovery via eBPF |
| **On-call scheduling** | PagerDuty does it better | Integrate, don't rebuild |
| **Session replay / RUM** | Different product entirely | Integrate with Sentry/LogRocket |
| **Continuous profiler** | Complex, separate value prop | Future maybe, not core |
| **SOC2 / HIPAA** | Legal/audit work, not code | Wait until enterprise demands it |
| **Distributed query engine** | Massive engineering effort | SQLite is fine until 10M series |
| **ML anomaly detection** | Overpromised, underdelivers | Simple statistical methods first |

### The Honest Assessment

**You are leagues behind on:**
- Scale (they handle petabytes, you handle gigabytes)
- Polish (1000s of UX iterations)
- Integrations (800 vs 0)
- Enterprise features (SAML, audit, compliance)

**But you're ahead on:**
- Simplicity (single binary vs 5+ components)
- Zero-config (eBPF vs "add SDK to every service")
- Cost (free vs $40K+/year)
- Speed (install in 60 seconds vs days of setup)

**The strategy:**
Don't try to be Datadog. Be the Datadog killer for teams who:
- Don't have $40K/year for observability
- Don't want to instrument every service
- Don't want to send data to someone else's cloud
- Value simplicity over 800 features they'll never use

---

## Success Metrics

### Product Metrics

| Metric | Target | Why |
|--------|--------|-----|
| Time to first trace | < 60 seconds | Zero-config promise |
| Services auto-discovered | 100% | No manual registration |
| Protocols parsed | 5+ (HTTP, MySQL, PG, Redis, gRPC) | Comprehensive visibility |
| Cost accuracy | ±10% vs actual Datadog | Credible comparison |

### Business Metrics

| Metric | Target | Why |
|--------|--------|-----|
| GitHub stars | 5,000 in year 1 | Community validation |
| Self-hosted installs | 1,000 | Usage traction |
| Paid customers | 50 at $100+/month | Revenue validation |
| MRR | $5,000 | Sustainable business |

---

## The North Star

A developer installs dogwatch with one command. Within 60 seconds:

1. **Sees every service** - auto-discovered from network traffic
2. **Sees every request** - traced across services automatically
3. **Sees every query** - database calls captured without instrumentation
4. **Sees the cost** - "$X/month on Datadog, $0 with dogwatch"
5. **Sees the waste** - "84% of your metrics are never queried"

When something breaks:

6. **Knows why** - "98% of slow requests hit db_shard=7"
7. **Knows what changed** - "Incident started 4 min after deploy v2.4.2"
8. **Knows if it's malicious** - "Shell spawned in container that never runs shells"

When switching from Datadog:

9. **Knows what migrates** - "23 dashboards, 47 alerts compatible"
10. **Knows the savings** - "You'll save $564,000/year"

No SDKs. No configuration. No bill shock. No hunting for root cause. No switching cost.

**That's the product.**

---

## Cutting-Edge Research & Future Technologies

This section tracks breakthrough technologies and academic research that could provide significant competitive advantage. Updated January 2025.

### eBPF-Level Breakthroughs (2025-2026)

These are paradigm-shifting technologies at the same level of impact as eBPF itself.

#### 1. eGPU: eBPF on GPUs

**Status:** Working code, HCDS '25 paper published

The most significant breakthrough for AI workload observability. Extends eBPF's programmability directly into GPU kernels.

```
BEFORE:                          AFTER:
CPU ████ (eBPF)                  CPU ████ (eBPF)
GPU ░░░░ (blind)                 GPU ████ (eBPF via PTX injection)
                                      └── Same toolchain, unified traces
```

**How it works:**
- Converts eBPF bytecode to PTX (NVIDIA GPU assembly)
- Injects into running GPU kernels dynamically
- Same eBPF toolchain for CPU AND GPU
- Works on NVIDIA and AMD GPUs

**Key resources:**
- [bpftime GitHub](https://github.com/eunomia-bpf/bpftime) - Working implementation
- [eGPU Paper (HCDS '25)](https://dl.acm.org/doi/10.1145/3723851.3726984)
- [GPU Observability Gap Analysis](https://eunomia.dev/blog/2025/10/14/the-gpu-observability-gap-why-we-need-ebpf-on-gpu-devices/)

**dogwatch opportunity:** First single-binary observability tool with unified CPU+GPU visibility. Critical for AI/ML workloads.

#### 2. sched_ext: Programmable CPU Scheduler

**Status:** In mainline Linux 6.12, shipping in production

Load custom BPF schedulers at runtime without kernel recompilation.

```
Traditional: Fixed scheduler algorithms (CFS, EEVDF)
sched_ext:   Load custom BPF scheduler → instant deployment
             ├── Gaming-optimized (LAVD on Steam Deck)
             ├── GPU-aware scheduling (2026 roadmap)
             └── Workload-specific optimization
```

**Key resources:**
- [sched_ext Future Plans](https://www.phoronix.com/news/sched-ext-future-plans-2026)
- [LAVD Gaming Scheduler](https://lwn.net/Articles/1051430/)
- [CachyOS sched_ext Guide](https://wiki.cachyos.org/configuration/sched-ext/)

**dogwatch opportunity:** Not just observe scheduling—CONTROL it. "Your latency-sensitive service gets dedicated cores."

#### 3. kTLS + eBPF: HTTPS Without Uprobes

**Status:** Maturing, growing adoption

Solves the HTTPS visibility problem elegantly without fragile uprobe hacks.

```
Current (broken):     App → OpenSSL (userspace) → Encrypted
                                   ↑
                              uprobe here (FRAGILE)

kTLS solution:        App → kTLS (kernel) → Encrypted
                                 ↑
                            eBPF hook here (STABLE)
                            See plaintext BEFORE encryption
```

**Key insight:** Many apps (nginx, HAProxy) support kTLS. Hook kernel TLS layer instead of userspace libraries.

**Key resources:**
- [kTLS + SPIFFE Research](https://blog.riptides.io/seamless-kernel-based-non-human-identity-with-ktls-and-spiffe/)
- [eBPF Ecosystem 2024-2025](https://eunomia.dev/blog/2025/02/12/ebpf-ecosystem-progress-in-20242025-a-technical-deep-dive/)

**dogwatch opportunity:** Fix the HTTPS problem permanently without maintaining uprobe compatibility matrices.

#### 4. Continuous GPU Profiling

**Status:** Released October 2025 by Polar Signals

First open-source NVIDIA CUDA profiler suitable for always-on production use.

```
Traditional GPU profiling: Manual, high overhead, dev-only
Continuous GPU profiling:  Always-on, <1% overhead, production
                           └── GPU flames alongside CPU flames
                           └── Correlate GPU kernels to requests
```

**Key resources:**
- [Polar Signals GPU Profiling](https://www.polarsignals.com/blog/posts/2025/10/22/gpu-profiling)
- [Parca Agent v0.43.0](https://github.com/parca-dev/parca)

**dogwatch opportunity:** Integrate continuous GPU profiling. Show GPU bottlenecks in unified flame graphs.

#### 5. eBPF for Windows

**Status:** Active development by Microsoft

Cross-platform eBPF enabling write-once observability.

```
Write once, run on:
├── Linux (native eBPF)
├── Windows 11/Server 2022+ (eBPF for Windows)
└── Same bytecode, same tools
```

**Key resources:**
- [microsoft/ebpf-for-windows](https://github.com/microsoft/ebpf-for-windows)
- [Retina - Cross-platform K8s observability](https://github.com/microsoft/retina)

**dogwatch opportunity:** Single binary that works on Linux AND Windows. Massive market expansion.

---

### Graph Neural Networks for Trace Analysis

The hottest area in academic observability research. Replace rule-based anomaly detection with learned patterns.

#### Key Papers

| Paper | Venue | Innovation | Performance |
|-------|-------|------------|-------------|
| [GAL-MAD](https://arxiv.org/html/2504.00058v2) | arXiv Apr 2025 | Graph Attention + Explainability | Explains WHY anomalous |
| [GNN + Temporal](https://arxiv.org/abs/2511.03285) | arXiv Nov 2025 | GNN + GRU temporal | Captures time evolution |
| [GAMMA](https://dl.acm.org/doi/10.1145/3589334.3645665) | WWW 2024 | Multi-bottleneck | Finds multiple root causes |
| [CHASE](https://arxiv.org/html/2406.19711v1) | arXiv Jun 2024 | Heterogeneous graphs | 36.2% improvement A@1 |
| [Sleuth](https://dl.acm.org/doi/10.1145/3623278.3624758) | ASPLOS 2023 | Trace-based GNN at scale | Production-ready |

#### The Technique

```
Traditional:     Trace → Rules → Alert
                 (misses complex patterns)

GNN-based:       Trace → Service Graph → GNN embedding
                     → Learn normal patterns
                     → Detect deviations
                     → Localize root cause

Key insight: Services form a GRAPH. Use graph learning.
```

#### Implementation Approach

```python
# Conceptual architecture
class TraceGNN:
    def __init__(self):
        self.graph_encoder = GATConv(...)  # Graph attention
        self.temporal = GRU(...)            # Time dynamics
        self.anomaly_head = MLP(...)        # Detection
        self.localization_head = MLP(...)   # Root cause

    def forward(self, trace_graph, timestamps):
        # 1. Build service graph from trace spans
        # 2. Encode with graph attention (captures topology)
        # 3. Add temporal dynamics (captures evolution)
        # 4. Detect anomalies without explicit rules
        # 5. Localize root cause service
```

**dogwatch opportunity:** eBPF generates traces → Build dynamic service graph → GNN learns patterns → Auto-detect anomalies without rules.

---

### Causal Inference for Root Cause Analysis

Move beyond correlation ("A and B happened together") to causation ("A caused B").

#### Key Papers

| Paper | Venue | Innovation | Accuracy |
|-------|-------|------------|----------|
| [Neural Granger Causal](https://arxiv.org/abs/2402.01140) | AAAI 2024 | Temporal causal discovery | Respects time ordering |
| [CIRCA](https://dl.acm.org/doi/10.1145/3534678.3539041) | KDD 2022 | Intervention recognition | 25% better recall |
| [Multi-Modal Causal](https://www.sciencedirect.com/science/article/abs/pii/S0951832025007203) | 2025 | Logs + metrics + traces | Finds trigger values |
| [FaultInsight](https://arxiv.org/pdf/2509.12231) | arXiv 2025 | Temporal perturbation | 15-20% better than VAR |
| [Causely Analysis](https://www.infoq.com/articles/causal-reasoning-observability/) | InfoQ 2025 | Production system | 90% accuracy (Instana) |

#### The Difference

```
Correlation:  "CPU spike and latency spike happened together"
              (could be coincidence, could be effect not cause)

Causation:    "CPU spike CAUSED latency spike"
              "If we fix CPU, latency will improve with 94% confidence"
              "Counterfactual: if CPU hadn't spiked, latency = 50ms"
```

#### Methods

| Method | Best For | Limitation |
|--------|----------|------------|
| **Structural Causal Models** | Explicit dependencies | Requires domain knowledge |
| **Granger Causality** | Time series | Only linear relationships |
| **Bayesian Networks** | Uncertainty quantification | Computationally expensive |
| **Intervention Analysis** | "What-if" scenarios | Needs historical interventions |
| **Counterfactual Reasoning** | Explaining outcomes | Requires good causal model |

**dogwatch opportunity:** Don't just correlate metrics. Build causal graphs. Answer "what caused this?" not just "what happened?"

---

### Time Series Foundation Models

Pre-trained models that work across any metric without task-specific training.

#### Key Models

| Model | Architecture | Capabilities | Paper |
|-------|--------------|--------------|-------|
| **Chronos** | Tokenized TS | Zero-shot forecasting | [arXiv](https://arxiv.org/html/2504.04011v1) |
| **TimeGPT** | Transformer | Multi-horizon prediction | [arXiv](https://arxiv.org/html/2412.19286v1) |
| **MOMENT** | Masked modeling | Forecast + anomaly + classify | [arXiv](https://arxiv.org/html/2405.02358v3) |
| **UniTS** | Unified multitask | Single model, all tasks | [Survey](https://arxiv.org/abs/2504.04011) |
| **Timer** | Generative | Unified as generation task | [Survey](https://arxiv.org/html/2405.02358v3) |

#### The Breakthrough

```
Before:  Train model per metric, per task
         (thousands of models, expensive, slow)

After:   One foundation model
         ├── Zero-shot on new metrics (no training!)
         ├── Forecasting
         ├── Anomaly detection
         ├── Classification
         └── Imputation
         All from same model!
```

#### Survey Reference

[Foundation Models for Time Series: A Survey](https://arxiv.org/abs/2504.04011) (April 2025) - Comprehensive taxonomy of 50+ models.

**dogwatch opportunity:** Don't train anomaly detectors per metric. Use a time series foundation model for zero-shot anomaly detection on ANY metric.

---

### LLM-Powered AIOps

Large language models for incident investigation, alert correlation, and automated diagnosis.

#### Comprehensive Survey

[A Survey of AIOps in the Era of Large Language Models](https://arxiv.org/abs/2507.12472) (July 2025) analyzed 183 papers from 2020-2024.

#### Key Applications

| Application | LLM Capability | State of Art |
|-------------|----------------|--------------|
| **Alert aggregation** | NLP understanding | Reduce "alert storms" by grouping |
| **Incident analysis** | Semantic reasoning | Auto-diagnose + suggest fixes |
| **Log parsing** | Pattern recognition | No regex rules needed |
| **Root cause** | Multi-step reasoning | [OpenRCA](https://arxiv.org/abs/2503.00981) (ICLR 2025) |
| **Runbook generation** | Knowledge synthesis | Auto-create from incidents |

#### Key Papers

| Paper | Venue | Focus |
|-------|-------|-------|
| [OpenRCA](https://arxiv.org/abs/2503.00981) | ICLR 2025 | "Can LLMs Locate Root Cause?" |
| [AIOpsLab](https://arxiv.org/abs/2407.12165) | MLSys 2025 | Agent evaluation framework |
| [ITBench](https://arxiv.org/abs/2410.16890) | ICML 2025 | IT automation benchmark |
| [Flow-of-Action](https://netman.aiops.org/publications/) | Tsinghua 2025 | SOP-enhanced multi-agent RCA |

#### Integration Pattern

```python
# Not just "ask AI" but structured reasoning
def investigate_incident(incident, context):
    # 1. Gather relevant data (eBPF traces, metrics, logs)
    evidence = collect_evidence(incident.time_range)

    # 2. Build causal context
    causal_graph = build_causal_model(evidence)

    # 3. LLM reasons over structured data
    analysis = llm.analyze(
        incident=incident,
        evidence=evidence,
        causal_graph=causal_graph,
        runbooks=get_relevant_runbooks()
    )

    # 4. Return actionable diagnosis
    return {
        "root_cause": analysis.root_cause,
        "confidence": analysis.confidence,
        "remediation": analysis.suggested_fix,
        "evidence": analysis.supporting_traces
    }
```

**dogwatch opportunity:** LLMs for natural language incident investigation. "Why is checkout slow?" → actual answer with evidence, not dashboard links.

---

### Self-Healing Infrastructure

The end goal: fully autonomous detection, diagnosis, and remediation.

#### Key Papers

| Paper | Venue | Approach | Results |
|-------|-------|----------|---------|
| [IFSHM](https://arxiv.org/html/2506.07411v1) | arXiv Jun 2025 | LLM + Deep RL | Semantic understanding + policy optimization |
| [Cloud Self-Healing](https://arxiv.org/html/2505.11743v1) | arXiv May 2025 | LLM fault detection | Best accuracy, fast recovery |
| [Self-Healing ML](https://arxiv.org/abs/2411.00186) | arXiv Oct 2024 | Autonomous adaptation | Handles distribution shift |

#### The Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   SELF-HEALING LOOP                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. OBSERVE (eBPF + eGPU)                                   │
│     └── Real-time telemetry from CPU, GPU, network          │
│                                                              │
│  2. DETECT (Foundation Model + GNN)                         │
│     └── Anomaly detection without explicit rules            │
│                                                              │
│  3. DIAGNOSE (Causal AI + LLM)                              │
│     └── Root cause, not just symptoms                       │
│                                                              │
│  4. DECIDE (LLM + Reinforcement Learning)                   │
│     └── Select optimal remediation action                   │
│                                                              │
│  5. ACT (Agentic Execution)                                 │
│     └── Execute runbook, verify fix                         │
│                                                              │
│  6. LEARN (Continuous Improvement)                          │
│     └── Update models from outcomes                         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

#### Industry Adoption

- [Gartner: 60% of enterprises moving to self-healing by 2026](https://ennetix.com/the-rise-of-autonomous-it-operations-what-aiops-platforms-must-enable-by-2026/)
- [Results: 96% improvement in MTTD, 83% in MTTR](https://www.algomox.com/resources/blog/self_healing_infrastructure_with_agentic_ai/)

**dogwatch opportunity:** Full autonomy loop. See → Diagnose → Fix → Learn. Human notified after resolution.

---

### Kernel Safety: Rex and Safe Rust Extensions

Replace fragile eBPF verifier with compile-time safety guarantees.

#### Key Papers

| Paper | Venue | Innovation |
|-------|-------|------------|
| [Rex](https://arxiv.org/html/2502.18832v2) | arXiv Apr 2025 | Safe Rust kernel extensions |
| [SafeBPF](https://arxiv.org/pdf/2409.07508) | arXiv Sep 2024 | Hardware MTE for eBPF defense |
| [CapsLock](https://arxiv.org/pdf/2507.03344) | arXiv Jul 2025 | Hardware capabilities for Rust |
| [eBPF Runtime](https://arxiv.org/html/2410.00026v2) | arXiv Oct 2024 | Comprehensive runtime analysis |

#### The Shift

```
Current eBPF:     C code → Verifier (complex, limits)
                  └── Verifier bugs = kernel compromise
                  └── Verifier limits = can't express some programs

Future (Rex):     Rust code → Compiler guarantees
                  └── Memory safety by construction
                  └── No verifier limitations
                  └── More expressive programs
                  └── Type safety extends to kernel
```

**dogwatch opportunity:** Write probes in safe Rust. More powerful, safer, easier to maintain than C eBPF.

---

### Agent Observability (AgentSight)

As AI agents proliferate, need visibility into agent reasoning and actions.

#### Key Paper

[AgentSight](https://arxiv.org/html/2508.02736v1) (Aug 2025) - System-level observability for AI agents using eBPF.

```
Problem: AI agents make tool calls, API requests, file writes
         Traditional observability sees syscalls
         Missing: Agent INTENT and REASONING

AgentSight solution:
├── eBPF captures system events
├── "Boundary tracing" links LLM calls to effects
├── Hybrid correlation engine
├── Detects: prompt injection, reasoning loops, bottlenecks
└── <3% overhead
```

#### Trace Example

```
agent-request (user: "book a flight")
  └── llm.reason() (200ms, "I need to search flights")
      └── tool.search_flights() (API call, 500ms)
          └── syscall: connect(api.flights.com)  ← eBPF sees this
      └── llm.reason() (150ms, "Found 3 options, selecting cheapest")
      └── tool.book_flight() (API call, 800ms)
          └── syscall: connect(api.bookings.com)  ← eBPF sees this
  └── response (total: 1.8s, $0.02 LLM cost)
```

**dogwatch opportunity:** First-class AI agent tracing. See agent reasoning + system effects in unified view.

---

### Emerging Areas to Watch

#### QUIC/HTTP3 Observability

**Problem:** QUIC is UDP-based, fully encrypted, connection IDs rotate. Traditional inspection doesn't work.

**Research:**
- [VisQUIC Dataset](https://arxiv.org/html/2410.03728v6) - 100K+ labeled QUIC traces
- ML achieving 97% accuracy on encrypted traffic classification

**Opportunity:** First-mover advantage in HTTP/3 world as QUIC becomes default.

#### Confidential Computing Observability

**Problem:** AMD SEV-SNP / Intel TDX encrypt VM memory. Hypervisor can't see inside.

**Research:**
- [SIGMETRICS 2025](https://dl.acm.org/doi/10.1145/3700418) - CVM analysis
- AMD working on "Performance Counter Virtualization"

**Opportunity:** Observability that works inside confidential VMs.

#### Telemetry Compression at Edge

**Problem:** IoT generates 180 zettabytes/year. Can't send everything.

**Key papers:**
- [TSCom-Bench](https://arxiv.org/abs/2509.21002) - Foundation models for compression
- [Semantic Compression](https://arxiv.org/html/2503.13246v2) - Compress for goals, not bits

**Opportunity:** Smart edge filtering that keeps anomalies, drops noise.

---

### Research-Informed Product Roadmap

Based on the research above, here's the technology adoption timeline:

#### Phase 1: Near-term (6-12 months)
| Technology | Maturity | Action |
|------------|----------|--------|
| kTLS + eBPF | Ready | Implement to fix HTTPS |
| Time series foundation model | Products exist | Integrate for zero-shot anomaly |
| LLM investigation | Mature | Add natural language queries |

#### Phase 2: Medium-term (12-24 months)
| Technology | Maturity | Action |
|------------|----------|--------|
| GNN for traces | Papers → code | Build trace-based anomaly detection |
| Causal inference | Early products | Replace correlation with causation |
| eGPU | Working code | Add GPU visibility for AI workloads |

#### Phase 3: Long-term (24-36 months)
| Technology | Maturity | Action |
|------------|----------|--------|
| Self-healing loop | Research | Full autonomy: detect → fix → learn |
| Rex (Rust eBPF) | New research | Migrate probes to safe Rust |
| Agent observability | Emerging | First-class AI agent tracing |

---

### The Killer Stack (Research-Backed)

```
dogwatch 2027 (research-informed architecture):
─────────────────────────────────────────────────

COLLECTION:     eBPF (CPU) + eGPU (GPU) + kTLS (HTTPS)
                       │
                       ▼
REPRESENTATION: Dynamic service graph from traces
                       │
                       ▼
DETECTION:      GNN for topology anomalies
                + Time series foundation model for metrics
                + No explicit rules needed
                       │
                       ▼
DIAGNOSIS:      Causal inference (not correlation)
                + LLM for natural language reasoning
                + Counterfactual "what-if" analysis
                       │
                       ▼
REMEDIATION:    Agentic AI + Reinforcement Learning
                + Runbook execution
                + Outcome-based learning
                       │
                       ▼
RESULT:         "We see everything (CPU, GPU, encrypted)
                 We understand causation (not just correlation)
                 We fix it automatically (self-healing)"
```

---

### Key Research Resources

**Surveys & Overviews:**
- [eBPF Ecosystem 2024-2025](https://eunomia.dev/blog/2025/02/12/ebpf-ecosystem-progress-in-20242025-a-technical-deep-dive/)
- [Foundation Models for Time Series](https://arxiv.org/abs/2504.04011)
- [AIOps in Era of LLMs](https://arxiv.org/abs/2507.12472)
- [RCA in Microservices Survey](https://arxiv.org/html/2408.00803v1)
- [Causal RCA Survey](https://arxiv.org/html/2510.19593v1)

**GitHub Repositories:**
- [bpftime](https://github.com/eunomia-bpf/bpftime) - Userspace eBPF + GPU
- [Awesome LLM-AIOps](https://github.com/Jun-jie-Huang/awesome-LLM-AIOps) - Curated papers
- [eBPF for Windows](https://github.com/microsoft/ebpf-for-windows)
- [Retina](https://github.com/microsoft/retina) - Cross-platform observability

**Academic Labs:**
- [Tsinghua NetMan Lab](https://netman.aiops.org/publications/) - Leading AIOps research
- [eunomia-bpf](https://github.com/eunomia-bpf) - eBPF innovation

---

### Additional Research Areas (Deep Dive)

The following sections cover additional cutting-edge research discovered through comprehensive arxiv analysis.

#### Smart Trace Sampling

The volume problem is real - Alibaba generates **18-20 PB of traces per day**. Traditional sampling loses critical context.

| Paper | Venue | Innovation | Key Result |
|-------|-------|------------|------------|
| [Trace Sampling 2.0](https://arxiv.org/abs/2509.13852) | arXiv Sep 2025 | Span-level sampling (not trace-level) | Keeps ALL traces, reduces storage via smart span selection |
| [Tracezip](https://arxiv.org/abs/2502.06318) | arXiv Feb 2025 | Trace compression at source | Compress spans on generation, decompress at backend |
| [TraceMesh](https://arxiv.org/abs/2406.06975) | arXiv Jun 2024 | Streaming + scalable sampling | Handles scale without buffering |
| [Astraea](https://arxiv.org/abs/2405.15645) | arXiv May 2024 | Bayesian + multi-armed bandit | Uses only 10-28% of instrumentation for same accuracy |

**Key insight:** Tail-based sampling catches anomalies but loses 99%+ of normal traces needed for comparison. Span-level and compression approaches preserve structure while reducing volume.

**dogwatch opportunity:** Implement span-level sampling that keeps trace structure while dramatically reducing storage. Enable "keep everything, store smartly" approach.

---

#### LLM-Powered Log Parsing

Eliminate regex maintenance hell.

| Paper | Venue | Innovation | Improvement |
|-------|-------|------------|-------------|
| [LogLLM](https://arxiv.org/html/2411.08561v5) | arXiv Apr 2025 | BERT + Llama for log anomaly | 6.6% better F1 than NeuralLog |
| [Log Parsing Survey](https://arxiv.org/html/2504.04877v2) | arXiv May 2025 | Comprehensive LLM parsing review | Covers Lemur, LogParser-LLM, LogBabylon |
| Lemur | Referenced in survey | Chain-of-Thought template merging | 3-round dialogue for semantic parsing |

**The shift:**
```
Traditional:     Log → Regex rules → Template → Analysis
                 (brittle, requires maintenance per format)

LLM-based:       Log → LLM semantic understanding → Analysis
                 (adapts to any format, no regex needed)
```

**dogwatch opportunity:** Parse any log format without predefined templates. Zero-config log analysis.

---

#### Automatic Service Dependency Discovery

Don't require manual service registration or topology maintenance.

| Paper | Venue | Innovation | Accuracy |
|-------|-------|------------|----------|
| [Retrofitting Dependency Discovery](https://arxiv.org/abs/2510.15490) | arXiv Oct 2025 | No instrumentation, NAT-resilient | Correct graph across all benchmarks |
| [Model Discovery for Chaos](https://arxiv.org/abs/2506.11176) | arXiv Jun 2025 | Mine traces → dependency graph in CI/CD | Enables chaos simulation |

**Key insight:** Extract live dependency graphs automatically from existing telemetry (traces, service mesh data, configs). No manual topology maintenance needed.

**dogwatch opportunity:** eBPF sees all connections. Automatically build and maintain service dependency graph without any configuration.

---

#### Performance Regression Detection

Catch degradation before users do.

| Paper | Venue | Innovation | Result |
|-------|-------|------------|--------|
| [Early Detection via Arch Models](https://arxiv.org/html/2408.08148v1) | arXiv Aug 2024 | Component → architectural propagation | Detect in dev phase, not production |
| [Tracing Optimization](https://arxiv.org/abs/2411.17548) | arXiv Nov 2024 | Smart function selection | 80% less tracing overhead, R² of 99% |
| [ConfFLARE](https://arxiv.org/abs/2511.17271) | arXiv Nov 2025 | Configurable system regression | 70-79% fewer configs to test |
| [Nyrkiö Change Detection](https://arxiv.org/html/2510.11310) | arXiv Oct 2025 | GitHub Actions integration | Automated regression alerts in CI |

**Key insight:** Predict performance regression from component-level tests before full deployment. Shift left on performance.

**dogwatch opportunity:** Integrate with CI/CD to detect performance regressions before they reach production.

---

#### Contrastive Learning for Anomaly Detection

Learn what's normal, detect what's not - without explicit rules.

| Paper | Venue | Innovation | Key Feature |
|-------|-------|------------|-------------|
| [CARLA](https://arxiv.org/abs/2308.09296) | arXiv Aug 2024 | Self-supervised contrastive | Learns both normal AND deviation patterns |
| [CNT](https://arxiv.org/abs/2304.07898) | arXiv Jan 2025 | Window-based + learnable transforms | Prevents representation collapse |
| [DACAD](https://arxiv.org/abs/2404.11269) | arXiv Apr 2024 | Domain adaptation + contrastive | Works across domains with scarce labels |
| [Causality-Aware Contrastive](https://arxiv.org/html/2506.03964v1) | arXiv Jun 2025 | Causal augmentations | Exploits causal relationships |
| [TFCL](https://link.springer.com/article/10.1007/s40747-025-02119-w) | Complex & Intelligent Systems 2025 | Time-frequency contrastive | Both time and frequency features |

**The technique:**
```python
# Contrastive learning creates embeddings where:
# - Similar patterns (normal) cluster together
# - Anomalies are naturally distant

def contrastive_anomaly_detection(time_series):
    # 1. Create augmented views of windows
    # 2. Train encoder to maximize similarity of same-window views
    # 3. Minimize similarity of different-window views
    # 4. Anomalies = windows far from normal clusters

    embedding = encoder(time_series)
    distance_to_normal = compute_distance(embedding, normal_cluster)
    return distance_to_normal > threshold
```

**dogwatch opportunity:** Replace rule-based anomaly detection with learned representations. No threshold tuning needed.

---

#### Diffusion Models for Time Series

The new frontier beyond transformers for forecasting and anomaly detection.

| Paper | Venue | Application | Innovation |
|-------|-------|-------------|------------|
| [Diffusion Models for TSF Survey](https://arxiv.org/abs/2507.14507) | arXiv Aug 2025 | Comprehensive survey | Taxonomy of diffusion for time series |
| [Survey on Diffusion for TS](https://arxiv.org/html/2404.18886v1) | arXiv Apr 2024 | Forecasting, generation, anomaly, imputation | All four tasks covered |
| MAFCD | Information Fusion 2025 | Multi-level adaptive conditional | State-of-art anomaly detection |
| DiffGSL | IEEE Big Data 2025 | Graph structure learning | Spatio-temporal forecasting |
| DiffusionAD | IEEE TPAMI 2025 | One-step denoising | Faster anomaly detection |

**Why diffusion models:**
- Handle multimodal distributions (metrics can have multiple "normal" states)
- Naturally model uncertainty
- Generate plausible counterfactuals ("what should this metric look like?")

**dogwatch opportunity:** Use diffusion models for anomaly detection that understands "this metric has multiple valid patterns."

---

#### Reinforcement Learning for Kubernetes Autoscaling

Learn optimal scaling policies that beat threshold-based HPAs.

| Paper | Venue | Innovation | Improvement |
|-------|-------|------------|-------------|
| [KARMA](https://arxiv.org/abs/2505.21559) | arXiv May 2025 | Multi-agent RL for resilience | Handles DDoS, cascading failures |
| [DRPC](https://arxiv.org/html/2407.10169v1) | arXiv Jul 2024 | Distributed RL (TD3) | 15% lower latency, 24% fewer failures |
| [KIS-S](https://arxiv.org/html/2507.07932) | arXiv Jul 2025 | GPU-aware RL autoscaler | Handles inference workloads |
| [Gwydion](https://www.sciencedirect.com/science/article/pii/S1084804524002443) | J. Network & Computer Apps Feb 2025 | RL for microservice graphs | Handles inter-dependencies |
| [SDQN](https://arxiv.org/html/2601.13579) | arXiv Jan 2025 | DQN-based scheduling | 20% lower CPU utilization |

**Key insight:** RL learns scaling policies from experience. Handles complex scenarios (DDoS, cascading failures) that threshold-based systems can't.

**dogwatch opportunity:** Offer intelligent autoscaling recommendations or direct integration based on learned policies.

---

#### Mamba/State Space Models for Time Series

Linear complexity alternative to Transformers for long sequences.

| Paper | Venue | Innovation | Result |
|-------|-------|------------|--------|
| [Mamba](https://arxiv.org/abs/2312.00752) | arXiv (Updated May 2024) | Selective state spaces | 5× throughput, linear scaling |
| [MambaTS](https://arxiv.org/abs/2405.16440) | arXiv May 2024 | Mamba for long-term forecasting | New SOTA on 8 datasets |
| [PowerMamba](https://arxiv.org/html/2412.06112) | arXiv Dec 2024 | Power systems time series | Outperforms all baselines |

**Why Mamba matters for observability:**
```
Transformer:  O(n²) complexity → struggles with long sequences
Mamba:        O(n) complexity → handles streaming data efficiently

For telemetry:
- Metrics arriving continuously
- Need to process hours/days of history
- Mamba: 5× faster inference, same accuracy
```

**dogwatch opportunity:** Use Mamba for efficient long-context analysis of metrics and traces. Handle hours of history in real-time.

---

#### Knowledge Graphs for Incident Management

Structured reasoning over incidents and operations.

| Paper | Venue | Innovation | Improvement |
|-------|-------|------------|-------------|
| [Support Ticket KG](https://arxiv.org/abs/2501.00461) | arXiv Dec 2024 | KG embeddings from multi-source | Better than TF-IDF for ticket routing |
| [BRIDG-ICS](https://arxiv.org/html/2512.12112v1) | arXiv Dec 2024 | Cybersecurity KG | Integrates CVE, CWE, attack patterns |
| [KG for Cloud Ops](https://link.springer.com/chapter/10.1007/978-3-032-11477-8_11) | Springer 2025 | Unified agentic memory | Better retrieval + causal analysis |

**The approach:**
```
Traditional:     Incident → Search logs → Manual correlation
                 (slow, misses connections)

KG-based:        Incident → Query knowledge graph
                 → Find related past incidents
                 → Traverse causal relationships
                 → Suggest likely root cause + remediation
```

**dogwatch opportunity:** Build knowledge graph from incidents over time. Enable "we've seen this before" retrieval and causal reasoning.

---

#### Federated eBPF Monitoring (FedMon)

Privacy-preserving distributed observability - critical for multi-tenant and regulated environments.

| Paper | Venue | Innovation | Why It Matters |
|-------|-------|------------|----------------|
| [FedMon](https://arxiv.org/html/2510.10126) | arXiv Oct 2025 | eBPF + Federated Learning | Train models without sharing raw telemetry |

**The architecture:**
```
Traditional centralized:
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Cluster1 │────▶│ Central  │◀────│ Cluster2 │
│ telemetry│     │ Analysis │     │ telemetry│
└──────────┘     └──────────┘     └──────────┘
                 (privacy risk, bandwidth cost)

FedMon distributed:
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Cluster1 │     │  Global  │     │ Cluster2 │
│ eBPF+    │────▶│  Model   │◀────│ eBPF+    │
│ Local ML │     │(gradients│     │ Local ML │
└──────────┘     │  only)   │     └──────────┘
                 └──────────┘
                 (privacy preserved, bandwidth efficient)
```

**Key insight:** Each cluster trains locally on eBPF telemetry, shares only model updates. Global model improves without raw data exposure.

**dogwatch opportunity:** Enable multi-org observability (MSPs, regulated industries) without sharing sensitive telemetry.

---

#### LLM-based Fault Localization

Connect observability to code-level diagnosis.

| Paper | Venue | Approach | Accuracy |
|-------|-------|----------|----------|
| [FaR-Loc](https://arxiv.org/abs/2509.20552) | arXiv Sep 2025 | RAG + LLM | 14.6% better Top-1 than SoapFL |
| [AgentFL](https://arxiv.org/abs/2403.16362v2) | arXiv 2024 | Multi-agent ChatGPT | 157/395 bugs at Top-1, $0.074/bug |
| [LLMAO](https://arxiv.org/pdf/2310.01726) | arXiv 2024 | Test-free localization | Works without test suite |
| [DEVLoRe](https://arxiv.org/html/2412.03905) | arXiv Dec 2024 | Multi-artifact integration | 142/259 bugs repaired |

**The connection:**
```
Observability data:           Code-level diagnosis:
├── Stack trace          ───▶ Likely faulty file/function
├── Error logs           ───▶ Error context
├── Trace spans          ───▶ Which service caused it
└── Metrics anomaly      ───▶ When it started

LLM synthesizes all artifacts → Points to exact line of code
```

**dogwatch opportunity:** Go beyond "service X is slow" to "function Y in file Z at line 142 is the likely cause."

---

#### Streaming & Long-Context Inference

Handle infinite telemetry streams with bounded memory.

| Paper | Venue | Innovation | Capability |
|-------|-------|------------|------------|
| [Infini-attention](https://arxiv.org/html/2404.07143v1) | arXiv Apr 2024 | Bounded memory + streaming | Infinite context under fixed memory |
| [StreamingLLM](https://arxiv.org/html/2404.07143v1) | Referenced | Attention sink + sliding window | Stable over arbitrarily long sequences |
| [Titans](https://arxiv.org/html/2501.00663v1) | arXiv Jan 2025 | Neural long-term memory | Learn to memorize at test time |
| [Radar](https://arxiv.org/abs/2503.10571) | arXiv Mar 2025 | Dynamic token importance | Training-free acceleration |

**The problem and solution:**
```
Problem:  Telemetry never stops. Can't keep infinite history in memory.
          Traditional: truncate context → lose important history

Solution: Streaming architectures
          ├── Compress old context into fixed-size memory
          ├── Keep recent context in full detail
          └── Attend to both: recent + compressed history

Result:   Analyze hours/days of telemetry with constant memory
```

**dogwatch opportunity:** Enable "ask anything about the last 24 hours" without storing full context. Bounded memory, infinite history.

---

#### Network Topology Inference

Automatically discover infrastructure topology from passive measurements.

| Paper | Venue | Innovation | Result |
|-------|-------|------------|--------|
| [Topology from Smooth Signals](https://arxiv.org/abs/2410.05707) | arXiv Oct 2024 | Partial observability handling | Linear convergence guarantee |
| [SecureNT](https://arxiv.org/html/2412.08177v2) | arXiv Dec 2024 | Privacy-preserving topology | Defends against inference attacks |
| [DFL Topology Attack](https://arxiv.org/abs/2501.03119) | arXiv Jan 2025 | Infer topology from model behavior | Reveals hidden connections |

**dogwatch opportunity:** Infer infrastructure topology from eBPF observations without explicit configuration.

---

### Implementation Priority Matrix

Based on research maturity and dogwatch relevance:

| Technology | Research Maturity | Implementation Effort | Impact | Priority |
|------------|-------------------|----------------------|--------|----------|
| **Span-level trace sampling** | Ready (papers + code) | Medium | Critical for scale | 🔥🔥🔥 P0 |
| **LLM log parsing** | Ready (LogLLM works) | Medium | No more regex | 🔥🔥🔥 P0 |
| **Auto dependency discovery** | Ready | Low (eBPF has data) | Zero-config topology | 🔥🔥🔥 P0 |
| **Contrastive anomaly** | Mature | Medium | No rule maintenance | 🔥🔥 P1 |
| **Mamba for streaming** | Ready | Medium | Efficient inference | 🔥🔥 P1 |
| **FedMon pattern** | New but solid | High | Multi-tenant privacy | 🔥🔥 P1 |
| **Streaming inference** | Ready | Medium | Infinite telemetry | 🔥🔥 P1 |
| **RL autoscaling** | Mature | High | Smart recommendations | 🔥 P2 |
| **Diffusion models** | Emerging | High | Better uncertainty | 🔥 P2 |
| **Knowledge graphs** | Research | High | Long-term reasoning | 🔥 P2 |
| **LLM fault localization** | Mature | Medium | Code-level diagnosis | 🔥 P2 |
| **Perf regression in CI** | Mature | Medium | Shift-left | 🔥 P2 |

---

### Additional Research Resources

**Trace Sampling:**
- [Awesome-TimeSeries-Diffusion](https://github.com/yyysjz1997/Awesome-TimeSeries-SpatioTemporal-Diffusion-Model) - Diffusion models for time series

**Log Analysis:**
- [AwesomeLLM4SE](https://github.com/iSEngLab/AwesomeLLM4SE) - LLM for software engineering survey

**Mamba/SSM:**
- [state-spaces/mamba](https://github.com/state-spaces/mamba) - Official Mamba implementation

**Federated Learning:**
- [FedMon concept](https://arxiv.org/html/2510.10126) - eBPF + FL integration

**Streaming/Long-Context:**
- [long-llms-learning](https://github.com/Strivin0311/long-llms-learning) - Long-context LLM resources

---

## Go-to-Market Strategy

### Customer Personas

#### Primary: The Frustrated SRE (Individual Contributor)

```
Name: Alex, Site Reliability Engineer
Company: Series A startup, 50-200 employees
Stack: Kubernetes, PostgreSQL, Redis, 10-50 microservices

Pain points:
- Paying $15K+/month for Datadog, leadership asking to cut costs
- Spent 2 weeks integrating APM SDKs, still missing services
- On-call and can't figure out why things are slow
- Dashboard sprawl, no one knows what's monitored

Buying behavior:
- Finds tools on Hacker News, Reddit, Twitter
- Tries before buying
- Champions tool internally, needs to convince manager
- Budget: Can expense $99/mo, needs approval for more

How we win:
- Zero-config demo in 5 minutes
- "Look, it found 3 services we forgot to instrument"
- Cost comparison screenshot to share with manager
```

#### Secondary: The Platform Team Lead

```
Name: Jordan, Platform Engineering Manager
Company: Growth stage, 200-1000 employees
Stack: Multi-cluster K8s, service mesh, 100+ services

Pain points:
- Teams shipping without observability
- Inconsistent instrumentation across teams
- Can't enforce standards
- Datadog bill growing 30% month-over-month

Buying behavior:
- Evaluates tools formally (POC, security review)
- Needs enterprise features (SSO, RBAC)
- Budget holder, can approve $5K+/mo
- Cares about vendor stability

How we win:
- Control plane shows "Team X costs $Y"
- Enforce observability without requiring team cooperation
- Migration path from Datadog with savings report
```

#### Tertiary: The Cost-Conscious CTO

```
Name: Sam, CTO/VP Engineering
Company: Series B+, cost optimization mode
Stack: Whatever the teams use

Pain points:
- Board asking about cloud spend
- Observability is 10%+ of infrastructure cost
- Can't cut it without losing visibility

Buying behavior:
- Hears about tools from team or peers
- Wants ROI calculation
- Needs to justify to CFO/board
- Will pay for enterprise if value is clear

How we win:
- "$564K/year savings" headline
- Professional services for migration
- Executive-friendly reports
```

---

### Finding First 10 Customers

#### Week 1-2: Personal Network

```
1. Post on personal Twitter/LinkedIn
   "Built an observability tool that auto-traces everything via eBPF.
    No SDKs. Shows what you'd pay on Datadog.
    Looking for 5 beta testers. DM me."

2. Email 50 people you know personally
   - Former colleagues
   - Meetup contacts
   - Conference connections
   Subject: "Need your help testing my observability tool"

3. Ask for intros
   "Do you know anyone frustrated with their Datadog bill?"
```

#### Week 3-4: Community Seeding

```
1. Hacker News "Show HN"
   Title: "Show HN: dogwatch – eBPF observability that shows your Datadog bill"
   - Post at 9am EST Tuesday/Wednesday
   - Be available to answer every comment
   - Don't be salesy, be helpful

2. Reddit
   - r/kubernetes (100K+ members)
   - r/devops (300K+ members)
   - r/sre (50K+ members)
   Post: "I built X, here's what I learned about eBPF"
   Not: "Check out my product"

3. Dev.to / Hashnode
   "How we replaced Datadog with eBPF and saved $40K/month"
   (Write about the technical journey, link to dogwatch at end)
```

#### Week 5-8: Targeted Outreach

```
1. Find Datadog complainers
   - Twitter search: "datadog expensive" OR "datadog bill"
   - Reply helpfully, don't pitch immediately
   - DM after building rapport

2. GitHub stars of similar projects
   - People who starred Pixie, Grafana, Prometheus
   - They're interested in observability
   - Cold DM: "Saw you starred X, working on Y, would love feedback"

3. Kubernetes Slack communities
   - #observability channels
   - Answer questions, become known
   - Mention dogwatch when relevant
```

---

### Content Strategy

#### SEO Target Keywords

| Keyword | Volume | Difficulty | Intent |
|---------|--------|------------|--------|
| "datadog alternative" | 1.2K/mo | Medium | High |
| "datadog pricing" | 2.4K/mo | Medium | Research |
| "open source apm" | 800/mo | Low | High |
| "ebpf observability" | 400/mo | Low | High |
| "kubernetes monitoring" | 3.2K/mo | High | Medium |
| "distributed tracing" | 1.8K/mo | High | Research |
| "grafana vs datadog" | 600/mo | Medium | Comparison |

#### Content Calendar (First 3 Months)

**Month 1: Foundation**
- Landing page with clear value prop
- "How dogwatch works" technical deep-dive
- "Datadog vs dogwatch" comparison page
- Install docs with video walkthrough

**Month 2: SEO Play**
- "Complete guide to eBPF observability"
- "How to reduce observability costs by 70%"
- "Distributed tracing without code changes"
- "Kubernetes monitoring in 2024: Options compared"

**Month 3: Social Proof**
- Case study: "How [Company] saved $X with dogwatch"
- "Why we switched from Datadog"
- Technical blog: "Building protocol parsers in eBPF"
- Comparison: "dogwatch vs Pixie vs Grafana"

---

### Competitive Battlecards

#### vs Datadog

```
When they say: "Datadog has 600+ integrations"
You say: "You don't need integrations when eBPF sees everything automatically.
          How many weeks did it take to integrate your last 10 services?"

When they say: "Datadog is the industry standard"
You say: "The industry standard for bill shock. Show me a company happy with
          their Datadog bill. We show you exactly what you'd pay them."

When they say: "We need enterprise support"
You say: "We offer enterprise support at $299/mo. Datadog enterprise is
          $50K+/year minimum. What level of support do you actually need?"

When they say: "Datadog has more features"
You say: "Which features do you actually use? Our control plane shows
          80% of metrics are never queried. More features = more cost."

Killer demo moment:
→ Install dogwatch (60 seconds)
→ Show service map auto-discovered
→ Show database queries auto-captured
→ Show "This would cost $X on Datadog"
→ Ask: "How long did Datadog take to set up?"
```

#### vs Grafana Stack

```
When they say: "Grafana is open source and free"
You say: "Free if you don't count the 3 engineers spending 20% of their time
          managing Prometheus + Loki + Tempo + Grafana + alertmanager.
          How's that going?"

When they say: "We already have Grafana dashboards"
You say: "Keep them. dogwatch imports Grafana dashboards. Add zero-config
          tracing and cost intelligence on top of what you have."

When they say: "We know Grafana, don't want to learn new tool"
You say: "Fair. But your team also knows 'why is this slow?' takes hours.
          dogwatch answers that in seconds. Worth learning for that?"

Killer demo moment:
→ Show BubbleUp: "98% of slow requests hit shard 7"
→ Ask: "How long would that take in Grafana?"
```

#### vs New Relic / Pixie

```
When they say: "Pixie does eBPF too"
You say: "Pixie is New Relic now. Same vendor lock-in, same pricing games.
          dogwatch is independent and self-hosted if you want."

When they say: "New Relic has better AI features"
You say: "AI that costs $0.30/GB. Our BubbleUp does automatic root cause
          analysis without the per-GB pricing."

Killer demo moment:
→ Show cost intelligence
→ "New Relic would charge $X for this data volume"
```

---

## First-Time User Experience

### The Golden Path (5 Minutes to Wow)

```
Minute 0:00 - Install
$ curl -sSL https://get.dogwatch.dev | sh
# or
$ kubectl apply -f https://dogwatch.dev/install.yaml

Minute 0:30 - First Data
Browser opens automatically to localhost:9999
"Discovering services... Found 7 services"
[Service map appears with live traffic]

Minute 1:00 - Wow Moment #1
"Click on 'api-service' to see all requests"
[Shows HTTP requests with latency, status codes]
[Shows database queries this service makes]
"We found this without any code changes."

Minute 2:00 - Wow Moment #2
"See this slow request? Click to trace it."
[Shows distributed trace across 3 services]
[Shows exact database query that was slow]
"Traditional APM needs SDK integration for this."

Minute 3:00 - Wow Moment #3
"Now let's see what this would cost elsewhere."
[Shows Cost Intelligence dashboard]
"Based on your usage: Datadog estimate $12,400/month"
"You're getting this for free with dogwatch."

Minute 4:00 - Hook
"Want to keep this data? Create an account."
[Simple signup - email only, no credit card]
"Your data persists. We'll email you weekly insights."

Minute 5:00 - Expansion
"Invite your team to see this too."
[Share link with read-only access]
```

### Onboarding Checklist UI

```
┌─────────────────────────────────────────────────────────────┐
│  🚀 Get the most out of dogwatch                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ✅ Install dogwatch agent                                  │
│  ✅ Discover services (found 7)                             │
│  ✅ View your first trace                                   │
│                                                              │
│  ◯ Set up your first alert                                  │
│     → Alert when API latency > 500ms                        │
│                                                              │
│  ◯ Connect Slack/PagerDuty                                  │
│     → Get notified when things break                        │
│                                                              │
│  ◯ Import Datadog dashboards                                │
│     → Bring your existing dashboards                        │
│                                                              │
│  ◯ Invite a teammate                                        │
│     → Share the visibility                                  │
│                                                              │
│  Progress: 3/7 complete                                     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Open Source Strategy

### License

**Recommendation: Apache 2.0 for core, proprietary for enterprise features**

```
Open Source (Apache 2.0):
├── eBPF probes and protocol parsing
├── Core tracing, metrics, logs
├── Basic dashboards and alerting
├── Single-node deployment
├── Community scripts
└── Basic RBAC

Proprietary (Source Available):
├── SSO/SAML integration
├── Advanced RBAC
├── Multi-tenancy
├── Control plane with shaping
├── Migration assistant (full)
├── Enterprise support
└── Clustering/HA
```

**Why this split:**
- Open core gets adoption and contributions
- Enterprise features fund development
- No one can fork and compete on enterprise
- Contributors get value, company makes money

### Community Building

**GitHub Strategy:**
- Respond to every issue within 24 hours
- Label issues clearly (good-first-issue, help-wanted)
- Public roadmap in GitHub Projects
- Monthly "office hours" for contributors

**Discord/Slack:**
- #general - community chat
- #help - support questions
- #contributing - for contributors
- #showcase - users sharing their setups

**Recognition:**
- Contributors in release notes
- "Community spotlight" blog posts
- Swag for significant contributors
- Invite top contributors to advisory role

---

## Team & Hiring Plan

### Solo → $20K MRR

**Just you:**
- Build core product
- Handle support (< 10 customers)
- Do your own marketing
- Wear all hats

### $20K → $50K MRR

**Hire #1: Developer Advocate / Growth**
```
Why: You're bottlenecked on awareness, not product
Role:
- Content creation (blog, videos, talks)
- Community management
- User onboarding calls
- Competitive research

Cost: $80-120K/year
```

### $50K → $100K MRR

**Hire #2: Senior Backend Engineer**
```
Why: You're bottlenecked on shipping features
Role:
- Core product development
- eBPF/systems work
- Performance optimization

Cost: $150-200K/year
```

**Hire #3: Customer Success / Support**
```
Why: Support is taking too much of your time
Role:
- Handle support tickets
- Onboarding calls
- Documentation
- Renewal conversations

Cost: $70-100K/year
```

### $100K MRR+ (Series A territory)

- VP Engineering (if you want to stay technical)
- VP Sales (if going enterprise)
- More engineers based on roadmap
- Consider outside CEO if you want to stay technical

---

## Exit Strategy

### What Makes Us Acquirable

| Asset | Value | Likely Acquirer |
|-------|-------|-----------------|
| **eBPF tech** | Unique expertise, hard to build | Datadog, Splunk, Cisco |
| **Customer base** | Warm leads for their sales team | Any observability vendor |
| **Team** | Systems engineers are expensive | Everyone |
| **Open source community** | Built-in distribution | Cloud providers |
| **Product** | Fill gap in their portfolio | Grafana Labs, Elastic |

### Potential Acquirers

**Tier 1 (Most Likely):**
- **Datadog** - Kill a competitor, acquire eBPF tech
- **Grafana Labs** - Add zero-config tracing to their stack
- **Elastic** - Observability is strategic for them

**Tier 2 (Strategic):**
- **Cisco/Splunk** - They're consolidating observability
- **VMware/Broadcom** - Tanzu observability play
- **ServiceNow** - They bought Lightstep, want more

**Tier 3 (Cloud):**
- **AWS** - Could be CloudWatch addition
- **Google** - Could be Cloud Monitoring addition
- **Microsoft** - Could be Azure Monitor addition

### Acquisition Math

| ARR | Typical Multiple | Valuation |
|-----|-----------------|-----------|
| $500K | 8-15x | $4-7.5M |
| $1M | 10-15x | $10-15M |
| $3M | 10-20x | $30-60M |
| $10M | 15-25x | $150-250M |

**Comparable exits:**
- Pixie → New Relic: ~$200M (2 months post-launch!)
- Lightstep → ServiceNow: ~$500M
- SignalFx → Splunk: $1.05B
- Chronosphere → acquired at $3.35B valuation

**What increases multiple:**
- Growth rate (> 100% YoY)
- Gross margin (> 80%)
- Net retention (> 120%)
- Unique tech (eBPF)
- Strategic fit

### Timeline

```
Year 1: Build product, find PMF, get to $100K ARR
Year 2: Scale to $500K-1M ARR, build team
Year 3: Either:
        a) Raise Series A, go for $10M+ ARR
        b) Accept acquisition offer ($10-30M)
        c) Stay bootstrapped, lifestyle business ($1-3M ARR)
```

---

## Legal Considerations

### Open Source License Compliance

```
Using Apache 2.0 licensed code:
- ✅ Can use commercially
- ✅ Can modify
- ✅ Can distribute
- ⚠️ Must include license
- ⚠️ Must state changes
- ❌ Cannot use contributor trademarks

Key dependencies to audit:
- eBPF libraries (check licenses)
- Prometheus client (Apache 2.0 ✅)
- SQLite (public domain ✅)
- UI frameworks (check each)
```

### Terms of Service (Key Points)

```
1. Service provided "as is" for self-hosted
2. Paid tiers include SLA
3. User responsible for their data
4. We can terminate for abuse
5. Limitation of liability
6. Governing law (Delaware)
```

### Privacy Policy (Key Points)

```
For self-hosted:
- We don't see your data
- Telemetry is opt-in
- No PII collected by default

For cloud features:
- What data we collect
- How we use it
- How to delete it
- GDPR compliance
```

### Trademark

```
Register:
- "dogwatch" wordmark
- Logo
- Domain (dogwatch.dev, dogwatch.io)

Protect against:
- Similar names in observability space
- Confusing forks
```

---

## Demo Environment

### Public Sandbox

```
https://demo.dogwatch.dev

Pre-populated with:
- 10 microservices (simulated e-commerce)
- 24 hours of realistic data
- Pre-built dashboards
- Sample alerts (some firing)
- Example traces with issues

Login:
- Email: demo@dogwatch.dev
- Password: trydogwatch

Restrictions:
- Read-only (can't create/modify)
- Resets every hour
- Rate limited
```

### "Try on Your Infra" Flow

```
1. One-line install (curl | sh)
2. Runs for 15 minutes
3. Data persists to /tmp (not permanent)
4. Shows "Create account to keep this data"
5. No credit card required

This is the conversion funnel:
Try (free) → Use (free tier) → Pay (team/business)
```

---

## Support Strategy

### Tiered Support Model

| Tier | Response Time | Channels | Who |
|------|--------------|----------|-----|
| **Community** | Best effort | GitHub, Discord | You + community |
| **Team $99** | 24 hours | Email, Discord | You |
| **Business $299** | 4 hours | Email, Slack | You (then hire) |
| **Enterprise** | 1 hour, 24/7 | Dedicated Slack | Hire for this |

### Scaling Support

**0-50 customers:**
- You answer everything
- Build FAQ from common questions
- Create video tutorials

**50-200 customers:**
- Hire part-time support person
- Implement help desk (Intercom, Zendesk)
- Create knowledge base

**200+ customers:**
- Full-time support hire
- On-call rotation for enterprise
- Support metrics (response time, satisfaction)

### Self-Service Priority

```
Best support is no support needed.

Invest in:
1. Comprehensive docs
2. In-app guidance
3. Error messages with solutions
4. Status page
5. Community forums where users help each other
```
