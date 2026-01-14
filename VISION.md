# dogwatch Vision & Roadmap

## Philosophy

**Current tagline**: "eBPF-powered observability"

**Target philosophy**: "From alert to resolution in one place"

The insight from successful platforms:
- **Datadog**: "All telemetry is connected" - correlation is the killer feature
- **PagerDuty**: "Context reduces MTTR" - it's about guided response, not just paging
- **Grafana**: "Flexibility over features" - query anything, visualize anything
- **Pixie/New Relic**: "Zero-instrumentation deep visibility" - eBPF sees everything at protocol level

dogwatch should combine all four: **deep automatic visibility, unified correlation, and guided incident response**.

---

## Current State (What We Have)

### Observability Pillars
| Pillar | Status | Implementation |
|--------|--------|----------------|
| Metrics | ✅ Complete | System metrics, custom metrics, OTLP, StatsD |
| Traces | ✅ Complete | OTLP receiver, service map, dependencies |
| Logs | ✅ Complete | FTS search, patterns, ingestion API |
| Profiling | ✅ Complete | CPU flame graphs (symbol resolution TODO) |
| Synthetics | ✅ Complete | HTTP/TCP checks, uptime tracking |

### Infrastructure Monitoring
| Feature | Status | Implementation |
|---------|--------|----------------|
| System Metrics | ✅ Complete | CPU, memory, disk, network via eBPF |
| TCP Connections | ✅ Complete | eBPF tracepoints |
| HTTP Tracing | ✅ Complete | Plain HTTP via syscall tracing |
| HTTPS Tracing | ❌ Broken | eBPF SSL probes not firing |
| Containers | ✅ Complete | Docker socket integration |
| Kubernetes | ✅ Complete | Full K8s API integration |

### Incident Management
| Feature | Status | Implementation |
|---------|--------|----------------|
| Incidents | ✅ Complete | Full lifecycle, severity, MTTA/MTTR |
| On-Call | ✅ Complete | Schedules, rotations, overrides |
| Escalations | ✅ Complete | Multi-level policies |
| Notifications | ✅ Complete | Webhook, Slack, Email, PagerDuty, OpsGenie, Teams, Discord |
| Status Pages | ✅ Complete | Public pages, components, uptime |

### Platform Features
| Feature | Status | Implementation |
|---------|--------|----------------|
| Dashboards | ✅ Complete | GridStack widgets, persistence |
| Alerting | ✅ Complete | Watch system with conditions |
| SLOs | ✅ Complete | Error budgets, burn rate |
| Anomaly Detection | ✅ Complete | Isolation forest ensemble |
| Query Language | ✅ Complete | WatchQL with planner |
| RBAC | ✅ Complete | Roles, permissions, API keys |
| SSO | ✅ Complete | OAuth2, SAML |
| Federation | ✅ Complete | Gossip-based multi-node |
| Audit Logs | ✅ Complete | Full audit trail |
| Rate Limiting | ✅ Complete | Token bucket per IP/API key |
| Pagination | ✅ Complete | Offset and cursor-based |

### Client Libraries
| Library | Status | Implementation |
|---------|--------|----------------|
| Go APM SDK | ✅ Complete | Spans, metrics, HTTP middleware |
| Go SQL instrumentation | ✅ Complete | Database tracing |
| Go gRPC instrumentation | ✅ Complete | gRPC tracing |
| Go Redis instrumentation | ✅ Complete | Redis tracing |

---

## Critical Gaps (P0/P1)

### 1. Correlation Engine (P0)
**The #1 missing feature.**

Current state: Data silos - traces, logs, metrics, incidents in separate DBs with no links.

Needed:
- Trace ID → find all logs for this trace
- Log entry → find the trace it belongs to
- Alert → show the traces/logs that triggered it
- Metric spike → show correlated traces/errors
- Deploy → show incidents that started after it

Implementation approach:
```
┌─────────────────────────────────────────────────────────┐
│                  Correlation Index                       │
│  trace_id | span_id | log_ids | metric_ts | deploy_id   │
└─────────────────────────────────────────────────────────┘
```

### 2. Service Catalog (P0)
**Foundation for ownership and dependencies.**

