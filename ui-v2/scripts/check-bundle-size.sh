#!/usr/bin/env bash
# Bundle budget checker — zero dependencies.
# Reads dist/assets/ and compares against size budgets.
# Exit 1 if any budget is exceeded.
set -euo pipefail

DIST="dist/assets"

# Budgets (bytes)
JS_RAW_BUDGET=450000     # 450 kB
CSS_RAW_BUDGET=30000     # 30 kB
JS_GZIP_BUDGET=130000    # 130 kB
CSS_GZIP_BUDGET=8000     # 8 kB

fail=0

check() {
  local label="$1" actual="$2" budget="$3"
  local actual_kb=$(awk "BEGIN{printf \"%.1f\", $actual/1024}")
  local budget_kb=$(awk "BEGIN{printf \"%.1f\", $budget/1024}")
  if [ "$actual" -gt "$budget" ]; then
    echo "FAIL  $label: ${actual_kb} kB > ${budget_kb} kB budget"
    fail=1
  else
    local pct=$(awk "BEGIN{printf \"%.0f\", ($actual/$budget)*100}")
    echo "OK    $label: ${actual_kb} kB / ${budget_kb} kB (${pct}%)"
  fi
}

if [ ! -d "$DIST" ]; then
  echo "ERROR: $DIST not found. Run 'npx vite build' first."
  exit 1
fi

# Sum raw sizes
js_raw=0
css_raw=0
for f in "$DIST"/*.js; do
  [ -f "$f" ] && js_raw=$((js_raw + $(wc -c < "$f")))
done
for f in "$DIST"/*.css; do
  [ -f "$f" ] && css_raw=$((css_raw + $(wc -c < "$f")))
done

# Gzip sizes
js_gzip=0
css_gzip=0
for f in "$DIST"/*.js; do
  if [ -f "$f" ]; then
    gz=$(gzip -c "$f" | wc -c)
    js_gzip=$((js_gzip + gz))
  fi
done
for f in "$DIST"/*.css; do
  if [ -f "$f" ]; then
    gz=$(gzip -c "$f" | wc -c)
    css_gzip=$((css_gzip + gz))
  fi
done

echo "=== Bundle Size Report ==="
check "JS  raw " "$js_raw"  "$JS_RAW_BUDGET"
check "JS  gzip" "$js_gzip" "$JS_GZIP_BUDGET"
check "CSS raw " "$css_raw"  "$CSS_RAW_BUDGET"
check "CSS gzip" "$css_gzip" "$CSS_GZIP_BUDGET"
echo "========================="

exit $fail
