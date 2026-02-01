#!/bin/bash
# Bundle all static assets for production
# Run: ./scripts/bundle-all.sh

set -e

echo "=== Building dogwatch static bundles ==="
echo ""

# Bundle JavaScript components
./scripts/bundle-components.sh
echo ""

# Bundle CSS
./scripts/bundle-css.sh
echo ""

# Summary
echo "=== Bundle Summary ==="
JS_SIZE=$(wc -c < "internal/web/static/js/components.bundle.js")
CSS_SIZE=$(wc -c < "internal/web/static/css/bundle.css")
JS_GZ=$(gzip -c "internal/web/static/js/components.bundle.js" | wc -c)
CSS_GZ=$(gzip -c "internal/web/static/css/bundle.css" | wc -c)

echo "JavaScript: $((JS_SIZE/1024)) KB raw, $((JS_GZ/1024)) KB gzipped"
echo "CSS:        $((CSS_SIZE/1024)) KB raw, $((CSS_GZ/1024)) KB gzipped"
echo "Total:      $(((JS_SIZE+CSS_SIZE)/1024)) KB raw, $(((JS_GZ+CSS_GZ)/1024)) KB gzipped"
echo ""
echo "HTTP requests reduced: 18+ -> 2 (JS + CSS bundles)"
echo ""

# Build Go binary
echo "Building Go binary..."
go build ./cmd/dogwatch
echo "Done! Binary: ./dogwatch"
