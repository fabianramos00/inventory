# Testing

Tests exist to prove contract parity, business correctness, and desktop
behavior. They are not optional polish.

## Layers

| Layer | Tool | Scope |
|---|---|---|
| Go unit | `testing` | Services, decimal math, status derivation, validation |
| Go handler | `httptest` | Routing, auth, status codes, JSON shape |
| Go integration | `testing` + PostgreSQL | Transactions, stock locking, migrations |
| Go repository contract | `testing` + PostgreSQL/SQLite | Same port cases against both adapters |
| Go migration | `testing` + SQLite | Schema rebuild and sale-payment backfill |
| Go bootstrap | `testing` | `DB_MODE` selection and SQLite fallback |
| Frontend unit | Vitest | Hooks, formatters, utilities, components |
| Frontend E2E | Playwright | Auth, users, inventory, sales, purchase orders, dashboard aggregates, floating menus, KPI strip |
| Contract parity | Go test | Same fixtures against FastAPI and Go (67 legacy endpoints) |

## Commands

```bash
# Go
go test ./...
go test -race ./...
go vet ./...
gofmt -l .

# One DSN for every PostgreSQL suite. The database name must end in "_test";
# testdb.ValidateTestDSN refuses anything else before opening a connection.
export INVENTORY_TEST_DATABASE_URL='postgresql://inventory:inventory@127.0.0.1:5432/inventory_desktop_test'

# Go integration (auth flow, real PostgreSQL 17)
go test -v ./internal/adapters/inbound/httpapi

# Go repository contract (PostgreSQL case skips when the env var is unset)
go test -v ./internal/adapters/outbound/persistence
go test -v ./internal/adapters/outbound/persistence   # SQLite contract only

# Go bootstrap and DB_MODE fallback
go test -v ./internal/bootstrap

# Frontend
cd frontend
pnpm lint
pnpm test
pnpm build
pnpm test:e2e
```

Run the checks that cover the changed area before committing. Run the full
suite before release. CI runs `make verify` plus one
`go test -race ./... -count=1`.

`scripts/verify.sh` is the only entry point that runs the Go suite **both
ways**: once with `INVENTORY_TEST_DATABASE_URL=` (SQLite and unit tests
only) and once with the variable set (PostgreSQL suites included).

## Go unit tests

- Table-driven.
- No database, no HTTP.
- Cover:
  - Payment status derivation (`pending`, `partial`, `paid`).
  - Delivery status derivation (`pending`, `partial`, `delivered`).
  - Sale/order total recalculation.
  - Decimal parsing and rounding.
  - `delivered_quantity <= quantity`.
  - `amount_paid <= total`.
  - Pagination helpers (`limit + 1`).
  - Password hashing and JWT claims.
- Each non-trivial branch has at least one case.

## Go handler tests

Use `httptest.NewServer` with a real router and fake services.

- Public routes succeed without auth; protected routes return `401`.
- Superuser routes return `403` for a regular user.
- Malformed JSON returns `422` with the normalized error shape.
- Unknown IDs return `404`.
- Money fields are JSON numbers with two decimals.
- Response field names match [API-CONTRACT.md](API-CONTRACT.md).

## PostgreSQL test database

