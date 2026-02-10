import { api } from "../../core/api";
import { LogComparison, LogEntry, LogPattern, LogQuery, TrendingPattern } from "./types";

interface LogApiResponse {
  data?: Array<Record<string, unknown>>;
  entries?: Array<Record<string, unknown>>;
}

function mapLog(raw: Record<string, unknown>, idx: number): LogEntry {
  return {
    id: String(raw.id || `${raw.timestamp || "log"}-${idx}`),
    timestamp: String(raw.timestamp || new Date().toISOString()),
    level: String(raw.level || "info").toLowerCase(),
    service: raw.service ? String(raw.service) : "",
    message: String(raw.message || "")
  };
}

export async function loadLogs(query: LogQuery): Promise<LogEntry[]> {
  const params = new URLSearchParams();
  if (query.q) params.set("q", query.q);
  if (query.level) params.set("level", query.level);
  if (query.service) params.set("service", query.service);
  if (query.since) params.set("since", query.since);
  params.set("limit", String(query.limit || 200));

  const res = await api.get<LogApiResponse>(`/api/logs?${params.toString()}`);
  const rows = (res.data || res.entries || []) as Array<Record<string, unknown>>;
  return rows.map(mapLog);
}

export async function loadLogServices(): Promise<string[]> {
  const res = await api.get<unknown>("/api/logs/services");
  return Array.isArray(res) ? res.map((s) => String(s)) : [];
}

export async function loadLogPatterns(): Promise<LogPattern[]> {
  try {
    const raw = await api.get<Array<Partial<LogPattern>>>("/api/logs/patterns");
    return (raw || []).map((row, idx) => ({
      id: row.id || `pattern-${idx}`,
      pattern: row.pattern || "",
      count: Number(row.count || 0),
      level: row.level || "info",
      services: Array.isArray(row.services) ? row.services : [],
      sample: row.sample || ""
    }));
  } catch {
    return [];
  }
}

export async function loadLogComparison(service: string, before: string, after: string): Promise<LogComparison> {
  try {
    const params = new URLSearchParams();
    if (service) params.set("service", service);
    if (before) params.set("before", before);
    if (after) params.set("after", after);
    const raw = await api.get<Partial<{
      before_entries?: Array<Record<string, unknown>>;
      after_entries?: Array<Record<string, unknown>>;
      added_patterns?: string[];
      removed_patterns?: string[];
    }>>(`/api/logs/compare?${params.toString()}`);
    return {
      beforeEntries: (raw.before_entries || []).map(mapLog),
      afterEntries: (raw.after_entries || []).map(mapLog),
      addedPatterns: raw.added_patterns || [],
      removedPatterns: raw.removed_patterns || []
    };
  } catch {
    return { beforeEntries: [], afterEntries: [], addedPatterns: [], removedPatterns: [] };
  }
}

export async function loadTrendingPatterns(): Promise<TrendingPattern[]> {
  try {
    const raw = await api.get<Array<Partial<TrendingPattern & { growth_percent?: number; first_seen?: string }>>>("/api/logs/patterns/trending");
    return (raw || []).map((row, idx) => ({
      id: row.id || `trending-${idx}`,
      pattern: row.pattern || "",
      count: Number(row.count || 0),
      growthPercent: Number(row.growthPercent ?? row.growth_percent ?? 0),
      level: row.level || "info",
      firstSeen: row.firstSeen || row.first_seen || ""
    }));
  } catch {
    return [];
  }
}
