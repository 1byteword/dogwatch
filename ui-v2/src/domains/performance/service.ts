import { api } from "../../core/api";
import { Anomaly, DbQuery, FlamegraphHotspot, SlowQuery } from "./types";

export async function loadAnomalies(): Promise<Anomaly[]> {
  try {
    const raw = await api.get<Array<Partial<Anomaly & { detected_at?: string }>>>("/api/anomaly/recent");
    return (raw || []).map((row, idx) => ({
      id: row.id || `anomaly-${idx}`,
      metric: row.metric || "unknown",
      service: row.service || "",
      severity: toSeverity(row.severity),
      detectedAt: row.detectedAt || row.detected_at || "",
      description: row.description || "",
      score: Number(row.score || 0)
    }));
  } catch {
    return [];
  }
}

function toSeverity(s: string | undefined): Anomaly["severity"] {
  const v = (s || "").toLowerCase();
  if (v === "critical" || v === "high" || v === "medium" || v === "low") return v;
  return "medium";
}

export async function loadDbQueries(): Promise<DbQuery[]> {
  try {
    const raw = await api.get<Array<Partial<DbQuery & { avg_ms?: number; max_ms?: number; call_count?: number; error_count?: number }>>>("/api/dbwatch/queries");
    return (raw || []).map((row, idx) => ({
      id: row.id || `dbq-${idx}`,
      query: row.query || "",
      database: row.database || "",
      avgMs: Number(row.avgMs ?? row.avg_ms ?? 0),
      maxMs: Number(row.maxMs ?? row.max_ms ?? 0),
      callCount: Number(row.callCount ?? row.call_count ?? 0),
      errorCount: Number(row.errorCount ?? row.error_count ?? 0)
    }));
  } catch {
    return [];
  }
}

export async function loadSlowQueries(): Promise<SlowQuery[]> {
  try {
    const raw = await api.get<Array<Partial<SlowQuery & { avg_ms?: number; max_ms?: number; call_count?: number }>>>("/api/dbwatch/slow");
    return (raw || []).map((row, idx) => ({
      id: row.id || `slow-${idx}`,
      query: row.query || "",
      database: row.database || "",
      avgMs: Number(row.avgMs ?? row.avg_ms ?? 0),
      maxMs: Number(row.maxMs ?? row.max_ms ?? 0),
      callCount: Number(row.callCount ?? row.call_count ?? 0)
    }));
  } catch {
    return [];
  }
}

export async function loadFlamegraphHotspots(): Promise<FlamegraphHotspot[]> {
  try {
    const raw = await api.get<Partial<{ hotspots?: Array<Partial<FlamegraphHotspot & { self_percent?: number; total_percent?: number }>> }>>("/api/flamegraph");
    return (raw.hotspots || []).map((row) => ({
      function: row.function || "unknown",
      module: row.module || "",
      selfPercent: Number(row.selfPercent ?? row.self_percent ?? 0),
      totalPercent: Number(row.totalPercent ?? row.total_percent ?? 0),
      samples: Number(row.samples || 0)
    }));
  } catch {
    return [];
  }
}
