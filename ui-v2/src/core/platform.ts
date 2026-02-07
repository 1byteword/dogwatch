export interface ReadyCheck {
  name: string;
  status: string;
  ok: boolean;
}

export interface PlatformStatus {
  healthy: boolean;
  checks: ReadyCheck[];
  okCount: number;
  total: number;
}

function parseReadyz(text: string): ReadyCheck[] {
  const lines = text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  return lines
    .map((line) => {
      const ok = line.startsWith("[+]");
      const body = line.replace(/^\[[+\-]\]\s*/, "");
      const parts = body.split(/\s+/);
      const name = parts[0] || "unknown";
      const status = parts.slice(1).join(" ") || "unknown";
      return { name, status, ok: ok || status === "ok" || status === "not configured" };
    })
    .filter((check) => check.name !== "unknown");
}

export async function loadPlatformStatus(): Promise<PlatformStatus> {
  const [healthRes, readyRes] = await Promise.all([
    fetch("/health"),
    fetch("/readyz?verbose=true")
  ]);

  const healthy = healthRes.ok;
  const readyText = await readyRes.text();
  const checks = parseReadyz(readyText);
  const okCount = checks.filter((check) => check.ok).length;

  return {
    healthy,
    checks,
    okCount,
    total: checks.length
  };
}
