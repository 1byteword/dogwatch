import { A } from "@solidjs/router";
import { For, Show } from "solid-js";
import type uPlot from "uplot";
import { Badge } from "../../design/components/Badge";
import { ChartPanel } from "../../design/components/ChartPanel";
import { Sparkline } from "../../design/components/Sparkline";
import type { DashboardWidgetConfig, DashboardWidgetPosition, DashboardVariable } from "../../domains/dashboards/types";
import type { AlertItem } from "../../domains/alerts/types";
import type { IncidentItem } from "../../domains/incidents/types";
import type { LogEntry, LogPattern, TrendingPattern, LogComparison } from "../../domains/logs/types";
import type { CatalogService, CatalogStats } from "../../domains/catalog/types";
import type { DeployIncidentCorrelation } from "../../domains/correlation/types";
import type { OncallSchedule, OncallPolicy, OncallCurrent } from "../../domains/oncall/types";
import type { K8sSummary, K8sContainer, K8sPod, K8sDeployment, K8sEvent } from "../../domains/kubernetes/types";
import type { NotifyChannel, NotifyLog } from "../../domains/notify/types";
import type { SystemMetrics, StatsSummary, ProcessInfo, ServiceMapData, TraceSummary, TraceDependency, SystemMetricPoint, TraceSpan } from "../../domains/ops/types";
import type { SloDefinition, SyntheticCheck, SyntheticFailure } from "../../domains/slo/types";
import type { CostEstimate, CardinalityHotspot, CostRecommendation, CostQuickWin } from "../../domains/cost/types";
import type { Anomaly, DbQuery, FlamegraphHotspot, SlowQuery } from "../../domains/performance/types";
import type { Deploy, DeployStats } from "../../domains/deploys/types";
import type { AuditLogRow, ApiKeyInfo, BackupInfo } from "../../domains/audit/types";
import type { WatchRule, AlertSilence } from "../../domains/alerts/types";
import { baseWidgetId, healthTone } from "./gridEngine";

export interface WidgetData {
  alerts: () => AlertItem[] | undefined;
  incidents: () => IncidentItem[] | undefined;
  logs: () => LogEntry[] | undefined;
  catalogStats: () => CatalogStats | undefined;
  catalogServices: () => CatalogService[] | undefined;
  correlations: () => DeployIncidentCorrelation[] | undefined;
  schedules: () => OncallSchedule[] | undefined;
  policies: () => OncallPolicy[] | undefined;
  currentOncall: () => OncallCurrent | null | undefined;
  k8sSummary: () => K8sSummary | undefined;
  channels: () => NotifyChannel[] | undefined;
  history: () => NotifyLog[] | undefined;
  systemMetrics: () => SystemMetrics | null | undefined;
  systemHistory: () => SystemMetricPoint[] | undefined;
  statsSummary: () => StatsSummary | undefined;
  topProcesses: () => ProcessInfo[] | undefined;
  serviceMap: () => ServiceMapData | undefined;
  traceSummaries: () => TraceSummary[] | undefined;
  traceServices: () => string[] | undefined;
  traceDependencies: () => TraceDependency[] | undefined;
  slos: () => SloDefinition[] | undefined;
  syntheticChecks: () => SyntheticCheck[] | undefined;
  syntheticFailures: () => SyntheticFailure[] | undefined;
  costEstimate: () => CostEstimate | null | undefined;
  cardinalityHotspots: () => CardinalityHotspot[] | undefined;
  costRecommendations: () => CostRecommendation[] | undefined;
  anomalies: () => Anomaly[] | undefined;
  dbQueries: () => DbQuery[] | undefined;
  flamegraphHotspots: () => FlamegraphHotspot[] | undefined;
  watches: () => WatchRule[] | undefined;
  silences: () => AlertSilence[] | undefined;
  logPatterns: () => LogPattern[] | undefined;
  trendingPatterns: () => TrendingPattern[] | undefined;
  deploys: () => Deploy[] | undefined;
  auditLogs: () => AuditLogRow[] | undefined;
  apiKeys: () => ApiKeyInfo[] | undefined;
  backups: () => BackupInfo[] | undefined;
  k8sContainers: () => K8sContainer[] | undefined;
  k8sPodList: () => K8sPod[] | undefined;
  k8sDeployments: () => K8sDeployment[] | undefined;
  k8sEvents: () => K8sEvent[] | undefined;
  traceSpans: () => TraceSpan[] | undefined;
  logComparison: () => LogComparison | undefined;
  deployStats: () => DeployStats | null | undefined;
  slowQueries: () => SlowQuery[] | undefined;
  costQuickWins: () => CostQuickWin[] | undefined;
}

