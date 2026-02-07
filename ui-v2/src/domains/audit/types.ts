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
