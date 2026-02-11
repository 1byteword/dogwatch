import { createResource, createSignal, createMemo, Show, For } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { loadRecordingRules, loadRecordingRuleHistory, loadRecordingRulesStatus } from "../domains/recording/service";
import type { RecordingRule, EvaluationHistory } from "../domains/recording/types";
import { Panel } from "../design/components/Panel";
import { Button } from "../design/components/Button";
import { Badge } from "../design/components/Badge";
import { Input } from "../design/components/Input";

function formatInterval(ns: number): string {
  const sec = ns / 1e9;
  if (sec >= 3600) return `${(sec / 3600).toFixed(0)}h`;
  if (sec >= 60) return `${(sec / 60).toFixed(0)}m`;
  return `${sec.toFixed(0)}s`;
}

function formatDurationNs(ns: number): string {
  const ms = ns / 1e6;
  if (ms < 1) return `${(ns / 1e3).toFixed(0)}us`;
  if (ms < 1000) return `${ms.toFixed(1)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function ruleStatus(rule: RecordingRule): { label: string; tone: "ok" | "warn" | "error" | "neutral" } {
  if (!rule.enabled) return { label: "disabled", tone: "neutral" };
  if (rule.last_error) return { label: "error", tone: "error" };
  return { label: "healthy", tone: "ok" };
}

export function RecordingRulesPage() {
  const [rules, { refetch: refetchRules }] = createResource(loadRecordingRules);
  const [status, { refetch: refetchStatus }] = createResource(loadRecordingRulesStatus);
  const [localRules, setLocalRules] = createSignal<RecordingRule[]>([]);
  const [initialized, setInitialized] = createSignal(false);
  const [selectedId, setSelectedId] = createSignal("");
  const [search, setSearch] = createSignal("");
  const [showCreate, setShowCreate] = createSignal(false);
  const [editing, setEditing] = createSignal(false);

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formExpr, setFormExpr] = createSignal("");
  const [formInterval, setFormInterval] = createSignal("60");
  const [formDesc, setFormDesc] = createSignal("");
  const [formLabels, setFormLabels] = createSignal("");
  const [formEnabled, setFormEnabled] = createSignal(true);

  const [history] = createResource(
    () => selectedId(),
    loadRecordingRuleHistory
  );

  useAutoRefresh(() => {
    if (!initialized()) { refetchRules(); refetchStatus(); }
  }, 30000);

  createMemo(() => {
    const data = rules();
    if (data && data.length > 0 && !initialized()) {
      setLocalRules(data);
      setInitialized(true);
    }
  });

  const allRules = () => initialized() ? localRules() : (rules() || []);

  const filtered = createMemo(() => {
    const q = search().toLowerCase();
    return allRules().filter((r) => {
      if (!q) return true;
      return `${r.name} ${r.expression} ${r.description}`.toLowerCase().includes(q);
    });
  });

  const selected = createMemo(() => allRules().find((r) => r.id === selectedId()) ?? null);

  function loadIntoForm(r: RecordingRule) {
    setFormName(r.name);
    setFormExpr(r.expression);
    setFormInterval(String(r.interval / 1e9));
    setFormDesc(r.description);
    setFormLabels(Object.entries(r.labels).map(([k, v]) => `${k}=${v}`).join(", "));
    setFormEnabled(r.enabled);
  }

  function resetForm() {
    setFormName(""); setFormExpr(""); setFormInterval("60");
    setFormDesc(""); setFormLabels(""); setFormEnabled(true);
  }

  function parseLabels(s: string): Record<string, string> {
    const labels: Record<string, string> = {};
    for (const part of s.split(",").map((p) => p.trim()).filter(Boolean)) {
      const eq = part.indexOf("=");
      if (eq > 0) labels[part.slice(0, eq).trim()] = part.slice(eq + 1).trim();
    }
    return labels;
  }

  function saveRule() {
    const now = new Date().toISOString();
    const labels = parseLabels(formLabels());
    const intervalNs = Number(formInterval()) * 1e9;

    if (editing() && selectedId()) {
      setLocalRules((prev) => prev.map((r) =>
        r.id === selectedId() ? {
          ...r, name: formName(), expression: formExpr(), interval: intervalNs,
          description: formDesc(), labels, enabled: formEnabled(), updated_at: now,
          last_error: "", last_value: r.last_value,
        } : r
      ));
      setEditing(false);
    } else {
      const newRule: RecordingRule = {
        id: `custom:${formName().replace(/[^a-z0-9:_]/gi, "_")}`,
        name: formName(), expression: formExpr(), interval: intervalNs,
        labels, enabled: formEnabled(), last_eval: "", last_error: "", last_value: 0,
        description: formDesc(), created_at: now, updated_at: now, created_by: "you@example.com",
      };
      setLocalRules((prev) => [newRule, ...prev]);
      setShowCreate(false);
      setSelectedId(newRule.id);
    }
    if (!initialized()) setInitialized(true);
  }

  function deleteRule(id: string) {
    setLocalRules((prev) => prev.filter((r) => r.id !== id));
    if (selectedId() === id) { setSelectedId(""); setEditing(false); }
  }

  function toggleRule(id: string) {
    setLocalRules((prev) => prev.map((r) =>
      r.id === id ? { ...r, enabled: !r.enabled, updated_at: new Date().toISOString() } : r
    ));
  }

  const st = () => status() || { total_rules: 0, enabled_rules: 0, total_evaluations: 0, successful_evaluations: 0, failed_evaluations: 0, last_eval_duration: 0, avg_eval_duration: 0 };

  return (
    <>
      {/* Status bar */}
      <div style={{ display: "flex", gap: "12px", "margin-bottom": "12px", "flex-wrap": "wrap" }}>
        <Badge tone="neutral">{st().total_rules} rules</Badge>
        <Badge tone="ok">{st().enabled_rules} enabled</Badge>
        <Badge tone="neutral">{st().total_evaluations.toLocaleString()} evals</Badge>
        <Badge tone={st().failed_evaluations > 0 ? "warn" : "ok"}>
          {st().failed_evaluations} failures
        </Badge>
        <Badge tone="neutral">avg {formatDurationNs(st().avg_eval_duration)}</Badge>
      </div>

      <div class="split-layout">
        <Panel title={`Recording Rules (${allRules().length})`} actions={
          <>
            <Input placeholder="Search rules…" value={search()} onInput={(e) => setSearch(e.currentTarget.value)} aria-label="Search recording rules" style={{ width: "180px" }} />
            <Button variant="primary" onClick={() => { resetForm(); setShowCreate(true); setEditing(false); }}>Create Rule</Button>
          </>
        }>
          <Show when={!rules.loading || initialized()} fallback={<div class="empty-state">Loading rules…</div>}>
            <Show when={filtered().length > 0} fallback={<div class="empty-state">No recording rules found</div>}>
              <div class="alert-list" role="list">
                <For each={filtered()}>
                  {(rule) => {
                    const st = ruleStatus(rule);
                    return (
                      <button
                        class={`alert-row ${selectedId() === rule.id ? "is-selected" : ""}`}
                        onClick={() => { setSelectedId(rule.id); setEditing(false); }}
                        role="listitem"
                      >
                        <div style={{ flex: "1", "min-width": "0" }}>
                          <div style={{ "font-weight": "500", opacity: rule.enabled ? 1 : 0.4, overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>
                            {rule.name}
                          </div>
                          <div style={{ "font-size": "12px", color: "var(--text-muted)", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>
                            {rule.expression}
                          </div>
                        </div>
                        <div style={{ display: "flex", gap: "6px", "align-items": "center", "flex-shrink": "0" }}>
                          <Badge tone={st.tone}>{st.label}</Badge>
                          <span style={{ "font-size": "12px", color: "var(--text-muted)" }}>
                            {formatInterval(rule.interval)}
                          </span>
                        </div>
                      </button>
                    );
                  }}
                </For>
              </div>
            </Show>
          </Show>
        </Panel>

        <Panel title={selected() ? (editing() ? "Edit Rule" : selected()!.name) : "Rule Detail"} actions={
          <Show when={selected() && !editing()}>
            <Button onClick={() => { loadIntoForm(selected()!); setEditing(true); }}>Edit</Button>
            <Button onClick={() => toggleRule(selected()!.id)}>{selected()!.enabled ? "Disable" : "Enable"}</Button>
            <Button variant="danger" onClick={() => deleteRule(selected()!.id)}>Delete</Button>
          </Show>
        }>
          <Show when={selected()} fallback={<div class="empty-state">Select a rule to view details</div>}>
            <Show when={!editing()} fallback={
              <RuleForm
                onSave={saveRule} onCancel={() => setEditing(false)}
                formName={formName} setFormName={setFormName}
                formExpr={formExpr} setFormExpr={setFormExpr}
                formInterval={formInterval} setFormInterval={setFormInterval}
                formDesc={formDesc} setFormDesc={setFormDesc}
                formLabels={formLabels} setFormLabels={setFormLabels}
                formEnabled={formEnabled} setFormEnabled={setFormEnabled}
              />
            }>
              {(() => {
                const rule = selected()!;
                const st = ruleStatus(rule);
                return (
                  <div style={{ display: "flex", "flex-direction": "column", gap: "16px" }}>
                    <div class="detail-grid">
                      <DetailRow label="Name" value={rule.name} />
                      <DetailRow label="Status">
                        <Badge tone={st.tone}>{st.label}</Badge>
                      </DetailRow>
                      <DetailRow label="Interval" value={formatInterval(rule.interval)} />
                      <DetailRow label="Expression">
                        <code style={{ "font-size": "12px", "word-break": "break-all", color: "var(--accent)" }}>{rule.expression}</code>
                      </DetailRow>
                      <Show when={rule.description}>
                        <DetailRow label="Description" value={rule.description} />
                      </Show>
                      <Show when={Object.keys(rule.labels).length > 0}>
                        <DetailRow label="Labels">
                          <div style={{ display: "flex", gap: "4px", "flex-wrap": "wrap" }}>
                            <For each={Object.entries(rule.labels)}>
                              {([k, v]) => (
                                <Badge tone="neutral">{k}={v}</Badge>
                              )}
                            </For>
                          </div>
                        </DetailRow>
                      </Show>
                      <DetailRow label="Created" value={`${rule.created_by} · ${rule.created_at ? new Date(rule.created_at).toLocaleDateString() : "—"}`} />
                    </div>

                    {/* Last evaluation stats */}
                    <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "16px" }}>
                      <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                        <div style={{ "font-size": "28px", "font-weight": "600", color: rule.last_error ? "var(--red)" : "var(--accent)" }}>
                          {rule.last_value !== undefined && rule.last_value !== 0
                            ? (rule.last_value < 1 ? rule.last_value.toFixed(3) : rule.last_value.toFixed(1))
                            : "—"}
                        </div>
                        <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Last Value</div>
                      </div>
                      <div style={{ background: "var(--surface)", padding: "16px", "border-radius": "8px", "text-align": "center" }}>
                        <div style={{ "font-size": "28px", "font-weight": "600" }}>
                          {rule.last_eval ? new Date(rule.last_eval).toLocaleTimeString() : "—"}
                        </div>
                        <div style={{ "font-size": "12px", color: "var(--text-muted)", "margin-top": "4px" }}>Last Evaluated</div>
                      </div>
                    </div>

                    <Show when={rule.last_error}>
                      <div style={{ background: "rgba(255,59,48,0.1)", border: "1px solid var(--red)", "border-radius": "6px", padding: "10px", "font-size": "13px", color: "var(--red)" }}>
                        {rule.last_error}
                      </div>
                    </Show>

                    {/* Evaluation history */}
                    <Show when={(history() || []).length > 0}>
                      <div>
                        <h4 style={{ margin: "0 0 8px", "font-size": "13px", color: "var(--text-muted)" }}>Evaluation History</h4>
                        <div class="alert-list" style={{ "max-height": "240px", overflow: "auto" }}>
                          <For each={history()}>
                            {(h) => (
                              <div class="alert-row" style={{ cursor: "default" }}>
                                <div style={{ flex: "1", "min-width": "0" }}>
                                  <div style={{ "font-size": "13px", display: "flex", gap: "8px", "align-items": "center" }}>
                                    <Badge tone={h.success ? "ok" : "error"}>{h.success ? "ok" : "err"}</Badge>
                                    <span style={{ "font-family": "monospace" }}>
                                      {h.value !== 0 ? (h.value < 1 ? h.value.toFixed(3) : h.value.toFixed(1)) : "—"}
                                    </span>
                                    <span style={{ color: "var(--text-muted)", "font-size": "11px" }}>
                                      {h.duration_ms.toFixed(1)}ms
                                    </span>
                                  </div>
                                  <Show when={h.error}>
                                    <div style={{ "font-size": "11px", color: "var(--red)", "margin-top": "2px" }}>{h.error}</div>
                                  </Show>
                                </div>
                                <span style={{ "font-size": "11px", color: "var(--text-muted)", "flex-shrink": "0" }}>
                                  {h.timestamp ? new Date(h.timestamp).toLocaleTimeString() : "—"}
                                </span>
                              </div>
                            )}
                          </For>
                        </div>
                      </div>
                    </Show>
                  </div>
                );
              })()}
            </Show>
          </Show>
        </Panel>
      </div>

      {/* Create modal */}
      <Show when={showCreate()}>
        <div class="modal-overlay" onClick={() => setShowCreate(false)}>
          <div class="modal-content" onClick={(e) => e.stopPropagation()} style={{ "max-width": "600px" }}>
            <h3 style={{ margin: "0 0 16px" }}>Create Recording Rule</h3>
            <RuleForm
              onSave={saveRule} onCancel={() => setShowCreate(false)}
              formName={formName} setFormName={setFormName}
              formExpr={formExpr} setFormExpr={setFormExpr}
              formInterval={formInterval} setFormInterval={setFormInterval}
              formDesc={formDesc} setFormDesc={setFormDesc}
              formLabels={formLabels} setFormLabels={setFormLabels}
              formEnabled={formEnabled} setFormEnabled={setFormEnabled}
            />
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

function RuleForm(props: {
  onSave: () => void; onCancel: () => void;
  formName: () => string; setFormName: (v: string) => void;
  formExpr: () => string; setFormExpr: (v: string) => void;
  formInterval: () => string; setFormInterval: (v: string) => void;
  formDesc: () => string; setFormDesc: (v: string) => void;
  formLabels: () => string; setFormLabels: (v: string) => void;
  formEnabled: () => boolean; setFormEnabled: (v: boolean) => void;
}) {
  return (
    <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
      <label class="form-label">
        Output Metric Name
        <Input value={props.formName()} onInput={(e) => props.setFormName(e.currentTarget.value)} placeholder="e.g. service:request_rate:5m" />
        <span style={{ "font-size": "11px", color: "var(--text-muted)" }}>Convention: level:metric:window (e.g. service:http_requests:5m)</span>
      </label>
      <label class="form-label">
        Description
        <Input value={props.formDesc()} onInput={(e) => props.setFormDesc(e.currentTarget.value)} placeholder="What does this rule compute?" />
      </label>
      <label class="form-label">
        PromQL Expression
        <textarea
          value={props.formExpr()}
          onInput={(e) => props.setFormExpr(e.currentTarget.value)}
          placeholder="e.g. sum(rate(http_requests_total[5m])) by (service)"
          rows={3}
          class="form-textarea"
          style={{ "font-family": "monospace", "font-size": "13px" }}
        />
      </label>
      <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "12px" }}>
        <label class="form-label">
          Evaluation Interval
          <select value={props.formInterval()} onChange={(e) => props.setFormInterval(e.currentTarget.value)} class="form-select">
            <option value="15">15 seconds</option>
            <option value="30">30 seconds</option>
            <option value="60">1 minute</option>
            <option value="300">5 minutes</option>
            <option value="900">15 minutes</option>
          </select>
        </label>
        <label class="form-label">
          Labels (key=value, comma-separated)
          <Input value={props.formLabels()} onInput={(e) => props.setFormLabels(e.currentTarget.value)} placeholder="e.g. __name__=my_metric, team=platform" />
        </label>
      </div>
      <label style={{ display: "flex", "align-items": "center", gap: "8px", "font-size": "13px", cursor: "pointer" }}>
        <input type="checkbox" checked={props.formEnabled()} onChange={(e) => props.setFormEnabled(e.currentTarget.checked)} />
        Enabled
      </label>
      <div style={{ display: "flex", gap: "8px", "justify-content": "flex-end", "padding-top": "8px" }}>
        <Button onClick={props.onCancel}>Cancel</Button>
        <Button variant="primary" onClick={props.onSave}>Save Rule</Button>
      </div>
    </div>
  );
}

export default RecordingRulesPage;
