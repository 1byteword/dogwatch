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

## Platform Engineering Adoption Roadmap

What platform teams actually evaluate before adopting a tool. Ranked by impact-per-effort.

### Tier 1 — Highest leverage (address the reasons teams say no)

#### Dashboards-as-Code

- [ ] `dogwatch apply -f monitoring.yaml` — declarative config that reconciles state
- [ ] Define alerts, SLOs, and dashboards in YAML/JSON, check into git
- [ ] Apply on startup or via API (like Prometheus alerting rules / Grafana provisioning)
- [ ] This is a dealbreaker for platform teams — they manage everything through git
- [ ] Without this, the tool doesn't fit their workflow no matter how good the UI is

#### Production Safety Documentation

- [ ] Create `SAFETY.md` or "Running in Production" docs page
- [ ] Document: CPU ceiling for eBPF probes — can you set a hard limit?
- [ ] Document: what happens when the ring buffer fills — graceful event drop or backpressure?
- [ ] Document: what happens if dogwatch crashes — are eBPF programs cleaned up or do orphaned probes linger?
- [ ] This is the #1 concern. "We hook into your kernel" makes senior engineers nervous
- [ ] Answers must be prominent, not buried in code comments

#### Log-Trace Correlation

- [ ] Accept structured logs (JSON) via OTLP receiver
- [ ] Correlate logs with traces — click from a trace span to logs emitted during that span
- [ ] Support Fluentbit / OTel Collector filelog receiver → dogwatch pipeline
- [ ] This is the "three pillars" story platform teams want
- [ ] Trace-to-log and log-to-trace correlation is the feature that makes people switch from Datadog
- [ ] Datadog charges insane per-GB log prices — this is the cost wedge

### Tier 2 — High impact, more engineering effort

#### SSO (OIDC)

- [ ] OIDC support covering Okta, Google Workspace, Azure AD
- [ ] Solid Go OIDC libraries exist — not a huge lift
- [ ] Without SSO, separate credentials = friction + security concern
- [ ] Difference between "cool tool I tried once" and "tool I can roll out to my org"
- [ ] RBAC already exists (Owner > Admin > Editor > Viewer) — SSO plugs into it

#### Kubernetes (deliberate, narrow scope)

- [ ] Helm chart: dogwatch as DaemonSet
- [ ] Auto-discover pods and services via Kubernetes API
- [ ] Label traces and metrics with K8s metadata (pod name, namespace, deployment)
- [ ] eBPF probes already work at host level — value-add is the K8s labeling
- [ ] Do NOT try to replace full Prometheus + Grafana stack inside a cluster on day one
- [ ] This is the minimum viable K8s story

### Tier 3 — Important, follows naturally once adoption starts

#### Upgrades and Backwards Compatibility

- [ ] Document schema versioning — migrations run automatically on startup
- [ ] Document rollback story — restore SQLite file from backup
- [ ] `dogwatch backup` subcommand (backup/restore scheduler already exists in `internal/backup/`)
- [ ] Platform teams think about month 6, month 12 — not just day 1
- [ ] "Will my dashboards and alerts survive an upgrade?" needs a clear answer

#### Retention and Resource Limits

- [ ] Ship sane defaults: retention period, max disk usage, max memory for aggregator
- [ ] Make all limits configurable
- [ ] Log warnings when approaching limits
- [ ] Auto-downsample older data: 1s resolution (last hour) → 1m (last day) → 5m (last week)
- [ ] Platform teams hate tools that silently eat disk until the machine falls over

#### Health and Self-Monitoring

- [ ] `/healthz` and `/readyz` endpoints (basic versions already exist)
- [ ] Expose internal metrics: events/sec, ring buffer utilization, SQLite WAL size, query latency
- [ ] Prometheus exposition format (`/metrics`) so people can monitor dogwatch with existing monitoring
- [ ] If dogwatch can't tell you it's healthy, they won't trust it to tell them their apps are healthy

#### Terraform/Pulumi Provider

