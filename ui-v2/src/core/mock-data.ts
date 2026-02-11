// Centralized mock data for demo mode.
// When no backend is available, api.ts falls back here.
// All data tells a coherent story: an e-commerce platform under moderate load
// with an active checkout-latency incident being triaged.

const SERVICES = [
  "checkout-api",
  "order-worker",
  "auth-service",
  "payment-gateway",
  "inventory-api",
  "frontend-bff",
];

const SEC = 1_000;
const MIN = 60_000;
const HR = 3_600_000;

function ago(ms: number): string {
  return new Date(Date.now() - ms).toISOString();
}

function systemHistory(): unknown[] {
  const now = Date.now();
  return Array.from({ length: 60 }, (_, i) => {
    const t = now - (59 - i) * MIN;
    const phase = (i / 60) * Math.PI * 4;
    return {
      timestamp: new Date(t).toISOString(),
      cpu_percent: +(30 + 15 * Math.sin(phase) + Math.random() * 5).toFixed(1),
      mem_percent: +(55 + 10 * Math.sin(phase * 0.7) + Math.random() * 3).toFixed(1),
      load_1: +(1.2 + 0.8 * Math.sin(phase)).toFixed(2),
      disk_read_bytes: Math.floor(1e6 + 5e5 * Math.sin(phase)),
      disk_write_bytes: Math.floor(5e5 + 3e5 * Math.sin(phase * 1.3)),
      net_rx_bytes: Math.floor(2e6 + 1e6 * Math.sin(phase * 0.9)),
      net_tx_bytes: Math.floor(1.5e6 + 8e5 * Math.sin(phase * 1.1)),
    };
  });
}

// --- Static route map (query params stripped before lookup) ---

