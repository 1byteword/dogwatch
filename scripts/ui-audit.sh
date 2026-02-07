#!/bin/bash
# UI governance audit: blocks regressions in inline styling/handlers debt.
# Run: ./scripts/ui-audit.sh

set -euo pipefail

ROOT="internal/web/static"
APP_JS="$ROOT/js/app.js"

# Baseline budgets (tighten over time).
MAX_INLINE_STYLE_ATTRS="${MAX_INLINE_STYLE_ATTRS:-540}"
MAX_INLINE_ONCLICK_ATTRS="${MAX_INLINE_ONCLICK_ATTRS:-494}"
MAX_INLINE_ONCHANGE_ATTRS="${MAX_INLINE_ONCHANGE_ATTRS:-49}"
MAX_INLINE_ONINPUT_ATTRS="${MAX_INLINE_ONINPUT_ATTRS:-3}"
MAX_APPJS_INNERHTML="${MAX_APPJS_INNERHTML:-186}"

count() {
    local pattern="$1"
    local target="$2"
    (rg -o "$pattern" "$target" || true) | wc -l | tr -d ' '
}

INLINE_STYLE_ATTRS="$(count 'style="' "$ROOT")"
INLINE_ONCLICK_ATTRS="$(count 'onclick=' "$ROOT")"
INLINE_ONCHANGE_ATTRS="$(count 'onchange=' "$ROOT")"
INLINE_ONINPUT_ATTRS="$(count 'oninput=' "$ROOT")"
APPJS_INNERHTML="$(count 'innerHTML[[:space:]]*=' "$APP_JS")"

echo "UI Audit:"
echo "  inline style attrs    : $INLINE_STYLE_ATTRS (max $MAX_INLINE_STYLE_ATTRS)"
echo "  inline onclick attrs  : $INLINE_ONCLICK_ATTRS (max $MAX_INLINE_ONCLICK_ATTRS)"
echo "  inline onchange attrs : $INLINE_ONCHANGE_ATTRS (max $MAX_INLINE_ONCHANGE_ATTRS)"
echo "  inline oninput attrs  : $INLINE_ONINPUT_ATTRS (max $MAX_INLINE_ONINPUT_ATTRS)"
echo "  app.js innerHTML uses : $APPJS_INNERHTML (max $MAX_APPJS_INNERHTML)"

FAIL=0
[ "$INLINE_STYLE_ATTRS" -le "$MAX_INLINE_STYLE_ATTRS" ] || FAIL=1
[ "$INLINE_ONCLICK_ATTRS" -le "$MAX_INLINE_ONCLICK_ATTRS" ] || FAIL=1
[ "$INLINE_ONCHANGE_ATTRS" -le "$MAX_INLINE_ONCHANGE_ATTRS" ] || FAIL=1
[ "$INLINE_ONINPUT_ATTRS" -le "$MAX_INLINE_ONINPUT_ATTRS" ] || FAIL=1
[ "$APPJS_INNERHTML" -le "$MAX_APPJS_INNERHTML" ] || FAIL=1

if [ "$FAIL" -ne 0 ]; then
    echo ""
    echo "UI audit failed: regression detected against current debt budget."
    echo "If this increase is intentional, raise max values explicitly in CI/job env."
    exit 1
fi

echo "UI audit passed."
