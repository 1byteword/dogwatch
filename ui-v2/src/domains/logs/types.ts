export interface LogEntry {
  id: string;
  timestamp: string;
  level: string;
  service?: string;
  message: string;
}

export interface LogQuery {
  q?: string;
  level?: string;
  service?: string;
  since?: string;
  limit?: number;
}

export interface LogPattern {
  id: string;
  pattern: string;
  count: number;
  level: string;
  services: string[];
  sample: string;
}

export interface TrendingPattern {
  id: string;
  pattern: string;
  count: number;
  growthPercent: number;
  level: string;
  firstSeen: string;
}

export interface LogComparison {
  beforeEntries: LogEntry[];
  afterEntries: LogEntry[];
  addedPatterns: string[];
  removedPatterns: string[];
}
