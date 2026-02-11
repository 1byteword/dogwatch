import { For, Show, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Panel } from "../design/components/Panel";
import { loadCatalogServices } from "../domains/catalog/service";
import { formatDuration, loadDeployIncidentCorrelations, loadServiceTimeline } from "../domains/correlation/service";

function confidenceTone(confidence: number) {
  if (confidence >= 0.8) return "ok";
  if (confidence >= 0.6) return "warn";
  return "error";
}

function eventTone(type: string, severity: string) {
  if (severity === "critical" || severity === "error") return "error";
  if (severity === "high" || severity === "warn") return "warn";
  if (type === "deploy") return "ok";
  return "neutral";
}

export function CorrelateTimelinePage() {
  const [since, setSince] = createSignal("24h");
  const [service, setService] = createSignal("");

  const [services] = createResource(() => loadCatalogServices({}));
  const [correlations, { refetch: refetchCorrelations }] = createResource(since, loadDeployIncidentCorrelations);

  const timelineQuery = createMemo(() => ({
    service: service(),
    since: since()
  }));

  const [timeline, { refetch: refetchTimeline }] = createResource(timelineQuery, (q) => loadServiceTimeline(q.service, q.since));
  const hasError = createMemo(() => Boolean(correlations.error || timeline.error || services.error));

  function refreshAll() {
    refetchCorrelations();
    refetchTimeline();
  }

  return (
    <div class="page-grid">
      <Panel
        title="Deploy -> Incident Correlations"
        actions={
          <>
            <select class="input corr-select" value={since()} onChange={(e) => setSince(e.currentTarget.value)}>
              <option value="1h">1h</option>
              <option value="6h">6h</option>
              <option value="24h">24h</option>
              <option value="72h">72h</option>
            </select>
            <Button onClick={refreshAll}>Refresh</Button>
            <Badge tone="neutral">{correlations()?.length || 0} linked changes</Badge>
          </>
        }
      >
        <Show when={hasError()}>
          <div class="inline-notice">Correlation data is unavailable right now.</div>
        </Show>
        <Show when={!correlations.loading} fallback={<div class="paragraph">Loading correlations…</div>}>
          <div class="alert-list">
            <For each={correlations() || []}>
              {(corr) => (
                <div class="alert-row">
                  <div class="alert-row-main">
                    <strong>{corr.deployment.version}</strong>
                    <span>{corr.deployment.service}</span>
                  </div>
                  <div class="alert-row-meta">
                    <Badge tone={confidenceTone(corr.confidence)}>{Math.round(corr.confidence * 100)}% confidence</Badge>
                    <span>{formatDuration(corr.timeDeltaMs)} before incident</span>
                  </div>
                  <p class="paragraph">{corr.reason}</p>
                </div>
              )}
            </For>
          </div>
        </Show>
      </Panel>

      <Panel
        title="Service Event Timeline"
        actions={
          <>
            <select class="input corr-select" value={service()} onChange={(e) => setService(e.currentTarget.value)}>
              <option value="">select service</option>
              <For each={services() || []}>{(svc) => <option value={svc.name}>{svc.displayName}</option>}</For>
            </select>
            <Badge tone="warn">{timeline()?.summary.errorLogCount || 0} error logs</Badge>
            <Badge tone="neutral">{timeline()?.summary.totalEvents || 0} events</Badge>
          </>
        }
      >
        <Show when={service()} fallback={<div class="paragraph">Select a service to load timeline context.</div>}>
          <Show when={!timeline.loading} fallback={<div class="paragraph">Loading timeline…</div>}>
            <div class="timeline-block corr-timeline">
              <ul>
                <For each={timeline()?.events || []}>
                  {(evt) => (
                    <li>
                      <Badge tone={eventTone(evt.type, evt.severity)}>{evt.type}</Badge>
                      <span class="mono timeline-time">{new Date(evt.timestamp).toLocaleTimeString()}</span>
                      <span>{evt.summary}</span>
                    </li>
                  )}
                </For>
              </ul>
            </div>
          </Show>
        </Show>
      </Panel>
    </div>
  );
}

export default CorrelateTimelinePage;