All PostgreSQL suites read one variable, the one exported in
[Commands](#commands). Nothing else is configurable per suite.

Rules:

- **The database name must end in `_test`.** `tests/testdb.ValidateTestDSN`
  parses the DSN and refuses anything else *before* a connection is opened,
  because the suites truncate every business table. A DSN pointing at
  `inventory_desktop` or `postgres` fails with
  `the test database name must end in "_test"`.
- **The suites skip when the variable is unset.** `go test ./...` without
  `INVENTORY_TEST_DATABASE_URL` is green even when the PostgreSQL suites
  never ran. A skipped suite is not coverage. Use `scripts/verify.sh`
  (`make verify`), which runs the suite twice: with the variable cleared
  (SQLite plus unit tests) and with it set (PostgreSQL included).
- Never point the variable at the development or production database.

Creating the test database depends on who owns the role:

- **Locally** the `inventory` role is not a superuser, so it cannot create a
  database. Use the admin role:

  ```bash
  PGPASSWORD=postgres psql -h 127.0.0.1 -p 5432 -U postgres -d postgres \
    -c 'CREATE DATABASE inventory_desktop_test OWNER inventory'
  ```

- **In CI** the service sets `POSTGRES_USER=inventory`, which is the
  container superuser, so the documented command works as-is:

  ```bash
  PGPASSWORD=inventory psql -h localhost -p 5433 -U inventory -d inventory \
    -c 'CREATE DATABASE inventory_desktop_test'
  ```

  Port `5433` is the CI-only container port. The local development database
  is PostgreSQL 17 on `127.0.0.1:5432`; the compose database on `5433` is
  optional.

## Go integration tests

The integration test is gated by the environment variable; without it the
test skips instead of failing.

```bash
# INVENTORY_TEST_DATABASE_URL is exported above.
go test -v ./internal/adapters/inbound/httpapi
```

Rules:

- Apply goose migrations before the suite.
- Each test runs in its own transaction when possible, or truncates the
  affected tables after itself.
- Never run integration tests against the development or production database.
- Required cases:
  - Concurrent sale creation cannot oversell the last unit.
  - Sale creation decrements stock; deletion restores it.
  - Sale item add/update/remove adjust stock by the delta.
  - Receiving an order increments stock exactly once.
  - Receiving twice does not double stock.
  - Deleting a received order does not reverse stock (legacy behavior).
  - Unique SKU and identity card violations map to a readable error.
  - Dashboard queries return zero-filled 7-day data.
  - Sale creation with `amount_paid > 0` inserts exactly one opening payment
    (amount, method, token user, UTC timestamp); `amount_paid = 0` inserts none
    and rejects a supplied `payment_method`.
  - Partial and exact payments update `amount_paid`, `payment_status`, and
    `debt_amount` from `SUM(sale_payments.amount)`; overpayment and a payment
    on a paid sale return `422`.
  - Payment `amount` zero or negative, and a missing or invalid
    `payment_method`, return `422`.
  - Deleting a payment hard-deletes the row and recomputes the sale; a second
    delete returns `404`.
  - Deleting a sale cascades all of its payment rows.
  - `GET /sales?payment_method=` matches through `EXISTS`, including a sale
    whose matching payment is not the opening row.
  - `PUT /sales/{id}` rejects `amount_paid` and `payment_method` as unknown
    fields (`422`) while still accepting `client_id`.

## Repository contract tests

`internal/adapters/outbound/persistence/contract_test.go` runs one shared suite
through the GORM store against both backends, so PostgreSQL and SQLite cannot
drift behind the same `internal/ports/outbound` interfaces.

- PostgreSQL: opens `db.OpenPostgresGORM`, uses `INVENTORY_TEST_DATABASE_URL`,
  applies goose migrations on the exposed `*sql.DB`, and truncates all
  auth/business tables before the suite. It skips when the variable is unset.
- SQLite: opens `db.OpenSQLiteGORM` on a fresh database under `t.TempDir()`,
  applies the SQLite goose migrations, and needs no external server.
- The contract suite and the `httpapi` auth integration test share the test
  database; both take the PostgreSQL advisory lock in `tests/testdb` so
  parallel package test binaries cannot truncate each other's fixtures.
- The sale suite includes the payment ledger: opening row, `id DESC` paging,
  `SumPayments`, `EXISTS` method filter, delete recompute, second-delete
  `NotFound`, and sale-delete cascade must pass against both adapters.

Refresh-token timestamps use `gormstore.Timestamp`, which writes a fixed-width
UTC format (`2006-01-02T15:04:05.000000000Z`) so the lexicographic
`expires_at > ?` comparison stays chronological on SQLite and PostgreSQL. The
read path parses with `time.RFC3339Nano` and normalizes to UTC, so values with
a zero fraction or trimmed trailing zeros are accepted instead of failing.

## Migration and backfill tests

`internal/db/sale_payments_migration_test.go` migrates a SQLite database to
version 6, seeds legacy sales, then runs `00007_sale_payments`. It must prove:

- `SUM(sale_payments.amount) = sales.amount_paid` for every sale.
- Sales with `amount_paid = 0` get no payment rows.
- Each paid legacy sale gets exactly one opening payment, copying amount,
  method, creator, and sale timestamp; a null legacy method stays null.
- The SQLite `sales` rebuild drops `payment_method` and its CHECK/index while
  preserving ids, rows, STRICT mode, remaining constraints, and indexes.
- Deleting a sale cascades its payment rows; `PRAGMA foreign_key_check` is
  clean and new payments enforce positive amounts and valid methods.

## Playwright specs

`frontend/e2e` runs the real React app against the real Go API: the API is
started on `127.0.0.1:8787` and the Vite app on `127.0.0.1:4173`. The API
uses `DATABASE_URL`, then `INVENTORY_TEST_DATABASE_URL`, and finally the
local `inventory_desktop_test` on `127.0.0.1:5432`, so the suite never
writes to the development database.

`frontend/playwright.config.ts` lists the full-stack specs in `testMatch`. A
new spec file that is not listed is never executed, so register it there.

Layout regressions are covered by two specs:

- `e2e/floating-menu.spec.ts` — at 1280×720 the last attribute dropdown of
  `/#/inventory/create` opens without growing the document
  (`document.scrollingElement.scrollHeight` is unchanged), stays fully inside
  the viewport, and is portaled to `body`. The second case covers the sale
  product search: the results list of `/#/sales/create` is a floating menu
  too, so it does not grow the page either.
- `e2e/kpi-strip.spec.ts` — on `/#/` the four KPI cards share one row, the
  lead amount stays on a single line with the ellipsis contract
  (`data-overflow="ellipsis"`, `white-space: nowrap`,
  `text-overflow: ellipsis`) and its text is intact, and the two count
  columns are narrower than the two money columns.

Sale-create layout expectations:

- The product search results are a floating menu portaled to the body
  (`#product-search-listbox`), anchored to the search input and as wide as
  it. They never take part in the form flow.
- The payment method dropdown is a portaled floating menu as well, so its
  options live outside the form (a click on an option is not a click inside
  the form).

## Contract parity

The FastAPI app is the reference. The Go API must match it.

Harness: `tests/contract` (build tag `contract`). Fixtures:
`testdata/contract/`.

```text
testdata/contract/
  auth/auth.json
  users/users.json
  inventory/catalog.json
  inventory/products.json
  clients/clients.json
  sales/sales.json
  supply-chain/providers.json
  supply-chain/supplier-catalogs.json
  supply-chain/purchase-orders.json
  dashboard/dashboard.json
  replay/                 # optional replay recordings
```

Each fixture contains `name`, `method`, `path`, `auth` (`none`, `superuser`,
or `user`), `body`, and `expect.status`. Optional keys:

- `capture`: maps a variable name to a response path (`id`, `items.0.id`,
  `user.id`). Later requests use `{{variable}}` in paths and bodies.
- `on_conflict`: when a create returns one of the listed statuses, the runner
  looks the resource up by natural key and reuses it. This keeps `PARITY-`
  named seed data reusable without cleanup.
- `compare`: `full` (default), `shape`, or `status`. Global aggregates use
  `shape` by default; set `CONTRACT_STRICT=1` to use their `strict_compare`
  mode (`full`) against an isolated legacy database.
- `content_type`: `form` for `/auth/login`; JSON otherwise.

The runner:

1. Starts an in-process Go `httptest` server on a temporary SQLite database
   (no port 8787 conflict).
2. Reads the legacy user profile from `POST /auth/login` and seeds matching
   Go users, so user fields compare without fixtures depending on local names.
3. Authenticates both sides and sends the same request to the Go server and
   `LEGACY_API_URL`.
4. Seeds through the API with stable `PARITY-` natural keys and captures
   generated IDs per side. No direct legacy database access.
5. Compares status, JSON object shape, array ordering, and values. Generated
   IDs, timestamps, and tokens are normalized; money is compared as a decimal.
6. Writes `testdata/contract/report.md` on failure, including the request,
   path-level differences, and skipped fixtures.

### Commands

```bash
# Live run. LEGACY_API_URL may include /api/v1 or omit it.
LEGACY_API_URL=http://127.0.0.1:8011 \
CONTRACT_ALLOW_WRITES=1 CONTRACT_STRICT=1 \
LEGACY_USER=... LEGACY_PASSWORD=... \
LEGACY_REGULAR_USER=... LEGACY_REGULAR_PASSWORD=... \
  go test -tags=contract ./tests/contract -count=1

# Whole module tree with the tag (contract test skips when the legacy target
# and replay file are absent).
go test -tags=contract ./... -count=1

# Record legacy responses, then replay them without a live FastAPI instance.
CONTRACT_RECORD_DIR=/tmp/contract-replay \
LEGACY_API_URL=http://127.0.0.1:8011 CONTRACT_ALLOW_WRITES=1 \
LEGACY_USER=... LEGACY_PASSWORD=... \
  go test -tags=contract ./tests/contract -count=1
CONTRACT_REPLAY_FILE=/tmp/contract-replay/replay.json \
  go test -tags=contract ./tests/contract -count=1
```

`CONTRACT_ALLOW_WRITES=1` is mandatory for a live run because the harness
creates, updates, and deletes resources. Use only a disposable legacy
instance and database. Never point it at production or at the user's existing
`inventory` database. Recorded replay files redact token values, but treat
them as test data and do not commit credentials.

`LEGACY_REGULAR_USER`/`LEGACY_REGULAR_PASSWORD` are optional. Without them
the regular-user `403` fixture is skipped with a recorded reason. Money and
timestamps are normalized only for comparison; the harness never rewrites
responses.

### Parity matrix

| Module | Endpoints | Status |
|---|---:|---|
| Auth | 3 | Passed |
| Users | 5 | Passed |
| Inventory — catalog | 18 | Passed |
| Inventory — products | 6 | Passed |
| Clients | 5 | Passed |
| Sales | 10 | Passed |
| Supply chain — providers | 5 | Passed |
| Supply chain — supplier catalogs | 2 | Passed |
| Supply chain — purchase orders | 9 | Passed |
| Dashboard | 4 | Passed |
| **Total** | **67** | **Passed** |

`TestFixtureCoverage` fails if a fixture file drops one of the 67 endpoints,
if a module declares the wrong count, or if an unlisted endpoint appears.
Update the status column only after a verified run.

### Sale payment ledger (Go-only)

`GET /api/v1/sales/{id}/payments`, `POST /api/v1/sales/{id}/payments`, and
`DELETE /api/v1/sales/{id}/payments/{payment_id}` are an approved additive
divergence. The legacy API has no such routes, so they stay outside the
67-endpoint matrix and no fixture targets them. Cover them with Go service,
handler, integration, and repository tests instead.

The legacy sale-level `payment_method` is normalized away on the legacy side
of sale comparisons because the Go sale response omits it; payment rows keep
their own `payment_method`.

## Remaining risks

- The PostgreSQL `00007_sale_payments` backfill and `sales.payment_method`
  drop are not automated. Only the SQLite migration is exercised; validate the
  PostgreSQL path on a disposable database copy before release.
- A plain `go test ./...` skips every PostgreSQL suite when
  `INVENTORY_TEST_DATABASE_URL` is unset and still reports success. Run
  `scripts/verify.sh` (or export the variable) before trusting a green run.

## Accessibility checks

- Every input has a label.
- Dialogs have `role="dialog"`, `aria-modal`, a title, focus trap, and focus
  restore.
- Dropdowns expose `aria-expanded`, `aria-controls`, and keyboard navigation.
- Errors use `role="alert"`.
- Skip link targets `#main-content`.
- Contrast and focus rings survive the es-CO/COP changes.

## Definition of Done for tests

- The changed behavior has at least one test.
- A bug fix has a regression test that fails before the fix.
- Contract parity is updated when an endpoint changes.
- A PostgreSQL suite that skipped because `INVENTORY_TEST_DATABASE_URL` was
  unset is not coverage: run it with the variable set.
- New tests are deterministic and do not depend on wall-clock time or remote
  services.
