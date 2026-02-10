import { api } from "../../core/api";
import { Deploy, DeployStats } from "./types";

export async function loadDeploys(): Promise<Deploy[]> {
  try {
    const raw = await api.get<Array<Partial<Deploy & { deployed_at?: string; deployed_by?: string }>>>("/api/deploys");
    return (raw || []).map((row, idx) => ({
      id: row.id || `deploy-${idx}`,
      service: row.service || "unknown",
      version: row.version || "unknown",
      environment: row.environment || "production",
      status: toDeployStatus(row.status),
      deployedAt: row.deployedAt || row.deployed_at || "",
      deployedBy: row.deployedBy || row.deployed_by || ""
    }));
  } catch {
    return [];
  }
}

function toDeployStatus(s: string | undefined): Deploy["status"] {
  const v = (s || "").toLowerCase();
  if (v === "success" || v === "failed" || v === "rolling") return v;
  return "unknown";
}

export async function loadDeployStats(): Promise<DeployStats | null> {
  try {
    const raw = await api.get<Partial<DeployStats & {
      total_deploys?: number;
      success_count?: number;
      failed_count?: number;
      rollback_count?: number;
      avg_frequency_per_day?: number;
    }>>("/api/deploys/stats");
    return {
      totalDeploys: Number(raw.totalDeploys ?? raw.total_deploys ?? 0),
      successCount: Number(raw.successCount ?? raw.success_count ?? 0),
      failedCount: Number(raw.failedCount ?? raw.failed_count ?? 0),
      rollbackCount: Number(raw.rollbackCount ?? raw.rollback_count ?? 0),
      avgFrequencyPerDay: Number(raw.avgFrequencyPerDay ?? raw.avg_frequency_per_day ?? 0)
    };
  } catch {
    return null;
  }
}
