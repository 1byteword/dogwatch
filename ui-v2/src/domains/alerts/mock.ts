import { AlertItem } from "./types";

export const mockAlerts: AlertItem[] = [
  {
    id: "a1",
    name: "Checkout latency p95 breach",
    service: "checkout-api",
    severity: "critical",
    state: "firing",
    trigger: "p95 latency > 850ms for 10m",
    startedAt: "4m ago",
    startedAtRaw: new Date(Date.now() - 4 * 60 * 1000).toISOString(),
    probableCause: "db pool saturation after deploy 2026.02.07-rc4",
    recentDeploy: "2026.02.07-rc4",
    traceErrors: 183
  },
  {
    id: "a2",
    name: "Order queue consumer lag",
    service: "order-worker",
    severity: "high",
    state: "firing",
    trigger: "consumer lag > 20k for 5m",
    startedAt: "11m ago",
    startedAtRaw: new Date(Date.now() - 11 * 60 * 1000).toISOString(),
    probableCause: "k8s node pressure, throttled worker pods",
    recentDeploy: "none in last 24h",
    traceErrors: 47
  },
  {
    id: "a3",
    name: "Auth 5xx anomaly",
    service: "auth-service",
    severity: "medium",
    state: "pending",
    trigger: "5xx ratio above baseline +220%",
    startedAt: "2m ago",
    startedAtRaw: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
    probableCause: "upstream identity provider timeout spikes",
    recentDeploy: "2026.02.06-hotfix-2",
    traceErrors: 19
  }
];