- [ ] Don't build yet — signal intent in the roadmap
- [ ] "Planned: Terraform provider for alerts, SLOs, and dashboards"
- [ ] Tells platform teams you understand their world
- [ ] Builds on dashboards-as-code foundation

---

## Project Maturity & Adoption Infrastructure

The silent killers of adoption. None of these are features, but each one changes how the project is perceived.

### Top 3 (highest perception impact)

#### Cut a Versioned Release

- [ ] Tag `v0.1.0` — 149+ commits and zero releases reads as "hobby project"
- [ ] Write `CHANGELOG.md` with dates and what changed
- [ ] Semver signals intent about stability; release notes give upgrade confidence
- [ ] Stable releases when meaningful (new feature, meaningful bugfix, breaking change) — not on a forced cadence
- [ ] Don't automate stable releases on a schedule — "bumped a dependency" every week looks like churn

#### Docs Site (not just the README)

- [ ] Docusaurus or MkDocs on GitHub Pages
- [ ] Structure: Getting Started (first 15 min, reading the dashboard, first alert), Architecture (data model, retention), Configuration Reference (every flag/env var), Troubleshooting
- [ ] Troubleshooting page is critical — it's the first thing people Google when something breaks
- [ ] If they can't find an answer, they uninstall and move on

#### Demo Instance

- [ ] Host a read-only demo with pre-loaded data — lets people explore the UI without installing
- [ ] Public URL with banner: "this is demo data"
- [ ] Alternatively: embedded screenshots or screen recording in docs
- [ ] Removes friction for "does this UI actually show me what I need?"

### Automated Build Pipeline

#### Nightly Builds (fully automated, don't think about them)

- [ ] Every commit to main → sha-tagged Docker image + binary artifact
- [ ] Daily (or per-merge) GitHub Action publishes as `nightly` pre-release, overwrites previous
- [ ] Tag as `nightly-YYYYMMDD`, clear warning: "no stability guarantees, don't run in production"
- [ ] Signals activity even when heads-down on features for weeks
- [ ] Gives adventurous users a way to test without building from source

#### Stable Release Workflow

- [ ] On `vX.Y.Z` tag → GitHub Action builds .deb, .rpm, Docker image, publishes as GitHub Release
- [ ] Hand-written release notes (or semi-automate with git-cliff from conventional commits)
- [ ] Release when meaningful, not on a calendar — every 3-6 weeks is fine
- [ ] The habit of writing release notes compounds into a changelog that tells the project's story

### Packaging (beyond curl-pipe-bash)

- [ ] `.deb` and `.rpm` packages for production installs
- [ ] Docker image (needs `--privileged` for eBPF, but teams want it for consistency)
- [ ] Homebrew/Linuxbrew formula for developer machines
- [ ] GitHub Actions workflow that builds all of these on every release — one-time investment

### Error Messages That Help

- [ ] Kernel too old → show current kernel version, minimum required, link to upgrade doc
- [ ] eBPF probe attachment failed → which probe, why (missing BTF? kernel config? seccomp blocking BPF?)
- [ ] Permission denied → specific guidance, not just "run as root"
- [ ] Invest disproportionately in first-run error paths — that's where you lose people permanently

### Public Roadmap with Status

- [ ] Turn roadmap into GitHub Issues or a GitHub Project board
- [ ] When someone's #1 need is K8s support, they want to see: planned? In progress? Discussion thread?
- [ ] Thumbs-up reactions tell you what to build next
- [ ] Turns passive readers into engaged community members

### Community Infrastructure (minimal)

- [ ] GitHub Discussions tab or single Discord server — somewhere for "how do I X" that isn't Issues
- [ ] Issues should be bugs and feature requests, not support questions
- [ ] Be responsive in the first few months — thoughtful answer within hours creates advocates
- [ ] Unanswered questions for a week = they leave and never come back
- [ ] At this scale, you are the community. Your responsiveness is the project's reputation

### "Who Is This For" Positioning

- [ ] Be specific: "teams running 1-50 services on Linux who don't want to pay for Datadog or operate a Prometheus stack"
- [ ] Be honest about who it's NOT for: "Windows, multi-region sub-second replication, already happy with your Datadog bill"
- [ ] Specificity builds trust — people believe you more when you tell them what you can't do
- [ ] Add to README or docs site landing page

