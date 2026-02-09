import { A } from "@solidjs/router";
import { For, Show, createEffect, createMemo, createResource, createSignal, onCleanup, onMount } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
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
import { loadK8sSummary } from "../domains/kubernetes/service";
import { loadNotifyChannels, loadNotifyHistory } from "../domains/notify/service";

type WidgetDef = {
  id: string;
  title: string;
  description: string;
  defaultW: number;
  defaultH: number;
};

const WIDGETS: WidgetDef[] = [
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
  { id: "command-links", title: "Ops Shortcuts", description: "Jump to triage and response surfaces", defaultW: 8, defaultH: 1 }
];

const DEFAULT_DASHBOARD_NAME = "Operations Command";
const EDIT_MODE_PREF_KEY = "dogwatch-v2-dashboard-edit-mode";
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
  { id: "command-links", x: 4, y: 6, w: 8, h: 1 }
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
      { id: "command-links", x: 0, y: 4, w: 12, h: 1 }
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
      { id: "logs-errors", x: 0, y: 4, w: 8, h: 2 },
      { id: "notify-delivery", x: 8, y: 4, w: 4, h: 2 }
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
      { id: "alerts-severity-map", x: 0, y: 4, w: 4, h: 2 },
      { id: "ops-action-queue", x: 4, y: 4, w: 4, h: 2 },
      { id: "command-links", x: 8, y: 4, w: 4, h: 1 }
    ]
  }
];

function clampSpan(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, Number(value) || min));
}

const GRID_COLUMNS = 12;
const GRID_MAX_Y = 60;
const GRID_ROW_HEIGHT_PX = 92;

function overlaps(a: DashboardWidgetPosition, b: DashboardWidgetPosition): boolean {
  const noXOverlap = a.x + a.w <= b.x || b.x + b.w <= a.x;
  const noYOverlap = a.y + a.h <= b.y || b.y + b.h <= a.y;
  return !(noXOverlap || noYOverlap);
}

