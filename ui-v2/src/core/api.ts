import { getMockResponse } from "./mock-data";

const API_BASE = import.meta.env.VITE_API_BASE ?? "";

async function request<T>(path: string): Promise<T> {
  try {
    const res = await fetch(`${API_BASE}${path}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as T;
  } catch {
    const mock = getMockResponse(path);
    if (mock !== undefined) return mock as T;
    throw new Error(`No backend and no mock data for ${path}`);
  }
}

async function postRequest<T>(path: string, body: unknown): Promise<T> {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as T;
  } catch {
    const mock = getMockResponse(path);
    if (mock !== undefined) return mock as T;
    throw new Error(`No backend and no mock data for ${path}`);
  }
}

export const api = {
  get: request,
  post: postRequest,
};
