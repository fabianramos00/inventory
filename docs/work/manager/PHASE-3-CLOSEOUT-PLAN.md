# Phase 3 Closeout Plan

Status: active
Owner: manager
Current state: 63 of 67 legacy endpoints implemented
Remaining feature: Dashboard (4 endpoints)
Exit goal: Phase 3 feature-complete, parity-tested, E2E-tested, documented, and release-checked

## 1. Current state

| Module | Endpoints | Status |
|---|---:|---|
| Auth | 3 | Implemented |
| Users | 5 | Implemented |
| Catalog | 18 | Implemented |
| Products | 6 | Implemented |
| Clients | 5 | Implemented |
| Sales | 10 | Implemented |
| Providers | 5 | Implemented |
| Supplier catalogs | 2 | Implemented |
| Purchase orders | 9 | Implemented |
| Dashboard | 4 | **Pending** |

Cross-cutting gaps:

- Live FastAPI-vs-Go parity has not been run for any module.
- Playwright covers only auth/users; inventory, sales, orders, clients, providers, and dashboard are Vitest-only.
- Docs still describe several implemented modules as pending.
- Release checks (Wails smoke, packaging, signing, backup/restore) are not complete.

## 2. Workstream A — Dashboard feature

### A1. Freeze contract

Create `docs/work/manager/PHASE-DASHBOARD-CONTRACT.md` with:

- summary:
  - `inventory_value = SUM(product.price * product.stock_quantity)` at sale price;
  - `total_sales_today` = sales created since midnight `America/Bogota`;
  - `pending_payment_sales` = `SUM(total_amount - amount_paid)` for sales whose
    payment status is not `paid` (money, not count);
  - `pending_orders` = count of purchase orders with status `pending`;
  - `delivered_orders` = count of **sales** with delivery status `delivered`.
- sales-trend:
  - last 7 Bogota days inclusive, zero-filled;
  - `date` = `YYYY-MM-DD`;
  - `sales_amount` = sum of sale totals per Bogota day;
  - `orders_cost` = sum of purchase-order `total_amount` per Bogota day.
- top-products:
  - last 7 days, grouped by product;
  - order by `SUM(sale_item.subtotal) DESC`;
  - `total_quantity_sold = SUM(delivered_quantity)`;
  - `total_sales_amount = SUM(subtotal)`;
  - brand/category/material names nullable.
- order-status-distribution:
  - counts by purchase-order status;
  - percentage = count / total * 100, `0` when empty;
  - statuses lowercase.

Database rules:

- PostgreSQL: `AT TIME ZONE 'America/Bogota'` for day grouping.
- SQLite: use the fixed `-05:00` offset (Colombia has no DST) or Go day
  bucketing; do not use the host timezone.
- Money: decimal in Go, integer cents on SQLite, `NUMERIC(12,2)` on PostgreSQL.
- Read-only; no new tables or migrations.

### A2. Backend

Owner: `backend`
Files: `internal/domain/dashboard.go`, ports, application service,
`gormstore/dashboard.go`, `httpapi/dashboard.go`, router, bootstrap, main.

- Four GET endpoints under `/api/v1/dashboard`.
- Any active user.
- PostgreSQL and SQLite queries with identical observable results.
- Money exact; percentages as numbers.
- Tests: empty DB zeros, one sale today, sale before Bogota midnight, 7-day
  zero fill, top-products ordering by subtotal while quantity uses delivered,
  status distribution percentages.

### A3. Frontend

Owner: `frontend`
Existing page/hook: `Dashboard.tsx`, `useDashboard.ts`, `dashboard.ts`.

- Wire the four endpoints; remove the current low-stock workaround if the
  dashboard contract covers it (keep the existing low-stock card by using the
  products stats endpoint if needed).
- Verify chart date labels use Bogota dates.
- Verify `pending_payment_sales` is rendered as currency, not a count.
- Verify empty/loading/error states and retries.
- Cache invalidation on sale/order changes is already in place.
- Add/refresh component tests for KPI formatting, trend data, top products,
  and empty data.

### A4. QA

Owner: `qa`

- Independent HTTP verification on PostgreSQL + SQLite.
- Assert Bogota day boundary: a sale at 23:59 Bogota and one at 00:01 next day.
- Assert zero-fill for all 7 days.
- Assert top-products ranking and delivered-quantity quirk.
- Assert status distribution percentages and empty DB.
- Assert frontend dashboard renders currency/count/percent correctly.

## 3. Workstream B — Legacy parity harness

Owner: `qa`, coordinated by `manager`

1. Create `testdata/contract/` fixtures for all modules:
   - auth, users, catalog, products, clients, sales, providers, supplier
     catalogs, purchase orders, dashboard.
2. Each fixture has method, path, auth role, body, and expected status.
3. Runner:
   - seeds identical data in FastAPI and Go;
   - sends the same request to both;
   - compares status, shape, ordering, and values;
   - normalizes generated IDs and timestamps;
   - compares money as decimals;
   - writes `testdata/contract/report.md` on failure.
4. Add a `contract` build tag and document the command in `docs/TESTING.md`.
5. Run with:
   ```bash
   LEGACY_API_URL=... LEGACY_USER=... LEGACY_PASSWORD=... \
     go test -tags=contract ./...
   ```
6. If the legacy service is unavailable, run saved fixtures in replay mode and
   record the reason. Do not mark parity as passing without a run.
