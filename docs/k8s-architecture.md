# Kubernetes Architecture: eBPF-Native Observability

## Problem Statement

dogwatch is a single-binary observability platform powered by eBPF. On a single host, it works out of the box — drop the binary, see everything. But Kubernetes clusters have many nodes, each with its own kernel. eBPF probes are per-kernel, so a single pod only sees traffic on the node it runs on.

The goal: deploy one DaemonSet, get **logs, traces, metrics, and profiling** across the entire cluster — automatically correlated — without touching application code.

---

## Why Not Integrate With Tetragon?

Tetragon (Cilium) already runs eBPF on every node and handles kernel compatibility, process lifecycle, network connections, file access, and security events. It exports via gRPC, JSON, and OTLP.

However, Tetragon is a **data source**, not an observability platform. It does not do wire protocol parsing (HTTP, MySQL, PostgreSQL, Redis), distributed trace assembly, metrics aggregation, PromQL, dashboards, alerting, or cross-signal correlation. It also does not capture application logs.

More importantly, dogwatch's value is being the **single thing you deploy**. Depending on Tetragon means requiring Cilium, which not every cluster runs. The goal is: install the dogwatch DaemonSet, get complete observability. No prerequisites.

Tetragon integration can be offered as an optional data source for clusters that already run it, but it is not the primary architecture.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│  Node                                                           │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  Linux Kernel (eBPF)                                      │  │
│  │                                                           │  │
│  │  Network probes ──────────► trace_events ring buffer      │  │
│  │   tcp_connect, tcp_accept,    (independent, 8MB)          │  │
│  │   tcp_sendmsg, tcp_close                                  │  │
│  │   HTTP/MySQL/PG/Redis parsing                             │  │
│  │                                                           │  │
│  │  Syscall probes ──────────► log_events ring buffer        │  │
│  │   sys_write (fd=1,2 only)     (independent, 16MB)         │  │
│  │   in-kernel level filtering                               │  │
│  │   in-kernel sampling                                      │  │
│  │                                                           │  │
│  │  Perf events ─────────────► profile_events ring buffer    │  │
│  │   CPU sampling at 100Hz       (independent, 4MB)          │  │
│  │                                                           │  │
│  │  Per-CPU hash maps ───────► request_metrics               │  │
│  │   counters, latency sums      (aggregated in-kernel)      │  │
│  │   no event stream needed                                  │  │
│  │                                                           │  │
│  │  TID → active span map ───► log-trace correlation         │  │
│  │   updated by network probes   (read by syscall probes)    │  │
│  │   enables auto-stitching                                  │  │
│  │                                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│         │              │              │              │           │
│  ┌──────┴──────────────┴──────────────┴──────────────┴───────┐  │
│  │  dogwatch agent (userspace, ~50MB RAM)                     │  │
│  │                                                           │  │
│  │  Goroutine: drain trace_events ──────┐                    │  │
│  │  Goroutine: drain log_events ────────┤  gRPC stream       │  │
│  │  Goroutine: read metric maps ────────┤  to central        │  │
│  │  Goroutine: drain profile_events ────┤  server            │  │
│  │  Goroutine: K8s API watch ───────────┘  (enrichment)      │  │
│  │                                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
         │
         │ gRPC (per-signal streams)
         ▼
┌─────────────────────────────────────────────────────────────────┐
│  dogwatch server (central, single Deployment)                   │
│                                                                 │
│  ├── Trace receiver → span assembly → trace storage             │
│  ├── Log receiver → indexing → log storage                      │
│  ├── Metric receiver → time-series storage                      │
│  ├── Profile receiver → flamegraph storage                      │
│  ├── Correlation engine (cross-signal linking)                  │
│  ├── PromQL engine                                              │
│  ├── Alerting engine                                            │
│  ├── Web UI                                                     │
│  └── API server                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## What eBPF Captures Per Signal

### Traces (network probes)

eBPF hooks into TCP lifecycle and parses wire protocols to produce distributed traces with zero instrumentation.

| Hook | Data Captured |
|---|---|
| `tcp_connect` | Outbound connections: dest IP, port, process, container |
| `inet_csk_accept` | Inbound connections: source IP, port, process, container |
| `tcp_sendmsg` / `tcp_recvmsg` | Payload bytes for protocol parsing |
| `tcp_close` | Connection duration, bytes transferred |
| SSL `SSL_read` / `SSL_write` uprobes | Decrypted payload for TLS traffic |

