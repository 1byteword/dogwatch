import { api } from "../../core/api";
import { OncallCurrent, OncallPolicy, OncallSchedule } from "./types";

interface ApiOncallSchedule {
  id?: string;
  name?: string;
  description?: string;
  timezone?: string;
  teams?: string[];
  layers?: Array<{ users?: Array<{ id?: string; name?: string; email?: string }> }>;
  rotations?: Array<{ users?: string[] }>;
}

interface ApiOncallPolicy {
  id?: string;
  name?: string;
  description?: string;
  rules?: unknown[];
  repeat_enabled?: boolean;
  repeat_after_minutes?: number;
}

interface ApiOncallCurrentResponse {
  on_call?: {
    user?: { id?: string; name?: string };
    start_time?: string;
    end_time?: string;
    is_override?: boolean;
  } | null;
}

function mapSchedule(raw: ApiOncallSchedule, idx: number): OncallSchedule {
  const layers = Array.isArray(raw.layers) ? raw.layers : [];
  const layerMembers = layers.reduce((count, layer) => count + (layer.users?.length || 0), 0);
  const rotationMembers = (raw.rotations || []).reduce((count, rot) => count + (rot.users?.length || 0), 0);

  return {
    id: raw.id || `schedule-${idx}`,
    name: raw.name || `Schedule ${idx + 1}`,
    description: raw.description || "",
    timezone: raw.timezone || "UTC",
    teams: raw.teams || [],
    layerCount: layers.length || (raw.rotations ? raw.rotations.length : 0),
    memberCount: layerMembers || rotationMembers
  };
}

function mapPolicy(raw: ApiOncallPolicy, idx: number): OncallPolicy {
  return {
    id: raw.id || `policy-${idx}`,
    name: raw.name || `Policy ${idx + 1}`,
    description: raw.description || "",
    ruleCount: Array.isArray(raw.rules) ? raw.rules.length : 0,
    repeatEnabled: Boolean(raw.repeat_enabled || (raw.repeat_after_minutes && raw.repeat_after_minutes > 0))
  };
}

export async function loadOncallSchedules(): Promise<OncallSchedule[]> {
  try {
    const raw = await api.get<unknown>("/api/oncall/schedules");
    if (!Array.isArray(raw)) return [];
    return raw.map((item, idx) => mapSchedule(item as ApiOncallSchedule, idx));
  } catch {
    const raw = await api.get<unknown>("/api/oncall");
    if (!Array.isArray(raw)) return [];
    return raw.map((item, idx) => mapSchedule(item as ApiOncallSchedule, idx));
  }
}

export async function loadOncallPolicies(): Promise<OncallPolicy[]> {
  try {
    const raw = await api.get<unknown>("/api/oncall/policies");
    if (!Array.isArray(raw)) return [];
    return raw.map((item, idx) => mapPolicy(item as ApiOncallPolicy, idx));
  } catch {
    const raw = await api.get<unknown>("/api/escalation");
    if (!Array.isArray(raw)) return [];
    return raw.map((item, idx) => mapPolicy(item as ApiOncallPolicy, idx));
  }
}

export async function loadCurrentOncall(scheduleId: string): Promise<OncallCurrent | null> {
  if (!scheduleId) return null;

  try {
    const raw = await api.get<ApiOncallCurrentResponse>(`/api/oncall/current/${encodeURIComponent(scheduleId)}`);
    const entry = raw.on_call;
    if (!entry || !entry.user) return null;
    return {
      userName: entry.user.name || entry.user.id || "on-call",
      userId: entry.user.id || "",
      startTime: entry.start_time,
      endTime: entry.end_time,
      isOverride: Boolean(entry.is_override)
    };
  } catch {
    const raw = await api.get<Record<string, string>>(`/api/oncall/current?schedule=${encodeURIComponent(scheduleId)}`);
    const userName = raw.user || "";
    if (!userName) return null;
    return {
      userName,
      userId: "",
      isOverride: false
    };
  }
}
