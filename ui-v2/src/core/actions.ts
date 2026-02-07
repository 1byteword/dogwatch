import { api } from "./api";

export async function ackAlert(id: string): Promise<void> {
  await fetch(`/api/alerting/alerts/${encodeURIComponent(id)}/acknowledge`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: "v2-ui" })
  }).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
  });
}

export async function silenceAlert(id: string, duration = "30m"): Promise<void> {
  await fetch(`/api/alerting/alerts/${encodeURIComponent(id)}/silence`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      duration,
      created_by: "v2-ui",
      comment: "Silenced from V2 UI"
    })
  }).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
  });
}

export async function ackIncident(id: string, user = "v2-ui"): Promise<void> {
  await fetch(`/api/incidents/${encodeURIComponent(id)}/ack`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user })
  }).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
  });
}

export async function resolveIncident(id: string): Promise<void> {
  await fetch(`/api/incidents/${encodeURIComponent(id)}/resolve`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user: "v2-ui", resolution: "Resolved from V2 UI" })
  }).then((r) => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
  });
}

export async function createIncident(payload: {
  title: string;
  severity: "critical" | "high" | "medium" | "low";
  service?: string;
  description?: string;
}): Promise<{ id?: string }> {
  const res = await fetch("/api/incidents", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      title: payload.title,
      severity: payload.severity,
      service: payload.service || "",
      description: payload.description || "",
      source: "v2-ui"
    })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as { id?: string };
}

export async function pingStatus(): Promise<boolean> {
  try {
    await api.get("/api/ping");
    return true;
  } catch {
    return false;
  }
}

export async function addIncidentNote(id: string, note: string, user = "v2-ui"): Promise<void> {
  const res = await fetch(`/api/incidents/${encodeURIComponent(id)}/note`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user, note })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function assignIncident(id: string, assignee: string, user = "v2-ui"): Promise<void> {
  const res = await fetch(`/api/incidents/${encodeURIComponent(id)}/assign`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user, assignee })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function createAlertRule(payload: {
  name: string;
  description?: string;
  service?: string;
  severity?: "critical" | "warning" | "info";
  metric?: string;
  condition?: "gt" | "lt" | "gte" | "lte" | "eq" | "neq";
  threshold?: number;
}): Promise<{ id?: string }> {
  const res = await fetch("/api/alerting/rules", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: payload.name,
      description: payload.description || "",
      type: "threshold",
      enabled: true,
      labels: payload.service ? { service: payload.service, severity: payload.severity || "warning" } : {},
      annotations: { summary: payload.description || payload.name },
      metric: payload.metric || "error_rate",
      condition: payload.condition || "gt",
      threshold: payload.threshold ?? 1,
      eval_interval: "30s",
      for_duration: "2m",
      created_by: "v2-ui"
    })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as { id?: string };
}

export async function createCatalogService(payload: {
  name: string;
  displayName?: string;
  description?: string;
  tier?: "critical" | "high" | "medium" | "low";
  ownerEmail?: string;
  teamName?: string;
  repoUrl?: string;
  docsUrl?: string;
  runbookUrl?: string;
}): Promise<{ id?: string }> {
  const res = await fetch("/api/catalog/services", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: payload.name,
      display_name: payload.displayName || payload.name,
      description: payload.description || "",
      tier: payload.tier || "medium",
      lifecycle: "active",
      health: "unknown",
      owner_email: payload.ownerEmail || "",
      team_name: payload.teamName || "",
      repo_url: payload.repoUrl || "",
      docs_url: payload.docsUrl || "",
      runbook_url: payload.runbookUrl || ""
    })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as { id?: string };
}

export async function createOncallSchedule(payload: {
  name: string;
  timezone: string;
  responderName: string;
  responderEmail?: string;
}): Promise<{ id?: string }> {
  const res = await fetch("/api/oncall/schedules", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: payload.name,
      description: "Created from V2 UI",
      timezone: payload.timezone,
      teams: [],
      layers: [
        {
          id: "primary",
          name: "Primary",
          priority: 1,
          rotation_type: "daily",
          handoff_time: "09:00",
          handoff_day: 1,
          shift_duration: "24h",
          start_date: new Date().toISOString(),
          users: [
            {
              id: payload.responderName.toLowerCase().replace(/\s+/g, "-"),
              name: payload.responderName,
              email: payload.responderEmail || ""
            }
          ]
        }
      ]
    })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as { id?: string };
}

export async function createNotifyChannel(payload: {
  name: string;
  type: "webhook" | "slack";
  target: string;
  enabled?: boolean;
}): Promise<{ id?: string }> {
  const config =
    payload.type === "webhook"
      ? { url: payload.target, method: "POST", timeout: 10 }
      : { webhook_url: payload.target };

  const res = await fetch("/api/notify/channels", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: payload.name,
      type: payload.type,
      config,
      enabled: payload.enabled ?? true
    })
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as { id?: string };
}

export async function testNotifyChannel(id: string): Promise<void> {
  const res = await fetch(`/api/notify/channels/${encodeURIComponent(id)}/test`, { method: "POST" });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
}
