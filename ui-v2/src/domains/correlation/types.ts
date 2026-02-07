export interface DeployContext {
  id: string;
  service: string;
  version: string;
  timestamp?: string;
}

export interface DeployIncidentCorrelation {
  id: string;
  confidence: number;
  reason: string;
  timeDeltaMs: number;
  deployment: DeployContext;
}

export interface CorrelationSummary {
  totalEvents: number;
  errorLogCount: number;
  traceCount: number;
  incidentCount: number;
  deployCount: number;
  alertCount: number;
}

export interface CorrelatedTimelineEvent {
  id: string;
  type: string;
  timestamp: string;
  summary: string;
  severity: string;
  service: string;
}

export interface ServiceTimeline {
  service: string;
  summary: CorrelationSummary;
  events: CorrelatedTimelineEvent[];
}
