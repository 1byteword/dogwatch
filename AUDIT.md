# Dogwatch Architecture Audit

**Date:** 2026-02-17
**Scope:** Full codebase — Go backend, eBPF probes, SolidJS frontend, storage, alerting, self-hosted readiness

---

## Executive Summary

Dogwatch is an ambitious and impressively scoped single-binary observability platform. 422 API endpoints, eBPF probes, PromQL engine, alerting, incidents, on-call, SLOs, synthetics, service catalog, and a full SolidJS frontend — all in one binary. However, the architecture has **fundamental performance and reliability gaps** that would prevent it from competing with even modest production workloads against Prometheus, Loki, or Tempo, let alone Datadog.

**The core tension:** The codebase optimizes for *feature breadth* (compete on surface area) when it needs to optimize for *data path depth* (compete on throughput and reliability). The priority should flip.

### By the Numbers

| Metric | Value |
|--------|-------|
| Backend API endpoints | 422 |
| Frontend API calls (V2 UI) | 42 unique endpoints |
| Backend endpoints with no UI | ~380 |
| Go test files / total files | 72 / 332 (21.7%) |
| TODO/FIXME comments | 3 actionable |
| Markdown docs | 19 files |
| V2 widgets | 54 (100% V1 parity) |
| E2E tests | 67 Playwright specs |
| Separate SQLite databases | 30+ |

---

## Table of Contents

