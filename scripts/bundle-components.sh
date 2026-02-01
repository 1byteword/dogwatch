#!/bin/bash
# Bundle all web components into a single file for faster loading
# Run: ./scripts/bundle-components.sh

set -e

COMPONENTS_DIR="internal/web/static/js/components"
OUTPUT="internal/web/static/js/components.bundle.js"

echo "Bundling components..."

# Start with header
cat > "$OUTPUT" << 'EOF'
/**
 * dogwatch Components Bundle
 * Auto-generated - do not edit directly
 * Run: ./scripts/bundle-components.sh
 */
(function() {
'use strict';

EOF

# Add base component first
if [ -f "$COMPONENTS_DIR/base.js" ]; then
    echo "// === base.js ===" >> "$OUTPUT"
    cat "$COMPONENTS_DIR/base.js" >> "$OUTPUT"
    echo "" >> "$OUTPUT"
fi

# Add all other components
for file in "$COMPONENTS_DIR"/*.js; do
    basename=$(basename "$file")
    if [ "$basename" != "base.js" ] && [ "$basename" != "components.bundle.js" ]; then
        echo "  Adding $basename"
        echo "// === $basename ===" >> "$OUTPUT"
        cat "$file" >> "$OUTPUT"
        echo "" >> "$OUTPUT"
    fi
done

# Close IIFE
echo "})();" >> "$OUTPUT"

# Get file size
SIZE=$(wc -c < "$OUTPUT")
SIZE_KB=$((SIZE / 1024))

echo "Bundle created: $OUTPUT ($SIZE_KB KB)"

# Optionally minify if terser is available
if command -v terser &> /dev/null; then
    echo "Minifying..."
    terser "$OUTPUT" -o "${OUTPUT%.js}.min.js" -c -m
    MIN_SIZE=$(wc -c < "${OUTPUT%.js}.min.js")
    MIN_KB=$((MIN_SIZE / 1024))
    echo "Minified: ${OUTPUT%.js}.min.js ($MIN_KB KB)"
fi

echo "Done!"
