export type ServiceTier = "critical" | "high" | "medium" | "low" | "unknown";
export type ServiceHealth = "healthy" | "degraded" | "unhealthy" | "unknown";

export interface CatalogService {
  id: string;
  name: string;
  displayName: string;
  description: string;
  tier: ServiceTier;
  health: ServiceHealth;
  lifecycle: string;
  teamName: string;
  ownerEmail: string;
  slackChannel: string;
  repoUrl: string;
  docsUrl: string;
  runbookUrl: string;
  incidentCount30d: number;
  uptimePercent30d: number;
  avgResponseTimeMs: number;
  updatedAt?: string;
}

export interface CatalogStats {
  total: number;
  critical: number;
  high: number;
  medium: number;
  low: number;
  healthy: number;
  degraded: number;
  unhealthy: number;
}

export interface CatalogFilters {
  tier?: string;
  health?: string;
}
