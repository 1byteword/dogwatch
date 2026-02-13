import { getMockResponse } from "./mock-data";

const API_BASE = import.meta.env.VITE_API_BASE ?? "";

let _onUnauthorized: (() => void) | null = null;

export function onUnauthorized(cb: () => void) {
  _onUnauthorized = cb;
}

async function request<T>(path: string): Promise<T> {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      credentials: "same-origin",
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as T;
  } catch (err) {
    const mock = getMockResponse(path);
    if (mock !== undefined) return mock as T;
    // Only signal 401 when no mock fallback recovered the call
    if (err instanceof Error && err.message === "HTTP 401" && _onUnauthorized) {
      _onUnauthorized();
    }
    throw err;
  }
}

async function postRequest<T>(path: string, body: unknown): Promise<T> {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return (await res.json()) as T;
  } catch (err) {
    const mock = getMockResponse(path);
    if (mock !== undefined) return mock as T;
    if (err instanceof Error && err.message === "HTTP 401" && _onUnauthorized) {
      _onUnauthorized();
    }
    throw err;
  }
}

export const api = {
  get: request,
  post: postRequest,
};
