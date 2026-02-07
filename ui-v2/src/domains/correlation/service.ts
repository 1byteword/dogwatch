import { api } from "../../core/api";
import { CorrelatedTimelineEvent, CorrelationSummary, DeployIncidentCorrelation, ServiceTimeline } from "./types";

interface ApiDeployCorrelation {
  confidence?: number;
  reason?: string;
  time_delta?: unknown;
  deployment?: {
    id?: string;
    service?: string;
    version?: string;
    timestamp?: string;
  };
}

interface ApiDeployCorrelationResponse {
  correlations?: ApiDeployCorrelation[];
}

interface ApiServiceTimeline {
  service?: string;
  events?: Array<{
    id?: string;
    type?: string;
    timestamp?: string;
    summary?: string;
    message?: string;
    severity?: string;
    level?: string;
    service?: string;
  }>;
  summary?: {
    total_events?: number;
    error_log_count?: number;
    trace_count?: number;
    incident_count?: number;
    deploy_count?: number;
    alert_count?: number;
  };
}

function parseDurationMs(raw: unknown): number {
  if (typeof raw === "number") return Math.max(0, Math.round(raw / 1_000_000));
  if (typeof raw !== "string" || !raw.trim()) return 0;

  const match = raw.trim().match(/^([\d.]+)\s*(ms|s|m|h)$/i);
  if (!match) return 0;

  const value = Number(match[1]);
  if (!Number.isFinite(value)) return 0;
  const unit = match[2].toLowerCase();
  if (unit === "ms") return Math.round(value);
  if (unit === "s") return Math.round(value * 1_000);
  if (unit === "m") return Math.round(value * 60_000);
  return Math.round(value * 3_600_000);
}

export function formatDuration(ms: number): string {
  if (ms < 1_000) return `${ms}ms`;
  if (ms < 60_000) return `${Math.round(ms / 1_000)}s`;
  if (ms < 3_600_000) return `${Math.round(ms / 60_000)}m`;
  return `${Math.round(ms / 3_600_000)}h`;
}

function mapCorrelation(raw: ApiDeployCorrelation, idx: number): DeployIncidentCorrelation {
  const deployment = raw.deployment || {};
  return {
    id: deployment.id || `deploy-corr-${idx}`,
    confidence: typeof raw.confidence === "number" ? raw.confidence : 0,
    reason: raw.reason || "Correlated by service and timing",
    timeDeltaMs: parseDurationMs(raw.time_delta),
    deployment: {
      id: deployment.id || `deploy-${idx}`,
      service: deployment.service || "unknown-service",
      version: deployment.version || "unknown",
      timestamp: deployment.timestamp
    }
  };
}

function emptySummary(): CorrelationSummary {
  return {
    totalEvents: 0,
    errorLogCount: 0,
    traceCount: 0,
    incidentCount: 0,
    deployCount: 0,
    alertCount: 0
  };
}

function mapTimelineEvent(raw: NonNullable<ApiServiceTimeline["events"]>[number], idx: number): CorrelatedTimelineEvent {
  return {
    id: raw.id || `timeline-event-${idx}`,
    type: (raw.type || "event").toLowerCase(),
    timestamp: raw.timestamp || new Date().toISOString(),
    summary: raw.summary || raw.message || "event",
    severity: (raw.severity || raw.level || "info").toLowerCase(),
    service: raw.service || ""
  };
}

export async function loadDeployIncidentCorrelations(since: string): Promise<DeployIncidentCorrelation[]> {
  const raw = await api.get<ApiDeployCorrelationResponse>(`/api/correlate/deploy-incidents?since=${encodeURIComponent(since)}`);
  const correlations = raw.correlations || [];
  return correlations.map(mapCorrelation);
}

export async function loadServiceTimeline(service: string, since: string): Promise<ServiceTimeline> {
  if (!service) {
    return {
      service: "",
      summary: emptySummary(),
      events: []
    };
  }

  const raw = await api.get<ApiServiceTimeline>(
    `/api/correlate/service/${encodeURIComponent(service)}/timeline?since=${encodeURIComponent(since)}`
  );

  return {
    service: raw.service || service,
    summary: {
      totalEvents: raw.summary?.total_events || 0,
      errorLogCount: raw.summary?.error_log_count || 0,
      traceCount: raw.summary?.trace_count || 0,
      incidentCount: raw.summary?.incident_count || 0,
      deployCount: raw.summary?.deploy_count || 0,
      alertCount: raw.summary?.alert_count || 0
    },
    events: (raw.events || []).map(mapTimelineEvent)
  };
}
