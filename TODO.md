# TODO.md

Last updated: 2026-02-19

---

## Launch Readiness Roadmap

Priority order. Benchmarks are the foundation — everything else references them.

### 1. Benchmark Suite

- [ ] Create `make bench` target that anyone can run reproducibly
- [ ] Measure: events ingested/sec (single node)
- [ ] Measure: query latency at 1h, 12h, 24h data volumes
- [ ] Measure: memory footprint under load
- [ ] Measure: CPU overhead of eBPF probes vs. baseline (no probes)
- [ ] Compare against OTel Collector + Prometheus + Loki on same hardware (not Datadog — unfair arch comparison)
- [ ] Publish results in `BENCHMARKS.md`, keep updated with each release
- [ ] Goal: honest, credible numbers — not marketing wins

### 2. Video Demo (3 minutes, no fluff)

- [ ] Record: fresh Ubuntu VM → `curl | sh` → `sudo dogwatch` → browser opens with data
- [ ] Show: hit machine with `wrk`/`hey` traffic against a demo app
- [ ] Show: traces appearing, service map building, latency histograms populating
- [ ] Show: create an alert, watch it fire
- [ ] No narration fluff, no intro animation — the pitch is "zero to useful in 60 seconds"
- [ ] Embed asciinema/gif in README for terminal portion, YouTube link for full video

### 3. Blog Posts (useful content, not launch announcements)

- [ ] "How we parse HTTP/2 from eBPF syscall tracepoints" — deep technical, attracts eBPF community
- [ ] "What SQLite can actually handle as a metrics store" — contrarian take, gets shared
- [ ] "Migrating from Datadog saved us $X/month" — cost story resonates even on own infra
- [ ] Each post is useful regardless of whether reader adopts dogwatch
- [ ] Target: Hacker News, lobste.rs, CNCF newsletter

### 4. Test Suite Visibility

- [ ] Add CI badge to README (tests passing)
- [ ] Integration tests: spin up real MySQL/Postgres/Redis, verify dogwatch captures queries
- [ ] "Time to first trace" test: start dogwatch → HTTP request → assert trace exists within 5 seconds
- [ ] Existing e2e suite (67 Playwright tests) signals project is serious — make it visible

### 5. Show HN Launch

- [ ] Time after benchmarks + video + at least one blog post are in place
- [ ] Link to repo + short compelling blog post
- [ ] eBPF + single binary + replaces-Datadog angle is HN catnip
- [ ] Repo must look polished when people land on it (README, badges, benchmarks, demo)

### Skip for now

- Conference talks (too early, not enough users)
- Paid ads (wrong audience at this stage)
- Detailed competitor comparison pages (comes across as defensive when small)

### Compounding strategy

Benchmarks → numbers to cite everywhere.
Video demo → reason to try it.
Blog posts → organic traffic from the right audience.
Test suite → confidence once they're evaluating.
Show HN → the ignition event. Each layer feeds the next.

---

## V2 Frontend Rebuild (Datadog-Competitive)

Tracks the V2 UI/Product rebuild decisions and execution plan.

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
- [x] Enforce CI checks (UI audit + perf budget baseline).

## Phase 2 - Core Workflows (MVP)
- [x] Implement Alerts/Incidents end-to-end workflow.
- [x] Implement Traces/Logs investigation workflow.
- [x] Implement Service Catalog + Correlation workflow.
- [x] Validate keyboard-first paths for triage workflows.

## Phase 3 - Domain Expansion
- [x] Migrate Kubernetes/On-call/Notifications/Audit flows into V2 patterns.
- [x] Unify standalone pages under shared V2 shell/styles.
- [x] Remove duplicate style islands and one-off component patterns.

## Phase 4 - Hardening
- [x] Performance optimization pass (rendering, streaming, bundle split).
- [x] Accessibility pass (focus order, keyboard nav, ARIA, contrast).
- [x] Visual regression stability for all critical workflows.
- [x] Reduce legacy inline-style/onclick debt below current budget thresholds.

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
- [x] Create V1 widget parity matrix + production hump plan: `docs/v2-ui/07-v1-parity-and-prod-hump.md`.
- [x] Deliver widget pack parity wave 1: System + Discovery + Trace.
- [x] Deliver widget pack parity wave 2: SLO/Synthetics + Cost/Usage + Admin.
- [x] Add autosave + crash recovery + per-widget error boundaries for dashboard editor.
- [x] Enforce CI performance budgets and golden-flow E2E for V2 default routes.
