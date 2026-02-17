import { ErrorBoundary, For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";
import { createOncallSchedule } from "../core/actions";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { loadCurrentOncall, loadOncallPolicies, loadOncallSchedules } from "../domains/oncall/service";

export function ImproveOncallPage() {
  const [selectedId, setSelectedId] = createSignal("");
  const [showNewSchedule, setShowNewSchedule] = createSignal(false);
  const [scheduleName, setScheduleName] = createSignal("");
  const [scheduleTimezone, setScheduleTimezone] = createSignal("UTC");
  const [responderName, setResponderName] = createSignal("");
  const [responderEmail, setResponderEmail] = createSignal("");
  const [notice, setNotice] = createSignal("");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal(Date.now());
  const now = createNowTicker(15000);

  const [schedules, { refetch: refetchSchedules }] = createResource(loadOncallSchedules);
  const [policies, { refetch: refetchPolicies }] = createResource(loadOncallPolicies);

  const selectedScheduleId = createMemo(() => selectedId());
  const [current, { refetch: refetchCurrent }] = createResource(selectedScheduleId, loadCurrentOncall);
  const hasError = createMemo(() => Boolean(schedules.error || policies.error || current.error));

  createEffect(() => {
    const first = schedules()?.[0]?.id;
    if (!selectedId() && first) setSelectedId(first);
  });

  createEffect(() => {
    if (schedules() || policies()) setLastUpdatedAt(Date.now());
  });

  useAutoRefresh(() => {
    refetchSchedules();
    refetchPolicies();
    refetchCurrent();
  }, 30000);

  const selectedSchedule = createMemo(() => (schedules() || []).find((s) => s.id === selectedId()) || null);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  function moveSelection(step: number) {
    const rows = schedules() || [];
    if (!rows.length) return;
    const curr = rows.findIndex((s) => s.id === selectedId());
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
    }
  }

  function refreshAll() {
    refetchSchedules();
    refetchPolicies();
    refetchCurrent();
    setLastUpdatedAt(Date.now());
  }

  function openNewSchedule() {
    setScheduleName("");
    setScheduleTimezone("UTC");
    setResponderName("");
    setResponderEmail("");
    setShowNewSchedule(true);
  }

  async function submitNewSchedule() {
    if (!scheduleName().trim() || !responderName().trim()) {
      setNotice("Schedule name and responder are required.");
      return;
    }
    try {
      await createOncallSchedule({
        name: scheduleName().trim(),
        timezone: scheduleTimezone().trim() || "UTC",
        responderName: responderName().trim(),
        responderEmail: responderEmail().trim()
      });
      setShowNewSchedule(false);
      setNotice("Schedule created.");
      refreshAll();
    } catch {
      setNotice("Failed to create schedule.");
    }
  }

  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
    <div class="split-layout">
      <Panel
        title="On-call Schedules"
        actions={
          <>
            <Button onClick={openNewSchedule}>New Schedule</Button>
            <Button onClick={refreshAll}>Refresh</Button>
            <Badge tone="neutral">updated {updatedAgo()}</Badge>
          </>
        }
      >
        <Show when={hasError()}>
          <div class="inline-notice">On-call data is unavailable. Check on-call backend configuration.</div>
        </Show>
        <Show when={!schedules.loading} fallback={<div class="paragraph">Loading schedules…</div>}>
          <div class="alert-list" tabindex={0} onKeyDown={onListKeyDown}>
            <For each={schedules() || []}>
              {(sched) => (
                <button
                  class={`alert-row${selectedId() === sched.id ? " is-selected" : ""}`}
                  onClick={() => setSelectedId(sched.id)}
                >
                  <div class="alert-row-main">
                    <strong>{sched.name}</strong>
                    <span>{sched.timezone}</span>
                  </div>
                  <div class="alert-row-meta">
                    <Badge tone="ok">{sched.layerCount} layers</Badge>
                    <Badge tone="neutral">{sched.memberCount} responders</Badge>
                  </div>
                </button>
              )}
            </For>
          </div>
        </Show>
      </Panel>

      <div class="detail-stack oncall-right-stack">
        <Panel title="Current Coverage" actions={<Badge tone={current() ? "ok" : "warn"}>{current() ? "active" : "missing"}</Badge>}>
          <Show when={selectedSchedule()} fallback={<div class="paragraph">No schedules configured.</div>}>
            <div class="detail-stack">
              <h3>{selectedSchedule()!.name}</h3>
              <p class="paragraph">{selectedSchedule()!.description || "No description set."}</p>
              <div class="kv-grid">
                <div>
                  <label>Timezone</label>
                  <div>{selectedSchedule()!.timezone}</div>
                </div>
                <div>
                  <label>Teams</label>
                  <div>{selectedSchedule()!.teams.length ? selectedSchedule()!.teams.join(", ") : "none"}</div>
                </div>
                <div>
                  <label>Layers</label>
                  <div>{selectedSchedule()!.layerCount}</div>
                </div>
              </div>
              <div class="oncall-current-card">
                <label>On duty now</label>
                <strong>{current()?.userName || "Unassigned"}</strong>
                <Show when={current()?.isOverride}>
                  <Badge tone="warn">override</Badge>
                </Show>
              </div>
            </div>
          </Show>
        </Panel>

        <Panel title="Escalation Policies" actions={<Badge tone="neutral">{policies()?.length || 0} total</Badge>}>
          <Show when={!policies.loading} fallback={<div class="paragraph">Loading policies…</div>}>
            <div class="policy-list">
              <For each={policies() || []}>
                {(policy) => (
                  <div class="policy-row">
                    <div>
                      <strong>{policy.name}</strong>
                      <p class="paragraph">{policy.description || "No description set."}</p>
                    </div>
                    <div class="row">
                      <Badge tone="warn">{policy.ruleCount} rules</Badge>
                      <Badge tone={policy.repeatEnabled ? "ok" : "neutral"}>
                        {policy.repeatEnabled ? "repeat" : "one-shot"}
                      </Badge>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Panel>
        <Show when={notice()}>
          <div class="inline-notice">{notice()}</div>
        </Show>
      </div>

      <Show when={showNewSchedule()}>
        <div class="modal-overlay" onClick={() => setShowNewSchedule(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>New On-call Schedule</h3>
            <div class="detail-stack">
              <Input value={scheduleName()} onInput={(e) => setScheduleName(e.currentTarget.value)} placeholder="Schedule name" />
              <Input value={scheduleTimezone()} onInput={(e) => setScheduleTimezone(e.currentTarget.value)} placeholder="Timezone (e.g. UTC)" />
              <Input value={responderName()} onInput={(e) => setResponderName(e.currentTarget.value)} placeholder="Primary responder" />
              <Input value={responderEmail()} onInput={(e) => setResponderEmail(e.currentTarget.value)} placeholder="Responder email (optional)" />
              <div class="row">
                <Button onClick={() => setShowNewSchedule(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitNewSchedule}>Create</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </div>
    </ErrorBoundary>
  );
}

export default ImproveOncallPage;
