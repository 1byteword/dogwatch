export interface Anomaly {
  id: string;
  metric: string;
  service: string;
  severity: "critical" | "high" | "medium" | "low";
  detectedAt: string;
  description: string;
  score: number;
}

export interface DbQuery {
  id: string;
  query: string;
  database: string;
  avgMs: number;
  maxMs: number;
  callCount: number;
  errorCount: number;
}

export interface FlamegraphHotspot {
  function: string;
  module: string;
  selfPercent: number;
  totalPercent: number;
  samples: number;
}

export interface SlowQuery {
  id: string;
  query: string;
  database: string;
  avgMs: number;
  maxMs: number;
  callCount: number;
}
