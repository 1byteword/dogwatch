export type WidgetDef = {
  id: string;
  title: string;
  description: string;
  category: string;
  defaultW: number;
  defaultH: number;
};

export const WIDGET_CATEGORIES = [
  "Infrastructure", "Tracing", "Alerts & Incidents", "Logs",
  "Reliability", "Kubernetes", "Deployments", "Cost & Performance",
  "On-call & Notify", "Services", "Admin", "Ops",
] as const;

export const WIDGETS: WidgetDef[] = [
  // Infrastructure
  { id: "system-overview", title: "System Overview", description: "CPU, memory, load, and core platform pressure", category: "Infrastructure", defaultW: 4, defaultH: 2 },
  { id: "traffic-overview", title: "Traffic Overview", description: "Request, error, and connection totals", category: "Infrastructure", defaultW: 4, defaultH: 2 },
  { id: "endpoint-latency", title: "Endpoint Latency", description: "Top endpoints by p99 latency and errors", category: "Infrastructure", defaultW: 6, defaultH: 2 },
  { id: "connection-hotspots", title: "Connection Hotspots", description: "Busiest process to remote network paths", category: "Infrastructure", defaultW: 6, defaultH: 2 },
  { id: "process-top", title: "Top Processes", description: "Highest CPU and memory process consumers", category: "Infrastructure", defaultW: 4, defaultH: 2 },
  { id: "endpoint-detail", title: "Endpoint Detail", description: "Full endpoint list with latency and error rate", category: "Infrastructure", defaultW: 6, defaultH: 2 },
  { id: "connection-detail", title: "Connection Detail", description: "Connection list with count, remote, and protocol", category: "Infrastructure", defaultW: 6, defaultH: 2 },
  // Tracing
  { id: "service-map-health", title: "Service Map Health", description: "Topology node/link pressure and shape", category: "Tracing", defaultW: 4, defaultH: 2 },
  { id: "trace-throughput", title: "Trace Throughput", description: "Recent traces with duration and status", category: "Tracing", defaultW: 6, defaultH: 2 },
  { id: "trace-services", title: "Trace Services", description: "Services currently emitting trace spans", category: "Tracing", defaultW: 4, defaultH: 2 },
  { id: "trace-dependencies", title: "Trace Dependencies", description: "Service dependency call relationships", category: "Tracing", defaultW: 6, defaultH: 2 },
  { id: "trace-detail", title: "Trace Detail", description: "Trace waterfall with span hierarchy and timing", category: "Tracing", defaultW: 8, defaultH: 3 },
  // Alerts & Incidents
  { id: "alerts-feed", title: "Alert Feed", description: "Active alert pressure and top triggers", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  { id: "alerts-severity-map", title: "Alert Severity Mix", description: "Current severity distribution and pending load", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  { id: "alerts-watches", title: "Watch Rules", description: "Configured alert watch rules and evaluation status", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  { id: "alerts-silences", title: "Alert Silences", description: "Active alert silences and expiry", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  { id: "incidents-live", title: "Live Incidents", description: "Open incident queue and ownership", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  { id: "incidents-by-commander", title: "Commander Load", description: "Incident ownership concentration by commander", category: "Alerts & Incidents", defaultW: 4, defaultH: 2 },
  // Logs
  { id: "logs-errors", title: "Error Log Stream", description: "Most recent error and warn events", category: "Logs", defaultW: 6, defaultH: 2 },
  { id: "logs-patterns", title: "Log Patterns", description: "Discovered log patterns grouped by frequency", category: "Logs", defaultW: 6, defaultH: 2 },
  { id: "logs-trending", title: "Trending Patterns", description: "Log patterns gaining frequency", category: "Logs", defaultW: 6, defaultH: 2 },
  { id: "log-compare", title: "Log Compare", description: "Side-by-side before/after log comparison", category: "Logs", defaultW: 8, defaultH: 3 },
  // Reliability
  { id: "kpi-reliability", title: "Reliability Pulse", description: "SLO health and error pressure", category: "Reliability", defaultW: 4, defaultH: 2 },
  { id: "slo-burn-rate", title: "SLO Burn Rate", description: "Error budget consumption speed across SLOs", category: "Reliability", defaultW: 4, defaultH: 2 },
  { id: "slo-budget-remaining", title: "SLO Budget Remaining", description: "Remaining error budgets and SLO health", category: "Reliability", defaultW: 4, defaultH: 2 },
  { id: "synthetic-uptime", title: "Synthetic Uptime", description: "Synthetic check uptime and latency overview", category: "Reliability", defaultW: 4, defaultH: 2 },
  { id: "synthetic-failures", title: "Synthetic Failures", description: "Recent synthetic check failures and errors", category: "Reliability", defaultW: 6, defaultH: 2 },
  // Kubernetes
  { id: "k8s-cluster", title: "Cluster Health", description: "Kubernetes readiness and warning load", category: "Kubernetes", defaultW: 4, defaultH: 2 },
  { id: "k8s-capacity-risk", title: "Capacity Risk", description: "Kubernetes saturation and readiness pressure", category: "Kubernetes", defaultW: 4, defaultH: 2 },
  { id: "k8s-containers", title: "Containers", description: "Container status, images, and restart counts", category: "Kubernetes", defaultW: 6, defaultH: 2 },
  { id: "k8s-pods", title: "Pods", description: "Pod list sorted by restarts with status and node", category: "Kubernetes", defaultW: 6, defaultH: 2 },
  { id: "k8s-deployments", title: "Deployments", description: "Deployment readiness and replica status", category: "Kubernetes", defaultW: 6, defaultH: 2 },
  { id: "k8s-events", title: "K8s Events", description: "Recent Kubernetes events sorted by time", category: "Kubernetes", defaultW: 6, defaultH: 2 },
  // Deployments
  { id: "deploy-correlation", title: "Deploy Correlation", description: "Deploy confidence with incident timing", category: "Deployments", defaultW: 6, defaultH: 2 },
  { id: "deploy-feed", title: "Deploy Feed", description: "Recent deployments across services", category: "Deployments", defaultW: 6, defaultH: 2 },
  { id: "deploy-stats", title: "Deploy Stats", description: "Deployment KPIs: total, success rate, rollback rate", category: "Deployments", defaultW: 4, defaultH: 2 },
  // Cost & Performance
  { id: "cost-estimate", title: "Cost Estimate", description: "Platform cost vs Datadog equivalent spend", category: "Cost & Performance", defaultW: 4, defaultH: 2 },
  { id: "cardinality-hotspots", title: "Cardinality Hotspots", description: "Highest cardinality metric series and growth", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "cost-recommendations", title: "Cost Recommendations", description: "Actionable cost optimizations and quick wins", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "cost-quick-wins", title: "Cost Quick Wins", description: "Top savings opportunities sorted by monthly impact", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "perf-anomalies", title: "Anomalies", description: "Recently detected metric anomalies", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "perf-db-queries", title: "DB Queries", description: "Slowest database queries and error rates", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "perf-slow-queries", title: "Slow Queries", description: "Slowest database queries by max execution time", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  { id: "perf-flamegraph-top", title: "CPU Hotspots", description: "Top CPU-consuming functions from profiling", category: "Cost & Performance", defaultW: 6, defaultH: 2 },
  // On-call & Notify
  { id: "oncall-now", title: "On-call Command", description: "Current rotation and policy readiness", category: "On-call & Notify", defaultW: 4, defaultH: 2 },
  { id: "notify-delivery", title: "Notification Delivery", description: "Channel reliability and failed sends", category: "On-call & Notify", defaultW: 4, defaultH: 2 },
  { id: "notify-failure-log", title: "Delivery Failures", description: "Recent failed notification deliveries", category: "On-call & Notify", defaultW: 6, defaultH: 2 },
  // Services
  { id: "service-health", title: "Service Health", description: "Catalog health and highest-risk services", category: "Services", defaultW: 4, defaultH: 2 },
  { id: "service-latency-top", title: "Latency Hotspots", description: "Services with highest response time", category: "Services", defaultW: 4, defaultH: 2 },
  // Admin
  { id: "admin-audit-feed", title: "Audit Feed", description: "Recent user actions and system events", category: "Admin", defaultW: 6, defaultH: 2 },
  { id: "admin-api-keys", title: "API Keys", description: "Active API key inventory and usage", category: "Admin", defaultW: 4, defaultH: 2 },
  { id: "admin-backup-status", title: "Backup Status", description: "Backup health and recent backups", category: "Admin", defaultW: 4, defaultH: 2 },
  // Ops
  { id: "ops-action-queue", title: "Ops Action Queue", description: "Highest-priority actions to execute now", category: "Ops", defaultW: 6, defaultH: 2 },
  { id: "command-links", title: "Ops Shortcuts", description: "Jump to triage and response surfaces", category: "Ops", defaultW: 8, defaultH: 2 },
];
