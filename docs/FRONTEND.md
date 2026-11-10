# Frontend

The frontend is the legacy React application from `../inventory-app`, adapted
to run inside Wails. Reuse components, hooks, and styles before writing new
ones.

## Stack

- React 19, TypeScript 6, Vite 8.
- TanStack Query v5 for server state.
- axios for HTTP.
- react-hook-form + zod for forms.
- CSS Modules and CSS custom properties.
- lucide-react icons.
- Vitest + Testing Library, Playwright for E2E.
- pnpm 12.

## Source and copy rules

- Source of truth for UI behavior: `../inventory-app/src`.
- Copy the app without `.git`, `node_modules`, `dist`, `test-results`,
  `playwright-report`, and `.env`.
- Keep the existing route structure and Spanish copy.
- Preserve accessibility behavior: labels, focus traps, skip link, keyboard
  dropdowns, `aria-*` attributes.
- Do not introduce a component library or a state manager.

## Structure

The current layout is [`frontend/src/`](../frontend/src). Pages keep their
folder and CSS Module convention: `src/pages/Inventory/Inventory.tsx` and
`Inventory.module.css`. Tokens and shared utilities live in
[`src/index.css`](../frontend/src/index.css); styling is CSS Modules and
custom properties, with no Tailwind.

## Desktop adaptations

### API base URL

`src/lib/axios.ts` must not read `VITE_API_URL` at module load for the
desktop build. Bootstrap the base URL before rendering:

```ts
// src/bootstrap.ts
export async function resolveApiBaseUrl(options: { isProduction?: boolean } = {}) {
  const isProduction = options.isProduction ?? !import.meta.env.DEV
  const binding = window.go?.main?.App?.GetAPIBaseURL
  // Call the binding and validate the result, then set it with setApiBaseUrl().
}
```

The shipped build is fail-closed:

- Production (`!import.meta.env.DEV`) requires the Wails
  `GetAPIBaseURL` binding. If it is missing, throws, or returns an invalid
  URL, `resolveApiBaseUrl()` throws and the app does not start.
- A binding result is accepted only when it is exactly
  `http://127.0.0.1:<port>/api/v1` or `http://localhost:<port>/api/v1`,
  with a port, the `/api/v1` path, and no credentials, query, or hash.
- Production never falls back to a fixed port or `VITE_API_URL`. A local
  process listening on the old `8787` port cannot receive credentials.
- Development may fall back to `VITE_API_URL` or
  `DEFAULT_API_BASE_URL` (`http://127.0.0.1:8787/api/v1`) when the binding
  is unavailable or invalid. `VITE_API_URL` must include the `/api/v1`
  prefix.

`main.tsx` awaits `bootstrap()` before creating the React root. If bootstrap
fails, it renders the Spanish startup error into `#root` instead of a blank
window and does not expose internal details. The axios instance reads the
configured base URL from `apiBase.ts` on every request and reuses it for
refresh. The generated `frontend/wailsjs` binding is optional at build time,
so a missing binding cannot break `pnpm build`; it breaks the release at
runtime by design.

### Routing

Use `HashRouter` in the desktop build. The legacy route table stays the same;
only the router wrapper changes. Browser dev may keep `BrowserRouter` behind
`import.meta.env.DEV`, but the shipped build must be hash-based.

The full route table lives in [`src/App.tsx`](../frontend/src/App.tsx). All
listed routes are implemented; protected pages sit under `AppLayout`, and
`/#/users` is additionally gated by the superuser route. Playwright specs must
use hash URLs.

### Health check and offline banner

- `GET /healthz` is unauthenticated and returns
  `{ status: "ok", database: "sqlite" | "postgres" }`.
- `src/lib/api/health.ts` derives the root URL by stripping the `/api/v1`
  suffix from `getApiBaseUrl()`, then requests `/healthz` before login. It uses
  the plain axios client so no bearer token or refresh flow is involved.
- `OfflineDatabaseBanner` renders inside `App` above the router, so it is
  visible on `/#/login` and on every protected route. It renders only when
  `database === "sqlite"`; it renders nothing for `postgres` or when the health
  request fails. Health failure never blocks startup.
