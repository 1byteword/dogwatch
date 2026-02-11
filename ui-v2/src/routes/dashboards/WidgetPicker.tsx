import { For, Show } from "solid-js";
import { Button } from "../../design/components/Button";
import { Input } from "../../design/components/Input";
import type { WidgetDef } from "./widgetDefs";
import { WIDGET_CATEGORIES } from "./widgetDefs";

export function WidgetPicker(props: {
  groups: { category: string; widgets: WidgetDef[] }[];
  query: string;
  category: string;
  onQueryChange: (v: string) => void;
  onCategoryChange: (v: string) => void;
  onAdd: (widgetId: string) => void;
  onClose: () => void;
  emptyResults: boolean;
}) {
  return (
    <div class="modal-overlay" onClick={props.onClose}>
      <div class="modal-card" onClick={(e) => e.stopPropagation()} style={{ "max-width": "720px" }}>
        <h3>Add Widget</h3>
        <div style={{ display: "flex", gap: "8px", "margin-bottom": "12px", "flex-wrap": "wrap" }}>
          <Input
            value={props.query}
            onInput={(e) => props.onQueryChange(e.currentTarget.value)}
            placeholder="Search widgets…"
            style={{ flex: "1", "min-width": "180px" }}
            aria-label="Search widgets"
          />
          <select class="form-select" value={props.category} onChange={(e) => props.onCategoryChange(e.currentTarget.value)} style={{ width: "160px" }}>
            <option value="">All Categories</option>
            <For each={WIDGET_CATEGORIES as unknown as string[]}>
              {(cat) => <option value={cat}>{cat}</option>}
            </For>
          </select>
        </div>
        <div style={{ "max-height": "60vh", overflow: "auto" }}>
          <For each={props.groups}>
            {(group) => (
              <div style={{ "margin-bottom": "16px" }}>
                <h4 style={{ margin: "0 0 8px", "font-size": "12px", "text-transform": "uppercase", "letter-spacing": "0.05em", color: "var(--text-muted)", "border-bottom": "1px solid var(--border)", "padding-bottom": "4px" }}>
                  {group.category} <span style={{ opacity: "0.5" }}>({group.widgets.length})</span>
                </h4>
                <div class="widget-picker-grid">
                  <For each={group.widgets}>
                    {(widget) => (
                      <button class="widget-picker-card" onClick={() => props.onAdd(widget.id)}>
                        <strong>{widget.title}</strong>
                        <p>{widget.description}</p>
                        <span class="mono">{widget.defaultW}x{widget.defaultH}</span>
                      </button>
                    )}
                  </For>
                </div>
              </div>
            )}
          </For>
        </div>
        <Show when={props.emptyResults}>
          <p class="paragraph">No widgets match that search.</p>
        </Show>
        <div class="row">
          <Button onClick={props.onClose}>Close</Button>
        </div>
      </div>
    </div>
  );
}
