import { A, useNavigate } from "@solidjs/router";
import { ParentProps, createResource, createSignal } from "solid-js";
import { useAutoRefresh } from "../../core/live";
import { loadPlatformStatus } from "../../core/platform";
import { Chip } from "../../design/components/Chip";
import { Input } from "../../design/components/Input";
import { Button } from "../../design/components/Button";
import { Badge } from "../../design/components/Badge";
import dogwatchLogo from "../../assets/dogwatch-logo.png";

const navItems = [
  { href: "/app/dashboards", label: "Dashboards" },
  { href: "/app/detect/alerts", label: "Detect" },
  { href: "/app/detect/monitors", label: "Monitors" },
  { href: "/app/explore/query", label: "Explore" },
  { href: "/app/investigate/logs", label: "Investigate" },
  { href: "/app/correlate/timeline", label: "Correlate" },
  { href: "/app/respond/incidents", label: "Respond" },
  { href: "/app/improve/oncall", label: "On-call" },
  { href: "/app/improve/kubernetes", label: "Kubernetes" },
  { href: "/app/configure/catalog", label: "Catalog" },
  { href: "/app/configure/notifications", label: "Notify" },
  { href: "/app/configure/slos", label: "SLOs" },
  { href: "/app/configure/synthetics", label: "Synthetics" },
  { href: "/app/configure/recording-rules", label: "Recording Rules" },
  { href: "/app/configure/audit", label: "Audit" },
  { href: "/app/style-guide", label: "Style Guide" }
];

export function AppShell(props: ParentProps) {
  const navigate = useNavigate();
  const [command, setCommand] = createSignal("");
  const [commandNotice, setCommandNotice] = createSignal("");
  const [status, { refetch: refetchStatus }] = createResource(loadPlatformStatus);

  useAutoRefresh(() => refetchStatus(), 20000);

  function runCommand() {
    const q = command().trim().toLowerCase();
    if (!q) {
      refetchStatus();
      setCommandNotice("Refreshed platform status.");
      return;
    }

    if (q === "seed") {
      setCommandNotice("Run scripts/seed-v2-data.sh in terminal.");
      return;
    }

    const match = navItems.find((item) => {
      const label = item.label.toLowerCase();
      return label.includes(q) || item.href.toLowerCase().includes(q);
    });

    if (match) {
      navigate(match.href);
      setCommand("");
      setCommandNotice(`Navigated to ${match.label}.`);
      return;
    }

    setCommandNotice("Unknown command. Try: dashboards, detect, investigate, respond, catalog, notify, audit.");
  }

  return (
    <div class="app-shell">
      <aside class="app-sidebar">
        <div class="brand-block">
          <img src={dogwatchLogo} alt="dogwatch" class="brand-logo" />
        </div>
        <nav class="nav-stack" aria-label="Main navigation">
          {navItems.map((item) => (
            <A href={item.href} class="nav-item" activeClass="is-active">
              {item.label}
            </A>
          ))}
        </nav>
      </aside>
      <main class="app-main">
        <header class="topbar">
          <div class="context-strip">
            <Chip>prod</Chip>
            <Chip>last 1h</Chip>
            <Chip>all services</Chip>
          </div>
          <div class="topbar-search">
            <Input
              placeholder="Command (e.g. detect, audit, seed)"
              value={command()}
              onInput={(e) => setCommand(e.currentTarget.value)}
              onKeyDown={(e) => e.key === "Enter" && runCommand()}
              aria-label="Command input"
            />
            <Button onClick={runCommand} aria-label="Run command">Run</Button>
          </div>
          <div class="topbar-note">
            <Badge tone={status()?.healthy ? "ok" : "error"}>
              {status()?.healthy ? "api healthy" : "api degraded"}
            </Badge>
            <Badge tone="neutral">
              ready {status()?.okCount || 0}/{status()?.total || 0}
            </Badge>
            <span>{commandNotice() || "V2 operations shell"}</span>
          </div>
        </header>
        <section class="page-frame">{props.children}</section>
      </main>
    </div>
  );
}