- The banner is non-dismissable and uses `role="status"` without taking focus.
  Its Spanish copy explains that data is stored locally on this computer, is
  not synchronized, and the local file is not encrypted; users must protect
  the equipment and keep backups.

### Offline banner offset contract

The banner sits above the router and pushes the page down by its rendered
height. Because the sidebar, mobile header, and skip link are `position: fixed`,
they must offset themselves instead of assuming the top of the viewport:

- `OfflineDatabaseBanner` measures its own height with `ResizeObserver` and
  sets `--offline-banner-height` on `document.documentElement`. The variable is
  only present while `database === "sqlite"`; it is removed on cleanup, on
  unmount, and whenever the banner is not rendered. The text wraps on narrow
  windows, so the value is measured rather than hardcoded.
- Consumers read it with a `0px` fallback, e.g.
  `top: var(--offline-banner-height, 0px)`. Postgres mode therefore keeps the
  original layout.
- Sidebar desktop: `top: calc(12px + var(--offline-banner-height, 0px))` and
  `height: calc(100dvh - 24px - var(--offline-banner-height, 0px))`.
- Sidebar mobile and the mobile header start at
  `top: var(--offline-banner-height, 0px)`.
- The skip link starts at
  `top: calc(8px + var(--offline-banner-height, 0px))` so it is not hidden
  under the banner.
- The app shell, main content, and login container use
  `min-height: calc(100dvh - var(--offline-banner-height, 0px))` so the banner
  does not add an artificial extra viewport of scroll.
- Unit coverage asserts the variable is set for sqlite and removed on unmount
  or non-sqlite. The visual overlap itself is covered by Playwright, where
  geometry and stacking are observable.

### Inline notices

Transient success confirmations use
[`src/components/InlineNotice/InlineNotice.tsx`](../frontend/src/components/InlineNotice/InlineNotice.tsx)
and its CSS Module. Do not add another toast or notification layer.

- Props: `tone` (`success` | `error` | `info`), `onClose`, optional
  `autoCloseMs`, and the message as `children`.
- Success notices auto-dismiss after 5000 ms by default. Pass `autoCloseMs` to
  override the window; `0` disables auto-dismiss.
- Error notices never auto-dismiss; they remain until the user closes them.
  `tone="error"` uses `role="alert"`; success and info use `role="status"`
  with `aria-live="polite"`.
- The close button is visible, keyboard reachable, named
  `Cerrar notificación`, and has `:focus-visible` styling.
- Hovering or focusing inside the notice pauses the timer. Leaving or blurring
  restarts the full window. The timer is cleared on unmount and on manual
  close, and it cannot fire after close.
- Auto-dismiss never moves focus. If the close button has focus, the notice
  stays until focus leaves.
- Use it for completion feedback after a mutation (payments, create, update,
  delete). Do not use it for permanent metadata, status badges, or form
  validation errors.

Current consumers: client payments on `/#/clients` and `/#/clients/:id`, sale
payments and sale updates in `/#/sales/:id`, and create/update/delete
confirmations for clients, products, providers, users, and sales. Product
create/delete messages travel through router state and are consumed by the
inventory list.

### Floating menus

Every menu that can open near the bottom of a page is portaled to
`document.body` and positioned with `position: fixed`, so it never extends the
scrollable overflow of the document.

- `src/hooks/useFloatingMenu.ts` owns the placement. It measures the trigger
  with `getBoundingClientRect()`, compares the space above and below with
  `window.innerHeight`, and opens the menu downward when there is room below,
  upward when there is not, and clamped to the roomier side when neither side
  fits. The placement math is the pure function `computeFloatingPlacement`,
  which is unit-tested with a table.
- The hook returns inline styles only (`position`, `top`, `left`, width and the
  `--floating-menu-max-height` custom property). It carries no CSS class names:
  each component keeps its own CSS Module for the visual design.
- The height available on the chosen side is published as
  `--floating-menu-max-height`. A menu with its own design cap declares it as
  `--floating-menu-cap` and uses
  `max-height: min(var(--floating-menu-cap), var(--floating-menu-max-height, var(--floating-menu-cap)))`.
  The list inside the menu scrolls when the space is not enough.
