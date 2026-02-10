export interface AuditSummary {
  period: string;
  totalQueries: number;
  failedQueries: number;
  totalLogins: number;
  failedLogins: number;
  totalAdminActions: number;
  totalExports: number;
}

export interface AuditLogRow {
  id: string;
  timestamp: string;
  userId: string;
  userEmail: string;
  action: string;
  resourceType: string;
  outcome: string;
  resourceName: string;
}

export interface AuditLogsPage {
  logs: AuditLogRow[];
  total: number;
  offset: number;
  limit: number;
  hasMore: boolean;
}

export interface ApiKeyInfo {
  id: string;
  name: string;
  prefix: string;
  createdAt: string;
  lastUsed: string;
  role: string;
}

export interface BackupInfo {
  id: string;
  filename: string;
  size: number;
  createdAt: string;
  status: "completed" | "failed" | "running" | "scheduled";
}