Needed:
- Service registry with metadata
- Ownership (team, oncall, contacts)
- Dependencies (upstream/downstream)
- Links (repo, runbook, dashboard, SLO)
- Health rollup from components

### 3. Change Intelligence (P1)
**Connect deployments to incidents.**

Needed:
- Auto-detect "incident started X min after deploy Y"
- Deployment diff view (what changed)
- Rollback correlation (did rollback fix it?)
- Feature flag change tracking
- Config change tracking

### 4. Error Tracking (P1)
**Sentry-like error management.**

Needed:
- Stack trace grouping/fingerprinting
- Error occurrence counting
- First seen / last seen / frequency
- Release/version correlation
- Assignment and status tracking
- Integration with incidents

### 5. Runbooks & Response (P1)
**Make incidents actionable.**

Needed:
- Runbook attachment to alerts/services
- Step-by-step response guides
- Auto-remediation actions
- Response templates
- Checklist tracking

### 6. Investigation Timeline (P2)
**Collaborative incident response.**

Needed:
- Chronological event timeline
- Responder comments/notes
- Evidence attachments (graphs, traces, logs)
- Hypothesis tracking
- Post-mortem generation

---

## eBPF Gaps (vs Pixie)

Pixie (acquired by New Relic for ~$100M) showed what's possible with eBPF. We're significantly underutilizing it.

### Current dogwatch eBPF Probes

| Probe | File | What It Does |
|-------|------|--------------|
| TCP Connect | `bpf/tcpconnect.c` | Track TCP connection establishment |
| HTTP | `bpf/http.c` | Extract method/path from first 256 bytes |
| SSL | `bpf/ssl.c` | Attempt to trace OpenSSL (broken) |
| Profile | `bpf/profile.c` | CPU sampling at 49Hz |

### What Pixie Does That We Don't

| Capability | Pixie | dogwatch | Gap |
|------------|-------|----------|-----|
| HTTP tracing | ✅ Full request/response body | ⚠️ First 256 bytes only | Need full payload + body parsing |
| HTTPS/TLS | ✅ Via SSL uprobes | ❌ Broken | Fix uprobe attachment |
| MySQL protocol | ✅ Full query capture | ❌ None | Add wire protocol parser |
| PostgreSQL protocol | ✅ Full query capture | ❌ None | Add wire protocol parser |
| Redis protocol | ✅ Command + args | ❌ None | Add RESP protocol parser |
| Cassandra protocol | ✅ CQL capture | ❌ None | Add CQL parser |
| Kafka protocol | ✅ Message tracing | ❌ None | Add Kafka wire parser |
| AMQP protocol | ✅ RabbitMQ support | ❌ None | Add AMQP parser |
| gRPC/HTTP2 | ✅ Method + payload | ❌ None | Add HTTP2 frame parser |
| DNS monitoring | ✅ Query/response/latency | ❌ None | Add DNS packet parser |
| Network metrics | ✅ Retransmits, RTT, bytes | ⚠️ Connection count only | Enhance TCP stats |
| Auto distributed tracing | ✅ Header extraction | ❌ Requires SDK | Parse trace headers from packets |
| Request↔Response correlation | ✅ Via socket cookie | ❌ None | Track socket state |
| Service map from traffic | ✅ Automatic | ⚠️ From traces only | Derive from eBPF traffic |
| Continuous profiling | ✅ Always-on, low overhead | ✅ Have this | - |
| Dynamic logging | ✅ Add logs without redeploy | ❌ None | Add uprobe-based logging |

### The Pixie Magic: Protocol Parsing

Pixie's breakthrough is parsing application protocols at the kernel level:

```
┌─────────────────────────────────────────────────────────────────┐
│                     Application Process                          │
│  MySQL Client → query("SELECT * FROM users WHERE id=?")         │
└─────────────────────────────────────────────────────────────────┘
                              │ write() syscall
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        eBPF Probe                                │
│  1. Intercept write() syscall                                   │
│  2. Read buffer contents                                        │
│  3. Detect MySQL wire protocol (0x03 = COM_QUERY)              │
│  4. Parse query string                                          │
│  5. Extract: query, latency, error code, rows affected          │
│  6. Send to userspace                                           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
           "SELECT * FROM users WHERE id=123" (14ms, 1 row)
```

