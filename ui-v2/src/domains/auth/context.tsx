import { createContext, createSignal, useContext, onMount, ParentProps } from "solid-js";
import { useNavigate } from "@solidjs/router";
import type { User } from "./types";
import * as authService from "./service";
import { onUnauthorized } from "../../core/api";

interface AuthContextValue {
  user: () => User | null;
  isAuthenticated: () => boolean;
  loading: () => boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue>();

export function AuthProvider(props: ParentProps) {
  const [user, setUser] = createSignal<User | null>(null);
  const [loading, setLoading] = createSignal(true);
  const navigate = useNavigate();

  const isAuthenticated = () => user() !== null;

  async function login(email: string, password: string) {
    const res = await authService.login(email, password);
    setUser(res.user);
    navigate("/app/dashboards");
  }

  async function logout() {
    await authService.logout().catch(() => {});
    setUser(null);
    navigate("/login");
  }

  onMount(async () => {
    try {
      const me = await authService.getMe();
      setUser(me.user);
    } catch {
      // Not authenticated — stay on current page, guard will redirect
    } finally {
      setLoading(false);
    }
  });

  onUnauthorized(() => {
    setUser(null);
    navigate("/login");
  });

  return (
    <AuthContext.Provider value={{ user, isAuthenticated, loading, login, logout }}>
      {props.children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
