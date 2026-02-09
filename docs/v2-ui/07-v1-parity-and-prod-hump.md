# V1 Parity Matrix And Production Hump Plan

Last updated: 2026-02-09

This doc answers two questions:
1. Have we matched the breadth of V1 dashboard widgets?
2. What work remains to make V2 production-ready and truly Datadog-competitive?

## 1) V1 -> V2 Widget Parity Matrix

Reference baseline: `UI.md` lists 54 V1 dashboard widgets across 12 categories.

Current V2 dashboard widgets (16 total): `ui-v2/src/routes/DashboardsPage.tsx`
- `kpi-reliability`
- `alerts-feed`
- `alerts-severity-map`
- `incidents-live`
- `incidents-by-commander`
- `logs-errors`
- `deploy-correlation`
- `service-health`
- `service-latency-top`
- `oncall-now`
- `k8s-cluster`
- `k8s-capacity-risk`
- `notify-delivery`
- `notify-failure-log`
- `ops-action-queue`
- `command-links`

### Category Coverage

| V1 Category | V1 Count | V2 Dashboard Parity | Status | Gap Summary |
|---|---:|---:|---|---|
| System Monitoring | 7 | 0 | Missing | Add CPU, memory, disk I/O, network, connections, requests, errors widgets. |
| Service Discovery | 4 | 0 | Missing | Add service map, endpoints, connections, top processes widgets. |
| Distributed Tracing | 4 | 0 | Missing | Add trace list, trace detail, trace services, dependencies widgets. |
| Logs & Patterns | 5 | 1 | Partial | `logs-errors` exists; missing pattern, trending, compare-focused widgets. |
| Alerting | 4 | 2 | Partial | `alerts-feed`, `alerts-severity-map` exist; missing watch/group/silence management widgets. |
| Synthetics & SLOs | 4 | 0 | Missing | Add synthetic checks, check results/uptime, SLO status widgets. |
| Infrastructure | 5 | 2 | Partial | `k8s-cluster`, `k8s-capacity-risk` exist; missing containers, pods/deployments/events detail widgets. |
| Incidents & On-Call | 4 | 3 | Strong partial | `incidents-live`, `incidents-by-commander`, `oncall-now` exist; add schedule/escalation detail widgets. |
| Deployments | 3 | 1 | Partial | `deploy-correlation` exists; add deploy feed/stats widgets. |
| Performance | 4 | 0 | Missing | Add flame graph, anomaly, dbwatch query widgets. |
| Cost & Usage | 5 | 0 | Missing | Add cost estimate, recommendations, quick wins, cardinality, usage widgets. |
| Administration | 5 | 0 | Missing | Add users/teams/apikey/audit/backups widgets (or admin command center equivalents). |

### Numeric Snapshot

- V1 widgets: 54
- V2 dashboard widgets implemented: 16
- Direct breadth parity: ~30%

Conclusion:
- V2 has better dashboard editing UX than V1.
- V2 does not yet match V1 surface breadth.
- We should add missing categories in focused packs, not one-off widget work.

## 2) Production Hump Plan (Beyond Parity)

Parity alone is not sufficient. V1 itself is not production-ready in several areas, so V2 needs stronger standards than "match V1."

## A. Product Surface Completion

1. Build widget packs by category
- `System Pack`: CPU, memory, disk I/O, network, req/s, errors, connections.
- `Discovery Pack`: service map mini-topology, endpoint latency/error table, top processes.
- `Trace Pack`: traces table, latency percentiles, dependency risk.
- `SLO Pack`: uptime, burn rate, budget remaining, synthetic failures.
- `Cost Pack`: spend estimate, cardinality hotspots, recommendations.
- `Admin Pack`: audit feed, key churn, backup status, user/team posture.

2. Add template dashboards for major roles
- Executive, Incident Commander, SRE Platform, Service Owner, FinOps, Security/Compliance.

3. Standardize widget contracts
- Every widget supports: global scope, per-widget overrides, loading/empty/error states, refresh latency indicator.

## B. Performance/Scale Readiness

1. Render performance guardrails
- Virtualize long tables/lists.
- Avoid full-canvas rerenders on single widget updates.
- Add memoized selectors for heavy aggregates.

2. Budgets in CI
- First meaningful dashboard render budget.
- Interaction budget for drag/resize/filter.
- Bundle size budget by route chunk.

3. High-volume realism
- Seed high-cardinality datasets.
- Run stress scenarios for logs/traces/events streams.

## C. Reliability/Correctness

1. Data contract hardening
- Typed API schemas for dashboard payloads and widget data.
- Backward-compatible migrations for widget config versions.

2. Autosave and recovery
- Debounced autosave in edit mode.
- Recover unsaved dashboard state after refresh/crash.

3. Error isolation
- Per-widget error boundaries so one failure does not degrade whole canvas.

## D. UX Quality Bar

1. Visual consistency
- Remove remaining style islands from standalone pages.
- Align typography/spacing rhythm across all routes.

2. Accessibility
- Keyboard complete dashboard editing and navigation.
- ARIA labels and focus states for drag/resize/inspector.
- Contrast checks for every semantic state.

3. Interaction polish
- Snaplines and drop previews.
- Multi-select and batch operations.
- Sticky inspector with clearer ownership/scoping signals.

## E. Test/Release Readiness

1. E2E coverage
- Golden flows: alert -> incident -> root-cause -> action.
- Dashboard CRUD + import/export + template apply + undo/redo.

2. Visual regression
- Snapshot critical dashboards at key breakpoints.

3. Release controls
- Feature flags for new packs.
- Staged rollout with telemetry and rollback path.

## F. Operations Readiness

1. Frontend observability
- Instrument web-vitals and UI errors.
- Dashboard interaction telemetry (slow widgets, failed queries).

2. Security basics
- Review auth boundaries for all management surfaces.
- CSRF/XSS hardening checks for new endpoints and forms.

3. Documentation/runbooks
- "How to operate V2" guide for on-call engineers.
- Migration guide from V1 dashboards to V2 templates.

## 3) Execution Order (Recommended)

1. Close dashboard breadth gap with System + Discovery + Trace packs first.
2. Add SLO/Synthetics and Cost packs.
3. Run hardening sweep (perf, a11y, error isolation, autosave).
4. Complete release gating (E2E + visual + budgets in CI).
5. Roll out gradually with telemetry and fallback.

## 4) Definition Of Done For "Over The Hump"

All of the following must be true:
- >=85% V1 category parity by meaningful widget capability (not just route presence).
- Performance budgets passing in CI.
- E2E golden flows stable.
- Accessibility checks passing for all tier-1 routes.
- Error isolation and autosave in place for dashboards.
- V2 is default UI with rollback switch retained for one release cycle.