function sinceToMs(since: string): number {
  if (since === "15m") return 15 * 60_000;
  if (since === "6h") return 6 * 3_600_000;
  if (since === "24h") return 24 * 3_600_000;
  return 3_600_000;
}

function resolveVariable(name: string, dashboardVars: DashboardVariable[]): string {
  const v = dashboardVars.find((dv) => dv.name === name);
  return v?.current || "";
}

function resolveWidgetService(
  widgetId: string,
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string,
  dashboardVars: DashboardVariable[]
): string {
  const wc = widgetConfig[widgetId]?.service || "";
  if (wc.startsWith("$")) return resolveVariable(wc.slice(1), dashboardVars);
  return wc || serviceFilter || resolveVariable("service", dashboardVars);
}

function resolveWidgetSince(
  widgetId: string,
  widgetConfig: Record<string, DashboardWidgetConfig>,
  timeRange: string
): string {
  return widgetConfig[widgetId]?.since || timeRange;
}

function resolveWidgetSeverity(
  widgetId: string,
  widgetConfig: Record<string, DashboardWidgetConfig>,
  severityFilter: string,
  dashboardVars: DashboardVariable[]
): string {
  const wc = widgetConfig[widgetId]?.severity || "";
  if (wc.startsWith("$")) return resolveVariable(wc.slice(1), dashboardVars);
  return wc || severityFilter;
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

function filterAlerts(
  widgetId: string, alerts: AlertItem[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string, severityFilter: string, timeRange: string,
  dashboardVars: DashboardVariable[]
) {
  const svc = resolveWidgetService(widgetId, widgetConfig, serviceFilter, dashboardVars);
  const since = resolveWidgetSince(widgetId, widgetConfig, timeRange);
  const severity = resolveWidgetSeverity(widgetId, widgetConfig, severityFilter, dashboardVars);
  return alerts.filter(
    (a) => (!svc || a.service === svc) && matchesSeverity(a.severity, severity) && isWithinSince(a.startedAtRaw, since)
  );
}

function filterIncidents(
  widgetId: string, incidents: IncidentItem[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string, severityFilter: string, timeRange: string,
  dashboardVars: DashboardVariable[]
) {
  const svc = resolveWidgetService(widgetId, widgetConfig, serviceFilter, dashboardVars);
  const since = resolveWidgetSince(widgetId, widgetConfig, timeRange);
  const severity = resolveWidgetSeverity(widgetId, widgetConfig, severityFilter, dashboardVars);
  return incidents.filter(
    (i) => (!svc || i.service === svc) && matchesSeverity(i.severity, severity) && isWithinSince(i.startedAtRaw, since)
  );
}

function filterLogs(
  widgetId: string, logs: LogEntry[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string, severityFilter: string, timeRange: string,
  dashboardVars: DashboardVariable[]
) {
  const svc = resolveWidgetService(widgetId, widgetConfig, serviceFilter, dashboardVars);
  const horizon = sinceToMs(resolveWidgetSince(widgetId, widgetConfig, timeRange));
  const severity = resolveWidgetSeverity(widgetId, widgetConfig, severityFilter, dashboardVars);
  const now = Date.now();
  return logs.filter((row) => {
    if (svc && row.service !== svc) return false;
    if (!matchesSeverity(row.level, severity)) return false;
    const ts = new Date(row.timestamp).getTime();
    if (!Number.isFinite(ts)) return false;
    return now - ts <= horizon;
  });
}

function filterCorrelations(
  widgetId: string, correlations: DeployIncidentCorrelation[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string, severityFilter: string, timeRange: string,
  dashboardVars: DashboardVariable[]
) {
  const svc = resolveWidgetService(widgetId, widgetConfig, serviceFilter, dashboardVars);
  const since = resolveWidgetSince(widgetId, widgetConfig, timeRange);
  const severity = resolveWidgetSeverity(widgetId, widgetConfig, severityFilter, dashboardVars);
  return correlations.filter((corr) => {
    if (svc && corr.deployment.service !== svc) return false;
    if (severity) {
      const level = corr.confidence >= 0.75 ? "critical" : corr.confidence >= 0.5 ? "high" : corr.confidence >= 0.25 ? "medium" : "low";
      if (!matchesSeverity(level, severity)) return false;
    }
    return isWithinSince(corr.deployment.timestamp, since);
  });
}

function filterServices(
  widgetId: string, services: CatalogService[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  serviceFilter: string, severityFilter: string,
  dashboardVars: DashboardVariable[]
) {
  const svc = resolveWidgetService(widgetId, widgetConfig, serviceFilter, dashboardVars);
  const severity = resolveWidgetSeverity(widgetId, widgetConfig, severityFilter, dashboardVars);
  return services.filter((row) => {
    if (svc && row.name !== svc && row.displayName !== svc) return false;
    if (!severity) return true;
    const healthLevel = row.health === "unhealthy" ? "critical" : row.health === "degraded" ? "high" : "low";
    return matchesSeverity(healthLevel, severity);
  });
}

export function WidgetRenderer(props: {
  item: DashboardWidgetPosition;
  data: WidgetData;
  dashboardVars: () => DashboardVariable[];
  widgetConfig: () => Record<string, DashboardWidgetConfig>;
  serviceFilter: () => string;
  severityFilter: () => string;
  timeRange: () => string;
}) {
  const widgetId = props.item.id;
  const widgetTypeId = baseWidgetId(widgetId);
  const listRows = Math.max(4, props.item.h * 2 + 1);
  const logRows = Math.max(6, props.item.h * 3);
  const actionRows = Math.max(6, props.item.h * 2 + 2);

  const cfg = () => props.widgetConfig();
  const sf = () => props.serviceFilter();
  const sevf = () => props.severityFilter();
  const tr = () => props.timeRange();
  const dv = () => props.dashboardVars();

  const widgetAlerts = filterAlerts(widgetId, props.data.alerts() || [], cfg(), sf(), sevf(), tr(), dv());
  const widgetIncidents = filterIncidents(widgetId, props.data.incidents() || [], cfg(), sf(), sevf(), tr(), dv());
  const widgetLogs = filterLogs(widgetId, props.data.logs() || [], cfg(), sf(), sevf(), tr(), dv());
  const widgetCorrelations = filterCorrelations(widgetId, props.data.correlations() || [], cfg(), sf(), sevf(), tr(), dv());
  const widgetServices = filterServices(widgetId, props.data.catalogServices() || [], cfg(), sf(), sevf(), dv())
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
    healthy: props.data.catalogStats()?.healthy || 0,
    degraded: props.data.catalogStats()?.degraded || 0,
    unhealthy: props.data.catalogStats()?.unhealthy || 0
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
  const endpointRows = (props.data.statsSummary()?.endpoints || [])
    .filter((row) => {
      const svc = resolveWidgetService(widgetId, cfg(), sf(), dv());
      if (!svc) return true;
      return row.path.includes(svc);
    })
    .slice()
    .sort((a, b) => b.p99_ms - a.p99_ms || b.error_rate - a.error_rate)
    .slice(0, listRows);
  const connectionRows = (props.data.statsSummary()?.connections || [])
    .slice()
    .sort((a, b) => b.count - a.count)
    .slice(0, logRows);
  const processRows = (props.data.topProcesses() || []).slice(0, listRows);
  const traceRows = (props.data.traceSummaries() || []).slice(0, logRows);
  const traceServiceRows = (props.data.traceServices() || []).slice(0, logRows);
  const traceDependencyRows = (props.data.traceDependencies() || []).slice(0, listRows);
  const failedDelivery = (props.data.history() || [])
    .filter((entry) => entry.status.toLowerCase() !== "sent")
    .slice(0, logRows);
  const k8sNodesReady = props.data.k8sSummary()?.nodesReady || 0;
  const k8sNodes = props.data.k8sSummary()?.nodes || 0;
  const k8sPodsRunning = props.data.k8sSummary()?.podsRunning || 0;
  const k8sPods = props.data.k8sSummary()?.pods || 0;
  const k8sDeployHealthy = props.data.k8sSummary()?.deploymentsHealthy || 0;
  const k8sDeploys = props.data.k8sSummary()?.deployments || 0;
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
    const horizon = sinceToMs(resolveWidgetSince(widgetId, cfg(), tr()));
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
    const sys = props.data.systemMetrics();
    const historyData = (): uPlot.AlignedData => {
      const pts = props.data.systemHistory() || [];
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
    const stats = props.data.statsSummary();
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
    const nodes = props.data.serviceMap()?.nodes?.length || 0;
    const links = props.data.serviceMap()?.links?.length || 0;
    const processNodes = (props.data.serviceMap()?.nodes || []).filter((n) => n.type === "process" || n.type === "service").length;
    const externalNodes = (props.data.serviceMap()?.nodes || []).filter((n) => n.type === "external").length;
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
          <strong>{props.data.currentOncall()?.userName || "unassigned"}</strong>
        </div>
        <div>
          <label>Schedules</label>
          <strong>{(props.data.schedules() || []).length}</strong>
        </div>
        <div>
          <label>Escalation Policies</label>
          <strong>{(props.data.policies() || []).length}</strong>
        </div>
        <Badge tone={props.data.currentOncall()?.isOverride ? "warn" : "ok"}>
          {props.data.currentOncall()?.isOverride ? "override active" : "rotation active"}
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
          <strong>{props.data.k8sSummary()?.nodesReady || 0}/{props.data.k8sSummary()?.nodes || 0}</strong>
        </div>
        <div>
          <label>Pods Running</label>
          <strong>{props.data.k8sSummary()?.podsRunning || 0}/{props.data.k8sSummary()?.pods || 0}</strong>
        </div>
        <div>
          <label>Deployments Healthy</label>
          <strong>{props.data.k8sSummary()?.deploymentsHealthy || 0}/{props.data.k8sSummary()?.deployments || 0}</strong>
        </div>
        <Badge tone={(props.data.k8sSummary()?.warningEvents || 0) > 0 ? "warn" : "ok"}>
          warnings {(props.data.k8sSummary()?.warningEvents || 0)}
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
        <Badge tone={(props.data.k8sSummary()?.warningEvents || 0) > 0 ? "warn" : "ok"}>warning events: {props.data.k8sSummary()?.warningEvents || 0}</Badge>
      </div>
    );
  }

  if (widgetTypeId === "notify-delivery") {
    const failed = (props.data.history() || []).filter((item) => item.status.toLowerCase() !== "sent").length;
    const avgSuccess =
      (props.data.channels() || []).length > 0
        ? Math.round((props.data.channels() || []).reduce((sum, ch) => sum + ch.successRate, 0) / (props.data.channels() || []).length)
        : 0;

    return (
      <div class="widget-body widget-kpi-grid">
        <div>
          <label>Channels</label>
          <strong>{(props.data.channels() || []).length}</strong>
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
    const sloRows = (props.data.slos() || [])
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
    const sloRows = (props.data.slos() || [])
      .slice()
      .sort((a, b) => a.budgetRemaining - b.budgetRemaining)
      .slice(0, listRows);
    const breached = (props.data.slos() || []).filter((s) => s.status === "breached").length;
    const atRisk = (props.data.slos() || []).filter((s) => s.status === "at_risk").length;
    return (
      <div class="widget-body widget-list">
        <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
          <div>
            <label>Total SLOs</label>
            <strong>{(props.data.slos() || []).length}</strong>
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
    const checks = (props.data.syntheticChecks() || [])
      .slice()
      .sort((a, b) => a.uptimePercent - b.uptimePercent)
      .slice(0, listRows);
    const failing = (props.data.syntheticChecks() || []).filter((c) => c.status === "failing").length;
    return (
      <div class="widget-body widget-list">
        <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
          <div>
            <label>Total Checks</label>
            <strong>{(props.data.syntheticChecks() || []).length}</strong>
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
    const failures = (props.data.syntheticFailures() || []).slice(0, logRows);
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
    const cost = props.data.costEstimate();
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
    const hotspots = (props.data.cardinalityHotspots() || [])
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
    const recs = (props.data.costRecommendations() || [])
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
    const rows = (props.data.anomalies() || [])
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
    const rows = (props.data.dbQueries() || [])
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
    const rows = (props.data.flamegraphHotspots() || [])
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
    const rows = (props.data.watches() || []).slice(0, listRows);
    const disabled = (props.data.watches() || []).filter((w) => !w.enabled).length;
    return (
      <div class="widget-body widget-list">
        <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
          <div>
            <label>Total Rules</label>
            <strong>{(props.data.watches() || []).length}</strong>
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
    const rows = (props.data.silences() || []).slice(0, listRows);
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
    const rows = (props.data.logPatterns() || [])
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
    const rows = (props.data.trendingPatterns() || [])
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
    const rows = (props.data.deploys() || []).slice(0, logRows);
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
    const rows = (props.data.auditLogs() || []).slice(0, logRows);
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
    const keys = (props.data.apiKeys() || []).slice(0, listRows);
    return (
      <div class="widget-body widget-list">
        <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
          <div>
            <label>Active Keys</label>
            <strong>{(props.data.apiKeys() || []).length}</strong>
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
    const rows = (props.data.backups() || []).slice(0, listRows);
    const failed = (props.data.backups() || []).filter((b) => b.status === "failed").length;
    return (
      <div class="widget-body widget-list">
        <div class="widget-kpi-grid" style={{ flex: "none", "padding-bottom": "var(--v2-space-2)" }}>
          <div>
            <label>Total Backups</label>
            <strong>{(props.data.backups() || []).length}</strong>
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
    const rows = (props.data.k8sContainers() || []).slice(0, logRows);
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
    const rows = (props.data.k8sPodList() || []).slice().sort((a, b) => b.restartCount - a.restartCount).slice(0, logRows);
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
    const rows = (props.data.k8sDeployments() || []).slice(0, logRows);
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
    const rows = (props.data.k8sEvents() || [])
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
    const rows = (props.data.statsSummary()?.endpoints || []).slice().sort((a, b) => b.p99_ms - a.p99_ms).slice(0, logRows);
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
    const rows = (props.data.statsSummary()?.connections || []).slice().sort((a, b) => b.count - a.count).slice(0, logRows);
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
    const spans = (props.data.traceSpans() || []).slice(0, logRows * 2);
    const firstTrace = (props.data.traceSummaries() || [])[0];
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
    const cmp = props.data.logComparison();
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
    const stats = props.data.deployStats();
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
    const rows = (props.data.slowQueries() || []).slice().sort((a, b) => b.maxMs - a.maxMs).slice(0, logRows);
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
    const rows = (props.data.costQuickWins() || []).slice().sort((a, b) => b.monthlySavings - a.monthlySavings).slice(0, logRows);
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
