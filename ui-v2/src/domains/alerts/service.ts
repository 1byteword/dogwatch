import { api } from "../../core/api";
import { relativeTime } from "../../core/time";
import { mockAlerts } from "./mock";
import { AlertItem, AlertSilence, WatchRule } from "./types";

interface AlertingApiAlert {
  id?: string;
  rule_name?: string;
  severity?: string;
  state?: string;
  starts_at?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  value?: number;
}

function mapAlert(raw: AlertingApiAlert, idx: number): AlertItem {
  const severity = (raw.severity || "medium").toLowerCase();
  const state = (raw.state || "pending").toLowerCase();

  return {
    id: raw.id || `api-alert-${idx}`,
    name: raw.rule_name || raw.annotations?.summary || "Alert",
    service: raw.labels?.service || raw.labels?.job || "unknown-service",
    severity: severity === "critical" || severity === "high" || severity === "low" ? severity : "medium",
    state: state === "firing" ? "firing" : "pending",
    trigger: raw.annotations?.description || raw.annotations?.summary || "Threshold condition met",
    startedAt: raw.starts_at ? relativeTime(raw.starts_at) : "now",
    startedAtRaw: raw.starts_at,
    probableCause: raw.annotations?.root_cause || "Correlation details loading",
    recentDeploy: raw.labels?.deploy || "unknown",
    traceErrors: typeof raw.value === "number" ? Math.round(raw.value) : 0
  };
}

export async function loadAlerts(): Promise<AlertItem[]> {
  try {
    const raw = await api.get<AlertingApiAlert[]>("/api/alerting/alerts?state=firing");
    if (!Array.isArray(raw) || raw.length === 0) return mockAlerts;
    return raw.map(mapAlert);
  } catch {
    return mockAlerts;
  }
}

export async function loadWatches(): Promise<WatchRule[]> {
  try {
    const raw = await api.get<Array<Partial<WatchRule & { last_evaluated?: string }>>>("/api/watches");
    return (raw || []).map((row, idx) => ({
      id: row.id || `watch-${idx}`,
      name: row.name || "unnamed",
      query: row.query || "",
      condition: row.condition || "",
      enabled: row.enabled !== false,
      lastEvaluated: row.lastEvaluated || row.last_evaluated || "",
      service: row.service || ""
    }));
  } catch {
    return [];
  }
}

export async function loadSilences(): Promise<AlertSilence[]> {
  try {
    const raw = await api.get<Array<Partial<AlertSilence & { created_by?: string; starts_at?: string; ends_at?: string }>>>("/api/alerting/silences");
    return (raw || []).map((row, idx) => ({
      id: row.id || `silence-${idx}`,
      matchers: row.matchers || "",
      createdBy: row.createdBy || row.created_by || "",
      startsAt: row.startsAt || row.starts_at || "",
      endsAt: row.endsAt || row.ends_at || "",
      comment: row.comment || ""
    }));
  } catch {
    return [];
  }
}
