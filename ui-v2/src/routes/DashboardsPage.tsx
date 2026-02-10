import { A } from "@solidjs/router";
import { ErrorBoundary, For, Show, batch, createEffect, createMemo, createResource, createSignal, onCleanup, onMount, untrack } from "solid-js";
import type uPlot from "uplot";
import { useAutoRefresh } from "../core/live";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { ChartPanel } from "../design/components/ChartPanel";
import { Input } from "../design/components/Input";
import { Sparkline } from "../design/components/Sparkline";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";
import { WidgetLoading } from "../design/components/WidgetLoading";
import {
  createDashboard,
  deleteDashboard,
  loadDashboard,
  loadDashboards,
  setDefaultDashboard,
  updateDashboard
} from "../domains/dashboards/service";
import { DashboardWidgetConfig, DashboardWidgetPosition } from "../domains/dashboards/types";
import { loadAlerts } from "../domains/alerts/service";
import { loadIncidents } from "../domains/incidents/service";
import { loadLogs } from "../domains/logs/service";
import { loadCatalogServices, loadCatalogStats } from "../domains/catalog/service";
import { loadDeployIncidentCorrelations } from "../domains/correlation/service";
import { loadCurrentOncall, loadOncallPolicies, loadOncallSchedules } from "../domains/oncall/service";
import { loadK8sContainers, loadK8sDeployments, loadK8sEvents, loadK8sPods, loadK8sSummary } from "../domains/kubernetes/service";
import { loadNotifyChannels, loadNotifyHistory } from "../domains/notify/service";
import {
  loadServiceMap,
  loadStatsSummary,
  loadSystemHistory,
  loadSystemMetrics,
  loadTopProcesses,
  loadTraceDependencies,
  loadTraceServices,
  loadTraceSpans,
  loadTraceSummaries
} from "../domains/ops/service";
import { loadSlos, loadSyntheticChecks, loadSyntheticFailures } from "../domains/slo/service";
import { loadCostEstimate, loadCardinalityHotspots, loadCostQuickWins, loadCostRecommendations } from "../domains/cost/service";
import { loadAnomalies, loadDbQueries, loadFlamegraphHotspots, loadSlowQueries } from "../domains/performance/service";
import { loadWatches, loadSilences } from "../domains/alerts/service";
import { loadLogComparison, loadLogPatterns, loadTrendingPatterns } from "../domains/logs/service";
import { loadDeploys, loadDeployStats } from "../domains/deploys/service";
import { loadRecentAuditLogs, loadApiKeys, loadBackups } from "../domains/audit/service";

type WidgetDef = {
  id: string;
  title: string;
  description: string;
  defaultW: number;
  defaultH: number;
};

const WIDGETS: WidgetDef[] = [
  { id: "system-overview", title: "System Overview", description: "CPU, memory, load, and core platform pressure", defaultW: 4, defaultH: 2 },
  { id: "traffic-overview", title: "Traffic Overview", description: "Request, error, and connection totals", defaultW: 4, defaultH: 2 },
  { id: "endpoint-latency", title: "Endpoint Latency", description: "Top endpoints by p99 latency and errors", defaultW: 6, defaultH: 2 },
  { id: "connection-hotspots", title: "Connection Hotspots", description: "Busiest process to remote network paths", defaultW: 6, defaultH: 2 },
  { id: "process-top", title: "Top Processes", description: "Highest CPU and memory process consumers", defaultW: 4, defaultH: 2 },
  { id: "service-map-health", title: "Service Map Health", description: "Topology node/link pressure and shape", defaultW: 4, defaultH: 2 },
  { id: "trace-throughput", title: "Trace Throughput", description: "Recent traces with duration and status", defaultW: 6, defaultH: 2 },
  { id: "trace-services", title: "Trace Services", description: "Services currently emitting trace spans", defaultW: 4, defaultH: 2 },
  { id: "trace-dependencies", title: "Trace Dependencies", description: "Service dependency call relationships", defaultW: 6, defaultH: 2 },
  { id: "kpi-reliability", title: "Reliability Pulse", description: "SLO health and error pressure", defaultW: 4, defaultH: 2 },
  { id: "alerts-feed", title: "Alert Feed", description: "Active alert pressure and top triggers", defaultW: 4, defaultH: 2 },
  { id: "alerts-severity-map", title: "Alert Severity Mix", description: "Current severity distribution and pending load", defaultW: 4, defaultH: 2 },
  { id: "incidents-live", title: "Live Incidents", description: "Open incident queue and ownership", defaultW: 4, defaultH: 2 },
  { id: "incidents-by-commander", title: "Commander Load", description: "Incident ownership concentration by commander", defaultW: 4, defaultH: 2 },
  { id: "logs-errors", title: "Error Log Stream", description: "Most recent error and warn events", defaultW: 6, defaultH: 2 },
  { id: "deploy-correlation", title: "Deploy Correlation", description: "Deploy confidence with incident timing", defaultW: 6, defaultH: 2 },
  { id: "service-health", title: "Service Health", description: "Catalog health and highest-risk services", defaultW: 4, defaultH: 2 },
  { id: "service-latency-top", title: "Latency Hotspots", description: "Services with highest response time", defaultW: 4, defaultH: 2 },
  { id: "oncall-now", title: "On-call Command", description: "Current rotation and policy readiness", defaultW: 4, defaultH: 2 },
  { id: "k8s-cluster", title: "Cluster Health", description: "Kubernetes readiness and warning load", defaultW: 4, defaultH: 2 },
  { id: "k8s-capacity-risk", title: "Capacity Risk", description: "Kubernetes saturation and readiness pressure", defaultW: 4, defaultH: 2 },
  { id: "notify-delivery", title: "Notification Delivery", description: "Channel reliability and failed sends", defaultW: 4, defaultH: 2 },
  { id: "notify-failure-log", title: "Delivery Failures", description: "Recent failed notification deliveries", defaultW: 6, defaultH: 2 },
  { id: "ops-action-queue", title: "Ops Action Queue", description: "Highest-priority actions to execute now", defaultW: 6, defaultH: 2 },
  { id: "command-links", title: "Ops Shortcuts", description: "Jump to triage and response surfaces", defaultW: 8, defaultH: 2 },
  { id: "slo-burn-rate", title: "SLO Burn Rate", description: "Error budget consumption speed across SLOs", defaultW: 4, defaultH: 2 },
  { id: "slo-budget-remaining", title: "SLO Budget Remaining", description: "Remaining error budgets and SLO health", defaultW: 4, defaultH: 2 },
  { id: "synthetic-uptime", title: "Synthetic Uptime", description: "Synthetic check uptime and latency overview", defaultW: 4, defaultH: 2 },
  { id: "synthetic-failures", title: "Synthetic Failures", description: "Recent synthetic check failures and errors", defaultW: 6, defaultH: 2 },
  { id: "cost-estimate", title: "Cost Estimate", description: "Platform cost vs Datadog equivalent spend", defaultW: 4, defaultH: 2 },
  { id: "cardinality-hotspots", title: "Cardinality Hotspots", description: "Highest cardinality metric series and growth", defaultW: 6, defaultH: 2 },
  { id: "cost-recommendations", title: "Cost Recommendations", description: "Actionable cost optimizations and quick wins", defaultW: 6, defaultH: 2 },
  { id: "perf-anomalies", title: "Anomalies", description: "Recently detected metric anomalies", defaultW: 6, defaultH: 2 },
  { id: "perf-db-queries", title: "DB Queries", description: "Slowest database queries and error rates", defaultW: 6, defaultH: 2 },
  { id: "perf-flamegraph-top", title: "CPU Hotspots", description: "Top CPU-consuming functions from profiling", defaultW: 6, defaultH: 2 },
  { id: "alerts-watches", title: "Watch Rules", description: "Configured alert watch rules and evaluation status", defaultW: 4, defaultH: 2 },
  { id: "alerts-silences", title: "Alert Silences", description: "Active alert silences and expiry", defaultW: 4, defaultH: 2 },
  { id: "logs-patterns", title: "Log Patterns", description: "Discovered log patterns grouped by frequency", defaultW: 6, defaultH: 2 },
  { id: "logs-trending", title: "Trending Patterns", description: "Log patterns gaining frequency", defaultW: 6, defaultH: 2 },
  { id: "deploy-feed", title: "Deploy Feed", description: "Recent deployments across services", defaultW: 6, defaultH: 2 },
  { id: "admin-audit-feed", title: "Audit Feed", description: "Recent user actions and system events", defaultW: 6, defaultH: 2 },
  { id: "admin-api-keys", title: "API Keys", description: "Active API key inventory and usage", defaultW: 4, defaultH: 2 },
  { id: "admin-backup-status", title: "Backup Status", description: "Backup health and recent backups", defaultW: 4, defaultH: 2 },
  { id: "k8s-containers", title: "Containers", description: "Container status, images, and restart counts", defaultW: 6, defaultH: 2 },
  { id: "k8s-pods", title: "Pods", description: "Pod list sorted by restarts with status and node", defaultW: 6, defaultH: 2 },
  { id: "k8s-deployments", title: "Deployments", description: "Deployment readiness and replica status", defaultW: 6, defaultH: 2 },
  { id: "k8s-events", title: "K8s Events", description: "Recent Kubernetes events sorted by time", defaultW: 6, defaultH: 2 },
  { id: "endpoint-detail", title: "Endpoint Detail", description: "Full endpoint list with latency and error rate", defaultW: 6, defaultH: 2 },
  { id: "connection-detail", title: "Connection Detail", description: "Connection list with count, remote, and protocol", defaultW: 6, defaultH: 2 },
  { id: "trace-detail", title: "Trace Detail", description: "Trace waterfall with span hierarchy and timing", defaultW: 8, defaultH: 3 },
  { id: "log-compare", title: "Log Compare", description: "Side-by-side before/after log comparison", defaultW: 8, defaultH: 3 },
  { id: "deploy-stats", title: "Deploy Stats", description: "Deployment KPIs: total, success rate, rollback rate", defaultW: 4, defaultH: 2 },
  { id: "perf-slow-queries", title: "Slow Queries", description: "Slowest database queries by max execution time", defaultW: 6, defaultH: 2 },
  { id: "cost-quick-wins", title: "Cost Quick Wins", description: "Top savings opportunities sorted by monthly impact", defaultW: 6, defaultH: 2 }
];

const DEFAULT_DASHBOARD_NAME = "Operations Command";
const EDIT_MODE_PREF_KEY = "dogwatch-v2-dashboard-edit-mode";
const DRAFT_STORAGE_KEY = "dogwatch-v2-dashboard-draft";
const DRAFT_MAX_AGE_MS = 24 * 60 * 60 * 1000;
const DRAFT_DEBOUNCE_MS = 2000;
const MAX_HISTORY = 50;
const WIDGET_INSTANCE_SEP = "::";

type EditorSnapshot = {
  layout: DashboardWidgetPosition[];
  widgetConfig: Record<string, DashboardWidgetConfig>;
};

type CopiedWidget = {
  baseId: string;
  w: number;
  h: number;
  config: DashboardWidgetConfig;
};

type ResizeAxis = "e" | "s" | "se";
type DashboardTemplate = {
  id: string;
  name: string;
  description: string;
  layout: DashboardWidgetPosition[];
  widgetConfig?: Record<string, DashboardWidgetConfig>;
};

