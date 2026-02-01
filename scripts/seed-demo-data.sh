#!/bin/bash
# Seed demo data for dogwatch widgets

API="http://localhost:9999"

echo "=== Seeding Demo Data for dogwatch ==="

# 1. Logs - ingest sample log entries
echo ">> Ingesting sample logs..."
for i in {1..50}; do
  level=$(shuf -e info info info warn error debug -n 1)
  service=$(shuf -e api-gateway user-service payment-service order-service auth-service -n 1)
  messages=(
    "Request processed successfully in ${RANDOM}ms"
    "User authentication completed for user_$((RANDOM % 1000))"
    "Database query executed: SELECT * FROM users"
    "Cache hit ratio: $((70 + RANDOM % 30))%"
    "Connection pool size: $((5 + RANDOM % 20))"
    "Rate limit check passed for client 192.168.1.$((RANDOM % 255))"
    "Order #$((10000 + RANDOM)) created successfully"
    "Payment processed: \$$((RANDOM % 500)).99"
    "Failed to connect to downstream service: timeout after 30s"
    "High memory usage detected: $((70 + RANDOM % 30))%"
    "Retrying failed request (attempt $((1 + RANDOM % 3))/3)"
    "Health check passed for $service"
  )
  msg="${messages[$((RANDOM % ${#messages[@]}))]}"

  curl -s -X POST "$API/api/logs/ingest" \
    -H "Content-Type: application/json" \
    -d "{\"service\":\"$service\",\"level\":\"$level\",\"message\":\"$msg\"}" > /dev/null
done
echo "   Ingested 50 log entries"

# 2. Traces - ingest sample distributed traces
echo ">> Ingesting sample traces..."
for i in {1..20}; do
  trace_id=$(cat /proc/sys/kernel/random/uuid | tr -d '-')
  span_id=$(cat /proc/sys/kernel/random/uuid | tr -d '-' | head -c 16)
  service=$(shuf -e api-gateway user-service payment-service order-service -n 1)
  operation=$(shuf -e "GET /api/users" "POST /api/orders" "GET /api/products" "POST /api/payments" "GET /api/health" -n 1)
  duration=$((20 + RANDOM % 500))
  status=$(shuf -e ok ok ok ok error -n 1)

  curl -s -X POST "$API/v1/traces" \
    -H "Content-Type: application/json" \
    -d "{
      \"resourceSpans\": [{
        \"resource\": {\"attributes\": [{\"key\": \"service.name\", \"value\": {\"stringValue\": \"$service\"}}]},
        \"scopeSpans\": [{
          \"spans\": [{
            \"traceId\": \"$trace_id\",
            \"spanId\": \"$span_id\",
            \"name\": \"$operation\",
            \"kind\": 2,
            \"startTimeUnixNano\": $(($(date +%s%N) - duration * 1000000)),
            \"endTimeUnixNano\": $(date +%s%N),
            \"status\": {\"code\": $([ \"$status\" = \"ok\" ] && echo 1 || echo 2)}
          }]
        }]
      }]
    }" > /dev/null
done
echo "   Ingested 20 traces"

# 3. Watches (Alerts) - create sample alert rules
echo ">> Creating sample watches..."
watches=(
  '{"name":"High CPU Alert","type":"threshold","metric":"cpu_usage","condition":"gt","threshold":80,"severity":"warning","enabled":true}'
  '{"name":"Memory Critical","type":"threshold","metric":"mem_usage","condition":"gt","threshold":90,"severity":"critical","enabled":true}'
  '{"name":"Error Rate Spike","type":"threshold","metric":"error_rate","condition":"gt","threshold":5,"severity":"warning","enabled":true}'
  '{"name":"Slow Response Time","type":"threshold","metric":"p99_latency","condition":"gt","threshold":500,"severity":"warning","enabled":true}'
  '{"name":"Disk Space Low","type":"threshold","metric":"disk_usage","condition":"gt","threshold":85,"severity":"critical","enabled":true}'
)
for watch in "${watches[@]}"; do
  curl -s -X POST "$API/api/watches" -H "Content-Type: application/json" -d "$watch" > /dev/null
done
echo "   Created ${#watches[@]} watches"

# 4. SLOs - create sample SLOs
echo ">> Creating sample SLOs..."
slos=(
  '{"name":"API Availability","description":"API should be available 99.9% of the time","target_percent":99.9,"service_id":"api-gateway","sli_type":"availability","window_days":30}'
  '{"name":"Payment Latency","description":"Payment API p99 latency under 500ms","target_percent":99.0,"service_id":"payment-service","sli_type":"latency","threshold_ms":500,"window_days":7}'
  '{"name":"Order Success Rate","description":"Orders should succeed 99.5% of the time","target_percent":99.5,"service_id":"order-service","sli_type":"success_rate","window_days":30}'
)
for slo in "${slos[@]}"; do
  curl -s -X POST "$API/api/slos" -H "Content-Type: application/json" -d "$slo" > /dev/null 2>&1
done
echo "   Created ${#slos[@]} SLOs"