1. [Critical Issues](#1-critical-issues)
2. [Storage Architecture: SQLite as TSDB](#2-storage-architecture-sqlite-as-tsdb)
3. [eBPF Implementation](#3-ebpf-implementation)
4. [Performance Hot Paths](#4-performance-hot-paths)
5. [PromQL Engine](#5-promql-engine)
6. [Log Ingestion and Indexing](#6-log-ingestion-and-indexing)
7. [Trace Ingestion and Search](#7-trace-ingestion-and-search)
8. [Memory Management](#8-memory-management)
9. [Goroutine Management](#9-goroutine-management)
10. [Serialization](#10-serialization)
11. [Alerting and Reliability Stack](#11-alerting-and-reliability-stack)
12. [Self-Hosted / Hybrid Cloud Readiness](#12-self-hosted--hybrid-cloud-readiness)
13. [Frontend (V2 UI)](#13-frontend-v2-ui)
14. [Security](#14-security)
15. [Backend Endpoints With No UI](#15-backend-endpoints-with-no-ui)
16. [Incomplete CRUD in V2 UI](#16-incomplete-crud-in-v2-ui)
17. [Consolidated TODOs](#17-consolidated-todos)
18. [Throughput Comparison Table](#18-throughput-comparison-table)
19. [Recommendations](#19-recommendations)

---

## 1. Critical Issues

These block any production deployment.

### 1.1 Auth Middleware Not Globally Applied

**Location:** `internal/web/server.go` lines 167-450+

RBAC middleware is created but never wrapped around route handlers. ~80+ API data endpoints are accessible without authentication if called directly (bypassing the UI). The auth check happens manually inside some handlers via `getUserFromRequest(r)`, but many data endpoints skip it entirely.

**Impact:** All API data endpoints are accessible without authentication.

**Fix:** Apply auth middleware globally: `wrappedHandler := rbacMiddleware.Authenticate(mux)`. Whitelist public endpoints (health, OTLP ingest) explicitly.

### 1.2 Unchecked JSON Decode

**Location:** `internal/web/server.go:3911`

```go
json.NewDecoder(r.Body).Decode(&req)  // ERROR NOT CHECKED
if req.User == "" {
    req.User = "api"
}
```

Invalid JSON is silently ignored, processing continues with zero-value struct fields.

### 1.3 No Request Body Size Limits

**Location:** `internal/web/server.go` (throughout)

No `http.MaxBytesReader` on any endpoint. A single POST with a multi-GB body will OOM the process.

### 1.4 No TLS Support

The binary cannot serve HTTPS. All traffic — auth tokens, API keys, telemetry — travels in cleartext. Even with a reverse proxy, inter-node gossip HTTP traffic is unencrypted. Non-starter for any security review.

### 1.5 No Schema Migrations

30+ SQLite databases use `CREATE TABLE IF NOT EXISTS`. There is no version tracking, no `ALTER TABLE`, no rollback mechanism. Any column change in a future release silently fails on existing installations. **You cannot safely ship upgrades.**

### 1.6 No Telemetry-Level Multi-Tenancy

The org model exists in RBAC, but `metrics.db`, `logs.db`, and `traces.db` are global — no `org_id` column. One tenant's PromQL query returns another tenant's data.

---

## 2. Storage Architecture: SQLite as TSDB

### What's There

- 30+ separate SQLite databases, one per subsystem (metrics.db, traces.db, logs.db, incidents.db, etc.)
- Each store gets its own `*sql.DB` + `sync.RWMutex`
- Tags/attributes stored as JSON TEXT blobs
- Timestamps stored as RFC3339 strings (not integers)
- FTS5 for log search (good choice)
- A `TimeSeriesOptimizer` with batch buffering, downsampling rules, and tiering (hot/warm/cold) — **not wired into actual hot paths**

### Missing SQLite PRAGMAs

| PRAGMA | Current | Should Be | Impact |
|--------|---------|-----------|--------|
| `journal_mode` | DELETE (default) | WAL | 5-10x write throughput; readers don't block during writes |
| `busy_timeout` | 0 (default) | 5000+ | `SQLITE_BUSY` errors retry instead of crashing |
| `synchronous` | FULL (default) | NORMAL (with WAL) | 10x faster writes; safe with WAL |
| `cache_size` | default | -64000 (64MB) | Fewer disk reads for repeated queries |
| `mmap_size` | 0 | 268435456 (256MB) | Memory-mapped reads bypass read() syscall |
| `auto_vacuum` | NONE | INCREMENTAL | Database files don't grow to peak size forever |

**Note:** The `TimeSeriesOptimizer` (in `internal/storage/timeseries.go`) already enables WAL and mmap — but only for databases that explicitly opt in. The main `storage.go`, `logs/store.go`, and `trace/trace.go` stores do **not** opt in.

### Schema Design Issues

| Issue | Location | Impact |
|-------|----------|--------|
| Tags as JSON TEXT blobs | All stores | Label filtering = full table scan + `json_extract()`. Cannot use indexes. |
| Timestamps as RFC3339 strings | All stores | 5x slower comparisons, 3x larger indexes vs int64 epoch nanoseconds |
| No compound indexes on (name, timestamp) | metrics store | Every query does a full scan within the time range |
| UUID primary keys via `crypto/rand` | logs/store.go | Syscall per insert; ULIDs would be faster and sort chronologically |
| No `VACUUM` or `auto_vacuum` | All stores | Database files grow to peak size and never shrink |

### Scaling Estimates

| Workload | Current | With WAL + Batching | Prometheus / VictoriaMetrics |
|----------|---------|--------------------|----|
| Metrics write | ~200/sec | ~50K/sec | >1M/sec |
| Log write | ~500/sec | ~30K/sec (FTS5 bottleneck) | Loki: 100K-500K/sec |
| Span write | ~100/sec | ~5K/sec | Tempo: 100K/sec |

### Verdict

SQLite is a **defensible choice** for a single-binary tool — it eliminates external dependencies and makes backup/restore trivial. But the implementation leaves ~50-100x performance on the table through missing PRAGMAs, no batching, and JSON tag storage. The `TimeSeriesOptimizer` and `TieringManager` show correct architectural thinking but are disconnected from the hot paths.

**The biggest risk is the missing schema migration framework.** Without it, any schema change in a future release has no way to apply to existing installations.

---

## 3. eBPF Implementation

### What's There

- 5 BPF C programs compiled via `bpf2go` (cilium/ebpf), pre-compiled `.o` files checked in
- TCP connect (kprobe), HTTP tracing (tracepoint), DB protocol parsing (tracepoint), SSL/TLS (uprobe), CPU profiler (perf_event)
- CO-RE support via `vmlinux.h` for kernel struct portability
- Userspace protocol parsers for HTTP/1.x, MySQL, PostgreSQL, Redis
- Ring buffer-based event delivery (kernel 5.8+)

### Issues

| Issue | Impact | Severity |
|-------|--------|----------|
| **Ring buffer requires kernel 5.8+** but compat code advertises 4.4+ | Perf buffer fallback is dead code — BPF programs hardcode `BPF_MAP_TYPE_RINGBUF` | High |
| **x86_64 only** — no ARM64 `.o` files | Cannot run on AWS Graviton, Apple Silicon, or any ARM server | High |
| **IPv6 completely unsupported** | TCP connect probe filters `family != AF_INET` — drops all IPv6 connections | High |
| **No HTTP/2 or gRPC parsing** | Blind to modern microservice traffic | High |
| **No sendmsg/recvmsg/writev/readv hooks** | Misses Go's net package (uses sendmsg), nginx/HAProxy (vectored I/O) | High |
| **SSL probe V1 acknowledged broken** with cilium/ebpf; V2 is a fragile tracefs workaround | TLS traffic capture is unreliable | High |
| **Double syscall tracing** — HTTP + DB probes both hook the same 6 syscalls | 12 BPF program executions per read/write syscall system-wide | Medium |
| **No FD-to-socket mapping** — probes fire on ALL read/write, not just sockets | Wasted CPU examining file reads, pipe reads, etc. | Medium |
| **No TCP reassembly** | Fragmented messages are missed or truncated | Medium |
| **No container awareness** | No cgroup filtering, no container ID extraction, no K8s pod metadata | Medium |
| **`connKey()` uses `string(rune(pid))`** | PID hash collisions above Unicode max code point (1,114,111) — correctness bug | Medium |
| **`binary.Read` with reflection** in event parsing | 2-5x CPU overhead per event vs unsafe cast | Medium |
| **Small channel buffers (100-200) with silent drop** | Data loss under burst with no backpressure signal | Medium |
| **`PreparedStatementCache` goroutine leak** | No stop channel, runs forever after `DBProbe.Close()` | Low |
| **`intToIP` allocates `make([]byte, 4)` per call** | Unnecessary allocation in hot path | Low |

### Comparison to Competitors

| Capability | Dogwatch | Pixie | Datadog system-probe |
|------------|----------|-------|---------------------|
| Protocols | HTTP/1.x, MySQL, PG, Redis | HTTP/1.1, HTTP/2, gRPC, MySQL, PG, Cassandra, Redis, Kafka, DNS, NATS, AMQP | HTTP, HTTP/2, gRPC, TLS, DNS, Kafka, MySQL, PG, Redis, MongoDB |
| TLS interception | Broken/fragile | Robust (OpenSSL, GnuTLS, BoringSSL, Go) | Production-grade |
| TCP reassembly | None | Full | Full |
| Architecture | x86_64 only | x86_64 + ARM64 | x86_64 + ARM64 |
| IPv6 | No | Yes | Yes |
| Kernel compat | 5.8+ (claimed 4.4+) | 4.14+ | 4.4+ with fallbacks |
| Container-aware | No | Yes (K8s metadata) | Yes (K8s metadata) |
| Events/sec | ~50K-100K | ~1M | ~500K+ |

### Verdict

The eBPF implementation is a **reasonable MVP/prototype** — it demonstrates the zero-config value proposition and the protocol parsers are well-written. But it's 2-3 years behind Pixie and Datadog in maturity. Most impactful fixes: (1) add FD tracking to stop tracing non-network I/O, (2) hook sendmsg/recvmsg, (3) add ARM64, (4) fix IPv6.

---

## 4. Performance Hot Paths

### Metrics Ingestion Path

**Flow:** HTTP/eBPF events → `internal/ingest/listeners.go` (TCP/UDP) → Protocol parsers (Graphite/StatsD/OpenTSDB) → `MetricStore.WriteSamples()` → SQLite INSERT

| Issue | Location | Impact |
|-------|----------|--------|
| Lock held during I/O | `storage/storage.go:116-140` | `sync.RWMutex` write lock held for entire `db.Exec()` INSERT — all writes serialize on disk I/O |
| No batching in main storage path | `storage/storage.go` | Each metric is a separate `db.Exec()`. SQLite: ~60 inserts/sec individually vs 50K+/sec in a transaction |
| `json.Marshal(tags)` on every write | `custommetrics/store.go:230-235` | Allocates byte slice + encodes map per data point |
| `RecordBatch` holds lock for entire batch | `custommetrics/store.go:253-320` | Large batch blocks all readers |

### Trace Ingestion: The Worst Bottleneck

**Flow:** OTLP HTTP → `trace/otlp.go` → `io.ReadAll(body)` (no size limit) → `json.Unmarshal` → `convertSpan()` per span → `store.RecordSpan()` → SQLite INSERT + `updateTraceSummary()` (6 SQL queries per span)

| Issue | Location | Impact |
|-------|----------|--------|
| `io.ReadAll(req.Body)` with no size limit | `trace/otlp.go:90` | OOM vector — single large request kills the process |
| No batching in OTLP receiver | `trace/otlp.go:108-113` | Each span processed individually; batch info from OTLP discarded |
| Lock held for INSERT + summary update | `trace/trace.go:152-189` | Write lock held for 7 SQL operations per span |
| `updateTraceSummary` is O(spans_in_trace) per new span | `trace/trace.go:237-307` | Re-queries ALL spans in the trace on every new span. 50th span scans 49 rows. |
| `GetLatencyPercentile` loads all durations into memory | `trace/trace.go:591-637` | Sorts all trace durations in-memory. 1M traces = 8MB allocation + full sort. No t-digest or DDSketch. |
| `parseNanoTime` uses `fmt.Sscanf` | `trace/otlp.go:213` | ~10x slower than `strconv.ParseInt` due to format string parsing |

**At 1000 spans/sec:** 7,000 SQL ops/sec, all serialized on one mutex. **Estimated ceiling: ~100 spans/sec.** Tempo handles 100K spans/sec by writing to a WAL and computing summaries lazily on read.

### eBPF Event Processing

| Issue | Location | Impact |
|-------|----------|--------|
| `binary.Read` with reflection per event | `probe/probe.go:116` | 2-5x CPU overhead vs unsafe cast |
| `bytes.NewReader` allocated per event | `probe/probe.go:116` | Unnecessary allocation in hot path |
| `fmt.Sprintf("%d:%d", pid, tid)` per event | `probe/dbproto.go:219` | String allocation; struct key would be zero-alloc |
| Channel buffers of 100-200 with drop semantics | `probe/probe.go:131-135` | Silent data loss under burst; no backpressure |

---

## 5. PromQL Engine

**Flow:** HTTP request → `promql/engine.go` → `Parse(query)` → `Evaluator.Eval()` → `SQLMetricsStore.ScanRange()` → SQL query → JSON parse tags per row → Label matching in Go → Aggregation → JSON response

| Issue | Location | Impact |
|-------|----------|--------|
| **Range query re-evaluates at every step** | `promql/engine.go:62-104` | 24h query at 15s step = 5,760 separate evaluations, each re-parsing AST, re-querying SQLite. **60-120 seconds** vs Prometheus's ~500ms. |
| Full table scan with post-filter | `promql/translate.go:60-90` | ALL rows matching metric name + time range loaded into memory. Label matching happens in Go after materialization. |
| JSON parsing per row | `promql/translate.go:72-73` | `json.Unmarshal` per row creates new `map[string]string`. 10K rows = 10K map allocations + 10K JSON parses. |
| Regex compiled per row | `promql/translate.go:243-244` | `regexp.MatchString` internally calls `regexp.Compile` each time. 10K rows = 10K regex compilations. |
| No iterator pattern | All of promql/ | Evaluator returns fully materialized `Vector`/`Matrix`/`Scalar`. No streaming. 1M data points all in memory before processing. |
| Function allocations | `promql/functions.go` | Every vector function allocates new `Vector` slice. `copyLabels()` allocates new map. Chained functions (`rate(sum(metric[5m]))`) allocate at every layer. |

### Latency Estimates

| Query Type | Dogwatch | Prometheus |
|------------|----------|------------|
| Instant, 10 series | ~20ms | ~5ms |
| Instant, 1000 series | ~500ms | ~20ms |
| Range, 100 series, 1h | ~5-10 seconds | ~100ms |
| Range, 100 series, 24h | **60-120 seconds** | ~500ms |

---

## 6. Log Ingestion and Indexing

**Flow:** HTTP → `logs/store.go` → `Insert()` / `InsertBatch()` → SQLite INSERT into `log_entries` + FTS5 `log_entries_fts` → Pattern detection per entry

| Issue | Location | Impact |
|-------|----------|--------|
| FTS5 full-text search | logs/store.go | **Good choice.** FTS5 is an inverted index internally, O(log n) search. Correct approach. |
| UUID generation per entry via `crypto/rand` | logs/store.go | Syscall per insert. ULIDs would be faster and sort chronologically. |
| Pattern detection on every insert | logs/store.go | `patternDetector.Process()` runs synchronously in the hot path. Should be async. |
| Batch insert uses transaction | logs/store.go:194-241 | **Good.** Prepared statement inside transaction. But still generates UUIDs + pattern detection per entry. |
| Search does two queries | logs/store.go | `SELECT COUNT(*)` then `SELECT ... LIMIT/OFFSET`. COUNT scans entire result set just for total. Doubles query time. |
| Attributes stored as JSON | logs/store.go | Same overhead as metrics tags — marshal on write, unmarshal on read. |
| No bloom filters | — | No quick negative lookups. "Does this stream have entries in this time range?" always hits disk. |

### Throughput

| Mode | Dogwatch | Loki |
|------|----------|------|
| Individual inserts | ~200-500/sec | — |
| Batch inserts | ~10K-30K/sec | 100K-500K/sec |

---

## 7. Trace Ingestion and Search

(Covered in detail in Section 4 — Performance Hot Paths.)

Key summary: **~100 spans/sec** ceiling due to 7 SQL operations per span under a single mutex. Tempo handles 100K/sec. The `updateTraceSummary` O(n²) pattern is the single worst performance bug in the codebase.

---

## 8. Memory Management

### Infrastructure Built But Not Connected

| Component | Status | Would Fix |
|-----------|--------|-----------|
| `sync.Pool` for spans, logs, metrics, buffers | **Dead code** — hot paths don't use pools | GC pressure from per-event allocations |
| `StreamingBatcher[T]` | **Dead code** — not wired to storage layer | Individual INSERT overhead |
| `TimeSeriesOptimizer` (WAL + batch + downsample) | **Only used if explicitly called** | All write path issues |
| `MemoryManager.ShouldShed()` | **Not called** from ingestion paths | OOM under load |
| `BufferPool` for byte reuse | **Dead code** | JSON encoding allocations |

This is the most frustrating finding — the team clearly understands the performance problems and has written correct solutions, but hasn't plumbed them into the actual data paths.

### Dominant Allocators

1. `json.Marshal`/`json.Unmarshal` for tags/attributes on every write and read
2. Map allocations (`map[string]string`) for labels/tags — one per metric point, per log entry, per span
3. `fmt.Sprintf` / string concatenation in hot paths (probe events, query keys)

### Memory per 1M Events

| Data Type | In-Flight Memory | Prometheus/Loki/Tempo Equivalent |
|-----------|-----------------|----------------------------------|
| Metrics (1M points) | ~350 MB | 1-3 MB (compressed chunks) |
| Logs (1M entries) | ~500 MB | ~50-100 MB (snappy compressed) |
| Spans (1M spans) | ~1-2 GB | ~100-200 MB (Parquet columnar) |

---

## 9. Goroutine Management

| Issue | Location | Impact |
|-------|----------|--------|
| **Unbounded goroutine-per-connection** | `ingest/listeners.go:104-110` | TCP `acceptLoop()` spawns unbounded goroutines. Connection storm → millions of goroutines → OOM. |
| No worker pool pattern anywhere | 64 `go func` calls across 28 files | All unbounded goroutine-per-task or single background goroutines |
| Channel backpressure is drop-only | eBPF probes | `select { case ch <- event: default: }` — no feedback mechanism |
| No rate limiting on ingestion endpoints | OTLP receiver, Graphite/StatsD listeners | Combined with unbounded goroutines = amplification vector |
| WaitGroup usage correct | listeners | Proper `sync.WaitGroup` for graceful shutdown ✓ |

### Goroutine Estimates

| Load | Goroutines | Stack Memory |
|------|-----------|--------------|
| Idle | ~20-30 | ~512 KB |
| Moderate (1K connections) | ~1,030 | ~16 MB |
| Stress (10K connections) | ~10,020+ | ~80-160 MB |

---

## 10. Serialization

### JSON Everywhere

- Tags/labels: `json.Marshal(map[string]string)` on write, `json.Unmarshal` on read
- Log attributes: JSON TEXT column
- Span attributes: JSON TEXT column
- OTLP receiver: `json.Unmarshal` for entire request body
- API responses: `encoding/json` for all HTTP responses
- **No binary serialization** (protobuf, msgpack, CBOR) for any internal data path
- **No compression** — SQLite stores JSON as uncompressed TEXT

### Quantified Overhead

- `json.Marshal` for typical 5-tag set: ~500ns, ~256 bytes allocated
- `json.Unmarshal` for same: ~1.2µs, ~512 bytes allocated
- At 100K metrics/sec: ~50ms CPU/sec just for tag serialization, plus ~25 MB/sec transient allocations

### Comparison

| System | Storage Format | Bytes/Sample |
|--------|---------------|--------------|
| Dogwatch | JSON TEXT in SQLite | ~100-200 |
| Prometheus | Double-delta + XOR encoded chunks | ~1.37 |
| VictoriaMetrics | ZSTD compressed | ~0.4 |
| Loki | Snappy-compressed chunks with label dictionary | ~5-10 |
| Tempo | Parquet columnar + dictionary encoding + snappy/zstd | ~10-20 |

---

## 11. Alerting and Reliability Stack

### Three Independent Alerting Paths

| System | Evaluation | Real Notifications? | Creates Incidents? |
|--------|-----------|--------------------|--------------------|
| **Watch engine** (`internal/watch/`) | 30s interval, system metrics | Yes (Slack + webhook) | Yes |
| **Alerting evaluator** (`internal/alerting/`) | 15s interval, PromQL rules | **No — all receivers are stubs** | No |
| **Synthetics runner** (`internal/synthetics/`) | 5s poll, HTTP/TCP checks | **No — just logs** | No |

Plus **two independent escalation engines** (`incidents/pager.go` and `oncall/escalation.go`) with separate state, schemas, and notification channels.

### Issues

| Issue | Severity |
|-------|----------|
| **All alerting receivers are stubs** — log messages only, no real Slack/email/PagerDuty | P0 |
| **Synthetics notifications not implemented** | P0 |
| **All escalation state is in-memory** — lost on crash/restart | P0 |
| **No incident deduplication across restarts** — duplicates created | P0 |
| **No resolved notifications** sent from alerting evaluator | P1 |
| **Escalation timing math is wrong** (double-counts current level delay) | P1 |
| **Escalation repeat logic is mathematically broken** (integer division by `len` always 0) | P1 |
| **Watch recovery doesn't auto-resolve incidents** | P1 |
| **Hardcoded `localhost:9999`** in Slack notification buttons | P1 |
| **TCP synthetic check has no timeout** (nil context → hangs forever) | P1 |
| **DNS check type declared but not implemented** (falls through to HTTP) | P1 |

### Comparison to Prometheus Alertmanager

The alerting *model* is sophisticated — route trees, GroupBy/GroupWait/GroupInterval, silences, inhibitions, dependency-aware inhibition, multi-layer on-call schedules. This matches or exceeds Alertmanager in design. But the **implementation** is incomplete: stub receivers, in-memory state, no notification persistence, and mathematical bugs in escalation timing.

---

## 12. Self-Hosted / Hybrid Cloud Readiness

### Scorecard

| Area | Rating | Notes |
|------|--------|-------|
| **Air-gapped deployment** | Production-Ready | Zero outbound dependencies, embedded UI, no phone-home |
| **Backup/restore** | Production-Ready | Scheduled, S3 offload, retention policies, verification |
| **Resource limits** | Production-Ready | Memory manager with 3-tier pressure, load shedding, rate limiting |
| **Self-observability** | Production-Ready | Health/readiness probes, self-monitoring, memory metrics |
| **Configuration** | Needs Work | CLI flags not env-mappable, no config file, secrets visible in `ps aux` |
| **Clustering** | Needs Work | Gossip works (memberlist), but metadata-only — no data replication or cross-node query |
| **Multi-tenancy** | Needs Work | Org model in RBAC, but core telemetry stores have no org_id column |
| **Kubernetes** | Needs Work | Health probes exist, K8s collector works, but no Helm chart, no agent/server split |
| **TLS/mTLS** | **Missing** | Binary cannot serve HTTPS. All traffic is plaintext. |
| **Schema migrations** | **Missing** | No version tracking, no ALTER TABLE, no rollback. |

### The Three Production Blockers

1. **No TLS** — Auth tokens, API keys, and all telemetry travel in cleartext. Even with a reverse proxy, inter-node gossip is unencrypted.

2. **No schema migrations** — 30+ SQLite databases, all using `CREATE TABLE IF NOT EXISTS`. Any column change in a future version silently fails on existing installations.

3. **No telemetry-level multi-tenancy** — Metrics, logs, and traces are global. One tenant's query returns another tenant's data.

### Hybrid Cloud Potential

The architecture is **well-positioned** for hybrid cloud if the gaps are filled:

- **Edge/on-prem agent** → The single binary is perfect for air-gapped edge nodes. Add `--mode agent` that runs only eBPF probes + OTLP forwarding.
- **Central server** → Already exists. Add data replication from the cluster gossip layer.
- **Cloud control plane** → Federation layer could report metadata to a cloud dashboard without sending telemetry off-premises (key differentiator vs Datadog SaaS-only).
- **Tiered storage** → Hot/warm/cold architecture is already designed. Wire the S3 cold tier for long-term retention with the control plane managing cross-tier queries.

The biggest architectural decision: **whether telemetry data leaves the customer's infrastructure**. Dogwatch's single-binary DNA is a natural fit for "control plane in cloud, data plane on-prem" — exactly what Grafana Cloud's agent model does.

---

## 13. Frontend (V2 UI)

### Good

- SolidJS + TypeScript, 54 widgets, 7 dashboard templates
- Code-split routes with `lazy()` (45kB initial load)
- ErrorBoundary + loading states on DashboardsPage
- Auth guard with cookie-based sessions
- Keyboard accessibility (ARIA labels, focus-visible)
- Works without backend (mock data fallbacks)

### Issues

| Issue | Location | Severity |
|-------|----------|----------|
| **ErrorBoundary on only 1/20 route pages** | All route pages except DashboardsPage | High |
| **Mock data masks real failures** — 404/500 silently fall back to mock | `ui-v2/src/core/api.ts:11-26` | High |
| **Form labels not associated** — `<label>` without `htmlFor` | Multiple route pages | Medium |
| **`any` types in 4 components** — `children?: any` | RecordingRules, SloManagement, Monitors, SyntheticsManagement | Medium |
| **Unsafe type assertions** — `as DragEvent`, `as Record<>` | DashboardsPage, oncall/service, logs/service | Medium |
| **No loading states** for several async operations | ConfigureCatalog, ConfigureNotifications | Medium |
| **`localStorage` dashboard drafts have no schema versioning** | DashboardsPage | Low |
| **Low color contrast on placeholder text** | LoginPage (`rgba(138,138,138,0.5)` on dark bg) | Low |
| **Promise chains mix `.then()` and `await`** | `ui-v2/src/core/actions.ts` | Low |

---

## 14. Security

### Good

- Passwords: bcrypt cost 12 ✓
- Sessions: JWT with HMAC-SHA256 ✓
- API keys: `dw_` prefix, hashed storage ✓
- RBAC: Owner > Admin > Editor > Viewer ✓
- CSRF: Token-based protection enabled ✓
- Headers: CSP, X-Frame-Options, X-Content-Type-Options ✓
- SQL: All queries use parameterized statements — no injection risk ✓
- No hardcoded secrets found ✓
- Rate limiting implemented ✓
- CORS with origin validation ✓

### Gaps

| Issue | Severity |
|-------|----------|
| Auth middleware not globally applied (see Section 1.1) | Critical |
| No TLS (see Section 1.4) | Critical |
| No request body size limits (see Section 1.3) | High |
| CSRF skip-paths list is manual and growing (`security.go:64-78`) | Medium |
| `io.ReadAll` with no size limit on OTLP receiver | High |
| Unbounded goroutines per TCP connection (DoS vector) | High |
| CSP allows `unsafe-inline` for styles (SolidJS runtime) | Low |

---

## 15. Backend Endpoints With No UI

These backend features are fully implemented but have no V2 UI pages:

| Feature | Endpoints | Count |
|---------|-----------|-------|
| Security & Threat Detection | `/api/security/*` | 7 |
| Compliance Reports | `/api/compliance/*` | 3 |
| Status Pages management | `/api/statuspages/*` | 6 |
| Knowledge Base | `/api/knowledge/*` | 5 |
| Lookout (anomaly overview) | `/api/lookout/*` | 2 |
| Entity Explorer | `/api/entities/*` | 5 |
| PII Detection | `/api/pii/*` | 7 |
| SIEM Export | `/api/siem/*` | 11 |
| Sampling Config | `/api/sampling/*` | 18 |
| Data Shaping Rules | `/api/shaping/*` | 5 |
| Quotas & Chargeback | `/api/quotas/*` + `/api/chargeback/*` | 8 |
| Migration Wizard | `/api/migration/*` | 14 |
| Scripts Engine | `/api/scripts/*` | 3 |
| Storage Management | `/api/storage/*` | 11 |
| eBPF Probe Management | `/api/probes/*` | 6 |
| Log Field Extraction (Grok) | `/api/logs/extraction/*` | 12 |
| Advanced Kubernetes views | `/api/k8s/nodes,deployments,...` | 12 |
| User & Team Management | `/api/users/*`, `/api/teams/*` | 10 |
| Cost deep-dive | `/api/cost/*` | 12 |
| Advanced Cardinality Control | `/api/cardinality/controller/*` | 4 |

**Total: ~380 endpoints with no UI.**

---

## 16. Incomplete CRUD in V2 UI

| Resource | Has | Missing |
|----------|-----|---------|
| Notification Channels | Create, Read, Test | Update, Delete |
| Catalog Services | Create, Read | Update, Delete |
| OnCall Schedules | Create, Read | Update, Delete |
| Recording Rules | Read | Create, Update, Delete |
| Saved Queries | Read | Create, Update, Delete |
| SLOs | List | Detail view, CRUD |
| Synthetics | List | Detail view, CRUD |
| Watch Rules | List | Detail, Update, Delete |
| Incidents | Create, Read, Ack/Resolve | Delete |
| Alerts | Read, Ack/Silence | Delete alert rules |

---

## 17. Consolidated TODOs

### From Code Comments

| TODO | Location |
|------|----------|
| "Implement proper symbol resolution" | `internal/probe/profile.go:326` |
| "Support multiple join conditions" | `internal/query/parser.go:1415` |
| "Integrate with watch.Notifier" | `internal/synthetics/runner.go:334` |

### From Markdown Files

| Item | Source | Status |
|------|--------|--------|
| Release gating (CI perf budgets, golden-flow E2E) | TODO.md | Phase 5 — in progress |
| BubbleUp root cause analysis (frontend + API wiring) | PLAN.md | Backend partial, UI not started |
| CI workflow push (needs workflow-scoped token) | ci.yml | Blocked |
| Unified notification service (7 channels) | PLAN_NOTIFICATIONS.md | Designed, not started |
| BubbleUp UI page + results display | PLAN.md Task 4 | Not started |
| Dashboard Folders | 07-v1-parity-and-prod-hump.md | Not started |

### Explicitly Deferred (Do Not Build Yet)

| Item | Source |
|------|--------|
| HA/clustering | CLAUDE.md |
| Wasm plugins | CLAUDE.md |
| LLM/AI features | CLAUDE.md |
| Terraform provider | CLAUDE.md |
| Custom ML models | CLAUDE.md |
| Multi-tenancy | CLAUDE.md |
| Pluggable storage backends | CLAUDE.md |

### Competitive Gaps (from VISION.md)

| Gap | Impact |
|-----|--------|
| TLS/HTTP2/gRPC eBPF parsing — blind to encrypted microservice traffic | Critical |
| Error Tracking — no Sentry-style grouping/fingerprinting | High |
| LogQL / TraceQL — Grafana users expect these query languages | High |
| Real User Monitoring (RUM) — no browser-side JS SDK | Medium |
| Cloud cost integration (AWS/GCP/Azure billing APIs) | Medium |
| Exemplars (Prometheus standard — trace_id on metric points) | Medium |
| Full-text log search (BM25) — Splunk's core feature | Medium |
| OpenAPI/Swagger spec for 422 endpoints | Low |
| Helm chart / K8s operator | Low |
| Data at rest encryption (SOC2/HIPAA) | Low |
| GitOps config sync | Low |
| Additional ingest receivers (Fluent Forward, Jaeger, Zipkin, Syslog, Loki, StatsD) | Low |

---

## 18. Throughput Comparison Table

### Write Path

| Subsystem | Dogwatch Current | Dogwatch Optimized* | Industry Leader |
|-----------|-----------------|--------------------|----|
| Metrics ingestion | ~200/sec | ~50K/sec | VictoriaMetrics: 800K/sec |
| Log ingestion | ~500/sec | ~30K/sec | Loki: 100K-500K/sec |
| Trace ingestion | ~100 spans/sec | ~10K spans/sec | Tempo: 100K/sec |
| eBPF event processing | ~50K events/sec | ~200K events/sec | Pixie: 1M/sec |

### Query Path

| Query Type | Dogwatch | Prometheus |
|------------|----------|------------|
| PromQL instant, 10 series | ~20ms | ~5ms |
| PromQL instant, 1000 series | ~500ms | ~20ms |
| PromQL range, 100 series, 1h | ~5-10s | ~100ms |
| PromQL range, 100 series, 24h | ~60-120s | ~500ms |

### Storage Efficiency

| System | Bytes per Metric Sample |
|--------|------------------------|
| Dogwatch (JSON in SQLite) | ~100-200 |
| Prometheus (chunks) | ~1.37 |
| VictoriaMetrics (ZSTD) | ~0.4 |

*Optimized = WAL mode + batching + iterator pattern + tag interning + pool integration. No architectural changes (still SQLite single-binary).

---

## 19. Recommendations

### Tier 0: Must Fix (Blocks Any Production Use)

1. **Enable WAL mode + busy_timeout + synchronous=NORMAL** on all 30+ SQLite databases.
   - Single biggest throughput improvement (~10-50x)
   - ~20 lines of code total
   - Risk: None with WAL

2. **Add TLS support** (`--tls-cert`, `--tls-key` flags, `ListenAndServeTLS`).
   - Non-negotiable for any deployment handling credentials
   - ~50 lines of code

3. **Add schema migration framework.**
   - Version-track each database, apply ALTER TABLE on upgrade
   - Without this, you can never ship a schema change safely
   - ~200 lines of code for a simple version-table approach

4. **Wire the existing `StreamingBatcher` and `sync.Pool`** into actual storage paths.
   - The code is already written — it just needs plumbing
   - ~100 lines of integration code

5. **Apply auth middleware globally** to all `/api/*` routes.
   - ~10 lines — wrap the mux

6. **Add `http.MaxBytesReader`** to all handlers accepting request bodies.
   - ~20 lines in a middleware

### Tier 1: Must Fix (Blocks Reliability Claims)

7. **Replace alerting receiver stubs** with real implementations (at minimum: webhook + Slack).
   - The unified notification service plan (PLAN_NOTIFICATIONS.md) is the right approach.

8. **Fix trace `RecordSpan` N+1** — batch inserts, compute summaries incrementally or lazily on read.

9. **Fix PromQL range query** — fetch data once, iterate with an iterator pattern instead of re-evaluating per step.

10. **Persist escalation state** to SQLite. In-memory state loss on restart is unacceptable for an on-call system.

11. **Add incident deduplication** with a dedup key (hash of source + labels) persisted in the database.

12. **Compile PromQL regex matchers once per query**, not per row.

### Tier 2: Competitive Parity

13. Add ARM64 eBPF support (compile `.o` files for aarch64).
14. Add IPv6 to TCP connect probe.
15. Hook sendmsg/recvmsg/writev/readv in eBPF probes.
16. Add HTTP/2 + gRPC protocol parsing.
17. Add FD-to-socket tracking to avoid tracing non-network I/O.
18. Implement tag interning — replace JSON text blobs with integer label IDs for indexed filtering.
19. Add ErrorBoundary to all 19 route pages missing it.
20. Increase Go test coverage (21% → 50%+).

### Tier 3: Differentiation

21. Hybrid cloud agent mode (`--mode agent` for eBPF + OTLP forwarding only).
22. Cross-node query federation (query metrics from any node via any node).
23. Multi-window SLO burn rate alerting.
24. Helm chart + K8s operator.
25. LogQL / TraceQL query support.
26. Error tracking with Sentry-style grouping.

---

## Bottom Line

**Is the architecture sound?** The *design* is sound — separate databases per concern, modular stores, gossip-based federation, tiered storage, memory pressure management. The *implementation* has critical gaps: missing PRAGMAs that leave 10-50x throughput on the table, in-memory state that should be persisted, optimizer code that's written but unwired, and stub receivers in the alerting path.

**Is it competitive with Datadog/Grafana?** Not on performance — current throughput is 100-1000x below Prometheus for metrics and Tempo for traces. But the feature surface area is genuinely impressive for a single binary, and the zero-config eBPF story is compelling. With Tier 0 and Tier 1 fixes, it could credibly compete for small-to-medium deployments (< 50K metrics/sec, < 5K spans/sec).

**Is it ready for self-hosted production?** No. TLS, schema migrations, and notification delivery are hard blockers. But the air-gapped story is excellent, backup/restore is solid, and the resource management infrastructure is sophisticated. The hybrid cloud potential is real — the architecture naturally supports a "data stays on-prem, control plane in cloud" model that differentiates from Datadog's SaaS-only approach.

**What should happen next?** Tier 0 items 1-6 are roughly a week of focused work and would transform the system from "impressive demo" to "usable for early adopters." The most impactful single change is enabling WAL mode — one PRAGMA per database, ~10-50x write throughput improvement, zero risk.
