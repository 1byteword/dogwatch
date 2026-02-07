import { For, Show, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Panel } from "../design/components/Panel";
import { loadAuditLogs, loadAuditSummary } from "../domains/audit/service";

export function ConfigureAuditPage() {
  const [period, setPeriod] = createSignal("24h");
  const [offset, setOffset] = createSignal(0);
  const limit = 40;

  const [summary, { refetch: refetchSummary }] = createResource(period, loadAuditSummary);
  const logsQuery = createMemo(() => ({ limit, offset: offset() }));
  const [logs, { refetch: refetchLogs }] = createResource(logsQuery, (q) => loadAuditLogs(q.limit, q.offset));

  const errorMessage = createMemo(() => {
    const sErr = summary.error;
    const lErr = logs.error;
    return sErr?.message || lErr?.message || "";
  });

  function refreshAll() {
    refetchSummary();
    refetchLogs();
  }

  function nextPage() {
    if (logs()?.hasMore) setOffset((v) => v + limit);
  }

  function prevPage() {
    setOffset((v) => Math.max(0, v - limit));
  }

  return (
    <div class="page-grid">
      <Panel
        title="Audit Summary"
        actions={
          <>
            <select class="input corr-select" value={period()} onChange={(e) => setPeriod(e.currentTarget.value)}>
              <option value="1h">1h</option>
              <option value="6h">6h</option>
              <option value="24h">24h</option>
              <option value="7d">7d</option>
              <option value="30d">30d</option>
            </select>
            <Button onClick={refreshAll}>Refresh</Button>
          </>
        }
      >
        <Show when={!summary.loading} fallback={<div class="paragraph">Loading summary…</div>}>
          <div class="catalog-stats-grid">
            <div class="catalog-stat-card"><label>Queries</label><strong>{summary()?.totalQueries || 0}</strong></div>
            <div class="catalog-stat-card"><label>Failed Queries</label><strong>{summary()?.failedQueries || 0}</strong></div>
            <div class="catalog-stat-card"><label>Logins</label><strong>{summary()?.totalLogins || 0}</strong></div>
            <div class="catalog-stat-card"><label>Failed Logins</label><strong>{summary()?.failedLogins || 0}</strong></div>
            <div class="catalog-stat-card"><label>Admin Actions</label><strong>{summary()?.totalAdminActions || 0}</strong></div>
            <div class="catalog-stat-card"><label>Exports</label><strong>{summary()?.totalExports || 0}</strong></div>
          </div>
        </Show>
        <Show when={errorMessage()}>
          <div class="inline-notice">{errorMessage()}</div>
        </Show>
      </Panel>

      <Panel
        title="Audit Log Stream"
        actions={
          <>
            <Button onClick={prevPage}>Prev</Button>
            <Button onClick={nextPage}>Next</Button>
            <Badge tone="neutral">offset {offset()}</Badge>
          </>
        }
      >
        <Show when={!logs.loading} fallback={<div class="paragraph">Loading logs…</div>}>
          <div class="dense-table">
            <div class="dense-head audit-grid"><span>Time</span><span>User</span><span>Action</span><span>Resource</span><span>Outcome</span></div>
            <div class="dense-body short-body">
              <For each={logs()?.logs || []}>
                {(row) => (
                  <div class="dense-row audit-grid">
                    <span class="mono">{new Date(row.timestamp).toLocaleTimeString()}</span>
                    <span>{row.userEmail || row.userId || "-"}</span>
                    <span>{row.action}</span>
                    <span>{row.resourceType}</span>
                    <span><Badge tone={row.outcome === "failure" || row.outcome === "denied" ? "error" : "ok"}>{row.outcome}</Badge></span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Panel>
    </div>
  );
}
