import { api } from "../../core/api";
import {
  ConnectionStat,
  EndpointStat,
  ProcessInfo,
  ServiceMapData,
  StatsSummary,
  SystemMetricPoint,
  SystemMetrics,
  TraceDependency,
  TraceSummary
} from "./types";

export async function loadSystemMetrics(): Promise<SystemMetrics | null> {
  try {
    const raw = await api.get<Partial<SystemMetrics>>("/api/system");
    return {
      timestamp: raw.timestamp,
      cpu_usage_percent: Number(raw.cpu_usage_percent || 0),
      mem_usage_percent: Number(raw.mem_usage_percent || 0),
      disk_read_per_sec: Number(raw.disk_read_per_sec || 0),
      disk_write_per_sec: Number(raw.disk_write_per_sec || 0),
      net_rx_per_sec: Number(raw.net_rx_per_sec || 0),
      net_tx_per_sec: Number(raw.net_tx_per_sec || 0),
      load_1: Number(raw.load_1 || 0),
      load_5: Number(raw.load_5 || 0),
      load_15: Number(raw.load_15 || 0)
    };
  } catch {
    return null;
  }
}

type RawStats = {
  total_connections?: number;
  total_requests?: number;
  total_errors?: number;
  endpoints?: EndpointStat[];
  connections?: ConnectionStat[];
};

export async function loadStatsSummary(): Promise<StatsSummary> {
  try {
    const raw = await api.get<RawStats>("/api/stats");
    return {
      total_connections: Number(raw.total_connections || 0),
      total_requests: Number(raw.total_requests || 0),
      total_errors: Number(raw.total_errors || 0),
      endpoints: (raw.endpoints || []).map((row) => ({
        method: row.method || "GET",
        path: row.path || "/",
        request_count: Number(row.request_count || 0),
        error_count: Number(row.error_count || 0),
        error_rate: Number(row.error_rate || 0),
        p99_ms: Number(row.p99_ms || 0),
        avg_ms: Number(row.avg_ms || 0)
      })),
      connections: (raw.connections || []).map((row) => ({
        process: row.process || "unknown",
        pid: Number(row.pid || 0),
        remote: row.remote || "-",
        port: Number(row.port || 0),
        count: Number(row.count || 0)
      }))
    };
  } catch {
    return { total_connections: 0, total_requests: 0, total_errors: 0, endpoints: [], connections: [] };
  }
}

export async function loadTopProcesses(): Promise<ProcessInfo[]> {
  try {
    const raw = await api.get<Array<Partial<ProcessInfo>>>("/api/processes");
    return (raw || []).map((row) => ({
      pid: Number(row.pid || 0),
      name: row.name || "unknown",
      cpu_pct: Number(row.cpu_pct || 0),
      mem_mb: Number(row.mem_mb || 0),
      state: row.state || "-",
      threads: Number(row.threads || 0)
    }));
  } catch {
    return [];
  }
}

export async function loadServiceMap(): Promise<ServiceMapData> {
  try {
    const raw = await api.get<Partial<ServiceMapData>>("/api/servicemap");
    return {
      nodes: (raw.nodes || []).map((node) => ({ id: node.id || "", name: node.name || "", type: node.type || "" })).filter((n) => n.id),
      links: (raw.links || [])
        .map((link) => ({ source: link.source || "", target: link.target || "", count: Number(link.count || 0) }))
        .filter((l) => l.source && l.target)
    };
  } catch {
    return { nodes: [], links: [] };
  }
}

type TraceListResponse = {
  data?: TraceSummary[];
};

export async function loadTraceSummaries(limit = 50, service = "", duration = "1h"): Promise<TraceSummary[]> {
  try {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    params.set("duration", duration);
    if (service) params.set("service", service);
    const raw = await api.get<TraceListResponse>(`/api/traces?${params.toString()}`);
    return (raw.data || []).map((row) => ({
      trace_id: row.trace_id || "",
      service_name: row.service_name || "",
      name: row.name || "trace",
      duration_ms: Number(row.duration_ms || 0),
      span_count: Number(row.span_count || 0),
      status: row.status || "UNSET"
    }));
  } catch {
    return [];
  }
}

export async function loadTraceServices(): Promise<string[]> {
  try {
    const raw = await api.get<string[]>("/api/trace/services");
    return Array.isArray(raw) ? raw.filter(Boolean) : [];
  } catch {
    return [];
  }
}

export async function loadSystemHistory(duration = "1h"): Promise<SystemMetricPoint[]> {
  try {
    const raw = await api.get<Array<Partial<SystemMetricPoint>>>(`/api/history/system?duration=${duration}`);
    return (raw || []).map((row) => ({
      timestamp: row.timestamp || "",
      cpu_percent: Number(row.cpu_percent || 0),
      mem_percent: Number(row.mem_percent || 0),
      load_1: Number(row.load_1 || 0),
      disk_read_bytes: Number(row.disk_read_bytes || 0),
      disk_write_bytes: Number(row.disk_write_bytes || 0),
      net_rx_bytes: Number(row.net_rx_bytes || 0),
      net_tx_bytes: Number(row.net_tx_bytes || 0),
    }));
  } catch {
    return [];
  }
}

export async function loadTraceDependencies(): Promise<TraceDependency[]> {
  try {
    const raw = await api.get<Array<Partial<TraceDependency>>>("/api/trace/dependencies");
    return (raw || []).map((row) => ({
      parent: row.parent || "unknown",
      child: row.child || "unknown",
      call_count: Number(row.call_count || 0)
    }));
  } catch {
    return [];
  }
}
