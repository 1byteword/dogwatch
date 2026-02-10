export interface SloDefinition {
  id: string;
  name: string;
  service: string;
  target: number;
  current: number;
  budgetRemaining: number;
  burnRate: number;
  window: string;
  status: "met" | "at_risk" | "breached";
}

export interface SyntheticCheck {
  id: string;
  name: string;
  url: string;
  type: string;
  interval: number;
  status: "passing" | "failing" | "degraded";
  lastRun: string;
  uptimePercent: number;
  avgLatencyMs: number;
}

export interface SyntheticFailure {
  checkId: string;
  checkName: string;
  timestamp: string;
  statusCode: number;
  latencyMs: number;
  error: string;
}
