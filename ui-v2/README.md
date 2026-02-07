# dogwatch UI V2 (Solid + TypeScript)

## Purpose
Frontend V2 workspace for the product-wide UI rebuild.

## Run
```bash
npm install
npm run dev
```

Dev server proxies `/api/*` to `http://localhost:9999` by default.

To hydrate local data for V2 flows:
```bash
../scripts/seed-v2-data.sh
```

## Scope
- Shared shell + route model for V2
- Tokenized design system primitives
- Core workflow pages (starting with Detect/Investigate/Respond)