Protocol parsers extract structured data from the wire:

| Protocol | Fields Extracted |
|---|---|
| HTTP/1.1 | Method, path, status, headers, latency |
| MySQL | Query text, latency, rows affected, error code |
| PostgreSQL | Query text, latency, rows, error |
| Redis | Command, key, latency |
| gRPC/HTTP2 | Method, status, latency (planned) |
| DNS | Query, response, latency (planned) |

Cross-service trace assembly uses trace header parsing (W3C traceparent, B3, X-Request-ID) when available, and falls back to socket-to-process-to-socket correlation and timing-based correlation when headers are absent.

### Logs (syscall probes)

Application logs are captured by hooking the `write()` syscall for stdout/stderr.

```c
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write(struct trace_event_raw_sys_enter *ctx) {
    int fd = (int)ctx->args[0];

    // Exit immediately for non-stdout/stderr writes (~10ns, 99.9% of calls)
    if (fd != 1 && fd != 2) return 0;

    // Verify this is a container process (check PID namespace)
    // Read buffer contents
    // Look up TID in active span map for trace correlation
    // Apply in-kernel filtering (level, sampling rate)
    // Submit to log_events ring buffer
}
```

**In-kernel filtering** reduces volume before events reach userspace:
- Parse the first bytes of the log line to detect level (ERROR, WARN, INFO, DEBUG)
- Always keep ERROR and WARN
- Sample INFO at a configurable rate
- Sample or drop DEBUG at a higher rate
- Rates are adjustable at runtime by updating an eBPF map from userspace

**Performance characteristics:**
- Non-stdout/stderr write() calls: ~10ns overhead (one comparison, immediate return)
- stdout/stderr writes: ~100-500ns overhead (buffer read, map lookup, ring buffer submit)
- For reference, Linux XDP processes millions of packets per second through eBPF. 50K log lines/sec is trivial.

### Metrics (in-kernel aggregation)

Golden signal metrics are derived directly from traced requests using per-CPU hash maps. No event stream is needed — the agent periodically reads the maps.

```c
struct metric_key {
    u32 service_hash;
    u32 method_hash;
    u16 status;
};

struct metric_val {
    u64 count;
    u64 sum_latency_ns;
    u64 min_latency_ns;
    u64 max_latency_ns;
};

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, 10000);
    __type(key, struct metric_key);
    __type(value, struct metric_val);
} request_metrics SEC(".maps");
```

Per-CPU maps have zero lock contention. Each CPU core updates its own copy. Userspace sums across CPUs when reading.

Additional metric sources (collected in userspace by the agent):
- **kubelet/cAdvisor scrape**: per-container CPU, memory, network, disk
- **Prometheus auto-scrape**: auto-discover `/metrics` endpoints via pod annotations
- **Local OTLP/StatsD receiver**: for apps that explicitly emit custom metrics

### Profiling (perf events)

CPU profiling uses `perf_event_open` with eBPF to sample stack traces at 100Hz across all processes.

```c
SEC("perf_event")
int profile_cpu(struct bpf_perf_event_data *ctx) {
    u32 pid = bpf_get_current_pid_tgid() >> 32;
    int kernel_stack_id = bpf_get_stackid(ctx, &stack_traces, 0);
    int user_stack_id = bpf_get_stackid(ctx, &stack_traces, BPF_F_USER_STACK);

    struct profile_event event = {
        .pid = pid,
        .kernel_stack_id = kernel_stack_id,
        .user_stack_id = user_stack_id,
        .timestamp = bpf_ktime_get_ns(),
    };

    bpf_ringbuf_output(&profile_events, &event, sizeof(event), 0);
    return 0;
}
```

Symbol resolution happens in userspace (agent or server) by reading `/proc/<pid>/maps` and ELF symbol tables.

---

## Novel Technique: Automatic Log-Trace Correlation

This is the key differentiator. No other tool does this without application-level instrumentation.

### How It Works

The network probes already track which thread is handling which inbound request:

