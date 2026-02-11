import { createResource, createSignal, createMemo, Show, For } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { loadSyntheticChecks, loadSyntheticFailures } from "../domains/slo/service";
import { SyntheticCheck, SyntheticFailure } from "../domains/slo/types";
import { Panel } from "../design/components/Panel";
import { Button } from "../design/components/Button";
import { Badge } from "../design/components/Badge";
import { Input } from "../design/components/Input";

const STATUS_TONE: Record<string, "ok" | "warn" | "error"> = {
  passing: "ok", degraded: "warn", failing: "error",
};

export function SyntheticsManagementPage() {
  const [checks, { refetch: refetchChecks }] = createResource(loadSyntheticChecks);
  const [failures, { refetch: refetchFailures }] = createResource(loadSyntheticFailures);
  const [localChecks, setLocalChecks] = createSignal<SyntheticCheck[]>([]);
  const [initialized, setInitialized] = createSignal(false);
  const [selectedId, setSelectedId] = createSignal("");
  const [showCreate, setShowCreate] = createSignal(false);

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formUrl, setFormUrl] = createSignal("");
  const [formType, setFormType] = createSignal("http");
  const [formInterval, setFormInterval] = createSignal(60);

  useAutoRefresh(() => {
    if (!initialized()) { refetchChecks(); refetchFailures(); }
  }, 30000);

  createMemo(() => {
    const data = checks();
    if (data && data.length > 0 && !initialized()) {
      setLocalChecks(data);
      setInitialized(true);
    }
  });

  const allChecks = () => initialized() ? localChecks() : (checks() || []);
  const selected = createMemo(() => allChecks().find((c) => c.id === selectedId()) ?? null);
  const selectedFailures = createMemo(() =>
    (failures() || []).filter((f) => f.checkId === selectedId())
  );

  function createCheck() {
    const newCheck: SyntheticCheck = {
      id: `syn-${Date.now()}`, name: formName(), url: formUrl(), type: formType(),
      interval: formInterval(), status: "passing",
      lastRun: new Date().toISOString(), uptimePercent: 100, avgLatencyMs: 0,
    };
    setLocalChecks((prev) => [newCheck, ...prev]);
    if (!initialized()) setInitialized(true);
    setShowCreate(false);
    setSelectedId(newCheck.id);
  }

  function deleteCheck(id: string) {
    setLocalChecks((prev) => prev.filter((c) => c.id !== id));
    if (selectedId() === id) setSelectedId("");
  }

  return (
    <>
      <div class="split-layout">
        <Panel title={`Synthetic Checks (${allChecks().length})`} actions={
          <Button variant="primary" onClick={() => { setFormName(""); setFormUrl(""); setFormType("http"); setFormInterval(60); setShowCreate(true); }}>Create Check</Button>
        }>
          <Show when={!checks.loading || initialized()} fallback={<div class="empty-state">Loading checks…</div>}>
            <Show when={allChecks().length > 0} fallback={<div class="empty-state">No synthetic checks configured</div>}>
              <div class="alert-list" role="list">
                <For each={allChecks()}>
                  {(check) => (
                    <button class={`alert-row ${selectedId() === check.id ? "is-selected" : ""}`} onClick={() => setSelectedId(check.id)} role="listitem">
                      <div style={{ flex: "1", "min-width": "0" }}>
                        <div style={{ "font-weight": "500", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>{check.name}</div>
                        <div style={{ "font-size": "12px", color: "var(--text-muted)", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>{check.url}</div>
                      </div>
                      <div style={{ display: "flex", gap: "6px", "align-items": "center", "flex-shrink": "0" }}>
                        <Badge tone={STATUS_TONE[check.status] || "neutral"}>{check.status}</Badge>
                        <span style={{ "font-size": "12px", color: "var(--text-muted)" }}>{check.uptimePercent.toFixed(1)}%</span>
                      </div>
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </Show>
        </Panel>

        <Panel title={selected() ? selected()!.name : "Check Detail"} actions={
          <Show when={selected()}>
            <Button variant="danger" onClick={() => deleteCheck(selected()!.id)}>Delete</Button>
          </Show>
        }>
          <Show when={selected()} fallback={<div class="empty-state">Select a check to view details</div>}>
            {(check) => (
              <div style={{ display: "flex", "flex-direction": "column", gap: "16px" }}>
                <div class="detail-grid">
                  <DetailRow label="URL" value={check().url} />
                  <DetailRow label="Type" value={check().type} />
                  <DetailRow label="Interval" value={`${check().interval}s`} />
                  <DetailRow label="Status">
                    <Badge tone={STATUS_TONE[check().status] || "neutral"}>{check().status}</Badge>
                  </DetailRow>
                </div>

                <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "16px" }}>
                  <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                    <div style={{ "font-size": "28px", "font-weight": "600", color: check().uptimePercent < 99 ? "var(--red)" : "var(--accent)" }}>
                      {check().uptimePercent.toFixed(2)}%
                    </div>
                    <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Uptime</div>
                  </div>
                  <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                    <div style={{ "font-size": "28px", "font-weight": "600" }}>
                      {check().avgLatencyMs}ms
                    </div>
                    <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Avg Latency</div>
                  </div>
                </div>

                <Show when={selectedFailures().length > 0}>
                  <div>
                    <h4 style={{ margin: "0 0 8px", "font-size": "13px", color: "var(--text-muted)" }}>Recent Failures</h4>
                    <div class="alert-list">
                      <For each={selectedFailures()}>
                        {(f) => (
                          <div class="alert-row" style={{ cursor: "default" }}>
                            <div style={{ flex: "1", "min-width": "0" }}>
                              <div style={{ "font-size": "13px" }}>{f.error}</div>
                              <div style={{ "font-size": "11px", color: "var(--text-muted)" }}>
                                {f.timestamp ? new Date(f.timestamp).toLocaleString() : "—"} · {f.statusCode > 0 ? `HTTP ${f.statusCode}` : "timeout"} · {f.latencyMs}ms
                              </div>
                            </div>
                          </div>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>
              </div>
            )}
          </Show>
        </Panel>
      </div>

      <Show when={showCreate()}>
        <div class="modal-overlay" onClick={() => setShowCreate(false)}>
          <div class="modal-content" onClick={(e) => e.stopPropagation()} style={{ "max-width": "480px" }}>
            <h3 style={{ margin: "0 0 16px" }}>Create Synthetic Check</h3>
            <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
              <label class="form-label">
                Name
                <Input value={formName()} onInput={(e) => setFormName(e.currentTarget.value)} placeholder="e.g. Homepage Load" />
              </label>
              <label class="form-label">
                URL
                <Input value={formUrl()} onInput={(e) => setFormUrl(e.currentTarget.value)} placeholder="https://example.com" />
              </label>
              <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "12px" }}>
                <label class="form-label">
                  Type
                  <select value={formType()} onChange={(e) => setFormType(e.currentTarget.value)} class="form-select">
                    <option value="http">HTTP</option>
                    <option value="browser">Browser</option>
                    <option value="grpc">gRPC</option>
                    <option value="dns">DNS</option>
                    <option value="tcp">TCP</option>
                  </select>
                </label>
                <label class="form-label">
                  Interval (seconds)
                  <Input type="number" value={String(formInterval())} onInput={(e) => setFormInterval(Number(e.currentTarget.value))} min="10" />
                </label>
              </div>
              <div style={{ display: "flex", gap: "8px", "justify-content": "flex-end", "padding-top": "8px" }}>
                <Button onClick={() => setShowCreate(false)}>Cancel</Button>
                <Button variant="primary" onClick={createCheck}>Create Check</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </>
  );
}

function DetailRow(props: { label: string; value?: string; children?: any }) {
  return (
    <div style={{ display: "flex", gap: "12px", padding: "6px 0", "border-bottom": "1px solid var(--border)" }}>
      <span style={{ width: "100px", "flex-shrink": "0", color: "var(--text-muted)", "font-size": "13px" }}>{props.label}</span>
      <span style={{ "font-size": "13px" }}>{props.children || props.value || "—"}</span>
    </div>
  );
}