- The menu is repositioned while it is open on scroll and on resize.
- The portal breaks the DOM relation between the trigger and the menu, so:
  - `useClickOutside` takes the portaled panel as an extra "inside" node,
    otherwise the `mousedown` that opens the menu would close it again and the
    selection would be lost.
  - `useDialogFocus` skips the focus trap while the focus is inside a portaled
    menu that belongs to the dialog, so the trap does not pull the focus back.
  - `useListbox` returns the focus to the trigger on `Tab`, because the menu is
    no longer a DOM sibling of the trigger.
  - Focus on open uses `focus({ preventScroll: true })` and the focused option
    is scrolled into view with `scrollIntoView({ block: 'nearest' })`, so
    opening a menu never scrolls the page.
- `Escape` inside a menu calls `stopPropagation()`, so it closes only the menu.
  The document-level `Escape` listener of `useDialogFocus` must not close the
  dialog that owns the menu.
- Portaled menus are direct children of `document.body` and carry the
  `data-floating-menu` attribute. Tests and E2E specs must look them up from
  the page, not from the dialog that opened them.
- Menus that use the hook: `Dropdown`, `ProductDropdown`, `FilterSelect`,
  `SearchSelect`, `CalendarDropdown` and the product search results of
  `SaleForm`.

### Sale page layout

The sale page does not use the shared `3fr 2fr` form grid. It renders a
fixed rail, because the receipt total has a hard minimum width:

```css
/* base (narrow band) */
.saleGrid { grid-template-columns: minmax(0, 1fr); gap: 16px; align-items: start; }

/* 1024–1279 */
@media (min-width: 1024px) {
  .saleGrid { grid-template-columns: minmax(0, 1fr) 320px; gap: 16px; }
}

/* ≥1280 */
@media (min-width: 1280px) {
  .saleGrid { grid-template-columns: minmax(0, 1fr) 360px; gap: 20px; }
}
```

- The rail width is fixed, not a ratio: the total needs 252px plus 40px of
  padding, so 320px is the narrowest rail that keeps it on one line.
- The page container is capped at `max-width: 1360px` and centered, so the
  products card does not become a wide strip on large windows.
- Page-specific classes live in
  [`src/pages/SaleForm/SaleForm.module.css`](../frontend/src/pages/SaleForm/SaleForm.module.css).
  [`src/pages/formPage.module.css`](../frontend/src/pages/formPage.module.css)
  keeps the shared `3fr 2fr` grid and the sticky `.rightColumn` used by
  `ProductForm` and `OrderForm`; it is not changed by the sale page.

Cells:

- Left column: the `Productos` card, plus the `Historial de pagos` card
  directly below it in edit mode. `Zona de riesgo` stays full width under
  the grid.
- Right rail (sticky from 1024px, `top: var(--page-gutter)`):
  1. `Resumen` receipt. Create: `Ítems` (row hidden while the count is 0) ·
     `Saldo por cobrar` (red only while it is above zero) · `Total estimado`.
     Edit: `Ítems` · `Pagado` · `Saldo` · `Total`.
     The total row stacks — label above, 30px value below, right-aligned —
     so it cannot overflow the 320–360px rail.
  2. `Pago y detalles` (create) or `Detalles de venta` (edit).
- `Guardar Venta` sits at the end of the rail, next to the money, so it
  stays reachable while the rail is sticky. There is exactly one save
  button in the DOM; `Cancelar` stays in the page header.

### Create-sale field order

In create mode the payment fields are rendered in this order: `Monto pagado`
first, `Método de pago` second. The method field is always rendered and
enabled; it is required only when `amount_paid > 0`. The order does not
change the payload, the validation or the derived `payment_status` badge.

### Create-sale validation

- The submit button is disabled only while saving. A failed submit focuses
  the first invalid control and shows the error next to it; the top-of-page
  error box carries API errors only.
- The invalid controls are: no items → the product search
  (`Agrega al menos un producto a la venta.`), `delivered_quantity >
  quantity` → that item's `Cant. entregada`, amount above the total →
  `Monto pagado`, amount above zero without a method → `Método de pago`, and
  `amount_paid < total` without a client → the `Cliente` selector
  (`Una venta con saldo pendiente requiere un cliente.`).
- The receipt total is a live region (`aria-live="polite"`), so a total
  change is announced.

