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
| Go bootstrap | `testing` | `DB_MODE` selection and SQLite fallback |
| Frontend unit | Vitest | Hooks, formatters, utilities, components |
| Frontend E2E | Playwright | Login, inventory, sales, orders, users, keyboard |
| Contract parity | Go test | Same fixtures against FastAPI and Go |

## Commands

```bash
# Go
go test ./...
go test -race ./...
go vet ./...
gofmt -l .

# Go integration (auth flow, real PostgreSQL 17)
INVENTORY_TEST_DATABASE_URL='postgresql://inventory:inventory@localhost:5433/inventory_desktop_test' \
  go test -v ./internal/httpapi

# Go repository contract (PostgreSQL case skips when the env var is unset)
INVENTORY_TEST_DATABASE_URL='postgresql://inventory:inventory@localhost:5433/inventory_desktop_test' \
  go test -v ./internal/adapters/persistence
go test -v ./internal/adapters/persistence   # SQLite contract only

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
suite before release.

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

## Go integration tests

Use a dedicated PostgreSQL 17 database from `docker compose`:

```bash
export INVENTORY_TEST_DATABASE_URL="postgresql://inventory:inventory@localhost:5433/inventory_desktop_test"
go test -v ./internal/httpapi
```

The integration test is gated by the environment variable; without it the
test skips instead of failing.

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

## Repository contract tests

`internal/adapters/persistence/contract_test.go` runs one shared suite against
both persistence adapters, so PostgreSQL and SQLite cannot drift behind the
same `internal/ports` interfaces.

- PostgreSQL: uses `INVENTORY_TEST_DATABASE_URL`, applies goose migrations, and
  truncates `refresh_tokens`/`users`. It skips when the variable is unset.
- SQLite: uses a fresh database under `t.TempDir()`, applies the SQLite goose
  migrations, and needs no external server.
- The contract suite and the `httpapi` auth integration test share the test
  database; both take the PostgreSQL advisory lock in `tests/testdb` so
  parallel package test binaries cannot truncate each other's fixtures.

Cases:

- User create, get by ID, get by email, update, and `domain.ErrNotFound` for
  unknown ID/email and for updating a missing row.
- Refresh-token create, find-valid, rotation, and revocation.
- The old token is rejected after rotation; an already-rotated or expired token
  cannot be rotated again.
- An expired token is rejected by `FindValid`.
- Token rows return the user ID they were created for (two users checked).

SQLite currently only has the `users` and `refresh_tokens` tables. The rest of
the business schema is Phase 3 work, so contract tests must not assume
inventory, sales, orders, or dashboard tables exist on the SQLite adapter.

Refresh-token timestamps are written with a fixed-width UTC format
(`2006-01-02T15:04:05.000000000Z`) so the lexicographic `expires_at > ?`
comparison stays chronological. The read path parses with `time.RFC3339Nano`
and normalizes to UTC, so values with a zero fraction or trimmed trailing
zeros are accepted instead of failing. Coverage:

- `internal/adapters/persistence/sqlite/values_test.go` verifies the
  fixed-width writer and round trip for 0, trailing-zero, and full
  nanoseconds.
- `internal/adapters/persistence/sqlite/refresh_tokens_test.go` runs
  `FindValid`/`Rotate` against a temporary database with driver-normalized
  expiry strings for those three shapes, and checks both validity and
  `domain.ErrNotFound` after expiry.

## Bootstrap and fallback tests

`internal/bootstrap/database_test.go` checks the `DB_MODE` contract without a
PostgreSQL server:

- `DB_MODE=sqlite` opens a temp database, applies migrations, and returns
  `Backend "sqlite"` with working repositories.
- `DB_MODE=postgres` with an unreachable DSN returns an error and no stores.
- `DB_MODE=auto` with an unreachable DSN falls back to SQLite and returns
  `Backend "sqlite"`.
- SQLite creates its parent directory with mode `0700` and the database file
  with mode `0600`.

## Phase 2 auth coverage

The following is covered now:

- JWT round-trip, signature failure, and expiry.
- Refresh-token randomness, SHA-256 hashing, rotation, old-token rejection,
  logout revocation, inactive-user rejection.
- Login handler: form-urlencoded input, exact 401 details, rate limiting,
  normalized error envelope, no password-hash serialization.
- Bearer enforcement on protected routes, dev-mode CORS origin gating, and
  rejection of dev origins when dev mode is off.
- `Cache-Control: no-store` and `Pragma: no-cache` on login, refresh, and
  logout responses.
- Config: blank secret generation, placeholder rejection, short-secret
  rejection, blank env fallback, config file mode hardening to `0600` on read,
  and symlink rejection.
- HTTP server: rejects non-loopback listen addresses and accepts
  `127.0.0.1:0`.
- Frontend: rotated refresh token storage, `auth:refreshed` state sync,
  fail-closed production bootstrap, es-CO/COP formatters.
- Playwright: real login → Home → logout against `cmd/server` and the test
  database. The seed reads `SEED_EMAIL`/`SEED_PASSWORD`/`SEED_NAME` from the
  environment so the password never appears in argv, and the test secret is
  at least 32 bytes.

Deferred until Phase 3: inventory CRUD, sales/stock, purchase orders,
dashboard, user management, and the full 67-endpoint parity matrix.

## Frontend tests

Vitest + Testing Library:

- Hook tests: loading, success, error, retry, pagination, invalidation.
- Auth interceptor: one refresh for concurrent 401s, queue drain, logout on
  refresh failure, no refresh loop.
- Formatters: `es-CO`, COP, `DD/MM/YYYY`.
- Component tests: dialogs trap focus, Escape closes, labels are associated,
  errors have `role="alert"`.
- Form validation: required fields, numeric ranges, delivered vs quantity,
  amount paid vs total.

Playwright:

- Login success and failure.
- Inventory list, filter, create, edit, delete.
- Sale creation with insufficient stock.
- Sale item edit and deletion restore stock.
- Purchase order receiving.
- User creation and role visibility.
- Keyboard-only dialog and dropdown flows.

Use hash URLs (`/#/inventory`) for the desktop router. Run against Vite dev
plus a test Go API. Add a manual Wails smoke test before release: launch,
login, create a sale, restart, confirm data persists.

