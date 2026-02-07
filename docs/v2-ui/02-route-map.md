# Route Map (V2)

## App Shell
- `/app`

Shared shell regions:
- left nav
- global time/env/service context rail
- global command bar
- main content

## Detect
- `/app/detect/alerts`
- `/app/detect/alerts/rules`
- `/app/detect/slo`
- `/app/detect/anomalies`

## Investigate
- `/app/investigate/logs`
- `/app/investigate/traces`
- `/app/investigate/map`
- `/app/investigate/query`

## Correlate
- `/app/correlate/timeline`
- `/app/correlate/deploys`
- `/app/correlate/incidents`

## Respond
- `/app/respond/incidents`
- `/app/respond/oncall`
- `/app/respond/notifications`
- `/app/respond/status-pages`

## Improve
- `/app/improve/reliability`
- `/app/improve/cost`
- `/app/improve/cardinality`
- `/app/improve/postmortems`

## Configure
- `/app/configure/catalog`
- `/app/configure/integrations`
- `/app/configure/teams`
- `/app/configure/admin`

## Entity Detail Routes
- `/app/service/:serviceId`
- `/app/incident/:incidentId`
- `/app/deploy/:deployId`
- `/app/trace/:traceId`
- `/app/entity/:entityId`
- `/app/team/:teamId`

## Legacy Compatibility
- Keep existing pages available during migration.
- Add controlled entry points from old routes to V2 routes.
