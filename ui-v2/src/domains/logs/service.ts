import { api } from "../../core/api";
import { LogEntry, LogQuery } from "./types";

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
