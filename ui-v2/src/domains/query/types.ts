export interface QueryResult {
  rows: Record<string, unknown>[];
  columns: string[];
  count: number;
  error?: string;
}

export interface SavedQuery {
  id: string;
  name: string;
  query: string;
  description: string;
  createdAt: string;
  updatedAt: string;
}

export interface QueryMetadata {
  sources: string[];
  functions: string[];
}
