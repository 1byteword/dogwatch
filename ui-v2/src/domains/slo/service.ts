import { api } from "../../core/api";
import { SloDefinition, SyntheticCheck, SyntheticFailure } from "./types";

export async function loadSlos(): Promise<SloDefinition[]> {
  try {
    const raw = await api.get<Array<Partial<SloDefinition & { budget_remaining?: number; burn_rate?: number }>>>("/api/slos");
    return (raw || []).map((row) => ({
      id: row.id || "",
      name: row.name || "unnamed",
      service: row.service || "",
      target: Number(row.target || 99.9),
      current: Number(row.current || 0),
      budgetRemaining: Number(row.budgetRemaining ?? row.budget_remaining ?? 100),
      burnRate: Number(row.burnRate ?? row.burn_rate ?? 0),
      window: row.window || "30d",
      status: toSloStatus(row.status)
    }));
  } catch {
    return [];
  }
}

function toSloStatus(s: string | undefined): SloDefinition["status"] {
  const v = (s || "").toLowerCase();
  if (v === "met" || v === "at_risk" || v === "breached") return v;
  return "met";
}

export async function loadSyntheticChecks(): Promise<SyntheticCheck[]> {
  try {
    const raw = await api.get<Array<Partial<SyntheticCheck & { last_run?: string; uptime_percent?: number; avg_latency_ms?: number }>>>("/api/synthetics/checks");
    return (raw || []).map((row) => ({
      id: row.id || "",
      name: row.name || "unnamed",
      url: row.url || "",
      type: row.type || "http",
      interval: Number(row.interval || 60),
      status: toCheckStatus(row.status),
      lastRun: row.lastRun || row.last_run || "",
      uptimePercent: Number(row.uptimePercent ?? row.uptime_percent ?? 0),
      avgLatencyMs: Number(row.avgLatencyMs ?? row.avg_latency_ms ?? 0)
    }));
  } catch {
    return [];
  }
}

function toCheckStatus(s: string | undefined): SyntheticCheck["status"] {
  const v = (s || "").toLowerCase();
  if (v === "passing" || v === "failing" || v === "degraded") return v;
  return "passing";
}

export async function loadSyntheticFailures(): Promise<SyntheticFailure[]> {
  try {
    const raw = await api.get<Array<Partial<SyntheticFailure & { check_id?: string; check_name?: string; status_code?: number; latency_ms?: number }>>>("/api/synthetics/failures");
    return (raw || []).map((row) => ({
      checkId: row.checkId || row.check_id || "",
      checkName: row.checkName || row.check_name || "unknown",
      timestamp: row.timestamp || "",
      statusCode: Number(row.statusCode ?? row.status_code ?? 0),
      latencyMs: Number(row.latencyMs ?? row.latency_ms ?? 0),
      error: row.error || ""
    }));
  } catch {
    return [];
  }
}
