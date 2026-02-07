import { api } from "../../core/api";
import { DashboardItem, DashboardWidgetConfig, DashboardWidgetPosition } from "./types";

interface ApiDashboard {
  id?: string;
  name?: string;
  is_default?: boolean;
  created?: string;
  updated?: string;
  layout?: Array<{ id?: string; x?: number; y?: number; w?: number; h?: number }>;
  widget_config?: Record<string, { service?: string; since?: string }>;
}

function mapLayout(layout: ApiDashboard["layout"]): DashboardWidgetPosition[] {
  return (layout || []).map((item, idx) => ({
    id: item.id || `widget-${idx}`,
    x: Number(item.x || 0),
    y: Number(item.y || 0),
    w: Number(item.w || 4),
    h: Number(item.h || 2)
  }));
}

function mapDashboard(raw: ApiDashboard, idx: number): DashboardItem {
  const widgetConfig: Record<string, DashboardWidgetConfig> = {};
  const configRaw = raw.widget_config || {};
  for (const [widgetId, cfg] of Object.entries(configRaw)) {
    widgetConfig[widgetId] = {
      service: cfg?.service || undefined,
      since: cfg?.since || undefined
    };
  }

  return {
    id: raw.id || `dash-${idx}`,
    name: raw.name || `Dashboard ${idx + 1}`,
    isDefault: Boolean(raw.is_default),
    created: raw.created,
    updated: raw.updated,
    layout: mapLayout(raw.layout),
    widgetConfig
  };
}

export async function loadDashboards(): Promise<DashboardItem[]> {
  const raw = await api.get<unknown>("/api/dashboards");
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => mapDashboard(item as ApiDashboard, idx));
}

export async function loadDashboard(id: string): Promise<DashboardItem | null> {
  if (!id) return null;
  try {
    const raw = await api.get<ApiDashboard>(`/api/dashboards/${encodeURIComponent(id)}`);
    return mapDashboard(raw, 0);
  } catch {
    return null;
  }
}

export async function loadDefaultDashboard(): Promise<DashboardItem | null> {
  try {
    const raw = await api.get<ApiDashboard | null>("/api/dashboards/default");
    if (!raw) return null;
    return mapDashboard(raw as ApiDashboard, 0);
  } catch {
    return null;
  }
}

export async function createDashboard(
  name: string,
  layout: DashboardWidgetPosition[],
  widgetConfig: Record<string, DashboardWidgetConfig>,
  isDefault = false
): Promise<DashboardItem | null> {
  const res = await fetch("/api/dashboards", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, layout, widget_config: widgetConfig, is_default: isDefault })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const raw = (await res.json()) as ApiDashboard;
  return mapDashboard(raw, 0);
}

export async function updateDashboard(
  id: string,
  name: string,
  layout: DashboardWidgetPosition[],
  widgetConfig: Record<string, DashboardWidgetConfig>
): Promise<DashboardItem | null> {
  const res = await fetch(`/api/dashboards/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name, layout, widget_config: widgetConfig })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const raw = (await res.json()) as ApiDashboard;
  return mapDashboard(raw, 0);
}

export async function deleteDashboard(id: string): Promise<void> {
  const res = await fetch(`/api/dashboards/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function setDefaultDashboard(id: string): Promise<void> {
  const res = await fetch(`/api/dashboards/${encodeURIComponent(id)}/default`, { method: "POST" });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}
