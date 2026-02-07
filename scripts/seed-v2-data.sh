#!/usr/bin/env bash
set -euo pipefail

API_BASE="${API_BASE:-http://127.0.0.1:9999}"

if ! curl -sS --max-time 2 "$API_BASE/health" >/dev/null; then
  echo "error: dogwatch API not reachable at $API_BASE"
  exit 1
fi

echo "Seeding V2 demo data into $API_BASE"

post_json() {
  local path="$1"
  local body="$2"
  local code
  code=$(curl -sS -o /tmp/seed-v2.out -w "%{http_code}" -X POST "$API_BASE$path" \
    -H "Content-Type: application/json" \
    -d "$body") || true
  if [[ "$code" =~ ^2 ]]; then
    echo "  ok   POST $path"
  else
    echo "  warn POST $path -> $code"
    head -c 160 /tmp/seed-v2.out || true
    echo
  fi
}

# Catalog services
post_json "/api/catalog/services" '{"name":"checkout-api","display_name":"Checkout API","description":"Checkout gateway","tier":"critical","team_name":"payments","owner_email":"payments@example.com","lifecycle":"active","health":"healthy"}'
post_json "/api/catalog/services" '{"name":"auth-api","display_name":"Auth API","description":"Authentication service","tier":"high","team_name":"identity","owner_email":"identity@example.com","lifecycle":"active","health":"degraded"}'
post_json "/api/catalog/services" '{"name":"orders-worker","display_name":"Orders Worker","description":"Order pipeline worker","tier":"medium","team_name":"commerce","owner_email":"commerce@example.com","lifecycle":"active","health":"healthy"}'

# Deployments for correlation timelines
post_json "/api/deploys" '{"service":"checkout-api","version":"v2.3.0","environment":"prod","status":"success","user":"ci-bot","description":"release train"}'
post_json "/api/deploys" '{"service":"auth-api","version":"v1.9.2","environment":"prod","status":"success","user":"ci-bot","description":"hotfix"}'

# Incidents
post_json "/api/incidents" '{"title":"Checkout latency spike","severity":"high","service":"checkout-api","description":"p99 above threshold","source":"watch"}'
post_json "/api/incidents" '{"title":"Auth error burst","severity":"critical","service":"auth-api","description":"5xx increase","source":"watch"}'

# Logs
post_json "/api/logs/ingest" '{"service":"checkout-api","level":"error","message":"timeout contacting payment provider"}'
post_json "/api/logs/ingest" '{"service":"checkout-api","level":"warn","message":"queue lag at 1800ms"}'
post_json "/api/logs/ingest" '{"service":"auth-api","level":"error","message":"token validation failed for issuer"}'
post_json "/api/logs/ingest" '{"service":"orders-worker","level":"info","message":"batch processed successfully"}'

# Alert rules
post_json "/api/alerting/rules" '{"name":"Checkout Error Rate","description":"Error rate > 2%","type":"threshold","enabled":true,"labels":{"service":"checkout-api","severity":"warning"},"annotations":{"summary":"checkout errors elevated"},"metric":"error_rate","condition":"gt","threshold":2}'
post_json "/api/alerting/rules" '{"name":"Auth 5xx Critical","description":"Auth 5xx > 5%","type":"threshold","enabled":true,"labels":{"service":"auth-api","severity":"critical"},"annotations":{"summary":"auth failures elevated"},"metric":"error_rate","condition":"gt","threshold":5}'

# On-call schedule + policy
post_json "/api/oncall/schedules" '{"name":"Core Platform","description":"Primary rotation","timezone":"UTC","teams":["platform"],"layers":[{"id":"primary","name":"Primary","priority":1,"rotation_type":"daily","handoff_time":"09:00","handoff_day":1,"shift_duration":"24h","start_date":"2026-01-01T00:00:00Z","users":[{"id":"alice","name":"Alice","email":"alice@example.com"},{"id":"bob","name":"Bob","email":"bob@example.com"}]}]}'
post_json "/api/oncall/policies" '{"name":"Critical Services","description":"Escalate quickly","teams":["platform"],"rules":[{"level":1,"delay_minutes":5,"targets":[{"type":"schedule","id":"core-platform"}]}],"repeat_enabled":true,"repeat_limit":2}'

# Notification channels
post_json "/api/notify/channels" '{"name":"Ops Webhook","type":"webhook","enabled":true,"config":{"url":"http://127.0.0.1:9999/health","method":"POST","timeout":5}}'
post_json "/api/notify/channels" '{"name":"Slack Alerts","type":"slack","enabled":true,"config":{"webhook_url":"https://hooks.slack.invalid/services/demo"}}'

echo "Seed run complete."
