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

```text
frontend/
  src/
    components/       # shared UI
    context/          # AuthContext
    hooks/            # data and UI hooks
    lib/              # axios, api modules, utils, constants
    pages/            # route pages
    types/            # shared TypeScript types
    App.tsx
    main.tsx
    index.css
  e2e/
  vite.config.ts
```

Pages keep their folder and CSS Module convention:
`src/pages/Inventory/Inventory.tsx` and `Inventory.module.css`.

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

Current routes are `/#/login`, the protected `/#/` Home, `/#/inventory`,
`/#/inventory/create`, `/#/inventory/product/:id`, and the superuser-only
`/#/users` page, all under `AppLayout` except login. The remaining page files
stay in `src/pages` without routes until later phases restore the full table
from [DESIGN-SYSTEM.md](DESIGN-SYSTEM.md). Playwright specs must use hash URLs.

### Browser assumptions to review

| Assumption | Desktop action |
|---|---|
| `window.location` redirects | Use hash navigation; never replace the webview URL |
| `beforeunload` unsaved-changes warning | Keep for window close only if Wails supports it; otherwise use an in-app guard |
| `localStorage` session | Keep for v1; document the risk |
| `document.title` | Keep page titles |
| External links | Open in the OS browser through a Wails binding, not in the webview |

## Data fetching

- Use the existing hooks; do not call `useQuery` directly in pages.
- Query keys keep the legacy shape:
  - `['products', { page, search, ...filters, limit }]`
  - `['product', id]`
  - `['inventoryStats']`
  - `['sales', { page, ...filters, limit }]`
  - `['sale', id]`, `['saleItems', id]`
  - `['orders', ...]`, `['order', id]`, `['orderItems', id]`
  - `['clients', ...]`, `['providers', ...]`, `['users', ...]`
  - `['dashboardSummary']`, `['dashboardTrend']`, `['dashboardTopProducts']`,
    `['dashboardOrderStatus']`, `['dashboardLowStock']`
- Invalidate by prefix after writes. Forms call the API directly, then
  invalidate the related list, stats, and dashboard keys. Product updates
  also invalidate `['product', Number(id)]` so the detail view refetches the
  saved product and its relations.
- Product list responses use the cursor shape
  `{ items, has_next, skip, limit }`. The product write payload uses
  `low_stock_threshold`; the legacy `min_stock`/`min_stock_quantity` names are
  not sent.
- Keep `isLoading` for skeletons and `isFetching` for background refreshes.

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

```bash
cd frontend
pnpm lint
pnpm test
pnpm build
pnpm test:e2e
```

- Unit/component tests: Vitest + Testing Library.
- Hooks: test loading, success, error, pagination, invalidation.
- E2E: login, inventory CRUD, sale creation with stock validation, order
  receiving, user management, keyboard dialogs.
- Every bug fix leaves a regression test.

## Boundaries

- Frontend owns `frontend/**`.
- Do not edit Go code, migrations, `wails.json`, build scripts, or root docs.
- Report API changes to `backend`.
- Report design ambiguities to `ux-designer`.
