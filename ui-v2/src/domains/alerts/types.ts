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