## Contract parity

The FastAPI app is the reference. The Go API must match it.

```text
testdata/contract/
  auth/
  inventory/
  sales/
  supply-chain/
  dashboard/
  users/
```

Each fixture contains:

```json
{
  "method": "GET",
  "path": "/api/v1/inventory/products?limit=10",
  "auth": "superuser",
  "body": null,
  "expect": { "status": 200 }
}
```

The parity test:

1. Seeds identical data in both databases.
2. Sends the same request to `LEGACY_API_URL` and the Go test server.
3. Compares status code, JSON object shape, array ordering, and values.
4. Normalizes timestamps and generated IDs before comparison.
5. Compares money as decimals, never floats.
6. Writes a diff report to `testdata/contract/report.md` on failure.

Environment:

```bash
LEGACY_API_URL=https://inventory-api-2rq4.onrender.com/api/v1 \
LEGACY_USER=... LEGACY_PASSWORD=... \
go test -tags=contract ./...
```

If the legacy service is unavailable, record the reason and run the saved
fixtures in replay mode. Do not mark parity as passing without a run.

### Parity matrix

| Module | Endpoints | Status |
|---|---:|---|
| Auth | 3 | Covered (Phase 2) |
| Users | 5 | Pending |
| Inventory | 24 | Pending |
| Sales + clients | 15 | Pending |
| Supply chain | 16 | Pending |
| Dashboard | 4 | Pending |
| **Total** | **67** | Pending |

Update the status column as each module passes. The matrix is the release
gate.

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
- New tests are deterministic and do not depend on wall-clock time or remote
  services.