This gives you **database query visibility without touching the application**.

### Protocol Parsing Priority

| Priority | Protocol | Why | Complexity |
|----------|----------|-----|------------|
| P0 | MySQL | Most common DB, huge debugging value | Medium |
| P0 | PostgreSQL | Second most common, similar to MySQL | Medium |
| P1 | Redis | Cache debugging, often source of issues | Low |
| P1 | DNS | Latency source, often overlooked | Low |
| P1 | HTTP/2 + gRPC | Modern microservices standard | High |
| P2 | Kafka | Message flow visibility | Medium |
| P2 | MongoDB | NoSQL coverage | Medium |
| P3 | Cassandra | Enterprise NoSQL | Medium |
| P3 | AMQP | RabbitMQ support | Medium |

### Auto-Instrumented Distributed Tracing

Pixie's other breakthrough: **distributed tracing without SDKs**.

How it works:
1. eBPF intercepts HTTP requests
2. Parses headers looking for `traceparent`, `x-request-id`, `x-b3-traceid`
3. Correlates requests across services by matching trace IDs
4. Builds service graph from actual traffic

dogwatch requires OTLP SDK instrumentation. Pixie does it automatically.

### Implementation Approach

```
bpf/
├── tcpconnect.c     # Existing
├── http.c           # Existing (enhance)
├── ssl.c            # Existing (fix)
├── profile.c        # Existing
├── mysql.c          # NEW: MySQL wire protocol
├── postgres.c       # NEW: PostgreSQL wire protocol
├── redis.c          # NEW: RESP protocol
├── dns.c            # NEW: DNS packet parsing
├── http2.c          # NEW: HTTP/2 frame parsing
└── kafka.c          # NEW: Kafka wire protocol
```

Each protocol parser needs:
1. **Syscall interception** - Hook read/write/sendmsg/recvmsg
2. **Protocol detection** - Identify protocol from magic bytes
3. **State machine** - Track request→response correlation
4. **Data extraction** - Parse relevant fields
5. **Ringbuf emission** - Send to userspace

---

## HashiCorp Adjacencies

HashiCorp's stack (Consul, Vault, Boundary, Nomad) has significant overlap with observability. Here's what we could subsume or integrate.

### Consul → Service Catalog + Service Mesh Observability

**What Consul Does:**
- Service discovery and registration
- Health checking (HTTP, TCP, script, TTL)
- Service mesh via Envoy sidecars (Consul Connect)
- Key/value store
- Multi-datacenter federation

**What dogwatch could subsume:**

| Consul Feature | dogwatch Equivalent | Gap |
|----------------|--------------------|----|
| Service registration | Service Catalog | Need to build |
| Health checks | Synthetics | ✅ Already have |
| Service dependencies | Trace-based service map | ✅ Already have |
| DNS-based discovery | Could add DNS interface | New feature |
| Envoy metrics | eBPF captures sidecar traffic | ✅ Automatic |
| Intentions (authz) | Could add service-to-service policies | New feature |

**The opportunity:** If we build a proper Service Catalog with health checks, ownership, and dependencies, we provide 80% of Consul's value for the observability use case. Teams using dogwatch wouldn't need Consul just for service discovery.

**Integration path:**
1. Import services from Consul API
2. Consul health check failures → dogwatch incidents
3. Consul Connect metrics → dogwatch dashboards
4. Consul intentions → visualize allowed/denied traffic

### Boundary → Secure Access + Session Recording

**What Boundary Does:**
- Identity-based access to infrastructure
- Session recording (SSH, RDP, databases)
- Just-in-time access with time limits
- Credential injection (no shared passwords)
- Session audit logs

**What dogwatch could add:**

| Boundary Feature | dogwatch Opportunity |
|------------------|---------------------|
| Session recording | Record debugging sessions during incidents |
| Access audit | Track who accessed what during incident response |
| Just-in-time access | "Break glass" access for on-call responders |
| Credential brokering | Integrate with Vault for secrets |

