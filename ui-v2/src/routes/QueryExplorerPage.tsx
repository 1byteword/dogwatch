import { ErrorBoundary, createResource, createSignal, Show, For } from "solid-js";
import { executeQuery, loadSavedQueries } from "../domains/query/service";
import { QueryResult, SavedQuery } from "../domains/query/types";
import { Panel } from "../design/components/Panel";
import { Button } from "../design/components/Button";
import { Badge } from "../design/components/Badge";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";

export function QueryExplorerPage() {
  const [query, setQuery] = createSignal("histogram_quantile(0.99, rate(http_request_duration_seconds_bucket{service=\"checkout-api\"}[5m]))");
  const [timeRange, setTimeRange] = createSignal("1h");
  const [result, setResult] = createSignal<QueryResult | null>(null);
  const [running, setRunning] = createSignal(false);
  const [error, setError] = createSignal("");
  const [saved] = createResource(loadSavedQueries);
  const [showSaved, setShowSaved] = createSignal(false);

  async function runQuery() {
    const q = query().trim();
    if (!q) return;
    setRunning(true);
    setError("");
    try {
      const res = await executeQuery(q, timeRange());
      if (res.error) {
        setError(res.error);
        setResult(null);
      } else {
        setResult(res);
      }
    } catch (e) {
      setError(String(e));
      setResult(null);
    } finally {
      setRunning(false);
    }
  }

  function loadSaved(sq: SavedQuery) {
    setQuery(sq.query);
    setShowSaved(false);
  }

  function handleKeyDown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      runQuery();
    }
  }

  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
    <div style={{ display: "flex", "flex-direction": "column", gap: "12px", height: "100%" }}>
      <Panel title="Query Explorer" actions={
        <div style={{ display: "flex", gap: "8px", "align-items": "center" }}>
          <Button onClick={() => setShowSaved(!showSaved())}>
            {showSaved() ? "Hide Saved" : `Saved (${saved()?.length || 0})`}
          </Button>
        </div>
      }>
        <div style={{ display: "flex", "flex-direction": "column", gap: "10px" }}>
          <textarea
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
            onKeyDown={handleKeyDown}
            placeholder="Enter PromQL or DQL query…"
            rows={4}
            class="form-textarea"
            style={{ "font-family": "monospace", "font-size": "13px" }}
            aria-label="Query editor"
          />
          <div style={{ display: "flex", gap: "8px", "align-items": "center" }}>
            <Button variant="primary" onClick={runQuery} disabled={running()}>
              {running() ? "Running…" : "Run Query"}
            </Button>
            <select value={timeRange()} onChange={(e) => setTimeRange(e.currentTarget.value)} class="form-select" style={{ width: "120px" }}>
              <option value="5m">Last 5m</option>
              <option value="15m">Last 15m</option>
              <option value="1h">Last 1h</option>
              <option value="6h">Last 6h</option>
              <option value="24h">Last 24h</option>
              <option value="7d">Last 7d</option>
            </select>
            <span style={{ color: "var(--text-muted)", "font-size": "12px" }}>Ctrl+Enter to run</span>
          </div>
        </div>
      </Panel>

      <Show when={showSaved()}>
        <Panel title="Saved Queries">
          <Show when={(saved() || []).length > 0} fallback={<div class="empty-state">No saved queries</div>}>
            <div class="alert-list" role="list">
              <For each={saved()}>
                {(sq) => (
                  <button class="alert-row" onClick={() => loadSaved(sq)} role="listitem">
                    <div style={{ flex: "1", "min-width": "0" }}>
                      <div style={{ "font-weight": "500" }}>{sq.name}</div>
                      <div style={{ "font-size": "12px", color: "var(--text-muted)", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>
                        {sq.description}
                      </div>
                    </div>
                    <code style={{ "font-size": "11px", color: "var(--accent)", "max-width": "300px", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>
                      {sq.query}
                    </code>
                  </button>
                )}
              </For>
            </div>
          </Show>
        </Panel>
      </Show>

      <Show when={error()}>
        <Panel title="Error">
          <div style={{ color: "var(--red)", "font-size": "13px" }}>{error()}</div>
        </Panel>
      </Show>

      <Show when={result()}>
        <Panel title={`Results (${result()!.count} rows)`} actions={
          <Badge tone="ok">{result()!.columns.length} columns</Badge>
        }>
          <div style={{ overflow: "auto", "max-height": "50vh" }}>
            <table class="data-table" style={{ width: "100%", "border-collapse": "collapse" }}>
              <thead>
                <tr>
                  <For each={result()!.columns}>
                    {(col) => (
                      <th style={{
                        "text-align": "left", padding: "8px 12px", "border-bottom": "2px solid var(--border)",
                        "font-size": "12px", "text-transform": "uppercase", color: "var(--text-muted)", "white-space": "nowrap",
                      }}>{col}</th>
                    )}
                  </For>
                </tr>
              </thead>
              <tbody>
                <For each={result()!.rows}>
                  {(row) => (
                    <tr>
                      <For each={result()!.columns}>
                        {(col) => (
                          <td style={{
                            padding: "6px 12px", "border-bottom": "1px solid var(--border)",
                            "font-size": "13px", "font-family": "monospace",
                          }}>
                            {formatCell(row[col])}
                          </td>
                        )}
                      </For>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </Panel>
      </Show>

      <Show when={!result() && !error() && !running()}>
        <div class="empty-state" style={{ padding: "48px", "text-align": "center" }}>
          <div style={{ "font-size": "15px", "margin-bottom": "8px" }}>Run a query to see results</div>
          <div style={{ "font-size": "13px", color: "var(--text-muted)" }}>
            Supports PromQL, DQL, and SQL-like syntax. Try clicking a saved query to get started.
          </div>
        </div>
      </Show>
    </div>
    </ErrorBoundary>
  );
}

function formatCell(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "number") return Number.isInteger(value) ? String(value) : value.toFixed(3);
  return String(value);
}

export default QueryExplorerPage;
