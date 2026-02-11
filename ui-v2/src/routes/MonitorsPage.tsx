import { createResource, createSignal, createMemo, Show, For } from "solid-js";
import { useAutoRefresh } from "../core/live";
import { loadAlertRules } from "../domains/alerts/service";
import { AlertRule, RuleType, RuleSeverity } from "../domains/alerts/types";
import { Panel } from "../design/components/Panel";
import { Button } from "../design/components/Button";
import { Badge } from "../design/components/Badge";
import { Input } from "../design/components/Input";

const RULE_TYPE_LABELS: Record<RuleType, string> = {
  threshold: "Threshold",
  anomaly: "Anomaly",
  change: "Change",
  absence: "Absence",
  composite: "Composite",
};

const SEVERITY_TONE: Record<RuleSeverity, "error" | "warn" | "neutral"> = {
  critical: "error",
  warning: "warn",
  info: "neutral",
};

const CONDITION_LABELS: Record<string, string> = {
  gt: ">", lt: "<", gte: ">=", lte: "<=", eq: "=", neq: "!=",
};

type MonitorTemplate = {
  id: string;
  name: string;
  description: string;
  category: string;
  rule: Omit<AlertRule, "id" | "createdAt" | "updatedAt" | "createdBy">;
};

const TEMPLATE_CATEGORIES = ["Latency", "Errors", "Saturation", "Availability", "Business"] as const;

const MONITOR_TEMPLATES: MonitorTemplate[] = [
  {
    id: "p99-latency", name: "P99 Latency Spike", category: "Latency",
    description: "Alert when p99 latency exceeds threshold for any service.",
    rule: {
      name: "P99 Latency > 500ms", description: "Service p99 latency exceeded 500ms for 5 minutes",
      type: "threshold", enabled: true,
      query: 'histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))',
      condition: "gt", threshold: 0.5, severity: "warning", forDuration: "5m",
      notifyChannels: ["#alerts"], labels: {},
    },
  },
  {
    id: "error-rate", name: "Error Rate Spike", category: "Errors",
    description: "Alert when HTTP 5xx error rate crosses a percentage threshold.",
    rule: {
      name: "Error Rate > 5%", description: "HTTP 5xx error rate exceeded 5% for 5 minutes",
      type: "threshold", enabled: true,
      query: 'sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100',
      condition: "gt", threshold: 5, severity: "critical", forDuration: "5m",
      notifyChannels: ["#incidents-critical", "PagerDuty"], labels: {},
    },
  },
  {
    id: "cpu-saturation", name: "CPU Saturation", category: "Saturation",
    description: "Alert when CPU usage exceeds safe operating threshold.",
    rule: {
      name: "CPU > 85%", description: "Host CPU utilization exceeded 85% for 10 minutes",
      type: "threshold", enabled: true,
      query: 'avg(rate(process_cpu_seconds_total[5m])) * 100',
      condition: "gt", threshold: 85, severity: "warning", forDuration: "10m",
      notifyChannels: ["#infra-alerts"], labels: {},
    },
  },
  {
    id: "memory-pressure", name: "Memory Pressure", category: "Saturation",
    description: "Alert when memory usage approaches limit.",
    rule: {
      name: "Memory > 90%", description: "Memory utilization exceeded 90%",
      type: "threshold", enabled: true,
      query: 'process_resident_memory_bytes / process_virtual_memory_max_bytes * 100',
      condition: "gt", threshold: 90, severity: "critical", forDuration: "5m",
      notifyChannels: ["#infra-alerts", "PagerDuty"], labels: {},
    },
  },
  {
    id: "pod-restarts", name: "Pod Restart Loop", category: "Availability",
    description: "Alert when a pod restarts repeatedly indicating crash loops.",
    rule: {
      name: "Pod Restarts > 3", description: "Container restarts exceeded 3 in 15 minutes",
      type: "threshold", enabled: true,
      query: 'increase(kube_pod_container_status_restarts_total[15m])',
      condition: "gt", threshold: 3, severity: "warning", forDuration: "1m",
      notifyChannels: ["#k8s-alerts"], labels: {},
    },
  },
  {
    id: "endpoint-down", name: "Endpoint Down", category: "Availability",
    description: "Alert when a synthetic check fails consecutively.",
    rule: {
      name: "Endpoint Unreachable", description: "Synthetic check failing for 3 consecutive checks",
      type: "absence", enabled: true,
      query: 'probe_success',
      condition: "lt", threshold: 1, severity: "critical", forDuration: "3m",
      notifyChannels: ["#incidents-critical", "PagerDuty"], labels: {},
    },
  },
  {
    id: "disk-space", name: "Disk Space Low", category: "Saturation",
    description: "Alert when disk usage exceeds safe threshold.",
    rule: {
      name: "Disk > 80%", description: "Disk utilization exceeded 80%",
      type: "threshold", enabled: true,
      query: '(node_filesystem_size_bytes - node_filesystem_avail_bytes) / node_filesystem_size_bytes * 100',
      condition: "gt", threshold: 80, severity: "warning", forDuration: "10m",
      notifyChannels: ["#infra-alerts"], labels: {},
    },
  },
  {
    id: "slo-burn", name: "SLO Budget Burn", category: "Business",
    description: "Alert when SLO error budget is burning too fast.",
    rule: {
      name: "SLO Burn Rate > 5x", description: "Error budget burning faster than 5x normal rate",
      type: "threshold", enabled: true,
      query: 'slo_burn_rate',
      condition: "gt", threshold: 5, severity: "critical", forDuration: "5m",
      notifyChannels: ["#slo-alerts", "PagerDuty"], labels: {},
    },
  },
  {
    id: "anomaly-detect", name: "Metric Anomaly", category: "Business",
    description: "Alert when a metric deviates significantly from baseline.",
    rule: {
      name: "Anomaly Detected", description: "Metric value deviated >3 standard deviations from rolling baseline",
      type: "anomaly", enabled: true,
      query: 'http_request_duration_seconds_sum / http_request_duration_seconds_count',
      condition: "gt", threshold: 3, severity: "warning", forDuration: "10m",
      notifyChannels: ["#alerts"], labels: {},
    },
  },
  {
    id: "queue-depth", name: "Queue Depth", category: "Business",
    description: "Alert when message queue depth indicates processing lag.",
    rule: {
      name: "Queue Depth > 1000", description: "Message queue depth exceeded 1000 messages",
      type: "threshold", enabled: true,
      query: 'queue_messages_ready',
      condition: "gt", threshold: 1000, severity: "warning", forDuration: "5m",
      notifyChannels: ["#alerts"], labels: {},
    },
  },
];

