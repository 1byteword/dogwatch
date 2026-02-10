export type AlertSeverity = "critical" | "high" | "medium" | "low";
export type AlertState = "firing" | "pending";

export interface AlertItem {
  id: string;
  name: string;
  service: string;
  severity: AlertSeverity;
  state: AlertState;
  trigger: string;
  startedAt: string;
  startedAtRaw?: string;
  probableCause: string;
  recentDeploy: string;
  traceErrors: number;
}

export interface WatchRule {
  id: string;
  name: string;
  query: string;
  condition: string;
  enabled: boolean;
  lastEvaluated: string;
  service: string;
}

export interface AlertSilence {
  id: string;
  matchers: string;
  createdBy: string;
  startsAt: string;
  endsAt: string;
  comment: string;
}
