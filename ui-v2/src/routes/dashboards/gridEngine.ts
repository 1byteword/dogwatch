import type { DashboardWidgetConfig, DashboardWidgetPosition } from "../../domains/dashboards/types";

export const WIDGET_INSTANCE_SEP = "::";
export const GRID_COLUMNS = 12;
export const GRID_MAX_Y = 60;
export const GRID_ROW_HEIGHT_PX = 92;
export const GRID_MAX_H = 6;
export const GRID_RESIZE_DRAG_STEP_PX = 68;

export function clampSpan(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, Number(value) || min));
}

export function quantizeDragDelta(pxDelta: number, stepPx: number): number {
  if (!Number.isFinite(pxDelta) || !Number.isFinite(stepPx) || stepPx <= 0) return 0;
  return Math.trunc(pxDelta / stepPx);
}

export function overlaps(a: DashboardWidgetPosition, b: DashboardWidgetPosition): boolean {
  const noXOverlap = a.x + a.w <= b.x || b.x + b.w <= a.x;
  const noYOverlap = a.y + a.h <= b.y || b.y + b.h <= a.y;
  return !(noXOverlap || noYOverlap);
}

export function hasOverlap(item: DashboardWidgetPosition, placed: DashboardWidgetPosition[]): boolean {
  return placed.some((other) => overlaps(item, other));
}

export function findNearestSlot(
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
  const maxUsedY = placed.reduce((max, p) => Math.max(max, p.y + p.h), 0);
  return {
    ...item,
    x: 0,
    y: maxUsedY
  };
}

export function packLayout(items: DashboardWidgetPosition[], prioritizeId?: string): DashboardWidgetPosition[] {
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

export function normalizeLayout(items: DashboardWidgetPosition[]): DashboardWidgetPosition[] {
  return packLayout(items);
}

export function compactLayout(items: DashboardWidgetPosition[]): DashboardWidgetPosition[] {
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

export function sanitizeLayout(items: DashboardWidgetPosition[], allowed: Set<string>): DashboardWidgetPosition[] {
  return compactLayout(items.filter((item) => allowed.has(baseWidgetId(item.id))));
}

export function normalizeWidgetConfig(config: Record<string, DashboardWidgetConfig>): Record<string, DashboardWidgetConfig> {
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

export function nextY(layout: DashboardWidgetPosition[]): number {
  if (!layout.length) return 0;
  return Math.max(...layout.map((item) => item.y + item.h));
}

export function healthTone(healthy: number, degraded: number, unhealthy: number): "ok" | "warn" | "error" {
  if (unhealthy > 0) return "error";
  if (degraded > 0) return "warn";
  if (healthy > 0) return "ok";
  return "warn";
}

export function baseWidgetId(id: string): string {
  return id.split(WIDGET_INSTANCE_SEP)[0];
}
