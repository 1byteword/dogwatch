import { ErrorBoundary, For, Show, createMemo, createResource, createSignal } from "solid-js";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Panel } from "../design/components/Panel";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";
import { createNowTicker, useAutoRefresh } from "../core/live";
import { relativeTime } from "../core/time";
import {
  loadK8sDeployments,
  loadK8sEvents,
  loadK8sNamespaces,
  loadK8sPods,
  loadK8sServices,
  loadK8sSummary
} from "../domains/kubernetes/service";

function statusTone(status: string) {
  const s = status.toLowerCase();
  if (s.includes("fail") || s.includes("critical") || s.includes("warning") || s.includes("degraded")) return "error";
  if (s.includes("pending") || s.includes("progress")) return "warn";
  return "ok";
}

export function ImproveKubernetesPage() {
  const [namespace, setNamespace] = createSignal("");
  const [lastUpdatedAt, setLastUpdatedAt] = createSignal(Date.now());
  const now = createNowTicker(15000);

  const [summary, { refetch: refetchSummary }] = createResource(loadK8sSummary);
  const [namespaces, { refetch: refetchNamespaces }] = createResource(loadK8sNamespaces);

  const ns = createMemo(() => namespace());
  const [pods, { refetch: refetchPods }] = createResource(ns, loadK8sPods);
  const [deployments, { refetch: refetchDeployments }] = createResource(ns, loadK8sDeployments);
  const [services, { refetch: refetchServices }] = createResource(ns, loadK8sServices);
  const [events, { refetch: refetchEvents }] = createResource(ns, (value) => loadK8sEvents(value, 40));
  const hasError = createMemo(
    () => Boolean(summary.error || namespaces.error || pods.error || deployments.error || services.error || events.error)
  );

  useAutoRefresh(() => {
    refetchSummary();
    refetchNamespaces();
    refetchPods();
    refetchDeployments();
    refetchServices();
    refetchEvents();
    setLastUpdatedAt(Date.now());
  }, 30000);

  const updatedAgo = createMemo(() => {
    now();
    return relativeTime(new Date(lastUpdatedAt()));
  });

  function refreshAll() {
    refetchSummary();
    refetchNamespaces();
    refetchPods();
    refetchDeployments();
    refetchServices();
    refetchEvents();
    setLastUpdatedAt(Date.now());
  }

  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
    <div class="page-grid">
      <Panel
        title="Cluster Summary"
        actions={
          <>
            <select class="input catalog-select" value={namespace()} onChange={(e) => setNamespace(e.currentTarget.value)}>
              <option value="">all namespaces</option>
              <For each={namespaces() || []}>{(nsItem) => <option value={nsItem.name}>{nsItem.name}</option>}</For>
            </select>
            <Button onClick={refreshAll}>Refresh</Button>
            <Badge tone="neutral">updated {updatedAgo()}</Badge>
          </>
        }
      >
        <Show when={hasError()}>
          <div class="inline-notice">Kubernetes data is unavailable. Check cluster connection/config.</div>
        </Show>
        <div class="catalog-stats-grid k8s-stats-grid">
          <div class="catalog-stat-card"><label>Nodes Ready</label><strong>{summary()?.nodesReady || 0}/{summary()?.nodes || 0}</strong></div>
          <div class="catalog-stat-card"><label>Pods Running</label><strong>{summary()?.podsRunning || 0}/{summary()?.pods || 0}</strong></div>
          <div class="catalog-stat-card"><label>Deployments</label><strong>{summary()?.deploymentsHealthy || 0}/{summary()?.deployments || 0}</strong></div>
          <div class="catalog-stat-card"><label>Warning Events</label><strong>{summary()?.warningEvents || 0}</strong></div>
        </div>
      </Panel>

      <Panel title="Pods" actions={<Badge tone="neutral">{pods()?.length || 0} total</Badge>}>
        <Show when={!pods.loading} fallback={<div class="paragraph">Loading pods…</div>}>
          <div class="dense-table">
            <div class="dense-head pods-grid">
              <span>Pod</span><span>Status</span><span>Node</span><span>Restarts</span>
            </div>
            <div class="dense-body">
              <For each={pods() || []}>
                {(pod) => (
                  <div class="dense-row pods-grid">
                    <span>{pod.name}</span>
                    <span><Badge tone={statusTone(pod.status)}>{pod.status}</Badge></span>
                    <span>{pod.nodeName || "-"}</span>
                    <span class="mono">{pod.restartCount}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Panel>

      <Panel title="Deployments + Services" actions={<Badge tone="ok">workload health</Badge>}>
        <div class="detail-stack">
          <div class="dense-table">
            <div class="dense-head deploy-grid"><span>Deployment</span><span>Status</span><span>Ready</span></div>
            <div class="dense-body short-body">
              <For each={deployments() || []}>
                {(dep) => (
                  <div class="dense-row deploy-grid">
                    <span>{dep.name}</span>
                    <span><Badge tone={statusTone(dep.status)}>{dep.status}</Badge></span>
                    <span class="mono">{dep.readyReplicas}/{dep.replicas}</span>
                  </div>
                )}
              </For>
            </div>
          </div>

          <div class="dense-table">
            <div class="dense-head svc-grid"><span>Service</span><span>Type</span><span>Endpoints</span></div>
            <div class="dense-body short-body">
              <For each={services() || []}>
                {(svc) => (
                  <div class="dense-row svc-grid">
                    <span>{svc.name}</span>
                    <span>{svc.type}</span>
                    <span class="mono">{svc.endpointCount}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </div>
      </Panel>

      <Panel title="Recent Events" actions={<Badge tone="warn">{events()?.length || 0} events</Badge>}>
        <Show when={!events.loading} fallback={<div class="paragraph">Loading events…</div>}>
          <div class="dense-table">
            <div class="dense-head evt-grid"><span>Time</span><span>Type</span><span>Reason</span><span>Message</span></div>
            <div class="dense-body short-body">
              <For each={events() || []}>
                {(evt) => (
                  <div class="dense-row evt-grid">
                    <span class="mono">{evt.lastTimestamp ? new Date(evt.lastTimestamp).toLocaleTimeString() : "-"}</span>
                    <span><Badge tone={evt.type === "Warning" ? "error" : "neutral"}>{evt.type}</Badge></span>
                    <span>{evt.reason}</span>
                    <span class="truncate-text">{evt.message}</span>
                  </div>
                )}
              </For>
            </div>
          </div>
        </Show>
      </Panel>
    </div>
    </ErrorBoundary>
  );
}

export default ImproveKubernetesPage;