```c
// When an HTTP request is received, record the active span for this thread
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 65536);
    __type(key, u64);              // PID << 32 | TID
    __type(value, struct span_ref); // trace_id + span_id
} tid_to_span SEC(".maps");

// Network probe: on HTTP request received
SEC("kprobe/tcp_recvmsg")
int trace_recv(struct pt_regs *ctx) {
    // ... parse HTTP request, generate span ...

    u64 tid_key = bpf_get_current_pid_tgid();
    struct span_ref ref = { .trace_id = trace_id, .span_id = span_id };
    bpf_map_update_elem(&tid_to_span, &tid_key, &ref, BPF_ANY);
}
```

When the same thread writes a log line to stdout, the write() probe looks up the active span:

```c
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write(struct trace_event_raw_sys_enter *ctx) {
    int fd = (int)ctx->args[0];
    if (fd != 1 && fd != 2) return 0;

    u64 tid_key = bpf_get_current_pid_tgid();

    // Cross-reference: is this thread handling a traced request?
    struct span_ref *ref = bpf_map_lookup_elem(&tid_to_span, &tid_key);

    struct log_event event = {
        .pid = tid_key >> 32,
        .tid = (u32)tid_key,
        .timestamp = bpf_ktime_get_ns(),
        .has_span = (ref != NULL),
    };

    if (ref) {
        event.trace_id = ref->trace_id;
        event.span_id = ref->span_id;
    }

    // Read log line content
    bpf_probe_read_user(event.buf, sizeof(event.buf), (void *)ctx->args[1]);

    bpf_ringbuf_output(&log_events, &event, sizeof(event), 0);
    return 0;
}
```

### Result

```
Application writes:
  log.Info("payment processed", "user_id", 1234, "amount", 99.99)

dogwatch captures:
  {
    log_line:  "INFO payment processed user_id=1234 amount=99.99",
    trace_id:  "abc123def456...",
    span_id:   "789ghi...",
    container: "payment-svc-7f8d9",
    pod:       "payment-svc-abc123",
    namespace: "production",
    timestamp: 1707472345123456789  // kernel clock, no app clock skew
  }
```

The log line is automatically linked to the trace that was being handled when it was written. No trace ID injection in the application. No log SDK. No configuration.

### Comparison

| Tool | Log-Trace Correlation Method | App Changes Required |
|---|---|---|
| Datadog | dd-trace SDK injects trace ID into log context | Yes — SDK integration |
| OpenTelemetry | Log bridge API + trace context propagation | Yes — code changes |
| Grafana | Manual LogQL → TraceQL linking by time/labels | No, but manual correlation |
| **dogwatch** | **eBPF TID→span map, automatic in-kernel** | **None** |

---

## Resource Management: eBPF Is the Orchestrator

A common concern with a unified agent is resource contention between signals. eBPF addresses this natively.

### Independent Ring Buffers

Each signal gets its own ring buffer. A log storm fills the log ring buffer and drops log events. Trace and metric collection continues unaffected on their own buffers.

```c
// Separate ring buffers — independent backpressure per signal
struct { __uint(type, BPF_MAP_TYPE_RINGBUF); __uint(max_entries, 16 * 1024 * 1024); } log_events     SEC(".maps");
struct { __uint(type, BPF_MAP_TYPE_RINGBUF); __uint(max_entries,  8 * 1024 * 1024); } trace_events   SEC(".maps");
struct { __uint(type, BPF_MAP_TYPE_RINGBUF); __uint(max_entries,  4 * 1024 * 1024); } profile_events SEC(".maps");
```

The kernel manages buffer lifecycle, memory allocation, and producer-consumer synchronization. No userspace orchestrator needed.

### In-Kernel Filtering and Sampling

Volume control happens before events cross the kernel-userspace boundary.

```c
// Configurable sampling rates — updated from userspace at runtime
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 4);          // One per log level
    __type(key, u32);
    __type(value, u32);              // Sample rate: 100 = keep all, 1 = keep 1%
} log_sample_rates SEC(".maps");

// In the write() probe:
u32 level = detect_log_level(buf);
u32 *rate = bpf_map_lookup_elem(&log_sample_rates, &level);
if (rate && bpf_get_prandom_u32() % 100 >= *rate) {
    return 0;  // Dropped in kernel — zero userspace cost
}
```

The userspace agent can adjust sampling rates at runtime by writing to the eBPF map. Under memory pressure, reduce log sampling. When load drops, increase it. The control loop runs in userspace but the filtering executes in the kernel at line speed.

### Per-CPU Maps for Metrics