# 5. Synthetic Checks - create sample checks
echo ">> Creating sample synthetic checks..."
checks=(
  '{"name":"Homepage Health","type":"http","url":"http://localhost:9999/","method":"GET","interval_seconds":60,"timeout_seconds":10,"expected_status":200,"enabled":true}'
  '{"name":"API Health","type":"http","url":"http://localhost:9999/api/stats","method":"GET","interval_seconds":30,"timeout_seconds":5,"expected_status":200,"enabled":true}'
  '{"name":"Auth Endpoint","type":"http","url":"http://localhost:9999/api/auth/providers","method":"GET","interval_seconds":60,"timeout_seconds":10,"expected_status":200,"enabled":true}'
)
for check in "${checks[@]}"; do
  curl -s -X POST "$API/api/synthetics/checks" -H "Content-Type: application/json" -d "$check" > /dev/null 2>&1
done
echo "   Created ${#checks[@]} synthetic checks"

# 6. Deployments - create sample deployment records
echo ">> Creating sample deployments..."
deploys=(
  '{"service":"api-gateway","version":"v1.2.3","environment":"production","status":"success","commit_sha":"abc123f","deployed_by":"ci-bot"}'
  '{"service":"user-service","version":"v2.0.1","environment":"production","status":"success","commit_sha":"def456a","deployed_by":"ci-bot"}'
  '{"service":"payment-service","version":"v1.8.0","environment":"production","status":"success","commit_sha":"789xyz1","deployed_by":"deploy-bot"}'
  '{"service":"order-service","version":"v3.1.0","environment":"staging","status":"success","commit_sha":"stage123","deployed_by":"dev-team"}'
  '{"service":"auth-service","version":"v1.0.5","environment":"production","status":"failed","commit_sha":"fail999","deployed_by":"ci-bot"}'
)
for deploy in "${deploys[@]}"; do
  curl -s -X POST "$API/api/deploys" -H "Content-Type: application/json" -d "$deploy" > /dev/null 2>&1
done
echo "   Created ${#deploys[@]} deployments"

# 7. Incidents - create sample incidents
echo ">> Creating sample incidents..."
incidents=(
  '{"title":"Payment Gateway Timeout","severity":"critical","status":"resolved","service_id":"payment-service","description":"Payment gateway experiencing intermittent timeouts"}'
  '{"title":"High Error Rate on API","severity":"warning","status":"investigating","service_id":"api-gateway","description":"Elevated 5xx error rate detected"}'
  '{"title":"Database Connection Pool Exhausted","severity":"critical","status":"resolved","service_id":"user-service","description":"Connection pool reached max capacity"}'
)
for incident in "${incidents[@]}"; do
  curl -s -X POST "$API/api/incidents" -H "Content-Type: application/json" -d "$incident" > /dev/null 2>&1
done
echo "   Created ${#incidents[@]} incidents"

# 8. Status Pages - create sample status page
echo ">> Creating sample status page..."
curl -s -X POST "$API/api/statuspage/pages" -H "Content-Type: application/json" \
  -d '{"name":"Platform Status","slug":"status","description":"Real-time platform status","public":true}' > /dev/null 2>&1

# Add components to status page
components=(
  '{"name":"API Gateway","description":"Main API endpoint","status":"operational","order":1}'
  '{"name":"User Service","description":"User authentication and profiles","status":"operational","order":2}'
  '{"name":"Payment Processing","description":"Payment gateway integration","status":"degraded","order":3}'
  '{"name":"Order Management","description":"Order processing system","status":"operational","order":4}'
  '{"name":"Database","description":"Primary database cluster","status":"operational","order":5}'
)
for comp in "${components[@]}"; do
  curl -s -X POST "$API/api/statuspage/pages/1/components" -H "Content-Type: application/json" -d "$comp" > /dev/null 2>&1
done
echo "   Created status page with ${#components[@]} components"

# 9. Service Catalog - create sample services
echo ">> Creating sample service catalog entries..."
services=(
  '{"name":"api-gateway","display_name":"API Gateway","description":"Main entry point for all API requests","tier":"tier1","owner_team":"platform","language":"go","repository":"github.com/company/api-gateway"}'
  '{"name":"user-service","display_name":"User Service","description":"Handles user authentication and profiles","tier":"tier1","owner_team":"identity","language":"go","repository":"github.com/company/user-service"}'
  '{"name":"payment-service","display_name":"Payment Service","description":"Processes payments and refunds","tier":"tier1","owner_team":"payments","language":"python","repository":"github.com/company/payment-service"}'
  '{"name":"order-service","display_name":"Order Service","description":"Order management and fulfillment","tier":"tier2","owner_team":"commerce","language":"java","repository":"github.com/company/order-service"}'
  '{"name":"notification-service","display_name":"Notification Service","description":"Email, SMS, and push notifications","tier":"tier2","owner_team":"platform","language":"node","repository":"github.com/company/notification-service"}'
)
for svc in "${services[@]}"; do
  curl -s -X POST "$API/api/catalog/services" -H "Content-Type: application/json" -d "$svc" > /dev/null 2>&1
done

