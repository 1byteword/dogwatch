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
