import type { DashboardWidgetConfig, DashboardWidgetPosition, DashboardVariable } from "../../domains/dashboards/types";

export const DEFAULT_DASHBOARD_NAME = "Operations Command";
export const EDIT_MODE_PREF_KEY = "dogwatch-v2-dashboard-edit-mode";
export const DRAFT_STORAGE_KEY = "dogwatch-v2-dashboard-draft";
export const DRAFT_MAX_AGE_MS = 24 * 60 * 60 * 1000;
export const DRAFT_DEBOUNCE_MS = 2000;
export const MAX_HISTORY = 50;

export type EditorSnapshot = {
  layout: DashboardWidgetPosition[];
  widgetConfig: Record<string, DashboardWidgetConfig>;
};

export type CopiedWidget = {
  baseId: string;
  w: number;
  h: number;
  config: DashboardWidgetConfig;
};

export type ResizeAxis = "e" | "s" | "se";

export type DashboardTemplate = {
  id: string;
  name: string;
  description: string;
  layout: DashboardWidgetPosition[];
  widgetConfig?: Record<string, DashboardWidgetConfig>;
  variables?: DashboardVariable[];
};

export const DEFAULT_VARIABLES: DashboardVariable[] = [
  { name: "service", label: "Service", type: "service", options: [], defaultValue: "", current: "" },
  { name: "environment", label: "Environment", type: "custom", options: ["prod", "staging", "dev"], defaultValue: "prod", current: "prod" },
];

export const defaultLayout: DashboardWidgetPosition[] = [
  { id: "kpi-reliability", x: 0, y: 0, w: 4, h: 2 },
  { id: "alerts-feed", x: 4, y: 0, w: 4, h: 2 },
  { id: "incidents-live", x: 8, y: 0, w: 4, h: 2 },
  { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
  { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
  { id: "service-health", x: 0, y: 4, w: 4, h: 2 },
  { id: "oncall-now", x: 4, y: 4, w: 4, h: 2 },
  { id: "k8s-cluster", x: 8, y: 4, w: 4, h: 2 },
  { id: "notify-delivery", x: 0, y: 6, w: 4, h: 2 },
  { id: "command-links", x: 4, y: 6, w: 8, h: 2 }
];

export const DASHBOARD_TEMPLATES: DashboardTemplate[] = [
  {
    id: "executive-ops",
    name: "Executive Ops",
    description: "Topline reliability, incidents, and delivery health.",
    layout: [
      { id: "kpi-reliability", x: 0, y: 0, w: 4, h: 2 },
      { id: "alerts-severity-map", x: 4, y: 0, w: 4, h: 2 },
      { id: "incidents-live", x: 8, y: 0, w: 4, h: 2 },
      { id: "service-health", x: 0, y: 2, w: 4, h: 2 },
      { id: "notify-delivery", x: 4, y: 2, w: 4, h: 2 },
      { id: "ops-action-queue", x: 8, y: 2, w: 4, h: 2 },
      { id: "command-links", x: 0, y: 4, w: 12, h: 2 }
    ]
  },
  {
    id: "incident-war-room",
    name: "Incident War Room",
    description: "Fast triage and ownership during active incidents.",
    layout: [
      { id: "incidents-live", x: 0, y: 0, w: 4, h: 2 },
      { id: "alerts-feed", x: 4, y: 0, w: 4, h: 2 },
      { id: "incidents-by-commander", x: 8, y: 0, w: 4, h: 2 },
      { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
      { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
      { id: "ops-action-queue", x: 0, y: 4, w: 8, h: 2 },
      { id: "oncall-now", x: 8, y: 4, w: 4, h: 2 }
    ]
  },
  {
    id: "platform-sre",
    name: "Platform SRE",
    description: "Infra capacity, service health, and paging pressure.",
    layout: [
      { id: "k8s-cluster", x: 0, y: 0, w: 4, h: 2 },
      { id: "k8s-capacity-risk", x: 4, y: 0, w: 4, h: 2 },
      { id: "oncall-now", x: 8, y: 0, w: 4, h: 2 },
      { id: "service-health", x: 0, y: 2, w: 4, h: 2 },
      { id: "service-latency-top", x: 4, y: 2, w: 4, h: 2 },
      { id: "notify-failure-log", x: 8, y: 2, w: 4, h: 2 },
      { id: "k8s-pods", x: 0, y: 4, w: 6, h: 2 },
      { id: "k8s-events", x: 6, y: 4, w: 6, h: 2 },
      { id: "logs-errors", x: 0, y: 6, w: 8, h: 2 },
      { id: "notify-delivery", x: 8, y: 6, w: 4, h: 2 }
    ]
  },
  {
    id: "service-owner",
    name: "Service Owner",
    description: "Service quality, deployments, and customer-facing risk.",
    layout: [
      { id: "service-health", x: 0, y: 0, w: 4, h: 2 },
      { id: "service-latency-top", x: 4, y: 0, w: 4, h: 2 },
      { id: "alerts-feed", x: 8, y: 0, w: 4, h: 2 },
      { id: "logs-errors", x: 0, y: 2, w: 6, h: 2 },
      { id: "deploy-correlation", x: 6, y: 2, w: 6, h: 2 },
      { id: "deploy-stats", x: 0, y: 4, w: 4, h: 2 },
      { id: "alerts-severity-map", x: 4, y: 4, w: 4, h: 2 },
      { id: "ops-action-queue", x: 8, y: 4, w: 4, h: 2 },
      { id: "command-links", x: 0, y: 6, w: 12, h: 2 }
    ]
  },
  {
    id: "finops",
    name: "FinOps",
    description: "Cost intelligence, SLO health, and cardinality control.",
    layout: [
      { id: "cost-estimate", x: 0, y: 0, w: 4, h: 2 },
      { id: "slo-burn-rate", x: 4, y: 0, w: 4, h: 2 },
      { id: "slo-budget-remaining", x: 8, y: 0, w: 4, h: 2 },
      { id: "cardinality-hotspots", x: 0, y: 2, w: 6, h: 2 },
      { id: "cost-recommendations", x: 6, y: 2, w: 6, h: 2 },
      { id: "synthetic-uptime", x: 0, y: 4, w: 4, h: 2 },
      { id: "synthetic-failures", x: 4, y: 4, w: 8, h: 2 },
      { id: "cost-quick-wins", x: 0, y: 6, w: 6, h: 2 }
    ]
  },
  {
    id: "security-compliance",
    name: "Security & Compliance",
    description: "Audit trail, API key posture, and backup health.",
    layout: [
      { id: "admin-audit-feed", x: 0, y: 0, w: 6, h: 2 },
      { id: "admin-api-keys", x: 6, y: 0, w: 3, h: 2 },
      { id: "admin-backup-status", x: 9, y: 0, w: 3, h: 2 },
      { id: "alerts-watches", x: 0, y: 2, w: 4, h: 2 },
      { id: "alerts-silences", x: 4, y: 2, w: 4, h: 2 },
      { id: "perf-anomalies", x: 8, y: 2, w: 4, h: 2 }
    ]
  }
];