# Create teams
teams=(
  '{"name":"platform","display_name":"Platform Team","description":"Core platform infrastructure","slack_channel":"#platform","email":"platform@company.com"}'
  '{"name":"identity","display_name":"Identity Team","description":"Authentication and authorization","slack_channel":"#identity","email":"identity@company.com"}'
  '{"name":"payments","display_name":"Payments Team","description":"Payment processing","slack_channel":"#payments","email":"payments@company.com"}'
  '{"name":"commerce","display_name":"Commerce Team","description":"E-commerce features","slack_channel":"#commerce","email":"commerce@company.com"}'
)
for team in "${teams[@]}"; do
  curl -s -X POST "$API/api/catalog/teams" -H "Content-Type: application/json" -d "$team" > /dev/null 2>&1
done
echo "   Created ${#services[@]} services and ${#teams[@]} teams"

# 10. On-Call Schedules - create sample schedule
echo ">> Creating sample on-call schedule..."
curl -s -X POST "$API/api/oncall/schedules" -H "Content-Type: application/json" \
  -d '{"name":"Platform On-Call","description":"24/7 platform support rotation","timezone":"America/New_York","rotation_type":"weekly"}' > /dev/null 2>&1
echo "   Created on-call schedule"

# 11. Notification Channels - create sample channels
echo ">> Creating sample notification channels..."
channels=(
  '{"name":"Slack Alerts","type":"slack","config":{"webhook_url":"https://hooks.slack.com/services/xxx","channel":"#alerts"},"enabled":true}'
  '{"name":"PagerDuty","type":"pagerduty","config":{"integration_key":"xxx","severity":"critical"},"enabled":true}'
  '{"name":"Email Team","type":"email","config":{"to":["oncall@company.com"],"subject_prefix":"[ALERT]"},"enabled":true}'
)
for channel in "${channels[@]}"; do
  curl -s -X POST "$API/api/notify/channels" -H "Content-Type: application/json" -d "$channel" > /dev/null 2>&1
done
echo "   Created ${#channels[@]} notification channels"

# 12. Alert Rules - create sample alerting rules
echo ">> Creating sample alert rules..."
rules=(
  '{"name":"High Error Rate","expr":"rate(http_errors_total[5m]) > 0.05","severity":"warning","for":"2m","annotations":{"summary":"Error rate exceeds 5%"}}'
  '{"name":"Service Down","expr":"up == 0","severity":"critical","for":"1m","annotations":{"summary":"Service is down"}}'
  '{"name":"High Latency","expr":"histogram_quantile(0.99, http_request_duration_seconds) > 1","severity":"warning","for":"5m","annotations":{"summary":"P99 latency exceeds 1s"}}'
)
for rule in "${rules[@]}"; do
  curl -s -X POST "$API/api/alerting/rules" -H "Content-Type: application/json" -d "$rule" > /dev/null 2>&1
done
echo "   Created ${#rules[@]} alert rules"

# 13. DB Queries - insert sample slow queries
echo ">> Recording sample database queries..."
queries=(
  'SELECT * FROM users WHERE email = $1'
  'SELECT o.*, u.name FROM orders o JOIN users u ON o.user_id = u.id WHERE o.status = $1'
  'UPDATE inventory SET quantity = quantity - $1 WHERE product_id = $2'
  'INSERT INTO audit_log (action, user_id, details) VALUES ($1, $2, $3)'
  'SELECT COUNT(*) FROM sessions WHERE expires_at > NOW()'
)
for q in "${queries[@]}"; do
  duration=$((10 + RANDOM % 200))
  curl -s -X POST "$API/api/dbwatch/record" -H "Content-Type: application/json" \
    -d "{\"query\":\"$q\",\"duration_ms\":$duration,\"database\":\"postgres\",\"rows_affected\":$((RANDOM % 100))}" > /dev/null 2>&1
done
echo "   Recorded ${#queries[@]} database queries"

# 14. Custom Metrics - send some prometheus metrics
echo ">> Sending sample metrics..."
cat <<EOF | curl -s -X POST "$API/api/v1/write" --data-binary @- > /dev/null 2>&1
# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET",path="/api/users",status="200"} $((1000 + RANDOM))
http_requests_total{method="POST",path="/api/orders",status="200"} $((500 + RANDOM))
http_requests_total{method="GET",path="/api/products",status="200"} $((2000 + RANDOM))
http_requests_total{method="POST",path="/api/payments",status="500"} $((10 + RANDOM % 50))
# HELP http_request_duration_seconds HTTP request latency
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{path="/api/users",le="0.1"} $((800 + RANDOM))
http_request_duration_seconds_bucket{path="/api/users",le="0.5"} $((900 + RANDOM))
http_request_duration_seconds_bucket{path="/api/users",le="1"} $((950 + RANDOM))
http_request_duration_seconds_bucket{path="/api/users",le="+Inf"} $((1000 + RANDOM))
EOF
echo "   Sent sample metrics"

echo ""
echo "=== Demo data seeding complete! ==="
echo "Refresh the dogwatch UI to see the data in widgets."
