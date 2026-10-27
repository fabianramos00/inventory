# Backend

The backend is a Go REST API embedded in the Wails process. It mirrors the
legacy FastAPI contract and owns database access through backend-specific
adapters.

## Stack

- Go 1.26.
- Standard library `net/http` with `http.ServeMux` method patterns.
- `pgx/v5` + `pgxpool` for PostgreSQL.
- `database/sql` + `modernc.org/sqlite` for the local SQLite fallback.
- `goose` for embedded SQL migrations.
- `golang-jwt/jwt/v5` for access tokens.
- `golang.org/x/crypto/bcrypt` for passwords.
- `shopspring/decimal` for money.
- `log/slog` for structured logs.
- `testing` + `httptest` for tests.

No ORM. No web framework. No new dependency without an entry in
[DECISIONS.md](DECISIONS.md).

## Layout

```text
main.go                 Wails entry and server lifecycle
app.go                  Wails bindings (GetAPIBaseURL)
internal/
  httpapi/
    router.go           route table
    middleware.go       auth, CORS, recovery, request ID, logging
    response.go         JSON and error helpers
    handlers/           one file per module
  service/
    auth.go             auth use cases
    inventory.go
    sales.go
    supply_chain.go
    dashboard.go
    users.go
  ports/
    users.go            UserRepository interface
    refresh_tokens.go   RefreshTokenRepository interface
  domain/
    user.go             User, RefreshToken
    errors.go           ErrNotFound and other domain errors
  adapters/
    persistence/
      postgres/
        users.go
        refresh_tokens.go
      sqlite/
        users.go
        refresh_tokens.go
  auth/
    jwt.go
    password.go
    refresh.go
  config/
    config.go
  db/
    postgres.go         OpenPostgres, MigratePostgres
    sqlite.go           OpenSQLite, MigrateSQLite
  bootstrap/
    database.go         DB_MODE selection and store wiring
migrations/
  postgres/
  sqlite/
```

Dependency direction: `httpapi → service → ports ← adapters → db`. The
domain package is shared. Services depend only on port interfaces. Handlers
never issue SQL. Repositories never write HTTP responses.

## HTTP layer

- Register routes with method patterns: `mux.HandleFunc("GET /api/v1/sales/{id}", h)`.
- Path values: `r.PathValue("id")`.
- Decode JSON with `json.NewDecoder`, reject unknown fields where practical,
  and enforce a body size limit (10 MiB, matching the legacy middleware).
- Always set `Content-Type: application/json`.
- Use `http.MaxBytesReader` for request bodies.
- Timeouts: read header 5s, read 15s, write 30s, idle 60s.
- Graceful shutdown with a 10s context.
- Request ID middleware adds `X-Request-Id`; do not log bodies.

Middleware order:

```text
recovery → request ID → logging → CORS → auth → handler
```

Auth whitelist: `/api/v1/auth/login`, `/api/v1/auth/refresh`,
`/api/v1/auth/logout`, `GET /healthz`.

## Service layer

Services own business rules and transactions:

- Validate references and state transitions.
- Calculate sale/order totals and statuses.
- Apply stock changes with row locks.
- Recalculate aggregate fields after item mutations.
- Return typed domain errors that handlers map to HTTP status codes.

Do not put business rules in handlers or repositories. Do not start a
transaction in a repository method that is called inside a service
transaction.

## Repository adapters

Ports live in `internal/ports` and are consumed by services. Each backend
implements the same interfaces:

- `internal/adapters/persistence/postgres` uses `pgxpool.Pool`, `pgx.Tx`, and
  explicit SQL.
- `internal/adapters/persistence/sqlite` uses `database/sql` and
  `modernc.org/sqlite`.

Rules:

- Use `QueryRow`, `Query`, and `pgx.CollectRows` in PostgreSQL.
- Use `pgx.Tx` for multi-statement writes; use `*sql.Tx` in SQLite.
- Parameterized queries only. Never concatenate SQL values.
- Pagination fetches `limit + 1` rows and returns `(items, hasNext)`.
- Search uses the legacy columns and `ILIKE` semantics; cap input length.
- Lock products with `SELECT ... FOR UPDATE` before stock changes.
- Map `pgx.ErrNoRows` and `sql.ErrNoRows` to `domain.ErrNotFound`.
- SQLite booleans are scanned as integers; timestamps are stored as fixed-width
  UTC strings so comparisons and round-trips stay correct.