### Browser assumptions to review

| Assumption | Desktop action |
|---|---|
| `window.location` redirects | Use hash navigation; never replace the webview URL |
| `beforeunload` unsaved-changes warning | Keep for window close only if Wails supports it; otherwise use an in-app guard |
| `localStorage` session | Keep for v1; document the risk |
| `document.title` | Keep page titles |
| External links | Open in the OS browser through a Wails binding, not in the webview |

## Data fetching

- Use the existing hooks; do not call `useQuery` directly in pages. List
  pages, including Clients and Providers, call `usePagedList` directly.
- Query keys keep the legacy shape:
  - `['products', { page, search, ...filters, limit }]`
  - `['product', id]`
  - `['inventoryStats']`
  - `['sales', { page, ...filters, limit }]`, `['salesStats']`
  - `['sale', id]`, `['saleItems', id]`, `['salePayments', id]`
  - `['orders', ...]`, `['order', id]`, `['orderItems', id]`
  - `['clients', ...]`, `['client', id]`, `['providers', ...]`, `['users', ...]`
  - `['dashboardSummary']`, `['dashboardTrend']`, `['dashboardTopProducts']`,
    `['dashboardOrderStatus']`, `['dashboardLowStock']`
- Invalidate by prefix after writes. Forms call the API directly, then
  invalidate the related list, stats, and dashboard keys. Product updates
  also invalidate `['product', Number(id)]` so the detail view refetches the
  saved product and its relations.
- Provider changes invalidate `['providers']`. Provider create omits empty
  optional fields; provider edit sends explicit `null` for cleared
  `contact_info`, `email`, or `phone`.