7. Promote matrix rows from “Backend contract covered; legacy FastAPI parity
   pending” only after a recorded run.

Acceptance: all 67 endpoints either pass or have an explicit approved
difference in `MIGRATION.md` and `DECISIONS.md`.

## 4. Workstream C — Playwright E2E

Owner: `qa` + `frontend`, configured by `devops`

Current `frontend/playwright.config.ts` only runs `auth.spec.ts` and
`users.spec.ts`. The mocked specs are excluded.

1. Convert the mocked specs to full-stack where routes exist:
   - inventory list/filter/create/edit/delete;
   - clients list/search/create/edit/delete;
   - providers list/search/create/edit/delete;
   - sales list/filter/create/delete and item edits;
   - purchase orders list/create/receive/delete and item edits;
   - dashboard summary/charts/empty state.
2. Add dashboard and new modules to `testMatch`.
3. Use hash URLs.
4. Assert stock changes after sale creation/deletion and order receiving.
5. Keep specs deterministic; no sleeps.
6. Run against the Go server and Vite dev server from the existing
   `webServer` config.

Acceptance: `pnpm test:e2e` runs all listed flows against PostgreSQL.

## 5. Workstream D — Documentation cleanup

Owner: `backend` + `frontend` (docs), `manager` (plan/status)

Update stale claims after Dashboard lands:

- `README.md`: Phase 3 status, implemented modules, SQLite business schema.
- `AGENTS.md`: current phase and implemented-module list.
- `docs/ARCHITECTURE.md`: purchase orders and dashboard implemented.
- `docs/MIGRATION.md`: remove “purchase orders/dashboard pending”; record any
  final differences.
- `docs/RELEASE.md`: update pending-module list and release checklist.
- `docs/TESTING.md`: dashboard coverage, Playwright coverage, parity status.
- `docs/FRONTEND.md`: dashboard route/contract if changed.
- `docs/SYNC.md`: providers/catalogs/orders implemented; sync still not.

Acceptance: no doc says an implemented module is pending; parity labels are the
weakest true label.

## 6. Workstream E — Release and security closeout

Owner: `devops` + `security`

1. `devops`:
   - add `scripts/verify.sh` running backend, frontend, lint, and build;
   - run Wails Linux build;
   - run Windows/macOS packaging where the environment supports it;
   - verify versioning, checksums, and signing steps;
   - run backup/restore rehearsal.
2. `security`:
   - review loopback binding, auth on all 67 endpoints, CORS, secret storage,
     SQLite file permissions, and token storage;
   - confirm no secrets in logs or repo.
3. `qa`:
   - Wails smoke test: launch, login, create a sale, restart, confirm data
     persists;
   - confirm dashboard and stock values after restart.

Acceptance: release checklist has no open Critical/High findings and the
desktop app persists data across restart.

## 7. Execution order

### Session 1 — Dashboard contract + implementation

- Manager: freeze dashboard contract.
- `backend` + `frontend`: implement in parallel against the contract.
- `qa`: author tests in parallel; run integration verification after both.

### Session 2 — Parity + E2E

- `qa`: build and run the parity harness.
- `qa`/`frontend`: convert Playwright specs; `devops` adds them to `testMatch`.
- `security`: review the new read-only dashboard surface.
- `manager`: resolve differences and update the matrix.

### Session 3 — Docs + release

- Docs cleanup.
- `devops`: verify script, build/package, backup/restore.
- Final QA pass, `git diff --check`, full verification.

## 8. Parallelism

| Session | Parallel tasks | Constraint |
|---|---|---|
| 1 | backend, frontend, qa test authoring | dashboard contract frozen; no shared files |
| 2 | parity harness, Playwright conversion, security review | implementations exist |
| 3 | docs, release build, final QA | docs after behavior is final |

Do not run two agents that write the same file family. Cap developer tasks at
2–3.

## 9. Exit criteria

- [ ] Dashboard 4 endpoints pass PostgreSQL + SQLite tests.
- [ ] Frontend dashboard tests and production build pass.
- [ ] All 67 endpoints have a parity result (pass or approved difference).
- [ ] Playwright covers inventory, clients, providers, sales, orders, dashboard.
- [ ] No stale “pending” claims in docs.
- [ ] `scripts/verify.sh` exists and passes.
- [ ] Wails smoke test passes; data persists after restart.
- [ ] Backup/restore rehearsal passes.
- [ ] Security has no open Critical/High findings.
- [ ] Working tree clean; releases tagged/pushed by the user.

## 10. Verification commands

```bash
gofmt -l internal cmd main.go app.go
go vet ./...
INVENTORY_TEST_DATABASE_URL="postgresql://inventory:inventory@localhost:5433/inventory_desktop_test" \
  go test ./... -count=1
go test -race ./...

cd frontend
pnpm test
pnpm lint
pnpm build
pnpm test:e2e

cd ..
git diff --check
```

## 11. Risks

| Risk | Mitigation |
|---|---|
| Dashboard SQL semantics differ between PG/SQLite | Freeze contract; dedicated boundary tests |
| Bogota day grouping uses host timezone | Explicit `-05:00`/`AT TIME ZONE` tests |
| Legacy service unavailable for parity | Record reason; replay saved fixtures; do not mark passed |
| Playwright specs were written for mocked routes | Convert to full-stack; add to `testMatch` |
| Release environment lacks signing credentials | Document blocked steps; ship unsigned dev build only with approval |
| Docs drift again | Update docs in the same change; weakest-true parity label |
