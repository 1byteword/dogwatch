export interface DashboardWidgetPosition {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface DashboardWidgetConfig {
  service?: string;
  since?: string;
  severity?: string;
  locked?: boolean;
}

export type DashboardVariableType = "custom" | "service" | "severity" | "timerange";

export interface DashboardVariable {
  name: string;
  label: string;
  type: DashboardVariableType;
  options: string[];
  defaultValue: string;
  current: string;
}

export interface DashboardItem {
  id: string;
  name: string;
  layout: DashboardWidgetPosition[];
  widgetConfig: Record<string, DashboardWidgetConfig>;
  variables?: DashboardVariable[];
  isDefault: boolean;
  created?: string;
  updated?: string;
}
