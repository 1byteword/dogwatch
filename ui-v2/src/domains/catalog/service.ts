import { api } from "../../core/api";
import { CatalogFilters, CatalogService, CatalogStats } from "./types";

interface ApiCatalogService {
  id?: string;
  name?: string;
  display_name?: string;
  description?: string;
  tier?: string;
  health?: string;
  lifecycle?: string;
  team_name?: string;
  owner_email?: string;
  slack_channel?: string;
  repo_url?: string;
  docs_url?: string;
  runbook_url?: string;
  incident_count_30d?: number;
  uptime_percent_30d?: number;
  avg_response_time_ms?: number;
  updated_at?: string;
}

interface ApiCatalogStats {
  total?: number;
  critical?: number;
  high?: number;
  medium?: number;
  low?: number;
  healthy?: number;
  degraded?: number;
  unhealthy?: number;
}

function toTier(tier: string | undefined): CatalogService["tier"] {
  const value = (tier || "unknown").toLowerCase();
  if (value === "critical" || value === "high" || value === "medium" || value === "low") return value;
  return "unknown";
}

function toHealth(health: string | undefined): CatalogService["health"] {
  const value = (health || "unknown").toLowerCase();
  if (value === "healthy" || value === "degraded" || value === "unhealthy") return value;
  return "unknown";
}

function mapService(raw: ApiCatalogService, idx: number): CatalogService {
  const name = raw.name || `service-${idx}`;
  return {
    id: raw.id || name,
    name,
    displayName: raw.display_name || name,
    description: raw.description || "",
    tier: toTier(raw.tier),
    health: toHealth(raw.health),
    lifecycle: raw.lifecycle || "active",
    teamName: raw.team_name || "unassigned",
    ownerEmail: raw.owner_email || "",
    slackChannel: raw.slack_channel || "",
    repoUrl: raw.repo_url || "",
    docsUrl: raw.docs_url || "",
    runbookUrl: raw.runbook_url || "",
    incidentCount30d: typeof raw.incident_count_30d === "number" ? raw.incident_count_30d : 0,
    uptimePercent30d: typeof raw.uptime_percent_30d === "number" ? raw.uptime_percent_30d : 0,
    avgResponseTimeMs: typeof raw.avg_response_time_ms === "number" ? raw.avg_response_time_ms : 0,
    updatedAt: raw.updated_at
  };
}

export async function loadCatalogServices(filters: CatalogFilters): Promise<CatalogService[]> {
  const params = new URLSearchParams();
  if (filters.tier) params.set("tier", filters.tier);
  if (filters.health) params.set("health", filters.health);
  const q = params.toString();
  const raw = await api.get<unknown>(`/api/catalog/services${q ? `?${q}` : ""}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => mapService(item as ApiCatalogService, idx));
}

export async function loadCatalogStats(): Promise<CatalogStats> {
  const raw = await api.get<ApiCatalogStats>("/api/catalog/services/stats");
  return {
    total: raw.total || 0,
    critical: raw.critical || 0,
    high: raw.high || 0,
    medium: raw.medium || 0,
    low: raw.low || 0,
    healthy: raw.healthy || 0,
    degraded: raw.degraded || 0,
    unhealthy: raw.unhealthy || 0
  };
}
