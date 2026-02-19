import type { LoginResponse, MeResponse } from "./types";

const API_BASE = import.meta.env.VITE_API_BASE ?? "";

export async function login(email: string, password: string): Promise<LoginResponse> {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "same-origin",
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const body = await res.text().catch(() => "");
    throw new Error(body || `Login failed (${res.status})`);
  }
  return res.json();
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/api/auth/logout`, {
    method: "POST",
    credentials: "same-origin",
  });
}

export async function getMe(): Promise<MeResponse> {
  const res = await fetch(`${API_BASE}/api/auth/me`, {
    credentials: "same-origin",
  });
  if (!res.ok) {
    // In dev mode without backend, return a mock user so the UI is accessible
    if (import.meta.env.DEV) {
      return { user: { id: "dev", email: "dev@localhost", name: "Dev User", role: "owner" } };
    }
    throw new Error(`Not authenticated (${res.status})`);
  }
  return res.json();
}
