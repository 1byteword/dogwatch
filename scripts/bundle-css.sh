#!/bin/bash
# Bundle all CSS files into a single file for faster loading
# Run: ./scripts/bundle-css.sh

set -e

CSS_DIR="internal/web/static/css"
OUTPUT="internal/web/static/css/bundle.css"

echo "Bundling CSS..."

# Start with header
cat > "$OUTPUT" << 'EOF'
/**
 * dogwatch CSS Bundle
 * Auto-generated - do not edit directly
 * Run: ./scripts/bundle-css.sh
 */

EOF

# Order matters for CSS - load in correct cascade order
FILES=(
    "variables.css"
    "base.css"
    "components.css"
    "components-shared.css"
    "layout.css"
    "dashboard.css"
)

for file in "${FILES[@]}"; do
    if [ -f "$CSS_DIR/$file" ]; then
        echo "  Adding $file"
        echo "/* === $file === */" >> "$OUTPUT"
        cat "$CSS_DIR/$file" >> "$OUTPUT"
        echo "" >> "$OUTPUT"
    fi
done

# Get file size
SIZE=$(wc -c < "$OUTPUT")
SIZE_KB=$((SIZE / 1024))

echo "Bundle created: $OUTPUT ($SIZE_KB KB)"

echo "Done!"
