export interface Deploy {
  id: string;
  service: string;
  version: string;
  environment: string;
  status: "success" | "failed" | "rolling" | "unknown";
  deployedAt: string;
  deployedBy: string;
}

export interface DeployStats {
  totalDeploys: number;
  successCount: number;
  failedCount: number;
  rollbackCount: number;
  avgFrequencyPerDay: number;
}
