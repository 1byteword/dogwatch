export interface SystemMetrics {
  timestamp?: string;
  cpu_usage_percent: number;
  mem_usage_percent: number;
  disk_read_per_sec: number;
  disk_write_per_sec: number;
  net_rx_per_sec: number;
  net_tx_per_sec: number;
  load_1: number;
  load_5: number;
  load_15: number;
}

export interface EndpointStat {
  method: string;
  path: string;
  request_count: number;
  error_count: number;
  error_rate: number;
  p99_ms: number;
  avg_ms: number;
}

export interface ConnectionStat {
  process: string;
  pid: number;
  remote: string;
  port: number;
  count: number;
}

export interface StatsSummary {
  total_connections: number;
  total_requests: number;
  total_errors: number;
  endpoints: EndpointStat[];
  connections: ConnectionStat[];
}

export interface ProcessInfo {
  pid: number;
  name: string;
  cpu_pct: number;
  mem_mb: number;
  state: string;
  threads: number;
}

export interface ServiceMapNode {
  id: string;
  name: string;
  type: string;
}

export interface ServiceMapLink {
  source: string;
  target: string;
  count: number;
}

export interface ServiceMapData {
  nodes: ServiceMapNode[];
  links: ServiceMapLink[];
}

export interface TraceSummary {
  trace_id: string;
  service_name: string;
  name: string;
  duration_ms: number;
  span_count: number;
  status: string;
}

export interface TraceDependency {
  parent: string;
  child: string;
  call_count: number;
}

export interface SystemMetricPoint {
  timestamp: string;
  cpu_percent: number;
  mem_percent: number;
  load_1?: number;
  disk_read_bytes?: number;
  disk_write_bytes?: number;
  net_rx_bytes?: number;
  net_tx_bytes?: number;
}

export interface TraceSpan {
  span_id: string;
  parent_span_id: string;
  operation_name: string;
  service_name: string;
  duration_ms: number;
  status: string;
  depth: number;
}
