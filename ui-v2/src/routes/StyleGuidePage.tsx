import { createMemo } from "solid-js";
import type uPlot from "uplot";
import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { ChartPanel } from "../design/components/ChartPanel";
import { Chip } from "../design/components/Chip";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";
import { Sparkline } from "../design/components/Sparkline";

function generateSineData(): uPlot.AlignedData {
  const now = Math.floor(Date.now() / 1000);
  const points = 100;
  const step = 36; // ~1h of data at 36s intervals
  const xs = new Float64Array(points);
  const cpu = new Float64Array(points);
  const mem = new Float64Array(points);
  for (let i = 0; i < points; i++) {
    xs[i] = now - (points - 1 - i) * step;
    cpu[i] = 35 + 25 * Math.sin(i * 0.08) + 10 * Math.sin(i * 0.23) + Math.random() * 5;
    mem[i] = 55 + 15 * Math.sin(i * 0.05 + 1) + Math.random() * 3;
  }
  return [xs, cpu, mem];
}

function generateSparklineValues(count: number): number[] {
  const vals: number[] = [];
  let v = 50;
  for (let i = 0; i < count; i++) {
    v += (Math.random() - 0.48) * 8;
    v = Math.max(5, Math.min(95, v));
    vals.push(v);
  }
  return vals;
}

export function StyleGuidePage() {
  const chartData = createMemo(() => generateSineData());
  const sparkA = createMemo(() => generateSparklineValues(30));
  const sparkB = createMemo(() => generateSparklineValues(20));

  return (
    <div class="page-grid">
      <Panel title="Buttons">
        <div class="row">
          <Button>Default</Button>
          <Button variant="primary">Primary</Button>
          <Button variant="danger">Danger</Button>
        </div>
      </Panel>

      <Panel title="Badges">
        <div class="row">
          <Badge tone="neutral">Neutral</Badge>
          <Badge tone="ok">OK</Badge>
          <Badge tone="warn">Warn</Badge>
          <Badge tone="error">Error</Badge>
        </div>
      </Panel>

      <Panel title="Inputs + Chips">
        <div class="detail-stack">
          <Input placeholder="Search services, incidents, traces..." />
          <div class="row">
            <Chip>prod</Chip>
            <Chip>1h</Chip>
            <Chip>checkout-api</Chip>
          </div>
        </div>
      </Panel>

      <Panel title="Typography + Surface">
        <p class="paragraph">
          V2 uses a no-glow baseline: matte surfaces, semantic color, and crisp hierarchy.
        </p>
      </Panel>

      <Panel title="ChartPanel">
        <ChartPanel
          data={chartData()}
          series={[
            { label: "CPU %", stroke: "#ccff00", fill: "rgba(204,255,0,0.08)" },
            { label: "Memory %", stroke: "#2ed67a", fill: "rgba(46,214,122,0.06)" },
          ]}
          height={200}
        />
      </Panel>

      <Panel title="Sparklines">
        <div class="detail-stack">
          <div class="row" style={{ "align-items": "end", gap: "1.5rem" }}>
            <div>
              <label style={{ display: "block", color: "var(--v2-text-muted)", "font-size": "0.72rem", "text-transform": "uppercase", "letter-spacing": "0.05em", "margin-bottom": "0.3rem" }}>Default (120 x 32)</label>
              <Sparkline values={sparkA()} />
            </div>
            <div>
              <label style={{ display: "block", color: "var(--v2-text-muted)", "font-size": "0.72rem", "text-transform": "uppercase", "letter-spacing": "0.05em", "margin-bottom": "0.3rem" }}>Small (80 x 24)</label>
              <Sparkline values={sparkB()} width={80} height={24} />
            </div>
            <div>
              <label style={{ display: "block", color: "var(--v2-text-muted)", "font-size": "0.72rem", "text-transform": "uppercase", "letter-spacing": "0.05em", "margin-bottom": "0.3rem" }}>Wide / Warning</label>
              <Sparkline values={sparkA()} width={200} height={28} color="var(--v2-warning)" />
            </div>
            <div>
              <label style={{ display: "block", color: "var(--v2-text-muted)", "font-size": "0.72rem", "text-transform": "uppercase", "letter-spacing": "0.05em", "margin-bottom": "0.3rem" }}>Error tone</label>
              <Sparkline values={sparkB()} color="var(--v2-error)" />
            </div>
          </div>
        </div>
      </Panel>
    </div>
  );
}
