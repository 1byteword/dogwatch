# Core Workflow Specs

## Workflow 1: Alert -> Cause -> Action

## Entry
- User opens alert feed with shared context filters applied.

## Flow
1. Select alert row.
2. Open alert detail panel with:
   - trigger reason
   - affected service
   - recent deploys
   - correlated traces/logs/incidents
3. Jump to correlated timeline for last 60 minutes.
4. Execute action:
   - acknowledge
   - assign owner
   - open incident
   - silence rule (scoped)

## Exit Criteria
- User can identify likely cause and execute first action without changing product section.

## Workflow 2: Incident Commander Loop

## Entry
- User opens incident page from alert or respond section.

## Flow
1. Incident summary and severity/status strip.
2. Unified timeline (alerts, deploys, traces, notes, state changes).
3. Ownership and responder panel.
4. Communication/status update panel.
5. Resolve incident with notes and postmortem pointer.

## Exit Criteria
- Incident lifecycle actions are possible in one command center view.

## Workflow 3: Service Reliability Investigation

## Entry
- User opens service command center.

## Flow
1. Review health/SLO burn/error budget strip.
2. Inspect live reliability timeline and recent changes.
3. Drill into traces/logs for affected paths.
4. Correlate with deployments and dependency failures.
5. Create follow-up item (alert tuning, SLO change, remediation task).

## Exit Criteria
- User can move from symptom to probable root cause with one continuous context.
