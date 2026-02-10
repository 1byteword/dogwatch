import { api } from "../../core/api";
import { K8sContainer, K8sDeployment, K8sEvent, K8sNamespace, K8sPod, K8sServiceItem, K8sSummary } from "./types";

interface ApiK8sSummary {
  nodes?: number;
  nodes_ready?: number;
  namespaces?: number;
  pods?: number;
  pods_running?: number;
  pods_pending?: number;
  pods_failed?: number;
  deployments?: number;
  deployments_healthy?: number;
  services?: number;
  warning_events?: number;
}

function mapSummary(raw: ApiK8sSummary): K8sSummary {
  return {
    nodes: raw.nodes || 0,
    nodesReady: raw.nodes_ready || 0,
    namespaces: raw.namespaces || 0,
    pods: raw.pods || 0,
    podsRunning: raw.pods_running || 0,
    podsPending: raw.pods_pending || 0,
    podsFailed: raw.pods_failed || 0,
    deployments: raw.deployments || 0,
    deploymentsHealthy: raw.deployments_healthy || 0,
    services: raw.services || 0,
    warningEvents: raw.warning_events || 0
  };
}

export async function loadK8sSummary(): Promise<K8sSummary> {
  const raw = await api.get<ApiK8sSummary>("/api/k8s/summary");
  return mapSummary(raw);
}

export async function loadK8sNamespaces(): Promise<K8sNamespace[]> {
  const raw = await api.get<unknown>("/api/k8s/namespaces");
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => ({ name: String((item as { name?: string }).name || `ns-${idx}`) }));
}

export async function loadK8sPods(namespace: string): Promise<K8sPod[]> {
  const q = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
  const raw = await api.get<unknown>(`/api/k8s/pods${q}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const pod = item as Record<string, unknown>;
    return {
      name: String(pod.name || `pod-${idx}`),
      namespace: String(pod.namespace || "default"),
      status: String(pod.status || pod.phase || "unknown"),
      nodeName: String(pod.node_name || ""),
      restartCount: Number(pod.restart_count || 0),
      createdAt: pod.created_at ? String(pod.created_at) : undefined
    };
  });
}

export async function loadK8sDeployments(namespace: string): Promise<K8sDeployment[]> {
  const q = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
  const raw = await api.get<unknown>(`/api/k8s/deployments${q}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const dep = item as Record<string, unknown>;
    return {
      name: String(dep.name || `deployment-${idx}`),
      namespace: String(dep.namespace || "default"),
      status: String(dep.status || "unknown"),
      replicas: Number(dep.replicas || 0),
      readyReplicas: Number(dep.ready_replicas || 0),
      updatedReplicas: Number(dep.updated_replicas || 0)
    };
  });
}

export async function loadK8sServices(namespace: string): Promise<K8sServiceItem[]> {
  const q = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
  const raw = await api.get<unknown>(`/api/k8s/services${q}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const svc = item as Record<string, unknown>;
    return {
      name: String(svc.name || `service-${idx}`),
      namespace: String(svc.namespace || "default"),
      type: String(svc.type || "ClusterIP"),
      clusterIP: String(svc.cluster_ip || ""),
      endpointCount: Number(svc.endpoint_count || 0)
    };
  });
}

export async function loadK8sContainers(namespace: string): Promise<K8sContainer[]> {
  const q = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
  const raw = await api.get<unknown>(`/api/k8s/containers${q}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const c = item as Record<string, unknown>;
    return {
      name: String(c.name || `container-${idx}`),
      podName: String(c.pod_name || c.podName || ""),
      namespace: String(c.namespace || "default"),
      image: String(c.image || ""),
      status: String(c.status || "unknown"),
      restartCount: Number(c.restart_count || c.restartCount || 0),
      ready: Boolean(c.ready)
    };
  });
}

export async function loadK8sEvents(namespace: string, limit = 50): Promise<K8sEvent[]> {
  const params = new URLSearchParams();
  if (namespace) params.set("namespace", namespace);
  params.set("limit", String(limit));
  const raw = await api.get<unknown>(`/api/k8s/events?${params.toString()}`);
  if (!Array.isArray(raw)) return [];
  return raw.map((item, idx) => {
    const evt = item as Record<string, unknown>;
    return {
      name: String(evt.name || `event-${idx}`),
      namespace: String(evt.namespace || "default"),
      type: String(evt.type || "Normal"),
      reason: String(evt.reason || ""),
      message: String(evt.message || ""),
      objectKind: String(evt.object_kind || ""),
      objectName: String(evt.object_name || ""),
      lastTimestamp: evt.last_timestamp ? String(evt.last_timestamp) : undefined
    };
  });
}