function packLayout(items: DashboardWidgetPosition[], prioritizeId?: string): DashboardWidgetPosition[] {
  const normalized = items.map((item) => {
    const w = clampSpan(item.w, 2, GRID_COLUMNS);
    return {
      ...item,
      w,
      h: clampSpan(item.h, 1, 4),
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
    const next = { ...item };
    while (placed.some((other) => overlaps(next, other))) {
      next.y += 1;
      if (next.y > GRID_MAX_Y) break;
    }
    placed.push(next);
  }

  return placed;
}

function normalizeLayout(items: DashboardWidgetPosition[]): DashboardWidgetPosition[] {
  return packLayout(items);
}

function normalizeWidgetConfig(config: Record<string, DashboardWidgetConfig>): Record<string, DashboardWidgetConfig> {
  const normalized: Record<string, DashboardWidgetConfig> = {};
  for (const widgetId of Object.keys(config).sort()) {
    const entry = config[widgetId];
    const service = entry?.service?.trim() || "";
    const since = entry?.since?.trim() || "";
    const locked = Boolean(entry?.locked);
    if (!service && !since && !locked) continue;
    normalized[widgetId] = {};
    if (service) normalized[widgetId].service = service;
    if (since) normalized[widgetId].since = since;
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
  const [layout, setLayout] = createSignal<DashboardWidgetPosition[]>(defaultLayout);
  const [showPicker, setShowPicker] = createSignal(false);
  const [pickerQuery, setPickerQuery] = createSignal("");
  const [newDashboardName, setNewDashboardName] = createSignal(DEFAULT_DASHBOARD_NAME);
  const [timeRange, setTimeRange] = createSignal("1h");
  const [serviceFilter, setServiceFilter] = createSignal("");
  const [draggingWidgetId, setDraggingWidgetId] = createSignal("");
  const [dragOverWidgetId, setDragOverWidgetId] = createSignal("");
  const [canvasDropPoint, setCanvasDropPoint] = createSignal<{ x: number; y: number } | null>(null);
  const [editMode, setEditMode] = createSignal(false);
  const [widgetConfig, setWidgetConfig] = createSignal<Record<string, DashboardWidgetConfig>>({});
  const [focusedWidgetId, setFocusedWidgetId] = createSignal("");
  const [showOnlyUnlocked, setShowOnlyUnlocked] = createSignal(false);
  const [selectedTemplateId, setSelectedTemplateId] = createSignal(DASHBOARD_TEMPLATES[0]?.id || "");
  const [historyPast, setHistoryPast] = createSignal<EditorSnapshot[]>([]);
  const [historyFuture, setHistoryFuture] = createSignal<EditorSnapshot[]>([]);
  const [copiedWidget, setCopiedWidget] = createSignal<CopiedWidget | null>(null);
  const [resizingWidgetId, setResizingWidgetId] = createSignal("");

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

  onMount(() => {
    if (typeof window === "undefined") return;
    const raw = window.localStorage.getItem(EDIT_MODE_PREF_KEY);
    if (raw === "1") setEditMode(true);

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
    setCanvasName(dashboard.name);
    setLayout(dashboard.layout.length ? dashboard.layout : defaultLayout);
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
  const visibleLayout = createMemo(() =>
    showOnlyUnlocked() ? activeLayout().filter((item) => !isWidgetLocked(item.id)) : activeLayout()
  );
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

  function isWidgetLocked(widgetId: string): boolean {
    return Boolean(widgetConfig()[widgetId]?.locked);
  }

  function isWithinSince(rawTs: string | undefined, since: string): boolean {
    if (!rawTs) return true;
    const ts = new Date(rawTs).getTime();
    if (!Number.isFinite(ts)) return true;
    return Date.now() - ts <= sinceToMs(since);
  }

  function filterAlertsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    return (alerts() || []).filter((a) => (!svc || a.service === svc) && isWithinSince(a.startedAtRaw, since));
  }

  function filterIncidentsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    return (incidents() || []).filter((i) => (!svc || i.service === svc) && isWithinSince(i.startedAtRaw, since));
  }

  function filterLogsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const horizon = sinceToMs(resolveWidgetSince(widgetId));
    const now = Date.now();
    return (logs() || []).filter((row) => {
      if (svc && row.service !== svc) return false;
      const ts = new Date(row.timestamp).getTime();
      if (!Number.isFinite(ts)) return false;
      return now - ts <= horizon;
    });
  }

  function filterCorrelationsForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    const since = resolveWidgetSince(widgetId);
    return (correlations() || []).filter((corr) => {
      if (svc && corr.deployment.service !== svc) return false;
      return isWithinSince(corr.deployment.timestamp, since);
    });
  }

  function filterServicesForWidget(widgetId: string) {
    const svc = resolveWidgetService(widgetId);
    return (catalogServices() || []).filter((row) => !svc || row.name === svc || row.displayName === svc);
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
    setLayout((curr) => curr.filter((item) => item.id !== widgetId));
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

  function applyWidgetUpdateRaw(widgetId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    setLayout((curr) =>
      curr.map((item) => {
        if (item.id !== widgetId) return item;
        const next = fn(item);
        const w = clampSpan(next.w, 2, GRID_COLUMNS);
        return {
          ...next,
          w,
          h: clampSpan(next.h, 1, 4),
          x: clampSpan(next.x, 0, Math.max(0, GRID_COLUMNS - w)),
          y: clampSpan(next.y, 0, GRID_MAX_Y)
        };
      })
    );
  }

  function nudgeWidget(widgetId: string, dx: number, dy: number) {
    updateWidget(widgetId, (item) => ({
      ...item,
      x: clampSpan(item.x + dx, 0, Math.max(0, 12 - item.w)),
      y: clampSpan(item.y + dy, 0, 30)
    }));
  }

  function resizeWidget(widgetId: string, dw: number, dh: number) {
    updateWidget(widgetId, (item) => {
      const nextW = clampSpan(item.w + dw, 2, 12);
      const nextH = clampSpan(item.h + dh, 1, 4);
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
      const dxCols = Math.round((e.clientX - startX) / colWidth);
      const dyRows = Math.round((e.clientY - startY) / GRID_ROW_HEIGHT_PX);
      const apply = e.altKey ? applyWidgetUpdateRaw : applyWidgetUpdate;
      apply(widgetId, (item) => {
        const nextW = axis === "e" || axis === "se" ? clampSpan(startW + dxCols, 2, GRID_COLUMNS) : startW;
        const nextH = axis === "s" || axis === "se" ? clampSpan(startH + dyRows, 1, 4) : startH;
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
      return packLayout(
        curr.map((item) => {
          if (item.id === aId) return { ...item, x: b.x, y: b.y };
          if (item.id === bId) return { ...item, x: a.x, y: a.y };
          return item;
        }),
        aId
      );
    });
    setNotice(`Moved ${widgetTitle(aId)}.`);
  }

  function onWidgetDragStart(widgetId: string) {
    if (isWidgetLocked(widgetId)) {
      setNotice("Widget is locked. Unlock in inspector to move.");
      return;
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

  function canvasPointToGrid(clientX: number, clientY: number): { x: number; y: number } | null {
    if (!canvasRef) return null;
    const rect = canvasRef.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const columnWidth = rect.width / GRID_COLUMNS;
    const x = clampSpan(Math.floor((clientX - rect.left) / columnWidth), 0, GRID_COLUMNS - 1);
    const y = clampSpan(Math.floor((clientY - rect.top) / GRID_ROW_HEIGHT_PX), 0, GRID_MAX_Y);
    return { x, y };
  }

  function moveDraggedWidgetTo(point: { x: number; y: number }, bypassPack = false) {
    const draggedId = draggingWidgetId();
    if (!draggedId) return;
    if (isWidgetLocked(draggedId)) return;
    pushHistory();
    if (bypassPack) {
      applyWidgetUpdateRaw(draggedId, (item) => ({
        ...item,
        x: clampSpan(point.x, 0, Math.max(0, GRID_COLUMNS - item.w)),
        y: clampSpan(point.y, 0, GRID_MAX_Y)
      }));
      return;
    }
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
      refetchHistory()
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
          h: clampSpan(item.h, 1, 4)
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
      .slice(0, 3);
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
      .slice(0, 5);
    const latencyHotspots = widgetServices
      .slice()
      .sort((a, b) => b.avgResponseTimeMs - a.avgResponseTimeMs)
      .slice(0, 5);
    const failedDelivery = (history() || [])
      .filter((entry) => entry.status.toLowerCase() !== "sent")
      .slice(0, 5);
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
        .slice(0, 3)
        .map((incident) => ({
          id: `incident-${incident.id}`,
          label: `Acknowledge incident: ${incident.title}`,
          owner: incident.commander || "unassigned",
          tone: "error" as const
        })),
      ...widgetAlerts
        .filter((alert) => alert.severity === "critical")
        .slice(0, 3)
        .map((alert) => ({
          id: `alert-${alert.id}`,
          label: `Investigate alert: ${alert.name}`,
          owner: alert.service,
          tone: "warn" as const
        }))
    ].slice(0, 6);
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
          <div class="widget-sparkline" aria-label="Error pulse trend">
            <For each={widgetPulse}>
              {(value) => <span style={{ height: `${value}%` }} />}
            </For>
          </div>
        </div>
      );
    }

    if (widgetTypeId === "alerts-feed") {
      return (
        <div class="widget-body widget-list">
          <For each={widgetAlerts.slice(0, 4)}>
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
          <For each={widgetIncidents.slice(0, 4)}>
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
          <For each={widgetLogs.slice(0, 6)}>
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
          <For each={widgetCorrelations.slice(0, 4)}>
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
          const point = canvasPointToGrid(e.clientX, e.clientY);
          if (point) setCanvasDropPoint(point);
        }}
        onDrop={(e) => {
          e.preventDefault();
          const point = canvasPointToGrid(e.clientX, e.clientY);
          if (point) moveDraggedWidgetTo(point, e.altKey);
          onWidgetDragEnd();
        }}
      >
        <Show when={draggingWidgetId() && dropPoint()}>
          <div
            class="canvas-drop-hint"
            style={{
              "grid-column": `${dropPoint()!.x + 1} / span 1`,
              "grid-row": `${dropPoint()!.y + 1} / span 1`
            }}
          />
        </Show>
        <For each={visibleLayout()}>
          {(item) => (
            <article
              class={`widget-card${draggingWidgetId() === item.id ? " is-dragging" : ""}${dragOverWidgetId() === item.id ? " is-drop-target" : ""}${focusedWidgetId() === item.id ? " is-focused" : ""}${isWidgetLocked(item.id) ? " is-locked" : ""}`}
              draggable={editMode() && resizingWidgetId() !== item.id && !isWidgetLocked(item.id)}
              tabindex={editMode() ? 0 : -1}
              onFocus={() => setFocusedWidgetId(item.id)}
              onClick={() => setFocusedWidgetId(item.id)}
              onDragStart={() => onWidgetDragStart(item.id)}
              onDragEnd={onWidgetDragEnd}
              onDragOver={(e) => {
                e.preventDefault();
                if (draggingWidgetId() && draggingWidgetId() !== item.id) setDragOverWidgetId(item.id);
              }}
              onDrop={(e) => {
                e.preventDefault();
                e.stopPropagation();
                onWidgetDrop(item.id);
              }}
              style={{
                "grid-column": `${clampSpan(item.x, 0, 11) + 1} / span ${clampSpan(item.w, 2, 12)}`,
                "grid-row": `${clampSpan(item.y, 0, 60) + 1} / span ${clampSpan(item.h, 1, 4)}`
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
                  <Button onClick={() => removeWidget(item.id)} disabled={isWidgetLocked(item.id)}>Remove</Button>
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
              {renderWidget(item)}
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
