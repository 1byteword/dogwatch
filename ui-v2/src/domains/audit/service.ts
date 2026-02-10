import { api } from "../../core/api";
import { ApiKeyInfo, AuditLogRow, AuditLogsPage, AuditSummary, BackupInfo } from "./types";

function parseErrorMessage(status: number): string {
  if (status === 403) return "Audit API requires admin/owner access.";
  if (status === 503) return "Audit service is not configured.";
  return `Audit API error (${status}).`;
}

export async function loadAuditSummary(period: string): Promise<AuditSummary> {
  const res = await fetch(`/api/audit/summary?period=${encodeURIComponent(period)}`);
  if (!res.ok) {
    throw new Error(parseErrorMessage(res.status));
  }

  const raw = (await res.json()) as Record<string, unknown>;
  return {
    period: String(raw.period || period),
    totalQueries: Number(raw.total_queries || 0),
    failedQueries: Number(raw.failed_queries || 0),
    totalLogins: Number(raw.total_logins || 0),
    failedLogins: Number(raw.failed_logins || 0),
    totalAdminActions: Number(raw.total_admin_actions || 0),
    totalExports: Number(raw.total_exports || 0)
  };
}

export async function loadAuditLogs(limit: number, offset: number): Promise<AuditLogsPage> {
  const params = new URLSearchParams();
  params.set("limit", String(limit));
  params.set("offset", String(offset));

  const res = await fetch(`/api/audit/logs/paginated?${params.toString()}`);
  if (!res.ok) {
    throw new Error(parseErrorMessage(res.status));
  }

  const raw = (await res.json()) as {
    logs?: Array<Record<string, unknown>>;
    total?: number;
    limit?: number;
    offset?: number;
    has_more?: boolean;
  };

  return {
    logs: (raw.logs || []).map((item, idx) => ({
      id: String(item.id || `audit-${offset + idx}`),
      timestamp: String(item.timestamp || new Date().toISOString()),
      userId: String(item.user_id || ""),
      userEmail: String(item.user_email || ""),
      action: String(item.action || ""),
      resourceType: String(item.resource_type || ""),
      outcome: String(item.outcome || ""),
      resourceName: String(item.resource_name || "")
    })),
    total: Number(raw.total || 0),
    limit: Number(raw.limit || limit),
    offset: Number(raw.offset || offset),
    hasMore: Boolean(raw.has_more)
  };
}

export async function loadRecentAuditLogs(): Promise<AuditLogRow[]> {
  try {
    const page = await loadAuditLogs(20, 0);
    return page.logs;
  } catch {
    return [];
  }
}

export async function loadApiKeys(): Promise<ApiKeyInfo[]> {
  try {
    const raw = await api.get<Array<Partial<ApiKeyInfo & { created_at?: string; last_used?: string }>>>("/api/apikeys");
    return (raw || []).map((row, idx) => ({
      id: row.id || `key-${idx}`,
      name: row.name || "unnamed",
      prefix: row.prefix || "dw_***",
      createdAt: row.createdAt || row.created_at || "",
      lastUsed: row.lastUsed || row.last_used || "",
      role: row.role || "viewer"
    }));
  } catch {
    return [];
  }
}

export async function loadBackups(): Promise<BackupInfo[]> {
  try {
    const raw = await api.get<Array<Partial<BackupInfo & { created_at?: string }>>>("/api/backup/list");
    return (raw || []).map((row, idx) => ({
      id: row.id || `backup-${idx}`,
      filename: row.filename || "",
      size: Number(row.size || 0),
      createdAt: row.createdAt || row.created_at || "",
      status: toBackupStatus(row.status)
    }));
  } catch {
    return [];
  }
}

function toBackupStatus(s: string | undefined): BackupInfo["status"] {
  const v = (s || "").toLowerCase();
  if (v === "completed" || v === "failed" || v === "running" || v === "scheduled") return v;
  return "completed";
}
