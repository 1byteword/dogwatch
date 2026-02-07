# Acceptance Criteria (Phase 0/1)

## Product Coherence
- Every V2 page uses shared shell + tokenized components.
- No local page theme forks for V2 routes.

## Workflow UX
- Alert -> first meaningful action in <= 3 interactions.
- Incident state changes are available from incident command center.
- Service view includes reliability + correlation context in one page.

## Performance
- V2 first interactive render target: <= 1.5s on typical laptop.
- Common interaction latency target: <= 100ms.
- Route transition target: <= 250ms perceived.

## Engineering Quality
- No inline `style` or inline event handlers in new V2 code.
- TypeScript strict mode enabled for new frontend modules.
- CI includes UI audit and baseline visual regression checks.

## Accessibility
- Keyboard navigation supported for primary controls.
- Visible focus states for actionable controls.
- Contrast meets dark-theme operational readability standards.

## Migration Safety
- Legacy routes remain functional during transition.
- V2 routes can be enabled incrementally behind feature flags.