const routes: Record<string, () => unknown> = {

  // ── ops / system ──────────────────────────────────────────────

  "/api/system": () => ({
    timestamp: new Date().toISOString(),
    cpu_usage_percent: 34.2,
    mem_usage_percent: 61.5,
    disk_read_per_sec: 1_200_000,
    disk_write_per_sec: 800_000,
    net_rx_per_sec: 2_500_000,
    net_tx_per_sec: 1_800_000,
    load_1: 1.82,
    load_5: 1.45,
    load_15: 1.21,
  }),

  "/api/history/system": systemHistory,

  "/api/stats": () => ({
    total_connections: 247,
    total_requests: 18_432,
    total_errors: 312,
    endpoints: [
      { method: "POST", path: "/api/checkout", request_count: 4521, error_count: 89, error_rate: 1.97, p99_ms: 850, avg_ms: 230 },
      { method: "GET", path: "/api/products", request_count: 8932, error_count: 12, error_rate: 0.13, p99_ms: 45, avg_ms: 18 },
      { method: "POST", path: "/api/auth/login", request_count: 2341, error_count: 156, error_rate: 6.67, p99_ms: 320, avg_ms: 85 },
      { method: "GET", path: "/api/inventory", request_count: 6210, error_count: 8, error_rate: 0.13, p99_ms: 62, avg_ms: 24 },
      { method: "POST", path: "/api/orders", request_count: 3180, error_count: 47, error_rate: 1.48, p99_ms: 420, avg_ms: 145 },
    ],
    connections: [
      { process: "checkout-api", pid: 1234, remote: "10.0.1.50", port: 5432, count: 32 },
      { process: "order-worker", pid: 1235, remote: "10.0.1.50", port: 5432, count: 16 },
      { process: "auth-service", pid: 1236, remote: "10.0.2.10", port: 6379, count: 48 },
      { process: "payment-gateway", pid: 1237, remote: "10.0.3.20", port: 443, count: 24 },
      { process: "inventory-api", pid: 1238, remote: "10.0.1.50", port: 5432, count: 12 },
      { process: "frontend-bff", pid: 1239, remote: "10.0.1.100", port: 8080, count: 64 },
    ],
  }),

  "/api/processes": () => [
    { pid: 1234, name: "checkout-api", cpu_pct: 12.3, mem_mb: 256, state: "running", threads: 24 },
    { pid: 1235, name: "order-worker", cpu_pct: 8.1, mem_mb: 192, state: "running", threads: 16 },
    { pid: 1236, name: "auth-service", cpu_pct: 3.2, mem_mb: 128, state: "running", threads: 8 },
    { pid: 1237, name: "payment-gateway", cpu_pct: 5.7, mem_mb: 384, state: "running", threads: 12 },
    { pid: 1238, name: "inventory-api", cpu_pct: 2.8, mem_mb: 96, state: "running", threads: 6 },
    { pid: 1239, name: "frontend-bff", cpu_pct: 6.4, mem_mb: 320, state: "running", threads: 20 },
    { pid: 4210, name: "postgres", cpu_pct: 15.8, mem_mb: 1024, state: "running", threads: 48 },
    { pid: 4310, name: "redis", cpu_pct: 1.2, mem_mb: 64, state: "running", threads: 4 },
  ],

  "/api/servicemap": () => ({
    nodes: [
      { id: "frontend-bff", name: "frontend-bff", type: "service" },
      { id: "checkout-api", name: "checkout-api", type: "service" },
      { id: "order-worker", name: "order-worker", type: "worker" },
      { id: "auth-service", name: "auth-service", type: "service" },
      { id: "payment-gateway", name: "payment-gateway", type: "service" },
      { id: "inventory-api", name: "inventory-api", type: "service" },
    ],
    links: [
      { source: "frontend-bff", target: "checkout-api", count: 4521 },
      { source: "frontend-bff", target: "auth-service", count: 2341 },
      { source: "frontend-bff", target: "inventory-api", count: 6210 },
      { source: "checkout-api", target: "payment-gateway", count: 3180 },
      { source: "checkout-api", target: "order-worker", count: 3180 },
      { source: "checkout-api", target: "inventory-api", count: 2100 },
      { source: "order-worker", target: "payment-gateway", count: 1500 },
      { source: "auth-service", target: "inventory-api", count: 800 },
    ],
  }),

  // ── traces ────────────────────────────────────────────────────

  "/api/traces": () => ({
    data: [
      { trace_id: "abc123def456", service_name: "checkout-api", name: "POST /checkout", duration_ms: 842, span_count: 12, status: "ERROR" },
      { trace_id: "bcd234efg567", service_name: "frontend-bff", name: "GET /products", duration_ms: 45, span_count: 3, status: "OK" },
      { trace_id: "cde345fgh678", service_name: "auth-service", name: "POST /auth/login", duration_ms: 312, span_count: 5, status: "OK" },
      { trace_id: "def456ghi789", service_name: "order-worker", name: "process-order", duration_ms: 1250, span_count: 8, status: "OK" },
      { trace_id: "efg567hij890", service_name: "payment-gateway", name: "POST /charge", duration_ms: 420, span_count: 4, status: "OK" },
      { trace_id: "fgh678ijk901", service_name: "checkout-api", name: "POST /checkout", duration_ms: 1580, span_count: 15, status: "ERROR" },
      { trace_id: "ghi789jkl012", service_name: "inventory-api", name: "GET /stock", duration_ms: 28, span_count: 2, status: "OK" },
      { trace_id: "hij890klm123", service_name: "frontend-bff", name: "GET /cart", duration_ms: 156, span_count: 6, status: "OK" },
      { trace_id: "ijk901lmn234", service_name: "checkout-api", name: "POST /checkout", duration_ms: 920, span_count: 11, status: "ERROR" },
      { trace_id: "jkl012mno345", service_name: "auth-service", name: "POST /auth/refresh", duration_ms: 18, span_count: 2, status: "OK" },
    ],
  }),

  "/api/trace/services": () => SERVICES,

  "/api/trace/dependencies": () => [
    { parent: "frontend-bff", child: "checkout-api", call_count: 4521 },
    { parent: "frontend-bff", child: "auth-service", call_count: 2341 },
    { parent: "checkout-api", child: "payment-gateway", call_count: 3180 },
    { parent: "checkout-api", child: "order-worker", call_count: 3180 },
    { parent: "order-worker", child: "payment-gateway", call_count: 1500 },
  ],

  // ── alerts ────────────────────────────────────────────────────
  // Returns AlertingApiAlert[] shape (rule_name, labels, annotations)

  "/api/alerting/alerts": () => [
    {
      id: "alert-1", rule_name: "Checkout P99 Latency > 500ms", severity: "critical", state: "firing",
      starts_at: ago(23 * MIN),
      labels: { service: "checkout-api", deploy: "v2.14.1" },
      annotations: { summary: "Checkout P99 Latency > 500ms", description: "P99 latency at 842ms, threshold 500ms", root_cause: "Likely caused by deploy v2.14.1 — new payment validation logic" },
      value: 23,
    },
    {
      id: "alert-2", rule_name: "Error Rate > 5%", severity: "high", state: "firing",
      starts_at: ago(18 * MIN),
      labels: { service: "auth-service", deploy: "v1.8.0" },
      annotations: { summary: "Error Rate > 5%", description: "Login error rate at 6.67%", root_cause: "Redis connection pool exhaustion under load" },
      value: 156,
    },
    {
      id: "alert-3", rule_name: "Memory Usage > 80%", severity: "medium", state: "firing",
      starts_at: ago(45 * MIN),
      labels: { service: "payment-gateway", deploy: "v3.2.0" },
      annotations: { summary: "Memory Usage > 80%", description: "Memory at 82% and climbing", root_cause: "Suspected memory leak in payment retry logic" },
      value: 5,
    },
    {
      id: "alert-4", rule_name: "Pod CrashLoopBackOff", severity: "high", state: "firing",
      starts_at: ago(12 * MIN),
      labels: { service: "order-worker", deploy: "v2.14.1" },
      annotations: { summary: "Pod CrashLoopBackOff", description: "order-worker-7d8f9 restarting every 30s", root_cause: "OOMKilled — memory limit 256Mi too low for new batch size" },
      value: 8,
    },
    {
      id: "alert-5", rule_name: "SLO Budget < 20%", severity: "medium", state: "pending",
      starts_at: ago(2 * HR),
      labels: { service: "checkout-api", deploy: "v2.14.1" },
      annotations: { summary: "SLO Budget < 20%", description: "Checkout availability SLO burning fast", root_cause: "Cascading from checkout latency incident" },
      value: 2,
    },
  ],

  "/api/watches": () => [
    { id: "w-1", name: "Checkout Latency Watch", query: "histogram_quantile(0.99, http_request_duration_bucket{service=\"checkout-api\"})", condition: "> 500ms for 5m", enabled: true, last_evaluated: ago(30_000), service: "checkout-api" },
    { id: "w-2", name: "Error Rate Watch", query: "rate(http_requests_total{status=~\"5..\"}[5m]) / rate(http_requests_total[5m])", condition: "> 0.05 for 3m", enabled: true, last_evaluated: ago(30_000), service: "" },
    { id: "w-3", name: "Memory Pressure", query: "container_memory_usage_bytes / container_spec_memory_limit_bytes", condition: "> 0.8 for 10m", enabled: true, last_evaluated: ago(60_000), service: "payment-gateway" },
    { id: "w-4", name: "Pod Restart Rate", query: "increase(kube_pod_container_status_restarts_total[1h])", condition: "> 3", enabled: false, last_evaluated: ago(5 * MIN), service: "" },
  ],

  "/api/alerting/silences": () => [
    { id: "s-1", matchers: "service=inventory-api, severity=low", created_by: "alice@example.com", starts_at: ago(4 * HR), ends_at: ago(-2 * HR), comment: "Planned maintenance window" },
    { id: "s-2", matchers: "alertname=DiskUsageHigh, host=worker-03", created_by: "bob@example.com", starts_at: ago(1 * HR), ends_at: ago(-23 * HR), comment: "Disk expansion in progress" },
  ],

  // ── incidents ─────────────────────────────────────────────────

  "/api/incidents": () => [
    {
      id: "inc-1", title: "Checkout latency spike affecting 12% of orders",
      severity: "critical", status: "triggered", service: "checkout-api",
      commander: "Alice Chen", responders: ["Alice Chen", "Bob Kumar", "Carol Wu"],
      startedAt: ago(23 * MIN), startedAtRaw: ago(23 * MIN),
      timeline: [
        { id: "t-1", time: ago(23 * MIN), kind: "alert", summary: "P99 latency exceeded 500ms threshold" },
        { id: "t-2", time: ago(20 * MIN), kind: "deploy", summary: "Deploy v2.14.1 rolled out 35min before spike" },
        { id: "t-3", time: ago(15 * MIN), kind: "status", summary: "Incident acknowledged by Alice Chen" },
        { id: "t-4", time: ago(8 * MIN), kind: "note", summary: "Identified new payment validation adding 400ms per request" },
      ],
    },
    {
      id: "inc-2", title: "Auth service elevated error rate",
      severity: "high", status: "acknowledged", service: "auth-service",
      commander: "Bob Kumar", responders: ["Bob Kumar", "Dana Lee"],
      startedAt: ago(18 * MIN), startedAtRaw: ago(18 * MIN),
      timeline: [
        { id: "t-5", time: ago(18 * MIN), kind: "alert", summary: "Login error rate exceeded 5%" },
        { id: "t-6", time: ago(14 * MIN), kind: "status", summary: "Investigating Redis connection pool" },
      ],
    },
  ],

  // ── logs ───────────────────────────────────────────────────────

  "/api/logs": () => [
    { id: "log-0", timestamp: ago(15 * MIN), level: "error", service: "checkout-api", message: "payment validation timeout after 800ms" },
    { id: "log-1", timestamp: ago(14 * MIN), level: "error", service: "checkout-api", message: "failed to process checkout: context deadline exceeded" },
    { id: "log-2", timestamp: ago(13 * MIN), level: "error", service: "auth-service", message: "redis connection refused: connection pool exhausted" },
    { id: "log-3", timestamp: ago(12 * MIN), level: "warn", service: "payment-gateway", message: "retry attempt 3/5 for charge txn-4892" },
    { id: "log-4", timestamp: ago(11 * MIN), level: "warn", service: "order-worker", message: "OOMKilled: memory limit 256Mi exceeded" },
    { id: "log-5", timestamp: ago(10 * MIN), level: "warn", service: "checkout-api", message: "slow query detected: SELECT * FROM carts WHERE... (450ms)" },
    { id: "log-6", timestamp: ago(9 * MIN), level: "info", service: "frontend-bff", message: "request completed: GET /products 200 18ms" },
    { id: "log-7", timestamp: ago(8 * MIN), level: "info", service: "inventory-api", message: "stock check completed for SKU-1234" },
    { id: "log-8", timestamp: ago(7 * MIN), level: "info", service: "auth-service", message: "user login successful: user_id=u-789" },
    { id: "log-9", timestamp: ago(6 * MIN), level: "info", service: "checkout-api", message: "order created: order_id=ord-5678" },
    { id: "log-10", timestamp: ago(5.5 * MIN), level: "info", service: "frontend-bff", message: "session started: session_id=sess-abc" },
    { id: "log-11", timestamp: ago(5 * MIN), level: "debug", service: "inventory-api", message: "cache hit for product catalog query" },
    { id: "log-12", timestamp: ago(4.5 * MIN), level: "error", service: "checkout-api", message: "payment gateway returned 503: service temporarily unavailable" },
    { id: "log-13", timestamp: ago(4 * MIN), level: "warn", service: "auth-service", message: "token refresh rate limit approaching threshold" },
    { id: "log-14", timestamp: ago(3.5 * MIN), level: "info", service: "payment-gateway", message: "charge succeeded: amount=$149.99 txn=txn-4891" },
    { id: "log-15", timestamp: ago(3 * MIN), level: "info", service: "order-worker", message: "order fulfillment queued: order_id=ord-5677" },
    { id: "log-16", timestamp: ago(2.5 * MIN), level: "debug", service: "frontend-bff", message: "CDN cache miss for /assets/bundle.js" },
    { id: "log-17", timestamp: ago(2 * MIN), level: "error", service: "auth-service", message: "failed to validate JWT: token expired" },
    { id: "log-18", timestamp: ago(1.5 * MIN), level: "info", service: "checkout-api", message: "cart updated: user_id=u-456, items=3" },
    { id: "log-19", timestamp: ago(1 * MIN), level: "warn", service: "payment-gateway", message: "high memory usage detected: 82% of limit" },
  ],

  "/api/logs/services": () => SERVICES,

  "/api/logs/patterns": () => [
    { id: "p-1", pattern: "payment validation timeout after {duration}", count: 89, level: "error", services: ["checkout-api"], sample: "payment validation timeout after 800ms" },
    { id: "p-2", pattern: "redis connection refused: {reason}", count: 156, level: "error", services: ["auth-service"], sample: "redis connection refused: connection pool exhausted" },
    { id: "p-3", pattern: "request completed: {method} {path} {status} {duration}", count: 8932, level: "info", services: ["frontend-bff", "checkout-api"], sample: "request completed: GET /products 200 18ms" },
    { id: "p-4", pattern: "retry attempt {n}/{max} for {operation}", count: 47, level: "warn", services: ["payment-gateway"], sample: "retry attempt 3/5 for charge txn-4892" },
    { id: "p-5", pattern: "OOMKilled: memory limit {limit} exceeded", count: 8, level: "warn", services: ["order-worker"], sample: "OOMKilled: memory limit 256Mi exceeded" },
  ],

  "/api/logs/patterns/trending": () => [
    { id: "tp-1", pattern: "payment validation timeout after {duration}", count: 89, growthPercent: 340, level: "error", firstSeen: ago(25 * MIN) },
    { id: "tp-2", pattern: "redis connection refused: {reason}", count: 156, growthPercent: 220, level: "error", firstSeen: ago(20 * MIN) },
    { id: "tp-3", pattern: "retry attempt {n}/{max} for {operation}", count: 47, growthPercent: 180, level: "warn", firstSeen: ago(22 * MIN) },
    { id: "tp-4", pattern: "slow query detected: {query}", count: 23, growthPercent: 150, level: "warn", firstSeen: ago(30 * MIN) },
  ],

  "/api/logs/compare": () => ({
    beforeEntries: [
      { id: "lc-1", timestamp: ago(2 * HR), level: "info", service: "checkout-api", message: "request completed: POST /checkout 200 120ms" },
      { id: "lc-2", timestamp: ago(2 * HR + 5000), level: "info", service: "checkout-api", message: "order created: order_id=ord-5670" },
    ],
    afterEntries: [
      { id: "lc-3", timestamp: ago(10 * MIN), level: "error", service: "checkout-api", message: "payment validation timeout after 800ms" },
      { id: "lc-4", timestamp: ago(8 * MIN), level: "error", service: "checkout-api", message: "failed to process checkout: context deadline exceeded" },
    ],
    addedPatterns: ["payment validation timeout after {duration}", "context deadline exceeded"],
    removedPatterns: [],
  }),

  // ── catalog ───────────────────────────────────────────────────

  "/api/catalog/services/stats": () => ({
    total: 6, critical: 1, high: 2, medium: 2, low: 1,
    healthy: 4, degraded: 1, unhealthy: 1,
  }),

  "/api/catalog/services": () =>
    SERVICES.map((name, i) => ({
      id: `svc-${i}`,
      name,
      displayName: name.replace(/-/g, " ").replace(/\b\w/g, (c: string) => c.toUpperCase()),
      description: `${name} microservice`,
      tier: (["critical", "high", "high", "medium", "medium", "low"] as const)[i],
      health: (["degraded", "healthy", "unhealthy", "healthy", "healthy", "healthy"] as const)[i],
      lifecycle: "production",
      teamName: ["Platform", "Platform", "Identity", "Payments", "Commerce", "Frontend"][i],
      ownerEmail: `team-${name}@example.com`,
      slackChannel: `#${name}`,
      repoUrl: `https://github.com/acme/${name}`,
      docsUrl: `https://docs.internal/${name}`,
      runbookUrl: `https://runbooks.internal/${name}`,
      incidentCount30d: [3, 1, 2, 0, 0, 1][i],
      uptimePercent30d: [99.2, 99.9, 98.5, 99.95, 99.99, 99.7][i],
      avgResponseTimeMs: [230, 145, 85, 62, 24, 42][i],
    })),

  // ── correlation ───────────────────────────────────────────────

  "/api/correlate/deploy-incidents": () => [
    {
      id: "corr-1", confidence: 0.92,
      reason: "Deploy v2.14.1 preceded checkout latency spike by 35 minutes",
      timeDeltaMs: 35 * MIN,
      deployment: { id: "dep-1", service: "checkout-api", version: "v2.14.1", timestamp: ago(58 * MIN) },
    },
    {
      id: "corr-2", confidence: 0.67,
      reason: "Deploy v1.8.0 preceded auth error rate increase by 2 hours",
      timeDeltaMs: 2 * HR,
      deployment: { id: "dep-2", service: "auth-service", version: "v1.8.0", timestamp: ago(2.3 * HR) },
    },
  ],

  // ── oncall ────────────────────────────────────────────────────
  // Returns raw API shape (layers/rotations) that the oncall service maps

  "/api/oncall/schedules": () => [
    { id: "sched-1", name: "Platform On-Call", description: "Primary platform team rotation", timezone: "America/New_York", teams: ["Platform"], layers: [{ users: [{ id: "u-1", name: "Alice Chen" }, { id: "u-2", name: "Bob Kumar" }] }] },
    { id: "sched-2", name: "Payments On-Call", description: "Payment systems rotation", timezone: "America/Los_Angeles", teams: ["Payments"], layers: [{ users: [{ id: "u-3", name: "Carol Wu" }, { id: "u-4", name: "Dana Lee" }] }] },
  ],

  "/api/oncall/policies": () => [
    { id: "pol-1", name: "Critical Alert Escalation", description: "Escalate critical alerts immediately", rules: [{}, {}, {}], repeat_enabled: true },
    { id: "pol-2", name: "Standard Escalation", description: "Standard 15-minute escalation", rules: [{}, {}], repeat_enabled: false },
  ],

  // ── kubernetes ────────────────────────────────────────────────

  "/api/k8s/summary": () => ({
    nodes: 3, nodesReady: 3, namespaces: 4,
    pods: 18, podsRunning: 15, podsPending: 2, podsFailed: 1,
    deployments: 6, deploymentsHealthy: 5, services: 8, warningEvents: 4,
  }),

  "/api/k8s/namespaces": () => [
    { name: "default" }, { name: "production" }, { name: "monitoring" }, { name: "kube-system" },
  ],

  "/api/k8s/pods": () => [
    { name: "checkout-api-7d8f9b6c4-x2k9m", namespace: "production", status: "Running", nodeName: "node-1", restartCount: 0, createdAt: ago(2 * HR) },
    { name: "checkout-api-7d8f9b6c4-p3j8n", namespace: "production", status: "Running", nodeName: "node-2", restartCount: 0, createdAt: ago(2 * HR) },
    { name: "order-worker-5c6d7e8f9-q4r5s", namespace: "production", status: "CrashLoopBackOff", nodeName: "node-1", restartCount: 8, createdAt: ago(1 * HR) },
    { name: "auth-service-3a4b5c6d7-t6u7v", namespace: "production", status: "Running", nodeName: "node-2", restartCount: 1, createdAt: ago(4 * HR) },
    { name: "payment-gateway-8e9f0a1b2-w8x9y", namespace: "production", status: "Running", nodeName: "node-3", restartCount: 0, createdAt: ago(6 * HR) },
    { name: "inventory-api-2b3c4d5e6-z0a1b", namespace: "production", status: "Running", nodeName: "node-1", restartCount: 0, createdAt: ago(12 * HR) },
    { name: "frontend-bff-6f7g8h9i0-c2d3e", namespace: "production", status: "Running", nodeName: "node-3", restartCount: 0, createdAt: ago(8 * HR) },
    { name: "prometheus-0", namespace: "monitoring", status: "Running", nodeName: "node-2", restartCount: 0, createdAt: ago(24 * HR) },
  ],

  "/api/k8s/deployments": () => [
    { name: "checkout-api", namespace: "production", status: "Available", replicas: 2, readyReplicas: 2, updatedReplicas: 2 },
    { name: "order-worker", namespace: "production", status: "Progressing", replicas: 2, readyReplicas: 1, updatedReplicas: 2 },
    { name: "auth-service", namespace: "production", status: "Available", replicas: 1, readyReplicas: 1, updatedReplicas: 1 },
    { name: "payment-gateway", namespace: "production", status: "Available", replicas: 1, readyReplicas: 1, updatedReplicas: 1 },
    { name: "inventory-api", namespace: "production", status: "Available", replicas: 1, readyReplicas: 1, updatedReplicas: 1 },
  ],

  "/api/k8s/events": () => [
    { name: "evt-1", namespace: "production", type: "Warning", reason: "OOMKilled", message: "Container order-worker exceeded memory limit", objectKind: "Pod", objectName: "order-worker-5c6d7e8f9-q4r5s", lastTimestamp: ago(2 * MIN) },
    { name: "evt-2", namespace: "production", type: "Warning", reason: "BackOff", message: "Back-off restarting failed container", objectKind: "Pod", objectName: "order-worker-5c6d7e8f9-q4r5s", lastTimestamp: ago(1 * MIN) },
    { name: "evt-3", namespace: "production", type: "Normal", reason: "Pulled", message: "Successfully pulled image checkout-api:v2.14.1", objectKind: "Pod", objectName: "checkout-api-7d8f9b6c4-x2k9m", lastTimestamp: ago(2 * HR) },
    { name: "evt-4", namespace: "production", type: "Normal", reason: "Scaled", message: "Scaled up replica set checkout-api-7d8f9b6c4 to 2", objectKind: "Deployment", objectName: "checkout-api", lastTimestamp: ago(2 * HR) },
    { name: "evt-5", namespace: "monitoring", type: "Normal", reason: "Started", message: "Started container prometheus", objectKind: "Pod", objectName: "prometheus-0", lastTimestamp: ago(24 * HR) },
    { name: "evt-6", namespace: "production", type: "Warning", reason: "FailedScheduling", message: "0/3 nodes are available: insufficient memory", objectKind: "Pod", objectName: "order-worker-5c6d7e8f9-m2n3o", lastTimestamp: ago(5 * MIN) },
  ],

  "/api/k8s/containers": () => [
    { name: "checkout-api", podName: "checkout-api-7d8f9b6c4-x2k9m", namespace: "production", image: "acme/checkout-api:v2.14.1", status: "Running", restartCount: 0, ready: true },
    { name: "checkout-api", podName: "checkout-api-7d8f9b6c4-p3j8n", namespace: "production", image: "acme/checkout-api:v2.14.1", status: "Running", restartCount: 0, ready: true },
    { name: "order-worker", podName: "order-worker-5c6d7e8f9-q4r5s", namespace: "production", image: "acme/order-worker:v2.14.1", status: "CrashLoopBackOff", restartCount: 8, ready: false },
    { name: "auth-service", podName: "auth-service-3a4b5c6d7-t6u7v", namespace: "production", image: "acme/auth-service:v1.8.0", status: "Running", restartCount: 1, ready: true },
    { name: "payment-gateway", podName: "payment-gateway-8e9f0a1b2-w8x9y", namespace: "production", image: "acme/payment-gateway:v3.2.0", status: "Running", restartCount: 0, ready: true },
    { name: "inventory-api", podName: "inventory-api-2b3c4d5e6-z0a1b", namespace: "production", image: "acme/inventory-api:v1.5.3", status: "Running", restartCount: 0, ready: true },
  ],

  "/api/k8s/services": () => [
    { name: "checkout-api", namespace: "production", type: "ClusterIP", clusterIP: "10.96.1.10", endpointCount: 2 },
    { name: "order-worker", namespace: "production", type: "ClusterIP", clusterIP: "10.96.1.11", endpointCount: 1 },
    { name: "auth-service", namespace: "production", type: "ClusterIP", clusterIP: "10.96.1.12", endpointCount: 1 },
    { name: "payment-gateway", namespace: "production", type: "ClusterIP", clusterIP: "10.96.1.13", endpointCount: 1 },
    { name: "inventory-api", namespace: "production", type: "ClusterIP", clusterIP: "10.96.1.14", endpointCount: 1 },
    { name: "frontend-bff", namespace: "production", type: "LoadBalancer", clusterIP: "10.96.1.15", endpointCount: 1 },
    { name: "prometheus", namespace: "monitoring", type: "ClusterIP", clusterIP: "10.96.2.10", endpointCount: 1 },
    { name: "kubernetes", namespace: "default", type: "ClusterIP", clusterIP: "10.96.0.1", endpointCount: 1 },
  ],

  // ── notify ────────────────────────────────────────────────────

  "/api/notify/channels": () => [
    { id: "ch-1", name: "#incidents-critical", type: "slack", enabled: true, successRate: 99.2, lastError: "", updatedAt: ago(5 * MIN) },
    { id: "ch-2", name: "PagerDuty", type: "pagerduty", enabled: true, successRate: 100, lastError: "", updatedAt: ago(10 * MIN) },
    { id: "ch-3", name: "ops-team@example.com", type: "email", enabled: true, successRate: 97.5, lastError: "SMTP timeout", updatedAt: ago(2 * HR) },
  ],

  "/api/notify/history": () => [
    { id: "n-1", channelName: "#incidents-critical", channelType: "slack", status: "delivered", title: "Checkout P99 Latency > 500ms", sentAt: ago(23 * MIN), responseTimeMs: 120 },
    { id: "n-2", channelName: "PagerDuty", channelType: "pagerduty", status: "delivered", title: "Checkout P99 Latency > 500ms", sentAt: ago(23 * MIN), responseTimeMs: 340 },
    { id: "n-3", channelName: "#incidents-critical", channelType: "slack", status: "delivered", title: "Error Rate > 5% (auth-service)", sentAt: ago(18 * MIN), responseTimeMs: 95 },
    { id: "n-4", channelName: "PagerDuty", channelType: "pagerduty", status: "delivered", title: "Error Rate > 5% (auth-service)", sentAt: ago(18 * MIN), responseTimeMs: 280 },
    { id: "n-5", channelName: "ops-team@example.com", channelType: "email", status: "failed", title: "Error Rate > 5% (auth-service)", sentAt: ago(18 * MIN), responseTimeMs: 5000 },
    { id: "n-6", channelName: "#incidents-critical", channelType: "slack", status: "delivered", title: "Memory Usage > 80% (payment-gateway)", sentAt: ago(45 * MIN), responseTimeMs: 110 },
    { id: "n-7", channelName: "PagerDuty", channelType: "pagerduty", status: "delivered", title: "Pod CrashLoopBackOff (order-worker)", sentAt: ago(12 * MIN), responseTimeMs: 295 },
    { id: "n-8", channelName: "ops-team@example.com", channelType: "email", status: "delivered", title: "Memory Usage > 80% (payment-gateway)", sentAt: ago(44 * MIN), responseTimeMs: 1200 },
  ],

  // ── slo / synthetics ──────────────────────────────────────────

  "/api/slos": () => [
    { id: "slo-1", name: "Checkout Availability", service: "checkout-api", target: 99.9, current: 99.2, budgetRemaining: 14.3, burnRate: 4.2, window: "30d", status: "at_risk" },
    { id: "slo-2", name: "API Latency P99 < 200ms", service: "frontend-bff", target: 99.5, current: 99.8, budgetRemaining: 72.1, burnRate: 0.4, window: "30d", status: "met" },
    { id: "slo-3", name: "Payment Success Rate", service: "payment-gateway", target: 99.95, current: 99.91, budgetRemaining: 45.0, burnRate: 1.1, window: "30d", status: "met" },
    { id: "slo-4", name: "Order Processing Time < 5s", service: "order-worker", target: 99.0, current: 97.8, budgetRemaining: 0, burnRate: 12.5, window: "7d", status: "breached" },
  ],

  "/api/synthetics/checks": () => [
    { id: "syn-1", name: "Homepage Load", url: "https://shop.example.com", type: "http", interval: 60, status: "passing", lastRun: ago(45_000), uptimePercent: 99.98, avgLatencyMs: 142 },
    { id: "syn-2", name: "Checkout Flow", url: "https://shop.example.com/checkout", type: "browser", interval: 300, status: "degraded", lastRun: ago(2 * MIN), uptimePercent: 98.5, avgLatencyMs: 2800 },
    { id: "syn-3", name: "API Health", url: "https://api.example.com/health", type: "http", interval: 30, status: "passing", lastRun: ago(20_000), uptimePercent: 99.99, avgLatencyMs: 12 },
    { id: "syn-4", name: "Payment API", url: "https://api.example.com/payments/health", type: "http", interval: 60, status: "passing", lastRun: ago(50_000), uptimePercent: 99.95, avgLatencyMs: 48 },
  ],

  "/api/synthetics/failures": () => [
    { checkId: "syn-2", checkName: "Checkout Flow", timestamp: ago(8 * MIN), statusCode: 0, latencyMs: 30000, error: "Timeout: page did not complete checkout within 30s" },
    { checkId: "syn-2", checkName: "Checkout Flow", timestamp: ago(13 * MIN), statusCode: 500, latencyMs: 4200, error: "Payment step returned HTTP 500" },
    { checkId: "syn-1", checkName: "Homepage Load", timestamp: ago(6 * HR), statusCode: 503, latencyMs: 0, error: "Service Unavailable during deployment" },
  ],

  // ── cost / cardinality ────────────────────────────────────────

  "/api/cost/estimate": () => ({
    totalMonthly: 2847,
    datadogEquivalent: 18500,
    savingsPercent: 84.6,
    breakdown: [
      { category: "Infrastructure Metrics", amount: 890, unit: "custom metrics" },
      { category: "APM & Traces", amount: 1200, unit: "spans/month" },
      { category: "Log Management", amount: 450, unit: "GB ingested" },
      { category: "Synthetics", amount: 180, unit: "test runs" },
      { category: "Real User Monitoring", amount: 127, unit: "sessions" },
    ],
  }),

  "/api/cardinality/high": () => [
    { metric: "http_request_duration_bucket", series: 48200, labels: 12, growthRate: 3.2 },
    { metric: "process_cpu_seconds_total", series: 18400, labels: 8, growthRate: 1.1 },
    { metric: "container_memory_usage_bytes", series: 12600, labels: 9, growthRate: 2.8 },
    { metric: "kube_pod_status_phase", series: 8900, labels: 6, growthRate: 0.5 },
  ],

  "/api/cost/recommendations": () => [
    { id: "rec-1", title: "Drop unused histogram buckets", description: "http_request_duration_bucket has 12 le labels; reducing to 8 standard buckets saves 30% cardinality", impact: "high", savingsEstimate: 420, effort: "easy" },
    { id: "rec-2", title: "Aggregate process_cpu by service", description: "Per-PID metrics can be aggregated to service-level with recording rules", impact: "medium", savingsEstimate: 180, effort: "moderate" },
    { id: "rec-3", title: "Enable log sampling for debug level", description: "Debug logs are 40% of volume but rarely queried — sample at 10%", impact: "high", savingsEstimate: 380, effort: "trivial" },
    { id: "rec-4", title: "Archive cold traces after 7 days", description: "Traces older than 7d are queried <1% of the time", impact: "medium", savingsEstimate: 240, effort: "easy" },
  ],

  "/api/cost/quick-wins": () => [
    { id: "qw-1", title: "Enable metric deduplication", description: "Duplicate series from multiple scrapers detected", category: "metrics", monthlySavings: 95, effort: "trivial", impact: "medium" },
    { id: "qw-2", title: "Set log retention to 30 days", description: "Currently retaining 90 days, only 30 needed per policy", category: "logs", monthlySavings: 300, effort: "trivial", impact: "high" },
    { id: "qw-3", title: "Remove stale synthetic checks", description: "2 checks target decommissioned endpoints", category: "synthetics", monthlySavings: 40, effort: "trivial", impact: "low" },
    { id: "qw-4", title: "Compress trace payloads", description: "Enable gzip on OTLP receiver — reduces storage 60%", category: "traces", monthlySavings: 180, effort: "easy", impact: "high" },
  ],

  // ── performance ───────────────────────────────────────────────

  "/api/anomaly/recent": () => [
    { id: "anom-1", metric: "http_request_duration_seconds", service: "checkout-api", severity: "critical", detectedAt: ago(25 * MIN), description: "P99 latency increased 340% in 5 minutes", score: 0.95 },
    { id: "anom-2", metric: "process_resident_memory_bytes", service: "payment-gateway", severity: "medium", detectedAt: ago(40 * MIN), description: "Steady memory growth — potential leak", score: 0.72 },
    { id: "anom-3", metric: "redis_connected_clients", service: "auth-service", severity: "high", detectedAt: ago(15 * MIN), description: "Connection count spiked from 48 to 200", score: 0.88 },
  ],

  "/api/dbwatch/queries": () => [
    { id: "q-1", query: "SELECT * FROM carts WHERE user_id = $1 AND status = 'active'", database: "postgres:checkout", avgMs: 45, maxMs: 450, callCount: 4521, errorCount: 12 },
    { id: "q-2", query: "INSERT INTO orders (user_id, total, items) VALUES ($1, $2, $3)", database: "postgres:checkout", avgMs: 12, maxMs: 85, callCount: 3180, errorCount: 3 },
    { id: "q-3", query: "SELECT p.*, i.quantity FROM products p JOIN inventory i ON p.id = i.product_id", database: "postgres:inventory", avgMs: 8, maxMs: 62, callCount: 6210, errorCount: 0 },
    { id: "q-4", query: "UPDATE payments SET status = $1 WHERE txn_id = $2", database: "postgres:payments", avgMs: 18, maxMs: 120, callCount: 3180, errorCount: 47 },
    { id: "q-5", query: "SELECT * FROM sessions WHERE token_hash = $1 AND expires_at > NOW()", database: "postgres:auth", avgMs: 3, maxMs: 15, callCount: 2341, errorCount: 0 },
  ],

  "/api/dbwatch/slow": () => [
    { id: "sq-1", query: "SELECT * FROM carts WHERE user_id = $1 AND status = 'active'", database: "postgres:checkout", avgMs: 450, maxMs: 1200, callCount: 89 },
    { id: "sq-2", query: "SELECT o.*, oi.* FROM orders o JOIN order_items oi ON o.id = oi.order_id WHERE o.user_id = $1", database: "postgres:checkout", avgMs: 380, maxMs: 920, callCount: 45 },
    { id: "sq-3", query: "SELECT COUNT(*) FROM audit_logs WHERE created_at > $1 GROUP BY action", database: "postgres:auth", avgMs: 520, maxMs: 1800, callCount: 12 },
    { id: "sq-4", query: "UPDATE payments SET status = $1, retry_count = retry_count + 1 WHERE txn_id = $2 AND status = 'pending'", database: "postgres:payments", avgMs: 280, maxMs: 650, callCount: 47 },
  ],

  "/api/flamegraph": () => ({
    hotspots: [
      { function: "validatePaymentDetails", module: "checkout-api/handlers", selfPercent: 18.4, totalPercent: 32.1, samples: 4210 },
      { function: "json.Marshal", module: "encoding/json", selfPercent: 12.2, totalPercent: 12.2, samples: 2800 },
      { function: "pgx.Query", module: "github.com/jackc/pgx/v5", selfPercent: 9.8, totalPercent: 15.6, samples: 2250 },
      { function: "http.(*conn).serve", module: "net/http", selfPercent: 8.1, totalPercent: 45.3, samples: 1860 },
      { function: "crypto/tls.(*Conn).Handshake", module: "crypto/tls", selfPercent: 6.5, totalPercent: 6.5, samples: 1490 },
    ],
  }),

  // ── deploys ───────────────────────────────────────────────────

  "/api/deploys": () => [
    { id: "dep-1", service: "checkout-api", version: "v2.14.1", environment: "production", status: "success", deployedAt: ago(58 * MIN), deployedBy: "ci/github-actions" },
    { id: "dep-2", service: "auth-service", version: "v1.8.0", environment: "production", status: "success", deployedAt: ago(2.3 * HR), deployedBy: "ci/github-actions" },
    { id: "dep-3", service: "frontend-bff", version: "v4.1.0", environment: "production", status: "success", deployedAt: ago(4 * HR), deployedBy: "ci/github-actions" },
    { id: "dep-4", service: "order-worker", version: "v2.14.1", environment: "production", status: "failed", deployedAt: ago(55 * MIN), deployedBy: "ci/github-actions" },
    { id: "dep-5", service: "inventory-api", version: "v1.5.3", environment: "production", status: "success", deployedAt: ago(12 * HR), deployedBy: "carol@example.com" },
  ],

  "/api/deploys/stats": () => ({
    totalDeploys: 47, successCount: 42, failedCount: 4, rollbackCount: 1, avgFrequencyPerDay: 6.7,
  }),

  // ── audit / admin ─────────────────────────────────────────────

  "/api/audit/summary": () => ({
    period: "24h",
    total_queries: 1248, failed_queries: 23,
    total_logins: 89, failed_logins: 7,
    total_admin_actions: 12, total_exports: 3,
  }),

  "/api/audit/logs/paginated": () => ({
    logs: [
      { id: "a-1", timestamp: ago(5 * MIN), user_id: "u-1", user_email: "alice@example.com", action: "incident.acknowledge", resource_type: "incident", outcome: "success", resource_name: "inc-1" },
      { id: "a-2", timestamp: ago(15 * MIN), user_id: "u-2", user_email: "bob@example.com", action: "alert.silence", resource_type: "alert", outcome: "success", resource_name: "alert-inv-disk" },
      { id: "a-3", timestamp: ago(30 * MIN), user_id: "u-1", user_email: "alice@example.com", action: "dashboard.update", resource_type: "dashboard", outcome: "success", resource_name: "Platform SRE" },
      { id: "a-4", timestamp: ago(1 * HR), user_id: "u-3", user_email: "carol@example.com", action: "apikey.create", resource_type: "apikey", outcome: "success", resource_name: "ci-deploy-key" },
      { id: "a-5", timestamp: ago(2 * HR), user_id: "u-4", user_email: "dana@example.com", action: "user.login", resource_type: "session", outcome: "success", resource_name: "" },
      { id: "a-6", timestamp: ago(3 * HR), user_id: "u-5", user_email: "unknown@example.com", action: "user.login", resource_type: "session", outcome: "failure", resource_name: "" },
    ],
    total: 156, limit: 20, offset: 0, has_more: true,
  }),

  "/api/apikeys": () => [
    { id: "key-1", name: "CI Deploy Key", prefix: "dw_ci_***", created_at: ago(30 * 24 * HR), last_used: ago(1 * HR), role: "editor" },
    { id: "key-2", name: "Grafana Integration", prefix: "dw_gf_***", created_at: ago(60 * 24 * HR), last_used: ago(5 * MIN), role: "viewer" },
    { id: "key-3", name: "Alerting Webhook", prefix: "dw_wh_***", created_at: ago(14 * 24 * HR), last_used: ago(23 * MIN), role: "editor" },
  ],

  "/api/backup/list": () => [
    { id: "bk-1", filename: "dogwatch-backup-20260210-0600.db", size: 524_288_000, created_at: ago(4 * HR), status: "completed" },
    { id: "bk-2", filename: "dogwatch-backup-20260209-0600.db", size: 518_400_000, created_at: ago(28 * HR), status: "completed" },
    { id: "bk-3", filename: "dogwatch-backup-20260210-1200.db", size: 0, created_at: ago(0), status: "scheduled" },
  ],

  // ── dashboards ────────────────────────────────────────────────

  "/api/dashboards": () => [],
  "/api/dashboards/default": () => null,

  // ── alerting rules (monitors) ─────────────────────────────────

  "/api/alerting/rules": () => [
    {
      id: "rule-1", name: "Checkout P99 Latency", description: "Alert when checkout p99 exceeds 500ms for 5 minutes",
      type: "threshold", enabled: true, query: "histogram_quantile(0.99, rate(http_request_duration_seconds_bucket{service=\"checkout-api\"}[5m]))",
      condition: "gt", threshold: 0.5, severity: "critical", for_duration: "5m",
      notify_channels: ["#incidents-critical", "PagerDuty"], labels: { service: "checkout-api", team: "platform" },
      created_at: ago(30 * 24 * HR), updated_at: ago(2 * HR), created_by: "alice@example.com",
    },
    {
      id: "rule-2", name: "Error Rate Spike", description: "Alert when 5xx error rate exceeds 5%",
      type: "threshold", enabled: true, query: "rate(http_requests_total{status=~\"5..\"}[5m]) / rate(http_requests_total[5m]) * 100",
      condition: "gt", threshold: 5, severity: "critical", for_duration: "3m",
      notify_channels: ["#incidents-critical", "PagerDuty"], labels: { team: "platform" },
      created_at: ago(60 * 24 * HR), updated_at: ago(10 * 24 * HR), created_by: "bob@example.com",
    },
    {
      id: "rule-3", name: "Memory Pressure", description: "Container memory usage above 80% of limit",
      type: "threshold", enabled: true, query: "container_memory_usage_bytes / container_spec_memory_limit_bytes * 100",
      condition: "gt", threshold: 80, severity: "warning", for_duration: "10m",
      notify_channels: ["#ops-alerts"], labels: { team: "platform" },
      created_at: ago(45 * 24 * HR), updated_at: ago(45 * 24 * HR), created_by: "alice@example.com",
    },
    {
      id: "rule-4", name: "Anomalous Latency", description: "Detect unusual latency patterns using z-score",
      type: "anomaly", enabled: true, query: "http_request_duration_seconds{service=~\".+\"}",
      condition: "gt", threshold: 3, severity: "warning", for_duration: "5m",
      notify_channels: ["#ops-alerts"], labels: {},
      created_at: ago(20 * 24 * HR), updated_at: ago(20 * 24 * HR), created_by: "carol@example.com",
    },
    {
      id: "rule-5", name: "Deploy Error Rate Change", description: "Alert if error rate increases 50% after deploy",
      type: "change", enabled: true, query: "rate(http_requests_total{status=~\"5..\"}[5m])",
      condition: "gt", threshold: 50, severity: "warning", for_duration: "10m",
      notify_channels: ["#deploys"], labels: {},
      created_at: ago(15 * 24 * HR), updated_at: ago(15 * 24 * HR), created_by: "bob@example.com",
    },
    {
      id: "rule-6", name: "Heartbeat Missing", description: "Alert when service heartbeat metric disappears",
      type: "absence", enabled: true, query: "up{service=~\".+\"}",
      condition: "eq", threshold: 0, severity: "critical", for_duration: "2m",
      notify_channels: ["#incidents-critical", "PagerDuty"], labels: {},
      created_at: ago(90 * 24 * HR), updated_at: ago(30 * 24 * HR), created_by: "alice@example.com",
    },
    {
      id: "rule-7", name: "Pod Restart Rate", description: "Alert on excessive pod restarts",
      type: "threshold", enabled: false, query: "increase(kube_pod_container_status_restarts_total[1h])",
      condition: "gt", threshold: 3, severity: "info", for_duration: "0m",
      notify_channels: ["#ops-alerts"], labels: { team: "platform" },
      created_at: ago(10 * 24 * HR), updated_at: ago(5 * 24 * HR), created_by: "dana@example.com",
    },
    {
      id: "rule-8", name: "SLO Budget Burn", description: "Composite: checkout latency SLO burning too fast AND error rate elevated",
      type: "composite", enabled: true, query: "rule-1 AND rule-2",
      condition: "gt", threshold: 0, severity: "critical", for_duration: "5m",
      notify_channels: ["#incidents-critical", "PagerDuty"], labels: { service: "checkout-api" },
      created_at: ago(7 * 24 * HR), updated_at: ago(7 * 24 * HR), created_by: "alice@example.com",
    },
  ],

  // ── query explorer ────────────────────────────────────────────

  "/api/query/metadata": () => ({
    sources: ["metrics", "logs", "traces", "events"],
    functions: ["avg", "sum", "count", "min", "max", "p50", "p90", "p95", "p99", "rate", "increase", "histogram_quantile", "topk", "bottomk", "sort", "sort_desc", "absent", "changes", "delta", "deriv", "predict_linear", "stddev", "stdvar"],
  }),

  "/api/query/saved": () => [
    { id: "saved-1", name: "Checkout Latency P99", query: "histogram_quantile(0.99, rate(http_request_duration_seconds_bucket{service=\"checkout-api\"}[5m]))", description: "P99 latency for checkout service", created_at: ago(7 * 24 * HR), updated_at: ago(1 * HR) },
    { id: "saved-2", name: "Error Rate by Service", query: "sum by (service) (rate(http_requests_total{status=~\"5..\"}[5m])) / sum by (service) (rate(http_requests_total[5m])) * 100", description: "5xx error rate per service", created_at: ago(14 * 24 * HR), updated_at: ago(3 * 24 * HR) },
    { id: "saved-3", name: "Top Memory Consumers", query: "topk(10, container_memory_usage_bytes / 1024 / 1024)", description: "Top 10 containers by memory usage in MB", created_at: ago(30 * 24 * HR), updated_at: ago(10 * 24 * HR) },
    { id: "saved-4", name: "Request Rate", query: "sum(rate(http_requests_total[5m])) by (service)", description: "Requests per second by service", created_at: ago(21 * 24 * HR), updated_at: ago(21 * 24 * HR) },
  ],

  "/api/query/execute": () => ({
    columns: ["service", "value", "timestamp"],
    rows: [
      { service: "checkout-api", value: 0.842, timestamp: ago(0) },
      { service: "frontend-bff", value: 0.045, timestamp: ago(0) },
      { service: "auth-service", value: 0.312, timestamp: ago(0) },
      { service: "payment-gateway", value: 0.420, timestamp: ago(0) },
      { service: "order-worker", value: 1.250, timestamp: ago(0) },
      { service: "inventory-api", value: 0.028, timestamp: ago(0) },
    ],
    count: 6,
  }),

  // Recording rules
  "/api/recording-rules": () => [
    {
      id: "builtin:request_rate:1m", name: "service:request_rate:1m",
      expression: 'sum(rate(http_requests_total[1m])) by (service)',
      interval: 60000000000, labels: { __name__: "service:request_rate:1m" },
      enabled: true, last_eval: ago(15 * SEC), last_error: "", last_value: 342.8,
      description: "Requests per second by service (1m rate)", created_at: ago(30 * 24 * 60 * MIN), updated_at: ago(2 * 24 * 60 * MIN), created_by: "system",
    },
    {
      id: "builtin:error_rate:1m", name: "service:error_rate:1m",
      expression: 'sum(rate(http_requests_total{status=~"5.."}[1m])) by (service) / sum(rate(http_requests_total[1m])) by (service) * 100',
      interval: 60000000000, labels: { __name__: "service:error_rate:1m" },
      enabled: true, last_eval: ago(15 * SEC), last_error: "", last_value: 2.4,
      description: "Error percentage by service (1m rate)", created_at: ago(30 * 24 * 60 * MIN), updated_at: ago(2 * 24 * 60 * MIN), created_by: "system",
    },
    {
      id: "builtin:latency_p99:1m", name: "service:latency_p99:1m",
      expression: 'histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[1m])) by (service, le))',
      interval: 60000000000, labels: { __name__: "service:latency_p99:1m" },
      enabled: true, last_eval: ago(15 * SEC), last_error: "", last_value: 0.487,
      description: "P99 latency by service (1m rate)", created_at: ago(30 * 24 * 60 * MIN), updated_at: ago(2 * 24 * 60 * MIN), created_by: "system",
    },
    {
      id: "custom:checkout_saturation:5m", name: "checkout:saturation:5m",
      expression: 'sum(rate(http_requests_total{service="checkout-api"}[5m])) / 500',
      interval: 300000000000, labels: { __name__: "checkout:saturation:5m", team: "platform" },
      enabled: true, last_eval: ago(45 * SEC), last_error: "", last_value: 0.68,
      description: "Checkout API saturation ratio (capacity 500 rps)", created_at: ago(7 * 24 * 60 * MIN), updated_at: ago(3 * 60 * MIN), created_by: "ops@example.com",
    },
    {
      id: "custom:order_throughput:5m", name: "order:throughput:5m",
      expression: 'sum(rate(orders_processed_total[5m]))',
      interval: 300000000000, labels: { __name__: "order:throughput:5m" },
      enabled: false, last_eval: ago(2 * 60 * MIN), last_error: "metric not found: orders_processed_total", last_value: 0,
      description: "Order processing throughput", created_at: ago(3 * 24 * 60 * MIN), updated_at: ago(60 * MIN), created_by: "dev@example.com",
    },
  ],

  "/api/recording-rules-status": () => ({
    total_rules: 5,
    enabled_rules: 4,
    total_evaluations: 18240,
    successful_evaluations: 18190,
    failed_evaluations: 50,
    last_eval_duration: 12000000,
    avg_eval_duration: 8500000,
  }),
};

