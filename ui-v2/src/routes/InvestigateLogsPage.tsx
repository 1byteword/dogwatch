import { ErrorBoundary, For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { loadLogs, loadLogServices } from "../domains/logs/service";

export function InvestigateLogsPage() {
  const [query, setQuery] = createSignal("");
  const [level, setLevel] = createSignal("");
  const [service, setService] = createSignal("");
  const [since, setSince] = createSignal("1h");
  const [live, setLive] = createSignal(false);
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal(Date.now());
  const now = createNowTicker(15000);

  const queryObj = createMemo(() => ({
    q: query(),
    level: level(),
    service: service(),
    since: since(),
    limit: 200
  }));

  const [rows, { refetch }] = createResource(queryObj, loadLogs);
  const [services] = createResource(loadLogServices);

  createEffect(() => {
    if (rows()) setLastUpdatedAt(Date.now());
  });

  useAutoRefresh(() => {
    if (live()) refetch();
  }, 5000);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  function runSearch() {
    refetch();
    setLastUpdatedAt(Date.now());
  }

  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
    <div class="page-grid">
      <Panel
        title="Logs Explorer"
        actions={
          <>
            <Button onClick={runSearch}>Refresh</Button>
            <Button variant={live() ? "primary" : "default"} onClick={() => setLive((v) => !v)}>
              {live() ? "Live On" : "Live Off"}
            </Button>
            <Badge tone="neutral">updated {updatedAgo()}</Badge>
          </>
        }
      >
        <div class="logs-toolbar">
          <Input
            placeholder="Search logs..."
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
          />
          <select class="input logs-select" value={level()} onChange={(e) => setLevel(e.currentTarget.value)}>
            <option value="">all levels</option>
            <option value="debug">debug</option>
            <option value="info">info</option>
            <option value="warn">warn</option>
            <option value="error">error</option>
            <option value="fatal">fatal</option>
          </select>
          <select class="input logs-select" value={service()} onChange={(e) => setService(e.currentTarget.value)}>
            <option value="">all services</option>
            <For each={services() || []}>{(svc) => <option value={svc}>{svc}</option>}</For>
          </select>
          <select class="input logs-select" value={since()} onChange={(e) => setSince(e.currentTarget.value)}>
            <option value="15m">15m</option>
            <option value="1h">1h</option>
            <option value="6h">6h</option>
            <option value="24h">24h</option>
          </select>
          <Button variant="primary" onClick={runSearch}>Run</Button>
        </div>

        <Show when={!rows.loading} fallback={<div class="paragraph">Loading logs…</div>}>
          <div class="logs-table">
            <div class="logs-head">
              <span>Time</span>
              <span>Level</span>
              <span>Service</span>
              <span>Message</span>
            </div>
            <div class="logs-body">
              <For each={rows() || []}>
                {(log) => (
                  <div class="logs-row">
                    <span class="mono">{new Date(log.timestamp).toLocaleTimeString()}</span>
                    <span class={`badge tone-${log.level === "error" ? "error" : log.level === "warn" ? "warn" : "neutral"}`}>
                      {log.level}
                    </span>
                    <span>{log.service || "-"}</span>
                    <span class="logs-message">{log.message}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Panel>

      <Panel title="Performance Notes" actions={<Badge tone="warn">High Volume</Badge>}>
        <ul class="list-simple">
          <li>Virtualized rows required</li>
          <li>Streaming updates with backpressure</li>
          <li>Stable keyboard navigation</li>
        </ul>
      </Panel>
    </div>
    </ErrorBoundary>
  );
}

export default InvestigateLogsPage;
