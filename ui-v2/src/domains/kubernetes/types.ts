export interface K8sSummary {
  nodes: number;
  nodesReady: number;
  namespaces: number;
  pods: number;
  podsRunning: number;
  podsPending: number;
  podsFailed: number;
  deployments: number;
  deploymentsHealthy: number;
  services: number;
  warningEvents: number;
}

export interface K8sNamespace {
  name: string;
}

export interface K8sPod {
  name: string;
  namespace: string;
  status: string;
  nodeName: string;
  restartCount: number;
  createdAt?: string;
}

export interface K8sDeployment {
  name: string;
  namespace: string;
  status: string;
  replicas: number;
  readyReplicas: number;
  updatedReplicas: number;
}

export interface K8sServiceItem {
  name: string;
  namespace: string;
  type: string;
  clusterIP: string;
  endpointCount: number;
}

export interface K8sEvent {
  name: string;
  namespace: string;
  type: string;
  reason: string;
  message: string;
  objectKind: string;
  objectName: string;
  lastTimestamp?: string;
}
