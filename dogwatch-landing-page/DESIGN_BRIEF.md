# Dogwatch Web Aesthetic Brief

## Goal
Create a high-impact, sales-forward interface with a gritty operator feel: dark, tactical, information-rich, and visually distinctive.

## Visual Direction
- Industrial grunge, not clean SaaS minimalism.
- Dark base with subtle texture layers (grid, noise, scanlines, vignette).
- Acid green is the primary accent and conversion color.
- Dense UI with intentional hierarchy, not empty whitespace-heavy blocks.

## Brand + Header Rules
- Use the grunge dogwatch logo as primary brand mark.
- Keep masthead compact in height but allow the logo to feel prominent within it.
- Navigation should feel tactical: bordered tabs/chips, active state in acid green.

## Color System
- Base background: near-black (`#030303` to `#0b0b0b` range).
- Primary accent: acid green (`#ccff00`).
- Secondary accents: muted cyan and rust for depth, not primary actions.
- Body text: soft gray; key values/headlines in white; important deltas in acid green.

## Typography
- Headline style: bold, condensed, uppercase, high contrast.
- Body style: monospaced/operator tone for product credibility.
- Keep copy punchy and outcome-first (incident time, cost, migration friction).

## Composition Principles
- Hero must immediately communicate business outcome and operational proof.
- Use layered panels, rails, and status blocks to imply live system context.
- Prefer meaningful density over filler prose.
- Every section should answer: “Why this matters operationally or financially.”

## Component Language
- Panels: hard edges, thin borders, subtle glow/shadow.
- Chips/tags: compact, uppercase, telemetry-like labels.
- Metrics: emphasize deltas and concrete impact.
- CTA buttons: high-contrast, direct verbs (“Install”, “See Pricing”, “Explore”).

## Motion + Effects
- Keep motion purposeful and lightweight (stagger reveal, subtle drift).
- Respect reduced-motion and low-power devices.
- Avoid heavy continuous animation that hurts readability/perf.

## Performance Guardrails
- Keep pages static-first and fast.
- Prefer optimized assets (`.webp` where possible).
- Minimize JS to behavior-only enhancements.
- Avoid runtime-heavy styling systems on first paint.

## Do / Don’t
- Do: make it feel like a real operator console with business clarity.
- Do: keep copy specific, outcome-driven, and sales-usable.
- Don’t: generic dashboard cards with vague language.
- Don’t: over-decorate at the expense of legibility or speed.

## Asset Source of Truth
- `assets/grunge-revamp.css`
- `assets/grunge-revamp.js`
- `assets/tailwind.generated.css`
- `assets/dogwatch-logo.png`
- `assets/dogwatch-logo.webp`

## New Session Kickoff Prompt (Template)
Use this in the first message of the new Codex session:

“Use `DESIGN_BRIEF.md` and the transferred grunge assets as the visual source of truth. Preserve the Dogwatch industrial grunge aesthetic, strong sales hierarchy, and compact operator-style information density while revamping the actual app UI.”
