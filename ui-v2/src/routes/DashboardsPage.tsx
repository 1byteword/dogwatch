import { A } from "@solidjs/router";
import { For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
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
  { id: "incidents-live", title: "Live Incidents", description: "Open incident queue and ownership", defaultW: 4, defaultH: 2 },
  { id: "logs-errors", title: "Error Log Stream", description: "Most recent error and warn events", defaultW: 6, defaultH: 2 },
  { id: "deploy-correlation", title: "Deploy Correlation", description: "Deploy confidence with incident timing", defaultW: 6, defaultH: 2 },
  { id: "service-health", title: "Service Health", description: "Catalog health and highest-risk services", defaultW: 4, defaultH: 2 },
  { id: "oncall-now", title: "On-call Command", description: "Current rotation and policy readiness", defaultW: 4, defaultH: 2 },
  { id: "k8s-cluster", title: "Cluster Health", description: "Kubernetes readiness and warning load", defaultW: 4, defaultH: 2 },
  { id: "notify-delivery", title: "Notification Delivery", description: "Channel reliability and failed sends", defaultW: 4, defaultH: 2 },
  { id: "command-links", title: "Ops Shortcuts", description: "Jump to triage and response surfaces", defaultW: 8, defaultH: 1 }
];

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

export function DashboardsPage() {
  let canvasRef: HTMLDivElement | undefined;
  const [notice, setNotice] = createSignal("");
  const [selectedDashboardId, setSelectedDashboardId] = createSignal("");
  const [canvasName, setCanvasName] = createSignal("Operations Command");
  const [layout, setLayout] = createSignal<DashboardWidgetPosition[]>(defaultLayout);
  const [showPicker, setShowPicker] = createSignal(false);
  const [newDashboardName, setNewDashboardName] = createSignal("Operations Command");
  const [timeRange, setTimeRange] = createSignal("1h");
  const [serviceFilter, setServiceFilter] = createSignal("");
  const [draggingWidgetId, setDraggingWidgetId] = createSignal("");
  const [dragOverWidgetId, setDragOverWidgetId] = createSignal("");
  const [canvasDropPoint, setCanvasDropPoint] = createSignal<{ x: number; y: number } | null>(null);
  const [editMode, setEditMode] = createSignal(false);
  const [widgetConfig, setWidgetConfig] = createSignal<Record<string, DashboardWidgetConfig>>({});
  const [settingsWidgetId, setSettingsWidgetId] = createSignal("");

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
    setSettingsWidgetId("");
  });

  const knownWidgetIds = createMemo(() => new Set(WIDGETS.map((w) => w.id)));

  const activeLayout = createMemo(() =>
    layout()
      .filter((item) => knownWidgetIds().has(item.id))
      .slice()
      .sort((a, b) => (a.y - b.y) || (a.x - b.x))
  );
  const dropPoint = createMemo(() => canvasDropPoint());

  const selectedDashboardName = createMemo(() => {
    const id = selectedDashboardId();
    const found = (dashboards() || []).find((item) => item.id === id);
    return found?.name || "Unsaved dashboard";
  });

  const availableToAdd = createMemo(() => {
    const existing = new Set(layout().map((item) => item.id));
    return WIDGETS.filter((widget) => !existing.has(widget.id));
  });

  const serviceOptions = createMemo(() => {
    const set = new Set<string>();
    for (const alert of alerts() || []) set.add(alert.service);
    for (const incident of incidents() || []) set.add(incident.service);
    for (const log of logs() || []) if (log.service) set.add(log.service);
    for (const service of catalogServices() || []) set.add(service.name);
    return Array.from(set).filter(Boolean).sort();
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

  function addWidget(widgetId: string) {
    const widget = WIDGETS.find((w) => w.id === widgetId);
    if (!widget) return;
    if (layout().some((item) => item.id === widget.id)) return;
    setLayout((curr) =>
      normalizeLayout([
        ...curr,
        {
          id: widget.id,
          x: 0,
          y: nextY(curr),
          w: widget.defaultW,
          h: widget.defaultH
        }
      ])
    );
    setNotice(`Added ${widget.title}.`);
  }

  function setWidgetSince(widgetId: string, since: string) {
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), since } }));
  }

  function setWidgetService(widgetId: string, service: string) {
    setWidgetConfig((curr) => ({ ...curr, [widgetId]: { ...(curr[widgetId] || {}), service } }));
  }

  function clearWidgetScope(widgetId: string) {
    setWidgetConfig((curr) => {
      const next = { ...curr };
      delete next[widgetId];
      return next;
    });
  }

  function removeWidget(widgetId: string) {
    const widget = WIDGETS.find((w) => w.id === widgetId);
    setLayout((curr) => curr.filter((item) => item.id !== widgetId));
    if (widget) setNotice(`Removed ${widget.title}.`);
  }

  function updateWidget(widgetId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    setLayout((curr) => packLayout(curr.map((item) => (item.id === widgetId ? fn(item) : item)), widgetId));
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

  function swapWidgetPositions(aId: string, bId: string) {
    if (!aId || !bId || aId === bId) return;
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

  function moveDraggedWidgetTo(point: { x: number; y: number }) {
    const draggedId = draggingWidgetId();
    if (!draggedId) return;
    setLayout((curr) => {
      const dragged = curr.find((item) => item.id === draggedId);
      if (!dragged) return curr;
      const placedX = clampSpan(point.x, 0, Math.max(0, GRID_COLUMNS - dragged.w));
      return packLayout(
        curr.map((item) => (item.id === draggedId ? { ...item, x: placedX, y: point.y } : item)),
        draggedId
      );
    });
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
      await updateDashboard(id, canvasName().trim() || selectedDashboardName(), activeLayout(), widgetConfig());
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
      const created = await createDashboard(name, activeLayout(), widgetConfig(), false);
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
    try {
      await deleteDashboard(id);
      setSelectedDashboardId("");
      await refetchDashboards();
      setNotice("Dashboard deleted.");
    } catch {
      setNotice("Failed to delete dashboard.");
    }
  }

  function widgetTitle(id: string): string {
    return WIDGETS.find((w) => w.id === id)?.title || id;
  }

  function renderWidget(item: DashboardWidgetPosition) {
    const widgetId = item.id;
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

    if (widgetId === "kpi-reliability") {
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

    if (widgetId === "alerts-feed") {
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

    if (widgetId === "incidents-live") {
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

    if (widgetId === "logs-errors") {
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

    if (widgetId === "deploy-correlation") {
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

    if (widgetId === "service-health") {
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

    if (widgetId === "oncall-now") {
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

    if (widgetId === "k8s-cluster") {
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

    if (widgetId === "notify-delivery") {
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
              onChange={(e) => setSelectedDashboardId(e.currentTarget.value)}
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
            <Button onClick={onRefreshAll}>Refresh</Button>
            <Button onClick={() => setShowPicker(true)}>Add Widget</Button>
            <Button variant="primary" onClick={onSaveCurrent}>Save</Button>
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
          <Input
            value={newDashboardName()}
            onInput={(e) => setNewDashboardName(e.currentTarget.value)}
            placeholder="New dashboard name"
          />
          <Button onClick={onSaveAsNew}>Save As New</Button>
          <Badge tone="neutral">{activeLayout().length} widgets</Badge>
          <Badge tone="ok">{selectedDashboardName()}</Badge>
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
          if (point) moveDraggedWidgetTo(point);
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
        <For each={activeLayout()}>
          {(item) => (
            <article
              class={`widget-card${draggingWidgetId() === item.id ? " is-dragging" : ""}${dragOverWidgetId() === item.id ? " is-drop-target" : ""}`}
              draggable
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
                <h3>{widgetTitle(item.id)}</h3>
                <div class="panel-actions widget-controls">
                  <Show when={editMode()}>
                    <button class="widget-mini-btn" onClick={() => nudgeWidget(item.id, -1, 0)} title="Move left">L</button>
                    <button class="widget-mini-btn" onClick={() => nudgeWidget(item.id, 1, 0)} title="Move right">R</button>
                    <button class="widget-mini-btn" onClick={() => nudgeWidget(item.id, 0, -1)} title="Move up">U</button>
                    <button class="widget-mini-btn" onClick={() => nudgeWidget(item.id, 0, 1)} title="Move down">D</button>
                    <button class="widget-mini-btn" onClick={() => resizeWidget(item.id, -1, 0)} title="Narrow">W-</button>
                    <button class="widget-mini-btn" onClick={() => resizeWidget(item.id, 1, 0)} title="Wider">W+</button>
                    <button class="widget-mini-btn" onClick={() => resizeWidget(item.id, 0, -1)} title="Shorter">H-</button>
                    <button class="widget-mini-btn" onClick={() => resizeWidget(item.id, 0, 1)} title="Taller">H+</button>
                    <button
                      class="widget-mini-btn"
                      onClick={() => setSettingsWidgetId((curr) => (curr === item.id ? "" : item.id))}
                      title="Widget scope"
                    >
                      Scope
                    </button>
                  </Show>
                  <Button onClick={() => removeWidget(item.id)}>Remove</Button>
                </div>
              </header>
              <Show when={editMode() && settingsWidgetId() === item.id}>
                <div class="widget-scope-row">
                  <select
                    class="input widget-scope-select"
                    value={widgetConfig()[item.id]?.since || ""}
                    onChange={(e) => setWidgetSince(item.id, e.currentTarget.value)}
                  >
                    <option value="">default time ({timeRange()})</option>
                    <option value="15m">last 15m</option>
                    <option value="1h">last 1h</option>
                    <option value="6h">last 6h</option>
                    <option value="24h">last 24h</option>
                  </select>
                  <select
                    class="input widget-scope-select"
                    value={widgetConfig()[item.id]?.service || ""}
                    onChange={(e) => setWidgetService(item.id, e.currentTarget.value)}
                  >
                    <option value="">default service ({serviceFilter() || "all"})</option>
                    <For each={serviceOptions()}>
                      {(svc) => <option value={svc}>{svc}</option>}
                    </For>
                  </select>
                  <Button onClick={() => clearWidgetScope(item.id)}>Clear Scope</Button>
                </div>
              </Show>
              {renderWidget(item)}
            </article>
          )}
        </For>
      </section>

      <Show when={showPicker()}>
        <div class="modal-overlay" onClick={() => setShowPicker(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Add Widget</h3>
            <div class="widget-picker-grid">
              <For each={availableToAdd()}>
                {(widget) => (
                  <button class="widget-picker-card" onClick={() => addWidget(widget.id)}>
                    <strong>{widget.title}</strong>
                    <p>{widget.description}</p>
                    <span class="mono">{widget.defaultW}x{widget.defaultH}</span>
                  </button>
                )}
              </For>
              <Show when={availableToAdd().length === 0}>
                <p class="paragraph">All widgets are already on this dashboard.</p>
              </Show>
            </div>
            <div class="row">
              <Button onClick={() => setShowPicker(false)}>Close</Button>
            </div>
          </div>
        </div>
      </Show>
    </>
  );
}