- Scan `NUMERIC` into a decimal, never into `float64`:

```go
var raw string
if err := row.Scan(&raw); err != nil { ... }
amount, err := decimal.NewFromString(raw)
```

Use `pgtype.Numeric` only when a query needs its integer/scale parts. Add a
compile-time interface assertion when implementing a port.

## Data model

Domain structs live in `internal/domain` and carry no database or HTTP
coupling. Keep JSON tags aligned with [API-CONTRACT.md](API-CONTRACT.md).

Money fields are `decimal.Decimal`. JSON marshaling must emit a number.
Implement a small `Money` wrapper only if both encoding and scanning are
simpler with it; otherwise marshal `decimal.Decimal` through a custom type at
the HTTP boundary.

Dates are `time.Time` in UTC. Format responses with `time.RFC3339`.

## Auth

- Access token: HS256 JWT, `sub` = user ID, expiry 30 minutes.
- Refresh token: 32 random bytes, URL-safe base64; store only a SHA-256 hash.
- Refresh rotates the token and revokes the old row in one transaction.
- Logout revokes the presented refresh token.
- Login accepts form data and uses the email in `username`.
- bcrypt cost from config, default 12; tests use 4.
- Reject inactive users on login and on every authenticated request.
- `is_superuser` is required for all `/users` routes.

Never log passwords, tokens, authorization headers, or full request bodies.

## Transactions and stock

Sale creation:

```text
BEGIN
  lock products for update
  verify stock for every item
  decrement stock
  insert sale
  insert items
COMMIT
```

Any failure rolls back the whole sale. The same rule applies to sale item
mutations and order receiving.

Order receiving:

```text
BEGIN
  lock order
  if old status != received and new status == received:
    for each item:
      increment product stock
      upsert supplier catalog (cost, lead time, supplier SKU)
COMMIT
```

Status and totals are derived on the server. Never trust client totals.

## Configuration

Load from environment, then the OS config file, then defaults. See
[ARCHITECTURE.md](ARCHITECTURE.md). `DB_MODE` selects `auto` (default),
`postgres`, or `sqlite`. In `auto`, PostgreSQL is probed with a 3-second
timeout and SQLite is opened on failure. Validate the DSN at startup when it
is used. Generate and persist `SECRET_KEY` when missing.

## Migrations

- SQL files live under `migrations/postgres/` and `migrations/sqlite/`, each
  embedded with `embed.FS`.
- Naming: `00001_initial_schema.sql`, `00002_add_index.sql`.
- PostgreSQL keeps the legacy version at `1` so existing databases do not
  re-run it.
- SQLite uses goose dialect `sqlite3` and currently contains only the auth
  tables.
- Full SQLite business-schema parity is Phase 3 work; do not translate the
  remaining tables piecemeal while porting modules.
- Every migration is forward-only unless a rollback is safe and tested.
- Run at startup when `AUTO_MIGRATE=true`; otherwise expose an explicit
  migrate command.
- Never edit a migration that has shipped.

## Errors

Define a small set of sentinel errors or a typed `AppError`:

```go
type AppError struct {
    Status int
    Code   string
    Detail string
}
```

Map them in one response helper to
`{"detail": "...", "code": "..."}`. Unknown errors return `500` with a
generic detail; log the cause with the request ID.

## Logging

- `slog` with JSON in production and text in development.
- Include request ID, method, path, status, and duration.
- Exclude query strings that may contain sensitive values.
- Never log tokens, passwords, or customer PII.

## Commands

```bash
go test ./...
go vet ./...
gofmt -w .
go run .            # dev once the Wails scaffold exists
wails build
```

## Boundaries

- Backend owns `main.go`, `app.go`, `internal/**`, `migrations/**`,
  `go.mod`, and `go.sum`.
- Do not edit `frontend/**`, `wails.json`, build scripts, or CI.
- Request schema changes through `database`; implement them here.
- Report contract changes to `frontend` and `qa`.