function emptyRule(): Omit<AlertRule, "id" | "createdAt" | "updatedAt" | "createdBy"> {
  return {
    name: "", description: "", type: "threshold", enabled: true,
    query: "", condition: "gt", threshold: 0, severity: "warning",
    forDuration: "5m", notifyChannels: [], labels: {},
  };
}

export function MonitorsPage() {
  const [rules, { refetch }] = createResource(loadAlertRules);
  const [localRules, setLocalRules] = createSignal<AlertRule[]>([]);
  const [initialized, setInitialized] = createSignal(false);
  const [search, setSearch] = createSignal("");
  const [selectedId, setSelectedId] = createSignal("");
  const [editing, setEditing] = createSignal(false);
  const [showCreate, setShowCreate] = createSignal(false);
  const [createStep, setCreateStep] = createSignal(0); // 0=template, 1=configure, 2=notify

  // Form state
  const [formName, setFormName] = createSignal("");
  const [formDesc, setFormDesc] = createSignal("");
  const [formType, setFormType] = createSignal<RuleType>("threshold");
  const [formQuery, setFormQuery] = createSignal("");
  const [formCondition, setFormCondition] = createSignal("gt");
  const [formThreshold, setFormThreshold] = createSignal(0);
  const [formSeverity, setFormSeverity] = createSignal<RuleSeverity>("warning");
  const [formFor, setFormFor] = createSignal("5m");
  const [formChannels, setFormChannels] = createSignal("");
  const [formEnabled, setFormEnabled] = createSignal(true);

  useAutoRefresh(() => {
    if (!initialized()) refetch();
  }, 30000);

  // Sync API data into local state on first load
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
      return `${r.name} ${r.query} ${r.description} ${r.severity} ${r.type}`.toLowerCase().includes(q);
    });
  });

  const selected = createMemo(() => allRules().find((r) => r.id === selectedId()) ?? null);

  function loadIntoForm(r: AlertRule) {
    setFormName(r.name); setFormDesc(r.description); setFormType(r.type);
    setFormQuery(r.query); setFormCondition(r.condition); setFormThreshold(r.threshold);
    setFormSeverity(r.severity); setFormFor(r.forDuration);
    setFormChannels(r.notifyChannels.join(", ")); setFormEnabled(r.enabled);
  }

  function loadFromTemplate(tpl: MonitorTemplate) {
    const r = tpl.rule;
    setFormName(r.name); setFormDesc(r.description); setFormType(r.type);
    setFormQuery(r.query); setFormCondition(r.condition); setFormThreshold(r.threshold);
    setFormSeverity(r.severity); setFormFor(r.forDuration);
    setFormChannels(r.notifyChannels.join(", ")); setFormEnabled(r.enabled);
    setCreateStep(1);
  }

  function resetForm() {
    const e = emptyRule();
    setFormName(e.name); setFormDesc(e.description); setFormType(e.type);
    setFormQuery(e.query); setFormCondition(e.condition); setFormThreshold(e.threshold);
    setFormSeverity(e.severity); setFormFor(e.forDuration);
    setFormChannels(""); setFormEnabled(e.enabled);
  }

  function saveRule() {
    const now = new Date().toISOString();
    const channels = formChannels().split(",").map((s) => s.trim()).filter(Boolean);

    if (editing() && selectedId()) {
      setLocalRules((prev) => prev.map((r) =>
        r.id === selectedId() ? {
          ...r, name: formName(), description: formDesc(), type: formType(),
          query: formQuery(), condition: formCondition(), threshold: formThreshold(),
          severity: formSeverity(), forDuration: formFor(), notifyChannels: channels,
          enabled: formEnabled(), updatedAt: now,
        } : r
      ));
      setEditing(false);
    } else {
      const newRule: AlertRule = {
        id: `rule-${Date.now()}`, name: formName(), description: formDesc(), type: formType(),
        enabled: formEnabled(), query: formQuery(), condition: formCondition(), threshold: formThreshold(),
        severity: formSeverity(), forDuration: formFor(), notifyChannels: channels,
        labels: {}, createdAt: now, updatedAt: now, createdBy: "you@example.com",
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
    setLocalRules((prev) => prev.map((r) => r.id === id ? { ...r, enabled: !r.enabled } : r));
  }

  const stepLabel = () => {
    const s = createStep();
    if (s === 0) return "Choose Template";
    if (s === 1) return "Configure Rule";
    return "Set Notifications";
  };

  return (
    <>
      <div class="split-layout">
        <Panel title={`Monitors (${allRules().length})`} actions={
          <>
            <Input placeholder="Search monitors…" value={search()} onInput={(e) => setSearch(e.currentTarget.value)} aria-label="Search monitors" style={{ width: "180px" }} />
            <Button variant="primary" onClick={() => { resetForm(); setCreateStep(0); setShowCreate(true); setEditing(false); }}>Create Monitor</Button>
          </>
        }>
          <Show when={!rules.loading || initialized()} fallback={<div class="empty-state">Loading monitors…</div>}>
            <Show when={filtered().length > 0} fallback={<div class="empty-state">No monitors found</div>}>
              <div class="alert-list" role="list">
                <For each={filtered()}>
                  {(rule) => (
                    <button
                      class={`alert-row ${selectedId() === rule.id ? "is-selected" : ""}`}
                      onClick={() => { setSelectedId(rule.id); setEditing(false); }}
                      role="listitem"
                    >
                      <div style={{ display: "flex", "align-items": "center", gap: "8px", flex: "1", "min-width": "0" }}>
                        <span style={{ opacity: rule.enabled ? 1 : 0.4, "font-weight": "500", overflow: "hidden", "text-overflow": "ellipsis", "white-space": "nowrap" }}>
                          {rule.name}
                        </span>
                      </div>
                      <div style={{ display: "flex", gap: "6px", "align-items": "center", "flex-shrink": "0" }}>
                        <Badge tone="neutral">{RULE_TYPE_LABELS[rule.type]}</Badge>
                        <Badge tone={SEVERITY_TONE[rule.severity]}>{rule.severity}</Badge>
                        <Badge tone={rule.enabled ? "ok" : "neutral"}>{rule.enabled ? "on" : "off"}</Badge>
                      </div>
                    </button>
                  )}
                </For>
              </div>
            </Show>
          </Show>
        </Panel>

        <Panel title={selected() ? (editing() ? "Edit Monitor" : selected()!.name) : "Monitor Detail"} actions={
          <Show when={selected() && !editing()}>
            <Button onClick={() => { loadIntoForm(selected()!); setEditing(true); }}>Edit</Button>
            <Button onClick={() => toggleRule(selected()!.id)}>{selected()!.enabled ? "Disable" : "Enable"}</Button>
            <Button variant="danger" onClick={() => deleteRule(selected()!.id)}>Delete</Button>
          </Show>
        }>
          <Show when={selected()} fallback={<div class="empty-state">Select a monitor to view details</div>}>
            <Show when={!editing()} fallback={<RuleForm onSave={saveRule} onCancel={() => setEditing(false)} formName={formName} setFormName={setFormName} formDesc={formDesc} setFormDesc={setFormDesc} formType={formType} setFormType={setFormType} formQuery={formQuery} setFormQuery={setFormQuery} formCondition={formCondition} setFormCondition={setFormCondition} formThreshold={formThreshold} setFormThreshold={setFormThreshold} formSeverity={formSeverity} setFormSeverity={setFormSeverity} formFor={formFor} setFormFor={setFormFor} formChannels={formChannels} setFormChannels={setFormChannels} formEnabled={formEnabled} setFormEnabled={setFormEnabled} />}>
              <div class="detail-grid">
                <DetailRow label="Type" value={RULE_TYPE_LABELS[selected()!.type]} />
                <DetailRow label="Severity" value={selected()!.severity} />
                <DetailRow label="Status" value={selected()!.enabled ? "Enabled" : "Disabled"} />
                <DetailRow label="Condition" value={`${CONDITION_LABELS[selected()!.condition] || selected()!.condition} ${selected()!.threshold}`} />
                <DetailRow label="For" value={selected()!.forDuration || "immediate"} />
                <DetailRow label="Query">
                  <code style={{ "font-size": "12px", "word-break": "break-all", color: "var(--accent)" }}>{selected()!.query}</code>
                </DetailRow>
                <Show when={selected()!.description}>
                  <DetailRow label="Description" value={selected()!.description} />
                </Show>
                <Show when={selected()!.notifyChannels.length > 0}>
                  <DetailRow label="Notify" value={selected()!.notifyChannels.join(", ")} />
                </Show>
                <DetailRow label="Created" value={`${selected()!.createdBy} · ${selected()!.createdAt ? new Date(selected()!.createdAt).toLocaleDateString() : "—"}`} />
              </div>
            </Show>
          </Show>
        </Panel>
      </div>

      <Show when={showCreate()}>
        <div class="modal-overlay" onClick={() => setShowCreate(false)}>
          <div class="modal-content" onClick={(e) => e.stopPropagation()} style={{ "max-width": "660px" }}>
            {/* Step indicator */}
            <div style={{ display: "flex", "align-items": "center", gap: "8px", "margin-bottom": "16px" }}>
              <h3 style={{ margin: "0", flex: "1" }}>Create Monitor — {stepLabel()}</h3>
              <div style={{ display: "flex", gap: "4px" }}>
                <For each={[0, 1, 2]}>
                  {(s) => (
                    <div style={{
                      width: "8px", height: "8px", "border-radius": "50%",
                      background: createStep() >= s ? "var(--accent)" : "var(--border)",
                    }} />
                  )}
                </For>
              </div>
            </div>

            {/* Step 0: Template picker */}
            <Show when={createStep() === 0}>
              <p style={{ color: "var(--text-muted)", "font-size": "13px", margin: "0 0 12px" }}>
                Pick a template to pre-fill your monitor, or start from scratch.
              </p>
              <div style={{ "max-height": "55vh", overflow: "auto" }}>
                <button
                  class="widget-picker-card"
                  style={{ width: "100%", "text-align": "left", "margin-bottom": "8px", "border": "2px solid var(--border)" }}
                  onClick={() => { resetForm(); setCreateStep(1); }}
                >
                  <strong>Blank Monitor</strong>
                  <p style={{ color: "var(--text-muted)", "font-size": "12px", margin: "4px 0 0" }}>
                    Start from scratch with an empty PromQL query.
                  </p>
                </button>
                <For each={TEMPLATE_CATEGORIES as unknown as string[]}>
                  {(cat) => {
                    const tpls = MONITOR_TEMPLATES.filter((t) => t.category === cat);
                    return (
                      <Show when={tpls.length > 0}>
                        <h4 style={{ margin: "12px 0 6px", "font-size": "12px", "text-transform": "uppercase", "letter-spacing": "0.05em", color: "var(--text-muted)", "border-bottom": "1px solid var(--border)", "padding-bottom": "4px" }}>
                          {cat}
                        </h4>
                        <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "8px" }}>
                          <For each={tpls}>
                            {(tpl) => (
                              <button
                                class="widget-picker-card"
                                style={{ "text-align": "left", "border": "2px solid var(--border)" }}
                                onClick={() => loadFromTemplate(tpl)}
                              >
                                <strong>{tpl.name}</strong>
                                <p style={{ color: "var(--text-muted)", "font-size": "12px", margin: "4px 0 0" }}>
                                  {tpl.description}
                                </p>
                                <div style={{ display: "flex", gap: "4px", "margin-top": "6px" }}>
                                  <Badge tone={SEVERITY_TONE[tpl.rule.severity]}>{tpl.rule.severity}</Badge>
                                  <Badge tone="neutral">{RULE_TYPE_LABELS[tpl.rule.type]}</Badge>
                                </div>
                              </button>
                            )}
                          </For>
                        </div>
                      </Show>
                    );
                  }}
                </For>
              </div>
              <div style={{ display: "flex", gap: "8px", "justify-content": "flex-end", "padding-top": "12px" }}>
                <Button onClick={() => setShowCreate(false)}>Cancel</Button>
              </div>
            </Show>

            {/* Step 1: Configure rule */}
            <Show when={createStep() === 1}>
              <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
                <label class="form-label">
                  Name
                  <Input value={formName()} onInput={(e) => setFormName(e.currentTarget.value)} placeholder="e.g. Checkout P99 Latency" />
                </label>
                <label class="form-label">
                  Description
                  <Input value={formDesc()} onInput={(e) => setFormDesc(e.currentTarget.value)} placeholder="What does this monitor check?" />
                </label>
                <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "12px" }}>
                  <label class="form-label">
                    Type
                    <select value={formType()} onChange={(e) => setFormType(e.currentTarget.value as RuleType)} class="form-select">
                      <option value="threshold">Threshold</option>
                      <option value="anomaly">Anomaly</option>
                      <option value="change">Change</option>
                      <option value="absence">Absence</option>
                      <option value="composite">Composite</option>
                    </select>
                  </label>
                  <label class="form-label">
                    Severity
                    <select value={formSeverity()} onChange={(e) => setFormSeverity(e.currentTarget.value as RuleSeverity)} class="form-select">
                      <option value="critical">Critical</option>
                      <option value="warning">Warning</option>
                      <option value="info">Info</option>
                    </select>
                  </label>
                </div>
                <label class="form-label">
                  Query (PromQL)
                  <textarea value={formQuery()} onInput={(e) => setFormQuery(e.currentTarget.value)} placeholder="e.g. histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))" rows={3} class="form-textarea" style={{ "font-family": "monospace", "font-size": "13px" }} />
                </label>
                <div style={{ display: "grid", "grid-template-columns": "1fr 1fr 1fr", gap: "12px" }}>
                  <label class="form-label">
                    Condition
                    <select value={formCondition()} onChange={(e) => setFormCondition(e.currentTarget.value)} class="form-select">
                      <option value="gt">&gt; (greater than)</option>
                      <option value="lt">&lt; (less than)</option>
                      <option value="gte">&gt;= (greater or equal)</option>
                      <option value="lte">&lt;= (less or equal)</option>
                      <option value="eq">= (equal)</option>
                      <option value="neq">!= (not equal)</option>
                    </select>
                  </label>
                  <label class="form-label">
                    Threshold
                    <Input type="number" value={String(formThreshold())} onInput={(e) => setFormThreshold(Number(e.currentTarget.value))} />
                  </label>
                  <label class="form-label">
                    For duration
                    <Input value={formFor()} onInput={(e) => setFormFor(e.currentTarget.value)} placeholder="e.g. 5m" />
                  </label>
                </div>
                <div style={{ display: "flex", gap: "8px", "justify-content": "space-between", "padding-top": "8px" }}>
                  <Button onClick={() => setCreateStep(0)}>Back</Button>
                  <Button variant="primary" onClick={() => setCreateStep(2)}>Next: Notifications</Button>
                </div>
              </div>
            </Show>

            {/* Step 2: Notifications */}
            <Show when={createStep() === 2}>
              <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
                <div style={{ background: "var(--surface)", padding: "12px", "border-radius": "8px", "font-size": "13px" }}>
                  <div style={{ display: "flex", gap: "8px", "flex-wrap": "wrap", "margin-bottom": "8px" }}>
                    <Badge tone={SEVERITY_TONE[formSeverity()]}>{formSeverity()}</Badge>
                    <Badge tone="neutral">{RULE_TYPE_LABELS[formType()]}</Badge>
                    <Badge tone="neutral">{CONDITION_LABELS[formCondition()] || formCondition()} {formThreshold()}</Badge>
                    <Badge tone="neutral">for {formFor()}</Badge>
                  </div>
                  <strong>{formName() || "Untitled Monitor"}</strong>
                  <Show when={formDesc()}>
                    <div style={{ color: "var(--text-muted)", "margin-top": "4px" }}>{formDesc()}</div>
                  </Show>
                  <div style={{ "margin-top": "8px" }}>
                    <code style={{ "font-size": "12px", color: "var(--accent)", "word-break": "break-all" }}>{formQuery()}</code>
                  </div>
                </div>
                <label class="form-label">
                  Notify channels (comma-separated)
                  <Input value={formChannels()} onInput={(e) => setFormChannels(e.currentTarget.value)} placeholder="e.g. #incidents-critical, PagerDuty, #slack-alerts" />
                </label>
                <div style={{ display: "flex", gap: "8px", "flex-wrap": "wrap" }}>
                  <For each={["#alerts", "#incidents-critical", "#infra-alerts", "PagerDuty", "Email", "#slo-alerts"]}>
                    {(ch) => (
                      <button
                        style={{
                          background: formChannels().includes(ch) ? "var(--accent)" : "var(--surface)",
                          color: formChannels().includes(ch) ? "#000" : "var(--text)",
                          border: "1px solid var(--border)", "border-radius": "4px",
                          padding: "4px 10px", "font-size": "12px", cursor: "pointer",
                        }}
                        onClick={() => {
                          const current = formChannels().split(",").map((s) => s.trim()).filter(Boolean);
                          if (current.includes(ch)) {
                            setFormChannels(current.filter((c) => c !== ch).join(", "));
                          } else {
                            setFormChannels([...current, ch].join(", "));
                          }
                        }}
                      >
                        {ch}
                      </button>
                    )}
                  </For>
                </div>
                <label style={{ display: "flex", "align-items": "center", gap: "8px", "font-size": "13px", cursor: "pointer" }}>
                  <input type="checkbox" checked={formEnabled()} onChange={(e) => setFormEnabled(e.currentTarget.checked)} />
                  Enable monitor immediately
                </label>
                <div style={{ display: "flex", gap: "8px", "justify-content": "space-between", "padding-top": "8px" }}>
                  <Button onClick={() => setCreateStep(1)}>Back</Button>
                  <Button variant="primary" onClick={saveRule}>Create Monitor</Button>
                </div>
              </div>
            </Show>
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
  formDesc: () => string; setFormDesc: (v: string) => void;
  formType: () => RuleType; setFormType: (v: RuleType) => void;
  formQuery: () => string; setFormQuery: (v: string) => void;
  formCondition: () => string; setFormCondition: (v: string) => void;
  formThreshold: () => number; setFormThreshold: (v: number) => void;
  formSeverity: () => RuleSeverity; setFormSeverity: (v: RuleSeverity) => void;
  formFor: () => string; setFormFor: (v: string) => void;
  formChannels: () => string; setFormChannels: (v: string) => void;
  formEnabled: () => boolean; setFormEnabled: (v: boolean) => void;
}) {
  return (
    <div style={{ display: "flex", "flex-direction": "column", gap: "12px" }}>
      <label class="form-label">
        Name
        <Input value={props.formName()} onInput={(e) => props.setFormName(e.currentTarget.value)} placeholder="e.g. Checkout P99 Latency" />
      </label>
      <label class="form-label">
        Description
        <Input value={props.formDesc()} onInput={(e) => props.setFormDesc(e.currentTarget.value)} placeholder="What does this monitor check?" />
      </label>
      <div style={{ display: "grid", "grid-template-columns": "1fr 1fr", gap: "12px" }}>
        <label class="form-label">
          Type
          <select value={props.formType()} onChange={(e) => props.setFormType(e.currentTarget.value as RuleType)} class="form-select">
            <option value="threshold">Threshold</option>
            <option value="anomaly">Anomaly</option>
            <option value="change">Change</option>
            <option value="absence">Absence</option>
            <option value="composite">Composite</option>
          </select>
        </label>
        <label class="form-label">
          Severity
          <select value={props.formSeverity()} onChange={(e) => props.setFormSeverity(e.currentTarget.value as RuleSeverity)} class="form-select">
            <option value="critical">Critical</option>
            <option value="warning">Warning</option>
            <option value="info">Info</option>
          </select>
        </label>
      </div>
      <label class="form-label">
        Query (PromQL)
        <textarea value={props.formQuery()} onInput={(e) => props.setFormQuery(e.currentTarget.value)} placeholder="e.g. histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))" rows={3} class="form-textarea" />
      </label>
      <div style={{ display: "grid", "grid-template-columns": "1fr 1fr 1fr", gap: "12px" }}>
        <label class="form-label">
          Condition
          <select value={props.formCondition()} onChange={(e) => props.setFormCondition(e.currentTarget.value)} class="form-select">
            <option value="gt">&gt; (greater than)</option>
            <option value="lt">&lt; (less than)</option>
            <option value="gte">&gt;= (greater or equal)</option>
            <option value="lte">&lt;= (less or equal)</option>
            <option value="eq">= (equal)</option>
            <option value="neq">!= (not equal)</option>
          </select>
        </label>
        <label class="form-label">
          Threshold
          <Input type="number" value={String(props.formThreshold())} onInput={(e) => props.setFormThreshold(Number(e.currentTarget.value))} />
        </label>
        <label class="form-label">
          For duration
          <Input value={props.formFor()} onInput={(e) => props.setFormFor(e.currentTarget.value)} placeholder="e.g. 5m" />
        </label>
      </div>
      <label class="form-label">
        Notify channels (comma-separated)
        <Input value={props.formChannels()} onInput={(e) => props.setFormChannels(e.currentTarget.value)} placeholder="e.g. #incidents-critical, PagerDuty" />
      </label>
      <label style={{ display: "flex", "align-items": "center", gap: "8px", "font-size": "13px", cursor: "pointer" }}>
        <input type="checkbox" checked={props.formEnabled()} onChange={(e) => props.setFormEnabled(e.currentTarget.checked)} />
        Enabled
      </label>
      <div style={{ display: "flex", gap: "8px", "justify-content": "flex-end", "padding-top": "8px" }}>
        <Button onClick={props.onCancel}>Cancel</Button>
        <Button variant="primary" onClick={props.onSave}>Save Monitor</Button>
      </div>
    </div>
  );
}

export default MonitorsPage;
