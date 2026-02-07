import { Badge } from "../design/components/Badge";
import { Button } from "../design/components/Button";
import { Chip } from "../design/components/Chip";
import { Input } from "../design/components/Input";
import { Panel } from "../design/components/Panel";

export function StyleGuidePage() {
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
    </div>
  );
}
