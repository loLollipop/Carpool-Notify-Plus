# CarpoolNotify Admin Design System

## Product posture

CarpoolNotify is an operations console, not a marketing dashboard. The interface should feel calm, dense and dependable: an administrator should know what needs attention, what changed financially and where capacity is constrained within one scan.

## Information hierarchy

1. Page title and primary action.
2. Four primary operating metrics at most.
3. Current work queue or decision surface.
4. Supporting charts and analysis.
5. Detailed records, filters and pagination.

Secondary metrics may use a compact row. Avoid presenting eight equally prominent cards because it hides which numbers matter.

## Navigation

- Workbench: dashboard, subscription log, goals.
- Customers & delivery: Team users, Plus rentals, redemptions, after-sales.
- Assets & finance: account list, billing analytics.
- System: settings.

## Visual language

- Background: quiet cool gray with very subtle teal ambient light.
- Surfaces: white/dark-neutral cards with crisp borders and soft elevation.
- Primary action and healthy operating state: teal.
- Warning/due soon: amber; destructive/overdue: red; analytical comparison only: blue/violet.
- Never use color as the only status signal; pair it with text or an icon.
- Use the sans family for all UI and money values. Monospace is reserved for codes, identifiers and technical metadata.

## Components

- Page headers remain compact and keep actions on the same visual baseline.
- KPI cards are clickable only when they open the matching records. Use tabular numerals and one short hint.
- Toolbars group search and filters in one bordered surface on record-heavy pages.
- Empty states explain what will appear and, when useful, provide one next action.
- Long settings pages keep section navigation and save controls visible.

## Charts

- Every chart must answer one operational question.
- Trends use an area or line chart; rankings use horizontal bars; composition uses a share band plus ranked list.
- Tooltips show exact values, while the chart itself stays readable without hovering.
- Focus the time window on meaningful operating data and label zero-data periods honestly.
- Avoid 3D charts, crowded legends, duplicated donut charts and unrelated mixed axes.

## Motion and accessibility

- Motion confirms state changes; it must not delay access to controls.
- Respect `prefers-reduced-motion`.
- Keep keyboard focus visible and interactive targets at least 36px on admin pages, 44px on the public redemption flow.
- Validate at 375px, 768px, 1280px and 1920px widths.