Metric aggregation uses `BPF_MAP_TYPE_PERCPU_HASH`, which maintains separate counters per CPU core with zero lock contention. The agent periodically reads and sums across CPUs. No event stream, no ring buffer, no backpressure concern.

### Overhead Budget

| Component | Overhead | Notes |
|---|---|---|
| write() probe (non-stdout) | ~10ns | One fd comparison, immediate return. 99.9% of calls. |
| write() probe (stdout/stderr) | ~100-500ns | Buffer read + map lookup + ring buffer submit |
| Network probes (per connection event) | ~200-500ns | Protocol parsing is the expensive part |
| Perf sampling (100Hz) | ~1-2% CPU | Configurable frequency |
| Per-CPU metric maps | Negligible | Map updates are O(1) |
| **Total per-node overhead** | **< 2% CPU, ~50MB RAM** | Dominated by profiling sample rate |

For context, Linux XDP (eBPF in the network stack) processes millions of packets per second. The event rates for observability are orders of magnitude lower.

---

## Deployment Model

### Single Binary, Two Modes

The same `dogwatch` binary runs in both modes:

```bash
# Single-host mode (default, current behavior)
./dogwatch

# Kubernetes agent mode
./dogwatch --mode=agent --server=dogwatch-server:4317

# Kubernetes server mode
./dogwatch --mode=server
```

### Kubernetes Manifests

**Agent (DaemonSet):**

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dogwatch-agent
  namespace: dogwatch
spec:
  selector:
    matchLabels:
      app: dogwatch-agent
  template:
    metadata:
      labels:
        app: dogwatch-agent
    spec:
      hostPID: true
      hostNetwork: true
      containers:
      - name: agent
        image: dogwatch:latest
        args: ["--mode=agent", "--server=dogwatch-server:4317"]
        securityContext:
          privileged: true
        resources:
          requests:
            cpu: 100m
            memory: 64Mi
          limits:
            cpu: 500m
            memory: 128Mi
        volumeMounts:
        - name: sys
          mountPath: /sys
          readOnly: true
        - name: proc
          mountPath: /host/proc
          readOnly: true
        - name: bpf
          mountPath: /sys/fs/bpf
      volumes:
      - name: sys
        hostPath:
          path: /sys
      - name: proc
        hostPath:
          path: /proc
      - name: bpf
        hostPath:
          path: /sys/fs/bpf
```

**Server (Deployment):**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dogwatch-server
  namespace: dogwatch
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dogwatch-server
  template:
    metadata:
      labels:
        app: dogwatch-server
    spec:
      containers:
      - name: server
        image: dogwatch:latest
        args: ["--mode=server"]
        ports:
        - containerPort: 9999   # Web UI + API
          name: http
        - containerPort: 4317   # Agent gRPC + OTLP gRPC
          name: grpc
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        volumeMounts:
        - name: data
          mountPath: /var/lib/dogwatch
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: dogwatch-data
```

### Scaling Characteristics

| Cluster Size | Agent RAM/Node | Server Requirements | Notes |
|---|---|---|---|
| < 10 nodes | ~50MB | 1 replica, 512MB RAM | SQLite is fine |
| 10-50 nodes | ~50MB | 1 replica, 1-2GB RAM | Increase storage IOPS |
| 50-100 nodes | ~50-100MB | 1-2 replicas, 2-4GB RAM | Consider per-signal write sharding |
| 100+ nodes | ~50-100MB | Multiple replicas | HA/clustering required (future work) |

The per-node agent cost is constant regardless of cluster size. The central server is the scaling constraint, and that is a separate design problem (see "Do NOT Build Yet: HA/clustering").

---

## What This Gives Users

Deploy one DaemonSet and one Deployment. Get:

- **Traces**: distributed traces across all services, auto-assembled from eBPF network probes, zero SDK
- **Logs**: application logs captured from stdout/stderr via eBPF write() interception
- **Metrics**: golden signals derived from traced requests (in-kernel aggregation) + kubelet/Prometheus scrape
- **Profiling**: continuous CPU flamegraphs via perf event sampling
- **Auto-correlation**: logs automatically linked to traces via eBPF TID→span mapping
- **Service map**: auto-discovered from traced network connections
- **Security events**: process execution, unusual network, sensitive file access from eBPF

No Prometheus. No Fluentd. No Jaeger. No OTel Collector. No Datadog Agent. One tool.
