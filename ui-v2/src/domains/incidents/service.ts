import { api } from "../../core/api";
import { relativeTime } from "../../core/time";
import { mockIncidents } from "./mock";
import { IncidentItem, IncidentTimelineEvent } from "./types";

interface ApiIncident {
  id?: string;
  title?: string;
  severity?: string;
  status?: string;
  service?: string;
  created_at?: string;
  assigned_to?: string;
  timeline?: Array<{ timestamp?: string; type?: string; message?: string }>;
}

interface ApiIncidentListResponse {
  data?: ApiIncident[];
}

function mapIncident(raw: ApiIncident, idx: number): IncidentItem {
  const severity = (raw.severity || "medium").toLowerCase();
  const status = (raw.status || "triggered").toLowerCase();
  const timeline: IncidentTimelineEvent[] = (raw.timeline || []).slice(0, 8).map((evt, i) => {
    let kind: IncidentTimelineEvent["kind"] = "note";
    if (evt.type === "acknowledged" || evt.type === "resolved") kind = "status";
    else if (evt.type === "deploy") kind = "deploy";
    else if (evt.type === "created") kind = "alert";

    return {
      id: `${raw.id || idx}-evt-${i}`,
      time: evt.timestamp ? relativeTime(evt.timestamp) : "now",
      kind,
      summary: evt.message || evt.type || "event"
    };
  });

  return {
    id: raw.id || `api-incident-${idx}`,
    title: raw.title || "Incident",
    severity: severity === "critical" || severity === "high" ? severity : "medium",
    status: status === "acknowledged" ? "acknowledged" : "triggered",
    service: raw.service || "unknown-service",
    commander: raw.assigned_to || "unassigned",
    responders: raw.assigned_to ? [raw.assigned_to] : ["unassigned"],
    startedAt: raw.created_at ? relativeTime(raw.created_at) : "now",
    startedAtRaw: raw.created_at,
    timeline:
      timeline.length > 0
        ? timeline
        : [{ id: `${raw.id || idx}-fallback`, time: "now", kind: "note", summary: "No timeline yet" }]
  };
}

export async function loadIncidents(): Promise<IncidentItem[]> {
  try {
    const raw = await api.get<ApiIncident[] | ApiIncidentListResponse>("/api/incidents?status=all&limit=20");
    const list = Array.isArray(raw) ? raw : Array.isArray(raw.data) ? raw.data : [];
    if (list.length === 0) return mockIncidents;
    return list.map(mapIncident);
  } catch {
    return mockIncidents;
  }
}
