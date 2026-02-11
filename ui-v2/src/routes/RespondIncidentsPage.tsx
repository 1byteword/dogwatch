import { For, Show, createEffect, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { loadIncidents } from "../domains/incidents/service";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import { ackIncident, addIncidentNote, assignIncident, createIncident, resolveIncident } from "../core/actions";
import { loadOncallPolicies } from "../domains/oncall/service";

function eventTone(kind: string) {
  if (kind === "alert") return "error";
  if (kind === "deploy") return "warn";
  return "neutral";
}

export function RespondIncidentsPage() {
  const [incidents, { refetch, mutate }] = createResource(loadIncidents);
  const [selectedId, setSelectedId] = createSignal("");
  const [notice, setNotice] = createSignal<string>("");
  const [showCreateIncident, setShowCreateIncident] = createSignal(false);
  const [showNoteModal, setShowNoteModal] = createSignal(false);
  const [showAssignModal, setShowAssignModal] = createSignal(false);
  const [showEscalateModal, setShowEscalateModal] = createSignal(false);
  const [incidentTitle, setIncidentTitle] = createSignal("");
  const [incidentSeverity, setIncidentSeverity] = createSignal<"critical" | "high" | "medium" | "low">("high");
  const [incidentService, setIncidentService] = createSignal("");
  const [incidentDescription, setIncidentDescription] = createSignal("");
  const [noteText, setNoteText] = createSignal("");
  const [assignee, setAssignee] = createSignal("");
  const [selectedPolicyId, setSelectedPolicyId] = createSignal("");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal<number>(Date.now());
  const now = createNowTicker(15000);
  const [policies] = createResource(loadOncallPolicies);

  createEffect(() => {
    const firstId = incidents()?.[0]?.id;
    if (!selectedId() && firstId) setSelectedId(firstId);
  });

  createEffect(() => {
    if (incidents()) setLastUpdatedAt(Date.now());
  });

  useAutoRefresh(() => refetch(), 20000);

  const selected = createMemo(() => incidents()?.find((i) => i.id === selectedId()) ?? null);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  async function onAck(id: string, user = "v2-ui") {
    const prev = incidents() || [];
    mutate((curr) =>
      (curr || []).map((i) => (i.id === id ? { ...i, status: "acknowledged", commander: user, responders: [user] } : i))
    );
    try {
      await ackIncident(id, user);
      setNotice("Incident acknowledged.");
      refetch();
    } catch {
      mutate(() => prev);
      setNotice("Failed to acknowledge incident.");
    }
  }

  async function onResolve(id: string) {
    const prev = incidents() || [];
    mutate((curr) => (curr || []).filter((i) => i.id !== id));
    try {
      await resolveIncident(id);
      setNotice("Incident resolved.");
      refetch();
    } catch {
      mutate(() => prev);
      setNotice("Failed to resolve incident.");
    }
  }

  function moveSelection(step: number) {
    const rows = incidents() || [];
    if (!rows.length) return;
    const curr = rows.findIndex((i) => i.id === selectedId());
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
    if (e.key === "r" && selected()) {
      e.preventDefault();
      onResolve(selected()!.id);
      return;
    }
    if (e.key === "n") {
      e.preventDefault();
      openCreateIncident();
    }
  }

  function openCreateIncident() {
    setIncidentTitle("");
    setIncidentSeverity("high");
    setIncidentService("");
    setIncidentDescription("");
    setShowCreateIncident(true);
  }

  async function submitIncident() {
    if (!incidentTitle().trim()) {
      setNotice("Incident title is required.");
      return;
    }
    try {
      await createIncident({
        title: incidentTitle().trim(),
        severity: incidentSeverity(),
        service: incidentService().trim(),
        description: incidentDescription().trim()
      });
      setShowCreateIncident(false);
      setNotice("Incident created.");
      refetch();
    } catch {
      setNotice("Failed to create incident.");
    }
  }

  async function submitStatusNote() {
    const current = selected();
    if (!current || !noteText().trim()) {
      setNotice("Status note is required.");
      return;
    }
    try {
      await addIncidentNote(current.id, noteText().trim(), "v2-ui");
      setShowNoteModal(false);
      setNoteText("");
      setNotice("Status update posted.");
      refetch();
    } catch {
      setNotice("Failed to post status update.");
    }
  }

  async function submitAssign() {
    const current = selected();
    if (!current || !assignee().trim()) {
      setNotice("Responder is required.");
      return;
    }
    try {
      await assignIncident(current.id, assignee().trim(), "v2-ui");
      setShowAssignModal(false);
      setAssignee("");
      setNotice("Responder assigned.");
      refetch();
    } catch {
      setNotice("Failed to assign responder.");
    }
  }

  async function submitEscalation() {
    const current = selected();
    if (!current || !selectedPolicyId()) {
      setNotice("Escalation policy is required.");
      return;
    }
    try {
      const res = await fetch("/api/oncall/escalate", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ incident_id: current.id, policy_id: selectedPolicyId() })
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setShowEscalateModal(false);
      setNotice("Escalation triggered.");
    } catch {
      setNotice("Failed to trigger escalation.");
    }
  }

  return (
    <>
      <div class="split-layout">
        <Panel
          title="Incidents"
          actions={
            <>
              <Button onClick={() => setShowEscalateModal(true)}>Escalation Policy</Button>
              <Button variant="primary" onClick={openCreateIncident}>New Incident</Button>
              <Button onClick={() => refetch()}>Refresh</Button>
              <Badge tone="neutral">updated {updatedAgo()}</Badge>
            </>
          }
        >
          <Show when={!incidents.loading} fallback={<div class="paragraph">Loading incidents…</div>}>
            <div class="alert-list" tabindex={0} onKeyDown={onListKeyDown}>
              <For each={incidents()}>
                {(incident) => (
                  <button
                    class={`alert-row${selectedId() === incident.id ? " is-selected" : ""}`}
                    onClick={() => setSelectedId(incident.id)}
                  >
                    <div class="alert-row-main">
                      <strong>{incident.title}</strong>
                      <span>{incident.service}</span>
                    </div>
                    <div class="alert-row-meta">
                      <Badge tone={incident.severity === "critical" ? "error" : "warn"}>
                        {incident.severity}
                      </Badge>
                      <Badge tone={incident.status === "triggered" ? "error" : "ok"}>
                        {incident.status}
                      </Badge>
                      <span>{incident.startedAtRaw ? relativeTime(incident.startedAtRaw) : incident.startedAt}</span>
                    </div>
                  </button>
                )}
              </For>
            </div>
          </Show>
        </Panel>

        <Panel title="Incident Command Center" actions={<Badge tone="ok">Core Flow</Badge>}>
          {selected() ? (
            <div class="detail-stack">
              <h3>{selected()!.title}</h3>
              <div class="kv-grid">
                <div>
                  <label>Commander</label>
                  <div>{selected()!.commander}</div>
                </div>
                <div>
                  <label>Responders</label>
                  <div>{selected()!.responders.join(", ")}</div>
                </div>
                <div>
                  <label>Service</label>
                  <div>{selected()!.service}</div>
                </div>
              </div>
              <div class="timeline-block">
                <label>Timeline</label>
                <ul>
                  <For each={selected()!.timeline}>
                    {(evt) => (
                      <li>
                        <Badge tone={eventTone(evt.kind)}>{evt.kind}</Badge>
                        <span class="mono timeline-time">{evt.time}</span>
                        <span>{evt.summary}</span>
                      </li>
                    )}
                  </For>
                </ul>
              </div>
              <div class="row">
                <Button variant="primary" onClick={() => onAck(selected()!.id)}>Acknowledge</Button>
                <Button onClick={() => setShowNoteModal(true)}>Status Update</Button>
                <Button onClick={() => setShowAssignModal(true)}>Assign Responder</Button>
                <Button variant="danger" onClick={() => onResolve(selected()!.id)}>Resolve</Button>
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
              <Input
                value={incidentService()}
                onInput={(e) => setIncidentService(e.currentTarget.value)}
                placeholder="Service"
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

      <Show when={showNoteModal()}>
        <div class="modal-overlay" onClick={() => setShowNoteModal(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Status Update</h3>
            <div class="detail-stack">
              <textarea
                class="input modal-textarea"
                value={noteText()}
                onInput={(e) => setNoteText(e.currentTarget.value)}
                placeholder="Write an update for the timeline"
              />
              <div class="row">
                <Button onClick={() => setShowNoteModal(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitStatusNote}>Post Update</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>

      <Show when={showAssignModal()}>
        <div class="modal-overlay" onClick={() => setShowAssignModal(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Assign Responder</h3>
            <div class="detail-stack">
              <Input value={assignee()} onInput={(e) => setAssignee(e.currentTarget.value)} placeholder="Responder username" />
              <div class="row">
                <Button onClick={() => setShowAssignModal(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitAssign}>Assign</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>

      <Show when={showEscalateModal()}>
        <div class="modal-overlay" onClick={() => setShowEscalateModal(false)}>
          <div class="modal-card" onClick={(e) => e.stopPropagation()}>
            <h3>Trigger Escalation</h3>
            <div class="detail-stack">
              <select class="input" value={selectedPolicyId()} onChange={(e) => setSelectedPolicyId(e.currentTarget.value)}>
                <option value="">Select policy</option>
                <For each={policies() || []}>{(policy) => <option value={policy.id}>{policy.name}</option>}</For>
              </select>
              <div class="row">
                <Button onClick={() => setShowEscalateModal(false)}>Cancel</Button>
                <Button variant="primary" onClick={submitEscalation}>Escalate</Button>
              </div>
            </div>
          </div>
        </div>
      </Show>
    </>
  );
}

export default RespondIncidentsPage;