### Contributor-Friendly Codebase

- [ ] `CONTRIBUTING.md` — how to build, test, submit PRs
- [ ] Code comments on non-obvious parts (especially eBPF C code — notoriously hard to read)
- [ ] "Good first issue" label on a handful of issues
- [ ] Clean separation between eBPF layer, storage layer, API layer so someone can contribute to one without understanding all three
- [ ] Go/C split is already a natural boundary — make it explicit

---

## Operational Trust & Developer Experience

The things that make platform teams say "this person has actually run software in production."

### API Versioning

- [ ] Version API paths now: `/api/v1/incidents` instead of `/api/incidents`
- [ ] Trivial change today, painful migration later if you skip it
- [ ] At v0.x you don't need full stability guarantees, but the path prefix buys room to evolve
- [ ] PromQL endpoint is especially important — people point Grafana at it and forget about it
- [ ] Once anyone integrates, breaking the API is painful for them

### Dogfooding

- [ ] Run dogwatch monitoring dogwatch's own infrastructure (landing page, CI, any server)
- [ ] Reference it in docs: "here's how we use dogwatch to monitor our own build pipeline"
- [ ] Nothing builds trust faster than the maintainer eating their own cooking
- [ ] You'll find the rough edges before your users do

### Migration Story (end-to-end)

- [ ] `dogwatch migrate --from datadog` is a great hook — follow-through matters
- [ ] Migration output must be verbose: "imported 12/15 monitors, skipped 3 (unsupported: anomaly detection, composite monitor, APM trace query)"
- [ ] If it silently drops half their alerts, they'll never trust the tool again
- [ ] 80% clean import + clear accounting of the 20% = trust. Silent data loss = uninstall.

### Failure Modes Documentation

- [ ] Create a "Failure Modes" doc page — platform teams think in failure modes
- [ ] Disk full — what happens? Graceful degradation or crash?
- [ ] SIGKILL mid-write — does SQLite corrupt? (WAL mode is robust, but say it explicitly)
- [ ] Federated node goes down — do alerts re-fire on surviving nodes? Does the cluster know?
- [ ] Two dogwatch instances on same host — do they fight over eBPF probes?
- [ ] The fact that you've thought about it is as important as the actual answers

### Default Alerts (zero-config value)

- [ ] Ship a handful of default alerts enabled out of the box:
  - [ ] Disk usage > 90%
  - [ ] Any 5xx error rate > 5%
  - [ ] Any endpoint p99 > 2 seconds
- [ ] Easy to dismiss or customize, but first-open experience should feel like dogwatch already understands the system
- [ ] The "zero config" pitch is only as good as the zero-config defaults

### Observability Gaps (meta-observability)

- [ ] Surface when dogwatch is missing data — don't silently not observe something
- [ ] If eBPF probe attachment fails for a process: "3 processes detected, 2 instrumented, 1 failed (permission denied on PID 4521)"
- [ ] If OTLP ingestion stops receiving from a previously-active service, flag it as a gap
- [ ] Worst thing an observability tool can do: let you think everything is fine when it's not

### Debug Escape Hatches

- [ ] `dogwatch debug probes` — which eBPF programs are loaded, event counts per probe
- [ ] `dogwatch debug storage` — SQLite stats (WAL size, table row counts, vacuum status)
- [ ] `dogwatch debug cluster` — gossip state, peer health, CRDT sync status
- [ ] Cheap to build (surfacing internal state), huge for trust
- [ ] Difference between "something's wrong, let me check" and "something's wrong, let me uninstall"

### "Why I'm Building This" (personal conviction)

- [ ] Write one paragraph somewhere — About page, blog post, or landing page
- [ ] Not a mission statement or manifesto. Just the honest version:
  - What frustrated you about current observability tooling
  - What you think is broken
  - What you're trying to do about it
- [ ] Platform engineers are skeptical of tools but they trust people with a clear point of view
- [ ] Personal conviction is what turns a GitHub repo into a project people root for

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