**The opportunity:** During an incident, engineers need access to production systems. Boundary solves secure access. dogwatch could add:

1. **Incident-scoped access** - Grant access to specific systems only during active incident
2. **Session recording** - Record terminal sessions for post-mortem
3. **Access audit** - "Who touched what" during incident timeline
4. **Runbook integration** - Auto-grant access when runbook step requires it

This turns dogwatch from "observe and alert" to "observe, alert, and respond safely."

### Vault → Secrets Management

**Not subsume, but integrate:**
- Store notification channel credentials (PagerDuty API key, Slack webhook)
- Encrypt API keys at rest
- Dynamic credentials for database monitoring
- PKI for mTLS between dogwatch components

### Nomad → Orchestrator Telemetry

**What we have:** Kubernetes integration
**What we could add:** Nomad job/allocation monitoring

Same approach as K8s:
- Job status, health, allocations
- Resource utilization per allocation
- Deployment tracking
- Event stream

### The "Infrastructure Control Plane" Vision

```
┌─────────────────────────────────────────────────────────────────┐
│                         dogwatch                                 │
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │   Observe    │  │    Alert     │  │   Respond    │          │
│  │              │  │              │  │              │          │
│  │ • Metrics    │  │ • Watches    │  │ • Incidents  │          │
│  │ • Traces     │  │ • SLOs       │  │ • On-call    │          │
│  │ • Logs       │  │ • Anomaly    │  │ • Runbooks   │          │
│  │ • Profiles   │  │              │  │ • Access     │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                    Service Catalog                        │  │
│  │  • Ownership  • Dependencies  • Health  • Runbooks       │  │
│  └──────────────────────────────────────────────────────────┘  │
│                              │                                  │
│              ┌───────────────┼───────────────┐                  │
│              ▼               ▼               ▼                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐          │
│  │  Kubernetes  │  │    Nomad     │  │   Consul     │          │
│  │  (native)    │  │  (integrate) │  │  (subsume)   │          │
│  └──────────────┘  └──────────────┘  └──────────────┘          │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

The vision: **dogwatch becomes the single pane of glass** for infrastructure, subsuming the observability aspects of Consul while integrating with Vault and Boundary for secure incident response.

---

## Enterprise Features Roadmap

### Data Management
- [ ] PostgreSQL backend option
- [ ] Configurable retention policies
- [ ] Data export (S3, GCS)
- [ ] Prometheus remote write
- [ ] Backup/restore

### Scale & Performance
- [ ] Distributed rate limiting (Redis)
- [ ] Read replicas
- [ ] Time-series compression
- [ ] Cardinality management

### Security & Compliance
- [ ] Field-level encryption
- [ ] PII detection/redaction
- [ ] SOC2 audit logging
- [ ] RBAC audit reports

### Integration Ecosystem
- [ ] Terraform provider
- [ ] Helm charts
- [ ] VS Code extension
- [ ] GitHub/GitLab integration
- [ ] Jira integration

---

## The North Star

When an engineer gets paged at 3am, they should be able to:

1. **See the alert** with full context (what, where, severity)
2. **Understand why** via correlated traces/logs/metrics
3. **Know what changed** via deployment/config correlation
4. **Follow the runbook** with step-by-step guidance
5. **Fix it** with auto-remediation or clear instructions
6. **Verify the fix** with real-time feedback
7. **Document it** for the post-mortem

All without leaving dogwatch. All without manual correlation. All powered by eBPF magic that sees everything automatically.

---

## Development Priorities

### Phase 1: Foundation
1. Correlation engine (trace ↔ log ↔ metric linking)
2. Service catalog with ownership
3. Fix HTTPS/SSL eBPF tracing

### Phase 2: Intelligence
4. Change correlation (deploy → incident)
5. Error tracking (Sentry-like)
6. Protocol parsing (MySQL, PostgreSQL, Redis)

### Phase 3: Response
7. Runbooks and response automation
8. Investigation timeline
9. Post-mortem generation

### Phase 4: Enterprise
10. PostgreSQL backend
11. Advanced eBPF (DNS, gRPC, Kafka)
12. Terraform provider