const defaultLayout: DashboardWidgetPosition[] = [
  { id: "kpi-reliability", x: 0, y: 0, w: 4, h: 2 },
  { id: "alerts-feed", x: 4, y: 0, w: 4, h: 2 },
  { id: "incidents-live", x: 8, y: 0, w: 4, h: 2 },
  { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
  { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
  { id: "service-health", x: 0, y: 4, w: 4, h: 2 },
  { id: "oncall-now", x: 4, y: 4, w: 4, h: 2 },
  { id: "k8s-cluster", x: 8, y: 4, w: 4, h: 2 },
  { id: "notify-delivery", x: 0, y: 6, w: 4, h: 2 },
  { id: "command-links", x: 4, y: 6, w: 8, h: 2 }
];

const DASHBOARD_TEMPLATES: DashboardTemplate[] = [
  {
    id: "executive-ops",
    name: "Executive Ops",
    description: "Topline reliability, incidents, and delivery health.",
    layout: [
      { id: "kpi-reliability", x: 0, y: 0, w: 4, h: 2 },
      { id: "alerts-severity-map", x: 4, y: 0, w: 4, h: 2 },
      { id: "incidents-live", x: 8, y: 0, w: 4, h: 2 },
      { id: "service-health", x: 0, y: 2, w: 4, h: 2 },
      { id: "notify-delivery", x: 4, y: 2, w: 4, h: 2 },
      { id: "ops-action-queue", x: 8, y: 2, w: 4, h: 2 },
      { id: "command-links", x: 0, y: 4, w: 12, h: 2 }
    ]
  },
  {
    id: "incident-war-room",
    name: "Incident War Room",
    description: "Fast triage and ownership during active incidents.",
    layout: [
      { id: "incidents-live", x: 0, y: 0, w: 4, h: 2 },
      { id: "alerts-feed", x: 4, y: 0, w: 4, h: 2 },
      { id: "incidents-by-commander", x: 8, y: 0, w: 4, h: 2 },
      { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
      { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
      { id: "ops-action-queue", x: 0, y: 4, w: 8, h: 2 },
      { id: "oncall-now", x: 8, y: 4, w: 4, h: 2 }
    ]
  },
  {
    id: "platform-sre",
    name: "Platform SRE",
    description: "Infra capacity, service health, and paging pressure.",
    layout: [
      { id: "k8s-cluster", x: 0, y: 0, w: 4, h: 2 },
      { id: "k8s-capacity-risk", x: 4, y: 0, w: 4, h: 2 },
      { id: "oncall-now", x: 8, y: 0, w: 4, h: 2 },
      { id: "service-health", x: 0, y: 2, w: 4, h: 2 },
      { id: "service-latency-top", x: 4, y: 2, w: 4, h: 2 },
      { id: "notify-failure-log", x: 8, y: 2, w: 4, h: 2 },
      { id: "k8s-pods", x: 0, y: 4, w: 6, h: 2 },
      { id: "k8s-events", x: 6, y: 4, w: 6, h: 2 },
      { id: "logs-errors", x: 0, y: 6, w: 8, h: 2 },
      { id: "notify-delivery", x: 8, y: 6, w: 4, h: 2 }
    ]
  },
  {
    id: "service-owner",
    name: "Service Owner",
    description: "Service quality, deployments, and customer-facing risk.",
    layout: [
      { id: "service-health", x: 0, y: 0, w: 4, h: 2 },
      { id: "service-latency-top", x: 4, y: 0, w: 4, h: 2 },
      { id: "alerts-feed", x: 8, y: 0, w: 4, h: 2 },
      { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
      { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
      { id: "deploy-stats", x: 0, y: 4, w: 4, h: 2 },
      { id: "alerts-severity-map", x: 4, y: 4, w: 4, h: 2 },
      { id: "ops-action-queue", x: 8, y: 4, w: 4, h: 2 },
      { id: "command-links", x: 0, y: 6, w: 12, h: 2 }
    ]
  },
  {
    id: "finops",
    name: "FinOps",
    description: "Cost intelligence, SLO health, and cardinality control.",
    layout: [
      { id: "cost-estimate", x: 0, y: 0, w: 4, h: 2 },
      { id: "slo-burn-rate", x: 4, y: 0, w: 4, h: 2 },
      { id: "slo-budget-remaining", x: 8, y: 0, w: 4, h: 2 },
      { id: "cardinality-hotspots", x: 0, y: 2, w: 6, h: 2 },
      { id: "cost-recommendations", x: 6, y: 2, w: 6, h: 2 },
      { id: "synthetic-uptime", x: 0, y: 4, w: 4, h: 2 },
      { id: "synthetic-failures", x: 4, y: 4, w: 8, h: 2 },
      { id: "cost-quick-wins", x: 0, y: 6, w: 6, h: 2 }
    ]
  },
  {
    id: "security-compliance",
    name: "Security & Compliance",
    description: "Audit trail, API key posture, and backup health.",
    layout: [
      { id: "admin-audit-feed", x: 0, y: 0, w: 6, h: 2 },
      { id: "admin-api-keys", x: 6, y: 0, w: 3, h: 2 },
      { id: "admin-backup-status", x: 9, y: 0, w: 3, h: 2 },
      { id: "alerts-watches", x: 0, y: 2, w: 4, h: 2 },
      { id: "alerts-silences", x: 4, y: 2, w: 4, h: 2 },
      { id: "perf-anomalies", x: 8, y: 2, w: 4, h: 2 }
    ]
  }
];

function clampSpan(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, Number(value) || min));
}

const GRID_COLUMNS = 12;
const GRID_MAX_Y = 60;
const GRID_ROW_HEIGHT_PX = 92;
const GRID_MAX_H = 6;
const GRID_RESIZE_DRAG_STEP_PX = 68;

function quantizeDragDelta(pxDelta: number, stepPx: number): number {
  if (!Number.isFinite(pxDelta) || !Number.isFinite(stepPx) || stepPx <= 0) return 0;
  return Math.trunc(pxDelta / stepPx);
}

function overlaps(a: DashboardWidgetPosition, b: DashboardWidgetPosition): boolean {
  const noXOverlap = a.x + a.w <= b.x || b.x + b.w <= a.x;
  const noYOverlap = a.y + a.h <= b.y || b.y + b.h <= a.y;
  return !(noXOverlap || noYOverlap);
}

function hasOverlap(item: DashboardWidgetPosition, placed: DashboardWidgetPosition[]): boolean {
  return placed.some((other) => overlaps(item, other));
}

function findNearestSlot(
  item: DashboardWidgetPosition,
  placed: DashboardWidgetPosition[],
  preferredX: number,
  preferredY: number
): DashboardWidgetPosition {
  const maxX = Math.max(0, GRID_COLUMNS - item.w);
  let best: DashboardWidgetPosition | null = null;
  let bestScore = Number.POSITIVE_INFINITY;

  for (let y = 0; y <= GRID_MAX_Y; y += 1) {
    for (let x = 0; x <= maxX; x += 1) {
      const candidate = { ...item, x, y };
      if (hasOverlap(candidate, placed)) continue;
      const score = Math.abs(y - preferredY) * 100 + Math.abs(x - preferredX);
      if (score < bestScore) {
        bestScore = score;
        best = candidate;
        if (score === 0) return candidate;
      }
    }
  }

  if (best) return best;
  // Fallback: place below all existing widgets to guarantee no overlap
  const maxUsedY = placed.reduce((max, p) => Math.max(max, p.y + p.h), 0);
  return {
    ...item,
    x: 0,
    y: maxUsedY
  };
}

function packLayout(items: DashboardWidgetPosition[], prioritizeId?: string): DashboardWidgetPosition[] {
  const normalized = items.map((item) => {
    const w = clampSpan(item.w, 2, GRID_COLUMNS);
    return {
      ...item,
      w,
      h: clampSpan(item.h, 2, GRID_MAX_H),
      x: clampSpan(item.x, 0, Math.max(0, GRID_COLUMNS - w)),
      y: clampSpan(item.y, 0, GRID_MAX_Y)
    };
  });

  const ordered = normalized.slice().sort((a, b) => {
    if (a.id === prioritizeId) return -1;
    if (b.id === prioritizeId) return 1;
    return a.y - b.y || a.x - b.x;
  });

  const placed: DashboardWidgetPosition[] = [];
  for (const item of ordered) {
    const next = findNearestSlot(item, placed, item.x, item.y);
    placed.push(next);
  }

  return placed;
}

function normalizeLayout(items: DashboardWidgetPosition[]): DashboardWidgetPosition[] {
  return packLayout(items);
}

function compactLayout(items: DashboardWidgetPosition[]): DashboardWidgetPosition[] {
  const normalized = items
    .map((item) => {
      const w = clampSpan(item.w, 2, GRID_COLUMNS);
      return {
        ...item,
        w,
        h: clampSpan(item.h, 2, GRID_MAX_H),
        x: clampSpan(item.x, 0, Math.max(0, GRID_COLUMNS - w)),
        y: clampSpan(item.y, 0, GRID_MAX_Y)
      };
    })
    .sort((a, b) => a.y - b.y || a.x - b.x);

  const placed: DashboardWidgetPosition[] = [];
  for (const item of normalized) {
    const next = findNearestSlot(item, placed, 0, 0);
    placed.push(next);
  }
  return placed;
}

function sanitizeLayout(items: DashboardWidgetPosition[], allowed: Set<string>): DashboardWidgetPosition[] {
  return compactLayout(items.filter((item) => allowed.has(baseWidgetId(item.id))));
}

function normalizeWidgetConfig(config: Record<string, DashboardWidgetConfig>): Record<string, DashboardWidgetConfig> {
  const normalized: Record<string, DashboardWidgetConfig> = {};
  for (const widgetId of Object.keys(config).sort()) {
    const entry = config[widgetId];
    const service = entry?.service?.trim() || "";
    const since = entry?.since?.trim() || "";
    const severity = entry?.severity?.trim() || "";
    const locked = Boolean(entry?.locked);
    if (!service && !since && !severity && !locked) continue;
    normalized[widgetId] = {};
    if (service) normalized[widgetId].service = service;
    if (since) normalized[widgetId].since = since;
    if (severity) normalized[widgetId].severity = severity;
    if (locked) normalized[widgetId].locked = true;
  }
  return normalized;
}

function nextY(layout: DashboardWidgetPosition[]): number {
  if (!layout.length) return 0;
  return Math.max(...layout.map((item) => item.y + item.h));
}

function healthTone(healthy: number, degraded: number, unhealthy: number): "ok" | "warn" | "error" {
  if (unhealthy > 0) return "error";
  if (degraded > 0) return "warn";
  if (healthy > 0) return "ok";
  return "warn";
}

function baseWidgetId(id: string): string {
  return id.split(WIDGET_INSTANCE_SEP)[0];
}

export function DashboardsPage() {
  let canvasRef: HTMLDivElement | undefined;
  let importInputRef: HTMLInputElement | undefined;
  const [notice, setNotice] = createSignal("");
  const [selectedDashboardId, setSelectedDashboardId] = createSignal("");
  const [canvasName, setCanvasName] = createSignal(DEFAULT_DASHBOARD_NAME);
  const [layoutRaw, setLayoutRaw] = createSignal<DashboardWidgetPosition[]>(defaultLayout);
  // Validated layout setter: always ensures no overlaps
  const layout = (): DashboardWidgetPosition[] => layoutRaw();
  const setLayout = (value: DashboardWidgetPosition[] | ((prev: DashboardWidgetPosition[]) => DashboardWidgetPosition[])) => {
    setLayoutRaw((prev) => {
      const next = typeof value === "function" ? value(prev) : value;
      // Verify: if any overlaps exist in the result, force a repack
      for (let i = 0; i < next.length; i++) {
        for (let j = i + 1; j < next.length; j++) {
          if (overlaps(next[i], next[j])) {
            console.error("[dogwatch] setLayout produced overlap, auto-repacking", next[i].id, next[j].id);
            return packLayout(next);
          }
        }
      }
      return next;
    });
  };
  const [showPicker, setShowPicker] = createSignal(false);
  const [pickerQuery, setPickerQuery] = createSignal("");
  const [newDashboardName, setNewDashboardName] = createSignal(DEFAULT_DASHBOARD_NAME);
  const [timeRange, setTimeRange] = createSignal("1h");
  const [serviceFilter, setServiceFilter] = createSignal("");
  const [severityFilter, setSeverityFilter] = createSignal("");
  const [draggingWidgetId, setDraggingWidgetId] = createSignal("");
  const [dragOverWidgetId, setDragOverWidgetId] = createSignal("");
  const [canvasDropPoint, setCanvasDropPoint] = createSignal<{ x: number; y: number } | null>(null);
  const [dragGrabOffset, setDragGrabOffset] = createSignal<{ cols: number; rows: number }>({ cols: 0, rows: 0 });
  const [editMode, setEditMode] = createSignal(false);
  const [widgetConfig, setWidgetConfig] = createSignal<Record<string, DashboardWidgetConfig>>({});
  const [focusedWidgetId, setFocusedWidgetId] = createSignal("");
  const [showOnlyUnlocked, setShowOnlyUnlocked] = createSignal(false);
  const [selectedTemplateId, setSelectedTemplateId] = createSignal(DASHBOARD_TEMPLATES[0]?.id || "");
  const [historyPast, setHistoryPast] = createSignal<EditorSnapshot[]>([]);
  const [historyFuture, setHistoryFuture] = createSignal<EditorSnapshot[]>([]);
  const [copiedWidget, setCopiedWidget] = createSignal<CopiedWidget | null>(null);
  const [resizingWidgetId, setResizingWidgetId] = createSignal("");
  const [pendingDraft, setPendingDraft] = createSignal<{
    dashboardId: string;
    name: string;
    layout: DashboardWidgetPosition[];
    widgetConfig: Record<string, DashboardWidgetConfig>;
    savedAt: number;
  } | null>(null);

  const [dashboards, { refetch: refetchDashboards }] = createResource(loadDashboards);
  const [selectedDashboard, { refetch: refetchSelectedDashboard }] = createResource(selectedDashboardId, loadDashboard);

  const [alerts, { refetch: refetchAlerts }] = createResource(loadAlerts);
  const [incidents, { refetch: refetchIncidents }] = createResource(loadIncidents);
  const logsQuery = createMemo(() => ({
    since: "24h",
    service: "",
    level: "",
    limit: 300
  }));
  const [logs, { refetch: refetchLogs }] = createResource(logsQuery, loadLogs);
  const [catalogStats, { refetch: refetchCatalogStats }] = createResource(loadCatalogStats);
  const [catalogServices, { refetch: refetchCatalogServices }] = createResource(() => loadCatalogServices({}));
  const [correlations, { refetch: refetchCorrelations }] = createResource(() => loadDeployIncidentCorrelations("24h"));
  const [schedules, { refetch: refetchSchedules }] = createResource(loadOncallSchedules);
  const [policies, { refetch: refetchPolicies }] = createResource(loadOncallPolicies);
  const [currentOncall, { refetch: refetchCurrentOncall }] = createResource(
    () => schedules()?.[0]?.id || "",
    loadCurrentOncall
  );
  const [k8sSummary, { refetch: refetchK8sSummary }] = createResource(loadK8sSummary);
  const [channels, { refetch: refetchChannels }] = createResource(loadNotifyChannels);
  const [history, { refetch: refetchHistory }] = createResource(() => loadNotifyHistory(""));
  const [systemMetrics, { refetch: refetchSystemMetrics }] = createResource(loadSystemMetrics);
  const [systemHistory, { refetch: refetchSystemHistory }] = createResource(
    () => timeRange(),
    (duration) => loadSystemHistory(duration)
  );
  const [statsSummary, { refetch: refetchStatsSummary }] = createResource(loadStatsSummary);
  const [topProcesses, { refetch: refetchTopProcesses }] = createResource(loadTopProcesses);
  const [serviceMap, { refetch: refetchServiceMap }] = createResource(loadServiceMap);
  const [traceSummaries, { refetch: refetchTraceSummaries }] = createResource(
    () => ({ limit: 60, service: serviceFilter(), duration: timeRange() }),
    (params) => loadTraceSummaries(params.limit, params.service, params.duration)
  );
  const [traceServices, { refetch: refetchTraceServices }] = createResource(loadTraceServices);
  const [traceDependencies, { refetch: refetchTraceDependencies }] = createResource(loadTraceDependencies);
  const [slos, { refetch: refetchSlos }] = createResource(loadSlos);
  const [syntheticChecks, { refetch: refetchSyntheticChecks }] = createResource(loadSyntheticChecks);
  const [syntheticFailures, { refetch: refetchSyntheticFailures }] = createResource(loadSyntheticFailures);
  const [costEstimate, { refetch: refetchCostEstimate }] = createResource(loadCostEstimate);
  const [cardinalityHotspots, { refetch: refetchCardinalityHotspots }] = createResource(loadCardinalityHotspots);
  const [costRecommendations, { refetch: refetchCostRecommendations }] = createResource(loadCostRecommendations);
  const [anomalies, { refetch: refetchAnomalies }] = createResource(loadAnomalies);
  const [dbQueries, { refetch: refetchDbQueries }] = createResource(loadDbQueries);
  const [flamegraphHotspots, { refetch: refetchFlamegraphHotspots }] = createResource(loadFlamegraphHotspots);
  const [watches, { refetch: refetchWatches }] = createResource(loadWatches);
  const [silences, { refetch: refetchSilences }] = createResource(loadSilences);
  const [logPatterns, { refetch: refetchLogPatterns }] = createResource(loadLogPatterns);
  const [trendingPatterns, { refetch: refetchTrendingPatterns }] = createResource(loadTrendingPatterns);
  const [deploys, { refetch: refetchDeploys }] = createResource(loadDeploys);
  const [auditLogs, { refetch: refetchAuditLogs }] = createResource(loadRecentAuditLogs);
  const [apiKeys, { refetch: refetchApiKeys }] = createResource(loadApiKeys);
  const [backups, { refetch: refetchBackups }] = createResource(loadBackups);
  const [k8sContainers, { refetch: refetchK8sContainers }] = createResource(() => loadK8sContainers(""));
  const [k8sPodList, { refetch: refetchK8sPodList }] = createResource(() => loadK8sPods(""));
  const [k8sDeployments, { refetch: refetchK8sDeployments }] = createResource(() => loadK8sDeployments(""));
  const [k8sEvents, { refetch: refetchK8sEvents }] = createResource(() => loadK8sEvents(""));
  const [traceSpans, { refetch: refetchTraceSpans }] = createResource(
    () => (traceSummaries() || [])[0]?.trace_id || "",
    loadTraceSpans
  );
  const [logComparison, { refetch: refetchLogComparison }] = createResource(() => loadLogComparison("", "1h", "now"));
  const [deployStats, { refetch: refetchDeployStats }] = createResource(loadDeployStats);
  const [slowQueries, { refetch: refetchSlowQueries }] = createResource(loadSlowQueries);
  const [costQuickWins, { refetch: refetchCostQuickWins }] = createResource(loadCostQuickWins);

  onMount(() => {
    if (typeof window === "undefined") return;
    const raw = window.localStorage.getItem(EDIT_MODE_PREF_KEY);
    if (raw === "1") setEditMode(true);

    try {
      const draftRaw = window.localStorage.getItem(DRAFT_STORAGE_KEY);
      if (draftRaw) {
        const draft = JSON.parse(draftRaw);
        if (draft && draft.savedAt && Date.now() - draft.savedAt < DRAFT_MAX_AGE_MS) {
          setPendingDraft(draft);
        } else {
          window.localStorage.removeItem(DRAFT_STORAGE_KEY);
        }
      }
    } catch {
      window.localStorage.removeItem(DRAFT_STORAGE_KEY);
    }

    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const tag = (target?.tagName || "").toLowerCase();
      const inTextInput = tag === "input" || tag === "textarea" || tag === "select" || Boolean(target?.isContentEditable);
      const key = e.key.toLowerCase();
      const mod = e.metaKey || e.ctrlKey;

      if (mod && key === "s") {
        e.preventDefault();
        if (selectedDashboardId() && dirty()) void onSaveCurrent();
        return;
      }

      if (!inTextInput && !mod && !e.altKey && key === "e") {
        e.preventDefault();
        setEditMode((v) => !v);
        return;
      }

      const redoCombo = mod && ((e.shiftKey && key === "z") || key === "y");
      if (redoCombo) {
        e.preventDefault();
        redoHistory();
        return;
      }

      if (mod && !e.shiftKey && key === "z") {
        e.preventDefault();
        undoHistory();
        return;
      }

      if (!inTextInput && editMode() && focusedWidgetId()) {
        const widgetId = focusedWidgetId();
        if (!mod && !e.altKey && key === "l") {
          e.preventDefault();
          setWidgetLocked(widgetId, !isWidgetLocked(widgetId));
          return;
        }
        if (mod && key === "d") {
          e.preventDefault();
          duplicateWidget(widgetId);
          return;
        }
        if (mod && key === "c") {
          e.preventDefault();
          copyWidget(widgetId);
          return;
        }
        if (mod && key === "v") {
          e.preventDefault();
          pasteWidget();
          return;
        }
              if (e.shiftKey) {
                if (e.key === "ArrowLeft") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  resizeWidget(widgetId, -1, 0);
                  return;
                }
                if (e.key === "ArrowRight") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  resizeWidget(widgetId, 1, 0);
                  return;
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  resizeWidget(widgetId, 0, -1);
                  return;
                }
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  resizeWidget(widgetId, 0, 1);
                  return;
                }
              } else {
                if (e.key === "ArrowLeft") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  nudgeWidget(widgetId, -1, 0);
                  return;
                }
                if (e.key === "ArrowRight") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  nudgeWidget(widgetId, 1, 0);
                  return;
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  nudgeWidget(widgetId, 0, -1);
                  return;
                }
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  if (isWidgetLocked(widgetId)) return;
                  nudgeWidget(widgetId, 0, 1);
                }
              }
      }
    };

    window.addEventListener("keydown", onKeyDown);
    onCleanup(() => window.removeEventListener("keydown", onKeyDown));
  });

  createEffect(() => {
    const draft = pendingDraft();
    if (!draft) return;
    const currentId = selectedDashboardId();
    if (!currentId && !draft.dashboardId) {
      setCanvasName(draft.name);
      setLayout(packLayout(draft.layout));
      setWidgetConfig(draft.widgetConfig);
      setPendingDraft(null);
      setNotice("Recovered unsaved dashboard changes.");
      return;
    }
    if (currentId && currentId === draft.dashboardId) {
      setCanvasName(draft.name);
      setLayout(packLayout(draft.layout));
      setWidgetConfig(draft.widgetConfig);
      setPendingDraft(null);
      setNotice("Recovered unsaved dashboard changes.");
    }
  });

  {
    let draftTimer: ReturnType<typeof setTimeout> | undefined;
    createEffect(() => {
      const currentLayout = layout();
      const currentConfig = widgetConfig();
      const currentName = canvasName();
      const dashboardId = selectedDashboardId();

      if (draftTimer) clearTimeout(draftTimer);
      draftTimer = setTimeout(() => {
        if (typeof window === "undefined") return;
        try {
          window.localStorage.setItem(DRAFT_STORAGE_KEY, JSON.stringify({
            dashboardId,
            name: currentName,
            layout: currentLayout,
            widgetConfig: currentConfig,
            savedAt: Date.now(),
          }));
        } catch {
          // localStorage full or unavailable — ignore
        }
      }, DRAFT_DEBOUNCE_MS);
    });
    onCleanup(() => { if (draftTimer) clearTimeout(draftTimer); });
  }

  createEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(EDIT_MODE_PREF_KEY, editMode() ? "1" : "0");
  });

  useAutoRefresh(() => {
    refetchAlerts();
    refetchIncidents();
    refetchLogs();
    refetchCatalogStats();
    refetchCatalogServices();
    refetchCorrelations();
    refetchSchedules();
    refetchPolicies();
    refetchCurrentOncall();
    refetchK8sSummary();
    refetchChannels();
    refetchHistory();
    refetchSystemMetrics();
    refetchSystemHistory();
    refetchStatsSummary();
    refetchTopProcesses();
    refetchServiceMap();
    refetchTraceSummaries();
    refetchTraceServices();
    refetchTraceDependencies();
    refetchSlos();
    refetchSyntheticChecks();
    refetchSyntheticFailures();
    refetchCostEstimate();
    refetchCardinalityHotspots();
    refetchCostRecommendations();
    refetchAnomalies();
    refetchDbQueries();
    refetchFlamegraphHotspots();
    refetchWatches();
    refetchSilences();
    refetchLogPatterns();
    refetchTrendingPatterns();
    refetchDeploys();
    refetchAuditLogs();
    refetchApiKeys();
    refetchBackups();
    refetchK8sContainers();
    refetchK8sPodList();
    refetchK8sDeployments();
    refetchK8sEvents();
    refetchTraceSpans();
    refetchLogComparison();
    refetchDeployStats();
    refetchSlowQueries();
    refetchCostQuickWins();
  }, 30000);

  createEffect(() => {
    const list = dashboards() || [];
    if (!list.length || selectedDashboardId()) return;
    const preferred = list.find((item) => item.isDefault) || list[0];
    setSelectedDashboardId(preferred.id);
  });

  createEffect(() => {
    const dashboard = selectedDashboard();
    if (!dashboard) return;
    const allowed = new Set(WIDGETS.map((w) => w.id));
    const nextLayout = sanitizeLayout(dashboard.layout.length ? dashboard.layout : defaultLayout, allowed);
    setCanvasName(dashboard.name);
    setLayout(nextLayout);
    setWidgetConfig(dashboard.widgetConfig || {});
    setFocusedWidgetId("");
    setResizingWidgetId("");
    setHistoryPast([]);
    setHistoryFuture([]);
  });

  const widgetDefsById = createMemo(() => new Map(WIDGETS.map((w) => [w.id, w] as const)));

  const activeLayout = createMemo(() =>
    layout()
      .filter((item) => widgetDefsById().has(baseWidgetId(item.id)))
      .slice()
      .sort((a, b) => (a.y - b.y) || (a.x - b.x))
  );
  const visibleLayoutRaw = createMemo(() =>
    showOnlyUnlocked() ? activeLayout().filter((item) => !isWidgetLocked(item.id)) : activeLayout()
  );
  // Render-level safety: guarantee no overlapping items reach the DOM
  const visibleLayout = createMemo(() => {
    const items = visibleLayoutRaw();
    for (let i = 0; i < items.length; i++) {
      for (let j = i + 1; j < items.length; j++) {
        if (overlaps(items[i], items[j])) {
          console.error("[dogwatch] render-level overlap detected, repacking", items[i].id, items[j].id);
          return packLayout(items);
        }
      }
    }
    return items;
  });
  const dropPoint = createMemo(() => canvasDropPoint());
  const normalizedCurrentConfig = createMemo(() => normalizeWidgetConfig(widgetConfig()));

  const selectedDashboardName = createMemo(() => {
    const id = selectedDashboardId();
    const found = (dashboards() || []).find((item) => item.id === id);
    return found?.name || "Unsaved dashboard";
  });

  const selectedLayoutSnapshot = createMemo(() => {
    const loaded = selectedDashboard();
    if (!loaded) return normalizeLayout(defaultLayout);
    return normalizeLayout((loaded.layout || []).slice().sort((a, b) => a.y - b.y || a.x - b.x));
  });

  const selectedConfigSnapshot = createMemo(() => {
    const loaded = selectedDashboard();
    if (!loaded) return {} as Record<string, DashboardWidgetConfig>;
    return normalizeWidgetConfig(loaded.widgetConfig || {});
  });

  const dirty = createMemo(() => {
    const currentLayout = normalizeLayout(activeLayout().slice());
    const currentLayoutJson = JSON.stringify(currentLayout);
    const currentConfigJson = JSON.stringify(normalizedCurrentConfig());
    const currentName = canvasName().trim();

    if (selectedDashboard()) {
      const savedLayoutJson = JSON.stringify(selectedLayoutSnapshot());
      const savedConfigJson = JSON.stringify(selectedConfigSnapshot());
      const savedName = selectedDashboard()!.name.trim();
      return currentName !== savedName || currentLayoutJson !== savedLayoutJson || currentConfigJson !== savedConfigJson;
    }

    const baseLayoutJson = JSON.stringify(normalizeLayout(defaultLayout.slice()));
    const baseConfigJson = JSON.stringify({});
    return currentName !== DEFAULT_DASHBOARD_NAME || currentLayoutJson !== baseLayoutJson || currentConfigJson !== baseConfigJson;
  });
  const canUndo = createMemo(() => historyPast().length > 0);
  const canRedo = createMemo(() => historyFuture().length > 0);

  const availableToAdd = createMemo(() => {
    const q = pickerQuery().trim().toLowerCase();
    if (!q) return WIDGETS;
    return WIDGETS.filter(
      (widget) =>
        widget.title.toLowerCase().includes(q) ||
        widget.id.toLowerCase().includes(q) ||
        widget.description.toLowerCase().includes(q)
    );
  });
  const shortcutHint = createMemo(() => {
    if (typeof navigator !== "undefined" && /mac/i.test(navigator.platform)) {
      return "drag + resize handles (hold ⌥ to bypass auto-pack), L lock, ⌘S/⌘Z/⇧⌘Z, ⌘D/⌘C/⌘V";
    }
    return "drag + resize handles (hold Alt to bypass auto-pack), L lock, Ctrl+S/Ctrl+Z/Ctrl+Y, Ctrl+D/C/V";
  });
  const focusedWidget = createMemo(() => activeLayout().find((item) => item.id === focusedWidgetId()) || null);

  const serviceOptions = createMemo(() => {
    const set = new Set<string>();
    for (const alert of alerts() || []) set.add(alert.service);
    for (const incident of incidents() || []) set.add(incident.service);
    for (const log of logs() || []) if (log.service) set.add(log.service);
    for (const service of catalogServices() || []) set.add(service.name);
    for (const service of traceServices() || []) set.add(service);
    return Array.from(set).filter(Boolean).sort();
  });

  createEffect(() => {
    if (typeof window === "undefined" || !dirty()) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    onCleanup(() => window.removeEventListener("beforeunload", onBeforeUnload));
  });
  createEffect(() => {
    const items = layout();
    if (items.length < 2) return;
    let hasOverlaps = false;
    for (let i = 0; i < items.length && !hasOverlaps; i++) {
      for (let j = i + 1; j < items.length; j++) {
        if (overlaps(items[i], items[j])) {
          hasOverlaps = true;
          break;
        }
      }
    }
    if (hasOverlaps) {
      console.warn("[dogwatch] overlap detected, repacking");
      const packed = untrack(() => packLayout(items));
      batch(() => setLayout(packed));
    }
  });
  createEffect(() => {
    if (!focusedWidgetId()) return;
    if (!activeLayout().some((item) => item.id === focusedWidgetId())) {
      setFocusedWidgetId("");
    }
  });
  createEffect(() => {
    if (!showOnlyUnlocked() || !focusedWidgetId()) return;
    if (isWidgetLocked(focusedWidgetId())) setFocusedWidgetId("");
  });

  function sinceToMs(since: string): number {
    if (since === "15m") return 15 * 60_000;
    if (since === "6h") return 6 * 3_600_000;
    if (since === "24h") return 24 * 3_600_000;
    return 3_600_000;
  }

  function resolveWidgetService(widgetId: string): string {
    return widgetConfig()[widgetId]?.service || serviceFilter();
  }

  function resolveWidgetSince(widgetId: string): string {
    return widgetConfig()[widgetId]?.since || timeRange();
  }

  function resolveWidgetSeverity(widgetId: string): string {
    return widgetConfig()[widgetId]?.severity || severityFilter();
  }

  function isWidgetLocked(widgetId: string): boolean {
    return Boolean(widgetConfig()[widgetId]?.locked);
  }

  function isWidgetLoading(widgetTypeId: string): boolean {
    const loadingMap: Record<string, () => boolean> = {
      "system-overview": () => systemMetrics.loading,
      "traffic-overview": () => statsSummary.loading,
      "endpoint-latency": () => statsSummary.loading,
      "connection-hotspots": () => statsSummary.loading,
      "process-top": () => topProcesses.loading,
      "service-map-health": () => serviceMap.loading,
      "trace-throughput": () => traceSummaries.loading,
      "trace-services": () => traceServices.loading,
      "trace-dependencies": () => traceDependencies.loading,
      "kpi-reliability": () => alerts.loading || catalogStats.loading,
      "alerts-feed": () => alerts.loading,
      "alerts-severity-map": () => alerts.loading,
      "incidents-live": () => incidents.loading,
      "incidents-by-commander": () => incidents.loading,
      "logs-errors": () => logs.loading,
      "deploy-correlation": () => correlations.loading,
      "service-health": () => catalogServices.loading,
      "service-latency-top": () => catalogServices.loading,
      "oncall-now": () => currentOncall.loading || schedules.loading || policies.loading,
      "k8s-cluster": () => k8sSummary.loading,
      "k8s-capacity-risk": () => k8sSummary.loading,
      "notify-delivery": () => channels.loading || history.loading,
      "notify-failure-log": () => history.loading,
      "ops-action-queue": () => alerts.loading || incidents.loading,
      "command-links": () => false,
      "slo-burn-rate": () => slos.loading,
      "slo-budget-remaining": () => slos.loading,
      "synthetic-uptime": () => syntheticChecks.loading,
      "synthetic-failures": () => syntheticFailures.loading,
      "cost-estimate": () => costEstimate.loading,
      "cardinality-hotspots": () => cardinalityHotspots.loading,
      "cost-recommendations": () => costRecommendations.loading,
      "perf-anomalies": () => anomalies.loading,
      "perf-db-queries": () => dbQueries.loading,
      "perf-flamegraph-top": () => flamegraphHotspots.loading,
      "alerts-watches": () => watches.loading,
      "alerts-silences": () => silences.loading,
      "logs-patterns": () => logPatterns.loading,
      "logs-trending": () => trendingPatterns.loading,
      "deploy-feed": () => deploys.loading,
      "admin-audit-feed": () => auditLogs.loading,
      "admin-api-keys": () => apiKeys.loading,
      "admin-backup-status": () => backups.loading,
      "k8s-containers": () => k8sContainers.loading,
      "k8s-pods": () => k8sPodList.loading,
      "k8s-deployments": () => k8sDeployments.loading,
      "k8s-events": () => k8sEvents.loading,
      "endpoint-detail": () => statsSummary.loading,
      "connection-detail": () => statsSummary.loading,
      "trace-detail": () => traceSpans.loading,
      "log-compare": () => logComparison.loading,
      "deploy-stats": () => deployStats.loading,
      "perf-slow-queries": () => slowQueries.loading,
      "cost-quick-wins": () => costQuickWins.loading,
    };
    const check = loadingMap[widgetTypeId];
    return check ? check() : false;
  }

  function isWithinSince(rawTs: string | undefined, since: string): boolean {
    if (!rawTs) return true;
    const ts = new Date(rawTs).getTime();
    if (!Number.isFinite(ts)) return true;
    return Date.now() - ts <= sinceToMs(since);
  }

  function matchesSeverity(level: string, wanted: string): boolean {
    if (!wanted) return true;
    const value = (level || "").toLowerCase();
    if (value === wanted) return true;
    if (wanted === "critical") return value === "critical" || value === "fatal" || value === "error";
    if (wanted === "high") return value === "high" || value === "error" || value === "fatal";
    if (wanted === "medium") return value === "medium" || value === "warn" || value === "warning";
    if (wanted === "low") return value === "low" || value === "info" || value === "debug" || value === "trace";
    return true;
  }

  function filterAlertsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    const severity = resolveWidgetSeverity(widgetId);
    return (alerts() || []).filter(
      (a) => (!svc || a.service === svc) && matchesSeverity(a.severity, severity) && isWithinSince(a.startedAtRaw, since)
    );
  }

  function filterIncidentsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    const severity = resolveWidgetSeverity(widgetId);
    return (incidents() || []).filter(
      (i) => (!svc || i.service === svc) && matchesSeverity(i.severity, severity) && isWithinSince(i.startedAtRaw, since)
    );
  }

  function filterLogsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const horizon = sinceToMs(resolveWidgetSince(widgetId));
    const severity = resolveWidgetSeverity(widgetId);
    const now = Date.now();
    return (logs() || []).filter((row) => {
      if (svc && row.service !== svc) return false;
      if (!matchesSeverity(row.level, severity)) return false;
      const ts = new Date(row.timestamp).getTime();
      if (!Number.isFinite(ts)) return false;
      return now - ts <= horizon;
    });
  }

  function filterCorrelationsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    const severity = resolveWidgetSeverity(widgetId);
    return (correlations() || []).filter((corr) => {
      if (svc && corr.deployment.service !== svc) return false;
      if (severity) {
        const level = corr.confidence >= 0.75 ? "critical" : corr.confidence >= 0.5 ? "high" : corr.confidence >= 0.25 ? "medium" : "low";
        if (!matchesSeverity(level, severity)) return false;
      }
      return isWithinSince(corr.deployment.timestamp, since);
    });
  }

  function filterServicesForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const severity = resolveWidgetSeverity(widgetId);
    return (catalogServices() || []).filter((row) => {
      if (svc && row.name !== svc && row.displayName !== svc) return false;
      if (!severity) return true;
      const healthLevel = row.health === "unhealthy" ? "critical" : row.health === "degraded" ? "high" : "low";
      return matchesSeverity(healthLevel, severity);
    });
  }

  function widgetDefForId(id: string): WidgetDef | undefined {
    return widgetDefsById().get(baseWidgetId(id));
  }

  function nextWidgetInstanceId(baseId: string): string {
    const ids = new Set(layout().map((item) => item.id));
    if (!ids.has(baseId)) return baseId;
    let n = 2;
    while (ids.has(`${baseId}${WIDGET_INSTANCE_SEP}${n}`)) n += 1;
    return `${baseId}${WIDGET_INSTANCE_SEP}${n}`;
  }

  function snapshotState(): EditorSnapshot {
    return {
      layout: normalizeLayout(activeLayout().slice()),
      widgetConfig: normalizeWidgetConfig(widgetConfig())
    };
  }

  function pushHistory() {
    const snap = snapshotState();
    setHistoryPast((curr) => {
      const next = [...curr, snap];
      if (next.length > MAX_HISTORY) next.shift();
      return next;
    });
    setHistoryFuture([]);
  }

  function applySnapshot(snap: EditorSnapshot) {
    setLayout(normalizeLayout(snap.layout.slice()));
    setWidgetConfig(normalizeWidgetConfig(snap.widgetConfig));
    setFocusedWidgetId("");
  }

  function undoHistory() {
    const past = historyPast();
    if (!past.length) return;
    const prev = past[past.length - 1];
    const current = snapshotState();
    setHistoryPast(past.slice(0, -1));
    setHistoryFuture((curr) => [current, ...curr].slice(0, MAX_HISTORY));
    applySnapshot(prev);
    setNotice("Undid last dashboard edit.");
  }

  function redoHistory() {
    const future = historyFuture();
    if (!future.length) return;
    const next = future[0];
    const current = snapshotState();
    setHistoryFuture(future.slice(1));
    setHistoryPast((curr) => [...curr, current].slice(-MAX_HISTORY));
    applySnapshot(next);
    setNotice("Redid dashboard edit.");
  }

  function addWidget(widgetId: string) {
    const baseId = baseWidgetId(widgetId);
    const widget = WIDGETS.find((w) => w.id === baseId);
    if (!widget) return;
    const id = nextWidgetInstanceId(baseId);
    pushHistory();
    setLayout((curr) =>
      normalizeLayout([
        ...curr,
        {
          id,
          x: 0,
          y: nextY(curr),
          w: widget.defaultW,
          h: widget.defaultH
        }
      ])
    );
    setFocusedWidgetId(id);
    setNotice(`Added ${widget.title}.`);
  }

  function addServicePack() {
    const service = serviceFilter() || serviceOptions()[0] || "";
    if (!service) {
      setNotice("Choose a service filter first to add a scoped service pack.");
      return;
    }
    const pack = [
      { id: "alerts-feed", w: 4, h: 2 },
      { id: "logs-errors", w: 6, h: 2 },
      { id: "deploy-correlation", w: 6, h: 2 },
      { id: "service-health", w: 4, h: 2 }
    ];

    pushHistory();

    const createdIds: string[] = [];
    setLayout((curr) => {
      const next = curr.slice();
      let y = nextY(next);
      for (const item of pack) {
        const id = nextWidgetInstanceId(item.id);
        createdIds.push(id);
        const width = item.w;
        const x = width >= 6 ? 0 : 8;
        next.push({ id, x, y, w: width, h: item.h });
        y += item.h;
      }
      return normalizeLayout(next);
    });
    setWidgetConfig((curr) => {
      const next = { ...curr };
      for (const id of createdIds) {
        next[id] = { ...(next[id] || {}), service };
      }
      return next;
    });
    setFocusedWidgetId(createdIds[0] || "");
    setNotice(`Added service pack for ${service}.`);
  }

  function setWidgetSince(widgetId: string, since: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), since } }));
  }

  function setWidgetService(widgetId: string, service: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), service } }));
  }

  function setWidgetSeverity(widgetId: string, severity: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), severity } }));
  }

  function setWidgetLocked(widgetId: string, locked: boolean) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), locked } }));
    setNotice(locked ? `Locked ${widgetTitle(widgetId)}.` : `Unlocked ${widgetTitle(widgetId)}.`);
  }

  function lockAllWidgets() {
    const ids = activeLayout().map((item) => item.id);
    if (!ids.length) return;
    pushHistory();
    setWidgetConfig((curr) => {
      const next = { ...curr };
      for (const id of ids) next[id] = { ...(next[id] || {}), locked: true };
      return next;
    });
    setNotice("Locked all widgets.");
  }

  function unlockAllWidgets() {
    const ids = activeLayout().map((item) => item.id);
    if (!ids.length) return;
    pushHistory();
    setWidgetConfig((curr) => {
      const next = { ...curr };
      for (const id of ids) {
        const current = next[id] || {};
        next[id] = { ...current, locked: false };
      }
      return next;
    });
    setNotice("Unlocked all widgets.");
  }

  function clearWidgetScope(widgetId: string) {
    pushHistory();
    setWidgetConfig((curr) => {
      const next = { ...curr };
      delete next[widgetId];
      return next;
    });
  }

  function removeWidget(widgetId: string) {
    if (isWidgetLocked(widgetId)) {
      setNotice("Widget is locked. Unlock in inspector to remove.");
      return;
    }
    const widget = widgetDefForId(widgetId);
    pushHistory();
    setLayout((curr) => compactLayout(curr.filter((item) => item.id !== widgetId)));
    setWidgetConfig((curr) => {
      if (!(widgetId in curr)) return curr;
      const next = { ...curr };
      delete next[widgetId];
      return next;
    });
    if (focusedWidgetId() === widgetId) setFocusedWidgetId("");
    if (widget) setNotice(`Removed ${widget.title}.`);
  }

  function duplicateWidget(widgetId: string) {
    const src = layout().find((item) => item.id === widgetId);
    if (!src) return;
    const baseId = baseWidgetId(widgetId);
    const def = widgetDefForId(widgetId);
    const id = nextWidgetInstanceId(baseId);
    const cfg = widgetConfig()[widgetId];
    pushHistory();
    setLayout((curr) =>
      normalizeLayout([
        ...curr,
        {
          id,
          x: clampSpan(src.x + 1, 0, Math.max(0, 12 - src.w)),
          y: clampSpan(src.y + 1, 0, GRID_MAX_Y),
          w: src.w,
          h: src.h
        }
      ])
    );
    if (cfg) setWidgetConfig((curr) => ({ ...curr, [id]: { ...cfg, locked: false } }));
    setFocusedWidgetId(id);
    setNotice(`Duplicated ${def?.title || baseId}.`);
  }

  function copyWidget(widgetId: string) {
    const src = layout().find((item) => item.id === widgetId);
    if (!src) return;
    setCopiedWidget({
      baseId: baseWidgetId(widgetId),
      w: src.w,
      h: src.h,
      config: { ...(widgetConfig()[widgetId] || {}) }
    });
    setNotice(`Copied ${widgetTitle(widgetId)}.`);
  }

  function pasteWidget() {
    const copied = copiedWidget();
    if (!copied) return;
    const id = nextWidgetInstanceId(copied.baseId);
    const def = widgetDefForId(copied.baseId);
    const focus = focusedWidget();
    const x = focus ? clampSpan(focus.x + 1, 0, Math.max(0, 12 - copied.w)) : 0;
    const y = focus ? clampSpan(focus.y + 1, 0, GRID_MAX_Y) : nextY(layout());
    pushHistory();
    setLayout((curr) =>
      normalizeLayout([
        ...curr,
        { id, x, y, w: copied.w, h: copied.h }
      ])
    );
    if (copied.config && (copied.config.service || copied.config.since)) {
      setWidgetConfig((curr) => ({ ...curr, [id]: { ...copied.config } }));
    }
    setFocusedWidgetId(id);
    setNotice(`Pasted ${def?.title || copied.baseId}.`);
  }

  function updateWidget(widgetId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    if (isWidgetLocked(widgetId)) {
      setNotice("Widget is locked. Unlock in inspector to edit.");
      return;
    }
    pushHistory();
    setLayout((curr) => packLayout(curr.map((item) => (item.id === widgetId ? fn(item) : item)), widgetId));
  }

  function applyWidgetUpdate(widgetId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    setLayout((curr) => packLayout(curr.map((item) => (item.id === widgetId ? fn(item) : item)), widgetId));
  }


  function nudgeWidget(widgetId: string, dx: number, dy: number) {
    updateWidget(widgetId, (item) => ({
      ...item,
      x: clampSpan(item.x + dx, 0, Math.max(0, GRID_COLUMNS - item.w)),
      y: clampSpan(item.y + dy, 0, GRID_MAX_Y)
    }));
  }

  function resizeWidget(widgetId: string, dw: number, dh: number) {
    updateWidget(widgetId, (item) => {
      const nextW = clampSpan(item.w + dw, 2, 12);
      const nextH = clampSpan(item.h + dh, 2, GRID_MAX_H);
      return {
        ...item,
        w: nextW,
        h: nextH,
        x: clampSpan(item.x, 0, Math.max(0, 12 - nextW))
      };
    });
  }

  function startResize(widgetId: string, axis: ResizeAxis, event: MouseEvent) {
    if (!editMode() || !canvasRef) return;
    if (isWidgetLocked(widgetId)) {
      setNotice("Widget is locked. Unlock in inspector to resize.");
      return;
    }
    const target = layout().find((item) => item.id === widgetId);
    if (!target) return;

    event.preventDefault();
    event.stopPropagation();

    pushHistory();
    setFocusedWidgetId(widgetId);
    setResizingWidgetId(widgetId);

    const startX = event.clientX;
    const startY = event.clientY;
    const startW = target.w;
    const startH = target.h;
    const rect = canvasRef.getBoundingClientRect();
    const colWidth = rect.width > 0 ? rect.width / GRID_COLUMNS : 1;

    const onMove = (e: MouseEvent) => {
      const dxCols = quantizeDragDelta(e.clientX - startX, colWidth);
      const dyRows = quantizeDragDelta(e.clientY - startY, GRID_RESIZE_DRAG_STEP_PX);
      applyWidgetUpdate(widgetId, (item) => {
        const nextW = axis === "e" || axis === "se" ? clampSpan(startW + dxCols, 2, GRID_COLUMNS) : startW;
        const nextH = axis === "s" || axis === "se" ? clampSpan(startH + dyRows, 2, GRID_MAX_H) : startH;
        return {
          ...item,
          w: nextW,
          h: nextH,
          x: clampSpan(item.x, 0, Math.max(0, GRID_COLUMNS - nextW))
        };
      });
    };

    const onUp = () => {
      setResizingWidgetId("");
      setLayout((curr) => packLayout(curr, widgetId));
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };

    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }

  function swapWidgetPositions(aId: string, bId: string) {
    if (!aId || !bId || aId === bId) return;
    if (isWidgetLocked(aId) || isWidgetLocked(bId)) {
      setNotice("Cannot move locked widget.");
      return;
    }
    pushHistory();
    setLayout((curr) => {
      const a = curr.find((item) => item.id === aId);
      const b = curr.find((item) => item.id === bId);
      if (!a || !b) return curr;
      const aTargetX = clampSpan(b.x, 0, Math.max(0, GRID_COLUMNS - a.w));
      const bTargetX = clampSpan(a.x, 0, Math.max(0, GRID_COLUMNS - b.w));
      return packLayout(
        curr.map((item) => {
          if (item.id === aId) return { ...item, x: aTargetX, y: b.y };
          if (item.id === bId) return { ...item, x: bTargetX, y: a.y };
          return item;
        }),
        aId
      );
    });
    setNotice(`Moved ${widgetTitle(aId)}.`);
  }

  function onWidgetDragStart(widgetId: string, e: DragEvent) {
    if (isWidgetLocked(widgetId)) {
      setNotice("Widget is locked. Unlock in inspector to move.");
      return;
    }
    // Compute grab offset in grid cells using the same column math as canvasPointToGrid
    const widget = layout().find((item) => item.id === widgetId);
    if (canvasRef && widget) {
      const canvasRect = canvasRef.getBoundingClientRect();
      const colWidth = canvasRect.width / GRID_COLUMNS;
      // Cursor position in grid columns, minus widget's current x = offset within widget
      const cursorCol = Math.floor((e.clientX - canvasRect.left) / colWidth);
      const cursorRow = Math.floor((e.clientY - canvasRect.top) / GRID_ROW_HEIGHT_PX);
      setDragGrabOffset({
        cols: clampSpan(cursorCol - widget.x, 0, widget.w - 1),
        rows: clampSpan(cursorRow - widget.y, 0, widget.h - 1)
      });
    } else {
      setDragGrabOffset({ cols: 0, rows: 0 });
    }
    setDraggingWidgetId(widgetId);
    setDragOverWidgetId("");
  }

  function onWidgetDragEnd() {
    setDraggingWidgetId("");
    setDragOverWidgetId("");
    setCanvasDropPoint(null);
  }

  function onWidgetDrop(targetId: string) {
    const source = draggingWidgetId();
    if (!source || source === targetId) {
      onWidgetDragEnd();
      return;
    }
    swapWidgetPositions(source, targetId);
    onWidgetDragEnd();
  }

  function canvasPointToGrid(clientX: number, clientY: number, applyGrabOffset = false): { x: number; y: number } | null {
    if (!canvasRef) return null;
    const rect = canvasRef.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const columnWidth = rect.width / GRID_COLUMNS;
    // Raw cursor position in grid cells
    const rawCol = Math.floor((clientX - rect.left) / columnWidth);
    const rawRow = Math.floor((clientY - rect.top) / GRID_ROW_HEIGHT_PX);
    // Subtract grab offset (in grid cells) so drop tracks the widget's top-left corner
    const offset = applyGrabOffset ? dragGrabOffset() : { cols: 0, rows: 0 };
    const dragged = applyGrabOffset ? layout().find((item) => item.id === draggingWidgetId()) : null;
    const maxX = dragged ? Math.max(0, GRID_COLUMNS - dragged.w) : GRID_COLUMNS - 1;
    return {
      x: clampSpan(rawCol - offset.cols, 0, maxX),
      y: clampSpan(rawRow - offset.rows, 0, GRID_MAX_Y)
    };
  }

  function moveDraggedWidgetTo(point: { x: number; y: number }) {
    const draggedId = draggingWidgetId();
    if (!draggedId) return;
    if (isWidgetLocked(draggedId)) return;
    pushHistory();
    applyWidgetUpdate(draggedId, (item) => ({
      ...item,
      x: clampSpan(point.x, 0, Math.max(0, GRID_COLUMNS - item.w)),
      y: clampSpan(point.y, 0, GRID_MAX_Y)
    }));
  }

  async function onRefreshAll() {
    await Promise.all([
      refetchDashboards(),
      refetchSelectedDashboard(),
      refetchAlerts(),
      refetchIncidents(),
      refetchLogs(),
      refetchCatalogStats(),
      refetchCatalogServices(),
      refetchCorrelations(),
      refetchSchedules(),
      refetchPolicies(),
      refetchCurrentOncall(),
      refetchK8sSummary(),
      refetchChannels(),
      refetchHistory(),
      refetchSystemMetrics(),
      refetchSystemHistory(),
      refetchStatsSummary(),
      refetchTopProcesses(),
      refetchServiceMap(),
      refetchTraceSummaries(),
      refetchTraceServices(),
      refetchTraceDependencies(),
      refetchSlos(),
      refetchSyntheticChecks(),
      refetchSyntheticFailures(),
      refetchCostEstimate(),
      refetchCardinalityHotspots(),
      refetchCostRecommendations(),
      refetchAnomalies(),
      refetchDbQueries(),
      refetchFlamegraphHotspots(),
      refetchWatches(),
      refetchSilences(),
      refetchLogPatterns(),
      refetchTrendingPatterns(),
      refetchDeploys(),
      refetchAuditLogs(),
      refetchApiKeys(),
      refetchBackups(),
      refetchK8sContainers(),
      refetchK8sPodList(),
      refetchK8sDeployments(),
      refetchK8sEvents(),
      refetchTraceSpans(),
      refetchLogComparison(),
      refetchDeployStats(),
      refetchSlowQueries(),
      refetchCostQuickWins()
    ]);
    setNotice("Dashboard and widget data refreshed.");
  }

  async function onSaveCurrent() {
    const id = selectedDashboardId();
    if (!id) {
      setNotice("No selected dashboard. Create one first.");
      return;
    }

    try {
      await updateDashboard(id, canvasName().trim() || selectedDashboardName(), activeLayout(), normalizedCurrentConfig());
      await refetchDashboards();
      await refetchSelectedDashboard();
      try { window.localStorage.removeItem(DRAFT_STORAGE_KEY); } catch {}
      setNotice("Dashboard saved.");
    } catch {
      setNotice("Failed to save dashboard.");
    }
  }

  async function onSaveAsNew() {
    const name = newDashboardName().trim();
    if (!name) {
      setNotice("Dashboard name is required.");
      return;
    }

    try {
      const created = await createDashboard(name, activeLayout(), normalizedCurrentConfig(), false);
      await refetchDashboards();
      if (created?.id) {
        setSelectedDashboardId(created.id);
      }
      setCanvasName(name);
      try { window.localStorage.removeItem(DRAFT_STORAGE_KEY); } catch {}
      setNotice("Dashboard created.");
    } catch {
      setNotice("Failed to create dashboard.");
    }
  }

  async function onMakeDefault() {
    const id = selectedDashboardId();
    if (!id) return;
    try {
      await setDefaultDashboard(id);
      await refetchDashboards();
      setNotice("Set as default dashboard.");
    } catch {
      setNotice("Failed to set default dashboard.");
    }
  }

  async function onDeleteCurrent() {
    const id = selectedDashboardId();
    if (!id) return;
    if (dirty()) {
      const ok = typeof window === "undefined" || window.confirm("This dashboard has unsaved changes. Delete anyway?");
      if (!ok) return;
    }
    try {
      await deleteDashboard(id);
      setSelectedDashboardId("");
      await refetchDashboards();
      setNotice("Dashboard deleted.");
    } catch {
      setNotice("Failed to delete dashboard.");
    }
  }

  function onSelectDashboard(nextId: string) {
    if (nextId === selectedDashboardId()) return;
    if (dirty()) {
      const ok = typeof window === "undefined" || window.confirm("You have unsaved changes. Discard and switch dashboards?");
      if (!ok) return;
    }
    setSelectedDashboardId(nextId);
  }

  function applyTemplate() {
    const template = DASHBOARD_TEMPLATES.find((item) => item.id === selectedTemplateId());
    if (!template) return;
    if (dirty()) {
      const ok = typeof window === "undefined" || window.confirm("You have unsaved changes. Apply template and keep editing?");
      if (!ok) return;
    }
    pushHistory();
    setLayout(normalizeLayout(template.layout.slice()));
    setWidgetConfig(normalizeWidgetConfig(template.widgetConfig || {}));
    setFocusedWidgetId("");
    setNotice(`Applied template: ${template.name}.`);
  }

  function exportDashboardJson() {
    const payload = {
      name: canvasName().trim() || selectedDashboardName(),
      layout: activeLayout(),
      widgetConfig: normalizedCurrentConfig()
    };
    try {
      const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      const stamp = new Date().toISOString().replace(/[:.]/g, "-");
      a.href = url;
      a.download = `dashboard-${payload.name.toLowerCase().replace(/[^a-z0-9]+/g, "-") || "export"}-${stamp}.json`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      setNotice("Exported dashboard JSON.");
    } catch {
      setNotice("Failed to export dashboard JSON.");
    }
  }

  function openImportDialog() {
    importInputRef?.click();
  }

  async function onImportDashboardFile(file: File) {
    try {
      const raw = await file.text();
      const parsed = JSON.parse(raw) as {
        name?: string;
        layout?: DashboardWidgetPosition[];
        widgetConfig?: Record<string, DashboardWidgetConfig>;
      };

      const importedLayout = Array.isArray(parsed.layout) ? parsed.layout : [];
      const filteredLayout = importedLayout
        .filter((item) => Boolean(item?.id) && widgetDefForId(item.id))
        .map((item) => ({
          id: item.id,
          x: clampSpan(item.x, 0, GRID_COLUMNS - 1),
          y: clampSpan(item.y, 0, GRID_MAX_Y),
          w: clampSpan(item.w, 2, GRID_COLUMNS),
          h: clampSpan(item.h, 2, GRID_MAX_H)
        }));

      if (!filteredLayout.length) {
        setNotice("Import failed: no valid widgets in file.");
        return;
      }

      pushHistory();
      setLayout(normalizeLayout(filteredLayout));
      setWidgetConfig(normalizeWidgetConfig(parsed.widgetConfig || {}));
      if (typeof parsed.name === "string" && parsed.name.trim()) {
        setCanvasName(parsed.name.trim());
      }
      setFocusedWidgetId("");
      setNotice(`Imported dashboard from ${file.name}.`);
    } catch {
      setNotice("Import failed: invalid JSON file.");
    } finally {
      if (importInputRef) importInputRef.value = "";
    }
  }

  function widgetTitle(id: string): string {
    const baseId = baseWidgetId(id);
    const baseTitle = WIDGETS.find((w) => w.id === baseId)?.title || baseId;
    if (id === baseId) return baseTitle;
    const suffix = id.split(WIDGET_INSTANCE_SEP)[1];
    return suffix ? `${baseTitle} #${suffix}` : baseTitle;
  }

  function renderWidget(item: DashboardWidgetPosition) {
    const widgetId = item.id;
    const widgetTypeId = baseWidgetId(widgetId);
    const listRows = Math.max(4, item.h * 2 + 1);
    const logRows = Math.max(6, item.h * 3);
    const actionRows = Math.max(6, item.h * 2 + 2);
    const widgetAlerts = filterAlertsForWidget(widgetId);
    const widgetIncidents = filterIncidentsForWidget(widgetId);
    const widgetLogs = filterLogsForWidget(widgetId);
    const widgetCorrelations = filterCorrelationsForWidget(widgetId);
    const widgetServices = filterServicesForWidget(widgetId)
      .slice()
      .sort((a, b) => {
        if (a.health === b.health) return b.incidentCount30d - a.incidentCount30d;
        const rank = (h: string) => (h === "unhealthy" ? 3 : h === "degraded" ? 2 : h === "healthy" ? 1 : 0);
        return rank(b.health) - rank(a.health);
      })
      .slice(0, listRows);
    const widgetReliability = {
      totalAlerts: widgetAlerts.length,
      criticalAlerts: widgetAlerts.filter((a) => a.severity === "critical").length,
      openIncidents: widgetIncidents.length,
      healthy: catalogStats()?.healthy || 0,
      degraded: catalogStats()?.degraded || 0,
      unhealthy: catalogStats()?.unhealthy || 0
    };
    const alertSeverityRows = [
      { key: "critical", label: "Critical", count: widgetAlerts.filter((a) => a.severity === "critical").length, tone: "error" as const },
      { key: "high", label: "High", count: widgetAlerts.filter((a) => a.severity === "high").length, tone: "warn" as const },
      { key: "medium", label: "Medium", count: widgetAlerts.filter((a) => a.severity === "medium").length, tone: "neutral" as const },
      { key: "low", label: "Low", count: widgetAlerts.filter((a) => a.severity === "low").length, tone: "ok" as const }
    ];
    const alertSeverityMax = Math.max(...alertSeverityRows.map((row) => row.count), 1);
    const pendingAlerts = widgetAlerts.filter((a) => a.state === "pending").length;
    const incidentsByCommander = Array.from(
      widgetIncidents.reduce((acc, incident) => {
        acc.set(incident.commander, (acc.get(incident.commander) || 0) + 1);
        return acc;
      }, new Map<string, number>())
    )
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .slice(0, listRows);
    const latencyHotspots = widgetServices
      .slice()
      .sort((a, b) => b.avgResponseTimeMs - a.avgResponseTimeMs)
      .slice(0, listRows);
    const endpointRows = (statsSummary()?.endpoints || [])
      .filter((row) => {
        const svc = resolveWidgetService(widgetId);
        if (!svc) return true;
        return row.path.includes(svc);
      })
      .slice()
      .sort((a, b) => b.p99_ms - a.p99_ms || b.error_rate - a.error_rate)
      .slice(0, listRows);
    const connectionRows = (statsSummary()?.connections || [])
      .slice()
      .sort((a, b) => b.count - a.count)
      .slice(0, logRows);
    const processRows = (topProcesses() || []).slice(0, listRows);
    const traceRows = (traceSummaries() || []).slice(0, logRows);
    const traceServiceRows = (traceServices() || []).slice(0, logRows);
    const traceDependencyRows = (traceDependencies() || []).slice(0, listRows);
    const failedDelivery = (history() || [])
      .filter((entry) => entry.status.toLowerCase() !== "sent")
      .slice(0, logRows);
    const k8sNodesReady = k8sSummary()?.nodesReady || 0;
    const k8sNodes = k8sSummary()?.nodes || 0;
    const k8sPodsRunning = k8sSummary()?.podsRunning || 0;
    const k8sPods = k8sSummary()?.pods || 0;
    const k8sDeployHealthy = k8sSummary()?.deploymentsHealthy || 0;
    const k8sDeploys = k8sSummary()?.deployments || 0;
    const k8sNodeReadiness = k8sNodes > 0 ? Math.round((k8sNodesReady / k8sNodes) * 100) : 0;
    const k8sPodPressure = k8sPods > 0 ? Math.round((1 - k8sPodsRunning / k8sPods) * 100) : 0;
    const k8sDeployReadiness = k8sDeploys > 0 ? Math.round((k8sDeployHealthy / k8sDeploys) * 100) : 0;
    const opsActions = [
      ...widgetIncidents
        .filter((incident) => incident.status === "triggered")
        .slice(0, Math.max(3, Math.ceil(actionRows / 2)))
        .map((incident) => ({
          id: `incident-${incident.id}`,
          label: `Acknowledge incident: ${incident.title}`,
          owner: incident.commander || "unassigned",
          tone: "error" as const
        })),
      ...widgetAlerts
        .filter((alert) => alert.severity === "critical")
        .slice(0, Math.max(3, Math.ceil(actionRows / 2)))
        .map((alert) => ({
          id: `alert-${alert.id}`,
          label: `Investigate alert: ${alert.name}`,
          owner: alert.service,
          tone: "warn" as const
        }))
    ].slice(0, actionRows);
    const widgetPulse = (() => {
      const now = Date.now();
      const horizon = sinceToMs(resolveWidgetSince(widgetId));
      const bucketCount = 12;
      const bucketSize = horizon / bucketCount;
      const buckets = Array.from({ length: bucketCount }, () => 0);
      for (const row of widgetLogs) {
        const ts = new Date(row.timestamp).getTime();
        if (!Number.isFinite(ts)) continue;
        const delta = now - ts;
        if (delta < 0 || delta > horizon) continue;
        const index = bucketCount - 1 - Math.floor(delta / bucketSize);
        const clamped = Math.max(0, Math.min(bucketCount - 1, index));
        buckets[clamped] += row.level === "error" || row.level === "fatal" ? 3 : row.level === "warn" ? 2 : 1;
      }
      const max = Math.max(...buckets, 1);
      return buckets.map((value) => Math.max(10, Math.round((value / max) * 100)));
    })();
    if (widgetTypeId === "system-overview") {
      const sys = systemMetrics();
      const historyData = (): uPlot.AlignedData => {
        const pts = systemHistory() || [];
        if (pts.length < 2) {
          const now = Math.floor(Date.now() / 1000);
          return [[now - 60, now], [sys?.cpu_usage_percent || 0, sys?.cpu_usage_percent || 0], [sys?.mem_usage_percent || 0, sys?.mem_usage_percent || 0]];
        }
        const xs = new Float64Array(pts.length);
        const cpu = new Float64Array(pts.length);
        const mem = new Float64Array(pts.length);
        for (let i = 0; i < pts.length; i++) {
          xs[i] = Math.floor(new Date(pts[i].timestamp).getTime() / 1000);
          cpu[i] = pts[i].cpu_percent;
          mem[i] = pts[i].mem_percent;
        }
        return [xs, cpu, mem];
      };
      return (
        <div class="widget-body" style={{ gap: "var(--v2-space-2)" }}>
          <ChartPanel
            data={historyData()}
            series={[
              { label: "CPU %", stroke: "#ccff00", fill: "rgba(204,255,0,0.08)", width: 1.5 },
              { label: "Memory %", stroke: "#2ed67a", fill: "rgba(46,214,122,0.06)", width: 1.5 },
            ]}
            height={120}
          />
          <div class="widget-kpi-grid" style={{ flex: "none" }}>
            <div>
              <label>CPU</label>
              <strong>{Math.round(sys?.cpu_usage_percent || 0)}%</strong>
            </div>
            <div>
              <label>Memory</label>
              <strong>{Math.round(sys?.mem_usage_percent || 0)}%</strong>
            </div>
            <div>
              <label>Disk I/O</label>
              <strong>{Math.round((sys?.disk_read_per_sec || 0) / 1024 / 1024)}/{Math.round((sys?.disk_write_per_sec || 0) / 1024 / 1024)} MB/s</strong>
            </div>
            <div>
              <label>Load (1/5/15)</label>
              <strong>{(sys?.load_1 || 0).toFixed(2)} / {(sys?.load_5 || 0).toFixed(2)} / {(sys?.load_15 || 0).toFixed(2)}</strong>
            </div>
          </div>
        </div>
      );
    }
    if (widgetTypeId === "traffic-overview") {
      const stats = statsSummary();
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Total Requests</label>
            <strong>{stats?.total_requests || 0}</strong>
          </div>
          <div>
            <label>Total Errors</label>
            <strong>{stats?.total_errors || 0}</strong>
          </div>
          <div>
            <label>Connections</label>
            <strong>{stats?.total_connections || 0}</strong>
          </div>
          <div>
            <label>Error Rate</label>
            <strong>
              {(stats?.total_requests || 0) > 0
                ? `${(((stats?.total_errors || 0) / (stats?.total_requests || 1)) * 100).toFixed(1)}%`
                : "0.0%"}
            </strong>
          </div>
          <Badge tone={(stats?.total_errors || 0) > 0 ? "warn" : "ok"}>
            {(stats?.endpoints?.length || 0) > 0 ? `${stats?.endpoints.length} endpoints observed` : "no endpoint traffic yet"}
          </Badge>
        </div>
      );
    }
    if (widgetTypeId === "endpoint-latency") {
      return (
        <div class="widget-body widget-list mono">
          <For each={endpointRows}>
            {(row) => (
              <div class="widget-list-row widget-log-row">
                <span>{row.method}</span>
                <Badge tone={row.error_rate > 10 ? "error" : row.error_rate > 2 ? "warn" : "ok"}>{row.error_rate.toFixed(1)}%</Badge>
                <span class="truncate-text">{row.path}</span>
                <span class="truncate-text">p99 {row.p99_ms.toFixed(1)}ms | req {row.request_count}</span>
              </div>
            )}
          </For>
          <Show when={endpointRows.length === 0}><p class="paragraph">No endpoint metrics yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "connection-hotspots") {
      return (
        <div class="widget-body widget-list mono">
          <For each={connectionRows}>
            {(row) => (
              <div class="widget-list-row widget-log-row">
                <span>{row.process}</span>
                <Badge tone={row.count > 100 ? "warn" : "neutral"}>{row.count}</Badge>
                <span class="truncate-text">{row.remote}:{row.port}</span>
                <span class="truncate-text">pid {row.pid}</span>
              </div>
            )}
          </For>
          <Show when={connectionRows.length === 0}><p class="paragraph">No connection flows yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "process-top") {
      return (
        <div class="widget-body widget-list">
          <For each={processRows}>
            {(row) => (
              <div class="widget-list-row">
                <strong>{row.name}</strong>
                <span>pid {row.pid}</span>
                <Badge tone={row.cpu_pct > 50 ? "error" : row.cpu_pct > 20 ? "warn" : "ok"}>
                  {row.cpu_pct.toFixed(1)}% cpu
                </Badge>
              </div>
            )}
          </For>
          <Show when={processRows.length === 0}><p class="paragraph">No process metrics yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "service-map-health") {
      const nodes = serviceMap()?.nodes?.length || 0;
      const links = serviceMap()?.links?.length || 0;
      const processNodes = (serviceMap()?.nodes || []).filter((n) => n.type === "process" || n.type === "service").length;
      const externalNodes = (serviceMap()?.nodes || []).filter((n) => n.type === "external").length;
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Nodes</label>
            <strong>{nodes}</strong>
          </div>
          <div>
            <label>Links</label>
            <strong>{links}</strong>
          </div>
          <div>
            <label>Internal</label>
            <strong>{processNodes}</strong>
          </div>
          <div>
            <label>External</label>
            <strong>{externalNodes}</strong>
          </div>
          <Badge tone={links > nodes * 2 ? "warn" : "ok"}>
            {nodes > 0 ? "topology observed" : "awaiting traffic"}
          </Badge>
        </div>
      );
    }
    if (widgetTypeId === "trace-throughput") {
      return (
        <div class="widget-body widget-list mono">
          <For each={traceRows}>
            {(row) => (
              <div class="widget-list-row widget-log-row">
                <span>{row.status}</span>
                <Badge tone={row.status === "ERROR" ? "error" : "ok"}>{row.duration_ms.toFixed(1)}ms</Badge>
                <span class="truncate-text">{row.service_name || "-"}</span>
                <span class="truncate-text">{row.name}</span>
              </div>
            )}
          </For>
          <Show when={traceRows.length === 0}><p class="paragraph">No recent traces.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "trace-services") {
      return (
        <div class="widget-body widget-list">
          <For each={traceServiceRows}>
            {(serviceName) => (
              <div class="widget-list-row">
                <strong>{serviceName}</strong>
                <span>trace-enabled</span>
                <Badge tone="ok">active</Badge>
              </div>
            )}
          </For>
          <Show when={traceServiceRows.length === 0}><p class="paragraph">No trace services yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "trace-dependencies") {
      return (
        <div class="widget-body widget-list mono">
          <For each={traceDependencyRows}>
            {(dep) => (
              <div class="widget-list-row widget-log-row">
                <span>{dep.parent}</span>
                <Badge tone={dep.call_count > 1000 ? "warn" : "neutral"}>{dep.call_count}</Badge>
                <span class="truncate-text">{"-> "}{dep.child}</span>
                <span class="truncate-text">calls</span>
              </div>
            )}
          </For>
          <Show when={traceDependencyRows.length === 0}><p class="paragraph">No service dependencies yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "kpi-reliability") {
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Open Incidents</label>
            <strong>{widgetReliability.openIncidents}</strong>
          </div>
          <div>
            <label>Critical Alerts</label>
            <strong>{widgetReliability.criticalAlerts}</strong>
          </div>
          <div>
            <label>Services Healthy</label>
            <strong>{widgetReliability.healthy}</strong>
          </div>
          <div>
            <label>Total Alerts</label>
            <strong>{widgetReliability.totalAlerts}</strong>
          </div>
          <Badge tone={healthTone(widgetReliability.healthy, widgetReliability.degraded, widgetReliability.unhealthy)}>
            health mix: {widgetReliability.healthy}/{widgetReliability.degraded}/{widgetReliability.unhealthy}
          </Badge>
          <Sparkline values={widgetPulse} width={200} height={36} color="var(--v2-error)" />
        </div>
      );
    }

    if (widgetTypeId === "alerts-feed") {
      return (
        <div class="widget-body widget-list">
          <For each={widgetAlerts.slice(0, listRows)}>
            {(alert) => (
              <div class="widget-list-row">
                <strong>{alert.name}</strong>
                <span>{alert.service}</span>
                <Badge tone={alert.severity === "critical" ? "error" : alert.severity === "high" ? "warn" : "neutral"}>
                  {alert.severity}
                </Badge>
              </div>
            )}
          </For>
          <Show when={widgetAlerts.length === 0}><p class="paragraph">No active alerts.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "incidents-live") {
      return (
        <div class="widget-body widget-list">
          <For each={widgetIncidents.slice(0, listRows)}>
            {(incident) => (
              <div class="widget-list-row">
                <strong>{incident.title}</strong>
                <span>{incident.service}</span>
                <Badge tone={incident.status === "triggered" ? "error" : "ok"}>{incident.status}</Badge>
              </div>
            )}
          </For>
          <Show when={widgetIncidents.length === 0}><p class="paragraph">No open incidents.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "alerts-severity-map") {
      return (
        <div class="widget-body widget-list">
          <For each={alertSeverityRows}>
            {(row) => (
              <div class="widget-meter-row">
                <span>{row.label}</span>
                <div class="widget-meter-track">
                  <div class="widget-meter-fill" style={{ width: `${Math.max(8, Math.round((row.count / alertSeverityMax) * 100))}%` }} />
                </div>
                <Badge tone={row.tone}>{row.count}</Badge>
              </div>
            )}
          </For>
          <Badge tone={pendingAlerts > 0 ? "warn" : "ok"}>pending alerts: {pendingAlerts}</Badge>
        </div>
      );
    }

    if (widgetTypeId === "logs-errors") {
      return (
        <div class="widget-body widget-list mono">
          <For each={widgetLogs.slice(0, logRows)}>
            {(entry) => (
              <div class="widget-list-row widget-log-row">
                <span>{new Date(entry.timestamp).toLocaleTimeString()}</span>
                <Badge tone={entry.level === "error" || entry.level === "fatal" ? "error" : entry.level === "warn" ? "warn" : "neutral"}>
                  {entry.level}
                </Badge>
                <span class="truncate-text">{entry.service || "-"}</span>
                <span class="truncate-text">{entry.message}</span>
              </div>
            )}
          </For>
          <Show when={widgetLogs.length === 0}><p class="paragraph">No log entries.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "deploy-correlation") {
      return (
        <div class="widget-body widget-list">
          <For each={widgetCorrelations.slice(0, listRows)}>
            {(corr) => (
              <div class="widget-list-row">
                <strong>{corr.deployment.service}</strong>
                <span>{corr.deployment.version}</span>
                <Badge tone={corr.confidence >= 0.75 ? "error" : corr.confidence >= 0.5 ? "warn" : "neutral"}>
                  conf {(corr.confidence * 100).toFixed(0)}%
                </Badge>
              </div>
            )}
          </For>
          <Show when={widgetCorrelations.length === 0}><p class="paragraph">No deploy correlations yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "service-health") {
      return (
        <div class="widget-body widget-list">
          <For each={widgetServices}>
            {(service) => (
              <div class="widget-list-row">
                <strong>{service.displayName}</strong>
                <span>{service.teamName}</span>
                <Badge tone={service.health === "unhealthy" ? "error" : service.health === "degraded" ? "warn" : "ok"}>
                  {service.health}
                </Badge>
              </div>
            )}
          </For>
          <Show when={widgetServices.length === 0}><p class="paragraph">No catalog services yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "service-latency-top") {
      return (
        <div class="widget-body widget-list">
          <For each={latencyHotspots}>
            {(service) => (
              <div class="widget-list-row">
                <strong>{service.displayName}</strong>
                <span>{service.teamName}</span>
                <Badge tone={service.avgResponseTimeMs > 1000 ? "error" : service.avgResponseTimeMs > 600 ? "warn" : "ok"}>
                  {service.avgResponseTimeMs}ms
                </Badge>
              </div>
            )}
          </For>
          <Show when={latencyHotspots.length === 0}><p class="paragraph">No service latency data yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "oncall-now") {
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Current On-call</label>
            <strong>{currentOncall()?.userName || "unassigned"}</strong>
          </div>
          <div>
            <label>Schedules</label>
            <strong>{(schedules() || []).length}</strong>
          </div>
          <div>
            <label>Escalation Policies</label>
            <strong>{(policies() || []).length}</strong>
          </div>
          <Badge tone={currentOncall()?.isOverride ? "warn" : "ok"}>
            {currentOncall()?.isOverride ? "override active" : "rotation active"}
          </Badge>
        </div>
      );
    }
    if (widgetTypeId === "incidents-by-commander") {
      return (
        <div class="widget-body widget-list">
          <For each={incidentsByCommander}>
            {(row) => (
              <div class="widget-list-row">
                <strong>{row[0] || "unassigned"}</strong>
                <span>open incidents</span>
                <Badge tone={row[1] > 2 ? "error" : row[1] > 1 ? "warn" : "ok"}>{row[1]}</Badge>
              </div>
            )}
          </For>
          <Show when={incidentsByCommander.length === 0}><p class="paragraph">No commander load right now.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "k8s-cluster") {
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Nodes Ready</label>
            <strong>{k8sSummary()?.nodesReady || 0}/{k8sSummary()?.nodes || 0}</strong>
          </div>
          <div>
            <label>Pods Running</label>
            <strong>{k8sSummary()?.podsRunning || 0}/{k8sSummary()?.pods || 0}</strong>
          </div>
          <div>
            <label>Deployments Healthy</label>
            <strong>{k8sSummary()?.deploymentsHealthy || 0}/{k8sSummary()?.deployments || 0}</strong>
          </div>
          <Badge tone={(k8sSummary()?.warningEvents || 0) > 0 ? "warn" : "ok"}>
            warnings {(k8sSummary()?.warningEvents || 0)}
          </Badge>
        </div>
      );
    }
    if (widgetTypeId === "k8s-capacity-risk") {
      return (
        <div class="widget-body widget-list">
          <div class="widget-meter-row">
            <span>Node readiness</span>
            <div class="widget-meter-track">
              <div class="widget-meter-fill" style={{ width: `${k8sNodeReadiness}%` }} />
            </div>
            <Badge tone={k8sNodeReadiness < 85 ? "error" : k8sNodeReadiness < 95 ? "warn" : "ok"}>{k8sNodeReadiness}%</Badge>
          </div>
          <div class="widget-meter-row">
            <span>Pod pressure</span>
            <div class="widget-meter-track">
              <div class="widget-meter-fill is-warn" style={{ width: `${Math.max(3, k8sPodPressure)}%` }} />
            </div>
            <Badge tone={k8sPodPressure > 25 ? "error" : k8sPodPressure > 10 ? "warn" : "ok"}>{k8sPodPressure}%</Badge>
          </div>
          <div class="widget-meter-row">
            <span>Deploy readiness</span>
            <div class="widget-meter-track">
              <div class="widget-meter-fill" style={{ width: `${k8sDeployReadiness}%` }} />
            </div>
            <Badge tone={k8sDeployReadiness < 85 ? "error" : k8sDeployReadiness < 95 ? "warn" : "ok"}>{k8sDeployReadiness}%</Badge>
          </div>
          <Badge tone={(k8sSummary()?.warningEvents || 0) > 0 ? "warn" : "ok"}>warning events: {k8sSummary()?.warningEvents || 0}</Badge>
        </div>
      );
    }

    if (widgetTypeId === "notify-delivery") {
      const failed = (history() || []).filter((item) => item.status.toLowerCase() !== "sent").length;
      const avgSuccess =
        (channels() || []).length > 0
          ? Math.round((channels() || []).reduce((sum, ch) => sum + ch.successRate, 0) / (channels() || []).length)
          : 0;

      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Channels</label>
            <strong>{(channels() || []).length}</strong>
          </div>
          <div>
            <label>Avg Success</label>
            <strong>{avgSuccess}%</strong>
          </div>
          <div>
            <label>Failed Sends</label>
            <strong>{failed}</strong>
          </div>
          <Badge tone={failed > 0 ? "warn" : "ok"}>{failed > 0 ? "degraded delivery" : "delivery healthy"}</Badge>
        </div>
      );
    }
    if (widgetTypeId === "notify-failure-log") {
      return (
        <div class="widget-body widget-list mono">
          <For each={failedDelivery}>
            {(entry) => (
              <div class="widget-list-row widget-log-row">
                <span>{entry.channelType}</span>
                <Badge tone="warn">{entry.status.toLowerCase()}</Badge>
                <span class="truncate-text">{entry.channelName}</span>
                <span class="truncate-text">{entry.title}</span>
              </div>
            )}
          </For>
          <Show when={failedDelivery.length === 0}><p class="paragraph">No failed deliveries.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "ops-action-queue") {
      return (
        <div class="widget-body widget-list">
          <For each={opsActions}>
            {(action) => (
              <div class="widget-list-row">
                <strong>{action.label}</strong>
                <span>{action.owner}</span>
                <Badge tone={action.tone}>now</Badge>
              </div>
            )}
          </For>
          <Show when={opsActions.length === 0}><p class="paragraph">No immediate actions. System stable.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "slo-burn-rate") {
      const sloRows = (slos() || [])
        .slice()
        .sort((a, b) => b.burnRate - a.burnRate)
        .slice(0, listRows);
      return (
        <div class="widget-body widget-list">
          <For each={sloRows}>
            {(slo) => (
              <div class="widget-list-row">
                <strong>{slo.name}</strong>
                <span>{slo.service || "-"}</span>
                <Badge tone={slo.burnRate > 2 ? "error" : slo.burnRate > 1 ? "warn" : "ok"}>
                  {slo.burnRate.toFixed(2)}x burn
                </Badge>
              </div>
            )}
          </For>
          <Show when={sloRows.length === 0}><p class="paragraph">No SLOs configured.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "slo-budget-remaining") {
      const sloRows = (slos() || [])
        .slice()
        .sort((a, b) => a.budgetRemaining - b.budgetRemaining)
        .slice(0, listRows);
      const breached = (slos() || []).filter((s) => s.status === "breached").length;
      const atRisk = (slos() || []).filter((s) => s.status === "at_risk").length;
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Total SLOs</label>
              <strong>{(slos() || []).length}</strong>
            </div>
            <div>
              <label>Breached</label>
              <strong>{breached}</strong>
            </div>
            <div>
              <label>At Risk</label>
              <strong>{atRisk}</strong>
            </div>
          </div>
          <For each={sloRows}>
            {(slo) => (
              <div class="widget-meter-row">
                <span>{slo.name}</span>
                <div class="widget-meter-track">
                  <div class="widget-meter-fill" style={{ width: `${Math.max(3, Math.min(100, slo.budgetRemaining))}%` }} />
                </div>
                <Badge tone={slo.status === "breached" ? "error" : slo.status === "at_risk" ? "warn" : "ok"}>
                  {slo.budgetRemaining.toFixed(1)}%
                </Badge>
              </div>
            )}
          </For>
          <Show when={sloRows.length === 0}><p class="paragraph">No SLOs configured.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "synthetic-uptime") {
      const checks = (syntheticChecks() || [])
        .slice()
        .sort((a, b) => a.uptimePercent - b.uptimePercent)
        .slice(0, listRows);
      const failing = (syntheticChecks() || []).filter((c) => c.status === "failing").length;
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Total Checks</label>
              <strong>{(syntheticChecks() || []).length}</strong>
            </div>
            <div>
              <label>Failing</label>
              <strong>{failing}</strong>
            </div>
          </div>
          <For each={checks}>
            {(check) => (
              <div class="widget-meter-row">
                <span>{check.name}</span>
                <div class="widget-meter-track">
                  <div class="widget-meter-fill" style={{ width: `${Math.max(3, check.uptimePercent)}%` }} />
                </div>
                <Badge tone={check.status === "failing" ? "error" : check.status === "degraded" ? "warn" : "ok"}>
                  {check.uptimePercent.toFixed(1)}%
                </Badge>
              </div>
            )}
          </For>
          <Show when={checks.length === 0}><p class="paragraph">No synthetic checks configured.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "synthetic-failures") {
      const failures = (syntheticFailures() || []).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={failures}>
            {(f) => (
              <div class="widget-list-row widget-log-row">
                <span>{f.timestamp ? new Date(f.timestamp).toLocaleTimeString() : "-"}</span>
                <Badge tone="error">{f.statusCode || "err"}</Badge>
                <span class="truncate-text">{f.checkName}</span>
                <span class="truncate-text">{f.error || `HTTP ${f.statusCode}`}</span>
              </div>
            )}
          </For>
          <Show when={failures.length === 0}><p class="paragraph">No synthetic failures.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "cost-estimate") {
      const cost = costEstimate();
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Monthly Cost</label>
            <strong>${cost?.totalMonthly?.toLocaleString() || "0"}</strong>
          </div>
          <div>
            <label>Datadog Equivalent</label>
            <strong>${cost?.datadogEquivalent?.toLocaleString() || "0"}</strong>
          </div>
          <div>
            <label>Savings</label>
            <strong>{cost?.savingsPercent || 0}%</strong>
          </div>
          <Badge tone={(cost?.savingsPercent || 0) > 50 ? "ok" : (cost?.savingsPercent || 0) > 20 ? "neutral" : "warn"}>
            {(cost?.savingsPercent || 0) > 0 ? `saving ${cost?.savingsPercent}% vs Datadog` : "calculating..."}
          </Badge>
        </div>
      );
    }

    if (widgetTypeId === "cardinality-hotspots") {
      const hotspots = (cardinalityHotspots() || [])
        .slice()
        .sort((a, b) => b.series - a.series)
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={hotspots}>
            {(hs) => (
              <div class="widget-list-row widget-log-row">
                <span class="truncate-text">{hs.metric}</span>
                <Badge tone={hs.series > 10000 ? "error" : hs.series > 1000 ? "warn" : "neutral"}>
                  {hs.series.toLocaleString()} series
                </Badge>
                <span>{hs.labels} labels</span>
                <span>{hs.growthRate > 0 ? `+${hs.growthRate.toFixed(1)}%` : `${hs.growthRate.toFixed(1)}%`}</span>
              </div>
            )}
          </For>
          <Show when={hotspots.length === 0}><p class="paragraph">No cardinality data yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "cost-recommendations") {
      const recs = (costRecommendations() || [])
        .slice()
        .sort((a, b) => b.savingsEstimate - a.savingsEstimate)
        .slice(0, listRows);
      return (
        <div class="widget-body widget-list">
          <For each={recs}>
            {(rec) => (
              <div class="widget-list-row">
                <strong>{rec.title}</strong>
                <span>${rec.savingsEstimate.toLocaleString()}/mo</span>
                <Badge tone={rec.impact === "high" ? "error" : rec.impact === "medium" ? "warn" : "neutral"}>
                  {rec.impact} impact
                </Badge>
              </div>
            )}
          </For>
          <Show when={recs.length === 0}><p class="paragraph">No cost recommendations yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "perf-anomalies") {
      const rows = (anomalies() || [])
        .slice()
        .sort((a, b) => b.score - a.score)
        .slice(0, listRows);
      return (
        <div class="widget-body widget-list">
          <For each={rows}>
            {(a) => (
              <div class="widget-list-row">
                <strong>{a.metric}</strong>
                <span>{a.service || "-"}</span>
                <Badge tone={a.severity === "critical" ? "error" : a.severity === "high" ? "warn" : "neutral"}>
                  {a.severity} ({a.score.toFixed(1)})
                </Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No anomalies detected.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "perf-db-queries") {
      const rows = (dbQueries() || [])
        .slice()
        .sort((a, b) => b.avgMs - a.avgMs)
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(q) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={q.avgMs > 500 ? "error" : q.avgMs > 100 ? "warn" : "ok"}>
                  {q.avgMs.toFixed(0)}ms
                </Badge>
                <span class="truncate-text">{q.database || "-"}</span>
                <span class="truncate-text">{q.query}</span>
                <span>{q.callCount} calls</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No DB query data yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "perf-flamegraph-top") {
      const rows = (flamegraphHotspots() || [])
        .slice()
        .sort((a, b) => b.selfPercent - a.selfPercent)
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(fn) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={fn.selfPercent > 10 ? "error" : fn.selfPercent > 5 ? "warn" : "neutral"}>
                  {fn.selfPercent.toFixed(1)}% self
                </Badge>
                <span class="truncate-text">{fn.function}</span>
                <span class="truncate-text">{fn.module}</span>
                <span>{fn.samples} samples</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No profiling data yet.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "alerts-watches") {
      const rows = (watches() || []).slice(0, listRows);
      const disabled = (watches() || []).filter((w) => !w.enabled).length;
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Total Rules</label>
              <strong>{(watches() || []).length}</strong>
            </div>
            <div>
              <label>Disabled</label>
              <strong>{disabled}</strong>
            </div>
          </div>
          <For each={rows}>
            {(w) => (
              <div class="widget-list-row">
                <strong>{w.name}</strong>
                <span>{w.service || "-"}</span>
                <Badge tone={w.enabled ? "ok" : "neutral"}>
                  {w.enabled ? "active" : "disabled"}
                </Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No watch rules configured.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "alerts-silences") {
      const rows = (silences() || []).slice(0, listRows);
      return (
        <div class="widget-body widget-list">
          <For each={rows}>
            {(s) => (
              <div class="widget-list-row">
                <strong>{s.matchers || "all"}</strong>
                <span>{s.createdBy || "-"}</span>
                <Badge tone="neutral">
                  until {s.endsAt ? new Date(s.endsAt).toLocaleString() : "forever"}
                </Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No active silences.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "logs-patterns") {
      const rows = (logPatterns() || [])
        .slice()
        .sort((a, b) => b.count - a.count)
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(p) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={p.level === "error" || p.level === "fatal" ? "error" : p.level === "warn" ? "warn" : "neutral"}>
                  {p.count}
                </Badge>
                <span class="truncate-text">{p.pattern}</span>
                <span class="truncate-text">{p.services.join(", ") || "-"}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No log patterns discovered.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "logs-trending") {
      const rows = (trendingPatterns() || [])
        .slice()
        .sort((a, b) => b.growthPercent - a.growthPercent)
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(p) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={p.growthPercent > 100 ? "error" : p.growthPercent > 30 ? "warn" : "neutral"}>
                  +{p.growthPercent.toFixed(0)}%
                </Badge>
                <span class="truncate-text">{p.pattern}</span>
                <span>{p.count} hits</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No trending patterns.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "deploy-feed") {
      const rows = (deploys() || []).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(d) => (
              <div class="widget-list-row widget-log-row">
                <span>{d.deployedAt ? new Date(d.deployedAt).toLocaleTimeString() : "-"}</span>
                <Badge tone={d.status === "failed" ? "error" : d.status === "rolling" ? "warn" : "ok"}>
                  {d.status}
                </Badge>
                <span class="truncate-text">{d.service}</span>
                <span class="truncate-text">{d.version}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No recent deploys.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "admin-audit-feed") {
      const rows = (auditLogs() || []).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(entry) => (
              <div class="widget-list-row widget-log-row">
                <span>{new Date(entry.timestamp).toLocaleTimeString()}</span>
                <Badge tone={entry.outcome === "failure" ? "error" : "neutral"}>
                  {entry.action}
                </Badge>
                <span class="truncate-text">{entry.userEmail || entry.userId}</span>
                <span class="truncate-text">{entry.resourceType}: {entry.resourceName}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No audit events.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "admin-api-keys") {
      const keys = (apiKeys() || []).slice(0, listRows);
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Active Keys</label>
              <strong>{(apiKeys() || []).length}</strong>
            </div>
          </div>
          <For each={keys}>
            {(k) => (
              <div class="widget-list-row">
                <strong>{k.name}</strong>
                <span>{k.prefix}</span>
                <Badge tone="neutral">{k.role}</Badge>
              </div>
            )}
          </For>
          <Show when={keys.length === 0}><p class="paragraph">No API keys configured.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "admin-backup-status") {
      const rows = (backups() || []).slice(0, listRows);
      const failed = (backups() || []).filter((b) => b.status === "failed").length;
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Total Backups</label>
              <strong>{(backups() || []).length}</strong>
            </div>
            <div>
              <label>Failed</label>
              <strong>{failed}</strong>
            </div>
          </div>
          <For each={rows}>
            {(b) => (
              <div class="widget-list-row">
                <strong>{b.filename || b.id}</strong>
                <span>{b.size > 0 ? `${(b.size / 1024 / 1024).toFixed(1)} MB` : "-"}</span>
                <Badge tone={b.status === "failed" ? "error" : b.status === "running" ? "warn" : "ok"}>
                  {b.status}
                </Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No backups found.</p></Show>
        </div>
      );
    }

    if (widgetTypeId === "k8s-containers") {
      const rows = (k8sContainers() || []).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(c) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={c.status === "running" ? "ok" : c.status === "waiting" ? "warn" : "error"}>{c.status}</Badge>
                <strong>{c.name}</strong>
                <span style={{ opacity: 0.6 }}>{c.image}</span>
                <Show when={c.restartCount > 0}>
                  <Badge tone="warn">{c.restartCount} restarts</Badge>
                </Show>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No containers found.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "k8s-pods") {
      const rows = (k8sPodList() || []).slice().sort((a, b) => b.restartCount - a.restartCount).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(pod) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={pod.status === "Running" ? "ok" : pod.status === "Pending" ? "warn" : "error"}>{pod.status}</Badge>
                <strong>{pod.name}</strong>
                <span style={{ opacity: 0.6 }}>{pod.nodeName || "-"}</span>
                <Show when={pod.restartCount > 0}>
                  <Badge tone="warn">{pod.restartCount} restarts</Badge>
                </Show>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No pods found.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "k8s-deployments") {
      const rows = (k8sDeployments() || []).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(dep) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={dep.readyReplicas >= dep.replicas ? "ok" : dep.readyReplicas > 0 ? "warn" : "error"}>
                  {dep.readyReplicas}/{dep.replicas}
                </Badge>
                <strong>{dep.name}</strong>
                <span style={{ opacity: 0.6 }}>{dep.namespace}</span>
                <span>{dep.status}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No deployments found.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "k8s-events") {
      const rows = (k8sEvents() || [])
        .slice()
        .sort((a, b) => (b.lastTimestamp || "").localeCompare(a.lastTimestamp || ""))
        .slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(evt) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={evt.type === "Warning" ? "warn" : "ok"}>{evt.type}</Badge>
                <strong>{evt.reason}</strong>
                <span style={{ opacity: 0.6 }}>{evt.objectKind}/{evt.objectName}</span>
                <span>{evt.message.slice(0, 80)}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No events found.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "endpoint-detail") {
      const rows = (statsSummary()?.endpoints || []).slice().sort((a, b) => b.p99_ms - a.p99_ms).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(ep) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone="neutral">{ep.method}</Badge>
                <strong>{ep.path}</strong>
                <span>avg {ep.avg_ms.toFixed(1)}ms</span>
                <span>p99 {ep.p99_ms.toFixed(1)}ms</span>
                <Badge tone={ep.error_rate > 5 ? "error" : ep.error_rate > 1 ? "warn" : "ok"}>
                  {ep.error_rate.toFixed(1)}% err
                </Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No endpoint data yet.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "connection-detail") {
      const rows = (statsSummary()?.connections || []).slice().sort((a, b) => b.count - a.count).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(conn) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone="neutral">{conn.count}</Badge>
                <strong>{conn.remote}:{conn.port}</strong>
                <span style={{ opacity: 0.6 }}>{conn.process}</span>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No connections found.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "trace-detail") {
      const spans = (traceSpans() || []).slice(0, logRows * 2);
      const firstTrace = (traceSummaries() || [])[0];
      return (
        <div class="widget-body widget-list">
          <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
            <div>
              <label>Trace ID</label>
              <strong style={{ "font-size": "0.75rem" }}>{firstTrace?.trace_id?.slice(0, 16) || "-"}…</strong>
            </div>
            <div>
              <label>Duration</label>
              <strong>{firstTrace?.duration_ms?.toFixed(1) || 0}ms</strong>
            </div>
            <div>
              <label>Spans</label>
              <strong>{spans.length}</strong>
            </div>
          </div>
          <div class="mono" style={{ overflow: "auto", flex: 1 }}>
            <For each={spans}>
              {(span) => (
                <div class="widget-list-row widget-log-row" style={{ "padding-left": `${span.depth * 16 + 4}px` }}>
                  <Badge tone={span.status === "ERROR" ? "error" : span.status === "OK" ? "ok" : "neutral"}>{span.duration_ms.toFixed(1)}ms</Badge>
                  <strong>{span.operation_name}</strong>
                  <span style={{ opacity: 0.6 }}>{span.service_name}</span>
                </div>
              )}
            </For>
            <Show when={spans.length === 0}><p class="paragraph">No spans loaded. Select a trace to view detail.</p></Show>
          </div>
        </div>
      );
    }
    if (widgetTypeId === "log-compare") {
      const cmp = logComparison();
      return (
        <div class="widget-body" style={{ display: "flex", "flex-direction": "column", gap: "var(--v2-space-2)" }}>
          <div style={{ display: "flex", gap: "var(--v2-space-3)", flex: 1, overflow: "hidden" }}>
            <div class="widget-list mono" style={{ flex: 1, overflow: "auto" }}>
              <label style={{ "font-weight": "bold", "padding-bottom": "var(--v2-space-1)" }}>Before</label>
              <For each={(cmp?.beforeEntries || []).slice(0, logRows)}>
                {(entry) => (
                  <div class="widget-list-row widget-log-row">
                    <Badge tone={entry.level === "error" ? "error" : entry.level === "warn" ? "warn" : "ok"}>{entry.level}</Badge>
                    <span>{entry.message.slice(0, 100)}</span>
                  </div>
                )}
              </For>
              <Show when={!cmp?.beforeEntries?.length}><p class="paragraph">No before entries.</p></Show>
            </div>
            <div class="widget-list mono" style={{ flex: 1, overflow: "auto" }}>
              <label style={{ "font-weight": "bold", "padding-bottom": "var(--v2-space-1)" }}>After</label>
              <For each={(cmp?.afterEntries || []).slice(0, logRows)}>
                {(entry) => (
                  <div class="widget-list-row widget-log-row">
                    <Badge tone={entry.level === "error" ? "error" : entry.level === "warn" ? "warn" : "ok"}>{entry.level}</Badge>
                    <span>{entry.message.slice(0, 100)}</span>
                  </div>
                )}
              </For>
              <Show when={!cmp?.afterEntries?.length}><p class="paragraph">No after entries.</p></Show>
            </div>
          </div>
          <div style={{ display: "flex", gap: "var(--v2-space-2)", "flex-wrap": "wrap" }}>
            <For each={cmp?.addedPatterns || []}>
              {(p) => <Badge tone="ok">+ {p}</Badge>}
            </For>
            <For each={cmp?.removedPatterns || []}>
              {(p) => <Badge tone="error">− {p}</Badge>}
            </For>
            <Show when={!cmp?.addedPatterns?.length && !cmp?.removedPatterns?.length}>
              <span class="paragraph">No pattern changes detected.</span>
            </Show>
          </div>
        </div>
      );
    }
    if (widgetTypeId === "deploy-stats") {
      const stats = deployStats();
      const successRate = stats && stats.totalDeploys > 0 ? ((stats.successCount / stats.totalDeploys) * 100).toFixed(1) : "0.0";
      const rollbackRate = stats && stats.totalDeploys > 0 ? ((stats.rollbackCount / stats.totalDeploys) * 100).toFixed(1) : "0.0";
      return (
        <div class="widget-body widget-kpi-grid">
          <div>
            <label>Total Deploys</label>
            <strong>{stats?.totalDeploys || 0}</strong>
          </div>
          <div>
            <label>Success Rate</label>
            <strong>{successRate}%</strong>
          </div>
          <div>
            <label>Rollback Rate</label>
            <strong>{rollbackRate}%</strong>
          </div>
          <div>
            <label>Avg/Day</label>
            <strong>{stats?.avgFrequencyPerDay?.toFixed(1) || "0.0"}</strong>
          </div>
        </div>
      );
    }
    if (widgetTypeId === "perf-slow-queries") {
      const rows = (slowQueries() || []).slice().sort((a, b) => b.maxMs - a.maxMs).slice(0, logRows);
      return (
        <div class="widget-body widget-list mono">
          <For each={rows}>
            {(q) => (
              <div class="widget-list-row widget-log-row">
                <Badge tone={q.maxMs > 1000 ? "error" : q.maxMs > 200 ? "warn" : "ok"}>{q.maxMs.toFixed(0)}ms</Badge>
                <span>avg {q.avgMs.toFixed(0)}ms</span>
                <span style={{ opacity: 0.6 }}>{q.database}</span>
                <strong style={{ "text-overflow": "ellipsis", overflow: "hidden", "white-space": "nowrap" }}>{q.query.slice(0, 80)}</strong>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No slow queries detected.</p></Show>
        </div>
      );
    }
    if (widgetTypeId === "cost-quick-wins") {
      const rows = (costQuickWins() || []).slice().sort((a, b) => b.monthlySavings - a.monthlySavings).slice(0, logRows);
      return (
        <div class="widget-body widget-list">
          <For each={rows}>
            {(win) => (
              <div class="widget-list-row widget-log-row">
                <strong>{win.title}</strong>
                <Badge tone="ok">${win.monthlySavings.toFixed(0)}/mo</Badge>
                <Badge tone={win.impact === "high" ? "error" : win.impact === "medium" ? "warn" : "neutral"}>{win.impact}</Badge>
                <Badge tone="neutral">{win.effort}</Badge>
              </div>
            )}
          </For>
          <Show when={rows.length === 0}><p class="paragraph">No quick wins found.</p></Show>
        </div>
      );
    }

    return (
      <div class="widget-body widget-links">
        <A href="/app/detect/alerts" class="catalog-link">Detect</A>
        <A href="/app/investigate/logs" class="catalog-link">Investigate</A>
        <A href="/app/correlate/timeline" class="catalog-link">Correlate</A>
        <A href="/app/respond/incidents" class="catalog-link">Respond</A>
        <A href="/app/improve/oncall" class="catalog-link">On-call</A>
        <A href="/app/improve/kubernetes" class="catalog-link">Kubernetes</A>
        <A href="/app/configure/catalog" class="catalog-link">Catalog</A>
        <A href="/app/configure/notifications" class="catalog-link">Notify</A>
      </div>
    );
  }

  return (
    <>
      <section class="dashboard-toolbar panel">
        <div class="panel-body dashboard-toolbar-body">
          <div class="dashboard-toolbar-left">
            <label>Dashboard</label>
            <select
              class="input dashboard-select"
              value={selectedDashboardId()}
              onChange={(e) => onSelectDashboard(e.currentTarget.value)}
            >
              <option value="">unsaved canvas</option>
              <For each={dashboards() || []}>
                {(dash) => <option value={dash.id}>{dash.name}{dash.isDefault ? " (default)" : ""}</option>}
              </For>
            </select>
            <Input
              class="dashboard-name-input"
              value={canvasName()}
              onInput={(e) => setCanvasName(e.currentTarget.value)}
              placeholder="Dashboard name"
            />
          </div>
          <div class="dashboard-toolbar-right">
            <Button variant={editMode() ? "primary" : "default"} onClick={() => setEditMode((v) => !v)}>
              {editMode() ? "Done Editing" : "Customize Layout"}
            </Button>
            <Show when={editMode()}>
              <Button onClick={lockAllWidgets}>Lock All</Button>
              <Button onClick={unlockAllWidgets}>Unlock All</Button>
              <Button variant={showOnlyUnlocked() ? "primary" : "default"} onClick={() => setShowOnlyUnlocked((v) => !v)}>
                {showOnlyUnlocked() ? "Show All" : "Show Unlocked"}
              </Button>
            </Show>
            <Button onClick={undoHistory} disabled={!canUndo()}>Undo</Button>
            <Button onClick={redoHistory} disabled={!canRedo()}>Redo</Button>
            <Button onClick={onRefreshAll}>Refresh</Button>
            <Button onClick={exportDashboardJson}>Export JSON</Button>
            <Button onClick={openImportDialog}>Import JSON</Button>
            <Button onClick={() => setShowPicker(true)}>Add Widget</Button>
            <Button variant="primary" onClick={onSaveCurrent} disabled={!selectedDashboardId() || !dirty()}>
              Save
            </Button>
            <Button onClick={onMakeDefault}>Set Default</Button>
            <Button variant="danger" onClick={onDeleteCurrent}>Delete</Button>
          </div>
        </div>
        <div class="dashboard-toolbar-subrow">
          <select class="input dashboard-filter-select" value={timeRange()} onChange={(e) => setTimeRange(e.currentTarget.value)}>
            <option value="15m">last 15m</option>
            <option value="1h">last 1h</option>
            <option value="6h">last 6h</option>
            <option value="24h">last 24h</option>
          </select>
          <select class="input dashboard-filter-select" value={serviceFilter()} onChange={(e) => setServiceFilter(e.currentTarget.value)}>
            <option value="">all services</option>
            <For each={serviceOptions()}>
              {(svc) => <option value={svc}>{svc}</option>}
            </For>
          </select>
          <select class="input dashboard-filter-select" value={severityFilter()} onChange={(e) => setSeverityFilter(e.currentTarget.value)}>
            <option value="">all severities</option>
            <option value="critical">critical</option>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
          </select>
          <Button onClick={addServicePack}>Add Service Pack</Button>
          <Input
            value={newDashboardName()}
            onInput={(e) => setNewDashboardName(e.currentTarget.value)}
            placeholder="New dashboard name"
          />
          <select
            class="input dashboard-template-select"
            value={selectedTemplateId()}
            onChange={(e) => setSelectedTemplateId(e.currentTarget.value)}
          >
            <For each={DASHBOARD_TEMPLATES}>
              {(tpl) => <option value={tpl.id}>{tpl.name}</option>}
            </For>
          </select>
          <Button onClick={applyTemplate}>Apply Template</Button>
          <Button onClick={onSaveAsNew}>Save As New</Button>
          <Badge tone="neutral">{activeLayout().length} widgets</Badge>
          <Show when={severityFilter()}>
            <Badge tone="warn">severity: {severityFilter()}</Badge>
          </Show>
          <Show when={showOnlyUnlocked()}>
            <Badge tone="warn">{visibleLayout().length} visible</Badge>
          </Show>
          <Badge tone="ok">{selectedDashboardName()}</Badge>
          <Badge tone={dirty() ? "warn" : "ok"}>{dirty() ? "unsaved changes" : "saved"}</Badge>
          <Badge tone="neutral">shortcuts: {shortcutHint()}</Badge>
          <Show when={notice()}>
            <span class="inline-notice">{notice()}</span>
          </Show>
        </div>
      </section>

      <section
        ref={canvasRef}
        class={`dashboard-canvas${draggingWidgetId() ? " is-drag-active" : ""}`}
        onDragOver={(e) => {
          e.preventDefault();
          const point = canvasPointToGrid(e.clientX, e.clientY, true);
          if (point) setCanvasDropPoint(point);
        }}
        onDrop={(e) => {
          e.preventDefault();
          const point = canvasPointToGrid(e.clientX, e.clientY, true);
          if (point) moveDraggedWidgetTo(point);
          onWidgetDragEnd();
        }}
      >
        <Show when={draggingWidgetId() && dropPoint()}>
          {(() => {
            const dragged = layout().find((item) => item.id === draggingWidgetId());
            const w = dragged ? dragged.w : 2;
            const h = dragged ? dragged.h : 1;
            return (
              <div
                class="canvas-drop-hint"
                style={{
                  "grid-column": `${dropPoint()!.x + 1} / span ${w}`,
                  "grid-row": `${dropPoint()!.y + 1} / span ${h}`
                }}
              />
            );
          })()}
        </Show>
        <For each={visibleLayout()}>
          {(item) => (
            <article
              class={`widget-card${draggingWidgetId() === item.id ? " is-dragging" : ""}${dragOverWidgetId() === item.id ? " is-drop-target" : ""}${focusedWidgetId() === item.id ? " is-focused" : ""}${isWidgetLocked(item.id) ? " is-locked" : ""}`}
              draggable={editMode() && resizingWidgetId() !== item.id && !isWidgetLocked(item.id)}
              tabindex={editMode() ? 0 : -1}
              onFocus={() => setFocusedWidgetId(item.id)}
              onClick={() => setFocusedWidgetId(item.id)}
              onDragStart={(e) => onWidgetDragStart(item.id, e as DragEvent)}
              onDragEnd={onWidgetDragEnd}
              onDragOver={(e) => {
                e.preventDefault();
                if (draggingWidgetId() && draggingWidgetId() !== item.id) setDragOverWidgetId(item.id);
              }}
              onDrop={(e) => {
                // Let the drop bubble to the canvas so the widget lands
                // where the hint shows, not at this widget's old position
                e.preventDefault();
              }}
              style={{
                "grid-column": `${clampSpan(item.x, 0, 11) + 1} / span ${clampSpan(item.w, 2, 12)}`,
                "grid-row": `${clampSpan(item.y, 0, 60) + 1} / span ${clampSpan(item.h, 2, GRID_MAX_H)}`
              }}
            >
              <header class="widget-head">
                <h3>
                  <Show when={isWidgetLocked(item.id)}>
                    <span class="widget-lock-icon" aria-hidden="true">🔒</span>
                  </Show>
                  {widgetTitle(item.id)}
                </h3>
                <div class="panel-actions widget-controls">
                  <Show when={editMode()}>
                    <Badge tone={isWidgetLocked(item.id) ? "warn" : "neutral"}>
                      {isWidgetLocked(item.id) ? "locked" : "drag to move"}
                    </Badge>
                  </Show>
                  <button
                    class="widget-trash-btn"
                    title="Remove widget"
                    aria-label={`Remove ${widgetTitle(item.id)}`}
                    disabled={isWidgetLocked(item.id)}
                    onClick={() => removeWidget(item.id)}
                  >
                    <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M2 4h12M5.33 4V2.67a1.33 1.33 0 011.34-1.34h2.66a1.33 1.33 0 011.34 1.34V4M6.67 7.33v4M9.33 7.33v4M12.67 4v9.33a1.33 1.33 0 01-1.34 1.34H4.67a1.33 1.33 0 01-1.34-1.34V4" />
                    </svg>
                  </button>
                </div>
              </header>
              <Show when={editMode() && !isWidgetLocked(item.id)}>
                <div
                  class="widget-resize-handle widget-resize-e"
                  title="Resize width"
                  onMouseDown={(e) => startResize(item.id, "e", e)}
                />
                <div
                  class="widget-resize-handle widget-resize-s"
                  title="Resize height"
                  onMouseDown={(e) => startResize(item.id, "s", e)}
                />
                <div
                  class="widget-resize-handle widget-resize-se"
                  title="Resize width and height"
                  onMouseDown={(e) => startResize(item.id, "se", e)}
                />
              </Show>
              <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
                <Show when={!isWidgetLoading(baseWidgetId(item.id))} fallback={<WidgetLoading />}>
                  {renderWidget(item)}
                </Show>
              </ErrorBoundary>
            </article>
          )}
        </For>
      </section>

      <Show when={editMode() && focusedWidget()}>
        <section class="widget-inspector panel">
          <header class="panel-head">
            <h2>Widget Inspector</h2>
            <div class="panel-actions">
              <Badge tone="ok">{widgetTitle(focusedWidget()!.id)}</Badge>
            </div>
          </header>
          <div class="panel-body widget-inspector-body">
            <div class="widget-inspector-grid">
              <label>Time Scope</label>
              <select
                class="input widget-scope-select"
                value={widgetConfig()[focusedWidget()!.id]?.since || ""}
                onChange={(e) => setWidgetSince(focusedWidget()!.id, e.currentTarget.value)}
              >
                <option value="">default time ({timeRange()})</option>
                <option value="15m">last 15m</option>
                <option value="1h">last 1h</option>
                <option value="6h">last 6h</option>
                <option value="24h">last 24h</option>
              </select>
              <label>Service Scope</label>
              <select
                class="input widget-scope-select"
                value={widgetConfig()[focusedWidget()!.id]?.service || ""}
                onChange={(e) => setWidgetService(focusedWidget()!.id, e.currentTarget.value)}
              >
                <option value="">default service ({serviceFilter() || "all"})</option>
                <For each={serviceOptions()}>
                  {(svc) => <option value={svc}>{svc}</option>}
                </For>
              </select>
              <label>Severity Scope</label>
              <select
                class="input widget-scope-select"
                value={widgetConfig()[focusedWidget()!.id]?.severity || ""}
                onChange={(e) => setWidgetSeverity(focusedWidget()!.id, e.currentTarget.value)}
              >
                <option value="">default severity ({severityFilter() || "all"})</option>
                <option value="critical">critical</option>
                <option value="high">high</option>
                <option value="medium">medium</option>
                <option value="low">low</option>
              </select>
            </div>
            <div class="row">
              <Badge tone="neutral">x:{focusedWidget()!.x} y:{focusedWidget()!.y} w:{focusedWidget()!.w} h:{focusedWidget()!.h}</Badge>
              <Button
                variant={isWidgetLocked(focusedWidget()!.id) ? "primary" : "default"}
                onClick={() => setWidgetLocked(focusedWidget()!.id, !isWidgetLocked(focusedWidget()!.id))}
              >
                {isWidgetLocked(focusedWidget()!.id) ? "Unlock" : "Lock"}
              </Button>
              <Button onClick={() => copyWidget(focusedWidget()!.id)}>Copy</Button>
              <Button onClick={() => duplicateWidget(focusedWidget()!.id)}>Duplicate</Button>
              <Button onClick={pasteWidget} disabled={!copiedWidget()}>Paste</Button>
              <Button onClick={() => clearWidgetScope(focusedWidget()!.id)} disabled={isWidgetLocked(focusedWidget()!.id)}>
                Clear Scope
              </Button>
              <Button variant="danger" onClick={() => removeWidget(focusedWidget()!.id)} disabled={isWidgetLocked(focusedWidget()!.id)}>
                Remove Widget
              </Button>
            </div>
          </div>
        </section>
      </Show>

      <Show when={showPicker()}>
        <div class="modal-overlay" onClick={() => setShowPicker(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Add Widget</h3>
            <Input
              value={pickerQuery()}
              onInput={(e) => setPickerQuery(e.currentTarget.value)}
              placeholder="Search widgets"
              class="widget-picker-search"
            />
            <div class="widget-picker-grid">
              <For each={availableToAdd()}>
                {(widget) => (
                  <button class="widget-picker-card" onClick={() => addWidget(widget.id)}>
                    <strong>{widget.title}</strong>
                    <p>{widget.description} Add another instance as needed.</p>
                    <span class="mono">{widget.defaultW}x{widget.defaultH}</span>
                  </button>
                )}
              </For>
            </div>
            <Show when={availableToAdd().length === 0}>
              <p class="paragraph">No widgets match that search.</p>
            </Show>
            <div class="row">
              <Button onClick={() => setShowPicker(false)}>Close</Button>
            </div>
          </div>
        </div>
      </Show>
      <input
        ref={importInputRef}
        type="file"
        accept="application/json,.json"
        class="dashboard-import-input"
        onChange={(e) => {
          const file = e.currentTarget.files?.[0];
          if (file) void onImportDashboardFile(file);
        }}
      />
    </>
  );
}
