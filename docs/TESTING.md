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
| Frontend E2E | Playwright | Auth, users, inventory, sales, purchase orders, dashboard aggregates |
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
suite before release. CI runs `make verify` plus one
`go test -race ./... -count=1`.

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
  applies goose migrations on the exposed `*sql.DB`, and truncates all
  auth/business tables before the suite. It skips when the variable is unset.
- SQLite: opens `db.OpenSQLiteGORM` on a fresh database under `t.TempDir()`,
  applies the SQLite goose migrations, and needs no external server.
- The contract suite and the `httpapi` auth integration test share the test
  database; both take the PostgreSQL advisory lock in `tests/testdb` so
  parallel package test binaries cannot truncate each other's fixtures.

Refresh-token timestamps use `gormstore.Timestamp`, which writes a fixed-width
UTC format (`2006-01-02T15:04:05.000000000Z`) so the lexicographic
`expires_at > ?` comparison stays chronological on SQLite and PostgreSQL. The
read path parses with `time.RFC3339Nano` and normalizes to UTC, so values with
a zero fraction or trimmed trailing zeros are accepted instead of failing.

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
