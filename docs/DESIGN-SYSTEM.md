# Design System — “Taller iluminado”

Visual and interaction specification for Inventory Desktop. The system comes
from the legacy GestiónStock UI. This document owns the desktop adaptation,
including the Colombian locale.

## Intent

- **Ambient working ground:** the shell uses a navy gradient with warm,
  steel, and blue glows plus a static grain tile.
- **Floating smoked navy rail:** the sidebar is translucent navy chrome,
  inset from the window edges.
- **Light glass panels:** content rides on light glass surfaces with a soft
  rim and shadow.
- **One accent:** yellow marks the brand, the focus halo, and at most one
  marker per screen. Primary actions are near-black.
- **Restraint:** hierarchy comes from type scale, 1px rules, and translucent
  panels, not heavy shadows.

## Tokens

Token definitions live in [`frontend/src/index.css`](../frontend/src/index.css).
Keep the token names used by the legacy app and do not add parallel systems.
Styling is CSS Modules plus those tokens; Tailwind is not used.

Legacy aliases (`--bg`, `--radius`, semantic colors) stay until every page is
migrated. Do not add a second token for a value that already exists.

## Typography

- UI: **Satoshi**.
- Data/code: **DM Mono**.
- No 700 weight in app UI.
- Sentence case. No uppercase tracked labels.
- Numeric values use `font-variant-numeric: tabular-nums`.

| Role | Size | Weight |
|---|---|---|
| Meta, labels, table headers | 12px | 400–500 |
| Body, table cells, nav | 14px | 400–500 |
| Section/card titles | 16px | 600 |
| Secondary KPI | 22px | 500 |
| Page title | 24px | 600 |
| Lead KPI / receipt total | 30px | 500 |

## Surfaces

| Surface | Border | Radius | Shadow |
|---|---|---|---|
| Card/panel | 1px glass border | 12px | rim + soft glass shadow |
| Control | 1px `--border` | 8px | none |
| Badge/chip | none | 4px | none |
| Dropdown/popover | 1px glass border | 8px | `--shadow-float` |
| Modal | 1px glass border + white rim | 16px | `--shadow-modal` |
| Item card | 1px `--border` | 8px | none |
| Receipt | 1px dark border | 12px | glass rim |
| Sidebar rail | 1px chrome border | 20px | soft shadow |

Glass panels share one soft shadow. Dropdowns and modals are the only
floating surfaces.

## Accent

Yellow is limited to:

- the sidebar brand mark;
- the `:focus-visible` halo;
- at most one marker per screen (today’s point, selected day);
- the sale/order receipt total.

It is not used for active nav pills, page-header underlines, selected
options, KPI values, or filled page buttons.

## Status colors

| Surface | Pending | Attention | Complete |
|---|---|---|---|
| Sale payment | neutral | amber | green |
| Sale delivery | neutral text | amber text | green text |
| Purchase order | neutral | sent = blue | received = green |
| Order payment | neutral text | — | green text |
| Stock | — | “Bajo mínimo” amber | blank |
| Zero stock | — | “Sin stock” red | — |

Pending is never red. Debt money is red only when greater than zero.

## Layout

### Desktop

- Ambient navy shell.
- Floating rail: 232px expanded, 80px collapsed, inset 12px, 20px radius.
- Nav groups: **Operación** and **Directorio**.
- 40px nav rows; neutral white active pill.
- User well at the bottom.
- Collapse state persists in `localStorage.sidebarCollapsed`.
- Main content sits above the ambient layers and never causes page-level
  horizontal scroll.

### Mobile / narrow window

- 56px top bar with a 44px menu button.
- Sidebar becomes a sheet with a scrim and body scroll lock.
- Closes on route change, Escape, scrim click, or close button.
- Focus returns to the menu button.
- 16px gutters; 44px touch targets.
- Tables become two-line rows or scroll inside their card.

## Forms

