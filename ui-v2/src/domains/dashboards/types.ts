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
  locked?: boolean;
}

export interface DashboardItem {
  id: string;
  name: string;
  layout: DashboardWidgetPosition[];
  widgetConfig: Record<string, DashboardWidgetConfig>;
  isDefault: boolean;
  created?: string;
  updated?: string;
}
