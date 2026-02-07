# Component Inventory (V2)

## Foundation
- Tokens: color, type, spacing, radius, elevation, motion
- Layout primitives: `AppShell`, `PageFrame`, `Panel`, `Stack`, `Cluster`
- State primitives: loading, empty, error, offline, no-results

## Input + Control
- `Button`
- `IconButton`
- `SegmentedControl`
- `Input`
- `Select`
- `Combobox`
- `TimeRangePicker`
- `FilterChip`
- `Tabs`
- `CommandPalette`

## Data Display
- `DataTable`
- `VirtualList`
- `StatCard`
- `Badge` and `SeverityBadge`
- `Timeline`
- `Sparkline`
- `ChartPanel` (uPlot wrapper)

## Feedback + Overlays
- `Toast`
- `Modal`
- `Drawer`
- `ConfirmDialog`
- `Tooltip`

## Domain Components
- `AlertRow`
- `IncidentRow`
- `ServiceHealthCard`
- `TraceRow`
- `DeployRow`
- `CorrelationCard`
- `OnCallShiftCard`

## Page Templates
- Workspace template (logs/traces/query)
- List-detail template (alerts/incidents/catalog)
- Configuration template (integrations/admin/settings)

## Rules
- Domain pages can compose primitives and domain components only.
- No page-specific one-off style islands for shared interaction patterns.
