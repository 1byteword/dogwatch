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

## Roadmap

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

| Feature | Min Effort | Full Effort | Impact | Build Order |
|---------|-----------|-------------|--------|-------------|
| Cost Intelligence | 1 week | 3 weeks | ★★★★★ | 1st |
| Change Correlation | 3 weeks | 5 weeks | ★★★★★ | 2nd |
| Production Essentials | 10 weeks | 10 weeks | ★★★★★ | 3rd |
| BubbleUp | 4 weeks | 7 weeks | ★★★★☆ | 4th |
| Pre-Built Scripts | 4 weeks | 7 weeks | ★★★★☆ | 5th |
| Protocol Parsing | 6 weeks | 20 weeks | ★★★★☆ | 6th |
| Control Plane | 6 weeks | 16 weeks | ★★★★☆ | 7th |
| Migration Assistant | 4 weeks | 13 weeks | ★★★☆☆ | 8th |
| Continuous Profiling | 3 weeks | 10 weeks | ★★★☆☆ | 9th |
| Security | 3 weeks | 13 weeks | ★★☆☆☆ | 10th |

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

The core differentiator. Each protocol parser needs:

```c
// Example: MySQL protocol parser structure
struct mysql_event {
    u64 timestamp;
    u32 pid;
    u32 tid;
    u64 connection_id;    // For correlation
    u8  command_type;     // COM_QUERY, COM_STMT_EXECUTE, etc.
    u16 error_code;
    u32 affected_rows;
    u64 latency_ns;
    char query[256];      // First 256 bytes of query
};

// Hook points:
// - kprobe/tcp_sendmsg (outbound queries)
// - kprobe/tcp_recvmsg (responses)
// - Protocol detection from first bytes (MySQL: packet header + command byte)
```

**Priority order:**
1. MySQL (most common, highest debugging value)
2. PostgreSQL (second most common)
3. Redis (simple protocol, cache debugging)
4. HTTP/2 + gRPC (modern microservices)
5. DNS (often overlooked latency source)

### Cross-Service Trace Correlation

The magic that makes zero-config tracing work:

```go
type TraceCorrelator struct {
    // Method 1: Parse existing trace headers
    // If services already propagate W3C traceparent, B3, X-Request-ID
    ParseTraceHeaders(httpRequest) -> traceID, spanID

    // Method 2: Timing-based correlation
    // When A calls B, the outbound timestamp from A should be
    // within ~1ms of inbound timestamp at B
    CorrelateByTiming(outbound, inbound) -> confidence

    // Method 3: Connection tuple matching
    // A's (srcIP:srcPort -> dstIP:dstPort) = B's accept()
    CorrelateByConnection(clientConn, serverConn) -> match
}
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

## What Pixie Did (That We're Not Doing)

Pixie was acquired by New Relic for ~$200M just **2 months** after public launch. Here's what made them special:

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
