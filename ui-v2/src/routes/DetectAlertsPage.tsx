import { For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { AlertItem } from "../domains/alerts/types";
import { loadAlerts } from "../domains/alerts/service";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { ackAlert, createAlertRule, createIncident, silenceAlert } from "../core/actions";

function severityTone(severity: AlertItem["severity"]) {
  if (severity === "critical") return "error";
  if (severity === "high") return "warn";
  return "neutral";
}

export function DetectAlertsPage() {
  const [alerts, { refetch, mutate }] = createResource(loadAlerts);
  const [selectedId, setSelectedId] = createSignal("");
  const [notice, setNotice] = createSignal<string>("");
  const [showCreateIncident, setShowCreateIncident] = createSignal(false);
  const [showCreateRule, setShowCreateRule] = createSignal(false);
  const [incidentTitle, setIncidentTitle] = createSignal("");
  const [incidentSeverity, setIncidentSeverity] = createSignal<"critical" | "high" | "medium" | "low">("high");
  const [incidentDescription, setIncidentDescription] = createSignal("");
  const [query, setQuery] = createSignal("");
  const [severityFilter, setSeverityFilter] = createSignal("");
  const [ruleName, setRuleName] = createSignal("");
  const [ruleService, setRuleService] = createSignal("");
  const [ruleThreshold, setRuleThreshold] = createSignal("1");
  const [ruleSeverity, setRuleSeverity] = createSignal<"critical" | "warning" | "info">("warning");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal<number>(Date.now());
  const now = createNowTicker(15000);

  createEffect(() => {
    const firstId = alerts()?.[0]?.id;
    if (!selectedId() && firstId) setSelectedId(firstId);
  });

  createEffect(() => {
    if (alerts()) setLastUpdatedAt(Date.now());
  });

  useAutoRefresh(() => refetch(), 20000);

  const selected = createMemo(() => alerts()?.find((a) => a.id === selectedId()) ?? null);

  const filteredAlerts = createMemo(() => {
    const q = query().trim().toLowerCase();
    return (alerts() || []).filter((alert) => {
      if (severityFilter() && alert.severity !== severityFilter()) return false;
      if (!q) return true;
      const haystack = `${alert.name} ${alert.service} ${alert.trigger}`.toLowerCase();
      return haystack.includes(q);
    });
  });

  const selectedAge = createMemo(() => {
    now();
    return selected()?.startedAtRaw ? relativeTime(selected()!.startedAtRaw!) : selected()?.startedAt ?? "now";
  });

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  async function onAck(id: string) {
    const prev = alerts() || [];
    mutate((curr) => (curr || []).filter((a) => a.id !== id));
    try {
      await ackAlert(id);
      setNotice("Alert acknowledged.");
      refetch();
    } catch {
      mutate(() => prev);
      setNotice("Failed to acknowledge alert.");
    }
  }

  async function onSilence(id: string) {
    const prev = alerts() || [];
    mutate((curr) => (curr || []).filter((a) => a.id !== id));
    try {
      await silenceAlert(id, "30m");
      setNotice("Alert silenced for 30m.");
      refetch();
    } catch {
      mutate(() => prev);
      setNotice("Failed to silence alert.");
    }
  }

  function moveSelection(step: number) {
    const rows = alerts() || [];
    if (!rows.length) return;
    const curr = rows.findIndex((a) => a.id === selectedId());
    const start = curr < 0 ? 0 : curr;
    const next = Math.max(0, Math.min(rows.length - 1, start + step));
    setSelectedId(rows[next].id);
  }

  function onListKeyDown(e: KeyboardEvent) {
    if (e.key === "ArrowDown" || e.key === "j") {
      e.preventDefault();
      moveSelection(1);
      return;
    }
    if (e.key === "ArrowUp" || e.key === "k") {
      e.preventDefault();
      moveSelection(-1);
      return;
    }
    if (e.key === "a" && selected()) {
      e.preventDefault();
      onAck(selected()!.id);
      return;
    }
    if (e.key === "s" && selected()) {
      e.preventDefault();
      onSilence(selected()!.id);
      return;
    }
    if (e.key === "i") {
      e.preventDefault();
      openCreateIncident();
    }
  }

  function openCreateIncident() {
    const current = selected();
    setIncidentTitle(current ? `Investigate: ${current.name}` : "");
    setIncidentDescription(current ? `Source alert: ${current.name}\nService: ${current.service}` : "");
    setIncidentSeverity(current?.severity === "critical" ? "critical" : current?.severity === "high" ? "high" : "medium");
    setShowCreateIncident(true);
  }

  function openCreateRule() {
    const current = selected();
    setRuleName(current ? `${current.service} error threshold` : "");
    setRuleService(current?.service || "");
    setRuleThreshold("1");
    setRuleSeverity("warning");
    setShowCreateRule(true);
  }

  async function submitIncident() {
    const current = selected();
    if (!incidentTitle().trim()) {
      setNotice("Incident title is required.");
      return;
    }
    try {
      await createIncident({
        title: incidentTitle().trim(),
        severity: incidentSeverity(),
        service: current?.service || "",
        description: incidentDescription().trim()
      });
      setNotice("Incident created.");
      setShowCreateIncident(false);
    } catch {
      setNotice("Failed to create incident.");
    }
  }

  async function submitRule() {
    if (!ruleName().trim()) {
      setNotice("Rule name is required.");
      return;
    }
    try {
      await createAlertRule({
        name: ruleName().trim(),
        description: `Auto-created from V2 (${ruleService().trim() || "global"})`,
        service: ruleService().trim(),
        severity: ruleSeverity(),
        threshold: Number(ruleThreshold()) || 1
      });
      setShowCreateRule(false);
      setNotice("Alert rule created.");
    } catch {
      setNotice("Failed to create alert rule.");
    }
  }

  return (
    <>
      <div class="split-layout">
        <Panel
          title="Alert Feed"
          actions={
            <>
              <Input
                class="inline-input"
                placeholder="Filter alerts..."
                value={query()}
                onInput={(e) => setQuery(e.currentTarget.value)}
              />
              <select class="input inline-select" value={severityFilter()} onChange={(e) => setSeverityFilter(e.currentTarget.value)}>
                <option value="">all severities</option>
                <option value="critical">critical</option>
                <option value="high">high</option>
                <option value="medium">medium</option>
                <option value="low">low</option>
              </select>
              <Button variant="primary" onClick={openCreateRule}>New Rule</Button>
              <Button onClick={() => refetch()}>Refresh</Button>
              <Badge tone="neutral">updated {updatedAgo()}</Badge>
            </>
          }
        >
          <Show when={!alerts.loading} fallback={<div class="paragraph">Loading alerts…</div>}>
            <div class="alert-list" tabindex={0} onKeyDown={onListKeyDown}>
              <For each={filteredAlerts()}>
                {(alert) => (
                  <button
                    class={`alert-row${selectedId() === alert.id ? " is-selected" : ""}`}
                    onClick={() => setSelectedId(alert.id)}
                  >
                    <div class="alert-row-main">
                      <strong>{alert.name}</strong>
                      <span>{alert.service}</span>
                    </div>
                    <div class="alert-row-meta">
                      <Badge tone={severityTone(alert.severity)}>{alert.severity}</Badge>
                      <Badge tone={alert.state === "firing" ? "error" : "warn"}>{alert.state}</Badge>
                      <span>{alert.startedAtRaw ? relativeTime(alert.startedAtRaw) : alert.startedAt}</span>
                    </div>
                  </button>
                )}
              </For>
            </div>
          </Show>
        </Panel>

        <Panel
          title="Triage Context"
          actions={<Badge tone={selected()?.state === "firing" ? "error" : "warn"}>{selected()?.state ?? "idle"}</Badge>}
        >
          {selected() ? (
            <div class="detail-stack">
              <h3>{selected()!.name}</h3>
              <p class="paragraph">{selected()!.trigger}</p>
              <div class="kv-grid">
                <div>
                  <label>Probable Cause</label>
                  <div>{selected()!.probableCause}</div>
                </div>
                <div>
                  <label>Recent Deploy</label>
                  <div>{selected()!.recentDeploy}</div>
                </div>
                <div>
                  <label>Trace Errors</label>
                  <div class="mono">{selected()!.traceErrors}</div>
                </div>
                <div>
                  <label>Started</label>
                  <div>{selectedAge()}</div>
                </div>
              </div>
              <div class="row">
                <Button variant="primary" onClick={() => onAck(selected()!.id)}>Acknowledge</Button>
                <Button onClick={openCreateIncident}>Open Incident</Button>
                <Button variant="danger" onClick={() => onSilence(selected()!.id)}>Silence 30m</Button>
              </div>
              <Show when={notice()}>
                <div class="inline-notice">{notice()}</div>
              </Show>
            </div>
          ) : null}
        </Panel>
      </div>

      <Show when={showCreateIncident()}>
        <div class="modal-overlay" onClick={() => setShowCreateIncident(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Create Incident</h3>
            <div class="detail-stack">
              <Input
                value={incidentTitle()}
                onInput={(e) => setIncidentTitle(e.currentTarget.value)}
                placeholder="Incident title"
              />
              <select
                class="input"
                value={incidentSeverity()}
                onChange={(e) =>
                  setIncidentSeverity(e.currentTarget.value as "critical" | "high" | "medium" | "low")
                }
              >
                <option value="critical">critical</option>
                <option value="high">high</option>
                <option value="medium">medium</option>
                <option value="low">low</option>
              </select>
              <textarea
                class="input modal-textarea"
                value={incidentDescription()}
                onInput={(e) => setIncidentDescription(e.currentTarget.value)}
                placeholder="Description"
              />
              <div class="row">
                <Button onClick={() => setShowCreateIncident(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitIncident}>Create Incident</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>

      <Show when={showCreateRule()}>
        <div class="modal-overlay" onClick={() => setShowCreateRule(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Create Alert Rule</h3>
            <div class="detail-stack">
              <Input value={ruleName()} onInput={(e) => setRuleName(e.currentTarget.value)} placeholder="Rule name" />
              <Input value={ruleService()} onInput={(e) => setRuleService(e.currentTarget.value)} placeholder="Service label (optional)" />
              <select
                class="input"
                value={ruleSeverity()}
                onChange={(e) => setRuleSeverity(e.currentTarget.value as "critical" | "warning" | "info")}
              >
                <option value="critical">critical</option>
                <option value="warning">warning</option>
                <option value="info">info</option>
              </select>
              <Input
                value={ruleThreshold()}
                onInput={(e) => setRuleThreshold(e.currentTarget.value)}
                placeholder="Threshold"
              />
              <div class="row">
                <Button onClick={() => setShowCreateRule(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitRule}>Create Rule</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </>
  );
}

export default DetectAlertsPage;