// --- Pattern matchers for parameterized routes ---

const patterns: [RegExp, () => unknown][] = [
  // /api/traces/:traceId/spans
  [/^\/api\/traces\/[^/]+\/spans$/, () => [
    { span_id: "span-1", parent_span_id: "", operation_name: "POST /checkout", service_name: "frontend-bff", duration_ms: 842, status: "ERROR", depth: 0 },
    { span_id: "span-2", parent_span_id: "span-1", operation_name: "POST /api/checkout", service_name: "checkout-api", duration_ms: 820, status: "ERROR", depth: 1 },
    { span_id: "span-3", parent_span_id: "span-2", operation_name: "SELECT carts", service_name: "checkout-api", duration_ms: 45, status: "OK", depth: 2 },
    { span_id: "span-4", parent_span_id: "span-2", operation_name: "validatePaymentDetails", service_name: "checkout-api", duration_ms: 410, status: "OK", depth: 2 },
    { span_id: "span-5", parent_span_id: "span-2", operation_name: "POST /charge", service_name: "payment-gateway", duration_ms: 320, status: "OK", depth: 2 },
    { span_id: "span-6", parent_span_id: "span-5", operation_name: "stripe.Charge", service_name: "payment-gateway", duration_ms: 280, status: "OK", depth: 3 },
    { span_id: "span-7", parent_span_id: "span-2", operation_name: "INSERT orders", service_name: "checkout-api", duration_ms: 12, status: "OK", depth: 2 },
    { span_id: "span-8", parent_span_id: "span-2", operation_name: "publish order.created", service_name: "checkout-api", duration_ms: 5, status: "OK", depth: 2 },
  ]],

  // /api/oncall/current/:scheduleId
  [/^\/api\/oncall\/current\//, () => ({
    on_call: {
      user: { id: "u-1", name: "Alice Chen" },
      start_time: ago(6 * HR),
      end_time: ago(-18 * HR),
      is_override: false,
    },
  })],

  // /api/correlate/service/:service/timeline
  [/^\/api\/correlate\/service\/[^/]+\/timeline/, () => ({
    service: "checkout-api",
    summary: { totalEvents: 12, errorLogCount: 89, traceCount: 4521, incidentCount: 1, deployCount: 1, alertCount: 2 },
    events: [
      { id: "ce-1", type: "deploy", timestamp: ago(58 * MIN), summary: "Deployed v2.14.1", severity: "info", service: "checkout-api" },
      { id: "ce-2", type: "alert", timestamp: ago(23 * MIN), summary: "P99 latency exceeded 500ms", severity: "critical", service: "checkout-api" },
      { id: "ce-3", type: "incident", timestamp: ago(23 * MIN), summary: "Checkout latency spike", severity: "critical", service: "checkout-api" },
    ],
  })],

  // /api/dashboards/:id (single dashboard fetch)
  [/^\/api\/dashboards\/[^/]+$/, () => null],

  // /api/recording-rules/:id/history
  [/^\/api\/recording-rules\/[^/]+\/history$/, () => {
    const entries = [];
    for (let i = 0; i < 20; i++) {
      const success = Math.random() > 0.05;
      entries.push({
        id: `eval-${i}`,
        rule_id: "builtin:request_rate:1m",
        timestamp: new Date(Date.now() - i * 60 * 1000).toISOString(),
        value: success ? 300 + Math.random() * 100 : 0,
        duration_ms: 5 + Math.random() * 15,
        error: success ? "" : "query timeout",
        success,
      });
    }
    return entries;
  }],
];

/** Return mock data for the given API path, or undefined if no mock exists. */
export function getMockResponse(path: string): unknown {
  const base = path.split("?")[0];

  const fn = routes[base];
  if (fn) return fn();

  for (const [re, gen] of patterns) {
    if (re.test(base)) return gen();
  }

  return undefined;
}
