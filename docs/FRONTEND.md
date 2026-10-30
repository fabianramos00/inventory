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
  - `['sale', id]`, `['saleItems', id]`
  - `['orders', ...]`, `['order', id]`, `['orderItems', id]`
  - `['clients', ...]`, `['providers', ...]`, `['users', ...]`
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
  sale changes only payment and client details; it does not change stock.
  These mutations invalidate `['sales']`, `['salesStats']`, `['products']`,
  `['product']` (detail prefix), `['inventoryStats']`, `['dashboardSummary']`,
  `['dashboardTrend']`, `['dashboardTopProducts']`, and
  `['dashboardLowStock']`. Item mutations also invalidate `['sale', id]` and
  `['saleItems', id]`. Clients changes invalidate `['clients']`.
- The create-sale payload is `payment_method`, `amount_paid`, `client_id`,
  and `items[{ product_id, quantity, delivered_quantity, unit_price }]`. It
  does not send `payment_status`; the server derives it from `amount_paid`.
  The create form shows that derived status as a read-only badge and does not
  offer a manual override.
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

### Dashboard

- Data comes from the four read-only `/dashboard/*` endpoints through
  `useDashboard`; the low-stock rail is not part of that contract. It uses
  `['dashboardLowStock']` (`stock_status: 'low_stock'`) and the exact
  `low_stock_count` from `['inventoryStats']`, falling back to the returned
  page length when stats fail.
- KPI cards: `Ventas de hoy` = `total_sales_today`, `Ventas sin pagar` =
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
