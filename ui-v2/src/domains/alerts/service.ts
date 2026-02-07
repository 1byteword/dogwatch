import { api } from "../../core/api";
import { relativeTime } from "../../core/time";
import { mockAlerts } from "./mock";
import { AlertItem } from "./types";

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
