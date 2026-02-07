# TODO.md - V2 Frontend Rebuild (Datadog-Competitive)

This file replaces the previous roadmap and tracks the V2 UI/Product rebuild decisions and execution plan.

Last updated: 2026-02-07

---

## Decisions Locked

### Backend
- Keep Go backend.
- Expose typed APIs for the new frontend via gRPC + Connect-Web.
- Use WebSocket/SSE for live updates where streaming is needed.

### Frontend
- Build V2 in Solid + TypeScript.
- No inline styles/inline handlers in new V2 code.
- Use route-level code splitting and domain-based modules.

### Data-Dense Rendering
- Use TanStack Table + TanStack Virtual for heavy tabular/list views.
- Use uPlot for high-frequency time series charts.
- Use WebGL-capable rendering path (as needed) for high-volume topology/timeline surfaces.

### Perf + Quality Testing
- Playwright for end-to-end flows.
- Visual regression snapshots for core workflows.
- Performance budgets enforced in CI.
- Keep `scripts/ui-audit.sh` and tighten thresholds over time.

---

## Product/Design Direction

- No-glow baseline visual system.
- Matte, high-contrast surfaces with crisp edges.
- Color is semantic signal (status/severity/selection), not decoration.
- Dense but readable operations-first layouts.
- One cohesive design system across dashboard and standalone pages.

---

## V2 Implementation Phases

## Phase 0 - Blueprint
- [x] Create `/docs/v2-ui/` with:
  - [x] IA map (Detect, Investigate, Correlate, Respond, Improve, Configure)
  - [x] Route map
  - [x] Component inventory
  - [x] Core workflow specs (Alert->Cause->Action, Incident Commander, Service Reliability)
  - [x] Acceptance criteria per workflow

## Phase 1 - Foundation
- [x] Scaffold Solid + TypeScript app structure for V2.
- [x] Define token system (color/type/spacing/elevation/motion).
- [x] Build app shell primitives (nav/header/filter rail/page frame).
- [x] Build core base components (button/input/select/tabs/badges/modal/empty-loading-error states).
- [ ] Enforce CI checks (UI audit + perf budget baseline).

## Phase 2 - Core Workflows (MVP)
- [x] Implement Alerts/Incidents end-to-end workflow.
- [x] Implement Traces/Logs investigation workflow.
- [x] Implement Service Catalog + Correlation workflow.
- [x] Validate keyboard-first paths for triage workflows.

## Phase 3 - Domain Expansion
- [x] Migrate Kubernetes/On-call/Notifications/Audit flows into V2 patterns.
- [ ] Unify standalone pages under shared V2 shell/styles.
- [ ] Remove duplicate style islands and one-off component patterns.

## Phase 4 - Hardening
- [ ] Performance optimization pass (rendering, streaming, bundle split).
- [ ] Accessibility pass (focus order, keyboard nav, ARIA, contrast).
- [ ] Visual regression stability for all critical workflows.
- [ ] Reduce legacy inline-style/onclick debt below current budget thresholds.

---

## Architecture Guardrails (Must Keep)

- Domain modules only; no new mega-file equivalent to old `app.js`.
- Shared component primitives for all new UI.
- Typed API clients only.
- Observability surfaces must support high-cardinality/high-volume views without UI jank.

---

## Immediate Next Actions

- [x] Start Phase 0 artifact creation in `/docs/v2-ui/`.
- [x] Define Solid project layout and module boundaries before coding pages.
- [x] Build first style guide page with canonical components and states.
