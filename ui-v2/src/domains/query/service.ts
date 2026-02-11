import { api } from "../../core/api";
import { QueryMetadata, QueryResult, SavedQuery } from "./types";

export async function executeQuery(query: string, timeRange: string): Promise<QueryResult> {
  try {
    return await api.post<QueryResult>("/api/query/execute", { query, timeRange });
  } catch {
    return { rows: [], columns: [], count: 0, error: "Query execution failed" };
  }
}

export async function loadQueryMetadata(): Promise<QueryMetadata> {
  try {
    return await api.get<QueryMetadata>("/api/query/metadata");
  } catch {
    return { sources: [], functions: [] };
  }
}

export async function loadSavedQueries(): Promise<SavedQuery[]> {
  try {
    const raw = await api.get<Array<Partial<SavedQuery & { created_at?: string; updated_at?: string }>>>("/api/query/saved");
    return (raw || []).map((row, idx) => ({
      id: row.id || `sq-${idx}`,
      name: row.name || "unnamed",
      query: row.query || "",
      description: row.description || "",
      createdAt: row.createdAt || row.created_at || "",
      updatedAt: row.updatedAt || row.updated_at || "",
    }));
  } catch {
    return [];
  }
}
