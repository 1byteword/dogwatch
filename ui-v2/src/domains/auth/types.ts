export interface User {
  id: string;
  email: string;
  name: string;
  role: "owner" | "admin" | "editor" | "viewer";
  isActive: boolean;
  avatarUrl?: string;
  timezone?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  expires_at: string;
  user: User;
}

export interface MeResponse {
  user: User;
  org?: string;
}
