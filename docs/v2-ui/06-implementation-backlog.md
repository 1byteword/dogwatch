# Implementation Backlog (Execution Order)

## P0 - V2 Repo Scaffolding
- [ ] Create `ui-v2/` Solid + TypeScript workspace.
- [ ] Add route skeleton for `/app/*`.
- [ ] Add API client layer with typed contracts.
- [ ] Add CI jobs for typecheck, lint, tests, UI audit.

## P1 - Design System Package
- [ ] Create `ui-v2/src/design/tokens/*`.
- [ ] Implement base primitives (button/input/select/tabs/modal/badge).
- [ ] Implement layout shell primitives (`AppShell`, `PageFrame`, `Panel`).
- [ ] Create style guide route showcasing states.

## P2 - Workflow 1 (Alerts/Incidents)
- [ ] Build alerts list/detail with virtualized list.
- [ ] Build correlation panel and action rail.
- [ ] Build incident command center MVP.
- [ ] Add Playwright scenario for triage flow.

## P3 - Workflow 2 (Traces/Logs Investigation)
- [ ] Build logs explorer (virtual list + filter rail).
- [ ] Build traces explorer + detail timeline pane.
- [ ] Add cross-linking from alerts/incidents into traces/logs context.
- [ ] Add performance benchmark for heavy data set.

## P4 - Workflow 3 (Service Reliability + Correlation)
- [ ] Build service command center.
- [ ] Add SLO burn and deploy/incident correlation strip.
- [ ] Add dependency and blast radius panel.
- [ ] Add action handoff to respond workflows.

## P5 - Domain Expansion + Migration
- [ ] Migrate catalog/k8s/on-call/notifications/audit patterns.
- [ ] Move standalone pages into shared shell.
- [ ] Remove duplicated legacy UI islands when equivalents are stable.

## Suggested Initial File Targets
- `ui-v2/package.json`
- `ui-v2/tsconfig.json`
- `ui-v2/vite.config.ts`
- `ui-v2/src/app/router.tsx`
- `ui-v2/src/design/tokens/index.css`
- `ui-v2/src/design/components/*`
- `ui-v2/src/domains/alerts/*`
- `ui-v2/src/domains/incidents/*`
- `ui-v2/src/domains/traces/*`
- `ui-v2/src/domains/logs/*`
- `ui-v2/src/domains/services/*`