- Sale create/delete and sale-item add/update/delete change stock. Updating a
  sale changes only client details; it does not change stock or payments.
  Because client debt is derived from sales, every sale write also invalidates
  `['clients']` (list prefix) and `['client']` (detail prefix) so the client
  header cannot keep a stale `debt_amount` within `staleTime`.
  These mutations invalidate `['sales']`, `['salesStats']`, `['products']`,
  `['product']` (detail prefix), `['inventoryStats']`, `['dashboardSummary']`,
  `['dashboardTrend']`, `['dashboardTopProducts']`, and
  `['dashboardLowStock']`. Item mutations also invalidate `['sale', id]` and
  `['saleItems', id]`. Client create/edit/delete invalidates `['clients']`; a
  client edit also invalidates `['client', id]`. Client-level payments
  invalidate `['clients']`, `['client', id]`, `['sales']`, `['salesStats']`,
  the affected `['sale', id]`, `['saleItems', id]`, and
  `['salePayments', id]` prefixes, plus the dashboard keys. See
  [Client detail and client payments](#client-detail-and-client-payments).
- The Sales `Usuario` filter reads `GET /users/options` through
  `getUserOptions`, which any authenticated user may call. The Users admin
  page keeps `GET /users`, which is superuser-only.
- Sale payments use `GET /sales/{id}/payments?skip=0&limit=5` (cursor via
  `has_next`, newest first) and `POST`/`DELETE /sales/{id}/payments` returning
  the updated sale. Payment writes invalidate the same sale, client, product,
  inventory, and dashboard keys through `invalidateSaleData`. The modal
  registers one payment and the history card loads older pages with `Ver más`;
  deleting a payment asks for confirmation and refreshes the same prefixes.
- The create-sale payload is `payment_method`, `amount_paid`, `client_id`,
  and `items[{ product_id, quantity, delivered_quantity, unit_price }]`. It
  does not send `payment_status`; the server derives it from `amount_paid`.
  The method field is always rendered and enabled; it is required only when
  `amount_paid > 0`, and the payload sends `payment_method: null` when
  `amount_paid` is 0. `client_id` is required when `amount_paid < total`; it
  stays `null` only for fully paid anonymous sales. The create form shows the
  derived status as a read-only badge and does not offer a manual override.
- Sale detail hides `Pagar` when `debt_amount` is 0 or the sale is paid. The
  payment history renders date/time (`DD/MM/YYYY`, 24h), method, registering
  user, and amount. Legacy payments may have `payment_method` or
  `created_by` as `null`; the history and delete confirmation then render
  `Sin especificar` and `Sin usuario`, never the raw value. Payment delete
  errors keep the confirmation open and surface through the modal's error
  box. Sale edit sends only `client_id` (explicit `null` when cleared); it
  never sends `amount_paid` or `payment_method`. The edit details card
  shows `Cliente`, `Estado entrega`, `Registrado por` and `Fecha`; the
  payment status stays in the page title badge and the amounts stay in the
  receipt, so nothing is duplicated.
- Sales date inputs emit `YYYY-MM-DD`; `useSales` converts `start_date` to
  the Bogota start of day and `end_date` to the inclusive Bogota end of day
  as RFC 3339 instants before calling the API. `useOrders` applies the same
  conversion to `min_date`/`max_date`. Changing either date filter resets the
  page to 1.
- Purchase-order create sends exactly `provider_id`, `status`,
  `payment_status`, and `items[{ product_id, quantity, unit_cost,
  supplier_sku? }]`; `supplier_sku` is omitted when empty. Details updates
  send only `provider_id`, `status`, and `payment_status`. Item add sends
  `{ product_id, quantity, unit_cost, supplier_sku? }`; item update sends
  `{ quantity, unit_cost, supplier_sku }` and sends `null` to clear
  `supplier_sku`; item delete uses the item id.
- Create/delete order invalidates `['orders']` and the dashboard keys. A
  details update invalidates `['order', id]`, `['orders']`, and the dashboard
  keys. Moving an order to `received` also invalidates `['product']` (detail
  prefix), `['products']`, `['inventoryStats']`, and the supplier-catalog key
  so stock reflects the backend increment. Item mutations invalidate
  `['order', id]`, `['orderItems', id]`, and `['orders']`; they never
  invalidate stock, because the backend does not adjust stock for received
  item edits. Deleting an order never reverses stock already received.
- API errors surface the backend `response.data.detail` through
  `getAPIErrorMessage(error, fallback)`; plain `Error` messages and the
  Spanish fallback are used when no detail exists. Error boxes use
  `role="alert"`.
- Product, sales, purchase-order, order-item, client, and provider list
  responses use the cursor shape `{ items, has_next, skip, limit }`. Provider
  search matches name, email, and phone; `contact_info` is not searched. The
  product write payload uses `low_stock_threshold`; the legacy
  `min_stock`/`min_stock_quantity` names are not sent. Pagination is
  cursor-only: lists use `has_next`, and there is no total count or page range.
- Keep `isLoading` for skeletons and `isFetching` for background refreshes.

### Client detail and client payments

- Route `/clients/:id`, entered from the client name on `/clients`. The client
  list card and the detail header show `Saldo pendiente` from
  `client.debt_amount`; it is red only when above zero, and the header shows
  `Sin deuda` at zero.
- Each client card with debt above zero also shows `Pagar` next to `Editar` and
  `Eliminar`. It opens the same `ClientPaymentModal` used by the detail page.
  After a successful payment the list refetches `['clients']` and
  `['client', id]`, so the card shows the new balance; the action stays while
  debt remains and disappears at zero. The trigger is `Pagar deuda de <name>`
  for assistive technology, and focus returns to `Pagar` if debt remains or to
  the `role="status"` success message when the action disappears.
- The header carries identity/contact and `Editar cliente` (ghost) and
  `Pagar deuda` (the only near-black primary) while debt is above zero. Editing
  reuses `CreateFormModal` with `CLIENT_FIELDS` and invalidates `['client', id]`
  and `['clients']`.
- `Ventas del cliente` reuses `useSales` with `client_id`, newest first and
  cursor paginated. Filters are payment status (`Todas`, `Pendiente`,
  `Parcial`, `Pagada`), delivery status (`Todos`, `Pendiente`, `Parcial`,
  `Entregado`), and `Desde`/`Hasta`; each change resets to page 1. An inverted
  date range shows the inline Spanish error and the list ignores both dates
  until corrected.
- Columns are `Fecha`, `Total`, `Pagado`, `Saldo`, `Estado pago`,
  `Estado entrega`, and `Ver`. `Ver` and a row click open `/sales/:id`; the
  client page has no per-row payment action.
- `ClientPaymentModal` calls
  `POST /sales/clients/{id}/payments` with `{ amount, payment_method }` from
  `Pagar todo` (fixed to the remaining debt) or `Pago parcial`. It shows the
  remaining-debt preview, the oldest-first note, inline `role="alert"`
  validation, keeps the values on API error, and disables submit while
  pending. The page reports success through an inline `role="status"` and
  refreshes the client, list, affected sales, stats, and dashboard keys.
- The sale form requires a client when `amount_paid < total`; otherwise it
  shows `Una venta con saldo pendiente requiere un cliente.` next to the client
  field and focuses that control. Editing a sale with debt cannot clear the
  client. A fully paid anonymous sale remains valid.

### Dashboard

- Data comes from the four read-only `/dashboard/*` endpoints through
  `useDashboard`; the low-stock rail is not part of that contract. It uses
  `['dashboardLowStock']` (`stock_status: 'low_stock'`) and the exact
  `low_stock_count` from `['inventoryStats']`, falling back to the returned
  page length when stats fail.
- KPI cards: `Ventas de hoy` = `total_sales_today`, `Saldo por cobrar` =
  `pending_payment_sales`, `Pedidos pendientes` = `pending_orders`, and
  `Stock bajo` = inventory stats. Money fields are COP currency
  (`$ 1.234,56`); `pending_payment_sales` is the outstanding balance, never a
  sale count. `delivered_orders` counts delivered sales, not purchase orders,
  and is not shown in the KPI strip.
- Trend `date` keys are `YYYY-MM-DD` Bogota calendar days. Derive weekday
  labels from the string itself (UTC noon/midnight) so the host timezone never
  shifts the displayed day. Seven zero-filled days are valid data; an empty
  array is the empty state.
- Top products may return `null` brand, category, or material names. The table
  renders the brand with a `-` fallback, and nullable category/material names
  must not break the row. The table shows the first five products returned by
  the API.
- Order status distribution is purchase orders grouped by status with
  `count` and `percentage`; an empty array renders the empty state. Each
  section shows its own error and retry action.

## Auth

- `AuthContext` keeps access token, refresh token, and the full user
  (`id`, `full_name`, `email`, `is_active`, `is_superuser`); older partial
  caches are normalized on read.
- The axios request interceptor adds `Authorization: Bearer`.
- The response interceptor handles one refresh at a time, queues concurrent
  401s, rotates the refresh token, and emits the auth-expired event on
  failure.
- After a successful refresh, the interceptor stores the rotated tokens and
  emits `auth:refreshed`; `AuthContext` listens and updates its token and
  refresh-token state so logout sends the current refresh token.
- Login uses `application/x-www-form-urlencoded` with `username` = email.
- Never log tokens or passwords.
- A 401 from login or refresh is not retried.

## Forms

- react-hook-form with zod resolvers.
- Validate required fields, email format, password length, non-negative
  quantities, `delivered_quantity <= quantity`, `amount_paid <= total`.
- Server-side validation is still mandatory.
- Show field errors inline; show API errors with `role="alert"`.
- Every validated field renders its inline error next to the input with the
  shared `styles.errorMessage` pattern; a field that blocks submit without a
  visible message is a defect.
- Disable submit while saving; keep a single primary action per view.

## Styling

- Use `var(--token)` values; never raw hex values in components.
- Keep the design system in [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md).
- CSS Modules per page/component; shared utilities live in `index.css`.
- Mobile-first responsive behavior; tables scroll inside their wrapper.
- No page-level horizontal scroll.
- Respect `prefers-reduced-motion` and `prefers-reduced-transparency`.

## Locale

Use `es-CO`:

```ts
new Intl.NumberFormat('es-CO', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
```

Display money as `$ 1.234,56`, dates as `DD/MM/YYYY`, and numbers with
Colombian separators. Replace the legacy `es-PE` formatters and update the
formatter tests.

## Testing

Commands and coverage live in [TESTING.md](TESTING.md). Every bug fix leaves a
regression test.

## Boundaries

- Frontend owns `frontend/**`.
- Do not edit Go code, migrations, `wails.json`, build scripts, or root docs.
- Report API changes to `backend`.
- Report design ambiguities to `ux-designer`.
