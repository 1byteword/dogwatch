import { For } from "solid-js";
import { Badge } from "../../design/components/Badge";
import { Button } from "../../design/components/Button";
import type { DashboardWidgetPosition, DashboardVariable } from "../../domains/dashboards/types";

export function WidgetInspector(props: {
  widget: DashboardWidgetPosition;
  widgetSince: string;
  widgetService: string;
  widgetSeverity: string;
  isLocked: boolean;
  widgetTitle: string;
  timeRange: string;
  serviceFilter: string;
  severityFilter: string;
  serviceOptions: string[];
  dashboardVars: DashboardVariable[];
  onSetSince: (v: string) => void;
  onSetService: (v: string) => void;
  onSetSeverity: (v: string) => void;
  onToggleLock: () => void;
  onCopy: () => void;
  onDuplicate: () => void;
  onPaste: () => void;
  onClearScope: () => void;
  onRemove: () => void;
  canPaste: boolean;
}) {
  return (
    <section class="widget-inspector panel">
      <header class="panel-head">
        <h2>Widget Inspector</h2>
        <div class="panel-actions">
          <Badge tone="ok">{props.widgetTitle}</Badge>
        </div>
      </header>
      <div class="panel-body widget-inspector-body">
        <div class="widget-inspector-grid">
          <label>Time Scope</label>
          <select
            class="input widget-scope-select"
            value={props.widgetSince}
            onChange={(e) => props.onSetSince(e.currentTarget.value)}
          >
            <option value="">default time ({props.timeRange})</option>
            <option value="15m">last 15m</option>
            <option value="1h">last 1h</option>
            <option value="6h">last 6h</option>
            <option value="24h">last 24h</option>
          </select>
          <label>Service Scope</label>
          <select
            class="input widget-scope-select"
            value={props.widgetService}
            onChange={(e) => props.onSetService(e.currentTarget.value)}
          >
            <option value="">default service ({props.serviceFilter || "all"})</option>
            <For each={props.dashboardVars.filter((v) => v.type === "service" || v.type === "custom")}>
              {(v) => <option value={`$${v.name}`}>{"$"}{v.name} (variable)</option>}
            </For>
            <For each={props.serviceOptions}>
              {(svc) => <option value={svc}>{svc}</option>}
            </For>
          </select>
          <label>Severity Scope</label>
          <select
            class="input widget-scope-select"
            value={props.widgetSeverity}
            onChange={(e) => props.onSetSeverity(e.currentTarget.value)}
          >
            <option value="">default severity ({props.severityFilter || "all"})</option>
            <For each={props.dashboardVars.filter((v) => v.type === "severity" || v.type === "custom")}>
              {(v) => <option value={`$${v.name}`}>{"$"}{v.name} (variable)</option>}
            </For>
            <option value="critical">critical</option>
            <option value="high">high</option>
            <option value="medium">medium</option>
            <option value="low">low</option>
          </select>
        </div>
        <div class="row">
          <Badge tone="neutral">x:{props.widget.x} y:{props.widget.y} w:{props.widget.w} h:{props.widget.h}</Badge>
          <Button
            variant={props.isLocked ? "primary" : "default"}
            onClick={props.onToggleLock}
          >
            {props.isLocked ? "Unlock" : "Lock"}
          </Button>
          <Button onClick={props.onCopy}>Copy</Button>
          <Button onClick={props.onDuplicate}>Duplicate</Button>
          <Button onClick={props.onPaste} disabled={!props.canPaste}>Paste</Button>
          <Button onClick={props.onClearScope} disabled={props.isLocked}>
            Clear Scope
          </Button>
          <Button variant="danger" onClick={props.onRemove} disabled={props.isLocked}>
            Remove Widget
          </Button>
        </div>
      </div>
    </section>
  );
}
