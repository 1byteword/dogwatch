import { ErrorBoundary, createResource, createSignal, createMemo, Show, For } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { loadSlos } from "../domains/slo/service";
import { SloDefinition } from "../domains/slo/types";
import { Panel } from "../design/components/Panel";
import { Button } from "../design/components/Button";
import { Badge } from "../design/components/Badge";
import { Input } from "../design/components/Input";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";

const STATUS_TONE: Record<string, "ok" | "warn" | "error"> = {
  met: "ok", at_risk: "warn", breached: "error",
};

export function SloManagementPage() {
  const [slos, { refetch }] = createResource(loadSlos);
  const [localSlos, setLocalSlos] = createSignal<SloDefinition[]>([]);
  const [initialized, setInitialized] = createSignal(false);
  const [selectedId, setSelectedId] = createSignal("");
  const [showCreate, setShowCreate] = createSignal(false);

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formService, setFormService] = createSignal("");
  const [formTarget, setFormTarget] = createSignal(99.9);
  const [formWindow, setFormWindow] = createSignal("30d");

  useAutoRefresh(() => { if (!initialized()) refetch(); }, 30000);

  createMemo(() => {
    const data = slos();
    if (data && data.length > 0 && !initialized()) {
      setLocalSlos(data);
      setInitialized(true);
    }
  });

  const allSlos = () => initialized() ? localSlos() : (slos() || []);
  const selected = createMemo(() => allSlos().find((s) => s.id === selectedId()) ?? null);

  function createSlo() {
    const now = new Date().toISOString();
    const newSlo: SloDefinition = {
      id: `slo-${Date.now()}`, name: formName(), service: formService(),
      target: formTarget(), current: formTarget() - 0.1 + Math.random() * 0.2,
      budgetRemaining: 80 + Math.random() * 20, burnRate: 0.3 + Math.random() * 0.5,
      window: formWindow(), status: "met",
    };
    setLocalSlos((prev) => [newSlo, ...prev]);
    if (!initialized()) setInitialized(true);
    setShowCreate(false);
    setSelectedId(newSlo.id);
  }

  function deleteSlo(id: string) {
    setLocalSlos((prev) => prev.filter((s) => s.id !== id));
    if (selectedId() === id) setSelectedId("");
  }

  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
      <div class="split-layout">
        <Panel title={`SLOs (${allSlos().length})`} actions={
          <Button variant="primary" onClick={() => { setFormName(""); setFormService(""); setFormTarget(99.9); setFormWindow("30d"); setShowCreate(true); }}>Create SLO</Button>
        }>
          <Show when={!slos.loading || initialized()} fallback={<div class="empty-state">Loading SLOs…</div>}>
            <Show when={allSlos().length > 0} fallback={<div class="empty-state">No SLOs defined</div>}>
              <div class="alert-list" role="list">
                <For each={allSlos()}>
                  {(slo) => (
                    <button class={`alert-row ${selectedId() === slo.id ? "is-selected" : ""}`} onClick={() => setSelectedId(slo.id)} role="listitem">
                      <div style={{ flex: "1", "min-width": "0" }}>
                        <div style={{ "font-weight": "500", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>{slo.name}</div>
                        <div style={{ "font-size": "12px", color: "var(--text-muted)" }}>{slo.service}</div>
                      </div>
                      <div style={{ display: "flex", gap: "6px", "align-items": "center", "flex-shrink": "0" }}>
                        <Badge tone={STATUS_TONE[slo.status] || "neutral"}>{slo.status.replace("_", " ")}</Badge>
                        <span style={{ "font-size": "13px", "font-weight": "500", color: slo.budgetRemaining < 20 ? "var(--red)" : "var(--text)" }}>
                          {slo.budgetRemaining.toFixed(1)}% budget
                        </span>
                      </div>
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </Show>
        </Panel>

        <Panel title={selected() ? selected()!.name : "SLO Detail"} actions={
          <Show when={selected()}>
            <Button variant="danger" onClick={() => deleteSlo(selected()!.id)}>Delete</Button>
          </Show>
        }>
          <Show when={selected()} fallback={<div class="empty-state">Select an SLO to view details</div>}>
            {(slo) => (
              <div style={{ display: "flex", "flex-direction": "column", gap: "16px" }}>
                <div class="detail-grid">
                  <DetailRow label="Service" value={slo().service} />
                  <DetailRow label="Target" value={`${slo().target}%`} />
                  <DetailRow label="Current" value={`${slo().current.toFixed(2)}%`} />
                  <DetailRow label="Window" value={slo().window} />
                  <DetailRow label="Status">
                    <Badge tone={STATUS_TONE[slo().status] || "neutral"}>{slo().status.replace("_", " ")}</Badge>
                  </DetailRow>
                </div>

                <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "16px" }}>
                  <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                    <div style={{ "font-size": "28px", "font-weight": "600", color: slo().budgetRemaining < 20 ? "var(--red)" : "var(--accent)" }}>
                      {slo().budgetRemaining.toFixed(1)}%
                    </div>
                    <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Budget Remaining</div>
                    <div style={{ height: "6px", background: "var(--border)", "border-radius": "3px", "margin-top": "8px", overflow: "hidden" }}>
                      <div style={{ height: "100%", width: `${Math.min(100, slo().budgetRemaining)}%`, background: slo().budgetRemaining < 20 ? "var(--red)" : "var(--accent)", "border-radius": "3px" }} />
                    </div>
                  </div>
                  <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                    <div style={{ "font-size": "28px", "font-weight": "600", color: slo().burnRate > 5 ? "var(--red)" : slo().burnRate > 1 ? "var(--yellow)" : "var(--text)" }}>
                      {slo().burnRate.toFixed(1)}x
                    </div>
                    <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Burn Rate</div>
                    <div style={{ "font-size": "11px", color: "var(--text-muted)", "margin-top": "4px" }}>
                      {slo().burnRate <= 1 ? "Normal" : slo().burnRate <= 5 ? "Elevated" : "Critical"}
                    </div>
                  </div>
                </div>
              </div>
            )}
          </Show>
        </Panel>
      </div>

      <Show when={showCreate()}>
        <div class="modal-overlay" onClick={() => setShowCreate(false)}>
          <div class="modal-content" onClick={(e) => e.stopPropagation()} style={{ "max-width": "480px" }}>
            <h3 style={{ margin: "0 0 16px" }}>Create SLO</h3>
            <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
              <label class="form-label">
                Name
                <Input value={formName()} onInput={(e) => setFormName(e.currentTarget.value)} placeholder="e.g. Checkout Availability" />
              </label>
              <label class="form-label">
                Service
                <Input value={formService()} onInput={(e) => setFormService(e.currentTarget.value)} placeholder="e.g. checkout-api" />
              </label>
              <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "12px" }}>
                <label class="form-label">
                  Target (%)
                  <Input type="number" value={String(formTarget())} onInput={(e) => setFormTarget(Number(e.currentTarget.value))} step="0.01" />
                </label>
                <label class="form-label">
                  Window
                  <select value={formWindow()} onChange={(e) => setFormWindow(e.currentTarget.value)} class="form-select">
                    <option value="7d">7 days</option>
                    <option value="30d">30 days</option>
                    <option value="90d">90 days</option>
                  </select>
                </label>
              </div>
              <div style={{ display: "flex", gap: "8px", "justify-content": "flex-end", "padding-top": "8px" }}>
                <Button onClick={() => setShowCreate(false)}>Cancel</Button>
                <Button variant="primary" onClick={createSlo}>Create SLO</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </ErrorBoundary>
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

export default SloManagementPage;
