import { ErrorBoundary, For, Show, batch, createEffect, createMemo, createResource, createSignal, onCleanup, onMount, untrack } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
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
import { DashboardWidgetConfig, DashboardWidgetPosition, DashboardVariable } from "../domains/dashboards/types";
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

import {
  baseWidgetId,
  clampSpan,
  compactLayout,
  GRID_COLUMNS,
  GRID_MAX_H,
  GRID_MAX_Y,
  GRID_RESIZE_DRAG_STEP_PX,
  GRID_ROW_HEIGHT_PX,
  nextY,
  normalizeLayout,
  normalizeWidgetConfig,
  overlaps,
  packLayout,
  quantizeDragDelta,
  sanitizeLayout,
  WIDGET_INSTANCE_SEP,
} from "./dashboards/gridEngine";
import { WIDGETS, WIDGET_CATEGORIES } from "./dashboards/widgetDefs";
import type { WidgetDef } from "./dashboards/widgetDefs";
import {
  DASHBOARD_TEMPLATES,
  DEFAULT_DASHBOARD_NAME,
  DEFAULT_VARIABLES,
  DRAFT_DEBOUNCE_MS,
  DRAFT_MAX_AGE_MS,
  DRAFT_STORAGE_KEY,
  defaultLayout,
  EDIT_MODE_PREF_KEY,
  MAX_HISTORY,
} from "./dashboards/templates";
import type { CopiedWidget, EditorSnapshot, ResizeAxis } from "./dashboards/templates";
import { WidgetRenderer } from "./dashboards/WidgetRenderer";
import type { WidgetData } from "./dashboards/WidgetRenderer";
import { WidgetInspector } from "./dashboards/WidgetInspector";
import { WidgetPicker } from "./dashboards/WidgetPicker";

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
  const [pickerCategory, setPickerCategory] = createSignal("");
  const [showCreateDashboard, setShowCreateDashboard] = createSignal(false);
  const [dashboardVars, setDashboardVars] = createSignal<DashboardVariable[]>(DEFAULT_VARIABLES.map((v) => ({ ...v })));
  const [showVarEditor, setShowVarEditor] = createSignal(false);
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

  const widgetData: WidgetData = {
    alerts, incidents, logs, catalogStats, catalogServices, correlations,
    schedules, policies, currentOncall, k8sSummary, channels, history,
    systemMetrics, systemHistory, statsSummary, topProcesses, serviceMap,
    traceSummaries, traceServices, traceDependencies, slos, syntheticChecks,
    syntheticFailures, costEstimate, cardinalityHotspots, costRecommendations,
    anomalies, dbQueries, flamegraphHotspots, watches, silences, logPatterns,
    trendingPatterns, deploys, auditLogs, apiKeys, backups, k8sContainers,
    k8sPodList, k8sDeployments, k8sEvents, traceSpans, logComparison,
    deployStats, slowQueries, costQuickWins,
  };

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
        const wId = focusedWidgetId();
        if (!mod && !e.altKey && key === "l") {
          e.preventDefault();
          setWidgetLocked(wId, !isWidgetLocked(wId));
          return;
        }
        if (mod && key === "d") {
          e.preventDefault();
          duplicateWidget(wId);
          return;
        }
        if (mod && key === "c") {
          e.preventDefault();
          copyWidget(wId);
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
                  if (isWidgetLocked(wId)) return;
                  resizeWidget(wId, -1, 0);
                  return;
                }
                if (e.key === "ArrowRight") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  resizeWidget(wId, 1, 0);
                  return;
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  resizeWidget(wId, 0, -1);
                  return;
                }
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  resizeWidget(wId, 0, 1);
                  return;
                }
              } else {
                if (e.key === "ArrowLeft") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  nudgeWidget(wId, -1, 0);
                  return;
                }
                if (e.key === "ArrowRight") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  nudgeWidget(wId, 1, 0);
                  return;
                }
                if (e.key === "ArrowUp") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  nudgeWidget(wId, 0, -1);
                  return;
                }
                if (e.key === "ArrowDown") {
                  e.preventDefault();
                  if (isWidgetLocked(wId)) return;
                  nudgeWidget(wId, 0, 1);
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
    const cat = pickerCategory();
    return WIDGETS.filter((widget) => {
      if (cat && widget.category !== cat) return false;
      if (!q) return true;
      return widget.title.toLowerCase().includes(q) ||
        widget.id.toLowerCase().includes(q) ||
        widget.description.toLowerCase().includes(q);
    });
  });
  const groupedWidgets = createMemo(() => {
    const items = availableToAdd();
    const groups: { category: string; widgets: WidgetDef[] }[] = [];
    for (const cat of WIDGET_CATEGORIES) {
      const ws = items.filter((w) => w.category === cat);
      if (ws.length > 0) groups.push({ category: cat, widgets: ws });
    }
    return groups;
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
    const opts = Array.from(set).filter(Boolean).sort();
    // Update service variable options
    setDashboardVars((prev) => prev.map((v) =>
      v.type === "service" ? { ...v, options: opts } : v
    ));
    return opts;
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
    const bId = baseWidgetId(widgetId);
    const widget = WIDGETS.find((w) => w.id === bId);
    if (!widget) return;
    const id = nextWidgetInstanceId(bId);
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

  function setWidgetSince(wId: string, since: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [wId]: { ...(curr[wId] || {}), since } }));
  }

  function setWidgetService(wId: string, service: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [wId]: { ...(curr[wId] || {}), service } }));
  }

  function setWidgetSeverity(wId: string, severity: string) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [wId]: { ...(curr[wId] || {}), severity } }));
  }

  function setWidgetLocked(wId: string, locked: boolean) {
    pushHistory();
    setWidgetConfig((curr) => ({ ...curr, [wId]: { ...(curr[wId] || {}), locked } }));
    setNotice(locked ? `Locked ${widgetTitle(wId)}.` : `Unlocked ${widgetTitle(wId)}.`);
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

  function clearWidgetScope(wId: string) {
    pushHistory();
    setWidgetConfig((curr) => {
      const next = { ...curr };
      delete next[wId];
      return next;
    });
  }

  function removeWidget(wId: string) {
    if (isWidgetLocked(wId)) {
      setNotice("Widget is locked. Unlock in inspector to remove.");
      return;
    }
    const widget = widgetDefForId(wId);
    pushHistory();
    setLayout((curr) => compactLayout(curr.filter((item) => item.id !== wId)));
    setWidgetConfig((curr) => {
      if (!(wId in curr)) return curr;
      const next = { ...curr };
      delete next[wId];
      return next;
    });
    if (focusedWidgetId() === wId) setFocusedWidgetId("");
    if (widget) setNotice(`Removed ${widget.title}.`);
  }

  function duplicateWidget(wId: string) {
    const src = layout().find((item) => item.id === wId);
    if (!src) return;
    const bId = baseWidgetId(wId);
    const def = widgetDefForId(wId);
    const id = nextWidgetInstanceId(bId);
    const cfg = widgetConfig()[wId];
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
    setNotice(`Duplicated ${def?.title || bId}.`);
  }

  function copyWidget(wId: string) {
    const src = layout().find((item) => item.id === wId);
    if (!src) return;
    setCopiedWidget({
      baseId: baseWidgetId(wId),
      w: src.w,
      h: src.h,
      config: { ...(widgetConfig()[wId] || {}) }
    });
    setNotice(`Copied ${widgetTitle(wId)}.`);
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

  function updateWidget(wId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    if (isWidgetLocked(wId)) {
      setNotice("Widget is locked. Unlock in inspector to edit.");
      return;
    }
    pushHistory();
    setLayout((curr) => packLayout(curr.map((item) => (item.id === wId ? fn(item) : item)), wId));
  }

  function applyWidgetUpdate(wId: string, fn: (item: DashboardWidgetPosition) => DashboardWidgetPosition) {
    setLayout((curr) => packLayout(curr.map((item) => (item.id === wId ? fn(item) : item)), wId));
  }


  function nudgeWidget(wId: string, dx: number, dy: number) {
    updateWidget(wId, (item) => ({
      ...item,
      x: clampSpan(item.x + dx, 0, Math.max(0, GRID_COLUMNS - item.w)),
      y: clampSpan(item.y + dy, 0, GRID_MAX_Y)
    }));
  }

  function resizeWidget(wId: string, dw: number, dh: number) {
    updateWidget(wId, (item) => {
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

  function startResize(wId: string, axis: ResizeAxis, event: MouseEvent) {
    if (!editMode() || !canvasRef) return;
    if (isWidgetLocked(wId)) {
      setNotice("Widget is locked. Unlock in inspector to resize.");
      return;
    }
    const target = layout().find((item) => item.id === wId);
    if (!target) return;

    event.preventDefault();
    event.stopPropagation();

    pushHistory();
    setFocusedWidgetId(wId);
    setResizingWidgetId(wId);

    const startX = event.clientX;
    const startY = event.clientY;
    const startW = target.w;
    const startH = target.h;
    const rect = canvasRef.getBoundingClientRect();
    const colWidth = rect.width > 0 ? rect.width / GRID_COLUMNS : 1;

    const onMove = (e: MouseEvent) => {
      const dxCols = quantizeDragDelta(e.clientX - startX, colWidth);
      const dyRows = quantizeDragDelta(e.clientY - startY, GRID_RESIZE_DRAG_STEP_PX);
      applyWidgetUpdate(wId, (item) => {
        const nW = axis === "e" || axis === "se" ? clampSpan(startW + dxCols, 2, GRID_COLUMNS) : startW;
        const nH = axis === "s" || axis === "se" ? clampSpan(startH + dyRows, 2, GRID_MAX_H) : startH;
        return {
          ...item,
          w: nW,
          h: nH,
          x: clampSpan(item.x, 0, Math.max(0, GRID_COLUMNS - nW))
        };
      });
    };

    const onUp = () => {
      setResizingWidgetId("");
      setLayout((curr) => packLayout(curr, wId));
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

  function onWidgetDragStart(wId: string, e: DragEvent) {
    if (isWidgetLocked(wId)) {
      setNotice("Widget is locked. Unlock in inspector to move.");
      return;
    }
    const widget = layout().find((item) => item.id === wId);
    if (canvasRef && widget) {
      const canvasRect = canvasRef.getBoundingClientRect();
      const colWidth = canvasRect.width / GRID_COLUMNS;
      const cursorCol = Math.floor((e.clientX - canvasRect.left) / colWidth);
      const cursorRow = Math.floor((e.clientY - canvasRect.top) / GRID_ROW_HEIGHT_PX);
      setDragGrabOffset({
        cols: clampSpan(cursorCol - widget.x, 0, widget.w - 1),
        rows: clampSpan(cursorRow - widget.y, 0, widget.h - 1)
      });
    } else {
      setDragGrabOffset({ cols: 0, rows: 0 });
    }
    setDraggingWidgetId(wId);
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

  function canvasPointToGrid(clientX: number, clientY: number, applyGrabOff = false): { x: number; y: number } | null {
    if (!canvasRef) return null;
    const rect = canvasRef.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const columnWidth = rect.width / GRID_COLUMNS;
    const rawCol = Math.floor((clientX - rect.left) / columnWidth);
    const rawRow = Math.floor((clientY - rect.top) / GRID_ROW_HEIGHT_PX);
    const offset = applyGrabOff ? dragGrabOffset() : { cols: 0, rows: 0 };
    const dragged = applyGrabOff ? layout().find((item) => item.id === draggingWidgetId()) : null;
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
    const bId = baseWidgetId(id);
    const baseTitle = WIDGETS.find((w) => w.id === bId)?.title || bId;
    if (id === bId) return baseTitle;
    const suffix = id.split(WIDGET_INSTANCE_SEP)[1];
    return suffix ? `${baseTitle} #${suffix}` : baseTitle;
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
          <Button variant="primary" onClick={() => setShowCreateDashboard(true)}>New Dashboard</Button>
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

      {/* Dashboard variables bar */}
      <Show when={dashboardVars().length > 0}>
        <section class="dashboard-variables-bar" style={{ display: "flex", "align-items": "center", gap: "12px", padding: "6px 12px", background: "var(--surface)", "border-bottom": "1px solid var(--border)", "flex-wrap": "wrap" }}>
          <For each={dashboardVars()}>
            {(v, idx) => (
              <div style={{ display: "flex", "align-items": "center", gap: "4px" }}>
                <label style={{ "font-size": "12px", color: "var(--text-muted)", "white-space": "nowrap" }}>
                  ${v.name}
                </label>
                <select
                  class="input dashboard-filter-select"
                  style={{ "min-width": "120px", "font-size": "12px" }}
                  value={v.current}
                  onChange={(e) => {
                    const val = e.currentTarget.value;
                    setDashboardVars((prev) => prev.map((dv, i) =>
                      i === idx() ? { ...dv, current: val } : dv
                    ));
                    // Sync $service to serviceFilter for backward compat
                    if (v.name === "service") setServiceFilter(val);
                  }}
                >
                  <option value="">all</option>
                  <For each={v.type === "service" ? serviceOptions() : v.options}>
                    {(opt) => <option value={opt}>{opt}</option>}
                  </For>
                </select>
              </div>
            )}
          </For>
          <Show when={editMode()}>
            <Button onClick={() => setShowVarEditor(true)} style={{ "font-size": "11px", padding: "2px 8px" }}>
              Edit Variables
            </Button>
          </Show>
        </section>
      </Show>

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
                  <WidgetRenderer
                    item={item}
                    data={widgetData}
                    dashboardVars={dashboardVars}
                    widgetConfig={widgetConfig}
                    serviceFilter={serviceFilter}
                    severityFilter={severityFilter}
                    timeRange={timeRange}
                  />
                </Show>
              </ErrorBoundary>
            </article>
          )}
        </For>
      </section>

      <Show when={editMode() && focusedWidget()}>
        <WidgetInspector
          widget={focusedWidget()!}
          widgetSince={widgetConfig()[focusedWidget()!.id]?.since || ""}
          widgetService={widgetConfig()[focusedWidget()!.id]?.service || ""}
          widgetSeverity={widgetConfig()[focusedWidget()!.id]?.severity || ""}
          isLocked={isWidgetLocked(focusedWidget()!.id)}
          widgetTitle={widgetTitle(focusedWidget()!.id)}
          timeRange={timeRange()}
          serviceFilter={serviceFilter()}
          severityFilter={severityFilter()}
          serviceOptions={serviceOptions()}
          dashboardVars={dashboardVars()}
          onSetSince={(v) => setWidgetSince(focusedWidget()!.id, v)}
          onSetService={(v) => setWidgetService(focusedWidget()!.id, v)}
          onSetSeverity={(v) => setWidgetSeverity(focusedWidget()!.id, v)}
          onToggleLock={() => setWidgetLocked(focusedWidget()!.id, !isWidgetLocked(focusedWidget()!.id))}
          onCopy={() => copyWidget(focusedWidget()!.id)}
          onDuplicate={() => duplicateWidget(focusedWidget()!.id)}
          onPaste={pasteWidget}
          onClearScope={() => clearWidgetScope(focusedWidget()!.id)}
          onRemove={() => removeWidget(focusedWidget()!.id)}
          canPaste={!!copiedWidget()}
        />
      </Show>

      <Show when={showPicker()}>
        <WidgetPicker
          groups={groupedWidgets()}
          query={pickerQuery()}
          category={pickerCategory()}
          onQueryChange={setPickerQuery}
          onCategoryChange={setPickerCategory}
          onAdd={addWidget}
          onClose={() => { setShowPicker(false); setPickerCategory(""); setPickerQuery(""); }}
          emptyResults={availableToAdd().length === 0}
        />
      </Show>
      {/* Variable editor modal */}
      <Show when={showVarEditor()}>
        <div class="modal-overlay" onClick={() => setShowVarEditor(false)}>
          <div class="modal-content" onClick={(e) => e.stopPropagation()} style={{ "max-width": "560px" }}>
            <h3 style={{ margin: "0 0 16px" }}>Dashboard Variables</h3>
            <p style={{ "font-size": "13px", color: "var(--text-muted)", margin: "0 0 12px" }}>
              Variables act as global filters. Use <code>$name</code> in widget service/severity scope to bind to a variable.
            </p>
            <div style={{ display: "flex", "flex-direction": "column", gap: "10px", "max-height": "50vh", overflow: "auto" }}>
              <For each={dashboardVars()}>
                {(v, idx) => (
                  <div style={{ display: "flex", gap: "8px", "align-items": "center", padding: "8px", background: "var(--surface)", "border-radius": "6px" }}>
                    <div style={{ flex: "1", display: "flex", "flex-direction": "column", gap: "6px" }}>
                      <div style={{ display: "grid", "grid-template-columns": "1fr 1fr 1fr", gap: "6px" }}>
                        <Input
                          value={v.name}
                          onInput={(e) => setDashboardVars((prev) => prev.map((dv, i) => i === idx() ? { ...dv, name: e.currentTarget.value } : dv))}
                          placeholder="name"
                          style={{ "font-size": "12px" }}
                          aria-label="Variable name"
                        />
                        <Input
                          value={v.label}
                          onInput={(e) => setDashboardVars((prev) => prev.map((dv, i) => i === idx() ? { ...dv, label: e.currentTarget.value } : dv))}
                          placeholder="label"
                          style={{ "font-size": "12px" }}
                          aria-label="Variable label"
                        />
                        <select
                          class="form-select"
                          style={{ "font-size": "12px" }}
                          value={v.type}
                          onChange={(e) => setDashboardVars((prev) => prev.map((dv, i) => i === idx() ? { ...dv, type: e.currentTarget.value as DashboardVariable["type"] } : dv))}
                        >
                          <option value="service">Service (auto)</option>
                          <option value="severity">Severity</option>
                          <option value="timerange">Time Range</option>
                          <option value="custom">Custom</option>
                        </select>
                      </div>
                      <Show when={v.type === "custom"}>
                        <Input
                          value={v.options.join(", ")}
                          onInput={(e) => setDashboardVars((prev) => prev.map((dv, i) => i === idx() ? { ...dv, options: e.currentTarget.value.split(",").map((s) => s.trim()).filter(Boolean) } : dv))}
                          placeholder="comma-separated values"
                          style={{ "font-size": "12px" }}
                          aria-label="Custom options"
                        />
                      </Show>
                    </div>
                    <Button
                      variant="danger"
                      onClick={() => setDashboardVars((prev) => prev.filter((_, i) => i !== idx()))}
                      style={{ "font-size": "11px", padding: "4px 8px" }}
                    >
                      X
                    </Button>
                  </div>
                )}
              </For>
            </div>
            <div style={{ display: "flex", gap: "8px", "justify-content": "space-between", "margin-top": "12px" }}>
              <Button onClick={() => setDashboardVars((prev) => [...prev, { name: "", label: "", type: "custom", options: [], defaultValue: "", current: "" }])}>
                Add Variable
              </Button>
              <Button variant="primary" onClick={() => setShowVarEditor(false)}>Done</Button>
            </div>
          </div>
        </div>
      </Show>

      <Show when={showCreateDashboard()}>
        <div class="modal-overlay" onClick={() => setShowCreateDashboard(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()} style={{ "max-width": "680px" }}>
            <h3 style={{ margin: "0 0 4px" }}>New Dashboard</h3>
            <p style={{ color: "var(--text-muted)", "font-size": "13px", margin: "0 0 16px" }}>
              Start from a template or build from scratch.
            </p>
            <div style={{ display: "flex", "flex-direction": "column", gap: "8px", "margin-bottom": "16px" }}>
              <Input
                value={newDashboardName()}
                onInput={(e) => setNewDashboardName(e.currentTarget.value)}
                placeholder="Dashboard name"
                aria-label="New dashboard name"
              />
            </div>
            <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "10px", "max-height": "50vh", overflow: "auto" }}>
              <button
                class="widget-picker-card"
                style={{ "text-align": "left", "border": "2px solid var(--border)" }}
                onClick={() => {
                  pushHistory();
                  setLayout([]);
                  setWidgetConfig({});
                  setCanvasName(newDashboardName().trim() || DEFAULT_DASHBOARD_NAME);
                  setShowCreateDashboard(false);
                  setEditMode(true);
                  setNotice("Blank canvas ready. Add widgets to get started.");
                }}
              >
                <strong>Blank Canvas</strong>
                <p style={{ color: "var(--text-muted)", "font-size": "12px", margin: "4px 0 0" }}>
                  Start empty and add widgets manually.
                </p>
              </button>
              <For each={DASHBOARD_TEMPLATES}>
                {(tpl) => (
                  <button
                    class="widget-picker-card"
                    style={{ "text-align": "left", "border": "2px solid var(--border)" }}
                    onClick={() => {
                      pushHistory();
                      setLayout(normalizeLayout(tpl.layout.slice()));
                      setWidgetConfig(normalizeWidgetConfig(tpl.widgetConfig || {}));
                      setCanvasName(newDashboardName().trim() || tpl.name);
                      setSelectedTemplateId(tpl.id);
                      setShowCreateDashboard(false);
                      setEditMode(true);
                      setNotice(`Created from template: ${tpl.name}. Save when ready.`);
                    }}
                  >
                    <strong>{tpl.name}</strong>
                    <p style={{ color: "var(--text-muted)", "font-size": "12px", margin: "4px 0 0" }}>
                      {tpl.description}
                    </p>
                    <span class="mono" style={{ "font-size": "11px", color: "var(--accent)" }}>
                      {tpl.layout.length} widgets
                    </span>
                  </button>
                )}
              </For>
            </div>
            <div class="row" style={{ "margin-top": "12px" }}>
              <Button onClick={() => setShowCreateDashboard(false)}>Cancel</Button>
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
