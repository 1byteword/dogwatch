export interface IncidentTimelineEvent {
  id: string;
  time: string;
  kind: "alert" | "deploy" | "note" | "status";
  summary: string;
}

export interface IncidentItem {
  id: string;
  title: string;
  severity: "critical" | "high" | "medium";
  status: "triggered" | "acknowledged";
  service: string;
  commander: string;
  responders: string[];
  startedAt: string;
  startedAtRaw?: string;
  timeline: IncidentTimelineEvent[];
}