- Grid: one column below 1024px; `3fr 2fr` above (content left, sticky rail).
- Section cards: light glass, 12px radius, rim + shadow.
- Labels: 14px / 500.
- Controls: 36–44px tall, opaque white, 1px border, 8px radius.
- Focus: 2px ink ring plus the yellow halo.
- One filled near-black primary action per view.
- Section saves/cancels are ghost.
- Destructive actions live in a bottom **Zona de riesgo** card, never beside
  Save.
- Item cards: solid white, 1px border, 8px radius, no shadow, no accent edge.
- Pending item: dashed neutral. Error item: red border and red text.
- Receipt: navy glass on desktop; light glass with a navy total band below
  1024px. Total is 30px yellow.

## Data tables

- Header: solid `--table-head-bg`, 12px / 500, sticky, 1px bottom rule.
- Rows: transparent, 14px, 44px comfortable / 40px compact, 1px dividers.
- Hover: `--table-row-hover`.
- Numbers: right-aligned, tabular.
- Two-line cells: 14px ink + 12px meta.
- Pinned first column: solid `--table-pinned-bg`, right rule.
- Stock: 3px `StockMeter` above the minimum line.
- Loading: static grey skeleton bars, no pulse.
- Empty/error: keep the frame; show a message and a retry action.

## Components

Keep the legacy component names and behavior:

- `PageHeader`, `CommandBar`, `KpiStrip`, `DataCard`, `DataTable`,
  `Pagination`.
- `SearchSelect`, `FilterSelect`, `DateFilterInput`, `Dropdown`.
- `CreateFormModal`, `ConfirmDeleteModal`, `StockMeter`, `ErrorState`,
  `ErrorBoundary`.
- `ProductDropdown`, `AttributeTab`, `CardGridSkeleton`.
- Layout: `AppLayout`, sidebar, mobile sheet.

Dialogs:

- Portal, `role="dialog"`, `aria-modal`, `aria-labelledby`.
- Focus trap, Escape, initial focus, focus restore.
- Scrim uses `--scrim`; no app-wide backdrop blur.
- Close button has `aria-label="Cerrar"`.

## Pages

The route table lives in [`frontend/src/App.tsx`](../frontend/src/App.tsx) and
is documented in [FRONTEND.md](FRONTEND.md#routing).

Preserve the existing breadcrumbs, page titles, KPI placement, filters, table
columns, empty copy, and button names. UI copy stays Spanish.

## Density

| Band | Gutter | Section | Card pad | Control | Row |
|---|---|---|---|---|---|
| Comfortable (≥900px height) | 24 | 24 | 20 | 36 | 44 |
| Compact (700–899) | 20 | 20 | 16 | 32 | 40 |
| Short (<700) | 16 | 16 | 14 | 32 | 36 |
| Mobile (<768) | 16 | 16 | 16 | 44 | 44 |

## Motion

- Hover: 90ms.
- Dropdown/popover: 120ms.
- Modal: 160ms.
- Sidebar width: 240ms.
- No page entrance animations, no skeleton pulse, static ambient shell.
- `prefers-reduced-motion` removes transitions.
- `prefers-reduced-transparency` makes panels opaque and drops blur.
- `forced-colors` maps surfaces to Canvas/CanvasText.

## Locale and formatting

- Locale `es-CO`.
- Currency COP: `$ 1.234,56`.
- Dates: `DD/MM/YYYY`.
- Times: 24-hour, `HH:mm`.
- Numbers: `.` thousands, `,` decimals.
- Empty values: `—`.
- UI labels stay Spanish: Guardar, Cancelar, Editar, Eliminar, Cerrar.

## Accessibility

- Every control has a visible or programmatic label.
- Focus is always visible.
- Keyboard reaches every action.
- Tables have headers.
- Errors use `role="alert"`.
- Color is never the only status signal.
- Contrast meets WCAG AA.
- Skip link: “Saltar al contenido” → `#main-content`.
