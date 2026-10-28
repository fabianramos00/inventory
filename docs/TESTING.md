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
  go test -v ./internal/adapters/inbound/httpapi

# Go repository contract (PostgreSQL case skips when the env var is unset)
INVENTORY_TEST_DATABASE_URL='postgresql://inventory:inventory@localhost:5433/inventory_desktop_test' \
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

### Application-layer port tests

`internal/application/auth_test.go` tests `AuthService` through fakes for every
outbound port (`UserRepository`, `RefreshTokenRepository`, `PasswordHasher`,
`AccessTokenIssuer`, `RefreshTokenService`). No database, HTTP server, JWT
library, or bcrypt is involved.

Covered:

- Login success returns the pair and user, and persists the hashed refresh
  token with the right user ID and a future expiry.
- Login rejects a wrong password (`domain.ErrInvalidCredentials`) and an
  inactive user (`domain.ErrInactiveUser`) without persisting a token.
- Refresh rotates the old hash, returns the new pair, and keeps the rotation
  expiry at `now + TTL`.
- Refresh rejects unknown/expired tokens and inactive users with
  `domain.ErrInvalidRefresh`, without rotating.
- Logout revokes the hashed token; an empty token is a no-op.
- `AuthenticateAccessToken` returns the user, and rejects bad tokens and
  inactive users with `domain.ErrInvalidAccess`.
- Compile-time assertion that `AuthService` satisfies `inbound.AuthUseCase`.

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
go test -v ./internal/adapters/inbound/httpapi
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

`internal/adapters/outbound/persistence/contract_test.go` runs one shared suite
through the GORM store against both backends, so PostgreSQL and SQLite cannot
drift behind the same `internal/ports/outbound` interfaces.

- PostgreSQL: opens `db.OpenPostgresGORM`, uses `INVENTORY_TEST_DATABASE_URL`,
  applies goose migrations on the exposed `*sql.DB`, and truncates
  `refresh_tokens`/`users`. It skips when the variable is unset.
- SQLite: opens `db.OpenSQLiteGORM` on a fresh database under `t.TempDir()`,
  applies the SQLite goose migrations, and needs no external server.
- The contract suite and the `httpapi` auth integration test share the test
  database; both take the PostgreSQL advisory lock in `tests/testdb` so
  parallel package test binaries cannot truncate each other's fixtures.

Cases:

- User create, get by ID, get by email, update, and `domain.ErrNotFound` for
  unknown ID/email and for updating a missing row.
- User list orders by ID, computes `has_next` from `limit + 1`, honors
  `skip`, and searches names/emails case-insensitively on both backends.
- User delete returns the deleted row, and a later get or delete returns
  `domain.ErrNotFound`.
- Duplicate email maps to `domain.ErrConflict`.
- PostgreSQL only: deleting a user referenced by a provider row maps to
  `domain.ErrForeignKeyViolation` and leaves the user in place.
- Refresh-token create, find-valid, rotation, and revocation.
- The old token is rejected after rotation; an already-rotated or expired token
  cannot be rotated again.
- An expired token is rejected by `FindValid`.
- Token rows return the user ID they were created for (two users checked).
- Catalog CRUD for categories, brands, materials, and measurement units on both
  backends: create, get, update, delete, and `domain.ErrNotFound` for missing
  rows and updates.
- Duplicate category name/code, brand name, material name, and unit
  name/abbreviation map to `domain.ErrConflict`.
- Deleting a category, brand, material, or unit referenced by a product maps to
  `domain.ErrForeignKeyViolation` and leaves the row in place.
- Catalog list orders by `id DESC`, computes `has_next` from `limit + 1`,
  honors `skip`, searches `name` only, and matches accented terms with different
  casing on SQLite and PostgreSQL.

SQLite now has the full Phase 3 business schema (`00001_auth.sql` plus
`00002_catalog.sql` through `00006_sales.sql`). Go service, handler, and
persistence contract coverage for catalog exists on both backends; the live
legacy FastAPI fixture run is still pending. Product, sales, order, and
dashboard repositories are still pending.

Refresh-token timestamps use `gormstore.Timestamp`, which writes a fixed-width
UTC format (`2006-01-02T15:04:05.000000000Z`) so the lexicographic
`expires_at > ?` comparison stays chronological on SQLite and PostgreSQL. The
read path parses with `time.RFC3339Nano` and normalizes to UTC, so values with
a zero fraction or trimmed trailing zeros are accepted instead of failing.
Coverage:

- `internal/adapters/outbound/persistence/gormstore/timestamp_test.go` verifies
  the fixed-width writer and round trip for 0, trailing-zero, and full
  nanoseconds, plus driver-normalized strings.
- `internal/adapters/outbound/persistence/gormstore/refresh_tokens_test.go`
  runs `FindValid`/`Rotate` against a temporary database with normalized expiry
  strings for those three shapes, and checks both validity and
  `domain.ErrNotFound` after expiry.
- `internal/adapters/outbound/persistence/gormstore/users_test.go` checks that
  a nullable `users.full_name` reads back as an empty string.

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

### Users CRUD slice

`internal/application/users_test.go` covers `UserService` through fake
`UserRepository`/`PasswordHasher` ports: list pass-through, create defaults
and password hashing, duplicate email, email/password validation, partial
update (empty password preserves the hash), update conflict, delete result,
and not-found paths.

`internal/adapters/inbound/httpapi/users_test.go` covers superuser-only access
on all five routes, the exact legacy `403` detail, list cursor pagination,
CRUD success, duplicate email `400`, `404 User not found`, and the
`FOREIGN_KEY_VIOLATION` code on delete.

### Catalog slice

`internal/application/catalog_test.go` covers create validation boundaries for
every catalog entity (150-character names, 10-character category code,
500-character description, 255-character logo URL, 5-character abbreviation,
empty required fields), partial updates that keep omitted fields and clear
empty optional strings, conflict/not-found propagation, and pagination/search
pass-through.

`internal/adapters/inbound/httpapi/catalog_test.go` covers bearer auth on all 18
routes, the four exact `404` details, `UNIQUE_VIOLATION` and
`FOREIGN_KEY_VIOLATION` codes, `422 VALIDATION_ERROR` for bad ids, query
parameters, malformed JSON, and unknown fields, the `{items, has_next, skip,
limit}` list shape, `200` create/update/delete responses, and the intentional
absence of `GET` by id for materials and measurement units (404).

`frontend/e2e/users.spec.ts` runs the full stack: real login, create a unique
user from the Usuarios page, search, rename, toggle active, and delete.

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

Deferred: product CRUD and stats, sales/stock, purchase orders, dashboard, and
the full 67-endpoint parity matrix. The Users CRUD slice and the catalog slice
are covered now (application, handler, repository contract, and, for Users,
full-stack E2E).

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
- Users CRUD against the real API: create, search, rename, toggle active,
  delete.
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
| Users | 5 | Covered (this slice) |
| Inventory — catalog | 18 | Backend contract covered; legacy FastAPI parity pending |
| Inventory — products | 6 | Pending |
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
