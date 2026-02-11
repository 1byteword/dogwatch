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

// --- Alert Rules (Monitors) ---

export type RuleType = "threshold" | "anomaly" | "change" | "absence" | "composite";
export type RuleSeverity = "critical" | "warning" | "info";

export interface AlertRule {
  id: string;
  name: string;
  description: string;
  type: RuleType;
  enabled: boolean;
  query: string;
  condition: string;
  threshold: number;
  severity: RuleSeverity;
  forDuration: string;
  notifyChannels: string[];
  labels: Record<string, string>;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
}
